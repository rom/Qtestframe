-- Scheduled backups (Administration → Backup; system settings section
-- backup, see internal/backup and docs/admin-guide backup-and-migration).
--
-- backup_runs: one row per backup the server took (on its schedule or with
-- "Run backup now"): when it started and finished, the file in the backup
-- directory with its size and SHA-256, whether it is encrypted, the object
-- key of the upload to S3-compatible storage, the outcome and when the
-- retention rules deleted the file. A log, like req_sync_runs: not part of
-- a backup.
CREATE TABLE backup_runs (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    started_by TEXT NOT NULL DEFAULT '',
    instance TEXT NOT NULL DEFAULT '',
    started_at {{TS}} NOT NULL,
    finished_at {{TS}},
    file_name TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    sha256 TEXT NOT NULL DEFAULT '',
    encrypted {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    remote_key TEXT NOT NULL DEFAULT '',
    row_count BIGINT NOT NULL DEFAULT 0,
    schema_version INTEGER NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    pruned_at {{TS}},
    verified_at {{TS}},
    verify_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX backup_runs_started ON backup_runs (started_at);

-- backup_locks: the lease of the server instance taking a backup, so that
-- with several servers on one PostgreSQL database only one backs up at a
-- time (holder: the instance; lease_until: renewed while it runs, so the
-- lock of a crashed server expires).
CREATE TABLE backup_locks (
    name TEXT PRIMARY KEY,
    holder TEXT NOT NULL DEFAULT '',
    lease_until {{TS}}
);
INSERT INTO backup_locks (name, holder) VALUES ('backup', '');
