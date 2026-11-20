# Testing

How Qtestframe is tested, how the 85 % coverage floor is enforced, and how
to run the suites on Linux and macOS.

- [1. Strategy](#1-strategy)
- [2. Go unit tests per package](#2-go-unit-tests-per-package)
- [3. API integration tests](#3-api-integration-tests)
- [4. Running against PostgreSQL](#4-running-against-postgresql)
- [5. Hardening tests in child processes](#5-hardening-tests-in-child-processes)
- [6. Fake identity providers](#6-fake-identity-providers)
- [7. Frontend tests](#7-frontend-tests)
- [8. Coverage floor](#8-coverage-floor)
- [9. Running everything on Linux and macOS](#9-running-everything-on-linux-and-macos)
- [10. Continuous integration](#10-continuous-integration)

## 1. Strategy

| Level | Tooling | Scope |
|---|---|---|
| Unit | `go test`, table-driven tests next to the code (`*_test.go`) | pure logic: query language, requirement lint, RBAC matrix, export writers, config loading, migrations rendering, logging/syslog framing, token and signer helpers |
| Store | `go test` with `internal/testutil` | SQL on SQLite and PostgreSQL: moves across projects, sessions and tokens, derived status, coverage |
| API integration | `net/http/httptest` server around `api.Server.Handler()` | every route through the full middleware chain with real sessions, CSRF, RBAC and a real database |
| CLI / process | `internal/app` tests | `serve` (HTTP and TLS with certificate reload), `migrate`, admin commands, bootstrap, `healthcheck`, hardening option mapping |
| OS hardening | child processes of the test binary | no_new_privs, capabilities, Landlock, seccomp, rlimits, `sandbox-exec` |
| Frontend | Vitest (jsdom) | `web/src/lib`, `web/src/i18n` and `web/src/themes` modules |
| Accessibility / responsive | Playwright + axe-core (`npm run a11y`) | every main route in five theme/language configurations at phone, tablet and desktop sizes against a running server |

No test needs network access: identity providers, syslog receivers and TLS
endpoints are started in process on loopback.

## 2. Go unit tests per package

Every `internal/*` package except `internal/testutil` has tests:

| Package | Focus |
|---|---|
| `internal/query` | grammar, precedence, error positions, limits, globs/regex/comparisons, multi-valued fields, `String()` round trip, `MatchedFields`, `Terms` |
| `internal/reqlang` | keyword table, strongest obligation, every lint rule, sentence splitting, offsets, score |
| `internal/rbac` | the full matrix, `Rank`, `ValidRole`, `AllPermissions` |
| `internal/export` | CSV (single and ZIP), formula-injection prefixing, XLSX sheets and names, PDF, DOCX XML validity, invalid UTF-8/XML characters |
| `internal/config` | defaults, YAML, `QTF_*` overrides, secret files and `$CREDENTIALS_DIRECTORY`, validation errors |
| `internal/db` | DSN pragmas, `Rebind`, `Render`, statement splitting, migration loading and failure rollback, unique-violation detection, a PostgreSQL round trip (skipped without DSN) |
| `internal/store` | sessions/tokens with a fake clock, moves across projects incl. re-keying and link removal, `DeriveStatus`, `Coverage` |
| `internal/auth` | password policy and bcrypt, tokens, signer tamper/expiry, OIDC and SAML providers against fake IdPs |
| `internal/logging` | levels, fan-out, RFC 5424/3164 formatting, octet counting, UDP/TCP/TLS/Unix writers with reconnect |
| `internal/runner` | pass/fail/timeout/truncation, environment, process-group kill, `sandbox-exec` argument parsing and execution |
| `internal/hardening` | deny-lists, report, child-process tests (below), stubs on non-Linux |
| `internal/model`, `internal/webui` | enumerations, embedded FS detection |
| `internal/app` | command line, bootstrap, `serve` over HTTP and TLS, SIGHUP certificate reload, healthcheck |

Tests that hash passwords set `auth.BcryptCost = bcrypt.MinCost` to keep the
suite fast. `internal/app` tests clear all `QTF_*` variables and
`CREDENTIALS_DIRECTORY` in `TestMain` so the developer's environment does
not leak in.

## 3. API integration tests

`internal/api/api_test.go` and `internal/api/sso_test.go` build a complete
server per test (`newEnv`):

* a fresh database from `testutil.OpenDB`,
* a tenant `acme` with a super-admin member,
* `config.Default()` with `cookie_secure: false`, `base_url:
  http://example.test` and the runner enabled without sandbox,
* a fake embedded UI (`fstest.MapFS`),
* `httptest.NewServer(srv.Handler())`, i.e. the real middleware chain.

Clients log in through `POST /api/v1/auth/login`, keep the cookie jar and
send the CSRF token from `Me`, exactly like the SPA. Covered areas include
security headers and the SPA fallback (`TestHealthUIAndHeaders`), the auth
flow and tenant switch (`TestAuthFlow`), the login rate limit
(`TestLoginRateLimit`, 429 after 10 attempts), tenants/users/members and the
last-admin guard, tokens and role capping (`TestTokens`), projects, folders
and categories, requirements (criteria replacement, versioning, bulk,
moves), tests, links, runs, executions, CI import, script runs, search and
export (`TestTestsLinksRunsAndExecution`), permission checks for viewer and
tester sessions (`TestRBACEnforcement`), and the Origin check (`TestOriginAndCategoryPermissions`).

## 4. Running against PostgreSQL

Tests use in-memory SQLite by default. Setting `QTF_TEST_POSTGRES_DSN` makes
`internal/testutil.OpenDB` use PostgreSQL instead:

1. connect with the DSN,
2. `CREATE SCHEMA t_<random hex>`,
3. reconnect with `search_path=<schema>` appended to the DSN and run all
   migrations,
4. on cleanup close the connection and `DROP SCHEMA … CASCADE`.

Each test therefore gets an isolated, freshly migrated schema, and tests can
run in parallel against one database. `internal/db`'s `TestPostgres` does
the same for the low-level layer.

```sh
scripts/dev-postgres.sh start                      # PostgreSQL 16 in podman/docker on 127.0.0.1:55432
export QTF_TEST_POSTGRES_DSN="$(scripts/dev-postgres.sh dsn)"
make test-go                                       # or: make coverage-go
```

The role in the DSN needs `CREATE` on the database (to create schemas).

## 5. Hardening tests in child processes

`hardening.Apply` and `runner.SandboxExec` restrict the calling process
irreversibly, so they cannot run inside the test process. The tests
re-execute the test binary:

* `internal/hardening/hardening_linux_test.go`: `runChild` starts
  `os.Args[0] -test.run=^TestHelperProcess$` with a JSON `childSpec` in
  `QTF_HARDENING_HELPER`. The helper applies the options and reports, as a
  `RESULT {…}` line, `/proc/self/status` (`NoNewPrivs`, `Seccomp`,
  `CapEff`, …), whether writes inside/outside the Landlock rule set
  succeed, and whether `unshare`, `ptrace` and `socket` are denied.
  Tests cover no_new_privs only, seccomp (with and without network deny),
  capability dropping (skipped in cgo builds), Landlock and the full
  sandbox, and rlimits.
* `internal/runner`: `TestMain` lets the test binary act as the
  `sandbox-exec` target, so `Runner.Run` with `Sandbox: true` re-executes the
  test binary just like the real server re-executes itself
  (`sandbox_linux_test.go`, `runner_test.go`).
* Kernel features that are missing (Landlock, seccomp) make the affected
  tests skip rather than fail.
* For coverage, the child receives `-test.gocoverdir` and the directory is
  added to its Landlock read-write paths, so child-process coverage counts.

On macOS these tests are excluded by build tags; `hardening_other_test.go`
checks that the stubs report every feature as skipped.

## 6. Fake identity providers

* **OIDC:** `internal/api/sso_test.go` (`newFakeIdP`) and
  `internal/auth/oidc_test.go` run an `httptest` server that serves
  discovery (`/.well-known/openid-configuration`), a JWKS with a fresh RSA
  key, and a token endpoint that requires `code=good` and a PKCE
  `code_verifier`, and returns an RS256-signed ID token with the nonce from
  the authorization request and configurable extra claims (e-mail,
  `email_verified`, groups). `TestOIDCLoginFlow` drives the browser flow
  (login redirect → callback with the state cookie → session) and
  `TestProvision` covers JIT provisioning, linking and group→role mapping.
* **SAML:** `internal/auth/saml_test.go` runs a `crewjam/saml`
  `IdentityProvider` with a self-signed certificate (`newTestIdP`);
  `TestSAMLFullFlow` performs AuthnRequest → IdP → signed response → ACS.
  `TestSAMLHandlers` in the API package covers metadata, login redirect and
  rejection of invalid responses and missing state cookies.

## 7. Frontend tests

`web/src/lib/*.test.ts` (Vitest, jsdom environment) test the non-visual
modules: API client and error handling (`api.ts`), collections, colours,
drag-and-drop payloads (`dnd.ts`), downloads, formatting, query composition
(`query.ts`), highlighting (`highlight.ts`), permissions, summaries and
folder trees (`tree.ts`). Vue components and views are not unit tested;
logic is kept out of them for this reason.

```sh
cd web
npm ci
npm test            # vitest run
npm run coverage    # with V8 coverage and thresholds
npm run typecheck   # vue-tsc --noEmit
```

Accessibility helpers (`a11y.ts`: label association, typing detection,
focus after route changes, stacked-table labels), the `useBreakpoint`
composable (`breakpoint.ts`), the execution runner shortcuts
(`shortcuts.ts`) and the per-theme chart colours (`src/themes/chart.ts`)
have their own tests.

### Accessibility and responsive checks

`web/scripts/a11y-check.mjs` (`npm run a11y`) drives a headless Chrome
with Playwright and runs [axe-core](https://github.com/dequelabs/axe-core)
against a running server with the demo data. It signs in as the bootstrap
administrator and visits the login page, projects, dashboard, requirements
(list and with the editor open), tests, runs, a run, the execution runner,
traceability, baselines, settings, export, documents, reports, search,
profile and the administration pages (members, tokens, audit, role matrix,
organisations, users, backup):

* in five configurations: light theme in English, dark theme in Swedish and
  the decorative themes navy, crazy and green_console (the theme and
  language are set as the admin's preferences and restored afterwards);
* at the viewports 390×844 (phone), 820×1180 (tablet), 1024×768 (tablet
  landscape) and 1440×900 (desktop), with touch emulation on the tablets
  and the phone;
* including open states: the editor drawer, the attachments tabs of a
  requirement and a run, the "new run" dialog and the off-canvas navigation
  (phone and tablet);
* with the axe tags `wcag2a`, `wcag2aa`, `wcag21a`, `wcag21aa`, and
  asserting `document.documentElement.scrollWidth <= innerWidth` (no
  horizontal page scroll).

It then checks keyboard and focus behaviour in the light theme: the skip
link is the first focusable element and moves focus to the page heading, a
route change focuses the new `h1` and announces the page title, the editor
drawer and the off-canvas navigation keep focus inside and give it back on
close (Escape), and the execution runner's P/F/B/S shortcuts do not fire
while typing.

It exits non-zero on any serious or critical violation, any overflow or a
failed keyboard check, and prints a summary; minor and moderate findings
are listed but do not fail. No axe rule is disabled.

```sh
# terminal 1: a scratch directory with the demo data
make build                    # or: (cd web && npm ci && npm run build) && CGO_ENABLED=0 go build -o bin/qtestframe ./cmd/qtestframe
mkdir -p /tmp/a11y && cp -r examples bin /tmp/a11y && cd /tmp/a11y
./bin/qtestframe seed-demo --config examples/qtestframe.yaml
./bin/qtestframe serve --config examples/qtestframe.yaml

# terminal 2
cd web
npm run a11y
```

| Variable | Default | Meaning |
|---|---|---|
| `QTF_URL` | `http://127.0.0.1:8080` | server to test |
| `QTF_EMAIL`, `QTF_PASSWORD` | `admin@example.com`, `Change-me-1234` | sign-in |
| `QTF_CHROME` | Playwright's Chromium, else a system Chrome | browser executable |
| `QTF_A11Y_ONLY` | all | comma-separated route names, e.g. `login,dashboard` (skips the keyboard checks) |
| `QTF_A11Y_CONFIGS` | all | e.g. `light/en,navy/en` |
| `QTF_A11Y_VIEWPORTS` | all | e.g. `phone,desktop` |
| `QTF_A11Y_KEYBOARD` | – | `0` skips the keyboard checks |
| `QTF_A11Y_SHOTS` | – | directory for a screenshot of every page, viewport and configuration |

The script needs a Chrome or Chromium (`npx playwright install chromium`
or a system browser). Each run signs in once; the login rate limit (10
attempts) is not an issue for repeated runs.

## 8. Coverage floor

The floor is 85 % for both code bases ([ADR 0013](adr/0013-coverage-floor-85-percent.md)).

**Go** — `make coverage` → `make coverage-go` → `scripts/coverage.sh`:

1. `PKGS` = all `./internal/...` packages except `internal/testutil`
   (`cmd/` is a three-line `main`);
2. `go test -count=1 -coverpkg="$PKGS" -coverprofile=coverage.out <packages that have tests>`
   with `CGO_ENABLED=0`, so code is credited no matter which package's
   tests exercise it (the API tests cover much of `store`, `auth`, `query`);
3. `go tool cover -func` total is compared with the floor
   (`COVERAGE_FLOOR`, default 85, or the first argument); below → exit 1.

`go tool cover -html=coverage.out` shows the details.

**Web** — `make coverage-web` → `npm run coverage`: Vitest's V8 provider
over `src/lib/**` (excluding tests and `types.ts`) with thresholds of 85 for
lines, functions, branches and statements (`web/vitest.config.ts`); any
threshold miss fails the command.

## 9. Running everything on Linux and macOS

Prerequisites: Go (version from `go.mod`), Node 22 with npm, GNU make
(the macOS system make 3.81 works), optionally podman or docker for
PostgreSQL.

```sh
make test            # Go tests (CGO_ENABLED=0) + web tests
make coverage        # both coverage floors
make lint            # go vet (linux and darwin) + gofmt check + vue-tsc
make build           # web build + static binary
```

Platform notes:

| | Linux | macOS |
|---|---|---|
| Hardening and sandbox tests | run; features missing in the kernel are skipped | not built; stub tests run instead |
| Capability tests | as root they also verify the bounding set and that `CAP_NET_BIND_SERVICE` is kept; as a normal user they verify that no other capability remains | n/a |
| Runner tests | need `/bin/sh` (`bash` for one test) | same, sandboxed variants skipped |
| Unix-socket syslog tests | run | run (skipped if the socket type is unavailable) |
| PostgreSQL | `scripts/dev-postgres.sh` with podman or docker | same |

Run tests with `CGO_ENABLED=0` (as the Makefile does): with cgo,
`AllThreadsSyscall` is unavailable and the capability tests skip.
`make vet` also vets with `GOOS=darwin`, so Linux-only code must stay
behind build tags.

## 10. Continuous integration

`.github/workflows/ci.yml` runs on every push and pull request:

| Job | Steps |
|---|---|
| `go` | PostgreSQL 16 service container, `QTF_TEST_POSTGRES_DSN` set; `make vet` (linux + darwin), gofmt check, `make coverage-go` (85 % floor, on PostgreSQL), upload `coverage.out` |
| `web` | Node 22, `npm ci`, `npm run build` (type check + build), `npm run coverage` (85 % thresholds) |
| `build` | after `go` and `web`, on `ubuntu-latest` and `macos-latest`: `make build`, `./bin/qtestframe version` |

Because `QTF_TEST_POSTGRES_DSN` is set in CI, the store and API suites run
against PostgreSQL there; locally they run against SQLite unless the
variable is set, so both engines are exercised.
