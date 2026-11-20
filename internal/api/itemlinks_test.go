package api

import (
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func TestItemLinks(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("LNK")
	other := a.project("OTH")
	rq := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "create_test": false}, 201)
	t1 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Valid login"}, 201)
	t2 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Logout"}, 201)
	foreign := a.m("POST", "/api/v1/projects/"+other+"/tests", map[string]any{"title": "Elsewhere"}, 201)
	run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "R1", "test_ids": []string{t1["id"].(string)}}, 201)
	doc := a.upload("POST", "/api/v1/projects/"+pid+"/documents", "file", "spec.pdf", "application/pdf", []byte("%PDF-1.4 x"), nil, 201)
	def := a.m("POST", "/api/v1/projects/"+pid+"/reports/definitions", map[string]any{"name": "Status", "config": map[string]any{"entity": "test", "columns": []string{"key"}}}, 201)
	tid, rid := t1["id"].(string), rq["id"].(string)
	base := "/api/v1/tests/" + tid + "/item-links"

	l1 := a.m("POST", base, map[string]any{"target_type": "test", "target_id": t2["id"], "label": "see also"}, 201)
	if l1["target_key"] != t2["key"] || l1["source_key"] != t1["key"] || l1["direction"] != "outgoing" {
		t.Fatal(l1)
	}
	a.m("POST", base, map[string]any{"target_type": "requirement", "target_id": rid}, 201)
	a.m("POST", base, map[string]any{"target_type": "run", "target_id": run["id"]}, 201)
	fl := a.m("POST", base, map[string]any{"target_type": "file", "target_id": doc["id"]}, 201)
	if fl["target_kind"] != "document" || fl["target_key"] != "spec.pdf" {
		t.Fatal(fl)
	}
	a.m("POST", base, map[string]any{"target_type": "report", "target_id": def["id"]}, 201)
	ext := a.m("POST", base, map[string]any{"target_type": "url", "url": " https://example.com/spec#4 ", "label": "Spec"}, 201)
	if ext["url"] != "https://example.com/spec#4" || ext["target_id"] != nil {
		t.Fatal(ext)
	}
	a.m("POST", base, map[string]any{"target_type": "url", "url": "mailto:qa@example.com"}, 201)
	for _, bad := range []map[string]any{
		{"target_type": "url", "url": "javascript:alert(1)"},
		{"target_type": "url", "url": "https://"},
		{"target_type": "url", "url": "mailto:"},
		{"target_type": "url", "url": ""},
		{"target_type": "test", "target_id": tid},           // itself
		{"target_type": "test", "target_id": foreign["id"]}, // other project
		{"target_type": "test"},                             // no target
		{"target_type": "bogus", "target_id": "x"},          // type
		{"target_type": "url", "url": "https://x.org", "label": strings.Repeat("l", 201)},
	} {
		a.m("POST", base, bad, 400)
	}
	a.m("POST", base, map[string]any{"target_type": "test", "target_id": "missing"}, 404)
	a.m("POST", base, map[string]any{"target_type": "test", "target_id": t2["id"]}, 409) // duplicate
	a.m("POST", "/api/v1/tests/missing/item-links", map[string]any{"target_type": "url", "url": "https://x.org"}, 404)

	// both directions
	if got := items(a.m("GET", base, nil, 200)); len(got) != 7 || got[0]["direction"] != "outgoing" {
		t.Fatal(got)
	}
	in := items(a.m("GET", "/api/v1/requirements/"+rid+"/item-links", nil, 200))
	if len(in) != 1 || in[0]["direction"] != "incoming" || in[0]["source_key"] != t1["key"] {
		t.Fatal(in)
	}
	a.m("POST", "/api/v1/runs/"+run["id"].(string)+"/item-links", map[string]any{"target_type": "url", "url": "https://ci.example.com/build/1"}, 201)
	if got := items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/item-links", nil, 200)); len(got) != 2 {
		t.Fatal(got)
	}
	a.m("GET", "/api/v1/requirements/missing/item-links", nil, 404)

	// label update and removal need the write permission of the source
	if up := a.m("PATCH", "/api/v1/item-links/"+ext["id"].(string), map[string]any{"label": "Specification"}, 200); up["label"] != "Specification" {
		t.Fatal(up)
	}
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	v.m("GET", base, nil, 200)
	v.m("POST", base, map[string]any{"target_type": "url", "url": "https://x.org"}, 403)
	v.m("DELETE", "/api/v1/item-links/"+ext["id"].(string), nil, 403)
	v.m("PATCH", "/api/v1/item-links/"+ext["id"].(string), map[string]any{"label": "x"}, 403)
	// a role without requirement.read does not see links to requirements and cannot add them
	e.member("tester@example.com", rbac.RoleTester)
	a.m("PUT", "/api/v1/roles/tester", map[string]any{"permissions": []string{"project.read", "test.read", "test.write", "run.read"}}, 200)
	ts := e.login("tester@example.com")
	if got := items(ts.m("GET", base, nil, 200)); len(got) != 6 {
		t.Fatal(len(got))
	}
	ts.m("POST", base, map[string]any{"target_type": "requirement", "target_id": rid}, 403)
	a.m("DELETE", "/api/v1/item-links/"+ext["id"].(string), nil, 204)
	a.m("DELETE", "/api/v1/item-links/"+ext["id"].(string), nil, 404)

	// deleting a target removes its links; deleting and restoring the source restores them
	a.m("DELETE", "/api/v1/reports/definitions/"+def["id"].(string), nil, 204)
	a.m("DELETE", "/api/v1/files/"+doc["id"].(string), nil, 204)
	if got := items(a.m("GET", base, nil, 200)); len(got) != 4 {
		t.Fatal(got)
	}
	res := a.raw("DELETE", "/api/v1/tests/"+tid, nil)
	trash := res.Header.Get("X-Trash-Id")
	if got := items(a.m("GET", "/api/v1/requirements/"+rid+"/item-links", nil, 200)); len(got) != 0 {
		t.Fatal(got)
	}
	a.m("DELETE", "/api/v1/tests/"+t2["id"].(string), nil, 204) // a target gone in the meantime
	a.m("POST", "/api/v1/trash/"+trash+"/restore", nil, 200)
	got := items(a.m("GET", base, nil, 200))
	if len(got) != 3 { // requirement, run, mailto (the link to the deleted test is left out)
		t.Fatal(got)
	}
	// deleting the run removes its links
	a.m("DELETE", "/api/v1/runs/"+run["id"].(string), nil, 204)
	if got := items(a.m("GET", base, nil, 200)); len(got) != 2 {
		t.Fatal(got)
	}
	// moving the test to another project keeps external links only
	a.m("POST", "/api/v1/projects/"+pid+"/tests/move", map[string]any{"ids": []string{tid}, "folder_id": nil, "project_id": other}, 200)
	if got := items(a.m("GET", base, nil, 200)); len(got) != 1 || got[0]["target_type"] != "url" || got[0]["project_id"] != other {
		t.Fatal(got)
	}
}

