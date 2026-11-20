# Qtestframe documentation

Qtestframe is a multi-tenant requirements and test management server in the
spirit of HP ALM: requirements with RFC 2119 obligation levels and
evaluation criteria, manual and automated test cases, traceability, test
runs with step-by-step execution and CI result import, dashboards, a query
language, and exports. It is a single Go binary with an embedded Vue 3 /
PrimeVue client, backed by SQLite or PostgreSQL.

## Documents

| Document | Audience | Contents |
|---|---|---|
| [architecture.md](architecture.md) | developers, reviewers | components, package map, request lifecycle and middleware chain, multi-tenancy model, data model (ER diagram), deployment topologies, build pipeline |
| [design.md](design.md) | product owners, developers, testers | functional design: views, requirement language and lint rules, categories and folders, move semantics, evaluation criteria, coverage, runs and executions, query language, search, export formats, RBAC matrix |
| [api.md](api.md) | integrators, client developers | REST API v1: conventions, errors, every endpoint, query language reference |
| [security.md](security.md) | security reviewers, operators | threat model, authentication (local, sessions, API tokens, OIDC, SAML), authorisation and tenant isolation, web protections, TLS, secrets, audit and syslog, runner sandbox, OS hardening and how to verify it |
| [operations.md](operations.md) | operators | installation (systemd, container), configuration, SQLite/PostgreSQL, TLS certificates, SSO setup for Keycloak/Entra ID/Okta/ADFS, central syslog, CLI, runner, backup/restore, upgrade, monitoring, troubleshooting, development setup |
| [user-guide/en/references.md](user-guide/en/references.md) ([sv](user-guide/sv/references.md)) | everyone | references: RFC 2119 and RFC 8174 (requirement key words), other RFCs used by Qtestframe, NIST publications, ISO/IEC/IEEE 29148, 29119, ISO/IEC Directives Part 2, INCOSE, EARS, ISTQB, OWASP ASVS, WCAG |
| [testing.md](testing.md) | developers | test strategy, API integration tests, PostgreSQL test runs, hardening tests in child processes, fake IdPs, frontend tests, the 85 % coverage floor, CI |

## Architecture decision records

Format: [ADR 0001](adr/0001-record-architecture-decisions.md) (Michael Nygard:
Status, Context, Decision, Consequences).

| ADR | Decision |
|---|---|
| [0001](adr/0001-record-architecture-decisions.md) | Record architecture decisions |
| [0002](adr/0002-single-binary-with-embedded-spa.md) | Go backend with the SPA embedded into a single binary |
| [0003](adr/0003-vue-and-primevue-client.md) | Vue 3 with PrimeVue 4 as the client framework |
| [0004](adr/0004-sqlite-and-postgresql-portable-sql.md) | SQLite and PostgreSQL through portable SQL, `?` rebinding and templated migrations |
| [0005](adr/0005-shared-schema-multi-tenancy.md) | Shared-schema multi-tenancy with `tenant_id` |
| [0006](adr/0006-server-side-sessions-and-csrf.md) | Server-side sessions with hashed tokens, CSRF token and Origin check instead of JWT |
| [0007](adr/0007-in-process-query-language.md) | A custom query language evaluated in process instead of translated to SQL |
| [0008](adr/0008-fixed-role-rbac.md) | Fixed-role RBAC per tenant with a super-admin flag (matrix superseded by 0014) |
| [0009](adr/0009-oidc-and-saml-with-jit-provisioning.md) | OIDC (PKCE + nonce) and SAML via crewjam/saml with JIT provisioning and group→role mapping |
| [0010](adr/0010-defence-in-depth-hardening.md) | Defence in depth: systemd and SELinux/AppArmor outside, no_new_privs/caps/Landlock/seccomp inside |
| [0011](adr/0011-script-runner-sandbox-via-re-exec.md) | Script runner sandbox via re-exec (`sandbox-exec`), disabled by default |
| [0012](adr/0012-syslog-shipping-rfc5424.md) | Syslog shipping with RFC 5424 over UDP, TCP and TLS |
| [0013](adr/0013-coverage-floor-85-percent.md) | A test coverage floor of 85 % |
| [0014](adr/0014-tenant-configurable-roles.md) | Tenant-configurable roles on top of built-in defaults (supersedes the fixed matrix of 0008) |

## Where to start

* Trying it out: `scripts/dev.sh --seed`, then open http://localhost:5173
  ([operations.md §15](operations.md#15-development-setup)).
* Installing a server: [operations.md §2](operations.md#2-installation),
  then the checklist in [security.md §12](security.md#12-hardening-checklist).
* Integrating CI: create an API token (`POST /api/v1/tokens`) and post results
  to `POST /api/v1/runs/{id}/results` ([api.md](api.md#operations-runs-and-executions)).
* Contributing: [architecture.md](architecture.md), [testing.md](testing.md)
  and the ADRs.

Related material outside `docs/`: `deploy/config.example.yaml` (commented
configuration), `deploy/` (systemd, SELinux, AppArmor, nftables, sysctl,
container, seccomp), `scripts/certs/README.md` (certificate tooling).
