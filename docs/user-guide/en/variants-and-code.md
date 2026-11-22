# Variants and code

A product is often built in several **variants**: models, markets or customer configurations. Not every requirement applies to every variant, and a test run on one variant says nothing about another. Qtestframe keeps track of which requirements and tests apply to which variants and computes coverage per variant.

The second half of this page is about **code**: the commits, merge requests and pull requests that refer to requirements and tests, the CI **builds** whose results were uploaded, and the **regression selection** that proposes which tests to run after a change.

## Variants

Select **Variants** in the project sidebar, then the **Variants** tab. With the **Manager** role (permission `project.update`) you can add, change and remove variants.

Each variant has:

- a **name**, unique in the project, such as *EU model*;
- optionally a place **below** another variant: variants form a hierarchy, for example *EU* → *EU model DE*;
- a **kind**: product variant, market, customer configuration or other;
- **feature attributes**: names and values such as `region = EU` or `voltage = 230V`. A variant inherits the attributes of the variants above it; its own values win;
- the **test configurations** it is tested in. When you choose some, only results in those configurations (and results without a configuration) count for the variant. Leave it empty to count every configuration. See [test configurations](runs.md).

Removing a variant moves the variants below it up one level. Runs for it become variant-neutral, and requirements and tests that applied only to it apply to all variants again.

## Applicability

A requirement or test **applies** to all variants until you say otherwise. Open it, and in the **Variants** section of its details choose **Change**:

- **All variants**: the default; it also covers variants added later.
- **Selected variants**: the item applies to the chosen variants and to the variants below them. Choosing *EU* covers *EU model DE* too.
- **Expression**: a [query](search-and-filters.md) evaluated against each variant. The fields are `name` and `variant` (the variant and the ones above it), `kind` and the feature attributes. Examples: `region:EU`, `region:EU AND NOT voltage:110V`, `kind:customer OR variant:"EU model"`.

To set the applicability of many items at once, select them in the list and choose **Applies to** in the toolbar.

### History, undo and baselines

A change of the applicability is part of the item's **History** (shown as *Applicability*, with the variants before and after) and of the audit log. **Undo** (Ctrl+Z) takes it back, also for a change of many items at once: each item gets what it had. Restoring an older version from the history restores the applicability of that version too.

[Baselines](baselines.md) record the applicability of every item. **Compare** lists *Applies to (variants)* among the changed fields when it differs, and **Restore** from a baseline gives the items the applicability they had, even when nothing else changed. Variants deleted since are left out. A deleted item keeps its applicability in the trash, and project archives and backups contain it.

### Show for a variant

In the requirement and test lists and on the **Traceability** page, choose a variant in **Show for variant**. The list then shows only the items that apply to it, with the coverage and last results that count for that variant. The query language knows variants too, and the API takes `?variant=` for the traceability matrix, exports, ReqIF and reports:

| Query | Finds |
|---|---|
| `variant:"EU model"` | items that apply to the variant *EU model* |
| `applicability:selected` | items that apply only to some variants |
| `applicability:all` | items that apply to all variants |
| `variant:US` (runs) | runs for the variant *US* |

## Coverage per variant

Coverage is computed per **variant × test configuration**:

1. A requirement counts in a variant when it applies to it. Otherwise its coverage there is **Not applicable**.
2. Only its linked tests that apply to the variant count.
3. Only results that count for the variant are used: results of runs for the variant or a variant above it, and results of **variant-neutral** runs. Results in configurations the variant is not tested in are left out.

