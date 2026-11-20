# Tests

The **Tests** section holds the test cases of a project. A test is either **manual**, executed step by step by a person, or **automated**, a script that the server runs. Tests are organised in folders, classified with categories, linked to the requirements they verify and reused in many runs.

Reading tests needs **Viewer**; creating, editing, linking and copying tests needs **Tester**.

The page works like the [requirements page](requirements.md#the-requirements-page): folder tree, filter bar with quick filters (**Status**, **Execution**, **Priority**, **Last result**, **Category**), selectable columns (including execution type, last result, linked requirements, language, steps, tags, **relations** — *verifies*, *calls* and *called by* — defects, created by and dates) and a table. Bulk edit, moving and deleting work the same way; the bulk toolbar has **Set execution** instead of *Set obligation*.

![The tests page with the test folder tree and the table with type, status and last result](../images/en-tests-list.png)

## Fields

| Field | Use |
|---|---|
| **Title** | A short name (required). |
| **Execution type** | **Manual** or **Automated**. |
| **Description** | What the test is about. |
| **Preconditions** | What must be true before the test starts, for example *a customer account exists*. Shown to the tester in the runner. |
| **Status** | Draft, Ready, Review or Deprecated. |
| **Priority** | Critical, High, Medium or Low. |
| **Estimate (minutes)** | Expected duration. |
| **Folder**, **Categories**, **Tags** | Filing and classification. **Suggested tags** under the tags field come from the title, description, preconditions and steps, as for [requirements](requirements.md#fields). |
| **Overall expected result** | The expected outcome of the test as a whole. |

## Manual tests: steps

Each step has an **Action** (what to do), an **Expected result** (what should happen) and optional **Data** (input values, accounts, files).

![The test editor with steps: action, expected result and data per step](../images/en-tests-steps.png)

- **Add step** appends a step; the **+** on a row inserts one below it.
- The arrows move a step up or down; the bin removes it.
- When you save, the step list replaces the old one. Saving the same steps again is not counted as a change.

When a test is added to a run, its steps are copied into the run. Editing the test afterwards does not change existing runs, only new ones.

## Automated tests: scripts

Switch **Execution type** to **Automated** to show the **Script** field. Choose the **Script language**: **sh**, **Bash**, **Python** or **JavaScript** (Node.js).

![An automated test with a shell script](../images/en-tests-script.png)

- The script runs on the server in a sandbox when someone selects **Run script** in the execution runner.
- Exit code **0** means **Passed**; any other exit code, or exceeding the time limit, means **Failed**.
- Standard output and errors are stored as the execution's **Output**, together with the duration.
- The script can read these environment variables: `QTF_TEST_KEY`, `QTF_RUN_ID`, `QTF_RUN_NAME`, `QTF_ENVIRONMENT` and `QTF_BUILD`, so one script can test the environment and build named in the run.

Running scripts needs the **Manager** role and must be enabled by the administrator (`runner.enabled`). If your scripts run elsewhere, for example in a CI pipeline, leave the script empty and [import the results](runs.md#import-results-from-ci) instead. Steps are optional for automated tests.

## Shared steps

A step can **call** another test of the same project instead of having its own action. Use it for sequences that many tests share, such as signing in.

1. In the steps of a test, select **Insert call to test…**.
2. Choose the test to call and select **Insert call**. The step shows **Calls** with the called test's key.
3. Move the call step to the right position and save.

When a run is created, the call is replaced by the called test's steps (marked `[KEY]`). Calls can be nested up to five levels. A test cannot call itself, and calls may not form a cycle. The called test shows **Used by:** with all tests that call it. If you delete a called test, the call becomes an empty step.

## Categories

Categories classify tests and requirements along **dimensions**. New projects get these defaults:

| Scope | Dimension | Values |
|---|---|---|
| Tests | Polarity | Positive, Negative |
| Tests | Interface | External, Internal |
| Tests | Medium | File based, Network based |
| Tests | Execution | Manual, Automated |
| Tests | Level | Unit, Integration, System, Acceptance |
| Requirements | Area | Core, UI, Security, Operations |

An item can have any number of category values, also several of the same dimension. Manage them in **Categories & settings**:

![The Categories & settings page with requirement and test category dimensions and their colours](../images/en-settings-categories.png)

- **New dimension** creates a dimension with its first value; **Add value** adds values to it.
- Select a colour swatch to recolour a value; it is shown in that colour everywhere.
- **Rename dimension** moves all values to a new dimension name. If the target dimension already has a value with the same name, the two are merged.
- Deleting a value removes it from all items.

In queries, `category:Negative` matches the value name and `dimension:Polarity/Negative` the full form.

The same page holds the project's **Name**, **Description**, the **Defect tracker URL template** (see [Defects](runs.md#defects)) and, for admins, **Delete project**. Editing categories needs **Manager** (or the write permission of their scope, Tester for test categories).

## Mark many tests ready at once

Holders of `test.review` can mark many tests *Ready* in one step: tick the tests and choose **Mark ready…**, or choose **Bulk review** above the list with nothing ticked. As for requirements (see [Approve many requirements at once](requirements.md#approve-many-requirements-at-once)), you choose the tests (selected, shown in the list, or all), the statuses they may come from (by default *Review*; add *Draft* to include drafts) and an optional comment, and a preview shows what changes.

Besides the checks for requirements (the latest review asked for changes, open review requests, suspect links), a test is skipped when it cannot be run as written: a **manual test without steps** or an **automated test without a script**. Choose the check under **Include anyway, despite** to mark them ready anyway.

## Copy a test

**Copy** in the editor creates a duplicate titled *Copy of …* with a new key in the same folder, including steps, categories and tags. The copy starts as **Draft**, so it is reviewed again. Links to requirements are not copied.

A test becomes **Ready** through a review: setting that status needs the permission *test.review* (Reviewer, Test lead or Manager); reviewers see a **Mark ready** button at the top of the editor.

## Linked requirements

The **Linked requirements** section shows which requirements the test verifies. **Link requirements** adds links; **×** removes one. Tests without any link do not contribute to coverage.

## Dependencies

A test can **depend on** other tests: they are its prerequisites and must have passed before it may be started. For example, *Change the delivery address* depends on *Sign in*.

In the test editor, the section **Depends on** lists the prerequisites with their latest result. Select **Add prerequisite** to pick tests of the same project, and **×** to remove one *(Tester)*. **Required by** lists the tests that wait for this one. A test cannot depend on itself, and dependencies may not form a cycle.

When the test is executed, each prerequisite counts as passed when its execution in the same run has passed; when the prerequisite is not part of the run, its latest result counts. Until then the runner shows why the test cannot start, unless the [test execution settings](administration.md#test-execution-settings) allow it. The **Relations** column of the test list shows *depends on* and *required by*, the [dependency graph](traceability-and-coverage.md#dependency-graphs) draws them, and the filter `depends_on:SHOP-TC-1` (or `depends:`) finds the tests that depend on SHOP-TC-1.

## Test flow

**Test flow** in the project navigation (or **Flow diagram** at the bottom of the test editor) draws a test as a flowchart:

- **Start**, then a check **Prerequisites passed?** when the test has [dependencies](#dependencies) (no → *Blocked*), then the **Preconditions**;
- every step as a box with its number, action and test data; a step with an expected result is followed by a decision **As expected?** whose *no* branch ends in **Failed**;
- a [shared step](#shared-steps) as a box with double side bars (*Runs the steps of TC-…*). Select it, or its button under **Shared steps**, to show the called test's steps in place, numbered 3.1, 3.2 …, inside a dashed frame; **Expand all** and **Collapse all** do it for every call. A test that calls itself is marked and not expanded;
- an automated test as **Run the script** followed by **Exit code 0?**;
- the overall **Expected result** as a last check, and **Passed** at the end.

**List** shows the same flow as a numbered text list, with the *otherwise* outcome of each check. Use the zoom buttons for large tests.

**Run order** shows a test run in **stages**: a test is placed after all tests of the run it depends on, so stage 1 holds the tests without prerequisites in the run. Tests of one stage can be executed in any order or in parallel. Each card shows the latest result in the run, which tests it waits for and links to the runner and to its flow diagram. Prerequisites that are not part of the run are listed as warnings.

## Execution history

The **Executions** tab lists every execution of the test in all runs, newest first, with the run, its environment and build, the result, who executed it, when, the duration and any defects. Use it to see whether a test is flaky or when it started to fail.

![The executions tab of a test with results across runs](../images/en-tests-executions.png)

The table column **Last result** shows the most recent result. In queries, `last_result:failed` finds tests whose latest result failed, and `defect:SHOP-*` tests with such a defect.

## Comments and history

**Comments** and **History** work exactly as for [requirements](requirements.md#comments). Restoring a test version restores its fields and steps as a new version; steps get new IDs and calls to deleted tests are dropped.

## Links

The **Links** tab of a test links it to other tests, requirements, runs, documents and saved reports of the archive, report definitions and web addresses, see [Links](requirements.md#links-to-other-items).

## Attachments

The **Attachments** tab holds files for the test: test data, expected screenshots, configuration files. Upload by drag and drop or file selection, or use **Attach from archive**. Uploading and removing needs **Tester**. See [Requirements → Attachments](requirements.md#attachments) for details.
