package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupRestoreCommands(t *testing.T) {
	ctx := context.Background()
	srcCfg := writeConfig(t, "")
	src := openTestStore(t, srcCfg)
	ten, err := src.CreateTenant(ctx, "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SeedDemo(ctx, src, ten.ID); err != nil {
		t.Fatal(err)
	}
	src.DB.Close() // release the SQLite file

	out := filepath.Join(t.TempDir(), "b.zip")
	if r := run(t, "", "backup", "--config", srcCfg); r.code != 1 || !strings.Contains(r.stderr, "--out is required") {
		t.Fatal(r)
	}
	r := run(t, "", "backup", "--config", srcCfg, "--out", out)
	if r.code != 0 || !strings.Contains(r.stdout, "backup written to "+out) {
		t.Fatal(r)
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		t.Fatal(st, err)
	}
	if r := run(t, "", "backup", "--config", srcCfg, "--out", filepath.Join(t.TempDir(), "missing", "b.zip")); r.code != 1 {
		t.Fatal(r)
	}
	if r := run(t, "", "backup", "--config", filepath.Join(t.TempDir(), "nope.yaml"), "--out", out); r.code != 1 {
		t.Fatal(r)
	}
	if r := run(t, "", "backup", "--bogus"); r.code != 1 {
		t.Fatal(r)
	}

	dstCfg := writeConfig(t, "")
	if r := run(t, "", "restore", "--config", dstCfg, "--in", out); r.code != 1 || !strings.Contains(r.stderr, "--yes") {
		t.Fatal(r)
	}
	if r := run(t, "", "restore", "--config", dstCfg, "--yes"); r.code != 1 || !strings.Contains(r.stderr, "--in is required") {
		t.Fatal(r)
	}
	if r := run(t, "", "restore", "--config", dstCfg, "--yes", "--in", filepath.Join(t.TempDir(), "none.zip")); r.code != 1 {
		t.Fatal(r)
	}
	bad := filepath.Join(t.TempDir(), "bad.zip")
	os.WriteFile(bad, []byte("nope"), 0o600)
	if r := run(t, "", "restore", "--config", dstCfg, "--yes", "--in", bad); r.code != 1 || !strings.Contains(r.stderr, "ZIP") {
		t.Fatal(r)
	}
	if r := run(t, "", "restore", "--config", filepath.Join(t.TempDir(), "nope.yaml"), "--yes", "--in", out); r.code != 1 {
		t.Fatal(r)
	}
	if r := run(t, "", "restore", "--bogus"); r.code != 1 {
		t.Fatal(r)
	}
	r = run(t, "", "restore", "--config", dstCfg, "--in", out, "--yes")
	if r.code != 0 || !strings.Contains(r.stdout, "restored ") || !strings.Contains(r.stdout, "requirements") {
		t.Fatal(r)
	}
	dst := openTestStore(t, dstCfg)
	ps, err := dst.ListProjects(ctx, ten.ID)
	if err != nil || len(ps) != 1 || ps[0].Key != "DEMO" {
		t.Fatal(ps, err)
	}
	entries, err := dst.ListAudit(ctx, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Action == "admin.restore" && e.UserEmail == "cli" {
			found = true
		}
	}
	if !found {
		t.Fatal(entries)
	}
	if r := run(t, "", "help"); !strings.Contains(r.stdout, "qtestframe backup") || !strings.Contains(r.stdout, "qtestframe restore") {
		t.Fatal(r.stdout)
	}
}
