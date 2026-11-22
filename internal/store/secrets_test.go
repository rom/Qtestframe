package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/secrets"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/testutil"
)

// fixture writes one secret of every kind through the store.
type fixture struct {
	tenant, user, project, webhook string
}

const (
	totpValue    = "JBSWY3DPEHPK3PXP"
	hookValue    = "whsec_test_value"
	trackerValue = "tracker-test-value"
	varValue     = "variable-test-value"
	mailValue    = "smtp-test-value"
	oidcValue    = "oidc-test-value"
	jiraValue    = "jira-import-test-value"
	azureValue   = "azure-import-test-value"
)

func writeSecrets(t *testing.T, st *store.Store) fixture {
	t.Helper()
	ctx := context.Background()
	ten, err := st.CreateTenant(ctx, "acme", "Acme")
	must(t, err)
	u := &model.User{Email: "a@example.com"}
	must(t, st.CreateUser(ctx, u))
	p := &model.Project{Key: "PR", Name: "P"}
	must(t, st.CreateProject(ctx, ten.ID, p, false))
	must(t, st.StartTOTP(ctx, u.ID, totpValue))
	w := &model.Webhook{ProjectID: p.ID, Name: "hook", URL: "https://example.com/h", Secret: hookValue, Events: []string{"*"}, Enabled: true}
	must(t, st.CreateWebhook(ctx, ten.ID, w))
	must(t, st.SaveDefectTracker(ctx, ten.ID, p.ID, &model.DefectTracker{Type: "github", Repo: "o/r", Token: trackerValue}, false))
	must(t, st.SaveImportSources(ctx, ten.ID, p.ID, &model.ImportSources{Jira: model.JiraImportSource{URL: "https://jira.example.com", Token: jiraValue},
		Azure: model.AzureDevOpsSource{URL: "https://dev.azure.com/acme", Project: "Shop", Token: azureValue}}, store.ClearTokens{}))
	must(t, st.SaveWebTestSettings(ctx, ten.ID, p.ID, &model.WebTestSettings{Variables: []model.TestVariable{
		{Name: "PASSWORD", Value: varValue, Secret: true}, {Name: "USER", Value: "plain-user"},
	}}))
	oidc, _ := json.Marshal([]map[string]any{{"name": "corp", "issuer": "https://idp.example.com", "client_id": "c", "client_secret": oidcValue}})
	mail, _ := json.Marshal(mailValue)
	must(t, st.SaveSystemSettings(ctx, map[string]json.RawMessage{"email.password": mail, "auth.oidc": oidc, "logging.level": json.RawMessage(`"debug"`)}, "test"))
	return fixture{tenant: ten.ID, user: u.ID, project: p.ID, webhook: w.ID}
}

// raw returns the stored columns that hold the secrets, concatenated.
func raw(t *testing.T, st *store.Store) string {
	t.Helper()
	ctx := context.Background()
	var b strings.Builder
	for _, q := range []string{
		`SELECT totp_secret FROM user_mfa`, `SELECT secret FROM webhooks`, `SELECT defect_tracker FROM projects`,
		`SELECT web_tests FROM projects`, `SELECT value FROM system_settings`, `SELECT import_sources FROM projects`,
	} {
		rows, err := st.DB.QueryContext(ctx, q)
		must(t, err)
		for rows.Next() {
			var v string
			must(t, rows.Scan(&v))
			b.WriteString(v + "\n")
		}
		rows.Close()
	}
	return b.String()
}

var allValues = []string{totpValue, hookValue, trackerValue, varValue, mailValue, oidcValue, jiraValue, azureValue}

// changedRows is the number of rows a migration rewrites: the two import
// source tokens share a row.
var changedRows = len(allValues) - 1

