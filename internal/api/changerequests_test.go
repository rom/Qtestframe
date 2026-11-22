package api

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

func TestBoardOutcome(t *testing.T) {
	v := func(email, decision string, roles ...string) BoardVote {
		return BoardVote{Email: email, Roles: roles, Decision: decision}
	}
	cases := []struct {
		board model.ChangeBoard
		votes []BoardVote
		want  string
	}{
		{model.ChangeBoard{Quorum: "all"}, nil, ""},
		{model.ChangeBoard{Quorum: "all"}, []BoardVote{v("a@x", "approve")}, "approved"},
		{model.ChangeBoard{Quorum: "2"}, []BoardVote{v("a@x", "approve"), v("A@x", "approve")}, ""},
		{model.ChangeBoard{Quorum: "2"}, []BoardVote{v("a@x", "approve"), v("b@x", "approve")}, "approved"},
		{model.ChangeBoard{Users: []string{"a@x", "b@x"}, Quorum: "all"}, []BoardVote{v("a@x", "approve")}, ""},
		{model.ChangeBoard{Users: []string{"a@x", "b@x"}, Quorum: "all"}, []BoardVote{v("a@x", "approve"), v("b@x", "approve")}, "approved"},
		{model.ChangeBoard{Users: []string{"a@x", "b@x"}, Quorum: "any"}, []BoardVote{v("b@x", "approve")}, "approved"},
		{model.ChangeBoard{Users: []string{"a@x"}, Roles: []string{"manager"}, Quorum: "all"}, []BoardVote{v("a@x", "approve"), v("m@x", "approve", "tester")}, ""},
		{model.ChangeBoard{Users: []string{"a@x"}, Roles: []string{"manager"}, Quorum: "all"}, []BoardVote{v("a@x", "approve"), v("m@x", "approve", "manager")}, "approved"},
		{model.ChangeBoard{Quorum: "any"}, []BoardVote{v("a@x", "approve"), v("b@x", "defer")}, "deferred"},
		{model.ChangeBoard{Quorum: "any"}, []BoardVote{v("b@x", "defer"), v("a@x", "reject")}, "rejected"},
	}
	for i, c := range cases {
		if got := BoardOutcome(c.board, c.votes); got != c.want {
			t.Errorf("%d: %q, want %q", i, got, c.want)
		}
	}
	if a, n := BoardProgress(model.ChangeBoard{Users: []string{"a@x"}, Roles: []string{"manager"}, Quorum: "all"}, []BoardVote{v("a@x", "approve")}); a != 1 || n != 2 {
		t.Fatal(a, n)
	}
	if a, n := BoardProgress(model.ChangeBoard{Quorum: "3"}, nil); a != 0 || n != 3 {
		t.Fatal(a, n)
	}
	if !onBoard(model.ChangeBoard{}, "x@x", nil) || onBoard(model.ChangeBoard{Users: []string{"a@x"}}, "x@x", nil) || !onBoard(model.ChangeBoard{Roles: []string{"sme"}}, "x@x", []string{"sme"}) {
		t.Fatal("onBoard")
	}
}

