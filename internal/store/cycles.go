package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// Test cycles (migration 0041): the plan of a cycle (tests per
// configuration with tester and estimate), the runs the cycle creates and
// the progress derived from their executions.

// MaxCycleTests bounds the plan of one cycle.
const MaxCycleTests = 5000

const cycleCols = `c.id, c.project_id, c.name, c.goal, c.description, c.status, c.owner_id, c.release_id, c.variant_id, c.build_id, c.environment, c.configuration_ids,
	c.start_date, c.end_date, c.scheduled_start_at, c.schedule_id, c.version, c.started_at, c.closed_at, c.created_by, c.created_at, c.updated_at,
	COALESCE((SELECT r.name FROM releases r WHERE r.id = c.release_id), ''), COALESCE((SELECT v.name FROM variants v WHERE v.id = c.variant_id), ''),
	COALESCE((SELECT CASE WHEN b.number <> '' THEN b.number ELSE b.external_id END FROM builds b WHERE b.id = c.build_id), ''), c.environment_id`

func scanCycle(r interface{ Scan(...any) error }) (*model.TestCycle, error) {
	var c model.TestCycle
	var rel, variant, build, sched, env sql.NullString
	var start, started, closed sql.NullTime
	var cfgs string
	if err := r.Scan(&c.ID, &c.ProjectID, &c.Name, &c.Goal, &c.Description, &c.Status, &c.OwnerID, &rel, &variant, &build, &c.Environment, &cfgs,
		&c.StartDate, &c.EndDate, &start, &sched, &c.Version, &started, &closed, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
		&c.ReleaseName, &c.VariantName, &c.BuildLabel, &env); err != nil {
		return nil, err
	}
	c.ReleaseID, c.VariantID, c.BuildID, c.ScheduleID = strPtr(rel), strPtr(variant), strPtr(build), strPtr(sched)
	c.EnvironmentID = strPtr(env)
	c.ScheduledStartAt, c.StartedAt, c.ClosedAt = timePtr(start), timePtr(started), timePtr(closed)
	c.ConfigurationIDs = decodeTags(cfgs)
	c.CreatedAt, c.UpdatedAt = c.CreatedAt.UTC(), c.UpdatedAt.UTC()
	c.RunIDs = []string{}
	return &c, nil
}

// validateCycle checks and normalises the editable fields of a cycle.
func validateCycle(c *model.TestCycle) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || len(c.Name) > 200 {
		return Invalidf("cycle name is required (at most 200 characters)")
	}
	c.Goal = strings.TrimSpace(c.Goal)
	if len(c.Goal) > 1000 || len(c.Description) > 20000 || len(c.Environment) > 200 {
		return Invalidf("goal, description or environment is too long")
	}
	if c.Status == "" {
		c.Status = model.CyclePlanned
	}
	if !slices.Contains(model.CycleStatuses, c.Status) {
		return Invalidf("status must be one of %s", strings.Join(model.CycleStatuses, ", "))
	}
	c.StartDate, c.EndDate = strings.TrimSpace(c.StartDate), strings.TrimSpace(c.EndDate)
	for _, d := range []string{c.StartDate, c.EndDate} {
		if d != "" && !dateRe.MatchString(d) {
			return Invalidf("start_date and end_date must be YYYY-MM-DD")
		}
	}
	if c.StartDate != "" && c.EndDate != "" && c.EndDate < c.StartDate {
		return Invalidf("end_date is before start_date")
	}
	if c.ConfigurationIDs == nil {
		c.ConfigurationIDs = []string{}
	}
	c.ConfigurationIDs = uniq(c.ConfigurationIDs)
	return nil
}

// checkMember verifies that a user is a member of the tenant ("" passes).
func checkMember(ctx context.Context, c conn, tenantID, userID, what string) error {
	if userID == "" {
		return nil
	}
	var n int
	if err := c.row(ctx, `SELECT COUNT(*) FROM memberships WHERE tenant_id = ? AND user_id = ?`, tenantID, userID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return Invalidf("the %s is not a member of the organisation", what)
	}
	return nil
}

func (s *Store) checkCycleRefs(ctx context.Context, c conn, tenantID string, cy *model.TestCycle) error {
	if err := checkRelease(ctx, c, tenantID, cy.ProjectID, cy.ReleaseID); err != nil {
		return err
	}
	if err := checkRunVariantBuild(ctx, c, tenantID, cy.ProjectID, cy.VariantID, cy.BuildID); err != nil {
		return err
	}
	if err := checkRunConfigurations(ctx, c, tenantID, cy.ProjectID, cy.ConfigurationIDs); err != nil {
		return err
	}
	var n int
	if err := c.row(ctx, `SELECT COUNT(*) FROM test_cycles WHERE project_id = ? AND LOWER(name) = LOWER(?) AND id <> ?`, cy.ProjectID, cy.Name, cy.ID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return Conflictf("a cycle named %q already exists", cy.Name)
	}
	return checkMember(ctx, c, tenantID, cy.OwnerID, "owner")
}

