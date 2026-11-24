// Package backup holds what scheduled and command-line backups add to the
// logical backup archive of internal/store: the encryption of an archive
// with a passphrase, the schedule, the retention of backup files in a
// directory and the upload to S3-compatible object storage.
package backup

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/scrypt"
)

// Encrypted archives: a header followed by the ZIP archive in chunks of
// ChunkSize bytes, each sealed with AES-256-GCM (the STREAM construction:
// the nonce is a random prefix, the chunk counter and a flag marking the
// last chunk, so chunks cannot be reordered, dropped or truncated). The
// key is derived from the passphrase with scrypt; its parameters and salt
// are in the header, which is the additional data of every chunk.
//
//	magic "QTFENC01" | logN | r | p | 0 | chunk size (uint32) | salt (16) | nonce prefix (7)

// Magic starts every encrypted archive.
const Magic = "QTFENC01"

// EncryptedExt is appended to the name of an encrypted archive.
const EncryptedExt = ".enc"

const (
	headerLen  = len(Magic) + 4 + 4 + saltLen + prefixLen
	saltLen    = 16
	prefixLen  = 7
	tagLen     = 16
	maxLogN    = 22
	maxChunk   = 16 << 20
	defaultLog = 15
)

// ChunkSize is the plaintext size of a chunk of new archives.
var ChunkSize = 1 << 20

// KDFLogN is log2 of the scrypt cost of new archives (tests lower it).
var KDFLogN = defaultLog

// ErrPassphrase is returned when an archive cannot be decrypted: the
// passphrase is wrong or the file is damaged.
var ErrPassphrase = errors.New("the archive cannot be decrypted: wrong passphrase or damaged file")

// ErrNoPassphrase is returned when an encrypted archive is opened without
// a passphrase.
var ErrNoPassphrase = errors.New("the archive is encrypted: a passphrase is needed")

// IsEncrypted reports whether head (the first bytes of a file) starts an
// encrypted archive.
func IsEncrypted(head []byte) bool {
	return len(head) >= len(Magic) && string(head[:len(Magic)]) == Magic
}

func deriveKey(pass string, salt []byte, logN, r, p int) ([]byte, error) {
	return scrypt.Key([]byte(pass), salt, 1<<logN, r, p, 32)
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

func nonce(prefix []byte, n uint32, last bool) []byte {
	out := make([]byte, 12)
	copy(out, prefix)
	binary.BigEndian.PutUint32(out[prefixLen:], n)
	if last {
		out[11] = 1
	}
	return out
}

// encWriter seals what is written to it chunk by chunk.
type encWriter struct {
	w      io.Writer
	aead   cipher.AEAD
	ad     []byte
	prefix []byte
	buf    []byte
	chunk  int
	n      uint32
	closed bool
}

// Encrypt returns a writer that encrypts what is written to it with a key
// derived from passphrase and writes it to w. Close writes the last chunk;
// it does not close w.
func Encrypt(w io.Writer, passphrase string) (io.WriteCloser, error) {
	if passphrase == "" {
		return nil, ErrNoPassphrase
	}
	hdr := make([]byte, headerLen)
	copy(hdr, Magic)
	o := len(Magic)
	hdr[o], hdr[o+1], hdr[o+2] = byte(KDFLogN), 8, 1
	binary.BigEndian.PutUint32(hdr[o+4:], uint32(ChunkSize))
	salt := hdr[o+8 : o+8+saltLen]
	prefix := hdr[o+8+saltLen:]
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(prefix); err != nil {
		return nil, err
	}
	key, err := deriveKey(passphrase, salt, KDFLogN, 8, 1)
	if err != nil {
		return nil, err
	}
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(hdr); err != nil {
		return nil, err
	}
	return &encWriter{w: w, aead: aead, ad: hdr, prefix: prefix, chunk: ChunkSize, buf: make([]byte, 0, ChunkSize)}, nil
}

func (e *encWriter) Write(p []byte) (int, error) {
	if e.closed {
		return 0, errors.New("backup: write after close")
	}
	total := 0
	for len(p) > 0 {
		// a full chunk is sealed only once more data follows: the last
		// chunk is sealed by Close with the flag set
		if len(e.buf) == e.chunk {
			if err := e.seal(false); err != nil {
				return total, err
			}
		}
		k := min(e.chunk-len(e.buf), len(p))
		e.buf = append(e.buf, p[:k]...)
		p = p[k:]
		total += k
	}
	return total, nil
}

func (e *encWriter) seal(last bool) error {
	if e.n == ^uint32(0) {
		return errors.New("backup: archive too large to encrypt")
	}
	out := e.aead.Seal(nil, nonce(e.prefix, e.n, last), e.buf, e.ad)
	e.n++
	e.buf = e.buf[:0]
	_, err := e.w.Write(out)
	return err
}

// Close seals the last chunk.
func (e *encWriter) Close() error {
	if e.closed {
		return nil
	}
	e.closed = true
	return e.seal(true)
}

// decReader opens the chunks of an encrypted archive.
type decReader struct {
	r      *bufio.Reader
	aead   cipher.AEAD
	ad     []byte
	prefix []byte
	ct     []byte
	plain  []byte
	n      uint32
	done   bool
}

// Decrypt returns a reader of the plaintext of the encrypted archive r. A
// wrong passphrase or a damaged, truncated or reordered file gives
// ErrPassphrase (or io.ErrUnexpectedEOF) while reading.
func Decrypt(r io.Reader, passphrase string) (io.Reader, error) {
	if passphrase == "" {
		return nil, ErrNoPassphrase
	}
	hdr := make([]byte, headerLen)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, fmt.Errorf("backup: not an encrypted archive: %w", err)
	}
	if !IsEncrypted(hdr) {
		return nil, errors.New("backup: not an encrypted archive")
	}
	o := len(Magic)
	logN, rr, p := int(hdr[o]), int(hdr[o+1]), int(hdr[o+2])
	chunk := int(binary.BigEndian.Uint32(hdr[o+4:]))
	if logN < 1 || logN > maxLogN || rr < 1 || rr > 32 || p < 1 || p > 16 || hdr[o+3] != 0 || chunk < 1 || chunk > maxChunk {
		return nil, errors.New("backup: unsupported encryption parameters")
	}
	key, err := deriveKey(passphrase, hdr[o+8:o+8+saltLen], logN, rr, p)
	if err != nil {
		return nil, err
	}
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	return &decReader{r: bufio.NewReaderSize(r, 64<<10), aead: aead, ad: hdr, prefix: hdr[o+8+saltLen:], ct: make([]byte, chunk+tagLen)}, nil
}

