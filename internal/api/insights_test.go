package api

import (
	"encoding/json"
	"testing"

	"github.com/rom/qtestframe/internal/model"
)

func TestDashboardInsights(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	fx := newReportFixture(t, a) // Login: 2 tests (passed, failed), Logout: 1 test (passed), Audit: none
	base := "/api/v1/projects/" + fx.pid
	path := base + "/dashboard/insights"
	get := func() dashboardInsights {
		t.Helper()
		var out dashboardInsights
		b, _ := json.Marshal(a.m("GET", path, nil, 200))
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	d := get()
	if d.Requirements.Total != 3 || d.Requirements.WithTests != 2 || d.Requirements.Passing != 1 || d.Requirements.Review["none"] != 3 || d.Requirements.ByStatus["approved"] != 2 {
		t.Fatalf("%+v", d.Requirements)
	}
	if d.Tests.Total != 3 || d.Tests.ByType["manual"]["passed"] != 1 || d.Tests.ByType["manual"]["not_run"] != 1 || d.Tests.ByType["automated"]["failed"] != 1 {
		t.Fatalf("%+v", d.Tests)
	}
	if d.Risks.Total != 0 || d.Release != nil || d.TraceRules.Rules != 0 || !d.TraceRules.Passed || d.Defects.Open != 0 || d.Suspect.Links != 0 {
		t.Fatalf("%+v", d)
	}
	if len(d.Activity.Days) != 30 || d.Activity.Days[29].Day != d.Activity.To || d.Activity.Total.Executions != 2 || d.Activity.Total.Changes < 6 {
		t.Fatalf("%+v", d.Activity)
	}

	reqs := map[string]string{}
	for _, r := range items(a.m("GET", base+"/requirements", nil, 200)) {
		reqs[r["title"].(string)] = r["id"].(string)
	}
	a.m("POST", base+"/risks", map[string]any{"title": "Takeover", "likelihood": 5, "impact": 4, "requirement_ids": []string{reqs["Login"]}}, 201)
	a.m("POST", base+"/risks", map[string]any{"title": "Logout fails", "likelihood": 2, "impact": 3, "status": "mitigating", "requirement_ids": []string{reqs["Logout"]}}, 201)
	a.m("POST", base+"/risks", map[string]any{"title": "Gone", "likelihood": 1, "impact": 1, "status": "closed"}, 201)
	a.m("POST", base+"/risks", map[string]any{"title": "Unaddressed", "likelihood": 3, "impact": 3}, 201)
	a.m("POST", base+"/defects", map[string]any{"title": "Crash", "severity": "critical"}, 201)
	a.m("POST", base+"/trace-rules", map[string]any{"name": "All verified", "kind": "verified_by"}, 201)
	a.m("POST", "/api/v1/reviews", map[string]any{"entity_type": "requirement", "entity_id": reqs["Audit"], "reviewers": []string{e.admin.ID}}, 201)
	rel := a.m("POST", base+"/releases", map[string]any{"name": "1.0", "status": "active"}, 201)["id"].(string)
	a.m("POST", base+"/releases", map[string]any{"name": "2.0"}, 201)
	var tests []string
	for _, x := range items(a.m("GET", base+"/tests", nil, 200)) {
		tests = append(tests, x["id"].(string))
	}
	run := a.m("POST", base+"/runs", map[string]any{"name": "RC1", "release_id": rel, "test_ids": tests[:2]}, 201)["id"].(string)
	ex := items(a.m("GET", "/api/v1/runs/"+run+"/executions", nil, 200))
	a.m("PATCH", "/api/v1/executions/"+ex[0]["id"].(string), map[string]any{"status": "passed"}, 200)
	// a test change makes its links suspect
	a.m("PATCH", "/api/v1/tests/"+tests[0], map[string]any{"title": "Changed title", "steps": []map[string]any{{"action": "other", "expected": "x"}}}, 200)

	d = get()
	rk := d.Risks
	if rk.Total != 4 || rk.Open != 3 || rk.ByLevel["critical"] != 1 || rk.ByLevel["medium"] != 2 || rk.ByStatus["closed"] != 1 || rk.Heat[4][3] != 1 || rk.Heat[0][0] != 0 || rk.Heat[1][2] != 1 {
		t.Fatalf("%+v", rk)
	}
	if rk.Mitigation["no_requirements"] != 1 || rk.Mitigation["tested"]+rk.Mitigation["verified"] != 2 {
		t.Fatalf("%+v", rk.Mitigation)
	}
	if d.Defects.Open != 1 || d.Defects.Buckets["lt7"] != 1 || d.Defects.BySeverity["critical"]["lt7"] != 1 {
		t.Fatalf("%+v", d.Defects)
	}
	if d.TraceRules.Rules != 1 || d.TraceRules.Failed != 1 || d.TraceRules.Errors != 1 || d.TraceRules.Violations != 1 || d.TraceRules.Passed {
		t.Fatalf("%+v", d.TraceRules)
	}
	if d.Requirements.Review["pending"] != 1 || d.Requirements.Review["none"] != 2 {
		t.Fatalf("%+v", d.Requirements.Review)
	}
	r := d.Release
	if r == nil || r.ID != rel || r.Runs != 1 || r.Total != 2 || r.Executed != 1 || r.Passed != 1 || r.Executions[model.ResultNotRun] != 1 {
		t.Fatalf("%+v", r)
	}
	if d.Activity.Total.Defects != 1 || d.Activity.Total.Executions != 3 {
		t.Fatalf("%+v", d.Activity.Total)
	}
	if d.Suspect.Links != 2 || d.Suspect.Requirements != 2 {
		t.Fatalf("%+v", d.Suspect)
	}

	a.m("GET", "/api/v1/projects/missing/dashboard/insights", nil, 404)
}

func TestCurrentRelease(t *testing.T) {
	if currentRelease(nil) != nil {
		t.Fatal("none")
	}
	rs := []model.Release{{ID: "a", Status: "released"}, {ID: "b", Status: "planned"}, {ID: "c", Status: "planned", TargetDate: "2026-05-01"}, {ID: "d", Status: "planned", TargetDate: "2026-04-01"}}
	if r := currentRelease(rs); r.ID != "d" {
		t.Fatal(r.ID)
	}
	rs = append(rs, model.Release{ID: "e", Status: "active"})
	if r := currentRelease(rs); r.ID != "e" {
		t.Fatal(r.ID)
	}
}
