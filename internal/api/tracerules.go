package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/tracerules"
)

// Traceability rules: reading and evaluating needs project.read, changing
// the rules project.update (the project settings permission).

func (s *Server) routesTraceRules(h routeFunc) {
	h("GET /api/v1/projects/{pid}/trace-rules", s.handleListTraceRules, "project.read")
	h("POST /api/v1/projects/{pid}/trace-rules", s.handleCreateTraceRule, "project.update")
	h("GET /api/v1/projects/{pid}/trace-rules/evaluate", s.handleEvaluateTraceRules, "project.read")
	h("POST /api/v1/projects/{pid}/trace-rules/preview", s.handlePreviewTraceRule, "project.read")
	h("GET /api/v1/trace-rules/{id}", s.handleGetTraceRule, "project.read")
	h("PATCH /api/v1/trace-rules/{id}", s.handleUpdateTraceRule, "project.update")
	h("DELETE /api/v1/trace-rules/{id}", s.handleDeleteTraceRule, "project.update")
	h("GET /api/v1/requirements/{id}/trace-rules", s.handleItemTraceRules(model.KindRequirement), "requirement.read")
	h("GET /api/v1/tests/{id}/trace-rules", s.handleItemTraceRules(model.KindTest), "test.read")
}

var traceSchemas = map[string]*query.Schema{model.KindRequirement: requirementSchema, model.KindTest: testSchema}

// checkTraceRule validates a rule including its queries.
func checkTraceRule(r *model.TraceRule) error {
	if err := store.ValidateTraceRule(r); err != nil {
		return err
	}
	if _, err := query.Parse(r.Selector, traceSchemas[r.Scope]); err != nil {
		return fmt.Errorf("selector: %w", err)
	}
	if r.TargetQuery != "" {
		kind := model.KindRequirement
		switch r.Kind {
		case model.TraceVerifiedBy:
			kind = model.KindTest
		case model.TraceItemLink:
			kind = r.TargetType
		}
		if _, err := query.Parse(r.TargetQuery, traceSchemas[kind]); err != nil {
			return fmt.Errorf("target_query: %w", err)
		}
	}
	return nil
}

func (s *Server) handleListTraceRules(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	rules, err := s.Store.ListTraceRules(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(rules))
}

// traceRuleBody is the editable part of a rule (pointers: PATCH changes
// what is given; a new rule is enabled unless enabled is false).
type traceRuleBody struct {
	Name         *string `json:"name"`
	Description  *string `json:"description"`
	Severity     *string `json:"severity"`
	Enabled      *bool   `json:"enabled"`
	Scope        *string `json:"scope"`
	Selector     *string `json:"selector"`
	Kind         *string `json:"kind"`
	Min          *int    `json:"min"`
	NotSuspect   *bool   `json:"not_suspect"`
	Passed       *bool   `json:"passed"`
	RelationType *string `json:"relation_type"`
	Direction    *string `json:"direction"`
	TargetType   *string `json:"target_type"`
	TargetQuery  *string `json:"target_query"`
	Every        *bool   `json:"every"`
}

