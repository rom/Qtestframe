package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// fixture: project with two requirements (one MUST), two linked tests and a run.
type qfix struct {
	pid, r1, r2, t1, t2, run string
	ex                       map[string]string // test id → execution id
}

func qualityFixture(t *testing.T, a *client, key string) qfix {
	t.Helper()
	f := qfix{pid: a.project(key), ex: map[string]string{}}
	f.r1 = a.m("POST", "/api/v1/projects/"+f.pid+"/requirements", map[string]any{"title": "Login", "description": "The user MUST log in.", "status": "approved"}, 201)["id"].(string)
	f.r2 = a.m("POST", "/api/v1/projects/"+f.pid+"/requirements", map[string]any{"title": "Theme", "description": "The UI MAY offer a dark theme.", "status": "approved"}, 201)["id"].(string)
	f.t1 = a.m("POST", "/api/v1/projects/"+f.pid+"/tests", map[string]any{"title": "Valid login", "status": "ready"}, 201)["id"].(string)
	f.t2 = a.m("POST", "/api/v1/projects/"+f.pid+"/tests", map[string]any{"title": "Dark theme", "status": "ready"}, 201)["id"].(string)
	a.m("POST", "/api/v1/projects/"+f.pid+"/links", map[string]any{"requirement_id": f.r1, "test_case_id": f.t1}, 201)
	a.m("POST", "/api/v1/projects/"+f.pid+"/links", map[string]any{"requirement_id": f.r2, "test_case_id": f.t2}, 201)
	return f
}

func criterion(t *testing.T, st map[string]any, typ string) map[string]any {
	t.Helper()
	for _, c := range st["criteria"].([]any) {
		if m := c.(map[string]any); m["type"] == typ {
			return m
		}
	}
	t.Fatalf("no criterion %s in %v", typ, st)
	return nil
}

