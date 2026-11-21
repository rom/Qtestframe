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
  - Status, type, priority, rationale, source (a per-project list of sources with free text), owner, tags (each tag in a colour of its own), version and **evaluation criteria** (test, inspection, analysis, demonstration, review).
  - **Import** from the files customers send: CSV, Excel, ReqIF (`.reqif`, `.reqifz`) and Word, with English and Swedish column names, a preview with an editable column mapping, folders from sections and headings, obligation detection and duplicate checks.
- **Test cases**
  - Manual tests are executed step by step (action, expected result, data).
  - Automated tests are shell, bash, Python or JavaScript scripts, run server-side in a sandbox. CI systems can also import results.
- **Catalogue of standard requirements and tests:** a built-in, searchable library for software development (security, privacy, performance, reliability, operations, usability, accessibility, common functionality such as workflow, collaboration, files and scheduling, and domain-specific areas such as checkout and billing) in English and Swedish, written for Qtestframe and citing the standards it is based on (OWASP ASVS, WCAG, ISO/IEC 25010, NIST SP 800-63B …) without copying them. Browse by group and area, filter, preview and add a selection to a project with its verifying tests and trace links in one step, into folders per area, with undo; added items remember their catalogue origin ([guide](docs/user-guide/en/catalogue.md)).
- **Undo and redo:** an undo stack in the top bar (<kbd>Ctrl</kbd>+<kbd>Z</kbd>, <kbd>Ctrl</kbd>+<kbd>Shift</kbd>+<kbd>Z</kbd>) for edits, deletions, copies, moves, bulk edits, links, relations, dependencies, review decisions, results and folders; deleted requirements and tests come back with their key, links, comments, history, attachments and results (a 30-day trash in the API).
- **Reuse and review:** copy a requirement as the basis of a new one (optionally related as *derived from*); a **Reviewer** role and the permissions `requirement.review` and `test.review` for approving requirements and marking tests ready; copies start as drafts.
- **Change history:** every change of a requirement or test is versioned, with a field-level diff and one-click restore. **Baselines** freeze a project's state for a release and compare it with the current state or another baseline.
- **Relations** between requirements (refines/decomposition, depends on, provides, derived from, conflicts with, duplicates, relates to) and **comment threads** on requirements and tests. A **Relations** column in the requirement and test lists shows every relation in both directions (*depends on*, *provided by*, *verified by*, *calls* …), next to many other optional columns (owner, source, tags, criteria, steps, defects, created by …).
- **@mentions:** type `@` in owner, assignee, descriptions, notes and comments to pick a user of the organisation or a person already named in the project; mentions are shown with name and avatar, and `mention:anna` finds them.
- **Tag suggestions** from the title and description, preferring tags the project already uses.
- **Test reuse and results:** shared steps (a step can call another test), **test dependencies** (a test may only start after its prerequisites have passed), execution history across runs, **evidence** per executed step (text and files such as screenshots), and defects linked to failed executions (with links into your issue tracker).
- **Execution readiness:** by default only Ready tests of approved requirements whose prerequisites have passed can be executed; admins and test leads decide which rules apply. The runner, run page and new-run dialog explain exactly why a test cannot run yet (test status, requirement status against the required statuses, or a prerequisite), what to do about it and which setting decides.
- **Project types:** general, software, hardware, system or business. **Software projects** get **AI prompts**: ready-to-paste prompts built from the requirements and tests (implement requirements, review them, design test cases of a test type, automate test cases in your framework, review tests, plan a test type), with the project's technical context; nothing is sent to an AI service.
- **Links** from requirements, tests and runs to other requirements, tests, runs, documents and saved reports of the archive, report definitions and web addresses, shown in both directions; item keys and web addresses in comments become links, and `/go/KEY` opens any item.
- **Requirement analytics:** the spread of obligation levels, inconsistent keywords (MUST in 95 % of the requirements but SHALL in a few), requirements without an obligation, tests or acceptance criteria, failing coverage, tests that verify nothing, duplicates, conflicts and the language findings per rule, with advice and the requirements concerned. While writing a requirement, a help panel explains the obligation levels and keywords in the interface language.
- **Test flow:** a flowchart of a test's steps (checks of the expected results, shared steps expandable in place, automated tests as script → exit code) and the run order of a test run in stages derived from the test dependencies; both with a text view.
- **Dependency graphs and tag cloud:** layered graphs of requirement relations and of test dependencies and shared steps, with filters and a list view; a tag cloud that lists the requirements, tests and documents carrying the selected tags.
- **Suspect links and evidence:** when a requirement or test changes, its links become *suspect* until someone confirms them; a requirement's status follows the evidence (implemented → verified when all its tests pass, back when one fails or a link is suspect).
- **Releases with exit criteria:** a release has a scope of requirements, the runs that test it and exit criteria (coverage, passed, mandatory requirements passed, no failures, no suspect links, no open defects of a severity, risks mitigated …) that give a go / no-go; releasing creates a baseline, and releasing despite a no-go needs a recorded reason.
- **Defects** as items with status, severity, assignee and links to requirements, tests and executions (report one straight from a failed execution), with **Jira and GitHub** push, status sync and import.
- **Risks:** a risk register with likelihood × impact, a heat map, mitigation, status and the requirements that address each risk, with risk coverage and a release criterion.
- **Reviews and sign-off:** ask named people to review a requirement or test with a checklist, record their decision, and sign baselines electronically (password re-check, content fingerprint, audit).
- **Notifications and webhooks:** in-app notifications for assignments, @mentions, review requests and defect changes, optionally by **e-mail** (immediately or a daily digest), e-mail invitations for new members and **scheduled report e-mails**; signed (HMAC-SHA256) webhooks per project for requirement, test, run, defect, risk, release, review and sign-off events.
- **Automated results:** import JSON or **JUnit XML** results into a run, the `qtestframe report` command-line uploader for CI, and a **Playwright reporter** package ([integrations/playwright-reporter](integrations/playwright-reporter/README.md)).
- **Test configurations and data-driven tests:** run the same tests against several environments (browser, OS, hardware …) with one execution per test and configuration and coverage per configuration; parameterised tests with a data table run once per row, with `{{parameter}}` placeholders in their steps.
- **Traceability rules:** project rules such as "every MUST requirement is verified by a test" or "every test verifies a requirement", with violations on the traceability page and in the editors, and a release exit criterion.
- **Dashboard trends and quality:** coverage and pass rate over time, defects opened vs closed, a release burn-up, progress of active runs, coverage by folder or category, and **flaky test** detection (pass ↔ fail flips) with a `flaky:` filter.
- **Safe concurrent editing:** saving a requirement or test that someone else saved in the meantime shows both versions field by field instead of overwriting them (ETag / If-Match in the API).
- **Requirement ↔ test correlation:** every new requirement gets a linked draft test case by default, with one step per evaluation criterion, placed in a test folder that mirrors the requirement's folder.
- **Attachments and documents:** attach files to requirements, tests, runs and executions; every project has a document archive with previews, tags and filters.
- **Reports:** six standard reports (project status, coverage, traceability, run summary, defects, test inventory) and a custom report builder; PDF, Word, Excel or CSV, downloaded or saved in the archive.
- **Personal touches:** seventeen colour themes (including ocean, forest, sunset, nordic, lavender and high contrast), avatars for users, icons and colours for projects (presets or uploaded images), a "last signed in" notice with failed attempts, and a first-login intro tour.
- **Built-in user guide** in English and Swedish with a tutorial, FAQ and a [references](docs/user-guide/en/references.md) page (RFC 2119, RFC 8174, NIST, ISO/IEC/IEEE 29148 and 29119 and more), available under Help in the tool ([docs/user-guide](docs/user-guide/en/index.md)).
- **Context-sensitive help:** an *i* next to form fields explains the field on mouse-over or keyboard focus and links to the matching section of the guide; <kbd>?</kbd> opens the guide page for the current view.
- **Classification:** each project defines its own category dimensions. The defaults are Polarity (positive/negative), Interface (external/internal), Medium (file/network based), Execution (manual/automated), Level (unit to acceptance) and Area.
- **Grouping:** nested folders for requirements and tests. Drag and drop moves items, whole folders, or entire category dimensions, also to another project.
- **Views:** requirements, tests, operations (test runs and an execution runner), traceability matrix with coverage, dependency graph, tag cloud, dashboard, global search, administration. Table columns can be chosen and reordered (drag the header or use the Columns menu).
- **Filtering and search on every field** with a [query language](#query-language): field operators, regular expressions, globs, comparisons, `AND`/`OR`/`NOT` and grouping.
- **Export** of a whole project or selected parts (requirements, criteria, tests, traceability, runs) to **XLSX, CSV, PDF and DOCX**, and of requirements to **ReqIF 1.2** (`.reqif`, or `.reqifz` with attachments) for DOORS, Polarion, ProR and other requirement tools, with folders as headings and relations; an export imports back with all fields.
- **Security:** local accounts, **OIDC** and **SAML 2.0** single sign-on, **SCIM 2.0** user provisioning from Entra ID, Okta and other identity providers (group → role mapping, deactivation ends access), **RBAC** (admin, manager, test lead, tester, reviewer, viewer + super admin, editable per organisation, plus custom roles), API tokens, an audit trail with date filters and CSV export, a role matrix, admin password reset with a forced change, backup and restore (also across SQLite/PostgreSQL), in-process sandboxing (Landlock, seccomp, capabilities, no_new_privs).
- **Operations:** structured logs with optional **syslog** shipping (UDP, TCP, TLS, Unix socket), certificate scripts (local CA, self-signed, Let's Encrypt), hardened systemd unit, SELinux and AppArmor profiles, container image.

## Quick start

Works on Linux and macOS. You need Go 1.26 (the `toolchain` line in `go.mod` pins 1.26.11; `go` downloads it automatically), Node.js ≥ 20 and `make`.

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
| `runner.web` | web tests in a real browser with Playwright: `enabled`, `node`, `playwright_dir`, `browsers_path`, `browser`, `headless`, `timeout`, `screenshots`, `trace`, `video`, `max_parallel` (see [docs/operations.md](docs/operations.md#web-tests-playwright)) |
| `logging.syslog` | ship logs to a central syslog server |
| `bootstrap.admin_email`, `bootstrap.admin_password` | first administrator, created only when the database has no users |
| `email.host`, `email.port`, `email.tls`, `email.from` | outgoing e-mail (SMTP) for notification e-mails, invitations and reports by e-mail; off while `host` is empty |

**E-mail** is optional. With it, users can get their notifications by e-mail (each one immediately or a daily digest, chosen in the profile), administrators can invite new members by e-mail instead of setting an initial password, and reports can be sent by e-mail or on a schedule. Without it these features are hidden in the web client and the API answers that e-mail is not configured. Links in e-mails use `server.base_url`, which is then required. The password is never written in the file: use `password_file` (relative paths are resolved against `$CREDENTIALS_DIRECTORY`) or the `QTF_EMAIL_PASSWORD` environment variable.

```yaml
email:
  host: smtp.example.com
  port: 587                  # 587 = STARTTLS, 465 = implicit TLS
  tls: starttls              # starttls (default) | tls | none (local relay only)
  username: qtestframe       # leave out for a relay without authentication
  password_file: /run/secrets/smtp-password
  from: qtestframe@example.com
  from_name: Qtestframe
  timeout: 30s               # per message
  digest_hour: 7             # hour (UTC) of the daily notification digest
  # enabled: false           # keep the settings but send nothing
```

Administrators check the settings with **Administration → Members → Send test e-mail**, which shows the answer of the mail server when sending fails. Mail is sent in the background (never for a request that fails), failures are logged.

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
- **Web tests** (script language *Web test (Playwright)*) run in a real browser on the server as background jobs; screenshots, the Playwright trace and videos are attached to the execution. **Run automated tests** on the run page queues all automated tests of a run (prerequisites first). Install Playwright next to the server (`npm i @playwright/test`, `npx playwright install --with-deps chromium`) and enable `runner.web` — see [docs/operations.md](docs/operations.md#web-tests-playwright).

### 5. Check coverage

- **Traceability** shows requirements × tests. Click a cell to link or unlink. The coverage column shows *none*, *not run*, *passed*, *failed* or *partial*, based on the latest result of each linked test.
- The **Dashboard** summarises coverage, results, requirements by status and obligation, and recent runs.

### 6. Filter and search

Every table has a filter field, and the search box in the top bar searches all projects of the organisation. Both use the same [query language](#query-language), described below. The **?** button next to every filter field lists the operators and fields of that list, and **Help → Query language** opens the full description in the built-in user guide.

```text
status:approved priority=high                  approved AND high priority
coverage:none category:Security                security requirements without any test
type=automated last_result:failed              failed automated tests
```

### 7. Export

**Export** downloads the project, or chosen parts (requirements, criteria, tests, traceability, runs), as **XLSX, CSV (ZIP), PDF or DOCX**, optionally filtered with a query.

### Collaborate and track changes

- The requirement and test editors have tabs for **Comments**, **History** (field-level diff of every version, **Restore this version**) and, for requirements, **Relations** (e.g. *refines* to build a requirement hierarchy).
- **Baselines** (project tab) freezes the current requirements and tests, e.g. *Release 1.0*. **Compare** shows what was added, removed or changed since then.
- In a test's steps, **Insert call to test…** reuses another test's steps (e.g. a common login sequence). The test's **Executions** tab shows its results across all runs.
- While executing, record **defects** (e.g. `JIRA-123`). Set the *Defect tracker URL template* under **Categories & settings** to turn them into links.

### Your profile

The user menu → **Profile & password** has your display name and preferences: theme, start page, default project and tenant, rows per page, date format and time zone, compact tables, and whether new requirements get a linked test by default. It also lists your active sessions (browser, IP, last activity), each of which can be signed out.

### Categories & settings

Manage category dimensions and values with their colours, rename or merge whole dimensions, and edit the project name and description.

## Query language

One small language filters every list in Qtestframe: requirements, tests, runs, executions, the traceability matrix, the audit log and the document archive. It is also used by the global search, by baselines, by exports and reports, and to select the tests of a new run. In the API it is the `q` parameter. Queries are evaluated by the server, so the same query gives the same result in the UI, in exports and in scripts.

The same description, with more examples, is in the user guide: [Search and filters](docs/user-guide/en/search-and-filters.md) ([på svenska](docs/user-guide/sv/search-and-filters.md)). It is also available in the tool under **Help → Query language**.

### Terms

| You write | It finds |
|---|---|
| `login` | *login* anywhere in any field (case-insensitive) |
| `"session timeout"` | the exact phrase anywhere |
| `status:approved` | items whose status **contains** *approved* |
| `priority=high` | priority **equals** *high* (case-insensitive) |
| `priority!=low` | priority is not *low* |
| `title~/^login.*(fail\|error)/i` | regular expression (RE2), `/i` ignores case |
| `description!~/TBD/` | the regular expression does not match |
| `key:*-REQ-1?` | glob: `*` any characters, `?` one character, matched against the whole value |
| `version>=2`, `estimate<30` | numeric comparison |
| `updated_at>=2026-01-01` | date comparison (timestamps are RFC 3339, UTC) |
| `has:criteria` or `criteria:*` | the field is not empty |
| `category=""` | the field is empty |
| `/timeout/i` | regular expression over all fields |

### Combining terms

| You write | Meaning |
|---|---|
| `status:approved priority=high` | terms next to each other are combined with **AND** |
| `a AND b`, `a && b` | both |
| `a OR b`, `a \|\| b` | either |
| `NOT a`, `!a`, `-a` | negation; `-` must be directly followed by the term |
| `(tag:smoke OR tag:regression) -status:deprecated` | parentheses group terms |

`NOT` binds tighter than `AND`, and `AND` tighter than `OR`. Keywords and field names are case-insensitive. Values with spaces need quotes (`category:"Network based"`); write `\"` for a quote inside quotes. An empty query matches everything.

Fields with several values (tags, categories, criteria, steps, linked items, defects) match a positive condition if **any** value matches, and a negative one (`!=`, `!~`) only if **no** value matches.

### Fields

| List | Fields (aliases in parentheses) |
|---|---|
| Requirements | `id`, `key`, `title`, `description` (`desc`), `obligation`, `type`, `priority`, `status`, `source`, `rationale`, `owner`, `tag` (`tags`), `version`, `folder`, `category` (`categories`), `dimension`, `coverage`, `linked`, `related` (`relation`), `parent` (`parents`), `child` (`children`), `comments`, `mention` (`mentions`), `criteria` (`criterion`), `created_by`, `created_at`, `updated_at` |
| Tests | `id`, `key`, `title`, `description` (`desc`), `preconditions`, `execution_type` (`type`), `script_language` (`language`), `script`, `expected`, `step` (`steps`), `priority`, `status`, `estimate`, `tag` (`tags`), `folder`, `category` (`categories`), `dimension`, `linked`, `last_result` (`result`), `defect` (`defects`), `calls`, `depends_on` (`depends`), `comments`, `mention` (`mentions`), `version`, `created_by`, `created_at`, `updated_at` |
| Runs | `id`, `name`, `description`, `status`, `environment` (`env`), `build`, `created_by`, `created_at` |
| Executions (run page) | `id`, `key`, `title`, `status`, `assignee`, `executed_by`, `notes`, `output`, `defect` (`defects`), `mention` (`mentions`) |
| Audit log | `action`, `entity`, `entity_id`, `user`, `details`, `created_at` |
| Document archive | `id`, `name`, `description`, `tag` (`tags`), `kind`, `type` (`content_type`), `uploaded_by` (`by`), `created_at`, `size`, `linked` |
| Global search | the requirement, test and run fields, plus `project` (key or name) and `kind` (`requirement`, `test`, `run`) |

What the fields hold: `folder` is the folder path (`Checkout / Payment`); `category` holds category names and `dimension` holds `Dimension/Name`; `linked`, `related`, `parent`, `child` and `calls` hold item keys; `criteria` holds description, method and threshold of every criterion; `step` holds action, expected result and data of every step; `mention` holds the @handles mentioned in the item (owner and text fields), so `mention:anna` and `mention:@anna` both work; `coverage` is `none`, `not_run`, `passed`, `failed` or `partial`; `comments`, `version`, `estimate` (minutes) and `size` (bytes) are numbers. A field that does not exist in a list is an error, so `coverage:none` works for requirements but not for tests.

### Examples

```text
obligation=must status!=approved               MUST requirements not yet approved
coverage:none category:Security                security requirements without any test
coverage:failed priority=critical              critical requirements whose tests fail
-has:criteria status:approved                  approved, but no evaluation criteria
description~/\b(fast|easy|TBD)\b/i             weak wording
folder:"Checkout / Payment"                    a folder and its subfolders
child:*                                        requirements that have been decomposed
linked:SHOP-TC-3                               requirements verified by SHOP-TC-3
version>=3 updated_at>=2026-06-01              changed often and recently
type=automated last_result:failed              failed automated tests
-linked:*                                      tests not linked to any requirement
defect:JIRA-*                                  tests with a JIRA defect
status:in_progress env:staging                 runs in progress on staging
assignee:anna -status:passed                   (run page) Anna's executions that have not passed
action:member.* created_at>=2026-01-01         (audit log) membership changes this year
kind=requirement project:SHOP coverage:none    (search) uncovered requirements of SHOP
mention:anna -status:approved                  items that mention @anna and are not approved
```

### Errors and limits

An invalid query is rejected with a message that points at the position of the problem (`400 invalid_query` with `position` in the API). The usual causes are an unknown field, an unclosed quote, parenthesis or regex, and a space after the operator (`status: approved`). Regular expressions use [RE2 syntax](https://github.com/google/re2/wiki/Syntax), so look-ahead and back-references are not supported. A query may be at most 4096 bytes long, with at most 64 regular expressions or globs and 128 levels of nesting. The grammar is in [docs/design.md §9](docs/design.md#9-query-language).

## Users, tenants and roles

A **tenant** is an isolated organisation; projects belong to a tenant. A user can be a member of several tenants, with one role in each, and switches between them with the selector at the top. **Super administrators** can manage all tenants and users.

| Role | Can (default) |
|---|---|
| viewer | read everything, export |
| reviewer | viewer + approve/reject requirements and mark tests ready (`requirement.review`, `test.review`), without editing them |
| tester | viewer + edit tests, execute runs, create API tokens |
| test lead | tester + mark tests ready, run scripts, read the audit log, change the test execution settings |
| manager | + edit and approve requirements, create/edit projects and categories |
| admin | + delete projects, manage members, roles and the tenant |

These are defaults. Under **Administration → Role matrix**, admins (or any role with `role.manage`) tick and untick the permissions of every role except admin, reset built-in roles to their defaults, and create their own roles, for example an *Auditor* who only reads the audit log. Nobody can grant a permission they do not hold, and admin always keeps every permission ([ADR 0014](docs/adr/0014-tenant-configurable-roles.md)).

In the web UI, **Administration** has Members (add people by e-mail and choose a role), API tokens, Audit log, Role matrix, Test execution settings, and for super admins Tenants, Users and Backup.

### Test execution settings

By default a test can only be executed when its status is **Ready**, the requirements it verifies are **Approved**, **Implemented** or **Verified**, and the tests it depends on have passed. Admins and test leads can relax each rule under **Administration → Test execution**. Forbidden results are refused by the server (`409 not_ready`, also for API clients; CI imports report them under `rejected`); the UI shows the reasons in red in the runner, in a Readiness column on the run page and as warnings in the New run dialog. Allowed exceptions are shown as warnings.

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

For single sign-on, add `auth.oidc` (Keycloak, Entra ID, Okta, …) or `auth.saml` (ADFS, Keycloak, …) entries. Users can be provisioned automatically, with their role taken from IdP groups through `role_mapping`. See [docs/operations.md](docs/operations.md). Identity providers can also provision users ahead of their first sign-in with SCIM 2.0 at `/scim/v2` (Administration → Provisioning (SCIM); see [docs/api.md](docs/api.md#scim-20-provisioning)).

## Automation and CI

Create a token under **Administration → API tokens**. It is shown once and starts with `qtf_`. Use it as a Bearer token; every UI action is also available through the REST API ([docs/api.md](docs/api.md)). The server describes the API as **OpenAPI 3.1** at `/api/v1/openapi.json` (no sign-in needed; source [docs/openapi.yaml](docs/openapi.yaml)): open it in any OpenAPI viewer such as Swagger UI or Redoc, or generate a client from it.

Report results from a CI pipeline into an existing run; tests are matched by key:

```bash
curl -X POST "https://qtf.example.com/api/v1/runs/$RUN_ID/results" \
  -H "Authorization: Bearer $QTF_TOKEN" -H "Content-Type: application/json" \
  -d '{"results":[{"test_key":"SHOP-TC-7","status":"passed","duration_ms":1234,"output":"ok"}]}'
```

The run page has a **CI import** button that shows this command with the right run ID. Results of tests that may not be executed yet (see [Test execution settings](#test-execution-settings)) are not recorded; the response lists them under `rejected` with the reasons.

### Reporting automated test results

Tests are matched by a test key such as `SHOP-TC-7` in the test name (`test_login[SHOP-TC-7]`, `SHOP_TC_7` where hyphens are not allowed), the class name, a JUnit property `qtestframe.key`/`test_key`, or a Playwright tag/annotation.

- **JUnit XML** – any tool that writes JUnit XML (Playwright, pytest, Go with go-junit-report, Maven Surefire, Gradle, Jest, Vitest …):

  ```bash
  curl -X POST "https://qtf.example.com/api/v1/runs/$RUN_ID/junit?create_missing=true" \
    -H "Authorization: Bearer $QTF_TOKEN" -H "Content-Type: application/xml" --data-binary @junit.xml
  ```

  Failures and errors become `failed`, `<skipped>` `skipped`, the rest `passed`; several test cases with one key combine (failed wins, durations add up). `create_missing=true` creates draft automated tests for test cases without a key; otherwise they are listed under `unmapped`.
- **Command line** – `qtestframe report` finds or creates the run and uploads JUnit XML or JSON files:

  ```bash
  QTF_URL=https://qtf.example.com QTF_TOKEN=qtf_… \
    qtestframe report --project SHOP --run "Nightly $BUILD" --create-run --build "$BUILD" --junit 'reports/*.xml'
  ```

  It prints updated, unknown, unmapped and rejected results and exits non-zero on errors (and with `--fail-on-unknown` when results matched no test).
- **Playwright reporter** – [`integrations/playwright-reporter`](integrations/playwright-reporter/README.md) (`@qtestframe/playwright-reporter`) reports while Playwright runs, includes `test.step()` results in the output and uploads screenshots and traces of failed tests as execution attachments.

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
| `qtestframe report --url U --token T (--project KEY --run NAME [--create-run] \| --run-id ID) --junit F… \| --json F…` | upload automated test results (`QTF_URL`, `QTF_TOKEN`); see [Reporting automated test results](#reporting-automated-test-results) |
| `qtestframe version` | print the version |

Health endpoints for load balancers: `/healthz` (process alive) and `/readyz` (database reachable).

## Development

```bash
scripts/dev.sh --seed
```

This builds and starts the backend on :8080 (SQLite in `./data`) and the Vite dev server with hot reload on http://localhost:5173. Sign in as `admin@example.com` / `admin-dev-password`. `scripts/dev.sh --postgres` uses PostgreSQL instead. Other `make` targets: `test`, `coverage`, `vet`, `lint`, `release` (static binaries for Linux amd64/arm64 and macOS), and `guide` / `guide-md` (the user guide as one PDF and one Markdown file per language in `dist/user-guide/`, see [docs/user-guide/README.md](docs/user-guide/README.md#pdf-and-single-file-markdown)).

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
| [docs/openapi.yaml](docs/openapi.yaml) | OpenAPI 3.1 description of the REST API (served at `/api/v1/openapi.json`) |
| [docs/operations.md](docs/operations.md) | installation, TLS, OIDC/SAML, syslog, backup |
| [docs/user-guide](docs/user-guide/en/index.md) | user guide with tutorial and FAQ (also [in Swedish](docs/user-guide/sv/index.md) and in the tool under Help) |
| [docs/user-guide/en/references.md](docs/user-guide/en/references.md) | references: RFC 2119, RFC 8174 and other RFCs, NIST publications, requirement and testing standards |
