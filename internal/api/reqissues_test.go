package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

// fakeGitLabAPI is project qa/shop of a GitLab instance (REST API v4).
type fakeGitLabAPI struct {
	mu     sync.Mutex
	issues map[string]map[string]any
	notes  []string
	tokens []string
	posts  []map[string]any
	fail   int // answer the next request with this status
}

func (g *fakeGitLabAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.tokens = append(g.tokens, r.Header.Get("PRIVATE-TOKEN"))
	if r.Header.Get("Authorization") != "" {
		writeJSON(w, 400, map[string]any{"message": "unexpected Authorization header"})
		return
	}
	if g.fail != 0 {
		code := g.fail
		g.fail = 0
		writeJSON(w, code, map[string]any{"message": http.StatusText(code)})
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	base := "/api/v4/projects/qa%2Fshop"
	p := r.URL.EscapedPath()
	switch {
	case p == base && r.Method == "GET":
		writeJSON(w, 200, map[string]any{"id": 1})
	case p == base+"/issues" && r.Method == "POST":
		g.posts = append(g.posts, body)
		n := strconv.Itoa(len(g.issues) + 1)
		is := map[string]any{"iid": len(g.issues) + 1, "web_url": "https://gitlab.example/qa/shop/-/issues/" + n, "title": body["title"],
			"description": body["description"], "state": "opened", "references": map[string]any{"full": "qa/shop#" + n}}
		g.issues[n] = is
		writeJSON(w, 201, is)
	case strings.HasPrefix(p, base+"/issues/"):
		rest := strings.Split(strings.TrimPrefix(p, base+"/issues/"), "/")
		is, ok := g.issues[rest[0]]
		if !ok {
			writeJSON(w, 404, map[string]any{"message": "404 Not found"})
			return
		}
		switch {
		case len(rest) == 2 && rest[1] == "notes":
			g.notes = append(g.notes, body["body"].(string))
			writeJSON(w, 201, map[string]any{"id": 1})
		case r.Method == "PUT":
			is["title"], is["description"] = body["title"], body["description"]
			writeJSON(w, 200, is)
		default:
			writeJSON(w, 200, is)
		}
	default:
		writeJSON(w, 404, map[string]any{"message": "404 Not Found"})
	}
}

func TestRequirementIssuesGitLab(t *testing.T) {
	e := newEnv(t)
	gl := &fakeGitLabAPI{issues: map[string]map[string]any{}}
	ts := httptest.NewTLSServer(gl)
	defer ts.Close()
	e.srv.HTTPClient = ts.Client()
	a := e.login("admin@example.com")
	f := qualityFixture(t, a, "GLI")
	a.m("PATCH", "/api/v1/requirements/"+f.r1, map[string]any{"rationale": "Security.", "tags": []string{"auth", "web"}, "priority": "high",
		"criteria": []map[string]any{{"description": "Login | logout work", "method": "test", "threshold": "100%"}}}, 200)

	// without a tracker: no preview, nothing to create
	if got := a.m("GET", "/api/v1/requirements/"+f.r1+"/issue", nil, 200); got["tracker"] != "" || got["preview"] != nil {
		t.Fatal(got)
	}
	a.m("POST", "/api/v1/requirements/"+f.r1+"/issue", nil, 400)

	// settings: https only, project path or ID, defaults
	a.m("PUT", "/api/v1/projects/"+f.pid+"/defect-tracker", map[string]any{"type": "gitlab", "url": "http://gitlab.example", "project": "qa/shop"}, 400)
	a.m("PUT", "/api/v1/projects/"+f.pid+"/defect-tracker", map[string]any{"type": "gitlab", "project": "no spaces/x y"}, 400)
	a.m("PUT", "/api/v1/projects/"+f.pid+"/defect-tracker", map[string]any{"type": "gitlab", "project": "qa/../x"}, 400)
	a.m("PUT", "/api/v1/projects/"+f.pid+"/defect-tracker", map[string]any{"type": "gitlab", "project": "single"}, 400)
	def := a.m("PUT", "/api/v1/projects/"+f.pid+"/defect-tracker", map[string]any{"type": "gitlab", "project": "123"}, 200)
	if def["url"] != "https://gitlab.com" || def["project"] != "123" {
		t.Fatal(def)
	}
	tr := a.m("PUT", "/api/v1/projects/"+f.pid+"/defect-tracker", map[string]any{"type": "gitlab", "url": ts.URL + "/api/v4/", "project": ts.URL + "/qa/shop/",
		"token": "glpat-example-not-real", "labels": "qtestframe", "requirement_labels": "req, prio::{priority}, {tags}, {obligation}", "comment_on_pass": true}, 200)
	if tr["url"] != ts.URL || tr["project"] != "qa/shop" || tr["token"] != nil || tr["has_token"] != true {
		t.Fatal(tr)
	}
	// the token is encrypted at rest and never returned
	var raw string
	if err := e.st.DB.QueryRowContext(context.Background(), db.Rebind(e.st.DB.Dialect, `SELECT defect_tracker FROM projects WHERE id = ?`), f.pid).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "glpat-example-not-real") || !strings.Contains(raw, "enc:v1:") {
		t.Fatal(raw)
	}
	if got := a.m("GET", "/api/v1/projects/"+f.pid+"/defect-tracker", nil, 200); got["token"] != nil || got["comment_on_pass"] != true {
		t.Fatal(got)
	}
	a.m("POST", "/api/v1/projects/"+f.pid+"/defect-tracker/check", nil, 200)

	// preview
	pv := a.m("GET", "/api/v1/requirements/"+f.r1+"/issue", nil, 200)
	prev := pv["preview"].(map[string]any)
	desc := prev["description"].(string)
	if pv["tracker"] != "gitlab" || pv["link"] != nil || prev["title"] != "[GLI-REQ-1] Login" {
		t.Fatal(pv)
	}
	for _, want := range []string{"<!-- qtestframe:begin -->", "The user MUST log in.", "### Rationale", "Security.", "| 1 | Login \\| logout work | test | 100% |",
		"[GLI-REQ-1](http://example.test/go/GLI-REQ-1)", "- **Priority:** high", "[GLI-TC-1](http://example.test/go/GLI-TC-1) | Valid login | not run", "0 of 1 tests passed"} {
		if !strings.Contains(desc, want) {
			t.Errorf("description lacks %q:\n%s", want, desc)
		}
	}
	if got := strings.Join(toStrings(prev["labels"]), ","); got != "qtestframe,req,prio::high,auth,web,must" {
		t.Fatal(got)
	}

	// create; a second create is refused with the link
	cr := a.m("POST", "/api/v1/requirements/"+f.r1+"/issue", nil, 201)
	l := cr["link"].(map[string]any)
	is := l["issue"].(map[string]any)
	if l["url"] != "https://gitlab.example/qa/shop/-/issues/1" || l["label"] != "GitLab qa/shop#1" || is["key"] != "qa/shop#1" || is["state"] != "open" || is["status"] != "opened" || is["tracker"] != "gitlab" {
		t.Fatal(cr)
	}
	if gl.tokens[len(gl.tokens)-1] != "glpat-example-not-real" || gl.posts[0]["labels"] != "qtestframe,req,prio::high,auth,web,must" {
		t.Fatal(gl.tokens, gl.posts)
	}
	dup := a.m("POST", "/api/v1/requirements/"+f.r1+"/issue", nil, 409)
	if dup["code"] != "issue_exists" || dup["link"].(map[string]any)["id"] != l["id"] {
		t.Fatal(dup)
	}
	// the link is an external item link of the requirement, with the issue
	links := items(a.m("GET", "/api/v1/requirements/"+f.r1+"/item-links", nil, 200))
	if len(links) != 1 || links[0]["target_type"] != "url" || links[0]["issue"].(map[string]any)["key"] != "qa/shop#1" {
		t.Fatal(links)
	}
	if got := a.m("GET", "/api/v1/requirements/"+f.r1+"/issue", nil, 200); got["link"] == nil {
		t.Fatal(got)
	}

	// update: someone added notes in GitLab below the Qtestframe part
	gl.mu.Lock()
	gl.issues["1"]["description"] = gl.issues["1"]["description"].(string) + "\n\nNotes from the team."
	gl.mu.Unlock()
	a.m("PATCH", "/api/v1/requirements/"+f.r1, map[string]any{"title": "Login with SSO", "description": "The user MUST log in with SSO."}, 200)
	up := a.m("PUT", "/api/v1/requirements/"+f.r1+"/issue", nil, 200)
	gl.mu.Lock()
	d1 := gl.issues["1"]["description"].(string)
	title := gl.issues["1"]["title"]
	gl.mu.Unlock()
	if title != "[GLI-REQ-1] Login with SSO" || !strings.Contains(d1, "with SSO.") || strings.Contains(d1, "MUST log in.\n") || !strings.HasSuffix(d1, "Notes from the team.") || strings.Count(d1, "qtestframe:begin") != 1 {
		t.Fatal(d1)
	}
	if up["link"].(map[string]any)["issue"].(map[string]any)["synced_at"] == nil {
		t.Fatal(up)
	}
	a.m("PUT", "/api/v1/requirements/"+f.r2+"/issue", nil, 404)
	a.m("POST", "/api/v1/requirements/"+f.r2+"/issue/sync", nil, 404)

	// sync: the tests have not passed yet, no comment
	sy := a.m("POST", "/api/v1/requirements/"+f.r1+"/issue/sync", nil, 200)
	if sy["changed"] != false || sy["commented"] != false || len(gl.notes) != 0 {
		t.Fatal(sy)
	}
	// the test passes: one comment, not repeated
	run := a.m("POST", "/api/v1/projects/"+f.pid+"/runs", map[string]any{"name": "R", "test_ids": []string{f.t1}}, 201)
	exID := items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200))[0]["id"].(string)
	a.m("PATCH", "/api/v1/executions/"+exID, map[string]any{"status": "passed"}, 200)
	sy = a.m("POST", "/api/v1/requirements/"+f.r1+"/issue/sync", nil, 200)
	if sy["commented"] != true || len(gl.notes) != 1 || !strings.Contains(gl.notes[0], "All 1 linked tests of requirement GLI-REQ-1 passed") || !strings.Contains(gl.notes[0], "http://example.test/go/GLI-REQ-1") {
		t.Fatal(sy, gl.notes)
	}
	a.m("POST", "/api/v1/requirements/"+f.r1+"/issue/sync", nil, 200)
	if len(gl.notes) != 1 {
		t.Fatal(gl.notes)
	}
	// closed in GitLab → closed here
	gl.mu.Lock()
	gl.issues["1"]["state"] = "closed"
	gl.mu.Unlock()
	sy = a.m("POST", "/api/v1/requirements/"+f.r1+"/issue/sync", nil, 200)
	if sy["changed"] != true || sy["link"].(map[string]any)["issue"].(map[string]any)["state"] != "closed" {
		t.Fatal(sy)
	}

	// bulk: r1 has an issue (skipped, or updated), r2 gets one
	bulk := a.m("POST", "/api/v1/projects/"+f.pid+"/requirements/issues", map[string]any{"ids": []string{f.r1, f.r2}}, 200)
	if len(bulk["created"].([]any)) != 1 || len(bulk["skipped"].([]any)) != 1 || len(bulk["updated"].([]any)) != 0 || len(bulk["failed"].([]any)) != 0 {
		t.Fatal(bulk)
	}
	if c := bulk["created"].([]any)[0].(map[string]any); c["key"] != "GLI-REQ-2" || c["issue_key"] != "qa/shop#2" {
		t.Fatal(c)
	}
	bulk = a.m("POST", "/api/v1/projects/"+f.pid+"/requirements/issues", map[string]any{"ids": []string{f.r1, f.r2}, "update": true}, 200)
	if len(bulk["updated"].([]any)) != 2 || len(bulk["created"].([]any)) != 0 {
		t.Fatal(bulk)
	}
	a.m("POST", "/api/v1/projects/"+f.pid+"/requirements/issues", map[string]any{"ids": []string{}}, 400)
	a.m("POST", "/api/v1/projects/"+f.pid+"/requirements/issues", map[string]any{"ids": []string{"missing"}}, 404)
	// a requirement of another project is not found
	other := a.project("GLO")
	oreq := a.m("POST", "/api/v1/projects/"+other+"/requirements", map[string]any{"title": "Other"}, 201)["id"].(string)
	a.m("POST", "/api/v1/projects/"+f.pid+"/requirements/issues", map[string]any{"ids": []string{oreq}}, 404)

	// sync all, and some
	all := a.m("POST", "/api/v1/projects/"+f.pid+"/requirements/issues/sync", nil, 200)
	if all["synced"].(float64) != 2 || len(all["failed"].([]any)) != 0 {
		t.Fatal(all)
	}
	some := a.m("POST", "/api/v1/projects/"+f.pid+"/requirements/issues/sync", map[string]any{"ids": []string{f.r2}}, 200)
	if some["synced"].(float64) != 1 {
		t.Fatal(some)
	}
	a.m("POST", "/api/v1/projects/"+f.pid+"/requirements/issues/sync", "{", 400)

	// errors of GitLab: a deleted issue fails the sync with 502 and a clear message
	gl.mu.Lock()
	delete(gl.issues, "2")
	gl.mu.Unlock()
	bad := a.m("POST", "/api/v1/requirements/"+f.r2+"/issue/sync", nil, 502)
	if bad["code"] != "tracker_error" || !strings.Contains(bad["error"].(string), "404 (not found") {
		t.Fatal(bad)
	}
	all = a.m("POST", "/api/v1/projects/"+f.pid+"/requirements/issues/sync", nil, 200)
	if all["synced"].(float64) != 1 || len(all["failed"].([]any)) != 1 {
		t.Fatal(all)
	}
	gl.mu.Lock()
	gl.fail = 403
	gl.mu.Unlock()
	bad = a.m("PUT", "/api/v1/requirements/"+f.r1+"/issue", nil, 502)
	if !strings.Contains(bad["error"].(string), "permission denied") || strings.Contains(bad["error"].(string), "glpat") {
		t.Fatal(bad)
	}
	gl.mu.Lock()
	gl.fail = 401
	gl.mu.Unlock()
	bulk = a.m("POST", "/api/v1/projects/"+f.pid+"/requirements/issues", map[string]any{"ids": []string{f.r1}, "update": true}, 200)
	if len(bulk["failed"].([]any)) != 1 || !strings.Contains(bulk["failed"].([]any)[0].(map[string]any)["error"].(string), "token was rejected") {
		t.Fatal(bulk)
	}
	// a milestone GitLab cannot look up fails the create
	a.m("PUT", "/api/v1/projects/"+f.pid+"/defect-tracker", map[string]any{"type": "gitlab", "url": ts.URL, "project": "qa/shop", "milestone": "nope"}, 200)
	r3 := a.m("POST", "/api/v1/projects/"+f.pid+"/requirements", map[string]any{"title": "Third"}, 201)["id"].(string)
	a.m("POST", "/api/v1/requirements/"+r3+"/issue", nil, 502) // the milestone lookup itself is unknown to the fake
	a.m("PUT", "/api/v1/projects/"+f.pid+"/defect-tracker", map[string]any{"type": "gitlab", "url": ts.URL, "project": "qa/shop", "milestone": ""}, 200)

	// defects go to GitLab too
	d := a.m("POST", "/api/v1/projects/"+f.pid+"/defects", map[string]any{"title": "Crash", "severity": "major"}, 201)
	pushed := a.m("POST", "/api/v1/defects/"+d["id"].(string)+"/push", nil, 200)
	if pushed["external_system"] != "gitlab" || !strings.HasPrefix(pushed["external_key"].(string), "qa/shop#") {
		t.Fatal(pushed)
	}
	gl.mu.Lock()
	gl.issues[strings.TrimPrefix(pushed["external_key"].(string), "qa/shop#")]["state"] = "closed"
	gl.mu.Unlock()
	if synced := a.m("POST", "/api/v1/defects/"+d["id"].(string)+"/sync", nil, 200); synced["status"] != "closed" {
		t.Fatal(synced)
	}

	// permissions: a reviewer may read the preview but not create, update or sync
	e.member("rev@example.com", rbac.RoleReviewer)
	rv := e.login("rev@example.com")
	rv.m("GET", "/api/v1/requirements/"+f.r1+"/issue", nil, 200)
	rv.m("POST", "/api/v1/requirements/"+r3+"/issue", nil, 403)
	rv.m("PUT", "/api/v1/requirements/"+f.r1+"/issue", nil, 403)
	rv.m("POST", "/api/v1/requirements/"+f.r1+"/issue/sync", nil, 403)
	rv.m("POST", "/api/v1/projects/"+f.pid+"/requirements/issues", map[string]any{"ids": []string{r3}}, 403)
	rv.m("POST", "/api/v1/projects/"+f.pid+"/requirements/issues/sync", nil, 403)

	// audit: every create, update and sync, never the token
	var actions []string
	for _, x := range items(a.m("GET", "/api/v1/audit?limit=500", nil, 200)) {
		actions = append(actions, x["action"].(string))
		if strings.Contains(x["details"].(string), "glpat") {
			t.Fatal(x)
		}
	}
	for _, want := range []string{"requirement.issue.create", "requirement.issue.update", "requirement.issue.sync", "defect_tracker.update"} {
		if !slices.Contains(actions, want) {
			t.Errorf("audit: no %s in %v", want, actions)
		}
	}
}

