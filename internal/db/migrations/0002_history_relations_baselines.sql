-- Revision history, requirement relations, baselines, comments, shared test
-- steps, execution defects, user preferences and session metadata.

ALTER TABLE users ADD COLUMN preferences TEXT NOT NULL DEFAULT '{}';
ALTER TABLE sessions ADD COLUMN ip TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN user_agent TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN last_seen {{TS}};
ALTER TABLE projects ADD COLUMN defect_url_template TEXT NOT NULL DEFAULT '';
ALTER TABLE test_cases ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE test_steps ADD COLUMN call_test_id TEXT REFERENCES test_cases(id) ON DELETE SET NULL;
ALTER TABLE test_executions ADD COLUMN defects TEXT NOT NULL DEFAULT '[]';

CREATE TABLE revisions (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    entity_type     TEXT NOT NULL,
    entity_id       TEXT NOT NULL,
    version         INTEGER NOT NULL,
    action          TEXT NOT NULL,
    changed_fields  TEXT NOT NULL DEFAULT '[]',
    snapshot        TEXT NOT NULL,
    user_email      TEXT NOT NULL DEFAULT '',
    created_at      {{TS}} NOT NULL
);
CREATE INDEX revisions_entity ON revisions(entity_type, entity_id, created_at);

CREATE TABLE requirement_relations (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    source_id   TEXT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    target_id   TEXT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    type        TEXT NOT NULL,
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  {{TS}} NOT NULL,
    UNIQUE (source_id, target_id, type)
);
CREATE INDEX relations_target ON requirement_relations(target_id);

CREATE TABLE baselines (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    created_by   TEXT NOT NULL DEFAULT '',
    created_at   {{TS}} NOT NULL,
    UNIQUE (project_id, name)
);

CREATE TABLE baseline_items (
    baseline_id  TEXT NOT NULL REFERENCES baselines(id) ON DELETE CASCADE,
    entity_type  TEXT NOT NULL,
    entity_id    TEXT NOT NULL,
    key          TEXT NOT NULL,
    title        TEXT NOT NULL,
    version      INTEGER NOT NULL,
    snapshot     TEXT NOT NULL,
    PRIMARY KEY (baseline_id, entity_type, entity_id)
);

CREATE TABLE comments (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    entity_type  TEXT NOT NULL,
    entity_id    TEXT NOT NULL,
    user_id      TEXT NOT NULL,
    body         TEXT NOT NULL,
    created_at   {{TS}} NOT NULL,
    updated_at   {{TS}} NOT NULL
);
CREATE INDEX comments_entity ON comments(entity_type, entity_id);
