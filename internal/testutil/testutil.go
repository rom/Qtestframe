// Package testutil provides database fixtures for tests. Tests run on
// SQLite by default; set QTF_TEST_POSTGRES_DSN to also run them against
// PostgreSQL (each test gets its own schema).
package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/db"
)

// Driver returns the database driver used for tests.
func Driver() string {
	if os.Getenv("QTF_TEST_POSTGRES_DSN") != "" {
		return "postgres"
	}
	return "sqlite"
}

// OpenDB returns a migrated, isolated database closed at test end.
func OpenDB(t testing.TB) *db.DB {
	t.Helper()
	ctx := context.Background()
	dsn := os.Getenv("QTF_TEST_POSTGRES_DSN")
	if dsn == "" {
		d, err := db.Open(ctx, "sqlite", ":memory:", 1)
		if err != nil {
			t.Fatal(err)
		}
		migrate(t, d)
		t.Cleanup(func() { d.Close() })
		return d
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	schema := "t_" + hex.EncodeToString(b)
	admin, err := db.Open(ctx, "postgres", dsn, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	d, err := db.Open(ctx, "postgres", dsn+sep+"search_path="+schema, 5)
	if err != nil {
		t.Fatal(err)
	}
	migrate(t, d)
	t.Cleanup(func() {
		d.Close()
		_, _ = admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	return d
}

func migrate(t testing.TB, d *db.DB) {
	t.Helper()
	if _, err := d.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
}
