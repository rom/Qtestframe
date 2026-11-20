-- Links from a requirement, test or run to another item of the same project
-- (requirement, test, run, archive file, report definition) or to an
-- external URL (target_type 'url', target_id NULL).
CREATE TABLE item_links (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    source_type  TEXT NOT NULL,
    source_id    TEXT NOT NULL,
    target_type  TEXT NOT NULL,
    target_id    TEXT,
    url          TEXT NOT NULL DEFAULT '',
    label        TEXT NOT NULL DEFAULT '',
    created_by   TEXT NOT NULL DEFAULT '',
    created_at   {{TS}} NOT NULL
);
CREATE INDEX item_links_source ON item_links(source_type, source_id);
CREATE INDEX item_links_target ON item_links(target_type, target_id);
