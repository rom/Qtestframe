package auth

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"slices"
	"strings"
)

// WebAuthn (passkeys): the relying-party side of registration and
// authentication ceremonies (W3C Web Authentication Level 2, §7.1 and
// §7.2), written against the standard library only.
//
// Supported: the attestation formats "none" and "packed" (self
// attestation, and x5c attestation whose signature is checked but whose
// certificate is not chained to a trust anchor: Qtestframe asks for no
// attestation and does not decide by authenticator model), the COSE
// algorithms ES256 (-7), RS256 (-257) and EdDSA with Ed25519 (-8).

// COSE algorithm identifiers.
const (
	COSEAlgES256 int64 = -7
	COSEAlgEdDSA int64 = -8
	COSEAlgRS256 int64 = -257
)

// WebAuthnAlgorithms are the accepted algorithms in order of preference
// (pubKeyCredParams of a registration).
var WebAuthnAlgorithms = []int64{COSEAlgES256, COSEAlgEdDSA, COSEAlgRS256}

// Authenticator data flags.
const (
	flagUP = 0x01 // user present
	flagUV = 0x04 // user verified
	flagBE = 0x08 // backup eligible
	flagBS = 0x10 // backed up
	flagAT = 0x40 // attested credential data included
	flagED = 0x80 // extension data included
)

// WebAuthnChallengeBytes is the size of a challenge (at least 16 per spec).
const WebAuthnChallengeBytes = 32

// MaxCredentialIDLength is the longest accepted credential id (spec: 1023).
const MaxCredentialIDLength = 1023

// ErrWebAuthn is matched (errors.Is) by every verification failure.
var ErrWebAuthn = errors.New("passkey verification failed")

type webauthnError struct{ msg string }

func (e webauthnError) Error() string     { return "passkey verification failed: " + e.msg }
func (e webauthnError) Is(t error) bool   { return t == ErrWebAuthn }
func waErr(format string, a ...any) error { return webauthnError{fmt.Sprintf(format, a...)} }

// RelyingParty is the server's WebAuthn identity: the RP ID (a domain,
// the scope of the credentials) and the web origins allowed to use it.
type RelyingParty struct {
	ID      string
	Name    string
	Origins []string
}

// NewRelyingParty derives the relying party from the base URL when rpID
// or origins are not given: the RP ID is the host of the base URL and the
// only origin is the base URL's scheme and host. Each origin must be
// https (or http on localhost) and its host the RP ID or a subdomain of it.
func NewRelyingParty(baseURL, rpID string, origins []string) (*RelyingParty, error) {
	rp := &RelyingParty{ID: strings.ToLower(strings.TrimSpace(rpID)), Name: "Qtestframe"}
	for _, o := range origins {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			rp.Origins = append(rp.Origins, strings.ToLower(o))
		}
	}
	if baseURL != "" && (rp.ID == "" || len(rp.Origins) == 0) {
		u, err := url.Parse(strings.TrimSpace(baseURL))
		if err != nil || u.Host == "" {
			return nil, fmt.Errorf("passkeys: server.base_url %q is not a URL", baseURL)
		}
		if rp.ID == "" {
			rp.ID = strings.ToLower(u.Hostname())
		}
		if len(rp.Origins) == 0 {
			rp.Origins = []string{strings.ToLower(u.Scheme + "://" + u.Host)}
		}
	}
	if rp.ID == "" || len(rp.Origins) == 0 {
		return nil, errors.New("passkeys need server.base_url (or auth.passkeys.rp_id and auth.passkeys.origins)")
	}
	if strings.ContainsAny(rp.ID, ":/ ") || strings.HasPrefix(rp.ID, ".") || strings.HasSuffix(rp.ID, ".") {
		return nil, fmt.Errorf("passkeys: the RP ID %q must be a domain name", rp.ID)
	}
	for _, o := range rp.Origins {
		if err := CheckOriginForRP(o, rp.ID); err != nil {
			return nil, err
		}
	}
	return rp, nil
}

