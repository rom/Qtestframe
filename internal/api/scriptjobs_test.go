package api

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/runner"
)

// webEnv is an env whose web tests run the fake Playwright of
// internal/runner/testdata (no browser needed).
func webEnv(t *testing.T, mut ...func(*config.Config)) *env {
	t.Helper()
	fake, err := filepath.Abs("../runner/testdata/fake-playwright.sh")
	if err != nil {
		t.Fatal(err)
	}
	return newEnv(t, append([]func(*config.Config){func(c *config.Config) {
		c.Runner.Web.Enabled = true
		c.Runner.Web.Command = "/bin/sh " + fake
		c.Runner.Web.MaxParallel = 2
	}}, mut...)...)
}

// startJobs runs the worker pool until the test ends.
func (e *env) startJobs() {
	ctx, cancel := context.WithCancel(context.Background())
	e.t.Cleanup(func() {
		cancel()
		e.srv.WaitJobsStopped(5 * time.Second)
	})
	e.srv.StartScriptJobs(ctx)
}

func (e *env) waitJobs() {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := e.srv.WaitScriptJobs(ctx); err != nil {
		e.t.Fatal(err)
	}
}

func webTest(t *testing.T, a *client, pid, title, script string) string {
	t.Helper()
	return a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": title, "execution_type": "automated",
		"script_language": "playwright", "script": script, "status": "ready"}, 201)["id"].(string)
}

func TestWebTestSettings(t *testing.T) {
	e := webEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("WTS")
	path := "/api/v1/projects/" + pid + "/web-tests"
	got := a.m("GET", path, nil, 200)
	if got["base_url"] != "" || len(got["variables"].([]any)) != 0 || got["enabled"] != true || got["browser"] != "chromium" {
		t.Fatal(got)
	}
	got = a.m("PUT", path, map[string]any{"base_url": " https://staging.example.com ",
		"environment_urls": []map[string]any{{"environment": "prod", "base_url": "https://example.com"}, {"environment": "", "base_url": ""}},
		"variables":        []map[string]any{{"name": "USERNAME", "value": "alice"}, {"name": "password", "value": "s3cr3t", "secret": true}}}, 200)
	vars := got["variables"].([]any)
	if got["base_url"] != "https://staging.example.com" || len(got["environment_urls"].([]any)) != 1 || len(vars) != 2 {
		t.Fatal(got)
	}
	if pw := vars[1].(map[string]any); pw["value"] != "" || pw["has_value"] != true || pw["secret"] != true {
		t.Fatalf("secret returned: %v", pw)
	}
	// the secret stays when sent without a value, and never comes back
	a.m("PUT", path, map[string]any{"base_url": "https://staging.example.com",
		"variables": []map[string]any{{"name": "USERNAME", "value": "bob"}, {"name": "PASSWORD", "secret": true}}}, 200)
	got = a.m("GET", path, nil, 200)
	if v := got["variables"].([]any)[0].(map[string]any); v["value"] != "bob" {
		t.Fatal(v)
	}
	if v := got["variables"].([]any)[1].(map[string]any); v["value"] != "" || v["has_value"] != true {
		t.Fatal(v)
	}
	set, err := e.st.WebTestSettings(context.Background(), e.tenant.ID, pid)
	if err != nil || set.Variables[1].Value != "s3cr3t" {
		t.Fatalf("stored secret %+v %v", set, err)
	}
	for _, bad := range []map[string]any{
		{"base_url": "ftp://x"},
		{"environment_urls": []map[string]any{{"environment": "a", "base_url": "nope"}}},
		{"environment_urls": []map[string]any{{"environment": "", "base_url": "https://x"}}},
		{"environment_urls": []map[string]any{{"environment": "A", "base_url": "https://x"}, {"environment": "a", "base_url": "https://y"}}},
		{"variables": []map[string]any{{"name": "1BAD"}}},
		{"variables": []map[string]any{{"name": "A"}, {"name": "a"}}},
		{"variables": []map[string]any{{"name": "A", "value": strings.Repeat("x", 5000)}}},
		{"variables": []map[string]any{{"name": "A", "value": "a\x00b"}}},
	} {
		a.m("PUT", path, bad, 400)
	}
	a.m("GET", "/api/v1/projects/missing/web-tests", nil, 404)
	e.member("viewer@example.com", "viewer")
	v := e.login("viewer@example.com")
	v.m("PUT", path, map[string]any{}, 403)
	if m := a.m("GET", "/api/v1/meta", nil, 200); m["web_tests_enabled"] != true || !strings.Contains(strings.Join(strs(m["script_languages"]), ","), "playwright") {
		t.Fatal(m)
	}
}

