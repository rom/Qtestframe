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
  (412, see *Concurrent edits*),
  `invalid_query` (400), `rate_limited` (429, login), `unavailable` (503,
  `/readyz`), `internal` (500).
* **Bulk requests are all or nothing** – every bulk endpoint (the `…/bulk`
  routes, bulk review, sign-offs and restore, link confirmation, execution
  and notification batches) runs in one database transaction. An error
  answer (4xx/5xx) means none of the changes was saved; items listed in
  `skipped` with a reason are part of a successful answer, so the others
  were saved. Webhooks for the changes are sent only after they are saved.
  Exceptions, because they call outside services: pushing and syncing
  defects with the tracker (`/defects/push`, `/defects/sync`), webhook
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
| POST | `/auth/login` | `{"email","password","tenant"?}` (tenant = slug); 401 on bad credentials, 429 after 10 attempts in 5 min per IP + e-mail, 403 if local login is disabled | `Me` + `qtf_session` cookie |
| POST | `/auth/logout` | deletes the session | 204 |
| GET | `/auth/me` | | `Me` |
| POST | `/auth/switch-tenant` | `{"tenant_id"}` | `Me` |
| POST | `/auth/password` | `{"current_password","new_password"}` (local accounts only) | 204 |
| GET | `/auth/oidc/{name}/login` | browser redirect | 302 |
| GET | `/auth/oidc/{name}/callback` | browser redirect | 302 → `/` |
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
  "csrf_token": "..."
}
```

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
| GET | `/projects` | project.read |
| POST | `/projects` | project.create – `{"key","name","description","color"?,"type"?,"tech_context"?,"seed_categories":true}` |
| GET | `/projects/{pid}` | project.read |
| PATCH | `/projects/{pid}` | project.update – `{"name","description","color","requirement_sources","type","tech_context"}` |
| POST | `/projects/{pid}/prompts` | project.read + the read permission of the items' kind – see [Prompts](#prompts-software-projects) |
| DELETE | `/projects/{pid}` | project.delete |
| GET | `/projects/{pid}/dashboard` | project.read – see [Dashboard and metrics](#dashboard-and-metrics) |
| GET | `/projects/{pid}/metrics/trends` | project.read – `?range=30\|90\|365` or `?since=YYYY-MM-DD` |
| GET | `/projects/{pid}/flaky-tests` | project.read – `?limit=n` |
| GET | `/projects/{pid}/requirement-sources` | requirement.read – `{"configured":[...], "custom":bool, "used":[{"source","count"}]}`: the project's list and the sources requirements use, most used first |

A `Project` has `color` (`#rrggbb` or `""` for the theme colour; anything
else → 400), `requirement_sources` (the list the requirement editor offers
for *Source*; at most 50 entries of at most 100 characters, trimmed and
de-duplicated case-insensitively) and `custom_sources` (false while the
project uses the defaults; sending `[]` or the default list returns to
them). `type` is one of `/meta` `project_types` (`general` by default,
`software`, `hardware`, `system`, `business`); `tech_context` (at most 8000
characters) describes the technology of a software project and is included
in prompts.

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

## Folders (groups) and categories

