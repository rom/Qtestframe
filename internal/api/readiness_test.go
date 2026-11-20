package api

import (
	"strings"
	"testing"
)

// strictPolicy restores the default execution policy (newEnv relaxes it).
func strictPolicy(t *testing.T, a *client) {
	t.Helper()
	a.m("PUT", "/api/v1/settings", map[string]any{}, 200)
}

func issueKinds(rd any) string {
	var out []string
	for _, x := range rd.(map[string]any)["issues"].([]any) {
		i := x.(map[string]any)
		b := ""
		if i["blocking"] == true {
			b = "!"
		}
		out = append(out, i["kind"].(string)+":"+i["key"].(string)+b)
	}
	return strings.Join(out, ",")
}

func TestExecutionReadiness(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	strictPolicy(t, a)
	pid := a.project("RDY")
	req := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "status": "draft"}, 201)
	tc := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Valid login", "status": "draft"}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": req["id"], "test_case_id": tc["id"]}, 201)
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R", "test_ids": []string{tc["id"].(string)}}, 201)
	ex := items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200))[0]
	if got := issueKinds(ex["readiness"]); got != "test_status:RDY-TC-1!,requirement_status:RDY-REQ-1!" {
		t.Fatalf("list readiness: %s", got)
	}
	exID := ex["id"].(string)

	// starting is refused; assigning, notes and blocked/skipped are fine
	res := a.m("PATCH", "/api/v1/executions/"+exID, map[string]any{"status": "passed"}, 409)
	if res["code"] != "not_ready" || !strings.Contains(res["error"].(string), `the test RDY-TC-1 has status "draft", but only tests with status "ready" may be executed`) ||
		!strings.Contains(res["error"].(string), `requirement RDY-REQ-1, which has status "draft", but the execution settings require linked requirements to be "approved", "implemented" or "verified"`) {
		t.Fatal(res)
	}
	for _, x := range res["readiness"].(map[string]any)["issues"].([]any) {
		i := x.(map[string]any)
		want := map[string]string{"test_status": "allow_unready_tests|ready", "requirement_status": "allow_unready_requirements|approved,implemented,verified"}[i["kind"].(string)]
		if got := i["setting"].(string) + "|" + strings.Join(strs(i["required"]), ","); got != want {
			t.Fatal(got, want)
		}
	}
	a.m("PATCH", "/api/v1/executions/"+exID, map[string]any{"assignee": "@admin", "notes": "waiting"}, 200)
	a.m("PATCH", "/api/v1/executions/missing", map[string]any{"status": "passed"}, 404)
	a.m("PATCH", "/api/v1/executions/"+exID, map[string]any{"status": "blocked"}, 200)
	d := a.m("GET", "/api/v1/executions/"+exID, nil, 200)
	if d["readiness"].(map[string]any)["allowed"] != false {
		t.Error(d["readiness"])
	}

	// test ready: the draft requirement still blocks; allowing unready requirements turns it into a warning
	a.m("PATCH", "/api/v1/tests/"+tc["id"].(string), map[string]any{"status": "ready"}, 200)
	a.m("PATCH", "/api/v1/executions/"+exID, map[string]any{"status": "passed"}, 409)
	a.m("PUT", "/api/v1/settings", map[string]any{"allow_unready_requirements": true}, 200)
	d = a.m("GET", "/api/v1/executions/"+exID, nil, 200)
	if rd := d["readiness"].(map[string]any); rd["allowed"] != true || rd["ready"] != false || issueKinds(rd) != "requirement_status:RDY-REQ-1" {
		t.Fatal(rd)
	}
	a.m("PATCH", "/api/v1/executions/"+exID, map[string]any{"status": "passed"}, 200)
	strictPolicy(t, a)
	// the result is recorded; saving it again (e.g. with new notes) is not a new execution
	a.m("PATCH", "/api/v1/executions/"+exID, map[string]any{"status": "passed", "notes": "rechecked"}, 200)
	a.m("PATCH", "/api/v1/executions/"+exID, map[string]any{"status": "failed"}, 409)
	a.m("PATCH", "/api/v1/requirements/"+req["id"].(string), map[string]any{"status": "approved"}, 200)
	if rd := a.m("GET", "/api/v1/executions/"+exID, nil, 200)["readiness"].(map[string]any); rd["ready"] != true {
		t.Fatal(rd)
	}

	// readiness of tests outside a run
	rd := a.m("GET", "/api/v1/projects/"+pid+"/readiness?test_ids="+tc["id"].(string)+",unknown", nil, 200)
	if tests := rd["tests"].(map[string]any); len(tests) != 1 || tests[tc["id"].(string)].(map[string]any)["ready"] != true {
		t.Fatal(rd)
	}
	if all := a.m("GET", "/api/v1/projects/"+pid+"/readiness", nil, 200)["tests"].(map[string]any); len(all) != 1 {
		t.Fatal(all)
	}

	// CI import skips results of tests that may not run
	draft := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Draft"}, 201)
	imp := a.m("POST", "/api/v1/runs/"+run["id"].(string)+"/results", map[string]any{"results": []map[string]any{
		{"test_key": "RDY-TC-1", "status": "failed"}, {"test_key": draft["key"], "status": "passed"}, {"test_key": draft["key"], "status": "skipped"}, {"test_key": "NOPE-1", "status": "passed"},
	}}, 200)
	if imp["updated"].(float64) != 2 || len(imp["rejected"].([]any)) != 1 || len(imp["unknown"].([]any)) != 1 {
		t.Fatal(imp)
	}
}

