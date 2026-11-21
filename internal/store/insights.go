package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// Dashboard insights: the review state of requirements, suspect links,
// the age of open defects, the risks with their requirements and the
// activity per day. Counting is done in SQL where it is portable; times
// are bucketed in Go (UTC days) so that SQLite and PostgreSQL agree.

// Review states of a requirement on the dashboard: the status of its
// latest review request that was not cancelled, or "none".
const ReviewStateNone = "none"

// ReviewStates in the order of the dashboard.
var ReviewStates = []string{ReviewStateNone, "pending", "changes_requested", "approved"}

// RequirementReviewStates returns, per requirement of a project that has
// review requests, the status of its latest request that was not
// cancelled (pending, approved or changes_requested).
func (s *Store) RequirementReviewStates(ctx context.Context, tenantID, projectID string) (map[string]string, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT entity_id, status, created_at FROM review_requests
		WHERE tenant_id = ? AND project_id = ? AND entity_type = ? AND status <> 'cancelled' ORDER BY created_at, id`, tenantID, projectID, model.KindRequirement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, status string
		var at time.Time
		if err := rows.Scan(&id, &status, &at); err != nil {
			return nil, err
		}
		out[id] = status // ordered by time: the latest wins
	}
	return out, rows.Err()
}

// SuspectCounts counts a project's suspect trace links and the
// requirements that have one.
func (s *Store) SuspectCounts(ctx context.Context, tenantID, projectID string) (links, requirements int, err error) {
	err = s.c(ctx).row(ctx, `SELECT COUNT(*), COUNT(DISTINCT requirement_id) FROM trace_links WHERE tenant_id = ? AND project_id = ? AND suspect_since IS NOT NULL`,
		tenantID, projectID).Scan(&links, &requirements)
	return links, requirements, err
}

// DefectAgeBuckets are the age groups of open defects (days, upper bound
// exclusive; the last group has no bound).
var DefectAgeBuckets = []struct {
	Key  string
	Days int
}{{"lt7", 7}, {"7to30", 30}, {"30to90", 90}, {"gt90", 0}}

// DefectAges is the age of the open defects of a project.
type DefectAges struct {
	Open int `json:"open"`
	// Buckets: open defects per age group (lt7, 7to30, 30to90, gt90).
	Buckets map[string]int `json:"buckets"`
	// BySeverity: per severity, open defects per age group.
	BySeverity map[string]map[string]int `json:"by_severity"`
	// AverageDays and OldestDays: age of the open defects in whole days.
	AverageDays float64 `json:"average_days"`
	OldestDays  int     `json:"oldest_days"`
}

func ageBucket(days int) string {
	for _, b := range DefectAgeBuckets {
		if b.Days == 0 || days < b.Days {
			return b.Key
		}
	}
	return DefectAgeBuckets[len(DefectAgeBuckets)-1].Key
}

// OpenDefectAges groups the open defects of a project by age at now.
func (s *Store) OpenDefectAges(ctx context.Context, tenantID, projectID string, now time.Time) (*DefectAges, error) {
	out := &DefectAges{Buckets: map[string]int{}, BySeverity: map[string]map[string]int{}}
	for _, b := range DefectAgeBuckets {
		out.Buckets[b.Key] = 0
	}
	for _, sv := range model.DefectSeverities {
		out.BySeverity[sv] = map[string]int{}
		for _, b := range DefectAgeBuckets {
			out.BySeverity[sv][b.Key] = 0
		}
	}
	rows, err := s.c(ctx).query(ctx, `SELECT severity, created_at FROM defects WHERE tenant_id = ? AND project_id = ? AND status IN ('open', 'in_progress')`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	total := 0
	for rows.Next() {
		var sv string
		var at time.Time
		if err := rows.Scan(&sv, &at); err != nil {
			return nil, err
		}
		days := max(int(now.Sub(at).Hours()/24), 0)
		b := ageBucket(days)
		out.Open++
		out.Buckets[b]++
		if out.BySeverity[sv] == nil {
			out.BySeverity[sv] = map[string]int{}
		}
		out.BySeverity[sv][b]++
		total += days
		out.OldestDays = max(out.OldestDays, days)
	}
	if out.Open > 0 {
		out.AverageDays = round1(float64(total) / float64(out.Open))
	}
	return out, rows.Err()
}

// RiskPoint is a risk with what the dashboard needs.
type RiskPoint struct {
	ID             string
	Likelihood     int
	Impact         int
	Status         string
	RequirementIDs []string
}

// RiskPoints returns the risks of a project with their requirements.
func (s *Store) RiskPoints(ctx context.Context, tenantID, projectID string) ([]RiskPoint, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id, likelihood, impact, status FROM risks WHERE tenant_id = ? AND project_id = ? ORDER BY seq`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	var out []RiskPoint
	idx := map[string]int{}
	for rows.Next() {
		var k RiskPoint
		if err := rows.Scan(&k.ID, &k.Likelihood, &k.Impact, &k.Status); err != nil {
			rows.Close()
			return nil, err
		}
		idx[k.ID] = len(out)
		out = append(out, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(out) == 0 {
		return out, err
	}
	rows, err = s.c(ctx).query(ctx, `SELECT rr.risk_id, rr.requirement_id FROM risk_requirements rr JOIN risks k ON k.id = rr.risk_id WHERE k.tenant_id = ? AND k.project_id = ?`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rid, qid string
		if err := rows.Scan(&rid, &qid); err != nil {
			return nil, err
		}
		if i, ok := idx[rid]; ok {
			out[i].RequirementIDs = append(out[i].RequirementIDs, qid)
		}
	}
	return out, rows.Err()
}

// ActivityDay is the activity of one day in a project.
type ActivityDay struct {
	Day string `json:"day"`
	// Changes: revisions of requirements and tests (created, edited, restored).
	Changes int `json:"changes"`
	// Executions: results recorded (executions finished that day).
	Executions int `json:"executions"`
	// Defects: defects reported.
	Defects int `json:"defects"`
}

// ActivityDays is the default range of Activity.
const ActivityDays = 30

// Activity counts per day, from the day of from to the day of to (UTC),
// the changes to requirements and tests, the executions finished and the
// defects reported in a project.
func (s *Store) Activity(ctx context.Context, tenantID, projectID string, from, to time.Time) ([]ActivityDay, error) {
	first, _ := dayStart(Day(from))
	last, _ := dayStart(Day(to))
	out := []ActivityDay{}
	idx := map[string]int{}
	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		idx[Day(d)] = len(out)
		out = append(out, ActivityDay{Day: Day(d)})
	}
	end := last.AddDate(0, 0, 1)
	count := func(q string, add func(*ActivityDay)) error {
		rows, err := s.c(ctx).query(ctx, q, tenantID, projectID, first, end)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var at sql.NullTime
			if err := rows.Scan(&at); err != nil {
				return err
			}
			if !at.Valid {
				continue
			}
			if i, ok := idx[Day(at.Time)]; ok {
				add(&out[i])
			}
		}
		return rows.Err()
	}
	if err := count(`SELECT v.created_at FROM revisions v JOIN requirements q ON q.id = v.entity_id
		WHERE v.entity_type = 'requirement' AND v.tenant_id = ? AND q.project_id = ? AND v.created_at >= ? AND v.created_at < ?`, func(d *ActivityDay) { d.Changes++ }); err != nil {
		return nil, err
	}
	if err := count(`SELECT v.created_at FROM revisions v JOIN test_cases t ON t.id = v.entity_id
		WHERE v.entity_type = 'test' AND v.tenant_id = ? AND t.project_id = ? AND v.created_at >= ? AND v.created_at < ?`, func(d *ActivityDay) { d.Changes++ }); err != nil {
		return nil, err
	}
	if err := count(`SELECT e.finished_at FROM test_executions e JOIN test_runs r ON r.id = e.run_id
		WHERE e.tenant_id = ? AND r.project_id = ? AND e.finished_at >= ? AND e.finished_at < ?`, func(d *ActivityDay) { d.Executions++ }); err != nil {
		return nil, err
	}
	if err := count(`SELECT created_at FROM defects WHERE tenant_id = ? AND project_id = ? AND created_at >= ? AND created_at < ?`, func(d *ActivityDay) { d.Defects++ }); err != nil {
		return nil, err
	}
	return out, nil
}
