package api

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

func cfgIDs(list []map[string]any) []string {
	var out []string
	for _, c := range list {
		out = append(out, c["id"].(string))
	}
	return out
}

func execsOf(t *testing.T, a *client, runID string) []map[string]any {
	t.Helper()
	return items(a.m("GET", "/api/v1/runs/"+runID+"/executions", nil, 200))
}

func TestConfigurationsCRUD(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("CFG")
	base := "/api/v1/projects/" + pid + "/configurations"
	c1 := a.m("POST", base, map[string]any{"values": []map[string]string{{"dimension": " Browser ", "value": "Chrome"}, {"dimension": "OS", "value": "Windows"}}}, 201)
	if c1["name"] != "Chrome / Windows" || c1["position"].(float64) != 0 {
		t.Fatal(c1)
	}
	c2 := a.m("POST", base, map[string]any{"name": "Firefox", "description": "latest", "values": []map[string]string{{"dimension": "Browser", "value": "Firefox"}}}, 201)
	if c2["position"].(float64) != 1 {
		t.Fatal(c2)
	}
	for _, bad := range []map[string]any{
		{},
		{"name": "x", "values": []map[string]string{{"dimension": "", "value": "v"}}},
		{"name": "y", "values": []map[string]string{{"dimension": "A", "value": "1"}, {"dimension": "a", "value": "2"}}},
		{"name": strings.Repeat("n", 201)},
		{"name": "z", "values": []map[string]string{{"dimension": strings.Repeat("d", 101), "value": "v"}}},
		{"name": "w", "description": strings.Repeat("d", 2001)},
	} {
		a.m("POST", base, bad, 400)
	}
	many := []map[string]string{}
	for i := 0; i < 21; i++ {
		many = append(many, map[string]string{"dimension": fmt.Sprint("D", i), "value": "v"})
	}
	a.m("POST", base, map[string]any{"name": "many", "values": many}, 400)
	a.m("POST", base, map[string]any{"name": "FIREFOX"}, 409)
	a.m("POST", "/api/v1/projects/missing/configurations", map[string]any{"name": "x"}, 404)

	up := a.m("PATCH", "/api/v1/configurations/"+c2["id"].(string), map[string]any{"name": "Firefox ESR", "position": 5, "description": "d", "values": []map[string]string{{"dimension": "Browser", "value": "Firefox ESR"}}}, 200)
	if up["name"] != "Firefox ESR" || up["position"].(float64) != 5 {
		t.Fatal(up)
	}
	a.m("PATCH", "/api/v1/configurations/"+c2["id"].(string), map[string]any{"name": "chrome / windows"}, 409)
	a.m("PATCH", "/api/v1/configurations/"+c2["id"].(string), map[string]any{"name": "", "values": []map[string]string{}}, 400)
	a.m("PATCH", "/api/v1/configurations/missing", map[string]any{"name": "x"}, 404)
	a.m("GET", "/api/v1/configurations/"+c1["id"].(string), nil, 200)
	a.m("GET", "/api/v1/configurations/missing", nil, 404)

	// bulk create (combinations), skipping names that exist
	bulk := a.m("POST", base+"/bulk", map[string]any{"skip_existing": true, "configurations": []map[string]any{
		{"values": []map[string]string{{"dimension": "Browser", "value": "Chrome"}, {"dimension": "OS", "value": "Windows"}}},
		{"values": []map[string]string{{"dimension": "Browser", "value": "Safari"}, {"dimension": "OS", "value": "macOS"}}},
	}}, 200)
	if len(bulk["created"].([]any)) != 1 || len(bulk["skipped"].([]any)) != 1 {
		t.Fatal(bulk)
	}
	a.m("POST", base+"/bulk", map[string]any{"configurations": []map[string]any{{"name": "New"}, {"name": "Firefox ESR"}}}, 409)
	a.m("POST", base+"/bulk", map[string]any{"configurations": []map[string]any{{"name": "ok"}, {"values": []map[string]string{{"dimension": "x"}}}}}, 400)
	a.m("POST", base+"/bulk", map[string]any{"configurations": []map[string]any{}}, 400)
	list := items(a.m("GET", base, nil, 200))
	if len(list) != 3 || list[0]["name"] != "Chrome / Windows" || list[2]["name"] != "Safari / macOS" {
		t.Fatal(list)
	}
	// all or nothing: "New" from the failed bulk request is not there
	for _, c := range list {
		if c["name"] == "New" || c["name"] == "ok" {
			t.Fatal(list)
		}
	}

	// permissions
	e.member("tester@example.com", "tester")
	tester := e.login("tester@example.com")
	tester.m("GET", base, nil, 200)
	tester.m("POST", base, map[string]any{"name": "x"}, 403)
	tester.m("PATCH", "/api/v1/configurations/"+c1["id"].(string), map[string]any{"name": "x"}, 403)
	tester.m("DELETE", "/api/v1/configurations/"+c1["id"].(string), nil, 403)

	// a configuration used by a run cannot be deleted
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R", "configuration_ids": []string{c1["id"].(string)}}, 201)
	if len(run["configurations"].([]any)) != 1 {
		t.Fatal(run)
	}
	a.m("DELETE", "/api/v1/configurations/"+c1["id"].(string), nil, 409)
	a.m("DELETE", base+"/bulk", map[string]any{"ids": []string{c2["id"].(string), c1["id"].(string)}}, 409)
	if len(items(a.m("GET", base, nil, 200))) != 3 {
		t.Fatal("bulk delete was not undone")
	}
	other := a.project("OTH")
	oc := a.m("POST", "/api/v1/projects/"+other+"/configurations", map[string]any{"name": "Other"}, 201)
	a.m("DELETE", base+"/bulk", map[string]any{"ids": []string{oc["id"].(string)}}, 404)
	a.m("DELETE", base+"/bulk", map[string]any{"ids": []string{}}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R2", "configuration_ids": []string{oc["id"].(string)}}, 400)
	if d := a.m("DELETE", base+"/bulk", map[string]any{"ids": []string{c2["id"].(string)}}, 200); d["deleted"].(float64) != 1 {
		t.Fatal(d)
	}
	a.m("DELETE", "/api/v1/configurations/"+bulk["created"].([]any)[0].(map[string]any)["id"].(string), nil, 204)
	a.m("DELETE", "/api/v1/configurations/missing", nil, 404)
	if l := items(a.m("GET", base, nil, 200)); len(l) != 1 || l[0]["run_count"].(float64) != 1 {
		t.Fatal(l)
	}
}

func TestRunsWithConfigurations(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("RC")
	base := "/api/v1/projects/" + pid
	chrome := a.m("POST", base+"/configurations", map[string]any{"name": "Chrome"}, 201)["id"].(string)
	firefox := a.m("POST", base+"/configurations", map[string]any{"name": "Firefox"}, 201)["id"].(string)
	safari := a.m("POST", base+"/configurations", map[string]any{"name": "Safari"}, 201)["id"].(string)
	t1 := a.m("POST", base+"/tests", map[string]any{"title": "Login", "steps": []map[string]any{{"action": "open", "expected": "ok"}}}, 201)
	t2 := a.m("POST", base+"/tests", map[string]any{"title": "Logout"}, 201)
	id1, id2 := t1["id"].(string), t2["id"].(string)

	run := a.m("POST", base+"/runs", map[string]any{"name": "Cross", "test_ids": []string{id1}, "configuration_ids": []string{chrome, firefox, chrome}}, 201)
	rid := run["id"].(string)
	if run["summary"].(map[string]any)["total"].(float64) != 2 || len(run["configurations"].([]any)) != 2 {
		t.Fatal(run)
	}
	ex := execsOf(t, a, rid)
	if len(ex) != 2 || ex[0]["configuration_name"] != "Chrome" || ex[1]["configuration_name"] != "Firefox" || ex[0]["iteration_count"].(float64) != 0 {
		t.Fatal(ex)
	}
	// adding tests creates one execution per configuration
	if r := a.m("POST", "/api/v1/runs/"+rid+"/executions", map[string]any{"test_ids": []string{id1, id2}}, 200); r["added"].(float64) != 2 {
		t.Fatal(r)
	}
	if l := items(a.m("GET", "/api/v1/runs/"+rid+"/executions?q="+url.QueryEscape("configuration:Firefox"), nil, 200)); len(l) != 2 {
		t.Fatal(l)
	}
	if l := items(a.m("GET", base+"/runs?q="+url.QueryEscape("config:Chrome"), nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	if l := items(a.m("GET", base+"/runs", nil, 200)); len(l[0]["configurations"].([]any)) != 2 {
		t.Fatal(l)
	}
	ex = execsOf(t, a, rid)
	passed := "passed"
	a.m("PATCH", "/api/v1/executions/"+ex[0]["id"].(string), map[string]any{"status": passed}, 200)
	// add Safari: one execution per test; remove Firefox (no results yet)
	ch := a.m("PUT", "/api/v1/runs/"+rid+"/configurations", map[string]any{"configuration_ids": []string{chrome, safari}}, 200)
	if ch["added"].(float64) != 2 || ch["removed"].(float64) != 2 || len(ch["run"].(map[string]any)["configurations"].([]any)) != 2 {
		t.Fatal(ch)
	}
	// Chrome has a result: removing it is refused (and nothing changes)
	a.m("PUT", "/api/v1/runs/"+rid+"/configurations", map[string]any{"configuration_ids": []string{safari}}, 409)
	if len(execsOf(t, a, rid)) != 4 {
		t.Fatal("refused change was applied")
	}
	a.m("PUT", "/api/v1/runs/"+rid+"/configurations", map[string]any{"configuration_ids": []string{"missing"}}, 400)
	a.m("PUT", "/api/v1/runs/missing/configurations", map[string]any{"configuration_ids": []string{}}, 404)
	a.m("PUT", "/api/v1/runs/"+rid+"/configurations", nil, 400)
	// PATCH also takes configuration_ids; the run completes once all have a result
	a.m("PATCH", "/api/v1/runs/"+rid, map[string]any{"configuration_ids": []string{chrome}}, 200)
	ex = execsOf(t, a, rid)
	if len(ex) != 2 {
		t.Fatal(ex)
	}
	a.m("PATCH", "/api/v1/executions/"+ex[1]["id"].(string), map[string]any{"status": passed}, 200)
	if r := a.m("GET", "/api/v1/runs/"+rid, nil, 200); r["status"] != "completed" {
		t.Fatal(r)
	}
	// removing the last configuration keeps its executions without configuration
	a.m("PUT", "/api/v1/runs/"+rid+"/configurations", map[string]any{"configuration_ids": []string{}}, 200)
	ex = execsOf(t, a, rid)
	if len(ex) != 2 || ex[0]["configuration_id"] != nil || ex[0]["status"] != "passed" {
		t.Fatal(ex)
	}
	// the first configuration of a run without any takes its executions
	a.m("PUT", "/api/v1/runs/"+rid+"/configurations", map[string]any{"configuration_ids": []string{firefox, safari}}, 200)
	ex = execsOf(t, a, rid)
	if len(ex) != 4 || ex[0]["configuration_name"] != "Firefox" || ex[0]["status"] != "passed" {
		t.Fatal(ex)
	}
	if r := a.m("GET", "/api/v1/runs/"+rid, nil, 200); r["status"] != "in_progress" {
		t.Fatal(r)
	}
	// history of a test shows the configuration
	h := items(a.m("GET", "/api/v1/tests/"+id1+"/executions", nil, 200))
	if len(h) != 2 || h[0]["configuration_name"] == "" {
		t.Fatal(h)
	}
	// export carries the configuration
	res := a.raw("GET", base+"/export?format=csv&aspects=runs", nil)
	body := readAll(t, res)
	if !strings.Contains(body, "Configuration") || !strings.Contains(body, "Safari") {
		t.Fatal(body)
	}
	// a run without configurations keeps working as before
	plain := a.m("POST", base+"/runs", map[string]any{"name": "Plain", "test_ids": []string{id1, id2}}, 201)
	if l := execsOf(t, a, plain["id"].(string)); len(l) != 2 || l[0]["configuration_id"] != nil || len(plain["configurations"].([]any)) != 0 {
		t.Fatal(l, plain)
	}
}

func readAll(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestImportResultsWithConfiguration(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("IMP")
	base := "/api/v1/projects/" + pid
	chrome := a.m("POST", base+"/configurations", map[string]any{"name": "Chrome"}, 201)["id"].(string)
	a.m("POST", base+"/configurations", map[string]any{"name": "Firefox"}, 201)
	t1 := a.m("POST", base+"/tests", map[string]any{"title": "A"}, 201)
	key := t1["key"].(string)
	run := a.m("POST", base+"/runs", map[string]any{"name": "CI"}, 201)
	rid := run["id"].(string)
	path := "/api/v1/runs/" + rid + "/results"
	// default configuration from the query; the run gets it
	r := a.m("POST", path+"?configuration=chrome", map[string]any{"results": []map[string]any{{"test_key": key, "status": "passed"}}}, 200)
	if r["updated"].(float64) != 1 {
		t.Fatal(r)
	}
	// per result configuration; Firefox is added to the run
	r = a.m("POST", path, map[string]any{"results": []map[string]any{{"test_key": key, "status": "failed", "configuration": "Firefox"}}}, 200)
	if r["updated"].(float64) != 1 {
		t.Fatal(r)
	}
	ex := execsOf(t, a, rid)
	if len(ex) != 2 || ex[0]["configuration_name"] != "Chrome" || ex[0]["status"] != "passed" || ex[1]["status"] != "failed" {
		t.Fatal(ex)
	}
	// several configurations and none named
	a.m("POST", path, map[string]any{"results": []map[string]any{{"test_key": key, "status": "passed"}}}, 400)
	a.m("POST", path+"?configuration=Opera", map[string]any{"results": []map[string]any{{"test_key": key, "status": "passed"}}}, 400)
	// a run with one configuration uses it by default
	one := a.m("POST", base+"/runs", map[string]any{"name": "One", "configuration_ids": []string{chrome}}, 201)
	a.m("POST", "/api/v1/runs/"+one["id"].(string)+"/results", map[string]any{"results": []map[string]any{{"test_key": key, "status": "passed"}}}, 200)
	if ex := execsOf(t, a, one["id"].(string)); len(ex) != 1 || ex[0]["configuration_name"] != "Chrome" {
		t.Fatal(ex)
	}
	// JUnit import with ?configuration=
	xml := `<testsuite><testcase name="` + key + ` works"/></testsuite>`
	ju := a.postXML("/api/v1/runs/"+rid+"/junit?configuration=Firefox", "application/xml", []byte(xml), 200)
	if ju["updated"].(float64) != 1 {
		t.Fatal(ju)
	}
	if ex := execsOf(t, a, rid); ex[1]["status"] != "passed" {
		t.Fatal(ex)
	}
	// readiness per configuration: a prerequisite must pass in the same configuration
	t2 := a.m("POST", base+"/tests", map[string]any{"title": "B"}, 201)
	a.m("POST", "/api/v1/tests/"+t2["id"].(string)+"/dependencies", map[string]any{"depends_on_id": t1["id"]}, 201)
	a.m("PUT", "/api/v1/settings", map[string]any{"allow_unready_tests": true, "allow_unready_requirements": true}, 200)
	a.m("PATCH", "/api/v1/executions/"+ex[1]["id"].(string), map[string]any{"status": "failed"}, 200)
	a.m("POST", "/api/v1/runs/"+rid+"/executions", map[string]any{"test_ids": []string{t2["id"].(string)}}, 200)
	res := a.m("POST", path+"?configuration=Chrome", map[string]any{"results": []map[string]any{{"test_key": t2["key"], "status": "passed"}}}, 200)
	if res["updated"].(float64) != 1 || len(res["rejected"].([]any)) != 0 {
		t.Fatal(res)
	}
	res = a.m("POST", path+"?configuration=Firefox", map[string]any{"results": []map[string]any{{"test_key": t2["key"], "status": "passed"}}}, 200)
	if res["updated"].(float64) != 0 || len(res["rejected"].([]any)) != 1 {
		t.Fatal(res)
	}
	for _, x := range execsOf(t, a, rid) {
		if x["test_key"] == t2["key"] {
			rd := x["readiness"].(map[string]any)
			if (x["configuration_name"] == "Firefox") == rd["allowed"].(bool) {
				t.Fatal(x)
			}
		}
	}
}

func TestCoveragePerConfiguration(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("COV")
	base := "/api/v1/projects/" + pid
	chrome := a.m("POST", base+"/configurations", map[string]any{"name": "Chrome"}, 201)["id"].(string)
	firefox := a.m("POST", base+"/configurations", map[string]any{"name": "Firefox"}, 201)["id"].(string)
	rq := a.m("POST", base+"/requirements", map[string]any{"title": "Login", "description": "MUST log in"}, 201)
	rid := rq["id"].(string)
	t1 := a.m("POST", base+"/tests", map[string]any{"title": "A"}, 201)
	a.m("POST", base+"/links", map[string]any{"requirement_id": rid, "test_case_id": t1["id"]}, 201)
	cov := a.m("GET", "/api/v1/requirements/"+rid+"/coverage", nil, 200)
	if cov["coverage"] != "not_run" || len(cov["configurations"].([]any)) != 0 {
		t.Fatal(cov)
	}
	run := a.m("POST", base+"/runs", map[string]any{"name": "X", "test_ids": []string{t1["id"].(string)}, "configuration_ids": []string{chrome, firefox}}, 201)
	ex := execsOf(t, a, run["id"].(string))
	a.m("PATCH", "/api/v1/executions/"+ex[0]["id"].(string), map[string]any{"status": "passed"}, 200)
	// passed in Chrome, not yet run in Firefox: partial
	if r := a.m("GET", "/api/v1/requirements/"+rid, nil, 200); r["coverage"] != "partial" {
		t.Fatal(r)
	}
	cov = a.m("GET", "/api/v1/requirements/"+rid+"/coverage", nil, 200)
	cfgs := cov["configurations"].([]any)
	if cov["coverage"] != "partial" || len(cfgs) != 2 || cfgs[0].(map[string]any)["coverage"] != "passed" || cfgs[1].(map[string]any)["coverage"] != "not_run" {
		t.Fatal(cov)
	}
	tst := cov["tests"].([]any)[0].(map[string]any)
	if tst["result"] != "in_progress" || tst["results"].(map[string]any)[chrome] != "passed" {
		t.Fatal(tst)
	}
	a.m("PATCH", "/api/v1/executions/"+ex[1]["id"].(string), map[string]any{"status": "passed"}, 200)
	if r := a.m("GET", "/api/v1/requirements/"+rid, nil, 200); r["coverage"] != "passed" {
		t.Fatal(r)
	}
	a.m("PATCH", "/api/v1/executions/"+ex[1]["id"].(string), map[string]any{"status": "failed"}, 200)
	tr := a.m("GET", base+"/traceability", nil, 200)
	if tr["requirements"].([]any)[0].(map[string]any)["coverage"] != "failed" {
		t.Fatal(tr)
	}
	tr = a.m("GET", base+"/traceability?configuration="+chrome, nil, 200)
	if tr["requirements"].([]any)[0].(map[string]any)["coverage"] != "passed" || tr["tests"].([]any)[0].(map[string]any)["last_result"] != "passed" || tr["configuration"] != chrome {
		t.Fatal(tr)
	}
	tr = a.m("GET", base+"/traceability?configuration=none", nil, 200)
	if tr["requirements"].([]any)[0].(map[string]any)["coverage"] != "not_run" || tr["tests"].([]any)[0].(map[string]any)["last_result"] != "not_run" {
		t.Fatal(tr)
	}
	a.m("GET", base+"/traceability?configuration=bogus", nil, 400)
	// releases: a test passes only when it passed in every configuration
	rel := a.m("POST", base+"/releases", map[string]any{"name": "1.0"}, 201)
	a.m("POST", "/api/v1/releases/"+rel["id"].(string)+"/requirements", map[string]any{"add": []string{rid}}, 200)
	a.m("PATCH", "/api/v1/runs/"+run["id"].(string), map[string]any{"release_id": rel["id"]}, 200)
	st := a.m("GET", "/api/v1/releases/"+rel["id"].(string)+"/status", nil, 200)
	if sum := st["summary"].(map[string]any); sum["failed"].(float64) != 1 || sum["executions"].(float64) != 2 {
		t.Fatal(st)
	}
	a.m("PATCH", "/api/v1/executions/"+ex[1]["id"].(string), map[string]any{"status": "passed"}, 200)
	if sum := a.m("GET", "/api/v1/releases/"+rel["id"].(string)+"/status", nil, 200)["summary"].(map[string]any); sum["passed"].(float64) != 1 {
		t.Fatal(sum)
	}
	a.m("PATCH", "/api/v1/executions/"+ex[1]["id"].(string), map[string]any{"status": "not_run"}, 200)
	if sum := a.m("GET", "/api/v1/releases/"+rel["id"].(string)+"/status", nil, 200)["summary"].(map[string]any); sum["not_run"].(float64) != 1 {
		t.Fatal(sum)
	}
	a.m("PATCH", "/api/v1/executions/"+ex[1]["id"].(string), map[string]any{"status": "failed"}, 200)
	a.m("GET", "/api/v1/requirements/missing/coverage", nil, 404)
	// a result without configuration counts as one more configuration
	plain := a.m("POST", base+"/runs", map[string]any{"name": "P", "test_ids": []string{t1["id"].(string)}}, 201)
	pe := execsOf(t, a, plain["id"].(string))
	a.m("PATCH", "/api/v1/executions/"+pe[0]["id"].(string), map[string]any{"status": "passed"}, 200)
	cov = a.m("GET", "/api/v1/requirements/"+rid+"/coverage", nil, 200)
	if cfgs := cov["configurations"].([]any); len(cfgs) != 3 || cfgs[2].(map[string]any)["id"] != "" || cfgs[2].(map[string]any)["coverage"] != "passed" {
		t.Fatal(cov)
	}
}

func TestDataDrivenTests(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("DD")
	base := "/api/v1/projects/" + pid
	shared := a.m("POST", base+"/tests", map[string]any{"title": "Log in", "steps": []map[string]any{{"action": "Log in as {{username}}", "expected": "Welcome {{ username }}, {{unknown}}"}}}, 201)
	tc := a.m("POST", base+"/tests", map[string]any{"title": "Order",
		"steps":      []map[string]any{{"call_test_id": shared["id"]}, {"action": "Order {{qty}} items", "expected": "Total {{total}}"}},
		"parameters": []string{"username", " qty ", "total"},
		"data_rows": []map[string]any{
			{"name": "small", "values": map[string]string{"username": "alice", "qty": "1", "total": "10", "extra": "dropped"}},
			{"values": map[string]string{"username": "bob", "qty": "3"}},
		}}, 201)
	tid := tc["id"].(string)
	if p := tc["parameters"].([]any); len(p) != 3 || p[1] != "qty" {
		t.Fatal(tc)
	}
	rows := tc["data_rows"].([]any)
	if v := rows[0].(map[string]any)["values"].(map[string]any); len(v) != 3 {
		t.Fatal(rows)
	}
	if v := rows[1].(map[string]any)["values"].(map[string]any); v["total"] != "" {
		t.Fatal(rows)
	}
	for _, bad := range []map[string]any{
		{"parameters": []string{"1abc"}},
		{"parameters": []string{"a b"}},
		{"parameters": []string{"a", "A"}},
		{"parameters": []string{}, "data_rows": []map[string]any{{"values": map[string]string{}}}},
		{"parameters": []string{"a"}, "data_rows": []map[string]any{{"name": strings.Repeat("n", 201)}}},
		{"parameters": []string{"a"}, "data_rows": []map[string]any{{"values": map[string]string{"a": strings.Repeat("v", 2001)}}}},
	} {
		a.m("PUT", "/api/v1/tests/"+tid+"/data", bad, 400)
	}
	many := make([]string, 51)
	for i := range many {
		many[i] = fmt.Sprint("p", i)
	}
	a.m("PUT", "/api/v1/tests/"+tid+"/data", map[string]any{"parameters": many}, 400)
	rowsMany := make([]map[string]any, 501)
	a.m("PUT", "/api/v1/tests/"+tid+"/data", map[string]any{"parameters": []string{"a"}, "data_rows": rowsMany}, 400)
	a.m("PUT", "/api/v1/tests/missing/data", map[string]any{}, 404)
	d := a.m("GET", "/api/v1/tests/"+tid+"/data", nil, 200)
	if len(d["data_rows"].([]any)) != 2 {
		t.Fatal(d)
	}
	// PATCH replaces data rows as a whole
	up := a.m("PATCH", "/api/v1/tests/"+tid, map[string]any{"data_rows": []map[string]any{
		{"name": "small", "values": map[string]string{"username": "alice", "qty": "1", "total": "10"}},
		{"name": "big", "values": map[string]string{"username": "bob", "qty": "3"}},
	}}, 200)
	if v := up["data_rows"].([]any)[1].(map[string]any)["values"].(map[string]any); v["total"] != "" || up["data_rows"].([]any)[1].(map[string]any)["name"] != "big" {
		t.Fatal(up)
	}
	e.member("viewer@example.com", "viewer")
	e.login("viewer@example.com").m("PUT", "/api/v1/tests/"+tid+"/data", map[string]any{"parameters": []string{"a"}}, 403)
	// history records the change
	if h := items(a.m("GET", "/api/v1/tests/"+tid+"/history", nil, 200)); len(h) < 2 {
		t.Fatal(h)
	}

	cfg := a.m("POST", base+"/configurations", map[string]any{"name": "Chrome"}, 201)["id"].(string)
	run := a.m("POST", base+"/runs", map[string]any{"name": "DD", "test_ids": []string{tid}, "configuration_ids": []string{cfg}}, 201)
	ex := execsOf(t, a, run["id"].(string))
	if len(ex) != 1 || ex[0]["iteration_count"].(float64) != 2 {
		t.Fatal(ex)
	}
	eid := ex[0]["id"].(string)
	x := a.m("GET", "/api/v1/executions/"+eid, nil, 200)
	its := x["iterations"].([]any)
	srs := x["step_results"].([]any)
	if len(its) != 2 || len(srs) != 4 || its[0].(map[string]any)["name"] != "small" {
		t.Fatal(x)
	}
	first := srs[0].(map[string]any)
	if first["action"] != "[DD-TC-1] Log in as alice" || first["expected"] != "Welcome alice, {{unknown}}" || first["iteration_id"] != its[0].(map[string]any)["id"] {
		t.Fatal(first)
	}
	if s := srs[3].(map[string]any); s["action"] != "Order 3 items" || s["expected"] != "Total " || s["iteration_id"] != its[1].(map[string]any)["id"] {
		t.Fatal(s)
	}
	it0, it1 := its[0].(map[string]any)["id"].(string), its[1].(map[string]any)["id"].(string)
	// first iteration passes: the execution is in progress
	x = a.m("PATCH", "/api/v1/executions/"+eid, map[string]any{"step_results": []map[string]any{
		{"id": srs[0].(map[string]any)["id"], "status": "passed"}, {"id": srs[1].(map[string]any)["id"], "status": "passed"}}}, 200)
	if x["status"] != "in_progress" || x["iterations"].([]any)[0].(map[string]any)["status"] != "passed" {
		t.Fatal(x)
	}
	// one step of the second iteration fails: failed
	x = a.m("PATCH", "/api/v1/executions/"+eid, map[string]any{"step_results": []map[string]any{
		{"id": srs[2].(map[string]any)["id"], "status": "passed"}, {"id": srs[3].(map[string]any)["id"], "status": "failed"}},
		"iterations": []map[string]any{{"id": it1, "notes": "qty 3 broke it"}}}, 200)
	if x["status"] != "failed" || x["iterations"].([]any)[1].(map[string]any)["notes"] != "qty 3 broke it" || x["iterations"].([]any)[1].(map[string]any)["status"] != "failed" {
		t.Fatal(x)
	}
	a.m("PATCH", "/api/v1/executions/"+eid, map[string]any{"iterations": []map[string]any{{"id": "nope", "status": "passed"}}}, 400)
	a.m("PATCH", "/api/v1/executions/"+eid, map[string]any{"iterations": []map[string]any{{"id": it0, "status": "bogus"}}}, 400)
	long := strings.Repeat("n", store.MaxEvidence+1)
	a.m("PATCH", "/api/v1/executions/"+eid, map[string]any{"iterations": []map[string]any{{"id": it0, "notes": long}}}, 400)

	// a data-driven test without steps: results per iteration
	nosteps := a.m("POST", base+"/tests", map[string]any{"title": "Search", "parameters": []string{"term"}, "data_rows": []map[string]any{
		{"values": map[string]string{"term": "a"}}, {"values": map[string]string{"term": "b"}}, {"values": map[string]string{"term": "c"}}}}, 201)
	a.m("POST", "/api/v1/runs/"+run["id"].(string)+"/executions", map[string]any{"test_ids": []string{nosteps["id"].(string)}}, 200)
	var nid string
	for _, x := range execsOf(t, a, run["id"].(string)) {
		if x["test_case_id"] == nosteps["id"] {
			nid = x["id"].(string)
		}
	}
	nx := a.m("GET", "/api/v1/executions/"+nid, nil, 200)
	nits := nx["iterations"].([]any)
	set := func(states ...string) string {
		var u []map[string]any
		for i, s := range states {
			u = append(u, map[string]any{"id": nits[i].(map[string]any)["id"], "status": s})
		}
		return a.m("PATCH", "/api/v1/executions/"+nid, map[string]any{"iterations": u}, 200)["status"].(string)
	}
	for _, c := range []struct {
		states []string
		want   string
	}{
		{[]string{"passed", "not_run", "not_run"}, "in_progress"},
		{[]string{"passed", "skipped", "passed"}, "passed"},
		{[]string{"passed", "blocked", "passed"}, "blocked"},
		{[]string{"failed", "blocked", "passed"}, "failed"},
		{[]string{"skipped", "skipped", "skipped"}, "skipped"},
		{[]string{"not_run", "not_run", "not_run"}, "not_run"},
	} {
		if got := set(c.states...); got != c.want {
			t.Fatalf("%v: %s, want %s", c.states, got, c.want)
		}
	}
	// a direct result is the overall result; iterations stay as they are
	if x := a.m("PATCH", "/api/v1/executions/"+nid, map[string]any{"status": "passed"}, 200); x["status"] != "passed" || x["iterations"].([]any)[0].(map[string]any)["status"] != "not_run" {
		t.Fatal(x)
	}
	// readiness: an iteration result counts as executing the test
	if !starts(store.ExecutionUpdate{Iterations: []store.IterationUpdate{{ID: "i", Status: "passed"}}}, &model.Execution{Iterations: []model.Iteration{{ID: "i", Status: "not_run"}}}) ||
		starts(store.ExecutionUpdate{Iterations: []store.IterationUpdate{{ID: "i", Status: "passed"}}}, &model.Execution{Iterations: []model.Iteration{{ID: "i", Status: "passed"}}}) {
		t.Fatal("starts")
	}

	// export carries the data table
	body := readAll(t, a.raw("GET", base+"/export?format=csv&aspects=test_data,tests", nil))
	if !strings.Contains(body, "Test data") && !strings.Contains(body, "test_data") {
		t.Fatal(body)
	}
	body = readAll(t, a.raw("GET", base+"/export?format=csv&aspects=test_data", nil))
	if !strings.Contains(body, "Parameter,Value") || !strings.Contains(body, "DD-TC-2,2,big,qty,3") {
		t.Fatal(body)
	}

	// copy keeps the data; trash and restore bring back iterations
	cp := a.m("POST", "/api/v1/tests/"+tid+"/copy", nil, 201)
	if len(cp["data_rows"].([]any)) != 2 {
		t.Fatal(cp)
	}
	res := a.raw("DELETE", "/api/v1/tests/"+tid, nil)
	trash := res.Header.Get("X-Trash-Id")
	res.Body.Close()
	if trash == "" {
		t.Fatal("no trash id")
	}
	a.m("POST", "/api/v1/trash/"+trash+"/restore", nil, 200)
	x = a.m("GET", "/api/v1/executions/"+eid, nil, 200)
	if len(x["iterations"].([]any)) != 2 || len(x["step_results"].([]any)) != 4 || x["configuration_name"] != "Chrome" {
		t.Fatal(x)
	}
}
