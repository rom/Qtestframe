package api

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/mention"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Discussions: threaded comments on requirements, tests, defects, risks,
// test runs and review requests (see docs/api.md "Discussions").
//
// Reading a discussion needs the read permission of the item; writing,
// replying, resolving, reopening and reacting need comment.create as well.
// The author edits and removes their own comments; moderators (project.update
// in the project, or member.manage) remove anyone's. Removed comments stay
// in the thread, shown as removed.

// discussionResources maps the resource of a route to the entity type and
// the permission that reads it ("" for review requests: see discussionTarget).
var discussionResources = []struct {
	path, kind string
	perm       rbac.Permission
}{
	{"requirements", model.KindRequirement, rbac.RequirementRd},
	{"tests", model.KindTest, rbac.TestRead},
	{"defects", model.KindDefect, rbac.ProjectRead},
	{"risks", model.KindRisk, rbac.RequirementRd},
	{"runs", model.DiscussionRun, rbac.RunRead},
	{"reviews", model.DiscussionReview, ""},
}

func (s *Server) routesDiscussions(h routeFunc) {
	for _, d := range discussionResources {
		perms := []string{}
		if d.perm != "" {
			perms = append(perms, string(d.perm))
		}
		h("GET /api/v1/"+d.path+"/{id}/comments", s.handleListComments(d.kind), perms...)
		h("POST /api/v1/"+d.path+"/{id}/comments", s.handleAddComment(d.kind), append(perms, string(rbac.CommentCreate))...)
	}
	h("PATCH /api/v1/comments/{id}", s.handleUpdateComment, string(rbac.CommentCreate))
	h("DELETE /api/v1/comments/{id}", s.handleDeleteComment)
	h("POST /api/v1/comments/{id}/resolve", s.handleResolveThread(true), string(rbac.CommentCreate))
	h("POST /api/v1/comments/{id}/reopen", s.handleResolveThread(false), string(rbac.CommentCreate))
	h("POST /api/v1/comments/{id}/reactions", s.handleReactComment, string(rbac.CommentCreate))
	h("GET /api/v1/projects/{pid}/discussions", s.handleProjectDiscussions, "project.read")
	h("GET /api/v1/discussions", s.handleMyDiscussions)
}

// discussionReadPerm is the permission that reads the discussions of an
// entity type ("" for review requests).
func discussionReadPerm(kind string) rbac.Permission {
	for _, d := range discussionResources {
		if d.kind == kind {
			return d.perm
		}
	}
	return rbac.RequirementRd
}

// discussionTarget is the item a discussion is about.
type discussionTarget struct {
	Kind, ID, ProjectID, Key, Title string
	// Owners are the people responsible for the item (told about new threads).
	Owners []string
}

// label is "KEY Title" (or the title alone).
func (t *discussionTarget) label() string {
	return strings.TrimSpace(t.Key + " " + t.Title)
}

// discussionTarget loads the item of a discussion and checks that the
// caller may read it (a review request: its reviewer, its requester, or
// the read permission of the reviewed item).
func (s *Server) discussionTarget(ctx context.Context, p *Principal, kind, id string) (*discussionTarget, error) {
	t := &discussionTarget{Kind: kind, ID: id}
	tid := p.TenantID()
	switch kind {
	case model.KindRequirement:
		q, err := s.Store.GetRequirement(ctx, tid, id)
		if err != nil {
			return nil, err
		}
		t.ProjectID, t.Key, t.Title, t.Owners = q.ProjectID, q.Key, q.Title, []string{q.Owner}
	case model.KindTest:
		x, err := s.Store.GetTest(ctx, tid, id)
		if err != nil {
			return nil, err
		}
		t.ProjectID, t.Key, t.Title = x.ProjectID, x.Key, x.Title
	case model.KindDefect:
		d, err := s.Store.GetDefect(ctx, tid, id)
		if err != nil {
			return nil, err
		}
		t.ProjectID, t.Key, t.Title, t.Owners = d.ProjectID, d.Key, d.Title, []string{d.Assignee}
	case model.KindRisk:
		k, err := s.Store.GetRisk(ctx, tid, id)
		if err != nil {
			return nil, err
		}
		t.ProjectID, t.Key, t.Title, t.Owners = k.ProjectID, k.Key, k.Title, []string{k.Owner}
	case model.DiscussionRun:
		run, err := s.Store.GetRun(ctx, tid, id)
		if err != nil {
			return nil, err
		}
		t.ProjectID, t.Title = run.ProjectID, run.Name
	case model.DiscussionReview:
		v, err := s.Store.GetReviewRequest(ctx, tid, id)
		if err != nil {
			return nil, err
		}
		if v.ReviewerID != p.User.ID && v.RequestedBy != p.User.Email && !p.Can(readPermOf(v.EntityType)) {
			return nil, fmt.Errorf("review request: %w", store.ErrNotFound)
		}
		t.ProjectID, t.Key, t.Title, t.Owners = v.ProjectID, v.EntityKey, v.EntityTitle, []string{v.ReviewerID, v.RequestedBy}
		return t, nil
	default:
		return nil, fmt.Errorf("item: %w", store.ErrNotFound)
	}
	if perm := discussionReadPerm(kind); !p.Can(perm) {
		return nil, fmt.Errorf("%w: permission %s required", errForbidden, perm)
	}
	return t, nil
}

