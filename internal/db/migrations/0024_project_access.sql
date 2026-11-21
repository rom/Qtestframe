-- Per-project access (project sharing). A project is visible to every
-- member of its organisation ('organisation', the behaviour before this
-- migration) or only to its project members and the organisation's
-- administrators ('restricted'). A project member is a user or a SCIM
-- group with a project role (one of the organisation's roles); the role
-- replaces the member's organisation role inside the project (see
-- internal/api/access.go and docs/security.md).
ALTER TABLE projects ADD COLUMN access TEXT NOT NULL DEFAULT 'organisation';

CREATE TABLE project_members (
    tenant_id    TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    -- user | group (a SCIM group of the tenant)
    member_type  TEXT NOT NULL DEFAULT 'user',
    member_id    TEXT NOT NULL,
    role         TEXT NOT NULL,
    added_by     TEXT NOT NULL DEFAULT '',
    created_at   {{TS}} NOT NULL,
    updated_at   {{TS}} NOT NULL,
    PRIMARY KEY (project_id, member_type, member_id)
);
CREATE INDEX project_members_member ON project_members (tenant_id, member_type, member_id);
