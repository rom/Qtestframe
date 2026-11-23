// Package auth provides password hashing, opaque tokens, signed state
// cookies and external identity providers (OIDC and SAML).
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

// BcryptCost is the work factor for password hashes.
var BcryptCost = 12

// MinPasswordLength is the minimum accepted password length.
const MinPasswordLength = 10

// ErrWeakPassword is returned for passwords violating the policy.
var ErrWeakPassword = errors.New("password must be at least 10 characters and contain letters and digits or symbols")

// weakPassword is ErrWeakPassword for another minimum length.
type weakPassword struct{ min int }

func (e weakPassword) Error() string {
	return fmt.Sprintf("password must be at least %d characters and contain letters and digits or symbols", e.min)
}

func (e weakPassword) Is(target error) bool { return target == ErrWeakPassword }

// ValidatePassword enforces a basic password policy.
func ValidatePassword(p string) error { return ValidatePasswordMin(p, MinPasswordLength) }

// PasswordPolicy is the policy of local passwords: the minimum length
// (auth.password_min_length) and the number of character classes
// (lower-case, upper-case, digit, symbol) a password must use
// (auth.password_min_classes; 0 or 1 = letters and a digit or symbol).
type PasswordPolicy struct {
	MinLength  int
	MinClasses int
}

// weakClasses is ErrWeakPassword for too few character classes.
type weakClasses struct{ min, classes int }

func (e weakClasses) Error() string {
	return fmt.Sprintf("password must be at least %d characters and use at least %d of: lower-case letters, upper-case letters, digits, symbols", e.min, e.classes)
}

func (e weakClasses) Is(target error) bool { return target == ErrWeakPassword }

// CharClasses counts the character classes of p (lower-case, upper-case,
// digit, other).
func CharClasses(p string) int {
	var lower, upper, digit, other bool
	for _, r := range p {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		case unicode.IsLetter(r):
			lower = true // letters without case
		default:
			other = true
		}
	}
	n := 0
	for _, b := range []bool{lower, upper, digit, other} {
		if b {
			n++
		}
	}
	return n
}

// Validate checks p against the policy. Its errors match ErrWeakPassword.
func (pol PasswordPolicy) Validate(p string) error {
	if err := ValidatePasswordMin(p, pol.MinLength); err != nil {
		return err
	}
	if pol.MinClasses > 1 && CharClasses(p) < pol.MinClasses {
		min := pol.MinLength
		if min < 1 {
			min = MinPasswordLength
		}
		return weakClasses{min, pol.MinClasses}
	}
	return nil
}

// ErrPasswordReused is returned for a password that was used before.
var ErrPasswordReused = errors.New("this password was used before: choose another one")

// Reused reports whether p matches one of the earlier hashes (the newest
// n of them).
func Reused(p string, hashes []string, n int) bool {
	for i, h := range hashes {
		if i >= n {
			break
		}
		if CheckPassword(h, p) {
			return true
		}
	}
	return false
}

// ValidatePasswordMin enforces the policy with a minimum length of min
// (auth.password_min_length; bcrypt limits passwords to 72 bytes). Its
// errors match ErrWeakPassword with errors.Is.
func ValidatePasswordMin(p string, min int) error {
	if min < 1 {
		min = MinPasswordLength
	}
	if len(p) < min || len(p) > 72 {
		if min == MinPasswordLength {
			return ErrWeakPassword
		}
		return weakPassword{min}
	}
	var letter, other bool
	for _, r := range p {
		if unicode.IsLetter(r) {
			letter = true
		} else {
			other = true
		}
	}
	if !letter || !other {
		if min == MinPasswordLength {
			return ErrWeakPassword
		}
		return weakPassword{min}
	}
	return nil
}

