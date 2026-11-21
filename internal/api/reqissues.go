package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/tracker"
)

// Issues created from requirements in the project's tracker (GitLab,
// GitHub or Jira: the same connection as for defects, see defects.go).
//
// The issue is remembered as an external item link of the requirement
// (target_type url) that also holds the issue's key and state. A
// requirement has at most one issue in the configured tracker: creating a
// second one is refused (409 issue_exists) and the caller is offered to
// open or update the first. Updating pushes the requirement's current
// title and text into the issue (in Markdown, only the part between the
// Qtestframe markers is replaced); syncing reads the issue's state back
// and, when the tracker settings ask for it, comments on the issue once
// all of the requirement's tests passed.
//
// Reading needs requirement.read; creating, updating and syncing change
// the requirement's links and need requirement.write. Every create, update
// and sync is written to the audit log; the tracker token never leaves the
// server.

func (s *Server) routesRequirementIssues(h routeFunc) {
	h("GET /api/v1/requirements/{id}/issue", s.handleGetRequirementIssue, "requirement.read")
	h("POST /api/v1/requirements/{id}/issue", s.handleCreateRequirementIssue, "requirement.write")
	h("PUT /api/v1/requirements/{id}/issue", s.handleUpdateRequirementIssue, "requirement.write")
	h("POST /api/v1/requirements/{id}/issue/sync", s.handleSyncRequirementIssue, "requirement.write")
	h("POST /api/v1/projects/{pid}/requirements/issues", s.handleBulkRequirementIssues, "requirement.write")
	h("POST /api/v1/projects/{pid}/requirements/issues/sync", s.handleSyncRequirementIssues, "requirement.write")
}

// maxBulkIssues bounds the requirements of one bulk request (each is a
// request to the tracker).
const maxBulkIssues = 100

// trackerNames name the trackers in link labels and comments.
var trackerNames = map[string]string{"gitlab": "GitLab", "github": "GitHub", "jira": "Jira"}

// issueLinkOf returns the requirement's link to an issue of the configured
// tracker (same type, and same repository or project).
func issueLinkOf(links []model.ItemLink, reqID string, t *model.DefectTracker) *model.ItemLink {
	for i := range links {
		l := &links[i]
		if l.SourceID == reqID && l.Issue != nil && tracker.Owns(t, l.Issue.Tracker, l.Issue.Key) {
			return l
		}
	}
	return nil
}

// reqIssueCtx is what building and syncing issues of a project needs.
type reqIssueCtx struct {
	s   *Server
	p   *Principal
	pr  *model.Project
	t   *model.DefectTracker
	c   tracker.Client
	ctx context.Context
	// last: test id → latest result; tests: test id → test
	last  map[string]string
	tests map[string]*model.TestCase
}

func (s *Server) newReqIssueCtx(r *http.Request, p *Principal, projectID string) (*reqIssueCtx, error) {
	pr, err := s.Store.GetProject(r.Context(), p.TenantID(), projectID)
	if err != nil {
		return nil, err
	}
	c, t, err := s.trackerClient(r, p, projectID)
	if err != nil {
		return nil, err
	}
	last, err := s.Store.LastResults(r.Context(), p.TenantID(), projectID)
	if err != nil {
		return nil, err
	}
	return &reqIssueCtx{s: s, p: p, pr: pr, t: t, c: c, ctx: r.Context(), last: last, tests: map[string]*model.TestCase{}}, nil
}

// linkedTests returns the requirement's tests in key order.
func (x *reqIssueCtx) linkedTests(req *model.Requirement) ([]*model.TestCase, error) {
	links, err := x.s.Store.ListLinks(x.ctx, x.p.TenantID(), req.ProjectID, req.ID, "")
	if err != nil {
		return nil, err
	}
	var out []*model.TestCase
	seen := map[string]bool{}
	for _, l := range links {
		if seen[l.TestCaseID] {
			continue
		}
		seen[l.TestCaseID] = true
		tc, ok := x.tests[l.TestCaseID]
		if !ok {
			if tc, err = x.s.Store.GetTest(x.ctx, x.p.TenantID(), l.TestCaseID); err != nil {
				if errors.Is(err, store.ErrNotFound) {
					continue
				}
				return nil, err
			}
			x.tests[l.TestCaseID] = tc
		}
		out = append(out, tc)
	}
	return out, nil
}

func (x *reqIssueCtx) coverage(tests []*model.TestCase) string {
	ids := make([]string, len(tests))
	for i, tc := range tests {
		ids[i] = tc.ID
	}
	return store.Coverage(ids, x.last)
}

