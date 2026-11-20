package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/uploader"
)

// postXML sends body with the given content type and decodes the answer.
func (c *client) postXML(path, ctype string, body []byte, want int) map[string]any {
	c.e.t.Helper()
	req, _ := http.NewRequest("POST", c.e.hs.URL+path, bytes.NewReader(body))
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		c.e.t.Fatalf("POST %s: status %d, want %d: %s", path, res.StatusCode, want, data)
	}
	out := map[string]any{}
	_ = json.Unmarshal(data, &out)
	return out
}

// multiXML builds a multipart body with one "file" part per document.
func multiXML(t *testing.T, docs map[string]string, extra map[string]string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range extra {
		_ = mw.WriteField(k, v)
	}
	for name, doc := range docs {
		w, _ := mw.CreateFormFile("file", name)
		_, _ = w.Write([]byte(doc))
	}
	_ = mw.Close()
	return buf.Bytes(), mw.FormDataContentType()
}

func strs2(v any) []string {
	var out []string
	if l, ok := v.([]any); ok {
		for _, x := range l {
			out = append(out, x.(string))
		}
	}
	return out
}

// junitSetup creates project JU with tests JU-TC-1..4 and a run holding
// the first two.
func junitSetup(t *testing.T, a *client) (pid, runID string, tests []map[string]any) {
	t.Helper()
	pid = a.project("JU")
	for _, title := range []string{"Pay by card", "Refund", "Login", "Logout"} {
		tests = append(tests, a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": title, "execution_type": "automated"}, 201))
	}
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "CI", "test_ids": []string{tests[0]["id"].(string), tests[1]["id"].(string)}}, 201)
	return pid, run["id"].(string), tests
}

func executionsByKey(a *client, runID string) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, x := range items(a.m("GET", "/api/v1/runs/"+runID+"/executions", nil, 200)) {
		out[x["test_key"].(string)] = x
	}
	return out
}

const junitSample = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
 <testsuite name="chromium">
  <testcase name="checkout › pays by card @JU-TC-1" classname="checkout.spec.ts" time="1.5"/>
  <testcase name="checkout › refund" classname="checkout.spec.ts" time="0.2">
   <properties><property name="qtestframe" value="JU-TC-2"/></properties>
   <skipped/>
  </testcase>
  <testcase name="login works" classname="tests.JU_TC_3_Login" time="0.1"><system-out>logged in</system-out></testcase>
  <testcase name="ghost JU-TC-99" time="0"/>
  <testcase name="checkout › guest" classname="checkout.spec.ts" time="0.3"><failure message="timeout">Timed out 5000ms</failure></testcase>
 </testsuite>
 <testsuite name="firefox">
  <testcase name="checkout › pays by card @JU-TC-1" classname="checkout.spec.ts" time="2">
   <failure message="expected 200, got 500" type="Error">at checkout.spec.ts:12</failure>
  </testcase>
 </testsuite>
