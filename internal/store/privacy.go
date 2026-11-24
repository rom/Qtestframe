package store

// Data subject tools (GDPR articles 15, 17 and 20): the export of the
// personal data held about an account and its erasure (anonymisation).
//
// Erasure keeps the account row (so that authored content, sign-offs,
// electronic signatures and the audit trail keep a valid reference) but
// replaces its name and address with a pseudonym, deletes its personal
// settings, sign-in data and inbox, and rewrites the e-mail address where
// authored content names its author. Electronic signatures, sign-offs and
// the audit trail are kept unchanged: they are records that regulations
// (21 CFR Part 11, EU GMP Annex 11) require to keep with the signer's
// printed name, and the audit trail is a hash chain that a rewrite would
// break.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
)

// ExportSection is one file of a personal data export: a name and rows.
type ExportSection struct {
	Name string           `json:"name"`
	Rows []map[string]any `json:"rows"`
}

// queryMaps returns the rows of q as maps of column → value.
func queryMaps(ctx context.Context, c conn, q string, args ...any) ([]map[string]any, error) {
	rows, err := c.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, col := range cols {
			v := vals[i]
			switch x := v.(type) {
			case []byte:
				v = string(x)
			case time.Time:
				v = x.UTC()
			}
			// JSON columns are exported as JSON
			if s, ok := v.(string); ok && len(s) > 1 && (s[0] == '{' || s[0] == '[') && json.Valid([]byte(s)) {
				v = json.RawMessage(s)
			}
			m[col] = v
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// likeEscape escapes the wildcards of a LIKE pattern (with ESCAPE '\\').
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s)
}

// mentionHandle is the handle "@<local part>" of an address.
func mentionHandle(email string) string {
	local, _, _ := strings.Cut(email, "@")
	return local
}

// ExportUserData collects the personal data held about an account: its
// profile, memberships and roles, authored comments, mentions, sessions,
// API tokens (without their secrets), sign-in log, notifications,
// electronic signatures, the audit entries by or about it and its input
// history. Secrets (password hash, second factor, token hashes) are left
// out.
func (s *Store) ExportUserData(ctx context.Context, userID string) ([]ExportSection, error) {
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	c := s.c(ctx)
	mfa := false
	var n int
	if err := c.row(ctx, `SELECT COUNT(*) FROM user_mfa WHERE user_id = ? AND enabled_at IS NOT NULL`, userID).Scan(&n); err != nil {
		return nil, err
	}
	mfa = n > 0
	profile := map[string]any{"id": u.ID, "email": u.Email, "display_name": u.DisplayName, "super_admin": u.SuperAdmin, "auth_provider": u.AuthProvider,
		"external_id": u.ExternalID, "disabled": u.Disabled, "created_at": u.CreatedAt, "preferences": u.Preferences, "avatar": u.Avatar,
		"password_changed_at": u.PasswordChangedAt, "mfa_enabled": mfa}
	out := []ExportSection{{Name: "profile", Rows: []map[string]any{profile}}}
	like := "%@" + likeEscape(mentionHandle(u.Email)) + "%"
	for _, q := range []struct{ name, q string }{
		{"memberships", `SELECT m.tenant_id, t.slug AS tenant_slug, t.name AS tenant_name, m.role, m.roles, m.created_at FROM memberships m JOIN tenants t ON t.id = m.tenant_id WHERE m.user_id = ? ORDER BY t.slug`},
		{"project_roles", `SELECT pm.tenant_id, pm.project_id, p.key AS project_key, p.name AS project_name, pm.role, pm.added_by, pm.created_at FROM project_members pm JOIN projects p ON p.id = pm.project_id WHERE pm.member_type = 'user' AND pm.member_id = ? ORDER BY p.key`},
		{"comments", `SELECT id, tenant_id, entity_type, entity_id, body, created_at, updated_at FROM comments WHERE user_id = ? ORDER BY created_at`},
		{"sessions", `SELECT tenant_id, created_at, expires_at, last_seen, ip, user_agent FROM sessions WHERE user_id = ? ORDER BY created_at`},
		{"api_tokens", `SELECT tenant_id, name, prefix, roles, created_at, expires_at, last_used FROM api_tokens WHERE user_id = ? ORDER BY created_at`},
		{"sign_in_log", `SELECT success, method, ip, user_agent, created_at FROM login_events WHERE user_id = ? ORDER BY created_at`},
		{"trusted_devices", `SELECT ip, user_agent, created_at, expires_at FROM mfa_trusted_devices WHERE user_id = ? ORDER BY created_at`},
		{"notifications", `SELECT tenant_id, project_id, event, title, body, entity_type, entity_id, actor, read_at, created_at FROM notifications WHERE user_id = ? ORDER BY created_at`},
		{"electronic_signatures", `SELECT tenant_id, project_id, entity_type, entity_id, entity_key, action, meaning, comment, user_email, user_name, method, time_zone, signed_at FROM esignatures WHERE user_id = ? ORDER BY signed_at`},
		{"input_history", `SELECT tenant_id, project_id, field, value, uses, last_used FROM suggest_history WHERE user_id = ? ORDER BY last_used`},
	} {
		rows, err := queryMaps(ctx, c, q.q, userID)
		if err != nil {
			return nil, fmt.Errorf("export %s: %w", q.name, err)
		}
		out = append(out, ExportSection{Name: q.name, Rows: rows})
	}
	mentions, err := queryMaps(ctx, c, `SELECT id, tenant_id, entity_type, entity_id, user_id AS author_id, body, created_at FROM comments
		WHERE user_id <> ? AND body LIKE ? ESCAPE '\' ORDER BY created_at`, userID, like)
	if err != nil {
		return nil, err
	}
	out = append(out, ExportSection{Name: "mentions", Rows: mentions})
	audit, err := queryMaps(ctx, c, `SELECT tenant_id, seq, created_at, user_email, action, entity_type, entity_id, details, reason FROM audit_log
		WHERE user_id = ? OR (entity_type = 'user' AND entity_id = ?) ORDER BY created_at, seq`, userID, userID)
	if err != nil {
		return nil, err
	}
	out = append(out, ExportSection{Name: "audit_entries", Rows: audit})
	return out, nil
}

