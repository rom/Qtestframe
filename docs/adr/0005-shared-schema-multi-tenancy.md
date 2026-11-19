# 5. Shared-schema multi-tenancy with `tenant_id`

Date: 2026-11-19

## Status

Accepted

## Context

One installation serves several organisations or departments (tenants)
whose data must be isolated. Users may belong to several tenants with
different roles. The options were a database per tenant, a schema per
tenant, or shared tables with a tenant column. SQLite has no schemas, the
number of tenants is expected to be small to moderate, and cross-tenant
administration (super admins, user management) must stay simple.

## Decision

* All tenants share one schema. Every tenant-owned table has a `tenant_id`
  column; child rows without one (criteria, steps, step results, category
  assignments) are only reachable through a parent loaded with a tenant
  predicate.
* Users are global (`users.email` unique); `memberships(tenant_id,
  user_id, role)` assigns exactly one role per tenant.
* The tenant of a request is the **principal tenant**: the current tenant
  of the session (`sessions.tenant_id`, changed only through
  `POST /auth/switch-tenant` after a membership check) or the tenant bound
  to the API token. It is never read from the URL or body.
* Store functions take `tenantID` as an explicit argument and every lookup
  by ID includes `AND tenant_id = ?`, so a foreign ID yields 404.
* Deleting a tenant cascades through foreign keys; tables without a foreign
  key to `tenants` (`audit_log`, `sessions`) are cleaned explicitly.

## Consequences

* Works identically on SQLite and PostgreSQL, with one migration path.
* Isolation depends on the discipline of always passing and filtering by
  the tenant; this is concentrated in `internal/store` and covered by API
  tests that access foreign IDs. PostgreSQL row-level security is not used.
* Per-tenant backup/restore or physical separation is not available; a
  tenant export is limited to the project exports.
* Project keys are unique per tenant, item keys per project.
