package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/rom/qtestframe/internal/config"
)

// fakeLLM is an Anthropic Messages API or OpenAI-compatible service
// (httptest; no network) that answers each feature by the schema in its
// system prompt. bad makes the next answers invalid.
type fakeLLM struct {
	mu      sync.Mutex
	systems []string
	users   []string
	keys    []string
	bad     int
}

func (f *fakeLLM) answer(system string) string {
	switch {
	case strings.Contains(system, `"mappings"`):
		return `{"mappings": [{"framework": "d3fend", "id": "d3-am", "reason": "Access rules"}, {"framework": "d3fend", "id": "D3-NOPE", "reason": "made up"},
			{"framework": "cwe", "id": "CWE-434", "reason": "Uploads"}, {"framework": "cwe", "id": "CWE-999999", "reason": "made up"},
			{"framework": "emb3d", "id": "TID-201", "reason": "Boot"}, {"framework": "attack", "id": "T1110", "reason": "not for requirements"},
			{"framework": "d3fend", "id": "D3-AM", "reason": "duplicate"}]}`
	case strings.Contains(system, `"causes"`):
		return `{"summary": "One test failed: the sign-in page shows an error.", "causes": [{"title": "Sign-in service down", "explanation": "500 in the output", "tests": ["AI-TC-1", "XX-9"]}],
			"defects": [{"title": "Sign-in fails with 500", "description": "Observed: 500. Expected: dashboard.", "severity": "major", "tests": ["AI-TC-1"]}]}`
	case strings.Contains(system, `"addresses"`):
		return `{"title": "Sign-in time", "description": "When the user submits valid credentials, the system MUST show the dashboard within 2 s.", "explanation": "Replaced the vague term and the TBD.", "addresses": ["ambiguous-term", "tbd"]}`
	case strings.Contains(system, `{"tests"`):
		return `{"tests": [{"title": "Valid sign-in", "description": "Happy path", "preconditions": "An account exists", "test_data": "alice / secret",
			"expected_result": "Dashboard shown", "steps": [{"action": "Open the sign-in page", "data": "", "expected": "Form shown"}, {"action": "Sign in", "data": "alice", "expected": "Dashboard"}], "criteria": [1, 2, 7]}]}`
	case strings.Contains(system, `"criteria"`):
		return `{"criteria": [{"description": "Valid sign-in opens the dashboard", "method": "test", "threshold": "< 2 s"}, {"description": "Existing criterion", "method": "review", "threshold": ""}]}`
	}
	return "OK"
}

func (f *fakeLLM) record(r *http.Request) (map[string]any, string) {
	var body map[string]any
	b, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(b, &body)
	f.mu.Lock()
	defer f.mu.Unlock()
	system, _ := body["system"].(string)
	var user string
	for _, m := range body["messages"].([]any) {
		mm := m.(map[string]any)
		if mm["role"] == "system" {
			system = mm["content"].(string)
		} else if mm["role"] == "user" {
			user = mm["content"].(string)
		}
	}
	f.systems = append(f.systems, system)
	f.users = append(f.users, user)
	f.keys = append(f.keys, r.Header.Get("x-api-key")+r.Header.Get("Authorization"))
	text := f.answer(system)
	if f.bad > 0 {
		f.bad--
		text = "not json"
	}
	return body, text
}

func (f *fakeLLM) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, text := f.record(r)
		w.Header().Set("Content-Type", "text/event-stream")
		if strings.HasSuffix(r.URL.Path, "/chat/completions") {
			b, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"delta": map[string]string{"content": text}, "finish_reason": "stop"}},
				"usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 20}})
			fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", b)
			return
		}
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"model\":\"fake-model\",\"usage\":{\"input_tokens\":100}}}\n\n")
		for len(text) > 0 {
			n := min(len(text), 20)
			b, _ := json.Marshal(map[string]any{"type": "content_block_delta", "delta": map[string]string{"type": "text_delta", "text": text[:n]}})
			fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", b)
			text = text[n:]
		}
		fmt.Fprint(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":20}}\n\n")
	})
}

func aiEnv(t *testing.T, f *fakeLLM, mut func(*config.AI)) (*env, *client, string) {
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	e := newEnv(t, func(c *config.Config) {
		c.AI.Enabled, c.AI.URL, c.AI.APIKey, c.AI.Model = true, srv.URL, "test-key", "test-model"
		if mut != nil {
			mut(&c.AI)
		}
	})
	a := e.login("admin@example.com")
	pid := a.project("AI")
	return e, a, pid
}

