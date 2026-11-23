# Command line reference

`qtestframe` is the server and the administration tool. Every command except `report`, `gate`, `select`, `version`, `secrets generate-key` and `healthcheck --url` reads the configuration (`--config FILE`, default `$QTF_CONFIG`, then the built-in defaults and `QTF_*` variables). The commands that open the database migrate it first.

```text
qtestframe <command> [flags]
qtestframe help                 # the overview
qtestframe <command> -h         # the flags of a command
```

- Run administration commands **as the service user** (`sudo -u qtestframe …`), so files such as a new key file get the right owner.
- Exit status: `0` on success (also for `-h`), `1` on an error (printed as `error: …`), `2` for an unknown command. `gate`: `0` the gate passed, `1` it failed, `2` it could not be evaluated.
- Flags take one or two dashes (`-config` and `--config`). Passwords are only read from standard input.
- `qtestframe sandbox-exec` is internal: the server starts it to sandbox scripts. Do not call it yourself.

The descriptions below are written for this guide; the flag tables are generated from the binary's own flag definitions.

<!-- BEGIN GENERATED: cli (make admin-guide-ref) -->

### `qtestframe serve`

Runs the HTTP(S) server. Migrates the database, opens the key provider, creates the bootstrap administrator when there are no users, applies the system settings stored in the database, locks the process down and listens. Stops on `SIGINT`/`SIGTERM` (15 s for requests in flight); `SIGHUP` re-reads the TLS certificate.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--listen` | `string` | listen address host:port (overrides server.listen) |
| `-p` | `int` | shorthand for --port |
| `--port` | `int` | listen port (overrides the port of server.listen) |

### `qtestframe migrate`

Applies the pending database migrations and prints how many. `serve` does the same at every start.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |

### `qtestframe user create`

Creates a user. With `--password-stdin` the password is read as one line from standard input (at least 10 characters with a letter and a non-letter); without it the user can only sign in with single sign-on.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--email` | `string` | e-mail address |
| `--name` | `string` | display name |
| `--password-stdin` | — | read the password from stdin |
| `--super-admin` | — | grant global super administrator rights |

### `qtestframe tenant create`

Creates an organisation (tenant).

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--name` | `string` | tenant name |
| `--slug` | `string` | tenant slug |

### `qtestframe member add`

Gives a user a role in an organisation, or changes it. The role is a built-in role or a custom role of the organisation.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--email` | `string` | user e-mail |
| `--role` | `string` | role, or several roles separated by commas (e.g. sme,tester) (default "viewer") |
| `--tenant` | `string` | tenant slug |

### `qtestframe seed-demo`

Adds the demo projects to an organisation (default `bootstrap.tenant_slug`, created when missing), with demo users. Not for production.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--tenant` | `string` | tenant slug (default: bootstrap tenant) |

### `qtestframe backup`

Writes a logical backup of all data (every organisation) to a ZIP file, through a temporary file that is renamed when complete. Audited as `admin.backup`.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--out` | `string` | backup file to write (ZIP) |

### `qtestframe restore`

Replaces **all** data with a backup, in one transaction, and signs everyone out. Needs `--yes`. Audited as `admin.restore`.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--in` | `string` | backup file to restore (ZIP) |
| `--yes` | — | confirm that ALL existing data is replaced |

### `qtestframe secrets`

The `secrets` commands share one set of flags; each uses the ones that concern it.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--from-config` | `string` | rewrap: configuration file of the key provider that wrapped the keys |
| `--prune` | — | rotate: delete the old data-encryption keys no value uses any more |
| `--yes` | — | decrypt: confirm that the secrets are stored as plaintext |

#### `qtestframe secrets status`

Shows the key provider, its key fingerprint, the data-encryption keys and how many stored secrets are encrypted. Changes nothing (creates no key file).

#### `qtestframe secrets migrate`

Encrypts the secrets still stored as plaintext (also done at every start).

#### `qtestframe secrets rotate`

Creates a new data-encryption key and re-encrypts every secret with it; `--prune` then deletes the keys no value uses.

#### `qtestframe secrets rewrap`

Wraps the data-encryption keys with the configured key provider; `--from-config OLD.yaml` names the configuration of the provider that wrapped them.

#### `qtestframe secrets decrypt`

Stores every secret as plaintext again (before a downgrade). Needs `--yes`.

