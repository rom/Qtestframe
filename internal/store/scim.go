package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

// SCIM provisioning (see internal/api/scim.go): tokens, settings, the
// users and groups an identity provider pushed into a tenant.

// ---------------------------------------------------------------- tokens

// CreateSCIMToken stores a SCIM token (hash computed by the caller).
func (s *Store) CreateSCIMToken(ctx context.Context, t *model.SCIMToken) error {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" || len(t.Name) > 100 {
		return Invalidf("token name is required (at most 100 characters)")
	}
	t.ID = newID()
	t.CreatedAt = s.now()
	_, err := s.c(ctx).exec(ctx, `INSERT INTO scim_tokens (id, tenant_id, name, hash, prefix, created_by, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.TenantID, t.Name, t.Hash, t.Prefix, t.CreatedBy, t.CreatedAt, nullTime(t.ExpiresAt))
	return mapErr(err, "token")
}

const scimTokenCols = `id, tenant_id, name, hash, prefix, created_by, created_at, expires_at, last_used`

func scanSCIMToken(r interface{ Scan(...any) error }) (*model.SCIMToken, error) {
	var t model.SCIMToken
	var exp, used sql.NullTime
	if err := r.Scan(&t.ID, &t.TenantID, &t.Name, &t.Hash, &t.Prefix, &t.CreatedBy, &t.CreatedAt, &exp, &used); err != nil {
		return nil, err
	}
	t.CreatedAt = t.CreatedAt.UTC()
	t.ExpiresAt, t.LastUsed = timePtr(exp), timePtr(used)
	return &t, nil
}

// GetSCIMTokenByHash returns a valid (non-expired) SCIM token and records
// its use.
func (s *Store) GetSCIMTokenByHash(ctx context.Context, hash string) (*model.SCIMToken, error) {
	t, err := scanSCIMToken(s.c(ctx).row(ctx, `SELECT `+scimTokenCols+` FROM scim_tokens WHERE hash = ?`, hash))
	if err != nil {
		return nil, mapErr(err, "token")
	}
	if t.ExpiresAt != nil && !t.ExpiresAt.After(s.now()) {
		return nil, mapErr(sql.ErrNoRows, "token")
	}
	now := s.now()
	if _, err := s.c(ctx).exec(ctx, `UPDATE scim_tokens SET last_used = ? WHERE id = ?`, now, t.ID); err == nil {
		t.LastUsed = &now
	}
	return t, nil
}

// ListSCIMTokens lists the SCIM tokens of a tenant.
func (s *Store) ListSCIMTokens(ctx context.Context, tenantID string) ([]model.SCIMToken, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+scimTokenCols+` FROM scim_tokens WHERE tenant_id = ? ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.SCIMToken{}
	for rows.Next() {
		t, err := scanSCIMToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// DeleteSCIMToken revokes a SCIM token of a tenant and returns it.
func (s *Store) DeleteSCIMToken(ctx context.Context, tenantID, id string) (*model.SCIMToken, error) {
	t, err := scanSCIMToken(s.c(ctx).row(ctx, `SELECT `+scimTokenCols+` FROM scim_tokens WHERE id = ? AND tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "token")
	}
	if _, err := s.c(ctx).exec(ctx, `DELETE FROM scim_tokens WHERE id = ? AND tenant_id = ?`, id, tenantID); err != nil {
		return nil, err
	}
	return t, nil
}

// ---------------------------------------------------------------- settings

// SCIMSettings returns the provisioning settings of a tenant (default
// role viewer until changed).
func (s *Store) SCIMSettings(ctx context.Context, tenantID string) (*model.SCIMSettings, error) {
	st := &model.SCIMSettings{DefaultRole: "viewer"}
	err := s.c(ctx).row(ctx, `SELECT default_role FROM scim_settings WHERE tenant_id = ?`, tenantID).Scan(&st.DefaultRole)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return st, nil
}

// SaveSCIMSettings stores the provisioning settings of a tenant (the
// caller validates the role).
func (s *Store) SaveSCIMSettings(ctx context.Context, tenantID string, st *model.SCIMSettings) error {
	return s.tx(ctx, func(c conn) error {
		res, err := c.exec(ctx, `UPDATE scim_settings SET default_role = ?, updated_at = ? WHERE tenant_id = ?`, st.DefaultRole, s.now(), tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			return nil
		}
		_, err = c.exec(ctx, `INSERT INTO scim_settings (tenant_id, default_role, updated_at) VALUES (?, ?, ?)`, tenantID, st.DefaultRole, s.now())
		return mapErr(err, "settings")
	})
}

// ---------------------------------------------------------------- users

const scimUserCols = `su.tenant_id, su.user_id, su.user_name, u.email, u.display_name, su.external_id, su.given_name, su.family_name, su.active, su.created_at, su.updated_at`

func scanSCIMUser(r interface{ Scan(...any) error }) (*model.SCIMUser, error) {
	var u model.SCIMUser
	if err := r.Scan(&u.TenantID, &u.UserID, &u.UserName, &u.Email, &u.DisplayName, &u.ExternalID, &u.GivenName, &u.FamilyName, &u.Active, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	u.CreatedAt, u.UpdatedAt = u.CreatedAt.UTC(), u.UpdatedAt.UTC()
	return &u, nil
}

// GetSCIMUser returns a user provisioned into a tenant.
func (s *Store) GetSCIMUser(ctx context.Context, tenantID, userID string) (*model.SCIMUser, error) {
	u, err := scanSCIMUser(s.c(ctx).row(ctx, `SELECT `+scimUserCols+` FROM scim_users su JOIN users u ON u.id = su.user_id WHERE su.tenant_id = ? AND su.user_id = ?`, tenantID, userID))
	return u, mapErr(err, "user")
}

// ListSCIMUsers returns the users provisioned into a tenant, by e-mail.
func (s *Store) ListSCIMUsers(ctx context.Context, tenantID string) ([]model.SCIMUser, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+scimUserCols+` FROM scim_users su JOIN users u ON u.id = su.user_id WHERE su.tenant_id = ? ORDER BY u.email`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.SCIMUser{}
	for rows.Next() {
		u, err := scanSCIMUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// SaveSCIMUser creates or updates the provisioning record of a user in a
// tenant (the SCIM fields only; the user row is changed with UpdateUser).
func (s *Store) SaveSCIMUser(ctx context.Context, u *model.SCIMUser) error {
	now := s.now()
	u.UpdatedAt = now
	return s.tx(ctx, func(c conn) error {
		res, err := c.exec(ctx, `UPDATE scim_users SET user_name = ?, external_id = ?, given_name = ?, family_name = ?, active = ?, updated_at = ? WHERE tenant_id = ? AND user_id = ?`,
			u.UserName, u.ExternalID, u.GivenName, u.FamilyName, u.Active, now, u.TenantID, u.UserID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			return nil
		}
		u.CreatedAt = now
		_, err = c.exec(ctx, `INSERT INTO scim_users (tenant_id, user_id, user_name, external_id, given_name, family_name, active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			u.TenantID, u.UserID, u.UserName, u.ExternalID, u.GivenName, u.FamilyName, u.Active, now, now)
		return mapErr(err, "user")
	})
}

// TouchSCIMUser moves the modification time of a provisioned user (its
// SCIM version) forward, e.g. after a group change.
func (s *Store) TouchSCIMUser(ctx context.Context, tenantID, userID string) error {
	_, err := s.c(ctx).exec(ctx, `UPDATE scim_users SET updated_at = ? WHERE tenant_id = ? AND user_id = ?`, s.now(), tenantID, userID)
	return err
}

// DeleteSCIMUser removes the provisioning record and group memberships of
// a user in a tenant. The membership is left to the caller.
func (s *Store) DeleteSCIMUser(ctx context.Context, tenantID, userID string) error {
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `DELETE FROM scim_group_members WHERE tenant_id = ? AND user_id = ?`, tenantID, userID); err != nil {
			return err
		}
		res, err := c.exec(ctx, `DELETE FROM scim_users WHERE tenant_id = ? AND user_id = ?`, tenantID, userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "user")
		}
		return nil
	})
}

