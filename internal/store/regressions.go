package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/model"
)

// Regressions: when a test that verified the fix of a defect (it passed
// after the defect was resolved or closed) fails again, the defect is
// reopened or a linked regression defect is created (the project's
// regression_action), and the regression is recorded.

// defectFixed: resolved or closed (rejected defects were never fixed).
func defectFixed(status string) bool { return status == "resolved" || status == "closed" }

// RegressionOutcome is one regression found by DetectRegressions.
type RegressionOutcome struct {
	Regression model.DefectRegression
	// Defect is the reopened defect (reopened) or the original one (created).
	Defect *model.Defect
	// Created is the new regression defect (created).
	Created *model.Defect
}

// DetectRegressions handles a failed execution: every resolved or closed
// defect linked to its test (or to an execution of the test) that the test
// verified — it passed in another execution after the defect was resolved —
// is reopened or gets a regression defect, once per execution and defect.
func (s *Store) DetectRegressions(ctx context.Context, tenantID string, e *model.Execution, user string) ([]RegressionOutcome, error) {
	if e == nil || e.Status != model.ResultFailed {
		return nil, nil
	}
	t, err := s.GetTest(ctx, tenantID, e.TestCaseID)
	if err != nil {
		return nil, err
	}
	set, err := s.CorrelationSettings(ctx, tenantID, t.ProjectID)
	if err != nil || set.RegressionAction == model.RegressionOff {
		return nil, err
	}
	linked, err := s.DefectsOf(ctx, tenantID, model.KindTest, t.ID)
	if err != nil {
		return nil, err
	}
	failedAt := s.now()
	if e.FinishedAt != nil {
		failedAt = *e.FinishedAt
	}
	var out []RegressionOutcome
	for _, ld := range linked {
		d := ld.Defect
		if !defectFixed(d.Status) {
			continue
		}
		verified, err := s.passedBetween(ctx, t.ID, e.ID, d.ResolvedAt, failedAt)
		if err != nil {
			return out, err
		}
		if !verified {
			continue
		}
		var n int
		if err := s.c(ctx).row(ctx, `SELECT COUNT(*) FROM defect_regressions WHERE defect_id = ? AND execution_id = ?`, d.ID, e.ID).Scan(&n); err != nil {
			return out, err
		}
		if n > 0 {
			continue
		}
		reg := model.DefectRegression{ID: newID(), ProjectID: t.ProjectID, DefectID: d.ID, DefectKey: d.Key, TestCaseID: t.ID, TestKey: t.Key,
			ExecutionID: e.ID, RunID: e.RunID, PreviousStatus: d.Status, CreatedBy: user, CreatedAt: s.now()}
		oc := RegressionOutcome{}
		switch set.RegressionAction {
		case model.RegressionNew:
			nd := &model.Defect{ProjectID: t.ProjectID, Title: regressionTitle(d.Title), Status: "open", Severity: d.Severity, Priority: d.Priority, Assignee: d.Assignee,
				Description: fmt.Sprintf("Regression of %s: %s failed again after the fix was verified.\n\n%s", d.Key, t.Key, d.Description), CreatedBy: user,
				Links: []model.DefectLink{{EntityType: model.KindTest, EntityID: t.ID}, {EntityType: "execution", EntityID: e.ID}}}
			if err := s.CreateDefect(ctx, tenantID, nd); err != nil {
				return out, err
			}
			reg.Action, reg.RegressionDefectID, reg.RegressionDefectKey = "created", &nd.ID, nd.Key
			oc.Created = nd
			dc := d
			oc.Defect = &dc
		default:
			d.Status = "open"
			d.ResolvedAt = nil
			if err := s.UpdateDefect(ctx, tenantID, &d); err != nil {
				return out, err
			}
			if err := s.LinkDefect(ctx, tenantID, d.ID, model.DefectLink{EntityType: "execution", EntityID: e.ID}); err != nil {
				return out, err
			}
			reg.Action = "reopened"
			dc := d
			oc.Defect = &dc
		}
		if _, err := s.c(ctx).exec(ctx, `INSERT INTO defect_regressions (id, tenant_id, project_id, defect_id, regression_defect_id, test_case_id, execution_id, action, previous_status, created_by, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, reg.ID, tenantID, reg.ProjectID, reg.DefectID, nullStr(reg.RegressionDefectID), reg.TestCaseID, reg.ExecutionID,
			reg.Action, reg.PreviousStatus, reg.CreatedBy, reg.CreatedAt); err != nil {
			return out, err
		}
		oc.Regression = reg
		out = append(out, oc)
	}
	return out, nil
}

func regressionTitle(title string) string {
	t := "Regression: " + title
	if utf8.RuneCountInString(t) > 300 {
		t = string([]rune(t)[:300])
	}
	return t
}

// passedBetween reports whether the test passed in an execution other than
// except, finished after since (any time when since is nil) and not after
// until (the failure).
func (s *Store) passedBetween(ctx context.Context, testID, except string, since *time.Time, until time.Time) (bool, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT e.finished_at, e.started_at, r.created_at FROM test_executions e JOIN test_runs r ON r.id = e.run_id
		WHERE e.test_case_id = ? AND e.id <> ? AND e.status = 'passed'`, testID, except)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var fin, start sql.NullTime
		var at time.Time
		if err := rows.Scan(&fin, &start, &at); err != nil {
			return false, err
		}
		if start.Valid {
			at = start.Time
		}
		if fin.Valid {
			at = fin.Time
		}
		if (since == nil || !at.Before(*since)) && !at.After(until) {
			return true, nil
		}
	}
	return false, rows.Err()
}

