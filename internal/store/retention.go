package store

// Data retention: the nightly job (and its dry-run preview) that deletes
// records older than the retention period of their category. Nothing of a
// project or release under legal hold is deleted, nor runs with an
// electronic signature. The audit trail is never purged: it is a hash
// chain whose verification needs every entry (see auditchain.go); it is
// archived by exporting it instead.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// retentionBatch is the number of rows deleted per statement.
const retentionBatch = 500

// NewRetentionPolicy returns the effective policy from the system values
// (days by category, keep latest) and an organisation's override.
func NewRetentionPolicy(days map[string]int, keepLatest int, override map[string]int) model.RetentionPolicy {
	p := model.RetentionPolicy{Days: map[string]int{}, RunsKeepLatest: keepLatest}
	for _, c := range model.RetentionCategories {
		p.Days[c] = days[c]
	}
	for _, k := range model.RetentionOverrideKeys {
		v, ok := override[k]
		if !ok {
			continue
		}
		p.Overridden = append(p.Overridden, k)
		if k == "runs_keep_latest" {
			p.RunsKeepLatest = v
		} else {
			p.Days[strings.TrimSuffix(k, "_days")] = v
		}
	}
	return p
}

// cutoff is the time before which records of a category are purged (zero
// for "forever").
func (s *Store) cutoff(p model.RetentionPolicy, category string) time.Time {
	d := p.Days[category]
	if d <= 0 {
		return time.Time{}
	}
	return s.now().Add(-time.Duration(d) * 24 * time.Hour)
}

// Retention previews (dryRun) or performs the purge of an organisation's
// records older than the policy. The counts are in the order of
// model.RetentionCategories; sign-in sessions and the sign-in log belong
// to no organisation (see RetentionSystem).
func (s *Store) Retention(ctx context.Context, tenantID string, p model.RetentionPolicy, dryRun bool) ([]model.RetentionCount, error) {
	out := make([]model.RetentionCount, 0, len(model.RetentionCategories))
	for _, cat := range model.RetentionCategories {
		rc := model.RetentionCount{Category: cat, Days: p.Days[cat]}
		before := s.cutoff(p, cat)
		if !before.IsZero() {
			var err error
			switch cat {
			case model.RetainRuns:
				err = s.retainRuns(ctx, tenantID, before, p.RunsKeepLatest, dryRun, &rc)
			case model.RetainLogs:
				err = s.retainLogs(ctx, tenantID, before, dryRun, &rc)
			case model.RetainNotifications:
				err = s.retainSimple(ctx, &rc, dryRun, `FROM notifications WHERE tenant_id = ? AND created_at < ?`, tenantID, before)
			case model.RetainSessions:
				err = s.retainSimple(ctx, &rc, dryRun, `FROM api_tokens WHERE tenant_id = ? AND expires_at IS NOT NULL AND expires_at < ?`, tenantID, before)
			case model.RetainChat:
				err = s.retainChat(ctx, tenantID, before, dryRun, &rc)
			case model.RetainExploratory:
				err = s.retainExploratory(ctx, tenantID, before, dryRun, &rc)
			}
			if err != nil {
				return nil, fmt.Errorf("retention %s: %w", cat, err)
			}
		}
		out = append(out, rc)
	}
	return out, nil
}

// RetentionSystem previews or purges what belongs to no organisation, with
// the system policy: sign-in sessions not used for longer than the
// sessions period and the sign-in log older than the logs period.
func (s *Store) RetentionSystem(ctx context.Context, p model.RetentionPolicy, dryRun bool) ([]model.RetentionCount, error) {
	out := []model.RetentionCount{}
	if before := s.cutoff(p, model.RetainLogs); !before.IsZero() {
		rc := model.RetentionCount{Category: model.RetainLogs, Days: p.Days[model.RetainLogs]}
		if err := s.retainSimple(ctx, &rc, dryRun, `FROM login_events WHERE created_at < ?`, before); err != nil {
			return nil, err
		}
		out = append(out, rc)
	} else {
		out = append(out, model.RetentionCount{Category: model.RetainLogs})
	}
	if before := s.cutoff(p, model.RetainSessions); !before.IsZero() {
		rc := model.RetentionCount{Category: model.RetainSessions, Days: p.Days[model.RetainSessions]}
		if err := s.retainSimple(ctx, &rc, dryRun, `FROM sessions WHERE COALESCE(last_seen, created_at) < ?`, before); err != nil {
			return nil, err
		}
		out = append(out, rc)
	} else {
		out = append(out, model.RetentionCount{Category: model.RetainSessions})
	}
	return out, nil
}

