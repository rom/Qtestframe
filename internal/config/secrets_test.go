package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretsConfig(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	pin := writeFile(t, dir, "pin", "1234\n")
	p := writeFile(t, dir, "c.yaml", `
secrets:
  provider: pkcs11
  tpm: {device: /dev/tpm0, pcrs: [0, 7], pcr_bank: sha1}
  pkcs11:
    module: /usr/lib/softhsm/libsofthsm2.so
    token_label: qtf
    pin_file: `+pin+`
    key_label: kek
    generate_key: true
`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Secrets.Provider != SecretsPKCS11 || c.Secrets.PKCS11.PIN != "1234" || !c.Secrets.PKCS11.GenerateKey || c.Secrets.TPM.PCRs[1] != 7 {
		t.Fatalf("%+v", c.Secrets)
	}
	t.Setenv("QTF_SECRETS_PROVIDER", "local")
	t.Setenv("QTF_SECRETS_KEY", "env-key")
	t.Setenv("QTF_SECRETS_KEY_FILE", "/k")
	t.Setenv("QTF_SECRETS_TPM_DEVICE", "/dev/tpmrm1")
	t.Setenv("QTF_SECRETS_PKCS11_MODULE", "/m.so")
	t.Setenv("QTF_SECRETS_PKCS11_PIN", "9999")
	c, err = Load(p)
	if err != nil {
		t.Fatal(err)
	}
	s := c.Secrets
	if s.Provider != "local" || s.Local.Key != "env-key" || s.Local.KeyFile != "/k" || s.TPM.Device != "/dev/tpmrm1" || s.PKCS11.Module != "/m.so" || s.PKCS11.PIN != "9999" ||
		c.Meta.EnvKeys["secrets.local.key"] != "QTF_SECRETS_KEY" || c.Meta.EnvKeys["secrets.pkcs11.pin"] != "QTF_SECRETS_PKCS11_PIN" {
		t.Fatalf("%+v", s)
	}
	// the PIN is never written in the file
	if _, err := Load(writeFile(t, dir, "bad.yaml", "secrets: {pkcs11: {pin: x}}")); err == nil {
		t.Fatal("pin in the file accepted")
	}
	clearEnv(t)
	if _, err := Load(writeFile(t, dir, "nopin.yaml", "secrets: {pkcs11: {pin_file: "+filepath.Join(dir, "missing")+"}}")); err == nil {
		t.Fatal("missing pin file accepted")
	}
}

func TestSecretsValidate(t *testing.T) {
	for name, tc := range map[string]struct {
		s    Secrets
		want string
	}{
		"provider": {Secrets{Provider: "vault"}, "local, tpm or pkcs11"},
		"pkcs11":   {Secrets{Provider: SecretsPKCS11}, "module is required"},
		"token":    {Secrets{Provider: SecretsPKCS11, PKCS11: PKCS11Key{Module: "m", KeyLabel: "k"}}, "token_label or slot"},
		"label":    {Secrets{Provider: SecretsPKCS11, PKCS11: PKCS11Key{Module: "m", Slot: new(uint(0))}}, "key_label is required"},
		"bank":     {Secrets{TPM: TPMKey{PCRBank: "md5"}}, "pcr_bank"},
		"pcr":      {Secrets{TPM: TPMKey{PCRs: []int{24}}}, "not a PCR"},
	} {
		c := Default()
		c.Secrets = tc.s
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	c := Default()
	c.Secrets = Secrets{Provider: SecretsPKCS11, PKCS11: PKCS11Key{Module: "m", Slot: new(uint(0)), KeyLabel: "k"}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestKeyFilePath(t *testing.T) {
	t.Setenv("STATE_DIRECTORY", "")
	c := Default()
	for dsn, want := range map[string]string{
		"/var/lib/q/qtestframe.db":        "/var/lib/q/" + DefaultKeyFileName,
		"file:/data/q.db?_pragma=x":       "/data/" + DefaultKeyFileName,
		"q.db":                            DefaultKeyFileName,
		":memory:":                        "",
		"file:x?mode=memory&cache=shared": "",
	} {
		c.Database.DSN = dsn
		if got := c.KeyFilePath(); got != want {
			t.Errorf("%s: %q, want %q", dsn, got, want)
		}
	}
	c.Secrets.Local.KeyFile = "/etc/k"
	if c.KeyFilePath() != "/etc/k" {
		t.Fatal(c.KeyFilePath())
	}
	c.Secrets.Local.KeyFile = ""
	c.Database.Driver = "postgres"
	t.Setenv("STATE_DIRECTORY", "/var/lib/qtf:/other")
	if c.KeyFilePath() != "/var/lib/qtf/"+DefaultKeyFileName {
		t.Fatal(c.KeyFilePath())
	}
	t.Setenv("STATE_DIRECTORY", "")
	want := DefaultKeyFileName
	if fi, err := os.Stat("/var/lib/qtestframe"); err == nil && fi.IsDir() {
		want = "/var/lib/qtestframe/" + DefaultKeyFileName
	}
	if c.KeyFilePath() != want {
		t.Fatal(c.KeyFilePath())
	}
}
