package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

// fakeGitLab records commit statuses and merge request notes.
type fakeGitLab struct {
	mu       sync.Mutex
	statuses []map[string]any
	notes    map[int]string
	nextNote int
	auth     []string
	fail     int // answer this status to status posts (0: none)
}

func newFakeGitLab(t *testing.T) (*fakeGitLab, *httptest.Server) {
	f := &fakeGitLab{notes: map[int]string{}, nextNote: 1}
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.auth = append(f.auth, r.Header.Get("PRIVATE-TOKEN")+r.Header.Get("JOB-TOKEN"))
		body, _ := io.ReadAll(r.Body)
		var in map[string]any
		_ = json.Unmarshal(body, &in)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/projects/grp%2Fapp/statuses/"):
			if f.fail != 0 {
				w.WriteHeader(f.fail)
				_, _ = w.Write([]byte(`{"message":"refused"}`))
				return
			}
			in["sha"] = strings.TrimPrefix(r.URL.EscapedPath(), "/api/v4/projects/grp%2Fapp/statuses/")
			f.statuses = append(f.statuses, in)
			_, _ = w.Write([]byte(`{"id":1}`))
		case r.Method == "GET" && r.URL.EscapedPath() == "/api/v4/projects/grp%2Fapp/merge_requests/5/notes":
			var list []map[string]any
			for id, b := range f.notes {
				list = append(list, map[string]any{"id": id, "body": b})
			}
			list = append(list, map[string]any{"id": 999, "body": "an unrelated note"})
			_ = json.NewEncoder(w).Encode(list)
		case r.Method == "POST" && r.URL.EscapedPath() == "/api/v4/projects/grp%2Fapp/merge_requests/5/notes":
			f.notes[f.nextNote] = in["body"].(string)
			f.nextNote++
			_, _ = w.Write([]byte(`{"id":1}`))
		case r.Method == "PUT" && strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/projects/grp%2Fapp/merge_requests/5/notes/"):
			f.notes[1] = in["body"].(string)
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
		}
	}))
	t.Cleanup(hs.Close)
	return f, hs
}

func (f *fakeGitLab) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.statuses), len(f.notes)
}

// ciFixture: requirements R1 (MUST; tests T1, T2) and R2 (T3, manual),
// automated tests T1 and T2 (runner:pytest), the run "Pipeline 1" of the
// build 100 at commit sha with T1 passed and T2 failed, and the merge
// request !5 that names R1.
type ciFixture struct {
	pid, pp, key       string
	r1, r2, t1, t2, t3 string
	k                  map[string]string
	run                string
}

const ciSHA = "0123456789abcdef0123456789abcdef01234567"

func newCIFixture(t *testing.T, a *client) *ciFixture {
	t.Helper()
	f := &ciFixture{k: map[string]string{}, key: "CI"}
	f.pid = a.project("CI")
	f.pp = "/api/v1/projects/" + f.pid
	mk := func(kind string, body map[string]any) string {
		m := a.m("POST", f.pp+"/"+kind, body, 201)
		f.k[m["id"].(string)] = m["key"].(string)
		return m["id"].(string)
	}
	f.r1 = mk("requirements", map[string]any{"title": "Checkout", "obligation": "must"})
	f.r2 = mk("requirements", map[string]any{"title": "Invoice", "obligation": "should"})
	f.t1 = mk("tests", map[string]any{"title": "Checkout works", "execution_type": "automated"})
	f.t2 = mk("tests", map[string]any{"title": "Checkout rejects bad cards", "execution_type": "automated", "tags": []string{"runner:pytest", "always-run"}})
	f.t3 = mk("tests", map[string]any{"title": "Invoice looks right"})
	for _, l := range [][2]string{{f.r1, f.t1}, {f.r1, f.t2}, {f.r2, f.t3}} {
		a.m("POST", f.pp+"/links", map[string]any{"requirement_id": l[0], "test_case_id": l[1]}, 201)
	}
	f.run = a.m("POST", f.pp+"/runs", map[string]any{"name": "Pipeline 1"}, 201)["id"].(string)
	a.m("POST", "/api/v1/runs/"+f.run+"/results?build_id=100&build_number=7&commit="+ciSHA+"&branch=feature&merge_request=5", map[string]any{"results": []map[string]any{
		{"test_key": f.k[f.t1], "status": "passed"}, {"test_key": f.k[f.t2], "status": "failed"}}}, 200)
	a.m("POST", "/api/v1/requirements/"+f.r1+"/code", map[string]any{"kind": "merge_request", "ref": "5", "title": "Checkout with cards", "provider": "gitlab", "status": "open", "branch": "feature"}, 201)
	return f
}

