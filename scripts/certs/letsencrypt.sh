#!/usr/bin/env bash
# Obtain a publicly trusted certificate from Let's Encrypt (ACME) with
# certbot or lego, install it for Qtestframe and set up automatic renewal
# that re-installs the certificate and reloads the service.
#
# Linux + systemd hosts; must run as root.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source-path=SCRIPTDIR source=lib.sh
. "$SCRIPT_DIR/lib.sh"

DOMAINS=""
EMAIL=""
STAGING=0
METHOD=standalone
WEBROOT=""
CLIENT=""
DEST=/etc/qtestframe/tls
GROUP=qtestframe
SERVICE=qtestframe
KEY_TYPE=ecdsa
CERT_NAME=qtestframe
DNS_CREDENTIALS=""
LEGO_PATH=/var/lib/lego
HOOK_DIR=/usr/local/lib/qtestframe
DRY_RUN=0

usage() {
	cat <<EOF
Usage: $QTF_PROG --domain NAME [--domain NAME2 ...] [options]

Options:
  --domain D            domain name (repeat or comma separate for SANs)
  --email E             ACME account e-mail (expiry notices); recommended,
                        required for lego
  --staging             use the Let's Encrypt staging CA (for testing)
  --method M            standalone (default; binds :80 during issuance and
                        renewals), webroot (needs --webroot), or
                        dns-<provider> (e.g. dns-cloudflare, dns-route53)
  --webroot DIR         document root served on http://<domain>/ by another
                        web server (method webroot)
  --dns-credentials F   certbot: credentials INI for the dns plugin
                        lego:    environment file with the provider variables
                                 (e.g. CLOUDFLARE_DNS_API_TOKEN=...)
  --client C            certbot | lego (default: whichever is installed,
                        certbot preferred)
  --key-type T          ecdsa (default, P-256) | rsa
  --dest DIR            install directory (default $DEST)
  --group G             group allowed to read the key (default $GROUP)
  --service S           systemd unit to reload (default $SERVICE)
  --cert-name N         certbot lineage name (default $CERT_NAME)
  --dry-run             print the commands instead of running them
  -h, --help            show this help

What it does:
  1. obtains the certificate (certbot certonly / lego run)
  2. installs a deploy hook ($HOOK_DIR/qtestframe-cert-deploy.sh) which
     copies fullchain + key to $DEST/server.crt / server.key
     (0644 / 0640 root:$GROUP), restores SELinux labels and runs
     "systemctl reload-or-restart $SERVICE" (SIGHUP = certificate reload)
  3. certbot: registers the hook in /etc/letsencrypt/renewal-hooks/deploy/
     and relies on the packaged certbot.timer / cron job;
     lego: installs qtestframe-lego-renew.{service,timer} (daily)

Ports: HTTP-01 (standalone/webroot) needs inbound TCP 80 reachable from the
internet during issuance AND renewals. Use a dns-* method otherwise.

Examples:
  $QTF_PROG --domain qtf.example.com --email ops@example.com
  $QTF_PROG --domain qtf.example.com --method dns-cloudflare \\
      --dns-credentials /root/.secrets/cloudflare.ini --email ops@example.com
  $QTF_PROG --client lego --domain qtf.example.com --email ops@example.com \\
      --method dns-cloudflare --dns-credentials /etc/qtestframe/lego.env
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--domain) DOMAINS="${DOMAINS:+$DOMAINS,}${2:?--domain needs a value}"; shift ;;
	--email) EMAIL=${2:?--email needs a value}; shift ;;
	--staging) STAGING=1 ;;
	--method) METHOD=${2:?--method needs a value}; shift ;;
	--webroot) WEBROOT=${2:?--webroot needs a value}; shift ;;
	--dns-credentials) DNS_CREDENTIALS=${2:?--dns-credentials needs a value}; shift ;;
	--client) CLIENT=${2:?--client needs a value}; shift ;;
	--key-type) KEY_TYPE=${2:?--key-type needs a value}; shift ;;
	--dest) DEST=${2:?--dest needs a value}; shift ;;
	--group) GROUP=${2:?--group needs a value}; shift ;;
	--service) SERVICE=${2:?--service needs a value}; shift ;;
	--cert-name) CERT_NAME=${2:?--cert-name needs a value}; shift ;;
	--dry-run) DRY_RUN=1 ;;
	-h | --help) usage; exit 0 ;;
	*) usage >&2; die "unknown option: $1" ;;
	esac
	shift
