package backup

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func init() { KDFLogN = 4 } // fast key derivation in tests

func encrypt(t *testing.T, plain []byte, pass string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := Encrypt(&buf, pass)
	if err != nil {
		t.Fatal(err)
	}
	// odd write sizes cross chunk boundaries
	for p := plain; len(p) > 0; {
		n := min(len(p), 7)
		if _, err := w.Write(p[:n]); err != nil {
			t.Fatal(err)
		}
		p = p[n:]
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil { // idempotent
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("x")); err == nil {
		t.Error("write after close accepted")
	}
	return buf.Bytes()
}

func decrypt(ct []byte, pass string) ([]byte, error) {
	r, err := Decrypt(bytes.NewReader(ct), pass)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

func TestEncryptRoundTrip(t *testing.T) {
	old := ChunkSize
	ChunkSize = 64
	defer func() { ChunkSize = old }()
	for _, size := range []int{0, 1, 63, 64, 65, 128, 1000} {
		plain := make([]byte, size)
		_, _ = rand.Read(plain)
		ct := encrypt(t, plain, "correct horse battery")
		if !IsEncrypted(ct) || IsEncrypted(ct[:3]) || IsEncrypted([]byte("PK\x03\x04xxxxxx")) {
			t.Fatal("magic")
		}
		got, err := decrypt(ct, "correct horse battery")
		if err != nil || !bytes.Equal(got, plain) {
			t.Fatalf("size %d: %v (%d bytes)", size, err, len(got))
		}
		if _, err := decrypt(ct, "wrong passphrase"); !errors.Is(err, ErrPassphrase) {
			t.Errorf("size %d: wrong passphrase: %v", size, err)
		}
		// truncation at a chunk boundary and inside a chunk
		chunk := 64 + tagLen
		if size >= 128 {
			cut := headerLen + chunk
			if _, err := decrypt(ct[:cut], "correct horse battery"); err == nil {
				t.Errorf("size %d: truncated at a chunk boundary accepted", size)
			}
		}
		if _, err := decrypt(ct[:len(ct)-1], "correct horse battery"); err == nil {
			t.Errorf("size %d: truncated archive accepted", size)
		}
		bad := slices.Clone(ct)
		bad[len(bad)-3] ^= 1
		if _, err := decrypt(bad, "correct horse battery"); err == nil {
			t.Errorf("size %d: modified archive accepted", size)
		}
	}
}

func TestDecryptErrors(t *testing.T) {
	if _, err := Encrypt(io.Discard, ""); !errors.Is(err, ErrNoPassphrase) {
		t.Error(err)
	}
	if _, err := Decrypt(bytes.NewReader(nil), ""); !errors.Is(err, ErrNoPassphrase) {
		t.Error(err)
	}
	if _, err := Decrypt(bytes.NewReader([]byte("short")), "p"); err == nil {
		t.Error("short header accepted")
	}
	if _, err := Decrypt(bytes.NewReader(make([]byte, headerLen)), "p"); err == nil {
		t.Error("no magic accepted")
	}
	ct := encrypt(t, []byte("hello"), "p")
	bad := slices.Clone(ct)
	bad[len(Magic)] = 40 // scrypt cost far too high
	if _, err := Decrypt(bytes.NewReader(bad), "p"); err == nil {
		t.Error("bad parameters accepted")
	}
	// header only: no chunk
	if _, err := decrypt(ct[:headerLen+3], "p"); err == nil {
		t.Error("missing chunk accepted")
	}
	// a writer that fails
	if _, err := Encrypt(failWriter{}, "p"); err == nil {
		t.Error("write error ignored")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestOpenArchive(t *testing.T) {
	dir := t.TempDir()
	plainPath := filepath.Join(dir, "plain.zip")
	if err := os.WriteFile(plainPath, []byte("PK plain"), 0o600); err != nil {
		t.Fatal(err)
	}
	encPath := filepath.Join(dir, "enc.zip.enc")
	if err := os.WriteFile(encPath, encrypt(t, []byte("PK secret"), "pass phrase"), 0o600); err != nil {
		t.Fatal(err)
	}
	if e, err := Sniff(plainPath); err != nil || e {
		t.Fatal(e, err)
	}
	if e, err := Sniff(encPath); err != nil || !e {
		t.Fatal(e, err)
	}
	if _, err := Sniff(filepath.Join(dir, "missing")); err == nil {
		t.Error("missing file")
	}
	a, err := Open(plainPath, "", "")
	if err != nil || a.Encrypted || a.Size != 8 {
		t.Fatal(a, err)
	}
	a.Close()
	a, err = Open(encPath, "pass phrase", TempDirFor(encPath))
	if err != nil || !a.Encrypted || a.Size != 9 {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(a)
	tmp := a.temp
	a.Close()
	if string(b) != "PK secret" || filepath.Dir(tmp) != dir {
		t.Errorf("%q in %s", b, tmp)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("temporary copy kept")
	}
	if _, err := Open(encPath, "", ""); !errors.Is(err, ErrNoPassphrase) {
		t.Error(err)
	}
	if _, err := Open(encPath, "wrong", ""); !errors.Is(err, ErrPassphrase) {
		t.Error(err)
	}
	if _, err := Open(filepath.Join(dir, "missing"), "", ""); err == nil {
		t.Error("missing file opened")
	}
	if TempDirFor(filepath.Join(dir, "nope", "x")) != "" {
		t.Error("unwritable directory chosen")
	}
}

func TestSchedule(t *testing.T) {
	for in, want := range map[string]string{
		"daily 02:00": "daily 02:00", " Daily  2:30 ": "daily 02:30", "weekly sun 23:59": "weekly sun 23:59",
		"WEEKLY Mon 00:00": "weekly mon 00:00", "every 6h": "every 6h", "every 90m": "every 1h30m", "every 15m": "every 15m",
	} {
		s, err := ParseSchedule(in)
		if err != nil || s.String() != want {
			t.Errorf("%q: %q %v", in, s.String(), err)
		}
		if CheckSchedule(in) != nil {
			t.Errorf("%q rejected", in)
		}
	}
	for _, in := range []string{"", "daily", "daily 24:00", "daily 2:3", "daily 02:60", "daily x:00", "weekly 02:00", "weekly xyz 02:00",
		"every 5m", "every 1000h", "every", "every x", "hourly", "daily 02:00 extra"} {
		if _, err := ParseSchedule(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
	if (Schedule{}).String() != "" {
		t.Error("empty schedule")
	}
}

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestScheduleDue(t *testing.T) {
	daily, _ := ParseSchedule("daily 02:00")
	now := at("2026-11-24T10:00:00Z")
	// never ran: record the latest slot, run at the next one
	due, rec := daily.Due(now, nil)
	if due || !rec.Equal(at("2026-11-24T02:00:00Z")) {
		t.Fatal(due, rec)
	}
	if n := daily.Next(now, nil); !n.Equal(at("2026-11-25T02:00:00Z")) {
		t.Error(n)
	}
	if due, _ := daily.Due(now, &rec); due {
		t.Error("slot run twice")
	}
	if n := daily.Next(now, &rec); !n.Equal(at("2026-11-25T02:00:00Z")) {
		t.Error(n)
	}
	// before 02:00 the latest slot is yesterday's
	early := at("2026-11-25T01:59:00Z")
	if due, _ := daily.Due(early, &rec); due {
		t.Error("due before the slot")
	}
	due, rec2 := daily.Due(at("2026-11-25T02:00:30Z"), &rec)
	if !due || !rec2.Equal(at("2026-11-25T02:00:00Z")) {
		t.Fatal(due, rec2)
	}
	// a missed slot runs at the next check
	old := at("2026-11-20T02:00:00Z")
	if due, _ := daily.Due(now, &old); !due {
		t.Error("missed slot not run")
	}
	if n := daily.Next(now, &old); !n.Equal(now) {
		t.Error(n)
	}

	weekly, _ := ParseSchedule("weekly mon 03:00") // 2026-11-23 is a Monday
	due, rec = weekly.Due(now, nil)
	if due || !rec.Equal(at("2026-11-23T03:00:00Z")) {
		t.Fatal(due, rec)
	}
	if n := weekly.Next(now, &rec); !n.Equal(at("2026-11-30T03:00:00Z")) {
		t.Error(n)
	}
	mon := at("2026-11-23T02:00:00Z") // Monday before the slot: last week's
	if _, rec := weekly.Due(mon, nil); !rec.Equal(at("2026-11-16T03:00:00Z")) {
		t.Error(rec)
	}

	every, _ := ParseSchedule("every 6h")
	if due, rec := every.Due(now, nil); !due || !rec.Equal(now) {
		t.Error(due, rec)
	}
	if n := every.Next(now, nil); !n.Equal(now) {
		t.Error(n)
	}
	last := now.Add(-time.Hour)
	if due, _ := every.Due(now, &last); due {
		t.Error("interval not waited for")
	}
	if n := every.Next(now, &last); !n.Equal(now.Add(5 * time.Hour)) {
		t.Error(n)
	}
	last = now.Add(-7 * time.Hour)
	if due, _ := every.Due(now, &last); !due {
		t.Error("interval passed")
	}
	if n := every.Next(now, &last); !n.Equal(now) {
		t.Error(n)
	}
}

func TestFileNamesAndPrune(t *testing.T) {
	t0 := at("2026-11-24T02:00:00Z")
	if n := FileName(t0, false); n != "qtestframe-backup-20261124-020000.zip" || !IsBackupFile(n) {
		t.Fatal(n)
	}
	if n := FileName(t0, true); n != "qtestframe-backup-20261124-020000.zip.enc" || !IsBackupFile(n) {
		t.Fatal(n)
	}
	for _, n := range []string{"../qtestframe-backup-20261124-020000.zip", "qtestframe-backup-x.zip", "other.zip", "qtestframe-backup-20261124-020000.zip.bak"} {
		if IsBackupFile(n) {
			t.Errorf("%s accepted", n)
		}
	}
	dir := t.TempDir()
	if n := FreeName(dir, t0, false); n != FileName(t0, false) {
		t.Fatal(n)
	}
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(FileName(t0, false))
	if n := FreeName(dir, t0, false); n != "qtestframe-backup-20261124-020000-1.zip" || !IsBackupFile(n) {
		t.Fatal(n)
	}
	if n := FreeName(dir, t0, true); n != FileName(t0, true) {
		t.Fatal(n)
	}
	for d := 1; d <= 9; d++ {
		write(FileName(t0.AddDate(0, 0, -d), d%2 == 0))
	}
	write("notes.txt")
	write("qtestframe-backup-latest.zip")
	if err := os.Mkdir(filepath.Join(dir, FileName(t0.AddDate(0, 0, -30), false)), 0o700); err != nil {
		t.Fatal(err)
	}
	names := func() []string {
		ents, _ := os.ReadDir(dir)
		var out []string
		for _, e := range ents {
			out = append(out, e.Name())
		}
		return out
	}
	if del, err := Prune(dir, 0, 0, t0); err != nil || del != nil {
		t.Fatal(del, err)
	}
	del, err := Prune(dir, 5, 0, t0)
	if err != nil || len(del) != 5 {
		t.Fatal(del, err)
	}
	// keep 5 newest; with days: older than 2 days go too
	del, err = Prune(dir, 5, 2, t0)
	if err != nil || len(del) != 2 {
		t.Fatal(del, err)
	}
	left := names()
	if len(left) != 6 || !slices.Contains(left, "notes.txt") || !slices.Contains(left, "qtestframe-backup-latest.zip") {
		t.Errorf("left %v", left)
	}
	// the newest file is kept even when it is too old
	del, err = Prune(dir, 0, 1, t0.AddDate(0, 1, 0))
	if err != nil || len(del) != 2 {
		t.Fatal(del, err)
	}
	if _, err := Prune(filepath.Join(dir, "missing"), 1, 0, t0); err == nil {
		t.Error("missing directory")
	}
	sum, n, err := FileSHA256(filepath.Join(dir, "notes.txt"))
	h := sha256.Sum256([]byte("x"))
	if err != nil || n != 1 || sum != hex.EncodeToString(h[:]) {
		t.Error(sum, n, err)
	}
	if _, _, err := FileSHA256(filepath.Join(dir, "missing")); err == nil {
		t.Error("missing file")
	}
}

func TestS3Upload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "qtestframe-backup-20261124-020000.zip")
	body := []byte("archive bytes")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	sum, _, _ := FileSHA256(path)
	var got *http.Request
	var gotBody []byte
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
		if status != http.StatusOK {
			_, _ = w.Write([]byte(`<?xml version="1.0"?><Error><Code>AccessDenied</Code><Message>Access Denied</Message></Error>`))
		}
	}))
	defer srv.Close()
	c := S3{Endpoint: srv.URL, Region: "eu-north-1", Bucket: "qtf-backups", Prefix: "/nightly/", AccessKeyID: "<access-key-id>", SecretAccessKey: "<secret>"}
	key := c.Key("qtestframe-backup-20261124-020000.zip")
	if key != "nightly/qtestframe-backup-20261124-020000.zip" || (S3{}).Key("a") != "a" {
		t.Fatal(key)
	}
	now := at("2026-11-24T02:00:05Z")
	if err := c.Upload(context.Background(), srv.Client(), key, path, sum, now); err != nil {
		t.Fatal(err)
	}
	if got.Method != http.MethodPut || got.URL.Path != "/qtf-backups/nightly/qtestframe-backup-20261124-020000.zip" || !bytes.Equal(gotBody, body) {
		t.Fatalf("%s %s %q", got.Method, got.URL.Path, gotBody)
	}
	auth := got.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=<access-key-id>/20261124/eu-north-1/s3/aws4_request, SignedHeaders=content-type;host;x-amz-content-sha256;x-amz-date, Signature=") ||
		got.Header.Get("X-Amz-Content-Sha256") != sum || got.Header.Get("X-Amz-Date") != "20261124T020005Z" {
		t.Errorf("%s %v", auth, got.Header)
	}
	// the signature is deterministic and depends on the secret
	req1, _ := http.NewRequest(http.MethodPut, srv.URL+"/b/k", nil)
	req2, _ := http.NewRequest(http.MethodPut, srv.URL+"/b/k", nil)
	signV4(req1, sum, "id", "one", "r", "s3", now)
	signV4(req2, sum, "id", "two", "r", "s3", now)
	if req1.Header.Get("Authorization") == req2.Header.Get("Authorization") {
		t.Error("secret not used")
	}
	status = http.StatusForbidden
	if err := c.Upload(context.Background(), srv.Client(), key, path, sum, now); err == nil || !strings.Contains(err.Error(), "AccessDenied Access Denied") {
		t.Errorf("%v", err)
	}
	if err := c.Upload(context.Background(), srv.Client(), key, filepath.Join(dir, "missing"), sum, now); err == nil {
		t.Error("missing file uploaded")
	}
	bad := c
	bad.Endpoint = "ftp://x"
	if err := bad.Upload(context.Background(), nil, key, path, sum, now); err == nil {
		t.Error("bad endpoint")
	}
	srv.Close()
	if err := c.Upload(context.Background(), nil, key, path, sum, now); err == nil {
		t.Error("closed server")
	}
}

func TestS3Check(t *testing.T) {
	ok := S3{Endpoint: "https://s3.example.com", Region: "us-east-1", Bucket: "b", AccessKeyID: "id", SecretAccessKey: "s"}
	if err := ok.Check(); err != nil {
		t.Fatal(err)
	}
	for _, f := range []func(*S3){
		func(c *S3) { c.Endpoint = "" }, func(c *S3) { c.Endpoint = "https://s3.example.com/path" }, func(c *S3) { c.Endpoint = "https://s3.example.com?x=1" },
		func(c *S3) { c.Bucket = "" }, func(c *S3) { c.Region = "" }, func(c *S3) { c.SecretAccessKey = "" },
	} {
		c := ok
		f(&c)
		if c.Check() == nil {
			t.Errorf("%+v accepted", c)
		}
	}
	if s3Message([]byte(" plain\ntext ")) != "plain text" {
		t.Error("message")
	}
	if uriEncode("a b/ü~", true) != "a%20b/%C3%BC~" || uriEncode("a/b", false) != "a%2Fb" {
		t.Error(uriEncode("a b/ü~", true))
	}
	if canonicalQuery(map[string][]string{"b": {"2", "1"}, "a": {"x y"}}) != "a=x%20y&b=1&b=2" {
		t.Error(canonicalQuery(map[string][]string{"b": {"2", "1"}, "a": {"x y"}}))
	}
}