Reading needs `project.read`. Writing folders needs `requirement.write` or
`test.write` according to the folder's `kind`; writing categories needs
`project.update` or the write permission of the category's `scope`.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/folders?kind=requirement\|test` | flat list (client builds tree via `parent_id`) |
| POST | `/projects/{pid}/folders` | `{"kind","name","description","parent_id"}` |
| PATCH | `/folders/{id}` | `{"name","description"}` |
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
target_title, target_kind?, direction}` (keys of runs, files and reports are
their names). Deleting an item removes its links (the trash restores them);
moving requirements or tests to another project drops their internal links
and keeps web links.

## Catalogue

A built-in library of standard requirements and tests for software
development (security, privacy, performance, reliability, operations,
usability, accessibility, common functionality), in English and Swedish.
The texts are written for Qtestframe and cite the standards they are based
on (OWASP ASVS, WCAG, ISO/IEC 25010, NIST SP 800-63B …) in `refs` by name
and section, without copying them. The content lives in
`internal/catalog/data` (format: `internal/catalog/FORMAT.md`) and ships
with the binary.

Items are organised in **groups** (e.g. Security) of **areas** (e.g.
Authentication). Requirement ids are `<PREFIX>-NN` (`AUTH-01`), test ids
`<PREFIX>-TNN` (`AUTH-T01`); a requirement lists the catalogue tests that
verify it (`tests`), a test the requirements it verifies (`requirements`).
`lang` is `en` (default) or `sv`; a text missing in Swedish falls back to
English. List filters take comma-separated values.

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/catalog?lang=` | project.read | `{languages, lang, groups:[{id, name, requirements, tests, areas:[{id, prefix, group, name, description, requirements, tests}]}], requirements, tests, requirement_tags:[{tag, count}], test_tags:[…]}` – groups in display order with item counts |
| GET | `/catalog/requirements?lang=&area=&group=&q=&type=&obligation=&priority=&tag=&project_id=&not_in_project=` | requirement.read | `{"items":[{id, area, group, type, obligation, priority, tags, refs, tests, title, description, in_project?}]}`. `q`: all words must occur (title, id, tags, refs, description, rationale); ranked by relevance (exact id, whole phrase in the title, title, tags, refs, text), else catalogue order. With `project_id`, `in_project:[{id, key, title}]` lists the project's requirements with that `catalog_ref`; `not_in_project=true` leaves those out |
| GET | `/catalog/requirements/{id}?lang=&project_id=` | requirement.read | the summary plus `area_name, rationale, criteria:[{method, text, threshold?}], linked_tests:[{id, area, title, in_project?}]` |
| GET | `/catalog/tests?lang=&area=&group=&q=&priority=&tag=&execution=&project_id=&not_in_project=` | test.read | `{"items":[{id, area, group, priority, execution, tags, refs, requirements, steps (count), title, description, in_project?}]}`; `q` also searches the steps and the ids of the verified requirements |
| GET | `/catalog/tests/{id}?lang=&project_id=` | test.read | the summary plus `area_name, preconditions, expected, step_list:[{action, expected, data?}], linked_requirements:[{id, area, title, in_project?}]` |
| GET | `/projects/{pid}/catalog` | project.read | `{"requirements": {"AUTH-01": [{id, key, title}]}, "tests": {…}}` – the project's items by `catalog_ref` (a kind the caller may not read stays empty) |
| POST | `/projects/{pid}/catalog/import` | project.read + see below | copies catalogue items into the project, see below |

**Origin.** Requirements and tests have `catalog_ref`: the catalogue id they
were copied from, `""` otherwise. It is set only by the import, ignored in
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
  "source": "Standard"
}
```

- At most 500 items per request (chosen requirements and tests plus the
  included tests), else 400. The request is atomic: on an error nothing is
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
  "links": [{"id": "…", "requirement_id": "…", "requirement_key": "WEB-REQ-12", "test_case_id": "…", "test_case_key": "WEB-TC-30"}],
  "skipped": [{"kind": "requirement", "catalog_ref": "AUTH-02", "reason": "exists", "existing": [{"id": "…", "key": "WEB-REQ-3", "title": "…"}]}],
  "folders_created": 2
}
```

To undo an import, delete the created items with the bulk delete routes
(requirements first, then tests); restoring the trash entries in the
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
`duplicate_titles`, `conflicts`, `low_quality`, `many_drafts`, `no_owner` and
`language_rule` (with `params.rule`, a language analysis rule). At most 200
items per finding (`total` gives the full count).

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

TraceRuleResult: `{"rule":TraceRule, "matched": n, "violations":[{kind, id, key, title, found, required, total}], "violation_count": n, "passed", "error"?}`;
`found` counts the qualifying traces, `total` the traces before the
conditions (`not_suspect`, `passed`, `target_query`). The **Traceability**
report lists the enabled rules and their violations (sections
`trace_rules` and `trace_rule_violations`, evaluated on the report's
requirements and tests).

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
| GET | `/executions/{id}` | `Execution` with `step_results` (each with `iteration_id`), `iterations` (data-driven tests), `readiness` and embedded `test` (TestCase incl. steps & script) |
| PATCH | `/executions/{id}` | `{"status","notes","assignee","output","duration_ms","step_results":[{"id","status","actual","evidence"}],"iterations":[{"id","status"?,"notes"?}]}` – status derived from steps (or iterations) when omitted and steps or iterations given; 409 `not_ready` when the update records an execution that the settings forbid |
| POST | `/executions/{id}/run-script` | run automated script server-side (permission `script.execute`; 400 if the test has no automated script, 409 if the runner is disabled or the test may not be executed) → execution with embedded `test` |

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
| `theme` | `light`, `dark`, `system`, `spring`, `autumn`, `desert`, `camouflage`, `city_camouflage`, `navy`, `green_console`, `crazy` | colour theme (applied on every device) |
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
| GET | `/releases/{id}/status` | `ReleaseStatusReport {release, go, criteria:[{type, threshold, level, met, actual, total, items:[{kind,id,key,title,note}], available}], summary}` |
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

