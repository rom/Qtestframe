package store

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

const runCols = `id, project_id, name, description, status, environment, build, created_by, created_at, started_at, completed_at`

func scanRun(r interface{ Scan(...any) error }) (*model.TestRun, error) {
	var run model.TestRun
	var st, ct sql.NullTime
	if err := r.Scan(&run.ID, &run.ProjectID, &run.Name, &run.Description, &run.Status, &run.Environment, &run.Build, &run.CreatedBy, &run.CreatedAt, &st, &ct); err != nil {
		return nil, err
	}
	run.StartedAt, run.CompletedAt = timePtr(st), timePtr(ct)
	return &run, nil
}

// CreateRun creates a run containing the given tests (in order).
func (s *Store) CreateRun(ctx context.Context, tenantID string, run *model.TestRun, testIDs []string) error {
	run.Name = strings.TrimSpace(run.Name)
	if run.Name == "" {
		return Invalidf("run name is required")
	}
	if run.Status == "" {
		run.Status = "planned"
	}
	if !model.Valid(model.RunStatuses, run.Status) {
		return Invalidf("invalid run status %q", run.Status)
	}
	if _, err := s.GetProject(ctx, tenantID, run.ProjectID); err != nil {
		return err
	}
	run.ID = newID()
	run.CreatedAt = s.now()
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `INSERT INTO test_runs (id, tenant_id, project_id, name, description, status, environment, build, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			run.ID, tenantID, run.ProjectID, run.Name, run.Description, run.Status, run.Environment, run.Build, run.CreatedBy, run.CreatedAt); err != nil {
			return err
		}
		_, err := s.addExecutions(ctx, c, tenantID, run.ProjectID, run.ID, testIDs)
		return err
	})
}

func (s *Store) addExecutions(ctx context.Context, c conn, tenantID, projectID, runID string, testIDs []string) (int, error) {
	var pos sql.NullInt64
	if err := c.row(ctx, `SELECT MAX(position) FROM test_executions WHERE run_id = ?`, runID).Scan(&pos); err != nil {
		return 0, err
	}
	next := 0
	if pos.Valid {
		next = int(pos.Int64) + 1
	}
	added := 0
	for _, tid := range uniq(testIDs) {
		var exists int
		if err := c.row(ctx, `SELECT COUNT(*) FROM test_cases WHERE id = ? AND tenant_id = ? AND project_id = ?`, tid, tenantID, projectID).Scan(&exists); err != nil {
			return added, err
		}
		if exists == 0 {
			return added, Invalidf("test %s not found in project", tid)
		}
		if err := c.row(ctx, `SELECT COUNT(*) FROM test_executions WHERE run_id = ? AND test_case_id = ?`, runID, tid).Scan(&exists); err != nil {
			return added, err
		}
		if exists > 0 {
			continue
		}
		eid := newID()
		if _, err := c.exec(ctx, `INSERT INTO test_executions (id, tenant_id, run_id, test_case_id, status, position) VALUES (?, ?, ?, ?, 'not_run', ?)`, eid, tenantID, runID, tid, next); err != nil {
			return added, err
		}
		// snapshot the steps
		rows, err := c.query(ctx, `SELECT id, position, action, expected FROM test_steps WHERE test_case_id = ? ORDER BY position`, tid)
		if err != nil {
			return added, err
		}
		var steps []model.TestStep
		for rows.Next() {
			var st model.TestStep
			if err := rows.Scan(&st.ID, &st.Position, &st.Action, &st.Expected); err != nil {
				rows.Close()
				return added, err
			}
			steps = append(steps, st)
		}
		rows.Close()
		for _, st := range steps {
			if _, err := c.exec(ctx, `INSERT INTO step_results (id, execution_id, step_id, position, action, expected, status, actual) VALUES (?, ?, ?, ?, ?, ?, 'not_run', '')`,
				newID(), eid, st.ID, st.Position, st.Action, st.Expected); err != nil {
				return added, err
			}
		}
		next++
		added++
	}
	return added, nil
}

// AddExecutions appends tests to an existing run.
func (s *Store) AddExecutions(ctx context.Context, tenantID, runID string, testIDs []string) (int, error) {
	run, err := s.GetRun(ctx, tenantID, runID)
	if err != nil {
		return 0, err
	}
	var n int
	err = s.tx(ctx, func(c conn) error {
		var err error
		n, err = s.addExecutions(ctx, c, tenantID, run.ProjectID, runID, testIDs)
		return err
	})
	return n, err
}

// GetRun returns a run with its result summary.
func (s *Store) GetRun(ctx context.Context, tenantID, id string) (*model.TestRun, error) {
	run, err := scanRun(s.c().row(ctx, `SELECT `+runCols+` FROM test_runs WHERE id = ? AND tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "run")
	}
	sum, err := s.runSummaries(ctx, `WHERE run_id = ?`, id)
	if err != nil {
		return nil, err
	}
	run.Summary = sum[id]
	if run.Summary == nil {
		run.Summary = map[string]int{}
	}
	return run, nil
}

