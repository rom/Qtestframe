package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/rom/qtestframe/internal/suggest"
)

// Autocomplete history (table suggest_history, migration 0028): the values
// a user saved in free-text fields, per organisation, field key and
// project. See internal/suggest for the ranking and internal/api/suggest.go
// for the field keys and the access checks.

// SuggestEntry is one value saved in a field.
type SuggestEntry struct {
	ProjectID string `json:"project_id"` // "" for a field outside a project
	Field     string `json:"field"`
	Value     string `json:"value"`
}

// SuggestHistoryRow is a recorded value with the project it was saved in.
type SuggestHistoryRow struct {
	ProjectID string
	suggest.Candidate
}

func valueHash(v string) string {
	h := sha256.Sum256([]byte(v))
	return hex.EncodeToString(h[:])
}

// RecordSuggestHistory records values a user saved: a new value is added, a
// known one counts one more use and becomes the most recent. Values that
// suggest.Normalize rejects are skipped. Afterwards each field keeps at
// most suggest.HistoryCap values (the least recently used are deleted).
func (s *Store) RecordSuggestHistory(ctx context.Context, tenantID, userID string, entries []SuggestEntry) error {
	if len(entries) == 0 {
		return nil
	}
	now := s.now()
	return s.tx(ctx, func(c conn) error {
		fields := map[string]bool{}
		for _, e := range entries {
			v, ok := suggest.Normalize(e.Value)
			if !ok || e.Field == "" {
				continue
			}
			if _, err := c.exec(ctx, `INSERT INTO suggest_history (id, user_id, tenant_id, project_id, field, value, value_hash, uses, last_used)
				VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?)
				ON CONFLICT (user_id, tenant_id, field, project_id, value_hash)
				DO UPDATE SET uses = suggest_history.uses + 1, last_used = excluded.last_used`,
				newID(), userID, tenantID, e.ProjectID, e.Field, v, valueHash(v), now); err != nil {
				return fmt.Errorf("suggest history: %w", err)
			}
			fields[e.Field] = true
		}
		for f := range fields {
			if _, err := c.exec(ctx, `DELETE FROM suggest_history WHERE user_id = ? AND tenant_id = ? AND field = ? AND id NOT IN (
				SELECT id FROM suggest_history WHERE user_id = ? AND tenant_id = ? AND field = ?
				ORDER BY last_used DESC, uses DESC, id LIMIT ?)`,
				userID, tenantID, f, userID, tenantID, f, suggest.HistoryCap); err != nil {
				return fmt.Errorf("suggest history cap: %w", err)
			}
		}
		return nil
	})
}

// SuggestHistory returns a user's recorded values of a field in an
// organisation, in every project (at most suggest.HistoryCap rows); the
// caller keeps those of the projects the user may read.
func (s *Store) SuggestHistory(ctx context.Context, tenantID, userID, field string) ([]SuggestHistoryRow, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT project_id, value, uses, last_used FROM suggest_history
		WHERE user_id = ? AND tenant_id = ? AND field = ? ORDER BY last_used DESC LIMIT ?`, userID, tenantID, field, suggest.HistoryCap)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SuggestHistoryRow{}
	for rows.Next() {
		var r SuggestHistoryRow
		var t time.Time
		if err := rows.Scan(&r.ProjectID, &r.Value, &r.Uses, &t); err != nil {
			return nil, err
		}
		r.LastUsed, r.Source = t.UTC(), suggest.SourceHistory
		out = append(out, r)
	}
	return out, rows.Err()
}

// CountSuggestHistory returns how many values a user has recorded (all
// organisations).
func (s *Store) CountSuggestHistory(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.c(ctx).row(ctx, `SELECT COUNT(*) FROM suggest_history WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}

// ClearSuggestHistory deletes a user's recorded values, in every
// organisation when tenantID is "", and returns how many were deleted.
func (s *Store) ClearSuggestHistory(ctx context.Context, tenantID, userID string) (int64, error) {
	q, args := `DELETE FROM suggest_history WHERE user_id = ?`, []any{userID}
	if tenantID != "" {
		q, args = q+` AND tenant_id = ?`, append(args, tenantID)
	}
	res, err := s.c(ctx).exec(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// projectValueColumns maps the field keys with project values to the
// column whose distinct values are suggested (a fixed list: the names go
// into SQL).
var projectValueColumns = map[string][2]string{
	"requirement.source": {"requirements", "source"},
	"requirement.owner":  {"requirements", "owner"},
	"risk.owner":         {"risks", "owner"},
}

// HasProjectValues reports whether a field key suggests project values.
func HasProjectValues(field string) bool {
	_, ok := projectValueColumns[field]
	return ok
}

// maxProjectValues bounds the distinct project values considered.
const maxProjectValues = 500

// SuggestProjectValues returns the distinct values of a field in a project
// with how often each is used, the most used first.
func (s *Store) SuggestProjectValues(ctx context.Context, tenantID, projectID, field string) ([]suggest.Candidate, error) {
	tc, ok := projectValueColumns[field]
	if !ok {
		return []suggest.Candidate{}, nil
	}
	table, col := tc[0], tc[1]
	rows, err := s.c(ctx).query(ctx, `SELECT `+col+`, COUNT(*) FROM `+table+` WHERE tenant_id = ? AND project_id = ? AND `+col+` <> ''
		GROUP BY `+col+` ORDER BY COUNT(*) DESC, `+col+` LIMIT ?`, tenantID, projectID, maxProjectValues)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []suggest.Candidate{}
	for rows.Next() {
		c := suggest.Candidate{Source: suggest.SourceProject}
		if err := rows.Scan(&c.Value, &c.Uses); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