</testsuites>`

func TestJUnitImport(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	_, runID, _ := junitSetup(t, a)
	path := "/api/v1/runs/" + runID + "/junit"

	res := a.postXML(path, "application/xml", []byte(junitSample), 200)
	if res["updated"].(float64) != 3 || strings.Join(strs2(res["unknown"]), ",") != "JU-TC-99" ||
		strings.Join(strs2(res["unmapped"]), ",") != "checkout › guest" || len(res["rejected"].([]any)) != 0 || len(res["created"].([]any)) != 0 {
		t.Fatalf("import: %v", res)
	}
	ex := executionsByKey(a, runID)
	if len(ex) != 3 {
		t.Fatalf("executions: %v", ex)
	}
	tc1 := ex["JU-TC-1"]
	if tc1["status"] != "failed" || tc1["duration_ms"].(float64) != 3500 || !strings.Contains(tc1["output"].(string), "Error: expected 200, got 500") ||
		!strings.Contains(tc1["output"].(string), "== checkout › pays by card @JU-TC-1") {
		t.Errorf("TC-1: %v", tc1)
	}
	if ex["JU-TC-2"]["status"] != "skipped" || ex["JU-TC-3"]["status"] != "passed" || ex["JU-TC-3"]["output"] != "[system-out]\nlogged in" {
		t.Errorf("TC-2/3: %v %v", ex["JU-TC-2"], ex["JU-TC-3"])
	}
	// every execution has a final result: the run completed by itself
	if run := a.m("GET", "/api/v1/runs/"+runID, nil, 200); run["status"] != "completed" || run["completed_at"] == nil {
		t.Errorf("run: %v", run)
	}

	// multipart with several files; results of one key combine across files
	body, ct := multiXML(t, map[string]string{
		"a.xml": `<testsuite><testcase name="JU-TC-4 a" time="1"/></testsuite>`,
		"b.xml": `<testsuite><testcase name="JU-TC-4 b" time="1"><error message="crash"/></testcase></testsuite>`,
	}, map[string]string{"note": "ignored"})
	res = a.postXML(path, ct, body, 200)
	if res["updated"].(float64) != 1 || len(strs2(res["unmapped"])) != 0 {
		t.Fatal(res)
	}
	if tc4 := executionsByKey(a, runID)["JU-TC-4"]; tc4["status"] != "failed" || tc4["duration_ms"].(float64) != 2000 {
		t.Errorf("TC-4: %v", tc4)
	}

	// errors
	a.postXML(path, "application/xml", []byte("<testsuites><testcase>"), 400)
	a.postXML(path, "text/xml", []byte("  "), 400)
	if r := a.postXML(path, "application/xml", []byte(`{"results":[]}`), 400); !strings.Contains(r["error"].(string), "invalid JUnit XML") {
		t.Error(r)
	}
	body, ct = multiXML(t, map[string]string{"broken.xml": "<nope/>"}, nil)
	if r := a.postXML(path, ct, body, 400); !strings.Contains(r["error"].(string), "broken.xml: invalid JUnit XML") {
		t.Error(r)
	}
	body, ct = multiXML(t, nil, map[string]string{"other": "x"})
	a.postXML(path, ct, body, 400)
	a.postXML(path, "multipart/form-data", []byte("x"), 400)
	a.postXML(path, "multipart/form-data; boundary=zz", []byte("--zz\r\nbroken"), 400)
	big := append([]byte("<testsuite>"), bytes.Repeat([]byte(" "), MaxJUnitBytes)...)
	if r := a.postXML(path, "application/xml", big, 400); !strings.Contains(r["error"].(string), "too large") {
		t.Error(r)
	}
	body, ct = multiXML(t, map[string]string{"big.xml": string(big)}, nil)
	a.postXML(path, ct, body, 400)
	a.postXML("/api/v1/runs/missing/junit", "application/xml", []byte(junitSample), 404)

	// permissions: run.execute
	e.member("viewer@example.com", rbac.RoleViewer)
	e.login("viewer@example.com").postXML(path, "application/xml", []byte(junitSample), 403)
}

func TestJUnitCreateMissing(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid, runID, _ := junitSetup(t, a)
	path := "/api/v1/runs/" + runID + "/junit?create_missing=true"
	doc := `<testsuites>
	<testsuite name="chromium"><testcase name="checkout   › guest" time="0.5"/><testcase name="checkout › wishlist" time="1"><failure/></testcase><testcase name="JU-TC-1 card"/></testsuite>
	<testsuite name="firefox"><testcase name="checkout › guest" time="0.5"/><testcase classname="only.Class" time="0"/></testsuite></testsuites>`
	res := a.postXML(path, "application/xml", []byte(doc), 200)
	created := res["created"].([]any)
	if len(created) != 3 || res["updated"].(float64) != 4 || len(strs2(res["unmapped"])) != 0 {
		t.Fatalf("create missing: %v", res)
	}
	first := created[0].(map[string]any)
	if first["key"] != "JU-TC-5" || first["title"] != "checkout › guest" || created[2].(map[string]any)["title"] != "only.Class" {
		t.Errorf("created: %v", created)
	}
	tc := a.m("GET", "/api/v1/tests/"+first["id"].(string), nil, 200)
	if tc["execution_type"] != "automated" || tc["status"] != "draft" || !strings.Contains(tc["description"].(string), "run CI") {
		t.Errorf("test: %v", tc)
	}
	ex := executionsByKey(a, runID)
	if ex["JU-TC-5"]["status"] != "passed" || ex["JU-TC-5"]["duration_ms"].(float64) != 1000 || ex["JU-TC-6"]["status"] != "failed" {
		t.Errorf("executions: %v", ex)
	}
	// a second import finds the tests by title instead of creating them again
	res = a.postXML(path, "application/xml", []byte(doc), 200)
	if len(res["created"].([]any)) != 0 || res["updated"].(float64) != 4 {
		t.Fatalf("again: %v", res)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/tests", nil, 200)); len(l) != 7 {
		t.Errorf("%d tests", len(l))
	}
	// a long name is cut to a valid title
	long := strings.Repeat("ä", 400)
	res = a.postXML(path, "application/xml", []byte(`<testsuite><testcase name="`+long+`"/></testsuite>`), 200)
	if title := res["created"].([]any)[0].(map[string]any)["title"].(string); len(title) > 500 || !strings.HasSuffix(title, "…") {
		t.Errorf("title %q", title)
	}
	if testTitle("  ") != "Unnamed test case" {
		t.Error(testTitle(""))
	}

	// create_missing needs test.write in addition to run.execute
	a.m("POST", "/api/v1/roles", map[string]any{"name": "ci", "permissions": []string{"project.read", "test.read", "run.read", "run.execute"}}, 201)
	a.m("POST", "/api/v1/members", map[string]any{"email": "ci@example.com", "role": "ci", "password": testPassword}, 201)
	ci := e.login("ci@example.com")
	ci.postXML(path, "application/xml", []byte(doc), 403)
	if r := ci.postXML("/api/v1/runs/"+runID+"/junit?create_missing=0", "application/xml", []byte(`<testsuite><testcase name="new one"/></testsuite>`), 200); strings.Join(strs2(r["unmapped"]), ",") != "new one" {
		t.Error(r)
	}
}

func TestJUnitReadiness(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	strictPolicy(t, a)
	_, runID, tests := junitSetup(t, a)
	a.m("PATCH", "/api/v1/tests/"+tests[0]["id"].(string), map[string]any{"status": "ready"}, 200)
	doc := `<testsuite><testcase name="JU-TC-1"/><testcase name="JU-TC-2"/><testcase name="JU-TC-3"><skipped/></testcase></testsuite>`
	res := a.postXML("/api/v1/runs/"+runID+"/junit", "application/xml", []byte(doc), 200)
	rej := res["rejected"].([]any)
	if res["updated"].(float64) != 2 || len(rej) != 1 {
		t.Fatalf("%v", res)
	}
	r := rej[0].(map[string]any)
	if r["test_key"] != "JU-TC-2" || !strings.Contains(r["reason"].(string), `the test JU-TC-2 has status "draft"`) || r["readiness"] == nil {
		t.Errorf("rejected: %v", r)
	}
	ex := executionsByKey(a, runID)
	if ex["JU-TC-1"]["status"] != "passed" || ex["JU-TC-2"]["status"] != "not_run" || ex["JU-TC-3"]["status"] != "skipped" {
		t.Errorf("%v", ex)
	}
	// the JSON import reports the reason as well
	res = a.m("POST", "/api/v1/runs/"+runID+"/results", map[string]any{"results": []map[string]any{{"test_key": "JU-TC-2", "status": "failed"}}}, 200)
	if rj := res["rejected"].([]any); len(rj) != 1 || !strings.Contains(rj[0].(map[string]any)["reason"].(string), "draft") {
		t.Error(res)
	}
	if run := a.m("GET", "/api/v1/runs/"+runID, nil, 200); run["status"] != "in_progress" {
		t.Errorf("run: %v", run["status"])
	}
}

// TestReportCommandEndToEnd runs the uploader behind `qtestframe report`
// against the real API with an API token.
func TestReportCommandEndToEnd(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid, _, _ := junitSetup(t, a)
	e.member("tester@example.com", rbac.RoleTester)
	tok := e.login("tester@example.com").m("POST", "/api/v1/tokens", map[string]any{"name": "ci"}, 201)["token"].(string)

	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("shard-1.xml", `<testsuite><testcase name="JU-TC-1 card" time="1"/><testcase name="new flow" time="1"/></testsuite>`)
	write("shard-2.xml", `<testsuite><testcase name="JU-TC-1 card" time="1"><failure message="boom"/></testcase><testcase name="JU-TC-77"/></testsuite>`)
	js := write("manual.json", `{"results":[{"test_key":"JU-TC-4","status":"passed","duration_ms":5}]}`)

	var log bytes.Buffer
	o := uploader.Options{URL: e.hs.URL, Token: tok, Project: "ju", Run: "Nightly 42", CreateRun: true, Build: "42", Environment: "ci",
		JUnit: []string{filepath.Join(dir, "shard-*.xml")}, JSON: []string{js}, CreateMissing: true, Log: &log}
	sum, err := uploader.Report(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Updated != 3 || strings.Join(sum.Unknown, ",") != "JU-TC-77" || len(sum.Created) != 1 || sum.Created[0].Title != "new flow" {
		t.Fatalf("summary: %+v", sum)
	}
	runs := items(a.m("GET", "/api/v1/projects/"+pid+"/runs", nil, 200))
	var run map[string]any
	for _, r := range runs {
		if r["name"] == "Nightly 42" {
			run = r
		}
	}
	if run == nil || run["build"] != "42" || run["environment"] != "ci" || run["status"] != "completed" || run["id"] != sum.RunID {
		t.Fatalf("run: %v", runs)
	}
	if ex := executionsByKey(a, sum.RunID); ex["JU-TC-1"]["status"] != "failed" || ex["JU-TC-4"]["status"] != "passed" || len(ex) != 3 {
		t.Errorf("executions: %v", ex)
	}

	// the second report finds the run; unknown keys fail with --fail-on-unknown
	o.CreateMissing, o.FailOnUnknown, o.JSON = false, true, nil
	sum, err = uploader.Report(context.Background(), o)
	if err == nil || sum == nil || sum.RunID != run["id"] {
		t.Fatalf("%v %+v", err, sum)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/runs", nil, 200)); len(l) != 2 {
		t.Errorf("%d runs", len(l))
	}
}
