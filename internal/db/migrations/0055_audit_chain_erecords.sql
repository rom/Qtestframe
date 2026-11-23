-- Tamper-evident audit trail and electronic-records mode.
--
-- Hash chain: every audit entry of a tenant gets a sequence number (seq,
-- 1, 2, …), the hash of the entry before it (prev_hash) and its own hash
-- (SHA-256 over a canonical encoding of its fields and prev_hash; see
-- internal/store/auditchain.go). Entries are written with seq NULL and
-- chained right after their transaction commits, in a short transaction
-- that holds the tenant's row in audit_heads (so appends of one tenant are
-- serialised without holding a lock during the request). reason is the
-- reason for change given with the request (electronic-records mode).
ALTER TABLE audit_log ADD COLUMN seq BIGINT;
ALTER TABLE audit_log ADD COLUMN prev_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_log ADD COLUMN hash TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_log ADD COLUMN reason TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX audit_log_seq ON audit_log(tenant_id, seq);
CREATE INDEX audit_log_unsealed ON audit_log(tenant_id, created_at) WHERE seq IS NULL;

-- The end of each tenant's chain (tenant '' holds system entries such as
-- command line backups), and the last signed checkpoint.
CREATE TABLE audit_heads (
    tenant_id      TEXT PRIMARY KEY,
    seq            BIGINT NOT NULL DEFAULT 0,
    hash           TEXT NOT NULL DEFAULT '',
    checkpoint_seq BIGINT NOT NULL DEFAULT 0,
    checkpoint_at  {{TS}},
    updated_at     {{TS}} NOT NULL
);

-- Signed checkpoints: HMAC-SHA256 with a server key (audit_keys) over the
-- tenant, a sequence number and the hash of that entry, written every
-- 1000 entries, at least hourly while entries are written and at every
-- verification. They make a rewritten or truncated chain detectable by
-- someone who can change the database but not read the server's keys.
CREATE TABLE audit_checkpoints (
    id         TEXT PRIMARY KEY,
    tenant_id  TEXT NOT NULL,
    seq        BIGINT NOT NULL,
    hash       TEXT NOT NULL,
    key_id     TEXT NOT NULL,
    signature  TEXT NOT NULL,
    created_at {{TS}} NOT NULL
);
CREATE INDEX audit_checkpoints_tenant ON audit_checkpoints(tenant_id, seq);

-- Checkpoint signing keys: 256 random bits, encrypted like the other
-- stored secrets (see internal/store/secrets.go).
CREATE TABLE audit_keys (
    id         TEXT PRIMARY KEY,
    secret     TEXT NOT NULL,
    created_at {{TS}} NOT NULL
);

-- Tenants whose existing entries are chained by the first sealing after
-- this upgrade: their chain starts with a genesis entry that says so.
CREATE TABLE audit_backfill (
    tenant_id TEXT PRIMARY KEY
);
INSERT INTO audit_backfill (tenant_id) SELECT DISTINCT tenant_id FROM audit_log;

-- Electronic-records mode (21 CFR Part 11 / EU GMP Annex 11): the
-- organisation's settings (JSON, see model.ERecordsSettings).
CREATE TABLE erecords_settings (
    tenant_id  TEXT PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    settings   TEXT NOT NULL DEFAULT '{}',
    updated_by TEXT NOT NULL DEFAULT '',
    updated_at {{TS}} NOT NULL
);

-- Electronic signatures: who signed which record, when (UTC, and the
-- signer's time zone), with which meaning, and the hash of the record
-- version signed (a later change of the record shows the signature as no
-- longer matching). Kept when the project is deleted.
CREATE TABLE esignatures (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id   TEXT NOT NULL DEFAULT '',
    entity_type  TEXT NOT NULL,
    entity_id    TEXT NOT NULL,
    entity_key   TEXT NOT NULL DEFAULT '',
    action       TEXT NOT NULL,
    meaning      TEXT NOT NULL DEFAULT '',
    comment      TEXT NOT NULL DEFAULT '',
    user_id      TEXT NOT NULL DEFAULT '',
    user_email   TEXT NOT NULL DEFAULT '',
    user_name    TEXT NOT NULL DEFAULT '',
    method       TEXT NOT NULL DEFAULT '',
    time_zone    TEXT NOT NULL DEFAULT 'UTC',
    content_hash TEXT NOT NULL DEFAULT '',
    signed_at    {{TS}} NOT NULL
);
CREATE INDEX esignatures_entity ON esignatures(tenant_id, entity_type, entity_id);
CREATE INDEX esignatures_project ON esignatures(tenant_id, project_id, signed_at);

-- Single-use signing tickets: the proof of a re-authentication for one
-- action, given to the request that performs it (header
-- X-Signature-Ticket). SHA-256 of the ticket; minutes long.
CREATE TABLE esign_tickets (
    hash       TEXT PRIMARY KEY,
    tenant_id  TEXT NOT NULL,
    user_id    TEXT NOT NULL,
    session_id TEXT NOT NULL,
    action     TEXT NOT NULL,
    meaning    TEXT NOT NULL,
    comment    TEXT NOT NULL DEFAULT '',
    method     TEXT NOT NULL DEFAULT '',
    time_zone  TEXT NOT NULL DEFAULT 'UTC',
    expires_at {{TS}} NOT NULL
);
