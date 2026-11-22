# Reviews and notifications

Qtestframe tells you when something needs your attention, lets you ask colleagues to review a requirement or a test, and records electronic sign-offs of baselines. Webhooks send the same events to other systems.

## My work

**My work** in the main navigation (under **Projects** and **Search**) is your personal work list: everything that needs you, across all projects you can see. The badge next to it shows how many items are waiting; it turns red when some of them are overdue. Each row links to the item and says why it is there:

| Reason | The item is listed because |
|---|---|
| **Owner** | you own a requirement in draft, review, approved or implemented, a test in draft or review, or an open risk (identified or mitigating) |
| **Assigned** | an open defect or a test execution that has not been run yet (in a planned or running run) is assigned to you |
| **Review requested** | someone asked you to review a requirement or a test |
| **Approval** | a requested waiver or deviation waits for your signature: you are one of its approvers, or it names none and you may approve waivers. Your own requests are not listed. |
| **Risk acceptance** | a mitigated or accepted risk is above the project's acceptance threshold and has no valid acceptance signature, and you may sign it |
| **Risk review** | you own the next review of a risk and it is due within a week |
| **Overdue** | its due date has passed: a review request's due date or a risk's review date |
| **Suspect**, **Stale results**, **Blocked** | a requirement or test you own became suspect (something linked or related changed), has stale test results, or is blocked by an open defect — also when it is otherwise finished |

Overdue items come first, then by due date, then the most recently changed. Choose a project to see only its items, or a reason to see only those rows. The project dashboard shows the same list for the project in the **My work** widget, with the number of items per reason; select a reason to open the filtered list.

The work list is computed when you open it; finishing the work (approving the review, closing the defect, running the test, changing the status) removes the row.

## Notifications

The bell in the top bar shows how many notifications you have not read. Open it to see the latest ones; choose one to go to the item it is about. **All notifications** lists everything, with a filter for unread ones.

You get a notification when someone else:

- makes you the owner of a requirement, a test or a risk (also in a bulk change), or assigns an execution or a defect to you,
- mentions you with `@handle` in a description, a comment or execution notes,
- asks you to review something, or finishes a review you asked for,
- changes the status of a defect assigned to you.

You are never notified about your own changes. Read notifications are cleaned up after some time.

On **All notifications**, tick notifications (or **Select all shown**) to **Mark read**, **Mark unread** or **Delete** them together. **Delete read** removes every notification you have already read.

### Notifications by e-mail

When the server can send e-mail, you can also get your notifications by e-mail. Choose under **Profile → E-mail notifications** (or **E-mail notifications** on the notifications page):

- **Off** (the default): notifications are only shown in the app.
- **Immediately**: each notification is e-mailed to you, with a link to the item.
- **Daily digest**: once a day you get one e-mail with the notifications you have not read yet and that were not e-mailed before. Nothing is sent on days without new notifications.

You never get e-mails about your own actions. Every notification e-mail has a link back to this setting, so you can change it or turn it off at any time.

## Request a review

Open a requirement or a test. Under the review bar, **Review requests** shows who has been asked and what they decided. Choose **Request review** (you need write permission for the item):

1. Choose one or more reviewers.
2. Write a message and, if you like, a due date.
3. Adjust the checklist: one point per line. Each kind of item has a default checklist (for example "Verifiable (can be tested)" for requirements).

Each reviewer gets a separate request and a notification. A person who already has a pending request for the item is not asked twice. You can cancel a pending request with the cross next to it.

## Do a review

**My reviews** in the side bar lists the requests sent to you; overdue ones are highlighted. Choose **Review** to open one:

- Tick each checklist point. A point that does not apply needs a short note instead.
- Write a comment.
- **Approve** needs every point ticked or explained. **Request changes** needs a comment saying what to change.
- **Save** keeps your work without deciding.

If the item changes after the request was sent, the review is marked "changed since" so everyone can see that the decision was about an older version.

### Items in review status

Below the requests, **In review status** lists the requirements and tests whose status is **Review** and that concern you, in all projects, even when nobody sent you a request. **Why you see it** tells you how each one concerns you:

- **you own it**: the requirement's **Owner** names you (your name, e-mail or @handle);
- **you wrote it**: you created the requirement or test;
- **review requested**: someone asked you to review it;
- **you may review**: your role may review requirements or tests.

Items that have waited longest come first. Turn on **Only items I own, wrote or was asked to review** to hide the ones you only see because of your role. Open an item to review it: approve it, reject it or set it back to **Draft**. Once its status is no longer **Review**, it leaves the list.

Many reviews at once: when you approve or reject requirements, or mark tests ready, in a [bulk review](requirements.md#approve-many-requirements-at-once), you can complete your open requests for those items in the same step. They are then decided without the checklist and marked as decided in bulk.

A review request records a decision; it does not change the item's status. Holders of the review permission still approve a requirement or mark a test ready with the review bar.

## Many review requests at once

**My reviews** has two tabs: **To review** (requests sent to you) and **Requested by me** (requests you sent). Tick requests in either list for a toolbar; only pending requests are changed.

- **To review**: **Approve** several at once (with a comment, and after confirming that the checklists are not ticked; each review records that it was approved in bulk), or **Request changes** with one comment for all of them. The people who asked are notified.
- **Requested by me**: **Remind** the reviewers (they get a new notification, with your message), set the **Due date** (empty removes it), or **Cancel** the requests.

## Sign a baseline

Open a baseline under **Baselines**. **Sign-offs** lists the electronic signatures: who signed, as what (author, reviewer, approver or release), when, and a fingerprint of the content.

To sign you need the review permission for requirements or tests. Choose **Sign**, choose what your signature means, and confirm with your password. If you sign in with single sign-on, your sign-in must be less than 15 minutes old. API tokens cannot sign.

Each signature stores a SHA-256 fingerprint of the baseline's items. **Valid** means the content is still exactly what was signed. A signed baseline cannot be deleted, and every signature is written to the audit log.

## Webhooks

Under **Categories & settings**, project administrators (permission to change the project) can add webhooks. A webhook sends events as HTTP POST requests with a JSON body:

```json
{ "id": "…", "event": "defect.created", "project_id": "…", "actor": "anna@example.com", "time": "…", "data": { … } }
```

- Choose the events, or **All events**. Events include `requirement.updated`, `run.completed`, `defect.status`, `release.released`, `review.completed` and `baseline.signed`.
- The signing secret is shown only once, when you create the webhook or choose **New secret**.
- Every request has the header `X-Qtestframe-Signature: sha256=<hex>`, the HMAC-SHA256 of the body with the secret. Check it before you trust a request.
- **Send test** sends a `ping` event at once. The table shows the result of the latest delivery.

Deliveries are not retried and redirects are not followed.

Tick webhooks in the list to **Turn on** or **Turn off** several at once, **Add events** or **Remove events** (each webhook must keep at least one event, otherwise nothing is changed), **Send test** to all of them (the result of each is listed), or **Delete** them. Changes other than deleting can be undone with the undo button.