func enableOrgAI(a *client, on bool) {
	st := a.m("GET", "/api/v1/settings", nil, 200)
	st["ai_enabled"] = on
	a.m("PUT", "/api/v1/settings", st, 200)
}

// sse reads the server-sent events of a response.
func sse(t *testing.T, res *http.Response) map[string][]map[string]any {
	t.Helper()
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content type %q", ct)
	}
	out := map[string][]map[string]any{}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	event := ""
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = line[7:]
		case strings.HasPrefix(line, "data: "):
			var v map[string]any
			if err := json.Unmarshal([]byte(line[6:]), &v); err != nil {
				t.Fatal(err, line)
			}
			out[event] = append(out[event], v)
		}
	}
	return out
}

func TestAIAvailability(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("OFF")
	st := a.m("GET", "/api/v1/projects/"+pid+"/ai", nil, 200)
	if st["available"] != false || st["reason"] != aiOff || st["features"].(map[string]any)["draft_tests"] != true {
		t.Fatal(st)
	}
	rid := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "description": "The user MUST sign in."}, 201)["id"].(string)
	if r := a.m("POST", "/api/v1/projects/"+pid+"/ai/suggest-criteria", map[string]any{"requirement_id": rid}, 409); r["code"] != "ai_unavailable" {
		t.Fatal(r)
	}

	f := &fakeLLM{}
	e, a, pid = aiEnv(t, f, nil)
	if st := a.m("GET", "/api/v1/projects/"+pid+"/ai", nil, 200); st["reason"] != aiOrganisation || st["model"] != nil {
		t.Fatal(st)
	}
	enableOrgAI(a, true)
	st = a.m("GET", "/api/v1/projects/"+pid+"/ai", nil, 200)
	if st["available"] != true || st["provider"] != "anthropic" || st["model"] != "test-model" || st["remaining"].(float64) != 30 {
		t.Fatal(st)
	}
	// the project opts out (project.update)
	if st := a.m("PUT", "/api/v1/projects/"+pid+"/ai", map[string]any{"disabled": true}, 200); st["reason"] != aiProject || st["project_disabled"] != true {
		t.Fatal(st)
	}
	a.m("PUT", "/api/v1/projects/"+pid+"/ai", map[string]any{}, 400)
	e.member("tess@example.com", "tester")
	ts := e.login("tess@example.com")
	ts.m("PUT", "/api/v1/projects/"+pid+"/ai", map[string]any{"disabled": false}, 403)
	if st := ts.m("GET", "/api/v1/projects/"+pid+"/ai", nil, 200); st["reason"] != aiProject {
		t.Fatal(st)
	}
	// the organisation view ignores the project's opt-out
	if st := ts.m("GET", "/api/v1/ai", nil, 200); st["available"] != true || st["organisation_enabled"] != true {
		t.Fatal(st)
	}
	a.m("PUT", "/api/v1/projects/"+pid+"/ai", map[string]any{"disabled": false}, 200)
	// a feature turned off
	e.srv.Cfg.AI.Features.SuggestCriteria = false
	e.srv.initSettings(t.Context())
	rid = a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "description": "The user MUST sign in."}, 201)["id"].(string)
	if r := a.m("POST", "/api/v1/projects/"+pid+"/ai/suggest-criteria", map[string]any{"requirement_id": rid}, 409); !strings.Contains(r["error"].(string), aiFeatureOff) {
		t.Fatal(r)
	}
	if len(f.systems) != 0 {
		t.Fatal("no call while unavailable")
	}
	// testers cannot change requirements: no criteria suggestions; viewers draft no tests
	ts.m("POST", "/api/v1/projects/"+pid+"/ai/improve-requirement", map[string]any{"requirement_id": rid}, 403)
	e.member("vic@example.com", "viewer")
	e.login("vic@example.com").m("POST", "/api/v1/projects/"+pid+"/ai/draft-tests", map[string]any{"requirement_id": rid}, 403)
}

