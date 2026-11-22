package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/verification"
)

// Exploratory testing (model.ExploratorySession): sessions with a charter,
// a timer kept here (start, pause, stop), notes, a debrief, and the
// sessions that count as verification evidence.

const exploratoryCols = `x.id, x.project_id, x.key, x.title, x.mission, x.tester, x.timebox_minutes, x.status, x.started_at, x.running_since,
	x.elapsed_seconds, x.ended_at, x.summary, x.coverage_pct, x.setup_pct, x.testing_pct, x.bug_pct, x.outcome, x.debriefed_by,
	x.debriefed_name, x.debrief_comment, x.debriefed_at, x.version, x.created_by, x.created_at, x.updated_at, p.key`

const exploratoryFrom = ` FROM exploratory_sessions x JOIN projects p ON p.id = x.project_id`

// Limits of the texts of sessions and notes (characters).
const (
	maxExploratoryTitle   = 300
	maxExploratoryMission = 20000
	maxExploratoryNote    = 20000
	maxExploratoryScope   = 500
)

func scanExploratory(row interface{ Scan(...any) error }, now time.Time) (*model.ExploratorySession, error) {
	var x model.ExploratorySession
	var started, running, ended, debriefed sql.NullTime
	var d model.ExploratoryDebrief
	if err := row.Scan(&x.ID, &x.ProjectID, &x.Key, &x.Title, &x.Mission, &x.Tester, &x.TimeboxMinutes, &x.Status, &started, &running,
		&x.ElapsedSeconds, &ended, &d.Summary, &d.CoveragePct, &d.SetupPct, &d.TestingPct, &d.BugPct, &d.Outcome, &d.By,
		&d.Name, &d.Comment, &debriefed, &x.Version, &x.CreatedBy, &x.CreatedAt, &x.UpdatedAt, &x.ProjectKey); err != nil {
		return nil, err
	}
	x.StartedAt, x.RunningSince, x.EndedAt = timePtr(started), timePtr(running), timePtr(ended)
	x.ElapsedSeconds += runningSeconds(x.RunningSince, now)
	if debriefed.Valid {
		d.At = debriefed.Time
		d.Reviewed = d.By != "" && !strings.EqualFold(d.By, x.Tester)
		x.Debrief = &d
	}
	x.Scope = []model.ExploratoryScopeItem{}
	x.NoteTypes = map[string]int{}
	return &x, nil
}

// runningSeconds is the time since the timer was last started (0 when it
// does not run).
func runningSeconds(since *time.Time, now time.Time) int {
	if since == nil {
		return 0
	}
	d := now.Sub(*since)
	if d < 0 {
		return 0
	}
	return int(d / time.Second)
}

// ExploratoryOpen reports whether a session is still to be done or under way.
func ExploratoryOpen(status string) bool {
	return slices.Contains(model.ExploratoryOpenStatuses, status)
}

func validateExploratory(x *model.ExploratorySession) error {
	x.Title = strings.TrimSpace(x.Title)
	if x.Title == "" || len([]rune(x.Title)) > maxExploratoryTitle {
		return Invalidf("the charter title is required (at most %d characters)", maxExploratoryTitle)
	}
	x.Mission = strings.TrimSpace(x.Mission)
	if len([]rune(x.Mission)) > maxExploratoryMission {
		return Invalidf("the mission may have at most %d characters", maxExploratoryMission)
	}
	x.Tester = strings.ToLower(strings.TrimSpace(x.Tester))
	if x.Tester != "" && (!strings.Contains(x.Tester, "@") || len(x.Tester) > 320) {
		return Invalidf("tester must be an e-mail address")
	}
	if x.TimeboxMinutes == 0 {
		x.TimeboxMinutes = 60
	}
	if x.TimeboxMinutes < 5 || x.TimeboxMinutes > 480 {
		return Invalidf("timebox_minutes must be from 5 to 480")
	}
	if len(x.Scope) > maxExploratoryScope {
		return Invalidf("at most %d items in the scope", maxExploratoryScope)
	}
	return nil
}

