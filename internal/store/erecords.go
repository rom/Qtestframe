package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// ---------------------------------------------------------------- settings

// normalizeERecords fills in defaults and checks the settings.
func normalizeERecords(st *model.ERecordsSettings) error {
	var meanings []string
	for _, m := range st.Meanings {
		m = strings.TrimSpace(m)
		if m == "" || slices.Contains(meanings, m) {
			continue
		}
		if len(m) > 60 {
			return Invalidf("a signature meaning is at most 60 characters: %q", m)
		}
		meanings = append(meanings, m)
	}
	if len(meanings) == 0 {
		meanings = append([]string{}, model.DefaultSignatureMeanings...)
	}
	if len(meanings) > 20 {
		return Invalidf("at most 20 signature meanings")
	}
	st.Meanings = meanings
	clean := func(list, allowed []string, what string) ([]string, error) {
		out := []string{}
		for _, a := range list {
			if !slices.Contains(allowed, a) {
				return nil, Invalidf("unknown %s %q (one of %s)", what, a, strings.Join(allowed, ", "))
			}
			if !slices.Contains(out, a) {
				out = append(out, a)
			}
		}
		return out, nil
	}
	var err error
	if st.SignatureActions, err = clean(st.SignatureActions, model.ERecordsSignatureActions, "signature action"); err != nil {
		return err
	}
	if st.ReasonEdits, err = clean(st.ReasonEdits, model.ERecordsReasonEdits, "edit type"); err != nil {
		return err
	}
	if st.IdleTimeoutMinutes == 0 {
		st.IdleTimeoutMinutes = 15
	}
	if st.IdleTimeoutMinutes < 1 || st.IdleTimeoutMinutes > 480 {
		return Invalidf("the inactivity timeout is 1 to 480 minutes")
	}
	if st.SSOReauthMinutes == 0 {
		st.SSOReauthMinutes = 5
	}
	if st.SSOReauthMinutes < 1 || st.SSOReauthMinutes > 60 {
		return Invalidf("the single sign-on signing window is 1 to 60 minutes")
	}
	return nil
}

// ERecordsSettings returns a tenant's electronic-records settings (off,
// with defaults, when never saved).
func (s *Store) ERecordsSettings(ctx context.Context, tenantID string) (*model.ERecordsSettings, error) {
	var raw, by string
	var at sql.NullTime
	st := &model.ERecordsSettings{}
	err := s.c(ctx).row(ctx, `SELECT settings, updated_by, updated_at FROM erecords_settings WHERE tenant_id = ?`, tenantID).Scan(&raw, &by, &at)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		_ = json.Unmarshal([]byte(raw), st)
		st.UpdatedBy = by
		if at.Valid {
			st.UpdatedAt = at.Time.UTC()
		}
	}
	if err := normalizeERecords(st); err != nil {
		// stored settings that no longer validate: keep the mode, drop the rest
		st = &model.ERecordsSettings{Enabled: st.Enabled}
		_ = normalizeERecords(st)
	}
	return st, nil
}

// SaveERecordsSettings validates and stores a tenant's settings.
func (s *Store) SaveERecordsSettings(ctx context.Context, tenantID string, st *model.ERecordsSettings, by string) error {
	if err := normalizeERecords(st); err != nil {
		return err
	}
	st.UpdatedBy, st.UpdatedAt = by, s.now()
	b, _ := json.Marshal(struct {
		Enabled            bool     `json:"enabled"`
		Meanings           []string `json:"meanings"`
		SignatureActions   []string `json:"signature_actions"`
		ReasonEdits        []string `json:"reason_edits"`
		IdleTimeoutMinutes int      `json:"idle_timeout_minutes"`
		SSOReauthMinutes   int      `json:"sso_reauth_minutes"`
	}{st.Enabled, st.Meanings, st.SignatureActions, st.ReasonEdits, st.IdleTimeoutMinutes, st.SSOReauthMinutes})
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `DELETE FROM erecords_settings WHERE tenant_id = ?`, tenantID); err != nil {
			return err
		}
		_, err := c.exec(ctx, `INSERT INTO erecords_settings (tenant_id, settings, updated_by, updated_at) VALUES (?, ?, ?, ?)`, tenantID, string(b), by, st.UpdatedAt)
		return mapErr(err, "settings")
	})
}

// ---------------------------------------------------------------- signatures

const esigCols = `id, project_id, entity_type, entity_id, entity_key, action, meaning, comment, user_id, user_email, user_name, method, time_zone, content_hash, signed_at`

