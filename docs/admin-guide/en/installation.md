# Installation

## System requirements

| | |
|---|---|
| **Operating system** | Linux on amd64 or arm64 for production. macOS (arm64) works for trying it out and for development; the in-process hardening (Landlock, seccomp, capabilities) exists only on Linux and is reported as skipped elsewhere. |
| **Kernel** | Landlock needs Linux 5.13 or later with `landlock` among the active security modules; older kernels run without it (logged at start-up). |
| **Memory and processes** | The shipped systemd unit allows 2 GB (`MemoryMax=2G`) and 512 tasks; the compose file 1 GB and 256 processes. Web tests start a browser per job and need considerably more (see `runner.web.max_parallel`). |
| **Disk** | The database holds every uploaded file. Plan for the attachments and documents of your projects plus the backups. |
| **Database** | Nothing for SQLite (built in, pure Go). PostgreSQL 13 or later (16 recommended) for larger installations. |
| **Optional** | An SMTP server for e-mail; an OIDC or SAML identity provider; a syslog server; Node.js 18+ and Playwright for web tests; Python, Bash and Node.js for scripts; a TPM 2.0 or an HSM for the encryption key. |
| **To build** | Go 1.26 (`go.mod` pins the toolchain 1.26.11, which `go` downloads), Node.js 20 or later (CI uses 22), `make`. A C compiler only for the HSM build. |

## SQLite or PostgreSQL

| | SQLite (default) | PostgreSQL |
|---|---|---|
| Set-up | none: the file is created at the first start | a separate database server |
| Server processes | exactly **one** per database file | one or several (load balancer, rolling restarts) |
| Concurrency | one connection, writes serialised; fine for tens of concurrent users and databases of a few GB | a connection pool (`database.max_open_conns`, default 20) |
| Backup | `qtestframe backup`, or a file copy with SQLite's online backup | `qtestframe backup`, `pg_dump`, point-in-time recovery |
| High availability | no | replication of the database server |
| File system | local disk only (not NFS); WAL files `-wal` and `-shm` next to it | — |

