// Package softauthn is a software WebAuthn authenticator for tests: it
// generates real key pairs and builds the clientDataJSON, attestation
// objects, authenticator data and signatures a browser and an
// authenticator would send, so that the relying-party code is tested
// end to end without a browser.
package softauthn

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math/big"
	"sort"
	"time"
)

// COSE algorithms.
const (
	ES256 int64 = -7
	EdDSA int64 = -8
	RS256 int64 = -257
)

// Authenticator is a software authenticator for one RP ID and origin.
type Authenticator struct {
	RPID   string
	Origin string
	// AAGUID of the authenticator model (16 bytes; zero by default).
	AAGUID []byte
}

// New returns an authenticator for rpID used from origin.
func New(rpID, origin string) *Authenticator {
	return &Authenticator{RPID: rpID, Origin: origin, AAGUID: make([]byte, 16)}
}

// Credential is a key pair of the authenticator.
type Credential struct {
	ID         []byte
	UserHandle []byte
	Alg        int64
	// Counter is the signature counter; Counting makes every assertion
	// increase it (false: always 0, like synced passkeys).
	Counter  uint32
	Counting bool
	signer   crypto.Signer
}

// Options change what the authenticator and the client send (for
// negative tests); the zero value is a correct ceremony with UP and UV.
type Options struct {
	Type        string // clientData type ("" = the right one)
	Origin      string // "" = the authenticator's
	Challenge   []byte // nil = the given one
	CrossOrigin bool
	RPID        string // for the RP ID hash ("" = the authenticator's)
	NoUP, NoUV  bool
	BE, BS      bool
	// Format of the attestation: "none" (default), "packed" (self) or
	// "packed-x5c" (a self-signed attestation certificate).
	Format string
	// BadSignature corrupts the signature.
	BadSignature bool
	// AttStmt replaces the attestation statement.
	AttStmt map[any]any
}

// NewCredential creates a key pair for alg (ES256, RS256 or EdDSA).
func NewCredential(alg int64, userHandle []byte) *Credential {
	c := &Credential{ID: random(32), UserHandle: userHandle, Alg: alg}
	var err error
	switch alg {
	case RS256:
		c.signer, err = rsa.GenerateKey(rand.Reader, 2048)
	case EdDSA:
		_, c.signer, err = ed25519.GenerateKey(rand.Reader)
	default:
		c.Alg = ES256
		c.signer, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	}
	if err != nil {
		panic(err)
	}
	return c
}

