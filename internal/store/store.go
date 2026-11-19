// Package store implements persistence for all domain entities. Every
// tenant-owned query is filtered by tenant_id to guarantee isolation.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/rom/qtestframe/internal/db"
)

// Sentinel errors.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrInvalid  = errors.New("invalid")
)

// Invalidf returns an ErrInvalid wrapping a formatted message.
func Invalidf(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Conflictf returns an ErrConflict wrapping a formatted message.
func Conflictf(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(format, a...))
}

// Store provides data access.
type Store struct {
	DB  *db.DB
	now func() time.Time
}

// New returns a store over database d.
func New(d *db.DB) *Store {
	return &Store{DB: d, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}
}

// SetClock overrides the time source (tests).
func (s *Store) SetClock(f func() time.Time) { s.now = f }

// querier is satisfied by *sql.DB and *sql.Tx.
type querier interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

// conn wraps a querier and rebinds placeholders.
type conn struct {
	q querier
	d db.Dialect
}

func (c conn) exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return c.q.ExecContext(ctx, db.Rebind(c.d, q), args...)
}

func (c conn) query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return c.q.QueryContext(ctx, db.Rebind(c.d, q), args...)
}

func (c conn) row(ctx context.Context, q string, args ...any) *sql.Row {
	return c.q.QueryRowContext(ctx, db.Rebind(c.d, q), args...)
}

func (s *Store) c() conn { return conn{q: s.DB.DB, d: s.DB.Dialect} }

// tx runs fn inside a transaction.
func (s *Store) tx(ctx context.Context, fn func(c conn) error) error {
	t, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(conn{q: t, d: s.DB.Dialect}); err != nil {
		_ = t.Rollback()
		return err
	}
	return t.Commit()
}

func newID() string { return uuid.NewString() }

// mapErr converts driver errors into sentinel errors.
func mapErr(err error, what string) error {
	switch {
	case err == nil:
		return nil
	case db.IsNoRows(err):
		return fmt.Errorf("%s: %w", what, ErrNotFound)
	case db.IsUniqueViolation(err):
		return fmt.Errorf("%w: %s already exists", ErrConflict, what)
	}
	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && (s[n]&0xC0) == 0x80 {
		n--
	}
	return s[:n]
}

func encodeTags(tags []string) string {
	b, _ := json.Marshal(cleanTags(tags))
	return string(b)
}

// cleanTags trims, drops empty and duplicate tags.
func cleanTags(tags []string) []string {
	clean := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		clean = append(clean, t)
	}
	return clean
}

func decodeTags(s string) []string {
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func timePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}

func nullStr(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

func strPtr(ns sql.NullString) *string {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	v := ns.String
	return &v
}

// placeholders returns "?, ?, ?" with n entries.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

func anySlice(ids []string) []any {
	out := make([]any, len(ids))
	for i, v := range ids {
		out[i] = v
	}
	return out
}

func scanStrings(rows *sql.Rows) ([]string, error) {
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
