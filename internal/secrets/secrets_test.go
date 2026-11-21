package secrets

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/config"
)

func keyring(t *testing.T) *Keyring {
	t.Helper()
	k := NewKeyring(NewEphemeral())
	if err := k.Add("k1", NewKey(), true); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestKeyringRoundTrip(t *testing.T) {
	k := keyring(t)
	enc, err := k.Encrypt("JBSWY3DPEHPK3PXP", "user_mfa.totp_secret|u1")
	if err != nil {
		t.Fatal(err)
	}
	if !IsEncrypted(enc) || !strings.HasPrefix(enc, "enc:v1:k1:") || strings.Contains(enc, "JBSWY3DP") {
		t.Fatalf("stored value %q", enc)
	}
	if id, ok := KeyOf(enc); !ok || id != "k1" {
		t.Fatalf("KeyOf %q %v", id, ok)
	}
	got, err := k.Decrypt(enc, "user_mfa.totp_secret|u1")
	if err != nil || got != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("decrypt %q %v", got, err)
	}
	// two encryptions differ (random nonce)
	enc2, _ := k.Encrypt("JBSWY3DPEHPK3PXP", "user_mfa.totp_secret|u1")
	if enc2 == enc {
		t.Fatal("nonce reused")
	}
	// bound to the row: another row's associated data does not decrypt
	if _, err := k.Decrypt(enc, "user_mfa.totp_secret|u2"); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("moved value: %v", err)
	}
	// empty stays empty; legacy plaintext is returned as it is
	if v, err := k.Encrypt("", "x"); v != "" || err != nil {
		t.Fatal(v, err)
	}
	if v, err := k.Decrypt("whsec_legacy", "x"); v != "whsec_legacy" || err != nil {
		t.Fatal(v, err)
	}
	// damaged values
	for _, bad := range []string{"enc:v1:", "enc:v1:k1", "enc:v1::abc", "enc:v1:k1:!!!", "enc:v1:k1:AAAA", enc[:len(enc)-2] + "AA"} {
		if _, err := k.Decrypt(bad, "user_mfa.totp_secret|u1"); err == nil {
			t.Errorf("%q decrypted", bad)
		}
	}
	if _, err := k.Decrypt("enc:v1:k9:AAAA", "x"); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("unknown key: %v", err)
	}
	var nilRing *Keyring
	if _, err := nilRing.Decrypt(enc, "x"); !errors.Is(err, ErrNoKeyring) {
		t.Fatalf("nil keyring: %v", err)
	}
	if v, err := nilRing.Decrypt("plain", "x"); v != "plain" || err != nil {
		t.Fatal(v, err)
	}
}

func TestKeyringKeys(t *testing.T) {
	k := NewKeyring(NewEphemeral())
	if _, err := k.Encrypt("x", "a"); err == nil {
		t.Fatal("encrypted without a key")
	}
	if err := k.Add("k1", []byte("short"), true); err == nil {
		t.Fatal("short key accepted")
	}
	for _, id := range []string{"", "a:b", "a|b"} {
		if err := k.Add(id, NewKey(), true); err == nil {
			t.Fatalf("id %q accepted", id)
		}
	}
	_ = k.Add("k1", NewKey(), true)
	enc, _ := k.Encrypt("v", "a")
	_ = k.Add("k2", NewKey(), true)
	if k.Active() != "k2" || !k.Has("k1") || len(k.Keys()) != 2 || k.Provider() == nil {
		t.Fatal(k.Active(), k.Keys())
	}
	// values of the older key still decrypt
	if v, err := k.Decrypt(enc, "a"); v != "v" || err != nil {
		t.Fatal(v, err)
	}
	enc2, _ := k.Encrypt("v", "a")
	if id, _ := KeyOf(enc2); id != "k2" {
		t.Fatal(enc2)
	}
	if _, ok := KeyOf("plain"); ok {
		t.Fatal("plain has a key")
	}
	if a, b := NewKeyID(), NewKeyID(); a == b || len(a) != 13 {
		t.Fatal(a, b)
	}
}

func TestParseKey(t *testing.T) {
	k := NewKey()
	for _, s := range []string{FormatKey(k), hex.EncodeToString(k), " " + strings.TrimSpace(FormatKey(k)) + " \n"} {
		got, err := ParseKey(s)
		if err != nil || !bytes.Equal(got, k) {
			t.Fatalf("%q: %v", s, err)
		}
	}
	for _, s := range []string{"", "short", hex.EncodeToString(k[:16])} {
		if _, err := ParseKey(s); err == nil {
			t.Fatalf("%q accepted", s)
		}
	}
}

func TestLocalProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "k.key")
	if _, _, err := OpenKeyFile(path, false); err == nil {
		t.Fatal("missing key file opened")
	}
	l, created, err := OpenKeyFile(path, true)
	if err != nil || !created {
		t.Fatal(err, created)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", fi.Mode(), err)
	}
	if d, _ := os.Stat(filepath.Dir(path)); d.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v", d.Mode())
	}
	l2, created, err := OpenKeyFile(path, true)
	if err != nil || created || l2.KeyID() != l.KeyID() {
		t.Fatal(err, created)
	}
	if l.Name() != "local" || !strings.HasPrefix(l.KeyID(), "local:") || l.Info()["key_source"] != "file" || l.Info()["key_file"] == "" || l.Close() != nil {
		t.Fatal(l.Info())
	}
	dek := NewKey()
	w, err := l.Wrap(dek, "k1")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := l2.Unwrap(w, "k1"); err != nil || !bytes.Equal(got, dek) {
		t.Fatal(err)
	}
	if _, err := l.Unwrap(w, "k2"); err == nil {
		t.Fatal("unwrapped with another label")
	}
	if _, err := NewEphemeral().Unwrap(w, "k1"); err == nil {
		t.Fatal("unwrapped with another key")
	}
	// the environment key
	e, err := NewLocalKey(FormatKey(NewKey()))
	if err != nil || e.Info()["key_source"] != "environment" {
		t.Fatal(err)
	}
	if _, err := NewLocalKey("nope"); err == nil {
		t.Fatal("bad env key accepted")
	}
	// a damaged key file, an unwritable location
	bad := filepath.Join(t.TempDir(), "bad.key")
	_ = os.WriteFile(bad, []byte("not a key"), 0o600)
	if _, _, err := OpenKeyFile(bad, true); err == nil || !strings.Contains(err.Error(), bad) {
		t.Fatal(err)
	}
	blocker := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(blocker, nil, 0o600)
	if _, _, err := OpenKeyFile(filepath.Join(blocker, "k.key"), true); err == nil {
		t.Fatal("created under a file")
	}
	if err := writeKeyFile(path, NewKey()); err == nil {
		t.Fatal("an existing key file was replaced")
	}
}

func TestOpenFromConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Database.DSN = filepath.Join(dir, "q.db")
	o, err := Open(&cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	if o.Provider.Name() != "local" || len(o.Notes) != 1 || !strings.Contains(o.Notes[0], "created the key file") {
		t.Fatal(o.Notes)
	}
	if _, err := os.Stat(filepath.Join(dir, config.DefaultKeyFileName)); err != nil {
		t.Fatal(err)
	}
	// a key file readable by others is reported
	_ = os.Chmod(filepath.Join(dir, config.DefaultKeyFileName), 0o644)
	if o, err := Open(&cfg, true); err != nil || len(o.Notes) != 1 || !strings.Contains(o.Notes[0], "chmod 600") {
		t.Fatal(err, o.Notes)
	}
	// QTF_SECRETS_KEY wins over the file
	cfg.Secrets.Local.Key = FormatKey(NewKey())
	if o, err := Open(&cfg, true); err != nil || o.Provider.Info()["key_source"] != "environment" {
		t.Fatal(err)
	}
	cfg.Secrets.Local.Key = "bad"
	if _, err := Open(&cfg, true); err == nil || !strings.Contains(err.Error(), "QTF_SECRETS_KEY") {
		t.Fatal(err)
	}
	cfg.Secrets.Local.Key = ""
	// in-memory databases get an ephemeral key
	cfg.Database.DSN = ":memory:"
	if o, err := Open(&cfg, true); err != nil || o.Provider.Info()["key_source"] != "ephemeral" {
		t.Fatal(err)
	}
	cfg.Database.DSN = filepath.Join(dir, "missing", "q.db")
	if _, err := Open(&cfg, false); err == nil {
		t.Fatal("missing key file opened without create")
	}
	cfg.Secrets.Provider = config.SecretsTPM
	cfg.Secrets.TPM.Device = filepath.Join(dir, "no-tpm")
	if _, err := Open(&cfg, true); err == nil || !strings.Contains(err.Error(), "open TPM") {
		t.Fatal(err)
	}
	cfg.Secrets.Provider = config.SecretsPKCS11
	if _, err := Open(&cfg, true); err == nil {
		t.Fatal("pkcs11 without a module")
	} else if !PKCS11Supported && !strings.Contains(err.Error(), "make go-pkcs11") {
		t.Fatal(err)
	}
	cfg.Secrets.Provider = "vault"
	if _, err := Open(&cfg, true); err == nil {
		t.Fatal("unknown provider")
	}
}