func condition(t *testing.T, rep map[string]any, typ string) map[string]any {
	t.Helper()
	for _, c := range toMaps(rep["conditions"]) {
		if c["type"] == typ {
			return c
		}
	}
	t.Fatalf("no condition %s in %v", typ, rep["conditions"])
	return nil
}

func TestCIGate(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	f := newCIFixture(t, a)
	// default conditions on the run: the MUST requirement failed
	rep := a.m("POST", f.pp+"/gate", map[string]any{"run": "Pipeline 1", "merge_request": "5"}, 200)
	if rep["pass"] != false || rep["verdict"] != "fail" || len(toMaps(rep["conditions"])) != 4 {
		t.Fatal(rep)
	}
	if c := condition(t, rep, "mandatory_passed"); c["met"] != false || toMaps(c["items"])[0]["key"] != f.k[f.r1] || !strings.Contains(toMaps(c["items"])[0]["url"].(string), "/requirements?item=") {
		t.Fatal(c)
	}
	if c := condition(t, rep, "no_failed"); c["met"] != false || c["label"] != "no_failed" {
		t.Fatal(c)
	}
	res := rep["results"].(map[string]any)
	if res["passed"] != 1.0 || res["failed"] != 1.0 || res["total"] != 2.0 || len(toMaps(rep["failures"])) != 1 {
		t.Fatal(res, rep["failures"])
	}
	if !strings.HasSuffix(rep["url"].(string), "/runs/"+f.run) || rep["run"].(map[string]any)["id"] != f.run {
		t.Fatal(rep["url"])
	}
	aff := rep["affected"].(map[string]any)
	if aff["total"] != 1.0 || toMaps(aff["requirements"])[0]["result"] != "failed" || aff["covered"] != 1.0 {
		t.Fatal(aff)
	}
	// ad-hoc conditions
	rep = a.m("POST", f.pp+"/gate", map[string]any{"run": f.run, "require": []string{"coverage>=50"}}, 200)
	if rep["pass"] != true || condition(t, rep, "coverage")["label"] != "coverage >= 50%" {
		t.Fatal(rep)
	}
	rep = a.m("GET", f.pp+"/gate?build=100&require=no_failed&require=must", nil, 200)
	if rep["pass"] != false || len(toMaps(rep["conditions"])) != 2 || rep["build"].(map[string]any)["name"] == "" {
		t.Fatal(rep)
	}
	// the scope of the merge request: only R1
	rep = a.m("POST", f.pp+"/gate", map[string]any{"run": f.run, "scope": "affected", "merge_request": "!5", "require": []string{"coverage"}}, 200)
	if rep["scope"] != "affected" || rep["scope_size"] != 1.0 || rep["pass"] != true {
		t.Fatal(rep)
	}
	rep = a.m("POST", f.pp+"/gate", map[string]any{"scope": "obligation:should", "require": []string{"coverage"}}, 200)
	if rep["scope_size"] != 1.0 {
		t.Fatal(rep)
	}
	// summary only
	rep = a.m("POST", f.pp+"/gate", map[string]any{"run": f.run, "summary_only": true}, 200)
	if rep["pass"] != false || len(toMaps(rep["conditions"])) != 0 || rep["summary_only"] != true {
		t.Fatal(rep)
	}
	// a release: its exit criteria on its runs
	rel := a.m("POST", f.pp+"/releases", map[string]any{"name": "1.0", "version": "1.0.0", "exit_criteria": []map[string]any{{"type": "coverage", "threshold": 100}, {"type": "no_stale"}}}, 201)
	a.m("POST", "/api/v1/releases/"+rel["id"].(string)+"/requirements", map[string]any{"add": []string{f.r1}}, 200)
	a.m("PATCH", "/api/v1/runs/"+f.run, map[string]any{"release_id": rel["id"]}, 200)
	rep = a.m("POST", f.pp+"/gate", map[string]any{"release": "1.0.0"}, 200)
	if rep["pass"] != true || rep["release"].(map[string]any)["name"] != "1.0" || rep["scope"] != "release" || rep["scope_size"] != 1.0 {
		t.Fatal(rep)
	}
	rep = a.m("POST", f.pp+"/gate", map[string]any{"release": "1.0", "require": []string{"no_failed"}, "build": "7"}, 200)
	if rep["pass"] != false {
		t.Fatal(rep)
	}
	// errors
	a.m("POST", f.pp+"/gate", map[string]any{"require": []string{"bogus"}}, 400)
	a.m("POST", f.pp+"/gate", map[string]any{"require": []string{"coverage>=200"}}, 400)
	a.m("POST", f.pp+"/gate", map[string]any{"require": []string{"cycle_executed:Nope"}}, 400)
	a.m("POST", f.pp+"/gate", map[string]any{"release": "nope"}, 400)
	a.m("POST", f.pp+"/gate", map[string]any{"run": "nope"}, 400)
	a.m("POST", f.pp+"/gate", map[string]any{"release": "1.0", "scope": "affected"}, 400)
	a.m("POST", f.pp+"/gate", map[string]any{"scope": "affected"}, 400)
	a.m("POST", f.pp+"/gate", map[string]any{"variant": "nope"}, 400)
	// a viewer may read the gate; results need run.read
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	v.m("POST", f.pp+"/gate", map[string]any{"require": []string{"coverage"}}, 200)
}

