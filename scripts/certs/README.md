# Certificate tooling

Helper scripts to create and check the X.509 material a Qtestframe
deployment needs. All scripts:

* are bash (>= 3.2, so the stock macOS bash works), `set -euo pipefail`,
  shellcheck-clean and print usage with `--help`;
* work with OpenSSL 1.1/3.x (Linux, Homebrew) and LibreSSL (macOS default):
  extensions are always passed through generated config files, never via
  `-addext`; no `sed -i`, no GNU `date`;
* are **idempotent**: re-running keeps existing, still valid material and only
  re-issues when it is about to expire (`--renew-days`), the SANs changed, or
  `--force` is given;
* write private keys as unencrypted PKCS#8 PEM with mode `0600` (or `0640`
  and a group with `--group qtestframe`), certificates with `0644`, and
  replace files atomically.

| Script | Purpose |
|---|---|
| `gen-ca.sh` | local root CA (`ca.key`, `ca.crt`), ECDSA P-256 default, `--rsa 4096`, optional name constraints (`--permit`) |
| `gen-server-cert.sh` | TLS server certificate (serverAuth, SAN required) signed by the local CA or `--self-signed`; writes `server.key`, `server.crt`, `chain.pem`, `fullchain.pem` |
| `gen-client-cert.sh` | TLS client certificate (clientAuth) for syslog mTLS or PostgreSQL `cert` auth |
| `gen-saml-sp-cert.sh` | SAML SP signing/encryption key pair (RSA 3072, 5 years, self-signed) |
| `letsencrypt.sh` | public certificate via certbot or lego, installs to `/etc/qtestframe/tls`, renewal hook reloads the service |
| `verify-cert.sh` | show and verify a certificate/chain, key match, hostname, expiry; or check a live server (`--connect`) |
| `lib.sh` | shared functions (sourced) |

## Quick start: private CA

```sh
cd scripts/certs
./gen-ca.sh --dir ../../certs/ca --cn "ACME Internal CA" \
    --permit "DNS:example.com,DNS:.example.com,IP:10.0.0.0/255.0.0.0"

sudo ./gen-server-cert.sh --ca-dir ../../certs/ca \
    --cn qtf.example.com --san DNS:qtf.example.com,DNS:qtf,IP:10.0.0.5 \
    --out /etc/qtestframe/tls --group qtestframe

./verify-cert.sh --cert /etc/qtestframe/tls/fullchain.pem \
    --key /etc/qtestframe/tls/server.key --ca ../../certs/ca/ca.crt \
    --host qtf.example.com
sudo systemctl reload qtestframe      # SIGHUP: reloads the certificate
```

Distribute `ca.crt` to browsers / OS trust stores
(`update-ca-certificates`, `update-ca-trust`, macOS Keychain). Keep `ca.key`
offline when possible. `--permit` adds critical name constraints so the CA
can only ever issue for your own domains.

`server.crt` contains only the leaf certificate, which is sufficient when
clients trust the CA directly. Configure `fullchain.pem` as `tls_cert` if
you use an intermediate CA.

## Self-signed (development / lab)

```sh
./gen-server-cert.sh --self-signed --cn localhost --san DNS:localhost,IP:127.0.0.1 --out ./tls
```

## Client certificates

```sh
# rsyslog/syslog-ng mutual TLS
sudo ./gen-client-cert.sh --ca-dir ../../certs/ca --cn qtf01.example.com \
    --san DNS:qtf01.example.com --name syslog-client \
    --out /etc/qtestframe/tls --group qtestframe
# PostgreSQL: CN must equal the database role
sudo ./gen-client-cert.sh --ca-dir ../../certs/ca --cn qtestframe --name db-client \
    --out /etc/qtestframe/tls --group qtestframe
```

## SAML SP key pair

```sh
sudo ./gen-saml-sp-cert.sh --cn qtf.example.com --name saml-adfs \
    --out /etc/qtestframe/tls --group qtestframe
```

The IdP pins this certificate from the SP metadata
(`/api/v1/auth/saml/<name>/metadata`); rotating it with `--force` requires
re-importing the metadata at the IdP.

## Let's Encrypt

```sh
# HTTP-01 on port 80 (must be reachable from the internet)
sudo ./letsencrypt.sh --domain qtf.example.com --email ops@example.com
# DNS-01 with a certbot plugin (no inbound port needed, wildcards possible)
sudo ./letsencrypt.sh --domain qtf.example.com --method dns-cloudflare \
    --dns-credentials /root/.secrets/cloudflare.ini --email ops@example.com
# lego instead of certbot (installs a systemd renewal timer)
sudo ./letsencrypt.sh --client lego --domain qtf.example.com --email ops@example.com
# test first against the staging CA, or just print what would happen
sudo ./letsencrypt.sh --domain qtf.example.com --staging
./letsencrypt.sh --domain qtf.example.com --dry-run
```

Renewal:

* certbot: the distribution's `certbot.timer` (or `snap.certbot.renew.timer`
  / cron) runs `certbot renew`; the deploy hook
  `/etc/letsencrypt/renewal-hooks/deploy/qtestframe.sh` →
  `/usr/local/lib/qtestframe/qtestframe-cert-deploy.sh` copies the new
  certificate and runs `systemctl reload-or-restart qtestframe`.
  Test with `certbot renew --dry-run --cert-name qtestframe`.
* lego: `qtestframe-lego-renew.timer` (daily, renews 30 days before expiry).

The hook can also be used manually:
`qtestframe-cert-deploy.sh /path/fullchain.pem /path/privkey.pem`.

## Notes for macOS

* `/usr/bin/openssl` is LibreSSL. All generation paths are tested with
  OpenSSL 3; LibreSSL supports the same commands. If `verify-cert.sh --host`
  reports an unknown option on very old LibreSSL versions, use Homebrew
  OpenSSL: `OPENSSL=/opt/homebrew/opt/openssl@3/bin/openssl ./verify-cert.sh ...`.
* Every script honours `OPENSSL=/path/to/openssl`.