// mentionedUsers resolves the @handles of a text to user ids of members.
func (s *Server) mentionedUsers(ctx context.Context, p *Principal, text string) []string {
	hs := mention.Extract(text)
	if len(hs) == 0 {
		return []string{}
	}
	index := s.peopleIndex(ctx, p)
	out := []string{}
	for _, h := range hs {
		if id, ok := index[h]; ok && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func (s *Server) handleListComments(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		t, err := s.discussionTarget(r.Context(), p, kind, r.PathValue("id"))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		cs, err := s.Store.ListComments(r.Context(), p.TenantID(), kind, t.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, listOf(cs))
	}
}

// handleAddComment: {"body", "parent_id"?} starts a thread or replies to one.
func (s *Server) handleAddComment(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		ctx := r.Context()
		t, err := s.discussionTarget(ctx, p, kind, r.PathValue("id"))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		var req struct {
			Body     string `json:"body"`
			ParentID string `json:"parent_id"`
		}
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
		c := &model.Comment{ProjectID: t.ProjectID, EntityType: kind, EntityID: t.ID, ParentID: strings.TrimSpace(req.ParentID),
			UserID: p.User.ID, Body: req.Body, Mentions: s.mentionedUsers(ctx, p, req.Body)}
		if err := s.Store.AddComment(ctx, p.TenantID(), c); err != nil {
			s.fail(w, r, err)
			return
		}
		c.UserEmail, c.UserName, c.UserAvatar = p.User.Email, p.User.DisplayName, p.User.Avatar
		s.audit(ctx, p, "comment.create", kind, t.ID, "comment "+c.ID)
		s.afterComment(r, p, t, c, c.Mentions)
		writeJSON(w, http.StatusCreated, c)
	}
}

// comment loads {id} and checks that the caller may read its item.
func (s *Server) comment(w http.ResponseWriter, r *http.Request, p *Principal) (*model.Comment, *discussionTarget, bool) {
	c, err := s.Store.GetComment(r.Context(), p.TenantID(), r.PathValue("id"))
	if err == nil {
		var t *discussionTarget
		if t, err = s.discussionTarget(r.Context(), p, c.EntityType, c.EntityID); err == nil {
			return c, t, true
		}
	}
	s.fail(w, r, err)
	return nil, nil, false
}

// moderator reports whether the caller may remove other people's comments.
func moderator(p *Principal) bool {
	return p.Can(rbac.ProjectUpdate) || p.Can(rbac.MemberManage)
}

func (s *Server) handleUpdateComment(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, t, ok := s.comment(w, r, p)
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
	ctx := r.Context()
	before := c.Mentions
	c.Body, c.Mentions = req.Body, s.mentionedUsers(ctx, p, req.Body)
	if err := s.Store.UpdateComment(ctx, p.TenantID(), c); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "comment.update", c.EntityType, c.EntityID, "comment "+c.ID)
	var added []string
	for _, u := range c.Mentions {
		if !slices.Contains(before, u) {
			added = append(added, u)
		}
	}
	if len(added) > 0 {
		s.notifyEvent(r, p, t.ProjectID, "comment.mentioned", s.commentEvent(t, c, added))
		s.notifyCommentMentions(ctx, p, t, c, added)
	}
	writeJSON(w, http.StatusOK, c)
}

