package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/prompt"
	"github.com/rom/qtestframe/internal/rbac"
)

// MaxPromptItems bounds the requirements and tests in one prompt;
// MaxSolutionItems the requirements of a solution prompt (which can be
// split into parts).
const (
	MaxPromptItems   = 200
	MaxSolutionItems = 2000
)

// promptRequest is the body of POST /projects/{pid}/prompts.
type promptRequest struct {
	Kind           string   `json:"kind"`
	Lang           string   `json:"lang"`
	TestType       string   `json:"test_type"`
	Framework      string   `json:"framework"`
	Notes          string   `json:"notes"`
	RequirementIDs []string `json:"requirement_ids"`
	TestIDs        []string `json:"test_ids"`
	Query          string   `json:"q"`
	// Scope of requirement prompts (combined with the above): a folder
	// (with its subfolders; "root" = no folder), a release, a variant (id
	// or name).
	FolderID  string `json:"folder_id"`
	ReleaseID string `json:"release_id"`
	Variant   string `json:"variant"`
	// Solution prompts: compact mode, split, token budget per part and
	// output formats (nil: the kind's defaults).
	Compact   bool     `json:"compact"`
	Split     string   `json:"split"`
	MaxTokens int      `json:"max_tokens"`
	Formats   []string `json:"formats"`
	// Format of the answer: "json" (default) or "zip" (the parts as
	// Markdown files).
	Format string `json:"format"`
	// Part to send to the AI assistance (POST …/prompts/send; 1-based, 0
	// = the first).
	Part int `json:"part"`
}

