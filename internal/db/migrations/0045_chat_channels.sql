-- Chat notifications: channels of a project that post its events to Slack,
-- Microsoft Teams, Mattermost, Rocket.Chat, Google Chat, Matrix or Discord,
-- and their deliveries.

-- secret: the incoming webhook URL, or the Matrix access token
-- (encrypted, write-only); target: where it posts, without the secret.
-- events: JSON list of event names; filters: JSON {failed_only,
-- min_priority, release_id}. failures: failed deliveries in a row (the
-- channel is turned off after five). next_digest_at: when the events
-- collected for a digest are posted.
CREATE TABLE chat_channels (
    id                  TEXT PRIMARY KEY,
    tenant_id           TEXT NOT NULL,
    project_id          TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    provider            TEXT NOT NULL,
    secret              TEXT NOT NULL DEFAULT '',
    homeserver          TEXT NOT NULL DEFAULT '',
    room                TEXT NOT NULL DEFAULT '',
    target              TEXT NOT NULL DEFAULT '',
    events              TEXT NOT NULL DEFAULT '[]',
    filters             TEXT NOT NULL DEFAULT '{}',
    language            TEXT NOT NULL DEFAULT 'en',
    digest_minutes      INTEGER NOT NULL DEFAULT 0,
    enabled             {{BOOL}} NOT NULL DEFAULT {{TRUE}},
    restricted_content  {{BOOL}} NOT NULL DEFAULT {{FALSE}},
    failures            INTEGER NOT NULL DEFAULT 0,
    disabled_reason     TEXT NOT NULL DEFAULT '',
    created_by          TEXT NOT NULL DEFAULT '',
    created_at          {{TS}} NOT NULL,
    updated_at          {{TS}} NOT NULL,
    last_delivery_at    {{TS}},
    last_status         INTEGER NOT NULL DEFAULT 0,
    last_error          TEXT NOT NULL DEFAULT '',
    next_digest_at      {{TS}}
);
CREATE INDEX chat_channels_project ON chat_channels(project_id);

-- One post of a channel (an event, a digest or a test message) with its
-- attempts: messages is the JSON list of the events in it. Batched rows
-- wait for the digest; queued and retry rows are due at next_attempt_at;
-- a sending row is claimed by a server until next_attempt_at. The newest
-- 50 finished deliveries of a channel are kept as its log.
CREATE TABLE chat_deliveries (
    id               TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL,
    channel_id       TEXT NOT NULL REFERENCES chat_channels(id) ON DELETE CASCADE,
    event            TEXT NOT NULL,
    messages         TEXT NOT NULL DEFAULT '[]',
    message_count    INTEGER NOT NULL DEFAULT 1,
    status           TEXT NOT NULL,
    attempts         INTEGER NOT NULL DEFAULT 0,
    status_code      INTEGER NOT NULL DEFAULT 0,
    error            TEXT NOT NULL DEFAULT '',
    created_at       {{TS}} NOT NULL,
    next_attempt_at  {{TS}},
    sent_at          {{TS}}
);
CREATE INDEX chat_deliveries_channel ON chat_deliveries(channel_id, created_at);
CREATE INDEX chat_deliveries_due ON chat_deliveries(status, next_attempt_at);
