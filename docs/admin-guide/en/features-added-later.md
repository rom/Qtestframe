# Features added later

This chapter collects the server-side administration notes of features that were added after the rest of this guide was written: configuration keys, permissions, background jobs, data that a backup does or does not contain, and outgoing connections. Each feature that needs such a note gets a section below; until then, the [user guide](../../user-guide/en/administration.md) describes the feature's screens.

When a feature adds configuration keys, system settings, permissions, webhook events, tables or command-line flags, the generated parts of this guide (configuration reference, system settings, permission matrix, webhook events, backup tables, migrations, command line) pick them up with `make admin-guide-ref`; only the explanation belongs here.

<!--
Placeholders for notes to fold in at merge (one "##" section per feature,
taken from the feature's admin notes in docs/user-guide/<lang>/administration.md
and checked against the code):

## Verification records
## Waivers
## Custom fields
## OSLC
## Code webhooks
## Variants
-->

## Exploratory testing

Exploratory test sessions (migration 0040) need no configuration. They use the run permissions: `run.read` to see sessions, `run.write` to plan, change and delete them and to debrief or stop someone else's session, `run.execute` to run one's own session (timer, notes, defects from notes, debrief). The session timer is kept in the database (`exploratory_sessions.running_since`, `elapsed_seconds`), so a restart of the server does not lose session time; a session left running keeps counting until someone pauses or stops it.

The tables `exploratory_sessions`, `exploratory_scope` and `exploratory_notes` are part of the backup and of project archives; screenshots are ordinary attachments (`files`, `attachments`) and count against `files.max_upload_mb`. Whether debriefed sessions are verification evidence is a project setting (`verification_settings.exploratory_evidence`, off by default), changed by someone with `project.update`. Every action is written to the audit log with actions `exploratory.*`.

## CI/CD integration

The quality gate, the test selection and the pipeline templates (migration 0053) need no configuration; the server-side posting of GitLab commit statuses and merge request notes is a project setting with an encrypted token and a background outbox (`ci_deliveries`, retried by the minute job, not backed up). Tokens, roles, the job token's limits and outgoing connections are described in [E-mail, webhooks and integrations](integrations.md); changes of the settings are audited as `project.ci_settings`.

## Live updates

Lists, dashboards, the notification bell and open items update when someone else changes something, and editors show who else is viewing or editing an item. The web client keeps one Server-Sent Events stream per browser tab open (`GET /api/v1/events/stream`, see the API reference `docs/openapi.yaml`); there is no table and no migration, everything is in memory. It is on by default; `live.enabled: false` (or `QTF_LIVE_ENABLED=false`) turns it off for the server, and every user can turn it off in the profile. When it is off, unavailable or interrupted, the client falls back to what it did before: the bell and *My work* poll every minute, lists reload when they are opened, and the conflict check on save (version, `If-Match`, 412) still protects against overwriting.

**What is sent.** An event says that an item changed: entity type, id, project, version, action and who did it. It never carries titles or content; the client reloads what it shows through the regular API, with its permission checks. Every event is checked against the user's access to its project (project sharing, project roles) and the read permission of its type (`requirement.read`, `test.read`, `run.read`, `project.read` for defects and releases) before it is written to a stream; events of other types or without a project are not sent at all. Notification events reach only their recipients and carry nothing but a ping. Changes of project access, roles or members make the streams re-read what their users may see; every minute a stream re-authenticates its request, so a revoked session or API token, a disabled user or a removed membership ends it. The events come from the audit trail (every audited change), after the request's transaction is committed; a change that is rolled back is never announced.

**Presence** (who is viewing or editing an item) is sent by the browser every 20 s while an item is open (`POST /api/v1/presence`) and forgotten after `live.presence_ttl` (45 s) without a heartbeat. It is not stored and not in the audit log. A user who turned live updates off is not shown to others.

**Several server instances.** On PostgreSQL the instances that share a database exchange events and presence with `LISTEN`/`NOTIFY` on a channel named after the database and schema (`qtf_live_…`); each instance keeps one extra database connection for listening. A listener that loses its connection reconnects with backoff and tells its clients to reload. On SQLite there is only one instance per database file, and events stay in that process (in-process fan-out only). No sticky sessions are needed: a client that reconnects to another instance resumes from its last event.

**Resume and limits.** Each instance keeps the last `live.replay_buffer` (512) events; a client that reconnects with `Last-Event-ID` receives what it missed, or reloads when that event is no longer kept. At most `live.max_connections` (2000) streams per instance and `live.max_connections_per_user` (8) per user; more are refused with `503` and the client retries with backoff (1 s up to 1 min) and polls meanwhile. A tab in the background closes its stream after a minute and resumes when it is shown again. Comment lines are sent every `live.heartbeat` (25 s) so that proxies do not close idle streams. On shutdown (`SIGTERM`) the streams are ended at once, so the 15 s drain is not held up; the clients reconnect.

**Reverse proxy.** The stream is a long-lived response that must not be buffered or compressed. Qtestframe sends `Content-Type: text/event-stream`, `Cache-Control: no-store, no-transform` and `X-Accel-Buffering: no` (nginx then does not buffer it even without the settings below). Keep the proxy's read timeout above `live.heartbeat`. With nginx:

```nginx
location /api/v1/events/stream {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_buffering off;
    proxy_cache off;
    gzip off;
    proxy_read_timeout 1h;
}
```

With Traefik (dynamic configuration) the stream needs no router of its own; a compression middleware must leave it out, and the entry point's `respondingTimeouts` must stay above `live.heartbeat` (the defaults are fine):

```yaml
http:
  routers:
    qtestframe:
      rule: Host(`qtf.example.com`)
      service: qtestframe
      middlewares: [compress-except-stream]
  middlewares:
    compress-except-stream:
      compress:
        excludedContentTypes: [text/event-stream]
  services:
    qtestframe:
      loadBalancer:
        servers:
          - url: http://127.0.0.1:8080
```

Each open tab is one HTTP connection; over HTTP/1.1 browsers allow six per site, so with many tabs open serve the site over HTTP/2 (one connection for all streams).