func TestCISelect(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	f := newCIFixture(t, a)
	sel := a.m("POST", f.pp+"/ci/select", map[string]any{"merge_request": "5"}, 200)
	tests := toMaps(sel["tests"])
	if len(tests) != 2 || tests[0]["key"] != f.k[f.t1] || tests[1]["runner"] != "pytest" || tests[0]["runner"] != "default" || tests[1]["identifier"] != strings.ReplaceAll(f.k[f.t2], "-", "_") {
		t.Fatal(tests)
	}
	if len(toMaps(sel["requirements"])) != 1 || len(toMaps(sel["changes"])) != 1 || toMaps(sel["changes"])[0]["key"] != "!5" {
		t.Fatal(sel)
	}
	// keys from commit messages (short keys of the project), tags, unknown keys
	sel = a.m("POST", f.pp+"/ci/select", map[string]any{"keys": []string{"REQ-2", "CI-REQ-999"}, "tags": []string{"Always-Run"}}, 200)
	if len(toMaps(sel["tests"])) != 1 || len(toMaps(sel["manual"])) != 1 || toMaps(sel["manual"])[0]["key"] != f.k[f.t3] {
		t.Fatal(sel)
	}
	if u := sel["unknown"].([]any); len(u) != 1 || u[0] != "CI-REQ-999" {
		t.Fatal(sel["unknown"])
	}
	sel = a.m("POST", f.pp+"/ci/select", map[string]any{"keys": []string{"REQ-2"}, "include_manual": true}, 200)
	if len(toMaps(sel["tests"])) != 1 || len(toMaps(sel["manual"])) != 0 {
		t.Fatal(sel)
	}
	// commits by SHA prefix (a stored commit)
	a.m("POST", "/api/v1/tests/"+f.t1+"/code", map[string]any{"kind": "commit", "ref": ciSHA, "title": "fix"}, 201)
	sel = a.m("POST", f.pp+"/ci/select", map[string]any{"commits": []string{ciSHA[:10], "short", "ffffffffff"}}, 200)
	if len(toMaps(sel["tests"])) != 1 || toMaps(sel["tests"])[0]["key"] != f.k[f.t1] {
		t.Fatal(sel)
	}
	sel = a.m("POST", f.pp+"/ci/select", map[string]any{"all": true}, 200)
	if len(toMaps(sel["tests"])) != 2 || len(toMaps(sel["manual"])) != 1 {
		t.Fatal(sel)
	}
	// a run of the selection, for a release given by name
	a.m("POST", f.pp+"/releases", map[string]any{"name": "2.0"}, 201)
	sel = a.m("POST", f.pp+"/ci/select", map[string]any{"merge_request": "5", "create_run": true, "run": "Pipeline 9", "release_id": "2.0"}, 200)
	run := sel["run"].(map[string]any)
	got := a.m("GET", "/api/v1/runs/"+run["id"].(string), nil, 200)
	if got["name"] != "Pipeline 9" || got["release_id"] == nil {
		t.Fatal(got)
	}
	a.m("POST", f.pp+"/ci/select", map[string]any{"create_run": true, "release_id": "nope"}, 400)
	// nothing selected: no run
	sel = a.m("POST", f.pp+"/ci/select", map[string]any{"create_run": true}, 200)
	if sel["run"] != nil || len(toMaps(sel["tests"])) != 0 {
		t.Fatal(sel)
	}
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	v.m("POST", f.pp+"/ci/select", map[string]any{"merge_request": "5"}, 200)
	v.m("POST", f.pp+"/ci/select", map[string]any{"merge_request": "5", "create_run": true}, 403)
}