func TestTargetFor(t *testing.T) {
	w := config.Default().Runner.Web
	set := &model.WebTestSettings{BaseURL: "https://default.example.com", EnvironmentURLs: []model.EnvironmentURL{{Environment: "Staging", BaseURL: "https://staging.example.com"}}}
	if tg := targetFor(w, set, "", nil); tg.BaseURL != "https://default.example.com" || tg.Browser != "chromium" || len(tg.Notes) != 0 {
		t.Errorf("%+v", tg)
	}
	if tg := targetFor(w, set, "staging", nil); tg.BaseURL != "https://staging.example.com" {
		t.Errorf("env %+v", tg)
	}
	tg := targetFor(w, set, "staging", []model.DimensionValue{{Dimension: "Webbläsare", Value: "Firefox"}, {Dimension: "Base URL", Value: "https://cfg.example.com"}, {Dimension: "OS", Value: "Linux"}})
	if tg.BaseURL != "https://cfg.example.com" || tg.Browser != "firefox" || len(tg.Notes) != 3 {
		t.Errorf("cfg %+v", tg)
	}
	tg = targetFor(w, set, "", []model.DimensionValue{{Dimension: "browser", Value: "Lynx"}, {Dimension: "URL", Value: "not a url"}})
	if tg.BaseURL != "https://default.example.com" || tg.Browser != "chromium" || !strings.Contains(strings.Join(tg.Notes, ";"), "unknown browser Lynx") {
		t.Errorf("bad values %+v", tg)
	}
	w.BrowserChannels = true
	if tg := targetFor(w, set, "", []model.DimensionValue{{Dimension: "Browser", Value: "Edge"}}); tg.Channel != "msedge" {
		t.Errorf("channel %+v", tg)
	}
}

func TestWebTestRunScriptJob(t *testing.T) {
	e := webEnv(t)
	e.startJobs()
	a := e.login("admin@example.com")
	pid := a.project("WEB")
	a.m("PUT", "/api/v1/projects/"+pid+"/web-tests", map[string]any{"base_url": "https://app.example.com",
		"variables": []map[string]any{{"name": "TOKEN", "value": "s3cr3t-token", "secret": true}, {"name": "USERNAME", "value": "alice"}}}, 200)
	ok := webTest(t, a, pid, "Login page", "await page.goto('/login')")
	bad := webTest(t, a, pid, "Broken", "// FAIL")
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R", "test_ids": []string{ok, bad}}, 201)
	ex := execsOf(t, a, run["id"].(string))
	byTest := map[string]string{}
	for _, x := range ex {
		byTest[x["test_case_id"].(string)] = x["id"].(string)
	}

	d := a.m("POST", "/api/v1/executions/"+byTest[ok]+"/run-script", nil, 202)
	if d["status"] != "in_progress" || d["job"] == nil {
		t.Fatal(d)
	}
	e.waitJobs()
	d = a.m("GET", "/api/v1/executions/"+byTest[ok], nil, 200)
	job := d["job"].(map[string]any)
	if d["status"] != "passed" || job["state"] != "passed" || d["executed_by"] != "admin@example.com" ||
		!strings.Contains(d["output"].(string), "base=https://app.example.com browser=chromium") || !strings.Contains(d["output"].(string), "user=alice") {
		t.Fatal(d)
	}

	a.m("POST", "/api/v1/executions/"+byTest[bad]+"/run-script", nil, 202)
	e.waitJobs()
	d = a.m("GET", "/api/v1/executions/"+byTest[bad], nil, 200)
	out := d["output"].(string)
	if d["status"] != "failed" || strings.Contains(out, "s3cr3t-token") || !strings.Contains(out, "token=***") {
		t.Fatalf("failed run: %v", d)
	}
	if j := d["job"].(map[string]any); j["state"] != "failed" || !strings.Contains(j["message"].(string), "Expected heading, got ***") {
		t.Fatal(j)
	}
	files := items(a.m("GET", "/api/v1/executions/"+byTest[bad]+"/attachments", nil, 200))
	names := []string{}
	for _, f := range files {
		names = append(names, f["name"].(string))
		if !strings.Contains(strings.Join(strs(f["tags"]), ","), "web-test") {
			t.Errorf("tags %v", f["tags"])
		}
	}
	if strings.Join(names, ",") != "screenshot.png,trace.zip,video.webm" {
		t.Fatalf("evidence %v", names)
	}
	// a manual attachment stays when the web test runs again; the evidence is replaced
	a.upload("POST", "/api/v1/executions/"+byTest[bad]+"/attachments", "file", "notes.txt", "text/plain", []byte("mine"), nil, 201)
	a.m("POST", "/api/v1/executions/"+byTest[bad]+"/run-script", nil, 202)
	e.waitJobs()
	if files := items(a.m("GET", "/api/v1/executions/"+byTest[bad]+"/attachments", nil, 200)); len(files) != 4 {
		t.Fatalf("after rerun %d files", len(files))
	}
	audit := a.m("GET", "/api/v1/audit", nil, 200)
	if !strings.Contains(stringify(audit), "execution.script_queued") {
		t.Error("audit lacks execution.script_queued")
	}

	// a viewer may read jobs but not run or cancel them
	e.member("viewer@example.com", "viewer")
	v := e.login("viewer@example.com")
	v.m("POST", "/api/v1/executions/"+byTest[ok]+"/run-script", nil, 403)
	v.m("POST", "/api/v1/runs/"+run["id"].(string)+"/run-automated", nil, 403)
	v.m("GET", "/api/v1/runs/"+run["id"].(string)+"/jobs", nil, 200)

	// web tests disabled
	e.srv.Runner.Cfg.Web.Enabled = false
	a.m("POST", "/api/v1/executions/"+byTest[ok]+"/run-script", nil, 409)
	r := a.m("POST", "/api/v1/runs/"+run["id"].(string)+"/run-automated", map[string]any{"include_passed": true}, 202)
	if len(r["queued"].([]any)) != 0 || r["skipped"].([]any)[0].(map[string]any)["code"] != "disabled" {
		t.Fatal(r)
	}
}

