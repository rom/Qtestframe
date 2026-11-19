#!/usr/bin/env bash
# Generate the SAML service-provider signing/encryption key pair for a
# Qtestframe SAML provider (auth.saml[].sp_cert_file / sp_key_file).
#
# SAML metadata certificates are trust anchors exchanged out-of-band with
# the IdP (ADFS, Keycloak, Entra ID, ...), so a self-signed certificate with
# a long lifetime is the norm; the IdP pins it from the SP metadata.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source-path=SCRIPTDIR source=lib.sh
. "$SCRIPT_DIR/lib.sh"

CN=""
OUT=./certs/saml
NAME=saml-sp
DAYS=1825
BITS=3072
KEY_TYPE=rsa
GROUP=""
FORCE=0

usage() {
	cat <<EOF
Usage: $QTF_PROG --cn ENTITY-HOST [options]

Create <name>.key / <name>.crt for SAML request signing and assertion
decryption. RSA is used by default because several IdPs (notably ADFS)
do not support ECDSA XML signatures.

Options:
  --cn NAME        subject CN, usually the public host (qtf.example.com)
  --out DIR        output directory (default $OUT)
  --name BASE      file base name (default "$NAME")
  --days N         validity (default $DAYS = 5 years)
  --rsa BITS       RSA key size (default $BITS)
  --ec             ECDSA P-256 instead (only if your IdP supports it)
  --group G        make the key readable by group G (0640), e.g. qtestframe
  --force          replace an existing key pair (IdP must re-import metadata!)
  -h, --help       show this help

Example:
  $QTF_PROG --cn qtf.example.com --name saml-adfs --out /etc/qtestframe/tls --group qtestframe
Then set in config.yaml:
  sp_cert_file: /etc/qtestframe/tls/saml-adfs.crt
  sp_key_file:  /etc/qtestframe/tls/saml-adfs.key
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--cn) CN=${2:?--cn needs a value}; shift ;;
	--out) OUT=${2:?--out needs a value}; shift ;;
	--name) NAME=${2:?--name needs a value}; shift ;;
	--days) DAYS=${2:?--days needs a value}; shift ;;
	--rsa) KEY_TYPE=rsa; BITS=${2:?--rsa needs a key size}; shift ;;
	--ec) KEY_TYPE=ec ;;
	--group) GROUP=${2:?--group needs a value}; shift ;;
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

KEY="$OUT/$NAME.key"
CRT="$OUT/$NAME.crt"

if [ "$FORCE" -eq 0 ] && [ -f "$KEY" ] && [ -f "$CRT" ]; then
	if key_matches_cert "$KEY" "$CRT"; then
		if cert_valid_for "$CRT" $((60 * 86400)); then
			info "$CRT exists and is valid; nothing to do (--force to replace)"
		else
			info "WARNING: $CRT expires within 60 days; plan a rollover with --force and re-import the SP metadata at the IdP"
		fi
		exit 0
	fi
	die "$KEY does not match $CRT; use --force to replace both"
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

[ v3_saml ]
basicConstraints     = critical,CA:false
keyUsage             = critical,digitalSignature$([ "$KEY_TYPE" = rsa ] && printf ',keyEncipherment')
subjectKeyIdentifier = hash
EOF

if [ "$KEY_TYPE" = rsa ]; then
	gen_key rsa "$BITS" "$TMP/new.key"
else
	gen_key ec P-256 "$TMP/new.key"
fi
"$OPENSSL" req -new -x509 -config "$TMP/req.cnf" -extensions v3_saml \
	-key "$TMP/new.key" -out "$TMP/new.crt" -days "$DAYS" -sha256 -set_serial "$(rand_serial)"

(
	umask 077
	cp "$TMP/new.key" "$KEY.tmp.$$" && chmod 0600 "$KEY.tmp.$$" && mv -f "$KEY.tmp.$$" "$KEY"
)
cp "$TMP/new.crt" "$CRT.tmp.$$" && chmod 0644 "$CRT.tmp.$$" && mv -f "$CRT.tmp.$$" "$CRT"
if [ -n "$GROUP" ]; then set_group_readable "$KEY" "$GROUP"; fi

info "created SAML SP key pair $KEY / $CRT"
cert_summary "$CRT" >&2
