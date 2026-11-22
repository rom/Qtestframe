# REST API (v1)

All endpoints are served below `/api/v1` and exchange JSON (`Content-Type:
application/json`) unless stated otherwise; request bodies are limited to
8 MiB. Field shapes are defined by the Go types in
`internal/model/model.go` (JSON tags are authoritative). Outside `/api/v1`
the server offers `GET /healthz` (`{"status":"ok","version"}`) and
`GET /readyz` (`{"status":"ready"}`, or 503 `unavailable` when the database
is unreachable); all other paths serve the web client.

## Conventions

* **Authentication** – browser clients use the `qtf_session` cookie
  (HttpOnly, Secure, SameSite=Lax). Non-interactive clients send
  `Authorization: Bearer qtf_<token>` (API tokens, see below).
* **CSRF** – every authenticated request with a method other than GET/HEAD
  made with a session cookie must carry the header `X-CSRF-Token` with the
  value returned as `csrf_token` by `GET /auth/me` / `POST /auth/login`, and
  an `Origin` header, if sent, must match the request host or
  `server.base_url`. Violations return 403. Bearer-token requests are
  exempt.
* **Tenant** – the session has a *current tenant*; all project data is scoped
  to it. Switch with `POST /auth/switch-tenant`. Bearer tokens are bound to
  one tenant; their role is capped at the user's current role.
* **Language** – the API itself is English (error messages, export
  headers); the web client translates its own texts.
