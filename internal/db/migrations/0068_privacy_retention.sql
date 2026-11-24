-- Privacy and data retention (see docs/admin-guide "Privacy and data
-- retention").
--
-- Legal hold of a project or a release: while legal_hold is set, the
-- nightly retention job purges nothing of it (runs, results, logs, notes)
-- and it cannot be deleted. legal_hold_reason, _by (e-mail) and _at say
-- who set it, when and why.
ALTER TABLE projects ADD COLUMN legal_hold {{BOOL}} NOT NULL DEFAULT {{FALSE}};
ALTER TABLE projects ADD COLUMN legal_hold_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN legal_hold_by TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN legal_hold_at {{TS}};
ALTER TABLE releases ADD COLUMN legal_hold {{BOOL}} NOT NULL DEFAULT {{FALSE}};
ALTER TABLE releases ADD COLUMN legal_hold_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE releases ADD COLUMN legal_hold_by TEXT NOT NULL DEFAULT '';
ALTER TABLE releases ADD COLUMN legal_hold_at {{TS}};

-- Erased (anonymised) accounts: erased_no is the number of the pseudonym
-- ("Former user #<n>", address former-user-<n>@erased.invalid), erased_at
-- when it was done. The account row stays so that authored content,
-- electronic signatures and the audit trail keep a valid reference.
ALTER TABLE users ADD COLUMN erased_no INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN erased_at {{TS}};

-- The retention job's purges look for old rows by time.
CREATE INDEX login_events_created ON login_events(created_at);
CREATE INDEX notifications_created ON notifications(tenant_id, created_at);
