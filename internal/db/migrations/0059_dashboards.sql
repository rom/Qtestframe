-- Customisable dashboards: layouts of widgets on a 12-column grid.
-- A project dashboard (project_id set) is shared with everyone who may read
-- the project (owner_id NULL) or personal (owner_id: its user); a home
-- dashboard (project_id NULL) is the personal cross-project one of its
-- owner. builtin names the built-in layout a shared dashboard replaces
-- (overview, trends, quality) or a personal one was copied from; source_id
-- the dashboard it was copied from. widgets is the JSON list
-- [{id, type, title, x, y, w, h, settings}].
CREATE TABLE dashboards (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    project_id  TEXT REFERENCES projects(id) ON DELETE CASCADE,
    owner_id    TEXT REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    builtin     TEXT NOT NULL DEFAULT '',
    source_id   TEXT NOT NULL DEFAULT '',
    widgets     TEXT NOT NULL DEFAULT '[]',
    position    INTEGER NOT NULL DEFAULT 0,
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  {{TS}} NOT NULL,
    updated_at  {{TS}} NOT NULL
);
CREATE INDEX dashboards_project ON dashboards(tenant_id, project_id);
CREATE INDEX dashboards_owner ON dashboards(owner_id);

-- The dashboard a project opens with (set by its administrators): the id
-- of a shared dashboard or the key of a built-in layout.
CREATE TABLE dashboard_defaults (
    project_id   TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    tenant_id    TEXT NOT NULL,
    dashboard_id TEXT NOT NULL,
    updated_by   TEXT NOT NULL DEFAULT '',
    updated_at   {{TS}} NOT NULL
);
