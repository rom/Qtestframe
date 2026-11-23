package api

import (
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func TestReviewerRole(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("REV")
	e.member("rev@example.com", rbac.RoleReviewer)
	e.member("tester@example.com", rbac.RoleTester)
	e.member("lead@example.com", rbac.RoleTestLead)
	rq := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "status": "review", "create_test": false}, 201)
	tc := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Valid login", "status": "review"}, 201)
	rid, tid := rq["id"].(string), tc["id"].(string)

	rev := e.login("rev@example.com")
	// a reviewer cannot edit content, but can decide on the status
	rev.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"title": "x"}, 403)
	rev.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"status": "approved", "title": "x"}, 403)
	rev.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"status": "implemented"}, 403) // not a review decision
	if got := rev.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"status": "approved"}, 200); got["status"] != "approved" {
		t.Fatal(got)
	}
	rev.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"status": "rejected"}, 200)
	rev.m("PATCH", "/api/v1/tests/"+tid, map[string]any{"status": "ready"}, 200)
	rev.m("PATCH", "/api/v1/tests/"+tid, map[string]any{"status": "draft"}, 403)
	rev.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "y"}, 403)

	// a tester writes tests but cannot mark them ready
	ts := e.login("tester@example.com")
	res := ts.m("PATCH", "/api/v1/tests/"+tid, map[string]any{"status": "draft"}, 200)
	if res["status"] != "draft" {
		t.Fatal(res)
	}
	if msg := ts.m("PATCH", "/api/v1/tests/"+tid, map[string]any{"status": "ready"}, 403)["error"].(string); !strings.Contains(msg, "test.review") {
		t.Fatal(msg)
	}
	ts.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "z", "status": "ready"}, 403)
	ts.m("PATCH", "/api/v1/tests/"+tid, map[string]any{"title": "Valid login v2"}, 200) // other edits keep working
	ts.m("POST", "/api/v1/projects/"+pid+"/tests/bulk", map[string]any{"ids": []string{tid}, "set": map[string]any{"status": "ready"}}, 403)
	ts.m("POST", "/api/v1/projects/"+pid+"/tests/bulk", map[string]any{"ids": []string{tid}, "set": map[string]any{"priority": "high"}}, 200)

	// a test lead reviews tests, not requirements
	lead := e.login("lead@example.com")
	lead.m("POST", "/api/v1/projects/"+pid+"/tests/bulk", map[string]any{"ids": []string{tid}, "set": map[string]any{"status": "ready"}}, 200)
	lead.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"status": "approved"}, 403)
	rev.m("POST", "/api/v1/projects/"+pid+"/requirements/bulk", map[string]any{"ids": []string{rid}, "set": map[string]any{"status": "approved"}}, 200)
	rev.m("POST", "/api/v1/projects/"+pid+"/requirements/bulk", map[string]any{"ids": []string{rid}, "set": map[string]any{"status": "approved", "priority": "low"}}, 403)
	rev.m("POST", "/api/v1/projects/"+pid+"/requirements/bulk", map[string]any{"ids": []string{rid}, "set": map[string]any{"priority": "low"}}, 403)

	// the decision is audited
	audit := items(a.m("GET", "/api/v1/audit?q=action:requirement.review", nil, 200))
	if len(audit) < 2 || !strings.Contains(audit[0]["details"].(string), "→") {
		t.Fatal(audit)
	}
	// restoring an approved version is a review decision too
	e.member("writer@example.com", rbac.RoleManager)
	a.m("PUT", "/api/v1/roles/manager", map[string]any{"permissions": []string{"project.read", "requirement.read", "requirement.write"}}, 200)
	w := e.login("writer@example.com")
	w.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"status": "draft", "title": "Login v2"}, 200)
	hist := items(w.m("GET", "/api/v1/requirements/"+rid+"/history", nil, 200))
	var approvedRev string
	for _, h := range hist {
		if snap, ok := h["snapshot"].(map[string]any); ok && snap["status"] == "approved" {
			approvedRev = h["id"].(string)
		}
	}
	if approvedRev == "" {
		t.Fatal("no approved revision")
	}
	w.m("POST", "/api/v1/requirements/"+rid+"/restore", map[string]any{"revision_id": approvedRev}, 403)
	a.m("POST", "/api/v1/requirements/"+rid+"/restore", map[string]any{"revision_id": approvedRev}, 200)
}

