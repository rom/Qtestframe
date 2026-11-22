-- Custom fields and item templates (see internal/customfield,
-- internal/store/customfields.go and docs/api.md "Custom fields").
--
-- custom_fields: the field definitions of a project, per item kind
-- (requirement, test, defect, risk, run). The key is the stable name used
-- in queries (cf.<key>), exports and the stored values; options (JSON array)
-- are the choices of select fields; default_value is the JSON encoding of
-- the default ('' for none).
CREATE TABLE custom_fields (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    entity_type   TEXT NOT NULL,
    key           TEXT NOT NULL,
    label         TEXT NOT NULL,
    type          TEXT NOT NULL,
    unit          TEXT NOT NULL DEFAULT '',
    options       TEXT NOT NULL DEFAULT '[]',
    required      {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    default_value TEXT NOT NULL DEFAULT '',
    help          TEXT NOT NULL DEFAULT '',
    position      INTEGER NOT NULL DEFAULT 0,
    show_in_list  {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    created_at    {{TS}} NOT NULL,
    updated_at    {{TS}} NOT NULL,
    UNIQUE (project_id, entity_type, key)
);
CREATE INDEX custom_fields_project ON custom_fields (project_id, entity_type, position);

-- field_set_fields: the organisation's field sets, one per project type.
-- A new project of that type gets a copy; existing projects can apply it.
CREATE TABLE field_set_fields (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_type  TEXT NOT NULL,
    entity_type   TEXT NOT NULL,
    key           TEXT NOT NULL,
    label         TEXT NOT NULL,
    type          TEXT NOT NULL,
    unit          TEXT NOT NULL DEFAULT '',
    options       TEXT NOT NULL DEFAULT '[]',
    required      {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    default_value TEXT NOT NULL DEFAULT '',
    help          TEXT NOT NULL DEFAULT '',
    position      INTEGER NOT NULL DEFAULT 0,
    show_in_list  {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    created_at    {{TS}} NOT NULL,
    updated_at    {{TS}} NOT NULL,
    UNIQUE (tenant_id, project_type, entity_type, key)
);

-- custom_field_archive: the values of a deleted field definition, kept so
-- that nothing is lost (definition: the field as JSON; item_values: JSON
-- array of {id, key, value} per item that had a value).
CREATE TABLE custom_field_archive (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    field_key   TEXT NOT NULL,
    label       TEXT NOT NULL DEFAULT '',
    definition  TEXT NOT NULL DEFAULT '{}',
    item_values TEXT NOT NULL DEFAULT '[]',
    item_count  INTEGER NOT NULL DEFAULT 0,
    deleted_by  TEXT NOT NULL DEFAULT '',
    deleted_at  {{TS}} NOT NULL
);
CREATE INDEX custom_field_archive_project ON custom_field_archive (project_id, deleted_at);

-- The values: one JSON object per item, field key → value (string, number,
-- boolean or array of strings), validated against the definitions on save.
ALTER TABLE requirements ADD COLUMN custom_fields TEXT NOT NULL DEFAULT '{}';
ALTER TABLE test_cases ADD COLUMN custom_fields TEXT NOT NULL DEFAULT '{}';
ALTER TABLE defects ADD COLUMN custom_fields TEXT NOT NULL DEFAULT '{}';
ALTER TABLE risks ADD COLUMN custom_fields TEXT NOT NULL DEFAULT '{}';
ALTER TABLE test_runs ADD COLUMN custom_fields TEXT NOT NULL DEFAULT '{}';

-- item_templates: templates for new requirements and tests of a project
-- (content: the pre-filled fields as JSON, see model.TemplateContent).
CREATE TABLE item_templates (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    content     TEXT NOT NULL DEFAULT '{}',
    position    INTEGER NOT NULL DEFAULT 0,
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  {{TS}} NOT NULL,
    updated_at  {{TS}} NOT NULL,
    UNIQUE (project_id, kind, name)
);

-- org_item_templates: the organisation's default templates per project type.
CREATE TABLE org_item_templates (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_type TEXT NOT NULL,
    kind         TEXT NOT NULL,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    content      TEXT NOT NULL DEFAULT '{}',
    position     INTEGER NOT NULL DEFAULT 0,
    created_by   TEXT NOT NULL DEFAULT '',
    created_at   {{TS}} NOT NULL,
    updated_at   {{TS}} NOT NULL,
    UNIQUE (tenant_id, project_type, kind, name)
);

-- project_template_defaults: the template "New requirement / New test"
-- preselects in a project (a project, organisation or built-in template).
CREATE TABLE project_template_defaults (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    template_id TEXT NOT NULL,
    UNIQUE (project_id, kind)
);
