package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func strs(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func TestRevisionHistoryAndRestore(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("HIS")
	r := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "description": "MUST log in", "criteria": []map[string]any{{"description": "c1"}}}, 201)
	id := r["id"].(string)
	a.m("PATCH", "/api/v1/requirements/"+id, map[string]any{"title": "Login v2", "status": "review"}, 200)
	a.m("PATCH", "/api/v1/requirements/"+id, map[string]any{"criteria": []any{}}, 200)
	hist := items(a.m("GET", "/api/v1/requirements/"+id+"/history", nil, 200))
	if len(hist) != 3 || hist[0]["version"].(float64) != 3 || hist[2]["action"] != "create" {
		t.Fatal(hist)
	}
	if hist[0]["user_avatar"] != "" || hist[0]["user_email"] != "admin@example.com" {
		t.Fatal(hist[0])
	}
	a.m("PATCH", "/api/v1/auth/me", map[string]any{"avatar": "preset:fox"}, 200)
	hist = items(a.m("GET", "/api/v1/requirements/"+id+"/history", nil, 200))
	if hist[0]["user_avatar"] != "preset:fox" {
		t.Fatal(hist[0])
	}
	audit := items(a.m("GET", "/api/v1/audit", nil, 200))
	if len(audit) == 0 || audit[0]["user_avatar"] != "preset:fox" {
		t.Fatal(audit)
	}
	if got := strs(hist[1]["changed_fields"]); strings.Join(got, ",") != "status,title" {
		t.Fatal(got)
	}
	if got := strs(hist[0]["changed_fields"]); strings.Join(got, ",") != "criteria" {
		t.Fatal(got)
	}
	// restore version 1 → title, status and criteria come back as version 4
	restored := a.m("POST", "/api/v1/requirements/"+id+"/restore", map[string]any{"revision_id": hist[2]["id"]}, 200)
	if restored["title"] != "Login" || restored["status"] != "draft" || len(restored["criteria"].([]any)) != 1 || restored["version"].(float64) != 4 {
		t.Fatal(restored)
	}
	hist = items(a.m("GET", "/api/v1/requirements/"+id+"/history", nil, 200))
	if hist[0]["action"] != "restore" {
		t.Fatal(hist[0])
	}
	a.m("POST", "/api/v1/requirements/"+id+"/restore", map[string]any{"revision_id": "missing"}, 404)
	a.m("POST", "/api/v1/requirements/missing/restore", map[string]any{"revision_id": "x"}, 404)
	a.m("GET", "/api/v1/requirements/missing/history", nil, 404)

	// tests: versions, history, restore of steps
	tc := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "T", "steps": []map[string]any{{"action": "a1"}, {"action": "a2"}}}, 201)
	tid := tc["id"].(string)
	if tc["version"].(float64) != 1 {
		t.Fatal(tc)
	}
	up := a.m("PATCH", "/api/v1/tests/"+tid, map[string]any{"steps": []map[string]any{{"action": "only"}}}, 200)
	if up["version"].(float64) != 2 {
		t.Fatal(up)
	}
	// re-saving identical steps (new step IDs) is not reported as a change
	a.m("PATCH", "/api/v1/tests/"+tid, map[string]any{"steps": []map[string]any{{"action": "only"}}}, 200)
	th := items(a.m("GET", "/api/v1/tests/"+tid+"/history", nil, 200))
	if len(th) != 3 || len(strs(th[0]["changed_fields"])) != 0 || strings.Join(strs(th[1]["changed_fields"]), ",") != "steps" {
		t.Fatal(th)
	}
	rt := a.m("POST", "/api/v1/tests/"+tid+"/restore", map[string]any{"revision_id": th[2]["id"]}, 200)
	if len(rt["steps"].([]any)) != 2 {
		t.Fatal(rt)
	}
	a.m("POST", "/api/v1/tests/"+tid+"/restore", map[string]any{"revision_id": "nope"}, 404)
	a.m("POST", "/api/v1/tests/"+tid+"/restore", "{", 400)
	a.m("POST", "/api/v1/requirements/"+id+"/restore", "{", 400)
	a.m("POST", "/api/v1/tests/missing/restore", map[string]any{"revision_id": "x"}, 404)

	// bulk and move record revisions; items without history get an "import" revision first
	other := a.project("HIS2")
	legacy := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "legacy"}, 201)
	_, _ = e.st.DB.ExecContext(context.Background(), e.st.DB.Rebind(`DELETE FROM revisions WHERE entity_id = ?`), legacy["id"])
	a.m("POST", "/api/v1/projects/"+pid+"/requirements/bulk", map[string]any{"ids": []string{legacy["id"].(string)}, "set": map[string]any{"priority": "high"}}, 200)
	lh := items(a.m("GET", "/api/v1/requirements/"+legacy["id"].(string)+"/history", nil, 200))
	if len(lh) != 2 || lh[1]["action"] != "import" || lh[0]["action"] != "bulk_update" || strings.Join(strs(lh[0]["changed_fields"]), ",") != "priority" {
		t.Fatal(lh)
	}
	a.m("POST", "/api/v1/projects/"+pid+"/tests/move", map[string]any{"ids": []string{tid}, "project_id": other}, 200)
	th = items(a.m("GET", "/api/v1/tests/"+tid+"/history", nil, 200))
	if th[0]["action"] != "move" || !contains(strs(th[0]["changed_fields"]), "key") {
		t.Fatal(th[0])
	}
	// deleting an item removes its comments and history
	a.m("POST", "/api/v1/requirements/"+id+"/comments", map[string]any{"body": "x"}, 201)
	a.call("DELETE", "/api/v1/requirements/"+id, nil, 204, nil)
	var n int
	_ = e.st.DB.QueryRowContext(context.Background(), e.st.DB.Rebind(`SELECT COUNT(*) FROM revisions WHERE entity_id = ?`), id).Scan(&n)
	var c int
	_ = e.st.DB.QueryRowContext(context.Background(), e.st.DB.Rebind(`SELECT COUNT(*) FROM comments WHERE entity_id = ?`), id).Scan(&c)
	if n != 0 || c != 0 {
		t.Fatal(n, c)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestRelations(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("REL")
	mk := func(title string) string {
		return a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": title}, 201)["id"].(string)
	}
	parent, child, grand, other := mk("Parent"), mk("Child"), mk("Grandchild"), mk("Other")
	rel := a.m("POST", "/api/v1/requirements/"+child+"/relations", map[string]any{"target_id": parent, "type": "refines"}, 201)
	if rel["direction"] != "outgoing" || rel["target_key"] != "REL-REQ-1" {
		t.Fatal(rel)
	}
	a.m("POST", "/api/v1/requirements/"+grand+"/relations", map[string]any{"target_id": child, "type": "refines"}, 201)
	a.m("POST", "/api/v1/requirements/"+parent+"/relations", map[string]any{"target_id": grand, "type": "refines"}, 400) // cycle
	a.m("POST", "/api/v1/requirements/"+child+"/relations", map[string]any{"target_id": parent, "type": "refines"}, 409) // duplicate
	a.m("POST", "/api/v1/requirements/"+child+"/relations", map[string]any{"target_id": child, "type": "depends_on"}, 400)
	a.m("POST", "/api/v1/requirements/"+child+"/relations", map[string]any{"target_id": other, "type": "loves"}, 400)
	a.m("POST", "/api/v1/requirements/"+child+"/relations", map[string]any{"target_id": "missing", "type": "relates_to"}, 400)
	a.m("POST", "/api/v1/requirements/missing/relations", map[string]any{"target_id": other, "type": "relates_to"}, 404)
	a.m("POST", "/api/v1/requirements/"+child+"/relations", "{", 400)
	pid2 := a.project("RELB")
	foreign := a.m("POST", "/api/v1/projects/"+pid2+"/requirements", map[string]any{"title": "F"}, 201)["id"]
	a.m("POST", "/api/v1/requirements/"+child+"/relations", map[string]any{"target_id": foreign, "type": "relates_to"}, 400)
	dep := a.m("POST", "/api/v1/requirements/"+other+"/relations", map[string]any{"target_id": child, "type": "depends_on"}, 201)

	rels := items(a.m("GET", "/api/v1/requirements/"+child+"/relations", nil, 200))
	dirs := map[string]int{}
	for _, r := range rels {
		dirs[r["direction"].(string)]++
	}
	if len(rels) != 3 || dirs["incoming"] != 2 || dirs["outgoing"] != 1 {
		t.Fatal(rels)
	}
	if n := len(items(a.m("GET", "/api/v1/projects/"+pid+"/relations", nil, 200))); n != 3 {
		t.Fatal(n)
	}
	list := func(q string) []map[string]any {
		return items(a.m("GET", "/api/v1/projects/"+pid+"/requirements?q="+url.QueryEscape(q), nil, 200))
	}
	if l := list("parent:REL-REQ-1"); len(l) != 1 || l[0]["id"] != child || l[0]["relation_count"].(float64) != 3 {
		t.Fatal(l)
	}
	if l := list("child:*"); len(l) != 2 {
		t.Fatal(l)
	}
	if l := list("related:REL-REQ-2"); len(l) != 3 {
		t.Fatal(l)
	}
	a.m("GET", "/api/v1/requirements/missing/relations", nil, 404)
	a.call("DELETE", "/api/v1/relations/"+dep["id"].(string), nil, 204, nil)
	a.m("DELETE", "/api/v1/relations/"+dep["id"].(string), nil, 404)
	// tester cannot create relations
	e.member("tester@example.com", rbac.RoleTester)
	e.login("tester@example.com").m("POST", "/api/v1/requirements/"+other+"/relations", map[string]any{"target_id": parent, "type": "relates_to"}, 403)
}

func TestBaselines(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BL")
	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "One", "status": "approved"}, 201)["id"].(string)
	r2 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Two"}, 201)["id"].(string)
	t1 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "T1"}, 201)["id"].(string)
	b1 := a.m("POST", "/api/v1/projects/"+pid+"/baselines", map[string]any{"name": "Release 1.0", "description": "first"}, 201)
	if b1["requirement_count"].(float64) != 2 || b1["test_count"].(float64) != 1 {
		t.Fatal(b1)
	}
	a.m("POST", "/api/v1/projects/"+pid+"/baselines", map[string]any{"name": "Release 1.0"}, 409)
	a.m("POST", "/api/v1/projects/"+pid+"/baselines", map[string]any{"name": " "}, 400)
	a.m("POST", "/api/v1/projects/"+pid+"/baselines", map[string]any{"name": "x", "query": "bogus:1"}, 400)
	approved := a.m("POST", "/api/v1/projects/"+pid+"/baselines", map[string]any{"name": "Approved only", "query": "status:approved"}, 201)
	if approved["requirement_count"].(float64) != 1 {
		t.Fatal(approved)
	}
	// change things
	a.m("PATCH", "/api/v1/requirements/"+r1, map[string]any{"title": "One!"}, 200)
	a.call("DELETE", "/api/v1/requirements/"+r2, nil, 204, nil)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Three"}, 201)
	a.m("PATCH", "/api/v1/tests/"+t1, map[string]any{"description": "same title"}, 200)

	d := a.m("GET", "/api/v1/baselines/"+b1["id"].(string)+"/compare?to=current", nil, 200)
	if len(d["added"].([]any)) != 1 || len(d["removed"].([]any)) != 1 || len(d["changed"].([]any)) != 2 || d["unchanged"].(float64) != 0 || d["to"] != "current" {
		t.Fatal(d)
	}
	ch := d["changed"].([]any)[0].(map[string]any)
	if ch["key"] != "BL-REQ-1" || ch["from_version"].(float64) != 1 || ch["to_version"].(float64) != 2 || strings.Join(strs(ch["changed_fields"]), ",") != "title" {
		t.Fatal(ch)
	}
	b2 := a.m("POST", "/api/v1/projects/"+pid+"/baselines", map[string]any{"name": "Release 1.1"}, 201)
	d = a.m("GET", "/api/v1/baselines/"+b1["id"].(string)+"/compare?to="+b2["id"].(string), nil, 200)
	if d["to"] != "Release 1.1" || len(d["changed"].([]any)) != 2 {
		t.Fatal(d)
	}
	if d := a.m("GET", "/api/v1/baselines/"+b2["id"].(string)+"/compare", nil, 200); d["unchanged"].(float64) != 3 {
		t.Fatal(d)
	}
	a.m("GET", "/api/v1/baselines/"+b1["id"].(string)+"/compare?to=missing", nil, 404)
	a.m("GET", "/api/v1/baselines/missing/compare", nil, 404)
	otherPid := a.project("BLX")
	ob := a.m("POST", "/api/v1/projects/"+otherPid+"/baselines", map[string]any{"name": "x"}, 201)
	a.m("GET", "/api/v1/baselines/"+b1["id"].(string)+"/compare?to="+ob["id"].(string), nil, 400)

	full := a.m("GET", "/api/v1/baselines/"+b1["id"].(string), nil, 200)
	its := full["items"].([]any)
	if len(its) != 3 || its[0].(map[string]any)["entity_type"] != "requirement" || its[2].(map[string]any)["entity_type"] != "test" {
		t.Fatal(its)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/baselines", nil, 200)); len(l) != 3 || l[0]["items"] != nil {
		t.Fatal(l)
	}
	a.m("GET", "/api/v1/baselines/missing", nil, 404)
	a.call("DELETE", "/api/v1/baselines/"+b1["id"].(string), nil, 204, nil)
	a.m("DELETE", "/api/v1/baselines/"+b1["id"].(string), nil, 404)
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	v.m("GET", "/api/v1/projects/"+pid+"/baselines", nil, 200)
	v.m("POST", "/api/v1/projects/"+pid+"/baselines", map[string]any{"name": "v"}, 403)
}

