package api

import (
	"net/url"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func TestCriterionVerificationAndRecords(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("VER")
	pp := "/api/v1/projects/" + pid
	a.m("PUT", "/api/v1/settings", map[string]any{"allow_unready_tests": true, "allow_unready_requirements": true}, 200)
	rq := a.m("POST", pp+"/requirements", map[string]any{"title": "Housing", "description": "The housing MUST be sealed.", "status": "implemented", "create_test": false,
		"criteria": []map[string]any{{"description": "Leak test passes", "method": "test"}, {"description": "Seal drawing checked", "method": "inspection"}}}, 201)
	rid := rq["id"].(string)
	crit := rq["criteria"].([]any)
	c1, c2 := crit[0].(map[string]any)["id"].(string), crit[1].(map[string]any)["id"].(string)
	steps := []map[string]any{{"action": "Pressurise", "expected": "no leak"}, {"action": "Release", "expected": "ok"}}
	tc := a.m("POST", pp+"/tests", map[string]any{"title": "Leak test", "status": "ready", "steps": steps}, 201)
	tid := tc["id"].(string)
	step2 := tc["steps"].([]any)[1].(map[string]any)["id"].(string)
	status := func() string { return a.m("GET", "/api/v1/requirements/"+rid, nil, 200)["status"].(string) }
	ver := func() map[string]any { return a.m("GET", "/api/v1/requirements/"+rid+"/verification", nil, 200) }

	// a plain link: as before, it counts for both criteria
	l := a.m("POST", pp+"/links", map[string]any{"requirement_id": rid, "test_case_id": tid}, 201)
	lid := l["id"].(string)
	if len(l["criteria"].([]any)) != 0 {
		t.Fatal(l)
	}
	run := a.m("POST", pp+"/runs", map[string]any{"name": "R", "test_ids": []string{tid}}, 201)
	ex := items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200))[0]
	exe := a.m("GET", "/api/v1/executions/"+ex["id"].(string), nil, 200)
	srs := exe["step_results"].([]any)
	sr := func(i int, st string) map[string]any {
		return map[string]any{"id": srs[i].(map[string]any)["id"], "status": st}
	}
	a.m("PATCH", "/api/v1/executions/"+ex["id"].(string), map[string]any{"step_results": []any{sr(0, "passed"), sr(1, "passed")}}, 200)
	if s := status(); s != "verified" {
		t.Fatal(s, ver())
	}
	// strict criterion coverage: the inspection criterion needs evidence of its own
	a.m("PUT", pp+"/verification-settings", map[string]any{"strict_criteria": true}, 200)
	v := ver()
	if v["verified"] != false || v["state"] != "pending" || status() != "implemented" {
		t.Fatal(v)
	}
	// map the link to criterion 1 through step 2
	l = a.m("PUT", "/api/v1/links/"+lid+"/criteria", map[string]any{"criteria": []map[string]any{{"criterion_id": c1, "step_id": step2}}}, 200)
	if l["criterion_id"] != c1 || l["criteria"].([]any)[0].(map[string]any)["step_id"] != step2 {
		t.Fatal(l)
	}
	a.m("PUT", "/api/v1/links/"+lid+"/criteria", map[string]any{"criteria": []map[string]any{{"criterion_id": "nope"}}}, 400)
	a.m("PUT", "/api/v1/links/"+lid+"/criteria", map[string]any{"criteria": []map[string]any{{"criterion_id": c1, "step_id": "nope"}}}, 400)
	v = ver()
	cs := v["criteria"].([]any)
	if cs[0].(map[string]any)["state"] != "verified" || cs[1].(map[string]any)["state"] != "none" || v["criteria_verified"].(float64) != 1 {
		t.Fatal(v)
	}
	if r0 := v["reasons"].([]any)[0].(map[string]any); r0["code"] != "criterion_missing" || r0["params"].(map[string]any)["criterion"] != "2" {
		t.Fatal(r0)
	}

	// a document of the archive and an inspection record for criterion 2
	doc := a.upload("POST", pp+"/documents", "file", "seal.pdf", "application/pdf", []byte("%PDF-1.4"), nil, 201)
	rec := a.m("POST", "/api/v1/requirements/"+rid+"/verification-records", map[string]any{"criterion_id": c2, "result": "pass", "performed_on": "2026-01-10",
		"note": "Drawing D-12 rev B", "file_ids": []string{doc["id"].(string)}, "until_change": true}, 201)
	recID := rec["id"].(string)
	if rec["method"] != "inspection" || rec["validity"] != "valid" || len(rec["files"].([]any)) != 1 || rec["performed_by"] == "" {
		t.Fatal(rec)
	}
	if s := status(); s != "verified" {
		t.Fatal(s, ver())
	}
	a.m("POST", "/api/v1/requirements/"+rid+"/verification-records", map[string]any{"result": "maybe", "method": "review"}, 400)
	a.m("POST", "/api/v1/requirements/"+rid+"/verification-records", map[string]any{"result": "pass", "method": "review", "performed_on": "10/01/2026"}, 400)
	a.m("POST", "/api/v1/requirements/"+rid+"/verification-records", map[string]any{"result": "pass", "method": "review", "criterion_id": "x"}, 400)
	if list := items(a.m("GET", pp+"/verification-records?requirement_id="+rid, nil, 200)); len(list) != 1 {
		t.Fatal(list)
	}
	if list := items(a.m("GET", "/api/v1/requirements/"+rid+"/verification-records", nil, 200)); len(list) != 1 {
		t.Fatal(list)
	}

	// sign-off by a second person only
	a.m("POST", "/api/v1/verification-records/"+recID+"/signoff", map[string]any{"comment": "ok"}, 409)
	e.member("rita@example.com", rbac.RoleReviewer)
	rv := e.login("rita@example.com")
	got := rv.m("POST", "/api/v1/verification-records/"+recID+"/signoff", map[string]any{"comment": "checked"}, 200)
	if got["signoff"].(map[string]any)["by"] != "rita@example.com" {
		t.Fatal(got)
	}
	rv.m("PATCH", "/api/v1/verification-records/"+recID, map[string]any{"note": "x"}, 403)
	e.member("vic@example.com", rbac.RoleViewer)
	e.login("vic@example.com").m("POST", "/api/v1/verification-records/"+recID+"/signoff", nil, 403)
	// a note change keeps the sign-off, a result change removes it
	got = a.m("PATCH", "/api/v1/verification-records/"+recID, map[string]any{"note": "Drawing D-12 rev C"}, 200)
	if got["signoff"] == nil {
		t.Fatal(got)
	}
	got = a.m("PATCH", "/api/v1/verification-records/"+recID, map[string]any{"result": "fail"}, 200)
	if got["signoff"] != nil || status() != "implemented" || ver()["state"] != "failed" {
		t.Fatal(got)
	}
	a.m("PATCH", "/api/v1/verification-records/"+recID, map[string]any{"result": "conditional"}, 200)
	if s := status(); s != "verified" {
		t.Fatal(s)
	}
	rv.m("DELETE", "/api/v1/verification-records/"+recID+"/signoff", nil, 200)

	// a content change invalidates an "until change" record
	a.m("PATCH", "/api/v1/requirements/"+rid, map[string]any{"description": "The housing MUST be sealed to IP67."}, 200)
	got = a.m("GET", "/api/v1/verification-records/"+recID, nil, 200)
	if got["validity"] != "invalidated" || got["invalidated_reason"] == "" || status() != "implemented" {
		t.Fatal(got, status())
	}

	// query fields and the traceability matrix
	if res := items(a.m("GET", pp+"/requirements?q=verification:pending", nil, 200)); len(res) != 1 || res[0]["criteria_verified"] != nil && res[0]["criteria_verified"].(float64) != 0 {
		t.Fatal(res)
	}
	tr := a.m("GET", pp+"/traceability", nil, 200)
	treq := tr["requirements"].([]any)[0].(map[string]any)
	if len(treq["criteria"].([]any)) != 2 || treq["verification"] != "pending" {
		t.Fatal(treq)
	}
	pv := a.m("GET", pp+"/verification", nil, 200)
	if pv["strict"] != true || pv["counts"].(map[string]any)["pending"].(float64) != 1 || pv["criteria_total"].(float64) != 2 {
		t.Fatal(pv)
	}
	// the verification report and the specification
	_, data := a.download(pp+"/reports/generate", map[string]any{"template": "verification", "format": "md"}, 200)
	for _, want := range []string{"Verification by criterion", "Seal drawing checked", "invalidated", "Leak test passes", "step 2"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("report: missing %q in\n%s", want, data)
		}
	}
	f := a.m("POST", pp+"/documents/generate", map[string]any{"kind": "requirements_spec", "format": "md", "options": map[string]any{"fields": []string{"description", "tests", "criteria"}}}, 201)
	if md := a.fileContent(f["id"].(string)); !strings.Contains(md, "**Verification:** Not yet passed · 0 of 2 criteria verified") {
		t.Errorf("spec:\n%s", md)
	}
	a.m("DELETE", "/api/v1/verification-records/"+recID, nil, 204)
	a.m("GET", "/api/v1/verification-records/"+recID, nil, 404)
	if got := a.m("GET", pp+"/verification-settings", nil, 200); got["strict_criteria"] != true {
		t.Fatal(got)
	}
}

