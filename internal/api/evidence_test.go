package api

import (
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

func TestSuspectLinksAndEvidenceStatus(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("EVI")
	a.m("PUT", "/api/v1/settings", map[string]any{"allow_unready_tests": true, "allow_unready_requirements": true}, 200)
	// this test is about suspect links: results stay evidence after a change
	// (staleness on change: correlations_test.go)
	a.m("PUT", "/api/v1/projects/"+pid+"/correlation-settings", map[string]any{"stale_on_change": false}, 200)
	rq := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "description": "The user MUST log in.", "status": "implemented", "create_test": false}, 201)
	t1 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Valid login", "status": "ready"}, 201)
	t2 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Wrong password", "status": "ready"}, 201)
	rid := rq["id"].(string)
	l1 := a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": rid, "test_case_id": t1["id"]}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": rid, "test_case_id": t2["id"]}, 201)
	status := func() string { return a.m("GET", "/api/v1/requirements/"+rid, nil, 200)["status"].(string) }
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R", "test_ids": []string{t1["id"].(string), t2["id"].(string)}}, 201)
	ex := map[string]string{}
	for _, x := range items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200)) {
		ex[x["test_case_id"].(string)] = x["id"].(string)
	}

	// one passed: still implemented; both passed: verified (with a revision "evidence")
	a.m("PATCH", "/api/v1/executions/"+ex[t1["id"].(string)], map[string]any{"status": "passed"}, 200)
	if s := status(); s != "implemented" {
		t.Fatal(s)
	}
	a.m("PATCH", "/api/v1/executions/"+ex[t2["id"].(string)], map[string]any{"status": "passed"}, 200)
	if s := status(); s != "verified" {
		t.Fatal(s)
	}
	hist := items(a.m("GET", "/api/v1/requirements/"+rid+"/history", nil, 200))
	if hist[0]["action"] != "evidence" {
		t.Fatal(hist[0])
	}
	// a failure takes it back
	a.m("PATCH", "/api/v1/executions/"+ex[t2["id"].(string)], map[string]any{"status": "failed"}, 200)
	if s := status(); s != "implemented" {
		t.Fatal(s)
	}
	a.m("PATCH", "/api/v1/executions/"+ex[t2["id"].(string)], map[string]any{"status": "passed"}, 200)
	if s := status(); s != "verified" {
		t.Fatal(s)
	}

	// changing the requirement's text makes its links suspect and the status falls back
	a.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"description": "The user MUST log in with 2FA."}, 200)
	links := items(a.m("GET", "/api/v1/requirements/"+rid+"/links", nil, 200))
	if len(links) != 2 || links[0]["suspect_since"] == nil || !strings.Contains(links[0]["suspect_reason"].(string), "description") {
		t.Fatal(links)
	}
	got := a.m("GET", "/api/v1/requirements/"+rid, nil, 200)
	if got["status"] != "implemented" || got["suspect_links"].(float64) != 2 {
		t.Fatal(got)
	}
	if res := items(a.m("GET", "/api/v1/projects/"+pid+"/requirements?q=suspect>0", nil, 200)); len(res) != 1 {
		t.Fatal(res)
	}
	// a priority change is not a content change
	a.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"priority": "high"}, 200)
	// confirming one link is not enough; confirming all restores verified
	if res := a.m("POST", "/api/v1/links/"+l1["id"].(string)+"/confirm", nil, 200); res["confirmed"].(float64) != 1 || status() != "implemented" {
		t.Fatal(res)
	}
	res := a.m("POST", "/api/v1/requirements/"+rid+"/links/confirm", nil, 200)
	if res["confirmed"].(float64) != 1 || len(res["status_changes"].([]any)) != 1 || status() != "verified" {
		t.Fatal(res)
	}
	a.m("POST", "/api/v1/links/missing/confirm", nil, 404)
	a.m("POST", "/api/v1/tests/missing/links/confirm", nil, 404)

	// a test's steps change: suspect from the test side
	a.m("PATCH", "/api/v1/tests/"+t1["id"].(string), map[string]any{"steps": []map[string]any{{"action": "Sign in", "expected": "Dashboard"}}}, 200)
	if got := a.m("GET", "/api/v1/tests/"+t1["id"].(string), nil, 200); got["suspect_links"].(float64) != 1 || status() != "implemented" {
		t.Fatal(got, status())
	}
	a.m("POST", "/api/v1/tests/"+t1["id"].(string)+"/links/confirm", nil, 200)
	if s := status(); s != "verified" {
		t.Fatal(s)
	}
	// linking a new, not yet run test takes it back; removing the link restores it
	t3 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Lockout"}, 201)
	l3 := a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": rid, "test_case_id": t3["id"]}, 201)
	if s := status(); s != "implemented" {
		t.Fatal(s)
	}
	a.m("DELETE", "/api/v1/links/"+l3["id"].(string), nil, 204)
	if s := status(); s != "verified" {
		t.Fatal(s)
	}
	// the setting turns it off
	a.m("PUT", "/api/v1/settings", map[string]any{"allow_unready_tests": true, "allow_unready_requirements": true, "manual_verification": true}, 200)
	a.m("PATCH", "/api/v1/executions/"+ex[t2["id"].(string)], map[string]any{"status": "failed"}, 200)
	if s := status(); s != "verified" {
		t.Fatal(s)
	}
	// requirements in other statuses and without tests are left alone
	if store.EvidenceStatus("approved", []string{"x"}, map[string]string{"x": "passed"}, false) != "" ||
		store.EvidenceStatus("implemented", nil, nil, false) != "" ||
		store.EvidenceStatus("implemented", []string{"x"}, map[string]string{"x": "passed"}, true) != "" {
		t.Fatal("EvidenceStatus")
	}
	if got := store.ContentChanged(model.KindTest, []string{"priority", "steps", "status"}); len(got) != 1 || got[0] != "steps" {
		t.Fatal(got)
	}
}