func TestTestDependencies(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	strictPolicy(t, a)
	a.m("PUT", "/api/v1/settings", map[string]any{"allow_unready_tests": true}, 200)
	pid := a.project("DEP")
	mk := func(title string) string {
		return a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": title}, 201)["id"].(string)
	}
	login, order, pay := mk("Login"), mk("Order"), mk("Pay")
	other := a.project("OTH")
	foreign := a.m("POST", "/api/v1/projects/"+other+"/tests", map[string]any{"title": "x"}, 201)["id"].(string)

	d1 := a.m("POST", "/api/v1/tests/"+order+"/dependencies", map[string]any{"depends_on_id": login}, 201)
	if d1["test_key"] != "DEP-TC-2" || d1["depends_on_key"] != "DEP-TC-1" {
		t.Fatal(d1)
	}
	a.m("POST", "/api/v1/tests/"+pay+"/dependencies", map[string]any{"depends_on_id": order}, 201)
	a.m("POST", "/api/v1/tests/"+order+"/dependencies", map[string]any{"depends_on_id": login}, 409)   // duplicate
	a.m("POST", "/api/v1/tests/"+login+"/dependencies", map[string]any{"depends_on_id": pay}, 400)     // cycle
	a.m("POST", "/api/v1/tests/"+login+"/dependencies", map[string]any{"depends_on_id": login}, 400)   // self
	a.m("POST", "/api/v1/tests/"+login+"/dependencies", map[string]any{"depends_on_id": foreign}, 400) // other project
	a.m("POST", "/api/v1/tests/"+login+"/dependencies", map[string]any{"depends_on_id": "missing"}, 404)

	deps := items(a.m("GET", "/api/v1/tests/"+order+"/dependencies", nil, 200))
	if len(deps) != 2 || deps[0]["direction"] != "outgoing" || deps[0]["other_last_result"] != "not_run" || deps[1]["direction"] != "incoming" {
		t.Fatal(deps)
	}
	if all := items(a.m("GET", "/api/v1/projects/"+pid+"/test-dependencies", nil, 200)); len(all) != 2 {
		t.Fatal(all)
	}
	if got := items(a.m("GET", "/api/v1/projects/"+pid+"/tests?q=depends:DEP-TC-1", nil, 200)); len(got) != 1 || got[0]["id"] != order {
		t.Fatal(got)
	}
	var orderRow map[string]any
	for _, x := range items(a.m("GET", "/api/v1/projects/"+pid+"/tests", nil, 200)) {
		if x["id"] == order {
			orderRow = x
		}
	}
	if s := (func() string {
		var out []string
		for _, r := range orderRow["relations"].([]any) {
			m := r.(map[string]any)
			out = append(out, m["type"].(string)+"/"+m["direction"].(string)+"/"+m["kind"].(string)+"/"+m["key"].(string))
		}
		return strings.Join(out, ",")
	})(); s != "depends_on/outgoing/test/DEP-TC-1,depends_on/incoming/test/DEP-TC-3" {
		t.Errorf("relations: %s", s)
	}

	// in a run: Order waits for Login's execution in the same run
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R", "test_ids": []string{login, order}}, 201)
	exs := map[string]string{}
	for _, x := range items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200)) {
		exs[x["test_case_id"].(string)] = x["id"].(string)
	}
	res := a.m("PATCH", "/api/v1/executions/"+exs[order], map[string]any{"step_results": []any{}, "status": "in_progress"}, 409)
	if !strings.Contains(res["error"].(string), `the prerequisite test DEP-TC-1 has not passed (its result in this run is "not_run")`) {
		t.Fatal(res)
	}
	a.m("PATCH", "/api/v1/executions/"+exs[login], map[string]any{"status": "failed"}, 200)
	a.m("PATCH", "/api/v1/executions/"+exs[order], map[string]any{"status": "passed"}, 409)
	a.m("PATCH", "/api/v1/executions/"+exs[login], map[string]any{"status": "passed"}, 200)
	a.m("PATCH", "/api/v1/executions/"+exs[order], map[string]any{"status": "passed"}, 200)

	// outside the run, the latest result counts: Pay's prerequisite Order has passed
	run2 := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R2", "test_ids": []string{pay}}, 201)
	ex2 := items(a.m("GET", "/api/v1/runs/"+run2["id"].(string)+"/executions", nil, 200))[0]
	if got := issueKinds(ex2["readiness"]); got != "test_status:DEP-TC-3" { // only the (allowed) draft status remains
		t.Fatal(got)
	}
	// allowing unmet dependencies turns them into warnings
	a.m("PATCH", "/api/v1/executions/"+exs[login], map[string]any{"status": "failed"}, 200)
	a.m("PUT", "/api/v1/settings", map[string]any{"allow_unready_tests": true, "allow_unmet_dependencies": true}, 200)
	a.m("PATCH", "/api/v1/executions/"+exs[order], map[string]any{"status": "failed"}, 200)

	// delete
	a.call("DELETE", "/api/v1/test-dependencies/"+d1["id"].(string), nil, 204, nil)
	a.m("DELETE", "/api/v1/test-dependencies/"+d1["id"].(string), nil, 404)
	e.member("viewer@example.com", "viewer")
	v := e.login("viewer@example.com")
	v.m("POST", "/api/v1/tests/"+login+"/dependencies", map[string]any{"depends_on_id": pay}, 403)
}

