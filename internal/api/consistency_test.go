package api

import (
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

func resultOf(t *testing.T, rep map[string]any, kind string) map[string]any {
	t.Helper()
	for _, x := range rep["results"].([]any) {
		r := x.(map[string]any)
		if r["rule"].(map[string]any)["kind"] == kind {
			return r
		}
	}
	t.Fatalf("no result of %s in %v", kind, rep)
	return nil
}

func violationIDs(res map[string]any) []string {
	var out []string
	for _, v := range res["violations"].([]any) {
		out = append(out, v.(map[string]any)["id"].(string))
	}
	return out
}

func TestBuiltinConsistencyChecks(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("CON")
	req := func(title, desc, status string) string {
		return a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": title, "description": desc, "status": status}, 201)["id"].(string)
	}
	r1 := req("Lockout", "The system MUST lock an account after five failed sign-in attempts.", "approved")
	r2 := req("Unlock", "The system MUST keep accounts unlocked.", "approved")
	r3 := req("Lockout again", "The system MUST lock an account after five failed sign-in attempt.", "draft")
	r4 := req("Old parent", "The system MAY show a banner.", "obsolete")
	r5 := req("Child", "The system SHOULD show a banner on start.", "draft")
	a.m("POST", "/api/v1/requirements/"+r1+"/relations", map[string]any{"target_id": r2, "type": "conflicts_with"}, 201)
	a.m("POST", "/api/v1/requirements/"+r5+"/relations", map[string]any{"target_id": r4, "type": "refines"}, 201)
	tst := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Banner"}, 201)["id"].(string)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": r4, "test_case_id": tst}, 201)
	rel := a.m("POST", "/api/v1/projects/"+pid+"/releases", map[string]any{"name": "1.0", "status": "active", "exit_criteria": []any{map[string]any{"type": "trace_rules", "level": "warning"}}}, 201)
	rid := rel["id"].(string)
	a.m("POST", "/api/v1/releases/"+rid+"/requirements", map[string]any{"add": []string{r1, r2}}, 200)
	a.m("POST", "/api/v1/projects/"+pid+"/risks", map[string]any{"title": "Account takeover", "likelihood": 5, "impact": 4}, 201)

	// the analytics evaluate every check, installed or not
	con := a.m("GET", "/api/v1/projects/"+pid+"/analytics/consistency", nil, 200)
	if con["installed"].(float64) != 0 || len(con["checks"].([]any)) != len(model.TraceBuiltinKinds) || len(con["results"].([]any)) != len(model.TraceBuiltinKinds) {
		t.Fatal(con)
	}
	conflict := resultOf(t, con, model.CheckConflictInRelease)
	if ids := violationIDs(conflict); len(ids) != 2 || conflict["rule"].(map[string]any)["id"] != "builtin:conflict_in_release" {
		t.Fatal(conflict)
	}
	v := conflict["violations"].([]any)[0].(map[string]any)
	if v["check"] != "conflict" || len(v["related"].([]any)) != 2 || !strings.Contains(strings.Join(anyStrings(v["fixes"]), ","), "remove_from_release") {
		t.Fatal(v)
	}
	if ids := violationIDs(resultOf(t, con, model.CheckObsoleteParent)); len(ids) != 1 || ids[0] != r5 {
		t.Fatal(ids)
	}
	if ids := violationIDs(resultOf(t, con, model.CheckVerifiesInactive)); len(ids) != 1 || ids[0] != tst {
		t.Fatal(ids)
	}
	if ids := violationIDs(resultOf(t, con, model.CheckUnevidencedMust)); len(ids) != 2 {
		t.Fatal(ids)
	}
	if ids := violationIDs(resultOf(t, con, model.CheckUnmitigatedRisk)); len(ids) != 1 {
		t.Fatal(ids)
	}
	dup := resultOf(t, con, model.CheckDuplicateSuspect)
	if ids := violationIDs(dup); len(ids) != 1 || ids[0] != r3 {
		t.Fatal(dup)
	}
	// not installed: the release does not see them
	st := a.m("GET", "/api/v1/releases/"+rid+"/status", nil, 200)
	if c := st["criteria"].([]any)[0].(map[string]any); c["met"] != true {
		t.Fatal(st)
	}

	// install the defaults (a tester may not; twice installs nothing more)
	e.member("tester@example.com", rbac.RoleTester)
	ts := e.login("tester@example.com")
	ts.m("POST", "/api/v1/projects/"+pid+"/trace-rules/builtins", nil, 403)
	a.m("POST", "/api/v1/projects/"+pid+"/trace-rules/builtins", map[string]any{"kinds": []string{"nope"}}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/trace-rules", map[string]any{"name": "Suspected duplicate requirements", "kind": "verifies"}, 201)
	created := items(a.m("POST", "/api/v1/projects/"+pid+"/trace-rules/builtins", map[string]any{"kinds": []string{model.CheckDuplicateSuspect}}, 201))
	if len(created) != 1 || created[0]["name"] != "Suspected duplicate requirements (2)" || created[0]["threshold"].(float64) != 0.68 || created[0]["severity"] != "warning" {
		t.Fatal(created)
	}
	created = items(a.m("POST", "/api/v1/projects/"+pid+"/trace-rules/builtins", nil, 201))
	if len(created) != len(model.TraceBuiltinKinds)-1 {
		t.Fatal(len(created))
	}
	if again := items(a.m("POST", "/api/v1/projects/"+pid+"/trace-rules/builtins", nil, 201)); len(again) != 0 {
		t.Fatal(again)
	}
	states := items(a.m("GET", "/api/v1/projects/"+pid+"/trace-rules/builtins", nil, 200))
	for _, s := range states {
		if s["installed"] != true || s["rule"] == nil {
			t.Fatal(s)
		}
	}
	// options: risk level and threshold are validated
	var riskRule, dupRule map[string]any
	for _, s := range states {
		switch s["kind"] {
		case model.CheckUnmitigatedRisk:
			riskRule = s["rule"].(map[string]any)
		case model.CheckDuplicateSuspect:
			dupRule = s["rule"].(map[string]any)
		}
	}
	a.m("PATCH", "/api/v1/trace-rules/"+riskRule["id"].(string), map[string]any{"level": "extreme"}, 400)
	a.m("PATCH", "/api/v1/trace-rules/"+riskRule["id"].(string), map[string]any{"selector": "type:x"}, 400)
	a.m("PATCH", "/api/v1/trace-rules/"+dupRule["id"].(string), map[string]any{"threshold": 0.2}, 400)
	if r := a.m("PATCH", "/api/v1/trace-rules/"+riskRule["id"].(string), map[string]any{"level": "critical", "scope": "risk"}, 200); r["level"] != "critical" || r["scope"] != "risk" {
		t.Fatal(r)
	}
	a.m("POST", "/api/v1/projects/"+pid+"/trace-rules", map[string]any{"name": "Wrong scope", "kind": model.CheckVerifiesInactive, "scope": "requirement"}, 400)
	prev := a.m("POST", "/api/v1/projects/"+pid+"/trace-rules/preview", map[string]any{"name": "p", "kind": model.CheckObsoleteParent}, 200)
	if prev["violation_count"].(float64) != 1 {
		t.Fatal(prev)
	}

	// installed: the release readiness counts them (warnings at level warning)
	st = a.m("GET", "/api/v1/releases/"+rid+"/status", nil, 200)
	c := st["criteria"].([]any)[0].(map[string]any)
	found := map[string]bool{}
	for _, it := range c["items"].([]any) {
		found[it.(map[string]any)["kind"].(string)+":"+it.(map[string]any)["id"].(string)] = true
	}
	if c["met"] != false || !found["requirement:"+r1] || !found["requirement:"+r2] {
		t.Fatal(c)
	}
	// item view and dashboard
	if vs := items(a.m("GET", "/api/v1/requirements/"+r5+"/trace-rules", nil, 200)); len(vs) != 1 || vs[0]["check"] != "obsolete" {
		t.Fatal(vs)
	}
	ins := a.m("GET", "/api/v1/projects/"+pid+"/dashboard/insights", nil, 200)
	if tr := ins["trace_rules"].(map[string]any); tr["rules"].(float64) < 9 || tr["warnings"].(float64) < 5 {
		t.Fatal(tr)
	}

	// dismiss the suspected duplicate; a viewer may not, list and undo
	base := "/api/v1/projects/" + pid + "/consistency/dismissals"
	e.member("viewer@example.com", rbac.RoleViewer)
	vw := e.login("viewer@example.com")
	body := map[string]any{"check": model.CheckDuplicateSuspect, "item_id": r3, "other_id": r1}
	vw.m("POST", base, body, 403)
	a.m("POST", base, map[string]any{"check": "conflict_in_release", "item_id": r1, "other_id": r2}, 400)
	a.m("POST", base, body, 201)
	if l := items(a.m("GET", base, nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	con = a.m("GET", "/api/v1/projects/"+pid+"/analytics/consistency", nil, 200)
	if con["installed"].(float64) != float64(len(model.TraceBuiltinKinds)) || len(violationIDs(resultOf(t, con, model.CheckDuplicateSuspect))) != 0 {
		t.Fatal(con)
	}
	a.m("DELETE", base, body, 204)
	// mark as duplicate instead: the relation clears it as well
	a.m("POST", "/api/v1/requirements/"+r3+"/relations", map[string]any{"target_id": r1, "type": "duplicates"}, 201)
	con = a.m("GET", "/api/v1/projects/"+pid+"/analytics/consistency", nil, 200)
	if len(violationIDs(resultOf(t, con, model.CheckDuplicateSuspect))) != 0 {
		t.Fatal(con)
	}

	// trace rule report: the involved items are listed
	_, data := a.download("/api/v1/projects/"+pid+"/reports/generate", map[string]any{"template": "traceability", "format": "csv"}, 200)
	if f := zipFiles(t, data)["trace_rule_violations.csv"]; !strings.Contains(f, "Conflicting requirements in a release") || !strings.Contains(f, "1.0") {
		t.Fatal(f)
	}
	a.m("GET", "/api/v1/projects/nope/analytics/consistency", nil, 404)
}

func anyStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func TestQualityMetricsAPI(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("QM")
	rq := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Fast", "description": "The system SHOULD be fast and user-friendly."}, 201)["id"].(string)
	a.m("PATCH", "/api/v1/requirements/"+rq, map[string]any{"title": "Quick"}, 200)
	tst := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Speed"}, 201)["id"].(string)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": rq, "test_case_id": tst}, 201)
	d := a.m("POST", "/api/v1/projects/"+pid+"/defects", map[string]any{"title": "Slow", "found_in": "production", "links": []map[string]any{{"entity_type": "test", "entity_id": tst}}}, 201)
	if d["found_in"] != "production" {
		t.Fatal(d)
	}
	a.m("POST", "/api/v1/projects/"+pid+"/defects", map[string]any{"title": "Bad", "found_in": "moon"}, 400)
	if u := a.m("PATCH", "/api/v1/defects/"+d["id"].(string), map[string]any{"status": "resolved"}, 200); u["found_in"] != "production" {
		t.Fatal(u)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/defects?q=found_in:production", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}

	q := a.m("GET", "/api/v1/projects/"+pid+"/analytics/quality?range=30&stale_days=10&period=week", nil, 200)
	if q["period"] != "week" || q["stale_days"].(float64) != 10 {
		t.Fatal(q)
	}
	vol := q["volatility"].(map[string]any)
	esc := q["escaped"].(map[string]any)
	dd := q["defect_density"].(map[string]any)
	eff := q["effectiveness"].(map[string]any)
	ft := q["fix_time"].(map[string]any)
	rqq := q["requirement_quality"].(map[string]any)
	if vol["changes"].(float64) != 1 || esc["escaped"].(float64) != 1 || esc["production"].(float64) != 1 || dd["defects"].(float64) != 1 ||
		eff["defects"].(float64) != 1 || ft["count"].(float64) != 1 || rqq["with_warnings"].(float64) != 1 || q["result_age"].(map[string]any)["never"].(float64) != 1 {
		t.Fatal(q)
	}
	for _, bad := range []string{"range=7", "stale_days=0", "period=day", "lang=xx"} {
		a.m("GET", "/api/v1/projects/"+pid+"/analytics/quality?"+bad, nil, 400)
	}
	e.member("viewer@example.com", rbac.RoleViewer)
	e.login("viewer@example.com").m("GET", "/api/v1/projects/"+pid+"/analytics/quality", nil, 200)
	a.m("GET", "/api/v1/projects/nope/analytics/quality", nil, 404)

	// the standard report
	var tpls []map[string]any
	a.call("GET", "/api/v1/reports/templates", nil, 200, &tpls)
	found := false
	for _, x := range tpls {
		found = found || x["id"] == "quality_metrics"
	}
	if !found {
		t.Fatal(tpls)
	}
	_, data := a.download("/api/v1/projects/"+pid+"/reports/generate", map[string]any{"template": "quality_metrics", "format": "csv", "params": map[string]string{"range": "90"}}, 200)
	files := zipFiles(t, data)
	if !strings.Contains(files["summary.csv"], "Escaped defects") || !strings.Contains(files["escaped_defects.csv"], "Slow") {
		t.Fatal(files)
	}
	a.download("/api/v1/projects/"+pid+"/reports/generate", map[string]any{"template": "quality_metrics", "format": "pdf", "params": map[string]string{"range": "3"}}, 400)
}
