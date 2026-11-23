package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Change requests (formal change control, model.ChangeRequest).
//
// Reading needs requirement.read; proposing, editing, submitting,
// assessing, sending to the board, applying, verifying and closing need
// change.create; deciding (an electronic signature, as for waivers) needs
// change.approve and a seat on the project's change control board (a
// listed user or a holder of a listed role; nobody listed: anyone with
// change.approve). The requester cannot decide their own change request.
//
// Change control (the project's setting): with "enforce", requirements and
// tests whose status is protected, or that are in a signed baseline, are
// not changed directly: PATCH, bulk edits, the spreadsheet batch, the
// exchange, restoring a version and deleting answer 409 change_control
// with a hint to propose the change in a change request. Applying an
// approved change request changes them; so does an emergency override
// (?change_override=<justification>, change.approve), which is audited.
// With "warn" the change is saved and the answer carries the header
// X-Change-Control: warn.

func (s *Server) routesChangeRequests(h routeFunc) {
	h("GET /api/v1/projects/{pid}/change-control", s.handleGetChangeControl, "requirement.read")
	h("PUT /api/v1/projects/{pid}/change-control", s.handleSetChangeControl, "project.update")
	h("GET /api/v1/projects/{pid}/change-requests", s.handleListChangeRequests, "requirement.read")
	h("POST /api/v1/projects/{pid}/change-requests", s.handleCreateChangeRequest, "change.create")
	h("GET /api/v1/projects/{pid}/change-requests/open-items", s.handleOpenChangeItems, "requirement.read")
	h("GET /api/v1/change-requests/{id}", s.handleGetChangeRequest, "requirement.read")
	h("PATCH /api/v1/change-requests/{id}", s.handleUpdateChangeRequest, "change.create")
	h("DELETE /api/v1/change-requests/{id}", s.handleDeleteChangeRequest, "change.create")
	h("POST /api/v1/change-requests/{id}/items", s.handlePutChangeRequestItems, "change.create")
	h("DELETE /api/v1/change-requests/{id}/items/{item}", s.handleRemoveChangeRequestItem, "change.create")
	h("GET /api/v1/change-requests/{id}/impact", s.handleChangeRequestImpact, "requirement.read")
	h("POST /api/v1/change-requests/{id}/submit", s.handleSubmitChangeRequest, "change.create")
	h("POST /api/v1/change-requests/{id}/assess", s.handleAssessChangeRequest, "change.create")
	h("POST /api/v1/change-requests/{id}/review", s.handleReviewChangeRequest, "change.create")
	h("POST /api/v1/change-requests/{id}/withdraw", s.handleWithdrawChangeRequest, "change.create")
	h("POST /api/v1/change-requests/{id}/decision", s.handleDecideChangeRequest, "change.approve")
	h("POST /api/v1/change-requests/{id}/apply", s.handleApplyChangeRequest, "change.create")
	h("POST /api/v1/change-requests/{id}/verify", s.handleVerifyChangeRequest, "change.create")
	h("POST /api/v1/change-requests/{id}/close", s.handleCloseChangeRequest, "change.create")
	h("GET /api/v1/requirements/{id}/change-control", s.handleItemChangeControl(model.KindRequirement), "requirement.read")
	h("GET /api/v1/tests/{id}/change-control", s.handleItemChangeControl(model.KindTest), "test.read")
}

// ---------------------------------------------------------------- change control guard

// crApplyKey marks a request context as applying a change request (its key).
type crApplyKey struct{}

// changeBlocked is the answer for a direct change of an item under
// enforced change control (409 change_control).
type changeBlocked struct {
	Kind, ID, Key string
	Reasons       []string
	Open          []string
}

func (e *changeBlocked) Error() string {
	msg := fmt.Sprintf("%s is under change control (%s): propose the change in a change request", e.Key, strings.Join(e.Reasons, "; "))
	if len(e.Open) > 0 {
		msg += " (open: " + strings.Join(e.Open, ", ") + ")"
	}
	return msg
}

// changeControlBody is the JSON of a changeBlocked answer.
type changeControlBody struct {
	Error   string   `json:"error"`
	Code    string   `json:"code"`
	Hint    string   `json:"hint"`
	Kind    string   `json:"kind"`
	ID      string   `json:"id"`
	Key     string   `json:"key"`
	Reasons []string `json:"reasons"`
	// Open: the open change requests of the item.
	Open []string `json:"open_change_requests"`
}

func writeChangeBlocked(w http.ResponseWriter, e *changeBlocked) {
	open := e.Open
	if open == nil {
		open = []string{}
	}
	writeJSON(w, http.StatusConflict, changeControlBody{Error: e.Error(), Code: "change_control", Kind: e.Kind, ID: e.ID, Key: e.Key, Reasons: e.Reasons, Open: open,
		Hint: "create a change request (POST /api/v1/projects/{pid}/change-requests) with the proposed values, or use the emergency override (?change_override=<justification>, change.approve)"})
}

