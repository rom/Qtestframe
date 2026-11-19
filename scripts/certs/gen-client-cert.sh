#!/usr/bin/env bash
# Issue a TLS client certificate (extendedKeyUsage clientAuth) signed by the
# local CA, e.g. for mutual TLS to a central syslog server or PostgreSQL
# client-certificate authentication.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source-path=SCRIPTDIR source=lib.sh
. "$SCRIPT_DIR/lib.sh"

CN=""
SAN=""
CA_DIR=${QTF_CA_DIR:-./certs/ca}
OUT=./certs/client
NAME=client
DAYS=397
KEY_TYPE=ec
KEY_PARAM=P-256
GROUP=""
FORCE=0
RENEW_DAYS=30
P12=0

usage() {
	cat <<EOF
Usage: $QTF_PROG --cn NAME [options]

Create a client key + certificate signed by the CA from gen-ca.sh.

Options:
  --cn NAME          subject CN. For PostgreSQL "cert" authentication this
                     must equal the database user name (e.g. qtestframe).
  --san LIST         optional subjectAltName (DNS:host,email:a@b,URI:...)
  --ca-dir DIR       CA directory (default: \$QTF_CA_DIR or ./certs/ca)
  --days N           validity (default $DAYS)
  --out DIR          output directory (default $OUT)
  --name BASE        file base name (default "$NAME")
  --rsa [BITS]       RSA key (default 3072) instead of ECDSA P-256
  --ec [CURVE]       ECDSA key (P-256 default, or P-384)
  --group G          make the key readable by group G (0640)
  --p12              additionally write <name>.p12 (password prompted by openssl)
  --renew-days N     re-issue if expiring within N days (default $RENEW_DAYS)
  --force            always re-issue
  -h, --help         show this help

Examples:
  # syslog mTLS (rsyslog/syslog-ng verify the client against the CA)
  $QTF_PROG --cn qtf01.example.com --san DNS:qtf01.example.com --name syslog-client \\
      --out /etc/qtestframe/tls --group qtestframe
  # PostgreSQL clientcert=verify-full
  $QTF_PROG --cn qtestframe --name db-client --out /etc/qtestframe/tls --group qtestframe
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--cn) CN=${2:?--cn needs a value}; shift ;;
	--san) SAN=${2:?--san needs a value}; shift ;;
	--ca-dir) CA_DIR=${2:?--ca-dir needs a value}; shift ;;
	--days) DAYS=${2:?--days needs a value}; shift ;;
	--out) OUT=${2:?--out needs a value}; shift ;;
	--name) NAME=${2:?--name needs a value}; shift ;;
	--ec)
		KEY_TYPE=ec
		if [ $# -gt 1 ] && [ "${2#-}" = "$2" ]; then KEY_PARAM=$2; shift; else KEY_PARAM=P-256; fi
		;;
	--rsa)
		KEY_TYPE=rsa
		if [ $# -gt 1 ] && is_uint "$2"; then KEY_PARAM=$2; shift; else KEY_PARAM=3072; fi
		;;
	--group) GROUP=${2:?--group needs a value}; shift ;;
	--p12) P12=1 ;;
	--renew-days) RENEW_DAYS=${2:?--renew-days needs a value}; shift ;;
	--force) FORCE=1 ;;
	-h | --help) usage; exit 0 ;;
	*) usage >&2; die "unknown option: $1" ;;
	esac
	shift
done

require_openssl
[ -n "$CN" ] || { usage >&2; die "--cn is required"; }
check_dn_value "$CN" "--cn"
check_dn_value "$NAME" "--name"
is_uint "$DAYS" || die "--days must be a number"
is_uint "$RENEW_DAYS" || die "--renew-days must be a number"
[ -z "$SAN" ] || SAN=$(normalize_san "$SAN")
[ -f "$CA_DIR/ca.crt" ] && [ -f "$CA_DIR/ca.key" ] || die "no CA in $CA_DIR (run gen-ca.sh first or pass --ca-dir)"

KEY="$OUT/$NAME.key"
CRT="$OUT/$NAME.crt"

if [ "$FORCE" -eq 0 ] && [ -f "$KEY" ] && [ -f "$CRT" ] &&
	key_matches_cert "$KEY" "$CRT" && cert_valid_for "$CRT" $((RENEW_DAYS * 86400)) &&
	"$OPENSSL" verify -CAfile "$CA_DIR/ca.crt" "$CRT" >/dev/null 2>&1; then
	info "$CRT is still valid for more than $RENEW_DAYS days; nothing to do (--force to re-issue)"
	exit 0
fi

mkdir -p "$OUT"
TMP=$(make_tmpdir)
trap 'rm -rf "$TMP"' EXIT

cat >"$TMP/req.cnf" <<EOF
[ req ]
distinguished_name = dn
prompt             = no
utf8               = yes

[ dn ]
CN = $CN

[ v3_client ]
basicConstraints       = critical,CA:false
keyUsage               = critical,digitalSignature$([ "$KEY_TYPE" = rsa ] && printf ',keyEncipherment')
extendedKeyUsage       = clientAuth
subjectKeyIdentifier   = hash
authorityKeyIdentifier = keyid,issuer
${SAN:+subjectAltName = $SAN}
EOF

DIGEST=$(digest_for "$KEY_TYPE" "$KEY_PARAM")
gen_key "$KEY_TYPE" "$KEY_PARAM" "$TMP/new.key"
"$OPENSSL" req -new -config "$TMP/req.cnf" -key "$TMP/new.key" -out "$TMP/new.csr"
"$OPENSSL" x509 -req -in "$TMP/new.csr" -CA "$CA_DIR/ca.crt" -CAkey "$CA_DIR/ca.key" \
	-set_serial "$(rand_serial)" -days "$DAYS" -"$DIGEST" \
	-extfile "$TMP/req.cnf" -extensions v3_client -out "$TMP/new.crt" 2>/dev/null
"$OPENSSL" verify -CAfile "$CA_DIR/ca.crt" -purpose sslclient "$TMP/new.crt" >/dev/null ||
	die "verification of the new certificate failed"

(
	umask 077
	cp "$TMP/new.key" "$KEY.tmp.$$" && chmod 0600 "$KEY.tmp.$$" && mv -f "$KEY.tmp.$$" "$KEY"
)
cp "$TMP/new.crt" "$CRT.tmp.$$" && chmod 0644 "$CRT.tmp.$$" && mv -f "$CRT.tmp.$$" "$CRT"
cp "$CA_DIR/ca.crt" "$OUT/ca.crt" && chmod 0644 "$OUT/ca.crt"
if [ -n "$GROUP" ]; then set_group_readable "$KEY" "$GROUP"; fi

if [ "$P12" -eq 1 ]; then
	(
		umask 077
		"$OPENSSL" pkcs12 -export -in "$CRT" -inkey "$KEY" -certfile "$CA_DIR/ca.crt" \
			-name "$CN" -out "$OUT/$NAME.p12"
	)
	info "wrote $OUT/$NAME.p12"
fi

info "issued client certificate $CRT"
cert_summary "$CRT" >&2
info "key: $KEY  CA: $OUT/ca.crt"
