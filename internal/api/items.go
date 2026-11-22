package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/reqlang"
	"github.com/rom/qtestframe/internal/store"
)

func uniqStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// jsonMerge overlays the fields present in body onto v.
func jsonMerge(body []byte, v any) error {
	if len(body) == 0 {
		return badRequest("request body required")
	}
	if err := json.Unmarshal(body, v); err != nil {
		return badRequest("invalid JSON: %v", err)
	}
	return nil
}

func boolParam(r *http.Request, name string, def bool) bool {
	switch strings.ToLower(r.URL.Query().Get(name)) {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	}
	return def
}

// ---------------------------------------------------------------- requirements

// filterRequirements returns decorated requirements matching q and folder.
func (s *Server) filterRequirements(r *http.Request, p *Principal, pr *model.Project, pc *projectContext, expr, folder string, recursive bool) ([]model.Requirement, error) {
	pc.forCaller(p)
	q, err := query.Parse(expr, requirementSchema)
	if err != nil {
		return nil, err
	}
	s.bindMe(r.Context(), p, q)
	reqs, err := s.Store.ListRequirements(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		return nil, err
	}
	// ?variant=: only the requirements that apply to it, with their coverage there
	variant, err := pc.listVariant(r)
	if err != nil {
		return nil, err
	}
	var vc *store.VariantCoverage
	if variant != "" {
		if vc, err = s.variantCoverage(r.Context(), p.TenantID(), pc); err != nil {
			return nil, err
		}
	}
	inFolder := pc.folderFilter(folder, recursive)
	out := make([]model.Requirement, 0, len(reqs))
	var recs []query.MapRecord
	for i := range reqs {
		rq := &reqs[i]
		pc.decorateRequirement(rq)
		if vc != nil {
			if !vc.AppliesTo(rq.ID, variant) {
				continue
			}
			rq.Coverage = vc.Requirement(rq.ID, variant)
			rq.LinkedTests = len(vc.Tests(rq.ID, variant))
		}
		if rec := pc.requirementRecord(rq); inFolder(rq.FolderID) && q.Match(rec) {
			out = append(out, *rq)
			recs = append(recs, rec)
		}
	}
	sortByQuery(q, out, recs)
	return out, nil
}