// handlePrompt: POST /projects/{pid}/prompts — builds a prompt for an AI
// assistant from requirements or tests of a software project (see package
// prompt). Items are chosen by ID, else by query (q), else all of the
// project; requirement prompts can further be limited to a folder, a
// release or a variant. Solution prompts start with the project's
// solution preamble. Read-only: needs the read permission of the items'
// kind.
func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req promptRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	pr, res, ok := s.buildPrompt(w, r, p, &req)
	if !ok {
		return
	}
	if req.Format == "zip" {
		writePromptZip(w, pr.Key, res)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// buildPrompt builds the prompt of a request; on an error it has answered
// and ok is false.
func (s *Server) buildPrompt(w http.ResponseWriter, r *http.Request, p *Principal, req *promptRequest) (*model.Project, prompt.Result, bool) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return nil, prompt.Result{}, false
	}
	if pr.Type != model.ProjectTypeSoftware {
		s.fail(w, r, badRequest("prompts are available in software projects; set the project type to software"))
		return nil, prompt.Result{}, false
	}
	in := prompt.Input{Kind: req.Kind, Lang: req.Lang, TestType: req.TestType, Framework: strings.TrimSpace(req.Framework), Notes: req.Notes,
		ProjectKey: pr.Key, ProjectName: pr.Name, ProjectDescription: pr.Description, TechContext: pr.TechContext,
		Compact: req.Compact, Split: req.Split, MaxTokens: req.MaxTokens, Formats: req.Formats}
	solution := prompt.IsSolution(req.Kind)
	if solution && in.Formats == nil {
		in.Formats = prompt.DefaultFormats(req.Kind)
	}
	if err := prompt.Validate(&in); err != nil {
		s.fail(w, r, badRequest("%v", err))
		return nil, prompt.Result{}, false
	}
	if len(in.Framework) > 200 || len(in.Notes) > 8000 {
		s.fail(w, r, badRequest("framework may have at most 200 and notes at most 8000 characters"))
		return nil, prompt.Result{}, false
	}
	if req.Format != "" && req.Format != "json" && req.Format != "zip" {
		s.fail(w, r, badRequest("format must be json or zip"))
		return nil, prompt.Result{}, false
	}
	forReqs := prompt.ForRequirements(in.Kind)
	need := rbac.TestRead
	if forReqs {
		need = rbac.RequirementRd
	}
	if !p.Can(need) {
		writeError(w, http.StatusForbidden, "forbidden", "permission "+string(need)+" required")
		return nil, prompt.Result{}, false
	}
	if !forReqs && (req.ReleaseID != "" || req.Variant != "") {
		s.fail(w, r, badRequest("release_id and variant apply to requirement prompts"))
		return nil, prompt.Result{}, false
	}
	ctx := r.Context()
	pc, err := s.loadContext(ctx, p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return nil, prompt.Result{}, false
	}
	// every requirement and test of the project, for keys and titles
	allReqs, err := s.Store.ListRequirements(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return nil, prompt.Result{}, false
	}
	allTests, err := s.Store.ListTests(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return nil, prompt.Result{}, false
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
	var sc prompt.Scope
	if forReqs {
		if reqIDs, sc, err = s.pickPromptRequirements(r, p, pr, pc, req); err != nil {
			s.fail(w, r, err)
			return nil, prompt.Result{}, false
		}
	} else {
		if testIDs, err = s.pickPromptItems(r, p, pr, pc, req.TestIDs, req.Query, req.FolderID, false); err != nil {
			s.fail(w, r, err)
			return nil, prompt.Result{}, false
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
	limit := MaxPromptItems
	if solution {
		limit = MaxSolutionItems
	}
	if n := len(reqIDs) + len(testIDs); n > limit {
		s.fail(w, r, badRequest("%d items selected; a prompt can hold at most %d — narrow the selection", n, limit))
		return nil, prompt.Result{}, false
	}

	if len(reqIDs) > 0 {
		rels, err := s.Store.ListRelations(ctx, p.TenantID(), pr.ID, "")
		if err != nil {
			s.fail(w, r, err)
			return nil, prompt.Result{}, false
		}
		var risksOf map[string][]prompt.Risk
		var refsOf func(string) []string
		if solution {
			if risksOf, err = s.promptRisks(r, p, pr); err != nil {
				s.fail(w, r, err)
				return nil, prompt.Result{}, false
			}
			if cat, err := s.catalogue(); err == nil && cat != nil {
				refsOf = func(id string) []string {
					if c := cat.Requirements[id]; c != nil {
						return c.Refs
					}
					return nil
				}
			}
		}
		for _, id := range reqIDs {
			full, err := s.Store.GetRequirement(ctx, p.TenantID(), id)
			if err != nil {
				s.fail(w, r, err)
				return nil, prompt.Result{}, false
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
					if rel.Type == "depends_on" {
						pr.DependsOn = append(pr.DependsOn, rel.TargetKey)
					}
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
			if solution {
				pr.Folder = pc.folderPath(full.FolderID)
				pr.Group = topFolderName(pc, full.FolderID)
				pr.Risks = risksOf[id]
				pr.D3fend, pr.Emb3d = full.D3fend, full.Emb3d
				if refsOf != nil && full.CatalogRef != "" {
					pr.Standards = refsOf(full.CatalogRef)
				}
			}
			in.Requirements = append(in.Requirements, pr)
		}
	}
	if len(testIDs) > 0 {
		deps, err := s.Store.ListTestDependencies(ctx, p.TenantID(), pr.ID, "")
		if err != nil {
			s.fail(w, r, err)
			return nil, prompt.Result{}, false
		}
		for _, id := range testIDs {
			full, err := s.Store.GetTest(ctx, p.TenantID(), id)
			if err != nil {
				s.fail(w, r, err)
				return nil, prompt.Result{}, false
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
	if solution {
		if in.Preamble, err = s.Store.SolutionPreamble(ctx, p.TenantID(), pr.ID); err != nil {
			s.fail(w, r, err)
			return nil, prompt.Result{}, false
		}
		if in.Summary, err = s.promptSummary(r, p, pr, pc, allReqs, len(allTests), in.Requirements, sc, in.Lang); err != nil {
			s.fail(w, r, err)
			return nil, prompt.Result{}, false
		}
	}
	return pr, prompt.Build(in), true
}

// topFolderName is the name of the top-level folder an item is in ("" none).
func topFolderName(pc *projectContext, folderID *string) string {
	if folderID == nil {
		return ""
	}
	byID := map[string]*model.Folder{}
	for i := range pc.folders {
		byID[pc.folders[i].ID] = &pc.folders[i]
	}
	f := byID[*folderID]
	for depth := 0; f != nil && f.ParentID != nil && depth < 64; depth++ {
		parent := byID[*f.ParentID]
		if parent == nil {
			break
		}
		f = parent
	}
	if f == nil {
		return ""
	}
	return f.Name
}

// pickPromptRequirements returns the requirements of a requirement prompt:
// by id or query (pickPromptItems), within the folder, release and
// variant of the request; and the scope for the prompt's summary.
func (s *Server) pickPromptRequirements(r *http.Request, p *Principal, pr *model.Project, pc *projectContext, req *promptRequest) ([]string, prompt.Scope, error) {
	var sc prompt.Scope
	ids, err := s.pickPromptItems(r, p, pr, pc, req.RequirementIDs, req.Query, req.FolderID, true)
	if err != nil {
		return nil, sc, err
	}
	if len(req.RequirementIDs) > 0 {
		sc.Selected = len(ids)
	} else {
		sc.Query = strings.TrimSpace(req.Query)
	}
	if req.FolderID != "" {
		if req.FolderID == "root" || req.FolderID == "none" {
			sc.Folder = "—"
		} else if sc.Folder = pc.paths[req.FolderID]; sc.Folder == "" {
			return nil, sc, badRequest("unknown folder %q", req.FolderID)
		}
	}
	keep := func(pred func(string) bool) {
		out := ids[:0]
		for _, id := range ids {
			if pred(id) {
				out = append(out, id)
			}
		}
		ids = out
	}
	if req.ReleaseID != "" {
		rel, err := s.Store.GetRelease(r.Context(), p.TenantID(), req.ReleaseID)
		if err != nil || rel.ProjectID != pr.ID {
			return nil, sc, badRequest("unknown release %q", req.ReleaseID)
		}
		in := map[string]bool{}
		for _, id := range rel.RequirementIDs {
			in[id] = true
		}
		keep(func(id string) bool { return in[id] })
		sc.Release = rel.Name
		if rel.Version != "" && rel.Version != rel.Name {
			sc.Release += " " + rel.Version
		}
	}
	if v := strings.TrimSpace(req.Variant); v != "" {
		variant := pc.variants.Find(v)
		if variant == nil {
			return nil, sc, badRequest("unknown variant %q", v)
		}
		vc, err := s.variantCoverage(r.Context(), p.TenantID(), pc)
		if err != nil {
			return nil, sc, err
		}
		keep(func(id string) bool { return vc.AppliesTo(id, variant.ID) })
		sc.Variant = variant.Name
	}
	return ids, sc, nil
}

// promptRisks maps requirement ids to the project risks they mitigate.
func (s *Server) promptRisks(r *http.Request, p *Principal, pr *model.Project) (map[string][]prompt.Risk, error) {
	risks, err := s.Store.ListRisks(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		return nil, err
	}
	out := map[string][]prompt.Risk{}
	for _, k := range risks {
		pk := prompt.Risk{Key: k.Key, Title: k.Title, Level: k.Level, Mitigation: k.Mitigation, Attack: k.Attack, Emb3d: k.Emb3d}
		for _, id := range k.RequirementIDs {
			out[id] = append(out[id], pk)
		}
	}
	return out, nil
}

// promptSummary is the project summary of a solution prompt.
func (s *Server) promptSummary(r *http.Request, p *Principal, pr *model.Project, pc *projectContext, all []model.Requirement, tests int,
	reqs []prompt.Requirement, sc prompt.Scope, lang string) (prompt.Summary, error) {
	sum := prompt.Summary{Scope: prompt.ScopeLabel(lang, sc), Total: len(all), InScope: len(reqs)}
	if p.Can(rbac.TestRead) {
		sum.Tests = tests
	}
	idx := map[string]int{}
	for _, rq := range reqs {
		name := rq.Group
		if name == "" {
			continue
		}
		i, ok := idx[name]
		if !ok {
			i = len(sum.Folders)
			idx[name] = i
			sum.Folders = append(sum.Folders, prompt.Count{Name: name})
		}
		sum.Folders[i].N++
	}
	rels, err := s.Store.ListReleases(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		return sum, err
	}
	for i, rel := range rels {
		if i == 12 {
			sum.Releases = append(sum.Releases, fmt.Sprintf("… (%d)", len(rels)))
			break
		}
		x := rel.Name
		if rel.Version != "" && rel.Version != rel.Name {
			x += " " + rel.Version
		}
		sum.Releases = append(sum.Releases, x+" ("+rel.Status+")")
	}
	if pc.variants != nil {
		vs := pc.variants.List()
		for i, v := range vs {
			if i == 20 {
				sum.Variants = append(sum.Variants, fmt.Sprintf("… (%d)", len(vs)))
				break
			}
			sum.Variants = append(sum.Variants, v.Name)
		}
	}
	return sum, nil
}

// pickPromptItems returns the IDs of the chosen requirements (or tests) of
// the project, in list order: the given IDs, else those matching q (all
// when q is empty), within the folder (and its subfolders) when one is
// given.
func (s *Server) pickPromptItems(r *http.Request, p *Principal, pr *model.Project, pc *projectContext, ids []string, q, folder string, reqs bool) ([]string, error) {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	if len(ids) > 0 {
		q = ""
	}
	var out []string
	if reqs {
		list, err := s.filterRequirements(r, p, pr, pc, q, folder, true)
		if err != nil {
			return nil, err
		}
		for _, x := range list {
			if len(ids) == 0 || want[x.ID] {
				out = append(out, x.ID)
			}
		}
	} else {
		list, err := s.filterTests(r, p, pr, pc, q, folder, true)
		if err != nil {
			return nil, err
		}
		for _, x := range list {
			if len(ids) == 0 || want[x.ID] {
				out = append(out, x.ID)
			}
		}
	}
	if len(ids) > 0 && folder == "" && len(out) < len(want) {
		return nil, badRequest("%d of the selected items do not belong to this project", len(want)-len(out))
	}
	return out, nil
}
