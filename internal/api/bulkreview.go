package api

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// Bulk review: approve or reject many requirements, or mark many tests
// ready, in one step. The items come from ids, a query or the whole
// project; each is checked, and the ones that do not qualify are skipped
// with a reason. dry_run returns the preview without changing anything.

const maxBulkReview = 5000

// Reasons an item is skipped.
const (
	skipAlready          = "already"           // already has the target status
	skipStatus           = "status"            // its status is not one of "from"
	skipChangesRequested = "changes_requested" // the latest review asked for changes
	skipPendingReview    = "pending_review"    // review requests are still open
	skipNoObligation     = "no_obligation"     // a requirement without obligation level
	skipNoSteps          = "no_steps"          // a manual test without steps
	skipNoScript         = "no_script"         // an automated test without script
	skipSuspect          = "suspect"           // it has suspect links
)

// skipChecks can be overridden with "ignore" (status and already cannot).
var skipChecks = []string{skipChangesRequested, skipPendingReview, skipNoObligation, skipNoSteps, skipNoScript, skipSuspect}

type bulkReviewItem struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type bulkReviewResult struct {
	Decision string           `json:"decision"`
	DryRun   bool             `json:"dry_run"`
	Updated  []bulkReviewItem `json:"updated"`
	Skipped  []bulkReviewItem `json:"skipped"`
}

// bulkReviewCandidate is what the checks need of an item.
type bulkReviewCandidate struct {
	bulkReviewItem
	obligation string
	manual     bool
	steps      int
	script     string
	suspect    int
}

// bulkReviewReason returns why an item cannot get the decision ("" = it can).
func bulkReviewReason(kind string, c bulkReviewCandidate, decision string, from []string, review string, ignore map[string]bool) string {
	switch {
	case c.Status == decision:
		return skipAlready
	case !slices.Contains(from, c.Status):
		return skipStatus
	}
	check := func(reason string, bad bool) bool { return bad && !ignore[reason] }
	if check(skipChangesRequested, review == "changes_requested") {
		return skipChangesRequested
	}
	if check(skipPendingReview, review == "pending") {
		return skipPendingReview
	}
	if check(skipSuspect, c.suspect > 0 && decision != "rejected") {
		return skipSuspect
	}
	if kind == model.KindRequirement {
		if check(skipNoObligation, c.obligation == "" && decision == "approved") {
			return skipNoObligation
		}
		return ""
	}
	if check(skipNoSteps, c.manual && c.steps == 0) {
		return skipNoSteps
	}
	if check(skipNoScript, !c.manual && strings.TrimSpace(c.script) == "") {
		return skipNoScript
	}
	return ""
}

// reviewStates returns, per item of kind in the project, "pending" when a
// review request is open, else the decision of the latest decided one.
func (s *Server) reviewStates(r *http.Request, p *Principal, projectID, kind string) map[string]string {
	list, err := s.Store.ListReviewRequests(r.Context(), p.TenantID(), store.ReviewFilter{ProjectID: projectID, EntityType: kind})
	if err != nil {
		return nil
	}
	out := map[string]string{}
	// newest first: the first decided request per item is the latest decision
	for _, v := range list {
		switch {
		case v.Status == "pending":
			out[v.EntityID] = "pending"
		case v.Status == "cancelled" || out[v.EntityID] != "":
		default:
			out[v.EntityID] = v.Status
		}
	}
	return out
}

