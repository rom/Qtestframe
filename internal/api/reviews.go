package api

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Review workflow: review requests to people with a checklist, and
// electronic sign-offs of baselines.

func (s *Server) routesReviews(h routeFunc) {
	h("GET /api/v1/review-checklists", s.handleReviewChecklists)
	h("GET /api/v1/reviews", s.handleListReviews)
	h("POST /api/v1/reviews", s.handleCreateReviews)
	h("POST /api/v1/reviews/bulk", s.handleBulkReviewRequests)
	h("GET /api/v1/reviews/{id}", s.handleGetReview)
	h("PATCH /api/v1/reviews/{id}", s.handleUpdateReview)
	h("GET /api/v1/baselines/{id}/signoffs", s.handleListSignoffs, "requirement.read")
	h("POST /api/v1/baselines/{id}/signoffs", s.handleSignBaseline, "requirement.read")
}

func (s *Server) handleReviewChecklists(w http.ResponseWriter, r *http.Request, p *Principal) {
	writeJSON(w, http.StatusOK, model.DefaultReviewChecklists)
}

func readPermOf(kind string) rbac.Permission {
	if kind == model.KindTest {
		return rbac.Permission("test.read")
	}
	return rbac.Permission("requirement.read")
}

func writePermOf(kind string) rbac.Permission {
	if kind == model.KindTest {
		return rbac.Permission("test.write")
	}
	return rbac.Permission("requirement.write")
}