// ListRegressions returns the regressions of a defect (as the original or
// the regression defect) or of a test, newest first.
func (s *Store) ListRegressions(ctx context.Context, tenantID, entityType, entityID string) ([]model.DefectRegression, error) {
	q := `SELECT g.id, g.project_id, g.defect_id, d.key, g.regression_defect_id, COALESCE(nd.key, ''), g.test_case_id, t.key, g.execution_id,
		COALESCE(e.run_id, ''), COALESCE(r.name, ''), g.action, g.previous_status, g.created_by, g.created_at
		FROM defect_regressions g JOIN defects d ON d.id = g.defect_id JOIN test_cases t ON t.id = g.test_case_id
		LEFT JOIN defects nd ON nd.id = g.regression_defect_id
		LEFT JOIN test_executions e ON e.id = g.execution_id LEFT JOIN test_runs r ON r.id = e.run_id
		WHERE g.tenant_id = ? AND `
	args := []any{tenantID}
	switch entityType {
	case model.KindDefect:
		q += `(g.defect_id = ? OR g.regression_defect_id = ?)`
		args = append(args, entityID, entityID)
	case model.KindTest:
		q += `g.test_case_id = ?`
		args = append(args, entityID)
	default:
		return nil, Invalidf("regressions of a defect or a test")
	}
	rows, err := s.c(ctx).query(ctx, q+` ORDER BY g.created_at DESC, g.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.DefectRegression{}
	for rows.Next() {
		var g model.DefectRegression
		var nd sql.NullString
		if err := rows.Scan(&g.ID, &g.ProjectID, &g.DefectID, &g.DefectKey, &nd, &g.RegressionDefectKey, &g.TestCaseID, &g.TestKey, &g.ExecutionID,
			&g.RunID, &g.RunName, &g.Action, &g.PreviousStatus, &g.CreatedBy, &g.CreatedAt); err != nil {
			return nil, err
		}
		g.RegressionDefectID = strPtr(nd)
		out = append(out, g)
	}
	return out, rows.Err()
}

// RegressionCounts returns the number of regressions per test of a project.
func (s *Store) RegressionCounts(ctx context.Context, tenantID, projectID string) (map[string]int, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT test_case_id FROM defect_regressions WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	ids, err := scanStrings(rows)
	out := map[string]int{}
	for _, id := range ids {
		out[id]++
	}
	return out, err
}