func (s *Store) runSummaries(ctx context.Context, where string, args ...any) (map[string]map[string]int, error) {
	rows, err := s.c().query(ctx, `SELECT run_id, status, COUNT(*) FROM test_executions `+where+` GROUP BY run_id, status`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]int{}
	for rows.Next() {
		var run, status string
		var n int
		if err := rows.Scan(&run, &status, &n); err != nil {
			return nil, err
		}
		if out[run] == nil {
			out[run] = map[string]int{}
		}
		out[run][status] = n
		out[run]["total"] += n
	}
	return out, rows.Err()
}

// ListRuns returns runs of a project, newest first.
func (s *Store) ListRuns(ctx context.Context, tenantID, projectID string) ([]model.TestRun, error) {
	rows, err := s.c().query(ctx, `SELECT `+runCols+` FROM test_runs WHERE tenant_id = ? AND project_id = ? ORDER BY created_at DESC`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	out := []model.TestRun{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *r)
	}
	rows.Close()
	sums, err := s.runSummaries(ctx, `WHERE run_id IN (SELECT id FROM test_runs WHERE project_id = ?)`, projectID)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Summary = sums[out[i].ID]
		if out[i].Summary == nil {
			out[i].Summary = map[string]int{}
		}
	}
	return out, nil
}

// UpdateRun stores run fields; status transitions maintain timestamps.
func (s *Store) UpdateRun(ctx context.Context, tenantID string, run *model.TestRun) error {
	run.Name = strings.TrimSpace(run.Name)
	if run.Name == "" {
		return Invalidf("run name is required")
	}
	if !model.Valid(model.RunStatuses, run.Status) {
		return Invalidf("invalid run status %q", run.Status)
	}
	now := s.now()
	if run.Status == "in_progress" && run.StartedAt == nil {
		run.StartedAt = &now
	}
	if (run.Status == "completed" || run.Status == "aborted") && run.CompletedAt == nil {
		run.CompletedAt = &now
	}
	if run.Status == "planned" || run.Status == "in_progress" {
		run.CompletedAt = nil
	}
	res, err := s.c().exec(ctx, `UPDATE test_runs SET name = ?, description = ?, status = ?, environment = ?, build = ?, started_at = ?, completed_at = ? WHERE id = ? AND tenant_id = ?`,
		run.Name, run.Description, run.Status, run.Environment, run.Build, nullTime(run.StartedAt), nullTime(run.CompletedAt), run.ID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "run")
	}
	return nil
}

// DeleteRun removes a run and its executions.
func (s *Store) DeleteRun(ctx context.Context, tenantID, id string) error {
	res, err := s.c().exec(ctx, `DELETE FROM test_runs WHERE id = ? AND tenant_id = ?`, id, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "run")
	}
	return nil
}

const execCols = `e.id, e.run_id, e.test_case_id, t.key, t.title, e.status, e.assignee, e.executed_by, e.started_at, e.finished_at, e.duration_ms, e.output, e.notes, e.position`

func scanExecution(r interface{ Scan(...any) error }) (*model.Execution, error) {
	var e model.Execution
	var st, ft sql.NullTime
	if err := r.Scan(&e.ID, &e.RunID, &e.TestCaseID, &e.TestKey, &e.TestTitle, &e.Status, &e.Assignee, &e.ExecutedBy, &st, &ft, &e.DurationMS, &e.Output, &e.Notes, &e.Position); err != nil {
		return nil, err
	}
	e.StartedAt, e.FinishedAt = timePtr(st), timePtr(ft)
	return &e, nil
}

