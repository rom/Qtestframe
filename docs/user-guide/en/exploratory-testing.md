# Exploratory testing

Exploratory testing is test design and test execution at the same time: the tester learns the product while testing it and decides what to try next from what they just saw. Qtestframe supports **session-based test management** (SBTM): the work is done in **sessions** of a fixed length, each with a **charter**, and every session ends with a **debrief**.

| Action | Role needed |
|---|---|
| View sessions, notes, metrics and the history | Viewer (`run.read`) |
| Plan, change and delete sessions; reopen a debrief | Tester (`run.write`) |
| Run a session: the timer, notes, files, defects from notes, the debrief | Tester (`run.execute`) |
| Export the session report | Viewer (`export`) |
| Let sessions count as verification evidence | Manager (`project.update`) |

Only the session's **tester** runs it, takes its notes and debriefs it. Someone else needs `run.write` as well: a test lead who stops a forgotten session, or a reviewer who debriefs it. A session without tester belongs to whoever starts it.

## Plan a session

Open **Exploratory** in the project's navigation and select **New session**.

- **Charter**: a short name for the mission, for example *Explore checkout with expired cards*.
- **Mission**: what to explore, with which resources, to discover what (*Explore … with … to discover …*).
- **Tester**: who does the session (by default you).
- **Time box**: the planned length, usually 30, 60 or 90 minutes; any length from 5 to 480 minutes is accepted.
- **Areas**: the requirement and test folders the session explores. The metrics count sessions and bugs per area.
- **Requirements** and **Tests** in scope: the items the session is about. A defect raised in the session is linked to these requirements, and a debriefed session can be evidence for them (see below).

A new session is **planned**. The list shows every session with its charter, tester, status, outcome, session time against the time box, the number of notes and of bugs. Filter it by text, status and area, or select **My sessions**.

## Run a session

Open a session to see the **session runner**: the timer at the top, the note field, the notes taken so far, and on the side the charter with its scope, the screenshots and files, and the history.

### The timer

**Start** starts the timer, **Pause** pauses it (a break does not count) and **Resume** continues. **Stop** ends the session for good. The time is kept by the server, not by the browser: you can reload the page, close the browser or continue on another computer, and the session time stays right. The bar under the clock shows how much of the time box is used; past the time box it turns orange, but the timer keeps going.

| Status | Meaning |
|---|---|
| Planned | Not started yet |
| In progress | The timer runs |
| Paused | Started, the timer is paused |
| Completed | Stopped, waiting for the debrief |
| Debriefed | Debriefed; the session is closed |

### Notes

Notes are taken from the start of the session until its debrief. Each note has a **type** and is stamped with the session time at which it was taken:

- **Observation**: what you saw, including set-up and what works.
- **Question**: something to ask the product owner or the developers.
- **Issue**: something that is wrong or unclear but perhaps not a bug.
- **Idea**: a test idea for later, or an improvement.
- **Bug**: a problem in the product.

Write the note and select **Add note**, or press **Ctrl+Enter**. Choose **Files of the session** to refer to screenshots you uploaded under **Screenshots and files** (the files are stored like every attachment and appear in the project's [documents](documents.md)).

### Keyboard shortcuts

The runner is made to be used without the mouse. Outside the note field:

| Key | Action |
|---|---|
| O, Q, I, D, B | Choose the note type (Observation, Question, Issue, iDea, Bug) and go to the note field |
| N | Go to the note field and keep the type |
| T | Start or pause the timer |
| ? | Show or hide the shortcuts |

In the note field, **Ctrl+Enter** (⌘+Enter on a Mac) adds the note and **Esc** leaves the field so that the letters work as shortcuts again. Screen readers announce the type chosen and each note added.

### Raise a defect from a note

Select **Raise defect** next to a note. The defect is prefilled: the title is the first line of the note, the description the note with the session's key, the session time and the charter, and *found in* is *test*. Change what you need and confirm. The defect is linked to the session and to the requirements in its scope, the note shows the defect's key and becomes a bug note. See [Defects](defects.md).

## Debrief

When the session is stopped, the debrief opens. In SBTM the debrief is a short conversation between the tester and a test lead; record its result here:

- **Summary**: what was covered, what was found, what is left.
- **Outcome**: *Passed* (the scope works as required), *Failed* (problems found that fail it) or *Inconclusive* (it could not be judged, for example because it was blocked).
- **Coverage of the charter**: your estimate in percent of how much of the charter the session covered.
- **Time split**: the share of the session spent on **set-up** (preparing the environment and the data), **testing** (test design and execution) and **bug investigation** (investigating and reporting problems). The three add up to 100 %. The form suggests a split from the notes.
- **Reviewer's comment**: what a reviewer adds.

The tester debriefs their own session. Someone else with `run.write` can debrief it as a **reviewer**: the session then shows *Reviewed by*. A debrief can be changed later; **Reopen** (with `run.write`) removes it and makes the session completed again. A debriefed session and its notes cannot be changed.

## Sessions as verification evidence

By default exploratory sessions are not verification evidence. Under **Settings → Project → Verification** a manager can turn on **Exploratory sessions are verification evidence**. Then a debriefed session counts as evidence of kind *exploratory* for the requirements in its scope, about the requirement as a whole:

- *Passed* passes, *Failed* fails, *Inconclusive* does not count;
- for each requirement only the newest debriefed session counts; older ones are shown as *superseded*;
- a session never verifies an [evaluation criterion](requirements.md) on its own, so it verifies on its own only a requirement without criteria. With criteria it adds to the evidence: a failed session fails the requirement.

The requirements' status follows the verification when the project lets the status follow the evidence. See [Traceability and coverage](traceability-and-coverage.md).

## Metrics

Above the list, **Metrics** show the number of sessions, the session hours, the bugs (bug notes and notes a defect was raised from) and the **bugs per session hour**, the average time split and coverage of the debriefs, and the same per **area** and per **tester**.

## Report, history and audit

**Markdown** and **PDF** at the top of the runner export the **session report**: the charter and scope, the session time, the debrief, every note in the order taken and the defects raised. The same report, and the summary of all sessions (*Exploratory testing*), are standard reports on the **Reports** page, where they can also be saved in the document archive or e-mailed ([Reports and export](reports-and-export.md)).

**History** lists the revisions of the session: when it was planned, changed, stopped, debriefed and reopened, with the changed fields. Every action (planning, timer, notes, defects, debrief) is recorded in the organisation's audit log. Sessions, their scope and notes are part of the project archive (**Save project**) and of the backup.
