package store

import (
	"context"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// The import of tests from other test management tools (internal/api/
// testimport.go) records where each test, run and past result came from as
// external references (system "testrail", "xray" …; entity types test, run
// and execution), so that importing the file again updates instead of
// duplicating.

// ListExternalRefsOfType returns the references of a project to items of
// one entity type, in every system.
func (s *Store) ListExternalRefsOfType(ctx context.Context, tenantID, projectID, entityType string) ([]ExternalRef, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+refCols+`
		FROM external_refs WHERE tenant_id = ? AND project_id = ? AND entity_type = ? ORDER BY imported_at, external_id`, tenantID, projectID, entityType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExternalRef{}
	for rows.Next() {
		r, err := scanRef(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// HistoryResult is a past result of a test from another tool.
type HistoryResult struct {
	TestID     string
	Status     string
	ExecutedBy string
	Notes      string
	At         *time.Time
}

// ImportHistory records past results in a run: each test is added to the
// run (once) and its execution gets the result, who executed it, when
// (nil: now) and the notes. The run is completed, started at the earliest
// and completed at the latest result. It returns the execution ids.
func (s *Store) ImportHistory(ctx context.Context, tenantID, runID string, results []HistoryResult) ([]string, error) {
	run, err := s.GetRun(ctx, tenantID, runID)
	if err != nil {
		return nil, err
	}
	var ids []string
	err = s.tx(ctx, func(c conn) error {
		ids = nil
		var first, last time.Time
		for _, h := range results {
			if !finalResult(h.Status) {
				return Invalidf("invalid past result %q", h.Status)
			}
			if _, err := s.addExecutions(ctx, c, tenantID, run.ProjectID, runID, []string{h.TestID}); err != nil {
				return err
			}
			var eid string
			if err := c.row(ctx, `SELECT id FROM test_executions WHERE run_id = ? AND test_case_id = ? ORDER BY position`, runID, h.TestID).Scan(&eid); err != nil {
				return mapErr(err, "execution")
			}
			at := s.now()
			if h.At != nil {
				at = h.At.UTC()
			}
			if first.IsZero() || at.Before(first) {
				first = at
			}
			if at.After(last) {
				last = at
			}
			if _, err := c.exec(ctx, `UPDATE test_executions SET status = ?, executed_by = ?, started_at = ?, finished_at = ?, notes = ? WHERE id = ?`,
				h.Status, h.ExecutedBy, at, at, h.Notes, eid); err != nil {
				return err
			}
			if _, err := c.exec(ctx, `UPDATE step_results SET status = ? WHERE execution_id = ? AND status = ?`, h.Status, eid, model.ResultNotRun); err != nil {
				return err
			}
			ids = append(ids, eid)
		}
		if len(results) == 0 {
			return nil
		}
		_, err := c.exec(ctx, `UPDATE test_runs SET status = 'completed',
			started_at = CASE WHEN started_at IS NULL OR started_at > ? THEN ? ELSE started_at END,
			completed_at = CASE WHEN completed_at IS NULL OR completed_at < ? THEN ? ELSE completed_at END WHERE id = ?`,
			first, first, last, last, runID)
		return err
	})
	return ids, err
}
