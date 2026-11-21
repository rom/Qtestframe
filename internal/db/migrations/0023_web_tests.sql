-- Web tests (script language "playwright"): the project's settings (base
-- URLs per run environment, test variables) and the background jobs that
-- run automated tests ("Run script" of a web test, "Run automated tests"
-- of a run). Jobs do not survive a restart: unfinished jobs are reset when
-- the server starts (see internal/api/scriptjobs.go).
ALTER TABLE projects ADD COLUMN web_tests TEXT NOT NULL DEFAULT '{}';

CREATE TABLE script_jobs (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    run_id        TEXT NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
    execution_id  TEXT NOT NULL REFERENCES test_executions(id) ON DELETE CASCADE,
    -- the jobs enqueued together ("Run automated tests" of a run)
    batch_id      TEXT NOT NULL,
    seq           INTEGER NOT NULL DEFAULT 0,
    -- queued | running | passed | failed | skipped | cancelled | error | interrupted
    state         TEXT NOT NULL DEFAULT 'queued',
    -- JSON list of job ids (prerequisites in the same batch) that must finish first
    after_jobs    TEXT NOT NULL DEFAULT '[]',
    -- JSON: the execution's result before the job, restored when the job
    -- is cancelled, skipped or interrupted
    previous      TEXT NOT NULL DEFAULT '{}',
    requested_by  TEXT NOT NULL DEFAULT '',
    user_id       TEXT NOT NULL DEFAULT '',
    -- the server process that runs the job (several instances share a database)
    instance      TEXT NOT NULL DEFAULT '',
    message       TEXT NOT NULL DEFAULT '',
    created_at    {{TS}} NOT NULL,
    started_at    {{TS}},
    finished_at   {{TS}},
    heartbeat_at  {{TS}}
);
CREATE INDEX script_jobs_run ON script_jobs (run_id, created_at);
CREATE INDEX script_jobs_state ON script_jobs (state);
CREATE INDEX script_jobs_execution ON script_jobs (execution_id);
