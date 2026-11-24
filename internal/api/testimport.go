package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/rom/qtestframe/internal/customfield"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/testimport"
)

// Import of tests from other test management tools (see internal/
// testimport and docs/api.md "Import tests from other tools"): a file of
// TestRail, Xray, Zephyr, qTest or Azure Test Plans is previewed (what
// would be created, updated, left unchanged or is in conflict, the
// priority, status and custom field mapping, the requirement references
// that match no requirement) and applied in one transaction. Each test
// remembers its source and external ID (external_refs, entity type test),
// so importing the file again updates the tests instead of duplicating
// them; past results become completed runs, also only once.

func (s *Server) routesTestImport(h routeFunc) {
	h("POST /api/v1/projects/{pid}/tests/import/preview", s.handleTestImportPreview, "test.write")
	h("POST /api/v1/projects/{pid}/tests/import/apply", s.impactAware(s.handleTestImportApply, true), "test.write")
}

// testImportSystem is the system of the external references of a source.
func testImportSystem(source string) string {
	if source == testimport.SourceAzure {
		return "azure-test-plans"
	}
	return source
}

// Row actions of the preview.
const (
	tiNew       = "new"
	tiUpdate    = "update"
	tiUnchanged = "unchanged"
	tiConflict  = "conflict"
	tiError     = "error"
)

type testImportRow struct {
	Index      int      `json:"index"`
	Location   string   `json:"location"`
	ExternalID string   `json:"external_id"`
	Title      string   `json:"title"`
	Folder     string   `json:"folder"`
	Type       string   `json:"type"`
	Priority   string   `json:"priority"`
	Status     string   `json:"status"`
	Steps      int      `json:"steps"`
	Action     string   `json:"action"`
	ID         string   `json:"id,omitempty"`
	Key        string   `json:"key,omitempty"`
	Links      []string `json:"links"`
	Unresolved []string `json:"unresolved"`
	Executions int      `json:"executions"`
	Warnings   []string `json:"warnings"`
	Errors     []string `json:"errors"`

	body    map[string]any
	folder  []string
	hash    string
	version int
	reqIDs  []string
	c       *testimport.Case
}

type testImportSummary struct {
	New       int `json:"new"`
	Update    int `json:"update"`
	Unchanged int `json:"unchanged"`
	Conflict  int `json:"conflict"`
	Error     int `json:"error"`
}

type testImportHistory struct {
	Runs       int `json:"runs"`
	Executions int `json:"executions"`
	// Known are the results imported before (left alone).
	Known int `json:"known"`
}

type testImportTarget struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

type testImportPreview struct {
	Source       string             `json:"source"`
	SourceName   string             `json:"source_name"`
	Format       string             `json:"format"`
	File         string             `json:"file"`
	TargetFolder string             `json:"target_folder"`
	Total        int                `json:"total"`
	Summary      testImportSummary  `json:"summary"`
	Rows         []testImportRow    `json:"rows"`
	Priorities   []testimport.Value `json:"priorities"`
	Statuses     []testimport.Value `json:"statuses"`
	Fields       []testimport.Field `json:"fields"`
	Targets      []testImportTarget `json:"custom_fields"`
	PriorityOpts []string           `json:"priority_options"`
	StatusOpts   []string           `json:"status_options"`
	History      testImportHistory  `json:"history"`
	Unresolved   []string           `json:"unresolved_links"`
	Previous     int                `json:"previous"`
	Warnings     []string           `json:"warnings"`
	Errors       []string           `json:"errors"`
	refs         map[string]store.ExternalRef
	tests        map[string]*model.TestCase
	root         *string
	system       string
	hist         map[string][]tiExec // run id → new results
	runRefs      map[string]store.ExternalRef
	runNames     map[string]string
}

type tiExec struct {
	row  int // index of the row
	exec testimport.Execution
	ref  string
}