func TestCopyRequirement(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("CPY")
	src := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "description": "MUST log in", "status": "approved",
		"tags": []string{"auth"}, "priority": "high", "create_test": false, "criteria": []map[string]any{{"description": "c1", "method": "test"}}}, 201)
	c := a.m("POST", "/api/v1/requirements/"+src["id"].(string)+"/copy", nil, 201)
	if c["title"] != "Copy of Login" || c["status"] != "draft" || c["priority"] != "high" || c["key"] == src["key"] || len(c["criteria"].([]any)) != 1 {
		t.Fatal(c)
	}
	if c["criteria"].([]any)[0].(map[string]any)["id"] == src["criteria"].([]any)[0].(map[string]any)["id"] {
		t.Error("criteria must be new")
	}
	d := a.m("POST", "/api/v1/requirements/"+src["id"].(string)+"/copy", map[string]any{"title": "Logout", "relation": "derived_from"}, 201)
	if d["title"] != "Logout" || len(d["relations"].([]any)) != 1 || d["relations"].([]any)[0].(map[string]any)["type"] != "derived_from" {
		t.Fatal(d)
	}
	a.m("POST", "/api/v1/requirements/"+src["id"].(string)+"/copy", map[string]any{"relation": "bogus"}, 400)
	a.m("POST", "/api/v1/requirements/missing/copy", nil, 404)
	// a copied test is reviewed again
	tc := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "T", "status": "ready"}, 201)
	if cp := a.m("POST", "/api/v1/tests/"+tc["id"].(string)+"/copy", nil, 201); cp["status"] != "draft" {
		t.Fatal(cp)
	}
}

func TestProjectColorAndSources(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	p := a.m("POST", "/api/v1/projects", map[string]any{"key": "COL", "name": "Colours", "color": "#1E88E5"}, 201)
	if p["color"] != "#1e88e5" || p["custom_sources"] != false || len(p["requirement_sources"].([]any)) < 5 {
		t.Fatal(p)
	}
	a.m("POST", "/api/v1/projects", map[string]any{"key": "BAD", "name": "x", "color": "blue"}, 400)
	pid := p["id"].(string)
	a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"color": "red"}, 400)
	up := a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"color": "", "requirement_sources": []string{" Customer ", "customer", "ISO 27001", ""}}, 200)
	if up["color"] != "" || up["custom_sources"] != true || strings.Join(strs(up["requirement_sources"]), "|") != "Customer|ISO 27001" {
		t.Fatal(up)
	}
	long := make([]string, 51)
	for i := range long {
		long[i] = strings.Repeat("x", i+1)
	}
	a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"requirement_sources": long}, 400)
	a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"requirement_sources": []string{strings.Repeat("x", 101)}}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "a", "source": "Contract", "create_test": false}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "b", "source": "Contract", "create_test": false}, 201)
	src := a.m("GET", "/api/v1/projects/"+pid+"/requirement-sources", nil, 200)
	used := src["used"].([]any)
	if len(used) != 1 || used[0].(map[string]any)["source"] != "Contract" || used[0].(map[string]any)["count"].(float64) != 2 || len(src["configured"].([]any)) != 2 {
		t.Fatal(src)
	}
	// an empty list (or the defaults) returns to the defaults
	back := a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"requirement_sources": []string{}}, 200)
	if back["custom_sources"] != false || len(back["requirement_sources"].([]any)) < 5 {
		t.Fatal(back)
	}
	if got := a.m("GET", "/api/v1/projects/"+pid, nil, 200); got["custom_sources"] != false {
		t.Fatal(got)
	}
}

