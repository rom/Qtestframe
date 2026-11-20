-- Tenant-specific roles, tenant settings (execution policy), test
-- dependencies and evidence on step results.

-- Changed built-in roles and custom roles of a tenant. Built-in roles
-- without a row use their defaults (internal/rbac); admin never has a row.
CREATE TABLE roles (
    tenant_id   TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    label       TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    permissions TEXT NOT NULL DEFAULT '[]',
    created_at  {{TS}} NOT NULL,
    updated_at  {{TS}} NOT NULL,
    PRIMARY KEY (tenant_id, name)
);

-- JSON object, see model.TenantSettings.
ALTER TABLE tenants ADD COLUMN settings TEXT NOT NULL DEFAULT '{}';

-- test_id can only be executed after depends_on_id has passed.
CREATE TABLE test_dependencies (
    id             TEXT PRIMARY KEY,
    tenant_id      TEXT NOT NULL,
    project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    test_id        TEXT NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    depends_on_id  TEXT NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    created_by     TEXT NOT NULL DEFAULT '',
    created_at     {{TS}} NOT NULL,
    UNIQUE (test_id, depends_on_id)
);
CREATE INDEX test_dependencies_project ON test_dependencies(project_id);
CREATE INDEX test_dependencies_target ON test_dependencies(depends_on_id);

-- What the tester observed as proof (text; files are attachments of the
-- step result, entity_type 'step').
ALTER TABLE step_results ADD COLUMN evidence TEXT NOT NULL DEFAULT '';