// ListExecutions lists executions of a run in order.
func (s *Store) ListExecutions(ctx context.Context, tenantID, runID string) ([]model.Execution, error) {
	rows, err := s.c().query(ctx, `SELECT `+execCols+` FROM test_executions e JOIN test_cases t ON t.id = e.test_case_id WHERE e.run_id = ? AND e.tenant_id = ? ORDER BY e.position`, runID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Execution{}
	for rows.Next() {
		e, err := scanExecution(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// GetExecution returns an execution with step results.
func (s *Store) GetExecution(ctx context.Context, tenantID, id string) (*model.Execution, error) {
	e, err := scanExecution(s.c().row(ctx, `SELECT `+execCols+` FROM test_executions e JOIN test_cases t ON t.id = e.test_case_id WHERE e.id = ? AND e.tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "execution")
	}
	rows, err := s.c().query(ctx, `SELECT id, step_id, position, action, expected, status, actual FROM step_results WHERE execution_id = ? ORDER BY position`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	e.StepResults = []model.StepResult{}
	for rows.Next() {
		var sr model.StepResult
		if err := rows.Scan(&sr.ID, &sr.StepID, &sr.Position, &sr.Action, &sr.Expected, &sr.Status, &sr.Actual); err != nil {
			return nil, err
		}
		e.StepResults = append(e.StepResults, sr)
	}
	return e, rows.Err()
}

// ExecutionUpdate holds changes for an execution.
type ExecutionUpdate struct {
	Status      *string            `json:"status"`
	Notes       *string            `json:"notes"`
	Assignee    *string            `json:"assignee"`
	Output      *string            `json:"output"`
	DurationMS  *int64             `json:"duration_ms"`
	StepResults []model.StepResult `json:"step_results"`
}

// DeriveStatus computes an overall result from step results.
func DeriveStatus(steps []model.StepResult) string {
	if len(steps) == 0 {
		return model.ResultNotRun
	}
	counts := map[string]int{}
	for _, s := range steps {
		counts[s.Status]++
	}
	switch {
	case counts[model.ResultFailed] > 0:
		return model.ResultFailed
	case counts[model.ResultBlocked] > 0:
		return model.ResultBlocked
	case counts[model.ResultPassed]+counts[model.ResultSkipped] == len(steps) && counts[model.ResultPassed] > 0:
		return model.ResultPassed
	case counts[model.ResultSkipped] == len(steps):
		return model.ResultSkipped
	case counts[model.ResultNotRun] == len(steps):
		return model.ResultNotRun
	}
	return model.ResultInProgress
}

// UpdateExecution applies an update, records the executor and timing and
// moves the run to in_progress.
func (s *Store) UpdateExecution(ctx context.Context, tenantID, id, user string, u ExecutionUpdate) (*model.Execution, error) {
	e, err := s.GetExecution(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if u.Status != nil && !model.Valid(model.ResultStatuses, *u.Status) {
		return nil, Invalidf("invalid status %q", *u.Status)
	}
	byID := map[string]int{}
	for i, sr := range e.StepResults {
		byID[sr.ID] = i
	}
	for _, sr := range u.StepResults {
		i, ok := byID[sr.ID]
		if !ok {
			return nil, Invalidf("unknown step result %s", sr.ID)
		}
		if sr.Status != "" {
			if !model.Valid(model.ResultStatuses, sr.Status) {
				return nil, Invalidf("invalid step status %q", sr.Status)
			}
			e.StepResults[i].Status = sr.Status
		}
		e.StepResults[i].Actual = sr.Actual
	}
	status := e.Status
	switch {
	case u.Status != nil:
		status = *u.Status
	case len(u.StepResults) > 0:
		status = DeriveStatus(e.StepResults)
	}
	now := s.now()
	if status != model.ResultNotRun && e.StartedAt == nil {
		e.StartedAt = &now
	}
	final := status == model.ResultPassed || status == model.ResultFailed || status == model.ResultBlocked || status == model.ResultSkipped
	if final {
		e.FinishedAt = &now
		if u.DurationMS != nil {
			e.DurationMS = *u.DurationMS
		} else if e.StartedAt != nil {
			e.DurationMS = now.Sub(*e.StartedAt).Milliseconds()
		}
		e.ExecutedBy = user
	} else {
		e.FinishedAt = nil
	}
	if status == model.ResultNotRun {
		e.StartedAt = nil
		e.DurationMS = 0
	}
	e.Status = status
	if u.Notes != nil {
		e.Notes = *u.Notes
	}
	if u.Assignee != nil {
		e.Assignee = *u.Assignee
	}
	if u.Output != nil {
		e.Output = *u.Output
	}
	err = s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `UPDATE test_executions SET status = ?, notes = ?, assignee = ?, output = ?, executed_by = ?, started_at = ?, finished_at = ?, duration_ms = ? WHERE id = ?`,
			e.Status, e.Notes, e.Assignee, e.Output, e.ExecutedBy, nullTime(e.StartedAt), nullTime(e.FinishedAt), e.DurationMS, e.ID); err != nil {
			return err
		}
		for _, sr := range e.StepResults {
			if _, err := c.exec(ctx, `UPDATE step_results SET status = ?, actual = ? WHERE id = ?`, sr.Status, sr.Actual, sr.ID); err != nil {
				return err
			}
		}
		if status != model.ResultNotRun {
			if _, err := c.exec(ctx, `UPDATE test_runs SET status = 'in_progress', started_at = COALESCE(started_at, ?) WHERE id = ? AND status = 'planned'`, now, e.RunID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return e, nil
}

// ImportResult is one automated result reported by CI.
type ImportResult struct {
	TestKey    string `json:"test_key"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Output     string `json:"output"`
}

// ImportResults records CI results by test key; tests not yet in the run
// are added. Returns updated count and unknown keys.
func (s *Store) ImportResults(ctx context.Context, tenantID, runID, user string, results []ImportResult) (int, []string, error) {
	run, err := s.GetRun(ctx, tenantID, runID)
	if err != nil {
		return 0, nil, err
	}
	tests, err := s.c().query(ctx, `SELECT key, id FROM test_cases WHERE project_id = ? AND tenant_id = ?`, run.ProjectID, tenantID)
	if err != nil {
		return 0, nil, err
	}
	byKey := map[string]string{}
	for tests.Next() {
		var k, id string
		if err := tests.Scan(&k, &id); err != nil {
			tests.Close()
			return 0, nil, err
		}
		byKey[strings.ToUpper(k)] = id
	}
	tests.Close()
	unknown := []string{}
	updated := 0
	for _, r := range results {
		if !model.Valid(model.ResultStatuses, r.Status) {
			return updated, unknown, Invalidf("invalid status %q for %s", r.Status, r.TestKey)
		}
		tid, ok := byKey[strings.ToUpper(strings.TrimSpace(r.TestKey))]
		if !ok {
			unknown = append(unknown, r.TestKey)
			continue
		}
		if _, err := s.AddExecutions(ctx, tenantID, runID, []string{tid}); err != nil {
			return updated, unknown, err
		}
		var eid string
		if err := s.c().row(ctx, `SELECT id FROM test_executions WHERE run_id = ? AND test_case_id = ?`, runID, tid).Scan(&eid); err != nil {
			return updated, unknown, err
		}
		st, out, dur := r.Status, r.Output, r.DurationMS
		if _, err := s.UpdateExecution(ctx, tenantID, eid, user, ExecutionUpdate{Status: &st, Output: &out, DurationMS: &dur}); err != nil {
			return updated, unknown, err
		}
		updated++
	}
	return updated, unknown, nil
}

// LastResults returns test id → latest non-not_run execution status.
func (s *Store) LastResults(ctx context.Context, tenantID, projectID string) (map[string]string, error) {
	rows, err := s.c().query(ctx, `SELECT e.test_case_id, e.status, e.finished_at, e.started_at, r.created_at FROM test_executions e JOIN test_runs r ON r.id = e.run_id
		WHERE r.project_id = ? AND e.tenant_id = ? AND e.status <> 'not_run'`, projectID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type best struct {
		at     time.Time
		status string
	}
	m := map[string]best{}
	for rows.Next() {
		var tid, st string
		var fin, start sql.NullTime
		var created time.Time
		if err := rows.Scan(&tid, &st, &fin, &start, &created); err != nil {
			return nil, err
		}
		at := created
		if start.Valid {
			at = start.Time
		}
		if fin.Valid {
			at = fin.Time
		}
		if b, ok := m[tid]; !ok || at.After(b.at) {
			m[tid] = best{at, st}
		}
	}
	out := map[string]string{}
	for k, v := range m {
		out[k] = v.status
	}
	return out, rows.Err()
}

// Coverage computes the coverage state of a requirement from the last
// results of its linked tests: none | not_run | passed | failed | partial.
func Coverage(linked []string, last map[string]string) string {
	if len(linked) == 0 {
		return "none"
	}
	passed, failed, notRun := 0, 0, 0
	for _, t := range linked {
		switch last[t] {
		case model.ResultPassed:
			passed++
		case model.ResultFailed, model.ResultBlocked:
			failed++
		default:
			notRun++
		}
	}
	switch {
	case failed > 0:
		return "failed"
	case passed == len(linked):
		return "passed"
	case passed > 0:
		return "partial"
	}
	return "not_run"
}

// LinkIndex returns requirement → test ids and test → requirement ids.
func LinkIndex(links []model.TraceLink) (map[string][]string, map[string][]string) {
	r2t, t2r := map[string][]string{}, map[string][]string{}
	for _, l := range links {
		r2t[l.RequirementID] = append(r2t[l.RequirementID], l.TestCaseID)
		t2r[l.TestCaseID] = append(t2r[l.TestCaseID], l.RequirementID)
	}
	for _, m := range []map[string][]string{r2t, t2r} {
		for k := range m {
			sort.Strings(m[k])
		}
	}
	return r2t, t2r
}
