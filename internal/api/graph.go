package api

import (
	"net/http"
	"strings"

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

// handleBulkTags: POST /projects/{pid}/tags/bulk
// {"tags": [...], "action": "rename" | "merge" | "delete", "to"}.
// rename takes one tag, merge several (into to, which may be one of them
// or a new name), delete removes the tags from every item. Requirements and
// tests are changed (needs requirement.write and test.write); documents too
// with project.update. → {"requirements", "tests", "documents"} (counts).
func (s *Server) handleBulkTags(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Tags   []string `json:"tags"`
		Action string   `json:"action"`
		To     string   `json:"to"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if !p.Can(rbac.RequirementWr) || !p.Can(rbac.TestWrite) {
		writeError(w, http.StatusForbidden, "forbidden", "changing tags across the project needs requirement.write and test.write")
		return
	}
	to := strings.TrimSpace(req.To)
	switch req.Action {
	case "rename":
		if len(req.Tags) != 1 || to == "" {
			s.fail(w, r, badRequest("rename takes one tag and a new name"))
			return
		}
	case "merge":
		if len(req.Tags) < 2 || to == "" {
			s.fail(w, r, badRequest("merge takes at least two tags and the name to keep"))
			return
		}
	case "delete":
		to = ""
	default:
		s.fail(w, r, badRequest("action must be rename, merge or delete"))
		return
	}
	changed, err := s.Store.ReplaceTags(r.Context(), p.TenantID(), pr.ID, req.Tags, to, p.Can(rbac.ProjectUpdate))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, id := range changed[model.KindRequirement] {
		s.record(r.Context(), p, model.KindRequirement, id, "bulk_update")
	}
	for _, id := range changed[model.KindTest] {
		s.record(r.Context(), p, model.KindTest, id, "bulk_update")
	}
	s.audit(r.Context(), p, "tag."+req.Action, "project", pr.ID, strings.Join(req.Tags, ", ")+" → "+to)
	writeJSON(w, http.StatusOK, map[string]int{"requirements": len(changed[model.KindRequirement]), "tests": len(changed[model.KindTest]), "documents": len(changed["file"])})
}
