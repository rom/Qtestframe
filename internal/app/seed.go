package app

import (
	"context"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/reqlang"
	"github.com/rom/qtestframe/internal/store"
)

// SeedDemo creates a demonstration project with folders, requirements,
// evaluation criteria, test cases, links and a test run.
func SeedDemo(ctx context.Context, st *store.Store, tenantID string) (*model.Project, error) {
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
	folder := func(kind, name string, parent *string) (*model.Folder, error) {
		f := &model.Folder{ProjectID: p.ID, Kind: kind, Name: name, ParentID: parent}
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
			CategoryIDs: []string{cat("Area", sp.area)}, Criteria: sp.criteria, CreatedBy: "seed", Source: "Customer RFP v1.2"}
		if strings.HasPrefix(sp.desc, "This section describes") {
			r.Obligation = model.ObligationDescribe
		}
		if err := st.CreateRequirement(ctx, tenantID, r); err != nil {
			return nil, err
		}
		reqs = append(reqs, r)
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
			Steps: ts.steps, Script: ts.script, Status: "ready", CreatedBy: "seed"}
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
	return p, nil
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
