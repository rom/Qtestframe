package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/secrets"
)

func TestHardeningAllowsTheTPM(t *testing.T) {
	cfg := config.Default()
	cfg.Secrets.Provider = config.SecretsTPM
	if o := hardeningOptions(cfg); !slices.Contains(o.ReadWritePaths, "/dev/tpmrm0") {
		t.Fatal(o.ReadWritePaths)
	}
	cfg.Secrets.Provider = config.SecretsLocal
	if o := hardeningOptions(cfg); slices.Contains(o.ReadWritePaths, "/dev/tpmrm0") {
		t.Fatal(o.ReadWritePaths)
	}
}

func TestSecretsCommands(t *testing.T) {
	ctx := context.Background()
	cfgPath := writeConfig(t, "")
	dir := filepath.Dir(cfgPath)
	keyFile := filepath.Join(dir, config.DefaultKeyFileName)

	// status changes nothing: no key file yet
	r := run(t, "", "secrets", "status", "--config", cfgPath)
	if r.code != 0 || !strings.Contains(r.stdout, "provider:  local") || !strings.Contains(r.stdout, "not available") || !strings.Contains(r.stdout, "data-encryption keys: 0") {
		t.Fatal(r)
	}
	if _, err := os.Stat(keyFile); err == nil {
		t.Fatal("status created the key file")
	}
	// a secret written by an earlier version (plaintext)
	_, plain, err := openDB(ctx, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	ten, _ := plain.CreateTenant(ctx, "acme", "Acme")
	p := &model.Project{Key: "PR", Name: "P"}
	if err := plain.CreateProject(ctx, ten.ID, p, false); err != nil {
		t.Fatal(err)
	}
	w := &model.Webhook{ProjectID: p.ID, Name: "h", URL: "https://example.com/h", Secret: "whsec_cli_test", Events: []string{"*"}}
	if err := plain.CreateWebhook(ctx, ten.ID, w); err != nil {
		t.Fatal(err)
	}
	plain.DB.Close()
	if r := run(t, "", "secrets", "status", "--config", cfgPath); !strings.Contains(r.stdout, "0 secret(s) encrypted, 1 stored as plaintext") {
		t.Fatal(r)
	}
	// migrate creates the key file and encrypts the value
	r = run(t, "", "secrets", "migrate", "--config", cfgPath)
	if r.code != 0 || !strings.Contains(r.stdout, "1 secret(s) encrypted, 0 stored as plaintext") || !strings.Contains(r.stderr, "created the key file") || !strings.Contains(r.stderr, "encrypted 1 secret(s)") {
		t.Fatal(r)
	}
	if fi, err := os.Stat(keyFile); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatal(fi, err)
	}
	r = run(t, "", "secrets", "status", "--config", cfgPath)
	if r.code != 0 || !strings.Contains(r.stdout, "(active)") || !strings.Contains(r.stdout, "  ok") || !strings.Contains(r.stdout, "webhooks.secret") {
		t.Fatal(r)
	}
	// rotate, then prune the old key
	r = run(t, "", "secrets", "rotate", "--config", cfgPath)
	if r.code != 0 || !strings.Contains(r.stdout, "re-encrypted 1 secret(s)") || !strings.Contains(r.stdout, "old keys are kept") {
		t.Fatal(r)
	}
	r = run(t, "", "secrets", "rotate", "--prune", "--config", cfgPath)
	if r.code != 0 || !strings.Contains(r.stdout, "deleted the old key(s)") {
		t.Fatal(r)
	}
	// move to a new key file: the old configuration names the old key
	newKey := filepath.Join(t.TempDir(), "new.key")
	newCfg := writeConfig(t, fmt.Sprintf("database: {dsn: %q}\nsecrets: {local: {key_file: %q}}", filepath.Join(dir, "qtf.db"), newKey))
	if r := run(t, "", "secrets", "migrate", "--config", newCfg); r.code != 1 || !strings.Contains(r.stderr, "secrets rewrap") {
		t.Fatal(r)
	}
	if r := run(t, "", "secrets", "rewrap", "--config", newCfg, "--from-config", filepath.Join(dir, "missing.yaml")); r.code != 1 {
		t.Fatal(r)
	}
	r = run(t, "", "secrets", "rewrap", "--config", newCfg, "--from-config", cfgPath)
	if r.code != 0 || !strings.Contains(r.stdout, "re-wrapped 1 data-encryption key(s)") {
		t.Fatal(r)
	}
	if r := run(t, "", "secrets", "migrate", "--config", cfgPath); r.code != 1 || !strings.Contains(r.stderr, "wrapped by another key") {
		t.Fatal(r)
	}
	st := openTestStore(t, newCfg)
	got, err := st.GetWebhook(ctx, ten.ID, w.ID)
	if err != nil || got.Secret != "whsec_cli_test" {
		t.Fatal(got, err)
	}
	st.DB.Close()
	// rewrap with the same provider
	if r := run(t, "", "secrets", "rewrap", "--config", newCfg); r.code != 0 {
		t.Fatal(r)
	}
	// decrypt before a downgrade
	if r := run(t, "", "secrets", "decrypt", "--config", newCfg); r.code != 1 || !strings.Contains(r.stderr, "--yes") {
		t.Fatal(r)
	}
	if r := run(t, "", "secrets", "decrypt", "--yes", "--config", newCfg); r.code != 0 || !strings.Contains(r.stdout, "decrypted 1 secret(s)") {
		t.Fatal(r)
	}
	if r := run(t, "", "secrets", "status", "--config", newCfg); !strings.Contains(r.stdout, "1 stored as plaintext") {
		t.Fatal(r)
	}

	r = run(t, "", "secrets", "generate-key")
	if _, err := secrets.ParseKey(r.stdout); r.code != 0 || err != nil {
		t.Fatal(r, err)
	}
	for _, args := range [][]string{{"secrets"}, {"secrets", "nope"}, {"secrets", "status", "--bogus"}} {
		if r := run(t, "", args...); r.code == 0 {
			t.Fatal(args, r)
		}
	}
	if r := run(t, "", "secrets", "help"); r.code != 0 || !strings.Contains(r.stdout, "rewrap") {
		t.Fatal(r)
	}
	// a configuration whose key cannot be used
	bad := writeConfig(t, "secrets: {provider: tpm, tpm: {device: /nonexistent/tpm}}")
	for _, c := range []string{"migrate", "rotate", "rewrap", "decrypt"} {
		if r := run(t, "", "secrets", c, "--yes", "--config", bad); r.code != 1 {
			t.Fatal(c, r)
		}
	}
	if r := run(t, "", "secrets", "status", "--config", bad); r.code != 0 || !strings.Contains(r.stdout, "not available") {
		t.Fatal(r)
	}
}
