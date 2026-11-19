package api

import (
	"net/http"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

// project loads the {pid} project of the principal's tenant.
func (s *Server) project(w http.ResponseWriter, r *http.Request, p *Principal) (*model.Project, bool) {
	pr, err := s.Store.GetProject(r.Context(), p.TenantID(), r.PathValue("pid"))
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	return pr, true
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request, p *Principal) {
	ps, err := s.Store.ListProjects(r.Context(), p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(ps))
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Key            string `json:"key"`
		Name           string `json:"name"`
		Description    string `json:"description"`
		SeedCategories *bool  `json:"seed_categories"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	pr := &model.Project{Key: req.Key, Name: req.Name, Description: req.Description}
	seed := req.SeedCategories == nil || *req.SeedCategories
	if err := s.Store.CreateProject(r.Context(), p.TenantID(), pr, seed); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "project.create", "project", pr.ID, pr.Key)
	writeJSON(w, http.StatusCreated, pr)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request, p *Principal) {
	if pr, ok := s.project(w, r, p); ok {
		writeJSON(w, http.StatusOK, pr)
	}
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Name != nil {
		pr.Name = *req.Name
	}
	if req.Description != nil {
		pr.Description = *req.Description
	}
	if err := s.Store.UpdateProject(r.Context(), p.TenantID(), pr); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "project.update", "project", pr.ID, pr.Name)
	writeJSON(w, http.StatusOK, pr)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	if err := s.Store.DeleteProject(r.Context(), p.TenantID(), pr.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "project.delete", "project", pr.ID, pr.Key)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	ctx := r.Context()
	pc, err := s.loadContext(ctx, p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reqs, err := s.Store.ListRequirements(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	tests, err := s.Store.ListTests(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	runs, err := s.Store.ListRuns(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reqStatus, reqObl, coverage := map[string]int{}, map[string]int{}, map[string]int{"passed": 0, "failed": 0, "not_run": 0, "none": 0, "partial": 0}
	covered := 0
	for i := range reqs {
		pc.decorateRequirement(&reqs[i])
		reqStatus[reqs[i].Status]++
		obl := reqs[i].Obligation
		if obl == "" {
			obl = "unspecified"
		}
		reqObl[obl]++
		coverage[reqs[i].Coverage]++
		if reqs[i].LinkedTests > 0 {
			covered++
		}
	}
	testType, testStatus, results := map[string]int{}, map[string]int{}, map[string]int{}
	for i := range tests {
		pc.decorateTest(&tests[i])
		testType[tests[i].ExecutionType]++
		testStatus[tests[i].Status]++
		results[tests[i].LastResult]++
	}
	runStatus := map[string]int{}
	for _, rn := range runs {
		runStatus[rn.Status]++
	}
	recent := runs
	if len(recent) > 5 {
		recent = recent[:5]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requirements": map[string]any{"total": len(reqs), "by_status": reqStatus, "by_obligation": reqObl, "covered": covered, "uncovered": len(reqs) - covered},
		"tests":        map[string]any{"total": len(tests), "by_execution_type": testType, "by_status": testStatus},
		"coverage":     coverage,
		"runs":         map[string]any{"total": len(runs), "by_status": runStatus},
		"recent_runs":  recent,
		"results":      results,
	})
}

// ---------------------------------------------------------------- folders

func kindPerm(kind string) rbac.Permission {
	if kind == model.KindTest {
		return rbac.TestWrite
	}
	return rbac.RequirementWr
}

func (s *Server) handleListFolders(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	fs, err := s.Store.ListFolders(r.Context(), p.TenantID(), pr.ID, r.URL.Query().Get("kind"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(fs))
}

func (s *Server) handleCreateFolder(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var f model.Folder
	if err := decode(r, &f); err != nil {
		s.fail(w, r, err)
		return
	}
	if !p.Can(kindPerm(f.Kind)) {
		writeError(w, http.StatusForbidden, "forbidden", "permission "+string(kindPerm(f.Kind))+" required")
		return
	}
	f.ProjectID = pr.ID
	if err := s.Store.CreateFolder(r.Context(), p.TenantID(), &f); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "folder.create", "folder", f.ID, f.Name)
	writeJSON(w, http.StatusCreated, f)
}

// folder loads {id} and checks write permission for its kind.
func (s *Server) folder(w http.ResponseWriter, r *http.Request, p *Principal) (*model.Folder, bool) {
	f, err := s.Store.GetFolder(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	if !p.Can(kindPerm(f.Kind)) {
		writeError(w, http.StatusForbidden, "forbidden", "permission "+string(kindPerm(f.Kind))+" required")
		return nil, false
	}
	return f, true
}

func (s *Server) handleUpdateFolder(w http.ResponseWriter, r *http.Request, p *Principal) {
	f, ok := s.folder(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Name != nil {
		f.Name = *req.Name
	}
	if req.Description != nil {
		f.Description = *req.Description
	}
	if err := s.Store.UpdateFolder(r.Context(), p.TenantID(), f); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "folder.update", "folder", f.ID, f.Name)
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleMoveFolder(w http.ResponseWriter, r *http.Request, p *Principal) {
	f, ok := s.folder(w, r, p)
	if !ok {
		return
	}
	var req struct {
		ParentID  *string `json:"parent_id"`
		Position  *int    `json:"position"`
		ProjectID string  `json:"project_id"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	pos := -1
	if req.Position != nil {
		pos = *req.Position
	}
	if err := s.Store.MoveFolder(r.Context(), p.TenantID(), f.ID, req.ParentID, pos, req.ProjectID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "folder.move", "folder", f.ID, f.Name)
	nf, err := s.Store.GetFolder(r.Context(), p.TenantID(), f.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nf)
}

func (s *Server) handleDeleteFolder(w http.ResponseWriter, r *http.Request, p *Principal) {
	f, ok := s.folder(w, r, p)
	if !ok {
		return
	}
	cascade := r.URL.Query().Get("cascade") == "true"
	if err := s.Store.DeleteFolder(r.Context(), p.TenantID(), f.ID, cascade); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "folder.delete", "folder", f.ID, f.Name)
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- categories

// canEditCategories: project.update, or write access to the scope's items.
func canEditCategories(w http.ResponseWriter, p *Principal, scope string) bool {
	if p.Can(rbac.ProjectUpdate) || (scope != "" && p.Can(kindPerm(scope))) {
		return true
	}
	writeError(w, http.StatusForbidden, "forbidden", "permission project.update required")
	return false
}

func (s *Server) handleListCategories(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	cs, err := s.Store.ListCategories(r.Context(), p.TenantID(), pr.ID, r.URL.Query().Get("scope"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(cs))
}

func (s *Server) handleCreateCategory(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var c model.Category
	if err := decode(r, &c); err != nil {
		s.fail(w, r, err)
		return
	}
	if !canEditCategories(w, p, c.Scope) {
		return
	}
	c.ProjectID = pr.ID
	if err := s.Store.CreateCategory(r.Context(), p.TenantID(), &c); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "category.create", "category", c.ID, c.Dimension+"/"+c.Name)
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleUpdateCategory(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, err := s.Store.GetCategory(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !canEditCategories(w, p, c.Scope) {
		return
	}
	body, err := readBody(r)
	if err == nil {
		err = jsonMerge(body, c)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.UpdateCategory(r.Context(), p.TenantID(), c); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "category.update", "category", c.ID, c.Dimension+"/"+c.Name)
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteCategory(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	c, err := s.Store.GetCategory(r.Context(), p.TenantID(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !canEditCategories(w, p, c.Scope) {
		return
	}
	if err := s.Store.DeleteCategory(r.Context(), p.TenantID(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "category.delete", "category", id, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMoveDimension(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Scope        string `json:"scope"`
		Dimension    string `json:"dimension"`
		NewDimension string `json:"new_dimension"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if !canEditCategories(w, p, req.Scope) {
		return
	}
	n, err := s.Store.RenameDimension(r.Context(), p.TenantID(), pr.ID, req.Scope, req.Dimension, req.NewDimension)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "category.move", "category", pr.ID, req.Dimension+" -> "+req.NewDimension)
	writeJSON(w, http.StatusOK, map[string]int{"moved": n})
}
