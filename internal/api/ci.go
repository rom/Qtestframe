package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/ci"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/tracker"
)

// CI/CD integration (see docs/api.md "CI/CD"): the quality gate a pipeline
// checks (qtestframe gate), the test selection of a change (qtestframe
// select), the ready-made pipeline templates and the commit statuses and
// merge request notes the server posts to GitLab when results with a
// commit arrive.
//
// Server-side posting: a result upload with ?commit= (and optionally
// ?merge_request=) into a project whose CI settings post statuses or notes
// adds a row to ci_deliveries (an outbox) once the upload is recorded; it
// is tried at once in the background and retried by the minute job
// (RunJobs → RunCIJobs) with exponential backoff (1, 2, 4, … minutes, or
// the Retry-After of HTTP 429) up to ciMaxAttempts; a refused token or
// project (HTTP 4xx other than 429) is not retried. What is posted is
// computed when it is sent, as the user who uploaded the results.

func (s *Server) routesCI(h routeFunc) {
	h("GET /api/v1/projects/{pid}/gate", s.handleGate, "project.read")
	h("POST /api/v1/projects/{pid}/gate", s.handleGate, "project.read")
	h("POST /api/v1/projects/{pid}/ci/select", s.handleCISelect, "test.read")
	h("GET /api/v1/projects/{pid}/ci/setup", s.handleCISetup, "project.read")
	h("PUT /api/v1/projects/{pid}/ci-settings", s.handlePutCISettings, "project.update")
	h("GET /api/v1/projects/{pid}/ci/deliveries", s.handleCIDeliveries, "project.update")
}

// routesCIPublic: the templates contain no secrets (GitLab's include:
// remote fetches them without credentials).
func (s *Server) routesCIPublic(pub func(string, http.HandlerFunc)) {
	pub("GET /api/v1/ci/templates/{name}", s.handleCITemplate)
}

const (
	// ciMaxAttempts: attempts of one delivery before it fails.
	ciMaxAttempts = 6
	// ciLease: how long a server owns a delivery it is posting.
	ciLease = 2 * time.Minute
	// Commit status names.
	ciStatusTests = "qtestframe/tests"
	ciStatusGate  = "qtestframe/gate"
)

// ---------------------------------------------------------------- gate

type gateReq struct {
	// Release: id, name or version.
	Release string `json:"release"`
	Variant string `json:"variant"`
	// Run: id or name (the newest run of that name); Build: id, CI id or number.
	Run   string `json:"run"`
	Build string `json:"build"`
	// Require: conditions (see store.ParseGateCondition).
	Require []string `json:"require"`
	// Scope of a gate without release: all | affected | a query.
	Scope string `json:"scope"`
	// Commit and MergeRequest: the change whose requirements are
	// "affected" (shown, and the scope "affected").
	Commit       string `json:"commit"`
	MergeRequest string `json:"merge_request"`
	// SummaryOnly: only the results of the run or build (no gate).
	SummaryOnly bool `json:"summary_only"`
}

func gateReqFromQuery(r *http.Request) gateReq {
	q := r.URL.Query()
	return gateReq{Release: q.Get("release"), Variant: q.Get("variant"), Run: q.Get("run"), Build: q.Get("build"),
		Require: q["require"], Scope: q.Get("scope"), Commit: q.Get("commit"), MergeRequest: q.Get("merge_request"),
		SummaryOnly: queryBool(r, "summary_only")}
}