#### `qtestframe secrets generate-key`

Prints a new random 256-bit key (base64) for `QTF_SECRETS_KEY` or a key file.

### `qtestframe audit verify`

Verifies the hash chain of the audit trail: every entry's hash, the links between entries, the sequence, the recorded end of the chain and the signed checkpoints (with `--tenant` one organisation, otherwise every organisation and the system entries). Exit `0` when intact, `1` at the first broken link (printed with its entry). `--file EXPORT.json` checks an export (Administration → Audit trail → Export with hashes) without the database; the signatures of its checkpoints need the server's key and are only checked against the database.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--file` | `string` | verify an exported audit trail (JSON) instead of the database |
| `--json` | — | print the result as JSON |
| `--tenant` | `string` | organisation (slug); default every organisation |

### `qtestframe check-config`

Loads and validates the configuration (secret files included) without touching the database, and prints `configuration OK`.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--listen` | `string` | listen address host:port (overrides server.listen) |
| `-p` | `int` | shorthand for --port |
| `--port` | `int` | listen port (overrides the port of server.listen) |

### `qtestframe healthcheck`

Requests `/healthz` of the local server (URL from `server.listen`, `https` with TLS, certificate not verified) and exits with 0 on HTTP 200. For container health checks without curl.

| Flag | Value | Meaning |
|---|---|---|
| `--config` | `string` | configuration file (default $QTF_CONFIG) |
| `--listen` | `string` | listen address host:port (overrides server.listen) |
| `-p` | `int` | shorthand for --port |
| `--port` | `int` | listen port (overrides the port of server.listen) |
| `--url` | `string` | URL to probe (default derived from server.listen) |

### `qtestframe report`

Uploads JUnit XML or JSON results of a CI job to a run on a server, through the REST API with an API token. Further arguments are report files (`*.json` as JSON, others as JUnit XML). In GitLab CI, GitHub Actions and Jenkins the pipeline is recorded as the build of the results (`--ci none` turns that off); `--post-status` and `--post-comment` post the commit status `qtestframe/tests` and a merge request note.

| Flag | Value | Meaning |
|---|---|---|
| `--branch` | `string` | branch of the build (default from the CI variables) |
| `--build` | `string` | build of a run created with --create-run (default the CI pipeline number) |
| `--build-id` | `string` | CI build id, unique in the project (default from the CI variables) |
| `--build-number` | `string` | CI build number (default from the CI variables) |
| `--ci` | `string` | record the CI build (pipeline id and number, commit, branch, merge request) of the results: auto (from the CI variables) or none (default "auto") |
| `--code-host` | `string` | where to post: auto (from the CI variables), gitlab or github (default "auto") |
| `--commit` | `string` | commit SHA of the build (default from the CI variables) |
| `--configuration` | `string` | configuration (name or id) the results were obtained in; added to the run when it does not target it yet (default $QTF_CONFIGURATION) |
| `--create-missing` | — | create draft automated tests for JUnit test cases without a test key (needs test.write) |
| `--create-run` | — | create the run named by --run when the project has none |
| `--environment` | `string` | environment of a run created with --create-run |
| `--fail-on-unknown` | — | exit with an error when results match no test (unknown keys or test cases without a key) |
| `--gitlab-comment` | — | same as --post-comment |
| `--gitlab-status` | — | same as --post-status |
| `--json` | `value` | results in the JSON import format {"results":[...]} (file or glob; repeatable) |
| `--junit` | `value` | JUnit XML report (file or glob; repeatable) |
| `--merge-request` | `string` | merge / pull request of the build (default from the CI variables) |
| `--pipeline-url` | `string` | pipeline URL of the build (default from the CI variables) |
| `--post-comment` | — | add or update a merge / pull request note with the summary (default $QTF_POST_COMMENT) |
| `--post-status` | — | post a commit status to the code host (default $QTF_POST_STATUS) |
| `--project` | `string` | project key, e.g. SHOP (default $QTF_PROJECT) |
| `--run` | `string` | name of the test run to report into |
| `--run-id` | `string` | id of the test run to report into |
| `--token` | `string` | API token (default $QTF_TOKEN) |
| `--url` | `string` | server base URL, e.g. https://qtestframe.example.com (default $QTF_URL) |

### `qtestframe gate`

