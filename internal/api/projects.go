package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// project loads the {pid} project of the principal's tenant, with the
// caller's access there (authed has scoped the principal to it).
func (s *Server) project(w http.ResponseWriter, r *http.Request, p *Principal) (*model.Project, bool) {
	pid := r.PathValue("pid")
	pr, err := s.Store.GetProject(r.Context(), p.TenantID(), pid)
	if err == nil && p.projectID != pid {
		// defence in depth: a principal that was not scoped to the project
		err = s.canIn(r.Context(), p, pid, "")
	}
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	pr.MyAccess = p.access
	return pr, true
}

// handleListProjects lists the projects the caller may read (see
// access.go), each with the caller's access there.
func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireOrg(w, p) {
		return
	}
	ps, err := s.Store.ListProjects(r.Context(), p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	vis, err := s.visibleProjects(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]model.Project, 0, len(ps))
	for i := range ps {
		if visibleIn(vis, p, ps[i].ID, rbac.ProjectRead) {
			ps[i].MyAccess = vis[ps[i].ID]
			out = append(out, ps[i])
		}
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Key            string `json:"key"`
		Name           string `json:"name"`
		Description    string `json:"description"`
		Color          string `json:"color"`
		Type           string `json:"type"`
		TechContext    string `json:"tech_context"`
		SeedCategories *bool  `json:"seed_categories"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	pr := &model.Project{Key: req.Key, Name: req.Name, Description: req.Description, Color: req.Color, Type: req.Type, TechContext: req.TechContext}
	seed := req.SeedCategories == nil || *req.SeedCategories
	if err := s.Store.CreateProject(r.Context(), p.TenantID(), pr, seed); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "project.create", "project", pr.ID, pr.Key)
	if acc, err := s.projectAccess(r.Context(), p, pr.ID); err == nil {
		pr.MyAccess = acc
	}
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
		Name               *string   `json:"name"`
		Description        *string   `json:"description"`
		DefectURLTemplate  *string   `json:"defect_url_template"`
		Icon               *string   `json:"icon"`
		Color              *string   `json:"color"`
		RequirementSources *[]string `json:"requirement_sources"`
		Type               *string   `json:"type"`
		TechContext        *string   `json:"tech_context"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Icon != nil {
		if !validPreset(*req.Icon) {
			s.fail(w, r, badRequest(`icon must be "" or "preset:<name>"; upload images with PUT /projects/{pid}/icon`))
			return
		}
		s.dropOldPicture(r.Context(), p.TenantID(), pr.Icon)
		pr.Icon = *req.Icon
	}
	if req.DefectURLTemplate != nil {
		pr.DefectURLTemplate = *req.DefectURLTemplate
	}
	if req.Color != nil {
		pr.Color = *req.Color
	}
	if req.Type != nil {
		pr.Type = *req.Type
	}
	if req.TechContext != nil {
		pr.TechContext = *req.TechContext
	}
	if req.RequirementSources != nil {
		// an empty list returns to the defaults
		pr.RequirementSources, pr.CustomSources = *req.RequirementSources, len(*req.RequirementSources) > 0
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
	// change and defect correlations (correlations.go)
	corr := map[string]int{"blocked_requirements": 0, "warned_requirements": 0, "stale_results": 0, "suspect_relations": 0}
	for i := range reqs {
		pc.decorateRequirement(&reqs[i])
		if reqs[i].BlockingDefects > 0 {
			corr["blocked_requirements"]++
		} else if reqs[i].DefectWarnings > 0 {
			corr["warned_requirements"]++
		}
		corr["suspect_relations"] += reqs[i].SuspectRelations
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
		if tests[i].Stale {
			corr["stale_results"]++
		}
	}
	runStatus := map[string]int{}
	for _, rn := range runs {
		runStatus[rn.Status]++
	}
	recent := runs
	if len(recent) > 5 {
		recent = recent[:5]
	}
	byFolder, byCategory := pc.coverageGroups(reqs)
	defects, err := s.Store.OpenDefectsBySeverity(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	flaky, err := s.Store.FlakyTests(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	flakyTotal := len(flaky)
	if len(flaky) > 10 {
		flaky = flaky[:10]
	}
	// today's snapshot for the trends (re-recorded when older than an hour)
	s.recordSnapshot(ctx, p.TenantID(), pr.ID, true)
	writeJSON(w, http.StatusOK, map[string]any{
		"requirements":         map[string]any{"total": len(reqs), "by_status": reqStatus, "by_obligation": reqObl, "covered": covered, "uncovered": len(reqs) - covered},
		"tests":                map[string]any{"total": len(tests), "by_execution_type": testType, "by_status": testStatus},
		"coverage":             coverage,
		"runs":                 map[string]any{"total": len(runs), "by_status": runStatus},
		"recent_runs":          recent,
		"results":              results,
		"active_runs":          activeRuns(runs),
		"coverage_by_folder":   byFolder,
		"coverage_by_category": byCategory,
		"defects_by_severity":  defects,
		"flaky":                map[string]any{"total": flakyTotal, "items": flaky, "window": store.FlakyWindow, "min_flips": store.FlakyMinFlips},
		"correlations":         corr,
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
		Color       *string `json:"color"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Name != nil {
		f.Name = *req.Name
	}
	if req.Color != nil {
		f.Color = *req.Color
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
	// moving into another project needs write access there too
	if req.ProjectID != "" && req.ProjectID != f.ProjectID {
		if err := s.canIn(r.Context(), p, req.ProjectID, kindPerm(f.Kind)); err != nil {
			s.fail(w, r, err)
			return
		}
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

// handleRequirementSources: GET /projects/{pid}/requirement-sources — the
// configured sources and those already used by the project's requirements.
func (s *Server) handleRequirementSources(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	used, err := s.Store.RequirementSourceUsage(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"configured": pr.RequirementSources, "custom": pr.CustomSources, "used": used})
}

// handleBulkCategories: POST /projects/{pid}/categories/bulk
// {"ids": [...], "action": "move|color|merge|copy|delete", "dimension", "color", "target_id"}.
// The values must be of one scope; the change is all or nothing.
func (s *Server) handleBulkCategories(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		IDs       []string `json:"ids"`
		Action    string   `json:"action"`
		Dimension string   `json:"dimension"`
		Color     string   `json:"color"`
		TargetID  string   `json:"target_id"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(req.IDs) == 0 {
		s.fail(w, r, badRequest("choose at least one category"))
		return
	}
	first, err := s.Store.GetCategory(r.Context(), p.TenantID(), req.IDs[0])
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if first.ProjectID != pr.ID {
		s.fail(w, r, fmt.Errorf("category %s: %w", first.ID, store.ErrNotFound))
		return
	}
	if !canEditCategories(w, p, first.Scope) {
		return
	}
	// copying writes categories of the other scope as well
	if req.Action == "copy" {
		other := model.KindTest
		if first.Scope == model.KindTest {
			other = model.KindRequirement
		}
		if !canEditCategories(w, p, other) {
			return
		}
	}
	n, err := s.Store.BulkCategories(r.Context(), p.TenantID(), pr.ID, req.IDs, store.CategoryBulk{Action: req.Action, Dimension: req.Dimension, Color: req.Color, TargetID: req.TargetID})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "category.bulk_"+req.Action, "category", strings.Join(req.IDs, ","), fmt.Sprintf("%d changed", n))
	writeJSON(w, http.StatusOK, map[string]int{"changed": n})
}

// ---------------------------------------------------------------- folders, bulk

// handleBulkFolders: POST /projects/{pid}/folders/bulk
// {"ids": [...], "action": "move" | "delete", "parent_id", "project_id", "cascade"}.
// The folders must be of one kind; a folder inside another selected folder
// goes along with it. Moving checks the target first, so a move into a
// selected folder changes nothing.
func (s *Server) handleBulkFolders(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		IDs       []string `json:"ids"`
		Action    string   `json:"action"`
		ParentID  *string  `json:"parent_id"`
		ProjectID string   `json:"project_id"`
		Cascade   bool     `json:"cascade"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Action != "move" && req.Action != "delete" {
		s.fail(w, r, badRequest("action must be move or delete"))
		return
	}
	ids, err := bulkIDs(req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	var kind string
	sel := map[string]bool{}
	for _, id := range ids {
		f, err := s.Store.GetFolder(ctx, p.TenantID(), id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if f.ProjectID != pr.ID {
			s.fail(w, r, fmt.Errorf("folder %s: %w", id, store.ErrNotFound))
			return
		}
		if kind != "" && f.Kind != kind {
			s.fail(w, r, badRequest("the folders must all be requirement folders or all test folders"))
			return
		}
		kind = f.Kind
		sel[id] = true
	}
	if !p.Can(kindPerm(kind)) {
		writeError(w, http.StatusForbidden, "forbidden", "permission "+string(kindPerm(kind))+" required")
		return
	}
	all, err := s.Store.ListFolders(ctx, p.TenantID(), pr.ID, kind)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// keep the top-most selected folders; the others move or go with them
	inside := map[string]bool{}
	for id := range sel {
		for _, d := range store.Descendants(all, id) {
			if d != id {
				inside[d] = true
			}
		}
	}
	var top []string
	for _, id := range ids {
		if !inside[id] {
			top = append(top, id)
		}
	}
	if req.Action == "move" {
		if req.ProjectID != "" && req.ProjectID != pr.ID {
			if err := s.canIn(ctx, p, req.ProjectID, kindPerm(kind)); err != nil {
				s.fail(w, r, err)
				return
			}
		}
		parent := req.ParentID
		if parent != nil && *parent == "" {
			parent = nil
		}
		if parent != nil && (sel[*parent] || inside[*parent]) {
			s.fail(w, r, store.Conflictf("cannot move folders into themselves or their subfolders"))
			return
		}
		for _, id := range top {
			if err := s.Store.MoveFolder(ctx, p.TenantID(), id, parent, -1, req.ProjectID); err != nil {
				s.fail(w, r, err)
				return
			}
			s.audit(ctx, p, "folder.move", "folder", id, "bulk")
		}
		writeJSON(w, http.StatusOK, map[string]int{"moved": len(top)})
		return
	}
	for _, id := range top {
		if err := s.Store.DeleteFolder(ctx, p.TenantID(), id, req.Cascade); err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(ctx, p, "folder.delete", "folder", id, "bulk")
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": len(top)})
}