func (s *Server) handleListRequirements(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	pc, err := s.loadContext(r.Context(), p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.filterRequirements(r, p, pr, pc, r.URL.Query().Get("q"), r.URL.Query().Get("folder_id"), boolParam(r, "recursive", true))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for i := range out {
		out[i].Criteria = nil
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

func (s *Server) handleCreateRequirement(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		model.Requirement
		// CreateTest overrides requirements.auto_create_test for this request.
		CreateTest *bool `json:"create_test"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	rq := req.Requirement
	createTest := s.conf().Requirements.AutoCreateTest
	if req.CreateTest != nil {
		createTest = *req.CreateTest
	}
	if isReviewDecision(model.KindRequirement, "", rq.Status) && !p.Can(rbac.RequirementRev) {
		writeError(w, http.StatusForbidden, "forbidden", "permission requirement.review required to create an "+rq.Status+" requirement")
		return
	}
	if createTest && !p.Can(rbac.TestWrite) {
		if req.CreateTest != nil {
			writeError(w, http.StatusForbidden, "forbidden", "permission test.write required to create a linked test")
			return
		}
		createTest = false // default behaviour degrades silently
	}
	rq.ProjectID = pr.ID
	rq.CreatedBy = p.User.Email
	if err := s.normalizeOwner(r.Context(), p, pr.ID, &rq.Owner); err != nil {
		s.fail(w, r, err)
		return
	}
	if rq.Obligation == "" {
		rq.Obligation = reqlang.DetectObligation(rq.Title + "\n" + rq.Description)
	}
	ctx := r.Context()
	if err := s.Store.CreateRequirement(ctx, p.TenantID(), &rq); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "requirement.create", "requirement", rq.ID, rq.Key)
	s.afterItemChange(r, p, model.KindRequirement, rq.ID, rq.ProjectID, rq.Key, rq.Title, "", rq.Owner, rq.Description, true)
	s.recordRequirement(ctx, p, rq.ID, "create")
	res := createdRequirement{Requirement: rq}
	if createTest {
		t, err := s.createLinkedTest(ctx, p, &rq)
		if err != nil {
			// keep the operation atomic from the client's point of view
			_, _ = s.Store.DeleteRequirements(ctx, p.TenantID(), pr.ID, []string{rq.ID})
			s.fail(w, r, err)
			return
		}
		s.audit(ctx, p, "test.create", "test", t.ID, t.Key+" (linked to "+rq.Key+")")
		s.recordTest(ctx, p, t.ID, "create")
		res.LinkedTests = 1
		res.Coverage = "not_run"
		res.LinkedTest = t
	}
	writeJSON(w, http.StatusCreated, res)
}

// createdRequirement is the create response: the requirement plus the
// automatically created linked test, if any.
type createdRequirement struct {
	model.Requirement
	LinkedTest *model.TestCase `json:"linked_test,omitempty"`
}

// createLinkedTest creates a draft manual test verifying rq and links it.
// The test goes into a test folder whose path mirrors the requirement's
// folder path (created on demand). Each evaluation criterion becomes a step.
func (s *Server) createLinkedTest(ctx context.Context, p *Principal, rq *model.Requirement) (*model.TestCase, error) {
	folderID, err := s.mirrorFolder(ctx, p.TenantID(), rq.ProjectID, rq.FolderID)
	if err != nil {
		return nil, err
	}
	title := "Verify " + rq.Key + ": " + rq.Title
	if r := []rune(title); len(r) > 500 {
		title = string(r[:500])
	}
	t := &model.TestCase{
		ProjectID: rq.ProjectID, FolderID: folderID, Title: title,
		Description:   "Verifies requirement " + rq.Key + ".\n\n" + rq.Description,
		ExecutionType: model.ExecManual, Status: "draft", Priority: rq.Priority,
		Tags: []string{"auto-generated"}, CreatedBy: p.User.Email,
	}
	for _, c := range rq.Criteria {
		expected := c.Threshold
		if expected == "" {
			expected = "Criterion is met"
		}
		t.Steps = append(t.Steps, model.TestStep{Action: "Verify (" + c.Method + "): " + c.Description, Expected: expected})
	}
	if len(t.Steps) == 0 {
		t.Steps = []model.TestStep{{Action: "Verify that the requirement is fulfilled: " + rq.Title, Expected: rq.Description}}
	}
	// the linked test gets the defaults of the test custom fields
	if err := s.Store.CreateTest(store.LenientCustomFields(ctx), p.TenantID(), t); err != nil {
		return nil, err
	}
	link := &model.TraceLink{RequirementID: rq.ID, TestCaseID: t.ID}
	if err := s.Store.CreateLink(ctx, p.TenantID(), link); err != nil {
		_, _ = s.Store.DeleteTests(ctx, p.TenantID(), rq.ProjectID, []string{t.ID})
		return nil, err
	}
	t.LinkedReqs = 1
	t.LastResult = model.ResultNotRun
	return t, nil
}

// mirrorFolder returns the id of the test folder with the same name path as
// requirement folder reqFolder, creating missing folders. nil means top level.
func (s *Server) mirrorFolder(ctx context.Context, tenantID, projectID string, reqFolder *string) (*string, error) {
	if reqFolder == nil {
		return nil, nil
	}
	reqFolders, err := s.Store.ListFolders(ctx, tenantID, projectID, model.KindRequirement)
	if err != nil {
		return nil, err
	}
	byID := map[string]model.Folder{}
	for _, f := range reqFolders {
		byID[f.ID] = f
	}
	var names, colors []string
	for id := reqFolder; id != nil && len(names) < 64; {
		f, ok := byID[*id]
		if !ok {
			break
		}
		names = append([]string{f.Name}, names...)
		colors = append([]string{f.Color}, colors...)
		id = f.ParentID
	}
	testFolders, err := s.Store.ListFolders(ctx, tenantID, projectID, model.KindTest)
	if err != nil {
		return nil, err
	}
	var parent *string
	for i, name := range names {
		var found *string
		for j := range testFolders {
			f := testFolders[j]
			if f.Name == name && ((parent == nil && f.ParentID == nil) || (parent != nil && f.ParentID != nil && *f.ParentID == *parent)) {
				found = &testFolders[j].ID
				break
			}
		}
		if found == nil {
			// a new mirror folder takes the requirement folder's colour
			nf := &model.Folder{ProjectID: projectID, Kind: model.KindTest, Name: name, ParentID: parent, Color: colors[i]}
			if err := s.Store.CreateFolder(ctx, tenantID, nf); err != nil {
				return nil, err
			}
			testFolders = append(testFolders, *nf)
			found = &nf.ID
		}
		parent = found
	}
	return parent, nil
}

func (s *Server) decoratedRequirement(r *http.Request, p *Principal, id string) (*model.Requirement, error) {
	rq, err := s.Store.GetRequirement(r.Context(), p.TenantID(), id)
	if err != nil {
		return nil, err
	}
	pr, err := s.Store.GetProject(r.Context(), p.TenantID(), rq.ProjectID)
	if err != nil {
		return nil, err
	}
	pc, err := s.loadContext(r.Context(), p.TenantID(), pr)
	if err != nil {
		return nil, err
	}
	pc.decorateRequirement(rq)
	return rq, nil
}

func (s *Server) handleGetRequirement(w http.ResponseWriter, r *http.Request, p *Principal) {
	rq, err := s.decoratedRequirement(r, p, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setVersionTag(w, rq.Version)
	writeJSON(w, http.StatusOK, rq)
}

func (s *Server) handleUpdateRequirement(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	rq, err := s.Store.GetRequirement(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !s.requireVersion(w, r, p, "requirement", rq.ID, rq.Version) {
		return
	}
	body, err := readBody(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	keys, err := patchKeys(body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id, project, key, oldStatus := rq.ID, rq.ProjectID, rq.Key, rq.Status
	oldOwner, oldDesc := rq.Owner, rq.Description
	before := store.SnapshotRequirement(rq)
	if keys["criteria"] {
		rq.Criteria = nil
	}
	if err := jsonMerge(body, rq); err != nil {
		s.fail(w, r, err)
		return
	}
	rq.ID, rq.ProjectID, rq.Key = id, project, key
	if !authorizeChange(w, p, model.KindRequirement, onlyKeys(keys, "status"), oldStatus, rq.Status) {
		return
	}
	if keys["owner"] && rq.Owner != oldOwner {
		if err := s.normalizeOwner(ctx, p, rq.ProjectID, &rq.Owner); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.ensureInitialRevision(ctx, p, model.KindRequirement, rq.ID)
	if err := s.Store.UpdateRequirement(ctx, p.TenantID(), rq, keys["criteria"]); isStale(err) {
		s.staleItem(w, r, p, "requirement", rq.ID)
		return
	} else if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "requirement.update", "requirement", rq.ID, rq.Key)
	s.afterItemChange(r, p, model.KindRequirement, rq.ID, rq.ProjectID, rq.Key, rq.Title, oldOwner, rq.Owner, newText(oldDesc, rq.Description), false)
	if a := reviewAction(model.KindRequirement, oldStatus, rq.Status); a != "" {
		s.audit(ctx, p, a, "requirement", rq.ID, rq.Key+" "+oldStatus+" → "+rq.Status)
	}
	s.recordRequirement(ctx, p, rq.ID, "update")
	s.afterContentChange(ctx, p, model.KindRequirement, rq.ID, rq.Key, rq.ProjectID, before)
	if inBatch(ctx) {
		// a step of a batch update (batch.go): the stored item is enough
		writeJSON(w, http.StatusOK, rq)
		return
	}
	out, err := s.decoratedRequirement(r, p, rq.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setVersionTag(w, out.Version)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteRequirement(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	n, trashID, err := s.Store.TrashRequirements(r.Context(), p.TenantID(), "", []string{id}, p.User.Email)
	if err == nil && n == 0 {
		err = store.ErrNotFound
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "requirement.delete", "requirement", id, "")
	w.Header().Set(trashHeader, trashID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Text string `json:"text"`
		// Language is a pack code ("en", "sv"); "" or "auto" detects it.
		Language string `json:"language"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Language != "" && req.Language != "auto" && !reqlang.Supported(req.Language) {
		s.fail(w, r, badRequest("language must be auto or one of %s", strings.Join(reqlang.Languages(), ", ")))
		return
	}
	writeJSON(w, http.StatusOK, reqlang.AnalyzeLang(req.Text, req.Language))
}

// ---------------------------------------------------------------- move / bulk

func (s *Server) handleMoveItems(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		pr, ok := s.project(w, r, p)
		if !ok {
			return
		}
		var req struct {
			IDs       []string `json:"ids"`
			FolderID  *string  `json:"folder_id"`
			Position  *int     `json:"position"`
			ProjectID string   `json:"project_id"`
		}
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
		pos := -1
		if req.Position != nil {
			pos = *req.Position
		}
		// moving into another project needs write access there too
		if req.ProjectID != "" && req.ProjectID != pr.ID {
			if err := s.canIn(r.Context(), p, req.ProjectID, kindPerm(kind)); err != nil {
				s.fail(w, r, err)
				return
			}
		}
		for _, id := range uniqStrings(req.IDs) {
			s.ensureInitialRevision(r.Context(), p, kind, id)
		}
		if err := s.Store.MoveItems(r.Context(), p.TenantID(), kind, pr.ID, req.IDs, req.FolderID, pos, req.ProjectID); err != nil {
			s.fail(w, r, err)
			return
		}
		for _, id := range uniqStrings(req.IDs) {
			s.record(r.Context(), p, kind, id, "move")
		}
		s.audit(r.Context(), p, kind+".move", kind, strings.Join(req.IDs, ","), req.ProjectID)
		writeJSON(w, http.StatusOK, map[string]int{"moved": len(req.IDs)})
	}
}

func (s *Server) handleBulk(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		pr, ok := s.project(w, r, p)
		if !ok {
			return
		}
		var req struct {
			IDs []string         `json:"ids"`
			Set store.BulkUpdate `json:"set"`
		}
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
		// a review decision needs the review permission; any other change write
		statusOnly := req.Set.Priority == nil && req.Set.Obligation == nil && req.Set.ExecutionType == nil && req.Set.Owner == nil &&
			len(req.Set.AddCategoryIDs) == 0 && len(req.Set.RemoveCategoryIDs) == 0 && len(req.Set.AddTags) == 0 && len(req.Set.RemoveTags) == 0
		if req.Set.Status != nil {
			if !authorizeChange(w, p, kind, statusOnly, "", *req.Set.Status) {
				return
			}
		} else if !p.Can(writePerm(kind)) {
			writeError(w, http.StatusForbidden, "forbidden", "permission "+string(writePerm(kind))+" required")
			return
		}
		if err := s.normalizeOwner(r.Context(), p, pr.ID, req.Set.Owner); err != nil {
			s.fail(w, r, err)
			return
		}
		// owners before the change: a new owner is told (notify.go)
		owners := map[string]string{}
		for _, id := range uniqStrings(req.IDs) {
			s.ensureInitialRevision(r.Context(), p, kind, id)
			if req.Set.Owner != nil {
				owners[id] = s.ownerNow(r.Context(), p, kind, id)
			}
		}
		n, err := s.Store.BulkUpdateItems(r.Context(), p.TenantID(), kind, pr.ID, req.IDs, req.Set)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, id := range uniqStrings(req.IDs) {
			s.record(r.Context(), p, kind, id, "bulk_update")
			if req.Set.Owner != nil && *req.Set.Owner != "" && owners[id] != *req.Set.Owner {
				s.ownerAssigned(r, p, kind, pr.ID, id)
			}
		}
		s.audit(r.Context(), p, kind+".bulk_update", kind, strings.Join(req.IDs, ","), "")
		writeJSON(w, http.StatusOK, map[string]int{"updated": n})
	}
}

func (s *Server) handleBulkDelete(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		pr, ok := s.project(w, r, p)
		if !ok {
			return
		}
		var req struct {
			IDs []string `json:"ids"`
		}
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
		del := s.Store.TrashRequirements
		if kind == model.KindTest {
			del = s.Store.TrashTests
		}
		n, trashID, err := del(r.Context(), p.TenantID(), pr.ID, req.IDs, p.User.Email)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(r.Context(), p, kind+".bulk_delete", kind, strings.Join(req.IDs, ","), "")
		writeJSON(w, http.StatusOK, map[string]any{"deleted": n, "trash_id": trashID})
	}
}

