package api

import (
	"context"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// The personal work list ("My work"): what needs the caller, across the
// projects they can see. GET /api/v1/my-work lists it, GET
// /api/v1/my-work/count gives the numbers for the badge in the main
// navigation and the dashboard widget.
//
// The list is built from work sources. A source finds one kind of work
// (owned items, assigned defects, review requests waiting, ...) and returns
// rows with the reasons the row is there; rows for the same item from
// several sources are merged, their reasons joined.
//
// Adding a source (for example item discussions with @mentions, test
// cycles with tester assignments or exploratory sessions):
//
//  1. Write a function with the signature of workSourceFunc that reads the
//     work of w.Principal (w.Names are the lower-case ways the caller can be
//     named in owner and assignee fields; w.Sees(projectID, perm) checks
//     project visibility and permissions; w.Today is today, YYYY-MM-DD).
//     Return one WorkItem per item: Kind and ID identify it (rows with the
//     same Kind and ID are merged), EntityType/EntityID (and RunID for
//     executions) say what the row links to in the client, Reasons say why
//     it is there (a short code; add its label to workList.reasons in
//     web/src/i18n), Due/Overdue its due date if it has one.
//  2. Register it in an init function: registerWorkSource("mentions",
//     sourceMentions). Names must be unique; the client can filter by them
//     (?source=). Keep a source cheap: it runs for every badge refresh.
//  3. If the client must open a new kind of item, add the route to
//     workItemRoute in web/src/lib/worklist.ts.
//
// A failing source is logged and left out, so one broken source does not
// hide the rest of the list.

// WorkItem is one row of the work list.
type WorkItem struct {
	// Kind of item (requirement, test, risk, defect, execution, waiver, ...)
	// and its id; Kind+ID identify the row (a review request is a row of
	// the item to review). Status: the item's status, when the source knows it.
	Kind string `json:"kind"`
	ID   string `json:"id"`
	// EntityType / EntityID: the item the row links to; RunID is set for
	// executions (the client opens the run's execution).
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	RunID      string `json:"run_id,omitempty"`
	Key        string `json:"key"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	ProjectID  string `json:"project_id"`
	// Derived from ProjectID.
	ProjectKey  string `json:"project_key"`
	ProjectName string `json:"project_name"`
	// Reasons: why the row is there (owner, assigned, review_requested,
	// review_returned, awaiting_review, in_review, waiver_approval, risk_acceptance, risk_review, overdue, suspect,
	// stale, blocked, ...); Detail: extra text (the run of an execution,
	// the requester of a review).
	Reasons []string `json:"reasons"`
	Detail  string   `json:"detail,omitempty"`
	// Due: date it is due (YYYY-MM-DD, "" none); Overdue when before today.
	Due       string    `json:"due,omitempty"`
	Overdue   bool      `json:"overdue"`
	UpdatedAt time.Time `json:"updated_at"`
	// Sources that contributed the row.
	Sources []string `json:"sources"`
	// ReviewIDs: the pending review requests of the item waiting for the
	// caller's decision (the client opens them from the row);
	// RequestedReviewIDs: review requests of the item the caller asked for
	// that are still pending or came back with changes requested.
	ReviewIDs          []string `json:"review_ids,omitempty"`
	RequestedReviewIDs []string `json:"requested_review_ids,omitempty"`
}

// waitingReasons are the reasons of rows that wait for someone else (a
// review the caller asked for, an item of theirs in review): they are
// listed but not counted as work in the badge unless the row has another
// reason too.
var waitingReasons = []string{"awaiting_review", "in_review"}

// waitingOnly reports whether every reason of a row is a waiting reason.
func waitingOnly(it *WorkItem) bool {
	for _, r := range it.Reasons {
		if !slices.Contains(waitingReasons, r) {
			return false
		}
	}
	return len(it.Reasons) > 0
}

// appendNew appends the values of add that are not in list yet.
func appendNew(list []string, add ...string) []string {
	for _, v := range add {
		if !slices.Contains(list, v) {
			list = append(list, v)
		}
	}
	return list
}

// WorkContext is what a source works with.
type WorkContext struct {
	Server    *Server
	Principal *Principal
	// Names: the lower-case ways the caller can be named (e-mail address
	// first, display name, handles, user id).
	Names []string
	// Today: YYYY-MM-DD (UTC).
	Today string
	// Visible: the projects the caller sees (project id → access);
	// ProjectID: the project the list is limited to ("" all).
	Visible   map[string]*model.ProjectAccessInfo
	ProjectID string
}

// Sees reports whether a row of a project belongs in the list: the caller
// sees the project and holds perm there ("" any), and it is the project the
// list is limited to.
func (w *WorkContext) Sees(projectID string, perm rbac.Permission) bool {
	if w.ProjectID != "" && projectID != w.ProjectID {
		return false
	}
	return visibleIn(w.Visible, w.Principal, projectID, perm)
}

// workSourceFunc collects the rows of one source.
type workSourceFunc func(ctx context.Context, w *WorkContext) ([]WorkItem, error)

type workSource struct {
	name    string
	collect workSourceFunc
}

var workSources []workSource

// registerWorkSource adds a source to the work list (see the package
// comment above for how to write one).
func registerWorkSource(name string, fn workSourceFunc) {
	for _, s := range workSources {
		if s.name == name {
			panic("work source registered twice: " + name)
		}
	}
	workSources = append(workSources, workSource{name: name, collect: fn})
}

// workSourceNames lists the registered sources.
func workSourceNames() []string {
	out := make([]string, 0, len(workSources))
	for _, s := range workSources {
		out = append(out, s.name)
	}
	return out
}

func init() {
	registerWorkSource("owned", sourceOwned)
	registerWorkSource("defects", sourceDefects)
	registerWorkSource("executions", sourceExecutions)
	registerWorkSource("reviews", sourceReviews)
	registerWorkSource("waivers", sourceWaivers)
	registerWorkSource("risk_acceptance", sourceRiskAcceptance)
}

func (s *Server) routesWorkList(h routeFunc) {
	h("GET /api/v1/my-work", s.handleMyWork)
	h("GET /api/v1/my-work/count", s.handleMyWorkCount)
}

// workList builds the caller's work list, limited to a project (id or key,
// "" all) and to some sources (nil all).
func (s *Server) workList(ctx context.Context, p *Principal, project string, only []string) ([]WorkItem, error) {
	vis, err := s.visibleProjects(ctx, p)
	if err != nil {
		return nil, err
	}
	projects, err := s.Store.ListProjects(ctx, p.TenantID())
	if err != nil {
		return nil, err
	}
	byID := map[string]*model.Project{}
	for i := range projects {
		byID[projects[i].ID] = &projects[i]
	}
	w := &WorkContext{Server: s, Principal: p, Names: s.myNames(ctx, p), Today: time.Now().UTC().Format("2006-01-02"), Visible: vis}
	if project != "" {
		for _, pr := range projects {
			if pr.ID == project || strings.EqualFold(pr.Key, project) {
				w.ProjectID = pr.ID
			}
		}
		if w.ProjectID == "" || vis[w.ProjectID] == nil {
			return nil, store.ErrNotFound
		}
	}
	rows := map[string]*WorkItem{}
	var order []string
	for _, src := range workSources {
		if only != nil && !slices.Contains(only, src.name) {
			continue
		}
		items, err := src.collect(ctx, w)
		if err != nil {
			s.Log.Warn("work list source", "source", src.name, "err", err)
			continue
		}
		for _, it := range items {
			if !w.Sees(it.ProjectID, "") {
				continue
			}
			k := it.Kind + ":" + it.ID
			cur := rows[k]
			if cur == nil {
				it := it
				it.Sources = []string{src.name}
				rows[k], order = &it, append(order, k)
				continue
			}
			cur.Reasons = appendNew(cur.Reasons, it.Reasons...)
			cur.Sources = appendNew(cur.Sources, src.name)
			cur.ReviewIDs = appendNew(cur.ReviewIDs, it.ReviewIDs...)
			cur.RequestedReviewIDs = appendNew(cur.RequestedReviewIDs, it.RequestedReviewIDs...)
			if cur.Status == "" {
				cur.Status = it.Status
			}
			if it.Due != "" && (cur.Due == "" || it.Due < cur.Due) {
				cur.Due = it.Due
			}
			if cur.Detail == "" {
				cur.Detail = it.Detail
			}
		}
	}
	out := make([]WorkItem, 0, len(order))
	for _, k := range order {
		it := rows[k]
		it.Overdue = it.Due != "" && it.Due < w.Today
		if it.Overdue && !slices.Contains(it.Reasons, "overdue") {
			it.Reasons = append(it.Reasons, "overdue")
		}
		if pr := byID[it.ProjectID]; pr != nil {
			it.ProjectKey, it.ProjectName = pr.Key, pr.Name
		}
		out = append(out, *it)
	}
	sortWork(out)
	return out, nil
}

// sortWork: overdue rows first, then by due date, then most recently
// changed first.
func sortWork(list []WorkItem) {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.Overdue != b.Overdue {
			return a.Overdue
		}
		if (a.Due != "") != (b.Due != "") {
			return a.Due != ""
		}
		if a.Due != b.Due {
			return a.Due < b.Due
		}
		return a.UpdatedAt.After(b.UpdatedAt)
	})
}

// workSummary counts the rows of a work list.
type workSummary struct {
	// Count: rows that need the caller (the badge); Waiting: rows that only
	// wait for others (awaiting_review, in_review), not in Count.
	Count   int `json:"count"`
	Waiting int `json:"waiting"`
	Overdue int `json:"overdue"`
	// ByReason / ByKind / ByProject: rows per reason, kind and project id.
	ByReason  map[string]int `json:"by_reason"`
	ByKind    map[string]int `json:"by_kind"`
	ByProject map[string]int `json:"by_project"`
}

func summarizeWork(list []WorkItem) workSummary {
	out := workSummary{ByReason: map[string]int{}, ByKind: map[string]int{}, ByProject: map[string]int{}}
	for _, it := range list {
		if waitingOnly(&it) {
			out.Waiting++
		} else {
			out.Count++
		}
		if it.Overdue {
			out.Overdue++
		}
		for _, r := range it.Reasons {
			out.ByReason[r]++
		}
		out.ByKind[it.Kind]++
		out.ByProject[it.ProjectID]++
	}
	return out
}

func workQuery(r *http.Request) (string, []string) {
	var only []string
	if v := strings.TrimSpace(r.URL.Query().Get("source")); v != "" {
		only = strings.Split(v, ",")
	}
	return strings.TrimSpace(r.URL.Query().Get("project")), only
}

// handleMyWork: GET /my-work?project=<id or key>&source=<a,b>&reason=<r>
// lists the caller's work; reason keeps the rows with that reason.
func (s *Server) handleMyWork(w http.ResponseWriter, r *http.Request, p *Principal) {
	project, only := workQuery(r)
	list, err := s.workList(r.Context(), p, project, only)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if reason := r.URL.Query().Get("reason"); reason != "" {
		list = slices.DeleteFunc(list, func(it WorkItem) bool { return !slices.Contains(it.Reasons, reason) })
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "total": len(list), "summary": summarizeWork(list), "sources": workSourceNames()})
}

// handleMyWorkCount: GET /my-work/count?project= gives the summary only.
func (s *Server) handleMyWorkCount(w http.ResponseWriter, r *http.Request, p *Principal) {
	project, only := workQuery(r)
	list, err := s.workList(r.Context(), p, project, only)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, summarizeWork(list))
}

// ---------------------------------------------------------------- sources

// Statuses in which owned items still need their owner.
var (
	openRequirementStatuses = []string{"draft", "review", "approved", "implemented"}
	openTestStatuses        = []string{"draft", "review"}
	// dead items are not flagged suspect, stale or blocked
	deadStatuses = []string{"rejected", "obsolete", "deprecated"}
)

// sourceOwned: requirements and tests the caller owns in a non-final
// status, owned open risks, risks whose review they own and that are due
// within a week, and owned requirements and tests that became suspect,
// have stale results or are blocked by defects.
func sourceOwned(ctx context.Context, w *WorkContext) ([]WorkItem, error) {
	s, p := w.Server, w.Principal
	rows, err := s.Store.OwnedItems(ctx, p.TenantID(), w.Names)
	if err != nil {
		return nil, err
	}
	isMe := func(name string) bool { return slices.Contains(w.Names, strings.ToLower(strings.TrimSpace(name))) }
	soon := time.Now().UTC().AddDate(0, 0, 7).Format("2006-01-02")
	flags := map[string]*itemFlags{}
	var out []WorkItem
	for _, row := range rows {
		perm := readPermOf(row.Kind)
		if row.Kind == model.KindRisk {
			perm = rbac.RequirementRd
		}
		if !w.Sees(row.ProjectID, perm) {
			continue
		}
		it := WorkItem{Kind: row.Kind, ID: row.ID, EntityType: row.Kind, EntityID: row.ID, Key: row.Key, Title: row.Title,
			Status: row.Status, ProjectID: row.ProjectID, UpdatedAt: row.UpdatedAt, Reasons: []string{}}
		switch row.Kind {
		case model.KindRequirement, model.KindTest:
			open := openTestStatuses
			if row.Kind == model.KindRequirement {
				open = openRequirementStatuses
			}
			if slices.Contains(open, row.Status) {
				it.Reasons = append(it.Reasons, "owner")
			}
			if !slices.Contains(deadStatuses, row.Status) {
				f := flags[row.ProjectID]
				if f == nil {
					if f, err = s.loadItemFlags(ctx, p.TenantID(), row.ProjectID); err != nil {
						return nil, err
					}
					flags[row.ProjectID] = f
				}
				it.Reasons = append(it.Reasons, f.of(row.Kind, row.ID)...)
			}
		case model.KindRisk:
			if isMe(row.Owner) && slices.Contains(model.RiskOpenStatuses, row.Status) {
				it.Reasons = append(it.Reasons, "owner")
			}
			if isMe(row.ReviewOwner) && row.Due != "" && row.Due <= soon && row.Status != "closed" {
				it.Reasons = append(it.Reasons, "risk_review")
				it.Due = row.Due
			}
		}
		if len(it.Reasons) > 0 {
			out = append(out, it)
		}
	}
	return out, nil
}

// itemFlags: suspect links and relations, stale results and blocking
// defects of the requirements and tests of a project.
type itemFlags struct {
	r2t         map[string][]string
	suspect     map[string]int
	suspectRels map[string]int
	stale       *store.StaleIndex
	blocks      map[string][]model.DefectBlock
}

func (s *Server) loadItemFlags(ctx context.Context, tenantID, projectID string) (*itemFlags, error) {
	links, err := s.Store.ListLinks(ctx, tenantID, projectID, "", "")
	if err != nil {
		return nil, err
	}
	f := &itemFlags{suspect: map[string]int{}}
	f.r2t, _ = store.LinkIndex(links)
	for _, l := range links {
		if l.SuspectSince != nil {
			f.suspect[l.RequirementID]++
			f.suspect[l.TestCaseID]++
		}
	}
	if f.stale, err = s.Store.LoadStaleIndex(ctx, tenantID, projectID); err != nil {
		return nil, err
	}
	if f.blocks, err = s.Store.DefectBlocks(ctx, tenantID, projectID, f.stale.Settings()); err != nil {
		return nil, err
	}
	if f.suspectRels, err = s.Store.SuspectRelationCounts(ctx, tenantID, projectID); err != nil {
		return nil, err
	}
	return f, nil
}

// of returns the flags of an item as reasons.
func (f *itemFlags) of(kind, id string) []string {
	var out []string
	if f.suspect[id] > 0 || f.suspectRels[id] > 0 {
		out = append(out, "suspect")
	}
	if kind == model.KindTest {
		if f.stale.Test(id).Stale {
			out = append(out, "stale")
		}
		return out
	}
	if f.stale.StaleCount(f.r2t[id]) > 0 {
		out = append(out, "stale")
	}
	if blocking, _ := store.BlockCounts(f.blocks[id]); blocking > 0 {
		out = append(out, "blocked")
	}
	return out
}

// sourceDefects: open defects assigned to the caller.
func sourceDefects(ctx context.Context, w *WorkContext) ([]WorkItem, error) {
	rows, err := w.Server.Store.AssignedDefects(ctx, w.Principal.TenantID(), w.Names, model.DefectOpenStatuses)
	if err != nil {
		return nil, err
	}
	var out []WorkItem
	for _, row := range rows {
		if !w.Sees(row.ProjectID, rbac.ProjectRead) {
			continue
		}
		out = append(out, WorkItem{Kind: model.KindDefect, ID: row.ID, EntityType: model.KindDefect, EntityID: row.ID, Key: row.Key, Title: row.Title,
			Status: row.Status, ProjectID: row.ProjectID, UpdatedAt: row.UpdatedAt, Reasons: []string{"assigned"}})
	}
	return out, nil
}

// sourceExecutions: test executions assigned to the caller that are not
// executed yet, in runs that are planned or in progress.
func sourceExecutions(ctx context.Context, w *WorkContext) ([]WorkItem, error) {
	rows, err := w.Server.Store.AssignedExecutions(ctx, w.Principal.TenantID(), w.Names)
	if err != nil {
		return nil, err
	}
	var out []WorkItem
	for _, row := range rows {
		if !w.Sees(row.ProjectID, rbac.RunRead) {
			continue
		}
		out = append(out, WorkItem{Kind: "execution", ID: row.ID, EntityType: "execution", EntityID: row.ID, RunID: row.RunID, Key: row.Key, Title: row.Title,
			Status: row.Status, ProjectID: row.ProjectID, UpdatedAt: row.UpdatedAt, Reasons: []string{"assigned"}, Detail: row.RunName})
	}
	return out, nil
}

// sourceReviews: review requests waiting for the caller's decision. The
// row is the item to review (it merges with an owner row of the item); the
// request ids are on the row so the client can open the review.
func sourceReviews(ctx context.Context, w *WorkContext) ([]WorkItem, error) {
	list, err := w.Server.Store.ListReviewRequests(ctx, w.Principal.TenantID(), store.ReviewFilter{ReviewerID: w.Principal.User.ID, Status: "pending"})
	if err != nil {
		return nil, err
	}
	var out []WorkItem
	for _, v := range list {
		if !w.Sees(v.ProjectID, readPermOf(v.EntityType)) {
			continue
		}
		out = append(out, WorkItem{Kind: v.EntityType, ID: v.EntityID, EntityType: v.EntityType, EntityID: v.EntityID, Key: v.EntityKey, Title: v.EntityTitle,
			ProjectID: v.ProjectID, UpdatedAt: v.CreatedAt, Reasons: []string{"review_requested"}, Due: v.DueDate, Detail: v.RequestedBy, ReviewIDs: []string{v.ID}})
	}
	return out, nil
}

// sourceWaivers: requested waivers the caller may decide and has not
// signed yet: they are one of its approvers, or it has none and the caller
// holds waiver.approve. The requester never approves their own waiver.
func sourceWaivers(ctx context.Context, w *WorkContext) ([]WorkItem, error) {
	s, p := w.Server, w.Principal
	ids, err := s.Store.RequestedWaiverIDs(ctx, p.TenantID())
	if err != nil {
		return nil, err
	}
	email := strings.ToLower(p.User.Email)
	var out []WorkItem
	for _, id := range ids {
		wv, err := s.Store.GetWaiver(ctx, p.TenantID(), id)
		if err != nil {
			return nil, err
		}
		if !w.Sees(wv.ProjectID, rbac.WaiverApprove) || strings.EqualFold(wv.RequestedBy, email) {
			continue
		}
		if len(wv.Approvers) > 0 && !slices.ContainsFunc(wv.Approvers, func(a string) bool { return strings.EqualFold(a, email) }) {
			continue
		}
		if slices.ContainsFunc(wv.Approvals, func(a model.WaiverApproval) bool { return a.UserID == p.User.ID && a.Valid }) {
			continue
		}
		out = append(out, WorkItem{Kind: model.KindWaiver, ID: wv.ID, EntityType: model.KindWaiver, EntityID: wv.ID, Key: wv.Key, Title: wv.Title,
			Status: wv.Status, ProjectID: wv.ProjectID, UpdatedAt: wv.UpdatedAt, Reasons: []string{"waiver_approval"}, Detail: wv.RequestedBy})
	}
	return out, nil
}

// sourceRiskAcceptance: risks whose residual level is above the project's
// acceptance threshold and that were mitigated or marked accepted without
// a valid acceptance signature, for the people who may sign it
// (waiver.approve).
func sourceRiskAcceptance(ctx context.Context, w *WorkContext) ([]WorkItem, error) {
	s, p := w.Server, w.Principal
	projects, err := s.Store.RiskAcceptanceProjects(ctx, p.TenantID())
	if err != nil {
		return nil, err
	}
	var out []WorkItem
	for _, pid := range projects {
		if !w.Sees(pid, rbac.WaiverApprove) {
			continue
		}
		risks, err := s.Store.ListRisks(ctx, p.TenantID(), pid)
		if err != nil {
			return nil, err
		}
		for _, k := range risks {
			if k.Status != "mitigated" && k.Status != "accepted" || !k.AboveThreshold || k.AcceptanceValid {
				continue
			}
			out = append(out, WorkItem{Kind: model.KindRisk, ID: k.ID, EntityType: model.KindRisk, EntityID: k.ID, Key: k.Key, Title: k.Title,
				Status: k.Status, ProjectID: k.ProjectID, UpdatedAt: k.UpdatedAt, Reasons: []string{"risk_acceptance"}, Detail: k.ResidualLevel})
		}
	}
	return out, nil
}