done

run() {
	if [ "$DRY_RUN" -eq 1 ]; then
		printf '+'
		printf ' %q' "$@"
		printf '\n'
	else
		"$@"
	fi
}

# Write stdin to FILE with MODE (honours --dry-run).
write_file() { # file mode
	if [ "$DRY_RUN" -eq 1 ]; then
		printf '+ write %s (mode %s):\n' "$1" "$2"
		sed 's/^/|   /'
		return 0
	fi
	local tmp="$1.tmp.$$"
	cat >"$tmp"
	chmod "$2" "$tmp"
	mv -f "$tmp" "$1"
}

[ -n "$DOMAINS" ] || { usage >&2; die "--domain is required"; }
DOMAINS=$(printf '%s' "$DOMAINS" | tr -d ' ')
IFS=',' read -r -a DOMAIN_LIST <<<"$DOMAINS"
for d in "${DOMAIN_LIST[@]}"; do
	case "$d" in
	'' | *[!A-Za-z0-9.*-]*) die "invalid domain '$d'" ;;
	esac
done
PRIMARY=${DOMAIN_LIST[0]}
check_dn_value "$CERT_NAME" "--cert-name"
[ -z "$EMAIL" ] || check_dn_value "$EMAIL" "--email"

case "$KEY_TYPE" in ecdsa | rsa) ;; *) die "--key-type must be ecdsa or rsa" ;; esac
case "$METHOD" in
standalone) ;;
webroot) [ -n "$WEBROOT" ] || die "--method webroot requires --webroot DIR" ;;
dns-?*) ;;
*) die "unknown --method '$METHOD' (standalone, webroot, dns-<provider>)" ;;
esac
case "$PRIMARY" in
\**)
	case "$METHOD" in dns-*) ;; *) die "wildcard domains require a dns-* method" ;; esac
	;;
esac

if [ -z "$CLIENT" ]; then
	if command -v certbot >/dev/null 2>&1; then
		CLIENT=certbot
	elif command -v lego >/dev/null 2>&1; then
		CLIENT=lego
	elif [ "$DRY_RUN" -eq 1 ]; then
		CLIENT=certbot
	else
		die "neither certbot nor lego found. Install one: apt/dnf install certbot | https://go-acme.github.io/lego/installation/"
	fi
fi
case "$CLIENT" in certbot | lego) ;; *) die "--client must be certbot or lego" ;; esac
if [ "$DRY_RUN" -eq 0 ]; then
	command -v "$CLIENT" >/dev/null 2>&1 || die "$CLIENT is not installed"
	[ "$(id -u)" -eq 0 ] || die "must run as root (writes $DEST, /etc/letsencrypt, systemd units)"
	getent group "$GROUP" >/dev/null 2>&1 || die "group $GROUP does not exist (install Qtestframe first or pass --group)"
fi

#
# 1. Deploy hook (shared by certbot and lego).
#
HOOK="$HOOK_DIR/qtestframe-cert-deploy.sh"
run mkdir -p "$HOOK_DIR" "$DEST"
write_file "$HOOK" 0755 <<EOF
#!/bin/sh
# Installed by Qtestframe scripts/certs/letsencrypt.sh — copies a renewed
# certificate into $DEST and reloads $SERVICE.
#
# certbot: called with RENEWED_LINEAGE=/etc/letsencrypt/live/<name>
# lego:    called with LEGO_CERT_PATH / LEGO_CERT_KEY_PATH
# manual:  qtestframe-cert-deploy.sh FULLCHAIN KEY
set -eu
DEST='$DEST'
GROUP='$GROUP'
SERVICE='$SERVICE'
CERT_NAME='$CERT_NAME'

