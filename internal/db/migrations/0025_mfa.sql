-- Multi-factor authentication (see docs/security.md, "Multi-factor
-- authentication"): TOTP enrolments, single-use recovery codes, pending
-- second-factor sign-ins, remembered devices and the MFA policies of the
-- server and of each tenant.

-- One TOTP authenticator per user. enabled_at is NULL while the enrolment
-- waits for its confirmation code. last_step is the RFC 6238 time step of
-- the last accepted code (a code is accepted once).
CREATE TABLE user_mfa (
    user_id      TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    totp_secret  TEXT NOT NULL,
    enabled_at   {{TS}},
    last_step    BIGINT NOT NULL DEFAULT 0,
    created_at   {{TS}} NOT NULL
);

-- Recovery codes (SHA-256 of the normalised code); used_at is set once used.
CREATE TABLE mfa_recovery_codes (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash   TEXT NOT NULL,
    used_at     {{TS}},
    created_at  {{TS}} NOT NULL
);
CREATE INDEX mfa_recovery_codes_user ON mfa_recovery_codes(user_id);

-- A sign-in that passed the first factor and waits for the second one. id
-- is the SHA-256 of the token in the qtf_mfa cookie.
CREATE TABLE mfa_challenges (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id   TEXT NOT NULL DEFAULT '',
    method      TEXT NOT NULL,
    details     TEXT NOT NULL DEFAULT '',
    attempts    INTEGER NOT NULL DEFAULT 0,
    created_at  {{TS}} NOT NULL,
    expires_at  {{TS}} NOT NULL
);
CREATE INDEX mfa_challenges_user ON mfa_challenges(user_id);

-- "Remember this device": id is the SHA-256 of the qtf_mfa_device cookie.
CREATE TABLE mfa_trusted_devices (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ip          TEXT NOT NULL DEFAULT '',
    user_agent  TEXT NOT NULL DEFAULT '',
    created_at  {{TS}} NOT NULL,
    expires_at  {{TS}} NOT NULL
);
CREATE INDEX mfa_trusted_devices_user ON mfa_trusted_devices(user_id);

-- MFA policies: scope '' is the whole server (super administrators), any
-- other scope a tenant id. mode: optional | all | roles.
CREATE TABLE mfa_policies (
    scope           TEXT PRIMARY KEY,
    mode            TEXT NOT NULL DEFAULT 'optional',
    roles           TEXT NOT NULL DEFAULT '[]',
    grace_days      INTEGER NOT NULL DEFAULT 0,
    remember_days   INTEGER NOT NULL DEFAULT 0,
    required_since  {{TS}},
    updated_by      TEXT NOT NULL DEFAULT '',
    updated_at      {{TS}} NOT NULL
);

-- A session that must enrol a second factor before anything else.
ALTER TABLE sessions ADD COLUMN mfa_enrol_required {{BOOL}} NOT NULL DEFAULT {{FALSE}};
