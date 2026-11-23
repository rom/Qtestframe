# CI/CD

Qtestframe works with your **CI/CD pipeline** (GitLab CI/CD in the first place, GitHub Actions and other CI systems too): the pipeline uploads its automated test results, a **quality gate** decides whether the change may go on, the result comes back to GitLab as a **commit status** and a **merge request note**, and **test selection** runs only the tests a change needs. Everything is done with the `qtestframe` command in the pipeline's jobs and an API token; the server needs no extra configuration.

This page builds on [test runs and the CI import](runs.md), [releases and their exit criteria](releases-and-risks.md) and [code links, builds and the regression selection](variants-and-code.md#code-links).

## Set up a pipeline

Open **Settings → Project** and scroll to **CI setup**. It shows, for this project and server:

1. **An API token.** Create one with the role **tester** under **Administration → API tokens**, preferably for a separate CI user that is a member of the project with the tester role. A token never gets more than its user's own role. See [API tokens](administration.md#api-tokens).
2. **The CI/CD variables** to add in GitLab under *Settings → CI/CD → Variables*:

   | Variable | Value |
   |---|---|
   | `QTF_URL` | the server URL (already in the templates) |
   | `QTF_PROJECT` | the project key (already in the templates) |
   | `QTF_TOKEN` | the API token; mark it **masked** (and **protected** if only protected branches report) |
   | `QTF_CLI_IMAGE` | a container image that contains the `qtestframe` binary |
   | `QTF_GITLAB_TOKEN` | optional: a GitLab project access token with the role *Developer* and the scope `api`, for commit statuses and notes from the pipeline (masked) |
   | `QTF_RELEASE` | optional: the release whose exit criteria are the gate |

3. **A template.** Choose one, then **Copy** or **Download** it:
   - **GitLab**: your own test jobs write JUnit XML to `qtf-results/`; a final job uploads it and checks the gate.
   - **GitLab with test selection**: a first job selects the tests of the change and starts a generated child pipeline that runs them (see [Test selection](#test-selection)).
   - **GitLab component**: the same as a [CI/CD component](https://docs.gitlab.com/ee/ci/components/) with inputs, for a component project.
   - **GitHub Actions**: a workflow that does the same.

   Instead of copying, a `.gitlab-ci.yml` can include the template from the server: `include: - remote: 'https://<server>/api/v1/ci/templates/gitlab-ci.yml'`. The same files are in the source code under `examples/ci/`.

Anyone who can read the project sees the CI setup; the tokens are never shown there.

## Name automated tests after their key

Qtestframe matches an automated result to a test by its **test key**, for example `SHOP-TC-12`. Put the key into the test (or [generate the test code](#generate-test-code) with the key already in it):

- in the **test name** or **class name** of the JUnit report, for example `test_SHOP_TC_12_checkout` or `SHOP-TC-12 checkout`. Underscores stand for hyphens, because most languages do not allow hyphens in names; the key must be written in full and in capitals (`ShopTc12` is not recognised);
- or as a **property** of the test case: `qtestframe.key`, `test_key` or `qtestframe`.

The same identifier is what test selection filters on (`SHOP_TC_12`). Results without a key can create draft automated tests with `--create-missing` (see [runs](runs.md)). To group tests into the jobs of a generated child pipeline, tag them `runner:pytest`, `runner:go`, `runner:playwright` or `runner:maven` (or any name of your own); without such a tag, tests with the script language *playwright* go to `playwright` and *python* to `pytest`, the others to `default`.

## Generate test code

Instead of naming the tests by hand, let Qtestframe write a **stub** for each test: a source file for your test framework that is already named after the key, documents the test and reports as *skipped* until somebody implements it. Its results then map back without further work.

**In the web client.** In **Tests**, select tests (or filter the list) and choose **Generate test code**. Pick the framework, the directory of the tests in the repository (and the package for Go and Java), look at the preview of each file and **Download zip** (or the one file). With the right to edit tests you can also tick **Set the script of the tests without one to their stub path** (see below). Under *Command line* the dialog shows the `qtestframe stubs` command that writes the same files.

**On the command line**, in the repository:

```
qtestframe stubs --framework pytest --query 'execution_type:automated has:!script' --out tests/
```

`--query` selects the tests ([query language](search-and-filters.md); the default is `execution_type:automated`, `""` is all tests), `--ids` names tests by key, `--package` sets the Go or Java package (Go: the name of the directory by default), `--dry-run` only lists what would change. `QTF_URL`, `QTF_TOKEN` and `QTF_PROJECT` are read as for `report`; a token with the role *viewer* is enough to generate.

**Frameworks:**

| `--framework` | File | Key in | Not implemented | Test data |
|---|---|---|---|---|
| `pytest` | `test_SHOP_TC_12.py` | `def test_SHOP_TC_12_checkout…()` | `@pytest.mark.skip` | `@pytest.mark.parametrize` |
| `playwright` | `SHOP-TC-12.spec.ts` | the title, `tag: '@SHOP-TC-12'` and the annotation `qtestframe` | `test.fixme()` | one test per row |
| `go` | `shop_tc_12_test.go` | `func TestSHOP_TC_12_Checkout…` | `t.Skip` | a table with subtests |
| `junit5` | `SHOP_TC_12Test.java` | the class and method names, `@Tag("SHOP-TC-12")` | `@Disabled` | `@ParameterizedTest` with `@CsvSource` |
| `jest`, `vitest` | `SHOP-TC-12.test.ts` | the test name | `test.skip` | `test.skip.each` |
| `robot` | `SHOP-TC-12.robot` | the test name and the tag `SHOP-TC-12` | `Skip` | one test case per row calling a keyword |
| `cucumber` | `SHOP-TC-12.feature` | the feature and scenario names, the tag `@SHOP-TC-12` | the steps have no definitions yet; tag `@qtestframe-stub` | a Scenario Outline with Examples |

Every stub contains the title, a link to the test, the requirements it verifies, the priority and tags, the description, the preconditions and the expected result, and the **steps as numbered TODO comments** with their data and expected results (Cucumber: as Given, When and Then steps). Parameters of the [test data](tests.md) become the parameters of the parametrised test; `{{name}}` placeholders stay in the comments (Cucumber: `<name>`). For Playwright, have the JUnit reporter write the annotation (`embedAnnotationsAsProperties: true`); the name alone is enough too. JUnit 5 needs `junit-jupiter-params` for data-driven tests. Cucumber has no skip marker: a stub's steps are undefined until you write their definitions; in strict mode Cucumber reports them as failed, so exclude the stubs with `--tags 'not @qtestframe-stub'` until they are implemented (they then have no result, and never a passed one).

**Generating again.** Each file starts with a block between the comment lines `qtestframe:begin SHOP-TC-12 pytest body=…` and `qtestframe:end`. Keep these two lines. When you run `qtestframe stubs` again:

- a file that does not exist is **created**;
- a file whose code below the block is still the generated stub is **regenerated** completely (new steps, test data or title are taken over);
- a file whose code you have changed (implemented) keeps it: only the **header block** is rewritten (title, requirements, preconditions…). The steps in the code are not touched again;
- a file without the marker lines (or with those of another test) is **left alone**.

Line endings and trailing spaces do not count as changes. The download from the web client always contains the full stubs: merge it into a repository with the command line instead.

**Mark the scripts.** With `--mark-script` (or the check box in the dialog) each selected test that has no script gets the script `qtestframe:stub tests/test_SHOP_TC_12.py`, becomes automated, gets the script language of the framework (python, playwright or javascript, where there is one) and the tag `runner:<name>` for the [child pipeline](#test-selection) (pytest, playwright, go, maven, jest, vitest, robot, cucumber) unless it has a runner tag. The server never runs such a script (*Run automated tests* skips it): the results come from CI. Because the script is no longer empty, `has:!script` leaves these tests out the next time. `--script-dir` sets the directory written into the script when `--out` is not the path in the repository. Marking needs the right to edit tests and is recorded in the history of each test.

## Upload results

```
qtestframe report --run "Pipeline $CI_PIPELINE_IID" --create-run --junit 'qtf-results/*.xml'
```

In GitLab CI, GitHub Actions and Jenkins the command reads the CI variables and records the pipeline as the **build** of the results: its id and number, the commit, the branch, the pipeline URL and the merge request. The requirement then shows *verified in build* (see [Builds](variants-and-code.md#builds)). Override any of them with `--build-id`, `--build-number`, `--commit`, `--branch`, `--pipeline-url` and `--merge-request`, or turn it off with `--ci none`. `QTF_URL`, `QTF_TOKEN` and `QTF_PROJECT` are the defaults of `--url`, `--token` and `--project`.

## Quality gate

`qtestframe gate` checks whether the results are good enough, prints a summary with what fails (with links) in the job log and ends with exit code **0** when the gate passes, **1** when it fails and **2** when it cannot be evaluated (a wrong token, an unknown release). A failing job stops the pipeline.

```
qtestframe gate --run "Pipeline $CI_PIPELINE_IID" --release 1.0 --format junit --output qtf-gate.xml
```

**What is checked:**

- With `--release`: the release's [exit criteria](releases-and-risks.md) on its scope, as on the release's *Status* tab. A release that targets variants is checked for each variant (`--variant` checks one).
- Without a release: the conditions of `--require` on the project's requirements (all except rejected and obsolete ones). Without `--require` they are: every MUST/SHALL requirement passed, no failed tests, no stale results and no requirement blocked by a defect.
- `--require` also replaces the release's criteria. It is repeatable and takes comma-separated lists.
- `--run` (name or id) or `--build` (the CI id or number) limits the results to that run or to the runs of that build; with a release, to its runs in the build.
- `--scope affected` checks only the requirements the merge request or commit refers to (from [code links](variants-and-code.md#code-links)); a [query](search-and-filters.md) such as `--scope "tag:checkout"` checks the requirements it finds.

**Conditions** (`TYPE[:ARGUMENT][>=PERCENT]`):

| Condition | Met when |
|---|---|
| `must` (`mandatory_passed`) | every MUST and SHALL requirement has all its tests passed |
| `no_failed` | no test failed or is blocked |
| `no_stale` | no test has only a stale pass (see [staleness](administration.md#change-impact-staleness-and-defects)) |
| `no_blocking_defects` (`blocking`) | no requirement is blocked by an open defect |
| `coverage>=80` | at least 80 % of the requirements have tests |
| `passed>=95` | at least 95 % of the requirements have all their tests passed |
| `executed>=100` | every execution of the runs has a result |
| `approved>=90` | at least 90 % of the requirements are approved, implemented or verified |
| `no_suspect` | no suspect trace links |
| `no_open_defects:major` | no open defect of severity major or higher is linked |
| `risks_mitigated:high`, `residual_risk:medium` | the risk conditions of releases |
| `trace_rules` (`trace_rules:warning`) | the [traceability rules](traceability-and-coverage.md) pass |
| `cycle_executed:"Sprint 3">=90` | 90 % of the planned tests of the test cycle have a result |

**Waivers** count as on the release status: an approved waiver for the release, or for the whole project, accepts its requirements, and the summary lists them as *waived*.

**Output.** `--format text` (the default) prints the summary; `json`, `junit` (one test case per condition, so GitLab's test report shows the gate) and `markdown` (also for GitHub's job summary) go to the file of `--output`, and the summary is still printed. The same check is available as `POST /api/v1/projects/{pid}/gate` (see the API description).

## Results back in GitLab

The result of a pipeline can show up where developers look:

- a **commit status** on the commit: `qtestframe/tests` (failed when a test failed or is blocked) and `qtestframe/gate` (the verdict of the gate), with a link to the run or release and a short summary;
- a **note on the merge request** with the results, the coverage of the requirements of the change, the failed tests and the gate's verdict. Qtestframe keeps **one** note per project and merge request and updates it with every upload (it finds its own note by a hidden marker), so the merge request is not flooded.

There are two ways, and you need only one.

### From the pipeline

Add `--post-status` and `--post-comment` to `qtestframe report` and `qtestframe gate`, or set the CI/CD variables `QTF_POST_STATUS` and `QTF_POST_COMMENT` to `true` (the templates do). The command reads `CI_API_V4_URL`, `CI_PROJECT_ID`, `CI_COMMIT_SHA` and `CI_MERGE_REQUEST_IID`, and authenticates with `QTF_GITLAB_TOKEN`: a project access token with the role **Developer** (commit statuses need Developer) and the scope `api`, stored as a masked variable.

The job's own token (`CI_JOB_TOKEN`) is tried when there is no `QTF_GITLAB_TOKEN`, but GitLab does not accept it for commit statuses and merge request notes today; the job log then says so. A failed post is a warning and does not fail the job.

### From the server

With the **Manager** role, open **Settings → Project → Results back to GitLab**:

1. Tick **Post commit statuses and merge request notes to GitLab**.
2. Enter the **GitLab API URL** (empty for gitlab.com), the **GitLab project** (its numeric id or `group/project`) and an **access token** (a project access token with the role Developer and the scope `api`). The token is stored encrypted and never shown again.
3. Choose what to post and, for `qtestframe/gate`, a **gate release** or **gate conditions** (for example `must, no_failed`). Without either, only `qtestframe/tests` is posted.

From then on, when results arrive with a commit (as `qtestframe report` sends them in a pipeline), the server posts the status and the note in the background. When GitLab is not reachable it tries again after 1, 2, 4, 8 and 16 minutes; a token GitLab refuses is not retried. **Recent posts** in the same card shows what was posted, or why not. The merge request is the one the pipeline names, or else the open merge request of the branch (from the [webhooks](variants-and-code.md#webhooks)).

## Test selection

`qtestframe select` answers "which tests does this change need?":

```
pytest -k "$(qtestframe select --format pytest)"
```

It collects the **change**: in a merge request pipeline the commits between `CI_MERGE_REQUEST_DIFF_BASE_SHA` and `CI_COMMIT_SHA` (with `git log`), the files they change (`git diff`) and the merge request; elsewhere the commits since `--since REV`, or `--commits` and `--files` given by hand. Then it selects:

- the requirements and tests the commits and the merge request are linked to ([code links](variants-and-code.md#code-links)), and the keys named in the commit messages (`SHOP-REQ-4`, `REQ-4`, `TC-7`);
- the tests of a **path map**: a file `qtestframe-map.yml` in the repository that maps file patterns to tests, tags and requirements (an example is in `examples/ci/qtestframe-map.yml`):

  ```yaml
  - paths: ["src/cart/**", "api/cart.go"]
    tests: [SHOP-TC-3]
    tags: [cart]
  - paths: ["src/payment/"]
    requirements: [SHOP-REQ-4]
  ```

- the [regression selection](variants-and-code.md#regression-selection) of them: the tests verifying the requirements, `--include-related` requirements and the prerequisite tests;
- the tests with a tag given with `--tag` (for example `--tag always-run` for smoke tests);
- with `--variant`, only what applies to the variant.

Only **automated** tests are selected (manual ones are counted in the log; `--include-manual` adds them). When the change is unknown (no commits, files, keys or merge request), nothing but the tagged tests is selected, unless `--fallback all`. Unknown keys are listed.

**Output** (`--format`):

| Format | Output | Use |
|---|---|---|
| `keys` | one key per line | your own scripts |
| `pytest` | `SHOP_TC_3 or SHOP_TC_7` | `pytest -k "…"` |
| `go` | `(SHOP_TC_3\|SHOP_TC_7)([^0-9]\|$)` | `go test -run '…'` |
| `playwright` | `(SHOP[-_]TC[-_]3\|…)(?![0-9])` | `npx playwright test --grep '…'` |
| `maven` | `*SHOP_TC_3*,*SHOP_TC_7*` | `mvn test -Dtest='…'` (also matches `SHOP_TC_31`; select more rather than less) |
| `json` | the selection with the reasons | |
| `gitlab-child-pipeline` | a child pipeline | see below |

An empty selection prints nothing: do not run the tests then, because an empty filter selects every test in most runners.

**A generated child pipeline.** `--format gitlab-child-pipeline --output qtestframe-pipeline.yml` writes a pipeline with one job per runner group, each with the selected tests in `$QTF_TESTS` and its filter in `$QTF_FILTER`, and a final job that runs `qtestframe report` and `qtestframe gate` also when tests failed. A parent job keeps the file as an artifact and starts it with `trigger: include: - artifact: qtestframe-pipeline.yml`; the template *GitLab with test selection* does that. Set each runner's image and command with `--image pytest=python:3.13` and `--job pytest='pytest -k "$QTF_FILTER" --junitxml="$QTF_RESULTS/pytest.xml"'`; the defaults use pytest, gotestsum, Playwright and Maven. `--create-run --run NAME` creates the run of the selection first, so the results land in it; `--gate-release` and `--gate-require` set the final gate.

The selection is also `POST /api/v1/projects/{pid}/ci/select`.

## GitHub Actions and other CI systems

- **GitHub Actions**: the template uses the workflow's `GITHUB_TOKEN` (with `statuses: write` and `pull-requests: write`) as `QTF_GITHUB_TOKEN`; `--post-status` and `--post-comment` then post a commit status and a pull request comment that is kept up to date. `--format markdown --output "$GITHUB_STEP_SUMMARY"` shows the gate in the job summary.
- **Jenkins** and others: `qtestframe report` reads Jenkins' variables (`BUILD_TAG`, `BUILD_NUMBER`, `GIT_COMMIT`, `BRANCH_NAME`, `CHANGE_ID`); elsewhere set `QTF_BUILD_ID`, `QTF_BUILD_NUMBER`, `QTF_COMMIT`, `QTF_BRANCH`, `QTF_MERGE_REQUEST` or pass the flags. `gate` and `select` work everywhere; posting statuses is for GitLab and GitHub.

## Limitations

- GitLab's job token (`CI_JOB_TOKEN`) cannot post commit statuses or merge request notes: use a project access token, or let the server post.
- The server posts only to GitLab; GitHub statuses come from the pipeline.
- Test selection is only as good as the links: commits and merge requests are known through the [webhooks](variants-and-code.md#webhooks) or the keys in commit messages, and files only through the path map.
- Generated stubs are templates built into the server; they cannot be customised per organisation yet. Project configurations are not turned into parameters: run the stubs once per configuration in CI (`qtestframe report --configuration`).
- API tokens are limited by roles, not by finer scopes: give the CI user the tester role, or a custom role with only `project.read`, `test.read`, `run.read`, `run.execute` and `run.write`.
