package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/model"
)

// Two-way sync of requirements with Jira and Azure DevOps (migration
// 0060): the configuration per project and source, the conflicts that wait
// for a decision and the sync log. The state per linked item is kept with
// its external reference (externalrefs.go).

func adReqSync(id string) string { return "req_sync_configs.webhook_secret|" + id }

// ReqSyncRow is a stored sync configuration (WebhookSecret in plaintext:
// for the server's own use).
type ReqSyncRow struct {
	ID              string
	TenantID        string
	ProjectID       string
	System          string
	Enabled         bool
	Settings        model.ReqSyncSettings
	WebhookSecret   string
	RunAs           string
	IntervalMinutes int
	NextRunAt       *time.Time
	LeaseUntil      *time.Time
	Failures        int
	LastRunAt       *time.Time
	LastStatus      string
	LastError       string
	Version         int
	UpdatedBy       string
	UpdatedAt       time.Time
}

const reqSyncCols = `id, tenant_id, project_id, system, enabled, settings, webhook_secret, run_as, interval_minutes, next_run_at, lease_until,
	failures, last_run_at, last_status, last_error, version, updated_by, updated_at`

func (s *Store) scanReqSync(ctx context.Context, r interface{ Scan(...any) error }) (*ReqSyncRow, error) {
	var x ReqSyncRow
	var settings string
	var next, lease, last sql.NullTime
	if err := r.Scan(&x.ID, &x.TenantID, &x.ProjectID, &x.System, &x.Enabled, &settings, &x.WebhookSecret, &x.RunAs, &x.IntervalMinutes,
		&next, &lease, &x.Failures, &last, &x.LastStatus, &x.LastError, &x.Version, &x.UpdatedBy, &x.UpdatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(settings), &x.Settings)
	x.NextRunAt, x.LeaseUntil, x.LastRunAt = timePtr(next), timePtr(lease), timePtr(last)
	x.UpdatedAt = x.UpdatedAt.UTC()
	var err error
	if x.WebhookSecret, err = s.unseal(ctx, x.WebhookSecret, adReqSync(x.ID)); err != nil {
		return nil, err
	}
	return &x, nil
}

// ReqSync returns the sync configuration of a project and system
// (ErrNotFound when there is none).
func (s *Store) ReqSync(ctx context.Context, tenantID, projectID, system string) (*ReqSyncRow, error) {
	x, err := s.scanReqSync(ctx, s.c(ctx).row(ctx, `SELECT `+reqSyncCols+` FROM req_sync_configs WHERE tenant_id = ? AND project_id = ? AND system = ?`, tenantID, projectID, system))
	return x, mapErr(err, "sync configuration")
}

// ReqSyncForHook returns the sync configuration a webhook of a project is
// for, whatever its tenant (the secret is the credential).
func (s *Store) ReqSyncForHook(ctx context.Context, projectID, system string) (*ReqSyncRow, error) {
	x, err := s.scanReqSync(ctx, s.c(ctx).row(ctx, `SELECT `+reqSyncCols+` FROM req_sync_configs WHERE project_id = ? AND system = ?`, projectID, system))
	return x, mapErr(err, "sync configuration")
}

