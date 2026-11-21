package auth

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestMain(m *testing.M) {
	BcryptCost = bcrypt.MinCost
	m.Run()
}

func TestValidatePasswordMin(t *testing.T) {
	if err := ValidatePasswordMin("abcdefgh-12", 12); !errors.Is(err, ErrWeakPassword) || !strings.Contains(err.Error(), "at least 12") {
		t.Errorf("short: %v", err)
	}
	if err := ValidatePasswordMin("abcdefghijklm", 12); !errors.Is(err, ErrWeakPassword) {
		t.Errorf("letters only: %v", err)
	}
	if ValidatePasswordMin("abcdefgh-12", 0) != nil || ValidatePasswordMin("abcdefghij-12", 12) != nil || ValidatePasswordMin("short1", 0) != ErrWeakPassword {
		t.Error("valid passwords")
	}
}

func TestValidatePassword(t *testing.T) {
	cases := map[string]bool{
		"":                             false,
		"short1":                       false,
		"abcdefghij":                   false, // letters only
		"1234567890":                   false, // digits only
		"!!!!!!!!!!":                   false,
		"abcdefghi1":                   true,
		"abcdefghi!":                   true,
		"päßwörtäöü1":                  true,
		"correct horse battery":        true, // space counts as "other"
		strings.Repeat("a1", 36):       true, // 72 bytes
		strings.Repeat("a1", 36) + "x": false,
	}
	for p, ok := range cases {
		err := ValidatePassword(p)
		if (err == nil) != ok {
			t.Errorf("ValidatePassword(%q) = %v, want ok=%v", p, err, ok)
		}
		if err != nil && !errors.Is(err, ErrWeakPassword) {
			t.Errorf("unexpected error type %v", err)
		}
	}
}

func TestHashAndCheckPassword(t *testing.T) {
	h, err := HashPassword("abcdefghi1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$2") {
		t.Errorf("not bcrypt: %q", h)
	}
	if !CheckPassword(h, "abcdefghi1") || CheckPassword(h, "abcdefghi2") {
		t.Error("CheckPassword mismatch")
	}
	if CheckPassword("", "anything") {
		t.Error("empty hash matched")
	}
	if CheckPassword("not-a-hash", "x") {
		t.Error("garbage hash matched")
	}
	if _, err := HashPassword(strings.Repeat("x", 80)); err == nil {
		t.Error("bcrypt should reject >72 byte passwords")
	}
}

func TestTokens(t *testing.T) {
	a, b := RandomToken(16), RandomToken(16)
	if a == b || len(a) != base64.RawURLEncoding.EncodedLen(16) {
		t.Errorf("RandomToken: %q %q", a, b)
	}
	if h := HashToken("x"); len(h) != 64 || h != HashToken("x") || h == HashToken("y") {
		t.Errorf("HashToken: %q", h)
	}
	tok, prefix := NewAPIToken()
	if !IsAPIToken(tok) || !strings.HasPrefix(tok, prefix) || len(prefix) != len(APITokenPrefix)+6 {
		t.Errorf("NewAPIToken: %q %q", tok, prefix)
	}
	if IsAPIToken("Bearer xyz") {
		t.Error("IsAPIToken false positive")
	}
}

func TestSigner(t *testing.T) {
	s := NewSigner([]byte("k"))
	type payload struct{ A string }
	tok, err := s.Sign(payload{A: "hello"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var p payload
	if err := s.Verify(tok, &p); err != nil || p.A != "hello" {
		t.Fatalf("Verify: %v %+v", err, p)
	}
	// Different key.
	if err := NewSigner([]byte("other")).Verify(tok, &p); !errors.Is(err, ErrBadState) {
		t.Errorf("other key: %v", err)
	}
	// Random key signers differ.
	if err := NewSigner(nil).Verify(tok, &p); !errors.Is(err, ErrBadState) {
		t.Errorf("random key: %v", err)
	}
	// Tampered payload / signature / format.
	payloadPart, sig, _ := strings.Cut(tok, ".")
	for name, bad := range map[string]string{
		"no dot":   payloadPart,
		"bad sig":  payloadPart + "." + sig[:len(sig)-2] + "AA",
		"tampered": "x" + payloadPart + "." + sig,
		"empty":    "",
	} {
		if err := s.Verify(bad, &p); !errors.Is(err, ErrBadState) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Correctly MAC'd but invalid base64 / JSON / data.
	sign := func(payload string) string { return payload + "." + s.mac(payload) }
	enc := base64.RawURLEncoding.EncodeToString
	for name, bad := range map[string]string{
		"bad base64": sign("!!!"),
		"bad json":   sign(enc([]byte("{"))),
		"bad data":   sign(enc([]byte(`{"exp":9999999999,"d":"str"}`))),
	} {
		if err := s.Verify(bad, &p); !errors.Is(err, ErrBadState) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Expired.
	old, _ := s.Sign(payload{A: "x"}, -2*time.Second)
	if err := s.Verify(old, &p); !errors.Is(err, ErrBadState) {
		t.Errorf("expired: %v", err)
	}
	// Unmarshalable value.
	if _, err := s.Sign(make(chan int), time.Minute); err == nil {
		t.Error("Sign(chan) should fail")
	}
}

func TestMapRole(t *testing.T) {
	rank := func(r string) int { return map[string]int{"admin": 4, "manager": 3, "tester": 2, "viewer": 1}[r] }
	m := map[string]string{"admins": "admin", "qa": "tester", "pm": "manager", "weird": "unknown"}
	cases := []struct {
		groups []string
		def    string
		want   string
	}{
		{nil, "viewer", "viewer"},
		{[]string{"qa"}, "viewer", "tester"},
		{[]string{"qa", "admins", "pm"}, "viewer", "admin"},
		{[]string{"qa"}, "manager", "manager"}, // never downgrade below default
		{[]string{"weird", "other"}, "viewer", "viewer"},
	}
	for _, c := range cases {
		if got := MapRole(c.groups, m, c.def, rank); got != c.want {
			t.Errorf("MapRole(%v, %s) = %s, want %s", c.groups, c.def, got, c.want)
		}
	}
}