if [ \$# -eq 2 ]; then
	CRT=\$1; KEY=\$2
elif [ -n "\${RENEWED_LINEAGE:-}" ]; then
	# Only act on our own lineage (certbot runs hooks for every cert).
	[ "\$(basename "\$RENEWED_LINEAGE")" = "\$CERT_NAME" ] || exit 0
	CRT=\$RENEWED_LINEAGE/fullchain.pem; KEY=\$RENEWED_LINEAGE/privkey.pem
elif [ -n "\${LEGO_CERT_PATH:-}" ]; then
	CRT=\$LEGO_CERT_PATH; KEY=\$LEGO_CERT_KEY_PATH
else
	echo "usage: \$0 FULLCHAIN KEY (or run from certbot/lego)" >&2; exit 2
fi

umask 077
install -d -m 0750 -o root -g "\$GROUP" "\$DEST"
install -m 0644 -o root -g "\$GROUP" "\$CRT" "\$DEST/server.crt.new"
install -m 0640 -o root -g "\$GROUP" "\$KEY" "\$DEST/server.key.new"
# Sanity check before replacing the live files.
a=\$(openssl x509 -in "\$DEST/server.crt.new" -noout -pubkey | openssl sha256)
b=\$(openssl pkey -in "\$DEST/server.key.new" -pubout | openssl sha256)
if [ "\$a" != "\$b" ]; then
	echo "qtestframe-cert-deploy: key does not match certificate, aborting" >&2
	rm -f "\$DEST/server.crt.new" "\$DEST/server.key.new"; exit 1
fi
mv -f "\$DEST/server.key.new" "\$DEST/server.key"
mv -f "\$DEST/server.crt.new" "\$DEST/server.crt"
install -m 0644 -o root -g "\$GROUP" "\$DEST/server.crt" "\$DEST/fullchain.pem"
if command -v restorecon >/dev/null 2>&1; then restorecon -RF "\$DEST" || true; fi
if systemctl is-active --quiet "\$SERVICE" 2>/dev/null; then
	systemctl reload-or-restart "\$SERVICE"
fi
logger -t qtestframe-cert-deploy "installed \$(openssl x509 -in "\$DEST/server.crt" -noout -subject -enddate | tr '\n' ' ')" 2>/dev/null || true
EOF

#
# 2. Obtain the certificate.
#
if [ "$CLIENT" = certbot ]; then
	args=(certonly --non-interactive --agree-tos --cert-name "$CERT_NAME" --key-type "$KEY_TYPE" --keep-until-expiring --expand)
	if [ -n "$EMAIL" ]; then args+=(--email "$EMAIL"); else args+=(--register-unsafely-without-email); fi
	[ "$STAGING" -eq 0 ] || args+=(--staging)
	for d in "${DOMAIN_LIST[@]}"; do args+=(-d "$d"); done
	case "$METHOD" in
	standalone) args+=(--standalone --preferred-challenges http) ;;
	webroot) args+=(--webroot -w "$WEBROOT") ;;
	dns-*)
		plugin=${METHOD}
		args+=("--$plugin")
		if [ -n "$DNS_CREDENTIALS" ]; then
			[ "$DRY_RUN" -eq 1 ] || [ -f "$DNS_CREDENTIALS" ] || die "credentials file $DNS_CREDENTIALS not found"
			run chmod 0600 "$DNS_CREDENTIALS"
			args+=("--$plugin-credentials" "$DNS_CREDENTIALS")
		fi
		;;
	esac
	run mkdir -p /etc/letsencrypt/renewal-hooks/deploy
	run ln -sf "$HOOK" /etc/letsencrypt/renewal-hooks/deploy/qtestframe.sh
	run certbot "${args[@]}"
	# Install now (the deploy hook only runs on actual issuance/renewal).
	run env RENEWED_LINEAGE="/etc/letsencrypt/live/$CERT_NAME" "$HOOK"

	echo
	echo "Renewal: certbot renews automatically via its packaged timer/cron job:"
	if [ "$DRY_RUN" -eq 0 ]; then
		systemctl list-timers --all --no-pager 2>/dev/null | grep -E 'certbot|snap\.certbot' ||
			echo "  (no certbot timer found — add e.g. a daily cron job: certbot renew -q)"
	fi
	echo "Test renewal: certbot renew --dry-run --cert-name $CERT_NAME"
