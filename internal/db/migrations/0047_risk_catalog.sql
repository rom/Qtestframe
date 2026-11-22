-- Risks in the catalogue (internal/catalog): the catalogue risk id a
-- project risk was copied from ('' = not from the catalogue), so that the
-- catalogue shows which risks a project already has and a second import
-- does not duplicate them.
ALTER TABLE risks ADD COLUMN catalog_ref TEXT NOT NULL DEFAULT '';
CREATE INDEX risks_catalog_ref ON risks (project_id, catalog_ref);
