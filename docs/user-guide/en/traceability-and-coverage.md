# Traceability and coverage

Traceability answers two questions: *which tests verify this requirement?* and *has it been verified successfully?* Qtestframe answers them with **links** between requirements and tests and with the **coverage** derived from the test results.

## The traceability matrix

Select **Traceability** in the project sidebar.

![The traceability matrix: requirements in rows, tests in columns, coverage per requirement and last result per test](../images/en-traceability.png)

- Requirements are the **rows**, tests the **columns**. A marked (pressed) cell means *linked*.
- Each mark is coloured by the linked test's latest result and has its own symbol: ✓ passed (green), ✕ failed (red), ⊘ blocked (amber), » skipped (purple), clock in progress (blue), – not run (grey). A suspect link has an amber frame and a warning triangle. The **Marks** legend explains them; screen readers announce the result with each cell. The **List** view colours the linked tests the same way.
- The coloured bar at the start of each row shows the requirement's coverage; the **Coverage legend** explains the colours. Point at a test's column header to see its key, title and last result.
- The summary says how many requirements are linked, for example *14/20 requirements linked (70 %)*.
- **Filter requirements** and **Filter tests** take [queries](search-and-filters.md), for example `folder:Checkout` and `type=automated`.
- **Only linked tests** hides tests that are not linked to any shown requirement.
- **View** switches between **Matrix** and **List**. The list shows each requirement with its linked tests and is easier to read on phones. **Rules** shows the [traceability rules](#traceability-rules).

**Link and unlink:** click (or focus and press <kbd>Enter</kbd> or <kbd>Space</kbd> on) an empty cell to link the requirement and the test; click a marked cell to remove the link. This needs the **Tester** role. Links can also be managed in the requirement editor (**Link tests**) and the test editor (**Link requirements**).

The matrix scrolls in both directions; the requirement column stays visible.

### Link several at once

With the **Tester** role, a toolbar above the matrix (and the list) works on many links at once:

1. Tick requirements with the checkbox at the start of their row, or **Select all shown requirements** (use the filters first to narrow them down).
2. Optionally choose tests in **Tests for the selected requirements**; with none chosen, the actions take all tests.
3. Choose an action; the number on each button says how many links it affects:
   - **Link** links every selected requirement to every chosen test; existing links are kept.
   - **Unlink** removes the links between the selected requirements and the chosen tests (after a confirmation).
   - **Confirm suspect** confirms the suspect links among them (cells with a warning sign: the requirement or test changed after the link was confirmed).

Ticking only tests (no requirements) lets **Unlink** and **Confirm suspect** act on all links of those tests. Linking and unlinking can be undone with **Undo**; the status that follows evidence is updated once for all affected requirements.

## Traceability rules

Rules say how the items of a project must be traced, so that gaps show up long before an audit or a release. A rule says: *the items that match a query must have at least N traces of a kind*, for example *every MUST requirement is verified by at least one test*.

Select **Traceability** in the project sidebar and **View → Rules**. Every rule shows:

- a green tick when no item breaks it, a red cross for a broken **error** and an amber one for a broken **warning**;
- which items it applies to and what they need, for example *Requirements matching obligation:must needs at least 1 linked test (not suspect)*;
- how many items match it and how many of them break it. **Show violations** lists the items that break it with what is missing (*has 0, needs 1*, or *1 of 2 qualify, needs 1* when some traces do not meet the conditions); select a key to open the item.

**Only violations** hides the rules that pass and the rules that are turned off; **Filter rules and items** finds a rule by name or a violating item by key or title. The requirement and test editors show the rules the item breaks above the fields, with a link to the rules.

### Create and edit rules

Rules are managed by users who can change the project settings (permission `project.update`, by default **Manager** and **Admin**); everyone else sees the results. Select **New rule**, optionally **Start from** a preset, and fill in:

| Field | Meaning |
|---|---|
| **Name**, **Description** | what the rule is for |
| **Severity** | **Error** or **Warning**. Errors make the rules fail; release exit criteria can count errors only or warnings too |
| **Rule is on** | rules that are off are listed but not counted |
| **Applies to** | requirements or tests |
| **Items the rule applies to** | a [query](search-and-filters.md), for example `obligation:must type:security` or `folder:Payments`; empty applies the rule to every item |
| **Must have** | the kind of trace: **Tests that verify it** (trace links of a requirement), **Requirements it verifies** (trace links of a test), **Relations to requirements** or **Item links** |
| **At least** | how many traces an item needs (default 1) |
| **Relation type**, **Direction** | for relations: for example *Refines* and *incoming* means *refined by child requirements*; *any relation* and *both directions* count every relation |
| **Link to**, **Direction** | for item links: what the link must point to (a requirement, test, run, file, report, web address or anything) |
| **Only … that match** | a query on the item at the other end, for example `type:functional` to count only functional child requirements |
| **Only links that are not suspect** | suspect links do not count |
| **Only tests whose last result passed** | tests that have not passed do not count |
| **Every trace must meet the conditions** | an item breaks the rule as soon as one of its traces does not meet the conditions (for example has a suspect link). With it, **At least** may be 0 |

**Try it** evaluates the rule without saving it and says how many items match and break it. Use the eye button to turn a rule off and on, the pencil to edit it and the trash icon to delete it; all of these can be undone.

The presets:

| Preset | Rule |
|---|---|
| Every MUST requirement is verified by a test | requirements matching `obligation:must OR obligation:shall` need at least 1 linked test (error) |
| Every MUST requirement has a passed test | as above, but the test must not be suspect and its last result must be passed (error) |
| No suspect links on MUST requirements | every link of a MUST or SHALL requirement must not be suspect, at least 0 (warning) |
| Every test verifies a requirement | every test is linked to at least 1 requirement (warning) |
| Business requirements are refined by at least one requirement | requirements matching `type:business` have at least 1 incoming *refines* relation (warning) |
| Security requirements are verified by two tests | requirements matching `type:security` need at least 2 linked tests (warning) |

The rules are also evaluated on a release's scope by the exit criterion [Traceability rules](releases-and-risks.md#exit-criteria), and the **Traceability** report lists every enabled rule with its result and the items that break it.

### Built-in consistency checks

Besides the rules you write, Qtestframe has **built-in checks** with a fixed condition. They find items whose state contradicts their relations, links or release:

| Check | An item breaks it when |
|---|---|
| Conflicting requirements in a release | two requirements related by *conflicts with* are both approved (or implemented or verified) and in the same release |
| Approved requirement with an unmet dependency | an approved requirement *depends on* one that is rejected, obsolete or deleted (still in the trash) |
| Active requirement with an obsolete parent | a requirement that *refines* or is *derived from* an obsolete requirement is not rejected or obsolete itself |
| Test verifies an inactive requirement | a test verifies a rejected or obsolete requirement |
| Broken test prerequisite | a prerequisite of a test is deleted, its latest result failed, or prerequisites form a cycle |
| MUST requirement in a release without passing evidence | a MUST or SHALL requirement in a planned or active release has no linked test that is not suspect and passed its latest execution |
| High risk without verified mitigation | a high or critical risk (not closed or accepted) has no linked requirement, or not every one of its requirements is verified (a risk whose requirements are all verified counts as mitigated) |
| Regulatory requirement rejected without justification | a requirement added from a regulation of the [catalogue](catalogue.md) (a law or directive) is rejected, and it has no comment and no rationale of its own |
| Suspected duplicate requirements | two active requirements have nearly identical title and description (similarity of at least 0.9 by default) and no *duplicates* relation |

A project gets them with **Add built-in checks** (on the rules or in **Analytics**): they are added as rules with the severity *warning*, so they show up in the release exit criterion [Traceability rules](releases-and-risks.md#exit-criteria) only when it counts warnings, or after you raise a check to *error*. A built-in check is marked **Built-in**; you can rename it, change its severity, turn it off, narrow it with a query and, for the duplicate check, set the **Similarity threshold** (0.5 to 1) or, for the risk check, the **Least risk level**.

Each violation names the other items involved (the conflicting requirement and the release, the obsolete parent, the failing prerequisite …), each a link, and offers a fix where one is obvious: **Remove relation**, **Unlink**, **Remove prerequisite**, **Remove from** the release, **Set obsolete**, **Record a justification**, **Link a mitigation**, and for a suspected duplicate **Mark as duplicate** (adds the *duplicates* relation) or **Not a problem** (the pair is not reported again). **Not a problem** also acknowledges a dependency on a deleted item.

## Change impact

A change of a requirement rarely stays with that requirement. When the **substance** of a requirement changes (its title, description, obligation or criteria), Qtestframe marks what may no longer be right:

- the tests that verify it: their links become [suspect](#the-traceability-matrix);
- the requirements related to it: by default those that **refine**, are **derived from** or **depend on** it. Their relation becomes suspect, and so do the links of their tests;
- its issue in the tracker: it is marked **out of date** until you update the issue from the requirement (**Update issue**) or select **The issue is still right**;
- the generated documents in the archive that contain it: they are shown as **Outdated** in **Documents**, with the items that changed since;
- the latest results of its tests: they become [stale](#stale-results).

A change of a test's title, description, preconditions, steps, expected result, script or execution type makes its links suspect and its latest result stale.

### A change travels on

What a change makes suspect can in turn change, so the impact travels on: when A changes, B (which refines A) becomes suspect, then C (which refines B), and so on, up to the project's **impact depth** (3 by default, at most 10; see [How far a change travels](#how-far-a-change-travels)). Every item is reached once, along the shortest way, even when the relations form a cycle, and the dialogs show the **path** to it: *A → refined by → B → refined by → C → verified by → T-12*.

Besides the requirement relations and the tests, a change reaches, as **Review recommended**:

- the **risks** a changed requirement addresses (and, from a risk, the other requirements that address it);
- the **open defects** of an affected requirement or test;
- requirements and tests connected by **item links**, and linked runs;
- tests that **call** a changed test as shared steps or **depend** on it;
- the **variants** an affected item applies to (when chosen explicitly) and the **code changes** (commits, merge requests) that refer to it.

These are not marked; their owners can be notified. A test that changes affects the requirements it verifies, not what lies beyond them. A change of the **status** alone (for example in a bulk status change) only recommends reviews. Tracker issues, documents and results belong to the changed item itself.

### Before you save

Every change goes through the same preview: saving a requirement or test, a **bulk edit** or **bulk status change** in the list, an **Excel** or **ReqIF** round-trip import, **restoring a version** from the history and **restoring items from a baseline**. When something would be affected, a dialog shows it as a tree grouped by type and depth, with the counts by type and by depth, and for each item whether it **Becomes suspect** or only **Review recommended**, and the path along which the change reaches it. Choose what to do:

| Choice | What happens |
|---|---|
| **Mark suspect** (the default) | Marks what the project's rules mark suspect, flags the tracker issues out of date and notifies the owners of the items recommended for review |
| **Notify owners only** | Marks nothing; the owners (requirement and risk owners, defect assignees, test authors) get one notification each, listing their affected items |
| **Skip** | Marks nothing and notifies nobody. Results recorded before the change still go stale and documents still become outdated: those follow from the change itself |

Select **Save** to save with your choice, or **Cancel** to change nothing. Items that are already marked are shown as such and are not counted again.

Through the API, every changing endpoint takes `?dry_run=impact` (the change is made in a transaction, its impact returned and everything rolled back) and `?impact=suspect|notify|skip`.

### The Impact tab

The **Impact** tab of a requirement or test shows the same list at any time, under **If this requirement changes** (or **If this test changes**). For a requirement it also shows:

- **Blocking defects**: the open defects that block it or warn about it (see [Blocking defects](#blocking-defects));
- **Suspect now**: its suspect relations, with **Confirm** for each (and **Confirm all**), and the linked tests whose result is stale.

The badges next to the requirement's key (**Blocked by 1 defect**, **1 stale result**, **1 suspect relation**) open the tab. For a test, the tab also shows its **Regression history** (see [Defects](defects.md#regressions)). **Impact analysis** opens the item in the [impact analysis](#impact-analysis).

### The impact of a baseline

In **Baselines**, compare a baseline with the current state and select **Impact**. The dialog lists every requirement and test whose substance changed since the baseline, the largest impact first, with the totals of the items affected. Open an item to see what its change makes suspect, out of date, outdated or stale.

### Impact analysis

**Impact analysis** (in the project's sections) shows the impact of one or more items, or of everything that changed since a baseline, without changing anything. Select **Requirements** or **Tests** to pick items, or choose a baseline under **Changes since a baseline**, set the **Impact depth** and select **Analyse**. The result shows the counts by type and depth and either a **Graph** (the changed items on the left, every affected item to the right of the item it is reached from; dashed: review recommended) or a **Table** with the type, depth, effect, owner and path of every item. **CSV** and **PDF** export the analysis. The address of the page keeps the choice, so you can send an analysis as a link.

### Along which relations

**Categories & settings → Change impact** decides, per relation type, which requirement becomes suspect when the one at the other end changes, and whether it becomes **suspect** or only gets a **review recommended**:

| Setting | Meaning |
|---|---|
| **Incoming (the requirement pointing at it)** | A *refines* B: when B changes, A becomes suspect (the default for refines, derived from and depends on) |
| **Outgoing (the requirement it points at)** | A *provides* B: when A changes, B becomes suspect |
| **Both directions** | either end changing makes the other suspect |
| **Off** | changes do not travel along this type (the default for the other types) |

The mode next to each type (**Becomes suspect** or **Review recommended**) applies to everything reached through it: an item is suspect only when every step on its path marks suspect.

**Mark the tracker issues of a changed requirement out of date** can be turned off.

### How far a change travels

**Impact depth** (1 to 10, default 3) is the number of steps between requirements, tests and risks a change travels. Depth 1 is the direct impact only: the requirements related to the changed one and the tests of both. The verifying tests of an affected requirement and the defects, variants and code changes of an affected item do not count as a step.

**Other connections** turns the other kinds of connection on or off: trace links (they can mark suspect or only recommend a review), shared steps, test dependencies, item links, risks, defects, variants and code changes (these only recommend a review).

## Stale results

A test result goes **stale** when

- it is older than the project allows (by default 90 days; **Results go stale after** can be changed or turned off), or
- the test, or a requirement it verifies, changed in substance after the result was recorded (**Results go stale when the test or a requirement it verifies changes**).

A stale result does not count as passing evidence. A requirement does not become **Verified** on it (and a verified one goes back to **Implemented**), coverage counts it as not run, and the release exit criteria **Passed** and **Mandatory passed** list the requirement with *stale result*. Stale results have a **Stale** badge in the test list, in the coverage column of the requirement list, in the traceability matrix and in reports. Run the test again to get a fresh result.

In the search, `stale:true` finds tests with a stale latest result and `stale>0` requirements with one.

## Blocking defects

An open defect linked to a requirement, to a test that verifies it or to an execution of such a test **blocks** the requirement, or only **warns** about it, depending on its severity. By default critical and major defects block, minor ones warn and trivial ones do neither; **Categories & settings → Change impact → Defects that block requirements** changes this per project.

Blocked requirements show **Blocked** in the requirement list and in the traceability matrix, the requirement shows **Blocked by 1 defect** next to its key, and the **Quality** section of the dashboard counts blocked requirements, requirements with warnings, stale results and suspect relations. In the search, `blocked>0` finds blocked requirements. The release exit criterion **No blocking defects** fails while a requirement in scope is blocked.

## How coverage is calculated

Coverage is derived from the **last result** of every linked test. It is computed every time you look at it and never stored, so it is always current.

**Last result of a test:** among the test's executions in runs of this project that are not *Not run*, the one with the latest time (finished, else started, else the run's creation time). A test without such an execution has the last result **Not run**.

**Coverage of a requirement**, evaluated top-down:

| Coverage | When |
|---|---|
| **None** | no test is linked |
| **Failed** | at least one linked test is *Failed* or *Blocked* |
| **Passed** | all linked tests are *Passed* |
| **Partial** | at least one linked test is *Passed*, the others are *Not run*, *Skipped* or *In progress* |
| **Not run** | otherwise: tests are linked, but none has passed or failed yet |

Examples:

- REQ-1 is linked to TC-1 (passed) and TC-2 (passed) → **Passed**.
- REQ-2 is linked to TC-1 (passed) and TC-3 (not run) → **Partial**.
- REQ-3 is linked to TC-1 (passed) and TC-4 (blocked) → **Failed**.
- REQ-4 has a linked test that was never executed → **Not run**.
- REQ-5 has no links → **None**.

**Configurations:** when tests are executed in [configurations](runs.md#configurations-in-a-run), every configuration a test is planned in counts **as a result of its own**: the last result of the test *in that configuration* (an execution without configuration counts as one more configuration). The rules above are then applied to all these results together. So a requirement is **Passed** only when each of its tests passed in **every configuration** it is planned in; one failed or blocked configuration makes it **Failed**; a test that passed in Chrome but has not been run in Firefox yet makes it **Partial**. A configuration counts as planned as soon as the test has an execution in it in any run of the project, even one that has not been run yet.

- REQ-6 is linked to TC-5, planned in *Chrome* (passed) and *Firefox* (passed) → **Passed**.
- REQ-7 is linked to TC-6, planned in *Chrome* (passed) and *Firefox* (not run) → **Partial**.
- REQ-8 is linked to TC-7, planned in *Chrome* (passed) and *Safari* (failed) → **Failed**.

Without configurations nothing changes: each test has one result, its last result. A release counts a test as passed only when it passed in every configuration of the release's runs.

**Coverage by configuration:** the requirement editor shows, below the linked tests, a table with the last result of each linked test in each configuration and the coverage **in each configuration** (counting only the tests planned in it). In the traceability matrix, **Coverage in configuration** shows the coverage and the test results of one configuration only, or of the executions without configuration.

Things to keep in mind:

- Coverage is calculated per requirement, not per evaluation criterion; every link counts the same. Which criteria the tests and other evidence verify is shown by [Verification](#verification).
- Only runs of the same project count. A test that was moved from another project keeps its old executions, but they no longer count.
- A newer *Passed* result replaces an older *Failed* one; there is no need to delete old runs.

Coverage appears as a column and quick filter in the requirements list, as the query field `coverage` (for example `coverage:failed` or `coverage:none category:Security`), in the matrix, in exports, in reports and on the dashboard.

## Verification

Coverage says whether the linked tests passed. **Verification** goes one step further: a requirement counts as **verified** only when **every evaluation criterion has passing evidence**, and — in a hierarchy — when every requirement that refines it is verified too. Evidence is a passed test, a passed test step, or a **verification record** for an inspection, analysis, demonstration or review. So hardware, facility and governance requirements that no test can verify can be verified too.

Open a requirement and choose the **Verification** tab. It shows the state with the reasons when the requirement is not verified (for example *Criterion 2 has no evidence.*), the evidence of every criterion, the requirements that refine it, the verification records, the criteria of the linked tests and the system elements the requirement is allocated to. The badge next to the coverage in the editor's header opens the tab.

| State | When |
|---|---|
| **Verified** | every criterion (and every child) has passing evidence |
| **Partly verified** | some of it is verified, not all |
| **Failed** | some evidence failed: a test failed or was blocked, a record's result is *Fail* |
| **Not yet passed** | there is evidence, but none of it has passed yet (not run, a suspect link, a step not executed) |
| **No evidence** | no test and no verification record |

### Tests for criteria and test steps

A test linked to a requirement can be mapped to the criteria it verifies. In the **Verification** tab choose **Map criteria** next to the test, tick the criteria and, if you want, the **step** of the test that shows it. A criterion mapped to a step is passed when that step passed in the test's latest execution. When you link a test you can also choose a criterion right away (the selector next to **Link tests**).

A link **without** criteria counts for the whole requirement, as before: for every criterion that has no evidence of its own. A project that wants every criterion to have its own evidence turns on **strict criterion coverage** under **Settings → Project → Verification**. A failing test always fails the requirement, mapped or not. There, too, a project can let debriefed [exploratory sessions](exploratory-testing.md#sessions-as-verification-evidence) count as evidence for the requirements in their scope (off by default).

### Verification records

For a criterion that is verified by inspection, analysis, demonstration or review choose **Record evidence for criterion n** (or **Record evidence** for the whole requirement) and fill in:

- **Method** and **Result**: *Pass*, *Fail* or *Conditional pass* (counts as passed and is shown as conditional);
- **Performed by** and the **Date**;
- a **Note** and **Documents from the archive** (for example the inspection protocol uploaded to [Documents](documents.md));
- **Valid until**: the record stops counting after that day; **Valid until the requirement changes**: when the title, description, obligation or criteria change, the record is invalidated (like a suspect link) and has to be recorded again.

For each criterion only the newest valid record counts; older ones are shown as superseded. A second person can **Sign off** a record (anyone with the review or execute permission except its author and its performer). Changing the method, result, criterion, dates or performer removes the sign-off.

### Requirement hierarchy and roll-up

The hierarchy is built from the relation **refines**: a requirement that refines another is its child (see [Relations](requirements.md)). A parent is verified only when its own criteria are verified and all its children (except rejected and obsolete ones) are. A parent without criteria or evidence of its own is verified when all its children are.

**Verification** in the project navigation shows the hierarchy as a tree with badges: the state, the criteria verified (*2/3 criteria*) and the children verified (*1/2 children*). **Only not verified** keeps the branches that still need work.

With the default setting the status follows the evidence (**Administration → Test execution**, see [Test execution settings](administration.md#test-execution-settings)): an implemented requirement becomes verified when it is verified, and a verified requirement falls back to implemented when a child or criterion no longer is — so a verified parent with an unverified child cannot remain. When the organisation verifies manually, such a requirement is listed under **Verification → Inconsistencies** and in the [analytics](#requirement-analytics) (*Verified requirements with unverified children*).

### System elements

**Verification → System elements** holds the product breakdown of the project: systems, subsystems, components, products and suppliers, as a tree. Requirements and tests are allocated to elements in the **Verification** tab of a requirement (and with the API). Every element shows how many of the requirements allocated to it **or to an element below it** are verified (*42/50 verified*), how many have evidence or failed, and how many of its tests passed. The counts link to the lists filtered by the element.

In the query language `element:"Power unit"` finds the requirements and tests allocated to *Power unit* or to an element below it, and `verification:verified`, `verification:none` … filter by the verification state.

The **Verification** report and the requirements specification show the same: the report has the evidence per criterion, the requirements that are not verified and why, the records, the roll-up, the inconsistencies and the coverage per element; the specification shows the verification of every requirement and criterion.

## The dashboard

**Dashboard** is the first page of every project. It has three tabs: **Overview** for the state today, **Trends** for how it changed and **Quality** for what needs attention. The tab is part of the address, so a link can open the dashboard on a given tab. On a phone the charts are stacked.

![The dashboard with key figures, requirement coverage, execution results and recent runs](../images/en-dashboard.png)

**Overview**

- **Key figures**: number of requirements (and how many are uncovered), coverage (how many are covered by tests), number of tests (manual and automated), test runs (in progress) and the pass rate of all executions.
- **Requirement coverage**: how many requirements are *Passed*, *Failed*, *Partial*, *Not run* and *None*.
- **Execution results**: all results of all runs.
- **Progress of active runs**: one bar per planned or running run (the newest eight), split into passed, failed, blocked, skipped and not run.
- **Recent runs**: the five newest runs with their progress; **All runs →** opens the run list.
- **Requirements by status** and **Requirements by obligation** (requirements without a keyword count as *unspecified*).
- **Tests by status**.
- **More key figures** (below the first ones): **Verified requirements** (the share of requirements whose linked tests all passed), **Open risks** (with how many are critical and high), **Open defects** (with their average age), **Suspect links** (links whose requirement or test changed since, and how many requirements have one, see [the traceability matrix](#the-traceability-matrix)), **Trace-rule violations** (items that break the enabled [traceability rules](#traceability-rules), and how many rules are broken) and **Release progress** (the share of executions with a result in the current release).
- **Manual and automated tests by last result**: one bar for manual and one for automated tests, split into passed, failed, blocked, skipped and not run.
- **Requirement review state**: the requirements by the state of their latest review request: *Not reviewed*, *Review pending*, *Changes requested* or *Approved* (cancelled requests do not count). See [Reviews](reviews-and-notifications.md).
- **Release execution progress**: for the current release (the active one, else the planned one with the earliest target date), the results of all executions in its runs, and how many have a result and passed. The release name opens it.

On the dashboard, a requirement counts as *covered* as soon as at least one test is linked, regardless of results. *Verified* requirements are those whose linked tests all passed.

**Trends**

- **Coverage and pass rate over time**: the coverage (share of requirements with linked tests) and the pass rate (share of executed tests whose latest result passed) per day. Choose the period above the chart: 30, 90 or 365 days.
- **Defects opened and closed per week**: new defects and defects that were resolved, closed or rejected, per week (Monday to Sunday).
- **Activity in the last 30 days**: per day, the changes to requirements and tests (every saved version), the executions finished and the defects reported. This chart always covers the last 30 days, whatever the period above.
- **Release burn-up**: for the release chosen in the list, the requirements in its scope, how many of them have linked tests and how many have all their tests passed in the release's runs, day by day from the creation of the release. The target date of the release is shown above the chart. See [Releases](releases-and-risks.md).

The trends come from a snapshot of the figures that Qtestframe saves once a day for every project (and when the dashboard is opened and today's snapshot is missing). A new project, or a project from before this feature, does not start empty: on the first snapshot, the earlier days are reconstructed from what has dates — when requirements, tests and links were created, the results of the runs and when defects were reported and resolved (up to a year back). Days without a snapshot are bridged by the line.

**Quality**

- **Flaky tests**: the ten tests whose results change most often between passed and failed, with their latest results (oldest first; a ring marks every flip) and when the result last flipped. Click a test to open it. See [Flaky tests](tests.md#flaky-tests).
- **Open defects by severity**: open and in-progress defects per severity.
- **Requirement coverage by group**: the coverage per top-level folder (requirements in subfolders count for the top folder) or, with **Category**, per requirement category; the twelve largest groups.
- **Open risks by level**: open risks (*Identified* or *Mitigating*) per level; **Risk register →** opens the [risk register](releases-and-risks.md#the-risk-register).
- **Risk heat map**: a small 5 × 5 table of the risks that are not closed, by likelihood (rows, 5 at the top) and impact (columns). Besides its colour, each level has its own border (dotted for medium, dashed for high, solid with stripes for critical), explained below the table.
- **Mitigation of open risks**: the open risks by how far the requirements that address them are tested: *No requirements*, *Untested*, *Partly tested*, *Tested, not all passed* and *Verified* (as in the [risk report](reports-and-export.md#standard-reports)).
- **Age of open defects**: open and in-progress defects by age (under 7 days, 7–30, 30–90, over 90 days), split by severity, with the average and the oldest age.
- **Traceability rules**: whether all enabled rules of severity *error* are met, how many rules are enabled, how many are broken (errors and warnings) and the number of violations. **Traceability →** opens the matrix with the rules.
- **Quality metrics** and **Consistency checks** (the failing ones): the same as in [Analytics](#quality-metrics).

The further key figures and these charts are counted on the server in one request, loaded next to the rest of the dashboard.

Every chart has a text alternative with its figures for screen readers, and the legend names each colour. The heat map is a table with a caption, and every cell names its likelihood, impact, level and number of risks.

## Dependency graphs

**Dependency graph** in the project sidebar draws how items build on each other. Choose **Requirements** or **Tests** at the top:

- **Requirements**: the relations between requirements, such as *depends on*, *provides*, *refines*, *derived from*, *conflicts with*, *duplicates* and *relates to* (see [Relations](requirements.md#relations));
- **Tests**: the [dependencies](tests.md#dependencies) between tests (*depends on*) and the shared steps (*calls*).

The graph is read from left to right: prerequisites, parent requirements and sources are on the left, what builds on them to the right. Each box shows the key, the title and the coverage (requirements) or last result (tests), with a coloured bar in the same colours as elsewhere. Each relation type has its own colour and line style, explained next to its checkbox.

- Untick a relation type to hide those lines; items left without relations disappear unless **Show items without relations** is on.
- Type in **Highlight items…** to highlight matching items.
- Right-click an item (or focus it and press <kbd>Space</kbd>) to highlight its neighbours; **Show all** ends this.
- Select an item (or press <kbd>Enter</kbd>) to open it. Zoom with the magnifier buttons; scroll to move around.
- **Rotate 90°** (the turning-arrow button next to the zoom) lays the graph out from top to bottom instead of left to right: prerequisites and parents are then at the top. The choice is kept in the address (`dir=vertical`) and in your browser.
- **Print or save** prints the graph on its own page (landscape when it is wide) or saves it as an **SVG** image (sharp at any size, keeps the links to the items), a **PNG** image, or **Markdown** with a Mermaid diagram (drawn by GitHub, GitLab and many editors) and a table of the items and their relations. The saved files show what is currently on screen: the chosen relation types, direction and items.
- **List** shows the same relations as a table, one item per row, which is easier to read with a screen reader or to print.

Cycles (for example two requirements that depend on each other) are drawn, but one of their lines is shown without an arrow.

## Requirement analytics

**Analytics** in the project navigation analyses all requirements of the project at once (open it again or select **Analyse again** after changes). At the top: the **health** (the share of requirements without problems or warnings), the number of requirements and tests, the share **covered by tests** (rejected and obsolete requirements are not counted) and the average **language score**.

- **Obligation levels** shows how many requirements have each level, including those with **no obligation**.
- **Keywords per meaning** groups the keywords that mean the same (MUST, SHALL and REQUIRED; MAY and OPTIONAL …) and shows how each is used, so you see the convention of the project at a glance.

**Findings** lists what the analysis noticed, problems first, each with an explanation, **what to do** and the requirements (or tests) concerned; select a key to open the item. Filter by severity or category:

| Category | Findings |
|---|---|
| Obligation levels | keywords not written in capitals as RFC 2119 and RFC 8174 require ("must", "Should"), and requirements whose keywords are *only* in lower case and so have no normative keyword at all; WILL used as an obligation (it is not an RFC 2119 keyword); requirements without an obligation level; a level in the field but no keyword in the text; field and text disagree; several levels in one requirement; the spread: almost everything mandatory, no mandatory requirements, many optional (MAY) or many SHOULD or descriptive requirements |
| Consistency | requirements in more than one language (for example mostly English with a few Swedish ones), a requirement whose title and sentences are in different languages, and English keywords in a requirement written in another language (MUST in a Swedish sentence); a keyword used by a few requirements while another with the same meaning dominates, for example *SHALL is used in 1 requirement, while MUST is used in 95 %*; keywords with the same meaning used about equally |
| Coverage | mandatory and other requirements without tests; failing or not yet run tests; requirements without acceptance criteria |
| Tests | tests that verify no requirement; accepted requirements without a test that is Ready |
| Quality | empty descriptions, duplicate titles, conflicting requirements, low language scores, many drafts, requirements without an owner |
| Language | the findings of the [language analysis](requirements.md#the-language-analysis) summed up per rule: ambiguous terms, TBDs, escape clauses … |

**Languages of the requirements** shows how many requirements are written in each language. Only texts long enough to tell the language are counted; a title of one or two words is not enough.

**Language** chooses the language of the analysis (detected per requirement by default). Test findings are shown when you may read tests.

### Consistency checks

**Consistency checks** in **Analytics** evaluates every [built-in consistency check](#built-in-consistency-checks) on the project, also those it has not added as rules yet (marked **Default**; they do not count for releases until they are added). **Show violations** lists the items with the items involved and the fixes.

### Quality metrics

**Quality metrics** in **Analytics** (and on the dashboard's **Quality** tab) measures the project from its history. Choose the period (30, 90, 180 or 365 days) and after how many days a latest result counts as stale. Trends are per week, or per month for periods over 120 days.

| Metric | What it measures |
|---|---|
| **Requirements volatility** | the changes to requirements after their creation (every saved version; moves do not count), per period, as changes per requirement and period, and the most changed requirements |
| **Defect density** | defects per active requirement, per folder and per category; a defect counts for a requirement when it is linked to it, to a test that verifies it or to an execution of such a test |
| **Escaped defects** | defects found in production (the defect's **Found in** field, see [Defects](defects.md)) or reported after a release they concern was released; the share of all defects, per phase found and per release |
| **Test effectiveness** | defects found by tests (linked to a test or one of its executions) per test executed and per execution hour; executions without a duration are not counted in the hours |
| **Result age** | how old the latest result of each test is (0–7, 8–30, 31–90, 91–180 and more days, never run) and how many are older than the limit |
| **Review turnaround** | the time from a review request to its decision (median, mean and 90th percentile), and the pending requests |
| **Time to fix defects** | the time from reporting a defect to resolving it, per severity (rejected defects do not count) |
| **Requirement quality** | the share of active requirements whose text has warnings of the language analysis; the trend replays each requirement's text from its history |

Every chart has a text alternative and **Show as table** shows its figures as a table. **Export as report** opens the reports: the standard report **Quality metrics** contains the same figures.

### Compliance with laws and standards

At the bottom of **Analytics**, **Compliance with laws, standards and best practices** shows how far the project meets the regulations and standards of the [catalogue](catalogue.md#comply-with-laws-standards-and-best-practices). Without a choice it shows every source the project uses; choose others in the list (grouped by kind, with a search field) to see what is missing for them too.

For each source:

- **Catalogue requirements** are the catalogue requirements that cite it. One is **adopted** when the project has a requirement added from it that is not rejected or obsolete; otherwise it is **missing**. **Own** requirements are project requirements that were not added from those items but name the source in their **Source** field, for example `GDPR Art. 32; ISO/IEC 27001:2022 A.8.5` (written as in the catalogue's *Based on*; upper and lower case do not matter).
- **Fulfilment**: a requirement is *fulfilled* when it has tests and the latest result of every one of them is passed, *failing* when one failed, *blocked* when one is blocked, and *untested* otherwise. When the project has several copies of one catalogue item, the worst state counts.
- **Compliance** is the share of fulfilled requirements of all catalogue and own requirements for the source; **Adopted** is the share of catalogue requirements the project has; **MUST requirements** is the compliance of the mandatory ones. The bar shows the fulfilled, failing, blocked, untested and missing requirements.
- **By obligation** splits the counts into MUST (including MUST NOT), SHOULD and MAY. **By clause** gives the compliance per article, annex or control group, taken from the citations (`Art. 32`, `A.8`, `Annex I Part I`, `SR 1` …; *(general)* for citations of the source as a whole).
- **Missing requirements** lists the gaps, MUST first: select one to open it in the catalogue, or **Add the missing from the catalogue** to open the catalogue with the source chosen. **Adopted but not fulfilled** lists the project's requirements that still fail or lack passing tests; select a key to open the requirement.

The standard report **Compliance** (see [Reports and export](reports-and-export.md)) contains the same figures, per source and clause, with the gaps. The analysis helps you plan and follow up; it is not a legal assessment.
