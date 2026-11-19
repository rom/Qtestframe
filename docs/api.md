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
* **Errors** – non-2xx responses have the body
  `{"error": "<message>", "code": "<machine_code>"}`. Codes: `bad_request`
  (400), `unauthorized` (401), `forbidden` (403), `not_found` (404),
  `conflict` (409, also for duplicates and a disabled runner),
  `invalid_query` (400), `rate_limited` (429, login), `unavailable` (503,
  `/readyz`), `internal` (500).
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
  "role": "admin|manager|tester|viewer|",
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
| GET | `/members` | member.read | members of current tenant |
| POST | `/members` | member.manage | `{"email","role","display_name"?,"password"?}` – creates the local user if it does not exist |
| PATCH | `/members/{user_id}` | member.manage | `{"role"}`; the last admin cannot be demoted (409) |
| DELETE | `/members/{user_id}` | member.manage | the last admin cannot be removed (409) |
| GET | `/rbac` | any | `{"roles":[...],"matrix":{role:[perm]}}` |
| GET | `/tokens` | token.manage | own tokens in tenant |
| POST | `/tokens` | token.manage | `{"name","role","expires_days"?}` → `{"token":"qtf_...", "item": APIToken}` (token only shown once; role defaults to and cannot exceed own role; a token cannot create tokens) |
| DELETE | `/tokens/{id}` | token.manage | |
| GET | `/audit?limit=&q=` | audit.read | audit log of tenant, newest first (`limit` default 500, max 5000; query fields `action, entity, entity_id, user, details, created_at`) |
| GET | `/meta` | any | enumerations: `{"obligations","requirement_statuses","requirement_types","priorities","test_statuses","script_languages","evaluation_methods","run_statuses","result_statuses","roles"}` plus `"runner_enabled"` and `"version"` |

## Projects

| Method | Path | Permission |
|---|---|---|
| GET | `/projects` | project.read |
| POST | `/projects` | project.create – `{"key","name","description","seed_categories":true}` |
| GET | `/projects/{pid}` | project.read |
| PATCH | `/projects/{pid}` | project.update – `{"name","description"}` |
| DELETE | `/projects/{pid}` | project.delete |
| GET | `/projects/{pid}/dashboard` | project.read |

Dashboard response:
```json
{
  "requirements": {"total": 10, "by_status": {"draft": 3}, "by_obligation": {"must": 4}, "covered": 7, "uncovered": 3},
  "tests": {"total": 20, "by_execution_type": {"manual": 12, "automated": 8}, "by_status": {}},
  "coverage": {"passed": 5, "failed": 1, "not_run": 1, "none": 3, "partial": 0},
  "runs": {"total": 3, "by_status": {}},
  "recent_runs": [TestRun],
  "results": {"passed": 30, "failed": 2, ...}
}
```

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
| DELETE | `/folders/{id}?cascade=true` | without cascade children/items are moved to the parent |
| GET | `/projects/{pid}/categories?scope=` | list |
| POST | `/projects/{pid}/categories` | `{"scope","dimension","name","color","description"}` |
| PATCH | `/categories/{id}` | same fields |
| DELETE | `/categories/{id}` | |
| POST | `/projects/{pid}/categories/move` | `{"scope","dimension","new_dimension"}` – rename/move a whole category dimension |

Default categories seeded for new projects (scope `test`): dimension
`Polarity` (Positive, Negative), `Interface` (External, Internal), `Medium`
(File based, Network based), `Execution` (Manual, Automated), `Level` (Unit,
Integration, System, Acceptance); scope `requirement`: `Area` (Core, UI,
Security, Operations).

## Requirements

