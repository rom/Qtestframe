package app

import (
	"context"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/reqlang"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/tracerules"
)

// SeedDemo creates a demonstration project with folders, requirements,
// evaluation criteria, test cases, links and a test run.
func SeedDemo(ctx context.Context, live *store.Store, tenantID string) (*model.Project, error) {
	// The project is created SeedHistoryDays ago and gets a history of
	// nightly runs (trends, burn-up, flaky tests); the last run is today's.
	clock := &seedClock{live: live}
	clock.at = live.Now().AddDate(0, 0, -SeedHistoryDays)
	st := live.WithClock(clock.now)
	key := "DEMO"
	ps, err := st.ListProjects(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	for i := 2; ; i++ {
		clash := false
		for _, p := range ps {
			if p.Key == key {
				clash = true
			}
		}
		if !clash {
			break
		}
		key = "DEMO" + itoa(i)
	}
	p := &model.Project{Key: key, Name: "Demo – Online Banking", Description: "Sample specification and test suite for a web banking portal."}
	if err := st.CreateProject(ctx, tenantID, p, true); err != nil {
		return nil, err
	}
	cats, err := st.ListCategories(ctx, tenantID, p.ID, "")
	if err != nil {
		return nil, err
	}
	cat := func(dim, name string) string {
		for _, c := range cats {
			if c.Dimension == dim && c.Name == name {
				return c.ID
			}
		}
		return ""
	}
	// top-level folders get a colour per area (shown on their items); the
	// same area has the same colour for requirements and tests
	folderColor := map[string]string{"Authentication": "#dc2626", "Payments": "#0d9488", "Operations": "#475569"}
	folder := func(kind, name string, parent *string) (*model.Folder, error) {
		f := &model.Folder{ProjectID: p.ID, Kind: kind, Name: name, ParentID: parent}
		if parent == nil {
			f.Color = folderColor[name]
		}
		return f, st.CreateFolder(ctx, tenantID, f)
	}
	rf, err := folder(model.KindRequirement, "Authentication", nil)
	if err != nil {
		return nil, err
	}
	rf2, err := folder(model.KindRequirement, "Payments", nil)
	if err != nil {
		return nil, err
	}
	rf3, err := folder(model.KindRequirement, "Operations", nil)
	if err != nil {
		return nil, err
	}
	tf, err := folder(model.KindTest, "Authentication", nil)
	if err != nil {
		return nil, err
	}
	tf2, err := folder(model.KindTest, "Negative", &tf.ID)
	if err != nil {
		return nil, err
	}
	tf3, err := folder(model.KindTest, "Payments", nil)
	if err != nil {
		return nil, err
	}

	type reqSpec struct {
		folder   *model.Folder
		title    string
		desc     string
		typ      string
		prio     string
		status   string
		area     string
		criteria []model.EvaluationCriterion
	}
	specs := []reqSpec{
		{rf, "Password login", "The system MUST authenticate users with e-mail address and password.", "functional", "critical", "approved", "Security",
			[]model.EvaluationCriterion{{Description: "Valid credentials grant access", Method: "test"}, {Description: "Invalid credentials are rejected with a generic message", Method: "test"}}},
		{rf, "Account lockout", "The system MUST NOT allow more than 10 failed login attempts per account within 5 minutes.", "security", "high", "approved", "Security",
			[]model.EvaluationCriterion{{Description: "11th attempt within 5 minutes is refused", Method: "test", Threshold: "10 attempts / 5 min"}}},
		{rf, "Single sign-on", "The system SHOULD support OpenID Connect and SAML 2.0 single sign-on.", "interface", "medium", "review", "Security",
			[]model.EvaluationCriterion{{Description: "Login through corporate IdP succeeds", Method: "demonstration"}}},
		{rf2, "Transfer limits", "The system SHALL reject transfers above the daily limit of the account.", "functional", "high", "approved", "Core",
			[]model.EvaluationCriterion{{Description: "Transfer above limit is rejected", Method: "test"}, {Description: "Limit configuration reviewed", Method: "inspection"}}},
		{rf2, "Transfer response time", "Transfers SHOULD be confirmed within 2 seconds for 95% of requests.", "performance", "medium", "draft", "Core",
			[]model.EvaluationCriterion{{Description: "p95 latency", Method: "analysis", Threshold: "<= 2 s"}}},
		{rf2, "Statement export", "Users MAY export account statements as PDF or CSV.", "functional", "low", "draft", "UI", nil},
		{rf3, "Central logging", "The system MUST send audit logs to a central syslog server.", "non_functional", "high", "implemented", "Operations",
			[]model.EvaluationCriterion{{Description: "Audit events visible on log server", Method: "demonstration"}}},
		{rf3, "Architecture overview", "This section describes the deployment architecture of the portal.", "constraint", "low", "approved", "Operations", nil},
	}
	reqs := make([]*model.Requirement, 0, len(specs))
	for _, sp := range specs {
		r := &model.Requirement{ProjectID: p.ID, FolderID: &sp.folder.ID, Title: sp.title, Description: sp.desc,
			Obligation: reqlang.DetectObligation(sp.desc), Type: sp.typ, Priority: sp.prio, Status: sp.status,
			CategoryIDs: []string{cat("Area", sp.area)}, Criteria: sp.criteria, CreatedBy: "seed", Source: seedSource(sp.title),
			// a tag per area, so the tag cloud has something to show
			Tags: []string{strings.ToLower(sp.area)}}
		if strings.HasPrefix(sp.desc, "This section describes") {
			r.Obligation = model.ObligationDescribe
		}
		if err := st.CreateRequirement(ctx, tenantID, r); err != nil {
			return nil, err
		}
		reqs = append(reqs, r)
		clock.advance(18 * time.Hour)
	}

	type testSpec struct {
		folder *model.Folder
		title  string
		exec   string
		cats   [][2]string
		steps  []model.TestStep
		script string
		links  []int
	}
	tspecs := []testSpec{
		{tf, "Login with valid credentials", model.ExecManual, [][2]string{{"Polarity", "Positive"}, {"Interface", "External"}, {"Execution", "Manual"}, {"Level", "System"}},
			[]model.TestStep{{Action: "Open the login page", Expected: "Login form is shown"}, {Action: "Enter valid e-mail and password", Data: "alice@example.com / ********"}, {Action: "Press Sign in", Expected: "Dashboard is shown"}}, "", []int{0}},
		{tf2, "Login with wrong password", model.ExecManual, [][2]string{{"Polarity", "Negative"}, {"Interface", "External"}, {"Execution", "Manual"}},
			[]model.TestStep{{Action: "Enter a valid e-mail and a wrong password", Expected: "Generic error message, no account hint"}}, "", []int{0}},
		{tf2, "Lockout after repeated failures", model.ExecAutomated, [][2]string{{"Polarity", "Negative"}, {"Medium", "Network based"}, {"Execution", "Automated"}},
			nil, "#!/bin/sh\n# simulated brute force check\nfor i in $(seq 1 11); do :; done\necho \"11th attempt rejected\"\nexit 0\n", []int{1}},
		{tf3, "Reject transfer above daily limit", model.ExecManual, [][2]string{{"Polarity", "Negative"}, {"Level", "Acceptance"}},
			[]model.TestStep{{Action: "Set daily limit to 1000", Expected: "Limit saved"}, {Action: "Transfer 1500", Expected: "Transfer rejected with limit message"}}, "", []int{3}},
		{tf3, "Statement CSV file format", model.ExecAutomated, [][2]string{{"Polarity", "Positive"}, {"Medium", "File based"}, {"Execution", "Automated"}},
			nil, "#!/bin/sh\nprintf 'date,amount\\n2026-01-01,10\\n' > statement.csv\ngrep -q '^date,amount' statement.csv && echo 'header ok'\n", []int{5}},
	}
	var testIDs []string
	for _, ts := range tspecs {
		t := &model.TestCase{ProjectID: p.ID, FolderID: &ts.folder.ID, Title: ts.title, ExecutionType: ts.exec,
			Steps: ts.steps, Script: ts.script, Status: "ready", CreatedBy: "seed", Tags: []string{ts.exec}}
		if ts.script != "" {
			t.ScriptLanguage = "sh"
		}
		for _, c := range ts.cats {
			if id := cat(c[0], c[1]); id != "" {
				t.CategoryIDs = append(t.CategoryIDs, id)
			}
		}
		if err := st.CreateTest(ctx, tenantID, t); err != nil {
			return nil, err
		}
		testIDs = append(testIDs, t.ID)
		for _, li := range ts.links {
			if err := st.CreateLink(ctx, tenantID, &model.TraceLink{RequirementID: reqs[li].ID, TestCaseID: t.ID}); err != nil {
				return nil, err
			}
		}
	}
	// prerequisites: the negative login tests need a working login first
	for _, d := range [][2]int{{1, 0}, {2, 1}} {
		if err := st.AddTestDependency(ctx, tenantID, &model.TestDependency{TestID: testIDs[d[0]], DependsOnID: testIDs[d[1]], CreatedBy: "seed"}); err != nil {
			return nil, err
		}
	}
	// requirement relations for the dependency graph
	for _, rel := range []struct {
		src, dst int
		typ      string
	}{{1, 0, "depends_on"}, {2, 0, "provides"}, {3, 0, "depends_on"}, {4, 3, "refines"}, {6, 7, "derived_from"}} {
		if rel.src >= len(reqs) || rel.dst >= len(reqs) {
			continue
		}
		if err := st.CreateRelation(ctx, tenantID, &model.Relation{SourceID: reqs[rel.src].ID, TargetID: reqs[rel.dst].ID, Type: rel.typ, CreatedBy: "seed"}); err != nil {
			return nil, err
		}
	}
	if err := seedHistory(ctx, st, clock, tenantID, p.ID, reqs, testIDs); err != nil {
		return nil, err
	}
	clock.at = time.Time{} // today: the live clock
	// traceability rules (see Traceability → Rules)
	for _, tr := range []model.TraceRule{
		{Name: "Every MUST requirement is verified by a test", Description: "Mandatory requirements (MUST, SHALL) need at least one linked test.",
			Severity: "error", Scope: model.KindRequirement, Selector: "obligation:must OR obligation:shall", Kind: model.TraceVerifiedBy, Min: 1},
		{Name: "No suspect links on MUST requirements", Description: "Review the tests of a mandatory requirement after it changed.",
			Severity: "warning", Scope: model.KindRequirement, Selector: "obligation:must OR obligation:shall", Kind: model.TraceVerifiedBy,
			TraceRuleOptions: model.TraceRuleOptions{NotSuspect: true, Every: true}},
		{Name: "Every test verifies a requirement", Severity: "warning", Scope: model.KindTest, Kind: model.TraceVerifies, Min: 1},
		{Name: "Every MUST requirement has a passing test", Description: "Before a release, each mandatory requirement needs at least one linked test whose last result passed.",
			Severity: "warning", Scope: model.KindRequirement, Selector: "obligation:must OR obligation:shall", Kind: model.TraceVerifiedBy, Min: 1,
			TraceRuleOptions: model.TraceRuleOptions{Passed: true}},
	} {
		tr.ProjectID, tr.Enabled, tr.CreatedBy = p.ID, true, "seed"
		if err := st.CreateTraceRule(ctx, tenantID, &tr); err != nil {
			return nil, err
		}
	}
	// the built-in consistency checks, with something for them to find: a
	// near-duplicate of "Account lockout" and a high risk nobody mitigates
	for _, b := range tracerules.Builtins {
		rule := tracerules.BuiltinRule(b, p.ID)
		rule.CreatedBy = "seed"
		if err := st.CreateTraceRule(ctx, tenantID, &rule); err != nil {
			return nil, err
		}
	}
	dup := &model.Requirement{ProjectID: p.ID, FolderID: &rf.ID, Title: "Account lockout after failed logins",
		Description: "The system MUST NOT allow more than 10 failed login attempts per account within 5 minutes.", Obligation: model.ObligationMustNot,
		Type: "security", Priority: "high", Status: "draft", CategoryIDs: []string{cat("Area", "Security")}, CreatedBy: "seed", Tags: []string{"security"}}
	if err := st.CreateRequirement(ctx, tenantID, dup); err != nil {
		return nil, err
	}
	// Duplicates & conflicts: a requirement that says "Password login" in
	// other words, two that contradict existing ones (a threshold and an
	// opposite obligation) and a test that repeats "Login with valid
	// credentials"
	for _, x := range []struct {
		folder                 *model.Folder
		title, desc, typ, area string
	}{
		{rf, "Password sign-in", "Users SHALL log in with their e-mail address and password.", "functional", "Security"},
		{rf2, "Transfer confirmation delay", "Transfers MUST be confirmed in no less than 5 seconds, so that the fraud checks can finish.", "performance", "Core"},
		{rf2, "No statement export", "Users MUST NOT export account statements.", "security", "UI"},
	} {
		r := &model.Requirement{ProjectID: p.ID, FolderID: &x.folder.ID, Title: x.title, Description: x.desc, Obligation: reqlang.DetectObligation(x.desc),
			Type: x.typ, Priority: "medium", Status: "draft", CategoryIDs: []string{cat("Area", x.area)}, CreatedBy: "seed", Tags: []string{strings.ToLower(x.area)}}
		if err := st.CreateRequirement(ctx, tenantID, r); err != nil {
			return nil, err
		}
	}
	if err := st.CreateTest(ctx, tenantID, &model.TestCase{ProjectID: p.ID, FolderID: &tf.ID, Title: "Sign in with a valid password", ExecutionType: model.ExecManual,
		Steps:  []model.TestStep{{Action: "Open the sign-in page", Expected: "Login form is shown"}, {Action: "Enter a valid email and password"}, {Action: "Press Sign in", Expected: "Dashboard is shown"}},
		Status: "draft", CreatedBy: "seed", Tags: []string{model.ExecManual}}); err != nil {
		return nil, err
	}
	if err := st.CreateRisk(ctx, tenantID, &model.Risk{ProjectID: p.ID, Title: "Credential stuffing takes over accounts", Category: "security",
		Likelihood: 4, Impact: 5, Owner: "security team", CreatedBy: "seed"}); err != nil {
		return nil, err
	}
	run := &model.TestRun{ProjectID: p.ID, Name: "Sprint 12 regression", Environment: "staging", Build: "1.4.0-rc1", CreatedBy: "seed"}
	if err := st.CreateRun(ctx, tenantID, run, testIDs); err != nil {
		return nil, err
	}
	es, err := st.ListExecutions(ctx, tenantID, run.ID)
	if err != nil {
		return nil, err
	}
	for i, e := range es {
		status := []string{model.ResultPassed, model.ResultFailed, model.ResultPassed}
		if i >= len(status) {
			break
		}
		s := status[i]
		notes := ""
		if s == model.ResultFailed {
			notes = "Error message reveals that the account exists."
		}
		if _, err := st.UpdateExecution(ctx, tenantID, e.ID, "seed", store.ExecutionUpdate{Status: &s, Notes: &notes}); err != nil {
			return nil, err
		}
	}
	if err := seedConfigurations(ctx, st, tenantID, p, tf, reqs[0], testIDs[0]); err != nil {
		return nil, err
	}
	tf4, err := folder(model.KindTest, "Operations", nil)
	if err != nil {
		return nil, err
	}
	if err := seedCoverageTests(ctx, st, tenantID, p.ID, reqs, tf, tf3, tf4); err != nil {
		return nil, err
	}
	// change impact, stale results, blocking defects and a regression (seedcorrelations.go)
	var changed *model.Requirement
	if len(reqs) > 4 {
		changed = reqs[3] // reqs[4] refines it
	}
	if err := seedCorrelations(ctx, st, tenantID, run, changed); err != nil {
		return nil, err
	}
	if err := seedVerification(ctx, st, tenantID, p, reqs, testIDs); err != nil {
		return nil, err
	}
	if err := seedWaivers(ctx, st, tenantID, p, reqs); err != nil {
		return nil, err
	}
	if err := seedVariantsAndCode(ctx, st, tenantID, p, reqs, testIDs, run); err != nil {
		return nil, err
	}
	if err := seedCustomFields(ctx, st, tenantID, p.ID, reqs); err != nil {
		return nil, err
	}
	return p, nil
}

// seedCustomFields gives the demo project custom fields (with values on
// some requirements) and a project template for new requirements.
func seedCustomFields(ctx context.Context, st *store.Store, tenantID, projectID string, reqs []*model.Requirement) error {
	for _, f := range []model.CustomField{
		{EntityType: model.KindRequirement, Label: "Component", Type: model.FieldSingleSelect, Options: []string{"Web portal", "Mobile app", "Core banking"}, ShowInList: true,
			Help: "The part of the product the requirement is about."},
		{EntityType: model.KindRequirement, Label: "Effort", Type: model.FieldNumber, Unit: "d", Help: "Estimated implementation effort in person-days."},
		{EntityType: model.KindRequirement, Label: "Contract clause", Type: model.FieldText},
		{EntityType: model.KindTest, Label: "Test environment", Type: model.FieldSingleSelect, Options: []string{"staging", "pre-production"}, Default: "staging"},
		{EntityType: model.KindDefect, Label: "Found in build", Type: model.FieldText},
	} {
		f.ProjectID = projectID
		if err := st.CreateCustomField(ctx, tenantID, &f); err != nil {
			return err
		}
	}
	values := []model.CustomValues{
		{"component": "Web portal", "effort": 3.0, "contract_clause": "4.2.1"},
		{"component": "Web portal", "effort": 5.0},
		{"component": "Core banking", "effort": 8.0, "contract_clause": "6.1"},
		{"component": "Core banking", "effort": 2.0},
	}
	for i, v := range values {
		if i >= len(reqs) {
			break
		}
		r, err := st.GetRequirement(ctx, tenantID, reqs[i].ID)
		if err != nil {
			return err
		}
		r.CustomFields = v
		if err := st.UpdateRequirement(ctx, tenantID, r, false); err != nil {
			return err
		}
	}
	tpl := &model.ItemTemplate{ProjectID: projectID, Kind: model.KindRequirement, Name: "Banking security requirement", CreatedBy: "seed",
		Description: "A MUST requirement with an abuse case and a review criterion.",
		Content: model.TemplateContent{Title: "The portal shall protect <asset>", Obligation: model.ObligationMust, Type: "security", Priority: "high",
			Description: "The portal MUST <control> so that <threat> is prevented.\n\n## Abuse case\n<How an attacker would try>",
			Tags:        []string{"security"},
			Criteria: []model.EvaluationCriterion{{Description: "Abuse case is rejected", Method: "test"},
				{Description: "Control reviewed by the security officer", Method: "review"}},
			CustomFields: model.CustomValues{"component": "Web portal"}}}
	// not the default: "New requirement" offers it in the template picker
	return st.CreateTemplate(ctx, tenantID, tpl)
}

// seedCoverageTests gives every demo requirement without a test one that
// verifies it (not yet run), so every requirement has a related test.
func seedCoverageTests(ctx context.Context, st *store.Store, tenantID, projectID string, reqs []*model.Requirement, auth, payments, ops *model.Folder) error {
	specs := []struct {
		req    string
		folder *model.Folder
		title  string
		exec   string
		steps  []model.TestStep
		script string
	}{
		{"Single sign-on", auth, "Sign in through the corporate identity provider", model.ExecManual, []model.TestStep{
			{Action: "Choose Sign in with company account on the login page", Expected: "The identity provider's sign-in page opens"},
			{Action: "Sign in at the identity provider with a test account", Expected: "The portal dashboard is shown for the same user"},
			{Action: "Repeat with the SAML 2.0 connection", Expected: "Sign-in succeeds the same way"},
		}, ""},
		{"Transfer response time", payments, "Transfer confirmation time under load", model.ExecAutomated, nil,
			"#!/bin/sh\n# simulated load test: 100 transfers, report the 95th percentile\necho \"p95 confirmation time: 1.4 s (limit 2 s)\"\nexit 0\n"},
		{"Central logging", ops, "Audit events reach the central log server", model.ExecManual, []model.TestStep{
			{Action: "Sign in and change a requirement", Expected: "The change is saved"},
			{Action: "Search the central syslog server for the audit event", Expected: "The event is found within 1 minute with user, time and action"},
		}, ""},
		{"Architecture overview", ops, "Review of the deployment architecture description", model.ExecManual, []model.TestStep{
			{Action: "Compare the architecture description with the deployed environment", Expected: "Every component and connection in the description exists in the deployment"},
		}, ""},
	}
	byTitle := map[string]*model.Requirement{}
	for _, r := range reqs {
		byTitle[r.Title] = r
	}
	for _, sp := range specs {
		r := byTitle[sp.req]
		if r == nil {
			continue
		}
		t := &model.TestCase{ProjectID: projectID, FolderID: &sp.folder.ID, Title: sp.title, ExecutionType: sp.exec,
			Steps: sp.steps, Script: sp.script, Status: "ready", CreatedBy: "seed", Tags: []string{sp.exec}}
		if sp.script != "" {
			t.ScriptLanguage = "sh"
		}
		if err := st.CreateTest(ctx, tenantID, t); err != nil {
			return err
		}
		if err := st.CreateLink(ctx, tenantID, &model.TraceLink{RequirementID: r.ID, TestCaseID: t.ID}); err != nil {
			return err
		}
	}
	return nil
}

// seedConfigurations adds browser/OS configurations, a data-driven login
// test and a cross-browser run with some results.
func seedConfigurations(ctx context.Context, st *store.Store, tenantID string, p *model.Project, folder *model.Folder, req *model.Requirement, loginTest string) error {
	var cfgs []model.ConfigurationRef
	for _, v := range [][2]string{{"Chrome", "Windows"}, {"Firefox", "Linux"}, {"Safari", "macOS"}} {
		c := &model.Configuration{ProjectID: p.ID, Values: []model.DimensionValue{{Dimension: "Browser", Value: v[0]}, {Dimension: "OS", Value: v[1]}}, CreatedBy: "seed"}
		if err := st.CreateConfiguration(ctx, tenantID, c); err != nil {
			return err
		}
		cfgs = append(cfgs, model.ConfigurationRef{ID: c.ID, Name: c.Name})
	}
	dd := &model.TestCase{ProjectID: p.ID, FolderID: &folder.ID, Title: "Login with different account types", ExecutionType: model.ExecManual, Status: "ready", CreatedBy: "seed",
		Description: "Data-driven: runs once per row of the test data.",
		Steps: []model.TestStep{
			{Action: "Open the login page and sign in as {{email}} with password {{password}}", Expected: "Sign-in succeeds"},
			{Action: "Open the account menu", Expected: "The role {{role}} is shown"},
		},
		Parameters: []string{"email", "password", "role"},
		DataRows: []model.DataRow{
			{Name: "Customer", Values: map[string]string{"email": "carol@example.com", "password": "Customer-123", "role": "Customer"}},
			{Name: "Business", Values: map[string]string{"email": "bob@example.com", "password": "Business-123", "role": "Business account"}},
			{Name: "Administrator", Values: map[string]string{"email": "admin@example.com", "password": "Admin-123", "role": "Administrator"}},
		}}
	if err := st.CreateTest(ctx, tenantID, dd); err != nil {
		return err
	}
	if err := st.CreateLink(ctx, tenantID, &model.TraceLink{RequirementID: req.ID, TestCaseID: dd.ID}); err != nil {
		return err
	}
	run := &model.TestRun{ProjectID: p.ID, Name: "Cross-browser smoke", Environment: "staging", Build: "1.4.0-rc1", CreatedBy: "seed", Configurations: cfgs}
	if err := st.CreateRun(ctx, tenantID, run, []string{loginTest, dd.ID}); err != nil {
		return err
	}
	es, err := st.ListExecutions(ctx, tenantID, run.ID)
	if err != nil {
		return err
	}
	passed := model.ResultPassed
	for _, e := range es {
		switch {
		case e.TestCaseID == loginTest && e.ConfigurationName != "Safari / macOS":
			if _, err := st.UpdateExecution(ctx, tenantID, e.ID, "seed", store.ExecutionUpdate{Status: &passed}); err != nil {
				return err
			}
		case e.TestCaseID == dd.ID && e.ConfigurationName == "Chrome / Windows":
			// every iteration passes except the administrator's role label
			full, err := st.GetExecution(ctx, tenantID, e.ID)
			if err != nil {
				return err
			}
			admin := full.Iterations[len(full.Iterations)-1].ID
			var srs []model.StepResult
			for i, sr := range full.StepResults {
				status := model.ResultPassed
				if sr.IterationID != nil && *sr.IterationID == admin && i == len(full.StepResults)-1 {
					status = model.ResultFailed
					sr.Actual = "The role is shown as \"Admin\""
				}
				srs = append(srs, model.StepResult{ID: sr.ID, Status: status, Actual: sr.Actual})
			}
			if _, err := st.UpdateExecution(ctx, tenantID, e.ID, "seed", store.ExecutionUpdate{StepResults: srs}); err != nil {
				return err
			}
		}
	}
	return nil
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// DemoWebTest is the script of the demo's web test (Playwright): it opens
// the login page of the server itself.
const DemoWebTest = `// the base URL is the project's (Settings → Web tests)
await page.goto('/login');
await expect(page).toHaveTitle(/Qtestframe/i);
await expect(page.getByRole('heading', { level: 1 })).toHaveText(/Sign in|Logga in/);
`

// SeedDemoWeb adds a web test that runs against baseURL (the demo server
// itself, server.base_url) and a run "Web smoke" with it, and sets the
// project's web test base URL and an example variable.
func SeedDemoWeb(ctx context.Context, st *store.Store, tenantID string, p *model.Project, baseURL string) error {
	f := &model.Folder{ProjectID: p.ID, Kind: model.KindTest, Name: "Web"}
	if err := st.CreateFolder(ctx, tenantID, f); err != nil {
		return err
	}
	t := &model.TestCase{ProjectID: p.ID, FolderID: &f.ID, Title: "Login page shows the sign-in form", ExecutionType: model.ExecAutomated,
		ScriptLanguage: "playwright", Script: DemoWebTest, Status: "ready", CreatedBy: "seed", Tags: []string{model.ExecAutomated, "web"},
		Description: "Web test (Playwright) against the server itself: press Run script in the execution runner."}
	if err := st.CreateTest(ctx, tenantID, t); err != nil {
		return err
	}
	reqs, err := st.ListRequirements(ctx, tenantID, p.ID)
	if err != nil {
		return err
	}
	for _, r := range reqs {
		if r.Title == "Password login" {
			if err := st.CreateLink(ctx, tenantID, &model.TraceLink{RequirementID: r.ID, TestCaseID: t.ID}); err != nil {
				return err
			}
		}
	}
	set := &model.WebTestSettings{BaseURL: strings.TrimRight(baseURL, "/"),
		Variables: []model.TestVariable{{Name: "USERNAME", Value: "admin@example.com"}}}
	if set.BaseURL != "" && !store.ValidBaseURL(set.BaseURL) {
		set.BaseURL = ""
	}
	if err := st.SaveWebTestSettings(ctx, tenantID, p.ID, set); err != nil {
		return err
	}
	run := &model.TestRun{ProjectID: p.ID, Name: "Web smoke", Environment: "local", CreatedBy: "seed"}
	return st.CreateRun(ctx, tenantID, run, []string{t.ID})
}