var resultIcons = map[string]string{"passed": "✅", "failed": "❌", "blocked": "⛔", "skipped": "⏭️"}

// issueTitle is "[KEY] title" (GitLab allows 255 characters).
func issueTitle(req *model.Requirement) string {
	t := "[" + req.Key + "] " + strings.Join(strings.Fields(req.Title), " ")
	if utf8.RuneCountInString(t) > 255 {
		t = string([]rune(t)[:254]) + "…"
	}
	return t
}

// requirementLabels maps a requirement's fields to labels with the
// tracker's templates (model.DefaultRequirementLabels when empty).
func requirementLabels(tmpl string, req *model.Requirement) []string {
	tmpl = strings.TrimSpace(tmpl)
	if tmpl == "" {
		tmpl = model.DefaultRequirementLabels
	}
	if strings.EqualFold(tmpl, "none") {
		return nil
	}
	clean := func(v string) string { return strings.TrimSpace(strings.ReplaceAll(v, ",", " ")) }
	vals := map[string]string{"{type}": clean(req.Type), "{priority}": clean(req.Priority), "{obligation}": clean(req.Obligation), "{status}": clean(req.Status)}
	expand := func(item string) string {
		for ph, v := range vals {
			if strings.Contains(item, ph) {
				if v == "" {
					return ""
				}
				item = strings.ReplaceAll(item, ph, v)
			}
		}
		return strings.TrimSpace(item)
	}
	var out []string
	seen := map[string]bool{}
	add := func(l string) {
		if l == "" || utf8.RuneCountInString(l) > 255 || seen[strings.ToLower(l)] {
			return
		}
		seen[strings.ToLower(l)] = true
		out = append(out, l)
	}
	for _, item := range strings.Split(tmpl, ",") {
		item = strings.TrimSpace(item)
		if strings.Contains(item, "{tags}") {
			for _, tag := range req.Tags {
				if tag = clean(tag); tag != "" {
					add(expand(strings.ReplaceAll(item, "{tags}", tag)))
				}
			}
			continue
		}
		add(expand(item))
	}
	return out
}

// draft builds the issue of a requirement: title, description in the
// tracker's markup (wrapped in the Qtestframe markers) and labels.
func (x *reqIssueCtx) draft(req *model.Requirement) (tracker.NewIssue, []*model.TestCase, error) {
	tests, err := x.linkedTests(req)
	if err != nil {
		return tracker.NewIssue{}, nil, err
	}
	d := tracker.NewDoc(x.c.Markup())
	link := x.s.itemLink(x.pr)
	ref := func(key string) string {
		if link == nil {
			return key
		}
		return d.Link(key, link(key))
	}
	d.Note("Requirement " + ref(req.Key) + " of project " + x.pr.Name + " in Qtestframe. This description is updated from Qtestframe; text added outside the Qtestframe part is kept.")
	d.Heading("Requirement")
	if strings.TrimSpace(req.Description) == "" {
		d.Text("(no text)")
	} else {
		d.Text(req.Description)
	}
	if strings.TrimSpace(req.Rationale) != "" {
		d.Heading("Rationale")
		d.Text(req.Rationale)
	}
	if len(req.Criteria) > 0 {
		d.Heading("Acceptance criteria")
		var rows [][]string
		for i, c := range req.Criteria {
			rows = append(rows, []string{strconv.Itoa(i + 1), d.Cell(c.Description), d.Cell(c.Method), d.Cell(c.Threshold)})
		}
		d.Table([]string{"#", "Criterion", "Method", "Threshold"}, rows)
	}
	d.Heading("Details")
	d.Fields([][2]string{
		{"Key", ref(req.Key)},
		{"Type", req.Type},
		{"Priority", req.Priority},
		{"Obligation", req.Obligation},
		{"Status", req.Status},
		{"Owner", req.Owner},
		{"Source", req.Source},
		{"Tags", strings.Join(req.Tags, ", ")},
		{"Version", strconv.Itoa(req.Version)},
	})
	d.Heading("Linked tests")
	if len(tests) == 0 {
		d.Text("No tests are linked to the requirement yet.")
	} else {
		var rows [][]string
		passed := 0
		for _, tc := range tests {
			res := x.last[tc.ID]
			if res == "" {
				res = "not run"
			}
			if res == "passed" {
				passed++
			}
			if ic := resultIcons[res]; ic != "" {
				res = ic + " " + res
			}
			rows = append(rows, []string{ref(tc.Key), d.Cell(tc.Title), d.Cell(res)})
		}
		d.Table([]string{"Test", "Title", "Latest result"}, rows)
		d.Text(fmt.Sprintf("%s of %d tests passed (coverage: %s).", strconv.Itoa(passed), len(tests), strings.ReplaceAll(x.coverage(tests), "_", " ")))
	}
	in := tracker.NewIssue{
		Title:       issueTitle(req),
		Description: tracker.Managed(x.c.Markup(), d.String()),
		Priority:    req.Priority,
		Labels:      requirementLabels(x.t.RequirementLabels, req),
		IssueType:   x.t.RequirementIssueType,
	}
	return in, tests, nil
}

