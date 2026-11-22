package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/reqimport"
	"github.com/rom/qtestframe/internal/store"
)

// Requirement import from customer files (see docs/api.md "Import
// requirements").

const (
	maxImportBytes      = 20 << 20
	maxImportCandidates = 5000
)

func (s *Server) routesReqImport(h routeFunc) {
	h("POST /api/v1/projects/{pid}/requirements/import/preview", s.handleImportPreview, "requirement.write")
	h("POST /api/v1/projects/{pid}/requirements/import", s.handleImportRequirements, "requirement.write")
}

// formBool reads a boolean multipart form value.
func formBool(r *http.Request, name string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(r.FormValue(name))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

// parseImportUpload reads the upload and parses it with the form options.
func (s *Server) parseImportUpload(w http.ResponseWriter, r *http.Request, p *Principal, pr *model.Project) (*reqimport.Result, *upload, error) {
	up, err := readUpload(w, r, "file", maxImportBytes)
	if err != nil {
		return nil, nil, err
	}
	defs, err := s.Store.ListCustomFields(r.Context(), p.TenantID(), pr.ID, model.KindRequirement)
	if err != nil {
		return nil, nil, err
	}
	opt := reqimport.Options{Format: r.FormValue("format"), Sheet: r.FormValue("sheet"), MaxCandidates: maxImportCandidates, CustomFields: defs}
	if m := strings.TrimSpace(r.FormValue("mapping")); m != "" && m != "null" {
		if err := json.Unmarshal([]byte(m), &opt.Mapping); err != nil {
			return nil, nil, badRequest("mapping must be a JSON object of column header → field: %v", err)
		}
	}
	res, err := reqimport.Parse(up.data, up.name, opt)
	if errors.Is(err, reqimport.ErrInvalid) {
		return nil, nil, badRequest("%s", strings.TrimPrefix(err.Error(), reqimport.ErrInvalid.Error()+": "))
	}
	return res, up, err
}

// importRoot returns the target folder of form field folder_id (nil = top
// level) and its path.
func (s *Server) importRoot(r *http.Request, p *Principal, pr *model.Project) (*string, string, error) {
	id := strings.TrimSpace(r.FormValue("folder_id"))
	if id == "" {
		return nil, "", nil
	}
	f, err := s.Store.GetFolder(r.Context(), p.TenantID(), id)
	if err != nil || f.ProjectID != pr.ID || f.Kind != model.KindRequirement {
		return nil, "", badRequest("folder_id: requirement folder not found in project")
	}
	folders, err := s.Store.ListFolders(r.Context(), p.TenantID(), pr.ID, model.KindRequirement)
	if err != nil {
		return nil, "", err
	}
	return &f.ID, store.FolderPaths(folders)[f.ID], nil
}

// externalIDRe finds the external IDs recorded in a requirement's source.
var externalIDRe = regexp.MustCompile(`(?i)\bID: ([^\s,;)]+)`)

// sourceWithID records an external ID in the source field.
func sourceWithID(source, id string) string {
	source = strings.TrimSpace(source)
	switch {
	case id == "":
		return source
	case source == "":
		return "ID: " + id
	case strings.Contains(strings.ToLower(source), "id: "+strings.ToLower(id)):
		return source
	}
	return source + ", ID: " + id
}

// markDuplicates sets Candidate.Duplicate: "external_id" when the external
// ID is already recorded as "ID: <id>" in the source of a requirement of the
// project (or used by an earlier candidate), else "title" when a requirement
// (or an earlier candidate) has the same title, ignoring case. It returns
// the number of duplicates.
func markDuplicates(existing []model.Requirement, cands []reqimport.Candidate) int {
	ids, titles := map[string]bool{}, map[string]bool{}
	for _, rq := range existing {
		for _, m := range externalIDRe.FindAllStringSubmatch(rq.Source, -1) {
			ids[strings.ToLower(m[1])] = true
		}
		titles[strings.ToLower(strings.TrimSpace(rq.Title))] = true
	}
	n := 0
	for i := range cands {
		c := &cands[i]
		id, title := strings.ToLower(c.ExternalID), strings.ToLower(strings.TrimSpace(c.Title))
		c.Duplicate = ""
		switch {
		case id != "" && ids[id]:
			c.Duplicate = "external_id"
		case titles[title]:
			c.Duplicate = "title"
		}
		if c.Duplicate != "" {
			n++
		}
		if id != "" {
			ids[id] = true
		}
		titles[title] = true
	}
	return n
}

type importPreview struct {
	*reqimport.Result
	Fields       []string `json:"fields"`
	Total        int      `json:"total"`
	Duplicates   int      `json:"duplicates"`
	TargetFolder string   `json:"target_folder"`
}

func (s *Server) handleImportPreview(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	res, _, err := s.parseImportUpload(w, r, p, pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	_, rootPath, err := s.importRoot(r, p, pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	existing, err := s.Store.ListRequirements(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	dups := markDuplicates(existing, res.Candidates)
	fields := append([]string(nil), reqimport.Fields...)
	if defs, err := s.Store.ListCustomFields(r.Context(), p.TenantID(), pr.ID, model.KindRequirement); err == nil {
		for _, d := range defs {
			fields = append(fields, reqimport.CustomPrefix+d.Key)
		}
	}
	writeJSON(w, http.StatusOK, importPreview{Result: res, Fields: fields, Total: len(res.Candidates), Duplicates: dups, TargetFolder: rootPath})
}

type importedItem struct {
	ID       string  `json:"id"`
	Key      string  `json:"key"`
	Title    string  `json:"title"`
	Row      int     `json:"row"`
	Location string  `json:"location"`
	FolderID *string `json:"folder_id"`
	TestID   string  `json:"test_id,omitempty"`
	TestKey  string  `json:"test_key,omitempty"`
}

type skippedItem struct {
	Title    string `json:"title"`
	Row      int    `json:"row"`
	Location string `json:"location"`
	Reason   string `json:"reason"` // duplicate_external_id | duplicate_title | excluded
}

type importResponse struct {
	Created        int            `json:"created"`
	Skipped        int            `json:"skipped"`
	FoldersCreated int            `json:"folders_created"`
	TestsCreated   int            `json:"tests_created"`
	DryRun         bool           `json:"dry_run"`
	Format         string         `json:"format"`
	Items          []importedItem `json:"items"`
	SkippedItems   []skippedItem  `json:"skipped_items"`
	Warnings       []string       `json:"warnings"`
	Errors         []string       `json:"errors"`
}

func (s *Server) handleImportRequirements(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	res, up, err := s.parseImportUpload(w, r, p, pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	root, _, err := s.importRoot(r, p, pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	createFolders := formBool(r, "create_folders", true)
	createTests := formBool(r, "create_tests", false)
	skipDups := formBool(r, "skip_duplicates", true)
	dryRun := formBool(r, "dry_run", false)
	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		status = "draft"
	}
	if !model.Valid(model.RequirementStatuses, status) {
		s.fail(w, r, badRequest("invalid status %q", status))
		return
	}
	exclude := map[int]bool{}
	if v := strings.TrimSpace(r.FormValue("exclude")); v != "" {
		var idx []int
		if err := json.Unmarshal([]byte(v), &idx); err != nil {
			s.fail(w, r, badRequest("exclude must be a JSON array of candidate indexes: %v", err))
			return
		}
		for _, i := range idx {
			exclude[i] = true
		}
	}
	if createTests && !p.Can(rbac.TestWrite) {
		writeError(w, http.StatusForbidden, "forbidden", "permission test.write required to create linked tests")
		return
	}
	ctx := r.Context()
	existing, err := s.Store.ListRequirements(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	markDuplicates(existing, res.Candidates)

	out := importResponse{DryRun: dryRun, Format: res.Format, Items: []importedItem{}, SkippedItems: []skippedItem{},
		Warnings: res.Warnings, Errors: res.Errors}
	var items []store.ImportItem
	var from []reqimport.Candidate
	for i, c := range res.Candidates {
		skip := ""
		switch {
		case exclude[i]:
			skip = "excluded"
		case skipDups && c.Duplicate != "":
			skip = "duplicate_" + c.Duplicate
		}
		if skip != "" {
			out.SkippedItems = append(out.SkippedItems, skippedItem{Title: c.Title, Row: c.Row, Location: c.Location, Reason: skip})
			continue
		}
		rq := &model.Requirement{Title: c.Title, Description: c.Description, Obligation: c.Obligation, Type: c.Type,
			Priority: c.Priority, Status: c.Status, Source: sourceWithID(c.Source, c.ExternalID), Rationale: c.Rationale,
			Owner: c.Owner, Tags: c.Tags, CreatedBy: p.User.Email}
		if rq.Status == "" {
			rq.Status = status
		}
		rq.CustomFields = s.importCustom(r, p, pr, c, &out.Warnings)
		for _, cr := range c.Criteria {
			rq.Criteria = append(rq.Criteria, model.EvaluationCriterion{Description: cr.Description, Method: cr.Method, Threshold: cr.Threshold})
		}
		if isReviewDecision(model.KindRequirement, "", rq.Status) && !p.Can(rbac.RequirementRev) {
			writeError(w, http.StatusForbidden, "forbidden", fmt.Sprintf("%s: permission requirement.review required to import an %s requirement; "+
				"choose another status or ignore the status column", c.Location, rq.Status))
			return
		}
		check := *rq
		check.Criteria = append([]model.EvaluationCriterion(nil), rq.Criteria...)
		if err := store.ValidateRequirement(&check); err != nil {
			s.fail(w, r, badRequest("%s: %s", c.Location, strings.TrimPrefix(err.Error(), store.ErrInvalid.Error()+": ")))
			return
		}
		var folder []string
		if createFolders {
			folder = reqimport.FolderParts(c.Folder)
		}
		items = append(items, store.ImportItem{Requirement: rq, Folder: folder})
		from = append(from, c)
	}
	out.Skipped = len(out.SkippedItems)
	if len(items) > 0 {
		out.FoldersCreated, err = s.Store.ImportRequirements(ctx, p.TenantID(), pr.ID, root, items, dryRun)
		if err != nil {
			s.fail(w, r, err)
			return
		}
	}
	for i, it := range items {
		rq := it.Requirement
		out.Items = append(out.Items, importedItem{ID: rq.ID, Key: rq.Key, Title: rq.Title, Row: from[i].Row, Location: from[i].Location, FolderID: rq.FolderID})
	}
	out.Created = len(out.Items)
	if dryRun {
		writeJSON(w, http.StatusOK, out)
		return
	}
	if createTests {
		var reqIDs, testIDs []string
		for i := range items {
			reqIDs = append(reqIDs, items[i].Requirement.ID)
		}
		for i := range items {
			t, err := s.createLinkedTest(ctx, p, items[i].Requirement)
			if err != nil {
				// keep the import atomic from the client's point of view
				_, _ = s.Store.DeleteTests(ctx, p.TenantID(), pr.ID, testIDs)
				_, _ = s.Store.DeleteRequirements(ctx, p.TenantID(), pr.ID, reqIDs)
				s.fail(w, r, err)
				return
			}
			testIDs = append(testIDs, t.ID)
			out.Items[i].TestID, out.Items[i].TestKey = t.ID, t.Key
		}
		out.TestsCreated = len(testIDs)
	}
	for _, it := range out.Items {
		s.recordRequirement(ctx, p, it.ID, "create")
		if it.TestID != "" {
			s.recordTest(ctx, p, it.TestID, "create")
		}
	}
	s.audit(ctx, p, "requirement.import", "project", pr.ID, fmt.Sprintf("%s (%s): %d created, %d skipped, %d folders, %d tests",
		up.name, res.Format, out.Created, out.Skipped, out.FoldersCreated, out.TestsCreated))
	writeJSON(w, http.StatusCreated, out)
}
