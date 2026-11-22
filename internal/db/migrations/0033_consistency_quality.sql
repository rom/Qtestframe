-- Built-in consistency checks and quality metrics.
--
-- found_in: the phase in which a defect was found (model.DefectPhases:
-- requirements, design, development, test, acceptance, production; ''
-- unknown). Defects found in production count as escaped defects in the
-- quality metrics.
ALTER TABLE defects ADD COLUMN found_in TEXT NOT NULL DEFAULT '';
-- Dismissed findings of the built-in consistency checks: "not a
-- duplicate" for a pair of requirements the duplicate check suspects.
-- item_a < item_b (ids, the pair is unordered). Rows stay when an item is
-- deleted (a restore from the trash brings the item back with its id).
CREATE TABLE consistency_dismissals (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    check_kind  TEXT NOT NULL,
    item_a      TEXT NOT NULL,
    item_b      TEXT NOT NULL,
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  {{TS}} NOT NULL,
    UNIQUE (project_id, check_kind, item_a, item_b)
);
