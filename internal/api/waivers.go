package api

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/auth"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/report"
	"github.com/rom/qtestframe/internal/store"
)

// Waivers and deviations (model.Waiver). Reading needs requirement.read;
// writing, submitting and withdrawing requirement.write; approving,
// rejecting and revoking waiver.approve with an electronic signature (the
// signer's password, or a recent single sign-on, as for baseline
// sign-offs). The requester cannot approve their own waiver.

func (s *Server) routesWaivers(h routeFunc) {
	h("GET /api/v1/projects/{pid}/waivers", s.handleListWaivers, "requirement.read")
	h("POST /api/v1/projects/{pid}/waivers", s.handleCreateWaiver, "requirement.write")
	h("GET /api/v1/waivers/{id}", s.handleGetWaiver, "requirement.read")
	h("PATCH /api/v1/waivers/{id}", s.handleUpdateWaiver, "requirement.write")
	h("DELETE /api/v1/waivers/{id}", s.handleDeleteWaiver, "requirement.write")
	h("POST /api/v1/waivers/{id}/submit", s.handleSubmitWaiver, "requirement.write")
	h("POST /api/v1/waivers/{id}/withdraw", s.handleWithdrawWaiver, "requirement.write")
	h("POST /api/v1/waivers/{id}/decision", s.handleDecideWaiver, "waiver.approve")
	h("POST /api/v1/waivers/{id}/revoke", s.handleRevokeWaiver, "waiver.approve")
	h("GET /api/v1/requirements/{id}/waivers", s.handleWaiversOfRequirement, "requirement.read")
	h("GET /api/v1/releases/{id}/waivers", s.handleWaiversOfRelease, "requirement.read")
	// residual risk: the project's acceptance threshold and the "accept risk" sign-off
	h("GET /api/v1/projects/{pid}/risk-settings", s.handleGetRiskSettings, "requirement.read")
	h("PUT /api/v1/projects/{pid}/risk-settings", s.handleSetRiskSettings, "project.update")
	h("POST /api/v1/risks/{id}/accept", s.handleAcceptRisk, "waiver.approve")
	h("DELETE /api/v1/risks/{id}/accept", s.handleWithdrawRiskAcceptance, "waiver.approve")
}

// waiverBody is the editable part of a waiver in a request. new_risk
// creates a risk for the waiver's requirements and links it.
type waiverBody struct {
	Type           *string   `json:"type"`
	Title          *string   `json:"title"`
	ReleaseID      *string   `json:"release_id"`
	RequirementIDs *[]string `json:"requirement_ids"`
	VariantIDs     *[]string `json:"variant_ids"`
	Justification  *string   `json:"justification"`
	Compensating   *string   `json:"compensating"`
	Conditions     *string   `json:"conditions"`
	RiskID         *string   `json:"risk_id"`
	Approvers      *[]string `json:"approvers"`
	ExpiresOn      *string   `json:"expires_on"`
	NewRisk        *struct {
		Title      string `json:"title"`
		Likelihood int    `json:"likelihood"`
		Impact     int    `json:"impact"`
	} `json:"new_risk"`
	// Submit: create the waiver as requested instead of a draft.
	Submit bool `json:"submit"`
}

