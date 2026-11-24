package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/backup"
	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/mail"
	"github.com/rom/qtestframe/internal/runner"
	"github.com/rom/qtestframe/internal/store"
)

func init() { backup.KDFLogN = 4 } // fast key derivation in tests

const testPassphrase = "<test passphrase>"

// backupEnv is an env with scheduled backups into a temporary directory
// and a clock the test moves.
func backupEnv(t *testing.T, mut ...func(*config.Config)) (*env, string, *time.Time) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "backups")
	e := newEnv(t, append([]func(*config.Config){func(c *config.Config) {
		c.Backup.Enabled, c.Backup.Directory, c.Backup.Schedule = true, dir, "every 1h"
		c.Email.From = "qtf@example.test"
	}}, mut...)...)
	now := time.Date(2026, 11, 24, 2, 0, 0, 0, time.UTC)
	e.srv.Clock = func() time.Time { return now }
	return e, dir, &now
}

func systemAuditActions(t *testing.T, e *env) map[string]string {
	t.Helper()
	entries, err := e.st.ListAudit(context.Background(), "", 1000)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, en := range entries {
		out[en.Action] = en.Details
	}
	return out
}

func listBackups(t *testing.T, c *client) (backupSettingsView, []store.BackupRun, bool) {
	t.Helper()
	var r struct {
		Settings backupSettingsView `json:"settings"`
		Runs     []store.BackupRun  `json:"runs"`
		Running  bool               `json:"running"`
	}
	c.call("GET", "/api/v1/admin/backups", nil, 200, &r)
	return r.Settings, r.Runs, r.Running
}

