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

## Test environments and test data

Test environments, their bookings and test data sets (migration 0057) need no configuration. The new permission `environment.manage` (test leads, managers and administrators by default; the migration adds it to stored test lead and manager roles that hold `run.write`) creates and changes environments, approves bookings and reveals an environment's secret; for an environment shared by the organisation it is taken from the organisation roles, not from project roles. Booking needs `run.execute` in the project the booking is for; test data sets use `test.read` and `test.write`.

An environment's optional secret is encrypted like the other stored secrets (`environments.secret`, re-encrypted by `qtestframe secrets rotate`), write-only in the API, left out of project archives, and every reveal is audited as `environment.secret_reveal`. Prefer the credentials reference (a vault path).

The minute job checks the environments that have a health check URL (at most 100 per minute, 8 at a time, 10 s timeout each) with the server's outgoing HTTP client: the URLs are entered by environment managers and the server must be able to reach them. A failing check marks an available environment broken and a passing one makes it available again; status changes are audited as `environment.health`. The tables `environments`, `environment_bookings`, `test_data_sets`, `test_data_links`, `test_data_usage` and `test_data_refresh_requests` are part of the backup; a project archive holds the project's own environments with their bookings and its test data sets (shared environments stay with the organisation). The migration turns the distinct free-text environments of each project's runs and cycles into environment items and links the runs, cycles and their final results to them. New webhook and chat events: `environment.status_changed`, `environment.booking_requested`, `environment.booking_approved`, `environment.booking_rejected`, `testdata.refresh_requested`, `testdata.refresh_resolved`.
