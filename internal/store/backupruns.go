package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// BackupRun is one backup the server took (table backup_runs).
type BackupRun struct {
	ID string `json:"id"`
	// Kind is "schedule" or "manual" (Run backup now).
	Kind string `json:"kind"`
	// Status is "running", "succeeded" or "failed".
	Status        string     `json:"status"`
	StartedBy     string     `json:"started_by"`
	Instance      string     `json:"-"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	FileName      string     `json:"file_name"`
	Size          int64      `json:"size"`
	SHA256        string     `json:"sha256"`
	Encrypted     bool       `json:"encrypted"`
	RemoteKey     string     `json:"remote_key"`
	Rows          int64      `json:"rows"`
	SchemaVersion int        `json:"schema_version"`
	Error         string     `json:"error"`
	PrunedAt      *time.Time `json:"pruned_at,omitempty"`
	VerifiedAt    *time.Time `json:"verified_at,omitempty"`
	VerifyError   string     `json:"verify_error"`
}

// Backup run states.
const (
	BackupRunning   = "running"
	BackupSucceeded = "succeeded"
	BackupFailed    = "failed"
)

const backupRunCols = `id, kind, status, started_by, instance, started_at, finished_at, file_name, size_bytes, sha256, encrypted, remote_key,
	row_count, schema_version, error, pruned_at, verified_at, verify_error`

func scanBackupRun(r interface{ Scan(...any) error }) (*BackupRun, error) {
	var b BackupRun
	var fin, pruned, verified sql.NullTime
	if err := r.Scan(&b.ID, &b.Kind, &b.Status, &b.StartedBy, &b.Instance, &b.StartedAt, &fin, &b.FileName, &b.Size, &b.SHA256, &b.Encrypted,
		&b.RemoteKey, &b.Rows, &b.SchemaVersion, &b.Error, &pruned, &verified, &b.VerifyError); err != nil {
		return nil, err
	}
	b.StartedAt = b.StartedAt.UTC()
	b.FinishedAt, b.PrunedAt, b.VerifiedAt = timePtr(fin), timePtr(pruned), timePtr(verified)
	return &b, nil
}

func clipRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// CreateBackupRun records a backup that starts now (status running).
func (s *Store) CreateBackupRun(ctx context.Context, b *BackupRun) error {
	b.ID = newID()
	b.Status = BackupRunning
	b.StartedAt = s.now().UTC()
	_, err := s.c(ctx).exec(ctx, `INSERT INTO backup_runs (id, kind, status, started_by, instance, started_at) VALUES (?, ?, ?, ?, ?, ?)`,
		b.ID, b.Kind, b.Status, b.StartedBy, b.Instance, b.StartedAt)
	return err
}

// FinishBackupRun records the outcome of a backup (finished now).
func (s *Store) FinishBackupRun(ctx context.Context, b *BackupRun) error {
	now := s.now().UTC()
	b.FinishedAt = &now
	b.Error = clipRunes(b.Error, 2000)
	_, err := s.c(ctx).exec(ctx, `UPDATE backup_runs SET status = ?, finished_at = ?, file_name = ?, size_bytes = ?, sha256 = ?, encrypted = ?, remote_key = ?,
		row_count = ?, schema_version = ?, error = ? WHERE id = ?`,
		b.Status, now, b.FileName, b.Size, b.SHA256, b.Encrypted, b.RemoteKey, b.Rows, b.SchemaVersion, b.Error, b.ID)
	return err
}

// GetBackupRun returns one backup run.
func (s *Store) GetBackupRun(ctx context.Context, id string) (*BackupRun, error) {
	b, err := scanBackupRun(s.c(ctx).row(ctx, `SELECT `+backupRunCols+` FROM backup_runs WHERE id = ?`, id))
	return b, mapErr(err, "backup run")
}

// ListBackupRuns returns the latest backup runs, newest first.
func (s *Store) ListBackupRuns(ctx context.Context, limit int) ([]BackupRun, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.c(ctx).query(ctx, `SELECT `+backupRunCols+` FROM backup_runs ORDER BY started_at DESC, id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BackupRun{}
	for rows.Next() {
		b, err := scanBackupRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// MarkBackupFilesPruned records that the retention rules deleted files.
func (s *Store) MarkBackupFilesPruned(ctx context.Context, names []string) error {
	if len(names) == 0 {
		return nil
	}
	args := []any{s.now().UTC()}
	for _, n := range names {
		args = append(args, n)
	}
	_, err := s.c(ctx).exec(ctx, `UPDATE backup_runs SET pruned_at = ? WHERE pruned_at IS NULL AND file_name IN (?`+strings.Repeat(", ?", len(names)-1)+`)`, args...)
	return err
}

// SetBackupVerified records the outcome of a verification ("" = intact).
func (s *Store) SetBackupVerified(ctx context.Context, id, errText string) error {
	_, err := s.c(ctx).exec(ctx, `UPDATE backup_runs SET verified_at = ?, verify_error = ? WHERE id = ?`, s.now().UTC(), clipRunes(errText, 2000), id)
	return err
}

// FailStaleBackupRuns marks the running backups of instances other than
// instance as failed (their server stopped while backing up); it is called
// while holding the backup lock, so no other backup runs.
func (s *Store) FailStaleBackupRuns(ctx context.Context, instance string) (int, error) {
	res, err := s.c(ctx).exec(ctx, `UPDATE backup_runs SET status = ?, finished_at = ?, error = ? WHERE status = ? AND instance <> ?`,
		BackupFailed, s.now().UTC(), "interrupted: the server stopped during the backup", BackupRunning, instance)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// AcquireBackupLock takes the backup lock for holder for lease; false
// when another holder has a lease that has not expired.
func (s *Store) AcquireBackupLock(ctx context.Context, holder string, lease time.Duration) (bool, error) {
	now := s.now().UTC()
	res, err := s.c(ctx).exec(ctx, `UPDATE backup_locks SET holder = ?, lease_until = ? WHERE name = 'backup' AND (lease_until IS NULL OR lease_until < ? OR holder = ?)`,
		holder, now.Add(lease), now, holder)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// RenewBackupLock extends the lease of holder to lease from now; false
// when it lost the lock.
func (s *Store) RenewBackupLock(ctx context.Context, holder string, lease time.Duration) (bool, error) {
	res, err := s.c(ctx).exec(ctx, `UPDATE backup_locks SET lease_until = ? WHERE name = 'backup' AND holder = ?`, s.now().UTC().Add(lease), holder)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ReleaseBackupLock gives up the lock of holder.
func (s *Store) ReleaseBackupLock(ctx context.Context, holder string) error {
	_, err := s.c(ctx).exec(ctx, `UPDATE backup_locks SET holder = '', lease_until = NULL WHERE name = 'backup' AND holder = ?`, holder)
	return err
}