func TestAIFeatures(t *testing.T) {
	f := &fakeLLM{}
	e, a, pid := aiEnv(t, f, func(c *config.AI) { c.PriceInput, c.PriceOutput = "3", "15" })
	enableOrgAI(a, true)
	req := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Sign-in", "description": "Sign-in should be fast. TBD.",
		"criteria": []map[string]any{{"description": "Existing criterion", "method": "test"}, {"description": "Second", "method": "review"}}}, 201)
	rid := req["id"].(string)
	crits := req["criteria"].([]any)

	// 1. draft tests (plain JSON answer)
	res := a.m("POST", "/api/v1/projects/"+pid+"/ai/draft-tests", map[string]any{"requirement_id": rid, "lang": "sv"}, 200)
	meta := res["meta"].(map[string]any)
	if meta["provider"] != "anthropic" || meta["model"] != "fake-model" || meta["usage"].(map[string]any)["input_tokens"].(float64) != 100 || meta["prompt_version"] == "" {
		t.Fatal(meta)
	}
	if c := meta["cost"].(float64); c < 0.00059 || c > 0.00061 {
		t.Fatalf("cost %v", c)
	}
	tests := res["result"].(map[string]any)["tests"].([]any)
	t0 := tests[0].(map[string]any)
	ids := t0["criterion_ids"].([]any)
	if len(tests) != 1 || len(ids) != 2 || ids[0] != crits[0].(map[string]any)["id"] || len(t0["steps"].([]any)) != 2 {
		t.Fatal(res)
	}
	if !strings.Contains(f.systems[0], "svenska") || !strings.Contains(f.users[0], "Existing criterion") || f.keys[0] != "test-key" {
		t.Fatal(f.systems[0], f.users[0])
	}
	// nothing was saved
	if n := len(items(a.m("GET", "/api/v1/projects/"+pid+"/tests", nil, 200))); n != 0 {
		t.Fatalf("%d tests saved", n)
	}

	// streamed: start, progress, result
	c := *a
	c.headers = map[string]string{"Accept": "text/event-stream"}
	ev := sse(t, c.raw("POST", "/api/v1/projects/"+pid+"/ai/draft-tests", map[string]any{"requirement_id": rid}))
	if len(ev["start"]) != 1 || len(ev["result"]) != 1 || len(ev["error"]) != 0 {
		t.Fatal(ev)
	}
	if r := ev["result"][0]["result"].(map[string]any); len(r["tests"].([]any)) != 1 {
		t.Fatal(r)
	}

	// 2. improve: the server checks the rewrite again
	res = a.m("POST", "/api/v1/projects/"+pid+"/ai/improve-requirement", map[string]any{"requirement_id": rid, "description": "Sign-in should be fast. TBD."}, 200)
	r := res["result"].(map[string]any)
	fixed := fmt.Sprint(r["fixed"])
	if !strings.Contains(fixed, "tbd") || !strings.Contains(fixed, "ambiguous-term") || len(r["new"].([]any)) != 0 || r["original"].(map[string]any)["title"] != "Sign-in" {
		t.Fatal(r)
	}
	if !strings.Contains(f.users[len(f.users)-1], "tbd") {
		t.Fatal("the findings are sent")
	}
	// an unsaved requirement (no id) can be improved too
	a.m("POST", "/api/v1/projects/"+pid+"/ai/improve-requirement", map[string]any{"title": "New", "description": "It should be fast."}, 200)
	a.m("POST", "/api/v1/projects/"+pid+"/ai/improve-requirement", map[string]any{"title": "", "description": ""}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/ai/improve-requirement", map[string]any{"requirement_id": rid, "analysis_language": "xx"}, 400)

	// 5. criteria: duplicates of existing ones are dropped
	res = a.m("POST", "/api/v1/projects/"+pid+"/ai/suggest-criteria", map[string]any{"requirement_id": rid}, 200)
	if cs := res["result"].(map[string]any)["criteria"].([]any); len(cs) != 1 || cs[0].(map[string]any)["threshold"] != "< 2 s" {
		t.Fatal(res)
	}

	// 4. mappings: validated against the embedded lists
	res = a.m("POST", "/api/v1/projects/"+pid+"/ai/suggest-mappings", map[string]any{"requirement_id": rid}, 200)
	r = res["result"].(map[string]any)
	got := []string{}
	for _, m := range r["mappings"].([]any) {
		mm := m.(map[string]any)
		got = append(got, mm["framework"].(string)+":"+mm["id"].(string))
		if mm["name"] == "" && mm["framework"] != "cwe" {
			t.Fatal(mm)
		}
	}
	if strings.Join(got, ",") != "d3fend:D3-AM,cwe:CWE-434,emb3d:TID-201" || len(r["dropped"].([]any)) != 3 {
		t.Fatal(r)
	}
	// risks: ATT&CK and EMB3D threats only
	risk := a.m("POST", "/api/v1/projects/"+pid+"/risks", map[string]any{"title": "Password guessing", "likelihood": 3, "impact": 3}, 201)
	res = a.m("POST", "/api/v1/projects/"+pid+"/ai/suggest-mappings", map[string]any{"risk_id": risk["id"]}, 200)
	got = got[:0]
	for _, m := range res["result"].(map[string]any)["mappings"].([]any) {
		mm := m.(map[string]any)
		got = append(got, mm["framework"].(string)+":"+mm["id"].(string))
	}
	if strings.Join(got, ",") != "emb3d:TID-201,attack:T1110" {
		t.Fatal(got)
	}
	// a risk not saved yet
	res = a.m("POST", "/api/v1/projects/"+pid+"/ai/suggest-mappings", map[string]any{"kind": "risk", "title": "Brute force", "description": "Guessing"}, 200)
	if res["result"].(map[string]any)["kind"] != "risk" {
		t.Fatal(res)
	}
	a.m("POST", "/api/v1/projects/"+pid+"/ai/suggest-mappings", map[string]any{"kind": "risk", "title": " "}, 400)
	// another project's risk
	other := a.project("OTH")
	a.m("POST", "/api/v1/projects/"+other+"/ai/suggest-mappings", map[string]any{"risk_id": risk["id"]}, 400)
	a.m("POST", "/api/v1/projects/"+other+"/ai/draft-tests", map[string]any{"requirement_id": rid}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/ai/draft-tests", map[string]any{}, 400)

	// 3. summarise a failed run
	tc := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Valid sign-in", "steps": []map[string]any{{"action": "Sign in", "expected": "Dashboard"}}}, 201)
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "Nightly", "test_ids": []string{tc["id"].(string)}}, 201)
	runID := run["id"].(string)
	a.m("POST", "/api/v1/projects/"+pid+"/ai/summarize-run", map[string]any{"run_id": runID}, 400) // nothing failed
	execs := items(a.m("GET", "/api/v1/runs/"+runID+"/executions", nil, 200))
	exID := execs[0]["id"].(string)
	ex := a.m("GET", "/api/v1/executions/"+exID, nil, 200)
	sr := ex["step_results"].([]any)[0].(map[string]any)
	a.m("PATCH", "/api/v1/executions/"+exID, map[string]any{"step_results": []map[string]any{{"id": sr["id"], "status": "failed", "actual": "HTTP 500"}}, "notes": "crashed"}, 200)
	a.m("POST", "/api/v1/projects/"+pid+"/defects", map[string]any{"title": "Old crash", "severity": "minor", "links": []map[string]any{{"entity_type": "execution", "entity_id": exID}}}, 201)
	res = a.m("POST", "/api/v1/projects/"+pid+"/ai/summarize-run", map[string]any{"run_id": runID, "lang": "en"}, 200)
	r = res["result"].(map[string]any)
	d0 := r["defects"].([]any)[0].(map[string]any)
	tref := d0["tests"].([]any)[0].(map[string]any)
	if tref["execution_id"] != exID || tref["test_case_id"] != tc["id"] || d0["severity"] != "major" {
		t.Fatal(r)
	}
	if cs := r["causes"].([]any)[0].(map[string]any)["tests"].([]any); len(cs) != 1 {
		t.Fatal("tests that are not in the run are dropped", cs)
	}
	user := f.users[len(f.users)-1]
	if !strings.Contains(user, "HTTP 500") || !strings.Contains(user, "Old crash") || !strings.Contains(user, "crashed") {
		t.Fatal(user)
	}

	// the audit log has one entry per call: feature, items, tokens; no content
	var audit struct {
		Items []map[string]any `json:"items"`
	}
	a.call("GET", "/api/v1/audit?limit=500", nil, 200, &audit)
	n := 0
	for _, it := range audit.Items {
		if it["action"] != "ai.request" {
			continue
		}
		n++
		d := it["details"].(string)
		if !strings.Contains(d, "input_tokens=100") || !strings.Contains(d, "status=ok") || strings.Contains(d, "Sign-in should") || strings.Contains(d, "HTTP 500") {
			t.Fatal(d)
		}
	}
	if n != 9 {
		t.Fatalf("%d audit entries", n)
	}
	_ = e
}