// readSecrets checks that every secret reads back as plaintext.
func readSecrets(t *testing.T, st *store.Store, f fixture) {
	t.Helper()
	ctx := context.Background()
	m, err := st.GetUserMFA(ctx, f.user)
	if err != nil || m.Secret != totpValue {
		t.Fatalf("totp: %v %v", m, err)
	}
	w, err := st.GetWebhook(ctx, f.tenant, f.webhook)
	if err != nil || w.Secret != hookValue {
		t.Fatalf("webhook: %v %v", w, err)
	}
	ws, err := st.ListWebhooks(ctx, f.tenant, f.project)
	if err != nil || len(ws) != 1 || ws[0].Secret != hookValue {
		t.Fatalf("webhooks: %v %v", ws, err)
	}
	tr, err := st.DefectTracker(ctx, f.tenant, f.project)
	if err != nil || tr.Token != trackerValue || !tr.HasToken {
		t.Fatalf("tracker: %v %v", tr, err)
	}
	is, err := st.ImportSources(ctx, f.tenant, f.project)
	if err != nil || is.Jira.Token != jiraValue || is.Azure.Token != azureValue || !is.Azure.HasToken {
		t.Fatalf("import sources: %v %v", is, err)
	}
	wt, err := st.WebTestSettings(ctx, f.tenant, f.project)
	if err != nil || len(wt.Variables) != 2 || wt.Variables[0].Value != varValue || wt.Variables[1].Value != "plain-user" {
		t.Fatalf("web tests: %v %v", wt, err)
	}
	ss, err := st.SystemSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var mail string
	_ = json.Unmarshal(ss["email.password"].Value, &mail)
	var oidc []map[string]any
	_ = json.Unmarshal(ss["auth.oidc"].Value, &oidc)
	if mail != mailValue || len(oidc) != 1 || oidc[0]["client_secret"] != oidcValue || string(ss["logging.level"].Value) != `"debug"` {
		t.Fatalf("settings: %s %v", mail, oidc)
	}
}

func encryptedStore(t *testing.T, p secrets.Provider) *store.Store {
	t.Helper()
	st := store.New(testutil.OpenDB(t))
	must(t, st.SetupSecrets(context.Background(), p))
	return st
}

func TestSecretsEncryptedAtRest(t *testing.T) {
	ctx := context.Background()
	st := encryptedStore(t, secrets.NewEphemeral())
	f := writeSecrets(t, st)
	r := raw(t, st)
	for _, v := range allValues {
		if strings.Contains(r, v) {
			t.Fatalf("%s is stored as plaintext:\n%s", v, r)
		}
	}
	if !strings.Contains(r, "plain-user") || strings.Count(r, secrets.Prefix) != len(allValues) {
		t.Fatalf("stored:\n%s", r)
	}
	readSecrets(t, st, f)
	stats, err := st.SecretStatus(ctx)
	must(t, err)
	if stats.Plaintext != 0 || stats.Encrypted[st.Keyring().Active()] != len(allValues) || stats.Fields["user_mfa.totp_secret"] != 1 {
		t.Fatalf("%+v", stats)
	}
	keys, err := st.SecretKeys(ctx)
	if err != nil || len(keys) != 1 || !keys[0].Active || keys[0].Provider != "local" {
		t.Fatal(keys, err)
	}
	// write-only secrets are kept when not sent again
	w, _ := st.GetWebhook(ctx, f.tenant, f.webhook)
	w.Secret = ""
	must(t, st.UpdateWebhook(ctx, f.tenant, w))
	must(t, st.SaveDefectTracker(ctx, f.tenant, f.project, &model.DefectTracker{Type: "github", Repo: "o/r"}, false))
	must(t, st.SaveWebTestSettings(ctx, f.tenant, f.project, &model.WebTestSettings{Variables: []model.TestVariable{
		{Name: "PASSWORD", Secret: true}, {Name: "USER", Value: "plain-user"},
	}}))
	readSecrets(t, st, f)
	w.Secret = "whsec_new"
	must(t, st.UpdateWebhook(ctx, f.tenant, w))
	if w2, _ := st.GetWebhook(ctx, f.tenant, f.webhook); w2.Secret != "whsec_new" {
		t.Fatal(w2.Secret)
	}

	// a value copied into another row does not decrypt
	u2 := &model.User{Email: "b@example.com"}
	must(t, st.CreateUser(ctx, u2))
	must(t, st.StartTOTP(ctx, u2.ID, "OTHER"))
	_, err = st.DB.ExecContext(ctx, st.DB.Rebind(`UPDATE user_mfa SET totp_secret = (SELECT totp_secret FROM user_mfa WHERE user_id = ?) WHERE user_id = ?`), f.user, u2.ID)
	must(t, err)
	if _, err := st.GetUserMFA(ctx, u2.ID); !errors.Is(err, secrets.ErrDecrypt) {
		t.Fatalf("swapped value: %v", err)
	}
	// so does a value whose key is gone
	_, err = st.DB.ExecContext(ctx, st.DB.Rebind(`UPDATE webhooks SET secret = ? WHERE id = ?`), "enc:v1:knotthere:AAAA", f.webhook)
	must(t, err)
	if _, err := st.GetWebhook(ctx, f.tenant, f.webhook); !errors.Is(err, secrets.ErrUnknownKey) {
		t.Fatalf("unknown key: %v", err)
	}
	if _, err := st.ListWebhooks(ctx, f.tenant, f.project); err == nil {
		t.Fatal("list with an unknown key")
	}
	if _, _, _, err := st.RotateSecrets(ctx, false); err == nil {
		t.Fatal("rotated over an undecryptable value")
	}
}

