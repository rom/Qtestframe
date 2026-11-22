-- Owners of requirements, tests and risks, and the personal work list
-- ("My work", internal/api/worklist.go).
--
-- An owner is a project member, referenced by e-mail address like the
-- assignees of defects and executions; a name that is not a member stays
-- as free text. Tests get an owner of their own. Existing requirement and
-- risk owners that name exactly one member of the tenant (by e-mail
-- address or display name, ignoring case) become that member's e-mail
-- address; all other owner texts are kept as they are.
ALTER TABLE test_cases ADD COLUMN owner TEXT NOT NULL DEFAULT '';

UPDATE requirements SET owner = (
    SELECT u.email FROM users u JOIN memberships m ON m.user_id = u.id
    WHERE m.tenant_id = requirements.tenant_id
      AND (lower(u.email) = lower(trim(requirements.owner)) OR lower(u.display_name) = lower(trim(requirements.owner)))
    LIMIT 1)
WHERE owner <> '' AND (
    SELECT COUNT(*) FROM users u JOIN memberships m ON m.user_id = u.id
    WHERE m.tenant_id = requirements.tenant_id
      AND (lower(u.email) = lower(trim(requirements.owner)) OR lower(u.display_name) = lower(trim(requirements.owner)))) = 1;

UPDATE risks SET owner = (
    SELECT u.email FROM users u JOIN memberships m ON m.user_id = u.id
    WHERE m.tenant_id = risks.tenant_id
      AND (lower(u.email) = lower(trim(risks.owner)) OR lower(u.display_name) = lower(trim(risks.owner)))
    LIMIT 1)
WHERE owner <> '' AND (
    SELECT COUNT(*) FROM users u JOIN memberships m ON m.user_id = u.id
    WHERE m.tenant_id = risks.tenant_id
      AND (lower(u.email) = lower(trim(risks.owner)) OR lower(u.display_name) = lower(trim(risks.owner)))) = 1;

-- The work list looks items up by owner and assignee across the projects
-- of a tenant.
CREATE INDEX requirements_owner ON requirements (tenant_id, lower(owner));
CREATE INDEX test_cases_owner ON test_cases (tenant_id, lower(owner));
CREATE INDEX risks_owner ON risks (tenant_id, lower(owner));
CREATE INDEX defects_assignee ON defects (tenant_id, lower(assignee));
