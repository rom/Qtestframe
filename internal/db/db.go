// Package db opens SQLite or PostgreSQL databases and applies migrations.
//
// SQL in this code base is written with `?` placeholders and portable types;
// Rebind converts placeholders for PostgreSQL and migration templates are
// rendered per dialect.
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // postgres driver
	_ "modernc.org/sqlite"             // sqlite driver (pure Go)
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Dialect identifies the SQL backend.
type Dialect string

// Supported dialects.
const (
	SQLite   Dialect = "sqlite"
	Postgres Dialect = "postgres"
)

// DB wraps *sql.DB with dialect information.
type DB struct {
	*sql.DB
	Dialect Dialect
}

// Open connects to the database. For sqlite, dsn is a file path or
// ":memory:"; pragmas for WAL, foreign keys and busy timeout are added.
func Open(ctx context.Context, driver, dsn string, maxOpen int) (*DB, error) {
	var (
		d   Dialect
		sdb *sql.DB
		err error
	)
	switch driver {
	case "sqlite":
		d = SQLite
		if err := ensureSQLiteDir(dsn); err != nil {
			return nil, err
		}
		sdb, err = sql.Open("sqlite", sqliteDSN(dsn))
		if err == nil {
			// A single connection serialises writers and avoids SQLITE_BUSY.
			sdb.SetMaxOpenConns(1)
		}
	case "postgres", "postgresql", "pgx":
		d = Postgres
		sdb, err = sql.Open("pgx", dsn)
		if err == nil {
			if maxOpen <= 0 {
				maxOpen = 20
			}
			sdb.SetMaxOpenConns(maxOpen)
			sdb.SetConnMaxIdleTime(5 * time.Minute)
		}
	default:
		return nil, fmt.Errorf("db: unsupported driver %q", driver)
	}
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}
	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := sdb.PingContext(pctx); err != nil {
		sdb.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return &DB{DB: sdb, Dialect: d}, nil
}

// ensureSQLiteDir creates the parent directory of a file database (mode
// 0700) so that a fresh install can point at e.g. ./data/qtestframe.db.
func ensureSQLiteDir(dsn string) error {
	path := strings.TrimPrefix(strings.SplitN(dsn, "?", 2)[0], "file:")
	if path == "" || path == ":memory:" || strings.HasPrefix(path, ":memory:") {
		return nil
	}
	dir := filepath.Dir(path)
	if dir == "." || dir == "/" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("db: create directory for %s: %w", path, err)
	}
	return nil
}

func sqliteDSN(dsn string) string {
	if strings.Contains(dsn, "_pragma=") {
		return dsn
	}
	base := dsn
	if dsn == ":memory:" {
		base = "file::memory:"
	} else if !strings.HasPrefix(dsn, "file:") {
		base = "file:" + dsn
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_time_format=sqlite"
}

// Rebind converts `?` placeholders to `$n` for PostgreSQL. Question marks
// inside single-quoted literals are preserved.
func (db *DB) Rebind(q string) string {
	return Rebind(db.Dialect, q)
}

// Rebind converts placeholders for dialect d.
func Rebind(d Dialect, q string) string {
	if d != Postgres {
		return q
	}
	var b strings.Builder
	b.Grow(len(q) + 16)
	n := 0
	inStr := false
	for i := 0; i < len(q); i++ {
		c := q[i]
		switch {
		case c == '\'':
			inStr = !inStr
			b.WriteByte(c)
		case c == '?' && !inStr:
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// sqliteUUID is an SQL expression giving a new random (version 4) UUID in
// SQLite: the {{UUID}} token of migrations that create rows (row ids are
// UUIDs, which project archives rewrite when they are loaded).
const sqliteUUID = `(lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
	substr('89ab', 1 + (abs(random()) % 4), 1) || substr(lower(hex(randomblob(2))), 2) || '-' || lower(hex(randomblob(6))))`

// Render substitutes dialect tokens in a migration template.
func Render(d Dialect, tmpl string) string {
	r := map[Dialect]*strings.Replacer{
		SQLite: strings.NewReplacer("{{TS}}", "TIMESTAMP", "{{BOOL}}", "INTEGER", "{{FALSE}}", "0", "{{TRUE}}", "1", "{{BLOB}}", "BLOB",
			"{{UUID}}", sqliteUUID),
		Postgres: strings.NewReplacer("{{TS}}", "TIMESTAMPTZ", "{{BOOL}}", "BOOLEAN", "{{FALSE}}", "FALSE", "{{TRUE}}", "TRUE", "{{BLOB}}", "BYTEA",
			"{{UUID}}", "gen_random_uuid()::text"),
	}[d]
	return r.Replace(tmpl)
}

// Migration is one numbered schema change.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// Migrations returns all embedded migrations in order.
func Migrations() ([]Migration, error) {
	return loadMigrations(migrationFS, "migrations")
}

func loadMigrations(fsys fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var out []Migration
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		num, _, ok := strings.Cut(name, "_")
		v, err := strconv.Atoi(num)
		if !ok || err != nil {
			return nil, fmt.Errorf("db: bad migration name %q", name)
		}
		b, err := fs.ReadFile(fsys, dir+"/"+name)
		if err != nil {
			return nil, err
		}
		out = append(out, Migration{Version: v, Name: name, SQL: string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// Migrate applies pending migrations and returns the number applied.
func (db *DB) Migrate(ctx context.Context) (int, error) {
	ms, err := Migrations()
	if err != nil {
		return 0, err
	}
	return db.apply(ctx, ms)
}

func (db *DB) apply(ctx context.Context, ms []Migration) (int, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		return 0, fmt.Errorf("db: migrations table: %w", err)
	}
	applied := map[int]bool{}
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return 0, err
		}
		applied[v] = true
	}
	rows.Close()
	n := 0
	for _, m := range ms {
		if applied[m.Version] {
			continue
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return n, err
		}
		for _, stmt := range splitStatements(Render(db.Dialect, m.SQL)) {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				tx.Rollback()
				return n, fmt.Errorf("db: migration %s: %w", m.Name, err)
			}
		}
		if _, err := tx.ExecContext(ctx, db.Rebind(`INSERT INTO schema_migrations (version, name) VALUES (?, ?)`), m.Version, m.Name); err != nil {
			tx.Rollback()
			return n, err
		}
		if err := tx.Commit(); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// splitStatements splits SQL on semicolons at line ends and strips comments.
func splitStatements(s string) []string {
	var out []string
	var cur strings.Builder
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "--") {
			continue
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
		if strings.HasSuffix(t, ";") {
			stmt := strings.TrimSpace(cur.String())
			out = append(out, strings.TrimSuffix(stmt, ";"))
			cur.Reset()
		}
	}
	if rest := strings.TrimSpace(cur.String()); rest != "" {
		out = append(out, rest)
	}
	return out
}

// IsUniqueViolation reports whether err is a unique constraint violation.
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "duplicate key value") ||
		strings.Contains(msg, "SQLSTATE 23505")
}

// ErrNoRows aliases sql.ErrNoRows for callers.
var ErrNoRows = sql.ErrNoRows

// IsNoRows reports whether err wraps sql.ErrNoRows.
func IsNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }
