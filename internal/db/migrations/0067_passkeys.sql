-- Passkeys (WebAuthn credentials; see docs/admin-guide "Authentication",
-- "Passkeys"): a passwordless sign-in and a second factor next to TOTP.

-- One row per registered passkey. credential_id is the base64url
-- credential id (unique across the server: it identifies the passkey of a
-- passwordless sign-in), public_key the base64url COSE key, alg its COSE
-- algorithm (-7 ES256, -8 EdDSA, -257 RS256). sign_count is the highest
-- signature counter seen (0 for passkeys without a counter). aaguid names
-- the authenticator model (hex; zeros when unknown), transports the JSON
-- list the browser reported, attestation the statement format (none or
-- packed). user_verified, backup_eligible and backed_up are the flags of
-- the registration (backed_up: of the last use).
CREATE TABLE webauthn_credentials (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id    TEXT NOT NULL UNIQUE,
    public_key       TEXT NOT NULL,
    alg              INTEGER NOT NULL,
    sign_count       BIGINT NOT NULL DEFAULT 0,
    aaguid           TEXT NOT NULL DEFAULT '',
    transports       TEXT NOT NULL DEFAULT '[]',
    attestation      TEXT NOT NULL DEFAULT 'none',
    name             TEXT NOT NULL,
    user_verified    {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    backup_eligible  {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    backed_up        {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    created_at       {{TS}} NOT NULL,
    last_used_at     {{TS}}
);
CREATE INDEX webauthn_credentials_user ON webauthn_credentials(user_id);

-- Challenges of WebAuthn ceremonies: single-use (deleted when taken) and
-- short-lived. id is the SHA-256 of the challenge. purpose: register,
-- login (passwordless), mfa (second factor of a pending sign-in) or reauth
-- (confirming a sensitive change). binding is the SHA-256 of what the
-- answer must come with: the session (register, reauth), the pending
-- sign-in (mfa) or the qtf_passkey cookie of the browser (login).
CREATE TABLE webauthn_challenges (
    id          TEXT PRIMARY KEY,
    purpose     TEXT NOT NULL,
    user_id     TEXT NOT NULL DEFAULT '',
    binding     TEXT NOT NULL DEFAULT '',
    user_verification TEXT NOT NULL DEFAULT 'preferred',
    created_at  {{TS}} NOT NULL,
    expires_at  {{TS}} NOT NULL
);
CREATE INDEX webauthn_challenges_expires ON webauthn_challenges(expires_at);

-- sessions.reauth_at: when the user last confirmed their identity in this
-- session (password, authenticator code or passkey) for changes that need
-- a recent sign-in, such as deleting a passkey (NULL = only the sign-in).
ALTER TABLE sessions ADD COLUMN reauth_at {{TS}};
