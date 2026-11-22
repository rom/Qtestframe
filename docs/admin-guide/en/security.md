# Security

This chapter is the operator's view. The threat model and the design behind it are in [docs/security.md](https://github.com/rom/Qtestframe/blob/rom/docs/security.md).

## TLS and certificates

Set `server.tls_cert` and `server.tls_key` to serve HTTPS directly. The server accepts TLS 1.2 and 1.3 only; TLS 1.2 is limited to ECDHE key exchange with AES-GCM or ChaCha20-Poly1305; ALPN offers HTTP/2. The certificate file holds the full chain, leaf first.

Scripts in `scripts/certs/`:

```sh
# Let's Encrypt (public name, port 80 reachable, or a DNS-01 method)
sudo scripts/certs/letsencrypt.sh --domain qtf.example.com --email ops@example.com

# an internal CA and a server certificate signed by it
scripts/certs/gen-ca.sh --dir /root/qtf-ca --cn "ACME Internal CA"
sudo scripts/certs/gen-server-cert.sh --ca-dir /root/qtf-ca --cn qtf.example.com \
    --san DNS:qtf.example.com,IP:10.0.0.5 --out /etc/qtestframe/tls --group qtestframe

# check a certificate, or what a server presents
scripts/certs/verify-cert.sh --cert /etc/qtestframe/tls/server.crt --key /etc/qtestframe/tls/server.key --host qtf.example.com
scripts/certs/verify-cert.sh -q --connect qtf.example.com:8443 --warn-days 21    # exit 2 = expires soon
```

**Renewal without downtime:** `systemctl reload qtestframe` sends `SIGHUP`, and the server loads the new pair for new connections; if it cannot be read, the old one stays in use and `tls reload failed; keeping previous certificate` is logged. The Let's Encrypt deploy hook does this for you. Key files: `0640 root:qtestframe`; with SELinux run `restorecon -RF /etc/qtestframe/tls`.

**Behind a TLS-terminating reverse proxy:** listen on `127.0.0.1`, set `server.trusted_proxies` to the proxy's address, keep `auth.cookie_secure: true` and `server.base_url` on `https://`, and let the proxy send `Strict-Transport-Security` – Qtestframe sends HSTS only on TLS connections it terminates itself.

## Hardening

### In the process

After start-up – once the database, the key provider, the certificate and the listener are open – `serve` restricts itself (`internal/hardening`, Linux only):

| Restriction | Key | What it does |
|---|---|---|
| `no_new_privs` | `hardening.no_new_privs` | no privilege gain through setuid binaries or file capabilities, for the server and everything it starts. A failure **stops the start**. |
| capabilities | `hardening.drop_capabilities` | drops every capability except `CAP_NET_BIND_SERVICE` (bounding, effective, permitted, inheritable, ambient) |
| Landlock | `hardening.landlock` | file access limited to the paths the configuration needs (best effort up to Landlock ABI 5) |
| seccomp | `hardening.seccomp` | a deny-list of system calls that a server never needs; they fail with `EPERM` |

Except for `no_new_privs`, a restriction that the kernel does not support is skipped, not fatal. The start-up log line shows the result:

```text
time=… level=INFO msg=hardening applied="[no_new_privs capabilities landlock seccomp]" skipped=[]
```

**Landlock paths.** The server may read the built-in list below plus the files it was configured with (TLS, syslog and SAML files, `$CREDENTIALS_DIRECTORY`), and write the temporary directory, the SQLite database directory, a Unix syslog socket, the TPM device, and with the runner its work directory (and execute from `/usr`, `/bin`, `/lib`, `/lib64`, `/opt`, the binary's directory and the interpreters' directories). Web tests add the Playwright, browser and Node.js directories, `/etc`, `/proc`, `/sys` and `/dev/shm`. **Only paths that exist at start-up are added.** Anything else needs `hardening.read_only_paths` or `hardening.read_write_paths`, for example `/run/postgresql` for a local PostgreSQL socket or the token directory of SoftHSM.

<!-- BEGIN GENERATED: hardening (make admin-guide-ref) -->