// planTestImport reads the upload and works out what an import does.
func (s *Server) planTestImport(w http.ResponseWriter, r *http.Request, p *Principal, pr *model.Project) (*testImportPreview, error) {
	up, err := readUpload(w, r, "file", maxImportBytes)
	if err != nil {
		return nil, err
	}
	source, ok := testimport.ValidSource(r.FormValue("source"))
	if !ok {
		return nil, badRequest("source must be one of %s", strings.Join(testimport.Sources, ", "))
	}
	ctx := r.Context()
	defs, err := s.Store.ListCustomFields(ctx, p.TenantID(), pr.ID, model.KindTest)
	if err != nil {
		return nil, err
	}
	opt := testimport.Options{Format: r.FormValue("format"), CustomFields: defs, MaxCases: maxImportCandidates}
	for name, dst := range map[string]*map[string]string{"priorities": &opt.Priorities, "statuses": &opt.Statuses, "fields": &opt.Fields} {
		if *dst, err = jsonMap(r, name, "source value → target"); err != nil {
			return nil, err
		}
	}
	res, err := testimport.Parse(source, up.data, up.name, opt)
	if errors.Is(err, testimport.ErrInvalid) {
		return nil, badRequest("%s", strings.TrimPrefix(err.Error(), testimport.ErrInvalid.Error()+": "))
	}
	if err != nil {
		return nil, err
	}
	pv := &testImportPreview{Source: source, SourceName: testimport.SourceNames[source], Format: res.Format, File: up.name, Rows: []testImportRow{},
		Priorities: res.Priorities, Statuses: res.Statuses, Fields: res.Fields, Targets: []testImportTarget{}, PriorityOpts: model.Priorities,
		StatusOpts: model.TestStatuses, Unresolved: []string{}, Warnings: res.Warnings, Errors: res.Errors, system: testImportSystem(source),
		refs: map[string]store.ExternalRef{}, tests: map[string]*model.TestCase{}, hist: map[string][]tiExec{}, runRefs: map[string]store.ExternalRef{}, runNames: map[string]string{}}
	for _, d := range defs {
		pv.Targets = append(pv.Targets, testImportTarget{Key: d.Key, Label: d.Label, Type: d.Type})
	}
	if id := strings.TrimSpace(r.FormValue("folder_id")); id != "" {
		f, err := s.Store.GetFolder(ctx, p.TenantID(), id)
		if err != nil || f.ProjectID != pr.ID || f.Kind != model.KindTest {
			return nil, badRequest("folder_id: test folder not found in project")
		}
		folders, err := s.Store.ListFolders(ctx, p.TenantID(), pr.ID, model.KindTest)
		if err != nil {
			return nil, err
		}
		pv.root, pv.TargetFolder = &f.ID, store.FolderPaths(folders)[f.ID]
	}
	tests, err := s.Store.ListTests(ctx, p.TenantID(), pr.ID)
	if err != nil {
		return nil, err
	}
	for i := range tests {
		pv.tests[tests[i].ID] = &tests[i]
	}
	refs, err := s.Store.ListExternalRefs(ctx, p.TenantID(), pr.ID, pv.system)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, ref := range refs {
		switch ref.EntityType {
		case model.KindTest:
			if pv.tests[ref.EntityID] != nil {
				pv.refs[strings.ToLower(ref.ExternalID)] = ref
			}
		case "run":
			pv.runRefs[ref.ExternalID] = ref
		case "execution":
			known[ref.ExternalID] = true
		}
	}
	reqs, err := s.requirementIndex(r, p, pr)
	if err != nil {
		return nil, err
	}
	targets := map[string]string{} // lower source field → custom field key
	for _, f := range res.Fields {
		targets[strings.ToLower(f.Name)] = f.Target
	}
	canReview := p.Can(rbac.TestReview)
	unresolved := map[string]bool{}
	seen := map[string]int{}
	for i := range res.Cases {
		c := &res.Cases[i]
		row := testImportRow{Index: i, Location: c.Location, ExternalID: c.ExternalID, Title: c.Title, Type: c.Type, Steps: len(c.Steps),
			Links: []string{}, Unresolved: []string{}, Warnings: append([]string{}, c.Warnings...), Errors: []string{}, c: c}
		row.folder = testimport.FolderParts(c.Folder)
		row.Folder = strings.Join(append(testimport.FolderParts(pv.TargetFolder), row.folder...), "/")
		row.body = s.testImportBody(r, p, c, defs, targets, &row)
		row.Priority, _ = row.body["priority"].(string)
		row.Status, _ = row.body["status"].(string)
		if row.Status == "ready" && !canReview {
			row.Warnings = append(row.Warnings, "status ready needs the permission test.review: imported as draft")
			row.body["status"], row.Status = "draft", "draft"
		}
		for _, ref := range c.Refs {
			if id, key, ok := reqs.lookup(ref); ok {
				row.reqIDs = append(row.reqIDs, id)
				row.Links = append(row.Links, key)
			} else {
				row.Unresolved = append(row.Unresolved, ref)
				if !unresolved[strings.ToLower(ref)] {
					unresolved[strings.ToLower(ref)] = true
					pv.Unresolved = append(pv.Unresolved, ref)
				}
			}
		}
		if len(row.Unresolved) > 0 {
			row.Warnings = append(row.Warnings, "no requirement matches "+strings.Join(row.Unresolved, ", "))
		}
		row.hash = testImportHash(row.body, row.folder, c.Refs)
		check := model.TestCase{Title: c.Title}
		if b, err := json.Marshal(row.body); err == nil {
			_ = json.Unmarshal(b, &check)
		}
		switch err := store.ValidateTest(&check); {
		case err != nil:
			row.Errors = append(row.Errors, strings.TrimPrefix(err.Error(), store.ErrInvalid.Error()+": "))
		case c.ExternalID != "" && seen[strings.ToLower(c.ExternalID)] > 0:
			row.Errors = append(row.Errors, fmt.Sprintf("the ID %s is used twice in the file", c.ExternalID))
		}
		if c.ExternalID != "" {
			seen[strings.ToLower(c.ExternalID)]++
		} else {
			row.Warnings = append(row.Warnings, "no ID in the file: importing the file again creates the test again")
		}
		ref, hasRef := pv.refs[strings.ToLower(c.ExternalID)]
		switch {
		case len(row.Errors) > 0:
			row.Action = tiError
		case c.ExternalID != "" && hasRef:
			t := pv.tests[ref.EntityID]
			row.ID, row.Key, row.version = t.ID, t.Key, t.Version
			pv.Previous++
			switch {
			case ref.RemoteHash == row.hash:
				row.Action = tiUnchanged
			case t.Version != ref.Version:
				row.Action = tiConflict
				row.Warnings = append(row.Warnings, fmt.Sprintf("%s was changed here since the last import", t.Key))
			default:
				row.Action = tiUpdate
			}
		default:
			row.Action = tiNew
		}
		for _, x := range c.Executions {
			pv.History.Executions++
			key := x.RunID + "/" + c.ExternalID
			if c.ExternalID != "" && known[key] {
				pv.History.Known++
				continue
			}
			row.Executions++
			pv.runNames[x.RunID] = x.Run
			pv.hist[x.RunID] = append(pv.hist[x.RunID], tiExec{row: i, exec: x, ref: key})
		}
		pv.Rows = append(pv.Rows, row)
	}
	for i := range pv.Rows {
		row := &pv.Rows[i]
		switch row.Action {
		case tiNew:
			pv.Summary.New++
		case tiUpdate:
			pv.Summary.Update++
		case tiUnchanged:
			pv.Summary.Unchanged++
		case tiConflict:
			pv.Summary.Conflict++
		default:
			pv.Summary.Error++
		}
	}
	pv.History.Runs = len(pv.hist)
	pv.Total = len(pv.Rows)
	return pv, nil
}

