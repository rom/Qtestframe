package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/secrets"
)

// Encryption of stored secrets (see internal/secrets): every secret the
// server must read back is encrypted here, on its way into the database,
// and decrypted on its way out, so the rest of the code sees plaintext.
// The associated data of a value names its table, column and row (and,
// inside JSON, its field), so a value moved to another row does not
// decrypt. Values without the enc:v1: prefix are legacy plaintext; they
// are read as they are and encrypted in place by MigrateSecrets.
//
// A store without a keyring (SetupSecrets not called; some tests) stores
// plaintext, as before.

// associated data of the encrypted columns
func adTOTP(userID string) string { return "user_mfa.totp_secret|" + userID }
func adWebhook(id string) string  { return "webhooks.secret|" + id }
func adTracker(projectID string) string {
	return "projects.defect_tracker.token|" + projectID
}
func adWebVar(projectID, name string) string {
	return "projects.web_tests.variables." + strings.ToUpper(name) + ".value|" + projectID
}
func adSetting(key string) string { return "system_settings.value|" + key }
func adOIDCSecret(provider string) string {
	return "system_settings.value|auth.oidc." + provider + ".client_secret"
}

// SecretSettings are the system settings (table system_settings) whose
// values hold secrets: the value of a "string" setting is a JSON string;
// an "oidc" setting is a list of providers whose client_secret is secret.
var SecretSettings = map[string]string{"email.password": "string", "auth.oidc": "oidc", "similarity.embeddings.api_key": "string"}

// Keyring returns the keyring (nil when secrets are stored as plaintext).
func (s *Store) Keyring() *secrets.Keyring { return s.keys }

// seal encrypts a secret for storage (plaintext without a keyring).
func (s *Store) seal(v, ad string) (string, error) {
	if s.keys == nil || v == "" {
		return v, nil
	}
	return s.keys.Encrypt(v, ad)
}

// unseal decrypts a stored secret; legacy plaintext is returned as it is.
// A value of a key this process does not know yet (another process
// rotated the keys) reloads the keys from the database once.
func (s *Store) unseal(ctx context.Context, v, ad string) (string, error) {
	if !secrets.IsEncrypted(v) {
		return v, nil
	}
	if s.keys == nil {
		return "", secrets.ErrNoKeyring
	}
	out, err := s.keys.Decrypt(v, ad)
	if errors.Is(err, secrets.ErrUnknownKey) {
		if rerr := s.ReloadSecretKeys(ctx); rerr != nil {
			return "", fmt.Errorf("%w (reloading the keys: %v)", err, rerr)
		}
		out, err = s.keys.Decrypt(v, ad)
	}
	return out, err
}

// ---------------------------------------------------------------- key ring

// SecretKey is a stored data-encryption key (wrapped; never the key).
type SecretKey struct {
	ID        string    `json:"id"`
	Provider  string    `json:"provider"`
	KEKID     string    `json:"kek_id"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	wrapped   string
}

func listSecretKeys(ctx context.Context, c conn) ([]SecretKey, error) {
	rows, err := c.query(ctx, `SELECT id, provider, kek_id, wrapped, active, created_at, updated_at FROM secret_keys ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SecretKey
	for rows.Next() {
		var k SecretKey
		if err := rows.Scan(&k.ID, &k.Provider, &k.KEKID, &k.wrapped, &k.Active, &k.CreatedAt, &k.UpdatedAt); err != nil {
			return nil, err
		}
		k.CreatedAt, k.UpdatedAt = k.CreatedAt.UTC(), k.UpdatedAt.UTC()
		out = append(out, k)
	}
	return out, rows.Err()
}

// SecretKeys lists the stored data-encryption keys.
func (s *Store) SecretKeys(ctx context.Context) ([]SecretKey, error) {
	return listSecretKeys(ctx, s.c(ctx))
}

// ErrKeyMismatch: the stored keys are wrapped by another key-encryption key
// than the configured one.
var ErrKeyMismatch = errors.New("secrets: the stored data-encryption keys are wrapped by another key")

