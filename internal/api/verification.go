package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/report"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/verification"
)

// Verification correlations (package verification): criterion-level
// verification, verification records, the roll-up along refines, and
// system elements.
//
// Reading needs requirement.read (elements: project.read). Verification
// records are evidence like test results: recording, changing and
// deleting them needs run.execute; signing one off needs
// requirement.review or run.execute and must be done by a second person.
// Mapping a link to criteria needs test.write. Elements and requirement
// allocations need requirement.write, test allocations test.write. The
// project's verification settings need project.update.

func (s *Server) routesVerification(h routeFunc) {
	h("GET /api/v1/projects/{pid}/verification", s.handleProjectVerification, "requirement.read")
	h("GET /api/v1/requirements/{id}/verification", s.handleRequirementVerification, "requirement.read")
	h("GET /api/v1/projects/{pid}/verification-settings", s.handleGetVerificationSettings, "project.read")
	h("PUT /api/v1/projects/{pid}/verification-settings", s.handlePutVerificationSettings, "project.update")
	h("GET /api/v1/projects/{pid}/verification-records", s.handleListRecords, "requirement.read")
	h("GET /api/v1/requirements/{id}/verification-records", s.handleRequirementRecords, "requirement.read")
	h("POST /api/v1/requirements/{id}/verification-records", s.handleCreateRecord, "run.execute")
	h("GET /api/v1/verification-records/{id}", s.handleGetRecord, "requirement.read")
	h("PATCH /api/v1/verification-records/{id}", s.handleUpdateRecord, "run.execute")
	h("DELETE /api/v1/verification-records/{id}", s.handleDeleteRecord, "run.execute")
	h("POST /api/v1/verification-records/{id}/signoff", s.handleSignRecord(true), "requirement.read") // review or execute: see handleSignRecord
	h("DELETE /api/v1/verification-records/{id}/signoff", s.handleSignRecord(false), "requirement.read")
	h("PUT /api/v1/links/{id}/criteria", s.handleSetLinkCriteria, "test.write")
	h("GET /api/v1/projects/{pid}/elements", s.handleListElements, "project.read")
	h("POST /api/v1/projects/{pid}/elements", s.handleCreateElement, "requirement.write")
	h("GET /api/v1/projects/{pid}/elements/coverage", s.handleElementCoverage, "requirement.read")
	h("GET /api/v1/elements/{id}", s.handleGetElement, "project.read")
	h("PATCH /api/v1/elements/{id}", s.handleUpdateElement, "requirement.write")
	h("DELETE /api/v1/elements/{id}", s.handleDeleteElement, "requirement.write")
	h("POST /api/v1/elements/{id}/allocations", s.handleAllocate, "project.read") // write of the kind: see handleAllocate
	h("PUT /api/v1/requirements/{id}/elements", s.handleSetItemElements(model.KindRequirement), "requirement.write")
	h("PUT /api/v1/tests/{id}/elements", s.handleSetItemElements(model.KindTest), "test.write")
}

// loadElements fills the element lookups of a project context.
func (s *Server) loadElements(ctx context.Context, tenantID string, pc *projectContext) error {
	var err error
	if pc.elements, err = s.Store.ListElements(ctx, tenantID, pc.project.ID); err != nil {
		return err
	}
	if pc.reqElems, pc.testElems, err = s.Store.ElementAllocations(ctx, tenantID, pc.project.ID); err != nil {
		return err
	}
	byID := map[string]model.SystemElement{}
	for _, e := range pc.elements {
		byID[e.ID] = e
	}
	pc.elemTerms = map[string][]string{}
	for id, anc := range store.ElementAncestors(pc.elements) {
		terms := []string{byID[id].Path}
		for _, a := range anc {
			terms = append(terms, byID[a].Name)
		}
		pc.elemTerms[id] = terms
	}
	return nil
}

// ---------------------------------------------------------------- verification

// projectVerification is the answer of GET /projects/{pid}/verification.
type projectVerification struct {
	Strict bool `json:"strict"`
	// Counts of the rolled-up states of the requirements listed.
	Counts map[string]int `json:"counts"`
	// CriteriaVerified of CriteriaTotal over the requirements listed.
	CriteriaVerified int                              `json:"criteria_verified"`
	CriteriaTotal    int                              `json:"criteria_total"`
	Requirements     []*model.RequirementVerification `json:"requirements"`
	// Inconsistent: requirements whose status disagrees with their
	// verification or with their children (verified parent, unverified child).
	Inconsistent []*model.RequirementVerification `json:"inconsistent"`
}

