package api

import (
	"testing"
)

func TestBulkRuns(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BRN")
	tc := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "T"}, 201)["id"].(string)
	var runs []string
	for _, n := range []string{"R1", "R2", "R3"} {
		runs = append(runs, a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": n, "test_ids": []string{tc}}, 201)["id"].(string))
	}
	rel := a.m("POST", "/api/v1/projects/"+pid+"/releases", map[string]any{"name": "1.0"}, 201)["id"].(string)
	path := "/api/v1/projects/" + pid + "/runs/bulk"

	a.m("POST", path, map[string]any{"ids": runs}, 400)
	a.m("POST", path, map[string]any{"ids": []string{}, "set": map[string]any{"build": "x"}}, 400)
	a.m("POST", path, map[string]any{"ids": runs, "set": map[string]any{"status": "nope"}}, 400)
	a.m("POST", path, map[string]any{"ids": []string{"missing"}, "set": map[string]any{"build": "x"}}, 404)
	other := a.project("OTH")
	orel := a.m("POST", "/api/v1/projects/"+other+"/releases", map[string]any{"name": "x"}, 201)["id"].(string)
	a.m("POST", path, map[string]any{"ids": runs, "set": map[string]any{"release_id": orel}}, 400)

	res := a.m("POST", path, map[string]any{"ids": runs[:2], "set": map[string]any{"status": "aborted", "release_id": rel, "environment": "staging", "build": "1.0.3"}}, 200)
	if res["updated"].(float64) != 2 {
		t.Fatal(res)
	}
	got := a.m("GET", "/api/v1/runs/"+runs[0], nil, 200)
	if got["status"] != "aborted" || got["release_id"] != rel || got["environment"] != "staging" || got["build"] != "1.0.3" {
		t.Fatal(got)
	}
	if got := a.m("GET", "/api/v1/runs/"+runs[2], nil, 200); got["status"] != "planned" {
		t.Fatal(got)
	}
	// clear the release
	a.m("POST", path, map[string]any{"ids": runs[:1], "set": map[string]any{"release_id": ""}}, 200)
	if got := a.m("GET", "/api/v1/runs/"+runs[0], nil, 200); got["release_id"] != nil {
		t.Fatal(got)
	}
	// testers may not change runs
	e.member("tester@example.com", "viewer")
	e.login("tester@example.com").m("POST", path, map[string]any{"ids": runs, "set": map[string]any{"build": "x"}}, 403)

	if res := a.m("DELETE", path, map[string]any{"ids": runs[1:]}, 200); res["deleted"].(float64) != 2 {
		t.Fatal(res)
	}
	a.m("GET", "/api/v1/runs/"+runs[2], nil, 404)
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/runs", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
}

