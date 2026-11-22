package api

import (
	"context"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
)

// correlationFixture: requirement A (implemented), B refines A, C relates
// to A; tests TA (verifies A) and TB (verifies B), both passed in run R.
type correlationFixture struct {
	a                *client
	pid              string
	ra, rb, rc       string
	ta, tb           string
	run              string
	execA, execB     string
	relBA, relCA     string
	requirementByKey map[string]string
}

func newCorrelationFixture(t *testing.T, e *env) *correlationFixture {
	t.Helper()
	a := e.login("admin@example.com")
	f := &correlationFixture{a: a, pid: a.project("COR")}
	pp := "/api/v1/projects/" + f.pid
	mk := func(title string) string {
		return a.m("POST", pp+"/requirements", map[string]any{"title": title, "description": "The system MUST " + title + ".", "status": "implemented"}, 201)["id"].(string)
	}
	f.ra, f.rb, f.rc = mk("log in"), mk("log in with a password"), mk("show a banner")
	f.relBA = a.m("POST", "/api/v1/requirements/"+f.rb+"/relations", map[string]any{"target_id": f.ra, "type": "refines"}, 201)["id"].(string)
	f.relCA = a.m("POST", "/api/v1/requirements/"+f.rc+"/relations", map[string]any{"target_id": f.ra, "type": "relates_to"}, 201)["id"].(string)
	f.ta = a.m("POST", pp+"/tests", map[string]any{"title": "Login works", "status": "ready"}, 201)["id"].(string)
	f.tb = a.m("POST", pp+"/tests", map[string]any{"title": "Password login", "status": "ready"}, 201)["id"].(string)
	a.m("POST", pp+"/links", map[string]any{"requirement_id": f.ra, "test_case_id": f.ta}, 201)
	a.m("POST", pp+"/links", map[string]any{"requirement_id": f.rb, "test_case_id": f.tb}, 201)
	f.run = a.m("POST", pp+"/runs", map[string]any{"name": "R", "test_ids": []string{f.ta, f.tb}}, 201)["id"].(string)
	for _, x := range items(a.m("GET", "/api/v1/runs/"+f.run+"/executions", nil, 200)) {
		if x["test_case_id"] == f.ta {
			f.execA = x["id"].(string)
		} else {
			f.execB = x["id"].(string)
		}
	}
	a.m("PATCH", "/api/v1/executions/"+f.execA, map[string]any{"status": "passed"}, 200)
	a.m("PATCH", "/api/v1/executions/"+f.execB, map[string]any{"status": "passed"}, 200)
	return f
}

func (f *correlationFixture) req(id string) map[string]any {
	return f.a.m("GET", "/api/v1/requirements/"+id, nil, 200)
}

func TestCorrelationSettings(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("SET")
	path := "/api/v1/projects/" + pid + "/correlation-settings"
	got := a.m("GET", path, nil, 200)
	if got["stale_after_days"].(float64) != model.DefaultStaleAfterDays || got["regression_action"] != "reopen" || got["relation_impact"].(map[string]any)["refines"] != "incoming" {
		t.Fatal(got)
	}
	got = a.m("PUT", path, map[string]any{"stale_after_days": 0, "relation_impact": map[string]any{"depends_on": "both"}}, 200)
	ri := got["relation_impact"].(map[string]any)
	if got["stale_after_days"].(float64) != 0 || ri["depends_on"] != "both" || ri["refines"] != "off" || got["stale_on_change"] != true {
		t.Fatal(got)
	}
	// kept on reload; partial updates keep the rest
	a.m("PUT", path, map[string]any{"regression_action": "new_defect"}, 200)
	got = a.m("GET", path, nil, 200)
	if got["regression_action"] != "new_defect" || got["relation_impact"].(map[string]any)["depends_on"] != "both" || got["stale_after_days"].(float64) != 0 {
		t.Fatal(got)
	}
	for _, bad := range []map[string]any{
		{"relation_impact": map[string]any{"nope": "incoming"}},
		{"relation_impact": map[string]any{"refines": "sideways"}},
		{"stale_after_days": -1},
		{"blocking_severities": []string{"minor"}, "warning_severities": []string{"minor"}},
		{"blocking_severities": []string{"huge"}},
		{"regression_action": "explode"},
		{"relation_impact": "x"},
		{"stale_after_days": "x"},
	} {
		a.m("PUT", path, bad, 400)
	}
	// viewers read, only project.update writes
	e.member("viewer@example.com", "viewer")
	v := e.login("viewer@example.com")
	v.m("GET", path, nil, 200)
	v.m("PUT", path, map[string]any{"stale_after_days": 5}, 403)
}