// handleDeleteComment removes a comment (it stays in the thread as removed).
func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, _, ok := s.comment(w, r, p)
	if !ok {
		return
	}
	own := c.UserID == p.User.ID
	if !own && !moderator(p) {
		writeError(w, http.StatusForbidden, "forbidden", "only the author or a moderator can delete a comment")
		return
	}
	if own && !p.Can(rbac.CommentCreate) && !moderator(p) {
		writeError(w, http.StatusForbidden, "forbidden", "permission comment.create required")
		return
	}
	if err := s.Store.RemoveComment(r.Context(), p.TenantID(), c.ID, p.User.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	details := "comment " + c.ID
	if !own {
		details += " by " + c.UserEmail + " (moderated)"
	}
	s.audit(r.Context(), p, "comment.delete", c.EntityType, c.EntityID, details)
	w.WriteHeader(http.StatusNoContent)
}

// handleResolveThread resolves or reopens the thread of {id} (any comment
// of the thread) and answers the thread's first comment.
func (s *Server) handleResolveThread(resolve bool) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		c, t, ok := s.comment(w, r, p)
		if !ok {
			return
		}
		ctx := r.Context()
		root := c.ID
		if c.ParentID != "" {
			root = c.ParentID
		}
		if err := s.Store.ResolveThread(ctx, p.TenantID(), root, p.User.ID, resolve); err != nil {
			s.fail(w, r, err)
			return
		}
		rc, err := s.Store.GetComment(ctx, p.TenantID(), root)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		action := "comment.reopen"
		if resolve {
			action = "comment.resolve"
		}
		s.audit(ctx, p, action, rc.EntityType, rc.EntityID, "thread "+root)
		if resolve {
			s.notifyEvent(r, p, t.ProjectID, "comment.resolved", s.commentEvent(t, rc, nil))
			if thread, err := s.Store.ThreadComments(ctx, p.TenantID(), root); err == nil {
				var people []string
				for _, x := range thread {
					if !x.Deleted {
						people = append(people, x.UserID)
					}
				}
				s.notifyUsers(ctx, p, s.recipients(ctx, p, people...), model.Notification{ProjectID: t.ProjectID, Event: "comment.resolved",
					Title: t.label(), Body: excerpt(rc.Body, 300), EntityType: t.Kind, EntityID: t.ID})
			}
		}
		writeJSON(w, http.StatusOK, rc)
	}
}

