-- Envelope encryption of stored secrets (see internal/secrets and
-- docs/security.md). Secrets that must be read back (TOTP secrets, the SMTP
-- password and OIDC client secrets in system_settings, webhook secrets,
-- tracker tokens, secret web test variables) are stored as
-- enc:v1:<id>:<ciphertext>, encrypted with AES-256-GCM under a data-
-- encryption key (DEK). This table holds the DEKs, each only wrapped by the
-- key-encryption key of the configured key provider (local key file, TPM
-- 2.0 or HSM): wrapped is base64, kek_id identifies the wrapping key. The
-- active key encrypts new values; older keys stay until no value uses them.
CREATE TABLE secret_keys (
    id          TEXT PRIMARY KEY,
    provider    TEXT NOT NULL,
    kek_id      TEXT NOT NULL,
    wrapped     TEXT NOT NULL,
    active      {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    created_at  {{TS}} NOT NULL,
    updated_at  {{TS}} NOT NULL
);
