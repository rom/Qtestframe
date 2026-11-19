# REST API (v1)

All endpoints are served below `/api/v1` and exchange JSON (`Content-Type:
application/json`) unless stated otherwise. Field shapes are defined by the Go
types in `internal/model/model.go` (JSON tags are authoritative).

## Conventions

* **Authentication** – browser clients use the `qtf_session` cookie
  (HttpOnly, Secure, SameSite=Lax). Non-interactive clients send
  `Authorization: Bearer qtf_<token>` (API tokens, see below).
* **CSRF** – every state-changing request (POST/PUT/PATCH/DELETE) made with a
  session cookie must carry the header `X-CSRF-Token` with the value returned
  as `csrf_token` by `GET /auth/me` / `POST /auth/login`. Bearer-token
  requests are exempt.
* **Tenant** – the session has a *current tenant*; all project data is scoped
  to it. Switch with `POST /auth/switch-tenant`. Bearer tokens are bound to
  one tenant.
* **Errors** – non-2xx responses have the body
  `{"error": "<message>", "code": "<machine_code>"}`. Codes: `bad_request`,
  `unauthorized`, `forbidden`, `not_found`, `conflict`, `invalid_query`,
  `internal`.
* **IDs** are UUID strings. Times are RFC 3339.
* **Lists** return `{"items": [...], "total": n}`.
* **Filtering** – list endpoints for requirements, tests, runs, executions
  accept `?q=<filter expression>` (see *Query language*). Invalid expressions
  return 400 with code `invalid_query` and `{"error","position"}`.

## Auth

| Method | Path | Body / Notes | Response |
|---|---|---|---|
| GET | `/auth/providers` | public | `{"local":true,"oidc":[{"name","display_name","login_url"}],"saml":[{"name","display_name","login_url"}]}` |
| POST | `/auth/login` | `{"email","password","tenant"?}` (tenant = slug) | `Me` |
| POST | `/auth/logout` | | 204 |
| GET | `/auth/me` | | `Me` |
| POST | `/auth/switch-tenant` | `{"tenant_id"}` | `Me` |
| POST | `/auth/password` | `{"current_password","new_password"}` | 204 |
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
| PATCH | `/users/{id}` | super admin | `{"display_name","disabled","super_admin","password"}` |
| GET | `/members` | member.read | members of current tenant |
| POST | `/members` | member.manage | `{"email","role","display_name"?,"password"?}` – creates the local user if it does not exist |
| PATCH | `/members/{user_id}` | member.manage | `{"role"}` |
| DELETE | `/members/{user_id}` | member.manage | |
| GET | `/rbac` | any | `{"roles":[...],"matrix":{role:[perm]}}` |
| GET | `/tokens` | token.manage | own tokens in tenant |
| POST | `/tokens` | token.manage | `{"name","role","expires_days"?}` → `{"token":"qtf_...", "item": APIToken}` (token only shown once; role cannot exceed own role) |
| DELETE | `/tokens/{id}` | token.manage | |
| GET | `/audit?limit=&q=` | audit.read | audit log of tenant |
| GET | `/meta` | any | enumerations: `{"obligations","requirement_statuses","requirement_types","priorities","test_statuses","script_languages","evaluation_methods","run_statuses","result_statuses","roles"}` |

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

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/requirements?q=&folder_id=&recursive=true` | list `Requirement` (without criteria) incl. derived `folder_path`, `linked_tests`, `coverage` |
| POST | `/projects/{pid}/requirements` | Requirement fields (`title` required; `obligation` auto-detected from description when empty); `criteria` array may be supplied |
| GET | `/requirements/{id}` | includes `criteria` |
| PATCH | `/requirements/{id}` | partial update; `criteria` (if present) replaces all criteria; bumps `version` |
| DELETE | `/requirements/{id}` | |
| POST | `/projects/{pid}/requirements/move` | `{"ids":[...],"folder_id": id\|null, "position"?: n, "project_id"?: id}` |
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

| Method | Path | Notes |
|---|---|---|
| GET | `/projects/{pid}/links` | all links |
| POST | `/projects/{pid}/links` | `{"requirement_id","test_case_id","criterion_id"?}` |
| DELETE | `/links/{id}` | |
| GET | `/projects/{pid}/traceability` | `{"requirements":[{id,key,title,obligation,coverage}], "tests":[{id,key,title,last_result}], "links":[TraceLink]}` |

## Operations (runs and executions)

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
| PATCH | `/executions/{id}` | `{"status","notes","assignee","output","step_results":[{"id","status","actual"}]}` – status derived from steps when omitted and steps given |
| POST | `/executions/{id}/run-script` | run automated script server-side (permission `script.execute`; runner must be enabled) → `Execution` |

## Search

`GET /search?q=<expr>&types=requirement,test,run&project_id=` – searches all
projects of the current tenant. Response:
```json
{"items":[{"type":"requirement","id","project_id","project_key","key","title","snippet","matched_fields":["description"]}],"total":n}
```

## Export

`GET /projects/{pid}/export?format=xlsx|csv|pdf|docx&aspects=requirements,tests,runs,traceability,criteria&q=`

* `aspects` defaults to all. `q` filters requirements and tests.
* CSV with more than one aspect is delivered as a ZIP of CSV files.
* Response has `Content-Disposition: attachment`.

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
key:REQ-1*                                 glob (* and ?)
version>=2 created_at<2026-01-01           numeric / date comparison
category:Negative  category:"Network based"
has:criteria   (or field:* )               field not empty
(a OR b) AND c                             grouping
```

Operators: `:` contains (case-insensitive; glob when value has `*`/`?`),
`=` equals (case-insensitive), `!=`, `~` regex, `!~`, `>`, `>=`, `<`, `<=`.
Keywords `AND`, `OR`, `NOT` are case-insensitive. Unknown fields are an error.

Requirement fields: `key, title, description, obligation, type, priority,
status, source, rationale, owner, tag, version, folder, category, dimension
(matches "Dimension/Name"), coverage, linked, created_by, created_at,
updated_at, criteria`.

Test fields: `key, title, description, preconditions, execution_type (alias
type), script_language, script, expected, step (actions/expected/data of
steps), priority, status, estimate, tag, folder, category, dimension, linked,
last_result, created_by, created_at, updated_at`.

Run fields: `name, description, status, environment, build, created_by,
created_at`. Execution fields: `key, title, status, assignee, executed_by,
notes, output`.
