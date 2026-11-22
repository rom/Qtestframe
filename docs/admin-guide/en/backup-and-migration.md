# Backup, restore and migration

## What to back up

| What | Where | How |
|---|---|---|
| **the data** | the database | `qtestframe backup` (logical, recommended), or a backup of the database itself |
| **the key of the stored secrets** | the key file (`qtestframe-secrets.key`), `QTF_SECRETS_KEY`, the HSM key; a TPM cannot be backed up | separately – **never in the same place as the database backups** |
| the configuration | `/etc/qtestframe` (config, secret files, TLS and SAML keys) | your configuration management or an encrypted backup |

Uploaded files (attachments, documents, reports, avatars) are stored in the database, so a database backup contains them. The runner's work directory is scratch space.

A backup without the key is useless for the encrypted secrets; a backup together with the key gives everything to whoever holds both. Encrypt backups and store them off-host.

## What a backup contains

`qtestframe backup` writes a **logical backup**: a ZIP with `manifest.json` (format version, schema version, time, version of Qtestframe, database driver, row count per table) and one `<table>.jsonl` file per table, one JSON object per row. Times, booleans and binary data are written in a database-neutral way, so a backup of SQLite restores into PostgreSQL and the other way round. It contains **every organisation**: users and password hashes, memberships, projects with all their items, history and files, the audit log, API and SCIM tokens (as hashes), the MFA settings, the system settings and the wrapped data-encryption keys. The backup is a consistent snapshot (one read transaction).

<!-- BEGIN GENERATED: backup-tables (make admin-guide-ref) -->

**In a backup** (87 tables, in restore order): `tenants`, `users`, `memberships`, `api_tokens`, `projects`, `configurations`, `releases`, `folders`, `categories`, `requirements`, `requirement_categories`, `evaluation_criteria`, `test_cases`, `test_steps`, `test_case_categories`, `trace_links`, `variants`, `builds`, `test_runs`, `run_configurations`, `test_executions`, `execution_iterations`, `step_results`, `audit_log`, `revisions`, `requirement_relations`, `baselines`, `baseline_items`, `comments`, `files`, `file_blobs`, `attachments`, `report_definitions`, `login_events`, `roles`, `test_dependencies`, `trash`, `item_links`, `release_requirements`, `defects`, `defect_links`, `risks`, `risk_requirements`, `notifications`, `webhooks`, `review_requests`, `baseline_signoffs`, `scim_tokens`, `scim_settings`, `scim_users`, `scim_groups`, `scim_group_members`, `metric_snapshots`, `trace_rules`, `document_items`, `defect_regressions`, `consistency_dismissals`, `invitations`, `report_subscriptions`, `user_mfa`, `mfa_recovery_codes`, `mfa_policies`, `project_members`, `waivers`, `waiver_requirements`, `waiver_signoffs`, `soa_decisions`, `system_settings`, `external_refs`, `link_criteria`, `system_elements`, `requirement_elements`, `test_elements`, `verification_settings`, `verification_records`, `verification_record_files`, `item_applicability`, `code_changes`, `code_change_items`, `code_settings`, `custom_fields`, `field_set_fields`, `custom_field_archive`, `item_templates`, `org_item_templates`, `project_template_defaults`, `secret_keys`.

**Not in a backup:** `job_runs`, `mfa_challenges`, `mfa_trusted_devices`, `schema_migrations`, `script_jobs`, `sessions`, `suggest_history`.

<!-- END GENERATED: backup-tables -->

So a restore signs everyone out (sessions), drops pending MFA sign-ins and remembered devices, empties every user's **autocomplete history** (a personal convenience, deliberately not backed up), and forgets queued background jobs. The **key** of the stored secrets is never in a backup: the stored secrets (TOTP secrets, webhook secrets, tracker tokens, secret test variables, SMTP password and OIDC client secrets of the system settings) stay **encrypted** inside it, together with their wrapped data keys (`secret_keys`).

A backup refuses to run while the database has a table that it does not know (a development safeguard).

## Taking a backup

```sh
sudo -u qtestframe qtestframe backup --config /etc/qtestframe/config.yaml --out /var/backups/qtestframe/qtf-$(date +%F).zip
```