// IsSCIMUser reports whether the membership of a user in a tenant is
// managed by SCIM provisioning.
func (s *Store) IsSCIMUser(ctx context.Context, tenantID, userID string) (bool, error) {
	var n int
	err := s.c(ctx).row(ctx, `SELECT COUNT(*) FROM scim_users WHERE tenant_id = ? AND user_id = ?`, tenantID, userID).Scan(&n)
	return n > 0, err
}

// RemoveTenantAccess ends a user's access to a tenant: the membership (if
// any), the sessions in the tenant and the user's API tokens there.
func (s *Store) RemoveTenantAccess(ctx context.Context, tenantID, userID string) error {
	return s.tx(ctx, func(c conn) error {
		for _, q := range []string{
			`DELETE FROM memberships WHERE tenant_id = ? AND user_id = ?`,
			`DELETE FROM sessions WHERE tenant_id = ? AND user_id = ?`,
			`DELETE FROM api_tokens WHERE tenant_id = ? AND user_id = ?`,
		} {
			if _, err := c.exec(ctx, q, tenantID, userID); err != nil {
				return err
			}
		}
		return nil
	})
}

// ---------------------------------------------------------------- groups

const scimGroupCols = `id, tenant_id, display_name, external_id, role, created_at, updated_at`

func scanSCIMGroup(r interface{ Scan(...any) error }) (*model.SCIMGroup, error) {
	var g model.SCIMGroup
	if err := r.Scan(&g.ID, &g.TenantID, &g.DisplayName, &g.ExternalID, &g.Role, &g.CreatedAt, &g.UpdatedAt); err != nil {
		return nil, err
	}
	g.CreatedAt, g.UpdatedAt = g.CreatedAt.UTC(), g.UpdatedAt.UTC()
	g.Members = []string{}
	return &g, nil
}

