-- E-mail: notification mails (immediate or daily digest), invitations and
-- scheduled report subscriptions.

-- When the notification was e-mailed to its user (NULL = not yet).
ALTER TABLE notifications ADD COLUMN emailed_at {{TS}};

-- Invitations to join a tenant. Only the SHA-256 hash of the token is
-- stored; a token is single use and expires.
CREATE TABLE invitations (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email        TEXT NOT NULL,
    role         TEXT NOT NULL,
    token_hash   TEXT NOT NULL DEFAULT '',
    sso          {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    invited_by   TEXT NOT NULL DEFAULT '',
    created_at   {{TS}} NOT NULL,
    sent_at      {{TS}} NOT NULL,
    expires_at   {{TS}} NOT NULL,
    accepted_at  {{TS}},
    revoked_at   {{TS}}
);
CREATE INDEX invitations_tenant ON invitations (tenant_id, user_id);
CREATE INDEX invitations_token ON invitations (token_hash);

-- Scheduled e-mailing of a saved report.
CREATE TABLE report_subscriptions (
    id             TEXT PRIMARY KEY,
    tenant_id      TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    definition_id  TEXT NOT NULL REFERENCES report_definitions(id) ON DELETE CASCADE,
    owner_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    frequency      TEXT NOT NULL,
    weekday        INTEGER NOT NULL DEFAULT 1,
    day_of_month   INTEGER NOT NULL DEFAULT 1,
    time_of_day    TEXT NOT NULL DEFAULT '07:00',
    timezone       TEXT NOT NULL DEFAULT 'UTC',
    format         TEXT NOT NULL DEFAULT 'pdf',
    recipients     TEXT NOT NULL DEFAULT '[]',
    addresses      TEXT NOT NULL DEFAULT '[]',
    message        TEXT NOT NULL DEFAULT '',
    enabled        {{BOOL}} NOT NULL DEFAULT {{TRUE}},
    next_run_at    {{TS}},
    last_run_at    {{TS}},
    last_sent_at   {{TS}},
    last_error     TEXT NOT NULL DEFAULT '',
    created_at     {{TS}} NOT NULL,
    updated_at     {{TS}} NOT NULL
);
CREATE INDEX report_subscriptions_project ON report_subscriptions (project_id);
CREATE INDEX report_subscriptions_due ON report_subscriptions (enabled, next_run_at);

-- Server jobs that run at a time of day (e.g. the daily digest): when each
-- last ran, so a restart catches up once.
CREATE TABLE job_runs (
    name     TEXT PRIMARY KEY,
    last_run {{TS}} NOT NULL
);
