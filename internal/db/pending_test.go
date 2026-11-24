package db

import (
	"context"
	"testing"
)

func TestPending(t *testing.T) {
	ctx := context.Background()
	d := openMem(t)
	all, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	if n, err := d.Pending(ctx); err != nil || n != len(all) {
		t.Fatalf("fresh database: %d %v", n, err)
	}
	if _, err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if n, err := d.Pending(ctx); err != nil || n != 0 {
		t.Fatalf("migrated: %d %v", n, err)
	}
	if _, err := d.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = ?`, all[len(all)-1].Version); err != nil {
		t.Fatal(err)
	}
	if n, err := d.Pending(ctx); err != nil || n != 1 {
		t.Fatalf("one missing: %d %v", n, err)
	}
	d.Close()
	if _, err := d.Pending(ctx); err == nil {
		t.Fatal("closed database: no error")
	}
}
