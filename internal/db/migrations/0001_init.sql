-- Portable schema. Tokens {{TS}}, {{BOOL}}, {{FALSE}}, {{TRUE}} are replaced per dialect.
CREATE TABLE tenants (
    id          TEXT PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    created_at  {{TS}} NOT NULL
);

CREATE TABLE users (
    id             TEXT PRIMARY KEY,
    email          TEXT NOT NULL UNIQUE,
    display_name   TEXT NOT NULL DEFAULT '',
    password_hash  TEXT NOT NULL DEFAULT '',
    super_admin    {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    auth_provider  TEXT NOT NULL DEFAULT 'local',
    external_id    TEXT NOT NULL DEFAULT '',
    disabled       {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    created_at     {{TS}} NOT NULL
);

CREATE TABLE memberships (
    tenant_id   TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        TEXT NOT NULL,
    created_at  {{TS}} NOT NULL,
    PRIMARY KEY (tenant_id, user_id)
);

CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id   TEXT NOT NULL DEFAULT '',
    csrf        TEXT NOT NULL,
    created_at  {{TS}} NOT NULL,
    expires_at  {{TS}} NOT NULL
);
CREATE INDEX sessions_user ON sessions(user_id);

CREATE TABLE api_tokens (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    role        TEXT NOT NULL,
    hash        TEXT NOT NULL UNIQUE,
    prefix      TEXT NOT NULL,
    created_at  {{TS}} NOT NULL,
    expires_at  {{TS}},
    last_used   {{TS}}
);

CREATE TABLE projects (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    key          TEXT NOT NULL,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    req_seq      INTEGER NOT NULL DEFAULT 0,
    test_seq     INTEGER NOT NULL DEFAULT 0,
    created_at   {{TS}} NOT NULL,
    updated_at   {{TS}} NOT NULL,
    UNIQUE (tenant_id, key)
);

CREATE TABLE folders (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    parent_id    TEXT REFERENCES folders(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    position     INTEGER NOT NULL DEFAULT 0,
    created_at   {{TS}} NOT NULL
);
CREATE INDEX folders_project ON folders(project_id, kind);

CREATE TABLE categories (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    scope        TEXT NOT NULL,
    dimension    TEXT NOT NULL,
    name         TEXT NOT NULL,
    color        TEXT NOT NULL DEFAULT '',
    description  TEXT NOT NULL DEFAULT '',
    UNIQUE (project_id, scope, dimension, name)
);

CREATE TABLE requirements (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    folder_id    TEXT REFERENCES folders(id) ON DELETE SET NULL,
    seq          INTEGER NOT NULL,
    key          TEXT NOT NULL,
    title        TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    obligation   TEXT NOT NULL DEFAULT '',
    type         TEXT NOT NULL DEFAULT 'functional',
    priority     TEXT NOT NULL DEFAULT 'medium',
    status       TEXT NOT NULL DEFAULT 'draft',
    source       TEXT NOT NULL DEFAULT '',
    rationale    TEXT NOT NULL DEFAULT '',
    owner        TEXT NOT NULL DEFAULT '',
    tags         TEXT NOT NULL DEFAULT '[]',
    version      INTEGER NOT NULL DEFAULT 1,
    position     INTEGER NOT NULL DEFAULT 0,
    created_by   TEXT NOT NULL DEFAULT '',
    created_at   {{TS}} NOT NULL,
    updated_at   {{TS}} NOT NULL,
    UNIQUE (project_id, key)
);
CREATE INDEX requirements_folder ON requirements(project_id, folder_id);

CREATE TABLE requirement_categories (
    requirement_id TEXT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    category_id    TEXT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    PRIMARY KEY (requirement_id, category_id)
);

CREATE TABLE evaluation_criteria (
    id              TEXT PRIMARY KEY,
    requirement_id  TEXT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    description     TEXT NOT NULL,
    method          TEXT NOT NULL DEFAULT 'test',
    threshold       TEXT NOT NULL DEFAULT '',
    position        INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX criteria_req ON evaluation_criteria(requirement_id);

CREATE TABLE test_cases (
    id               TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL,
    project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    folder_id        TEXT REFERENCES folders(id) ON DELETE SET NULL,
    seq              INTEGER NOT NULL,
    key              TEXT NOT NULL,
    title            TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    preconditions    TEXT NOT NULL DEFAULT '',
    execution_type   TEXT NOT NULL DEFAULT 'manual',
    script_language  TEXT NOT NULL DEFAULT '',
    script           TEXT NOT NULL DEFAULT '',
    expected_result  TEXT NOT NULL DEFAULT '',
    priority         TEXT NOT NULL DEFAULT 'medium',
    status           TEXT NOT NULL DEFAULT 'draft',
    estimated_min    INTEGER NOT NULL DEFAULT 0,
    tags             TEXT NOT NULL DEFAULT '[]',
    position         INTEGER NOT NULL DEFAULT 0,
    created_by       TEXT NOT NULL DEFAULT '',
    created_at       {{TS}} NOT NULL,
    updated_at       {{TS}} NOT NULL,
    UNIQUE (project_id, key)
);
CREATE INDEX tests_folder ON test_cases(project_id, folder_id);

CREATE TABLE test_steps (
    id            TEXT PRIMARY KEY,
    test_case_id  TEXT NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    position      INTEGER NOT NULL,
    action        TEXT NOT NULL DEFAULT '',
    expected      TEXT NOT NULL DEFAULT '',
    data          TEXT NOT NULL DEFAULT ''
);
CREATE INDEX steps_test ON test_steps(test_case_id);

CREATE TABLE test_case_categories (
    test_case_id  TEXT NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    category_id   TEXT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    PRIMARY KEY (test_case_id, category_id)
);

CREATE TABLE trace_links (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    requirement_id  TEXT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    test_case_id    TEXT NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    criterion_id    TEXT REFERENCES evaluation_criteria(id) ON DELETE SET NULL,
    created_at      {{TS}} NOT NULL,
    UNIQUE (requirement_id, test_case_id)
);

CREATE TABLE test_runs (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'planned',
    environment   TEXT NOT NULL DEFAULT '',
    build         TEXT NOT NULL DEFAULT '',
    created_by    TEXT NOT NULL DEFAULT '',
    created_at    {{TS}} NOT NULL,
    started_at    {{TS}},
    completed_at  {{TS}}
);
CREATE INDEX runs_project ON test_runs(project_id);

CREATE TABLE test_executions (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    run_id        TEXT NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
    test_case_id  TEXT NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    status        TEXT NOT NULL DEFAULT 'not_run',
    assignee      TEXT NOT NULL DEFAULT '',
    executed_by   TEXT NOT NULL DEFAULT '',
    started_at    {{TS}},
    finished_at   {{TS}},
    duration_ms   BIGINT NOT NULL DEFAULT 0,
    output        TEXT NOT NULL DEFAULT '',
    notes         TEXT NOT NULL DEFAULT '',
    position      INTEGER NOT NULL DEFAULT 0,
    UNIQUE (run_id, test_case_id)
);
CREATE INDEX executions_test ON test_executions(test_case_id);

CREATE TABLE step_results (
    id            TEXT PRIMARY KEY,
    execution_id  TEXT NOT NULL REFERENCES test_executions(id) ON DELETE CASCADE,
    step_id       TEXT NOT NULL DEFAULT '',
    position      INTEGER NOT NULL,
    action        TEXT NOT NULL DEFAULT '',
    expected      TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'not_run',
    actual        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX step_results_exec ON step_results(execution_id);

CREATE TABLE audit_log (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    user_id      TEXT NOT NULL DEFAULT '',
    user_email   TEXT NOT NULL DEFAULT '',
    action       TEXT NOT NULL,
    entity_type  TEXT NOT NULL,
    entity_id    TEXT NOT NULL DEFAULT '',
    details      TEXT NOT NULL DEFAULT '',
    created_at   {{TS}} NOT NULL
);
CREATE INDEX audit_tenant ON audit_log(tenant_id, created_at);