func TestAIRetryErrorsAndLimit(t *testing.T) {
	f := &fakeLLM{}
	_, a, pid := aiEnv(t, f, func(c *config.AI) { c.RateLimit = 3 })
	enableOrgAI(a, true)
	rid := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "description": "The user MUST sign in."}, 201)["id"].(string)
	// one invalid answer: retried
	f.bad = 1
	res := a.m("POST", "/api/v1/projects/"+pid+"/ai/suggest-criteria", map[string]any{"requirement_id": rid}, 200)
	if m := res["meta"].(map[string]any); m["retried"] != true || m["usage"].(map[string]any)["output_tokens"].(float64) != 40 {
		t.Fatal(m)
	}
	// two: an error (streamed as an error event)
	f.bad = 2
	if r := a.m("POST", "/api/v1/projects/"+pid+"/ai/suggest-criteria", map[string]any{"requirement_id": rid}, 502); r["code"] != "ai_invalid_answer" {
		t.Fatal(r)
	}
	f.bad = 2
	c := *a
	c.headers = map[string]string{"Accept": "text/event-stream"}
	ev := sse(t, c.raw("POST", "/api/v1/projects/"+pid+"/ai/suggest-criteria", map[string]any{"requirement_id": rid}))
	if len(ev["error"]) != 1 || ev["error"][0]["code"] != "ai_invalid_answer" {
		t.Fatal(ev)
	}
	// the rate limit per user
	if r := a.m("POST", "/api/v1/projects/"+pid+"/ai/suggest-criteria", map[string]any{"requirement_id": rid}, 429); r["code"] != "rate_limited" {
		t.Fatal(r)
	}
	if st := a.m("GET", "/api/v1/projects/"+pid+"/ai", nil, 200); st["remaining"].(float64) != 0 {
		t.Fatal(st)
	}
}

