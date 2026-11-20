-- Traceability rules: how the items of a project must be traced (for
-- example "every MUST requirement is verified by a test"). The kind-specific
-- settings are kept as JSON in options.
CREATE TABLE trace_rules (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    severity    TEXT NOT NULL DEFAULT 'error',
    enabled     {{BOOL}} NOT NULL DEFAULT {{TRUE}},
    scope       TEXT NOT NULL DEFAULT 'requirement',
    selector    TEXT NOT NULL DEFAULT '',
    kind        TEXT NOT NULL,
    min_count   INTEGER NOT NULL DEFAULT 1,
    options     TEXT NOT NULL DEFAULT '{}',
    position    INTEGER NOT NULL DEFAULT 0,
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  {{TS}} NOT NULL,
    updated_at  {{TS}} NOT NULL,
    UNIQUE (project_id, name)
);
CREATE INDEX trace_rules_project ON trace_rules (project_id, position);
