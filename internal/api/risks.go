package api

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/store"
)

// Risks: likelihood × impact, mitigation, and the requirements that address
// them. Reading needs requirement.read, changing requirement.write.

func (s *Server) routesRisks(h routeFunc) {
	h("GET /api/v1/projects/{pid}/risks", s.handleListRisks, "requirement.read")
	h("POST /api/v1/projects/{pid}/risks", s.handleCreateRisk, "requirement.write")
	h("POST /api/v1/projects/{pid}/risks/bulk", s.atomic(s.handleBulkRisks), "requirement.write")
	h("DELETE /api/v1/projects/{pid}/risks/bulk", s.atomic(s.handleBulkDeleteRisks), "requirement.write")
	h("GET /api/v1/risks/{id}", s.handleGetRisk, "requirement.read")
	h("PATCH /api/v1/risks/{id}", s.handleUpdateRisk, "requirement.write")
	h("DELETE /api/v1/risks/{id}", s.handleDeleteRisk, "requirement.write")
	h("POST /api/v1/risks/{id}/requirements", s.handleRiskRequirements, "requirement.write")
	h("GET /api/v1/requirements/{id}/risks", s.handleRisksOfRequirement, "requirement.read")
	h("POST /api/v1/requirements/{id}/risks", s.handleRequirementRisks, "requirement.write")
}

// catalogAreas sets the catalogue area of risks copied from the catalogue.
func (s *Server) catalogAreas(list []model.Risk) {
	c, err := s.catalogue()
	if err != nil {
		return
	}
	for i := range list {
		if k := c.Risks[list[i].CatalogRef]; k != nil {
			list[i].CatalogArea = k.Area
		}
	}
}

// handleRequirementRisks: {"add": [risk ids], "remove": [risk ids]} – the
// risks a requirement mitigates, changed from the requirement's side.
func (s *Server) handleRequirementRisks(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := s.Store.SetRequirementRisks(r.Context(), p.TenantID(), id, req.Add, req.Remove); err != nil {
		s.fail(w, r, err)
		return
	}
	for _, rid := range uniqStrings(append(append([]string{}, req.Add...), req.Remove...)) {
		s.audit(r.Context(), p, "risk.requirements", "risk", rid, "requirement "+id)
	}
	s.handleRisksOfRequirement(w, r, p)
}

func (s *Server) handleListRisks(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	q, err := query.Parse(r.URL.Query().Get("q"), riskSchema)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.Store.ListRisks(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.catalogAreas(list)
	out := []model.Risk{}
	var recs []query.MapRecord
	for i := range list {
		if rec := riskRecord(&list[i]); q.Match(rec) {
			out = append(out, list[i])
			recs = append(recs, rec)
		}
	}
	sortByQuery(q, out, recs)
	writeJSON(w, http.StatusOK, listOf(out))
}

func (s *Server) handleCreateRisk(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req model.Risk
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	k := &model.Risk{ProjectID: pr.ID, Title: req.Title, Description: req.Description, Category: req.Category, Likelihood: req.Likelihood,
		Impact: req.Impact, Status: req.Status, Mitigation: req.Mitigation, Owner: req.Owner, CreatedBy: p.User.Email, RequirementIDs: req.RequirementIDs,
		ResidualLikelihood: req.ResidualLikelihood, ResidualImpact: req.ResidualImpact, ResidualManual: req.ResidualManual, ResidualJustification: req.ResidualJustification,
		ReviewDate: req.ReviewDate, ReviewOwner: req.ReviewOwner, CustomFields: req.CustomFields}
	if err := s.Store.CreateRisk(r.Context(), p.TenantID(), k); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "risk.create", "risk", k.ID, k.Key+" "+k.Title)
	s.notifyEvent(r, p, pr.ID, "risk.created", map[string]any{"risk": k})
	s.writeRisk(w, r, p, k.ID, http.StatusCreated)
}

