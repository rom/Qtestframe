-- Two-way synchronisation of requirements with Jira and Azure DevOps (see
-- internal/api/reqsync.go and docs/user-guide "Two-way sync with Jira and
-- Azure DevOps").
--
-- external_refs (0035) keeps, per linked requirement, what both sides
-- looked like after the last sync: remote_version is the tracker's own
-- version of the item (the Jira "updated" time, the Azure DevOps revision),
-- local_hash and remote_hash the hashes of the mapped fields on each side,
-- sync_state the JSON {"l": {field: value}, "r": {field: value}} of the
-- mapped values themselves (the base of the three-way comparison and of the
-- conflict view's field diff). origin is 'qtestframe' for a tracker item
-- the sync created from a requirement, '' for one imported or pulled.
ALTER TABLE external_refs ADD COLUMN remote_version TEXT NOT NULL DEFAULT '';
ALTER TABLE external_refs ADD COLUMN local_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE external_refs ADD COLUMN remote_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE external_refs ADD COLUMN sync_state TEXT NOT NULL DEFAULT '';
ALTER TABLE external_refs ADD COLUMN origin TEXT NOT NULL DEFAULT '';
ALTER TABLE external_refs ADD COLUMN synced_at {{TS}};

-- The sync configuration of a project per source (system 'jira' or
-- 'azure-devops'): settings is the JSON of the field mapping with
-- directions and value maps, the scopes, the conflict policy, the impact
-- choice and the options (model.ReqSyncSettings). webhook_secret is the
-- shared secret of the webhook receiver (encrypted, write-only). run_as is
-- the user whose permissions the scheduled and webhook syncs use (the one
-- who saved the configuration). next_run_at is the next scheduled sync,
-- lease_until the end of a sync in progress (one server at a time), and
-- failures the consecutive failed runs (the backoff).
CREATE TABLE req_sync_configs (
    id               TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    system           TEXT NOT NULL,
    enabled          {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    settings         TEXT NOT NULL DEFAULT '{}',
    webhook_secret   TEXT NOT NULL DEFAULT '',
    run_as           TEXT NOT NULL DEFAULT '',
    interval_minutes INTEGER NOT NULL DEFAULT 0,
    next_run_at      {{TS}},
    lease_until      {{TS}},
    failures         INTEGER NOT NULL DEFAULT 0,
    last_run_at      {{TS}},
    last_status      TEXT NOT NULL DEFAULT '',
    last_error       TEXT NOT NULL DEFAULT '',
    version          INTEGER NOT NULL DEFAULT 1,
    updated_by       TEXT NOT NULL DEFAULT '',
    updated_at       {{TS}} NOT NULL
);
CREATE UNIQUE INDEX req_sync_configs_project ON req_sync_configs (project_id, system);
CREATE INDEX req_sync_configs_due ON req_sync_configs (enabled, next_run_at);

-- Items changed on both sides since the last sync under the "manual"
-- policy: fields is the JSON list [{field, base, local, remote}] of the
-- fields in conflict. One open conflict per requirement and system; a
-- later sync refreshes it. status open | resolved | dismissed.
CREATE TABLE req_sync_conflicts (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    system          TEXT NOT NULL,
    requirement_id  TEXT NOT NULL,
    external_id     TEXT NOT NULL,
    fields          TEXT NOT NULL DEFAULT '[]',
    local_version   INTEGER NOT NULL DEFAULT 0,
    remote_version  TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'open',
    resolution      TEXT NOT NULL DEFAULT '',
    resolved_by     TEXT NOT NULL DEFAULT '',
    resolved_at     {{TS}},
    created_at      {{TS}} NOT NULL,
    updated_at      {{TS}} NOT NULL
);
CREATE INDEX req_sync_conflicts_project ON req_sync_conflicts (project_id, status);

-- The sync log: one row per run (scheduled, manual, webhook, or a dry run
-- that changed nothing) with its counts (summary JSON), what it did per
-- item (entries JSON, at most 500) and the error that stopped it. The
-- newest 100 runs of a project and source are kept.
CREATE TABLE req_sync_runs (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    system       TEXT NOT NULL,
    trigger_kind TEXT NOT NULL,
    dry_run      {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    status       TEXT NOT NULL,
    summary      TEXT NOT NULL DEFAULT '{}',
    entries      TEXT NOT NULL DEFAULT '[]',
    error        TEXT NOT NULL DEFAULT '',
    started_by   TEXT NOT NULL DEFAULT '',
    started_at   {{TS}} NOT NULL,
    finished_at  {{TS}}
);
CREATE INDEX req_sync_runs_project ON req_sync_runs (project_id, system, started_at);
