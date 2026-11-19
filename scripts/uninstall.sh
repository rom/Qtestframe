#!/usr/bin/env bash
# Remove Qtestframe from a host installed with scripts/install.sh.
#
# By default configuration (/etc/qtestframe), data (/var/lib/qtestframe) and
# the system user are KEPT. Use --purge to delete them as well.
set -euo pipefail

PROG=$(basename "$0")
REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PURGE=0
YES=0
DRY_RUN=0

usage() {
	cat <<EOF
Usage: $PROG [--purge [--yes]] [--dry-run]

  --purge     also delete /etc/qtestframe, /var/lib/qtestframe (DATABASE!),
              /var/log/qtestframe, the qtestframe user and the lego timer
  --yes       do not ask for confirmation with --purge
  --dry-run   print the actions only
  -h, --help  show this help

Take a backup first (see docs/operations.md, "Backup and restore").
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--purge) PURGE=1 ;;
	--yes | -y) YES=1 ;;
	--dry-run) DRY_RUN=1 ;;
	-h | --help) usage; exit 0 ;;
	*) echo "$PROG: unknown option: $1" >&2; usage >&2; exit 2 ;;
	esac
	shift
done

say() { printf '==> %s\n' "$*"; }
run() {
	if [ "$DRY_RUN" -eq 1 ]; then
		printf '+'
		printf ' %q' "$@"
		printf '\n'
	else
		"$@"
	fi
}
rm_if() {
	local p
	for p in "$@"; do
		if [ -e "$p" ] || [ -L "$p" ]; then run rm -rf -- "$p"; fi
	done
}

if [ "$DRY_RUN" -eq 0 ] && [ "$(id -u)" -ne 0 ]; then
	echo "$PROG: must run as root" >&2
	exit 1
fi

if [ "$PURGE" -eq 1 ] && [ "$YES" -eq 0 ] && [ "$DRY_RUN" -eq 0 ]; then
	printf 'This permanently deletes the Qtestframe database and configuration. Type "purge" to continue: '
	read -r answer
	[ "$answer" = purge ] || { echo "aborted"; exit 1; }
fi

say "stopping and disabling qtestframe.service"
if command -v systemctl >/dev/null 2>&1; then
	run systemctl disable --now qtestframe.service 2>/dev/null || true
	if [ "$PURGE" -eq 1 ]; then
		run systemctl disable --now qtestframe-lego-renew.timer 2>/dev/null || true
	fi
fi

say "removing unit, binary and system integration files"
rm_if /etc/systemd/system/qtestframe.service \
	/usr/lib/systemd/system/qtestframe.service \
	/etc/systemd/system/multi-user.target.wants/qtestframe.service \
	/usr/bin/qtestframe \
	/usr/share/doc/qtestframe \
	/etc/tmpfiles.d/qtestframe.conf \
	/etc/sysctl.d/99-qtestframe.conf

if [ -f /etc/apparmor.d/usr.bin.qtestframe ]; then
	say "removing AppArmor profile"
	if command -v apparmor_parser >/dev/null 2>&1; then
		run apparmor_parser -R /etc/apparmor.d/usr.bin.qtestframe || true
	fi
	rm_if /etc/apparmor.d/usr.bin.qtestframe
fi

if command -v semodule >/dev/null 2>&1 && semodule -l 2>/dev/null | grep -qw qtestframe; then
	say "removing SELinux module and port label"
	run "$REPO_DIR/deploy/selinux/build.sh" --uninstall || true
fi

if [ "$PURGE" -eq 1 ]; then
	say "purging configuration, data, logs and the service account"
	rm_if /etc/qtestframe /var/lib/qtestframe /var/log/qtestframe /run/qtestframe \
		/etc/systemd/system/qtestframe-lego-renew.service \
		/etc/systemd/system/qtestframe-lego-renew.timer \
		/etc/letsencrypt/renewal-hooks/deploy/qtestframe.sh \
		/usr/local/lib/qtestframe \
		/etc/sysusers.d/qtestframe.conf
	if getent passwd qtestframe >/dev/null 2>&1; then run userdel qtestframe || true; fi
	if getent group qtestframe >/dev/null 2>&1; then run groupdel qtestframe || true; fi
else
	say "kept: /etc/qtestframe, /var/lib/qtestframe, /var/log/qtestframe, user qtestframe (use --purge to remove)"
fi

if command -v systemctl >/dev/null 2>&1; then run systemctl daemon-reload || true; fi
say "done"