Reading needs `requirement.read`, writing `requirement.write`;
`/reqlang/analyze` is open to any authenticated user.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/requirements?q=&folder_id=&recursive=true` | list `Requirement` (without criteria) incl. derived `folder_path`, `linked_tests`, `coverage`; `folder_id=root` (or `none`) lists unfiled items |
| POST | `/projects/{pid}/requirements` | Requirement fields (`title` required; `obligation` auto-detected from title + description when empty); `criteria` array may be supplied. `create_test` (bool, default = server setting `requirements.auto_create_test`, which defaults to `true`) also creates a linked draft test; the response then contains `linked_test` (TestCase) |
| GET | `/requirements/{id}` | includes `criteria` |
| PATCH | `/requirements/{id}` | partial update; `criteria` (if present) replaces all criteria; bumps `version` |
| DELETE | `/requirements/{id}` | |
| POST | `/projects/{pid}/requirements/move` | `{"ids":[...],"folder_id": id\|null, "position"?: n, "project_id"?: id}`; another `project_id` re-keys the items, remaps categories by dimension/name and drops their links ([design.md §5](design.md#5-move-semantics)) |
| POST | `/projects/{pid}/requirements/bulk` | `{"ids":[...], "set": {"status"?, "priority"?, "obligation"?, "add_category_ids"?, "remove_category_ids"?}}` |
| DELETE | `/projects/{pid}/requirements/bulk` | `{"ids":[...]}` |
| GET | `/requirements/{id}/links` | `[TraceLink]` with test keys |
| POST | `/reqlang/analyze` | `{"text"}` → `Analysis` (below) |

`Analysis`:
```json
{
  "obligation": "must",
  "keywords": [{"keyword":"MUST","level":"must","offset":10,"length":4}],
  "issues": [{"rule":"ambiguous-term","severity":"warning","message":"...","offset":0,"length":4}],
  "score": 85
}
```

## Test cases

Reading needs `test.read`, writing `test.write`.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/tests?q=&folder_id=&recursive=true` | list (`steps` omitted) incl. `linked_requirements`, `last_result` |
| POST | `/projects/{pid}/tests` | TestCase fields, `steps` array |
| GET | `/tests/{id}` | includes `steps` |
| PATCH | `/tests/{id}` | partial; `steps` (if present) replaces all |
| DELETE | `/tests/{id}` | |
| POST | `/tests/{id}/copy` | duplicate → new TestCase |
| POST | `/projects/{pid}/tests/move` | like requirements |
| POST | `/projects/{pid}/tests/bulk` | like requirements (`status`, `priority`, `execution_type`, categories) |
| DELETE | `/projects/{pid}/tests/bulk` | |
| GET | `/tests/{id}/links` | `[TraceLink]` |

## Traceability

Reading needs `project.read`; creating and deleting links needs
`test.write`.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/links` | all links |
| POST | `/projects/{pid}/links` | `{"requirement_id","test_case_id","criterion_id"?}` |
| DELETE | `/links/{id}` | |
| GET | `/projects/{pid}/traceability?q=&tq=` | `{"requirements":[{id,key,title,obligation,coverage}], "tests":[{id,key,title,last_result}], "links":[TraceLink]}`; `q` filters requirements, `tq` tests |

## Operations (runs and executions)

Permissions: reading `run.read`; creating, updating, deleting runs and
adding tests `run.write`; recording results (`PATCH /executions/{id}`, CI
import) `run.execute`.

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/runs?q=` | list `TestRun` with `summary` |
| POST | `/projects/{pid}/runs` | `{"name","description","environment","build","test_ids":[...],"folder_ids":[...],"query":"<filter expr>"}` – union of explicit tests, tests in folders (recursive) and tests matching query |
| GET | `/runs/{id}` | `TestRun` |
| PATCH | `/runs/{id}` | `{"name","description","status","environment","build"}` |
| DELETE | `/runs/{id}` | |
| GET | `/runs/{id}/executions?q=` | list `Execution` (no step results) |
| POST | `/runs/{id}/executions` | add tests: `{"test_ids":[...]}` |
| POST | `/runs/{id}/results` | CI import: `{"results":[{"test_key","status","duration_ms","output"}]}` → `{"updated":n,"unknown":["KEY"]}` |
| GET | `/executions/{id}` | `Execution` with `step_results` and embedded `test` (TestCase incl. steps & script) |
| PATCH | `/executions/{id}` | `{"status","notes","assignee","output","duration_ms","step_results":[{"id","status","actual"}]}` – status derived from steps when omitted and steps given |
| POST | `/executions/{id}/run-script` | run automated script server-side (permission `script.execute`; 400 if the test has no automated script, 409 if the runner is disabled) → execution with embedded `test` |

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

`GET /projects/{pid}/export?format=xlsx|csv|pdf|docx&aspects=requirements,criteria,tests,traceability,runs&q=`
(permission `export`)

* `format` also accepts `excel` and `word`. `aspects` defaults to all;
  unknown aspects are a 400. `q` is applied to requirements if it is a valid
  requirement query and to tests if it is a valid test query.
* CSV with more than one aspect is delivered as a ZIP of CSV files.
* Response has `Content-Disposition: attachment;
  filename="<KEY>-<YYYYMMDD-HHMMSS>.<ext>"`. Formula injection is
  neutralised in CSV; see [design.md §11](design.md#11-export-formats).

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
updated_at, criteria`.

Test fields: `id, key, title, description, preconditions, execution_type (alias
type), script_language, script, expected, step (actions/expected/data of
steps), priority, status, estimate, tag, folder, category, dimension, linked,
last_result, created_by, created_at, updated_at`.

Run fields: `id, name, description, status, environment (alias env), build,
created_by, created_at`. Execution fields: `id, key, title, status, assignee,
executed_by, notes, output`. Further aliases: `tags`, `categories`, `desc`,
`criterion` (requirements); `steps`, `language`, `result` (tests).