// applyIssueLink copies what was read of an issue into the link.
func (x *reqIssueCtx) applyIssueLink(l *model.ItemLink, is *tracker.Issue) {
	now := x.s.Store.Now()
	state := "open"
	if is.Closed {
		state = "closed"
	}
	if l.Issue == nil {
		l.Issue = &model.LinkedIssue{Tracker: x.t.Type}
	}
	l.Issue.Key, l.Issue.Status, l.Issue.State, l.Issue.SyncedAt = is.Key, is.Status, state, &now
	if is.URL != "" {
		l.URL = is.URL
	}
	l.Label = trackerNames[x.t.Type] + " " + is.Key
	if utf8.RuneCountInString(l.Label) > store.MaxLinkLabel {
		l.Label = string([]rune(l.Label)[:store.MaxLinkLabel])
	}
}

// create creates the requirement's issue and links it.
func (x *reqIssueCtx) create(req *model.Requirement) (*model.ItemLink, error) {
	in, _, err := x.draft(req)
	if err != nil {
		return nil, err
	}
	is, err := x.c.Create(x.ctx, in)
	if err != nil {
		return nil, err
	}
	l := &model.ItemLink{SourceType: model.KindRequirement, SourceID: req.ID, TargetType: "url", URL: is.URL, CreatedBy: x.p.User.Email}
	x.applyIssueLink(l, is)
	if err := x.s.Store.CreateItemLink(x.ctx, x.p.TenantID(), l); err != nil {
		return nil, fmt.Errorf("the issue %s was created but could not be linked: %w", is.Key, err)
	}
	l.Direction = "outgoing"
	x.s.audit(x.ctx, x.p, "requirement.issue.create", model.KindRequirement, req.ID, req.Key+" → "+x.t.Type+" "+is.Key)
	return l, nil
}

// update pushes the requirement's title and text into its issue.
func (x *reqIssueCtx) update(req *model.Requirement, l *model.ItemLink) error {
	in, _, err := x.draft(req)
	if err != nil {
		return err
	}
	cur, err := x.c.Get(x.ctx, l.Issue.Key)
	if err != nil {
		return err
	}
	in.Description = tracker.MergeManaged(x.c.Markup(), cur.Description, in.Description)
	is, err := x.c.Update(x.ctx, l.Issue.Key, in)
	if err != nil {
		return err
	}
	x.applyIssueLink(l, is)
	if err := x.s.Store.SetItemLinkIssue(x.ctx, x.p.TenantID(), l); err != nil {
		return err
	}
	x.s.audit(x.ctx, x.p, "requirement.issue.update", model.KindRequirement, req.ID, req.Key+" → "+x.t.Type+" "+is.Key)
	return nil
}

// sync reads the issue's state; with comment_on_pass it comments once on
// an open issue when all of the requirement's tests passed. changed: the
// state changed.
func (x *reqIssueCtx) sync(req *model.Requirement, l *model.ItemLink) (changed, commented bool, err error) {
	is, err := x.c.Get(x.ctx, l.Issue.Key)
	if err != nil {
		return false, false, err
	}
	old := l.Issue.State
	x.applyIssueLink(l, is)
	changed = old != l.Issue.State
	if x.t.CommentOnPass {
		tests, err := x.linkedTests(req)
		if err != nil {
			return false, false, err
		}
		switch {
		case x.coverage(tests) != "passed":
			l.Issue.PassedAt = nil
		case l.Issue.PassedAt == nil && !is.Closed:
			d := tracker.NewDoc(x.c.Markup())
			keys := make([]string, len(tests))
			for i, tc := range tests {
				keys[i] = tc.Key
			}
			text := fmt.Sprintf("%s All %d linked tests of requirement %s passed in Qtestframe: %s.", resultIcons["passed"], len(tests), req.Key, strings.Join(keys, ", "))
			if link := x.s.itemLink(x.pr); link != nil {
				text += " " + d.Link("Open "+req.Key, link(req.Key))
			}
			if err := x.c.Comment(x.ctx, l.Issue.Key, text); err != nil {
				return false, false, err
			}
			now := x.s.Store.Now()
			l.Issue.PassedAt, commented = &now, true
		}
	}
	if err := x.s.Store.SetItemLinkIssue(x.ctx, x.p.TenantID(), l); err != nil {
		return false, false, err
	}
	detail := req.Key + " ← " + x.t.Type + " " + is.Key + " " + is.Status
	if changed {
		detail += " (" + old + " → " + l.Issue.State + ")"
	}
	if commented {
		detail += ", commented: all tests passed"
	}
	x.s.audit(x.ctx, x.p, "requirement.issue.sync", model.KindRequirement, req.ID, detail)
	return changed, commented, nil
}

