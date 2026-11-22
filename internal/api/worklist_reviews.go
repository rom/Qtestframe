package api

import (
	"context"
	"slices"

	"github.com/rom/qtestframe/internal/store"
)

func init() {
	registerWorkSource("requested_reviews", sourceRequestedReviews)
	registerWorkSource("in_review", sourceInReview)
}

// sourceRequestedReviews: review requests the caller asked for. A pending
// one is "awaiting_review" (it waits for the reviewer); the latest request
// of a reviewer that came back with changes requested is "review_returned"
// while the item has not changed since (editing the item, or a newer
// request to that reviewer, answers it). Approved and cancelled requests
// are left out. Seeing the project is enough: they are the caller's own.
func sourceRequestedReviews(ctx context.Context, w *WorkContext) ([]WorkItem, error) {
	p := w.Principal
	if p.User == nil || p.User.Email == "" {
		return nil, nil
	}
	list, err := w.Server.Store.ListReviewRequests(ctx, p.TenantID(), store.ReviewFilter{RequestedBy: p.User.Email, ProjectID: w.ProjectID})
	if err != nil {
		return nil, err
	}
	// newest first: the first request of an item and reviewer is the latest
	seen := map[string]bool{}
	var out []WorkItem
	for _, v := range list {
		k := v.EntityType + ":" + v.EntityID + ":" + v.ReviewerID
		if seen[k] {
			continue
		}
		seen[k] = true
		var reason string
		switch {
		case v.Status == "pending":
			reason = "awaiting_review"
		case v.Status == "changes_requested" && !v.Outdated:
			reason = "review_returned"
		default:
			continue
		}
		if !w.Sees(v.ProjectID, "") {
			continue
		}
		who := v.ReviewerName
		if who == "" {
			who = v.ReviewerEmail
		}
		out = append(out, WorkItem{Kind: v.EntityType, ID: v.EntityID, EntityType: v.EntityType, EntityID: v.EntityID, Key: v.EntityKey, Title: v.EntityTitle,
			ProjectID: v.ProjectID, UpdatedAt: v.UpdatedAt, Reasons: []string{reason}, Detail: who, RequestedReviewIDs: []string{v.ID}})
	}
	return out, nil
}

// sourceInReview: requirements and tests in status "review" that concern
// the caller personally: they own or wrote it, or were asked to review it
// (as GET /reviews/in-review, without the items listed only because the
// caller may review items of that kind). The reason is "in_review".
func sourceInReview(ctx context.Context, w *WorkContext) ([]WorkItem, error) {
	if w.Principal.User == nil {
		return nil, nil
	}
	items, err := w.Server.inReviewFor(ctx, w.Principal, w.Visible)
	if err != nil {
		return nil, err
	}
	var out []WorkItem
	for _, it := range items {
		if !w.Sees(it.ProjectID, readPermOf(it.EntityType)) || !slices.ContainsFunc(it.Reasons, func(r string) bool { return r != "reviewer" }) {
			continue
		}
		row := WorkItem{Kind: it.EntityType, ID: it.EntityID, EntityType: it.EntityType, EntityID: it.EntityID, Key: it.Key, Title: it.Title,
			Status: "review", ProjectID: it.ProjectID, UpdatedAt: it.UpdatedAt, Reasons: []string{"in_review"}}
		if it.RequestID != "" {
			row.ReviewIDs = []string{it.RequestID}
		}
		out = append(out, row)
	}
	return out, nil
}