func TestReleasesExitCriteria(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	f := qualityFixture(t, a, "REL")
	rel := a.m("POST", "/api/v1/projects/"+f.pid+"/releases", map[string]any{"name": "1.0", "target_date": "2027-01-31"}, 201)
	rid := rel["id"].(string)
	if len(rel["exit_criteria"].([]any)) == 0 || rel["status"] != "planned" {
		t.Fatal(rel)
	}
	a.m("POST", "/api/v1/projects/"+f.pid+"/releases", map[string]any{"name": ""}, 400)
	a.m("POST", "/api/v1/projects/"+f.pid+"/releases", map[string]any{"name": "x", "target_date": "soon"}, 400)
	a.m("POST", "/api/v1/projects/"+f.pid+"/releases", map[string]any{"name": "x", "status": "released"}, 400)
	a.m("POST", "/api/v1/projects/"+f.pid+"/releases", map[string]any{"name": "x", "exit_criteria": []map[string]any{{"type": "nope"}}}, 400)

	// scope: one by id, one by query; then remove and re-add
	res := a.m("POST", "/api/v1/releases/"+rid+"/requirements", map[string]any{"add": []string{f.r1}, "query": "title:Theme"}, 200)
	if res["added"].(float64) != 2 {
		t.Fatal(res)
	}
	a.m("POST", "/api/v1/releases/"+rid+"/requirements", map[string]any{"remove": []string{f.r2}}, 200)
	a.m("POST", "/api/v1/releases/"+rid+"/requirements", map[string]any{"add": []string{f.r2}}, 200)
	a.m("POST", "/api/v1/releases/"+rid+"/requirements", map[string]any{"add": []string{"missing"}}, 404)

	st := a.m("GET", "/api/v1/releases/"+rid+"/status", nil, 200)
	if st["go"] != false || criterion(t, st, "coverage")["met"] != true || criterion(t, st, "passed")["met"] != false {
		t.Fatal(st)
	}
	// releasing without go is refused, with override needs a note
	a.m("PATCH", "/api/v1/releases/"+rid, map[string]any{"status": "released"}, 409)
	a.m("PATCH", "/api/v1/releases/"+rid, map[string]any{"status": "released", "override": true}, 400)

	// a run in the release with t1 passed and t2 failed
	run := a.m("POST", "/api/v1/projects/"+f.pid+"/runs", map[string]any{"name": "RC1", "release_id": rid, "test_ids": []string{f.t1, f.t2}}, 201)
	if run["release_id"] != rid {
		t.Fatal(run)
	}
	for _, x := range items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200)) {
		f.ex[x["test_case_id"].(string)] = x["id"].(string)
	}
	a.m("PATCH", "/api/v1/executions/"+f.ex[f.t1], map[string]any{"status": "passed"}, 200)
	a.m("PATCH", "/api/v1/executions/"+f.ex[f.t2], map[string]any{"status": "failed"}, 200)
	st = a.m("GET", "/api/v1/releases/"+rid+"/status", nil, 200)
	if criterion(t, st, "mandatory_passed")["met"] != true || criterion(t, st, "no_failed")["met"] != false || st["go"] != false {
		t.Fatal(st)
	}
	if got := a.m("GET", "/api/v1/releases/"+rid, nil, 200); got["run_count"].(float64) != 1 || len(got["requirement_ids"].([]any)) != 2 {
		t.Fatal(got)
	}
	// open critical defect and high risk criteria
	a.m("PATCH", "/api/v1/releases/"+rid, map[string]any{"exit_criteria": []map[string]any{
		{"type": "mandatory_passed"}, {"type": "no_open_defects", "level": "major"}, {"type": "risks_mitigated", "level": "high"}}}, 200)
	d := a.m("POST", "/api/v1/projects/"+f.pid+"/defects", map[string]any{"title": "Login crashes", "severity": "critical",
		"links": []map[string]any{{"entity_type": "test", "entity_id": f.t1}}}, 201)
	k := a.m("POST", "/api/v1/projects/"+f.pid+"/risks", map[string]any{"title": "Account takeover", "likelihood": 4, "impact": 5, "requirement_ids": []string{f.r1}}, 201)
	st = a.m("GET", "/api/v1/releases/"+rid+"/status", nil, 200)
	if criterion(t, st, "no_open_defects")["met"] != false || criterion(t, st, "risks_mitigated")["met"] != false {
		t.Fatal(st)
	}
	a.m("PATCH", "/api/v1/defects/"+d["id"].(string), map[string]any{"status": "resolved"}, 200)
	a.m("PATCH", "/api/v1/risks/"+k["id"].(string), map[string]any{"status": "mitigated"}, 200)
	st = a.m("GET", "/api/v1/releases/"+rid+"/status", nil, 200)
	if st["go"] != true {
		t.Fatal(st)
	}
	out := a.m("PATCH", "/api/v1/releases/"+rid, map[string]any{"status": "released"}, 200)
	if out["released_at"] == nil || out["released_by"] != "admin@example.com" {
		t.Fatal(out)
	}
	found := false
	for _, b := range items(a.m("GET", "/api/v1/projects/"+f.pid+"/baselines", nil, 200)) {
		found = found || b["name"] == "Release 1.0"
	}
	if !found {
		t.Fatal("no release baseline")
	}
	if list := items(a.m("GET", "/api/v1/projects/"+f.pid+"/releases", nil, 200)); len(list) != 1 {
		t.Fatal(list)
	}
	// viewers can read, not change
	e.member("viewer@example.com", "viewer")
	v := e.login("viewer@example.com")
	v.m("GET", "/api/v1/releases/"+rid+"/status", nil, 200)
	v.m("PATCH", "/api/v1/releases/"+rid, map[string]any{"name": "x"}, 403)
	// deleting the release detaches its runs
	a.m("DELETE", "/api/v1/releases/"+rid, nil, 204)
	if got := a.m("GET", "/api/v1/runs/"+run["id"].(string), nil, 200); got["release_id"] != nil {
		t.Fatal(got)
	}
	a.m("GET", "/api/v1/releases/"+rid, nil, 404)
}

// fakeGitHub is a minimal GitHub issues API.
type fakeGitHub struct {
	mu     sync.Mutex
	issues map[string]map[string]any
	auth   []string
}

func (g *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.auth = append(g.auth, r.Header.Get("Authorization"))
	switch {
	case r.Method == "GET" && r.URL.Path == "/repos/acme/shop":
		writeJSON(w, 200, map[string]any{"full_name": "acme/shop"})
	case r.Method == "POST" && r.URL.Path == "/repos/acme/shop/issues":
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		n := len(g.issues) + 1
		is := map[string]any{"number": n, "html_url": "https://github.example/acme/shop/issues/" + itoa(n), "title": in["title"], "state": "open"}
		g.issues[itoa(n)] = is
		writeJSON(w, 201, is)
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/repos/acme/shop/issues/"):
		is, ok := g.issues[strings.TrimPrefix(r.URL.Path, "/repos/acme/shop/issues/")]
		if !ok {
			writeJSON(w, 404, map[string]any{"message": "Not Found"})
			return
		}
		writeJSON(w, 200, is)
	default:
		writeJSON(w, 404, map[string]any{"message": "Not Found"})
	}
}

