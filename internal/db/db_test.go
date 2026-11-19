package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func openMem(t *testing.T) *DB {
	t.Helper()
	d, err := Open(context.Background(), "sqlite", ":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestOpenSQLiteMemoryAndMigrate(t *testing.T) {
	ctx := context.Background()
	d := openMem(t)
	if d.Dialect != SQLite {
		t.Fatalf("dialect = %s", d.Dialect)
	}
	n, err := d.Migrate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ms, _ := Migrations()
	if n != len(ms) || n == 0 {
		t.Fatalf("applied %d, want %d", n, len(ms))
	}
	n, err = d.Migrate(ctx)
	if err != nil || n != 0 {
		t.Fatalf("second migrate: n=%d err=%v", n, err)
	}
	var fk int
	if err := d.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Errorf("foreign_keys = %d (%v)", fk, err)
	}
	checkUniqueAndNoRows(t, d)
}

func checkUniqueAndNoRows(t *testing.T, d *DB) {
	t.Helper()
	ctx := context.Background()
	ins := d.Rebind(`INSERT INTO tenants (id, slug, name, created_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`)
	if _, err := d.ExecContext(ctx, ins, "t1", "acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	_, err := d.ExecContext(ctx, ins, "t2", "acme", "Acme 2")
	if !IsUniqueViolation(err) {
		t.Errorf("IsUniqueViolation(%v) = false", err)
	}
	var name string
	err = d.QueryRowContext(ctx, d.Rebind(`SELECT name FROM tenants WHERE id = ?`), "missing").Scan(&name)
	if !IsNoRows(err) || !errors.Is(err, ErrNoRows) {
		t.Errorf("IsNoRows(%v) = false", err)
	}
}

func TestOpenSQLiteFile(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "q.db")
	d, err := Open(ctx, "sqlite", path, 5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var mode string
	if err := d.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Errorf("journal_mode = %q (%v)", mode, err)
	}
	d.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	// Reopen: nothing left to migrate.
	d, err = Open(ctx, "sqlite", "file:"+path, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if n, err := d.Migrate(ctx); err != nil || n != 0 {
		t.Fatalf("reopen migrate n=%d err=%v", n, err)
	}
}

func TestOpenErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := Open(ctx, "mysql", "x", 0); err == nil || !strings.Contains(err.Error(), "unsupported driver") {
		t.Errorf("unsupported: %v", err)
	}
	// A directory where the database file should be: the ping fails.
	bad := t.TempDir()
	if _, err := Open(ctx, "sqlite", bad, 0); err == nil || !strings.Contains(err.Error(), "ping") {
		t.Errorf("bad sqlite path: %v", err)
	}
	// Nothing listens on port 1: the ping fails quickly.
	if _, err := Open(ctx, "postgres", "postgres://u:p@127.0.0.1:1/db?sslmode=disable&connect_timeout=2", 0); err == nil || !strings.Contains(err.Error(), "ping") {
		t.Errorf("bad postgres: %v", err)
	}
	if _, err := Open(ctx, "pgx", "::not a dsn::", 3); err == nil {
		t.Error("expected error for invalid postgres dsn")
	}
}

func TestSQLiteDSN(t *testing.T) {
	cases := map[string]string{
		":memory:":              "file::memory:?_pragma=",
		"a.db":                  "file:a.db?_pragma=",
		"file:a.db":             "file:a.db?_pragma=",
		"file:a.db?mode=ro":     "file:a.db?mode=ro&_pragma=",
		"file:x?_pragma=foo(1)": "file:x?_pragma=foo(1)",
	}
	for in, prefix := range cases {
		got := sqliteDSN(in)
		if !strings.HasPrefix(got, prefix) {
			t.Errorf("sqliteDSN(%q) = %q, want prefix %q", in, got, prefix)
		}
	}
	if got := sqliteDSN("file:x?_pragma=foo(1)"); got != "file:x?_pragma=foo(1)" {
		t.Errorf("explicit pragmas changed: %q", got)
	}
}

