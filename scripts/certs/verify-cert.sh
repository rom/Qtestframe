#!/usr/bin/env bash
# Show and verify a certificate (chain), its key and its expiry; or check
# what a running server presents.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source-path=SCRIPTDIR source=lib.sh
. "$SCRIPT_DIR/lib.sh"

CERT=""
KEY=""
CA=""
HOST=""
CONNECT=""
WARN_DAYS=30
PURPOSE=sslserver
QUIET=0

usage() {
	cat <<EOF
Usage: $QTF_PROG [--cert FILE] [--key FILE] [--ca FILE] [--host NAME] [options]
       $QTF_PROG --connect HOST:PORT [--ca FILE] [--host NAME]

Checks performed:
  * the certificate (first in FILE) parses; subject/issuer/SAN/validity shown
  * the chain (rest of FILE, plus --ca or the system trust store) verifies
  * the private key matches (--key)
  * the hostname matches a SAN (--host)
  * it does not expire within --warn-days (default $WARN_DAYS)
Exit status: 0 OK, 1 error, 2 expiring soon.

Options:
  --cert FILE        PEM certificate or full chain (leaf first)
  --key FILE         private key to match against the certificate
  --ca FILE          trust anchor (default: system trust store)
  --host NAME        hostname/IP that must be covered by the SANs
  --connect H:P      fetch the chain from a running TLS server instead
  --warn-days N      expiry warning threshold (default $WARN_DAYS)
  --client           verify for clientAuth instead of serverAuth
  -q, --quiet        print only problems
  -h, --help         show this help

Examples:
  $QTF_PROG --cert /etc/qtestframe/tls/server.crt --key /etc/qtestframe/tls/server.key --host qtf.example.com
  $QTF_PROG --connect qtf.example.com:8443 --ca certs/ca/ca.crt
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--cert) CERT=${2:?--cert needs a value}; shift ;;
	--key) KEY=${2:?--key needs a value}; shift ;;
	--ca) CA=${2:?--ca needs a value}; shift ;;
	--host) HOST=${2:?--host needs a value}; shift ;;
	--connect) CONNECT=${2:?--connect needs a value}; shift ;;
	--warn-days) WARN_DAYS=${2:?--warn-days needs a value}; shift ;;
	--client) PURPOSE=sslclient ;;
	-q | --quiet) QUIET=1 ;;
	-h | --help) usage; exit 0 ;;
	*) usage >&2; die "unknown option: $1" ;;
	esac
	shift
done

require_openssl
is_uint "$WARN_DAYS" || die "--warn-days must be a number"
[ -n "$CERT" ] || [ -n "$CONNECT" ] || { usage >&2; die "--cert or --connect is required"; }
[ -z "$CA" ] || [ -f "$CA" ] || die "CA file $CA not found"

TMP=$(make_tmpdir)
trap 'rm -rf "$TMP"' EXIT

say() { [ "$QUIET" -eq 1 ] || printf '%s\n' "$*"; }
STATUS=0
fail() { printf 'FAIL: %s\n' "$*"; STATUS=1; }
ok() { say "OK:   $*"; }

if [ -n "$CONNECT" ]; then
	sni=${HOST:-${CONNECT%:*}}
	[ -n "$HOST" ] || HOST=$sni
	# -showcerts prints the whole chain the server sends.
	"$OPENSSL" s_client -connect "$CONNECT" -servername "$sni" -showcerts </dev/null >"$TMP/sclient.txt" 2>&1 ||
		{ cat "$TMP/sclient.txt" >&2; die "could not connect to $CONNECT"; }
	awk '/-----BEGIN CERTIFICATE-----/,/-----END CERTIFICATE-----/' "$TMP/sclient.txt" >"$TMP/bundle.pem"
	CERT="$TMP/bundle.pem"
	proto=$(awk -F': *' '/Protocol *:/ {print $2; exit}' "$TMP/sclient.txt")
	cipher=$(awk -F': *' '/Cipher *:/ {print $2; exit}' "$TMP/sclient.txt")
	say "Connected to $CONNECT (SNI $sni): ${proto:-?} ${cipher:-?}"
fi

[ -s "$CERT" ] || die "certificate file $CERT not found or empty"

# Split the bundle: leaf -> leaf.pem, rest -> chain.pem
awk -v dir="$TMP" '
	/-----BEGIN CERTIFICATE-----/ { n++ }
	n == 1 { print > (dir "/leaf.pem") }
	n > 1  { print > (dir "/chain.pem") }