func validGroupName(name string) error {
	if name == "" || len(name) > 256 {
		return Invalidf("displayName is required (at most 256 characters)")
	}
	return nil
}

// CreateSCIMGroup inserts a group with its members.
func (s *Store) CreateSCIMGroup(ctx context.Context, g *model.SCIMGroup) error {
	g.DisplayName = strings.TrimSpace(g.DisplayName)
	if err := validGroupName(g.DisplayName); err != nil {
		return err
	}
	g.ID = newID()
	g.CreatedAt = s.now()
	g.UpdatedAt = g.CreatedAt
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `INSERT INTO scim_groups (`+scimGroupCols+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			g.ID, g.TenantID, g.DisplayName, g.ExternalID, g.Role, g.CreatedAt, g.UpdatedAt); err != nil {
			return mapErr(err, "group")
		}
		return s.setGroupMembers(ctx, c, g, g.Members)
	})
}

func (s *Store) setGroupMembers(ctx context.Context, c conn, g *model.SCIMGroup, ids []string) error {
	if _, err := c.exec(ctx, `DELETE FROM scim_group_members WHERE group_id = ?`, g.ID); err != nil {
		return err
	}
	seen := map[string]bool{}
	clean := []string{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := c.exec(ctx, `INSERT INTO scim_group_members (group_id, tenant_id, user_id, created_at) VALUES (?, ?, ?, ?)`, g.ID, g.TenantID, id, s.now()); err != nil {
			return mapErr(err, "group member")
		}
		clean = append(clean, id)
	}
	g.Members = clean
	return nil
}