func (s *Server) handleProjectVerification(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	pc, err := s.loadContext(r.Context(), p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reqs, err := s.filterRequirements(r, p, pr, pc, r.URL.Query().Get("q"), r.URL.Query().Get("folder_id"), true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	res := pc.verification()
	st, err := s.Store.VerificationSettings(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := projectVerification{Strict: st.StrictCriteria, Counts: map[string]int{}, Requirements: []*model.RequirementVerification{}, Inconsistent: []*model.RequirementVerification{}}
	for _, st := range model.VerificationStates {
		out.Counts[st] = 0
	}
	for _, rq := range reqs {
		v := res[rq.ID]
		if v == nil {
			continue
		}
		if rq.Status != "rejected" && rq.Status != "obsolete" {
			out.Counts[v.State]++
			out.CriteriaVerified += v.CriteriaVerified
			out.CriteriaTotal += v.CriteriaTotal
		}
		out.Requirements = append(out.Requirements, v)
		if len(v.Inconsistencies) > 0 {
			out.Inconsistent = append(out.Inconsistent, v)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRequirementVerification(w http.ResponseWriter, r *http.Request, p *Principal) {
	v, err := s.Store.RequirementVerification(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleGetVerificationSettings(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	st, err := s.Store.VerificationSettings(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handlePutVerificationSettings(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req model.VerificationSettings
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.SetVerificationSettings(r.Context(), p.TenantID(), pr.ID, p.User.Email, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	detail := "strict criterion coverage off"
	if req.StrictCriteria {
		detail = "strict criterion coverage on"
	}
	s.audit(r.Context(), p, "verification.settings", "project", pr.ID, detail)
	s.syncEvidence(r.Context(), p, pr.ID, nil)
	writeJSON(w, http.StatusOK, req)
}

// ---------------------------------------------------------------- records

func (s *Server) handleListRecords(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	list, err := s.Store.ListRecords(r.Context(), p.TenantID(), pr.ID, r.URL.Query().Get("requirement_id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

func (s *Server) handleRequirementRecords(w http.ResponseWriter, r *http.Request, p *Principal) {
	rq, err := s.Store.GetRequirement(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.Store.ListRecords(r.Context(), p.TenantID(), rq.ProjectID, rq.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

// recordInput is the body of record creation and changes.
type recordInput struct {
	CriterionID *string  `json:"criterion_id"`
	Method      string   `json:"method"`
	Result      string   `json:"result"`
	PerformedBy string   `json:"performed_by"`
	PerformedOn string   `json:"performed_on"`
	Note        string   `json:"note"`
	FileIDs     []string `json:"file_ids"`
	ValidUntil  string   `json:"valid_until"`
	UntilChange bool     `json:"until_change"`
}

func (s *Server) handleCreateRecord(w http.ResponseWriter, r *http.Request, p *Principal) {
	rq, err := s.Store.GetRequirement(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in recordInput
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	v := &model.VerificationRecord{RequirementID: rq.ID, CriterionID: in.CriterionID, Method: in.Method, Result: in.Result, PerformedBy: in.PerformedBy,
		PerformedOn: in.PerformedOn, Note: in.Note, FileIDs: in.FileIDs, ValidUntil: in.ValidUntil, UntilChange: in.UntilChange, CreatedBy: p.User.Email}
	if v.PerformedBy == "" {
		v.PerformedBy = p.User.DisplayName
	}
	if err := s.Store.CreateRecord(r.Context(), p.TenantID(), v); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "verification.create", "requirement", rq.ID, rq.Key+" "+v.Method+" "+v.Result)
	s.syncEvidence(r.Context(), p, rq.ProjectID, []string{rq.ID})
	s.writeRecord(w, r, p, v.ID, http.StatusCreated)
}

func (s *Server) writeRecord(w http.ResponseWriter, r *http.Request, p *Principal, id string, code int) {
	v, err := s.Store.GetRecord(r.Context(), p.TenantID(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, code, v)
}

func (s *Server) handleGetRecord(w http.ResponseWriter, r *http.Request, p *Principal) {
	s.writeRecord(w, r, p, r.PathValue("id"), http.StatusOK)
}

func (s *Server) handleUpdateRecord(w http.ResponseWriter, r *http.Request, p *Principal) {
	v, err := s.Store.GetRecord(r.Context(), p.TenantID(), r.PathValue("id"))
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
	files := v.FileIDs
	for name, dst := range map[string]any{"criterion_id": &v.CriterionID, "method": &v.Method, "result": &v.Result, "performed_by": &v.PerformedBy,
		"performed_on": &v.PerformedOn, "note": &v.Note, "file_ids": &files, "valid_until": &v.ValidUntil, "until_change": &v.UntilChange} {
		if x, ok := raw[name]; ok {
			if err := json.Unmarshal(x, dst); err != nil {
				s.fail(w, r, badRequest("invalid %s: %v", name, err))
				return
			}
		}
	}
	v.FileIDs = nil
	if _, ok := raw["file_ids"]; ok {
		v.FileIDs = files
		if v.FileIDs == nil {
			v.FileIDs = []string{}
		}
	}
	if err := s.Store.UpdateRecord(r.Context(), p.TenantID(), v); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "verification.update", "requirement", v.RequirementID, v.RequirementKey+" "+v.Method+" "+v.Result)
	s.syncEvidence(r.Context(), p, v.ProjectID, []string{v.RequirementID})
	s.writeRecord(w, r, p, v.ID, http.StatusOK)
}

func (s *Server) handleDeleteRecord(w http.ResponseWriter, r *http.Request, p *Principal) {
	v, err := s.Store.GetRecord(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteRecord(r.Context(), p.TenantID(), v.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "verification.delete", "requirement", v.RequirementID, v.RequirementKey+" "+v.Method+" "+v.Result)
	s.syncEvidence(r.Context(), p, v.ProjectID, []string{v.RequirementID})
	w.WriteHeader(http.StatusNoContent)
}

// handleSignRecord: POST {"comment"} signs a record off (a second person:
// not its author nor its performer), DELETE removes the sign-off.
func (s *Server) handleSignRecord(sign bool) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		if !p.Can(rbac.RequirementRev) && !p.Can(rbac.RunExecute) {
			writeError(w, http.StatusForbidden, "forbidden", "permission requirement.review or run.execute required")
			return
		}
		var req struct {
			Comment string `json:"comment"`
		}
		if sign {
			if err := decodeOptional(r, &req); err != nil {
				s.fail(w, r, err)
				return
			}
		}
		email, name := p.User.Email, p.User.DisplayName
		if !sign {
			email, name = "", ""
		}
		v, err := s.Store.SignRecord(r.Context(), p.TenantID(), r.PathValue("id"), email, name, req.Comment)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		action := "verification.signoff"
		if !sign {
			action = "verification.signoff.remove"
		}
		s.audit(r.Context(), p, action, "requirement", v.RequirementID, v.RequirementKey+" "+v.Method)
		writeJSON(w, http.StatusOK, v)
	}
}

// handleSetLinkCriteria: {"criteria": [{"criterion_id", "step_id"?}]}.
func (s *Server) handleSetLinkCriteria(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Criteria []model.LinkCriterion `json:"criteria"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	l, err := s.Store.SetLinkCriteria(r.Context(), p.TenantID(), r.PathValue("id"), req.Criteria)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	l.RequirementKey, l.TestCaseKey = s.linkKeys(r.Context(), p, l)
	s.audit(r.Context(), p, "link.criteria", "link", l.ID, l.RequirementKey+" -> "+l.TestCaseKey)
	s.syncEvidence(r.Context(), p, l.ProjectID, []string{l.RequirementID})
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) linkKeys(ctx context.Context, p *Principal, l *model.TraceLink) (string, string) {
	links, err := s.Store.ListLinks(ctx, p.TenantID(), l.ProjectID, l.RequirementID, l.TestCaseID)
	if err != nil || len(links) == 0 {
		return "", ""
	}
	return links[0].RequirementKey, links[0].TestCaseKey
}

// ---------------------------------------------------------------- elements

func (s *Server) handleListElements(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	list, err := s.Store.ListElements(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

type elementInput struct {
	ParentID    *string `json:"parent_id"`
	Name        string  `json:"name"`
	Kind        string  `json:"kind"`
	Description string  `json:"description"`
}

func (s *Server) handleCreateElement(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var in elementInput
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	e := &model.SystemElement{ProjectID: pr.ID, ParentID: in.ParentID, Name: in.Name, Kind: in.Kind, Description: in.Description, CreatedBy: p.User.Email}
	if err := s.Store.CreateElement(r.Context(), p.TenantID(), e); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "element.create", "element", e.ID, e.Name)
	s.writeElement(w, r, p, e.ID, http.StatusCreated)
}

func (s *Server) writeElement(w http.ResponseWriter, r *http.Request, p *Principal, id string, code int) {
	e, err := s.Store.GetElement(r.Context(), p.TenantID(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, code, e)
}

func (s *Server) handleGetElement(w http.ResponseWriter, r *http.Request, p *Principal) {
	s.writeElement(w, r, p, r.PathValue("id"), http.StatusOK)
}

func (s *Server) handleUpdateElement(w http.ResponseWriter, r *http.Request, p *Principal) {
	e, err := s.Store.GetElement(r.Context(), p.TenantID(), r.PathValue("id"))
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
	for name, dst := range map[string]any{"parent_id": &e.ParentID, "name": &e.Name, "kind": &e.Kind, "description": &e.Description, "position": &e.Position} {
		if x, ok := raw[name]; ok {
			if err := json.Unmarshal(x, dst); err != nil {
				s.fail(w, r, badRequest("invalid %s: %v", name, err))
				return
			}
		}
	}
	if err := s.Store.UpdateElement(r.Context(), p.TenantID(), e); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "element.update", "element", e.ID, e.Name)
	s.writeElement(w, r, p, e.ID, http.StatusOK)
}

func (s *Server) handleDeleteElement(w http.ResponseWriter, r *http.Request, p *Principal) {
	e, err := s.Store.GetElement(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteElement(r.Context(), p.TenantID(), e.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "element.delete", "element", e.ID, e.Name)
	w.WriteHeader(http.StatusNoContent)
}

// handleAllocate: {"kind": "requirement"|"test", "add": [ids], "remove": [ids]}.
func (s *Server) handleAllocate(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Kind   string   `json:"kind"`
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	perm := rbac.RequirementWr
	if req.Kind == model.KindTest {
		perm = rbac.TestWrite
	}
	if !p.Can(perm) {
		writeError(w, http.StatusForbidden, "forbidden", "permission "+string(perm)+" required")
		return
	}
	id := r.PathValue("id")
	if err := s.Store.Allocate(r.Context(), p.TenantID(), id, req.Kind, req.Add, req.Remove); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "element.allocate", "element", id, req.Kind)
	s.writeElement(w, r, p, id, http.StatusOK)
}

// handleSetItemElements: {"element_ids": [ids]} replaces the allocation.
func (s *Server) handleSetItemElements(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		var req struct {
			ElementIDs []string `json:"element_ids"`
		}
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
		id := r.PathValue("id")
		if err := s.Store.SetItemElements(r.Context(), p.TenantID(), kind, id, req.ElementIDs); err != nil {
			s.fail(w, r, err)
			return
		}
		ids, err := s.Store.ItemElements(r.Context(), kind, id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(r.Context(), p, "element.allocate", kind, id, "")
		writeJSON(w, http.StatusOK, map[string]any{"element_ids": ids})
	}
}

// handleElementCoverage: the coverage and verification per element (of
// the requirements matching ?q=, default all).
func (s *Server) handleElementCoverage(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	pc, err := s.loadContext(r.Context(), p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reqs, err := s.filterRequirements(r, p, pr, pc, r.URL.Query().Get("q"), "", true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(pc.elementCoverage(reqs)))
}

// elementCoverage computes the coverage per element of reqs.
func (pc *projectContext) elementCoverage(reqs []model.Requirement) []model.ElementCoverage {
	status := map[string]string{}
	for _, rq := range reqs {
		status[rq.ID] = rq.Status
	}
	return verification.ElementCoverage(pc.elements, store.ElementAncestors(pc.elements), status, pc.verification(), pc.last)
}

// verificationData loads the verification of reqs for reports and
// documents.
func (s *Server) verificationData(ctx context.Context, p *Principal, pc *projectContext, reqs []model.Requirement) (*report.VerificationData, error) {
	st, err := s.Store.VerificationSettings(ctx, p.TenantID(), pc.project.ID)
	if err != nil {
		return nil, err
	}
	recs, err := s.Store.ListRecords(ctx, p.TenantID(), pc.project.ID, "")
	if err != nil {
		return nil, err
	}
	return &report.VerificationData{Requirements: pc.verification(), Records: recs, Elements: pc.elementCoverage(reqs), Strict: st.StrictCriteria}, nil
}