func (d *decReader) Read(p []byte) (int, error) {
	for len(d.plain) == 0 {
		if d.done {
			return 0, io.EOF
		}
		if err := d.next(); err != nil {
			return 0, err
		}
	}
	n := copy(p, d.plain)
	d.plain = d.plain[n:]
	return n, nil
}

func (d *decReader) next() error {
	n, err := io.ReadFull(d.r, d.ct)
	last := false
	switch {
	case err == io.EOF || err == io.ErrUnexpectedEOF:
		last = true
	case err != nil:
		return err
	default:
		if _, perr := d.r.Peek(1); perr == io.EOF {
			last = true
		} else if perr != nil {
			return perr
		}
	}
	if n < tagLen {
		return io.ErrUnexpectedEOF
	}
	plain, err := d.aead.Open(d.ct[:0:0], nonce(d.prefix, d.n, last), d.ct[:n], d.ad)
	if err != nil {
		return ErrPassphrase
	}
	d.n++
	d.plain, d.done = plain, last
	return nil
}

// Archive is a backup archive opened for reading: the file itself, or a
// decrypted temporary copy of an encrypted one.
type Archive struct {
	*os.File
	Size      int64
	Encrypted bool
	temp      string
}

// Close closes the archive and removes a decrypted copy.
func (a *Archive) Close() error {
	err := a.File.Close()
	if a.temp != "" {
		_ = os.Remove(a.temp)
	}
	return err
}

// Sniff reports whether the file at path is an encrypted archive.
func Sniff(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	head := make([]byte, len(Magic))
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return false, err
	}
	return IsEncrypted(head[:n]), nil
}

// Open opens the archive at path for reading. An encrypted archive is
// decrypted with passphrase into a temporary file in tmpDir ("" = the
// system's), which Close removes.
func Open(path, passphrase, tmpDir string) (*Archive, error) {
	enc, err := Sniff(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if !enc {
		st, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, err
		}
		return &Archive{File: f, Size: st.Size()}, nil
	}
	defer f.Close()
	return DecryptToTemp(f, passphrase, tmpDir)
}

// DecryptToTemp decrypts the encrypted archive r into a temporary file in
// tmpDir ("" = the system's), which the Archive's Close removes.
func DecryptToTemp(r io.Reader, passphrase, tmpDir string) (*Archive, error) {
	dr, err := Decrypt(r, passphrase)
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(tmpDir, ".qtf-restore-*")
	if err != nil {
		return nil, err
	}
	a := &Archive{File: tmp, Encrypted: true, temp: tmp.Name()}
	n, err := io.Copy(tmp, dr)
	if err == nil {
		_, err = tmp.Seek(0, io.SeekStart)
	}
	if err != nil {
		a.Close()
		return nil, err
	}
	a.Size = n
	return a, nil
}

// TempDirFor is the directory for a decrypted copy of the archive at path:
// next to it when writable, else the system's temporary directory.
func TempDirFor(path string) string {
	dir := filepath.Dir(path)
	if f, err := os.CreateTemp(dir, ".qtf-probe-*"); err == nil {
		name := f.Name()
		f.Close()
		os.Remove(name)
		return dir
	}
	return ""
}