func TestSecretsMigrateLegacyPlaintext(t *testing.T) {
	ctx := context.Background()
	d := testutil.OpenDB(t)
	old := store.New(d) // an earlier version: no encryption
	f := writeSecrets(t, old)
	r := raw(t, old)
	for _, v := range allValues {
		if !strings.Contains(r, v) {
			t.Fatalf("%s is not stored as plaintext", v)
		}
	}
	// the plaintext store reports the values; migrating does nothing
	if st, err := old.MigrateSecrets(ctx); err != nil || st.Plaintext != len(allValues) || st.Changed != 0 {
		t.Fatal(st, err)
	}
	// an encrypted value without a key ring is an error, not ciphertext
	st := store.New(d)
	must(t, st.SetupSecrets(ctx, secrets.NewEphemeral()))
	readSecrets(t, st, f) // legacy plaintext reads as it is
	stats, err := st.MigrateSecrets(ctx)
	must(t, err)
	if stats.Changed != changedRows || stats.Plaintext != 0 {
		t.Fatalf("%+v", stats)
	}
	r = raw(t, st)
	for _, v := range allValues {
		if strings.Contains(r, v) {
			t.Fatalf("%s still plaintext", v)
		}
	}
	readSecrets(t, st, f)
	if stats, _ := st.MigrateSecrets(ctx); stats.Changed != 0 {
		t.Fatal("migrated twice")
	}
	if _, err := old.GetUserMFA(ctx, f.user); !errors.Is(err, secrets.ErrNoKeyring) {
		t.Fatalf("no key ring: %v", err)
	}
	// decrypt (before a downgrade) writes plaintext back
	stats, err = st.DecryptSecrets(ctx)
	must(t, err)
	if stats.Changed != changedRows || stats.Plaintext != len(allValues) {
		t.Fatalf("%+v", stats)
	}
	readSecrets(t, old, f)
}

func TestSecretsRotateAndReload(t *testing.T) {
	ctx := context.Background()
	p := secrets.NewEphemeral()
	d := testutil.OpenDB(t)
	st := store.New(d)
	must(t, st.SetupSecrets(ctx, p))
	f := writeSecrets(t, st)
	first := st.Keyring().Active()
	// another server process on the same database
	other := store.New(d)
	must(t, other.SetupSecrets(ctx, p))
	if other.Keyring().Active() != first {
		t.Fatal("a second key was created")
	}
	id, stats, pruned, err := st.RotateSecrets(ctx, false)
	must(t, err)
	if id == first || stats.Changed != changedRows || stats.Encrypted[id] != len(allValues) || len(pruned) != 0 {
		t.Fatalf("%s %+v %v", id, stats, pruned)
	}
	readSecrets(t, st, f)
	// the other process learns the new key when it meets a value of it
	readSecrets(t, other, f)
	if other.Keyring().Active() != id {
		t.Fatal("the other process kept writing with the old key")
	}
	keys, _ := st.SecretKeys(ctx)
	if len(keys) != 2 {
		t.Fatal(keys)
	}
	_, _, pruned, err = st.RotateSecrets(ctx, true)
	must(t, err)
	if len(pruned) != 2 {
		t.Fatal(pruned)
	}
	if keys, _ := st.SecretKeys(ctx); len(keys) != 1 {
		t.Fatal(keys)
	}
	readSecrets(t, st, f)
	if _, _, _, err := store.New(d).RotateSecrets(ctx, false); err == nil {
		t.Fatal("rotated without a key ring")
	}
}