// protection returns why an item is under change control in its project
// (nil: it is not, or change control is off) and the project's mode.
func (s *Server) protection(ctx context.Context, p *Principal, projectID, kind, id, status string) ([]string, string, error) {
	cc, err := s.Store.ChangeControl(ctx, p.TenantID(), projectID)
	if err != nil {
		return nil, "", err
	}
	if cc.Mode == "off" {
		return nil, cc.Mode, nil
	}
	var reasons []string
	if slices.Contains(store.EffectiveProtectedStatuses(cc), status) {
		reasons = append(reasons, "status "+status)
	}
	if cc.LockSignedBaselines {
		names, err := s.Store.SignedBaselinesOf(ctx, p.TenantID(), kind, id)
		if err != nil {
			return nil, "", err
		}
		for _, n := range names {
			reasons = append(reasons, "signed baseline "+n)
		}
	}
	return reasons, cc.Mode, nil
}

// overrideKey holds the emergency override justification of a request
// (?change_override=, set by impactAware for the endpoints it wraps).
type overrideKey struct{}

// withOverride puts the request's override justification in its context.
func withOverride(r *http.Request) *http.Request {
	if v := strings.TrimSpace(r.URL.Query().Get("change_override")); v != "" {
		return r.WithContext(context.WithValue(r.Context(), overrideKey{}, v))
	}
	return r
}

// changeGuard checks a direct change of an item: nil when it may be made.
// Under enforce it returns a *changeBlocked, unless the request applies a
// change request or carries an emergency override (which is audited once
// per item). Under warn it sets X-Change-Control on w (when given).
func (s *Server) changeGuard(r *http.Request, w http.ResponseWriter, p *Principal, projectID, kind, id, key, status string) error {
	return s.guardChange(withOverride(r).Context(), w, p, projectID, kind, id, key, status)
}

// guardRestore checks restoring an older version of an item.
func (s *Server) guardRestore(ctx context.Context, p *Principal, projectID, kind, id, key, status string) error {
	return s.guardChange(ctx, nil, p, projectID, kind, id, key, status)
}

func (s *Server) guardChange(ctx context.Context, w http.ResponseWriter, p *Principal, projectID, kind, id, key, status string) error {
	if ctx.Value(crApplyKey{}) != nil {
		return nil
	}
	reasons, mode, err := s.protection(ctx, p, projectID, kind, id, status)
	if err != nil || len(reasons) == 0 {
		return err
	}
	if mode == "warn" {
		if w != nil {
			w.Header().Set("X-Change-Control", "warn")
			w.Header().Add("X-Change-Control-Reason", key+": "+strings.Join(reasons, "; "))
		}
		return nil
	}
	if why, _ := ctx.Value(overrideKey{}).(string); why != "" {
		if !p.Can(rbac.ChangeApprove) {
			return forbidden("permission change.approve required for an emergency override of change control")
		}
		if len([]rune(why)) < 10 {
			return badRequest("an emergency override needs a justification of at least 10 characters")
		}
		s.audit(ctx, p, "change_control.override", kind, id, key+" ("+strings.Join(reasons, "; ")+"): "+excerpt(why, 500))
		return nil
	}
	open, _ := s.Store.OpenChangeRequestsByItem(ctx, p.TenantID(), projectID)
	return &changeBlocked{Kind: kind, ID: id, Key: key, Reasons: reasons, Open: open[kind+":"+id]}
}

// guardItems checks the direct change of several items of a project.
func (s *Server) guardItems(r *http.Request, w http.ResponseWriter, p *Principal, projectID, kind string, ids []string) error {
	for _, id := range uniqStrings(ids) {
		pid, _, status, key, err := s.Store.ItemState(r.Context(), p.TenantID(), kind, id)
		if err != nil || pid != projectID {
			continue // the change itself answers
		}
		if err := s.changeGuard(r, w, p, projectID, kind, id, key, status); err != nil {
			return err
		}
	}
	return nil
}

// handleItemChangeControl: GET /requirements/{id}/change-control — whether
// the item is under change control, why, and its open change requests.
func (s *Server) handleItemChangeControl(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		ctx := r.Context()
		pid, _, status, key, err := s.Store.ItemState(ctx, p.TenantID(), kind, r.PathValue("id"))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		id := r.PathValue("id")
		reasons, mode, err := s.protection(ctx, p, pid, kind, id, status)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		list, err := s.Store.ListChangeRequests(ctx, p.TenantID(), pid, store.CRFilter{Open: true, EntityType: kind, EntityID: id})
		if err != nil {
			s.fail(w, r, err)
			return
		}
		type openCR struct {
			ID     string `json:"id"`
			Key    string `json:"key"`
			Title  string `json:"title"`
			Status string `json:"status"`
		}
		open := []openCR{}
		for _, c := range list {
			open = append(open, openCR{c.ID, c.Key, c.Title, c.Status})
		}
		if reasons == nil {
			reasons = []string{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"key": key, "mode": mode, "protected": len(reasons) > 0, "reasons": reasons,
			"blocked": mode == "enforce" && len(reasons) > 0, "open_change_requests": open,
			"can_propose": p.Can(rbac.ChangeCreate), "can_override": p.Can(rbac.ChangeApprove)})
	}
}

// ---------------------------------------------------------------- settings

func (s *Server) handleGetChangeControl(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	cc, err := s.Store.ChangeControl(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": cc, "effective_statuses": store.EffectiveProtectedStatuses(cc), "default_statuses": model.DefaultProtectedStatuses,
		"modes": model.ChangeControlModes, "default_signature_meaning": model.DefaultSignatureMeaning})
}