func TestComments(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("COM")
	rid := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "R"}, 201)["id"].(string)
	tid := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "T"}, 201)["id"].(string)
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	c := v.m("POST", "/api/v1/requirements/"+rid+"/comments", map[string]any{"body": "  Is this testable?  "}, 201)
	if c["body"] != "Is this testable?" || c["user_email"] != "viewer@example.com" {
		t.Fatal(c)
	}
	a.m("POST", "/api/v1/requirements/"+rid+"/comments", map[string]any{"body": "Yes"}, 201)
	a.m("POST", "/api/v1/tests/"+tid+"/comments", map[string]any{"body": "flaky?"}, 201)
	a.m("POST", "/api/v1/requirements/"+rid+"/comments", map[string]any{"body": ""}, 400)
	a.m("POST", "/api/v1/requirements/"+rid+"/comments", map[string]any{"body": strings.Repeat("x", 10001)}, 400)
	a.m("POST", "/api/v1/requirements/"+rid+"/comments", "{", 400)
	a.m("POST", "/api/v1/requirements/missing/comments", map[string]any{"body": "x"}, 404)
	a.m("GET", "/api/v1/tests/missing/comments", nil, 404)
	list := items(a.m("GET", "/api/v1/requirements/"+rid+"/comments", nil, 200))
	if len(list) != 2 || list[0]["body"] != "Is this testable?" {
		t.Fatal(list)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/requirements?q=comments>=2", nil, 200)); len(l) != 1 || l[0]["comment_count"].(float64) != 2 {
		t.Fatal(l)
	}
	if tc := a.m("GET", "/api/v1/tests/"+tid, nil, 200); tc["comment_count"].(float64) != 1 {
		t.Fatal(tc)
	}
	cid := c["id"].(string)
	a.m("PATCH", "/api/v1/comments/"+cid, map[string]any{"body": "hijack"}, 403) // not the author
	if u := v.m("PATCH", "/api/v1/comments/"+cid, map[string]any{"body": "Edited"}, 200); u["body"] != "Edited" {
		t.Fatal(u)
	}
	v.m("PATCH", "/api/v1/comments/"+cid, map[string]any{"body": ""}, 400)
	v.m("PATCH", "/api/v1/comments/"+cid, "{", 400)
	other := list[1]["id"].(string)
	v.m("DELETE", "/api/v1/comments/"+other, nil, 403)       // viewer cannot delete others' comments
	a.call("DELETE", "/api/v1/comments/"+cid, nil, 204, nil) // admin can
	a.m("DELETE", "/api/v1/comments/"+cid, nil, 404)
	a.m("PATCH", "/api/v1/comments/missing", map[string]any{"body": "x"}, 404)
}

