# Functional design

This document describes what Qtestframe does and the rules it applies. The
structure of the code is in [architecture.md](architecture.md), endpoint
details are in [api.md](api.md).

- [1. Concepts](#1-concepts)
- [2. Views](#2-views)
- [3. Requirement language](#3-requirement-language)
- [4. Classification: categories and folders](#4-classification-categories-and-folders)
- [5. Move semantics](#5-move-semantics)
- [6. Evaluation criteria and trace links](#6-evaluation-criteria-and-trace-links)
- [7. Coverage calculation](#7-coverage-calculation)
- [8. Runs and executions](#8-runs-and-executions)
- [9. Query language](#9-query-language)
- [10. Search](#10-search)
- [11. Export formats](#11-export-formats)
- [12. RBAC matrix](#12-rbac-matrix)
- [13. Change history, relations, baselines and collaboration](#13-change-history-relations-baselines-and-collaboration)
- [14. Profile and preferences](#14-profile-and-preferences)

## 1. Concepts

| Concept | Meaning |
|---|---|
| Tenant | An isolated organisation. Users are global; a membership gives a user one role in a tenant. |
| Project | A container with a key (`DEMO`, 2–16 characters `A-Z0-9`, starting with a letter) for requirements, tests, links and runs. |
| Requirement | A specification statement `<KEY>-REQ-<n>` with obligation level, type, priority, status, source, rationale, owner, tags, categories, evaluation criteria and a version counter. |
| Test case | A manual or automated test `<KEY>-TC-<n>` with preconditions, ordered steps (action, expected, data), expected result, optional script (sh, bash, python, javascript), estimate, tags and categories. |
| Trace link | A requirement–test pair, optionally narrowed to one evaluation criterion of the requirement. |
| Run | An execution campaign over a set of tests, with environment and build labels. |
| Execution | One test inside a run, with a result, step results, notes, output and duration. |

Enumerations are served by `GET /api/v1/meta` (from `internal/model/model.go`):

| Enumeration | Values |
|---|---|
| Requirement status | draft, review, approved, implemented, verified, rejected, obsolete |
| Requirement type | functional, non_functional, security, performance, usability, interface, constraint, business, regulatory |
| Priority | critical, high, medium, low |
| Test status | draft, ready, review, deprecated |
| Execution type | manual, automated |
| Run status | planned, in_progress, completed, aborted |
| Result status | not_run, in_progress, passed, failed, blocked, skipped |
| Evaluation method | test, inspection, analysis, demonstration, review |

## 2. Views

The SPA (`web/src/router.ts`) has these views. Each one is a thin client of
the REST API; all rules below are enforced on the server, and the client
hides actions the principal lacks permissions for (`web/src/lib/permissions.ts`).

| View | Route | Purpose and API used |
|---|---|---|
| Login | `/login` | Local e-mail/password form plus one button per OIDC/SAML provider (`GET /auth/providers`). |
| Projects | `/projects` | List, create (optionally seeding default categories), rename and delete projects. |
| Dashboard | `/p/:id/dashboard` | Chart.js charts from `GET /projects/{pid}/dashboard`: requirements by status and obligation, covered vs. uncovered, coverage states, tests by execution type and status, latest results, runs by status, the five newest runs. |
| Requirements | `/p/:id/requirements` | Folder tree, filterable list (query input with quick filters composed into a query string, `web/src/lib/query.ts`), editor drawer, bulk edit/delete, drag and drop of items onto folders (`web/src/lib/dnd.ts`). The editor calls `POST /reqlang/analyze` while typing and highlights keywords and lint issues in the text, with the quality score. |
| Tests | `/p/:id/tests` | Same workspace for test cases (shared `ItemWorkspace.vue`); the editor manages steps, execution type and script; tests can be copied. |
| Runs (operations) | `/p/:id/runs` | Run list with result summaries; create a run from selected tests, folders and/or a query. |
| Run detail | `/p/:id/runs/:runId` | Executions with status, assignee and notes; quick result buttons; add tests; change run status; a dialog with a ready-made `curl` call for CI result import. |
| Execution | `/p/:id/runs/:runId/exec/:executionId` | Step-by-step execution of a manual test (status and actual result per step, overall status derived), or server-side script execution for automated tests (`POST /executions/{id}/run-script`) when the runner is enabled. |
| Traceability | `/p/:id/traceability` | Requirement × test matrix (`GET /projects/{pid}/traceability`) with coverage per requirement and last result per test; filters for both axes; clicking a cell creates or deletes a link. |
| Settings | `/p/:id/settings` | Project name/description, category management (create, recolour, rename, move a whole dimension), project deletion. |
| Export | `/p/:id/export` | Choose format, aspects and an optional filter, then download. |
| Search | `/search` | Cross-project search over requirements, tests and runs with snippets and highlighted terms. |
| Admin | `/admin/*` | Members (`member.read`/`member.manage`), API tokens (`token.manage`), audit log (`audit.read`); tenants and users for super admins. |
| Profile | `/profile` | Own account, tenant switch, password change for local accounts. |

## 3. Requirement language

`internal/reqlang` analyses requirement text. `POST /api/v1/reqlang/analyze`
returns the result; on creation of a requirement without an explicit
`obligation`, the server sets it to `DetectObligation(title + "\n" + description)`.
Updates never re-detect it.

### Obligation levels and keyword detection

Keywords follow RFC 2119; per RFC 8174 only the UPPERCASE forms carry
normative meaning. Matching is case-insensitive, multi-word keywords allow
any whitespace between words, matches must be word-bounded, longer phrases
win over shorter ones and matches do not overlap.

| Keyword(s) | Level |
|---|---|
| `MUST NOT` | `must_not` |
| `MUST`, `REQUIRED` | `must` |
| `SHALL NOT` | `shall_not` |
| `SHALL` | `shall` |
| `SHOULD NOT`, `NOT RECOMMENDED` | `should_not` |
| `SHOULD`, `RECOMMENDED` | `should` |
| `MAY` | `may` |
| `OPTIONAL` | `optional` |
| `WILL` | `will` (statement of fact/intent, not RFC 2119) |
| `DESCRIBE`, `DESCRIBES`, `INFORMATIVE`, `NOTE:` | `describe` (informative text) |

The obligation of a text is its **strongest** keyword, in the order
`must_not > must > shall_not > shall > should_not > should > may > optional > will > describe`
(`reqlang.Levels`). Text without keywords has obligation `""`; the dashboard
counts such requirements as `unspecified`.

### Lint rules

Sentences are split at `.`, `!` or `?` followed by whitespace or the end of
text, and at line breaks.

| Rule | Severity | Triggered by |
|---|---|---|
| `missing-keyword` | warning | no obligation keyword anywhere in the text |
| `lowercase-keyword` | info | a keyword that is not all uppercase (`must` instead of `MUST`) |
| `negative-requirement` | info | `MUST NOT`, `SHALL NOT` (harder to verify) |
| `multiple-obligations` | warning | one sentence contains more than one distinct non-`describe` keyword |
| `long-sentence` | info | a sentence with more than 40 words |
| `ambiguous-term` | warning | vague words such as fast, easy, user-friendly, simple, efficient, adequate, appropriate, approximately, about, flexible, robust, reliable, intuitive, minimal, maximize, sufficient, etc., as needed, normally, typically, usually, seamless, scalable, optimal, best, several, some, many, few, large, small, significant, reasonable, timely, acceptable, as soon as possible, various, similar, suitable (full list: `reqlang.AmbiguousTerms`) |
| `tbd` | error | `TBD`, `TBC`, `TODO`, `XXX`, `???` |
| `unbounded-term` | info | all, always, never, every, any, none |
| `and-or` | warning | `and/or` |
| `passive-capability` | info | `be able to`, `be capable of` |
| `escape-clause` | warning | if possible, where applicable, unless, except |

Each finding has a byte `offset` and `length` into the analysed text, used
by the editor for highlighting.

**Score:** `100 − 25 × errors − 10 × warnings − 2 × infos`, floored at 0.

Example: `The system shall respond fast and/or retry TBD.` gives obligation
`shall` and the issues `lowercase-keyword` (info), `ambiguous-term` for
"fast" (warning), `and-or` (warning) and `tbd` (error), so the score is
100 − 25 − 20 − 2 = 53.

## 4. Classification: categories and folders

Requirements and tests are classified along two independent axes.

### Categories (dimension / value)

A category is a value inside a **dimension**, scoped to one item kind:
`(project, scope ∈ {requirement, test}, dimension, name)` is unique, with a
colour and description. An item may carry any number of categories,
including several of the same dimension. Dimension names must not contain
`/`, because the query field `dimension` matches the combined form
`Dimension/Name`.

New projects are seeded (unless `seed_categories: false`) from
`store.DefaultCategories`:

| Scope | Dimension | Values |
|---|---|---|
| test | Polarity | Positive, Negative |
| test | Interface | External, Internal |
| test | Medium | File based, Network based |
| test | Execution | Manual, Automated |
| test | Level | Unit, Integration, System, Acceptance |
| requirement | Area | Core, UI, Security, Operations |

Rules:

* `category_ids` on an item replace its assignments; every ID must belong to
  the same project and scope (otherwise 400). Bulk endpoints add or remove
  category IDs on many items.
* **Rename or move a dimension** (`POST /projects/{pid}/categories/move`):
  all categories of `dimension` in the scope are moved to `new_dimension`.
  If the target already has a value with the same name, assignments are
  merged into the existing category and the duplicate is deleted.
* Deleting a category removes its assignments.
* Editing categories requires `project.update`, or write access to the
  scope's items (`requirement.write` / `test.write`).

### Folders (groups)

Folders form a tree per project and kind (`requirement` or `test`). Each
item lives in at most one folder (`folder_id`, `null` = root). Siblings and
items are ordered by `position`.

* The derived `folder_path` (`A / B / C`) is returned with items and used by
  the `folder` query field.
* Listing with `folder_id=<id>` includes descendants unless
  `recursive=false`; `folder_id=root` (or `none`) selects unfiled items.
* Deleting a folder moves its child folders and items to the parent folder,
  or, with `?cascade=true`, deletes the subtree and all items in it.
* Writing folders requires `requirement.write` or `test.write` according to
  the folder's kind.

## 5. Move semantics

Implemented in `internal/store/project.go` (`MoveFolder`) and
`internal/store/requirement.go` (`MoveItems`, `moveItemsToProject`). Each
move runs in one transaction.

### Items (`POST /projects/{pid}/requirements/move`, `.../tests/move`)

Body: `ids`, `folder_id` (`null` = root), optional `position`, optional
`project_id`.

* All IDs must be items of the source project in the principal's tenant
  (otherwise 404). The target folder must belong to the target project and
  the same kind (otherwise 400).
* **Ordering:** the remaining items of the target folder keep their order;
  the moved items are inserted as one block, in request order, at index
  `position` (missing, negative or too large = append). Positions of the
  target folder are renumbered `0..n-1`.

**Within a project** only `folder_id` and `position` change. Keys, links,
categories and history are untouched.

**Across projects** (`project_id` differs from `{pid}`, same tenant), for
each item in addition:

1. **Re-keying:** the item gets the next key of the target project
   (`TGT-REQ-17`); the old key is not reused in the source project.
2. **Category remap:** each assignment is mapped to the target project's
   category with the same scope, dimension and name. Assignments without a
   counterpart are **dropped**; categories are never created implicitly.
3. **Links dropped:** all trace links of the item are deleted, because links
   never cross project boundaries.
4. Evaluation criteria and test steps move with the item (they belong to it).
   Executions in runs of the source project keep pointing at the test, but
   the target project's last-result and coverage computation only considers
   runs of the target project.

### Folders (`POST /folders/{id}/move`)

Body: `parent_id` (`null` = root), optional `position`, optional
`project_id`.

* **Cycle prevention:** the new parent must not be the folder itself or one
  of its descendants (`Descendants`), otherwise **409 conflict**.
* The parent must have the same kind and belong to the target project.
* The folder is inserted at `position` among its new siblings, which are
  renumbered.
* **Across projects:** the folder and its whole subtree change project
  (folder IDs are kept), and every item in the subtree is moved with the
  same re-keying, category remapping and link removal as above.

## 6. Evaluation criteria and trace links

An evaluation criterion states how a requirement is judged fulfilled:
`description` (required), `method` and `threshold` (free text, e.g.
"p95 < 200 ms").

**Methods** are the classic IADT verification methods plus review:
`test` (default), `inspection`, `analysis`, `demonstration`, `review`.

* Criteria are submitted as an array with the requirement. In a `PATCH`, a
  present `criteria` key replaces the whole list; entries that carry the ID
  of an existing criterion keep that ID (and their links), entries without
  an ID are created, missing ones are deleted after their links have been
  detached (`criterion_id` set to `NULL`).
* A **trace link** connects a requirement and a test of the same project
  (`UNIQUE(requirement_id, test_case_id)`). An optional `criterion_id` must
  be a criterion of that requirement; it records which criterion the test
  verifies. Coverage is computed per requirement, not per criterion.
* Creating and deleting links requires `test.write`.

### Automatic requirement ↔ test correlation

By default every new requirement gets a **linked test case** (server setting
`requirements.auto_create_test: true`; per request `"create_test": false`
opts out, `true` opts in when the server default is off; the web editor has
an "Also create linked test" checkbox). The test (`createLinkedTest` in
`internal/api/items.go`):

* is titled `Verify <REQ-KEY>: <requirement title>`, manual, `draft`, with
  the requirement's priority and the tag `auto-generated`;
* gets one step per evaluation criterion (`Verify (<method>): <criterion>`,
  expected = threshold or "Criterion is met"), or a single step derived from
  the requirement text when there are no criteria;
* is placed in the test folder whose name path mirrors the requirement's
  folder path (e.g. requirement in `Security / Auth` → test in test folder
  `Security / Auth`), creating missing folders;
* is linked to the requirement, so the requirement starts with coverage
  `not_run` instead of `none`.

If creating the test or link fails, the requirement is deleted again and the
request fails, so the client never sees a half-done result.

## 7. Coverage calculation

Derived on every read, never stored (`store.LastResults`, `store.Coverage`).

**Last result of a test:** among the test's executions in runs of the
project whose status is not `not_run`, the one with the latest timestamp
(`finished_at`, else `started_at`, else the run's `created_at`). A test
without such an execution has last result `not_run`.

**Coverage of a requirement** from the last results of its linked tests:

| Coverage | Condition (evaluated top-down) |
|---|---|
| `none` | no linked tests |
| `failed` | at least one linked test is `failed` or `blocked` |
| `passed` | all linked tests are `passed` |
| `partial` | at least one linked test is `passed`, the others are `not_run`, `skipped` or `in_progress` |
| `not_run` | otherwise (no linked test has passed or failed) |

Coverage is available as the `coverage` field of requirements, as a query
field (`coverage:failed`), in the traceability matrix, in exports and on
the dashboard (which also counts requirements with at least one link as
"covered").

## 8. Runs and executions

**Creating a run** (`POST /projects/{pid}/runs`): the test set is the union of
`test_ids`, all tests in `folder_ids` (recursive) and all tests matching
`query` (test query schema), de-duplicated in that order. Each test must be
in the project. Tests can be added later (`POST /runs/{id}/executions`);
a test appears at most once per run.

**Step snapshots:** when a test is added, its steps are copied into
`step_results` (step ID, position, action, expected; status `not_run`).
Later edits of the test do not change existing executions.

**Run status** is set by the user (`planned`, `in_progress`, `completed`,
`aborted`). `started_at` is set when the run first becomes `in_progress`,
`completed_at` when it becomes `completed` or `aborted` (and cleared when it
returns to `planned`/`in_progress`). Recording the first result in a
`planned` run moves it to `in_progress`.

**Updating an execution** (`PATCH /executions/{id}`):

* Step results are addressed by their ID; status and actual text are set.
* The execution status is the explicit `status` if given; otherwise, if step
  results were sent, it is **derived** (`store.DeriveStatus`):

  | Derived status | Condition (top-down) |
  |---|---|
  | `not_run` | no steps, or all steps `not_run` |
  | `failed` | any step failed |
  | `blocked` | any step blocked |
  | `passed` | every step passed or skipped, at least one passed |
  | `skipped` | every step skipped |
  | `in_progress` | otherwise (partially executed) |

* Any status other than `not_run` sets `started_at` once. Final statuses
  (`passed`, `failed`, `blocked`, `skipped`) set `finished_at`,
  `executed_by` (the principal's e-mail) and `duration_ms` (the given value,
  or the time since `started_at`). Going back to `not_run` clears
  `started_at` and the duration.

**CI import** (`POST /runs/{id}/results`, permission `run.execute`): each
result names a `test_key` (matched case-insensitively within the run's
project), `status`, `duration_ms` and `output`. Tests not yet in the run are
added first. Unknown keys are reported in `unknown` and skipped. An invalid
status aborts the request with 400; results before it remain applied (the
import is not one transaction). The run-detail view shows a ready-to-use
`curl` call with an API token.

**Server-side script runner** (`POST /executions/{id}/run-script`,
permission `script.execute`, `runner.enabled: true`):

1. The test must be `automated` and have a script (otherwise 400); with the
   runner disabled the answer is 409.
2. The execution is set to `in_progress`; the script is written to a fresh
   directory below `runner.work_dir` and run with the interpreter configured
   for its language (`runner.interpreters`, default language `sh`),
   through `qtestframe sandbox-exec` when `runner.sandbox` is true (with
   socket creation denied when `runner.deny_network` is true).
3. Environment: `PATH`, `HOME`/`TMPDIR` (the job directory), `LANG`, and
   `QTF_TEST_KEY`, `QTF_RUN_ID`, `QTF_RUN_NAME`, `QTF_ENVIRONMENT`,
   `QTF_BUILD`.
4. Exit code 0 → `passed`, any other exit code or a timeout
   (`runner.timeout`) → `failed`. Combined stdout/stderr (capped at
   `runner.max_output_bytes`) is stored as `output`, with the duration.
5. If the script cannot be started, the previous status is restored and the
   error returned.

The request is synchronous; keep `runner.timeout` below
`server.write_timeout`. Security layers: [security.md §9](security.md#9-script-runner-sandbox).

## 9. Query language

One expression language (`internal/query`) filters every list (`q` on
requirements, tests, runs, executions, audit; `q`/`tq` on traceability;
`query` when creating a run; `q` on export) and drives search. Expressions
are parsed into an AST and **evaluated in process** against records built
from the loaded entities (`internal/api/records.go`); they are never
translated to SQL ([ADR 0007](adr/0007-in-process-query-language.md)).

### Grammar

```
expr    = or
or      = and { ("OR" | "||") and }
and     = unary { ["AND" | "&&"] unary }        // adjacency = implicit AND
unary   = ("NOT" | "!" | "-") unary | primary
primary = "(" expr ")" | term
term    = field op value | value
op      = ":" | "=" | "!=" | "~" | "!~" | ">" | ">=" | "<" | "<="
value   = bare word | "quoted \"string\"" | /regex/flags
```

* Precedence `NOT` > `AND` > `OR`; keywords are case-insensitive.
* `-` negates only when directly followed by a non-space character.
* Field names are case-insensitive, may contain letters, digits, `_` and
  `.`, and are resolved through the entity's schema and aliases; an unknown
  field is a parse error.
* A quoted string may contain spaces and `\"`/`\\` escapes. A regex is
  `/…/` with optional flag `i`; `\/` escapes a slash.
* An empty expression matches everything.
* Errors carry a byte position; the API returns `400 invalid_query` with
  `position`.
* Limits: 4096 bytes per expression, 64 regular expressions or globs,
  nesting depth 128.

### Semantics

Every field is multi-valued (tags, categories, criteria, steps, linked keys);
a positive condition holds if **any** value satisfies it, a negative one
(`!=`, `!~`) if **no** value does.

| Form | Meaning |
|---|---|
| `word`, `"two words"` | unqualified: case-insensitive substring over all values of the record |
| `field:value` | case-insensitive substring |
| `field:va*ue`, `field:v?lue` | glob (`*`, `?`), case-insensitive, matched against the **whole** value |
| `field:*`, `has:field` | field has a non-blank value (`has:` only when the schema has no field named `has`) |
| `field=value` | case-insensitive equality; `field=""` matches when all values are empty |
| `field!=value` | no value equals |
| `field~/re/`, `field~re` | RE2 regular expression, unanchored, case-sensitive unless `/i` |
| `field!~/re/` | no value matches |
| `/re/` | regex over all values |
| `field>v`, `>=`, `<`, `<=` | numeric comparison when both sides parse as numbers, otherwise byte-wise string comparison (works for ISO dates against RFC 3339 timestamps) |

`Query.String()` renders a normalised form that parses back to an
equivalent query; the client uses this for quick filters
(`web/src/lib/query.ts`).

### Fields

| Entity | Fields (aliases) |
|---|---|
| Requirement | `id, key, title, description (desc), obligation, type, priority, status, source, rationale, owner, tag (tags), version, folder, category (categories), dimension, coverage, linked, created_by, created_at, updated_at, criteria (criterion)` |
| Test | `id, key, title, description (desc), preconditions, execution_type (type), script_language (language), script, expected, step (steps), priority, status, estimate, tag (tags), folder, category (categories), dimension, linked, last_result (result), created_by, created_at, updated_at` |
| Run | `id, name, description, status, environment (env), build, created_by, created_at` |
| Execution | `id, key, title, status, assignee, executed_by, notes, output` |
| Audit entry | `action, entity, entity_id, user, details, created_at` |
| Search | union of requirement, test and run fields, plus `project` (key and name) and `kind` (`requirement`, `test`, `run`) |

Field contents: `category` holds category names, `dimension` holds
`Dimension/Name`; `folder` is the folder path; `linked` holds the keys of
linked items; `criteria` holds description, method and threshold of every
criterion; `step` holds action, expected and data of every step; `version`
and `estimate` are numbers; timestamps are RFC 3339 UTC.

### Examples

```text
status:approved priority=high                    implicit AND
obligation=must OR obligation=shall
NOT status:obsolete                              same as -status:obsolete
title~/^login.*(fail|error)/i                    regex, case-insensitive
description!~/TBD/                               no value matches
"session timeout"                                phrase anywhere
key:*-REQ-1?                                     glob over the whole key (DEMO-REQ-10 … 19)
version>=2 updated_at>=2026-01-01                numeric / timestamp comparison
dimension:Polarity/Negative category:"Network based"
has:criteria  coverage:none                      requirements without links but with criteria
linked:DEMO-TC-3                                 requirements linked to that test
type=automated last_result:failed                failed automated tests
(tag:smoke OR tag:regression) -status:deprecated
kind=requirement project:DEMO                    search only
```

## 10. Search

`GET /api/v1/search?q=` evaluates the expression against every requirement,
test and run of every project of the current tenant (optionally limited by
`types` and `project_id`). Each hit carries `matched_fields` (fields of the
positive terms that matched) and a `snippet` of about 160 characters around
the first free-text term, taken from the first matched field other than
`key`/`title`, else the description. `limit` defaults to 200 (maximum
1000); `total` counts all hits before the limit.

## 11. Export formats

`GET /api/v1/projects/{pid}/export?format=&aspects=&q=` (permission
`export`) builds a format-neutral `export.Dataset` and renders it with
`internal/export`.

**Aspects** (default: all, in this order):

| Aspect | Columns |
|---|---|
| `requirements` | Key, Title, Obligation, Type, Priority, Status, Folder, Categories (`Dimension/Name`), Tags, Description, Rationale, Source, Owner, Version, Linked tests, Coverage |
| `criteria` | Requirement, #, Criterion, Method, Threshold |
| `tests` | Key, Title, Execution, Status, Priority, Folder, Categories, Tags, Preconditions, Steps (`1. action => expected`, one per line), Expected result, Script language, Linked requirements, Last result |
| `traceability` | Requirement, Requirement title, Coverage, Test, Test title, Last result (one row per link; unlinked requirements get one row with empty test columns) |
| `runs` | Run, Run status, Environment, Build, Created, Test, Title, Result, Executed by, Duration (ms), Notes (one row per execution, runs oldest first) |

`q` is applied to requirements if the requirement schema accepts it and to
tests if the test schema accepts it, so `obligation=must` filters only
requirements and `type=automated` only tests.

**Formats:**

| Format | Output |
|---|---|
| `csv` | RFC 4180, UTF-8, CRLF. One aspect → a single `.csv`; several → a `.zip` with one `<aspect>.csv` per section. |
| `xlsx` (`excel`) | Workbook with a *Summary* sheet (title, generation time, rows per section) and one sheet per section with a bold filled header, frozen first row, autofilter, wrapped cells and column widths 10–60. |
| `pdf` | Landscape A4, Helvetica, one table per section, header row repeated after page breaks, "Page n/N" footer. Text is converted to cp1252 (unmappable characters become `.`). |
| `docx` (`word`) | Hand-written OOXML: title, subtitle, generation time, a heading and a bordered table per section, landscape pages. |

The file is named `<PROJECTKEY>-<YYYYMMDD-HHMMSS>.<ext>` (UTC) and sent with
`Content-Disposition: attachment`; the name contains only `A-Z0-9-.` by
construction.

**Injection protection:**

* **CSV:** a cell starting with `=`, `+`, `-`, `@`, TAB or CR is prefixed
  with `'` (`sanitizeCSVCell`), so spreadsheet applications do not evaluate
  it as a formula. Invalid UTF-8 is replaced.
* **XLSX:** values are written as string cells through `excelize`
  (`SetSheetRow`/`SetCellValue` with strings), never as formulas, so a
  leading `=` is inert. Characters not allowed in XML 1.0 are removed and
  cells are truncated to 32 767 characters; sheet names are sanitised and
  made unique.
* **DOCX:** all text is XML-escaped (`esc`) after removal of invalid XML
  characters.
* **PDF:** text is drawn as text; control characters are removed.

## 12. RBAC matrix

From `internal/rbac/rbac.go` (also served at `GET /api/v1/rbac`). Each user
has one role per tenant; ranks are admin 4 > manager 3 > tester 2 >
viewer 1.

| Permission | viewer | tester | manager | admin | Used by |
|---|:-:|:-:|:-:|:-:|---|
| `project.read` | ✔ | ✔ | ✔ | ✔ | projects, dashboard, folders/categories (read), links, traceability, search |
| `requirement.read` | ✔ | ✔ | ✔ | ✔ | requirement list/get/links |
| `test.read` | ✔ | ✔ | ✔ | ✔ | test list/get/links |
| `run.read` | ✔ | ✔ | ✔ | ✔ | runs, executions |
| `member.read` | ✔ | ✔ | ✔ | ✔ | `GET /members` |
| `export` | ✔ | ✔ | ✔ | ✔ | project export |
| `test.write` | | ✔ | ✔ | ✔ | tests, test folders/categories, trace links |
| `run.write` | | ✔ | ✔ | ✔ | create/update/delete runs, add tests |
| `run.execute` | | ✔ | ✔ | ✔ | record results, CI import |
| `token.manage` | | ✔ | ✔ | ✔ | own API tokens |
| `requirement.write` | | | ✔ | ✔ | requirements, requirement folders/categories |
| `project.create` | | | ✔ | ✔ | create projects |
| `project.update` | | | ✔ | ✔ | rename project, all categories |
| `script.execute` | | | ✔ | ✔ | server-side script runner |
| `audit.read` | | | ✔ | ✔ | audit log |
| `project.delete` | | | | ✔ | delete projects |
| `member.manage` | | | | ✔ | add/change/remove members |
| `tenant.manage` | | | | ✔ | rename own tenant |

Additional rules:

* **Super admin** (`users.super_admin`) holds every permission in every
  tenant, and alone may list/create/delete tenants and manage users. This
  applies only to session logins: requests authenticated with an API token
  are limited to the token's role.
* **API tokens** carry a role not above the creator's role at creation, and
  are capped at the user's current membership role on every request. Tokens
  cannot create tokens.
* **Guards:** a tenant must keep at least one admin (409); a super admin
  cannot demote or disable themselves (409).
* Any authenticated user may call `GET /auth/me`, `/meta`, `/rbac`,
  `/tenants` (own tenants) and `POST /reqlang/analyze`.

## 13. Change history, relations, baselines and collaboration

### Revision history (`internal/store/history.go`, `internal/api/extras.go`)

Every create, update, restore, move and bulk update of a requirement or test
stores a **revision**: the version number, the action, the user, and a full
JSON snapshot. Derived fields (coverage, counts, folder path) are left out.
`changed_fields` is computed against the previous revision. Step IDs and
positions are ignored, so re-saving identical steps is not a change.

* Tests now carry a `version` like requirements; both are incremented on every
  change, including bulk updates.
* Items created before history tracking get an `import` revision (their state
  before the first tracked change), so their first diff is meaningful.
* **Restore** copies the content fields of a revision back as a new version.
  The item's folder and project stay as they are. Categories that no longer
  exist are dropped; for tests, steps get new IDs and calls to deleted tests
  are dropped.

### Requirement relations

Directed links between requirements of the same project:

| Type | Meaning (source → target) |
|---|---|
| `refines` | source is a child (decomposition) of target; cycles are rejected |
| `depends_on` | source needs target |
| `derived_from` | source was derived from target |
| `conflicts_with` | the two cannot both be satisfied |
| `duplicates` | source duplicates target |
| `relates_to` | informal relation |

Query fields: `related`, `parent` (targets of `refines`) and `child`
(sources of `refines`), e.g. `child:*` finds requirements that have been
decomposed.

### Baselines

A baseline freezes the current version and snapshot of all requirements and
tests, or those matching a filter. Baselines can be compared with the current
state or with another baseline of the same project. Items are matched by ID,
so moved or re-keyed items show as *changed*, not removed and added. The diff
lists added, removed and changed items (with versions and changed fields)
and counts unchanged ones. Comparing with *current* uses all current items.

### Comments

Requirements and tests have comment threads. Anyone who can read the item can
comment. Authors can edit and delete their own comments; members with
`member.manage` can delete any comment. Comments and revisions are removed
together with their item.

### Shared steps

A test step can **call** another test of the same project (`call_test_id`).
When a run is created, call steps are replaced by the called test's steps,
recursively up to 5 levels, with the action prefixed by `[KEY] `. Self-calls,
cycles and deeper nesting are rejected when saving. `called_by` shows where a
test is reused. Deleting a called test turns the call into an empty step.

### Defects and execution history

Executions can carry defect references (IDs such as `JIRA-123` or URLs). A
project-level `defect_url_template` (`https://…{id}`) turns IDs into links.
Query field `defect` exists on executions and on tests (any execution of the
test). `GET /tests/{id}/executions` lists a test's results across all runs.

## 14. Profile and preferences

Users edit their display name and **preferences** on the profile page
(`PATCH /auth/me`): theme, start page, default project, default tenant
(selected at login), table page size, date format, time zone, compact tables,
hiding the live requirement analysis, and a personal default for "create
linked test". Values are validated on the server, and unknown keys are
rejected. The **sessions** list shows each login (IP, browser, last activity,
refreshed at most once a minute). Users can revoke a single session or all
other sessions.