func stringify(v any) string {
	var b strings.Builder
	var walk func(any)
	walk = func(x any) {
		switch y := x.(type) {
		case map[string]any:
			for _, z := range y {
				walk(z)
			}
		case []any:
			for _, z := range y {
				walk(z)
			}
		case string:
			b.WriteString(y + "\n")
		}
	}
	walk(v)
	return b.String()
}

func TestRunAutomatedTests(t *testing.T) {
	e := webEnv(t)
	a := e.login("admin@example.com")
	// prerequisites must pass (the other rules stay relaxed)
	a.m("PUT", "/api/v1/settings", map[string]any{"allow_unready_tests": true, "allow_unready_requirements": true}, 200)
	pid := a.project("RAT")
	base := "/api/v1/projects/" + pid
	a.m("PUT", base+"/web-tests", map[string]any{"base_url": "https://default.example.com",
		"environment_urls": []map[string]any{{"environment": "staging", "base_url": "https://staging.example.com"}}}, 200)
	ff := a.m("POST", base+"/configurations", map[string]any{"values": []map[string]any{{"dimension": "Browser", "value": "Firefox"}}}, 201)["id"].(string)
	wk := a.m("POST", base+"/configurations", map[string]any{"values": []map[string]any{{"dimension": "Browser", "value": "Safari"}}}, 201)["id"].(string)
	login := webTest(t, a, pid, "Login", "await page.goto('/login')")
	broken := webTest(t, a, pid, "Broken", "// FAIL")
	after := webTest(t, a, pid, "After login", "await page.goto('/home')")
	afterBroken := webTest(t, a, pid, "After broken", "await page.goto('/x')")
	shell := a.m("POST", base+"/tests", map[string]any{"title": "Shell", "execution_type": "automated", "script_language": "sh",
		"script": "echo shell $QTF_BASE_URL", "status": "ready"}, 201)["id"].(string)
	manual := a.m("POST", base+"/tests", map[string]any{"title": "Manual", "status": "ready"}, 201)["id"].(string)
	noScript := a.m("POST", base+"/tests", map[string]any{"title": "Empty", "execution_type": "automated", "status": "ready"}, 201)["id"].(string)
	a.m("POST", "/api/v1/tests/"+after+"/dependencies", map[string]any{"depends_on_id": login}, 201)
	a.m("POST", "/api/v1/tests/"+afterBroken+"/dependencies", map[string]any{"depends_on_id": broken}, 201)
	run := a.m("POST", base+"/runs", map[string]any{"name": "All", "environment": "Staging", "configuration_ids": []string{ff, wk},
		"test_ids": []string{after, afterBroken, login, broken, shell, manual, noScript}}, 201)
	rid := run["id"].(string)

	res := a.m("POST", "/api/v1/runs/"+rid+"/run-automated", nil, 202)
	queued := res["queued"].([]any)
	skipped := map[string]int{}
	for _, s := range res["skipped"].([]any) {
		skipped[s.(map[string]any)["code"].(string)]++
	}
	if len(queued) != 10 || skipped["not_automated"] != 0 || skipped["no_script"] != 2 {
		t.Fatalf("queued %d, skipped %v", len(queued), res["skipped"])
	}
	// prerequisites first; their dependents wait for them
	pos := map[string]int{}
	jobOf := map[string]map[string]any{}
	for i, q := range queued {
		j := q.(map[string]any)
		k := j["test_title"].(string) + "/" + j["configuration_name"].(string)
		pos[k] = i
		jobOf[k] = j
	}
	if pos["Login/Firefox"] > pos["After login/Firefox"] || len(jobOf["After login/Firefox"]["after"].([]any)) != 1 ||
		jobOf["After login/Firefox"]["after"].([]any)[0] != jobOf["Login/Firefox"]["id"] {
		t.Fatalf("order %v", pos)
	}
	jobs := a.m("GET", "/api/v1/runs/"+rid+"/jobs", nil, 200)
	if c := jobs["counts"].(map[string]any); c["queued"] != float64(10) || jobs["active"] != true {
		t.Fatal(jobs)
	}
	for _, x := range execsOf(t, a, rid) {
		if x["test_title"] == "Login" && x["status"] != "in_progress" {
			t.Fatal(x)
		}
	}
	// a second request queues nothing new
	if r := a.m("POST", "/api/v1/runs/"+rid+"/run-automated", nil, 202); len(r["queued"].([]any)) != 0 {
		t.Fatal(r)
	}
	// cancel the shell jobs (queued): their executions are not run again
	var shellJobs []string
	for k, j := range jobOf {
		if strings.HasPrefix(k, "Shell/") {
			shellJobs = append(shellJobs, j["id"].(string))
		}
	}
	if c := a.m("POST", "/api/v1/runs/"+rid+"/jobs/cancel", map[string]any{"job_ids": shellJobs}, 200); c["cancelled"] != float64(2) {
		t.Fatal(c)
	}
	e.startJobs()
	e.waitJobs()
	jobs = a.m("GET", "/api/v1/runs/"+rid+"/jobs", nil, 200)
	c := jobs["counts"].(map[string]any)
	if c["done"] != float64(10) || c["passed"] != float64(4) || c["failed"] != float64(2) || c["skipped"] != float64(2) || c["cancelled"] != float64(2) || jobs["active"] != false {
		t.Fatalf("counts %v", c)
	}
	results := map[string]string{}
	outputs := map[string]string{}
	for _, x := range execsOf(t, a, rid) {
		k := x["test_title"].(string) + "/" + x["configuration_name"].(string)
		results[k] = x["status"].(string)
		outputs[k] = x["output"].(string)
	}
	if results["Login/Firefox"] != "passed" || results["After login/Safari"] != "passed" || results["Broken/Firefox"] != "failed" ||
		results["After broken/Firefox"] != "not_run" || results["Shell/Firefox"] != "not_run" || results["Manual/Firefox"] != "not_run" {
		t.Fatalf("results %v", results)
	}
	if !strings.Contains(outputs["Login/Firefox"], "base=https://staging.example.com browser=firefox") || !strings.Contains(outputs["Login/Safari"], "browser=webkit") ||
		!strings.Contains(outputs["Login/Firefox"], "Browser Firefox runs in firefox") {
		t.Fatalf("output %q", outputs["Login/Firefox"])
	}
	for _, j := range items(jobs) {
		if j["test_title"] == "After broken" && (j["state"] != "skipped" || !strings.Contains(j["message"].(string), "not executed")) {
			t.Fatal(j)
		}
	}
	// a selection: only the shell script, run as a job
	var shellExec string
	for _, x := range execsOf(t, a, rid) {
		if x["test_title"] == "Shell" && x["configuration_name"] == "Firefox" {
			shellExec = x["id"].(string)
		}
	}
	r := a.m("POST", "/api/v1/runs/"+rid+"/run-automated", map[string]any{"execution_ids": []string{shellExec}}, 202)
	if len(r["queued"].([]any)) != 1 {
		t.Fatal(r)
	}
	e.waitJobs()
	if d := a.m("GET", "/api/v1/executions/"+shellExec, nil, 200); d["status"] != "passed" || !strings.Contains(d["output"].(string), "shell https://staging.example.com") {
		t.Fatal(d)
	}
	a.m("POST", "/api/v1/runs/"+rid+"/run-automated", map[string]any{"execution_ids": []string{"missing"}}, 400)
	a.m("POST", "/api/v1/runs/missing/run-automated", nil, 404)
	a.m("GET", "/api/v1/runs/missing/jobs", nil, 404)
	a.m("POST", "/api/v1/runs/missing/jobs/cancel", nil, 404)
	e.srv.Runner.Cfg.Enabled = false
	a.m("POST", "/api/v1/runs/"+rid+"/run-automated", nil, 409)
}