' "$CERT"
[ -s "$TMP/leaf.pem" ] || die "no PEM certificate found in $CERT"
nchain=0
if [ -s "$TMP/chain.pem" ]; then nchain=$(grep -c 'BEGIN CERTIFICATE' "$TMP/chain.pem"); fi

if [ "$QUIET" -eq 0 ]; then
	echo "--- Certificate"
	cert_summary "$TMP/leaf.pem"
	"$OPENSSL" x509 -in "$TMP/leaf.pem" -noout -fingerprint -sha256
	"$OPENSSL" x509 -in "$TMP/leaf.pem" -noout -text |
		awk '/Public Key Algorithm|Public-Key:|ASN1 OID|Extended Key Usage/ {gsub(/^ +/, ""); print} /X509v3 Extended Key Usage/ {getline; gsub(/^ +/, ""); print "  " $0}'
	echo "--- Intermediate/CA certificates in file: $nchain"
	if [ "$nchain" -gt 0 ]; then
		awk -v dir="$TMP" '/-----BEGIN CERTIFICATE-----/ { n++ } { print > (dir "/c" n ".pem") }' "$TMP/chain.pem"
		i=1
		while [ -f "$TMP/c$i.pem" ]; do
			printf '  [%d] ' "$i"
			"$OPENSSL" x509 -in "$TMP/c$i.pem" -noout -subject -enddate | tr '\n' ' '
			echo
			i=$((i + 1))
		done
	fi
	echo "--- Checks"
fi

# Chain verification.
verify_args=(-purpose "$PURPOSE")
if [ -n "$CA" ]; then verify_args+=(-CAfile "$CA"); fi
if [ "$nchain" -gt 0 ]; then verify_args+=(-untrusted "$TMP/chain.pem"); fi
# Self-signed leaf without --ca: trust itself, but say so.
if [ -z "$CA" ] && [ "$("$OPENSSL" x509 -in "$TMP/leaf.pem" -noout -subject | sed 's/^subject=//')" = \
	"$("$OPENSSL" x509 -in "$TMP/leaf.pem" -noout -issuer | sed 's/^issuer=//')" ]; then
	say "NOTE: certificate is self-signed; verifying against itself"
	verify_args+=(-CAfile "$TMP/leaf.pem")
fi
if out=$("$OPENSSL" verify "${verify_args[@]}" "$TMP/leaf.pem" 2>&1); then
	ok "chain verifies ($PURPOSE)"
else
	fail "chain verification: $(printf '%s' "$out" | tail -n 2 | tr '\n' ' ')"
fi

# Expiry.
if ! cert_valid_for "$TMP/leaf.pem" 0; then
	fail "certificate has EXPIRED ($("$OPENSSL" x509 -in "$TMP/leaf.pem" -noout -enddate))"
elif ! cert_valid_for "$TMP/leaf.pem" $((WARN_DAYS * 86400)); then
	printf 'WARN: certificate expires within %s days (%s)\n' "$WARN_DAYS" \
		"$("$OPENSSL" x509 -in "$TMP/leaf.pem" -noout -enddate | sed 's/^notAfter=//')"
	[ "$STATUS" -ne 0 ] || STATUS=2
else
	ok "valid for more than $WARN_DAYS days"
fi

# Key match.
if [ -n "$KEY" ]; then
	if [ ! -r "$KEY" ]; then
		fail "key $KEY not readable"
	elif key_matches_cert "$KEY" "$TMP/leaf.pem"; then
		ok "private key matches"
		# portable (GNU/BSD stat differ); file name is not parsed
		# shellcheck disable=SC2012
		perm=$(ls -l "$KEY" | awk '{print $1}')
		case "$perm" in
		-rw-------* | -rw-r-----* | -r--------* | -r--r-----*) ok "key permissions $perm" ;;
		*) fail "key $KEY has permissions $perm (expected 0600 or 0640)" ;;
		esac
	else
		fail "private key does NOT match the certificate"
	fi
fi

# Hostname.
if [ -n "$HOST" ]; then
	case "$HOST" in
	*[!0-9.]*) host_flag=-verify_hostname ;;
	*) host_flag=-verify_ip ;;
	esac
	if "$OPENSSL" verify "${verify_args[@]}" "$host_flag" "$HOST" "$TMP/leaf.pem" >/dev/null 2>&1; then
		ok "$HOST is covered by the certificate"
	else
		fail "$HOST is NOT covered by the certificate SANs"
	fi
fi

exit "$STATUS"
