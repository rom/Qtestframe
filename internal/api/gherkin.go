package api

import (
	"archive/zip"
	"bytes"
	"errors"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/gherkin"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// BDD authoring (see docs/api.md "Gherkin tests" and internal/gherkin):
// checking Gherkin while it is edited, converting between the step and
// Gherkin formats, the step library of a project (usage, undefined steps,
// renaming a step in every test), importing .feature files as tests and
// exporting tests as .feature files, and the latest result of each
// Gherkin step.

// Limits of feature imports and exports.
const (
	MaxFeatureFiles  = 200
	MaxFeatureTests  = 2000
	maxGherkinIssues = 200
)

func (s *Server) routesGherkin(h routeFunc) {
	h("POST /api/v1/projects/{pid}/gherkin/check", s.handleGherkinCheck, "test.read")
	h("POST /api/v1/projects/{pid}/gherkin/convert", s.handleGherkinConvert, "test.read")
	h("GET /api/v1/projects/{pid}/step-definitions", s.handleListStepDefs, "test.read")
	h("POST /api/v1/projects/{pid}/step-definitions", s.handleCreateStepDef, "test.write")
	h("PATCH /api/v1/step-definitions/{id}", s.handleUpdateStepDef, "test.write")
	h("DELETE /api/v1/step-definitions/{id}", s.handleDeleteStepDef, "test.write")
	h("POST /api/v1/step-definitions/{id}/rename", s.handleRenameStepDef, "test.write")
	h("POST /api/v1/projects/{pid}/tests/features/import", s.handleImportFeatures, "test.write")
	h("GET /api/v1/projects/{pid}/tests/features", s.handleExportFeatures, "test.read")
	h("POST /api/v1/projects/{pid}/tests/features/export", s.handleExportFeatures, "test.read")
	h("GET /api/v1/tests/{id}/gherkin-results", s.handleGherkinResults, "test.read")
}

// ---------------------------------------------------------------- check

type gherkinCheckReq struct {
	Source string `json:"source"`
}

// gherkinStepInfo is a step of a checked document.
type gherkinStepInfo struct {
	Line    int    `json:"line"`
	Keyword string `json:"keyword"`
	Text    string `json:"text"`
	// Defined: a step of the library matches it (its id in Definition).
	Defined    bool   `json:"defined"`
	Definition string `json:"definition,omitempty"`
	// Suggestion: a Cucumber expression for an undefined step.
	Suggestion string `json:"suggestion,omitempty"`
}

type gherkinCheckResp struct {
	Valid    bool              `json:"valid"`
	Errors   []*gherkin.Error  `json:"errors"`
	Language string            `json:"language"`
	Feature  string            `json:"feature"`
	Steps    []gherkinStepInfo `json:"steps"`
	// Scenarios: the executable scenarios (one per Examples row).
	Scenarios []gherkinScenarioInfo `json:"scenarios"`
	// Undefined: steps no definition of the step library matches.
	Undefined int `json:"undefined"`
}

type gherkinScenarioInfo struct {
	Name  string `json:"name"`
	Line  int    `json:"line"`
	Steps int    `json:"steps"`
}

// handleGherkinCheck: POST /projects/{pid}/gherkin/check {"source"} —
// parses a feature and matches its steps against the project's library
// (what the editor shows inline). Always 200; errors are in the body.
func (s *Server) handleGherkinCheck(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var in gherkinCheckReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	defs, err := s.Store.ListStepDefinitions(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, checkGherkin(in.Source, defs))
}

func checkGherkin(src string, defs []model.StepDefinition) gherkinCheckResp {
	out := gherkinCheckResp{Errors: []*gherkin.Error{}, Steps: []gherkinStepInfo{}, Scenarios: []gherkinScenarioInfo{}}
	doc, err := gherkin.Parse(src)
	out.Language = doc.Language
	var es gherkin.Errors
	if errors.As(err, &es) {
		out.Errors = es
		if len(out.Errors) > maxGherkinIssues {
			out.Errors = out.Errors[:maxGherkinIssues]
		}
	}
	out.Valid = len(out.Errors) == 0
	if doc.Feature == nil {
		return out
	}
	out.Feature = doc.Feature.Name
	exprs, _ := gherkin.CompileAll(defsOf(defs))
	for _, st := range doc.Feature.AllSteps() {
		info := gherkinStepInfo{Line: st.Line, Keyword: st.Keyword, Text: st.Text}
		for _, d := range defs {
			if e := exprs[d.ID]; e != nil {
				if _, ok := e.Match(st.Text); ok {
					info.Defined, info.Definition = true, d.ID
					break
				}
			}
		}
		if !info.Defined && st.Text != "" {
			info.Suggestion = gherkin.Suggest(st.Text)
			out.Undefined++
		}
		out.Steps = append(out.Steps, info)
	}
	if out.Valid {
		for _, in := range gherkin.Expand(doc) {
			out.Scenarios = append(out.Scenarios, gherkinScenarioInfo{Name: in.Name, Line: in.ScenarioLine, Steps: len(in.Steps)})
		}
	}
	return out
}

func defsOf(list []model.StepDefinition) []gherkin.Definition {
	out := make([]gherkin.Definition, len(list))
	for i, d := range list {
		out[i] = gherkin.Definition{ID: d.ID, Pattern: d.Pattern, Kind: d.Kind}
	}
	return out
}

// ---------------------------------------------------------------- convert

type gherkinConvertReq struct {
	// To: "gherkin" (from the step fields) or "steps" (from gherkin).
	To            string           `json:"to"`
	Language      string           `json:"language"`
	Title         string           `json:"title"`
	Description   string           `json:"description"`
	Preconditions string           `json:"preconditions"`
	Steps         []model.TestStep `json:"steps"`
	Parameters    []string         `json:"parameters"`
	DataRows      []model.DataRow  `json:"data_rows"`
	Tags          []string         `json:"tags"`
	Gherkin       string           `json:"gherkin"`
}

type gherkinConvertResp struct {
	Gherkin       string           `json:"gherkin,omitempty"`
	Preconditions string           `json:"preconditions"`
	Steps         []model.TestStep `json:"steps"`
	Parameters    []string         `json:"parameters"`
	DataRows      []model.DataRow  `json:"data_rows"`
	Warnings      []string         `json:"warnings"`
}

// handleGherkinConvert: POST /projects/{pid}/gherkin/convert — converts a
// test between the formats (best effort); nothing is saved.
func (s *Server) handleGherkinConvert(w http.ResponseWriter, r *http.Request, p *Principal) {
	if _, ok := s.project(w, r, p); !ok {
		return
	}
	var in gherkinConvertReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := convertGherkin(in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func convertGherkin(in gherkinConvertReq) (*gherkinConvertResp, error) {
	out := &gherkinConvertResp{Steps: []model.TestStep{}, Parameters: []string{}, DataRows: []model.DataRow{}, Warnings: []string{}}
	switch in.To {
	case model.FormatGherkin:
		pt := gherkin.PlainTest{Title: in.Title, Description: in.Description, Preconditions: in.Preconditions, Parameters: in.Parameters, Tags: in.Tags}
		for _, st := range in.Steps {
			ps := gherkin.PlainStep{Action: st.Action, Expected: st.Expected, Data: st.Data}
			if st.CallTestID != nil && *st.CallTestID != "" {
				ps.Call = strings.TrimSpace(st.CallTestKey + " " + st.CallTestTitle)
				if ps.Call == "" {
					ps.Call = "another test"
				}
				out.Warnings = append(out.Warnings, "called tests become a step that names them")
			}
			pt.Steps = append(pt.Steps, ps)
		}
		for _, r := range in.DataRows {
			pt.Rows = append(pt.Rows, gherkin.PlainRow{Name: r.Name, Values: r.Values})
		}
		lang := in.Language
		if lang == "" {
			lang = "en"
		}
		out.Gherkin = gherkin.FromPlain(pt, lang)
	case model.FormatSteps:
		doc, err := gherkin.Parse(in.Gherkin)
		if err != nil {
			return nil, store.Invalidf("gherkin: %v", err)
		}
		pt, warn := gherkin.ToPlain(doc)
		out.Preconditions = pt.Preconditions
		for i, st := range pt.Steps {
			out.Steps = append(out.Steps, model.TestStep{Position: i + 1, Action: st.Action, Expected: st.Expected, Data: st.Data})
		}
		out.Parameters = pt.Parameters
		for _, r := range pt.Rows {
			out.DataRows = append(out.DataRows, model.DataRow{Name: r.Name, Values: r.Values})
		}
		out.Warnings = append(out.Warnings, warn...)
	default:
		return nil, badRequest("to must be gherkin or steps")
	}
	out.Warnings = uniqStrings(out.Warnings)
	return out, nil
}

// ---------------------------------------------------------------- step library

type stepDefReq struct {
	Keyword     *string `json:"keyword"`
	Pattern     *string `json:"pattern"`
	Kind        *string `json:"kind"`
	Description *string `json:"description"`
}

func (in *stepDefReq) apply(d *model.StepDefinition) {
	for _, f := range []struct{ src, dst *string }{{in.Keyword, &d.Keyword}, {in.Pattern, &d.Pattern}, {in.Kind, &d.Kind}, {in.Description, &d.Description}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
}

// handleListStepDefs: GET /projects/{pid}/step-definitions — the library
// with usage counts, where each step is used and the undefined steps.
func (s *Server) handleListStepDefs(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	lib, err := s.Store.StepLibraryUsage(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, lib)
}

func (s *Server) handleCreateStepDef(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var in stepDefReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	d := &model.StepDefinition{ProjectID: pr.ID, CreatedBy: p.User.Email}
	in.apply(d)
	if err := s.Store.CreateStepDefinition(r.Context(), p.TenantID(), d); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "step.create", "step_definition", d.ID, d.Pattern)
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) handleUpdateStepDef(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	d, err := s.Store.GetStepDefinition(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in stepDefReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	in.apply(d)
	if err := s.Store.UpdateStepDefinition(ctx, p.TenantID(), d); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "step.update", "step_definition", d.ID, d.Pattern)
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDeleteStepDef(w http.ResponseWriter, r *http.Request, p *Principal) {
	d, err := s.Store.GetStepDefinition(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteStepDefinition(r.Context(), p.TenantID(), d.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "step.delete", "step_definition", d.ID, d.Pattern)
	w.WriteHeader(http.StatusNoContent)
}

// handleRenameStepDef: POST /step-definitions/{id}/rename {"pattern"} —
// changes the step and rewrites it in every Gherkin test of the project.
func (s *Server) handleRenameStepDef(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	var in struct {
		Pattern string `json:"pattern"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	d, tests, err := s.Store.RenameStepDefinition(ctx, p.TenantID(), r.PathValue("id"), in.Pattern)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	steps := 0
	for _, t := range tests {
		steps += t.Steps
		s.audit(ctx, p, "test.update", "test", t.ID, t.Key+" (step renamed)")
		s.recordTest(ctx, p, t.ID, "update")
	}
	s.audit(ctx, p, "step.rename", "step_definition", d.ID, d.Pattern+" ("+strconv.Itoa(steps)+" steps in "+strconv.Itoa(len(tests))+" tests)")
	writeJSON(w, http.StatusOK, map[string]any{"item": d, "tests": tests, "steps": steps})
}

// ---------------------------------------------------------------- import

type featureFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type featureImportReq struct {
	Files []featureFile `json:"files"`
	// Mode: "scenario" (one test per scenario, the default) or "feature".
	Mode     string  `json:"mode"`
	FolderID *string `json:"folder_id"`
	// DryRun: only report what would be created.
	DryRun bool `json:"dry_run"`
}

type featureFileError struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

type importedFeatureTest struct {
	ID    string   `json:"id,omitempty"`
	Key   string   `json:"key,omitempty"`
	Title string   `json:"title"`
	File  string   `json:"file"`
	Tags  []string `json:"tags"`
	// Requirements: the keys of the requirements it is linked to.
	Requirements []string `json:"requirements"`
}

type featureImportResp struct {
	Tests  []importedFeatureTest `json:"tests"`
	Links  int                   `json:"links"`
	Errors []featureFileError    `json:"errors"`
	DryRun bool                  `json:"dry_run"`
}

var reqTagPrefix = regexp.MustCompile(`(?i)^(req|requirement)[:=]`)

// requirementOfTag returns the id and key of the requirement a tag names:
// its key (@SHOP-REQ-12), with a req: or req= prefix, or without the
// project key (@REQ-12 in project SHOP).
func requirementOfTag(tag, projectKey string, byKey map[string]model.Requirement) (model.Requirement, bool) {
	k := strings.ToUpper(reqTagPrefix.ReplaceAllString(strings.TrimPrefix(tag, "@"), ""))
	if q, ok := byKey[k]; ok {
		return q, true
	}
	q, ok := byKey[strings.ToUpper(projectKey)+"-"+k]
	return q, ok
}

// handleImportFeatures: POST /projects/{pid}/tests/features/import — makes
// Gherkin tests of .feature files: one per scenario or per feature. Tags
// become tags, and tags that name requirements of the project become
// trace links. Files with errors are reported and skipped.
func (s *Server) handleImportFeatures(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	ctx := r.Context()
	var in featureImportReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(in.Files) == 0 {
		s.fail(w, r, badRequest("no files"))
		return
	}
	if len(in.Files) > MaxFeatureFiles {
		s.fail(w, r, badRequest("at most %d files per import", MaxFeatureFiles))
		return
	}
	perScenario := true
	switch in.Mode {
	case "", "scenario":
	case "feature":
		perScenario = false
	default:
		s.fail(w, r, badRequest("mode must be scenario or feature"))
		return
	}
	reqs, err := s.Store.ListRequirements(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	byKey := map[string]model.Requirement{}
	for _, q := range reqs {
		byKey[strings.ToUpper(q.Key)] = q
	}
	out := featureImportResp{Tests: []importedFeatureTest{}, Errors: []featureFileError{}, DryRun: in.DryRun}
	type planned struct {
		part gherkin.Part
		file string
		reqs []model.Requirement
		tags []string
	}
	var plan []planned
	for _, f := range in.Files {
		name := path.Base(strings.ReplaceAll(f.Name, `\`, "/"))
		doc, err := gherkin.Parse(f.Content)
		if err != nil {
			var es gherkin.Errors
			if errors.As(err, &es) {
				for _, e := range es {
					out.Errors = append(out.Errors, featureFileError{File: name, Line: e.Line, Message: e.Message})
				}
			}
			continue
		}
		for _, part := range gherkin.Split(doc, f.Content, perScenario) {
			pl := planned{part: part, file: name, tags: []string{}}
			for _, tg := range part.Tags {
				if q, ok := requirementOfTag(tg, pr.Key, byKey); ok {
					pl.reqs = append(pl.reqs, q)
				} else {
					pl.tags = append(pl.tags, tg)
				}
			}
			plan = append(plan, pl)
		}
	}
	if len(plan) > MaxFeatureTests {
		s.fail(w, r, badRequest("%d tests; at most %d per import", len(plan), MaxFeatureTests))
		return
	}
	for _, pl := range plan {
		it := importedFeatureTest{Title: testTitle(pl.part.Title), File: pl.file, Tags: pl.tags, Requirements: []string{}}
		for _, q := range pl.reqs {
			it.Requirements = append(it.Requirements, q.Key)
		}
		if !in.DryRun {
			t := &model.TestCase{ProjectID: pr.ID, FolderID: in.FolderID, Title: it.Title, Description: pl.part.Description, Tags: pl.tags,
				Format: model.FormatGherkin, Gherkin: pl.part.Source, ExecutionType: model.ExecManual, Status: "draft", CreatedBy: p.User.Email}
			if err := s.Store.CreateTest(store.LenientCustomFields(ctx), p.TenantID(), t); err != nil {
				s.fail(w, r, err)
				return
			}
			s.audit(ctx, p, "test.create", "test", t.ID, t.Key+" (feature import)")
			s.recordTest(ctx, p, t.ID, "create")
			it.ID, it.Key = t.ID, t.Key
			var linked []string
			for _, q := range pl.reqs {
				l := &model.TraceLink{ProjectID: pr.ID, RequirementID: q.ID, TestCaseID: t.ID}
				if err := s.Store.CreateLink(ctx, p.TenantID(), l); err != nil {
					if errors.Is(err, store.ErrConflict) {
						continue
					}
					s.fail(w, r, err)
					return
				}
				out.Links++
				linked = append(linked, q.ID)
				s.audit(ctx, p, "link.create", "link", l.ID, q.Key+" -> "+t.Key)
			}
			if len(linked) > 0 {
				s.syncEvidence(ctx, p, pr.ID, linked)
			}
		} else {
			out.Links += len(pl.reqs)
		}
		out.Tests = append(out.Tests, it)
	}
	status := http.StatusCreated
	if in.DryRun {
		status = http.StatusOK
	}
	writeJSON(w, status, out)
}

// ---------------------------------------------------------------- export

var fileWordRe = regexp.MustCompile(`[^A-Za-z0-9]+`)

// featureFileName: SHOP-TC-12-checkout-with-a-card.feature
func featureFileName(key, title string) string {
	slug := strings.Trim(fileWordRe.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(slug) > 48 {
		slug = strings.TrimRight(slug[:48], "-")
	}
	if slug == "" {
		return key + ".feature"
	}
	return key + "-" + slug + ".feature"
}

// FeatureOf returns the .feature text of a test: its Gherkin (or its
// steps converted, for a test in the step format) tagged with its key and
// the keys of the requirements it verifies.
func FeatureOf(t *model.TestCase, reqKeys []string, lang string) string {
	src := t.Gherkin
	if t.Format != model.FormatGherkin || strings.TrimSpace(src) == "" {
		pt := gherkin.PlainTest{Title: t.Title, Description: t.Description, Preconditions: t.Preconditions, Parameters: t.Parameters, Tags: t.Tags}
		for _, st := range t.Steps {
			ps := gherkin.PlainStep{Action: st.Action, Expected: st.Expected, Data: st.Data}
			if st.CallTestID != nil {
				ps.Call = strings.TrimSpace(st.CallTestKey + " " + st.CallTestTitle)
			}
			pt.Steps = append(pt.Steps, ps)
		}
		for _, r := range t.DataRows {
			pt.Rows = append(pt.Rows, gherkin.PlainRow{Name: r.Name, Values: r.Values})
		}
		src = gherkin.FromPlain(pt, lang)
	}
	tags := append([]string{t.Key}, reqKeys...)
	if tagged, err := gherkin.AddTags(src, tags); err == nil {
		src = tagged
	}
	return strings.TrimRight(src, "\n") + "\n"
}

// handleExportFeatures: GET /projects/{pid}/tests/features?query=&ids= or
// POST …/features/export {"query", "ids"} — a zip with a .feature file
// per selected test.
func (s *Server) handleExportFeatures(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var in struct {
		Query    string   `json:"query"`
		IDs      []string `json:"ids"`
		Language string   `json:"language"`
	}
	if r.Method == http.MethodPost {
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
	} else {
		qv := r.URL.Query()
		in.Query, in.IDs, in.Language = qv.Get("query"), qv["ids"], qv.Get("language")
		if in.Query == "" {
			in.Query = qv.Get("q")
		}
	}
	pc, err := s.loadContext(r.Context(), p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	tests, err := s.filterTests(r, p, pr, pc, in.Query, "", true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	want := map[string]bool{}
	for _, v := range in.IDs {
		for _, id := range strings.Split(v, ",") {
			if id = strings.TrimSpace(id); id != "" {
				want[id] = true
			}
		}
	}
	if len(want) > 0 {
		var sel []model.TestCase
		for _, t := range tests {
			if want[t.ID] || want[t.Key] {
				sel = append(sel, t)
			}
		}
		tests = sel
	}
	if len(tests) == 0 {
		s.fail(w, r, badRequest("no tests selected"))
		return
	}
	if len(tests) > MaxFeatureTests {
		s.fail(w, r, badRequest("%d tests selected; at most %d per export", len(tests), MaxFeatureTests))
		return
	}
	lang := in.Language
	if lang == "" {
		lang = "en"
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := range tests {
		t := &tests[i]
		fh := &zip.FileHeader{Name: featureFileName(t.Key, t.Title), Method: zip.Deflate}
		fh.SetMode(0o644)
		fw, err := zw.CreateHeader(fh)
		if err == nil {
			_, err = fw.Write([]byte(FeatureOf(t, pc.t2rKeys[t.ID], lang)))
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if err := zw.Close(); err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDisposition(strings.ToLower(pr.Key)+"-features.zip"))
	_, _ = w.Write(buf.Bytes())
}

// ---------------------------------------------------------------- results

// handleGherkinResults: GET /tests/{id}/gherkin-results — the latest
// result of each step line of a Gherkin test.
func (s *Server) handleGherkinResults(w http.ResponseWriter, r *http.Request, p *Principal) {
	t, err := s.Store.GetTest(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	exec, steps, err := s.Store.LastGherkinResults(r.Context(), p.TenantID(), t.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"execution_id": exec, "steps": steps})
}
