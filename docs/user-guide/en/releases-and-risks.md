# Releases and risks

A **release** is a version of your product that you plan, test and ship, for example *Spring 2027* or *2.1.0*. It brings together the requirements it must deliver (its **scope**), the test runs that test it and the **exit criteria** that decide whether it may ship. The **risk register** lists what could go wrong, how likely and how bad it would be, and which requirements address each risk. Open risks feed straight into the exit criteria of a release.

Releases and risks are in the project sidebar.

| Action | Role needed |
|---|---|
| View releases, their status and the risk register | Viewer |
| Create, edit and delete releases, change their scope and exit criteria, mark them released | Manager |
| Create, edit and delete risks and link requirements to them | Manager |
| Put a run in a release | Tester |

## The release list

**Releases** lists every release of the project with its version, status (*Planned*, *Active*, *Released* or *Cancelled*), target date, number of requirements and runs, and a **Go** or **No-go** badge. Select a release to open its details below the list.

To create a release, select **New release** and enter a name and, if you like, a version, status, target date and description. A new release starts with the default exit criteria:

- coverage at least 100 %,
- every MUST and SHALL requirement passed,
- at least 95 % of the requirements passed,
- no failed tests,
- no suspect links.

## Scope

The **Scope** tab lists the requirements the release must deliver, with their status, coverage and risk level.

- Select **Add requirements** to pick requirements from a list.
- Or enter a filter in the query language, for example `tag:checkout` or `risk:high`, and select **Add matching**: every requirement that matches is added. See [Search and filters](search-and-filters.md) for the syntax.
- Select **×** on a row to remove a requirement from the scope.

A requirement can be in the scope of several releases.

## Runs in a release

A run belongs to a release when you choose the release under **Release** in the **New run** dialog or in **Edit run** on the run page. Choose **No release** to take it out again. The **Operations** list shows the release of each run in its **Release** column, and the run page shows it next to the environment and build.

Only results of the release's runs count for the exit criteria: a test that passed in a run of another release does not make this release pass.

The **Runs** tab of a release lists its runs with their status and progress; select a run to open it.

## Exit criteria

The **Exit criteria** tab lists the conditions the release must meet. Select **Add criterion**, choose a type and, where the type has one, a threshold or level; select the trash icon to remove one. Select **Save criteria** to keep the changes.

