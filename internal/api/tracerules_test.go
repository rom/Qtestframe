package api

import (
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func TestTraceRules(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("TRR")
	req := func(title, desc, typ string) string {
		return a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": title, "description": desc, "type": typ}, 201)["id"].(string)
	}
	r1 := req("Login", "The system MUST log in.", "functional")
	r2 := req("Lockout", "The system MUST lock accounts.", "security")
	r3 := req("Business goal", "Customers SHOULD bank online.", "business")
	r4 := req("Child", "The system MAY show a banner.", "functional")
	test := func(title string) string {
		return a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": title}, 201)["id"].(string)
	}
	t1 := test("Valid login")
	t2 := test("Orphan")
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": r1, "test_case_id": t1}, 201)
	a.m("POST", "/api/v1/requirements/"+r4+"/relations", map[string]any{"target_id": r3, "type": "refines"}, 201)

	base := "/api/v1/projects/" + pid + "/trace-rules"
	// validation
	if e := a.m("POST", base, map[string]any{"name": "x", "kind": "verified_by", "selector": "nofield:x"}, 400); e["code"] != "invalid_query" || !strings.HasPrefix(e["error"].(string), "selector: ") || e["position"].(float64) != 0 {
		t.Fatal(e)
	}
	a.m("POST", base, map[string]any{"name": "x", "kind": "verified_by", "target_query": "("}, 400)
	a.m("POST", base, map[string]any{"name": "x", "kind": "verifies", "scope": "requirement"}, 400)
	a.m("POST", base, map[string]any{"name": "", "kind": "verified_by"}, 400)
	a.m("POST", base, nil, 400)

	must := a.m("POST", base, map[string]any{"name": "MUST verified", "kind": "verified_by", "selector": "obligation:must", "description": "d"}, 201)
	if must["enabled"] != true || must["min"].(float64) != 1 || must["severity"] != "error" || must["scope"] != "requirement" {
		t.Fatal(must)
	}
	a.m("POST", base, map[string]any{"name": "MUST verified", "kind": "verified_by"}, 409)
	tests := a.m("POST", base, map[string]any{"name": "Tests verify", "kind": "verifies", "severity": "warning"}, 201)
	refined := a.m("POST", base, map[string]any{"name": "Business refined", "kind": "relation", "selector": "type:business",
		"relation_type": "refines", "direction": "incoming", "target_query": "type:functional", "min": 1}, 201)
	off := a.m("POST", base, map[string]any{"name": "Links", "kind": "item_link", "enabled": false}, 201)

	list := items(a.m("GET", base, nil, 200))
	if len(list) != 4 || list[0]["id"] != must["id"] || list[2]["relation_type"] != "refines" {
		t.Fatal(list)
	}
	if g := a.m("GET", "/api/v1/trace-rules/"+off["id"].(string), nil, 200); g["kind"] != "item_link" || g["enabled"] != false {
		t.Fatal(g)
	}

	ev := a.m("GET", base+"/evaluate", nil, 200)
	results := ev["results"].([]any)
	sum := ev["summary"].(map[string]any)
	if len(results) != 3 || ev["passed"] != false || sum["failed"].(float64) != 2 || sum["errors"].(float64) != 1 || sum["warnings"].(float64) != 1 || sum["violations"].(float64) != 2 {
		t.Fatal(ev)
	}
	first := results[0].(map[string]any)
	viol := first["violations"].([]any)
	if first["matched"].(float64) != 2 || first["passed"] != false || len(viol) != 1 || viol[0].(map[string]any)["id"] != r2 || viol[0].(map[string]any)["key"] == "" {
		t.Fatal(first)
	}
	if results[2].(map[string]any)["passed"] != true {
		t.Fatal(results[2])
	}
	// all rules, one rule, limit
	if ev := a.m("GET", base+"/evaluate?all=1&limit=1", nil, 200); len(ev["results"].([]any)) != 4 {
		t.Fatal(ev)
	}
	one := a.m("GET", base+"/evaluate?rule_id="+off["id"].(string), nil, 200)
	if r := one["results"].([]any); len(r) != 1 || r[0].(map[string]any)["violation_count"].(float64) != 4 {
		t.Fatal(one)
	}
	a.m("GET", base+"/evaluate?rule_id=nope", nil, 404)

	// preview of an unsaved rule
	pv := a.m("POST", base+"/preview", map[string]any{"name": "p", "kind": "verified_by", "min": 2}, 200)
	if pv["violation_count"].(float64) != 4 || pv["rule"].(map[string]any)["id"] != "" {
		t.Fatal(pv)
	}
	a.m("POST", base+"/preview", map[string]any{"name": "p", "kind": "verified_by", "selector": "("}, 400)

	// per item
	if v := items(a.m("GET", "/api/v1/requirements/"+r2+"/trace-rules", nil, 200)); len(v) != 1 || v[0]["rule"].(map[string]any)["id"] != must["id"] || v[0]["required"].(float64) != 1 || v[0]["found"].(float64) != 0 {
		t.Fatal(v)
	}
	if v := items(a.m("GET", "/api/v1/requirements/"+r1+"/trace-rules", nil, 200)); len(v) != 0 {
		t.Fatal(v)
	}
	if v := items(a.m("GET", "/api/v1/tests/"+t2+"/trace-rules", nil, 200)); len(v) != 1 || v[0]["rule"].(map[string]any)["id"] != tests["id"] {
		t.Fatal(v)
	}
	a.m("GET", "/api/v1/tests/nope/trace-rules", nil, 404)
	a.m("GET", "/api/v1/requirements/nope/trace-rules", nil, 404)

	// update: severity warning makes the project pass; disabled rules are skipped
	upd := a.m("PATCH", "/api/v1/trace-rules/"+must["id"].(string), map[string]any{"severity": "warning", "min": 1}, 200)
	if upd["severity"] != "warning" || upd["selector"] != "obligation:must" {
		t.Fatal(upd)
	}
	if ev := a.m("GET", base+"/evaluate", nil, 200); ev["passed"] != true {
		t.Fatal(ev)
	}
	a.m("PATCH", "/api/v1/trace-rules/"+must["id"].(string), map[string]any{"selector": "("}, 400)
	a.m("PATCH", "/api/v1/trace-rules/"+must["id"].(string), map[string]any{"name": "Tests verify"}, 409)
	a.m("PATCH", "/api/v1/trace-rules/"+must["id"].(string), nil, 400)
	a.m("PATCH", "/api/v1/trace-rules/nope", map[string]any{"name": "x"}, 404)
	a.m("PATCH", "/api/v1/trace-rules/"+refined["id"].(string), map[string]any{"enabled": false}, 200)

	// permissions: a viewer reads and evaluates, a tester cannot change rules
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	v.m("GET", base, nil, 200)
	v.m("GET", base+"/evaluate", nil, 200)
	v.m("GET", "/api/v1/requirements/"+r2+"/trace-rules", nil, 200)
	e.member("tester@example.com", rbac.RoleTester)
	ts := e.login("tester@example.com")
	ts.m("POST", base, map[string]any{"name": "x", "kind": "verified_by"}, 403)
	ts.m("PATCH", "/api/v1/trace-rules/"+must["id"].(string), map[string]any{"name": "y"}, 403)
	ts.m("DELETE", "/api/v1/trace-rules/"+must["id"].(string), nil, 403)

	// release exit criterion on the scope (R2 only): errors only vs warnings too
	a.m("PATCH", "/api/v1/trace-rules/"+must["id"].(string), map[string]any{"severity": "error"}, 200)
	rel := a.m("POST", "/api/v1/projects/"+pid+"/releases", map[string]any{"name": "1.0", "exit_criteria": []any{map[string]any{"type": "trace_rules"}}}, 201)
	rid := rel["id"].(string)
	if c := rel["exit_criteria"].([]any)[0].(map[string]any); c["level"] != "error" {
		t.Fatal(rel)
	}
	st := a.m("GET", "/api/v1/releases/"+rid+"/status", nil, 200)
	if c := st["criteria"].([]any)[0].(map[string]any); c["met"] != true || c["available"] != true {
		t.Fatal(st) // empty scope: nothing breaks a rule
	}
	a.m("POST", "/api/v1/releases/"+rid+"/requirements", map[string]any{"add": []string{r1, r2}}, 200)
	st = a.m("GET", "/api/v1/releases/"+rid+"/status", nil, 200)
	c := st["criteria"].([]any)[0].(map[string]any)
	its := c["items"].([]any)
	if c["met"] != false || len(its) != 1 || its[0].(map[string]any)["id"] != r2 || its[0].(map[string]any)["note"] != "MUST verified" || st["go"] != false {
		t.Fatal(st)
	}
	// the scope test t1 verifies a requirement: the warning rule passes there
	a.m("PATCH", "/api/v1/releases/"+rid, map[string]any{"exit_criteria": []any{map[string]any{"type": "trace_rules", "level": "warning"}}}, 200)
	a.m("PATCH", "/api/v1/trace-rules/"+must["id"].(string), map[string]any{"severity": "warning"}, 200)
	a.m("POST", base, map[string]any{"name": "Also tested twice", "kind": "verified_by", "min": 2, "severity": "warning"}, 201)
	st = a.m("GET", "/api/v1/releases/"+rid+"/status", nil, 200)
	c = st["criteria"].([]any)[0].(map[string]any)
	if its := c["items"].([]any); c["met"] != false || len(its) != 2 || !strings.Contains(its[0].(map[string]any)["note"].(string), "; ") {
		t.Fatal(c)
	}
	a.m("PATCH", "/api/v1/releases/"+rid, map[string]any{"exit_criteria": []any{map[string]any{"type": "trace_rules", "level": "fatal"}}}, 400)

	// traceability report lists the rules and their violations
	_, data := a.download("/api/v1/projects/"+pid+"/reports/generate", map[string]any{"template": "traceability", "format": "csv"}, 200)
	files := zipFiles(t, data)
	if !strings.Contains(files["trace_rules.csv"], "MUST verified") || !strings.Contains(files["trace_rule_violations.csv"], "Lockout") {
		t.Fatal(files)
	}

	// delete
	a.m("DELETE", "/api/v1/trace-rules/"+off["id"].(string), nil, 204)
	a.m("DELETE", "/api/v1/trace-rules/"+off["id"].(string), nil, 404)
	a.m("GET", "/api/v1/trace-rules/"+off["id"].(string), nil, 404)
	_ = refined
}

func TestTraceRulesNoRules(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("TRN")
	ev := a.m("GET", "/api/v1/projects/"+pid+"/trace-rules/evaluate", nil, 200)
	if len(ev["results"].([]any)) != 0 || ev["passed"] != true {
		t.Fatal(ev)
	}
	a.m("GET", "/api/v1/projects/nope/trace-rules", nil, 404)
	a.m("GET", "/api/v1/projects/nope/trace-rules/evaluate", nil, 404)
	a.m("POST", "/api/v1/projects/nope/trace-rules", map[string]any{"name": "x", "kind": "verifies"}, 404)
	a.m("POST", "/api/v1/projects/nope/trace-rules/preview", map[string]any{"name": "x", "kind": "verifies"}, 404)
	// no trace_rules section without rules
	_, data := a.download("/api/v1/projects/"+pid+"/reports/generate", map[string]any{"template": "traceability", "format": "csv"}, 200)
	if _, ok := zipFiles(t, data)["trace_rules.csv"]; ok {
		t.Fatal("unexpected section")
	}
}