func TestExecutionHistoryDefectsAndSharedSteps(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("SHR")
	a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"defect_url_template": "ftp://x/{id}"}, 400)
	a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"defect_url_template": "https://jira.example.com/browse"}, 400)
	if p := a.m("PATCH", "/api/v1/projects/"+pid, map[string]any{"defect_url_template": " https://jira.example.com/browse/{id} "}, 200); p["defect_url_template"] != "https://jira.example.com/browse/{id}" {
		t.Fatal(p)
	}
	login := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Login", "steps": []map[string]any{{"action": "open login", "expected": "form"}, {"action": "sign in"}}}, 201)
	lid := login["id"].(string)
	checkout := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Checkout", "steps": []map[string]any{
		{"call_test_id": lid}, {"action": "pay", "expected": "receipt"}, {"call_test_id": ""}}}, 201)
	cid := checkout["id"].(string)
	got := a.m("GET", "/api/v1/tests/"+cid, nil, 200)
	steps := got["steps"].([]any)
	if len(steps) != 3 || steps[0].(map[string]any)["call_test_key"] != "SHR-TC-1" || steps[2].(map[string]any)["call_test_id"] != nil {
		t.Fatal(steps)
	}
	if lg := a.m("GET", "/api/v1/tests/"+lid, nil, 200); len(lg["called_by"].([]any)) != 1 {
		t.Fatal(lg)
	}
	// self call, cycle, foreign test
	a.m("PATCH", "/api/v1/tests/"+lid, map[string]any{"steps": []map[string]any{{"call_test_id": lid}}}, 400)
	a.m("PATCH", "/api/v1/tests/"+lid, map[string]any{"steps": []map[string]any{{"call_test_id": cid}}}, 400)
	other := a.project("SHRB")
	ft := a.m("POST", "/api/v1/projects/"+other+"/tests", map[string]any{"title": "F"}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "x", "steps": []map[string]any{{"call_test_id": ft["id"]}}}, 400)
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/tests?q=calls:SHR-TC-1", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	// nesting deeper than the limit is rejected
	prev := lid
	for i := 0; i < 5; i++ {
		prev = a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "nest", "steps": []map[string]any{{"call_test_id": prev}}}, 201)["id"].(string)
	}
	a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "too deep", "steps": []map[string]any{{"call_test_id": prev}}}, 400)

	// a run expands the called steps
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R1", "test_ids": []string{cid}}, 201)
	ex := items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200))[0]
	detail := a.m("GET", "/api/v1/executions/"+ex["id"].(string), nil, 200)
	srs := detail["step_results"].([]any)
	// the call expands to Login's two steps; the blank call_test_id "" is a plain (empty) step
	if len(srs) != 4 || srs[0].(map[string]any)["action"] != "[SHR-TC-1] open login" || srs[1].(map[string]any)["action"] != "[SHR-TC-1] sign in" ||
		srs[2].(map[string]any)["action"] != "pay" || srs[3].(map[string]any)["position"].(float64) != 3 {
		t.Fatal(srs)
	}
	// defects on executions
	upd := a.m("PATCH", "/api/v1/executions/"+ex["id"].(string), map[string]any{"status": "failed", "defects": []string{"JIRA-1", " JIRA-1", "https://bugs/2"}}, 200)
	if strings.Join(strs(upd["defects"]), ",") != "JIRA-1,https://bugs/2" {
		t.Fatal(upd)
	}
	a.m("PATCH", "/api/v1/executions/"+ex["id"].(string), map[string]any{"defects": []string{strings.Repeat("d", 501)}}, 400)
	if l := items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions?q=defect:JIRA-1", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/tests?q=defect:JIRA-1", nil, 200)); len(l) != 1 || l[0]["id"] != cid {
		t.Fatal(l)
	}
	// execution history across runs
	run2 := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R2", "build": "2.0", "test_ids": []string{cid}}, 201)
	hist := items(a.m("GET", "/api/v1/tests/"+cid+"/executions", nil, 200))
	if len(hist) != 2 || hist[0]["run_name"] != "R2" || hist[0]["run_build"] != "2.0" || hist[1]["status"] != "failed" || hist[0]["run_created_at"] == nil {
		t.Fatal(hist)
	}
	_ = run2
	a.m("GET", "/api/v1/tests/missing/executions", nil, 404)
	// export includes defects
	res := a.raw("GET", "/api/v1/projects/"+pid+"/export?format=csv&aspects=runs", nil)
	buf := make([]byte, 4096)
	n, _ := res.Body.Read(buf)
	res.Body.Close()
	if !strings.Contains(string(buf[:n]), "JIRA-1") {
		t.Fatal(string(buf[:n]))
	}
	// deleting the called test turns the call into an empty step
	a.call("DELETE", "/api/v1/tests/"+lid, nil, 204, nil)
	got = a.m("GET", "/api/v1/tests/"+cid, nil, 200)
	if got["steps"].([]any)[0].(map[string]any)["call_test_id"] != nil {
		t.Fatal(got)
	}
}

