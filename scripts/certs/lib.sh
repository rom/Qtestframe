# shellcheck shell=bash
# Shared helpers for the Qtestframe certificate scripts.
# Sourced, not executed. Portable across Linux (OpenSSL 1.1/3.x) and macOS
# (LibreSSL or Homebrew OpenSSL) with bash >= 3.2: no associative arrays,
# no `sed -i`, no GNU-only `date`, no `openssl -addext` (config files only).

QTF_PROG=${QTF_PROG:-$(basename "$0")}
OPENSSL=${OPENSSL:-openssl}

die() {
	printf '%s: error: %s\n' "$QTF_PROG" "$*" >&2
	exit 1
}

info() {
	printf '%s: %s\n' "$QTF_PROG" "$*" >&2
}

require_openssl() {
	command -v "$OPENSSL" >/dev/null 2>&1 || die "openssl not found (set OPENSSL=/path/to/openssl)"
}

# Print "LibreSSL" or "OpenSSL".
openssl_flavor() {
	case "$("$OPENSSL" version 2>/dev/null)" in
	LibreSSL*) echo LibreSSL ;;
	*) echo OpenSSL ;;
	esac
}

# Positive random 128-bit serial in the form 0x...
rand_serial() {
	printf '0x0%s' "$("$OPENSSL" rand -hex 15)"
}

# Integer check.
is_uint() {
	case "$1" in
	'' | *[!0-9]*) return 1 ;;
	*) return 0 ;;
	esac
}

# Restrict strings written into openssl config files to a safe character
# set (no "$", quotes, newlines, ...).
check_dn_value() {
	case "$1" in
	'') die "empty value for $2" ;;
	*[!A-Za-z0-9\ ._@*:/,+-]*) die "invalid characters in $2: '$1' (allowed: letters, digits, space . _ @ * : / , + -)" ;;
	esac
}

# Validate and normalise a SAN list "DNS:a,DNS:b,IP:1.2.3.4,email:x@y,URI:u".
# Prints the normalised list.
normalize_san() {
	local list=$1 out='' item type value
	local IFS=','
	# shellcheck disable=SC2086 # intentional word splitting on commas
	set -- $list
	for item in "$@"; do
		# trim spaces
		item=$(printf '%s' "$item" | tr -d ' ')
		[ -n "$item" ] || continue
		type=${item%%:*}
		value=${item#*:}
		[ "$type" != "$item" ] || die "SAN entry '$item' needs a type prefix (DNS:, IP:, email:, URI:)"
		case "$type" in
		DNS | dns) type=DNS ;;
		IP | ip) type=IP ;;
		email | EMAIL) type=email ;;
		URI | uri) type=URI ;;
		*) die "unsupported SAN type '$type' in '$item'" ;;
		esac
		check_dn_value "$value" "SAN"
		out="${out:+$out,}$type:$value"
	done
	printf '%s' "$out"
}

# san_contains LIST ENTRY
san_contains() {
	case ",$1," in
	*",$2,"*) return 0 ;;
	*) return 1 ;;
	esac
}

# gen_key TYPE BITS_OR_CURVE OUTFILE
#   TYPE=ec  -> curve name (P-256, P-384, prime256v1, secp384r1)
#   TYPE=rsa -> key size
# Writes an unencrypted PKCS#8 PEM key with mode 0600.
gen_key() {
	local type=$1 param=$2 out=$3 tmp curve
	tmp="$out.tmp.$$"
	(
		umask 077
		case "$type" in
		ec)
			case "$param" in
			P-256 | p256 | prime256v1 | secp256r1) curve=prime256v1 ;;
			P-384 | p384 | secp384r1) curve=secp384r1 ;;
			*) die "unsupported EC curve '$param' (P-256, P-384)" ;;
			esac
			"$OPENSSL" ecparam -name "$curve" -genkey -noout -out "$tmp.sec1"
			;;
		rsa)
			is_uint "$param" || die "RSA key size must be a number"
			[ "$param" -ge 2048 ] || die "RSA keys must be at least 2048 bits"
			"$OPENSSL" genrsa -out "$tmp.sec1" "$param" 2>/dev/null
			;;
		*) die "unknown key type '$type'" ;;
		esac
		"$OPENSSL" pkcs8 -topk8 -nocrypt -in "$tmp.sec1" -out "$tmp"
		rm -f "$tmp.sec1"
		chmod 0600 "$tmp"
		mv -f "$tmp" "$out"
	) || {
		rm -f "$tmp" "$tmp.sec1"
		die "key generation failed"
	}
}

# Digest to sign with: sha384 for P-384, sha256 otherwise.
digest_for() {
	case "$1:$2" in
	ec:P-384 | ec:p384 | ec:secp384r1) echo sha384 ;;
	*) echo sha256 ;;
	esac
}

# cert_valid_for FILE SECONDS -> true if FILE is a certificate valid for at
# least SECONDS more seconds.
cert_valid_for() {
	[ -f "$1" ] && "$OPENSSL" x509 -in "$1" -noout -checkend "$2" >/dev/null 2>&1
}

# key_matches_cert KEY CERT
key_matches_cert() {
	local a b
	a=$("$OPENSSL" pkey -in "$1" -pubout 2>/dev/null | "$OPENSSL" sha256 2>/dev/null) || return 1
	b=$("$OPENSSL" x509 -in "$2" -noout -pubkey 2>/dev/null | "$OPENSSL" sha256 2>/dev/null) || return 1
	[ -n "$a" ] && [ "$a" = "$b" ]
}

# make_tmpdir -> prints a private temporary directory (caller removes it).
make_tmpdir() {
	mktemp -d "${TMPDIR:-/tmp}/qtf-certs.XXXXXX"
}

# set_group_readable FILE GROUP: chgrp + 0640 (used for files the service
# group must read, e.g. /etc/qtestframe/tls/server.key).
set_group_readable() {
	chgrp "$2" "$1" || die "chgrp $2 $1 failed (run as root?)"
	chmod 0640 "$1"
}

# Print a short summary of a certificate.
cert_summary() {
	"$OPENSSL" x509 -in "$1" -noout -subject -issuer -serial -startdate -enddate
	"$OPENSSL" x509 -in "$1" -noout -text 2>/dev/null |
		awk '/X509v3 Subject Alternative Name/ {getline; gsub(/^ +/, ""); print "SAN=" $0}'
}
