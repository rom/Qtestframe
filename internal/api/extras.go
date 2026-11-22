package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// ---------------------------------------------------------------- revisions

// recordRequirement stores a revision of requirement id; failures are
// logged, never fatal to the request.
func (s *Server) recordRequirement(ctx context.Context, p *Principal, id, action string) {
	r, err := s.Store.GetRequirement(ctx, p.TenantID(), id)
	if err == nil {
		err = s.Store.AddRevision(ctx, p.TenantID(), model.KindRequirement, id, r.Version, action, p.User.Email, store.SnapshotRequirement(r))
	}
	if err != nil {
		s.Log.Warn("revision not recorded", "entity", "requirement", "id", id, "err", err)
	}
}

func (s *Server) recordTest(ctx context.Context, p *Principal, id, action string) {
	t, err := s.Store.GetTest(ctx, p.TenantID(), id)
	if err == nil {
		err = s.Store.AddRevision(ctx, p.TenantID(), model.KindTest, id, t.Version, action, p.User.Email, store.SnapshotTest(t))
	}
	if err != nil {
		s.Log.Warn("revision not recorded", "entity", "test", "id", id, "err", err)
	}
}

func (s *Server) record(ctx context.Context, p *Principal, kind, id, action string) {
	if kind == model.KindTest {
		s.recordTest(ctx, p, id, action)
	} else {
		s.recordRequirement(ctx, p, id, action)
	}
}

// ensureInitialRevision records the state before a change for items that
// predate revision tracking, so their first diff is meaningful.
func (s *Server) ensureInitialRevision(ctx context.Context, p *Principal, kind, id string) {
	revs, err := s.Store.ListRevisions(ctx, p.TenantID(), kind, id)
	if err != nil || len(revs) > 0 {
		return
	}
	s.record(ctx, p, kind, id, "import")
}

func (s *Server) handleHistory(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		id := r.PathValue("id")
		if err := s.itemExists(r.Context(), p, kind, id); err != nil {
			s.fail(w, r, err)
			return
		}
		revs, err := s.Store.ListRevisions(r.Context(), p.TenantID(), kind, id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, listOf(revs))
	}
}

func (s *Server) itemExists(ctx context.Context, p *Principal, kind, id string) error {
	var err error
	if kind == model.KindTest {
		_, err = s.Store.GetTest(ctx, p.TenantID(), id)
	} else {
		_, err = s.Store.GetRequirement(ctx, p.TenantID(), id)
	}
	return err
}