else
	[ -n "$EMAIL" ] || die "lego requires --email"
	largs=(--accept-tos --email "$EMAIL" --path "$LEGO_PATH" --key-type)
	if [ "$KEY_TYPE" = ecdsa ]; then largs+=(ec256); else largs+=(rsa3072); fi
	[ "$STAGING" -eq 0 ] || largs+=(--server https://acme-staging-v02.api.letsencrypt.org/directory)
	for d in "${DOMAIN_LIST[@]}"; do largs+=(--domains "$d"); done
	envfile_line=""
	case "$METHOD" in
	standalone) largs+=(--http --http.port :80) ;;
	webroot) largs+=(--http --http.webroot "$WEBROOT") ;;
	dns-*)
		largs+=(--dns "${METHOD#dns-}")
		if [ -n "$DNS_CREDENTIALS" ]; then
			[ "$DRY_RUN" -eq 1 ] || [ -f "$DNS_CREDENTIALS" ] || die "environment file $DNS_CREDENTIALS not found"
			run chmod 0600 "$DNS_CREDENTIALS"
			envfile_line="EnvironmentFile=$DNS_CREDENTIALS"
		fi
		;;
	esac
	run mkdir -p "$LEGO_PATH"
	run chmod 0700 "$LEGO_PATH"
	if [ -n "$DNS_CREDENTIALS" ] && [ "$DRY_RUN" -eq 0 ]; then
		# export the provider variables for the initial run
		set -a
		# shellcheck disable=SC1090
		. "$DNS_CREDENTIALS"
		set +a
	fi
	run lego "${largs[@]}" run --run-hook "$HOOK"

	# lego stores <domain>.crt (full chain) and <domain>.key; the hook gets
	# LEGO_CERT_PATH/LEGO_CERT_KEY_PATH. Daily renewal timer:
	quoted=$(printf ' %q' "${largs[@]}")
	write_file /etc/systemd/system/qtestframe-lego-renew.service 0644 <<EOF
[Unit]
Description=Renew the Qtestframe TLS certificate with lego
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
$envfile_line
ExecStart=/usr/bin/env lego$quoted renew --days 30 --renew-hook $HOOK
PrivateTmp=yes
ProtectHome=yes
NoNewPrivileges=yes
EOF
	write_file /etc/systemd/system/qtestframe-lego-renew.timer 0644 <<EOF
[Unit]
Description=Daily renewal check for the Qtestframe TLS certificate

[Timer]
OnCalendar=daily
RandomizedDelaySec=6h
Persistent=true

[Install]
WantedBy=timers.target
EOF
	run systemctl daemon-reload
	run systemctl enable --now qtestframe-lego-renew.timer
	echo
	echo "Renewal: systemd timer qtestframe-lego-renew.timer (daily, renews 30 days before expiry)"
	[ "$DRY_RUN" -eq 1 ] || systemctl list-timers --no-pager qtestframe-lego-renew.timer || true
	echo "Test renewal: systemctl start qtestframe-lego-renew.service && journalctl -u qtestframe-lego-renew"
fi

echo
echo "Certificate for $DOMAINS installed to $DEST/server.crt / server.key."
echo "config.yaml: server.tls_cert: $DEST/server.crt  server.tls_key: $DEST/server.key"
[ "$STAGING" -eq 0 ] || echo "NOTE: staging certificates are NOT trusted by browsers; re-run without --staging."