func TestProfileAndSessions(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	other, _ := e.st.CreateTenant(context.Background(), "second", "Second")
	_ = e.st.SetMembership(context.Background(), other.ID, e.admin.ID, rbac.RoleViewer)

	me := a.m("PATCH", "/api/v1/auth/me", map[string]any{"display_name": " Ada ", "preferences": map[string]any{
		"theme": "dark", "page_size": 50, "date_format": "iso", "timezone": "Europe/Stockholm", "start_page": "last_project",
		"compact_tables": true, "create_linked_test": false, "default_tenant_id": other.ID}}, 200)
	u := me["user"].(map[string]any)
	prefs := u["preferences"].(map[string]any)
	if u["display_name"] != "Ada" || prefs["theme"] != "dark" || prefs["page_size"].(float64) != 50 || prefs["create_linked_test"] != false {
		t.Fatal(u)
	}
	// merge + reset with null
	for _, th := range []string{"spring", "autumn", "desert", "camouflage", "city_camouflage", "navy", "green_console", "crazy"} {
		a.m("PATCH", "/api/v1/auth/me", map[string]any{"preferences": map[string]any{"theme": th}}, 200)
	}
	if me := a.m("PATCH", "/api/v1/auth/me", map[string]any{"preferences": map[string]any{"language": "sv"}}, 200); me["user"].(map[string]any)["preferences"].(map[string]any)["language"] != "sv" {
		t.Fatal(me)
	}
	me = a.m("PATCH", "/api/v1/auth/me", map[string]any{"preferences": map[string]any{"theme": nil, "hide_analysis": true}}, 200)
	prefs = me["user"].(map[string]any)["preferences"].(map[string]any)
	if _, ok := prefs["theme"]; ok || prefs["hide_analysis"] != true || prefs["date_format"] != "iso" {
		t.Fatal(prefs)
	}
	for _, bad := range []map[string]any{
		{"display_name": ""}, {"display_name": strings.Repeat("n", 101)},
		{"preferences": map[string]any{"theme": "pink"}}, {"preferences": map[string]any{"page_size": 7}},
		{"preferences": map[string]any{"date_format": "x"}}, {"preferences": map[string]any{"timezone": "Mars/Base"}},
		{"preferences": map[string]any{"start_page": "x"}}, {"preferences": map[string]any{"unknown": 1}},
		{"preferences": map[string]any{"default_tenant_id": "nope"}}, {"preferences": map[string]any{"default_project_id": strings.Repeat("p", 65)}},
		{"preferences": map[string]any{"page_size": "big"}}, {"preferences": map[string]any{"language": "de"}},
	} {
		a.m("PATCH", "/api/v1/auth/me", bad, 400)
	}
	a.m("PATCH", "/api/v1/auth/me", "{", 400)

	// preferred tenant is selected at the next login
	b := e.login("admin@example.com")
	if me := b.m("GET", "/api/v1/auth/me", nil, 200); me["tenant"].(map[string]any)["id"] != other.ID {
		t.Fatal(me["tenant"])
	}
	// sessions: a, b and one more
	e.login("admin@example.com")
	ss := items(a.m("GET", "/api/v1/auth/sessions", nil, 200))
	if len(ss) != 3 {
		t.Fatal(len(ss))
	}
	cur := 0
	for _, s := range ss {
		if s["current"] == true {
			cur++
		}
		if s["ip"] == "" || s["last_seen"] == nil || !strings.HasPrefix(s["user_agent"].(string), "Go-http-client") {
			t.Fatal(s)
		}
	}
	if cur != 1 {
		t.Fatal(ss)
	}
	// revoke one other session explicitly, then all others
	var victim string
	for _, s := range ss {
		if s["current"] != true {
			victim = s["id"].(string)
			break
		}
	}
	a.call("DELETE", "/api/v1/auth/sessions/"+victim, nil, 204, nil)
	a.m("DELETE", "/api/v1/auth/sessions/"+victim, nil, 404)
	if r := a.m("POST", "/api/v1/auth/sessions/revoke-others", nil, 200); r["revoked"].(float64) != 1 {
		t.Fatal(r)
	}
	b.m("GET", "/api/v1/auth/me", nil, 401) // b was either the victim or revoked now
	// revoking the current session logs out
	ss = items(a.m("GET", "/api/v1/auth/sessions", nil, 200))
	res := a.raw("DELETE", "/api/v1/auth/sessions/"+ss[0]["id"].(string), nil)
	res.Body.Close()
	if res.StatusCode != 204 || !strings.Contains(res.Header.Get("Set-Cookie"), "Max-Age=0") {
		t.Fatal(res.StatusCode, res.Header)
	}
	a.m("GET", "/api/v1/auth/me", nil, 401)

	// API tokens cannot change the profile
	c := e.login("admin@example.com")
	tok := c.m("POST", "/api/v1/tokens", map[string]any{"name": "t"}, 201)["token"].(string)
	bt := e.anon()
	bt.bearer = tok
	bt.m("PATCH", "/api/v1/auth/me", map[string]any{"display_name": "x"}, 403)
	if r := bt.m("POST", "/api/v1/auth/sessions/revoke-others", nil, 200); r["revoked"].(float64) != 1 {
		t.Fatal(r) // a token has no session: all sessions are revoked
	}
	if ss := items(bt.m("GET", "/api/v1/auth/sessions", nil, 200)); len(ss) != 0 {
		t.Fatal(ss)
	}
	// super admins may prefer any existing tenant
	third, _ := e.st.CreateTenant(context.Background(), "third", "Third")
	d := e.login("admin@example.com")
	d.m("PATCH", "/api/v1/auth/me", map[string]any{"preferences": map[string]any{"default_tenant_id": third.ID}}, 200)
	if me := e.login("admin@example.com").m("GET", "/api/v1/auth/me", nil, 200); me["tenant"].(map[string]any)["id"] != third.ID {
		t.Fatal(me["tenant"])
	}
	// stored JSON round-trips
	got, _ := e.st.GetUser(context.Background(), e.admin.ID)
	raw, _ := json.Marshal(got.Preferences)
	if !strings.Contains(string(raw), "Europe/Stockholm") {
		t.Fatal(string(raw))
	}
	_ = http.StatusOK
}
