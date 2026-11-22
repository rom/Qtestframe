-- CI/CD integration: what the server posts back to the code host when
-- results of a pipeline arrive (GitLab commit statuses and merge request
-- notes). ci_token is the access token for the code host's API (encrypted,
-- write-only); ci_settings the JSON {provider, api_url, project,
-- post_status, post_note, release_id, conditions}.
ALTER TABLE code_settings ADD COLUMN ci_token TEXT NOT NULL DEFAULT '';
ALTER TABLE code_settings ADD COLUMN ci_settings TEXT NOT NULL DEFAULT '{}';

-- The outbox of status posts: one row per run and commit (and merge
-- request) waiting to be posted, being posted (sending, claimed by a server
-- until next_attempt_at), retried after a failure (retry) or finished
-- (sent, failed). What is posted is computed when it is sent, so results
-- that arrive while a row waits are included. The newest 50 finished rows
-- of a project are kept as its log.
CREATE TABLE ci_deliveries (
    id               TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL,
    project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    run_id           TEXT NOT NULL DEFAULT '',
    build_id         TEXT NOT NULL DEFAULT '',
    commit_sha       TEXT NOT NULL,
    merge_request    TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL,
    attempts         INTEGER NOT NULL DEFAULT 0,
    status_code      INTEGER NOT NULL DEFAULT 0,
    error            TEXT NOT NULL DEFAULT '',
    outcome          TEXT NOT NULL DEFAULT '',
    created_by       TEXT NOT NULL DEFAULT '',
    created_at       {{TS}} NOT NULL,
    next_attempt_at  {{TS}},
    sent_at          {{TS}}
);
CREATE INDEX ci_deliveries_project ON ci_deliveries(project_id, created_at);
CREATE INDEX ci_deliveries_due ON ci_deliveries(status, next_attempt_at);
