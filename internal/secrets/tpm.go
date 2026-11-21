package secrets

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/google/go-tpm/tpm2"
	"github.com/google/go-tpm/tpm2/transport"
)

// TPM seals data-encryption keys to a TPM 2.0. The sealed object is a
// keyed-hash object under the TPM's storage root key (the ECC P-256 SRK
// of the owner hierarchy, recreated from the TPM's seed on each use), so
// it can only be unsealed by the same TPM (until the owner hierarchy is
// cleared). With PCRs set, unsealing also requires those PCRs to have the
// values they had when the key was sealed. Commands that carry the key are
// encrypted with a session salted to the SRK.
type TPM struct {
	mu     sync.Mutex
	t      transport.TPMCloser
	device string
	pcrs   []uint
	bank   tpm2.TPMIAlgHash
	keyID  string
}

// openTPM opens a TPM device or emulator socket (platform specific).
var openTPM = openTPMDevice

// NewTPM opens the TPM at device (a character device such as /dev/tpmrm0,
// or the Unix socket of a TPM emulator) and checks that it can create the
// storage root key. pcrs (may be empty) and bank (sha256 or sha1) are the
// PCR policy of keys sealed from now on.
func NewTPM(device string, pcrs []int, bank string) (*TPM, error) {
	t, err := openTPM(device)
	if err != nil {
		return nil, fmt.Errorf("secrets: open TPM %s: %w", device, err)
	}
	p, err := NewTPMTransport(t, pcrs, bank)
	if err != nil {
		t.Close()
		return nil, err
	}
	p.device = device
	return p, nil
}

// NewTPMTransport is NewTPM over an open transport (tests, simulators).
func NewTPMTransport(t transport.TPMCloser, pcrs []int, bank string) (*TPM, error) {
	p := &TPM{t: t, bank: tpm2.TPMAlgSHA256}
	switch bank {
	case "", "sha256":
	case "sha1":
		p.bank = tpm2.TPMAlgSHA1
	default:
		return nil, fmt.Errorf("secrets: unknown PCR bank %q (sha256 or sha1)", bank)
	}
	for _, x := range pcrs {
		if x < 0 || x > 23 {
			return nil, fmt.Errorf("secrets: %d is not a PCR (0-23)", x)
		}
		p.pcrs = append(p.pcrs, uint(x))
	}
	slices.Sort(p.pcrs)
	p.pcrs = slices.Compact(p.pcrs)
	srk, err := p.primary()
	if err != nil {
		return nil, fmt.Errorf("secrets: TPM: create the storage root key: %w", err)
	}
	defer p.flush(srk.handle)
	h := sha256.Sum256(srk.name.Buffer)
	p.keyID = "tpm:" + hex.EncodeToString(h[:8])
	return p, nil
}

type srk struct {
	handle tpm2.TPMHandle
	name   tpm2.TPM2BName
	pub    tpm2.TPMTPublic
}

// primary creates the storage root key (the same key every time: it is
// derived from the owner hierarchy's seed and the standard template).
func (p *TPM) primary() (*srk, error) {
	rsp, err := tpm2.CreatePrimary{PrimaryHandle: tpm2.TPMRHOwner, InPublic: tpm2.New2B(tpm2.ECCSRKTemplate)}.Execute(p.t)
	if err != nil {
		return nil, err
	}
	pub, err := rsp.OutPublic.Contents()
	if err != nil {
		p.flush(rsp.ObjectHandle)
		return nil, err
	}
	return &srk{handle: rsp.ObjectHandle, name: rsp.Name, pub: *pub}, nil
}

func (p *TPM) flush(h tpm2.TPMHandle) {
	_, _ = tpm2.FlushContext{FlushHandle: h}.Execute(p.t)
}

func (p *TPM) selection(pcrs []uint, bank tpm2.TPMIAlgHash) tpm2.TPMLPCRSelection {
	return tpm2.TPMLPCRSelection{PCRSelections: []tpm2.TPMSPCRSelection{{Hash: bank, PCRSelect: tpm2.PCClientCompatible.PCRs(pcrs...)}}}
}

