//go:build pkcs11 && cgo

package secrets

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/miekg/pkcs11"
)

// PKCS11Supported reports whether this binary was built with HSM support.
const PKCS11Supported = true

// PKCS11 wraps data-encryption keys with an AES-256 key held in an HSM
// (CKM_AES_GCM; the key never leaves the HSM).
type PKCS11 struct {
	mu      sync.Mutex
	ctx     *pkcs11.Ctx
	session pkcs11.SessionHandle
	key     pkcs11.ObjectHandle
	token   string
	slot    uint
	label   string
	module  string
}

// NewPKCS11 loads the module, opens a session on the token, logs in and
// finds (or, with GenerateKey, creates) the key.
func NewPKCS11(c PKCS11Config) (*PKCS11, error) {
	if c.KeyLabel == "" {
		return nil, errors.New("secrets: pkcs11: key_label is required")
	}
	ctx := pkcs11.New(c.Module)
	if ctx == nil {
		return nil, fmt.Errorf("secrets: pkcs11: cannot load module %s", c.Module)
	}
	if err := ctx.Initialize(); err != nil && !errors.Is(err, pkcs11.Error(pkcs11.CKR_CRYPTOKI_ALREADY_INITIALIZED)) {
		ctx.Destroy()
		return nil, fmt.Errorf("secrets: pkcs11: initialize %s: %w", c.Module, err)
	}
	p := &PKCS11{ctx: ctx, label: c.KeyLabel, module: c.Module}
	fail := func(err error) (*PKCS11, error) {
		p.Close()
		return nil, err
	}
	slots, err := ctx.GetSlotList(true)
	if err != nil {
		return fail(fmt.Errorf("secrets: pkcs11: list slots: %w", err))
	}
	found := false
	for _, s := range slots {
		ti, err := ctx.GetTokenInfo(s)
		if err != nil {
			continue
		}
		if (c.Slot != nil && s == *c.Slot) || (c.Slot == nil && ti.Label == c.TokenLabel) {
			p.slot, p.token, found = s, ti.Label, true
			break
		}
	}
	if !found {
		want := "label " + strconv.Quote(c.TokenLabel)
		if c.Slot != nil {
			want = "slot " + strconv.FormatUint(uint64(*c.Slot), 10)
		}
		return fail(fmt.Errorf("secrets: pkcs11: no token with %s", want))
	}
	if p.session, err = ctx.OpenSession(p.slot, pkcs11.CKF_SERIAL_SESSION|pkcs11.CKF_RW_SESSION); err != nil {
		return fail(fmt.Errorf("secrets: pkcs11: open session: %w", err))
	}
	if err := ctx.Login(p.session, pkcs11.CKU_USER, c.PIN); err != nil && !errors.Is(err, pkcs11.Error(pkcs11.CKR_USER_ALREADY_LOGGED_IN)) {
		return fail(fmt.Errorf("secrets: pkcs11: log in to token %q (check the PIN): %w", p.token, err))
	}
	keys, err := p.findKeys()
	if err != nil {
		return fail(err)
	}
	switch {
	case len(keys) == 1:
		p.key = keys[0]
	case len(keys) > 1:
		return fail(fmt.Errorf("secrets: pkcs11: %d keys are labelled %q; the label must be unique", len(keys), c.KeyLabel))
	case c.GenerateKey:
		if p.key, err = p.generate(); err != nil {
			return fail(err)
		}
	default:
		return fail(fmt.Errorf("secrets: pkcs11: token %q has no AES key labelled %q (create one, or set secrets.pkcs11.generate_key)", p.token, c.KeyLabel))
	}
	return p, nil
}

func (p *PKCS11) findKeys() ([]pkcs11.ObjectHandle, error) {
	tmpl := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_SECRET_KEY),
		pkcs11.NewAttribute(pkcs11.CKA_LABEL, p.label),
	}
	if err := p.ctx.FindObjectsInit(p.session, tmpl); err != nil {
		return nil, fmt.Errorf("secrets: pkcs11: find key: %w", err)
	}
	objs, _, err := p.ctx.FindObjects(p.session, 4)
	_ = p.ctx.FindObjectsFinal(p.session)
	if err != nil {
		return nil, fmt.Errorf("secrets: pkcs11: find key: %w", err)
	}
	return objs, nil
}