| List | Entries |
|---|---|
| Read-only paths after the lock-down (besides the configured files) | `/etc/ssl`, `/etc/pki`, `/etc/ca-certificates`, `/usr/share/ca-certificates`, `/etc/resolv.conf`, `/etc/hosts`, `/etc/nsswitch.conf`, `/etc/localtime`, `/etc/gai.conf`, `/etc/services`, `/etc/protocols`, `/usr/share/zoneinfo`, `/etc/qtestframe`, `/proc/self` |
| System calls denied by seccomp (`EPERM`) | `acct`, `add_key`, `bpf`, `chroot`, `clock_adjtime`, `clock_settime`, `delete_module`, `finit_module`, `fsconfig`, `fsmount`, `fsopen`, `fspick`, `init_module`, `iopl`, `ioperm`, `kexec_file_load`, `kexec_load`, `keyctl`, `lookup_dcookie`, `mount`, `mount_setattr`, `move_mount`, `name_to_handle_at`, `open_by_handle_at`, `open_tree`, `perf_event_open`, `personality`, `pivot_root`, `process_vm_readv`, `process_vm_writev`, `ptrace`, `quotactl`, `reboot`, `request_key`, `setdomainname`, `sethostname`, `setns`, `settimeofday`, `swapoff`, `swapon`, `syslog`, `umount2`, `unshare`, `userfaultfd`, `vhangup` |
| Further system calls denied to scripts with `runner.deny_network` | `socket`, `socketpair`, `connect`, `bind`, `listen`, `accept`, `accept4` |

<!-- END GENERATED: hardening -->

A `permission denied` after start-up for a file the server needs means a path is missing from these rules. A `landlock: …` entry under `skipped` means the kernel lacks Landlock (older than 5.13, or `landlock` missing from the `lsm=` boot parameter).

### Operating-system layers

The in-process hardening also works in containers and without systemd. On a host, more layers apply:

| Layer | File | Notes |
|---|---|---|
| systemd sandbox | `deploy/systemd/qtestframe.service` | own user, `ProtectSystem=strict`, private `/tmp` and devices, `NoNewPrivileges`, only `CAP_NET_BIND_SERVICE`, system call filter, `MemoryDenyWriteExecute`, `MemoryMax`/`TasksMax`. `systemd-analyze security qtestframe` should report about 1.x. |
| SELinux | `deploy/selinux/` | domain `qtestframe_t`, booleans for optional features (off by default): `qtestframe_connect_postgresql`, `qtestframe_remote_syslog`, `qtestframe_connect_http`, `qtestframe_bind_http_port`, `qtestframe_run_scripts`, `qtestframe_runner_execmem` |
| AppArmor | `deploy/apparmor/usr.bin.qtestframe` | read-only configuration, scripts in the child profile `qtestframe//runner` without network |
| container seccomp | `deploy/seccomp/qtestframe-seccomp.json` | an allow-list for container runtimes |
| firewall, sysctl | `deploy/nftables/`, `deploy/sysctl/` | inbound and optional outbound policy, kernel settings |

Verify a running server:

```sh
PID=$(systemctl show -p MainPID --value qtestframe)
grep -E 'NoNewPrivs|Seccomp|CapEff' /proc/$PID/status    # NoNewPrivs 1, Seccomp 2, CapEff 0000000000000400
journalctl -u qtestframe | grep -i hardening
systemd-analyze security qtestframe
```

### The script and web test sandbox

When `runner.enabled` is on, members with `script.execute` run code on the server. With `runner.sandbox: true` (default) each script runs through `qtestframe sandbox-exec`, which before starting the interpreter sets resource limits (CPU time = timeout + 1 s, 64 MiB file size, 256 open files, no core dumps), no_new_privs, drops capabilities, applies a Landlock rule set (read-only system directories, write only to the job directory) and the seccomp deny-list, with `runner.deny_network: true` also the socket calls. The environment holds only `PATH`, `HOME`/`TMPDIR` (the job directory), `LANG`, the `QTF_*` run variables and the project's test variables. The whole process group is killed at `runner.timeout`.

**Web tests have a weaker sandbox:** no seccomp filter and always network access, because a browser needs both. A web test can reach every host the server can reach, including internal services and cloud metadata endpoints. Restrict the server's outgoing connections (firewall, egress proxy) or run web tests on a dedicated instance.

Even with every layer, treat the runner as a boundary that a determined person with `script.execute` may eventually cross. Note that `/etc` (with `/etc/qtestframe`) is readable inside the sandbox: protect configuration and keys with file permissions and SELinux/AppArmor.

## Encryption of stored secrets

The server must read some secrets back, so it cannot hash them. These are stored **encrypted** with AES-256-GCM:

