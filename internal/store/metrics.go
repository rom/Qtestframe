package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// Metrics: daily snapshots of a project's counts (trends over time), the
// burn-up of a release and the opened / closed defects per week. Days are
// UTC calendar days written as YYYY-MM-DD.

// Snapshot sources.
const (
	SnapshotRecorded = "recorded"
	SnapshotBackfill = "backfill"
)

// Limits of the metrics history.
const (
	// MaxBackfillDays bounds how far back a first recording reconstructs
	// snapshots from history.
	MaxBackfillDays = 365
	// MaxTrendDays bounds the range of a trend request.
	MaxTrendDays = 3660
	// SnapshotRefresh: opening the dashboard re-records today's snapshot
	// when it is older than this.
	SnapshotRefresh = time.Hour
)

// MetricData holds the counts of one snapshot. Maps are keyed by status,
// result, severity or level. RequirementStatus, TestStatus and OpenRisks
// are not known for backfilled days (no history of them is kept).
type MetricData struct {
	Requirements int `json:"requirements"`
	// Covered: requirements with at least one linked test.
	Covered int `json:"covered"`
	// Coverage: requirements by coverage (passed, failed, partial, not_run, none).
	Coverage          map[string]int `json:"coverage"`
	RequirementStatus map[string]int `json:"requirement_status,omitempty"`
	Tests             int            `json:"tests"`
	TestStatus        map[string]int `json:"test_status,omitempty"`
	// Results: tests by their last result (not_run when never executed).
	Results map[string]int `json:"results"`
	// Executions: results recorded on that day.
	Executions  map[string]int `json:"executions"`
	OpenDefects map[string]int `json:"open_defects"`
	OpenRisks   map[string]int `json:"open_risks,omitempty"`
}

// CoveragePct is the share of requirements with linked tests (0–100).
func (d *MetricData) CoveragePct() float64 { return pct(d.Covered, d.Requirements) }

// PassRate is the share of executed tests whose last result passed (0–100),
// as on the dashboard: tests not run or in progress do not count.
func (d *MetricData) PassRate() float64 {
	executed := d.Tests - d.Results[model.ResultNotRun] - d.Results[model.ResultInProgress]
	return pct(d.Results[model.ResultPassed], executed)
}

func pct(n, total int) float64 {
	if total <= 0 {
		return 0
	}
	return round1(float64(n) * 100 / float64(total))
}

// MetricSnapshot is one day of a project's metrics.
type MetricSnapshot struct {
	Day       string     `json:"day"`
	Source    string     `json:"source"`
	Data      MetricData `json:"data"`
	CreatedAt time.Time  `json:"created_at"`
}

// Day returns the UTC calendar day of t.
func Day(t time.Time) string { return t.UTC().Format(time.DateOnly) }

// dayStart parses a YYYY-MM-DD day as midnight UTC.
func dayStart(day string) (time.Time, error) { return time.Parse(time.DateOnly, day) }

// ---------------------------------------------------------------- history

// resultEvent is an execution result with the time it was recorded.
type resultEvent struct {
	test, status, run, exec string
	at                      time.Time
}

// resultEvents returns the results (not not_run) of a project's executions
// in time order; releaseID limits them to the runs of a release. The time
// of a result is finished_at, else started_at, else the run's creation (as
// in LastResults).
func resultEvents(ctx context.Context, c conn, tenantID, projectID, releaseID string) ([]resultEvent, error) {
	q := `SELECT e.id, e.test_case_id, e.status, e.finished_at, e.started_at, r.id, r.created_at FROM test_executions e JOIN test_runs r ON r.id = e.run_id
		WHERE r.project_id = ? AND e.tenant_id = ? AND e.status <> 'not_run'`
	args := []any{projectID, tenantID}
	if releaseID != "" {
		q += ` AND r.release_id = ?`
		args = append(args, releaseID)
	}
	rows, err := c.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []resultEvent
	for rows.Next() {
		var ev resultEvent
		var fin, start sql.NullTime
		if err := rows.Scan(&ev.exec, &ev.test, &ev.status, &fin, &start, &ev.run, &ev.at); err != nil {
			return nil, err
		}
		if fin.Valid {
			ev.at = fin.Time
		} else if start.Valid {
			ev.at = start.Time
		}
		ev.at = ev.at.UTC()
		out = append(out, ev)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].at.Equal(out[j].at) {
			return out[i].at.Before(out[j].at)
		}
		return out[i].exec < out[j].exec
	})
	return out, rows.Err()
}

