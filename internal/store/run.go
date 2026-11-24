package store

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/gherkin"
	"github.com/rom/qtestframe/internal/model"
)

const runCols = `id, project_id, name, description, status, environment, build, created_by, created_at, started_at, completed_at, release_id,
	variant_id, build_id, COALESCE((SELECT v.name FROM variants v WHERE v.id = test_runs.variant_id), ''),
	COALESCE((SELECT CASE WHEN b.number <> '' THEN b.number ELSE b.external_id END FROM builds b WHERE b.id = test_runs.build_id), ''),
	custom_fields, cycle_id, COALESCE((SELECT c.name FROM test_cycles c WHERE c.id = test_runs.cycle_id), ''), environment_id`

func scanRun(r interface{ Scan(...any) error }) (*model.TestRun, error) {
	var run model.TestRun
	var st, ct sql.NullTime
	var rel, variant, build, cycle, env sql.NullString
	var custom string
	if err := r.Scan(&run.ID, &run.ProjectID, &run.Name, &run.Description, &run.Status, &run.Environment, &run.Build, &run.CreatedBy, &run.CreatedAt, &st, &ct, &rel,
		&variant, &build, &run.VariantName, &run.BuildLabel, &custom, &cycle, &run.CycleName, &env); err != nil {
		return nil, err
	}
	run.CustomFields = decodeCustom(custom)
	run.StartedAt, run.CompletedAt = timePtr(st), timePtr(ct)
	run.ReleaseID = strPtr(rel)
	run.VariantID, run.BuildID = strPtr(variant), strPtr(build)
	run.CycleID = strPtr(cycle)
	run.EnvironmentID = strPtr(env)
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
		if err := checkRelease(ctx, c, tenantID, run.ProjectID, run.ReleaseID); err != nil {
			return err
		}
		if err := checkRunVariantBuild(ctx, c, tenantID, run.ProjectID, run.VariantID, run.BuildID); err != nil {
			return err
		}
		if err := resolveRunEnvironment(ctx, c, tenantID, run.ProjectID, &run.EnvironmentID, &run.Environment); err != nil {
			return err
		}
		cfgs := make([]string, 0, len(run.Configurations))
		for _, r := range run.Configurations {
			cfgs = append(cfgs, r.ID)
		}
		cfgs = uniq(cfgs)
		if err := checkRunConfigurations(ctx, c, tenantID, run.ProjectID, cfgs); err != nil {
			return err
		}
		// a run created without custom_fields (a result import) gets the
		// defaults; required fields are checked when values are given
		custom, err := checkCustomOpt(ctx, c, tenantID, run.ProjectID, "run", run.CustomFields, nil, true, run.CustomFields == nil)
		if err != nil {
			return err
		}
		run.CustomFields = custom
		if _, err := c.exec(ctx, `INSERT INTO test_runs (id, tenant_id, project_id, name, description, status, environment, build, created_by, created_at, release_id, variant_id, build_id, custom_fields, cycle_id, environment_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			run.ID, tenantID, run.ProjectID, run.Name, run.Description, run.Status, run.Environment, run.Build, run.CreatedBy, run.CreatedAt, nullStr(run.ReleaseID), nullStr(run.VariantID), nullStr(run.BuildID), encodeCustom(run.CustomFields), nullStr(run.CycleID), nullStr(run.EnvironmentID)); err != nil {
			return err
		}
		for i, id := range cfgs {
			if _, err := c.exec(ctx, `INSERT INTO run_configurations (run_id, configuration_id, position) VALUES (?, ?, ?)`, run.ID, id, i); err != nil {
				return err
			}
		}
		if run.Configurations, err = runConfigurations(ctx, c, run.ID); err != nil {
			return err
		}
		_, err = s.addExecutions(ctx, c, tenantID, run.ProjectID, run.ID, testIDs)
		return err
	})
}

// addExecutions adds the tests to a run: one execution per test and
// configuration of the run (one without configuration when the run targets
// none). Pairs already in the run are skipped. The steps are snapshotted;
// a data-driven test gets one iteration per data row, each with its own
// step results whose {{parameter}} placeholders are resolved.
func (s *Store) addExecutions(ctx context.Context, c conn, tenantID, projectID, runID string, testIDs []string) (int, error) {
	var pos sql.NullInt64
	if err := c.row(ctx, `SELECT MAX(position) FROM test_executions WHERE run_id = ?`, runID).Scan(&pos); err != nil {
		return 0, err
	}
	next := 0
	if pos.Valid {
		next = int(pos.Int64) + 1
	}
	cfgRefs, err := runConfigurations(ctx, c, runID)
	if err != nil {
		return 0, err
	}
	cfgs := []*string{nil}
	if len(cfgRefs) > 0 {
		cfgs = cfgs[:0]
		for i := range cfgRefs {
			cfgs = append(cfgs, &cfgRefs[i].ID)
		}
	}
	added := 0
	for _, tid := range uniq(testIDs) {
		var exists int
		var params, rowsJSON, format, gherkinSrc string
		err := c.row(ctx, `SELECT parameters, data_rows, format, gherkin FROM test_cases WHERE id = ? AND tenant_id = ? AND project_id = ?`, tid, tenantID, projectID).Scan(&params, &rowsJSON, &format, &gherkinSrc)
		if err == sql.ErrNoRows {
			return added, Invalidf("test %s not found in project", tid)
		}
		if err != nil {
			return added, err
		}
		// a test that takes its data from a shared set gets the set's rows
		setID, setVersion, _, setRows, fromSet, err := testDataSource(ctx, c, tid)
		if err != nil {
			return added, err
		}
		if fromSet {
			rowsJSON = setRows
		}
		dataRows := decodeRows(rowsJSON)
		// a Gherkin test: one iteration per scenario and Examples row
		var scenarios []gherkin.Instance
		isGherkin := format == model.FormatGherkin
		if isGherkin {
			if scenarios, err = gherkinInstances(gherkinSrc); err != nil {
				return added, err
			}
			fromSet = false
		}
		usageNoted := false
		var steps []model.TestStep
		stepsLoaded := false
		for _, cfg := range cfgs {
			if cfg == nil {
				err = c.row(ctx, `SELECT COUNT(*) FROM test_executions WHERE run_id = ? AND test_case_id = ? AND configuration_id IS NULL`, runID, tid).Scan(&exists)
			} else {
				err = c.row(ctx, `SELECT COUNT(*) FROM test_executions WHERE run_id = ? AND test_case_id = ? AND configuration_id = ?`, runID, tid, *cfg).Scan(&exists)
			}
			if err != nil {
				return added, err
			}
			if exists > 0 {
				continue
			}
			if !stepsLoaded && !isGherkin {
				// snapshot the steps, expanding shared (call) steps in place
				if steps, err = expandSteps(ctx, c, tid, "", 0, map[string]bool{tid: true}); err != nil {
					return added, err
				}
				stepsLoaded = true
			}
			eid := newID()
			if _, err := c.exec(ctx, `INSERT INTO test_executions (id, tenant_id, run_id, test_case_id, status, position, configuration_id) VALUES (?, ?, ?, ?, 'not_run', ?, ?)`, eid, tenantID, runID, tid, next, nullStr(cfg)); err != nil {
				return added, err
			}
			if isGherkin {
				for i, sc := range scenarios {
					iid := newID()
					values := sc.Values
					if values == nil {
						values = map[string]string{}
					}
					if _, err := c.exec(ctx, `INSERT INTO execution_iterations (id, execution_id, position, name, data_values, status, notes) VALUES (?, ?, ?, ?, ?, 'not_run', '')`,
						iid, eid, i, truncate(sc.Name, 500), encodeJSON(values)); err != nil {
						return added, err
					}
					if err := insertStepResults(ctx, c, eid, &iid, instanceSteps(sc), nil); err != nil {
						return added, err
					}
				}
				dataRows = nil
			} else if len(dataRows) == 0 {
				if err := insertStepResults(ctx, c, eid, nil, steps, nil); err != nil {
					return added, err
				}
			}
			for i, r := range dataRows {
				iid := newID()
				if _, err := c.exec(ctx, `INSERT INTO execution_iterations (id, execution_id, position, name, data_values, status, notes) VALUES (?, ?, ?, ?, ?, 'not_run', '')`,
					iid, eid, i, r.Name, encodeJSON(r.Values)); err != nil {
					return added, err
				}
				if err := insertStepResults(ctx, c, eid, &iid, steps, r.Values); err != nil {
					return added, err
				}
			}
			next++
			added++
			if fromSet && !usageNoted {
				usageNoted = true
				if err := recordDataUsage(ctx, c, tenantID, setID, setVersion, runID, tid, "", s.now()); err != nil {
					return added, err
				}
			}
		}
	}
	return added, nil
}

// insertStepResults snapshots steps as step results of an execution (and
// iteration), resolving placeholders with values.
func insertStepResults(ctx context.Context, c conn, execID string, iterID *string, steps []model.TestStep, values map[string]string) error {
	for i, st := range steps {
		if _, err := c.exec(ctx, `INSERT INTO step_results (id, execution_id, step_id, position, action, expected, status, actual, iteration_id) VALUES (?, ?, ?, ?, ?, ?, 'not_run', '', ?)`,
			newID(), execID, st.ID, i, ResolvePlaceholders(st.Action, values), ResolvePlaceholders(st.Expected, values), nullStr(iterID)); err != nil {
			return err
		}
	}
	return nil
}

// expandSteps returns the steps of test tid with call steps replaced by the
// called test's steps (recursively), prefixed with "[KEY] ".
func expandSteps(ctx context.Context, c conn, tid, prefix string, depth int, onPath map[string]bool) ([]model.TestStep, error) {
	rows, err := c.query(ctx, `SELECT s.id, s.action, s.expected, s.call_test_id, COALESCE(t.key, '') FROM test_steps s LEFT JOIN test_cases t ON t.id = s.call_test_id WHERE s.test_case_id = ? ORDER BY s.position`, tid)
	if err != nil {
		return nil, err
	}
	type raw struct {
		step model.TestStep
		call sql.NullString
		key  string
	}
	var list []raw
	for rows.Next() {
		var r raw
		if err := rows.Scan(&r.step.ID, &r.step.Action, &r.step.Expected, &r.call, &r.key); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, r)
	}
	rows.Close()
	var out []model.TestStep
	for _, r := range list {
		if !r.call.Valid {
			st := r.step
			st.Action = prefix + st.Action
			out = append(out, st)
			continue
		}
		// cycles and excessive depth are rejected on save; guard anyway
		if depth >= MaxCallDepth || onPath[r.call.String] {
			continue
		}
		onPath[r.call.String] = true
		sub, err := expandSteps(ctx, c, r.call.String, prefix+"["+r.key+"] ", depth+1, onPath)
		delete(onPath, r.call.String)
		if err != nil {
			return nil, err
		}
		out = append(out, sub...)
	}
	return out, nil
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
		if n, err = s.addExecutions(ctx, c, tenantID, run.ProjectID, runID, testIDs); err != nil || n == 0 {
			return err
		}
		// new tests to execute reopen a completed run
		return syncRunStatus(ctx, c, runID, s.now(), true)
	})
	return n, err
}

// syncRunStatus completes a run once every execution has a final result
// (passed, failed, blocked or skipped). With reopen, a completed run that
// has executions without a final result again goes back to in_progress.
// Aborted runs are left alone; a planned run without results stays planned.
func syncRunStatus(ctx context.Context, c conn, runID string, now time.Time, reopen bool) error {
	var total, open int
	if err := c.row(ctx, `SELECT COUNT(*), COALESCE(SUM(CASE WHEN status IN ('not_run', 'in_progress') THEN 1 ELSE 0 END), 0) FROM test_executions WHERE run_id = ?`, runID).Scan(&total, &open); err != nil {
		return err
	}
	if total == 0 {
		return nil
	}
	if open == 0 {
		_, err := c.exec(ctx, `UPDATE test_runs SET status = 'completed', started_at = COALESCE(started_at, ?), completed_at = ? WHERE id = ? AND status IN ('planned', 'in_progress')`, now, now, runID)
		return err
	}
	if reopen {
		_, err := c.exec(ctx, `UPDATE test_runs SET status = 'in_progress', completed_at = NULL WHERE id = ? AND status = 'completed'`, runID)
		return err
	}
	return nil
}

// GetRun returns a run with its result summary.
func (s *Store) GetRun(ctx context.Context, tenantID, id string) (*model.TestRun, error) {
	run, err := scanRun(s.c(ctx).row(ctx, `SELECT `+runCols+` FROM test_runs WHERE id = ? AND tenant_id = ?`, id, tenantID))
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
	if run.Configurations, err = runConfigurations(ctx, s.c(ctx), id); err != nil {
		return nil, err
	}
	return run, nil
}

func (s *Store) runSummaries(ctx context.Context, where string, args ...any) (map[string]map[string]int, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT run_id, status, COUNT(*) FROM test_executions `+where+` GROUP BY run_id, status`, args...)
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
	rows, err := s.c(ctx).query(ctx, `SELECT `+runCols+` FROM test_runs WHERE tenant_id = ? AND project_id = ? ORDER BY created_at DESC`, tenantID, projectID)
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
	cfgs, err := s.runConfigurationsOf(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Summary = sums[out[i].ID]
		if out[i].Summary == nil {
			out[i].Summary = map[string]int{}
		}
		out[i].Configurations = cfgs[out[i].ID]
		if out[i].Configurations == nil {
			out[i].Configurations = []model.ConfigurationRef{}
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
	if err := checkRelease(ctx, s.c(ctx), tenantID, run.ProjectID, run.ReleaseID); err != nil {
		return err
	}
	if err := checkRunVariantBuild(ctx, s.c(ctx), tenantID, run.ProjectID, run.VariantID, run.BuildID); err != nil {
		return err
	}
	if err := resolveRunEnvironment(ctx, s.c(ctx), tenantID, run.ProjectID, &run.EnvironmentID, &run.Environment); err != nil {
		return err
	}
	custom, err := s.checkItemCustom(ctx, tenantID, "run", run.ID, run.CustomFields)
	if err != nil {
		return err
	}
	run.CustomFields = custom
	res, err := s.c(ctx).exec(ctx, `UPDATE test_runs SET name = ?, description = ?, status = ?, environment = ?, build = ?, started_at = ?, completed_at = ?, release_id = ?, variant_id = ?, build_id = ?, custom_fields = ?, environment_id = ? WHERE id = ? AND tenant_id = ?`,
		run.Name, run.Description, run.Status, run.Environment, run.Build, nullTime(run.StartedAt), nullTime(run.CompletedAt), nullStr(run.ReleaseID), nullStr(run.VariantID), nullStr(run.BuildID), encodeCustom(run.CustomFields), nullStr(run.EnvironmentID), run.ID, tenantID)
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
	return s.tx(ctx, func(c conn) error {
		if err := legalHoldGuard(ctx, c, `SELECT COUNT(*) FROM test_runs r JOIN projects p ON p.id = r.project_id LEFT JOIN releases rel ON rel.id = r.release_id
			WHERE r.id = ? AND (p.legal_hold = ? OR COALESCE(rel.legal_hold, ?) = ?)`, id, true, false, true); err != nil {
			return err
		}
		rows, err := c.query(ctx, `SELECT id FROM test_executions WHERE run_id = ? AND tenant_id = ?`, id, tenantID)
		if err != nil {
			return err
		}
		execs, err := scanStrings(rows)
		if err != nil {
			return err
		}
		if err := removeExecutionAttachments(ctx, c, execs); err != nil {
			return err
		}
		if err := deleteDefectLinks(ctx, c, "execution", execs); err != nil {
			return err
		}
		if err := removeAttachments(ctx, c, "run", []string{id}); err != nil {
			return err
		}
		if err := deleteItemLinks(ctx, c, "run", []string{id}); err != nil {
			return err
		}
		if err := deleteDefectLinks(ctx, c, "run", []string{id}); err != nil {
			return err
		}
		if err := deleteComments(ctx, c, model.DiscussionRun, []string{id}); err != nil {
			return err
		}
		res, err := c.exec(ctx, `DELETE FROM test_runs WHERE id = ? AND tenant_id = ?`, id, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "run")
		}
		return nil
	})
}

const execCols = `e.id, e.run_id, e.test_case_id, t.key, t.title, e.status, e.assignee, e.executed_by, e.started_at, e.finished_at, e.duration_ms, e.output, e.notes, e.position, e.defects,
	e.configuration_id, COALESCE(cf.name, ''), (SELECT COUNT(*) FROM execution_iterations i WHERE i.execution_id = e.id),
	e.environment_id, e.environment_name, e.environment_components`

// execFrom joins what execCols needs.
const execFrom = ` FROM test_executions e JOIN test_cases t ON t.id = e.test_case_id LEFT JOIN configurations cf ON cf.id = e.configuration_id`

func scanExecution(r interface{ Scan(...any) error }, extra ...any) (*model.Execution, error) {
	var e model.Execution
	var st, ft sql.NullTime
	var defects string
	var cfg, env sql.NullString
	var comps string
	dest := append([]any{&e.ID, &e.RunID, &e.TestCaseID, &e.TestKey, &e.TestTitle, &e.Status, &e.Assignee, &e.ExecutedBy, &st, &ft, &e.DurationMS, &e.Output, &e.Notes, &e.Position, &defects,
		&cfg, &e.ConfigurationName, &e.IterationCount, &env, &e.EnvironmentName, &comps}, extra...)
	if err := r.Scan(dest...); err != nil {
		return nil, err
	}
	e.StartedAt, e.FinishedAt = timePtr(st), timePtr(ft)
	e.Defects = decodeTags(defects)
	e.ConfigurationID = strPtr(cfg)
	e.EnvironmentID = strPtr(env)
	if comps != "" && comps != "[]" {
		e.EnvironmentComponents = decodeComponents(comps)
	}
	return &e, nil
}

// ListTestExecutions returns the executions of a test across all runs,
// newest run first.
func (s *Store) ListTestExecutions(ctx context.Context, tenantID, testID string) ([]model.Execution, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+execCols+`, r.name, r.status, r.environment, r.build, r.created_at
		`+execFrom+` JOIN test_runs r ON r.id = e.run_id
		WHERE e.test_case_id = ? AND e.tenant_id = ? ORDER BY r.created_at DESC, e.position`, testID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Execution{}
	for rows.Next() {
		var created time.Time
		var x model.Execution
		e, err := scanExecution(rows, &x.RunName, &x.RunStatus, &x.RunEnvironment, &x.RunBuild, &created)
		if err != nil {
			return nil, err
		}
		e.RunName, e.RunStatus, e.RunEnvironment, e.RunBuild = x.RunName, x.RunStatus, x.RunEnvironment, x.RunBuild
		c := created.UTC()
		e.RunCreatedAt = &c
		out = append(out, *e)
	}
	return out, rows.Err()
}

// ListExecutions lists executions of a run in order.
func (s *Store) ListExecutions(ctx context.Context, tenantID, runID string) ([]model.Execution, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+execCols+execFrom+` WHERE e.run_id = ? AND e.tenant_id = ? ORDER BY e.position`, runID, tenantID)
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

// MaxEvidence bounds the evidence text of one step result.
const MaxEvidence = 20000

// StepResultExecution returns the execution a step result belongs to.
func (s *Store) StepResultExecution(ctx context.Context, tenantID, stepResultID string) (string, error) {
	var id string
	err := s.c(ctx).row(ctx, `SELECT e.id FROM step_results sr JOIN test_executions e ON e.id = sr.execution_id WHERE sr.id = ? AND e.tenant_id = ?`, stepResultID, tenantID).Scan(&id)
	return id, mapErr(err, "step result")
}

// GetExecution returns an execution with step results.
func (s *Store) GetExecution(ctx context.Context, tenantID, id string) (*model.Execution, error) {
	e, err := scanExecution(s.c(ctx).row(ctx, `SELECT `+execCols+execFrom+` WHERE e.id = ? AND e.tenant_id = ?`, id, tenantID))
	if err != nil {
		return nil, mapErr(err, "execution")
	}
	if e.Iterations, err = listIterations(ctx, s.c(ctx), id); err != nil {
		return nil, err
	}
	pos := map[string]int{}
	for _, it := range e.Iterations {
		pos[it.ID] = it.Position
	}
	rows, err := s.c(ctx).query(ctx, `SELECT sr.id, sr.step_id, sr.position, sr.action, sr.expected, sr.status, sr.actual, sr.evidence, sr.iteration_id,
		(SELECT COUNT(*) FROM attachments a WHERE a.entity_type = 'step' AND a.entity_id = sr.id)
		FROM step_results sr WHERE sr.execution_id = ? ORDER BY sr.position`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	e.StepResults = []model.StepResult{}
	for rows.Next() {
		var sr model.StepResult
		var it sql.NullString
		if err := rows.Scan(&sr.ID, &sr.StepID, &sr.Position, &sr.Action, &sr.Expected, &sr.Status, &sr.Actual, &sr.Evidence, &it, &sr.AttachmentCount); err != nil {
			return nil, err
		}
		sr.IterationID = strPtr(it)
		e.StepResults = append(e.StepResults, sr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// iteration by iteration, steps in order
	sort.SliceStable(e.StepResults, func(i, j int) bool {
		a, b := -1, -1
		if e.StepResults[i].IterationID != nil {
			a = pos[*e.StepResults[i].IterationID]
		}
		if e.StepResults[j].IterationID != nil {
			b = pos[*e.StepResults[j].IterationID]
		}
		return a < b
	})
	return e, nil
}

// listIterations returns the iterations of an execution in order.
func listIterations(ctx context.Context, c conn, execID string) ([]model.Iteration, error) {
	rows, err := c.query(ctx, `SELECT id, position, name, data_values, status, notes FROM execution_iterations WHERE execution_id = ? ORDER BY position`, execID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Iteration{}
	for rows.Next() {
		var it model.Iteration
		var vals string
		if err := rows.Scan(&it.ID, &it.Position, &it.Name, &vals, &it.Status, &it.Notes); err != nil {
			return nil, err
		}
		it.Values = decodeValues(vals)
		out = append(out, it)
	}
	return out, rows.Err()
}

// ExecutionUpdate holds changes for an execution.
type ExecutionUpdate struct {
	Status      *string            `json:"status"`
	Notes       *string            `json:"notes"`
	Assignee    *string            `json:"assignee"`
	Output      *string            `json:"output"`
	DurationMS  *int64             `json:"duration_ms"`
	Defects     *[]string          `json:"defects"`
	StepResults []model.StepResult `json:"step_results"`
	// Iterations sets results (and notes) of iterations of a data-driven
	// execution; the status of an iteration with steps is always derived
	// from its step results.
	Iterations []IterationUpdate `json:"iterations"`
}

// IterationUpdate changes one iteration.
type IterationUpdate struct {
	ID     string  `json:"id"`
	Status string  `json:"status"`
	Notes  *string `json:"notes"`
}

// DeriveStatus computes an overall result from step results.
func DeriveStatus(steps []model.StepResult) string {
	st := make([]string, len(steps))
	for i, s := range steps {
		st[i] = s.Status
	}
	return DeriveResult(st)
}

// DeriveResult combines results (of steps, or of the iterations of a
// data-driven execution) into one: failed if any failed, else blocked if
// any is blocked, passed when all passed or were skipped (at least one
// passed), skipped when all were skipped, not_run when none has a result,
// otherwise in_progress.
func DeriveResult(statuses []string) string {
	if len(statuses) == 0 {
		return model.ResultNotRun
	}
	counts := map[string]int{}
	for _, s := range statuses {
		counts[s]++
	}
	switch {
	case counts[model.ResultFailed] > 0:
		return model.ResultFailed
	case counts[model.ResultBlocked] > 0:
		return model.ResultBlocked
	case counts[model.ResultPassed]+counts[model.ResultSkipped] == len(statuses) && counts[model.ResultPassed] > 0:
		return model.ResultPassed
	case counts[model.ResultSkipped] == len(statuses):
		return model.ResultSkipped
	case counts[model.ResultNotRun] == len(statuses):
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
		if len(sr.Evidence) > MaxEvidence {
			return nil, Invalidf("evidence of step %d is longer than %d characters", e.StepResults[i].Position+1, MaxEvidence)
		}
		e.StepResults[i].Actual = sr.Actual
		e.StepResults[i].Evidence = sr.Evidence
	}
	iterByID := map[string]int{}
	for i, it := range e.Iterations {
		iterByID[it.ID] = i
	}
	for _, iu := range u.Iterations {
		i, ok := iterByID[iu.ID]
		if !ok {
			return nil, Invalidf("unknown iteration %s", iu.ID)
		}
		if iu.Status != "" {
			if !model.Valid(model.ResultStatuses, iu.Status) {
				return nil, Invalidf("invalid iteration status %q", iu.Status)
			}
			e.Iterations[i].Status = iu.Status
		}
		if iu.Notes != nil {
			if len(*iu.Notes) > MaxEvidence {
				return nil, Invalidf("notes of iteration %d are longer than %d characters", e.Iterations[i].Position+1, MaxEvidence)
			}
			e.Iterations[i].Notes = *iu.Notes
		}
	}
	// iterations with steps take the result of their steps
	if len(u.StepResults) > 0 || len(u.Iterations) > 0 {
		perIter := map[string][]model.StepResult{}
		for _, sr := range e.StepResults {
			if sr.IterationID != nil {
				perIter[*sr.IterationID] = append(perIter[*sr.IterationID], sr)
			}
		}
		for i, it := range e.Iterations {
			if steps := perIter[it.ID]; len(steps) > 0 {
				e.Iterations[i].Status = DeriveStatus(steps)
			}
		}
	}
	wasFinal := finalResult(e.Status)
	status := e.Status
	switch {
	case u.Status != nil:
		status = *u.Status
	case len(e.Iterations) > 0 && (len(u.StepResults) > 0 || len(u.Iterations) > 0):
		st := make([]string, len(e.Iterations))
		for i, it := range e.Iterations {
			st[i] = it.Status
		}
		status = DeriveResult(st)
	case len(u.StepResults) > 0:
		status = DeriveStatus(e.StepResults)
	}
	now := s.now()
	if status != model.ResultNotRun && e.StartedAt == nil {
		e.StartedAt = &now
	}
	final := finalResult(status)
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
	if u.Defects != nil {
		e.Defects = cleanTags(*u.Defects)
		for _, d := range e.Defects {
			if len(d) > 500 {
				return nil, Invalidf("defect reference too long")
			}
		}
	}
	err = s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `UPDATE test_executions SET defects = ?, status = ?, notes = ?, assignee = ?, output = ?, executed_by = ?, started_at = ?, finished_at = ?, duration_ms = ? WHERE id = ?`, encodeTags(e.Defects),
			e.Status, e.Notes, e.Assignee, e.Output, e.ExecutedBy, nullTime(e.StartedAt), nullTime(e.FinishedAt), e.DurationMS, e.ID); err != nil {
			return err
		}
		// the environment and its component versions at the time of the result
		if final || wasFinal {
			if err := snapshotExecutionEnvironment(ctx, c, e.ID, e.RunID, final); err != nil {
				return err
			}
		}
		for _, sr := range e.StepResults {
			if _, err := c.exec(ctx, `UPDATE step_results SET status = ?, actual = ?, evidence = ? WHERE id = ?`, sr.Status, sr.Actual, sr.Evidence, sr.ID); err != nil {
				return err
			}
		}
		for _, it := range e.Iterations {
			if _, err := c.exec(ctx, `UPDATE execution_iterations SET status = ?, notes = ? WHERE id = ?`, it.Status, it.Notes, it.ID); err != nil {
				return err
			}
		}
		if status != model.ResultNotRun {
			if _, err := c.exec(ctx, `UPDATE test_runs SET status = 'in_progress', started_at = COALESCE(started_at, ?) WHERE id = ? AND status = 'planned'`, now, e.RunID); err != nil {
				return err
			}
		}
		// the last final result completes the run; taking one back reopens it
		return syncRunStatus(ctx, c, e.RunID, now, wasFinal && !final)
	})
	if err != nil {
		return nil, err
	}
	return e, nil
}

// finalResult: the execution is done (passed, failed, blocked or skipped).
func finalResult(status string) bool {
	return status == model.ResultPassed || status == model.ResultFailed || status == model.ResultBlocked || status == model.ResultSkipped
}

// ImportResult is one automated result reported by CI.
type ImportResult struct {
	TestKey    string `json:"test_key"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Output     string `json:"output"`
	// Configuration (name or id) the result was obtained in; empty uses
	// the import's default (see ResolveImportConfigurations).
	Configuration string `json:"configuration,omitempty"`
	// ConfigurationID is set by ResolveImportConfigurations.
	ConfigurationID *string `json:"-"`
}

// ResolveImportConfigurations sets the configuration of every result: its
// own (name or id), else def. Without either, a run targeting exactly one
// configuration uses it, a run without configurations records results
// without configuration and a run targeting several is an error. A named
// configuration the run does not target yet is added to the run (see
// SetRunConfigurations).
func (s *Store) ResolveImportConfigurations(ctx context.Context, tenantID, runID, def string, results []ImportResult) error {
	run, err := s.GetRun(ctx, tenantID, runID)
	if err != nil {
		return err
	}
	targeted := map[string]bool{}
	order := []string{}
	for _, c := range run.Configurations {
		targeted[c.ID] = true
		order = append(order, c.ID)
	}
	found := map[string]string{}
	for i := range results {
		ref := strings.TrimSpace(results[i].Configuration)
		if ref == "" {
			ref = strings.TrimSpace(def)
		}
		if ref == "" {
			switch len(run.Configurations) {
			case 0:
				results[i].ConfigurationID = nil
			case 1:
				id := run.Configurations[0].ID
				results[i].ConfigurationID = &id
			default:
				return Invalidf("run %q targets %d configurations: name the configuration of the results", run.Name, len(run.Configurations))
			}
			continue
		}
		id, ok := found[strings.ToLower(ref)]
		if !ok {
			c, err := s.FindConfiguration(ctx, tenantID, run.ProjectID, ref)
			if err != nil {
				return err
			}
			id = c.ID
			found[strings.ToLower(ref)] = id
		}
		if !targeted[id] {
			targeted[id] = true
			order = append(order, id)
		}
		results[i].ConfigurationID = &id
	}
	if len(order) != len(run.Configurations) {
		if _, err := s.SetRunConfigurations(ctx, tenantID, runID, order); err != nil {
			return err
		}
	}
	return nil
}

// ImportResults records CI results by test key; tests not yet in the run
// are added. Results carry the configuration set by
// ResolveImportConfigurations (nil: the run's only configuration, or none).
// Returns updated count and unknown keys.
func (s *Store) ImportResults(ctx context.Context, tenantID, runID, user string, results []ImportResult) (int, []string, error) {
	run, err := s.GetRun(ctx, tenantID, runID)
	if err != nil {
		return 0, nil, err
	}
	tests, err := s.c(ctx).query(ctx, `SELECT key, id FROM test_cases WHERE project_id = ? AND tenant_id = ?`, run.ProjectID, tenantID)
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
		eid, err := s.executionOf(ctx, runID, tid, r.ConfigurationID)
		if err != nil {
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

// executionOf returns the execution of a test in a run and configuration
// (nil: the first one of the test in the run).
func (s *Store) executionOf(ctx context.Context, runID, testID string, cfg *string) (string, error) {
	var eid string
	var err error
	if cfg == nil {
		err = s.c(ctx).row(ctx, `SELECT id FROM test_executions WHERE run_id = ? AND test_case_id = ? ORDER BY position`, runID, testID).Scan(&eid)
	} else {
		err = s.c(ctx).row(ctx, `SELECT id FROM test_executions WHERE run_id = ? AND test_case_id = ? AND configuration_id = ?`, runID, testID, *cfg).Scan(&eid)
	}
	return eid, mapErr(err, "execution")
}

// LastResults returns test id → latest non-not_run execution status.
func (s *Store) LastResults(ctx context.Context, tenantID, projectID string) (map[string]string, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT e.test_case_id, e.status, e.finished_at, e.started_at, r.created_at FROM test_executions e JOIN test_runs r ON r.id = e.run_id
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

// DeleteExecutions removes executions from a run (with their step results,
// attachments and defect links) and completes the run when every remaining
// execution has a final result. It returns the number removed.
func (s *Store) DeleteExecutions(ctx context.Context, tenantID, runID string, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	n := 0
	err := s.tx(ctx, func(c conn) error {
		args := append([]any{runID, tenantID}, anySlice(ids)...)
		rows, err := c.query(ctx, `SELECT id FROM test_executions WHERE run_id = ? AND tenant_id = ? AND id IN (`+placeholders(len(ids))+`)`, args...)
		if err != nil {
			return err
		}
		found, err := scanStrings(rows)
		if err != nil {
			return err
		}
		if len(found) != len(uniq(ids)) {
			return mapErr(sql.ErrNoRows, "execution")
		}
		if err := removeExecutionAttachments(ctx, c, found); err != nil {
			return err
		}
		if err := deleteDefectLinks(ctx, c, "execution", found); err != nil {
			return err
		}
		res, err := c.exec(ctx, `DELETE FROM test_executions WHERE id IN (`+placeholders(len(found))+`)`, anySlice(found)...)
		if err != nil {
			return err
		}
		m, _ := res.RowsAffected()
		n = int(m)
		return syncRunStatus(ctx, c, runID, s.now(), false)
	})
	return n, err
}