| Secret | Stored in |
|---|---|
| TOTP secrets of authenticator apps | `user_mfa.totp_secret` |
| webhook signing secrets | `webhooks.secret` |
| defect tracker tokens (Jira, GitHub, GitLab) | `projects.defect_tracker` |
| values of secret web test variables | `projects.web_tests` |
| SMTP password, OIDC client secrets and the key of the embeddings service entered in the web client | `system_settings` |

Passwords (bcrypt), API and SCIM tokens, sessions, invitations, remembered devices and recovery codes are stored as hashes and need no key. Secrets of the configuration file, `*_file` options and `QTF_*` variables are never stored in the database.

**Envelope encryption.** Each value is encrypted with a random 256-bit *data-encryption key* (DEK), stored as `enc:v1:<dek id>:<data>`, and bound to its table, column, row and field, so a value copied elsewhere does not decrypt. The DEKs are kept in the table `secret_keys`, **only wrapped** by the *key-encryption key* (KEK) of the configured **key provider**. The KEK never enters the database; unwrapped DEKs exist only in memory.

At every start, and with `qtestframe secrets migrate`, values that an older version stored as plaintext are encrypted in place. `qtestframe secrets status` shows the provider, the key fingerprint, the DEKs and how many values are encrypted; **Administration → System settings → Security** shows the same read-only.

### Key providers

| Provider | KEK | Protects against | Build |
|---|---|---|---|
| `local` (default) | a 256-bit key in a key file, or `QTF_SECRETS_KEY` | copies of the database, dumps, backups and replicas that leave without the key | static |
| `tpm` | sealed to this machine's TPM 2.0, optionally bound to PCR values | additionally: a copied disk, and with PCRs a changed boot chain | static (pure Go) |
| `pkcs11` | an AES-256 key in an HSM, used through PKCS#11 | additionally: the key can never be copied out of the HSM | `make go-pkcs11` (cgo) |

None of them protects against someone who controls the running server.

**local.** Nothing to configure. The key file `qtestframe-secrets.key` is created with mode 0600 when missing, next to a SQLite database (else in `$STATE_DIRECTORY`, `/var/lib/qtestframe` or the working directory); `secrets.local.key_file` puts it elsewhere, for example on another volume. Or generate a key and give it in the environment from your secret store – then no file is used:

```sh
qtestframe secrets generate-key        # prints a base64 key for QTF_SECRETS_KEY
```

The server warns at start-up when the key file is readable by group or others. Several server processes on one PostgreSQL database need the **same** key.

**tpm.**

```yaml
secrets:
  provider: tpm
  tpm:
    device: /dev/tpmrm0
    pcrs: [7]              # optional: bind to the Secure Boot state
    pcr_bank: sha256
```

Give the service the device in a drop-in, because the unit has `PrivateDevices=yes`:

```ini
[Service]
PrivateDevices=no
DeviceAllow=/dev/tpmrm0 rw
SupplementaryGroups=tss
```

The DEK is sealed under the TPM's storage root key through a salted session, so it never crosses the bus in clear. It unseals only on the same TPM (until its owner hierarchy is cleared), and with `pcrs` only while those PCRs keep their values. Test against a software TPM with `swtpm` (`make test-tpm`).

**pkcs11.**

```yaml
secrets:
  provider: pkcs11
  pkcs11:
    module: /usr/lib/softhsm/libsofthsm2.so
    token_label: qtestframe       # or slot: 0
    pin_file: hsm-pin             # or QTF_SECRETS_PKCS11_PIN
    key_label: qtestframe-kek
    generate_key: false           # true: create a non-extractable AES-256 key if missing
```

The DEK is wrapped with `CKM_AES_GCM` by the HSM key. The binary must be built with `make go-pkcs11`. With Landlock, a file-based token such as SoftHSM needs its token directory in `hardening.read_write_paths`. For a test token: `softhsm2-util --init-token --free --label qtestframe --pin … --so-pin …`.

**If the key is missing or wrong**, the server refuses to start with an error that names the key it expects (for example `the stored data-encryption keys are wrapped by another key: … the configured key is …`). It never falls back to plaintext.

### Rotation and re-wrapping

| Task | Command | Effect |
|---|---|---|
| new data key | `qtestframe secrets rotate` | creates a new DEK, makes it active and re-encrypts every value; old DEKs are kept |
| remove old data keys | `qtestframe secrets rotate --prune` | as above, then deletes the DEKs no value uses; stop or restart the other server processes first |
| new KEK, or another provider | `qtestframe secrets rewrap --config NEW.yaml --from-config OLD.yaml` | unwraps the DEKs with the provider of `OLD.yaml` and wraps them with the one of `NEW.yaml`; the stored values are unchanged |