func TestRebind(t *testing.T) {
	q := `SELECT * FROM t WHERE a = ? AND b = '?' AND c IN (?, ?) AND d = 'it''s ?'`
	if got := Rebind(SQLite, q); got != q {
		t.Errorf("sqlite changed query: %q", got)
	}
	want := `SELECT * FROM t WHERE a = $1 AND b = '?' AND c IN ($2, $3) AND d = 'it''s ?'`
	if got := Rebind(Postgres, q); got != want {
		t.Errorf("Rebind = %q\nwant      %q", got, want)
	}
	d := &DB{Dialect: Postgres}
	if got := d.Rebind("?"); got != "$1" {
		t.Errorf("method Rebind = %q", got)
	}
}

func TestRender(t *testing.T) {
	tmpl := "a {{TS}} b {{BOOL}} DEFAULT {{FALSE}} {{TRUE}}"
	if got := Render(SQLite, tmpl); got != "a TIMESTAMP b INTEGER DEFAULT 0 1" {
		t.Errorf("sqlite: %q", got)
	}
	if got := Render(Postgres, tmpl); got != "a TIMESTAMPTZ b BOOLEAN DEFAULT FALSE TRUE" {
		t.Errorf("postgres: %q", got)
	}
}

func TestSplitStatements(t *testing.T) {
	in := "-- comment\nCREATE TABLE a (\n  x INT\n);\n\n  -- indented comment\nINSERT INTO a VALUES (1);\nSELECT 1"
	got := splitStatements(in)
	want := []string{"CREATE TABLE a (\n  x INT\n)", "INSERT INTO a VALUES (1)", "SELECT 1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
	if got := splitStatements("\n-- only\n"); len(got) != 0 {
		t.Errorf("empty: %q", got)
	}
}

func TestLoadMigrations(t *testing.T) {
	fsys := fstest.MapFS{
		"m/0002_second.sql": {Data: []byte("SELECT 2;")},
		"m/0001_first.sql":  {Data: []byte("SELECT 1;")},
		"m/README.md":       {Data: []byte("ignored")},
	}
	ms, err := loadMigrations(fsys, "m")
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 2 || ms[0].Version != 1 || ms[1].Version != 2 || ms[0].Name != "0001_first.sql" || ms[1].SQL != "SELECT 2;" {
		t.Fatalf("unexpected %+v", ms)
	}
	for _, bad := range []string{"m/first.sql", "m/x_first.sql", "m/0003.sql"} {
		f := fstest.MapFS{bad: {Data: []byte("x")}}
		if _, err := loadMigrations(f, "m"); err == nil || !strings.Contains(err.Error(), "bad migration name") {
			t.Errorf("%s: err = %v", bad, err)
		}
	}
	if _, err := loadMigrations(fsys, "missing"); err == nil {
		t.Error("expected error for missing directory")
	}
	// A directory named like a migration cannot be read as a file.
	dirFS := fstest.MapFS{"m/0001_dir.sql/x": {Data: []byte("x")}}
	if _, err := loadMigrations(dirFS, "m"); err == nil {
		t.Error("expected read error")
	}
	emb, err := Migrations()
	if err != nil || len(emb) == 0 || emb[0].Version != 1 {
		t.Fatalf("embedded migrations: %v %+v", err, emb)
	}
}

func TestApplyErrors(t *testing.T) {
	ctx := context.Background()

	d := openMem(t)
	n, err := d.apply(ctx, []Migration{
		{Version: 1, Name: "0001_ok.sql", SQL: "CREATE TABLE a (x INTEGER);"},
		{Version: 2, Name: "0002_bad.sql", SQL: "CREATE TABLE b (x INTEGER);\nTHIS IS NOT SQL;"},
	})
	if n != 1 || err == nil || !strings.Contains(err.Error(), "0002_bad.sql") {
		t.Fatalf("n=%d err=%v", n, err)
	}
	// The failed migration was rolled back.
	if _, err := d.Exec("SELECT * FROM b"); err == nil {
		t.Error("table b should not exist after rollback")
	}

	// Corrupt version column -> scan error.
	d2 := openMem(t)
	if _, err := d2.Exec(`CREATE TABLE schema_migrations (version TEXT, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d2.Exec(`INSERT INTO schema_migrations VALUES ('abc', 'x')`); err != nil {
		t.Fatal(err)
	}
	if _, err := d2.apply(ctx, nil); err == nil {
		t.Error("expected scan error")
	}

	// schema_migrations without a name column -> insert fails.
	d3 := openMem(t)
	if _, err := d3.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d3.apply(ctx, []Migration{{Version: 1, Name: "1_x.sql", SQL: "SELECT 1;"}}); err == nil {
		t.Error("expected insert error")
	}

	// Closed database.
	d4, err := Open(ctx, "sqlite", ":memory:", 0)
	if err != nil {
		t.Fatal(err)
	}
	d4.Close()
	if _, err := d4.Migrate(ctx); err == nil || !strings.Contains(err.Error(), "migrations table") {
		t.Errorf("closed db: %v", err)
	}
}

func TestIsUniqueViolationMessages(t *testing.T) {
	if IsUniqueViolation(nil) {
		t.Error("nil")
	}
	for _, m := range []string{
		"UNIQUE constraint failed: users.email",
		`ERROR: duplicate key value violates unique constraint "x"`,
		"something (SQLSTATE 23505)",
	} {
		if !IsUniqueViolation(errors.New(m)) {
			t.Errorf("%q not detected", m)
		}
	}
	if IsUniqueViolation(errors.New("other")) {
		t.Error("false positive")
	}
	if IsNoRows(nil) || IsNoRows(errors.New("x")) || !IsNoRows(fmt.Errorf("w: %w", sql.ErrNoRows)) {
		t.Error("IsNoRows")
	}
}

// TestPostgres runs the migrations against a real PostgreSQL server in a
// throw-away schema when QTF_TEST_POSTGRES_DSN is set.
func TestPostgres(t *testing.T) {
	dsn := os.Getenv("QTF_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("QTF_TEST_POSTGRES_DSN not set")
	}
	ctx := context.Background()
	admin, err := Open(ctx, "postgres", dsn, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	schema := "test_db_" + hex.EncodeToString(b)
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })

	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	d, err := Open(ctx, "postgresql", dsn+sep+"search_path="+url.QueryEscape(schema), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.Dialect != Postgres {
		t.Fatalf("dialect %s", d.Dialect)
	}
	n, err := d.Migrate(ctx)
	if err != nil || n == 0 {
		t.Fatalf("migrate n=%d err=%v", n, err)
	}
	if n, err := d.Migrate(ctx); err != nil || n != 0 {
		t.Fatalf("second migrate n=%d err=%v", n, err)
	}
	var got string
	if err := d.QueryRowContext(ctx, "SELECT current_schema()").Scan(&got); err != nil || got != schema {
		t.Fatalf("current_schema = %q (%v)", got, err)
	}
	checkUniqueAndNoRows(t, d)
}

func TestOpenSQLiteCreatesParentDir(t *testing.T) {
	base := t.TempDir()
	p := filepath.Join(base, "nested", "deeper", "q.db")
	d, err := Open(context.Background(), "sqlite", "file:"+p+"?_pragma=foreign_keys(1)", 1)
	if err != nil {
		t.Fatal(err)
	}
	d.Close()
	st, err := os.Stat(filepath.Dir(p))
	if err != nil || st.Mode().Perm() != 0o700 {
		t.Fatalf("dir: %v %v", st, err)
	}
	// a parent that is a regular file cannot be created
	blocker := filepath.Join(base, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), "sqlite", filepath.Join(blocker, "x", "q.db"), 1); err == nil {
		t.Fatal("expected error")
	}
	for _, dsn := range []string{":memory:", "file::memory:", "q.db", "/q.db"} {
		if err := ensureSQLiteDir(dsn); err != nil {
			t.Errorf("%s: %v", dsn, err)
		}
	}
}