func TestDefectsAndTrackerSync(t *testing.T) {
	e := newEnv(t)
	gh := &fakeGitHub{issues: map[string]map[string]any{}}
	ts := httptest.NewServer(gh)
	defer ts.Close()
	e.srv.HTTPClient = ts.Client()
	a := e.login("admin@example.com")
	f := qualityFixture(t, a, "BUGS")
	run := a.m("POST", "/api/v1/projects/"+f.pid+"/runs", map[string]any{"name": "R", "test_ids": []string{f.t1}}, 201)
	exID := items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200))[0]["id"].(string)

	d := a.m("POST", "/api/v1/projects/"+f.pid+"/defects", map[string]any{"title": "Login fails", "severity": "major",
		"links": []map[string]any{{"entity_type": "execution", "entity_id": exID}}}, 201)
	did := d["id"].(string)
	if d["key"] != "BUGS-BUG-1" || d["status"] != "open" || len(d["links"].([]any)) != 1 {
		t.Fatal(d)
	}
	a.m("POST", "/api/v1/projects/"+f.pid+"/defects", map[string]any{"title": ""}, 400)
	a.m("POST", "/api/v1/projects/"+f.pid+"/defects", map[string]any{"title": "x", "severity": "huge"}, 400)
	a.m("PATCH", "/api/v1/defects/"+did, map[string]any{"status": "nope"}, 400)
	// links: add the requirement, then remove it; foreign ids are refused
	a.m("POST", "/api/v1/defects/"+did+"/links", map[string]any{"entity_type": "requirement", "entity_id": f.r2}, 200)
	a.m("DELETE", "/api/v1/defects/"+did+"/links", map[string]any{"entity_type": "requirement", "entity_id": f.r2}, 200)
	a.m("POST", "/api/v1/defects/"+did+"/links", map[string]any{"entity_type": "requirement", "entity_id": "missing"}, 404)
	// the requirement sees the defect via its test's execution
	if list := items(a.m("GET", "/api/v1/requirements/"+f.r1+"/defects", nil, 200)); len(list) != 1 {
		t.Fatal(list)
	}
	if list := items(a.m("GET", "/api/v1/executions/"+exID+"/defects", nil, 200)); len(list) != 1 {
		t.Fatal(list)
	}
	if list := items(a.m("GET", "/api/v1/projects/"+f.pid+"/defects?q=severity:major", nil, 200)); len(list) != 1 {
		t.Fatal(list)
	}
	if list := items(a.m("GET", "/api/v1/projects/"+f.pid+"/defects?q=severity:minor", nil, 200)); len(list) != 0 {
		t.Fatal(list)
	}
	if got := a.m("GET", "/api/v1/requirements/"+f.r1, nil, 200); got["open_defects"].(float64) != 1 {
		t.Fatal(got)
	}

	// without a tracker push fails
	a.m("POST", "/api/v1/defects/"+did+"/push", nil, 400)
	a.m("PUT", "/api/v1/projects/"+f.pid+"/defect-tracker", map[string]any{"type": "github", "url": ts.URL}, 400) // repo missing
	tr := a.m("PUT", "/api/v1/projects/"+f.pid+"/defect-tracker", map[string]any{"type": "github", "url": ts.URL, "repo": "acme/shop", "token": "ghp_secret"}, 200)
	if tr["token"] != nil || tr["has_token"] != true {
		t.Fatal(tr)
	}
	if got := a.m("GET", "/api/v1/projects/"+f.pid+"/defect-tracker", nil, 200); got["token"] != nil || got["has_token"] != true {
		t.Fatal(got)
	}
	// saving without a token keeps it
	a.m("PUT", "/api/v1/projects/"+f.pid+"/defect-tracker", map[string]any{"type": "github", "url": ts.URL, "repo": "acme/shop", "labels": "bug"}, 200)
	a.m("POST", "/api/v1/projects/"+f.pid+"/defect-tracker/check", nil, 200)

	pushed := a.m("POST", "/api/v1/defects/"+did+"/push", nil, 200)
	if pushed["external_key"] != "acme/shop#1" || pushed["external_url"] == "" {
		t.Fatal(pushed)
	}
	a.m("POST", "/api/v1/defects/"+did+"/push", nil, 409)
	if gh.auth[len(gh.auth)-1] != "Bearer ghp_secret" {
		t.Fatal(gh.auth)
	}
	// closed on GitHub → closed here
	gh.mu.Lock()
	gh.issues["1"]["state"] = "closed"
	gh.issues["1"]["state_reason"] = "completed"
	gh.mu.Unlock()
	synced := a.m("POST", "/api/v1/defects/"+did+"/sync", nil, 200)
	if synced["status"] != "closed" || synced["external_status"] == "" {
		t.Fatal(synced)
	}
	// import an existing issue; sync all
	gh.mu.Lock()
	gh.issues["7"] = map[string]any{"number": 7, "html_url": "https://github.example/acme/shop/issues/7", "title": "Slow search", "state": "closed", "state_reason": "not_planned"}
	gh.mu.Unlock()
	imp := a.m("POST", "/api/v1/projects/"+f.pid+"/defects/import", map[string]any{"external_key": "#7", "severity": "minor"}, 201)
	if imp["title"] != "Slow search" || imp["status"] != "rejected" {
		t.Fatal(imp)
	}
	a.m("POST", "/api/v1/projects/"+f.pid+"/defects/import", map[string]any{"external_key": "#99"}, 502)
	// a key the tracker cannot have is the caller's mistake, not the tracker's
	a.m("POST", "/api/v1/projects/"+f.pid+"/defects/import", map[string]any{"external_key": "abc"}, 400)
	a.m("POST", "/api/v1/projects/"+f.pid+"/defects/import", map[string]any{"external_key": "  "}, 400)
	// defect keys are found by the search (links to PROJ-BUG-n keys)
	if hits := items(a.m("GET", "/api/v1/search?q="+url.QueryEscape(`key:"BUGS-BUG-1"`)+"&types=defect", nil, 200)); len(hits) != 1 || hits[0]["type"] != "defect" || hits[0]["id"] != did {
		t.Fatal(hits)
	}
	if hits := items(a.m("GET", "/api/v1/search?q="+url.QueryEscape(`key:"BUGS-BUG-1"`), nil, 200)); len(hits) != 0 {
		t.Fatal("defects only when asked for", hits)
	}
	all := a.m("POST", "/api/v1/projects/"+f.pid+"/defects/sync", nil, 200)
	if all["synced"].(float64) != 2 {
		t.Fatal(all)
	}
	// clearing the token
	if got := a.m("PUT", "/api/v1/projects/"+f.pid+"/defect-tracker", map[string]any{"type": "github", "url": ts.URL, "repo": "acme/shop", "clear_token": true}, 200); got["has_token"] != false {
		t.Fatal(got)
	}
	// permissions: viewer reads, cannot create
	e.member("viewer@example.com", "viewer")
	v := e.login("viewer@example.com")
	v.m("GET", "/api/v1/defects/"+did, nil, 200)
	v.m("POST", "/api/v1/projects/"+f.pid+"/defects", map[string]any{"title": "x"}, 403)
	// deleting the run removes the execution link
	a.m("DELETE", "/api/v1/runs/"+run["id"].(string), nil, 204)
	if got := a.m("GET", "/api/v1/defects/"+did, nil, 200); got["links"] != nil && len(got["links"].([]any)) != 0 {
		t.Fatal(got)
	}
	a.m("DELETE", "/api/v1/defects/"+did, nil, 204)
	a.m("GET", "/api/v1/defects/"+did, nil, 404)
}

