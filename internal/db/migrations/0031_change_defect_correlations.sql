-- Change and defect correlations.
--
-- When the substance of a requirement changes, the requirements related to
-- it (per relation type and direction, see the project's correlation
-- settings) become suspect: the relation remembers since when and why
-- until someone confirms it.
ALTER TABLE requirement_relations ADD COLUMN suspect_since {{TS}};
ALTER TABLE requirement_relations ADD COLUMN suspect_reason TEXT NOT NULL DEFAULT '';
-- the suspect end: source | target
ALTER TABLE requirement_relations ADD COLUMN suspect_end TEXT NOT NULL DEFAULT '';
-- When the substance (content fields) of a requirement or test last
-- changed: results recorded before it are stale, documents generated
-- before it are outdated. NULL: not changed since this was introduced.
ALTER TABLE requirements ADD COLUMN content_changed_at {{TS}};
ALTER TABLE test_cases ADD COLUMN content_changed_at {{TS}};
-- A tracker issue created from a requirement is out of date once the
-- requirement changed, until the issue is updated from it again.
ALTER TABLE item_links ADD COLUMN issue_outdated_since {{TS}};
ALTER TABLE item_links ADD COLUMN issue_outdated_reason TEXT NOT NULL DEFAULT '';
-- Per-project settings of change impact, result staleness, blocking
-- defects and regressions (JSON, see model.CorrelationSettings).
ALTER TABLE projects ADD COLUMN correlation_settings TEXT NOT NULL DEFAULT '{}';
-- The requirements and tests a generated document contains (with their
-- version at generation).
CREATE TABLE document_items (
    file_id      TEXT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    entity_type  TEXT NOT NULL,
    entity_id    TEXT NOT NULL,
    version      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (file_id, entity_type, entity_id)
);
CREATE INDEX document_items_entity ON document_items(entity_type, entity_id);
-- Regressions: a test that verified a resolved or closed defect failed
-- again; the defect was reopened (action reopened) or a linked regression
-- defect was created (action created).
CREATE TABLE defect_regressions (
    id                    TEXT PRIMARY KEY,
    tenant_id             TEXT NOT NULL,
    project_id            TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    defect_id             TEXT NOT NULL REFERENCES defects(id) ON DELETE CASCADE,
    regression_defect_id  TEXT REFERENCES defects(id) ON DELETE SET NULL,
    test_case_id          TEXT NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    execution_id          TEXT NOT NULL DEFAULT '',
    action                TEXT NOT NULL,
    previous_status       TEXT NOT NULL DEFAULT '',
    created_by            TEXT NOT NULL DEFAULT '',
    created_at            {{TS}} NOT NULL
);
CREATE INDEX defect_regressions_defect ON defect_regressions(defect_id);
CREATE INDEX defect_regressions_test ON defect_regressions(test_case_id);
