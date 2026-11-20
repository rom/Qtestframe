package api

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Bulk operations on test runs (the run list) and on the executions of a
// run.

const maxBulkRuns = 500

// ---------------------------------------------------------------- runs

// handleBulkRuns: POST /projects/{pid}/runs/bulk
// {"ids": [...], "set": {"status", "release_id" ("" removes), "environment", "build"}}.
func (s *Server) handleBulkRuns(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		IDs []string `json:"ids"`
		Set struct {
			Status      *string `json:"status"`
			ReleaseID   *string `json:"release_id"`
			Environment *string `json:"environment"`
			Build       *string `json:"build"`
		} `json:"set"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	set := req.Set
	if set.Status == nil && set.ReleaseID == nil && set.Environment == nil && set.Build == nil {
		s.fail(w, r, badRequest("nothing to change: set status, release_id, environment or build"))
		return
	}
	if set.Status != nil && !model.Valid(model.RunStatuses, *set.Status) {
		s.fail(w, r, badRequest("invalid run status %q", *set.Status))
		return
	}
	runs, err := s.bulkRunsOf(r, p, pr.ID, req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if set.ReleaseID != nil && *set.ReleaseID != "" {
		rel, err := s.Store.GetRelease(r.Context(), p.TenantID(), *set.ReleaseID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if rel.ProjectID != pr.ID {
			s.fail(w, r, badRequest("the release belongs to another project"))
			return
		}
	}
	ctx := r.Context()
	n := 0
	for _, run := range runs {
		old := run.Status
		if set.Status != nil {
			run.Status = *set.Status
		}
		if set.Environment != nil {
			run.Environment = *set.Environment
		}
		if set.Build != nil {
			run.Build = *set.Build
		}
		if set.ReleaseID != nil {
			run.ReleaseID = set.ReleaseID
			if *set.ReleaseID == "" {
				run.ReleaseID = nil
			}
		}
		if err := s.Store.UpdateRun(ctx, p.TenantID(), run); err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", run.Name, err))
			return
		}
		n++
		s.audit(ctx, p, "run.update", "run", run.ID, run.Status+" (bulk)")
		s.notifyEvent(r, p, pr.ID, "run.updated", map[string]any{"run": run, "bulk": true})
		if old != "completed" && run.Status == "completed" {
			s.notifyEvent(r, p, pr.ID, "run.completed", map[string]any{"run": run})
		}
	}
	s.audit(ctx, p, "run.bulk_update", "run", "", fmt.Sprintf("%d runs", n))
	writeJSON(w, http.StatusOK, map[string]int{"updated": n})
}

// handleBulkDeleteRuns: DELETE /projects/{pid}/runs/bulk {"ids": [...]}.
func (s *Server) handleBulkDeleteRuns(w http.ResponseWriter, r *http.Request, p *Principal) {
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
	runs, err := s.bulkRunsOf(r, p, pr.ID, req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	n := 0
	for _, run := range runs {
		if err := s.Store.DeleteRun(r.Context(), p.TenantID(), run.ID); err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", run.Name, err))
			return
		}
		n++
		s.audit(r.Context(), p, "run.delete", "run", run.ID, run.Name+" (bulk)")
	}
	s.syncEvidence(r.Context(), p, pr.ID, nil)
	writeJSON(w, http.StatusOK, map[string]int{"deleted": n})
}

// bulkRunsOf loads the runs, which must all belong to the project.
func (s *Server) bulkRunsOf(r *http.Request, p *Principal, projectID string, ids []string) ([]*model.TestRun, error) {
	ids = uniqStrings(ids)
	if len(ids) == 0 {
		return nil, badRequest("choose at least one run")
	}
	if len(ids) > maxBulkRuns {
		return nil, badRequest("at most %d runs at a time", maxBulkRuns)
	}
	var out []*model.TestRun
	for _, id := range ids {
		run, err := s.Store.GetRun(r.Context(), p.TenantID(), id)
		if err != nil {
			return nil, err
		}
		if run.ProjectID != projectID {
			return nil, fmt.Errorf("run %s: %w", id, store.ErrNotFound)
		}
		out = append(out, run)
	}
	return out, nil
}

// ---------------------------------------------------------------- executions

type bulkExecItem struct {
	ID       string `json:"id"`
	Key      string `json:"key"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Assignee string `json:"assignee"`
	Reason   string `json:"reason,omitempty"`
}

func execItem(e *model.Execution, reason string) bulkExecItem {
	return bulkExecItem{ID: e.ID, Key: e.TestKey, Title: e.TestTitle, Status: e.Status, Assignee: e.Assignee, Reason: reason}
}