// testImportBody is the test of a case as a create/PATCH body.
func (s *Server) testImportBody(r *http.Request, p *Principal, c *testimport.Case, defs []model.CustomField, targets map[string]string, row *testImportRow) map[string]any {
	desc := c.Description
	if c.Gherkin != "" {
		desc = strings.TrimSpace(desc + "\n\n```gherkin\n" + c.Gherkin + "\n```")
	}
	if c.Definition != "" {
		desc = strings.TrimSpace(desc + "\n\n" + c.Type + " definition: `" + strings.ReplaceAll(c.Definition, "`", "'") + "`")
	}
	steps := []map[string]any{}
	for _, st := range c.Steps {
		steps = append(steps, map[string]any{"action": st.Action, "expected": st.Expected, "data": st.Data})
	}
	exec := model.ExecManual
	if c.Automated {
		exec = model.ExecAutomated
	}
	body := map[string]any{"title": c.Title, "description": desc, "preconditions": c.Preconditions, "expected_result": c.ExpectedResult,
		"steps": steps, "execution_type": exec, "tags": c.Tags, "estimated_minutes": c.EstimatedMin}
	if c.Priority != "" {
		body["priority"] = c.Priority
	}
	if c.Status != "" {
		body["status"] = c.Status
	}
	if c.Owner != "" {
		body["owner"] = c.Owner
	}
	custom := map[string]any{}
	for i := range defs {
		f := &defs[i]
		for name, text := range c.Custom {
			if targets[strings.ToLower(name)] != f.Key {
				continue
			}
			v, err := customfield.Parse(f, text)
			if err != nil {
				row.Warnings = append(row.Warnings, f.Label+": "+strings.TrimPrefix(err.Error(), customfield.ErrInvalid.Error()+": ")+"; value not imported")
				continue
			}
			if e, isUser := v.(string); isUser && f.Type == model.FieldUser && !s.Store.IsMemberEmail(r.Context(), p.TenantID(), e) {
				row.Warnings = append(row.Warnings, f.Label+": "+e+" is not a member of the organisation; value not imported")
				continue
			}
			if v != nil {
				custom[f.Key] = v
			}
		}
	}
	if len(custom) > 0 {
		body["custom_fields"] = custom
	}
	return body
}

