package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/rom/qtestframe/internal/catalog"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// The requirement and test catalogue (internal/catalog): browsing and
// copying standard items into a project (see docs/api.md "Catalogue").

// maxCatalogImport bounds the items (requirements and tests together) of
// one import request.
const maxCatalogImport = 500

func (s *Server) routesCatalog(h routeFunc) {
	h("GET /api/v1/catalog", s.handleCatalogOverview, "project.read")
	h("GET /api/v1/catalog/requirements", s.handleCatalogRequirements, "requirement.read")
	h("GET /api/v1/catalog/requirements/{id}", s.handleCatalogRequirement, "requirement.read")
	h("GET /api/v1/catalog/tests", s.handleCatalogTests, "test.read")
	h("GET /api/v1/catalog/tests/{id}", s.handleCatalogTest, "test.read")
	h("GET /api/v1/projects/{pid}/catalog", s.handleProjectCatalog, "project.read")
	// write of each kind imported: see handleCatalogImport
	h("POST /api/v1/projects/{pid}/catalog/import", s.atomic(s.handleCatalogImport), "project.read")
}

// catalogue returns the catalogue served (Server.Catalog, else the built-in one).
func (s *Server) catalogue() (*catalog.Catalog, error) {
	if s.Catalog != nil {
		return s.Catalog, nil
	}
	return catalog.Default()
}

func catalogLang(r *http.Request) (string, error) {
	l := strings.TrimSpace(r.URL.Query().Get("lang"))
	if l == "" {
		return "en", nil
	}
	for _, x := range catalog.Languages {
		if l == x {
			return l, nil
		}
	}
	return "", badRequest("lang must be one of %s", strings.Join(catalog.Languages, ", "))
}

func (s *Server) handleCatalogOverview(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, err := s.catalogue()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	lang, err := catalogLang(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"languages": catalog.Languages, "lang": lang, "groups": c.Overview(lang),
		"requirements": len(c.Requirements), "tests": len(c.Tests),
		"requirement_tags": c.Tags(model.KindRequirement), "test_tags": c.Tags(model.KindTest),
	})
}

func catalogFilter(r *http.Request) catalog.Filter {
	q := r.URL.Query()
	return catalog.Filter{Area: q.Get("area"), Group: q.Get("group"), Type: q.Get("type"), Obligation: q.Get("obligation"),
		Priority: q.Get("priority"), Tag: q.Get("tag"), Execution: q.Get("execution"), Q: q.Get("q")}
}

// catalogProject returns the items of ?project_id= by catalogue id (nil
// without the parameter).
func (s *Server) catalogProject(r *http.Request, p *Principal, kind string) (map[string][]store.CatalogItem, error) {
	id := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if id == "" {
		return nil, nil
	}
	pr, err := s.Store.GetProject(r.Context(), p.TenantID(), id)
	if err != nil {
		return nil, err
	}
	return s.Store.CatalogItems(r.Context(), p.TenantID(), pr.ID, kind)
}

type catalogReqItem struct {
	catalog.ReqSummary
	InProject []store.CatalogItem `json:"in_project,omitempty"`
}

type catalogTestItem struct {
	catalog.TestSummary
	InProject []store.CatalogItem `json:"in_project,omitempty"`
}

// notInProject reads ?not_in_project=true.
func notInProject(r *http.Request) bool { return boolParam(r, "not_in_project", false) }

