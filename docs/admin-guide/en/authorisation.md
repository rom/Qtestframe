# Authorisation

## The model

- Every member holds **one or more roles per organisation**; without a membership a user sees nothing of an organisation. Someone who both writes requirements and tests holds, for example, *Subject matter expert* and *Tester*.
- A role is a named set of **permissions**; a member with several roles has the **union** of their permissions (roles only add rights). Seven roles are built in: Admin, Manager, Test lead, Tester, Subject matter expert (SME), Reviewer and Viewer.
- Holding Admin among one's roles makes a member an organisation administrator. Older API clients see one `role`, the *primary* role: the strongest built-in role of the member (Admin whenever the member holds it).
- **Super administrator** is a flag of the user account, set with `qtestframe user create --super-admin` or under **Administration → Users**. A super administrator holds every permission in every organisation (and acts as admin in an organisation without being a member), and alone may manage organisations and users, back up and restore, and change the system settings. This applies to signed-in sessions only: API tokens never carry it.
- Permissions are checked by the server on every request; the web client only hides what you may not do. An object of another organisation, or of a project you may not see, answers `404`, so its existence is not revealed.

## The default permission matrix

These are the permissions of the built-in roles until an organisation changes them. Admin always holds every permission and cannot be changed.

<!-- BEGIN GENERATED: permissions (make admin-guide-ref) -->

| Permission | Viewer | Reviewer | SME | Tester | Test lead | Manager | Admin |
|---|:-:|:-:|:-:|:-:|:-:|:-:|:-:|
| `project.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `project.create` |  |  |  |  |  | ✔ | ✔ |
| `project.update` |  |  |  |  |  | ✔ | ✔ |
| `project.delete` |  |  |  |  |  |  | ✔ |
| `requirement.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `requirement.write` |  |  | ✔ |  |  | ✔ | ✔ |
| `requirement.review` |  | ✔ | ✔ |  |  | ✔ | ✔ |
| `waiver.approve` |  |  |  |  |  | ✔ | ✔ |
| `change.create` |  |  | ✔ |  | ✔ | ✔ | ✔ |
| `change.approve` |  |  |  |  | ✔ | ✔ | ✔ |
| `test.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `test.write` |  |  |  | ✔ | ✔ | ✔ | ✔ |
| `test.review` |  | ✔ |  |  | ✔ | ✔ | ✔ |
| `run.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `run.write` |  |  |  | ✔ | ✔ | ✔ | ✔ |
| `run.execute` |  |  |  | ✔ | ✔ | ✔ | ✔ |
| `cycle.plan` |  |  |  |  | ✔ | ✔ | ✔ |
| `environment.manage` |  |  |  |  | ✔ | ✔ | ✔ |
| `script.execute` |  |  |  |  | ✔ | ✔ | ✔ |
| `comment.create` |  | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `export` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `audit.read` |  |  |  |  | ✔ | ✔ | ✔ |
| `token.manage` |  |  |  | ✔ | ✔ | ✔ | ✔ |
| `member.read` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `member.manage` |  |  |  |  |  |  | ✔ |
| `role.manage` |  |  |  |  |  |  | ✔ |
| `settings.manage` |  |  |  |  | ✔ | ✔ | ✔ |
| `tenant.manage` |  |  |  |  |  |  | ✔ |

<!-- END GENERATED: permissions -->

### The subject matter expert

The **Subject matter expert** (`sme`) is a domain expert who takes part in writing the requirements of their field – a clinician, a safety engineer, a lawyer. The role:

- creates and edits requirements (`requirement.write`): text, acceptance criteria, rationale, sources, relations, trace and item links and attachments;
- takes part in requirement reviews (`requirement.review`) and discussions (`comment.create`);
- reads tests, runs, defects and risks for context, and exports and reports (`export`).

It deliberately does **not** write or review tests, create or execute runs, plan cycles, approve waivers or accept residual risks, change project settings, sharing or members, or create API tokens. Evidence on test executions is recorded by testers; an SME attaches supporting documents to the requirements themselves. Give a second role to someone who also does more, e.g. *Subject matter expert* + *Tester*.

