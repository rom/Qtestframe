package api

import (
	"context"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/settings"
	"github.com/rom/qtestframe/internal/store"
)

// Every system setting that holds a secret is encrypted by the store.
func TestSecretSettingsAreEncrypted(t *testing.T) {
	for _, d := range settings.Registry {
		want := map[settings.Kind]string{settings.Secret: "string", settings.OIDCList: "oidc"}[d.Kind]
		if want != "" && store.SecretSettings[d.Key] != want {
			t.Errorf("setting %s (%s) is not listed in store.SecretSettings", d.Key, d.Kind)
		}
	}
}

func TestSystemSettingsShowSecretsProvider(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	putSettings(a, map[string]any{"email.host": "smtp.example.test", "email.from": "qtf@example.test", "email.username": "u", "email.password": "pw-for-test"}, 200)
	var r struct {
		Secrets *secretsView `json:"secrets"`
	}
	a.call("GET", "/api/v1/admin/settings", nil, 200, &r)
	s := r.Secrets
	if s == nil || s.Provider != "local" || !strings.HasPrefix(s.KeyID, "local:") || s.Info["key_source"] != "ephemeral" ||
		s.Keys != 1 || s.ActiveKey == "" || s.Encrypted != 2 || s.Plaintext != 0 { // the password and the audit trail's checkpoint key
		t.Fatalf("secrets view: %+v", s)
	}
	// the password is stored encrypted and still used
	var raw string
	if err := e.st.DB.QueryRowContext(context.Background(), `SELECT value FROM system_settings WHERE key = 'email.password'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "pw-for-test") || !strings.Contains(raw, "enc:v1:") {
		t.Fatalf("stored %s", raw)
	}
	if e.srv.conf().Email.Password != "pw-for-test" {
		t.Fatal("password not applied")
	}
	// without a key ring (plaintext store) the view has no section
	plain := &Server{Store: store.New(e.st.DB)}
	if v, err := plain.secretsView(context.Background()); v != nil || err != nil {
		t.Fatal(v, err)
	}
}