func TestChangeRequestFlow(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	e.member("mgr@example.com", rbac.RoleManager)
	e.member("sme@example.com", rbac.RoleSME)
	e.member("tester@example.com", rbac.RoleTester)
	mgr, sme, tester := e.login("mgr@example.com"), e.login("sme@example.com"), e.login("tester@example.com")
	pid := a.project("CHG")
	base := "/api/v1/projects/" + pid
	rq := a.m("POST", base+"/requirements", map[string]any{"title": "Encrypt backups", "description": "The system MUST encrypt backups.", "status": "approved", "owner": "admin@example.com"}, 201)
	rid := rq["id"].(string)
	draft := a.m("POST", base+"/requirements", map[string]any{"title": "Draft one"}, 201)["id"].(string)
	tc := a.m("POST", base+"/tests", map[string]any{"title": "Backup is encrypted"}, 201)
	a.m("POST", base+"/links", map[string]any{"requirement_id": rid, "test_case_id": tc["id"]}, 201)

	// change control off: editing works as before
	cc := a.m("GET", base+"/change-control", nil, 200)
	if cc["settings"].(map[string]any)["mode"] != "off" {
		t.Fatal(cc)
	}
	a.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"rationale": "Data at rest"}, 200)

	// enforce
	tester.m("PUT", base+"/change-control", map[string]any{"mode": "enforce"}, 403)
	a.m("PUT", base+"/change-control", map[string]any{"mode": "enforce", "board": map[string]any{"roles": []string{"nobody"}}}, 400)
	a.m("PUT", base+"/change-control", map[string]any{"mode": "enforce", "board": map[string]any{"users": []string{"mgr@example.com"}, "quorum": "all"},
		"signature_meaning": "Approved by the CCB"}, 200)
	blocked := a.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"title": "Encrypt all backups"}, 409)
	if blocked["code"] != "change_control" || blocked["hint"] == "" || !strings.Contains(toJSON(blocked["reasons"]), "status approved") {
		t.Fatal(blocked)
	}
	a.m("PATCH", "/api/v1/requirements/"+draft, map[string]any{"title": "Draft two"}, 200) // not protected
	a.m("POST", base+"/requirements/bulk", map[string]any{"ids": []string{rid, draft}, "set": map[string]any{"priority": "high"}}, 409)
	a.m("POST", base+"/requirements/batch", map[string]any{"items": []map[string]any{{"id": rid, "set": map[string]any{"title": "x"}}}}, 409)
	a.m("DELETE", "/api/v1/requirements/"+rid, nil, 409)
	if st := a.m("GET", "/api/v1/requirements/"+rid+"/change-control", nil, 200); st["blocked"] != true || st["mode"] != "enforce" {
		t.Fatal(st)
	}
	// emergency override: change.approve and a justification
	sme.m("PATCH", "/api/v1/requirements/"+rid+"?change_override=Production+is+down+now", map[string]any{"rationale": "x"}, 403)
	a.m("PATCH", "/api/v1/requirements/"+rid+"?change_override=short", map[string]any{"rationale": "x"}, 400)
	a.m("PATCH", "/api/v1/requirements/"+rid+"?change_override=Audit+finding+fixed+today", map[string]any{"rationale": "Data at rest (audit)"}, 200)
	audit := a.m("GET", "/api/v1/audit?action=change_control.override", nil, 200)
	if !strings.Contains(toJSON(audit), "Audit finding fixed today") {
		t.Fatal(audit)
	}

	// propose: validation and permissions
	tester.m("POST", base+"/change-requests", map[string]any{"title": "x"}, 403)
	sme.m("POST", base+"/change-requests", map[string]any{"title": ""}, 400)
	sme.m("POST", base+"/change-requests", map[string]any{"title": "x", "items": []map[string]any{{"entity_type": "requirement", "entity_id": rid, "proposed": map[string]any{"version": 9}}}}, 400)
	cr := sme.m("POST", base+"/change-requests", map[string]any{"title": "Encrypt every backup", "category": "regulatory", "priority": "high",
		"items": []map[string]any{{"entity_type": "requirement", "entity_id": rid, "proposed": map[string]any{"title": "Encrypt all backups", "description": "The system MUST encrypt all backups."}}}}, 201)
	id := cr["id"].(string)
	path := "/api/v1/change-requests/" + id
	if cr["key"] != "CHG-CR-1" || cr["status"] != "draft" || len(cr["items"].([]any)) != 1 {
		t.Fatal(cr)
	}
	item := cr["items"].([]any)[0].(map[string]any)
	if item["current"].(map[string]any)["title"] != "Encrypt backups" || item["key"] != rq["key"] {
		t.Fatal(item)
	}
	sme.m("POST", path+"/submit", nil, 400) // no reason
	sme.m("PATCH", path, map[string]any{"reason": "ISO 27001 audit: all backups, also offsite"}, 200)
	sme.m("POST", path+"/items", map[string]any{"items": []map[string]any{{"entity_type": "test", "entity_id": tc["id"], "proposed": map[string]any{"expected_result": "Every backup is encrypted"}}}}, 200)
	// the item's badge and query term
	got := a.m("GET", "/api/v1/requirements/"+rid, nil, 200)
	if !strings.Contains(toJSON(got["open_change_requests"]), "CHG-CR-1") {
		t.Fatal(got["open_change_requests"])
	}
	if l := items(a.m("GET", base+"/requirements?q=cr:open", nil, 200)); len(l) != 1 || l[0]["id"] != rid {
		t.Fatal(l)
	}
	if l := items(a.m("GET", base+"/tests?q=has:change_request", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	if m := a.m("GET", base+"/change-requests/open-items", nil, 200); !strings.Contains(toJSON(m), "CHG-CR-1") {
		t.Fatal(m)
	}

	// submit: the impact snapshot
	cr = sme.m("POST", path+"/submit", nil, 200)
	if cr["status"] != "submitted" || cr["impact"] == nil || len(cr["impact"].(map[string]any)["roots"].([]any)) != 2 {
		t.Fatal(toJSON(cr["impact"]))
	}
	sme.m("PATCH", path, map[string]any{"title": "x"}, 409)
	mgr.m("POST", path+"/decision", map[string]any{"decision": "approve", "password": testPassword}, 409) // not in review
	cr = sme.m("POST", path+"/assess", map[string]any{"notes": "One test changes too", "effort": "1 day", "review": true}, 200)
	if cr["status"] != "in_review" || cr["effort"] != "1 day" || cr["assessed_by"] != "sme@example.com" {
		t.Fatal(cr)
	}
	if n := mgr.m("GET", "/api/v1/notifications", nil, 200); !strings.Contains(toJSON(n), "change_request.review") {
		t.Fatal(n)
	}
	if w := items(mgr.m("GET", "/api/v1/my-work?source=change_requests", nil, 200)); len(w) != 1 || w[0]["reasons"].([]any)[0] != "change_approval" {
		t.Fatal(w)
	}

	// who may decide
	sme.m("POST", path+"/decision", map[string]any{"decision": "approve", "password": testPassword}, 403)    // the requester, no change.approve
	a.m("POST", path+"/decision", map[string]any{"decision": "approve", "password": testPassword}, 403)      // not on the board
	mgr.m("POST", path+"/decision", map[string]any{"decision": "approve", "password": "wrong"}, 403)         // signature
	mgr.m("POST", path+"/decision", map[string]any{"decision": "defer", "password": testPassword}, 400)      // no comment
	cr = mgr.m("POST", path+"/decision", map[string]any{"decision": "approve", "comment": "Go", "password": testPassword}, 200)
	appr := cr["approvals"].([]any)
	if cr["status"] != "approved" || len(appr) != 1 || appr[0].(map[string]any)["meaning"] != "Approved by the CCB" || appr[0].(map[string]any)["valid"] != true {
		t.Fatal(cr)
	}
	if w := items(sme.m("GET", "/api/v1/my-work?source=change_requests", nil, 200)); len(w) != 1 || w[0]["reasons"].([]any)[0] != "change_approved" {
		t.Fatal(w)
	}

	// apply: atomically, with revisions and the impact action
	tester.m("POST", path+"/apply", nil, 403)
	sme.m("POST", path+"/apply", nil, 403) // the SME may not write tests: nothing is applied
	if r := a.m("GET", "/api/v1/requirements/"+rid, nil, 200); r["title"] != "Encrypt backups" {
		t.Fatal("rolled back", r["title"])
	}
	cr = a.m("POST", path+"/apply", map[string]any{"impact": "notify"}, 200)
	if cr["status"] != "implemented" || cr["applied_by"] != "admin@example.com" {
		t.Fatal(cr)
	}
	r := a.m("GET", "/api/v1/requirements/"+rid, nil, 200)
	if r["title"] != "Encrypt all backups" || r["open_change_requests"] != nil {
		t.Fatal(r)
	}
	if h := items(a.m("GET", "/api/v1/requirements/"+rid+"/history", nil, 200)); len(h) < 2 {
		t.Fatal(h)
	}
	if tt := a.m("GET", "/api/v1/tests/"+tc["id"].(string), nil, 200); tt["expected_result"] != "Every backup is encrypted" {
		t.Fatal(tt)
	}
	a.m("POST", path+"/apply", nil, 409)
	a.m("POST", path+"/close", nil, 409) // not verified
	a.m("POST", path+"/verify", map[string]any{"comment": "Checked in the test run"}, 200)
	cr = a.m("POST", path+"/close", map[string]any{"create_baseline": true, "baseline_name": "After CR-1"}, 200)
	if cr["status"] != "closed" || cr["baseline_id"] == "" {
		t.Fatal(cr)
	}
	if bl := items(a.m("GET", base+"/baselines", nil, 200)); len(bl) != 1 || bl[0]["name"] != "After CR-1" {
		t.Fatal(bl)
	}
	tl := cr["events"].([]any)
	if len(tl) < 7 {
		t.Fatal(toJSON(tl))
	}

	// the report
	res, data := a.download(base+"/reports/generate", map[string]any{"template": "change_requests", "format": "pdf", "params": map[string]string{"change_request": "CHG-CR-1"}}, 200)
	if !bytes.HasPrefix(data, []byte("%PDF")) || res.StatusCode != 200 {
		t.Fatal("pdf")
	}
	a.download(base+"/reports/generate", map[string]any{"template": "change_requests", "format": "xlsx"}, 200)
	a.download(base+"/reports/generate", map[string]any{"template": "change_requests", "format": "pdf", "params": map[string]string{"change_request": "CHG-CR-9"}}, 400)

	// warn mode: saved, with a header
	a.m("PUT", base+"/change-control", map[string]any{"mode": "warn"}, 200)
	resp := a.raw("PATCH", "/api/v1/requirements/"+rid, map[string]any{"rationale": "warned"})
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-Change-Control") != "warn" {
		t.Fatal(resp.StatusCode, resp.Header)
	}
}

