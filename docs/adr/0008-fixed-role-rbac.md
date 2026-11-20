# 8. Fixed-role RBAC per tenant with a super-admin flag

Date: 2026-11-19

## Status

Accepted; the fixed matrix is superseded by [ADR 0014](0014-tenant-configurable-roles.md)
(built-in roles with tenant-configurable permissions and custom roles).

## Context

Access control must be understandable by administrators and auditable by
reviewers. Configurable roles and per-project ACLs are flexible but make
it hard to answer "who can delete requirements?" and multiply the test
matrix. Typical teams map well to four levels: readers, testers who
execute, managers who own specifications, and tenant administrators.
Operators also need a global administrator for tenants and users.

## Decision

* `internal/rbac` defines a fixed permission set and four roles, ranked
  `admin` > `manager` > `tester` > `viewer`, with a static matrix
  (the full table is in [design.md §12](../design.md#12-rbac-matrix)).
* A user has exactly one role per tenant (`memberships.role`); permissions
  apply to all projects of the tenant.
* Routes declare their required permissions in the route table
  (`internal/api/server.go`); `authed` checks them before the handler runs.
  Handlers check additional permissions that depend on the data (folder
  kind, category scope).
* `users.super_admin` is a global flag granting all permissions in every
  tenant plus tenant and user management. It is honoured for session logins
  only, not for API tokens.
* API tokens carry a role not above the creator's role, and are capped at
  the user's current role on every request (`rbac.Rank`).
* The matrix is served at `GET /api/v1/rbac` so the client can hide actions.

## Consequences

* The permission model fits in one table and is tested exhaustively
  (`internal/rbac/rbac_test.go`, `TestRBACEnforcement`).
* No per-project permissions: a tester can edit tests in every project of
  the tenant. Organisations needing separation use separate tenants.
* Adding a permission or role requires a code change and a release.
* Guards prevent lock-out: the last admin of a tenant cannot be demoted or
  removed, and a super admin cannot demote or disable themselves.
