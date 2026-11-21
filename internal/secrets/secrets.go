// Package secrets encrypts the secrets Qtestframe must be able to read back
// (TOTP secrets, the SMTP password, OIDC client secrets, webhook signing
// secrets, tracker tokens, secret web test variables) before they are
// stored in the database.
//
// It uses envelope encryption: values are encrypted with AES-256-GCM under
// a random 256-bit data-encryption key (DEK). The DEK is stored in the
// database (table secret_keys) only in wrapped form: encrypted or sealed by
// a key-encryption key (KEK) that never leaves its key provider:
//
//   - local:  a 256-bit key in a file (created with mode 0600) or in the
//     QTF_SECRETS_KEY environment variable (the default);
//   - tpm:    the DEK is sealed to a TPM 2.0 (optionally to PCR values);
//   - pkcs11: the DEK is encrypted by an AES key held in an HSM (needs a
//     binary built with -tags pkcs11, see docs/operations.md).
//
// An encrypted value is a versioned string,
//
//	enc:v1:<dek id>:<base64url(nonce | ciphertext | tag)>
//
// whose associated data binds it to its table, column and row, so a value
// copied into another row (or another column) does not decrypt. Values
// without the prefix are legacy plaintext and are returned unchanged, so an
// upgrade never locks anyone out; they are encrypted in place at start-up.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Prefix starts every encrypted value.
const Prefix = "enc:v1:"

// KeySize is the size of data- and key-encryption keys in bytes.
const KeySize = 32

const nonceSize = 12

// Errors.
var (
	// ErrUnknownKey: a value names a data-encryption key the keyring does
	// not hold.
	ErrUnknownKey = errors.New("secrets: unknown data-encryption key")
	// ErrDecrypt: a value does not decrypt (wrong key, wrong row, damaged).
	ErrDecrypt = errors.New("secrets: value does not decrypt (wrong key, moved between rows or damaged)")
	// ErrNoKeyring: an encrypted value was read but encryption is not set up.
	ErrNoKeyring = errors.New("secrets: an encrypted value was read but no key provider is configured")
)

// Provider protects data-encryption keys with a key-encryption key.
type Provider interface {
	// Name is the provider type: local, tpm or pkcs11.
	Name() string
	// KeyID identifies the key-encryption key (stored next to each wrapped
	// DEK, so a mismatch is reported clearly).
	KeyID() string
	// Wrap protects a DEK; label is the DEK's id (bound to the result
	// where the mechanism supports associated data).
	Wrap(dek []byte, label string) ([]byte, error)
	// Unwrap reverses Wrap.
	Unwrap(wrapped []byte, label string) ([]byte, error)
	// Info describes the provider for the administration view (no secrets).
	Info() map[string]string
	// Close releases the provider (device, HSM session).
	Close() error
}

// NewKey returns a random 256-bit key.
func NewKey() []byte {
	k := make([]byte, KeySize)
	if _, err := rand.Read(k); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return k
}

// NewKeyID returns a new random data-encryption key id.
func NewKeyID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "k" + hex.EncodeToString(b)
}

// IsEncrypted reports whether a stored value is encrypted.
func IsEncrypted(v string) bool { return strings.HasPrefix(v, Prefix) }

// KeyOf returns the id of the data-encryption key of an encrypted value.
func KeyOf(v string) (string, bool) {
	if !IsEncrypted(v) {
		return "", false
	}
	id, _, ok := strings.Cut(v[len(Prefix):], ":")
	return id, ok && id != ""
}

func gcm(key []byte) (cipher.AEAD, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("secrets: key must be %d bytes, got %d", KeySize, len(key))
	}
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

// sealBytes encrypts plain with AES-256-GCM; the result is nonce|ct|tag.
func sealBytes(key, plain, ad []byte) ([]byte, error) {
	a, err := gcm(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return a.Seal(nonce, nonce, plain, ad), nil
}

// openBytes reverses sealBytes.
func openBytes(key, box, ad []byte) ([]byte, error) {
	a, err := gcm(key)
	if err != nil {
		return nil, err
	}
	if len(box) < nonceSize+a.Overhead() {
		return nil, ErrDecrypt
	}
	out, err := a.Open(nil, box[:nonceSize], box[nonceSize:], ad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return out, nil
}

func valueAD(keyID, ad string) []byte { return []byte("qtestframe/secret/v1|" + keyID + "|" + ad) }

// Keyring holds the unwrapped data-encryption keys in memory.
type Keyring struct {
	mu       sync.RWMutex
	provider Provider
	keys     map[string][]byte
	active   string
}

// NewKeyring returns an empty keyring of a provider.
func NewKeyring(p Provider) *Keyring {
	return &Keyring{provider: p, keys: map[string][]byte{}}
}

// Provider returns the key provider.
func (k *Keyring) Provider() Provider { return k.provider }

// Add adds an unwrapped data-encryption key; active makes it the key new
// values are encrypted with.
func (k *Keyring) Add(id string, dek []byte, active bool) error {
	if len(dek) != KeySize {
		return fmt.Errorf("secrets: data-encryption key %s has %d bytes, want %d", id, len(dek), KeySize)
	}
	if id == "" || strings.ContainsAny(id, ":|") {
		return fmt.Errorf("secrets: invalid key id %q", id)
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.keys[id] = append([]byte(nil), dek...)
	if active {
		k.active = id
	}
	return nil
}

// Has reports whether the keyring holds key id.
func (k *Keyring) Has(id string) bool {
	k.mu.RLock()
	defer k.mu.RUnlock()
	_, ok := k.keys[id]
	return ok
}

// Active returns the id of the key new values are encrypted with.
func (k *Keyring) Active() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.active
}

// Keys returns the ids of all keys.
func (k *Keyring) Keys() []string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	out := make([]string, 0, len(k.keys))
	for id := range k.keys {
		out = append(out, id)
	}
	return out
}

// Encrypt encrypts plain with the active key, bound to ad (e.g.
// "webhooks.secret|<id>"). An empty value stays empty.
func (k *Keyring) Encrypt(plain, ad string) (string, error) {
	if plain == "" {
		return "", nil
	}
	k.mu.RLock()
	id, key := k.active, k.keys[k.active]
	k.mu.RUnlock()
	if key == nil {
		return "", errors.New("secrets: no active data-encryption key")
	}
	box, err := sealBytes(key, []byte(plain), valueAD(id, ad))
	if err != nil {
		return "", err
	}
	return Prefix + id + ":" + base64.RawURLEncoding.EncodeToString(box), nil
}

// Decrypt returns the plaintext of a stored value. Values without the
// encryption prefix are legacy plaintext and are returned unchanged.
func (k *Keyring) Decrypt(stored, ad string) (string, error) {
	if !IsEncrypted(stored) {
		return stored, nil
	}
	if k == nil {
		return "", ErrNoKeyring
	}
	id, b64, ok := strings.Cut(stored[len(Prefix):], ":")
	if !ok || id == "" {
		return "", ErrDecrypt
	}
	k.mu.RLock()
	key := k.keys[id]
	k.mu.RUnlock()
	if key == nil {
		return "", fmt.Errorf("%w %s", ErrUnknownKey, id)
	}
	box, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return "", ErrDecrypt
	}
	out, err := openBytes(key, box, valueAD(id, ad))
	if err != nil {
		return "", err
	}
	return string(out), nil
}
