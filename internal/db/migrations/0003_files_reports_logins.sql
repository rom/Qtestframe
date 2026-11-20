-- Files (attachments, documents, reports, avatars), report definitions,
-- login history, password-change enforcement and avatars/icons.

ALTER TABLE users ADD COLUMN avatar TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN must_change_password {{BOOL}} NOT NULL DEFAULT {{FALSE}};
ALTER TABLE projects ADD COLUMN icon TEXT NOT NULL DEFAULT '';

-- Metadata of every stored file. tenant_id is '' for global files (user
-- avatars, which are shared across tenants).
CREATE TABLE files (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL DEFAULT '',
    project_id    TEXT REFERENCES projects(id) ON DELETE CASCADE,
    kind          TEXT NOT NULL,            -- attachment | document | report | avatar | icon
    name          TEXT NOT NULL,
    content_type  TEXT NOT NULL,
    size          BIGINT NOT NULL,
    sha256        TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    tags          TEXT NOT NULL DEFAULT '[]',
    uploaded_by   TEXT NOT NULL DEFAULT '',
    created_at    {{TS}} NOT NULL
);
CREATE INDEX files_project ON files(project_id, kind);

-- File contents, kept apart so listings never load blobs.
CREATE TABLE file_blobs (
    file_id  TEXT PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
    data     {{BLOB}} NOT NULL
);

-- Links a file to a requirement, test, run or execution.
CREATE TABLE attachments (
    file_id      TEXT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    entity_type  TEXT NOT NULL,             -- requirement | test | run | execution
    entity_id    TEXT NOT NULL,
    PRIMARY KEY (file_id, entity_type, entity_id)
);
CREATE INDEX attachments_entity ON attachments(entity_type, entity_id);

CREATE TABLE report_definitions (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    config      TEXT NOT NULL,              -- JSON, see docs/api.md "Reports"
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  {{TS}} NOT NULL,
    updated_at  {{TS}} NOT NULL,
    UNIQUE (project_id, name)
);

CREATE TABLE login_events (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    success     {{BOOL}} NOT NULL,
    method      TEXT NOT NULL DEFAULT '',  -- local | oidc:<name> | saml:<name>
    ip          TEXT NOT NULL DEFAULT '',
    user_agent  TEXT NOT NULL DEFAULT '',
    created_at  {{TS}} NOT NULL
);
CREATE INDEX login_events_user ON login_events(user_id, created_at);