func TestCISetupAndTemplates(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("SETUP")
	setup := a.m("GET", "/api/v1/projects/"+pid+"/ci/setup", nil, 200)
	if setup["project_key"] != "SETUP" || setup["server_url"] != "http://example.test" || setup["token_role"] != "tester" {
		t.Fatal(setup)
	}
	tpls := toMaps(setup["templates"])
	if len(tpls) != 4 || !strings.Contains(tpls[0]["content"].(string), `QTF_PROJECT: "SETUP"`) || !strings.Contains(tpls[0]["content"].(string), "http://example.test") {
		t.Fatal(tpls[0])
	}
	if tpls[0]["url"] != "http://example.test/api/v1/ci/templates/gitlab-ci.yml" {
		t.Fatal(tpls[0]["url"])
	}
	// public template download
	res := e.anon().raw("GET", "/api/v1/ci/templates/gitlab-ci.yml?project=SHOP&download=true", nil)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), `QTF_PROJECT: "SHOP"`) || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/yaml") || res.Header.Get("Content-Disposition") == "" {
		t.Fatal(res.StatusCode, string(body))
	}
	for _, path := range []string{"/api/v1/ci/templates/nope.yml", "/api/v1/ci/templates/gitlab-ci.yml?project=a%20b"} {
		res := e.anon().raw("GET", path, nil)
		res.Body.Close()
		if res.StatusCode == 200 {
			t.Fatal(path)
		}
	}
}

