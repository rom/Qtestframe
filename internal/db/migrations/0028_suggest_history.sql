-- Autocomplete history (see internal/suggest, internal/api/suggest.go and
-- docs/security.md): the values a user saved in a free-text field, so the
-- field can suggest them next time. One row per user, organisation, field
-- key, project ('' for fields outside a project, such as the search box)
-- and distinct value; value_hash (SHA-256 of the value) keeps the unique
-- index short whatever the length of the value. Each user keeps at most
-- 200 values per field and organisation (the least recently used go). The
-- rows are personal and are deleted with the user, with the membership of
-- the organisation, and by "Clear my autocomplete history" on the profile;
-- they are not part of a backup.
CREATE TABLE suggest_history (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id  TEXT NOT NULL DEFAULT '',
    field       TEXT NOT NULL,
    value       TEXT NOT NULL,
    value_hash  TEXT NOT NULL,
    uses        INTEGER NOT NULL DEFAULT 1,
    last_used   {{TS}} NOT NULL
);
CREATE UNIQUE INDEX suggest_history_value ON suggest_history (user_id, tenant_id, field, project_id, value_hash);
CREATE INDEX suggest_history_recent ON suggest_history (user_id, tenant_id, field, last_used);
