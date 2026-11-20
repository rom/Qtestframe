package api

import (
	"net/http/httptest"
	"testing"
)

func TestBulkDefects(t *testing.T) {
	e := newEnv(t)
	gh := &fakeGitHub{issues: map[string]map[string]any{}}
	ts := httptest.NewServer(gh)
	defer ts.Close()
	e.srv.HTTPClient = ts.Client()
	a := e.login("admin@example.com")
	e.member("anna.berg@example.com", "tester")
	pid := a.project("BD")
	var ids []string
	for _, title := range []string{"One", "Two", "Three"} {
		ids = append(ids, a.m("POST", "/api/v1/projects/"+pid+"/defects", map[string]any{"title": title, "severity": "minor"}, 201)["id"].(string))
	}
	path := "/api/v1/projects/" + pid + "/defects/bulk"
	a.m("POST", path, map[string]any{"ids": ids}, 400)
	a.m("POST", path, map[string]any{"ids": []string{}, "set": map[string]any{"status": "closed"}}, 400)
	a.m("POST", path, map[string]any{"ids": []string{"missing"}, "set": map[string]any{"status": "closed"}}, 404)
	// one bad value changes nothing
	a.m("POST", path, map[string]any{"ids": ids, "set": map[string]any{"status": "closed", "severity": "huge"}}, 400)
	if d := a.m("GET", "/api/v1/defects/"+ids[0], nil, 200); d["status"] != "open" {
		t.Fatal(d)
	}
	res := a.m("POST", path, map[string]any{"ids": ids[:2], "set": map[string]any{"status": "in_progress", "severity": "major", "assignee": "@anna.berg"}}, 200)
	if res["updated"].(float64) != 2 {
		t.Fatal(res)
	}
	d := a.m("GET", "/api/v1/defects/"+ids[1], nil, 200)
	if d["status"] != "in_progress" || d["severity"] != "major" || d["assignee"] != "@anna.berg" {
		t.Fatal(d)
	}
	if n := e.login("anna.berg@example.com").m("GET", "/api/v1/notifications", nil, 200); n["unread"].(float64) < 2 {
		t.Fatal(n)
	}

	// push the selection to GitHub; already linked ones are skipped; sync just those
	a.m("PUT", "/api/v1/projects/"+pid+"/defect-tracker", map[string]any{"type": "github", "url": ts.URL, "repo": "acme/shop", "token": "t"}, 200)
	push := a.m("POST", "/api/v1/projects/"+pid+"/defects/push", map[string]any{"ids": ids[:2]}, 200)
	if len(push["pushed"].([]any)) != 2 || len(push["failed"].([]any)) != 0 {
		t.Fatal(push)
	}
	push = a.m("POST", "/api/v1/projects/"+pid+"/defects/push", map[string]any{"ids": ids}, 200)
	if len(push["pushed"].([]any)) != 1 || len(push["skipped"].([]any)) != 2 {
		t.Fatal(push)
	}
	gh.mu.Lock()
	gh.issues["1"]["state"] = "closed"
	gh.mu.Unlock()
	sync := a.m("POST", "/api/v1/projects/"+pid+"/defects/sync", map[string]any{"ids": ids[:1]}, 200)
	if sync["synced"].(float64) != 1 || sync["changed"].(float64) != 1 {
		t.Fatal(sync)
	}
	if all := a.m("POST", "/api/v1/projects/"+pid+"/defects/sync", nil, 200); all["synced"].(float64) != 3 {
		t.Fatal(all)
	}

	// deleting needs project.update; testers cannot
	e.login("anna.berg@example.com").m("DELETE", path, map[string]any{"ids": ids}, 403)
	if res := a.m("DELETE", path, map[string]any{"ids": ids[1:]}, 200); res["deleted"].(float64) != 2 {
		t.Fatal(res)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/defects", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	other := a.project("OTB")
	od := a.m("POST", "/api/v1/projects/"+other+"/defects", map[string]any{"title": "x"}, 201)["id"].(string)
	a.m("POST", path, map[string]any{"ids": []string{od}, "set": map[string]any{"status": "closed"}}, 404)
}

func TestBulkRisks(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BK")
	rq := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "R"}, 201)["id"].(string)
	var ids []string
	for _, title := range []string{"A", "B", "C"} {
		ids = append(ids, a.m("POST", "/api/v1/projects/"+pid+"/risks", map[string]any{"title": title}, 201)["id"].(string))
	}
	path := "/api/v1/projects/" + pid + "/risks/bulk"
	a.m("POST", path, map[string]any{"ids": ids}, 400)
	a.m("POST", path, map[string]any{"ids": ids, "set": map[string]any{"impact": 9}}, 400)
	a.m("POST", path, map[string]any{"ids": ids, "set": map[string]any{"impact": 0}}, 400)
	a.m("POST", path, map[string]any{"ids": ids, "set": map[string]any{"impact": "big"}}, 400)
	a.m("POST", path, map[string]any{"ids": ids, "set": map[string]any{"status": "gone"}}, 400)
	res := a.m("POST", path, map[string]any{"ids": ids[:2], "set": map[string]any{"status": "mitigating", "owner": "Security", "impact": 5}, "add_requirement_ids": []string{rq}}, 200)
	if res["updated"].(float64) != 2 {
		t.Fatal(res)
	}
	k := a.m("GET", "/api/v1/risks/"+ids[0], nil, 200)
	if k["status"] != "mitigating" || k["owner"] != "Security" || k["impact"].(float64) != 5 || k["level"] != "high" || len(k["requirement_ids"].([]any)) != 1 {
		t.Fatal(k)
	}
	if k := a.m("GET", "/api/v1/risks/"+ids[2], nil, 200); k["status"] != "identified" {
		t.Fatal(k)
	}
	a.m("POST", path, map[string]any{"ids": ids[:1], "remove_requirement_ids": []string{rq}}, 200)
	if k := a.m("GET", "/api/v1/risks/"+ids[0], nil, 200); len(k["requirement_ids"].([]any)) != 0 || k["status"] != "mitigating" {
		t.Fatal(k)
	}
	e.member("viewer@example.com", "viewer")
	e.login("viewer@example.com").m("POST", path, map[string]any{"ids": ids, "set": map[string]any{"status": "closed"}}, 403)
	if res := a.m("DELETE", path, map[string]any{"ids": ids[:2]}, 200); res["deleted"].(float64) != 2 {
		t.Fatal(res)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/risks", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
}
