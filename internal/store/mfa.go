package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// Multi-factor authentication: TOTP enrolments, recovery codes, pending
// second-factor sign-ins, remembered devices and MFA policies (see
// internal/api/mfa.go and docs/security.md).

// GetUserMFA returns the TOTP enrolment of a user (pending or confirmed)
// with the number of unused recovery codes.
func (s *Store) GetUserMFA(ctx context.Context, userID string) (*model.UserMFA, error) {
	var m model.UserMFA
	var enabled sql.NullTime
	err := s.c(ctx).row(ctx, `SELECT user_id, totp_secret, enabled_at, last_step, created_at FROM user_mfa WHERE user_id = ?`, userID).
		Scan(&m.UserID, &m.Secret, &enabled, &m.LastStep, &m.CreatedAt)
	if err != nil {
		return nil, mapErr(err, "mfa enrolment")
	}
	m.EnabledAt = timePtr(enabled)
	if err := s.c(ctx).row(ctx, `SELECT COUNT(*), COALESCE(SUM(CASE WHEN used_at IS NULL THEN 1 ELSE 0 END), 0) FROM mfa_recovery_codes WHERE user_id = ?`, userID).
		Scan(&m.CodesTotal, &m.CodesLeft); err != nil {
		return nil, err
	}
	return &m, nil
}

// StartTOTP stores a new, unconfirmed TOTP secret, replacing an earlier
// unconfirmed one. A confirmed enrolment must be removed first.
func (s *Store) StartTOTP(ctx context.Context, userID, secret string) error {
	return s.tx(ctx, func(c conn) error {
		var n int
		if err := c.row(ctx, `SELECT COUNT(*) FROM user_mfa WHERE user_id = ? AND enabled_at IS NOT NULL`, userID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return Conflictf("an authenticator app is already set up; turn it off first")
		}
		if _, err := c.exec(ctx, `DELETE FROM user_mfa WHERE user_id = ?`, userID); err != nil {
			return err
		}
		_, err := c.exec(ctx, `INSERT INTO user_mfa (user_id, totp_secret, last_step, created_at) VALUES (?, ?, 0, ?)`, userID, secret, s.now())
		return mapErr(err, "mfa enrolment")
	})
}

// ConfirmTOTP activates the pending enrolment (step is the step of the
// confirmation code, which cannot be used again), stores the recovery code
// hashes and lifts the enrolment requirement of the user's sessions.
func (s *Store) ConfirmTOTP(ctx context.Context, userID string, step int64, codeHashes []string) error {
	return s.tx(ctx, func(c conn) error {
		res, err := c.exec(ctx, `UPDATE user_mfa SET enabled_at = ?, last_step = ? WHERE user_id = ? AND enabled_at IS NULL`, s.now(), step, userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return Conflictf("no authenticator set-up is waiting for confirmation")
		}
		if err := replaceCodes(ctx, c, s.now(), userID, codeHashes); err != nil {
			return err
		}
		_, err = c.exec(ctx, `UPDATE sessions SET mfa_enrol_required = ? WHERE user_id = ?`, false, userID)
		return err
	})
}