// checkExploratoryScope de-duplicates the scope and checks that its items
// exist in the project.
func checkExploratoryScope(ctx context.Context, c conn, tenantID, projectID string, in []model.ExploratoryScopeItem) ([]model.ExploratoryScopeItem, error) {
	out := []model.ExploratoryScopeItem{}
	seen := map[string]bool{}
	for _, it := range in {
		it.ID = strings.TrimSpace(it.ID)
		if it.ID == "" || seen[it.Type+":"+it.ID] {
			continue
		}
		var q string
		switch it.Type {
		case "folder":
			q = `SELECT project_id FROM folders WHERE id = ? AND tenant_id = ?`
		case model.KindRequirement:
			q = `SELECT project_id FROM requirements WHERE id = ? AND tenant_id = ?`
		case model.KindTest:
			q = `SELECT project_id FROM test_cases WHERE id = ? AND tenant_id = ?`
		default:
			return nil, Invalidf("scope type must be one of %s", strings.Join(model.ExploratoryScopeTypes, ", "))
		}
		var pid string
		if err := c.row(ctx, q, it.ID, tenantID).Scan(&pid); err != nil {
			return nil, mapErr(err, it.Type)
		}
		if pid != projectID {
			return nil, Invalidf("the %s belongs to another project", it.Type)
		}
		seen[it.Type+":"+it.ID] = true
		out = append(out, model.ExploratoryScopeItem{Type: it.Type, ID: it.ID})
	}
	return out, nil
}

func writeExploratoryScope(ctx context.Context, c conn, id string, scope []model.ExploratoryScopeItem) error {
	if _, err := c.exec(ctx, `DELETE FROM exploratory_scope WHERE session_id = ?`, id); err != nil {
		return err
	}
	for _, it := range scope {
		if _, err := c.exec(ctx, `INSERT INTO exploratory_scope (session_id, entity_type, entity_id) VALUES (?, ?, ?)`, id, it.Type, it.ID); err != nil {
			return mapErr(err, "scope item")
		}
	}
	return nil
}

