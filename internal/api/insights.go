package api

import (
	"net/http"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/report"
	"github.com/rom/qtestframe/internal/store"
)

// Dashboard insights (GET /projects/{pid}/dashboard/insights): the key
// figures and breakdowns the dashboard shows next to GET /dashboard —
// verified coverage, review states, tests by type and result, risks with
// a heat map, the age of open defects, suspect links, traceability rule
// results, the execution progress of the current release and the activity
// of the last 30 days. Everything is counted on the server.

type insightRequirements struct {
	Total int `json:"total"`
	// WithTests: requirements with at least one linked test; Passing: all
	// linked tests passed (coverage "passed").
	WithTests int `json:"with_tests"`
	Passing   int `json:"passing"`
	// Review: requirements by the state of their latest review request
	// (none, pending, changes_requested, approved).
	Review   map[string]int `json:"review"`
	ByStatus map[string]int `json:"by_status"`
}

type insightTests struct {
	Total int `json:"total"`
	// ByType: per execution type (manual, automated) the tests by last result.
	ByType map[string]map[string]int `json:"by_type"`
}

type insightRisks struct {
	Total int `json:"total"`
	Open  int `json:"open"`
	// ByLevel: open risks per level; ByStatus: all risks per status.
	ByLevel  map[string]int `json:"by_level"`
	ByStatus map[string]int `json:"by_status"`
	// Heat[l-1][i-1]: risks that are not closed with likelihood l and impact i.
	Heat [5][5]int `json:"heat"`
	// Mitigation: open risks per mitigation state (report.MitigationStates).
	Mitigation map[string]int `json:"mitigation"`
}

type insightTraceRules struct {
	Rules      int  `json:"rules"`
	Failed     int  `json:"failed"`
	Errors     int  `json:"errors"`
	Warnings   int  `json:"warnings"`
	Violations int  `json:"violations"`
	Passed     bool `json:"passed"`
}

type insightRelease struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	Status     string `json:"status"`
	TargetDate string `json:"target_date"`
	Runs       int    `json:"runs"`
	// Executions of the release's runs by result; Total, Executed (with a
	// final result) and Passed.
	Executions map[string]int `json:"executions"`
	Total      int            `json:"total"`
	Executed   int            `json:"executed"`
	Passed     int            `json:"passed"`
}

type insightActivity struct {
	From  string              `json:"from"`
	To    string              `json:"to"`
	Days  []store.ActivityDay `json:"days"`
	Total store.ActivityDay   `json:"total"`
}

type insightSuspect struct {
	Links        int `json:"links"`
	Requirements int `json:"requirements"`
}

type dashboardInsights struct {
	Requirements insightRequirements `json:"requirements"`
	Tests        insightTests        `json:"tests"`
	Risks        insightRisks        `json:"risks"`
	Defects      *store.DefectAges   `json:"defects"`
	Suspect      insightSuspect      `json:"suspect"`
	TraceRules   insightTraceRules   `json:"trace_rules"`
	Release      *insightRelease     `json:"release"`
	Activity     insightActivity     `json:"activity"`
}

// currentRelease is the release the dashboard follows: the newest active
// one, else the planned one with the earliest target date, else none.
func currentRelease(rs []model.Release) *model.Release {
	for i := range rs {
		if rs[i].Status == "active" {
			return &rs[i]
		}
	}
	var pick *model.Release
	for i := range rs {
		r := &rs[i]
		if r.Status != "planned" {
			continue
		}
		if pick == nil || (r.TargetDate != "" && (pick.TargetDate == "" || r.TargetDate < pick.TargetDate)) {
			pick = r
		}
	}
	return pick
}