// ---------------------------------------------------------------- handlers

// requirementIssue loads a requirement, its project's tracker and its issue link.
func (s *Server) requirementIssue(r *http.Request, p *Principal) (*reqIssueCtx, *model.Requirement, *model.ItemLink, error) {
	req, err := s.Store.GetRequirement(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		return nil, nil, nil, err
	}
	x, err := s.newReqIssueCtx(r, p, req.ProjectID)
	if err != nil {
		return nil, nil, nil, err
	}
	links, err := s.Store.IssueLinks(r.Context(), p.TenantID(), req.ProjectID, []string{req.ID})
	if err != nil {
		return nil, nil, nil, err
	}
	return x, req, issueLinkOf(links, req.ID, x.t), nil
}

type issuePreview struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
}

// handleGetRequirementIssue: GET /requirements/{id}/issue — the tracker
// type, the requirement's issue link (null when none) and a preview of
// the issue as it would be created or updated now.
func (s *Server) handleGetRequirementIssue(w http.ResponseWriter, r *http.Request, p *Principal) {
	req, err := s.Store.GetRequirement(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	t, err := s.Store.DefectTracker(r.Context(), p.TenantID(), req.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := map[string]any{"tracker": t.Type, "link": nil, "preview": nil}
	if t.Type == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	x, req, l, err := s.requirementIssue(r, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if l != nil {
		out["link"] = l
	}
	in, _, err := x.draft(req)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	labels := in.Labels
	if t.Type != "jira" {
		labels = mergeStrings(splitList(t.Labels), labels)
	}
	if labels == nil {
		labels = []string{}
	}
	out["preview"] = issuePreview{Title: in.Title, Description: in.Description, Labels: labels}
	writeJSON(w, http.StatusOK, out)
}

func splitList(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func mergeStrings(a, b []string) []string {
	out := slices.Clone(a)
	for _, x := range b {
		if !slices.ContainsFunc(out, func(y string) bool { return strings.EqualFold(x, y) }) {
			out = append(out, x)
		}
	}
	return out
}

// issueFail answers a failed tracker request (502) or another error.
func (s *Server) issueFail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, tracker.ErrConfig) {
		s.fail(w, r, badRequest("%v", err))
		return
	}
	s.trackerFail(w, r, err)
}

// handleCreateRequirementIssue: POST /requirements/{id}/issue creates the
// requirement's issue; 409 issue_exists (with the link) when it has one.
func (s *Server) handleCreateRequirementIssue(w http.ResponseWriter, r *http.Request, p *Principal) {
	x, req, l, err := s.requirementIssue(r, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if l != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "the requirement already has the issue " + l.Issue.Key + ": open or update it", "code": "issue_exists", "link": l})
		return
	}
	l, err = x.create(req)
	if err != nil {
		s.issueFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"link": l})
}

// handleUpdateRequirementIssue: PUT /requirements/{id}/issue pushes the
// requirement's current title and text into its issue.
func (s *Server) handleUpdateRequirementIssue(w http.ResponseWriter, r *http.Request, p *Principal) {
	x, req, l, err := s.requirementIssue(r, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if l == nil {
		s.fail(w, r, store.ErrNotFound)
		return
	}
	if err := x.update(req, l); err != nil {
		s.issueFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"link": l})
}

// handleSyncRequirementIssue: POST /requirements/{id}/issue/sync reads the
// issue's state back.
func (s *Server) handleSyncRequirementIssue(w http.ResponseWriter, r *http.Request, p *Principal) {
	x, req, l, err := s.requirementIssue(r, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if l == nil {
		s.fail(w, r, store.ErrNotFound)
		return
	}
	changed, commented, err := x.sync(req, l)
	if err != nil {
		s.issueFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"link": l, "changed": changed, "commented": commented})
}