func TestBulkExecutions(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	e.member("anna.berg@example.com", "tester")
	pid := a.project("BEX")
	mk := func(title string) string {
		return a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": title}, 201)["id"].(string)
	}
	login, order, pay, extra := mk("Login"), mk("Order"), mk("Pay"), mk("Extra")
	// order needs login, pay needs order
	a.m("POST", "/api/v1/tests/"+order+"/dependencies", map[string]any{"depends_on_id": login}, 201)
	a.m("POST", "/api/v1/tests/"+pay+"/dependencies", map[string]any{"depends_on_id": order}, 201)
	a.m("PUT", "/api/v1/settings", map[string]any{"allow_unready_tests": true, "allow_unready_requirements": true, "allow_unmet_dependencies": false}, 200)
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R", "test_ids": []string{pay, order, login, extra}}, 201)["id"].(string)
	ex := map[string]string{}
	for _, x := range items(a.m("GET", "/api/v1/runs/"+run+"/executions", nil, 200)) {
		ex[x["test_case_id"].(string)] = x["id"].(string)
	}
	path := "/api/v1/runs/" + run + "/executions/bulk"
	a.m("POST", path, map[string]any{"all": true}, 400)
	a.m("POST", path, map[string]any{"set": map[string]any{"status": "passed"}}, 400)
	a.m("POST", path, map[string]any{"all": true, "set": map[string]any{"status": "in_progress"}}, 400)
	a.m("POST", path, map[string]any{"all": true, "from": []string{"nope"}, "set": map[string]any{"status": "passed"}}, 400)
	a.m("POST", path, map[string]any{"ids": []string{"missing"}, "set": map[string]any{"status": "passed"}}, 404)

	// passing all three in one go works despite the dependency order in the list
	res := a.m("POST", path, map[string]any{"ids": []string{ex[pay], ex[order], ex[login]}, "set": map[string]any{"status": "passed", "notes_append": "Smoke OK"}}, 200)
	if len(res["updated"].([]any)) != 3 || len(res["rejected"].([]any)) != 0 {
		t.Fatal(res)
	}
	got := a.m("GET", "/api/v1/executions/"+ex[pay], nil, 200)
	if got["status"] != "passed" || got["notes"] != "Smoke OK" || got["executed_by"] != "admin@example.com" {
		t.Fatal(got)
	}
	// login fails: order and pay can no longer be passed again from not_run
	a.m("POST", path, map[string]any{"ids": []string{ex[login]}, "set": map[string]any{"status": "failed"}}, 200)
	a.m("POST", path, map[string]any{"ids": []string{ex[order], ex[pay]}, "set": map[string]any{"status": "not_run"}}, 200)
	res = a.m("POST", path, map[string]any{"all": true, "from": []string{"not_run"}, "set": map[string]any{"status": "passed"}}, 200)
	if got := bulkKeys(res["updated"]); len(got) != 1 || got[0] != "BEX-TC-4" {
		t.Fatal(res)
	}
	if rej := res["rejected"].([]any); len(rej) != 2 || rej[0].(map[string]any)["reason"] == "" {
		t.Fatal(res)
	}
	if sk := bulkKeys(res["skipped"]); len(sk) != 1 || sk[0] != "BEX-TC-1:status" {
		t.Fatal(res)
	}
	// blocked and skipped need no readiness; the run completes and assignment notifies
	res = a.m("POST", path, map[string]any{"all": true, "from": []string{"not_run"}, "set": map[string]any{"status": "skipped", "assignee": "@anna.berg"}}, 200)
	if len(res["updated"].([]any)) != 2 {
		t.Fatal(res)
	}
	if r := a.m("GET", "/api/v1/runs/"+run, nil, 200); r["status"] != "completed" {
		t.Fatal(r)
	}
	if n := e.login("anna.berg@example.com").m("GET", "/api/v1/notifications", nil, 200); n["unread"].(float64) != 2 {
		t.Fatal(n)
	}
	// unchanged items are skipped as "already"
	res = a.m("POST", path, map[string]any{"ids": []string{ex[extra]}, "set": map[string]any{"status": "passed"}}, 200)
	if sk := bulkKeys(res["skipped"]); len(sk) != 1 || sk[0] != "BEX-TC-4:already" {
		t.Fatal(res)
	}

	// removing needs run.write; a role with run.execute only can record results
	a.m("POST", "/api/v1/roles", map[string]any{"name": "executor", "permissions": []string{"project.read", "test.read", "run.read", "run.execute"}}, 201)
	e.member("exe@example.com", "executor")
	exe := e.login("exe@example.com")
	exe.m("POST", path, map[string]any{"ids": []string{ex[extra]}, "set": map[string]any{"status": "failed"}}, 200)
	exe.m("POST", path, map[string]any{"ids": []string{ex[extra]}, "remove": true}, 403)
	a.m("POST", path, map[string]any{"ids": []string{ex[extra], ex[pay]}, "remove": true}, 200)
	if l := items(a.m("GET", "/api/v1/runs/"+run+"/executions", nil, 200)); len(l) != 2 {
		t.Fatal(l)
	}
	a.m("GET", "/api/v1/executions/"+ex[extra], nil, 404)
	e.member("viewer@example.com", "viewer")
	e.login("viewer@example.com").m("POST", path, map[string]any{"all": true, "set": map[string]any{"status": "passed"}}, 403)
}
