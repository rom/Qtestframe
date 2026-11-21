package secrets

import (
	"fmt"
	"os"

	"github.com/rom/qtestframe/internal/config"
)

// PKCS11Config selects the HSM token and key of the pkcs11 provider.
type PKCS11Config struct {
	Module      string
	TokenLabel  string
	Slot        *uint
	PIN         string
	KeyLabel    string
	GenerateKey bool
}

// Opened is a key provider opened from the configuration.
type Opened struct {
	Provider Provider
	// Notes are messages for the log (a key file was created, a key file
	// is readable by others, an ephemeral key is used).
	Notes []string
}

// Open returns the key provider configured in c.Secrets. The local
// provider creates its key file when it is missing and create is true
// (the server and the secrets commands do; read-only commands do not).
func Open(c *config.Config, create bool) (*Opened, error) {
	s := c.Secrets
	switch s.Provider {
	case "", config.SecretsLocal:
		if s.Local.Key != "" {
			l, err := NewLocalKey(s.Local.Key)
			if err != nil {
				return nil, fmt.Errorf("QTF_SECRETS_KEY: %w", err)
			}
			return &Opened{Provider: l}, nil
		}
		path := c.KeyFilePath()
		if path == "" {
			return &Opened{Provider: NewEphemeral(), Notes: []string{"in-memory database: stored secrets are encrypted with an ephemeral key"}}, nil
		}
		l, created, err := OpenKeyFile(path, create)
		if err != nil {
			return nil, err
		}
		o := &Opened{Provider: l}
		if created {
			o.Notes = append(o.Notes, "created the key file "+l.path+": keep it safe and back it up separately from the database; without it the stored secrets cannot be decrypted")
		}
		if fi, err := os.Stat(path); err == nil && fi.Mode().Perm()&0o077 != 0 {
			o.Notes = append(o.Notes, fmt.Sprintf("the key file %s is readable by group or others (mode %v); run chmod 600 %s", l.path, fi.Mode().Perm(), path))
		}
		return o, nil
	case config.SecretsTPM:
		p, err := NewTPM(s.TPM.Device, s.TPM.PCRs, s.TPM.PCRBank)
		if err != nil {
			return nil, err
		}
		return &Opened{Provider: p}, nil
	case config.SecretsPKCS11:
		p, err := NewPKCS11(PKCS11Config{Module: s.PKCS11.Module, TokenLabel: s.PKCS11.TokenLabel, Slot: s.PKCS11.Slot,
			PIN: s.PKCS11.PIN, KeyLabel: s.PKCS11.KeyLabel, GenerateKey: s.PKCS11.GenerateKey})
		if err != nil {
			return nil, err
		}
		return &Opened{Provider: p}, nil
	}
	return nil, fmt.Errorf("secrets: unknown provider %q (local, tpm or pkcs11)", s.Provider)
}
