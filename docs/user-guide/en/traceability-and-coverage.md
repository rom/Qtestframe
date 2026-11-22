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
| High risk without verified mitigation | a high or critical risk (not closed or accepted) has no linked requirement, or none of its requirements is verified |
| Regulatory requirement rejected without justification | a requirement added from a regulation of the [catalogue](catalogue.md) (a law or directive) is rejected, and it has no comment and no rationale of its own |
| Suspected duplicate requirements | two active requirements have nearly identical title and description (similarity of at least 0.9 by default) and no *duplicates* relation |

A project gets them with **Add built-in checks** (on the rules or in **Analytics**): they are added as rules with the severity *warning*, so they show up in the release exit criterion [Traceability rules](releases-and-risks.md#exit-criteria) only when it counts warnings, or after you raise a check to *error*. A built-in check is marked **Built-in**; you can rename it, change its severity, turn it off, narrow it with a query and, for the duplicate check, set the **Similarity threshold** (0.5 to 1) or, for the risk check, the **Least risk level**.

Each violation names the other items involved (the conflicting requirement and the release, the obsolete parent, the failing prerequisite …), each a link, and offers a fix where one is obvious: **Remove relation**, **Unlink**, **Remove prerequisite**, **Remove from** the release, **Set obsolete**, **Record a justification**, **Link a mitigation**, and for a suspected duplicate **Mark as duplicate** (adds the *duplicates* relation) or **Not a problem** (the pair is not reported again). **Not a problem** also acknowledges a dependency on a deleted item.

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

- Coverage is calculated per requirement, not per evaluation criterion. A link can name the criterion a test verifies, but every link counts the same.
- Only runs of the same project count. A test that was moved from another project keeps its old executions, but they no longer count.
- A newer *Passed* result replaces an older *Failed* one; there is no need to delete old runs.

Coverage appears as a column and quick filter in the requirements list, as the query field `coverage` (for example `coverage:failed` or `coverage:none category:Security`), in the matrix, in exports, in reports and on the dashboard.

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