func TestChangeImpactAcrossRelations(t *testing.T) {
	e := newEnv(t)
	f := newCorrelationFixture(t, e)
	a := f.a
	if s := f.req(f.ra)["status"]; s != "verified" {
		t.Fatal(s)
	}
	// an issue of A in a tracker (as created by POST /requirements/{id}/issue)
	issue := &model.ItemLink{SourceType: model.KindRequirement, SourceID: f.ra, TargetType: "url", URL: "https://tracker.example/1", Label: "GitLab g/p#1",
		Issue: &model.LinkedIssue{Tracker: "gitlab", Key: "g/p#1", State: "open"}}
	if err := e.st.CreateItemLink(context.Background(), e.tenant.ID, issue); err != nil {
		t.Fatal(err)
	}
	// a generated document containing A
	doc := a.m("POST", "/api/v1/projects/"+f.pid+"/documents/generate", map[string]any{"kind": "requirements_spec", "format": "md"}, 201)

	// the preview: B through refines (C's relates_to does not travel), the
	// tests of A and B, the issue, the document and TA's result
	imp := a.m("GET", "/api/v1/requirements/"+f.ra+"/impact", nil, 200)
	reqs := imp["requirements"].([]any)
	if len(reqs) != 1 || reqs[0].(map[string]any)["id"] != f.rb || reqs[0].(map[string]any)["relation"] != "refines" || reqs[0].(map[string]any)["direction"] != "incoming" {
		t.Fatal(imp)
	}
	if len(imp["tests"].([]any)) != 2 || len(imp["issues"].([]any)) != 1 || len(imp["documents"].([]any)) != 1 || len(imp["results"].([]any)) != 1 || imp["total"].(float64) != 6 {
		t.Fatal(imp)
	}
	a.m("GET", "/api/v1/requirements/missing/impact", nil, 404)
	if ti := a.m("GET", "/api/v1/tests/"+f.tb+"/impact", nil, 200); len(ti["requirements"].([]any)) != 1 || len(ti["results"].([]any)) != 1 {
		t.Fatal(ti)
	}

	// a baseline before the change
	bl := a.m("POST", "/api/v1/projects/"+f.pid+"/baselines", map[string]any{"name": "B1"}, 201)

	// changing A's substance
	a.m("PATCH", "/api/v1/requirements/"+f.ra, map[string]any{"description": "The system MUST log in with MFA."}, 200)
	rels := a.m("GET", "/api/v1/requirements/"+f.rb+"/relations", nil, 200)
	var relB map[string]any
	for _, r := range items(rels) {
		if r["id"] == f.relBA {
			relB = r
		}
		if r["id"] == f.relCA && r["suspect_since"] != nil {
			t.Fatal("relates_to does not travel", r)
		}
	}
	if relB == nil || relB["suspect_since"] == nil || relB["suspect_end"] != "source" {
		t.Fatal(rels)
	}
	b := f.req(f.rb)
	if b["suspect_links"].(float64) != 1 || b["suspect_relations"].(float64) != 1 || b["status"] != "implemented" {
		t.Fatal(b)
	}
	if res := items(a.m("GET", "/api/v1/projects/"+f.pid+"/requirements?q=suspect_relations>0", nil, 200)); len(res) != 1 || res[0]["id"] != f.rb {
		t.Fatal(res)
	}
	// A itself: links suspect, results stale, not verified any more
	ra := f.req(f.ra)
	if ra["status"] != "implemented" || ra["stale_results"].(float64) != 1 || ra["coverage"] == "passed" {
		t.Fatal(ra)
	}
	// the issue is out of date, the document outdated
	l, _ := e.st.GetItemLink(context.Background(), e.tenant.ID, issue.ID)
	if l.Issue.OutdatedSince == nil {
		t.Fatal(l.Issue)
	}
	var found bool
	for _, d := range items(a.m("GET", "/api/v1/projects/"+f.pid+"/documents", nil, 200)) {
		if d["id"] == doc["id"] {
			found = d["outdated"] == true && len(d["outdated_items"].([]any)) == 1
		}
	}
	if !found {
		t.Fatal("document not outdated")
	}
	// the preview now marks them as already affected
	if imp := a.m("GET", "/api/v1/requirements/"+f.ra+"/impact", nil, 200); imp["total"].(float64) != 0 {
		t.Fatal(imp)
	}

	// the impact of the baseline diff
	bi := a.m("GET", "/api/v1/baselines/"+bl["id"].(string)+"/impact", nil, 200)
	if ch := bi["changes"].([]any); len(ch) != 1 || ch[0].(map[string]any)["key"] != ra["key"] {
		t.Fatal(bi)
	}
	if tot := bi["totals"].(map[string]any); tot["requirements"].(float64) != 1 || tot["tests"].(float64) != 2 || tot["documents"].(float64) != 1 {
		t.Fatal(tot)
	}
	a.m("GET", "/api/v1/baselines/missing/impact", nil, 404)

	// confirming
	if res := a.m("POST", "/api/v1/relations/"+f.relBA+"/confirm", nil, 200); res["confirmed"].(float64) != 1 {
		t.Fatal(res)
	}
	a.m("POST", "/api/v1/relations/missing/confirm", nil, 404)
	if res := a.m("POST", "/api/v1/requirements/"+f.rb+"/relations/confirm", nil, 200); res["confirmed"].(float64) != 0 {
		t.Fatal(res)
	}
	a.m("POST", "/api/v1/requirements/missing/relations/confirm", nil, 404)
	if got := a.m("POST", "/api/v1/item-links/"+issue.ID+"/issue/confirm", nil, 200); got["issue"].(map[string]any)["outdated_since"] != nil {
		t.Fatal(got)
	}
	other := a.m("POST", "/api/v1/requirements/"+f.rb+"/item-links", map[string]any{"target_type": "url", "url": "https://x.example"}, 201)
	a.m("POST", "/api/v1/item-links/"+other["id"].(string)+"/issue/confirm", nil, 400)
	a.m("POST", "/api/v1/item-links/missing/issue/confirm", nil, 404)

	// re-running A's test after the change: evidence again
	a.m("POST", "/api/v1/requirements/"+f.ra+"/links/confirm", nil, 200)
	run2 := a.m("POST", "/api/v1/projects/"+f.pid+"/runs", map[string]any{"name": "R2", "test_ids": []string{f.ta}}, 201)
	ex := items(a.m("GET", "/api/v1/runs/"+run2["id"].(string)+"/executions", nil, 200))[0]
	a.m("PATCH", "/api/v1/executions/"+ex["id"].(string), map[string]any{"status": "passed"}, 200)
	if ra := f.req(f.ra); ra["status"] != "verified" || ra["stale_results"] != nil {
		t.Fatal(ra)
	}
	// the stale results of a requirement
	sr := a.m("GET", "/api/v1/requirements/"+f.ra+"/stale-results", nil, 200)
	if it := items(sr); len(it) != 1 || it[0]["stale"] == true {
		t.Fatal(sr)
	}
}

