-- Releases: a version of the product with the requirements it contains, the
-- runs that test it and exit criteria that decide go / no-go.
CREATE TABLE releases (
    id             TEXT PRIMARY KEY,
    tenant_id      TEXT NOT NULL,
    project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    version        TEXT NOT NULL DEFAULT '',
    description    TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'planned',
    target_date    TEXT NOT NULL DEFAULT '',
    exit_criteria  TEXT NOT NULL DEFAULT '[]',
    released_at    {{TS}},
    released_by    TEXT NOT NULL DEFAULT '',
    release_note   TEXT NOT NULL DEFAULT '',
    created_by     TEXT NOT NULL DEFAULT '',
    created_at     {{TS}} NOT NULL,
    updated_at     {{TS}} NOT NULL,
    UNIQUE (project_id, name)
);
CREATE TABLE release_requirements (
    release_id      TEXT NOT NULL REFERENCES releases(id) ON DELETE CASCADE,
    requirement_id  TEXT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    PRIMARY KEY (release_id, requirement_id)
);
CREATE INDEX release_requirements_req ON release_requirements(requirement_id);
ALTER TABLE test_runs ADD COLUMN release_id TEXT REFERENCES releases(id) ON DELETE SET NULL;
