package store_test

import (
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

func TestParseGateConditions(t *testing.T) {
	ok := map[string]model.ExitCriterion{
		"coverage>=80":              {Type: "coverage", Threshold: 80},
		"coverage":                  {Type: "coverage", Threshold: 100},
		" passed >= 95.5% ":         {Type: "passed", Threshold: 95.5},
		"must":                      {Type: "mandatory_passed"},
		"blocking":                  {Type: "no_blocking_defects"},
		"No-Stale":                  {Type: "no_stale"},
		"no_open_defects:major":     {Type: "no_open_defects", Level: "major"},
		"risks_mitigated:high":      {Type: "risks_mitigated", Level: "high"},
		"trace_rules":               {Type: "trace_rules", Level: "error"},
		"trace_rules:warning":       {Type: "trace_rules", Level: "warning"},
		"cycle_executed:Sprint 3":   {Type: "cycle_executed", Threshold: 100, CycleID: "Sprint 3"},
		"cycle:CYC-1>=90":           {Type: "cycle_executed", Threshold: 90, CycleID: "CYC-1"},
		"executed>=0":               {Type: "executed"},
		"residual_risk:medium":      {Type: "residual_risk", Level: "medium"},
		"defects":                   {Type: "no_open_defects"},
		"approved>=50":              {Type: "approved", Threshold: 50},
		"suspect":                   {Type: "no_suspect"},
		"failed":                    {Type: "no_failed"},
		"no_blocking_defects":       {Type: "no_blocking_defects"},
		"mandatory_passed":          {Type: "mandatory_passed"},
		"risks":                     {Type: "risks_mitigated"},
		"residual":                  {Type: "residual_risk"},
		"rules:error":               {Type: "trace_rules", Level: "error"},
		"passed_percent>=10":        {Type: "passed", Threshold: 10},
		"must_verified":             {Type: "mandatory_passed"},
		"no_blocking":               {Type: "no_blocking_defects"},
		"stale":                     {Type: "no_stale"},
		"mandatory":                 {Type: "mandatory_passed"},
		"cycle_executed:x>=100":     {Type: "cycle_executed", Threshold: 100, CycleID: "x"},
		"no_open_defects:critical":  {Type: "no_open_defects", Level: "critical"},
		"risks_mitigated:low":       {Type: "risks_mitigated", Level: "low"},
		"residual_risk:critical":    {Type: "residual_risk", Level: "critical"},
		"no_open_defects:trivial":   {Type: "no_open_defects", Level: "trivial"},
		"coverage>=100%":            {Type: "coverage", Threshold: 100},
		"executed>=99.9":            {Type: "executed", Threshold: 99.9},
		"cycle_executed:a:b":        {Type: "cycle_executed", Threshold: 100, CycleID: "a:b"},
		"no_suspect":                {Type: "no_suspect"},
		"no_failed":                 {Type: "no_failed"},
		"approved":                  {Type: "approved", Threshold: 100},
		"risks_mitigated:critical":  {Type: "risks_mitigated", Level: "critical"},
		"no_open_defects:minor":     {Type: "no_open_defects", Level: "minor"},
		"trace_rules:error":         {Type: "trace_rules", Level: "error"},
		"risks_mitigated:medium":    {Type: "risks_mitigated", Level: "medium"},
		"residual_risk:low":         {Type: "residual_risk", Level: "low"},
		"residual_risk:high":        {Type: "residual_risk", Level: "high"},
		"passed>=0":                 {Type: "passed"},
		"cycle_executed:Sprint>=50": {Type: "cycle_executed", Threshold: 50, CycleID: "Sprint"},
	}
	for in, want := range ok {
		got, err := store.ParseGateCondition(in)
		if err != nil || got != want {
			t.Errorf("%q: %+v %v, want %+v", in, got, err, want)
		}
	}
	for _, bad := range []string{"bogus", "coverage>=x", "coverage>=101", "no_failed>=5", "no_failed:x", "cycle_executed", "no_open_defects:huge", "risks_mitigated:x", "trace_rules:info"} {
		if _, err := store.ParseGateCondition(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	list, err := store.ParseGateConditions([]string{"must, no_failed", "", "coverage>=80"})
	if err != nil || len(list) != 3 {
		t.Fatal(list, err)
	}
	if _, err := store.ParseGateConditions([]string{strings.Repeat("must,", 31)}); err == nil {
		t.Error("31 conditions accepted")
	}
	if _, err := store.ParseGateConditions(store.DefaultGateConditions); err != nil {
		t.Error(err)
	}
}

func TestGateStatusAndCIDeliveries(t *testing.T) {
	st, ten, ctx := setup(t)
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	st.SetClock(func() time.Time { return now })
	p := &model.Project{Key: "GT", Name: "Gate"}
	must(t, st.CreateProject(ctx, ten.ID, p, false))
	mkReq := func(title, ob, status string) *model.Requirement {
		r := &model.Requirement{ProjectID: p.ID, Title: title, Obligation: ob, Status: status, CreatedBy: "u"}
		must(t, st.CreateRequirement(ctx, ten.ID, r))
		return r
	}
	r1, r2 := mkReq("Must", "must", "approved"), mkReq("Should", "should", "approved")
	mkReq("Gone", "must", "obsolete")
	mkTest := func(title string) *model.TestCase {
		tc := &model.TestCase{ProjectID: p.ID, Title: title, ExecutionType: model.ExecAutomated, Status: "ready", CreatedBy: "u"}
		must(t, st.CreateTest(ctx, ten.ID, tc))
		return tc
	}
	t1, t2 := mkTest("A"), mkTest("B")
	must(t, st.CreateLink(ctx, ten.ID, &model.TraceLink{ProjectID: p.ID, RequirementID: r1.ID, TestCaseID: t1.ID}))
	must(t, st.CreateLink(ctx, ten.ID, &model.TraceLink{ProjectID: p.ID, RequirementID: r2.ID, TestCaseID: t2.ID}))
	old := &model.TestRun{ProjectID: p.ID, Name: "old", CreatedBy: "u"}
	must(t, st.CreateRun(ctx, ten.ID, old, nil))
	_, _, err := st.ImportResults(ctx, ten.ID, old.ID, "u", []store.ImportResult{{TestKey: t1.Key, Status: "passed"}, {TestKey: t2.Key, Status: "passed"}})
	must(t, err)
	// 100 days later the passes are stale
	now = now.AddDate(0, 0, 100)
	run := &model.TestRun{ProjectID: p.ID, Name: "new", CreatedBy: "u"}
	must(t, st.CreateRun(ctx, ten.ID, run, nil))
	_, _, err = st.ImportResults(ctx, ten.ID, run.ID, "u", []store.ImportResult{{TestKey: t2.Key, Status: "failed"}})
	must(t, err)
	gate := func(q store.GateQuery) *store.ReleaseStatusReport {
		t.Helper()
		q.ProjectID = p.ID
		rep, err := st.GateStatus(ctx, ten.ID, q, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return rep
	}
	crit := func(rep *store.ReleaseStatusReport, typ string) store.CriterionResult {
		for _, c := range rep.Criteria {
			if c.Type == typ {
				return c
			}
		}
		t.Fatalf("no %s", typ)
		return store.CriterionResult{}
	}
	// all runs: t1 stale pass, t2 failed; the obsolete requirement is not in scope
	rep := gate(store.GateQuery{})
	if rep.Go || len(rep.Criteria) != 4 || rep.Summary["requirements"] != 2 || rep.Summary["runs"] != 2 {
		t.Fatalf("%+v", rep.Summary)
	}
	if c := crit(rep, "no_stale"); c.Met || len(c.Items) != 1 || c.Items[0].Key != t1.Key {
		t.Fatalf("%+v", c)
	}
	if c := crit(rep, "no_failed"); c.Met || c.Items[0].Key != t2.Key {
		t.Fatalf("%+v", c)
	}
	if c := crit(rep, "mandatory_passed"); c.Met || c.Items[0].Note != "stale" {
		t.Fatalf("%+v", c)
	}
	// only the new run: t1 has no result there
	rep = gate(store.GateQuery{RunID: run.ID, Criteria: []model.ExitCriterion{{Type: "no_stale"}, {Type: "executed", Threshold: 100}}})
	if c := crit(rep, "no_stale"); !c.Met || rep.Summary["runs"] != 1 || crit(rep, "executed").Total != 1 {
		t.Fatalf("%+v", rep.Criteria)
	}
	// an ad-hoc scope without the failing requirement
	rep = gate(store.GateQuery{RunID: run.ID, RequirementIDs: []string{r1.ID}, Criteria: []model.ExitCriterion{{Type: "coverage", Threshold: 100}}})
	if !rep.Go || rep.Summary["requirements"] != 1 {
		t.Fatalf("%+v", rep)
	}
	// a project-wide waiver accepts the failing requirement
	w := &model.Waiver{ProjectID: p.ID, Type: "waiver", Title: "later", RequirementIDs: []string{r2.ID}, Justification: "Not needed before the next release.", RequestedBy: "u", Approvers: []string{"a@x"}, Status: "requested", CreatedBy: "u"}
	must(t, st.CreateWaiver(ctx, ten.ID, w))
	must(t, st.SignWaiver(ctx, ten.ID, &model.WaiverApproval{WaiverID: w.ID, UserID: "a", UserEmail: "a@x", Decision: "approve"}))
	must(t, st.SetWaiverStatus(ctx, ten.ID, w.ID, []string{"requested"}, "approved", "a@x", ""))
	rep = gate(store.GateQuery{RunID: run.ID, Criteria: []model.ExitCriterion{{Type: "no_failed"}}})
	if c := crit(rep, "no_failed"); !c.Met || c.State != store.CriterionWaived || len(rep.Waivers) != 1 {
		t.Fatalf("%+v", rep)
	}
	// a release: its scope and runs; criteria may be replaced
	rel := &model.Release{ProjectID: p.ID, Name: "1.0", ExitCriteria: []model.ExitCriterion{{Type: "no_stale"}}}
	must(t, st.CreateRelease(ctx, ten.ID, rel))
	_, _, err = st.SetReleaseRequirements(ctx, ten.ID, rel.ID, []string{r1.ID}, nil)
	must(t, err)
	old.ReleaseID = &rel.ID
	must(t, st.UpdateRun(ctx, ten.ID, old))
	rep = gate(store.GateQuery{ReleaseID: rel.ID})
	if rep.Go || rep.Release.ID != rel.ID || crit(rep, "no_stale").Items[0].Key != t1.Key {
		t.Fatalf("%+v", rep)
	}
	rep = gate(store.GateQuery{ReleaseID: rel.ID, RunID: run.ID})
	if !rep.Go || rep.Summary["runs"] != 0 {
		t.Fatalf("%+v", rep.Summary)
	}
	if _, err := st.GateStatus(ctx, ten.ID, store.GateQuery{ProjectID: "other", ReleaseID: rel.ID}, nil, nil); err == nil {
		t.Fatal("release of another project")
	}
	if _, err := st.GateStatus(ctx, ten.ID, store.GateQuery{ProjectID: p.ID, Variant: "nope"}, nil, nil); err == nil {
		t.Fatal("unknown variant")
	}

	// CI deliveries: a waiting post takes the next results along
	d := &model.CIDelivery{ProjectID: p.ID, RunID: run.ID, CommitSHA: "ABCDEF1234", MergeRequest: "5", CreatedBy: "u"}
	id, created, err := st.AddCIDelivery(ctx, ten.ID, d)
	if err != nil || !created {
		t.Fatal(err)
	}
	if id2, created, err := st.AddCIDelivery(ctx, ten.ID, &model.CIDelivery{ProjectID: p.ID, RunID: run.ID, CommitSHA: "abcdef1234", MergeRequest: "5"}); err != nil || created || id2 != id {
		t.Fatal(id2, created, err)
	}
	if _, _, err := st.AddCIDelivery(ctx, ten.ID, &model.CIDelivery{ProjectID: p.ID, CommitSHA: "not hex"}); err == nil {
		t.Fatal("bad sha")
	}
	due, err := st.DueCIDeliveries(ctx, 10)
	if err != nil || len(due) != 1 {
		t.Fatal(due, err)
	}
	job, err := st.ClaimCIDelivery(ctx, id, time.Minute)
	if err != nil || job == nil || job.TenantID != ten.ID || job.Delivery.CommitSHA != "abcdef1234" || job.Delivery.Status != model.CISending {
		t.Fatalf("%+v %v", job, err)
	}
	if again, _ := st.ClaimCIDelivery(ctx, id, time.Minute); again != nil {
		t.Fatal("claimed twice")
	}
	retry := now.Add(time.Minute)
	must(t, st.FinishCIDelivery(ctx, id, store.CIOutcome{StatusCode: 500, Error: "boom", RetryAt: &retry}))
	if due, _ := st.DueCIDeliveries(ctx, 10); len(due) != 0 {
		t.Fatal("due before the backoff")
	}
	now = now.Add(2 * time.Minute)
	if job, _ := st.ClaimCIDelivery(ctx, id, time.Minute); job == nil || job.Delivery.Attempts != 1 {
		t.Fatalf("%+v", job)
	}
	must(t, st.FinishCIDelivery(ctx, id, store.CIOutcome{OK: true, StatusCode: 200, Outcome: "qtestframe/tests failed"}))
	// the log keeps the newest CILogSize finished posts
	for i := 0; i < store.CILogSize+3; i++ {
		now = now.Add(time.Second)
		x, _, err := st.AddCIDelivery(ctx, ten.ID, &model.CIDelivery{ProjectID: p.ID, RunID: "r" + strings.Repeat("x", i), CommitSHA: "abcdef1234"})
		must(t, err)
		must(t, st.FinishCIDelivery(ctx, x, store.CIOutcome{Error: "refused"}))
	}
	list, err := st.ListCIDeliveries(ctx, ten.ID, p.ID, 0)
	if err != nil || len(list) != store.CILogSize || list[0].Status != model.CIFailed || list[0].Error != "refused" {
		t.Fatal(len(list), err)
	}
	if err := st.FinishCIDelivery(ctx, "missing", store.CIOutcome{}); err == nil {
		t.Fatal("missing delivery")
	}

	// code changes by ref; the merge request of a branch
	c := &model.CodeChange{ProjectID: p.ID, Kind: model.CodeCommit, Provider: model.ProviderGitLab, Ref: "abcdef1234567890", Title: "x"}
	_, err = st.UpsertCodeChange(ctx, ten.ID, c)
	must(t, err)
	mr := &model.CodeChange{ProjectID: p.ID, Kind: model.CodeMergeRequest, Provider: model.ProviderGitLab, Ref: "7", Title: "mr", Branch: "feature", Status: "open"}
	_, err = st.UpsertCodeChange(ctx, ten.ID, mr)
	must(t, err)
	got, err := st.CodeChangesByRef(ctx, ten.ID, p.ID, model.CodeCommit, []string{"ABCDEF1", "abc", "zzzzzzzz", "", "abcdef1234567890"})
	if err != nil || len(got) != 1 || got[0].ID != c.ID {
		t.Fatal(got, err)
	}
	if got, _ := st.CodeChangesByRef(ctx, ten.ID, p.ID, model.CodeMergeRequest, []string{"!7"}); len(got) != 1 {
		t.Fatal(got)
	}
	if ref, err := st.FindCommitMergeRequest(ctx, ten.ID, p.ID, "feature"); err != nil || ref != "7" {
		t.Fatal(ref, err)
	}
	if ref, _ := st.FindCommitMergeRequest(ctx, ten.ID, p.ID, ""); ref != "" {
		t.Fatal(ref)
	}
	if ref, _ := st.FindCommitMergeRequest(ctx, ten.ID, p.ID, "main"); ref != "" {
		t.Fatal(ref)
	}

	// CI settings: validation; the token is kept, cleared and never empty by accident
	if _, err := st.SaveCISettings(ctx, ten.ID, p.ID, model.CISettings{Provider: "gitlab", Project: "g/p", ReleaseID: rel.ID, Conditions: []string{" must ", ""}}, "tok", false, "u"); err != nil {
		t.Fatal(err)
	}
	cs, err := st.CodeSettings(ctx, ten.ID, p.ID)
	if err != nil || cs.CIToken != "tok" || !cs.HasCIToken || len(cs.CI.Conditions) != 1 || cs.CI.Conditions[0] != "must" || !cs.CI.Enabled() == true && cs.CI.PostStatus {
		t.Fatalf("%+v %v", cs, err)
	}
	if cs.CI.Enabled() {
		t.Fatal("enabled without posting")
	}
	cs, err = st.SaveCISettings(ctx, ten.ID, p.ID, model.CISettings{Provider: "gitlab", Project: "g/p", PostStatus: true}, "", false, "u")
	if err != nil || cs.CIToken != "tok" || !cs.CI.Enabled() {
		t.Fatalf("%+v", cs)
	}
	for _, bad := range []model.CISettings{{Provider: "x"}, {Provider: "gitlab"}, {APIURL: "nope"}, {Provider: "gitlab", Project: strings.Repeat("p", 201)},
		{Conditions: []string{"bogus"}}, {Conditions: make([]string, 31)}} {
		if len(bad.Conditions) == 31 {
			for i := range bad.Conditions {
				bad.Conditions[i] = "must"
			}
		}
		if err := store.ValidateCISettings(&bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if _, err := st.SaveCISettings(ctx, ten.ID, p.ID, model.CISettings{}, strings.Repeat("t", 501), false, "u"); err == nil {
		t.Fatal("long token")
	}
	cs, _ = st.SaveCISettings(ctx, ten.ID, p.ID, model.CISettings{}, "", true, "u")
	if cs.HasCIToken {
		t.Fatal("token kept")
	}
}