* **Errors** – non-2xx responses have the body
  `{"error": "<message>", "code": "<machine_code>"}`. Codes: `bad_request`
  (400), `unauthorized` (401), `forbidden` (403), `not_found` (404),
  `conflict` (409, also for duplicates and a disabled runner), `stale`
  (412, see *Concurrent edits*; 409 for a [batch update](#batch-update-spreadsheet-view)),
  `invalid_query` (400), `rate_limited` (429, login), `unavailable` (503,
  `/readyz`), `internal` (500).
* **Bulk requests are all or nothing** – every bulk endpoint (the `…/bulk`
  routes, bulk review, sign-offs and restore, link confirmation, execution
  and notification batches) runs in one database transaction. An error
  answer (4xx/5xx) means none of the changes was saved; items listed in
  `skipped` with a reason are part of a successful answer, so the others
  were saved. Webhooks for the changes are sent only after they are saved.
  Exceptions, because they call outside services: pushing and syncing
  defects and requirement issues with the tracker (`/defects/push`,
  `/defects/sync`, `/requirements/{id}/issue`, `…/requirements/issues`), webhook
  tests and report batches, which save item by item.
* **Concurrent edits** – `GET` and `PATCH` of a requirement or test answer
  with `ETag: "<version>"`. Send `If-Match: "<version>"` with a `PATCH` to
  save only if nobody saved in between; otherwise the answer is `412` with
  code `stale` and `{"error", "code", "current": <the saved item>,
  "changed_by", "changed_at"}`. Without `If-Match` the change is saved as
  before (two saves that race are still refused with 412). The web client
  always sends it and lets the user compare and merge.
* **OpenAPI** – a machine-readable description of all routes is served at
  `GET /api/v1/openapi.json` (see [OpenAPI description](#openapi-description)).
* **IDs** are UUID strings. Times are RFC 3339.
* **Lists** return `{"items": [...], "total": n}`. There is no pagination
  (except `limit` on search and audit).
* **Filtering** – list endpoints for requirements, tests, runs, executions
  accept `?q=<filter expression>` (see *Query language*); so does the audit
  log. Invalid expressions return 400 with code `invalid_query` and
  `{"error","code","position"}` (byte offset).

## Auth

| Method | Path | Body / Notes | Response |
|---|---|---|---|
| GET | `/auth/providers` | public | `{"local":true,"oidc":[{"name","display_name","login_url"}],"saml":[{"name","display_name","login_url"}]}` |
| POST | `/auth/login` | `{"email","password","tenant"?}` (tenant = slug); 401 on bad credentials, 429 after `auth.login_max_attempts` attempts (default 10) within `auth.login_lockout` (default 5 min) per IP + e-mail, 403 if local login is disabled | `Me` + `qtf_session` cookie, or `{"mfa_required": true, …}` + `qtf_mfa` cookie when a second factor is needed (see [Multi-factor authentication](#multi-factor-authentication)) |
| POST | `/auth/logout` | deletes the session | 204 |
| GET | `/auth/me` | | `Me` |
| POST | `/auth/switch-tenant` | `{"tenant_id"}` | `Me` |
| POST | `/auth/password` | `{"current_password","new_password"}` (local accounts only) | 204 |
| GET | `/auth/oidc/{name}/login` | browser redirect | 302 |
| GET | `/auth/oidc/{name}/callback` | browser redirect | 302 → `/`, or → `/login?mfa=1` when the provider's `local_mfa` asks for Qtestframe's second factor |
| GET | `/auth/saml/{name}/metadata` | SP metadata XML | XML |
| GET | `/auth/saml/{name}/login` | browser redirect | 302 |
| POST | `/auth/saml/{name}/acs` | IdP POST binding | 302 → `/` |

`Me`:
```json
{
  "user": User,
  "tenant": Tenant | null,
  "role": "admin|manager|test_lead|tester|reviewer|viewer|<custom>|",
  "permissions": ["project.read", ...],
  "memberships": [Membership],
  "csrf_token": "...",
  "mfa": MFAStatus
}
```

`User` and `Membership` carry `"mfa_enabled"` (a confirmed second factor);
in `GET /users` and `GET /members` it is filled for every row, and members
also have `"mfa_required"` (the server or tenant policy requires MFA for
their role here).

## Multi-factor authentication

A second factor (TOTP authenticator app, RFC 6238: SHA-1, 6 digits, 30 s,
±1 step; each code is accepted once) plus 10 single-use recovery codes.
API tokens are not affected. Design and protections: [security.md](security.md#multi-factor-authentication).

**Sign-in.** When the password is right and the account has a second
factor (and the device is not remembered), `POST /auth/login` answers

```json
{"mfa_required": true, "methods": ["totp", "recovery_code"], "remember_days": 0, "expires_at": "…"}
```

and sets the `qtf_mfa` cookie (5 minutes, `Path=/api/v1/auth/`); there is
no session yet. Then:

| Method | Path | Body / Notes | Response |
|---|---|---|---|
| GET | `/auth/mfa/challenge` | public; the pending sign-in (after an OIDC redirect to `/login?mfa=1`) | as above; 401 `mfa_expired` |
| POST | `/auth/mfa/verify` | public; `{"code", "remember"?}` – a 6-digit code or a recovery code; `remember` (when the server policy has `remember_days` > 0) sets the `qtf_mfa_device` cookie | `Me` + `qtf_session`; 401 `mfa_invalid` (wrong or reused code, recorded as a failed sign-in with method `mfa`), 401 `mfa_expired` (none pending, expired, or 5 wrong codes), 429 `rate_limited` |

A session whose policy requires a second factor that is not set up yet
(grace period over) gets **403 `mfa_enrolment_required`** from every
endpoint outside `/api/v1/auth/` until the authenticator is confirmed.

**Own second factor** (browser sessions only; API tokens get 403):

| Method | Path | Body / Notes | Response |
|---|---|---|---|
| GET | `/auth/mfa` | | `MFAStatus` |
| POST | `/auth/mfa/totp` | starts the set-up; the secret is returned only here | `{"secret", "otpauth_uri", "issuer", "account", "digits", "period", "algorithm"}`; 409 when already on |
| POST | `/auth/mfa/totp/confirm` | `{"code"}` | `{"recovery_codes": [10 codes]}` (shown once); 400 `mfa_invalid` |
| POST | `/auth/mfa/recovery-codes` | `{"code"}` (TOTP or recovery code) | `{"recovery_codes": [...]}`; old codes stop working |
| POST | `/auth/mfa/disable` | `{"code"}` (TOTP or recovery code); a pending set-up needs none | 204; 403 `mfa_required` when a policy requires MFA |
| DELETE | `/auth/mfa/trusted-devices` | forgets remembered devices | `{"forgotten": n}` |

`MFAStatus`:

```json
{
  "enabled": true, "enabled_at": "…", "pending": false,
  "recovery_codes_left": 9, "recovery_codes_total": 10,
  "trusted_devices": 0, "remember_days": 0,
  "required": true, "enrolment_deadline": null, "enrolment_required": false
}
```

`enrolment_deadline` is set while a required second factor is missing (the
end of the grace period); `enrolment_required` is true for a session that
must set it up first.

**Policy and resets:**

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/admin/mfa` | tenant.manage | `{"server": MFAPolicy, "tenant": MFAPolicy\|null, "can_edit_server", "members", "members_enrolled", "server_roles", "users"?, "users_enrolled"?}` |
| PUT | `/admin/mfa/server` | super admin | `{"mode", "roles", "grace_days", "remember_days"}` → MFAPolicy; roles are built-in roles or `super_admin` and count in any tenant |
| PUT | `/admin/mfa/tenant` | tenant.manage | `{"mode", "roles", "grace_days"}` for the current tenant (its roles); adds to the server policy |
| POST | `/members/{user_id}/mfa/reset` | member.manage | removes authenticator, recovery codes and remembered devices, signs the user out, notifies them (in-app and e-mail) → 204; 409 when they have none; 403 for super administrators and users of other tenants (as for password resets) |
| POST | `/users/{id}/mfa/reset` | super admin | the same for any user |

`MFAPolicy`: `{"scope": ""|tenant id, "mode": "optional"|"all"|"roles",
"roles": [...], "grace_days": 0-365, "remember_days": 0-365 (server only),
"required_since", "updated_by", "updated_at"}`. The grace period counts
from `required_since` (set when the policy starts to require MFA for more
people) or the user's creation, whichever is later. Policy changes and
resets are audited (`mfa.policy`, `member.reset_mfa`, `user.reset_mfa`), as
are `mfa.enable`, `mfa.disable`, `mfa.recovery_codes`,
`mfa.recovery_code_used` and `mfa.forget_devices`.

## Administration

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/tenants` | super admin (all) / member (own) | list |
| POST | `/tenants` | super admin | `{"slug","name"}` |
| PATCH | `/tenants/{id}` | super admin or tenant.manage | `{"name"}` |
| DELETE | `/tenants/{id}` | super admin | |
| GET | `/users` | super admin | list all users |
| POST | `/users` | super admin | `{"email","display_name","password","super_admin"}` |
| PATCH | `/users/{id}` | super admin | `{"display_name","disabled","super_admin","password"}`; disabling or setting a password ends the user's sessions; you cannot demote or disable yourself (409) |
| GET | `/members` | member.read | members of current tenant; `"scim": true` marks members provisioned by SCIM, whose role and membership only the identity provider changes (`PATCH`, `DELETE` and `POST /members` answer 409 `scim_managed` for them); a member with an open e-mail invitation has `"invitation": {id, sent_at, expires_at, sso, expired, invited_by}` |
| POST | `/members` | member.manage | `{"email","role","display_name"?,"password"?,"invite"?,"sso"?}` – creates the local user if it does not exist. With `"invite": true` (needs e-mail, see [E-mail](#e-mail)) a person without a used account gets an invitation e-mail instead of an initial password (`password` must then be empty), and the answer has `invitation`; an existing account only gets an e-mail that it was added. `"sso": true` invites to sign in with single sign-on |
| POST | `/members/{user_id}/invitation` | member.manage | sends a new invitation link (the old link stops working) → invitation; 409 when the person already uses the account |
| DELETE | `/members/{user_id}/invitation` | member.manage | revokes the open invitation → `{"revoked": true, "removed": bool}`; a person who never used the account is also removed from the tenant |
| PATCH | `/members/{user_id}` | member.manage | `{"role"}`; the last admin cannot be demoted (409) |
| DELETE | `/members/{user_id}` | member.manage | the last admin cannot be removed (409) |
| POST | `/members/bulk` | member.manage | `{"user_ids":[...], "action": "role"\|"remove", "role"?}` (max 1000, all or nothing) → `{"changed":[{user_id, email, role}], "skipped":[{user_id, email, role, reason}]}`; reasons `self` (your own membership is never changed in bulk), `not_found`, `scim` (provisioned by SCIM), `unchanged`. A change that would leave no admin is refused (409). For a removal, `role` is the role the member had |
| GET | `/rbac` | any | `{"roles":[...],"matrix":{role:[perm]},"permissions":[...]}` – the effective matrix of the current tenant |
| GET | `/roles` | any member | `{"items":[Role],"permissions":[...]}`; Role = `{name, label?, description?, permissions, builtin, customized?, editable, deletable, members, tokens}` |
| POST | `/roles` | role.manage | `{"name","label","description"?,"permissions":[...]}` → 201 Role; `name` is 2–32 of `a-z0-9_` starting with a letter; 409 when it exists |
| PUT | `/roles/{name}` | role.manage | `{"label"?,"description"?,"permissions"?}`; `admin` cannot be changed (400) |
| POST | `/roles/{name}/reset` | role.manage | built-in roles only: back to the default permissions → 204 |
| DELETE | `/roles/{name}` | role.manage | custom roles only; 409 while members or API tokens use it |
| GET | `/settings` | any member | `TenantSettings` (below) |
| PUT | `/settings` | settings.manage | `TenantSettings` → the stored settings |
| GET | `/tokens` | token.manage | own tokens in tenant |
| POST | `/tokens` | token.manage | `{"name","role","expires_days"?}` → `{"token":"qtf_...", "item": APIToken}` (token only shown once; role defaults to the own role, and its permissions must all be held by the creator; a token cannot create tokens; on every request a token gets the intersection of its role's and the user's current role's permissions) |
| DELETE | `/tokens/{id}` | token.manage | |
| GET | `/audit?limit=&q=` | audit.read | audit log of tenant, newest first (`limit` default 500, max 5000; query fields `action, entity, entity_id, user, details, created_at`); entries include `user_name` and `user_avatar` of the acting user |
| GET | `/meta` | any | enumerations: `{"obligations","requirement_statuses","requirement_types","priorities","test_statuses","script_languages","evaluation_methods","run_statuses","result_statuses","roles"}` plus `"analysis_languages"` (language packs of `/reqlang/analyze`), `"runner_enabled"`, `"email_enabled"`, `"email_digest_hour"` (UTC) and `"version"` |
| POST | `/admin/email/test` | tenant.manage | sends a test e-mail to the caller's own address now → `{"ok", "to", "error"?}` with the SMTP error; 400 when e-mail is not configured |

Roles: nobody can grant a permission they do not hold (403 when a role
change or new role adds one). Role changes are audited (`role.create`,
`role.update`, `role.reset`, `role.delete`).

`TenantSettings` (the test execution policy; every `false` is the strict
default):

```json
{
  "allow_unready_tests": false,
  "allow_unready_requirements": false,
  "ready_requirement_statuses": ["approved", "implemented", "verified"],
  "allow_unmet_dependencies": false
}
```

## Projects

| Method | Path | Permission |
|---|---|---|
| GET | `/projects` | signed in – the projects the caller may read (see [Project sharing](#project-sharing-per-project-access)), each with `access` and `my_access` |
| POST | `/projects` | project.create – `{"key","name","description","color"?,"type"?,"tech_context"?,"seed_categories":true}` |
| GET | `/projects/{pid}` | project.read |
| PATCH | `/projects/{pid}` | project.update – `{"name","description","color","requirement_sources","type","tech_context"}` |
| POST | `/projects/{pid}/prompts` | project.read + the read permission of the items' kind – see [Prompts](#prompts-software-projects) |
| DELETE | `/projects/{pid}` | project.delete |
| GET | `/projects/{pid}/dashboard` | project.read – see [Dashboard and metrics](#dashboard-and-metrics) |
| GET | `/projects/{pid}/metrics/trends` | project.read – `?range=30\|90\|365` or `?since=YYYY-MM-DD` |
| GET | `/projects/{pid}/flaky-tests` | project.read – `?limit=n` |
| GET | `/projects/{pid}/dashboard/insights` | project.read – further key figures of the dashboard, see [Dashboard and metrics](#dashboard-and-metrics) |
| GET | `/projects/{pid}/requirement-sources` | requirement.read – `{"configured":[...], "custom":bool, "used":[{"source","count"}]}`: the project's list and the sources requirements use, most used first |
| GET | `/projects/{pid}/archive` | project.update + export – save the project as a [project archive](#project-archives-save-load-duplicate) (`application/zip`) |
| POST | `/projects/import` | project.create – multipart `archive` + optional `key`, `name`, `dry_run=true`: load an archive as a new project → 201 `ProjectLoadResult` |
| POST | `/projects/{pid}/duplicate` | project.create + project.update + export – `{"key","name"?}`: save and load in one step → 201 `ProjectLoadResult` |

A `Project` has `color` (`#rrggbb` or `""` for the theme colour; anything
else → 400), `requirement_sources` (the list the requirement editor offers
for *Source*; at most 50 entries of at most 100 characters, trimmed and
de-duplicated case-insensitively) and `custom_sources` (false while the
project uses the defaults; sending `[]` or the default list returns to
them). `type` is one of `/meta` `project_types` (`general` by default,
`software`, `hardware`, `system`, `business`); `tech_context` (at most 8000
characters) describes the technology of a software project and is included
in prompts.

`access` is `organisation` or `restricted` (see below); `my_access`
(`GET /projects`, `GET /projects/{pid}`, `POST /projects`) is the caller's
access to the project: `{"source": "admin"|"member"|"group"|"organisation",
"role", "permissions": [...], "can_share": bool}` – `permissions` are the
effective permissions in the project, which clients use to show or hide
actions inside it.

### Project sharing (per-project access)

A project is **organisation-wide** (`access: "organisation"`, the default:
every member of the organisation sees it with their organisation role) or
**restricted** (`access: "restricted"`: only its project members and the
organisation's administrators see it; everyone else gets **404** for the
project and for every resource in it, also when addressed by its own id,
e.g. `/requirements/{id}`, `/runs/{id}`, `/executions/{id}`, `/files/{id}`).

A project member is a user of the organisation or a SCIM group, with a
**project role** – any role of the organisation (built-in or custom). The
effective permissions of a caller in a project:

* organisation administrators (role `admin`) and super administrators keep
  their organisation permissions in every project;
* a project member gets the project permissions of the project role
  (`project.*` except `project.create`, `requirement.*`, `waiver.approve`, `test.*`, `run.*`,
  `script.execute`, `export`) **instead of** those of the organisation role
  – in organisation-wide projects too, so a project role raises or lowers a
  user's rights in one project. A direct membership wins over group
  memberships; several groups give the union of their roles;
* anyone else has their organisation role in an organisation-wide project
  and no access to a restricted one.

Organisation-level permissions (`member.*`, `role.manage`,
`settings.manage`, `tenant.manage`, `token.manage`, `audit.read`,
`project.create`) always come from the organisation role. An API token
never exceeds its own role, inside a project too. Lists across projects
(`/projects`, `/search`, `/reviews`, `/reviews/in-review`,
`/notifications`, the catalogue's `?project_id=`) leave out what the caller
may not see; moving folders or items into another project needs write
access there; notifications, @mentions, report recipients and review
requests only reach people who may see the project.

| Method | Path | Permission |
|---|---|---|
| GET | `/projects/{pid}/access` | project.read – `{"access", "members": n, "my_access", "grantable_roles": [...]}` |
| PUT | `/projects/{pid}/access` | project.update – `{"access": "organisation"\|"restricted"}`; restricting keeps the caller in the project (a caller who is neither an administrator nor a member becomes a member with their organisation role) |
| GET | `/projects/{pid}/members` | project.read – `ProjectMember`: `{"type": "user"\|"group", "id", "role", "email", "display_name", "avatar", "org_role", "added_by", "created_at", "updated_at"}` |
| GET | `/projects/{pid}/members/candidates` | project.update – `?q=`: organisation members and SCIM groups that are not project members yet (at most 50) |
| POST | `/projects/{pid}/members` | project.update – `{"user_id" \| "email" \| "group_id", "role"}` → 201 (200 when the member existed: the role is changed); the user must already be a member of the organisation (400 otherwise) |
| PATCH | `/projects/{pid}/members/{mid}` | project.update – `{"role"}` |
| DELETE | `/projects/{pid}/members/{mid}` | project.update, or the member themselves (leaving) → 204 |
| POST | `/projects/{pid}/members/bulk` | project.update – `{"ids": [...], "action": "role"\|"remove", "role"?}`, all or nothing → `{"changed": [...], "skipped": [{"id","label","reason": "not_found"\|"unchanged"\|"forbidden"}]}` |

Nobody can give, take or remove a project role whose project permissions
they do not hold in the project themselves (403; administrators may give
any role). An added user is notified (`project.shared`, in the app and by
e-mail per their preferences). Changes are audited: `project.access`,
`project.member.add`, `project.member.role`, `project.member.remove`.
Removing someone from the organisation or deleting a SCIM group removes
their project memberships; a custom role used by a project member cannot
be deleted (409). A duplicated project keeps the original's members; a
loaded or duplicated restricted project keeps the caller in it.

### Project archives (save, load, duplicate)

`GET /projects/{pid}/archive` streams one project as a ZIP file
(`<key>-project-<yyyymmdd-hhmmss>.zip`), read in one consistent snapshot:

| Entry | Content |
|---|---|
| `manifest.json` | `{"kind":"qtestframe-project", "format_version":1, "schema_version", "app_version", "created_at", "driver", "project":{"id","key","name"}, "tables":{table: rows}, "files", "file_bytes"}` |
| `tables/<table>.jsonl` | every row of the project, one JSON object per line (column → value; times RFC 3339, booleans `true`/`false`), the same portable encoding as a [backup](#administration-additions) |
| `files/<file id>` | the content of each document, attachment, report and the project icon |

The tables are the project row (settings, sequence counters, defect tracker
settings), configurations, releases, folders, categories, requirements with
categories and criteria, tests with steps and categories, trace links, runs
with run configurations, executions, iterations and step results, relations,
baselines with items and sign-offs, files and attachments, report
definitions, test dependencies, item links, release scopes, defects and their
links, risks and their requirements, webhooks, review requests, metric
snapshots, traceability rules, and the revisions and comments of all these
rows. **Not included:** webhook secrets (webhooks are saved switched off),
the tracker token, accounts, memberships, roles, sessions, API and SCIM
tokens, invitations, notifications, e-mail report subscriptions, the audit
log and the trash. References to people (`created_by`, `user_id`,
`reviewer_id`, e-mail addresses) are kept as plain values.

`POST /projects/import` (multipart: `archive`, optional `key` and `name`,
default the archive's) creates a **new** project in the current organisation
in one transaction:

- every row gets a new id; every value that is an id of an archived row is
  rewritten, in id columns, in JSON (snapshots, options) and in texts
  (descriptions that link to an item). When the key changes, item keys and
  key references in texts follow (`OLD-REQ-1` → `NEW-REQ-1`); the sequence
  counters are kept, so new items get the next free key;
- foreign keys are checked against the archive: a reference to a row
  outside it is cleared when optional, otherwise the row is left out (as
  are its dependants); a reference to an account that does not exist on
  this server (e.g. a reviewer) leaves the row out. Left-out rows are
  counted in `skipped`;
- secrets are stripped again and every `tenant_id` is the importer's
  organisation, so a hand-made archive cannot reach other projects.

Answer (201): `ProjectLoadResult` = `{"project": Project, "manifest": {…},
"imported": {table: rows}, "skipped": {table: rows}?, "files": n}`. With
`dry_run=true` everything is checked and rolled back (200, `"dry_run": true`,
`"key_taken": true` when the key exists). Errors: 400 for files that are not
project archives (also full backups), unknown entries, wrong counts or
checksums and invalid keys; 409 when the key is taken or the archive was
saved with a newer schema version (older archives load; missing columns get
their defaults). The upload is limited like a restore (4 GiB uncompressed).
Audit actions: `project.save`, `project.load`, `project.duplicate`.

Roles are organisation-wide, so the loading user's access to the new project
is that of their role; no membership is created or copied.

### Prompts (software projects)

`POST /projects/{pid}/prompts` builds a prompt for an AI assistant
(Markdown); it is read-only and nothing is sent to an AI service. Only for
projects of type `software` (else 400).

```json
{"kind": "automate_tests", "lang": "en", "test_type": "integration", "framework": "pytest",
 "notes": "…", "test_ids": ["…"], "q": "tag:checkout"}
```

* `kind` (`/meta` `prompt_kinds`): `implement`, `review_requirements`,
  `design_tests` (requirements; need `requirement.read`), `automate_tests`,
  `review_tests`, `test_strategy` (tests; need `test.read`; the last two also
  include the requirements the tests verify when the caller may read them).
* Items: `requirement_ids` / `test_ids` (must belong to the project), else
  those matching `q`, else all; more than 200 → 400.
* `test_type` (`/meta` `test_types`) is required for `design_tests` and
  `test_strategy`; `lang` is `en` (default) or `sv`; `framework` ≤ 200 and
  `notes` ≤ 8000 characters.
* Response: `{"kind", "lang", "title", "prompt", "items"}`.

### Dashboard and metrics

`GET /projects/{pid}/dashboard` aggregates on the server:
```json
{
  "requirements": {"total": 10, "by_status": {"draft": 3}, "by_obligation": {"must": 4}, "covered": 7, "uncovered": 3},
  "tests": {"total": 20, "by_execution_type": {"manual": 12, "automated": 8}, "by_status": {}},
  "coverage": {"passed": 5, "failed": 1, "not_run": 1, "none": 3, "partial": 0},
  "runs": {"total": 3, "by_status": {}},
  "recent_runs": [TestRun],
  "results": {"passed": 30, "failed": 2, ...},
  "active_runs": [TestRun],
  "coverage_by_folder": [{"id": "…", "name": "Authentication", "total": 4, "coverage": {"passed": 2, "none": 2}}],
  "coverage_by_category": [{"id": "…", "name": "Security", "dimension": "Area", "total": 3, "coverage": {}}],
  "defects_by_severity": {"critical": 0, "major": 2, "minor": 1, "trivial": 0},
  "flaky": {"total": 3, "items": [FlakyTest], "window": 10, "min_flips": 2}
}
```

* `active_runs`: planned and in-progress runs, newest first (at most 8),
  with their `summary` (results per status).
* `coverage_by_folder`: requirements per top-level folder (`id` `""` = no
  folder) and `coverage_by_category`: per requirement category; both with
  the coverage states, largest groups first (at most 12).
* `defects_by_severity`: open defects (`open`, `in_progress`).
* `flaky`: the 10 flakiest tests (see below) and how many there are.

Opening the dashboard also records today's metric snapshot (again when the
one of today is older than an hour).

**Metric snapshots.** Every project gets one snapshot of its figures per
UTC day: a background job of the server records it for every project
without one for the day (at start and then hourly), and the dashboard and
the trends request record it when it is missing. When a project gets its
first snapshot, the days before (from the first requirement, test, result
or defect; at most 365 days) are reconstructed from history: requirement
and test counts by creation date, coverage from the creation of the trace
links and the results with their timestamps, open defects from their
creation and resolution dates (`source: "backfill"`; requirement and test
statuses and risks have no history and are left out). Snapshots are kept in
backups and deleted with their project.

`GET /projects/{pid}/metrics/trends?range=30|90|365` (default 30) or
`?since=YYYY-MM-DD` (at most 3660 days back) returns the snapshots of the
period:
```json
{
  "from": "2026-02-09", "to": "2026-03-10", "days": 30,
  "points": [{"day": "2026-03-10", "source": "recorded", "requirements": 10, "covered": 7,
              "coverage_pct": 70, "passed": 5, "tests": 20, "executed": 18, "pass_rate": 88.9,
              "open_defects": 3, "data": MetricData}],
  "defect_weeks": [{"week": "2026-03-09", "opened": 2, "closed": 1}]
}
```
Days without a snapshot are missing from `points`. `coverage_pct` is the
share of requirements with linked tests, `passed` the requirements whose
linked tests all passed and `pass_rate` the share of executed tests
(`executed`: last result not `not_run` / `in_progress`) whose last result
passed — the figures of the dashboard. `MetricData` holds the counts:
`requirements`, `covered`, `coverage` (per coverage state),
`requirement_status`, `tests`, `test_status`, `results` (tests per last
result), `executions` (results recorded that day), `open_defects` (per
severity) and `open_risks` (per level). `defect_weeks` counts the defects
created and resolved (closed, resolved or rejected) per week, Monday to
Sunday (UTC), `week` being the Monday.

**Flaky tests.** `GET /projects/{pid}/flaky-tests` lists the flaky tests
of the project, highest score first, then the latest flip:
```json
{"items": [{"test_id": "…", "key": "DEMO-TC-3", "title": "…", "execution_type": "automated",
            "score": 5, "window": 7, "last_flip": "2026-03-08T10:00:00Z",
            "results": [{"status": "passed", "at": "…", "run_id": "…", "execution_id": "…", "flip": false}]}],
 "window": 10, "min_flips": 2}
```
Only decisive results count — `passed` and `failed` of every run (manual
results and automated ones from JUnit imports, the `qtestframe report`
uploader and the Playwright reporter); blocked, skipped and not run are
left out. The score is the number of flips (a result that differs from the
previous one) among the test's last 10 decisive results (`results`, oldest
first; `window` is how many there are); a test with at least 2 flips is
flaky. Tests carry `flakiness` (the score, when above 0) and `flaky`, and
the test query fields `flaky` (`true`/`false`) and `flakiness` (alias
`flips`), e.g. `flaky:true` or `flakiness>=3`.

**Dashboard insights.** `GET /projects/{pid}/dashboard/insights` returns
the further key figures and breakdowns of the dashboard, all counted on the
server:
```json
{
  "requirements": {"total": 10, "with_tests": 7, "passing": 5, "by_status": {"approved": 6},
                   "review": {"none": 6, "pending": 2, "changes_requested": 1, "approved": 1}},
  "tests": {"total": 20, "by_type": {"manual": {"passed": 8, "not_run": 4}, "automated": {"passed": 6, "failed": 2}}},
  "risks": {"total": 6, "open": 4, "by_level": {"low": 0, "medium": 1, "high": 2, "critical": 1},
            "by_status": {"identified": 3, "mitigating": 1, "mitigated": 1, "accepted": 0, "closed": 1},
            "heat": [[0,0,0,0,0], [0,1,0,0,0], [0,0,0,1,0], [0,0,1,1,0], [0,0,0,0,1]],
            "mitigation": {"no_requirements": 1, "untested": 0, "partly_tested": 1, "tested": 1, "verified": 1}},
  "defects": {"open": 3, "buckets": {"lt7": 1, "7to30": 1, "30to90": 0, "gt90": 1},
              "by_severity": {"critical": {"lt7": 1, "7to30": 0, "30to90": 0, "gt90": 0}},
              "average_days": 41.3, "oldest_days": 112},
  "suspect": {"links": 2, "requirements": 1},
  "trace_rules": {"rules": 3, "failed": 1, "errors": 1, "warnings": 0, "violations": 4, "passed": false},
  "release": {"id": "…", "name": "1.0", "version": "1.0.0", "status": "active", "target_date": "2026-04-01",
              "runs": 2, "executions": {"passed": 12, "failed": 1, "not_run": 7}, "total": 20, "executed": 13, "passed": 12},
  "activity": {"from": "2026-02-09", "to": "2026-03-10", "days": [{"day": "2026-03-10", "changes": 4, "executions": 9, "defects": 1}],
               "total": {"day": "", "changes": 40, "executions": 120, "defects": 6}}
}
```
`passing` counts the requirements whose linked tests all passed
(coverage `passed`). `review` is the state of each requirement's latest
review request that was not cancelled (`none` without one). `heat[l-1][i-1]`
counts the risks that are not closed with likelihood `l` and impact `i`;
`by_level` and `mitigation` count the open risks (identified, mitigating;
the mitigation states are those of the risk report). Defect ages are whole
days since the defect was reported, for open and in-progress defects.
`trace_rules` evaluates the enabled [traceability rules](#traceability-rules).
`release` is the newest active release, else the planned release with the
earliest target date, else `null`; its executions are those of the runs
assigned to it. `activity` covers the last 30 days (UTC, today included):
`changes` are new revisions of requirements and tests, `executions` the
executions finished, `defects` the defects reported.

## Folders (groups) and categories

Reading needs `project.read`. Writing folders needs `requirement.write` or
`test.write` according to the folder's `kind`; writing categories needs
`project.update` or the write permission of the category's `scope`.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/folders?kind=requirement\|test` | flat list (client builds tree via `parent_id`) |
| POST | `/projects/{pid}/folders` | `{"kind","name","description","parent_id","color"}` |
| PATCH | `/folders/{id}` | `{"name","description","color"}` – `color` is `#rrggbb` or `""` (none). Items show the colour of their folder or of the nearest coloured parent folder; a test folder mirrored from a requirement folder takes its colour. |
| POST | `/folders/{id}/move` | `{"parent_id": id\|null, "position": n, "project_id"?: id}` – moving to another project moves contained items too. Cycles are rejected (409). |
| POST | `/projects/{pid}/folders/bulk` | `{"ids":[...], "action": "move\|delete", "parent_id"?, "project_id"?, "cascade"?}` – folders of one kind (write permission of that kind). Folders inside another selected folder go with it; a move into a selected folder is refused (409) before anything moves → `{"moved": n}` / `{"deleted": n}` |
| DELETE | `/folders/{id}?cascade=true` | without cascade children/items are moved to the parent |
| GET | `/projects/{pid}/categories?scope=` | list |
| POST | `/projects/{pid}/categories` | `{"scope","dimension","name","color","description"}` |
| PATCH | `/categories/{id}` | same fields |
| DELETE | `/categories/{id}` | |
| POST | `/projects/{pid}/categories/move` | `{"scope","dimension","new_dimension"}` – rename/move a whole category dimension |
| POST | `/projects/{pid}/categories/bulk` | `{"ids":[...], "action": "move\|color\|merge\|copy\|delete", "dimension"?, "color"?, "target_id"?}` – values of one scope, all or nothing → `{"changed": n}`. `move`: to `dimension` (a value with the same name there absorbs the moved one); `color`: `#rrggbb`; `merge`: into `target_id` (assignments move, the others are deleted); `copy`: to the other scope (existing ones skipped; needs edit rights there too) |

Default categories seeded for new projects (scope `test`): dimension
`Polarity` (Positive, Negative), `Interface` (External, Internal), `Medium`
(File based, Network based), `Execution` (Manual, Automated), `Level` (Unit,
Integration, System, Acceptance); scope `requirement`: `Area` (Core, UI,
Security, Operations).

## Requirements

Reading needs `requirement.read`, writing `requirement.write`;
`/reqlang/analyze` is open to any authenticated user.

**Review decisions.** Setting `status` to `approved` or `rejected` needs
`requirement.review` (tests: `ready` needs `test.review`), in PATCH, bulk
`set`, create and restore. A caller with the review permission but without
the write permission may PATCH or bulk-set `status` alone (the routes accept
`requirement.read` and check the rest; anything else → 403 naming the
missing permission). Decisions are audited as `requirement.review` /
`test.review` with `old → new` in the details.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/requirements?q=&folder_id=&recursive=true` | list `Requirement` (without criteria) incl. derived `folder_path`, `linked_tests`, `coverage`; `folder_id=root` (or `none`) lists unfiled items |
| POST | `/projects/{pid}/requirements` | Requirement fields (`title` required; `obligation` auto-detected from title + description when empty); `criteria` array may be supplied. `create_test` (bool, default = server setting `requirements.auto_create_test`, which defaults to `true`) also creates a linked draft test; the response then contains `linked_test` (TestCase) |
| GET | `/requirements/{id}` | includes `criteria` |
| PATCH | `/requirements/{id}` | partial update; `criteria` (if present) replaces all criteria; bumps `version` |
| DELETE | `/requirements/{id}` | 204; the response header `X-Trash-Id` names the [trash](#trash-undo-a-deletion) entry |
| POST | `/requirements/{id}/copy` | requirement.write – `{"title"?, "relation"?}` → 201 Requirement: a copy as `draft` titled *Copy of …* (or `title`) with fields, tags, categories and new criteria; `relation` (a relation type, e.g. `derived_from`) also relates the copy to the original |
| POST | `/projects/{pid}/requirements/move` | `{"ids":[...],"folder_id": id\|null, "position"?: n, "project_id"?: id}`; another `project_id` re-keys the items, remaps categories by dimension/name and drops their links ([design.md §5](design.md#5-move-semantics)) |
| POST | `/projects/{pid}/requirements/bulk` | `{"ids":[...], "set": {"status"?, "priority"?, "obligation"?, "add_category_ids"?, "remove_category_ids"?, "add_tags"?, "remove_tags"?}}` (tag removal ignores case) |
| DELETE | `/projects/{pid}/requirements/bulk` | `{"ids":[...]}` → `{"deleted": n, "trash_id"}` |
| POST | `/projects/{pid}/requirements/batch` | [batch update](#batch-update-spreadsheet-view): different fields per item, all or nothing |
| POST | `/projects/{pid}/requirements/review` | bulk review, see below |
| GET | `/requirements/{id}/links` | `[TraceLink]` with test keys |
| POST | `/reqlang/analyze` | `{"text", "language"?}` → `Analysis` (below). `language` is a pack code from `/meta` `analysis_languages` (`en`, `sv`); empty or `auto` detects it from the text; anything else → 400 |

`Analysis`:
```json
{
  "obligation": "must",
  "keywords": [{"keyword":"MUST","level":"must","offset":10,"length":4}],
  "issues": [{"rule":"ambiguous-term","severity":"warning","message":"...","offset":0,"length":4,"params":{"term":"fast"}}],
  "score": 85,
  "language": "en"
}
```

`message` is always English. `params` holds the values a client needs to
translate it by `rule`: `term` (the matched text), `keyword`, `keywords`,
`words`/`max` (`long-sentence`) and `examples` (`missing-keyword`, the
pack's typical keywords). `language` is the pack that was used.

### Import requirements

Requirements from customer files: CSV, XLSX, ReqIF and DOCX. Both routes
need `requirement.write` and take a `multipart/form-data` upload: field
`file` (at most 20 MB; at most 5,000 requirements per file, else 400) and

| Field | Meaning |
|---|---|
| `format` | `csv`, `xlsx`, `reqif` (also `reqifz`) or `docx`; default from the file extension (`.csv .tsv .txt .xlsx .xlsm .reqif .reqifz .docx`), else sniffed from the content. `.xls`/`.doc` → 400 |
| `sheet` | XLSX sheet name (case-insensitive); default: a sheet named *requirements*/*krav*, else the first sheet with a header row |
| `mapping` | JSON object `{"<column header>": "<field>"}` overriding the guessed field of a column (case-insensitive header; `"ignore"` or `""` ignores it). Fields: `title`, `description`, `external_id`, `obligation`, `type`, `priority`, `status`, `source`, `rationale`, `owner`, `tags`, `folder`, `criteria`. For ReqIF the headers are the attribute long names |
| `folder_id` | target requirement folder (root of the imported folder paths); default top level; unknown → 400 |

| Method | Path | Notes |
|---|---|---|
| POST | `/projects/{pid}/requirements/import/preview` | parses only, nothing is stored → `{format, sheet?, sheets?, columns:[{index, header, field}], candidates:[Candidate], warnings:[], errors:[], fields:[], total, duplicates, target_folder}` |
| POST | `/projects/{pid}/requirements/import` | 201 (200 with `dry_run`) `{created, skipped, folders_created, tests_created, dry_run, format, items:[{id, key, title, row, location, folder_id, test_id?, test_key?}], skipped_items:[{title, row, location, reason}], warnings, errors}`; audited as `requirement.import` |

Import options (form fields): `create_folders` (default `true`: create the
candidates' folder paths below `folder_id`, reusing folders with the same
name, case-insensitive; `false` puts everything in `folder_id`), `status`
(default `draft`; used for candidates without a status), `create_tests`
(default `false`; a linked draft test per requirement as with `create_test`,
needs `test.write`), `skip_duplicates` (default `true`), `dry_run` (default
`false`: everything is done in a transaction that is rolled back, so keys and
counts show what would happen) and `exclude` (JSON array of candidate indexes
of the preview to leave out). Importing a candidate with status `approved` or
`rejected` (from the file or `status`) needs `requirement.review` → else 403.
All requirements are stored in one transaction: if one fails validation
(→ 400 naming its `location`), none is stored. Linked tests are created
afterwards; if that fails, the imported requirements are deleted again.

`Candidate`: `{title, description, obligation, obligation_from, type,
priority, status, source, rationale, owner, tags, folder, criteria:[{description,
method, threshold}], external_id, row, location, warnings, duplicate?}`.
Enumerations are model codes or `""` (default on import). `obligation_from`
is `column`, `text` (detected with the requirement language analysis from title
and description, as on create) or `""`. `folder` is a path `A/B` relative to
the target folder. `row`/`location` point into the file (`row 5`,
`table 2, row 3`, `paragraph 12`, `SPEC-OBJECT …`). `errors` lists parts of
the file that were left out (e.g. a row without title or description);
problems that make the file unusable (no header row, broken XML/ZIP, unknown
mapping field) → 400.

**External ID and duplicates.** There is no external-ID column: the ID is
recorded in `source` as `ID: <id>` (appended as `<source>, ID: <id>` when the
file has a source). With `skip_duplicates` a candidate is skipped
(`reason` `duplicate_external_id`) when a requirement of the project has
`ID: <id>` in its source or an earlier candidate has the same ID, else
(`duplicate_title`) when a requirement or earlier candidate has the same
title, ignoring case. The preview marks these as `duplicate`.

**Presets for other tools' exports.** Form field `preset` (`doors` or
`polarion`; the preview lists them as `presets:[{id, name, mapping,
headings, heading_types}]`) applies a column mapping for the CSV / Excel
exports of IBM DOORS (`Absolute Number` → external ID, `Object Heading` →
title, `Object Text` → description, `Object Level` / `Object Number` for the
hierarchy …) or Siemens Polarion (`ID`, `Title`, `Description`, `Status`,
`Type`, `Priority`, `Severity` → obligation (MoSCoW), `Assignee` → owner,
`Document` → folder, `Outline Number` for the hierarchy …); `mapping` still
wins. With a preset, heading rows become folders of the rows below them:
DOORS rows with a heading and no text, Polarion rows of type *Heading*,
nested by `Object Level` or the outline number (`1.2.3`; Polarion's
`1.2-1` is the first item below heading 1.2).

**Formats.**

- **CSV / XLSX:** the header row is the first of the first 20 non-empty rows
  that has a known title or description column (or two recognised columns
  including one of them, or a column named in `mapping`); rows above it are
  ignored. CSV: delimiter `,` `;` or tab (sniffed, or Excel's `sep=` line);
  UTF-8 with or without BOM, UTF-16 with BOM, otherwise Windows-1252. The
  export's formula guard (a leading `'` before `= + - @`) is removed. XLSX:
  merged cells repeat their value over the merged range; a criteria sheet as
  in the export (`Requirement`, `Criterion`, `Method`, `Threshold`) adds
  criteria to the candidates with that ID.
- **Column names** (case-insensitive, decoration such as `(optional)` or `*`
  ignored), English · Swedish: title `Title, Name, Heading, Summary · Titel,
  Namn, Rubrik`; description `Description, Text, Requirement, Statement ·
  Beskrivning, Krav, Kravtext`; external_id `ID, Key, Ref, Reference, Number,
  Requirement ID · Nyckel, Referens, Nummer, Krav-ID`; obligation `Obligation,
  Level · Kravnivå, Nivå`; type `Type · Typ, Kravtyp`; priority `Priority ·
  Prioritet`; status `Status, State · Tillstånd`; source `Source, Origin ·
  Källa, Ursprung`; rationale `Rationale, Justification, Comment · Motivering,
  Kommentar`; owner `Owner, Responsible · Ägare, Ansvarig`; tags `Tags, Labels
  · Taggar, Etiketter` (split on `,` `;` `|`); folder `Folder, Section, Chapter,
  Path · Mapp, Avsnitt, Kapitel` (path split on `/`, `\` or `>`); criteria
  `Criteria, Acceptance criteria · Kriterier, Acceptanskriterier` (one per
  line, bullets removed; `method: text` or `[method] text` sets the method).
  The export columns `Key`, `Version`, `Linked tests`, `Coverage` and
  `Categories` are recognised (`Key` → external_id, the others ignored).
  Values: obligation `MUST`, `MUST NOT`, `SHALL`, `SHOULD`, `MAY`, `OPTIONAL`,
  `WILL`, `SKA`, `SKA INTE`, `MÅSTE`, `BÖR`, `FÅR`, `FÅR INTE` … or the model
  codes; type, priority, status and method in English or Swedish (`Hög`,
  `Godkänd`, `Säkerhet`, `Inspektion` …) or as codes; an unknown value is a
  candidate warning and the default applies.
- **ReqIF 1.x** (`.reqif`, `.reqifz` = ZIP of `.reqif` files): each
  SPEC-OBJECT is a candidate. STRING, XHTML (converted to text with line
  breaks; list items as `- `), ENUMERATION (long names) and other values are
  read; attributes map by long name: `ReqIF.Name`/title/name → title,
  `ReqIF.Text`/text/description → description, `ReqIF.ForeignID`/ID →
  external_id, others like column names. An object with a
  `ReqIF.ChapterName` and no text is a heading: its name becomes a folder of
  the objects below it in the SPECIFICATION hierarchy. With several
  specifications their long names are the top folders. Objects outside every
  hierarchy follow at the end.
- **DOCX:** paragraphs with a heading style (style name `heading N` in
  `styles.xml`, style IDs `Heading1`/`Rubrik1` …, or an outline level) set
  the folder path. Every other paragraph or list item with a normative
  keyword (MUST/SHALL/SHOULD/MAY/OPTIONAL and negations, SKA/BÖR/FÅR …,
  detected with `reqlang`) is a candidate: the paragraph is the description,
  the title its first eight words, and a leading `REQ-12:`, `[SRS-7]` or
  `K12 -` identifier the external ID. If the document uses uppercase English
  keywords anywhere, lowercase English keywords are not normative (RFC 8174).
  Numbered list items after a candidate ending with `:` are appended to it.
  Tables with a header row are parsed like CSV (vertically merged cells repeat
  the value above); tables without one are scanned like paragraphs. Tracked
  deletions and field codes are ignored.

### Bulk review

`POST /projects/{pid}/requirements/review` approves or rejects many
requirements, `POST /projects/{pid}/tests/review` marks many tests ready.
Needs `requirement.review` / `test.review` (write permission is not
needed).

```json
{"decision": "approved", "ids": ["…"], "query": "folder:Payments", "all": false,
 "from": ["review"], "ignore": ["no_obligation"], "comment": "…",
 "complete_reviews": true, "dry_run": true}
```

- `decision`: `approved` or `rejected` for requirements, `ready` for tests.
- The items: `ids`, else the matches of `query`, else every item of the
  project with `all: true` (one of them is required; at most 5000 items).
- `from`: the statuses an item may have (default `["review"]`).
- Items are skipped with a `reason`: `already`, `status` (not in `from`),
  `changes_requested` (the latest decided review request asked for
  changes), `pending_review` (a review request is open), `suspect` (suspect
  links; not checked when rejecting), `no_obligation` (approving a
  requirement without obligation), `no_steps` (manual test without steps),
  `no_script` (automated test without script). The last six can be listed
  in `ignore` to include such items anyway.
- `comment` is added as a comment to every changed item.
- `complete_reviews`: the caller's own open review requests no longer
  count as `pending_review`, and after the change those of the changed
  items are decided: `approved` for approve / ready, `changes_requested`
  for reject, with the comment "Approved in a bulk review." (or
  "Rejected …") plus `comment`. Checklists are left as they are; the
  requester is notified, and each is audited (`review.approved` /
  `review.changes_requested`) and sent as a `review.completed` webhook.
  Other reviewers' open requests still skip the item.
- `dry_run` returns the preview without changing anything.

Response: `{"decision", "dry_run", "updated": [{id, key, title, status}],
"skipped": [{id, key, title, status, reason}], "my_reviews",
"reviews_completed"}` (`status` is the status before; `my_reviews` counts
the caller's open review requests for the chosen items,
`reviews_completed` those completed, or in a dry run to be completed). Each change gets a revision (`bulk_update`), an audit entry
`requirement.review` / `test.review` and a `requirement.updated` /
`test.updated` webhook; the call is audited as `…bulk_review`.

## Custom fields and templates

Projects define their own fields for requirements, tests, defects, risks and
runs (`internal/customfield`, `internal/store/customfields.go`). Reading needs
`project.read`; changing a project's definitions, templates and default
templates `project.update`; the organisation's field sets and templates per
project type `settings.manage`.

### Definitions

| Method | Path | Body / notes |
|---|---|---|
| GET | `/projects/{pid}/custom-fields` | `?entity_type=requirement\|test\|defect\|risk\|run` → `[CustomField]` in order |
| POST | `/projects/{pid}/custom-fields` | `{entity_type, label, key?, type, unit?, options?, required?, default?, help?, position?, show_in_list?}` → 201 CustomField |
| GET | `/custom-fields/{id}` | one definition |
| GET | `/custom-fields/{id}/usage` | `{id, key, items}`: items with a value |
| PATCH | `/custom-fields/{id}` | label, type (only while no item has a value, else 409), unit, options, required, default, help, position, show_in_list; `key` and `entity_type` never change |
| DELETE | `/custom-fields/{id}?confirm=true` | without `confirm` → **409** `{code: "confirmation_required", usage}`; with it the definition is deleted, the values are removed from the items and kept in the field archive → `CustomFieldArchive` |
| GET | `/projects/{pid}/custom-fields/archive` | `?values=true` → `[CustomFieldArchive]` `{id, entity_type, field_key, label, definition, values?: [{id, key, value}], item_count, deleted_by, deleted_at}` |
| POST | `/projects/{pid}/custom-fields/apply-set` | `{project_type?}` (default the project's type) copies the organisation's field set; keys the project has are skipped → `{project_type, added, skipped}` (`"<entity>.<key>"`) |
| GET | `/field-sets` | `?project_type=` → the organisation's field sets `[CustomField]` with `project_type` |
| POST | `/field-sets/{type}/fields` | as a project field; new projects of the type get a copy when they are created (`POST /projects`) |
| PATCH / DELETE | `/field-sets/fields/{id}` | projects keep the copies they have |

* `type`: `text` (≤ 1000 characters), `long_text` (≤ 100 000), `number`
  (with optional `unit`), `date` (`YYYY-MM-DD`), `single_select`,
  `multi_select` (`options`, 1–200, case-insensitive on input, stored in the
  option's spelling and order), `checkbox`, `user` (the e-mail of a member of
  the organisation), `url` (absolute http(s)).
* `key`: `^[a-z][a-z0-9_]{0,39}$`, derived from the label when omitted
  ("Mass (kg)" → `mass_kg`), unique per project and item kind; at most 100
  definitions per item kind.

### Values

Items carry `custom_fields`, an object of key → value (string, number,
boolean or array of strings). It is stored as one JSON column per item table
(read with the item, no joins) and validated on every save against the
definitions:

* create: defaults apply to fields the request does not name; required
  fields must have a value (runs created without `custom_fields`, e.g. by a
  JUnit import, and imported requirements get the defaults without the
  required check);
* `PATCH` (and the batch endpoint of the spreadsheet view) **merges**
  `custom_fields`: named keys are set, `null` removes a value, other values
  stay. A required value cannot be removed; an item that never had it can
  still be saved;
* an unknown key, a value of the wrong type, an option that does not exist
  or a user who is not a member → 400. A stored value that no longer fits
  (an option was removed) is kept until it is changed.

Custom fields are in history snapshots (`changed_fields` lists
`custom_fields.<key>`), baselines and their comparison, restore (values of
fields the project still has), the trash, project archives and backups.

### Templates

| Method | Path | Body / notes |
|---|---|---|
| GET | `/projects/{pid}/templates` | `?kind=requirement\|test&lang=en\|sv` → `{items: [ItemTemplate], defaults: {requirement?, test?}}`: the project's templates, the organisation's for the project type and the built-in ones (`source`: `project`, `organisation`, `builtin`); `default` marks the project default |
| POST | `/projects/{pid}/templates` | `{kind, name, description?, content}` → 201 ItemTemplate; at most 100 per kind |
| PATCH / DELETE | `/templates/{id}` | `content` is replaced as a whole; deleting clears a default naming it |
| PUT | `/projects/{pid}/templates/default` | `{kind, template_id}` (`""` clears) → `{defaults}`; the template must be the project's, the organisation's for its type or a built-in one of its type |
| GET | `/template-sets` | `?project_type=&kind=&lang=` → the organisation's templates and the built-in ones |
| POST | `/template-sets/{type}/templates` | as a project template |
| PATCH / DELETE | `/template-sets/templates/{id}` | |

`content`: `{title, description, priority, status, tags, obligation, type,
source, rationale, criteria: [{description, method, threshold}],
preconditions, expected_result, execution_type, estimated_minutes, steps:
[{action, expected, data}], custom_fields}`; enumerations are checked,
requirement-only fields are dropped from test templates and the other way
round. Built-in templates (ids `builtin:<type>:<name>`, English and
Swedish): general (requirement with acceptance criteria, manual test),
software (user story, measurable non-functional requirement, API test),
hardware (design requirement with tolerance, IEC 60068 environmental test,
EMC test), system (interface requirement, integration test) and business
(business rule, acceptance test). The client applies a template to the new
item's form; the item is then created as usual.

## Test cases

Reading needs `test.read`, writing `test.write`.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/tests?q=&folder_id=&recursive=true` | list (`steps` omitted) incl. `linked_requirements`, `last_result` |
| POST | `/projects/{pid}/tests` | TestCase fields, `steps` array, optional `parameters` and `data_rows` (see [Test data](#test-data-data-driven-tests)) |
| GET | `/tests/{id}` | includes `steps`, `parameters` and `data_rows` |
| PATCH | `/tests/{id}` | partial; `steps`, `parameters` and `data_rows` (each if present) replace all |
| GET | `/tests/{id}/data` | `{"parameters":[...], "data_rows":[DataRow]}` |
| PUT | `/tests/{id}/data` | `{"parameters":[...], "data_rows":[DataRow]}` replaces the test data (a new version of the test; `test.write`) |
| DELETE | `/tests/{id}` | 204 with `X-Trash-Id` |
| POST | `/tests/{id}/copy` | duplicate → new TestCase (status `draft`) |
| POST | `/projects/{pid}/tests/move` | like requirements |
| POST | `/projects/{pid}/tests/bulk` | like requirements (`status`, `priority`, `execution_type`, categories) |
| DELETE | `/projects/{pid}/tests/bulk` | → `{"deleted": n, "trash_id"}` |
| POST | `/projects/{pid}/tests/batch` | [batch update](#batch-update-spreadsheet-view), like requirements |
| POST | `/projects/{pid}/tests/review` | bulk review: mark tests ready (see [Bulk review](#bulk-review)) |
| GET | `/tests/{id}/links` | `[TraceLink]` |
| GET | `/tests/{id}/dependencies` | `[TestDependency]` both directions: `direction` = `outgoing` (this test depends on the other) or `incoming`; `other_last_result` is the latest result of the other test |
| POST | `/tests/{id}/dependencies` | `{"depends_on_id"}` → 201 TestDependency; same project only; self-dependencies and cycles → 400, duplicates → 409 |
| DELETE | `/test-dependencies/{id}` | |
| GET | `/projects/{pid}/test-dependencies` | all dependencies of the project |
| GET | `/projects/{pid}/readiness?test_ids=a,b` | `{"tests":{test_id: Readiness},"settings":TenantSettings}` – whether tests may be executed (all tests when `test_ids` is omitted) |

`TestDependency`: `{id, project_id, test_id, depends_on_id, created_by,
created_at, test_key, test_title, depends_on_key, depends_on_title}`.
Test lists also carry `step_count` and `defects`; `relations` includes
`depends_on` references (with `kind: "test"`). Query field `depends_on`
(aliases `depends`, `dependency`, `prerequisite`) holds the keys of a
test's prerequisites.

### Test data (data-driven tests)

A test with `parameters` (names) and `data_rows` is executed once per row
(an *iteration*). `DataRow` is `{"name": "", "values": {"param": "value"}}`.
Parameter names start with a letter or `_` and contain letters, digits,
`_`, `.` and `-` (at most 50 characters, unique ignoring case, at most 50
parameters); at most 500 rows, row names up to 200 and values up to 2 000
characters. Values of unknown parameters are dropped, missing ones become
`""`; data rows need at least one parameter (else 400).

Step actions and expected results reference parameters as `{{name}}`
(spaces inside the braces allowed). When the test is added to a run, every
row becomes an iteration with its own step results whose texts have the
placeholders replaced; placeholders of unknown parameters are kept. Steps of
called tests (shared steps) are resolved with the parameters of the calling
test; the called test's own data rows are not used. Description,
preconditions, the overall expected result and step `data` are returned
unresolved with the embedded `test`; the client resolves them with the
iteration's `values`.

### Trash: undo a deletion

Deleting requirements or tests keeps a snapshot of everything the deletion
removed (criteria or steps, categories, trace links, relations,
dependencies, execution and step results, comments, history, attachments
with their files) for 30 days. The web client's undo uses it.

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/projects/{pid}/trash` | project.read (entries of kinds the caller cannot read are left out) | `[TrashEntry]` newest first: `{id, project_id, kind, item_ids, keys, label, deleted_by, created_at}` |
| POST | `/trash/{id}/restore` | the kind's write permission, plus its review permission when a restored item is `approved`/`rejected`/`ready` | puts the items back with their IDs and keys and removes the entry → TrashEntry; rows whose counterpart is gone (a test deleted since, a run, a category) are skipped and a missing folder puts the item at the root; 404 when unknown or already restored |

### Item links

Links from a requirement, test or run to another item of the same project
or to a web address. Reading needs the read permission of the item, adding
and removing the write permission of the item that carries the link (and
the read permission of an internal target's kind).

| Method | Path | Notes |
|---|---|---|
| GET | `/requirements/{id}/item-links`, `/tests/{id}/item-links`, `/runs/{id}/item-links` | `[ItemLink]`, outgoing first, then incoming (`direction`); links to kinds the caller cannot read are left out |
| POST | same paths | `{"target_type", "target_id"?, "url"?, "label"?}` → 201 ItemLink. `target_type`: `requirement`, `test`, `run`, `file` (a `document` or `report` file of the project's archive), `report` (a report definition) or `url` (`http(s)://` or `mailto:`, at most 2000 characters); label at most 200 characters; same project only; no self-links; duplicates → 409 |
| PATCH | `/item-links/{id}` | `{"label"}` |
| DELETE | `/item-links/{id}` | 204 |
| GET | `/projects/{pid}/item-links?source_type=requirement\|test\|run` | the outgoing links of the project's items (of one source type, or all), without links the caller cannot read |
| POST | `/projects/{pid}/item-links/bulk` | `{"links":[{source_type, source_id, target_type, target_id?, url?, label?}]}` (max 1000) → `{"created":[ItemLink], "skipped":[{source_type, source_id, target_type, target_id, url, reason, message?}]}`; reasons `exists`, `not_found` (the source is not in the project), `not_allowed` (no write permission for the source's kind or no read permission for the target's), `invalid` (e.g. a self-link or a bad address, with `message`) |
| DELETE | `/projects/{pid}/item-links/bulk` | `{"ids":[...]}` (max 1000) → `{"deleted":[ItemLink], "skipped":[{id, reason: "not_found"\|"not_allowed"}]}`; the deleted links are returned so they can be recreated |

`ItemLink`: `{id, project_id, source_type, source_id, target_type, target_id,
url, label, created_by, created_at, source_key, source_title, target_key,
target_title, target_kind?, direction, issue?}` (keys of runs, files and
reports are their names). Deleting an item removes its links (the trash
restores them); moving requirements or tests to another project drops their
internal links and keeps web links. A web link to an issue created from a
requirement (see [Issues from requirements](#issues-from-requirements))
carries `issue: {tracker, key, status, state: open|closed, synced_at,
passed_at?}`.

## Exchange: Excel round-trip and ReqIF import

Edit requirements or tests in Excel and import them back, or import ReqIF
documents from other tools again and again, with a preview of every change.
The preview compares the file with the stored items and stores nothing; the
apply call compares again and saves the confirmed rows **in one
transaction** (all or nothing) through the same create and PATCH handlers as
a normal save, so validation, permissions, history, audit, suspect links and
notifications are those of single saves.

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/projects/{pid}/exchange/xlsx?kind=requirement\|test&q=&folder_id=` | `export` + read of the kind | the round-trip workbook; audited as `project.export` |
| POST | `/projects/{pid}/exchange/xlsx/preview` | write of the kind | multipart `file` (+ `kind`) → diff |
| POST | `/projects/{pid}/exchange/xlsx/apply` | write of the kind | + `overwrite_conflicts`, `create` (default true), `exclude` (JSON array of row indexes) → result; audited as `requirement.import` / `test.import` |
| POST | `/projects/{pid}/exchange/reqif/preview` | `requirement.write` (+ `test.write` for test objects) | multipart `file` (.reqif / .reqifz), `mapping`, `test_mapping`, `object_types`, `relation_types`, `folder_id`, `test_folder_id`, `hierarchy`, `refines` → diff + mappings |
| POST | `/projects/{pid}/exchange/reqif/apply` | `requirement.write` (+ `test.write`) | + `overwrite_conflicts`, `create`, `exclude`, `exclude_tests` → result; audited as `requirement.import` |

**The workbook.** Sheet *Requirements* or *Tests*: `ID` (the item id: do
not change it), `Key`, `Version` (the version exported: the base of the
conflict check), then one column per field: requirements *Folder* (path
`A/B`), *Title*, *Description*, *Obligation*, *Type*, *Priority*, *Status*,
*Source*, *Rationale*, *Owner*, *Tags*, *Acceptance criteria* (one per line,
`method: text (threshold: x)`), one column per relation type (*Refines*,
*Depends on* … : the keys of the target requirements, comma separated) and
*Verified by* (read-only); tests *Folder*, *Title*, *Description*,
*Preconditions*, *Expected result*, *Execution type*, *Priority*, *Status*,
*Estimated minutes*, *Tags*, *Verifies* (requirement keys). Tests also have
a sheet *Steps*: `Test ID`, `Test` (key), `Step`, `Action`, `Data`,
`Expected result`, `Call test` (key of a called test). A hidden sheet
`_qtf` holds the kind, the project and the field key of every column;
enumerations have drop-down lists. Columns can be reordered or deleted (a
missing column changes nothing; unknown columns are listed in `ignored`);
headers are matched by key, English label or Swedish alias.

**Rows.** A row with an `ID` updates that item (an ID of another project or
of a deleted item is an error); without an ID, a `Key` of the project
updates that item; otherwise the row is new (a `Key` typed there is only a
reference for the Steps sheet). Missing folders are created. The steps of a
test listed on the Tests sheet are replaced by its rows on the Steps sheet
(by `Test ID`, or by the key of a new row), keeping the step ids by position.

**Diff.** `{kind, file, summary:{new, changed, unchanged, conflicts,
errors}, rows:[{index, row, location, status, id, key, ref, title,
base_version, version, changes:[{field, label, cell, old, new}],
errors:[{field, label, cell, message}], warnings}], columns:[{key, label,
type, values, read_only, relation}], ignored, warnings}`. `status` is
`new`, `changed`, `unchanged`, `conflict` (the stored version differs from
`Version` and the values differ) or `error` (a value that cannot be
imported: an unknown enumeration value, an unknown key, an empty title, a
review status without the review permission …). Only differing values are
written; `unchanged` rows are not saved.

**Result.** `{created, updated, unchanged, skipped, folders_created,
relations_added, relations_removed, links_added, links_removed,
items:[{index, location, id, key, title, action, version}],
skipped_rows:[{index, location, title, reason}]}` with `reason` `error`,
`conflict`, `excluded` or `new`. When an item was saved by someone else
between preview and apply, nothing is saved and the answer is 409 `stale`.

**ReqIF import.** ReqIF 1.0–1.2 (`.reqif`, or `.reqifz` with one or more
`.reqif` files), e.g. from IBM DOORS, DOORS Next, Siemens Polarion,
Codebeamer or Qtestframe's own export.

* *Object types.* Each `SPEC-OBJECT-TYPE` is mapped to `requirement`,
  `test`, `heading` (the objects are folders) or `ignore` by
  `object_types` (JSON: type long name → kind). The guess: the *Test case*
  type of Qtestframe's export and other types with *test* in the name are
  tests (without `test.write` they are left out, with a warning), *Verification
  record* is ignored, everything else is a requirement.
* *Attributes.* All `ATTRIBUTE-DEFINITION-*` types; XHTML becomes Markdown
  (paragraphs, line breaks, lists, headings, tables as GFM tables, code,
  bold, italics, links; `<object>` and `<img>` as images — the preview image
  inside an OLE object stands for it), enumerations their value names.
  `mapping` (requirement attributes) and `test_mapping` (attributes of test
  types) are JSON objects attribute long name → field key (custom fields as
  `cf:<key>`, matched by label like the built-in fields; enumerations map to
  select fields by option name), `heading` (the chapter name: the object is
  a folder), `external_id` (recorded as `ID: <id>` in the source of new
  requirements), `version`, `steps` (tests: an XHTML table Action | Data |
  Expected result | Calls, headers optional, or a list, one step per item)
  or `""` to ignore; guessed from the name (`ReqIF.Name`, `ReqIF.Text`,
  `ReqIF.ChapterName`, `ReqIF.ForeignID`, Polarion's `Title`,
  `Description`, `ID`, `Status`, `Severity`, `Assignee`, *Steps*,
  *Preconditions*, *Expected result* …).
* *Hierarchy.* With `hierarchy` (default true) heading objects (a chapter
  name and no text, an object type named *Heading*/*Chapter*, or a type
  mapped to `heading`) become folders below `folder_id` (requirements) or
  `test_folder_id` (tests), nested as in the `SPECIFICATION`; when several
  specifications hold items of one kind each is a top folder. With
  `refines` a requirement nested below another requirement (not a heading)
  refines it.
* *Relations.* `SPEC-RELATION`s between imported objects: between a test and
  a requirement (either direction) a trace link, between requirements a
  relation. `relation_types` (JSON: type long name → relation type,
  `verifies` or `ignore`) overrides the guess (`Refines`, *satisfies*,
  *parent* → `refines`; *derived* → `derived_from`; *depends*, *requires*
  → `depends_on`; *conflicts* → `conflicts_with`; *duplicates*;
  *provides*, *implements*; *verifies*, *validated by*, *tested by* →
  `verifies`; otherwise `relates_to`). Relations between two tests are left
  out.
* *Embedded files.* Files of a `.reqifz` that the XHTML references
  (`<object data="files/…">`, `<img src>`, links) become attachments of the
  item, and the Markdown points to them. A file the item has already (same
  name and content) is not added again.
* Objects without a title attribute get one from the first words of their
  text (tests also from their `LONG-NAME`) when they are created. A call
  step to a test that is new in the same import is imported as a plain step
  (import again to make it a call).

The preview adds `{tests (the tests' diff, or null), title, tool,
qtestframe, attributes, test_attributes:[{name, type, values, objects,
sample, object_types, field, guess}], targets, test_targets:[{key, label,
type}], object_types:[{name, objects, kind, guess}], relation_types:{name:
type}, relation_type_mapping:[{name, relations, type, guess}],
relation_targets, relations:{add, remove, keep, links, changes:[{ref, type,
file_type, source, target, action, kind}]}, folders, test_folders,
files:[{object, ref, name, size, existing}], skipped, errors, previous,
target_folder, test_target_folder, can_import_tests}`. The apply result
holds the totals of requirements and tests plus `tests` (the tests' share),
`references_saved` and `files_attached`.

**Re-import.** The import keeps the `IDENTIFIER` of every imported object,
relation and trace link and the version the import wrote (table
`external_refs`). Importing the document again updates those items
(`previous` counts them); an item edited here since the last import is a
`conflict` when the document's values differ. Relations and links imported
before that the document no longer has are removed; those made in
Qtestframe are kept. Items are never deleted. A document exported from the
same project (`IDENTIFIER` `qtf-req-<id>` / `qtf-test-<id>`, see *ReqIF
export*) matches its items directly, with its `Version` attribute as the
base version, so the ReqIF export round-trips through the import: export a
project with `include=tests`, import it into an empty project with the same
custom fields, export that again, and the documents hold the same items,
values, folders, relations and links (a test checks this).

**Custom fields and other extra fields** take part through the field
registry of `internal/interop` (`interop.Register`): an extension returns
fields with a stable `key`, a `label`, a type and functions to read a value
and write it into the create / PATCH body; they then appear as workbook
columns, as ReqIF mapping targets and in the diff.


### Import from Jira and Azure DevOps

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/projects/{pid}/import-sources` | project.read | `{jira: {url, user, jql, has_token}, azure_devops: {url, project, area_path, work_item_types, has_token}, jira_from_tracker, tracker_jira_url, tracker_jira_key}` |
| PUT | `/projects/{pid}/import-sources` | project.update | same fields plus `jira.token`, `azure_devops.token` (write-only; empty keeps the saved one) and `clear_tokens: {jira, azure_devops}` |
| POST | `/projects/{pid}/import-sources/{source}/check` | project.update | `source` `jira` or `azure-devops`; 200 or 502 `tracker_error` |
| POST | `/projects/{pid}/requirements/import/{source}/preview` | requirement.write | runs the query; the diff as for ReqIF plus `{source, query, truncated}` |
| POST | `/projects/{pid}/requirements/import/{source}/apply` | requirement.write | runs the query again and saves (one transaction) |

The body of preview and apply is JSON: `query` (JQL for Jira, WIQL for
Azure DevOps), or for Azure DevOps `area_path` and `work_item_types` (both
missing: the saved defaults), `mapping` (the tracker's field name →
requirement field, `cf.<key>` for a custom field, `""` to ignore),
`folder_id` (target of new requirements), `relations` (default true) and
`limit` (at most 1000); apply also `overwrite_conflicts`, `create` and
`exclude_refs` (issue keys or work item IDs to leave out). The items are
put into a ReqIF model and go through the ReqIF import: the same
`attributes` (the tracker's fields, with `guess` the default mapping),
`targets`, diff and relations. Each imported item is recorded as an
external reference (system `jira` with the issue key, or `azure-devops`
with the work item ID), so the next import updates the requirement; links
between imported items (Jira issue links and parents, Azure DevOps parent,
predecessor, duplicate and related links) become relations and are removed
when the link is gone. Jira descriptions (wiki markup) and Azure DevOps
HTML fields become Markdown; statuses and priorities are mapped by name
(and by the Jira status category), values without a match are left out
with a warning on the row. Without a Jira URL of its own the Jira import
uses the project's defect tracker when it is Jira. Tokens are encrypted
and never returned; tracker errors answer 502 `tracker_error`.
## Catalogue

A built-in library of standard requirements and tests for software
development (security, privacy, performance, reliability, operations,
usability, accessibility, common functionality, domain-specific functionality
such as checkout and billing), in English and Swedish.
The texts are written for Qtestframe and cite the standards they are based
on (OWASP ASVS, WCAG, ISO/IEC 25010, NIST SP 800-63B, ISO/IEC 27001, IEC 62443,
GDPR, CRA, DORA …) in `refs` by name
and section, without copying them. The content lives in
`internal/catalog/data` (format: `internal/catalog/FORMAT.md`) and ships
with the binary.

Items are organised in **groups** (e.g. Security) of **areas** (e.g.
Authentication). Requirement ids are `<PREFIX>-NN` (`AUTH-01`), test ids
`<PREFIX>-TNN` (`AUTH-T01`) and risk ids `<PREFIX>-RNN` (`AUTH-R01`); a
requirement lists the catalogue tests that verify it (`tests`) and the risks
it mitigates (`risks`), a test the requirements it verifies (`requirements`),
a risk the requirements that mitigate it (`mitigated_by`).
`lang` is `en` (default) or `sv`; a text missing in Swedish falls back to
English. List filters take comma-separated values. Every area belongs to a
**catalogue type** (`catalogue`: software, hardware, equipment, system,
communications, project, services, facilities, governance, documentation,
education); `catalogue=` filters by it. Every entry of `refs` resolves to
one **reference source** of the registry (`internal/catalog/data/sources.yaml`:
the laws, regulations, directives, standards, best practices and guidelines
cited, such as `gdpr`, `cra`, `nis2`, `cer`, `dora`, `red`,
`se-cybersakerhetslag`, `se-sakerhetsskydd`, `iso-27001`, `iec-62443-3-3`,
`owasp-asvs`, `wcag`); items list the ids in `sources`, and `source=`
selects the items that comply with them.

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/catalog?lang=` | project.read | `{languages, lang, groups:[{id, name, requirements, tests, risks, areas:[{id, prefix, group, name, description, requirements, tests, risks}]}], requirements, tests, risks, requirement_tags:[{tag, count}], test_tags:[…], risk_tags:[…], risk_categories:[{id, name}], catalogues:[{id, name, description, areas, requirements, tests, risks}]}` – groups in display order with item counts; `catalogues` are the catalogue types that have areas |
| GET | `/catalog/sources?lang=&catalogue=&all=` | project.read | `{lang, kinds:[{id, name}], sources:[{id, kind, jurisdiction?, short?, name, url?, requirements, tests, risks, catalogues:[id]}]}` – the reference sources grouped by kind (law, directive, standard, practice, guideline). `requirements` cite the source, `tests` cite it or verify such a requirement (what adding them brings), `risks` cite it, counted in the catalogue types of `catalogue`; sources nothing cites are left out unless `all=true` |
| GET | `/catalog/requirements?lang=&area=&group=&catalogue=&source=&q=&type=&obligation=&priority=&tag=&project_id=&not_in_project=` | requirement.read | `{"items":[{id, area, group, catalogue, type, obligation, priority, tags, refs, sources, tests, risks, title, description, in_project?}]}`. `source`: requirements whose refs cite one of the sources. `q`: all words must occur (title, id, tags, refs, description, rationale); ranked by relevance (exact id, whole phrase in the title, title, tags, refs, text), else catalogue order. With `project_id`, `in_project:[{id, key, title}]` lists the project's requirements with that `catalog_ref`; `not_in_project=true` leaves those out |
| GET | `/catalog/requirements/{id}?lang=&project_id=` | requirement.read | the summary plus `area_name, rationale, criteria:[{method, text, threshold?}], linked_tests:[{id, area, title, in_project?}], mitigates_risks:[{id, area, title, in_project?}]` |
| GET | `/catalog/tests?lang=&area=&group=&catalogue=&source=&q=&priority=&tag=&execution=&project_id=&not_in_project=` | test.read | `{"items":[{id, area, group, catalogue, priority, execution, tags, refs, sources, requirements, steps (count), title, description, in_project?}]}`; `source` matches tests that cite a source or verify a requirement citing it; `q` also searches the steps and the ids of the verified requirements |
| GET | `/catalog/tests/{id}?lang=&project_id=` | test.read | the summary plus `area_name, preconditions, expected, step_list:[{action, expected, data?}], linked_requirements:[{id, area, title, in_project?}]` |
| GET | `/catalog/risks?lang=&area=&group=&catalogue=&source=&q=&category=&level=&tag=&attack=&tactic=&project_id=&not_in_project=` | requirement.read | `{"items":[{id, area, group, catalogue, category, category_name, likelihood, impact, score, level, tags, refs, sources, mitigated_by, tests, attack, atlas, tactics, title, description, mitigation, in_project?}]}` – the typical risks of the areas: the threat or hazard and its consequence (`description`), a typical likelihood and impact on the project scale 1–5 with their level, and a suggested `mitigation`. `category`: security, privacy, safety, availability, data, compliance, supply-chain, ai, operational, quality, project, environmental; `level`: low, medium, high, critical; `source` matches risks that cite a source or are mitigated by a requirement citing it; `attack` (MITRE ATT&CK or ATLAS technique ids, comma-separated) matches risks that list one of them or one of its sub-techniques (`T1110` matches `T1110.004`), `tactic` (tactic ids such as `TA0006`, `AML.TA0005`) risks with a technique of one of the tactics; `attack`, `atlas` and `tactics` are the risk's ATT&CK techniques, ATLAS techniques and the tactics they serve; `q` also searches the mitigation, the mitigating requirement ids and the technique ids and names |
| GET | `/catalog/risks/{id}?lang=&project_id=` | requirement.read | the summary plus `area_name, ref_targets, techniques:[{id, name, matrix, tactics:[{id, name, url}], url}], mitigating_requirements:[{id, area, title, in_project?}], linked_tests:[…]`; `techniques` are those of `attack` and `atlas` with names (a sub-technique with its parent's: `Brute Force: Credential Stuffing`) and links to attack.mitre.org or atlas.mitre.org |
| GET | `/catalog/attack/techniques?q=&matrix=&tactic=` | project.read | `{matrices:[{id, name, version, url}], tactics:[{id, name, matrix, url}], items:[{id, name, matrix, tactics:[{id, name, url}], url}]}` – the MITRE ATT&CK techniques and sub-techniques of the Enterprise, ICS and Mobile matrices (no revoked or deprecated ids) and the MITRE ATLAS techniques: the valid values of a risk's `attack`. `q` matches the id or the name, `matrix` (`enterprise`, `ics`, `mobile`, `atlas`) and `tactic` narrow the list |
| GET | `/catalog/attack?catalogue=` | requirement.read | `{matrices:[{id, name, version, url, risks, tactics:[{id, name, url, risks, requirements, techniques:[{id, name, url, risks, requirements}]}]}]}` – the ATT&CK coverage of the catalogue: per matrix and tactic (in matrix order) the catalogue risks that list one of its techniques, the requirements that cite one in their refs or mitigate such a risk, and the techniques used (a technique counts its sub-techniques); `risks` of a matrix counts each risk once |
| GET | `/projects/{pid}/catalog` | project.read | `{"requirements": {"AUTH-01": [{id, key, title}]}, "tests": {…}, "risks": {…}}` – the project's items by `catalog_ref` (a kind the caller may not read stays empty) |
| POST | `/projects/{pid}/catalog/import` | project.read + see below | copies catalogue items into the project, see below |

**Origin.** Requirements, tests and risks have `catalog_ref`: the catalogue id they
were copied from, `""` otherwise (a risk also has the derived `catalog_area`;
query fields `catalog` and `catalog_area`, alias `area`, in the risk list). It is set only by the import, ignored in
create and update requests, and kept when an item is edited, restored from a
revision or baseline, copied, moved (also to another project), trashed and
restored, and in backups. The query field `catalog` filters by it:
`catalog:AUTH-01`, `catalog:AUTH-*`, `has:catalog`.

**Import.** `POST /projects/{pid}/catalog/import`

```json
{
  "requirement_ids": ["AUTH-01", "AUTH-02"],
  "test_ids": ["PERF-T03"],
  "lang": "sv",
  "requirement_folder_id": "…", "test_folder_id": "…",
  "folders": "area",
  "status": "draft", "test_status": "draft",
  "include_tests": true,
  "add_copies": false,
  "source": "Standard",
  "risk_ids": ["AUTH-R01"],
  "include_requirements": true,
  "include_risks": false,
  "risk_status": "identified"
}
```

- At most 500 items per request (chosen requirements, tests and risks plus
  the included ones), else 400. The request is atomic: on an error nothing is
  stored.
- `include_tests` (default `true` when the caller has `test.write`) also adds
  the catalogue tests that verify the chosen requirements.
- Requirements get title, description, obligation, type, priority, rationale,
  tags, the evaluation criteria (method, text, threshold) and `catalog_ref`;
  `source` is the cited standards joined with `; ` (or `Catalogue <id>`)
  unless the request names one. Tests get title, description,
  preconditions, expected result, steps (action, expected result, data),
  execution type, priority, tags and `catalog_ref`. No extra tag is added:
  `catalog_ref` (query field `catalog`) records the origin.
- `status` (requirements, default `draft`) and `test_status` (default
  `draft`); a review decision status needs `requirement.review` /
  `test.review`.
- Folders: the items go into `requirement_folder_id` / `test_folder_id`
  (`""` = top level). `folders: "area"` adds a folder per area below them,
  `"group"` a group folder with area folders, named in `lang`; folders of
  the same name are reused.
- **Risks:** `risk_ids` adds catalogue risks as project risks (title,
  description, mitigation, category in `lang`, the typical likelihood and
  impact, `risk_status` `identified` (default) or `mitigating`,
  `catalog_ref`). `include_requirements` (default `true`) adds the catalogue
  requirements that mitigate them, and with `include_tests` their tests and
  the tests listed on the risk; `include_risks` (default `false`) adds the
  risks the chosen requirements mitigate. Each new risk is linked
  (`risk_requirements`) to the project requirements of its mitigating
  catalogue requirements and each new requirement to the project risks it
  mitigates, new or added earlier (`risk_links` in the response). Risks need
  `requirement.write`.
- Items the project already has (same `catalog_ref`) are skipped with reason
  `exists` (and the `existing` items) unless `add_copies` is true; unknown
  ids are skipped with `not_found`.
- **Trace links:** each new requirement is linked to the project tests of its
  catalogue tests and each new test to the project requirements of the
  catalogue requirements it verifies, whether they were added now or earlier.
  Links need `test.write`; the requirements' evidence status is updated.
- The server setting `requirements.auto_create_test` does not apply: the
  catalogue tests take the place of the automatic draft tests.
- Permissions: `requirement.write` for requirements, `test.write` for tests
  (also for an explicit `include_tests: true`); otherwise 403.
- Revisions are recorded as for manual creation; the import is audited as
  `catalog.import`.

→ 201

```json
{
  "requirements": [{"id": "…", "key": "WEB-REQ-12", "title": "…", "catalog_ref": "AUTH-01", "folder_id": "…"}],
  "tests": [{"id": "…", "key": "WEB-TC-30", "title": "…", "catalog_ref": "AUTH-T01", "folder_id": "…"}],
  "risks": [{"id": "…", "key": "WEB-RISK-4", "title": "…", "catalog_ref": "AUTH-R01", "folder_id": null}],
  "links": [{"id": "…", "requirement_id": "…", "requirement_key": "WEB-REQ-12", "test_case_id": "…", "test_case_key": "WEB-TC-30"}],
  "risk_links": [{"risk_id": "…", "risk_key": "WEB-RISK-4", "requirement_id": "…", "requirement_key": "WEB-REQ-12"}],
  "skipped": [{"kind": "requirement", "catalog_ref": "AUTH-02", "reason": "exists", "existing": [{"id": "…", "key": "WEB-REQ-3", "title": "…"}]}],
  "folders_created": 2
}
```

To undo an import, delete the created items with the bulk delete routes
(risks, then requirements, then tests); restoring the trash entries in the
reverse order brings them back with their links.

## Requirement analytics

`GET /projects/{pid}/analytics?lang=` (requirement.read) analyses all
requirements of the project; `lang` is `auto` (default, per requirement) or
an analysis language. Test findings are included with `test.read`.

```json
{"summary": {"requirements": 20, "tests": 18, "with_findings": 3, "health": 85, "problems": 1, "warnings": 2, "infos": 1,
             "covered": 17, "coverage_percent": 85, "average_score": 91},
 "obligations": [{"value": "must", "count": 19, "percent": 95}, {"value": "shall", "count": 1, "percent": 5}],
 "families": [{"family": "mandatory", "total": 20, "dominant": "MUST", "keywords": [{"value": "MUST", "count": 19, "percent": 95}, …]}],
 "statuses": [{"value": "approved", "count": 12, "percent": 60}, …], "coverage": [{"value": "passed", …}],
 "languages": [{"value": "en", "count": 18, "percent": 90}, {"value": "sv", "count": 2, "percent": 10}],
 "findings": [{"code": "inconsistent_keyword", "severity": "warning", "category": "consistency",
               "params": {"family": "mandatory", "dominant": "MUST", "dominantPercent": "95", "keyword": "SHALL", "count": "1", …},
               "items": [{"kind": "requirement", "id": "…", "key": "SHOP-REQ-7", "title": "…", "note": "SHALL"}], "total": 1}]}
```

Findings carry a `code` and `params` for clients to phrase, and are ordered
problems, warnings, infos. Codes: `rfc_not_normative` (keywords only in lower case), `rfc_casing` (keywords also in lower or mixed case), `non_rfc_keyword` (WILL), `mixed_languages`, `mixed_language_text`, `english_keywords`, `obligation_missing`, `keyword_missing`,
`obligation_mismatch`, `multiple_levels`, `spread_all_mandatory`,
`spread_no_mandatory`, `spread_many_optional`, `spread_many_should`,
`spread_many_descriptive`, `inconsistent_keyword` (a keyword used where
another of the same meaning has ≥ 75 %), `mixed_keywords`,
`no_tests_mandatory`, `no_tests`, `tests_failing`, `tests_not_run`,
`no_criteria`, `tests_not_ready`, `orphan_tests`, `empty_description`,
`duplicate_titles`, `conflicts`, `low_quality`, `many_drafts`, `no_owner`,
`waived` (info: requirements without tests or with failing or unrun tests
that an approved, unexpired project-wide [waiver](#waivers-and-deviations)
accepts; they are left out of `no_tests*`, `tests_failing` and
`tests_not_run`, and `summary.waived` counts them) and
`language_rule` (with `params.rule`, a language analysis rule). At most 200
items per finding (`total` gives the full count).

### Compliance

`GET /projects/{pid}/analytics/compliance?source=&lang=` (requirement.read)
measures how far the project complies with reference sources of the
catalogue (`source`: ids from `GET /catalog/sources`, comma-separated;
without it every source the project has adopted requirements of or cites
itself). Per source:

- **Catalogue requirements** are the catalogue requirements that cite it.
  One is **adopted** when a project requirement was imported from it
  (`catalog_ref`) and is not rejected or obsolete, else **missing** (a gap).
  `obligations` splits them into must (MUST, MUST NOT), should and may.
- **Own requirements** are project requirements not imported from those
  items whose `source` field names the source (each part separated by `;`
  or a line break is matched like a ref, also without regard to case, e.g.
  `GDPR Art. 32; ISO/IEC 27001:2022 A.8.5`).
- **Fulfilment** of each counted requirement from the latest results of its
  linked tests: `passed` (it has tests and all passed), `failing` (one
  failed), `blocked` (one blocked, none failed) or `untested`; a
  requirement that has not passed but is accepted by a project-wide
  [waiver](#waivers-and-deviations) in force is `waived` (counted in
  `fulfilment.waived`, not as fulfilled). Several
  project copies of one catalogue item count as one, with the worst state.
- `percent` = fulfilled / (catalogue + own requirements);
  `adoption_percent` = adopted / catalogue; `mandatory_percent` the
  compliance of the must group (-1 without one); `clauses` give the same
  per clause or article group, derived from the refs (`Art. 32`, `A.8`,
  `Annex I Part I`, `SR 1`; `""` for refs without a clause).

```json
{"summary": {"sources": 1, "catalogue": 34, "adopted": 12, "missing": 22, "own": 1,
             "fulfilment": {"passed": 8, "failing": 1, "blocked": 0, "untested": 4}, "percent": 23},
 "sources": [{"id": "cra", "kind": "law", "jurisdiction": "EU", "short": "CRA", "name": "Cyber Resilience Act, Regulation (EU) 2024/2847",
              "catalogue": 34, "adopted": 12, "missing": 22, "own": 1, "fulfilment": {…}, "adoption_percent": 35, "percent": 23, "mandatory_percent": 25,
              "obligations": [{"obligation": "must", "catalogue": 30, "adopted": 11, "missing": 19, "own": 1, "fulfilled": 8, "percent": 26}],
              "clauses": [{"clause": "Annex I Part I", "catalogue": 11, "adopted": 5, "missing": 6, "own": 0, "fulfilment": {…}, "percent": 36}],
              "gaps": [{"catalog_id": "VULN-03", "area": "vulnerability-handling", "title": "…", "obligation": "must", "priority": "high",
                        "clauses": ["Annex I Part II"], "refs": ["…"], "tests": 1, "inactive": [{"kind": "requirement", "id": "…", "key": "…", "title": "…", "note": "rejected"}]}],
              "items": [{"id": "…", "key": "WEB-REQ-4", "title": "…", "obligation": "must", "state": "failing", "catalog_ref": "SUP-02", "own": false, "clauses": ["Annex I Part I"]}],
              "gaps_total": 22, "items_total": 13}],
 "unknown": ["no-such-source"]}
```

Gaps come MUST first, then by priority; items not fulfilled first; at most
200 of each (`gaps_total`, `items_total`). The standard report `compliance`
presents the same, with a section of the project's waivers and deviations.

### Statement of Applicability

A Statement of Applicability (SoA) lists every control or clause of a
reference source with the project's decision whether it applies, the
justification, the owner and the implementation status. ISO/IEC 27001:2022
Annex A ships its 93 control identifiers and short names (A.5.1–A.5.37,
A.6.1–A.6.8, A.7.1–A.7.14, A.8.1–A.8.34; no normative text); for every other
source the controls are the clause groups of the catalogue refs that cite it
and of the project's own requirements' `source` fields.

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/projects/{pid}/soa/sources?lang=` | requirement.read | `[{id, kind, short, name, title, shipped, controls}]`, sources with a shipped list first |
| GET | `/projects/{pid}/soa?source=&lang=` | requirement.read | the statement (`source` default `iso-27001`; 400 unknown) |
| PUT | `/projects/{pid}/soa/decisions` | requirement.write | `{"source", "decisions": [{"control_id", "applicable": true\|false\|null, "justification", "owner"}]}` → `{changed, statement}`; only the controls given change; an excluded control needs a justification |

```json
{"source": {"id": "iso-27001", "title": "ISO/IEC 27001:2022 Annex A", "shipped": true, "controls": 93, …},
 "summary": {"controls": 93, "applicable": 60, "excluded": 5, "undecided": 28, "implemented": 20, "partial": 15, "planned": 10, "not_implemented": 15},
 "rows": [{"id": "A.8.5", "title": "Secure authentication", "group": "A.8", "group_title": "Technological controls",
           "applicable": true, "justification": "Users sign in", "owner": "CISO", "updated_by": "…", "updated_at": "…",
           "status": "implemented", "requirements": [{"id": "…", "key": "WEB-REQ-3", "title": "…", "state": "passed", "catalog_ref": "AUTH-01", "waiver": ""}],
           "evidence": [{"id": "…", "key": "WEB-TC-4", "title": "…", "result": "passed"}], "catalogue": 12, "missing": 9, "suggested": false}]}
```

A control's requirements are the project requirements (not rejected or
obsolete) imported from a catalogue requirement whose refs cite the control
(`ISO/IEC 27001:2022 A.8.5`) or whose own `source` cites it. `status`:
`implemented` (all of them passed their tests), `partial` (some passed or
are accepted by a project-wide waiver), `planned` (requirements adopted, none
passed yet), `not_implemented` (none); `""` for an excluded control.
`catalogue`/`missing` count the catalogue requirements citing the control and
those not adopted. `suggested` marks undecided controls the project already
has requirements for. The decisions are stored per project and source
(table `soa_decisions`). The document is the standard report `soa`
(PDF, DOCX, XLSX, CSV, Markdown; `archive: true` saves it to the document
archive).

### Consistency checks

`GET /projects/{pid}/analytics/consistency?limit=` (project.read)
evaluates every [built-in consistency check](#built-in-consistency-checks)
on the project: with the project's own rule where the check is installed,
else with its default (rule id `builtin:<kind>`; such a default is shown
but does not count for releases or the dashboard). → TraceRulesReport plus
`"checks":[{kind, scope, name, description, severity, enabled, installed, rule?}]`
and `"installed": n`.

### Quality metrics

`GET /projects/{pid}/analytics/quality?range=&period=&stale_days=&lang=`
(project.read) computes quality metrics from the project's history.
`range` is 30, 90 (default), 180 or 365 days; `period` week or month
(default week up to 120 days, else month); `stale_days` (default 30) the
age from which a latest result is stale; `lang` the requirement analysis
language (`auto` by default).

| Section | What it measures |
|---|---|
| `volatility` | revisions that changed a requirement after its creation (moves not counted), in the range: `changes`, `changed` (requirements), `per_requirement` (changes per requirement per period), `trend`, `most_changed` |
| `defect_density` | defects per active requirement; a defect counts for a requirement when linked to it, to a test that verifies it or to an execution of such a test: `per_requirement`, `by_severity`, `by_requirement`, `by_folder`, `by_category` (`[{name, requirements, defects, density}]`) |
| `escaped` | defects found in production (`found_in`) or created after a released release whose scope they concern (its requirements, their tests, its runs): `escaped`, `percent`, `production`, `after_release`, `by_phase`, `by_release`, `trend` (share per period), `items` |
| `effectiveness` | defects linked to a test or its executions, created in the range: `per_test` (per test executed in the range), `per_hour` (per execution hour; `untimed_percent` executions without a duration), `top_tests`, `trend` |
| `result_age` | age of the latest result of each test (deprecated tests excluded): `buckets` (0-7, 8-30, 31-90, 91-180, 181+ days), `never`, `stale`, `stale_percent` (of the tests with a result), `median_days`, `oldest` |
| `review_turnaround` | hours from a review request to its decision (decided in the range, cancelled not counted): `mean_hours`, `median_hours`, `p90_hours`, `pending`, `oldest_pending_days`, `trend` (median per period) |
| `fix_time` | hours from a defect's creation to its resolution (resolved in the range, rejected not counted): `mean_hours`, `median_hours`, `p90_hours`, `open`, `by_severity`, `trend` (mean per period) |
| `requirement_quality` | active requirements whose text has warnings or errors of the requirement analysis: `percent`, `by_rule`, `items`, `trend` (replayed from the revisions: the text each requirement had at the end of the period) |

Trends are `[{period, count, total, value}]`, one per period (`periods`
lists their first days; weeks start on Monday, UTC). The standard report
`quality_metrics` (params `range`, `stale_days`) presents the same.

## Traceability

Reading needs `project.read`; creating and deleting links needs
`test.write`.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/links` | all links |
| POST | `/projects/{pid}/links` | `{"requirement_id","test_case_id","criterion_id"?}` |
| POST | `/projects/{pid}/links/bulk` | `{"links":[{requirement_id, test_case_id, criterion_id?}]}` (max 1000) → `{"created":[TraceLink], "skipped":[{requirement_id, test_case_id, reason}], "status_changes"}`; reasons `exists`, `not_found` (not in the project), `invalid` (e.g. a criterion of another requirement) |
| DELETE | `/projects/{pid}/links/bulk` | `{"ids":[...]}` (max 1000) → `{"deleted":[TraceLink], "skipped":[{id, reason: "not_found"}], "status_changes"}`; the deleted links are returned so they can be recreated |
| POST | `/projects/{pid}/links/confirm` | `{"ids":[...]}` → `{"confirmed": n, "status_changes"}`; confirms the suspect ones among the project's links |
| DELETE | `/links/{id}` | |
| GET | `/projects/{pid}/traceability?q=&tq=&configuration=` | `{"requirements":[{id,key,title,obligation,coverage}], "tests":[{id,key,title,last_result}], "links":[TraceLink]}`; `q` filters requirements, `tq` tests; `configuration=<id>` (or `none` for results without configuration) gives coverage and last results in that configuration only (400 for an unknown configuration) |
| GET | `/requirements/{id}/coverage` | coverage by configuration (`requirement.read`): `{"requirement_id", "coverage", "configurations":[{id, name, coverage}], "tests":[{id, key, title, results: {configuration_id: result}, result}]}`; `id` `""` stands for results without configuration; `results` holds the latest result of each linked test in every configuration it is planned in (`""` when planned but not run yet); `result` combines them (failed, blocked, passed in all, else in_progress/skipped/not_run) |

**Coverage with configurations.** Every configuration a linked test is
planned in (it has an execution there in any run of the project; an
execution without configuration counts as one more configuration) counts as
one result: the latest result other than `not_run` in that configuration.
The requirement's `coverage` applies the usual rules to all these results:
`failed` if one failed or is blocked, `passed` if all passed, `partial` if
some passed, else `not_run` (`none` without links). A requirement is thus
passed only when its tests passed in every configuration they are planned
in. Without configurations this is the latest result of each test, as
before. The coverage *in one configuration* counts only the linked tests
planned in it (`not_run` when none is). Releases count a test as passed only
when it passed in every configuration of the release's runs.

### Traceability rules

Rules say that the items matching a selector must have at least `min`
traces of a kind. Reading and evaluating needs `project.read` (the
per-item routes `requirement.read` / `test.read`); creating, changing and
deleting rules needs `project.update`.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/trace-rules` | `[TraceRule]` in their order |
| POST | `/projects/{pid}/trace-rules` | TraceRule fields (below) → 201 TraceRule; a new rule is enabled unless `enabled: false`; at most 200 rules per project |
| GET / PATCH / DELETE | `/trace-rules/{id}` | PATCH changes the fields given |
| GET | `/projects/{pid}/trace-rules/evaluate?all=&rule_id=&limit=` | evaluates the enabled rules (`all=1`: also the disabled ones; `rule_id`: one rule) → `{"results":[TraceRuleResult], "passed", "summary":{rules, passed, failed, errors, warnings, violations}}`; `passed` is false when an enabled rule of severity `error` fails. `limit` bounds the violations listed per rule (default 500), `violation_count` is always the full number |
| POST | `/projects/{pid}/trace-rules/preview?limit=` | evaluates an unsaved rule (the body of a POST) → TraceRuleResult |
| GET | `/requirements/{id}/trace-rules`, `/tests/{id}/trace-rules` | the enabled rules the item breaks → `{"items":[{rule, kind, id, key, title, found, required, total}]}` |
| GET | `/projects/{pid}/trace-rules/builtins` | the built-in checks with their defaults → `{"items":[{kind, scope, name, description, severity, enabled, installed, rule?}]}` |
| POST | `/projects/{pid}/trace-rules/builtins` | `{"kinds"?:[kind]}` (body optional) creates the built-in checks the project lacks with their defaults (enabled, severity `warning`) → 201 `{"items":[TraceRule]}`; a name taken by another rule gets a number |
| GET | `/projects/{pid}/consistency/dismissals` | `{"items":[{id, check, item_a, item_b, created_by, created_at}]}` |
| POST / DELETE | `/projects/{pid}/consistency/dismissals` | `{"check","item_id","other_id"}` dismisses (201) or restores (204) a finding: `duplicate_suspect` (not a duplicate; both requirements of the project), `unmet_dependency` / `broken_prerequisite` (a dependency on a deleted item that has been dealt with). Needs `requirement.write` (`test.write` for `broken_prerequisite`) |

TraceRule: `{"id","project_id","name","description","severity":"error|warning","enabled","scope":"requirement|test","selector":"<query>","kind","min","not_suspect"?,"passed"?,"relation_type"?,"direction"?,"target_type"?,"target_query"?,"every"?,"position","created_by","created_at","updated_at"}`.
`selector` is a [query](#query-language) on the items of `scope` (empty: all).
Options that do not apply to the kind are cleared.

| `kind` | Applies to | Counts | Options |
|---|---|---|---|
| `verified_by` | requirements | its trace links to tests | `not_suspect`, `passed` (the test's last result is passed), `target_query` (on the test) |
| `verifies` | tests | its trace links to requirements | `not_suspect`, `target_query` (on the requirement) |
| `relation` | requirements | its relations to other requirements | `relation_type` (empty: any), `direction` (`incoming`: the other requirement is the source, e.g. `refines` + `incoming` = refined by child requirements; `outgoing`; empty: both), `target_query` |
| `item_link` | requirements, tests | its item links | `target_type` (`requirement`, `test`, `run`, `file`, `report`, `url`; empty: any), `direction`, `target_query` (needs `target_type` requirement or test) |

`min` is 1 to 1000 (default 1). With `every: true` an item also breaks the
rule when one of its traces (of the relation type, direction and target
type) does not meet the conditions; `min` may then be 0, e.g. "no suspect
link": `{"kind":"verified_by","not_suspect":true,"every":true,"min":0}`.
An invalid selector or target query is a 400 `invalid_query` whose message
starts with `selector:` or `target_query:`.

TraceRuleResult: `{"rule":TraceRule, "matched": n, "violations":[{kind, id, key, title, status?, found, required, total, check?, note?, related?, fixes?}], "violation_count": n, "passed", "error"?, "waived"?}`;
`found` counts the qualifying traces, `total` the traces before the
conditions (`not_suspect`, `passed`, `target_query`). The **Traceability**
report lists the enabled rules and their violations (sections
`trace_rules` and `trace_rule_violations`, evaluated on the report's
requirements and tests).

#### Built-in consistency checks

Besides the kinds above, a rule's `kind` can be a built-in check with a
fixed condition. They take a `selector` (not `unmitigated_risk`), no `min`
and no options except `threshold` and `level`; the scope is fixed.

| `kind` | Applies to | Violation when |
|---|---|---|
| `conflict_in_release` | requirements | it and a requirement it `conflicts_with` are both approved, implemented or verified and in the same release (not cancelled) |
| `unmet_dependency` | requirements | it is approved (or later) and `depends_on` a requirement that is rejected, obsolete or deleted (in the trash) |
| `obsolete_parent` | requirements | it is not rejected or obsolete and `refines` or is `derived_from` an obsolete requirement |
| `verifies_inactive` | tests | it verifies a rejected or obsolete requirement |
| `broken_prerequisite` | tests | a prerequisite is deleted, its latest result is failed, or prerequisites form a cycle |
| `unevidenced_must` | requirements | it is MUST / MUST NOT / SHALL / SHALL NOT, in a planned or active release and has no passing evidence (a linked test that is not suspect passed its latest execution) |
| `unmitigated_risk` | risks | a risk of at least `level` (default `high`) that is not closed or accepted has no mitigation requirement, or not every one of them is verified (a risk whose mitigations are all verified counts as mitigated); in a release scope the risks of its requirements count |
| `unjustified_rejection` | requirements | it was adopted from a regulation of the catalogue (its catalogue item cites a law or directive) and is rejected without a justification: no comment and no rationale of its own |
| `duplicate_suspect` | requirements | its normalised title and description are at least `threshold` (0.5–1, default 0.9) similar to an older active requirement's (Dice coefficient of character trigrams) and neither a `duplicates` relation nor a dismissal links them |

Violations of built-in checks add `check` (the reason: `conflict`,
`rejected`, `obsolete`, `deleted`, `failing`, `cycle`, `no_evidence`,
`no_mitigation`, `not_verified`, `unjustified`, `similar`), `note`,
`related` (the other items: `{kind, id, key, title, status, role, link?, check?, note?}`,
where `kind` may be `release` or `risk` and `link` is the id of the
relation, trace link or prerequisite) and `fixes`, the actions that resolve
them with the ordinary endpoints: `remove_relation`, `remove_link`,
`remove_dependency`, `remove_from_release`, `set_obsolete`,
`add_justification`, `link_mitigation`, `mark_duplicate` (a `duplicates`
relation from the item to the related one) and `dismiss`.

### Verification (criteria, records, roll-up, system elements)

A requirement counts as **verified** only when every one of its evaluation
criteria has passing evidence, and — along the `refines` hierarchy — every
active child requirement is verified too. Evidence is

* a **test**: a trace link, optionally mapped to one or more criteria, each
  optionally through one **step** of the test (`criteria` of a TraceLink:
  `[{"criterion_id","step_id"?}]`; `criterion_id` stays the first of them
  for older clients). The test counts as passed when its last result passed
  (with a step: that step's result in the same execution), failed when it
  failed or was blocked, else pending; a suspect link is pending;
* a **verification record**: an inspection, analysis, demonstration or
  review (or a test done outside the tool) of a criterion, or of the
  requirement as a whole (`criterion_id` null). Its result `pass` or
  `conditional` passes, `fail` fails, while it is valid: not after
  `valid_until`, and not after it was invalidated (`until_change`: the
  requirement's content changed; or its criterion was removed). Per
  criterion only the newest valid record counts (the others are
  `superseded`).

**Backwards compatibility.** A test link without criterion mapping (and a
record without criterion) is *plain* evidence: it counts for every
criterion that has no evidence of its own, so existing projects verify as
before. With the project setting **strict criterion coverage** each
criterion needs its own evidence. A failing plain test fails the
requirement in both modes.

**Roll-up.** `refines` (source refines target: the source is a child of
the target) is the decomposition. A parent is verified only when its own
state is verified (or it has no criteria and no evidence of its own) and
all its children that are not rejected or obsolete are. "Status follows
evidence" applies this to the status (implemented ↔ verified) of the
requirement and of every requirement above it, so with the default
settings an unverified child under a verified parent cannot persist; with
manual verification it is reported as an inconsistency (`inconsistencies`,
the analytics finding `verified_parent_unverified_child`).

States (`verification` of a requirement, `state` of a criterion):
`verified`, `partial` (some verified), `failed`, `pending` (evidence, none
passed yet), `none` (no evidence).

| Method | Path | Notes |
|---|---|---|
| GET | `/requirements/{id}/verification` | RequirementVerification: is it verified, and why (`requirement.read`) |
| GET | `/projects/{pid}/verification?q=&folder_id=` | `{"strict","counts":{state: n},"criteria_verified","criteria_total","requirements":[RequirementVerification],"inconsistent":[…]}` over the requirements matching `q` (counts leave out rejected and obsolete ones) |
| GET / PUT | `/projects/{pid}/verification-settings` | `{"strict_criteria", "updated_by", "updated_at"}`; PUT needs `project.update` and lets the statuses follow the evidence again |
| PUT | `/links/{id}/criteria` | `{"criteria":[{"criterion_id","step_id"?}]}` → TraceLink (`test.write`); `[]` makes it a plain link; 400 for a criterion of another requirement or a step of another test |
| GET | `/projects/{pid}/verification-records?requirement_id=` | `[VerificationRecord]`, newest first |
| GET / POST | `/requirements/{id}/verification-records` | POST (`run.execute`): `{"criterion_id"?, "method"? (default the criterion's), "result", "performed_by"? (default the author's name), "performed_on"? (YYYY-MM-DD, default today), "note"?, "file_ids"? (documents of the project's archive), "valid_until"?, "until_change"?}` → 201 |
| GET / PATCH / DELETE | `/verification-records/{id}` | PATCH and DELETE need `run.execute`; a change of method, result, criterion, dates or performer removes the sign-off |
| POST / DELETE | `/verification-records/{id}/signoff` | `{"comment"?}`: sign-off by a second person (`requirement.review` or `run.execute`; 409 for the author or the performer); DELETE removes it |
| GET / POST | `/projects/{pid}/elements` | system elements in tree order with `path`, `requirement_ids`, `test_ids` (direct allocations); POST (`requirement.write`) `{"name","kind"?,"parent_id"?,"description"?}`; names are unique among siblings (409), `kind` one of `system, subsystem, component, product, supplier, other` |
| GET / PATCH / DELETE | `/elements/{id}` | PATCH (`requirement.write`): `name, kind, description, parent_id` (move; not below itself), `position`; DELETE removes the elements below it and their allocations |
| POST | `/elements/{id}/allocations` | `{"kind":"requirement|test","add":[ids],"remove":[ids]}` (`requirement.write` or `test.write` for the kind) → SystemElement |
| PUT | `/requirements/{id}/elements`, `/tests/{id}/elements` | `{"element_ids":[…]}` replaces the item's allocation → `{"element_ids"}` |
| GET | `/projects/{pid}/elements/coverage?q=` | `[ElementCoverage]` per element: requirements allocated to it or below it (of those matching `q`; not rejected or obsolete), `covered` (any evidence), `verified`, `failed`, `percent`, and `tests` / `tests_passed` |

RequirementVerification: `{"requirement_id","key","title","status","strict","own","state","verified","criteria":[{criterion_id, position, description, method, state, evidence:[Evidence]}],"evidence":[Evidence],"criteria_verified","criteria_total","children":[{id,key,title,status,state}],"children_verified","parent_ids","reasons":[Reason],"inconsistencies":[Reason]}`;
`own` is the state from its own criteria and evidence, `state` includes the children.
Evidence: `{"kind":"test|record","id","link_id"?,"key","title","step_id"?,"step_position"?,"result","state":"passed|failed|pending|void","plain"?,"note"?,"date"?}`
(`note`: `suspect`, `not_run`, `step_not_run`, `expired`, `invalidated`, `superseded`, `conditional`).
Reason: `{"code","params","text"}` with the codes `no_evidence`, `criterion_missing`, `criterion_failed`, `criterion_pending`, `evidence_failed`, `evidence_pending`, `child_not_verified` and, for inconsistencies, `status_inconsistent`, `child_status_unverified`.
VerificationRecord: `{"id","project_id","requirement_id","requirement_key","criterion_id","criterion_text","method","result","performed_by","performed_on","note","file_ids","files":[{id,name,kind}],"valid_until","until_change","invalidated_at","invalidated_reason","validity":"valid|expired|invalidated","signoff":{by,name,comment,at}|null,"created_by","created_at","updated_at"}`.

Requirements carry `verification` (rolled-up state), `criteria_verified`
and `element_ids`; tests `element_ids`. The traceability matrix
(`/projects/{pid}/traceability`) adds `verification`, `criteria`
(`[{id, position, description, method, state, records}]`), `records` and
`parent_ids` per requirement, and its links carry `criteria`. The
**Verification** report (`template: "verification"`, `query` filters the
requirements) and the requirements specification (a Verification field,
a column in the criteria table and in the traceability matrix; not for a
baseline) show the same. Analytics adds `verification` counts and the
findings `criteria_no_evidence`, `verified_parent_unverified_child` and
`status_ahead_of_evidence`.

## Operations (runs and executions)

Permissions: reading `run.read`; creating, updating, deleting runs and
adding tests `run.write`; recording results (`PATCH /executions/{id}`, CI
import) `run.execute`.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/runs?q=` | list `TestRun` with `summary` and `configurations` (`[{id, name}]`); query field `configuration` (alias `config`) |
| POST | `/projects/{pid}/runs` | `{"name","description","environment","build","test_ids":[...],"folder_ids":[...],"query":"<filter expr>","configuration_ids":[...]}` – union of explicit tests, tests in folders (recursive) and tests matching query; with `configuration_ids` (configurations of the project, at most 50) every test is executed once per configuration |
| GET | `/runs/{id}` | `TestRun` |
| PATCH | `/runs/{id}` | `{"name","description","status","environment","build","release_id","configuration_ids"}` (`configuration_ids` as for `PUT /runs/{id}/configurations`; all or nothing). A run also changes status by itself: the first result moves a `planned` run to `in_progress`; when every execution has a final result (`passed`, `failed`, `blocked`, `skipped`) a `planned`/`in_progress` run becomes `completed`; taking a result back to `not_run`/`in_progress`, or adding tests, reopens a `completed` run. `aborted` runs are never changed automatically. |
| DELETE | `/runs/{id}` | |
| POST | `/projects/{pid}/runs/bulk` | `{"ids":[...], "set": {"status"?, "release_id"? ("" removes), "environment"?, "build"?}}` → `{"updated": n}` (run.write) |
| DELETE | `/projects/{pid}/runs/bulk` | `{"ids":[...]}` → `{"deleted": n}` (run.write) |
| GET | `/runs/{id}/executions?q=` | list `Execution` (no step results) with `readiness`, `configuration_id`, `configuration_name` and `iteration_count`; query field `configuration` (alias `config`) |
| POST | `/runs/{id}/executions` | add tests: `{"test_ids":[...]}` – one execution per test and configuration of the run → `{"added": n}` |
| PUT | `/runs/{id}/configurations` | `{"configuration_ids":[...]}` (run.write, all or nothing) → `{"run", "added", "removed"}`: an added configuration gets an execution for every test of the run; the executions of a removed one are deleted (409 while one of them has a result other than `not_run`). Adding the first configuration to a run without any assigns its executions to it; removing the last one keeps its executions without configuration |
| POST | `/runs/{id}/executions/bulk` | `{"ids":[...] \| "all": true, "from": [results]?, "set": {"status"?, "assignee"?, "notes_append"?}}` (run.execute) → `{"updated", "rejected", "skipped"}`, each `[{id, key, title, status, assignee, reason}]` with the values before; or `{"ids" \| "all", "from"?, "remove": true}` (run.write) → `{"removed": n, "skipped"}`. `status` is `passed`, `failed`, `blocked`, `skipped` or `not_run`; `from` keeps only executions with those results (others are `skipped` with reason `status`, unchanged ones with `already`). Results that record an execution go through the readiness check; refusals are `rejected` with the blocking issues as `reason`. Executions are applied in passes, so prerequisites in the same request are recorded first. Step results are not changed. |
| POST | `/runs/{id}/results?configuration=` | CI import: `{"results":[{"test_key","status","duration_ms","output","configuration"?}]}` (see *Configurations of imported results* below) → `{"updated":n,"unknown":["KEY"],"rejected":[{"test_key","readiness","reason"}]}`; results of tests that may not be executed (see below) are not recorded; `reason` spells out the blocking issues |
| POST | `/runs/{id}/junit?create_missing=&configuration=` | JUnit XML import (see below): body `application/xml`/`text/xml`, or multipart with one or more `file` parts; at most 20 MB → `{"updated","unknown","rejected","unmapped":["test case name"],"created":[{"id","key","title"}]}` (the first three as for `/results`); 400 for malformed XML |
| GET | `/executions/{id}` | `Execution` with `step_results` (each with `iteration_id`), `iterations` (data-driven tests), `readiness`, embedded `test` (TestCase incl. steps & script) and `job`, its latest background job (see *Background jobs* below; absent if it never had one) |
| PATCH | `/executions/{id}` | `{"status","notes","assignee","output","duration_ms","step_results":[{"id","status","actual","evidence"}],"iterations":[{"id","status"?,"notes"?}]}` – status derived from steps (or iterations) when omitted and steps or iterations given; 409 `not_ready` when the update records an execution that the settings forbid |
| POST | `/executions/{id}/run-script` | run automated script server-side (permission `script.execute`; 400 if the test has no automated script, 409 if the runner is disabled, the test may not be executed or it is queued or running as a job) → execution with embedded `test`. A **web test** (script language `playwright`) is queued as a background job instead → 202 with the execution (`in_progress`) and its `job` |
| POST | `/executions/{id}/run-script/cancel` | (script.execute) cancels the queued job of the execution or stops its running one → `{cancelled, stopping}`; 409 if it has none |
| POST | `/runs/{id}/run-automated` | (script.execute) "Run automated tests": `{"execution_ids"?: [...], "include_passed"?: false}` queues background jobs for the automated executions of the run — without `execution_ids` every automated execution that has not passed (all with `include_passed`) → 202 `{"queued": [job], "skipped": [{execution_id, test_key, configuration_name, reason, code}]}`; `code` is `not_automated`, `no_script`, `disabled` (the server does not run that language), `active` (already queued or running), `not_ready` (readiness: the blocking issues are the `reason`) or `cycle` |
| GET | `/runs/{id}/jobs` | (run.read) the current jobs of the run — the batches with queued or running jobs, else the latest batch → `{"items": [job], "counts": {queued, running, done, passed, failed, skipped, cancelled, error, total}, "active": bool}` |
| POST | `/runs/{id}/jobs/cancel` | (script.execute) `{"job_ids"?: [...], "running"?: false}` cancels queued jobs (all queued ones of the run without `job_ids`); their executions get their earlier result back; with `running` running jobs are stopped too → `{cancelled, stopping}` |

**Background jobs.** Web tests take seconds to minutes, so they never run
inside a request: `run-script` of a web test and `run-automated` enqueue
**jobs** and answer at once. A job is `{id, run_id, execution_id, batch_id,
seq, state, after, requested_by, message, created_at, started_at,
finished_at, test_key, test_title, configuration_name}`; `state` is
`queued`, `running`, `passed`, `failed`, `skipped` (not executed: e.g. its
prerequisite failed meanwhile — `message` says why), `cancelled`, `error`
(it could not run, e.g. Node.js is missing) or `interrupted` (the server
stopped). Its execution is `in_progress` while the job is queued or running
and gets its earlier result back when the job is cancelled, skipped,
failed to start or interrupted. A worker pool of `runner.web.max_parallel`
jobs per server process runs them in queue order; `after` lists the jobs
of prerequisites (tests the test depends on, in the same run and
configuration) that must finish first, and the readiness is checked again
just before a job starts. Jobs do not survive a restart: on start-up (and,
with several instances on PostgreSQL, when an instance's heartbeat is more
than a minute old) unfinished jobs become `interrupted` and their
executions are reset with a note in the output. `run-automated` also runs
scripts of the other languages as jobs; `run-script` of those still runs
synchronously. A finished web test stores its result, the Playwright
output (secrets masked) and the duration, and attaches its screenshots,
trace (`trace.zip`) and video to the execution (tagged `web-test`,
replacing those of its previous run).

**Web tests.** Script language `playwright`: the script is the body of a
Playwright test (`await page.goto('/login'); …`), wrapped into
`test(<title>, async ({ page, context, request, baseURL }) => { … })`, or,
when it calls `test(` itself, a complete spec file. The base URL is, in
this order, the value of a configuration dimension named `Base URL` (or
`URL`, `Bas-URL`), the URL of the run's environment in the project's web
test settings, or the project's base URL; a configuration dimension
`Browser`/`Webbläsare` picks the browser (Chrome, Chromium, Edge →
chromium; Firefox → firefox; Safari, WebKit → webkit), else the server's
default. Project variables are passed to every automated script as
`QTF_VAR_<NAME>`, together with `QTF_BASE_URL`, `QTF_BROWSER`,
`QTF_TEST_KEY`, `QTF_RUN_ID`, `QTF_RUN_NAME`, `QTF_ENVIRONMENT` and
`QTF_BUILD`.

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/projects/{pid}/web-tests` | project.read | `{base_url, environment_urls: [{environment, base_url}], variables: [{name, value, secret, has_value}], enabled, browser}`; secret values are never returned (`value` empty, `has_value`) |
| PUT | `/projects/{pid}/web-tests` | project.update | `{base_url, environment_urls, variables}` (replaces them); a secret variable with an empty `value` keeps its saved value; 400 for non-http(s) URLs, duplicate environments or variable names (case-insensitive), names other than `[A-Za-z_][A-Za-z0-9_]*` (at most 64), values over 4096 bytes, more than 100 variables |

**JUnit XML import.** `POST /runs/{id}/junit` reads `<testsuites>` or
`<testsuite>` documents (suites may nest) as written by Playwright, pytest,
go-junit-report, Maven Surefire, Gradle, Jest and Vitest. Each `<testcase>`
is mapped to a test key: a testcase property named `qtestframe.key`,
`test_key` or `qtestframe` (Playwright annotations with
`embedAnnotationsAsProperties`), else the first match of
`[A-Z][A-Z0-9]{1,15}[-_]TC[-_][0-9]+` in the name, then in the classname
(underscores become hyphens, so `test_SHOP_TC_3` is `SHOP-TC-3`). Status:
`<failure>` or `<error>` → `failed`, `<skipped>` → `skipped`, otherwise
`passed`; `time` (seconds) becomes `duration_ms`; the output holds the
failure/error/skipped messages and texts and `system-out`/`system-err`
(at most 64 KiB). Several testcases with one key give one result: `failed`
wins over `passed` (and `passed` over `skipped`), durations add up and the
outputs are joined. Results then go through the same readiness check, run
auto-completion and response as `/results`; `unmapped` lists the names of
testcases without a key. With `create_missing=true` (needs `test.write`
in addition to `run.execute`, else 403) every testcase without a key gets a
test: an automated test of the run's project with exactly that title, or a
new draft automated test titled after the testcase (listed in `created`);
its result is recorded and the test added to the run.

**Configurations of imported results.** A result names its configuration
with `configuration` (name, ignoring case, or id); results without one use
the `?configuration=` parameter of the request. Without either, a run
targeting exactly one configuration uses it, a run without configurations
records the result without configuration (as before) and a run targeting
several answers 400. An unknown configuration is a 400. A configuration the
run does not target yet is added to it (see `PUT /runs/{id}/configurations`).
`qtestframe report --configuration NAME` and the Playwright reporter's
`configuration` option (both default `$QTF_CONFIGURATION`) pass it on.

**Iterations.** An execution of a data-driven test has `iterations`:
`[{id, position, name, values: {param: value}, status, notes}]`, one per
data row (a snapshot made when the test was added). Step results of an
iteration carry its `iteration_id`. An iteration with steps takes the
result derived from its steps (like an execution); one without steps takes
the `status` given in `iterations`. The execution's result is then derived
from the iterations: `failed` if any failed, else `blocked` if any is
blocked, `passed` when all passed or were skipped (at least one passed),
`skipped` when all were skipped, `not_run` when none has a result, else
`in_progress`. An explicit `status` (also from bulk updates and CI imports)
is the overall result and leaves the iterations unchanged. Iteration notes
are at most 20 000 characters.

**Readiness.** Before a result is recorded the server checks the test:
status `ready`; every linked requirement in one of
`ready_requirement_statuses`; every prerequisite (`/tests/{id}/dependencies`)
`passed` – its execution in the same run (and the same configuration) when
it is part of the run there, otherwise its latest result. Each failed check is an issue; it is
`blocking` unless the matching `allow_*` setting is on.

```json
"readiness": {"ready": false, "allowed": false, "issues": [
  {"kind": "test_status", "id": "…", "key": "SHOP-TC-4", "title": "…", "status": "draft", "blocking": true,
   "required": ["ready"], "setting": "allow_unready_tests"},
  {"kind": "requirement_status", "id": "…", "key": "SHOP-REQ-2", "title": "…", "status": "review", "blocking": true,
   "required": ["approved", "implemented", "verified"], "setting": "allow_unready_requirements"},
  {"kind": "dependency", "id": "…", "key": "SHOP-TC-1", "title": "…", "status": "failed", "blocking": true,
   "required": ["passed"], "setting": "allow_unmet_dependencies", "in_run": true}
]}
```

`required` lists the statuses (or result) that would clear an issue,
`setting` names the [execution setting](#administration) that decides
whether it blocks, and `in_run` tells whether a prerequisite's status is its
result in this run (else its latest result). The `error` text of a refusal
spells each blocking issue out, e.g. *the test SHOP-TC-4 has status "draft",
but only tests with status "ready" may be executed (setting
allow_unready_tests is off)*.

Setting `status` (or a step status) to `in_progress`, `passed` or `failed`
records an execution and needs `allowed`; `not_run`, `blocked`, `skipped`,
assignee, notes and defects are always possible. A refusal is
`409 {"code":"not_ready","error":"…","readiness":{…}}`.

**Step evidence.** Step results carry `evidence` (text, at most 20 000
characters) and `attachment_count`; files are attachments of the step
result: `/step-results/{id}/attachments` (below). They appear in the
document archive with link `entity_type: "step"`, `run_id` and
`execution_id`, and are deleted with the run or the test.

## Configurations

Test configurations are named combinations of dimension values a run can
target. Reading needs `project.read`, changes need `project.update`.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/configurations` | `[Configuration]` in order (`position`, then name) |
| POST | `/projects/{pid}/configurations` | `{"name"?, "description"?, "values":[{"dimension","value"}]}` → 201 Configuration; without `name` it is named after its values (`Chrome / Windows`); names are unique per project ignoring case (409) |
| POST | `/projects/{pid}/configurations/bulk` | `{"configurations":[...], "skip_existing"?: bool}` (1–200, all or nothing) → `{"created":[Configuration], "skipped":["name"]}`; e.g. every combination of some dimensions |
| DELETE | `/projects/{pid}/configurations/bulk` | `{"ids":[...]}` (all or nothing) → `{"deleted": n}` |
| GET | `/configurations/{id}` | Configuration |
| PATCH | `/configurations/{id}` | `{"name"?, "description"?, "values"?, "position"?}` |
| DELETE | `/configurations/{id}` | 204; 409 while a run targets it |

`Configuration`: `{id, project_id, name, description, values:[{dimension,
value}], position, created_by, created_at, updated_at, run_count}`. Each
dimension at most once per configuration (ignoring case), at most 20 values,
dimensions and values up to 100 characters, names up to 200.

## Variants and applicability

Variants are product variants, markets or customer configurations of a
project, optionally in a hierarchy. Requirements and tests apply to all
variants unless their applicability says otherwise. Reading variants needs
`project.read`, changing them `project.update`.

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/projects/{pid}/variants` | project.read | `[Variant]` in hierarchy order (parents first), each with `path` (`EU / EU model DE`) |
| POST | `/projects/{pid}/variants` | project.update | `{"name", "parent_id"?, "description"?, "kind"?: "product\|market\|customer\|other", "attributes"?: {"region": "EU"}, "configuration_ids"?: [...]}` → 201 Variant; names unique per project ignoring case (409); at most 500 variants, 50 attributes |
| GET | `/variants/{id}` | project.read | Variant |
| PATCH | `/variants/{id}` | project.update | the fields to change; `parent_id: ""` makes it a root (a variant cannot be below itself, 400); `position` orders siblings |
| DELETE | `/variants/{id}` | project.update | 204; children move to its parent, its runs become variant-neutral, it is removed from applicability lists |
| GET | `/requirements/{id}/applicability` | requirement.read | `{"variant_ids", "variants":[{id,name}], "expression", "all", "applies_to":[{id,name}]}` |
| PUT | `/requirements/{id}/applicability` | requirement.write | `{"variant_ids": [ids or names], "expression": ""}`; both empty: all variants |
| GET / PUT | `/tests/{id}/applicability` | test.read / test.write | the same for tests |
| PUT | `/projects/{pid}/applicability` | requirement.write or test.write | `{"item_type": "requirement\|test", "item_ids": [...] (1–1000), "variant_ids", "expression"}` → `{"updated", "applicability"}` (all or nothing) |
| GET | `/requirements/{id}/variants` | requirement.read | `{"requirement_id", "applicability", "variants": [{variant, applies, coverage, tests: [keys], configurations: [{id, name, coverage}]}]}` — coverage per variant × configuration (`id ""`: results without configuration) |
| GET | `/projects/{pid}/variants/matrix` | requirement.read | `?q=`, `?folder_id=`, `?variants=` (ids or names, comma-separated), `?configuration=<id>\|none` → `{"variants", "requirements": [{id, key, title, obligation, coverage: {variant id: coverage}}], "summary": {variant id: {"passed": n, …, "applicable": n}}}` |

`Variant`: `{id, project_id, parent_id, name, description, kind, attributes,
configuration_ids, position, created_by, created_at, updated_at, path}`.
Attributes are inherited by the variants below (their own values win);
attribute names may not be `name`, `variant`, `kind` or `id`.
`configuration_ids` are the test configurations the variant is tested in
(empty: all).

**Applicability.** A listed variant includes the variants below it. The
expression is in the [query language](#query-language), evaluated against
each variant with the fields `name` and `variant` (the variant and its
ancestors), `kind` and the attributes (unqualified terms match the names),
e.g. `region:EU AND NOT voltage:110V`. The list and the expression are
combined with OR. Requirements and tests carry `applicability` (absent: all
variants); the query fields `variant` (names of the variants an item applies
to) and `applicability` (`all` | `selected`) filter on it, e.g.
`variant:"EU model"`.

**Coverage per variant** (store `VariantCoverage`): in variant V a
requirement is `not_applicable` unless it applies to V; otherwise its
coverage is computed over its linked tests that apply to V, with the
results of runs for V or one of its ancestors and of variant-neutral runs
(`variant_id` null), in the configurations V is tested in (results without
a configuration always count). Runs carry `variant_id` / `variant_name`
(set on `POST /projects/{pid}/runs` and `PATCH /runs/{id}` with
`variant_id`: id or name, `""` clears it).

**`?variant=<id or name>`** restricts a list to the items that apply to the
variant and reports their coverage (`coverage`, `linked_tests`) or result
(`last_result`) there: requirement and test lists, `GET
/projects/{pid}/traceability` (also with `?configuration=`), exports, ReqIF
and reports that filter requirements or tests. `GET
/releases/{id}/status?variant=` evaluates a release for one variant (its
scope requirements that apply, their tests that apply and the results that
count; the report has `variant`). Generated documents take
`options.variant`. Runs have the query field `variant` (the name of the
variant a run tests; none for variant-neutral runs), e.g. `variant:US`.

**History and baselines.** A change of an item's applicability is a
revision (action `applicability`, changed field `applicability`) in its
history; the snapshot's `applicability` holds `variant_ids`, the
variants' names at the time and `expression` (`null`: all variants). The
audit entry `requirement.applicability` / `test.applicability` reads
`KEY: before → after`. Restoring a revision or restoring from a baseline
restores the applicability the snapshot records (snapshots from before it
was recorded leave it as it is; deleted variants are left out); a baseline
item whose content is unchanged but whose applicability differs is
restored too. Baselines snapshot the applicability and `compare` reports
`applicability` among the changed fields. The trash keeps it with a
deleted item; project archives and backups contain `item_applicability`.

**Release gates per variant.** A release with `variant_ids` (the
variants it targets) is evaluated for each of them (`GET
/releases/{id}/status` → `variants: [{variant, go, criteria, summary}]`),
with the rules above: per variant only its applicable scope requirements,
their applicable tests and the results of runs for the variant, an
ancestor or no variant (variant-neutral runs count for every variant), in
the variant's configurations. The release is a go only when every
targeted variant is (marking it released needs that go or an override);
the combined `criteria` are met when met in every variant, `state` is
`pass`, `waived` (met thanks to waivers or accepted risks) or `fail`,
and blocking items name their `variants`. A waiver with `variant_ids`
accepts its requirements only in those variants (and the variants below
them); a release evaluated without a variant counts only waivers for every
variant. The release readiness report adds the table "Exit criteria per
variant". Deleting a variant removes it from the releases that target it;
waivers limited to it then cover no variant (deleting never widens a
waiver). Projects without variants, and releases without `variant_ids`,
are evaluated as before.

## Code and builds

Commits and merge / pull requests are linked to requirements and tests
automatically, from GitLab and GitHub webhooks, or by hand. CI builds
are recorded when results are uploaded for them.

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/projects/{pid}/code-settings` | project.read | `{"has_secret", "gitlab_url", "github_url", "updated_by", "updated_at"}` |
| PUT | `/projects/{pid}/code-settings` | project.update | `{"secret"?: "16–200 characters", "clear_secret"?: true}`; an empty secret keeps the stored one. The secret is write-only and stored encrypted |
| POST | `/hooks/gitlab/{pid}` | none (public) | GitLab push and merge request events; `X-Gitlab-Token` must equal the secret |
| POST | `/hooks/github/{pid}` | none (public) | GitHub `push`, `pull_request` and `ping` events (`X-GitHub-Event`); `X-Hub-Signature-256` must be `sha256=` + the hex HMAC-SHA256 of the body with the secret |
| GET | `/projects/{pid}/code-changes` | project.read | `[CodeChange]` newest first; `?kind=commit\|merge_request`, `?branch=`, `?since_build=<build id, CI id or number>`, `?limit=` (default 100, max 500) |
| GET | `/requirements/{id}/code` | requirement.read | `{"changes": [CodeChange], "builds": [{build, coverage}], "verified_build"}`; the builds (with run.read) are those a linked test ran in, newest first; `verified_build` is the latest in which every linked test passed |
| POST | `/requirements/{id}/code` | requirement.write | `{"change_id"}`, or a change to add: `{"kind": "commit\|merge_request", "ref", "url"?, "title"?, "repository"?, "provider"?, "status"?, "branch"?}` → 201 CodeChange (provider `manual` by default) |
| DELETE | `/requirements/{id}/code/{cid}` | requirement.write | 204; a manual change linked to nothing else is removed |
| GET / POST / DELETE | `/tests/{id}/code`, `/tests/{id}/code/{cid}` | test.read / test.write | the same for tests; `builds` is the test's build history `[{build, result, results: {configuration id: result}}]` |
| GET | `/tests/{id}/builds` | test.read and run.read | the build history alone (`{items, total}`, at most 100) |
| GET | `/projects/{pid}/builds` | run.read | `[Build]` newest first with `runs` and the result `summary`; `?limit=` |
| POST | `/projects/{pid}/builds` | run.execute | `{"external_id", "number"?, "commit_sha"?, "branch"?, "pipeline_url"?}` → 201 new Build, or 200 with the given fields updated |
| GET | `/builds/{id}` | run.read | `{"build", "runs", "changes"}` (`changes`: the commit it was built from) |
| DELETE | `/builds/{id}` | run.write | 204; its runs stay, without build |
| POST | `/projects/{pid}/regression` | test.read | regression selection, see below |
| POST | `/projects/{pid}/regression/run` | run.write and test.read | the selection's request plus `name`, `description`, `environment`, `build`, `release_id`, `configuration_ids` → 201 `{"run", "selection"}`; 400 when no test is selected |

**Webhooks.** An unknown project, a project without secret and a wrong
signature all answer 401 `invalid_signature`; other events answer 202
`{"ignored": true}`; handled events answer `{"event", "changes", "links"}`.
Commits (push) and merge / pull requests are stored per provider,
repository, kind and ref (SHA or number; a repeated delivery updates title,
status and branches). Their text — the commit message; the MR title,
description and source branch — is scanned for item keys: full keys of
the project (`DEMO-REQ-12`, `DEMO-TC-3`) and short keys (`REQ-12`, `TC-3`);
keys with another project's upper-case prefix are ignored. Every delivery
is audited as `code.webhook`. Bodies are limited to 5 MiB, pushes to
1000 commits.

`CodeChange`: `{id, project_id, kind, provider: gitlab|github|manual,
repository, ref, title, url, author, branch, target_branch, status:
open|merged|closed (merge requests), occurred_at, created_at, updated_at,
items: [{item_type, item_id, key, title, source: auto|manual, created_by,
created_at}], builds}`.

**Builds of uploaded results.** `POST /runs/{id}/junit` and `POST
/runs/{id}/results` take `?build_id=` (the CI's build id, unique per
project), `?build_number=`, `?commit=` (hex SHA), `?branch=` and
`?pipeline_url=`: the build is created or updated and the run is
associated with it (`build_id`, `build_label`; the run's `build` text is
set when empty); the answer has `build`. `?variant=` makes the run a run
for that variant. `Build`: `{id, project_id, external_id, number,
commit_sha, branch, pipeline_url, created_by, created_at, updated_at, runs,
summary}`.

**Regression selection.** Body: `{"requirement_ids"?: [ids or keys],
"change_ids"?: [...], "since_build"?: "<build>", "variant"?: "<variant>",
"include_related"?: false, "include_prerequisites"?: true}`. The changed
requirements are the given ones and those the code changes name — the
given changes and the commits after the build's own commit (or after the
build was recorded when its commit is unknown), on the build's branch.
`include_related` adds the requirements that `depends_on`, `refines` or are
`derived_from` a changed one (one step). The tests are those verifying the
requirements, those the changes name and (by default) their prerequisites,
transitively; deprecated tests and, with a variant, items that do not apply
to it are left out. Answer: `{"requirements": [Item], "tests": [Item],
"changes": [CodeChange], "variant", "since_build"}` with `Item = {id, key,
title, last_result, reasons: [{kind: requirement|change|related|verifies|prerequisite,
ref, type}]}`.

**Combining coverage rules.** The verdict per requirement is a pluggable
`store.CoverageFunc(requirementID, tests, results)` (default
`store.CoverageOf`); `VariantCoverage` and `BuildIndex` hand it only the
tests and results that count for the variant or build.

## Dependency graph and tags

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/projects/{pid}/graph?kind=requirement\|test` | project.read and requirement.read / test.read | `{"kind","nodes":[{id,key,title,status,result,folder}],"edges":[{id,source,target,type}]}`; `result` is the coverage of a requirement or the last result of a test. Requirement edges are relations; test edges are `depends_on` (test → prerequisite) and `calls` (shared steps) |
| GET | `/projects/{pid}/tags` | project.read | `[{tag, requirements, tests, documents, total}]`, most used first |
| POST | `/projects/{pid}/tags/bulk` | requirement.write and test.write | `{"tags":[...], "action": "rename\|merge\|delete", "to"}` – rename one tag, merge several into `to`, or remove them from every requirement and test (and document, with project.update) → `{"requirements","tests","documents"}` (items changed) |

## Profile and sessions

| Method | Path | Notes |
|---|---|---|
| PATCH | `/auth/me` | `{"display_name"?, "preferences"?}` → `Me`. `preferences` keys present in the body replace the stored ones; `null` resets a key. Invalid values → 400 |
| GET | `/auth/sessions` | own active sessions: `[{id, created_at, expires_at, last_seen, ip, user_agent, tenant_id, current}]` (newest first) |
| DELETE | `/auth/sessions/{id}` | revoke one own session (revoking `current` logs out) |
| POST | `/auth/sessions/revoke-others` | revoke all own sessions except the current one → `{"revoked": n}` |

`Me.user.preferences` (all optional; empty = default):

| Key | Values | Effect |
|---|---|---|
| `theme` | `light`, `dark`, `system`, `spring`, `autumn`, `desert`, `camouflage`, `city_camouflage`, `arctic_camouflage`, `navy`, `midnight`, `green_console`, `amber_terminal`, `crazy`, `ocean`, `forest`, `sunset`, `nordic`, `lavender`, `rose`, `high_contrast`, `high_contrast_light` | colour theme (applied on every device) |
| `language` | `en`, `sv`; empty = browser language | language of the web client |
| `start_page` | `projects`, `last_project`, `default_project`, `search` | page shown after login / on `/` |
| `default_project_id` | project id | used by `start_page: default_project` |
| `default_tenant_id` | tenant id (must be a membership) | tenant selected at login when no tenant is given |
| `page_size` | `10`, `25`, `50`, `100` | default rows per table page |
| `date_format` | `locale`, `iso`, `relative` | how timestamps are shown |
| `timezone` | IANA name, e.g. `Europe/Stockholm`; `""` = browser | time zone for timestamps |
| `compact_tables` | bool | denser tables |
| `hide_analysis` | bool | hide the live requirement-language analysis in the editor |
| `create_linked_test` | bool | personal default for "Also create linked test" (overrides the server default) |
| `hide_obligation_highlight` | bool | do not highlight obligation keywords (MUST, SKA INTE …) in requirement descriptions; highlighting is on by default |
| `autocomplete` | `history`, `project`, `off`; empty = on | autocomplete suggestions in free-text fields (see [Autocomplete](#autocomplete)): `history` = only my own history, `project` = only the values used in the project, `off` = no suggestions. Only empty and `history` record what you save |

## Revision history

Every create, update, restore, move and bulk update of a requirement or test
stores a revision (a full JSON snapshot). Tests now also have a `version`.

| Method | Path | Notes |
|---|---|---|
| GET | `/requirements/{id}/history` | `[Revision]` newest first: `{id, entity_type, entity_id, version, action, changed_fields, snapshot, user_email, user_name, user_avatar, created_at}`; `user_name`/`user_avatar` come from the current user record (empty if the user no longer exists) |
| GET | `/tests/{id}/history` | same for tests |
| POST | `/requirements/{id}/restore` | `{"revision_id"}` → restores the snapshot's fields (title, description, obligation, type, priority, status, source, rationale, owner, tags, categories, criteria) as a new version → Requirement |
| POST | `/tests/{id}/restore` | `{"revision_id"}` → restores fields and steps as a new version → TestCase |

`changed_fields` lists top-level keys that differ from the previous revision
(derived fields and timestamps ignored).

## Requirement relations

Types: `refines` (source is a child/decomposition of target), `depends_on`,
`provides` (source supplies something the target needs),
`derived_from`, `conflicts_with`, `duplicates`, `relates_to`. Both
requirements must be in the same project; self-relations and `refines`
cycles are rejected (400); duplicates → 409.

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/requirements/{id}/relations` | requirement.read | `[Relation]` both directions; `direction` = `outgoing` (this is source) or `incoming` |
| POST | `/requirements/{id}/relations` | requirement.write | `{"target_id", "type"}` → Relation |
| DELETE | `/relations/{id}` | requirement.write | |
| GET | `/projects/{pid}/relations` | requirement.read | all relations of the project |
| POST | `/projects/{pid}/relations/bulk` | requirement.write | `{"relations":[{source_id, target_id, type}]}` (max 1000) → `{"created":[Relation], "skipped":[{source_id, target_id, type, reason, message?}]}`; reasons `exists`, `self`, `not_found` (not in the project), `invalid` (unknown type or a `refines` cycle, with `message`) |
| DELETE | `/projects/{pid}/relations/bulk` | requirement.write | `{"ids":[...]}` (max 1000) → `{"deleted":[Relation], "skipped":[{id, reason: "not_found"}]}`; the deleted relations are returned so they can be recreated |

Requirement lists add `relation_count`. Query fields: `related` (keys of all
related requirements), `parent` (keys this requirement refines), `child`
(keys refining this requirement).

Requirement and test responses (lists and single items) also carry
`relations`, the related items in both directions, used by the Relations
column:

```json
"relations": [
  {"type": "depends_on", "direction": "incoming", "id": "…", "key": "SHOP-REQ-2", "title": "Account lockout"},
  {"type": "verifies",   "direction": "incoming", "id": "…", "key": "SHOP-TC-1"}
]
```

`type` is a relation type, `verifies` (a trace link: outgoing on the test,
incoming on the requirement) or `calls` (a shared step: outgoing on the
calling test, incoming on the called one). `direction` is `outgoing` when
this item is the source. Requirements also carry `criteria_count`; tests
carry `step_count` and `defects` (defects recorded on any execution).

## People and @mentions

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/projects/{pid}/people` | requirement.read | `[Person]`: who can be mentioned in the project |

`Person`: `{handle, name, kind, user_id?, email?, avatar?, uses?}`. `kind`
is `user` for members of the organisation (handle = local part of the
e-mail address, made unique with `-2`, `-3` …) or `person` for other names
used as requirement owner or execution assignee (handle = the name in lower
case with spaces replaced by dots). `uses` counts those owner and assignee
values. Users come first, then people, each sorted by name.

A mention is `@handle` anywhere in a text field (an `@` right after a
letter or digit, as in an e-mail address, is not a mention). The query
field `mention` (alias `mentions`) holds the handles mentioned in an item —
for requirements in owner, title, description, rationale and source; for
tests in title, description, preconditions, expected result and steps; for
executions in assignee and notes — so both `mention:anna` and
`mention:@anna` match.

## Tag suggestions

| Method | Path | Permission | Notes |
|---|---|---|---|
| POST | `/projects/{pid}/tags/suggest` | requirement.read | `{"title", "description", "tags"?, "limit"?}` → `[{tag, reason, score, uses?}]` |

Suggestions come from the title and description (at most 100 000
characters together); `tags` are the item's current tags, which are never
suggested again; `limit` is 1–50 (default 8; 0 = default). `reason` is
`existing` (a tag the project already uses, mentioned in the text, also in
inflected form; `uses` is how many requirements and tests carry it, and its
spelling is kept), `acronym` (an upper-case abbreviation such as GDPR) or
`keyword` (a word of the title, or a word used more than once). English and
Swedish stop words, requirement keywords (MUST, SKA …) and placeholders are
never suggested. Existing tags rank first.

## Autocomplete

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/suggest?field=&q=&project_id=&limit=&sources=` | read of the field in `project_id` | → `{items: [{value, source, uses, last_used?}], total}` |
| POST | `/suggest/history` | read of the field in each project | `{"project_id"?, "entries": [{"field", "value", "project_id"?}]}` → `{"recorded": n}` |
| GET | `/suggest/history` | signed in | → `{"count": n}`: how many values you have stored (all organisations) |
| DELETE | `/suggest/history` | signed in | clears your own history in every organisation → `{"deleted": n}` |

Free-text fields of the web client suggest what was typed in them before.
A suggestion comes from two sources (`source`):

- `history`: your own values of the field, recorded by `POST
  /suggest/history` when you save a form (not while you type). Values are
  trimmed, at most 1000 characters, and de-duplicated per field and
  project with a use count and the time of the last use; each user keeps
  at most 200 values per field and organisation (the least recently used
  go). History saved in one project is suggested in the others, but only
  from projects you may still read with the field's read permission.
- `project`: the distinct values already used in the project, ranked by
  how often they are used, for the short fields `requirement.source`,
  `requirement.owner` and `risk.owner`.

`field` is one of `requirement.title`, `requirement.source`,
`requirement.owner`, `requirement.rationale` (permission
`requirement.read`), `test.title`, `test.preconditions`, `test.expected`,
`test.step.action`, `test.step.expected`, `test.step.data` (`test.read`),
`execution.notes`, `execution.step.actual`, `run.name` (`run.read`),
`defect.title`, `defect.description`, `comment`, `review.comment`,
`release.name` (`project.read`), `risk.title`, `risk.mitigation`,
`risk.owner` (`requirement.read`) and `search` (the search box; it belongs
to no project, so `project_id` must be empty). Every other field needs
`project_id`; a project the caller may not read answers 404 and a missing
permission 403, as for any project resource. Unknown fields answer 400.

Ranking: values that start with `q` first, then values where a word
starts with `q`, then values that contain it (ignoring case); values that
do not contain `q`, and `q` itself, are left out. Ties go to the value used
more often and more recently (uses weighted down by age: half after a
month), then to the alphabetical order. An empty `q` gives the most used
and recent values. `limit` is 1–8 (default 8). `sources=history` or
`sources=project` narrows the sources.

The personal preference `autocomplete` applies on the server too: `off`
answers an empty list and records nothing, `project` records nothing,
`history` suggests no project values. The queries of one user are limited
to 300 a minute (429 `rate_limited`; the web client waits 200 ms after the
last keystroke). The history is personal: it is deleted with your
membership of an organisation (in that organisation) and with your
account, and it is not part of a backup.

## Baselines

A baseline freezes the current version of all (or filtered) requirements
and tests of a project.

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/projects/{pid}/baselines` | requirement.read | `[Baseline]` without items, newest first |
| POST | `/projects/{pid}/baselines` | requirement.write | `{"name", "description", "query"?}` (`query` filters requirements and tests like the export filter) → Baseline |
| GET | `/baselines/{id}` | requirement.read | Baseline with `items` |
| GET | `/baselines/{id}/compare?to=current\|<baseline_id>` | requirement.read | BaselineDiff `{from, to, added, removed, changed, unchanged}`; each entry `{entity_type, entity_id, key, title, from_version, to_version, changed_fields}` |
| DELETE | `/baselines/{id}` | requirement.write | 409 when the baseline is signed |
| POST | `/baselines/{id}/restore` | requirement.read, plus write permission for each item's kind | bulk restore to the baseline's version: `{"items":[{entity_type, entity_id}]}` (max 1000) → `{"restored":[{entity_type, entity_id, key}], "skipped":[{entity_type, entity_id, key, reason}]}`; reasons `not_in_baseline`, `deleted`, `unchanged`, `not_allowed`, `needs_review` (restoring a review decision needs the review permission). Each restore is a new version with history action `restore`; audited as `baseline.restore` |
| GET | `/baselines/{id}/signoffs` | requirement.read | `[Signoff]` `{id, user_email, user_name, meaning, comment, content_hash, signed_at, valid}`; `valid` is false when the content no longer has the signed hash |
| DELETE | `/projects/{pid}/baselines/bulk` | requirement.write | `{"ids":[...]}` → `{"deleted": n, "skipped":[{id, name, reason: "signed"}]}`; signed baselines are kept |
| POST | `/projects/{pid}/baselines/signoffs` | requirement.review or test.review | bulk sign-off: `{"ids":[...], "meaning", "comment", "password"}` with the same identity checks as below → `{"signed":[{id, name, content_hash}], "skipped":[{id, name, reason: "already"}]}` |
| POST | `/baselines/{id}/signoffs` | requirement.review or test.review | electronic signature `{"meaning": "authored\|reviewed\|approved\|released", "comment", "password"}`. The password is checked again (403 `reauth_failed`); single sign-on accounts need a session less than 15 minutes old (403 `reauth_required`); API tokens cannot sign. `content_hash` is the SHA-256 of the baseline's items. 409 when the user already signed with that meaning. Audited as `baseline.signed`. |

## Releases

A release is a version of the product: the requirements it contains
(scope), the runs that test it and exit criteria that decide go / no-go.
Reading needs `project.read`, changing `project.update`.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/releases` | `[Release]` with `requirement_count`, `run_count` |
| POST | `/projects/{pid}/releases` | `{"name","version","description","status","target_date":"YYYY-MM-DD","exit_criteria":[ExitCriterion]}`; without `exit_criteria` the defaults are used |
| GET | `/releases/{id}` | Release with `requirement_ids` |
| PATCH | `/releases/{id}` | any of the fields above, plus `release_note`, `override`. Setting `status` to `released` needs a go; without it 409 `{"code":"no_go","status":ReleaseStatusReport}` unless `override: true` and a `release_note`. Releasing sets `released_at`/`released_by` and creates the baseline "Release <name>" of the scope and its tests |
| DELETE | `/releases/{id}` | runs keep their results and lose the release |
| POST | `/projects/{pid}/releases/bulk` | `{"ids":[...], "set": {"status"?, "target_date"?, "exit_criteria"?}, "add_requirement_ids"?, "remove_requirement_ids"?, "query"?}` → `{"updated":[names], "skipped":[{id, name, reason}]}`. Setting `released` releases only the releases with a go (each with its baseline); the others are skipped with reason `no_go` (no override in bulk). Releasing cannot be combined with a scope change. Every release is validated before any is saved |
| DELETE | `/projects/{pid}/releases/bulk` | `{"ids":[...]}` → `{"deleted": n}` |
| POST | `/releases/{id}/requirements` | `{"add":[ids],"remove":[ids],"query":"<filter expr>"}` (the query adds its matches) → `{"added","removed","release"}` |
| GET | `/releases/{id}/status` | `ReleaseStatusReport {release, go, criteria:[{type, threshold, level, met, actual, total, items:[{kind,id,key,title,note}], waived:[…], accepted:[…], available}], summary, waivers:[Waiver], risk_threshold, residual_risks:[…]}` |
| GET | `/releases/{id}/waivers` | (requirement.read) the waivers for the release and the project-wide waivers covering its scope |
| GET | `/releases/{id}/burnup` | `{"release_id","name","target_date","from","to","points":[{"day","scope","covered","passed"}]}`: one point per day from the creation of the release (at most 365 days) to its release, or today |

Runs belong to a release with `release_id` on `POST /projects/{pid}/runs`
and `PATCH /runs/{id}` (`""` removes it). The results of a release are the
latest result of each test in its runs.

The burn-up is computed from history: `scope` counts the release's current
requirements that existed on the day, `covered` those with a trace link
made by then and `passed` those whose linked tests all passed in the
release's runs by the end of the day (as in the release status).

A percentage criterion with nothing to measure (an empty scope; no
executions for `executed`) is not met and has `available: false`. A release
without exit criteria is always a go.

Exit criterion types (`threshold` in percent, default 100):

| Type | Met when |
|---|---|
| `coverage` | at least `threshold` % of the scope requirements have tests |
| `passed` | at least `threshold` % of the scope requirements have all their tests passed |
| `mandatory_passed` | every MUST/SHALL requirement has all tests passed |
| `executed` | at least `threshold` % of the release executions have a result |
| `no_failed` | no test of the release failed or is blocked |
| `approved` | at least `threshold` % of the scope requirements are approved, implemented or verified |
| `no_suspect` | no scope requirement has a suspect link |
| `no_open_defects` | no open defect of at least severity `level` (critical, major, minor, trivial) is linked to the scope |
| `risks_mitigated` | no identified or mitigating risk of at least `level` (low, medium, high, critical) addresses a scope requirement |
| `trace_rules` | no scope requirement and no test linked to one breaks an enabled [traceability rule](#traceability-rules) of severity `level` (`error`, the default: errors only; `warning`: errors and warnings). The items list each item once with the names of the rules it breaks as `note` |
| `no_blocking_defects` | no scope requirement is blocked by an open defect under the project's severity rules (see [change and defect correlations](#change-and-defect-correlations)); a default of new releases |
| `residual_risk` | no risk (not closed) linked to a scope requirement has a residual level above `level` (the release's risk acceptance threshold; default the project's, see [Risks](#risks)) unless its "accept risk" sign-off is valid; accepted risks are listed in `accepted` |
| `cycle_executed` | at least `threshold` % of the planned tests of the [test cycle](#test-cycles) `cycle_id` (of the release's project) have a final result in the cycle's runs (default 100: "fully executed"); `items` lists the tests not yet executed |

**Waivers.** An approved, unexpired [waiver](#waivers-and-deviations) for
the release or for the whole project makes the criteria treat its
requirements as accepted: `coverage` and `passed` count them as met,
`mandatory_passed` does not list them, `no_failed` does not count a failed
test whose scope requirements are all waived. They are listed in `waived`
(note: the waiver keys) instead of `items`; `summary.waived` counts them and
`waivers` lists the waivers in force. `residual_risks` always lists the
scope risks whose residual level is above `risk_threshold` (note: the
level, with `, accepted` when signed off) and
`summary.residual_above_threshold` counts those not accepted.

## Test cycles

A test cycle groups the executions planned for a goal ("Sprint 14
regression"), optionally for a release, a variant and a build, with start
and end dates, a status (`planned`, `active`, `closed`) and an owner (user
id). Its **plan** has one row per test and configuration with a tester
(`assignee_id`, a member who may see the project's runs) and an estimate in
minutes. Starting the cycle creates its run; the progress is derived from
the executions of the cycle's runs (`test_runs.cycle_id`, read-only on the
run as `cycle_id` / `cycle_name`). Reading needs `run.read`, planning
`cycle.plan` (test leads, managers, admins); creating runs needs
`run.write` too.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/cycles` | `[TestCycle]` with `run_ids` and `progress`; `?q=` filters (fields `name`, `goal`, `description`, `status`, `owner`, `release`, `variant`, `build`, `environment`, `configuration`, `start`, `end`, `planned`, `executed`, `progress` (percent executed), `passed`, `failed`, `blocked`, `unassigned`, `test` (planned test keys), `assignee` (testers by e-mail or name), `created_by`, `created_at`) |
| POST | `/projects/{pid}/cycles` | `{"name","goal","description","owner_id" (default: the caller),"release_id","variant_id" (id or name),"build_id","environment","configuration_ids","start_date","end_date","scheduled_start_at","copy_from"}` → 201. `copy_from` copies another cycle's plan, testers and estimates (and its release, variant, build and configurations). A new cycle is `planned`; names are unique per project (case-insensitive) |
| GET | `/cycles/{id}` | TestCycle |
| PATCH | `/cycles/{id}` | the fields above, `status` and `clear_scheduled_start`. `planned` → `active` creates the run of the planned tests (in their configurations, executions assigned to the testers' e-mails, with the cycle's release, variant, build and environment) and tells the testers |
| DELETE | `/cycles/{id}` | the plan goes; the runs stay without cycle |
| GET | `/cycles/{id}/tests` | the plan: `[{id, test_case_id, test_key, test_title, configuration_id, configuration_name, assignee_id, estimate_minutes, added_by, added_at, execution_id, run_id, status, executed_by, finished_at, duration_ms}]`; the result is the latest execution of the test and configuration in the cycle's runs (a final result wins over a newer execution without one). `?assignee=<user id>` or `none` |
| POST | `/cycles/{id}/tests` | (atomic) `{"test_ids"?, "tests"?: [{test_id, configuration_id?, assignee_id?, estimate_minutes?}], "query"?, "folder_id"?, "recursive"? (default true), "regression"?: <body of POST /projects/{pid}/regression>, "copy_from"?, "assignee_id"?, "estimate_minutes"?, "configuration_ids"?}` → `{"added", "cycle"}`. Without a configuration a test is planned once per configuration of the cycle; pairs already planned are skipped (at most 5000 rows). The default estimate is the test's `estimated_minutes`, else the average duration of its executions with a final result (rounded up to minutes). Query, folder and regression need `test.read`. 409 for a closed cycle |
| PATCH | `/cycles/{id}/tests` | (atomic) `{"ids": [row ids] \| "all": true, "assignee_id"?, "estimate_minutes"?}` → `{"updated"}`; a new tester is also set on the rows' executions that have no final result |
| DELETE | `/cycles/{id}/tests` | (atomic) `{"ids": [row ids]}` → `{"removed"}`; their executions stay |
| POST | `/cycles/{id}/runs` | creates a run (named after the cycle, "(2)", "(3)" … for later ones) of the planned tests that no run of the cycle contains yet → 201 TestRun; a planned cycle becomes active. 400 when there is nothing new, 409 for a closed cycle |
| GET | `/cycles/{id}/progress` | `{"cycle", "testers": [{assignee_id, planned, executed, passed, failed, blocked, estimated_minutes, remaining_minutes, actual_ms}], "burndown": [{day, remaining, remaining_minutes, ideal, ideal_minutes}]}` |
| GET | `/cycles/{id}/history` | revisions of the cycle: its fields and its plan (test key, configuration, tester, estimate) |
| GET | `/projects/{pid}/cycle-schedules` | recurring cycles |
| POST | `/projects/{pid}/cycle-schedules` | `{"name_pattern" ({date}: the start date, {n}: the number), "template_cycle_id"?, "query"?, "owner_id"?, "frequency" (daily, weekly, monthly), "weekday" (0 = Sunday … 6), "day_of_month" (1–28), "time" (HH:MM), "timezone", "duration_days" (1–366), "auto_start", "enabled"}` → 201 |
| PATCH | `/cycle-schedules/{id}` | the fields above; the next run is computed again |
| DELETE | `/cycle-schedules/{id}` | the cycles it created stay |
| POST | `/cycle-schedules/{id}/run` | creates the next cycle now → 201 TestCycle |

`progress` counts the plan: `planned`, `executed` (a final result:
passed, failed, blocked or skipped), `passed`, `failed`, `blocked`,
`skipped`, `in_progress`, `not_run`, `not_in_run` (rows no run of the
cycle contains yet), `unassigned`, `estimated_minutes`,
`remaining_minutes` (estimates of the rows not executed),
`estimated_minutes_executed`, `actual_ms` (durations of the executed rows),
`percent_executed`, `percent_passed`.

The **burndown** has one point per day from `start_date` (else the day the
cycle was created) to `end_date` (else today; at most 365 days):
`remaining` / `remaining_minutes` are the rows planned by the end of the
day that had no final result then (`null` for days after today); the
ideal line falls linearly from the whole plan on the first day to 0 on the
last.

**Scheduling.** The background jobs (every minute, also without e-mail)
start the planned cycles whose `scheduled_start_at` has come (as the owner,
who must hold `cycle.plan`; otherwise the scheduled start is dropped and
`cycle.start_failed` is audited) and run the schedules that are due: a new
cycle named after the pattern (made unique), starting today in the
schedule's time zone and lasting `duration_days`, with the template's
fields and plan plus the tests matching `query`; with `auto_start` it
starts. A schedule that missed runs creates one cycle; `last_error`,
`last_cycle_id` and `runs` record the outcome. Schedules use the same
daily / weekly / monthly times as [e-mailed reports](#e-mail).

Audit actions: `cycle.create`, `cycle.update`, `cycle.plan`,
`cycle.delete`, `cycle_schedule.create`, `cycle_schedule.update`,
`cycle_schedule.delete`, and `run.create` for the cycle's runs. The
report template `cycle` (parameter `cycle_id`; default the first active or
planned cycle) renders a cycle's progress, workload, burndown and plan.
Test query term `cycle:` matches the names of the cycles planning a test,
run query term `cycle:` the name of a run's cycle.

The store function `Store.AssignedCycleTests(ctx, tenantID, userID)` lists
a user's planned tests without a final result in open cycles (for a
personal work list; the caller filters the projects the user may see).

## Defects

Defects have keys like `SHOP-BUG-3`. Reading needs `project.read`; creating,
changing and linking `run.execute`; deleting `project.update`.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/defects?q=` | `[Defect]`; query fields `key, title, description, status, severity, priority, assignee, external, created_by, open, linked, found_in` (alias `phase`) |
| POST | `/projects/{pid}/defects` | `{"title","description","status","severity","priority","assignee","external_key","external_url","found_in","links":[{"entity_type":"requirement\|test\|execution","entity_id"}]}`; `found_in` is the phase the defect was found in: `requirements`, `design`, `development`, `test`, `acceptance`, `production` or `""` (unknown); defects found in production count as escaped in the quality metrics |
| GET | `/defects/{id}` | Defect with `links` (key and title of each item) |
| PATCH | `/defects/{id}` | any field above |
| DELETE | `/defects/{id}` | |
| POST | `/projects/{pid}/defects/bulk` | `{"ids":[...], "set": {"status"?, "severity"?, "priority"?, "assignee"?}}` → `{"updated": n}`; every defect is validated before any is saved |
| DELETE | `/projects/{pid}/defects/bulk` | `{"ids":[...]}` → `{"deleted": n}` (project.update) |
| POST / DELETE | `/defects/{id}/links` | `{"entity_type","entity_id"}` (same project) |
| GET | `/requirements/{id}/defects`, `/tests/{id}/defects`, `/executions/{id}/defects` | `[LinkedDefect]` with `via`; a requirement includes the defects of its tests and their executions |

Statuses: `open`, `in_progress` (open), `resolved`, `closed`, `rejected`.
Severities: `critical`, `major`, `minor`, `trivial`. Requirements carry
`open_defects` (query field `defects`).

**Jira, GitHub, GitLab and Request Tracker.** Each project can be connected to one tracker.
The same connection is used for defects and for
[issues created from requirements](#issues-from-requirements).

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/projects/{pid}/defect-tracker` | project.read | `{type: ""\|jira\|github\|gitlab\|rt, url, project_key, repo, project, queue, labels_field, issue_type, labels, milestone, assignee, requirement_labels, requirement_issue_type, comment_on_pass, user, has_token}`; the token is never returned |
| PUT | `/projects/{pid}/defect-tracker` | project.update | same fields plus `token` (write-only; empty keeps the saved one) and `clear_token` |
| POST | `/projects/{pid}/defect-tracker/check` | project.update | 200 or 502 `tracker_error` |
| POST | `/defects/{id}/push` | run.execute | creates the issue → Defect with `external_key`, `external_url`; 409 when already linked |
| POST | `/defects/{id}/sync` | run.execute | reads the issue status and maps it onto the defect |
| POST | `/projects/{pid}/defects/sync` | run.execute | all linked defects, or only `{"ids":[...]}` → `{"synced","changed","failed":[{key,error}]}` |
| POST | `/projects/{pid}/defects/push` | run.execute | `{"ids":[...]}`: creates issues for those without one → `{"pushed":[{key,external_key}],"skipped":[keys already linked],"failed":[{key,error}]}` |
| POST | `/projects/{pid}/defects/import` | run.execute | `{"external_key","severity","links"}` creates a defect from an existing issue |

Jira uses REST API v2 at the site URL with e-mail + API token (basic
auth), or a personal access token when `user` is empty. GitHub uses
`https://api.github.com` (or a GitHub Enterprise API URL) and `repo`
`owner/name`; a fine-grained token needs Issues read and write. Status
mapping: Jira status categories To Do → `open`, In Progress →
`in_progress`, Done → `closed` (a resolution such as Won't Do or Duplicate
→ `rejected`); GitHub open → `open`, closed → `closed`, closed as not
planned → `rejected`; GitLab `opened` → `open`, `closed` → `closed`.

GitLab uses REST API v4 at `url` (default `https://gitlab.com`, or a
self-managed instance; `https` only, a trailing `/api/v4` is removed) and
`project`, the numeric project ID or its path `group/subgroup/project` (a
pasted project URL is reduced to the path). The token, a personal, project
or group access token with the `api` scope, is sent in the `PRIVATE-TOKEN`
header. `labels` (comma-separated) are added to every new GitHub and GitLab
issue; `milestone` (GitLab: the number shown as `%3` or the title, also of
a group milestone; GitHub: number or title) and `assignee` (username or
numeric ID; GitHub login) are given to new issues. Jira ignores labels,
milestone and assignee of the settings.

Request Tracker (`type: "rt"`) uses the REST2 API of RT 5 below
`url` + `/REST/2.0` (a trailing `/REST/2.0` is removed) with an RT auth
token sent as `Authorization: token <token>`. New tickets go to `queue`
(name or ID) as `text/plain`; `labels` and the labels of issues from
requirements are written to the custom field `labels_field` when it is set
(RT has no labels). Tickets are referred to as `#123` (`123` and `RT#123`
are accepted). Status mapping of the default lifecycle: `new` → `open`,
`open` and `stalled` → `in_progress`, `resolved` → `closed`, `rejected` and
`deleted` → `rejected`; other statuses leave the defect's status. An update
of an issue from a requirement sets the subject and adds the new text as a
comment (an RT ticket's content cannot be replaced). The check reads the
queue.

The token is encrypted at rest like the other stored secrets and is write
only: no answer, error message or audit entry contains it. Requests to the
tracker use the server's outbound proxy settings (`HTTPS_PROXY`,
`NO_PROXY`) and the system's trusted CAs, time out after 30 seconds, and
are retried after the tracker's `Retry-After` when it limits the rate (429,
or 403 with no requests remaining) and asks for at most 10 seconds;
otherwise the error says how long to wait. Errors of the tracker answer 502
`tracker_error` with an explanation of 401 (the token was rejected), 403
(missing scope or role), 404 (unknown project, repository or issue) and
422 (the tracker rejected the request) and the tracker's own message.

## Issues from requirements

A requirement can be turned into an issue of the project's tracker
(GitLab, GitHub or Jira, as configured above). The issue is linked as a
web [item link](#item-links) of the requirement with `issue` (key and
state). Reading needs `requirement.read`; creating, updating and syncing
change the requirement's links and need `requirement.write`. An issue
link counts for the configured tracker only (same type and, for GitHub and
GitLab, the same repository or project), so after switching the tracker a
requirement can get an issue in the new one. Every create,
update and sync is written to the audit log (`requirement.issue.create`,
`requirement.issue.update`, `requirement.issue.sync`).

| Method | Path | Notes |
|---|---|---|
| GET | `/requirements/{id}/issue` | `{tracker, link, preview}`: the tracker type (`""` when none), the requirement's issue link in that tracker (or null) and `preview` `{title, description, labels}` of the issue as it would be sent now |
| POST | `/requirements/{id}/issue` | creates the issue and links it → 201 `{link}`; 409 `issue_exists` with `link` when the requirement already has an issue in the tracker (open or update it instead) |
| PUT | `/requirements/{id}/issue` | pushes the current title and description into the issue → `{link}`; 404 when it has none |
| POST | `/requirements/{id}/issue/sync` | reads the issue's state back → `{link, changed, commented}` |
| POST | `/projects/{pid}/requirements/issues` | `{"ids":[...], "update": false}` (at most 100): creates the issues of those without one; those with one are `skipped`, or `updated` with `"update": true` → `{"created":[{key,issue_key,url,state}],"updated":[...],"skipped":[...],"failed":[{key,error}]}` |
| POST | `/projects/{pid}/requirements/issues/sync` | the issues of all requirements, or only `{"ids":[...]}` → `{"synced","changed","commented","failed":[{key,error}]}` |

The issue's title is `[KEY] title`. Its description (GitLab and GitHub
Markdown, Jira wiki markup) holds the requirement's text, rationale,
acceptance criteria, type, priority, obligation, status, owner, source,
tags and version, the linked tests with their latest results, and links to
`/go/{key}` of the requirement and its tests (when `server.base_url` is
set). In Markdown the generated part is enclosed in
`<!-- qtestframe:begin -->` and `<!-- qtestframe:end -->`: an update
replaces only that part and keeps text written in GitLab or GitHub around
it. A Jira update replaces the whole description.

Labels: the tracker's `labels`, plus `requirement_labels`, comma-separated
templates with `{type}`, `{priority}`, `{obligation}`, `{status}` and
`{tags}` (one label per tag); a template whose value is empty is left
out. Empty uses `requirement, type::{type}, priority::{priority}, {tags}`;
`none` adds no mapped labels. An update adds the mapped labels and removes
none. Jira issues get the type `requirement_issue_type` (default Task) and
the labels with spaces replaced by `_`.

Syncing maps the issue state to `open` or `closed`. With
`comment_on_pass`, a sync that finds all linked tests passed comments once
on the open issue; when the tests stop passing, the next pass comments
again. Syncing runs on demand (there is no schedule). Like the defect push
and sync, these calls save item by item rather than in one transaction.

## Change and defect correlations

How a change of a requirement or test travels to the items around it, when
test results stop counting as evidence, which open defects block
requirements, and what happens when a fixed defect comes back. The rules
are per project.

| Method | Path | Permission | Description |
|---|---|---|---|
| GET | `/projects/{pid}/correlation-settings` | project.read | `{relation_impact, relation_modes, impact_depth, impact_rules, flag_issues, stale_after_days, stale_on_change, blocking_severities, warning_severities, regression_action}` |
| PUT | `/projects/{pid}/correlation-settings` | project.update | partial update; `relation_impact` is replaced as a whole |
| GET | `/requirements/{id}/impact` | requirement.read | what a change of the requirement's substance makes suspect, out of date, outdated or stale |
| GET | `/tests/{id}/impact` | test.read | the same for a test |
| GET | `/baselines/{id}/impact` | requirement.read | the impact of every change of substance since the baseline, with totals |
| POST | `/projects/{pid}/impact` | requirement.read | impact analysis: `{items: [{kind, id}], baseline_id?, depth?}` → the combined transitive impact |
| GET | `/projects/{pid}/impact/export` | requirement.read | the same as CSV or PDF (`?items=kind:id,…&baseline=&depth=&format=csv|pdf&lang=`) |
| POST | `/relations/{id}/confirm` | requirement.write | clears the suspect mark of a relation → `{confirmed}` |
| POST | `/requirements/{id}/relations/confirm` | requirement.write | confirms all suspect relations of a requirement |
| POST | `/item-links/{id}/issue/confirm` | requirement.write | the tracker issue is still right: clears `issue.outdated_since` |
| GET | `/requirements/{id}/blocking-defects` | requirement.read | `{blocked, blocking, warnings, items: [{defect_id, key, title, severity, status, level, via}]}` |
| GET | `/requirements/{id}/stale-results` | requirement.read | the latest results of its tests with their staleness |
| GET | `/defects/{id}/regressions` | project.read | regression history of a defect |
| GET | `/tests/{id}/regressions` | test.read | regression history of a test |

**Settings.** The defaults: `relation_impact` `refines`, `derived_from` and
`depends_on` `incoming`, the other types `off`; `flag_issues` true;
`stale_after_days` 90 (0 turns the age rule off, at most 3650);
`stale_on_change` true; `blocking_severities` `["critical", "major"]`,
`warning_severities` `["minor"]` (a severity cannot be in both);
`regression_action` `reopen` (or `new_defect`, `off`). Directions:
`incoming` — when the target of a relation changes, its source becomes
suspect (A `refines` B: B changed, A is suspect); `outgoing` — when the
source changes, the target becomes suspect; `both`; `off`.

**Change impact.** When the substance of a requirement changes (`title`,
`description`, `obligation`, `criteria`; for a test `title`, `description`,
`preconditions`, `steps`, `expected_result`, `script`, `execution_type`):

- its own trace links become suspect, as before;
- the relations along which the change travels become suspect
  (`Relation.suspect_since`, `suspect_reason`, `suspect_end`: `source` |
  `target`), and the trace links of the requirements at their other end —
  their verifying tests — become suspect too (audit `relation.suspect`);
- with `flag_issues`, its tracker issues are flagged out of date
  (`ItemLink.issue.outdated_since`, `outdated_reason`) until the issue is
  updated from the requirement (`PUT /requirements/{id}/issue`) or
  confirmed;
- the generated documents of the archive that contain it are outdated:
  `GET /projects/{pid}/documents` returns `outdated: true` and
  `outdated_items` (keys) for them. Generated documents remember the
  requirements (specification) or tests (test plan) they contain;
- results recorded before the change go stale (below).

`GET /requirements/{id}/impact` previews this before a change is saved:

```json
{"kind": "requirement", "id": "…", "key": "SHOP-1", "title": "…",
 "requirements": [{"kind": "requirement", "id": "…", "key": "SHOP-2", "relation": "refines", "direction": "incoming"}],
 "tests": [{"kind": "test", "id": "…", "key": "SHOP-T-1", "requirement_id": "…"},
           {"kind": "test", "id": "…", "key": "SHOP-T-2", "via": "SHOP-2", "already": true}],
 "issues": [{"kind": "issue", "id": "<item link id>", "key": "group/shop#12", "url": "…"}],
 "documents": [{"kind": "document", "id": "<file id>", "key": "SHOP-requirements-specification-v1.0.pdf"}],
 "results": [{"kind": "result", "id": "…", "test_id": "…", "key": "SHOP-T-1", "result": "passed"}],
 "total": 5}
```

`already` marks items that are suspect, out of date, outdated or stale
already; `total` counts the others. Without `test.read` the tests and
results are left out. `GET /baselines/{id}/impact` returns
`{baseline, changes: [{entity_type, entity_id, key, title, from_version, to_version, changed_fields, impact}], totals: {requirements, tests, issues, documents, results}}`
for the items whose substance changed since the baseline (largest impact
first; totals count distinct items).

**Transitive impact.** The change travels on from what it reaches, breadth
first and cycle safe, up to `impact_depth` steps (1–10, default 3; `?depth=`
on the impact endpoints). Depth counts the steps between requirements,
tests and risks; trace links attach evidence without counting as a step
and are not followed further, and the defects, variants, code changes and
runs of a reached item are attached the same way, so depth 1 is the
one-level impact above. Besides relations (`relation_impact`, with
`relation_modes` `suspect` | `review`), `impact_rules` (`{direction, mode}`)
govern `verifies`, `calls` (shared steps), `test_depends_on`, `item_link`,
`risk`, `defect`, `variant` and `code`; only `verifies` can mark suspect,
the others recommend a review. The mode of an item is the weakest on its
path. Every item has `depth`, `mode`, `path` (`[{kind, id, key, edge,
direction}]` from the changed item) and `owner`; the impact has `roots`,
`depth`, `suspect`, `review`, `by_kind`, `by_depth` and the further lists
`risks`, `defects`, `links` (runs), `variants`, `code`. A status change only
recommends reviews.

**Dry run and choice.** Every endpoint that changes requirements or tests
(`PATCH /requirements/{id}`, `PATCH /tests/{id}`, the `bulk` and `batch`
endpoints, `exchange/{xlsx,reqif}/apply`, `requirements/import`,
`catalog/import`, `POST /requirements/{id}/restore`, `POST /tests/{id}/restore`,
`POST /baselines/{id}/restore`) takes `?dry_run=impact` — the change runs in
a transaction, the answer is `{dry_run: "impact", status, impact}` and
everything is rolled back — and `?impact=suspect|notify|skip`: `suspect`
(default) marks what the rules mark and notifies the owners of the items
recommended for review (event `impact.review`); `notify` marks nothing and
notifies every affected owner; `skip` marks nothing and notifies nobody.
A bulk change applies the combined impact once, before it commits.

**Result staleness.** A result is stale when it is older than
`stale_after_days`, or (with `stale_on_change`) when its test or a
requirement the test verifies changed in substance after it was recorded.
A stale pass is no passing evidence: status follows evidence does not
verify on it, coverage counts it as not run, the release exit criteria
`passed` and `mandatory_passed` note it as `stale`, and the traceability
rule option `passed` ignores it. Tests carry `stale` and `stale_reason`
(`age`, `test_changed`, `requirement_changed`), requirements
`stale_results` (linked tests with a stale latest result); the traceability
matrix has `stale` per test and `stale_results` per requirement; reports
mark stale results. Query fields: tests `stale` (`true`/`false`),
requirements `stale` (count). Re-running the test makes a fresh result.

**Blocking defects.** An open defect linked to a requirement, to a test
verifying it or to an execution of such a test blocks the requirement when
its severity is in `blocking_severities`, and warns about it when it is in
`warning_severities`. Requirements carry `blocking_defects` and
`defect_warnings`; query field `blocked` (count, e.g. `blocked>0`); the
matrix has `blocked`; the dashboard `correlations: {blocked_requirements,
warned_requirements, stale_results, suspect_relations}`. The exit criterion
`no_blocking_defects` (in the defaults of new releases) fails while a scope
requirement is blocked; the release status summary counts `blocked` and
`stale`.

**Regressions.** When an execution fails (manually, in bulk, by a script
job or by imported results) and its test verified the fix of a defect — the
defect is resolved or closed, is linked to the test or one of its
executions, and the test passed in another execution after the defect was
resolved and before this failure — the defect is reopened (`reopen`: status
`open`, linked to the failing execution) or a regression defect is created
(`new_defect`: "Regression: …", same severity, priority and assignee, linked
to the test and the execution). Each defect and execution counts once. The
regression is recorded, the webhooks get `defect.status` or
`defect.created` and `defect.regression` (`{regression, defect}`), and the
defect's assignee (else its author) gets a notification. History:

```json
{"items": [{"id": "…", "defect_id": "…", "defect_key": "SHOP-BUG-3", "regression_defect_id": null,
  "test_case_id": "…", "test_key": "SHOP-T-1", "execution_id": "…", "run_id": "…", "run_name": "Nightly 14",
  "action": "reopened", "previous_status": "resolved", "created_by": "anna@example.com", "created_at": "…"}]}
```

## Risks

Reading needs `requirement.read`, changing `requirement.write`.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/risks` | `[Risk]`, highest score first |
| POST | `/projects/{pid}/risks` | `{"title","description","category","likelihood":1-5,"impact":1-5,"status","mitigation","owner","requirement_ids":[...],"attack":[...]}` (likelihood and impact default to 3); `catalog_ref` is kept only when it names a catalogue risk (an undone import created again) |
| GET / PATCH / DELETE | `/risks/{id}` | PATCH takes the fields of POST except `requirement_ids` |

`attack` lists the MITRE ATT&CK and ATLAS technique ids of a risk
(`["T1110.004", "T0886", "AML.T0051"]`, at most 30): ids are trimmed and
upper-cased, duplicates dropped, and each must be in the technique list of
`GET /catalog/attack/techniques` (400 otherwise, e.g. for a revoked or
deprecated id). A catalogue import copies the catalogue risk's `attack` and
`atlas` into it. The field is in exports, project archives and backups
(column `risks.attack`, a JSON list).
| POST | `/projects/{pid}/risks/bulk` | `{"ids":[...], "set": {"status"?, "owner"?, "category"?, "likelihood"?, "impact"?}, "add_requirement_ids"?, "remove_requirement_ids"?}` → `{"updated": n}`; every risk is validated before any is saved |
| DELETE | `/projects/{pid}/risks/bulk` | `{"ids":[...]}` → `{"deleted": n}` |
| POST | `/risks/{id}/requirements` | `{"add":[ids],"remove":[ids]}` |
| GET | `/requirements/{id}/risks` | the risks a requirement addresses |
| POST | `/requirements/{id}/risks` | `{add:[risk ids], remove:[risk ids]}` – the risks a requirement mitigates, changed from its side (requirement.write); → the risks as for GET |
| GET | `/projects/{pid}/risk-settings` | (requirement.read) `{"threshold", "effective", "default", "levels"}` |
| PUT | `/projects/{pid}/risk-settings` | (project.update) `{"threshold": "low"\|"medium"\|"high"\|"critical"\|""}`: the project's risk acceptance threshold (`""` = medium) |
| POST | `/risks/{id}/accept` | (waiver.approve) `{"comment", "password"}`: the "accept risk" electronic signature (as for [waiver decisions](#waivers-and-deviations)); the risk becomes `accepted` |
| DELETE | `/risks/{id}/accept` | (waiver.approve) withdraws the sign-off; an accepted risk goes back to `mitigating` |

`score` is likelihood × impact; `level` is `low` (1–4), `medium` (5–9),
`high` (10–16) or `critical` (17–25). Statuses: `identified`, `mitigating`
(open), `mitigated`, `accepted`, `closed`. `covered` and `verified` count
the linked requirements with tests and with all tests passed. Requirements
carry `risk_level`, the highest level of their open risks (query field
`risk`).

**Residual risk.** `residual_likelihood` and `residual_impact` (1–5, 0 =
not assessed, both or neither) are the risk after mitigation. They apply
once the mitigation is verified (every linked requirement passed all its
tests), or at once with `residual_manual: true` and a
`residual_justification`. The derived `residual_score`, `residual_level`
and `residual_source` (`inherent`: no assessment or mitigation not verified
yet, so the inherent score applies; `verified`; `manual`) give the score in
effect; `threshold` is the project's risk acceptance threshold and
`above_threshold` whether the residual level is above it (risks not closed).
`review_date` (YYYY-MM-DD) and `review_owner` plan the next review
(`review_due` once the date is reached). The acceptance sign-off fills
`accepted_by`, `accepted_name`, `accepted_at` and `acceptance_comment`;
`acceptance_valid` is false when the residual assessment changed since.

## Exploratory testing

Session-based exploratory testing (SBTM). A session has a charter (`title`,
`mission`, `scope` of areas = folders, requirements and tests), a `tester`
(e-mail), a time box and a timer kept by the server. Reading needs
`run.read`; planning, changing and deleting `run.write`; running a session
(timer, notes, defects from notes, debrief) `run.execute`, and only the
session's tester does that unless the caller also holds `run.write`.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/exploratory-sessions?status=&tester=&outcome=&mine=&requirement_id=&test_id=&folder_id=&q=` | `[ExploratorySession]`, newest first; `q` over key, title, mission (alias charter), status, tester (assignee, owner), outcome (result), area (folder), requirement, test, notes, bugs, timebox |
| POST | `/projects/{pid}/exploratory-sessions` | `{"title", "mission", "tester", "timebox_minutes": 5–480, "scope": [{"type": "folder"\|"requirement"\|"test", "id"}]}` → 201, planned, key `PROJ-EXP-n` |
| GET / PATCH / DELETE | `/exploratory-sessions/{id}` | PATCH not after the debrief (409); DELETE removes notes, attachments and defect links |
| POST | `/exploratory-sessions/{id}/timer` | `{"action": "start"\|"pause"\|"stop"}`; planned/paused → in_progress → paused → completed |
| PUT / DELETE | `/exploratory-sessions/{id}/debrief` | `{"summary", "coverage_pct", "setup_pct", "testing_pct", "bug_pct", "outcome": "passed"\|"failed"\|"inconclusive", "comment"}` (split sums to 100) → debriefed; DELETE (run.write) reopens |
| GET / POST | `/exploratory-sessions/{id}/notes` | `{"type": "observation"\|"question"\|"issue"\|"idea"\|"bug", "body", "file_ids"}`; stamped with the session time |
| PATCH / DELETE | `/exploratory-sessions/{id}/notes/{note}` | the note's author, or run.write |
| POST | `/exploratory-sessions/{id}/notes/{note}/defect` | creates a prefilled defect linked to the session and its scoped requirements (optional `title`, `description`, `severity`, `priority`, `assignee`) |
| GET | `/exploratory-sessions/{id}/history` | revisions (create, update, stop, debrief, reopen) |
| GET | `/exploratory-sessions/{id}/report?format=md\|pdf\|docx&lang=&archive=` | the `exploratory_session` report (also needs `export`) |
| GET / POST / DELETE | `/exploratory-sessions/{id}/attachments…` | screenshots and files, as for other attachments |
| GET | `/projects/{pid}/exploratory-metrics` | counts by status and outcome, hours, bugs, bugs per session hour, average split and coverage, by area and by tester |

`ExploratorySession` = `{id, project_id, key, title, mission, tester,
timebox_minutes, status, started_at, running_since, elapsed_seconds,
ended_at, scope: [{type, id, key, title}], debrief: {summary, coverage_pct,
setup_pct, testing_pct, bug_pct, outcome, by, name, comment, at, reviewed} |
null, version, note_count, note_types, bug_count, defects, created_by,
created_at, updated_at}`. `elapsed_seconds` includes the running part at
the time of the answer.

With `exploratory_evidence` in `PUT /projects/{pid}/verification-settings`
(default false), a debriefed session counts as verification evidence of kind
`exploratory` for its scoped requirements, about the requirement as a whole
(never for a criterion on its own): passed passes, failed fails,
inconclusive does not count, and only the newest session per requirement
counts. Defects link to sessions with `entity_type` `exploratory_session`.
The report templates `exploratory_session` (param `session_id`) and
`exploratory_sessions` are part of `/reports/templates`.

## Waivers and deviations

A waiver accepts requirements that are failed, untested or not fulfilled
for now; a deviation accepts a different solution. Either applies to one
release (`release_id`) or to the whole project (`null`). Reading needs
`requirement.read`; creating, changing, submitting and withdrawing
`requirement.write`; deciding and revoking `waiver.approve` (Manager and
Admin by default).

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/waivers?status=&type=&release_id=&requirement_id=&active=` | `[Waiver]`, newest first; `release_id=project` for project-wide ones, `active=true` for those in force |
| POST | `/projects/{pid}/waivers` | `{"type": "waiver"\|"deviation", "title", "release_id", "requirement_ids", "justification", "compensating", "conditions", "risk_id", "new_risk": {"title","likelihood","impact"}, "approvers": [e-mail], "expires_on": "YYYY-MM-DD", "submit"}` → 201 (key `PROJ-WVR-n`; a draft, or requested with `submit`) |
| GET / PATCH / DELETE | `/waivers/{id}` | PATCH only in draft or requested (409 otherwise); DELETE not while approved (revoke first) |
| POST | `/waivers/{id}/submit` | draft → requested; needs requirements, a justification and, for a deviation, compensating measures; notifies the approvers (`waiver.requested`) |
| POST | `/waivers/{id}/withdraw` | requested or rejected → draft |
| POST | `/waivers/{id}/decision` | (waiver.approve) `{"decision": "approve"\|"reject", "comment", "password"}` |
| POST | `/waivers/{id}/revoke` | (waiver.approve) `{"comment"}` (required): approved → revoked |
| GET | `/requirements/{id}/waivers` | the waivers covering a requirement |

`Waiver` = `{id, project_id, key, type, title, release_id, release_name,
requirement_ids, requirements: [{id, key, title}], justification,
compensating, conditions, risk_id, risk_key, requested_by, approvers,
status, expires_on, decided_at, decided_by, decision_comment, approvals:
[{id, user_email, user_name, decision, comment, content_hash, signed_at,
valid}], active, content_hash, created_by, created_at, updated_at}`.

States: `draft` → `requested` → `approved` or `rejected`; `approved` →
`expired` or `revoked`. A **decision** is an electronic signature over the
hash of the waiver's content (`content_hash`): given in the browser, not
with an API token, with the signer's password or a single sign-on less than
15 minutes old (403 `reauth_failed` / `reauth_required`). The requester
cannot decide their own waiver. With designated `approvers` only they may
sign, and the waiver is approved once all of them approved it; without, one
approval suffices. Any counted rejection (it needs a comment) rejects it.
Changing a requested waiver invalidates earlier signatures (`valid:
false`). Each step is in the audit log (`waiver.create`, `waiver.submit`,
`waiver.signed`, `waiver.approved`, `waiver.rejected`, `waiver.revoked`,
`waiver.expired`) and notifies the requester and approvers
(`waiver.approved`, `waiver.rejected`, `waiver.revoked`, `waiver.expired`;
webhook events `waiver.requested`, `waiver.approved`, `waiver.rejected`,
`waiver.revoked`).

**Expiry.** `active` is true while the waiver is approved and today is not
after `expires_on`. Every minute the server marks approved waivers whose
expiry date has passed as `expired` and notifies the requester and
approvers; their requirements are no longer accepted.

**Effects.** A waiver in force makes [release readiness](#releases) and the
exit criteria treat its requirements as accepted, marked as waived. The
[analytics](#requirement-analytics) and [compliance](#compliance) show
requirements under a project-wide waiver separately (`waived`), and the
[Statement of Applicability](#statement-of-applicability) counts them as
partly implemented. The compliance and `release` reports have a waiver
section; the requirements specification marks waived requirements
("Waived by") and lists the waivers in an appendix.

## Reviews

| Method | Path | Notes |
|---|---|---|
| GET | `/review-checklists` | default checklist texts per kind |
| GET | `/reviews?project_id=&entity_type=&entity_id=&status=&reviewer=me&requested_by=me` | review requests; without a project or item, the caller's own; `requested_by=me` lists the requests the caller sent |
| POST | `/reviews/bulk` | `{"ids":[...], "action": "approve\|request_changes\|cancel\|remind\|due_date", "comment"?, "due_date"?}` → `{"updated":[{id,key}], "skipped":[{id,key,reason}]}`. `approve` / `request_changes`: the caller's own pending requests, without the checklist rule (the comment records the bulk decision; `request_changes` needs a comment). `cancel`, `remind` (a `review.reminder` notification) and `due_date`: pending requests the caller sent, or with write permission on the item. Reasons: `not_pending`, `not_reviewer`, `not_requester` |
| POST | `/reviews` | `{"entity_type":"requirement\|test","entity_id","reviewers":[user id, e-mail or handle],"message","due_date","checklist":[{"text"}]}` (write permission on the item) → the new requests; reviewers with a pending request for the item are skipped |
| GET | `/reviews/in-review` | requirements and tests in status `review`, across projects, that concern the caller, oldest change first: `[{entity_type, entity_id, key, title, project_id, project_key, project_name, owner, created_by, version, updated_at, reasons, request_id?}]`; `reasons`: `owner` (the requirement's owner names the caller by e-mail, name or @handle), `author`, `requested` (an open review request to the caller, `request_id`), `reviewer` (the caller may review that kind) |
| GET | `/reviews/{id}` | ReviewRequest with `entity_key`, `entity_title`, `outdated` (the item changed since) |
| PATCH | `/reviews/{id}` | reviewer: `{"checklist":[{"checked","note"}],"comment","decision":"approved\|changes_requested"}`. Approving needs every point checked or noted; requesting changes needs a comment. Requester (or write permission): `{"cancel":true}`. 409 once decided |

## Notifications and webhooks

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/notifications?unread=1&limit=` | signed in | `{"items":[Notification],"unread":n}` |
| POST | `/notifications/read` | signed in | `{"ids":[...]}`, or `{}` for all |
| POST | `/notifications/unread` | signed in | `{"ids":[...]}` → `{"marked": n}` |
| DELETE | `/notifications` | signed in | `{"ids":[...]}`, or `{"read": true}` for all read ones → `{"deleted": n}`; only the caller's own notifications are affected |
| GET | `/webhook-events` | signed in | event names |
| GET | `/projects/{pid}/webhooks` | project.update | `[Webhook]` without secrets (`has_secret`) |
| POST | `/projects/{pid}/webhooks` | project.update | `{"name","url","events":["*"\|event...],"enabled","secret"?}` → Webhook with `secret` (shown only here) |
| PATCH | `/webhooks/{id}` | project.update | the same fields; `"rotate_secret": true` returns a new secret |
| DELETE | `/webhooks/{id}` | project.update | |
| POST | `/webhooks/{id}/test` | project.update | sends `ping` now → `{"ok","status","error"}` |
| POST | `/projects/{pid}/webhooks/bulk` | project.update | `{"ids":[...], "enabled"?, "add_events"?, "remove_events"?}` → `{"updated": n}`; every webhook must keep an event (else 400, nothing saved); secrets are kept |
| DELETE | `/projects/{pid}/webhooks/bulk` | project.update | `{"ids":[...]}` → `{"deleted": n}` |
| POST | `/projects/{pid}/webhooks/test` | project.update | `{"ids":[...]}` (at most 50) sends `ping` to each → `{"results":[{id, name, ok, status, error}]}` |

Users choose in their profile whether notifications are also e-mailed:
preference `email_notifications` (`PATCH /auth/me`
`{"preferences":{"email_notifications":"off"|"immediate"|"digest"}}`,
default off). See [E-mail](#e-mail).

Users are notified (never about their own actions) when they are made
owner, assignee of an execution or a defect, are @mentioned in a
description, comment or execution notes, get a review request or a
decision on one, and when a defect assigned to them changes status. In
discussions, the people who wrote in a thread hear about replies and about
the thread being resolved, and the owner of an item (requirement owner,
defect assignee, risk owner, reviewer and requester) about new threads.

A webhook delivery is `POST <url>` with `Content-Type: application/json`,
headers `X-Qtestframe-Event`, `X-Qtestframe-Webhook` and
`X-Qtestframe-Signature: sha256=<hex HMAC-SHA256 of the body with the
secret>`, and body
`{"id","event","project_id","actor","time","data"}`. Events:
`requirement.created/updated`, `test.created/updated`,
`run.created/updated/completed`, `execution.updated`,
`defect.created/updated/status/assigned`, `risk.created/updated`,
`release.created/updated/released`, `comment.created/mentioned/resolved`,
`review.requested/completed`, `baseline.signed`, `defect.regression`,
`waiver.requested/approved/rejected/revoked`. Deliveries run in the
background with a 10 s timeout, are not retried and do not follow
redirects; the latest outcome is stored as `last_status`, `last_error`.

## My work

The personal work list: what needs the caller, across the projects they
can see (project sharing and the permissions there apply to every row).

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/my-work?project=&source=&reason=` | signed in | `{"items":[WorkItem],"total":n,"summary":WorkSummary,"sources":[...]}`; `project` is a project id or key, `source` comma-separated source names, `reason` keeps the rows with that reason |
| GET | `/my-work/count?project=&source=` | signed in | `WorkSummary`: `{"count","waiting","overdue","by_reason","by_kind","by_project"}` (the badge in the navigation, the dashboard widget); `count` is the rows that need the caller, `waiting` the rows whose only reasons are `awaiting_review` or `in_review` (listed, not in `count`) |

`WorkItem`: `{kind, id, entity_type, entity_id, run_id?, key, title,
status, project_id, project_key, project_name, reasons, detail?, due?,
overdue, updated_at, sources, review_ids?, requested_review_ids?}`
(`review_ids`: the pending review requests of the item waiting for the
caller's decision, which the client opens from the row;
`requested_review_ids`: the caller's own requests of the item still pending
or returned). Rows of the same item (`kind` + `id`) are
merged and their reasons joined; overdue rows come first, then by due
date, then the most recently changed. Sources and reasons:

| Source | Rows | Reasons |
|---|---|---|
| `owned` | requirements the caller owns in draft, review, approved or implemented; tests in draft or review; open risks (identified, mitigating) | `owner` |
| | owned requirements and tests (not rejected, obsolete or deprecated) with suspect links or relations, stale results or blocking defects | `suspect`, `stale`, `blocked` |
| | risks whose review the caller owns, due within 7 days | `risk_review` (`due` = review date) |
| `defects` | open defects (open, in progress) assigned to the caller | `assigned` |
| `executions` | executions assigned to the caller, not run or in progress, in planned or running runs (`run_id`, `detail` = run name) | `assigned` |
| `reviews` | pending review requests to the caller (the row is the item to review; `due` = the review's due date, `review_ids` = the requests) | `review_requested` |
| `requested_reviews` | review requests the caller asked for: pending ones, and the latest request of a reviewer that came back with changes requested while the item has not changed since (`detail` = the reviewer, `requested_review_ids`) | `awaiting_review` (waiting, not counted), `review_returned` |
| `in_review` | requirements and tests in status review that the caller owns, wrote or was asked to review (as `GET /reviews/in-review`, without the items listed only for the caller's review permission) | `in_review` (waiting, not counted) |
| `waivers` | requested waivers the caller may decide and has not signed (one of its approvers, or holds `waiver.approve` when it names none; never the requester) | `waiver_approval` |
| `risk_acceptance` | mitigated or accepted risks above the acceptance threshold without a valid acceptance signature, for holders of `waiver.approve` | `risk_acceptance` |
| `cycles` | tests of planned or active test cycles assigned to the caller without a final result (a test in one of the cycle's runs is the row of its execution; `detail` = cycle, `due` = its end date) | `assigned` |
| `exploratory` | exploratory sessions the caller is the tester of, planned, running or paused | `assigned` |
| `mentions` | items with an unresolved discussion thread that mentions the caller (`detail` = who wrote last); a requirement can also be an `owned` row, and the reasons are joined | `mentioned` |

A row with a due date before today also has the reason `overdue`. The
sources are a small registry in `internal/api/worklist.go`; its comment
describes how to add one.

## Discussions

Threaded comments on requirements, tests, defects, risks, test runs and
review requests. A thread is a comment with `parent_id` "" and its replies
(`parent_id` = the first comment; a reply to a reply joins the same
thread). Bodies are Markdown (rendered and sanitised by the client);
`@handle` mentions of members are stored as user ids in `mentions`.

| Method | Path | Notes |
|---|---|---|
| GET | `/requirements/{id}/comments`, `/tests/{id}/comments`, `/defects/{id}/comments`, `/risks/{id}/comments`, `/runs/{id}/comments`, `/reviews/{id}/comments` | `[Comment]` oldest first, removed ones included: `{id, project_id, entity_type, entity_id, parent_id, user_id, user_email, user_name, body, created_at, updated_at, resolved_at, resolved_by, resolved_by_name, deleted, mentions, reactions}`; the read permission of the item (a review request: its reviewer, its requester or the read permission of the reviewed item) |
| POST | same | (`comment.create`) `{"body", "parent_id"?}` (1–10000 chars) |
| PATCH | `/comments/{id}` | (`comment.create`) `{"body"}`, author only |
| DELETE | `/comments/{id}` | the author, or a moderator (`project.update` in the project, or `member.manage`): the comment stays in its thread as removed, its body, mentions and reactions are cleared |
| POST | `/comments/{id}/resolve`, `/comments/{id}/reopen` | (`comment.create`) resolves or reopens the thread of the comment → its first comment |
| POST | `/comments/{id}/reactions` | (`comment.create`) `{"reaction"}` (`+1`, `-1`, `heart`, `tada`, `eyes`, `check`) adds or takes back the caller's reaction → `{reactions}` |
| GET | `/projects/{pid}/discussions` | the project's threads: `?status=open` (default) `\|resolved\|all`, `?filter=mine` (threads the caller wrote in) `\|mentioned`, `?type=`, `?q=` (text), `?limit=` → `[DiscussionThread]` `{root, project_key, entity_key, entity_title, replies, last_at, last_by, participants, mentioned}`, latest activity first |
| GET | `/discussions` | the same across the projects the caller sees; without `filter`, the threads that mention the caller |

Requirement and test lists add `comment_count` (comments not removed) and
`open_discussions` (unresolved threads). In their query language
`has:open-discussion` (alias of `open_discussions`, e.g.
`open_discussions>1`) finds items with unresolved threads; `mention:`
(`mentions:`) also matches the people mentioned in an item's discussion,
and `mentions:me` the caller.

Audit log: `comment.create`, `comment.update`, `comment.delete` (with
"(moderated)" when someone else's comment was removed), `comment.resolve`,
`comment.reopen`. Webhook events: `comment.created`, `comment.mentioned`
(data with `mentioned`: user ids), `comment.resolved`; each with
`{kind, id, key, title, comment}`. Comments are part of project archives,
backups and the trash of requirements and tests.

## Test execution history, defects and shared steps

* `GET /tests/{id}/executions` (run.read) → all executions of the test across
  runs, newest first, with `run_name`, `run_status`, `run_environment`,
  `run_build`, `run_created_at`.
* Executions have `defects: [string]` (IDs such as `JIRA-123` or URLs),
  settable with `PATCH /executions/{id}` `{"defects": [...]}`. Projects have
  `defect_url_template` (e.g. `https://jira.example.com/browse/{id}`, set via
  `PATCH /projects/{pid}`); clients render links by replacing `{id}`.
  Execution query field `defect`; test query field `defect` (defects of the
  latest executions).
* **Shared steps:** a test step may set `call_test_id` (another test of the
  same project) instead of action/expected. `GET /tests/{id}` returns
  `call_test_key`/`call_test_title` on such steps and `called_by`
  (`[{id,key,title}]`). When a run is created the called test's steps are
  inserted in place (recursively, max depth 5), prefixed with `[KEY] `.
  Self-calls and cycles → 400.

## Files, attachments and the document archive

Files are stored in the database (so backups contain them). Upload with
`multipart/form-data`: field `file`, optional `description` and `tags`
(comma separated). Maximum size: `files.max_upload_mb` (default 25). Uploads
with CSRF need the `X-CSRF-Token` header like any other mutation.

`File`: `{id, project_id, kind, name, content_type, size, sha256, description,
tags, uploaded_by, created_at, url, links: [{entity_type, entity_id, key}]}`
with `kind` = `attachment` | `document` | `report` | `avatar` | `icon`.

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/files/{id}` | read access to the file's project | metadata |
| GET | `/files/{id}/content[?inline=1]` | same | download; `inline=1` only for images (png, jpeg, gif, webp), PDF and plain text, otherwise `attachment`. Always `X-Content-Type-Options: nosniff` and `Content-Security-Policy: sandbox` |
| PATCH | `/files/{id}` | uploader, or `project.update` | `{name?, description?, tags?}` |
| DELETE | `/files/{id}` | uploader, or `project.update` | removes the file and all its links |
| GET | `/{requirements\|tests\|runs\|executions\|step-results}/{id}/attachments` | read permission of the item | `[File]`; `step-results` are the evidence files of an executed step |
| POST | same | `requirement.write` / `test.write` / `run.execute` (runs, executions and step results) | multipart upload → File (`kind: attachment`) |
| POST | `/{type}/{id}/attachments/link` | same | `{file_id}` attach an existing file of the same project |
| DELETE | `/{type}/{id}/attachments/{file_id}` | same | unlink; an `attachment` file without remaining links is deleted |
| GET | `/projects/{pid}/documents?q=&kind=` | project.read | document archive: every file of the project (attachments, documents, reports) |
| POST | `/projects/{pid}/documents` | `requirement.write` or `test.write` | multipart upload → File (`kind: document`) |
| POST | `/projects/{pid}/documents/bulk` | project.read + uploader or project.update per file | `{"ids":[...], "add_tags"?, "remove_tags"?, "description"?}` → `{"updated":[{id,name}], "skipped":[{id,name,reason:"not_allowed"}]}` |
| DELETE | `/projects/{pid}/documents/bulk` | as above | `{"ids":[...]}` → `{"deleted": n, "skipped"}` |
| POST | `/projects/{pid}/documents/zip` | project.read | `{"ids":[...]}` → `application/zip` with the files (unique names; at most 512 MB together, else 400) |
| POST | `/projects/{pid}/documents/generate` | export + requirement.read (+ test.read for a test plan) + `requirement.write` or `test.write` | generate a requirements specification or test plan into the archive → 201 File, see [Generated documents](#generated-documents) |

Archive query fields: `name`, `description`, `tag`, `kind`, `type`
(content type), `uploaded_by`, `created_at`, `size`, `linked` (keys of
linked items).

### Generated documents

`POST /projects/{pid}/documents/generate` builds a complete document from the
project's requirements and tests and saves it in the document archive. It
needs `export` and `requirement.read` (router), `test.read` for a test plan,
and the archive's upload permission (`requirement.write` or `test.write`).
Every generation is audit-logged (`document.generate`, with the kind,
format, file name, item counts and duration).

```json
{
  "kind": "requirements_spec",          // or "test_plan"
  "format": "pdf",                      // pdf (default) | docx | md
  "name": "optional file name",
  "description": "optional file description",
  "options": {
    "title": "Shop SRS", "version": "2.0", "author": "Ann",
    "query": "status:approved",         // requirements (spec) or tests (plan)
    "include_inactive": false,          // rejected/obsolete requirements, deprecated tests
    "baseline_id": "…",                 // generate from a baseline instead of the current data
    "group_by": "folder",               // folder | category | type (spec only)
    "dimension": "Level",               // category dimension for group_by category
    "fields": ["description", "tests"], // default: all but "results"
    "traceability": true, "glossary": true,
    "include_discussions": false,       // appendix with the items' discussions
    "dependency_graph": true,           // specification: dependency graph appendix (default true)
    "lang": "sv"                        // default: the user's language
  }
}
```

`dependency_graph` (requirements specification only, default `true`)
adds an appendix with the relations between the requirements of the
specification: layered graphs as vector figures (drawn in PDF, SVG in
Markdown, SVG with a PNG fallback in DOCX) of at most 24 requirements each
(connected groups packed together, larger groups split by top-level
folder, references to the other figures), a legend of the relation types
and statuses, and a table of every relation as text. It is left out when
the requirements have no relations.

→ `201` with the `File` (`kind: document`). Its name defaults to
`<KEY>-requirements-specification-v<version>.<ext>` or
`<KEY>-test-plan-v<version>.<ext>`, its tags are `generated`, the kind's
tag (`requirements-specification` | `test-plan`) and the version
(`v2.0`), its description the title and version (and the baseline).
Download it with `GET /files/{id}/content`. 400 for an unknown kind,
format, field or grouping, an invalid query or a baseline of another
project.

* **Version** defaults to one more than the highest major version of the
  earlier generated documents of the same kind in the archive (`1.0`
  first); those earlier documents make up the revision history.
* **Fields** of a specification: `description` (obligation keywords found
  by the requirement analysis are emphasised), `rationale`, `criteria`,
  `type`, `priority`, `obligation`, `status`, `source` (and the catalogue
  reference), `owner`, `tags`, `categories`, `relations`, `tests` (keys of
  the linked tests), `results` (their latest results), `version`. Of a test
  plan: `description`, `preconditions`, `steps`, `expected`, `priority`,
  `execution_type`, `status`, `estimate`, `tags`, `categories`,
  `dependencies`, `requirements`, `data` (data-driven rows), `results`,
  `version`.
* **Requirements specification** (ISO/IEC/IEEE 29148): title page,
  document control (metadata, revision history, approvals = the signatures
  of the baseline), contents, 1 Introduction (purpose, scope = the project
  description, the BCP 14 key words – in Swedish the SKA/BÖR/FÅR
  convention –, overview), 2 Requirements grouped by folder, category or
  type, Appendix A traceability matrix, Appendix B glossary and references.
* **Test plan** (ISO/IEC/IEEE 29119-3): title page, document control,
  contents, introduction, test items and features (the requirements the
  plan's tests verify, and how many are not verified), test approach with
  the entry criteria (the organisation's execution-readiness settings) and
  exit criteria (of the releases, or the defaults), test environment
  (configurations, run environments), roles and project members, schedule
  (releases, the 50 most recent runs), the test specification by folder,
  Appendix A coverage (requirements → tests, requirements without tests),
  Appendix B glossary and references.
* With `baseline_id` the requirements and tests are the baseline's frozen
  snapshots; trace links, folders, results and the planning data are the
  current ones (links only between items of the baseline).
* PDF and DOCX are portrait A4 with a title page, page numbers, page breaks
  between chapters and a table of contents (PDF: with page numbers, links
  and bookmarks; DOCX: a Word TOC field, updated when Word opens the file).
  Markdown has a linked list of contents.

### Avatars and project icons

`user.avatar` and `project.icon` are `""` (initials), `"preset:<name>"`
(`<name>` = `[a-z0-9-]{1,32}`, the client ships the preset images) or
`"file:<id>"`.

| Method | Path | Notes |
|---|---|---|
| PUT | `/auth/me/avatar` | multipart image (png, jpeg, gif, webp; max 1 MB) → `Me` |
| PATCH | `/auth/me` | `{"avatar": "preset:fox" \| ""}` |
| GET | `/avatars/{file_id}` | any signed-in user; serves avatar images |
| PUT | `/projects/{pid}/icon` | project.update; multipart image (max 1 MB) → Project |
| PATCH | `/projects/{pid}` | `{"icon": "preset:rocket" \| ""}` |

Project icon files are served by `/files/{id}/content?inline=1`.

## Sign-in information

* `Me` additionally contains `previous_login` (`{created_at, ip, user_agent,
  method}` of the successful sign-in before the current one, or `null`),
  `failed_logins_since_previous` (failed password attempts on the account
  since then) and `first_login` (true when the user has never signed in
  before this session).
* `GET /auth/logins` → the caller's last 50 sign-in events
  (`[{id, success, method, ip, user_agent, created_at}]`).
* Preference `intro_done` (bool) records that the first-login intro tour was
  finished or skipped.
* When `user.must_change_password` is true (after an administrator reset),
  every endpoint except `/auth/*` returns 403 with code
  `password_change_required` until `POST /auth/password` succeeds.

## Administration additions

| Method | Path | Permission | Notes |
|---|---|---|---|
| POST | `/members/{uid}/reset-password` | member.manage | resets a local user's password to a random temporary one → `{"temporary_password"}` (shown once); sets `must_change_password` and ends the user's sessions. Refused (403) for super admins and for users who are members of other tenants, unless the caller is a super admin |
| PATCH | `/users/{id}` | super admin | also accepts `{"must_change_password": bool}` |
| GET | `/audit?q=&from=&to=&limit=&format=csv\|md` | audit.read | `from`/`to` are RFC 3339 or `YYYY-MM-DD`; `format=csv` downloads a CSV file, `format=md` (alias `markdown`) a [Markdown document](#markdown-documents) with the range and filter; other formats → 400 |
| GET | `/admin/backup` | super admin | ZIP: `manifest.json` + one `<table>.jsonl` per table (all tenants, files included; sessions excluded) |
| POST | `/admin/restore` | super admin | multipart `backup` (ZIP) + field `confirm=RESTORE`; the schema version must match; replaces **all** data in one transaction and signs everyone out → `{"restored": {table: rows}}` |

CLI equivalents: `qtestframe backup --out FILE.zip`,
`qtestframe restore --in FILE.zip --yes`.

### System settings

Runtime-editable server configuration (Administration → System settings).
All endpoints need a **super administrator signed in with a session**
(API tokens get 403). Precedence and locking: see
[operations.md §3](operations.md#runtime-system-settings-administration--system-settings).

| Method | Path | Notes |
|---|---|---|
| GET | `/admin/settings` | → `{sections, settings: [...], oidc: {providers, source, stored, locked}, locked_patterns, config_file, restart_pending, load_error, trash_retention_days, logging_runtime, secrets}`; `secrets` (read-only) is the key provider of the stored secrets: `{provider: local\|tpm\|pkcs11, key_id, info, active_key, keys, encrypted, plaintext}` (a key fingerprint, never a key) |
| PUT | `/admin/settings` | `{"values": {key: value \| null}}`; returns the GET document |
| DELETE | `/admin/settings/{key}` | reset `key` (e.g. `logging.level`, `auth.oidc`) to the file value |
| GET | `/admin/settings/export` | effective configuration as YAML (attachment), secrets `REDACTED` |
| POST | `/admin/settings/oidc` | add an OIDC provider |
| PUT | `/admin/settings/oidc/{name}` | change a provider (the name cannot change) |
| DELETE | `/admin/settings/oidc/{name}` | delete a provider |
| POST | `/admin/settings/test/oidc` | `{"issuer"}` → discovery result |
| POST | `/admin/settings/test/syslog` | `{"values": {...}}` (unsaved logging values) → sends a test message |
| POST | `/admin/settings/test/email` | `{"values": {...}}` (unsaved e-mail values) → sends a test e-mail to the caller |

A setting in `settings`:

```json
{"key": "logging.level", "section": "logging", "type": "enum", "apply": "runtime",
 "options": ["debug", "info", "warn", "error"],
 "value": "debug", "file_value": "info", "file_source": "file", "source": "database",
 "stored": true, "ignored": false, "locked": false,
 "updated_by": "admin@example.com", "updated_at": "2026-11-21T09:00:00Z"}
```

* `type`: `string`, `int` (with `min`/`max`), `bool`, `duration` (Go
  duration strings such as `"12h"`, `"90s"`), `enum` (`options`), `secret`,
  `list` (read-only).
* `apply`: `runtime` (applies at once), `restart` (stored now, applies at
  the next start; `running_value` and `pending_restart` show the value in
  use), `readonly` (configuration file only).
* `source`: `default`, `file`, `environment` or `database`; `file_value` /
  `file_source` are the value without the database override.
* `locked` with `lock_reason`: `environment` (set by the variable `env`;
  the environment always wins), `config` (`system_settings.locked`) or
  `readonly`. Changing a locked setting answers 409 `locked`.
* `ignored`: a stored override exists but is not applied (locked or
  invalid).
* Secrets (`email.password`) are **write-only**: `value` is always `null`,
  `has_secret` / `file_has_secret` say whether one is set. In PUT a string
  sets the secret, `""` or `null` removes the stored one.

PUT validates all values together with the configuration validation of the
server (as `check-config`) and stores them only when valid (400 otherwise,
naming the key). Unknown keys answer 400. Every change is audited as
`settings.update` (`old → new`, secrets as `set/changed`). `auth.oidc`
cannot be set with PUT; use the provider endpoints:

```json
POST /api/v1/admin/settings/oidc
{"name": "corp", "display_name": "Corporate SSO", "enabled": true,
 "issuer": "https://idp.example.com/realms/x", "client_id": "qtf", "client_secret": "…",
 "scopes": ["openid", "email", "profile"], "email_claim": "email", "name_claim": "name",
 "groups_claim": "groups", "auto_provision": true, "default_tenant": "", "default_role": "viewer",
 "role_mapping": {"qa": "test_lead"}, "local_mfa": "unless_idp_mfa",
 "mfa_amr_values": ["mfa", "otp"], "mfa_acr_values": []}
```

`local_mfa` decides whether Qtestframe asks for its own second factor after
a sign-in through the provider: `off` (default), `unless_idp_mfa` (unless the
ID token's `amr`/`acr` claims contain one of `mfa_amr_values` (default
`mfa`) / `mfa_acr_values`) or `always`.

`name` is 1–40 characters of `a-z`, `0-9`, `-`, `_`. In PUT an omitted or
empty `client_secret` keeps the secret, `"clear_secret": true` removes the
stored one (a secret of the file or `QTF_OIDC_<NAME>_CLIENT_SECRET` then
applies). Providers in GET carry `has_secret`, `secret_source` (`file`,
`environment`, `database` or `""`), `in_file` and `callback_url`, never the
secret. The first change stores the whole list (it then replaces the
providers of the file); `DELETE /admin/settings/auth.oidc` returns to the
file's providers.

Test results are always 200 with `{"ok": bool, "message", "error",
"details"}`; e.g. a discovery test returns the endpoints found in
`details` and an error such as `the document names the issuer "…", not
"…" (they must match exactly)`.

`GET /auth/providers` (public) also returns `password_min_length`, and
`GET /invitations/{token}` returns `password_min_length`.

## SCIM 2.0 provisioning

Identity providers (Microsoft Entra ID, Okta, OneLogin, JumpCloud …)
provision the members of one tenant with SCIM 2.0 ([RFC 7643](https://www.rfc-editor.org/rfc/rfc7643),
[RFC 7644](https://www.rfc-editor.org/rfc/rfc7644)). The service provider
lives **outside** `/api/v1`, at `<base_url>/scim/v2`, and has its own
authentication: `Authorization: Bearer qtfscim_…`, a **SCIM token** that is
bound to one tenant. SCIM tokens do not work for `/api/v1`, and API tokens
and sessions do not work for `/scim/v2`. Responses are
`application/scim+json`; requests may be `application/scim+json` or
`application/json`.

### Model

* **Users** are matched by e-mail (the primary or work address in `emails`,
  else `userName`). A new user gets an account with `auth_provider` `scim`
  and **no password**; the first OIDC/SAML sign-in links it to the provider
  (as for just-in-time provisioning). An existing account with that e-mail
  is taken over. `id` is the Qtestframe user id; `userName`, `externalId`
  and `name` are kept per tenant.
* `active: false` removes the tenant membership and ends the user's
  sessions in the tenant and API tokens there; `active: true` restores the
  membership. `DELETE /Users/{id}` removes the membership and the
  provisioning record (the account stays).
* **Groups → roles.** Groups pushed by the provider are mapped to roles by
  a tenant admin (`PATCH /api/v1/scim/groups/{id}`). An active user gets the
  most privileged role of their mapped groups (admin first, then by number
  of permissions), else the tenant's default role (`default_role`, `""` =
  no membership). Membership and role are recomputed on every user or group
  change and when the mapping changes.
* SCIM never changes `super_admin` and never leaves a tenant without an
  admin (409). The e-mail and display name of a user who also belongs to
  other tenants, or who is a super admin, are not changed (a different
  e-mail is refused with 400 `mutability`).
* Every request that writes is one transaction: when any part fails, nothing
  is saved. Every change is audited with `user_email` `SCIM: <token name>`
  (`scim.user.create|update|deactivate|reactivate|delete`,
  `scim.group.create|update|delete`).
* Rate limits: 1200 requests per minute per token, 20 failed
  authentications per 5 minutes per client address (then 429 with
  `Retry-After`).

### Endpoints

| Method | Path | Notes |
|---|---|---|
| GET | `/scim/v2/ServiceProviderConfig` | patch yes, bulk no, filter yes (max 1000), sort no, ETag yes, changePassword no |
| GET | `/scim/v2/ResourceTypes`, `/ResourceTypes/{User\|Group}` | |
| GET | `/scim/v2/Schemas`, `/Schemas/{urn}` | core User and Group schemas |
| GET | `/scim/v2/Users?filter=&startIndex=&count=&attributes=&excludedAttributes=` | ListResponse; `count` default 100, max 1000 |
| POST | `/scim/v2/Users` | 201 with `Location`; 409 `uniqueness` for a known `userName` |
| GET | `/scim/v2/Users/{id}` | `ETag` = `meta.version` |
| PUT | `/scim/v2/Users/{id}` | replaces `userName`, `externalId`, `name`, `displayName`, `emails`, `active` (an absent `active` keeps the state) |
| PATCH | `/scim/v2/Users/{id}` | PatchOp, see below |
| DELETE | `/scim/v2/Users/{id}` | 204 |
| GET | `/scim/v2/Groups?filter=&startIndex=&count=&excludedAttributes=members` | |
| POST | `/scim/v2/Groups` | `{"displayName","externalId"?,"members"?:[{"value": user id}]}`; members must be provisioned users of the tenant (400 `invalidValue`) |
| GET | `/scim/v2/Groups/{id}` | |
| PUT | `/scim/v2/Groups/{id}` | replaces name and members |
| PATCH | `/scim/v2/Groups/{id}` | PatchOp, see below → 200 with the group |
| DELETE | `/scim/v2/Groups/{id}` | 204; its members' roles are recomputed |
| any | `/scim/v2/Me`, `/scim/v2/Bulk` | 501 |

**Filters** support `eq ne co sw ew gt ge lt le pr`, `and`, `or`, `not ( )`,
parentheses, sub-attributes (`name.givenName`), schema-qualified names
(`urn:ietf:params:scim:schemas:core:2.0:User:userName`) and value paths
(`emails[type eq "work"]`, `emails[type eq "work"].value eq "a@example.com"`,
`members[value eq "<id>"]`). Strings compare case-insensitively except `id`
and `externalId`.

**PATCH** (`{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],
"Operations":[…]}`): `op` is `add`, `replace` or `remove` in any letter
case. Users: paths `active` (boolean or `"True"`/`"False"`, as Entra ID
sends), `userName`, `displayName`, `externalId`, `name`, `name.givenName`,
`name.familyName`, `emails`, `emails[type eq "work"].value`; without a path
the value is an object of such attributes, also with dotted keys
(`{"op":"Replace","value":{"name.givenName":"Ada","active":false}}`).
Attributes Qtestframe does not store (title, phone numbers, the enterprise
extension …) are accepted and ignored. Groups: `displayName`, `externalId`,
`members` (`add` a list, `replace` the list, `remove` with a list as value,
with a filter path `members[value eq "<id>"]`, or without value for all),
or a path-less object (`{"op":"replace","value":{"id":"…","displayName":"…"}}`,
as Okta sends).

Errors use the SCIM format:

```json
{"schemas":["urn:ietf:params:scim:api:messages:2.0:Error"],"status":"409","scimType":"uniqueness","detail":"a user with userName ada@example.com already exists"}
```

`scimType` is one of `invalidFilter`, `invalidPath`, `invalidSyntax`,
`invalidValue`, `mutability`, `noTarget`, `uniqueness` (absent for 401,
404, 409 last admin, 429, 501).

### Administration (`/api/v1`, tenant.manage)

| Method | Path | Notes |
|---|---|---|
| GET | `/scim` | `{"base_url", "default_role", "tokens":[SCIMToken], "groups":[{id, display_name, external_id, role, members}], "users", "active_users"}` |
| PATCH | `/scim/settings` | `{"default_role": role or ""}`; recomputes all provisioned members (409 if the last admin would lose the role) |
| POST | `/scim/tokens` | `{"name","expires_days"?}` → 201 `{"token":"qtfscim_…","item":SCIMToken}` (shown once, stored as SHA-256); API tokens cannot create SCIM tokens (403) |
| DELETE | `/scim/tokens/{id}` | revokes at once → 204 |
| PATCH | `/scim/groups/{id}` | `{"role": role or ""}`; recomputes the group's members (409 if the last admin would lose the role) |
| GET | `/scim/events?limit=` | newest audit entries whose action starts with `scim.` (default 50, max 500) |

`SCIMToken` = `{id, tenant_id, name, prefix, created_by, created_at, expires_at?, last_used?}`.
Admin changes are audited as `scim.token.create`, `scim.token.delete`,
`scim.settings` and `scim.group.role`.

## Reports

Standard reports (`GET /reports/templates` → `[{id, name, description,
params, formats}]`; `formats` is `pdf`, `docx`, `xlsx`, `csv`, `md`):

| id | Content | Params |
|---|---|---|
| `project_status` | KPIs, requirements by status/obligation, coverage, recent runs | – |
| `requirements_coverage` | every requirement with linked tests, last results and coverage | `query` |
| `traceability` | requirement × test links | `query` |
| `run_summary` | one run: totals, executions, defects, durations | `run_id` (required) |
| `defects` | defects recorded on executions, grouped by defect | – |
| `test_inventory` | tests by folder, type, status and category | `query` |
| `risks` | risk report: key figures (open, mitigated, accepted, closed, open per level, untested and verified mitigations, open risks not updated for 90 days), risks by level × status, open risks by mitigation state, the likelihood × impact heat map (risks not closed; every cell gives its count and level), the top 10 open risks, the risk register, risk → requirement → test traceability with coverage and latest results, open risks with untested mitigation, and open risks due for review | – |
| `compliance` | compliance with reference sources: by source, by clause, missing catalogue requirements, requirements not fulfilled (see *Compliance*), waivers and deviations | `sources` (comma-separated source ids; default: the sources the project uses) |
| `soa` | Statement of Applicability: key figures, applicability by control group, every control with applicability, justification, implementation status, owner, requirements and evidence (see *Statement of Applicability*) | `source` (default `iso-27001`) |
| `release` | release readiness: go / no-go, every exit criterion with what keeps it from being met and what a waiver or sign-off accepts, the waivers in force, the risks above the acceptance threshold | `release_id` (default: the newest planned or active release) |

The risk report's *mitigation state* of a risk is derived from the
requirements that address it: `no_requirements`, `untested` (none has
linked tests), `partly_tested` (some have), `tested` (all have, not all
passed) or `verified` (all linked tests of all its requirements passed).
The heat map is shown before mitigation (`heat_map`, the inherent score)
and after it (`heat_map_residual`, the residual score in effect);
`above_threshold` lists the risks whose residual level is above the
project's acceptance threshold with their sign-off, and the register adds
the residual assessment, the threshold flag, the acceptance and the review
date and owner. "Due for review" lists the open risks whose `updated_at` is
90 days old or more. Recipients of an e-mailed
risk report need requirement.read and test.read.

Custom report config (`CustomConfig`):

```json
{"entity": "requirement|test|run|execution", "query": "status:approved",
 "columns": ["key","title","status"], "group_by": "status",
 "sort_by": "key", "sort_desc": false, "include_summary": true,
 "title": "Approved requirements", "run_id": "(execution only)"}
```

**Report language.** Every text a report adds – its title, section
names, column headers, metric names, enumeration values (statuses,
results, coverage, obligation, priority, types, severities), the
"Generated" line, page numbers and "no data" notes – is produced in English
(`en`, the default) or Swedish (`sv`); the project's own content (titles,
names, notes, tags, …) is never translated. The language is chosen by,
in this order: the request's `lang` field (body), the `lang` query
parameter, the user's preferred language (`preferences.language`), the
browser's `Accept-Language` header, English. Tags like `sv-SE` are
accepted; unsupported languages are ignored at each step. The web client
sends its current UI language as `lang` with every report request. In
English, custom reports keep the stored enumeration values (`approved`);
in Swedish they show the labels of the UI (`Godkänt`). The e-mail of an
e-mailed report is in the report's language.

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/projects/{pid}/reports/fields?entity=` | export | available columns `[{id, label}]` |
| POST | `/projects/{pid}/reports/generate?lang=` | export | `{template?, params?, custom?, format: pdf\|docx\|xlsx\|csv\|md, archive?: bool, name?, lang?: en\|sv}`; without `archive` the file is downloaded, with `archive: true` it is stored in the document archive (`kind: report`) → 201 File |
| GET | `/projects/{pid}/reports/definitions` | export | saved custom reports |
| POST | `/projects/{pid}/reports/definitions` | requirement.write or test.write | `{name, description, config}` |
| GET / PATCH / DELETE | `/reports/definitions/{id}` | export / write | |
| POST | `/reports/definitions/{id}/generate?lang=` | export | `{format, archive?, name?, lang?}` |
| POST | `/projects/{pid}/reports/batch?lang=` | export (archive also needs requirement.write or test.write) | `{"templates":[{"template","params"}], "definitions":[ids], "format", "archive", "lang"?}` (at most 50; every report in `lang`). Without `archive` → one `application/zip` with every report (any failure → error, nothing sent); with `archive` → `{"files":[File], "failed":[{name, error}]}` |
| DELETE | `/projects/{pid}/reports/definitions/bulk` | requirement.write or test.write | `{"ids":[...]}` → `{"deleted": n}` |
| GET | `/projects/{pid}/reports/recipients` | export | members who may read the project `{"items":[{user_id, email, display_name, role}], "addresses_allowed": bool, "email_enabled": bool}` |
| POST | `/projects/{pid}/reports/email?lang=` | export | `{template + params? \| custom \| definition_id, format, recipients: [user ids], addresses?: [...], message?, lang?}` → 202 `{"queued": n, "skipped": [{user_id, reason}]}`. The report is rendered now and e-mailed in the background as an attachment. Members who may not read what the report shows are skipped (`no_access`, `disabled`); `addresses` (other e-mail addresses) need project.update (403). At most 50 recipients; 30 report e-mails per user in 10 minutes (429). A failed delivery becomes an in-app notification (`report.email_failed`) of the sender |
| GET | `/projects/{pid}/reports/subscriptions` | export | `{"items":[ReportSubscription], "email_enabled"}` |
| POST | `/projects/{pid}/reports/subscriptions?lang=` | export + (requirement.write or test.write) | `{definition_id, frequency: daily\|weekly\|monthly, weekday?: 0–6 (0 = Sunday), day_of_month?: 1–28, time: "HH:MM", timezone: IANA name, format, recipients: [user ids], addresses?, message?, enabled?, lang?}` → 201; the caller is the owner. Not with an API token. `lang` (body, else query parameter) is stored with the subscription |
| PATCH | `/reports/subscriptions/{id}?lang=` | owner or project.update (and requirement.write or test.write) | the same fields except `definition_id`; the next run is computed again; a supported `lang` replaces the stored language, otherwise it is kept |
| DELETE | `/reports/subscriptions/{id}` | owner or project.update | |
| POST | `/reports/subscriptions/{id}/send` | owner or project.update | sends the report now and waits for the mail server → the subscription with `last_sent_at` / `last_error` (the schedule is unchanged) |

`ReportSubscription` = `{id, project_id, definition_id, definition_name,
owner_id, owner_email, frequency, weekday, day_of_month, time, timezone,
format, recipients, addresses, message, lang, enabled, next_run_at, last_run_at,
last_sent_at, last_error, created_at, updated_at}`. `lang` is the language
the report and its e-mail are sent in: the UI language of whoever created
or last changed the subscription; `""` (subscriptions without one) uses
the owner's preferred language, else English. A scheduled report is
generated with the owner's permissions at the time it is sent (the owner
must still be an enabled member with `export`; outside addresses are only
used while the owner holds project.update) and goes to the recipients who
may still read it. A subscription that missed runs while the server was
down is sent once, then the next run is computed from that moment.
Deleting the saved report deletes its subscriptions.

## E-mail

E-mail is configured in the `email` section of the server configuration
(see the README). Without it, `/meta` reports `"email_enabled": false`
and the e-mail endpoints answer 400 `e-mail is not configured`. Messages
are sent in the background after the request's transaction is committed
(never for a request that fails), with the configured timeout; failures
are logged.

- **Notification e-mails**: `immediate` mails each notification with a
  link to the item; `digest` sends one e-mail a day (at `email.digest_hour`
  UTC) with the unread notifications that were not mailed yet. Every
  notification e-mail links to the profile (`/profile#email`) to change
  the setting, also as `List-Unsubscribe` header.
- **Invitations**: the link `/invite/{token}` is valid for 7 days and can
  be used once; only a SHA-256 hash of the token is stored. Public
  endpoints (rate limited per client address):

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/invitations/{token}` | public | `{email, display_name, tenant_name, role, invited_by, expires_at, sso, local}`; 404 for an unknown, used or revoked link, 410 when it expired |
| POST | `/invitations/{token}/accept` | public | `{"display_name","password"}` (password policy as everywhere) sets name and password, closes the invitation and signs in → the same answer as `/auth/login` (`Me`, session cookie). 400 for single sign-on invitations: those are accepted by signing in with the identity provider |

Audit actions: `invitation.send`, `invitation.resend`, `invitation.revoke`,
`invitation.accept`, `email.test`, `report.email`,
`report_subscription.create/update/delete/send`.

## Search

`GET /search?q=<expr>&types=requirement,test,run&project_id=&limit=` –
searches the projects of the current tenant the caller may read (each kind
of item with its read permission in the project; see
[Project sharing](#project-sharing-per-project-access)).
`limit` defaults to 200 (max 1000); `total` counts all hits. Besides the
requirement, test and run fields, `project` (key or name) and `kind` can be
queried. Response:
```json
{"items":[{"type":"requirement","id","project_id","project_key","key","title","snippet","matched_fields":["description"]}],"total":n}
```

## Batch update (spreadsheet view)

`POST /projects/{pid}/requirements/batch` and `POST /projects/{pid}/tests/batch`
save different field values for several items in one transaction (the
spreadsheet view saves an edit, a paste or a fill-down with one call):

```json
{"items": [
  {"id": "…", "version": 3, "set": {"title": "Login", "priority": "high"}},
  {"id": "…", "version": 1, "set": {"tags": ["auth"], "status": "review"}}
]}
```

* 1–5000 items, each id once; `set` holds the fields to change, exactly as a
  `PATCH` of the item, and each item goes through the same checks:
  validation, the write permission (a review decision also needs the review
  permission; a reviewer may change `status` alone), history (action
  `update`), audit, suspect links, notifications and webhooks.
* All or nothing: when an item fails, nothing is saved and the answer is that
  item's error with its position: `{"error", "code", "index", "id"}` (e.g.
  400 for an invalid value, 403, 404 for an item that is not in the
  project).
* `version` (optional) is the version the change is based on. Versions are
  checked before anything is written: when any item was saved by someone
  else since, nothing is saved and the answer is **409** with code `stale`
  and every outdated item: `{"error", "code": "stale", "conflicts": [{"id",
  "key", "version", "current": <the saved item>}]}`. Without `version` the
  change is saved as it is (undo and redo do this).
* → `{"updated": n, "items": [{"id", "version", "updated_at"}]}` with the new
  versions.

## OSLC

An OSLC provider for Requirements Management 2.1 and Quality Management 2.1,
so that OSLC clients (IBM ELM / DOORS Next, Polarion, Codebeamer, custom
tools) can discover, query, create, update, delete and link requirements and
test cases. All paths are below `/api/v1/oslc/`. The administrator guide
(*Connecting other tools*) shows how to connect DOORS Next, Polarion and
Codebeamer.

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/oslc/rootservices` | none | Jazz-style discovery: `oslc_rm_v1:rmServiceProviders`, `oslc_qm_v1:qmServiceProviders` and `jd:oslcCatalogs` point to the catalog |
| GET | `/oslc/catalog` | authenticated | `oslc:ServiceProviderCatalog`, one `oslc:ServiceProvider` (and a catalog) per project the caller may read |
| GET | `/oslc/projects/{pid}/catalog` | `project.read` | the catalog of one project |
| GET | `/oslc/projects/{pid}` | `project.read` | `oslc:ServiceProvider` with an RM and a QM `oslc:Service`: query capability, selection dialog and (with the write permission) creation factory and creation dialog |
| GET | `/oslc/projects/{pid}/shapes/{requirement\|test}` | `project.read` | `oslc:ResourceShape` |
| GET | `/oslc/projects/{pid}/requirements` | `requirement.read` | query capability for `oslc_rm:Requirement` |
| POST | `/oslc/projects/{pid}/requirements` | `requirement.write` | creation factory → 201, `Location`, `ETag` |
| GET | `/oslc/projects/{pid}/requirements/{item}` | `requirement.read` | one requirement, by id or key; `ETag` = version |
| PUT | `/oslc/projects/{pid}/requirements/{item}` | `requirement.write` | update; `If-Match` → 412 when stale |
| DELETE | `/oslc/projects/{pid}/requirements/{item}` | `requirement.write` | to the trash; `If-Match` optional → 204 |
| … | `/oslc/projects/{pid}/tests`, `…/tests/{item}` | `test.read` / `test.write` | the same for `oslc_qm:TestCase` |
| GET | `/oslc/projects/{pid}/dialogs/{requirement\|test}/{select\|create}` | none (the page uses the session) | delegated selection and creation dialogs (HTML) |

**Formats.** Responses by the `Accept` header: `application/rdf+xml`
(default, also for `application/xml` and `*/*`), `text/turtle`,
`application/ld+json` (also `application/json`). Request bodies by
`Content-Type`: Turtle, RDF/XML (`rdf:Description` or typed nodes,
`rdf:parseType="Literal"` XHTML, `rdf:resource`) or JSON-LD (inline
`@context`, `@graph`) and the OSLC 2.0 JSON format; others → 415.
Responses carry `OSLC-Core-Version: 2.0`; errors in an RDF request are an
`oslc:Error` (`oslc:statusCode`, `oslc:message`).

**Resources.** Requirements: `dcterms:identifier` (key), `dcterms:title`,
`dcterms:description`, `dcterms:created`, `dcterms:modified`,
`dcterms:creator` (`foaf:Person`), `dcterms:subject` (tags),
`oslc:shortTitle`, `oslc:serviceProvider`, `oslc:instanceShape`,
`rdfs:seeAlso` (the web page), `oslc_rm:validatedBy` (tests), relations as
`qtf:<type>` on the source and the OSLC RM properties (`refines` →
`oslc_rm:elaborates` / on the target `oslc_rm:elaboratedBy`, `depends_on`
→ `oslc_rm:constrainedBy` / `oslc_rm:constrains`, `derived_from` →
`oslc_rm:satisfies` / `oslc_rm:satisfiedBy`, `relates_to` →
`dcterms:relation`), item links to URLs (`oslc_rm:implementedBy`,
`trackedBy`, `affectedBy` … by their label, others `dcterms:references`),
and `qtf:status`, `qtf:obligation`, `qtf:type`, `qtf:priority`,
`qtf:source`, `qtf:rationale`, `qtf:owner`, `qtf:version`, `qtf:folder`.
Test cases: the dcterms properties, `oslc_qm:validatesRequirement`, item
links (`oslc_qm:testsChangeRequest`, `relatedChangeRequest` …),
`qtf:status`, `qtf:priority`, `qtf:executionType`, `qtf:preconditions`,
`qtf:expectedResult`, `qtf:estimatedMinutes`, `qtf:version`,
`qtf:folder`. The `qtf:` namespace is `<base_url>/api/v1/oslc/vocab#`.

**Writes.** POST to the query base creates (`dcterms:title` required); PUT
changes the properties the body carries and leaves the others;
`oslc.properties=dcterms:title,oslc_rm:validatedBy` makes it a selective
update, clearing the listed properties the body leaves out. Writable:
`dcterms:title`, `dcterms:description` (an `rdf:XMLLiteral` is converted
from XHTML to Markdown), `dcterms:subject`, the `qtf:` properties above
(`qtf:folder`: a path, missing folders are created) and the link
properties: `oslc_rm:validatedBy` / `oslc_qm:validatesRequirement` replace
the trace links (needs `test.write`), the relation properties (both
directions) replace the relations of their type, and values that are not
resources of the project (a work item or requirement in another tool)
become item links to their URL labelled with the property
(`oslc_rm:implementedBy` …). With `If-Match` the ETag must be the current
version (412 otherwise, as for a PATCH of the web client). Every save goes
through the regular handlers: validation, review permissions, history,
audit, suspect links, notifications. DELETE moves the item to the trash.

**Queries.** `oslc.where` works on every property of the resources: terms
joined by `and`, operators `= != < > <= >=` and `in [v1, v2]`, values
quoted strings (a `^^xsd:dateTime` or `@lang` suffix is accepted), numbers
or `<iri>`; dates compare as times; of several values one must match (`!=`:
none may be equal). `oslc.select` limits the properties of the members
(`*` all; nested selections select the property), `oslc.orderBy`
(`+dcterms:title,-dcterms:modified`) sorts, `oslc.searchTerms` (`"brake"
"stop"`) searches identifier, title and description. `oslc.prefix`
declares further prefixes (`dc=<http://purl.org/dc/terms/>`). Results are
the members of the query base (`rdfs:member`) with their properties and an
`oslc:ResponseInfo` (`oslc:totalCount`, `oslc:nextPage`); `oslc.pageSize`
(default 100, at most 1,000) and `page` page through them. Nested terms →
400.

**Delegated dialogs.** `…/dialogs/{kind}/select` lists the project's items
with a search and check boxes, `…/create` a short form. They use the
browser's Qtestframe session (without one they offer to sign in in another
tab) and send `{"oslc:results": [{"oslc:label": "<key> <title>",
"rdf:resource": "<resource URL>"}]}` by `postMessage` (`oslc-response:`
prefix, to the opener or parent) or, with `#oslc-core-windowName-1.0`, by
the windowName protocol. They may be framed by the origins of
`server.frame_ancestors` (else the same origin only); English or Swedish by
`lang` or `Accept-Language`.

**Authentication.** An API token as `Authorization: Bearer <token>`, or —
for clients that only offer Basic authentication — as the password of
`Authorization: Basic` (any user name), on the OSLC paths only; a browser
session works too (with the CSRF token for writes). The token's role and
the project access rules apply: restricted projects are listed and served
only to their members (404 otherwise).

**Not supported:** OAuth 1.0a (use API tokens), resource previews
(compact), TRS, nested `oslc.where` terms and the OSLC Configuration
Management domain.

### ReqIF export

The document follows the ReqIF 1.2 schema
(`http://www.omg.org/spec/ReqIF/20110401/reqif.xsd`, `REQ-IF-VERSION` 1.0
as the standard prescribes); XHTML values use the prefix `xhtml:` bound to
`http://www.w3.org/1999/xhtml`, as DOORS, Polarion and ProR expect.

* `REQ-IF-HEADER`: `REQ-IF-TOOL-ID` `Qtestframe`, `SOURCE-TOOL-ID`
  `Qtestframe <version>`, title `<KEY> – <name>`, creation time and a
  comment with the filter.
* `DATATYPES`: string, XHTML, integer, date and the enumerations
  `Obligation` (`MUST` … `DESCRIBE`), `Type`, `Priority`, `Status`. Each
  `ENUM-VALUE` has a readable `LONG-NAME` and the model code as
  `EMBEDDED-VALUE/@OTHER-CONTENT`.
* `SPEC-OBJECT-TYPE` *Requirement* with the attributes `ReqIF.ForeignID`
  (key), `ReqIF.Name` (title), `ReqIF.Text` (description), `Obligation`,
  `Type`, `Priority`, `Status`, `Source`, `Rationale`, `Owner`, `Tags`,
  `Categories` (`Dimension/Value`, comma separated), `Acceptance criteria`
  (XHTML list of `method: text (threshold: …)`), `Version`,
  `ReqIF.ForeignCreatedBy`, `ReqIF.ForeignCreatedOn`, one attribute per
  requirement custom field (`LONG-NAME` the label, REAL, BOOLEAN, DATE,
  ENUMERATION with the options, else STRING) and, in `.reqifz`,
  `Attachments` (`<xhtml:object data="files/…">` per file). The
  description is converted from Markdown (paragraphs, line breaks, lists,
  headings, code blocks, bold, italics, code, links and images; images of
  exported attachments point into the archive).
* Folders are heading objects: `SPEC-OBJECT-TYPE` *Heading* with
  `ReqIF.ChapterName`. One `SPECIFICATION` holds the folder tree as
  `SPEC-HIERARCHY`: below a heading come the folder's requirements, then
  its subfolders. With a filter only folders that contain exported
  requirements are included.
* Requirement relations between exported requirements are `SPEC-RELATION`s
  (source → target) with one `SPEC-RELATION-TYPE` per relation type
  (`Refines`, `Depends on`, …).
* With `include=tests`: `SPEC-OBJECT-TYPE` *Test case* with
  `ReqIF.ForeignID`, `ReqIF.Name`, `ReqIF.Text`, `Preconditions`, `Steps`
  (an XHTML table Action | Data | Expected result | Calls), `Expected
  result`, `Execution type`, `Priority`, `Status` (enumerations),
  `Estimated minutes`, `Tags`, `Version`, created by / on, the test custom
  fields and `Attachments`; a second `SPECIFICATION` holds the test folder
  tree; trace links are `SPEC-RELATION`s of the type *Verifies* (test →
  requirement, `qtf-link-<id>`). With a filter only the tests that verify
  an exported requirement are included.
* With `include=records`: `SPEC-OBJECT-TYPE` *Verification record*
  (requirement, criterion, method, result, performed by / on, note, valid
  until, validity, signed off by) in a third `SPECIFICATION`, each related
  to its requirement by a *Verification record of* relation. The importer
  ignores them.
* `baseline=<id>`: the items as the baseline froze them (with their
  custom fields, criteria and steps); folders, relations, links and records
  are today's between those items. The title and comment name the
  baseline.
* `IDENTIFIER`s are derived from the database ids (`qtf-req-<id>`,
  `qtf-folder-<id>`, `qtf-rel-<id>`, …), so repeated exports keep them;
  `LONG-NAME` of a requirement is its key, of a heading the folder name;
  `LAST-CHANGE` is the item's last update.

The ReqIF import with re-import (`/projects/{pid}/exchange/reqif/…`, see
*Exchange*) reads everything back except categories, records and the
creation data: requirements and tests with their fields, custom fields,
criteria, steps, folders, relations, trace links and attachments. The simple
requirement import (`POST /projects/{pid}/requirements/import`) restores
the requirements' title, description, obligation, type, priority, status,
source, rationale, owner, tags, criteria and folders; the key becomes the
external ID (`ID: <key>` in the source).

## OpenAPI description

`GET /api/v1/openapi.json` (and `/api/v1/openapi.yaml`, no authentication)
serves an OpenAPI 3.1 description of every route: parameters, the main
request and response bodies, the security schemes (session cookie with
`X-CSRF-Token`, bearer tokens) and the permission of each operation
(`x-permission` and in the description). Its source is
[`docs/openapi.yaml`](openapi.yaml); a test fails when a route is added
without describing it. View it with any OpenAPI viewer, for example by
opening the JSON URL in Swagger UI, Redoc or an editor plug-in, or generate
a client from it.

## Query language

Used by `q` parameters (filtering) and `/search`.

```
status:approved priority=high              implicit AND
obligation:must OR obligation:shall
NOT status:obsolete      -status:obsolete   negation
title~/^login.*(fail|error)/i              regular expression (RE2)
description!~/TBD/                         regex not matching
"exact phrase"                             free text over all fields
/time[- ]?out/                             regex over all fields
key:*-REQ-1*                               glob (* and ?) over the whole value
version>=2 created_at<2026-01-01           numeric / date comparison
category:Negative  category:"Network based"
has:criteria   (or field:* )               field not empty
(a OR b) AND c                             grouping
```

Operators: `:` contains (case-insensitive; glob when value has `*`/`?`,
matched against the whole value), `=` equals (case-insensitive), `!=`, `~`
regex (RE2, unanchored), `!~`, `>`, `>=`, `<`, `<=` (numeric when both
sides are numbers, otherwise string comparison). Keywords `AND`, `OR`,
`NOT` are case-insensitive. Unknown fields are an error. Full grammar and
semantics: [design.md §9](design.md#9-query-language).

Requirement fields: `id, key, title, description, obligation, type, priority,
status, source, rationale, owner, tag, version, folder, category, dimension
(matches "Dimension/Name"), coverage, linked, created_by, created_at,
updated_at, criteria, catalog` (the [catalogue](#catalogue) id it was copied from),
`blocked`, `stale` and `suspect_relations` (counts, see
[change and defect correlations](#change-and-defect-correlations)),
and `verification` (alias `verified`: `verified`, `partial`, `failed`, `pending`,
`none`) and `element` (alias `elements`: the name or path of a system element
the requirement is allocated to, or of one above it, e.g. `element:"Power unit"`).

Test fields: `id, key, title, description, preconditions, execution_type (alias
type), script_language, script, expected, step (actions/expected/data of
steps), priority, status, estimate, owner, tag, folder, category, dimension, linked,
last_result, created_by, created_at, updated_at, flaky (true/false),
flakiness (alias flips), catalog, stale` (true/false) and `element` (as for requirements).

Requirements and tests also have `variant` (alias variants: the names of the
variants the item applies to) and `applicability` (`all` | `selected`), see
[Variants and applicability](#variants-and-applicability).

Run fields: `id, name, description, status, environment (alias env), build,
created_by, created_at`. Risk fields (`GET /projects/{pid}/risks?q=`): `id,
key, title, description, category, likelihood, impact, score, level, status,
mitigation, owner, review_owner, review_date, created_by, catalog, catalog_area`
(alias `area`), `attack` (aliases `technique`, `atlas`: the technique ids
and names, so `attack:T1110` also finds `T1110.004` and
`attack:"credential stuffing"` works) and `tactic` (the ids and names of
the tactics of the techniques: `tactic:TA0006`, `tactic:"initial access"`).

Owners and assignees: `owner:me` (also `owner=me`, `owner!=me`) and
`assignee:me` (defects, executions) match the items of the caller, however
they are named in the field (e-mail address, display name or @handle), and
not merely values containing "me"; `review_owner:me` likewise for risks.
`owner:anna@example.com` finds the items of one member. An owner is a
project member stored as their e-mail address; writes accept the display
name, the e-mail address, `@handle` or the user id of a member and store
the e-mail address (a member who may not see the project is refused with
400), any other name stays as free text. Changing a member's e-mail address
moves their owner and assignee references along.

Custom fields: every list with custom fields (requirements, tests, runs,
defects, risks) accepts `cf.<key>` with every operator (`cf.supplier:"Acme"`,
`cf.mass>2`, `has:cf.mass`); numbers compare numerically, checkboxes are
`true`/`false`, multi-select fields have one value per option. A key the
project does not have matches nothing.

Sorting: `sort:<field>`, `sort:-<field>` or `sort:<field>:desc` (any field
of the schema, `cf.<key>` included) orders the result of the list endpoints;
several sort terms apply in order, numbers sort numerically, items without a
value come last. Sort terms combine with AND only (inside `OR` or `NOT` → 400). Execution fields: `id, key, title, status, assignee,
executed_by, notes, output`. Further aliases: `tags`, `categories`, `desc`,
`criterion` (requirements); `steps`, `language`, `result` (tests).
