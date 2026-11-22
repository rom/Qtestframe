-- Where items came from in other tools (see internal/interop and
-- docs/user-guide "Exchanging data with other tools"): the ReqIF import
-- records, per project, the IDENTIFIER of each SPEC-OBJECT it imported as a
-- requirement (entity_type 'requirement') and of each SPEC-RELATION it
-- imported as a requirement relation ('relation'), so that importing the
-- document again updates those items instead of creating new ones, and
-- removes relations that were deleted in the other tool. version is the
-- item's version right after the import that wrote it: a different version
-- now means the item was edited here since, which the import preview shows
-- as a conflict. source is the document's tool (REQ-IF-TOOL-ID). Rows whose
-- item no longer exists are ignored and replaced on the next import.
CREATE TABLE external_refs (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    system       TEXT NOT NULL,
    entity_type  TEXT NOT NULL,
    entity_id    TEXT NOT NULL,
    external_id  TEXT NOT NULL,
    source       TEXT NOT NULL DEFAULT '',
    version      INTEGER NOT NULL DEFAULT 0,
    imported_at  {{TS}} NOT NULL
);
CREATE UNIQUE INDEX external_refs_external ON external_refs (project_id, system, entity_type, external_id);
CREATE INDEX external_refs_entity ON external_refs (entity_type, entity_id);