func TestResultStalenessByAge(t *testing.T) {
	e := newEnv(t)
	f := newCorrelationFixture(t, e)
	a := f.a
	if s := f.req(f.ra)["status"]; s != "verified" {
		t.Fatal(s)
	}
	// results of 100 days ago are stale (default 90)
	old := time.Now().UTC().Add(-100 * 24 * time.Hour)
	if _, err := e.st.DB.ExecContext(context.Background(), db.Rebind(e.st.DB.Dialect, `UPDATE test_executions SET started_at = ?, finished_at = ?`), old, old); err != nil {
		t.Fatal(err)
	}
	tests := items(a.m("GET", "/api/v1/projects/"+f.pid+"/tests?q=stale:true", nil, 200))
	if len(tests) != 2 || tests[0]["stale_reason"] != "age" || tests[0]["last_result"] != "passed" {
		t.Fatal(tests)
	}
	tr := a.m("GET", "/api/v1/projects/"+f.pid+"/traceability", nil, 200)
	if tt := tr["tests"].([]any); tt[0].(map[string]any)["stale"] != true {
		t.Fatal(tr)
	}
	if rr := tr["requirements"].([]any); rr[0].(map[string]any)["coverage"] == "passed" {
		t.Fatal(rr)
	}
	dash := a.m("GET", "/api/v1/projects/"+f.pid+"/dashboard", nil, 200)
	if dash["correlations"].(map[string]any)["stale_results"].(float64) != 2 {
		t.Fatal(dash["correlations"])
	}
	// the release does not count stale passes
	rel := a.m("POST", "/api/v1/projects/"+f.pid+"/releases", map[string]any{"name": "1.0", "exit_criteria": []map[string]any{{"type": "passed", "threshold": 100}}}, 201)["id"].(string)
	a.m("POST", "/api/v1/releases/"+rel+"/requirements", map[string]any{"add": []string{f.ra}}, 200)
	a.m("PATCH", "/api/v1/runs/"+f.run, map[string]any{"release_id": rel}, 200)
	st := a.m("GET", "/api/v1/releases/"+rel+"/status", nil, 200)
	crit := st["criteria"].([]any)[0].(map[string]any)
	if st["go"] != false || crit["items"].([]any)[0].(map[string]any)["note"] != "stale" {
		t.Fatal(st)
	}
	// turning the age rule off makes them evidence again
	a.m("PUT", "/api/v1/projects/"+f.pid+"/correlation-settings", map[string]any{"stale_after_days": 0}, 200)
	if st := a.m("GET", "/api/v1/releases/"+rel+"/status", nil, 200); st["go"] != true {
		t.Fatal(st)
	}
	if tests := items(a.m("GET", "/api/v1/projects/"+f.pid+"/tests?q=stale:true", nil, 200)); len(tests) != 0 {
		t.Fatal(tests)
	}
}

