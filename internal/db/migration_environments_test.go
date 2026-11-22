package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// migrationDB runs fn on an empty SQLite database and (with
// QTF_TEST_POSTGRES_DSN) on an empty PostgreSQL schema.
func migrationDB(t *testing.T, fn func(t *testing.T, d *DB)) {
	t.Run("sqlite", func(t *testing.T) { fn(t, openMem(t)) })
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("QTF_TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("QTF_TEST_POSTGRES_DSN not set")
		}
		ctx := context.Background()
		admin, err := Open(ctx, "postgres", dsn, 2)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { admin.Close() })
		b := make([]byte, 6)
		_, _ = rand.Read(b)
		schema := "test_env_" + hex.EncodeToString(b)
		if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		d, err := Open(ctx, "postgres", dsn+sep+"search_path="+url.QueryEscape(schema), 0)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { d.Close() })
		fn(t, d)
	})
}

// Migration 0057 turns the distinct free-text environments of the runs and
// cycles of each project into environment items, links the runs, cycles
// and results to them and gives stored test lead roles environment.manage.
func TestMigrationEnvironments(t *testing.T) {
	migrationDB(t, func(t *testing.T, d *DB) {
		ctx := context.Background()
		ms, err := Migrations()
		if err != nil {
			t.Fatal(err)
		}
		var before []Migration
		for _, m := range ms {
			if m.Version < 57 {
				before = append(before, m)
			}
		}
		if _, err := d.apply(ctx, before); err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC()
		exec := func(q string, args ...any) {
			t.Helper()
			if _, err := d.ExecContext(ctx, d.Rebind(q), args...); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
		exec(`INSERT INTO tenants (id, slug, name, created_at) VALUES ('t1', 'acme', 'Acme', ?)`, now)
		for _, p := range []string{"p1", "p2"} {
			exec(`INSERT INTO projects (id, tenant_id, key, name, created_at, updated_at) VALUES (?, 't1', ?, ?, ?, ?)`, p, strings.ToUpper(p), p, now, now)
		}
		exec(`INSERT INTO test_cases (id, tenant_id, project_id, seq, key, title, created_at, updated_at) VALUES ('tc1', 't1', 'p1', 1, 'P1-TC-1', 'T', ?, ?)`, now, now)
		runs := []struct{ id, project, env string }{{"r1", "p1", "staging"}, {"r2", "p1", " Staging "}, {"r3", "p1", "ci"}, {"r4", "p2", "staging"}, {"r5", "p2", ""}}
		for _, r := range runs {
			exec(`INSERT INTO test_runs (id, tenant_id, project_id, name, environment, created_at) VALUES (?, 't1', ?, ?, ?, ?)`, r.id, r.project, r.id, r.env, now)
		}
		exec(`INSERT INTO test_cycles (id, tenant_id, project_id, name, environment, created_at, updated_at) VALUES ('c1', 't1', 'p2', 'C', 'lab', ?, ?)`, now, now)
		exec(`INSERT INTO test_executions (id, tenant_id, run_id, test_case_id, status) VALUES ('e1', 't1', 'r1', 'tc1', 'passed')`)
		exec(`INSERT INTO test_executions (id, tenant_id, run_id, test_case_id, status) VALUES ('e2', 't1', 'r1', 'tc1', 'not_run')`)
		exec(`INSERT INTO roles (tenant_id, name, label, description, permissions, created_at, updated_at) VALUES ('t1', 'test_lead', '', '', '["run.read","run.write"]', ?, ?)`, now, now)
		exec(`INSERT INTO roles (tenant_id, name, label, description, permissions, created_at, updated_at) VALUES ('t1', 'viewer', '', '', '["run.read"]', ?, ?)`, now, now)

		if _, err := d.apply(ctx, ms); err != nil {
			t.Fatal(err)
		}
		one := func(q string) string {
			t.Helper()
			var s string
			if err := d.QueryRowContext(ctx, q).Scan(&s); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
			return s
		}
		for q, want := range map[string]string{
			`SELECT COUNT(*) FROM environments`:                                                                               `4`,
			`SELECT COUNT(*) FROM environments WHERE project_id = 'p1'`:                                                       `2`,
			`SELECT name FROM environments WHERE project_id = 'p2' AND name = 'lab'`:                                          `lab`,
			`SELECT COUNT(*) FROM test_runs WHERE environment_id IS NULL`:                                                     `1`,
			`SELECT COUNT(DISTINCT environment_id) FROM test_runs WHERE id IN ('r1', 'r2')`:                                   `1`,
			`SELECT COUNT(*) FROM test_runs r JOIN environments e ON e.id = r.environment_id AND e.project_id = r.project_id`: `4`,
			`SELECT COUNT(*) FROM test_cycles WHERE environment_id IS NOT NULL`:                                               `1`,
			`SELECT environment_name FROM test_executions WHERE id = 'e1'`:                                                    `staging`,
			`SELECT COUNT(*) FROM test_executions WHERE id = 'e1' AND environment_id IS NOT NULL`:                             `1`,
			`SELECT environment_name FROM test_executions WHERE id = 'e2'`:                                                    ``,
			`SELECT permissions FROM roles WHERE name = 'test_lead'`:                                                          `["run.read","environment.manage","run.write"]`,
			`SELECT permissions FROM roles WHERE name = 'viewer'`:                                                             `["run.read"]`,
			`SELECT status FROM environments WHERE name = 'ci'`:                                                               `available`,
		} {
			if got := one(q); got != want {
				t.Errorf("%s = %q, want %q", q, got, want)
			}
		}
		// the new ids are UUIDs (project archives rewrite them)
		rows, err := d.QueryContext(ctx, `SELECT id FROM environments`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		uuid := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
		seen := map[string]bool{}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			if !uuid.MatchString(id) || seen[id] {
				t.Errorf("id %q", id)
			}
			seen[id] = true
		}
	})
}
