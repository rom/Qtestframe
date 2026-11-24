package api

// Scheduled backups (Administration → Backup; system settings section
// backup): the server writes the logical backup archive of store.Backup
// into backup.directory on the schedule of backup.schedule, optionally
// encrypted with backup.passphrase and uploaded to S3-compatible storage,
// deletes old files by the retention rules and records every run in
// backup_runs. With several servers on one database the lock row in
// backup_locks makes one of them take each backup. Failures are audited
// and e-mailed to the super administrators (when e-mail is set up).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/rom/qtestframe/internal/backup"
	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/mail"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// backupJob is the job_runs entry of the scheduled backups (the latest
// slot of the schedule that was handled).
const backupJob = "backup.schedule"

// backupLease is how long the backup lock is held without renewal.
const backupLease = 5 * time.Minute

// backupState is the state of the scheduled backups of a server.
type backupState struct {
	mu sync.Mutex
	// ctx is the server's lifetime (StartBackups); backups started by a
	// request run in it.
	ctx context.Context
	wg  sync.WaitGroup
	// lease overrides backupLease (tests).
	lease time.Duration
}

func (s *Server) backupLease() time.Duration {
	if s.bk.lease > 0 {
		return s.bk.lease
	}
	return backupLease
}

func (s *Server) backupInstance() string { return s.jobs.instance }

