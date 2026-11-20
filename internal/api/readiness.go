package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// Readiness issue kinds.
const (
	IssueTestStatus        = "test_status"        // the test is not "ready"
	IssueRequirementStatus = "requirement_status" // a verified requirement is not ready for testing
	IssueDependency        = "dependency"         // a prerequisite test has not passed
)

// ReadinessIssue is one reason why a test should not be executed yet.
type ReadinessIssue struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Key    string `json:"key"`
	Title  string `json:"title"`
	Status string `json:"status"`
	// Blocking issues prevent execution; others are warnings because the
	// organisation settings allow them.
	Blocking bool `json:"blocking"`
	// Required lists the statuses (or result) that would clear the issue:
	// ["ready"] for a test, the ready requirement statuses of the
	// organisation for a requirement, ["passed"] for a prerequisite.
	Required []string `json:"required"`
	// Setting names the organisation setting (TenantSettings JSON field)
	// that decides whether the issue blocks execution.
	Setting string `json:"setting"`
	// InRun (prerequisites): the status is the result of the prerequisite in
	// the same run; otherwise it is its latest result anywhere.
	InRun bool `json:"in_run,omitempty"`
}

// Readiness tells whether a test may be executed (see TenantSettings).
type Readiness struct {
	// Ready: no issues at all.
	Ready bool `json:"ready"`
	// Allowed: no blocking issues.
	Allowed bool             `json:"allowed"`
	Issues  []ReadinessIssue `json:"issues"`
}

// readinessContext holds what the readiness check of a project (and run)
// needs, loaded once.
type readinessContext struct {
	settings *model.TenantSettings
	readyReq map[string]bool
	tests    map[string]*model.TestCase
	reqs     map[string]*model.Requirement
	t2r      map[string][]string
	deps     map[string][]model.TestDependency
	last     map[string]string
	// inRun: test id → status of its execution in the run being checked
	inRun map[string]string
}