// CreateCycle adds a test cycle (without tests).
func (s *Store) CreateCycle(ctx context.Context, tenantID string, cy *model.TestCycle) error {
	if err := validateCycle(cy); err != nil {
		return err
	}
	if _, err := s.GetProject(ctx, tenantID, cy.ProjectID); err != nil {
		return err
	}
	cy.ID, cy.Version = newID(), 1
	cy.CreatedAt = s.now()
	cy.UpdatedAt = cy.CreatedAt
	if cy.Status != model.CyclePlanned {
		at := cy.CreatedAt
		cy.StartedAt = &at
		if cy.Status == model.CycleClosed {
			cy.ClosedAt = &at
		}
	}
	return s.tx(ctx, func(c conn) error {
		if err := s.checkCycleRefs(ctx, c, tenantID, cy); err != nil {
			return err
		}
		_, err := c.exec(ctx, `INSERT INTO test_cycles (id, tenant_id, project_id, name, goal, description, status, owner_id, release_id, variant_id, build_id, environment, configuration_ids,
			start_date, end_date, scheduled_start_at, schedule_id, version, started_at, closed_at, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			cy.ID, tenantID, cy.ProjectID, cy.Name, cy.Goal, cy.Description, cy.Status, cy.OwnerID, nullStr(cy.ReleaseID), nullStr(cy.VariantID), nullStr(cy.BuildID), cy.Environment,
			encodeList(cy.ConfigurationIDs), cy.StartDate, cy.EndDate, nullTime(cy.ScheduledStartAt), nullStr(cy.ScheduleID), cy.Version, nullTime(cy.StartedAt), nullTime(cy.ClosedAt),
			cy.CreatedBy, cy.CreatedAt, cy.UpdatedAt)
		return mapErr(err, "cycle name")
	})
}

// UpdateCycle saves the editable fields of a cycle (and a new version);
// status changes record when the cycle started and closed.
func (s *Store) UpdateCycle(ctx context.Context, tenantID string, cy *model.TestCycle) error {
	if err := validateCycle(cy); err != nil {
		return err
	}
	now := s.now()
	if cy.Status != model.CyclePlanned && cy.StartedAt == nil {
		cy.StartedAt = &now
	}
	if cy.Status == model.CyclePlanned {
		cy.StartedAt = nil
	}
	switch {
	case cy.Status == model.CycleClosed && cy.ClosedAt == nil:
		cy.ClosedAt = &now
	case cy.Status != model.CycleClosed:
		cy.ClosedAt = nil
	}
	if cy.Status != model.CyclePlanned {
		cy.ScheduledStartAt = nil // started: nothing left to schedule
	}
	cy.UpdatedAt = now
	return s.tx(ctx, func(c conn) error {
		if err := s.checkCycleRefs(ctx, c, tenantID, cy); err != nil {
			return err
		}
		res, err := c.exec(ctx, `UPDATE test_cycles SET name = ?, goal = ?, description = ?, status = ?, owner_id = ?, release_id = ?, variant_id = ?, build_id = ?, environment = ?,
			configuration_ids = ?, start_date = ?, end_date = ?, scheduled_start_at = ?, version = version + 1, started_at = ?, closed_at = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
			cy.Name, cy.Goal, cy.Description, cy.Status, cy.OwnerID, nullStr(cy.ReleaseID), nullStr(cy.VariantID), nullStr(cy.BuildID), cy.Environment,
			encodeList(cy.ConfigurationIDs), cy.StartDate, cy.EndDate, nullTime(cy.ScheduledStartAt), nullTime(cy.StartedAt), nullTime(cy.ClosedAt), cy.UpdatedAt, cy.ID, tenantID)
		if err != nil {
			return mapErr(err, "cycle name")
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "cycle")
		}
		cy.Version++
		return nil
	})
}

