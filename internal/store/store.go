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
	"github.com/rom/qtestframe/internal/secrets"
)

// Sentinel errors.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrInvalid  = errors.New("invalid")
	// ErrStale: the item was saved by someone else since it was read (the
	// version no longer matches). It is also an ErrConflict.
	ErrStale = fmt.Errorf("%w: it was changed by someone else in the meantime", ErrConflict)
)

// sameVersion checks that an UPDATE … AND version = ? hit its row.
func sameVersion(res sql.Result) error {
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrStale
	}
	return nil
}

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
	// keys encrypts the stored secrets (nil: plaintext; see secrets.go)
	keys *secrets.Keyring
	// sealer chains the audit log (auditchain.go)
	sealer *auditSealer
}

// New returns a store over database d.
func New(d *db.DB) *Store {
	return &Store{DB: d, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }, sealer: newAuditSealer()}
}

// SetClock overrides the time source (tests).
func (s *Store) SetClock(f func() time.Time) { s.now = f }

// WithClock returns a store over the same database with another time
// source (e.g. to record backdated demo data).
func (s *Store) WithClock(f func() time.Time) *Store {
	return &Store{DB: s.DB, now: f, keys: s.keys, sealer: s.sealer}
}

// Now returns the current time of the store's clock.
func (s *Store) Now() time.Time { return s.now() }

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

// c returns the connection for ctx: the transaction started by InTx when
// ctx carries one, otherwise the pool.
func (s *Store) c(ctx context.Context) conn {
	if t := txOf(ctx); t != nil {
		return conn{q: t.tx, d: s.DB.Dialect}
	}
	return conn{q: s.DB.DB, d: s.DB.Dialect}
}

// tx runs fn inside a transaction. Inside InTx it joins the transaction
// already open, so a whole request can be atomic.
func (s *Store) tx(ctx context.Context, fn func(c conn) error) error {
	if t := txOf(ctx); t != nil {
		// a savepoint, so a failure undoes only this step and the request
		// transaction stays usable (Postgres aborts it after a failed statement)
		return s.Savepoint(ctx, func() error { return fn(conn{q: t.tx, d: s.DB.Dialect}) })
	}
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

// ---------------------------------------------------------------- request transactions

type txKey struct{}

type ctxTx struct {
	tx    *sql.Tx
	after []func()
	n     int // savepoints so far
}

func txOf(ctx context.Context) *ctxTx {
	t, _ := ctx.Value(txKey{}).(*ctxTx)
	return t
}

// InTx runs fn with a context that carries one transaction: every store
// call made with that context (or one derived from it) uses it, and nested
// transactions join it. When fn returns an error, or panics, nothing is
// saved; otherwise the transaction is committed and the functions given to
// AfterCommit run. Store calls with another context would wait for the
// transaction (SQLite has a single connection), so fn must pass its ctx on.
func (s *Store) InTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if txOf(ctx) != nil {
		return fn(ctx)
	}
	t, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	ct := &ctxTx{tx: t}
	defer func() {
		if p := recover(); p != nil {
			_ = t.Rollback()
			panic(p)
		}
	}()
	if err := fn(context.WithValue(ctx, txKey{}, ct)); err != nil {
		_ = t.Rollback()
		return err
	}
	if err := t.Commit(); err != nil {
		return err
	}
	for _, f := range ct.after {
		f()
	}
	return nil
}

// AfterCommit runs f once the transaction of ctx is committed (never when
// it is rolled back), or at once outside a transaction. Use it for effects
// outside the database, such as webhook deliveries.
func AfterCommit(ctx context.Context, f func()) {
	if t := txOf(ctx); t != nil {
		t.after = append(t.after, f)
		return
	}
	f()
}

// Savepoint runs fn so that, inside a transaction of InTx, an error of fn
// undoes only what fn wrote and the transaction can go on. Call it around
// store calls whose errors a request turns into skipped items. Outside a
// transaction it just runs fn.
func (s *Store) Savepoint(ctx context.Context, fn func() error) error {
	t := txOf(ctx)
	if t == nil {
		return fn()
	}
	t.n++
	name := fmt.Sprintf("qtf_sp%d", t.n)
	if _, err := t.tx.ExecContext(ctx, "SAVEPOINT "+name); err != nil {
		return err
	}
	if err := fn(); err != nil {
		if _, rerr := t.tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+name); rerr != nil {
			return errors.Join(err, rerr)
		}
		_, _ = t.tx.ExecContext(ctx, "RELEASE SAVEPOINT "+name)
		return err
	}
	_, err := t.tx.ExecContext(ctx, "RELEASE SAVEPOINT "+name)
	return err
}

// InTransaction reports whether ctx carries a transaction of InTx.
func InTransaction(ctx context.Context) bool { return txOf(ctx) != nil }

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
