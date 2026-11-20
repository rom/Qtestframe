-- Risks (likelihood × impact) and the requirements that address them.
ALTER TABLE projects ADD COLUMN risk_seq INTEGER NOT NULL DEFAULT 0;
CREATE TABLE risks (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    seq          INTEGER NOT NULL,
    key          TEXT NOT NULL,
    title        TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    category     TEXT NOT NULL DEFAULT '',
    likelihood   INTEGER NOT NULL DEFAULT 3,
    impact       INTEGER NOT NULL DEFAULT 3,
    status       TEXT NOT NULL DEFAULT 'identified',
    mitigation   TEXT NOT NULL DEFAULT '',
    owner        TEXT NOT NULL DEFAULT '',
    created_by   TEXT NOT NULL DEFAULT '',
    created_at   {{TS}} NOT NULL,
    updated_at   {{TS}} NOT NULL,
    UNIQUE (project_id, key)
);
CREATE TABLE risk_requirements (
    risk_id         TEXT NOT NULL REFERENCES risks(id) ON DELETE CASCADE,
    requirement_id  TEXT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    PRIMARY KEY (risk_id, requirement_id)
);
CREATE INDEX risk_requirements_req ON risk_requirements(requirement_id);