func TestScheduledBackups(t *testing.T) {
	e, dir, now := backupEnv(t, func(c *config.Config) {
		c.Backup.Passphrase, c.Backup.KeepLast = testPassphrase, 2
	})
	ctx := context.Background()
	a := e.login("admin@example.com")
	a.project("BAK")

	ran, err := e.srv.RunBackupSchedule(ctx)
	if err != nil || !ran {
		t.Fatal(ran, err)
	}
	if ran, _ := e.srv.RunBackupSchedule(ctx); ran {
		t.Fatal("ran again within the interval")
	}
	set, runs, running := listBackups(t, a)
	if len(runs) != 1 || running {
		t.Fatalf("%+v", runs)
	}
	r := runs[0]
	if r.Status != store.BackupSucceeded || r.Kind != "schedule" || r.StartedBy != "system" || !r.Encrypted || r.Rows == 0 || r.Size == 0 ||
		r.FileName != "qtestframe-backup-20261124-020000.zip.enc" || r.SchemaVersion == 0 {
		t.Fatalf("%+v", r)
	}
	if !set.Enabled || !set.Encrypted || set.Schedule != "every 1h" || set.KeepLast != 2 || set.NextRunAt == nil || !set.NextRunAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("%+v", set)
	}
	// no secret in the response
	res, body := a.getBytes("/api/v1/admin/backups", 200)
	_ = res
	if bytes.Contains(body, []byte(testPassphrase)) {
		t.Fatal("passphrase in the response")
	}
	sum, _, _ := backup.FileSHA256(filepath.Join(dir, r.FileName))
	if sum != r.SHA256 {
		t.Fatal("sha256", sum, r.SHA256)
	}
	if st, err := os.Stat(dir); err != nil || st.Mode().Perm() != 0o700 {
		t.Fatal(st, err)
	}
	if acts := systemAuditActions(t, e); !strings.Contains(acts["admin.backup.completed"], r.FileName) {
		t.Fatal(acts)
	}

	// download (audited)
	res, data := a.getBytes("/api/v1/admin/backups/"+r.ID+"/download", 200)
	if res.Header.Get("Content-Type") != "application/octet-stream" || !strings.Contains(res.Header.Get("Content-Disposition"), r.FileName) || !backup.IsEncrypted(data) {
		t.Fatal(res.Header)
	}
	if !auditActions(t, e)["admin.backup.download"] {
		t.Fatal("download not audited")
	}
	// verify: intact
	var chk backupCheck
	a.call("POST", "/api/v1/admin/backups/"+r.ID+"/verify", nil, 200, &chk)
	if !chk.OK || !chk.SHA256OK || !chk.Encrypted || chk.Manifest == nil || chk.Manifest.Rows() != int(r.Rows) {
		t.Fatalf("%+v", chk)
	}
	if !auditActions(t, e)["admin.backup.verify"] {
		t.Fatal("verify not audited")
	}
	if got, _ := e.st.GetBackupRun(ctx, r.ID); got.VerifiedAt == nil || got.VerifyError != "" {
		t.Fatalf("%+v", got)
	}

	// two more runs: the first file goes (keep_last 2)
	*now = now.Add(time.Hour)
	if ran, _ := e.srv.RunBackupSchedule(ctx); !ran {
		t.Fatal("not due after the interval")
	}
	*now = now.Add(time.Hour)
	if ran, _ := e.srv.RunBackupSchedule(ctx); !ran {
		t.Fatal("not due after the interval")
	}
	_, runs, _ = listBackups(t, a)
	if len(runs) != 3 || runs[2].PrunedAt == nil || runs[0].PrunedAt != nil {
		t.Fatalf("%+v", runs)
	}
	if _, err := os.Stat(filepath.Join(dir, r.FileName)); !os.IsNotExist(err) {
		t.Fatal("pruned file kept")
	}
	a.m("GET", "/api/v1/admin/backups/"+r.ID+"/download", nil, 410)
	a.m("POST", "/api/v1/admin/backups/"+r.ID+"/verify", nil, 410)
	a.m("GET", "/api/v1/admin/backups/nope/download", nil, 404)

	// a damaged file fails the verification
	latest := runs[0]
	path := filepath.Join(dir, latest.FileName)
	b, _ := os.ReadFile(path)
	b[len(b)/2] ^= 0xff
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	a.call("POST", "/api/v1/admin/backups/"+latest.ID+"/verify", nil, 200, &chk)
	if chk.OK || chk.SHA256OK || !strings.Contains(chk.Error, "SHA-256") {
		t.Fatalf("%+v", chk)
	}
	if got, _ := e.st.GetBackupRun(ctx, latest.ID); got.VerifyError == "" {
		t.Fatal("verification failure not recorded")
	}
	// a file that is gone
	os.Remove(path)
	a.m("GET", "/api/v1/admin/backups/"+latest.ID+"/download", nil, 404)

	// only super administrators
	e.member("viewer@example.com", "admin")
	v := e.login("viewer@example.com")
	v.m("GET", "/api/v1/admin/backups", nil, 403)
	v.m("POST", "/api/v1/admin/backups/run", nil, 403)
	v.m("GET", "/api/v1/admin/backups/"+runs[1].ID+"/download", nil, 403)
	v.m("POST", "/api/v1/admin/backups/"+runs[1].ID+"/verify", nil, 403)
}

func TestVerifyBackupFile(t *testing.T) {
	dir := t.TempDir()
	if c := verifyBackupFile(filepath.Join(dir, "missing"), "", ""); c.OK || c.Error == "" {
		t.Fatal(c)
	}
	path := filepath.Join(dir, "x.zip.enc")
	var buf bytes.Buffer
	w, _ := backup.Encrypt(&buf, testPassphrase)
	w.Write([]byte("not a zip"))
	w.Close()
	os.WriteFile(path, buf.Bytes(), 0o600)
	if c := verifyBackupFile(path, "", ""); c.OK || !c.Encrypted || !strings.Contains(c.Error, "passphrase") {
		t.Fatalf("%+v", c)
	}
	if c := verifyBackupFile(path, "", testPassphrase); c.OK || !c.Encrypted || !strings.Contains(c.Error, "ZIP") {
		t.Fatalf("%+v", c)
	}
}