// GetSCIMGroup returns a group of a tenant with its member ids.
func (s *Store) GetSCIMGroup(ctx context.Context, tenantID, id string) (*model.SCIMGroup, error) {
	g, err := scanSCIMGroup(s.c(ctx).row(ctx, `SELECT `+scimGroupCols+` FROM scim_groups WHERE id = ? AND tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "group")
	}
	rows, err := s.c(ctx).query(ctx, `SELECT user_id FROM scim_group_members WHERE group_id = ? ORDER BY created_at, user_id`, id)
	if err != nil {
		return nil, err
	}
	ids, err := scanStrings(rows)
	if err != nil {
		return nil, err
	}
	if ids != nil {
		g.Members = ids
	}
	return g, nil
}

// ListSCIMGroups returns the groups of a tenant with their member ids.
func (s *Store) ListSCIMGroups(ctx context.Context, tenantID string) ([]model.SCIMGroup, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+scimGroupCols+` FROM scim_groups WHERE tenant_id = ? ORDER BY display_name`, tenantID)
	if err != nil {
		return nil, err
	}
	out := []model.SCIMGroup{}
	idx := map[string]int{}
	for rows.Next() {
		g, err := scanSCIMGroup(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		idx[g.ID] = len(out)
		out = append(out, *g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	mrows, err := s.c(ctx).query(ctx, `SELECT group_id, user_id FROM scim_group_members WHERE tenant_id = ? ORDER BY created_at, user_id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var gid, uid string
		if err := mrows.Scan(&gid, &uid); err != nil {
			return nil, err
		}
		if i, ok := idx[gid]; ok {
			out[i].Members = append(out[i].Members, uid)
		}
	}
	return out, mrows.Err()
}

// UpdateSCIMGroup stores the name, external id and role of a group, and
// its members unless members is nil.
func (s *Store) UpdateSCIMGroup(ctx context.Context, g *model.SCIMGroup, members []string) error {
	g.DisplayName = strings.TrimSpace(g.DisplayName)
	if err := validGroupName(g.DisplayName); err != nil {
		return err
	}
	g.UpdatedAt = s.now()
	return s.tx(ctx, func(c conn) error {
		res, err := c.exec(ctx, `UPDATE scim_groups SET display_name = ?, external_id = ?, role = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
			g.DisplayName, g.ExternalID, g.Role, g.UpdatedAt, g.ID, g.TenantID)
		if err != nil {
			return mapErr(err, "group")
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "group")
		}
		if members == nil {
			return nil
		}
		return s.setGroupMembers(ctx, c, g, members)
	})
}

// DeleteSCIMGroup deletes a group of a tenant (its memberships cascade).
func (s *Store) DeleteSCIMGroup(ctx context.Context, tenantID, id string) error {
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `DELETE FROM scim_group_members WHERE group_id = ? AND tenant_id = ?`, id, tenantID); err != nil {
			return err
		}
		res, err := c.exec(ctx, `DELETE FROM scim_groups WHERE id = ? AND tenant_id = ?`, id, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "group")
		}
		return nil
	})
}

// SCIMUserGroups returns the groups of a tenant a user belongs to (without
// member lists), by name.
func (s *Store) SCIMUserGroups(ctx context.Context, tenantID, userID string) ([]model.SCIMGroup, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT g.id, g.tenant_id, g.display_name, g.external_id, g.role, g.created_at, g.updated_at
		FROM scim_groups g JOIN scim_group_members m ON m.group_id = g.id
		WHERE m.tenant_id = ? AND m.user_id = ? ORDER BY g.display_name`, tenantID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.SCIMGroup{}
	for rows.Next() {
		g, err := scanSCIMGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

// SetUserEmail changes the e-mail address (sign-in name) of a user.
func (s *Store) SetUserEmail(ctx context.Context, userID, email string) error {
	email = NormalizeEmail(email)
	if !strings.Contains(email, "@") || len(email) > 254 {
		return Invalidf("a valid e-mail address is required")
	}
	res, err := s.c(ctx).exec(ctx, `UPDATE users SET email = ? WHERE id = ?`, email, userID)
	if err != nil {
		return mapErr(err, "user with this e-mail")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "user")
	}
	return nil
}

// OtherTenants counts the tenants other than tenantID a user belongs to or
// is provisioned into.
func (s *Store) OtherTenants(ctx context.Context, tenantID, userID string) (int, error) {
	var n int
	err := s.c(ctx).row(ctx, `SELECT COUNT(*) FROM (SELECT tenant_id FROM memberships WHERE user_id = ? AND tenant_id <> ?
		UNION SELECT tenant_id FROM scim_users WHERE user_id = ? AND tenant_id <> ?) x`, userID, tenantID, userID, tenantID).Scan(&n)
	return n, err
}

// ListAuditByPrefix returns the newest audit entries of a tenant whose
// action starts with prefix.
func (s *Store) ListAuditByPrefix(ctx context.Context, tenantID, prefix string, limit int) ([]model.AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.c(ctx).query(ctx, `SELECT id, tenant_id, user_id, user_email, action, entity_type, entity_id, details, created_at
		FROM audit_log WHERE tenant_id = ? AND action LIKE ? ORDER BY created_at DESC LIMIT ?`, tenantID, prefix+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.AuditEntry{}
	for rows.Next() {
		var e model.AuditEntry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.UserID, &e.UserEmail, &e.Action, &e.EntityType, &e.EntityID, &e.Details, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