Other server processes pick up a rotated DEK when they meet a value encrypted with it. **Procedure for a new KEK or provider** (for example a new local key file, local → TPM, local → HSM):

1. Take a backup (`qtestframe backup`) and keep the current key.
2. Copy the configuration: `cp config.yaml config.new.yaml`, and change the `secrets` section in the copy (new `key_file`, or `provider: tpm` / `pkcs11`).
3. Stop the server, then run `qtestframe secrets rewrap --config config.new.yaml --from-config config.yaml`. A new local key file is created by this step.
4. Make `config.new.yaml` the configuration and start the server; check `qtestframe secrets status`.
5. Keep the old key until the new set-up works – **backups taken before need the old key**.

### Disaster recovery for TPM-sealed keys

A TPM cannot be backed up, and with `pcrs` even a firmware update can make the key unsealable. Plan for it:

- **Planned changes** (firmware, Secure Boot keys, boot loader, a new mainboard): first re-wrap to a local key (`secrets rewrap` with a `local` configuration as `--config` and the TPM configuration as `--from-config`), run with the local key during the change, then re-wrap back to the TPM.
- **Unplanned loss** (broken mainboard, cleared TPM): only a **recovery backup** whose DEKs are wrapped by a key you hold elsewhere helps. Make one regularly, and after every `secrets rotate`, on the TPM machine and as the service user (which may use the TPM):

  ```sh
  # recovery-tpm.yaml:    sqlite /var/lib/qtestframe/recovery.db, secrets as in production (tpm)
  # recovery-local.yaml:  the same database, provider local, key_file on removable or offline storage
  qtestframe backup  --config /etc/qtestframe/config.yaml --out /var/backups/qtestframe/qtf.zip
  qtestframe restore --config recovery-tpm.yaml --in /var/backups/qtestframe/qtf.zip --yes
  qtestframe secrets rewrap --config recovery-local.yaml --from-config recovery-tpm.yaml
  qtestframe backup  --config recovery-local.yaml --out /var/backups/qtestframe/qtf-recovery.zip
  rm /var/lib/qtestframe/recovery.db*
  ```

  Store `qtf-recovery.zip` and the escrow key apart from each other (the key never next to the backups). The restore into the scratch database works because the same TPM unwraps the keys; the re-wrap creates the escrow key file. To recover, restore the recovery backup on a server configured with the escrow key (local provider), then re-wrap to the new TPM.
- If the KEK is lost and there is no recovery backup, the stored secrets cannot be recovered; the server refuses to start on that database.