func TestAIOpenAIAndServiceErrors(t *testing.T) {
	f := &fakeLLM{}
	e, a, pid := aiEnv(t, f, func(c *config.AI) { c.Provider, c.Model, c.URL = config.AIOpenAI, "local-model", c.URL+"/v1" })
	enableOrgAI(a, true)
	rid := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "description": "The user MUST sign in."}, 201)["id"].(string)
	res := a.m("POST", "/api/v1/projects/"+pid+"/ai/suggest-criteria", map[string]any{"requirement_id": rid}, 200)
	if m := res["meta"].(map[string]any); m["provider"] != "openai" || m["model"] != "local-model" {
		t.Fatal(m)
	}
	if f.keys[0] != "Bearer test-key" {
		t.Fatal(f.keys)
	}
	// the settings test (super admins), with unsaved values
	if r := a.m("POST", "/api/v1/admin/settings/test/ai", map[string]any{}, 200); r["ok"] != true || !strings.Contains(r["message"].(string), "100 input") {
		t.Fatal(r)
	}
	if r := a.m("POST", "/api/v1/admin/settings/test/ai", map[string]any{"values": map[string]any{"ai.url": "http://127.0.0.1:1/v1"}}, 200); r["ok"] != false {
		t.Fatal(r)
	}
	a.m("POST", "/api/v1/admin/settings/test/ai", map[string]any{"values": map[string]any{"similarity.embeddings.url": "x"}}, 400)
	// a failing service
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"message":"overloaded"}}`)
	}))
	defer down.Close()
	e.srv.Cfg.AI.URL = down.URL
	e.srv.initSettings(t.Context())
	if r := a.m("POST", "/api/v1/projects/"+pid+"/ai/suggest-criteria", map[string]any{"requirement_id": rid}, 502); r["code"] != "ai_service" || !strings.Contains(r["error"].(string), "overloaded") {
		t.Fatal(r)
	}
	// the key is write-only in the system settings
	var view struct {
		Settings []map[string]any `json:"settings"`
	}
	a.call("GET", "/api/v1/admin/settings", nil, 200, &view)
	found := false
	for _, s := range view.Settings {
		if s["key"] == "ai.api_key" {
			found = true
			if s["value"] != nil {
				t.Fatal(s)
			}
		}
	}
	if !found {
		t.Fatal("ai.api_key is a system setting")
	}
	ts := e.member("t2@example.com", "test_lead")
	_ = ts
	e.login("t2@example.com").m("POST", "/api/v1/admin/settings/test/ai", map[string]any{}, 403)
}

func TestRuleDiff(t *testing.T) {
	if got := fmt.Sprint(ruleDiff(nil, nil)); got != "[]" {
		t.Fatal(got)
	}
	if head("abcdef", 3) != "abc…" || tail("abcdef", 3) != "…def" || head("ab", 3) != "ab" {
		t.Fatal("head/tail")
	}
	st, c, n := aiError(fmt.Errorf("x: %w", errTestTimeout))
	if st != http.StatusGatewayTimeout || c != "ai_timeout" || n == "" {
		t.Fatal(st, c)
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string { return "Client.Timeout exceeded" }

var errTestTimeout = timeoutErr{}