func TestCancelRunningJob(t *testing.T) {
	e := webEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("CAN")
	slow := webTest(t, a, pid, "Slow", "// SLEEP")
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R", "test_ids": []string{slow}}, 201)
	exID := execsOf(t, a, run["id"].(string))[0]["id"].(string)
	a.m("POST", "/api/v1/executions/"+exID+"/run-script/cancel", nil, 409)
	a.m("PATCH", "/api/v1/executions/"+exID, map[string]any{"status": "failed"}, 200)
	a.m("POST", "/api/v1/executions/"+exID+"/run-script", nil, 202)
	e.startJobs()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if j := a.m("GET", "/api/v1/executions/"+exID, nil, 200)["job"].(map[string]any); j["state"] == "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job never started")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// the synchronous run-script is refused while the job runs
	if c := a.m("POST", "/api/v1/executions/"+exID+"/run-script/cancel", nil, 200); c["stopping"] != float64(1) {
		t.Fatal(c)
	}
	e.waitJobs()
	d := a.m("GET", "/api/v1/executions/"+exID, nil, 200)
	if d["status"] != "failed" || d["job"].(map[string]any)["state"] != "cancelled" || !strings.Contains(d["output"].(string), "cancelled while running") {
		t.Fatal(d)
	}
	a.m("POST", "/api/v1/executions/missing/run-script/cancel", nil, 404)
}

