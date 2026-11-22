-- Test cycles: a planned group of test executions for a goal ("Sprint 14
-- regression"), optionally for a release, variant and build, with start
-- and end dates, a status (planned | active | closed) and an owner (user
-- id). The cycle creates its runs (test_runs.cycle_id) and tracks their
-- results. scheduled_start_at: a one-off start (planned → active, runs
-- created) by the background jobs; schedule_id: the recurring schedule
-- that created the cycle.
CREATE TABLE test_cycles (
    id                  TEXT PRIMARY KEY,
    tenant_id           TEXT NOT NULL,
    project_id          TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    goal                TEXT NOT NULL DEFAULT '',
    description         TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL DEFAULT 'planned',
    owner_id            TEXT NOT NULL DEFAULT '',
    release_id          TEXT REFERENCES releases(id) ON DELETE SET NULL,
    variant_id          TEXT REFERENCES variants(id) ON DELETE SET NULL,
    build_id            TEXT REFERENCES builds(id) ON DELETE SET NULL,
    environment         TEXT NOT NULL DEFAULT '',
    configuration_ids   TEXT NOT NULL DEFAULT '[]',
    start_date          TEXT NOT NULL DEFAULT '',
    end_date            TEXT NOT NULL DEFAULT '',
    scheduled_start_at  {{TS}},
    schedule_id         TEXT,
    version             INTEGER NOT NULL DEFAULT 1,
    started_at          {{TS}},
    closed_at           {{TS}},
    created_by          TEXT NOT NULL DEFAULT '',
    created_at          {{TS}} NOT NULL,
    updated_at          {{TS}} NOT NULL,
    UNIQUE (project_id, name)
);
CREATE INDEX test_cycles_start ON test_cycles(scheduled_start_at);

-- The plan of a cycle: one row per test and configuration ('' without
-- configuration) with the tester it is assigned to (user id, '' nobody)
-- and the estimated duration in minutes.
CREATE TABLE test_cycle_tests (
    id                TEXT PRIMARY KEY,
    cycle_id          TEXT NOT NULL REFERENCES test_cycles(id) ON DELETE CASCADE,
    tenant_id         TEXT NOT NULL,
    test_case_id      TEXT NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    configuration_id  TEXT NOT NULL DEFAULT '',
    assignee_id       TEXT NOT NULL DEFAULT '',
    estimate_minutes  INTEGER NOT NULL DEFAULT 0,
    position          INTEGER NOT NULL DEFAULT 0,
    added_by          TEXT NOT NULL DEFAULT '',
    added_at          {{TS}} NOT NULL,
    UNIQUE (cycle_id, test_case_id, configuration_id)
);
CREATE INDEX test_cycle_tests_assignee ON test_cycle_tests(tenant_id, assignee_id);
CREATE INDEX test_cycle_tests_test ON test_cycle_tests(test_case_id);

-- The runs a cycle created.
ALTER TABLE test_runs ADD COLUMN cycle_id TEXT REFERENCES test_cycles(id) ON DELETE SET NULL;
CREATE INDEX test_runs_cycle ON test_runs(cycle_id);

-- Recurring cycles: on the schedule (frequency daily | weekly | monthly as
-- the e-mailed reports) a new cycle is created from the template cycle (its
-- plan, testers, estimates, release, variant and configurations) plus the
-- tests matching query, named after name_pattern ({date}, {n}), lasting
-- duration_days; with auto_start it starts at once (its runs are created).
CREATE TABLE test_cycle_schedules (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL,
    project_id         TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name_pattern       TEXT NOT NULL,
    template_cycle_id  TEXT REFERENCES test_cycles(id) ON DELETE SET NULL,
    query              TEXT NOT NULL DEFAULT '',
    owner_id           TEXT NOT NULL,
    frequency          TEXT NOT NULL,
    weekday            INTEGER NOT NULL DEFAULT 1,
    day_of_month       INTEGER NOT NULL DEFAULT 1,
    time_of_day        TEXT NOT NULL DEFAULT '06:00',
    timezone           TEXT NOT NULL DEFAULT 'UTC',
    duration_days      INTEGER NOT NULL DEFAULT 1,
    auto_start         {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    enabled            {{BOOL}} NOT NULL DEFAULT {{TRUE}},
    runs               INTEGER NOT NULL DEFAULT 0,
    next_run_at        {{TS}},
    last_run_at        {{TS}},
    last_cycle_id      TEXT,
    last_error         TEXT NOT NULL DEFAULT '',
    created_by         TEXT NOT NULL DEFAULT '',
    created_at         {{TS}} NOT NULL,
    updated_at         {{TS}} NOT NULL
);
CREATE INDEX test_cycle_schedules_due ON test_cycle_schedules(enabled, next_run_at);

-- Test leads plan cycles: the new permission cycle.plan for the roles whose
-- permissions an organisation changed (the defaults include it).
UPDATE roles SET permissions = REPLACE(permissions, '"run.write"', '"cycle.plan","run.write"')
    WHERE name IN ('test_lead', 'manager') AND permissions LIKE '%"run.write"%' AND permissions NOT LIKE '%"cycle.plan"%';
