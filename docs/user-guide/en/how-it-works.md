# How Qtestframe works

This page explains how Qtestframe is built up and how its parts work together, without going into the source code. Read it when you want to understand why the tool behaves as it does, or to explain it to colleagues. The words in bold are explained in the [Glossary](glossary.md); administrators find the details in the [administrator guide](../../admin-guide/en/index.md).

## One program in your browser

Qtestframe is **one program** on a server. The same program contains the web application you use in the browser, the REST API that the web application, CI pipelines and other tools call, and the command line tools of the administrator. Nothing needs to be installed on your computer: a current browser is enough, on a desktop, tablet or phone.

- The web application is a single-page application: after the first load it only exchanges data with the server, so pages switch quickly and your unsaved input stays in the page while you move around.
- Everything you can do in the browser can also be done through the API, described as **[OpenAPI](glossary.md#openapi)** at `/api/v1/openapi.json`. This is how [`qtestframe report`](runs.md#the-qtestframe-report-command), the JUnit upload, the Playwright reporter and other tools work.
- The user interface and this guide are in English and Swedish; the [language analysis](requirements.md#the-language-analysis) understands requirement texts in both.

## Where your data lives

Everything is stored in **one database**: organisations, users, projects, requirements, tests, runs and their results, history, comments, the audit log and also uploaded files (attachments, documents, generated reports). There is no separate file share.

- The database is **SQLite** (a single file on the server, no database server needed) or **PostgreSQL** (for larger installations or several servers behind a load balancer). You see no difference; your administrator chooses.
- Secrets that the server must read back, such as tracker tokens or webhook secrets, are encrypted with a key that is kept outside the database (see [Security](#security)).
- Deleted items go to the trash for 30 days before they are removed for good.

## Organisations, projects and roles

An installation can host several **organisations** (tenants) that never see each other's data. You can be a member of several and switch between them in the top bar.

| Level | What belongs to it | Who manages it |
|---|---|---|
| Installation | users, organisations, system settings, backups | super admins |
| Organisation | members and their roles, projects, API tokens, test configurations, execution settings, the audit log | organisation admins |
| Project | requirements, tests, runs, test cycles, exploratory sessions, defects, risks, releases, waivers, baselines, documents, project settings | managers (and project roles) |

What you may do is decided by your **role** in the organisation: *Viewer*, *Reviewer*, *Tester*, *Test lead*, *Manager* or *Admin*, or a role your organisation created. A role is a set of permissions such as `requirement.write` or `run.execute`. A project can be restricted to some members and give someone a different role in that project only. The server checks every request against these permissions; buttons you may not use are hidden or disabled, and the reason is shown. See [Roles and permissions](administration.md#roles-and-permissions).

## The items and how they relate

![Diagram of the items of a project: requirements with evaluation criteria and verification records, risks they address, tests that verify them, releases, baselines, test cycles that create runs, runs that contain executions of tests, defects found in executions, and exploratory sessions whose notes become defects](../images/en-item-model.png)

Every item has a **key** made of the project key and a number, such as `SHOP-REQ-12` or `SHOP-TC-7`, a status, an owner and a **version history**. The most important relations:

| Item | What it is | Related to |
|---|---|---|
| [Requirement](requirements.md) | what the product must, should or may do | its **evaluation criteria**; other requirements (*refines*, *derived from*, *depends on* …); the **risks** it addresses; the tests that verify it |
| [Test](tests.md) | a manual test with steps or an automated test with a script | the requirements (and criteria) it **verifies** through trace links; tests it calls as shared steps or depends on |
| [Run](runs.md) | one round of testing in an environment and build | its **executions**: one per test (and configuration), each with a result, step results, evidence and defects |
| [Test cycle](test-cycles.md) | the plan for a testing period | the tests, testers and estimates it plans; the run it creates |
| [Exploratory session](exploratory-testing.md) | time-boxed exploratory testing with a charter | the requirements in its scope; the defects raised from its notes |
| [Defect](defects.md) | a problem found | the executions, tests and requirements it is linked to; an issue in Jira, GitHub or GitLab |
| [Risk](releases-and-risks.md#the-risk-register) | what could go wrong, by likelihood and impact | the requirements that address it; MITRE techniques |
| [Release](releases-and-risks.md) | a version to deliver | its **scope** (requirements), its runs, its **exit criteria**, waivers |
| [Waiver](waivers-and-soa.md) | a signed acceptance of unmet requirements | the requirements and the release it applies to, a risk |
| [Baseline](baselines.md) | a frozen set of versions | the version of every requirement and test at that moment |
| [Document](documents.md) | a file in the project archive | the items it is linked to |

Coverage, verification, staleness and release readiness are **never stored**: they are computed from these items every time you look, so they are always current.

## From requirement to release

A typical flow through a project, and what Qtestframe does at each step:

1. **Write requirements.** The language analysis checks the text as you type and detects the [obligation level](requirements.md#obligation-levels) from MUST, SHOULD, MAY (or SKA, BÖR, FÅR). Add [evaluation criteria](requirements.md#evaluation-criteria) that say how fulfilment is judged. A [review](reviews-and-notifications.md#request-a-review) approves them.
2. **Design tests** and link them to the requirements. A new requirement can get a linked test automatically. The [traceability matrix](traceability-and-coverage.md#the-traceability-matrix) and [traceability rules](traceability-and-coverage.md#traceability-rules) show what is missing.
3. **Plan and run.** A [test cycle](test-cycles.md) or a [run](runs.md) selects the tests; before a test is executed, the [execution rules](runs.md#when-can-a-test-be-executed) check that it is ready. Testers record results in the runner; CI uploads automated results.
4. **Follow up.** Failures become [defects](defects.md); [coverage](traceability-and-coverage.md#how-coverage-is-calculated) and [verification](traceability-and-coverage.md#verification) are derived from the latest results; the [dashboard](traceability-and-coverage.md#the-dashboard) shows the state.
5. **Change.** When a requirement or test changes in substance, its links become **suspect**, its results **stale**, and the [change impact](traceability-and-coverage.md#change-impact) travels on to related items, documents and issues.
6. **Release.** A [release](releases-and-risks.md) checks its exit criteria and shows go or no-go; [waivers](waivers-and-soa.md) accept what is knowingly not met; a [baseline](baselines.md) freezes the result, and [reports](reports-and-export.md) document it.

## Versions, history and baselines

Every change of a requirement, test and most other items creates a new **revision**: the **History** tab shows who changed what and when, compares versions and restores an old one. **Undo** in the top bar takes back your last actions. Two people editing the same item do not overwrite each other: the second save is refused with the version it is based on (an [ETag](glossary.md#etag-and-if-match)), and you choose how to merge (see [When someone else saved first](requirements.md#when-someone-else-saved-first)).

A **baseline** records the revision of every item at one moment, for a release or an audit. The **audit log** records actions across the organisation: sign-ins, review decisions, signatures, imports, exports and settings.

## The query language

All lists, the traceability matrix, release scopes, test selection for runs, reports, traceability rules and the API use the same [query language](search-and-filters.md): `status:approved priority=high`, `-has:tests`, `linked:SHOP-TC-7`, `updated_at>=2026-01-01`, `title~/login/i`. What you learn in one list works everywhere. Global search covers all projects of the current organisation that you can read.

## Background work

Some work happens on the server without you waiting for it:

- **Automated tests** that you start with **Run automated tests** run as jobs, a few at a time; results appear in the run as they finish.
- **Notifications** and **webhooks** are sent after the change that caused them is saved.
- **E-mail digests** and **scheduled reports** go out at the times you chose.
- A daily **snapshot** of each project's metrics feeds the trends on the dashboard.
- Expired sessions and trash entries older than 30 days are purged every hour.

## Notifications, webhooks and chat

You are told about what concerns you: review requests, mentions, items assigned to you, replies in your discussions, status changes of your defects. [Notifications](reviews-and-notifications.md#notifications) appear in the app and, if you want, by e-mail at once or as a daily digest. [My work](reviews-and-notifications.md#my-work) lists everything waiting for you across projects.

Projects can also send their events to other systems: **[webhooks](reviews-and-notifications.md#webhooks)** deliver signed JSON requests to any receiver, and **[chat channels](reviews-and-notifications.md#chat-notifications)** post readable messages to Slack, Microsoft Teams, Mattermost, Rocket.Chat, Google Chat, Matrix or Discord, filtered by event, priority and release.

## Integrations

| With | How | Read more |
|---|---|---|
| Defect and issue trackers (Jira, GitHub Issues, GitLab, Request Tracker) | push defects and requirements as issues, sync their status, import issues | [Defects](defects.md#connect-jira-github-or-gitlab) |
| Jira and Azure DevOps | import requirements, update them later | [Reports and export](reports-and-export.md#jira-and-azure-devops) |
| Code hosts (GitHub, GitLab) | webhooks link commits and merge requests that name an item key; builds record which commits were tested | [Variants and code](variants-and-code.md#code-links) |
| CI pipelines | `qtestframe report`, JUnit XML upload, Playwright reporter, with an API token | [Import results from CI](runs.md#import-results-from-ci) |
| Requirements and ALM tools (DOORS, DOORS Next, Polarion, Codebeamer, Jama, ProR) | ReqIF files in both directions; live OSLC links, selection and creation | [Interoperability](interoperability.md) |
| Office tools | export to PDF, Word, Excel, CSV and Markdown; edit in Excel and import back | [Reports and export](reports-and-export.md) |
| Identity providers | single sign-on with OpenID Connect or SAML 2.0, user provisioning with SCIM | [Administration](administration.md#provisioning-with-scim) |

The standards and formats behind these integrations are listed in the [Glossary](glossary.md#entries-by-topic) and on the [References](references.md#formats-protocols-and-frameworks) page.

## Security

### Sessions and CSRF protection

When you sign in, the server creates a **session** and your browser gets a random cookie that scripts in the page cannot read. Sessions end after a maximum age (12 hours by default), when you sign out, or when an administrator resets your password or disables your account; **Profile → Active sessions** lists yours. Every change you make sends a per-session **CSRF token** and passes an origin check, so another website cannot act in your name. Scripts and CI use **API tokens** instead, which are shown once and stored only as a hash.

### Single sign-on and multi-factor authentication

Your organisation can let you sign in with its identity provider through **OpenID Connect** or **SAML 2.0**; users and groups can be provisioned automatically with **SCIM**. Local accounts can, or must, add a second factor: a **TOTP** code from an authenticator app, with single-use recovery codes. See [Multi-factor authentication](getting-started.md#multi-factor-authentication).

### Secrets

Passwords and tokens that only need to be checked are stored as hashes. Secrets the server must use itself, such as authenticator secrets, webhook secrets, tracker tokens and secret test variables, are encrypted in the database with a key kept outside it (a key file, an environment variable, a TPM or an HSM). Secrets are **write-only**: once saved, no screen or API returns them, and project archives never contain them.

### The script sandbox

When your administrator enables it, automated test scripts and Playwright web tests run on the server. Each script runs in a separate, restricted process: limited CPU time, file sizes and open files, no new privileges, file access limited to its own job directory with **Landlock**, dangerous system calls blocked with **seccomp**, and optionally no network. The server restricts itself in the same way after start-up. Only people with the `script.execute` permission can run scripts.

## Backups and project archives

- A **backup** contains the whole installation: all organisations with their data and files. Super admins take it in the web client or on the server; a restore replaces everything and signs everyone out. See [Backup and restore](administration.md#backup-and-restore).
- A **project archive** contains one project, with history, results and attachments but without secrets, as a ZIP file. Load it as a new project on the same or another server. See [Save and load projects](reports-and-export.md#save-and-load-projects).
- **Exports** (PDF, Word, Excel, CSV, Markdown, ReqIF) take data out for other people and tools.

## For administrators

The [administrator guide](../../admin-guide/en/index.md) describes how to run Qtestframe:

- the [overview and architecture](../../admin-guide/en/overview.md): components, data stores, ports, outgoing connections and background tasks;
- [installation](../../admin-guide/en/installation.md) and [configuration](../../admin-guide/en/configuration.md), including the choice between SQLite and PostgreSQL;
- [authentication](../../admin-guide/en/authentication.md) (passwords, MFA policies, OIDC, SAML, SCIM, API tokens, sessions) and [authorisation](../../admin-guide/en/authorisation.md);
- [security](../../admin-guide/en/security.md): TLS, hardening with Landlock and seccomp, the script sandbox, encryption of stored secrets and key rotation;
- [integrations](../../admin-guide/en/integrations.md), [connecting other tools](../../admin-guide/en/connecting-tools.md) over OSLC and ReqIF, [logging and monitoring](../../admin-guide/en/logging-and-monitoring.md) with syslog;
- [backup, restore and migration](../../admin-guide/en/backup-and-migration.md), the [runbook](../../admin-guide/en/runbook.md) and the [command line reference](../../admin-guide/en/cli-reference.md).