// handleReactComment: {"reaction"} adds the caller's reaction or takes it back.
func (s *Server) handleReactComment(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, _, ok := s.comment(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Reaction string `json:"reaction"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	m, err := s.Store.ToggleReaction(r.Context(), p.TenantID(), c.ID, p.User.ID, req.Reaction)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reactions": m})
}

// ---------------------------------------------------------------- events

// commentEvent is the data of a comment webhook event.
func (s *Server) commentEvent(t *discussionTarget, c *model.Comment, mentioned []string) map[string]any {
	d := map[string]any{"kind": t.Kind, "id": t.ID, "key": t.Key, "title": t.Title, "comment": c}
	if mentioned != nil {
		d["mentioned"] = mentioned
	}
	return d
}

// afterComment sends the webhooks of a new comment and tells the people
// mentioned in it, the others in its thread (a reply) or the item's owners
// (a new thread). Everybody is told once.
func (s *Server) afterComment(r *http.Request, p *Principal, t *discussionTarget, c *model.Comment, mentioned []string) {
	ctx := r.Context()
	s.notifyEvent(r, p, t.ProjectID, "comment.created", s.commentEvent(t, c, nil))
	if len(mentioned) > 0 {
		s.notifyEvent(r, p, t.ProjectID, "comment.mentioned", s.commentEvent(t, c, mentioned))
	}
	told := s.notifyCommentMentions(ctx, p, t, c, mentioned)
	var people []string
	if c.ParentID != "" {
		if thread, err := s.Store.ThreadComments(ctx, p.TenantID(), c.ParentID); err == nil {
			for _, x := range thread {
				if !x.Deleted {
					people = append(people, x.UserID)
				}
			}
		}
	} else {
		people = t.Owners
	}
	var rest []string
	for _, u := range s.recipients(ctx, p, people...) {
		if !slices.Contains(told, u) {
			rest = append(rest, u)
		}
	}
	s.notifyUsers(ctx, p, rest, model.Notification{ProjectID: t.ProjectID, Event: "comment.created",
		Title: t.label(), Body: excerpt(c.Body, 300), EntityType: t.Kind, EntityID: t.ID})
}

// notifyCommentMentions tells the mentioned users (not the author) and
// returns who was told.
func (s *Server) notifyCommentMentions(ctx context.Context, p *Principal, t *discussionTarget, c *model.Comment, users []string) []string {
	var to []string
	for _, u := range users {
		if u != p.User.ID {
			to = append(to, u)
		}
	}
	s.notifyUsers(ctx, p, to, model.Notification{ProjectID: t.ProjectID, Event: "mention",
		Title: t.label(), Body: excerpt(c.Body, 300), EntityType: t.Kind, EntityID: t.ID})
	return to
}

// ---------------------------------------------------------------- lists

// discussionFilter reads ?status= (open, resolved, all), ?filter= (mine:
// threads the caller wrote in; mentioned: threads that mention the
// caller), ?type= (entity type), ?q= (text) and ?limit=.
func discussionFilter(r *http.Request, p *Principal) (store.DiscussionFilter, error) {
	q := r.URL.Query()
	f := store.DiscussionFilter{Status: q.Get("status"), EntityType: q.Get("type"), Text: strings.TrimSpace(q.Get("q"))}
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	if f.Status != "" && f.Status != store.ThreadsOpen && f.Status != store.ThreadsResolved && f.Status != store.ThreadsAll {
		return f, badRequest("status must be open, resolved or all")
	}
	if f.EntityType != "" && !slices.Contains(model.DiscussionKinds, f.EntityType) {
		return f, badRequest("type must be one of %s", strings.Join(model.DiscussionKinds, ", "))
	}
	switch q.Get("filter") {
	case "":
	case "mine":
		f.Participant = p.User.ID
	case "mentioned":
		f.Mentioned = p.User.ID
	default:
		return f, badRequest("filter must be mine or mentioned")
	}
	return f, nil
}

// visibleThreads keeps the threads whose item the caller may read.
func (s *Server) visibleThreads(ctx context.Context, p *Principal, vis map[string]*model.ProjectAccessInfo, list []model.DiscussionThread) []model.DiscussionThread {
	out := []model.DiscussionThread{}
	reviews := map[string]bool{}
	for _, t := range list {
		perm := discussionReadPerm(t.Root.EntityType)
		if t.Root.EntityType == model.DiscussionReview {
			ok, seen := reviews[t.Root.EntityID]
			if !seen {
				if v, err := s.Store.GetReviewRequest(ctx, p.TenantID(), t.Root.EntityID); err == nil {
					ok = v.ReviewerID == p.User.ID || v.RequestedBy == p.User.Email || visibleIn(vis, p, v.ProjectID, readPermOf(v.EntityType))
				}
				reviews[t.Root.EntityID] = ok
			}
			if ok && vis[t.Root.ProjectID] != nil {
				out = append(out, t)
			}
			continue
		}
		if visibleIn(vis, p, t.Root.ProjectID, perm) {
			out = append(out, t)
		}
	}
	return out
}

// handleProjectDiscussions lists a project's threads.
func (s *Server) handleProjectDiscussions(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	f, err := discussionFilter(r, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f.ProjectIDs = []string{pr.ID}
	s.writeThreads(w, r, p, f)
}

// handleMyDiscussions lists threads across the projects the caller sees
// (default: the open threads that mention the caller).
func (s *Server) handleMyDiscussions(w http.ResponseWriter, r *http.Request, p *Principal) {
	if r.URL.Query().Get("filter") == "" {
		q := r.URL.Query()
		q.Set("filter", "mentioned")
		r.URL.RawQuery = q.Encode()
	}
	f, err := discussionFilter(r, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	vis, err := s.visibleProjects(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f.ProjectIDs = []string{}
	for id := range vis {
		f.ProjectIDs = append(f.ProjectIDs, id)
	}
	slices.Sort(f.ProjectIDs)
	s.writeThreads(w, r, p, f)
}

func (s *Server) writeThreads(w http.ResponseWriter, r *http.Request, p *Principal, f store.DiscussionFilter) {
	list, err := s.Store.ListDiscussionThreads(r.Context(), p.TenantID(), f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	vis, err := s.visibleProjects(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(s.visibleThreads(r.Context(), p, vis, list)))
}

// userHandles maps the user ids of the tenant's members to their @handles.
func (s *Server) userHandles(ctx context.Context, tenantID string) map[string]string {
	ms, err := s.Store.ListTenantMembers(ctx, tenantID)
	if err != nil {
		return map[string]string{}
	}
	users := make([]mention.User, 0, len(ms))
	for _, m := range ms {
		users = append(users, mention.User{ID: m.UserID, Email: m.Email, Name: m.Name})
	}
	out := map[string]string{}
	for _, d := range mention.Directory(users, nil) {
		if d.UserID != "" {
			out[d.UserID] = d.Handle
		}
	}
	return out
}
