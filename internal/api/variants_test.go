package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

// variantFixture: project VAR with variants EU (attributes region=EU),
// EU model DE below it and US; R1 applies to all, R2 to EU (and so to DE),
// R3 to the expression region:US; tests T1 (all) and T2 (US only) verify
// R1, T3 (EU) verifies R2.
type variantFixture struct {
	pid, eu, de, us string
	r1, r2, r3      string
	t1, t2, t3      string
	k               map[string]string
}

func newVariantFixture(t *testing.T, a *client) *variantFixture {
	t.Helper()
	f := &variantFixture{k: map[string]string{}}
	f.pid = a.project("VAR")
	pp := "/api/v1/projects/" + f.pid
	f.eu = a.m("POST", pp+"/variants", map[string]any{"name": "EU", "kind": "market", "attributes": map[string]string{"region": "EU", "voltage": "230V"}}, 201)["id"].(string)
	de := a.m("POST", pp+"/variants", map[string]any{"name": "EU model DE", "parent_id": f.eu, "attributes": map[string]string{"lang": "de"}}, 201)
	f.de = de["id"].(string)
	if de["path"] != "EU / EU model DE" {
		t.Fatal(de)
	}
	f.us = a.m("POST", pp+"/variants", map[string]any{"name": "US", "kind": "market", "attributes": map[string]string{"region": "US", "voltage": "110V"}}, 201)["id"].(string)
	mk := func(kind, title string) string {
		m := a.m("POST", pp+"/"+kind, map[string]any{"title": title}, 201)
		f.k[m["id"].(string)] = m["key"].(string)
		return m["id"].(string)
	}
	f.r1, f.r2, f.r3 = mk("requirements", "Power on"), mk("requirements", "CE mark"), mk("requirements", "UL listing")
	f.t1, f.t2, f.t3 = mk("tests", "Boot"), mk("tests", "Boot at 110V"), mk("tests", "CE check")
	for _, l := range [][2]string{{f.r1, f.t1}, {f.r1, f.t2}, {f.r2, f.t3}} {
		a.m("POST", pp+"/links", map[string]any{"requirement_id": l[0], "test_case_id": l[1]}, 201)
	}
	a.m("PUT", "/api/v1/requirements/"+f.r2+"/applicability", map[string]any{"variant_ids": []string{"EU"}}, 200)
	a.m("PUT", "/api/v1/requirements/"+f.r3+"/applicability", map[string]any{"expression": "region:US"}, 200)
	a.m("PUT", pp+"/applicability", map[string]any{"item_type": "test", "item_ids": []string{f.t2}, "variant_ids": []string{f.us}}, 200)
	a.m("PUT", "/api/v1/tests/"+f.t3+"/applicability", map[string]any{"variant_ids": []string{f.eu}}, 200)
	return f
}

func keysOf(list []map[string]any) []string {
	var out []string
	for _, m := range list {
		out = append(out, m["key"].(string))
	}
	return out
}