func (b *waiverBody) apply(w *model.Waiver) {
	for _, f := range []struct{ src, dst *string }{{b.Type, &w.Type}, {b.Title, &w.Title}, {b.Justification, &w.Justification},
		{b.Compensating, &w.Compensating}, {b.Conditions, &w.Conditions}, {b.ExpiresOn, &w.ExpiresOn}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	if b.ReleaseID != nil {
		w.ReleaseID = b.ReleaseID
	}
	if b.RiskID != nil {
		w.RiskID = b.RiskID
	}
	if b.RequirementIDs != nil {
		w.RequirementIDs = *b.RequirementIDs
	}
	if b.VariantIDs != nil {
		w.VariantIDs = *b.VariantIDs
	}
	if b.Approvers != nil {
		w.Approvers = *b.Approvers
	}
}

// newWaiverRisk creates the risk of new_risk for the waiver's requirements.
func (s *Server) newWaiverRisk(r *http.Request, p *Principal, w *model.Waiver, b *waiverBody) error {
	if b.NewRisk == nil {
		return nil
	}
	if !p.Can(rbac.RequirementWr) {
		return errForbidden
	}
	k := &model.Risk{ProjectID: w.ProjectID, Title: b.NewRisk.Title, Likelihood: b.NewRisk.Likelihood, Impact: b.NewRisk.Impact,
		Status: "identified", Mitigation: strings.TrimSpace(w.Compensating), CreatedBy: p.User.Email, RequirementIDs: w.RequirementIDs}
	if strings.TrimSpace(k.Title) == "" {
		k.Title = w.Title
	}
	if err := s.Store.CreateRisk(r.Context(), p.TenantID(), k); err != nil {
		return err
	}
	s.audit(r.Context(), p, "risk.create", "risk", k.ID, k.Key+" "+k.Title+" (waiver)")
	w.RiskID = &k.ID
	return nil
}

func (s *Server) handleListWaivers(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	q := r.URL.Query()
	f := store.WaiverFilter{Status: q.Get("status"), Type: q.Get("type"), ReleaseID: q.Get("release_id"), RequirementID: q.Get("requirement_id"), ActiveOnly: q.Get("active") == "true"}
	if f.Status != "" && !slices.Contains(model.WaiverStatuses, f.Status) {
		s.fail(w, r, badRequest("status must be one of %s", strings.Join(model.WaiverStatuses, ", ")))
		return
	}
	list, err := s.Store.ListWaivers(r.Context(), p.TenantID(), pr.ID, f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

func (s *Server) handleCreateWaiver(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var b waiverBody
	if err := decode(r, &b); err != nil {
		s.fail(w, r, err)
		return
	}
	wv := &model.Waiver{ProjectID: pr.ID, RequestedBy: p.User.Email, CreatedBy: p.User.Email, Status: "draft"}
	b.apply(wv)
	if b.Submit {
		wv.Status = "requested"
	}
	ctx := r.Context()
	err := s.Store.InTx(ctx, func(ctx context.Context) error {
		r := r.WithContext(ctx)
		if err := s.newWaiverRisk(r, p, wv, &b); err != nil {
			return err
		}
		return s.Store.CreateWaiver(ctx, p.TenantID(), wv)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "waiver.create", "waiver", wv.ID, wv.Key+" "+wv.Type+" "+wv.Title)
	out, err := s.Store.GetWaiver(ctx, p.TenantID(), wv.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if out.Status == "requested" {
		s.waiverRequested(r, p, out)
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleGetWaiver(w http.ResponseWriter, r *http.Request, p *Principal) {
	wv, err := s.Store.GetWaiver(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, wv)
}

func (s *Server) handleUpdateWaiver(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	wv, err := s.Store.GetWaiver(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var b waiverBody
	if err := decode(r, &b); err != nil {
		s.fail(w, r, err)
		return
	}
	if !store.WaiverEditable(wv.Status) {
		s.fail(w, r, store.Conflictf("only a draft or requested waiver can be changed (it is %s)", wv.Status))
		return
	}
	b.apply(wv)
	err = s.Store.InTx(ctx, func(ctx context.Context) error {
		if err := s.newWaiverRisk(r.WithContext(ctx), p, wv, &b); err != nil {
			return err
		}
		return s.Store.UpdateWaiver(ctx, p.TenantID(), wv)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "waiver.update", "waiver", wv.ID, wv.Key+" "+wv.Title)
	s.writeWaiver(w, r, p, wv.ID)
}

func (s *Server) writeWaiver(w http.ResponseWriter, r *http.Request, p *Principal, id string) {
	out, err := s.Store.GetWaiver(r.Context(), p.TenantID(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteWaiver(w http.ResponseWriter, r *http.Request, p *Principal) {
	wv, err := s.Store.GetWaiver(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteWaiver(r.Context(), p.TenantID(), wv.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "waiver.delete", "waiver", wv.ID, wv.Key+" "+wv.Title)
	w.WriteHeader(http.StatusNoContent)
}

// handleSubmitWaiver: draft → requested (the approvers are notified).
func (s *Server) handleSubmitWaiver(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	wv, err := s.Store.GetWaiver(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := store.ValidateWaiverForRequest(wv); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.SetWaiverStatus(ctx, p.TenantID(), wv.ID, []string{"draft"}, "requested", p.User.Email, ""); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "waiver.submit", "waiver", wv.ID, wv.Key)
	out, err := s.Store.GetWaiver(ctx, p.TenantID(), wv.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.waiverRequested(r, p, out)
	writeJSON(w, http.StatusOK, out)
}

// handleWithdrawWaiver: requested (or rejected) → draft, to change it.
func (s *Server) handleWithdrawWaiver(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	if err := s.Store.SetWaiverStatus(r.Context(), p.TenantID(), id, []string{"requested", "rejected"}, "draft", p.User.Email, ""); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "waiver.withdraw", "waiver", id, "")
	s.writeWaiver(w, r, p, id)
}

// signerOK confirms the identity of a signer: a browser session and their
// password, or a single sign-on less than 15 minutes old. It writes the
// error and returns false otherwise.
func signerOK(w http.ResponseWriter, p *Principal, password string) bool {
	if p.Token != nil || p.Session == nil {
		writeError(w, http.StatusForbidden, "forbidden", "electronic signatures are given in the browser, not with an API token")
		return false
	}
	if p.User.PasswordHash != "" {
		if !auth.CheckPassword(p.User.PasswordHash, password) {
			writeError(w, http.StatusForbidden, "reauth_failed", "the password is not correct")
			return false
		}
	} else if time.Since(p.Session.CreatedAt) > 15*time.Minute {
		writeError(w, http.StatusForbidden, "reauth_required", "sign in again (less than 15 minutes ago) to sign")
		return false
	}
	return true
}

func signerName(p *Principal) string {
	if p.User.DisplayName != "" {
		return p.User.DisplayName
	}
	return p.User.Email
}

// handleDecideWaiver records an approver's signed decision on a requested
// waiver: {"decision": "approve"|"reject", "comment", "password"}. The
// waiver is approved when every designated approver (or, without any, one
// approver) approved it, and rejected by any rejection.
func (s *Server) handleDecideWaiver(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	var req struct {
		Decision string `json:"decision"`
		Comment  string `json:"comment"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	wv, err := s.Store.GetWaiver(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if wv.Status != "requested" {
		s.fail(w, r, store.Conflictf("only a requested waiver can be decided (it is %s)", wv.Status))
		return
	}
	email := strings.ToLower(p.User.Email)
	if strings.EqualFold(wv.RequestedBy, email) {
		writeError(w, http.StatusForbidden, "forbidden", "the requester cannot approve or reject their own waiver")
		return
	}
	if len(wv.Approvers) > 0 && !slices.Contains(wv.Approvers, email) {
		writeError(w, http.StatusForbidden, "forbidden", "you are not one of the waiver's approvers")
		return
	}
	if !signerOK(w, p, req.Password) {
		return
	}
	a := &model.WaiverApproval{WaiverID: wv.ID, UserID: p.User.ID, UserEmail: p.User.Email, UserName: signerName(p), Decision: req.Decision, Comment: req.Comment}
	if err := s.Store.SignWaiver(ctx, p.TenantID(), a); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "waiver.signed", "waiver", wv.ID, wv.Key+" "+a.Decision+" "+a.ContentHash[:12])
	if wv, err = s.Store.GetWaiver(ctx, p.TenantID(), wv.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	if outcome := store.ApprovalOutcome(wv); outcome != "" {
		if err := s.Store.SetWaiverStatus(ctx, p.TenantID(), wv.ID, []string{"requested"}, outcome, p.User.Email, req.Comment); err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(ctx, p, "waiver."+outcome, "waiver", wv.ID, wv.Key)
		if wv, err = s.Store.GetWaiver(ctx, p.TenantID(), wv.ID); err != nil {
			s.fail(w, r, err)
			return
		}
		s.waiverDecided(r, p, wv, "waiver."+outcome)
	}
	writeJSON(w, http.StatusOK, wv)
}

// handleRevokeWaiver: approved → revoked, {"comment"} (required).
func (s *Server) handleRevokeWaiver(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Comment string `json:"comment"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if len([]rune(strings.TrimSpace(req.Comment))) < 3 {
		s.fail(w, r, badRequest("say why the waiver is revoked (comment)"))
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")
	if err := s.Store.SetWaiverStatus(ctx, p.TenantID(), id, []string{"approved"}, "revoked", p.User.Email, req.Comment); err != nil {
		s.fail(w, r, err)
		return
	}
	wv, err := s.Store.GetWaiver(ctx, p.TenantID(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "waiver.revoked", "waiver", id, wv.Key+" "+strings.TrimSpace(req.Comment))
	s.waiverDecided(r, p, wv, "waiver.revoked")
	writeJSON(w, http.StatusOK, wv)
}

func (s *Server) handleWaiversOfRequirement(w http.ResponseWriter, r *http.Request, p *Principal) {
	q, err := s.Store.GetRequirement(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.Store.ListWaivers(r.Context(), p.TenantID(), q.ProjectID, store.WaiverFilter{RequirementID: q.ID})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

// handleWaiversOfRelease lists the waivers of a release and the
// project-wide waivers that cover requirements of its scope.
func (s *Server) handleWaiversOfRelease(w http.ResponseWriter, r *http.Request, p *Principal) {
	rel, err := s.Store.GetRelease(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	all, err := s.Store.ListWaivers(r.Context(), p.TenantID(), rel.ProjectID, store.WaiverFilter{})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := []model.Waiver{}
	for _, wv := range all {
		if wv.ReleaseID != nil {
			if *wv.ReleaseID == rel.ID {
				out = append(out, wv)
			}
			continue
		}
		if slices.ContainsFunc(wv.RequirementIDs, func(id string) bool { return slices.Contains(rel.RequirementIDs, id) }) {
			out = append(out, wv)
		}
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

// ---------------------------------------------------------------- notifications

func waiverTitle(wv *model.Waiver) string { return wv.Key + " " + wv.Title }

// waiverRequested notifies the designated approvers of a requested waiver.
func (s *Server) waiverRequested(r *http.Request, p *Principal, wv *model.Waiver) {
	ctx := r.Context()
	s.notifyUsers(ctx, p, s.recipients(ctx, p, wv.Approvers...), model.Notification{ProjectID: wv.ProjectID, Event: "waiver.requested",
		Title: waiverTitle(wv), Body: excerpt(wv.Justification, 300), EntityType: model.KindWaiver, EntityID: wv.ID})
	s.notifyEvent(r, p, wv.ProjectID, "waiver.requested", map[string]any{"waiver": wv})
}

// waiverDecided notifies the requester (and the approvers) of a decision.
func (s *Server) waiverDecided(r *http.Request, p *Principal, wv *model.Waiver, event string) {
	ctx := r.Context()
	people := append([]string{wv.RequestedBy}, wv.Approvers...)
	s.notifyUsers(ctx, p, s.recipients(ctx, p, people...), model.Notification{ProjectID: wv.ProjectID, Event: event,
		Title: waiverTitle(wv), Body: excerpt(wv.DecisionComment, 300), EntityType: model.KindWaiver, EntityID: wv.ID})
	s.notifyEvent(r, p, wv.ProjectID, event, map[string]any{"waiver": wv})
}

// RunWaiverExpiry marks the approved waivers whose expiry date has passed
// as expired and notifies their requesters and approvers: from then on
// their requirements count as not accepted again. It returns the number
// expired.
func (s *Server) RunWaiverExpiry(ctx context.Context) (int, error) {
	list, err := s.Store.ExpireWaivers(ctx)
	for _, e := range list {
		s.Log.Info("waiver expired", "waiver", e.Key, "project", e.ProjectID)
		var users []string
		for _, who := range append([]string{e.RequestedBy}, e.Approvers...) {
			u, err := s.Store.GetUserByEmail(ctx, who)
			if err != nil || slices.Contains(users, u.ID) || !s.userCanSee(ctx, e.TenantID, e.ProjectID, u.ID, "") {
				continue
			}
			users = append(users, u.ID)
		}
		ns := make([]model.Notification, 0, len(users))
		for _, u := range users {
			ns = append(ns, model.Notification{UserID: u, ProjectID: e.ProjectID, Event: "waiver.expired", Title: waiverTitle(&e.Waiver),
				Body: "expired " + e.ExpiresOn, EntityType: model.KindWaiver, EntityID: e.ID, Actor: "Qtestframe"})
		}
		if err := s.Store.AddNotifications(ctx, e.TenantID, ns); err != nil {
			s.Log.Warn("waiver expiry notifications", "err", err)
		}
		_ = s.Store.Audit(ctx, model.AuditEntry{TenantID: e.TenantID, Action: "waiver.expired", EntityType: model.KindWaiver, EntityID: e.ID, Details: e.Key + " " + e.ExpiresOn})
	}
	return len(list), err
}

// ---------------------------------------------------------------- residual risk

func (s *Server) handleGetRiskSettings(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	t, err := s.Store.RiskThreshold(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"threshold": t, "effective": store.EffectiveThreshold(t), "default": model.RiskThresholdDefault, "levels": model.RiskLevels})
}

// handleSetRiskSettings: {"threshold": "low"|"medium"|"high"|"critical"|""}.
func (s *Server) handleSetRiskSettings(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Threshold string `json:"threshold"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	req.Threshold = strings.TrimSpace(req.Threshold)
	if err := s.Store.SetRiskThreshold(r.Context(), p.TenantID(), pr.ID, req.Threshold); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "project.risk_threshold", "project", pr.ID, pr.Key+" "+store.EffectiveThreshold(req.Threshold))
	s.handleGetRiskSettings(w, r, p)
}

// handleAcceptRisk signs the acceptance of a risk's residual assessment:
// {"comment", "password"}; the risk becomes accepted. The signature stays
// valid while the residual assessment does not change.
func (s *Server) handleAcceptRisk(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Comment  string `json:"comment"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if !signerOK(w, p, req.Password) {
		return
	}
	k, err := s.Store.AcceptRisk(r.Context(), p.TenantID(), r.PathValue("id"), p.User.Email, signerName(p), req.Comment)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "risk.accepted", "risk", k.ID, k.Key+" residual "+k.ResidualLevel)
	s.notifyEvent(r, p, k.ProjectID, "risk.updated", map[string]any{"risk": k})
	writeJSON(w, http.StatusOK, k)
}

func (s *Server) handleWithdrawRiskAcceptance(w http.ResponseWriter, r *http.Request, p *Principal) {
	k, err := s.Store.WithdrawRiskAcceptance(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "risk.acceptance_withdrawn", "risk", k.ID, k.Key)
	writeJSON(w, http.StatusOK, k)
}

// ---------------------------------------------------------------- reports

// waiverReportData loads the waivers, the evaluated release and the SoA
// decisions of the reports that show them.
func (s *Server) waiverReportData(r *http.Request, p *Principal, pr *model.Project, id string, params map[string]string, d *report.Data) error {
	ctx := r.Context()
	if id != report.Compliance && id != report.SoA && id != report.ReleaseReadiness {
		return nil
	}
	var err error
	if d.Waivers, err = s.Store.ListWaivers(ctx, p.TenantID(), pr.ID, store.WaiverFilter{}); err != nil {
		return err
	}
	switch id {
	case report.SoA:
		source := strings.TrimSpace(params["source"])
		if source == "" {
			source = report.DefaultSoASource
		}
		if d.SoADecisions, err = s.Store.SoADecisions(ctx, p.TenantID(), pr.ID, source); err != nil {
			return err
		}
	case report.ReleaseReadiness:
		if d.Release, err = s.releaseReport(r, p, pr, strings.TrimSpace(params["release_id"])); err != nil {
			return err
		}
	}
	return nil
}

// releaseReport evaluates a release of pr for the release readiness report:
// id, else the newest planned or active release, else the newest one (nil
// when the project has none).
func (s *Server) releaseReport(r *http.Request, p *Principal, pr *model.Project, id string) (*report.ReleaseReport, error) {
	ctx := r.Context()
	if id == "" {
		list, err := s.Store.ListReleases(ctx, p.TenantID(), pr.ID)
		if err != nil {
			return nil, err
		}
		for _, rel := range list {
			if rel.Status == "planned" || rel.Status == "active" {
				id = rel.ID
				break
			}
		}
		if id == "" && len(list) > 0 {
			id = list[0].ID
		}
		if id == "" {
			return nil, nil
		}
	}
	rel, err := s.Store.GetRelease(ctx, p.TenantID(), id)
	if err != nil || rel.ProjectID != pr.ID {
		return nil, badRequest("release %q does not belong to project %s", id, pr.Key)
	}
	st, err := s.releaseStatus(r, p, rel.ID)
	if err != nil {
		return nil, err
	}
	refs := func(items []store.ReleaseItem) []report.ReleaseRef {
		out := make([]report.ReleaseRef, len(items))
		for i, it := range items {
			out[i] = report.ReleaseRef{Kind: it.Kind, Key: it.Key, Title: it.Title, Note: it.Note}
		}
		return out
	}
	rr := &report.ReleaseReport{Release: *st.Release, Go: st.Go, Summary: st.Summary, Waivers: st.Waivers, RiskThreshold: st.RiskThreshold, ResidualRisks: refs(st.ResidualRisks)}
	for _, c := range st.Criteria {
		rr.Criteria = append(rr.Criteria, report.ReleaseCriterion{Type: c.Type, Threshold: c.Threshold, Level: c.Level, Met: c.Met, Available: c.Available,
			Actual: c.Actual, Total: c.Total, Items: refs(c.Items), Waived: refs(c.Waived), Accepted: refs(c.Accepted)})
	}
	for _, g := range st.Variants {
		vg := report.ReleaseVariantGate{Name: g.Variant.Name, Go: g.Go}
		for _, c := range g.Criteria {
			vg.States = append(vg.States, c.State)
		}
		rr.Variants = append(rr.Variants, vg)
	}
	return rr, nil
}