// generate creates a non-extractable AES-256 token key.
func (p *PKCS11) generate() (pkcs11.ObjectHandle, error) {
	tmpl := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_SECRET_KEY),
		pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, pkcs11.CKK_AES),
		pkcs11.NewAttribute(pkcs11.CKA_VALUE_LEN, KeySize),
		pkcs11.NewAttribute(pkcs11.CKA_TOKEN, true),
		pkcs11.NewAttribute(pkcs11.CKA_PRIVATE, true),
		pkcs11.NewAttribute(pkcs11.CKA_SENSITIVE, true),
		pkcs11.NewAttribute(pkcs11.CKA_EXTRACTABLE, false),
		pkcs11.NewAttribute(pkcs11.CKA_ENCRYPT, true),
		pkcs11.NewAttribute(pkcs11.CKA_DECRYPT, true),
		pkcs11.NewAttribute(pkcs11.CKA_LABEL, p.label),
	}
	h, err := p.ctx.GenerateKey(p.session, []*pkcs11.Mechanism{pkcs11.NewMechanism(pkcs11.CKM_AES_KEY_GEN, nil)}, tmpl)
	if err != nil {
		return 0, fmt.Errorf("secrets: pkcs11: generate key %q: %w", p.label, err)
	}
	return h, nil
}

// Name implements Provider.
func (p *PKCS11) Name() string { return "pkcs11" }

// KeyID implements Provider.
func (p *PKCS11) KeyID() string { return "pkcs11:" + p.token + "/" + p.label }

const pkcs11BlobVersion = 1

// Wrap implements Provider: AES-256-GCM in the HSM.
func (p *PKCS11) Wrap(dek []byte, label string) ([]byte, error) {
	iv := make([]byte, nonceSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	params := pkcs11.NewGCMParams(iv, wrapAD(label), 128)
	defer params.Free()
	if err := p.ctx.EncryptInit(p.session, []*pkcs11.Mechanism{pkcs11.NewMechanism(pkcs11.CKM_AES_GCM, params)}, p.key); err != nil {
		return nil, fmt.Errorf("secrets: pkcs11: encrypt: %w", err)
	}
	ct, err := p.ctx.Encrypt(p.session, dek)
	if err != nil {
		return nil, fmt.Errorf("secrets: pkcs11: encrypt: %w", err)
	}
	var b bytes.Buffer
	b.WriteByte(pkcs11BlobVersion)
	b.Write(iv)
	b.Write(ct)
	return b.Bytes(), nil
}

// Unwrap implements Provider.
func (p *PKCS11) Unwrap(wrapped []byte, label string) ([]byte, error) {
	if len(wrapped) < 1+nonceSize+16 || wrapped[0] != pkcs11BlobVersion {
		return nil, errors.New("secrets: pkcs11: the wrapped key is damaged")
	}
	iv := wrapped[1 : 1+nonceSize]
	p.mu.Lock()
	defer p.mu.Unlock()
	params := pkcs11.NewGCMParams(iv, wrapAD(label), 128)
	defer params.Free()
	if err := p.ctx.DecryptInit(p.session, []*pkcs11.Mechanism{pkcs11.NewMechanism(pkcs11.CKM_AES_GCM, params)}, p.key); err != nil {
		return nil, fmt.Errorf("secrets: pkcs11: decrypt: %w", err)
	}
	out, err := p.ctx.Decrypt(p.session, wrapped[1+nonceSize:])
	if err != nil {
		return nil, fmt.Errorf("secrets: pkcs11: decrypt (wrong key?): %w", err)
	}
	return out, nil
}

// Info implements Provider.
func (p *PKCS11) Info() map[string]string {
	return map[string]string{"module": p.module, "token": p.token, "slot": strconv.FormatUint(uint64(p.slot), 10), "key_label": p.label}
}

// Close implements Provider.
func (p *PKCS11) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ctx == nil {
		return nil
	}
	if p.session != 0 {
		_ = p.ctx.Logout(p.session)
		_ = p.ctx.CloseSession(p.session)
	}
	_ = p.ctx.Finalize()
	p.ctx.Destroy()
	p.ctx = nil
	return nil
}