func TestRisks(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	f := qualityFixture(t, a, "RSK")
	k := a.m("POST", "/api/v1/projects/"+f.pid+"/risks", map[string]any{"title": "Data loss", "likelihood": 2, "impact": 5, "requirement_ids": []string{f.r1}}, 201)
	kid := k["id"].(string)
	if k["key"] != "RSK-RISK-1" || k["score"].(float64) != 10 || k["level"] != "high" || k["status"] != "identified" || k["covered"].(float64) != 1 {
		t.Fatal(k)
	}
	a.m("POST", "/api/v1/projects/"+f.pid+"/risks", map[string]any{"title": "x", "likelihood": 6}, 400)
	a.m("POST", "/api/v1/projects/"+f.pid+"/risks", map[string]any{"title": ""}, 400)
	low := a.m("POST", "/api/v1/projects/"+f.pid+"/risks", map[string]any{"title": "Typo"}, 201)
	if low["likelihood"].(float64) != 3 || low["level"] != "medium" {
		t.Fatal(low)
	}
	list := items(a.m("GET", "/api/v1/projects/"+f.pid+"/risks", nil, 200))
	if len(list) != 2 || list[0]["id"] != kid {
		t.Fatal(list)
	}
	if got := a.m("GET", "/api/v1/requirements/"+f.r1, nil, 200); got["risk_level"] != "high" {
		t.Fatal(got)
	}
	a.m("POST", "/api/v1/risks/"+kid+"/requirements", map[string]any{"add": []string{f.r2}}, 200)
	if got := items(a.m("GET", "/api/v1/requirements/"+f.r2+"/risks", nil, 200)); len(got) != 1 {
		t.Fatal(got)
	}
	upd := a.m("PATCH", "/api/v1/risks/"+kid, map[string]any{"likelihood": 5, "status": "mitigating", "mitigation": "Backups"}, 200)
	if upd["level"] != "critical" || upd["mitigation"] != "Backups" || len(upd["requirement_ids"].([]any)) != 2 {
		t.Fatal(upd)
	}
	a.m("PATCH", "/api/v1/risks/"+kid, map[string]any{"impact": "big"}, 400)
	a.m("PATCH", "/api/v1/risks/"+kid, map[string]any{"status": "gone"}, 400)
	// another project's requirement is refused
	other := a.project("OTH")
	oq := a.m("POST", "/api/v1/projects/"+other+"/requirements", map[string]any{"title": "Other"}, 201)
	a.m("POST", "/api/v1/risks/"+kid+"/requirements", map[string]any{"add": []string{oq["id"].(string)}}, 400)
	// viewers read only
	e.member("viewer@example.com", "viewer")
	v := e.login("viewer@example.com")
	v.m("GET", "/api/v1/risks/"+kid, nil, 200)
	v.m("PATCH", "/api/v1/risks/"+kid, map[string]any{"status": "closed"}, 403)
	// deleting a requirement removes it from the risk; trash restore brings it back
	a.m("DELETE", "/api/v1/requirements/"+f.r2, nil, 204)
	if got := a.m("GET", "/api/v1/risks/"+kid, nil, 200); len(got["requirement_ids"].([]any)) != 1 {
		t.Fatal(got)
	}
	a.m("DELETE", "/api/v1/risks/"+kid, nil, 204)
	a.m("GET", "/api/v1/risks/"+kid, nil, 404)
}