func TestAnalyticsEndpoint(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("ANA")
	for i := 0; i < 19; i++ {
		a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Req " + string(rune('A'+i)), "description": "The system MUST do thing " + string(rune('a'+i)) + " within 2 seconds.", "create_test": true}, 201)
	}
	odd := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Odd", "description": "The system SHALL archive data for 5 years.", "create_test": false}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Orphan"}, 201)
	rep := a.m("GET", "/api/v1/projects/"+pid+"/analytics", nil, 200)
	codes := map[string]map[string]any{}
	for _, f := range rep["findings"].([]any) {
		m := f.(map[string]any)
		codes[m["code"].(string)] = m
	}
	inc := codes["inconsistent_keyword"]
	if inc == nil || inc["params"].(map[string]any)["dominant"] != "MUST" || inc["items"].([]any)[0].(map[string]any)["id"] != odd["id"] {
		t.Fatal(rep)
	}
	if codes["no_tests_mandatory"] == nil || codes["orphan_tests"] == nil {
		t.Fatal(codes)
	}
	if s := rep["summary"].(map[string]any); s["requirements"].(float64) != 20 || s["tests"].(float64) != 20 {
		t.Fatal(s)
	}
	a.m("GET", "/api/v1/projects/"+pid+"/analytics?lang=sv", nil, 200)
	a.m("GET", "/api/v1/projects/"+pid+"/analytics?lang=xx", nil, 400)
	// without test.read there are no test findings
	e.member("r@example.com", rbac.RoleViewer)
	a.m("PUT", "/api/v1/roles/viewer", map[string]any{"permissions": []string{"project.read", "requirement.read"}}, 200)
	v := e.login("r@example.com")
	for _, f := range v.m("GET", "/api/v1/projects/"+pid+"/analytics", nil, 200)["findings"].([]any) {
		if f.(map[string]any)["code"] == "orphan_tests" {
			t.Fatal("test findings without test.read")
		}
	}
}
