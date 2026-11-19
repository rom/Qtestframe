package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/reqlang"
	"github.com/rom/qtestframe/internal/store"
)

// jsonMerge overlays the fields present in body onto v.
func jsonMerge(body []byte, v any) error {
	if len(body) == 0 {
		return badRequest("request body required")
	}
	if err := json.Unmarshal(body, v); err != nil {
		return badRequest("invalid JSON: %v", err)
	}
	return nil
}

func boolParam(r *http.Request, name string, def bool) bool {
	switch strings.ToLower(r.URL.Query().Get(name)) {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	}
	return def
}

// ---------------------------------------------------------------- requirements

// filterRequirements returns decorated requirements matching q and folder.
func (s *Server) filterRequirements(r *http.Request, p *Principal, pr *model.Project, pc *projectContext, expr, folder string, recursive bool) ([]model.Requirement, error) {
	q, err := query.Parse(expr, requirementSchema)
	if err != nil {
		return nil, err
	}
	reqs, err := s.Store.ListRequirements(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		return nil, err
	}
	inFolder := pc.folderFilter(folder, recursive)
	out := make([]model.Requirement, 0, len(reqs))
	for i := range reqs {
		rq := &reqs[i]
		pc.decorateRequirement(rq)
		if inFolder(rq.FolderID) && q.Match(pc.requirementRecord(rq)) {
			out = append(out, *rq)
		}
	}
	return out, nil
}

