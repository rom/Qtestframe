package auth

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/testutil/softauthn"
)

func testRP(t *testing.T) *RelyingParty {
	t.Helper()
	rp, err := NewRelyingParty("https://qtf.example.com", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return rp
}

func TestNewRelyingParty(t *testing.T) {
	rp := testRP(t)
	if rp.ID != "qtf.example.com" || len(rp.Origins) != 1 || rp.Origins[0] != "https://qtf.example.com" {
		t.Fatalf("%+v", rp)
	}
	rp, err := NewRelyingParty("https://qtf.example.com/", "example.com", []string{"https://qtf.example.com/", "https://other.example.com:8443"})
	if err != nil || rp.ID != "example.com" || len(rp.Origins) != 2 || !rp.originAllowed("https://OTHER.example.com:8443") {
		t.Fatalf("%+v %v", rp, err)
	}
	if _, err := NewRelyingParty("http://localhost:8080", "", nil); err != nil {
		t.Error(err)
	}
	bad := []struct {
		base, rp string
		origins  []string
	}{
		{"", "", nil},
		{"::", "", nil},
		{"http://qtf.example.com", "", nil},
		{"https://qtf.example.com", "other.com", nil},
		{"https://qtf.example.com", "example.com", []string{"https://evil.com"}},
		{"https://qtf.example.com", "", []string{"https://qtf.example.com/path"}},
		{"", "example.com", []string{"https://user@x.example.com"}},
		{"https://qtf.example.com", "http://example.com", nil},
		{"https://qtf.example.com", ".example.com", nil},
	}
	for _, b := range bad {
		if _, err := NewRelyingParty(b.base, b.rp, b.origins); err == nil {
			t.Errorf("accepted %+v", b)
		}
	}
	if !isLoopback("::1") || CheckOriginForRP("http://127.0.0.1:8080", "127.0.0.1") != nil {
		t.Error("loopback")
	}
}

func TestB64AndChallenge(t *testing.T) {
	c := NewChallenge()
	if len(c) != WebAuthnChallengeBytes {
		t.Fatal(len(c))
	}
	got, err := UnB64(B64(c) + "==")
	if err != nil || !bytes.Equal(got, c) {
		t.Fatal(err)
	}
	if bytes.Equal(NewChallenge(), c) {
		t.Error("challenge repeated")
	}
}

func register(t *testing.T, rp *RelyingParty, a *softauthn.Authenticator, cred *softauthn.Credential, o softauthn.Options, uv bool) (*NewCredential, error) {
	t.Helper()
	ch := NewChallenge()
	cd, att := a.Create(cred, ch, o)
	return rp.VerifyRegistration(RegistrationResponse{ClientDataJSON: cd, AttestationObject: att}, ch, uv)
}

func TestRegistrationAndAssertion(t *testing.T) {
	rp := testRP(t)
	a := softauthn.New("qtf.example.com", "https://qtf.example.com")
	for _, tc := range []struct {
		alg    int64
		format string
	}{{softauthn.ES256, ""}, {softauthn.ES256, "packed"}, {softauthn.ES256, "packed-x5c"}, {softauthn.RS256, "packed"}, {softauthn.EdDSA, ""}, {softauthn.EdDSA, "packed"}} {
		cred := softauthn.NewCredential(tc.alg, []byte("user-1"))
		cred.Counting = true
		nc, err := register(t, rp, a, cred, softauthn.Options{Format: tc.format, BE: true, BS: true}, true)
		if err != nil {
			t.Fatalf("%d %s: %v", tc.alg, tc.format, err)
		}
		wantFmt := "none"
		if tc.format != "" {
			wantFmt = "packed"
		}
		if nc.Alg != tc.alg || nc.Format != wantFmt || !bytes.Equal(nc.ID, cred.ID) || !nc.UserVerified || !nc.BackupEligible || !nc.BackedUp || len(nc.AAGUID) != 16 {
			t.Fatalf("%+v", nc)
		}
		// sign in twice; the counter increases
		count := nc.SignCount
		for i := 0; i < 2; i++ {
			ch := NewChallenge()
			cd, ad, sig := a.Get(cred, ch, softauthn.Options{})
			res, err := rp.VerifyAssertion(AssertionResponse{cd, ad, sig}, ch, nc.PublicKey, count, true)
			if err != nil {
				t.Fatalf("%d: %v", tc.alg, err)
			}
			if res.CounterRegressed || res.SignCount != count+1 || !res.UserVerified {
				t.Fatalf("%+v", res)
			}
			count = res.SignCount
		}
		// a replayed (or cloned) counter is reported, not refused
		cred.Counter = 1
		cred.Counting = false
		ch := NewChallenge()
		cd, ad, sig := a.Get(cred, ch, softauthn.Options{})
		res, err := rp.VerifyAssertion(AssertionResponse{cd, ad, sig}, ch, nc.PublicKey, count, true)
		if err != nil || !res.CounterRegressed {
			t.Fatalf("%+v %v", res, err)
		}
	}
	// no counter at all (synced passkeys): never a regression
	cred := softauthn.NewCredential(softauthn.ES256, nil)
	nc, err := register(t, rp, a, cred, softauthn.Options{}, false)
	if err != nil {
		t.Fatal(err)
	}
	ch := NewChallenge()
	cd, ad, sig := a.Get(cred, ch, softauthn.Options{})
	if res, err := rp.VerifyAssertion(AssertionResponse{cd, ad, sig}, ch, nc.PublicKey, 0, false); err != nil || res.CounterRegressed {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestRegistrationRejected(t *testing.T) {
	rp := testRP(t)
	a := softauthn.New("qtf.example.com", "https://qtf.example.com")
	cred := softauthn.NewCredential(softauthn.ES256, nil)
	cases := map[string]softauthn.Options{
		"type":                {Type: "webauthn.get"},
		"origin":              {Origin: "https://evil.example.net"},
		"challenge":           {Challenge: []byte("another challenge")},
		"cross-origin":        {CrossOrigin: true},
		"rp id":               {RPID: "evil.example.net"},
		"UP":                  {NoUP: true},
		"UV":                  {NoUV: true},
		"BS without BE":       {BS: true},
		"none with statement": {AttStmt: map[any]any{"x": int64(1)}},
		"packed bad sig":      {Format: "packed", AttStmt: map[any]any{"alg": softauthn.ES256, "sig": []byte{1, 2}}},
		"packed alg":          {Format: "packed", AttStmt: map[any]any{"alg": softauthn.RS256, "sig": []byte{1, 2}}},
		"packed no sig":       {Format: "packed", AttStmt: map[any]any{"alg": softauthn.ES256}},
		"packed ecdaa":        {Format: "packed", AttStmt: map[any]any{"alg": softauthn.ES256, "sig": []byte{1}, "ecdaaKeyId": []byte{1}}},
		"x5c empty":           {Format: "packed", AttStmt: map[any]any{"alg": softauthn.ES256, "sig": []byte{1}, "x5c": []any{}}},
		"x5c not bytes":       {Format: "packed", AttStmt: map[any]any{"alg": softauthn.ES256, "sig": []byte{1}, "x5c": []any{"x"}}},
		"x5c bad cert":        {Format: "packed", AttStmt: map[any]any{"alg": softauthn.ES256, "sig": []byte{1}, "x5c": []any{[]byte{1, 2}}}},
	}
	for name, o := range cases {
		if _, err := register(t, rp, a, cred, o, true); !errors.Is(err, ErrWebAuthn) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// an x5c signature by another key
	ch := NewChallenge()
	cd, att := a.Create(cred, ch, softauthn.Options{Format: "packed-x5c"})
	if _, err := rp.VerifyRegistration(RegistrationResponse{cd, att}, NewChallenge(), false); err == nil {
		t.Error("challenge of another ceremony")
	}
	// unsupported format, malformed objects
	ad := mustAuthData(t, a, cred)
	for name, obj := range map[string][]byte{
		"tpm":            softauthn.Encode(map[any]any{"fmt": "tpm", "attStmt": map[any]any{}, "authData": ad}),
		"no authData":    softauthn.Encode(map[any]any{"fmt": "none", "attStmt": map[any]any{}}),
		"not a map":      softauthn.Encode([]any{int64(1)}),
		"garbage":        {0xff},
		"trailing":       append(softauthn.Encode(map[any]any{"fmt": "none", "attStmt": map[any]any{}, "authData": ad}), 0),
		"no AT":          softauthn.Encode(map[any]any{"fmt": "none", "attStmt": map[any]any{}, "authData": ad[:37]}),
		"too long":       make([]byte, 70<<10),
		"short authData": softauthn.Encode(map[any]any{"fmt": "none", "attStmt": map[any]any{}, "authData": []byte{1}}),
	} {
		ch := NewChallenge()
		cd, _ := a.Create(cred, ch, softauthn.Options{})
		if name == "no AT" {
			// clear the AT flag
			b := append([]byte{}, ad[:37]...)
			b[32] &^= 0x40
			obj = softauthn.Encode(map[any]any{"fmt": "none", "attStmt": map[any]any{}, "authData": b})
		}
		if _, err := rp.VerifyRegistration(RegistrationResponse{cd, obj}, ch, false); !errors.Is(err, ErrWebAuthn) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// bad client data
	for _, cd := range [][]byte{nil, []byte("{"), []byte(`{"type":"webauthn.create","challenge":"***"}`), make([]byte, 5000)} {
		_, att := a.Create(cred, ch, softauthn.Options{})
		if _, err := rp.VerifyRegistration(RegistrationResponse{cd, att}, ch, false); !errors.Is(err, ErrWebAuthn) {
			t.Errorf("%q: %v", cd, err)
		}
	}
	if !strings.Contains(waErr("x %d", 1).Error(), "passkey verification failed: x 1") {
		t.Error("message")
	}
}

func mustAuthData(t *testing.T, a *softauthn.Authenticator, cred *softauthn.Credential) []byte {
	t.Helper()
	_, att := a.Create(cred, NewChallenge(), softauthn.Options{})
	v, err := cborDecodeAll(att)
	if err != nil {
		t.Fatal(err)
	}
	return v.(map[any]any)["authData"].([]byte)
}

func TestAssertionRejected(t *testing.T) {
	rp := testRP(t)
	a := softauthn.New("qtf.example.com", "https://qtf.example.com")
	cred := softauthn.NewCredential(softauthn.ES256, nil)
	nc, err := register(t, rp, a, cred, softauthn.Options{}, true)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]softauthn.Options{
		"type":      {Type: "webauthn.create"},
		"origin":    {Origin: "https://qtf.example.com.evil.net"},
		"challenge": {Challenge: []byte("x")},
		"rp id":     {RPID: "example.com"},
		"UP":        {NoUP: true},
		"UV":        {NoUV: true},
		"signature": {BadSignature: true},
	}
	for name, o := range cases {
		ch := NewChallenge()
		cd, ad, sig := a.Get(cred, ch, o)
		if _, err := rp.VerifyAssertion(AssertionResponse{cd, ad, sig}, ch, nc.PublicKey, 0, true); !errors.Is(err, ErrWebAuthn) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// UV is optional for a second factor
	ch := NewChallenge()
	cd, ad, sig := a.Get(cred, ch, softauthn.Options{NoUV: true})
	if res, err := rp.VerifyAssertion(AssertionResponse{cd, ad, sig}, ch, nc.PublicKey, 0, false); err != nil || res.UserVerified {
		t.Fatalf("%+v %v", res, err)
	}
	// another credential's key
	other := softauthn.NewCredential(softauthn.ES256, nil)
	if _, err := rp.VerifyAssertion(AssertionResponse{cd, ad, sig}, ch, other.COSEKey(), 0, false); err == nil {
		t.Error("other key")
	}
	// malformed authenticator data or key
	if _, err := rp.VerifyAssertion(AssertionResponse{cd, ad[:10], sig}, ch, nc.PublicKey, 0, false); err == nil {
		t.Error("short authData")
	}
	if _, err := rp.VerifyAssertion(AssertionResponse{cd, ad, sig}, ch, []byte{0xa0}, 0, false); err == nil {
		t.Error("empty key")
	}
	if _, err := rp.VerifyAssertion(AssertionResponse{cd, append(append([]byte{}, ad...), 1), sig}, ch, nc.PublicKey, 0, false); err == nil {
		t.Error("trailing bytes")
	}
}

func TestAuthDataExtensions(t *testing.T) {
	h := testRP(t).RPIDHash()
	b := append(append([]byte{}, h[:]...), flagUP|flagED, 0, 0, 0, 5)
	ext := softauthn.Encode(map[any]any{"credProtect": int64(2)})
	ad, err := ParseAuthData(append(append([]byte{}, b...), ext...))
	if err != nil || ad.Extensions["credProtect"] != int64(2) || ad.SignCount != 5 {
		t.Fatalf("%+v %v", ad, err)
	}
	if _, err := ParseAuthData(append(append([]byte{}, b...), softauthn.Encode([]any{})...)); err == nil {
		t.Error("extensions not a map")
	}
	if _, err := ParseAuthData(append(append([]byte{}, b...), 0xff)); err == nil {
		t.Error("bad extensions")
	}
	// attested data: bad lengths and keys
	at := append(append([]byte{}, h[:]...), flagUP|flagAT, 0, 0, 0, 0)
	for _, tail := range [][]byte{
		make([]byte, 10),
		append(make([]byte, 16), 0, 0),
		append(make([]byte, 16), 0x10, 0, 1),
		append(append(make([]byte, 16), 0, 1, 7), 0xff),
	} {
		if _, err := ParseAuthData(append(append([]byte{}, at...), tail...)); err == nil {
			t.Errorf("accepted %x", tail)
		}
	}
}

func TestParsePublicKey(t *testing.T) {
	for name, k := range map[string]map[any]any{
		"no alg":       {int64(1): int64(2)},
		"alg":          {int64(1): int64(2), int64(3): int64(-35)},
		"ec short":     {int64(1): int64(2), int64(3): COSEAlgES256, int64(-1): int64(1), int64(-2): []byte{1}, int64(-3): []byte{2}},
		"ec off curve": {int64(1): int64(2), int64(3): COSEAlgES256, int64(-1): int64(1), int64(-2): make([]byte, 32), int64(-3): make([]byte, 32)},
		"ec curve":     {int64(1): int64(2), int64(3): COSEAlgES256, int64(-1): int64(2), int64(-2): make([]byte, 32), int64(-3): make([]byte, 32)},
		"rsa small":    {int64(1): int64(3), int64(3): COSEAlgRS256, int64(-1): []byte{0xff, 0xff}, int64(-2): []byte{1, 0, 1}},
		"rsa e":        {int64(1): int64(3), int64(3): COSEAlgRS256, int64(-1): bytes.Repeat([]byte{0xff}, 256), int64(-2): []byte{2}},
		"rsa missing":  {int64(1): int64(3), int64(3): COSEAlgRS256},
		"okp":          {int64(1): int64(1), int64(3): COSEAlgEdDSA, int64(-1): int64(6), int64(-2): []byte{1}},
	} {
		if _, err := ParsePublicKey(softauthn.Encode(k)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := ParsePublicKey(softauthn.Encode([]any{})); err == nil {
		t.Error("array")
	}
	if _, err := ParsePublicKey([]byte{0x1c}); err == nil {
		t.Error("garbage")
	}
	if err := verifySig(-35, nil, nil, nil); err == nil {
		t.Error("unknown alg")
	}
}

func TestCBOR(t *testing.T) {
	ok := map[string]struct {
		in   []byte
		want any
	}{
		"uint8":   {[]byte{0x18, 200}, int64(200)},
		"uint16":  {[]byte{0x19, 1, 0}, int64(256)},
		"uint32":  {[]byte{0x1a, 0, 1, 0, 0}, int64(65536)},
		"uint64":  {[]byte{0x1b, 0, 0, 0, 1, 0, 0, 0, 0}, int64(1 << 32)},
		"neg":     {[]byte{0x38, 0x63}, int64(-100)},
		"text":    {[]byte{0x62, 'h', 'i'}, "hi"},
		"false":   {[]byte{0xf4}, false},
		"true":    {[]byte{0xf5}, true},
		"null":    {[]byte{0xf6}, nil},
		"undef":   {[]byte{0xf7}, nil},
		"tag":     {[]byte{0xc1, 0x01}, int64(1)},
		"half":    {[]byte{0xf9, 0x3c, 0x00}, 1.0},
		"half-":   {[]byte{0xf9, 0xc0, 0x00}, -2.0},
		"halfsub": {[]byte{0xf9, 0x00, 0x01}, math.Ldexp(1, -24)},
		"float32": {[]byte{0xfa, 0x3f, 0xc0, 0, 0}, 1.5},
		"float64": {[]byte{0xfb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0}, 1.5},
	}
	for name, c := range ok {
		v, err := cborDecodeAll(c.in)
		if err != nil || v != c.want {
			t.Errorf("%s: %v %v", name, v, err)
		}
	}
	if v, _ := cborDecodeAll([]byte{0xf9, 0x7c, 0x00}); !math.IsInf(v.(float64), 1) {
		t.Error("inf")
	}
	if v, _ := cborDecodeAll([]byte{0xf9, 0x7e, 0x00}); !math.IsNaN(v.(float64)) {
		t.Error("nan")
	}
	deep := bytes.Repeat([]byte{0x81}, 40)
	deep = append(deep, 0)
	many := []byte{0x9a}
	many = binary.BigEndian.AppendUint32(many, 5000)
	many = append(many, make([]byte, 5000)...)
	bad := map[string][]byte{
		"empty":       {},
		"short":       {0x19, 1},
		"short8":      {0x18},
		"short32":     {0x1a, 0},
		"short64":     {0x1b, 0},
		"indefinite":  {0x9f},
		"reserved":    {0x1c},
		"big":         {0x1b, 0xff, 0, 0, 0, 0, 0, 0, 0},
		"bigneg":      {0x3b, 0xff, 0, 0, 0, 0, 0, 0, 0},
		"bytes":       {0x45, 1},
		"text":        {0x65, 1},
		"array":       {0x85, 1},
		"array item":  {0x82, 1, 0x1c},
		"map":         {0xa5, 1},
		"map key":     {0xa1, 0x41, 1, 1},
		"map key err": {0xa1, 0x1c, 1},
		"map value":   {0xa1, 1, 0x1c},
		"duplicate":   {0xa2, 1, 1, 1, 2},
		"simple":      {0xf0},
		"deep":        deep,
		"items":       many,
		"trailing":    {1, 2},
	}
	for name, in := range bad {
		if _, err := cborDecodeAll(in); !errors.Is(err, errCBOR) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// maps and byte strings round-trip with the encoder of the tests
	in := map[any]any{"a": []byte{1, 2}, int64(-1): []any{true, nil, "x"}}
	v, err := cborDecodeAll(softauthn.Encode(in))
	if err != nil {
		t.Fatal(err)
	}
	m := v.(map[any]any)
	if !bytes.Equal(m["a"].([]byte), []byte{1, 2}) || m[int64(-1)].([]any)[2] != "x" {
		t.Fatal(m)
	}
}