// CheckOriginForRP reports whether origin (scheme://host[:port]) may use
// credentials of the RP ID: https, or http on localhost, with a host that
// equals the RP ID or ends with "."+rpID.
func CheckOriginForRP(origin, rpID string) error {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return fmt.Errorf("passkeys: %q is not an origin (scheme://host[:port])", origin)
	}
	host := strings.ToLower(u.Hostname())
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && (host == "localhost" || strings.HasSuffix(host, ".localhost") || isLoopback(host)):
		// browsers treat localhost as a secure context
	default:
		return fmt.Errorf("passkeys: the origin %q must use https", origin)
	}
	if host != rpID && !strings.HasSuffix(host, "."+rpID) {
		return fmt.Errorf("passkeys: the origin %q is not within the RP ID %q", origin, rpID)
	}
	return nil
}

func isLoopback(h string) bool { return h == "127.0.0.1" || h == "::1" }

// RPIDHash returns SHA-256 of the RP ID.
func (rp *RelyingParty) RPIDHash() [32]byte { return sha256.Sum256([]byte(rp.ID)) }

func (rp *RelyingParty) originAllowed(o string) bool {
	return slices.Contains(rp.Origins, strings.ToLower(strings.TrimRight(o, "/")))
}

// NewChallenge returns a random challenge.
func NewChallenge() []byte {
	b := make([]byte, WebAuthnChallengeBytes)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("auth: crypto/rand failed: %v", err))
	}
	return b
}

// B64 encodes bytes as unpadded base64url (the JSON form of WebAuthn).
func B64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// UnB64 decodes base64url with or without padding.
func UnB64(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

// ---------------------------------------------------------------- client data

// ClientData is the collected client data (clientDataJSON).
type ClientData struct {
	Type        string `json:"type"`
	Challenge   string `json:"challenge"`
	Origin      string `json:"origin"`
	CrossOrigin bool   `json:"crossOrigin"`
	TopOrigin   string `json:"topOrigin"`
}

// ParseClientData decodes clientDataJSON.
func ParseClientData(b []byte) (*ClientData, error) {
	if len(b) == 0 || len(b) > 4096 {
		return nil, waErr("clientDataJSON is empty or too long")
	}
	var c ClientData
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, waErr("clientDataJSON is not JSON")
	}
	return &c, nil
}

// ChallengeBytes returns the decoded challenge.
func (c *ClientData) ChallengeBytes() ([]byte, error) {
	b, err := UnB64(c.Challenge)
	if err != nil || len(b) == 0 {
		return nil, waErr("the challenge is not base64url")
	}
	return b, nil
}

func (rp *RelyingParty) checkClientData(raw []byte, typ string, challenge []byte) error {
	c, err := ParseClientData(raw)
	if err != nil {
		return err
	}
	if c.Type != typ {
		return waErr("clientData type is %q, want %q", c.Type, typ)
	}
	got, err := c.ChallengeBytes()
	if err != nil {
		return err
	}
	if len(challenge) == 0 || subtle.ConstantTimeCompare(got, challenge) != 1 {
		return waErr("the challenge does not match")
	}
	if !rp.originAllowed(c.Origin) {
		return waErr("the origin %q is not allowed", c.Origin)
	}
	if c.CrossOrigin || c.TopOrigin != "" {
		return waErr("cross-origin use (in a frame) is not allowed")
	}
	return nil
}

// ---------------------------------------------------------------- authenticator data

// AuthData is parsed authenticator data.
type AuthData struct {
	RPIDHash  []byte
	Flags     byte
	SignCount uint32
	// attested credential data (registration)
	AAGUID       []byte
	CredentialID []byte
	PublicKey    []byte // the COSE key, as encoded
	Extensions   map[any]any
}

// UserPresent, UserVerified, BackupEligible and BackedUp read the flags.
func (a *AuthData) UserPresent() bool    { return a.Flags&flagUP != 0 }
func (a *AuthData) UserVerified() bool   { return a.Flags&flagUV != 0 }
func (a *AuthData) BackupEligible() bool { return a.Flags&flagBE != 0 }
func (a *AuthData) BackedUp() bool       { return a.Flags&flagBS != 0 }

