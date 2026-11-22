package api

import (
	"context"

	"github.com/rom/qtestframe/internal/rbac"
)

func init() {
	registerWorkSource("cycles", sourceCycles)
}

// sourceCycles: tests of open test cycles assigned to the caller without a
// final result. A test already in one of the cycle's runs is the row of its
// execution (it merges with the executions source); one not in a run yet
// leads to the cycle. The cycle's end date is the due date
// (overdue is set when the rows are merged).
func sourceCycles(ctx context.Context, w *WorkContext) ([]WorkItem, error) {
	if w.Principal.User == nil {
		return nil, nil
	}
	list, err := w.Server.Store.AssignedCycleTests(ctx, w.Principal.TenantID(), w.Principal.User.ID)
	if err != nil {
		return nil, err
	}
	var out []WorkItem
	for _, ct := range list {
		if !w.Sees(ct.ProjectID, rbac.RunRead) {
			continue
		}
		it := WorkItem{Kind: "cycle_test", ID: ct.ID, EntityType: "cycle", EntityID: ct.CycleID, Key: ct.TestKey, Title: ct.TestTitle,
			Status: ct.Status, ProjectID: ct.ProjectID, UpdatedAt: ct.AddedAt, Reasons: []string{"assigned"}, Detail: ct.CycleName, Due: ct.EndDate}
		if ct.ExecutionID != nil && ct.RunID != nil {
			it.Kind, it.ID, it.EntityType, it.EntityID, it.RunID = "execution", *ct.ExecutionID, "execution", *ct.ExecutionID, *ct.RunID
		}
		out = append(out, it)
	}
	return out, nil
}
