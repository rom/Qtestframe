package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/backup"
	"github.com/rom/qtestframe/internal/config"
)

func TestEncryptedBackupCommands(t *testing.T) {
	old := backup.KDFLogN
	backup.KDFLogN = 4
	defer func() { backup.KDFLogN = old }()
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
	src.DB.Close()

	dir := t.TempDir()
	passFile := filepath.Join(dir, "pass")
	os.WriteFile(passFile, []byte("  <a long test passphrase>\n"), 0o600)
	out := filepath.Join(dir, "b.zip.enc")
	r := run(t, "", "backup", "--config", srcCfg, "--out", out, "--encrypt", "--passphrase-file", passFile)
	if r.code != 0 || !strings.Contains(r.stdout, "encrypted") || !strings.Contains(r.stdout, "sha256 ") {
		t.Fatal(r)
	}
	if enc, err := backup.Sniff(out); err != nil || !enc {
		t.Fatal("not encrypted", err)
	}
	// verify: with the file, from standard input and from the environment
	r = run(t, "", "backup", "verify", "--passphrase-file", passFile, out)
	if r.code != 0 || !strings.Contains(r.stdout, "intact (encrypted") || !strings.Contains(r.stdout, "rows in") {
		t.Fatal(r)
	}
	if r := run(t, "<a long test passphrase>\n", "backup", "verify", out); r.code != 0 {
		t.Fatal(r)
	}
	t.Setenv(passphraseEnv, "<a long test passphrase>")
	if r := run(t, "", "backup", "verify", out); r.code != 0 {
		t.Fatal(r)
	}
	t.Setenv(passphraseEnv, "")
	if r := run(t, "wrong\n", "backup", "verify", out); r.code != 1 || !strings.Contains(r.stderr, "wrong passphrase") {
		t.Fatal(r)
	}
	if r := run(t, "\n", "backup", "verify", out); r.code != 1 || !strings.Contains(r.stderr, "empty") {
		t.Fatal(r)
	}
	if r := run(t, "", "backup", "verify", "--passphrase-file", filepath.Join(dir, "none"), out); r.code != 1 {
		t.Fatal(r)
	}
	if r := run(t, "", "backup", "verify"); r.code != 1 || !strings.Contains(r.stderr, "usage") {
		t.Fatal(r)
	}
	if r := run(t, "", "backup", "verify", filepath.Join(dir, "missing.zip")); r.code != 1 {
		t.Fatal(r)
	}
	if r := run(t, "", "backup", "verify", "--bogus"); r.code != 1 {
		t.Fatal(r)
	}
	if r := run(t, "", "backup", "verify", "-h"); r.code != 0 {
		t.Fatal(r)
	}
	bad := filepath.Join(dir, "bad.zip")
	os.WriteFile(bad, []byte("nope"), 0o600)
	if r := run(t, "", "backup", "verify", bad); r.code != 1 || !strings.Contains(r.stderr, "ZIP") {
		t.Fatal(r)
	}
	// an unencrypted backup verifies without a passphrase
	plain := filepath.Join(dir, "plain.zip")
	if r := run(t, "", "backup", "--config", srcCfg, "--out", plain); r.code != 0 {
		t.Fatal(r)
	}
	if r := run(t, "", "backup", "verify", plain); r.code != 0 || !strings.Contains(r.stdout, "not encrypted") {
		t.Fatal(r)
	}
	if r := run(t, "\n", "backup", "--config", srcCfg, "--out", out, "--encrypt"); r.code != 1 {
		t.Fatal(r)
	}

	// restore decrypts
	srcKey := filepath.Join(filepath.Dir(srcCfg), config.DefaultKeyFileName)
	dstCfg := writeConfig(t, fmt.Sprintf("secrets: {local: {key_file: %q}}", srcKey))
	if r := run(t, "wrong\n", "restore", "--config", dstCfg, "--in", out, "--yes"); r.code != 1 || !strings.Contains(r.stderr, "wrong passphrase") {
		t.Fatal(r)
	}
	r = run(t, "", "restore", "--config", dstCfg, "--in", out, "--yes", "--passphrase-file", passFile)
	if r.code != 0 || !strings.Contains(r.stdout, "restored ") {
		t.Fatal(r)
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".qtf-restore-") {
			t.Error("decrypted copy left behind:", e.Name())
		}
	}
	dst := openTestStore(t, dstCfg)
	if ps, err := dst.ListProjects(ctx, ten.ID); err != nil || len(ps) != 1 {
		t.Fatal(ps, err)
	}
	if r := run(t, "", "help"); !strings.Contains(r.stdout, "qtestframe backup verify") {
		t.Fatal(r.stdout)
	}
}

func TestHardeningBackupDirectory(t *testing.T) {
	cfg := config.Default()
	cfg.Backup.Directory = "/var/backups/qtestframe"
	o := hardeningOptions(cfg)
	found := false
	for _, p := range o.ReadWritePaths {
		if p == cfg.Backup.Directory {
			found = true
		}
	}
	if !found {
		t.Fatal(o.ReadWritePaths)
	}
}
