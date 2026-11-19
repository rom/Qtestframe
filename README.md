# Qtestframe

Qtestframe is a multi-user, multi-tenant tool for managing requirement specifications and tests, similar in scope to HP/Micro Focus ALM. It is a single Go binary with an embedded Vue 3 + PrimeVue web client. Data lives in **SQLite** or **PostgreSQL**.

## Features

- **Requirements:**
  - Obligation levels based on RFC 2119/8174: `MUST`, `MUST NOT`, `SHALL`, `SHALL NOT`, `SHOULD`, `SHOULD NOT`, `MAY`, `OPTIONAL`, `WILL` and `DESCRIBE`. The level is detected automatically from the text.
  - Live language checks while you write: ambiguous terms, TBDs, compound requirements, escape clauses, and/or, and more.
  - Each requirement has status, type, priority, rationale, source, owner, tags, version and evaluation criteria (test, inspection, analysis, demonstration, review).
- **Test cases:**
  - Manual tests are executed step by step (action, expected result, data).
  - Automated tests can be shell, bash, Python or JavaScript scripts, run server-side in a sandbox. CI systems can also import results.
- **Classification:** each project defines its own category dimensions. The defaults are Polarity (positive/negative), Interface (external/internal), Medium (file/network based), Execution (manual/automated), Level (unit to acceptance) and Area.
- **Grouping:** requirements and tests are organised in nested folders. Drag and drop moves single items, whole folders, or entire category dimensions. Items can also be moved to another project.
- **Views:**
  - requirements
  - tests
  - operations: test runs and an execution runner
  - traceability matrix with coverage
  - dashboard
  - global search
  - administration
- **Filtering and search on every field:** a query language with field operators, regular expressions (`title~/^login/i`), globs, comparisons, `AND`/`OR`/`NOT` and grouping. See [docs/api.md](docs/api.md#query-language).
- **Export:** a whole project or selected parts (requirements, criteria, tests, traceability, runs) to **XLSX, CSV, PDF and DOCX**.
- **Security:**
  - local accounts, **OIDC** (PKCE + nonce) and **SAML 2.0** with just-in-time provisioning and group→role mapping
  - **RBAC** with the roles admin, manager, tester and viewer, plus a super-admin flag
  - API tokens for CI
  - an audit log
- **Operations:**
  - structured logs, optionally shipped to a central **syslog** server (RFC 5424/3164 over UDP, TCP, TLS or a Unix socket)
  - X.509 scripts for a local CA, self-signed certificates and Let's Encrypt
  - hardening with systemd, SELinux, AppArmor, seccomp, Landlock, capabilities and no_new_privs

## Quick start (Linux or macOS)

Requirements: Go ≥ 1.24, Node ≥ 20, make.

```bash
make build
```

```bash
QTF_BOOTSTRAP_ADMIN_EMAIL=admin@example.com QTF_BOOTSTRAP_ADMIN_PASSWORD='change-me-now-1' QTF_COOKIE_SECURE=false ./bin/qtestframe serve
```

Then open http://localhost:8080. The default configuration uses `./qtestframe.db` (SQLite) on `:8080`. To load sample data:

```bash
./bin/qtestframe seed-demo
```

For development with hot reload, run the following. It starts the backend on :8080 and Vite on :5173.

```bash
scripts/dev.sh --seed
```

To use PostgreSQL, start a throwaway instance with `scripts/dev-postgres.sh`, or set:

```bash
export QTF_DATABASE_DRIVER=postgres
export QTF_DATABASE_DSN='postgres://qtf:qtf@localhost:5432/qtf?sslmode=disable'
```

## Production

See [docs/operations.md](docs/operations.md). In short:

```bash
make build
```

```bash
sudo scripts/install.sh
```

```bash
sudo scripts/certs/letsencrypt.sh --domain qtf.example.com
```

`install.sh` installs the binary, the hardened systemd unit and an optional SELinux module. The last command issues a Let's Encrypt certificate; `scripts/certs/gen-server-cert.sh` creates self-signed or local-CA certificates instead. Edit `/etc/qtestframe/config.yaml` (template: [deploy/config.example.yaml](deploy/config.example.yaml)) and run `systemctl enable --now qtestframe`. There is a container image and a compose file in [deploy/container](deploy/container).

## Tests and coverage

```bash
make test
```

```bash
make coverage
```

`make test` runs the Go and web unit tests. `make coverage` enforces the **85 % floor**: Go across all packages through `scripts/coverage.sh`, and vitest thresholds for the web client. Set `QTF_TEST_POSTGRES_DSN` to run the store and API tests against PostgreSQL as well. See [docs/testing.md](docs/testing.md).

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
