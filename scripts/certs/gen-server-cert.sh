#!/usr/bin/env bash
# Issue a TLS server certificate for Qtestframe, signed by the local CA
# (gen-ca.sh) or fully self-signed.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source-path=SCRIPTDIR source=lib.sh
. "$SCRIPT_DIR/lib.sh"

CN=""
SAN=""
CA_DIR=${QTF_CA_DIR:-./certs/ca}
OUT=./certs/server
NAME=server
DAYS=397
SELF_SIGNED=0
KEY_TYPE=ec
KEY_PARAM=P-256
GROUP=""
FORCE=0
RENEW_DAYS=30

usage() {
	cat <<EOF
Usage: $QTF_PROG --cn HOST [--san DNS:a,DNS:b,IP:1.2.3.4] [options]

Create a TLS server key + certificate (extendedKeyUsage serverAuth).

Options:
  --cn HOST          common name (also added as DNS SAN if missing)
  --san LIST         subjectAltName list: DNS:name,IP:addr (default DNS:<cn>)
  --ca-dir DIR       CA created by gen-ca.sh (default: \$QTF_CA_DIR or ./certs/ca)
  --self-signed      do not use a CA; issue a self-signed certificate
  --days N           validity (default $DAYS; browsers reject > 398 days)
  --out DIR          output directory (default $OUT)
  --name BASE        file base name (default "$NAME" -> server.key/server.crt)
  --rsa [BITS]       RSA key (default 3072) instead of ECDSA P-256
  --ec [CURVE]       ECDSA key (P-256 default, or P-384)
  --group G          make the key readable by group G (0640), e.g. qtestframe
  --renew-days N     re-issue if the existing cert expires within N days (default $RENEW_DAYS)
  --force            always re-issue
  -h, --help         show this help

Output in --out:
  <name>.key       private key (0600, or 0640 with --group)
  <name>.crt       leaf certificate
  chain.pem        issuing CA certificate (CA mode only)
  fullchain.pem    leaf + CA (what to configure when clients lack the CA)

Example (install directly for the service, as root):
  $QTF_PROG --cn qtf.example.com --san DNS:qtf.example.com,DNS:qtf,IP:10.0.0.5 \\
      --out /etc/qtestframe/tls --group qtestframe
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--cn) CN=${2:?--cn needs a value}; shift ;;
	--san) SAN=${2:?--san needs a value}; shift ;;
	--ca-dir) CA_DIR=${2:?--ca-dir needs a value}; shift ;;
	--self-signed) SELF_SIGNED=1 ;;
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

SAN=$(normalize_san "${SAN:-DNS:$CN}")
# Browsers/Go ignore the CN: make sure it is a SAN, unless it is an IP.
case "$CN" in
*[!0-9.]*) san_contains "$SAN" "DNS:$CN" || SAN="DNS:$CN,$SAN" ;;
*) san_contains "$SAN" "IP:$CN" || SAN="IP:$CN,$SAN" ;;
esac
[ -n "$SAN" ] || die "at least one subjectAltName is required"

KEY="$OUT/$NAME.key"
CRT="$OUT/$NAME.crt"
CHAIN="$OUT/chain.pem"
FULL="$OUT/fullchain.pem"

if [ "$SELF_SIGNED" -eq 0 ]; then
	[ -f "$CA_DIR/ca.crt" ] && [ -f "$CA_DIR/ca.key" ] ||
		die "no CA in $CA_DIR (run gen-ca.sh first, pass --ca-dir, or use --self-signed)"
fi

# Idempotency: keep a matching, sufficiently valid certificate.
if [ "$FORCE" -eq 0 ] && [ -f "$KEY" ] && [ -f "$CRT" ] &&
	key_matches_cert "$KEY" "$CRT" && cert_valid_for "$CRT" $((RENEW_DAYS * 86400)); then
	current_san=$("$OPENSSL" x509 -in "$CRT" -noout -text |
		awk '/X509v3 Subject Alternative Name/ {getline; gsub(/ /, ""); gsub(/IPAddress:/, "IP:"); print}')
	if [ "$current_san" = "$SAN" ]; then
		info "$CRT is valid for more than $RENEW_DAYS days with the same SANs; nothing to do (--force to re-issue)"
		exit 0
	fi
	info "SANs changed ($current_san -> $SAN); re-issuing"
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

[ v3_server ]
basicConstraints       = critical,CA:false
keyUsage               = critical,digitalSignature$([ "$KEY_TYPE" = rsa ] && printf ',keyEncipherment')
extendedKeyUsage       = serverAuth
subjectAltName         = $SAN
subjectKeyIdentifier   = hash
EOF

DIGEST=$(digest_for "$KEY_TYPE" "$KEY_PARAM")
gen_key "$KEY_TYPE" "$KEY_PARAM" "$TMP/new.key"

if [ "$SELF_SIGNED" -eq 1 ]; then
	"$OPENSSL" req -new -x509 -config "$TMP/req.cnf" -extensions v3_server \
		-key "$TMP/new.key" -out "$TMP/new.crt" -days "$DAYS" -"$DIGEST" \
		-set_serial "$(rand_serial)"
	cp "$TMP/new.crt" "$TMP/fullchain.pem"
else
	printf 'authorityKeyIdentifier = keyid,issuer\n' >>"$TMP/req.cnf"
	"$OPENSSL" req -new -config "$TMP/req.cnf" -key "$TMP/new.key" -out "$TMP/new.csr"
	"$OPENSSL" x509 -req -in "$TMP/new.csr" -CA "$CA_DIR/ca.crt" -CAkey "$CA_DIR/ca.key" \
		-set_serial "$(rand_serial)" -days "$DAYS" -"$DIGEST" \
		-extfile "$TMP/req.cnf" -extensions v3_server -out "$TMP/new.crt" 2>/dev/null
	"$OPENSSL" verify -CAfile "$CA_DIR/ca.crt" -purpose sslserver "$TMP/new.crt" >/dev/null ||
		die "verification of the new certificate failed"
	cat "$TMP/new.crt" "$CA_DIR/ca.crt" >"$TMP/fullchain.pem"
	cp "$CA_DIR/ca.crt" "$TMP/chain.pem"
fi

# Install atomically (rename within the target directory).
install_file() { # src dst mode
	cp "$1" "$2.tmp.$$"
	chmod "$3" "$2.tmp.$$"
	mv -f "$2.tmp.$$" "$2"
}
(
	umask 077
	install_file "$TMP/new.key" "$KEY" 0600
)
install_file "$TMP/new.crt" "$CRT" 0644
install_file "$TMP/fullchain.pem" "$FULL" 0644
if [ -f "$TMP/chain.pem" ]; then install_file "$TMP/chain.pem" "$CHAIN" 0644; fi
if [ -n "$GROUP" ]; then set_group_readable "$KEY" "$GROUP"; fi

info "issued $CRT ($([ "$SELF_SIGNED" -eq 1 ] && echo self-signed || echo "CA: $CA_DIR"))"
cert_summary "$CRT" >&2
info "key: $KEY  full chain: $FULL"
