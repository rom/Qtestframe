# Command line reference

`qtestframe` is the server and the administration tool. Every command except `report`, `version`, `secrets generate-key` and `healthcheck --url` reads the configuration (`--config FILE`, default `$QTF_CONFIG`, then the built-in defaults and `QTF_*` variables). The commands that open the database migrate it first.

```text
qtestframe <command> [flags]
qtestframe help                 # the overview
qtestframe <command> -h         # the flags of a command
```

- Run administration commands **as the service user** (`sudo -u qtestframe …`), so files such as a new key file get the right owner.
- Exit status: `0` on success (also for `-h`), `1` on an error (printed as `error: …`), `2` for an unknown command.
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

Uploads JUnit XML or JSON results of a CI job to a run on a server, through the REST API with an API token. Further arguments are report files (`*.json` as JSON, others as JUnit XML).

| Flag | Value | Meaning |
|---|---|---|
| `--build` | `string` | build of a run created with --create-run |
| `--configuration` | `string` | configuration (name or id) the results were obtained in; added to the run when it does not target it yet (default $QTF_CONFIGURATION) |
| `--create-missing` | — | create draft automated tests for JUnit test cases without a test key (needs test.write) |
| `--create-run` | — | create the run named by --run when the project has none |
| `--environment` | `string` | environment of a run created with --create-run |
| `--fail-on-unknown` | — | exit with an error when results match no test (unknown keys or test cases without a key) |
| `--json` | `value` | results in the JSON import format {"results":[...]} (file or glob; repeatable) |
| `--junit` | `value` | JUnit XML report (file or glob; repeatable) |
| `--project` | `string` | project key, e.g. SHOP (needed with --run) |
| `--run` | `string` | name of the test run to report into |
| `--run-id` | `string` | id of the test run to report into |
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
```
