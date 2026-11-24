package store_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/testutil"
)

func TestBackupRunsAndLock(t *testing.T) {
	ctx := context.Background()
	st := store.New(testutil.OpenDB(t))
	now := time.Date(2026, 11, 24, 2, 0, 0, 0, time.UTC)
	st.SetClock(func() time.Time { return now })

	ok, err := st.AcquireBackupLock(ctx, "a", 5*time.Minute)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, _ := st.AcquireBackupLock(ctx, "b", 5*time.Minute); ok {
		t.Fatal("second holder got the lock")
	}
	if ok, _ := st.AcquireBackupLock(ctx, "a", 6*time.Minute); !ok {
		t.Fatal("holder could not take its lock again")
	}
	if ok, _ := st.RenewBackupLock(ctx, "b", time.Hour); ok {
		t.Fatal("other holder renewed")
	}
	if ok, _ := st.RenewBackupLock(ctx, "a", 10*time.Minute); !ok {
		t.Fatal("not renewed")
	}
	// the lease of a crashed holder expires
	now = now.Add(11 * time.Minute)
	if ok, _ := st.AcquireBackupLock(ctx, "b", 5*time.Minute); !ok {
		t.Fatal("expired lease kept")
	}
	if err := st.ReleaseBackupLock(ctx, "a"); err != nil { // not the holder: no-op
		t.Fatal(err)
	}
	if ok, _ := st.AcquireBackupLock(ctx, "a", 5*time.Minute); ok {
		t.Fatal("released by the wrong holder")
	}
	if err := st.ReleaseBackupLock(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := st.AcquireBackupLock(ctx, "a", 5*time.Minute); !ok {
		t.Fatal("released lock not free")
	}

	r1 := &store.BackupRun{Kind: "schedule", StartedBy: "system", Instance: "a"}
	if err := st.CreateBackupRun(ctx, r1); err != nil || r1.ID == "" || r1.Status != store.BackupRunning {
		t.Fatal(r1, err)
	}
	now = now.Add(time.Minute)
	r1.Status, r1.FileName, r1.Size, r1.SHA256, r1.Encrypted, r1.Rows, r1.SchemaVersion = store.BackupSucceeded, "qtestframe-backup-1.zip", 42, "ab", true, 7, 65
	r1.RemoteKey, r1.Error = "nightly/qtestframe-backup-1.zip", strings.Repeat("é", 3000)
	if err := st.FinishBackupRun(ctx, r1); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	r2 := &store.BackupRun{Kind: "manual", StartedBy: "root@example.com", Instance: "b"}
	if err := st.CreateBackupRun(ctx, r2); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetBackupRun(ctx, r1.ID)
	if err != nil || got.FileName != r1.FileName || got.Size != 42 || !got.Encrypted || got.Rows != 7 || got.SchemaVersion != 65 ||
		got.FinishedAt == nil || got.RemoteKey == "" || len([]rune(got.Error)) != 2000 || got.Instance != "a" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := st.GetBackupRun(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Error(err)
	}
	list, err := st.ListBackupRuns(ctx, 0)
	if err != nil || len(list) != 2 || list[0].ID != r2.ID {
		t.Fatalf("%+v %v", list, err)
	}
	if list, _ := st.ListBackupRuns(ctx, 1); len(list) != 1 {
		t.Error(len(list))
	}
	if err := st.MarkBackupFilesPruned(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkBackupFilesPruned(ctx, []string{"qtestframe-backup-1.zip", "other.zip"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetBackupVerified(ctx, r1.ID, "damaged"); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetBackupRun(ctx, r1.ID)
	if got.PrunedAt == nil || got.VerifiedAt == nil || got.VerifyError != "damaged" {
		t.Errorf("%+v", got)
	}
	// running backups of another instance are interrupted
	n, err := st.FailStaleBackupRuns(ctx, "a")
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	got, _ = st.GetBackupRun(ctx, r2.ID)
	if got.Status != store.BackupFailed || !strings.Contains(got.Error, "interrupted") || got.FinishedAt == nil {
		t.Errorf("%+v", got)
	}
	if n, _ := st.FailStaleBackupRuns(ctx, "a"); n != 0 {
		t.Error(n)
	}
}

func TestVerifyBackup(t *testing.T) {
	ctx := context.Background()
	st := sqliteStore(t)
	seed(t, st)
	var buf bytes.Buffer
	m, err := st.Backup(ctx, &buf, "test")
	if err != nil {
		t.Fatal(err)
	}
	good := buf.Bytes()
	got, err := store.VerifyBackup(bytes.NewReader(good), int64(len(good)))
	if err != nil || got.Rows() != m.Rows() || got.SchemaVersion != m.SchemaVersion {
		t.Fatal(got, err)
	}
	// rebuild the archive with a change to one entry
	rewrite := func(f func(name string, data []byte) ([]byte, bool)) []byte {
		zr, err := zip.NewReader(bytes.NewReader(good), int64(len(good)))
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		zw := zip.NewWriter(&out)
		for _, e := range zr.File {
			rc, _ := e.Open()
			var b bytes.Buffer
			_, _ = b.ReadFrom(rc)
			rc.Close()
			data, keep := f(e.Name, b.Bytes())
			if !keep {
				continue
			}
			w, _ := zw.Create(e.Name)
			_, _ = w.Write(data)
		}
		_ = zw.Close()
		return out.Bytes()
	}
	cases := map[string][]byte{
		"not a zip":     []byte("nope"),
		"no manifest":   rewrite(func(n string, d []byte) ([]byte, bool) { return d, n != "manifest.json" }),
		"missing table": rewrite(func(n string, d []byte) ([]byte, bool) { return d, n != "users.jsonl" }),
		"row count": rewrite(func(n string, d []byte) ([]byte, bool) {
			if n == "users.jsonl" {
				return append(d, []byte(`{"id":"extra"}`+"\n")...), true
			}
			return d, true
		}),
		"bad json": rewrite(func(n string, d []byte) ([]byte, bool) {
			if n == "tenants.jsonl" {
				return append([]byte("[1]\n"), d...), true
			}
			return d, true
		}),
		"bad manifest": rewrite(func(n string, d []byte) ([]byte, bool) {
			if n == "manifest.json" {
				return []byte(`{"format_version": 99}`), true
			}
			return d, true
		}),
		"unlisted table": rewrite(func(n string, d []byte) ([]byte, bool) {
			if n == "manifest.json" {
				return bytes.Replace(d, []byte(`"users":`), []byte(`"users_x":`), 1), true
			}
			return d, true
		}),
	}
	// a flipped bit inside a compressed entry fails its checksum
	damaged := bytes.Clone(good)
	damaged[len(damaged)/3] ^= 0xff
	cases["damaged"] = damaged
	for name, b := range cases {
		if _, err := store.VerifyBackup(bytes.NewReader(b), int64(len(b))); !errors.Is(err, store.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// a table file the manifest does not list
	extra := rewrite(func(n string, d []byte) ([]byte, bool) {
		if n == "manifest.json" {
			return bytes.Replace(d, []byte(`"users": `), []byte(`"ignored": 0, "users": `), 1), true
		}
		return d, true
	})
	if _, err := store.VerifyBackup(bytes.NewReader(extra), int64(len(extra))); !errors.Is(err, store.ErrInvalid) {
		t.Errorf("extra: %v", err)
	}
	old := store.MaxRestoreLine
	store.MaxRestoreLine = 16
	defer func() { store.MaxRestoreLine = old }()
	if _, err := store.VerifyBackup(bytes.NewReader(good), int64(len(good))); !errors.Is(err, store.ErrInvalid) || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("long line: %v", err)
	}
}