func testImportHash(body map[string]any, folder, refs []string) string {
	b, _ := json.Marshal(map[string]any{"body": body, "folder": folder, "refs": refs})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// reqIndex finds requirements by key, by the external IDs recorded by
// requirement imports (Jira, Azure DevOps, ReqIF) and by "ID: x" in their
// source.
type reqIndex struct {
	byRef map[string][2]string // lower ref → id, key
}

var idInSourceRe = regexp.MustCompile(`(?i)\bID: ([^\s,;)]+)`)

func (s *Server) requirementIndex(r *http.Request, p *Principal, pr *model.Project) (*reqIndex, error) {
	reqs, err := s.Store.ListRequirements(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		return nil, err
	}
	ix := &reqIndex{byRef: map[string][2]string{}}
	keyOf := map[string]string{}
	add := func(ref, id, key string) {
		ref = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ref), "#"))
		if _, dup := ix.byRef[ref]; ref != "" && !dup {
			ix.byRef[ref] = [2]string{id, key}
		}
	}
	for _, rq := range reqs {
		keyOf[rq.ID] = rq.Key
		add(rq.Key, rq.ID, rq.Key)
	}
	refs, err := s.Store.ListExternalRefsOfType(r.Context(), p.TenantID(), pr.ID, model.KindRequirement)
	if err != nil {
		return nil, err
	}
	for _, ref := range refs {
		if k, ok := keyOf[ref.EntityID]; ok {
			add(ref.ExternalID, ref.EntityID, k)
		}
	}
	for _, rq := range reqs {
		for _, m := range idInSourceRe.FindAllStringSubmatch(rq.Source, -1) {
			add(m[1], rq.ID, rq.Key)
		}
	}
	return ix, nil
}

func (ix *reqIndex) lookup(ref string) (string, string, bool) {
	v, ok := ix.byRef[strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ref), "#"))]
	return v[0], v[1], ok
}

func (s *Server) handleTestImportPreview(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	pv, err := s.planTestImport(w, r, p, pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, pv)
}

type testImportItem struct {
	Index  int    `json:"index"`
	ID     string `json:"id"`
	Key    string `json:"key"`
	Title  string `json:"title"`
	Action string `json:"action"` // created | updated
}

type testImportSkipped struct {
	Index    int    `json:"index"`
	Location string `json:"location"`
	Title    string `json:"title"`
	Reason   string `json:"reason"` // excluded | conflict | error | new
}

type testImportResult struct {
	Created        int                 `json:"created"`
	Updated        int                 `json:"updated"`
	Unchanged      int                 `json:"unchanged"`
	Skipped        int                 `json:"skipped"`
	FoldersCreated int                 `json:"folders_created"`
	LinksAdded     int                 `json:"links_added"`
	RunsCreated    int                 `json:"runs_created"`
	Results        int                 `json:"results_imported"`
	Unresolved     []string            `json:"unresolved_links"`
	Items          []testImportItem    `json:"items"`
	SkippedRows    []testImportSkipped `json:"skipped_rows"`
	Warnings       []string            `json:"warnings"`
}