func TestTrashRestore(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("TRS")
	rq := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "status": "approved", "create_test": false,
		"tags": []string{"auth"}, "criteria": []map[string]any{{"description": "c1", "method": "test"}}}, 201)
	other := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Other", "create_test": false}, 201)
	tc := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Valid login", "steps": []map[string]any{{"action": "a", "expected": "b"}}}, 201)
	caller := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Caller", "steps": []map[string]any{{"call_test_id": tc["id"]}}}, 201)
	rid, tid, oid := rq["id"].(string), tc["id"].(string), other["id"].(string)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": rid, "test_case_id": tid}, 201)
	a.m("POST", "/api/v1/requirements/"+rid+"/relations", map[string]any{"target_id": oid, "type": "depends_on"}, 201)
	a.m("POST", "/api/v1/requirements/"+rid+"/comments", map[string]any{"body": "hello"}, 201)
	f := a.upload("POST", "/api/v1/requirements/"+rid+"/attachments", "file", "spec.txt", "text/plain", []byte("the spec"), nil, 201)
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R1", "test_ids": []string{tid}}, 201)
	_ = run

	// single delete answers 204 with the trash entry in a header
	res := a.raw("DELETE", "/api/v1/requirements/"+rid, nil)
	if res.StatusCode != 204 || res.Header.Get("X-Trash-Id") == "" {
		t.Fatal(res.Status, res.Header)
	}
	trashID := res.Header.Get("X-Trash-Id")
	a.m("GET", "/api/v1/requirements/"+rid, nil, 404)
	list := items(a.m("GET", "/api/v1/projects/"+pid+"/trash", nil, 200))
	if len(list) != 1 || list[0]["kind"] != "requirement" || strs(list[0]["keys"])[0] != rq["key"] {
		t.Fatal(list)
	}
	// a writer without requirement.review cannot bring back an approved requirement
	e.member("writer@example.com", rbac.RoleTester)
	a.m("PUT", "/api/v1/roles/tester", map[string]any{"permissions": []string{"project.read", "requirement.read", "requirement.write", "test.read", "test.write"}}, 200)
	w := e.login("writer@example.com")
	w.m("POST", "/api/v1/trash/"+trashID+"/restore", nil, 403)
	back := a.m("POST", "/api/v1/trash/"+trashID+"/restore", nil, 200)
	if strs(back["item_ids"])[0] != rid {
		t.Fatal(back)
	}
	got := a.m("GET", "/api/v1/requirements/"+rid, nil, 200)
	if got["key"] != rq["key"] || got["status"] != "approved" || len(got["criteria"].([]any)) != 1 || strs(got["tags"])[0] != "auth" || got["relation_count"].(float64) != 1 {
		t.Fatal(got)
	}
	if links := items(a.m("GET", "/api/v1/requirements/"+rid+"/links", nil, 200)); len(links) != 1 {
		t.Fatal(links)
	}
	if cs := items(a.m("GET", "/api/v1/requirements/"+rid+"/comments", nil, 200)); len(cs) != 1 {
		t.Fatal(cs)
	}
	if att := items(a.m("GET", "/api/v1/requirements/"+rid+"/attachments", nil, 200)); len(att) != 1 {
		t.Fatal(att)
	}
	if body := a.raw("GET", "/api/v1/files/"+f["id"].(string)+"/content", nil); body.StatusCode != 200 {
		t.Fatal(body.Status)
	}
	a.m("POST", "/api/v1/trash/"+trashID+"/restore", nil, 404) // used up

	// bulk delete of a test with executions and a caller; the run result and the call come back
	del := a.m("DELETE", "/api/v1/projects/"+pid+"/tests/bulk", map[string]any{"ids": []string{tid}}, 200)
	tr := del["trash_id"].(string)
	if del["deleted"].(float64) != 1 || tr == "" {
		t.Fatal(del)
	}
	if c := a.m("GET", "/api/v1/tests/"+caller["id"].(string), nil, 200); c["steps"].([]any)[0].(map[string]any)["call_test_id"] != nil {
		t.Fatal("call kept", c)
	}
	w.m("POST", "/api/v1/trash/"+tr+"/restore", nil, 200)
	tback := a.m("GET", "/api/v1/tests/"+tid, nil, 200)
	if tback["key"] != tc["key"] || len(tback["steps"].([]any)) != 1 {
		t.Fatal(tback)
	}
	if c := a.m("GET", "/api/v1/tests/"+caller["id"].(string), nil, 200); c["steps"].([]any)[0].(map[string]any)["call_test_id"] != tid {
		t.Fatal("call not restored", c)
	}
	if ex := items(a.m("GET", "/api/v1/tests/"+tid+"/executions", nil, 200)); len(ex) != 1 {
		t.Fatal(ex)
	}
	if links := items(a.m("GET", "/api/v1/tests/"+tid+"/links", nil, 200)); len(links) != 1 {
		t.Fatal(links)
	}

	// a link to an item deleted in the meantime is left out
	d1 := a.raw("DELETE", "/api/v1/tests/"+tid, nil).Header.Get("X-Trash-Id")
	a.m("DELETE", "/api/v1/requirements/"+rid, nil, 204)
	a.m("POST", "/api/v1/trash/"+d1+"/restore", nil, 200)
	if links := items(a.m("GET", "/api/v1/tests/"+tid+"/links", nil, 200)); len(links) != 0 {
		t.Fatal(links)
	}
	// readers without write cannot restore; unknown entries are 404
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	list = items(v.m("GET", "/api/v1/projects/"+pid+"/trash", nil, 200))
	if len(list) != 1 {
		t.Fatal(list)
	}
	v.m("POST", "/api/v1/trash/"+list[0]["id"].(string)+"/restore", nil, 403)
	a.m("POST", "/api/v1/trash/missing/restore", nil, 404)
}