func (s *Server) handleListRequirements(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	pc, err := s.loadContext(r.Context(), p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.filterRequirements(r, p, pr, pc, r.URL.Query().Get("q"), r.URL.Query().Get("folder_id"), boolParam(r, "recursive", true))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for i := range out {
		out[i].Criteria = nil
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

func (s *Server) handleCreateRequirement(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var rq model.Requirement
	if err := decode(r, &rq); err != nil {
		s.fail(w, r, err)
		return
	}
	rq.ProjectID = pr.ID
	rq.CreatedBy = p.User.Email
	if rq.Obligation == "" {
		rq.Obligation = reqlang.DetectObligation(rq.Title + "\n" + rq.Description)
	}
	if err := s.Store.CreateRequirement(r.Context(), p.TenantID(), &rq); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "requirement.create", "requirement", rq.ID, rq.Key)
	writeJSON(w, http.StatusCreated, rq)
}

func (s *Server) decoratedRequirement(r *http.Request, p *Principal, id string) (*model.Requirement, error) {
	rq, err := s.Store.GetRequirement(r.Context(), p.TenantID(), id)
	if err != nil {
		return nil, err
	}
	pr, err := s.Store.GetProject(r.Context(), p.TenantID(), rq.ProjectID)
	if err != nil {
		return nil, err
	}
	pc, err := s.loadContext(r.Context(), p.TenantID(), pr)
	if err != nil {
		return nil, err
	}
	pc.decorateRequirement(rq)
	return rq, nil
}

func (s *Server) handleGetRequirement(w http.ResponseWriter, r *http.Request, p *Principal) {
	rq, err := s.decoratedRequirement(r, p, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rq)
}

func (s *Server) handleUpdateRequirement(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	rq, err := s.Store.GetRequirement(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	body, err := readBody(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	keys, err := patchKeys(body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id, project, key := rq.ID, rq.ProjectID, rq.Key
	if keys["criteria"] {
		rq.Criteria = nil
	}
	if err := jsonMerge(body, rq); err != nil {
		s.fail(w, r, err)
		return
	}
	rq.ID, rq.ProjectID, rq.Key = id, project, key
	if err := s.Store.UpdateRequirement(ctx, p.TenantID(), rq, keys["criteria"]); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "requirement.update", "requirement", rq.ID, rq.Key)
	out, err := s.decoratedRequirement(r, p, rq.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteRequirement(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	n, err := s.Store.DeleteRequirements(r.Context(), p.TenantID(), "", []string{id})
	if err == nil && n == 0 {
		err = store.ErrNotFound
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "requirement.delete", "requirement", id, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Text string `json:"text"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, reqlang.Analyze(req.Text))
}

// ---------------------------------------------------------------- move / bulk

func (s *Server) handleMoveItems(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		pr, ok := s.project(w, r, p)
		if !ok {
			return
		}
		var req struct {
			IDs       []string `json:"ids"`
			FolderID  *string  `json:"folder_id"`
			Position  *int     `json:"position"`
			ProjectID string   `json:"project_id"`
		}
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
		pos := -1
		if req.Position != nil {
			pos = *req.Position
		}
		if err := s.Store.MoveItems(r.Context(), p.TenantID(), kind, pr.ID, req.IDs, req.FolderID, pos, req.ProjectID); err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(r.Context(), p, kind+".move", kind, strings.Join(req.IDs, ","), req.ProjectID)
		writeJSON(w, http.StatusOK, map[string]int{"moved": len(req.IDs)})
	}
}

func (s *Server) handleBulk(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		pr, ok := s.project(w, r, p)
		if !ok {
			return
		}
		var req struct {
			IDs []string         `json:"ids"`
			Set store.BulkUpdate `json:"set"`
		}
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
		n, err := s.Store.BulkUpdateItems(r.Context(), p.TenantID(), kind, pr.ID, req.IDs, req.Set)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(r.Context(), p, kind+".bulk_update", kind, strings.Join(req.IDs, ","), "")
		writeJSON(w, http.StatusOK, map[string]int{"updated": n})
	}
}

func (s *Server) handleBulkDelete(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		pr, ok := s.project(w, r, p)
		if !ok {
			return
		}
		var req struct {
			IDs []string `json:"ids"`
		}
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
		del := s.Store.DeleteRequirements
		if kind == model.KindTest {
			del = s.Store.DeleteTests
		}
		n, err := del(r.Context(), p.TenantID(), pr.ID, req.IDs)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(r.Context(), p, kind+".bulk_delete", kind, strings.Join(req.IDs, ","), "")
		writeJSON(w, http.StatusOK, map[string]int{"deleted": n})
	}
}

// ---------------------------------------------------------------- tests

func (s *Server) filterTests(r *http.Request, p *Principal, pr *model.Project, pc *projectContext, expr, folder string, recursive bool) ([]model.TestCase, error) {
	q, err := query.Parse(expr, testSchema)
	if err != nil {
		return nil, err
	}
	tests, err := s.Store.ListTests(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		return nil, err
	}
	inFolder := pc.folderFilter(folder, recursive)
	out := make([]model.TestCase, 0, len(tests))
	for i := range tests {
		t := &tests[i]
		pc.decorateTest(t)
		if inFolder(t.FolderID) && q.Match(pc.testRecord(t)) {
			out = append(out, *t)
		}
	}
	return out, nil
}

func (s *Server) handleListTests(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	pc, err := s.loadContext(r.Context(), p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.filterTests(r, p, pr, pc, r.URL.Query().Get("q"), r.URL.Query().Get("folder_id"), boolParam(r, "recursive", true))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for i := range out {
		out[i].Steps = nil
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

func (s *Server) handleCreateTest(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var t model.TestCase
	if err := decode(r, &t); err != nil {
		s.fail(w, r, err)
		return
	}
	t.ProjectID = pr.ID
	t.CreatedBy = p.User.Email
	if err := s.Store.CreateTest(r.Context(), p.TenantID(), &t); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "test.create", "test", t.ID, t.Key)
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) decoratedTest(r *http.Request, p *Principal, id string) (*model.TestCase, error) {
	t, err := s.Store.GetTest(r.Context(), p.TenantID(), id)
	if err != nil {
		return nil, err
	}
	pr, err := s.Store.GetProject(r.Context(), p.TenantID(), t.ProjectID)
	if err != nil {
		return nil, err
	}
	pc, err := s.loadContext(r.Context(), p.TenantID(), pr)
	if err != nil {
		return nil, err
	}
	pc.decorateTest(t)
	return t, nil
}

func (s *Server) handleGetTest(w http.ResponseWriter, r *http.Request, p *Principal) {
	t, err := s.decoratedTest(r, p, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleUpdateTest(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	t, err := s.Store.GetTest(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	body, err := readBody(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	keys, err := patchKeys(body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id, project, key := t.ID, t.ProjectID, t.Key
	if keys["steps"] {
		t.Steps = nil
	}
	if err := jsonMerge(body, t); err != nil {
		s.fail(w, r, err)
		return
	}
	t.ID, t.ProjectID, t.Key = id, project, key
	if err := s.Store.UpdateTest(ctx, p.TenantID(), t, keys["steps"]); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "test.update", "test", t.ID, t.Key)
	out, err := s.decoratedTest(r, p, t.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteTest(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	n, err := s.Store.DeleteTests(r.Context(), p.TenantID(), "", []string{id})
	if err == nil && n == 0 {
		err = store.ErrNotFound
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "test.delete", "test", id, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCopyTest(w http.ResponseWriter, r *http.Request, p *Principal) {
	t, err := s.Store.CopyTest(r.Context(), p.TenantID(), r.PathValue("id"), p.User.Email)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "test.copy", "test", t.ID, t.Key)
	writeJSON(w, http.StatusCreated, t)
}

// ---------------------------------------------------------------- links

func (s *Server) handleItemLinks(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		id := r.PathValue("id")
		var err error
		if kind == model.KindRequirement {
			_, err = s.Store.GetRequirement(r.Context(), p.TenantID(), id)
		} else {
			_, err = s.Store.GetTest(r.Context(), p.TenantID(), id)
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		var links []model.TraceLink
		if kind == model.KindRequirement {
			links, err = s.Store.ListLinks(r.Context(), p.TenantID(), "", id, "")
		} else {
			links, err = s.Store.ListLinks(r.Context(), p.TenantID(), "", "", id)
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, listOf(links))
	}
}

func (s *Server) handleListLinks(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	links, err := s.Store.ListLinks(r.Context(), p.TenantID(), pr.ID, "", "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(links))
}

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var l model.TraceLink
	if err := decode(r, &l); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.CreateLink(r.Context(), p.TenantID(), &l); err != nil {
		s.fail(w, r, err)
		return
	}
	if l.ProjectID != pr.ID {
		_ = s.Store.DeleteLink(r.Context(), p.TenantID(), l.ID)
		s.fail(w, r, badRequest("items do not belong to this project"))
		return
	}
	s.audit(r.Context(), p, "link.create", "link", l.ID, l.RequirementKey+" -> "+l.TestCaseKey)
	writeJSON(w, http.StatusCreated, l)
}

func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	if err := s.Store.DeleteLink(r.Context(), p.TenantID(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "link.delete", "link", id, "")
	w.WriteHeader(http.StatusNoContent)
}

type traceReq struct {
	ID         string `json:"id"`
	Key        string `json:"key"`
	Title      string `json:"title"`
	Obligation string `json:"obligation"`
	Coverage   string `json:"coverage"`
}

type traceTest struct {
	ID         string `json:"id"`
	Key        string `json:"key"`
	Title      string `json:"title"`
	LastResult string `json:"last_result"`
}

func (s *Server) handleTraceability(w http.ResponseWriter, r *http.Request, p *Principal) {
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
	tests, err := s.filterTests(r, p, pr, pc, r.URL.Query().Get("tq"), "", true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	res := struct {
		Requirements []traceReq        `json:"requirements"`
		Tests        []traceTest       `json:"tests"`
		Links        []model.TraceLink `json:"links"`
	}{Requirements: []traceReq{}, Tests: []traceTest{}, Links: pc.links}
	for _, rq := range reqs {
		res.Requirements = append(res.Requirements, traceReq{rq.ID, rq.Key, rq.Title, rq.Obligation, rq.Coverage})
	}
	for _, t := range tests {
		res.Tests = append(res.Tests, traceTest{t.ID, t.Key, t.Title, t.LastResult})
	}
	writeJSON(w, http.StatusOK, res)
}