func replaceCodes(ctx context.Context, c conn, now time.Time, userID string, hashes []string) error {
	if _, err := c.exec(ctx, `DELETE FROM mfa_recovery_codes WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for _, h := range hashes {
		if _, err := c.exec(ctx, `INSERT INTO mfa_recovery_codes (id, user_id, code_hash, created_at) VALUES (?, ?, ?, ?)`, newID(), userID, h, now); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceRecoveryCodes replaces all recovery codes of a user.
func (s *Store) ReplaceRecoveryCodes(ctx context.Context, userID string, hashes []string) error {
	return s.tx(ctx, func(c conn) error { return replaceCodes(ctx, c, s.now(), userID, hashes) })
}

// UseTOTPStep records step as the last accepted TOTP step of a confirmed
// enrolment. It reports false when a code of this or a later step was
// already accepted (replay).
func (s *Store) UseTOTPStep(ctx context.Context, userID string, step int64) (bool, error) {
	res, err := s.c(ctx).exec(ctx, `UPDATE user_mfa SET last_step = ? WHERE user_id = ? AND last_step < ? AND enabled_at IS NOT NULL`, step, userID, step)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// UseRecoveryCode marks an unused recovery code as used and reports
// whether there was one.
func (s *Store) UseRecoveryCode(ctx context.Context, userID, hash string) (bool, error) {
	res, err := s.c(ctx).exec(ctx, `UPDATE mfa_recovery_codes SET used_at = ? WHERE user_id = ? AND code_hash = ? AND used_at IS NULL`, s.now(), userID, hash)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeleteUserMFA removes a user's second factor, recovery codes, remembered
// devices and pending sign-ins, and reports whether a confirmed enrolment
// existed.
func (s *Store) DeleteUserMFA(ctx context.Context, userID string) (bool, error) {
	had := false
	err := s.tx(ctx, func(c conn) error {
		var n int
		if err := c.row(ctx, `SELECT COUNT(*) FROM user_mfa WHERE user_id = ? AND enabled_at IS NOT NULL`, userID).Scan(&n); err != nil {
			return err
		}
		had = n > 0
		for _, t := range []string{"user_mfa", "mfa_recovery_codes", "mfa_trusted_devices", "mfa_challenges"} {
			if _, err := c.exec(ctx, `DELETE FROM `+t+` WHERE user_id = ?`, userID); err != nil {
				return err
			}
		}
		return nil
	})
	return had, err
}

// MFAEnabledUsers returns the ids of all users with a confirmed second factor.
func (s *Store) MFAEnabledUsers(ctx context.Context) (map[string]bool, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT user_id FROM user_mfa WHERE enabled_at IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- challenges

// CreateMFAChallenge stores a pending second-factor sign-in. Older pending
// sign-ins of the user are dropped.
func (s *Store) CreateMFAChallenge(ctx context.Context, ch *model.MFAChallenge) error {
	ch.CreatedAt = s.now()
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `DELETE FROM mfa_challenges WHERE user_id = ? OR expires_at <= ?`, ch.UserID, ch.CreatedAt); err != nil {
			return err
		}
		_, err := c.exec(ctx, `INSERT INTO mfa_challenges (id, user_id, tenant_id, method, details, attempts, created_at, expires_at) VALUES (?, ?, ?, ?, ?, 0, ?, ?)`,
			ch.ID, ch.UserID, ch.TenantID, ch.Method, truncate(ch.Details, 500), ch.CreatedAt, ch.ExpiresAt.UTC())
		return mapErr(err, "mfa challenge")
	})
}

// GetMFAChallenge returns a pending, unexpired sign-in.
func (s *Store) GetMFAChallenge(ctx context.Context, id string) (*model.MFAChallenge, error) {
	var ch model.MFAChallenge
	err := s.c(ctx).row(ctx, `SELECT id, user_id, tenant_id, method, details, attempts, created_at, expires_at FROM mfa_challenges WHERE id = ?`, id).
		Scan(&ch.ID, &ch.UserID, &ch.TenantID, &ch.Method, &ch.Details, &ch.Attempts, &ch.CreatedAt, &ch.ExpiresAt)
	if err != nil {
		return nil, mapErr(err, "mfa challenge")
	}
	if !ch.ExpiresAt.After(s.now()) {
		_ = s.DeleteMFAChallenge(ctx, id)
		return nil, mapErr(sql.ErrNoRows, "mfa challenge")
	}
	return &ch, nil
}

// CountMFAAttempt adds an attempt to a pending sign-in and returns the
// attempts so far.
func (s *Store) CountMFAAttempt(ctx context.Context, id string) (int, error) {
	if _, err := s.c(ctx).exec(ctx, `UPDATE mfa_challenges SET attempts = attempts + 1 WHERE id = ?`, id); err != nil {
		return 0, err
	}
	var n int
	err := s.c(ctx).row(ctx, `SELECT attempts FROM mfa_challenges WHERE id = ?`, id).Scan(&n)
	return n, mapErr(err, "mfa challenge")
}

// DeleteMFAChallenge removes a pending sign-in.
func (s *Store) DeleteMFAChallenge(ctx context.Context, id string) error {
	_, err := s.c(ctx).exec(ctx, `DELETE FROM mfa_challenges WHERE id = ?`, id)
	return err
}

// ---------------------------------------------------------------- remembered devices

// AddTrustedDevice remembers a device (hash of its cookie token) until expires.
func (s *Store) AddTrustedDevice(ctx context.Context, hash, userID, ip, userAgent string, expires time.Time) error {
	_, err := s.c(ctx).exec(ctx, `INSERT INTO mfa_trusted_devices (id, user_id, ip, user_agent, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		hash, userID, ip, truncate(userAgent, 512), s.now(), expires.UTC())
	return mapErr(err, "trusted device")
}

// TrustedDevice reports whether hash is an unexpired remembered device of
// the user that was remembered less than maxAge ago (so lowering the
// policy's number of days also shortens devices remembered before).
func (s *Store) TrustedDevice(ctx context.Context, hash, userID string, maxAge time.Duration) (bool, error) {
	var n int
	now := s.now()
	err := s.c(ctx).row(ctx, `SELECT COUNT(*) FROM mfa_trusted_devices WHERE id = ? AND user_id = ? AND expires_at > ? AND created_at > ?`,
		hash, userID, now, now.Add(-maxAge)).Scan(&n)
	return n > 0, err
}

// CountTrustedDevices returns the unexpired remembered devices of a user.
func (s *Store) CountTrustedDevices(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.c(ctx).row(ctx, `SELECT COUNT(*) FROM mfa_trusted_devices WHERE user_id = ? AND expires_at > ?`, userID, s.now()).Scan(&n)
	return n, err
}

// DeleteTrustedDevices forgets all remembered devices of a user.
func (s *Store) DeleteTrustedDevices(ctx context.Context, userID string) (int, error) {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM mfa_trusted_devices WHERE user_id = ?`, userID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// PurgeExpiredMFA deletes expired pending sign-ins and remembered devices.
func (s *Store) PurgeExpiredMFA(ctx context.Context) (int64, error) {
	var total int64
	for _, t := range []string{"mfa_challenges", "mfa_trusted_devices"} {
		res, err := s.c(ctx).exec(ctx, `DELETE FROM `+t+` WHERE expires_at <= ?`, s.now())
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// ---------------------------------------------------------------- policies

const mfaPolicyCols = `scope, mode, roles, grace_days, remember_days, required_since, updated_by, updated_at`

func scanMFAPolicy(r interface{ Scan(...any) error }) (*model.MFAPolicy, error) {
	var p model.MFAPolicy
	var roles string
	var since, updated sql.NullTime
	if err := r.Scan(&p.Scope, &p.Mode, &roles, &p.GraceDays, &p.RememberDays, &since, &p.UpdatedBy, &updated); err != nil {
		return nil, err
	}
	p.Roles = decodeTags(roles)
	p.RequiredSince, p.UpdatedAt = timePtr(since), timePtr(updated)
	return &p, nil
}

// DefaultMFAPolicy is the policy of a scope that has none stored.
func DefaultMFAPolicy(scope string) *model.MFAPolicy {
	return &model.MFAPolicy{Scope: scope, Mode: model.MFAOptional, Roles: []string{}}
}

// MFAPolicy returns the policy of a scope ("" = server, else a tenant id).
func (s *Store) MFAPolicy(ctx context.Context, scope string) (*model.MFAPolicy, error) {
	p, err := scanMFAPolicy(s.c(ctx).row(ctx, `SELECT `+mfaPolicyCols+` FROM mfa_policies WHERE scope = ?`, scope))
	if err != nil {
		if err = mapErr(err, "mfa policy"); errors.Is(err, ErrNotFound) {
			return DefaultMFAPolicy(scope), nil
		}
		return nil, err
	}
	return p, nil
}

// MFAPolicies returns all stored policies that require MFA, by scope.
func (s *Store) MFAPolicies(ctx context.Context) (map[string]*model.MFAPolicy, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+mfaPolicyCols+` FROM mfa_policies`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*model.MFAPolicy{}
	for rows.Next() {
		p, err := scanMFAPolicy(rows)
		if err != nil {
			return nil, err
		}
		out[p.Scope] = p
	}
	return out, rows.Err()
}

// SaveMFAPolicy validates and stores a policy. RequiredSince is set when
// the policy starts to require MFA and cleared when it stops.
func (s *Store) SaveMFAPolicy(ctx context.Context, p *model.MFAPolicy) error {
	switch p.Mode {
	case model.MFAOptional, model.MFAAll, model.MFARoles:
	default:
		return Invalidf("mode must be optional, all or roles")
	}
	if p.GraceDays < 0 || p.GraceDays > 365 {
		return Invalidf("grace_days must be between 0 and 365")
	}
	if p.RememberDays < 0 || p.RememberDays > 365 {
		return Invalidf("remember_days must be between 0 and 365")
	}
	p.Roles = cleanTags(p.Roles)
	if p.Mode == model.MFARoles && len(p.Roles) == 0 {
		return Invalidf("choose at least one role")
	}
	if p.Mode != model.MFARoles {
		p.Roles = []string{}
	}
	old, err := s.MFAPolicy(ctx, p.Scope)
	if err != nil {
		return err
	}
	now := s.now()
	switch {
	case p.Mode == model.MFAOptional:
		p.RequiredSince = nil
	case old.Mode == model.MFAOptional || old.RequiredSince == nil ||
		(p.Mode == model.MFAAll && old.Mode != model.MFAAll) ||
		(p.Mode == model.MFARoles && old.Mode == model.MFARoles && !subsetOf(p.Roles, old.Roles)):
		// required for more people than before: their grace period starts now
		p.RequiredSince = &now
	default:
		p.RequiredSince = old.RequiredSince
	}
	p.UpdatedAt = &now
	roles, _ := json.Marshal(p.Roles)
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `DELETE FROM mfa_policies WHERE scope = ?`, p.Scope); err != nil {
			return err
		}
		_, err := c.exec(ctx, `INSERT INTO mfa_policies (`+mfaPolicyCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			p.Scope, p.Mode, string(roles), p.GraceDays, p.RememberDays, nullTime(p.RequiredSince), p.UpdatedBy, now)
		return err
	})
}

// subsetOf reports whether every element of a is in b.
func subsetOf(a, b []string) bool {
	set := map[string]bool{}
	for _, x := range b {
		set[x] = true
	}
	for _, x := range a {
		if !set[x] {
			return false
		}
	}
	return true
}
