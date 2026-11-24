package config

import (
	"strings"
	"testing"
)

func TestBackupConfig(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	if c := Default(); c.Backup.Enabled || c.Backup.Schedule != "daily 02:00" || c.Backup.KeepLast != 7 || c.Backup.S3.Region != "us-east-1" {
		t.Fatalf("defaults: %+v", c.Backup)
	}
	pass := writeFile(t, dir, "pass", "<a backup passphrase>\n")
	key := writeFile(t, dir, "s3key", "<secret>\n")
	p := writeFile(t, dir, "c.yaml", `
backup:
  enabled: true
  schedule: every 6h
  directory: /var/backups/qtestframe
  keep_days: 30
  passphrase_file: `+pass+`
  s3:
    enabled: true
    endpoint: https://s3.example.com
    bucket: qtf
    secret_access_key_file: `+key+`
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backup.Passphrase != "<a backup passphrase>" || cfg.Backup.S3.SecretAccessKey != "<secret>" ||
		!cfg.Meta.FileKeys["backup.passphrase"] || !cfg.Meta.FileKeys["backup.s3.secret_access_key"] {
		t.Fatalf("%+v", cfg.Backup)
	}
	for body, want := range map[string]string{
		"backup:\n  schedule: sometimes\n":                             "backup.schedule",
		"backup:\n  directory: relative\n":                             "absolute path",
		"backup:\n  enabled: true\n":                                   "backup.directory is required",
		"backup:\n  keep_last: -1\n":                                   "keep_last",
		"backup:\n  keep_days: 5000\n":                                 "keep_days",
		"backup:\n  s3:\n    endpoint: ftp://x\n":                      "backup.s3.endpoint",
		"backup:\n  passphrase_file: /nonexistent/file\n":              "nonexistent",
		"backup:\n  s3:\n    secret_access_key_file: /nonexistent/f\n": "nonexistent",
	} {
		if _, err := Load(writeFile(t, dir, "bad.yaml", body)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v", body, err)
		}
	}
}