func (s *Server) handleSetChangeControl(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var cc model.ChangeControlSettings
	if err := decode(r, &cc); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(cc.Board.Roles) > 0 {
		if err := s.validRoles(r.Context(), p.TenantID(), cc.Board.Roles); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	cc.UpdatedBy = p.User.Email
	if err := s.Store.SetChangeControl(r.Context(), p.TenantID(), pr.ID, &cc); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "project.change_control", "project", pr.ID, fmt.Sprintf("%s mode %s, quorum %s, board %d users %d roles", pr.Key, cc.Mode, cc.Board.Quorum, len(cc.Board.Users), len(cc.Board.Roles)))
	s.handleGetChangeControl(w, r, p)
}

// ---------------------------------------------------------------- CRUD

// crBody is the editable part of a change request in a request.
type crBody struct {
	Title           *string                   `json:"title"`
	Reason          *string                   `json:"reason"`
	Category        *string                   `json:"category"`
	Priority        *string                   `json:"priority"`
	ImpactAction    *string                   `json:"impact_action"`
	Links           *model.ChangeRequestLinks `json:"links"`
	BaselineOnClose *bool                     `json:"baseline_on_close"`
	Items           []model.ChangeRequestItem `json:"items"`
	// Submit: submit the new change request at once.
	Submit bool `json:"submit"`
}