// CreateExploratorySession adds a planned session with a new key
// (PROJ-EXP-n).
func (s *Store) CreateExploratorySession(ctx context.Context, tenantID string, x *model.ExploratorySession) error {
	if err := validateExploratory(x); err != nil {
		return err
	}
	if _, err := s.GetProject(ctx, tenantID, x.ProjectID); err != nil {
		return err
	}
	x.ID = newID()
	x.Status = "planned"
	x.Version = 1
	x.CreatedAt = s.now()
	x.UpdatedAt = x.CreatedAt
	return s.tx(ctx, func(c conn) error {
		scope, err := checkExploratoryScope(ctx, c, tenantID, x.ProjectID, x.Scope)
		if err != nil {
			return err
		}
		x.Scope = scope
		var seq int
		err = c.row(ctx, `UPDATE projects SET exploratory_seq = exploratory_seq + 1 WHERE id = ? RETURNING exploratory_seq, key`, x.ProjectID).Scan(&seq, &x.ProjectKey)
		if err != nil {
			return mapErr(err, "project")
		}
		x.Key = x.ProjectKey + "-EXP-" + itoa(seq)
		if _, err := c.exec(ctx, `INSERT INTO exploratory_sessions (id, tenant_id, project_id, seq, key, title, mission, tester, timebox_minutes, status, version, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			x.ID, tenantID, x.ProjectID, seq, x.Key, x.Title, x.Mission, x.Tester, x.TimeboxMinutes, x.Status, x.Version, x.CreatedBy, x.CreatedAt, x.UpdatedAt); err != nil {
			return mapErr(err, "exploratory session")
		}
		return writeExploratoryScope(ctx, c, x.ID, x.Scope)
	})
}

// GetExploratorySession returns a session with its scope, note counts and
// linked defects.
func (s *Store) GetExploratorySession(ctx context.Context, tenantID, id string) (*model.ExploratorySession, error) {
	x, err := scanExploratory(s.c(ctx).row(ctx, `SELECT `+exploratoryCols+exploratoryFrom+` WHERE x.id = ? AND x.tenant_id = ?`, id, tenantID), s.now())
	if err != nil {
		return nil, mapErr(err, "exploratory session")
	}
	list := []model.ExploratorySession{*x}
	if err := s.decorateExploratory(ctx, tenantID, list, true); err != nil {
		return nil, err
	}
	return &list[0], nil
}

// ExploratoryFilter narrows ListExploratorySessions.
type ExploratoryFilter struct {
	// Status, Tester (e-mail) and Outcome; "" = any.
	Status, Tester, Outcome string
	// Statuses: any of them (nil = any).
	Statuses []string
	// Scope: sessions whose scope holds this item ("folder" | requirement
	// | test and its id).
	ScopeType, ScopeID string
}

// ListExploratorySessions returns the sessions of a project (projectID ""
// = every project of the tenant), newest first.
func (s *Store) ListExploratorySessions(ctx context.Context, tenantID, projectID string, f ExploratoryFilter) ([]model.ExploratorySession, error) {
	q := `SELECT ` + exploratoryCols + exploratoryFrom + ` WHERE x.tenant_id = ?`
	args := []any{tenantID}
	if projectID != "" {
		q += ` AND x.project_id = ?`
		args = append(args, projectID)
	}
	if f.Status != "" {
		q += ` AND x.status = ?`
		args = append(args, f.Status)
	}
	if len(f.Statuses) > 0 {
		q += ` AND x.status IN (` + placeholders(len(f.Statuses)) + `)`
		args = append(args, anySlice(f.Statuses)...)
	}
	if f.Tester != "" {
		q += ` AND x.tester = ?`
		args = append(args, strings.ToLower(strings.TrimSpace(f.Tester)))
	}
	if f.Outcome != "" {
		q += ` AND x.outcome = ?`
		args = append(args, f.Outcome)
	}
	if f.ScopeID != "" {
		q += ` AND x.id IN (SELECT session_id FROM exploratory_scope WHERE entity_type = ? AND entity_id = ?)`
		args = append(args, f.ScopeType, f.ScopeID)
	}
	rows, err := s.c(ctx).query(ctx, q+` ORDER BY x.created_at DESC, x.seq DESC`, args...)
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := []model.ExploratorySession{}
	for rows.Next() {
		x, err := scanExploratory(rows, now)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.decorateExploratory(ctx, tenantID, out, false); err != nil {
		return nil, err
	}
	return out, nil
}

// ListUserExploratorySessions returns the planned, running and paused
// sessions of a tester (e-mail) in every project of the tenant, the
// running ones first, then by creation (oldest first). It is the
// exploratory part of a person's work list; callers keep only the projects
// the person may see.
func (s *Store) ListUserExploratorySessions(ctx context.Context, tenantID, email string) ([]model.ExploratorySession, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return []model.ExploratorySession{}, nil
	}
	list, err := s.ListExploratorySessions(ctx, tenantID, "", ExploratoryFilter{Tester: email, Statuses: model.ExploratoryOpenStatuses})
	if err != nil {
		return nil, err
	}
	rank := map[string]int{"in_progress": 0, "paused": 1, "planned": 2}
	sort.SliceStable(list, func(i, j int) bool {
		if rank[list[i].Status] != rank[list[j].Status] {
			return rank[list[i].Status] < rank[list[j].Status]
		}
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})
	return list, nil
}

// decorateExploratory adds the scope (with keys and titles), the note
// counts and, with defects, the linked defects.
func (s *Store) decorateExploratory(ctx context.Context, tenantID string, list []model.ExploratorySession, defects bool) error {
	if len(list) == 0 {
		return nil
	}
	idx := map[string]int{}
	ids := make([]string, len(list))
	for i, x := range list {
		idx[x.ID] = i
		ids[i] = x.ID
	}
	for start := 0; start < len(ids); start += 500 {
		chunk := ids[start:min(start+500, len(ids))]
		in := placeholders(len(chunk))
		rows, err := s.c(ctx).query(ctx, `SELECT sc.session_id, sc.entity_type, sc.entity_id, COALESCE(r.key, t.key, f.kind, ''), COALESCE(r.title, t.title, f.name, '')
			FROM exploratory_scope sc
			LEFT JOIN requirements r ON sc.entity_type = 'requirement' AND r.id = sc.entity_id
			LEFT JOIN test_cases t ON sc.entity_type = 'test' AND t.id = sc.entity_id
			LEFT JOIN folders f ON sc.entity_type = 'folder' AND f.id = sc.entity_id
			WHERE sc.session_id IN (`+in+`) AND (r.id IS NOT NULL OR t.id IS NOT NULL OR f.id IS NOT NULL)
			ORDER BY sc.entity_type, COALESCE(r.key, t.key, f.name, '')`, anySlice(chunk)...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var sid string
			var it model.ExploratoryScopeItem
			if err := rows.Scan(&sid, &it.Type, &it.ID, &it.Key, &it.Title); err != nil {
				rows.Close()
				return err
			}
			x := &list[idx[sid]]
			x.Scope = append(x.Scope, it)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		rows, err = s.c(ctx).query(ctx, `SELECT session_id, type, defect_id IS NOT NULL, COUNT(*) FROM exploratory_notes
			WHERE session_id IN (`+in+`) GROUP BY session_id, type, defect_id IS NOT NULL`, anySlice(chunk)...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var sid, typ string
			var raised bool
			var n int
			if err := rows.Scan(&sid, &typ, &raised, &n); err != nil {
				rows.Close()
				return err
			}
			x := &list[idx[sid]]
			x.NoteCount += n
			x.NoteTypes[typ] += n
			if typ == "bug" || raised {
				x.BugCount += n
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	if !defects {
		return nil
	}
	for i := range list {
		rows, err := s.c(ctx).query(ctx, `SELECT d.id, d.key, d.title FROM defect_links l JOIN defects d ON d.id = l.defect_id
			WHERE l.entity_type = ? AND l.entity_id = ? AND d.tenant_id = ? ORDER BY d.seq`, model.KindExploratory, list[i].ID, tenantID)
		if err != nil {
			return err
		}
		list[i].Defects = []model.DefectLink{}
		for rows.Next() {
			l := model.DefectLink{EntityType: model.KindDefect}
			if err := rows.Scan(&l.EntityID, &l.Key, &l.Title); err != nil {
				rows.Close()
				return err
			}
			list[i].Defects = append(list[i].Defects, l)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	return nil
}

// UpdateExploratorySession saves the charter (title, mission, scope), the
// tester and the time box. A debriefed session's charter is frozen.
func (s *Store) UpdateExploratorySession(ctx context.Context, tenantID string, x *model.ExploratorySession) error {
	old, err := s.GetExploratorySession(ctx, tenantID, x.ID)
	if err != nil {
		return err
	}
	if old.Status == "debriefed" {
		return Conflictf("a debriefed session cannot be changed (reopen the debrief first)")
	}
	if err := validateExploratory(x); err != nil {
		return err
	}
	x.UpdatedAt = s.now()
	return s.tx(ctx, func(c conn) error {
		scope, err := checkExploratoryScope(ctx, c, tenantID, old.ProjectID, x.Scope)
		if err != nil {
			return err
		}
		x.Scope = scope
		if _, err := c.exec(ctx, `UPDATE exploratory_sessions SET title = ?, mission = ?, tester = ?, timebox_minutes = ?, version = version + 1, updated_at = ?
			WHERE id = ? AND tenant_id = ?`, x.Title, x.Mission, x.Tester, x.TimeboxMinutes, x.UpdatedAt, x.ID, tenantID); err != nil {
			return err
		}
		return writeExploratoryScope(ctx, c, x.ID, x.Scope)
	})
}

// DeleteExploratorySession removes a session with its notes, its
// attachments (files without other links) and its defect links.
func (s *Store) DeleteExploratorySession(ctx context.Context, tenantID, id string) error {
	return s.tx(ctx, func(c conn) error {
		var pid string
		if err := c.row(ctx, `SELECT project_id FROM exploratory_sessions WHERE id = ? AND tenant_id = ?`, id, tenantID).Scan(&pid); err != nil {
			return mapErr(err, "exploratory session")
		}
		if err := removeAttachments(ctx, c, model.KindExploratory, []string{id}); err != nil {
			return err
		}
		if err := deleteDefectLinks(ctx, c, model.KindExploratory, []string{id}); err != nil {
			return err
		}
		_, err := c.exec(ctx, `DELETE FROM exploratory_sessions WHERE id = ? AND tenant_id = ?`, id, tenantID)
		return err
	})
}

// Timer actions.
const (
	TimerStart = "start"
	TimerPause = "pause"
	TimerStop  = "stop"
)

// ExploratoryTimer starts, pauses or stops a session's timer: start
// (planned or paused → in_progress), pause (in_progress → paused), stop
// (in_progress or paused → completed). The elapsed time is recorded by
// the server, so it survives reloads and devices.
func (s *Store) ExploratoryTimer(ctx context.Context, tenantID, id, action string) (*model.ExploratorySession, error) {
	now := s.now()
	err := s.tx(ctx, func(c conn) error {
		var status string
		var running sql.NullTime
		var started sql.NullTime
		var elapsed int
		if err := c.row(ctx, `SELECT status, running_since, started_at, elapsed_seconds FROM exploratory_sessions WHERE id = ? AND tenant_id = ?`, id, tenantID).
			Scan(&status, &running, &started, &elapsed); err != nil {
			return mapErr(err, "exploratory session")
		}
		elapsed += runningSeconds(timePtr(running), now)
		switch action {
		case TimerStart:
			if status != "planned" && status != "paused" {
				return Conflictf("only a planned or paused session can be started (it is %s)", status)
			}
			first := timePtr(started)
			if first == nil {
				first = &now
			}
			_, err := c.exec(ctx, `UPDATE exploratory_sessions SET status = 'in_progress', running_since = ?, started_at = ?, elapsed_seconds = ?, updated_at = ? WHERE id = ?`,
				now, *first, elapsed, now, id)
			return err
		case TimerPause:
			if status != "in_progress" {
				return Conflictf("only a running session can be paused (it is %s)", status)
			}
			_, err := c.exec(ctx, `UPDATE exploratory_sessions SET status = 'paused', running_since = NULL, elapsed_seconds = ?, updated_at = ? WHERE id = ?`, elapsed, now, id)
			return err
		case TimerStop:
			if status != "in_progress" && status != "paused" {
				return Conflictf("only a running or paused session can be stopped (it is %s)", status)
			}
			_, err := c.exec(ctx, `UPDATE exploratory_sessions SET status = 'completed', running_since = NULL, elapsed_seconds = ?, ended_at = ?, version = version + 1, updated_at = ? WHERE id = ?`,
				elapsed, now, now, id)
			return err
		}
		return Invalidf("action must be start, pause or stop")
	})
	if err != nil {
		return nil, err
	}
	return s.GetExploratorySession(ctx, tenantID, id)
}

// ValidateDebrief checks a debrief: the summary, a coverage estimate and
// a time split in percent (0–100, the split summing to 100) and an outcome.
func ValidateDebrief(d *model.ExploratoryDebrief) error {
	d.Summary, d.Comment = strings.TrimSpace(d.Summary), strings.TrimSpace(d.Comment)
	if len([]rune(d.Summary)) > maxExploratoryMission || len([]rune(d.Comment)) > 8000 {
		return Invalidf("the summary may have at most %d characters, the comment 8000", maxExploratoryMission)
	}
	for _, v := range []int{d.CoveragePct, d.SetupPct, d.TestingPct, d.BugPct} {
		if v < 0 || v > 100 {
			return Invalidf("percentages must be from 0 to 100")
		}
	}
	if d.SetupPct+d.TestingPct+d.BugPct != 100 {
		return Invalidf("the time split (set-up, testing, bug investigation) must add up to 100%%")
	}
	if !slices.Contains(model.ExploratoryOutcomes, d.Outcome) {
		return Invalidf("outcome must be one of %s", strings.Join(model.ExploratoryOutcomes, ", "))
	}
	return nil
}

// DebriefExploratorySession records the debrief of a completed session
// (or replaces that of a debriefed one); by and name are who debriefs.
func (s *Store) DebriefExploratorySession(ctx context.Context, tenantID, id string, d *model.ExploratoryDebrief) (*model.ExploratorySession, error) {
	x, err := s.GetExploratorySession(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if x.Status != "completed" && x.Status != "debriefed" {
		return nil, Conflictf("stop the session before the debrief (it is %s)", x.Status)
	}
	if err := ValidateDebrief(d); err != nil {
		return nil, err
	}
	now := s.now()
	if _, err := s.c(ctx).exec(ctx, `UPDATE exploratory_sessions SET status = 'debriefed', summary = ?, coverage_pct = ?, setup_pct = ?, testing_pct = ?, bug_pct = ?,
		outcome = ?, debriefed_by = ?, debriefed_name = ?, debrief_comment = ?, debriefed_at = ?, version = version + 1, updated_at = ? WHERE id = ? AND tenant_id = ?`,
		d.Summary, d.CoveragePct, d.SetupPct, d.TestingPct, d.BugPct, d.Outcome, d.By, d.Name, d.Comment, now, now, id, tenantID); err != nil {
		return nil, err
	}
	return s.GetExploratorySession(ctx, tenantID, id)
}

// ReopenExploratorySession removes the debrief: debriefed → completed.
func (s *Store) ReopenExploratorySession(ctx context.Context, tenantID, id string) (*model.ExploratorySession, error) {
	now := s.now()
	res, err := s.c(ctx).exec(ctx, `UPDATE exploratory_sessions SET status = 'completed', summary = '', coverage_pct = -1, setup_pct = -1, testing_pct = -1, bug_pct = -1,
		outcome = '', debriefed_by = '', debriefed_name = '', debrief_comment = '', debriefed_at = NULL, version = version + 1, updated_at = ?
		WHERE id = ? AND tenant_id = ? AND status = 'debriefed'`, now, id, tenantID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := s.GetExploratorySession(ctx, tenantID, id); err != nil {
			return nil, err
		}
		return nil, Conflictf("only a debriefed session can be reopened")
	}
	return s.GetExploratorySession(ctx, tenantID, id)
}

// SnapshotExploratory is the history snapshot of a session (charter,
// tester, time box, status, timer and debrief).
func SnapshotExploratory(x *model.ExploratorySession) json.RawMessage {
	scope := make([]string, 0, len(x.Scope))
	for _, it := range x.Scope {
		scope = append(scope, it.Type+":"+it.ID)
	}
	sort.Strings(scope)
	m := map[string]any{"key": x.Key, "title": x.Title, "mission": x.Mission, "tester": x.Tester, "timebox_minutes": x.TimeboxMinutes,
		"status": x.Status, "scope": scope, "version": x.Version, "elapsed_minutes": x.ElapsedSeconds / 60}
	if x.Debrief != nil {
		d := x.Debrief
		m["summary"], m["coverage_pct"], m["setup_pct"], m["testing_pct"], m["bug_pct"] = d.Summary, d.CoveragePct, d.SetupPct, d.TestingPct, d.BugPct
		m["outcome"], m["debriefed_by"], m["debrief_comment"] = d.Outcome, d.By, d.Comment
	}
	b, _ := json.Marshal(m)
	return b
}

// ---------------------------------------------------------------- notes

const noteCols = `n.id, n.session_id, n.type, n.body, n.elapsed_seconds, n.file_ids, n.defect_id, COALESCE(d.key, ''), n.created_by, n.created_at, n.updated_at`

const noteFrom = ` FROM exploratory_notes n LEFT JOIN defects d ON d.id = n.defect_id`

func scanNote(row interface{ Scan(...any) error }) (*model.ExploratoryNote, error) {
	var n model.ExploratoryNote
	var files string
	var defect sql.NullString
	if err := row.Scan(&n.ID, &n.SessionID, &n.Type, &n.Body, &n.ElapsedSeconds, &files, &defect, &n.DefectKey, &n.CreatedBy, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(files), &n.FileIDs)
	if n.FileIDs == nil {
		n.FileIDs = []string{}
	}
	n.Files = []model.FileRef{}
	n.DefectID = strPtr(defect)
	return &n, nil
}

func validateNote(n *model.ExploratoryNote) error {
	if n.Type == "" {
		n.Type = "observation"
	}
	if !slices.Contains(model.ExploratoryNoteTypes, n.Type) {
		return Invalidf("type must be one of %s", strings.Join(model.ExploratoryNoteTypes, ", "))
	}
	n.Body = strings.TrimSpace(n.Body)
	if n.Body == "" || len([]rune(n.Body)) > maxExploratoryNote {
		return Invalidf("the note is required (at most %d characters)", maxExploratoryNote)
	}
	return nil
}

// noteFiles checks that the files are attachments of the session.
func noteFiles(ctx context.Context, c conn, sessionID string, ids []string) ([]string, error) {
	out := []string{}
	for _, id := range ids {
		if id == "" || slices.Contains(out, id) {
			continue
		}
		var n int
		if err := c.row(ctx, `SELECT COUNT(*) FROM attachments WHERE file_id = ? AND entity_type = ? AND entity_id = ?`, id, model.KindExploratory, sessionID).Scan(&n); err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, Invalidf("the file is not attached to the session")
		}
		out = append(out, id)
	}
	return out, nil
}

// AddExploratoryNote adds a note to a session at the session's current
// time. Notes are taken while the session runs, is paused or completed,
// not before it started nor after the debrief.
func (s *Store) AddExploratoryNote(ctx context.Context, tenantID string, n *model.ExploratoryNote) error {
	x, err := s.GetExploratorySession(ctx, tenantID, n.SessionID)
	if err != nil {
		return err
	}
	if x.Status == "planned" || x.Status == "debriefed" {
		return Conflictf("notes are taken from the start of the session until its debrief (it is %s)", x.Status)
	}
	if err := validateNote(n); err != nil {
		return err
	}
	n.ID = newID()
	n.ElapsedSeconds = x.ElapsedSeconds
	n.CreatedAt = s.now()
	n.UpdatedAt = n.CreatedAt
	n.DefectID = nil
	return s.tx(ctx, func(c conn) error {
		files, err := noteFiles(ctx, c, x.ID, n.FileIDs)
		if err != nil {
			return err
		}
		n.FileIDs = files
		fb, _ := json.Marshal(files)
		_, err = c.exec(ctx, `INSERT INTO exploratory_notes (id, tenant_id, session_id, type, body, elapsed_seconds, file_ids, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, n.ID, tenantID, n.SessionID, n.Type, n.Body, n.ElapsedSeconds, string(fb), n.CreatedBy, n.CreatedAt, n.UpdatedAt)
		return mapErr(err, "note")
	})
}

// GetExploratoryNote returns a note of a session.
func (s *Store) GetExploratoryNote(ctx context.Context, tenantID, sessionID, id string) (*model.ExploratoryNote, error) {
	n, err := scanNote(s.c(ctx).row(ctx, `SELECT `+noteCols+noteFrom+` WHERE n.id = ? AND n.session_id = ? AND n.tenant_id = ?`, id, sessionID, tenantID))
	if err != nil {
		return nil, mapErr(err, "note")
	}
	list := []model.ExploratoryNote{*n}
	if err := s.noteFileRefs(ctx, list); err != nil {
		return nil, err
	}
	return &list[0], nil
}

// ListExploratoryNotes returns the notes of a session in the order taken.
func (s *Store) ListExploratoryNotes(ctx context.Context, tenantID, sessionID string) ([]model.ExploratoryNote, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+noteCols+noteFrom+` WHERE n.session_id = ? AND n.tenant_id = ? ORDER BY n.created_at, n.id`, sessionID, tenantID)
	if err != nil {
		return nil, err
	}
	out := []model.ExploratoryNote{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, s.noteFileRefs(ctx, out)
}

func (s *Store) noteFileRefs(ctx context.Context, list []model.ExploratoryNote) error {
	var ids []string
	for _, n := range list {
		ids = append(ids, n.FileIDs...)
	}
	if len(ids) == 0 {
		return nil
	}
	refs := map[string]model.FileRef{}
	for start := 0; start < len(ids); start += 500 {
		chunk := ids[start:min(start+500, len(ids))]
		rows, err := s.c(ctx).query(ctx, `SELECT id, name, kind FROM files WHERE id IN (`+placeholders(len(chunk))+`)`, anySlice(chunk)...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var f model.FileRef
			if err := rows.Scan(&f.ID, &f.Name, &f.Kind); err != nil {
				rows.Close()
				return err
			}
			refs[f.ID] = f
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	for i := range list {
		kept := []string{}
		for _, id := range list[i].FileIDs {
			if f, ok := refs[id]; ok {
				kept = append(kept, id)
				list[i].Files = append(list[i].Files, f)
			}
		}
		list[i].FileIDs = kept
	}
	return nil
}

// UpdateExploratoryNote changes a note's type, text and files (not after
// the debrief).
func (s *Store) UpdateExploratoryNote(ctx context.Context, tenantID string, n *model.ExploratoryNote) error {
	x, err := s.GetExploratorySession(ctx, tenantID, n.SessionID)
	if err != nil {
		return err
	}
	if x.Status == "debriefed" {
		return Conflictf("the notes of a debriefed session cannot be changed")
	}
	if _, err := s.GetExploratoryNote(ctx, tenantID, n.SessionID, n.ID); err != nil {
		return err
	}
	if err := validateNote(n); err != nil {
		return err
	}
	n.UpdatedAt = s.now()
	return s.tx(ctx, func(c conn) error {
		files, err := noteFiles(ctx, c, x.ID, n.FileIDs)
		if err != nil {
			return err
		}
		fb, _ := json.Marshal(files)
		_, err = c.exec(ctx, `UPDATE exploratory_notes SET type = ?, body = ?, file_ids = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
			n.Type, n.Body, string(fb), n.UpdatedAt, n.ID, tenantID)
		return err
	})
}

// DeleteExploratoryNote removes a note (not after the debrief).
func (s *Store) DeleteExploratoryNote(ctx context.Context, tenantID, sessionID, id string) error {
	x, err := s.GetExploratorySession(ctx, tenantID, sessionID)
	if err != nil {
		return err
	}
	if x.Status == "debriefed" {
		return Conflictf("the notes of a debriefed session cannot be changed")
	}
	res, err := s.c(ctx).exec(ctx, `DELETE FROM exploratory_notes WHERE id = ? AND session_id = ? AND tenant_id = ?`, id, sessionID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "note")
	}
	return nil
}

// SetNoteDefect records the defect raised from a note.
func (s *Store) SetNoteDefect(ctx context.Context, tenantID, noteID, defectID string) error {
	_, err := s.c(ctx).exec(ctx, `UPDATE exploratory_notes SET defect_id = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`, defectID, s.now(), noteID, tenantID)
	return err
}

// ---------------------------------------------------------------- evidence

// ExploratoryEvidence returns the debriefed sessions of a project with the
// requirements in their scope, as verification evidence (see package
// verification). Callers use it only when the project counts them
// (VerificationSettings.ExploratoryEvidence).
func (s *Store) ExploratoryEvidence(ctx context.Context, tenantID, projectID string) ([]verification.Session, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT x.id, x.key, x.title, x.outcome, x.debriefed_at, sc.entity_id FROM exploratory_sessions x
		JOIN exploratory_scope sc ON sc.session_id = x.id AND sc.entity_type = 'requirement'
		WHERE x.tenant_id = ? AND x.project_id = ? AND x.status = 'debriefed' AND x.debriefed_at IS NOT NULL ORDER BY x.debriefed_at, x.id`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []verification.Session{}
	idx := map[string]int{}
	for rows.Next() {
		var v verification.Session
		var rid string
		if err := rows.Scan(&v.ID, &v.Key, &v.Title, &v.Outcome, &v.At, &rid); err != nil {
			return nil, err
		}
		i, ok := idx[v.ID]
		if !ok {
			i = len(out)
			idx[v.ID] = i
			out = append(out, v)
		}
		out[i].RequirementIDs = append(out[i].RequirementIDs, rid)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- metrics

func hours(sec int) float64 { return math.Round(float64(sec)/36) / 100 }

func perHour(bugs, sec int) float64 {
	if sec <= 0 {
		return 0
	}
	return math.Round(float64(bugs)*3600*100/float64(sec)) / 100
}

// ExploratoryMetrics computes the figures of a project's sessions: counts
// by status and outcome, session hours, bugs and bugs per session hour,
// the average time split and coverage of the debriefs, and the same per
// area (folder in the scope) and per tester.
func (s *Store) ExploratoryMetrics(ctx context.Context, tenantID, projectID string) (*model.ExploratoryMetrics, error) {
	list, err := s.ListExploratorySessions(ctx, tenantID, projectID, ExploratoryFilter{})
	if err != nil {
		return nil, err
	}
	m := &model.ExploratoryMetrics{ByStatus: map[string]int{}, ByOutcome: map[string]int{}, ByArea: []model.ExploratoryGroup{}, ByTester: []model.ExploratoryGroup{},
		AvgSetupPct: -1, AvgTestingPct: -1, AvgBugPct: -1, AvgCoveragePct: -1}
	type acc struct {
		g   model.ExploratoryGroup
		sec int
	}
	areas, testers := map[string]*acc{}, map[string]*acc{}
	add := func(set map[string]*acc, id, name string, x *model.ExploratorySession) {
		a := set[id]
		if a == nil {
			a = &acc{g: model.ExploratoryGroup{ID: id, Name: name}}
			set[id] = a
		}
		a.g.Sessions++
		a.g.Bugs += x.BugCount
		a.sec += x.ElapsedSeconds
	}
	totalSec := 0
	var split [4]int
	debriefs := 0
	for i := range list {
		x := &list[i]
		m.Sessions++
		m.ByStatus[x.Status]++
		m.Bugs += x.BugCount
		totalSec += x.ElapsedSeconds
		if x.Debrief != nil {
			m.ByOutcome[x.Debrief.Outcome]++
			debriefs++
			split[0] += x.Debrief.SetupPct
			split[1] += x.Debrief.TestingPct
			split[2] += x.Debrief.BugPct
			split[3] += x.Debrief.CoveragePct
		}
		inArea := false
		for _, it := range x.Scope {
			if it.Type == "folder" {
				add(areas, it.ID, it.Title, x)
				inArea = true
			}
		}
		if !inArea {
			add(areas, "", "", x)
		}
		add(testers, x.Tester, x.Tester, x)
	}
	m.Hours, m.BugsPerHour = hours(totalSec), perHour(m.Bugs, totalSec)
	if debriefs > 0 {
		avg := func(n int) float64 { return math.Round(float64(n)*10/float64(debriefs)) / 10 }
		m.AvgSetupPct, m.AvgTestingPct, m.AvgBugPct, m.AvgCoveragePct = avg(split[0]), avg(split[1]), avg(split[2]), avg(split[3])
	}
	rows := func(set map[string]*acc) []model.ExploratoryGroup {
		out := []model.ExploratoryGroup{}
		for _, a := range set {
			a.g.Hours, a.g.BugsPerHour = hours(a.sec), perHour(a.g.Bugs, a.sec)
			out = append(out, a.g)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Sessions != out[j].Sessions {
				return out[i].Sessions > out[j].Sessions
			}
			if (out[i].ID == "") != (out[j].ID == "") {
				return out[j].ID == ""
			}
			return out[i].Name < out[j].Name
		})
		return out
	}
	m.ByArea, m.ByTester = rows(areas), rows(testers)
	return m, nil
}