Because the role both writes and reviews requirements, an SME can approve their own requirements. Where separation of duties matters, remove `requirement.review` from the role in the matrix (or give it only to separate reviewers).

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
- Admin always keeps every permission, and an organisation always keeps at least one member holding Admin: the last one cannot lose the role or be removed – not in the member list, not in bulk, not through SCIM. Admin may be held together with other roles.
- A custom role has a technical name of 2–32 characters (`a-z`, `0-9`, `_`, starting with a letter), which is what `member add --role` (several roles separated by commas: `--role sme,tester`), `role_mapping` and SCIM group mappings use. It can be deleted once no member, project member or token holds it, also among other roles.
- A built-in role whose permissions differ from the defaults is marked *changed* and can be reset. Changes are audited, and a change of a member's roles is logged with the roles added and taken away (`+sme −viewer`).
- API tokens hold roles too (by default all roles of their owner) and never get more than their owner's current roles allow.
- With single sign-on (`role_mapping`) and SCIM, a user in several mapped groups holds the roles of all of them.

The effective matrix of an organisation is served at `GET /api/v1/roles`.

## Restricted and shared projects

A project is either **organisation-wide** (the default: every member sees it with their organisation role) or **restricted** (only its project members see it). Project members are users or SCIM groups with one or more **project roles**, which replace the organisation roles inside that project, upwards or downwards. Users with `project.update` in the project manage this under **Share** ([user guide](../../user-guide/en/administration.md#project-sharing)).

| Who | Permissions inside a project |
|---|---|
| organisation admins and super administrators | their own, always – they see every project |
| a project member (directly or through a group; the direct membership wins, groups add up) | the project-scoped permissions of the project roles (added up) |
| any other member, organisation-wide project | the organisation roles |
| any other member, restricted project | none: the project answers `404` |

Only **project-scoped** permissions come from the project roles (`project.read`, `project.update`, `project.delete`, `requirement.*`, `waiver.approve`, `change.*`, `test.*`, `run.*`, `cycle.plan`, `environment.manage`, `script.execute`, `export`, `comment.create`). Organisation-level permissions – members, roles, API tokens, the audit log, creating projects – always come from the organisation roles, and an API token is capped by its own roles inside a project as well.

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

## Organisations and users (super administrators)

**Administration → Organisations** lists every organisation with its members, projects and storage, against its quotas.

- **Create** with a slug (2–63 of `a-z`, `0-9`, `-`, fixed) and a name; **rename** at any time.
- **Suspend** stops an organisation without deleting anything: its members cannot open it (a session in it has no organisation, switching to it or signing in into it answers 403) and its API tokens are refused. **Reactivate** ends it. Super administrators can still open a suspended organisation.
- **Usage and policy** shows the counts (members, projects, requirements, tests, runs, files, storage, last activity), what the organisation's admins have set (AI assistance, electronic-records mode, MFA required), and the **policy** only super administrators change: quotas for members, projects and storage (adding beyond them is refused, nothing existing is removed), *Forbid AI assistance*, the **allowed sign-in methods** (password, an OIDC or SAML provider; members whose account uses another method cannot open the organisation) and **e-mail domains** whose new single sign-on users join it.
- **Delete** removes the organisation and all its data for good. The dialog offers a backup first and asks for the slug to be typed; the organisation you are working in cannot be deleted.
- **Audit trail** opens the organisation's audit log; super administrators can pick any organisation there.

Nobody can sign in as another user – super administrators included. Switching to an organisation opens it under your own name, and every action is recorded with it.

**Administration → Users** lists every account across organisations, with filters (status, sign-in method, MFA, super administrators). Super administrators create and edit accounts, **disable and enable** them, reset a local password (a temporary one, shown once) or the second factor, **sign a user out everywhere** (API tokens stay valid), see the user's **organisations and roles**, and give or take the super administrator flag. The last enabled super administrator cannot be demoted or disabled.

Who may create **projects** is the permission `project.create` of each organisation's role matrix; only super administrators create organisations. A new project is visible to the whole organisation until its members restrict it (above).
