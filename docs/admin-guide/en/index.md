# Qtestframe administrator guide

This guide is for the people who **install, configure and run** a Qtestframe server: system administrators, operators and security officers. It covers the server, its configuration file, single sign-on, encryption keys, backups and day-to-day operation.

The people who *use* Qtestframe – including organisation administrators who manage members and roles in the web client – have the [user guide](../../user-guide/en/index.md). Where both guides touch the same subject (members, roles, system settings), this guide explains the server side and links to the user guide for the screens.

## Chapters

1. [Overview and architecture](overview.md) – components, data stores, ports, what runs where.
2. [Installation](installation.md) – system requirements, binary, container, systemd, building from source, SQLite or PostgreSQL.
3. [Configuration](configuration.md) – how settings are loaded, secret files, and the reference of every key and environment variable.
4. [First start](first-start.md) – the first administrator, organisations, demo data.
5. [Authentication](authentication.md) – local passwords, multi-factor authentication, OIDC, SAML, SCIM, API tokens, sessions.
6. [Authorisation](authorisation.md) – roles, custom roles, the permission matrix, restricted and shared projects.
7. [Runtime system settings](system-settings.md) – what super administrators can change in the web client, and what needs a restart.
8. [Security](security.md) – TLS, hardening with Landlock and seccomp, encryption of stored secrets and the key providers, key rotation, disaster recovery, the audit log, security headers.
9. [E-mail, webhooks and integrations](integrations.md) – SMTP, notifications, webhooks, Jira, GitHub and GitLab, the script and web test runner.
10. [Connecting other tools](connecting-tools.md) – OSLC connections with IBM DOORS Next and ETM, Polarion and Codebeamer, dialogs in frames, ReqIF.
11. [Logging and monitoring](logging-and-monitoring.md) – log formats, syslog, health endpoints.
12. [Backup, restore and migration](backup-and-migration.md) – what a backup contains, scheduled and encrypted backups, verification, restore, upgrades and downgrades, moving between SQLite and PostgreSQL.
13. [Operations runbook](runbook.md) – routine tasks, capacity, troubleshooting, log messages.
14. [Command line reference](cli-reference.md) – every `qtestframe` command and flag.
15. [Features added later](features-added-later.md) – administration notes of newer features.

## Conventions

- `qtestframe …` is the server binary; every command takes `--config FILE` (default: `$QTF_CONFIG`). On a systemd host run it as the service user, for example `sudo -u qtestframe qtestframe secrets status --config /etc/qtestframe/config.yaml`.
- Configuration keys are written with dots: `server.base_url` is `base_url` in the `server:` section of the YAML file. `auth.oidc[].name` is `name` in one entry of the list `auth.oidc`.
- **Super administrator** (super admin) is a flag of a user account that grants every permission in every organisation and the installation-wide pages (organisations, users, backup, system settings). **Admin** is a role inside one organisation.
- Paths such as `/etc/qtestframe` and `/var/lib/qtestframe` are those of the systemd installation (`scripts/install.sh`); the container image uses the same paths.

Parts of the [configuration reference](configuration.md#reference), the [system settings](system-settings.md#all-settings), the [permission matrix](authorisation.md#the-default-permission-matrix), the [webhook events](integrations.md#webhooks), the [backup tables](backup-and-migration.md#what-a-backup-contains), the [migrations](backup-and-migration.md#migrations) and the [command line reference](cli-reference.md) are generated from the source code (`make admin-guide-ref`), so they match the version they ship with.

## Also available as

The guide is also published as a PDF and as a single Markdown file per language on the [user-guide release](https://github.com/rom/Qtestframe/releases/tag/user-guide) (`qtestframe-admin-guide-en.pdf`, `qtestframe-admin-guide-sv.pdf`), and is shown in the web client under **Help → Administrator guide** to super administrators and organisation admins.