func (s *Server) handleRestoreRequirement(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	cur, err := s.Store.GetRequirement(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req struct {
		RevisionID string `json:"revision_id"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	rev, err := s.Store.GetRevision(ctx, p.TenantID(), model.KindRequirement, cur.ID, req.RevisionID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var old model.Requirement
	if err := json.Unmarshal(rev.Snapshot, &old); err != nil {
		s.fail(w, r, err)
		return
	}
	// restoring an approved version is a review decision too
	if isReviewDecision(model.KindRequirement, cur.Status, old.Status) && !p.Can(rbac.RequirementRev) {
		writeError(w, http.StatusForbidden, "forbidden", "permission requirement.review required to restore a version with status "+old.Status)
		return
	}
	if err := s.restoreRequirementTo(ctx, p, cur, &old, rev.Snapshot, "version "+itoa(rev.Version)); err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.decoratedRequirement(r, p, cur.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRestoreTest(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	cur, err := s.Store.GetTest(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req struct {
		RevisionID string `json:"revision_id"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	rev, err := s.Store.GetRevision(ctx, p.TenantID(), model.KindTest, cur.ID, req.RevisionID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var old model.TestCase
	if err := json.Unmarshal(rev.Snapshot, &old); err != nil {
		s.fail(w, r, err)
		return
	}
	if isReviewDecision(model.KindTest, cur.Status, old.Status) && !p.Can(rbac.TestReview) {
		writeError(w, http.StatusForbidden, "forbidden", "permission test.review required to restore a version with status "+old.Status)
		return
	}
	if err := s.restoreTestTo(ctx, p, cur, &old, rev.Snapshot, "version "+itoa(rev.Version)); err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.decoratedTest(r, p, cur.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// restoreRequirementTo gives a requirement the content of an older version
// (from a revision or a baseline, snap its snapshot: the applicability it
// records is restored too); what names the source for the audit.
func (s *Server) restoreRequirementTo(ctx context.Context, p *Principal, cur, old *model.Requirement, snap json.RawMessage, what string) error {
	s.ensureInitialRevision(ctx, p, model.KindRequirement, cur.ID)
	before := store.SnapshotRequirement(cur)
	cur.Title, cur.Description, cur.Obligation, cur.Type = old.Title, old.Description, old.Obligation, old.Type
	cur.Priority, cur.Status, cur.Source, cur.Rationale = old.Priority, old.Status, old.Source, old.Rationale
	cur.Owner, cur.Tags, cur.Criteria = old.Owner, old.Tags, old.Criteria
	cur.CustomFields = s.restorableCustom(ctx, p, cur.ProjectID, model.KindRequirement, old.CustomFields, cur.CustomFields)
	var err error
	if cur.CategoryIDs, err = s.existingCategories(ctx, p, cur.ProjectID, old.CategoryIDs); err != nil {
		return err
	}
	if err := s.Store.UpdateRequirement(ctx, p.TenantID(), cur, true); err != nil {
		return err
	}
	if _, err := s.restoreApplicability(ctx, p, model.KindRequirement, cur.ID, cur.ProjectID, snap); err != nil {
		return err
	}
	s.recordRequirement(ctx, p, cur.ID, "restore")
	s.afterContentChange(ctx, p, model.KindRequirement, cur.ID, cur.Key, cur.ProjectID, before)
	s.audit(ctx, p, "requirement.restore", "requirement", cur.ID, cur.Key+" to "+what)
	return nil
}

// restoreTestTo gives a test the content of an older version.
func (s *Server) restoreTestTo(ctx context.Context, p *Principal, cur, old *model.TestCase, snap json.RawMessage, what string) error {
	s.ensureInitialRevision(ctx, p, model.KindTest, cur.ID)
	before := store.SnapshotTest(cur)
	cur.Title, cur.Description, cur.Preconditions, cur.ExecutionType = old.Title, old.Description, old.Preconditions, old.ExecutionType
	cur.ScriptLanguage, cur.Script, cur.ExpectedResult = old.ScriptLanguage, old.Script, old.ExpectedResult
	cur.Priority, cur.Status, cur.EstimatedMin, cur.Tags = old.Priority, old.Status, old.EstimatedMin, old.Tags
	cur.Owner = old.Owner
	cur.CustomFields = s.restorableCustom(ctx, p, cur.ProjectID, model.KindTest, old.CustomFields, cur.CustomFields)
	var err error
	if cur.CategoryIDs, err = s.existingCategories(ctx, p, cur.ProjectID, old.CategoryIDs); err != nil {
		return err
	}
	// steps get fresh IDs; calls to tests that no longer exist are dropped
	cur.Steps = cur.Steps[:0]
	for _, st := range old.Steps {
		st.ID = ""
		if st.CallTestID != nil {
			if _, err := s.Store.GetTest(ctx, p.TenantID(), *st.CallTestID); err != nil {
				continue
			}
		}
		cur.Steps = append(cur.Steps, st)
	}
	if err := s.Store.UpdateTest(ctx, p.TenantID(), cur, true); err != nil {
		return err
	}
	if _, err := s.restoreApplicability(ctx, p, model.KindTest, cur.ID, cur.ProjectID, snap); err != nil {
		return err
	}
	s.recordTest(ctx, p, cur.ID, "restore")
	s.afterContentChange(ctx, p, model.KindTest, cur.ID, cur.Key, cur.ProjectID, before)
	s.audit(ctx, p, "test.restore", "test", cur.ID, cur.Key+" to "+what)
	return nil
}

// snapshotApplicability returns the applicability a snapshot records,
// limited to the variants that still exist, and whether it records one
// (snapshots from before applicability was recorded do not).
func (s *Server) snapshotApplicability(ctx context.Context, p *Principal, projectID string, snap json.RawMessage) (*model.Applicability, bool, error) {
	a, ok := store.SnapshotApplicability(snap)
	if !ok || a == nil {
		return nil, ok, nil
	}
	names, err := s.variantNameMap(ctx, p, projectID)
	if err != nil {
		return nil, false, err
	}
	ids := []string{}
	for _, id := range a.VariantIDs {
		if _, ok := names[id]; ok {
			ids = append(ids, id)
		}
	}
	return &model.Applicability{VariantIDs: ids, Expression: a.Expression}, true, nil
}

// applicabilityDiffers reports whether an item's applicability differs
// from the one a snapshot records.
func (s *Server) applicabilityDiffers(ctx context.Context, p *Principal, kind, id, projectID string, snap json.RawMessage) (bool, error) {
	want, ok, err := s.snapshotApplicability(ctx, p, projectID, snap)
	if err != nil || !ok {
		return false, err
	}
	cur, err := s.Store.ApplicabilityOf(ctx, p.TenantID(), kind, id)
	if err != nil {
		return false, err
	}
	return !sameApplicability(cur, want), nil
}

// restoreApplicability gives an item the applicability a snapshot records
// (nothing when it records none or the same); reports whether it changed.
func (s *Server) restoreApplicability(ctx context.Context, p *Principal, kind, id, projectID string, snap json.RawMessage) (bool, error) {
	want, ok, err := s.snapshotApplicability(ctx, p, projectID, snap)
	if err != nil || !ok {
		return false, err
	}
	cur, err := s.Store.ApplicabilityOf(ctx, p.TenantID(), kind, id)
	if err != nil {
		return false, err
	}
	if sameApplicability(cur, want) {
		return false, nil
	}
	if want == nil {
		want = &model.Applicability{}
	}
	if _, err := s.Store.SetApplicability(ctx, p.TenantID(), projectID, kind, []string{id}, want, p.User.Email); err != nil {
		return false, err
	}
	names, _ := s.variantNameMap(ctx, p, projectID)
	s.applicabilityImpact(ctx, p, projectID, kind, id, snapshotKey(snap), applicabilityText(cur, names)+" → "+applicabilityText(want, names))
	return true, nil
}

// existingCategories filters category ids to those still in the project.
func (s *Server) existingCategories(ctx context.Context, p *Principal, projectID string, ids []string) ([]string, error) {
	cats, err := s.Store.ListCategories(ctx, p.TenantID(), projectID, "")
	if err != nil {
		return nil, err
	}
	ok := map[string]bool{}
	for _, c := range cats {
		ok[c.ID] = true
	}
	out := []string{}
	for _, id := range ids {
		if ok[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// ---------------------------------------------------------------- relations

func (s *Server) handleListRequirementRelations(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	if _, err := s.Store.GetRequirement(r.Context(), p.TenantID(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	rels, err := s.Store.ListRelations(r.Context(), p.TenantID(), "", id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(rels))
}

func (s *Server) handleCreateRelation(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		TargetID string `json:"target_id"`
		Type     string `json:"type"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	rel := &model.Relation{SourceID: r.PathValue("id"), TargetID: req.TargetID, Type: req.Type, CreatedBy: p.User.Email}
	if err := s.Store.CreateRelation(r.Context(), p.TenantID(), rel); err != nil {
		s.fail(w, r, err)
		return
	}
	rel.Direction = "outgoing"
	s.audit(r.Context(), p, "relation.create", "relation", rel.ID, rel.SourceKey+" "+rel.Type+" "+rel.TargetKey)
	if rel.Type == "refines" {
		// the parent's verification rolls up its children
		s.syncEvidence(r.Context(), p, rel.ProjectID, []string{rel.SourceID, rel.TargetID})
	}
	writeJSON(w, http.StatusCreated, rel)
}

func (s *Server) handleDeleteRelation(w http.ResponseWriter, r *http.Request, p *Principal) {
	rel, err := s.Store.GetRelation(r.Context(), p.TenantID(), r.PathValue("id"))
	if err == nil {
		err = s.Store.DeleteRelation(r.Context(), p.TenantID(), rel.ID)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "relation.delete", "relation", rel.ID, rel.SourceKey+" "+rel.Type+" "+rel.TargetKey)
	if rel.Type == "refines" {
		s.syncEvidence(r.Context(), p, rel.ProjectID, []string{rel.SourceID, rel.TargetID})
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListProjectRelations(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	rels, err := s.Store.ListRelations(r.Context(), p.TenantID(), pr.ID, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(rels))
}

// ---------------------------------------------------------------- baselines

func (s *Server) handleListBaselines(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	bs, err := s.Store.ListBaselines(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(bs))
}

// currentItems returns the current requirements and tests as baseline items.
func (s *Server) currentItems(r *http.Request, p *Principal, pr *model.Project, expr string) ([]model.BaselineItem, error) {
	pc, err := s.loadContext(r.Context(), p.TenantID(), pr)
	if err != nil {
		return nil, err
	}
	reqs, err := s.filterRequirements(r, p, pr, pc, reqExpr(expr), "", true)
	if err != nil {
		return nil, err
	}
	tests, err := s.filterTests(r, p, pr, pc, testExpr(expr), "", true)
	if err != nil {
		return nil, err
	}
	items := make([]model.BaselineItem, 0, len(reqs)+len(tests))
	// the snapshots record the applicability (the variants each item applies to)
	for i := range reqs {
		rq := &reqs[i]
		items = append(items, model.BaselineItem{EntityType: model.KindRequirement, EntityID: rq.ID, Key: rq.Key, Title: rq.Title, Version: rq.Version,
			Snapshot: store.WithApplicability(store.SnapshotRequirement(rq), rq.Applicability)})
	}
	for i := range tests {
		t := &tests[i]
		items = append(items, model.BaselineItem{EntityType: model.KindTest, EntityID: t.ID, Key: t.Key, Title: t.Title, Version: t.Version,
			Snapshot: store.WithApplicability(store.SnapshotTest(t), t.Applicability)})
	}
	return items, nil
}

func (s *Server) handleCreateBaseline(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Query       string `json:"query"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	items, err := s.currentItems(r, p, pr, req.Query)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	b := &model.Baseline{ProjectID: pr.ID, Name: req.Name, Description: req.Description, CreatedBy: p.User.Email}
	if err := s.Store.CreateBaseline(r.Context(), p.TenantID(), b, items); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "baseline.create", "baseline", b.ID, b.Name)
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) handleGetBaseline(w http.ResponseWriter, r *http.Request, p *Principal) {
	b, err := s.Store.GetBaseline(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) handleCompareBaseline(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	from, err := s.Store.GetBaseline(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	to := r.URL.Query().Get("to")
	var items []model.BaselineItem
	label := "current"
	if to == "" || to == "current" {
		pr, err := s.Store.GetProject(ctx, p.TenantID(), from.ProjectID)
		if err == nil {
			items, err = s.currentItems(r, p, pr, "")
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
	} else {
		other, err := s.Store.GetBaseline(ctx, p.TenantID(), to)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if other.ProjectID != from.ProjectID {
			s.fail(w, r, badRequest("baselines belong to different projects"))
			return
		}
		items, label = other.Items, other.Name
	}
	writeJSON(w, http.StatusOK, store.CompareItems(from.Name, label, from.Items, items))
}

func (s *Server) handleDeleteBaseline(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	if n, err := s.Store.SignoffCount(r.Context(), p.TenantID(), id); err == nil && n > 0 {
		s.fail(w, r, store.Conflictf("the baseline is signed (%d signatures) and cannot be deleted", n))
		return
	}
	if err := s.Store.DeleteBaseline(r.Context(), p.TenantID(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "baseline.delete", "baseline", id, "")
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- test executions

func (s *Server) handleTestExecutions(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	if _, err := s.Store.GetTest(r.Context(), p.TenantID(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	es, err := s.Store.ListTestExecutions(r.Context(), p.TenantID(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(es))
}

// ---------------------------------------------------------------- profile

// validatePreferences checks preference values.
func validatePreferences(pr model.Preferences, memberOf func(tenantID string) bool) error {
	if pr.Theme != "" && !model.Valid(model.Themes, pr.Theme) {
		return badRequest("theme must be one of %s", strings.Join(model.Themes, ", "))
	}
	if pr.Language != "" && !model.Valid(model.Languages, pr.Language) {
		return badRequest("language must be one of %s", strings.Join(model.Languages, ", "))
	}
	if pr.StartPage != "" && !model.Valid(model.StartPages, pr.StartPage) {
		return badRequest("start_page must be one of %s", strings.Join(model.StartPages, ", "))
	}
	if pr.DateFormat != "" && !model.Valid(model.DateFormats, pr.DateFormat) {
		return badRequest("date_format must be one of %s", strings.Join(model.DateFormats, ", "))
	}
	if pr.PageSize != 0 {
		ok := false
		for _, n := range model.PageSizes {
			ok = ok || n == pr.PageSize
		}
		if !ok {
			return badRequest("page_size must be 10, 25, 50 or 100")
		}
	}
	if pr.Timezone != "" {
		if _, err := time.LoadLocation(pr.Timezone); err != nil || len(pr.Timezone) > 64 {
			return badRequest("unknown time zone %q", pr.Timezone)
		}
	}
	if pr.EmailNotifications != "" && !model.Valid(model.EmailModes, pr.EmailNotifications) {
		return badRequest("email_notifications must be one of %s", strings.Join(model.EmailModes, ", "))
	}
	if pr.Autocomplete != "" && !model.Valid(model.AutocompleteModes, pr.Autocomplete) {
		return badRequest("autocomplete must be one of %s", strings.Join(model.AutocompleteModes, ", "))
	}
	if pr.DefaultTenant != "" && !memberOf(pr.DefaultTenant) {
		return badRequest("default_tenant_id must be a tenant you are a member of")
	}
	if len(pr.DefaultProject) > 64 {
		return badRequest("default_project_id is invalid")
	}
	return nil
}

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request, p *Principal) {
	if p.Token != nil {
		writeError(w, http.StatusForbidden, "forbidden", "the profile cannot be changed with an API token")
		return
	}
	body, err := readBody(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req struct {
		Avatar      *string                    `json:"avatar"`
		DisplayName *string                    `json:"display_name"`
		Preferences map[string]json.RawMessage `json:"preferences"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		s.fail(w, r, badRequest("invalid JSON: %v", err))
		return
	}
	ctx := r.Context()
	u := p.User
	if req.Avatar != nil {
		if !validPreset(*req.Avatar) {
			s.fail(w, r, badRequest(`avatar must be "" or "preset:<name>"; upload images with PUT /auth/me/avatar`))
			return
		}
		s.dropOldPicture(ctx, "", u.Avatar)
		u.Avatar = *req.Avatar
	}
	if req.DisplayName != nil {
		name := strings.TrimSpace(*req.DisplayName)
		if name == "" || len([]rune(name)) > 100 {
			s.fail(w, r, badRequest("display name must be 1-100 characters"))
			return
		}
		u.DisplayName = name
	}
	if req.Preferences != nil {
		// merge: present keys replace, null resets
		cur := map[string]json.RawMessage{}
		b, _ := json.Marshal(u.Preferences)
		_ = json.Unmarshal(b, &cur)
		for k, v := range req.Preferences {
			if string(v) == "null" {
				delete(cur, k)
			} else {
				cur[k] = v
			}
		}
		merged, _ := json.Marshal(cur)
		var np model.Preferences
		dec := json.NewDecoder(strings.NewReader(string(merged)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&np); err != nil {
			s.fail(w, r, badRequest("invalid preferences: %v", err))
			return
		}
		ms, err := s.Store.ListUserMemberships(ctx, u.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		memberOf := func(t string) bool {
			if u.SuperAdmin {
				_, err := s.Store.GetTenant(ctx, t)
				return err == nil
			}
			for _, m := range ms {
				if m.TenantID == t {
					return true
				}
			}
			return false
		}
		if err := validatePreferences(np, memberOf); err != nil {
			s.fail(w, r, err)
			return
		}
		u.Preferences = np
	}
	if err := s.Store.UpdateUser(ctx, u); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "profile.update", "user", u.ID, "")
	s.handleMe(w, r, p)
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request, p *Principal) {
	ss, err := s.Store.ListUserSessions(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for i := range ss {
		ss[i].Current = p.Session != nil && ss[i].ID == p.Session.ID
	}
	writeJSON(w, http.StatusOK, listOf(ss))
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request, p *Principal) {
	id := r.PathValue("id")
	if err := s.Store.DeleteUserSession(r.Context(), p.User.ID, id); err != nil {
		s.fail(w, r, err)
		return
	}
	if p.Session != nil && p.Session.ID == id {
		s.clearCookie(w, sessionCookie)
	}
	s.audit(r.Context(), p, "session.revoke", "user", p.User.ID, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRevokeOtherSessions(w http.ResponseWriter, r *http.Request, p *Principal) {
	keep := ""
	if p.Session != nil {
		keep = p.Session.ID
	}
	n, err := s.Store.DeleteOtherSessions(r.Context(), p.User.ID, keep)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "session.revoke_others", "user", p.User.ID, itoa(n))
	writeJSON(w, http.StatusOK, map[string]int{"revoked": n})
}

// restoreOnlyApplicability restores the applicability of an item whose
// content has not changed since a snapshot; reports whether it changed.
func (s *Server) restoreOnlyApplicability(ctx context.Context, p *Principal, kind, id, key, projectID string, snap json.RawMessage, what string) (bool, error) {
	differs, err := s.applicabilityDiffers(ctx, p, kind, id, projectID, snap)
	if err != nil || !differs {
		return false, err
	}
	s.ensureInitialRevision(ctx, p, kind, id)
	if _, err := s.restoreApplicability(ctx, p, kind, id, projectID, snap); err != nil {
		return false, err
	}
	s.record(ctx, p, kind, id, "restore")
	s.audit(ctx, p, kind+".restore", kind, id, key+" applicability to "+what)
	return true, nil
}

// handleRestoreFromBaseline: POST /baselines/{id}/restore
// {"items": [{"entity_type", "entity_id"}]} gives each item the content it
// had in the baseline. Items not in the baseline, deleted, unchanged since,
// or that the caller may not change are skipped with a reason.
func (s *Server) handleRestoreFromBaseline(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	b, err := s.Store.GetBaseline(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req struct {
		Items []struct {
			EntityType string `json:"entity_type"`
			EntityID   string `json:"entity_id"`
		} `json:"items"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(req.Items) == 0 {
		s.fail(w, r, badRequest("choose at least one item"))
		return
	}
	if len(req.Items) > maxBulkQuality {
		s.fail(w, r, badRequest("at most %d items at a time", maxBulkQuality))
		return
	}
	snap := map[string]model.BaselineItem{}
	for _, it := range b.Items {
		snap[it.EntityType+":"+it.EntityID] = it
	}
	type item struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		Key        string `json:"key"`
		Reason     string `json:"reason,omitempty"`
	}
	restored, skipped := []item{}, []item{}
	what := "baseline " + b.Name
	for _, x := range req.Items {
		bi, ok := snap[x.EntityType+":"+x.EntityID]
		if !ok {
			skipped = append(skipped, item{x.EntityType, x.EntityID, "", "not_in_baseline"})
			continue
		}
		out := item{bi.EntityType, bi.EntityID, bi.Key, ""}
		if !p.Can(writePerm(bi.EntityType)) {
			out.Reason = "not_allowed"
			skipped = append(skipped, out)
			continue
		}
		switch bi.EntityType {
		case model.KindRequirement:
			cur, err := s.Store.GetRequirement(ctx, p.TenantID(), bi.EntityID)
			if err != nil {
				out.Reason = "deleted"
				skipped = append(skipped, out)
				continue
			}
			if cur.Version == bi.Version {
				// the content is the same: only the applicability may differ
				done, err := s.restoreOnlyApplicability(ctx, p, model.KindRequirement, cur.ID, cur.Key, cur.ProjectID, bi.Snapshot, what)
				if err != nil {
					s.fail(w, r, err)
					return
				}
				if !done {
					out.Reason = "unchanged"
					skipped = append(skipped, out)
					continue
				}
				restored = append(restored, out)
				continue
			}
			var old model.Requirement
			if err := json.Unmarshal(bi.Snapshot, &old); err != nil {
				s.fail(w, r, err)
				return
			}
			if isReviewDecision(model.KindRequirement, cur.Status, old.Status) && !p.Can(rbac.RequirementRev) {
				out.Reason = "needs_review"
				skipped = append(skipped, out)
				continue
			}
			if err := s.restoreRequirementTo(ctx, p, cur, &old, bi.Snapshot, what); err != nil {
				s.fail(w, r, err)
				return
			}
		case model.KindTest:
			cur, err := s.Store.GetTest(ctx, p.TenantID(), bi.EntityID)
			if err != nil {
				out.Reason = "deleted"
				skipped = append(skipped, out)
				continue
			}
			if cur.Version == bi.Version {
				done, err := s.restoreOnlyApplicability(ctx, p, model.KindTest, cur.ID, cur.Key, cur.ProjectID, bi.Snapshot, what)
				if err != nil {
					s.fail(w, r, err)
					return
				}
				if !done {
					out.Reason = "unchanged"
					skipped = append(skipped, out)
					continue
				}
				restored = append(restored, out)
				continue
			}
			var old model.TestCase
			if err := json.Unmarshal(bi.Snapshot, &old); err != nil {
				s.fail(w, r, err)
				return
			}
			if isReviewDecision(model.KindTest, cur.Status, old.Status) && !p.Can(rbac.TestReview) {
				out.Reason = "needs_review"
				skipped = append(skipped, out)
				continue
			}
			if err := s.restoreTestTo(ctx, p, cur, &old, bi.Snapshot, what); err != nil {
				s.fail(w, r, err)
				return
			}
		default:
			continue
		}
		restored = append(restored, out)
	}
	s.audit(ctx, p, "baseline.restore", "baseline", b.ID, fmt.Sprintf("%s: %d restored, %d skipped", b.Name, len(restored), len(skipped)))
	writeJSON(w, http.StatusOK, map[string]any{"restored": restored, "skipped": skipped})
}