// dated is an item id with its creation time (and an optional value).
type dated struct {
	id, value string
	at        time.Time
}

func datedRows(ctx context.Context, c conn, q string, args ...any) ([]dated, error) {
	rows, err := c.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dated
	for rows.Next() {
		var d dated
		if err := rows.Scan(&d.id, &d.value, &d.at); err != nil {
			return nil, err
		}
		d.at = d.at.UTC()
		out = append(out, d)
	}
	return out, rows.Err()
}

// projectLink is a trace link with its creation time.
type projectLink struct {
	req, test string
	at        time.Time
}

func projectLinks(ctx context.Context, c conn, tenantID, projectID string) ([]projectLink, error) {
	rows, err := c.query(ctx, `SELECT requirement_id, test_case_id, created_at FROM trace_links WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []projectLink
	for rows.Next() {
		var l projectLink
		if err := rows.Scan(&l.req, &l.test, &l.at); err != nil {
			return nil, err
		}
		l.at = l.at.UTC()
		out = append(out, l)
	}
	return out, rows.Err()
}

// defectSpan is a defect's severity and open period.
type defectSpan struct {
	severity   string
	open       bool
	created    time.Time
	resolvedAt *time.Time
}

func projectDefects(ctx context.Context, c conn, tenantID, projectID string) ([]defectSpan, error) {
	rows, err := c.query(ctx, `SELECT severity, status, created_at, resolved_at FROM defects WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []defectSpan
	for rows.Next() {
		var d defectSpan
		var status string
		var res sql.NullTime
		if err := rows.Scan(&d.severity, &status, &d.created, &res); err != nil {
			return nil, err
		}
		d.open = DefectOpen(status)
		d.created = d.created.UTC()
		if res.Valid && !d.open {
			t := res.Time.UTC()
			d.resolvedAt = &t
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// openAt reports whether the defect was open at time t.
func (d defectSpan) openAt(t time.Time) bool {
	if d.created.After(t) {
		return false
	}
	if d.open {
		return true
	}
	return d.resolvedAt != nil && d.resolvedAt.After(t)
}

// projectHistory is everything needed to compute a project's metrics at
// any point in time.
type projectHistory struct {
	reqs, tests []dated // value: status
	links       []projectLink
	events      []resultEvent
	defects     []defectSpan
	risks       map[string]int
}

func loadHistory(ctx context.Context, c conn, tenantID, projectID string) (*projectHistory, error) {
	h := &projectHistory{}
	var err error
	if h.reqs, err = datedRows(ctx, c, `SELECT id, status, created_at FROM requirements WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID); err != nil {
		return nil, err
	}
	if h.tests, err = datedRows(ctx, c, `SELECT id, status, created_at FROM test_cases WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID); err != nil {
		return nil, err
	}
	if h.links, err = projectLinks(ctx, c, tenantID, projectID); err != nil {
		return nil, err
	}
	if h.events, err = resultEvents(ctx, c, tenantID, projectID, ""); err != nil {
		return nil, err
	}
	if h.defects, err = projectDefects(ctx, c, tenantID, projectID); err != nil {
		return nil, err
	}
	rows, err := c.query(ctx, `SELECT likelihood, impact, status FROM risks WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	h.risks = map[string]int{}
	for rows.Next() {
		var l, i int
		var st string
		if err := rows.Scan(&l, &i, &st); err != nil {
			return nil, err
		}
		if RiskOpen(st) {
			h.risks[model.RiskLevel(l, i)]++
		}
	}
	return h, rows.Err()
}

// replay walks a project's history day by day. Each call of at(end) moves
// the state to the time end (which must not decrease) and returns the
// metrics of the items that existed before end.
type replay struct {
	h    *projectHistory
	last map[string]string
	next int
	// day → results recorded that day
	perDay map[string]map[string]int
}

func newReplay(h *projectHistory) *replay {
	r := &replay{h: h, last: map[string]string{}, perDay: map[string]map[string]int{}}
	for _, ev := range h.events {
		d := Day(ev.at)
		if r.perDay[d] == nil {
			r.perDay[d] = map[string]int{}
		}
		r.perDay[d][ev.status]++
	}
	return r
}

// advance applies the results recorded before end.
func (r *replay) advance(end time.Time) {
	for r.next < len(r.h.events) && r.h.events[r.next].at.Before(end) {
		ev := r.h.events[r.next]
		r.last[ev.test] = ev.status
		r.next++
	}
}

// linkIndex returns requirement id → tests linked before end.
func (r *replay) linkIndex(end time.Time) map[string][]string {
	out := map[string][]string{}
	for _, l := range r.h.links {
		if l.at.Before(end) {
			out[l.req] = append(out[l.req], l.test)
		}
	}
	return out
}

// at computes the metrics at time end; Executions are those of the day
// before end. current also fills the statuses and risks, which have no
// history, and counts every item.
func (r *replay) at(end time.Time, current bool) MetricData {
	r.advance(end)
	d := MetricData{Coverage: map[string]int{}, Results: map[string]int{}, Executions: map[string]int{}, OpenDefects: map[string]int{}}
	if current {
		d.RequirementStatus, d.TestStatus, d.OpenRisks = map[string]int{}, map[string]int{}, map[string]int{}
		for k, v := range r.h.risks {
			d.OpenRisks[k] = v
		}
	}
	r2t := r.linkIndex(end)
	for _, q := range r.h.reqs {
		if !current && !q.at.Before(end) {
			continue
		}
		d.Requirements++
		if len(r2t[q.id]) > 0 {
			d.Covered++
		}
		d.Coverage[Coverage(r2t[q.id], r.last)]++
		if current {
			d.RequirementStatus[q.value]++
		}
	}
	for _, t := range r.h.tests {
		if !current && !t.at.Before(end) {
			continue
		}
		d.Tests++
		res := r.last[t.id]
		if res == "" {
			res = model.ResultNotRun
		}
		d.Results[res]++
		if current {
			d.TestStatus[t.value]++
		}
	}
	for k, v := range r.perDay[Day(end.Add(-time.Nanosecond))] {
		d.Executions[k] = v
	}
	for _, df := range r.h.defects {
		if (current && df.open) || (!current && df.openAt(end)) {
			d.OpenDefects[df.severity]++
		}
	}
	return d
}

// ---------------------------------------------------------------- snapshots

// ComputeMetrics returns a project's metrics now.
func (s *Store) ComputeMetrics(ctx context.Context, tenantID, projectID string) (MetricData, error) {
	h, err := loadHistory(ctx, s.c(ctx), tenantID, projectID)
	if err != nil {
		return MetricData{}, err
	}
	now := s.now()
	return newReplay(h).at(now.Add(time.Nanosecond), true), nil
}

func putSnapshot(ctx context.Context, c conn, tenantID, projectID, day, source string, d MetricData, now time.Time, replace bool) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	q := `INSERT INTO metric_snapshots (project_id, tenant_id, day, source, data, created_at) VALUES (?, ?, ?, ?, ?, ?)`
	if replace {
		q += ` ON CONFLICT (project_id, day) DO UPDATE SET source = excluded.source, data = excluded.data, created_at = excluded.created_at`
	} else {
		q += ` ON CONFLICT DO NOTHING`
	}
	_, err = c.exec(ctx, q, projectID, tenantID, day, source, string(b), now)
	return err
}

// RecordSnapshot records (or, when refresh is set and the existing one is
// older than SnapshotRefresh, re-records) today's snapshot of a project.
// A project without any snapshot first gets the days before today
// reconstructed from its history (see Backfill). It reports whether a
// snapshot was written.
func (s *Store) RecordSnapshot(ctx context.Context, tenantID, projectID string, refresh bool) (bool, error) {
	now := s.now()
	today := Day(now)
	wrote := false
	err := s.tx(ctx, func(c conn) error {
		var at time.Time
		err := c.row(ctx, `SELECT created_at FROM metric_snapshots WHERE project_id = ? AND day = ?`, projectID, today).Scan(&at)
		switch {
		case err == nil:
			if !refresh || now.Sub(at) < SnapshotRefresh {
				return nil
			}
		case errors.Is(err, sql.ErrNoRows):
		default:
			return err
		}
		var n int
		if err := c.row(ctx, `SELECT COUNT(*) FROM metric_snapshots WHERE project_id = ?`, projectID).Scan(&n); err != nil {
			return err
		}
		h, err := loadHistory(ctx, c, tenantID, projectID)
		if err != nil {
			return err
		}
		if n == 0 {
			if err := backfill(ctx, c, tenantID, projectID, h, now); err != nil {
				return err
			}
		}
		wrote = true
		return putSnapshot(ctx, c, tenantID, projectID, today, SnapshotRecorded, newReplay(h).at(now.Add(time.Nanosecond), true), now, true)
	})
	return wrote, err
}

// backfill writes reconstructed snapshots for the days before today, from
// the first day with data (at most MaxBackfillDays back). Days that already
// have a snapshot are kept.
func backfill(ctx context.Context, c conn, tenantID, projectID string, h *projectHistory, now time.Time) error {
	today, _ := dayStart(Day(now))
	first := today
	for _, ts := range [][]time.Time{eventTimes(h.events), datedTimes(h.reqs), datedTimes(h.tests)} {
		if len(ts) > 0 && ts[0].Before(first) {
			first = ts[0]
		}
	}
	for _, d := range h.defects {
		if d.created.Before(first) {
			first = d.created
		}
	}
	start, _ := dayStart(Day(first))
	if limit := today.AddDate(0, 0, -MaxBackfillDays); start.Before(limit) {
		start = limit
	}
	rp := newReplay(h)
	for day := start; day.Before(today); day = day.AddDate(0, 0, 1) {
		if err := putSnapshot(ctx, c, tenantID, projectID, Day(day), SnapshotBackfill, rp.at(day.AddDate(0, 0, 1), false), now, false); err != nil {
			return err
		}
	}
	return nil
}

func eventTimes(evs []resultEvent) []time.Time {
	if len(evs) == 0 {
		return nil
	}
	return []time.Time{evs[0].at}
}

func datedTimes(ds []dated) []time.Time {
	var min []time.Time
	for _, d := range ds {
		if len(min) == 0 || d.at.Before(min[0]) {
			min = []time.Time{d.at}
		}
	}
	return min
}

// RecordAllSnapshots records today's snapshot of every project that has
// none yet (the daily background job). It returns the number written.
func (s *Store) RecordAllSnapshots(ctx context.Context) (int, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT p.tenant_id, p.id FROM projects p WHERE NOT EXISTS (SELECT 1 FROM metric_snapshots m WHERE m.project_id = p.id AND m.day = ?) ORDER BY p.created_at`, Day(s.now()))
	if err != nil {
		return 0, err
	}
	var ids [][2]string
	for rows.Next() {
		var t, p string
		if err := rows.Scan(&t, &p); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, [2]string{t, p})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		wrote, err := s.RecordSnapshot(ctx, id[0], id[1], false)
		if err != nil {
			return n, err
		}
		if wrote {
			n++
		}
	}
	return n, nil
}

// ListSnapshots returns a project's snapshots from day from to day to
// (inclusive, YYYY-MM-DD), oldest first.
func (s *Store) ListSnapshots(ctx context.Context, tenantID, projectID, from, to string) ([]MetricSnapshot, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT day, source, data, created_at FROM metric_snapshots WHERE tenant_id = ? AND project_id = ? AND day >= ? AND day <= ? ORDER BY day`,
		tenantID, projectID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MetricSnapshot{}
	for rows.Next() {
		var m MetricSnapshot
		var data string
		if err := rows.Scan(&m.Day, &m.Source, &data, &m.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(data), &m.Data)
		m.CreatedAt = m.CreatedAt.UTC()
		out = append(out, m)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- defects per week

// WeekCount is the number of defects opened and closed in a week (Monday
// to Sunday, UTC); Week is the Monday.
type WeekCount struct {
	Week   string `json:"week"`
	Opened int    `json:"opened"`
	Closed int    `json:"closed"`
}

// WeekStart returns the Monday (UTC) of t's week.
func WeekStart(t time.Time) time.Time {
	d, _ := dayStart(Day(t))
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

// DefectWeeks counts the defects opened and closed per week from the week
// of from to the week of to.
func (s *Store) DefectWeeks(ctx context.Context, tenantID, projectID string, from, to time.Time) ([]WeekCount, error) {
	ds, err := projectDefects(ctx, s.c(ctx), tenantID, projectID)
	if err != nil {
		return nil, err
	}
	first, last := WeekStart(from), WeekStart(to)
	out := []WeekCount{}
	idx := map[string]int{}
	for w := first; !w.After(last); w = w.AddDate(0, 0, 7) {
		idx[Day(w)] = len(out)
		out = append(out, WeekCount{Week: Day(w)})
	}
	for _, d := range ds {
		if i, ok := idx[Day(WeekStart(d.created))]; ok {
			out[i].Opened++
		}
		if d.resolvedAt != nil {
			if i, ok := idx[Day(WeekStart(*d.resolvedAt))]; ok {
				out[i].Closed++
			}
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- burn-up

// BurnupPoint is one day of a release burn-up: the requirements in scope
// (existing that day), those with linked tests and those whose linked
// tests all passed in the release's runs.
type BurnupPoint struct {
	Day     string `json:"day"`
	Scope   int    `json:"scope"`
	Covered int    `json:"covered"`
	Passed  int    `json:"passed"`
}

// Burnup is the burn-up of a release from its creation (at most
// MaxBackfillDays) to its release, or today.
type Burnup struct {
	ReleaseID  string        `json:"release_id"`
	Name       string        `json:"name"`
	TargetDate string        `json:"target_date"`
	From       string        `json:"from"`
	To         string        `json:"to"`
	Points     []BurnupPoint `json:"points"`
}

// ReleaseBurnup computes the burn-up of a release from history: the scope
// is the release's current requirements (by creation date), covered and
// passed follow the trace links' creation and the results of the release's
// runs, as in the release status.
func (s *Store) ReleaseBurnup(ctx context.Context, tenantID, id string) (*Burnup, error) {
	rel, err := s.GetRelease(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	c := s.c(ctx)
	scope := map[string]bool{}
	for _, rid := range rel.RequirementIDs {
		scope[rid] = true
	}
	reqs, err := datedRows(ctx, c, `SELECT id, status, created_at FROM requirements WHERE tenant_id = ? AND project_id = ?`, tenantID, rel.ProjectID)
	if err != nil {
		return nil, err
	}
	reqs = slices.DeleteFunc(reqs, func(d dated) bool { return !scope[d.id] })
	links, err := projectLinks(ctx, c, tenantID, rel.ProjectID)
	if err != nil {
		return nil, err
	}
	links = slices.DeleteFunc(links, func(l projectLink) bool { return !scope[l.req] })
	events, err := resultEvents(ctx, c, tenantID, rel.ProjectID, rel.ID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	end := now
	if rel.ReleasedAt != nil && rel.ReleasedAt.Before(now) {
		end = *rel.ReleasedAt
	}
	last, _ := dayStart(Day(end))
	first, _ := dayStart(Day(rel.CreatedAt))
	if limit := last.AddDate(0, 0, -MaxBackfillDays); first.Before(limit) {
		first = limit
	}
	if first.After(last) {
		first = last
	}
	rp := newReplay(&projectHistory{reqs: reqs, links: links, events: events})
	b := &Burnup{ReleaseID: rel.ID, Name: rel.Name, TargetDate: rel.TargetDate, From: Day(first), To: Day(last), Points: []BurnupPoint{}}
	for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
		dayEnd := day.AddDate(0, 0, 1)
		if dayEnd.After(end) {
			dayEnd = end.Add(time.Nanosecond)
		}
		rp.advance(dayEnd)
		r2t := rp.linkIndex(dayEnd)
		p := BurnupPoint{Day: Day(day)}
		for _, q := range reqs {
			if !q.at.Before(dayEnd) {
				continue
			}
			p.Scope++
			if len(r2t[q.id]) > 0 {
				p.Covered++
			}
			if Coverage(r2t[q.id], rp.last) == model.ResultPassed {
				p.Passed++
			}
		}
		b.Points = append(b.Points, p)
	}
	return b, nil
}

// OpenDefectsBySeverity counts a project's open defects per severity
// (every severity present, 0 when none).
func (s *Store) OpenDefectsBySeverity(ctx context.Context, tenantID, projectID string) (map[string]int, error) {
	out := map[string]int{}
	for _, sv := range model.DefectSeverities {
		out[sv] = 0
	}
	rows, err := s.c(ctx).query(ctx, `SELECT severity, COUNT(*) FROM defects WHERE tenant_id = ? AND project_id = ? AND status IN ('open', 'in_progress') GROUP BY severity`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sv string
		var n int
		if err := rows.Scan(&sv, &n); err != nil {
			return nil, err
		}
		out[sv] = n
	}
	return out, rows.Err()
}
