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
| Command line | `qtestframe migrate`, `user create`, `backup`, `secrets …` and others open the database directly; see the [command line reference](cli-reference.md). |
| Script runner | Optional (`runner.enabled`). Runs the scripts of automated tests on the server, each in a sandboxed child process (`qtestframe sandbox-exec`). |
| Web test runner | Optional (`runner.web.enabled`). Runs Playwright tests with Node.js and a real browser as background jobs. |

## Data stores

| Store | Content | Notes |
|---|---|---|
| **Database** (SQLite or PostgreSQL) | Everything: organisations, users, projects, requirements, tests, runs, history, comments, the audit log, sessions, **uploaded files** (attachments, documents, reports, avatars) and the system settings changed in the web client. | Created and upgraded automatically by the [migrations](backup-and-migration.md#migrations). |
| **Key of the stored secrets** | The key-encryption key that protects the secrets the server must read back (TOTP secrets, webhook secrets, tracker tokens, secret test variables, SMTP password and OIDC client secrets stored in the web client). | A key file (`qtestframe-secrets.key`), the `QTF_SECRETS_KEY` variable, a TPM 2.0 or an HSM; see [Encryption of stored secrets](security.md#encryption-of-stored-secrets). **Never in the database.** |
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

Because project administrators choose the URLs of trackers and webhooks, and script authors the targets of web tests, filter the server's outgoing traffic when it can reach internal services (see [Security](security.md#outgoing-connections)).

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

How to choose between the databases is described under [Installation](installation.md#sqlite-or-postgresql).