func TestGraphAndTags(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("GRA")
	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "A", "tags": []string{"login", "security"}}, 201)["id"].(string)
	r2 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "B", "tags": []string{"login"}}, 201)["id"].(string)
	a.m("POST", "/api/v1/requirements/"+r2+"/relations", map[string]any{"target_id": r1, "type": "depends_on"}, 201)
	t1 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "T1", "tags": []string{"Login"}}, 201)["id"].(string)
	t2 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "T2", "steps": []map[string]any{{"call_test_id": t1}}}, 201)["id"].(string)
	a.m("POST", "/api/v1/tests/"+t2+"/dependencies", map[string]any{"depends_on_id": t1}, 201)

	g := a.m("GET", "/api/v1/projects/"+pid+"/graph", nil, 200)
	if len(g["nodes"].([]any)) != 2 || len(g["edges"].([]any)) != 1 || g["edges"].([]any)[0].(map[string]any)["type"] != "depends_on" {
		t.Fatal(g)
	}
	g = a.m("GET", "/api/v1/projects/"+pid+"/graph?kind=test", nil, 200)
	types := []string{}
	for _, x := range g["edges"].([]any) {
		types = append(types, x.(map[string]any)["type"].(string))
	}
	if len(g["nodes"].([]any)) != 2 || strings.Join(types, ",") != "calls,depends_on" {
		t.Fatal(g)
	}
	if n := g["nodes"].([]any)[0].(map[string]any); n["result"] != "not_run" {
		t.Error(n)
	}
	a.m("GET", "/api/v1/projects/"+pid+"/graph?kind=run", nil, 400)

	tags := items(a.m("GET", "/api/v1/projects/"+pid+"/tags", nil, 200))
	if len(tags) != 3 || tags[0]["tag"] != "login" || tags[0]["requirements"].(float64) != 2 || tags[0]["total"].(float64) != 2 {
		t.Fatal(tags)
	}
	e.member("lim@example.com", "viewer")
	a.m("PUT", "/api/v1/roles/viewer", map[string]any{"permissions": []string{"project.read", "requirement.read"}}, 200)
	v := e.login("lim@example.com")
	v.m("GET", "/api/v1/projects/"+pid+"/graph?kind=test", nil, 403)
	v.m("GET", "/api/v1/projects/"+pid+"/graph?kind=requirement", nil, 200)
}
