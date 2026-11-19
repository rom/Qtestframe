# Operations guide

Installing, configuring, running and maintaining a Qtestframe server. The
security design behind these defaults is described in
[security.md](security.md).

- [1. Layout](#1-layout)
- [2. Installation](#2-installation)
- [3. Configuration basics](#3-configuration-basics)
- [4. Database: SQLite or PostgreSQL](#4-database-sqlite-or-postgresql)
- [5. TLS certificates](#5-tls-certificates)
- [6. Single sign-on: OIDC](#6-single-sign-on-oidc)
- [7. Single sign-on: SAML](#7-single-sign-on-saml)
- [8. Logging and central syslog](#8-logging-and-central-syslog)
- [9. Users, tenants and the CLI](#9-users-tenants-and-the-cli)
- [10. Script runner](#10-script-runner)
- [11. Backup and restore](#11-backup-and-restore)
- [12. Upgrade](#12-upgrade)
- [13. Monitoring](#13-monitoring)
- [14. Troubleshooting](#14-troubleshooting)
- [15. Development setup](#15-development-setup)

## 1. Layout

| Path | Owner / mode | Content |
|---|---|---|
| `/usr/bin/qtestframe` | root 0755 | the binary (server + CLI) |
| `/etc/qtestframe/config.yaml` | root:qtestframe 0640 | configuration ([example](../deploy/config.example.yaml)) |
| `/etc/qtestframe/tls/` | root:qtestframe 0750 | `server.crt` (0644), `server.key` (0640), SAML/client keys |
| `/etc/qtestframe/secrets/` | root:qtestframe 0750 | secret files referenced by `*_file` options |
| `/var/lib/qtestframe/` | qtestframe 0750 | `qtestframe.db` (SQLite), `runner/` work dir |
| `/run/qtestframe/` | qtestframe 0750 | runtime files |
| `/var/log/qtestframe/` | qtestframe 0750 | optional file logs (default: journald) |
| `/etc/systemd/system/qtestframe.service` | root 0644 | unit ([source](../deploy/systemd/qtestframe.service)) |

The service runs as the system user/group `qtestframe` and, with the
example configuration, listens on `:8443` (HTTPS); the built-in default
without a configuration file is `:8080`. Health endpoints: `/healthz`
(process alive, returns the version), `/readyz` (database reachable).

## 2. Installation

### From source on a systemd host

```sh
# build (needs Go and Node 22)
(cd web && npm ci && npm run build)              # -> internal/webui/dist
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(git describe --tags --always)" \
    -o bin/qtestframe ./cmd/qtestframe

# install (idempotent; keeps an existing config)
sudo scripts/install.sh --binary bin/qtestframe --self-signed-cert qtf.example.com
#   --selinux yes|no|auto   SELinux module (auto: when SELinux is enabled)
#   --apparmor              AppArmor profile (Debian/Ubuntu)
#   --sysctl                network/kernel hardening sysctls
```

`install.sh` installs the binary, `sysusers.d`/`tmpfiles.d` entries, the
unit, `config.yaml` (only if absent), docs, optionally the SELinux module /
AppArmor profile, then enables and starts the service. If the configured
certificate does not exist yet it does not start the service.

Remove with `sudo scripts/uninstall.sh` (keeps config and data) or
`--purge` (deletes everything, including the database).

### Firewall and kernel

```sh
sudo install -m 0644 deploy/nftables/qtestframe.nft /etc/nftables.d/
sudo nft -c -f /etc/nftables.d/qtestframe.nft && sudo nft -f /etc/nftables.d/qtestframe.nft
sudo install -m 0644 deploy/sysctl/99-qtestframe.conf /etc/sysctl.d/ && sudo sysctl --system
```

Review the sets (`admin_v4`, `out_tcp_ports`, ...) first. Let's Encrypt
HTTP-01 needs inbound TCP 80 — add it to `qtf_ports` or use DNS-01.

### Container (docker / podman compose)

```sh
cd deploy/container
# prepare secrets/ and tls/ as described at the top of compose.yaml
docker compose up -d --build        # or: podman compose up -d --build
```

The image (`deploy/container/Containerfile`) is distroless/static, runs as
UID 65532 with a read-only root file system; state lives in the
`/var/lib/qtestframe` volume. Compose applies `cap_drop: ALL`,
`no-new-privileges`, the seccomp profile `deploy/seccomp/qtestframe-seccomp.json`
and runs `qtestframe migrate` as a one-shot service before `serve`.
Run compose from `deploy/container` (the seccomp path is relative).
The image's `HEALTHCHECK` runs `qtestframe healthcheck`, which requests
`/healthz` of the local listener (derived from `server.listen`, HTTPS when
TLS is configured, certificate not verified) and needs no shell or curl.
Kubernetes: use the same image with `runAsNonRoot`, `readOnlyRootFilesystem`,
`allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`,
`seccompProfile: {type: Localhost, localhostProfile: qtestframe-seccomp.json}`
and HTTP probes on `/healthz` and `/readyz` (scheme HTTPS).

### Binding port 443

Either set `server.listen: ":443"` (the unit grants `CAP_NET_BIND_SERVICE`;
on SELinux also `setsebool -P qtestframe_bind_http_port on`), or keep
`:8443` and redirect with nftables (`tcp dport 443 redirect to :8443`, see
the commented chain in `qtestframe.nft`), which allows the strict drop-in
without any capability (security.md §10).

## 3. Configuration basics

Edit `/etc/qtestframe/config.yaml` (fully commented example:
`deploy/config.example.yaml`), then `sudo systemctl restart qtestframe`.
`systemctl reload qtestframe` (SIGHUP) only re-reads the TLS certificate/key.

The file is YAML and is decoded strictly: unknown keys (typos such as
`listn:`) and multiple YAML documents are rejected at start-up. Validate a
file without starting the server with
`qtestframe check-config --config /etc/qtestframe/config.yaml`.

Precedence: **command line flags > `QTF_*` environment variables > YAML file
> built-in defaults.**

The listen port can be given on the command line of `serve` (and of
`check-config`/`healthcheck`): `--port N` / `-p N` replaces only the port of
`server.listen` (keeping the host), `--listen HOST:PORT` replaces the whole
address:

```sh
qtestframe serve --config /etc/qtestframe/config.yaml --port 9443
qtestframe serve --listen 127.0.0.1:8080
```

Environment overrides (e.g. in a drop-in `Environment=` line or a container):
`QTF_CONFIG`, `QTF_DATABASE_DRIVER`, `QTF_DATABASE_DSN`, `QTF_SERVER_LISTEN`,
`QTF_SERVER_PORT` (port only),
`QTF_SERVER_BASE_URL`, `QTF_LOG_LEVEL`, `QTF_LOG_FORMAT`, `QTF_COOKIE_SECURE`,
`QTF_BOOTSTRAP_ADMIN_EMAIL`, `QTF_BOOTSTRAP_ADMIN_PASSWORD`,
`QTF_OIDC_<NAME>_CLIENT_SECRET`.

Local changes to the unit go into a drop-in, never into the unit file:

```sh
sudo systemctl edit qtestframe
```
```ini
[Service]
LoadCredential=oidc-corp:/etc/qtestframe/secrets/oidc-corp
LoadCredential=db-dsn:/etc/qtestframe/secrets/db-dsn
MemoryMax=4G
```

Secret files referenced with a **relative** path are looked up in
`$CREDENTIALS_DIRECTORY` (`client_secret_file: oidc-corp`). This applies to
`database.dsn_file`, `auth.oidc[].client_secret_file` and
`bootstrap.admin_password_file`; key and certificate paths (`tls_key`,
`sp_key_file`, syslog `key_file`, ...) are used as given.

`qtestframe check-config [--config FILE]` validates a configuration
(including secret files) without touching the database.

Always set `server.base_url` to the URL users type (it is used for SSO
redirect URIs, SAML entity/ACS URLs and the CSRF origin check). Behind a
reverse proxy also set `server.trusted_proxies`.

## 4. Database: SQLite or PostgreSQL

| | SQLite (default) | PostgreSQL |
|---|---|---|
| Setup | none — file `/var/lib/qtestframe/qtestframe.db` is created on first start | separate server |
| Scale | one instance (single connection, writes serialised), tens of concurrent users, comfortably up to a few GB | many users, several app instances behind a load balancer (see below) |
| Backup | file copy via SQLite online backup | `pg_dump` / PITR |
| HA | no | yes (replication) |

### SQLite

```yaml
database:
  driver: sqlite
  dsn: "/var/lib/qtestframe/qtestframe.db"
```

The database runs in WAL mode (`qtestframe.db-wal`, `-shm` next to it) with
foreign keys on and a 5 s busy timeout, through a single connection
(`max_open_conns` is ignored for SQLite). Keep it on a local file system
(not NFS) and never point two server processes at the same file.

### PostgreSQL

Create a dedicated role and database (PostgreSQL ≥ 13, 16 recommended):

```sql
CREATE ROLE qtestframe LOGIN PASSWORD 'change-me';   -- or no password + cert auth
CREATE DATABASE qtestframe OWNER qtestframe ENCODING 'UTF8' TEMPLATE template0;
REVOKE ALL ON DATABASE qtestframe FROM PUBLIC;
\c qtestframe
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT ALL ON SCHEMA public TO qtestframe;
```

`pg_hba.conf` (TLS only):

```
# password (SCRAM) over TLS
hostssl  qtestframe  qtestframe  10.0.0.5/32  scram-sha-256
# or: client certificate, CN must equal the role (gen-client-cert.sh --cn qtestframe)
hostssl  qtestframe  qtestframe  10.0.0.5/32  cert  clientcert=verify-full
```

Configuration — keep the DSN (with password) in a secret file:

```sh
sudo install -m 0640 -o root -g qtestframe /dev/stdin /etc/qtestframe/secrets/db-dsn <<'EOF'
postgres://qtestframe:change-me@db.example.com:5432/qtestframe?sslmode=verify-full&sslrootcert=/etc/qtestframe/tls/db-ca.crt
EOF
```
```yaml
database:
  driver: postgres
  dsn_file: /etc/qtestframe/secrets/db-dsn     # or "db-dsn" with LoadCredential=
  max_open_conns: 20
```

Client certificate variant:
`...?sslmode=verify-full&sslrootcert=/etc/qtestframe/tls/db-ca.crt&sslcert=/etc/qtestframe/tls/db-client.crt&sslkey=/etc/qtestframe/tls/db-client.key`
(key `0640 root:qtestframe`).

Several instances against one PostgreSQL database: sessions, tokens and
data are shared, but each process keeps its own SSO login-state signing key
and its own login rate limiter. Configure session affinity (sticky
sessions) at the load balancer at least for `/api/v1/auth/`, so that the
OIDC/SAML redirect and callback reach the same instance; a restarted
instance invalidates logins in progress. Set `server.trusted_proxies` to
the load balancer addresses and let the load balancer send HSTS when it
terminates TLS.

Local PostgreSQL on the same host: uncomment `After=postgresql.service` in
a drop-in; for the unix socket (`host=/run/postgresql`) add
`/run/postgresql` to `hardening.read_write_paths`.

SELinux: `sudo setsebool -P qtestframe_connect_postgresql on`.

Migrating SQLite → PostgreSQL: export each project (`/projects/{pid}/export`)
is for reporting only; for a full migration stop the service, run
`qtestframe migrate` against the empty PostgreSQL database, and copy the
tables with a tool such as `pgloader` (schema created by `migrate`, data
only: `pgloader --with "data only" sqlite:///var/lib/qtestframe/qtestframe.db pgsql://...`).

## 5. TLS certificates

Scripts in `scripts/certs/` (see its [README](../scripts/certs/README.md)):

```sh
# Let's Encrypt (public DNS name, port 80 reachable, or --method dns-<provider>)
sudo scripts/certs/letsencrypt.sh --domain qtf.example.com --email ops@example.com

# internal CA
scripts/certs/gen-ca.sh --dir /root/qtf-ca --cn "ACME Internal CA"
sudo scripts/certs/gen-server-cert.sh --ca-dir /root/qtf-ca --cn qtf.example.com \
    --san DNS:qtf.example.com,IP:10.0.0.5 --out /etc/qtestframe/tls --group qtestframe

# check
scripts/certs/verify-cert.sh --cert /etc/qtestframe/tls/server.crt \
    --key /etc/qtestframe/tls/server.key --host qtf.example.com
sudo systemctl reload qtestframe     # SIGHUP: load the new certificate
```

Certificates from another CA: put the full chain (leaf first) into
`/etc/qtestframe/tls/server.crt` and the key into `server.key`
(`chown root:qtestframe server.key; chmod 0640 server.key`), then reload.
On SELinux run `restorecon -RF /etc/qtestframe/tls`.

Renewal with Let's Encrypt is automatic (certbot timer or the installed
`qtestframe-lego-renew.timer`); the deploy hook copies the files and runs
`systemctl reload-or-restart qtestframe`.

## 6. Single sign-on: OIDC

General steps: register a confidential web client at the IdP with redirect
URI `https://qtf.example.com/api/v1/auth/oidc/<name>/callback`, grant type
*authorization code* (PKCE is used automatically), scopes
`openid email profile` (+ `groups` if offered). Put the client secret in a
file:

```sh
sudo install -m 0640 -o root -g qtestframe /dev/stdin /etc/qtestframe/secrets/oidc-corp <<<'the-secret'
```

The server needs outbound HTTPS to the issuer (SELinux:
`setsebool -P qtestframe_connect_http on`; IdP on port 8443: also add that
port to `http_port_t` or allow it in a local module).

### Keycloak

1. Realm `x` → Clients → Create: *OpenID Connect*, Client ID `qtf`,
   *Client authentication* on, *Standard flow* only.
2. Valid redirect URIs: `https://qtf.example.com/api/v1/auth/oidc/corp/callback`;
   Web origins: `https://qtf.example.com`; Advanced → PKCE method `S256`.
3. Client scopes → `qtf-dedicated` → Add mapper → *Group Membership*: token
   claim name `groups`, *Full group path* off, add to ID token + userinfo.
4. Credentials tab → copy the client secret.

```yaml
auth:
  oidc:
    - name: corp
      display_name: "Keycloak"
      issuer: "https://sso.example.com/realms/x"
      client_id: qtf
      client_secret_file: /etc/qtestframe/secrets/oidc-corp
      scopes: [openid, email, profile]
      auto_provision: true
      default_tenant: acme
      default_role: viewer
      email_claim: email
      name_claim: name
      groups_claim: groups
      role_mapping: {"qtf-admins": admin, "qtf-managers": manager, "qtf-testers": tester}
```

### Microsoft Entra ID (Azure AD)

1. App registrations → New: *Web* redirect URI
   `https://qtf.example.com/api/v1/auth/oidc/entra/callback`.
2. Certificates & secrets → new client secret.
3. Token configuration → *Add groups claim* (Security groups; emitted as
   object IDs) and optional claim `email` for the ID token.
4. API permissions: `openid`, `email`, `profile` (Microsoft Graph, delegated).

```yaml
    - name: entra
      display_name: "Microsoft"
      issuer: "https://login.microsoftonline.com/<tenant-id>/v2.0"
      client_id: "<application-client-id>"
      client_secret_file: entra          # with LoadCredential=entra:/etc/qtestframe/secrets/entra
      scopes: [openid, email, profile]
      email_claim: email                 # or preferred_username if users have no mail attribute
      name_claim: name
      groups_claim: groups
      default_tenant: acme
      default_role: viewer
      auto_provision: true
      role_mapping:
        "1b2c3d4e-0000-0000-0000-000000000001": admin     # group object IDs
        "1b2c3d4e-0000-0000-0000-000000000002": tester
```

Users in more than 200 groups receive an overage claim instead of `groups`;
use *Groups assigned to the application* to keep the list short.

### Okta

1. Applications → Create App Integration → *OIDC*, *Web Application*;
   sign-in redirect URI `https://qtf.example.com/api/v1/auth/oidc/okta/callback`;
   grant type *Authorization Code*; require PKCE.
2. Security → API → Authorization server `default` → Claims → Add claim
   `groups` (ID token, always, value type *Groups*, filter e.g.
   *starts with* `qtf-`).

```yaml
    - name: okta
      display_name: "Okta"
      issuer: "https://example.okta.com/oauth2/default"
      client_id: "0oa..."
      client_secret_file: okta
      scopes: [openid, email, profile, groups]
      groups_claim: groups
      default_tenant: acme
      default_role: viewer
      auto_provision: true
      role_mapping: {"qtf-admins": admin, "qtf-testers": tester}
```

## 7. Single sign-on: SAML

Create the SP key pair and point the IdP at the SP metadata:

```sh
sudo scripts/certs/gen-saml-sp-cert.sh --cn qtf.example.com --name saml-adfs \
    --out /etc/qtestframe/tls --group qtestframe
# after restart, the metadata is at:
curl -s https://qtf.example.com/api/v1/auth/saml/adfs/metadata
```

### ADFS

1. AD FS Management → Relying Party Trusts → Add → *Claims aware* → import
   data from `https://qtf.example.com/api/v1/auth/saml/adfs/metadata`.
2. Access control policy as required.
3. Claim issuance policy:
   * *Send LDAP Attributes as Claims*: `E-Mail-Addresses` → *E-Mail Address*,
     `Display-Name` → *Name*, `Token-Groups - Unqualified Names` → *Group*.
   * *Transform*: *E-Mail Address* → *Name ID* (format *Email*).
4. Ensure the relying party signs the assertion (default) — PowerShell:
   `Set-AdfsRelyingPartyTrust -TargetName qtestframe -SamlResponseSignature MessageAndAssertion`.

```yaml
auth:
  saml:
    - name: adfs
      display_name: "Corporate login (ADFS)"
      idp_metadata_url: "https://adfs.example.com/FederationMetadata/2007-06/FederationMetadata.xml"
      entity_id: "https://qtf.example.com/api/v1/auth/saml/adfs/metadata"
      sp_cert_file: /etc/qtestframe/tls/saml-adfs.crt
      sp_key_file: /etc/qtestframe/tls/saml-adfs.key
      email_attr: "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress"
      name_attr: "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name"
      groups_attr: "http://schemas.xmlsoap.org/claims/Group"
      auto_provision: true
      default_tenant: acme
      default_role: viewer
      role_mapping: {"QTF-Admins": admin, "QTF-Testers": tester}
```

### Keycloak (SAML)

1. Clients → Create → *SAML*, Client ID =
   `https://qtf.example.com/api/v1/auth/saml/kc/metadata` (the entity ID), or
   import the SP metadata XML.
2. Valid redirect URIs / Master SAML processing URL:
   `https://qtf.example.com/api/v1/auth/saml/kc/acs`.
3. Settings: *Sign assertions* on, *Name ID format* email. Keys: turn
   *Client signature required* **off** — Qtestframe does not sign
   AuthnRequests. Import `saml-kc.crt` only for assertion encryption.
4. Client scopes → dedicated → mappers: *User Property* `email` → SAML
   attribute `email`; *User Property* `firstName`/`lastName` or *User
   Attribute* → `name`; *Group list* → attribute `groups`, full path off.

```yaml
    - name: kc
      display_name: "Keycloak (SAML)"
      idp_metadata_url: "https://sso.example.com/realms/x/protocol/saml/descriptor"
      entity_id: "https://qtf.example.com/api/v1/auth/saml/kc/metadata"
      sp_cert_file: /etc/qtestframe/tls/saml-kc.crt
      sp_key_file: /etc/qtestframe/tls/saml-kc.key
      email_attr: email
      name_attr: name
      groups_attr: groups
      auto_provision: true
      default_tenant: acme
      default_role: viewer
      role_mapping: {"qtf-admins": admin}
```

Metadata fetched by URL needs `qtestframe_connect_http` on SELinux; with
`idp_metadata_file` (stored below `/etc/qtestframe`) no outbound access is
needed.

## 8. Logging and central syslog

By default logs go to stdout → journald: `journalctl -u qtestframe -f`
(`-o json` for structured fields). Set `logging.format: json` for machine
processing.

### Sending to a central server over TLS

```sh
# client certificate for mutual TLS (optional)
sudo scripts/certs/gen-client-cert.sh --ca-dir /root/qtf-ca --cn qtf01.example.com \
    --san DNS:qtf01.example.com --name syslog-client --out /etc/qtestframe/tls --group qtestframe
```
```yaml
logging:
  level: info
  format: json
  syslog:
    enabled: true
    network: tls
    address: "logs.example.com:6514"
    tag: qtestframe
    facility: local0
    format: rfc5424
    ca_file: /etc/qtestframe/tls/ca.crt
    cert_file: /etc/qtestframe/tls/syslog-client.crt
    key_file: /etc/qtestframe/tls/syslog-client.key
```

SELinux: `setsebool -P qtestframe_remote_syslog on`.

### rsyslog receiver (RFC 5425)

`/etc/rsyslog.d/10-qtestframe-tls.conf` (package `rsyslog-gnutls` or
`rsyslog-openssl`):

```
global(
  DefaultNetstreamDriver="gtls"
  DefaultNetstreamDriverCAFile="/etc/rsyslog.d/tls/ca.crt"
  DefaultNetstreamDriverCertFile="/etc/rsyslog.d/tls/logs.example.com.crt"
  DefaultNetstreamDriverKeyFile="/etc/rsyslog.d/tls/logs.example.com.key"
)
module(load="imtcp"
  StreamDriver.Name="gtls"
  StreamDriver.Mode="1"                 # TLS only
  StreamDriver.AuthMode="x509/name"     # verify client certificates
  PermittedPeer=["qtf01.example.com", "*.qtf.example.com"])
input(type="imtcp" port="6514" ruleset="remote_qtf")

template(name="QtfFile" type="string" string="/var/log/remote/qtestframe/%HOSTNAME%.log")
ruleset(name="remote_qtf") {
  if ($app-name == "qtestframe") then {
    action(type="omfile" dynaFile="QtfFile" template="RSYSLOG_SyslogProtocol23Format")
    stop
  }
}
```

Without client certificates use `StreamDriver.AuthMode="anon"`.

### syslog-ng receiver

`/etc/syslog-ng/conf.d/qtestframe.conf`:

```
source s_qtf_tls {
  network(
    ip("0.0.0.0") port(6514)
    transport("tls")
    flags(syslog-protocol)
    tls(
      key-file("/etc/syslog-ng/tls/logs.example.com.key")
      cert-file("/etc/syslog-ng/tls/logs.example.com.crt")
      ca-file("/etc/syslog-ng/tls/ca.crt")
      peer-verify(required-trusted)      # mutual TLS; optional-untrusted to disable
    )
  );
};
filter f_qtf { program("qtestframe"); };
destination d_qtf { file("/var/log/remote/qtestframe/${HOST}.log" create-dirs(yes)); };
log { source(s_qtf_tls); filter(f_qtf); destination(d_qtf); };
```

JSON payloads can be parsed into fields with rsyslog `mmjsonparse` or
syslog-ng `json-parser()` for forwarding to Elasticsearch/Loki/Splunk.

## 9. Users, tenants and the CLI

The CLI (`internal/app/app.go`) reads the same configuration and talks to
the database directly. Every command takes `--config FILE`; without it,
`$QTF_CONFIG` is used (`config.Load`), then built-in defaults plus `QTF_*`
environment overrides.

| Command | Purpose |
|---|---|
| `serve` | run the HTTP(S) server (migrates and bootstraps first); `--port N`/`-p N`, `--listen HOST:PORT` |
| `migrate` | apply pending database migrations, print how many were applied |
| `user create --email E [--name N] [--password-stdin] [--super-admin]` | create a user; the password is read as one line from stdin (without `--password-stdin` the user has no password and can only use SSO) |
| `tenant create --slug S --name N` | create a tenant (slug: 2–63 characters `a-z0-9-`) |
| `member add --tenant SLUG --email E --role admin\|manager\|tester\|viewer` | add a user to a tenant or change their role (default role `viewer`) |
| `seed-demo [--tenant SLUG]` | create the demo project (`DEMO`, `DEMO2`, ...) in the tenant, default `bootstrap.tenant_slug`; the tenant is created if missing |
| `check-config` | load and validate the configuration, print `configuration OK` |
| `healthcheck [--url URL]` | GET `/healthz` of the local server (URL derived from `server.listen`), exit 0 on 200 |
| `version` | print the version |
| `sandbox-exec ...` | internal: entry point of the script runner sandbox, started by the server itself |

`user create`, `tenant create`, `member add` and `seed-demo` migrate the
database before they run. Run the CLI as the service user so file
ownership stays correct:

```sh
alias qtf='sudo -u qtestframe env QTF_CONFIG=/etc/qtestframe/config.yaml /usr/bin/qtestframe'
qtf version
qtf migrate
qtf tenant create --slug acme --name "ACME Corp"
read -rs PW && printf '%s' "$PW" | qtf user create --email alice@example.com --password-stdin --super-admin
qtf member add --tenant acme --email bob@example.com --role tester
qtf seed-demo      # demo project — never on production
qtf check-config
```

Everything else
(projects, members, roles, API tokens) is managed in the web UI or via the
REST API (`docs/api.md`).

## 10. Script runner

Disabled by default. To enable:

```yaml
runner:
  enabled: true
  timeout: 60s                      # keep below server.write_timeout (default 120s)
  work_dir: /var/lib/qtestframe/runner
  max_output_bytes: 1048576
  interpreters: {sh: /bin/sh, python: /usr/bin/python3}   # only what you need
  sandbox: true                     # run through `qtestframe sandbox-exec`
  deny_network: false               # true: scripts cannot create sockets
```

Without `work_dir` the runner uses `$TMPDIR/qtestframe-runner` (the private
`/tmp` under systemd). The server creates the directory with mode 0700 and
adds it to its Landlock read-write paths.

SELinux: `setsebool -P qtestframe_run_scripts on`. AppArmor: the profile
already contains the `runner` child profile. Only managers and admins
(`script.execute`) can run scripts. JavaScript requires relaxing
`MemoryDenyWriteExecute` (see security.md §9). Read the security
considerations in security.md §9 before enabling.

## 11. Backup and restore

What to back up: the database, `/etc/qtestframe` (config, keys, secrets —
encrypt the backup!). The runner directory is scratch space.

### SQLite

Online, consistent copy (WAL-safe) with the `sqlite3` CLI:

```sh
sudo -u qtestframe sqlite3 /var/lib/qtestframe/qtestframe.db \
    ".backup '/var/lib/qtestframe/backup-$(date +%F).db'"
# or: VACUUM INTO (compact copy)
sudo -u qtestframe sqlite3 /var/lib/qtestframe/qtestframe.db \
    "VACUUM INTO '/var/lib/qtestframe/backup-$(date +%F).db'"
```

Without `sqlite3`: `systemctl stop qtestframe`, copy `qtestframe.db*`, start.
Never copy the `.db` file alone while the service is running.

Restore:

```sh
sudo systemctl stop qtestframe
sudo -u qtestframe install -m 0600 backup-2026-01-31.db /var/lib/qtestframe/qtestframe.db
sudo rm -f /var/lib/qtestframe/qtestframe.db-wal /var/lib/qtestframe/qtestframe.db-shm
sudo restorecon -RF /var/lib/qtestframe 2>/dev/null || true
sudo systemctl start qtestframe      # ExecStartPre migrates an older backup forward
```

### PostgreSQL

```sh
pg_dump -Fc -h db.example.com -U qtestframe -d qtestframe -f qtestframe-$(date +%F).dump
# restore into an empty database
pg_restore --clean --if-exists --no-owner -h db.example.com -U qtestframe -d qtestframe qtestframe-2026-01-31.dump
```

For point-in-time recovery use WAL archiving (pgBackRest, Barman).

### Example systemd timer (SQLite)

```ini
# /etc/systemd/system/qtestframe-backup.service
[Service]
Type=oneshot
User=qtestframe
ExecStart=/bin/sh -c 'sqlite3 /var/lib/qtestframe/qtestframe.db ".backup /var/lib/qtestframe/backup.db"'
# /etc/systemd/system/qtestframe-backup.timer
[Timer]
OnCalendar=daily
Persistent=true
[Install]
WantedBy=timers.target
```

Copy the result off-host (restic, borg) and test restores regularly.

## 12. Upgrade

1. Read the release notes.
2. Back up (section 11).
3. Install the new binary: `sudo scripts/install.sh --binary bin/qtestframe`
   (keeps config; re-installs unit/policies) — or replace
   `/usr/bin/qtestframe` and `restorecon /usr/bin/qtestframe`.
4. `sudo systemctl restart qtestframe` — `ExecStartPre` runs
   `qtestframe migrate` before the new version serves traffic.
5. Check `journalctl -u qtestframe -b`, `curl -fsk https://localhost:8443/readyz`.
6. Compare `deploy/config.example.yaml` with your config for new options.

Migrations are forward-only. Roll back by restoring the backup taken in
step 2 together with the previous binary.

Containers: `docker compose pull`/`build`, `docker compose up -d` (the
`migrate` service runs first).

## 13. Monitoring

| Signal | How |
|---|---|
| Liveness | `GET /healthz` → 200 |
| Readiness (DB) | `GET /readyz` → 200, 503 when the database is unreachable |
| Service state | `systemctl is-active qtestframe`, `Restart=on-failure` restarts crashes |
| Logs / errors | journald or central syslog; alert on `level=ERROR` and repeated `login failed` warnings (failed logins are log records, not audit entries) |
| Certificate expiry | `scripts/certs/verify-cert.sh -q --connect qtf.example.com:8443 --warn-days 21` (exit 2 = expiring) |
| Resources | `systemctl status qtestframe` (memory/tasks vs. `MemoryMax`/`TasksMax`), disk space of `/var/lib/qtestframe` |
| Backups | age of the newest backup file |

Example blackbox probes (Prometheus `blackbox_exporter`, module `http_2xx`
with `tls_config.ca_file` for internal CAs) against `/healthz` and
`/readyz`; a simple cron alternative:

```sh
curl -fsS --max-time 5 https://qtf.example.com:8443/readyz >/dev/null || echo "qtestframe not ready" | mail -s ALERT ops@example.com
```

## 14. Troubleshooting

| Symptom | Check |
|---|---|
| service does not start | `journalctl -u qtestframe -b -e`; `sudo -u qtestframe qtestframe check-config --config /etc/qtestframe/config.yaml` shows config errors |
| `permission denied` on config/keys | `ls -l /etc/qtestframe /etc/qtestframe/tls` (group `qtestframe`, 0640), SELinux labels `ls -Z`, `restorecon -RF /etc/qtestframe` |
| `Operation not permitted` in logs | a syscall blocked by `SystemCallFilter`: `journalctl -k \| grep -i audit` / `ausearch -m SECCOMP`; test with a drop-in `SystemCallFilter=` (empty) to confirm, then report |
| SELinux denials | `ausearch -m AVC -ts recent -c qtestframe`; missing boolean? `getsebool -a \| grep qtestframe` |
| AppArmor denials | `journalctl -k \| grep -i apparmor="DENIED"`; `aa-complain /usr/bin/qtestframe` temporarily |
| bind on :443 fails | `CAP_NET_BIND_SERVICE` removed by a drop-in? SELinux `qtestframe_bind_http_port`? |
| SSO login loops / `state mismatch` | `base_url` must match the URL in the browser; cookies blocked by `cookie_secure` on plain HTTP; clock skew (NTP) |
| OIDC discovery fails | outbound HTTPS blocked (nftables egress, `IPAddressDeny=`, SELinux `qtestframe_connect_http`) |
| PostgreSQL TLS errors | `sslrootcert` path readable by `qtestframe`; `sslmode=verify-full` requires the server name in the certificate SAN |
| `landlock: …` in the `skipped` list of the `hardening` start-up log line | kernel < 5.13 or `lsm=` without landlock: protection is skipped; add `landlock` to the `lsm=` boot parameter |
| `permission denied` for a file after start-up | Landlock: the path is not in the rule set; add it to `hardening.read_only_paths` / `read_write_paths` |
| `429 rate_limited` on login | 10 attempts per 5 minutes per client IP and e-mail; wait, or check `trusted_proxies` if all users appear with the proxy's IP |

## 15. Development setup

Linux or macOS with Go and Node 22:

```sh
scripts/dev.sh                    # SQLite in ./data, backend :8080, Vite :5173
scripts/dev.sh --postgres --seed  # throw-away PostgreSQL (podman/docker) + demo data
scripts/dev-postgres.sh start|stop|rm|psql|env
```

Open http://localhost:5173 (Vite proxies `/api` to the backend). The dev
config (`data/dev-config.yaml`) uses plain HTTP, insecure cookies and turns
Landlock off — never use it in production.