// ParseAuthData decodes authenticator data (§6.1).
func ParseAuthData(b []byte) (*AuthData, error) {
	if len(b) < 37 {
		return nil, waErr("authenticator data is too short")
	}
	a := &AuthData{RPIDHash: b[:32], Flags: b[32], SignCount: binary.BigEndian.Uint32(b[33:37])}
	rest := b[37:]
	if a.Flags&flagAT != 0 {
		if len(rest) < 18 {
			return nil, waErr("attested credential data is too short")
		}
		a.AAGUID = rest[:16]
		n := int(binary.BigEndian.Uint16(rest[16:18]))
		rest = rest[18:]
		if n == 0 || n > MaxCredentialIDLength || n > len(rest) {
			return nil, waErr("bad credential id length %d", n)
		}
		a.CredentialID, rest = rest[:n], rest[n:]
		_, used, err := cborDecode(rest)
		if err != nil {
			return nil, waErr("credential public key: %v", err)
		}
		a.PublicKey, rest = rest[:used], rest[used:]
	}
	if a.Flags&flagED != 0 {
		v, used, err := cborDecode(rest)
		if err != nil {
			return nil, waErr("extensions: %v", err)
		}
		m, ok := v.(map[any]any)
		if !ok {
			return nil, waErr("extensions are not a map")
		}
		a.Extensions, rest = m, rest[used:]
	}
	if len(rest) != 0 {
		return nil, waErr("%d bytes after the authenticator data", len(rest))
	}
	return a, nil
}

func (rp *RelyingParty) checkAuthData(a *AuthData, requireUV bool) error {
	want := rp.RPIDHash()
	if subtle.ConstantTimeCompare(a.RPIDHash, want[:]) != 1 {
		return waErr("the RP ID hash does not match %q", rp.ID)
	}
	if !a.UserPresent() {
		return waErr("the user was not present (UP flag)")
	}
	if requireUV && !a.UserVerified() {
		return waErr("the user was not verified (UV flag): use a PIN, fingerprint or face")
	}
	if a.BackedUp() && !a.BackupEligible() {
		return waErr("backed up but not backup eligible (BE/BS flags)")
	}
	return nil
}

// ---------------------------------------------------------------- COSE keys

// PublicKey is a credential public key.
type PublicKey struct {
	Alg int64
	key crypto.PublicKey
}

func coseInt(m map[any]any, k int64) (int64, bool) {
	v, ok := m[k].(int64)
	return v, ok
}

func coseBytes(m map[any]any, k int64) ([]byte, bool) {
	v, ok := m[k].([]byte)
	return v, ok
}

// ParsePublicKey decodes a COSE_Key (RFC 9053) of a supported algorithm.
func ParsePublicKey(b []byte) (*PublicKey, error) {
	v, err := cborDecodeAll(b)
	if err != nil {
		return nil, waErr("public key: %v", err)
	}
	m, ok := v.(map[any]any)
	if !ok {
		return nil, waErr("public key is not a COSE key")
	}
	kty, _ := coseInt(m, 1)
	alg, ok := coseInt(m, 3)
	if !ok {
		return nil, waErr("public key without algorithm")
	}
	switch alg {
	case COSEAlgES256:
		crv, _ := coseInt(m, -1)
		x, okx := coseBytes(m, -2)
		y, oky := coseBytes(m, -3)
		if kty != 2 || crv != 1 || !okx || !oky || len(x) != 32 || len(y) != 32 {
			return nil, waErr("bad ES256 key (EC2, P-256)")
		}
		pk, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append(append([]byte{4}, x...), y...))
		if err != nil {
			return nil, waErr("ES256 key: %v", err)
		}
		return &PublicKey{Alg: alg, key: pk}, nil
	case COSEAlgRS256:
		n, okn := coseBytes(m, -1)
		e, oke := coseBytes(m, -2)
		if kty != 3 || !okn || !oke || len(e) == 0 || len(e) > 4 {
			return nil, waErr("bad RS256 key")
		}
		ev := 0
		for _, c := range e {
			ev = ev<<8 | int(c)
		}
		pk := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: ev}
		if pk.N.BitLen() < 2048 || ev < 3 || ev%2 == 0 {
			return nil, waErr("RS256 key too small or bad exponent")
		}
		return &PublicKey{Alg: alg, key: pk}, nil
	case COSEAlgEdDSA:
		crv, _ := coseInt(m, -1)
		x, okx := coseBytes(m, -2)
		if kty != 1 || crv != 6 || !okx || len(x) != ed25519.PublicKeySize {
			return nil, waErr("bad EdDSA key (OKP, Ed25519)")
		}
		return &PublicKey{Alg: alg, key: ed25519.PublicKey(x)}, nil
	}
	return nil, waErr("unsupported algorithm %d (use ES256, RS256 or EdDSA)", alg)
}

