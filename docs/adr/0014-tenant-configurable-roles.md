# 14. Tenant-configurable roles on top of built-in defaults

Date: 2026-11-20

## Status

Accepted. Supersedes the "fixed matrix" part of [ADR 0008](0008-fixed-role-rbac.md);
one role per tenant, the super-admin flag and route-level checks stay.

## Context

Organisations asked for a test lead between tester and manager, for their
own roles (for example an auditor that only reads the audit log) and for
adjusting what the built-in roles may do, without a release. The model
must stay answerable ("who can delete requirements?"), must not allow
privilege escalation, and an organisation must not lock itself out.

## Decision

* `internal/rbac` keeps the list of permissions and five **built-in roles**
  with default permissions: `admin`, `manager`, `test_lead`, `tester`,
  `viewer`. Two permissions were added: `role.manage` (change roles) and
  `settings.manage` (organisation settings for test execution).
* Table `roles` stores, per tenant, **changed built-in roles** (their
  permissions) and **custom roles** (name, label, description,
  permissions). `rbac.Overlay` merges them over the defaults into an
  `rbac.Set`. Built-in roles without a row use the defaults of the running
  version, so new permissions reach them automatically.
* `admin` is never stored and always holds every permission, so a tenant
  cannot lock itself out. Built-in roles cannot be deleted; custom roles
  only when no member or API token uses them.
* The principal's permissions are resolved per request from the tenant's
  role set. An API token gets the **intersection** of its role's
  permissions and the user's current role's permissions (instead of a rank
  comparison, which custom roles make meaningless).
* Nobody can grant a permission they do not hold: creating or changing a
  role, and creating an API token, require the added permissions to be a
  subset of the caller's own.
* Identity-provider group mappings may name custom roles; the strongest
  mapped role is the one with the most permissions (`admin` above all).

## Consequences

* The matrix shown at **Administration → Role matrix** (`GET /roles`) is
  the effective one of the tenant; `GET /rbac` returns it too.
* One extra query per request (the tenant's role rows), cheap compared to
  the rest of a request.
* Role changes are audited (`role.create`, `role.update`, `role.reset`,
  `role.delete`) and included in backups.
* Tests cover the defaults, the overlay, escalation checks and token
  intersection (`internal/rbac/rbac_test.go`, `internal/api/roles_test.go`).
