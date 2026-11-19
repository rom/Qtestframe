#!/usr/bin/env bash
# Install Qtestframe on a systemd-based Linux host.
#
#   sudo scripts/install.sh --binary ./bin/qtestframe
#
# Idempotent: re-running upgrades the binary/unit and keeps the existing
# configuration, TLS material and data.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PROG=$(basename "$0")

BINARY=""
SELINUX=auto
APPARMOR=no
SYSCTL=no
ENABLE=1
START=1
SELF_SIGNED=0
CERT_CN=""
DRY_RUN=0

BIN_DST=/usr/bin/qtestframe
UNIT_DST=/etc/systemd/system/qtestframe.service
SYSUSERS_DST=/etc/sysusers.d/qtestframe.conf
TMPFILES_DST=/etc/tmpfiles.d/qtestframe.conf
CONF_DIR=/etc/qtestframe
DOC_DIR=/usr/share/doc/qtestframe

usage() {
	cat <<EOF
Usage: $PROG [options]

Installs:
  $BIN_DST                     (from --binary, ./bin/qtestframe or ./dist/qtestframe)
  $UNIT_DST
  $SYSUSERS_DST, $TMPFILES_DST
  $CONF_DIR/config.yaml          (from deploy/config.example.yaml, never overwritten)
  $DOC_DIR/                (security.md, operations.md)
and enables + starts qtestframe.service.

Options:
  --binary PATH          qtestframe binary to install
  --selinux auto|yes|no  build + load the SELinux module (default auto:
                         when SELinux is enabled)
  --apparmor             install and enforce the AppArmor profile
  --sysctl               install deploy/sysctl/99-qtestframe.conf
  --self-signed-cert [CN] create a self-signed TLS certificate if none exists
                         (CN defaults to the host FQDN)
  --no-enable            do not enable the unit
  --no-start             do not (re)start the service
  --dry-run              print the actions only
  -h, --help             show this help
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--binary) BINARY=${2:?--binary needs a value}; shift ;;
	--selinux) SELINUX=${2:?--selinux needs auto, yes or no}; shift ;;
	--apparmor) APPARMOR=yes ;;
	--sysctl) SYSCTL=yes ;;
	--self-signed-cert)
		SELF_SIGNED=1
		if [ $# -gt 1 ] && [ "${2#-}" = "$2" ]; then CERT_CN=$2; shift; fi
		;;
	--no-enable) ENABLE=0; START=0 ;;
	--no-start) START=0 ;;
	--dry-run) DRY_RUN=1 ;;
	-h | --help) usage; exit 0 ;;
	*) echo "$PROG: unknown option: $1" >&2; usage >&2; exit 2 ;;
	esac
	shift
done

say() { printf '==> %s\n' "$*"; }
warn() { printf '%s: warning: %s\n' "$PROG" "$*" >&2; }
die() { printf '%s: error: %s\n' "$PROG" "$*" >&2; exit 1; }
run() {
	if [ "$DRY_RUN" -eq 1 ]; then
		printf '+'
		printf ' %q' "$@"
		printf '\n'
	else
		"$@"
	fi
}

[ "$(uname -s)" = Linux ] || die "only Linux hosts are supported (use scripts/dev.sh on macOS)"
if [ "$DRY_RUN" -eq 0 ]; then
	[ "$(id -u)" -eq 0 ] || die "must run as root"
	[ -d /run/systemd/system ] || die "systemd is not running on this host"
fi

if [ -z "$BINARY" ]; then
	for c in "$REPO_DIR/bin/qtestframe" "$REPO_DIR/dist/qtestframe" ./qtestframe; do
		if [ -x "$c" ]; then BINARY=$c; break; fi
	done
fi
[ -n "$BINARY" ] || die "no binary found; build one (CGO_ENABLED=0 go build -o bin/qtestframe ./cmd/qtestframe) or pass --binary"
[ -f "$BINARY" ] || die "$BINARY not found"
case "$(head -c 4 "$BINARY" | od -An -c | tr -d ' ')" in
*ELF*) ;;
*) die "$BINARY is not an ELF executable" ;;
esac

# 1. Binary
say "installing binary $BINARY -> $BIN_DST"
run install -D -m 0755 -o root -g root "$BINARY" "$BIN_DST.new"
run mv -f "$BIN_DST.new" "$BIN_DST"
if [ "$DRY_RUN" -eq 0 ]; then "$BIN_DST" version || warn "'qtestframe version' failed"; fi

# 2. System user
say "creating system user qtestframe"
run install -D -m 0644 "$REPO_DIR/deploy/systemd/qtestframe.sysusers.conf" "$SYSUSERS_DST"
if command -v systemd-sysusers >/dev/null 2>&1; then
	run systemd-sysusers "$SYSUSERS_DST"
elif ! getent passwd qtestframe >/dev/null; then
	run useradd --system --user-group --home-dir /var/lib/qtestframe --shell /usr/sbin/nologin qtestframe
fi