func (s *Server) handleGate(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	in := gateReqFromQuery(r)
	if r.Method == http.MethodPost {
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	rep, err := s.gateReport(r, p, pr, &in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// findRelease finds a release of a project by id, name or version.
func (s *Server) findRelease(ctx context.Context, p *Principal, projectID, ref string) (*model.Release, error) {
	ref = strings.TrimSpace(ref)
	if rel, err := s.Store.GetRelease(ctx, p.TenantID(), ref); err == nil && rel.ProjectID == projectID {
		return rel, nil
	}
	list, err := s.Store.ListReleases(ctx, p.TenantID(), projectID)
	if err != nil {
		return nil, err
	}
	for _, by := range []func(model.Release) bool{
		func(x model.Release) bool { return strings.EqualFold(x.Name, ref) },
		func(x model.Release) bool { return x.Version != "" && strings.EqualFold(x.Version, ref) },
	} {
		for i := range list {
			if by(list[i]) {
				return s.Store.GetRelease(ctx, p.TenantID(), list[i].ID)
			}
		}
	}
	return nil, badRequest("release %q not found in project", ref)
}

// findRun finds a run of a project by id or name (the newest of that name,
// preferring runs that are not aborted).
func (s *Server) findRun(ctx context.Context, p *Principal, projectID, ref string) (*model.TestRun, error) {
	ref = strings.TrimSpace(ref)
	if run, err := s.Store.GetRun(ctx, p.TenantID(), ref); err == nil && run.ProjectID == projectID {
		return run, nil
	}
	list, err := s.Store.ListRuns(ctx, p.TenantID(), projectID)
	if err != nil {
		return nil, err
	}
	var found []model.TestRun
	for _, x := range list {
		if x.Name == ref {
			found = append(found, x)
		}
	}
	if len(found) == 0 {
		return nil, badRequest("run %q not found in project", ref)
	}
	sort.SliceStable(found, func(i, j int) bool {
		if a, b := found[i].Status == "aborted", found[j].Status == "aborted"; a != b {
			return b
		}
		return found[i].CreatedAt.After(found[j].CreatedAt)
	})
	return s.Store.GetRun(ctx, p.TenantID(), found[0].ID)
}

// resolveCycles turns the cycle (id or name) of cycle_executed conditions
// into a cycle id of the project.
func (s *Server) resolveCycles(ctx context.Context, p *Principal, projectID string, crit []model.ExitCriterion) error {
	var cycles []model.TestCycle
	for i := range crit {
		ref := crit[i].CycleID
		if crit[i].Type != "cycle_executed" || ref == "" {
			continue
		}
		if cycles == nil {
			var err error
			if cycles, err = s.Store.ListCycles(ctx, p.TenantID(), projectID); err != nil {
				return err
			}
		}
		i2 := slices.IndexFunc(cycles, func(c model.TestCycle) bool { return c.ID == ref || strings.EqualFold(c.Name, ref) })
		if i2 < 0 {
			return badRequest("test cycle %q not found in project", ref)
		}
		crit[i].CycleID = cycles[i2].ID
	}
	return nil
}

// changeItems returns the code changes of a commit and a merge request.
func (s *Server) changeItems(ctx context.Context, p *Principal, projectID, commit, mr string) ([]model.CodeChange, error) {
	var out []model.CodeChange
	if commit != "" {
		list, err := s.Store.CodeChangesByRef(ctx, p.TenantID(), projectID, model.CodeCommit, []string{commit})
		if err != nil {
			return nil, err
		}
		out = append(out, list...)
	}
	if mr != "" {
		list, err := s.Store.CodeChangesByRef(ctx, p.TenantID(), projectID, model.CodeMergeRequest, []string{mr})
		if err != nil {
			return nil, err
		}
		out = append(out, list...)
	}
	return out, nil
}

// gateResults are the results of the run or build of a gate.
type gateResults struct {
	counts *ci.Results
	// latest: test id → latest result in the runs (combined over configurations: worst first)
	latest   map[string]string
	failures []ci.Item
}

// runResults collects the results of runs.
func (s *Server) runResults(ctx context.Context, p *Principal, projectID string, runIDs []string) (*gateResults, error) {
	g := &gateResults{counts: &ci.Results{}, latest: map[string]string{}, failures: []ci.Item{}}
	rank := map[string]int{model.ResultFailed: 5, model.ResultBlocked: 4, model.ResultInProgress: 3, model.ResultNotRun: 2, model.ResultSkipped: 1, model.ResultPassed: 0}
	seenFail := map[string]bool{}
	for _, id := range runIDs {
		execs, err := s.Store.ListExecutions(ctx, p.TenantID(), id)
		if err != nil {
			return nil, err
		}
		for _, e := range execs {
			g.counts.Total++
			switch e.Status {
			case model.ResultPassed:
				g.counts.Passed++
			case model.ResultFailed:
				g.counts.Failed++
			case model.ResultBlocked:
				g.counts.Blocked++
			case model.ResultSkipped:
				g.counts.Skipped++
			case model.ResultInProgress:
				g.counts.InProgress++
			default:
				g.counts.NotRun++
			}
			if cur, ok := g.latest[e.TestCaseID]; !ok || rank[e.Status] > rank[cur] {
				g.latest[e.TestCaseID] = e.Status
			}
			if (e.Status == model.ResultFailed || e.Status == model.ResultBlocked) && !seenFail[e.TestCaseID] && len(g.failures) < 50 {
				seenFail[e.TestCaseID] = true
				note := e.Status
				if e.ConfigurationName != "" {
					note += ", " + e.ConfigurationName
				}
				g.failures = append(g.failures, ci.Item{Kind: model.KindTest, ID: e.TestCaseID, Key: e.TestKey, Title: e.TestTitle, Note: note,
					URL: s.chatItemURL(projectID, "execution", e.ID, id)})
			}
		}
	}
	sort.SliceStable(g.failures, func(i, j int) bool { return naturalKeyLess(g.failures[i].Key, g.failures[j].Key) })
	return g, nil
}

func (s *Server) ciItem(projectID string, it store.ReleaseItem) ci.Item {
	return ci.Item{Kind: it.Kind, ID: it.ID, Key: it.Key, Title: it.Title, Note: it.Note, Variants: it.Variants, URL: s.chatItemURL(projectID, it.Kind, it.ID)}
}

// gateReport evaluates a gate request.
func (s *Server) gateReport(r *http.Request, p *Principal, pr *model.Project, in *gateReq) (*ci.Report, error) {
	ctx := r.Context()
	rep := &ci.Report{Project: ci.Ref{ID: pr.ID, Key: pr.Key, Name: pr.Name}, Conditions: []ci.Condition{}, Failures: []ci.Item{}, Waivers: []ci.Item{},
		SummaryOnly: in.SummaryOnly, Commit: strings.TrimSpace(in.Commit), MergeRequest: strings.TrimLeft(strings.TrimSpace(in.MergeRequest), "!#"), Scope: "all"}
	q := store.GateQuery{ProjectID: pr.ID, Variant: strings.TrimSpace(in.Variant)}
	if strings.TrimSpace(in.Release) != "" {
		rel, err := s.findRelease(ctx, p, pr.ID, in.Release)
		if err != nil {
			return nil, err
		}
		q.ReleaseID = rel.ID
		rep.Release = &ci.Ref{ID: rel.ID, Name: rel.Name, URL: s.chatItemURL(pr.ID, "release", rel.ID)}
		rep.Scope = "release"
	}
	var runIDs []string
	if strings.TrimSpace(in.Run) != "" {
		run, err := s.findRun(ctx, p, pr.ID, in.Run)
		if err != nil {
			return nil, err
		}
		q.RunID = run.ID
		runIDs = []string{run.ID}
		rep.Run = &ci.Ref{ID: run.ID, Name: run.Name, URL: s.chatItemURL(pr.ID, "run", run.ID)}
	}
	if strings.TrimSpace(in.Build) != "" {
		b, err := s.findBuild(r, p, pr.ID, in.Build)
		if err != nil {
			return nil, err
		}
		q.BuildID = b.ID
		rep.Build = &ci.Ref{ID: b.ID, Name: b.Label(), URL: b.PipelineURL}
		if rep.Run == nil {
			runs, err := s.Store.ListRuns(ctx, p.TenantID(), pr.ID)
			if err != nil {
				return nil, err
			}
			for _, x := range runs {
				if x.BuildID != nil && *x.BuildID == b.ID && (q.ReleaseID == "" || (x.ReleaseID != nil && *x.ReleaseID == q.ReleaseID)) {
					runIDs = append(runIDs, x.ID)
				}
			}
		}
	}
	if q.Variant != "" {
		v, err := s.Store.FindVariant(ctx, p.TenantID(), pr.ID, q.Variant)
		if err != nil {
			return nil, err
		}
		rep.Variant = &ci.Ref{ID: v.ID, Name: v.Name}
	}
	if len(in.Require) > 0 {
		crit, err := store.ParseGateConditions(in.Require)
		if err != nil {
			return nil, err
		}
		if err := s.resolveCycles(ctx, p, pr.ID, crit); err != nil {
			return nil, err
		}
		q.Criteria = crit
	}
	// the requirements of the change
	changes, err := s.changeItems(ctx, p, pr.ID, rep.Commit, rep.MergeRequest)
	if err != nil {
		return nil, err
	}
	links, err := s.Store.ListLinks(ctx, p.TenantID(), pr.ID, "", "")
	if err != nil {
		return nil, err
	}
	r2t, t2r := map[string][]string{}, map[string][]string{}
	for _, l := range links {
		r2t[l.RequirementID] = append(r2t[l.RequirementID], l.TestCaseID)
		t2r[l.TestCaseID] = append(t2r[l.TestCaseID], l.RequirementID)
	}
	var affected []string
	for _, c := range changes {
		for _, it := range c.Items {
			ids := []string{it.ItemID}
			if it.ItemType == model.KindTest {
				ids = t2r[it.ItemID]
			}
			for _, id := range ids {
				if !slices.Contains(affected, id) {
					affected = append(affected, id)
				}
			}
		}
	}
	switch sc := strings.TrimSpace(in.Scope); {
	case sc == "" || sc == "all":
	case q.ReleaseID != "":
		return nil, badRequest("scope cannot be combined with a release (the release's requirements are the scope)")
	case sc == "affected":
		if rep.Commit == "" && rep.MergeRequest == "" {
			return nil, badRequest("the scope affected needs a commit or merge_request")
		}
		q.RequirementIDs, rep.Scope = append([]string{}, affected...), "affected"
	default:
		pc, err := s.loadContext(ctx, p.TenantID(), pr)
		if err != nil {
			return nil, err
		}
		list, err := s.filterRequirements(r, p, pr, pc, sc, "", true)
		if err != nil {
			return nil, err
		}
		q.RequirementIDs, rep.Scope = []string{}, sc
		for _, x := range list {
			q.RequirementIDs = append(q.RequirementIDs, x.ID)
		}
	}
	// results of the run or build
	var res *gateResults
	if len(runIDs) > 0 || rep.Build != nil {
		if !p.Can(rbac.RunRead) {
			return nil, fmt.Errorf("%w: permission run.read required for the results of a run or build", errForbidden)
		}
		if res, err = s.runResults(ctx, p, pr.ID, runIDs); err != nil {
			return nil, err
		}
		rep.Results, rep.Failures = res.counts, res.failures
	}
	if len(affected) > 0 {
		a := &ci.Affected{Requirements: []ci.AffectedRequirement{}}
		for _, id := range affected {
			rq, err := s.Store.GetRequirement(ctx, p.TenantID(), id)
			if err != nil || rq.ProjectID != pr.ID {
				continue
			}
			ar := ci.AffectedRequirement{ID: rq.ID, Key: rq.Key, Title: rq.Title, URL: s.chatItemURL(pr.ID, model.KindRequirement, rq.ID), Tests: len(r2t[rq.ID]), Result: "none"}
			if ar.Tests > 0 {
				a.Covered++
				ar.Result = model.ResultPassed
				for _, tid := range r2t[rq.ID] {
					st := ""
					if res != nil {
						st = res.latest[tid]
					}
					switch {
					case st == model.ResultFailed || st == model.ResultBlocked:
						ar.Result = model.ResultFailed
					case st != model.ResultPassed && ar.Result != model.ResultFailed:
						ar.Result = model.ResultNotRun
					}
				}
				if ar.Result == model.ResultPassed {
					a.Passed++
				}
			}
			a.Requirements = append(a.Requirements, ar)
		}
		sort.SliceStable(a.Requirements, func(i, j int) bool { return naturalKeyLess(a.Requirements[i].Key, a.Requirements[j].Key) })
		a.Total = len(a.Requirements)
		rep.Affected = a
	}
	rep.URL = s.chatItemURL(pr.ID, "dashboard", "")
	switch {
	case rep.Run != nil:
		rep.URL = rep.Run.URL
	case rep.Release != nil:
		rep.URL = rep.Release.URL
	case len(runIDs) == 1:
		rep.URL = s.chatItemURL(pr.ID, "run", runIDs[0])
	}
	if in.SummaryOnly {
		rep.Pass = rep.Results == nil || rep.Results.Failed+rep.Results.Blocked == 0
		rep.Verdict = map[bool]string{true: ci.VerdictPass, false: ci.VerdictFail}[rep.Pass]
		return rep, nil
	}
	st, err := s.Store.GateStatus(ctx, p.TenantID(), q,
		func(scope []string, level string) []store.ReleaseItem { return s.releaseDefects(r, p, scope, level) },
		func(scope []string, level string) []store.ReleaseItem { return s.releaseRisks(r, p, scope, level) },
		func(projectID string, reqs, tests []string, level string) []store.ReleaseItem {
			return s.releaseRuleViolations(r, p, projectID, reqs, tests, level)
		})
	if err != nil {
		return nil, err
	}
	rep.Pass, rep.Summary, rep.ScopeSize = st.Go, st.Summary, len(st.Release.RequirementIDs)
	cycleNames := map[string]string{}
	for _, c := range st.Criteria {
		cond := ci.Condition{Type: c.Type, Threshold: c.Threshold, Level: c.Level, CycleID: c.CycleID, State: c.State, Met: c.Met,
			Actual: c.Actual, Total: c.Total, Available: c.Available, Items: []ci.Item{}, Waived: []ci.Item{}}
		cycle := ""
		if c.CycleID != "" {
			if _, ok := cycleNames[c.CycleID]; !ok {
				if cy, err := s.Store.GetCycle(ctx, p.TenantID(), c.CycleID); err == nil {
					cycleNames[c.CycleID] = cy.Name
				}
			}
			cycle = strconv.Quote(cycleNames[c.CycleID])
		}
		cond.Label = ci.ConditionLabel(c.Type, c.Threshold, c.Level, cycle)
		for _, it := range c.Items {
			cond.Items = append(cond.Items, s.ciItem(pr.ID, it))
		}
		for _, it := range c.Waived {
			cond.Waived = append(cond.Waived, s.ciItem(pr.ID, it))
		}
		rep.Conditions = append(rep.Conditions, cond)
	}
	for _, g := range st.Variants {
		rep.Variants = append(rep.Variants, ci.VariantVerdict{Name: g.Variant.Name, Pass: g.Go})
	}
	for _, wv := range st.Waivers {
		rep.Waivers = append(rep.Waivers, ci.Item{Kind: "waiver", ID: wv.ID, Key: wv.Key, Title: wv.Title, URL: s.chatItemURL(pr.ID, "waiver", wv.ID)})
	}
	rep.Verdict = map[bool]string{true: ci.VerdictPass, false: ci.VerdictFail}[rep.Pass]
	return rep, nil
}

// ---------------------------------------------------------------- selection

type ciSelectReq struct {
	regressionReq
	// CreateRun creates a run of the selected automated tests (needs
	// run.write); Run names it (default "CI selection <date>").
	CreateRun bool   `json:"create_run"`
	Run       string `json:"run"`
	// IncludeManual also selects tests that are not automated (default:
	// they are only listed in manual).
	IncludeManual bool `json:"include_manual"`
}

func (s *Server) handleCISelect(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var in ciSelectReq
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	if in.CreateRun && !p.Can(rbac.RunWrite) {
		writeError(w, http.StatusForbidden, "forbidden", "permission run.write required to create the run")
		return
	}
	if in.ReleaseID != nil && strings.TrimSpace(*in.ReleaseID) != "" {
		rel, err := s.findRelease(r.Context(), p, pr.ID, *in.ReleaseID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		in.ReleaseID = &rel.ID
	}
	sel, err := s.regression(r, p, pr, &in.regressionReq)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	tests, err := s.Store.ListTests(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	byID := map[string]*model.TestCase{}
	for i := range tests {
		byID[tests[i].ID] = &tests[i]
	}
	out := &ci.Selection{Project: ci.Ref{ID: pr.ID, Key: pr.Key, Name: pr.Name}, Tests: []ci.SelectedTest{}, Manual: []ci.SelectedTest{},
		Requirements: []ci.SelectedRequirement{}, Changes: []ci.Ref{}, Unknown: sel.Unknown}
	if sel.Variant != nil {
		out.Variant = &ci.Ref{ID: sel.Variant.ID, Name: sel.Variant.Name}
	}
	reasons := func(list []regressionReason) []ci.Reason {
		o := make([]ci.Reason, len(list))
		for i, x := range list {
			o[i] = ci.Reason{Kind: x.Kind, Ref: x.Ref, Type: x.Type}
		}
		return o
	}
	var ids []string
	for _, it := range sel.Tests {
		t := byID[it.ID]
		if t == nil {
			continue
		}
		tags := t.Tags
		if tags == nil {
			tags = []string{}
		}
		st := ci.SelectedTest{ID: t.ID, Key: t.Key, Title: t.Title, ExecutionType: t.ExecutionType, Tags: tags, Runner: ci.RunnerOf(t.Tags, t.ScriptLanguage),
			Identifier: ci.Identifier(t.Key), LastResult: it.LastResult, Reasons: reasons(it.Reasons)}
		if t.ExecutionType == model.ExecAutomated || in.IncludeManual {
			out.Tests = append(out.Tests, st)
			ids = append(ids, t.ID)
		} else {
			out.Manual = append(out.Manual, st)
		}
	}
	for _, it := range sel.Requirements {
		out.Requirements = append(out.Requirements, ci.SelectedRequirement{ID: it.ID, Key: it.Key, Title: it.Title, Reasons: reasons(it.Reasons)})
	}
	for _, c := range sel.Changes {
		out.Changes = append(out.Changes, ci.Ref{ID: c.ID, Key: shortRef(&c), Name: c.Title, URL: c.URL})
	}
	if in.CreateRun && len(ids) > 0 {
		name := strings.TrimSpace(in.Run)
		if name == "" {
			name = "CI selection " + time.Now().UTC().Format("2006-01-02 15:04")
		}
		run := &model.TestRun{ProjectID: pr.ID, Name: name, Description: in.Description, Environment: in.Environment, Build: in.Build,
			CreatedBy: p.User.Email, ReleaseID: in.ReleaseID}
		if sel.Variant != nil {
			run.VariantID = &sel.Variant.ID
		}
		for _, id := range in.ConfigurationIDs {
			run.Configurations = append(run.Configurations, model.ConfigurationRef{ID: id})
		}
		if err := s.Store.CreateRun(r.Context(), p.TenantID(), run, ids); err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(r.Context(), p, "run.create", "run", run.ID, fmt.Sprintf("%s (CI selection, %d tests)", run.Name, len(ids)))
		s.notifyEvent(r, p, run.ProjectID, "run.created", map[string]any{"run": run})
		out.Run = &ci.Ref{ID: run.ID, Name: run.Name, URL: s.chatItemURL(pr.ID, "run", run.ID)}
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------- setup and templates

type ciVariable struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	Masked      bool   `json:"masked"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type ciTemplate struct {
	ci.Template
	Content string `json:"content"`
	URL     string `json:"url,omitempty"`
}

// handleCISetup: GET /projects/{pid}/ci/setup — what the CI setup panel
// shows: the variables to define and the templates with the server URL
// and project key filled in.
func (s *Server) handleCISetup(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	base := s.baseURL()
	vars := []ciVariable{
		{Name: "QTF_URL", Value: base, Required: true, Description: "server URL (set in the templates)"},
		{Name: "QTF_PROJECT", Value: pr.Key, Required: true, Description: "project key (set in the templates)"},
		{Name: "QTF_TOKEN", Masked: true, Required: true, Description: "API token of a CI user with the role tester"},
		{Name: "QTF_CLI_IMAGE", Required: true, Description: "container image with the qtestframe binary"},
		{Name: "QTF_GITLAB_TOKEN", Masked: true, Description: "GitLab project access token (role Developer, scope api) for commit statuses and merge request notes"},
		{Name: "QTF_RELEASE", Description: "release whose exit criteria are the gate"},
	}
	out := []ciTemplate{}
	for _, t := range ci.Templates {
		content, _ := ci.RenderTemplate(t.Name, base, pr.Key)
		ct := ciTemplate{Template: t, Content: content}
		if base != "" {
			ct.URL = base + "/api/v1/ci/templates/" + t.Name
		}
		out = append(out, ct)
	}
	writeJSON(w, http.StatusOK, map[string]any{"server_url": base, "project_key": pr.Key, "project_id": pr.ID,
		"variables": vars, "templates": out, "token_role": rbac.RoleTester})
}

// handleCITemplate: GET /ci/templates/{name} (public) — a template with
// the server URL filled in (?project= sets the project key).
func (s *Server) handleCITemplate(w http.ResponseWriter, r *http.Request) {
	key := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("project")))
	if len(key) > 20 || strings.IndexFunc(key, func(c rune) bool { return !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') }) >= 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid project key")
		return
	}
	content, ok := ci.RenderTemplate(r.PathValue("name"), s.baseURL(), key)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "no such template")
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if queryBool(r, "download") {
		w.Header().Set("Content-Disposition", `attachment; filename="`+r.PathValue("name")+`"`)
	}
	_, _ = w.Write([]byte(content))
}

// ---------------------------------------------------------------- settings

func (s *Server) handlePutCISettings(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var in struct {
		CI           model.CISettings `json:"ci"`
		CIToken      string           `json:"ci_token"`
		ClearCIToken bool             `json:"clear_ci_token"`
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	cs, err := s.Store.SaveCISettings(r.Context(), p.TenantID(), pr.ID, in.CI, in.CIToken, in.ClearCIToken, p.User.Email)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	details := fmt.Sprintf("CI posting: provider %q, project %q, status %v, note %v", cs.CI.Provider, cs.CI.Project, cs.CI.PostStatus, cs.CI.PostNote)
	switch {
	case in.ClearCIToken:
		details += "; token removed"
	case in.CIToken != "":
		details += "; token set"
	}
	s.audit(r.Context(), p, "project.ci_settings", "project", pr.ID, details)
	writeJSON(w, http.StatusOK, s.codeSettingsView(pr.ID, cs))
}

func (s *Server) handleCIDeliveries(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.Store.ListCIDeliveries(r.Context(), p.TenantID(), pr.ID, limit)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

// ---------------------------------------------------------------- server-side posting

// queueCIDelivery adds a status post for results uploaded into a run with
// a commit (?commit=, ?merge_request=) when the project's CI settings post
// statuses or notes, and tries it at once in the background.
func (s *Server) queueCIDelivery(r *http.Request, p *Principal, run *model.TestRun, build *model.Build) {
	commit := strings.TrimSpace(r.URL.Query().Get("commit"))
	if commit == "" && build != nil {
		commit = build.CommitSHA
	}
	if commit == "" {
		return
	}
	cs, err := s.Store.CodeSettings(r.Context(), p.TenantID(), run.ProjectID)
	if err != nil || !cs.CI.Enabled() || cs.CIToken == "" {
		return
	}
	mr := strings.TrimLeft(strings.TrimSpace(r.URL.Query().Get("merge_request")), "!#")
	if _, err := strconv.Atoi(mr); mr != "" && err != nil {
		mr = ""
	}
	if mr == "" && build != nil && cs.CI.PostNote {
		mr, _ = s.Store.FindCommitMergeRequest(r.Context(), p.TenantID(), run.ProjectID, build.Branch)
	}
	d := &model.CIDelivery{ProjectID: run.ProjectID, RunID: run.ID, CommitSHA: commit, MergeRequest: mr, CreatedBy: p.User.Email}
	if build != nil {
		d.BuildID = build.ID
	}
	tenant := p.TenantID()
	store.AfterCommit(r.Context(), func() {
		id, _, err := s.Store.AddCIDelivery(context.Background(), tenant, d)
		if err != nil {
			s.Log.Warn("ci queue", "project", run.ProjectID, "err", err)
			return
		}
		s.hooks.Add(1)
		go func() {
			defer s.hooks.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			_ = s.deliverCI(ctx, id)
		}()
	})
}

// ciPrincipal is the user who uploaded the results of a delivery.
func (s *Server) ciPrincipal(ctx context.Context, tenantID, email, projectID string) (*Principal, error) {
	u, err := s.Store.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("the user %s who uploaded the results no longer exists", email)
	}
	p, err := s.principalFor(ctx, tenantID, u.ID)
	if err != nil {
		return nil, err
	}
	acc, err := s.projectAccess(ctx, p, projectID)
	if err != nil {
		return nil, err
	}
	if acc == nil || !slices.Contains(acc.Permissions, string(rbac.ProjectRead)) {
		return nil, fmt.Errorf("the user %s may no longer read the project", email)
	}
	p.scope(projectID, acc)
	return p, nil
}

// ciBackoff is the wait after the n-th failed attempt.
func ciBackoff(n int, retryAfter time.Duration) time.Duration { return chatBackoff(n, retryAfter) }

// deliverCI makes one attempt at a due delivery: the commit status
// qtestframe/tests (the run's results), qtestframe/gate when the settings
// name a release or conditions, and the merge request note.
func (s *Server) deliverCI(ctx context.Context, id string) error {
	job, err := s.Store.ClaimCIDelivery(ctx, id, ciLease)
	if err != nil || job == nil {
		return err
	}
	d := job.Delivery
	o := store.CIOutcome{}
	var te *tracker.Error
	err = func() error {
		cs, err := s.Store.CodeSettings(ctx, job.TenantID, d.ProjectID)
		if err != nil {
			return err
		}
		if !cs.CI.Enabled() || cs.CIToken == "" {
			return errPermanent("posting is turned off or has no token")
		}
		p, err := s.ciPrincipal(ctx, job.TenantID, d.CreatedBy, d.ProjectID)
		if err != nil {
			return errPermanent(err.Error())
		}
		pr, err := s.Store.GetProject(ctx, job.TenantID, d.ProjectID)
		if err != nil {
			return errPermanent(err.Error())
		}
		r := jobRequest(ctx)
		results, err := s.gateReport(r, p, pr, &gateReq{Run: d.RunID, Commit: d.CommitSHA, MergeRequest: d.MergeRequest, SummaryOnly: true})
		if err != nil {
			return errPermanent(err.Error())
		}
		var gate *ci.Report
		if cs.CI.ReleaseID != "" || len(cs.CI.Conditions) > 0 {
			gr := &gateReq{Run: d.RunID, Commit: d.CommitSHA, MergeRequest: d.MergeRequest, Release: cs.CI.ReleaseID, Require: cs.CI.Conditions}
			if gate, err = s.gateReport(r, p, pr, gr); err != nil {
				return errPermanent(err.Error())
			}
		}
		host := tracker.NewGitLabHost(cs.CI.APIURL, cs.CI.Project, cs.CIToken, tracker.GitLabPrivateToken, s.webhookClient())
		var done []string
		if cs.CI.PostStatus {
			for _, x := range []struct {
				name string
				rep  *ci.Report
			}{{ciStatusTests, results}, {ciStatusGate, gate}} {
				if x.rep == nil {
					continue
				}
				state := tracker.StatusSuccess
				if !x.rep.Pass {
					state = tracker.StatusFailed
				}
				if err := host.SetStatus(ctx, tracker.CommitStatus{SHA: d.CommitSHA, State: state, Name: x.name, TargetURL: x.rep.URL, Description: x.rep.StatusDescription()}); err != nil {
					return err
				}
				done = append(done, x.name+" "+state)
			}
		}
		if cs.CI.PostNote && d.MergeRequest != "" {
			note := results
			if gate != nil {
				note = gate
			}
			updated, err := host.UpsertNote(ctx, d.MergeRequest, ci.NoteMarker(pr.Key), note.Markdown())
			if err != nil {
				return err
			}
			done = append(done, map[bool]string{true: "note !" + d.MergeRequest + " updated", false: "note !" + d.MergeRequest + " added"}[updated])
		}
		o.Outcome = strings.Join(done, "; ")
		return nil
	}()
	attempt := d.Attempts + 1
	if err != nil {
		o.Error = err.Error()
		var perm *permanentError
		retry := !errors.As(err, &perm) && attempt < ciMaxAttempts
		var wait time.Duration
		if errors.As(err, &te) {
			o.StatusCode = te.Status
			// a refused token or project is not retried; rate limits are
			if te.Status >= 400 && te.Status < 500 && te.Status != http.StatusTooManyRequests {
				retry = false
			}
			wait = te.RetryAfter
		}
		if retry {
			at := s.Store.Now().Add(ciBackoff(attempt, wait))
			o.RetryAt = &at
		}
		s.Log.Warn("ci delivery", "project", d.ProjectID, "commit", d.CommitSHA, "attempt", attempt, "err", o.Error)
	} else {
		o.OK, o.StatusCode = true, http.StatusOK
	}
	if ferr := s.Store.FinishCIDelivery(context.Background(), id, o); ferr != nil {
		s.Log.Warn("ci record", "err", ferr)
	}
	return err
}

type permanentError struct{ msg string }

func (e *permanentError) Error() string { return e.msg }

func errPermanent(msg string) error { return &permanentError{msg: msg} }

// RunCIJobs retries the CI deliveries whose backoff has passed (called
// every minute by RunJobs).
func (s *Server) RunCIJobs(ctx context.Context) (int, error) {
	due, err := s.Store.DueCIDeliveries(ctx, 200)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range due {
		if ctx.Err() != nil {
			break
		}
		actx, cancel := context.WithTimeout(ctx, time.Minute)
		if err := s.deliverCI(actx, id); err == nil {
			n++
		}
		cancel()
	}
	return n, nil
}