func (s *Server) handleTestImportApply(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	pv, err := s.planTestImport(w, r, p, pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	opt, err := applyOptionsOf(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	createFolders, links, history := formBool(r, "create_folders", true), formBool(r, "link_requirements", true), formBool(r, "history", true)
	res, err := s.applyTestImport(r, p, pr, pv, opt, createFolders, links, history)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "test.import", "project", pr.ID, fmt.Sprintf("%s (%s %s): %d created, %d updated, %d unchanged, %d skipped, %d folders, %d links, %d runs, %d results",
		pv.File, pv.SourceName, pv.Format, res.Created, res.Updated, res.Unchanged, res.Skipped, res.FoldersCreated, res.LinksAdded, res.RunsCreated, res.Results))
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) applyTestImport(r *http.Request, p *Principal, pr *model.Project, pv *testImportPreview, opt applyOptions, createFolders, links, history bool) (*testImportResult, error) {
	ctx := r.Context()
	res := &testImportResult{Items: []testImportItem{}, SkippedRows: []testImportSkipped{}, Unresolved: pv.Unresolved, Warnings: pv.Warnings}
	fr, err := s.newFolderResolver(ctx, p, pr.ID, model.KindTest)
	if err != nil {
		return nil, err
	}
	var touched []string
	done := map[int]string{} // row index → test id (applied or unchanged)
	for i := range pv.Rows {
		row := &pv.Rows[i]
		skip := ""
		switch {
		case opt.exclude[row.Index]:
			skip = "excluded"
		case row.Action == tiError:
			skip = "error"
		case row.Action == tiConflict && !opt.overwrite:
			skip = "conflict"
		case row.Action == tiNew && !opt.create:
			skip = "new"
		}
		if skip != "" {
			res.SkippedRows = append(res.SkippedRows, testImportSkipped{Index: row.Index, Location: row.Location, Title: row.Title, Reason: skip})
			continue
		}
		if row.Action == tiUnchanged {
			res.Unchanged++
			done[row.Index] = row.ID
		} else {
			body := row.body
			folder := pv.root
			if createFolders {
				if folder, err = fr.resolve(pv.root, row.folder); err != nil {
					return nil, err
				}
			}
			body["folder_id"] = folder
			var rec *heldResponse
			action := "updated"
			if row.Action == tiNew {
				action = "created"
				rec = s.subCall(r, p, s.handleCreateTest, http.MethodPost, map[string]string{"pid": pr.ID}, body, 0)
			} else {
				// the source's owner is only taken over by new tests
				delete(body, "owner")
				rec = s.subCall(r, p, s.handleUpdateTest, http.MethodPatch, map[string]string{"id": row.ID}, body, row.version)
			}
			if rec.status() >= 400 {
				var e apiError
				_ = json.Unmarshal(rec.body.Bytes(), &e)
				if e.Code == "" {
					e.Code = "error"
				}
				return nil, &apiErr{status: rec.status(), code: e.Code, msg: fmt.Sprintf("%s (%s): %s", row.Location, firstNonBlank(row.ExternalID, row.Title), e.Error)}
			}
			var out struct {
				ID      string `json:"id"`
				Key     string `json:"key"`
				Version int    `json:"version"`
			}
			_ = json.Unmarshal(rec.body.Bytes(), &out)
			if action == "created" {
				res.Created++
			} else {
				res.Updated++
			}
			res.Items = append(res.Items, testImportItem{Index: row.Index, ID: out.ID, Key: out.Key, Title: row.Title, Action: action})
			done[row.Index] = out.ID
			if row.ExternalID != "" {
				ref := &store.ExternalRef{ProjectID: pr.ID, System: pv.system, EntityType: model.KindTest, EntityID: out.ID, ExternalID: row.ExternalID,
					Source: pv.SourceName + ": " + pv.File, Version: out.Version}
				if err := s.Store.SaveExternalRef(ctx, p.TenantID(), ref); err != nil {
					return nil, err
				}
				ref.RemoteHash = row.hash
				if err := s.Store.UpdateSyncRef(ctx, p.TenantID(), ref); err != nil {
					return nil, err
				}
			}
		}
		if links {
			added, err := s.addTestLinks(r, p, pr, done[row.Index], row.reqIDs)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", row.Location, err)
			}
			res.LinksAdded += len(added)
			touched = append(touched, added...)
		}
	}
	if len(touched) > 0 {
		s.syncEvidence(ctx, p, pr.ID, touched)
	}
	if history && len(pv.hist) > 0 {
		if !p.Can(rbac.RunWrite) {
			res.Warnings = append(res.Warnings, "past results were not imported: the permission run.write is required")
		} else if err := s.importTestHistory(r, p, pr, pv, done, res); err != nil {
			return nil, err
		}
	}
	res.Skipped = len(res.SkippedRows)
	res.FoldersCreated = fr.created
	return res, nil
}

