package auth

import (
	"encoding/base32"
	"net/url"
	"strings"
	"testing"
	"time"
)

// RFC 6238 appendix B (SHA-1 key "12345678901234567890"), last 6 digits.
func TestTOTPVectors(t *testing.T) {
	secret := base32.StdEncoding.EncodeToString([]byte("12345678901234567890"))
	for _, tc := range []struct {
		unix int64
		want string
	}{{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}, {20000000000, "353130"}} {
		got, err := TOTPCode(secret, TOTPStep(time.Unix(tc.unix, 0)))
		if err != nil || got != tc.want {
			t.Errorf("t=%d: %s %v, want %s", tc.unix, got, err, tc.want)
		}
	}
	// lower case, spaces and padding are tolerated
	if got, _ := TOTPCode(strings.ToLower(secret[:4])+" "+secret[4:]+"==", 1); got == "" {
		t.Error("lenient secret")
	}
	if _, err := TOTPCode("not base32!", 1); err != ErrBadSecret {
		t.Error(err)
	}
}

func TestVerifyTOTP(t *testing.T) {
	secret := NewTOTPSecret()
	if len(secret) != 32 {
		t.Fatal(secret)
	}
	now := time.Unix(1_800_000_000, 0)
	step := TOTPStep(now)
	code := func(s int64) string { c, _ := TOTPCode(secret, s); return c }
	for _, d := range []int64{-1, 0, 1} {
		got, ok := VerifyTOTP(secret, code(step+d), now, 0)
		if !ok || got != step+d {
			t.Errorf("drift %d: %d %v", d, got, ok)
		}
	}
	for _, d := range []int64{-2, 2} {
		if _, ok := VerifyTOTP(secret, code(step+d), now, 0); ok {
			t.Errorf("drift %d accepted", d)
		}
	}
	// replay: the step of the last accepted code and older ones are rejected
	if _, ok := VerifyTOTP(secret, code(step), now, step); ok {
		t.Error("replay accepted")
	}
	if _, ok := VerifyTOTP(secret, code(step-1), now, step-1); ok {
		t.Error("older step accepted")
	}
	if got, ok := VerifyTOTP(secret, code(step+1), now, step); !ok || got != step+1 {
		t.Error("next step rejected")
	}
	// formatting the user may type
	c := code(step)
	if _, ok := VerifyTOTP(secret, c[:3]+" "+c[3:], now, 0); !ok {
		t.Error("spaced code rejected")
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef", "12345a"} {
		if _, ok := VerifyTOTP(secret, bad, now, 0); ok {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, ok := VerifyTOTP("!!", "123456", now, 0); ok {
		t.Error("bad secret accepted")
	}
}

func TestOTPAuthURI(t *testing.T) {
	u, err := url.Parse(OTPAuthURI("Qtestframe", "anna@example.com", "ABC"))
	if err != nil || u.Scheme != "otpauth" || u.Host != "totp" || u.Path != "/Qtestframe:anna@example.com" {
		t.Fatal(u, err)
	}
	q := u.Query()
	if q.Get("secret") != "ABC" || q.Get("issuer") != "Qtestframe" || q.Get("digits") != "6" || q.Get("period") != "30" || q.Get("algorithm") != "SHA1" {
		t.Fatal(q)
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes := NewRecoveryCodes(RecoveryCodeCount)
	seen := map[string]bool{}
	for _, c := range codes {
		if len(c) != 14 || c[4] != '-' || c[9] != '-' || !IsRecoveryCode(c) || seen[c] {
			t.Fatal(c)
		}
		seen[c] = true
		if HashRecoveryCode(c) != HashRecoveryCode(" "+strings.ToUpper(strings.ReplaceAll(c, "-", ""))+" ") {
			t.Error("normalisation")
		}
	}
	if len(codes) != 10 || IsRecoveryCode("123456") {
		t.Fatal(codes)
	}
	if NormalizeCode(" 12 3-45\t6 ") != "123456" {
		t.Error(NormalizeCode(" 12 3-45\t6 "))
	}
}