func random(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

// COSEKey returns the credential's public key as a COSE_Key.
func (c *Credential) COSEKey() []byte {
	switch k := c.signer.Public().(type) {
	case *ecdsa.PublicKey:
		raw, err := k.Bytes()
		if err != nil {
			panic(err)
		}
		return Encode(map[any]any{int64(1): int64(2), int64(3): ES256, int64(-1): int64(1), int64(-2): raw[1:33], int64(-3): raw[33:]})
	case *rsa.PublicKey:
		return Encode(map[any]any{int64(1): int64(3), int64(3): RS256, int64(-1): k.N.Bytes(), int64(-2): big.NewInt(int64(k.E)).Bytes()})
	case ed25519.PublicKey:
		return Encode(map[any]any{int64(1): int64(1), int64(3): EdDSA, int64(-1): int64(6), int64(-2): []byte(k)})
	}
	panic("unknown key")
}

func (c *Credential) sign(data []byte) []byte {
	var sig []byte
	var err error
	if c.Alg == EdDSA {
		sig, err = c.signer.Sign(rand.Reader, data, crypto.Hash(0))
	} else {
		sum := sha256.Sum256(data)
		sig, err = c.signer.Sign(rand.Reader, sum[:], crypto.SHA256)
	}
	if err != nil {
		panic(err)
	}
	return sig
}

func (a *Authenticator) clientData(typ string, challenge []byte, o Options) []byte {
	if o.Type != "" {
		typ = o.Type
	}
	if o.Challenge != nil {
		challenge = o.Challenge
	}
	origin := a.Origin
	if o.Origin != "" {
		origin = o.Origin
	}
	b, _ := json.Marshal(map[string]any{"type": typ, "challenge": base64.RawURLEncoding.EncodeToString(challenge),
		"origin": origin, "crossOrigin": o.CrossOrigin})
	return b
}

func (a *Authenticator) authData(c *Credential, attested bool, o Options) []byte {
	rp := a.RPID
	if o.RPID != "" {
		rp = o.RPID
	}
	h := sha256.Sum256([]byte(rp))
	var flags byte
	if !o.NoUP {
		flags |= 0x01
	}
	if !o.NoUV {
		flags |= 0x04
	}
	if o.BE {
		flags |= 0x08
	}
	if o.BS {
		flags |= 0x10
	}
	if attested {
		flags |= 0x40
	}
	var buf bytes.Buffer
	buf.Write(h[:])
	buf.WriteByte(flags)
	var cnt [4]byte
	binary.BigEndian.PutUint32(cnt[:], c.Counter)
	buf.Write(cnt[:])
	if attested {
		buf.Write(a.AAGUID)
		var l [2]byte
		binary.BigEndian.PutUint16(l[:], uint16(len(c.ID)))
		buf.Write(l[:])
		buf.Write(c.ID)
		buf.Write(c.COSEKey())
	}
	return buf.Bytes()
}

// Create answers a registration (navigator.credentials.create) for the
// challenge with credential c: clientDataJSON and attestationObject.
func (a *Authenticator) Create(c *Credential, challenge []byte, o Options) (clientDataJSON, attestationObject []byte) {
	cd := a.clientData("webauthn.create", challenge, o)
	ad := a.authData(c, true, o)
	cdHash := sha256.Sum256(cd)
	signed := append(append([]byte{}, ad...), cdHash[:]...)
	format, stmt := "none", map[any]any{}
	switch o.Format {
	case "packed":
		format = "packed"
		stmt = map[any]any{"alg": c.Alg, "sig": c.sign(signed)}
	case "packed-x5c":
		format = "packed"
		att := NewCredential(ES256, nil)
		stmt = map[any]any{"alg": ES256, "sig": att.sign(signed), "x5c": []any{att.certificate()}}
	}
	if o.AttStmt != nil {
		stmt = o.AttStmt
	}
	return cd, Encode(map[any]any{"fmt": format, "attStmt": stmt, "authData": ad})
}

// certificate returns a self-signed end-entity certificate of the key.
func (c *Credential) certificate() []byte {
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Soft Authenticator Attestation", OrganizationalUnit: []string{"Authenticator Attestation"}},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, c.signer.Public(), c.signer)
	if err != nil {
		panic(err)
	}
	return der
}

// Get answers an authentication (navigator.credentials.get) with c.
func (a *Authenticator) Get(c *Credential, challenge []byte, o Options) (clientDataJSON, authenticatorData, signature []byte) {
	if c.Counting {
		c.Counter++
	}
	cd := a.clientData("webauthn.get", challenge, o)
	ad := a.authData(c, false, o)
	cdHash := sha256.Sum256(cd)
	sig := c.sign(append(append([]byte{}, ad...), cdHash[:]...))
	if o.BadSignature {
		sig[len(sig)/2] ^= 0xff
	}
	return cd, ad, sig
}

// ---------------------------------------------------------------- JSON (what the browser posts)

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// CreateJSON returns the PublicKeyCredential JSON of a registration for
// the challenge (base64url, as in the server's creation options).
func (a *Authenticator) CreateJSON(c *Credential, challenge string, o Options) map[string]any {
	ch, err := base64.RawURLEncoding.DecodeString(challenge)
	if err != nil {
		panic(err)
	}
	cd, att := a.Create(c, ch, o)
	return map[string]any{"id": b64(c.ID), "rawId": b64(c.ID), "type": "public-key",
		"response": map[string]any{"clientDataJSON": b64(cd), "attestationObject": b64(att), "transports": []string{"internal", "hybrid"}}}
}

// GetJSON returns the PublicKeyCredential JSON of an assertion.
func (a *Authenticator) GetJSON(c *Credential, challenge string, o Options) map[string]any {
	ch, err := base64.RawURLEncoding.DecodeString(challenge)
	if err != nil {
		panic(err)
	}
	cd, ad, sig := a.Get(c, ch, o)
	resp := map[string]any{"clientDataJSON": b64(cd), "authenticatorData": b64(ad), "signature": b64(sig)}
	if c.UserHandle != nil {
		resp["userHandle"] = b64(c.UserHandle)
	}
	return map[string]any{"id": b64(c.ID), "rawId": b64(c.ID), "type": "public-key", "response": resp}
}

// ---------------------------------------------------------------- CBOR encoding

// Encode encodes v in canonical CBOR: int64/int, []byte, string, bool,
// nil, []any and map[any]any (int64 or string keys, sorted).
func Encode(v any) []byte {
	var b bytes.Buffer
	enc(&b, v)
	return b.Bytes()
}

func head(b *bytes.Buffer, major byte, n uint64) {
	switch {
	case n < 24:
		b.WriteByte(major<<5 | byte(n))
	case n <= 0xff:
		b.WriteByte(major<<5 | 24)
		b.WriteByte(byte(n))
	case n <= 0xffff:
		b.WriteByte(major<<5 | 25)
		_ = binary.Write(b, binary.BigEndian, uint16(n))
	case n <= 0xffffffff:
		b.WriteByte(major<<5 | 26)
		_ = binary.Write(b, binary.BigEndian, uint32(n))
	default:
		b.WriteByte(major<<5 | 27)
		_ = binary.Write(b, binary.BigEndian, n)
	}
}

func enc(b *bytes.Buffer, v any) {
	switch x := v.(type) {
	case int:
		enc(b, int64(x))
	case int64:
		if x >= 0 {
			head(b, 0, uint64(x))
		} else {
			head(b, 1, uint64(-1-x))
		}
	case []byte:
		head(b, 2, uint64(len(x)))
		b.Write(x)
	case string:
		head(b, 3, uint64(len(x)))
		b.WriteString(x)
	case []any:
		head(b, 4, uint64(len(x)))
		for _, e := range x {
			enc(b, e)
		}
	case map[any]any:
		type kv struct{ k, v []byte }
		var items []kv
		for k, e := range x {
			items = append(items, kv{Encode(k), Encode(e)})
		}
		// canonical: shorter keys first, then bytewise
		sort.Slice(items, func(i, j int) bool {
			if len(items[i].k) != len(items[j].k) {
				return len(items[i].k) < len(items[j].k)
			}
			return bytes.Compare(items[i].k, items[j].k) < 0
		})
		head(b, 5, uint64(len(items)))
		for _, it := range items {
			b.Write(it.k)
			b.Write(it.v)
		}
	case bool:
		if x {
			b.WriteByte(0xf5)
		} else {
			b.WriteByte(0xf4)
		}
	case nil:
		b.WriteByte(0xf6)
	default:
		panic("softauthn: cannot encode value")
	}
}