func TestRunBackupNow(t *testing.T) {
	e, dir, _ := backupEnv(t, func(c *config.Config) { c.Backup.Enabled = false })
	a := e.login("admin@example.com")
	var run store.BackupRun
	a.call("POST", "/api/v1/admin/backups/run", nil, 202, &run)
	if run.Kind != "manual" || run.StartedBy != "admin@example.com" || run.Status != store.BackupRunning {
		t.Fatalf("%+v", run)
	}
	e.srv.WaitBackups()
	set, runs, _ := listBackups(t, a)
	if len(runs) != 1 || runs[0].Status != store.BackupSucceeded || runs[0].Encrypted || set.NextRunAt != nil {
		t.Fatalf("%+v %+v", runs, set)
	}
	if _, err := os.Stat(filepath.Join(dir, runs[0].FileName)); err != nil {
		t.Fatal(err)
	}
	if !auditActions(t, e)["admin.backup.run"] {
		t.Fatal("not audited")
	}
	res, data := a.getBytes("/api/v1/admin/backups/"+runs[0].ID+"/download", 200)
	if res.Header.Get("Content-Type") != "application/zip" || !bytes.HasPrefix(data, []byte("PK")) {
		t.Fatal(res.Header)
	}
	// scheduled backups are off: nothing runs on the schedule
	if ran, err := e.srv.RunBackupSchedule(context.Background()); ran || err != nil {
		t.Fatal(ran, err)
	}
	// another server holds the lock
	if ok, _ := e.st.AcquireBackupLock(context.Background(), "other-instance", time.Hour); !ok {
		t.Fatal("lock")
	}
	a.m("POST", "/api/v1/admin/backups/run", nil, 409)
}

func TestRunBackupNowNeedsDirectory(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	a.m("POST", "/api/v1/admin/backups/run", nil, 400)
	set, runs, _ := listBackups(t, a)
	if set.Enabled || len(runs) != 0 || set.Schedule != "daily 02:00" {
		t.Fatalf("%+v %+v", set, runs)
	}
	if ran, err := e.srv.RunBackupSchedule(context.Background()); ran || err != nil {
		t.Fatal(ran, err)
	}
}

func TestBackupFailureNotifies(t *testing.T) {
	// the directory cannot be created: a file is in the way
	base := t.TempDir()
	blocker := filepath.Join(base, "file")
	os.WriteFile(blocker, []byte("x"), 0o600)
	e, _, _ := backupEnv(t, func(c *config.Config) { c.Backup.Directory = filepath.Join(blocker, "backups") })
	f := &mail.Fake{}
	e.srv.Mail = f
	mkUser(t, e.st, "other-root@example.com", true)
	if ran, err := e.srv.RunBackupSchedule(context.Background()); !ran || err != nil {
		t.Fatal(ran, err)
	}
	runs, _ := e.st.ListBackupRuns(context.Background(), 10)
	if len(runs) != 1 || runs[0].Status != store.BackupFailed || runs[0].Error == "" {
		t.Fatalf("%+v", runs)
	}
	msgs := sent(e, f)
	if len(msgs) != 2 || !strings.Contains(msgs[0].Subject, "backup failed") || !strings.Contains(msgs[0].Text, runs[0].Error) {
		t.Fatalf("%d %+v", len(msgs), msgs)
	}
	if acts := systemAuditActions(t, e); acts["admin.backup.failed"] == "" {
		t.Fatal(acts)
	}
	// the lock is released after a failure
	if ok, _ := e.st.AcquireBackupLock(context.Background(), "x", time.Minute); !ok {
		t.Fatal("lock kept")
	}
}

