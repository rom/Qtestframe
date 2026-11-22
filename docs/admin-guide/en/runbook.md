# Operations runbook

On a systemd host the examples use this alias, which runs the command as the service user with the production configuration:

```sh
alias qtf='sudo -u qtestframe env QTF_CONFIG=/etc/qtestframe/config.yaml /usr/bin/qtestframe'
```

## Routine tasks

| When | Task |
|---|---|
| daily | Backup (`qtf backup --out …`) copied off-host; look at ERROR and WARN records (`journalctl -u qtestframe -p warning --since yesterday`). |
| weekly | Certificate expiry (`scripts/certs/verify-cert.sh -q --connect …`); disk space of the database and the backups; failed sign-ins and SCIM failures. |
| monthly | A **restore test** into a scratch installation (with a copy of the key); `qtf secrets status`; review super administrators (**Administration → Users**), API tokens that were not used for a long time, SCIM tokens. |
| per release | Upgrade (see [Upgrading](backup-and-migration.md#upgrading)); compare `deploy/config.example.yaml` with your configuration. |
| yearly, or by policy | `qtf secrets rotate` (new data key), then `qtf secrets rotate --prune` once every server process has restarted; with a TPM, refresh the [recovery backup](security.md#disaster-recovery-for-tpm-sealed-keys). |

Other common tasks:

| Task | How |
|---|---|
| reload the TLS certificate | `sudo systemctl reload qtestframe` |
| change the log level for a while | **Administration → System settings → Logging and syslog**, or `QTF_LOG_LEVEL=debug` in a drop-in and a restart |
| a user forgot the password | organisation admin: **Members → Reset password**; super admin: **Users**. There is no password reset on the command line; if no super administrator can sign in, create a new one (below) |
| a user lost the authenticator and the recovery codes | **Members → Reset MFA** (organisation admin) or **Users** (super admin), after verifying the person |
| lock everyone out quickly | disable the account under **Users** (ends its sessions); revoke its API tokens; for SSO accounts disable them at the identity provider as well |
| the last super administrator is gone | on the host: `qtf user create --email … --password-stdin --super-admin` creates a new one |
| check a configuration change | `qtf check-config` before the restart |

## Capacity

- **SQLite** serialises writes through one connection. It comfortably serves tens of concurrent users and databases of a few GB on a local SSD. When requests start to queue (rising `duration_ms` in the request log under load), move to PostgreSQL.
- **PostgreSQL**: `database.max_open_conns` (default 20) per server process; the database server must allow the sum over all processes.
- **Files** are stored in the database. `files.max_upload_mb` limits one file; the database and every backup grow with the attachments.
- **Memory**: the unit caps the server at 2 GB (`MemoryMax`, `MemoryHigh=1536M`). Large imports, exports and reports need memory while they are built; web tests start a browser per job, so raise `MemoryMax` with `runner.web.max_parallel`.
- **Timeouts**: `server.write_timeout` (default 2 minutes) bounds downloads, exports and synchronous script runs; raise it for very large exports, or use the command line for backups.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| the service does not start | `journalctl -u qtestframe -b -e`; `qtf check-config` lists configuration errors. |
| `config: parse …: field listn not found` | a misspelt or removed key; the file is decoded strictly. |
| `server.base_url is required for e-mail` | set `server.base_url`, or turn e-mail off. |
| `secrets: the stored data-encryption keys are wrapped by another key: … the configured key is …` | the server has a different key than the one the data was encrypted with: wrong `key_file`, `QTF_SECRETS_KEY`, another TPM or HSM key. Configure the right key, or re-wrap with `secrets rewrap --from-config OLD.yaml`. |
| `secrets: create the key file …` | the default key location is not writable; set `secrets.local.key_file` or `QTF_SECRETS_KEY`. |
| `secrets: TPM: unseal failed; the PCRs … have changed` | firmware or boot chain changed with PCR binding: boot the previous state and re-wrap, or use the recovery backup. |
| `secrets: TPM: load the sealed key (was it sealed by another TPM, or was the TPM cleared?)` | the data was sealed on another machine or the TPM was cleared; see [disaster recovery](security.md#disaster-recovery-for-tpm-sealed-keys). |
| `secrets: open TPM /dev/tpmrm0: …` | the service cannot open the device: drop-in with `PrivateDevices=no`, `DeviceAllow=/dev/tpmrm0 rw`, group `tss`. |
| `secrets: the pkcs11 (HSM) key provider is not available in this binary` | build with `make go-pkcs11`. |
| `secrets: pkcs11: log in to token … (check the PIN)` / `no token with …` / `has no AES key labelled …` | PIN, token label or slot, or key label; `generate_key: true` creates the key. |
| `hardening: …` and the start stops | `no_new_privs` failed; check the container runtime or init system. |
| `permission denied` for a file after start-up | the path is not in the Landlock rules: add it to `hardening.read_only_paths` / `read_write_paths` (it must exist at start-up). |
| `landlock: …` in `skipped` | kernel without Landlock; add `landlock` to the `lsm=` boot parameter, or accept it. |
| `Operation not permitted` in a system call | a systemd `SystemCallFilter` or the seccomp list; confirm with `ausearch -m SECCOMP` and report it. |
| SELinux or AppArmor denials | `ausearch -m AVC -ts recent -c qtestframe` and `getsebool -a \| grep qtestframe`; `journalctl -k \| grep apparmor="DENIED"`. |
| bind on `:443` fails | `CAP_NET_BIND_SERVICE` removed by a drop-in, or SELinux `qtestframe_bind_http_port` off; or redirect 443 to 8443 with nftables. |
| nobody can sign in over plain HTTP | `auth.cookie_secure` must be `false` without TLS. |
| `429 rate_limited` at sign-in | `auth.login_max_attempts` within `auth.login_lockout` per address and e-mail; behind a proxy, set `server.trusted_proxies` or everyone shares one address. |
| single sign-on loops, `oidc: state mismatch` | `server.base_url` must match the browser's address; session affinity with several instances; the login took over 10 minutes; clock skew. |
| `external login failed … oidc …: discovery` | the server cannot reach the issuer: egress firewall, proxy, SELinux `qtestframe_connect_http`. |
| `external login failed … e-mail address not verified` / `no e-mail claim` | the IdP sends `email_verified: false` or no `email_claim`. |
| `external login failed … user not provisioned` | the user does not exist and `auto_provision` is off. |
| `external login failed … e-mail already bound to another identity provider` | an account with that e-mail is linked to another provider. |
| `403 mfa_enrolment_required` for API calls of a browser session | the MFA policy requires a second factor; the user must set one up. |
| `stored system settings are invalid and were ignored` | a file change made a stored setting invalid; fix or reset it under **System settings**. |
| `backup schema version N does not match the database schema version M` | restore with the version that made the backup, then upgrade. |
| PostgreSQL TLS errors | `sslrootcert` readable by the service; `sslmode=verify-full` needs the server name in the certificate. |
| web tests fail at once | `MemoryDenyWriteExecute=no` and `ExecPaths=` for the Playwright directory; `/dev/shm` writable; `runner.work_dir` short (socket path under 108 bytes). |
| scripts: `409` with `not_ready` | the test execution settings forbid the result; see the run's readiness column. |

## Log messages to look for

| Message | Level | Meaning |
|---|---|---|
| `qtestframe listening` | INFO | the server is up (address, TLS, version, database) |
| `hardening` | INFO | which restrictions were applied and which skipped |
| `secrets encrypted` | INFO | the key provider, key fingerprint and active data key in use |
| `secrets: created the key file …` | WARN | a new key file was created: back it up now |
| `secrets: the key file … is readable by group or others` | WARN | fix the mode with `chmod 600` |
| `secrets: encrypted N secret(s) that were stored as plaintext` | WARN | values of an older version were encrypted at this start |
| `applied database migrations` | INFO | an upgrade migrated the schema |
| `bootstrapped administrator` | INFO | the first administrator was created |
| `no users exist; …` | WARN | there is no account yet and no bootstrap configuration |
| `stored system settings are invalid and were ignored; …` | ERROR | see above |
| `login failed`, `second factor failed` | WARN | a failed sign-in, with e-mail and address |
| `external login failed` | WARN | a failed OIDC or SAML sign-in, with the reason |
| `scim authentication failed` | WARN | a request with a wrong SCIM token |
| `request failed` | ERROR | an unexpected error behind a `500` answer, with path and error |
| `tls certificate reloaded` / `tls reload failed; keeping previous certificate` | INFO / ERROR | the result of a `SIGHUP` |
| `webhook delivery` | WARN | a webhook could not be delivered |
| `digest job`, `report subscriptions job` | WARN | scheduled e-mail failed |
| `reset interrupted job` | INFO | a web test job of a stopped process was reset |
| `audit write failed` | WARN | an audit entry could not be stored (the log record is still written) |
| `shutting down` | INFO | the server received `SIGTERM` or `SIGINT` |
