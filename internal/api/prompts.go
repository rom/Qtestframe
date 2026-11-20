package api

import (
	"net/http"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/prompt"
	"github.com/rom/qtestframe/internal/rbac"
)

// MaxPromptItems bounds the requirements and tests in one prompt.
const MaxPromptItems = 200

// handlePrompt: POST /projects/{pid}/prompts — builds a prompt for an AI
// assistant from requirements or tests of a software project (see package
// prompt). Items are chosen by ID, else by query (q), else all of the
// project. Read-only: needs the read permission of the items' kind.
func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	if pr.Type != model.ProjectTypeSoftware {
		s.fail(w, r, badRequest("prompts are available in software projects; set the project type to software"))
		return
	}
	var req struct {
		Kind           string   `json:"kind"`
		Lang           string   `json:"lang"`
		TestType       string   `json:"test_type"`
		Framework      string   `json:"framework"`
		Notes          string   `json:"notes"`
		RequirementIDs []string `json:"requirement_ids"`
		TestIDs        []string `json:"test_ids"`
		Query          string   `json:"q"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	in := prompt.Input{Kind: req.Kind, Lang: req.Lang, TestType: req.TestType, Framework: strings.TrimSpace(req.Framework), Notes: req.Notes,
		ProjectKey: pr.Key, ProjectName: pr.Name, ProjectDescription: pr.Description, TechContext: pr.TechContext}
	if err := prompt.Validate(&in); err != nil {
		s.fail(w, r, badRequest("%v", err))
		return
	}
	if len(in.Framework) > 200 || len(in.Notes) > 8000 {
		s.fail(w, r, badRequest("framework may have at most 200 and notes at most 8000 characters"))
		return
	}
	forReqs := prompt.ForRequirements(in.Kind)
	need := rbac.TestRead
	if forReqs {
		need = rbac.RequirementRd
	}
	if !p.Can(need) {
		writeError(w, http.StatusForbidden, "forbidden", "permission "+string(need)+" required")
		return
	}
	ctx := r.Context()
	pc, err := s.loadContext(ctx, p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// every requirement and test of the project, for keys and titles
	allReqs, err := s.Store.ListRequirements(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	allTests, err := s.Store.ListTests(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reqByID := map[string]*model.Requirement{}
	for i := range allReqs {
		reqByID[allReqs[i].ID] = &allReqs[i]
	}
	testByID := map[string]*model.TestCase{}
	for i := range allTests {
		testByID[allTests[i].ID] = &allTests[i]
	}
	ref := func(key, title string) string { return strings.TrimSpace(key + " " + title) }

	var reqIDs, testIDs []string
	if forReqs {
		if reqIDs, err = s.pickPromptItems(r, p, pr, pc, req.RequirementIDs, req.Query, true); err != nil {
			s.fail(w, r, err)
			return
		}
	} else {
		if testIDs, err = s.pickPromptItems(r, p, pr, pc, req.TestIDs, req.Query, false); err != nil {
			s.fail(w, r, err)
			return
		}
		if in.Kind == prompt.TestStrategy || in.Kind == prompt.ReviewTests {
			// the requirements the tests verify (when the caller may read them)
			if p.Can(rbac.RequirementRd) {
				seen := map[string]bool{}
				for _, tid := range testIDs {
					for _, rid := range pc.t2r[tid] {
						if !seen[rid] {
							seen[rid] = true
							reqIDs = append(reqIDs, rid)
						}
					}
				}
			}
		}
	}
	if n := len(reqIDs) + len(testIDs); n > MaxPromptItems {
		s.fail(w, r, badRequest("%d items selected; a prompt can hold at most %d — narrow the selection", n, MaxPromptItems))
		return
	}

	if len(reqIDs) > 0 {
		rels, err := s.Store.ListRelations(ctx, p.TenantID(), pr.ID, "")
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, id := range reqIDs {
			full, err := s.Store.GetRequirement(ctx, p.TenantID(), id)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			pr := prompt.Requirement{Key: full.Key, Title: full.Title, Description: full.Description, Obligation: full.Obligation, Type: full.Type,
				Priority: full.Priority, Status: full.Status, Source: full.Source, Rationale: full.Rationale, Tags: full.Tags}
			for _, c := range full.Criteria {
				pr.Criteria = append(pr.Criteria, prompt.Criterion{Description: c.Description, Method: c.Method, Threshold: c.Threshold})
			}
			for _, rel := range rels {
				switch id {
				case rel.SourceID:
					pr.Relations = append(pr.Relations, prompt.RelationText(in.Lang, rel.Type, true)+" "+ref(rel.TargetKey, rel.TargetTitle))
				case rel.TargetID:
					pr.Relations = append(pr.Relations, prompt.RelationText(in.Lang, rel.Type, false)+" "+ref(rel.SourceKey, rel.SourceTitle))
				}
			}
			if p.Can(rbac.TestRead) {
				for _, tid := range pc.r2t[id] {
					if t := testByID[tid]; t != nil {
						pr.Tests = append(pr.Tests, ref(t.Key, t.Title))
					}
				}
			}
			in.Requirements = append(in.Requirements, pr)
		}
	}
	if len(testIDs) > 0 {
		deps, err := s.Store.ListTestDependencies(ctx, p.TenantID(), pr.ID, "")
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, id := range testIDs {
			full, err := s.Store.GetTest(ctx, p.TenantID(), id)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			pt := prompt.Test{Key: full.Key, Title: full.Title, Description: full.Description, Preconditions: full.Preconditions, ExecutionType: full.ExecutionType,
				ScriptLanguage: full.ScriptLanguage, Script: full.Script, ExpectedResult: full.ExpectedResult, Priority: full.Priority, Status: full.Status,
				LastResult: pc.last[id], Tags: full.Tags}
			for _, st := range full.Steps {
				step := prompt.Step{Action: st.Action, Expected: st.Expected, Data: st.Data}
				if st.CallTestID != nil && *st.CallTestID != "" {
					step.Calls = ref(st.CallTestKey, st.CallTestTitle)
				}
				pt.Steps = append(pt.Steps, step)
			}
			if p.Can(rbac.RequirementRd) {
				for _, rid := range pc.t2r[id] {
					if rq := reqByID[rid]; rq != nil {
						pt.Requirements = append(pt.Requirements, ref(rq.Key, rq.Title))
					}
				}
			}
			for _, d := range deps {
				if d.TestID == id {
					pt.DependsOn = append(pt.DependsOn, ref(d.DependsOnKey, d.DependsOnTitle))
				}
			}
			in.Tests = append(in.Tests, pt)
		}
	}
	writeJSON(w, http.StatusOK, prompt.Build(in))
}

// pickPromptItems returns the IDs of the chosen requirements (or tests) of
// the project, in list order: the given IDs, else those matching q (all
// when q is empty).
func (s *Server) pickPromptItems(r *http.Request, p *Principal, pr *model.Project, pc *projectContext, ids []string, q string, reqs bool) ([]string, error) {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	if len(ids) > 0 {
		q = ""
	}
	var out []string
	if reqs {
		list, err := s.filterRequirements(r, p, pr, pc, q, "", true)
		if err != nil {
			return nil, err
		}
		for _, x := range list {
			if len(ids) == 0 || want[x.ID] {
				out = append(out, x.ID)
			}
		}
	} else {
		list, err := s.filterTests(r, p, pr, pc, q, "", true)
		if err != nil {
			return nil, err
		}
		for _, x := range list {
			if len(ids) == 0 || want[x.ID] {
				out = append(out, x.ID)
			}
		}
	}
	if len(ids) > 0 && len(out) < len(want) {
		return nil, badRequest("%d of the selected items do not belong to this project", len(want)-len(out))
	}
	return out, nil
}