func TestBlockingDefects(t *testing.T) {
	e := newEnv(t)
	f := newCorrelationFixture(t, e)
	a := f.a
	pp := "/api/v1/projects/" + f.pid
	// critical on test TA blocks A; minor on execution B warns about B
	crit := a.m("POST", pp+"/defects", map[string]any{"title": "Crash", "severity": "critical", "links": []map[string]any{{"entity_type": "test", "entity_id": f.ta}}}, 201)
	a.m("POST", pp+"/defects", map[string]any{"title": "Typo", "severity": "minor", "links": []map[string]any{{"entity_type": "execution", "entity_id": f.execB}}}, 201)
	a.m("POST", pp+"/defects", map[string]any{"title": "Trivial", "severity": "trivial", "links": []map[string]any{{"entity_type": "requirement", "entity_id": f.rc}}}, 201)
	ra, rb, rc := f.req(f.ra), f.req(f.rb), f.req(f.rc)
	if ra["blocking_defects"].(float64) != 1 || rb["defect_warnings"].(float64) != 1 || rb["blocking_defects"] != nil || rc["blocking_defects"] != nil {
		t.Fatal(ra, rb, rc)
	}
	bd := a.m("GET", "/api/v1/requirements/"+f.ra+"/blocking-defects", nil, 200)
	if bd["blocked"] != true || items(bd)[0]["via"] == "" || items(bd)[0]["level"] != "block" {
		t.Fatal(bd)
	}
	a.m("GET", "/api/v1/requirements/missing/blocking-defects", nil, 404)
	if res := items(a.m("GET", pp+"/requirements?q=blocked>0", nil, 200)); len(res) != 1 || res[0]["id"] != f.ra {
		t.Fatal(res)
	}
	dash := a.m("GET", pp+"/dashboard", nil, 200)["correlations"].(map[string]any)
	if dash["blocked_requirements"].(float64) != 1 || dash["warned_requirements"].(float64) != 1 {
		t.Fatal(dash)
	}
	tr := a.m("GET", pp+"/traceability", nil, 200)
	blocked := 0
	for _, r := range tr["requirements"].([]any) {
		if r.(map[string]any)["blocked"] == true {
			blocked++
		}
	}
	if blocked != 1 {
		t.Fatal(tr)
	}
	// release exit criterion
	rel := a.m("POST", pp+"/releases", map[string]any{"name": "1.0"}, 201)
	found := false
	for _, c := range rel["exit_criteria"].([]any) {
		found = found || c.(map[string]any)["type"] == "no_blocking_defects"
	}
	if !found {
		t.Fatal(rel["exit_criteria"])
	}
	rid := rel["id"].(string)
	a.m("PATCH", "/api/v1/releases/"+rid, map[string]any{"exit_criteria": []map[string]any{{"type": "no_blocking_defects"}}}, 200)
	a.m("POST", "/api/v1/releases/"+rid+"/requirements", map[string]any{"add": []string{f.ra, f.rb}}, 200)
	st := a.m("GET", "/api/v1/releases/"+rid+"/status", nil, 200)
	if st["go"] != false || st["summary"].(map[string]any)["blocked"].(float64) != 1 {
		t.Fatal(st)
	}
	// severity rules are per project: critical only warns now
	a.m("PUT", pp+"/correlation-settings", map[string]any{"blocking_severities": []string{}, "warning_severities": []string{"critical", "minor"}}, 200)
	if st := a.m("GET", "/api/v1/releases/"+rid+"/status", nil, 200); st["go"] != true {
		t.Fatal(st)
	}
	a.m("PUT", pp+"/correlation-settings", map[string]any{"blocking_severities": []string{"critical"}, "warning_severities": []string{}}, 200)
	a.m("PATCH", "/api/v1/defects/"+crit["id"].(string), map[string]any{"status": "closed"}, 200)
	if st := a.m("GET", "/api/v1/releases/"+rid+"/status", nil, 200); st["go"] != true {
		t.Fatal(st)
	}
}

