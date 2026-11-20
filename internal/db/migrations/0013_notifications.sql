-- In-app notifications per user and outgoing webhooks per project.
CREATE TABLE notifications (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id   TEXT NOT NULL DEFAULT '',
    event        TEXT NOT NULL,
    title        TEXT NOT NULL,
    body         TEXT NOT NULL DEFAULT '',
    entity_type  TEXT NOT NULL DEFAULT '',
    entity_id    TEXT NOT NULL DEFAULT '',
    actor        TEXT NOT NULL DEFAULT '',
    read_at      {{TS}},
    created_at   {{TS}} NOT NULL
);
CREATE INDEX notifications_user ON notifications (tenant_id, user_id, created_at);
CREATE TABLE webhooks (
    id               TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL,
    project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    url              TEXT NOT NULL,
    secret           TEXT NOT NULL DEFAULT '',
    events           TEXT NOT NULL DEFAULT '[]',
    enabled          {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    created_by       TEXT NOT NULL DEFAULT '',
    created_at       {{TS}} NOT NULL,
    updated_at       {{TS}} NOT NULL,
    last_delivery_at {{TS}},
    last_status      INTEGER NOT NULL DEFAULT 0,
    last_error       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX webhooks_project ON webhooks (project_id);