The same applies, in a weaker form, to HSM keys (use the HSM's own key backup) and to local key files (back them up – separately from the database).

## The audit log

Every organisation has an audit log (**Administration → Audit log**, permission `audit.read`, API `GET /api/v1/audit`, newest first, up to 5000 entries per request): who (user id and e-mail, or `SCIM: <token>`, or `cli`), what (`action`, entity, short details) and when. Recorded are sign-ins (with the method, and `mfa=…`, `amr`/`acr` for single sign-on), changes of projects, items, runs, members, roles, tokens, sharing, organisations and users, exports, backups and restores (`admin.backup`, `admin.restore`), MFA resets and policy changes, and system settings (`settings.update`). Old and new values of items are kept in the item history, not in the audit log; the client address is in the request log, not in the audit entry.

Each audit entry is also written as an INFO log record `audit`. Ship the log to a central syslog server (see [Logging](logging-and-monitoring.md#central-syslog)), so the trail survives a compromise of the host. Failed sign-ins appear in the server log (`login failed`) and in the user's sign-in history, not in the audit log. The audit log is part of a backup. Entries also record the reason for change given in electronic-records mode (`reason`).

### Tamper-evident audit trail

The audit log of each organisation is a **hash chain** (the system entries of the command line, such as `admin.backup`, have their own chain). Every entry gets a sequence number (`seq`, 1, 2, …), the hash of the entry before it (`prev_hash`) and its own hash: SHA-256 over a canonical encoding of its fields (organisation, sequence number, id, UTC time to the microsecond, user, action, entity, details, reason for change) and the previous hash. The exact encoding is in `docs/api.md` ("Tamper-evident audit trail"), so an export can be checked with other tools.

- **How entries are chained.** An entry is written in the transaction of the change it records (a rolled-back change leaves no entry) and chained right after that transaction commits, in a short transaction of its own that holds the organisation's row in `audit_heads` (PostgreSQL: `SELECT … FOR UPDATE`). So appends of one organisation are serialised without a lock during the request, and several server processes can share the database. Until it is chained (normally milliseconds) an entry shows `seq` 0; entries left unchained by a process that stopped are chained by the next entry or the next start.
- **Signed checkpoints.** Every 1 000 entries, at least hourly while entries are written, and at every verification and export, the server writes a checkpoint: an HMAC-SHA256 over the organisation, a sequence number and that entry's hash, with a 256-bit key in `audit_keys` that is **encrypted like the other stored secrets** (see [Encryption of stored secrets](#encryption-of-stored-secrets)). Someone who can change the database but not read the server's key (the key file, TPM or HSM) can recompute hashes, but cannot forge checkpoints: rewriting the chain or removing entries before the newest checkpoint is detected. Entries after the newest checkpoint can be removed undetected by such an attacker; the central syslog copy and regular exports close that gap. Without a key provider the checkpoint key is stored as plaintext and the verification says so.
- **What it detects**, with the first broken link: a changed entry (`entry_changed`), a removed, replaced or reordered entry (`link_broken`, `sequence_gap`), entries removed at the end (`truncated`, against the recorded end of the chain and the checkpoints), a rewritten chain (`checkpoint_mismatch`) and a forged checkpoint (`checkpoint_signature`). It does **not** prevent changes: a database administrator can still change or delete rows; the chain makes it evident. Anyone with the server's key and the database can rewrite everything – protect the key and the hosts accordingly.
- **Verify** under **Administration → Audit log → Verify audit trail** (permission `audit.read`, `POST /api/v1/audit/verify`), or on the server:

  ```sh
  sudo -u qtestframe qtestframe audit verify --config /etc/qtestframe/config.yaml            # every organisation; exit 1 when broken
  sudo -u qtestframe qtestframe audit verify --config /etc/qtestframe/config.yaml --tenant acme --json
  ```

  Run it from a timer and alert on exit status 1. 100 000 entries verify in about a second (SQLite and PostgreSQL on a laptop); the first chaining after the upgrade chains 100 000 entries in 2–4 seconds.
- **Export for external archiving**: **Export with hashes (JSON / CSV)** (`GET /api/v1/audit/archive?format=json|csv`) gives the whole chain with sequence numbers and hashes; JSON also has the checkpoints and the end of the chain. `qtestframe audit verify --file EXPORT.json` checks an export without the database (entries, links, end, checkpoint hashes; the checkpoint signatures need the server key). Keep exports on write-once storage to extend the protection beyond the newest checkpoint.
- **After the upgrade** (migration 0055) the existing entries of each organisation are chained at the first start: the chain begins with an `audit.chain_start` entry that says how many earlier entries follow and when they were chained. The chain proves that those entries were not changed **after** that moment, not before.

The chain in backups, restores and project archives is described in [Backup, restore and migration](backup-and-migration.md#the-audit-trail-in-backups).

## Electronic-records mode (21 CFR Part 11 / EU GMP Annex 11)

An organisation setting under **Administration → Electronic records** (change: `tenant.manage`; read and report: `audit.read`; `GET`/`PUT /api/v1/erecords`), **off by default**. When it is on, the server enforces:

| Control | What the server does |
|---|---|
| Re-authentication at every signature | Baseline sign-offs, waiver decisions, risk acceptance, sign-offs of verification records and the actions below need the signer's **password** and, when the signer has a second factor, a **current code** (a recovery code works too). Accounts without a password here (single sign-on) sign within *SSO signing window* minutes (default 5) of signing in at the identity provider, plus the code when they have a second factor here. API tokens cannot sign. Failed attempts are rate-limited. |
| Meaning of the signature | The signer chooses a meaning from the organisation's list (default authored, reviewed, approved, verified, released; up to 20, your own words allowed). |
| Manifestation | Every signature records the printed name, e-mail, UTC time, the signer's time zone, the meaning, the method (`password`, `password+mfa`, `sso`, `sso+mfa`) and the hash of the record version signed; signed records show this, and a later change of the record shows *Record changed after signing*. |
| Actions that need a signature | Chosen from: requirement approval (status → approved, also by bulk change, spreadsheet and bulk review), test approval (status → ready), release go (status → released), baseline creation. Enforced on every route that performs them; without a signature the request answers 428 `signature_required`. |
| Reason for change | Chosen kinds of change (requirement, test, run, execution, release, baseline, risk, waiver and verification-record changes and deletions) need a reason, recorded in the audit entries of the change. |
| Inactivity timeout | Browser sessions end after *Sign out after inactivity* minutes (1–480, default 15) without requests; the client's own polling does not count. |

The settings are cached for up to 5 seconds per server process. Signatures given in the dialogs that existed before (baseline sign-offs, waiver decisions, risk acceptance) are recorded with their manifestation in every mode.

**Part 11 / Annex 11 controls report.** **Create report** on the same page (`GET /api/v1/erecords/report`, `?format=md` for Markdown) lists every control with its references, who covers it (*tool*, *tool and procedures*, *your procedures*), its status here and the evidence (settings, a fresh verification of the audit trail, signature counts, multi-factor use).

### What the tool provides – and what your procedures must cover

Qtestframe provides technical controls. It is **not** "Part 11 compliant" by itself, and no setting makes it so: compliance is a property of the validated system in use, its procedures and its people.

| The tool provides | Your procedures must cover |
|---|---|
| A computer-generated, time-stamped, tamper-evident audit trail with verification and export | Validation of the installed version for its intended use (and after upgrades); review of audit trails as part of record review; periodic verification and archiving of exports |
| Signatures with re-authentication, meaning, manifestation and linking to the record version | A written policy that electronic signatures are binding, and the certification to the FDA (§11.100(c)); deciding which records need which signatures and when re-signing is needed |
| Role-based access, multi-factor authentication, session expiry and an inactivity timeout | Identity checks before an account is created, never reusing accounts, access reviews, leaver processes, password ageing and loss management (or your identity provider's) |
| Backups that keep the chain valid; exports in open formats | Backup schedule, restore tests, retention periods, business continuity and disaster recovery |
| Documentation of each version | Training, change control of the installation and its configuration, supplier assessment |

Time stamps come from the server clock: keep it synchronised (NTP).

## Security headers and CSP

Every response carries:

| Header | Value |
|---|---|
| `Content-Security-Policy` | `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self' https:; object-src 'none'` |
| `Strict-Transport-Security` | `max-age=63072000; includeSubDomains`, with `server.hsts: true` on TLS connections the server terminates |
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=(), payment=()` |
| `Cross-Origin-Opener-Policy`, `Cross-Origin-Resource-Policy` | `same-origin` |
| `Cache-Control` | `no-store` for `/api/…` |

The CSP allows no inline scripts and no `eval`; inline styles are needed by the UI library, and `form-action https:` by the single sign-on redirects. A reverse proxy should pass these headers through unchanged; Qtestframe cannot be embedded in frames of other sites.

Other limits: JSON bodies at most 8 MiB, headers 1 MiB, `ReadHeaderTimeout` 10 s, `IdleTimeout` 120 s; queries at most 4096 bytes with RE2 regular expressions (no catastrophic backtracking).

## Outgoing connections

Project administrators (`project.update`) choose the URLs of trackers and webhooks, and script authors (`script.execute`) the targets of web tests; super administrators can make the server connect to addresses with the test buttons of the system settings. When the optional [embeddings service](system-settings.md#embeddings-service-for-similar-items) is on, the server sends the text of requirements and tests to it. Every such request leaves from the server's network. If the server can reach internal services, filter its egress with a firewall or proxy (`deploy/nftables` has an optional egress policy). Webhook deliveries never follow redirects.

## Checklist

- [ ] TLS certificate from a trusted CA; `server.hsts: true`; `auth.cookie_secure: true`; `server.base_url` on `https://`
- [ ] single sign-on configured; `auth.local: false`, or few local accounts with strong passwords
- [ ] MFA required at least for administrators (MFA policy, or at the identity provider with `local_mfa` set accordingly)
- [ ] secrets in `*_file` options or `LoadCredential=`, files `0640 root:qtestframe`
- [ ] key of the stored secrets backed up apart from the database (or TPM/HSM with a recovery plan); `secrets status` shows no plaintext
- [ ] PostgreSQL with `sslmode=verify-full` and a dedicated role
- [ ] logs shipped to a central syslog server over `tls`
- [ ] `runner.enabled: false` unless needed; otherwise only trusted people with `script.execute`, MAC enforced, egress filtered
- [ ] SELinux module or AppArmor profile loaded; `systemd-analyze security qtestframe` ≤ 2.0
- [ ] `server.trusted_proxies` set behind a reverse proxy
- [ ] backups encrypted, stored off-host and a restore tested