func TestChangeRequestRejectWithdrawDelete(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	e.member("mgr@example.com", rbac.RoleManager)
	mgr := e.login("mgr@example.com")
	pid := a.project("CHR")
	base := "/api/v1/projects/" + pid
	rid := a.m("POST", base+"/requirements", map[string]any{"title": "R", "status": "approved"}, 201)["id"].(string)
	a.m("PUT", base+"/change-control", map[string]any{"mode": "enforce", "board": map[string]any{"roles": []string{"manager"}, "quorum": "any"}}, 200)
	cr := a.m("POST", base+"/change-requests", map[string]any{"title": "Rename", "reason": "Clearer", "submit": true,
		"items": []map[string]any{{"entity_type": "requirement", "entity_id": rid, "proposed": map[string]any{"title": "R2"}}}}, 201)
	path := "/api/v1/change-requests/" + cr["id"].(string)
	if cr["status"] != "submitted" {
		t.Fatal(cr)
	}
	a.m("POST", path+"/review", nil, 409) // not assessed
	a.m("POST", path+"/assess", map[string]any{}, 200)
	a.m("POST", path+"/review", nil, 200)
	cr = mgr.m("POST", path+"/decision", map[string]any{"decision": "reject", "comment": "Not now", "password": testPassword}, 200)
	if cr["status"] != "rejected" || cr["decision_comment"] != "Not now" {
		t.Fatal(cr)
	}
	if w := items(a.m("GET", "/api/v1/my-work?source=change_requests", nil, 200)); len(w) != 1 || w[0]["reasons"].([]any)[0] != "change_returned" {
		t.Fatal(w)
	}
	a.m("POST", path+"/withdraw", map[string]any{"comment": "Rework"}, 200)
	a.m("PATCH", path, map[string]any{"reason": "Clearer wording for the auditors"}, 200)
	if l := items(a.m("GET", base+"/change-requests?status=draft", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	a.m("GET", base+"/change-requests?status=nope", nil, 400)
	if imp := a.m("GET", path+"/impact", nil, 200); imp["roots"] == nil {
		t.Fatal(imp)
	}
	item := cr["items"].([]any)[0].(map[string]any)["id"].(string)
	a.m("DELETE", path+"/items/"+item, nil, 200)
	a.m("DELETE", path, nil, 204)
	a.m("GET", path, nil, 404)

	// the item is in a signed baseline: under change control whatever its status
	a.m("PUT", base+"/change-control", map[string]any{"mode": "enforce", "protected_statuses": []string{"verified"}, "lock_signed_baselines": true}, 200)
	a.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"title": "R3"}, 200)
	bl := a.m("POST", base+"/baselines", map[string]any{"name": "v1"}, 201)
	a.m("POST", "/api/v1/baselines/"+bl["id"].(string)+"/signoffs", map[string]any{"meaning": "approved", "password": testPassword}, 201)
	if b := a.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"title": "R4"}, 409); !strings.Contains(toJSON(b["reasons"]), "signed baseline v1") {
		t.Fatal(b)
	}
	a.m("PATCH", "/api/v1/requirements/"+rid+"?change_override=Hotfix+for+the+release", map[string]any{"title": "R5"}, 200)
	a.m("POST", "/api/v1/baselines/"+bl["id"].(string)+"/restore", map[string]any{"items": []map[string]any{{"entity_type": "requirement", "entity_id": rid}}}, 409)
}
