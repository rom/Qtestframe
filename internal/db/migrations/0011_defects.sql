-- Defects with status and severity, linked to requirements, tests,
-- executions and runs, optionally synchronised with Jira or GitHub Issues.
ALTER TABLE projects ADD COLUMN defect_seq INTEGER NOT NULL DEFAULT 0;
-- Defect tracker of the project (JSON: type, url, project_key, repo, user,
-- token). The token is never returned by the API.
ALTER TABLE projects ADD COLUMN defect_tracker TEXT NOT NULL DEFAULT '{}';
CREATE TABLE defects (
    id               TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL,
    project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    seq              INTEGER NOT NULL,
    key              TEXT NOT NULL,
    title            TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'open',
    severity         TEXT NOT NULL DEFAULT 'major',
    priority         TEXT NOT NULL DEFAULT 'medium',
    assignee         TEXT NOT NULL DEFAULT '',
    external_system  TEXT NOT NULL DEFAULT '',
    external_key     TEXT NOT NULL DEFAULT '',
    external_url     TEXT NOT NULL DEFAULT '',
    external_status  TEXT NOT NULL DEFAULT '',
    synced_at        {{TS}},
    created_by       TEXT NOT NULL DEFAULT '',
    created_at       {{TS}} NOT NULL,
    updated_at       {{TS}} NOT NULL,
    resolved_at      {{TS}},
    UNIQUE (project_id, key)
);
CREATE INDEX defects_project ON defects(project_id, status);
CREATE TABLE defect_links (
    defect_id    TEXT NOT NULL REFERENCES defects(id) ON DELETE CASCADE,
    entity_type  TEXT NOT NULL,
    entity_id    TEXT NOT NULL,
    PRIMARY KEY (defect_id, entity_type, entity_id)
);
CREATE INDEX defect_links_entity ON defect_links(entity_type, entity_id);