// StartBackups checks the backup schedule every minute until ctx ends.
func (s *Server) StartBackups(ctx context.Context) {
	s.bk.mu.Lock()
	s.bk.ctx = ctx
	s.bk.mu.Unlock()
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			if _, err := s.RunBackupSchedule(ctx); err != nil && ctx.Err() == nil {
				s.Log.Warn("backup schedule", "err", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

// WaitBackups waits for backups started in the background (tests, shutdown).
func (s *Server) WaitBackups() { s.bk.wg.Wait() }

// RunBackupSchedule takes a backup when the schedule says one is due and no
// other server holds the backup lock; it reports whether it ran one (the
// backup itself is recorded in backup_runs, failures included).
func (s *Server) RunBackupSchedule(ctx context.Context) (bool, error) {
	c := s.conf().Backup
	if !c.Enabled || c.Directory == "" {
		return false, nil
	}
	sch, err := backup.ParseSchedule(c.Schedule)
	if err != nil {
		return false, err
	}
	now := s.now()
	last, err := s.Store.JobLastRun(ctx, backupJob)
	if err != nil {
		return false, err
	}
	due, record := sch.Due(now, last)
	if !due {
		if last == nil && !record.IsZero() {
			return false, s.Store.SetJobLastRun(ctx, backupJob, record)
		}
		return false, nil
	}
	ok, err := s.Store.AcquireBackupLock(ctx, s.backupInstance(), s.backupLease())
	if err != nil || !ok {
		return false, err
	}
	// another server may have handled the slot before this one got the lock
	if last, err = s.Store.JobLastRun(ctx, backupJob); err != nil {
		_ = s.Store.ReleaseBackupLock(ctx, s.backupInstance())
		return false, err
	}
	if due, record = sch.Due(now, last); !due {
		return false, s.Store.ReleaseBackupLock(ctx, s.backupInstance())
	}
	// recorded first: a failing backup is not retried every minute but at
	// the next slot (or with Run backup now)
	if err := s.Store.SetJobLastRun(ctx, backupJob, record); err != nil {
		_ = s.Store.ReleaseBackupLock(ctx, s.backupInstance())
		return false, err
	}
	run, err := s.startBackupRun(ctx, "schedule", "system")
	if err != nil {
		_ = s.Store.ReleaseBackupLock(ctx, s.backupInstance())
		return false, err
	}
	s.performBackup(ctx, run, c)
	return true, nil
}

func (s *Server) startBackupRun(ctx context.Context, kind, by string) (*store.BackupRun, error) {
	if _, err := s.Store.FailStaleBackupRuns(ctx, s.backupInstance()); err != nil {
		return nil, err
	}
	run := &store.BackupRun{Kind: kind, StartedBy: by, Instance: s.backupInstance()}
	if err := s.Store.CreateBackupRun(ctx, run); err != nil {
		return nil, err
	}
	return run, nil
}

// performBackup takes the backup of run while holding the backup lock
// (renewed while it runs, released at the end) and records the outcome.
func (s *Server) performBackup(ctx context.Context, run *store.BackupRun, c config.Backup) {
	holder := s.backupInstance()
	stop := make(chan struct{})
	var renew sync.WaitGroup
	renew.Add(1)
	go func() {
		defer renew.Done()
		t := time.NewTicker(s.backupLease() / 3)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if ok, err := s.Store.RenewBackupLock(context.WithoutCancel(ctx), holder, s.backupLease()); err != nil || !ok {
					s.Log.Warn("backup lock not renewed", "ok", ok, "err", err)
				}
			}
		}
	}()
	err := s.writeBackup(ctx, run, c)
	close(stop)
	renew.Wait()
	// the outcome is recorded even when the server is shutting down
	bg := context.WithoutCancel(ctx)
	run.Status = store.BackupSucceeded
	if err != nil {
		run.Status, run.Error = store.BackupFailed, err.Error()
	}
	if ferr := s.Store.FinishBackupRun(bg, run); ferr != nil {
		s.Log.Error("backup run not recorded", "err", ferr)
	}
	if rerr := s.Store.ReleaseBackupLock(bg, holder); rerr != nil {
		s.Log.Warn("backup lock not released", "err", rerr)
	}
	who := run.StartedBy
	if err != nil {
		s.Log.Error("backup failed", "kind", run.Kind, "file", run.FileName, "err", err)
		s.systemAudit(bg, who, "admin.backup.failed", run.ID, fmt.Sprintf("%s backup: %s", run.Kind, run.Error))
		if ctx.Err() == nil { // not for a backup cut short by a shutdown
			s.notifyBackupFailure(bg, run)
		}
		return
	}
	details := fmt.Sprintf("%s backup %s: %d bytes, %d rows, schema %d, sha256 %s", run.Kind, run.FileName, run.Size, run.Rows, run.SchemaVersion, run.SHA256)
	if run.RemoteKey != "" {
		details += ", uploaded as " + run.RemoteKey
	}
	s.Log.Info("backup written", "kind", run.Kind, "file", run.FileName, "bytes", run.Size)
	s.systemAudit(bg, who, "admin.backup.completed", run.ID, details)
}

// systemAudit writes a system audit entry (no organisation) as who.
func (s *Server) systemAudit(ctx context.Context, who, action, id, details string) {
	if who == "" {
		who = "system"
	}
	if err := s.Store.Audit(ctx, model.AuditEntry{UserEmail: who, Action: action, EntityType: "backup", EntityID: id, Details: details}); err != nil {
		s.Log.Warn("audit write failed", "err", err)
	}
}

// countWriter counts the bytes written through it.
type countWriter struct{ n int64 }

func (c *countWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}

// writeBackup writes the archive into the backup directory, uploads it and
// applies the retention rules; run receives the file and its checksum.
func (s *Server) writeBackup(ctx context.Context, run *store.BackupRun, c config.Backup) error {
	if c.Directory == "" {
		return errors.New("backup.directory is not set")
	}
	if err := os.MkdirAll(c.Directory, 0o700); err != nil {
		return err
	}
	enc := c.Passphrase != ""
	tmp, err := os.CreateTemp(c.Directory, ".qtf-backup-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after the rename
	h := sha256.New()
	cnt := &countWriter{}
	var out io.Writer = io.MultiWriter(tmp, h, cnt)
	var ew io.WriteCloser
	if enc {
		if ew, err = backup.Encrypt(out, c.Passphrase); err != nil {
			tmp.Close()
			return err
		}
		out = ew
	}
	m, err := s.Store.Backup(ctx, out, Version)
	if err == nil && ew != nil {
		err = ew.Close()
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	name := backup.FreeName(c.Directory, s.now(), enc)
	path := filepath.Join(c.Directory, name)
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	run.FileName, run.Size, run.SHA256, run.Encrypted = name, cnt.n, hex.EncodeToString(h.Sum(nil)), enc
	run.Rows, run.SchemaVersion = int64(m.Rows()), m.SchemaVersion
	var upErr error
	if c.S3.Enabled {
		s3 := backupS3(c)
		key := s3.Key(name)
		if upErr = s3.Upload(ctx, s.HTTPClient, key, path, run.SHA256, s.now()); upErr == nil {
			run.RemoteKey = key
		}
	}
	deleted, perr := backup.Prune(c.Directory, c.KeepLast, c.KeepDays, s.now())
	if len(deleted) > 0 {
		s.Log.Info("old backups deleted", "files", deleted)
		if err := s.Store.MarkBackupFilesPruned(context.WithoutCancel(ctx), deleted); err != nil {
			s.Log.Warn("pruned backups not recorded", "err", err)
		}
	}
	if upErr != nil {
		return upErr
	}
	if perr != nil {
		return fmt.Errorf("retention: %w", perr)
	}
	return nil
}

func backupS3(c config.Backup) backup.S3 {
	return backup.S3{Endpoint: c.S3.Endpoint, Region: c.S3.Region, Bucket: c.S3.Bucket, Prefix: c.S3.Prefix,
		AccessKeyID: c.S3.AccessKeyID, SecretAccessKey: c.S3.SecretAccessKey}
}

// notifyBackupFailure e-mails the super administrators about a failed backup.
func (s *Server) notifyBackupFailure(ctx context.Context, run *store.BackupRun) {
	if !s.emailOn() {
		return
	}
	users, err := s.Store.ListUsers(ctx)
	if err != nil {
		s.Log.Warn("backup failure not e-mailed", "err", err)
		return
	}
	for i := range users {
		u := &users[i]
		if !u.SuperAdmin || u.Disabled || u.Email == "" {
			continue
		}
		lang := u.Preferences.Language
		c := mailContent{Lang: mailLang(lang), Title: tr(lang, "backup.failed.subject"),
			Intro:  []string{tr(lang, "backup.failed.intro", run.StartedAt.Format("2006-01-02 15:04 UTC")), run.Error},
			Action: &mailLink{Text: tr(lang, "backup.failed.action"), URL: s.baseURL() + "/admin/backup"}}
		s.sendMail(ctx, s.message([]mail.Address{userAddress(u)}, tr(lang, "backup.failed.subject"), c), nil)
	}
}

// ---------------------------------------------------------------- HTTP

// backupSettingsView is the schedule as the Backup page shows it (no
// secrets: only whether they are set).
type backupSettingsView struct {
	Enabled       bool       `json:"enabled"`
	Schedule      string     `json:"schedule"`
	ScheduleError string     `json:"schedule_error,omitempty"`
	Directory     string     `json:"directory"`
	KeepLast      int        `json:"keep_last"`
	KeepDays      int        `json:"keep_days"`
	Encrypted     bool       `json:"encrypted"`
	S3Enabled     bool       `json:"s3_enabled"`
	S3Target      string     `json:"s3_target,omitempty"`
	NextRunAt     *time.Time `json:"next_run_at,omitempty"`
}

func (s *Server) backupSettings(ctx context.Context) backupSettingsView {
	c := s.conf().Backup
	v := backupSettingsView{Enabled: c.Enabled, Schedule: c.Schedule, Directory: c.Directory, KeepLast: c.KeepLast, KeepDays: c.KeepDays,
		Encrypted: c.Passphrase != "", S3Enabled: c.S3.Enabled}
	if c.S3.Enabled && c.S3.Bucket != "" {
		v.S3Target = c.S3.Endpoint + "/" + c.S3.Bucket + "/" + backupS3(c).Key("")
	}
	sch, err := backup.ParseSchedule(c.Schedule)
	if err != nil {
		v.ScheduleError = err.Error()
		return v
	}
	v.Schedule = sch.String()
	if c.Enabled && c.Directory != "" {
		last, _ := s.Store.JobLastRun(ctx, backupJob)
		next := sch.Next(s.now(), last)
		v.NextRunAt = &next
	}
	return v
}

func (s *Server) routesBackups(h routeFunc) {
	h("GET /api/v1/admin/backups", s.handleListBackups)
	h("POST /api/v1/admin/backups/run", s.handleRunBackup)
	h("GET /api/v1/admin/backups/{id}/download", s.handleDownloadBackup)
	h("POST /api/v1/admin/backups/{id}/verify", s.handleVerifyBackup)
}

// handleListBackups: the schedule and the latest runs.
func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	runs, err := s.Store.ListBackupRuns(r.Context(), limit)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	running := false
	for _, b := range runs {
		if b.Status == store.BackupRunning {
			running = true
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": s.backupSettings(r.Context()), "running": running, "runs": runs})
}

// handleRunBackup starts a backup now (in the background) with the
// settings of the scheduled backups.
func (s *Server) handleRunBackup(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	c := s.conf().Backup
	if c.Directory == "" {
		s.fail(w, r, badRequest("set backup.directory in the system settings (section Scheduled backups) first"))
		return
	}
	ok, err := s.Store.AcquireBackupLock(r.Context(), s.backupInstance(), s.backupLease())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !ok {
		s.fail(w, r, &apiErr{http.StatusConflict, "conflict", "a backup is running; try again when it has finished"})
		return
	}
	run, err := s.startBackupRun(r.Context(), "manual", p.User.Email)
	if err != nil {
		_ = s.Store.ReleaseBackupLock(r.Context(), s.backupInstance())
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "admin.backup.run", "backup", run.ID, "backup started")
	s.bk.mu.Lock()
	ctx := s.bk.ctx
	s.bk.mu.Unlock()
	if ctx == nil {
		ctx = context.WithoutCancel(r.Context())
	}
	s.bk.wg.Add(1)
	go func() {
		defer s.bk.wg.Done()
		s.performBackup(ctx, run, c)
	}()
	writeJSON(w, http.StatusAccepted, run)
}

// backupFile returns the run and the path of its file in the backup
// directory (the name is checked: no other file is ever served).
func (s *Server) backupFile(ctx context.Context, id string) (*store.BackupRun, string, error) {
	run, err := s.Store.GetBackupRun(ctx, id)
	if err != nil {
		return nil, "", err
	}
	dir := s.conf().Backup.Directory
	if run.FileName == "" || !backup.IsBackupFile(run.FileName) || dir == "" {
		return run, "", &apiErr{http.StatusNotFound, "not_found", "this backup has no file"}
	}
	if run.PrunedAt != nil {
		return run, "", &apiErr{http.StatusGone, "gone", "the file of this backup was deleted by the retention rules"}
	}
	path := filepath.Join(dir, run.FileName)
	if _, err := os.Stat(path); err != nil {
		return run, "", &apiErr{http.StatusNotFound, "not_found", "the file of this backup is not in the backup directory"}
	}
	return run, path, nil
}

// handleDownloadBackup streams the file of a backup (audited).
func (s *Server) handleDownloadBackup(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	run, path, err := s.backupFile(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// large files outlast server.write_timeout (super administrators only)
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	s.audit(r.Context(), p, "admin.backup.download", "backup", run.ID, run.FileName)
	ctype := "application/zip"
	if run.Encrypted {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", `attachment; filename="`+run.FileName+`"`)
	w.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

// backupCheck is the outcome of a verification.
type backupCheck struct {
	OK        bool                  `json:"ok"`
	Error     string                `json:"error,omitempty"`
	SHA256    string                `json:"sha256"`
	SHA256OK  bool                  `json:"sha256_ok"`
	Encrypted bool                  `json:"encrypted"`
	Manifest  *store.BackupManifest `json:"manifest,omitempty"`
}

// verifyBackupFile checks a backup file: its SHA-256 against want ("" =
// not recorded), the decryption and the archive (store.VerifyBackup).
func verifyBackupFile(path, want, passphrase string) backupCheck {
	var out backupCheck
	sum, _, err := backup.FileSHA256(path)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.SHA256, out.SHA256OK = sum, want == "" || sum == want
	if !out.SHA256OK {
		out.Error = "the SHA-256 of the file differs from the one recorded when it was written: the file was changed or damaged"
		return out
	}
	a, err := backup.Open(path, passphrase, filepath.Dir(path))
	if err != nil {
		out.Encrypted = errors.Is(err, backup.ErrNoPassphrase) || errors.Is(err, backup.ErrPassphrase)
		out.Error = err.Error()
		return out
	}
	defer a.Close()
	out.Encrypted = a.Encrypted
	m, err := store.VerifyBackup(a, a.Size)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.OK, out.Manifest = true, m
	return out
}

// handleVerifyBackup opens the file of a backup and checks its integrity
// and manifest (audited; the outcome is recorded with the run).
func (s *Server) handleVerifyBackup(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireSuper(w, p) {
		return
	}
	run, path, err := s.backupFile(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	res := verifyBackupFile(path, run.SHA256, s.conf().Backup.Passphrase)
	if err := s.Store.SetBackupVerified(r.Context(), run.ID, res.Error); err != nil {
		s.fail(w, r, err)
		return
	}
	details := run.FileName + ": intact"
	if !res.OK {
		details = run.FileName + ": " + res.Error
	}
	s.audit(r.Context(), p, "admin.backup.verify", "backup", run.ID, details)
	writeJSON(w, http.StatusOK, res)
}