func TestVariantsAndApplicability(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	f := newVariantFixture(t, a)
	pp := "/api/v1/projects/" + f.pid

	// variants: list in hierarchy order, validation, cycles, names
	vs := items(a.m("GET", pp+"/variants", nil, 200))
	if len(vs) != 3 || vs[0]["name"] != "EU" || vs[1]["name"] != "EU model DE" || vs[2]["name"] != "US" {
		t.Fatal(vs)
	}
	a.m("POST", pp+"/variants", map[string]any{"name": "eu"}, 409)
	a.m("POST", pp+"/variants", map[string]any{"name": ""}, 400)
	a.m("POST", pp+"/variants", map[string]any{"name": "X", "kind": "planet"}, 400)
	a.m("POST", pp+"/variants", map[string]any{"name": "X", "attributes": map[string]string{"name": "x"}}, 400)
	a.m("POST", pp+"/variants", map[string]any{"name": "X", "configuration_ids": []string{"nope"}}, 400)
	a.m("PATCH", "/api/v1/variants/"+f.eu, map[string]any{"parent_id": f.de}, 400)
	a.m("PATCH", "/api/v1/variants/"+f.eu, map[string]any{"description": "European Union"}, 200)
	if g := a.m("GET", "/api/v1/variants/"+f.de, nil, 200); g["path"] != "EU / EU model DE" {
		t.Fatal(g)
	}

	// applicability of one item
	ap := a.m("GET", "/api/v1/requirements/"+f.r2+"/applicability", nil, 200)
	if ap["all"] != false || len(ap["applies_to"].([]any)) != 2 {
		t.Fatal(ap)
	}
	ap = a.m("GET", "/api/v1/requirements/"+f.r3+"/applicability", nil, 200)
	if to := ap["applies_to"].([]any); len(to) != 1 || to[0].(map[string]any)["name"] != "US" {
		t.Fatal(ap)
	}
	if ap := a.m("GET", "/api/v1/tests/"+f.t1+"/applicability", nil, 200); ap["all"] != true || len(ap["applies_to"].([]any)) != 3 {
		t.Fatal(ap)
	}
	a.m("PUT", "/api/v1/requirements/"+f.r1+"/applicability", map[string]any{"variant_ids": []string{"Mars"}}, 400)
	a.m("PUT", "/api/v1/requirements/"+f.r1+"/applicability", map[string]any{"expression": "(region:"}, 400)
	a.m("PUT", pp+"/applicability", map[string]any{"item_type": "requirement", "item_ids": []string{f.t1}}, 400)
	a.m("PUT", pp+"/applicability", map[string]any{"item_type": "requirement", "item_ids": []string{}}, 400)

	// lists for a variant, the query fields variant and applicability
	reqs := items(a.m("GET", pp+"/requirements?variant=EU+model+DE", nil, 200))
	if got := strings.Join(keysOf(reqs), ","); got != f.k[f.r1]+","+f.k[f.r2] {
		t.Fatal(got)
	}
	if reqs[0]["applicability"] != nil || reqs[1]["applicability"].(map[string]any)["variants"].([]any)[0].(map[string]any)["name"] != "EU" {
		t.Fatal(reqs)
	}
	// R1 is verified in DE by T1 only (T2 is for the US)
	if reqs[0]["linked_tests"] != float64(1) {
		t.Fatal(reqs[0])
	}
	if got := keysOf(items(a.m("GET", pp+"/requirements?q="+url.QueryEscape(`variant:"US"`), nil, 200))); strings.Join(got, ",") != f.k[f.r1]+","+f.k[f.r3] {
		t.Fatal(got)
	}
	if got := keysOf(items(a.m("GET", pp+"/requirements?q=applicability:selected", nil, 200))); len(got) != 2 {
		t.Fatal(got)
	}
	if got := keysOf(items(a.m("GET", pp+"/tests?variant=US", nil, 200))); strings.Join(got, ",") != f.k[f.t1]+","+f.k[f.t2] {
		t.Fatal(got)
	}
	a.m("GET", pp+"/requirements?variant=Mars", nil, 400)

	// results: a variant-neutral run passes T1; a US run fails T2; an EU run passes T3
	runOf := func(name, variant string, tests ...string) string {
		body := map[string]any{"name": name, "test_ids": tests}
		if variant != "" {
			body["variant_id"] = variant
		}
		run := a.m("POST", pp+"/runs", body, 201)
		if variant != "" && run["variant_name"] == "" {
			t.Fatal(run)
		}
		return run["id"].(string)
	}
	result := func(run, key, status string) {
		a.m("POST", "/api/v1/runs/"+run+"/results", map[string]any{"results": []map[string]any{{"test_key": key, "status": status}}}, 200)
	}
	neutral := runOf("Smoke", "", f.t1)
	result(neutral, f.k[f.t1], "passed")
	us := runOf("US run", "US", f.t2)
	result(us, f.k[f.t2], "failed")
	eu := runOf("EU run", f.eu, f.t3)
	result(eu, f.k[f.t3], "passed")

	// matrix
	mx := a.m("GET", pp+"/variants/matrix", nil, 200)
	cov := map[string]map[string]any{}
	for _, r := range mx["requirements"].([]any) {
		m := r.(map[string]any)
		cov[m["id"].(string)] = m["coverage"].(map[string]any)
	}
	for _, c := range []struct{ req, variant, want string }{
		{f.r1, f.eu, "passed"}, {f.r1, f.de, "passed"}, {f.r1, f.us, "failed"},
		{f.r2, f.eu, "passed"}, {f.r2, f.de, "passed"}, {f.r2, f.us, "not_applicable"},
		{f.r3, f.us, "none"}, {f.r3, f.eu, "not_applicable"},
	} {
		if got := cov[c.req][c.variant]; got != c.want {
			t.Errorf("%s in %s: %v, want %s", f.k[c.req], c.variant, got, c.want)
		}
	}
	sum := mx["summary"].(map[string]any)[f.us].(map[string]any)
	if sum["applicable"] != float64(2) || sum["failed"] != float64(1) {
		t.Fatal(sum)
	}
	a.m("GET", pp+"/variants/matrix?variants=US&configuration=none", nil, 200)
	a.m("GET", pp+"/variants/matrix?variants=Mars", nil, 400)
	a.m("GET", pp+"/variants/matrix?configuration=nope", nil, 400)

	// the coverage of a requirement per variant × configuration
	rv := a.m("GET", "/api/v1/requirements/"+f.r1+"/variants", nil, 200)
	rows := rv["variants"].([]any)
	if len(rows) != 3 || rows[2].(map[string]any)["coverage"] != "failed" || len(rows[2].(map[string]any)["configurations"].([]any)) != 1 {
		t.Fatal(rows)
	}
	// the US run's result does not count for the EU: the test list for a variant
	if tl := items(a.m("GET", pp+"/tests?variant=EU", nil, 200)); len(tl) != 2 || tl[0]["last_result"] != "passed" {
		t.Fatal(tl)
	}
	tr := a.m("GET", pp+"/traceability?variant=US&configuration=none", nil, 200)
	if tr["variant"] != f.us || len(tr["requirements"].([]any)) != 2 {
		t.Fatal(tr)
	}

	// release readiness for a variant
	rel := a.m("POST", pp+"/releases", map[string]any{"name": "2.0"}, 201)["id"].(string)
	a.m("POST", "/api/v1/releases/"+rel+"/requirements", map[string]any{"add": []string{f.r1, f.r2, f.r3}}, 200)
	for _, run := range []string{neutral, us, eu} {
		a.m("PATCH", "/api/v1/runs/"+run, map[string]any{"release_id": rel}, 200)
	}
	all := a.m("GET", "/api/v1/releases/"+rel+"/status", nil, 200)
	euSt := a.m("GET", "/api/v1/releases/"+rel+"/status?variant=EU", nil, 200)
	if all["summary"].(map[string]any)["requirements"] != float64(3) || euSt["summary"].(map[string]any)["requirements"] != float64(2) {
		t.Fatal(all["summary"], euSt["summary"])
	}
	if euSt["variant"].(map[string]any)["name"] != "EU" || euSt["summary"].(map[string]any)["passed"] != float64(2) || euSt["summary"].(map[string]any)["failed"] != float64(0) {
		t.Fatal(euSt)
	}
	if all["summary"].(map[string]any)["failed"] != float64(1) {
		t.Fatal(all["summary"])
	}
	a.m("GET", "/api/v1/releases/"+rel+"/status?variant=Mars", nil, 400)

	// a document for one variant
	doc := a.m("POST", pp+"/documents/generate", map[string]any{"kind": "requirements_spec", "format": "md", "options": map[string]any{"variant": "US", "fields": []string{"tests", "results"}}}, 201)
	md := a.fileContent(doc["id"].(string))
	if !strings.Contains(md, "UL listing") || strings.Contains(md, "CE mark") || !strings.Contains(md, "US") {
		t.Fatal(md)
	}
	plan := a.m("POST", pp+"/documents/generate", map[string]any{"kind": "test_plan", "format": "md", "options": map[string]any{"variant": f.eu}}, 201)
	if md := a.fileContent(plan["id"].(string)); strings.Contains(md, "Boot at 110V") || !strings.Contains(md, "CE check") {
		t.Fatal(md)
	}
	a.m("POST", pp+"/documents/generate", map[string]any{"kind": "test_plan", "format": "md", "options": map[string]any{"variant": "Mars"}}, 400)

	// a run's variant can be changed and cleared
	if run := a.m("PATCH", "/api/v1/runs/"+us, map[string]any{"variant_id": ""}, 200); run["variant_id"] != nil {
		t.Fatal(run)
	}
	a.m("PATCH", "/api/v1/runs/"+us, map[string]any{"variant_id": "Mars"}, 400)

	// deleting a variant: children move up, lists lose it
	a.m("DELETE", "/api/v1/variants/"+f.eu, nil, 204)
	if g := a.m("GET", "/api/v1/variants/"+f.de, nil, 200); g["parent_id"] != nil {
		t.Fatal(g)
	}
	if ap := a.m("GET", "/api/v1/requirements/"+f.r2+"/applicability", nil, 200); ap["all"] != true {
		t.Fatal(ap)
	}
	if ap := a.m("GET", "/api/v1/tests/"+f.t3+"/applicability", nil, 200); ap["all"] != true {
		t.Fatal(ap)
	}
	a.m("DELETE", "/api/v1/variants/"+f.eu, nil, 404)

	// deleting an item removes its applicability
	a.m("DELETE", "/api/v1/requirements/"+f.r3, nil, 204)
	var n int
	if err := e.st.DB.QueryRow(e.st.DB.Rebind(`SELECT COUNT(*) FROM item_applicability WHERE item_id = ?`), f.r3).Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}

	// permissions: a viewer reads but does not change
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	v.m("GET", pp+"/variants", nil, 200)
	v.m("POST", pp+"/variants", map[string]any{"name": "Y"}, 403)
	v.m("PUT", "/api/v1/requirements/"+f.r1+"/applicability", map[string]any{"variant_ids": []string{}}, 403)
	v.m("PUT", pp+"/applicability", map[string]any{"item_type": "test", "item_ids": []string{f.t1}}, 403)
}