func (s *Server) writeRisk(w http.ResponseWriter, r *http.Request, p *Principal, id string, code int) {
	k, err := s.Store.GetRisk(r.Context(), p.TenantID(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list := []model.Risk{*k}
	s.catalogAreas(list)
	writeJSON(w, code, list[0])
}

func (s *Server) handleGetRisk(w http.ResponseWriter, r *http.Request, p *Principal) {
	s.writeRisk(w, r, p, r.PathValue("id"), http.StatusOK)
}

func (s *Server) handleUpdateRisk(w http.ResponseWriter, r *http.Request, p *Principal) {
	k, err := s.Store.GetRisk(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	body, err := readBody(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		s.fail(w, r, badRequest("invalid JSON: %v", err))
		return
	}
	if err := mergeCustom(raw["custom_fields"], &k.CustomFields); err != nil {
		s.fail(w, r, err)
		return
	}
	for name, dst := range map[string]any{"title": &k.Title, "description": &k.Description, "category": &k.Category, "status": &k.Status,
		"mitigation": &k.Mitigation, "owner": &k.Owner, "likelihood": &k.Likelihood, "impact": &k.Impact,
		"residual_likelihood": &k.ResidualLikelihood, "residual_impact": &k.ResidualImpact, "residual_manual": &k.ResidualManual,
		"residual_justification": &k.ResidualJustification, "review_date": &k.ReviewDate, "review_owner": &k.ReviewOwner} {
		if v, ok := raw[name]; ok {
			if err := json.Unmarshal(v, dst); err != nil {
				s.fail(w, r, badRequest("invalid %s: %v", name, err))
				return
			}
		}
	}
	if err := s.Store.UpdateRisk(r.Context(), p.TenantID(), k); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "risk.update", "risk", k.ID, k.Key+" "+k.Status)
	s.notifyEvent(r, p, k.ProjectID, "risk.updated", map[string]any{"risk": k})
	s.writeRisk(w, r, p, k.ID, http.StatusOK)
}

func (s *Server) handleDeleteRisk(w http.ResponseWriter, r *http.Request, p *Principal) {
	k, err := s.Store.GetRisk(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteRisk(r.Context(), p.TenantID(), k.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "risk.delete", "risk", k.ID, k.Key+" "+k.Title)
	w.WriteHeader(http.StatusNoContent)
}

// handleRiskRequirements: {"add": [ids], "remove": [ids]}.
func (s *Server) handleRiskRequirements(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	id := r.PathValue("id")
	if err := s.Store.SetRiskRequirements(r.Context(), p.TenantID(), id, req.Add, req.Remove); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "risk.requirements", "risk", id, "")
	s.writeRisk(w, r, p, id, http.StatusOK)
}

func (s *Server) handleRisksOfRequirement(w http.ResponseWriter, r *http.Request, p *Principal) {
	q, err := s.Store.GetRequirement(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.Store.ListRisks(r.Context(), p.TenantID(), q.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := []model.Risk{}
	for _, k := range list {
		if slices.Contains(k.RequirementIDs, q.ID) {
			out = append(out, k)
		}
	}
	s.catalogAreas(out)
	writeJSON(w, http.StatusOK, listOf(out))
}

// releaseRisks returns the open risks at or above level that address a
// requirement of the release scope.
func (s *Server) releaseRisks(r *http.Request, p *Principal, scope []string, level string) []store.ReleaseItem {
	out := []store.ReleaseItem{}
	if len(scope) == 0 {
		return out
	}
	q, err := s.Store.GetRequirement(r.Context(), p.TenantID(), scope[0])
	if err != nil {
		return out
	}
	list, err := s.Store.ListRisks(r.Context(), p.TenantID(), q.ProjectID)
	if err != nil {
		return out
	}
	in := map[string]bool{}
	for _, id := range scope {
		in[id] = true
	}
	for _, k := range list {
		if !store.RiskOpen(k.Status) || !store.RiskLevelAtLeast(k.Level, level) {
			continue
		}
		if slices.ContainsFunc(k.RequirementIDs, func(id string) bool { return in[id] }) {
			out = append(out, store.ReleaseItem{Kind: model.KindRisk, ID: k.ID, Key: k.Key, Title: k.Title, Note: k.Level})
		}
	}
	return out
}
