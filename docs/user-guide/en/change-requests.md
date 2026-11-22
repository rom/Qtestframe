# Change requests and change control

Once a requirement is approved, or frozen in a signed baseline, changing it should be a decision, not an edit. **Change control** makes it one: a **change request** proposes new values for one or more requirements and tests, says why, shows what else the change affects, is decided by the project's **change control board** with electronic signatures, and is then applied in one step, with normal versions and history.

| Action | Role needed |
|---|---|
| View change requests | Viewer |
| Propose changes; edit, submit, assess, send to the board, apply, verify and close change requests | Subject matter expert, Test lead, Manager (`change.create`) |
| Decide change requests as a board member; emergency override | Test lead, Manager (`change.approve`) |
| Change the project's change control settings | Manager (`project.update`) |

Applying a change request also needs the permission to edit the items it changes (`requirement.write`, `test.write`, and the review permission for a review status).

## Change control in a project

In **Settings → Project → Change control** choose how strictly the project controls changes:

- **Off** (the default): items are edited directly, as without change control. Change requests can still be used.
- **Warn**: protected items can still be edited directly, but the editor says that they are under change control and the change is flagged.
- **Enforce**: protected items cannot be edited directly. Saving them, a bulk edit, the spreadsheet view, an Excel or ReqIF import, restoring an older version or a baseline and deleting are refused with *Not saved: the item is under change control*, and the editor offers to **propose the change** instead.

An item is **protected** when its status is one of the **protected statuses** (by default *Approved*, *Implemented*, *Verified* and, for tests, *Ready*) or, with **Signed baselines lock their items**, when it is in a baseline that has a [sign-off](baselines.md). Review decisions and the automatic status of evidence are not affected.

The **change control board** is a list of members (e-mail addresses) and roles. Only they decide change requests, and only if they also hold `change.approve`; with nobody listed, anyone with `change.approve` decides. The **quorum** says how many approvals are needed: *all* (every listed member and one holder of each listed role), *any* (one member) or a number. The **signature meaning** is the sentence a board member signs, for example *Approved for release by the change control board*. **Closing a change request creates a baseline by default** pre-selects that choice when a change request is closed.

### Emergency override

When a fix cannot wait for the board, someone with `change.approve` can save a protected item directly with an **emergency override** and a justification of at least ten characters (in the API `?change_override=<justification>`). The change is saved like any other, and the audit log records `change_control.override` with the item, the reason it was protected and the justification.

## Propose a change

There are three ways to put a change into a change request:

- **In the editor**: open a requirement or test, change its fields as usual and select **Propose change** below the form instead of **Save**. The fields you changed (title, description, obligation, type, priority, status, source, rationale, owner, tags and custom fields; for tests also preconditions, expected result, execution type and estimate) become the proposal. Nothing is saved on the item.
- **In bulk**: select items in the list and choose **Propose change** in the selection bar, then the new status, priority, obligation or execution type and owner for all of them.
- **After a refused save**: under *enforce*, saving a protected item opens the same dialog with your changes.

In the dialog choose **A new change request** (title, reason, category and priority) or **One of my drafts** to add the change to a draft you are still writing. The dialog shows each change as a difference: removed text struck through, added text highlighted.

Items with open change requests show a **CR** badge in the lists, and the editor lists their open change requests. In the [query language](search-and-filters.md), `cr:open` (or `has:cr`) finds items with an open change request and `cr:DEMO-CR-3` the items of one.

## The workflow

**Change requests** in the project sidebar lists them with key (*PROJ-CR-n*), status, category, priority, items and requester. Open one to see its proposed changes item by item and field by field, the impact, the signatures and the timeline.

1. **Draft**: the requester writes the change request. **Category** is one of *Correction*, *Enhancement*, *Regulatory*, *Defect-driven*, *Risk-driven* and *Other*. Link the defects, risks and tracker issues the change comes from, choose what applying does with the [change impact](traceability-and-coverage.md) (*Mark suspect*, *Notify owners* or *Neither*) and whether closing creates a baseline.
2. **Submit**: needs at least one item and a reason. The transitive change impact of the proposed changes is computed and **stored on the change request** as a snapshot, with the path to every affected item, so the board decides on what was assessed.
3. **Assess impact**: an assessor adds notes and an effort estimate; the impact is computed again. *Send to the change control board afterwards* moves it on at once.
4. **In review**: the board members are notified, and it appears in their [My work](reviews-and-notifications.md) as *Change decision*.
5. **Decide**: a board member selects **Decide…**, chooses *Approve*, *Reject* or *Defer*, adds a comment (required to reject or defer) and confirms with their password. This is an **electronic signature** over the exact content of the change request with the project's signature meaning. A rejection or deferral decides at once; approvals decide once the quorum is reached. The requester cannot decide their own change request and is notified of the decision.
6. **Apply**: an approved change request is applied in one step. Every item gets the proposed values through the normal save, so validation, permissions, versions, history and the audit log are the same as for an edit; then the impact is applied once, with the chosen action. If one item cannot be saved, nothing is saved. If an item changed since its change was proposed, the change request is *outdated* and cannot be decided or applied: withdraw it and propose the change again.
7. **Verify** and **Close**: someone confirms that the change works as intended, then closes it, optionally with a new baseline of the project.

A rejected or deferred change request can be **withdrawn** back to a draft and submitted again, or closed. Drafts and rejected change requests can be deleted. Every step is on the **timeline** with who did it, when and why.

## Reports, notifications and integrations

- The report template **Change requests** (in **Reports**) produces one change request (its proposed changes field by field, the impact, the signatures and the timeline) or the register of all of them; **PDF report** in the change request does the same for it.
- Webhooks and chat channels can subscribe to `change_request.submitted`, `change_request.review`, `change_request.approved`, `change_request.rejected`, `change_request.deferred`, `change_request.implemented` and `change_request.closed`.
- Change requests are part of project archives and backups.

## In the demo

The demo project has change control set to *warn*, with the administrator and the managers as the board. Sara's change request *Lock accounts for 30 minutes instead of 15* waits for the administrator's decision in My work; there is also a closed change request whose change was applied, a deferred one and a draft.