func TestSecretsRewrapAndMismatch(t *testing.T) {
	ctx := context.Background()
	a, b := secrets.NewEphemeral(), secrets.NewEphemeral()
	d := testutil.OpenDB(t)
	st := store.New(d)
	must(t, st.SetupSecrets(ctx, a))
	f := writeSecrets(t, st)
	_, _, _, err := st.RotateSecrets(ctx, false) // two keys
	must(t, err)
	// another key-encryption key does not open the stored keys
	err = store.New(d).SetupSecrets(ctx, b)
	if !errors.Is(err, store.ErrKeyMismatch) || !strings.Contains(err.Error(), "secrets rewrap") {
		t.Fatalf("mismatch: %v", err)
	}
	if _, err := store.New(d).RewrapSecretKeys(ctx, b, a); !errors.Is(err, store.ErrKeyMismatch) {
		t.Fatalf("rewrap from the wrong key: %v", err)
	}
	n, err := st.RewrapSecretKeys(ctx, a, b)
	if err != nil || n != 2 {
		t.Fatal(n, err)
	}
	readSecrets(t, st, f)
	if err := store.New(d).SetupSecrets(ctx, a); !errors.Is(err, store.ErrKeyMismatch) {
		t.Fatalf("old key after rewrap: %v", err)
	}
	nb := store.New(d)
	must(t, nb.SetupSecrets(ctx, b))
	readSecrets(t, nb, f)
	// a damaged wrapped key
	_, err = st.DB.ExecContext(ctx, `UPDATE secret_keys SET wrapped = 'AAAA'`)
	must(t, err)
	if err := store.New(d).SetupSecrets(ctx, b); err == nil {
		t.Fatal("damaged key unwrapped")
	}
	_, err = st.DB.ExecContext(ctx, `UPDATE secret_keys SET wrapped = '!'`)
	must(t, err)
	if err := store.New(d).SetupSecrets(ctx, b); err == nil {
		t.Fatal("damaged key decoded")
	}
	if _, err := store.New(d).RewrapSecretKeys(ctx, b, a); err == nil {
		t.Fatal("damaged key re-wrapped")
	}
}

func TestSecretsBackupRestore(t *testing.T) {
	ctx := context.Background()
	key := secrets.NewEphemeral()
	src := encryptedStore(t, key)
	f := writeSecrets(t, src)
	var buf bytes.Buffer
	_, err := src.Backup(ctx, &buf, "test")
	must(t, err)
	// the backup keeps the ciphertext
	for _, v := range allValues {
		if bytes.Contains(buf.Bytes(), []byte(v)) {
			t.Fatalf("backup contains %s", v)
		}
	}
	// a server with another key refuses the backup and keeps its data
	other := encryptedStore(t, secrets.NewEphemeral())
	g := writeSecrets(t, other)
	if _, err := other.Restore(ctx, bytes.NewReader(buf.Bytes()), int64(buf.Len())); !errors.Is(err, store.ErrConflict) || !strings.Contains(err.Error(), "cannot be decrypted") {
		t.Fatalf("restore with another key: %v", err)
	}
	readSecrets(t, other, g)
	// the same key restores
	dst := encryptedStore(t, key)
	_, err = dst.Restore(ctx, bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	must(t, err)
	readSecrets(t, dst, f)

	// a backup of plaintext secrets (no keys) is encrypted when restored
	plain := store.New(testutil.OpenDB(t))
	f = writeSecrets(t, plain)
	buf.Reset()
	_, err = plain.Backup(ctx, &buf, "test")
	must(t, err)
	dst = encryptedStore(t, secrets.NewEphemeral())
	_, err = dst.Restore(ctx, bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	must(t, err)
	readSecrets(t, dst, f)
	if r := raw(t, dst); strings.Contains(r, totpValue) {
		t.Fatal("restored plaintext not encrypted")
	}
}
