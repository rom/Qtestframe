-- Several roles per user (see docs/security.md, "Several roles"): an
-- organisation membership, a project membership, an invitation and an API
-- token hold a set of roles; the effective permissions are the union of
-- the roles' permissions. roles is a JSON list of role names in
-- rbac.NormalizeRoles order (admin first, then the built-in roles from most
-- to least privileged, then custom roles alphabetically). The role column
-- stays and holds the first of them, the primary role: older clients read
-- it, and "role = 'admin'" still finds every administrator because admin
-- is always the primary role of a set that holds it. Rows written before
-- this migration (or restored from an older backup) with roles = '[]'
-- read as [role].
ALTER TABLE memberships ADD COLUMN roles TEXT NOT NULL DEFAULT '[]';
ALTER TABLE project_members ADD COLUMN roles TEXT NOT NULL DEFAULT '[]';
ALTER TABLE invitations ADD COLUMN roles TEXT NOT NULL DEFAULT '[]';
ALTER TABLE api_tokens ADD COLUMN roles TEXT NOT NULL DEFAULT '[]';

-- Role names are 2–32 characters of a-z, 0-9 and _ (rbac.ValidName), so
-- they need no JSON escaping.
UPDATE memberships SET roles = '["' || role || '"]' WHERE role <> '';
UPDATE project_members SET roles = '["' || role || '"]' WHERE role <> '';
UPDATE invitations SET roles = '["' || role || '"]' WHERE role <> '';
UPDATE api_tokens SET roles = '["' || role || '"]' WHERE role <> '';

-- The new built-in role sme (subject matter expert) has its permissions
-- from the built-in defaults, like every built-in role nobody changed, so
-- no row is needed for it. An organisation that already made a custom role
-- named sme keeps it: its row now customises the built-in role (with the
-- organisation's permissions), and its label and description move into
-- the description so they are not lost.
UPDATE roles SET description = CASE WHEN description = '' THEN label ELSE label || ': ' || description END, label = ''
    WHERE name = 'sme' AND label <> '';
