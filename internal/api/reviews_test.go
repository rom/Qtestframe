package api

import (
	"testing"
)

func TestReviewRequestsAndSignoffs(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	rev := e.member("rita@example.com", "reviewer")
	e.member("viewer@example.com", "viewer")
	rc := e.login("rita@example.com")
	v := e.login("viewer@example.com")
	f := qualityFixture(t, a, "REV")

	if cl := a.m("GET", "/api/v1/review-checklists", nil, 200); len(cl["requirement"].([]any)) < 3 {
		t.Fatal(cl)
	}
	a.m("POST", "/api/v1/reviews", map[string]any{"entity_type": "run", "entity_id": f.r1, "reviewers": []string{"@rita"}}, 400)
	a.m("POST", "/api/v1/reviews", map[string]any{"entity_type": "requirement", "entity_id": f.r1, "reviewers": []string{"nobody@example.com"}}, 400)
	a.m("POST", "/api/v1/reviews", map[string]any{"entity_type": "requirement", "entity_id": f.r1, "reviewers": []string{}}, 400)
	a.m("POST", "/api/v1/reviews", map[string]any{"entity_type": "requirement", "entity_id": "missing", "reviewers": []string{"@rita"}}, 404)
	v.m("POST", "/api/v1/reviews", map[string]any{"entity_type": "requirement", "entity_id": f.r1, "reviewers": []string{"@rita"}}, 403)
	res := a.m("POST", "/api/v1/reviews", map[string]any{"entity_type": "requirement", "entity_id": f.r1, "reviewers": []string{"@rita"}, "message": "Please review", "due_date": "2027-02-01"}, 201)
	list := items(res)
	if len(list) != 1 || list[0]["reviewer_id"] != rev.ID || list[0]["entity_key"] != "REV-REQ-1" || len(list[0]["checklist"].([]any)) < 3 {
		t.Fatal(res)
	}
	id := list[0]["id"].(string)
	// a second request to the same person is skipped while one is pending
	if again := items(a.m("POST", "/api/v1/reviews", map[string]any{"entity_type": "requirement", "entity_id": f.r1, "reviewers": []string{rev.ID}}, 201)); len(again) != 0 {
		t.Fatal(again)
	}
	// the reviewer is notified and sees it in "my reviews"
	if n := rc.m("GET", "/api/v1/notifications", nil, 200); n["unread"].(float64) != 1 {
		t.Fatal(n)
	}
	if mine := items(rc.m("GET", "/api/v1/reviews", nil, 200)); len(mine) != 1 {
		t.Fatal(mine)
	}
	if byItem := items(a.m("GET", "/api/v1/reviews?entity_type=requirement&entity_id="+f.r1, nil, 200)); len(byItem) != 1 {
		t.Fatal(byItem)
	}
	// only the reviewer works on it
	a.m("PATCH", "/api/v1/reviews/"+id, map[string]any{"decision": "approved"}, 403)
	// approval needs every item checked or explained
	rc.m("PATCH", "/api/v1/reviews/"+id, map[string]any{"decision": "approved"}, 400)
	rc.m("PATCH", "/api/v1/reviews/"+id, map[string]any{"decision": "changes_requested"}, 400)
	rc.m("PATCH", "/api/v1/reviews/"+id, map[string]any{"checklist": []map[string]any{{"checked": true}}}, 400)
	rc.m("PATCH", "/api/v1/reviews/"+id, map[string]any{"decision": "maybe"}, 400)
	cl := []map[string]any{}
	for i := range list[0]["checklist"].([]any) {
		if i == 0 {
			cl = append(cl, map[string]any{"checked": false, "note": "Not applicable"})
		} else {
			cl = append(cl, map[string]any{"checked": true})
		}
	}
	done := rc.m("PATCH", "/api/v1/reviews/"+id, map[string]any{"checklist": cl, "comment": "Fine", "decision": "approved"}, 200)
	if done["status"] != "approved" || done["decided_at"] == nil || done["outdated"] != false {
		t.Fatal(done)
	}
	rc.m("PATCH", "/api/v1/reviews/"+id, map[string]any{"decision": "approved"}, 409)
	if n := a.m("GET", "/api/v1/notifications", nil, 200); n["unread"].(float64) != 1 || items(n)[0]["event"] != "review.completed" {
		t.Fatal(n)
	}
	// a changed item makes older reviews outdated; the requester can cancel
	a.m("PATCH", "/api/v1/requirements/"+f.r1, map[string]any{"description": "The user MUST log in with 2FA."}, 200)
	if got := rc.m("GET", "/api/v1/reviews/"+id, nil, 200); got["outdated"] != true {
		t.Fatal(got)
	}
	x := items(a.m("POST", "/api/v1/reviews", map[string]any{"entity_type": "test", "entity_id": f.t1, "reviewers": []string{"rita@example.com"}}, 201))[0]
	rc.m("PATCH", "/api/v1/reviews/"+x["id"].(string), map[string]any{"cancel": true}, 403)
	if c := a.m("PATCH", "/api/v1/reviews/"+x["id"].(string), map[string]any{"cancel": true}, 200); c["status"] != "cancelled" {
		t.Fatal(c)
	}
	v.m("GET", "/api/v1/reviews/"+id, nil, 200)

	// sign-offs on a baseline
	b := a.m("POST", "/api/v1/projects/"+f.pid+"/baselines", map[string]any{"name": "B1"}, 201)
	bid := b["id"].(string)
	v.m("POST", "/api/v1/baselines/"+bid+"/signoffs", map[string]any{"meaning": "approved", "password": testPassword}, 403)
	rc.m("POST", "/api/v1/baselines/"+bid+"/signoffs", map[string]any{"meaning": "approved", "password": "wrong"}, 403)
	rc.m("POST", "/api/v1/baselines/"+bid+"/signoffs", map[string]any{"meaning": "liked", "password": testPassword}, 400)
	so := rc.m("POST", "/api/v1/baselines/"+bid+"/signoffs", map[string]any{"meaning": "approved", "comment": "OK", "password": testPassword}, 201)
	if so["user_email"] != "rita@example.com" || len(so["content_hash"].(string)) != 64 || so["valid"] != true {
		t.Fatal(so)
	}
	rc.m("POST", "/api/v1/baselines/"+bid+"/signoffs", map[string]any{"meaning": "approved", "password": testPassword}, 409)
	if l := items(v.m("GET", "/api/v1/baselines/"+bid+"/signoffs", nil, 200)); len(l) != 1 || l[0]["valid"] != true {
		t.Fatal(l)
	}
	a.m("DELETE", "/api/v1/baselines/"+bid, nil, 409)
	// an API token cannot sign
	tok := a.m("POST", "/api/v1/tokens", map[string]any{"name": "ci"}, 201)
	tc := e.anon()
	tc.bearer = tok["token"].(string)
	tc.m("POST", "/api/v1/baselines/"+bid+"/signoffs", map[string]any{"meaning": "reviewed", "password": testPassword}, 403)
	// deleting the requirement removes its reviews
	a.m("DELETE", "/api/v1/requirements/"+f.r1, nil, 204)
	if l := items(rc.m("GET", "/api/v1/reviews", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
}

func TestBulkReviewRequestActions(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	e.member("rita@example.com", "reviewer")
	e.member("olle@example.com", "reviewer")
	rc := e.login("rita@example.com")
	oc := e.login("olle@example.com")
	f := qualityFixture(t, a, "BRR")
	ask := func(id, kind string, who ...string) []string {
		var out []string
		for _, x := range items(a.m("POST", "/api/v1/reviews", map[string]any{"entity_type": kind, "entity_id": id, "reviewers": who}, 201)) {
			out = append(out, x["id"].(string))
		}
		return out
	}
	r1 := ask(f.r1, "requirement", "rita@example.com")[0]
	r2 := ask(f.r2, "requirement", "rita@example.com")[0]
	t1 := ask(f.t1, "test", "rita@example.com")[0]
	o1 := ask(f.r1, "requirement", "olle@example.com")[0]
	path := "/api/v1/reviews/bulk"
	rc.m("POST", path, map[string]any{"ids": []string{r1}, "action": "nope"}, 400)
	rc.m("POST", path, map[string]any{"ids": []string{r1}, "action": "request_changes"}, 400)
	rc.m("POST", path, map[string]any{"ids": []string{r1}, "action": "due_date", "due_date": "soon"}, 400)

	// the requester's list
	if l := items(a.m("GET", "/api/v1/reviews?requested_by=me&status=pending", nil, 200)); len(l) != 4 {
		t.Fatal(l)
	}
	// Rita approves her two requirement reviews and Olle's is skipped
	res := rc.m("POST", path, map[string]any{"ids": []string{r1, r2, o1}, "action": "approve", "comment": "Meeting 3 May"}, 200)
	if len(res["updated"].([]any)) != 2 || len(res["skipped"].([]any)) != 1 || res["skipped"].([]any)[0].(map[string]any)["reason"] != "not_reviewer" {
		t.Fatal(res)
	}
	if v := rc.m("GET", "/api/v1/reviews/"+r1, nil, 200); v["status"] != "approved" || v["comment"] != "Approved in a bulk review. Meeting 3 May" {
		t.Fatal(v)
	}
	// already decided ones are skipped
	res = rc.m("POST", path, map[string]any{"ids": []string{r1, t1}, "action": "request_changes", "comment": "Add test data"}, 200)
	if len(res["updated"].([]any)) != 1 || res["skipped"].([]any)[0].(map[string]any)["reason"] != "not_pending" {
		t.Fatal(res)
	}
	if v := rc.m("GET", "/api/v1/reviews/"+t1, nil, 200); v["status"] != "changes_requested" {
		t.Fatal(v)
	}
	// the requester: remind Olle, change the due date, cancel; reviewers cannot
	oc.m("POST", path, map[string]any{"ids": []string{o1}, "action": "cancel"}, 200)
	if v := a.m("GET", "/api/v1/reviews/"+o1, nil, 200); v["status"] != "pending" {
		t.Fatal("reviewer cancelled", v)
	}
	before := oc.m("GET", "/api/v1/notifications", nil, 200)["unread"].(float64)
	a.m("POST", path, map[string]any{"ids": []string{o1}, "action": "remind", "comment": "Due Friday"}, 200)
	n := oc.m("GET", "/api/v1/notifications", nil, 200)
	if n["unread"].(float64) != before+1 || items(n)[0]["event"] != "review.reminder" {
		t.Fatal(n)
	}
	a.m("POST", path, map[string]any{"ids": []string{o1}, "action": "due_date", "due_date": "2027-05-01"}, 200)
	if v := a.m("GET", "/api/v1/reviews/"+o1, nil, 200); v["due_date"] != "2027-05-01" {
		t.Fatal(v)
	}
	res = a.m("POST", path, map[string]any{"ids": []string{o1, r1}, "action": "cancel"}, 200)
	if len(res["updated"].([]any)) != 1 || len(res["skipped"].([]any)) != 1 {
		t.Fatal(res)
	}
	if v := a.m("GET", "/api/v1/reviews/"+o1, nil, 200); v["status"] != "cancelled" {
		t.Fatal(v)
	}
	a.m("POST", path, map[string]any{"ids": []string{"missing"}, "action": "cancel"}, 404)
}
