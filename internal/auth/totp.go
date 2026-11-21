package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" // RFC 6238 default; what authenticator apps support
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP parameters (RFC 6238): HMAC-SHA-1, 6 digits, 30 s steps. Codes of
// the previous and the next step are accepted too (clock drift).
const (
	TOTPDigits = 6
	TOTPPeriod = 30
	TOTPSkew   = 1
	// totpSecretBytes is the key length (160 bits, RFC 4226 recommendation).
	totpSecretBytes = 20
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns a random TOTP key, base32 encoded without padding.
func NewTOTPSecret() string {
	b := make([]byte, totpSecretBytes)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("auth: crypto/rand failed: %v", err))
	}
	return b32.EncodeToString(b)
}

// ErrBadSecret is returned for a TOTP key that is not base32.
var ErrBadSecret = errors.New("invalid TOTP secret")

func decodeSecret(secret string) ([]byte, error) {
	s := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	s = strings.TrimRight(s, "=")
	key, err := b32.DecodeString(s)
	if err != nil || len(key) == 0 {
		return nil, ErrBadSecret
	}
	return key, nil
}

// TOTPStep returns the time step of t.
func TOTPStep(t time.Time) int64 { return t.Unix() / TOTPPeriod }

// TOTPCode returns the code of secret for a time step (RFC 4226 HOTP with
// the step as counter).
func TOTPCode(secret string, step int64) (string, error) {
	key, err := decodeSecret(secret)
	if err != nil {
		return "", err
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", v%1000000), nil
}

// NormalizeCode removes spaces and dashes users type or paste.
func NormalizeCode(code string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '\t' || r == ' ' {
			return -1
		}
		return r
	}, strings.TrimSpace(code))
}

// VerifyTOTP checks code against secret at time t, accepting ±TOTPSkew
// steps. Steps up to and including lastStep are rejected (each code is
// accepted once). It returns the matched step.
func VerifyTOTP(secret, code string, t time.Time, lastStep int64) (int64, bool) {
	code = NormalizeCode(code)
	if len(code) != TOTPDigits {
		return 0, false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	now := TOTPStep(t)
	matched, found := int64(0), false
	// check every candidate (no early exit) so the timing does not tell
	// which step matched
	for d := int64(-TOTPSkew); d <= TOTPSkew; d++ {
		step := now + d
		want, err := TOTPCode(secret, step)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 && step > lastStep && !found {
			matched, found = step, true
		}
	}
	return matched, found
}

// OTPAuthURI returns the otpauth:// URI authenticator apps read from the
// enrolment QR code (Key Uri Format).
func OTPAuthURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(TOTPDigits))
	q.Set("period", fmt.Sprint(TOTPPeriod))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// Recovery codes: RecoveryCodeCount codes of 12 characters (60 bits) from
// an alphabet without look-alike characters, shown as xxxx-xxxx-xxxx.
const (
	RecoveryCodeCount = 10
	recoveryAlphabet  = "abcdefghjkmnpqrstuvwxyz23456789"
	recoveryLen       = 12
)

// NewRecoveryCodes returns n random recovery codes.
func NewRecoveryCodes(n int) []string {
	out := make([]string, n)
	for i := range out {
		b := make([]byte, recoveryLen)
		if _, err := rand.Read(b); err != nil {
			panic(fmt.Sprintf("auth: crypto/rand failed: %v", err))
		}
		var sb strings.Builder
		for j, x := range b {
			if j > 0 && j%4 == 0 {
				sb.WriteByte('-')
			}
			// 256 % 31 != 0: the tiny bias is irrelevant at 60 bits
			sb.WriteByte(recoveryAlphabet[int(x)%len(recoveryAlphabet)])
		}
		out[i] = sb.String()
	}
	return out
}

// NormalizeRecoveryCode lowercases a recovery code and removes separators.
func NormalizeRecoveryCode(code string) string {
	return strings.ToLower(NormalizeCode(code))
}

// IsRecoveryCode reports whether code has the shape of a recovery code
// (rather than a 6-digit TOTP code).
func IsRecoveryCode(code string) bool {
	return len(NormalizeRecoveryCode(code)) == recoveryLen
}

// HashRecoveryCode returns the stored form of a recovery code.
func HashRecoveryCode(code string) string {
	return HashToken("qtf-recovery:" + NormalizeRecoveryCode(code))
}