// GitHub and Jira create issues from requirements through the same path.
func TestRequirementIssuesGitHubJira(t *testing.T) {
	e := newEnv(t)
	gh := &fakeGitHub{issues: map[string]map[string]any{}}
	ts := httptest.NewServer(gh)
	defer ts.Close()
	e.srv.HTTPClient = ts.Client()
	a := e.login("admin@example.com")
	f := qualityFixture(t, a, "GHI")
	a.m("PUT", "/api/v1/projects/"+f.pid+"/defect-tracker", map[string]any{"type": "github", "url": ts.URL, "repo": "acme/shop", "requirement_labels": "none"}, 200)
	pv := a.m("GET", "/api/v1/requirements/"+f.r1+"/issue", nil, 200)
	if len(pv["preview"].(map[string]any)["labels"].([]any)) != 0 {
		t.Fatal(pv)
	}
	cr := a.m("POST", "/api/v1/requirements/"+f.r1+"/issue", nil, 201)
	if cr["link"].(map[string]any)["issue"].(map[string]any)["key"] != "acme/shop#1" {
		t.Fatal(cr)
	}
	// another repository: the issue of acme/shop is not its issue
	a.m("PUT", "/api/v1/projects/"+f.pid+"/defect-tracker", map[string]any{"type": "github", "url": ts.URL, "repo": "acme/web"}, 200)
	if pv := a.m("GET", "/api/v1/requirements/"+f.r1+"/issue", nil, 200); pv["link"] != nil {
		t.Fatal(pv)
	}
	a.m("PUT", "/api/v1/requirements/"+f.r1+"/issue", nil, 404)
	// switching the tracker: the GitHub issue does not block a Jira one,
	// and is not synced with Jira
	a.m("PUT", "/api/v1/projects/"+f.pid+"/defect-tracker", map[string]any{"type": "jira", "url": ts.URL, "project_key": "SHOP"}, 200)
	if got := a.m("GET", "/api/v1/projects/"+f.pid+"/defect-tracker", nil, 200); got["requirement_issue_type"] != "Task" {
		t.Fatal(got)
	}
	pv = a.m("GET", "/api/v1/requirements/"+f.r1+"/issue", nil, 200)
	desc := pv["preview"].(map[string]any)["description"].(string)
	if pv["link"] != nil || !strings.Contains(desc, "h3. Requirement") || strings.Contains(desc, "qtestframe:begin") || !strings.Contains(desc, "[GHI-REQ-1|http://example.test/go/GHI-REQ-1]") {
		t.Fatal(desc)
	}
	if all := a.m("POST", "/api/v1/projects/"+f.pid+"/requirements/issues/sync", nil, 200); all["synced"].(float64) != 0 {
		t.Fatal(all)
	}
}

func TestRequirementLabels(t *testing.T) {
	req := &model.Requirement{Type: "functional", Priority: "high", Obligation: "", Status: "draft", Tags: []string{"a", "b,c", " "}}
	for tmpl, want := range map[string]string{
		"":                       "requirement,type::functional,priority::high,a,b c",
		"none":                   "",
		"NONE":                   "",
		"{obligation}, x":        "x",
		"tag::{tags}, {status}":  "tag::a,tag::b c,draft",
		"{type}/{priority}, , X": "functional/high,X",
		"a, A":                   "a",
	} {
		if got := strings.Join(requirementLabels(tmpl, req), ","); got != want {
			t.Errorf("%q: %q", tmpl, got)
		}
	}
	long := &model.Requirement{Key: "A-REQ-1", Title: strings.Repeat("x", 300)}
	if got := issueTitle(long); len([]rune(got)) != 255 || !strings.HasPrefix(got, "[A-REQ-1] x") {
		t.Fatal(len(got))
	}
}
