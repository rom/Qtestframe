-- Deleted requirements and tests, kept for 30 days so a deletion can be
-- undone. snapshot is JSON: every row the deletion removed (see
-- internal/store/trash.go).
CREATE TABLE trash (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    item_ids    TEXT NOT NULL DEFAULT '[]',
    keys        TEXT NOT NULL DEFAULT '[]',
    label       TEXT NOT NULL DEFAULT '',
    snapshot    TEXT NOT NULL,
    deleted_by  TEXT NOT NULL DEFAULT '',
    created_at  {{TS}} NOT NULL
);
CREATE INDEX trash_project ON trash(tenant_id, project_id, created_at);