func TestDailyBackupScheduleAndCluster(t *testing.T) {
	e, _, now := backupEnv(t, func(c *config.Config) { c.Backup.Schedule = "daily 03:00" })
	ctx := context.Background()
	*now = time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	// first check: the latest slot is recorded, nothing runs
	if ran, err := e.srv.RunBackupSchedule(ctx); ran || err != nil {
		t.Fatal(ran, err)
	}
	a := e.login("admin@example.com")
	set, _, _ := listBackups(t, a)
	if set.NextRunAt == nil || !set.NextRunAt.Equal(time.Date(2026, 11, 25, 3, 0, 0, 0, time.UTC)) || set.Schedule != "daily 03:00" {
		t.Fatalf("%+v", set)
	}
	// a second server on the same database
	srv2, err := New(e.st, e.cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), runner.New(e.cfg.Runner), nil)
	if err != nil {
		t.Fatal(err)
	}
	srv2.Clock = e.srv.Clock
	*now = time.Date(2026, 11, 25, 3, 0, 30, 0, time.UTC)
	// both check at the same time: one backup
	var wg sync.WaitGroup
	results := make([]bool, 2)
	for i, s := range []*Server{e.srv, srv2} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _ = s.RunBackupSchedule(ctx)
		}()
	}
	wg.Wait()
	runs, _ := e.st.ListBackupRuns(ctx, 10)
	if len(runs) != 1 || results[0] == results[1] {
		t.Fatalf("%v %+v", results, runs)
	}
	// the other server holds the lock: no backup here
	if ok, _ := e.st.AcquireBackupLock(ctx, srv2.backupInstance(), time.Hour); !ok {
		t.Fatal("lock")
	}
	*now = time.Date(2026, 11, 26, 3, 0, 30, 0, time.UTC)
	if ran, _ := e.srv.RunBackupSchedule(ctx); ran {
		t.Fatal("ran while another server holds the lock")
	}
	// a stale run of a crashed server is marked interrupted at the next backup
	_ = e.st.ReleaseBackupLock(ctx, srv2.backupInstance())
	stale := &store.BackupRun{Kind: "schedule", Instance: "crashed"}
	e.st.CreateBackupRun(ctx, stale)
	if ran, _ := e.srv.RunBackupSchedule(ctx); !ran {
		t.Fatal("not run")
	}
	if got, _ := e.st.GetBackupRun(ctx, stale.ID); got.Status != store.BackupFailed {
		t.Fatalf("%+v", got)
	}
}

func TestBackupScheduleInvalid(t *testing.T) {
	e, _, _ := backupEnv(t)
	c := *e.srv.conf()
	c.Backup.Schedule = "sometimes"
	e.srv.sys.live.Store(&c)
	if _, err := e.srv.RunBackupSchedule(context.Background()); err == nil {
		t.Fatal("invalid schedule accepted")
	}
	a := e.login("admin@example.com")
	if set, _, _ := listBackups(t, a); set.ScheduleError == "" {
		t.Fatalf("%+v", set)
	}
}

func TestBackupUploadS3(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	status := http.StatusOK
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		io.Copy(io.Discard, r.Body)
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		paths = append(paths, r.URL.Path)
		w.WriteHeader(status)
	}))
	defer s3.Close()
	e, _, now := backupEnv(t, func(c *config.Config) {
		c.Backup.S3 = config.BackupS3{Enabled: true, Endpoint: s3.URL, Region: "us-east-1", Bucket: "qtf", Prefix: "nightly", AccessKeyID: "<id>", SecretAccessKey: "<secret>"}
	})
	ctx := context.Background()
	if ran, _ := e.srv.RunBackupSchedule(ctx); !ran {
		t.Fatal("not run")
	}
	runs, _ := e.st.ListBackupRuns(ctx, 10)
	if len(runs) != 1 || runs[0].Status != store.BackupSucceeded || runs[0].RemoteKey != "nightly/"+runs[0].FileName || len(paths) != 1 || paths[0] != "/qtf/nightly/"+runs[0].FileName {
		t.Fatalf("%+v %v", runs, paths)
	}
	a := e.login("admin@example.com")
	if set, _, _ := listBackups(t, a); !set.S3Enabled || set.S3Target != s3.URL+"/qtf/nightly/" {
		t.Fatalf("%+v", set)
	}
	// a failed upload fails the run (the file is kept)
	status = http.StatusInternalServerError
	*now = now.Add(2 * time.Hour)
	e.srv.RunBackupSchedule(ctx)
	runs, _ = e.st.ListBackupRuns(ctx, 10)
	if runs[0].Status != store.BackupFailed || !strings.Contains(runs[0].Error, "upload") || runs[0].FileName == "" || runs[0].SHA256 == "" {
		t.Fatalf("%+v", runs[0])
	}
}