func waitDeliveries(t *testing.T, a *client, pp string, done func([]map[string]any) bool) []map[string]any {
	t.Helper()
	for i := 0; i < 100; i++ {
		list := items(a.m("GET", pp+"/ci/deliveries", nil, 200))
		if done(list) {
			return list
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("deliveries did not finish:", items(a.m("GET", pp+"/ci/deliveries", nil, 200)))
	return nil
}

func TestCIServerSidePosting(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	gl, hs := newFakeGitLab(t)
	f := newCIFixture(t, a)
	// settings: validation, write-only token
	a.m("PUT", f.pp+"/ci-settings", map[string]any{"ci": map[string]any{"provider": "jenkins"}}, 400)
	a.m("PUT", f.pp+"/ci-settings", map[string]any{"ci": map[string]any{"provider": "gitlab"}}, 400)
	a.m("PUT", f.pp+"/ci-settings", map[string]any{"ci": map[string]any{"provider": "gitlab", "project": "g/a", "api_url": "ftp://x"}}, 400)
	a.m("PUT", f.pp+"/ci-settings", map[string]any{"ci": map[string]any{"provider": "gitlab", "project": "g/a", "conditions": []string{"bogus"}}}, 400)
	a.m("PUT", f.pp+"/ci-settings", map[string]any{"ci": map[string]any{"provider": "gitlab", "project": "g/a", "release_id": "nope"}}, 404)
	a.m("PUT", f.pp+"/ci-settings", map[string]any{"ci": map[string]any{"provider": "gitlab", "project": "g/a"}, "ci_token": "two words"}, 400)
	cs := a.m("PUT", f.pp+"/ci-settings", map[string]any{"ci": map[string]any{"provider": "gitlab", "api_url": hs.URL + "/api/v4/", "project": "/grp/app/",
		"post_status": true, "post_note": true, "conditions": []string{"no_failed"}}, "ci_token": "test-token-value"}, 200)
	if cs["has_ci_token"] != true || cs["ci_token"] != nil || cs["ci"].(map[string]any)["project"] != "grp/app" {
		t.Fatal(cs)
	}
	// the token is encrypted at rest
	var raw string
	if err := e.st.DB.QueryRowContext(context.Background(), e.st.DB.Rebind(`SELECT ci_token FROM code_settings WHERE project_id = ?`), f.pid).Scan(&raw); err != nil || raw == "" || strings.Contains(raw, "test-token") {
		t.Fatal(raw, err)
	}
	// the webhook secret keeps the CI settings
	a.m("PUT", f.pp+"/code-settings", map[string]any{"secret": strings.Repeat("s", 20)}, 200)
	cs = a.m("GET", f.pp+"/code-settings", nil, 200)
	if cs["has_ci_token"] != true || cs["has_secret"] != true || cs["ci"].(map[string]any)["post_note"] != true {
		t.Fatal(cs)
	}
	// results with a commit: status and note posted in the background
	up := "/api/v1/runs/" + f.run + "/results?build_id=100&commit=" + ciSHA + "&merge_request=5"
	a.m("POST", up, map[string]any{"results": []map[string]any{{"test_key": f.k[f.t2], "status": "failed"}}}, 200)
	list := waitDeliveries(t, a, f.pp, func(l []map[string]any) bool { return len(l) == 1 && l[0]["status"] == model.CISent })
	if !strings.Contains(list[0]["outcome"].(string), "qtestframe/gate failed") || !strings.Contains(list[0]["outcome"].(string), "note !5 added") {
		t.Fatal(list[0])
	}
	gl.mu.Lock()
	if len(gl.statuses) != 2 || gl.statuses[0]["name"] != "qtestframe/tests" || gl.statuses[0]["state"] != "failed" || gl.statuses[0]["sha"] != ciSHA ||
		!strings.HasSuffix(gl.statuses[0]["target_url"].(string), "/runs/"+f.run) || gl.statuses[1]["name"] != "qtestframe/gate" {
		t.Fatal(gl.statuses)
	}
	if len(gl.notes) != 1 || !strings.Contains(gl.notes[1], "<!-- qtestframe:ci-summary:CI -->") || !strings.Contains(gl.notes[1], "quality gate failed") || gl.auth[0] != "test-token-value" {
		t.Fatal(gl.notes, gl.auth)
	}
	gl.mu.Unlock()
	// the next upload updates the note instead of adding one
	a.m("POST", up, map[string]any{"results": []map[string]any{{"test_key": f.k[f.t2], "status": "passed"}}}, 200)
	list = waitDeliveries(t, a, f.pp, func(l []map[string]any) bool { return len(l) == 2 && l[0]["status"] == model.CISent })
	if !strings.Contains(list[0]["outcome"].(string), "qtestframe/gate success") || !strings.Contains(list[0]["outcome"].(string), "updated") {
		t.Fatal(list[0])
	}
	if s, n := gl.counts(); s != 4 || n != 1 {
		t.Fatal(s, n)
	}
	// a refused token is not retried; a server error is
	gl.mu.Lock()
	gl.fail = http.StatusUnauthorized
	gl.mu.Unlock()
	a.m("POST", up, map[string]any{"results": []map[string]any{{"test_key": f.k[f.t1], "status": "passed"}}}, 200)
	list = waitDeliveries(t, a, f.pp, func(l []map[string]any) bool { return len(l) == 3 && l[0]["status"] == model.CIFailed })
	if list[0]["status_code"] != 401.0 || !strings.Contains(list[0]["error"].(string), "GitLab answered 401") {
		t.Fatal(list[0])
	}
	gl.mu.Lock()
	gl.fail = http.StatusInternalServerError
	gl.mu.Unlock()
	a.m("POST", up, map[string]any{"results": []map[string]any{{"test_key": f.k[f.t1], "status": "passed"}}}, 200)
	list = waitDeliveries(t, a, f.pp, func(l []map[string]any) bool { return len(l) == 4 && l[0]["status"] == model.CIRetry })
	if list[0]["attempts"] != 1.0 || list[0]["next_attempt_at"] == nil {
		t.Fatal(list[0])
	}
	// the minute job retries once the backoff has passed
	gl.mu.Lock()
	gl.fail = 0
	gl.mu.Unlock()
	later := time.Now().Add(10 * time.Minute)
	e.st.SetClock(func() time.Time { return later })
	defer e.st.SetClock(time.Now)
	if n, err := e.srv.RunCIJobs(context.Background()); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	list = items(a.m("GET", f.pp+"/ci/deliveries", nil, 200))
	if list[0]["status"] != model.CISent || list[0]["attempts"] != 2.0 {
		t.Fatal(list[0])
	}
	// without a commit nothing is queued; posting off: nothing either
	a.m("POST", "/api/v1/runs/"+f.run+"/results", map[string]any{"results": []map[string]any{{"test_key": f.k[f.t1], "status": "passed"}}}, 200)
	a.m("PUT", f.pp+"/ci-settings", map[string]any{"ci": map[string]any{}, "clear_ci_token": true}, 200)
	a.m("POST", up, map[string]any{"results": []map[string]any{{"test_key": f.k[f.t1], "status": "passed"}}}, 200)
	if n := len(items(a.m("GET", f.pp+"/ci/deliveries", nil, 200))); n != 4 {
		t.Fatal(n)
	}
	// a viewer may not see the settings or deliveries
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	v.m("GET", f.pp+"/ci/deliveries", nil, 403)
	v.m("PUT", f.pp+"/ci-settings", map[string]any{"ci": map[string]any{}}, 403)
}
