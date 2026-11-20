-- Daily metric snapshots per project (trends over time). One row per
-- project and UTC day; data is a JSON object with the counts (see
-- store.MetricData). source is 'recorded' (taken on that day) or 'backfill'
-- (reconstructed from history when the project had no snapshots yet).
CREATE TABLE metric_snapshots (
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    tenant_id   TEXT NOT NULL,
    day         TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT 'recorded',
    data        TEXT NOT NULL DEFAULT '{}',
    created_at  {{TS}} NOT NULL,
    PRIMARY KEY (project_id, day)
);
