# Configuration

## The configuration file

The configuration is one **YAML** file, given with `--config FILE` or the environment variable `QTF_CONFIG`. Every command reads the same file. Without a file the built-in defaults apply (HTTP on `:8080`, SQLite file `qtestframe.db` in the working directory).

Three files to start from:

| File | Use |
|---|---|
| `examples/qtestframe.yaml` | local trial: HTTP on `:8080`, SQLite in `./data`, script runner on, bootstrap administrator |
| `examples/qtestframe-tls.yaml` | the same with HTTPS on `:8443` and the certificate of `make certs-dev` |
| `deploy/config.example.yaml` | the commented production template that `install.sh` copies to `/etc/qtestframe/config.yaml` |

The file is decoded **strictly**: an unknown or misspelt key (`listn:`) and a second YAML document stop the program with an error that names the line. Check a file without starting the server, secret files included:

```sh
qtestframe check-config --config /etc/qtestframe/config.yaml
```

Durations are written as Go durations: `30s`, `15m`, `12h`, `1h30m`.

## Where a value comes from

Later sources win:

1. the built-in defaults (the *Default* column of the [reference](#reference));
2. the YAML file;
3. `QTF_*` environment variables ([list](#environment-variables));
4. command line flags of `serve`, `check-config` and `healthcheck`: `--listen HOST:PORT` replaces `server.listen`, `--port N` / `-p N` only its port;
5. for `serve`: the [runtime system settings](system-settings.md) that super administrators changed in the web client, stored in the database – except settings that an environment variable sets or that `system_settings.locked` lists.

Secrets have their own rules, applied after the environment:

| Secret | Order (the last one that is set wins) |
|---|---|
| database DSN | `database.dsn`, `QTF_DATABASE_DSN`, **`database.dsn_file`** |
| bootstrap password | `bootstrap.admin_password`, `QTF_BOOTSTRAP_ADMIN_PASSWORD`, **`bootstrap.admin_password_file`** |
| SMTP password | `email.password_file`, a password stored in the web client, **`QTF_EMAIL_PASSWORD`** |
| OIDC client secret | `client_secret`, `client_secret_file`, a secret stored in the web client, **`QTF_OIDC_<NAME>_CLIENT_SECRET`** |
| HSM PIN | `secrets.pkcs11.pin_file`, **`QTF_SECRETS_PKCS11_PIN`** |
| local key | `secrets.local.key_file`, **`QTF_SECRETS_KEY`** (then no file is used) |

So a `*_file` option set in the file wins over the environment for the DSN and the bootstrap password, while for the other secrets the environment wins.

## Secrets in files

Do not write passwords into `config.yaml`. Use the `*_file` options: `database.dsn_file`, `auth.oidc[].client_secret_file`, `bootstrap.admin_password_file`, `email.password_file` and `secrets.pkcs11.pin_file`. Their content is read once at start-up and trimmed.

A **relative** path in one of these options is looked up in `$CREDENTIALS_DIRECTORY`, which systemd sets for `LoadCredential=`. The credentials live in a private, non-swappable directory that only the service can read:

```ini
# sudo systemctl edit qtestframe
[Service]
LoadCredential=db-dsn:/etc/qtestframe/secrets/db-dsn
LoadCredential=oidc-corp:/etc/qtestframe/secrets/oidc-corp
```

```yaml
database:
  driver: postgres
  dsn_file: db-dsn             # $CREDENTIALS_DIRECTORY/db-dsn
auth:
  oidc:
    - name: corp
      client_secret_file: oidc-corp
```

Paths of keys and certificates (`server.tls_key`, `auth.saml[].sp_key_file`, `logging.syslog.key_file`, …) are used as given. Environment variables work too (and are what containers use), but are visible in `/proc/<pid>/environ` to the same user; prefer files.

## After a change

| Changed | Takes effect |
|---|---|
| the YAML file or an environment variable | after `systemctl restart qtestframe` |
| the TLS certificate or key file | after `systemctl reload qtestframe` (`SIGHUP`); if the new pair cannot be read, the old one stays in use and an error is logged |
| a setting in **Administration → System settings** | at once, or after a restart for the runner (see [Runtime system settings](system-settings.md)) |

## Reference

Every key of the file, its type, its default and the environment variable that sets it. *Type* `duration` takes values such as `30s` or `12h`; `list` is a YAML list; `map` a YAML mapping. Keys of the providers in `auth.oidc` and `auth.saml` are written with `[]`: they belong to each entry of the list.

<!-- BEGIN GENERATED: config-reference (make admin-guide-ref) -->

### `server`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `server.listen` | string | `:8080` | `QTF_SERVER_LISTEN`, `QTF_SERVER_PORT` | Address and port of the HTTP(S) listener, `host:port`; `:8080` listens on every address. Replaced by `--listen`, and the port alone by `--port`/`-p` or `QTF_SERVER_PORT`. |
| `server.tls_cert` | string | — | — | PEM file with the server certificate and its chain (leaf first). With `tls_key` it turns on HTTPS (TLS 1.2 or later); re-read on `SIGHUP`. |
| `server.tls_key` | string | — | — | PEM file with the private key of `tls_cert`. Both must be set, or neither. |
| `server.base_url` | string | — | `QTF_SERVER_BASE_URL` | The URL users type in the browser, without a trailing slash (`https://qtf.example.com`). Needed for single sign-on (callback and SAML entity URLs), for e-mail (links; required when e-mail is on) and for the origin check of the CSRF protection. |
| `server.read_timeout` | duration | `30s` | — | Longest time to read a whole request, body included (uploads, restores). |
| `server.write_timeout` | duration | `2m` | — | Longest time to write a response (downloads, exports, backups in the web client). Keep it above `runner.timeout`. |
| `server.trusted_proxies` | list | — | — | Addresses or CIDR ranges of reverse proxies. Only for requests from them is `X-Forwarded-For` used as the client address (its last entry), for logs, the audit trail and the sign-in limits. |
| `server.hsts` | bool | `true` | — | Send `Strict-Transport-Security` (two years, subdomains included) on HTTPS connections the server terminates itself. A TLS-terminating proxy must send it instead. |
| `server.frame_ancestors` | list | — | — | Origins (`https://doors.example.com`) of other tools that may show the OSLC selection and creation dialogs in a frame (`Content-Security-Policy: frame-ancestors`). Without any the dialogs open in a window of their own. The browser must also send the session cookie to the frame: both tools on the same site (subdomains of one domain). |

### `database`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `database.driver` | string | `sqlite` | `QTF_DATABASE_DRIVER` | `sqlite` (a file, nothing to install) or `postgres`. |
| `database.dsn` | string | `qtestframe.db` | `QTF_DATABASE_DSN` | SQLite: the database file (its directory is created with mode 0700). PostgreSQL: a connection URL such as `postgres://user:pass@host:5432/db?sslmode=verify-full`. |
| `database.dsn_file` | string | — | — | File that holds the DSN, so that the password is not in the configuration. A relative path is looked up in `$CREDENTIALS_DIRECTORY`. When set, it replaces `dsn` and `QTF_DATABASE_DSN`. |
| `database.max_open_conns` | integer | `20` | — | Size of the PostgreSQL connection pool. SQLite always uses one connection. |

### `logging`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `logging.level` | string | `info` | `QTF_LOG_LEVEL` | `debug`, `info`, `warn` or `error`. |
| `logging.format` | string | `text` | `QTF_LOG_FORMAT` | `text` (key=value) or `json`, written to standard error. |
| `logging.syslog.enabled` | bool | `false` | — | Also send every log record, the audit records included, to a syslog server. |
| `logging.syslog.network` | string | `udp` | — | `udp`, `tcp`, `tls` (RFC 5425), `unix` or `unixgram`. |
| `logging.syslog.address` | string | — | — | `host:port` of the syslog server, or the path of a Unix socket such as `/dev/log`. |
| `logging.syslog.tag` | string | `qtestframe` | — | APP-NAME (RFC 5424) or tag (RFC 3164) of the messages. |
| `logging.syslog.facility` | string | `local0` | — | Syslog facility, e.g. `local0` or `daemon`. |
| `logging.syslog.format` | string | `rfc5424` | — | `rfc5424` or `rfc3164`. The message is the log record as JSON. |
| `logging.syslog.ca_file` | string | — | — | `tls`: CA certificates that verify the syslog server (default: the system roots). |
| `logging.syslog.cert_file` | string | — | — | `tls`: client certificate for mutual TLS. |
| `logging.syslog.key_file` | string | — | — | `tls`: private key of `cert_file`. |

### `auth`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `auth.local` | bool | `true` | — | Allow sign-in with e-mail and password. Can only be turned off in the web client while a single sign-on provider is enabled. |
| `auth.session_ttl` | duration | `12h` | — | Absolute lifetime of a session (no sliding renewal). |
| `auth.cookie_secure` | bool | `true` | `QTF_COOKIE_SECURE` | Mark the session cookies `Secure`. Must be `false` when the server is reached over plain HTTP, else browsers do not keep the session cookie. |
| `auth.password_min_length` | integer | `10` | — | Minimum length of local passwords (8–72) set in the web client or the API. `user create` and the bootstrap administrator always use 10. |
| `auth.login_max_attempts` | integer | `10` | — | Sign-in attempts per client address and e-mail address within `login_lockout` (1–1000); further attempts get HTTP 429. |
| `auth.login_lockout` | duration | `5m` | — | Window of `login_max_attempts` (1m–24h). |
| `auth.session_idle_timeout` | duration | `0s` | — | Ends a browser session after this long without activity, e.g. `30m` (`0` = off: only `session_ttl` applies). The electronic-records mode of an organisation can set a shorter one; the shorter applies. |
| `auth.password_min_classes` | integer | `0` | — | How many of the character classes lower-case letters, upper-case letters, digits and symbols a local password must use (2 to 4; `0` = letters and at least one digit or symbol). |
| `auth.password_history` | integer | `0` | — | How many earlier passwords a user cannot choose again when they change their password (0 to 24; `0` = off). |
| `auth.password_max_age_days` | integer | `0` | — | A local password expires this many days after it was set: the next sign-in must change it (`0` = never). For accounts older than this setting the days count from their next sign-in. |
| `auth.token_max_days` | integer | `0` | — | Longest lifetime of a new API token in days (`0` = no limit). With a limit every new token needs an expiry date within it; existing tokens keep theirs. |
| `auth.token_require_expiry` | bool | `false` | — | Refuse new API tokens without an expiry date. |

### `auth.oidc`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `auth.oidc` | list | — | — | OpenID Connect providers, each shown as a button on the sign-in page. Can be replaced in **Administration → System settings**. |
| `auth.oidc[].name` | string | — | — | Identifier of the provider in its URLs (`/api/v1/auth/oidc/<name>/callback`) and in `QTF_OIDC_<NAME>_CLIENT_SECRET`. Use lower-case letters, digits and `_`. |
| `auth.oidc[].enabled` | bool | `true` | — | `false` keeps the provider configured but hides it from the sign-in page. Default `true`. |
| `auth.oidc[].display_name` | string | — | — | Label of the button on the sign-in page. |
| `auth.oidc[].icon` | string | — | — | Icon of the sign-in button: `sign-in`, `key`, `shield`, `building`, `microsoft`, `google`, `github`, `amazon` or `apple` (empty = a generic icon). |
| `auth.oidc[].issuer` | string | — | — | Issuer URL; the server reads `<issuer>/.well-known/openid-configuration` at the first sign-in. Required. |
| `auth.oidc[].client_id` | string | — | — | Client ID registered at the provider. Required. |
| `auth.oidc[].client_secret` | string | — | `QTF_OIDC_<NAME>_CLIENT_SECRET` | Client secret written in the file. Prefer `client_secret_file` or the environment variable. |
| `auth.oidc[].client_secret_file` | string | — | — | File with the client secret (relative: in `$CREDENTIALS_DIRECTORY`). `QTF_OIDC_<NAME>_CLIENT_SECRET` wins over it. |
| `auth.oidc[].scopes` | list | `[openid, email, profile]` | — | Requested scopes. Default `[openid, email, profile]`; add `groups` where the provider needs it. |
| `auth.oidc[].email_claim` | string | `email` | — | Claim with the e-mail address (default `email`). A token with `email_verified: false` is refused. |
| `auth.oidc[].name_claim` | string | `name` | — | Claim with the display name (default `name`). |
| `auth.oidc[].groups_claim` | string | `groups` | — | Claim with the groups used by `role_mapping` (default `groups`). |
| `auth.oidc[].local_mfa` | string | `off` | — | Qtestframe's own second factor after this provider: `off` (default; the provider is responsible), `unless_idp_mfa` (unless `amr`/`acr` show that the provider used MFA) or `always`. |
| `auth.oidc[].mfa_amr_values` | list | `[mfa]` | — | `amr` values that count as MFA by the provider (default `[mfa]`). |
| `auth.oidc[].mfa_acr_values` | list | — | — | `acr` values that count as MFA by the provider (default none). |
| `auth.oidc[].auto_provision` | bool | `false` | — | Create unknown users at their first sign-in (and their membership in `default_tenant`). |
| `auth.oidc[].default_tenant` | string | — | — | Slug of the organisation whose membership the sign-in creates or updates. |
| `auth.oidc[].default_role` | string | `viewer` | — | Role when no group is mapped (an unknown or empty role means `viewer`). |
| `auth.oidc[].role_mapping` | map | — | — | Group → role. The most privileged mapped role wins and is set again at every sign-in. |
| `auth.oidc[].allowed_domains` | list | — | — | E-mail domains that may sign in through the provider (`example.com`; `*.example.com` for its subdomains). Empty = any. Other addresses are refused before an account is looked up or created. |

### `auth.saml`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `auth.saml` | list | — | — | SAML 2.0 identity providers (only in the file). |
| `auth.saml[].name` | string | — | — | Identifier in the URLs `/api/v1/auth/saml/<name>/{metadata,login,acs}`. |
| `auth.saml[].display_name` | string | — | — | Label of the button on the sign-in page. |
| `auth.saml[].idp_metadata_url` | string | — | — | URL of the IdP metadata, fetched at the first sign-in. This or `idp_metadata_file` is required. |
| `auth.saml[].idp_metadata_file` | string | — | — | File with the IdP metadata (no outgoing request needed). |
| `auth.saml[].entity_id` | string | the SP metadata URL | — | Entity ID of Qtestframe (default: the SP metadata URL `<base_url>/api/v1/auth/saml/<name>/metadata`). |
| `auth.saml[].sp_cert_file` | string | — | — | Certificate of the SP key pair (`scripts/certs/gen-saml-sp-cert.sh`), published in the SP metadata. |
| `auth.saml[].sp_key_file` | string | — | — | Private key of the SP; decrypts encrypted assertions. |
| `auth.saml[].email_attr` | string | `email` | — | Attribute with the e-mail address (default `email`; else a NameID that contains `@`). |
| `auth.saml[].name_attr` | string | `displayName` | — | Attribute with the display name (default `displayName`). |
| `auth.saml[].groups_attr` | string | `groups` | — | Attribute with the groups (default `groups`). |
| `auth.saml[].auto_provision` | bool | `false` | — | As for OIDC. |
| `auth.saml[].default_tenant` | string | — | — | As for OIDC. |
| `auth.saml[].default_role` | string | `viewer` | — | As for OIDC. |
| `auth.saml[].role_mapping` | map | — | — | As for OIDC. |
| `auth.saml[].allowed_domains` | list | — | — | E-mail domains that may sign in through the provider, as for OIDC. Empty = any. |

### `runner`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `runner.enabled` | bool | `false` | `QTF_RUNNER_ENABLED` | Allow running automated test scripts on the server. Read the security chapter first. |
| `runner.timeout` | duration | `1m` | — | Wall-clock limit of one script; the process group is then killed. |
| `runner.work_dir` | string | `$TMPDIR/qtestframe-runner` | `QTF_RUNNER_WORK_DIR` | Directory of the job directories (created with mode 0700). Keep the path short: Chromium's socket path must stay below 108 bytes. |
| `runner.max_output_bytes` | integer | `1048576` | — | Maximum stored output (stdout and stderr together) of one script. |
| `runner.interpreters` | map | `bash: /bin/bash`, `javascript: /usr/bin/node`, `python: /usr/bin/python3`, `sh: /bin/sh` | — | Script language → interpreter. Remove the languages you do not need. |
| `runner.sandbox` | bool | `true` | — | Start scripts through `qtestframe sandbox-exec` (rlimits, no_new_privs, Landlock, seccomp). Only in the file. |
| `runner.deny_network` | bool | `false` | — | Block socket creation for sandboxed scripts (not for web tests). |

### `runner.web`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `runner.web.enabled` | bool | `false` | `QTF_RUNNER_WEB_ENABLED` | Web tests with Playwright in a real browser. Needs `runner.enabled`. |
| `runner.web.node` | string | `/usr/bin/node` | `QTF_RUNNER_WEB_NODE` | Node.js binary. |
| `runner.web.playwright_dir` | string | — | `QTF_RUNNER_WEB_PLAYWRIGHT_DIR` | Directory that contains `node_modules/@playwright/test`. Required (unless `command` is set). |
| `runner.web.command` | string | — | — | Command that replaces `<node> <playwright_dir>/node_modules/@playwright/test/cli.js`; `test --config <file>` is appended. Split at spaces. |
| `runner.web.browsers_path` | string | — | `QTF_RUNNER_WEB_BROWSERS_PATH` | `PLAYWRIGHT_BROWSERS_PATH`, where `npx playwright install` put the browsers (empty: Playwright's default). |
| `runner.web.browser` | string | `chromium` | — | Default browser: `chromium`, `firefox` or `webkit`. A test configuration with a Browser dimension picks another. |
| `runner.web.channel` | string | — | — | Chromium channel, e.g. `chrome` or `msedge` (empty: the bundled Chromium). |
| `runner.web.browser_channels` | bool | `false` | — | Configurations named Chrome or Edge use the installed Google Chrome or Microsoft Edge. |
| `runner.web.executable_path` | string | — | — | Chromium executable to launch instead of Playwright's. |
| `runner.web.headless` | bool | `true` | — | Run the browser without a window. |
| `runner.web.timeout` | duration | `1m` | — | Playwright's per-test timeout. |
| `runner.web.job_timeout` | duration | `10m` | — | Limit of a whole Playwright process. |
| `runner.web.screenshots` | string | `on_failure` | — | `on_failure`, `always` or `off`. |
| `runner.web.trace` | string | `on_failure` | — | `on_failure`, `always` or `off`. |
| `runner.web.video` | string | `off` | — | `off`, `on_failure` or `always`. |
| `runner.web.max_parallel` | integer | `2` | — | Background jobs (web tests and *Run automated tests*) running at the same time per server process (1–32). |
| `runner.web.max_evidence_mb` | integer | `0` (= `files.max_upload_mb`) | — | Largest screenshot, trace or video kept; larger files are left out (0: `files.max_upload_mb`). |

### `files`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `files.max_upload_mb` | integer | `25` | — | Largest uploaded file in MB (1–1024). Files are stored in the database. |

### `requirements`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `requirements.auto_create_test` | bool | `true` | — | Create a linked draft test with every new requirement (the request can opt out). |

### `hardening`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `hardening.no_new_privs` | bool | `true` | — | Set `PR_SET_NO_NEW_PRIVS` after start-up. The only hardening whose failure stops the server. |
| `hardening.landlock` | bool | `true` | — | Restrict file access with Landlock (Linux 5.13+); skipped with a log entry where unsupported. |
| `hardening.seccomp` | bool | `true` | — | Install a seccomp deny-list of dangerous system calls. |
| `hardening.drop_capabilities` | bool | `true` | — | Drop every capability except `CAP_NET_BIND_SERVICE`. |
| `hardening.read_only_paths` | list | — | — | Further paths the server may read after the lock-down. |
| `hardening.read_write_paths` | list | — | — | Further paths the server may write after the lock-down (e.g. `/run/postgresql`, a SoftHSM token directory). |

### `bootstrap`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `bootstrap.tenant_slug` | string | `default` | — | Organisation created for the first administrator (and used by `seed-demo`). |
| `bootstrap.tenant_name` | string | `Default` | — | Name of that organisation. |
| `bootstrap.admin_email` | string | — | `QTF_BOOTSTRAP_ADMIN_EMAIL` | E-mail of the first administrator, created as super admin and admin of `tenant_slug` only while the database has no users. |
| `bootstrap.admin_password` | string | — | `QTF_BOOTSTRAP_ADMIN_PASSWORD` | Its password (at least 10 characters with a letter and a non-letter). Prefer `admin_password_file`. |
| `bootstrap.admin_password_file` | string | — | — | File with that password (relative: in `$CREDENTIALS_DIRECTORY`). When set, it replaces `admin_password` and `QTF_BOOTSTRAP_ADMIN_PASSWORD`. |

### `email`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `email.enabled` | bool | on while `host` is set | — | `false` keeps the settings but sends nothing. |
| `email.host` | string | — | `QTF_EMAIL_HOST` | SMTP server. E-mail is off while it is empty. |
| `email.port` | integer | `587` | `QTF_EMAIL_PORT` | SMTP port: 587 for STARTTLS, 465 for implicit TLS. |
| `email.username` | string | — | `QTF_EMAIL_USERNAME` | User name for SMTP authentication (leave out for a relay without authentication). |
| `email.password_file` | string | — | — | File with the SMTP password (relative: in `$CREDENTIALS_DIRECTORY`). `QTF_EMAIL_PASSWORD` wins over it. |
| `email.from` | string | — | `QTF_EMAIL_FROM` | Sender address, a plain address without a name. |
| `email.from_name` | string | `Qtestframe` | — | Sender name. |
| `email.tls` | string | `starttls` | — | `starttls`, `tls` (implicit) or `none` (a local relay only). |
| `email.timeout` | duration | `30s` | — | Limit for sending one message. |
| `email.digest_hour` | integer | `7` | — | Hour of the day (UTC, 0–23) when daily digests are sent. |
| `email.reply_to` | string | — | — | Reply-To address of e-mails that have none of their own, e.g. a support mailbox (empty = none). Invitations and shared reports reply to the person who sent them. |

### `system_settings`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `system_settings.locked` | list | — | — | Settings that cannot be changed in **Administration → System settings**: keys (`logging.level`), prefixes ending in `.*` (`email.*`) or `*`. Their stored overrides are ignored. |

### `secrets`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `secrets.provider` | string | `local` | `QTF_SECRETS_PROVIDER` | Key provider that protects the stored secrets: `local`, `tpm` or `pkcs11`. |
| `secrets.local.key_file` | string | `qtestframe-secrets.key` next to a SQLite database, else in `$STATE_DIRECTORY`, `/var/lib/qtestframe` or the working directory | `QTF_SECRETS_KEY_FILE` | Key file of the local provider (256-bit key, created with mode 0600 when missing). `QTF_SECRETS_KEY` replaces the file. |
| `secrets.tpm.device` | string | `/dev/tpmrm0` | `QTF_SECRETS_TPM_DEVICE` | TPM resource manager, or the Unix socket of a TPM emulator (`swtpm`). |
| `secrets.tpm.pcrs` | list | — | — | PCRs the sealed key is bound to, e.g. `[7]` (Secure Boot state); unsealing fails after they change. |
| `secrets.tpm.pcr_bank` | string | `sha256` | — | PCR bank of the policy: `sha256` or `sha1`. |
| `secrets.pkcs11.module` | string | — | `QTF_SECRETS_PKCS11_MODULE` | PKCS#11 library of the HSM, e.g. `/usr/lib/softhsm/libsofthsm2.so`. Needs a binary built with `make go-pkcs11`. |
| `secrets.pkcs11.token_label` | string | — | — | Label of the token; this or `slot` is required. |
| `secrets.pkcs11.slot` | integer | none | — | Slot number, used instead of `token_label`. |
| `secrets.pkcs11.pin_file` | string | — | — | File with the user PIN (relative: in `$CREDENTIALS_DIRECTORY`). `QTF_SECRETS_PKCS11_PIN` wins over it. |
| `secrets.pkcs11.key_label` | string | `qtestframe-kek` | — | `CKA_LABEL` of the AES key that wraps the data keys; must be unique in the token. |
| `secrets.pkcs11.generate_key` | bool | `false` | — | Create a non-extractable AES-256 key with `key_label` when the token has none. |

### `similarity`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `similarity.embeddings.enabled` | bool | `false` | — | Use an embeddings service for the similarity of requirements and tests (suspected duplicates, the *Similar requirements* panel). Off by default: the similarity is then computed on the server. **When on, the title, description and criteria or steps of the items are sent to that service.** Also in **Administration → System settings**. |
| `similarity.embeddings.url` | string | — | — | URL of an OpenAI-compatible embeddings endpoint, e.g. `https://llm.example.com/v1/embeddings` (`POST {model, input}`, answer `{data: [{embedding}]}`). |
| `similarity.embeddings.model` | string | — | — | Model sent as `model` (empty: the service's default). Vectors are cached per item version and model; changing the model computes them again. |
| `similarity.embeddings.api_key_file` | string | — | — | File with the API key, sent as a bearer token (relative: in `$CREDENTIALS_DIRECTORY`). The key can also be stored in the system settings; it is never shown again. |
| `similarity.embeddings.timeout` | duration | `30s` | — | Longest time to wait for the service. When it fails or times out, the local similarity is used. |

### `live`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `live.enabled` | bool | `true` | `QTF_LIVE_ENABLED` | Live updates and presence: lists, dashboards, the notification bell and open items update when someone else changes something, and editors show who else is viewing or editing an item (a Server-Sent Events stream, `GET /api/v1/events/stream`). Off: the web client falls back to polling and to the conflict check on save. Each user can also turn it off in the profile. Behind a reverse proxy, turn off response buffering for the stream (see *Live updates* in the chapter *Features added later*). |
| `live.heartbeat` | duration | `25s` | — | Interval of the keep-alive comments on an idle stream. Keep it below the idle timeout of proxies and load balancers (nginx `proxy_read_timeout`, 60 s by default). |
| `live.max_connections` | integer | `2000` | — | Most open event streams of this server instance; further connections are refused with 503 and the clients poll instead. |
| `live.max_connections_per_user` | integer | `8` | — | Most open event streams of one user (one per browser tab; a tab in the background closes its stream after a minute). |
| `live.replay_buffer` | integer | `512` | — | Number of recent events kept in memory so that a client that reconnects (`Last-Event-ID`) receives what it missed; when its last event is no longer kept, it reloads what it shows instead. |
| `live.presence_ttl` | duration | `45s` | — | How long a user stays listed as viewing or editing an item after the last presence heartbeat of their browser (sent every 20 s). |

### `ai`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `ai.enabled` | bool | `false` | — | Turn on the optional [AI assistance](system-settings.md#ai-assistance) (drafts of tests, improved requirements, run summaries, framework mappings, acceptance criteria). Off by default. **When on, the text of the items a user asks about is sent to the configured service.** Each organisation must also turn it on, and projects can opt out. Also in **Administration → System settings**. |
| `ai.provider` | string | `anthropic` | — | Type of the service: `anthropic` (the Anthropic Messages API) or `openai` (an OpenAI-compatible chat completions endpoint: OpenAI, Azure OpenAI, or a local server such as Ollama or vLLM). |
| `ai.url` | string | — | — | Base URL of the service. Empty with `anthropic`: `https://api.anthropic.com`. With `openai` required, e.g. `https://llm.example.com/v1` (the server appends `/chat/completions`); an Azure OpenAI deployment URL with `?api-version=…` sends the key as `api-key`. |
| `ai.model` | string | — | — | Model id sent to the service (required): the model of your Anthropic account, or the model or deployment of your OpenAI-compatible service. There is no built-in default. |
| `ai.api_key_file` | string | — | — | File with the API key (relative: in `$CREDENTIALS_DIRECTORY`); sent as `x-api-key` (Anthropic), `Authorization: Bearer` or `api-key` (Azure). The key can also be stored, encrypted, in the system settings; it is never shown again. Local servers may need no key. |
| `ai.timeout` | duration | `2m` | — | Longest time to wait for one answer (5 s–10 min). The user can also cancel a request. |
| `ai.max_tokens` | integer | `8192` | — | Most tokens of one answer (256–128000). |
| `ai.temperature` | string | — | — | Sampling temperature sent to the service (0–2). Empty: not sent (the service's default; some models accept no other value). |
| `ai.rate_limit` | integer | `30` | — | Most AI requests of one user per hour. |
| `ai.features.draft_tests` | bool | `true` | — | Offer *Draft tests* in the requirement editor (and drafting a test from a linked requirement in the test editor). |
| `ai.features.improve_requirement` | bool | `true` | — | Offer *Improve requirement* (a rewrite for the requirement checks, shown as a diff). |
| `ai.features.summarize_run` | bool | `true` | — | Offer *Summarise failures* on the run page (a summary and defect drafts). |
| `ai.features.suggest_mappings` | bool | `true` | — | Offer *Suggest mappings* (ATT&CK, D3FEND, EMB3D, CWE) for requirements and risks, checked against the embedded lists. |
| `ai.features.suggest_criteria` | bool | `true` | — | Offer *Suggest acceptance criteria* in the requirement editor. |
| `ai.price_input` | string | — | — | Optional price per million input tokens (any currency), to show an estimated cost with each answer. |
| `ai.price_output` | string | — | — | Optional price per million output tokens. |

### `outbound`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `outbound.proxy` | string | — | — | HTTP(S) proxy for the server's outgoing calls (webhooks, chat, defect trackers and requirement sources, AI, embeddings, OIDC discovery, environment health checks), e.g. `http://proxy.example.com:3128`. Empty = the environment variables `HTTPS_PROXY`, `HTTP_PROXY` and `NO_PROXY`. |
| `outbound.no_proxy` | list | — | — | Hosts reached without `proxy` (exact names or `*.example.com`). |
| `outbound.allowed_hosts` | list | — | — | When set, the only hosts outgoing calls may reach (exact names, addresses or `*.example.com`); others are refused. Protects against server-side request forgery through webhook and tracker URLs. |
| `outbound.denied_hosts` | list | — | — | Hosts outgoing calls never reach, e.g. `metadata.google.internal` or `169.254.169.254`. |
| `outbound.block_private` | bool | `false` | — | Refuse outgoing calls to loopback, private, link-local and other non-public addresses (checked on the address actually connected to), unless the host is in `allowed_hosts`. Turn it on when the server is reachable from the internet and its users should not reach your internal network through it. |

### `retention`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `retention.enabled` | bool | `false` | — | Run the nightly retention job, which deletes records older than their retention period (see *Privacy and data retention*). Check the dry-run preview under **Administration → System settings → Retention and backups** before turning it on. |
| `retention.hour` | integer | `2` | — | Hour of the day (UTC, 0–23) the retention job runs. |
| `retention.runs_days` | integer | `0` | — | Days to keep test runs with their results, step results, comments and attachments (0 = forever). Runs in progress, of a project or release under legal hold or with an electronic signature are kept. |
| `retention.runs_keep_latest` | integer | `0` | — | Always keep the runs holding one of the latest N results of each test, however old (0 = no exception). |
| `retention.logs_days` | integer | `0` | — | Days to keep logs and uploaded artifacts: the sign-in log, finished test runner jobs, CI status posts, tracker sync logs and web test evidence (screenshots, traces, videos) (0 = forever). |
| `retention.notifications_days` | integer | `0` | — | Days to keep in-app notifications, read or not (0 = forever). |
| `retention.sessions_days` | integer | `0` | — | Days to keep sign-in sessions not used for that long and API tokens that expired that long ago (0 = forever). |
| `retention.chat_days` | integer | `0` | — | Days to keep the chat message history (sent or failed messages with their text) and the personal input suggestion history (0 = forever). |
| `retention.exploratory_days` | integer | `0` | — | Days to keep the notes of completed or debriefed exploratory sessions, counted from the session's last change (0 = forever). The session and its debrief stay. |

### `privacy`

| Key | Type | Default | Environment | Description |
|---|---|---|---|---|
| `privacy.truncate_ip` | bool | `false` | — | Store and log client addresses shortened: IPv4 to /24, IPv6 to /48 (sign-in log, sessions, remembered devices, request log). Rate limits still use the full address in memory. |
| `privacy.notice_url` | string | — | — | Link to your privacy notice, shown on the sign-in page (empty = none). |

<!-- END GENERATED: config-reference -->

## Environment variables

<!-- BEGIN GENERATED: environment (make admin-guide-ref) -->

| Variable | Sets |
|---|---|
| `QTF_CONFIG` | the configuration file when `--config` is not given |
| `QTF_BOOTSTRAP_ADMIN_EMAIL` | `bootstrap.admin_email` |
| `QTF_BOOTSTRAP_ADMIN_PASSWORD` | `bootstrap.admin_password` (replaced by `bootstrap.admin_password_file` when that is set) |
| `QTF_COOKIE_SECURE` | `auth.cookie_secure` (`1` or `true` = on, anything else off) |
| `QTF_DATABASE_DRIVER` | `database.driver` |
| `QTF_DATABASE_DSN` | `database.dsn` (replaced by `database.dsn_file` when that is set) |
| `QTF_EMAIL_FROM` | `email.from` |
| `QTF_EMAIL_HOST` | `email.host` |
| `QTF_EMAIL_PASSWORD` | the SMTP password; wins over `email.password_file` and a password stored in the web client |
| `QTF_EMAIL_PORT` | `email.port` (ignored when not a number) |
| `QTF_EMAIL_USERNAME` | `email.username` |
| `QTF_LIVE_ENABLED` | `live.enabled` (`1` or `true` = on, anything else = off) |
| `QTF_LOG_FORMAT` | `logging.format` |
| `QTF_LOG_LEVEL` | `logging.level` |
| `QTF_RUNNER_ENABLED` | `runner.enabled` (`1` or `true` = on) |
| `QTF_RUNNER_WEB_BROWSERS_PATH` | `runner.web.browsers_path` |
| `QTF_RUNNER_WEB_ENABLED` | `runner.web.enabled` (`1` or `true` = on) |
| `QTF_RUNNER_WEB_NODE` | `runner.web.node` |
| `QTF_RUNNER_WEB_PLAYWRIGHT_DIR` | `runner.web.playwright_dir` |
| `QTF_RUNNER_WORK_DIR` | `runner.work_dir` |
| `QTF_SECRETS_KEY` | the key of the local provider itself (32 bytes, base64 or hex; `qtestframe secrets generate-key`); no key file is used then |
| `QTF_SECRETS_KEY_FILE` | `secrets.local.key_file` |
| `QTF_SECRETS_PKCS11_MODULE` | `secrets.pkcs11.module` |
| `QTF_SECRETS_PKCS11_PIN` | the HSM user PIN; wins over `secrets.pkcs11.pin_file` |
| `QTF_SECRETS_PROVIDER` | `secrets.provider` |
| `QTF_SECRETS_TPM_DEVICE` | `secrets.tpm.device` |
| `QTF_SERVER_BASE_URL` | `server.base_url` |
| `QTF_SERVER_LISTEN` | `server.listen` |
| `QTF_SERVER_PORT` | the port of `server.listen` (the host stays) |
| `QTF_OIDC_<NAME>_CLIENT_SECRET` | client secret of the OIDC provider `<name>` (upper case); wins over `client_secret_file` and a secret stored in the web client |
| `CREDENTIALS_DIRECTORY` | set by systemd (`LoadCredential=`): relative `*_file` paths are resolved in it, and it is readable after the lock-down |
| `STATE_DIRECTORY` | set by systemd (`StateDirectory=`): default place of the key file when the database is not SQLite |
| `QTF_URL`, `QTF_TOKEN`, `QTF_CONFIGURATION` | defaults of `--url`, `--token` and `--configuration` of `qtestframe report` |

<!-- END GENERATED: environment -->

Boolean variables are on for `1` or `true` (any case) and off for every other value. `QTF_SERVER_PORT` only changes the port when `server.listen` is a valid `host:port`.
