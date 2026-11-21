package secrets

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-tpm/tpm2"
)

// startSWTPM starts a software TPM 2.0 (swtpm) in a temporary directory and
// returns the path of its command socket. The test is skipped when swtpm
// is not installed (apt-get install swtpm); QTF_TEST_SWTPM=require fails
// it instead.
func startSWTPM(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("swtpm")
	if err != nil {
		if os.Getenv("QTF_TEST_SWTPM") == "require" {
			t.Fatal("swtpm is not installed")
		}
		t.Skip("swtpm is not installed (apt-get install swtpm)")
	}
	// Unix socket paths are limited to ~108 bytes: use a short directory
	dir, err := os.MkdirTemp("", "qtf-tpm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "tpm.sock")
	cmd := exec.Command(bin, "socket", "--tpm2", "--tpmstate", "dir="+dir,
		"--server", "type=unixio,path="+sock, "--ctrl", "type=unixio,path="+filepath.Join(dir, "ctrl.sock"),
		"--flags", "not-need-init,startup-clear")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	for i := 0; i < 100; i++ {
		if fi, err := os.Stat(sock); err == nil && fi.Mode()&os.ModeSocket != 0 {
			return sock
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("swtpm did not start: %s", out.String())
	return ""
}

func TestTPMSealUnseal(t *testing.T) {
	sock := startSWTPM(t)
	p, err := NewTPM(sock, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if p.Name() != "tpm" || !strings.HasPrefix(p.KeyID(), "tpm:") || p.Info()["device"] != sock {
		t.Fatalf("provider: %s %s %v", p.Name(), p.KeyID(), p.Info())
	}
	dek := NewKey()
	w, err := p.Wrap(dek, "k1")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(w, dek) {
		t.Fatal("the sealed blob contains the key")
	}
	got, err := p.Unwrap(w, "k1")
	if err != nil || !bytes.Equal(got, dek) {
		t.Fatalf("unwrap: %v", err)
	}
	// the same TPM gives the same key id (the SRK is derived from its seed)
	p2, err := NewTPM(sock, []int{7, 0, 7}, "sha1")
	if err != nil {
		t.Fatal(err)
	}
	defer p2.Close()
	if p2.KeyID() != p.KeyID() {
		t.Fatalf("key id changed: %s %s", p.KeyID(), p2.KeyID())
	}
	if p2.Info()["pcrs"] != "sha1:0,7" {
		t.Fatalf("info: %v", p2.Info())
	}
	// damaged blobs
	for _, b := range [][]byte{nil, {2, 0, 0, 0}, w[:len(w)-3], append(append([]byte{}, w...), 1)} {
		if _, err := p.Unwrap(b, "k1"); err == nil {
			t.Fatalf("damaged blob %x unwrapped", b)
		}
	}
	// a blob with a flipped bit in the private part does not load
	bad := append([]byte{}, w...)
	bad[len(bad)-5] ^= 1
	if _, err := p.Unwrap(bad, "k1"); err == nil {
		t.Fatal("tampered blob unwrapped")
	}
}

func TestTPMPCRPolicy(t *testing.T) {
	sock := startSWTPM(t)
	p, err := NewTPM(sock, []int{16}, "sha256")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	dek := NewKey()
	w, err := p.Wrap(dek, "k1")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Unwrap(w, "k1")
	if err != nil || !bytes.Equal(got, dek) {
		t.Fatalf("unwrap: %v", err)
	}
	// extend PCR 16 (the debug PCR): the policy no longer matches
	_, err = tpm2.PCRExtend{
		PCRHandle: tpm2.AuthHandle{Handle: tpm2.TPMHandle(16), Auth: tpm2.PasswordAuth(nil)},
		Digests:   tpm2.TPMLDigestValues{Digests: []tpm2.TPMTHA{{HashAlg: tpm2.TPMAlgSHA256, Digest: make([]byte, 32)}}},
	}.Execute(p.t)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Unwrap(w, "k1"); err == nil || !strings.Contains(err.Error(), "PCRs 16 have changed") {
		t.Fatalf("unwrap after the PCR changed: %v", err)
	}
}

func TestTPMOpenErrors(t *testing.T) {
	if _, err := NewTPM(filepath.Join(t.TempDir(), "missing"), nil, ""); err == nil {
		t.Fatal("missing device opened")
	}
	sock := startSWTPM(t)
	if _, err := NewTPM(sock, nil, "md5"); err == nil {
		t.Fatal("bad bank accepted")
	}
	if _, err := NewTPM(sock, []int{24}, ""); err == nil {
		t.Fatal("bad PCR accepted")
	}
}
