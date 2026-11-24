package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

// ---------------------------------------------------------------- tenants

// CreateTenant inserts a tenant.
func (s *Store) CreateTenant(ctx context.Context, slug, name string) (*model.Tenant, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !validSlug(slug) {
		return nil, Invalidf("tenant slug must be 2-63 chars of a-z, 0-9 and '-'")
	}
	if strings.TrimSpace(name) == "" {
		return nil, Invalidf("tenant name is required")
	}
	t := &model.Tenant{ID: newID(), Slug: slug, Name: strings.TrimSpace(name), CreatedAt: s.now()}
	_, err := s.c(ctx).exec(ctx, `INSERT INTO tenants (id, slug, name, created_at) VALUES (?, ?, ?, ?)`, t.ID, t.Slug, t.Name, t.CreatedAt)
	if err != nil {
		return nil, mapErr(err, "tenant")
	}
	return t, nil
}

func validSlug(s string) bool {
	if len(s) < 2 || len(s) > 63 || s[0] == '-' {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

const tenantCols = `id, slug, name, created_at, suspended_at`

func scanTenant(r interface{ Scan(...any) error }) (*model.Tenant, error) {
	var t model.Tenant
	var susp sql.NullTime
	if err := r.Scan(&t.ID, &t.Slug, &t.Name, &t.CreatedAt, &susp); err != nil {
		return nil, err
	}
	t.CreatedAt = t.CreatedAt.UTC()
	t.SuspendedAt = timePtr(susp)
	return &t, nil
}

// GetTenant returns a tenant by id.
func (s *Store) GetTenant(ctx context.Context, id string) (*model.Tenant, error) {
	t, err := scanTenant(s.c(ctx).row(ctx, `SELECT `+tenantCols+` FROM tenants WHERE id = ?`, id))
	return t, mapErr(err, "tenant")
}

// GetTenantBySlug returns a tenant by slug.
func (s *Store) GetTenantBySlug(ctx context.Context, slug string) (*model.Tenant, error) {
	t, err := scanTenant(s.c(ctx).row(ctx, `SELECT `+tenantCols+` FROM tenants WHERE slug = ?`, strings.ToLower(slug)))
	return t, mapErr(err, "tenant")
}

// ListTenants returns all tenants.
func (s *Store) ListTenants(ctx context.Context) ([]model.Tenant, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+tenantCols+` FROM tenants ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Tenant{}
	for rows.Next() {
		t, err := scanTenant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// UpdateTenant renames a tenant.
func (s *Store) UpdateTenant(ctx context.Context, id, name string) (*model.Tenant, error) {
	if strings.TrimSpace(name) == "" {
		return nil, Invalidf("tenant name is required")
	}
	res, err := s.c(ctx).exec(ctx, `UPDATE tenants SET name = ? WHERE id = ?`, strings.TrimSpace(name), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, mapErr(sql.ErrNoRows, "tenant")
	}
	return s.GetTenant(ctx, id)
}

// DeleteTenant removes a tenant and all its data.
func (s *Store) DeleteTenant(ctx context.Context, id string) error {
	return s.tx(ctx, func(c conn) error {
		if err := legalHoldGuard(ctx, c, `SELECT COUNT(*) FROM projects WHERE tenant_id = ? AND legal_hold = ?`, id); err != nil {
			return err
		}
		// Tables carrying tenant_id without FK to tenants.
		for _, t := range []string{"audit_log", "audit_heads", "audit_checkpoints", "audit_backfill", "sessions", "comments", "revisions"} {
			if _, err := c.exec(ctx, `DELETE FROM `+t+` WHERE tenant_id = ?`, id); err != nil {
				return err
			}
		}
		// the tenant's MFA policy (its scope is the tenant id) and pending
		// sign-ins into it
		if _, err := c.exec(ctx, `DELETE FROM mfa_policies WHERE scope = ?`, id); err != nil {
			return err
		}
		if _, err := c.exec(ctx, `DELETE FROM mfa_challenges WHERE tenant_id = ?`, id); err != nil {
			return err
		}
		res, err := c.exec(ctx, `DELETE FROM tenants WHERE id = ?`, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "tenant")
		}
		return nil
	})
}

// ---------------------------------------------------------------- users

const userCols = `id, email, display_name, password_hash, super_admin, auth_provider, external_id, disabled, created_at, preferences, avatar, must_change_password, password_changed_at`

func scanUser(r interface{ Scan(...any) error }) (*model.User, error) {
	var u model.User
	var prefs string
	var changed sql.NullTime
	if err := r.Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.SuperAdmin, &u.AuthProvider, &u.ExternalID, &u.Disabled, &u.CreatedAt, &prefs, &u.Avatar, &u.MustChangePassword, &changed); err != nil {
		return nil, err
	}
	u.CreatedAt = u.CreatedAt.UTC()
	u.PasswordChangedAt = timePtr(changed)
	u.Erased = strings.HasSuffix(u.Email, "@"+model.ErasedEmailDomain)
	_ = json.Unmarshal([]byte(prefs), &u.Preferences) // unknown/old keys are ignored
	return &u, nil
}

func encodePrefs(p model.Preferences) string {
	b, _ := json.Marshal(p)
	return string(b)
}

// NormalizeEmail lowercases and trims an e-mail address.
func NormalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// CreateUser inserts a user. PasswordHash must already be hashed.
func (s *Store) CreateUser(ctx context.Context, u *model.User) error {
	u.Email = NormalizeEmail(u.Email)
	if !strings.Contains(u.Email, "@") || len(u.Email) > 254 {
		return Invalidf("a valid e-mail address is required")
	}
	if u.ID == "" {
		u.ID = newID()
	}
	if u.AuthProvider == "" {
		u.AuthProvider = "local"
	}
	if u.DisplayName == "" {
		u.DisplayName = strings.Split(u.Email, "@")[0]
	}
	u.CreatedAt = s.now()
	u.PasswordChangedAt = nil
	if u.PasswordHash != "" {
		u.PasswordChangedAt = &u.CreatedAt
	}
	_, err := s.c(ctx).exec(ctx, `INSERT INTO users (`+userCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Email, u.DisplayName, u.PasswordHash, u.SuperAdmin, u.AuthProvider, u.ExternalID, u.Disabled, u.CreatedAt, encodePrefs(u.Preferences), u.Avatar, u.MustChangePassword, nullTime(u.PasswordChangedAt))
	return mapErr(err, "user")
}

// GetUser returns a user by id.
func (s *Store) GetUser(ctx context.Context, id string) (*model.User, error) {
	u, err := scanUser(s.c(ctx).row(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
	return u, mapErr(err, "user")
}

// GetUserByEmail returns a user by e-mail.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	u, err := scanUser(s.c(ctx).row(ctx, `SELECT `+userCols+` FROM users WHERE email = ?`, NormalizeEmail(email)))
	return u, mapErr(err, "user")
}

// GetUserByExternal returns a user by provider and external subject.
func (s *Store) GetUserByExternal(ctx context.Context, provider, externalID string) (*model.User, error) {
	u, err := scanUser(s.c(ctx).row(ctx, `SELECT `+userCols+` FROM users WHERE auth_provider = ? AND external_id = ?`, provider, externalID))
	return u, mapErr(err, "user")
}

// ListUsers returns all users.
func (s *Store) ListUsers(ctx context.Context) ([]model.User, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+userCols+` FROM users ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// UpdateUser stores mutable user fields. A changed password hash records
// the time of the change and keeps the earlier hash in the password
// history (see PasswordHistory).
func (s *Store) UpdateUser(ctx context.Context, u *model.User) error {
	return s.tx(ctx, func(c conn) error {
		var old, hist string
		var changed sql.NullTime
		if err := c.row(ctx, `SELECT password_hash, password_history, password_changed_at FROM users WHERE id = ?`, u.ID).Scan(&old, &hist, &changed); err != nil {
			return mapErr(err, "user")
		}
		u.PasswordChangedAt = timePtr(changed)
		if u.PasswordHash != old {
			now := s.now()
			u.PasswordChangedAt = &now
			if u.PasswordHash == "" {
				u.PasswordChangedAt = nil
			}
			if old != "" {
				var list []string
				_ = json.Unmarshal([]byte(hist), &list)
				list = append([]string{old}, list...)
				if len(list) > MaxPasswordHistory {
					list = list[:MaxPasswordHistory]
				}
				b, _ := json.Marshal(list)
				hist = string(b)
			}
		}
		_, err := c.exec(ctx, `UPDATE users SET display_name = ?, password_hash = ?, super_admin = ?, disabled = ?, auth_provider = ?, external_id = ?, preferences = ?, avatar = ?, must_change_password = ?, password_changed_at = ?, password_history = ? WHERE id = ?`,
			u.DisplayName, u.PasswordHash, u.SuperAdmin, u.Disabled, u.AuthProvider, u.ExternalID, encodePrefs(u.Preferences), u.Avatar, u.MustChangePassword, nullTime(u.PasswordChangedAt), hist, u.ID)
		return err
	})
}

// StartPasswordAge records now as the time of the password change of a
// user whose password has none recorded (accounts older than migration
// 0063), so auth.password_max_age_days counts from now on.
func (s *Store) StartPasswordAge(ctx context.Context, userID string) error {
	_, err := s.c(ctx).exec(ctx, `UPDATE users SET password_changed_at = ? WHERE id = ? AND password_changed_at IS NULL`, s.now(), userID)
	return err
}

// MaxPasswordHistory is the most earlier password hashes kept per user.
const MaxPasswordHistory = 24

// PasswordHistory returns the hashes of the earlier passwords of a user,
// newest first.
func (s *Store) PasswordHistory(ctx context.Context, userID string) ([]string, error) {
	var hist string
	if err := s.c(ctx).row(ctx, `SELECT password_history FROM users WHERE id = ?`, userID).Scan(&hist); err != nil {
		return nil, mapErr(err, "user")
	}
	var list []string
	_ = json.Unmarshal([]byte(hist), &list)
	return list, nil
}

// CountActiveSuperAdmins returns the number of enabled super administrators.
func (s *Store) CountActiveSuperAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.c(ctx).row(ctx, `SELECT COUNT(*) FROM users WHERE super_admin = ? AND disabled = ?`, true, false).Scan(&n)
	return n, err
}

// CountUsers returns the number of users.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.c(ctx).row(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// ---------------------------------------------------------------- memberships

// SetMembership creates or updates a membership with one role.
func (s *Store) SetMembership(ctx context.Context, tenantID, userID, role string) error {
	return s.SetMembershipRoles(ctx, tenantID, userID, []string{role})
}

// SetMembershipRoles creates or updates a membership with a set of roles
// (at least one).
func (s *Store) SetMembershipRoles(ctx context.Context, tenantID, userID string, roles []string) error {
	rs, raw, err := normRoles(roles)
	if err != nil {
		return err
	}
	role := rbac.Primary(rs)
	return s.tx(ctx, func(c conn) error {
		res, err := c.exec(ctx, `UPDATE memberships SET role = ?, roles = ? WHERE tenant_id = ? AND user_id = ?`, role, raw, tenantID, userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			return nil
		}
		if err := s.checkQuota(ctx, c, tenantID, quotaMembers, 0); err != nil {
			return err
		}
		_, err = c.exec(ctx, `INSERT INTO memberships (tenant_id, user_id, role, roles, created_at) VALUES (?, ?, ?, ?, ?)`, tenantID, userID, role, raw, s.now())
		return mapErr(err, "membership")
	})
}

// GetMembership returns the roles of a user in a tenant.
func (s *Store) GetMembership(ctx context.Context, tenantID, userID string) (*model.Membership, error) {
	var m model.Membership
	var roles string
	err := s.c(ctx).row(ctx, `SELECT m.tenant_id, t.slug, t.name, m.user_id, m.role, m.roles, m.created_at FROM memberships m JOIN tenants t ON t.id = m.tenant_id WHERE m.tenant_id = ? AND m.user_id = ?`, tenantID, userID).
		Scan(&m.TenantID, &m.TenantSlug, &m.TenantName, &m.UserID, &m.Role, &roles, &m.CreatedAt)
	if err != nil {
		return nil, mapErr(err, "membership")
	}
	m.Roles = decodeRoles(roles, m.Role)
	return &m, nil
}

// RemoveMembership deletes a membership.
func (s *Store) RemoveMembership(ctx context.Context, tenantID, userID string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM memberships WHERE tenant_id = ? AND user_id = ?`, tenantID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "membership")
	}
	_, err = s.c(ctx).exec(ctx, `DELETE FROM sessions WHERE tenant_id = ? AND user_id = ?`, tenantID, userID)
	if err != nil {
		return err
	}
	// the user's project memberships in the tenant go with it
	_, err = s.c(ctx).exec(ctx, `DELETE FROM project_members WHERE tenant_id = ? AND member_type = 'user' AND member_id = ?`, tenantID, userID)
	if err != nil {
		return err
	}
	// and so does the user's autocomplete history there
	_, err = s.ClearSuggestHistory(ctx, tenantID, userID)
	return err
}

// ListUserMemberships returns all tenants a user belongs to.
func (s *Store) ListUserMemberships(ctx context.Context, userID string) ([]model.Membership, error) {
	return s.listMemberships(ctx, `WHERE m.user_id = ? ORDER BY t.name`, userID)
}

// ListTenantMembers returns all members of a tenant.
func (s *Store) ListTenantMembers(ctx context.Context, tenantID string) ([]model.Membership, error) {
	return s.listMemberships(ctx, `WHERE m.tenant_id = ? ORDER BY u.email`, tenantID)
}

// CountTenantAdmins returns the number of members of a tenant who hold the
// admin role (among other roles or alone).
func (s *Store) CountTenantAdmins(ctx context.Context, tenantID string) (int, error) {
	var n int
	err := s.c(ctx).row(ctx, `SELECT COUNT(*) FROM memberships WHERE tenant_id = ? AND `+hasRoleSQL(""), tenantID, rbac.RoleAdmin, likeRole(rbac.RoleAdmin)).Scan(&n)
	return n, err
}

func (s *Store) listMemberships(ctx context.Context, where string, arg any) ([]model.Membership, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT m.tenant_id, t.slug, t.name, m.user_id, u.email, u.display_name, u.avatar, m.role, m.roles, m.created_at,
		CASE WHEN su.user_id IS NULL THEN 0 ELSE 1 END, CASE WHEN mf.enabled_at IS NULL THEN 0 ELSE 1 END
		FROM memberships m JOIN tenants t ON t.id = m.tenant_id JOIN users u ON u.id = m.user_id
		LEFT JOIN scim_users su ON su.tenant_id = m.tenant_id AND su.user_id = m.user_id
		LEFT JOIN user_mfa mf ON mf.user_id = m.user_id `+where, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Membership{}
	for rows.Next() {
		var m model.Membership
		var scim, mfa int
		var roles string
		if err := rows.Scan(&m.TenantID, &m.TenantSlug, &m.TenantName, &m.UserID, &m.Email, &m.Name, &m.Avatar, &m.Role, &roles, &m.CreatedAt, &scim, &mfa); err != nil {
			return nil, err
		}
		m.Roles = decodeRoles(roles, m.Role)
		m.SCIM, m.MFAEnabled = scim == 1, mfa == 1
		out = append(out, m)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- sessions

// CreateSession stores a session.
func (s *Store) CreateSession(ctx context.Context, sess *model.Session) error {
	sess.CreatedAt = s.now()
	sess.LastSeen = &sess.CreatedAt
	_, err := s.c(ctx).exec(ctx, `INSERT INTO sessions (id, user_id, tenant_id, csrf, created_at, expires_at, ip, user_agent, last_seen, mfa_enrol_required) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.UserID, sess.TenantID, sess.CSRF, sess.CreatedAt, sess.ExpiresAt.UTC(), sess.IP, truncate(sess.UserAgent, 512), sess.CreatedAt, sess.MFAEnrolRequired)
	return mapErr(err, "session")
}

// GetSession returns a non-expired session.
func (s *Store) GetSession(ctx context.Context, id string) (*model.Session, error) {
	var sess model.Session
	var seen sql.NullTime
	err := s.c(ctx).row(ctx, `SELECT id, user_id, tenant_id, csrf, created_at, expires_at, ip, user_agent, last_seen, mfa_enrol_required FROM sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.UserID, &sess.TenantID, &sess.CSRF, &sess.CreatedAt, &sess.ExpiresAt, &sess.IP, &sess.UserAgent, &seen, &sess.MFAEnrolRequired)
	sess.LastSeen = timePtr(seen)
	if err != nil {
		return nil, mapErr(err, "session")
	}
	if !sess.ExpiresAt.After(s.now()) {
		_ = s.DeleteSession(ctx, id)
		return nil, mapErr(sql.ErrNoRows, "session")
	}
	return &sess, nil
}

// TouchSession records activity, at most once per minute per session.
func (s *Store) TouchSession(ctx context.Context, sess *model.Session, ip string) error {
	return s.TouchSessionEvery(ctx, sess, ip, time.Minute)
}

// TouchSessionEvery records activity at most once per interval (shorter for
// an inactivity timeout).
func (s *Store) TouchSessionEvery(ctx context.Context, sess *model.Session, ip string, every time.Duration) error {
	now := s.now()
	if sess.LastSeen != nil && now.Sub(*sess.LastSeen) < every && sess.IP == ip {
		return nil
	}
	sess.LastSeen, sess.IP = &now, ip
	_, err := s.c(ctx).exec(ctx, `UPDATE sessions SET last_seen = ?, ip = ? WHERE id = ?`, now, ip, sess.ID)
	return err
}

// ListUserSessions returns the active sessions of a user, newest first.
// IDs are the stored session hashes, which cannot be used to log in.
func (s *Store) ListUserSessions(ctx context.Context, userID string) ([]model.SessionInfo, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id, tenant_id, created_at, expires_at, ip, user_agent, last_seen FROM sessions WHERE user_id = ? AND expires_at > ? ORDER BY created_at DESC`, userID, s.now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.SessionInfo{}
	for rows.Next() {
		var si model.SessionInfo
		var seen sql.NullTime
		if err := rows.Scan(&si.ID, &si.TenantID, &si.CreatedAt, &si.ExpiresAt, &si.IP, &si.UserAgent, &seen); err != nil {
			return nil, err
		}
		si.LastSeen = timePtr(seen)
		out = append(out, si)
	}
	return out, rows.Err()
}

// DeleteUserSession revokes one session of a user.
func (s *Store) DeleteUserSession(ctx context.Context, userID, id string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM sessions WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "session")
	}
	return nil
}

// SetSessionTenant changes the current tenant of a session.
func (s *Store) SetSessionTenant(ctx context.Context, id, tenantID string) error {
	_, err := s.c(ctx).exec(ctx, `UPDATE sessions SET tenant_id = ? WHERE id = ?`, tenantID, id)
	return err
}

// DeleteSession removes a session.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.c(ctx).exec(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// DeleteUserSessions removes all sessions of a user.
func (s *Store) DeleteUserSessions(ctx context.Context, userID string) error {
	_, err := s.c(ctx).exec(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

// DeleteOtherSessions removes all sessions of a user except keep and
// returns how many were removed.
func (s *Store) DeleteOtherSessions(ctx context.Context, userID, keep string) (int, error) {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM sessions WHERE user_id = ? AND id <> ?`, userID, keep)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// PurgeExpiredSessions deletes expired sessions (and expired pending MFA
// sign-ins and remembered devices) and returns the count.
func (s *Store) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, s.now())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	m, err := s.PurgeExpiredMFA(ctx)
	return n + m, err
}

// ---------------------------------------------------------------- tokens

// CreateToken stores an API token (hash computed by caller).
func (s *Store) CreateToken(ctx context.Context, t *model.APIToken) error {
	if len(t.Roles) == 0 && t.Role != "" {
		t.Roles = []string{t.Role}
	}
	rs, raw, err := normRoles(t.Roles)
	if err != nil {
		return err
	}
	t.Roles, t.Role = rs, rbac.Primary(rs)
	t.ID = newID()
	t.CreatedAt = s.now()
	_, err = s.c(ctx).exec(ctx, `INSERT INTO api_tokens (id, tenant_id, user_id, name, role, roles, hash, prefix, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.TenantID, t.UserID, t.Name, t.Role, raw, t.Hash, t.Prefix, t.CreatedAt, nullTime(t.ExpiresAt))
	return mapErr(err, "token")
}

const tokenCols = `id, tenant_id, user_id, name, role, roles, hash, prefix, created_at, expires_at, last_used`

func scanToken(r interface{ Scan(...any) error }) (*model.APIToken, error) {
	var t model.APIToken
	var exp, used sql.NullTime
	var roles string
	if err := r.Scan(&t.ID, &t.TenantID, &t.UserID, &t.Name, &t.Role, &roles, &t.Hash, &t.Prefix, &t.CreatedAt, &exp, &used); err != nil {
		return nil, err
	}
	t.Roles = decodeRoles(roles, t.Role)
	t.ExpiresAt, t.LastUsed = timePtr(exp), timePtr(used)
	return &t, nil
}

// GetTokenByHash returns a valid (non-expired) token.
func (s *Store) GetTokenByHash(ctx context.Context, hash string) (*model.APIToken, error) {
	t, err := scanToken(s.c(ctx).row(ctx, `SELECT `+tokenCols+` FROM api_tokens WHERE hash = ?`, hash))
	if err != nil {
		return nil, mapErr(err, "token")
	}
	if t.ExpiresAt != nil && !t.ExpiresAt.After(s.now()) {
		return nil, mapErr(sql.ErrNoRows, "token")
	}
	_, _ = s.c(ctx).exec(ctx, `UPDATE api_tokens SET last_used = ? WHERE id = ?`, s.now(), t.ID)
	return t, nil
}

// ListTokens lists a user's tokens in a tenant.
func (s *Store) ListTokens(ctx context.Context, tenantID, userID string) ([]model.APIToken, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+tokenCols+` FROM api_tokens WHERE tenant_id = ? AND user_id = ? ORDER BY created_at`, tenantID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.APIToken{}
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// DeleteToken revokes a token owned by user in tenant.
func (s *Store) DeleteToken(ctx context.Context, tenantID, userID, id string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM api_tokens WHERE id = ? AND tenant_id = ? AND user_id = ?`, id, tenantID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "token")
	}
	return nil
}

// ---------------------------------------------------------------- audit

// Audit appends an audit log entry; failures are returned but callers may
// choose to only log them. The entry records the reason for change of ctx
// (WithChangeReason) and is chained once the transaction of ctx commits
// (auditchain.go).
func (s *Store) Audit(ctx context.Context, e model.AuditEntry) error {
	e.ID = newID()
	e.CreatedAt = s.now().UTC().Truncate(time.Microsecond)
	if e.Reason == "" {
		e.Reason = ChangeReason(ctx)
	}
	_, err := s.c(ctx).exec(ctx, `INSERT INTO audit_log (id, tenant_id, user_id, user_email, action, entity_type, entity_id, details, reason, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.TenantID, e.UserID, e.UserEmail, e.Action, e.EntityType, e.EntityID, e.Details, e.Reason, e.CreatedAt)
	if err != nil {
		return err
	}
	AfterCommit(ctx, func() { _ = s.SealAudit(ctx, e.TenantID) })
	return nil
}

const auditListCols = `a.id, a.tenant_id, a.user_id, a.user_email, COALESCE(u.display_name, ''), COALESCE(u.avatar, ''), a.action, a.entity_type, a.entity_id, a.details, a.created_at, a.reason, COALESCE(a.seq, 0), a.prev_hash, a.hash`

func scanAuditList(rows *sql.Rows) ([]model.AuditEntry, error) {
	defer rows.Close()
	out := []model.AuditEntry{}
	for rows.Next() {
		var e model.AuditEntry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.UserID, &e.UserEmail, &e.UserName, &e.UserAvatar, &e.Action, &e.EntityType, &e.EntityID, &e.Details, &e.CreatedAt,
			&e.Reason, &e.Seq, &e.PrevHash, &e.Hash); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListAuditRange returns the newest audit entries of a tenant within an
// optional time range.
func (s *Store) ListAuditRange(ctx context.Context, tenantID string, from, to *time.Time, limit int) ([]model.AuditEntry, error) {
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	q := `SELECT ` + auditListCols + `
		FROM audit_log a LEFT JOIN users u ON u.id = a.user_id WHERE a.tenant_id = ?`
	args := []any{tenantID}
	if from != nil {
		q += ` AND a.created_at >= ?`
		args = append(args, from.UTC())
	}
	if to != nil {
		q += ` AND a.created_at <= ?`
		args = append(args, to.UTC())
	}
	rows, err := s.c(ctx).query(ctx, q+` ORDER BY a.created_at DESC, a.seq DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	return scanAuditList(rows)
}

// ListAudit returns the newest audit entries of a tenant.
func (s *Store) ListAudit(ctx context.Context, tenantID string, limit int) ([]model.AuditEntry, error) {
	return s.ListAuditRange(ctx, tenantID, nil, nil, limit)
}

// expiry helper used by token creation.
func (s *Store) ExpiryIn(days int) *time.Time {
	if days <= 0 {
		return nil
	}
	t := s.now().Add(time.Duration(days) * 24 * time.Hour)
	return &t
}
