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

	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

func TestDiscussionThreads(t *testing.T) {
	e, mails := mailEnv(t)
	var mu sync.Mutex
	var events []string
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		mu.Lock()
		events = append(events, r.Header.Get("X-Qtestframe-Event"))
		mu.Unlock()
		w.WriteHeader(204)
	}))
	defer hook.Close()
	a := e.login("admin@example.com")
	pid := a.project("DSC")
	a.m("POST", "/api/v1/projects/"+pid+"/webhooks", map[string]any{"name": "C", "url": hook.URL, "events": []string{"comment.created", "comment.mentioned", "comment.resolved"}, "enabled": true}, 201)
	anna := e.member("anna.berg@example.com", rbac.RoleTester)
	e.member("bo@example.com", rbac.RoleTester)
	e.member("vic@example.com", rbac.RoleViewer)
	an := e.login("anna.berg@example.com")
	setPrefs(t, an, map[string]any{"email_notifications": "immediate"})
	bo := e.login("bo@example.com")
	vic := e.login("vic@example.com")
	rid := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "owner": "bo@example.com"}, 201)["id"].(string)
	tid := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "T"}, 201)["id"].(string)

	// a new thread mentioning anna: she gets an in-app notification and an
	// e-mail, the owner (bo) hears about the new thread
	root := a.m("POST", "/api/v1/requirements/"+rid+"/comments", map[string]any{"body": "**Is** this testable, @anna.berg?"}, 201)
	rootID := root["id"].(string)
	if ms := root["mentions"].([]any); len(ms) != 1 || ms[0] != anna.ID || root["parent_id"] != "" || root["project_id"] != pid {
		t.Fatal(root)
	}
	ns := items(an.m("GET", "/api/v1/notifications", nil, 200))
	if len(ns) != 1 || ns[0]["event"] != "mention" || ns[0]["entity_type"] != "requirement" {
		t.Fatal(ns)
	}
	if ns := items(bo.m("GET", "/api/v1/notifications", nil, 200)); len(ns) != 2 || ns[0]["event"] != "comment.created" { // and "assigned"
		t.Fatal(ns)
	}
	if ms := sent(e, mails); len(ms) != 1 || ms[0].To[0].Email != "anna.berg@example.com" {
		t.Fatal(ms)
	}
	// replies: a reply to a reply joins the thread; the others are told
	r1 := an.m("POST", "/api/v1/requirements/"+rid+"/comments", map[string]any{"body": "Yes", "parent_id": rootID}, 201)
	r2 := bo.m("POST", "/api/v1/requirements/"+rid+"/comments", map[string]any{"body": "Agreed", "parent_id": r1["id"]}, 201)
	if r2["parent_id"] != rootID {
		t.Fatal(r2)
	}
	a.m("POST", "/api/v1/requirements/"+rid+"/comments", map[string]any{"body": "x", "parent_id": "missing"}, 404)
	other := a.m("POST", "/api/v1/tests/"+tid+"/comments", map[string]any{"body": "on a test"}, 201)
	a.m("POST", "/api/v1/requirements/"+rid+"/comments", map[string]any{"body": "x", "parent_id": other["id"]}, 400)
	vic.m("POST", "/api/v1/requirements/"+rid+"/comments", map[string]any{"body": "x"}, 403)
	if l := items(vic.m("GET", "/api/v1/requirements/"+rid+"/comments", nil, 200)); len(l) != 3 {
		t.Fatal(l)
	}

	// the item and the query language
	if q := a.m("GET", "/api/v1/requirements/"+rid, nil, 200); q["open_discussions"].(float64) != 1 || q["comment_count"].(float64) != 3 {
		t.Fatal(q)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/requirements?q=has:open-discussion", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	if l := items(an.m("GET", "/api/v1/projects/"+pid+"/requirements?q=mentions:me", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	if l := items(bo.m("GET", "/api/v1/projects/"+pid+"/requirements?q=mentions:me", nil, 200)); len(l) != 0 {
		t.Fatal(l)
	}
	// "My work": anna sees the item she is mentioned on; bo (the owner)
	// sees it as owner only
	mw := an.m("GET", "/api/v1/my-work?source=mentions", nil, 200)["items"].([]any)
	if len(mw) != 1 || mw[0].(map[string]any)["id"] != rid || mw[0].(map[string]any)["reasons"].([]any)[0] != "mentioned" {
		t.Fatal(mw)
	}
	if mw := bo.m("GET", "/api/v1/my-work?source=mentions", nil, 200)["items"].([]any); len(mw) != 0 {
		t.Fatal(mw)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/requirements?q=mention:anna.berg", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/tests?q=open_discussions>0", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}

	// project list and personal list
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/discussions", nil, 200)); len(l) != 2 {
		t.Fatal(l)
	}
	th := items(an.m("GET", "/api/v1/projects/"+pid+"/discussions?filter=mentioned", nil, 200))
	if len(th) != 1 || th[0]["replies"].(float64) != 2 || th[0]["entity_title"] != "Login" || th[0]["project_key"] != "DSC" || th[0]["last_by"] == "" {
		t.Fatal(th)
	}
	if l := items(bo.m("GET", "/api/v1/projects/"+pid+"/discussions?filter=mine", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	if l := items(an.m("GET", "/api/v1/discussions", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/discussions?q=TESTABLE&type=requirement", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	a.m("GET", "/api/v1/projects/"+pid+"/discussions?status=bogus", nil, 400)
	a.m("GET", "/api/v1/projects/"+pid+"/discussions?filter=bogus", nil, 400)
	a.m("GET", "/api/v1/projects/"+pid+"/discussions?type=bogus", nil, 400)
	if list, err := e.st.OpenThreadsMentioning(context.Background(), e.tenant.ID, anna.ID, nil); err != nil || len(list) != 1 {
		t.Fatal(list, err)
	}

	// edit: only the author; newly mentioned people are told
	a.m("PATCH", "/api/v1/comments/"+r1["id"].(string), map[string]any{"body": "hijack"}, 403)
	if u := bo.m("PATCH", "/api/v1/comments/"+r2["id"].(string), map[string]any{"body": "Agreed, @anna.berg"}, 200); len(u["mentions"].([]any)) != 1 {
		t.Fatal(u)
	}
	// reactions
	if res := bo.m("POST", "/api/v1/comments/"+rootID+"/reactions", map[string]any{"reaction": "+1"}, 200); len(res["reactions"].(map[string]any)["+1"].([]any)) != 1 {
		t.Fatal(res)
	}
	if res := bo.m("POST", "/api/v1/comments/"+rootID+"/reactions", map[string]any{"reaction": "+1"}, 200); len(res["reactions"].(map[string]any)) != 0 {
		t.Fatal(res)
	}
	bo.m("POST", "/api/v1/comments/"+rootID+"/reactions", map[string]any{"reaction": "bogus"}, 400)
	vic.m("POST", "/api/v1/comments/"+rootID+"/reactions", map[string]any{"reaction": "+1"}, 403)

	// resolve (from a reply) and reopen
	if rc := an.m("POST", "/api/v1/comments/"+r1["id"].(string)+"/resolve", nil, 200); rc["id"] != rootID || rc["resolved_at"] == nil || rc["resolved_by"] != anna.ID {
		t.Fatal(rc)
	}
	if q := a.m("GET", "/api/v1/requirements/"+rid, nil, 200); q["open_discussions"] != nil {
		t.Fatal(q)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/discussions?status=resolved", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	if ns := items(bo.m("GET", "/api/v1/notifications", nil, 200)); ns[0]["event"] != "comment.resolved" {
		t.Fatal(ns)
	}
	vic.m("POST", "/api/v1/comments/"+rootID+"/reopen", nil, 403)
	if rc := a.m("POST", "/api/v1/comments/"+rootID+"/reopen", nil, 200); rc["resolved_at"] != nil {
		t.Fatal(rc)
	}

	// remove: own comment, or a moderator; it stays as removed
	bo.m("DELETE", "/api/v1/comments/"+r1["id"].(string), nil, 403)
	an.call("DELETE", "/api/v1/comments/"+r1["id"].(string), nil, 204, nil)
	a.call("DELETE", "/api/v1/comments/"+r2["id"].(string), nil, 204, nil) // moderator
	a.m("DELETE", "/api/v1/comments/"+r2["id"].(string), nil, 404)
	an.m("PATCH", "/api/v1/comments/"+r1["id"].(string), map[string]any{"body": "again"}, 409)
	list := items(a.m("GET", "/api/v1/requirements/"+rid+"/comments", nil, 200))
	if len(list) != 3 || list[1]["deleted"] != true || list[1]["body"] != "" || len(list[1]["mentions"].([]any)) != 0 {
		t.Fatal(list)
	}
	if q := a.m("GET", "/api/v1/requirements/"+rid, nil, 200); q["comment_count"].(float64) != 1 {
		t.Fatal(q)
	}
	audit := items(a.m("GET", "/api/v1/audit?limit=200", nil, 200))
	actions := map[string]int{}
	moderated := false
	for _, x := range audit {
		actions[x["action"].(string)]++
		if x["action"] == "comment.delete" && strings.Contains(x["details"].(string), "moderated") {
			moderated = true
		}
	}
	if actions["comment.create"] != 4 || actions["comment.update"] != 1 || actions["comment.delete"] != 2 || actions["comment.resolve"] != 1 || actions["comment.reopen"] != 1 || !moderated {
		t.Fatal(actions, moderated)
	}
	e.srv.WaitWebhooks()
	mu.Lock()
	count := map[string]int{}
	for _, ev := range events {
		count[ev]++
	}
	mu.Unlock()
	if count["comment.created"] != 4 || count["comment.mentioned"] != 2 || count["comment.resolved"] != 1 {
		t.Fatal(count)
	}
}

func TestDiscussionsOnOtherItems(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("DOT")
	rid := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Spec", "description": "The system shall work."}, 201)["id"].(string)
	tid := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Works"}, 201)["id"].(string)
	did := a.m("POST", "/api/v1/projects/"+pid+"/defects", map[string]any{"title": "Crash"}, 201)["id"].(string)
	kid := a.m("POST", "/api/v1/projects/"+pid+"/risks", map[string]any{"title": "Outage"}, 201)["id"].(string)
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "Nightly", "test_ids": []string{tid}}, 201)["id"].(string)
	rev := e.member("rita@example.com", rbac.RoleReviewer)
	e.member("ola@example.com", rbac.RoleTester)
	rv := items(a.m("POST", "/api/v1/reviews", map[string]any{"entity_type": "requirement", "entity_id": rid, "reviewers": []string{rev.ID}}, 201))[0]["id"].(string)
	for path, id := range map[string]string{"defects": did, "risks": kid, "runs": run, "reviews": rv} {
		c := a.m("POST", "/api/v1/"+path+"/"+id+"/comments", map[string]any{"body": "About " + path}, 201)
		if l := items(a.m("GET", "/api/v1/"+path+"/"+id+"/comments", nil, 200)); len(l) != 1 || l[0]["id"] != c["id"] {
			t.Fatal(path, l)
		}
		a.m("GET", "/api/v1/"+path+"/missing/comments", nil, 404)
	}
	// the reviewer takes part in the review's discussion
	rc := e.login("rita@example.com")
	rc.m("POST", "/api/v1/reviews/"+rv+"/comments", map[string]any{"body": "Looks fine"}, 201)
	ola := e.login("ola@example.com")
	ola.m("GET", "/api/v1/reviews/"+rv+"/comments", nil, 200)
	th := items(a.m("GET", "/api/v1/projects/"+pid+"/discussions?type=review", nil, 200))
	if len(th) != 2 || th[0]["replies"].(float64) != 0 || th[0]["entity_title"] != "Spec" || th[0]["last_by"] != "rita" {
		t.Fatal(th)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/discussions?status=all", nil, 200)); len(l) != 5 {
		t.Fatal(l)
	}

	// a generated document with the discussions of its requirements
	a.m("POST", "/api/v1/requirements/"+rid+"/comments", map[string]any{"body": "Which system?"}, 201)
	f := a.m("POST", "/api/v1/projects/"+pid+"/documents/generate", map[string]any{"kind": "requirements_spec", "format": "md", "options": map[string]any{"include_discussions": true}}, 201)
	if md := a.fileContent(f["id"].(string)); !strings.Contains(md, "Discussions") || !strings.Contains(md, "Which system?") {
		t.Fatal(md)
	}
	f = a.m("POST", "/api/v1/projects/"+pid+"/documents/generate", map[string]any{"kind": "requirements_spec", "format": "md"}, 201)
	if md := a.fileContent(f["id"].(string)); strings.Contains(md, "Which system?") {
		t.Fatal(md)
	}

	// deleting the items removes their discussions
	a.call("DELETE", "/api/v1/defects/"+did, nil, 204, nil)
	a.call("DELETE", "/api/v1/risks/"+kid, nil, 204, nil)
	a.call("DELETE", "/api/v1/runs/"+run, nil, 204, nil)
	var n int
	_ = e.st.DB.QueryRowContext(context.Background(), e.st.DB.Rebind(`SELECT COUNT(*) FROM comments WHERE entity_type IN ('defect', 'risk', 'run')`)).Scan(&n)
	if n != 0 {
		t.Fatal(n)
	}
	// trash and restore of a requirement keeps its discussion
	res := a.m("DELETE", "/api/v1/projects/"+pid+"/requirements/bulk", map[string]any{"ids": []string{rid}}, 200)
	// (the review request goes with the requirement, and its discussion too)
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/discussions?status=all", nil, 200)); len(l) != 0 {
		t.Fatal(l)
	}
	a.m("POST", "/api/v1/trash/"+res["trash_id"].(string)+"/restore", nil, 200)
	if l := items(a.m("GET", "/api/v1/requirements/"+rid+"/comments", nil, 200)); len(l) != 1 || l[0]["body"] != "Which system?" {
		t.Fatal(l)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/discussions", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	// deleting the project removes all of them
	a.call("DELETE", "/api/v1/projects/"+pid, nil, 204, nil)
	_ = e.st.DB.QueryRowContext(context.Background(), e.st.DB.Rebind(`SELECT COUNT(*) FROM comments`)).Scan(&n)
	if n != 0 {
		t.Fatal(n)
	}
	if _, err := e.st.ListDiscussionThreads(context.Background(), e.tenant.ID, store.DiscussionFilter{ProjectIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
}
