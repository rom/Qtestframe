#!/usr/bin/env bash
# Create a local root certificate authority for Qtestframe deployments
# (server certificates, syslog/PostgreSQL client certificates).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source-path=SCRIPTDIR source=lib.sh
. "$SCRIPT_DIR/lib.sh"

CA_DIR=${QTF_CA_DIR:-./certs/ca}
CN="Qtestframe Local Root CA"
ORG=""
DAYS=3650
KEY_TYPE=ec
KEY_PARAM=P-256
PERMIT=""
FORCE=0

usage() {
	cat <<EOF
Usage: $QTF_PROG [options]

Create a self-signed root CA (ca.key + ca.crt) in --dir. Idempotent: an
existing, valid CA is kept unless --force is given.

Options:
  --dir DIR              output directory (default: \$QTF_CA_DIR or ./certs/ca)
  --cn NAME              CA common name (default: "$CN")
  --org NAME             organization (O=)
  --days N               validity in days (default: $DAYS)
  --ec [CURVE]           ECDSA key, CURVE P-256 (default) or P-384
  --rsa [BITS]           RSA key instead of ECDSA (default 4096 bits)
  --permit LIST          add critical X.509 name constraints, e.g.
                         "DNS:example.com,DNS:.example.com,IP:10.0.0.0/255.0.0.0"
                         (the CA can then only issue for these names)
  --force                overwrite an existing CA (old key is moved aside)
  -h, --help             show this help

Files: ca.key (0600, keep offline if possible), ca.crt (distribute to
clients / trust stores).
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--dir) CA_DIR=${2:?--dir needs a value}; shift ;;
	--cn) CN=${2:?--cn needs a value}; shift ;;
	--org) ORG=${2:?--org needs a value}; shift ;;
	--days) DAYS=${2:?--days needs a value}; shift ;;
	--ec)
		KEY_TYPE=ec
		if [ $# -gt 1 ] && [ "${2#-}" = "$2" ]; then KEY_PARAM=$2; shift; else KEY_PARAM=P-256; fi
		;;
	--rsa)
		KEY_TYPE=rsa
		if [ $# -gt 1 ] && is_uint "$2"; then KEY_PARAM=$2; shift; else KEY_PARAM=4096; fi
		;;
	--permit) PERMIT=${2:?--permit needs a value}; shift ;;
	--force) FORCE=1 ;;
	-h | --help) usage; exit 0 ;;
	*) usage >&2; die "unknown option: $1" ;;
	esac
	shift
done

require_openssl
is_uint "$DAYS" || die "--days must be a number"
check_dn_value "$CN" "--cn"
[ -z "$ORG" ] || check_dn_value "$ORG" "--org"

KEY="$CA_DIR/ca.key"
CRT="$CA_DIR/ca.crt"

if [ -f "$KEY" ] && [ -f "$CRT" ] && [ "$FORCE" -eq 0 ]; then
	if key_matches_cert "$KEY" "$CRT" && cert_valid_for "$CRT" 0; then
		info "CA already exists in $CA_DIR (use --force to replace):"
		cert_summary "$CRT" >&2
		exit 0
	fi
	die "$CA_DIR contains an invalid/expired CA or a key not matching ca.crt; use --force"
fi

mkdir -p "$CA_DIR"
chmod 0700 "$CA_DIR"

if [ "$FORCE" -eq 1 ] && [ -f "$KEY" ]; then
	stamp=$(date +%Y%m%d%H%M%S)
	mv -f "$KEY" "$KEY.old-$stamp"
	[ ! -f "$CRT" ] || mv -f "$CRT" "$CRT.old-$stamp"
	info "previous CA moved to *.old-$stamp"
fi

TMP=$(make_tmpdir)
trap 'rm -rf "$TMP"' EXIT

NC_LINE=""
if [ -n "$PERMIT" ]; then
	PERMIT=$(normalize_san "$PERMIT")
	nc=""
	IFS=',' read -r -a parts <<<"$PERMIT"
	for p in "${parts[@]}"; do nc="${nc:+$nc,}permitted;$p"; done
	NC_LINE="nameConstraints = critical,$nc"
fi

cat >"$TMP/ca.cnf" <<EOF
[ req ]
distinguished_name = dn
prompt             = no
x509_extensions    = v3_ca
utf8               = yes

[ dn ]
CN = $CN
${ORG:+O = $ORG}

[ v3_ca ]
basicConstraints       = critical,CA:true
keyUsage               = critical,keyCertSign,cRLSign
subjectKeyIdentifier   = hash
authorityKeyIdentifier = keyid:always
$NC_LINE
EOF

gen_key "$KEY_TYPE" "$KEY_PARAM" "$KEY"
"$OPENSSL" req -new -x509 -config "$TMP/ca.cnf" -extensions v3_ca \
	-key "$KEY" -out "$CRT" -days "$DAYS" -"$(digest_for "$KEY_TYPE" "$KEY_PARAM")" \
	-set_serial "$(rand_serial)"
chmod 0644 "$CRT"

info "created CA in $CA_DIR"
cert_summary "$CRT" >&2
info "fingerprint: $("$OPENSSL" x509 -in "$CRT" -noout -fingerprint -sha256)"
info "protect $KEY (mode 0600); distribute only $CRT"
