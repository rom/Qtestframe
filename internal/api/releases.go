package api

import (
	"fmt"
	"net/http"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// Releases: scope (requirements), runs, exit criteria and go / no-go.
// Reading needs project.read, changing project.update.

func (s *Server) routesReleases(h routeFunc) {
	h("GET /api/v1/projects/{pid}/releases", s.handleListReleases, "project.read")
	h("POST /api/v1/projects/{pid}/releases", s.handleCreateRelease, "project.update")
	h("POST /api/v1/projects/{pid}/releases/bulk", s.handleBulkReleases, "project.update")
	h("DELETE /api/v1/projects/{pid}/releases/bulk", s.handleBulkDeleteReleases, "project.update")
	h("GET /api/v1/releases/{id}", s.handleGetRelease, "project.read")
	h("PATCH /api/v1/releases/{id}", s.handleUpdateRelease, "project.update")
	h("DELETE /api/v1/releases/{id}", s.handleDeleteRelease, "project.update")
	h("POST /api/v1/releases/{id}/requirements", s.handleReleaseRequirements, "project.update")
	h("GET /api/v1/releases/{id}/status", s.handleReleaseStatus, "project.read")
}

func (s *Server) handleListReleases(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	list, err := s.Store.ListReleases(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

func (s *Server) handleCreateRelease(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req model.Release
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	rel := &model.Release{ProjectID: pr.ID, Name: req.Name, Version: req.Version, Description: req.Description, Status: req.Status,
		TargetDate: req.TargetDate, ExitCriteria: req.ExitCriteria, CreatedBy: p.User.Email}
	if rel.Status == "released" {
		s.fail(w, r, badRequest("create the release first, then mark it released"))
		return
	}
	if err := s.Store.CreateRelease(r.Context(), p.TenantID(), rel); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "release.create", "release", rel.ID, rel.Name)
	s.notifyEvent(r, p, rel.ProjectID, "release.created", map[string]any{"release": rel})
	out, err := s.Store.GetRelease(r.Context(), p.TenantID(), rel.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleGetRelease(w http.ResponseWriter, r *http.Request, p *Principal) {
	rel, err := s.Store.GetRelease(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

// handleUpdateRelease changes a release. Marking it released needs a go
// on the exit criteria, or "override": true with a release_note saying why;
// the release then gets a baseline of its requirements and their tests.
func (s *Server) handleUpdateRelease(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	rel, err := s.Store.GetRelease(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req struct {
		Name         *string                `json:"name"`
		Version      *string                `json:"version"`
		Description  *string                `json:"description"`
		Status       *string                `json:"status"`
		TargetDate   *string                `json:"target_date"`
		ExitCriteria *[]model.ExitCriterion `json:"exit_criteria"`
		ReleaseNote  *string                `json:"release_note"`
		Override     bool                   `json:"override"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	wasReleased := rel.Status == "released"
	for _, f := range []struct{ src, dst *string }{{req.Name, &rel.Name}, {req.Version, &rel.Version}, {req.Description, &rel.Description},
		{req.Status, &rel.Status}, {req.TargetDate, &rel.TargetDate}, {req.ReleaseNote, &rel.ReleaseNote}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	if req.ExitCriteria != nil {
		rel.ExitCriteria = *req.ExitCriteria
	}
	releasing := rel.Status == "released" && !wasReleased
	var report *store.ReleaseStatusReport
	if releasing {
		if report, err = s.releaseStatus(r, p, rel.ID); err != nil {
			s.fail(w, r, err)
			return
		}
		if !report.Go && !req.Override {
			writeJSON(w, http.StatusConflict, map[string]any{"code": "no_go", "error": "the exit criteria are not met; set override with a release_note to release anyway", "status": report})
			return
		}
		if !report.Go && len(rel.ReleaseNote) < 3 {
			s.fail(w, r, badRequest("a release_note is required to release despite unmet exit criteria"))
			return
		}
	}
	if err := s.Store.UpdateRelease(ctx, p.TenantID(), rel, p.User.Email); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "release.update", "release", rel.ID, rel.Name+" "+rel.Status)
	if releasing {
		verdict := "go"
		if !report.Go {
			verdict = "no-go, released with override: " + rel.ReleaseNote
		}
		s.audit(ctx, p, "release.released", "release", rel.ID, rel.Name+" ("+verdict+")")
		s.releaseBaseline(r, p, rel)
	}
	out, err := s.Store.GetRelease(ctx, p.TenantID(), rel.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.notifyEvent(r, p, out.ProjectID, "release.updated", map[string]any{"release": out})
	if releasing {
		s.notifyEvent(r, p, out.ProjectID, "release.released", map[string]any{"release": out, "status": report})
	}
	writeJSON(w, http.StatusOK, out)
}

// releaseBaseline freezes the release's requirements and their tests.
func (s *Server) releaseBaseline(r *http.Request, p *Principal, rel *model.Release) {
	ctx := r.Context()
	pr, err := s.Store.GetProject(ctx, p.TenantID(), rel.ProjectID)
	if err != nil {
		return
	}
	all, err := s.currentItems(r, p, pr, "")
	if err != nil {
		return
	}
	keep := map[string]bool{}
	for _, id := range rel.RequirementIDs {
		keep[id] = true
	}
	links, _ := s.Store.ListLinks(ctx, p.TenantID(), pr.ID, "", "")
	for _, l := range links {
		if keep[l.RequirementID] {
			keep[l.TestCaseID] = true
		}
	}
	var items []model.BaselineItem
	for _, it := range all {
		if keep[it.EntityID] {
			items = append(items, it)
		}
	}
	name := "Release " + rel.Name
	if rel.Version != "" && rel.Version != rel.Name {
		name += " (" + rel.Version + ")"
	}
	b := &model.Baseline{ProjectID: pr.ID, Name: name, Description: fmt.Sprintf("Created when release %s was marked released.", rel.Name), CreatedBy: p.User.Email}
	if err := s.Store.CreateBaseline(ctx, p.TenantID(), b, items); err != nil {
		s.Log.Warn("release baseline", "release", rel.ID, "err", err)
		return
	}
	s.audit(ctx, p, "baseline.create", "baseline", b.ID, b.Name)
}

func (s *Server) handleDeleteRelease(w http.ResponseWriter, r *http.Request, p *Principal) {
	rel, err := s.Store.GetRelease(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteRelease(r.Context(), p.TenantID(), rel.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "release.delete", "release", rel.ID, rel.Name)
	w.WriteHeader(http.StatusNoContent)
}

// handleReleaseRequirements adds and removes scope requirements:
// {"add": [ids], "remove": [ids], "query": "tag:v2"} (the query adds matches).
func (s *Server) handleReleaseRequirements(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	rel, err := s.Store.GetRelease(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req struct {
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
		Query  string   `json:"query"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Query != "" {
		pr, err := s.Store.GetProject(ctx, p.TenantID(), rel.ProjectID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		pc, err := s.loadContext(ctx, p.TenantID(), pr)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		list, err := s.filterRequirements(r, p, pr, pc, req.Query, "", true)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, x := range list {
			req.Add = append(req.Add, x.ID)
		}
	}
	added, removed, err := s.Store.SetReleaseRequirements(ctx, p.TenantID(), rel.ID, req.Add, req.Remove)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "release.scope", "release", rel.ID, fmt.Sprintf("%s: %d added, %d removed", rel.Name, added, removed))
	out, err := s.Store.GetRelease(ctx, p.TenantID(), rel.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": added, "removed": removed, "release": out})
}

func (s *Server) handleReleaseStatus(w http.ResponseWriter, r *http.Request, p *Principal) {
	rep, err := s.releaseStatus(r, p, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// releaseStatus evaluates a release with the defects and risks known to
// the server (see releaseDefects / releaseRisks).
func (s *Server) releaseStatus(r *http.Request, p *Principal, id string) (*store.ReleaseStatusReport, error) {
	ctx := r.Context()
	return s.Store.ReleaseStatus(ctx, p.TenantID(), id,
		func(scope []string, level string) []store.ReleaseItem { return s.releaseDefects(r, p, scope, level) },
		func(scope []string, level string) []store.ReleaseItem { return s.releaseRisks(r, p, scope, level) })
}