It may run while the server runs. The file is written under a temporary name and renamed when complete, and the backup is audited (`admin.backup`). Super administrators can also download one under **Administration → Backup** (`GET /api/v1/admin/backup`); a download through the browser is bounded by `server.write_timeout`, so use the command line for large installations. On SQLite the server is busy while a backup streams.

A daily timer:

```ini
# /etc/systemd/system/qtestframe-backup.service
[Service]
Type=oneshot
User=qtestframe
ExecStart=/bin/sh -c '/usr/bin/qtestframe backup --config /etc/qtestframe/config.yaml --out /var/backups/qtestframe/qtf-$(date +%%F).zip'

# /etc/systemd/system/qtestframe-backup.timer
[Timer]
OnCalendar=daily
Persistent=true
[Install]
WantedBy=timers.target
```

The directory must exist and be writable by `qtestframe`. Copy the files off-host (restic, borg) and delete old ones.

## Restore

A restore **replaces all data** of the installation with the backup, in **one transaction**: every table is emptied and refilled, so either everything is restored or nothing changes.

```sh
sudo systemctl stop qtestframe
sudo -u qtestframe qtestframe restore --config /etc/qtestframe/config.yaml --in /var/backups/qtestframe/qtf-2026-11-19.zip --yes
sudo systemctl start qtestframe
```

The command prints the rows per table and is audited (`admin.restore`). Super administrators can also restore under **Administration → Backup** (`POST /api/v1/admin/restore`, confirmed by typing `RESTORE`); the upload is bounded by `server.read_timeout`, so use the command line for large backups. Stop the server for a command-line restore: a running server keeps its keys and caches of the old data.

A restore is refused, and nothing is changed, when:

