package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// Migration 0052 (several roles per member) on a database with members,
// project members, invitations, API tokens and a custom role named sme
// from before it, on SQLite and (with QTF_TEST_POSTGRES_DSN) PostgreSQL.
func TestMigrationMultipleRoles(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) { checkMultipleRolesMigration(t, openMem(t)) })
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
		schema := "test_roles_" + hex.EncodeToString(b)
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
		checkMultipleRolesMigration(t, d)
	})
}

func checkMultipleRolesMigration(t *testing.T, d *DB) {
	t.Helper()
	ctx := context.Background()
	ms, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	var before []Migration
	for _, m := range ms {
		if m.Version < 52 {
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
	exec(`INSERT INTO users (id, email, created_at) VALUES ('u1', 'a@example.com', ?)`, now)
	exec(`INSERT INTO users (id, email, created_at) VALUES ('u2', 'b@example.com', ?)`, now)
	exec(`INSERT INTO memberships (tenant_id, user_id, role, created_at) VALUES ('t1', 'u1', 'admin', ?)`, now)
	exec(`INSERT INTO memberships (tenant_id, user_id, role, created_at) VALUES ('t1', 'u2', 'test_lead', ?)`, now)
	exec(`INSERT INTO projects (id, tenant_id, key, name, created_at, updated_at) VALUES ('p1', 't1', 'P', 'P', ?, ?)`, now, now)
	exec(`INSERT INTO project_members (tenant_id, project_id, member_type, member_id, role, created_at, updated_at) VALUES ('t1', 'p1', 'user', 'u2', 'reviewer', ?, ?)`, now, now)
	exec(`INSERT INTO invitations (id, tenant_id, user_id, email, role, created_at, sent_at, expires_at) VALUES ('i1', 't1', 'u2', 'b@example.com', 'tester', ?, ?, ?)`, now, now, now)
	exec(`INSERT INTO api_tokens (id, tenant_id, user_id, name, role, hash, prefix, created_at) VALUES ('k1', 't1', 'u2', 'ci', 'viewer', 'h', 'qtf_', ?)`, now)
	// an organisation's own role that happens to be called sme
	exec(`INSERT INTO roles (tenant_id, name, label, description, permissions, created_at, updated_at) VALUES ('t1', 'sme', 'Domain expert', 'Our experts', '["project.read"]', ?, ?)`, now, now)
	exec(`INSERT INTO roles (tenant_id, name, label, description, permissions, created_at, updated_at) VALUES ('t1', 'auditor', 'Auditor', '', '["audit.read"]', ?, ?)`, now, now)

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
		`SELECT roles FROM memberships WHERE user_id = 'u1'`:            `["admin"]`,
		`SELECT roles FROM memberships WHERE user_id = 'u2'`:            `["test_lead"]`,
		`SELECT role FROM memberships WHERE user_id = 'u2'`:             `test_lead`,
		`SELECT roles FROM project_members WHERE member_id = 'u2'`:      `["reviewer"]`,
		`SELECT roles FROM invitations WHERE id = 'i1'`:                 `["tester"]`,
		`SELECT roles FROM api_tokens WHERE id = 'k1'`:                  `["viewer"]`,
		`SELECT label FROM roles WHERE name = 'sme'`:                    ``,
		`SELECT description FROM roles WHERE name = 'sme'`:              `Domain expert: Our experts`,
		`SELECT permissions FROM roles WHERE name = 'sme'`:              `["project.read"]`,
		`SELECT label FROM roles WHERE name = 'auditor'`:                `Auditor`,
		`SELECT COUNT(*) FROM memberships WHERE roles LIKE '%"admin"%'`: `1`,
	} {
		if got := one(q); got != want {
			t.Errorf("%s = %q, want %q", q, got, want)
		}
	}
	// rows written later without roles keep the default
	exec(`INSERT INTO users (id, email, created_at) VALUES ('u3', 'c@example.com', ?)`, now)
	exec(`INSERT INTO memberships (tenant_id, user_id, role, created_at) VALUES ('t1', 'u3', 'viewer', ?)`, now)
	if got := one(`SELECT roles FROM memberships WHERE user_id = 'u3'`); got != `[]` {
		t.Errorf("default roles %q", got)
	}
}
