# Privacy and data retention

This chapter is for super administrators and data protection officers. It explains how long Qtestframe keeps data, how the retention job deletes older records, what a **legal hold** protects, how to answer a **data subject request** (export and erasure under the GDPR) and where regulatory record-keeping rules win over erasure.

## Retention policy

Super administrators set the retention under **Administration → System settings → Retention and backups**. Every period is in **days; 0 keeps the data forever**, which is the default. Nothing is deleted until you turn on the nightly job.

| Setting | What it deletes when older than the period |
|---|---|
| `retention.runs_days` | Test runs with their executions, step results, iterations, comments, defect and item links and attachments. Runs in progress are kept. |
| `retention.runs_keep_latest` | Not a period: runs that hold one of the latest *N* results of a test are always kept, however old (0 = no exception). |
| `retention.logs_days` | Logs and uploaded artifacts: the sign-in log, finished test runner jobs, CI status posts that were sent or gave up, finished tracker sync logs, and web test evidence (screenshots, traces, videos) of old executions – the results themselves stay. |
| `retention.notifications_days` | In-app notifications, read or not. |
| `retention.sessions_days` | Sign-in sessions not used for that long, and API tokens that expired that long ago. |
| `retention.chat_days` | The chat message history (messages sent or given up, with their text) and the personal input suggestion history. |
| `retention.exploratory_days` | The notes of completed or debriefed exploratory sessions, counted from the session's last change. The session and its debrief stay. |
| `retention.enabled`, `retention.hour` | The nightly job and the hour (UTC) it runs. |

**Per-organisation overrides.** Under **Administration → Organisations → (organisation) → Policy → Data retention** a super administrator can give one organisation its own periods (for example a customer contract that requires 10 years for runs). An empty field uses the system setting. The sign-in log and sign-in sessions belong to no organisation and always use the system setting.

**The audit trail is never purged.** It is a hash chain ([Tamper-evident audit trail](security.md#tamper-evident-audit-trail)): verification walks every entry from the first, and signed checkpoints name entries by sequence number, so deleting old entries would break the chain. The chain code has no sealed starting point that would allow cutting it safely, so Qtestframe keeps the trail whole. To limit what the live database holds, archive the trail with **Export with hashes** to write-once storage; the archive verifies on its own (`qtestframe audit verify --file`). Electronic signatures and sign-offs are not purged either.

### Preview before you enable it

Below the settings, **Preview and purge** shows what the job would delete *now*: per organisation and category the number of records older than the period, and how many older records are **held** – kept by a legal hold or an electronic signature. The preview changes nothing. Check it, adjust the periods, and only then turn on `retention.enabled`.

**Purge now** runs the purge at once (also while the nightly job is off) after you type `PURGE`. API: `GET /api/v1/admin/retention` (preview), `POST /api/v1/admin/retention/run` with `{"confirm": "PURGE"}`.

Every purge is recorded in the audit trail as `retention.purge`, in each organisation where something was deleted and in the system trail, with the counts per category and the held records. Deleted records cannot be restored except from a [backup](backup-and-migration.md); take one before the first purge.

## Legal hold

A legal hold protects a whole **project** or a single **release** when data must be preserved – litigation, an authority's request, an audit. While the hold is on:

- the retention job deletes nothing of it (runs of the project or release, their results and evidence, its logs, exploratory notes and chat history);
- the project, the organisation that holds it, and the runs of a held release **cannot be deleted** (409).

Organisation administrators (`tenant.manage`) and super administrators set and lift holds in the project's **Settings → Legal hold**, with a reason that is shown with the hold. Setting and lifting are audited (`legal_hold.set`, `legal_hold.lift`). The retention page lists every project and release under hold across organisations. API: `GET`/`PUT /api/v1/projects/{pid}/legal-hold` and `PUT /api/v1/projects/{pid}/releases/{rid}/legal-hold` with `{"on": true, "reason": "…"}`.

## Data subject requests

### Access and portability (export)

In **Administration → Users**, **Export personal data** downloads a ZIP of JSON files with everything the server holds about the person: profile and preferences, memberships and roles, project roles, comments they wrote, comments that mention them, sessions, API tokens (names and dates, never the secret), the sign-in log, remembered devices, notifications, electronic signatures, input history and the audit entries by or about them. Passwords, second-factor secrets and token hashes are never exported. API: `GET /api/v1/users/{id}/export`.

Every user can download the same export for themselves on their **Profile** (**Download my data**, `GET /api/v1/auth/me/export`). Both are audited as `user.data_export`.

The content a person created in projects (requirements, tests, runs, documents) belongs to the organisation; export it with the project exports if the request covers it.

### Erasure (anonymisation)

**Erase user** in **Administration → Users** anonymises an account after you type its e-mail address:

- its name and address become a pseudonym, **Former user #*n*** (`former-user-n@erased.invalid`), numbered in order;
- deleted: personal settings, avatar, sessions, API tokens, second factor and recovery codes, remembered devices, pending signing tickets, notifications, the sign-in log, input history, invitations, SCIM links, memberships and project roles;
- the account is disabled, has no password and **can never sign in again**; single sign-on can no longer match it;
- content the person authored stays and is **attributed to the pseudonym**: fields that name the author by e-mail address (created by, assignee, owner, tester, uploaded by and similar) are rewritten, and `@` mentions of the person in comments become `@former-user-n`.

You cannot erase your own account or the last super administrator. The erasure is audited as `user.erase` with the pseudonym and counts – not with the erased address. API: `POST /api/v1/users/{id}/erase` with `{"confirm": "<e-mail address>"}`.

### What erasure keeps, and why

- **Electronic signatures and sign-offs** (baseline, waiver and change-request sign-offs, `esignatures`) keep the signer's printed name and e-mail exactly as signed. 21 CFR Part 11 (§11.50, §11.70) and EU GMP Annex 11 (§14) require the signature manifestation to stay linked to the signed record; changing it would invalidate the record. GDPR art. 17(3)(b) allows keeping data needed to comply with such a legal obligation.
- **The audit trail** keeps the e-mail address of the entries the person made, and entries that name them in their details. Rewriting an entry breaks the hash chain, and the trail is itself a regulatory record (Part 11 §11.10(e), Annex 11 §9). The user list and new audit views show the pseudonym for the account.
- **Backups** made before the erasure still contain the original data. Keep backups only as long as your retention schedule says, and repeat the erasure after a restore.
- **Free text** – a name typed into a description, a step or an owner field as plain text – is not searched and replaced; review it if the request requires.

Document these exceptions in your records of processing and in the answer to the person.

## Privacy settings

| Setting | Effect |
|---|---|
| `privacy.truncate_ip` | Client addresses are stored and logged shortened – IPv4 to /24 (`192.0.2.0`), IPv6 to /48 – in the sign-in log, sessions, remembered devices, the request log and the failed sign-in messages. Rate limits still use the full address, in memory only. Audit entries never contain client addresses. |
| `privacy.notice_url` | A link to your privacy notice, shown on the sign-in page (empty = none). |

Both are in **Administration → System settings → Privacy** and apply at once.