func TestHierarchyRollUpAndElements(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("SYS")
	pp := "/api/v1/projects/" + pid
	a.m("PUT", "/api/v1/settings", map[string]any{"allow_unready_tests": true, "allow_unready_requirements": true}, 200)
	mk := func(title string) string {
		return a.m("POST", pp+"/requirements", map[string]any{"title": title, "status": "implemented", "create_test": false}, 201)["id"].(string)
	}
	parent, c1, c2 := mk("Vehicle"), mk("Brakes"), mk("Lights")
	a.m("POST", "/api/v1/requirements/"+c1+"/relations", map[string]any{"target_id": parent, "type": "refines"}, 201)
	rel := a.m("POST", "/api/v1/requirements/"+c2+"/relations", map[string]any{"target_id": parent, "type": "refines"}, 201)
	t1 := a.m("POST", pp+"/tests", map[string]any{"title": "Brake test"}, 201)["id"].(string)
	t2 := a.m("POST", pp+"/tests", map[string]any{"title": "Light test"}, 201)["id"].(string)
	a.m("POST", pp+"/links", map[string]any{"requirement_id": c1, "test_case_id": t1}, 201)
	a.m("POST", pp+"/links", map[string]any{"requirement_id": c2, "test_case_id": t2}, 201)
	run := a.m("POST", pp+"/runs", map[string]any{"name": "R", "test_ids": []string{t1, t2}}, 201)
	ex := map[string]string{}
	for _, x := range items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200)) {
		ex[x["test_case_id"].(string)] = x["id"].(string)
	}
	status := func(id string) string { return a.m("GET", "/api/v1/requirements/"+id, nil, 200)["status"].(string) }
	a.m("PATCH", "/api/v1/executions/"+ex[t1], map[string]any{"status": "passed"}, 200)
	if status(c1) != "verified" || status(parent) != "implemented" {
		t.Fatal(status(c1), status(parent))
	}
	v := a.m("GET", "/api/v1/requirements/"+parent+"/verification", nil, 200)
	if v["state"] != "partial" || v["children_verified"].(float64) != 1 || len(v["children"].([]any)) != 2 {
		t.Fatal(v)
	}
	// the last child verified: the parent rolls up
	a.m("PATCH", "/api/v1/executions/"+ex[t2], map[string]any{"status": "passed"}, 200)
	if status(parent) != "verified" {
		t.Fatal(status(parent))
	}
	// a child fails: the parent falls back (no unverified child under a verified parent)
	a.m("PATCH", "/api/v1/executions/"+ex[t2], map[string]any{"status": "failed"}, 200)
	if status(c2) != "implemented" || status(parent) != "implemented" {
		t.Fatal(status(c2), status(parent))
	}
	// removing the failing child restores the parent
	a.m("DELETE", "/api/v1/relations/"+rel["id"].(string), nil, 204)
	if status(parent) != "verified" {
		t.Fatal(status(parent))
	}
	// with manual verification an inconsistency is flagged, not fixed
	a.m("PUT", "/api/v1/settings", map[string]any{"allow_unready_tests": true, "allow_unready_requirements": true, "manual_verification": true}, 200)
	a.m("POST", "/api/v1/requirements/"+c2+"/relations", map[string]any{"target_id": parent, "type": "refines"}, 201)
	pv := a.m("GET", pp+"/verification", nil, 200)
	if inc := pv["inconsistent"].([]any); len(inc) != 1 || inc[0].(map[string]any)["key"] != "SYS-REQ-1" {
		t.Fatal(pv["inconsistent"])
	}
	an := a.m("GET", pp+"/analytics", nil, 200)
	found := false
	for _, f := range an["findings"].([]any) {
		if f.(map[string]any)["code"] == "verified_parent_unverified_child" {
			found = true
		}
	}
	if !found || an["verification"] == nil {
		t.Fatal(an["findings"])
	}

	// system elements
	root := a.m("POST", pp+"/elements", map[string]any{"name": "Vehicle", "kind": "system"}, 201)["id"].(string)
	pu := a.m("POST", pp+"/elements", map[string]any{"name": "Power unit", "kind": "subsystem", "parent_id": root}, 201)
	puID := pu["id"].(string)
	if pu["path"] != "Vehicle / Power unit" {
		t.Fatal(pu)
	}
	a.m("POST", pp+"/elements", map[string]any{"name": "power UNIT", "parent_id": root}, 409)
	a.m("POST", pp+"/elements", map[string]any{"name": "a/b"}, 400)
	a.m("POST", pp+"/elements", map[string]any{"name": "x", "kind": "planet"}, 400)
	a.m("PATCH", "/api/v1/elements/"+root, map[string]any{"parent_id": puID}, 400) // below itself
	a.m("POST", "/api/v1/elements/"+puID+"/allocations", map[string]any{"kind": "requirement", "add": []string{c1, c2}}, 200)
	a.m("PUT", "/api/v1/requirements/"+parent+"/elements", map[string]any{"element_ids": []string{root}}, 200)
	got := a.m("PUT", "/api/v1/tests/"+t1+"/elements", map[string]any{"element_ids": []string{puID}}, 200)
	if len(got["element_ids"].([]any)) != 1 {
		t.Fatal(got)
	}
	if res := items(a.m("GET", pp+"/requirements?q="+url.QueryEscape(`element:"Power unit"`), nil, 200)); len(res) != 2 {
		t.Fatal(res)
	}
	if res := items(a.m("GET", pp+`/requirements?q=element:Vehicle`, nil, 200)); len(res) != 3 {
		t.Fatal(res)
	}
	if res := items(a.m("GET", pp+"/tests?q="+url.QueryEscape(`element:"power unit"`), nil, 200)); len(res) != 1 || len(res[0]["element_ids"].([]any)) != 1 {
		t.Fatal(res)
	}
	cov := items(a.m("GET", pp+"/elements/coverage", nil, 200))
	if len(cov) != 2 || cov[0]["requirements"].(float64) != 3 || cov[1]["requirements"].(float64) != 2 || cov[1]["verified"].(float64) != 1 || cov[1]["tests"].(float64) != 1 {
		t.Fatal(cov)
	}
	a.m("POST", "/api/v1/elements/"+puID+"/allocations", map[string]any{"kind": "requirement", "remove": []string{c2}}, 200)
	a.m("POST", "/api/v1/elements/"+puID+"/allocations", map[string]any{"kind": "other"}, 400)
	got = a.m("PATCH", "/api/v1/elements/"+puID, map[string]any{"name": "Drive unit", "description": "E-motor"}, 200)
	if got["path"] != "Vehicle / Drive unit" || len(got["requirement_ids"].([]any)) != 1 {
		t.Fatal(got)
	}
	e.member("tess@example.com", rbac.RoleTester)
	ts := e.login("tess@example.com")
	ts.m("POST", "/api/v1/elements/"+puID+"/allocations", map[string]any{"kind": "requirement", "add": []string{c2}}, 403)
	ts.m("POST", "/api/v1/elements/"+puID+"/allocations", map[string]any{"kind": "test", "add": []string{t2}}, 200)
	ts.m("POST", pp+"/elements", map[string]any{"name": "Nope"}, 403)
	if list := items(ts.m("GET", pp+"/elements", nil, 200)); len(list) != 2 {
		t.Fatal(list)
	}
	a.m("DELETE", "/api/v1/elements/"+root, nil, 204)
	if list := items(a.m("GET", pp+"/elements", nil, 200)); len(list) != 0 {
		t.Fatal(list)
	}
}
