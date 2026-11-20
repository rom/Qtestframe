package api

import (
	"context"
	"encoding/json"
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
	s.ensureInitialRevision(ctx, p, model.KindRequirement, cur.ID)
	before := store.SnapshotRequirement(cur)
	cur.Title, cur.Description, cur.Obligation, cur.Type = old.Title, old.Description, old.Obligation, old.Type
	cur.Priority, cur.Status, cur.Source, cur.Rationale = old.Priority, old.Status, old.Source, old.Rationale
	cur.Owner, cur.Tags, cur.Criteria = old.Owner, old.Tags, old.Criteria
	if cur.CategoryIDs, err = s.existingCategories(ctx, p, cur.ProjectID, old.CategoryIDs); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.UpdateRequirement(ctx, p.TenantID(), cur, true); err != nil {
		s.fail(w, r, err)
		return
	}
	s.recordRequirement(ctx, p, cur.ID, "restore")
	s.afterContentChange(ctx, p, model.KindRequirement, cur.ID, cur.Key, cur.ProjectID, before)
	s.audit(ctx, p, "requirement.restore", "requirement", cur.ID, cur.Key+" to version "+itoa(rev.Version))
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
	s.ensureInitialRevision(ctx, p, model.KindTest, cur.ID)
	before := store.SnapshotTest(cur)
	cur.Title, cur.Description, cur.Preconditions, cur.ExecutionType = old.Title, old.Description, old.Preconditions, old.ExecutionType
	cur.ScriptLanguage, cur.Script, cur.ExpectedResult = old.ScriptLanguage, old.Script, old.ExpectedResult
	cur.Priority, cur.Status, cur.EstimatedMin, cur.Tags = old.Priority, old.Status, old.EstimatedMin, old.Tags
	if cur.CategoryIDs, err = s.existingCategories(ctx, p, cur.ProjectID, old.CategoryIDs); err != nil {
		s.fail(w, r, err)
		return
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
		s.fail(w, r, err)
		return
	}
	s.recordTest(ctx, p, cur.ID, "restore")
	s.afterContentChange(ctx, p, model.KindTest, cur.ID, cur.Key, cur.ProjectID, before)
	s.audit(ctx, p, "test.restore", "test", cur.ID, cur.Key+" to version "+itoa(rev.Version))
	out, err := s.decoratedTest(r, p, cur.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
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
	for i := range reqs {
		rq := &reqs[i]
		items = append(items, model.BaselineItem{EntityType: model.KindRequirement, EntityID: rq.ID, Key: rq.Key, Title: rq.Title, Version: rq.Version, Snapshot: store.SnapshotRequirement(rq)})
	}
	for i := range tests {
		t := &tests[i]
		items = append(items, model.BaselineItem{EntityType: model.KindTest, EntityID: t.ID, Key: t.Key, Title: t.Title, Version: t.Version, Snapshot: store.SnapshotTest(t)})
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

// ---------------------------------------------------------------- comments

func readPerm(kind string) rbac.Permission {
	if kind == model.KindTest {
		return rbac.TestRead
	}
	return rbac.RequirementRd
}

func (s *Server) handleListComments(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		id := r.PathValue("id")
		if err := s.itemExists(r.Context(), p, kind, id); err != nil {
			s.fail(w, r, err)
			return
		}
		cs, err := s.Store.ListComments(r.Context(), p.TenantID(), kind, id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, listOf(cs))
	}
}

func (s *Server) handleAddComment(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		id := r.PathValue("id")
		if err := s.itemExists(r.Context(), p, kind, id); err != nil {
			s.fail(w, r, err)
			return
		}
		var req struct {
			Body string `json:"body"`
		}
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
		c := &model.Comment{EntityType: kind, EntityID: id, UserID: p.User.ID, Body: req.Body}
		if err := s.Store.AddComment(r.Context(), p.TenantID(), c); err != nil {
			s.fail(w, r, err)
			return
		}
		c.UserEmail, c.UserName = p.User.Email, p.User.DisplayName
		s.audit(r.Context(), p, "comment.create", kind, id, "")
		s.afterComment(r, p, kind, id, c)
		writeJSON(w, http.StatusCreated, c)
	}
}

// comment loads {id} and checks that the caller may see its entity.
func (s *Server) comment(w http.ResponseWriter, r *http.Request, p *Principal) (*model.Comment, bool) {
	c, err := s.Store.GetComment(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	if !p.Can(readPerm(c.EntityType)) {
		writeError(w, http.StatusForbidden, "forbidden", "permission "+string(readPerm(c.EntityType))+" required")
		return nil, false
	}
	return c, true
}

func (s *Server) handleUpdateComment(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, ok := s.comment(w, r, p)
	if !ok {
		return
	}
	if c.UserID != p.User.ID {
		writeError(w, http.StatusForbidden, "forbidden", "only the author can edit a comment")
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	c.Body = req.Body
	if err := s.Store.UpdateComment(r.Context(), p.TenantID(), c); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, ok := s.comment(w, r, p)
	if !ok {
		return
	}
	if c.UserID != p.User.ID && !p.Can(rbac.MemberManage) {
		writeError(w, http.StatusForbidden, "forbidden", "only the author or an administrator can delete a comment")
		return
	}
	if err := s.Store.DeleteComment(r.Context(), p.TenantID(), c.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "comment.delete", c.EntityType, c.EntityID, "")
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