// GetCycle returns a cycle with its runs and progress.
func (s *Store) GetCycle(ctx context.Context, tenantID, id string) (*model.TestCycle, error) {
	cy, err := scanCycle(s.c(ctx).row(ctx, `SELECT `+cycleCols+` FROM test_cycles c WHERE c.id = ? AND c.tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "cycle")
	}
	rows, err := s.c(ctx).query(ctx, `SELECT id FROM test_runs WHERE cycle_id = ? ORDER BY created_at, id`, id)
	if err != nil {
		return nil, err
	}
	if cy.RunIDs, err = scanStrings(rows); err != nil {
		return nil, err
	}
	if cy.RunIDs == nil {
		cy.RunIDs = []string{}
	}
	plan, err := s.CycleTests(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	cy.Progress = CycleProgressOf(plan)
	return cy, nil
}

// ListCycles lists the cycles of a project (open ones first, then by start
// date, newest first) with their progress.
func (s *Store) ListCycles(ctx context.Context, tenantID, projectID string) ([]model.TestCycle, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+cycleCols+` FROM test_cycles c WHERE c.tenant_id = ? AND c.project_id = ?`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	out := []model.TestCycle{}
	for rows.Next() {
		cy, err := scanCycle(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *cy)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	plans, err := s.cyclePlans(ctx, `c.tenant_id = ? AND c.project_id = ?`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	runRows, err := s.c(ctx).query(ctx, `SELECT cycle_id, id FROM test_runs WHERE project_id = ? AND cycle_id IS NOT NULL ORDER BY created_at, id`, projectID)
	if err != nil {
		return nil, err
	}
	runs := map[string][]string{}
	for runRows.Next() {
		var c, r string
		if err := runRows.Scan(&c, &r); err != nil {
			runRows.Close()
			return nil, err
		}
		runs[c] = append(runs[c], r)
	}
	runRows.Close()
	for i := range out {
		out[i].Progress = CycleProgressOf(plans[out[i].ID])
		if r := runs[out[i].ID]; r != nil {
			out[i].RunIDs = r
		}
	}
	rank := map[string]int{model.CycleActive: 0, model.CyclePlanned: 1, model.CycleClosed: 2}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if rank[a.Status] != rank[b.Status] {
			return rank[a.Status] < rank[b.Status]
		}
		if a.StartDate != b.StartDate {
			return a.StartDate > b.StartDate
		}
		return a.CreatedAt.After(b.CreatedAt)
	})
	return out, nil
}

// DeleteCycle removes a cycle and its plan; its runs stay (without cycle).
func (s *Store) DeleteCycle(ctx context.Context, tenantID, id string) error {
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `UPDATE test_runs SET cycle_id = NULL WHERE cycle_id = ? AND tenant_id = ?`, id, tenantID); err != nil {
			return err
		}
		if _, err := c.exec(ctx, `UPDATE test_cycle_schedules SET template_cycle_id = NULL WHERE template_cycle_id = ? AND tenant_id = ?`, id, tenantID); err != nil {
			return err
		}
		if _, err := c.exec(ctx, `DELETE FROM test_cycle_tests WHERE cycle_id = ? AND tenant_id = ?`, id, tenantID); err != nil {
			return err
		}
		res, err := c.exec(ctx, `DELETE FROM test_cycles WHERE id = ? AND tenant_id = ?`, id, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "cycle")
		}
		return nil
	})
}

// ---------------------------------------------------------------- the plan

// cycleExec is an execution of a cycle's run.
type cycleExec struct {
	id, run, test, cfg, status, by string
	finished                       *time.Time
	at                             time.Time
	ms                             int64
}

func execRank(status string) int {
	switch status {
	case model.ResultPassed, model.ResultFailed, model.ResultBlocked, model.ResultSkipped:
		return 2
	case model.ResultInProgress:
		return 1
	}
	return 0
}

// cyclePlans loads the plans of the cycles matching where (on alias c)
// with their latest executions in the cycles' runs, by cycle id.
func (s *Store) cyclePlans(ctx context.Context, where string, args ...any) (map[string][]model.TestCycleTest, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT ct.id, ct.cycle_id, ct.test_case_id, t.key, t.title, ct.configuration_id, COALESCE(cf.name, ''), ct.assignee_id, ct.estimate_minutes,
		ct.position, ct.added_by, ct.added_at
		FROM test_cycle_tests ct JOIN test_cycles c ON c.id = ct.cycle_id JOIN test_cases t ON t.id = ct.test_case_id LEFT JOIN configurations cf ON cf.id = ct.configuration_id
		WHERE `+where+` ORDER BY ct.cycle_id, ct.position, t.seq`, args...)
	if err != nil {
		return nil, err
	}
	out := map[string][]model.TestCycleTest{}
	for rows.Next() {
		var x model.TestCycleTest
		var cfg string
		if err := rows.Scan(&x.ID, &x.CycleID, &x.TestCaseID, &x.TestKey, &x.TestTitle, &cfg, &x.ConfigurationName, &x.AssigneeID, &x.EstimateMinutes,
			&x.Position, &x.AddedBy, &x.AddedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if cfg != "" {
			x.ConfigurationID = &cfg
		}
		x.AddedAt = x.AddedAt.UTC()
		x.Status = model.ResultNotRun
		out[x.CycleID] = append(out[x.CycleID], x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	erows, err := s.c(ctx).query(ctx, `SELECT r.cycle_id, e.id, e.run_id, e.test_case_id, COALESCE(e.configuration_id, ''), e.status, e.executed_by, e.finished_at, e.started_at, e.duration_ms, r.created_at
		FROM test_executions e JOIN test_runs r ON r.id = e.run_id WHERE r.cycle_id IN (SELECT c.id FROM test_cycles c WHERE `+where+`)`, args...)
	if err != nil {
		return nil, err
	}
	latest := map[string]cycleExec{}
	for erows.Next() {
		var cyc string
		var e cycleExec
		var fin, sta sql.NullTime
		if err := erows.Scan(&cyc, &e.id, &e.run, &e.test, &e.cfg, &e.status, &e.by, &fin, &sta, &e.ms, &e.at); err != nil {
			erows.Close()
			return nil, err
		}
		e.finished = timePtr(fin)
		if fin.Valid {
			e.at = fin.Time
		} else if sta.Valid {
			e.at = sta.Time
		}
		k := cyc + "\x00" + e.test + "\x00" + e.cfg
		cur, ok := latest[k]
		if !ok || execRank(e.status) > execRank(cur.status) || (execRank(e.status) == execRank(cur.status) && e.at.After(cur.at)) {
			latest[k] = e
		}
	}
	erows.Close()
	if err := erows.Err(); err != nil {
		return nil, err
	}
	for cid, list := range out {
		for i := range list {
			x := &list[i]
			cfg := ""
			if x.ConfigurationID != nil {
				cfg = *x.ConfigurationID
			}
			if e, ok := latest[cid+"\x00"+x.TestCaseID+"\x00"+cfg]; ok {
				id, run := e.id, e.run
				x.ExecutionID, x.RunID = &id, &run
				x.Status, x.ExecutedBy, x.DurationMS = e.status, e.by, e.ms
				if e.finished != nil {
					f := e.finished.UTC()
					x.FinishedAt = &f
				}
			}
		}
	}
	return out, nil
}

// CycleTests returns the plan of a cycle with the latest result of each
// planned test in the cycle's runs.
func (s *Store) CycleTests(ctx context.Context, tenantID, cycleID string) ([]model.TestCycleTest, error) {
	plans, err := s.cyclePlans(ctx, `c.id = ? AND c.tenant_id = ?`, cycleID, tenantID)
	if err != nil {
		return nil, err
	}
	if plans[cycleID] == nil {
		return []model.TestCycleTest{}, nil
	}
	return plans[cycleID], nil
}

// CycleProgressOf counts a plan.
func CycleProgressOf(plan []model.TestCycleTest) model.CycleProgress {
	var p model.CycleProgress
	for i := range plan {
		x := &plan[i]
		p.Planned++
		p.EstimatedMin += x.EstimateMinutes
		if x.AssigneeID == "" {
			p.Unassigned++
		}
		if x.ExecutionID == nil {
			p.NotInRun++
		}
		switch x.Status {
		case model.ResultPassed:
			p.Passed++
		case model.ResultFailed:
			p.Failed++
		case model.ResultBlocked:
			p.Blocked++
		case model.ResultSkipped:
			p.Skipped++
		case model.ResultInProgress:
			p.InProgress++
		default:
			p.NotRun++
		}
		if x.Executed() {
			p.Executed++
			p.ActualMS += x.DurationMS
			p.EstimatedDone += x.EstimateMinutes
		} else {
			p.RemainingMin += x.EstimateMinutes
		}
	}
	if p.Planned > 0 {
		p.PercentDone = round1(float64(p.Executed) * 100 / float64(p.Planned))
		p.PercentPassed = round1(float64(p.Passed) * 100 / float64(p.Planned))
	}
	return p
}

// CycleTestInput is a test to add to a plan: in one configuration
// (ConfigurationID; nil: every configuration of the cycle), with a tester
// and an estimate (nil: the test's estimate, else the average duration of
// its past executions).
type CycleTestInput struct {
	TestID          string  `json:"test_id"`
	ConfigurationID *string `json:"configuration_id"`
	AssigneeID      string  `json:"assignee_id"`
	EstimateMinutes *int    `json:"estimate_minutes"`
}

// DefaultEstimates returns the estimated minutes of tests: the test's
// estimate, else the average duration of its executions with a final
// result (rounded up to whole minutes), else 0.
func (s *Store) DefaultEstimates(ctx context.Context, tenantID string, testIDs []string) (map[string]int, error) {
	out := map[string]int{}
	ids := uniq(testIDs)
	for start := 0; start < len(ids); start += 500 {
		chunk := ids[start:min(start+500, len(ids))]
		args := append([]any{tenantID}, anySlice(chunk)...)
		rows, err := s.c(ctx).query(ctx, `SELECT t.id, t.estimated_min, COALESCE((SELECT AVG(e.duration_ms) FROM test_executions e WHERE e.test_case_id = t.id AND e.duration_ms > 0
			AND e.status IN ('passed', 'failed', 'blocked', 'skipped')), 0) FROM test_cases t WHERE t.tenant_id = ? AND t.id IN (`+placeholders(len(chunk))+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			var est int
			var avg float64
			if err := rows.Scan(&id, &est, &avg); err != nil {
				rows.Close()
				return nil, err
			}
			if est <= 0 && avg > 0 {
				est = int(math.Ceil(avg / 60000))
			}
			out[id] = est
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// AddCycleTests adds tests to the plan of a cycle; pairs of test and
// configuration already planned are skipped. Returns the number of rows
// added.
func (s *Store) AddCycleTests(ctx context.Context, tenantID, cycleID, user string, in []CycleTestInput) (int, error) {
	cy, err := scanCycle(s.c(ctx).row(ctx, `SELECT `+cycleCols+` FROM test_cycles c WHERE c.id = ? AND c.tenant_id = ?`, cycleID, tenantID))
	if err != nil {
		return 0, mapErr(err, "cycle")
	}
	if cy.Status == model.CycleClosed {
		return 0, Conflictf("the cycle is closed")
	}
	var ids []string
	for _, x := range in {
		ids = append(ids, x.TestID)
	}
	est, err := s.DefaultEstimates(ctx, tenantID, ids)
	if err != nil {
		return 0, err
	}
	added := 0
	err = s.tx(ctx, func(c conn) error {
		var n int
		var pos sql.NullInt64
		if err := c.row(ctx, `SELECT COUNT(*), MAX(position) FROM test_cycle_tests WHERE cycle_id = ?`, cycleID).Scan(&n, &pos); err != nil {
			return err
		}
		next := 0
		if pos.Valid {
			next = int(pos.Int64) + 1
		}
		cfgOK := map[string]bool{"": true}
		for _, id := range cy.ConfigurationIDs {
			cfgOK[id] = true
		}
		members := map[string]bool{"": true}
		now := s.now()
		for _, x := range in {
			var pid string
			if err := c.row(ctx, `SELECT project_id FROM test_cases WHERE id = ? AND tenant_id = ?`, x.TestID, tenantID).Scan(&pid); err != nil {
				return mapErr(err, "test")
			}
			if pid != cy.ProjectID {
				return Invalidf("test %s belongs to another project", x.TestID)
			}
			if !members[x.AssigneeID] {
				if err := checkMember(ctx, c, tenantID, x.AssigneeID, "tester"); err != nil {
					return err
				}
				members[x.AssigneeID] = true
			}
			e := est[x.TestID]
			if x.EstimateMinutes != nil {
				if *x.EstimateMinutes < 0 || *x.EstimateMinutes > 100000 {
					return Invalidf("estimate_minutes must be between 0 and 100000")
				}
				e = *x.EstimateMinutes
			}
			cfgs := []string{""}
			switch {
			case x.ConfigurationID != nil:
				if !cfgOK[*x.ConfigurationID] {
					return Invalidf("configuration %s is not a configuration of the cycle", *x.ConfigurationID)
				}
				cfgs = []string{*x.ConfigurationID}
			case len(cy.ConfigurationIDs) > 0:
				cfgs = cy.ConfigurationIDs
			}
			for _, cfg := range cfgs {
				var exists int
				if err := c.row(ctx, `SELECT COUNT(*) FROM test_cycle_tests WHERE cycle_id = ? AND test_case_id = ? AND configuration_id = ?`, cycleID, x.TestID, cfg).Scan(&exists); err != nil {
					return err
				}
				if exists > 0 {
					continue
				}
				if n+added >= MaxCycleTests {
					return Invalidf("a cycle can plan at most %d tests", MaxCycleTests)
				}
				if _, err := c.exec(ctx, `INSERT INTO test_cycle_tests (id, cycle_id, tenant_id, test_case_id, configuration_id, assignee_id, estimate_minutes, position, added_by, added_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					newID(), cycleID, tenantID, x.TestID, cfg, x.AssigneeID, e, next, user, now); err != nil {
					return err
				}
				next++
				added++
			}
		}
		return nil
	})
	return added, err
}

// CopyCyclePlan adds the plan of cycle from (tests, configurations,
// testers and estimates) to cycle to. Rows in configurations that to does
// not plan are added without configuration when to has none, else skipped.
func (s *Store) CopyCyclePlan(ctx context.Context, tenantID, from, to, user string) (int, error) {
	src, err := s.GetCycle(ctx, tenantID, from)
	if err != nil {
		return 0, err
	}
	dst, err := s.GetCycle(ctx, tenantID, to)
	if err != nil {
		return 0, err
	}
	if src.ProjectID != dst.ProjectID {
		return 0, Invalidf("the cycle to copy belongs to another project")
	}
	plan, err := s.CycleTests(ctx, tenantID, from)
	if err != nil {
		return 0, err
	}
	in := make([]CycleTestInput, 0, len(plan))
	for _, x := range plan {
		est := x.EstimateMinutes
		ci := CycleTestInput{TestID: x.TestCaseID, AssigneeID: x.AssigneeID, EstimateMinutes: &est}
		switch {
		case x.ConfigurationID != nil && slices.Contains(dst.ConfigurationIDs, *x.ConfigurationID):
			ci.ConfigurationID = x.ConfigurationID
		case len(dst.ConfigurationIDs) == 0:
			none := ""
			ci.ConfigurationID = &none
		case x.ConfigurationID != nil:
			continue
		}
		in = append(in, ci)
	}
	return s.AddCycleTests(ctx, tenantID, to, user, in)
}

// CycleTestUpdate changes planned rows: the tester (user id, "" nobody)
// and / or the estimate.
type CycleTestUpdate struct {
	AssigneeID      *string `json:"assignee_id"`
	EstimateMinutes *int    `json:"estimate_minutes"`
}

// UpdateCycleTests changes rows of a plan (ids: row ids; all of them when
// nil). A new tester is also set on the executions of the rows in the
// cycle's runs that have no final result yet; assigneeEmail is what the
// executions show (the tester's e-mail). Returns the rows changed.
func (s *Store) UpdateCycleTests(ctx context.Context, tenantID, cycleID string, ids []string, u CycleTestUpdate, assigneeEmail string) (int, error) {
	if u.EstimateMinutes != nil && (*u.EstimateMinutes < 0 || *u.EstimateMinutes > 100000) {
		return 0, Invalidf("estimate_minutes must be between 0 and 100000")
	}
	if u.AssigneeID == nil && u.EstimateMinutes == nil {
		return 0, Invalidf("nothing to change: set assignee_id or estimate_minutes")
	}
	plan, err := s.CycleTests(ctx, tenantID, cycleID)
	if err != nil {
		return 0, err
	}
	if _, err := s.GetCycle(ctx, tenantID, cycleID); err != nil {
		return 0, err
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	n := 0
	err = s.tx(ctx, func(c conn) error {
		if u.AssigneeID != nil {
			if err := checkMember(ctx, c, tenantID, *u.AssigneeID, "tester"); err != nil {
				return err
			}
		}
		for _, x := range plan {
			if ids != nil && !want[x.ID] {
				continue
			}
			if u.AssigneeID != nil {
				if _, err := c.exec(ctx, `UPDATE test_cycle_tests SET assignee_id = ? WHERE id = ?`, *u.AssigneeID, x.ID); err != nil {
					return err
				}
				if x.ExecutionID != nil && !x.Executed() {
					if _, err := c.exec(ctx, `UPDATE test_executions SET assignee = ? WHERE id = ?`, assigneeEmail, *x.ExecutionID); err != nil {
						return err
					}
				}
			}
			if u.EstimateMinutes != nil {
				if _, err := c.exec(ctx, `UPDATE test_cycle_tests SET estimate_minutes = ? WHERE id = ?`, *u.EstimateMinutes, x.ID); err != nil {
					return err
				}
			}
			n++
		}
		return nil
	})
	return n, err
}

// RemoveCycleTests removes rows from a plan (their executions stay in the
// runs). Returns the rows removed.
func (s *Store) RemoveCycleTests(ctx context.Context, tenantID, cycleID string, ids []string) (int, error) {
	if _, err := s.GetCycle(ctx, tenantID, cycleID); err != nil {
		return 0, err
	}
	n := 0
	err := s.tx(ctx, func(c conn) error {
		for _, id := range uniq(ids) {
			res, err := c.exec(ctx, `DELETE FROM test_cycle_tests WHERE id = ? AND cycle_id = ? AND tenant_id = ?`, id, cycleID, tenantID)
			if err != nil {
				return err
			}
			k, _ := res.RowsAffected()
			n += int(k)
		}
		return nil
	})
	return n, err
}

// CreateCycleRun creates a run of the cycle with the planned tests that no
// run of the cycle contains yet (in their configurations, assigned to
// their testers: emails maps user ids to the e-mails executions show).
// Returns nil when every planned test is already in a run.
func (s *Store) CreateCycleRun(ctx context.Context, tenantID, cycleID, user string, emails map[string]string) (*model.TestRun, error) {
	cy, err := s.GetCycle(ctx, tenantID, cycleID)
	if err != nil {
		return nil, err
	}
	plan, err := s.CycleTests(ctx, tenantID, cycleID)
	if err != nil {
		return nil, err
	}
	var todo []model.TestCycleTest
	for _, x := range plan {
		if x.ExecutionID == nil {
			todo = append(todo, x)
		}
	}
	if len(todo) == 0 {
		return nil, nil
	}
	name := cy.Name
	if len(cy.RunIDs) > 0 {
		name = cy.Name + " (" + strconv.Itoa(len(cy.RunIDs)+1) + ")"
	}
	var cfgs []string
	var tests []string
	want := map[string]model.TestCycleTest{}
	for _, x := range todo {
		cfg := ""
		if x.ConfigurationID != nil {
			cfg = *x.ConfigurationID
			if !slices.Contains(cfgs, cfg) {
				cfgs = append(cfgs, cfg)
			}
		}
		tests = append(tests, x.TestCaseID)
		want[x.TestCaseID+"\x00"+cfg] = x
	}
	id := cy.ID
	run := &model.TestRun{ProjectID: cy.ProjectID, Name: name, Description: cy.Goal, Environment: cy.Environment, EnvironmentID: cy.EnvironmentID, CreatedBy: user,
		ReleaseID: cy.ReleaseID, VariantID: cy.VariantID, BuildID: cy.BuildID, CycleID: &id}
	if cy.BuildLabel != "" {
		run.Build = cy.BuildLabel
	}
	for _, c := range cfgs {
		run.Configurations = append(run.Configurations, model.ConfigurationRef{ID: c})
	}
	err = s.InTx(ctx, func(ctx context.Context) error {
		if err := s.CreateRun(ctx, tenantID, run, tests); err != nil {
			return err
		}
		c := s.c(ctx)
		rows, err := c.query(ctx, `SELECT id, test_case_id, COALESCE(configuration_id, '') FROM test_executions WHERE run_id = ?`, run.ID)
		if err != nil {
			return err
		}
		type ex struct{ id, test, cfg string }
		var list []ex
		for rows.Next() {
			var e ex
			if err := rows.Scan(&e.id, &e.test, &e.cfg); err != nil {
				rows.Close()
				return err
			}
			list = append(list, e)
		}
		rows.Close()
		var extra []string
		for _, e := range list {
			x, ok := want[e.test+"\x00"+e.cfg]
			if !ok {
				// a configuration the test is not planned in
				extra = append(extra, e.id)
				continue
			}
			if x.AssigneeID != "" && emails[x.AssigneeID] != "" {
				if _, err := c.exec(ctx, `UPDATE test_executions SET assignee = ? WHERE id = ?`, emails[x.AssigneeID], e.id); err != nil {
					return err
				}
			}
		}
		if len(extra) > 0 {
			if _, err := s.DeleteExecutions(ctx, tenantID, run.ID, extra); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetRun(ctx, tenantID, run.ID)
}

// ---------------------------------------------------------------- progress

// CycleReport returns the progress of a cycle: totals, workload per tester
// (most planned first, unassigned last) and the burndown.
func (s *Store) CycleReport(ctx context.Context, tenantID, id string) (*model.CycleReport, error) {
	cy, err := s.GetCycle(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	plan, err := s.CycleTests(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return &model.CycleReport{Cycle: cy, Testers: CycleTesters(plan), Burndown: CycleBurndown(cy, plan, s.now())}, nil
}

// CycleTesters is the workload per tester of a plan.
func CycleTesters(plan []model.TestCycleTest) []model.CycleTesterLoad {
	by := map[string]*model.CycleTesterLoad{}
	var order []string
	for i := range plan {
		x := &plan[i]
		l := by[x.AssigneeID]
		if l == nil {
			l = &model.CycleTesterLoad{AssigneeID: x.AssigneeID}
			by[x.AssigneeID] = l
			order = append(order, x.AssigneeID)
		}
		l.Planned++
		l.EstimatedMin += x.EstimateMinutes
		switch x.Status {
		case model.ResultPassed:
			l.Passed++
		case model.ResultFailed:
			l.Failed++
		case model.ResultBlocked:
			l.Blocked++
		}
		if x.Executed() {
			l.Executed++
			l.ActualMS += x.DurationMS
		} else {
			l.RemainingMin += x.EstimateMinutes
		}
	}
	out := make([]model.CycleTesterLoad, 0, len(order))
	for _, k := range order {
		out = append(out, *by[k])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].AssigneeID == "") != (out[j].AssigneeID == "") {
			return out[j].AssigneeID == ""
		}
		return out[i].Planned > out[j].Planned
	})
	return out
}

// CycleBurndown computes the burndown of a cycle from its start date (else
// the day it was created) to its end date (else today, at most
// MaxBackfillDays): per day the planned tests and estimated minutes not
// executed at the end of the day (nil after today) and the ideal line,
// from the whole plan on the first day to 0 on the last.
func CycleBurndown(cy *model.TestCycle, plan []model.TestCycleTest, now time.Time) []model.CycleBurndownPoint {
	out := []model.CycleBurndownPoint{}
	first, err := dayStart(cy.StartDate)
	if err != nil {
		first, _ = dayStart(Day(cy.CreatedAt))
	}
	today, _ := dayStart(Day(now))
	last, err := dayStart(cy.EndDate)
	if err != nil {
		last = today
		if first.After(last) {
			last = first
		}
	}
	if limit := first.AddDate(0, 0, MaxBackfillDays); last.After(limit) {
		last = limit
	}
	total, totalMin := 0, 0
	for _, x := range plan {
		total++
		totalMin += x.EstimateMinutes
	}
	days := int(last.Sub(first).Hours()/24) + 1
	for i := 0; i < days; i++ {
		day := first.AddDate(0, 0, i)
		frac := 1.0
		if days > 1 {
			frac = 1 - float64(i)/float64(days-1)
		}
		p := model.CycleBurndownPoint{Day: Day(day), Ideal: round1(float64(total) * frac), IdealMin: round1(float64(totalMin) * frac)}
		if !day.After(today) {
			end := day.AddDate(0, 0, 1)
			rem, remMin := 0, 0
			for _, x := range plan {
				if !x.AddedAt.Before(end) {
					continue // planned later
				}
				if x.Executed() && ((x.FinishedAt == nil && !day.Before(today)) || (x.FinishedAt != nil && x.FinishedAt.Before(end))) {
					continue
				}
				rem++
				remMin += x.EstimateMinutes
			}
			p.Remaining, p.RemainingMin = &rem, &remMin
		}
		out = append(out, p)
	}
	return out
}

// ---------------------------------------------------------------- my work

// AssignedCycleTests lists the planned tests assigned to a user that have
// no final result yet, in the open (planned or active) cycles of the
// tenant: for the user's work list (active cycles first, then by end date).
// The caller filters the projects the user may see.
func (s *Store) AssignedCycleTests(ctx context.Context, tenantID, userID string) ([]model.AssignedCycleTest, error) {
	out := []model.AssignedCycleTest{}
	if userID == "" {
		return out, nil
	}
	rows, err := s.c(ctx).query(ctx, `SELECT DISTINCT c.id, c.project_id, p.key, c.name, c.status, c.end_date FROM test_cycles c JOIN projects p ON p.id = c.project_id
		JOIN test_cycle_tests ct ON ct.cycle_id = c.id WHERE c.tenant_id = ? AND c.status IN ('planned', 'active') AND ct.assignee_id = ?`, tenantID, userID)
	if err != nil {
		return nil, err
	}
	type cyc struct{ id, project, key, name, status, end string }
	var cycles []cyc
	for rows.Next() {
		var c cyc
		if err := rows.Scan(&c.id, &c.project, &c.key, &c.name, &c.status, &c.end); err != nil {
			rows.Close()
			return nil, err
		}
		cycles = append(cycles, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, c := range cycles {
		plan, err := s.CycleTests(ctx, tenantID, c.id)
		if err != nil {
			return nil, err
		}
		for _, x := range plan {
			if x.AssigneeID != userID || x.Executed() {
				continue
			}
			out = append(out, model.AssignedCycleTest{TestCycleTest: x, ProjectID: c.project, ProjectKey: c.key, CycleName: c.name, CycleState: c.status, EndDate: c.end})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.CycleState != b.CycleState {
			return a.CycleState == model.CycleActive
		}
		if a.EndDate != b.EndDate {
			return a.EndDate != "" && (b.EndDate == "" || a.EndDate < b.EndDate)
		}
		if a.CycleID != b.CycleID {
			return a.CycleName < b.CycleName
		}
		return a.Position < b.Position
	})
	return out, nil
}

// ---------------------------------------------------------------- queries

// CycleNamesByTest returns, per test of a project, the names of the cycles
// that plan it (for the query term cycle:).
func (s *Store) CycleNamesByTest(ctx context.Context, tenantID, projectID string) (map[string][]string, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT DISTINCT ct.test_case_id, c.name FROM test_cycle_tests ct JOIN test_cycles c ON c.id = ct.cycle_id WHERE c.tenant_id = ? AND c.project_id = ?`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var t, n string
		if err := rows.Scan(&t, &n); err != nil {
			return nil, err
		}
		out[t] = append(out[t], n)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- one-off starts

// DueCycle is a planned cycle whose scheduled start has come.
type DueCycle struct {
	TenantID string
	Cycle    model.TestCycle
}

// DueCycleStarts returns the planned cycles whose scheduled start is at or
// before now.
func (s *Store) DueCycleStarts(ctx context.Context, now time.Time) ([]DueCycle, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+cycleCols+`, c.tenant_id FROM test_cycles c WHERE c.status = 'planned' AND c.scheduled_start_at IS NOT NULL AND c.scheduled_start_at <= ? ORDER BY c.scheduled_start_at`, now.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DueCycle
	for rows.Next() {
		var d DueCycle
		cy, err := scanCycle(scanAppend{rows, &d.TenantID})
		if err != nil {
			return nil, err
		}
		d.Cycle = *cy
		out = append(out, d)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- schedules

const cycleScheduleCols = `s.id, s.project_id, s.name_pattern, s.template_cycle_id, s.query, s.owner_id, s.frequency, s.weekday, s.day_of_month, s.time_of_day, s.timezone,
	s.duration_days, s.auto_start, s.enabled, s.runs, s.next_run_at, s.last_run_at, s.last_cycle_id, s.last_error, s.created_by, s.created_at, s.updated_at,
	COALESCE((SELECT c.name FROM test_cycles c WHERE c.id = s.template_cycle_id), '')`

func scanCycleSchedule(r interface{ Scan(...any) error }) (*model.CycleSchedule, error) {
	var x model.CycleSchedule
	var tpl, last sql.NullString
	var next, lastRun sql.NullTime
	if err := r.Scan(&x.ID, &x.ProjectID, &x.NamePattern, &tpl, &x.Query, &x.OwnerID, &x.Frequency, &x.Weekday, &x.DayOfMonth, &x.TimeOfDay, &x.Timezone,
		&x.DurationDays, &x.AutoStart, &x.Enabled, &x.Runs, &next, &lastRun, &last, &x.LastError, &x.CreatedBy, &x.CreatedAt, &x.UpdatedAt, &x.TemplateName); err != nil {
		return nil, err
	}
	x.TemplateCycleID, x.LastCycleID = strPtr(tpl), strPtr(last)
	x.NextRunAt, x.LastRunAt = timePtr(next), timePtr(lastRun)
	x.CreatedAt, x.UpdatedAt = x.CreatedAt.UTC(), x.UpdatedAt.UTC()
	return &x, nil
}

func (s *Store) validateCycleSchedule(ctx context.Context, c conn, tenantID string, x *model.CycleSchedule) error {
	x.NamePattern = strings.TrimSpace(x.NamePattern)
	if x.NamePattern == "" || len(x.NamePattern) > 180 {
		return Invalidf("name_pattern is required (at most 180 characters)")
	}
	if !slices.Contains(model.Frequencies, x.Frequency) {
		return Invalidf("frequency must be daily, weekly or monthly")
	}
	if x.Timezone == "" {
		x.Timezone = "UTC"
	}
	if x.DurationDays == 0 {
		x.DurationDays = 1
	}
	if x.DurationDays < 1 || x.DurationDays > 366 {
		return Invalidf("duration_days must be between 1 and 366")
	}
	x.Query = strings.TrimSpace(x.Query)
	if len(x.Query) > 2000 {
		return Invalidf("query is too long")
	}
	if (x.TemplateCycleID == nil || *x.TemplateCycleID == "") && x.Query == "" {
		return Invalidf("choose a template cycle or a query")
	}
	if x.TemplateCycleID != nil && *x.TemplateCycleID != "" {
		var pid string
		if err := c.row(ctx, `SELECT project_id FROM test_cycles WHERE id = ? AND tenant_id = ?`, *x.TemplateCycleID, tenantID).Scan(&pid); err != nil {
			return mapErr(err, "template cycle")
		}
		if pid != x.ProjectID {
			return Invalidf("the template cycle belongs to another project")
		}
	}
	if x.OwnerID == "" {
		return Invalidf("owner_id is required")
	}
	if err := checkMember(ctx, c, tenantID, x.OwnerID, "owner"); err != nil {
		return err
	}
	next, err := x.Schedule.NextRun(s.now())
	if err != nil {
		return Invalidf("%v", err)
	}
	if x.Enabled {
		x.NextRunAt = &next
	} else {
		x.NextRunAt = nil
	}
	return nil
}

// CreateCycleSchedule adds a recurring cycle schedule.
func (s *Store) CreateCycleSchedule(ctx context.Context, tenantID string, x *model.CycleSchedule) error {
	if _, err := s.GetProject(ctx, tenantID, x.ProjectID); err != nil {
		return err
	}
	return s.tx(ctx, func(c conn) error {
		if err := s.validateCycleSchedule(ctx, c, tenantID, x); err != nil {
			return err
		}
		x.ID, x.CreatedAt = newID(), s.now()
		x.UpdatedAt = x.CreatedAt
		_, err := c.exec(ctx, `INSERT INTO test_cycle_schedules (id, tenant_id, project_id, name_pattern, template_cycle_id, query, owner_id, frequency, weekday, day_of_month, time_of_day, timezone,
			duration_days, auto_start, enabled, runs, next_run_at, last_error, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, '', ?, ?, ?)`,
			x.ID, tenantID, x.ProjectID, x.NamePattern, nullStr(x.TemplateCycleID), x.Query, x.OwnerID, x.Frequency, x.Weekday, x.DayOfMonth, x.TimeOfDay, x.Timezone,
			x.DurationDays, x.AutoStart, x.Enabled, nullTime(x.NextRunAt), x.CreatedBy, x.CreatedAt, x.UpdatedAt)
		return err
	})
}

// GetCycleSchedule returns a cycle schedule.
func (s *Store) GetCycleSchedule(ctx context.Context, tenantID, id string) (*model.CycleSchedule, error) {
	x, err := scanCycleSchedule(s.c(ctx).row(ctx, `SELECT `+cycleScheduleCols+` FROM test_cycle_schedules s WHERE s.id = ? AND s.tenant_id = ?`, id, tenantID))
	return x, mapErr(err, "cycle schedule")
}

// ListCycleSchedules lists the cycle schedules of a project.
func (s *Store) ListCycleSchedules(ctx context.Context, tenantID, projectID string) ([]model.CycleSchedule, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+cycleScheduleCols+` FROM test_cycle_schedules s WHERE s.tenant_id = ? AND s.project_id = ? ORDER BY s.created_at`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.CycleSchedule{}
	for rows.Next() {
		x, err := scanCycleSchedule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

// UpdateCycleSchedule saves a cycle schedule (its next run is computed
// again from now).
func (s *Store) UpdateCycleSchedule(ctx context.Context, tenantID string, x *model.CycleSchedule) error {
	return s.tx(ctx, func(c conn) error {
		if err := s.validateCycleSchedule(ctx, c, tenantID, x); err != nil {
			return err
		}
		x.UpdatedAt = s.now()
		res, err := c.exec(ctx, `UPDATE test_cycle_schedules SET name_pattern = ?, template_cycle_id = ?, query = ?, owner_id = ?, frequency = ?, weekday = ?, day_of_month = ?, time_of_day = ?, timezone = ?,
			duration_days = ?, auto_start = ?, enabled = ?, next_run_at = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`,
			x.NamePattern, nullStr(x.TemplateCycleID), x.Query, x.OwnerID, x.Frequency, x.Weekday, x.DayOfMonth, x.TimeOfDay, x.Timezone,
			x.DurationDays, x.AutoStart, x.Enabled, nullTime(x.NextRunAt), x.UpdatedAt, x.ID, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "cycle schedule")
		}
		return nil
	})
}

// DeleteCycleSchedule removes a cycle schedule (the cycles it created stay).
func (s *Store) DeleteCycleSchedule(ctx context.Context, tenantID, id string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM test_cycle_schedules WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "cycle schedule")
	}
	return nil
}

// DueCycleSchedule is a schedule whose next run has come.
type DueCycleSchedule struct {
	TenantID string
	Schedule model.CycleSchedule
}

// DueCycleSchedules returns the enabled schedules due at now.
func (s *Store) DueCycleSchedules(ctx context.Context, now time.Time) ([]DueCycleSchedule, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+cycleScheduleCols+`, s.tenant_id FROM test_cycle_schedules s WHERE s.enabled = ? AND s.next_run_at IS NOT NULL AND s.next_run_at <= ? ORDER BY s.next_run_at`, true, now.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DueCycleSchedule
	for rows.Next() {
		var d DueCycleSchedule
		x, err := scanCycleSchedule(scanAppend{rows, &d.TenantID})
		if err != nil {
			return nil, err
		}
		d.Schedule = *x
		out = append(out, d)
	}
	return out, rows.Err()
}

// RecordCycleScheduleRun records a run of a schedule: when, the cycle it
// created (nil on error), the error and the next run.
func (s *Store) RecordCycleScheduleRun(ctx context.Context, id string, at time.Time, cycleID *string, errMsg string, next *time.Time) error {
	q := `UPDATE test_cycle_schedules SET last_run_at = ?, last_error = ?, next_run_at = ?`
	args := []any{at.UTC(), truncate(errMsg, 1000), nullTime(next)}
	if cycleID != nil {
		q += `, last_cycle_id = ?, runs = runs + 1`
		args = append(args, *cycleID)
	}
	_, err := s.c(ctx).exec(ctx, q+` WHERE id = ?`, append(args, id)...)
	return err
}

// SnapshotCycle returns the JSON snapshot stored for a cycle's history:
// its fields and its plan (test key, configuration, tester, estimate).
func SnapshotCycle(cy *model.TestCycle, plan []model.TestCycleTest) json.RawMessage {
	type row struct {
		Test          string `json:"test"`
		Configuration string `json:"configuration,omitempty"`
		Assignee      string `json:"assignee_id,omitempty"`
		Estimate      int    `json:"estimate_minutes"`
	}
	tests := make([]row, 0, len(plan))
	for _, x := range plan {
		tests = append(tests, row{Test: x.TestKey, Configuration: x.ConfigurationName, Assignee: x.AssigneeID, Estimate: x.EstimateMinutes})
	}
	snap := struct {
		Name             string     `json:"name"`
		Goal             string     `json:"goal"`
		Description      string     `json:"description"`
		Status           string     `json:"status"`
		OwnerID          string     `json:"owner_id"`
		ReleaseID        *string    `json:"release_id"`
		VariantID        *string    `json:"variant_id"`
		BuildID          *string    `json:"build_id"`
		Environment      string     `json:"environment"`
		ConfigurationIDs []string   `json:"configuration_ids"`
		StartDate        string     `json:"start_date"`
		EndDate          string     `json:"end_date"`
		ScheduledStartAt *time.Time `json:"scheduled_start_at"`
		Tests            []row      `json:"tests"`
		Version          int        `json:"version"`
	}{cy.Name, cy.Goal, cy.Description, cy.Status, cy.OwnerID, cy.ReleaseID, cy.VariantID, cy.BuildID, cy.Environment, cy.ConfigurationIDs,
		cy.StartDate, cy.EndDate, cy.ScheduledStartAt, tests, cy.Version}
	b, _ := json.Marshal(snap)
	return b
}