Checks the quality gate of a CI pipeline on a server: the exit criteria of a release (`--release`, optionally `--variant`) or the conditions of `--require` (`must`, `no_failed`, `no_stale`, `no_blocking_defects`, `coverage>=80`, `no_open_defects:major`, `cycle_executed:NAME>=90`, …) on the results of a run (`--run`) or build (`--build`). Prints a summary with the failing items and links; `--format junit|markdown|json --output FILE` also writes a report for the pipeline. Exit code 0 when the gate passes, 1 when it fails, 2 when it cannot be evaluated. `--post-status` and `--post-comment` post a commit status and a merge request note (GitLab or GitHub, see the user guide page CI/CD).

| Flag | Value | Meaning |
|---|---|---|
| `--build` | `string` | build (CI id or number) whose runs' results are checked |
| `--code-host` | `string` | where to post: auto (from the CI variables), gitlab or github (default "auto") |
| `--commit` | `string` | commit of the change (default from the CI variables) |
| `--format` | `string` | output: text, json, junit or markdown (default "text") |
| `--gitlab-comment` | — | same as --post-comment |
| `--gitlab-status` | — | same as --post-status |
| `--merge-request` | `string` | merge / pull request of the change (default from the CI variables) |
| `--output` | `string` | write the output to this file (the text summary still goes to the log) |
| `--post-comment` | — | add or update a merge / pull request note with the summary (default $QTF_POST_COMMENT) |
| `--post-status` | — | post a commit status to the code host (default $QTF_POST_STATUS) |
| `--project` | `string` | project key, e.g. SHOP (default $QTF_PROJECT) |
| `--release` | `string` | release (name, version or id) whose exit criteria are checked |
| `--require` | `value` | condition, e.g. coverage>=80, must, no_failed, no_stale, no_blocking_defects, no_open_defects:major, cycle_executed:NAME>=90 (repeatable, or comma-separated); replaces the release's exit criteria |
| `--run` | `string` | run (name or id) whose results are checked |
| `--run-id` | `string` | id of the run whose results are checked |
| `--scope` | `string` | requirements of a gate without --release: all (default), affected (those the merge request / commit refers to) or a query |
| `--token` | `string` | API token (default $QTF_TOKEN) |
| `--url` | `string` | server base URL, e.g. https://qtestframe.example.com (default $QTF_URL) |
| `--variant` | `string` | evaluate for this variant (name or id) only |

### `qtestframe select`

Prints the tests a CI pipeline runs for a change: the requirements and tests that the change's commits (from git, `--since`, or `--commits`), merge request, changed files (with the path map `qtestframe-map.yml`) and commit messages refer to, the regression selection and the tests with the `--tag` tags. `--format` gives test keys, a test runner filter (`pytest`, `go`, `playwright`, `maven`), JSON or a GitLab child pipeline (`gitlab-child-pipeline`); `--create-run` creates a run of the selection.

