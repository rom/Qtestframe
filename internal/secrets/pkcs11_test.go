//go:build pkcs11 && cgo

package secrets

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/config"
)

// softHSM initialises a SoftHSM2 token in a temporary directory and
// returns its module path, token label and user PIN (generated here). The
// test is skipped when SoftHSM2 is not installed (apt-get install
// softhsm2); QTF_TEST_SOFTHSM=require fails it instead.
func softHSM(t *testing.T) (module, label, pin string) {
	t.Helper()
	util, err := exec.LookPath("softhsm2-util")
	module = os.Getenv("QTF_TEST_SOFTHSM_MODULE")
	if module == "" {
		module = "/usr/lib/softhsm/libsofthsm2.so"
	}
	if _, serr := os.Stat(module); err != nil || serr != nil {
		if os.Getenv("QTF_TEST_SOFTHSM") == "require" {
			t.Fatal("SoftHSM2 is not installed")
		}
		t.Skip("SoftHSM2 is not installed (apt-get install softhsm2)")
	}
	dir := t.TempDir()
	conf := filepath.Join(dir, "softhsm2.conf")
	if err := os.MkdirAll(filepath.Join(dir, "tokens"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conf, []byte("directories.tokendir = "+filepath.Join(dir, "tokens")+"\nobjectstore.backend = file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SOFTHSM2_CONF", conf)
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	pin = hex.EncodeToString(b)
	_, _ = rand.Read(b)
	label = "qtf-test"
	out, err := exec.Command(util, "--init-token", "--free", "--label", label, "--pin", pin, "--so-pin", hex.EncodeToString(b)).CombinedOutput()
	if err != nil {
		t.Fatalf("softhsm2-util: %v: %s", err, out)
	}
	return module, label, pin
}

func TestPKCS11WrapUnwrap(t *testing.T) {
	module, label, pin := softHSM(t)
	c := PKCS11Config{Module: module, TokenLabel: label, PIN: pin, KeyLabel: "qtf-kek"}
	if _, err := NewPKCS11(c); err == nil || !strings.Contains(err.Error(), "no AES key labelled") {
		t.Fatalf("missing key: %v", err)
	}
	c.GenerateKey = true
	p, err := NewPKCS11(c)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "pkcs11" || p.KeyID() != "pkcs11:"+label+"/qtf-kek" || p.Info()["token"] != label {
		t.Fatalf("provider: %s %s %v", p.Name(), p.KeyID(), p.Info())
	}
	dek := NewKey()
	w, err := p.Wrap(dek, "k1")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(w, dek) {
		t.Fatal("the wrapped key contains the key")
	}
	got, err := p.Unwrap(w, "k1")
	if err != nil || !bytes.Equal(got, dek) {
		t.Fatalf("unwrap: %v", err)
	}
	if _, err := p.Unwrap(w, "k2"); err == nil {
		t.Fatal("unwrapped with another label")
	}
	if _, err := p.Unwrap(w[:10], "k1"); err == nil {
		t.Fatal("damaged blob unwrapped")
	}
	p.Close()
	_ = p.Close()

	// a new session finds the generated key; a keyring round trip works
	c.GenerateKey = false
	cfg := config.Default()
	cfg.Secrets = config.Secrets{Provider: config.SecretsPKCS11, PKCS11: config.PKCS11Key{Module: module, TokenLabel: label, PIN: pin, KeyLabel: "qtf-kek"}}
	o, err := Open(&cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	got, err = o.Provider.Unwrap(w, "k1")
	if err != nil || !bytes.Equal(got, dek) {
		t.Fatalf("unwrap in a new session: %v", err)
	}
	o.Provider.Close()

	// wrong PIN, unknown token, unknown slot, bad module
	for name, bad := range map[string]PKCS11Config{
		"pin":    {Module: module, TokenLabel: label, PIN: "0000", KeyLabel: "qtf-kek"},
		"token":  {Module: module, TokenLabel: "nope", PIN: pin, KeyLabel: "qtf-kek"},
		"slot":   {Module: module, Slot: new(uint(4242)), PIN: pin, KeyLabel: "qtf-kek"},
		"module": {Module: filepath.Join(t.TempDir(), "missing.so"), TokenLabel: label, PIN: pin, KeyLabel: "qtf-kek"},
		"label":  {Module: module, TokenLabel: label, PIN: pin},
	} {
		if p, err := NewPKCS11(bad); err == nil {
			p.Close()
			t.Errorf("%s: accepted", name)
		}
	}
}