// addKey creates a new data-encryption key wrapped by p; active makes it
// the active key (and the others inactive).
func addKey(ctx context.Context, c conn, now time.Time, p secrets.Provider, active bool) (string, []byte, error) {
	id, dek := secrets.NewKeyID(), secrets.NewKey()
	w, err := p.Wrap(dek, id)
	if err != nil {
		return "", nil, err
	}
	if active {
		if _, err := c.exec(ctx, `UPDATE secret_keys SET active = ?, updated_at = ? WHERE active = ?`, false, now, true); err != nil {
			return "", nil, err
		}
	}
	_, err = c.exec(ctx, `INSERT INTO secret_keys (id, provider, kek_id, wrapped, active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, p.Name(), p.KeyID(), base64.StdEncoding.EncodeToString(w), active, now, now)
	return id, dek, err
}

// loadKeyring unwraps the stored keys with p; with create, a first key is
// created when there is none.
func loadKeyring(ctx context.Context, c conn, now time.Time, p secrets.Provider, create bool) (*secrets.Keyring, error) {
	keys, err := listSecretKeys(ctx, c)
	if err != nil {
		return nil, err
	}
	kr := secrets.NewKeyring(p)
	if len(keys) == 0 {
		if !create {
			return kr, nil
		}
		id, dek, err := addKey(ctx, c, now, p, true)
		if err != nil {
			return nil, fmt.Errorf("secrets: create the data-encryption key: %w", err)
		}
		return kr, kr.Add(id, dek, true)
	}
	if err := unwrapInto(kr, keys, p); err != nil {
		return nil, err
	}
	return kr, nil
}

// unwrapInto adds the keys, unwrapped by p, to kr. The newest active key
// is the active one.
func unwrapInto(kr *secrets.Keyring, keys []SecretKey, p secrets.Provider) error {
	var mismatch []string
	for _, k := range keys {
		if kr.Has(k.ID) {
			continue
		}
		if k.KEKID != p.KeyID() {
			mismatch = append(mismatch, fmt.Sprintf("%s (wrapped by %s)", k.ID, k.KEKID))
			continue
		}
		w, err := base64.StdEncoding.DecodeString(k.wrapped)
		if err != nil {
			return fmt.Errorf("secrets: data-encryption key %s is damaged: %w", k.ID, err)
		}
		dek, err := p.Unwrap(w, k.ID)
		if err != nil {
			return fmt.Errorf("secrets: unwrap the data-encryption key %s with %s: %w", k.ID, p.KeyID(), err)
		}
		if err := kr.Add(k.ID, dek, k.Active); err != nil {
			return err
		}
	}
	if len(mismatch) > 0 {
		return fmt.Errorf("%w: %s; the configured key is %s. Configure the key the data was encrypted with (the same key file or QTF_SECRETS_KEY, the same TPM, the same HSM key), "+
			"or re-wrap the keys with `qtestframe secrets rewrap --from-config OLD.yaml` (see docs/security.md)", ErrKeyMismatch, strings.Join(mismatch, ", "), p.KeyID())
	}
	if kr.Active() == "" && len(keys) > 0 {
		// no key is marked active (hand-edited data): use the newest
		k := keys[len(keys)-1]
		w, _ := base64.StdEncoding.DecodeString(k.wrapped)
		dek, err := p.Unwrap(w, k.ID)
		if err != nil {
			return err
		}
		return kr.Add(k.ID, dek, true)
	}
	return nil
}

// SetupSecrets makes the store encrypt secrets with keys protected by p:
// the stored data-encryption keys are unwrapped (a first key is created
// for a new database). It fails when the stored keys are wrapped by
// another key-encryption key.
func (s *Store) SetupSecrets(ctx context.Context, p secrets.Provider) error {
	var kr *secrets.Keyring
	err := s.tx(ctx, func(c conn) error {
		var err error
		kr, err = loadKeyring(ctx, c, s.now(), p, true)
		return err
	})
	if err != nil {
		// two processes created the first key at the same time: read theirs
		if kr2, err2 := loadKeyring(ctx, s.c(ctx), s.now(), p, false); err2 == nil && kr2.Active() != "" {
			s.useKeys(kr2)
			return nil
		}
		return err
	}
	s.useKeys(kr)
	return nil
}

// useKeys installs a key ring (replacing the keys of the current one, so
// stores sharing it — WithClock — see the change).
func (s *Store) useKeys(kr *secrets.Keyring) {
	if s.keys == nil {
		s.keys = kr
	} else {
		s.keys.Replace(kr)
	}
}

// ReloadSecretKeys reads keys created by another process (secrets rotate,
// another server instance) and adopts the active key of the database.
func (s *Store) ReloadSecretKeys(ctx context.Context) error {
	if s.keys == nil {
		return nil
	}
	keys, err := listSecretKeys(ctx, s.c(ctx))
	if err != nil {
		return err
	}
	p := s.keys.Provider()
	if err := unwrapInto(s.keys, keys, p); err != nil {
		return err
	}
	for _, k := range keys {
		if k.Active && s.keys.Has(k.ID) && s.keys.Active() != k.ID {
			w, _ := base64.StdEncoding.DecodeString(k.wrapped)
			dek, err := p.Unwrap(w, k.ID)
			if err != nil {
				return err
			}
			return s.keys.Add(k.ID, dek, true)
		}
	}
	return nil
}

// RewrapSecretKeys re-wraps every stored data-encryption key with to; the
// keys are unwrapped with from (the provider that wrapped them, which may
// be the same). The stored values are unchanged. The store uses to
// afterwards.
func (s *Store) RewrapSecretKeys(ctx context.Context, from, to secrets.Provider) (int, error) {
	n := 0
	var kr *secrets.Keyring
	err := s.tx(ctx, func(c conn) error {
		keys, err := listSecretKeys(ctx, c)
		if err != nil {
			return err
		}
		kr = secrets.NewKeyring(to)
		for _, k := range keys {
			w, err := base64.StdEncoding.DecodeString(k.wrapped)
			if err != nil {
				return fmt.Errorf("secrets: data-encryption key %s is damaged: %w", k.ID, err)
			}
			if k.KEKID != from.KeyID() {
				return fmt.Errorf("%w: key %s is wrapped by %s, not by %s (the --from-config key)", ErrKeyMismatch, k.ID, k.KEKID, from.KeyID())
			}
			dek, err := from.Unwrap(w, k.ID)
			if err != nil {
				return fmt.Errorf("secrets: unwrap the data-encryption key %s: %w", k.ID, err)
			}
			nw, err := to.Wrap(dek, k.ID)
			if err != nil {
				return fmt.Errorf("secrets: wrap the data-encryption key %s: %w", k.ID, err)
			}
			if _, err := c.exec(ctx, `UPDATE secret_keys SET provider = ?, kek_id = ?, wrapped = ?, updated_at = ? WHERE id = ?`,
				to.Name(), to.KeyID(), base64.StdEncoding.EncodeToString(nw), s.now(), k.ID); err != nil {
				return err
			}
			if err := kr.Add(k.ID, dek, k.Active); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	s.useKeys(kr)
	return n, nil
}

// ---------------------------------------------------------------- recoding

// SecretStats counts the stored secret values.
type SecretStats struct {
	// Plaintext values (not encrypted yet).
	Plaintext int `json:"plaintext"`
	// Encrypted values per data-encryption key.
	Encrypted map[string]int `json:"encrypted"`
	// Changed values (rewritten by MigrateSecrets, RotateSecrets or
	// DecryptSecrets).
	Changed int `json:"changed"`
	// Fields counts the values per field (e.g. "user_mfa.totp_secret").
	Fields map[string]int `json:"fields"`
}

func (st *SecretStats) count(field, v string) {
	if v == "" {
		return
	}
	if id, ok := secrets.KeyOf(v); ok {
		st.Encrypted[id]++
	} else {
		st.Plaintext++
	}
	st.Fields[field]++
}

// recodeFunc maps a stored value to the value to store (the same value
// for no change).
type recodeFunc func(ctx context.Context, stored, ad string) (string, error)

// recode applies fn to every stored secret; only changed rows are written
// (each only if it was not changed in the meantime). The stats describe
// the values after the change.
func (s *Store) recode(ctx context.Context, fn recodeFunc) (*SecretStats, error) {
	st := &SecretStats{Encrypted: map[string]int{}, Fields: map[string]int{}}
	c := s.c(ctx)
	apply := func(field, old, ad string) (string, error) {
		v, err := fn(ctx, old, ad)
		if err != nil {
			return "", fmt.Errorf("%s: %w", field, err)
		}
		st.count(field, v)
		return v, nil
	}
	// user_mfa.totp_secret, webhooks.secret, code_settings.webhook_secret
	for _, t := range []struct{ table, id, col, field string }{
		{"user_mfa", "user_id", "totp_secret", "user_mfa.totp_secret"},
		{"webhooks", "id", "secret", "webhooks.secret"},
		{"code_settings", "project_id", "webhook_secret", "code_settings.webhook_secret"},
	} {
		pairs, err := idValues(ctx, c, `SELECT `+t.id+`, `+t.col+` FROM `+t.table)
		if err != nil {
			return nil, err
		}
		for _, p := range pairs {
			ad := adTOTP(p[0])
			switch t.table {
			case "webhooks":
				ad = adWebhook(p[0])
			case "code_settings":
				ad = adCodeSecret(p[0])
			}
			v, err := apply(t.field, p[1], ad)
			if err != nil {
				return nil, fmt.Errorf("%w (row %s)", err, p[0])
			}
			if v != p[1] {
				res, err := c.exec(ctx, `UPDATE `+t.table+` SET `+t.col+` = ? WHERE `+t.id+` = ? AND `+t.col+` = ?`, v, p[0], p[1])
				if err != nil {
					return nil, err
				}
				if n, _ := res.RowsAffected(); n > 0 {
					st.Changed++
				}
			}
		}
	}
	// projects.defect_tracker, projects.web_tests
	for _, col := range []string{"defect_tracker", "web_tests"} {
		pairs, err := idValues(ctx, c, `SELECT id, `+col+` FROM projects`)
		if err != nil {
			return nil, err
		}
		for _, p := range pairs {
			v, changed, err := recodeProjectJSON(col, p[0], p[1], func(field, old, ad string) (string, error) { return apply(field, old, ad) })
			if err != nil {
				return nil, fmt.Errorf("%w (project %s)", err, p[0])
			}
			if changed {
				res, err := c.exec(ctx, `UPDATE projects SET `+col+` = ? WHERE id = ? AND `+col+` = ?`, v, p[0], p[1])
				if err != nil {
					return nil, err
				}
				if n, _ := res.RowsAffected(); n > 0 {
					st.Changed++
				}
			}
		}
	}
	// system_settings
	pairs, err := idValues(ctx, c, `SELECT key, value FROM system_settings`)
	if err != nil {
		return nil, err
	}
	for _, p := range pairs {
		if _, ok := SecretSettings[p[0]]; !ok {
			continue
		}
		v, err := transformSetting(p[0], json.RawMessage(p[1]), func(field, old, ad string) (string, error) {
			return apply("system_settings."+field, old, ad)
		})
		if err != nil {
			return nil, fmt.Errorf("%w (setting %s)", err, p[0])
		}
		if string(v) != p[1] {
			res, err := c.exec(ctx, `UPDATE system_settings SET value = ? WHERE key = ? AND value = ?`, string(v), p[0], p[1])
			if err != nil {
				return nil, err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				st.Changed++
			}
		}
	}
	return st, nil
}

func idValues(ctx context.Context, c conn, q string) ([][2]string, error) {
	rows, err := c.query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		out = append(out, [2]string{a, b})
	}
	return out, rows.Err()
}

// recodeProjectJSON applies fn to the secrets in a project's
// defect_tracker (token) or web_tests (values of secret variables) JSON,
// keeping every other field as it is.
func recodeProjectJSON(col, projectID, raw string, fn func(field, old, ad string) (string, error)) (string, bool, error) {
	var m map[string]any
	if raw == "" || json.Unmarshal([]byte(raw), &m) != nil || m == nil {
		return raw, false, nil
	}
	changed := false
	switch col {
	case "defect_tracker":
		tok, _ := m["token"].(string)
		if tok == "" {
			return raw, false, nil
		}
		v, err := fn("projects.defect_tracker.token", tok, adTracker(projectID))
		if err != nil {
			return "", false, err
		}
		if v != tok {
			m["token"], changed = v, true
		}
	case "web_tests":
		vars, _ := m["variables"].([]any)
		for _, x := range vars {
			vm, _ := x.(map[string]any)
			secret, _ := vm["secret"].(bool)
			val, _ := vm["value"].(string)
			name, _ := vm["name"].(string)
			if !secret || val == "" {
				continue
			}
			v, err := fn("projects.web_tests.variables", val, adWebVar(projectID, name))
			if err != nil {
				return "", false, err
			}
			if v != val {
				vm["value"], changed = v, true
			}
		}
	}
	if !changed {
		return raw, false, nil
	}
	b, err := json.Marshal(m)
	return string(b), true, err
}

// transformSetting applies fn to the secrets in the stored JSON value of a
// secret system setting (see SecretSettings).
func transformSetting(key string, raw json.RawMessage, fn func(field, old, ad string) (string, error)) (json.RawMessage, error) {
	switch SecretSettings[key] {
	case "string":
		var v string
		if json.Unmarshal(raw, &v) != nil || v == "" {
			return raw, nil
		}
		nv, err := fn(key, v, adSetting(key))
		if err != nil || nv == v {
			return raw, err
		}
		return json.Marshal(nv)
	case "oidc":
		var list []map[string]any
		if json.Unmarshal(raw, &list) != nil {
			return raw, nil
		}
		changed := false
		for _, p := range list {
			sec, _ := p["client_secret"].(string)
			name, _ := p["name"].(string)
			if sec == "" {
				continue
			}
			nv, err := fn(key+".client_secret", sec, adOIDCSecret(name))
			if err != nil {
				return nil, err
			}
			if nv != sec {
				p["client_secret"], changed = nv, true
			}
		}
		if !changed {
			return raw, nil
		}
		return json.Marshal(list)
	}
	return raw, nil
}

// SecretStatus counts the stored secret values without changing them.
func (s *Store) SecretStatus(ctx context.Context) (*SecretStats, error) {
	return s.recode(ctx, func(_ context.Context, v, _ string) (string, error) { return v, nil })
}

// MigrateSecrets encrypts the secrets still stored as plaintext (values
// stored before encryption was introduced, or restored from such a
// backup). It does nothing without a keyring.
func (s *Store) MigrateSecrets(ctx context.Context) (*SecretStats, error) {
	if s.keys == nil {
		return s.SecretStatus(ctx)
	}
	return s.recode(ctx, func(_ context.Context, v, ad string) (string, error) {
		if v == "" || secrets.IsEncrypted(v) {
			return v, nil
		}
		return s.seal(v, ad)
	})
}

// RotateSecrets creates a new data-encryption key, makes it the active key
// and re-encrypts every stored secret with it. With prune, older keys no
// value uses any more are deleted (only safe while no other server process
// still writes with an old key: stop them, or restart them first).
func (s *Store) RotateSecrets(ctx context.Context, prune bool) (string, *SecretStats, []string, error) {
	if s.keys == nil {
		return "", nil, nil, errors.New("secrets: no key provider is configured")
	}
	if err := s.ReloadSecretKeys(ctx); err != nil {
		return "", nil, nil, err
	}
	p := s.keys.Provider()
	var id string
	var dek []byte
	if err := s.tx(ctx, func(c conn) error {
		var err error
		id, dek, err = addKey(ctx, c, s.now(), p, true)
		return err
	}); err != nil {
		return "", nil, nil, err
	}
	if err := s.keys.Add(id, dek, true); err != nil {
		return "", nil, nil, err
	}
	st, err := s.recode(ctx, func(ctx context.Context, v, ad string) (string, error) {
		if v == "" {
			return v, nil
		}
		if k, ok := secrets.KeyOf(v); ok && k == id {
			return v, nil
		}
		plain, err := s.unseal(ctx, v, ad)
		if err != nil {
			return "", err
		}
		return s.seal(plain, ad)
	})
	if err != nil {
		return id, nil, nil, err
	}
	var pruned []string
	if prune {
		if pruned, err = s.pruneKeys(ctx, st); err != nil {
			return id, st, nil, err
		}
	}
	return id, st, pruned, nil
}

// pruneKeys deletes inactive keys that no value (counted in st) uses.
func (s *Store) pruneKeys(ctx context.Context, st *SecretStats) ([]string, error) {
	keys, err := s.SecretKeys(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, k := range keys {
		if k.Active || st.Encrypted[k.ID] > 0 {
			continue
		}
		if _, err := s.c(ctx).exec(ctx, `DELETE FROM secret_keys WHERE id = ? AND active = ?`, k.ID, false); err != nil {
			return out, err
		}
		out = append(out, k.ID)
	}
	sort.Strings(out)
	return out, nil
}

// DecryptSecrets writes every stored secret back as plaintext (before a
// downgrade to a version without encryption). The keys stay; the next
// start of this version encrypts the values again.
func (s *Store) DecryptSecrets(ctx context.Context) (*SecretStats, error) {
	return s.recode(ctx, func(ctx context.Context, v, ad string) (string, error) { return s.unseal(ctx, v, ad) })
}

// restoreKeys is called inside the restore transaction: the keys of the
// backup must be unwrappable with the configured provider (else the
// restore is refused, so nobody is locked out); a backup without keys
// gets a new one.
func (s *Store) restoreKeys(ctx context.Context, c conn) (*secrets.Keyring, error) {
	if s.keys == nil {
		return nil, nil
	}
	kr, err := loadKeyring(ctx, c, s.now(), s.keys.Provider(), true)
	if err != nil {
		return nil, Conflictf("the backup's encrypted secrets cannot be decrypted with this server's key: %v", err)
	}
	return kr, nil
}