func (b *crBody) apply(c *model.ChangeRequest) {
	for _, f := range []struct{ src, dst *string }{{b.Title, &c.Title}, {b.Reason, &c.Reason}, {b.Category, &c.Category}, {b.Priority, &c.Priority}, {b.ImpactAction, &c.ImpactAction}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	if b.Links != nil {
		c.Links = *b.Links
	}
	if b.BaselineOnClose != nil {
		c.BaselineOnClose = *b.BaselineOnClose
	}
}

func (s *Server) handleListChangeRequests(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	q := r.URL.Query()
	f := store.CRFilter{Status: q.Get("status"), Category: q.Get("category"), Open: q.Get("open") == "true"}
	if f.Status != "" && !slices.Contains(model.CRStatuses, f.Status) {
		s.fail(w, r, badRequest("status must be one of %s", strings.Join(model.CRStatuses, ", ")))
		return
	}
	if v := q.Get("item"); v != "" {
		kind, id, ok := strings.Cut(v, ":")
		if !ok {
			kind, id = model.KindRequirement, v
		}
		f.EntityType, f.EntityID = kind, id
	}
	list, err := s.Store.ListChangeRequests(r.Context(), p.TenantID(), pr.ID, f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

// handleOpenChangeItems: the items of a project with open change requests
// ("kind:id" → keys), for the badges of the lists.
func (s *Server) handleOpenChangeItems(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	m, err := s.Store.OpenChangeRequestsByItem(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": m})
}

func (s *Server) handleCreateChangeRequest(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var b crBody
	if err := decode(r, &b); err != nil {
		s.fail(w, r, err)
		return
	}
	cr := &model.ChangeRequest{ProjectID: pr.ID, CreatedBy: p.User.Email, Items: b.Items}
	b.apply(cr)
	if err := s.checkProposalPerms(p, cr.Items); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	if err := s.Store.CreateChangeRequest(ctx, p.TenantID(), cr); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "change_request.create", model.KindChangeRequest, cr.ID, cr.Key+" "+cr.Title)
	if b.Submit {
		if err := s.submitCR(r, p, cr.ID); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	out, err := s.Store.GetChangeRequest(ctx, p.TenantID(), cr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// checkProposalPerms: proposing changes to tests needs test.read (and
// what the change needs is checked when it is applied).
func (s *Server) checkProposalPerms(p *Principal, items []model.ChangeRequestItem) error {
	for _, it := range items {
		if it.EntityType == model.KindTest && !p.Can(rbac.TestRead) {
			return forbidden("permission test.read required to propose changes to tests")
		}
	}
	return nil
}

func (s *Server) writeChangeRequest(w http.ResponseWriter, r *http.Request, p *Principal, id string) {
	out, err := s.Store.GetChangeRequest(r.Context(), p.TenantID(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetChangeRequest(w http.ResponseWriter, r *http.Request, p *Principal) {
	s.writeChangeRequest(w, r, p, r.PathValue("id"))
}

func (s *Server) handleUpdateChangeRequest(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	cr, err := s.Store.GetChangeRequest(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var b crBody
	if err := decode(r, &b); err != nil {
		s.fail(w, r, err)
		return
	}
	if !store.CREditable(cr.Status) {
		s.fail(w, r, store.Conflictf("only a draft change request can be changed (it is %s): withdraw it first", cr.Status))
		return
	}
	b.apply(cr)
	if err := s.Store.UpdateChangeRequest(ctx, p.TenantID(), cr); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(b.Items) > 0 {
		if err := s.checkProposalPerms(p, b.Items); err != nil {
			s.fail(w, r, err)
			return
		}
		if err := s.Store.PutChangeRequestItems(ctx, p.TenantID(), cr.ID, b.Items); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.audit(ctx, p, "change_request.update", model.KindChangeRequest, cr.ID, cr.Key+" "+cr.Title)
	s.writeChangeRequest(w, r, p, cr.ID)
}

func (s *Server) handleDeleteChangeRequest(w http.ResponseWriter, r *http.Request, p *Principal) {
	cr, err := s.Store.GetChangeRequest(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteChangeRequest(r.Context(), p.TenantID(), cr.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "change_request.delete", model.KindChangeRequest, cr.ID, cr.Key+" "+cr.Title)
	w.WriteHeader(http.StatusNoContent)
}

// handlePutChangeRequestItems: {"items": [{entity_type, entity_id,
// proposed, base_version}]} adds items to a draft (or replaces their
// proposals).
func (s *Server) handlePutChangeRequestItems(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Items []model.ChangeRequestItem `json:"items"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(req.Items) == 0 {
		s.fail(w, r, badRequest("choose at least one item"))
		return
	}
	if err := s.checkProposalPerms(p, req.Items); err != nil {
		s.fail(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := s.Store.PutChangeRequestItems(r.Context(), p.TenantID(), id, req.Items); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "change_request.items", model.KindChangeRequest, id, strconv.Itoa(len(req.Items))+" items")
	s.writeChangeRequest(w, r, p, id)
}

func (s *Server) handleRemoveChangeRequestItem(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	if err := s.Store.RemoveChangeRequestItem(r.Context(), p.TenantID(), id, r.PathValue("item")); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "change_request.items", model.KindChangeRequest, id, "removed "+r.PathValue("item"))
	s.writeChangeRequest(w, r, p, id)
}

// ---------------------------------------------------------------- impact

// crImpact computes the transitive impact of a change request's items.
func (s *Server) crImpact(ctx context.Context, p *Principal, cr *model.ChangeRequest) (*model.ChangeImpact, error) {
	x, err := s.Store.LoadImpactIndex(ctx, p.TenantID(), cr.ProjectID)
	if err != nil {
		return nil, err
	}
	var roots []store.ImpactRoot
	for _, it := range cr.Items {
		if it.Missing || !x.Has(it.EntityType, it.EntityID) {
			continue
		}
		fields := make([]string, 0, len(it.Proposed))
		for k := range it.Proposed {
			fields = append(fields, k)
		}
		slices.Sort(fields)
		content := store.ContentChanged(it.EntityType, fields)
		if len(content) == 0 && !slices.Contains(fields, "status") {
			continue
		}
		roots = append(roots, store.ImpactRoot{Kind: it.EntityType, ID: it.EntityID, ReviewOnly: len(content) == 0, Fields: fields})
	}
	if len(roots) == 0 {
		return emptyImpact(), nil
	}
	imp, _ := x.Analyze(roots, 0)
	s.redactImpact(p, imp)
	return imp, nil
}

// handleChangeRequestImpact: the impact of the change request now (the
// stored snapshot is the one of the assessment).
func (s *Server) handleChangeRequestImpact(w http.ResponseWriter, r *http.Request, p *Principal) {
	cr, err := s.Store.GetChangeRequest(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	imp, err := s.crImpact(r.Context(), p, cr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, imp)
}

// ---------------------------------------------------------------- workflow

// submitCR: draft → submitted, with the impact snapshot computed.
func (s *Server) submitCR(r *http.Request, p *Principal, id string) error {
	ctx := r.Context()
	cr, err := s.Store.GetChangeRequest(ctx, p.TenantID(), id)
	if err != nil {
		return err
	}
	if len(cr.Items) == 0 {
		return badRequest("a change request needs at least one item to change")
	}
	if len([]rune(strings.TrimSpace(cr.Reason))) < 3 {
		return badRequest("a change request needs a reason")
	}
	imp, err := s.crImpact(ctx, p, cr)
	if err != nil {
		return err
	}
	if err := s.Store.SetChangeRequestStatus(ctx, p.TenantID(), id, store.CRTransition{From: []string{"draft"}, To: "submitted", By: p.User.Email, Impact: imp}); err != nil {
		return err
	}
	s.audit(ctx, p, "change_request.submit", model.KindChangeRequest, id, cr.Key)
	s.crEvent(r, p, id, "change_request.submitted", nil)
	return nil
}

func (s *Server) handleSubmitChangeRequest(w http.ResponseWriter, r *http.Request, p *Principal) {
	if err := s.submitCR(r, p, r.PathValue("id")); err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeChangeRequest(w, r, p, r.PathValue("id"))
}

// handleAssessChangeRequest: submitted | assessed → assessed: the impact
// is computed again and stored with the assessor's notes and effort
// estimate ({"notes", "effort", "review": true sends it to the board too}).
func (s *Server) handleAssessChangeRequest(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Notes  *string `json:"notes"`
		Effort *string `json:"effort"`
		Review bool    `json:"review"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	for _, f := range []*string{req.Notes, req.Effort} {
		if f != nil && len([]rune(*f)) > 8000 {
			s.fail(w, r, badRequest("notes and effort are at most 8000 characters"))
			return
		}
	}
	ctx := r.Context()
	cr, err := s.Store.GetChangeRequest(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	imp, err := s.crImpact(ctx, p, cr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	comment := ""
	if req.Effort != nil && strings.TrimSpace(*req.Effort) != "" {
		comment = "effort " + strings.TrimSpace(*req.Effort)
	}
	if err := s.Store.SetChangeRequestStatus(ctx, p.TenantID(), cr.ID, store.CRTransition{From: []string{"submitted", "assessed"}, To: "assessed", By: p.User.Email,
		Impact: imp, ImpactNotes: req.Notes, Effort: req.Effort, Comment: comment}); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "change_request.assess", model.KindChangeRequest, cr.ID, fmt.Sprintf("%s impact %d", cr.Key, imp.Total))
	if req.Review {
		if err := s.reviewCR(r, p, cr.ID); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.writeChangeRequest(w, r, p, cr.ID)
}

// reviewCR: assessed → in_review; the board is notified.
func (s *Server) reviewCR(r *http.Request, p *Principal, id string) error {
	ctx := r.Context()
	if err := s.Store.SetChangeRequestStatus(ctx, p.TenantID(), id, store.CRTransition{From: []string{"assessed"}, To: "in_review", By: p.User.Email}); err != nil {
		return err
	}
	cr, err := s.Store.GetChangeRequest(ctx, p.TenantID(), id)
	if err != nil {
		return err
	}
	s.audit(ctx, p, "change_request.review", model.KindChangeRequest, id, cr.Key)
	board, err := s.boardMembers(ctx, p, cr.ProjectID)
	if err == nil {
		s.notifyUsers(ctx, p, board, model.Notification{ProjectID: cr.ProjectID, Event: "change_request.review", Title: crTitle(cr),
			Body: excerpt(cr.Reason, 300), EntityType: model.KindChangeRequest, EntityID: cr.ID})
	}
	s.notifyEvent(r, p, cr.ProjectID, "change_request.review", map[string]any{"change_request": crPayload(cr)})
	return nil
}

func (s *Server) handleReviewChangeRequest(w http.ResponseWriter, r *http.Request, p *Principal) {
	if err := s.reviewCR(r, p, r.PathValue("id")); err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeChangeRequest(w, r, p, r.PathValue("id"))
}

// handleWithdrawChangeRequest: back to draft, to change it ({"comment"}).
func (s *Server) handleWithdrawChangeRequest(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Comment string `json:"comment"`
	}
	if r.ContentLength != 0 {
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	id := r.PathValue("id")
	if err := s.Store.SetChangeRequestStatus(r.Context(), p.TenantID(), id, store.CRTransition{From: []string{"submitted", "assessed", "in_review", "rejected", "deferred"},
		To: "draft", By: p.User.Email, Comment: req.Comment, Event: "withdrawn"}); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "change_request.withdraw", model.KindChangeRequest, id, "")
	s.writeChangeRequest(w, r, p, id)
}

// rolesIn returns the roles a user holds in a project (project roles of a
// member, else the organisation roles).
func (s *Server) rolesIn(ctx context.Context, tenantID, projectID, userID string) []string {
	up, err := s.principalFor(ctx, tenantID, userID)
	if err != nil {
		return nil
	}
	if acc, err := s.projectAccess(ctx, up, projectID); err == nil && acc != nil && len(acc.Roles) > 0 {
		return acc.Roles
	}
	return up.Roles
}

// onBoard reports whether a user sits on the change control board.
func onBoard(b model.ChangeBoard, email string, roles []string) bool {
	if len(b.Users) == 0 && len(b.Roles) == 0 {
		return true
	}
	if slices.ContainsFunc(b.Users, func(u string) bool { return strings.EqualFold(u, email) }) {
		return true
	}
	return slices.ContainsFunc(b.Roles, func(r string) bool { return slices.Contains(roles, r) })
}

// boardMembers returns the user ids of the board members who can decide
// in the project (holding change.approve there).
func (s *Server) boardMembers(ctx context.Context, p *Principal, projectID string) ([]string, error) {
	cc, err := s.Store.ChangeControl(ctx, p.TenantID(), projectID)
	if err != nil {
		return nil, err
	}
	access, members, err := s.membersIn(ctx, p.TenantID(), projectID)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, m := range members {
		acc := access[m.UserID]
		if acc == nil || !slices.Contains(acc.Permissions, string(rbac.ChangeApprove)) || m.UserID == p.User.ID {
			continue
		}
		roles := acc.Roles
		if len(roles) == 0 {
			roles = m.Roles
		}
		if onBoard(cc.Board, m.Email, roles) {
			out = append(out, m.UserID)
		}
	}
	return out, nil
}

// BoardVote is a valid signature of a board member counted for a decision.
type BoardVote struct {
	Email    string
	Roles    []string
	Decision string
}

// BoardOutcome decides a change request in review from the valid votes of
// board members: "rejected" by any rejection, "deferred" by any deferral,
// "approved" when the quorum approved (all: every listed user and a
// holder of every listed role; any: one; N: N members; an empty board:
// one, or N), else "".
func BoardOutcome(b model.ChangeBoard, votes []BoardVote) string {
	var approvers []BoardVote
	deferred := false
	for _, v := range votes {
		switch v.Decision {
		case "reject":
			return "rejected"
		case "defer":
			deferred = true
		case "approve":
			if !slices.ContainsFunc(approvers, func(a BoardVote) bool { return strings.EqualFold(a.Email, v.Email) }) {
				approvers = append(approvers, v)
			}
		}
	}
	if deferred {
		return "deferred"
	}
	need := 1
	if n, err := strconv.Atoi(b.Quorum); err == nil {
		need = n
	}
	if b.Quorum == "all" && (len(b.Users) > 0 || len(b.Roles) > 0) {
		for _, u := range b.Users {
			if !slices.ContainsFunc(approvers, func(a BoardVote) bool { return strings.EqualFold(a.Email, u) }) {
				return ""
			}
		}
		for _, role := range b.Roles {
			if !slices.ContainsFunc(approvers, func(a BoardVote) bool { return slices.Contains(a.Roles, role) }) {
				return ""
			}
		}
		return "approved"
	}
	if len(approvers) >= need {
		return "approved"
	}
	return ""
}

// BoardProgress: approvals so far and needed (for the client).
func BoardProgress(b model.ChangeBoard, votes []BoardVote) (approved, needed int) {
	for _, v := range votes {
		if v.Decision == "approve" {
			approved++
		}
	}
	switch {
	case b.Quorum == "all" && (len(b.Users) > 0 || len(b.Roles) > 0):
		needed = len(b.Users) + len(b.Roles)
	case b.Quorum == "all" || b.Quorum == "any":
		needed = 1
	default:
		needed, _ = strconv.Atoi(b.Quorum)
	}
	return min(approved, max(needed, approved)), needed
}

// crVotes returns the valid signatures of board members.
func (s *Server) crVotes(ctx context.Context, p *Principal, cr *model.ChangeRequest, b model.ChangeBoard) []BoardVote {
	var out []BoardVote
	for _, a := range cr.Approvals {
		if !a.Valid {
			continue
		}
		roles := s.rolesIn(ctx, p.TenantID(), cr.ProjectID, a.UserID)
		if !onBoard(b, a.UserEmail, roles) {
			continue
		}
		out = append(out, BoardVote{Email: a.UserEmail, Roles: roles, Decision: a.Decision})
	}
	return out
}

// handleDecideChangeRequest records a board member's signed decision on a
// change request in review: {"decision": "approve"|"reject"|"defer",
// "comment", "password"}; the quorum rule decides the outcome.
func (s *Server) handleDecideChangeRequest(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	var req struct {
		signerCreds
		Decision string `json:"decision"`
		Comment  string `json:"comment"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	cr, err := s.Store.GetChangeRequest(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if cr.Status != "in_review" {
		s.fail(w, r, store.Conflictf("only a change request in review can be decided (it is %s)", cr.Status))
		return
	}
	if cr.Outdated {
		s.fail(w, r, store.Conflictf("an item of the change request changed since it was proposed: withdraw it and propose the change again"))
		return
	}
	if strings.EqualFold(cr.CreatedBy, p.User.Email) {
		writeError(w, http.StatusForbidden, "forbidden", "the requester cannot decide their own change request")
		return
	}
	cc, err := s.Store.ChangeControl(ctx, p.TenantID(), cr.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !onBoard(cc.Board, p.User.Email, s.rolesIn(ctx, p.TenantID(), cr.ProjectID, p.User.ID)) {
		writeError(w, http.StatusForbidden, "forbidden", "you are not on the project's change control board")
		return
	}
	// the same re-authentication as every signature (electronic-records
	// mode: password, second factor and a meaning of the configured list)
	proof, ok := s.confirmSigner(w, r, p, req.signerCreds, nil, model.SignChangeRequestDecision)
	if !ok {
		return
	}
	meaning := proof.Meaning
	if meaning == "" {
		meaning = cc.SignatureMeaning
	}
	if meaning == "" {
		meaning = model.DefaultSignatureMeaning
	}
	a := &model.ChangeRequestSignoff{UserID: p.User.ID, UserEmail: p.User.Email, UserName: signerName(p), Decision: req.Decision, Meaning: meaning, Comment: req.Comment}
	if err := s.Store.SignChangeRequest(ctx, p.TenantID(), cr, a); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "change_request.signed", model.KindChangeRequest, cr.ID, cr.Key+" "+a.Decision+" "+a.ContentHash[:12])
	if cr, err = s.Store.GetChangeRequest(ctx, p.TenantID(), cr.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	if outcome := BoardOutcome(cc.Board, s.crVotes(ctx, p, cr, cc.Board)); outcome != "" {
		if err := s.Store.SetChangeRequestStatus(ctx, p.TenantID(), cr.ID, store.CRTransition{From: []string{"in_review"}, To: outcome, By: p.User.Email, Comment: req.Comment}); err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(ctx, p, "change_request."+outcome, model.KindChangeRequest, cr.ID, cr.Key)
		if cr, err = s.Store.GetChangeRequest(ctx, p.TenantID(), cr.ID); err != nil {
			s.fail(w, r, err)
			return
		}
		s.crDecided(r, p, cr, "change_request."+outcome)
	}
	writeJSON(w, http.StatusOK, cr)
}

// crApplyFailure is the answer when applying a change request failed.
type crApplyFailure struct {
	apiError
	Item string `json:"item"`
}

// handleApplyChangeRequest: approved → implemented. The proposed values
// are applied atomically through the normal PATCH handlers (validation,
// revisions, history, audit), and the change impact is applied once with
// the change request's impact action (or {"impact": ...}). An item that
// changed since the proposal stops it (409 outdated).
func (s *Server) handleApplyChangeRequest(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Impact string `json:"impact"`
	}
	if r.ContentLength != 0 {
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	ctx := r.Context()
	cr, err := s.Store.GetChangeRequest(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if cr.Status != "approved" {
		s.fail(w, r, store.Conflictf("only an approved change request can be applied (it is %s)", cr.Status))
		return
	}
	choice := cr.ImpactAction
	if req.Impact != "" {
		choice = req.Impact
	}
	if !slices.Contains(model.ImpactChoices, choice) {
		s.fail(w, r, badRequest("impact must be one of %s", strings.Join(model.ImpactChoices, ", ")))
		return
	}
	for _, it := range cr.Items {
		if it.Missing {
			s.fail(w, r, store.Conflictf("%s %s of the change request no longer exists", it.EntityType, it.EntityID))
			return
		}
		if it.Outdated {
			writeJSON(w, http.StatusConflict, crApplyFailure{apiError{Error: fmt.Sprintf("%s changed since the change was proposed (version %d, proposed on %d)", it.Key, it.CurrentVersion, it.BaseVersion), Code: "outdated"}, it.Key})
			return
		}
	}
	col := &impactCollector{choice: choice, batch: true, roots: map[string][]store.ImpactRoot{}, reasons: map[string]string{}}
	actx := context.WithValue(context.WithValue(context.WithValue(ctx, impactKey{}, col), batchKey{}, true), crApplyKey{}, cr.Key)
	var failure *crApplyFailure
	var status int
	err = s.Store.InTx(actx, func(tctx context.Context) error {
		for _, it := range cr.Items {
			update := s.handleUpdateRequirement
			if it.EntityType == model.KindTest {
				update = s.handleUpdateTest
			}
			body, _ := json.Marshal(it.Proposed)
			sub := r.Clone(tctx)
			sub.Method = http.MethodPatch
			sub.URL.RawQuery = ""
			sub.Body = io.NopCloser(bytes.NewReader(body))
			sub.ContentLength = int64(len(body))
			sub.SetPathValue("id", it.EntityID)
			sub.Header.Set("If-Match", `"`+strconv.Itoa(it.BaseVersion)+`"`)
			rec := &heldResponse{header: http.Header{}}
			update(rec, sub, p)
			if rec.status() >= 400 {
				var e apiError
				_ = json.Unmarshal(rec.body.Bytes(), &e)
				if e.Code == "" {
					e.Code = "error"
				}
				status = rec.status()
				if status == http.StatusPreconditionFailed {
					status = http.StatusConflict
				}
				failure = &crApplyFailure{apiError{Error: it.Key + ": " + e.Error, Code: e.Code}, it.Key}
				return errRolledBack
			}
			var res struct {
				Version int `json:"version"`
			}
			_ = json.Unmarshal(rec.body.Bytes(), &res)
			if err := s.Store.SetChangeRequestItemApplied(tctx, it.ID, res.Version); err != nil {
				return err
			}
		}
		s.flushImpact(tctx, p, col)
		return s.Store.SetChangeRequestStatus(tctx, p.TenantID(), cr.ID, store.CRTransition{From: []string{"approved"}, To: "implemented", By: p.User.Email,
			Comment: "impact " + choice})
	})
	if failure != nil {
		writeJSON(w, status, failure)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "change_request.apply", model.KindChangeRequest, cr.ID, fmt.Sprintf("%s %d items, impact %s", cr.Key, len(cr.Items), choice))
	if cr, err = s.Store.GetChangeRequest(ctx, p.TenantID(), cr.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.crEvent(r, p, cr.ID, "change_request.implemented", cr)
	writeJSON(w, http.StatusOK, cr)
}

// handleVerifyChangeRequest: implemented → verified ({"comment"}).
func (s *Server) handleVerifyChangeRequest(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Comment string `json:"comment"`
	}
	if r.ContentLength != 0 {
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	id := r.PathValue("id")
	if err := s.Store.SetChangeRequestStatus(r.Context(), p.TenantID(), id, store.CRTransition{From: []string{"implemented"}, To: "verified", By: p.User.Email, Comment: req.Comment}); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "change_request.verify", model.KindChangeRequest, id, excerpt(req.Comment, 200))
	s.writeChangeRequest(w, r, p, id)
}

// handleCloseChangeRequest: verified, rejected or deferred → closed;
// {"create_baseline": bool (default the change request's choice),
// "baseline_name", "comment"}. A baseline is only created for a verified
// change request.
func (s *Server) handleCloseChangeRequest(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		CreateBaseline *bool  `json:"create_baseline"`
		BaselineName   string `json:"baseline_name"`
		Comment        string `json:"comment"`
	}
	if r.ContentLength != 0 {
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	ctx := r.Context()
	cr, err := s.Store.GetChangeRequest(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	baseline := cr.BaselineOnClose
	if req.CreateBaseline != nil {
		baseline = *req.CreateBaseline
	}
	var b *model.Baseline
	err = s.Store.InTx(ctx, func(ctx context.Context) error {
		if baseline && cr.Status == "verified" {
			pr, err := s.Store.GetProject(ctx, p.TenantID(), cr.ProjectID)
			if err != nil {
				return err
			}
			items, err := s.currentItems(r.WithContext(ctx), p, pr, "")
			if err != nil {
				return err
			}
			name := strings.TrimSpace(req.BaselineName)
			if name == "" {
				name = cr.Key + " " + time.Now().UTC().Format("2006-01-02")
			}
			b = &model.Baseline{ProjectID: pr.ID, Name: name, Description: "Created when closing " + cr.Key + " " + cr.Title, CreatedBy: p.User.Email}
			if err := s.Store.CreateBaseline(ctx, p.TenantID(), b, items); err != nil {
				return err
			}
		}
		bid := ""
		if b != nil {
			bid = b.ID
		}
		return s.Store.SetChangeRequestStatus(ctx, p.TenantID(), cr.ID, store.CRTransition{From: []string{"verified", "rejected", "deferred"}, To: "closed", By: p.User.Email,
			Comment: req.Comment, BaselineID: bid})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if b != nil {
		s.audit(ctx, p, "baseline.create", "baseline", b.ID, b.Name)
	}
	s.audit(ctx, p, "change_request.close", model.KindChangeRequest, cr.ID, cr.Key)
	s.crEvent(r, p, cr.ID, "change_request.closed", nil)
	s.writeChangeRequest(w, r, p, cr.ID)
}

// ---------------------------------------------------------------- notifications

func crTitle(cr *model.ChangeRequest) string { return cr.Key + " " + cr.Title }

// crPayload is a change request in webhook and chat payloads (without the
// impact snapshot and timeline).
func crPayload(cr *model.ChangeRequest) map[string]any {
	keys := make([]string, 0, len(cr.Items))
	for _, it := range cr.Items {
		keys = append(keys, it.Key)
	}
	return map[string]any{"id": cr.ID, "key": cr.Key, "title": cr.Title, "status": cr.Status, "category": cr.Category, "priority": cr.Priority,
		"reason": cr.Reason, "decision_comment": cr.DecisionComment, "created_by": cr.CreatedBy, "items": keys}
}

// crEvent sends the webhook and chat event of a step (cr: the loaded
// change request, nil to load it).
func (s *Server) crEvent(r *http.Request, p *Principal, id, event string, cr *model.ChangeRequest) {
	if cr == nil {
		var err error
		if cr, err = s.Store.GetChangeRequest(r.Context(), p.TenantID(), id); err != nil {
			return
		}
	}
	s.notifyEvent(r, p, cr.ProjectID, event, map[string]any{"change_request": crPayload(cr)})
}

// crDecided notifies the requester of the board's decision.
func (s *Server) crDecided(r *http.Request, p *Principal, cr *model.ChangeRequest, event string) {
	ctx := r.Context()
	s.notifyUsers(ctx, p, s.recipients(ctx, p, cr.CreatedBy), model.Notification{ProjectID: cr.ProjectID, Event: event,
		Title: crTitle(cr), Body: excerpt(cr.DecisionComment, 300), EntityType: model.KindChangeRequest, EntityID: cr.ID})
	s.crEvent(r, p, cr.ID, event, cr)
}

// ---------------------------------------------------------------- my work

func init() {
	registerWorkSource("change_requests", sourceChangeRequests)
}

// sourceChangeRequests: change requests in review the caller can decide
// and has not signed ("change_approval"), and the caller's own change
// requests that came back rejected or deferred ("change_returned") or are
// approved and wait to be applied ("change_approved").
func sourceChangeRequests(ctx context.Context, w *WorkContext) ([]WorkItem, error) {
	s, p := w.Server, w.Principal
	if p.User == nil {
		return nil, nil
	}
	ids, err := s.Store.ChangeRequestIDsInStatus(ctx, p.TenantID(), "in_review", "rejected", "deferred", "approved")
	if err != nil {
		return nil, err
	}
	boards := map[string]*model.ChangeControlSettings{}
	var out []WorkItem
	for _, id := range ids {
		cr, err := s.Store.GetChangeRequestSummary(ctx, p.TenantID(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			return nil, err
		}
		if !w.Sees(cr.ProjectID, rbac.RequirementRd) {
			continue
		}
		mine := strings.EqualFold(cr.CreatedBy, p.User.Email)
		reason := ""
		switch {
		case cr.Status == "in_review" && !mine:
			if !w.Sees(cr.ProjectID, rbac.ChangeApprove) {
				continue
			}
			cc := boards[cr.ProjectID]
			if cc == nil {
				if cc, err = s.Store.ChangeControl(ctx, p.TenantID(), cr.ProjectID); err != nil {
					return nil, err
				}
				boards[cr.ProjectID] = cc
			}
			if !onBoard(cc.Board, p.User.Email, s.rolesIn(ctx, p.TenantID(), cr.ProjectID, p.User.ID)) {
				continue
			}
			if slices.ContainsFunc(cr.Approvals, func(a model.ChangeRequestSignoff) bool { return a.UserID == p.User.ID && a.Valid }) {
				continue
			}
			reason = "change_approval"
		case mine && (cr.Status == "rejected" || cr.Status == "deferred"):
			reason = "change_returned"
		case mine && cr.Status == "approved":
			reason = "change_approved"
		default:
			continue
		}
		out = append(out, WorkItem{Kind: model.KindChangeRequest, ID: cr.ID, EntityType: model.KindChangeRequest, EntityID: cr.ID, Key: cr.Key, Title: cr.Title,
			Status: cr.Status, ProjectID: cr.ProjectID, UpdatedAt: cr.UpdatedAt, Reasons: []string{reason}, Detail: cr.CreatedBy})
	}
	return out, nil
}