// ErasedPseudonym is the name an erased account gets.
func ErasedPseudonym(n int) string { return fmt.Sprintf("Former user #%d", n) }

// ErasedEmail is the address an erased account gets.
func ErasedEmail(n int) string { return fmt.Sprintf("former-user-%d@%s", n, model.ErasedEmailDomain) }

// ErasureResult says what an erasure did.
type ErasureResult struct {
	UserID    string `json:"user_id"`
	Pseudonym string `json:"pseudonym"`
	Email     string `json:"email"`
	// Deleted counts the deleted personal records by kind.
	Deleted map[string]int `json:"deleted"`
	// Rewritten is the number of fields of authored content now naming the
	// pseudonym.
	Rewritten int `json:"rewritten"`
	// Kept counts the records kept unchanged (electronic signatures,
	// sign-offs, audit entries).
	Kept map[string]int `json:"kept"`
}

// erasureKeep are the tables erasure leaves unchanged: records that must
// stay as signed or written (signatures, sign-offs, the audit chain).
var erasureKeep = []string{"audit_log", "audit_heads", "audit_checkpoints", "audit_keys", "audit_backfill", "esignatures",
	"baseline_signoffs", "waiver_signoffs", "change_request_signoffs", "users", "schema_migrations"}

// erasureDelete are the personal tables whose rows of the account are
// deleted (column user_id).
var erasureDelete = []string{"sessions", "api_tokens", "user_mfa", "mfa_recovery_codes", "mfa_trusted_devices", "mfa_challenges", "esign_tickets",
	"notifications", "login_events", "suggest_history", "invitations", "scim_users", "scim_group_members", "memberships"}

// personColumn reports whether a column names a person by e-mail (the
// author, owner or assignee of something).
func personColumn(name string) bool {
	switch name {
	case "assignee", "tester", "owner", "actor", "user_email", "author", "reviewer", "requester", "email":
		return true
	}
	return strings.HasSuffix(name, "_by")
}

// textColumns returns the columns of a table.
func textColumns(ctx context.Context, c conn, table string) ([]string, error) {
	q := `SELECT name FROM pragma_table_info(?)`
	if c.d == db.Postgres {
		q = `SELECT column_name FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND data_type = 'text'`
	}
	rows, err := c.query(ctx, q, table)
	if err != nil {
		return nil, err
	}
	return scanStrings(rows)
}

