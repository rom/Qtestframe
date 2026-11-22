# Authorisation

## The model

- Every user has **one role per organisation** in which they are a member; without a membership a user sees nothing of an organisation.
- A role is a named set of **permissions**. Six roles are built in: Admin, Manager, Test lead, Tester, Reviewer and Viewer.
- **Super administrator** is a flag of the user account, set with `qtestframe user create --super-admin` or under **Administration → Users**. A super administrator holds every permission in every organisation (and acts as admin in an organisation without being a member), and alone may manage organisations and users, back up and restore, and change the system settings. This applies to signed-in sessions only: API tokens never carry it.
- Permissions are checked by the server on every request; the web client only hides what you may not do. An object of another organisation, or of a project you may not see, answers `404`, so its existence is not revealed.

## The default permission matrix

These are the permissions of the built-in roles until an organisation changes them. Admin always holds every permission and cannot be changed.

<!-- BEGIN GENERATED: permissions (make admin-guide-ref) -->

| Permission | Viewer | Reviewer | Tester | Test lead | Manager | Admin |
|---|:-:|:-:|:-:|:-:|:-:|:-:|
| `project.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `project.create` |  |  |  |  | ✔ | ✔ |
| `project.update` |  |  |  |  | ✔ | ✔ |
| `project.delete` |  |  |  |  |  | ✔ |
| `requirement.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `requirement.write` |  |  |  |  | ✔ | ✔ |
| `requirement.review` |  | ✔ |  |  | ✔ | ✔ |
| `waiver.approve` |  |  |  |  | ✔ | ✔ |
| `test.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `test.write` |  |  | ✔ | ✔ | ✔ | ✔ |
| `test.review` |  | ✔ |  | ✔ | ✔ | ✔ |
| `run.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `run.write` |  |  | ✔ | ✔ | ✔ | ✔ |
| `run.execute` |  |  | ✔ | ✔ | ✔ | ✔ |
| `script.execute` |  |  |  | ✔ | ✔ | ✔ |
| `export` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `audit.read` |  |  |  | ✔ | ✔ | ✔ |
| `token.manage` |  |  | ✔ | ✔ | ✔ | ✔ |
| `member.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `member.manage` |  |  |  |  |  | ✔ |
| `role.manage` |  |  |  |  |  | ✔ |
| `settings.manage` |  |  |  | ✔ | ✔ | ✔ |
| `tenant.manage` |  |  |  |  |  | ✔ |

<!-- END GENERATED: permissions -->

What the permissions allow:

| Permission | Allows |
|---|---|
| `project.read`, `requirement.read`, `test.read`, `run.read` | reading projects and their requirements, tests and runs |
| `project.create`, `project.update`, `project.delete` | creating projects, changing their settings (also trackers, webhooks, web test settings and sharing), deleting them |
| `requirement.write`, `test.write` | editing requirements and tests |
| `requirement.review`, `test.review` | review decisions: approving or rejecting requirements, marking tests ready |
| `run.write`, `run.execute` | creating and editing runs; recording results, CI imports, pushing defects to trackers |
| `script.execute` | running scripts and web tests **on the server** |
| `export` | exports and reports |
| `audit.read` | the organisation's audit log |
| `token.manage` | own API tokens |
| `member.read`, `member.manage` | the member list; adding, changing and removing members, resetting their passwords and MFA |
| `role.manage` | the role matrix and custom roles |
| `settings.manage` | the test execution settings of the organisation |
| `tenant.manage` | renaming the organisation, its MFA policy and SCIM provisioning |

Grant `script.execute` only to people you would let run code on the server: scripts and web tests run with the privileges of the server's user, inside the sandbox described under [Security](security.md#the-script-and-web-test-sandbox).

## Custom roles and changed roles

Holders of `role.manage` (Admin by default) change the permissions of every built-in role except Admin and create **custom roles** under **Administration → Role matrix** ([user guide](../../user-guide/en/administration.md#change-roles-and-create-your-own)). The rules that keep this safe:

- Nobody can grant a permission they do not hold themselves – not to a role, not to an API token, not through project sharing.
- Admin always keeps every permission, and an organisation always keeps at least one admin (the last one cannot be demoted or removed, also not through SCIM).
- A custom role has a technical name of 2–32 characters (`a-z`, `0-9`, `_`, starting with a letter), which is what `member add --role`, `role_mapping` and SCIM group mappings use. It can be deleted once no member or token uses it.
- A built-in role whose permissions differ from the defaults is marked *changed* and can be reset. Changes are audited.

The effective matrix of an organisation is served at `GET /api/v1/roles`.

## Restricted and shared projects

A project is either **organisation-wide** (the default: every member sees it with their organisation role) or **restricted** (only its project members see it). Project members are users or SCIM groups with a **project role**, which replaces the organisation role inside that project, upwards or downwards. Users with `project.update` in the project manage this under **Share** ([user guide](../../user-guide/en/administration.md#project-sharing)).

| Who | Permissions inside a project |
|---|---|
| organisation admins and super administrators | their own, always – they see every project |
| a project member (directly or through a group; the direct membership wins, groups add up) | the project-scoped permissions of the project role |
| any other member, organisation-wide project | the organisation role |
| any other member, restricted project | none: the project answers `404` |

Only **project-scoped** permissions come from the project role (`project.read`, `project.update`, `project.delete`, `requirement.*`, `test.*`, `run.*`, `script.execute`, `export`). Organisation-level permissions – members, roles, API tokens, the audit log, creating projects – always come from the organisation role, and an API token is capped by its own role inside a project as well.

What restriction does **not** hide, so plan accordingly:

- the organisation's **audit log** (`audit.read`) records actions in every project;
- e-mail digests may list notifications that were created while the recipient still had access;
- **backups** contain every project; project archives (*Save project*) contain the whole project.

## Where administration happens

| Page in the web client | Who |
|---|---|
| Members, Role matrix | `member.read` (every built-in role) to see them; changes need `member.manage` / `role.manage` |
| API tokens | `token.manage` |
| Test execution | `settings.manage` |
| Audit log | `audit.read` (Test lead and up by default) |
| Provisioning (SCIM), Multi-factor authentication (organisation policy) | `tenant.manage` |
| Organisations, Users, Backup, System settings, server MFA policy | super administrators |