func hookPost(t *testing.T, e *env, path string, headers map[string]string, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("POST", e.hs.URL+path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	out := map[string]any{}
	_ = json.Unmarshal(data, &out)
	return res.StatusCode, out
}

func githubSig(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestCodeLinksBuildsAndRegression(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	f := newVariantFixture(t, a)
	pp := "/api/v1/projects/" + f.pid
	const secret = "a-long-webhook-secret"

	// settings: hook URLs, write-only secret
	cs := a.m("GET", pp+"/code-settings", nil, 200)
	if cs["has_secret"] != false || cs["github_url"] != "http://example.test/api/v1/hooks/github/"+f.pid {
		t.Fatal(cs)
	}
	a.m("PUT", pp+"/code-settings", map[string]any{"secret": "short"}, 400)
	if cs := a.m("PUT", pp+"/code-settings", map[string]any{"secret": secret}, 200); cs["has_secret"] != true || cs["secret"] != nil {
		t.Fatal(cs)
	}
	if cs := a.m("PUT", pp+"/code-settings", map[string]any{}, 200); cs["has_secret"] != true {
		t.Fatal(cs)
	}

	// GitHub push: signature checked, keys linked (full and short keys)
	r1, t3 := f.k[f.r1], f.k[f.t3]
	push := `{"ref":"refs/heads/main","repository":{"full_name":"acme/fw"},"commits":[
		{"id":"aaaaaaa1111111111111111111111111111111111","message":"Fix boot (` + r1 + `)\n\nalso TC-3 and OTHER-REQ-1","timestamp":"2026-11-01T10:00:00Z","url":"https://github.com/acme/fw/commit/aaaaaaa1","author":{"name":"Dev"}},
		{"id":"bbbbbbb2222222222222222222222222222222222","message":"Docs only","timestamp":"2026-11-02T10:00:00Z","url":"https://github.com/acme/fw/commit/bbbbbbb2","author":{"name":"Dev"}}]}`
	hook := "/api/v1/hooks/github/" + f.pid
	if code, _ := hookPost(t, e, hook, map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature-256": githubSig("wrong-secret-xxxxx", push)}, push); code != 401 {
		t.Fatal(code)
	}
	if code, _ := hookPost(t, e, "/api/v1/hooks/github/nope", map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature-256": githubSig(secret, push)}, push); code != 401 {
		t.Fatal(code)
	}
	code, out := hookPost(t, e, hook, map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature-256": githubSig(secret, push)}, push)
	if code != 200 || out["changes"] != float64(2) || out["links"] != float64(2) {
		t.Fatal(code, out)
	}
	// the same push again adds nothing
	if _, out := hookPost(t, e, hook, map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature-256": githubSig(secret, push)}, push); out["links"] != float64(0) {
		t.Fatal(out)
	}
	ping := `{"zen":"hi"}`
	if code, out := hookPost(t, e, hook, map[string]string{"X-GitHub-Event": "ping", "X-Hub-Signature-256": githubSig(secret, ping)}, ping); code != 200 || out["event"] != "ping" {
		t.Fatal(code, out)
	}
	if code, _ := hookPost(t, e, hook, map[string]string{"X-GitHub-Event": "issues", "X-Hub-Signature-256": githubSig(secret, ping)}, ping); code != 202 {
		t.Fatal(code)
	}
	pr := `{"action":"closed","repository":{"full_name":"acme/fw"},"pull_request":{"number":7,"title":"CE marking","body":"Implements ` + f.k[f.r2] + `","html_url":"https://github.com/acme/fw/pull/7","state":"closed","merged":true,"user":{"login":"dev"},"head":{"ref":"feature/x"},"base":{"ref":"main"}}}`
	if _, out := hookPost(t, e, hook, map[string]string{"X-GitHub-Event": "pull_request", "X-Hub-Signature-256": githubSig(secret, pr)}, pr); out["links"] != float64(1) {
		t.Fatal(out)
	}

	// GitLab: secret token, push and merge request
	gl := "/api/v1/hooks/gitlab/" + f.pid
	mr := `{"object_kind":"merge_request","user":{"username":"dev"},"project":{"path_with_namespace":"acme/fw"},"object_attributes":{"iid":3,"title":"Boot at 110V","description":"","url":"https://gitlab.com/acme/fw/-/merge_requests/3","state":"opened","source_branch":"TC-2-boot","target_branch":"main","created_at":"2026-11-03 10:00:00 UTC"}}`
	if code, _ := hookPost(t, e, gl, map[string]string{"X-Gitlab-Token": "nope"}, mr); code != 401 {
		t.Fatal(code)
	}
	if code, out := hookPost(t, e, gl, map[string]string{"X-Gitlab-Token": secret}, mr); code != 200 || out["links"] != float64(1) {
		t.Fatal(code, out)
	}
	glPush := `{"object_kind":"push","ref":"refs/heads/main","project":{"path_with_namespace":"acme/fw"},"commits":[{"id":"ccccccc3333333333333333333333333333333333","message":"REQ-2: CE label","timestamp":"2026-11-04T10:00:00+01:00","url":"https://gitlab.com/acme/fw/-/commit/ccc","author":{"name":"Dev"}}]}`
	if _, out := hookPost(t, e, gl, map[string]string{"X-Gitlab-Token": secret}, glPush); out["links"] != float64(1) {
		t.Fatal(out)
	}
	if code, _ := hookPost(t, e, gl, map[string]string{"X-Gitlab-Token": secret}, `{"object_kind":"note"}`); code != 202 {
		t.Fatal(code)
	}
	if code, _ := hookPost(t, e, gl, map[string]string{"X-Gitlab-Token": secret}, `not json`); code != 400 {
		t.Fatal(code)
	}

	// the code panel of a requirement
	rc := a.m("GET", "/api/v1/requirements/"+f.r1+"/code", nil, 200)
	if ch := rc["changes"].([]any); len(ch) != 1 || ch[0].(map[string]any)["ref"] != "aaaaaaa1111111111111111111111111111111111" {
		t.Fatal(rc)
	}
	if ch := a.m("GET", "/api/v1/tests/"+f.t3+"/code", nil, 200)["changes"].([]any); len(ch) != 1 {
		t.Fatal(ch, t3)
	}
	all := items(a.m("GET", pp+"/code-changes", nil, 200))
	if len(all) != 5 {
		t.Fatal(len(all), all[0])
	}
	if mrs := items(a.m("GET", pp+"/code-changes?kind=merge_request", nil, 200)); len(mrs) != 2 || mrs[0]["status"] != "merged" || mrs[1]["status"] != "open" {
		t.Fatal(mrs)
	}
	a.m("GET", pp+"/code-changes?kind=x", nil, 400)

	// manual links
	man := a.m("POST", "/api/v1/tests/"+f.t1+"/code", map[string]any{"kind": "merge_request", "ref": "!12", "url": "https://git.example.org/fw/-/merge_requests/12", "title": "Boot loader"}, 201)
	if man["provider"] != "manual" || man["ref"] != "12" || man["status"] != "open" {
		t.Fatal(man)
	}
	a.m("POST", "/api/v1/requirements/"+f.r1+"/code", map[string]any{"change_id": man["id"]}, 201)
	a.m("POST", "/api/v1/requirements/"+f.r1+"/code", map[string]any{"change_id": "nope"}, 400)
	a.m("POST", "/api/v1/requirements/"+f.r1+"/code", map[string]any{"kind": "commit", "url": "ftp://x"}, 400)
	a.m("DELETE", "/api/v1/tests/"+f.t1+"/code/"+man["id"].(string), nil, 204)
	a.m("DELETE", "/api/v1/tests/"+f.t1+"/code/"+man["id"].(string), nil, 404)
	a.m("DELETE", "/api/v1/requirements/"+f.r1+"/code/"+man["id"].(string), nil, 204)
	// a manual change linked to nothing is gone
	var left int
	if err := e.st.DB.QueryRow(e.st.DB.Rebind(`SELECT COUNT(*) FROM code_changes WHERE id = ?`), man["id"]).Scan(&left); err != nil || left != 0 {
		t.Fatal(left, err)
	}

	// builds: JUnit results uploaded for a build
	run := a.m("POST", pp+"/runs", map[string]any{"name": "CI 41", "test_ids": []string{f.t1, f.t2}}, 201)["id"].(string)
	junit := `<testsuite><testcase name="boot ` + f.k[f.t1] + `"/><testcase name="110 ` + f.k[f.t2] + `"/></testsuite>`
	q := "?build_id=41&build_number=1.4.41&commit=aaaaaaa1111111111111111111111111111111111&branch=main&pipeline_url=" + url.QueryEscape("https://ci.example.org/p/41")
	res := a.postXML("/api/v1/runs/"+run+"/junit"+q, "application/xml", []byte(junit), 200)
	if b := res["build"].(map[string]any); b["number"] != "1.4.41" {
		t.Fatal(res)
	}
	if r := a.m("GET", "/api/v1/runs/"+run, nil, 200); r["build_label"] != "1.4.41" || r["build"] != "1.4.41" {
		t.Fatal(r)
	}
	a.postXML("/api/v1/runs/"+run+"/junit?commit=zz", "application/xml", []byte(junit), 400)
	a.postXML("/api/v1/runs/"+run+"/junit?build_id=1&commit=xyz", "application/xml", []byte(junit), 400)
	a.postXML("/api/v1/runs/"+run+"/junit?variant=Mars", "application/xml", []byte(junit), 400)
	// a second build where T2 fails, via the JSON results and a variant
	run2 := a.m("POST", pp+"/runs", map[string]any{"name": "CI 42", "test_ids": []string{f.t1, f.t2}}, 201)["id"].(string)
	a.m("POST", "/api/v1/runs/"+run2+"/results?build_id=42&variant=US", map[string]any{"results": []map[string]any{{"test_key": f.k[f.t1], "status": "passed"}, {"test_key": f.k[f.t2], "status": "failed"}}}, 200)
	if r := a.m("GET", "/api/v1/runs/"+run2, nil, 200); r["variant_name"] != "US" {
		t.Fatal(r)
	}
	builds := items(a.m("GET", pp+"/builds", nil, 200))
	if len(builds) != 2 || builds[0]["external_id"] != "42" || builds[1]["runs"] != float64(1) {
		t.Fatal(builds)
	}
	b41 := builds[1]["id"].(string)
	bd := a.m("GET", "/api/v1/builds/"+b41, nil, 200)
	if len(bd["runs"].([]any)) != 1 || len(bd["changes"].([]any)) != 1 {
		t.Fatal(bd)
	}
	// verified in build: R1 passed in 41, failed in 42
	rc = a.m("GET", "/api/v1/requirements/"+f.r1+"/code", nil, 200)
	if vb := rc["verified_build"].(map[string]any); vb["external_id"] != "41" {
		t.Fatal(rc)
	}
	if bs := rc["builds"].([]any); len(bs) != 2 || bs[0].(map[string]any)["coverage"] != "failed" {
		t.Fatal(bs)
	}
	if ch := rc["changes"].([]any); len(ch[0].(map[string]any)["builds"].([]any)) != 1 {
		t.Fatal(ch)
	}
	hist := items(a.m("GET", "/api/v1/tests/"+f.t2+"/builds", nil, 200))
	if len(hist) != 2 || hist[0]["result"] != "failed" || hist[1]["result"] != "passed" {
		t.Fatal(hist)
	}
	if tc := a.m("GET", "/api/v1/tests/"+f.t2+"/code", nil, 200); len(tc["builds"].([]any)) != 2 {
		t.Fatal(tc)
	}
	// registering a build
	if b := a.m("POST", pp+"/builds", map[string]any{"external_id": "43", "commit_sha": "bbbbbbb2222222222222222222222222222222222"}, 201); b["external_id"] != "43" {
		t.Fatal(b)
	}
	a.m("POST", pp+"/builds", map[string]any{"external_id": "43", "branch": "refs/heads/main"}, 200)
	a.m("POST", pp+"/builds", map[string]any{}, 400)

	// regression selection: changed requirements
	sel := a.m("POST", pp+"/regression", map[string]any{"requirement_ids": []string{r1}}, 200)
	if got := strings.Join(keysOf(toMaps(sel["tests"])), ","); got != f.k[f.t1]+","+f.k[f.t2] {
		t.Fatal(got)
	}
	sel = a.m("POST", pp+"/regression", map[string]any{"requirement_ids": []string{r1}, "variant": "EU"}, 200)
	if got := keysOf(toMaps(sel["tests"])); len(got) != 1 || sel["variant"].(map[string]any)["name"] != "EU" {
		t.Fatal(got)
	}
	a.m("POST", pp+"/regression", map[string]any{"requirement_ids": []string{"NOPE-1"}}, 400)
	a.m("POST", pp+"/regression", map[string]any{"variant": "Mars"}, 400)
	// commits since build 41 (its commit aaaa…): bbbb (no keys) and cccc (REQ-2 → T3)
	sel = a.m("POST", pp+"/regression", map[string]any{"since_build": "1.4.41"}, 200)
	if len(sel["changes"].([]any)) != 2 || strings.Join(keysOf(toMaps(sel["tests"])), ",") != t3 {
		t.Fatal(sel)
	}
	if rs := toMaps(sel["requirements"]); len(rs) != 1 || rs[0]["reasons"].([]any)[0].(map[string]any)["kind"] != "change" {
		t.Fatal(rs)
	}
	a.m("POST", pp+"/regression", map[string]any{"since_build": "nope"}, 400)
	// related requirements and prerequisites
	a.m("POST", "/api/v1/requirements/"+f.r3+"/relations", map[string]any{"target_id": f.r2, "type": "depends_on"}, 201)
	t4 := a.m("POST", pp+"/tests", map[string]any{"title": "UL check"}, 201)
	a.m("POST", pp+"/links", map[string]any{"requirement_id": f.r3, "test_case_id": t4["id"]}, 201)
	a.m("POST", "/api/v1/tests/"+t4["id"].(string)+"/dependencies", map[string]any{"depends_on_id": f.t1}, 201)
	sel = a.m("POST", pp+"/regression", map[string]any{"requirement_ids": []string{f.r2}, "include_related": true}, 200)
	if got := strings.Join(keysOf(toMaps(sel["tests"])), ","); got != f.k[f.t1]+","+f.k[f.t3]+","+t4["key"].(string) {
		t.Fatal(got)
	}
	// a run from the selection
	created := a.m("POST", pp+"/regression/run", map[string]any{"since_build": "41", "variant": "EU"}, 201)
	if r := created["run"].(map[string]any); r["name"] != "Regression since build 1.4.41" || r["variant_name"] != "EU" || r["summary"].(map[string]any)["total"] != float64(1) {
		t.Fatal(created)
	}
	a.m("POST", pp+"/regression/run", map[string]any{"requirement_ids": []string{}}, 400)
	ids := a.m("POST", pp+"/regression", map[string]any{"change_ids": []string{all[0]["id"].(string)}}, 200)
	if len(ids["changes"].([]any)) != 1 {
		t.Fatal(ids)
	}
	a.m("POST", pp+"/regression", map[string]any{"change_ids": []string{"nope"}}, 400)
	if cs := items(a.m("GET", pp+"/code-changes?since_build=41", nil, 200)); len(cs) != 2 {
		t.Fatal(cs)
	}

	// deleting a build keeps its runs
	a.m("DELETE", "/api/v1/builds/"+b41, nil, 204)
	if r := a.m("GET", "/api/v1/runs/"+run, nil, 200); r["build_id"] != nil {
		t.Fatal(r)
	}
	a.m("DELETE", "/api/v1/builds/"+b41, nil, 404)

	// permissions
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	v.m("GET", "/api/v1/requirements/"+f.r1+"/code", nil, 200)
	v.m("POST", "/api/v1/requirements/"+f.r1+"/code", map[string]any{"kind": "commit", "ref": "abcd"}, 403)
	v.m("PUT", pp+"/code-settings", map[string]any{"secret": secret}, 403)
	v.m("POST", pp+"/regression/run", map[string]any{"requirement_ids": []string{r1}}, 403)
	v.m("POST", pp+"/builds", map[string]any{"external_id": "x"}, 403)

	// clearing the secret disables the hooks
	a.m("PUT", pp+"/code-settings", map[string]any{"clear_secret": true}, 200)
	if code, _ := hookPost(t, e, gl, map[string]string{"X-Gitlab-Token": secret}, glPush); code != 401 {
		t.Fatal(code)
	}
}

func toMaps(v any) []map[string]any {
	var out []map[string]any
	for _, x := range v.([]any) {
		out = append(out, x.(map[string]any))
	}
	return out
}