// EraseUser anonymises an account (see the package comment above): the
// next pseudonym number, its name and address replaced, personal records
// deleted, authored content attributed to the pseudonym. An erased account
// cannot sign in again.
func (s *Store) EraseUser(ctx context.Context, userID string) (*ErasureResult, error) {
	res := &ErasureResult{UserID: userID, Deleted: map[string]int{}, Kept: map[string]int{}}
	err := s.tx(ctx, func(c conn) error {
		u, err := scanUser(c.row(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, userID))
		if err != nil {
			return mapErr(err, "user")
		}
		if u.Erased {
			return Conflictf("this account was already erased")
		}
		var n int
		if err := c.row(ctx, `SELECT COALESCE(MAX(erased_no), 0) + 1 FROM users`).Scan(&n); err != nil {
			return err
		}
		res.Pseudonym, res.Email = ErasedPseudonym(n), ErasedEmail(n)
		for _, k := range []struct{ name, q string }{
			{"electronic_signatures", `SELECT COUNT(*) FROM esignatures WHERE user_id = ?`},
			{"audit_entries", `SELECT COUNT(*) FROM audit_log WHERE user_id = ?`},
			{"sign_offs", `SELECT (SELECT COUNT(*) FROM baseline_signoffs WHERE user_id = ?) + (SELECT COUNT(*) FROM waiver_signoffs WHERE user_id = ?) + (SELECT COUNT(*) FROM change_request_signoffs WHERE user_id = ?)`},
		} {
			var k2 int
			args := []any{userID}
			if k.name == "sign_offs" {
				args = []any{userID, userID, userID}
			}
			if err := c.row(ctx, k.q, args...).Scan(&k2); err != nil {
				return err
			}
			res.Kept[k.name] = k2
		}
		// the avatar picture
		if id, ok := strings.CutPrefix(u.Avatar, "file:"); ok {
			if _, err := c.exec(ctx, `DELETE FROM file_blobs WHERE file_id IN (SELECT id FROM files WHERE id = ? AND kind = ?)`, id, model.FileAvatar); err != nil {
				return err
			}
			if _, err := c.exec(ctx, `DELETE FROM files WHERE id = ? AND kind = ?`, id, model.FileAvatar); err != nil {
				return err
			}
		}
		for _, t := range erasureDelete {
			r, err := c.exec(ctx, `DELETE FROM `+t+` WHERE user_id = ?`, userID)
			if err != nil {
				return fmt.Errorf("erase %s: %w", t, err)
			}
			if k, _ := r.RowsAffected(); k > 0 {
				res.Deleted[t] = int(k)
			}
		}
		r, err := c.exec(ctx, `DELETE FROM project_members WHERE member_type = 'user' AND member_id = ?`, userID)
		if err != nil {
			return err
		}
		if k, _ := r.RowsAffected(); k > 0 {
			res.Deleted["project_members"] = int(k)
		}
		// authored content: the columns naming a person by address
		tables, err := listTables(ctx, c)
		if err != nil {
			return err
		}
		slices.Sort(tables)
		for _, t := range tables {
			if slices.Contains(erasureKeep, t) || strings.HasPrefix(t, "sqlite_") {
				continue
			}
			cols, err := textColumns(ctx, c, t)
			if err != nil {
				return err
			}
			for _, col := range cols {
				if !personColumn(col) {
					continue
				}
				r, err := c.exec(ctx, `UPDATE `+quoteIdent(t)+` SET `+quoteIdent(col)+` = ? WHERE `+quoteIdent(col)+` = ?`, res.Email, u.Email)
				if err != nil {
					return fmt.Errorf("erase %s.%s: %w", t, col, err)
				}
				k, _ := r.RowsAffected()
				res.Rewritten += int(k)
			}
		}
		// exploratory debriefs also keep the name
		if _, err := c.exec(ctx, `UPDATE exploratory_sessions SET debriefed_name = ? WHERE debriefed_by = ?`, res.Pseudonym, res.Email); err != nil {
			return err
		}
		// mentions in comments: "@<old handle>" → "@<pseudonym handle>"
		if old := mentionHandle(u.Email); old != "" {
			re := regexp.MustCompile(`(^|[^\p{L}\p{N}])@` + regexp.QuoteMeta(old) + `($|[^\p{L}\p{N}._-]|[._-]($|[^\p{L}\p{N}]))`)
			rows, err := queryMaps(ctx, c, `SELECT id, body FROM comments WHERE body LIKE ? ESCAPE '\'`, "%@"+likeEscape(old)+"%")
			if err != nil {
				return err
			}
			for _, row := range rows {
				body, _ := row["body"].(string)
				next := re.ReplaceAllString(body, "${1}@"+mentionHandle(res.Email)+"${2}")
				if next == body {
					continue
				}
				if _, err := c.exec(ctx, `UPDATE comments SET body = ? WHERE id = ?`, next, row["id"]); err != nil {
					return err
				}
				res.Rewritten++
			}
		}
		_, err = c.exec(ctx, `UPDATE users SET email = ?, display_name = ?, password_hash = '', super_admin = ?, auth_provider = 'local', external_id = '',
			disabled = ?, preferences = '{}', avatar = '', must_change_password = ?, password_changed_at = NULL, password_history = '[]',
			erased_no = ?, erased_at = ? WHERE id = ?`, res.Email, res.Pseudonym, false, true, false, n, s.now(), userID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// CountSuperAdmins returns the number of enabled super administrators.
func (s *Store) CountSuperAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.c(ctx).row(ctx, `SELECT COUNT(*) FROM users WHERE super_admin = ? AND disabled = ?`, true, false).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}
