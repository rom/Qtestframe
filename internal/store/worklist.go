package store

import (
	"context"
	"strings"
	"time"
)

// Queries behind the personal work list ("My work", internal/api/worklist.go):
// what a user owns or is assigned to across the projects of a tenant.
// People are matched by the names given (lower case: e-mail address,
// display name, handles), the way owners and assignees are stored.

// WorkRow is an item found for the work list.
type WorkRow struct {
	// Kind: requirement | test | risk | defect | execution | waiver.
	Kind      string
	ID        string
	ProjectID string
	Key       string
	Title     string
	Status    string
	UpdatedAt time.Time
	// RunID / RunName / RunStatus: the run of an execution.
	RunID, RunName, RunStatus string
	// Due: a date (YYYY-MM-DD) by which something is due ("" none): the
	// next review of a risk.
	Due string
	// ReviewOwner of a risk; Owner of the item (risks: the owner, so that
	// a risk found by its review owner tells whether it is also owned).
	Owner, ReviewOwner string
}

func lowerArgs(names []string) []any {
	out := make([]any, 0, len(names))
	seen := map[string]bool{}
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

func (s *Store) workRows(ctx context.Context, q string, args []any, scan func(*WorkRow) []any) ([]WorkRow, error) {
	rows, err := s.c(ctx).query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkRow{}
	for rows.Next() {
		var w WorkRow
		if err := rows.Scan(scan(&w)...); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// OwnedItems returns the requirements, tests and risks a person owns (any
// status), and the risks whose next review they own.
func (s *Store) OwnedItems(ctx context.Context, tenantID string, names []string) ([]WorkRow, error) {
	who := lowerArgs(names)
	if len(who) == 0 {
		return []WorkRow{}, nil
	}
	in := placeholders(len(who))
	args := []any{tenantID}
	args = append(args, who...)
	args = append(args, tenantID)
	args = append(args, who...)
	args = append(args, tenantID)
	args = append(args, who...)
	args = append(args, who...)
	return s.workRows(ctx, `
		SELECT 'requirement', id, project_id, key, title, status, updated_at, '', owner, '' FROM requirements
		WHERE tenant_id = ? AND lower(owner) IN (`+in+`)
		UNION ALL
		SELECT 'test', id, project_id, key, title, status, updated_at, '', owner, '' FROM test_cases
		WHERE tenant_id = ? AND lower(owner) IN (`+in+`)
		UNION ALL
		SELECT 'risk', id, project_id, key, title, status, updated_at, review_date, owner, review_owner FROM risks
		WHERE tenant_id = ? AND (lower(owner) IN (`+in+`) OR lower(review_owner) IN (`+in+`))`, args,
		func(w *WorkRow) []any {
			return []any{&w.Kind, &w.ID, &w.ProjectID, &w.Key, &w.Title, &w.Status, &w.UpdatedAt, &w.Due, &w.Owner, &w.ReviewOwner}
		})
}

// AssignedDefects returns the defects assigned to a person in the given
// statuses.
func (s *Store) AssignedDefects(ctx context.Context, tenantID string, names, statuses []string) ([]WorkRow, error) {
	who := lowerArgs(names)
	if len(who) == 0 || len(statuses) == 0 {
		return []WorkRow{}, nil
	}
	args := []any{tenantID}
	args = append(args, who...)
	for _, st := range statuses {
		args = append(args, st)
	}
	return s.workRows(ctx, `SELECT 'defect', id, project_id, key, title, status, updated_at FROM defects
		WHERE tenant_id = ? AND lower(assignee) IN (`+placeholders(len(who))+`) AND status IN (`+placeholders(len(statuses))+`)`, args,
		func(w *WorkRow) []any {
			return []any{&w.Kind, &w.ID, &w.ProjectID, &w.Key, &w.Title, &w.Status, &w.UpdatedAt}
		})
}

// AssignedExecutions returns the executions assigned to a person that are
// not executed yet (not run or in progress) in runs that are planned or in
// progress.
func (s *Store) AssignedExecutions(ctx context.Context, tenantID string, names []string) ([]WorkRow, error) {
	who := lowerArgs(names)
	if len(who) == 0 {
		return []WorkRow{}, nil
	}
	args := []any{tenantID}
	args = append(args, who...)
	return s.workRows(ctx, `SELECT 'execution', e.id, r.project_id, t.key, t.title, e.status, r.created_at, r.id, r.name, r.status
		FROM test_executions e JOIN test_runs r ON r.id = e.run_id JOIN test_cases t ON t.id = e.test_case_id
		WHERE e.tenant_id = ? AND lower(e.assignee) IN (`+placeholders(len(who))+`)
		AND e.status IN ('not_run', 'in_progress') AND r.status IN ('planned', 'in_progress')
		ORDER BY r.created_at, e.position`, args,
		func(w *WorkRow) []any {
			return []any{&w.Kind, &w.ID, &w.ProjectID, &w.Key, &w.Title, &w.Status, &w.UpdatedAt, &w.RunID, &w.RunName, &w.RunStatus}
		})
}

// RequestedWaiverIDs returns the waivers of a tenant waiting for decisions
// (status requested).
func (s *Store) RequestedWaiverIDs(ctx context.Context, tenantID string) ([]string, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id FROM waivers WHERE tenant_id = ? AND status = 'requested' ORDER BY updated_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// RiskAcceptanceProjects returns the projects of a tenant with risks that
// may wait for an acceptance signature (mitigated or accepted ones).
func (s *Store) RiskAcceptanceProjects(ctx context.Context, tenantID string) ([]string, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT DISTINCT project_id FROM risks WHERE tenant_id = ? AND status IN ('mitigated', 'accepted')`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