func (s *Server) loadReadiness(ctx context.Context, tenantID, projectID, runID string) (*readinessContext, error) {
	st, err := s.Store.TenantSettings(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	rc := &readinessContext{settings: st, readyReq: map[string]bool{}, tests: map[string]*model.TestCase{},
		reqs: map[string]*model.Requirement{}, deps: map[string][]model.TestDependency{}, inRun: map[string]string{}}
	for _, v := range st.ReadyRequirementStatuses {
		rc.readyReq[v] = true
	}
	tests, err := s.Store.ListTests(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	for i := range tests {
		rc.tests[tests[i].ID] = &tests[i]
	}
	reqs, err := s.Store.ListRequirements(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	for i := range reqs {
		rc.reqs[reqs[i].ID] = &reqs[i]
	}
	links, err := s.Store.ListLinks(ctx, tenantID, projectID, "", "")
	if err != nil {
		return nil, err
	}
	_, rc.t2r = store.LinkIndex(links)
	deps, err := s.Store.ListTestDependencies(ctx, tenantID, projectID, "")
	if err != nil {
		return nil, err
	}
	for _, d := range deps {
		rc.deps[d.TestID] = append(rc.deps[d.TestID], d)
	}
	if rc.last, err = s.Store.LastResults(ctx, tenantID, projectID); err != nil {
		return nil, err
	}
	if runID != "" {
		es, err := s.Store.ListExecutions(ctx, tenantID, runID)
		if err != nil {
			return nil, err
		}
		for _, e := range es {
			rc.inRun[e.TestCaseID] = e.Status
		}
	}
	return rc, nil
}

// check returns the readiness of a test. A prerequisite counts as passed
// when its execution in the same run passed, or — when it is not part of
// the run — when its latest result anywhere is passed.
func (rc *readinessContext) check(testID string) *Readiness {
	out := &Readiness{Issues: []ReadinessIssue{}}
	if t, ok := rc.tests[testID]; ok && t.Status != model.ReadyTestStatus {
		out.Issues = append(out.Issues, ReadinessIssue{Kind: IssueTestStatus, ID: t.ID, Key: t.Key, Title: t.Title, Status: t.Status,
			Blocking: !rc.settings.AllowUnreadyTests, Required: []string{model.ReadyTestStatus}, Setting: "allow_unready_tests"})
	}
	seen := map[string]bool{}
	for _, rid := range rc.t2r[testID] {
		r, ok := rc.reqs[rid]
		if !ok || seen[rid] || rc.readyReq[r.Status] {
			continue
		}
		seen[rid] = true
		out.Issues = append(out.Issues, ReadinessIssue{Kind: IssueRequirementStatus, ID: r.ID, Key: r.Key, Title: r.Title, Status: r.Status,
			Blocking: !rc.settings.AllowUnreadyRequirements, Required: append([]string{}, rc.settings.ReadyRequirementStatuses...), Setting: "allow_unready_requirements"})
	}
	for _, d := range rc.deps[testID] {
		status, inRun := rc.inRun[d.DependsOnID]
		if !inRun {
			status = rc.last[d.DependsOnID]
		}
		if status == "" {
			status = model.ResultNotRun
		}
		if status != model.ResultPassed {
			out.Issues = append(out.Issues, ReadinessIssue{Kind: IssueDependency, ID: d.DependsOnID, Key: d.DependsOnKey, Title: d.DependsOnTitle, Status: status,
				Blocking: !rc.settings.AllowUnmetDependencies, Required: []string{model.ResultPassed}, Setting: "allow_unmet_dependencies", InRun: inRun})
		}
	}
	out.Ready = len(out.Issues) == 0
	out.Allowed = true
	for _, i := range out.Issues {
		if i.Blocking {
			out.Allowed = false
		}
	}
	return out
}

func activeResult(s string) bool {
	return s != "" && s != model.ResultNotRun && s != model.ResultBlocked && s != model.ResultSkipped
}

// starts reports whether an execution update records an actual execution
// (as opposed to assigning, commenting, or marking it blocked or skipped).
// With the current execution cur, only changes count: saving notes of an
// execution that already has a result is not a new execution.
func starts(u store.ExecutionUpdate, cur *model.Execution) bool {
	if u.Status != nil && activeResult(*u.Status) && (cur == nil || cur.Status != *u.Status) {
		return true
	}
	prev := map[string]string{}
	if cur != nil {
		for _, sr := range cur.StepResults {
			prev[sr.ID] = sr.Status
		}
	}
	for _, sr := range u.StepResults {
		if activeResult(sr.Status) && (cur == nil || prev[sr.ID] != sr.Status) {
			return true
		}
	}
	return false
}

// notReady answers 409 not_ready with the blocking issues.
func notReady(w http.ResponseWriter, key string, rd *Readiness) {
	var parts []string
	for _, i := range rd.Issues {
		if i.Blocking {
			parts = append(parts, issueText(i))
		}
	}
	writeJSON(w, http.StatusConflict, map[string]any{
		"error":     fmt.Sprintf("%s cannot be executed: %s", key, strings.Join(parts, "; ")),
		"code":      "not_ready",
		"readiness": rd,
	})
}

// executionReadiness checks the readiness of an execution's test in its run.
func (s *Server) executionReadiness(ctx context.Context, tenantID string, e *model.Execution) (*Readiness, error) {
	run, err := s.Store.GetRun(ctx, tenantID, e.RunID)
	if err != nil {
		return nil, err
	}
	rc, err := s.loadReadiness(ctx, tenantID, run.ProjectID, run.ID)
	if err != nil {
		return nil, err
	}
	return rc.check(e.TestCaseID), nil
}

// handleTestReadiness: GET /projects/{pid}/readiness?test_ids=a,b — the
// readiness of tests outside a run (e.g. before creating one).
func (s *Server) handleTestReadiness(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	rc, err := s.loadReadiness(r.Context(), p.TenantID(), pr.ID, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := map[string]*Readiness{}
	ids := strings.Split(r.URL.Query().Get("test_ids"), ",")
	if r.URL.Query().Get("test_ids") == "" {
		ids = ids[:0]
		for id := range rc.tests {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if _, ok := rc.tests[id]; ok {
			out[id] = rc.check(id)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tests": out, "settings": rc.settings})
}

// issueText explains a readiness issue in one sentence (English; the web
// client builds its own, translated text from the fields).
func issueText(i ReadinessIssue) string {
	switch i.Kind {
	case IssueTestStatus:
		return fmt.Sprintf("the test %s has status %q, but only tests with status \"ready\" may be executed (setting %s is off)", i.Key, i.Status, i.Setting)
	case IssueRequirementStatus:
		return fmt.Sprintf("it verifies requirement %s, which has status %q, but the execution settings require linked requirements to be %s (setting %s is off)",
			i.Key, i.Status, quoteList(i.Required), i.Setting)
	default:
		where := "its latest result"
		if i.InRun {
			where = "its result in this run"
		}
		return fmt.Sprintf("the prerequisite test %s has not passed (%s is %q) (setting %s is off)", i.Key, where, i.Status, i.Setting)
	}
}

// quoteList: ["a","b","c"] → "a", "b" or "c".
func quoteList(v []string) string {
	q := make([]string, len(v))
	for i, s := range v {
		q[i] = fmt.Sprintf("%q", s)
	}
	switch len(q) {
	case 0:
		return "ready for testing"
	case 1:
		return q[0]
	}
	return strings.Join(q[:len(q)-1], ", ") + " or " + q[len(q)-1]
}
