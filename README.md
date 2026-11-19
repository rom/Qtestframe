# Qtestframe

Qtestframe is a multi-user, multi-tenant tool for managing requirement specifications and tests, similar in scope to HP/Micro Focus ALM. It is a single Go binary with an embedded Vue 3 + PrimeVue web client. Data lives in **SQLite** (zero setup) or **PostgreSQL**.

- [Features](#features)
- [Quick start](#quick-start)
- [Configuration](#configuration)
- [Using Qtestframe](#using-qtestframe)
- [Users, tenants and roles](#users-tenants-and-roles)
- [Automation and CI](#automation-and-ci)
- [Command line reference](#command-line-reference)
- [Development](#development)
- [Production deployment](#production-deployment)
- [Tests and coverage](#tests-and-coverage)
- [Documentation](#documentation)

## Features

- **Requirements**
  - Obligation levels based on RFC 2119/8174: `MUST`, `MUST NOT`, `SHALL`, `SHALL NOT`, `SHOULD`, `SHOULD NOT`, `MAY`, `OPTIONAL`, `WILL` and `DESCRIBE`. The level is detected automatically from the text.
  - Live language checks while you write: ambiguous terms, TBDs, compound requirements, escape clauses, and/or, and more.
  - Status, type, priority, rationale, source, owner, tags, version and **evaluation criteria** (test, inspection, analysis, demonstration, review).
- **Test cases**
  - Manual tests are executed step by step (action, expected result, data).
  - Automated tests are shell, bash, Python or JavaScript scripts, run server-side in a sandbox. CI systems can also import results.
- **Requirement ↔ test correlation:** every new requirement gets a linked draft test case by default, with one step per evaluation criterion, placed in a test folder that mirrors the requirement's folder.
- **Classification:** each project defines its own category dimensions. The defaults are Polarity (positive/negative), Interface (external/internal), Medium (file/network based), Execution (manual/automated), Level (unit to acceptance) and Area.
- **Grouping:** nested folders for requirements and tests. Drag and drop moves items, whole folders, or entire category dimensions, also to another project.
- **Views:** requirements, tests, operations (test runs and an execution runner), traceability matrix with coverage, dashboard, global search, administration.
- **Filtering and search on every field** with a query language: field operators, regular expressions, globs, comparisons, `AND`/`OR`/`NOT` and grouping.
- **Export** of a whole project or selected parts (requirements, criteria, tests, traceability, runs) to **XLSX, CSV, PDF and DOCX**.
- **Security:** local accounts, **OIDC** and **SAML 2.0** single sign-on, **RBAC** (admin, manager, tester, viewer + super admin), API tokens, audit log, in-process sandboxing (Landlock, seccomp, capabilities, no_new_privs).
- **Operations:** structured logs with optional **syslog** shipping (UDP, TCP, TLS, Unix socket), certificate scripts (local CA, self-signed, Let's Encrypt), hardened systemd unit, SELinux and AppArmor profiles, container image.

## Quick start

Works on Linux and macOS. You need Go (the version in `go.mod`, currently 1.26), Node.js ≥ 20 and `make`.

**1. Build** the web client and the server binary (`bin/qtestframe`):

```bash
make build
```

**2. Start** the server with the ready-to-use example configuration:

```bash
./bin/qtestframe serve --config examples/qtestframe.yaml
```

The server listens on port 8080, stores its SQLite database in `./data/` (created automatically) and creates an administrator on the first start.

**3. Sign in** at http://localhost:8080 as `admin@example.com` with the password `Change-me-1234`. Change it right away under the user menu → **Profile & password**.

**4. Optional: load demo data**, a sample "Online Banking" project with requirements, tests, links and a test run:

```bash
./bin/qtestframe seed-demo --config examples/qtestframe.yaml
```

Use another port with `-p`:

```bash
./bin/qtestframe serve --config examples/qtestframe.yaml -p 9090
```

For HTTPS, create a self-signed certificate for localhost in `./certs` and use the TLS example; then open https://localhost:8443:

```bash
make certs-dev
```

```bash
./bin/qtestframe serve --config examples/qtestframe-tls.yaml
```

## Configuration

The configuration is a **YAML** file passed with `--config FILE` (or the `QTF_CONFIG` environment variable). There are three files to start from:

| File | Use |
|---|---|
| [examples/qtestframe.yaml](examples/qtestframe.yaml) | working local setup: HTTP on :8080, SQLite in `./data`, script runner on, bootstrap admin |
| [examples/qtestframe-tls.yaml](examples/qtestframe-tls.yaml) | the same with HTTPS on :8443 and certificates from `make certs-dev` |
| [deploy/config.example.yaml](deploy/config.example.yaml) | fully commented production template: every option, OIDC/SAML, syslog, hardening |

Settings are applied in this order, later ones winning:

1. built-in defaults: `:8080`, SQLite file `./qtestframe.db`
2. the YAML file
3. `QTF_*` environment variables, e.g. `QTF_DATABASE_DSN`, `QTF_SERVER_PORT` (full list in the production template)
4. command line flags: `--port N` / `-p N` (port only), `--listen HOST:PORT`

The file is checked strictly: misspelled or unknown keys stop the server with an error naming the line. Check a file without starting the server:

```bash
./bin/qtestframe check-config --config examples/qtestframe.yaml
```

The most important settings:

| Key | Meaning |
|---|---|
| `server.listen` | address and port, e.g. `":8080"` or `"127.0.0.1:8443"` |
| `server.base_url` | URL users type in the browser; needed for OIDC/SAML callbacks |
| `server.tls_cert`, `server.tls_key` | enable HTTPS (reloaded on `SIGHUP`) |
| `database.driver`, `database.dsn` | `sqlite` + file path, or `postgres` + `postgres://user:pass@host:5432/db?sslmode=…` |
| `auth.cookie_secure` | must be `false` when serving plain HTTP |
| `auth.oidc`, `auth.saml` | single sign-on providers (examples in the production template and [docs/operations.md](docs/operations.md)) |
| `requirements.auto_create_test` | create a linked test with every new requirement (default `true`) |
| `runner.enabled` | allow server-side execution of automated test scripts (default `false`) |
| `logging.syslog` | ship logs to a central syslog server |
| `bootstrap.admin_email`, `bootstrap.admin_password` | first administrator, created only when the database has no users |

To use **PostgreSQL**, set:

```yaml
database:
  driver: postgres
  dsn: "postgres://qtf:qtf@localhost:5432/qtf?sslmode=disable"
```

`scripts/dev-postgres.sh` starts a throwaway PostgreSQL in Docker or Podman for trying it out. Tables are created automatically on start (or with `qtestframe migrate`).

## Using Qtestframe

The left sidebar shows **Projects**, **Search** and, inside a project, the project views. A typical flow:

### 1. Create a project

**Projects → New project.** Give it a short key (e.g. `SHOP`) that prefixes all IDs (`SHOP-REQ-1`, `SHOP-TC-1`) and a name. Leave *seed categories* on to get the default classification dimensions.

### 2. Write requirements

**Requirements** shows a folder tree on the left and the requirement table on the right.

- **Folders** are the groups. Create them with **+** in the tree, nest them by dragging, and right-click to rename, delete or move a folder to another project.
- **New requirement** opens the editor:
  - Write the statement in the description, e.g. *"The system MUST lock an account after 10 failed logins."* The language analysis detects the obligation (MUST) and flags weak wording (*fast*, *user-friendly*, *TBD*, *and/or*, …). **Auto-detect from text** stores the detected level, or pick one yourself.
  - Add **evaluation criteria**: how fulfilment is judged (method + threshold, e.g. *analysis: p95 < 2 s*).
  - **Also create linked test** (ticked by default) creates a draft test `Verify SHOP-REQ-n: …` with one step per criterion, in a test folder with the same path, and links it.
- Select rows to **bulk-edit** status, priority, obligation and categories, to move them to a folder, or to delete them. Drag rows onto a folder to move them.

### 3. Refine the tests

**Tests** has the same layout. Open a test to:

- edit the **steps** (action, expected result, test data; drag to reorder)
- switch **Execution** to *automated* and enter a **script** (sh, bash, python, javascript); the script fails when it exits non-zero
- classify it with **categories**: positive/negative, external/internal, file/network based, manual/automated, test level
- link it to further requirements, or **copy** it

### 4. Run the tests

**Operations → New run.** Name the run (e.g. *Sprint 12 regression*), set environment and build, and choose the tests: pick them individually, by folder, or with a filter expression such as `status:ready category:Negative`.

In the run, **Execute** walks through the tests one by one:

- **Manual tests** show each step. Mark it passed, failed, blocked or skipped (keyboard shortcuts P, F, B, S) and note the actual result. The overall result is derived from the steps.
- **Automated tests** have a **Run script** button that executes the script on the server (requires `runner.enabled: true`) and stores output and duration.

### 5. Check coverage

- **Traceability** shows requirements × tests. Click a cell to link or unlink. The coverage column shows *none*, *not run*, *passed*, *failed* or *partial*, based on the latest result of each linked test.
- The **Dashboard** summarises coverage, results, requirements by status and obligation, and recent runs.

### 6. Filter and search

Every table has a filter bar, and the search box at the top searches all projects. Both use the same query language:

| Query | Finds |
|---|---|
| `login` | "login" in any field |
| `status:approved priority=high` | both conditions (implicit AND) |
| `obligation:must OR obligation:shall` | either |
| `-status:obsolete` or `NOT status:obsolete` | negation |
| `title~/^(login\|logout)/i` | regular expression, case-insensitive |
| `key:*-REQ-1*` | glob |
| `version>=2 updated_at>2026-01-01` | comparisons |
| `category:Security coverage:none` | requirements in the Security area without any linked test |
| `has:criteria` | field is not empty |

The full syntax and field list is in [docs/api.md](docs/api.md#query-language).

### 7. Export

**Export** downloads the project, or chosen parts (requirements, criteria, tests, traceability, runs), as **XLSX, CSV (ZIP), PDF or DOCX**, optionally filtered with a query.

### Categories & settings

Manage category dimensions and values with their colours, rename or merge whole dimensions, and edit the project name and description.

## Users, tenants and roles

A **tenant** is an isolated organisation; projects belong to a tenant. A user can be a member of several tenants, with one role in each, and switches between them with the selector at the top. **Super administrators** can manage all tenants and users.

| Role | Can |
|---|---|
| viewer | read everything, export |
| tester | + edit tests, execute runs, create API tokens |
| manager | + edit requirements, create/edit projects and categories, run scripts, read the audit log |
| admin | + delete projects, manage members and the tenant |

In the web UI, **Administration** has Members (add people by e-mail and choose a role), API tokens, Audit log, and for super admins Tenants and Users.

The same can be done from the command line:

```bash
./bin/qtestframe tenant create --config examples/qtestframe.yaml --slug acme --name "Acme Corp"
```

```bash
echo 'Initial-pass-123' | ./bin/qtestframe user create --config examples/qtestframe.yaml --email alice@example.com --name Alice --password-stdin
```

```bash
./bin/qtestframe member add --config examples/qtestframe.yaml --tenant acme --email alice@example.com --role manager
```

For single sign-on, add `auth.oidc` (Keycloak, Entra ID, Okta, …) or `auth.saml` (ADFS, Keycloak, …) entries. Users can be provisioned automatically, with their role taken from IdP groups through `role_mapping`. See [docs/operations.md](docs/operations.md).

## Automation and CI

Create a token under **Administration → API tokens**. It is shown once and starts with `qtf_`. Use it as a Bearer token; every UI action is also available through the REST API ([docs/api.md](docs/api.md)).

Report results from a CI pipeline into an existing run; tests are matched by key:

```bash
curl -X POST "https://qtf.example.com/api/v1/runs/$RUN_ID/results" \
  -H "Authorization: Bearer $QTF_TOKEN" -H "Content-Type: application/json" \
  -d '{"results":[{"test_key":"SHOP-TC-7","status":"passed","duration_ms":1234,"output":"ok"}]}'
```

The run page has a **CI import** button that shows this command with the right run ID.

## Command line reference

| Command | Purpose |
|---|---|
| `qtestframe serve [--config F] [-p PORT] [--listen HOST:PORT]` | run the server (migrates the database first) |
| `qtestframe check-config [--config F]` | validate a configuration file |
| `qtestframe migrate [--config F]` | create/upgrade database tables |
| `qtestframe seed-demo [--config F] [--tenant SLUG]` | add a demo project |
| `qtestframe tenant create --slug S --name N` | create a tenant |
| `qtestframe user create --email E [--name N] [--password-stdin] [--super-admin]` | create a local user |
| `qtestframe member add --tenant S --email E --role R` | grant a role in a tenant |
| `qtestframe healthcheck [--config F] [-p PORT] [--url U]` | probe `/healthz` (exit code 0 = healthy) |
| `qtestframe version` | print the version |

Health endpoints for load balancers: `/healthz` (process alive) and `/readyz` (database reachable).

## Development

```bash
scripts/dev.sh --seed
```

This builds and starts the backend on :8080 (SQLite in `./data`) and the Vite dev server with hot reload on http://localhost:5173. Sign in as `admin@example.com` / `admin-dev-password`. `scripts/dev.sh --postgres` uses PostgreSQL instead. Other `make` targets: `test`, `coverage`, `vet`, `lint`, `release` (static binaries for Linux amd64/arm64 and macOS).

## Production deployment

See [docs/operations.md](docs/operations.md) for the full guide. In short:

```bash
make build && sudo scripts/install.sh
```

```bash
sudo scripts/certs/letsencrypt.sh --domain qtf.example.com
```

```bash
sudo systemctl enable --now qtestframe
```

- `install.sh` installs the binary, a hardened systemd unit, the system user and an optional SELinux module.
- Use `scripts/certs/gen-server-cert.sh` for self-signed or local-CA certificates instead of Let's Encrypt.
- Edit `/etc/qtestframe/config.yaml` (from [deploy/config.example.yaml](deploy/config.example.yaml)) before starting the service.
- A container image and a compose file with PostgreSQL are in [deploy/container](deploy/container).
- Security background is in [docs/security.md](docs/security.md).

## Tests and coverage

```bash
make test
```

```bash
make coverage
```

`make coverage` enforces the **85 % floor**: Go across all packages through `scripts/coverage.sh`, and the vitest thresholds of the web client. Set `QTF_TEST_POSTGRES_DSN` to run the store and API tests against PostgreSQL as well. See [docs/testing.md](docs/testing.md).

## Documentation

| Document | Content |
|---|---|
| [docs/architecture.md](docs/architecture.md) | components, packages, data model, deployment |
| [docs/design.md](docs/design.md) | functional design: views, requirement language, coverage, query language |
| [docs/adr/](docs/adr) | architecture decision records |
| [docs/security.md](docs/security.md) | threat model, authentication, RBAC, hardening |
| [docs/testing.md](docs/testing.md) | test strategy and coverage |
| [docs/api.md](docs/api.md) | REST API and query language reference |
| [docs/operations.md](docs/operations.md) | installation, TLS, OIDC/SAML, syslog, backup |