func TestJobsHandledByHand(t *testing.T) {
	e := webEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("HND")
	id := webTest(t, a, pid, "Login", "await page.goto('/')")
	sh := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Sh", "execution_type": "automated", "script_language": "sh", "script": "echo hi"}, 201)["id"].(string)
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R", "test_ids": []string{id, sh}}, 201)
	var webEx, shEx string
	for _, x := range execsOf(t, a, run["id"].(string)) {
		if x["test_case_id"] == id {
			webEx = x["id"].(string)
		} else {
			shEx = x["id"].(string)
		}
	}
	a.m("POST", "/api/v1/runs/"+run["id"].(string)+"/run-automated", nil, 202)
	// queued twice is refused
	a.m("POST", "/api/v1/executions/"+webEx+"/run-script", nil, 409)
	// a result recorded by hand while queued wins
	a.m("PATCH", "/api/v1/executions/"+webEx, map[string]any{"status": "blocked"}, 200)
	// synchronous run-script of a script that is queued as a job is refused
	a.m("POST", "/api/v1/executions/"+shEx+"/run-script", nil, 409)
	e.startJobs()
	e.waitJobs()
	d := a.m("GET", "/api/v1/executions/"+webEx, nil, 200)
	if d["status"] != "blocked" || d["job"].(map[string]any)["state"] != "cancelled" {
		t.Fatal(d)
	}
	// cancel without ids: nothing queued is left
	if c := a.m("POST", "/api/v1/runs/"+run["id"].(string)+"/jobs/cancel", map[string]any{"running": true}, 200); c["cancelled"] != float64(0) {
		t.Fatal(c)
	}
}

