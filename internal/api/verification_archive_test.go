package api

import "testing"

func TestVerificationTrashAndArchive(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("ARC")
	pp := "/api/v1/projects/" + pid
	rq := a.m("POST", pp+"/requirements", map[string]any{"title": "Frame", "create_test": false, "criteria": []map[string]any{{"description": "Weld check", "method": "inspection"}}}, 201)
	rid := rq["id"].(string)
	cid := rq["criteria"].([]any)[0].(map[string]any)["id"].(string)
	tid := a.m("POST", pp+"/tests", map[string]any{"title": "Load test"}, 201)["id"].(string)
	a.m("POST", pp+"/links", map[string]any{"requirement_id": rid, "test_case_id": tid, "criterion_id": cid}, 201)
	a.m("POST", "/api/v1/requirements/"+rid+"/verification-records", map[string]any{"criterion_id": cid, "result": "pass"}, 201)
	parent := a.m("POST", pp+"/elements", map[string]any{"name": "Chassis"}, 201)["id"].(string)
	el := a.m("POST", pp+"/elements", map[string]any{"name": "Frame", "parent_id": parent}, 201)["id"].(string)
	a.m("PUT", "/api/v1/requirements/"+rid+"/elements", map[string]any{"element_ids": []string{el}}, 200)
	a.m("PUT", "/api/v1/tests/"+tid+"/elements", map[string]any{"element_ids": []string{el}}, 200)
	a.m("PUT", pp+"/verification-settings", map[string]any{"strict_criteria": true}, 200)
	check := func(p string) {
		t.Helper()
		reqs := items(a.m("GET", "/api/v1/projects/"+p+"/requirements", nil, 200))
		if len(reqs) != 1 || len(reqs[0]["element_ids"].([]any)) != 1 {
			t.Fatal(reqs)
		}
		nrid := reqs[0]["id"].(string)
		if recs := items(a.m("GET", "/api/v1/requirements/"+nrid+"/verification-records", nil, 200)); len(recs) != 1 || recs[0]["criterion_id"] == nil {
			t.Fatal(recs)
		}
		links := items(a.m("GET", "/api/v1/projects/"+p+"/links", nil, 200))
		if len(links) != 1 || len(links[0]["criteria"].([]any)) != 1 {
			t.Fatal(links)
		}
		if tests := items(a.m("GET", "/api/v1/projects/"+p+"/tests", nil, 200)); len(tests[0]["element_ids"].([]any)) != 1 {
			t.Fatal(tests)
		}
		if els := items(a.m("GET", "/api/v1/projects/"+p+"/elements", nil, 200)); len(els) != 2 || els[1]["path"] != "Chassis / Frame" {
			t.Fatal(els)
		}
		if st := a.m("GET", "/api/v1/projects/"+p+"/verification-settings", nil, 200); st["strict_criteria"] != true {
			t.Fatal(st)
		}
	}
	// deleting and restoring the requirement and the test keeps it all
	res := a.raw("DELETE", "/api/v1/requirements/"+rid, nil)
	a.m("POST", "/api/v1/trash/"+res.Header.Get("X-Trash-Id")+"/restore", nil, 200)
	res = a.raw("DELETE", "/api/v1/tests/"+tid, nil)
	a.m("POST", "/api/v1/trash/"+res.Header.Get("X-Trash-Id")+"/restore", nil, 200)
	check(pid)
	// a project archive carries it to a new project
	_, archive := a.getBytes(pp+"/archive", 200)
	out := a.upload("POST", "/api/v1/projects/import", "archive", "arc.zip", "application/zip", archive, map[string]string{"key": "ARC2"}, 201)
	check(out["project"].(map[string]any)["id"].(string))
}
