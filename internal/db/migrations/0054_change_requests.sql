-- Change requests (formal change control): a change request (PROJ-CR-n)
-- proposes new field values for requirements and tests, is assessed (a
-- snapshot of the transitive change impact is stored on it), decided by
-- the project's change control board with electronic signatures, applied
-- atomically and verified and closed.
ALTER TABLE projects ADD COLUMN cr_seq INTEGER NOT NULL DEFAULT 0;

-- The change control settings of a project (no row: change control off).
-- mode: off | warn | enforce; protected_statuses: JSON list of the item
-- statuses that are under change control ('[]' = the defaults);
-- lock_signed_baselines: items in a signed baseline are under change
-- control too; board: JSON {"users": [e-mail], "roles": [role]};
-- quorum: all | any | a number; signature_meaning: the text an approver
-- signs; baseline_on_close: closing creates a baseline by default.
CREATE TABLE change_control_settings (
    project_id            TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    tenant_id             TEXT NOT NULL,
    mode                  TEXT NOT NULL DEFAULT 'off',
    protected_statuses    TEXT NOT NULL DEFAULT '[]',
    lock_signed_baselines {{BOOL}} NOT NULL DEFAULT {{TRUE}},
    board                 TEXT NOT NULL DEFAULT '{}',
    quorum                TEXT NOT NULL DEFAULT 'all',
    signature_meaning     TEXT NOT NULL DEFAULT '',
    baseline_on_close     {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    updated_by            TEXT NOT NULL DEFAULT '',
    updated_at            {{TS}} NOT NULL
);

-- status: draft → submitted → assessed → in_review → approved | rejected |
-- deferred; approved → implemented → verified → closed. impact: the JSON
-- snapshot of the change impact taken at the assessment; links: JSON
-- {"defects": [id], "risks": [id], "issues": [text]}.
CREATE TABLE change_requests (
    id               TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL,
    project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    seq              INTEGER NOT NULL,
    key              TEXT NOT NULL,
    title            TEXT NOT NULL,
    reason           TEXT NOT NULL DEFAULT '',
    category         TEXT NOT NULL DEFAULT 'correction',
    priority         TEXT NOT NULL DEFAULT 'medium',
    status           TEXT NOT NULL DEFAULT 'draft',
    impact_action    TEXT NOT NULL DEFAULT 'suspect',
    impact           TEXT NOT NULL DEFAULT '',
    impact_notes     TEXT NOT NULL DEFAULT '',
    effort           TEXT NOT NULL DEFAULT '',
    assessed_by      TEXT NOT NULL DEFAULT '',
    assessed_at      {{TS}},
    links            TEXT NOT NULL DEFAULT '{}',
    baseline_on_close {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    baseline_id      TEXT NOT NULL DEFAULT '',
    decided_by       TEXT NOT NULL DEFAULT '',
    decided_at       {{TS}},
    decision_comment TEXT NOT NULL DEFAULT '',
    applied_by       TEXT NOT NULL DEFAULT '',
    applied_at       {{TS}},
    verified_by      TEXT NOT NULL DEFAULT '',
    verified_at      {{TS}},
    closed_by        TEXT NOT NULL DEFAULT '',
    closed_at        {{TS}},
    created_by       TEXT NOT NULL DEFAULT '',
    created_at       {{TS}} NOT NULL,
    updated_at       {{TS}} NOT NULL,
    UNIQUE (project_id, key)
);
CREATE INDEX change_requests_project ON change_requests (project_id, status);

-- The items a change request changes: proposed is the JSON object of the
-- new field values (as in a PATCH), base_version the version they were
-- proposed against, applied_version the version applying created.
CREATE TABLE change_request_items (
    id               TEXT PRIMARY KEY,
    cr_id            TEXT NOT NULL REFERENCES change_requests(id) ON DELETE CASCADE,
    entity_type      TEXT NOT NULL,
    entity_id        TEXT NOT NULL,
    base_version     INTEGER NOT NULL DEFAULT 0,
    proposed         TEXT NOT NULL DEFAULT '{}',
    position         INTEGER NOT NULL DEFAULT 0,
    applied_version  INTEGER NOT NULL DEFAULT 0,
    UNIQUE (cr_id, entity_type, entity_id)
);
CREATE INDEX change_request_items_entity ON change_request_items (entity_type, entity_id);

-- Electronic signatures of the board's decisions over a hash of the
-- change request's content.
CREATE TABLE change_request_signoffs (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    cr_id        TEXT NOT NULL REFERENCES change_requests(id) ON DELETE CASCADE,
    user_id      TEXT NOT NULL,
    user_email   TEXT NOT NULL,
    user_name    TEXT NOT NULL DEFAULT '',
    decision     TEXT NOT NULL,
    meaning      TEXT NOT NULL DEFAULT '',
    comment      TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL,
    signed_at    {{TS}} NOT NULL,
    UNIQUE (cr_id, user_id, content_hash)
);

-- The timeline of a change request: every step with who and why.
CREATE TABLE change_request_events (
    id        TEXT PRIMARY KEY,
    cr_id     TEXT NOT NULL REFERENCES change_requests(id) ON DELETE CASCADE,
    action    TEXT NOT NULL,
    actor     TEXT NOT NULL DEFAULT '',
    comment   TEXT NOT NULL DEFAULT '',
    at        {{TS}} NOT NULL
);
CREATE INDEX change_request_events_cr ON change_request_events (cr_id, at);

-- New permissions change.create (propose changes) and change.approve
-- (decide as the change control board, apply, emergency override) for
-- the built-in roles whose permissions an organisation changed (the
-- defaults include them).
UPDATE roles SET permissions = REPLACE(permissions, '"comment.create"', '"change.create","comment.create"')
    WHERE name IN ('sme', 'test_lead', 'manager') AND permissions LIKE '%"comment.create"%' AND permissions NOT LIKE '%"change.create"%';
UPDATE roles SET permissions = REPLACE(permissions, '"change.create"', '"change.approve","change.create"')
    WHERE name IN ('test_lead', 'manager') AND permissions LIKE '%"change.create"%' AND permissions NOT LIKE '%"change.approve"%';
