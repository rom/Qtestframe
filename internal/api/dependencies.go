package api

import (
	"net/http"

	"github.com/rom/qtestframe/internal/model"
)

func (s *Server) routesDependencies(h routeFunc) {
	h("GET /api/v1/tests/{id}/dependencies", s.handleTestDependencies, "test.read")
	h("POST /api/v1/tests/{id}/dependencies", s.handleAddTestDependency, "test.write")
	h("DELETE /api/v1/test-dependencies/{id}", s.handleDeleteTestDependency, "test.write")
	h("GET /api/v1/projects/{pid}/test-dependencies", s.handleProjectTestDependencies, "test.read")
	h("GET /api/v1/projects/{pid}/readiness", s.handleTestReadiness, "test.read")
	h("GET /api/v1/projects/{pid}/graph", s.handleGraph, "project.read")
	h("GET /api/v1/projects/{pid}/tags", s.handleProjectTags, "project.read")
}

func (s *Server) handleTestDependencies(w http.ResponseWriter, r *http.Request, p *Principal) {
	t, err := s.Store.GetTest(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	deps, err := s.Store.ListTestDependencies(r.Context(), p.TenantID(), t.ProjectID, t.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	last, err := s.Store.LastResults(r.Context(), p.TenantID(), t.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for i := range deps {
		other := deps[i].DependsOnID
		if deps[i].Direction == "incoming" {
			other = deps[i].TestID
		}
		deps[i].OtherLastResult = last[other]
		if deps[i].OtherLastResult == "" {
			deps[i].OtherLastResult = model.ResultNotRun
		}
	}
	writeJSON(w, http.StatusOK, listOf(deps))
}

func (s *Server) handleAddTestDependency(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		DependsOnID string `json:"depends_on_id"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	d := &model.TestDependency{TestID: r.PathValue("id"), DependsOnID: req.DependsOnID, CreatedBy: p.User.Email}
	if err := s.Store.AddTestDependency(r.Context(), p.TenantID(), d); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "test.dependency.add", "test", d.TestID, d.TestKey+" depends on "+d.DependsOnKey)
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) handleDeleteTestDependency(w http.ResponseWriter, r *http.Request, p *Principal) {
	d, err := s.Store.GetTestDependency(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteTestDependency(r.Context(), p.TenantID(), d.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "test.dependency.remove", "test", d.TestID, d.TestKey+" no longer depends on "+d.DependsOnKey)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleProjectTestDependencies(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	deps, err := s.Store.ListTestDependencies(r.Context(), p.TenantID(), pr.ID, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(deps))
}
