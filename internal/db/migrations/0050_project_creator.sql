-- Who started a project: the user id of the account that created it (or
-- loaded or duplicated it from a project archive); '' when unknown (seeded,
-- created by an older version without an audit entry, or by the command
-- line). The API resolves the display name and e-mail.
ALTER TABLE projects ADD COLUMN created_by TEXT NOT NULL DEFAULT '';

-- Backfill from the audit log: the earliest project.create, project.load or
-- project.duplicate entry of the project (left unknown when there is none).
UPDATE projects SET created_by = COALESCE((
    SELECT a.user_id FROM audit_log a
    WHERE a.tenant_id = projects.tenant_id AND a.entity_type = 'project' AND a.entity_id = projects.id
      AND a.action IN ('project.create', 'project.load', 'project.duplicate') AND a.user_id <> ''
    ORDER BY a.created_at LIMIT 1), '');