// policy runs PolicyPCR in a policy (or trial) session over the current
// PCR values.
func (p *TPM) policy(sess tpm2.Session, pcrs []uint, bank tpm2.TPMIAlgHash) error {
	_, err := tpm2.PolicyPCR{PolicySession: sess.Handle(), Pcrs: p.selection(pcrs, bank)}.Execute(p.t)
	return err
}

// Name implements Provider.
func (p *TPM) Name() string { return "tpm" }

// KeyID implements Provider: a fingerprint of the TPM's storage root key.
func (p *TPM) KeyID() string { return p.keyID }

// sealed blob: version 1 | bank (2) | PCR count (1) | PCRs | TPM2B_PUBLIC
// (2 + n) | TPM2B_PRIVATE (2 + n).
const tpmBlobVersion = 1

// Wrap implements Provider: it seals dek to the TPM.
func (p *TPM) Wrap(dek []byte, _ string) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s, err := p.primary()
	if err != nil {
		return nil, fmt.Errorf("secrets: TPM: %w", err)
	}
	defer p.flush(s.handle)
	tmpl := tpm2.TPMTPublic{
		Type:    tpm2.TPMAlgKeyedHash,
		NameAlg: tpm2.TPMAlgSHA256,
		ObjectAttributes: tpm2.TPMAObject{
			FixedTPM:     true,
			FixedParent:  true,
			UserWithAuth: len(p.pcrs) == 0,
			NoDA:         true,
		},
		Parameters: tpm2.NewTPMUPublicParms(tpm2.TPMAlgKeyedHash, &tpm2.TPMSKeyedHashParms{
			Scheme: tpm2.TPMTKeyedHashScheme{Scheme: tpm2.TPMAlgNull},
		}),
	}
	if len(p.pcrs) > 0 {
		sess, closeSess, err := tpm2.PolicySession(p.t, tpm2.TPMAlgSHA256, 16, tpm2.Trial())
		if err != nil {
			return nil, fmt.Errorf("secrets: TPM: trial session: %w", err)
		}
		err = p.policy(sess, p.pcrs, p.bank)
		var dg *tpm2.PolicyGetDigestResponse
		if err == nil {
			dg, err = tpm2.PolicyGetDigest{PolicySession: sess.Handle()}.Execute(p.t)
		}
		_ = closeSess()
		if err != nil {
			return nil, fmt.Errorf("secrets: TPM: PCR policy: %w", err)
		}
		tmpl.AuthPolicy = dg.PolicyDigest
	}
	rsp, err := tpm2.Create{
		ParentHandle: tpm2.AuthHandle{Handle: s.handle, Name: s.name,
			Auth: tpm2.HMAC(tpm2.TPMAlgSHA256, 16, tpm2.AESEncryption(128, tpm2.EncryptIn), tpm2.Salted(s.handle, s.pub))},
		InSensitive: tpm2.TPM2BSensitiveCreate{Sensitive: &tpm2.TPMSSensitiveCreate{
			Data: tpm2.NewTPMUSensitiveCreate(&tpm2.TPM2BSensitiveData{Buffer: dek}),
		}},
		InPublic: tpm2.New2B(tmpl),
	}.Execute(p.t)
	if err != nil {
		return nil, fmt.Errorf("secrets: TPM: seal: %w", err)
	}
	var b bytes.Buffer
	b.WriteByte(tpmBlobVersion)
	_ = binary.Write(&b, binary.BigEndian, uint16(p.bank))
	b.WriteByte(byte(len(p.pcrs)))
	for _, x := range p.pcrs {
		b.WriteByte(byte(x))
	}
	for _, part := range [][]byte{tpm2.Marshal(rsp.OutPublic), tpm2.Marshal(rsp.OutPrivate)} {
		b.Write(part)
	}
	return b.Bytes(), nil
}

type tpmBlob struct {
	bank tpm2.TPMIAlgHash
	pcrs []uint
	pub  tpm2.TPM2BPublic
	priv tpm2.TPM2BPrivate
}

var errTPMBlob = errors.New("secrets: TPM: the sealed key is damaged")

