package secrets

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Local wraps data-encryption keys with a 256-bit key-encryption key held
// by the process: read from a key file or an environment variable.
type Local struct {
	kek    []byte
	source string // "file", "environment" or "ephemeral"
	path   string
}

// ParseKey decodes a 256-bit key written as base64 (standard or URL, with
// or without padding) or hex.
func ParseKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if b, err := hex.DecodeString(s); err == nil && len(b) == KeySize {
		return b, nil
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil && len(b) == KeySize {
			return b, nil
		}
	}
	return nil, fmt.Errorf("secrets: the key must be %d random bytes, base64 or hex encoded (e.g. the output of `openssl rand -base64 32`)", KeySize)
}

// FormatKey encodes a key as written to a key file.
func FormatKey(k []byte) string { return base64.StdEncoding.EncodeToString(k) + "\n" }

// NewLocalKey returns a local provider over a key given as base64 or hex
// (the QTF_SECRETS_KEY environment variable).
func NewLocalKey(encoded string) (*Local, error) {
	k, err := ParseKey(encoded)
	if err != nil {
		return nil, err
	}
	return &Local{kek: k, source: "environment"}, nil
}

// NewEphemeral returns a local provider with a random key that lives only
// in memory (in-memory databases and tests).
func NewEphemeral() *Local { return &Local{kek: NewKey(), source: "ephemeral"} }

// OpenKeyFile returns a local provider over the key in path. When the file
// does not exist and create is true, a new random key is written to it
// with mode 0600 (its directory is created with mode 0700); created
// reports that.
func OpenKeyFile(path string, create bool) (l *Local, created bool, err error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && create {
		if err := writeKeyFile(path, NewKey()); err != nil {
			return nil, false, err
		}
		created = true
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, false, fmt.Errorf("secrets: key file: %w", err)
	}
	k, err := ParseKey(string(b))
	if err != nil {
		return nil, false, fmt.Errorf("%w (file %s)", err, path)
	}
	abs, _ := filepath.Abs(path)
	return &Local{kek: k, source: "file", path: abs}, created, nil
}

// writeKeyFile writes a new key file without replacing an existing one.
func writeKeyFile(path string, k []byte) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("secrets: create the directory of the key file: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("secrets: create key file: %w", err)
	}
	if _, err := f.WriteString(FormatKey(k)); err != nil {
		f.Close()
		return fmt.Errorf("secrets: write key file: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("secrets: write key file: %w", err)
	}
	return f.Close()
}

// Name implements Provider.
func (l *Local) Name() string { return "local" }

// KeyID implements Provider: a fingerprint of the key (not the key).
func (l *Local) KeyID() string {
	h := sha256.Sum256(append([]byte("qtestframe key-encryption key id\x00"), l.kek...))
	return "local:" + hex.EncodeToString(h[:8])
}

func wrapAD(label string) []byte { return []byte("qtestframe/dek/v1|" + label) }

// Wrap implements Provider (AES-256-GCM, bound to the DEK's id).
func (l *Local) Wrap(dek []byte, label string) ([]byte, error) {
	return sealBytes(l.kek, dek, wrapAD(label))
}

// Unwrap implements Provider.
func (l *Local) Unwrap(wrapped []byte, label string) ([]byte, error) {
	return openBytes(l.kek, wrapped, wrapAD(label))
}

// Info implements Provider.
func (l *Local) Info() map[string]string {
	m := map[string]string{"key_source": l.source}
	if l.path != "" {
		m["key_file"] = l.path
	}
	return m
}

// Close implements Provider.
func (l *Local) Close() error { return nil }