func TestRecoverInterruptedJobs(t *testing.T) {
	e := webEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("REC")
	id := webTest(t, a, pid, "Login", "await page.goto('/')")
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R", "test_ids": []string{id}}, 201)
	exID := execsOf(t, a, run["id"].(string))[0]["id"].(string)
	a.m("PATCH", "/api/v1/executions/"+exID, map[string]any{"status": "passed"}, 200)
	before := a.m("GET", "/api/v1/executions/"+exID, nil, 200)
	a.m("POST", "/api/v1/runs/"+run["id"].(string)+"/run-automated", map[string]any{"include_passed": true}, 202)
	if d := a.m("GET", "/api/v1/executions/"+exID, nil, 200); d["status"] != "in_progress" {
		t.Fatal(d)
	}
	// "the server restarts": another server process (instance) starts on the database
	newSrv := func() *Server {
		s, err := New(e.st, e.cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), runner.New(e.cfg.Runner), nil)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	srv2 := newSrv()
	// SQLite resets every unfinished job at start-up, PostgreSQL the stale ones
	srv2.JobHeartbeat = time.Millisecond
	time.Sleep(20 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv2.StartScriptJobs(ctx)
	srv2.StartScriptJobs(ctx) // a second start is a no-op
	cancel()
	srv2.WaitJobsStopped(time.Second)
	ctx = context.Background()
	d := a.m("GET", "/api/v1/executions/"+exID, nil, 200)
	if d["status"] != "passed" || d["finished_at"] != before["finished_at"] || d["job"].(map[string]any)["state"] != "interrupted" ||
		!strings.Contains(d["output"].(string), "the server stopped while this automated test was queued") {
		t.Fatal(d)
	}
	if r, _ := e.st.GetRun(context.Background(), e.tenant.ID, run["id"].(string)); r.Status != "completed" {
		t.Errorf("run %s", r.Status)
	}

	// with several instances only stale jobs are reset
	a.m("POST", "/api/v1/runs/"+run["id"].(string)+"/run-automated", map[string]any{"include_passed": true}, 202)
	srv3 := newSrv()
	srv3.JobHeartbeat = time.Hour
	srv3.recoverJobs(ctx, false)
	if d := a.m("GET", "/api/v1/executions/"+exID, nil, 200); d["status"] != "in_progress" {
		t.Fatal("a fresh job was reset")
	}
	srv3.JobHeartbeat = time.Millisecond
	time.Sleep(20 * time.Millisecond)
	srv3.recoverJobs(ctx, false)
	if d := a.m("GET", "/api/v1/executions/"+exID, nil, 200); d["status"] != "passed" || !strings.Contains(d["output"].(string), "queued") {
		t.Fatal(d)
	}
	if err := e.st.HeartbeatJobs(ctx, "another instance"); err != nil {
		t.Fatal(err)
	}
	if srv3.heartbeat() != time.Millisecond || (&Server{}).heartbeat() != jobHeartbeat || (&Server{}).maxParallel() != 2 {
		t.Error("defaults")
	}
}

func TestScriptVariablesMasked(t *testing.T) {
	e := webEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("VAR")
	a.m("PUT", "/api/v1/projects/"+pid+"/web-tests", map[string]any{"base_url": "https://x.example.com",
		"variables": []map[string]any{{"name": "API_KEY", "value": "k-123456", "secret": true}, {"name": "USER", "value": "carol"}}}, 200)
	sh := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Sh", "execution_type": "automated", "script_language": "sh",
		"script": "echo key=$QTF_VAR_API_KEY user=$QTF_VAR_USER url=$QTF_BASE_URL"}, 201)["id"].(string)
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R", "test_ids": []string{sh}}, 201)
	exID := execsOf(t, a, run["id"].(string))[0]["id"].(string)
	d := a.m("POST", "/api/v1/executions/"+exID+"/run-script", nil, 200)
	if out := d["output"].(string); !strings.Contains(out, "key=*** user=carol url=https://x.example.com") {
		t.Fatal(out)
	}
}

func TestCurrentJobs(t *testing.T) {
	now := time.Now()
	list := []model.ScriptJob{
		{ID: "a", BatchID: "new", State: model.JobPassed, CreatedAt: now, Seq: 1},
		{ID: "b", BatchID: "new", State: model.JobError, CreatedAt: now, Seq: 0},
		{ID: "c", BatchID: "old", State: model.JobFailed, CreatedAt: now.Add(-time.Hour)},
	}
	cur, c := currentJobs(list)
	if len(cur) != 2 || cur[0].ID != "b" || c.Error != 1 || c.Passed != 1 || c.Total != 2 {
		t.Errorf("%v %+v", cur, c)
	}
	if cur, c := currentJobs(nil); cur != nil || c.Total != 0 {
		t.Error("empty")
	}
}