// Verify checks sig over data.
func (k *PublicKey) Verify(data, sig []byte) error {
	return verifySig(k.Alg, k.key, data, sig)
}

func verifySig(alg int64, key crypto.PublicKey, data, sig []byte) error {
	switch alg {
	case COSEAlgES256:
		pk, ok := key.(*ecdsa.PublicKey)
		sum := sha256.Sum256(data)
		if ok && ecdsa.VerifyASN1(pk, sum[:], sig) {
			return nil
		}
	case COSEAlgRS256:
		pk, ok := key.(*rsa.PublicKey)
		sum := sha256.Sum256(data)
		if ok && rsa.VerifyPKCS1v15(pk, crypto.SHA256, sum[:], sig) == nil {
			return nil
		}
	case COSEAlgEdDSA:
		pk, ok := key.(ed25519.PublicKey)
		if ok && ed25519.Verify(pk, data, sig) {
			return nil
		}
	default:
		return waErr("unsupported algorithm %d", alg)
	}
	return waErr("the signature is not valid")
}

// ---------------------------------------------------------------- registration

// RegistrationResponse is the authenticator's answer to create().
type RegistrationResponse struct {
	ClientDataJSON    []byte
	AttestationObject []byte
}

// NewCredential is a verified new credential.
type NewCredential struct {
	ID        []byte
	PublicKey []byte // COSE key
	Alg       int64
	SignCount uint32
	AAGUID    []byte
	// Format is the attestation statement format ("none" or "packed").
	Format         string
	UserVerified   bool
	BackupEligible bool
	BackedUp       bool
}

// VerifyRegistration verifies a registration ceremony (§7.1) for the
// challenge the server issued. requireUV demands user verification.
func (rp *RelyingParty) VerifyRegistration(res RegistrationResponse, challenge []byte, requireUV bool) (*NewCredential, error) {
	if err := rp.checkClientData(res.ClientDataJSON, "webauthn.create", challenge); err != nil {
		return nil, err
	}
	if len(res.AttestationObject) > 64<<10 {
		return nil, waErr("attestation object too long")
	}
	v, err := cborDecodeAll(res.AttestationObject)
	if err != nil {
		return nil, waErr("attestation object: %v", err)
	}
	att, ok := v.(map[any]any)
	if !ok {
		return nil, waErr("attestation object is not a map")
	}
	format, _ := att["fmt"].(string)
	stmt, okStmt := att["attStmt"].(map[any]any)
	raw, okAD := att["authData"].([]byte)
	if format == "" || !okStmt || !okAD {
		return nil, waErr("attestation object lacks fmt, attStmt or authData")
	}
	ad, err := ParseAuthData(raw)
	if err != nil {
		return nil, err
	}
	if err := rp.checkAuthData(ad, requireUV); err != nil {
		return nil, err
	}
	if ad.Flags&flagAT == 0 {
		return nil, waErr("no attested credential data")
	}
	key, err := ParsePublicKey(ad.PublicKey)
	if err != nil {
		return nil, err
	}
	cdHash := sha256.Sum256(res.ClientDataJSON)
	switch format {
	case "none":
		if len(stmt) != 0 {
			return nil, waErr("a \"none\" attestation with a statement")
		}
	case "packed":
		if err := verifyPacked(stmt, raw, cdHash[:], key, ad.AAGUID); err != nil {
			return nil, err
		}
	default:
		return nil, waErr("unsupported attestation format %q", format)
	}
	return &NewCredential{ID: bytes.Clone(ad.CredentialID), PublicKey: bytes.Clone(ad.PublicKey), Alg: key.Alg,
		SignCount: ad.SignCount, AAGUID: bytes.Clone(ad.AAGUID), Format: format,
		UserVerified: ad.UserVerified(), BackupEligible: ad.BackupEligible(), BackedUp: ad.BackedUp()}, nil
}