| Flag | Value | Meaning |
|---|---|---|
| `--all` | — | select every test |
| `--cli-image` | `string` | image with the qtestframe binary for the child pipeline's report job (default $QTF_CLI_IMAGE) |
| `--commits` | `value` | commit SHAs of the change (repeatable or comma-separated; instead of git) |
| `--create-run` | — | create a run of the selected tests (results of report --run-id land in it) |
| `--environment` | `string` | environment of the created run |
| `--fallback` | `string` | when the change is unknown (no commits, files or keys): none (select nothing) or all (default "none") |
| `--files` | `value` | changed files (repeatable or comma-separated; instead of git) |
| `--format` | `string` | output: keys, pytest, go, playwright, maven, json, gitlab-child-pipeline (default "keys") |
| `--gate-release` | `string` | release whose exit criteria the child pipeline's gate checks |
| `--gate-require` | `value` | condition of the child pipeline's gate (repeatable) |
| `--git` | — | read the commits (messages) and changed files with git (default true) |
| `--head` | `string` | the revision of the change (default $CI_COMMIT_SHA, else HEAD) |
| `--image` | `value` | image of a runner's job in the child pipeline, RUNNER=IMAGE (repeatable) |
| `--include-manual` | — | select manual tests too |
| `--include-related` | — | add requirements that depend on, refine or derive from a changed one |
| `--job` | `value` | test command of a runner's job in the child pipeline, RUNNER=COMMAND (repeatable) |
| `--map` | `string` | path map: changed files → tests, tags, requirements (default qtestframe-map.yml when it exists) |
| `--merge-request` | `string` | merge / pull request number (default from the CI variables) |
| `--no-gate` | — | no gate in the child pipeline |
| `--output` | `string` | write the output to this file |
| `--project` | `string` | project key, e.g. SHOP (default $QTF_PROJECT) |
| `--release` | `string` | release (name, version or id) of the created run |
| `--requirement` | `value` | changed requirement (key or id; repeatable) |
| `--run` | `string` | name of the run to create (default $QTF_RUN) |
| `--since` | `string` | compare with this git revision (default: the merge request's diff base, else the commit before the push) |
| `--since-build` | `string` | also the commits after this build (CI id or number) |
| `--tag` | `value` | always run the tests with this tag (repeatable) |
| `--token` | `string` | API token (default $QTF_TOKEN) |
| `--url` | `string` | server base URL, e.g. https://qtestframe.example.com (default $QTF_URL) |
| `--variant` | `string` | only tests that apply to this variant |

### `qtestframe stubs`

Writes generated test code stubs into a directory (`--out`): one file per test of `--query` (default `execution_type:automated`) or `--ids`, for `--framework` `pytest`, `playwright`, `go`, `junit5`, `jest`, `vitest`, `robot` or `cucumber`, named after the test key so that JUnit results map back, and marked as skipped until implemented. Generating again creates new files, regenerates unchanged stubs, only updates the header block of implemented ones and leaves files without qtestframe markers alone; `--dry-run` lists what would change. `--mark-script` sets the script of tests without one to their stub path (see the user guide page CI/CD).

| Flag | Value | Meaning |
|---|---|---|
| `--dry-run` | — | only print what would be written |
| `--framework` | `string` | test framework: pytest, playwright, go, junit5, jest, vitest, robot, cucumber |
| `--ids` | `value` | only these tests (keys or ids; repeatable or comma-separated) |
| `--mark-script` | — | set the script of tests without one to their stub path (needs test.write) |
| `--out` | `string` | directory to write the stubs to (default ".") |
| `--package` | `string` | Go package (default: the name of --out, else tests) or Java package of the stubs |
| `--project` | `string` | project key, e.g. SHOP (default $QTF_PROJECT) |
| `--query` | `string` | the tests to generate stubs for (query language); "" for all (default "execution_type:automated") |
| `--script-dir` | `string` | directory of the stubs in the repository for --mark-script (default --out when it is relative) |
| `--token` | `string` | API token (default $QTF_TOKEN) |
| `--url` | `string` | server base URL, e.g. https://qtestframe.example.com (default $QTF_URL) |

### `qtestframe version`

Prints the version (also `--version`, `-v`).

<!-- END GENERATED: cli -->

## Examples

```sh
# first super administrator and an organisation
read -rs PW && printf '%s\n' "$PW" | qtestframe user create --email admin@example.com --password-stdin --super-admin
qtestframe tenant create --slug acme --name "Acme Corp"
qtestframe member add --tenant acme --email admin@example.com --role admin

# backup and restore
qtestframe backup --out /var/backups/qtestframe/qtf.zip
qtestframe restore --in /var/backups/qtestframe/qtf.zip --yes

# keys of the stored secrets
qtestframe secrets status
qtestframe secrets rewrap --config config.new.yaml --from-config config.yaml

# CI: report JUnit results into a run, creating it when missing
QTF_URL=https://qtf.example.com QTF_TOKEN=qtf_… \
  qtestframe report --project SHOP --run "Nightly $BUILD" --create-run --build "$BUILD" --junit 'reports/*.xml'

# CI: the quality gate of the pipeline's run, as JUnit for the pipeline
qtestframe gate --project SHOP --run "Nightly $BUILD" --release 1.0 --format junit --output qtf-gate.xml

# CI: the tests of a merge request as a pytest filter, or as a child pipeline
pytest -k "$(qtestframe select --project SHOP --format pytest)"
qtestframe select --project SHOP --format gitlab-child-pipeline --create-run --run "MR $CI_MERGE_REQUEST_IID" --output child.yml
```

Ready-made pipeline definitions are in `examples/ci/` and in the app (**Settings → Project → CI setup**); see the user guide page *CI/CD*.
