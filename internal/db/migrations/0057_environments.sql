-- Test environments and test data management (see docs/api.md
-- "Test environments" and "Test data sets").
--
-- environments: the systems tests run against (a dev or staging system, a
-- lab, a hardware-in-the-loop rig, a device). project_id NULL is an
-- environment shared by the organisation (every project can book it).
-- urls, components ([{name, version}]), tags, configuration_ids and
-- variant_ids are JSON lists. capacity: how many bookings may overlap.
-- credentials_ref: where the credentials are (a vault path, a note; never
-- the secret itself); secret: an optional write-only secret, encrypted like
-- the other stored secrets (enc:v1:..., bound to environments.secret and
-- the row) and revealed only to environment managers, with an audit entry.
-- health_url: an optional URL the background job checks every
-- health_interval_minutes; a failing check marks an available environment
-- broken (status_by_health), a passing one makes it available again.
CREATE TABLE environments (
    id                       TEXT PRIMARY KEY,
    tenant_id                TEXT NOT NULL,
    project_id               TEXT REFERENCES projects(id) ON DELETE CASCADE,
    name                     TEXT NOT NULL,
    type                     TEXT NOT NULL DEFAULT 'test',
    urls                     TEXT NOT NULL DEFAULT '[]',
    description              TEXT NOT NULL DEFAULT '',
    components               TEXT NOT NULL DEFAULT '[]',
    owner_id                 TEXT NOT NULL DEFAULT '',
    status                   TEXT NOT NULL DEFAULT 'available',
    status_note              TEXT NOT NULL DEFAULT '',
    status_by_health         {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    tags                     TEXT NOT NULL DEFAULT '[]',
    configuration_ids        TEXT NOT NULL DEFAULT '[]',
    variant_ids              TEXT NOT NULL DEFAULT '[]',
    capacity                 INTEGER NOT NULL DEFAULT 1,
    requires_approval        {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    credentials_ref          TEXT NOT NULL DEFAULT '',
    secret                   TEXT NOT NULL DEFAULT '',
    health_url               TEXT NOT NULL DEFAULT '',
    health_interval_minutes  INTEGER NOT NULL DEFAULT 5,
    health_status            TEXT NOT NULL DEFAULT '',
    health_checked_at        {{TS}},
    health_error             TEXT NOT NULL DEFAULT '',
    version                  INTEGER NOT NULL DEFAULT 1,
    created_by               TEXT NOT NULL DEFAULT '',
    created_at               {{TS}} NOT NULL,
    updated_at               {{TS}} NOT NULL
);
CREATE INDEX environments_project ON environments(tenant_id, project_id);

-- Bookings of an environment: who (user id and e-mail), from when to when,
-- for what, linked to a run, a cycle or an exploratory session of the
-- project it was booked for. status: pending (waits for approval) |
-- approved | rejected | cancelled; pending and approved bookings take up
-- the environment's capacity.
CREATE TABLE environment_bookings (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    environment_id  TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    project_id      TEXT REFERENCES projects(id) ON DELETE CASCADE,
    user_id         TEXT NOT NULL DEFAULT '',
    booked_by       TEXT NOT NULL DEFAULT '',
    starts_at       {{TS}} NOT NULL,
    ends_at         {{TS}} NOT NULL,
    purpose         TEXT NOT NULL DEFAULT '',
    run_id          TEXT REFERENCES test_runs(id) ON DELETE SET NULL,
    cycle_id        TEXT REFERENCES test_cycles(id) ON DELETE SET NULL,
    session_id      TEXT REFERENCES exploratory_sessions(id) ON DELETE SET NULL,
    status          TEXT NOT NULL DEFAULT 'approved',
    decided_by      TEXT NOT NULL DEFAULT '',
    decided_at      {{TS}},
    decision_note   TEXT NOT NULL DEFAULT '',
    created_at      {{TS}} NOT NULL,
    updated_at      {{TS}} NOT NULL
);
CREATE INDEX environment_bookings_env ON environment_bookings(environment_id, starts_at);
CREATE INDEX environment_bookings_status ON environment_bookings(tenant_id, status);

-- Runs, cycles and exploratory sessions select an environment item; the
-- free-text environment of runs and cycles stays (it holds the name).
ALTER TABLE test_runs ADD COLUMN environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL;
ALTER TABLE test_cycles ADD COLUMN environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL;
ALTER TABLE exploratory_sessions ADD COLUMN environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL;

-- A result records the environment it was obtained in and the versions of
-- its components at that moment (a JSON list like environments.components).
ALTER TABLE test_executions ADD COLUMN environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL;
ALTER TABLE test_executions ADD COLUMN environment_name TEXT NOT NULL DEFAULT '';
ALTER TABLE test_executions ADD COLUMN environment_components TEXT NOT NULL DEFAULT '[]';

-- The distinct free-text environments of the runs and cycles of each
-- project become environment items (type test, available), and the runs
-- and cycles point to them. Names are compared trimmed and ignoring case.
INSERT INTO environments (id, tenant_id, project_id, name, type, created_by, created_at, updated_at)
    SELECT {{UUID}}, v.tenant_id, v.project_id, MIN(v.name), 'test', 'migration', MIN(v.created_at), MIN(v.created_at)
    FROM (SELECT tenant_id, project_id, TRIM(environment) AS name, LOWER(TRIM(environment)) AS lname, created_at FROM test_runs WHERE TRIM(environment) <> ''
          UNION ALL
          SELECT tenant_id, project_id, TRIM(environment), LOWER(TRIM(environment)), created_at FROM test_cycles WHERE TRIM(environment) <> '') v
    GROUP BY v.tenant_id, v.project_id, v.lname;
UPDATE test_runs SET environment_id = (SELECT e.id FROM environments e WHERE e.project_id = test_runs.project_id AND LOWER(e.name) = LOWER(TRIM(test_runs.environment)))
    WHERE TRIM(environment) <> '';
UPDATE test_cycles SET environment_id = (SELECT e.id FROM environments e WHERE e.project_id = test_cycles.project_id AND LOWER(e.name) = LOWER(TRIM(test_cycles.environment)))
    WHERE TRIM(environment) <> '';
UPDATE test_executions SET environment_id = (SELECT r.environment_id FROM test_runs r WHERE r.id = test_executions.run_id),
    environment_name = COALESCE((SELECT TRIM(r.environment) FROM test_runs r WHERE r.id = test_executions.run_id), '')
    WHERE status IN ('passed', 'failed', 'blocked', 'skipped') AND run_id IN (SELECT id FROM test_runs WHERE environment_id IS NOT NULL);

-- Test data sets: named, versioned data (a table of parameters and rows, or
-- files attached to the set) with a classification (synthetic |
-- anonymised | production_copy), an owner, an expiry date and a status
-- (active | archived). version counts the changes; a run records the
-- version it used.
CREATE TABLE test_data_sets (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    kind            TEXT NOT NULL DEFAULT 'table',
    parameters      TEXT NOT NULL DEFAULT '[]',
    data_rows       TEXT NOT NULL DEFAULT '[]',
    classification  TEXT NOT NULL DEFAULT 'synthetic',
    owner_id        TEXT NOT NULL DEFAULT '',
    expires_on      TEXT NOT NULL DEFAULT '',
    tags            TEXT NOT NULL DEFAULT '[]',
    status          TEXT NOT NULL DEFAULT 'active',
    version         INTEGER NOT NULL DEFAULT 1,
    created_by      TEXT NOT NULL DEFAULT '',
    created_at      {{TS}} NOT NULL,
    updated_at      {{TS}} NOT NULL,
    UNIQUE (project_id, name)
);

-- Links of a data set to tests, configurations and runs. usage source: the
-- data-driven test takes its parameters and rows from the set instead of
-- its own (one source per test); reference: the item uses the set.
CREATE TABLE test_data_links (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    data_set_id  TEXT NOT NULL REFERENCES test_data_sets(id) ON DELETE CASCADE,
    item_type    TEXT NOT NULL,
    item_id      TEXT NOT NULL,
    usage        TEXT NOT NULL DEFAULT 'reference',
    created_by   TEXT NOT NULL DEFAULT '',
    created_at   {{TS}} NOT NULL,
    UNIQUE (data_set_id, item_type, item_id)
);
CREATE INDEX test_data_links_item ON test_data_links(item_type, item_id);

-- Usage: a run used a version of a set for a test (written when the test's
-- executions are created from the set).
CREATE TABLE test_data_usage (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    data_set_id   TEXT NOT NULL REFERENCES test_data_sets(id) ON DELETE CASCADE,
    data_version  INTEGER NOT NULL DEFAULT 1,
    run_id        TEXT REFERENCES test_runs(id) ON DELETE SET NULL,
    test_case_id  TEXT REFERENCES test_cases(id) ON DELETE SET NULL,
    used_by       TEXT NOT NULL DEFAULT '',
    used_at       {{TS}} NOT NULL
);
CREATE INDEX test_data_usage_set ON test_data_usage(data_set_id, used_at);

-- Refresh requests: someone asks the owner of a set to refresh it (new
-- data, new anonymisation, a later expiry). status: open | done | rejected.
CREATE TABLE test_data_refresh_requests (
    id               TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL,
    data_set_id      TEXT NOT NULL REFERENCES test_data_sets(id) ON DELETE CASCADE,
    requester_id     TEXT NOT NULL DEFAULT '',
    requested_by     TEXT NOT NULL DEFAULT '',
    reason           TEXT NOT NULL DEFAULT '',
    needed_by        TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'open',
    resolved_by      TEXT NOT NULL DEFAULT '',
    resolved_at      {{TS}},
    resolution_note  TEXT NOT NULL DEFAULT '',
    created_at       {{TS}} NOT NULL,
    updated_at       {{TS}} NOT NULL
);
CREATE INDEX test_data_refresh_set ON test_data_refresh_requests(data_set_id, status);

-- The new permission environment.manage (environments, their bookings'
-- approvals, the secret) for the test leads and managers whose
-- permissions an organisation changed (the defaults include it).
UPDATE roles SET permissions = REPLACE(permissions, '"run.write"', '"environment.manage","run.write"')
    WHERE name IN ('test_lead', 'manager') AND permissions LIKE '%"run.write"%' AND permissions NOT LIKE '%"environment.manage"%';