// idFidoGenCeAAGUID is the certificate extension holding the AAGUID.
var idFidoGenCeAAGUID = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 45724, 1, 1, 4}

// verifyPacked checks a "packed" attestation statement (§8.2).
func verifyPacked(stmt map[any]any, authData, cdHash []byte, cred *PublicKey, aaguid []byte) error {
	alg, okAlg := stmt["alg"].(int64)
	sig, okSig := stmt["sig"].([]byte)
	if !okAlg || !okSig {
		return waErr("packed attestation without alg or sig")
	}
	if _, ecdaa := stmt["ecdaaKeyId"]; ecdaa {
		return waErr("ECDAA attestation is not supported")
	}
	signed := append(append([]byte{}, authData...), cdHash...)
	x5c, hasX5C := stmt["x5c"]
	if !hasX5C {
		// self attestation: signed with the credential key itself
		if alg != cred.Alg {
			return waErr("self attestation algorithm %d differs from the key's %d", alg, cred.Alg)
		}
		return cred.Verify(signed, sig)
	}
	chain, ok := x5c.([]any)
	if !ok || len(chain) == 0 {
		return waErr("packed attestation x5c is empty")
	}
	der, ok := chain[0].([]byte)
	if !ok {
		return waErr("packed attestation x5c is not a certificate")
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return waErr("attestation certificate: %v", err)
	}
	if cert.Version != 3 || cert.IsCA {
		return waErr("the attestation certificate must be a version 3 end-entity certificate")
	}
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(idFidoGenCeAAGUID) {
			var got []byte
			if _, err := asn1.Unmarshal(ext.Value, &got); err != nil || !bytes.Equal(got, aaguid) {
				return waErr("the attestation certificate's AAGUID differs")
			}
		}
	}
	return verifySig(alg, cert.PublicKey, signed, sig)
}

// ---------------------------------------------------------------- authentication

// AssertionResponse is the authenticator's answer to get().
type AssertionResponse struct {
	ClientDataJSON    []byte
	AuthenticatorData []byte
	Signature         []byte
}

// AssertionResult is a verified assertion.
type AssertionResult struct {
	SignCount    uint32
	UserVerified bool
	BackedUp     bool
	// CounterRegressed: the authenticator's signature counter did not
	// increase (a possibly cloned authenticator); the caller warns.
	CounterRegressed bool
}

// VerifyAssertion verifies an authentication ceremony (§7.2) with the
// stored public key and signature counter of the credential.
func (rp *RelyingParty) VerifyAssertion(res AssertionResponse, challenge, publicKey []byte, storedCount uint32, requireUV bool) (*AssertionResult, error) {
	if err := rp.checkClientData(res.ClientDataJSON, "webauthn.get", challenge); err != nil {
		return nil, err
	}
	ad, err := ParseAuthData(res.AuthenticatorData)
	if err != nil {
		return nil, err
	}
	if err := rp.checkAuthData(ad, requireUV); err != nil {
		return nil, err
	}
	key, err := ParsePublicKey(publicKey)
	if err != nil {
		return nil, err
	}
	cdHash := sha256.Sum256(res.ClientDataJSON)
	signed := append(append([]byte{}, res.AuthenticatorData...), cdHash[:]...)
	if err := key.Verify(signed, res.Signature); err != nil {
		return nil, err
	}
	out := &AssertionResult{SignCount: ad.SignCount, UserVerified: ad.UserVerified(), BackedUp: ad.BackedUp()}
	// a counter of 0 on both sides: the authenticator has none (synced passkeys)
	if (ad.SignCount != 0 || storedCount != 0) && ad.SignCount <= storedCount {
		out.CounterRegressed = true
	}
	return out, nil
}