func parseTPMBlob(b []byte) (*tpmBlob, error) {
	if len(b) < 4 || b[0] != tpmBlobVersion {
		return nil, errTPMBlob
	}
	out := &tpmBlob{bank: tpm2.TPMIAlgHash(binary.BigEndian.Uint16(b[1:3]))}
	n := int(b[3])
	b = b[4:]
	if len(b) < n {
		return nil, errTPMBlob
	}
	for _, x := range b[:n] {
		out.pcrs = append(out.pcrs, uint(x))
	}
	b = b[n:]
	// TPM2B structures: a 2-byte size and the bytes
	part := func() ([]byte, error) {
		if len(b) < 2 {
			return nil, errTPMBlob
		}
		size := int(binary.BigEndian.Uint16(b))
		if len(b) < 2+size {
			return nil, errTPMBlob
		}
		x := b[:2+size]
		b = b[2+size:]
		return x, nil
	}
	pub, err := part()
	if err != nil {
		return nil, err
	}
	priv, err := part()
	if err != nil {
		return nil, err
	}
	if len(b) != 0 {
		return nil, errTPMBlob
	}
	pp, err := tpm2.Unmarshal[tpm2.TPM2BPublic](pub)
	if err != nil {
		return nil, errTPMBlob
	}
	pv, err := tpm2.Unmarshal[tpm2.TPM2BPrivate](priv)
	if err != nil {
		return nil, errTPMBlob
	}
	out.pub, out.priv = *pp, *pv
	return out, nil
}

// Unwrap implements Provider: it unseals a key sealed by Wrap.
func (p *TPM) Unwrap(wrapped []byte, _ string) ([]byte, error) {
	blob, err := parseTPMBlob(wrapped)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	s, err := p.primary()
	if err != nil {
		return nil, fmt.Errorf("secrets: TPM: %w", err)
	}
	defer p.flush(s.handle)
	obj, err := tpm2.Load{
		ParentHandle: tpm2.AuthHandle{Handle: s.handle, Name: s.name, Auth: tpm2.PasswordAuth(nil)},
		InPrivate:    blob.priv,
		InPublic:     blob.pub,
	}.Execute(p.t)
	if err != nil {
		return nil, fmt.Errorf("secrets: TPM: load the sealed key (was it sealed by another TPM, or was the TPM cleared?): %w", err)
	}
	defer p.flush(obj.ObjectHandle)
	var auth tpm2.Session
	if len(blob.pcrs) > 0 {
		sess, closeSess, err := tpm2.PolicySession(p.t, tpm2.TPMAlgSHA256, 16,
			tpm2.Salted(s.handle, s.pub), tpm2.AESEncryption(128, tpm2.EncryptOut))
		if err != nil {
			return nil, fmt.Errorf("secrets: TPM: policy session: %w", err)
		}
		defer closeSess() //nolint:errcheck // best effort
		if err := p.policy(sess, blob.pcrs, blob.bank); err != nil {
			return nil, fmt.Errorf("secrets: TPM: PCR policy: %w", err)
		}
		auth = sess
	} else {
		auth = tpm2.HMAC(tpm2.TPMAlgSHA256, 16, tpm2.AESEncryption(128, tpm2.EncryptOut), tpm2.Salted(s.handle, s.pub))
	}
	rsp, err := tpm2.Unseal{ItemHandle: tpm2.AuthHandle{Handle: obj.ObjectHandle, Name: obj.Name, Auth: auth}}.Execute(p.t)
	if err != nil {
		if len(blob.pcrs) > 0 {
			return nil, fmt.Errorf("secrets: TPM: unseal failed; the PCRs %s have changed since the key was sealed: %w", pcrList(blob.pcrs), err)
		}
		return nil, fmt.Errorf("secrets: TPM: unseal: %w", err)
	}
	return rsp.OutData.Buffer, nil
}

func pcrList(pcrs []uint) string {
	s := make([]string, len(pcrs))
	for i, x := range pcrs {
		s[i] = strconv.Itoa(int(x))
	}
	return strings.Join(s, ",")
}

// Info implements Provider.
func (p *TPM) Info() map[string]string {
	m := map[string]string{"device": p.device}
	if len(p.pcrs) > 0 {
		bank := "sha256"
		if p.bank == tpm2.TPMAlgSHA1 {
			bank = "sha1"
		}
		m["pcrs"] = bank + ":" + pcrList(p.pcrs)
	}
	return m
}

// Close implements Provider.
func (p *TPM) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.t == nil {
		return nil
	}
	err := p.t.Close()
	p.t = nil
	return err
}
