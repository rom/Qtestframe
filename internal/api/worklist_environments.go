package api

import (
	"context"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

func init() {
	registerWorkSource("environments", sourceEnvironments)
	registerWorkSource("test_data", sourceTestData)
}

// sourceEnvironments: bookings waiting for the caller's approval (the
// caller owns or manages the environment; reason "booking_approval") and
// the caller's own bookings still waiting (reason "awaiting_approval").
func sourceEnvironments(ctx context.Context, w *WorkContext) ([]WorkItem, error) {
	if w.Principal.User == nil {
		return nil, nil
	}
	list, err := w.Server.Store.PendingBookings(ctx, w.Principal.TenantID())
	if err != nil || len(list) == 0 {
		return nil, err
	}
	envs := map[string]*model.Environment{}
	var out []WorkItem
	for _, b := range list {
		e, ok := envs[b.EnvironmentID]
		if !ok {
			if e, err = w.Server.Store.GetEnvironment(ctx, w.Principal.TenantID(), b.EnvironmentID); err != nil {
				continue
			}
			envs[b.EnvironmentID] = e
		}
		pid := b.ProjectID
		if pid == "" {
			pid = e.ProjectID
		}
		if !w.Sees(pid, rbac.RunRead) {
			continue
		}
		var reasons []string
		if b.UserID == w.Principal.User.ID {
			reasons = append(reasons, "awaiting_approval")
		}
		approver := e.OwnerID == w.Principal.User.ID
		if !approver {
			if e.Shared {
				approver = w.Principal.orgCan(rbac.EnvironmentManage)
			} else {
				approver = visibleIn(w.Visible, w.Principal, e.ProjectID, rbac.EnvironmentManage)
			}
		}
		if approver && b.UserID != w.Principal.User.ID {
			reasons = append(reasons, "booking_approval")
		}
		if len(reasons) == 0 {
			continue
		}
		out = append(out, WorkItem{Kind: model.KindBooking, ID: b.ID, EntityType: model.KindEnvironment, EntityID: e.ID, Key: e.Name,
			Title: bookingTitle(&b), Status: b.Status, ProjectID: pid, UpdatedAt: b.UpdatedAt, Reasons: reasons, Detail: b.BookedBy + nonBlank(": ", b.Purpose),
			Due: b.StartsAt.Format("2006-01-02"), Overdue: b.StartsAt.Format("2006-01-02") < w.Today})
	}
	return out, nil
}

// sourceTestData: data sets the caller owns that expired or expire soon
// (reasons "data_expired", "data_expiring") or have open refresh requests
// ("refresh_requested").
func sourceTestData(ctx context.Context, w *WorkContext) ([]WorkItem, error) {
	if w.Principal.User == nil {
		return nil, nil
	}
	tenant := w.Principal.TenantID()
	sets, err := w.Server.Store.ExpiringDataSets(ctx, tenant)
	if err != nil {
		return nil, err
	}
	reqs, err := w.Server.Store.OpenRefreshRequests(ctx, tenant)
	if err != nil {
		return nil, err
	}
	me := w.Principal.User.ID
	byID := map[string]int{}
	var out []WorkItem
	add := func(d *model.DataSet, reason string) {
		if i, ok := byID[d.ID]; ok {
			out[i].Reasons = appendNew(out[i].Reasons, reason)
			return
		}
		byID[d.ID] = len(out)
		out = append(out, WorkItem{Kind: model.KindDataSet, ID: d.ID, EntityType: model.KindDataSet, EntityID: d.ID, Key: d.Name, Title: d.Name,
			Status: d.Status, ProjectID: d.ProjectID, UpdatedAt: d.UpdatedAt, Reasons: []string{reason}, Due: d.ExpiresOn, Overdue: d.Expired})
	}
	for i := range sets {
		d := &sets[i]
		if d.OwnerID != me || !w.Sees(d.ProjectID, rbac.TestRead) {
			continue
		}
		if d.Expired {
			add(d, "data_expired")
		} else {
			add(d, "data_expiring")
		}
	}
	seen := map[string]bool{}
	for _, q := range reqs {
		if seen[q.DataSetID] || !w.Sees(q.ProjectID, rbac.TestRead) {
			continue
		}
		seen[q.DataSetID] = true
		d, err := w.Server.Store.GetDataSet(ctx, tenant, q.DataSetID)
		if err != nil || d.OwnerID != me {
			continue
		}
		add(d, "refresh_requested")
		i := byID[d.ID]
		if out[i].Detail == "" {
			out[i].Detail = q.RequestedBy + nonBlank(": ", excerpt(q.Reason, 120))
		}
		if q.NeededBy != "" && (out[i].Due == "" || q.NeededBy < out[i].Due) {
			out[i].Due, out[i].Overdue = q.NeededBy, q.NeededBy < w.Today
		}
	}
	return out, nil
}