func (s *Server) handleCatalogRequirements(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, lang, ok := s.catalogRead(w, r)
	if !ok {
		return
	}
	inProject, err := s.catalogProject(r, p, model.KindRequirement)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	hide := notInProject(r) && inProject != nil
	out := []catalogReqItem{}
	for _, x := range c.FindRequirements(catalogFilter(r), lang) {
		if hide && len(inProject[x.ID]) > 0 {
			continue
		}
		out = append(out, catalogReqItem{ReqSummary: x, InProject: inProject[x.ID]})
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

func (s *Server) handleCatalogTests(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, lang, ok := s.catalogRead(w, r)
	if !ok {
		return
	}
	inProject, err := s.catalogProject(r, p, model.KindTest)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	hide := notInProject(r) && inProject != nil
	out := []catalogTestItem{}
	for _, x := range c.FindTests(catalogFilter(r), lang) {
		if hide && len(inProject[x.ID]) > 0 {
			continue
		}
		out = append(out, catalogTestItem{TestSummary: x, InProject: inProject[x.ID]})
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

func (s *Server) catalogRead(w http.ResponseWriter, r *http.Request) (*catalog.Catalog, string, bool) {
	c, err := s.catalogue()
	if err == nil {
		var lang string
		if lang, err = catalogLang(r); err == nil {
			return c, lang, true
		}
	}
	s.fail(w, r, err)
	return nil, "", false
}

// catalogLinked is an item at the other end of a catalogue link.
type catalogLinked struct {
	ID        string              `json:"id"`
	Area      string              `json:"area"`
	Title     string              `json:"title"`
	InProject []store.CatalogItem `json:"in_project,omitempty"`
}

type catalogReqDetail struct {
	catalog.ReqSummary
	AreaName  string              `json:"area_name"`
	Rationale string              `json:"rationale"`
	Criteria  []catalog.Criterion `json:"criteria"`
	// LinkedTests are the catalogue tests that verify it.
	LinkedTests []catalogLinked     `json:"linked_tests"`
	InProject   []store.CatalogItem `json:"in_project,omitempty"`
}

type catalogTestDetail struct {
	catalog.TestSummary
	AreaName      string         `json:"area_name"`
	Preconditions string         `json:"preconditions"`
	Expected      string         `json:"expected"`
	StepList      []catalog.Step `json:"step_list"`
	// LinkedRequirements are the catalogue requirements it verifies.
	LinkedRequirements []catalogLinked     `json:"linked_requirements"`
	InProject          []store.CatalogItem `json:"in_project,omitempty"`
}

func areaName(c *catalog.Catalog, id, lang string) string {
	if a := c.Area(id); a != nil {
		return a.Name.Get(lang)
	}
	return id
}

func (s *Server) handleCatalogRequirement(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, lang, ok := s.catalogRead(w, r)
	if !ok {
		return
	}
	rq := c.Requirements[r.PathValue("id")]
	if rq == nil {
		s.fail(w, r, store.ErrNotFound)
		return
	}
	reqs, err := s.catalogProject(r, p, model.KindRequirement)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var tests map[string][]store.CatalogItem
	if reqs != nil && p.Can(rbac.TestRead) {
		if tests, err = s.catalogProject(r, p, model.KindTest); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	x := rq.Localized(lang)
	out := catalogReqDetail{ReqSummary: c.SummarizeRequirement(rq, lang), AreaName: areaName(c, rq.Area, lang), Rationale: x.Rationale,
		Criteria: x.Criteria, LinkedTests: []catalogLinked{}, InProject: reqs[rq.ID]}
	for _, id := range rq.Tests {
		if t := c.Tests[id]; t != nil {
			out.LinkedTests = append(out.LinkedTests, catalogLinked{ID: id, Area: t.Area, Title: t.Localized(lang).Title, InProject: tests[id]})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCatalogTest(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, lang, ok := s.catalogRead(w, r)
	if !ok {
		return
	}
	t := c.Tests[r.PathValue("id")]
	if t == nil {
		s.fail(w, r, store.ErrNotFound)
		return
	}
	tests, err := s.catalogProject(r, p, model.KindTest)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var reqs map[string][]store.CatalogItem
	if tests != nil && p.Can(rbac.RequirementRd) {
		if reqs, err = s.catalogProject(r, p, model.KindRequirement); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	x := t.Localized(lang)
	out := catalogTestDetail{TestSummary: c.SummarizeTest(t, lang), AreaName: areaName(c, t.Area, lang), Preconditions: x.Preconditions,
		Expected: x.Expected, StepList: x.Steps, LinkedRequirements: []catalogLinked{}, InProject: tests[t.ID]}
	for _, id := range t.Requirements {
		if rq := c.Requirements[id]; rq != nil {
			out.LinkedRequirements = append(out.LinkedRequirements, catalogLinked{ID: id, Area: rq.Area, Title: rq.Localized(lang).Title, InProject: reqs[id]})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleProjectCatalog: which catalogue items a project already has, by
// catalogue id (only the kinds the caller may read).
func (s *Server) handleProjectCatalog(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	out := map[string]map[string][]store.CatalogItem{"requirements": {}, "tests": {}}
	for kind, name := range map[string]string{model.KindRequirement: "requirements", model.KindTest: "tests"} {
		perm := rbac.RequirementRd
		if kind == model.KindTest {
			perm = rbac.TestRead
		}
		if !p.Can(perm) {
			continue
		}
		m, err := s.Store.CatalogItems(r.Context(), p.TenantID(), pr.ID, kind)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		out[name] = m
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------- import

type catalogImportRequest struct {
	RequirementIDs []string `json:"requirement_ids"`
	TestIDs        []string `json:"test_ids"`
	Lang           string   `json:"lang"`
	// RequirementFolderID and TestFolderID are the target folders ("" = top level).
	RequirementFolderID string `json:"requirement_folder_id"`
	TestFolderID        string `json:"test_folder_id"`
	// Folders: "none" (default), "area" (a folder per area) or "group"
	// (group folder / area folder) below the target folders.
	Folders string `json:"folders"`
	// Status of the requirements (draft by default); TestStatus of the tests.
	Status     string `json:"status"`
	TestStatus string `json:"test_status"`
	// IncludeTests adds the catalogue tests that verify the chosen
	// requirements (default true when the caller may write tests).
	IncludeTests *bool `json:"include_tests"`
	// AddCopies imports items the project already has (by catalog_ref) again.
	AddCopies bool `json:"add_copies"`
	// Source of the requirements; "" = the cited standards (refs).
	Source *string `json:"source"`
}

type catalogCreated struct {
	ID         string  `json:"id"`
	Key        string  `json:"key"`
	Title      string  `json:"title"`
	CatalogRef string  `json:"catalog_ref"`
	FolderID   *string `json:"folder_id"`
}

type catalogLink struct {
	ID             string `json:"id"`
	RequirementID  string `json:"requirement_id"`
	RequirementKey string `json:"requirement_key"`
	TestCaseID     string `json:"test_case_id"`
	TestCaseKey    string `json:"test_case_key"`
}

type catalogSkipped struct {
	Kind       string              `json:"kind"`
	CatalogRef string              `json:"catalog_ref"`
	Reason     string              `json:"reason"` // exists | not_found | duplicate
	Existing   []store.CatalogItem `json:"existing,omitempty"`
}

type catalogImportResponse struct {
	Requirements   []catalogCreated `json:"requirements"`
	Tests          []catalogCreated `json:"tests"`
	Links          []catalogLink    `json:"links"`
	Skipped        []catalogSkipped `json:"skipped"`
	FoldersCreated int              `json:"folders_created"`
}

// folderResolver finds or creates folders by name path below a root.
type folderResolver struct {
	s        *Server
	ctx      context.Context
	p        *Principal
	project  string
	kind     string
	children map[string]string // parent id + "\x00" + lower(name) → id
	created  int
}

func (s *Server) newFolderResolver(ctx context.Context, p *Principal, projectID, kind string) (*folderResolver, error) {
	fs, err := s.Store.ListFolders(ctx, p.TenantID(), projectID, kind)
	if err != nil {
		return nil, err
	}
	fr := &folderResolver{s: s, ctx: ctx, p: p, project: projectID, kind: kind, children: map[string]string{}}
	for _, f := range fs {
		k := ptrStr(f.ParentID) + "\x00" + strings.ToLower(f.Name)
		if _, dup := fr.children[k]; !dup {
			fr.children[k] = f.ID
		}
	}
	return fr, nil
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (fr *folderResolver) resolve(root *string, path []string) (*string, error) {
	parent := root
	for _, name := range path {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		k := ptrStr(parent) + "\x00" + strings.ToLower(name)
		if id, ok := fr.children[k]; ok {
			parent = &id
			continue
		}
		f := &model.Folder{ProjectID: fr.project, Kind: fr.kind, Name: name, ParentID: parent}
		if err := fr.s.Store.CreateFolder(fr.ctx, fr.p.TenantID(), f); err != nil {
			return nil, err
		}
		fr.created++
		fr.children[k] = f.ID
		id := f.ID
		parent = &id
	}
	return parent, nil
}

// targetFolder checks a target folder of kind in the project ("" = nil).
func (s *Server) targetFolder(ctx context.Context, p *Principal, projectID, kind, id, field string) (*string, error) {
	if strings.TrimSpace(id) == "" {
		return nil, nil
	}
	f, err := s.Store.GetFolder(ctx, p.TenantID(), id)
	if err != nil || f.ProjectID != projectID || f.Kind != kind {
		return nil, badRequest("%s: %s folder not found in project", field, kind)
	}
	return &f.ID, nil
}

// folderPath returns the folder names below the target folder for an
// item of area a.
func folderPathFor(c *catalog.Catalog, mode, area, lang string) []string {
	a := c.Area(area)
	if a == nil || mode == "" || mode == "none" {
		return nil
	}
	if mode == "group" {
		if g := c.Group(a.Group); g != nil {
			return []string{g.Name.Get(lang), a.Name.Get(lang)}
		}
	}
	return []string{a.Name.Get(lang)}
}

// catalogSource is the default source of an imported requirement: the
// standards it cites, else "Catalogue <id>".
func catalogSource(rq *catalog.Requirement, lang string) string {
	if len(rq.Refs) > 0 {
		return strings.Join(rq.Refs, "; ")
	}
	if lang == "sv" {
		return "Katalog " + rq.ID
	}
	return "Catalogue " + rq.ID
}

// catalogRequirement builds the project requirement for a catalogue one.
func catalogRequirement(rq *catalog.Requirement, lang, status, source string) *model.Requirement {
	x := rq.Localized(lang)
	out := &model.Requirement{Title: x.Title, Description: x.Description, Obligation: rq.Obligation, Type: rq.Type,
		Priority: rq.Priority, Status: status, Source: source, Rationale: x.Rationale, Tags: append([]string{}, rq.Tags...),
		CatalogRef: rq.ID}
	for _, cr := range x.Criteria {
		out.Criteria = append(out.Criteria, model.EvaluationCriterion{Description: cr.Text, Method: cr.Method, Threshold: cr.Threshold})
	}
	return out
}

// catalogTest builds the project test for a catalogue one.
func catalogTest(t *catalog.Test, lang, status string) *model.TestCase {
	x := t.Localized(lang)
	out := &model.TestCase{Title: x.Title, Description: x.Description, Preconditions: x.Preconditions, ExpectedResult: x.Expected,
		ExecutionType: t.Execution, Priority: t.Priority, Status: status, Tags: append([]string{}, t.Tags...), CatalogRef: t.ID,
		Steps: []model.TestStep{}}
	for _, st := range x.Steps {
		out.Steps = append(out.Steps, model.TestStep{Action: st.Action, Expected: st.Expected, Data: st.Data})
	}
	return out
}

// handleCatalogImport: POST /projects/{pid}/catalog/import copies
// catalogue requirements and tests into the project, with trace links
// between them and to matching items already there. Atomic (s.atomic).
func (s *Server) handleCatalogImport(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	c, err := s.catalogue()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req catalogImportRequest
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	lang := req.Lang
	if lang == "" {
		lang = "en"
	}
	if catalog.Lang(lang) != lang {
		s.fail(w, r, badRequest("lang must be one of %s", strings.Join(catalog.Languages, ", ")))
		return
	}
	switch req.Folders {
	case "", "none", "area", "group":
	default:
		s.fail(w, r, badRequest("folders must be none, area or group"))
		return
	}
	if req.Status == "" {
		req.Status = "draft"
	}
	if req.TestStatus == "" {
		req.TestStatus = "draft"
	}
	if !model.Valid(model.RequirementStatuses, req.Status) {
		s.fail(w, r, badRequest("invalid status %q", req.Status))
		return
	}
	if !model.Valid(model.TestStatuses, req.TestStatus) {
		s.fail(w, r, badRequest("invalid test_status %q", req.TestStatus))
		return
	}
	reqIDs, testIDs := uniqStrings(req.RequirementIDs), uniqStrings(req.TestIDs)
	if len(reqIDs)+len(testIDs) == 0 {
		s.fail(w, r, badRequest("choose at least one requirement or test"))
		return
	}
	include := p.Can(rbac.TestWrite)
	if req.IncludeTests != nil {
		include = *req.IncludeTests
	}
	if len(reqIDs) > 0 && !p.Can(rbac.RequirementWr) {
		writeError(w, http.StatusForbidden, "forbidden", "permission requirement.write required to add catalogue requirements")
		return
	}
	if (len(testIDs) > 0 || (include && req.IncludeTests != nil && len(reqIDs) > 0)) && !p.Can(rbac.TestWrite) {
		writeError(w, http.StatusForbidden, "forbidden", "permission test.write required to add catalogue tests")
		return
	}
	if len(reqIDs) > 0 && isReviewDecision(model.KindRequirement, "", req.Status) && !p.Can(rbac.RequirementRev) {
		writeError(w, http.StatusForbidden, "forbidden", "permission requirement.review required to add "+req.Status+" requirements")
		return
	}
	canTests := p.Can(rbac.TestWrite)

	out := catalogImportResponse{Requirements: []catalogCreated{}, Tests: []catalogCreated{}, Links: []catalogLink{}, Skipped: []catalogSkipped{}}
	// the requirements and tests to import, in catalogue order
	var reqs []*catalog.Requirement
	for _, id := range reqIDs {
		if rq := c.Requirements[id]; rq != nil {
			reqs = append(reqs, rq)
		} else {
			out.Skipped = append(out.Skipped, catalogSkipped{Kind: model.KindRequirement, CatalogRef: id, Reason: "not_found"})
		}
	}
	var tests []*catalog.Test
	seenTest := map[string]bool{}
	for _, id := range testIDs {
		if t := c.Tests[id]; t != nil {
			tests = append(tests, t)
			seenTest[id] = true
		} else {
			out.Skipped = append(out.Skipped, catalogSkipped{Kind: model.KindTest, CatalogRef: id, Reason: "not_found"})
		}
	}
	if include && canTests {
		for _, rq := range reqs {
			for _, id := range rq.Tests {
				if t := c.Tests[id]; t != nil && !seenTest[id] {
					tests = append(tests, t)
					seenTest[id] = true
				}
			}
		}
	}
	if len(tests) > 0 && isReviewDecision(model.KindTest, "", req.TestStatus) && !p.Can(rbac.TestReview) {
		writeError(w, http.StatusForbidden, "forbidden", "permission test.review required to add "+req.TestStatus+" tests")
		return
	}
	if len(reqs)+len(tests) > maxCatalogImport {
		s.fail(w, r, badRequest("at most %d items per import (%d requirements and %d tests chosen)", maxCatalogImport, len(reqs), len(tests)))
		return
	}

	existingReqs, err := s.Store.CatalogItems(ctx, p.TenantID(), pr.ID, model.KindRequirement)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	existingTests, err := s.Store.CatalogItems(ctx, p.TenantID(), pr.ID, model.KindTest)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reqRoot, err := s.targetFolder(ctx, p, pr.ID, model.KindRequirement, req.RequirementFolderID, "requirement_folder_id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	testRoot, err := s.targetFolder(ctx, p, pr.ID, model.KindTest, req.TestFolderID, "test_folder_id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reqFolders, err := s.newFolderResolver(ctx, p, pr.ID, model.KindRequirement)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	testFolders, err := s.newFolderResolver(ctx, p, pr.ID, model.KindTest)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	// catalogue id → project items now created for it
	newReq, newTest := map[string][]*model.Requirement{}, map[string][]*model.TestCase{}
	for _, rq := range reqs {
		if ex := existingReqs[rq.ID]; len(ex) > 0 && !req.AddCopies {
			out.Skipped = append(out.Skipped, catalogSkipped{Kind: model.KindRequirement, CatalogRef: rq.ID, Reason: "exists", Existing: ex})
			continue
		}
		source := catalogSource(rq, lang)
		if req.Source != nil {
			source = strings.TrimSpace(*req.Source)
		}
		m := catalogRequirement(rq, lang, req.Status, source)
		m.ProjectID, m.CreatedBy = pr.ID, p.User.Email
		if m.FolderID, err = reqFolders.resolve(reqRoot, folderPathFor(c, req.Folders, rq.Area, lang)); err != nil {
			s.fail(w, r, err)
			return
		}
		if err := s.Store.CreateRequirement(ctx, p.TenantID(), m); err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", rq.ID, err))
			return
		}
		newReq[rq.ID] = append(newReq[rq.ID], m)
		out.Requirements = append(out.Requirements, catalogCreated{ID: m.ID, Key: m.Key, Title: m.Title, CatalogRef: rq.ID, FolderID: m.FolderID})
	}
	for _, t := range tests {
		if ex := existingTests[t.ID]; len(ex) > 0 && !req.AddCopies {
			out.Skipped = append(out.Skipped, catalogSkipped{Kind: model.KindTest, CatalogRef: t.ID, Reason: "exists", Existing: ex})
			continue
		}
		m := catalogTest(t, lang, req.TestStatus)
		m.ProjectID, m.CreatedBy = pr.ID, p.User.Email
		if m.FolderID, err = testFolders.resolve(testRoot, folderPathFor(c, req.Folders, t.Area, lang)); err != nil {
			s.fail(w, r, err)
			return
		}
		if err := s.Store.CreateTest(ctx, p.TenantID(), m); err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", t.ID, err))
			return
		}
		newTest[t.ID] = append(newTest[t.ID], m)
		out.Tests = append(out.Tests, catalogCreated{ID: m.ID, Key: m.Key, Title: m.Title, CatalogRef: t.ID, FolderID: m.FolderID})
	}
	out.FoldersCreated = reqFolders.created + testFolders.created

	// trace links: every new requirement to the tests of its catalogue
	// tests (new or already in the project), and every new test to the
	// project requirements of the catalogue requirements it verifies.
	type ref struct{ id, key string }
	reqsOf := func(cid string) []ref {
		var rs []ref
		for _, m := range newReq[cid] {
			rs = append(rs, ref{m.ID, m.Key})
		}
		for _, ex := range existingReqs[cid] {
			rs = append(rs, ref{ex.ID, ex.Key})
		}
		return rs
	}
	testsOf := func(cid string) []ref {
		var ts []ref
		for _, m := range newTest[cid] {
			ts = append(ts, ref{m.ID, m.Key})
		}
		for _, ex := range existingTests[cid] {
			ts = append(ts, ref{ex.ID, ex.Key})
		}
		return ts
	}
	linked := map[[2]string]bool{}
	var linkedReqs []string
	link := func(rq, t ref) error {
		k := [2]string{rq.id, t.id}
		if linked[k] {
			return nil
		}
		linked[k] = true
		l := &model.TraceLink{RequirementID: rq.id, TestCaseID: t.id}
		if err := s.Store.CreateLink(ctx, p.TenantID(), l); err != nil {
			return err
		}
		linkedReqs = append(linkedReqs, rq.id)
		out.Links = append(out.Links, catalogLink{ID: l.ID, RequirementID: rq.id, RequirementKey: rq.key, TestCaseID: t.id, TestCaseKey: t.key})
		return nil
	}
	if canTests { // links need test.write
		for _, rq := range reqs {
			for _, m := range newReq[rq.ID] {
				for _, tid := range rq.Tests {
					for _, t := range testsOf(tid) {
						if err := link(ref{m.ID, m.Key}, t); err != nil {
							s.fail(w, r, err)
							return
						}
					}
				}
			}
		}
		for _, t := range tests {
			for _, m := range newTest[t.ID] {
				for _, rid := range t.Requirements {
					for _, rq := range reqsOf(rid) {
						if err := link(rq, ref{m.ID, m.Key}); err != nil {
							s.fail(w, r, err)
							return
						}
					}
				}
			}
		}
	}

	for _, it := range out.Requirements {
		s.recordRequirement(ctx, p, it.ID, "create")
	}
	for _, it := range out.Tests {
		s.recordTest(ctx, p, it.ID, "create")
	}
	if len(linkedReqs) > 0 {
		s.syncEvidence(ctx, p, pr.ID, uniqStrings(linkedReqs))
	}
	refs := make([]string, 0, len(out.Requirements)+len(out.Tests))
	for _, it := range append(append([]catalogCreated{}, out.Requirements...), out.Tests...) {
		refs = append(refs, it.Key+"="+it.CatalogRef)
	}
	s.audit(ctx, p, "catalog.import", "project", pr.ID, fmt.Sprintf("%d requirements, %d tests, %d links, %d skipped (%s): %s",
		len(out.Requirements), len(out.Tests), len(out.Links), len(out.Skipped), lang, truncate(strings.Join(refs, ", "), 2000)))
	writeJSON(w, http.StatusCreated, out)
}

// truncate shortens s to at most n bytes (on a rune boundary).
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && n < len(s) && (s[n]&0xC0) == 0x80 {
		n--
	}
	return s[:n] + "…"
}