# 3. Directories
say "creating directories"
run install -D -m 0644 "$REPO_DIR/deploy/systemd/qtestframe.tmpfiles.conf" "$TMPFILES_DST"
run systemd-tmpfiles --create "$TMPFILES_DST"

# 4. Configuration (never overwritten)
run install -m 0640 -o root -g qtestframe "$REPO_DIR/deploy/config.example.yaml" "$CONF_DIR/config.example.yaml"
if [ -e "$CONF_DIR/config.yaml" ]; then
	say "keeping existing $CONF_DIR/config.yaml (new example: $CONF_DIR/config.example.yaml)"
else
	say "installing default configuration $CONF_DIR/config.yaml"
	run install -m 0640 -o root -g qtestframe "$REPO_DIR/deploy/config.example.yaml" "$CONF_DIR/config.yaml"
fi

# 5. TLS
if [ "$SELF_SIGNED" -eq 1 ]; then
	if [ -e "$CONF_DIR/tls/server.crt" ]; then
		say "keeping existing $CONF_DIR/tls/server.crt"
	else
		[ -n "$CERT_CN" ] || CERT_CN=$(hostname -f 2>/dev/null || hostname)
		say "creating self-signed certificate for $CERT_CN"
		run "$REPO_DIR/scripts/certs/gen-server-cert.sh" --self-signed --cn "$CERT_CN" \
			--san "DNS:$CERT_CN,DNS:localhost,IP:127.0.0.1" --out "$CONF_DIR/tls" --group qtestframe
		run chown root:qtestframe "$CONF_DIR/tls/server.crt" "$CONF_DIR/tls/fullchain.pem"
	fi
fi

# 6. systemd unit and docs
say "installing systemd unit $UNIT_DST"
run install -D -m 0644 "$REPO_DIR/deploy/systemd/qtestframe.service" "$UNIT_DST"
for d in security.md operations.md; do
	if [ -f "$REPO_DIR/docs/$d" ]; then run install -D -m 0644 "$REPO_DIR/docs/$d" "$DOC_DIR/$d"; fi
done

# 7. SELinux
if [ "$SELINUX" = auto ]; then
	if command -v selinuxenabled >/dev/null 2>&1 && selinuxenabled; then SELINUX=yes; else SELINUX=no; fi
fi
if [ "$SELINUX" = yes ]; then
	say "building and loading the SELinux module"
	run "$REPO_DIR/deploy/selinux/build.sh" --install
	say "enable optional access with e.g.: setsebool -P qtestframe_connect_postgresql on"
fi

# 8. AppArmor
if [ "$APPARMOR" = yes ]; then
	command -v apparmor_parser >/dev/null 2>&1 || die "apparmor_parser not found"
	say "installing AppArmor profile"
	run install -D -m 0644 "$REPO_DIR/deploy/apparmor/usr.bin.qtestframe" /etc/apparmor.d/usr.bin.qtestframe
	run mkdir -p /etc/apparmor.d/local
	[ -e /etc/apparmor.d/local/usr.bin.qtestframe ] || run touch /etc/apparmor.d/local/usr.bin.qtestframe
	run apparmor_parser -r /etc/apparmor.d/usr.bin.qtestframe
fi

# 9. sysctl
if [ "$SYSCTL" = yes ]; then
	say "installing sysctl hardening"
	run install -D -m 0644 "$REPO_DIR/deploy/sysctl/99-qtestframe.conf" /etc/sysctl.d/99-qtestframe.conf
	run sysctl --system
fi

# 10. Service
run systemctl daemon-reload
if [ "$ENABLE" -eq 1 ]; then run systemctl enable qtestframe.service; fi

tls_cert=$(awk '/^[[:space:]]*tls_cert:/ {print $2; exit}' "$CONF_DIR/config.yaml" 2>/dev/null | tr -d '"' || true)
if [ -n "$tls_cert" ] && [ ! -e "$tls_cert" ] && [ "$DRY_RUN" -eq 0 ]; then
	warn "TLS certificate $tls_cert does not exist yet; not starting."
	warn "create one (scripts/certs/, or re-run with --self-signed-cert) then: systemctl start qtestframe"
	START=0
fi

if [ "$START" -eq 1 ]; then
	say "(re)starting qtestframe"
	run systemctl restart qtestframe.service
	if [ "$DRY_RUN" -eq 0 ]; then
		sleep 2
		systemctl --no-pager --lines=5 status qtestframe.service || warn "service is not running; see journalctl -u qtestframe"
	fi
fi

cat <<EOF

Qtestframe installed.
  Configure:   $CONF_DIR/config.yaml   (then: systemctl restart qtestframe)
  First admin: sudo -u qtestframe env QTF_CONFIG=$CONF_DIR/config.yaml qtestframe user create \\
                   --email admin@example.com --password-stdin --super-admin
               (type the password, then Ctrl-D; or set bootstrap.* in config.yaml)
  Logs:        journalctl -u qtestframe -f
  Security:    systemd-analyze security qtestframe
EOF
