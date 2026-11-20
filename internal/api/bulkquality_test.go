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

func TestBulkReleases(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BRL")
	q1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "description": "The user MUST log in."}, 201)["id"].(string)
	q2 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Logout", "description": "The user MAY log out."}, 201)["id"].(string)
	// r1 has no criteria (go), r2 the defaults with an untested requirement (no-go)
	r1 := a.m("POST", "/api/v1/projects/"+pid+"/releases", map[string]any{"name": "1.0", "exit_criteria": []any{}}, 201)["id"].(string)
	r2 := a.m("POST", "/api/v1/projects/"+pid+"/releases", map[string]any{"name": "1.1"}, 201)["id"].(string)
	r3 := a.m("POST", "/api/v1/projects/"+pid+"/releases", map[string]any{"name": "2.0"}, 201)["id"].(string)
	a.m("POST", "/api/v1/releases/"+r2+"/requirements", map[string]any{"add": []string{q1}}, 200)
	path := "/api/v1/projects/" + pid + "/releases/bulk"

	a.m("POST", path, map[string]any{"ids": []string{r1}}, 400)
	a.m("POST", path, map[string]any{"ids": []string{r1}, "set": map[string]any{"target_date": "soon"}}, 400)
	a.m("POST", path, map[string]any{"ids": []string{r1}, "set": map[string]any{"status": "released"}, "add_requirement_ids": []string{q1}}, 400)
	a.m("POST", path, map[string]any{"ids": []string{"missing"}, "set": map[string]any{"status": "active"}}, 404)

	// scope by query and ids, target date and criteria copied to two releases
	crit := []map[string]any{{"type": "coverage", "threshold": 80}, {"type": "no_failed"}}
	res := a.m("POST", path, map[string]any{"ids": []string{r2, r3}, "set": map[string]any{"target_date": "2027-03-01", "exit_criteria": crit, "status": "active"}, "query": "title:Logout"}, 200)
	if len(res["updated"].([]any)) != 2 {
		t.Fatal(res)
	}
	got := a.m("GET", "/api/v1/releases/"+r3, nil, 200)
	if got["target_date"] != "2027-03-01" || got["status"] != "active" || len(got["exit_criteria"].([]any)) != 2 || len(got["requirement_ids"].([]any)) != 1 || got["requirement_ids"].([]any)[0] != q2 {
		t.Fatal(got)
	}
	a.m("POST", path, map[string]any{"ids": []string{r3}, "remove_requirement_ids": []string{q2}}, 200)
	if got := a.m("GET", "/api/v1/releases/"+r3, nil, 200); got["requirement_count"].(float64) != 0 {
		t.Fatal(got)
	}

	// releasing: go releases are released (with a baseline), no-go ones skipped
	res = a.m("POST", path, map[string]any{"ids": []string{r1, r2}, "set": map[string]any{"status": "released"}}, 200)
	if up := res["updated"].([]any); len(up) != 1 || up[0] != "1.0" {
		t.Fatal(res)
	}
	if sk := res["skipped"].([]any); len(sk) != 1 || sk[0].(map[string]any)["reason"] != "no_go" {
		t.Fatal(res)
	}
	if got := a.m("GET", "/api/v1/releases/"+r1, nil, 200); got["status"] != "released" || got["released_at"] == nil {
		t.Fatal(got)
	}
	if got := a.m("GET", "/api/v1/releases/"+r2, nil, 200); got["status"] != "active" {
		t.Fatal(got)
	}
	found := false
	for _, b := range items(a.m("GET", "/api/v1/projects/"+pid+"/baselines", nil, 200)) {
		found = found || b["name"] == "Release 1.0"
	}
	if !found {
		t.Fatal("no baseline")
	}
	// an empty scope is nothing to measure: no-go, not a vacuous pass
	if got := a.m("GET", "/api/v1/releases/"+r3+"/status", nil, 200); got["go"] != false || criterion(t, got, "coverage")["available"] != false {
		t.Fatal(got)
	}
	res = a.m("POST", path, map[string]any{"ids": []string{r3}, "set": map[string]any{"status": "released"}}, 200)
	if len(res["updated"].([]any)) != 0 || len(res["skipped"].([]any)) != 1 {
		t.Fatal(res)
	}
	e.member("tester@example.com", "tester")
	e.login("tester@example.com").m("POST", path, map[string]any{"ids": []string{r3}, "set": map[string]any{"status": "cancelled"}}, 403)
	if res := a.m("DELETE", path, map[string]any{"ids": []string{r2, r3}}, 200); res["deleted"].(float64) != 2 {
		t.Fatal(res)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/releases", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
}

func TestBulkBaselines(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	e.member("rita@example.com", "reviewer")
	rc := e.login("rita@example.com")
	pid := a.project("BBL")
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "R"}, 201)
	var ids []string
	for _, n := range []string{"B1", "B2", "B3"} {
		ids = append(ids, a.m("POST", "/api/v1/projects/"+pid+"/baselines", map[string]any{"name": n}, 201)["id"].(string))
	}
	sign := "/api/v1/projects/" + pid + "/baselines/signoffs"
	rc.m("POST", sign, map[string]any{"ids": ids[:2], "meaning": "approved", "password": "wrong"}, 403)
	rc.m("POST", sign, map[string]any{"ids": ids[:2], "meaning": "liked", "password": testPassword}, 400)
	rc.m("POST", sign, map[string]any{"ids": []string{}, "meaning": "approved", "password": testPassword}, 400)
	e.member("tester@example.com", "tester")
	e.login("tester@example.com").m("POST", sign, map[string]any{"ids": ids[:2], "meaning": "approved", "password": testPassword}, 403)

	res := rc.m("POST", sign, map[string]any{"ids": ids[:2], "meaning": "approved", "comment": "Sprint review", "password": testPassword}, 200)
	signed := res["signed"].([]any)
	if len(signed) != 2 || len(signed[0].(map[string]any)["content_hash"].(string)) != 64 {
		t.Fatal(res)
	}
	// once more: the first two are skipped, the third is signed
	res = rc.m("POST", sign, map[string]any{"ids": ids, "meaning": "approved", "password": testPassword}, 200)
	if len(res["signed"].([]any)) != 1 || len(res["skipped"].([]any)) != 2 || res["skipped"].([]any)[0].(map[string]any)["reason"] != "already" {
		t.Fatal(res)
	}
	if l := items(a.m("GET", "/api/v1/baselines/"+ids[0]+"/signoffs", nil, 200)); len(l) != 1 || l[0]["comment"] != "Sprint review" || l[0]["valid"] != true {
		t.Fatal(l)
	}

	// deleting: signed baselines are kept
	b4 := a.m("POST", "/api/v1/projects/"+pid+"/baselines", map[string]any{"name": "B4"}, 201)["id"].(string)
	del := "/api/v1/projects/" + pid + "/baselines/bulk"
	rc.m("DELETE", del, map[string]any{"ids": []string{b4}}, 403)
	res = a.m("DELETE", del, map[string]any{"ids": append([]string{b4}, ids...)}, 200)
	if res["deleted"].(float64) != 1 || len(res["skipped"].([]any)) != 3 || res["skipped"].([]any)[0].(map[string]any)["reason"] != "signed" {
		t.Fatal(res)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/baselines", nil, 200)); len(l) != 3 {
		t.Fatal(l)
	}
	a.m("DELETE", del, map[string]any{"ids": []string{"missing"}}, 404)
}