When you create a run, choose the **Variant** it tests, or leave it variant-neutral. CI uploads can name the variant too (see [builds](#builds)).

The **Coverage matrix** tab shows every requirement against every variant, with the number of applicable requirements that passed. Filter the requirements with a query, and choose **In configuration** to see one test configuration only. In a requirement's details, the **Variants** section shows its coverage in every variant and, per variant, in every configuration.

### Releases and documents for a variant

On the **Releases** page, choose a variant in **Readiness for variant** to evaluate the exit criteria for it: only the release's requirements that apply to the variant, the tests that apply and the results that count.

### Release gates per variant

A release can **target** one or more variants: edit the release and choose them in **Target variants**. The exit criteria are then evaluated for every targeted variant, with the rules of [coverage per variant](#coverage-per-variant):

- only the release's requirements that apply to the variant, and only their linked tests that apply to it, count;
- only results of the release's runs for the variant or a variant above it, and of **variant-neutral** runs, count, in the configurations the variant is tested in. A variant-neutral run counts for every targeted variant.

The **Status** tab shows the matrix **Exit criteria per variant**: one row per criterion, one column per variant, each cell *Met*, *Waived* (met because waivers or signed risk acceptances accept what is missing) or *Not met*, and the go / no-go of each variant at the bottom. The release is a **go only when every targeted variant is a go**; marking it released needs that go or a release note for the override. In the list of criteria below the matrix, a criterion is met when it is met in every variant, and each blocking item names the variants it blocks in. The [release readiness report](reports-and-export.md) contains the same matrix.

A [waiver](waivers-and-soa.md) can be limited to variants (**Only for variants**): it then accepts its requirements only in those variants and the variants below them. A release without target variants counts only waivers for every variant. A release that targets no variant, and a project without variants, are evaluated as before.

When you [create a document](documents.md), choose **For variant**: the requirements specification or test plan then contains only what applies to the variant, with its coverage and results, and names the variant on the title page.

## Code links

Requirements and tests can be linked to **commits** and **merge or pull requests**. Open an item and select the **Code** tab. It lists the linked changes with their status (open, merged or closed), repository, branch, author and the builds of each commit.

Links are made in two ways:

- **Automatically**: when a commit message, or the title, description or branch name of a merge or pull request, names an item key, the change is linked to it. Full keys (`DEMO-REQ-12`) and short keys (`REQ-12`, `TC-3`, meaning this project's) both work. A key of another project (`OTHER-REQ-1`) is ignored.
- **By hand**: choose **Link commit or MR** in the **Code** tab and give the SHA or number, the address and a title.

### Webhooks

The automatic links come from webhooks. With the **Manager** role, open **Settings → Project → Code integration**:

1. Choose **Generate** (or type a secret of 16 to 200 characters) and **Save**. Copy the secret now: it is never shown again.
2. In **GitLab**, open *Settings → Webhooks* of the repository, enter the **GitLab webhook URL**, put the secret into *Secret token* and tick *Push events* and *Merge request events*.
3. In **GitHub**, open *Settings → Webhooks → Add webhook*, enter the **GitHub webhook URL**, choose *application/json*, put the secret into *Secret* and choose the *Pushes* and *Pull requests* events.

Qtestframe checks every delivery: GitLab must send the secret token, GitHub must sign the body with the secret (HMAC-SHA256). Deliveries without a valid signature are rejected. **Remove secret** turns the webhooks off.

## Builds

A **build** is a CI build that test results were uploaded for. The CI passes it with the upload of [JUnit results](runs.md) or of JSON results, as query parameters:

```
POST /api/v1/runs/{run}/junit?build_id=4711&build_number=1.4.41&commit=3f2c9e1&branch=main&pipeline_url=https://ci.example.org/p/4711&variant=EU%20model
```

`build_id` is the CI's own id (unique per project); `build_number`, `commit`, `branch` and `pipeline_url` are optional, and `variant` makes the run a run for that variant. The run is then associated with the build.

- **Verified in build**: the **Code** tab of a requirement shows the latest build in which every linked test passed, and the builds its tests ran in with the coverage in each.
- **Build history**: the **Code** tab of a test lists the builds it ran in, newest first, with the result in each.
- **Code → Builds** lists all builds with their runs and pass rate.

## Regression selection

Select **Code → Regression selection** to find the tests to run after changes:

- **Changed requirements**: keys of requirements that changed;
- **Commits since build**: the commits after a build's own commit (on the build's branch), and the requirements and tests they name;
- **Variant**: only requirements and tests that apply to it;
- **Include requirements that depend on, refine or derive from them**: one step along these [relations](requirements.md);
- **Include prerequisite tests**: the tests the selected tests depend on.

**Propose tests** lists the tests with the reason each is included. Deprecated tests are left out. **Create run** makes a run of them (for the chosen variant), named after the build when you start from one. CI can do the same with `POST /api/v1/projects/{pid}/regression/run`.