// restoreEncrypted posts a multipart restore with a passphrase.
func restoreEncrypted(t *testing.T, c *client, archive []byte, pass string, want int) map[string]any {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("confirm", "RESTORE")
	if pass != "" {
		mw.WriteField("passphrase", pass)
	}
	fw, _ := mw.CreateFormFile("backup", "backup.zip.enc")
	fw.Write(archive)
	mw.Close()
	req, _ := http.NewRequest("POST", c.e.hs.URL+"/api/v1/admin/restore", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", c.csrf)
	res, err := c.hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		t.Fatalf("restore: %d, want %d: %s", res.StatusCode, want, data)
	}
	out := map[string]any{}
	json.Unmarshal(data, &out)
	return out
}

func TestRestoreEncryptedBackup(t *testing.T) {
	e, dir, _ := backupEnv(t, func(c *config.Config) { c.Backup.Passphrase = testPassphrase })
	ctx := context.Background()
	a := e.login("admin@example.com")
	a.project("KEEP")
	if ran, _ := e.srv.RunBackupSchedule(ctx); !ran {
		t.Fatal("not run")
	}
	runs, _ := e.st.ListBackupRuns(ctx, 1)
	archive, _ := os.ReadFile(filepath.Join(dir, runs[0].FileName))
	a.project("GONE")
	restoreEncrypted(t, a, archive, "a wrong passphrase", 400)
	// the passphrase of the scheduled backups is used when none is given
	out := restoreEncrypted(t, a, archive, "", 200)
	if out["restored"].(map[string]any)["projects"].(float64) != 1 {
		t.Fatal(out)
	}
	a = e.login("admin@example.com")
	restoreEncrypted(t, a, archive, testPassphrase, 200)
	ps, _ := e.st.ListProjects(ctx, e.tenant.ID)
	if len(ps) != 1 || ps[0].Key != "KEEP" {
		t.Fatal(ps)
	}
}

func TestBackupSettingsAreSecret(t *testing.T) {
	e, _, _ := backupEnv(t)
	a := e.login("admin@example.com")
	putSettings(a, map[string]any{"backup.passphrase": "short"}, 400)
	putSettings(a, map[string]any{"backup.schedule": "hourly"}, 400)
	putSettings(a, map[string]any{"backup.s3.bucket": "Bad_Bucket"}, 400)
	r := putSettings(a, map[string]any{"backup.passphrase": testPassphrase, "backup.schedule": "weekly sun 04:30", "backup.keep_days": 30}, 200)
	if s := r.get(t, "backup.passphrase"); s.Value != nil || s.HasSecret == nil || !*s.HasSecret {
		t.Fatalf("%+v", s)
	}
	set, _, _ := listBackups(t, a)
	if !set.Encrypted || set.Schedule != "weekly sun 04:30" || set.KeepDays != 30 {
		t.Fatalf("%+v", set)
	}
	// directory applies after a restart
	r = putSettings(a, map[string]any{"backup.directory": "/var/backups/other"}, 200)
	if s := r.get(t, "backup.directory"); !s.PendingRestart {
		t.Fatalf("%+v", s)
	}
	putSettings(a, map[string]any{"backup.directory": "relative/dir"}, 400)
}