| Criterion | Met when | Parameter |
|---|---|---|
| Coverage | at least the threshold of the scope requirements have a test | threshold in % |
| Passed | at least the threshold of the scope requirements passed all their tests in the release's runs | threshold in % |
| Mandatory passed | every MUST and SHALL requirement in scope passed | — |
| Executed | at least the threshold of the executions of the release's runs have a result | threshold in % |
| No failed tests | no test failed or is blocked in the release's runs | — |
| Approved | at least the threshold of the scope requirements are approved, implemented or verified | threshold in % |
| No suspect links | no scope requirement has a [suspect link](traceability-and-coverage.md) | — |
| No open defects | no open defect of at least the chosen severity is linked to a scope requirement or its tests | minimum severity |
| Risks mitigated | every risk of at least the chosen level that a scope requirement addresses is mitigated, accepted or closed | minimum risk level |
| Traceability rules | no requirement in scope and no test linked to one breaks an enabled [traceability rule](traceability-and-coverage.md#traceability-rules) of the chosen severity; the status tab lists the items with the rules they break | errors only, or errors and warnings |
| No blocking defects | no requirement in scope is [blocked by an open defect](traceability-and-coverage.md#blocking-defects) under the project's severity rules; a stale pass does not count for **Passed** and **Mandatory passed** | — |
| Residual risk | no risk linked to the scope that is not closed has a [residual level](waivers-and-soa.md#residual-risk) above the threshold, unless its acceptance is signed | the project's risk acceptance threshold, or a level |

A release without exit criteria is always a go.

An approved [waiver](waivers-and-soa.md#waivers-and-deviations) for the release or for the whole project makes the criteria accept its requirements: they are listed under each criterion as *waived* instead of blocking it. The **Status** tab also lists the waivers in force and the risks of the scope above the risk acceptance threshold.

## Go / no-go

The **Status** tab evaluates the exit criteria now. It shows **Go** when every criterion is met and **No-go** otherwise, how many criteria are met, and a summary of the scope (requirements, how many have tests, passed, failed and not run) and the runs.

Each criterion shows whether it is met, the actual value next to the target (for example *80 % (target ≥ 95 %)*), and the items that block it: requirements without tests, failed tests, open defects or risks that still need action. Every item is a link that opens it.

The badge in the release list is the same evaluation, so you can see at a glance which releases are ready.

A percentage criterion with nothing to measure is **not met**: a release without requirements in its scope, or without executions for *executed*, shows *Nothing to measure yet* and is a no-go. A release without any exit criteria is always a go.

## Mark a release released

Select **Mark released** on an open release.

- If the release is a **go**, confirm and the release is marked released.
- If it is a **no-go**, the dialog lists the criteria that are not met. You can still release with an **override**: write why in **Why release anyway?** (at least 3 characters) and select **Release anyway**. The note is saved with the release and in the audit log.

When a release is marked released, Qtestframe creates the [baseline](baselines.md) *Release* followed by the release name: it freezes the scope requirements and their tests in the versions they had. The release then shows when and by whom it was released, the override note if there was one, and a link to the baseline.

## Change many releases at once

Tick releases in the list. The toolbar then offers:

- **Set status**. Choosing **Released** releases only the ticked releases that are a go, each with its baseline; releases with a no-go are skipped and listed. To release despite a no-go, use **Mark released** on that release, which asks for the reason.
- **Target date** for all of them (empty clears it).
- **Copy exit criteria** from another release; they replace the criteria of the ticked releases.
- **Add requirements** (pick them) or **Add by filter** (for example `obligation:must tag:payments`) to the scope of every ticked release.
- **Delete**, after confirmation; the runs keep their results.

Every release is checked before any is changed. Changes of status (other than *Released*), target date and exit criteria can be undone with the undo button.

## The risk register

**Risks** lists the risks of the project. Every risk has a key such as *SHOP-RISK-3*, a title, a category, an owner (a project member — type `@` to pick one — or a plain name), a **likelihood** and an **impact** from 1 to 5, a status and a **mitigation**: what you do to make it less likely or less harmful.

| Likelihood | | Impact | |
|---|---|---|---|
| 1 | Rare | 1 | Negligible |
| 2 | Unlikely | 2 | Minor |
| 3 | Possible | 3 | Moderate |
| 4 | Likely | 4 | Major |
| 5 | Almost certain | 5 | Severe |

The register shows the score and level of each risk, its status, owner, the requirements that address it, and how many of them are **covered** (have tests) and **verified** (all their tests passed). Filter it by text, level or status. Closed risks are hidden unless you select **Include closed risks** or filter on the status *Closed*.

To add a risk, select **New risk**. Choose the likelihood and impact (the dialog shows the score and level as you go), describe the mitigation and select **Link requirements** to choose the requirements that address the risk. Select a risk in the register to change it.

The requirement editor shows the risks a requirement addresses under **Risks**, and the requirement list has optional **Risk** and **Open defects** columns (see **Columns**).

### Change many risks at once

Tick risks in the register. The toolbar sets **status**, **likelihood** or **impact** of all of them, **Owner / category** sets either or both (empty clears), **Link requirements** links the same requirements to every ticked risk, and **Delete** removes them after confirmation. Only rows that are shown stay ticked, so filtering the register or choosing a heat-map cell never leaves hidden risks in the selection. Every risk is checked before any is changed. Changes other than linking and deleting can be undone with the undo button.

## The heat map

The heat map counts the risks in a 5 × 5 grid: likelihood from 5 at the top to 1 at the bottom, impact from 1 on the left to 5 on the right. The colour of a cell is the level of its score. Select a cell to show only the risks with that likelihood and impact in the register; select it again, or the **×** on the filter chip, to show all.

Next to the heat map, **Open risks by level** counts the risks that are not closed, per level; select a level to filter the register.

## Risk levels

The **score** of a risk is likelihood × impact, from 1 to 25. The score gives the **level**:

| Level | Score |
|---|---|
| Low | 1–4 |
| Medium | 5–9 |
| High | 10–16 |
| Critical | 17–25 |

The statuses *Identified* and *Mitigating* mean the risk still needs action; *Mitigated*, *Accepted* and *Closed* mean it does not.

## How risks affect a release

A requirement gets the level of the highest **open** risk it addresses. You can filter requirements on it with `risk:critical` or `risk:high`, and on open defects with `defects>0`.

The exit criterion **Risks mitigated** with the level *High* blocks a release while a requirement in its scope addresses a risk of level high or critical that is still *Identified* or *Mitigating*. The status tab lists these risks; mitigate them (or accept them) and set their status, and the criterion is met. A typical workflow:

1. Record the risks and link the requirements that address them.
2. Put those requirements in the release scope.
3. Add **Risks mitigated** with the level you can accept to the exit criteria.
4. Test the requirements in the release's runs; as the mitigation works, set the risks to *Mitigated*.

## The risk report

The **Risk report** under **Reports** summarises the register for a review meeting or an audit: key figures, risks by level and status, the heat map, the top risks, the register, the traceability from each risk to its requirements, their tests and latest results, and the open risks whose mitigation is not tested yet. See [Standard reports](reports-and-export.md#standard-reports). The [dashboard](traceability-and-coverage.md#the-dashboard) shows the open risks per level and a small heat map.