func (s *Store) listReqSync(ctx context.Context, where string, args ...any) ([]ReqSyncRow, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+reqSyncCols+` FROM req_sync_configs WHERE `+where+` ORDER BY project_id, system`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReqSyncRow{}
	for rows.Next() {
		x, err := s.scanReqSync(ctx, rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

// ListReqSync returns a project's sync configurations.
func (s *Store) ListReqSync(ctx context.Context, tenantID, projectID string) ([]ReqSyncRow, error) {
	return s.listReqSync(ctx, `tenant_id = ? AND project_id = ?`, tenantID, projectID)
}

// DueReqSyncs returns the enabled scheduled syncs whose next run has come
// and that no server is running.
func (s *Store) DueReqSyncs(ctx context.Context, now time.Time) ([]ReqSyncRow, error) {
	return s.listReqSync(ctx, `enabled = ? AND interval_minutes > 0 AND next_run_at IS NOT NULL AND next_run_at <= ? AND (lease_until IS NULL OR lease_until < ?)`, true, now, now)
}

// Limits of a sync configuration.
const (
	MaxSyncInterval = 7 * 24 * 60
	MinSyncInterval = 5
)

// ValidateReqSync checks and normalises the settings of a configuration.
func ValidateReqSync(x *ReqSyncRow) error {
	st := &x.Settings
	if x.IntervalMinutes != 0 && (x.IntervalMinutes < MinSyncInterval || x.IntervalMinutes > MaxSyncInterval) {
		return Invalidf("interval_minutes must be 0 (no schedule) or between %d and %d", MinSyncInterval, MaxSyncInterval)
	}
	if st.ConflictPolicy == "" {
		st.ConflictPolicy = model.SyncManual
	}
	if !slices.Contains(model.SyncConflictPolicies, st.ConflictPolicy) {
		return Invalidf("conflict_policy must be one of %s", strings.Join(model.SyncConflictPolicies, ", "))
	}
	if st.Impact == "" {
		st.Impact = model.ImpactApplySuspect
	}
	if !slices.Contains(model.ImpactChoices, st.Impact) {
		return Invalidf("impact must be one of %s", strings.Join(model.ImpactChoices, ", "))
	}
	if st.MaxWritesPerMinute == 0 {
		st.MaxWritesPerMinute = 60
	}
	if st.MaxWritesPerMinute < 1 || st.MaxWritesPerMinute > 600 {
		return Invalidf("max_writes_per_minute must be between 1 and 600")
	}
	st.Query = strings.TrimSpace(st.Query)
	if utf8.RuneCountInString(st.Query) > 20000 {
		return Invalidf("query may have at most 20000 characters")
	}
	p := &st.Push
	p.FolderID, p.Tag, p.Query = strings.TrimSpace(p.FolderID), strings.TrimSpace(p.Tag), strings.TrimSpace(p.Query)
	p.ItemType, p.ProjectKey = strings.TrimSpace(p.ItemType), strings.ToUpper(strings.TrimSpace(p.ProjectKey))
	for name, v := range map[string]string{"push.tag": p.Tag, "push.query": p.Query, "push.item_type": p.ItemType, "push.project_key": p.ProjectKey, "folder_id": st.FolderID} {
		if utf8.RuneCountInString(v) > 2000 {
			return Invalidf("%s may have at most 2000 characters", name)
		}
	}
	if p.Create && p.FolderID == "" && p.Tag == "" && p.Query == "" {
		return Invalidf("choose the requirements to create tracker items for: a folder, a tag or a query")
	}
	if len(st.Fields) > 50 {
		return Invalidf("at most 50 mapped fields")
	}
	seen := map[string]bool{}
	for i := range st.Fields {
		f := &st.Fields[i]
		f.Field, f.Remote = strings.TrimSpace(f.Field), strings.TrimSpace(f.Remote)
		if f.Direction == "" {
			f.Direction = model.SyncBoth
		}
		switch {
		case !slices.Contains(model.SyncFields, f.Field):
			return Invalidf("fields[%d].field must be one of %s", i, strings.Join(model.SyncFields, ", "))
		case seen[f.Field]:
			return Invalidf("the field %s is mapped twice", f.Field)
		case f.Remote == "" || utf8.RuneCountInString(f.Remote) > 200:
			return Invalidf("fields[%d].remote (the tracker field) is required (at most 200 characters)", i)
		case !slices.Contains(model.SyncDirections, f.Direction):
			return Invalidf("fields[%d].direction must be one of %s", i, strings.Join(model.SyncDirections, ", "))
		case len(f.Values) > 200 || len(f.PushValues) > 200:
			return Invalidf("fields[%d]: at most 200 value mappings", i)
		}
		seen[f.Field] = true
		for _, m := range []map[string]string{f.Values, f.PushValues} {
			for k, v := range m {
				if utf8.RuneCountInString(k) > 200 || utf8.RuneCountInString(v) > 200 {
					return Invalidf("fields[%d]: values may have at most 200 characters", i)
				}
			}
		}
		if f.Field == "status" {
			for k, v := range f.Values {
				if v != "" && !slices.Contains(model.RequirementStatuses, v) {
					return Invalidf("the status %q of %q is not a requirement status (%s)", v, k, strings.Join(model.RequirementStatuses, ", "))
				}
			}
		}
		if f.Field == "priority" {
			for k, v := range f.Values {
				if v != "" && !slices.Contains(model.Priorities, v) {
					return Invalidf("the priority %q of %q is not a priority (%s)", v, k, strings.Join(model.Priorities, ", "))
				}
			}
		}
	}
	return nil
}

// SaveReqSync creates or updates a configuration. secret replaces the
// webhook secret when not nil ("" removes it). A configuration read at
// version (x.Version > 0) that changed since answers ErrStale.
func (s *Store) SaveReqSync(ctx context.Context, x *ReqSyncRow, secret *string) error {
	if err := ValidateReqSync(x); err != nil {
		return err
	}
	cur, err := s.ReqSync(ctx, x.TenantID, x.ProjectID, x.System)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	now := s.now()
	x.UpdatedAt = now
	if cur == nil {
		x.ID, x.Version = newID(), 1
	} else {
		if x.Version > 0 && x.Version != cur.Version {
			return ErrStale
		}
		x.ID, x.Version = cur.ID, cur.Version+1
		x.LeaseUntil, x.Failures, x.LastRunAt, x.LastStatus, x.LastError = cur.LeaseUntil, cur.Failures, cur.LastRunAt, cur.LastStatus, cur.LastError
		if secret == nil {
			x.WebhookSecret = cur.WebhookSecret
		}
	}
	if secret != nil {
		x.WebhookSecret = strings.TrimSpace(*secret)
		if x.WebhookSecret != "" && (len(x.WebhookSecret) < 16 || len(x.WebhookSecret) > 200) {
			return Invalidf("the webhook secret must have 16 to 200 characters")
		}
	}
	sealed, err := s.seal(x.WebhookSecret, adReqSync(x.ID))
	if err != nil {
		return err
	}
	settings, _ := json.Marshal(x.Settings)
	if cur == nil {
		_, err = s.c(ctx).exec(ctx, `INSERT INTO req_sync_configs (`+reqSyncCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			x.ID, x.TenantID, x.ProjectID, x.System, x.Enabled, string(settings), sealed, x.RunAs, x.IntervalMinutes, nullTime(x.NextRunAt), nil,
			0, nil, "", "", x.Version, x.UpdatedBy, now)
		return err
	}
	_, err = s.c(ctx).exec(ctx, `UPDATE req_sync_configs SET enabled = ?, settings = ?, webhook_secret = ?, run_as = ?, interval_minutes = ?, next_run_at = ?,
		version = ?, updated_by = ?, updated_at = ? WHERE id = ? AND version = ?`,
		x.Enabled, string(settings), sealed, x.RunAs, x.IntervalMinutes, nullTime(x.NextRunAt), x.Version, x.UpdatedBy, now, x.ID, cur.Version)
	return err
}

// ClaimReqSync takes the lease of a configuration until until; false when
// another sync holds it.
func (s *Store) ClaimReqSync(ctx context.Context, id string, now, until time.Time) (bool, error) {
	res, err := s.c(ctx).exec(ctx, `UPDATE req_sync_configs SET lease_until = ? WHERE id = ? AND (lease_until IS NULL OR lease_until < ?)`, until, id, now)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// FinishReqSync releases the lease and records the outcome of a run; next
// is the next scheduled run (nil none), failures the consecutive failures.
func (s *Store) FinishReqSync(ctx context.Context, id string, at time.Time, status, errText string, failures int, next *time.Time) error {
	if r := []rune(errText); len(r) > 2000 {
		errText = string(r[:2000])
	}
	_, err := s.c(ctx).exec(ctx, `UPDATE req_sync_configs SET lease_until = NULL, last_run_at = ?, last_status = ?, last_error = ?, failures = ?, next_run_at = ? WHERE id = ?`,
		at, status, errText, failures, nullTime(next), id)
	return err
}

// ReleaseReqSync releases the lease without recording a run (a dry run).
func (s *Store) ReleaseReqSync(ctx context.Context, id string) error {
	_, err := s.c(ctx).exec(ctx, `UPDATE req_sync_configs SET lease_until = NULL WHERE id = ?`, id)
	return err
}

// ---------------------------------------------------------------- conflicts

const conflictCols = `c.id, c.project_id, c.system, c.requirement_id, COALESCE(r.key, ''), COALESCE(r.title, ''), c.external_id, c.fields,
	c.local_version, c.remote_version, c.status, c.resolution, c.resolved_by, c.resolved_at, c.created_at, c.updated_at`

func scanConflict(r interface{ Scan(...any) error }) (*model.ReqSyncConflict, error) {
	var c model.ReqSyncConflict
	var fields string
	var resolved sql.NullTime
	if err := r.Scan(&c.ID, &c.ProjectID, &c.System, &c.RequirementID, &c.RequirementKey, &c.Title, &c.ExternalID, &fields,
		&c.LocalVersion, &c.RemoteVersion, &c.Status, &c.Resolution, &c.ResolvedBy, &resolved, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(fields), &c.Fields)
	if c.Fields == nil {
		c.Fields = []model.ReqSyncFieldDiff{}
	}
	c.ResolvedAt = timePtr(resolved)
	c.CreatedAt, c.UpdatedAt = c.CreatedAt.UTC(), c.UpdatedAt.UTC()
	return &c, nil
}

// SaveReqSyncConflict records the open conflict of a requirement (a later
// sync refreshes the one already open).
func (s *Store) SaveReqSyncConflict(ctx context.Context, tenantID string, c *model.ReqSyncConflict) error {
	now := s.now()
	fields, _ := json.Marshal(c.Fields)
	return s.tx(ctx, func(cn conn) error {
		var id string
		err := cn.row(ctx, `SELECT id FROM req_sync_conflicts WHERE tenant_id = ? AND project_id = ? AND system = ? AND requirement_id = ? AND status = 'open'`,
			tenantID, c.ProjectID, c.System, c.RequirementID).Scan(&id)
		if err == nil {
			c.ID, c.Status, c.UpdatedAt = id, "open", now
			_, err = cn.exec(ctx, `UPDATE req_sync_conflicts SET external_id = ?, fields = ?, local_version = ?, remote_version = ?, updated_at = ? WHERE id = ?`,
				c.ExternalID, string(fields), c.LocalVersion, c.RemoteVersion, now, id)
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		c.ID, c.Status, c.CreatedAt, c.UpdatedAt = newID(), "open", now, now
		_, err = cn.exec(ctx, `INSERT INTO req_sync_conflicts (id, tenant_id, project_id, system, requirement_id, external_id, fields, local_version, remote_version, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'open', ?, ?)`, c.ID, tenantID, c.ProjectID, c.System, c.RequirementID, c.ExternalID, string(fields), c.LocalVersion, c.RemoteVersion, now, now)
		return err
	})
}

// ListReqSyncConflicts returns a project's conflicts (status "" all), newest first.
func (s *Store) ListReqSyncConflicts(ctx context.Context, tenantID, projectID, system, status string) ([]model.ReqSyncConflict, error) {
	q := `SELECT ` + conflictCols + ` FROM req_sync_conflicts c LEFT JOIN requirements r ON r.id = c.requirement_id WHERE c.tenant_id = ? AND c.project_id = ?`
	args := []any{tenantID, projectID}
	if system != "" {
		q += ` AND c.system = ?`
		args = append(args, system)
	}
	if status != "" {
		q += ` AND c.status = ?`
		args = append(args, status)
	}
	rows, err := s.c(ctx).query(ctx, q+` ORDER BY c.updated_at DESC, c.id LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ReqSyncConflict{}
	for rows.Next() {
		c, err := scanConflict(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// ReqSyncConflict returns a conflict of a project.
func (s *Store) ReqSyncConflict(ctx context.Context, tenantID, projectID, id string) (*model.ReqSyncConflict, error) {
	c, err := scanConflict(s.c(ctx).row(ctx, `SELECT `+conflictCols+` FROM req_sync_conflicts c LEFT JOIN requirements r ON r.id = c.requirement_id
		WHERE c.tenant_id = ? AND c.project_id = ? AND c.id = ?`, tenantID, projectID, id))
	return c, mapErr(err, "conflict")
}

// CloseReqSyncConflicts closes the open conflicts of a requirement
// (status resolved or dismissed, with the resolution and who decided; by
// "" when a sync found the sides agree again).
func (s *Store) CloseReqSyncConflicts(ctx context.Context, tenantID, projectID, system, requirementID, status, resolution, by string) (int, error) {
	now := s.now()
	res, err := s.c(ctx).exec(ctx, `UPDATE req_sync_conflicts SET status = ?, resolution = ?, resolved_by = ?, resolved_at = ?, updated_at = ?
		WHERE tenant_id = ? AND project_id = ? AND system = ? AND requirement_id = ? AND status = 'open'`, status, resolution, by, now, now, tenantID, projectID, system, requirementID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// CountReqSyncConflicts counts the open conflicts per system of a project.
func (s *Store) CountReqSyncConflicts(ctx context.Context, tenantID, projectID string) (map[string]int, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT system, COUNT(*) FROM req_sync_conflicts WHERE tenant_id = ? AND project_id = ? AND status = 'open' GROUP BY system`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var sys string
		var n int
		if err := rows.Scan(&sys, &n); err != nil {
			return nil, err
		}
		out[sys] = n
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- runs

// MaxSyncEntries bounds the entries kept with a run.
const MaxSyncEntries = 500

// keptSyncRuns: the newest runs kept per project and system.
const keptSyncRuns = 100

const syncRunCols = `id, project_id, system, trigger_kind, dry_run, status, summary, entries, error, started_by, started_at, finished_at`

func scanSyncRun(r interface{ Scan(...any) error }) (*model.ReqSyncRun, error) {
	var x model.ReqSyncRun
	var summary, entries string
	var finished sql.NullTime
	if err := r.Scan(&x.ID, &x.ProjectID, &x.System, &x.Trigger, &x.DryRun, &x.Status, &summary, &entries, &x.Error, &x.StartedBy, &x.StartedAt, &finished); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(summary), &x.Summary)
	_ = json.Unmarshal([]byte(entries), &x.Entries)
	if x.Entries == nil {
		x.Entries = []model.ReqSyncEntry{}
	}
	x.StartedAt, x.FinishedAt = x.StartedAt.UTC(), timePtr(finished)
	return &x, nil
}

// SaveReqSyncRun stores a finished run and drops the oldest runs beyond
// the kept number.
func (s *Store) SaveReqSyncRun(ctx context.Context, tenantID string, x *model.ReqSyncRun) error {
	if x.ID == "" {
		x.ID = newID()
	}
	if x.StartedAt.IsZero() {
		x.StartedAt = s.now()
	}
	entries := x.Entries
	if len(entries) > MaxSyncEntries {
		entries = entries[:MaxSyncEntries]
	}
	if entries == nil {
		entries = []model.ReqSyncEntry{}
	}
	summary, _ := json.Marshal(x.Summary)
	ents, _ := json.Marshal(entries)
	if r := []rune(x.Error); len(r) > 2000 {
		x.Error = string(r[:2000])
	}
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `INSERT INTO req_sync_runs (`+syncRunCols+`, tenant_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			x.ID, x.ProjectID, x.System, x.Trigger, x.DryRun, x.Status, string(summary), string(ents), x.Error, x.StartedBy, x.StartedAt, nullTime(x.FinishedAt), tenantID); err != nil {
			return err
		}
		var cutoff time.Time
		err := c.row(ctx, `SELECT started_at FROM req_sync_runs WHERE project_id = ? AND system = ? ORDER BY started_at DESC, id DESC LIMIT 1 OFFSET ?`,
			x.ProjectID, x.System, keptSyncRuns).Scan(&cutoff)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		_, err = c.exec(ctx, `DELETE FROM req_sync_runs WHERE project_id = ? AND system = ? AND started_at <= ?`, x.ProjectID, x.System, cutoff)
		return err
	})
}

// ListReqSyncRuns returns a project's newest runs (system "" both; without
// their entries unless entries is set).
func (s *Store) ListReqSyncRuns(ctx context.Context, tenantID, projectID, system string, limit int, entries bool) ([]model.ReqSyncRun, error) {
	q := `SELECT ` + syncRunCols + ` FROM req_sync_runs WHERE tenant_id = ? AND project_id = ?`
	args := []any{tenantID, projectID}
	if system != "" {
		q += ` AND system = ?`
		args = append(args, system)
	}
	if limit <= 0 || limit > keptSyncRuns {
		limit = keptSyncRuns
	}
	rows, err := s.c(ctx).query(ctx, q+` ORDER BY started_at DESC, id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ReqSyncRun{}
	for rows.Next() {
		x, err := scanSyncRun(rows)
		if err != nil {
			return nil, err
		}
		if !entries {
			x.Entries = nil
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

// ReqSyncRun returns a run with its entries.
func (s *Store) ReqSyncRun(ctx context.Context, tenantID, projectID, id string) (*model.ReqSyncRun, error) {
	x, err := scanSyncRun(s.c(ctx).row(ctx, `SELECT `+syncRunCols+` FROM req_sync_runs WHERE tenant_id = ? AND project_id = ? AND id = ?`, tenantID, projectID, id))
	return x, mapErr(err, "sync run")
}