// handleBulkReview: POST /projects/{pid}/requirements/review and
// /projects/{pid}/tests/review with
// {"decision", "ids" | "query" | "all": true, "from": [statuses], "ignore": [checks], "comment", "dry_run"}.
func (s *Server) handleBulkReview(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		pr, ok := s.project(w, r, p)
		if !ok {
			return
		}
		var req struct {
			Decision string   `json:"decision"`
			IDs      []string `json:"ids"`
			Query    string   `json:"query"`
			All      bool     `json:"all"`
			From     []string `json:"from"`
			Ignore   []string `json:"ignore"`
			Comment  string   `json:"comment"`
			DryRun   bool     `json:"dry_run"`
		}
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
		if !reviewStatuses[kind][req.Decision] {
			var allowed []string
			for k := range reviewStatuses[kind] {
				allowed = append(allowed, k)
			}
			slices.Sort(allowed)
			s.fail(w, r, badRequest("decision must be one of %s", strings.Join(allowed, ", ")))
			return
		}
		if !p.Can(reviewPerm(kind)) {
			writeError(w, http.StatusForbidden, "forbidden", "permission "+string(reviewPerm(kind))+" required")
			return
		}
		if len(req.IDs) == 0 && strings.TrimSpace(req.Query) == "" && !req.All {
			s.fail(w, r, badRequest("give ids, a query or all: true"))
			return
		}
		statuses := model.RequirementStatuses
		if kind == model.KindTest {
			statuses = model.TestStatuses
		}
		if len(req.From) == 0 {
			req.From = []string{"review"}
		}
		for _, f := range req.From {
			if !slices.Contains(statuses, f) {
				s.fail(w, r, badRequest("unknown status %q in from", f))
				return
			}
		}
		ignore := map[string]bool{}
		for _, i := range req.Ignore {
			if !slices.Contains(skipChecks, i) {
				s.fail(w, r, badRequest("ignore must name checks among %s", strings.Join(skipChecks, ", ")))
				return
			}
			ignore[i] = true
		}
		if len([]rune(req.Comment)) > 10000 {
			s.fail(w, r, badRequest("the comment is at most 10000 characters"))
			return
		}

		cands, err := s.bulkReviewCandidates(r, p, pr, kind, req.IDs, req.Query)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if len(cands) > maxBulkReview {
			s.fail(w, r, badRequest("at most %d items at a time (%d selected)", maxBulkReview, len(cands)))
			return
		}
		states := s.reviewStates(r, p, pr.ID, kind)
		res := bulkReviewResult{Decision: req.Decision, DryRun: req.DryRun, Updated: []bulkReviewItem{}, Skipped: []bulkReviewItem{}}
		var ids []string
		for _, c := range cands {
			if reason := bulkReviewReason(kind, c, req.Decision, req.From, states[c.ID], ignore); reason != "" {
				c.Reason = reason
				res.Skipped = append(res.Skipped, c.bulkReviewItem)
				continue
			}
			res.Updated = append(res.Updated, c.bulkReviewItem)
			ids = append(ids, c.ID)
		}
		if req.DryRun || len(ids) == 0 {
			writeJSON(w, http.StatusOK, res)
			return
		}

		ctx := r.Context()
		for _, id := range ids {
			s.ensureInitialRevision(ctx, p, kind, id)
		}
		decision := req.Decision
		if _, err := s.Store.BulkUpdateItems(ctx, p.TenantID(), kind, pr.ID, ids, store.BulkUpdate{Status: &decision}); err != nil {
			s.fail(w, r, err)
			return
		}
		comment := strings.TrimSpace(req.Comment)
		for _, it := range res.Updated {
			s.record(ctx, p, kind, it.ID, "bulk_update")
			s.audit(ctx, p, kind+".review", kind, it.ID, it.Key+" "+it.Status+" → "+decision+" (bulk)")
			if comment != "" {
				c := &model.Comment{EntityType: kind, EntityID: it.ID, UserID: p.User.ID, Body: comment}
				if err := s.Store.AddComment(ctx, p.TenantID(), c); err != nil {
					s.Log.Warn("bulk review comment", "id", it.ID, "err", err)
				}
			}
			s.notifyEvent(r, p, pr.ID, kind+".updated", map[string]any{"kind": kind, "id": it.ID, "key": it.Key, "title": it.Title, "status": decision, "bulk": true})
		}
		s.audit(ctx, p, kind+".bulk_review", kind, "", fmt.Sprintf("%s: %d updated, %d skipped", decision, len(res.Updated), len(res.Skipped)))
		writeJSON(w, http.StatusOK, res)
	}
}

// bulkReviewCandidates resolves ids (in that order) or the query (all
// items when empty) of the project.
func (s *Server) bulkReviewCandidates(r *http.Request, p *Principal, pr *model.Project, kind string, ids []string, query string) ([]bulkReviewCandidate, error) {
	if len(ids) > 0 {
		query = "" // explicit ids win over the query
	}
	pc, err := s.loadContext(r.Context(), p.TenantID(), pr)
	if err != nil {
		return nil, err
	}
	var out []bulkReviewCandidate
	if kind == model.KindRequirement {
		list, err := s.filterRequirements(r, p, pr, pc, query, "", true)
		if err != nil {
			return nil, err
		}
		for _, q := range list {
			out = append(out, bulkReviewCandidate{bulkReviewItem: bulkReviewItem{ID: q.ID, Key: q.Key, Title: q.Title, Status: q.Status},
				obligation: q.Obligation, suspect: q.SuspectLinks})
		}
	} else {
		list, err := s.filterTests(r, p, pr, pc, query, "", true)
		if err != nil {
			return nil, err
		}
		for _, t := range list {
			out = append(out, bulkReviewCandidate{bulkReviewItem: bulkReviewItem{ID: t.ID, Key: t.Key, Title: t.Title, Status: t.Status},
				manual: t.ExecutionType != model.ExecAutomated, steps: max(len(t.Steps), t.StepCount), script: t.Script, suspect: t.SuspectLinks})
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	byID := map[string]bulkReviewCandidate{}
	for _, c := range out {
		byID[c.ID] = c
	}
	var sel []bulkReviewCandidate
	for _, id := range uniqStrings(ids) {
		c, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("%s %s: %w", kind, id, store.ErrNotFound)
		}
		sel = append(sel, c)
	}
	return sel, nil
}