func scanESig(sc interface{ Scan(...any) error }) (*model.ESignature, error) {
	var e model.ESignature
	if err := sc.Scan(&e.ID, &e.ProjectID, &e.EntityType, &e.EntityID, &e.EntityKey, &e.Action, &e.Meaning, &e.Comment, &e.UserID, &e.UserEmail, &e.UserName,
		&e.Method, &e.TimeZone, &e.ContentHash, &e.SignedAt); err != nil {
		return nil, err
	}
	e.SignedAt = e.SignedAt.UTC()
	return &e, nil
}

// AddESignature records an electronic signature.
func (s *Store) AddESignature(ctx context.Context, tenantID string, e *model.ESignature) error {
	e.ID = newID()
	if e.SignedAt.IsZero() {
		e.SignedAt = s.now()
	}
	if e.TimeZone == "" {
		e.TimeZone = "UTC"
	}
	_, err := s.c(ctx).exec(ctx, `INSERT INTO esignatures (id, tenant_id, `+strings.TrimPrefix(esigCols, "id, ")+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, tenantID, e.ProjectID, e.EntityType, e.EntityID, e.EntityKey, e.Action, e.Meaning, e.Comment, e.UserID, e.UserEmail, e.UserName,
		e.Method, e.TimeZone, e.ContentHash, e.SignedAt)
	return err
}

// ESignatureFilter selects signatures (empty fields: any).
type ESignatureFilter struct {
	ProjectID  string
	EntityType string
	EntityID   string
	Limit      int
}

// ListESignatures returns signatures, newest first.
func (s *Store) ListESignatures(ctx context.Context, tenantID string, f ESignatureFilter) ([]model.ESignature, error) {
	q := `SELECT ` + esigCols + ` FROM esignatures WHERE tenant_id = ?`
	args := []any{tenantID}
	if f.ProjectID != "" {
		q += ` AND project_id = ?`
		args = append(args, f.ProjectID)
	}
	if f.EntityType != "" {
		q += ` AND entity_type = ?`
		args = append(args, f.EntityType)
	}
	if f.EntityID != "" {
		q += ` AND entity_id = ?`
		args = append(args, f.EntityID)
	}
	if f.Limit <= 0 || f.Limit > 5000 {
		f.Limit = 500
	}
	rows, err := s.c(ctx).query(ctx, q+` ORDER BY signed_at DESC, id LIMIT ?`, append(args, f.Limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ESignature{}
	for rows.Next() {
		e, err := scanESig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// CountESignatures counts a tenant's signatures by action.
func (s *Store) CountESignatures(ctx context.Context, tenantID string) (map[string]int, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT action, COUNT(*) FROM esignatures WHERE tenant_id = ? GROUP BY action`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var a string
		var n int
		if err := rows.Scan(&a, &n); err != nil {
			return nil, err
		}
		out[a] = n
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- tickets

// SigningTicket is the proof of a re-authentication for one action.
type SigningTicket struct {
	TenantID  string
	UserID    string
	SessionID string
	Action    string
	Meaning   string
	Comment   string
	Method    string
	TimeZone  string
	ExpiresAt time.Time
}

// AddSigningTicket stores a ticket under the hash of its secret (expired
// tickets are removed).
func (s *Store) AddSigningTicket(ctx context.Context, hash string, t *SigningTicket) error {
	if _, err := s.c(ctx).exec(ctx, `DELETE FROM esign_tickets WHERE expires_at < ?`, s.now()); err != nil {
		return err
	}
	_, err := s.c(ctx).exec(ctx, `INSERT INTO esign_tickets (hash, tenant_id, user_id, session_id, action, meaning, comment, method, time_zone, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		hash, t.TenantID, t.UserID, t.SessionID, t.Action, t.Meaning, t.Comment, t.Method, t.TimeZone, t.ExpiresAt)
	return err
}

// TakeSigningTicket returns and deletes a valid ticket (single use); it
// answers ErrNotFound for an unknown, used or expired one.
func (s *Store) TakeSigningTicket(ctx context.Context, hash string) (*SigningTicket, error) {
	var t SigningTicket
	err := s.c(ctx).row(ctx, `SELECT tenant_id, user_id, session_id, action, meaning, comment, method, time_zone, expires_at FROM esign_tickets WHERE hash = ?`, hash).
		Scan(&t.TenantID, &t.UserID, &t.SessionID, &t.Action, &t.Meaning, &t.Comment, &t.Method, &t.TimeZone, &t.ExpiresAt)
	if err != nil {
		return nil, mapErr(err, "signing ticket")
	}
	res, err := s.c(ctx).exec(ctx, `DELETE FROM esign_tickets WHERE hash = ?`, hash)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 || !t.ExpiresAt.After(s.now()) {
		return nil, mapErr(sql.ErrNoRows, "signing ticket")
	}
	return &t, nil
}