func TestRegressions(t *testing.T) {
	e := newEnv(t)
	f := newCorrelationFixture(t, e)
	a := f.a
	pp := "/api/v1/projects/" + f.pid
	// the defect was found by TA in run R, fixed, and TA passed again in R2
	e.member("dev@example.com", "tester")
	d := a.m("POST", pp+"/defects", map[string]any{"title": "Login fails", "severity": "major", "assignee": "dev@example.com",
		"links": []map[string]any{{"entity_type": "test", "entity_id": f.ta}}}, 201)
	did := d["id"].(string)
	a.m("PATCH", "/api/v1/defects/"+did, map[string]any{"status": "resolved"}, 200)
	exec := func(name, status string) string {
		run := a.m("POST", pp+"/runs", map[string]any{"name": name, "test_ids": []string{f.ta}}, 201)
		ex := items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200))[0]["id"].(string)
		a.m("PATCH", "/api/v1/executions/"+ex, map[string]any{"status": status}, 200)
		return ex
	}
	exec("R2", "passed")
	// failing again reopens it, once
	ex3 := exec("R3", "failed")
	if got := a.m("GET", "/api/v1/defects/"+did, nil, 200); got["status"] != "open" {
		t.Fatal(got)
	}
	regs := items(a.m("GET", "/api/v1/defects/"+did+"/regressions", nil, 200))
	if len(regs) != 1 || regs[0]["action"] != "reopened" || regs[0]["previous_status"] != "resolved" || regs[0]["execution_id"] != ex3 || regs[0]["run_name"] != "R3" {
		t.Fatal(regs)
	}
	if regs := items(a.m("GET", "/api/v1/tests/"+f.ta+"/regressions", nil, 200)); len(regs) != 1 {
		t.Fatal(regs)
	}
	a.m("GET", "/api/v1/defects/missing/regressions", nil, 404)
	a.m("GET", "/api/v1/tests/missing/regressions", nil, 404)
	// the owner was told
	ns := items(e.login("dev@example.com").m("GET", "/api/v1/notifications", nil, 200))
	told := false
	for _, n := range ns {
		told = told || n["event"] == "defect.regression"
	}
	if !told {
		t.Fatal(ns)
	}

	// a test that never passed after the fix: no regression
	a.m("PATCH", "/api/v1/defects/"+did, map[string]any{"status": "closed"}, 200)
	exec("R4", "failed")
	if got := a.m("GET", "/api/v1/defects/"+did, nil, 200); got["status"] != "closed" {
		t.Fatal(got)
	}

	// new_defect: a linked regression defect
	a.m("PUT", pp+"/correlation-settings", map[string]any{"regression_action": "new_defect"}, 200)
	exec("R5", "passed")
	exec("R6", "failed")
	regs = items(a.m("GET", "/api/v1/defects/"+did+"/regressions", nil, 200))
	if len(regs) != 2 || regs[0]["action"] != "created" || regs[0]["regression_defect_key"] == "" {
		t.Fatal(regs)
	}
	nd := a.m("GET", "/api/v1/defects/"+regs[0]["regression_defect_id"].(string), nil, 200)
	if nd["status"] != "open" || nd["severity"] != "major" {
		t.Fatal(nd)
	}
	if got := a.m("GET", "/api/v1/defects/"+did, nil, 200); got["status"] != "closed" {
		t.Fatal(got)
	}
	// off: nothing
	a.m("PUT", pp+"/correlation-settings", map[string]any{"regression_action": "off"}, 200)
	exec("R7", "passed")
	exec("R8", "failed")
	if regs := items(a.m("GET", "/api/v1/defects/"+did+"/regressions", nil, 200)); len(regs) != 2 {
		t.Fatal(regs)
	}
}