## Defects

Defects have keys like `SHOP-BUG-3`. Reading needs `project.read`; creating,
changing and linking `run.execute`; deleting `project.update`.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/defects?q=` | `[Defect]`; query fields `key, title, description, status, severity, priority, assignee, external, created_by, open, linked` |
| POST | `/projects/{pid}/defects` | `{"title","description","status","severity","priority","assignee","external_key","external_url","links":[{"entity_type":"requirement\|test\|execution","entity_id"}]}` |
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

**Jira and GitHub.** Each project can be connected to one tracker.

| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/projects/{pid}/defect-tracker` | project.read | `{type: ""\|jira\|github, url, project_key, repo, issue_type, labels, user, has_token}`; the token is never returned |
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
planned → `rejected`.

## Risks

Reading needs `requirement.read`, changing `requirement.write`.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/risks` | `[Risk]`, highest score first |
| POST | `/projects/{pid}/risks` | `{"title","description","category","likelihood":1-5,"impact":1-5,"status","mitigation","owner","requirement_ids":[...]}` (likelihood and impact default to 3) |
| GET / PATCH / DELETE | `/risks/{id}` | |
| POST | `/projects/{pid}/risks/bulk` | `{"ids":[...], "set": {"status"?, "owner"?, "category"?, "likelihood"?, "impact"?}, "add_requirement_ids"?, "remove_requirement_ids"?}` → `{"updated": n}`; every risk is validated before any is saved |
| DELETE | `/projects/{pid}/risks/bulk` | `{"ids":[...]}` → `{"deleted": n}` |
| POST | `/risks/{id}/requirements` | `{"add":[ids],"remove":[ids]}` |
| GET | `/requirements/{id}/risks` | the risks a requirement addresses |

`score` is likelihood × impact; `level` is `low` (1–4), `medium` (5–9),
`high` (10–16) or `critical` (17–25). Statuses: `identified`, `mitigating`
(open), `mitigated`, `accepted`, `closed`. `covered` and `verified` count
the linked requirements with tests and with all tests passed. Requirements
carry `risk_level`, the highest level of their open risks (query field
`risk`).

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
decision on one, and when a defect assigned to them changes status.

A webhook delivery is `POST <url>` with `Content-Type: application/json`,
headers `X-Qtestframe-Event`, `X-Qtestframe-Webhook` and
`X-Qtestframe-Signature: sha256=<hex HMAC-SHA256 of the body with the
secret>`, and body
`{"id","event","project_id","actor","time","data"}`. Events:
`requirement.created/updated`, `test.created/updated`,
`run.created/updated/completed`, `execution.updated`,
`defect.created/updated/status/assigned`, `risk.created/updated`,
`release.created/updated/released`, `comment.created`,
`review.requested/completed`, `baseline.signed`. Deliveries run in the
background with a 10 s timeout, are not retried and do not follow
redirects; the latest outcome is stored as `last_status`, `last_error`.

## Comments

| Method | Path | Notes |
|---|---|---|
| GET | `/requirements/{id}/comments`, `/tests/{id}/comments` | `[Comment]` oldest first: `{id, entity_type, entity_id, user_id, user_email, user_name, body, created_at, updated_at}` |
| POST | same | `{"body"}` (1–10000 chars); any member who can read the item |
| PATCH | `/comments/{id}` | `{"body"}`, author only |
| DELETE | `/comments/{id}` | author, or `member.manage` |

Requirement and test lists add `comment_count`.

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

Archive query fields: `name`, `description`, `tag`, `kind`, `type`
(content type), `uploaded_by`, `created_at`, `size`, `linked` (keys of
linked items).

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
| GET | `/audit?q=&from=&to=&limit=&format=csv` | audit.read | `from`/`to` are RFC 3339 or `YYYY-MM-DD`; `format=csv` downloads |
| GET | `/admin/backup` | super admin | ZIP: `manifest.json` + one `<table>.jsonl` per table (all tenants, files included; sessions excluded) |
| POST | `/admin/restore` | super admin | multipart `backup` (ZIP) + field `confirm=RESTORE`; the schema version must match; replaces **all** data in one transaction and signs everyone out → `{"restored": {table: rows}}` |

CLI equivalents: `qtestframe backup --out FILE.zip`,
`qtestframe restore --in FILE.zip --yes`.

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
params, formats}]`):

