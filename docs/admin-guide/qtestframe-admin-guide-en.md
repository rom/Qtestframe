# Qtestframe administrator guide

This guide is for the people who **install, configure and run** a Qtestframe server: system administrators, operators and security officers. It covers the server, its configuration file, single sign-on, encryption keys, backups and day-to-day operation.

The people who *use* Qtestframe – including organisation administrators who manage members and roles in the web client – have the [user guide](https://github.com/rom/Qtestframe/blob/rom/docs/user-guide/en/index.md). Where both guides touch the same subject (members, roles, system settings), this guide explains the server side and links to the user guide for the screens.

## Chapters

1. [Overview and architecture](#overview-and-architecture) – components, data stores, ports, what runs where.
2. [Installation](#installation) – system requirements, binary, container, systemd, building from source, SQLite or PostgreSQL.
3. [Configuration](#configuration) – how settings are loaded, secret files, and the reference of every key and environment variable.
4. [First start](#first-start) – the first administrator, organisations, demo data.
5. [Authentication](#authentication) – local passwords, multi-factor authentication, OIDC, SAML, SCIM, API tokens, sessions.
6. [Authorisation](#authorisation) – roles, custom roles, the permission matrix, restricted and shared projects.
7. [Runtime system settings](#runtime-system-settings) – what super administrators can change in the web client, and what needs a restart.
8. [Security](#security) – TLS, hardening with Landlock and seccomp, encryption of stored secrets and the key providers, key rotation, disaster recovery, the audit log, security headers.
9. [E-mail, webhooks and integrations](#e-mail-webhooks-and-integrations) – SMTP, notifications, webhooks, Jira, GitHub and GitLab, the script and web test runner.
10. [Connecting other tools](#connecting-other-tools-oslc-and-reqif) – OSLC connections with IBM DOORS Next and ETM, Polarion and Codebeamer, dialogs in frames, ReqIF.
11. [Logging and monitoring](#logging-and-monitoring) – log formats, syslog, health endpoints.
12. [Backup, restore and migration](#backup-restore-and-migration) – what a backup contains, restore, upgrades and downgrades, moving between SQLite and PostgreSQL.
13. [Operations runbook](#operations-runbook) – routine tasks, capacity, troubleshooting, log messages.
14. [Command line reference](#command-line-reference) – every `qtestframe` command and flag.
15. [Features added later](#features-added-later) – administration notes of newer features.

## Conventions

- `qtestframe …` is the server binary; every command takes `--config FILE` (default: `$QTF_CONFIG`). On a systemd host run it as the service user, for example `sudo -u qtestframe qtestframe secrets status --config /etc/qtestframe/config.yaml`.
- Configuration keys are written with dots: `server.base_url` is `base_url` in the `server:` section of the YAML file. `auth.oidc[].name` is `name` in one entry of the list `auth.oidc`.
- **Super administrator** (super admin) is a flag of a user account that grants every permission in every organisation and the installation-wide pages (organisations, users, backup, system settings). **Admin** is a role inside one organisation.
- Paths such as `/etc/qtestframe` and `/var/lib/qtestframe` are those of the systemd installation (`scripts/install.sh`); the container image uses the same paths.

Parts of the [configuration reference](#reference), the [system settings](#all-settings), the [permission matrix](#the-default-permission-matrix), the [webhook events](#webhooks), the [backup tables](#what-a-backup-contains), the [migrations](#migrations) and the [command line reference](#command-line-reference) are generated from the source code (`make admin-guide-ref`), so they match the version they ship with.

## Also available as

The guide is also published as a PDF and as a single Markdown file per language on the [user-guide release](https://github.com/rom/Qtestframe/releases/tag/user-guide) (`qtestframe-admin-guide-en.pdf`, `qtestframe-admin-guide-sv.pdf`), and is shown in the web client under **Help → Administrator guide** to super administrators and organisation admins.

# Overview and architecture

Qtestframe is **one program**, `qtestframe`. The same binary is the web server, the REST API, the embedded web client and the command line tool for administration. It needs nothing else at run time except a database, and with SQLite not even that.

## Components

| Component | What it is |
|---|---|
| `qtestframe serve` | The server process: HTTP(S) listener, REST API, the web client, background jobs. |
| Web client | A Vue 3 single-page application compiled into the binary (`internal/webui`). It is served from `/` and talks only to the server's own API. |
| REST API | `/api/v1/…`, described as OpenAPI 3.1 at `/api/v1/openapi.json` (public, no sign-in). Used by the web client, CI pipelines (API tokens) and `qtestframe report`. |
| SCIM 2.0 | `/scim/v2/…`, user and group provisioning from an identity provider, with its own tokens. |
| Health endpoints | `/healthz` (process alive, returns the version) and `/readyz` (database reachable). Not logged, no sign-in. |
| Command line | `qtestframe migrate`, `user create`, `backup`, `secrets …` and others open the database directly; see the [command line reference](#command-line-reference). |
| Script runner | Optional (`runner.enabled`). Runs the scripts of automated tests on the server, each in a sandboxed child process (`qtestframe sandbox-exec`). |
| Web test runner | Optional (`runner.web.enabled`). Runs Playwright tests with Node.js and a real browser as background jobs. |

## Data stores

| Store | Content | Notes |
|---|---|---|
| **Database** (SQLite or PostgreSQL) | Everything: organisations, users, projects, requirements, tests, runs, history, comments, the audit log, sessions, **uploaded files** (attachments, documents, reports, avatars) and the system settings changed in the web client. | Created and upgraded automatically by the [migrations](#migrations). |
| **Key of the stored secrets** | The key-encryption key that protects the secrets the server must read back (TOTP secrets, webhook secrets, tracker tokens, secret test variables, SMTP password and OIDC client secrets stored in the web client). | A key file (`qtestframe-secrets.key`), the `QTF_SECRETS_KEY` variable, a TPM 2.0 or an HSM; see [Encryption of stored secrets](#encryption-of-stored-secrets). **Never in the database.** |
| **Configuration** | The YAML file (`/etc/qtestframe/config.yaml`), secret files, TLS and SAML keys. | Read once at start-up; only the TLS key pair is re-read on `SIGHUP`. |
| **Runner work directory** | One job directory per script or web test (`runner.work_dir`). | Scratch space, not backed up. |

There is no separate file store, cache or message queue: a backup of the database (plus the key) is a backup of the installation.

## Ports and connections

Qtestframe listens on **one TCP port** (`server.listen`, default `:8080`; the production templates use `:8443` with TLS). All of the following go through it: the web client, the API, SCIM, the single sign-on callbacks and the health endpoints.

Outgoing connections, only when the feature is configured:

| To | For | Configured by |
|---|---|---|
| PostgreSQL | the database | `database.dsn` |
| OIDC provider | discovery, token endpoint, keys (JWKS) | `auth.oidc` |
| SAML IdP | metadata download (not with `idp_metadata_file`) | `auth.saml` |
| SMTP server | notification e-mails, invitations, reports | `email` |
| Syslog server | log shipping | `logging.syslog` |
| Jira, GitHub, GitLab | defects and issues | project settings (permission `project.update`) |
| Webhook receivers | signed event deliveries | project settings (permission `project.update`) |
| Systems under test | web tests and scripts | test content (permission `script.execute`) |

Because project administrators choose the URLs of trackers and webhooks, and script authors the targets of web tests, filter the server's outgoing traffic when it can reach internal services (see [Security](#outgoing-connections)).

## What runs inside the server process

Besides answering requests, `serve` runs a few background tasks:

| Task | When |
|---|---|
| Purge expired sessions and trash entries older than 30 days | every hour |
| Record a daily metric snapshot per project (dashboard trends) | at start-up and then every hour, once per project and day |
| Daily e-mail digests and scheduled report e-mails | checked every minute, only while e-mail is on |
| Worker pool for web tests and *Run automated tests* | `runner.web.max_parallel` jobs at a time |
| Webhook deliveries | after the change that triggers them is committed |

## One instance or several

**SQLite** means exactly one server process per database file: the database runs through a single connection in WAL mode, which serialises writes. This is the simplest setup and is enough for small and medium teams.

**PostgreSQL** allows several server processes against one database (for example behind a load balancer). Sessions, tokens and all data are shared. Each process keeps some state of its own, so:

- Configure **session affinity** at the load balancer at least for `/api/v1/auth/`: the OIDC/SAML login state is signed with a key that each process creates at start, so the redirect to the identity provider and the callback must reach the same process.
- The **sign-in rate limits** count per process.
- Each process runs its own **job pool**; jobs of a process that stops are reset by the others after about a minute.
- **Upgrade all processes together**: an older version cannot read what a newer one wrote (migrations, encrypted secrets).

How to choose between the databases is described under [Installation](#sqlite-or-postgresql).

# Installation

## System requirements

| | |
|---|---|
| **Operating system** | Linux on amd64 or arm64 for production. macOS (arm64) works for trying it out and for development; the in-process hardening (Landlock, seccomp, capabilities) exists only on Linux and is reported as skipped elsewhere. |
| **Kernel** | Landlock needs Linux 5.13 or later with `landlock` among the active security modules; older kernels run without it (logged at start-up). |
| **Memory and processes** | The shipped systemd unit allows 2 GB (`MemoryMax=2G`) and 512 tasks; the compose file 1 GB and 256 processes. Web tests start a browser per job and need considerably more (see `runner.web.max_parallel`). |
| **Disk** | The database holds every uploaded file. Plan for the attachments and documents of your projects plus the backups. |
| **Database** | Nothing for SQLite (built in, pure Go). PostgreSQL 13 or later (16 recommended) for larger installations. |
| **Optional** | An SMTP server for e-mail; an OIDC or SAML identity provider; a syslog server; Node.js 18+ and Playwright for web tests; Python, Bash and Node.js for scripts; a TPM 2.0 or an HSM for the encryption key. |
| **To build** | Go 1.26 (`go.mod` pins the toolchain 1.26.11, which `go` downloads), Node.js 20 or later (CI uses 22), `make`. A C compiler only for the HSM build. |

## SQLite or PostgreSQL

| | SQLite (default) | PostgreSQL |
|---|---|---|
| Set-up | none: the file is created at the first start | a separate database server |
| Server processes | exactly **one** per database file | one or several (load balancer, rolling restarts) |
| Concurrency | one connection, writes serialised; fine for tens of concurrent users and databases of a few GB | a connection pool (`database.max_open_conns`, default 20) |
| Backup | `qtestframe backup`, or a file copy with SQLite's online backup | `qtestframe backup`, `pg_dump`, point-in-time recovery |
| High availability | no | replication of the database server |
| File system | local disk only (not NFS); WAL files `-wal` and `-shm` next to it | — |

Choose **SQLite** for a single server, evaluations and teams that want the least to run. Choose **PostgreSQL** when you need more than one server process, the database must be managed by a database team, or you want replication and point-in-time recovery. You can move later in either direction with a [logical backup](#moving-between-sqlite-and-postgresql).

## From a binary

The binary is static (`CGO_ENABLED=0`, the SQLite driver is pure Go), so a copy of `qtestframe` is the whole installation. Static binaries for Linux amd64/arm64 and macOS arm64 are built with:

```sh
make release          # dist/qtestframe-linux-amd64, -linux-arm64, -darwin-arm64
```

Try it with the ready-made example configuration (HTTP on port 8080, SQLite in `./data`, a bootstrap administrator):

```sh
./qtestframe serve --config examples/qtestframe.yaml
```

Then sign in at `http://localhost:8080` as `admin@example.com` with `Change-me-1234` and change the password. **This example is not for production**: it turns off secure cookies and HSTS, enables the script runner and contains the bootstrap password in the file.

## On a systemd host

`scripts/install.sh` installs the binary, a hardened unit, the service user and the directory layout. It is idempotent: running it again upgrades binary and unit and keeps configuration, keys and data.

```sh
make build                                      # web client + bin/qtestframe
sudo scripts/install.sh --binary bin/qtestframe --self-signed-cert qtf.example.com
```

| Option | Effect |
|---|---|
| `--binary PATH` | binary to install (default `./bin/qtestframe` or `./dist/qtestframe`) |
| `--selinux auto\|yes\|no` | build and load the SELinux module (default `auto`: when SELinux is enabled) |
| `--apparmor` | install and enforce the AppArmor profile |
| `--sysctl` | install `deploy/sysctl/99-qtestframe.conf` |
| `--self-signed-cert [CN]` | create a self-signed certificate when none exists |
| `--no-enable`, `--no-start` | do not enable or (re)start the unit |
| `--dry-run` | print the actions only |

It creates:

| Path | Owner and mode | Content |
|---|---|---|
| `/usr/bin/qtestframe` | root, 0755 | the binary |
| `/etc/qtestframe/config.yaml` | root:qtestframe, 0640 | the configuration, from `deploy/config.example.yaml`; never overwritten |
| `/etc/qtestframe/tls/` | root:qtestframe, 0750 | `server.crt`, `server.key` (0640), SAML and client keys |
| `/etc/qtestframe/secrets/` | root:qtestframe, 0750 | files referenced by `*_file` options |
| `/var/lib/qtestframe/` | qtestframe, 0750 | the SQLite database, the key file, `runner/` |
| `/etc/systemd/system/qtestframe.service` | root, 0644 | the unit (`deploy/systemd/qtestframe.service`) |
| `/etc/sysusers.d/`, `/etc/tmpfiles.d/` | root | the user `qtestframe` and the directories |

If the configured certificate does not exist yet, the service is not started. Edit `/etc/qtestframe/config.yaml` (at least `server.base_url`, TLS and the bootstrap administrator, see [First start](#first-start)), then:

```sh
sudo systemctl enable --now qtestframe
journalctl -u qtestframe -f
```

The unit runs `qtestframe migrate` before `serve` (`ExecStartPre`), restarts the service on failure, and `systemctl reload qtestframe` sends `SIGHUP`, which re-reads the TLS certificate. Change the unit only with a drop-in (`sudo systemctl edit qtestframe`), for example for `LoadCredential=`, `MemoryMax=` or the relaxations the runner needs (see [Integrations](#script-and-web-test-runner)).

Remove it with `sudo scripts/uninstall.sh` (keeps configuration and data) or `--purge` (deletes everything, the database included).

### Firewall, MAC and port 443

- `deploy/nftables/qtestframe.nft` is a firewall ruleset with an optional egress policy; review its sets (`admin_v4`, `out_tcp_ports`, …) before loading it. Let's Encrypt HTTP-01 needs inbound port 80.
- SELinux (`deploy/selinux`) and AppArmor (`deploy/apparmor`) profiles confine the service; optional features are behind SELinux booleans such as `qtestframe_connect_postgresql`, `qtestframe_connect_http` (single sign-on), `qtestframe_remote_syslog` and `qtestframe_run_scripts`. See [Security](#operating-system-layers).
- To serve port 443, either set `server.listen: ":443"` (the unit grants `CAP_NET_BIND_SERVICE`; with SELinux also `setsebool -P qtestframe_bind_http_port on`) or keep `:8443` and redirect 443 to it with nftables, which allows a unit without any capability.

## In a container (Docker or Podman)

`deploy/container/Containerfile` builds a distroless, static image that runs as UID 65532 with a read-only root file system; state is in the volume `/var/lib/qtestframe`. The image sets `QTF_DATABASE_DRIVER=sqlite`, `QTF_DATABASE_DSN=/var/lib/qtestframe/qtestframe.db`, `QTF_SERVER_LISTEN=:8443` and `QTF_LOG_FORMAT=json`, so it runs without a configuration file. Its `HEALTHCHECK` runs `qtestframe healthcheck` (no shell or curl in the image).

```sh
docker build -f deploy/container/Containerfile --build-arg VERSION=$(git describe --tags --always) -t qtestframe .
```

`deploy/container/compose.yaml` runs Qtestframe with PostgreSQL 16 on an internal network. Prepare the secrets and the certificate as described at the top of the file, then, from `deploy/container` (the seccomp profile path is relative):

```sh
docker compose up -d --build        # or: podman compose up -d --build
docker compose logs -f qtestframe
```

A one-shot `migrate` service runs before the server. The compose file applies `cap_drop: ALL`, `no-new-privileges`, the seccomp allow-list `deploy/seccomp/qtestframe-seccomp.json`, a memory and process limit, and sets `QTF_DATABASE_DRIVER=postgres` (the image's default is SQLite).

Points to remember with containers:

- **Environment variables lock system settings.** A setting set by a `QTF_*` variable cannot be changed under **Administration → System settings** (see [Runtime system settings](#precedence-and-locking)); in the image this applies to the log format and the listen address.
- **The key file** of the stored secrets lives on the data volume by default (`/var/lib/qtestframe/qtestframe-secrets.key`). Back it up separately, or give the key as `QTF_SECRETS_KEY` from your secret store.
- **Web tests** need a bigger image: `--build-arg WITH_PLAYWRIGHT=true` builds one based on Debian with Node.js, `@playwright/test` and Chromium (`PLAYWRIGHT_BROWSERS` adds browsers). Enable them with `QTF_RUNNER_ENABLED=true` and `QTF_RUNNER_WEB_ENABLED=true`. The default image ships no interpreters, so scripts cannot run in it.
- **Kubernetes**: use the same image with `runAsNonRoot`, `readOnlyRootFilesystem`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, the seccomp profile as a `Localhost` profile and HTTP probes on `/healthz` (liveness) and `/readyz` (readiness).

## Building from source

```sh
make build            # npm ci + web client build, then the static Go binary in bin/qtestframe
make go               # only the Go binary (needs the web client already built)
make test             # Go and web unit tests
```

The build embeds the version from `git describe` (`qtestframe version`). `make web` writes the web client to `internal/webui/dist`, which the Go build embeds.

### With HSM support (PKCS#11)

PKCS#11 modules are C libraries, so the HSM key provider needs cgo. The default binary refuses `secrets.provider: pkcs11` and says how to build one that supports it:

```sh
make go-pkcs11        # CGO_ENABLED=1 go build -tags pkcs11 → bin/qtestframe
```

The result is linked dynamically against libc and loads the vendor's module at run time. Test the provider against SoftHSM2 with `make test-pkcs11` (`apt-get install softhsm2`), and the TPM provider against a software TPM with `make test-tpm` (`apt-get install swtpm`). How to configure both is in [Security](#key-providers).

# Configuration

## The configuration file

The configuration is one **YAML** file, given with `--config FILE` or the environment variable `QTF_CONFIG`. Every command reads the same file. Without a file the built-in defaults apply (HTTP on `:8080`, SQLite file `qtestframe.db` in the working directory).

Three files to start from:

| File | Use |
|---|---|
| `examples/qtestframe.yaml` | local trial: HTTP on `:8080`, SQLite in `./data`, script runner on, bootstrap administrator |
| `examples/qtestframe-tls.yaml` | the same with HTTPS on `:8443` and the certificate of `make certs-dev` |
| `deploy/config.example.yaml` | the commented production template that `install.sh` copies to `/etc/qtestframe/config.yaml` |

The file is decoded **strictly**: an unknown or misspelt key (`listn:`) and a second YAML document stop the program with an error that names the line. Check a file without starting the server, secret files included:

```sh
qtestframe check-config --config /etc/qtestframe/config.yaml
```

Durations are written as Go durations: `30s`, `15m`, `12h`, `1h30m`.

## Where a value comes from

Later sources win:

1. the built-in defaults (the *Default* column of the [reference](#reference));
2. the YAML file;
3. `QTF_*` environment variables ([list](#environment-variables));
4. command line flags of `serve`, `check-config` and `healthcheck`: `--listen HOST:PORT` replaces `server.listen`, `--port N` / `-p N` only its port;
5. for `serve`: the [runtime system settings](#runtime-system-settings) that super administrators changed in the web client, stored in the database – except settings that an environment variable sets or that `system_settings.locked` lists.

Secrets have their own rules, applied after the environment:

| Secret | Order (the last one that is set wins) |
|---|---|
| database DSN | `database.dsn`, `QTF_DATABASE_DSN`, **`database.dsn_file`** |
| bootstrap password | `bootstrap.admin_password`, `QTF_BOOTSTRAP_ADMIN_PASSWORD`, **`bootstrap.admin_password_file`** |
| SMTP password | `email.password_file`, a password stored in the web client, **`QTF_EMAIL_PASSWORD`** |
| OIDC client secret | `client_secret`, `client_secret_file`, a secret stored in the web client, **`QTF_OIDC_<NAME>_CLIENT_SECRET`** |
| HSM PIN | `secrets.pkcs11.pin_file`, **`QTF_SECRETS_PKCS11_PIN`** |
| local key | `secrets.local.key_file`, **`QTF_SECRETS_KEY`** (then no file is used) |

So a `*_file` option set in the file wins over the environment for the DSN and the bootstrap password, while for the other secrets the environment wins.

## Secrets in files

Do not write passwords into `config.yaml`. Use the `*_file` options: `database.dsn_file`, `auth.oidc[].client_secret_file`, `bootstrap.admin_password_file`, `email.password_file` and `secrets.pkcs11.pin_file`. Their content is read once at start-up and trimmed.

A **relative** path in one of these options is looked up in `$CREDENTIALS_DIRECTORY`, which systemd sets for `LoadCredential=`. The credentials live in a private, non-swappable directory that only the service can read:

```ini
# sudo systemctl edit qtestframe
[Service]
LoadCredential=db-dsn:/etc/qtestframe/secrets/db-dsn
LoadCredential=oidc-corp:/etc/qtestframe/secrets/oidc-corp
```

```yaml
database:
  driver: postgres
  dsn_file: db-dsn             # $CREDENTIALS_DIRECTORY/db-dsn
auth:
  oidc:
    - name: corp
      client_secret_file: oidc-corp
```

Paths of keys and certificates (`server.tls_key`, `auth.saml[].sp_key_file`, `logging.syslog.key_file`, …) are used as given. Environment variables work too (and are what containers use), but are visible in `/proc/<pid>/environ` to the same user; prefer files.

## After a change

| Changed | Takes effect |
|---|---|
| the YAML file or an environment variable | after `systemctl restart qtestframe` |
| the TLS certificate or key file | after `systemctl reload qtestframe` (`SIGHUP`); if the new pair cannot be read, the old one stays in use and an error is logged |
| a setting in **Administration → System settings** | at once, or after a restart for the runner (see [Runtime system settings](#runtime-system-settings)) |

## Reference

Every key of the file, its type, its default and the environment variable that sets it. *Type* `duration` takes values such as `30s` or `12h`; `list` is a YAML list; `map` a YAML mapping. Keys of the providers in `auth.oidc` and `auth.saml` are written with `[]`: they belong to each entry of the list.

<!-- BEGIN GENERATED: config-reference (make admin-guide-ref) -->

### `server`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `server.listen` | string | `:8080` | `QTF_SERVER_LISTEN`, `QTF_SERVER_PORT` | Address and port of the HTTP(S) listener, `host:port`; `:8080` listens on every address. Replaced by `--listen`, and the port alone by `--port`/`-p` or `QTF_SERVER_PORT`. |
| `server.tls_cert` | string | — | — | PEM file with the server certificate and its chain (leaf first). With `tls_key` it turns on HTTPS (TLS 1.2 or later); re-read on `SIGHUP`. |
| `server.tls_key` | string | — | — | PEM file with the private key of `tls_cert`. Both must be set, or neither. |
| `server.base_url` | string | — | `QTF_SERVER_BASE_URL` | The URL users type in the browser, without a trailing slash (`https://qtf.example.com`). Needed for single sign-on (callback and SAML entity URLs), for e-mail (links; required when e-mail is on) and for the origin check of the CSRF protection. |
| `server.read_timeout` | duration | `30s` | — | Longest time to read a whole request, body included (uploads, restores). |
| `server.write_timeout` | duration | `2m` | — | Longest time to write a response (downloads, exports, backups in the web client). Keep it above `runner.timeout`. |
| `server.trusted_proxies` | list | — | — | Addresses or CIDR ranges of reverse proxies. Only for requests from them is `X-Forwarded-For` used as the client address (its last entry), for logs, the audit trail and the sign-in limits. |
| `server.hsts` | bool | `true` | — | Send `Strict-Transport-Security` (two years, subdomains included) on HTTPS connections the server terminates itself. A TLS-terminating proxy must send it instead. |
| `server.frame_ancestors` | list | — | — | Origins (`https://doors.example.com`) of other tools that may show the OSLC selection and creation dialogs in a frame (`Content-Security-Policy: frame-ancestors`). Without any the dialogs open in a window of their own. The browser must also send the session cookie to the frame: both tools on the same site (subdomains of one domain). |

### `database`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `database.driver` | string | `sqlite` | `QTF_DATABASE_DRIVER` | `sqlite` (a file, nothing to install) or `postgres`. |
| `database.dsn` | string | `qtestframe.db` | `QTF_DATABASE_DSN` | SQLite: the database file (its directory is created with mode 0700). PostgreSQL: a connection URL such as `postgres://user:pass@host:5432/db?sslmode=verify-full`. |
| `database.dsn_file` | string | — | — | File that holds the DSN, so that the password is not in the configuration. A relative path is looked up in `$CREDENTIALS_DIRECTORY`. When set, it replaces `dsn` and `QTF_DATABASE_DSN`. |
| `database.max_open_conns` | integer | `20` | — | Size of the PostgreSQL connection pool. SQLite always uses one connection. |

### `logging`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `logging.level` | string | `info` | `QTF_LOG_LEVEL` | `debug`, `info`, `warn` or `error`. |
| `logging.format` | string | `text` | `QTF_LOG_FORMAT` | `text` (key=value) or `json`, written to standard error. |
| `logging.syslog.enabled` | bool | `false` | — | Also send every log record, the audit records included, to a syslog server. |
| `logging.syslog.network` | string | `udp` | — | `udp`, `tcp`, `tls` (RFC 5425), `unix` or `unixgram`. |
| `logging.syslog.address` | string | — | — | `host:port` of the syslog server, or the path of a Unix socket such as `/dev/log`. |
| `logging.syslog.tag` | string | `qtestframe` | — | APP-NAME (RFC 5424) or tag (RFC 3164) of the messages. |
| `logging.syslog.facility` | string | `local0` | — | Syslog facility, e.g. `local0` or `daemon`. |
| `logging.syslog.format` | string | `rfc5424` | — | `rfc5424` or `rfc3164`. The message is the log record as JSON. |
| `logging.syslog.ca_file` | string | — | — | `tls`: CA certificates that verify the syslog server (default: the system roots). |
| `logging.syslog.cert_file` | string | — | — | `tls`: client certificate for mutual TLS. |
| `logging.syslog.key_file` | string | — | — | `tls`: private key of `cert_file`. |

### `auth`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `auth.local` | bool | `true` | — | Allow sign-in with e-mail and password. Can only be turned off in the web client while a single sign-on provider is enabled. |
| `auth.session_ttl` | duration | `12h` | — | Absolute lifetime of a session (no sliding renewal). |
| `auth.cookie_secure` | bool | `true` | `QTF_COOKIE_SECURE` | Mark the session cookies `Secure`. Must be `false` when the server is reached over plain HTTP, else browsers do not keep the session cookie. |
| `auth.password_min_length` | integer | `10` | — | Minimum length of local passwords (8–72) set in the web client or the API. `user create` and the bootstrap administrator always use 10. |
| `auth.login_max_attempts` | integer | `10` | — | Sign-in attempts per client address and e-mail address within `login_lockout` (1–1000); further attempts get HTTP 429. |
| `auth.login_lockout` | duration | `5m` | — | Window of `login_max_attempts` (1m–24h). |

### `auth.oidc`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `auth.oidc` | list | — | — | OpenID Connect providers, each shown as a button on the sign-in page. Can be replaced in **Administration → System settings**. |
| `auth.oidc[].name` | string | — | — | Identifier of the provider in its URLs (`/api/v1/auth/oidc/<name>/callback`) and in `QTF_OIDC_<NAME>_CLIENT_SECRET`. Use lower-case letters, digits and `_`. |
| `auth.oidc[].enabled` | bool | `true` | — | `false` keeps the provider configured but hides it from the sign-in page. Default `true`. |
| `auth.oidc[].display_name` | string | — | — | Label of the button on the sign-in page. |
| `auth.oidc[].issuer` | string | — | — | Issuer URL; the server reads `<issuer>/.well-known/openid-configuration` at the first sign-in. Required. |
| `auth.oidc[].client_id` | string | — | — | Client ID registered at the provider. Required. |
| `auth.oidc[].client_secret` | string | — | `QTF_OIDC_<NAME>_CLIENT_SECRET` | Client secret written in the file. Prefer `client_secret_file` or the environment variable. |
| `auth.oidc[].client_secret_file` | string | — | — | File with the client secret (relative: in `$CREDENTIALS_DIRECTORY`). `QTF_OIDC_<NAME>_CLIENT_SECRET` wins over it. |
| `auth.oidc[].scopes` | list | `[openid, email, profile]` | — | Requested scopes. Default `[openid, email, profile]`; add `groups` where the provider needs it. |
| `auth.oidc[].email_claim` | string | `email` | — | Claim with the e-mail address (default `email`). A token with `email_verified: false` is refused. |
| `auth.oidc[].name_claim` | string | `name` | — | Claim with the display name (default `name`). |
| `auth.oidc[].groups_claim` | string | `groups` | — | Claim with the groups used by `role_mapping` (default `groups`). |
| `auth.oidc[].local_mfa` | string | `off` | — | Qtestframe's own second factor after this provider: `off` (default; the provider is responsible), `unless_idp_mfa` (unless `amr`/`acr` show that the provider used MFA) or `always`. |
| `auth.oidc[].mfa_amr_values` | list | `[mfa]` | — | `amr` values that count as MFA by the provider (default `[mfa]`). |
| `auth.oidc[].mfa_acr_values` | list | — | — | `acr` values that count as MFA by the provider (default none). |
| `auth.oidc[].auto_provision` | bool | `false` | — | Create unknown users at their first sign-in (and their membership in `default_tenant`). |
| `auth.oidc[].default_tenant` | string | — | — | Slug of the organisation whose membership the sign-in creates or updates. |
| `auth.oidc[].default_role` | string | `viewer` | — | Role when no group is mapped (an unknown or empty role means `viewer`). |
| `auth.oidc[].role_mapping` | map | — | — | Group → role. The most privileged mapped role wins and is set again at every sign-in. |

### `auth.saml`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `auth.saml` | list | — | — | SAML 2.0 identity providers (only in the file). |
| `auth.saml[].name` | string | — | — | Identifier in the URLs `/api/v1/auth/saml/<name>/{metadata,login,acs}`. |
| `auth.saml[].display_name` | string | — | — | Label of the button on the sign-in page. |
| `auth.saml[].idp_metadata_url` | string | — | — | URL of the IdP metadata, fetched at the first sign-in. This or `idp_metadata_file` is required. |
| `auth.saml[].idp_metadata_file` | string | — | — | File with the IdP metadata (no outgoing request needed). |
| `auth.saml[].entity_id` | string | the SP metadata URL | — | Entity ID of Qtestframe (default: the SP metadata URL `<base_url>/api/v1/auth/saml/<name>/metadata`). |
| `auth.saml[].sp_cert_file` | string | — | — | Certificate of the SP key pair (`scripts/certs/gen-saml-sp-cert.sh`), published in the SP metadata. |
| `auth.saml[].sp_key_file` | string | — | — | Private key of the SP; decrypts encrypted assertions. |
| `auth.saml[].email_attr` | string | `email` | — | Attribute with the e-mail address (default `email`; else a NameID that contains `@`). |
| `auth.saml[].name_attr` | string | `displayName` | — | Attribute with the display name (default `displayName`). |
| `auth.saml[].groups_attr` | string | `groups` | — | Attribute with the groups (default `groups`). |
| `auth.saml[].auto_provision` | bool | `false` | — | As for OIDC. |
| `auth.saml[].default_tenant` | string | — | — | As for OIDC. |
| `auth.saml[].default_role` | string | `viewer` | — | As for OIDC. |
| `auth.saml[].role_mapping` | map | — | — | As for OIDC. |

### `runner`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `runner.enabled` | bool | `false` | `QTF_RUNNER_ENABLED` | Allow running automated test scripts on the server. Read the security chapter first. |
| `runner.timeout` | duration | `1m` | — | Wall-clock limit of one script; the process group is then killed. |
| `runner.work_dir` | string | `$TMPDIR/qtestframe-runner` | `QTF_RUNNER_WORK_DIR` | Directory of the job directories (created with mode 0700). Keep the path short: Chromium's socket path must stay below 108 bytes. |
| `runner.max_output_bytes` | integer | `1048576` | — | Maximum stored output (stdout and stderr together) of one script. |
| `runner.interpreters` | map | `bash: /bin/bash`, `javascript: /usr/bin/node`, `python: /usr/bin/python3`, `sh: /bin/sh` | — | Script language → interpreter. Remove the languages you do not need. |
| `runner.sandbox` | bool | `true` | — | Start scripts through `qtestframe sandbox-exec` (rlimits, no_new_privs, Landlock, seccomp). Only in the file. |
| `runner.deny_network` | bool | `false` | — | Block socket creation for sandboxed scripts (not for web tests). |

### `runner.web`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `runner.web.enabled` | bool | `false` | `QTF_RUNNER_WEB_ENABLED` | Web tests with Playwright in a real browser. Needs `runner.enabled`. |
| `runner.web.node` | string | `/usr/bin/node` | `QTF_RUNNER_WEB_NODE` | Node.js binary. |
| `runner.web.playwright_dir` | string | — | `QTF_RUNNER_WEB_PLAYWRIGHT_DIR` | Directory that contains `node_modules/@playwright/test`. Required (unless `command` is set). |
| `runner.web.command` | string | — | — | Command that replaces `<node> <playwright_dir>/node_modules/@playwright/test/cli.js`; `test --config <file>` is appended. Split at spaces. |
| `runner.web.browsers_path` | string | — | `QTF_RUNNER_WEB_BROWSERS_PATH` | `PLAYWRIGHT_BROWSERS_PATH`, where `npx playwright install` put the browsers (empty: Playwright's default). |
| `runner.web.browser` | string | `chromium` | — | Default browser: `chromium`, `firefox` or `webkit`. A test configuration with a Browser dimension picks another. |
| `runner.web.channel` | string | — | — | Chromium channel, e.g. `chrome` or `msedge` (empty: the bundled Chromium). |
| `runner.web.browser_channels` | bool | `false` | — | Configurations named Chrome or Edge use the installed Google Chrome or Microsoft Edge. |
| `runner.web.executable_path` | string | — | — | Chromium executable to launch instead of Playwright's. |
| `runner.web.headless` | bool | `true` | — | Run the browser without a window. |
| `runner.web.timeout` | duration | `1m` | — | Playwright's per-test timeout. |
| `runner.web.job_timeout` | duration | `10m` | — | Limit of a whole Playwright process. |
| `runner.web.screenshots` | string | `on_failure` | — | `on_failure`, `always` or `off`. |
| `runner.web.trace` | string | `on_failure` | — | `on_failure`, `always` or `off`. |
| `runner.web.video` | string | `off` | — | `off`, `on_failure` or `always`. |
| `runner.web.max_parallel` | integer | `2` | — | Background jobs (web tests and *Run automated tests*) running at the same time per server process (1–32). |
| `runner.web.max_evidence_mb` | integer | `0` (= `files.max_upload_mb`) | — | Largest screenshot, trace or video kept; larger files are left out (0: `files.max_upload_mb`). |

### `files`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `files.max_upload_mb` | integer | `25` | — | Largest uploaded file in MB (1–1024). Files are stored in the database. |

### `requirements`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `requirements.auto_create_test` | bool | `true` | — | Create a linked draft test with every new requirement (the request can opt out). |

### `hardening`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `hardening.no_new_privs` | bool | `true` | — | Set `PR_SET_NO_NEW_PRIVS` after start-up. The only hardening whose failure stops the server. |
| `hardening.landlock` | bool | `true` | — | Restrict file access with Landlock (Linux 5.13+); skipped with a log entry where unsupported. |
| `hardening.seccomp` | bool | `true` | — | Install a seccomp deny-list of dangerous system calls. |
| `hardening.drop_capabilities` | bool | `true` | — | Drop every capability except `CAP_NET_BIND_SERVICE`. |
| `hardening.read_only_paths` | list | — | — | Further paths the server may read after the lock-down. |
| `hardening.read_write_paths` | list | — | — | Further paths the server may write after the lock-down (e.g. `/run/postgresql`, a SoftHSM token directory). |

### `bootstrap`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `bootstrap.tenant_slug` | string | `default` | — | Organisation created for the first administrator (and used by `seed-demo`). |
| `bootstrap.tenant_name` | string | `Default` | — | Name of that organisation. |
| `bootstrap.admin_email` | string | — | `QTF_BOOTSTRAP_ADMIN_EMAIL` | E-mail of the first administrator, created as super admin and admin of `tenant_slug` only while the database has no users. |
| `bootstrap.admin_password` | string | — | `QTF_BOOTSTRAP_ADMIN_PASSWORD` | Its password (at least 10 characters with a letter and a non-letter). Prefer `admin_password_file`. |
| `bootstrap.admin_password_file` | string | — | — | File with that password (relative: in `$CREDENTIALS_DIRECTORY`). When set, it replaces `admin_password` and `QTF_BOOTSTRAP_ADMIN_PASSWORD`. |

### `email`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `email.enabled` | bool | on while `host` is set | — | `false` keeps the settings but sends nothing. |
| `email.host` | string | — | `QTF_EMAIL_HOST` | SMTP server. E-mail is off while it is empty. |
| `email.port` | integer | `587` | `QTF_EMAIL_PORT` | SMTP port: 587 for STARTTLS, 465 for implicit TLS. |
| `email.username` | string | — | `QTF_EMAIL_USERNAME` | User name for SMTP authentication (leave out for a relay without authentication). |
| `email.password_file` | string | — | — | File with the SMTP password (relative: in `$CREDENTIALS_DIRECTORY`). `QTF_EMAIL_PASSWORD` wins over it. |
| `email.from` | string | — | `QTF_EMAIL_FROM` | Sender address, a plain address without a name. |
| `email.from_name` | string | `Qtestframe` | — | Sender name. |
| `email.tls` | string | `starttls` | — | `starttls`, `tls` (implicit) or `none` (a local relay only). |
| `email.timeout` | duration | `30s` | — | Limit for sending one message. |
| `email.digest_hour` | integer | `7` | — | Hour of the day (UTC, 0–23) when daily digests are sent. |

### `system_settings`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `system_settings.locked` | list | — | — | Settings that cannot be changed in **Administration → System settings**: keys (`logging.level`), prefixes ending in `.*` (`email.*`) or `*`. Their stored overrides are ignored. |

### `secrets`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `secrets.provider` | string | `local` | `QTF_SECRETS_PROVIDER` | Key provider that protects the stored secrets: `local`, `tpm` or `pkcs11`. |
| `secrets.local.key_file` | string | `qtestframe-secrets.key` next to a SQLite database, else in `$STATE_DIRECTORY`, `/var/lib/qtestframe` or the working directory | `QTF_SECRETS_KEY_FILE` | Key file of the local provider (256-bit key, created with mode 0600 when missing). `QTF_SECRETS_KEY` replaces the file. |
| `secrets.tpm.device` | string | `/dev/tpmrm0` | `QTF_SECRETS_TPM_DEVICE` | TPM resource manager, or the Unix socket of a TPM emulator (`swtpm`). |
| `secrets.tpm.pcrs` | list | — | — | PCRs the sealed key is bound to, e.g. `[7]` (Secure Boot state); unsealing fails after they change. |
| `secrets.tpm.pcr_bank` | string | `sha256` | — | PCR bank of the policy: `sha256` or `sha1`. |
| `secrets.pkcs11.module` | string | — | `QTF_SECRETS_PKCS11_MODULE` | PKCS#11 library of the HSM, e.g. `/usr/lib/softhsm/libsofthsm2.so`. Needs a binary built with `make go-pkcs11`. |
| `secrets.pkcs11.token_label` | string | — | — | Label of the token; this or `slot` is required. |
| `secrets.pkcs11.slot` | integer | none | — | Slot number, used instead of `token_label`. |
| `secrets.pkcs11.pin_file` | string | — | — | File with the user PIN (relative: in `$CREDENTIALS_DIRECTORY`). `QTF_SECRETS_PKCS11_PIN` wins over it. |
| `secrets.pkcs11.key_label` | string | `qtestframe-kek` | — | `CKA_LABEL` of the AES key that wraps the data keys; must be unique in the token. |
| `secrets.pkcs11.generate_key` | bool | `false` | — | Create a non-extractable AES-256 key with `key_label` when the token has none. |

### `similarity`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `similarity.embeddings.enabled` | bool | `false` | — | Use an embeddings service for the similarity of requirements and tests (suspected duplicates, the *Similar requirements* panel). Off by default: the similarity is then computed on the server. **When on, the title, description and criteria or steps of the items are sent to that service.** Also in **Administration → System settings**. |
| `similarity.embeddings.url` | string | — | — | URL of an OpenAI-compatible embeddings endpoint, e.g. `https://llm.example.com/v1/embeddings` (`POST {model, input}`, answer `{data: [{embedding}]}`). |
| `similarity.embeddings.model` | string | — | — | Model sent as `model` (empty: the service's default). Vectors are cached per item version and model; changing the model computes them again. |
| `similarity.embeddings.api_key_file` | string | — | — | File with the API key, sent as a bearer token (relative: in `$CREDENTIALS_DIRECTORY`). The key can also be stored in the system settings; it is never shown again. |
| `similarity.embeddings.timeout` | duration | `30s` | — | Longest time to wait for the service. When it fails or times out, the local similarity is used. |

### `live`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `live.enabled` | bool | `true` | `QTF_LIVE_ENABLED` | Live updates and presence: lists, dashboards, the notification bell and open items update when someone else changes something, and editors show who else is viewing or editing an item (a Server-Sent Events stream, `GET /api/v1/events/stream`). Off: the web client falls back to polling and to the conflict check on save. Each user can also turn it off in the profile. Behind a reverse proxy, turn off response buffering for the stream (see *Live updates* in the chapter *Features added later*). |
| `live.heartbeat` | duration | `25s` | — | Interval of the keep-alive comments on an idle stream. Keep it below the idle timeout of proxies and load balancers (nginx `proxy_read_timeout`, 60 s by default). |
| `live.max_connections` | integer | `2000` | — | Most open event streams of this server instance; further connections are refused with 503 and the clients poll instead. |
| `live.max_connections_per_user` | integer | `8` | — | Most open event streams of one user (one per browser tab; a tab in the background closes its stream after a minute). |
| `live.replay_buffer` | integer | `512` | — | Number of recent events kept in memory so that a client that reconnects (`Last-Event-ID`) receives what it missed; when its last event is no longer kept, it reloads what it shows instead. |
| `live.presence_ttl` | duration | `45s` | — | How long a user stays listed as viewing or editing an item after the last presence heartbeat of their browser (sent every 20 s). |

<!-- END GENERATED: config-reference -->

## Environment variables

<!-- BEGIN GENERATED: environment (make admin-guide-ref) -->

| Variable | Sets |
|---|---|
| `QTF_CONFIG` | the configuration file when `--config` is not given |
| `QTF_BOOTSTRAP_ADMIN_EMAIL` | `bootstrap.admin_email` |
| `QTF_BOOTSTRAP_ADMIN_PASSWORD` | `bootstrap.admin_password` (replaced by `bootstrap.admin_password_file` when that is set) |
| `QTF_COOKIE_SECURE` | `auth.cookie_secure` (`1` or `true` = on, anything else off) |
| `QTF_DATABASE_DRIVER` | `database.driver` |
| `QTF_DATABASE_DSN` | `database.dsn` (replaced by `database.dsn_file` when that is set) |
| `QTF_EMAIL_FROM` | `email.from` |
| `QTF_EMAIL_HOST` | `email.host` |
| `QTF_EMAIL_PASSWORD` | the SMTP password; wins over `email.password_file` and a password stored in the web client |
| `QTF_EMAIL_PORT` | `email.port` (ignored when not a number) |
| `QTF_EMAIL_USERNAME` | `email.username` |
| `QTF_LIVE_ENABLED` | `live.enabled` (`1` or `true` = on, anything else = off) |
| `QTF_LOG_FORMAT` | `logging.format` |
| `QTF_LOG_LEVEL` | `logging.level` |
| `QTF_RUNNER_ENABLED` | `runner.enabled` (`1` or `true` = on) |
| `QTF_RUNNER_WEB_BROWSERS_PATH` | `runner.web.browsers_path` |
| `QTF_RUNNER_WEB_ENABLED` | `runner.web.enabled` (`1` or `true` = on) |
| `QTF_RUNNER_WEB_NODE` | `runner.web.node` |
| `QTF_RUNNER_WEB_PLAYWRIGHT_DIR` | `runner.web.playwright_dir` |
| `QTF_RUNNER_WORK_DIR` | `runner.work_dir` |
| `QTF_SECRETS_KEY` | the key of the local provider itself (32 bytes, base64 or hex; `qtestframe secrets generate-key`); no key file is used then |
| `QTF_SECRETS_KEY_FILE` | `secrets.local.key_file` |
| `QTF_SECRETS_PKCS11_MODULE` | `secrets.pkcs11.module` |
| `QTF_SECRETS_PKCS11_PIN` | the HSM user PIN; wins over `secrets.pkcs11.pin_file` |
| `QTF_SECRETS_PROVIDER` | `secrets.provider` |
| `QTF_SECRETS_TPM_DEVICE` | `secrets.tpm.device` |
| `QTF_SERVER_BASE_URL` | `server.base_url` |
| `QTF_SERVER_LISTEN` | `server.listen` |
| `QTF_SERVER_PORT` | the port of `server.listen` (the host stays) |
| `QTF_OIDC_<NAME>_CLIENT_SECRET` | client secret of the OIDC provider `<name>` (upper case); wins over `client_secret_file` and a secret stored in the web client |
| `CREDENTIALS_DIRECTORY` | set by systemd (`LoadCredential=`): relative `*_file` paths are resolved in it, and it is readable after the lock-down |
| `STATE_DIRECTORY` | set by systemd (`StateDirectory=`): default place of the key file when the database is not SQLite |
| `QTF_URL`, `QTF_TOKEN`, `QTF_CONFIGURATION` | defaults of `--url`, `--token` and `--configuration` of `qtestframe report` |

<!-- END GENERATED: environment -->

Boolean variables are on for `1` or `true` (any case) and off for every other value. `QTF_SERVER_PORT` only changes the port when `server.listen` is a valid `host:port`.

# First start

## What happens at start-up

`qtestframe serve` does, in this order:

1. load and validate the configuration (an invalid one stops it, with every error listed);
2. open the database and apply the pending [migrations](#migrations);
3. open the key provider of the stored secrets, and encrypt secrets that an earlier version stored as plaintext (a missing or wrong key stops it here);
4. create the **bootstrap administrator** if the database has no users;
5. apply the system settings stored in the database;
6. open the listener and the TLS certificate, then **lock the process down** (no_new_privs, capabilities, Landlock, seccomp) and log one `hardening` line;
7. start the background jobs and log `qtestframe listening` with the address, TLS, version and database driver.

## The first administrator

There are two ways to get the first account.

**Bootstrap in the configuration.** While the database has **no users at all**, the server creates a user from the `bootstrap` section, makes it a **super administrator** and **admin** of the organisation `bootstrap.tenant_slug` (created if missing, default `default` / *Default*):

```yaml
bootstrap:
  tenant_slug: acme
  tenant_name: "Acme Corp"
  admin_email: admin@acme.example
  admin_password_file: /etc/qtestframe/secrets/admin-password
```

The password must have at least 10 characters with at least one letter and one non-letter; a weaker one stops the start. Once any user exists, the section is ignored, so it can stay in the file, but remove the password file once the administrator has signed in and changed the password. Without bootstrap settings the server logs `no users exist; configure bootstrap.admin_email/admin_password or run qtestframe user create`.

**On the command line**, as the service user:

```sh
alias qtf='sudo -u qtestframe env QTF_CONFIG=/etc/qtestframe/config.yaml /usr/bin/qtestframe'
read -rs PW && printf '%s\n' "$PW" | qtf user create --email admin@acme.example --name "Admin" --password-stdin --super-admin
qtf tenant create --slug acme --name "Acme Corp"
qtf member add --tenant acme --email admin@acme.example --role admin
```

The password is read from standard input, never from the command line (where `ps` would show it). A user created without `--password-stdin` has no password and can only sign in through single sign-on.

## Organisations

An **organisation** (tenant) is an isolated workspace: projects, members, roles, API tokens, SCIM tokens, MFA policy and audit log belong to one organisation and are invisible to the others. A user account belongs to the installation and can be a member of several organisations, with one role in each; the user switches between them in the web client.

- Create organisations under **Administration → Organisations** (super administrators) or with `qtestframe tenant create --slug S --name N`. The slug is 2–63 characters of `a-z`, `0-9` and `-`; users can type it at sign-in.
- Give people roles under **Administration → Members** (organisation admins) or with `qtestframe member add`.
- Deleting an organisation deletes all its projects and data and cannot be undone (only a [restore](#restore) brings it back).

Single sign-on can put new users straight into an organisation (`default_tenant`, `role_mapping`, see [Authentication](#provisioning-of-single-sign-on-users)), and SCIM provisions the members of one organisation per token.

## Demo data

```sh
qtf seed-demo                 # into bootstrap.tenant_slug
qtf seed-demo --tenant acme
```

`seed-demo` adds the sample project *Demo – Online Banking* (key `DEMO`, then `DEMO2`, … when it already exists) with requirements, tests, links, runs with history, a release, a demo web test, and a restricted project shared with one of two demo users, `dana@example.com` and `erik@example.com`, whose password is `Demo-user-1234`. It runs the bootstrap first, so the demo users never take the place of the first administrator. The demo web test opens the server's own sign-in page at `server.base_url`.

**Never run `seed-demo` on a production database**: it creates accounts with a published password. Use a separate database for trials and training.

## Checklist after the first start

- Sign in as the bootstrap administrator and **change the password** (user menu → **Profile & password**); set up an authenticator app.
- Set `server.base_url` to the address users type; it is needed for single sign-on and e-mail.
- Back up the **key file** of the stored secrets (`/var/lib/qtestframe/qtestframe-secrets.key` on a SQLite installation) to a place apart from the database backups. See [Security](#encryption-of-stored-secrets).
- Decide on single sign-on, an [MFA policy](#multi-factor-authentication), e-mail and syslog.
- Set up regular [backups](#backup-restore-and-migration) and try a restore.

# Authentication

Qtestframe knows these ways to prove who someone is:

| Method | For | Configured in |
|---|---|---|
| Local password | people | `auth.local`, password policy |
| Second factor (TOTP authenticator app, recovery codes) | people with a local password, optionally after OIDC | MFA policy in the web client; `local_mfa` per OIDC provider |
| OpenID Connect | people | `auth.oidc` (file or system settings) |
| SAML 2.0 | people | `auth.saml` (file only) |
| E-mail invitation | a person's first sign-in | needs e-mail |
| API token (`qtf_…`) | scripts and CI | **Administration → API tokens** |
| SCIM token (`qtfscim_…`) | an identity provider's provisioning | **Administration → Provisioning (SCIM)** |

## Local passwords

- Passwords are hashed with **bcrypt** (cost 12). The policy is: at least `auth.password_min_length` characters (default 10, at most 72 bytes, because bcrypt ignores the rest), with at least one letter and one character that is not a letter. `qtestframe user create` and the bootstrap administrator always check against 10.
- Unknown users, wrong passwords, disabled accounts and accounts without a password all get the same answer (`401 invalid credentials`), and take the same time.
- **Sign-in limit:** at most `auth.login_max_attempts` attempts (default 10) per *client address + e-mail address* within `auth.login_lockout` (default 5 minutes); more get `429 rate_limited`. A successful sign-in clears the counter. The counter is in memory, per server process. Behind a reverse proxy set `server.trusted_proxies`, or every user shares the proxy's address.
- Failed sign-ins are logged at WARN (`login failed`, with e-mail and client address) and recorded in the user's sign-in history, which the web client shows as "failed attempts since your last sign-in".
- **Disable local sign-in** with `auth.local: false` once single sign-on works. In the web client this is only possible while a single sign-on provider is enabled. Super administrators can still be managed on the command line.
- Organisation admins reset the passwords of members who belong only to their organisation (**Administration → Members → Reset password**: a temporary password, shown once, that must be changed at the next sign-in); super administrators reset anyone's under **Users**. Both sign the user out everywhere.

## Multi-factor authentication

The second factor is a **TOTP** code from an authenticator app (RFC 6238: HMAC-SHA-1, 6 digits, 30-second steps, ±1 step of clock drift), with **10 single-use recovery codes**. Users set it up under their profile. Each code works only once (the last used step is stored), so at most one sign-in per 30 seconds per user.

**Policies** decide who must use it (**Administration → Multi-factor authentication**, see the [user guide](https://github.com/rom/Qtestframe/blob/rom/docs/user-guide/en/administration.md#multi-factor-authentication)):

| Policy | Set by | Applies to |
|---|---|---|
| Server policy | super administrators | every user; roles count in any organisation, plus the pseudo-role *super admin* |
| Organisation policy | organisation admins (`tenant.manage`) | the members of that organisation by their role there |

Each is *optional* (default), *required for everyone* or *required for some roles*. An organisation can require more than the server, never less. A **grace period** (days) starts when a policy begins to require MFA for more people, or at the user's creation if that is later; afterwards a user without a second factor must set one up before anything else works (`403 mfa_enrolment_required`). The policy is evaluated at sign-in, so a stricter policy reaches existing sessions at their next sign-in.

**Remember this device** (server policy, *days*, default 0 = off) lets a browser skip the code – never the password – for that many days. The token is revocable on the server; users can forget their devices, and an administrator reset or turning MFA off forgets them too.

Guessing is limited three ways: every code counts against the sign-in limit above (which the correct password alone does not reset), an account allows 10 code attempts per 15 minutes from any address, and a pending sign-in is dropped after 5 wrong codes.

**Lost phone and recovery codes:** an organisation admin uses **Reset MFA** in **Members** (for members of only that organisation), a super administrator under **Users**. It removes the authenticator, recovery codes and remembered devices, signs the user out and tells them by notification and e-mail. Verify the request out of band first.

The TOTP secrets are stored encrypted (see [Security](#encryption-of-stored-secrets)). API tokens and SCIM tokens are not affected by MFA; protect them like passwords. WebAuthn and passkeys are not implemented.

## OpenID Connect

Register a **confidential web client** at the provider with:

- redirect URI `<server.base_url>/api/v1/auth/oidc/<name>/callback`,
- grant type *authorization code* (Qtestframe always uses PKCE with S256),
- scopes `openid email profile`, plus `groups` if the provider offers it.

Then add the provider:

```yaml
auth:
  oidc:
    - name: corp
      display_name: "Corporate login"
      issuer: "https://sso.example.com/realms/acme"
      client_id: qtestframe
      client_secret_file: oidc-corp            # or QTF_OIDC_CORP_CLIENT_SECRET
      scopes: [openid, email, profile]
      groups_claim: groups
      auto_provision: true
      default_tenant: acme
      default_role: viewer
      role_mapping: {"qtf-admins": admin, "qtf-testers": tester}
```

or add it in **Administration → System settings → Single sign-on (OIDC)**, which can test the discovery document. Every key is in the [reference](#authoidc). Provider-specific steps for Keycloak, Microsoft Entra ID and Okta are in [docs/operations.md](https://github.com/rom/Qtestframe/blob/rom/docs/operations.md#6-single-sign-on-oidc).

What Qtestframe checks: state, nonce and the PKCE verifier are kept in a signed, single-use cookie that is valid for 10 minutes; the ID token signature is verified against the provider's keys, and issuer, audience and expiry are checked. The token must carry an e-mail address (`email_claim`); `email_verified: false` is refused. After the sign-in the browser always goes to `/`.

The server needs outgoing HTTPS to the issuer. With SELinux: `setsebool -P qtestframe_connect_http on`. Name providers with lower-case letters, digits and `_`: the name is part of the URL and, upper-cased, of `QTF_OIDC_<NAME>_CLIENT_SECRET` (a `-` would make that variable hard to set).

### Local MFA and amr/acr

By default the identity provider is responsible for MFA, and Qtestframe's own second factor is not asked for after an OIDC sign-in (`local_mfa: off`). Two stricter settings:

| `local_mfa` | Qtestframe asks for its own code |
|---|---|
| `off` (default) | never |
| `unless_idp_mfa` | unless the ID token shows MFA: its `amr` claim contains `mfa` or a value of `mfa_amr_values` (e.g. `[mfa, otp, hwk]`), or its `acr` is one of `mfa_acr_values` |
| `always` | on every sign-in through this provider |

When the code is asked for, users need an authenticator app in Qtestframe too (or must set one up if the MFA policy requires it). The reported `amr`/`acr` values are written to the audit entry of the sign-in. SAML sign-ins never use the local second factor; configure MFA at the identity provider.

## SAML 2.0

Create a key pair for Qtestframe as service provider and point the IdP at the SP metadata:

```sh
sudo scripts/certs/gen-saml-sp-cert.sh --cn qtf.example.com --name saml-adfs --out /etc/qtestframe/tls --group qtestframe
```

```yaml
auth:
  saml:
    - name: adfs
      display_name: "Corporate login (ADFS)"
      idp_metadata_url: "https://adfs.example.com/FederationMetadata/2007-06/FederationMetadata.xml"
      sp_cert_file: /etc/qtestframe/tls/saml-adfs.crt
      sp_key_file: /etc/qtestframe/tls/saml-adfs.key
      email_attr: "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress"
      groups_attr: "http://schemas.xmlsoap.org/claims/Group"
      auto_provision: true
      default_tenant: acme
      role_mapping: {"QTF-Admins": admin}
```

| URL | |
|---|---|
| `<base_url>/api/v1/auth/saml/<name>/metadata` | SP metadata (also the default entity ID) |
| `<base_url>/api/v1/auth/saml/<name>/acs` | assertion consumer service (HTTP-POST) |

Only **SP-initiated** sign-in is supported; IdP-initiated (unsolicited) responses are refused. The response or every assertion must be signed by the IdP certificate from the metadata; destination, audience, recipient and the validity window are checked. AuthnRequests are not signed, so turn off *client signature required* at the IdP. Encrypted assertions are decrypted with `sp_key_file`. SAML providers are configured only in the file and read `server.base_url` at start-up. The metadata URL is fetched at the first sign-in; `idp_metadata_file` needs no outgoing connection.

## Provisioning of single sign-on users

At every OIDC or SAML sign-in:

1. The user is found by provider and subject. Otherwise by e-mail: a local account (or one created by SCIM) with that address is **linked** to the provider and its password removed; an account bound to *another* provider is refused. Otherwise a new user is created only with `auto_provision: true`.
2. Disabled users are refused.
3. With `default_tenant`, the role is the most privileged role that `role_mapping` gives one of the user's groups, else `default_role` (`viewer` when empty or unknown). A missing membership is created with `auto_provision`; with a `role_mapping`, an existing membership is set to the mapped role at every sign-in – the identity provider is authoritative.

Because accounts are linked by e-mail address, connect only identity providers whose addresses are verified and belong to your organisation. Members that SCIM manages are left to SCIM.

## SCIM provisioning

With SCIM 2.0 an identity provider (Microsoft Entra ID, Okta and others) creates, updates and deactivates the members of **one organisation**. The organisation admin creates a SCIM token under **Administration → Provisioning (SCIM)** and gives the provider the base URL `<base_url>/scim/v2` and the token; the [user guide](https://github.com/rom/Qtestframe/blob/rom/docs/user-guide/en/administration.md#provisioning-with-scim) describes the provider screens.

What the server administrator should know:

- A SCIM token (`qtfscim_` + 256 random bits) is stored as a hash, shown once, may expire, and authenticates only `/scim/v2` of its organisation. It controls that organisation's membership up to the admin role (through a group mapped to admin), so treat it like an admin password.
- It cannot change super administrator status, passwords, project data or other organisations; an organisation always keeps one admin.
- Provisioned accounts have no password: they sign in through OIDC or SAML, which links them by e-mail.
- Deactivating a user removes the membership and ends the user's sessions and API tokens in that organisation.
- Limits: 1200 requests per minute per token; 20 failed authentications per 5 minutes block the client address (`429`). Every change is audited with the actor `SCIM: <token name>`.

## API tokens

API tokens are for scripts and CI pipelines (`Authorization: Bearer qtf_…`). Members with `token.manage` (Tester and up by default) create them under **Administration → API tokens**.

- A token is `qtf_` + 32 random bytes; only its SHA-256 hash and a short prefix are stored, and it is shown once.
- It belongs to **one organisation** and has a role that cannot exceed its creator's. On every request its permissions are **capped by the creator's current role**: demoting the user demotes the token, removing the member ends it.
- Tokens cannot create tokens, never carry super administrator rights (not even a super administrator's), cannot use the system settings and are not subject to MFA.
- Optional expiry in days; the last use is recorded; revoking is immediate.
- Requests with a token skip the CSRF check (there is no cookie), so the token is the only credential: keep it in the CI system's secret store.

## Sessions

- A session is created at every sign-in. The browser holds only the random cookie `qtf_session` (`HttpOnly`, `SameSite=Lax`, `Secure` unless `auth.cookie_secure: false`); the database stores a SHA-256 hash of it.
- Sessions end after `auth.session_ttl` (default 12 h, absolute, no extension), at sign-out, and when an administrator disables the user, resets the password or resets MFA. Changing one's own password ends the user's other sessions. Expired sessions are purged hourly.
- Users see their sessions (browser, address, last activity) in their profile and can end each one.
- Requests from the browser that change something must send the per-session CSRF token and pass an origin check against the request host or `server.base_url`.
- Sessions are not part of a backup; a restore signs everyone out.

# Authorisation

## The model

- Every member holds **one or more roles per organisation**; without a membership a user sees nothing of an organisation. Someone who both writes requirements and tests holds, for example, *Subject matter expert* and *Tester*.
- A role is a named set of **permissions**; a member with several roles has the **union** of their permissions (roles only add rights). Seven roles are built in: Admin, Manager, Test lead, Tester, Subject matter expert (SME), Reviewer and Viewer.
- Holding Admin among one's roles makes a member an organisation administrator. Older API clients see one `role`, the *primary* role: the strongest built-in role of the member (Admin whenever the member holds it).
- **Super administrator** is a flag of the user account, set with `qtestframe user create --super-admin` or under **Administration → Users**. A super administrator holds every permission in every organisation (and acts as admin in an organisation without being a member), and alone may manage organisations and users, back up and restore, and change the system settings. This applies to signed-in sessions only: API tokens never carry it.
- Permissions are checked by the server on every request; the web client only hides what you may not do. An object of another organisation, or of a project you may not see, answers `404`, so its existence is not revealed.

## The default permission matrix

These are the permissions of the built-in roles until an organisation changes them. Admin always holds every permission and cannot be changed.

<!-- BEGIN GENERATED: permissions (make admin-guide-ref) -->

| Permission | Viewer | Reviewer | SME | Tester | Test lead | Manager | Admin |
|---|:-:|:-:|:-:|:-:|:-:|:-:|:-:|
| `project.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `project.create` |  |  |  |  |  | ✔ | ✔ |
| `project.update` |  |  |  |  |  | ✔ | ✔ |
| `project.delete` |  |  |  |  |  |  | ✔ |
| `requirement.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `requirement.write` |  |  | ✔ |  |  | ✔ | ✔ |
| `requirement.review` |  | ✔ | ✔ |  |  | ✔ | ✔ |
| `waiver.approve` |  |  |  |  |  | ✔ | ✔ |
| `change.create` |  |  | ✔ |  | ✔ | ✔ | ✔ |
| `change.approve` |  |  |  |  | ✔ | ✔ | ✔ |
| `test.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `test.write` |  |  |  | ✔ | ✔ | ✔ | ✔ |
| `test.review` |  | ✔ |  |  | ✔ | ✔ | ✔ |
| `run.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `run.write` |  |  |  | ✔ | ✔ | ✔ | ✔ |
| `run.execute` |  |  |  | ✔ | ✔ | ✔ | ✔ |
| `cycle.plan` |  |  |  |  | ✔ | ✔ | ✔ |
| `script.execute` |  |  |  |  | ✔ | ✔ | ✔ |
| `comment.create` |  | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `export` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `audit.read` |  |  |  |  | ✔ | ✔ | ✔ |
| `token.manage` |  |  |  | ✔ | ✔ | ✔ | ✔ |
| `member.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `member.manage` |  |  |  |  |  |  | ✔ |
| `role.manage` |  |  |  |  |  |  | ✔ |
| `settings.manage` |  |  |  |  | ✔ | ✔ | ✔ |
| `tenant.manage` |  |  |  |  |  |  | ✔ |

<!-- END GENERATED: permissions -->

### The subject matter expert

The **Subject matter expert** (`sme`) is a domain expert who takes part in writing the requirements of their field – a clinician, a safety engineer, a lawyer. The role:

- creates and edits requirements (`requirement.write`): text, acceptance criteria, rationale, sources, relations, trace and item links and attachments;
- takes part in requirement reviews (`requirement.review`) and discussions (`comment.create`);
- reads tests, runs, defects and risks for context, and exports and reports (`export`).

It deliberately does **not** write or review tests, create or execute runs, plan cycles, approve waivers or accept residual risks, change project settings, sharing or members, or create API tokens. Evidence on test executions is recorded by testers; an SME attaches supporting documents to the requirements themselves. Give a second role to someone who also does more, e.g. *Subject matter expert* + *Tester*.

Because the role both writes and reviews requirements, an SME can approve their own requirements. Where separation of duties matters, remove `requirement.review` from the role in the matrix (or give it only to separate reviewers).

What the permissions allow:

| Permission | Allows |
|---|---|
| `project.read`, `requirement.read`, `test.read`, `run.read` | reading projects and their requirements, tests and runs |
| `project.create`, `project.update`, `project.delete` | creating projects, changing their settings (also trackers, webhooks, web test settings and sharing), deleting them |
| `requirement.write`, `test.write` | editing requirements and tests |
| `requirement.review`, `test.review` | review decisions: approving or rejecting requirements, marking tests ready |
| `run.write`, `run.execute` | creating and editing runs; recording results, CI imports, pushing defects to trackers |
| `script.execute` | running scripts and web tests **on the server** |
| `export` | exports and reports |
| `audit.read` | the organisation's audit log |
| `token.manage` | own API tokens |
| `member.read`, `member.manage` | the member list; adding, changing and removing members, resetting their passwords and MFA |
| `role.manage` | the role matrix and custom roles |
| `settings.manage` | the test execution settings of the organisation |
| `tenant.manage` | renaming the organisation, its MFA policy and SCIM provisioning |

Grant `script.execute` only to people you would let run code on the server: scripts and web tests run with the privileges of the server's user, inside the sandbox described under [Security](#the-script-and-web-test-sandbox).

## Custom roles and changed roles

Holders of `role.manage` (Admin by default) change the permissions of every built-in role except Admin and create **custom roles** under **Administration → Role matrix** ([user guide](https://github.com/rom/Qtestframe/blob/rom/docs/user-guide/en/administration.md#change-roles-and-create-your-own)). The rules that keep this safe:

- Nobody can grant a permission they do not hold themselves – not to a role, not to an API token, not through project sharing.
- Admin always keeps every permission, and an organisation always keeps at least one member holding Admin: the last one cannot lose the role or be removed – not in the member list, not in bulk, not through SCIM. Admin may be held together with other roles.
- A custom role has a technical name of 2–32 characters (`a-z`, `0-9`, `_`, starting with a letter), which is what `member add --role` (several roles separated by commas: `--role sme,tester`), `role_mapping` and SCIM group mappings use. It can be deleted once no member, project member or token holds it, also among other roles.
- A built-in role whose permissions differ from the defaults is marked *changed* and can be reset. Changes are audited, and a change of a member's roles is logged with the roles added and taken away (`+sme −viewer`).
- API tokens hold roles too (by default all roles of their owner) and never get more than their owner's current roles allow.
- With single sign-on (`role_mapping`) and SCIM, a user in several mapped groups holds the roles of all of them.

The effective matrix of an organisation is served at `GET /api/v1/roles`.

## Restricted and shared projects

A project is either **organisation-wide** (the default: every member sees it with their organisation role) or **restricted** (only its project members see it). Project members are users or SCIM groups with one or more **project roles**, which replace the organisation roles inside that project, upwards or downwards. Users with `project.update` in the project manage this under **Share** ([user guide](https://github.com/rom/Qtestframe/blob/rom/docs/user-guide/en/administration.md#project-sharing)).

| Who | Permissions inside a project |
|---|---|
| organisation admins and super administrators | their own, always – they see every project |
| a project member (directly or through a group; the direct membership wins, groups add up) | the project-scoped permissions of the project roles (added up) |
| any other member, organisation-wide project | the organisation roles |
| any other member, restricted project | none: the project answers `404` |

Only **project-scoped** permissions come from the project roles (`project.read`, `project.update`, `project.delete`, `requirement.*`, `waiver.approve`, `change.*`, `test.*`, `run.*`, `cycle.plan`, `script.execute`, `export`, `comment.create`). Organisation-level permissions – members, roles, API tokens, the audit log, creating projects – always come from the organisation roles, and an API token is capped by its own roles inside a project as well.

What restriction does **not** hide, so plan accordingly:

- the organisation's **audit log** (`audit.read`) records actions in every project;
- e-mail digests may list notifications that were created while the recipient still had access;
- **backups** contain every project; project archives (*Save project*) contain the whole project.

## Where administration happens

| Page in the web client | Who |
|---|---|
| Members, Role matrix | `member.read` (every built-in role) to see them; changes need `member.manage` / `role.manage` |
| API tokens | `token.manage` |
| Test execution | `settings.manage` |
| Audit log | `audit.read` (Test lead and up by default) |
| Provisioning (SCIM), Multi-factor authentication (organisation policy) | `tenant.manage` |
| Organisations, Users, Backup, System settings, server MFA policy | super administrators |

# Runtime system settings

Super administrators can change a defined set of settings in the web client, under **Administration → System settings** (API `GET`/`PUT /api/v1/admin/settings`), without editing the file or restarting the server. The changes are stored in the database (table `system_settings`) and apply to the whole installation; no setting is per organisation. What the page looks like is described in the [user guide](https://github.com/rom/Qtestframe/blob/rom/docs/user-guide/en/administration.md#system-settings); this chapter is about the rules.

Only super administrators **signed in with a session** can read or change the settings; API tokens (even a super administrator's) and organisation admins get `403`.

## Precedence and locking

**Precedence:** a value changed in the web client wins over the `QTF_*` environment variable and the configuration file, which win over the built-in default. The page shows the source of every value (*Default*, *Configuration file*, *Environment*, *Changed here*), and **Reset to file value** deletes the stored value.

**The operator always wins**, through three kinds of lock:

| Lock | How | Effect |
|---|---|---|
| Environment | the setting is set by a `QTF_*` variable | read-only on the page, with the variable's name; a stored value is ignored |
| Configuration | `system_settings.locked` in the file lists it | read-only; a stored value is ignored |
| File only | the setting is part of the process set-up | shown for information, never changeable at runtime |

`system_settings.locked` takes keys, prefixes ending in `.*`, or `*` for everything:

```yaml
system_settings:
  locked: ["auth.oidc", "email.*", "runner.*"]
```

Use environment variables or locks for what must not change from the web client – in containers, the compose file is a natural place. Operators who do not want secrets in the database lock `email.*` and `auth.oidc` and keep using `*_file` options.

## When a change applies

| Section | Applies |
|---|---|
| General, Security, Single sign-on (OIDC), Logging and syslog, E-mail (SMTP) | **at once** – the e-mail sender, the OIDC providers (unchanged providers are kept, so sign-ins in progress continue), the logger and the sign-in limits are rebuilt |
| Script and web test runner | **after a restart** – the runner and its sandbox are set up before the process is locked down (Landlock execute paths, work directory); the value is stored at once and the page shows the value in use until then |
| Retention and backups | nothing to change: the trash keeps deleted items for 30 days, which is fixed |

`auth.session_ttl` applies to sessions created after the change. New values are validated together with the rest of the configuration – the same checks as `check-config` – before they are stored. If the stored settings are invalid at start-up (for example after a change of the file), the server logs `stored system settings are invalid and were ignored`, uses the file, and the page shows a warning; fix or reset the values there.

## All settings

The settings the page offers, generated from `internal/settings`:

<!-- BEGIN GENERATED: system-settings (make admin-guide-ref) -->

| Section | Key | Values | Applies |
|---|---|---|---|
| General | `server.base_url` | text | at once |
| General | `files.max_upload_mb` | 1–1024 | at once |
| General | `requirements.auto_create_test` | on/off | at once |
| General | `server.listen` | text | shown only (file) |
| General | `database.driver` | text | shown only (file) |
| Security | `auth.local` | on/off | at once |
| Security | `auth.session_ttl` | 5m–2160h | at once |
| Security | `auth.password_min_length` | 8–72 | at once |
| Security | `auth.login_max_attempts` | 1–1000 | at once |
| Security | `auth.login_lockout` | 1m–24h | at once |
| Security | `auth.cookie_secure` | on/off | shown only (file) |
| Security | `server.hsts` | on/off | shown only (file) |
| Security | `server.trusted_proxies` | list | shown only (file) |
| Single sign-on (OIDC) | `auth.oidc` | list of providers | at once |
| Logging and syslog | `logging.level` | `debug`, `info`, `warn`, `error` | at once |
| Logging and syslog | `logging.format` | `text`, `json` | at once |
| Logging and syslog | `logging.syslog.enabled` | on/off | at once |
| Logging and syslog | `logging.syslog.network` | `udp`, `tcp`, `tls`, `unix`, `unixgram` | at once |
| Logging and syslog | `logging.syslog.address` | text | at once |
| Logging and syslog | `logging.syslog.format` | `rfc5424`, `rfc3164` | at once |
| Logging and syslog | `logging.syslog.facility` | `kern`, `user`, `mail`, `daemon`, `auth`, `syslog`, `lpr`, `news`, `uucp`, `cron`, `authpriv`, `ftp`, `local0`, `local1`, `local2`, `local3`, `local4`, `local5`, `local6`, `local7` | at once |
| Logging and syslog | `logging.syslog.tag` | text | at once |
| Logging and syslog | `logging.syslog.ca_file` | text | shown only (file) |
| Logging and syslog | `logging.syslog.cert_file` | text | shown only (file) |
| E-mail (SMTP) | `email.enabled` | on/off | at once |
| E-mail (SMTP) | `email.host` | text | at once |
| E-mail (SMTP) | `email.port` | 1–65535 | at once |
| E-mail (SMTP) | `email.tls` | `starttls`, `tls`, `none` | at once |
| E-mail (SMTP) | `email.username` | text | at once |
| E-mail (SMTP) | `email.password` | secret (write-only) | at once |
| E-mail (SMTP) | `email.from` | text | at once |
| E-mail (SMTP) | `email.from_name` | text | at once |
| E-mail (SMTP) | `email.timeout` | 1s–10m | at once |
| E-mail (SMTP) | `email.digest_hour` | 0–23 | at once |
| Script and web test runner | `runner.enabled` | on/off | after a restart |
| Script and web test runner | `runner.timeout` | 1s–24h | after a restart |
| Script and web test runner | `runner.max_output_bytes` | 1024–67108864 | after a restart |
| Script and web test runner | `runner.deny_network` | on/off | after a restart |
| Script and web test runner | `runner.sandbox` | on/off | shown only (file) |
| Script and web test runner | `runner.web.enabled` | on/off | after a restart |
| Script and web test runner | `runner.web.browser` | `chromium`, `firefox`, `webkit` | after a restart |
| Script and web test runner | `runner.web.headless` | on/off | after a restart |
| Script and web test runner | `runner.web.timeout` | 1s–24h | after a restart |
| Script and web test runner | `runner.web.job_timeout` | 1s–24h | after a restart |
| Script and web test runner | `runner.web.max_parallel` | 1–32 | after a restart |
| Script and web test runner | `runner.web.max_evidence_mb` | 0–1024 | after a restart |
| Script and web test runner | `runner.web.screenshots` | `on_failure`, `always`, `off` | after a restart |
| Script and web test runner | `runner.web.trace` | `on_failure`, `always`, `off` | after a restart |
| Script and web test runner | `runner.web.video` | `off`, `on_failure`, `always` | after a restart |
| Script and web test runner | `runner.web.playwright_dir` | text | shown only (file) |
| Similarity (duplicates) | `similarity.embeddings.enabled` | on/off | at once |
| Similarity (duplicates) | `similarity.embeddings.url` | text | at once |
| Similarity (duplicates) | `similarity.embeddings.model` | text | at once |
| Similarity (duplicates) | `similarity.embeddings.api_key` | secret (write-only) | at once |
| Similarity (duplicates) | `similarity.embeddings.timeout` | 1s–5m | at once |

<!-- END GENERATED: system-settings -->

What each key means is in the [configuration reference](#reference). Everything else – TLS, the database, hardening, the key provider, SAML, `trusted_proxies`, `cookie_secure`, `hsts`, the paths of the runner – is set only in the file.

## Notes on single sign-on, syslog and e-mail

- **OIDC providers.** Changing the providers in the web client stores the whole list, which then **replaces** `auth.oidc` of the file. Client secrets of providers that come from the file are not copied into the database: they keep coming from `client_secret_file` or `QTF_OIDC_<NAME>_CLIENT_SECRET`, which always wins. **Reset to file value** returns to the file's list.
- **SAML** providers can only be configured in the file.
- **Syslog over TLS** uses `ca_file`, `cert_file` and `key_file` from the file; they are readable after the lock-down because they are added to the Landlock read-only paths at start-up. A Unix socket that was not configured at start-up may be refused by Landlock; use `udp`, `tcp` or `tls`, or restart.
- **Secrets** entered on the page (the SMTP password, OIDC client secrets) are write-only, stored [encrypted](#encryption-of-stored-secrets), never logged and never shown again.
- **Test buttons** (*Test connection* for OIDC discovery, *Send test message* for syslog, *Send test e-mail*) make the server connect to an address the super administrator enters. Filter the server's egress if it can reach internal services.

## Embeddings service for similar items

Suspected duplicate requirements and tests, the *Similar requirements* panel of the editors and the catalogue import warning compare items on the server by default: TF-IDF over the project with stemming, stop words and synonyms for English and Swedish (see the user guide, *Duplicates and conflicts*). Nothing is sent anywhere.

Optionally, an **OpenAI-compatible embeddings endpoint** computes the similarity instead: **Administration → System settings → Similarity (duplicates)**, or `similarity.embeddings` in the [configuration file](#reference).

| Setting | Meaning |
|---|---|
| `similarity.embeddings.enabled` | off by default |
| `similarity.embeddings.url` | the endpoint, e.g. `https://llm.example.com/v1/embeddings`; it receives `POST {"model": …, "input": [texts]}` and answers `{"data": [{"index": i, "embedding": [numbers]}]}` |
| `similarity.embeddings.model` | sent as `model` (empty: the service's default) |
| `similarity.embeddings.api_key` | sent as `Authorization: Bearer …`; write-only, stored [encrypted](#encryption-of-stored-secrets); in the file only as `api_key_file` |
| `similarity.embeddings.timeout` | longest wait (default 30 s) |

**Privacy.** When it is on, the **title, description and acceptance criteria of requirements and the title, description, preconditions and steps of tests are sent to that service**, of every project of every organisation on the server, whenever the similarity checks run or someone types in an editor (the text being written is sent too). Use a service run by your organisation or one you have an agreement with; leave it off otherwise.

The vectors are stored in the table `similarity_embeddings` per item, model and text (a changed item is sent again, an unchanged one is not). It is a cache: it is not part of backups or project archives and is filled again after a restore. Changing the model computes every vector again. **Test the service** on the page sends one short example sentence and reports the vector size. When the service fails or times out, the server logs a warning and uses the local similarity, so the checks never stop working. The conflict detection (opposite obligations, thresholds, exclusive values) always runs on the server. The thresholds of the checks apply to the service's cosine similarity as well; services differ, so check the suspected duplicates after turning it on and adjust the threshold of the duplicate checks if needed.

## Audit and export

- Every change is written to the audit log as `settings.update` with old → new value (secrets only as *set* / *changed* / *removed*), and to the server log. The test buttons are audited as `settings.test_*`.
- **Export configuration** (`GET /api/v1/admin/settings/export`) downloads the effective configuration as YAML with secrets replaced by `REDACTED` – for example to move the values into the file and then reset them on the page.
- [Backups](#backup-restore-and-migration) contain the `system_settings` table with its secrets, still encrypted; a restore applies it at once. Project archives do not contain it.

# Security

This chapter is the operator's view. The threat model and the design behind it are in [docs/security.md](https://github.com/rom/Qtestframe/blob/rom/docs/security.md).

## TLS and certificates

Set `server.tls_cert` and `server.tls_key` to serve HTTPS directly. The server accepts TLS 1.2 and 1.3 only; TLS 1.2 is limited to ECDHE key exchange with AES-GCM or ChaCha20-Poly1305; ALPN offers HTTP/2. The certificate file holds the full chain, leaf first.

Scripts in `scripts/certs/`:

```sh
# Let's Encrypt (public name, port 80 reachable, or a DNS-01 method)
sudo scripts/certs/letsencrypt.sh --domain qtf.example.com --email ops@example.com

# an internal CA and a server certificate signed by it
scripts/certs/gen-ca.sh --dir /root/qtf-ca --cn "ACME Internal CA"
sudo scripts/certs/gen-server-cert.sh --ca-dir /root/qtf-ca --cn qtf.example.com \
    --san DNS:qtf.example.com,IP:10.0.0.5 --out /etc/qtestframe/tls --group qtestframe

# check a certificate, or what a server presents
scripts/certs/verify-cert.sh --cert /etc/qtestframe/tls/server.crt --key /etc/qtestframe/tls/server.key --host qtf.example.com
scripts/certs/verify-cert.sh -q --connect qtf.example.com:8443 --warn-days 21    # exit 2 = expires soon
```

**Renewal without downtime:** `systemctl reload qtestframe` sends `SIGHUP`, and the server loads the new pair for new connections; if it cannot be read, the old one stays in use and `tls reload failed; keeping previous certificate` is logged. The Let's Encrypt deploy hook does this for you. Key files: `0640 root:qtestframe`; with SELinux run `restorecon -RF /etc/qtestframe/tls`.

**Behind a TLS-terminating reverse proxy:** listen on `127.0.0.1`, set `server.trusted_proxies` to the proxy's address, keep `auth.cookie_secure: true` and `server.base_url` on `https://`, and let the proxy send `Strict-Transport-Security` – Qtestframe sends HSTS only on TLS connections it terminates itself.

## Hardening

### In the process

After start-up – once the database, the key provider, the certificate and the listener are open – `serve` restricts itself (`internal/hardening`, Linux only):

| Restriction | Key | What it does |
|---|---|---|
| `no_new_privs` | `hardening.no_new_privs` | no privilege gain through setuid binaries or file capabilities, for the server and everything it starts. A failure **stops the start**. |
| capabilities | `hardening.drop_capabilities` | drops every capability except `CAP_NET_BIND_SERVICE` (bounding, effective, permitted, inheritable, ambient) |
| Landlock | `hardening.landlock` | file access limited to the paths the configuration needs (best effort up to Landlock ABI 5) |
| seccomp | `hardening.seccomp` | a deny-list of system calls that a server never needs; they fail with `EPERM` |

Except for `no_new_privs`, a restriction that the kernel does not support is skipped, not fatal. The start-up log line shows the result:

```text
time=… level=INFO msg=hardening applied="[no_new_privs capabilities landlock seccomp]" skipped=[]
```

**Landlock paths.** The server may read the built-in list below plus the files it was configured with (TLS, syslog and SAML files, `$CREDENTIALS_DIRECTORY`), and write the temporary directory, the SQLite database directory, a Unix syslog socket, the TPM device, and with the runner its work directory (and execute from `/usr`, `/bin`, `/lib`, `/lib64`, `/opt`, the binary's directory and the interpreters' directories). Web tests add the Playwright, browser and Node.js directories, `/etc`, `/proc`, `/sys` and `/dev/shm`. **Only paths that exist at start-up are added.** Anything else needs `hardening.read_only_paths` or `hardening.read_write_paths`, for example `/run/postgresql` for a local PostgreSQL socket or the token directory of SoftHSM.

<!-- BEGIN GENERATED: hardening (make admin-guide-ref) -->

| List | Entries |
|---|---|
| Read-only paths after the lock-down (besides the configured files) | `/etc/ssl`, `/etc/pki`, `/etc/ca-certificates`, `/usr/share/ca-certificates`, `/etc/resolv.conf`, `/etc/hosts`, `/etc/nsswitch.conf`, `/etc/localtime`, `/etc/gai.conf`, `/etc/services`, `/etc/protocols`, `/usr/share/zoneinfo`, `/etc/qtestframe`, `/proc/self` |
| System calls denied by seccomp (`EPERM`) | `acct`, `add_key`, `bpf`, `chroot`, `clock_adjtime`, `clock_settime`, `delete_module`, `finit_module`, `fsconfig`, `fsmount`, `fsopen`, `fspick`, `init_module`, `iopl`, `ioperm`, `kexec_file_load`, `kexec_load`, `keyctl`, `lookup_dcookie`, `mount`, `mount_setattr`, `move_mount`, `name_to_handle_at`, `open_by_handle_at`, `open_tree`, `perf_event_open`, `personality`, `pivot_root`, `process_vm_readv`, `process_vm_writev`, `ptrace`, `quotactl`, `reboot`, `request_key`, `setdomainname`, `sethostname`, `setns`, `settimeofday`, `swapoff`, `swapon`, `syslog`, `umount2`, `unshare`, `userfaultfd`, `vhangup` |
| Further system calls denied to scripts with `runner.deny_network` | `socket`, `socketpair`, `connect`, `bind`, `listen`, `accept`, `accept4` |

<!-- END GENERATED: hardening -->

A `permission denied` after start-up for a file the server needs means a path is missing from these rules. A `landlock: …` entry under `skipped` means the kernel lacks Landlock (older than 5.13, or `landlock` missing from the `lsm=` boot parameter).

### Operating-system layers

The in-process hardening also works in containers and without systemd. On a host, more layers apply:

| Layer | File | Notes |
|---|---|---|
| systemd sandbox | `deploy/systemd/qtestframe.service` | own user, `ProtectSystem=strict`, private `/tmp` and devices, `NoNewPrivileges`, only `CAP_NET_BIND_SERVICE`, system call filter, `MemoryDenyWriteExecute`, `MemoryMax`/`TasksMax`. `systemd-analyze security qtestframe` should report about 1.x. |
| SELinux | `deploy/selinux/` | domain `qtestframe_t`, booleans for optional features (off by default): `qtestframe_connect_postgresql`, `qtestframe_remote_syslog`, `qtestframe_connect_http`, `qtestframe_bind_http_port`, `qtestframe_run_scripts`, `qtestframe_runner_execmem` |
| AppArmor | `deploy/apparmor/usr.bin.qtestframe` | read-only configuration, scripts in the child profile `qtestframe//runner` without network |
| container seccomp | `deploy/seccomp/qtestframe-seccomp.json` | an allow-list for container runtimes |
| firewall, sysctl | `deploy/nftables/`, `deploy/sysctl/` | inbound and optional outbound policy, kernel settings |

Verify a running server:

```sh
PID=$(systemctl show -p MainPID --value qtestframe)
grep -E 'NoNewPrivs|Seccomp|CapEff' /proc/$PID/status    # NoNewPrivs 1, Seccomp 2, CapEff 0000000000000400
journalctl -u qtestframe | grep -i hardening
systemd-analyze security qtestframe
```

### The script and web test sandbox

When `runner.enabled` is on, members with `script.execute` run code on the server. With `runner.sandbox: true` (default) each script runs through `qtestframe sandbox-exec`, which before starting the interpreter sets resource limits (CPU time = timeout + 1 s, 64 MiB file size, 256 open files, no core dumps), no_new_privs, drops capabilities, applies a Landlock rule set (read-only system directories, write only to the job directory) and the seccomp deny-list, with `runner.deny_network: true` also the socket calls. The environment holds only `PATH`, `HOME`/`TMPDIR` (the job directory), `LANG`, the `QTF_*` run variables and the project's test variables. The whole process group is killed at `runner.timeout`.

**Web tests have a weaker sandbox:** no seccomp filter and always network access, because a browser needs both. A web test can reach every host the server can reach, including internal services and cloud metadata endpoints. Restrict the server's outgoing connections (firewall, egress proxy) or run web tests on a dedicated instance.

Even with every layer, treat the runner as a boundary that a determined person with `script.execute` may eventually cross. Note that `/etc` (with `/etc/qtestframe`) is readable inside the sandbox: protect configuration and keys with file permissions and SELinux/AppArmor.

## Encryption of stored secrets

The server must read some secrets back, so it cannot hash them. These are stored **encrypted** with AES-256-GCM:

| Secret | Stored in |
|---|---|
| TOTP secrets of authenticator apps | `user_mfa.totp_secret` |
| webhook signing secrets | `webhooks.secret` |
| defect tracker tokens (Jira, GitHub, GitLab) | `projects.defect_tracker` |
| values of secret web test variables | `projects.web_tests` |
| SMTP password, OIDC client secrets and the key of the embeddings service entered in the web client | `system_settings` |

Passwords (bcrypt), API and SCIM tokens, sessions, invitations, remembered devices and recovery codes are stored as hashes and need no key. Secrets of the configuration file, `*_file` options and `QTF_*` variables are never stored in the database.

**Envelope encryption.** Each value is encrypted with a random 256-bit *data-encryption key* (DEK), stored as `enc:v1:<dek id>:<data>`, and bound to its table, column, row and field, so a value copied elsewhere does not decrypt. The DEKs are kept in the table `secret_keys`, **only wrapped** by the *key-encryption key* (KEK) of the configured **key provider**. The KEK never enters the database; unwrapped DEKs exist only in memory.

At every start, and with `qtestframe secrets migrate`, values that an older version stored as plaintext are encrypted in place. `qtestframe secrets status` shows the provider, the key fingerprint, the DEKs and how many values are encrypted; **Administration → System settings → Security** shows the same read-only.

### Key providers

| Provider | KEK | Protects against | Build |
|---|---|---|---|
| `local` (default) | a 256-bit key in a key file, or `QTF_SECRETS_KEY` | copies of the database, dumps, backups and replicas that leave without the key | static |
| `tpm` | sealed to this machine's TPM 2.0, optionally bound to PCR values | additionally: a copied disk, and with PCRs a changed boot chain | static (pure Go) |
| `pkcs11` | an AES-256 key in an HSM, used through PKCS#11 | additionally: the key can never be copied out of the HSM | `make go-pkcs11` (cgo) |

None of them protects against someone who controls the running server.

**local.** Nothing to configure. The key file `qtestframe-secrets.key` is created with mode 0600 when missing, next to a SQLite database (else in `$STATE_DIRECTORY`, `/var/lib/qtestframe` or the working directory); `secrets.local.key_file` puts it elsewhere, for example on another volume. Or generate a key and give it in the environment from your secret store – then no file is used:

```sh
qtestframe secrets generate-key        # prints a base64 key for QTF_SECRETS_KEY
```

The server warns at start-up when the key file is readable by group or others. Several server processes on one PostgreSQL database need the **same** key.

**tpm.**

```yaml
secrets:
  provider: tpm
  tpm:
    device: /dev/tpmrm0
    pcrs: [7]              # optional: bind to the Secure Boot state
    pcr_bank: sha256
```

Give the service the device in a drop-in, because the unit has `PrivateDevices=yes`:

```ini
[Service]
PrivateDevices=no
DeviceAllow=/dev/tpmrm0 rw
SupplementaryGroups=tss
```

The DEK is sealed under the TPM's storage root key through a salted session, so it never crosses the bus in clear. It unseals only on the same TPM (until its owner hierarchy is cleared), and with `pcrs` only while those PCRs keep their values. Test against a software TPM with `swtpm` (`make test-tpm`).

**pkcs11.**

```yaml
secrets:
  provider: pkcs11
  pkcs11:
    module: /usr/lib/softhsm/libsofthsm2.so
    token_label: qtestframe       # or slot: 0
    pin_file: hsm-pin             # or QTF_SECRETS_PKCS11_PIN
    key_label: qtestframe-kek
    generate_key: false           # true: create a non-extractable AES-256 key if missing
```

The DEK is wrapped with `CKM_AES_GCM` by the HSM key. The binary must be built with `make go-pkcs11`. With Landlock, a file-based token such as SoftHSM needs its token directory in `hardening.read_write_paths`. For a test token: `softhsm2-util --init-token --free --label qtestframe --pin … --so-pin …`.

**If the key is missing or wrong**, the server refuses to start with an error that names the key it expects (for example `the stored data-encryption keys are wrapped by another key: … the configured key is …`). It never falls back to plaintext.

### Rotation and re-wrapping

| Task | Command | Effect |
|---|---|---|
| new data key | `qtestframe secrets rotate` | creates a new DEK, makes it active and re-encrypts every value; old DEKs are kept |
| remove old data keys | `qtestframe secrets rotate --prune` | as above, then deletes the DEKs no value uses; stop or restart the other server processes first |
| new KEK, or another provider | `qtestframe secrets rewrap --config NEW.yaml --from-config OLD.yaml` | unwraps the DEKs with the provider of `OLD.yaml` and wraps them with the one of `NEW.yaml`; the stored values are unchanged |

Other server processes pick up a rotated DEK when they meet a value encrypted with it. **Procedure for a new KEK or provider** (for example a new local key file, local → TPM, local → HSM):

1. Take a backup (`qtestframe backup`) and keep the current key.
2. Copy the configuration: `cp config.yaml config.new.yaml`, and change the `secrets` section in the copy (new `key_file`, or `provider: tpm` / `pkcs11`).
3. Stop the server, then run `qtestframe secrets rewrap --config config.new.yaml --from-config config.yaml`. A new local key file is created by this step.
4. Make `config.new.yaml` the configuration and start the server; check `qtestframe secrets status`.
5. Keep the old key until the new set-up works – **backups taken before need the old key**.

### Disaster recovery for TPM-sealed keys

A TPM cannot be backed up, and with `pcrs` even a firmware update can make the key unsealable. Plan for it:

- **Planned changes** (firmware, Secure Boot keys, boot loader, a new mainboard): first re-wrap to a local key (`secrets rewrap` with a `local` configuration as `--config` and the TPM configuration as `--from-config`), run with the local key during the change, then re-wrap back to the TPM.
- **Unplanned loss** (broken mainboard, cleared TPM): only a **recovery backup** whose DEKs are wrapped by a key you hold elsewhere helps. Make one regularly, and after every `secrets rotate`, on the TPM machine and as the service user (which may use the TPM):

  ```sh
  # recovery-tpm.yaml:    sqlite /var/lib/qtestframe/recovery.db, secrets as in production (tpm)
  # recovery-local.yaml:  the same database, provider local, key_file on removable or offline storage
  qtestframe backup  --config /etc/qtestframe/config.yaml --out /var/backups/qtestframe/qtf.zip
  qtestframe restore --config recovery-tpm.yaml --in /var/backups/qtestframe/qtf.zip --yes
  qtestframe secrets rewrap --config recovery-local.yaml --from-config recovery-tpm.yaml
  qtestframe backup  --config recovery-local.yaml --out /var/backups/qtestframe/qtf-recovery.zip
  rm /var/lib/qtestframe/recovery.db*
  ```

  Store `qtf-recovery.zip` and the escrow key apart from each other (the key never next to the backups). The restore into the scratch database works because the same TPM unwraps the keys; the re-wrap creates the escrow key file. To recover, restore the recovery backup on a server configured with the escrow key (local provider), then re-wrap to the new TPM.
- If the KEK is lost and there is no recovery backup, the stored secrets cannot be recovered; the server refuses to start on that database.

The same applies, in a weaker form, to HSM keys (use the HSM's own key backup) and to local key files (back them up – separately from the database).

## The audit log

Every organisation has an audit log (**Administration → Audit log**, permission `audit.read`, API `GET /api/v1/audit`, newest first, up to 5000 entries per request): who (user id and e-mail, or `SCIM: <token>`, or `cli`), what (`action`, entity, short details) and when. Recorded are sign-ins (with the method, and `mfa=…`, `amr`/`acr` for single sign-on), changes of projects, items, runs, members, roles, tokens, sharing, organisations and users, exports, backups and restores (`admin.backup`, `admin.restore`), MFA resets and policy changes, and system settings (`settings.update`). Old and new values of items are kept in the item history, not in the audit log; the client address is in the request log, not in the audit entry.

Each audit entry is also written as an INFO log record `audit`. Ship the log to a central syslog server (see [Logging](#central-syslog)), so the trail survives a compromise of the host. Failed sign-ins appear in the server log (`login failed`) and in the user's sign-in history, not in the audit log. The audit log is part of a backup.

## Security headers and CSP

Every response carries:

| Header | Value |
|---|---|
| `Content-Security-Policy` | `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self' https:; object-src 'none'` |
| `Strict-Transport-Security` | `max-age=63072000; includeSubDomains`, with `server.hsts: true` on TLS connections the server terminates |
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=(), payment=()` |
| `Cross-Origin-Opener-Policy`, `Cross-Origin-Resource-Policy` | `same-origin` |
| `Cache-Control` | `no-store` for `/api/…` |

The CSP allows no inline scripts and no `eval`; inline styles are needed by the UI library, and `form-action https:` by the single sign-on redirects. A reverse proxy should pass these headers through unchanged; Qtestframe cannot be embedded in frames of other sites.

Other limits: JSON bodies at most 8 MiB, headers 1 MiB, `ReadHeaderTimeout` 10 s, `IdleTimeout` 120 s; queries at most 4096 bytes with RE2 regular expressions (no catastrophic backtracking).

## Outgoing connections

Project administrators (`project.update`) choose the URLs of trackers and webhooks, and script authors (`script.execute`) the targets of web tests; super administrators can make the server connect to addresses with the test buttons of the system settings. When the optional [embeddings service](#embeddings-service-for-similar-items) is on, the server sends the text of requirements and tests to it. Every such request leaves from the server's network. If the server can reach internal services, filter its egress with a firewall or proxy (`deploy/nftables` has an optional egress policy). Webhook deliveries never follow redirects.

## Checklist

- [ ] TLS certificate from a trusted CA; `server.hsts: true`; `auth.cookie_secure: true`; `server.base_url` on `https://`
- [ ] single sign-on configured; `auth.local: false`, or few local accounts with strong passwords
- [ ] MFA required at least for administrators (MFA policy, or at the identity provider with `local_mfa` set accordingly)
- [ ] secrets in `*_file` options or `LoadCredential=`, files `0640 root:qtestframe`
- [ ] key of the stored secrets backed up apart from the database (or TPM/HSM with a recovery plan); `secrets status` shows no plaintext
- [ ] PostgreSQL with `sslmode=verify-full` and a dedicated role
- [ ] logs shipped to a central syslog server over `tls`
- [ ] `runner.enabled: false` unless needed; otherwise only trusted people with `script.execute`, MAC enforced, egress filtered
- [ ] SELinux module or AppArmor profile loaded; `systemd-analyze security qtestframe` ≤ 2.0
- [ ] `server.trusted_proxies` set behind a reverse proxy
- [ ] backups encrypted, stored off-host and a restore tested

# E-mail, webhooks and integrations

## E-mail

Outgoing e-mail is optional. With it, users can get their notifications by e-mail (each one at once or in a daily digest, chosen in their profile), admins can invite new members by e-mail, and reports can be sent by e-mail or on a schedule. Without it these features are hidden and the API answers that e-mail is not configured.

```yaml
server:
  base_url: https://qtf.example.com      # required with e-mail: links in the messages
email:
  host: smtp.example.com
  port: 587                  # 587 = STARTTLS, 465 = implicit TLS
  tls: starttls              # starttls | tls | none (local relay only)
  username: qtestframe       # leave out for a relay without authentication
  password_file: smtp-password
  from: qtestframe@example.com
  from_name: Qtestframe
  timeout: 30s
  digest_hour: 7             # UTC
```

E-mail is on while `email.host` is set (`enabled: false` keeps the settings but sends nothing). The same settings can be changed at runtime in **Administration → System settings → E-mail (SMTP)**, which also has **Send test e-mail**; organisation admins have **Send test e-mail** under **Members**. Both show the answer of the mail server when sending fails.

- Mail is sent in the background, never for a request that failed; failures are logged at WARN. On shutdown the server waits for messages in flight (each bounded by `email.timeout`).
- The daily digest and scheduled reports are checked every minute; the digest goes out at `digest_hour` (UTC).
- Invitation links are valid for 7 days and can be used once. Sending reports by e-mail is limited to 30 per user in 10 minutes.
- Notifications, @mentions and report e-mails go only to people who may see the project they are about.

## Notifications

In-app notifications (assignments, @mentions, review requests, defect changes, project sharing) need no configuration. Users choose in their profile whether they also get them by e-mail.

## Webhooks

Projects can send signed events to other systems. Users with `project.update` set them up in the project settings; every webhook has a URL, the events it wants (or all) and a secret that is shown once when created or rotated.

<!-- BEGIN GENERATED: webhook-events (make admin-guide-ref) -->

| Object | Events |
|---|---|
| requirement | `requirement.created`, `requirement.updated` |
| test | `test.created`, `test.updated` |
| run | `run.created`, `run.updated`, `run.completed` |
| execution | `execution.updated` |
| defect | `defect.created`, `defect.updated`, `defect.status`, `defect.assigned`, `defect.regression` |
| risk | `risk.created`, `risk.updated` |
| release | `release.created`, `release.updated`, `release.released` |
| comment | `comment.created`, `comment.mentioned`, `comment.resolved` |
| review | `review.requested`, `review.completed` |
| baseline | `baseline.signed` |
| waiver | `waiver.requested`, `waiver.approved`, `waiver.rejected`, `waiver.revoked` |
| change_request | `change_request.submitted`, `change_request.review`, `change_request.approved`, `change_request.rejected`, `change_request.deferred`, `change_request.implemented`, `change_request.closed` |

<!-- END GENERATED: webhook-events -->

Each delivery is a `POST` with a JSON body `{"id", "event", "project_id", "actor", "time", "data"}` and the headers:

| Header | |
|---|---|
| `X-Qtestframe-Event` | the event, e.g. `run.completed` |
| `X-Qtestframe-Webhook` | the id of the webhook |
| `X-Qtestframe-Signature` | `sha256=` + hex HMAC-SHA256 of the raw body with the webhook's secret |
| `User-Agent` | `Qtestframe-Webhook/1` |

Receivers should verify the signature (compare in constant time) and ignore repeated `id`s. Deliveries are made after the change is committed, time out after 10 seconds, **never follow redirects** and are tried once; the status of the last delivery is shown with the webhook, and failures are logged (`webhook delivery`). The webhook secrets are stored encrypted. Loaded project archives contain no secrets and their webhooks are disabled.

## Chat notifications

Projects can post their events to chat channels: Slack, Microsoft Teams, Mattermost, Rocket.Chat, Google Chat, Matrix and Discord. Users with `project.update` set the channels up in the project settings (see the user guide, *Reviews and notifications*). Nothing needs to be configured on the server, but:

- Set `server.base_url` (or the system setting): without it messages cannot link to the items.
- The server must reach the chat tool over HTTPS (outbound proxy and firewall). Posts time out after 10 seconds and never follow redirects.
- The webhook URL or the Matrix token of a channel is stored encrypted like the other secrets (`qtestframe secrets` re-encrypts it), is never returned by the API, written to the audit log or shown in an error, and is left out of project archives (a loaded channel is off until someone enters a new URL).

**Events.** A channel offers the events of the central event registry (the same as the webhooks above), grouped for the settings. The list is derived from the registry: an event that a later version adds appears in the channel settings and in **All events** without any change to the chat code; its group is taken from the part of its name before the dot (unknown parts go to *Other*), its label from the event name unless a translation exists.

<!-- BEGIN GENERATED: chat-events (make admin-guide-ref) -->

| Group | Events |
|---|---|
| Test runs | `run.created` (Test run created), `run.updated` (Test run updated), `run.completed` (Test run completed), `execution.updated` (Test result recorded) |
| Defects | `defect.created` (Defect reported), `defect.updated` (Defect updated), `defect.status` (Defect status changed), `defect.assigned` (Defect assigned), `defect.regression` (Regression detected) |
| Requirements and tests | `requirement.created` (Requirement created), `requirement.updated` (Requirement updated), `test.created` (Test created), `test.updated` (Test updated), `comment.created` (New comment), `comment.mentioned` (Comment mentioned), `comment.resolved` (Comment resolved), `baseline.signed` (Baseline signed) |
| Reviews | `review.requested` (Review requested), `review.completed` (Review completed) |
| Releases | `release.created` (Release created), `release.updated` (Release updated), `release.released` (Release released) |
| Waivers | `waiver.requested` (Waiver requested), `waiver.approved` (Waiver approved), `waiver.rejected` (Waiver rejected), `waiver.revoked` (Waiver revoked) |
| Change requests | `change_request.submitted` (Change request submitted), `change_request.review` (Change request to decide), `change_request.approved` (Change request approved), `change_request.rejected` (Change request rejected), `change_request.deferred` (Change request deferred), `change_request.implemented` (Change request implemented), `change_request.closed` (Change request closed) |
| Risks | `risk.created` (Risk identified), `risk.updated` (Risk updated) |

<!-- END GENERATED: chat-events -->

**Delivery.** Every post is stored in a queue (table `chat_deliveries`) once the change is committed and sent at once in the background. A failed post is retried by the server's minute job with exponential backoff (1, 2, 4, 8 and 16 minutes, or the `Retry-After` of an HTTP 429, at most an hour), up to 6 attempts; an HTTP 4xx answer other than 408 and 429 (the chat tool refused the URL, token or message) is final. Several servers sharing one PostgreSQL database never post the same message twice (a post is claimed before it is sent). After 5 failed posts in a row the channel turns itself off; the project's administrators get an in-app notification (and an e-mail if they chose immediate e-mails), and the audit log has a `chat.disabled` entry. The last 50 posts of each channel are kept as its delivery log; the queue and the log are not part of a backup (the channels are). Digest channels collect their events and post them as one message every N minutes (1–1440).

**Content.** Messages contain the event, the item key and title, the status, the priority, the person and a summary of at most a few hundred characters. They never contain secrets. In a project with restricted access, messages contain only the event, key, status and link unless an organisation administrator (`tenant.manage`) allows the channel the full content. Broadcast mentions (`@channel`, `@here`, `@all`, `@everyone`) in texts are defused.

### Setting up each chat tool

| Tool | What to create | What to enter in Qtestframe |
|---|---|---|
| Slack | A Slack app with **Incoming Webhooks** turned on; **Add New Webhook to Workspace** and choose the channel. Messages use Block Kit. | The webhook URL `https://hooks.slack.com/services/…` |
| Microsoft Teams | In the channel: **Workflows** → template *Post to a channel when a webhook request is received* (Power Automate). Messages are Adaptive Cards 1.4. | The URL of the workflow's HTTP trigger |
| Microsoft Teams (Office 365 connector) | The older **Incoming Webhook** connector of a channel (MessageCard format). Microsoft is retiring these connectors: use it only where Workflows are not available. | The connector URL |
| Mattermost | **Integrations → Incoming Webhooks → Add Incoming Webhook**, choose the channel. Allow integrations to override the user name if the posts should show *Qtestframe*. | The URL `https://<server>/hooks/<id>` |
| Rocket.Chat | **Administration → Workspace → Integrations → New → Incoming**: turn it on, choose the channel (`#qa`) and the user to post as; no script is needed. | The webhook URL shown after saving |
| Google Chat | In the space: **Apps & integrations → Webhooks → Add webhook**. Messages are cards (cardsV2). | The webhook URL (it contains `key` and `token` parameters) |
| Matrix | A bot account on the homeserver; invite it to the room and let it join. Get its access token (for example by signing in with it once through the client API). Messages are `m.notice` events with HTML. | The homeserver URL, the room ID (`!…:server`) or alias (`#…:server`) and the access token |
| Discord | **Channel settings → Integrations → Webhooks → New Webhook**. Messages are embeds; mentions are turned off. | The webhook URL `https://discord.com/api/webhooks/…` |

Use **Send test message** to check a channel; the delivery log shows the HTTP status and the error of each post (for example `HTTP 404: no_service` from Slack when the webhook was removed, `HTTP 401: M_UNKNOWN_TOKEN` from Matrix). Personal chat notifications (direct messages to one user) are not offered; users get those as in-app notifications and e-mails.

## Defect and issue trackers

Projects can push defects and requirements to **Jira**, **GitHub**, **GitLab** or **Request Tracker (RT)**, sync their status back and import issues. Users with `project.update` connect a project under its settings ([user guide](https://github.com/rom/Qtestframe/blob/rom/docs/user-guide/en/defects.md#connect-jira-github-or-gitlab)); pushing and syncing needs `run.execute`, issues from requirements `requirement.write`.

| Tracker | URL | Credential | Smallest useful scope |
|---|---|---|---|
| Jira Cloud | `https://<site>.atlassian.net` | user e-mail + API token (HTTP Basic) | a user that may browse and create issues in the project |
| Jira Server / Data Center | your Jira URL | personal access token (Bearer) | as above |
| GitHub | default `https://api.github.com` (GitHub Enterprise: its API URL) | token (Bearer) | fine-grained token with *Issues: read and write* on the one repository |
| GitLab | default `https://gitlab.com`, or a self-managed instance (HTTPS) | token in `PRIVATE-TOKEN` | project access token with the `api` scope and the Reporter role |
| Request Tracker 5 | the RT base URL, e.g. `https://rt.example.com` (REST2 below `/REST/2.0`) | auth token in `Authorization: token …` | an RT user with *CreateTicket*, *ShowTicket*, *ModifyTicket* and *CommentOnTicket* in the queue |

The token is entered once, stored encrypted, and never returned by the API (only *has token*); it is not part of a project archive. Requests honour `HTTPS_PROXY`/`NO_PROXY` and the system's CA certificates, have a timeout, and wait for the tracker's `Retry-After` when it is short. Pushes and syncs are audited (`defect.push`, `defect.sync`, `requirement.issue.*`).

### Request Tracker

RT needs the REST2 API, which is part of RT 5 (for RT 4.4 the extension *RT::Extension::REST2*). Create a dedicated RT user for Qtestframe, grant it the rights above in the queue (a group right on the queue is the simplest), sign in as that user and create a token under **Logged in as → Settings → Auth Tokens**. The project settings take the RT base URL, the **queue** (name or ID) and optionally the name of a custom field that receives labels (RT has no labels of its own). Check that a reverse proxy in front of RT passes the `Authorization` header to `/REST/2.0`. Status sync maps the default lifecycle (*new*, *open*, *stalled*, *resolved*, *rejected*, *deleted*); statuses of custom lifecycles are shown but do not change the defect. Sync is on demand (**Sync** / **Sync all**, or `POST /api/v1/projects/{pid}/defects/sync` from a scheduler), as for the other trackers: no inbound webhook is needed.

## Requirement import from Jira and Azure DevOps

Users with `requirement.write` import requirements from a Jira JQL query or an Azure DevOps WIQL query ([user guide](https://github.com/rom/Qtestframe/blob/rom/docs/user-guide/en/reports-and-export.md#jira-and-azure-devops)); the connections are set by users with `project.update` under **Project → Import sources** and stored in `projects.import_sources` (migration 0037).

| Source | URL | Credential | Smallest useful scope |
|---|---|---|---|
| Jira Cloud | `https://<site>.atlassian.net` (or none: the project's Jira issue tracker is used) | user e-mail + API token (HTTP Basic) | *Browse projects* in the projects queried |
| Jira Server / Data Center | your Jira URL | personal access token (Bearer) | as above |
| Azure DevOps Services | `https://dev.azure.com/<organization>` and the project name | personal access token (HTTP Basic with an empty user name) | scope *Work Items (Read)*, organization limited to the one organization |
| Azure DevOps Server | the collection URL, e.g. `https://tfs.example.com/tfs/DefaultCollection` | personal access token | as above |

Jira is searched with `POST /rest/api/2/search/jql` (Jira Cloud) and, when that does not exist, `POST /rest/api/2/search` (Server and Data Center); Azure DevOps with the WIQL, fields and work items endpoints of REST API 7.1. At most 1,000 items are read per import. The import only reads; it is audited as `requirement.import`, the connection changes as `import_sources.update`. Tokens are encrypted with the server's secret keys (they are re-encrypted by key rotation), never returned, and left out of project archives. The import is reached through the server's outbound proxy (`HTTPS_PROXY`, `NO_PROXY`) like the trackers; allow the hosts above in a firewall.

## CI/CD pipelines (GitLab, GitHub Actions)

Pipelines talk to the server with the `qtestframe` command (`report`, `gate`, `select`; [command line reference](#command-line-reference)) and an API token. Nothing needs to be configured on the server; what each part needs:

- **API token.** Create a dedicated CI user, make it a member with the **tester** role in the projects it reports to (tester holds `run.execute` for uploads, `run.write` for `--create-run` and `test.write` for `--create-missing`), and create its token with the role *tester* under **Administration → API tokens**. A token never gets more than its user's current role. Store it as a masked CI/CD variable `QTF_TOKEN`. There are no finer token scopes than roles; a custom role with only `project.read`, `test.read`, `run.read`, `run.execute` and `run.write` is the narrowest that runs every command.
- **The binary.** The jobs need an image with the static `qtestframe` binary (`QTF_CLI_IMAGE`); the server image is distroless, so copy the binary into your CI image (`COPY --from=qtestframe:<version> /usr/bin/qtestframe /usr/local/bin/`).
- **Templates.** `examples/ci/` holds `.gitlab-ci.yml` templates (with and without test selection), a CI/CD component and a GitHub Actions workflow; the same, with the server URL and project key filled in, are in **Settings → Project → CI setup** and at the public `GET /api/v1/ci/templates/{name}` (for `include: remote:`; they contain no secrets).
- **Commit statuses and merge request notes from the pipeline** need a GitLab token in `QTF_GITLAB_TOKEN`: a project access token with the role **Developer** (commit statuses need Developer; notes Reporter) and the scope `api`. GitLab does not accept the job token `CI_JOB_TOKEN` for `POST /projects/:id/statuses/:sha` or merge request notes at the time of writing (its fine-grained job token permissions do not cover them); the command falls back to it and prints a warning when GitLab refuses it. In GitHub Actions the workflow's `GITHUB_TOKEN` with `statuses: write` and `pull-requests: write` is enough (`QTF_GITHUB_TOKEN`).
- **Server-side posting.** Instead, a project can let the server post: **Settings → Project → Results back to GitLab** stores the GitLab API URL, project and a project access token (encrypted like the other secrets, write-only, re-encrypted by `qtestframe secrets`, left out of project archives). When results with `?commit=` arrive, a row is added to the `ci_deliveries` outbox and posted in the background: `qtestframe/tests` (and `qtestframe/gate` with a release or conditions) and a merge request note that is updated, not repeated. Failed posts are retried by the minute job with exponential backoff (1, 2, 4, … minutes, up to 6 attempts; HTTP 429 waits as long as GitLab asks); a refused token or project (other 4xx) is not retried. The last 50 posts per project are shown in the settings; `ci_deliveries` is not backed up. The server needs outbound HTTPS to the GitLab instance.

## Script and web test runner

The runner executes the scripts of automated tests (`sh`, `bash`, `python`, `javascript`) and web tests (Playwright) on the server. It is **off by default**; read [the sandbox section](#the-script-and-web-test-sandbox) before turning it on. Its settings apply after a restart.

### Scripts

```yaml
runner:
  enabled: true
  timeout: 60s                       # keep below server.write_timeout
  work_dir: /var/lib/qtestframe/runner
  max_output_bytes: 1048576
  interpreters: {sh: /bin/sh, python: /usr/bin/python3}   # only what you need
  sandbox: true
  deny_network: true                 # when scripts need no network
```

*Run script* of a single script runs within the request, so `runner.timeout` must stay below `server.write_timeout`. *Run automated tests* on a run queues all automated tests of the run as background jobs. The work directory is created with mode 0700 and added to the Landlock paths; job directories are scratch space.

Under systemd, Node.js needs `MemoryDenyWriteExecute=no` in a drop-in (its JIT), or configure the interpreter as a wrapper that runs `node --jitless`; with SELinux enable `qtestframe_run_scripts` (and `qtestframe_runner_execmem` for JIT interpreters). AppArmor runs interpreters in the child profile `qtestframe//runner` without network.

### Web tests (Playwright)

Install Node.js 18 or later and Playwright with at least one browser:

```sh
sudo mkdir -p /opt/qtestframe-playwright && cd /opt/qtestframe-playwright
sudo npm init -y && sudo npm i @playwright/test
sudo PLAYWRIGHT_BROWSERS_PATH=/opt/qtestframe-playwright/browsers npx playwright install --with-deps chromium
```

```yaml
runner:
  enabled: true
  work_dir: /var/lib/qtestframe/runner       # keep it short
  web:
    enabled: true
    node: /usr/bin/node
    playwright_dir: /opt/qtestframe-playwright
    browsers_path: /opt/qtestframe-playwright/browsers
    browser: chromium
    timeout: 60s              # per test
    job_timeout: 10m          # whole Playwright process
    max_parallel: 2           # per server process
    screenshots: on_failure
    trace: on_failure
    video: "off"
```

And a systemd drop-in:

```ini
[Service]
MemoryDenyWriteExecute=no
ExecPaths=/opt/qtestframe-playwright
ReadWritePaths=/dev/shm
```

For each web test the server writes the spec and a generated `playwright.config.js` into a job directory and runs `<node> <playwright_dir>/node_modules/@playwright/test/cli.js test --config <file>` (`npx` is not used, nothing is downloaded); the result comes from Playwright's JSON report. Screenshots, the trace and the video become attachments of the execution (each at most `max_evidence_mb`). A trace contains the pages, the network requests (cookies included) and the console of the test.

Web tests always run as **background jobs**; `max_parallel` of them run at a time per server process. Jobs do not survive a restart: unfinished ones are marked interrupted and their executions get their earlier result back, at start-up (SQLite) or when the instance that queued them has not been seen for about a minute (PostgreSQL).

Projects set the base URL of the system under test and test variables under **Settings → Web tests**. Test variables reach every automated script as `QTF_VAR_<NAME>`; secret ones are stored encrypted and masked as `***` in stored output (not in screenshots or traces).

The container image needs `--build-arg WITH_PLAYWRIGHT=true` and `QTF_RUNNER_ENABLED=true`, `QTF_RUNNER_WEB_ENABLED=true` (see [Installation](#in-a-container-docker-or-podman)).

# Connecting other tools (OSLC and ReqIF)

Qtestframe exchanges requirements and test cases with ALM and requirements tools in two ways (the user guide page *Interoperability* describes what users see):

- **OSLC**, live: Qtestframe is an OSLC provider for Requirements Management 2.1 (requirements) and Quality Management 2.1 (test cases). Other tools discover it, query, create, change and delete items, link their own items to them, and show Qtestframe's selection and creation dialogs to their users.
- **ReqIF files**, for whole specifications: export from one tool, import into the other, repeatedly. Nothing needs to be set up on the server.

This chapter is about setting up OSLC connections with IBM Engineering Lifecycle Management (DOORS Next, Engineering Test Management), Siemens Polarion and PTC Codebeamer, and about what to check when one does not work.

## What the server offers

| Address | Use |
|---|---|
| `<base_url>/api/v1/oslc/rootservices` | Jazz-style discovery document (public, holds no data). IBM ELM and tools that use "friends" start here. |
| `<base_url>/api/v1/oslc/catalog` | Service provider catalog: one service provider per project the account may read. |
| `<base_url>/api/v1/oslc/projects/<project id>/catalog` | A catalog with one project only, for tools that connect one of their projects to one project here. |
| `<base_url>/api/v1/oslc/projects/<project id>` | The service provider of a project: query capabilities, creation factories, selection and creation dialogs, resource shapes. |

The project id is the one in the web address of the project (`/p/<id>/…`). Resources are served as RDF/XML (the OSLC default), Turtle or JSON-LD; request bodies are accepted in the same formats. The API reference (`docs/api.md`, section *OSLC*) lists every property, the link properties and the query syntax.

## Before you connect a tool

1. **Base URL.** Set `server.base_url` (or **Base URL** in *System settings*) to the address the other tool reaches Qtestframe at, for example `https://qtf.example.com`. Every OSLC resource address is built from it; behind a reverse proxy it must be the public address, not the internal one.
2. **TLS.** Most ALM servers refuse plain HTTP and certificates they do not trust. Use a certificate of a CA the other tool's Java trust store knows (or add your CA to it).
3. **A service account and an API token.** OSLC clients authenticate with an API token, either as `Authorization: Bearer <token>` or as the password of Basic authentication (any user name), on the OSLC paths only. OAuth 1.0a, which IBM ELM uses between Jazz applications, is not supported; use the Basic/token form in the other tool's "custom" or "basic" connection type. Create a user (for example `alm-connector@example.com`) and give it the role the connection needs:
   - **Viewer** to let the tool read and link (the link is stored in the other tool; Qtestframe shows it there).
   - A role with `requirement.write` and/or `test.write` to let the tool create and change items, and `test.write` to change trace links (*validatedBy* / *validatesRequirement*).
   Sign in as that user, create the token under **Administration → API tokens** with the role the connection needs (a token can have a lower role than its account, never a higher one) and give it to the administrator of the other tool. Restricted projects are only visible when the account is a member.
4. **Dialogs in a frame** (optional). The selection and creation dialogs open in a window of their own, which works everywhere. Tools that show them inside their own page (an iframe) need two things:
   - the tool's origin in `server.frame_ancestors`, for example
     ```yaml
     server:
       base_url: https://qtf.example.com
       frame_ancestors:
         - https://elm.example.com
         - https://polarion.example.com
     ```
     The dialog pages then send `Content-Security-Policy: frame-ancestors 'self' https://elm.example.com …`; without the setting they may only be framed by Qtestframe itself.
   - the browser must send Qtestframe's session cookie inside the frame. The cookie is `SameSite=Lax`, so the frame works when both tools are on the **same site** (subdomains of one registrable domain, such as `qtf.example.com` and `elm.example.com`). For tools on another site, leave the dialogs in a window.
   The dialogs use the user's own session: each user signs in to Qtestframe in the browser (the dialog offers a link if they have not).

## IBM DOORS Next and Engineering Test Management

DOORS Next links its requirements to test cases of a QM provider, and ETM links its test cases to requirements of an RM provider. Qtestframe can take the RM role (its requirements) and the QM role (its test cases).

1. In the ELM **Lifecycle Project Administration** (or the project area's **Associations**), add an association to an **OSLC provider that is not a Jazz application**: choose *Add → Service provider catalog* (wording varies by version) and enter `<base_url>/api/v1/oslc/catalog`, or `<base_url>/api/v1/oslc/projects/<id>/catalog` for one project. When the version only accepts friends, register a friend with the root services `<base_url>/api/v1/oslc/rootservices` and choose the basic authentication method with the service account's e-mail and the API token as password.
2. Choose the association type: **Provides Requirements** (or *Uses Requirements* from the ETM side) for Qtestframe requirements, **Provides Test Cases** (*Validated By*) for Qtestframe test cases.
3. In a DOORS Next artifact, **Links → Add link → Validated By** now offers *Qtestframe* as a target: the selection dialog opens, the user searches and ticks test cases, and DOORS Next stores the links. **Create** opens Qtestframe's creation dialog instead.

Links stored by DOORS Next live in DOORS Next. When a tool writes the link into the Qtestframe resource as well (a PUT with `oslc_qm:validatesRequirement` or `oslc_rm:validatedBy`), Qtestframe keeps it as a trace link when the other end is one of its own items, or as an item link to the other tool's address otherwise.

## Siemens Polarion

Polarion links work items to OSLC resources of other servers through *OSLC connections*:

1. In **Administration → Linked data / OSLC → Remote OSLC providers** (Polarion's *Connections* configuration), add a provider with the catalog `<base_url>/api/v1/oslc/catalog`, authentication **Basic**, user the service account's e-mail and password the API token.
2. Map the link roles: a Polarion requirement *is validated by* → Qtestframe test cases (`oslc_qm:TestCase`), a Polarion test case *verifies* → Qtestframe requirements (`oslc_rm:Requirement`).
3. Polarion users then pick Qtestframe items in the **Link to OSLC resource** dialog, which opens Qtestframe's selection dialog.

For a one-off or regular transfer of a Polarion document, use ReqIF instead: export the document as ReqIF in Polarion and import it in Qtestframe with *Import → ReqIF … with updates*; map the work item types (for example *Test Case* to test cases) in the import preview.

## PTC Codebeamer

Codebeamer connects to OSLC providers as *Remote trackers / OSLC associations*:

1. In the project's **Admin → Integrations → OSLC**, add a server with the root services or catalog address above, authentication **Basic** with the service account and the API token.
2. Choose the remote service: *Requirements* (Qtestframe requirements) or *Test cases* (Qtestframe test cases), and the Codebeamer association type it should create (for example *verifies* or *related*).
3. Codebeamer's association dialog then lists Qtestframe items; the delegated dialogs are used where Codebeamer supports them.

Codebeamer also exports and imports ReqIF; its *Test Case* trackers map to Qtestframe test cases in the import preview.

## Other OSLC clients and scripts

Any OSLC 2.0 client works with the catalog and a token. A quick check from the command line:

```sh
curl -H "Authorization: Bearer $QTF_TOKEN" -H "Accept: text/turtle" \
  "https://qtf.example.com/api/v1/oslc/projects/<id>/requirements?oslc.where=qtf:status=%22approved%22&oslc.select=dcterms:title"
```

Creating a requirement:

```sh
curl -H "Authorization: Bearer $QTF_TOKEN" -H "Content-Type: text/turtle" -H "Accept: text/turtle" \
  --data-binary '<> a <http://open-services.net/ns/rm#Requirement> ; <http://purl.org/dc/terms/title> "Emergency stop" .' \
  "https://qtf.example.com/api/v1/oslc/projects/<id>/requirements"
```

The answer is `201 Created` with the new address in `Location` and the version in `ETag`. An update (PUT) with `If-Match: "<ETag>"` is refused with `412 Precondition Failed` when someone saved in the meantime, as in the web client.

## What is stored, audited and backed up

- Items created, changed or deleted through OSLC are ordinary items: they have history, the changes are in the audit log under the account of the token, and deletion moves them to the project's trash.
- Trace links, relations and item links written through OSLC are ordinary links; links to resources of other tools are item links to their URL, labelled with the OSLC property (`oslc_rm:implementedBy` …).
- The ReqIF import records where every item, relation and trace link came from (table `external_refs`), so that importing the same document again updates them. This table is part of backups and of project archives.
- Nothing about a connection is stored in Qtestframe besides the API token: the other tool keeps the connection settings.

## Troubleshooting

| Symptom | Check |
|---|---|
| The tool says the catalog is empty | The account is not a member of the project (restricted projects), or the token's role has no read permission. Open the catalog address with the token in a browser or with `curl`. |
| Links point to `http://localhost:…` or an internal host | `server.base_url` is not the public address. |
| 401 Unauthorized | The token is wrong, expired or revoked; Basic authentication must carry the token as the password. Plain passwords are not accepted on the OSLC paths. |
| 403 Forbidden on create or update | The token's role lacks `requirement.write` / `test.write`, or `test.write` for trace links. |
| 412 Precondition Failed | The item changed since the tool read it; the tool must read it again (its ETag is the item's version). |
| 415 Unsupported Media Type | The tool sends a body that is not Turtle, RDF/XML or JSON-LD. |
| The dialog stays blank in a frame | The tool's origin is not in `server.frame_ancestors`, or the tools are on different sites so the browser does not send the session cookie: use the dialog in a window. |
| The dialog says *You are not signed in* | The user has no Qtestframe session in this browser: sign in in another tab and choose **Try again**. |

# Logging and monitoring

## The server log

The server writes structured log records (Go `log/slog`) to **standard error**; under systemd they go to the journal (`journalctl -u qtestframe -f`, `-o json` for the fields). Two formats:

| `logging.format` | Example |
|---|---|
| `text` (default) | `time=2026-11-22T09:14:03.120Z level=INFO msg="http request" method=GET path=/api/v1/projects status=200 bytes=5120 duration_ms=4 remote=10.0.0.7` |
| `json` | `{"time":"2026-11-22T09:14:03.120Z","level":"INFO","msg":"http request","method":"GET","path":"/api/v1/projects","status":200,…}` |

`logging.level` is `debug`, `info` (default), `warn` or `error`. Level and format can be changed at runtime in **Administration → System settings → Logging and syslog**, unless `QTF_LOG_LEVEL`/`QTF_LOG_FORMAT` set them (the container image sets the format to `json`).

What is logged:

- **Every request** except `/healthz` and `/readyz`: method, path (without the query), status, bytes, duration and client address (from `X-Forwarded-For` only for `trusted_proxies`). Requests answered with 5xx are logged at ERROR, the others at INFO.
- **Audit records** (`msg=audit`, INFO) with organisation, user, action, entity and id, one for every entry of the audit log.
- **Security events**: `login failed` and `second factor failed` (WARN, with e-mail and client address), configuration and key problems at start-up, the `hardening` line.
- **Background work**: migrations, purged sessions and trash, metric snapshots, e-mail and webhook failures, job resets.

Passwords, tokens, cookies and `Authorization` headers are never logged. Unexpected errors are answered with only "internal server error"; the details are in the log line `request failed` (path and error).

## Central syslog

Every record – the audit records included – can also go to a syslog server, as one JSON message per record:

```yaml
logging:
  level: info
  format: json
  syslog:
    enabled: true
    network: tls                 # udp | tcp | tls | unix | unixgram
    address: "logs.example.com:6514"
    tag: qtestframe
    facility: local0
    format: rfc5424              # or rfc3164
    ca_file: /etc/qtestframe/tls/ca.crt
    cert_file: /etc/qtestframe/tls/syslog-client.crt   # optional, mutual TLS
    key_file: /etc/qtestframe/tls/syslog-client.key
```

- Stream transports (`tcp`, `tls`, `unix`) use RFC 6587 octet counting; `tls` follows RFC 5425 and verifies the server against `ca_file` (or the system roots) with TLS 1.2 or later. A client certificate is made with `scripts/certs/gen-client-cert.sh`.
- Severities: ERROR → 3, WARN → 4, INFO → 6, DEBUG → 7. The message's APP-NAME is `tag`.
- Delivery is best effort: one reconnect per message, a 5-second write deadline, no disk queue. The journal keeps the local copy.
- With SELinux: `setsebool -P qtestframe_remote_syslog on`.
- **Send test message** in the system settings sends one message with the values of the form.

Receiver examples for rsyslog (`imtcp` with `gtls`) and syslog-ng (`network(transport("tls") flags(syslog-protocol))`) are in [docs/operations.md](https://github.com/rom/Qtestframe/blob/rom/docs/operations.md#8-logging-and-central-syslog). JSON payloads can be parsed into fields with rsyslog `mmjsonparse` or syslog-ng `json-parser()`.

## Health endpoints

| Endpoint | Answers | Use |
|---|---|---|
| `GET /healthz` | `200 {"status":"ok","version":"…"}` while the process serves requests | liveness probe |
| `GET /readyz` | `200 {"status":"ready"}`, or `503` when the database does not answer within 2 seconds | readiness probe, load balancer |

Both need no sign-in and are not written to the request log. `qtestframe healthcheck` probes `/healthz` of the local server for container health checks.

There is **no metrics endpoint** (no Prometheus exporter). The dashboard *trends* are project data (daily snapshots), not server metrics.

## What to monitor

| Signal | How |
|---|---|
| liveness and readiness | `/healthz`, `/readyz` (e.g. Prometheus `blackbox_exporter`, module `http_2xx`) |
| service state | `systemctl is-active qtestframe`; the unit restarts the server on failure |
| errors | log records with `level=ERROR`; `stored system settings are invalid`; `tls reload failed` |
| attacks | many `login failed`, `second factor failed` or SCIM authentication failures from one address |
| certificate expiry | `scripts/certs/verify-cert.sh -q --connect qtf.example.com:8443 --warn-days 21` (exit 2 = expiring) |
| resources | `systemctl status qtestframe` (memory and tasks against `MemoryMax`/`TasksMax`), free disk for the database and backups |
| backups | the age of the newest backup and of the last restore test |
| key of the stored secrets | `qtestframe secrets status` shows no plaintext secrets and the expected key |

A simple cron alternative to a monitoring system:

```sh
curl -fsS --max-time 5 https://qtf.example.com/readyz >/dev/null || echo "qtestframe not ready" | mail -s ALERT ops@example.com
```

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

**In a backup** (99 tables, in restore order): `tenants`, `users`, `memberships`, `api_tokens`, `projects`, `configurations`, `releases`, `folders`, `categories`, `requirements`, `requirement_categories`, `evaluation_criteria`, `test_cases`, `test_steps`, `test_case_categories`, `trace_links`, `variants`, `builds`, `test_cycles`, `test_cycle_tests`, `test_runs`, `run_configurations`, `test_executions`, `execution_iterations`, `step_results`, `audit_log`, `revisions`, `requirement_relations`, `baselines`, `baseline_items`, `comments`, `files`, `file_blobs`, `attachments`, `report_definitions`, `login_events`, `roles`, `test_dependencies`, `trash`, `item_links`, `release_requirements`, `defects`, `defect_links`, `risks`, `risk_requirements`, `notifications`, `webhooks`, `review_requests`, `baseline_signoffs`, `scim_tokens`, `scim_settings`, `scim_users`, `scim_groups`, `scim_group_members`, `metric_snapshots`, `trace_rules`, `document_items`, `defect_regressions`, `consistency_dismissals`, `invitations`, `report_subscriptions`, `user_mfa`, `mfa_recovery_codes`, `mfa_policies`, `project_members`, `waivers`, `waiver_requirements`, `waiver_signoffs`, `soa_decisions`, `system_settings`, `external_refs`, `link_criteria`, `system_elements`, `requirement_elements`, `test_elements`, `verification_settings`, `verification_records`, `verification_record_files`, `item_applicability`, `code_changes`, `code_change_items`, `code_settings`, `custom_fields`, `field_set_fields`, `custom_field_archive`, `item_templates`, `org_item_templates`, `project_template_defaults`, `exploratory_sessions`, `exploratory_scope`, `exploratory_notes`, `test_cycle_schedules`, `secret_keys`, `chat_channels`, `change_control_settings`, `change_requests`, `change_request_items`, `change_request_signoffs`, `change_request_events`.

**Not in a backup:** `chat_deliveries`, `ci_deliveries`, `job_runs`, `mfa_challenges`, `mfa_trusted_devices`, `schema_migrations`, `script_jobs`, `sessions`, `similarity_embeddings`, `suggest_history`.

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
- this server's **key cannot unwrap** the backup's data keys (`the backup's encrypted secrets cannot be decrypted with this server's key`). Configure the key the backup was made with – the same key file or `QTF_SECRETS_KEY`, the same TPM, the same HSM key – or see [disaster recovery](#disaster-recovery-for-tpm-sealed-keys);
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
| 38 | `0038_owners_worklist.sql` |
| 39 | `0039_discussions.sql` |
| 40 | `0040_exploratory.sql` |
| 41 | `0041_test_cycles.sql` |
| 43 | `0043_similarity.sql` |
| 45 | `0045_chat_channels.sql` |
| 46 | `0046_variant_gates.sql` |
| 47 | `0047_risk_catalog.sql` |
| 49 | `0049_risk_attack.sql` |
| 50 | `0050_project_creator.sql` |
| 51 | `0051_frameworks.sql` |
| 52 | `0052_multiple_roles.sql` |
| 53 | `0053_ci_integration.sql` |
| 54 | `0054_change_requests.sql` |

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

# Operations runbook

On a systemd host the examples use this alias, which runs the command as the service user with the production configuration:

```sh
alias qtf='sudo -u qtestframe env QTF_CONFIG=/etc/qtestframe/config.yaml /usr/bin/qtestframe'
```

## Routine tasks

| When | Task |
|---|---|
| daily | Backup (`qtf backup --out …`) copied off-host; look at ERROR and WARN records (`journalctl -u qtestframe -p warning --since yesterday`). |
| weekly | Certificate expiry (`scripts/certs/verify-cert.sh -q --connect …`); disk space of the database and the backups; failed sign-ins and SCIM failures. |
| monthly | A **restore test** into a scratch installation (with a copy of the key); `qtf secrets status`; review super administrators (**Administration → Users**), API tokens that were not used for a long time, SCIM tokens. |
| per release | Upgrade (see [Upgrading](#upgrading)); compare `deploy/config.example.yaml` with your configuration. |
| yearly, or by policy | `qtf secrets rotate` (new data key), then `qtf secrets rotate --prune` once every server process has restarted; with a TPM, refresh the [recovery backup](#disaster-recovery-for-tpm-sealed-keys). |

Other common tasks:

| Task | How |
|---|---|
| reload the TLS certificate | `sudo systemctl reload qtestframe` |
| change the log level for a while | **Administration → System settings → Logging and syslog**, or `QTF_LOG_LEVEL=debug` in a drop-in and a restart |
| a user forgot the password | organisation admin: **Members → Reset password**; super admin: **Users**. There is no password reset on the command line; if no super administrator can sign in, create a new one (below) |
| a user lost the authenticator and the recovery codes | **Members → Reset MFA** (organisation admin) or **Users** (super admin), after verifying the person |
| lock everyone out quickly | disable the account under **Users** (ends its sessions); revoke its API tokens; for SSO accounts disable them at the identity provider as well |
| the last super administrator is gone | on the host: `qtf user create --email … --password-stdin --super-admin` creates a new one |
| check a configuration change | `qtf check-config` before the restart |

## Capacity

- **SQLite** serialises writes through one connection. It comfortably serves tens of concurrent users and databases of a few GB on a local SSD. When requests start to queue (rising `duration_ms` in the request log under load), move to PostgreSQL.
- **PostgreSQL**: `database.max_open_conns` (default 20) per server process; the database server must allow the sum over all processes.
- **Files** are stored in the database. `files.max_upload_mb` limits one file; the database and every backup grow with the attachments.
- **Memory**: the unit caps the server at 2 GB (`MemoryMax`, `MemoryHigh=1536M`). Large imports, exports and reports need memory while they are built; web tests start a browser per job, so raise `MemoryMax` with `runner.web.max_parallel`.
- **Timeouts**: `server.write_timeout` (default 2 minutes) bounds downloads, exports and synchronous script runs; raise it for very large exports, or use the command line for backups.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| the service does not start | `journalctl -u qtestframe -b -e`; `qtf check-config` lists configuration errors. |
| `config: parse …: field listn not found` | a misspelt or removed key; the file is decoded strictly. |
| `server.base_url is required for e-mail` | set `server.base_url`, or turn e-mail off. |
| `secrets: the stored data-encryption keys are wrapped by another key: … the configured key is …` | the server has a different key than the one the data was encrypted with: wrong `key_file`, `QTF_SECRETS_KEY`, another TPM or HSM key. Configure the right key, or re-wrap with `secrets rewrap --from-config OLD.yaml`. |
| `secrets: create the key file …` | the default key location is not writable; set `secrets.local.key_file` or `QTF_SECRETS_KEY`. |
| `secrets: TPM: unseal failed; the PCRs … have changed` | firmware or boot chain changed with PCR binding: boot the previous state and re-wrap, or use the recovery backup. |
| `secrets: TPM: load the sealed key (was it sealed by another TPM, or was the TPM cleared?)` | the data was sealed on another machine or the TPM was cleared; see [disaster recovery](#disaster-recovery-for-tpm-sealed-keys). |
| `secrets: open TPM /dev/tpmrm0: …` | the service cannot open the device: drop-in with `PrivateDevices=no`, `DeviceAllow=/dev/tpmrm0 rw`, group `tss`. |
| `secrets: the pkcs11 (HSM) key provider is not available in this binary` | build with `make go-pkcs11`. |
| `secrets: pkcs11: log in to token … (check the PIN)` / `no token with …` / `has no AES key labelled …` | PIN, token label or slot, or key label; `generate_key: true` creates the key. |
| `hardening: …` and the start stops | `no_new_privs` failed; check the container runtime or init system. |
| `permission denied` for a file after start-up | the path is not in the Landlock rules: add it to `hardening.read_only_paths` / `read_write_paths` (it must exist at start-up). |
| `landlock: …` in `skipped` | kernel without Landlock; add `landlock` to the `lsm=` boot parameter, or accept it. |
| `Operation not permitted` in a system call | a systemd `SystemCallFilter` or the seccomp list; confirm with `ausearch -m SECCOMP` and report it. |
| SELinux or AppArmor denials | `ausearch -m AVC -ts recent -c qtestframe` and `getsebool -a \| grep qtestframe`; `journalctl -k \| grep apparmor="DENIED"`. |
| bind on `:443` fails | `CAP_NET_BIND_SERVICE` removed by a drop-in, or SELinux `qtestframe_bind_http_port` off; or redirect 443 to 8443 with nftables. |
| nobody can sign in over plain HTTP | `auth.cookie_secure` must be `false` without TLS. |
| `429 rate_limited` at sign-in | `auth.login_max_attempts` within `auth.login_lockout` per address and e-mail; behind a proxy, set `server.trusted_proxies` or everyone shares one address. |
| single sign-on loops, `oidc: state mismatch` | `server.base_url` must match the browser's address; session affinity with several instances; the login took over 10 minutes; clock skew. |
| `external login failed … oidc …: discovery` | the server cannot reach the issuer: egress firewall, proxy, SELinux `qtestframe_connect_http`. |
| `external login failed … e-mail address not verified` / `no e-mail claim` | the IdP sends `email_verified: false` or no `email_claim`. |
| `external login failed … user not provisioned` | the user does not exist and `auto_provision` is off. |
| `external login failed … e-mail already bound to another identity provider` | an account with that e-mail is linked to another provider. |
| `403 mfa_enrolment_required` for API calls of a browser session | the MFA policy requires a second factor; the user must set one up. |
| `stored system settings are invalid and were ignored` | a file change made a stored setting invalid; fix or reset it under **System settings**. |
| `backup schema version N does not match the database schema version M` | restore with the version that made the backup, then upgrade. |
| PostgreSQL TLS errors | `sslrootcert` readable by the service; `sslmode=verify-full` needs the server name in the certificate. |
| web tests fail at once | `MemoryDenyWriteExecute=no` and `ExecPaths=` for the Playwright directory; `/dev/shm` writable; `runner.work_dir` short (socket path under 108 bytes). |
| scripts: `409` with `not_ready` | the test execution settings forbid the result; see the run's readiness column. |

## Log messages to look for

| Message | Level | Meaning |
|---|---|---|
| `qtestframe listening` | INFO | the server is up (address, TLS, version, database) |
| `hardening` | INFO | which restrictions were applied and which skipped |
| `secrets encrypted` | INFO | the key provider, key fingerprint and active data key in use |
| `secrets: created the key file …` | WARN | a new key file was created: back it up now |
| `secrets: the key file … is readable by group or others` | WARN | fix the mode with `chmod 600` |
| `secrets: encrypted N secret(s) that were stored as plaintext` | WARN | values of an older version were encrypted at this start |
| `applied database migrations` | INFO | an upgrade migrated the schema |
| `bootstrapped administrator` | INFO | the first administrator was created |
| `no users exist; …` | WARN | there is no account yet and no bootstrap configuration |
| `stored system settings are invalid and were ignored; …` | ERROR | see above |
| `login failed`, `second factor failed` | WARN | a failed sign-in, with e-mail and address |
| `external login failed` | WARN | a failed OIDC or SAML sign-in, with the reason |
| `scim authentication failed` | WARN | a request with a wrong SCIM token |
| `request failed` | ERROR | an unexpected error behind a `500` answer, with path and error |
| `tls certificate reloaded` / `tls reload failed; keeping previous certificate` | INFO / ERROR | the result of a `SIGHUP` |
| `webhook delivery` | WARN | a webhook could not be delivered |
| `digest job`, `report subscriptions job` | WARN | scheduled e-mail failed |
| `reset interrupted job` | INFO | a web test job of a stopped process was reset |
| `audit write failed` | WARN | an audit entry could not be stored (the log record is still written) |
| `shutting down` | INFO | the server received `SIGTERM` or `SIGINT` |

# Command line reference

`qtestframe` is the server and the administration tool. Every command except `report`, `gate`, `select`, `version`, `secrets generate-key` and `healthcheck --url` reads the configuration (`--config FILE`, default `$QTF_CONFIG`, then the built-in defaults and `QTF_*` variables). The commands that open the database migrate it first.

```text
qtestframe <command> [flags]
qtestframe help                 # the overview
qtestframe <command> -h         # the flags of a command
```

- Run administration commands **as the service user** (`sudo -u qtestframe …`), so files such as a new key file get the right owner.
- Exit status: `0` on success (also for `-h`), `1` on an error (printed as `error: …`), `2` for an unknown command. `gate`: `0` the gate passed, `1` it failed, `2` it could not be evaluated.
- Flags take one or two dashes (`-config` and `--config`). Passwords are only read from standard input.
- `qtestframe sandbox-exec` is internal: the server starts it to sandbox scripts. Do not call it yourself.

The descriptions below are written for this guide; the flag tables are generated from the binary's own flag definitions.

<!-- BEGIN GENERATED: cli (make admin-guide-ref) -->

### `qtestframe serve`

Runs the HTTP(S) server. Migrates the database, opens the key provider, creates the bootstrap administrator when there are no users, applies the system settings stored in the database, locks the process down and listens. Stops on `SIGINT`/`SIGTERM` (15 s for requests in flight); `SIGHUP` re-reads the TLS certificate.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--listen` | `string` | listen address host:port (overrides server.listen) |
| `-p` | `int` | shorthand for --port |
| `--port` | `int` | listen port (overrides the port of server.listen) |

### `qtestframe migrate`

Applies the pending database migrations and prints how many. `serve` does the same at every start.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |

### `qtestframe user create`

Creates a user. With `--password-stdin` the password is read as one line from standard input (at least 10 characters with a letter and a non-letter); without it the user can only sign in with single sign-on.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--email` | `string` | e-mail address |
| `--name` | `string` | display name |
| `--password-stdin` | — | read the password from stdin |
| `--super-admin` | — | grant global super administrator rights |

### `qtestframe tenant create`

Creates an organisation (tenant).

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--name` | `string` | tenant name |
| `--slug` | `string` | tenant slug |

### `qtestframe member add`

Gives a user a role in an organisation, or changes it. The role is a built-in role or a custom role of the organisation.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--email` | `string` | user e-mail |
| `--role` | `string` | role, or several roles separated by commas (e.g. sme,tester) (default "viewer") |
| `--tenant` | `string` | tenant slug |

### `qtestframe seed-demo`

Adds the demo projects to an organisation (default `bootstrap.tenant_slug`, created when missing), with demo users. Not for production.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--tenant` | `string` | tenant slug (default: bootstrap tenant) |

### `qtestframe backup`

Writes a logical backup of all data (every organisation) to a ZIP file, through a temporary file that is renamed when complete. Audited as `admin.backup`.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--out` | `string` | backup file to write (ZIP) |

### `qtestframe restore`

Replaces **all** data with a backup, in one transaction, and signs everyone out. Needs `--yes`. Audited as `admin.restore`.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--in` | `string` | backup file to restore (ZIP) |
| `--yes` | — | confirm that ALL existing data is replaced |

### `qtestframe secrets`

The `secrets` commands share one set of flags; each uses the ones that concern it.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--from-config` | `string` | rewrap: configuration file of the key provider that wrapped the keys |
| `--prune` | — | rotate: delete the old data-encryption keys no value uses any more |
| `--yes` | — | decrypt: confirm that the secrets are stored as plaintext |

#### `qtestframe secrets status`

Shows the key provider, its key fingerprint, the data-encryption keys and how many stored secrets are encrypted. Changes nothing (creates no key file).

#### `qtestframe secrets migrate`

Encrypts the secrets still stored as plaintext (also done at every start).

#### `qtestframe secrets rotate`

Creates a new data-encryption key and re-encrypts every secret with it; `--prune` then deletes the keys no value uses.

#### `qtestframe secrets rewrap`

Wraps the data-encryption keys with the configured key provider; `--from-config OLD.yaml` names the configuration of the provider that wrapped them.

#### `qtestframe secrets decrypt`

Stores every secret as plaintext again (before a downgrade). Needs `--yes`.

#### `qtestframe secrets generate-key`

Prints a new random 256-bit key (base64) for `QTF_SECRETS_KEY` or a key file.

### `qtestframe check-config`

Loads and validates the configuration (secret files included) without touching the database, and prints `configuration OK`.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--listen` | `string` | listen address host:port (overrides server.listen) |
| `-p` | `int` | shorthand for --port |
| `--port` | `int` | listen port (overrides the port of server.listen) |

### `qtestframe healthcheck`

Requests `/healthz` of the local server (URL from `server.listen`, `https` with TLS, certificate not verified) and exits with 0 on HTTP 200. For container health checks without curl.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--listen` | `string` | listen address host:port (overrides server.listen) |
| `-p` | `int` | shorthand for --port |
| `--port` | `int` | listen port (overrides the port of server.listen) |
| `--url` | `string` | URL to probe (default derived from server.listen) |

### `qtestframe report`

Uploads JUnit XML or JSON results of a CI job to a run on a server, through the REST API with an API token. Further arguments are report files (`*.json` as JSON, others as JUnit XML). In GitLab CI, GitHub Actions and Jenkins the pipeline is recorded as the build of the results (`--ci none` turns that off); `--post-status` and `--post-comment` post the commit status `qtestframe/tests` and a merge request note.

| Flag | Value | Meaning |
|---|---|---|
| `--branch` | `string` | branch of the build (default from the CI variables) |
| `--build` | `string` | build of a run created with --create-run (default the CI pipeline number) |
| `--build-id` | `string` | CI build id, unique in the project (default from the CI variables) |
| `--build-number` | `string` | CI build number (default from the CI variables) |
| `--ci` | `string` | record the CI build (pipeline id and number, commit, branch, merge request) of the results: auto (from the CI variables) or none (default "auto") |
| `--code-host` | `string` | where to post: auto (from the CI variables), gitlab or github (default "auto") |
| `--commit` | `string` | commit SHA of the build (default from the CI variables) |
| `--configuration` | `string` | configuration (name or id) the results were obtained in; added to the run when it does not target it yet (default $QTF_CONFIGURATION) |
| `--create-missing` | — | create draft automated tests for JUnit test cases without a test key (needs test.write) |
| `--create-run` | — | create the run named by --run when the project has none |
| `--environment` | `string` | environment of a run created with --create-run |
| `--fail-on-unknown` | — | exit with an error when results match no test (unknown keys or test cases without a key) |
| `--gitlab-comment` | — | same as --post-comment |
| `--gitlab-status` | — | same as --post-status |
| `--json` | `value` | results in the JSON import format {"results":[...]} (file or glob; repeatable) |
| `--junit` | `value` | JUnit XML report (file or glob; repeatable) |
| `--merge-request` | `string` | merge / pull request of the build (default from the CI variables) |
| `--pipeline-url` | `string` | pipeline URL of the build (default from the CI variables) |
| `--post-comment` | — | add or update a merge / pull request note with the summary (default $QTF_POST_COMMENT) |
| `--post-status` | — | post a commit status to the code host (default $QTF_POST_STATUS) |
| `--project` | `string` | project key, e.g. SHOP (default $QTF_PROJECT) |
| `--run` | `string` | name of the test run to report into |
| `--run-id` | `string` | id of the test run to report into |
| `--token` | `string` | API token (default $QTF_TOKEN) |
| `--url` | `string` | server base URL, e.g. https://qtestframe.example.com (default $QTF_URL) |

### `qtestframe gate`

Checks the quality gate of a CI pipeline on a server: the exit criteria of a release (`--release`, optionally `--variant`) or the conditions of `--require` (`must`, `no_failed`, `no_stale`, `no_blocking_defects`, `coverage>=80`, `no_open_defects:major`, `cycle_executed:NAME>=90`, …) on the results of a run (`--run`) or build (`--build`). Prints a summary with the failing items and links; `--format junit|markdown|json --output FILE` also writes a report for the pipeline. Exit code 0 when the gate passes, 1 when it fails, 2 when it cannot be evaluated. `--post-status` and `--post-comment` post a commit status and a merge request note (GitLab or GitHub, see the user guide page CI/CD).

| Flag | Value | Meaning |
|---|---|---|
| `--build` | `string` | build (CI id or number) whose runs' results are checked |
| `--code-host` | `string` | where to post: auto (from the CI variables), gitlab or github (default "auto") |
| `--commit` | `string` | commit of the change (default from the CI variables) |
| `--format` | `string` | output: text, json, junit or markdown (default "text") |
| `--gitlab-comment` | — | same as --post-comment |
| `--gitlab-status` | — | same as --post-status |
| `--merge-request` | `string` | merge / pull request of the change (default from the CI variables) |
| `--output` | `string` | write the output to this file (the text summary still goes to the log) |
| `--post-comment` | — | add or update a merge / pull request note with the summary (default $QTF_POST_COMMENT) |
| `--post-status` | — | post a commit status to the code host (default $QTF_POST_STATUS) |
| `--project` | `string` | project key, e.g. SHOP (default $QTF_PROJECT) |
| `--release` | `string` | release (name, version or id) whose exit criteria are checked |
| `--require` | `value` | condition, e.g. coverage>=80, must, no_failed, no_stale, no_blocking_defects, no_open_defects:major, cycle_executed:NAME>=90 (repeatable, or comma-separated); replaces the release's exit criteria |
| `--run` | `string` | run (name or id) whose results are checked |
| `--run-id` | `string` | id of the run whose results are checked |
| `--scope` | `string` | requirements of a gate without --release: all (default), affected (those the merge request / commit refers to) or a query |
| `--token` | `string` | API token (default $QTF_TOKEN) |
| `--url` | `string` | server base URL, e.g. https://qtestframe.example.com (default $QTF_URL) |
| `--variant` | `string` | evaluate for this variant (name or id) only |

### `qtestframe select`

Prints the tests a CI pipeline runs for a change: the requirements and tests that the change's commits (from git, `--since`, or `--commits`), merge request, changed files (with the path map `qtestframe-map.yml`) and commit messages refer to, the regression selection and the tests with the `--tag` tags. `--format` gives test keys, a test runner filter (`pytest`, `go`, `playwright`, `maven`), JSON or a GitLab child pipeline (`gitlab-child-pipeline`); `--create-run` creates a run of the selection.

| Flag | Value | Meaning |
|---|---|---|
| `--all` | — | select every test |
| `--cli-image` | `string` | image with the qtestframe binary for the child pipeline's report job (default $QTF_CLI_IMAGE) |
| `--commits` | `value` | commit SHAs of the change (repeatable or comma-separated; instead of git) |
| `--create-run` | — | create a run of the selected tests (results of report --run-id land in it) |
| `--environment` | `string` | environment of the created run |
| `--fallback` | `string` | when the change is unknown (no commits, files or keys): none (select nothing) or all (default "none") |
| `--files` | `value` | changed files (repeatable or comma-separated; instead of git) |
| `--format` | `string` | output: keys, pytest, go, playwright, maven, json, gitlab-child-pipeline (default "keys") |
| `--gate-release` | `string` | release whose exit criteria the child pipeline's gate checks |
| `--gate-require` | `value` | condition of the child pipeline's gate (repeatable) |
| `--git` | — | read the commits (messages) and changed files with git (default true) |
| `--head` | `string` | the revision of the change (default $CI_COMMIT_SHA, else HEAD) |
| `--image` | `value` | image of a runner's job in the child pipeline, RUNNER=IMAGE (repeatable) |
| `--include-manual` | — | select manual tests too |
| `--include-related` | — | add requirements that depend on, refine or derive from a changed one |
| `--job` | `value` | test command of a runner's job in the child pipeline, RUNNER=COMMAND (repeatable) |
| `--map` | `string` | path map: changed files → tests, tags, requirements (default qtestframe-map.yml when it exists) |
| `--merge-request` | `string` | merge / pull request number (default from the CI variables) |
| `--no-gate` | — | no gate in the child pipeline |
| `--output` | `string` | write the output to this file |
| `--project` | `string` | project key, e.g. SHOP (default $QTF_PROJECT) |
| `--release` | `string` | release (name, version or id) of the created run |
| `--requirement` | `value` | changed requirement (key or id; repeatable) |
| `--run` | `string` | name of the run to create (default $QTF_RUN) |
| `--since` | `string` | compare with this git revision (default: the merge request's diff base, else the commit before the push) |
| `--since-build` | `string` | also the commits after this build (CI id or number) |
| `--tag` | `value` | always run the tests with this tag (repeatable) |
| `--token` | `string` | API token (default $QTF_TOKEN) |
| `--url` | `string` | server base URL, e.g. https://qtestframe.example.com (default $QTF_URL) |
| `--variant` | `string` | only tests that apply to this variant |

### `qtestframe stubs`

Writes generated test code stubs into a directory (`--out`): one file per test of `--query` (default `execution_type:automated`) or `--ids`, for `--framework` `pytest`, `playwright`, `go`, `junit5`, `jest`, `vitest`, `robot` or `cucumber`, named after the test key so that JUnit results map back, and marked as skipped until implemented. Generating again creates new files, regenerates unchanged stubs, only updates the header block of implemented ones and leaves files without qtestframe markers alone; `--dry-run` lists what would change. `--mark-script` sets the script of tests without one to their stub path (see the user guide page CI/CD).

| Flag | Value | Meaning |
|---|---|---|
| `--dry-run` | — | only print what would be written |
| `--framework` | `string` | test framework: pytest, playwright, go, junit5, jest, vitest, robot, cucumber |
| `--ids` | `value` | only these tests (keys or ids; repeatable or comma-separated) |
| `--mark-script` | — | set the script of tests without one to their stub path (needs test.write) |
| `--out` | `string` | directory to write the stubs to (default ".") |
| `--package` | `string` | Go package (default: the name of --out, else tests) or Java package of the stubs |
| `--project` | `string` | project key, e.g. SHOP (default $QTF_PROJECT) |
| `--query` | `string` | the tests to generate stubs for (query language); "" for all (default "execution_type:automated") |
| `--script-dir` | `string` | directory of the stubs in the repository for --mark-script (default --out when it is relative) |
| `--token` | `string` | API token (default $QTF_TOKEN) |
| `--url` | `string` | server base URL, e.g. https://qtestframe.example.com (default $QTF_URL) |

### `qtestframe version`

Prints the version (also `--version`, `-v`).

<!-- END GENERATED: cli -->

## Examples

```sh
# first super administrator and an organisation
read -rs PW && printf '%s\n' "$PW" | qtestframe user create --email admin@example.com --password-stdin --super-admin
qtestframe tenant create --slug acme --name "Acme Corp"
qtestframe member add --tenant acme --email admin@example.com --role admin

# backup and restore
qtestframe backup --out /var/backups/qtestframe/qtf.zip
qtestframe restore --in /var/backups/qtestframe/qtf.zip --yes

# keys of the stored secrets
qtestframe secrets status
qtestframe secrets rewrap --config config.new.yaml --from-config config.yaml

# CI: report JUnit results into a run, creating it when missing
QTF_URL=https://qtf.example.com QTF_TOKEN=qtf_… \
  qtestframe report --project SHOP --run "Nightly $BUILD" --create-run --build "$BUILD" --junit 'reports/*.xml'

# CI: the quality gate of the pipeline's run, as JUnit for the pipeline
qtestframe gate --project SHOP --run "Nightly $BUILD" --release 1.0 --format junit --output qtf-gate.xml

# CI: the tests of a merge request as a pytest filter, or as a child pipeline
pytest -k "$(qtestframe select --project SHOP --format pytest)"
qtestframe select --project SHOP --format gitlab-child-pipeline --create-run --run "MR $CI_MERGE_REQUEST_IID" --output child.yml
```

Ready-made pipeline definitions are in `examples/ci/` and in the app (**Settings → Project → CI setup**); see the user guide page *CI/CD*.

# Features added later

This chapter collects the server-side administration notes of features that were added after the rest of this guide was written: configuration keys, permissions, background jobs, data that a backup does or does not contain, and outgoing connections. Each feature that needs such a note gets a section below; until then, the [user guide](https://github.com/rom/Qtestframe/blob/rom/docs/user-guide/en/administration.md) describes the feature's screens.

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

The quality gate, the test selection and the pipeline templates (migration 0053) need no configuration; the server-side posting of GitLab commit statuses and merge request notes is a project setting with an encrypted token and a background outbox (`ci_deliveries`, retried by the minute job, not backed up). Tokens, roles, the job token's limits and outgoing connections are described in [E-mail, webhooks and integrations](#e-mail-webhooks-and-integrations); changes of the settings are audited as `project.ci_settings`.

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