func TestProjectTypeAndPrompts(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("SW")
	if got := a.m("GET", "/api/v1/projects/"+pid, nil, 200); got["type"] != "general" {
		t.Fatal(got)
	}
	body := map[string]any{"kind": "implement"}
	a.m("POST", "/api/v1/projects/"+pid+"/prompts", body, 400) // not a software project
	a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"type": "rocket"}, 400)
	up := a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"type": "software", "tech_context": "  Go and Vue  "}, 200)
	if up["type"] != "software" || up["tech_context"] != "Go and Vue" {
		t.Fatal(up)
	}
	a.m("POST", "/api/v1/projects", map[string]any{"key": "HW", "name": "Hardware", "type": "hardware"}, 201)
	meta := a.m("GET", "/api/v1/meta", nil, 200)
	if len(meta["project_types"].([]any)) != 5 || len(meta["prompt_kinds"].([]any)) != 10 || len(meta["test_types"].([]any)) < 10 {
		t.Fatal(meta)
	}

	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "description": "The user MUST log in.", "create_test": false,
		"criteria": []map[string]any{{"description": "works", "method": "test", "threshold": "2 s"}}}, 201)
	r2 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Accounts", "create_test": false, "tags": []string{"later"}}, 201)
	a.m("POST", "/api/v1/requirements/"+r1["id"].(string)+"/relations", map[string]any{"target_id": r2["id"], "type": "depends_on"}, 201)
	shared := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Log out", "steps": []map[string]any{{"action": "out"}}}, 201)
	tc := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Valid login", "execution_type": "automated", "script_language": "sh", "script": "exit 0",
		"steps": []map[string]any{{"action": "Sign in", "data": "alice", "expected": "ok"}, {"call_test_id": shared["id"]}}}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": r1["id"], "test_case_id": tc["id"]}, 201)
	a.m("POST", "/api/v1/tests/"+tc["id"].(string)+"/dependencies", map[string]any{"depends_on_id": shared["id"]}, 201)

	// all requirements of the project
	res := a.m("POST", "/api/v1/projects/"+pid+"/prompts", body, 200)
	txt := res["prompt"].(string)
	if res["items"].(float64) != 2 || !strings.Contains(txt, "Go and Vue") || !strings.Contains(txt, "depends on SW-REQ-2 Accounts") ||
		!strings.Contains(txt, "is required by SW-REQ-1 Login") || !strings.Contains(txt, "SW-TC-2 Valid login") || !strings.Contains(txt, "2 s") {
		t.Fatal(txt)
	}
	// chosen by ID, by query, in Swedish
	res = a.m("POST", "/api/v1/projects/"+pid+"/prompts", map[string]any{"kind": "review_requirements", "lang": "sv", "requirement_ids": []string{r1["id"].(string)}}, 200)
	if res["items"].(float64) != 1 || !strings.Contains(res["title"].(string), "Granska") {
		t.Fatal(res)
	}
	res = a.m("POST", "/api/v1/projects/"+pid+"/prompts", map[string]any{"kind": "design_tests", "test_type": "unit", "q": "tag:later"}, 200)
	if res["items"].(float64) != 1 || !strings.Contains(res["prompt"].(string), "SW-REQ-2") {
		t.Fatal(res)
	}
	// tests: automate includes the script, calls and dependencies; strategy adds verified requirements
	res = a.m("POST", "/api/v1/projects/"+pid+"/prompts", map[string]any{"kind": "automate_tests", "framework": "bats", "test_ids": []string{tc["id"].(string)}}, 200)
	txt = res["prompt"].(string)
	if !strings.Contains(txt, "exit 0") || !strings.Contains(txt, "SW-TC-1 Log out") || !strings.Contains(txt, "bats") || !strings.Contains(txt, "SW-REQ-1 Login") {
		t.Fatal(txt)
	}
	res = a.m("POST", "/api/v1/projects/"+pid+"/prompts", map[string]any{"kind": "test_strategy", "test_type": "security", "test_ids": []string{tc["id"].(string)}}, 200)
	if res["items"].(float64) != 2 {
		t.Fatal(res)
	}
	// validation
	a.m("POST", "/api/v1/projects/"+pid+"/prompts", map[string]any{"kind": "design_tests"}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/prompts", map[string]any{"kind": "implement", "requirement_ids": []string{"elsewhere"}}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/prompts", map[string]any{"kind": "implement", "q": "((("}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/prompts", map[string]any{"kind": "implement", "notes": strings.Repeat("x", 8001)}, 400)
	// permissions: a role without test.read cannot build test prompts
	e.member("req@example.com", rbac.RoleViewer)
	a.m("PUT", "/api/v1/roles/viewer", map[string]any{"permissions": []string{"project.read", "requirement.read"}}, 200)
	v := e.login("req@example.com")
	v.m("POST", "/api/v1/projects/"+pid+"/prompts", map[string]any{"kind": "automate_tests"}, 403)
	if res := v.m("POST", "/api/v1/projects/"+pid+"/prompts", body, 200); strings.Contains(res["prompt"].(string), "SW-TC-2") {
		t.Fatal("test keys leak without test.read")
	}
}