// ---------------------------------------------------------------- tests

func (s *Server) filterTests(r *http.Request, p *Principal, pr *model.Project, pc *projectContext, expr, folder string, recursive bool) ([]model.TestCase, error) {
	pc.forCaller(p)
	q, err := query.Parse(expr, testSchema)
	if err != nil {
		return nil, err
	}
	s.bindMe(r.Context(), p, q)
	tests, err := s.Store.ListTests(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		return nil, err
	}
	pc.indexCalls(tests)
	// ?variant=: only the tests that apply to it, with their result there
	variant, err := pc.listVariant(r)
	if err != nil {
		return nil, err
	}
	var vc *store.VariantCoverage
	if variant != "" {
		if vc, err = s.variantCoverage(r.Context(), p.TenantID(), pc); err != nil {
			return nil, err
		}
	}
	inFolder := pc.folderFilter(folder, recursive)
	out := make([]model.TestCase, 0, len(tests))
	var recs []query.MapRecord
	for i := range tests {
		t := &tests[i]
		pc.decorateTest(t)
		if vc != nil {
			if !vc.AppliesTo(t.ID, variant) {
				continue
			}
			t.LastResult = vc.TestResult(t.ID, variant)
		}
		if rec := pc.testRecord(t); inFolder(t.FolderID) && q.Match(rec) {
			out = append(out, *t)
			recs = append(recs, rec)
		}
	}
	sortByQuery(q, out, recs)
	return out, nil
}

