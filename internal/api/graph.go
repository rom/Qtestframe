package api

import (
	"net/http"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

// GraphNode is an item in a dependency graph.
type GraphNode struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Title  string `json:"title"`
	Status string `json:"status"`
	// Result: coverage of a requirement, last result of a test.
	Result string `json:"result"`
	Folder string `json:"folder,omitempty"`
}

// GraphEdge is a directed relation between two nodes (source → target, as
// stored: "A depends on B" is A → B).
type GraphEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

// handleGraph: GET /projects/{pid}/graph?kind=requirement|test — the
// requirement relations or the test dependencies and shared-step calls of a
// project.
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = model.KindRequirement
	}
	if kind != model.KindRequirement && kind != model.KindTest {
		s.fail(w, r, badRequest("kind must be requirement or test"))
		return
	}
	need := "requirement.read"
	if kind == model.KindTest {
		need = "test.read"
	}
	if !p.Can(rbac.Permission(need)) {
		writeError(w, http.StatusForbidden, "forbidden", "permission "+need+" required")
		return
	}
	pc, err := s.loadContext(r.Context(), p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	nodes := []GraphNode{}
	edges := []GraphEdge{}
	if kind == model.KindRequirement {
		reqs, err := s.Store.ListRequirements(r.Context(), p.TenantID(), pr.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for i := range reqs {
			pc.decorateRequirement(&reqs[i])
			nodes = append(nodes, GraphNode{ID: reqs[i].ID, Key: reqs[i].Key, Title: reqs[i].Title, Status: reqs[i].Status, Result: reqs[i].Coverage, Folder: reqs[i].FolderPath})
		}
		rels, err := s.Store.ListRelations(r.Context(), p.TenantID(), pr.ID, "")
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, rel := range rels {
			edges = append(edges, GraphEdge{ID: rel.ID, Source: rel.SourceID, Target: rel.TargetID, Type: rel.Type})
		}
	} else {
		tests, err := s.Store.ListTests(r.Context(), p.TenantID(), pr.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for i := range tests {
			t := &tests[i]
			pc.decorateTest(t)
			nodes = append(nodes, GraphNode{ID: t.ID, Key: t.Key, Title: t.Title, Status: t.Status, Result: t.LastResult, Folder: t.FolderPath})
			seen := map[string]bool{}
			for _, st := range t.Steps {
				if st.CallTestID != nil && !seen[*st.CallTestID] {
					seen[*st.CallTestID] = true
					edges = append(edges, GraphEdge{ID: "call:" + t.ID + ":" + *st.CallTestID, Source: t.ID, Target: *st.CallTestID, Type: "calls"})
				}
			}
		}
		for _, d := range pc.deps {
			edges = append(edges, GraphEdge{ID: d.ID, Source: d.TestID, Target: d.DependsOnID, Type: "depends_on"})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"kind": kind, "nodes": nodes, "edges": edges})
}

// handleProjectTags: GET /projects/{pid}/tags — every tag of the project
// with how many requirements, tests and documents carry it.
func (s *Server) handleProjectTags(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	tags, err := s.Store.ProjectTagCounts(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(tags))
}
