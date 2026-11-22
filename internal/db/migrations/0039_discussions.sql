-- Discussions: the comments of requirements and tests become threaded
-- discussions, also on defects, risks, test runs and review requests
-- (entity_type defect | risk | run | review).
--
-- project_id: the project of the item (set for every new comment; older
-- comments get theirs below), so the project's discussions can be listed
-- and are removed with the project.
-- parent_id: the first comment of the thread for a reply ('' = the comment
-- starts a thread; replies to a reply belong to the same thread).
-- resolved_at / resolved_by: set on the first comment of a resolved thread
-- (user id).
-- deleted_at / deleted_by: a removed comment stays in its thread as
-- "removed" (its body and mentions are cleared).
-- mentions: JSON list of the user ids @mentioned in the body.
-- reactions: JSON object emoji → list of user ids.
ALTER TABLE comments ADD COLUMN project_id TEXT REFERENCES projects(id) ON DELETE CASCADE;
ALTER TABLE comments ADD COLUMN parent_id TEXT NOT NULL DEFAULT '';
ALTER TABLE comments ADD COLUMN resolved_at {{TS}};
ALTER TABLE comments ADD COLUMN resolved_by TEXT NOT NULL DEFAULT '';
ALTER TABLE comments ADD COLUMN deleted_at {{TS}};
ALTER TABLE comments ADD COLUMN deleted_by TEXT NOT NULL DEFAULT '';
ALTER TABLE comments ADD COLUMN mentions TEXT NOT NULL DEFAULT '[]';
ALTER TABLE comments ADD COLUMN reactions TEXT NOT NULL DEFAULT '{}';

UPDATE comments SET project_id = (SELECT r.project_id FROM requirements r WHERE r.id = comments.entity_id)
    WHERE entity_type = 'requirement';
UPDATE comments SET project_id = (SELECT t.project_id FROM test_cases t WHERE t.id = comments.entity_id)
    WHERE entity_type = 'test';

CREATE INDEX comments_project ON comments(project_id, parent_id);
CREATE INDEX comments_parent ON comments(parent_id);

-- The new permission comment.create (writing, resolving and reacting in
-- discussions) for the changed built-in roles and the custom roles that
-- work in projects (they can write or review something); the built-in
-- defaults have it from this version on.
UPDATE roles SET permissions = '["comment.create",' || SUBSTR(permissions, 2)
    WHERE permissions NOT LIKE '%"comment.create"%' AND permissions LIKE '["%'
      AND (permissions LIKE '%.write"%' OR permissions LIKE '%.review"%' OR permissions LIKE '%"run.execute"%');