func (s *Server) handleListTests(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	pc, err := s.loadContext(r.Context(), p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.filterTests(r, p, pr, pc, r.URL.Query().Get("q"), r.URL.Query().Get("folder_id"), boolParam(r, "recursive", true))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for i := range out {
		out[i].Steps = nil
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

func (s *Server) handleCreateTest(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var t model.TestCase
	if err := decode(r, &t); err != nil {
		s.fail(w, r, err)
		return
	}
	if isReviewDecision(model.KindTest, "", t.Status) && !p.Can(rbac.TestReview) {
		writeError(w, http.StatusForbidden, "forbidden", "permission test.review required to create a ready test")
		return
	}
	t.ProjectID = pr.ID
	t.CreatedBy = p.User.Email
	if err := s.normalizeOwner(r.Context(), p, pr.ID, &t.Owner); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.CreateTest(r.Context(), p.TenantID(), &t); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "test.create", "test", t.ID, t.Key)
	s.afterItemChange(r, p, model.KindTest, t.ID, t.ProjectID, t.Key, t.Title, "", t.Owner, t.Description, true)
	s.recordTest(r.Context(), p, t.ID, "create")
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) decoratedTest(r *http.Request, p *Principal, id string) (*model.TestCase, error) {
	t, err := s.Store.GetTest(r.Context(), p.TenantID(), id)
	if err != nil {
		return nil, err
	}
	pr, err := s.Store.GetProject(r.Context(), p.TenantID(), t.ProjectID)
	if err != nil {
		return nil, err
	}
	pc, err := s.loadContext(r.Context(), p.TenantID(), pr)
	if err != nil {
		return nil, err
	}
	pc.decorateTest(t)
	return t, nil
}

func (s *Server) handleGetTest(w http.ResponseWriter, r *http.Request, p *Principal) {
	t, err := s.decoratedTest(r, p, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setVersionTag(w, t.Version)
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleUpdateTest(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	t, err := s.Store.GetTest(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !s.requireVersion(w, r, p, "test", t.ID, t.Version) {
		return
	}
	body, err := readBody(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	keys, err := patchKeys(body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id, project, key, oldStatus := t.ID, t.ProjectID, t.Key, t.Status
	oldOwner, oldDesc := t.Owner, t.Description
	if keys["steps"] {
		t.Steps = nil
	}
	before := store.SnapshotTest(t)
	if keys["parameters"] {
		t.Parameters = nil
	}
	if keys["data_rows"] {
		t.DataRows = nil
	}
	if err := jsonMerge(body, t); err != nil {
		s.fail(w, r, err)
		return
	}
	t.ID, t.ProjectID, t.Key = id, project, key
	if !authorizeChange(w, p, model.KindTest, onlyKeys(keys, "status"), oldStatus, t.Status) {
		return
	}
	if keys["owner"] && t.Owner != oldOwner {
		if err := s.normalizeOwner(ctx, p, t.ProjectID, &t.Owner); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.ensureInitialRevision(ctx, p, model.KindTest, t.ID)
	if err := s.Store.UpdateTest(ctx, p.TenantID(), t, keys["steps"]); isStale(err) {
		s.staleItem(w, r, p, "test", t.ID)
		return
	} else if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "test.update", "test", t.ID, t.Key)
	s.afterItemChange(r, p, model.KindTest, t.ID, t.ProjectID, t.Key, t.Title, oldOwner, t.Owner, newText(oldDesc, t.Description), false)
	if a := reviewAction(model.KindTest, oldStatus, t.Status); a != "" {
		s.audit(ctx, p, a, "test", t.ID, t.Key+" "+oldStatus+" → "+t.Status)
	}
	s.recordTest(ctx, p, t.ID, "update")
	s.afterContentChange(ctx, p, model.KindTest, t.ID, t.Key, t.ProjectID, before)
	if inBatch(ctx) {
		writeJSON(w, http.StatusOK, t)
		return
	}
	out, err := s.decoratedTest(r, p, t.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setVersionTag(w, out.Version)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteTest(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	n, trashID, err := s.Store.TrashTests(r.Context(), p.TenantID(), "", []string{id}, p.User.Email)
	if err == nil && n == 0 {
		err = store.ErrNotFound
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "test.delete", "test", id, "")
	w.Header().Set(trashHeader, trashID)
	w.WriteHeader(http.StatusNoContent)
}

// handleCopyRequirement: POST /requirements/{id}/copy {"title"?, "relation"?}
// creates a draft copy (criteria, categories, tags, folder) as the basis of a
// new requirement; relation "derived_from" also relates the copy to the
// original.
func (s *Server) handleCopyRequirement(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Title    string `json:"title"`
		Relation string `json:"relation"`
	}
	if r.ContentLength != 0 {
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if req.Relation != "" && !model.Valid(model.RelationTypes, req.Relation) {
		s.fail(w, r, badRequest("relation must be one of %s", strings.Join(model.RelationTypes, ", ")))
		return
	}
	ctx := r.Context()
	rq, err := s.Store.CopyRequirement(ctx, p.TenantID(), r.PathValue("id"), req.Title, p.User.Email)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "requirement.copy", "requirement", rq.ID, rq.Key)
	s.recordRequirement(ctx, p, rq.ID, "create")
	if req.Relation != "" {
		rel := &model.Relation{SourceID: rq.ID, TargetID: r.PathValue("id"), Type: req.Relation, CreatedBy: p.User.Email}
		if err := s.Store.CreateRelation(ctx, p.TenantID(), rel); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	out, err := s.decoratedRequirement(r, p, rq.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleCopyTest(w http.ResponseWriter, r *http.Request, p *Principal) {
	t, err := s.Store.CopyTest(r.Context(), p.TenantID(), r.PathValue("id"), p.User.Email)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "test.copy", "test", t.ID, t.Key)
	s.recordTest(r.Context(), p, t.ID, "create")
	writeJSON(w, http.StatusCreated, t)
}

// ---------------------------------------------------------------- links

func (s *Server) handleItemLinks(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		id := r.PathValue("id")
		var err error
		if kind == model.KindRequirement {
			_, err = s.Store.GetRequirement(r.Context(), p.TenantID(), id)
		} else {
			_, err = s.Store.GetTest(r.Context(), p.TenantID(), id)
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		var links []model.TraceLink
		if kind == model.KindRequirement {
			links, err = s.Store.ListLinks(r.Context(), p.TenantID(), "", id, "")
		} else {
			links, err = s.Store.ListLinks(r.Context(), p.TenantID(), "", "", id)
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, listOf(links))
	}
}

func (s *Server) handleListLinks(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	links, err := s.Store.ListLinks(r.Context(), p.TenantID(), pr.ID, "", "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(links))
}

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var l model.TraceLink
	if err := decode(r, &l); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.CreateLink(r.Context(), p.TenantID(), &l); err != nil {
		s.fail(w, r, err)
		return
	}
	if l.ProjectID != pr.ID {
		_ = s.Store.DeleteLink(r.Context(), p.TenantID(), l.ID)
		s.fail(w, r, badRequest("items do not belong to this project"))
		return
	}
	s.audit(r.Context(), p, "link.create", "link", l.ID, l.RequirementKey+" -> "+l.TestCaseKey)
	s.syncEvidence(r.Context(), p, pr.ID, []string{l.RequirementID})
	writeJSON(w, http.StatusCreated, l)
}

func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	l, err := s.Store.GetLink(r.Context(), p.TenantID(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteLink(r.Context(), p.TenantID(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "link.delete", "link", id, "")
	s.syncEvidence(r.Context(), p, l.ProjectID, []string{l.RequirementID})
	w.WriteHeader(http.StatusNoContent)
}

type traceReq struct {
	ID         string `json:"id"`
	Key        string `json:"key"`
	Title      string `json:"title"`
	Obligation string `json:"obligation"`
	Coverage   string `json:"coverage"`
	// Blocked: open defects block it; StaleResults: linked tests with a
	// stale latest result (correlations.go).
	Blocked      bool `json:"blocked,omitempty"`
	StaleResults int  `json:"stale_results,omitempty"`
	// Verification: the rolled-up verification state; Criteria: the
	// criteria with their states (criterion rows of the matrix);
	// Records: the number of verification records that count.
	Verification string           `json:"verification"`
	Criteria     []traceCriterion `json:"criteria"`
	Records      int              `json:"records"`
	ParentIDs    []string         `json:"parent_ids"`
}

// traceCriterion is a criterion row of the traceability matrix.
type traceCriterion struct {
	ID          string `json:"id"`
	Position    int    `json:"position"`
	Description string `json:"description"`
	Method      string `json:"method"`
	State       string `json:"state"`
	// Records counted for it.
	Records int `json:"records"`
}

type traceTest struct {
	ID         string `json:"id"`
	Key        string `json:"key"`
	Title      string `json:"title"`
	LastResult string `json:"last_result"`
	// Stale: the latest result is stale (no passing evidence).
	Stale bool `json:"stale,omitempty"`
}

func (s *Server) handleTraceability(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	pc, err := s.loadContext(r.Context(), p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reqs, err := s.filterRequirements(r, p, pr, pc, r.URL.Query().Get("q"), "", true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	tests, err := s.filterTests(r, p, pr, pc, r.URL.Query().Get("tq"), "", true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	res := struct {
		Requirements []traceReq        `json:"requirements"`
		Tests        []traceTest       `json:"tests"`
		Links        []model.TraceLink `json:"links"`
		// Configuration the coverage and results are filtered to ("none":
		// results without configuration), when given.
		Configuration string `json:"configuration,omitempty"`
		// Variant the requirements, tests, coverage and results are of
		// (?variant=), when given.
		Variant string `json:"variant,omitempty"`
	}{Requirements: []traceReq{}, Tests: []traceTest{}, Links: pc.links}
	// ?configuration=<id> | none: coverage and results in that configuration only
	cfg, filtered := r.URL.Query().Get("configuration"), false
	if cfg != "" {
		filtered = true
		res.Configuration = cfg
		if cfg == "none" {
			cfg = store.NoConfiguration
		} else if c, err := s.Store.GetConfiguration(r.Context(), p.TenantID(), cfg); err != nil || c.ProjectID != pr.ID {
			s.fail(w, r, badRequest("unknown configuration %q", cfg))
			return
		}
	}
	// with ?variant= the requirements and tests are already those of the variant
	variant, _ := pc.listVariant(r)
	var vc *store.VariantCoverage
	if variant != "" {
		if vc, err = s.variantCoverage(r.Context(), p.TenantID(), pc); err != nil {
			s.fail(w, r, err)
			return
		}
		res.Variant = variant
	}
	for _, rq := range reqs {
		cov := rq.Coverage
		if filtered && vc != nil {
			cov = vc.RequirementIn(rq.ID, variant, cfg)
		} else if filtered {
			cov = store.CoverageInConfiguration(pc.r2t[rq.ID], pc.cfgResults, cfg)
		}
		tr := traceReq{ID: rq.ID, Key: rq.Key, Title: rq.Title, Obligation: rq.Obligation, Coverage: cov, Verification: rq.Verification,
			Criteria: []traceCriterion{}, ParentIDs: []string{},
			Blocked: rq.BlockingDefects > 0, StaleResults: rq.StaleResults}
		if v := pc.verification()[rq.ID]; v != nil {
			tr.ParentIDs = v.ParentIDs
			for _, c := range v.Criteria {
				tc := traceCriterion{ID: c.CriterionID, Position: c.Position, Description: c.Description, Method: c.Method, State: c.State}
				for _, e := range c.Evidence {
					if e.Kind == "record" && e.State != model.EvidenceVoid {
						tc.Records++
						tr.Records++
					}
				}
				tr.Criteria = append(tr.Criteria, tc)
			}
			for _, e := range v.Evidence {
				if e.Kind == "record" && e.State != model.EvidenceVoid {
					tr.Records++
				}
			}
		}
		res.Requirements = append(res.Requirements, tr)
	}
	for _, t := range tests {
		last := t.LastResult
		if filtered && vc != nil {
			last = vc.Results(variant)[t.ID][cfg]
			if last == "" {
				last = model.ResultNotRun
			}
		} else if filtered {
			last = pc.cfgRaw[t.ID][cfg]
			if last == "" {
				last = model.ResultNotRun
			}
		}
		stale := t.Stale
		if filtered {
			stale = pc.stale.Unit(t.ID, cfg).Stale
		}
		res.Tests = append(res.Tests, traceTest{t.ID, t.Key, t.Title, last, stale})
	}
	writeJSON(w, http.StatusOK, res)
}
