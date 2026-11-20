-- Review requests to people (with a checklist) and signed baselines.
CREATE TABLE review_requests (
    id             TEXT PRIMARY KEY,
    tenant_id      TEXT NOT NULL,
    project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    entity_type    TEXT NOT NULL,
    entity_id      TEXT NOT NULL,
    entity_version INTEGER NOT NULL DEFAULT 0,
    reviewer_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    requested_by   TEXT NOT NULL DEFAULT '',
    message        TEXT NOT NULL DEFAULT '',
    checklist      TEXT NOT NULL DEFAULT '[]',
    status         TEXT NOT NULL DEFAULT 'pending',
    comment        TEXT NOT NULL DEFAULT '',
    due_date       TEXT NOT NULL DEFAULT '',
    decided_at     {{TS}},
    created_at     {{TS}} NOT NULL,
    updated_at     {{TS}} NOT NULL
);
CREATE INDEX review_requests_entity ON review_requests (entity_type, entity_id);
CREATE INDEX review_requests_reviewer ON review_requests (reviewer_id, status);
CREATE TABLE baseline_signoffs (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    baseline_id  TEXT NOT NULL REFERENCES baselines(id) ON DELETE CASCADE,
    user_id      TEXT NOT NULL,
    user_email   TEXT NOT NULL,
    user_name    TEXT NOT NULL DEFAULT '',
    meaning      TEXT NOT NULL,
    comment      TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL,
    signed_at    {{TS}} NOT NULL,
    UNIQUE (baseline_id, user_id, meaning)
);
