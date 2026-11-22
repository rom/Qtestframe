package api

import (
	"context"

	"github.com/rom/qtestframe/internal/rbac"
)

func init() {
	registerWorkSource("exploratory", sourceExploratory)
}

// sourceExploratory: exploratory sessions the caller is the tester of that
// are planned, running or paused (reason "assigned").
func sourceExploratory(ctx context.Context, w *WorkContext) ([]WorkItem, error) {
	if w.Principal.User == nil {
		return nil, nil
	}
	list, err := w.Server.Store.ListUserExploratorySessions(ctx, w.Principal.TenantID(), w.Principal.User.Email)
	if err != nil {
		return nil, err
	}
	var out []WorkItem
	for _, x := range list {
		if !w.Sees(x.ProjectID, rbac.RunRead) {
			continue
		}
		out = append(out, WorkItem{Kind: "exploratory_session", ID: x.ID, EntityType: "exploratory_session", EntityID: x.ID,
			Key: x.Key, Title: x.Title, Status: x.Status, ProjectID: x.ProjectID, UpdatedAt: x.UpdatedAt, Reasons: []string{"assigned"}})
	}
	return out, nil
}
