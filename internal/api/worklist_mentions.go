package api

import (
	"context"

	"github.com/rom/qtestframe/internal/rbac"
)

func init() {
	registerWorkSource("mentions", sourceMentions)
}

// sourceMentions: unresolved discussion threads that mention the caller.
// The row is the item the thread is on (so it merges with an owner row of
// the same item); the reason is "mentioned", the detail who wrote last.
func sourceMentions(ctx context.Context, w *WorkContext) ([]WorkItem, error) {
	if w.Principal.User == nil {
		return nil, nil
	}
	var projects []string
	if w.ProjectID != "" {
		projects = []string{w.ProjectID}
	}
	threads, err := w.Server.Store.OpenThreadsMentioning(ctx, w.Principal.TenantID(), w.Principal.User.ID, projects)
	if err != nil {
		return nil, err
	}
	var out []WorkItem
	seen := map[string]bool{}
	for _, th := range threads {
		c := th.Root
		if !w.Sees(c.ProjectID, rbac.ProjectRead) || seen[c.EntityType+"/"+c.EntityID] {
			continue
		}
		seen[c.EntityType+"/"+c.EntityID] = true
		out = append(out, WorkItem{Kind: c.EntityType, ID: c.EntityID, EntityType: c.EntityType, EntityID: c.EntityID,
			Key: th.EntityKey, Title: th.EntityTitle, ProjectID: c.ProjectID, UpdatedAt: th.LastAt,
			Reasons: []string{"mentioned"}, Detail: th.LastBy})
	}
	return out, nil
}
