-- Requirement and test catalogue (internal/catalog): the catalogue id an
-- item was copied from ('' = not from the catalogue). It is kept when the
-- item is edited, copied, moved, trashed and restored, so a project shows
-- which catalogue items it already contains.
ALTER TABLE requirements ADD COLUMN catalog_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE test_cases ADD COLUMN catalog_ref TEXT NOT NULL DEFAULT '';
CREATE INDEX requirements_catalog_ref ON requirements (project_id, catalog_ref);
CREATE INDEX test_cases_catalog_ref ON test_cases (project_id, catalog_ref);
