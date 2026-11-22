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