// handleListReviews filters by project_id, entity_type, entity_id, status
// and reviewer ("me" = the caller). Without a project or item it lists the
// caller's own requests.
func (s *Server) handleListReviews(w http.ResponseWriter, r *http.Request, p *Principal) {
	q := r.URL.Query()
	f := store.ReviewFilter{ProjectID: q.Get("project_id"), EntityType: q.Get("entity_type"), EntityID: q.Get("entity_id"), Status: q.Get("status"), ReviewerID: q.Get("reviewer")}
	if q.Get("requested_by") == "me" {
		// the caller's own requests, across projects
		f.RequestedBy = p.User.Email
		if f.ReviewerID == "me" {
			f.ReviewerID = p.User.ID
		}
		list, err := s.Store.ListReviewRequests(r.Context(), p.TenantID(), f)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, listOf(list))
		return
	}
	if f.ReviewerID == "me" || (f.ProjectID == "" && f.EntityID == "") {
		f.ReviewerID = p.User.ID
	}
	if f.ReviewerID != p.User.ID && !p.Can(readPermOf(f.EntityType)) {
		writeError(w, http.StatusForbidden, "forbidden", "permission "+string(readPermOf(f.EntityType))+" required")
		return
	}
	list, err := s.Store.ListReviewRequests(r.Context(), p.TenantID(), f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

// handleCreateReviews: {"entity_type", "entity_id", "reviewers": [user id,
// e-mail or @handle], "message", "checklist": [{text}] (default per kind),
// "due_date"}. Needs write permission on the item.
func (s *Server) handleCreateReviews(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		EntityType string                `json:"entity_type"`
		EntityID   string                `json:"entity_id"`
		Reviewers  []string              `json:"reviewers"`
		Message    string                `json:"message"`
		Checklist  []model.ChecklistItem `json:"checklist"`
		DueDate    string                `json:"due_date"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.EntityType != model.KindRequirement && req.EntityType != model.KindTest {
		s.fail(w, r, badRequest("entity_type must be requirement or test"))
		return
	}
	if !p.Can(writePermOf(req.EntityType)) {
		writeError(w, http.StatusForbidden, "forbidden", "permission "+string(writePermOf(req.EntityType))+" required")
		return
	}
	ctx := r.Context()
	if req.Checklist == nil {
		for _, t := range model.DefaultReviewChecklists[req.EntityType] {
			req.Checklist = append(req.Checklist, model.ChecklistItem{Text: t})
		}
	}
	for i := range req.Checklist {
		req.Checklist[i].Checked, req.Checklist[i].Note = false, ""
	}
	ids := s.resolveUsers(r, p, req.Reviewers)
	if len(ids) < len(req.Reviewers) {
		s.fail(w, r, badRequest("every reviewer must be a member of the organisation"))
		return
	}
	created, err := s.Store.CreateReviewRequests(ctx, p.TenantID(), model.ReviewRequest{EntityType: req.EntityType, EntityID: req.EntityID,
		RequestedBy: p.User.Email, Message: req.Message, Checklist: req.Checklist, DueDate: req.DueDate}, ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, v := range created {
		s.audit(ctx, p, "review.request", v.EntityType, v.EntityID, v.EntityKey+" → "+v.ReviewerEmail)
		title := v.EntityKey + " " + v.EntityTitle
		s.notifyUsers(ctx, p, []string{v.ReviewerID}, model.Notification{ProjectID: v.ProjectID, Event: "review.requested", Title: title,
			Body: excerpt(v.Message, 300), EntityType: v.EntityType, EntityID: v.EntityID})
		s.notifyEvent(r, p, v.ProjectID, "review.requested", map[string]any{"review": v})
	}
	writeJSON(w, http.StatusCreated, listOf(created))
}

// resolveUsers maps user IDs, e-mail addresses or @handles to user IDs
// (the caller included).
func (s *Server) resolveUsers(r *http.Request, p *Principal, people []string) []string {
	var out []string
	for _, v := range people {
		if strings.TrimSpace(v) == "" {
			continue
		}
		if strings.EqualFold(v, p.User.ID) || strings.EqualFold(v, p.User.Email) {
			out = append(out, p.User.ID)
			continue
		}
		if ids := s.recipients(r.Context(), p, v); len(ids) == 1 {
			out = append(out, ids[0])
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func (s *Server) review(w http.ResponseWriter, r *http.Request, p *Principal) (*model.ReviewRequest, bool) {
	v, err := s.Store.GetReviewRequest(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	if v.ReviewerID != p.User.ID && !p.Can(readPermOf(v.EntityType)) {
		writeError(w, http.StatusNotFound, "not_found", "review request not found")
		return nil, false
	}
	return v, true
}

func (s *Server) handleGetReview(w http.ResponseWriter, r *http.Request, p *Principal) {
	if v, ok := s.review(w, r, p); ok {
		writeJSON(w, http.StatusOK, v)
	}
}

// handleUpdateReview: the reviewer saves the checklist and comment and
// decides ("decision": "approved" or "changes_requested"); the requester
// (or someone with write permission on the item) can cancel ("cancel": true).
// Approving needs every checklist item checked or a note saying why not.
func (s *Server) handleUpdateReview(w http.ResponseWriter, r *http.Request, p *Principal) {
	v, ok := s.review(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Checklist *[]model.ChecklistItem `json:"checklist"`
		Comment   *string                `json:"comment"`
		Decision  string                 `json:"decision"`
		Cancel    bool                   `json:"cancel"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if v.Status != "pending" {
		s.fail(w, r, store.Conflictf("the review is already %s", v.Status))
		return
	}
	ctx := r.Context()
	if req.Cancel {
		if v.RequestedBy != p.User.Email && !p.Can(writePermOf(v.EntityType)) {
			writeError(w, http.StatusForbidden, "forbidden", "only the requester can cancel a review")
			return
		}
		v.Status = "cancelled"
	} else {
		if v.ReviewerID != p.User.ID {
			writeError(w, http.StatusForbidden, "forbidden", "only the reviewer can work on this review")
			return
		}
		if req.Checklist != nil {
			// the reviewer ticks items; texts stay those of the request
			if len(*req.Checklist) != len(v.Checklist) {
				s.fail(w, r, badRequest("the checklist has %d items", len(v.Checklist)))
				return
			}
			for i, c := range *req.Checklist {
				v.Checklist[i].Checked, v.Checklist[i].Note = c.Checked, strings.TrimSpace(c.Note)
			}
		}
		if req.Comment != nil {
			v.Comment = *req.Comment
		}
		if req.Decision != "" {
			if !slices.Contains(model.ReviewDecisions, req.Decision) {
				s.fail(w, r, badRequest("decision must be approved or changes_requested"))
				return
			}
			if req.Decision == "approved" {
				for _, c := range v.Checklist {
					if !c.Checked && c.Note == "" {
						s.fail(w, r, badRequest("check %q or add a note saying why it does not apply", c.Text))
						return
					}
				}
			} else if strings.TrimSpace(v.Comment) == "" {
				s.fail(w, r, badRequest("say which changes are needed in the comment"))
				return
			}
			v.Status = req.Decision
		}
	}
	if err := s.Store.UpdateReviewRequest(ctx, p.TenantID(), v); err != nil {
		s.fail(w, r, err)
		return
	}
	if v.Status != "pending" && v.ReviewerID == p.User.ID {
		_ = s.Store.MarkEntityNotificationsRead(ctx, p.TenantID(), p.User.ID, "review.requested", []string{v.EntityID})
	}
	if v.Status != "pending" {
		s.audit(ctx, p, "review."+v.Status, v.EntityType, v.EntityID, v.EntityKey+" ("+v.ReviewerEmail+")")
		if v.Status != "cancelled" {
			s.notifyUsers(ctx, p, s.recipients(ctx, p, v.RequestedBy), model.Notification{ProjectID: v.ProjectID, Event: "review.completed",
				Title: v.EntityKey + " " + v.EntityTitle + ": " + v.Status, Body: excerpt(v.Comment, 300), EntityType: v.EntityType, EntityID: v.EntityID})
			s.notifyEvent(r, p, v.ProjectID, "review.completed", map[string]any{"review": v})
		}
	}
	out, err := s.Store.GetReviewRequest(ctx, p.TenantID(), v.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------- sign-offs

func (s *Server) handleListSignoffs(w http.ResponseWriter, r *http.Request, p *Principal) {
	list, err := s.Store.ListSignoffs(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

// handleSignBaseline records an electronic signature: {"meaning",
// "comment", "password"}. The signer must have a review permission and
// confirm their identity: with their password, or for single sign-on
// accounts with a sign-in less than 15 minutes old. API tokens cannot sign.
func (s *Server) handleSignBaseline(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req signRequest
	if !s.checkSigner(w, r, p, &req) {
		return
	}
	so, err := s.signBaseline(r, p, r.PathValue("id"), req)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, so)
}

type signRequest struct {
	Meaning  string `json:"meaning"`
	Comment  string `json:"comment"`
	Password string `json:"password"`
	// IDs: the baselines of a bulk sign-off.
	IDs []string `json:"ids"`
}

// checkSigner decodes the request and confirms that the caller may sign:
// a browser session, a review permission and their password (or a recent
// single sign-on). It writes the error and returns false otherwise.
func (s *Server) checkSigner(w http.ResponseWriter, r *http.Request, p *Principal, req *signRequest) bool {
	if p.Token != nil || p.Session == nil {
		writeError(w, http.StatusForbidden, "forbidden", "baselines are signed in the browser, not with an API token")
		return false
	}
	if !p.Can(rbac.RequirementRev) && !p.Can(rbac.TestReview) {
		writeError(w, http.StatusForbidden, "forbidden", "permission requirement.review or test.review required")
		return false
	}
	if err := decode(r, req); err != nil {
		s.fail(w, r, err)
		return false
	}
	if p.User.PasswordHash != "" {
		if !auth.CheckPassword(p.User.PasswordHash, req.Password) {
			writeError(w, http.StatusForbidden, "reauth_failed", "the password is not correct")
			return false
		}
	} else if time.Since(p.Session.CreatedAt) > 15*time.Minute {
		writeError(w, http.StatusForbidden, "reauth_required", "sign in again (less than 15 minutes ago) to sign")
		return false
	}
	return true
}

// signBaseline records the caller's signature on one baseline.
func (s *Server) signBaseline(r *http.Request, p *Principal, baselineID string, req signRequest) (*model.Signoff, error) {
	name := p.User.DisplayName
	if name == "" {
		name = p.User.Email
	}
	so := &model.Signoff{BaselineID: baselineID, UserID: p.User.ID, UserEmail: p.User.Email, UserName: name, Meaning: req.Meaning, Comment: strings.TrimSpace(req.Comment)}
	if err := s.Store.SignBaseline(r.Context(), p.TenantID(), so); err != nil {
		return nil, err
	}
	s.audit(r.Context(), p, "baseline.signed", "baseline", so.BaselineID, so.Meaning+" "+so.ContentHash[:12])
	if b, err := s.Store.GetBaseline(r.Context(), p.TenantID(), so.BaselineID); err == nil {
		s.notifyEvent(r, p, b.ProjectID, "baseline.signed", map[string]any{"baseline_id": b.ID, "baseline": b.Name, "signoff": so})
	}
	return so, nil
}

// handleBulkReviewRequests: POST /reviews/bulk
// {"ids": [...], "action", "comment", "due_date"}. Actions:
//   - approve, request_changes: the caller's own pending requests (the
//     checklist is not checked; the comment records the bulk decision;
//     request_changes needs a comment);
//   - cancel, remind, due_date: pending requests the caller asked for, or
//     with write permission on the item.
//
// Requests that do not qualify are skipped with a reason.
func (s *Server) handleBulkReviewRequests(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		IDs     []string `json:"ids"`
		Action  string   `json:"action"`
		Comment string   `json:"comment"`
		DueDate string   `json:"due_date"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	actions := []string{"approve", "request_changes", "cancel", "remind", "due_date"}
	if !slices.Contains(actions, req.Action) {
		s.fail(w, r, badRequest("action must be one of %s", strings.Join(actions, ", ")))
		return
	}
	comment := strings.TrimSpace(req.Comment)
	if req.Action == "request_changes" && comment == "" {
		s.fail(w, r, badRequest("say which changes are needed in the comment"))
		return
	}
	if req.Action == "due_date" && strings.TrimSpace(req.DueDate) != "" && !store.ValidDate(strings.TrimSpace(req.DueDate)) {
		s.fail(w, r, badRequest("due_date must be YYYY-MM-DD"))
		return
	}
	ids, err := bulkIDs(req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	type item struct {
		ID     string `json:"id"`
		Key    string `json:"key"`
		Reason string `json:"reason,omitempty"`
	}
	updated, skipped := []item{}, []item{}
	var mine []model.ReviewRequest
	for _, id := range ids {
		v, err := s.Store.GetReviewRequest(ctx, p.TenantID(), id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		own := v.ReviewerID == p.User.ID
		manage := v.RequestedBy == p.User.Email || p.Can(writePermOf(v.EntityType))
		switch {
		case !own && !manage && !p.Can(readPermOf(v.EntityType)):
			s.fail(w, r, fmt.Errorf("review request %s: %w", id, store.ErrNotFound))
			return
		case v.Status != "pending":
			skipped = append(skipped, item{v.ID, v.EntityKey, "not_pending"})
			continue
		case (req.Action == "approve" || req.Action == "request_changes") && !own:
			skipped = append(skipped, item{v.ID, v.EntityKey, "not_reviewer"})
			continue
		case req.Action != "approve" && req.Action != "request_changes" && !manage:
			skipped = append(skipped, item{v.ID, v.EntityKey, "not_requester"})
			continue
		}
		switch req.Action {
		case "approve", "request_changes":
			mine = append(mine, *v)
			continue
		case "cancel":
			v.Status = "cancelled"
			if err := s.Store.UpdateReviewRequest(ctx, p.TenantID(), v); err != nil {
				s.fail(w, r, err)
				return
			}
			s.audit(ctx, p, "review.cancelled", v.EntityType, v.EntityID, v.EntityKey+" ("+v.ReviewerEmail+", bulk)")
		case "remind":
			s.notifyUsers(ctx, p, []string{v.ReviewerID}, model.Notification{ProjectID: v.ProjectID, Event: "review.reminder",
				Title: v.EntityKey + " " + v.EntityTitle, Body: excerpt(comment, 300), EntityType: v.EntityType, EntityID: v.EntityID})
			s.audit(ctx, p, "review.remind", v.EntityType, v.EntityID, v.EntityKey+" ("+v.ReviewerEmail+")")
		case "due_date":
			if err := s.Store.SetReviewDueDate(ctx, p.TenantID(), v.ID, req.DueDate); err != nil {
				s.fail(w, r, err)
				return
			}
			s.audit(ctx, p, "review.due_date", v.EntityType, v.EntityID, v.EntityKey+" "+req.DueDate)
		}
		updated = append(updated, item{ID: v.ID, Key: v.EntityKey})
	}
	if len(mine) > 0 {
		status, note := "approved", "Approved in a bulk review."
		if req.Action == "request_changes" {
			status, note = "changes_requested", "Changes requested in a bulk review."
		}
		if comment != "" {
			note += " " + comment
		}
		s.completeReviews(r, p, mine, status, note)
		for _, v := range mine {
			updated = append(updated, item{ID: v.ID, Key: v.EntityKey})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": updated, "skipped": skipped})
}
