package api

import (
	"testing"
)

func bulkKeys(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		m := x.(map[string]any)
		k := m["key"].(string)
		if r, ok := m["reason"].(string); ok {
			k += ":" + r
		}
		out = append(out, k)
	}
	return out
}

func TestBulkReviewRequirements(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BR")
	mk := func(title, desc, status string) string {
		return a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": title, "description": desc, "status": status}, 201)["id"].(string)
	}
	r1 := mk("One", "The system MUST do one.", "review")
	r2 := mk("Two", "The system MUST do two.", "review")
	r3 := mk("Three", "The system SHOULD do three.", "draft")
	r4 := mk("Four", "The system MUST do four.", "approved")
	r5 := mk("Five", "Just a description.", "review") // no obligation
	_ = r4
	a.m("PATCH", "/api/v1/requirements/"+r5, map[string]any{"obligation": ""}, 200)
	path := "/api/v1/projects/" + pid + "/requirements/review"

	a.m("POST", path, map[string]any{"decision": "ready", "all": true}, 400)
	a.m("POST", path, map[string]any{"decision": "approved"}, 400)
	a.m("POST", path, map[string]any{"decision": "approved", "all": true, "from": []string{"nope"}}, 400)
	a.m("POST", path, map[string]any{"decision": "approved", "all": true, "ignore": []string{"status"}}, 400)
	a.m("POST", path, map[string]any{"decision": "approved", "ids": []string{"missing"}}, 404)

	// preview of all: only those in review qualify; nothing changes
	pre := a.m("POST", path, map[string]any{"decision": "approved", "all": true, "dry_run": true}, 200)
	if got := bulkKeys(pre["updated"]); len(got) != 2 || got[0] != "BR-REQ-1" || got[1] != "BR-REQ-2" {
		t.Fatal(pre)
	}
	skipped := map[string]bool{}
	for _, k := range bulkKeys(pre["skipped"]) {
		skipped[k] = true
	}
	if !skipped["BR-REQ-3:status"] || !skipped["BR-REQ-4:already"] || !skipped["BR-REQ-5:no_obligation"] {
		t.Fatal(pre["skipped"])
	}
	if s := a.m("GET", "/api/v1/requirements/"+r1, nil, 200)["status"]; s != "review" {
		t.Fatal(s)
	}

	// a reviewer asked for changes on r2: skipped unless ignored
	e.member("rita@example.com", "reviewer")
	rc := e.login("rita@example.com")
	rv := items(a.m("POST", "/api/v1/reviews", map[string]any{"entity_type": "requirement", "entity_id": r2, "reviewers": []string{"rita@example.com"}}, 201))[0]
	pend := a.m("POST", path, map[string]any{"decision": "approved", "ids": []string{r2}, "dry_run": true}, 200)
	if got := bulkKeys(pend["skipped"]); len(got) != 1 || got[0] != "BR-REQ-2:pending_review" {
		t.Fatal(pend)
	}
	rc.m("PATCH", "/api/v1/reviews/"+rv["id"].(string), map[string]any{"comment": "Unclear", "decision": "changes_requested"}, 200)

	// a reviewer (no write permission) approves a subset by query, with a comment
	res := rc.m("POST", path, map[string]any{"decision": "approved", "query": "title:One OR title:Two OR title:Three", "from": []string{"review", "draft"}, "comment": "Approved in the review meeting"}, 200)
	if got := bulkKeys(res["updated"]); len(got) != 2 || got[0] != "BR-REQ-1" || got[1] != "BR-REQ-3" {
		t.Fatal(res)
	}
	if got := bulkKeys(res["skipped"]); len(got) != 1 || got[0] != "BR-REQ-2:changes_requested" {
		t.Fatal(res)
	}
	for _, id := range []string{r1, r3} {
		got := a.m("GET", "/api/v1/requirements/"+id, nil, 200)
		if got["status"] != "approved" || got["comment_count"].(float64) != 1 {
			t.Fatal(got)
		}
	}
	if hist := items(a.m("GET", "/api/v1/requirements/"+r1+"/history", nil, 200)); hist[0]["action"] != "bulk_update" {
		t.Fatal(hist[0])
	}
	// ignoring the check approves it anyway; rejecting works the same way
	if res := rc.m("POST", path, map[string]any{"decision": "approved", "ids": []string{r2}, "ignore": []string{"changes_requested"}}, 200); len(res["updated"].([]any)) != 1 {
		t.Fatal(res)
	}
	if res := rc.m("POST", path, map[string]any{"decision": "rejected", "ids": []string{r5}}, 200); len(res["updated"].([]any)) != 1 {
		t.Fatal(res)
	}

	// testers cannot review
	e.member("tester@example.com", "tester")
	e.login("tester@example.com").m("POST", path, map[string]any{"decision": "approved", "all": true}, 403)
	acts := auditActions(t, e)
	if !acts["requirement.bulk_review"] || !acts["requirement.review"] {
		t.Fatal(acts)
	}
}

func TestBulkReviewTests(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BT")
	steps := []map[string]any{{"action": "Open", "expected": "Shown"}}
	mk := func(body map[string]any) string {
		return a.m("POST", "/api/v1/projects/"+pid+"/tests", body, 201)["id"].(string)
	}
	t1 := mk(map[string]any{"title": "Manual with steps", "status": "review", "steps": steps})
	mk(map[string]any{"title": "Manual without steps", "status": "review"})
	mk(map[string]any{"title": "Automated without script", "status": "draft", "execution_type": "automated"})
	t4 := mk(map[string]any{"title": "Automated", "status": "draft", "execution_type": "automated", "script_language": "sh", "script": "exit 0"})
	path := "/api/v1/projects/" + pid + "/tests/review"
	a.m("POST", path, map[string]any{"decision": "approved", "all": true}, 400)

	res := a.m("POST", path, map[string]any{"decision": "ready", "all": true, "from": []string{"draft", "review"}}, 200)
	if got := bulkKeys(res["updated"]); len(got) != 2 || got[0] != "BT-TC-1" || got[1] != "BT-TC-4" {
		t.Fatal(res)
	}
	if got := bulkKeys(res["skipped"]); len(got) != 2 || got[0] != "BT-TC-2:no_steps" || got[1] != "BT-TC-3:no_script" {
		t.Fatal(res)
	}
	for _, id := range []string{t1, t4} {
		if s := a.m("GET", "/api/v1/tests/"+id, nil, 200)["status"]; s != "ready" {
			t.Fatal(s)
		}
	}
	res = a.m("POST", path, map[string]any{"decision": "ready", "all": true, "from": []string{"draft", "review"}, "ignore": []string{"no_steps", "no_script"}}, 200)
	if len(res["updated"].([]any)) != 2 {
		t.Fatal(res)
	}
}