// bulkRequirements loads requirements of a project by id (in the given order).
func (s *Server) bulkRequirements(r *http.Request, p *Principal, projectID string, ids []string) ([]*model.Requirement, error) {
	ids = uniqStrings(ids)
	if len(ids) == 0 {
		return nil, badRequest("choose at least one requirement")
	}
	if len(ids) > maxBulkIssues {
		return nil, badRequest("at most %d requirements at a time", maxBulkIssues)
	}
	out := make([]*model.Requirement, 0, len(ids))
	for _, id := range ids {
		req, err := s.Store.GetRequirement(r.Context(), p.TenantID(), id)
		if err != nil {
			return nil, err
		}
		if req.ProjectID != projectID {
			return nil, fmt.Errorf("requirement %s: %w", id, store.ErrNotFound)
		}
		out = append(out, req)
	}
	return out, nil
}

type issueResult struct {
	Key      string `json:"key"`
	IssueKey string `json:"issue_key"`
	URL      string `json:"url"`
	State    string `json:"state,omitempty"`
}

type issueFailure struct {
	Key   string `json:"key"`
	Error string `json:"error"`
}

// handleBulkRequirementIssues: POST /projects/{pid}/requirements/issues
// {"ids": [...], "update": false} creates the issues of the requirements
// that have none; those that have one are skipped, or updated with
// "update": true.
func (s *Server) handleBulkRequirementIssues(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var body struct {
		IDs    []string `json:"ids"`
		Update bool     `json:"update"`
	}
	if err := decode(r, &body); err != nil {
		s.fail(w, r, err)
		return
	}
	reqs, err := s.bulkRequirements(r, p, pr.ID, body.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	x, err := s.newReqIssueCtx(r, p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ids := make([]string, len(reqs))
	for i, req := range reqs {
		ids[i] = req.ID
	}
	links, err := s.Store.IssueLinks(r.Context(), p.TenantID(), pr.ID, ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	created, updated, skipped := []issueResult{}, []issueResult{}, []issueResult{}
	failed := []issueFailure{}
	for _, req := range reqs {
		l := issueLinkOf(links, req.ID, x.t)
		switch {
		case l == nil:
			nl, err := x.create(req)
			if err != nil {
				failed = append(failed, issueFailure{req.Key, err.Error()})
				continue
			}
			created = append(created, issueResult{req.Key, nl.Issue.Key, nl.URL, nl.Issue.State})
		case body.Update:
			if err := x.update(req, l); err != nil {
				failed = append(failed, issueFailure{req.Key, err.Error()})
				continue
			}
			updated = append(updated, issueResult{req.Key, l.Issue.Key, l.URL, l.Issue.State})
		default:
			skipped = append(skipped, issueResult{req.Key, l.Issue.Key, l.URL, l.Issue.State})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": created, "updated": updated, "skipped": skipped, "failed": failed})
}

// handleSyncRequirementIssues: POST /projects/{pid}/requirements/issues/sync
// with an optional body {"ids": [...]} reads back the state of the issues
// of the project's requirements (or of those requirements).
func (s *Server) handleSyncRequirementIssues(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var ids []string // nil: all
	if raw, err := readBody(r); err == nil && len(strings.TrimSpace(string(raw))) > 0 {
		var body struct {
			IDs []string `json:"ids"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			s.fail(w, r, badRequest("invalid JSON: %v", err))
			return
		}
		if body.IDs != nil {
			reqs, err := s.bulkRequirements(r, p, pr.ID, body.IDs)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			ids = []string{}
			for _, req := range reqs {
				ids = append(ids, req.ID)
			}
		}
	}
	x, err := s.newReqIssueCtx(r, p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	links, err := s.Store.IssueLinks(r.Context(), p.TenantID(), pr.ID, ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	synced, changed, commented := 0, 0, 0
	failed := []issueFailure{}
	for i := range links {
		l := &links[i]
		if !tracker.Owns(x.t, l.Issue.Tracker, l.Issue.Key) {
			continue
		}
		req, err := s.Store.GetRequirement(r.Context(), p.TenantID(), l.SourceID)
		if err != nil {
			failed = append(failed, issueFailure{l.SourceKey, err.Error()})
			continue
		}
		ch, cm, err := x.sync(req, l)
		if err != nil {
			failed = append(failed, issueFailure{req.Key, err.Error()})
			continue
		}
		synced++
		if ch {
			changed++
		}
		if cm {
			commented++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"synced": synced, "changed": changed, "commented": commented, "failed": failed})
}