// retainSimple counts or deletes the rows of "FROM … WHERE …".
func (s *Store) retainSimple(ctx context.Context, rc *model.RetentionCount, dryRun bool, from string, args ...any) error {
	if dryRun {
		var n int
		if err := s.c(ctx).row(ctx, `SELECT COUNT(*) `+from, args...).Scan(&n); err != nil {
			return err
		}
		rc.Count += n
		return nil
	}
	res, err := s.c(ctx).exec(ctx, `DELETE `+from, args...)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	rc.Count += int(n)
	return nil
}

// heldProjects returns the projects of a tenant under legal hold.
func (s *Store) heldProjects(ctx context.Context, tenantID string) (map[string]bool, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id FROM projects WHERE tenant_id = ? AND legal_hold = ?`, tenantID, true)
	if err != nil {
		return nil, err
	}
	ids, err := scanStrings(rows)
	m := map[string]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m, err
}

// idProject is a candidate row: its id and its project.
type idProject struct{ id, project string }

func (s *Store) candidates(ctx context.Context, q string, args ...any) ([]idProject, error) {
	rows, err := s.c(ctx).query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []idProject
	for rows.Next() {
		var x idProject
		if err := rows.Scan(&x.id, &x.project); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// split separates the candidates of held projects (counted as held).
func split(list []idProject, held map[string]bool, rc *model.RetentionCount) []string {
	ids := make([]string, 0, len(list))
	for _, x := range list {
		if held[x.project] {
			rc.Held++
			continue
		}
		ids = append(ids, x.id)
	}
	return ids
}

// deleteIDs deletes rows of table by id, in batches.
func (s *Store) deleteIDs(ctx context.Context, table string, ids []string) error {
	for len(ids) > 0 {
		n := min(len(ids), retentionBatch)
		if _, err := s.c(ctx).exec(ctx, `DELETE FROM `+table+` WHERE id IN (`+placeholders(n)+`)`, anySlice(ids[:n])...); err != nil {
			return err
		}
		ids = ids[n:]
	}
	return nil
}

// retainRuns: runs created before the cutoff (not in progress), except
// those of a held project or release, those with an electronic signature
// and those holding one of the latest keep results of a test.
func (s *Store) retainRuns(ctx context.Context, tenantID string, before time.Time, keep int, dryRun bool, rc *model.RetentionCount) error {
	rows, err := s.c(ctx).query(ctx, `SELECT r.id, p.legal_hold, COALESCE(rel.legal_hold, ?) FROM test_runs r
		JOIN projects p ON p.id = r.project_id LEFT JOIN releases rel ON rel.id = r.release_id
		WHERE r.tenant_id = ? AND r.created_at < ? AND r.status <> 'in_progress' ORDER BY r.created_at`, false, tenantID, before)
	if err != nil {
		return err
	}
	type cand struct {
		id   string
		held bool
	}
	var list []cand
	for rows.Next() {
		var c cand
		var ph, rh bool
		if err := rows.Scan(&c.id, &ph, &rh); err != nil {
			rows.Close()
			return err
		}
		c.held = ph || rh
		list = append(list, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(list) == 0 {
		return nil
	}
	protect := map[string]bool{}
	signed, err := s.c(ctx).query(ctx, `SELECT entity_id FROM esignatures WHERE tenant_id = ? AND entity_type = 'run'
		UNION SELECT e.run_id FROM esignatures g JOIN test_executions e ON e.id = g.entity_id WHERE g.tenant_id = ? AND g.entity_type = 'execution'`, tenantID, tenantID)
	if err != nil {
		return err
	}
	ids, err := scanStrings(signed)
	if err != nil {
		return err
	}
	for _, id := range ids {
		protect[id] = true
	}
	latest := map[string]bool{}
	if keep > 0 {
		rows, err := s.c(ctx).query(ctx, `SELECT DISTINCT run_id FROM (
			SELECT e.run_id, ROW_NUMBER() OVER (PARTITION BY e.test_case_id ORDER BY r.created_at DESC, r.id DESC) AS rn
			FROM test_executions e JOIN test_runs r ON r.id = e.run_id WHERE e.tenant_id = ?) x WHERE rn <= ?`, tenantID, keep)
		if err != nil {
			return err
		}
		ids, err := scanStrings(rows)
		if err != nil {
			return err
		}
		for _, id := range ids {
			latest[id] = true
		}
	}
	for _, c := range list {
		switch {
		case latest[c.id]:
			// kept as one of the latest results of a test (not "held")
		case c.held || protect[c.id]:
			rc.Held++
		case dryRun:
			rc.Count++
		default:
			if err := s.DeleteRun(ctx, tenantID, c.id); err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
			rc.Count++
		}
	}
	return nil
}

// retainLogs: finished test runner jobs, sent or failed CI status posts,
// finished tracker syncs and web test evidence (screenshots, traces,
// videos) of old executions.
func (s *Store) retainLogs(ctx context.Context, tenantID string, before time.Time, dryRun bool, rc *model.RetentionCount) error {
	held, err := s.heldProjects(ctx, tenantID)
	if err != nil {
		return err
	}
	for _, x := range []struct{ table, q string }{
		{"script_jobs", `SELECT id, project_id FROM script_jobs WHERE tenant_id = ? AND created_at < ? AND state NOT IN ('queued', 'running')`},
		{"ci_deliveries", `SELECT id, project_id FROM ci_deliveries WHERE tenant_id = ? AND created_at < ? AND status IN ('sent', 'failed')`},
		{"req_sync_runs", `SELECT id, project_id FROM req_sync_runs WHERE tenant_id = ? AND started_at < ? AND finished_at IS NOT NULL`},
	} {
		list, err := s.candidates(ctx, x.q, tenantID, before)
		if err != nil {
			return err
		}
		ids := split(list, held, rc)
		rc.Count += len(ids)
		if !dryRun {
			if err := s.deleteIDs(ctx, x.table, ids); err != nil {
				return err
			}
		}
	}
	// web test evidence: files tagged "web-test" attached to executions of
	// runs whose release is not held
	rows, err := s.c(ctx).query(ctx, `SELECT f.id, COALESCE(f.project_id, ''), a.entity_id, COALESCE(rel.legal_hold, ?) FROM files f
		JOIN attachments a ON a.file_id = f.id AND a.entity_type = 'execution'
		JOIN test_executions e ON e.id = a.entity_id JOIN test_runs r ON r.id = e.run_id LEFT JOIN releases rel ON rel.id = r.release_id
		WHERE f.tenant_id = ? AND f.kind = 'attachment' AND f.created_at < ? AND f.tags LIKE ?`, false, tenantID, before, `%"web-test"%`)
	if err != nil {
		return err
	}
	type ev struct {
		file, project, exec string
		relHeld             bool
	}
	var evs []ev
	for rows.Next() {
		var x ev
		if err := rows.Scan(&x.file, &x.project, &x.exec, &x.relHeld); err != nil {
			rows.Close()
			return err
		}
		evs = append(evs, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, x := range evs {
		if held[x.project] || x.relHeld {
			rc.Held++
			continue
		}
		rc.Count++
		if !dryRun {
			if err := s.Detach(ctx, tenantID, x.file, "execution", x.exec); err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
		}
	}
	return nil
}

// retainChat: chat messages sent or given up (with their text) and the
// personal input suggestion history.
func (s *Store) retainChat(ctx context.Context, tenantID string, before time.Time, dryRun bool, rc *model.RetentionCount) error {
	held, err := s.heldProjects(ctx, tenantID)
	if err != nil {
		return err
	}
	list, err := s.candidates(ctx, `SELECT d.id, c.project_id FROM chat_deliveries d JOIN chat_channels c ON c.id = d.channel_id
		WHERE d.tenant_id = ? AND d.created_at < ? AND d.status IN ('sent', 'failed')`, tenantID, before)
	if err != nil {
		return err
	}
	ids := split(list, held, rc)
	rc.Count += len(ids)
	if !dryRun {
		if err := s.deleteIDs(ctx, "chat_deliveries", ids); err != nil {
			return err
		}
	}
	return s.retainSimple(ctx, rc, dryRun, `FROM suggest_history WHERE tenant_id = ? AND last_used < ?`, tenantID, before)
}

// retainExploratory: the notes of completed (or debriefed) exploratory
// sessions last changed before the cutoff; the sessions and their debrief
// stay.
func (s *Store) retainExploratory(ctx context.Context, tenantID string, before time.Time, dryRun bool, rc *model.RetentionCount) error {
	held, err := s.heldProjects(ctx, tenantID)
	if err != nil {
		return err
	}
	list, err := s.candidates(ctx, `SELECT n.id, x.project_id FROM exploratory_notes n JOIN exploratory_sessions x ON x.id = n.session_id
		WHERE n.tenant_id = ? AND x.status IN ('completed', 'debriefed') AND x.updated_at < ?`, tenantID, before)
	if err != nil {
		return err
	}
	ids := split(list, held, rc)
	rc.Count += len(ids)
	if dryRun {
		return nil
	}
	return s.deleteIDs(ctx, "exploratory_notes", ids)
}

// ---------------------------------------------------------------- legal hold

// ErrLegalHold is returned when deleting what is under legal hold.
var ErrLegalHold = Conflictf("this is under legal hold and cannot be deleted: a super administrator or organisation administrator must lift the hold first")

// legalHoldGuard fails with ErrLegalHold when the count query (whose last
// argument is appended: true) finds a held row.
func legalHoldGuard(ctx context.Context, c conn, q string, args ...any) error {
	if len(args) == 1 {
		args = append(args, true)
	}
	var n int
	if err := c.row(ctx, q, args...).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrLegalHold
	}
	return nil
}

func scanHold(on bool, reason, by string, at sql.NullTime) model.LegalHold {
	return model.LegalHold{On: on, Reason: reason, By: by, At: timePtr(at)}
}

// ProjectLegalHold returns the legal hold of a project and of its
// releases.
func (s *Store) ProjectLegalHold(ctx context.Context, tenantID, projectID string) (model.LegalHold, []model.ReleaseHold, error) {
	var on bool
	var reason, by string
	var at sql.NullTime
	err := s.c(ctx).row(ctx, `SELECT legal_hold, legal_hold_reason, legal_hold_by, legal_hold_at FROM projects WHERE id = ? AND tenant_id = ?`, projectID, tenantID).
		Scan(&on, &reason, &by, &at)
	if err != nil {
		return model.LegalHold{}, nil, mapErr(err, "project")
	}
	h := scanHold(on, reason, by, at)
	rows, err := s.c(ctx).query(ctx, `SELECT id, name, version, legal_hold, legal_hold_reason, legal_hold_by, legal_hold_at FROM releases
		WHERE project_id = ? AND tenant_id = ? ORDER BY created_at DESC`, projectID, tenantID)
	if err != nil {
		return h, nil, err
	}
	defer rows.Close()
	rels := []model.ReleaseHold{}
	for rows.Next() {
		var r model.ReleaseHold
		if err := rows.Scan(&r.ID, &r.Name, &r.Version, &on, &reason, &by, &at); err != nil {
			return h, nil, err
		}
		r.Hold = scanHold(on, reason, by, at)
		rels = append(rels, r)
	}
	return h, rels, rows.Err()
}

// normHold validates a hold being set.
func normHold(h *model.LegalHold) error {
	h.Reason = strings.TrimSpace(h.Reason)
	if h.On && h.Reason == "" {
		return Invalidf("give the reason for the legal hold (e.g. the case or request it is for)")
	}
	if len(h.Reason) > 500 || strings.ContainsAny(h.Reason, "\r\n") {
		return Invalidf("the reason must be one line of at most 500 characters")
	}
	if !h.On {
		h.Reason = ""
	}
	return nil
}

// SetLegalHold sets or lifts the legal hold of a project (releaseID "")
// or of one of its releases; by is the e-mail of who does it.
func (s *Store) SetLegalHold(ctx context.Context, tenantID, projectID, releaseID string, h model.LegalHold, by string) (model.LegalHold, error) {
	if err := normHold(&h); err != nil {
		return h, err
	}
	now := s.now()
	var at any
	h.By, h.At = "", nil
	if h.On {
		h.By, h.At, at = by, &now, now
	}
	var res sql.Result
	var err error
	if releaseID == "" {
		res, err = s.c(ctx).exec(ctx, `UPDATE projects SET legal_hold = ?, legal_hold_reason = ?, legal_hold_by = ?, legal_hold_at = ? WHERE id = ? AND tenant_id = ?`,
			h.On, h.Reason, h.By, at, projectID, tenantID)
	} else {
		res, err = s.c(ctx).exec(ctx, `UPDATE releases SET legal_hold = ?, legal_hold_reason = ?, legal_hold_by = ?, legal_hold_at = ? WHERE id = ? AND project_id = ? AND tenant_id = ?`,
			h.On, h.Reason, h.By, at, releaseID, projectID, tenantID)
	}
	if err != nil {
		return h, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		what := "project"
		if releaseID != "" {
			what = "release"
		}
		return h, mapErr(sql.ErrNoRows, what)
	}
	return h, nil
}

// LegalHolds lists the projects and releases under legal hold (tenantID
// "" = all organisations).
func (s *Store) LegalHolds(ctx context.Context, tenantID string) ([]model.HeldItem, error) {
	q := `SELECT 'project', p.id, p.tenant_id, t.name, p.id, p.key, p.name, p.name, p.legal_hold_reason, p.legal_hold_by, p.legal_hold_at
		FROM projects p JOIN tenants t ON t.id = p.tenant_id WHERE p.legal_hold = ?`
	q2 := `SELECT 'release', r.id, r.tenant_id, t.name, p.id, p.key, p.name, r.name, r.legal_hold_reason, r.legal_hold_by, r.legal_hold_at
		FROM releases r JOIN projects p ON p.id = r.project_id JOIN tenants t ON t.id = r.tenant_id WHERE r.legal_hold = ?`
	args := []any{true}
	if tenantID != "" {
		q += ` AND p.tenant_id = ?`
		q2 += ` AND r.tenant_id = ?`
		args = append(args, tenantID)
	}
	rows, err := s.c(ctx).query(ctx, q+` UNION ALL `+q2, append(slices.Clone(args), args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.HeldItem{}
	for rows.Next() {
		var x model.HeldItem
		var at sql.NullTime
		if err := rows.Scan(&x.Kind, &x.ID, &x.TenantID, &x.TenantName, &x.ProjectID, &x.ProjectKey, &x.ProjectName, &x.Name, &x.Hold.Reason, &x.Hold.By, &at); err != nil {
			return nil, err
		}
		x.Hold.On, x.Hold.At = true, timePtr(at)
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b model.HeldItem) int {
		return strings.Compare(a.TenantName+"\x00"+a.ProjectKey+"\x00"+a.Kind+a.Name, b.TenantName+"\x00"+b.ProjectKey+"\x00"+b.Kind+b.Name)
	})
	return out, nil
}