// handleBulkExecutions: POST /runs/{id}/executions/bulk
// {"ids": [...] | "all": true, "from": [statuses], "set": {"status", "assignee", "notes_append"}}
// (run.execute), or {"ids" | "all", "from", "remove": true} (run.write).
func (s *Server) handleBulkExecutions(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	run, err := s.Store.GetRun(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req struct {
		IDs    []string `json:"ids"`
		All    bool     `json:"all"`
		From   []string `json:"from"`
		Remove bool     `json:"remove"`
		Set    struct {
			Status      *string `json:"status"`
			Assignee    *string `json:"assignee"`
			NotesAppend string  `json:"notes_append"`
		} `json:"set"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	need := "run.execute"
	if req.Remove {
		need = "run.write"
	}
	if !p.Can(rbac.Permission(need)) {
		writeError(w, http.StatusForbidden, "forbidden", "permission "+need+" required")
		return
	}
	set := req.Set
	note := strings.TrimSpace(set.NotesAppend)
	if !req.Remove && set.Status == nil && set.Assignee == nil && note == "" {
		s.fail(w, r, badRequest("nothing to change: set status, assignee or notes_append, or remove"))
		return
	}
	if set.Status != nil && (*set.Status == model.ResultInProgress || !model.Valid(model.ResultStatuses, *set.Status)) {
		s.fail(w, r, badRequest("status must be passed, failed, blocked, skipped or not_run"))
		return
	}
	for _, f := range req.From {
		if !model.Valid(model.ResultStatuses, f) {
			s.fail(w, r, badRequest("unknown status %q in from", f))
			return
		}
	}
	if len(req.IDs) == 0 && !req.All {
		s.fail(w, r, badRequest("give ids or all: true"))
		return
	}
	all, err := s.Store.ListExecutions(ctx, p.TenantID(), run.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	byID := map[string]*model.Execution{}
	for i := range all {
		byID[all[i].ID] = &all[i]
	}
	var chosen []*model.Execution
	if req.All {
		for i := range all {
			chosen = append(chosen, &all[i])
		}
	} else {
		for _, id := range uniqStrings(req.IDs) {
			e, ok := byID[id]
			if !ok {
				s.fail(w, r, fmt.Errorf("execution %s: %w", id, store.ErrNotFound))
				return
			}
			chosen = append(chosen, e)
		}
	}
	res := map[string]any{}
	skipped := []bulkExecItem{}
	var work []*model.Execution
	for _, e := range chosen {
		if len(req.From) > 0 && !slices.Contains(req.From, e.Status) {
			skipped = append(skipped, execItem(e, "status"))
			continue
		}
		work = append(work, e)
	}
	res["skipped"] = skipped

	if req.Remove {
		ids := make([]string, 0, len(work))
		for _, e := range work {
			ids = append(ids, e.ID)
		}
		n, err := s.Store.DeleteExecutions(ctx, p.TenantID(), run.ID, ids)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(ctx, p, "run.remove_tests", "run", run.ID, fmt.Sprintf("%s: %d removed", run.Name, n))
		s.syncEvidence(ctx, p, run.ProjectID, nil)
		res["removed"] = n
		writeJSON(w, http.StatusOK, res)
		return
	}

	// Apply in passes: a result that needs readiness waits until its
	// prerequisites in this batch have been recorded.
	updated := []bulkExecItem{}
	var tests []string
	pending := work
	var rejected []bulkExecItem
	for len(pending) > 0 {
		rc, err := s.loadReadiness(ctx, p.TenantID(), run.ProjectID, run.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		var next []*model.Execution
		rejected = rejected[:0]
		progress := false
		for _, e := range pending {
			u := store.ExecutionUpdate{Status: set.Status, Assignee: set.Assignee}
			if note != "" {
				n := note
				if strings.TrimSpace(e.Notes) != "" {
					n = e.Notes + "\n" + note
				}
				u.Notes = &n
			}
			if (u.Status == nil || *u.Status == e.Status) && (u.Assignee == nil || *u.Assignee == e.Assignee) && u.Notes == nil {
				skipped = append(skipped, execItem(e, "already"))
				continue
			}
			if starts(u, e) {
				if rd := rc.check(e.TestCaseID); !rd.Allowed {
					var why []string
					for _, i := range rd.Issues {
						if i.Blocking {
							why = append(why, issueText(i))
						}
					}
					rejected = append(rejected, execItem(e, strings.Join(why, "; ")))
					next = append(next, e)
					continue
				}
			}
			runBefore := ""
			if cur, err := s.Store.GetRun(ctx, p.TenantID(), run.ID); err == nil {
				runBefore = cur.Status
			}
			after, err := s.Store.UpdateExecution(ctx, p.TenantID(), e.ID, p.User.Email, u)
			if err != nil {
				s.fail(w, r, fmt.Errorf("%s: %w", e.TestKey, err))
				return
			}
			progress = true
			updated = append(updated, execItem(e, ""))
			tests = append(tests, e.TestCaseID)
			s.audit(ctx, p, "execution.update", "execution", e.ID, e.TestKey+" "+after.Status+" (bulk)")
			s.afterExecutionChange(r, p, e, after, runBefore)
		}
		if !progress {
			break
		}
		pending = next
	}
	if rejected == nil {
		rejected = []bulkExecItem{}
	}
	if len(tests) > 0 {
		if reqs := s.requirementsOfTests(ctx, p, run.ProjectID, tests); len(reqs) > 0 {
			s.syncEvidence(ctx, p, run.ProjectID, reqs)
		}
	}
	s.audit(ctx, p, "run.bulk_executions", "run", run.ID, fmt.Sprintf("%s: %d updated, %d rejected, %d skipped", run.Name, len(updated), len(rejected), len(skipped)))
	res["updated"] = updated
	res["rejected"] = rejected
	res["skipped"] = skipped
	writeJSON(w, http.StatusOK, res)
}