// HashPassword hashes a password with bcrypt.
func HashPassword(p string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(p), BcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// dummyHash is compared against when a user does not exist so that failed
// logins for unknown accounts take as long as for real ones. It is created
// lazily with the configured cost (tests lower BcryptCost before first use).
var (
	dummyOnce sync.Once
	dummyHash []byte
)

func dummy() []byte {
	dummyOnce.Do(func() {
		dummyHash, _ = bcrypt.GenerateFromPassword([]byte("qtestframe-dummy-password"), BcryptCost)
	})
	return dummyHash
}

// CheckPassword reports whether p matches hash. An empty hash never matches
// but still costs a bcrypt comparison.
func CheckPassword(hash, p string) bool {
	if hash == "" {
		_ = bcrypt.CompareHashAndPassword(dummy(), []byte(p))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil
}

// RandomToken returns n random bytes encoded as base64url.
func RandomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("auth: crypto/rand failed: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken returns the hex SHA-256 of an opaque token; only hashes are
// stored in the database.
func HashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// APITokenPrefix prefixes all API tokens to make them recognisable by
// secret scanners.
const APITokenPrefix = "qtf_"

// NewAPIToken returns a new API token and its display prefix.
func NewAPIToken() (token, prefix string) {
	token = APITokenPrefix + RandomToken(32)
	return token, token[:len(APITokenPrefix)+6]
}

// IsAPIToken reports whether s looks like an API token.
func IsAPIToken(s string) bool { return strings.HasPrefix(s, APITokenPrefix) }

// Signer produces and verifies HMAC-signed, expiring JSON payloads used
// for short-lived login state cookies.
type Signer struct{ key []byte }

// NewSigner returns a signer for key; an empty key generates a random one.
func NewSigner(key []byte) *Signer {
	if len(key) == 0 {
		key = []byte(RandomToken(32))
	}
	return &Signer{key: key}
}

type envelope struct {
	Exp  int64           `json:"exp"`
	Data json.RawMessage `json:"d"`
}

// Sign encodes v with an expiry.
func (s *Signer) Sign(v any, ttl time.Duration) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	env, _ := json.Marshal(envelope{Exp: time.Now().Add(ttl).Unix(), Data: data})
	payload := base64.RawURLEncoding.EncodeToString(env)
	return payload + "." + s.mac(payload), nil
}

func (s *Signer) mac(p string) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(p))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// ErrBadState is returned for tampered or expired state values.
var ErrBadState = errors.New("invalid or expired state")

// Verify decodes a signed value into v.
func (s *Signer) Verify(token string, v any) error {
	payload, sig, ok := strings.Cut(token, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(s.mac(payload))) {
		return ErrBadState
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return ErrBadState
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return ErrBadState
	}
	if time.Now().Unix() > env.Exp {
		return ErrBadState
	}
	if err := json.Unmarshal(env.Data, v); err != nil {
		return ErrBadState
	}
	return nil
}

// Identity is a user asserted by an external identity provider.
type Identity struct {
	Provider string   // "oidc:<name>" or "saml:<name>"
	Subject  string   // stable subject identifier
	Email    string   // e-mail address
	Name     string   // display name
	Groups   []string // group memberships for role mapping
	// AMR and ACR are the authentication methods and context class the
	// identity provider reported (OIDC amr/acr claims; empty for SAML).
	AMR []string
	ACR string
}

// MapRoles returns every role granted by groups through mapping that valid
// accepts (a user in several mapped groups holds all their roles), in
// first-seen order; def alone when no group maps to a valid role.
func MapRoles(groups []string, mapping map[string]string, def string, valid func(string) bool) []string {
	var out []string
	seen := map[string]bool{}
	add := func(list string) {
		for _, r := range SplitRoles(list) {
			if valid(r) && !seen[r] {
				seen[r] = true
				out = append(out, r)
			}
		}
	}
	for _, g := range groups {
		if r, ok := mapping[g]; ok {
			add(r)
		}
	}
	if len(out) == 0 {
		add(def)
	}
	if len(out) == 0 {
		return []string{def}
	}
	return out
}

// SplitRoles splits a role list of a mapping or default role ("tester" or
// "tester, reviewer") into its roles.
func SplitRoles(s string) []string {
	var out []string
	for _, r := range strings.Split(s, ",") {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	return out
}

// MapRole returns the most privileged role granted by groups through
// mapping, falling back to def. rank orders roles by privilege.
func MapRole(groups []string, mapping map[string]string, def string, rank func(string) int) string {
	role := def
	for _, g := range groups {
		if r, ok := mapping[g]; ok && rank(r) > rank(role) {
			role = r
		}
	}
	return role
}