func (s *Server) handleDashboardInsights(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	ten := p.TenantID()
	out, err := s.dashboardInsights(r, ten, pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) dashboardInsights(r *http.Request, ten string, pr *model.Project) (*dashboardInsights, error) {
	ctx := r.Context()
	pc, err := s.loadContext(ctx, ten, pr)
	if err != nil {
		return nil, err
	}
	reqs, err := s.Store.ListRequirements(ctx, ten, pr.ID)
	if err != nil {
		return nil, err
	}
	tests, err := s.Store.ListTests(ctx, ten, pr.ID)
	if err != nil {
		return nil, err
	}
	review, err := s.Store.RequirementReviewStates(ctx, ten, pr.ID)
	if err != nil {
		return nil, err
	}
	out := &dashboardInsights{}

	// requirements: verified coverage and review states
	rq := insightRequirements{Total: len(reqs), Review: map[string]int{}, ByStatus: map[string]int{}}
	for _, st := range store.ReviewStates {
		rq.Review[st] = 0
	}
	coverage := make(map[string]string, len(reqs))
	for i := range reqs {
		q := &reqs[i]
		pc.decorateRequirement(q)
		coverage[q.ID] = q.Coverage
		rq.ByStatus[q.Status]++
		if q.LinkedTests > 0 {
			rq.WithTests++
		}
		if q.Coverage == model.ResultPassed {
			rq.Passing++
		}
		st := review[q.ID]
		if st == "" {
			st = store.ReviewStateNone
		}
		rq.Review[st]++
	}
	out.Requirements = rq

	// tests by execution type and last result
	tt := insightTests{Total: len(tests), ByType: map[string]map[string]int{}}
	for _, et := range []string{model.ExecManual, model.ExecAutomated} {
		tt.ByType[et] = map[string]int{}
	}
	for i := range tests {
		t := &tests[i]
		pc.decorateTest(t)
		res := t.LastResult
		if res == "" {
			res = model.ResultNotRun
		}
		if tt.ByType[t.ExecutionType] == nil {
			tt.ByType[t.ExecutionType] = map[string]int{}
		}
		tt.ByType[t.ExecutionType][res]++
	}
	out.Tests = tt

	// risks
	points, err := s.Store.RiskPoints(ctx, ten, pr.ID)
	if err != nil {
		return nil, err
	}
	rk := insightRisks{Total: len(points), ByLevel: map[string]int{}, ByStatus: map[string]int{}, Mitigation: map[string]int{}}
	for _, lv := range model.RiskLevels {
		rk.ByLevel[lv] = 0
	}
	for _, st := range model.RiskStatuses {
		rk.ByStatus[st] = 0
	}
	for _, ms := range report.MitigationStates {
		rk.Mitigation[ms] = 0
	}
	for _, k := range points {
		rk.ByStatus[k.Status]++
		if k.Status != "closed" && k.Likelihood >= 1 && k.Likelihood <= 5 && k.Impact >= 1 && k.Impact <= 5 {
			rk.Heat[k.Likelihood-1][k.Impact-1]++
		}
		if store.RiskOpen(k.Status) {
			rk.Open++
			rk.ByLevel[model.RiskLevel(k.Likelihood, k.Impact)]++
			rk.Mitigation[report.MitigationState(k.RequirementIDs, coverage)]++
		}
	}
	out.Risks = rk

	now := s.Store.Now()
	if out.Defects, err = s.Store.OpenDefectAges(ctx, ten, pr.ID, now); err != nil {
		return nil, err
	}
	if out.Suspect.Links, out.Suspect.Requirements, err = s.Store.SuspectCounts(ctx, ten, pr.ID); err != nil {
		return nil, err
	}

	// traceability rules (enabled rules only)
	results, err := s.evaluateTraceRules(ctx, ten, pr, false, nil)
	if err != nil {
		return nil, err
	}
	sum := summarize(results)
	out.TraceRules = insightTraceRules{Rules: sum.Summary["rules"], Failed: sum.Summary["failed"], Errors: sum.Summary["errors"],
		Warnings: sum.Summary["warnings"], Violations: sum.Summary["violations"], Passed: sum.Passed}

	// execution progress of the current release
	rels, err := s.Store.ListReleases(ctx, ten, pr.ID)
	if err != nil {
		return nil, err
	}
	if rel := currentRelease(rels); rel != nil {
		runs, err := s.Store.ListRuns(ctx, ten, pr.ID)
		if err != nil {
			return nil, err
		}
		ir := &insightRelease{ID: rel.ID, Name: rel.Name, Version: rel.Version, Status: rel.Status, TargetDate: rel.TargetDate, Executions: map[string]int{}}
		for _, st := range model.ResultStatuses {
			ir.Executions[st] = 0
		}
		for _, rn := range runs {
			if rn.ReleaseID == nil || *rn.ReleaseID != rel.ID {
				continue
			}
			ir.Runs++
			for _, st := range model.ResultStatuses {
				ir.Executions[st] += rn.Summary[st]
			}
			ir.Total += rn.Summary["total"]
		}
		for st, n := range ir.Executions {
			if st != model.ResultNotRun && st != model.ResultInProgress {
				ir.Executed += n
			}
		}
		ir.Passed = ir.Executions[model.ResultPassed]
		out.Release = ir
	}

	// activity of the last 30 days (today included)
	to := now.UTC()
	from := to.AddDate(0, 0, 1-store.ActivityDays)
	days, err := s.Store.Activity(ctx, ten, pr.ID, from, to)
	if err != nil {
		return nil, err
	}
	act := insightActivity{From: store.Day(from), To: store.Day(to), Days: days}
	for _, d := range days {
		act.Total.Changes += d.Changes
		act.Total.Executions += d.Executions
		act.Total.Defects += d.Defects
	}
	out.Activity = act
	return out, nil
}