func TestRegressionsOnImport(t *testing.T) {
	e := newEnv(t)
	f := newCorrelationFixture(t, e)
	a := f.a
	pp := "/api/v1/projects/" + f.pid
	d := a.m("POST", pp+"/defects", map[string]any{"title": "Login fails", "links": []map[string]any{{"entity_type": "test", "entity_id": f.ta}}}, 201)
	a.m("PATCH", "/api/v1/defects/"+d["id"].(string), map[string]any{"status": "resolved"}, 200)
	run := a.m("POST", pp+"/runs", map[string]any{"name": "Again", "test_ids": []string{f.ta}}, 201)["id"].(string)
	ex := items(a.m("GET", "/api/v1/runs/"+run+"/executions", nil, 200))[0]["id"].(string)
	a.m("PATCH", "/api/v1/executions/"+ex, map[string]any{"status": "passed"}, 200)
	run2 := a.m("POST", pp+"/runs", map[string]any{"name": "CI", "test_ids": []string{f.ta}}, 201)["id"].(string)
	ta := a.m("GET", "/api/v1/tests/"+f.ta, nil, 200)
	a.m("POST", "/api/v1/runs/"+run2+"/results", map[string]any{"results": []map[string]any{{"test_key": ta["key"], "status": "failed"}}}, 200)
	if got := a.m("GET", "/api/v1/defects/"+d["id"].(string), nil, 200); got["status"] != "open" {
		t.Fatal(got)
	}
}
