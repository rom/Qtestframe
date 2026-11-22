# Test cycles

A **test cycle** plans the testing for one goal, for example *Sprint 14 regression* or *Release 2.1 acceptance*. It says which tests are to be run (its **plan**), in which configurations, by whom and how long they should take. When the cycle starts it creates its [test run](runs.md) and from then on tracks the results: planned against executed, passed, failed and blocked, estimated against actual time, a burndown and the workload of every tester.

Select **Test cycles** in the project sidebar.

| Task | Role (permission) |
|---|---|
| See cycles, their plan and progress | every role that sees runs (`run.read`) |
| Create and change cycles, plan tests, assign testers, start and close cycles, recurring cycles | **Test lead**, **Manager**, **Admin** (`cycle.plan`) |
| Create the cycle's runs | also `run.write` |
| Execute the planned tests | **Tester** and above (`run.execute`), in the cycle's run as usual |

## Creating a cycle

Select **New cycle** and enter:

- a **name**, unique in the project, and optionally a **goal**;
- the **owner**, usually the test lead (you by default);
- the **start date** and **end date**; they also bound the burndown;
- optionally the **release**, **variant** and **build** the cycle tests, and its **environment**. The cycle's runs get them, so the results count for that release and variant;
- the **configurations** to test in. Each planned test is then executed once per configuration, and each configuration can have its own tester;
- optionally a time to **start automatically**: at that time the cycle starts by itself and its run is created;
- or choose **Copy a previous cycle**: the new cycle gets that cycle's plan, testers, estimates, release, variant and configurations. This is the quickest way to the next sprint's regression.

A new cycle is **Planned**.

## Planning tests

On the **Plan** tab, select **Plan tests** and choose how to add tests:

| By | What is added |
|---|---|
| **Selection** | the tests you pick in the test picker |
| **Query** | every test matching a [query](search-and-filters.md), for example `tag:regression priority:high` |
| **Folder** | the tests of a folder, and of the folders below it if you like |
| **Regression selection** | the tests that verify changed requirements (and, if you like, the requirements that depend on, refine or are derived from them) with their prerequisites, as the [regression selection](variants-and-code.md) on the Code page; for the cycle's variant |
| **Previous cycle** | another cycle's plan with its testers, estimates and configurations |

You can choose a **tester** and an **estimate** for the tests you add, and the configurations (without a choice, a test is planned in every configuration of the cycle). Tests already planned in a configuration are skipped.

**Estimates.** Without an estimate of your own, a planned test gets the test's **Estimated minutes** (a field of the test), or else the average duration of its past executions with a result. Change estimates and testers later by selecting rows in the plan and choosing **Assign** or **Set estimate**; **Remove** takes rows out of the plan (results already recorded stay in the run).

The plan shows every row with its configuration, tester, estimate, the latest result in the cycle's runs (select it to open the execution) and the actual time.

## Running the cycle

Select **Start cycle**. The cycle becomes **Active** and creates its run: one execution per planned test and configuration, assigned to the tester, with the cycle's release, variant, build and environment. The testers get a notification and execute as usual in the [execution runner](runs.md). Changing a tester in the plan also changes the executions that have no result yet.

Tests planned after the start are not in a run yet; the button **Create run for n new tests** creates a further run of the cycle for them. **Close cycle** ends it; **Reopen** makes it active again.

A planned test counts as **executed** once it has a final result (passed, failed, blocked or skipped) in one of the cycle's runs. If a test was run more than once, the latest final result counts.

## Board by tester

The **Board by tester** tab has a column per tester (and one for tests nobody is assigned to) with the open tests, the estimated time left, and the executed tests folded away. Select a test to open its execution.

## Progress

The key figures at the top show the planned tests (and how many have no tester), the share executed, the results, the estimated time with the time left, and the actual time of the executed tests against their estimates.

The **Progress** tab has:

- the **burndown**: per day from the start date to the end date, the planned tests (or the estimated time) not yet executed at the end of the day, against the **ideal** line from the whole plan on the first day to nothing on the last. The text above it says how far the cycle is behind or ahead of the ideal line today. Tests planned later appear from the day they were added;
- the **workload per tester**: planned, executed (by result) and open tests per tester, with estimated time, time left and actual time.

## Recurring cycles

On the **Recurring** tab of the cycle list, a **recurring cycle** creates a new cycle on a schedule — daily, weekly on a weekday or monthly on a day, at a time in a time zone, like [scheduled report e-mails](reports-and-export.md). Each new cycle gets:

- its **name** from a pattern: `{date}` becomes the start date and `{n}` the number of the cycle, for example *Nightly regression {date}*;
- the fields, plan, testers and estimates of a **template cycle**, and/or the tests matching a **query**;
- today as its start date and the **duration** you set;
- with **Start each new cycle at once**, its run straight away.

The cycles are created as the recurring cycle's **owner**, who must still be allowed to plan cycles in the project. **Create the next cycle now** (the play button) creates one immediately. A recurring cycle whose last run failed shows the error.

## Timeline

The **Timeline** tab shows the cycles with dates as bars on a common time axis, coloured by status.

## Filtering cycles

The filter field of the cycle list takes the [query language](search-and-filters.md) with these fields:

| Field | Matches |
|---|---|
| `name`, `goal`, `description`, `environment` | text of the cycle |
| `status` | `planned`, `active`, `closed` |
| `owner`, `assignee` | the owner, any tester (e-mail or name) |
| `release`, `variant`, `build`, `configuration` | what the cycle tests |
| `start`, `end` | the dates, e.g. `start>=2027-03-01` |
| `planned`, `executed`, `passed`, `failed`, `blocked`, `unassigned` | counts, e.g. `failed>0` |
| `progress` | percent executed, e.g. `progress<50` |
| `test` | a planned test's key |

In the test list, `cycle:"Sprint 14"` finds the tests planned in a cycle; in the run list, `cycle:` finds the runs of a cycle.

## Releases and reports

A release can wait for a cycle: the exit criterion **Test cycle executed** (see [exit criteria](releases-and-risks.md#exit-criteria)) is met when at least the threshold — 100 % by default — of the cycle's planned tests have a result.

The **Test cycle report** on the [Reports](reports-and-export.md) page shows one cycle's key figures, the workload per tester, the burndown and the whole plan with results, in PDF, Word, Excel, CSV or Markdown.

## History and audit

The **History** tab lists the versions of the cycle — its fields and its plan — with who changed what. Creating, changing, planning and deleting cycles and recurring cycles are recorded in the audit log. Cycles and recurring cycles are part of project archives and backups; a loaded project's recurring cycles are switched off until someone enables them.