func TestNotificationsAndWebhooks(t *testing.T) {
	e := newEnv(t)
	type delivery struct {
		event, sig string
		body       []byte
	}
	var mu sync.Mutex
	var got []delivery
	fail := false
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, delivery{r.Header.Get("X-Qtestframe-Event"), r.Header.Get("X-Qtestframe-Signature"), b})
		f := fail
		mu.Unlock()
		if f {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(204)
	}))
	defer hook.Close()
	a := e.login("admin@example.com")
	anna := e.member("anna.berg@example.com", "tester")
	_ = anna
	n := e.login("anna.berg@example.com")
	f := qualityFixture(t, a, "NOT")

	a.m("POST", "/api/v1/projects/"+f.pid+"/webhooks", map[string]any{"name": "CI", "url": "ftp://x", "events": []string{"*"}}, 400)
	a.m("POST", "/api/v1/projects/"+f.pid+"/webhooks", map[string]any{"name": "CI", "url": hook.URL, "events": []string{"bogus"}}, 400)
	a.m("POST", "/api/v1/projects/"+f.pid+"/webhooks", map[string]any{"name": "CI", "url": hook.URL, "events": []string{}}, 400)
	wh := a.m("POST", "/api/v1/projects/"+f.pid+"/webhooks", map[string]any{"name": "CI", "url": hook.URL, "events": []string{"defect.created", "comment.created"}, "enabled": true}, 201)
	secret, _ := wh["secret"].(string)
	if !strings.HasPrefix(secret, "whsec_") {
		t.Fatal(wh)
	}
	if list := items(a.m("GET", "/api/v1/projects/"+f.pid+"/webhooks", nil, 200)); len(list) != 1 || list[0]["secret"] != nil || list[0]["has_secret"] != true {
		t.Fatal(list)
	}
	if ev := a.m("GET", "/api/v1/webhook-events", nil, 200); len(ev["items"].([]any)) < 10 {
		t.Fatal(ev)
	}

	// a defect assigned to anna: webhook + in-app notification
	a.m("POST", "/api/v1/projects/"+f.pid+"/defects", map[string]any{"title": "Crash", "assignee": "Anna.Berg@example.com"}, 201)
	// a comment mentioning anna
	a.m("POST", "/api/v1/requirements/"+f.r1+"/comments", map[string]any{"body": "@anna.berg please check"}, 201)
	// an event the webhook is not subscribed to
	a.m("POST", "/api/v1/projects/"+f.pid+"/risks", map[string]any{"title": "R"}, 201)
	// owner change
	a.m("PATCH", "/api/v1/requirements/"+f.r2, map[string]any{"owner": "@anna.berg"}, 200)
	e.srv.WaitWebhooks()

	mu.Lock()
	if len(got) != 2 || got[0].event != "defect.created" || got[1].event != "comment.created" {
		t.Fatal(got)
	}
	if got[0].sig != Sign(secret, got[0].body) {
		t.Fatal("bad signature")
	}
	var payload map[string]any
	_ = json.Unmarshal(got[0].body, &payload)
	if payload["event"] != "defect.created" || payload["project_id"] != f.pid || payload["actor"] != "admin@example.com" {
		t.Fatal(payload)
	}
	mu.Unlock()

	res := n.m("GET", "/api/v1/notifications", nil, 200)
	list := items(res)
	if res["unread"].(float64) != 3 || len(list) != 3 {
		t.Fatal(res)
	}
	events := map[string]bool{}
	for _, x := range list {
		events[x["event"].(string)] = true
	}
	if !events["defect.created"] || !events["mention"] || !events["assigned"] {
		t.Fatal(events)
	}
	// the actor gets nothing
	if res := a.m("GET", "/api/v1/notifications", nil, 200); res["unread"].(float64) != 0 {
		t.Fatal(res)
	}
	n.m("POST", "/api/v1/notifications/read", map[string]any{"ids": []string{list[0]["id"].(string)}}, 200)
	if res := n.m("GET", "/api/v1/notifications?unread=1", nil, 200); res["unread"].(float64) != 2 || len(items(res)) != 2 {
		t.Fatal(res)
	}
	if res := n.m("POST", "/api/v1/notifications/read", map[string]any{}, 200); res["marked"].(float64) != 2 {
		t.Fatal(res)
	}

	// test delivery, failure is recorded
	if res := a.m("POST", "/api/v1/webhooks/"+wh["id"].(string)+"/test", nil, 200); res["ok"] != true {
		t.Fatal(res)
	}
	mu.Lock()
	fail = true
	mu.Unlock()
	if res := a.m("POST", "/api/v1/webhooks/"+wh["id"].(string)+"/test", nil, 200); res["ok"] != false || res["status"].(float64) != 500 {
		t.Fatal(res)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+f.pid+"/webhooks", nil, 200)); l[0]["last_status"].(float64) != 500 || l[0]["last_error"] == "" {
		t.Fatal(l)
	}
	// rotate, disable, delete
	rot := a.m("PATCH", "/api/v1/webhooks/"+wh["id"].(string), map[string]any{"rotate_secret": true, "enabled": false}, 200)
	if rot["secret"] == secret || rot["secret"] == "" || rot["enabled"] != false {
		t.Fatal(rot)
	}
	if upd := a.m("PATCH", "/api/v1/webhooks/"+wh["id"].(string), map[string]any{"name": "CI 2"}, 200); upd["secret"] != nil {
		t.Fatal(upd)
	}
	n.m("GET", "/api/v1/projects/"+f.pid+"/webhooks", nil, 403)
	a.m("DELETE", "/api/v1/webhooks/"+wh["id"].(string), nil, 204)
	a.m("DELETE", "/api/v1/webhooks/"+wh["id"].(string), nil, 404)
}
