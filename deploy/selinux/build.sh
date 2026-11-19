#!/usr/bin/env bash
# Build and (optionally) install the qtestframe SELinux policy module.
#
# Requirements (Fedora/RHEL/Alma/Rocky):
#   dnf install selinux-policy-devel policycoreutils-python-utils
# Debian/Ubuntu (refpolicy):
#   apt install selinux-policy-dev policycoreutils semodule-utils
#
# Usage: build.sh [--install] [--port N] [--no-port] [--enable-booleans LIST] [--uninstall]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MODULE=qtestframe
DEVEL_MAKEFILE=${DEVEL_MAKEFILE:-/usr/share/selinux/devel/Makefile}
PORT=8443
DO_INSTALL=0
DO_PORT=1
DO_UNINSTALL=0
BOOLEANS=""

usage() {
	cat <<EOF
Usage: $(basename "$0") [options]

Compile ${MODULE}.pp from ${MODULE}.te/.fc/.if. With --install, also load
the module, label the port and restore file contexts.

Options:
  --install              semodule -i, semanage port, restorecon
  --port N               TCP port to label qtestframe_port_t (default 8443)
  --no-port              do not touch port labels
  --enable-booleans L    comma separated booleans to enable persistently,
                         e.g. qtestframe_connect_postgresql,qtestframe_connect_http
  --uninstall            remove the module, port label and relabel files
  -h, --help             this help

Booleans: qtestframe_connect_postgresql qtestframe_remote_syslog
          qtestframe_connect_http qtestframe_bind_http_port
          qtestframe_run_scripts qtestframe_runner_execmem
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--install) DO_INSTALL=1 ;;
	--port) PORT=${2:?--port needs a value}; shift ;;
	--no-port) DO_PORT=0 ;;
	--enable-booleans) BOOLEANS=${2:?--enable-booleans needs a value}; shift ;;
	--uninstall) DO_UNINSTALL=1 ;;
	-h | --help) usage; exit 0 ;;
	*) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
	esac
	shift
done

need_root() {
	if [ "$(id -u)" -ne 0 ]; then
		echo "error: $1 requires root" >&2
		exit 1
	fi
}

relabel() {
	local p
	for p in /usr/bin/qtestframe /usr/local/bin/qtestframe /etc/qtestframe \
		/var/lib/qtestframe /run/qtestframe /var/log/qtestframe; do
		if [ -e "$p" ]; then restorecon -RF "$p"; fi
	done
}

if [ "$DO_UNINSTALL" -eq 1 ]; then
	need_root "--uninstall"
	if [ "$DO_PORT" -eq 1 ]; then
		semanage port -d -t qtestframe_port_t -p tcp "$PORT" 2>/dev/null || true
	fi
	semodule -r "$MODULE" || true
	relabel
	echo "SELinux module ${MODULE} removed."
	exit 0
fi

if [ ! -f "$DEVEL_MAKEFILE" ]; then
	echo "error: $DEVEL_MAKEFILE not found; install selinux-policy-devel (dnf) or selinux-policy-dev (apt)" >&2
	exit 1
fi

# Build in a scratch copy so the source tree stays clean.
BUILD_DIR="$(mktemp -d)"
trap 'rm -rf "$BUILD_DIR"' EXIT
cp "$SCRIPT_DIR/${MODULE}.te" "$SCRIPT_DIR/${MODULE}.fc" "$SCRIPT_DIR/${MODULE}.if" "$BUILD_DIR/"
make -C "$BUILD_DIR" -f "$DEVEL_MAKEFILE" "${MODULE}.pp"
cp "$BUILD_DIR/${MODULE}.pp" "$SCRIPT_DIR/${MODULE}.pp"
echo "Built $SCRIPT_DIR/${MODULE}.pp"

if [ "$DO_INSTALL" -eq 0 ]; then
	echo "Run with --install (as root) to load it."
	exit 0
fi

need_root "--install"
semodule -i "$SCRIPT_DIR/${MODULE}.pp"
echo "Loaded module ${MODULE}."

if [ "$DO_PORT" -eq 1 ]; then
	# -a fails if the port already has a policy-defined type (on Fedora/RHEL
	# 8443 is http_port_t); in that case override it with -m.
	if semanage port -l | awk '$1=="qtestframe_port_t" && $2=="tcp"' | grep -qw "$PORT"; then
		echo "Port tcp/$PORT already labelled qtestframe_port_t."
	elif semanage port -a -t qtestframe_port_t -p tcp "$PORT" 2>/dev/null; then
		echo "Labelled tcp/$PORT as qtestframe_port_t."
	else
		semanage port -m -t qtestframe_port_t -p tcp "$PORT"
		echo "Re-labelled tcp/$PORT as qtestframe_port_t (was a policy-defined port type)."
	fi
fi

if [ -n "$BOOLEANS" ]; then
	IFS=',' read -r -a bools <<<"$BOOLEANS"
	for b in "${bools[@]}"; do
		setsebool -P "$b" on
		echo "Enabled boolean $b."
	done
fi

relabel
echo "Done. Verify with: ps -eZ | grep qtestframe ; getsebool -a | grep qtestframe"