| id | Content | Params |
|---|---|---|
| `project_status` | KPIs, requirements by status/obligation, coverage, recent runs | – |
| `requirements_coverage` | every requirement with linked tests, last results and coverage | `query` |
| `traceability` | requirement × test links | `query` |
| `run_summary` | one run: totals, executions, defects, durations | `run_id` (required) |
| `defects` | defects recorded on executions, grouped by defect | – |
| `test_inventory` | tests by folder, type, status and category | `query` |

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
| POST | `/projects/{pid}/reports/generate?lang=` | export | `{template?, params?, custom?, format: pdf\|docx\|xlsx\|csv, archive?: bool, name?, lang?: en\|sv}`; without `archive` the file is downloaded, with `archive: true` it is stored in the document archive (`kind: report`) → 201 File |
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
searches all projects of the current tenant (permission `project.read`).
`limit` defaults to 200 (max 1000); `total` counts all hits. Besides the
requirement, test and run fields, `project` (key or name) and `kind` can be
queried. Response:
```json
{"items":[{"type":"requirement","id","project_id","project_key","key","title","snippet","matched_fields":["description"]}],"total":n}
```

## Export

`GET /projects/{pid}/export?format=xlsx|csv|pdf|docx|reqif|reqifz&aspects=requirements,criteria,tests,test_data,traceability,runs&q=`
(permission `export`)

* `format` also accepts `excel` and `word`. `aspects` defaults to all;
  unknown aspects are a 400. `q` is applied to requirements if it is a valid
  requirement query and to tests if it is a valid test query.
* CSV with more than one aspect is delivered as a ZIP of CSV files.
* `reqif` and `reqifz` export the (filtered) requirements as
  [ReqIF 1.2](#reqif-export); `aspects` does not apply. `reqif` is the XML
  document (`application/xml`), `reqifz` a ZIP (`application/zip`) with the
  document `<KEY>.reqif` and the requirements' attachments below
  `files/<requirement key>/`.
* `tests` has the columns `Parameters` and `Data rows` (count); `test_data`
  lists the data table, one line per row and parameter (`Test`, `Row`,
  `Row name`, `Parameter`, `Value`); `runs` has `Configuration` and
  `Iterations` per execution.
* Response has `Content-Disposition: attachment;
  filename="<KEY>-<YYYYMMDD-HHMMSS>.<ext>"`. Formula injection is
  neutralised in CSV; see [design.md §11](design.md#11-export-formats).

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
  `ReqIF.ForeignCreatedBy`, `ReqIF.ForeignCreatedOn` and, in `.reqifz`,
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
* `IDENTIFIER`s are derived from the database ids (`qtf-req-<id>`,
  `qtf-folder-<id>`, `qtf-rel-<id>`, …), so repeated exports keep them;
  `LONG-NAME` of a requirement is its key, of a heading the folder name;
  `LAST-CHANGE` is the item's last update.

Importing the file again (`POST /projects/{pid}/requirements/import`)
restores title, description (the importer reads the XHTML of a Qtestframe
file back as Markdown), obligation, type, priority, status, source,
rationale, owner, tags, criteria and folders; the key becomes the external
ID (`ID: <key>` in the source). Relations, categories and attachments are
not imported.

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
updated_at, criteria, catalog` (the [catalogue](#catalogue) id it was copied from).

Test fields: `id, key, title, description, preconditions, execution_type (alias
type), script_language, script, expected, step (actions/expected/data of
steps), priority, status, estimate, tag, folder, category, dimension, linked,
last_result, created_by, created_at, updated_at, flaky (true/false),
flakiness (alias flips), catalog`.

Run fields: `id, name, description, status, environment (alias env), build,
created_by, created_at`. Execution fields: `id, key, title, status, assignee,
executed_by, notes, output`. Further aliases: `tags`, `categories`, `desc`,
`criterion` (requirements); `steps`, `language`, `result` (tests).