// addTestLinks links a test to the requirements it does not verify yet;
// it returns the requirements linked.
func (s *Server) addTestLinks(r *http.Request, p *Principal, pr *model.Project, testID string, reqIDs []string) ([]string, error) {
	if testID == "" || len(reqIDs) == 0 {
		return nil, nil
	}
	ctx := r.Context()
	have, err := s.Store.ListLinks(ctx, p.TenantID(), pr.ID, "", testID)
	if err != nil {
		return nil, err
	}
	linked := map[string]bool{}
	for _, l := range have {
		linked[l.RequirementID] = true
	}
	var added []string
	for _, id := range reqIDs {
		if linked[id] {
			continue
		}
		linked[id] = true
		l := &model.TraceLink{RequirementID: id, TestCaseID: testID}
		if err := s.Store.CreateLink(ctx, p.TenantID(), l); err != nil {
			return nil, err
		}
		s.audit(ctx, p, "link.create", "link", l.ID, l.RequirementKey+" -> "+l.TestCaseKey+" (test import)")
		added = append(added, id)
	}
	return added, nil
}

// importTestHistory records the past results of the imported tests: one
// completed run per run of the source (reused when imported before).
func (s *Server) importTestHistory(r *http.Request, p *Principal, pr *model.Project, pv *testImportPreview, done map[int]string, res *testImportResult) error {
	ctx := r.Context()
	runIDs := make([]string, 0, len(pv.hist))
	for id := range pv.hist {
		runIDs = append(runIDs, id)
	}
	sort.Strings(runIDs)
	for _, rid := range runIDs {
		var results []store.HistoryResult
		var refs []string
		for _, x := range pv.hist[rid] {
			if done[x.row] == "" {
				continue
			}
			results = append(results, store.HistoryResult{TestID: done[x.row], Status: x.exec.Status, ExecutedBy: x.exec.ExecutedBy, Notes: x.exec.Comment, At: x.exec.ExecutedAt})
			refs = append(refs, x.ref)
		}
		if len(results) == 0 {
			continue
		}
		runID := ""
		if ref, ok := pv.runRefs[rid]; ok {
			if _, err := s.Store.GetRun(ctx, p.TenantID(), ref.EntityID); err == nil {
				runID = ref.EntityID
			}
		}
		if runID == "" {
			run := &model.TestRun{ProjectID: pr.ID, Name: pv.SourceName + ": " + firstNonBlank(pv.runNames[rid], rid),
				Description: fmt.Sprintf("Past results imported from %s (%s).", pv.SourceName, pv.File), Status: "planned", CreatedBy: p.User.Email}
			if err := s.Store.CreateRun(ctx, p.TenantID(), run, nil); err != nil {
				return err
			}
			s.audit(ctx, p, "run.create", "run", run.ID, run.Name+" (test import)")
			if err := s.Store.SaveExternalRef(ctx, p.TenantID(), &store.ExternalRef{ProjectID: pr.ID, System: pv.system, EntityType: "run", EntityID: run.ID,
				ExternalID: rid, Source: pv.SourceName + ": " + pv.File}); err != nil {
				return err
			}
			runID = run.ID
			res.RunsCreated++
		}
		ids, err := s.Store.ImportHistory(ctx, p.TenantID(), runID, results)
		if err != nil {
			return err
		}
		for i, eid := range ids {
			if err := s.Store.SaveExternalRef(ctx, p.TenantID(), &store.ExternalRef{ProjectID: pr.ID, System: pv.system, EntityType: "execution", EntityID: eid,
				ExternalID: refs[i], Source: pv.SourceName + ": " + pv.File}); err != nil {
				return err
			}
		}
		res.Results += len(ids)
	}
	return nil
}