func TestRunCompletesWithLastResult(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("RUN")
	a.m("PUT", "/api/v1/settings", map[string]any{"allow_unready_tests": true, "allow_unready_requirements": true}, 200)
	t1 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Only test", "status": "ready"}, 201)
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "One", "test_ids": []string{t1["id"].(string)}}, 201)
	rid := run["id"].(string)
	ex := items(a.m("GET", "/api/v1/runs/"+rid+"/executions", nil, 200))[0]["id"].(string)
	status := func() string { return a.m("GET", "/api/v1/runs/"+rid, nil, 200)["status"].(string) }

	// a run with a single test completes with its result
	a.m("PATCH", "/api/v1/executions/"+ex, map[string]any{"status": "in_progress"}, 200)
	if s := status(); s != "in_progress" {
		t.Fatal(s)
	}
	a.m("PATCH", "/api/v1/executions/"+ex, map[string]any{"status": "passed"}, 200)
	if r := a.m("GET", "/api/v1/runs/"+rid, nil, 200); r["status"] != "completed" || r["completed_at"] == nil {
		t.Fatal(r)
	}
	// notes on a finished execution keep it completed; taking the result back reopens it
	a.m("PATCH", "/api/v1/executions/"+ex, map[string]any{"notes": "fine"}, 200)
	if s := status(); s != "completed" {
		t.Fatal(s)
	}
	a.m("PATCH", "/api/v1/executions/"+ex, map[string]any{"status": "not_run"}, 200)
	if r := a.m("GET", "/api/v1/runs/"+rid, nil, 200); r["status"] != "in_progress" || r["completed_at"] != nil {
		t.Fatal(r)
	}
	// blocked and skipped also finish a run; adding a test reopens it
	a.m("PATCH", "/api/v1/executions/"+ex, map[string]any{"status": "skipped"}, 200)
	if s := status(); s != "completed" {
		t.Fatal(s)
	}
	t2 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Second", "status": "ready"}, 201)
	a.m("POST", "/api/v1/runs/"+rid+"/executions", map[string]any{"test_ids": []string{t2["id"].(string)}}, 200)
	if s := status(); s != "in_progress" {
		t.Fatal(s)
	}
	// a run completed by hand with open tests stays completed while they are worked on
	a.m("PATCH", "/api/v1/runs/"+rid, map[string]any{"status": "completed"}, 200)
	ex2 := ""
	for _, x := range items(a.m("GET", "/api/v1/runs/"+rid+"/executions", nil, 200)) {
		if x["test_case_id"] == t2["id"] {
			ex2 = x["id"].(string)
		}
	}
	a.m("PATCH", "/api/v1/executions/"+ex2, map[string]any{"status": "in_progress"}, 200)
	if s := status(); s != "completed" {
		t.Fatal(s)
	}
	// aborted runs are left alone
	a.m("PATCH", "/api/v1/runs/"+rid, map[string]any{"status": "aborted"}, 200)
	a.m("PATCH", "/api/v1/executions/"+ex2, map[string]any{"status": "failed"}, 200)
	if s := status(); s != "aborted" {
		t.Fatal(s)
	}
	// CI results complete a run too
	run2 := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "CI", "test_ids": []string{t1["id"].(string)}}, 201)
	a.m("POST", "/api/v1/runs/"+run2["id"].(string)+"/results", map[string]any{"results": []map[string]any{{"test_key": t1["key"], "status": "passed"}}}, 200)
	if r := a.m("GET", "/api/v1/runs/"+run2["id"].(string), nil, 200); r["status"] != "completed" {
		t.Fatal(r)
	}
}