Choose **SQLite** for a single server, evaluations and teams that want the least to run. Choose **PostgreSQL** when you need more than one server process, the database must be managed by a database team, or you want replication and point-in-time recovery. You can move later in either direction with a [logical backup](backup-and-migration.md#moving-between-sqlite-and-postgresql).

## From a binary

The binary is static (`CGO_ENABLED=0`, the SQLite driver is pure Go), so a copy of `qtestframe` is the whole installation. Static binaries for Linux amd64/arm64 and macOS arm64 are built with:

```sh
make release          # dist/qtestframe-linux-amd64, -linux-arm64, -darwin-arm64
```

Try it with the ready-made example configuration (HTTP on port 8080, SQLite in `./data`, a bootstrap administrator):

```sh
./qtestframe serve --config examples/qtestframe.yaml
```

Then sign in at `http://localhost:8080` as `admin@example.com` with `Change-me-1234` and change the password. **This example is not for production**: it turns off secure cookies and HSTS, enables the script runner and contains the bootstrap password in the file.

## On a systemd host

`scripts/install.sh` installs the binary, a hardened unit, the service user and the directory layout. It is idempotent: running it again upgrades binary and unit and keeps configuration, keys and data.

```sh
make build                                      # web client + bin/qtestframe
sudo scripts/install.sh --binary bin/qtestframe --self-signed-cert qtf.example.com
```

| Option | Effect |
|---|---|
| `--binary PATH` | binary to install (default `./bin/qtestframe` or `./dist/qtestframe`) |
| `--selinux auto\|yes\|no` | build and load the SELinux module (default `auto`: when SELinux is enabled) |
| `--apparmor` | install and enforce the AppArmor profile |
| `--sysctl` | install `deploy/sysctl/99-qtestframe.conf` |
| `--self-signed-cert [CN]` | create a self-signed certificate when none exists |
| `--no-enable`, `--no-start` | do not enable or (re)start the unit |
| `--dry-run` | print the actions only |

It creates:

| Path | Owner and mode | Content |
|---|---|---|
| `/usr/bin/qtestframe` | root, 0755 | the binary |
| `/etc/qtestframe/config.yaml` | root:qtestframe, 0640 | the configuration, from `deploy/config.example.yaml`; never overwritten |
| `/etc/qtestframe/tls/` | root:qtestframe, 0750 | `server.crt`, `server.key` (0640), SAML and client keys |
| `/etc/qtestframe/secrets/` | root:qtestframe, 0750 | files referenced by `*_file` options |
| `/var/lib/qtestframe/` | qtestframe, 0750 | the SQLite database, the key file, `runner/` |
| `/etc/systemd/system/qtestframe.service` | root, 0644 | the unit (`deploy/systemd/qtestframe.service`) |
| `/etc/sysusers.d/`, `/etc/tmpfiles.d/` | root | the user `qtestframe` and the directories |

If the configured certificate does not exist yet, the service is not started. Edit `/etc/qtestframe/config.yaml` (at least `server.base_url`, TLS and the bootstrap administrator, see [First start](first-start.md)), then:

```sh
sudo systemctl enable --now qtestframe
journalctl -u qtestframe -f
```

The unit runs `qtestframe migrate` before `serve` (`ExecStartPre`), restarts the service on failure, and `systemctl reload qtestframe` sends `SIGHUP`, which re-reads the TLS certificate. Change the unit only with a drop-in (`sudo systemctl edit qtestframe`), for example for `LoadCredential=`, `MemoryMax=` or the relaxations the runner needs (see [Integrations](integrations.md#script-and-web-test-runner)).

Remove it with `sudo scripts/uninstall.sh` (keeps configuration and data) or `--purge` (deletes everything, the database included).

### Firewall, MAC and port 443

- `deploy/nftables/qtestframe.nft` is a firewall ruleset with an optional egress policy; review its sets (`admin_v4`, `out_tcp_ports`, …) before loading it. Let's Encrypt HTTP-01 needs inbound port 80.
- SELinux (`deploy/selinux`) and AppArmor (`deploy/apparmor`) profiles confine the service; optional features are behind SELinux booleans such as `qtestframe_connect_postgresql`, `qtestframe_connect_http` (single sign-on), `qtestframe_remote_syslog` and `qtestframe_run_scripts`. See [Security](security.md#operating-system-layers).
- To serve port 443, either set `server.listen: ":443"` (the unit grants `CAP_NET_BIND_SERVICE`; with SELinux also `setsebool -P qtestframe_bind_http_port on`) or keep `:8443` and redirect 443 to it with nftables, which allows a unit without any capability.

## In a container (Docker or Podman)

`deploy/container/Containerfile` builds a distroless, static image that runs as UID 65532 with a read-only root file system; state is in the volume `/var/lib/qtestframe`. The image sets `QTF_DATABASE_DRIVER=sqlite`, `QTF_DATABASE_DSN=/var/lib/qtestframe/qtestframe.db`, `QTF_SERVER_LISTEN=:8443` and `QTF_LOG_FORMAT=json`, so it runs without a configuration file. Its `HEALTHCHECK` runs `qtestframe healthcheck` (no shell or curl in the image).

```sh
docker build -f deploy/container/Containerfile --build-arg VERSION=$(git describe --tags --always) -t qtestframe .
```

`deploy/container/compose.yaml` runs Qtestframe with PostgreSQL 16 on an internal network. Prepare the secrets and the certificate as described at the top of the file, then, from `deploy/container` (the seccomp profile path is relative):

```sh
docker compose up -d --build        # or: podman compose up -d --build
docker compose logs -f qtestframe
```

A one-shot `migrate` service runs before the server. The compose file applies `cap_drop: ALL`, `no-new-privileges`, the seccomp allow-list `deploy/seccomp/qtestframe-seccomp.json`, a memory and process limit, and sets `QTF_DATABASE_DRIVER=postgres` (the image's default is SQLite).

Points to remember with containers:

- **Environment variables lock system settings.** A setting set by a `QTF_*` variable cannot be changed under **Administration → System settings** (see [Runtime system settings](system-settings.md#precedence-and-locking)); in the image this applies to the log format and the listen address.
- **The key file** of the stored secrets lives on the data volume by default (`/var/lib/qtestframe/qtestframe-secrets.key`). Back it up separately, or give the key as `QTF_SECRETS_KEY` from your secret store.
- **Web tests** need a bigger image: `--build-arg WITH_PLAYWRIGHT=true` builds one based on Debian with Node.js, `@playwright/test` and Chromium (`PLAYWRIGHT_BROWSERS` adds browsers). Enable them with `QTF_RUNNER_ENABLED=true` and `QTF_RUNNER_WEB_ENABLED=true`. The default image ships no interpreters, so scripts cannot run in it.
- **Kubernetes**: use the same image with `runAsNonRoot`, `readOnlyRootFilesystem`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, the seccomp profile as a `Localhost` profile and HTTP probes on `/healthz` (liveness) and `/readyz` (readiness).

## Building from source

```sh
make build            # npm ci + web client build, then the static Go binary in bin/qtestframe
make go               # only the Go binary (needs the web client already built)
make test             # Go and web unit tests
```

The build embeds the version from `git describe` (`qtestframe version`). `make web` writes the web client to `internal/webui/dist`, which the Go build embeds.

### With HSM support (PKCS#11)

PKCS#11 modules are C libraries, so the HSM key provider needs cgo. The default binary refuses `secrets.provider: pkcs11` and says how to build one that supports it:

```sh
make go-pkcs11        # CGO_ENABLED=1 go build -tags pkcs11 → bin/qtestframe
```

The result is linked dynamically against libc and loads the vendor's module at run time. Test the provider against SoftHSM2 with `make test-pkcs11` (`apt-get install softhsm2`), and the TPM provider against a software TPM with `make test-tpm` (`apt-get install swtpm`). How to configure both is in [Security](security.md#key-providers).
