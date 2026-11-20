-- Test configurations (named combinations of dimension values such as
-- Browser = Chrome, OS = Windows), runs that target them, and data-driven
-- tests: parameters and data rows of a test, executed as iterations.

CREATE TABLE configurations (
    id                TEXT PRIMARY KEY,
    tenant_id         TEXT NOT NULL,
    project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    -- JSON [{"dimension": "Browser", "value": "Chrome"}, ...]
    dimension_values  TEXT NOT NULL DEFAULT '[]',
    position          INTEGER NOT NULL DEFAULT 0,
    created_by        TEXT NOT NULL DEFAULT '',
    created_at        {{TS}} NOT NULL,
    updated_at        {{TS}} NOT NULL,
    UNIQUE (project_id, name)
);

-- The configurations a run targets: every test of the run is executed once
-- per configuration.
CREATE TABLE run_configurations (
    run_id            TEXT NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
    configuration_id  TEXT NOT NULL REFERENCES configurations(id) ON DELETE CASCADE,
    position          INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (run_id, configuration_id)
);
CREATE INDEX run_configurations_config ON run_configurations(configuration_id);

-- Parameters (JSON list of names) and data rows (JSON
-- [{"name": "", "values": {"param": "value"}}]) of data-driven tests.
ALTER TABLE test_cases ADD COLUMN parameters TEXT NOT NULL DEFAULT '[]';
ALTER TABLE test_cases ADD COLUMN data_rows TEXT NOT NULL DEFAULT '[]';

-- test_executions loses UNIQUE (run_id, test_case_id): a test is executed
-- once per configuration of its run. Neither database can drop a table
-- constraint portably, so the table is rebuilt; step_results (the only
-- table referencing it) is set aside first so that dropping the old table
-- cannot cascade into it.
CREATE TABLE step_results_old AS SELECT id, execution_id, step_id, position, action, expected, status, actual, evidence FROM step_results;
DROP TABLE step_results;

CREATE TABLE test_executions_new (
    id                TEXT PRIMARY KEY,
    tenant_id         TEXT NOT NULL,
    run_id            TEXT NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
    test_case_id      TEXT NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    status            TEXT NOT NULL DEFAULT 'not_run',
    assignee          TEXT NOT NULL DEFAULT '',
    executed_by       TEXT NOT NULL DEFAULT '',
    started_at        {{TS}},
    finished_at       {{TS}},
    duration_ms       BIGINT NOT NULL DEFAULT 0,
    output            TEXT NOT NULL DEFAULT '',
    notes             TEXT NOT NULL DEFAULT '',
    position          INTEGER NOT NULL DEFAULT 0,
    defects           TEXT NOT NULL DEFAULT '[]',
    configuration_id  TEXT REFERENCES configurations(id) ON DELETE SET NULL
);
INSERT INTO test_executions_new (id, tenant_id, run_id, test_case_id, status, assignee, executed_by, started_at, finished_at, duration_ms, output, notes, position, defects)
    SELECT id, tenant_id, run_id, test_case_id, status, assignee, executed_by, started_at, finished_at, duration_ms, output, notes, position, defects FROM test_executions;
DROP TABLE test_executions;
ALTER TABLE test_executions_new RENAME TO test_executions;
CREATE INDEX executions_test ON test_executions(test_case_id);
CREATE INDEX executions_run ON test_executions(run_id, test_case_id);
CREATE INDEX executions_config ON test_executions(configuration_id);

-- One iteration per data row of a data-driven test (snapshot of the row).
CREATE TABLE execution_iterations (
    id            TEXT PRIMARY KEY,
    execution_id  TEXT NOT NULL REFERENCES test_executions(id) ON DELETE CASCADE,
    position      INTEGER NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    -- JSON {"param": "value"}
    data_values   TEXT NOT NULL DEFAULT '{}',
    status        TEXT NOT NULL DEFAULT 'not_run',
    notes         TEXT NOT NULL DEFAULT ''
);
CREATE INDEX execution_iterations_exec ON execution_iterations(execution_id);

CREATE TABLE step_results (
    id            TEXT PRIMARY KEY,
    execution_id  TEXT NOT NULL REFERENCES test_executions(id) ON DELETE CASCADE,
    step_id       TEXT NOT NULL DEFAULT '',
    position      INTEGER NOT NULL,
    action        TEXT NOT NULL DEFAULT '',
    expected      TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'not_run',
    actual        TEXT NOT NULL DEFAULT '',
    evidence      TEXT NOT NULL DEFAULT '',
    -- the iteration (data row) of a data-driven execution, else NULL
    iteration_id  TEXT REFERENCES execution_iterations(id) ON DELETE CASCADE
);
INSERT INTO step_results (id, execution_id, step_id, position, action, expected, status, actual, evidence)
    SELECT id, execution_id, step_id, position, action, expected, status, actual, evidence FROM step_results_old;
DROP TABLE step_results_old;
CREATE INDEX step_results_exec ON step_results(execution_id);