- the **schema version** of the backup differs from the database's. Restore with the Qtestframe version that made the backup (the manifest names it), then upgrade; or upgrade the old installation first, then back up again;
- this server's **key cannot unwrap** the backup's data keys (`the backup's encrypted secrets cannot be decrypted with this server's key`). Configure the key the backup was made with – the same key file or `QTF_SECRETS_KEY`, the same TPM, the same HSM key – or see [disaster recovery](security.md#disaster-recovery-for-tpm-sealed-keys);
- the archive is damaged or incomplete, or larger than 4 GiB uncompressed.

The system settings of the backup apply at once. After a restore, check `qtestframe secrets status` and sign in.

To recover a single item, use its history in the web client; to move one project, see below.

## Upgrading

1. Read the release notes.
2. Take a backup and make sure you have the key.
3. Install the new binary (`sudo scripts/install.sh --binary bin/qtestframe` keeps the configuration and re-installs unit and policies), or pull or build the new image.
4. Restart: `sudo systemctl restart qtestframe`. The unit's `ExecStartPre` runs `qtestframe migrate`, and `serve` migrates as well, before the new version serves requests. In compose, the `migrate` service runs first.
5. Check `journalctl -u qtestframe -b` (`applied database migrations count=…`) and `curl -fsk https://localhost:8443/readyz`.
6. Compare `deploy/config.example.yaml` with your configuration for new options; `check-config` rejects keys that no longer exist.

With PostgreSQL and several server processes, **upgrade all of them together**: run the migration once, then start the new version everywhere. An older version cannot read what a newer one wrote.

## Migrations

The database schema is created and upgraded by numbered migrations embedded in the binary (`internal/db/migrations`). Each is applied once, in its own transaction, and recorded in `schema_migrations`; the highest number is the *schema version* that backups carry.

<!-- BEGIN GENERATED: migrations (make admin-guide-ref) -->

| Version | Migration |
|---|---|
| 1 | `0001_init.sql` |
| 2 | `0002_history_relations_baselines.sql` |
| 3 | `0003_files_reports_logins.sql` |
| 4 | `0004_roles_settings_dependencies.sql` |
| 5 | `0005_project_color_sources.sql` |
| 6 | `0006_trash.sql` |
| 7 | `0007_project_type.sql` |
| 8 | `0008_item_links.sql` |
| 9 | `0009_suspect_links.sql` |
| 10 | `0010_releases.sql` |
| 11 | `0011_defects.sql` |
| 12 | `0012_risks.sql` |
| 13 | `0013_notifications.sql` |
| 14 | `0014_reviews.sql` |
| 15 | `0015_email.sql` |
| 16 | `0016_configurations.sql` |
| 17 | `0017_trace_rules.sql` |
| 18 | `0018_metrics.sql` |
| 19 | `0019_scim.sql` |
| 20 | `0020_catalog.sql` |
| 21 | `0021_report_language.sql` |
| 22 | `0022_folder_color.sql` |
| 23 | `0023_web_tests.sql` |
| 24 | `0024_project_access.sql` |
| 25 | `0025_mfa.sql` |
| 26 | `0026_system_settings.sql` |
| 27 | `0027_secret_keys.sql` |
| 28 | `0028_item_link_issues.sql` |
| 29 | `0029_suggest_history.sql` |
| 30 | `0030_verification.sql` |
| 31 | `0031_change_defect_correlations.sql` |
| 32 | `0032_waivers_residual_soa.sql` |
| 33 | `0033_consistency_quality.sql` |
| 34 | `0034_custom_fields.sql` |
| 35 | `0035_external_refs.sql` |
| 36 | `0036_variants_code.sql` |
| 37 | `0037_import_sources.sql` |

<!-- END GENERATED: migrations -->

## Downgrading

Migrations only go forward. To go back to an earlier version, restore the backup taken before the upgrade **with the earlier binary**; data written since is lost. Do not start an older binary on a database that a newer one has migrated.

Versions from before the encryption of stored secrets (schema version below 27) read encrypted values as wrong secrets. Before going back to one of them without a backup, run

```sh
qtestframe secrets decrypt --yes --config /etc/qtestframe/config.yaml
```

which stores every secret as plaintext again (a later start of a current version encrypts them again).

## Moving between SQLite and PostgreSQL

The logical backup is the way to move, in either direction:

1. Back up the old installation: `qtestframe backup --config old.yaml --out move.zip`.
2. Prepare the new configuration with the other `database` section and **the same key** of the stored secrets: copy the key file and point `secrets.local.key_file` at it (the default place differs: next to a SQLite database, else `$STATE_DIRECTORY` or `/var/lib/qtestframe`), or set the same `QTF_SECRETS_KEY`.
3. Create the empty schema: `qtestframe migrate --config new.yaml` (same version of Qtestframe).
4. Restore: `qtestframe restore --config new.yaml --in move.zip --yes`.
5. Start the server with `new.yaml` and check it.

For PostgreSQL, create a dedicated role and database first, for example:

```sql
CREATE ROLE qtestframe LOGIN PASSWORD 'change-me';
CREATE DATABASE qtestframe OWNER qtestframe ENCODING 'UTF8' TEMPLATE template0;
```

and connect with TLS (`sslmode=verify-full`). Details, `pg_hba.conf` and client certificates are in [docs/operations.md](https://github.com/rom/Qtestframe/blob/rom/docs/operations.md#4-database-sqlite-or-postgresql).

## Backups of the database itself

These are fine as well, but are tied to one database system:

- **SQLite:** an online copy with `sqlite3 /var/lib/qtestframe/qtestframe.db ".backup '/var/backups/qtestframe/qtf.db'"` (or `VACUUM INTO`). Never copy the `.db` file alone while the server runs; without `sqlite3`, stop the service and copy `qtestframe.db*`. To restore, stop the service, put the copy in place, delete the `-wal` and `-shm` files and start (an older copy is migrated forward).
- **PostgreSQL:** `pg_dump -Fc` and `pg_restore`, or WAL archiving (pgBackRest, Barman) for point-in-time recovery.

Both contain the same encrypted secrets and need the same key.

## Moving a single project

To move or copy **one** project rather than the installation, use **Save project** (needs `project.update` and `export`) and **Load project** on the target server (needs `project.create`). The project gets new ids and a key of the user's choice. Archives of an older schema load into a newer server, not the other way round. Secrets (webhook secrets, tracker tokens) are not in the archive and must be set again; loaded webhooks are disabled.