func (b *traceRuleBody) apply(r *model.TraceRule) {
	for _, f := range []struct{ src, dst *string }{{b.Name, &r.Name}, {b.Description, &r.Description}, {b.Severity, &r.Severity},
		{b.Scope, &r.Scope}, {b.Selector, &r.Selector}, {b.Kind, &r.Kind}, {b.RelationType, &r.RelationType},
		{b.Direction, &r.Direction}, {b.TargetType, &r.TargetType}, {b.TargetQuery, &r.TargetQuery}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	for _, f := range []struct{ src, dst *bool }{{b.Enabled, &r.Enabled}, {b.NotSuspect, &r.NotSuspect}, {b.Passed, &r.Passed}, {b.Every, &r.Every}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	if b.Min != nil {
		r.Min = *b.Min
	}
}

// newTraceRule builds a rule of project pid from a request body.
func newTraceRule(r *http.Request, pid string) (*model.TraceRule, error) {
	var body traceRuleBody
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	rule := &model.TraceRule{ProjectID: pid, Enabled: true, Min: 1}
	body.apply(rule)
	return rule, checkTraceRule(rule)
}

func (s *Server) handleCreateTraceRule(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	rule, err := newTraceRule(r, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rule.CreatedBy = p.User.Email
	if err := s.Store.CreateTraceRule(r.Context(), p.TenantID(), rule); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "trace_rule.create", "trace_rule", rule.ID, rule.Name)
	writeJSON(w, http.StatusCreated, rule)
}

func (s *Server) handleGetTraceRule(w http.ResponseWriter, r *http.Request, p *Principal) {
	rule, err := s.Store.GetTraceRule(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handleUpdateTraceRule(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	rule, err := s.Store.GetTraceRule(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var body traceRuleBody
	if err := decode(r, &body); err != nil {
		s.fail(w, r, err)
		return
	}
	body.apply(rule)
	if err := checkTraceRule(rule); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.UpdateTraceRule(ctx, p.TenantID(), rule); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "trace_rule.update", "trace_rule", rule.ID, rule.Name)
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handleDeleteTraceRule(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	rule, err := s.Store.GetTraceRule(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteTraceRule(ctx, p.TenantID(), rule.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "trace_rule.delete", "trace_rule", rule.ID, rule.Name)
	w.WriteHeader(http.StatusNoContent)
}

// traceGraph loads the project once for rule evaluation.
func (s *Server) traceGraph(ctx context.Context, tenantID string, pr *model.Project) (*tracerules.Graph, error) {
	pc, err := s.loadContext(ctx, tenantID, pr)
	if err != nil {
		return nil, err
	}
	reqs, err := s.Store.ListRequirements(ctx, tenantID, pr.ID)
	if err != nil {
		return nil, err
	}
	tests, err := s.Store.ListTests(ctx, tenantID, pr.ID)
	if err != nil {
		return nil, err
	}
	pc.indexCalls(tests)
	rels, err := s.Store.ListRelations(ctx, tenantID, pr.ID, "")
	if err != nil {
		return nil, err
	}
	links, err := s.Store.ListProjectItemLinks(ctx, tenantID, pr.ID, "")
	if err != nil {
		return nil, err
	}
	g := &tracerules.Graph{Links: pc.links, Relations: rels, ItemLinks: links, LastResults: pc.evidence(), Schemas: traceSchemas}
	reqByID := make(map[string]*model.Requirement, len(reqs))
	for i := range reqs {
		rq := &reqs[i]
		reqByID[rq.ID] = rq
		g.Requirements = append(g.Requirements, tracerules.Item{Kind: model.KindRequirement, ID: rq.ID, Key: rq.Key, Title: rq.Title})
	}
	testByID := make(map[string]*model.TestCase, len(tests))
	for i := range tests {
		t := &tests[i]
		testByID[t.ID] = t
		g.Tests = append(g.Tests, tracerules.Item{Kind: model.KindTest, ID: t.ID, Key: t.Key, Title: t.Title})
	}
	g.Record = func(kind, id string) query.Record {
		switch kind {
		case model.KindRequirement:
			if rq, ok := reqByID[id]; ok {
				pc.decorateRequirement(rq)
				return pc.requirementRecord(rq)
			}
		case model.KindTest:
			if t, ok := testByID[id]; ok {
				pc.decorateTest(t)
				return pc.testRecord(t)
			}
		}
		return nil
	}
	return g, nil
}

// evaluateTraceRules evaluates the enabled rules (all with all) of a project.
func (s *Server) evaluateTraceRules(ctx context.Context, tenantID string, pr *model.Project, all bool, scope tracerules.Scope) ([]tracerules.Result, error) {
	rules, err := s.Store.ListTraceRules(ctx, tenantID, pr.ID)
	if err != nil {
		return nil, err
	}
	var use []model.TraceRule
	for _, r := range rules {
		if r.Enabled || all {
			use = append(use, r)
		}
	}
	if len(use) == 0 {
		return []tracerules.Result{}, nil
	}
	g, err := s.traceGraph(ctx, tenantID, pr)
	if err != nil {
		return nil, err
	}
	return tracerules.Evaluate(use, g, scope), nil
}

// TraceRulesReport is the evaluation of a project's rules.
type TraceRulesReport struct {
	Results []tracerules.Result `json:"results"`
	// Passed is true when no enabled rule of severity error fails.
	Passed  bool           `json:"passed"`
	Summary map[string]int `json:"summary"`
}

func summarize(results []tracerules.Result) TraceRulesReport {
	rep := TraceRulesReport{Results: results, Passed: true, Summary: map[string]int{"rules": len(results), "passed": 0, "failed": 0, "errors": 0, "warnings": 0, "violations": 0}}
	for _, res := range results {
		rep.Summary["violations"] += res.ViolationCount
		if res.Passed {
			rep.Summary["passed"]++
			continue
		}
		rep.Summary["failed"]++
		if res.Rule.Severity == "warning" {
			rep.Summary["warnings"]++
		} else {
			rep.Summary["errors"]++
			if res.Rule.Enabled {
				rep.Passed = false
			}
		}
	}
	return rep
}

// limitViolations keeps at most n violations per result (violation_count
// stays the full number).
func limitViolations(results []tracerules.Result, n int) {
	for i := range results {
		if len(results[i].Violations) > n {
			results[i].Violations = results[i].Violations[:n]
		}
	}
}

func violationLimit(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 {
		return 500
	}
	return min(n, 100000)
}

// handleEvaluateTraceRules: ?all=1 includes disabled rules, ?rule_id=
// evaluates one rule, ?limit= bounds the violations listed per rule.
func (s *Server) handleEvaluateTraceRules(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	ctx := r.Context()
	ruleID := strings.TrimSpace(r.URL.Query().Get("rule_id"))
	results, err := s.evaluateTraceRules(ctx, p.TenantID(), pr, boolParam(r, "all", false) || ruleID != "", nil)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if ruleID != "" {
		var one []tracerules.Result
		for _, res := range results {
			if res.Rule.ID == ruleID {
				one = append(one, res)
			}
		}
		if one == nil {
			s.fail(w, r, store.ErrNotFound)
			return
		}
		results = one
	}
	limitViolations(results, violationLimit(r))
	writeJSON(w, http.StatusOK, summarize(results))
}

// handlePreviewTraceRule evaluates an unsaved rule (the body of a create).
func (s *Server) handlePreviewTraceRule(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	rule, err := newTraceRule(r, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	g, err := s.traceGraph(r.Context(), p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	res := tracerules.Evaluate([]model.TraceRule{*rule}, g, nil)
	limitViolations(res, violationLimit(r))
	writeJSON(w, http.StatusOK, res[0])
}

// handleItemTraceRules lists the enabled rules a requirement or test breaks.
func (s *Server) handleItemTraceRules(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		ctx := r.Context()
		id := r.PathValue("id")
		var pid string
		if kind == model.KindRequirement {
			rq, err := s.Store.GetRequirement(ctx, p.TenantID(), id)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			pid = rq.ProjectID
		} else {
			t, err := s.Store.GetTest(ctx, p.TenantID(), id)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			pid = t.ProjectID
		}
		pr, err := s.Store.GetProject(ctx, p.TenantID(), pid)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		results, err := s.evaluateTraceRules(ctx, p.TenantID(), pr, false, func(k, x string) bool { return k == kind && x == id })
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, listOf(tracerules.ForItem(results, kind, id)))
	}
}

// releaseRuleViolations returns the items of a release scope that break
// enabled rules of at least level (error or warning), one per item with
// the names of the rules as note.
func (s *Server) releaseRuleViolations(r *http.Request, p *Principal, projectID string, reqs, tests []string, level string) []store.ReleaseItem {
	ctx := r.Context()
	pr, err := s.Store.GetProject(ctx, p.TenantID(), projectID)
	if err != nil {
		return []store.ReleaseItem{}
	}
	in := map[string]bool{}
	for _, id := range reqs {
		in[model.KindRequirement+":"+id] = true
	}
	for _, id := range tests {
		in[model.KindTest+":"+id] = true
	}
	results, err := s.evaluateTraceRules(ctx, p.TenantID(), pr, false, func(k, id string) bool { return in[k+":"+id] })
	if err != nil {
		s.Log.Warn("release trace rules", "project", projectID, "err", err)
		return []store.ReleaseItem{}
	}
	out := []store.ReleaseItem{}
	at := map[string]int{}
	for _, res := range results {
		if !tracerules.Failing(res, level) {
			continue
		}
		if res.Error != "" {
			out = append(out, store.ReleaseItem{Kind: "trace_rule", ID: res.Rule.ID, Key: res.Rule.Name, Title: res.Error})
			continue
		}
		for _, v := range res.Violations {
			k := v.Kind + ":" + v.ID
			if i, ok := at[k]; ok {
				out[i].Note += "; " + res.Rule.Name
				continue
			}
			at[k] = len(out)
			out = append(out, store.ReleaseItem{Kind: v.Kind, ID: v.ID, Key: v.Key, Title: v.Title, Note: res.Rule.Name})
		}
	}
	return out
}
