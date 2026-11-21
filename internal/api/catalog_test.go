package api

import (
	"context"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/catalog"
	"github.com/rom/qtestframe/internal/rbac"
)

func catalogEnv(t *testing.T, areas, n int) *env {
	t.Helper()
	e := newEnv(t)
	c, err := catalog.Load(catalog.SampleFS(areas, n), "data")
	if err != nil {
		t.Fatal(err)
	}
	e.srv.Catalog = c
	return e
}

func anyList(v any) []any { l, _ := v.([]any); return l }

func TestCatalogBuiltIn(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	ov := a.m("GET", "/api/v1/catalog?lang=sv", nil, 200)
	if ov["lang"] != "sv" || ov["requirements"].(float64) < 1 || len(anyList(ov["groups"])) == 0 {
		t.Fatalf("overview: %v", ov)
	}
	r := a.m("GET", "/api/v1/catalog/requirements/AUTH-01?lang=sv", nil, 200)
	if r["title"] != "Minsta lösenordslängd" || len(anyList(r["linked_tests"])) == 0 || r["area_name"] != "Autentisering" {
		t.Fatalf("AUTH-01: %v", r)
	}
	// each ref with its source and the link to the cited text
	targets := anyList(r["ref_targets"])
	if len(targets) != len(anyList(r["refs"])) {
		t.Fatalf("ref_targets: %v", r["ref_targets"])
	}
	for _, x := range targets {
		tg := x.(map[string]any)
		if tg["source"] == "" || tg["name"] == "" || !strings.HasPrefix(tg["url"].(string), "https://") {
			t.Errorf("ref target %v", tg)
		}
	}
	// CWE and ATT&CK refs link to the weakness and technique pages
	links := map[string]string{}
	for _, x := range targets {
		tg := x.(map[string]any)
		links[tg["ref"].(string)] = tg["url"].(string)
	}
	if links["CWE-521 (weak password requirements)"] != "https://cwe.mitre.org/data/definitions/521.html" ||
		links["MITRE ATT&CK T1110 (brute force)"] != "https://attack.mitre.org/techniques/T1110/" {
		t.Errorf("CWE/ATT&CK links: %v", links)
	}
	tt := a.m("GET", "/api/v1/catalog/tests/AUTH-T01?lang=en", nil, 200)
	if len(anyList(tt["ref_targets"])) != len(anyList(tt["refs"])) {
		t.Errorf("test ref_targets: %v", tt["ref_targets"])
	}
	a.m("GET", "/api/v1/catalog?lang=de", nil, 400)
}

func TestCatalogBrowse(t *testing.T) {
	e := catalogEnv(t, 3, 4)
	a := e.login("admin@example.com")
	pid := a.project("CAT")

	ov := a.m("GET", "/api/v1/catalog", nil, 200)
	if ov["requirements"].(float64) != 12 || ov["tests"].(float64) != 12 || len(anyList(ov["groups"])) != 3 || len(anyList(ov["requirement_tags"])) == 0 {
		t.Fatalf("overview: %v", ov)
	}
	list := items(a.m("GET", "/api/v1/catalog/requirements?area=sample-2&lang=sv", nil, 200))
	if len(list) != 4 || !strings.HasPrefix(list[0]["title"].(string), "Granskningspost") || list[0]["in_project"] != nil {
		t.Fatalf("list: %v", list)
	}
	if l := items(a.m("GET", "/api/v1/catalog/requirements?q=sac-02", nil, 200)); l[0]["id"] != "SAC-02" {
		t.Fatalf("search: %v", l[0])
	}
	if l := items(a.m("GET", "/api/v1/catalog/tests?execution=automated&group=security", nil, 200)); len(l) != 2 {
		t.Fatalf("tests: %v", l)
	}
	a.m("GET", "/api/v1/catalog/requirements?lang=xx", nil, 400)
	a.m("GET", "/api/v1/catalog/tests?lang=xx", nil, 400)
	a.m("GET", "/api/v1/catalog/requirements?project_id=nope", nil, 404)
	a.m("GET", "/api/v1/catalog/tests?project_id=nope", nil, 404)
	a.m("GET", "/api/v1/catalog/requirements/NOPE-1", nil, 404)
	a.m("GET", "/api/v1/catalog/tests/NOPE-T1", nil, 404)

	d := a.m("GET", "/api/v1/catalog/requirements/SAA-01?project_id="+pid, nil, 200)
	if len(anyList(d["criteria"])) != 1 || len(anyList(d["linked_tests"])) != 2 || d["rationale"] == "" {
		t.Fatalf("detail: %v", d)
	}
	td := a.m("GET", "/api/v1/catalog/tests/SAA-T02?lang=sv&project_id="+pid, nil, 200)
	if len(anyList(td["step_list"])) != 1 || len(anyList(td["linked_requirements"])) != 2 || td["preconditions"] != "En administratör är inloggad." {
		t.Fatalf("test detail: %v", td)
	}

	// a viewer reads, but does not import
	e.member("viewer@example.com", rbac.RoleViewer)
	v := e.login("viewer@example.com")
	v.m("GET", "/api/v1/catalog/requirements", nil, 200)
	v.m("POST", "/api/v1/projects/"+pid+"/catalog/import", map[string]any{"requirement_ids": []string{"SAA-01"}}, 403)
	v.m("POST", "/api/v1/projects/"+pid+"/catalog/import", map[string]any{"test_ids": []string{"SAA-T01"}}, 403)
}

func TestCatalogImport(t *testing.T) {
	e := catalogEnv(t, 3, 4)
	a := e.login("admin@example.com")
	pid := a.project("CAT")
	base := "/api/v1/projects/" + pid
	f := a.m("POST", base+"/folders", map[string]any{"kind": "requirement", "name": "Standard"}, 201)

	res := a.m("POST", base+"/catalog/import", map[string]any{
		"requirement_ids": []string{"SAA-01", "SAB-02", "SAA-01"}, "lang": "sv", "requirement_folder_id": f["id"], "folders": "area",
	}, 201)
	reqs, tests, links := anyList(res["requirements"]), anyList(res["tests"]), anyList(res["links"])
	// SAA-01 is verified by SAA-T01 and SAA-T02; SAB-02 by SAB-T02
	if len(reqs) != 2 || len(tests) != 3 || len(links) != 3 || res["folders_created"].(float64) != 4 {
		t.Fatalf("import: %v", res)
	}
	r0 := reqs[0].(map[string]any)
	rq := a.m("GET", "/api/v1/requirements/"+r0["id"].(string), nil, 200)
	if rq["catalog_ref"] != "SAA-01" || rq["title"] != "Granskningspost 1 i område 1" || rq["status"] != "draft" || rq["source"] != "ISO/IEC 25010 §1" ||
		rq["folder_path"] != "Standard / Exempelområde 1" || len(anyList(rq["criteria"])) != 1 || rq["linked_tests"].(float64) != 2 {
		t.Fatalf("requirement: %v", rq)
	}
	if c := anyList(rq["criteria"])[0].(map[string]any); c["threshold"] != "≤ 2 s" || c["method"] != "test" {
		t.Fatalf("criterion: %v", c)
	}
	t0 := tests[0].(map[string]any)
	tc := a.m("GET", "/api/v1/tests/"+t0["id"].(string), nil, 200)
	if tc["catalog_ref"] != "SAA-T01" || tc["expected_result"] != "Posten skrivs." || len(anyList(tc["steps"])) != 1 || tc["folder_path"] != "Exempelområde 1" {
		t.Fatalf("test: %v", tc)
	}
	// the query field catalog
	if l := items(a.m("GET", base+"/requirements?q=catalog:SAA-01", nil, 200)); len(l) != 1 {
		t.Fatalf("catalog query: %v", l)
	}
	in := a.m("GET", base+"/catalog", nil, 200)
	if len(in["requirements"].(map[string]any)) != 2 || len(in["tests"].(map[string]any)) != 3 {
		t.Fatalf("in project: %v", in)
	}
	l := items(a.m("GET", "/api/v1/catalog/requirements?area=sample-1&project_id="+pid, nil, 200))
	if l[0]["in_project"] == nil || l[1]["in_project"] != nil {
		t.Fatalf("marked: %v", l)
	}
	if l := items(a.m("GET", "/api/v1/catalog/requirements?area=sample-1&not_in_project=true&project_id="+pid, nil, 200)); len(l) != 3 {
		t.Fatalf("hidden: %v", l)
	}
	if l := items(a.m("GET", "/api/v1/catalog/tests?area=sample-1&not_in_project=1&project_id="+pid, nil, 200)); len(l) != 2 {
		t.Fatalf("hidden tests: %v", l)
	}

	// again: existing items are skipped; a new test links to the existing requirement
	res = a.m("POST", base+"/catalog/import", map[string]any{"requirement_ids": []string{"SAA-01"}, "test_ids": []string{"SAA-T03", "NOPE"}}, 201)
	if len(anyList(res["requirements"])) != 0 || len(anyList(res["tests"])) != 1 || len(anyList(res["skipped"])) != 4 {
		t.Fatalf("second import: %v", res)
	}
	for _, s := range anyList(res["skipped"]) {
		s := s.(map[string]any)
		if (s["catalog_ref"] == "NOPE") != (s["reason"] == "not_found") {
			t.Fatalf("skipped: %v", s)
		}
	}
	// SAA-T03 verifies SAA-03, which is not in the project: no link
	if len(anyList(res["links"])) != 0 {
		t.Fatalf("links: %v", res["links"])
	}
	// copies on request; a new requirement links to existing tests
	res = a.m("POST", base+"/catalog/import", map[string]any{"requirement_ids": []string{"SAA-01"}, "add_copies": true, "include_tests": false, "source": "Standard", "status": "review"}, 201)
	if len(anyList(res["requirements"])) != 1 || len(anyList(res["tests"])) != 0 || len(anyList(res["links"])) != 2 {
		t.Fatalf("copy: %v", res)
	}
	cp := a.m("GET", "/api/v1/requirements/"+anyList(res["requirements"])[0].(map[string]any)["id"].(string), nil, 200)
	if cp["source"] != "Standard" || cp["status"] != "review" || cp["folder_path"] != nil {
		t.Fatalf("copy fields: %v", cp)
	}
	// SAA-03 now links to the existing SAA-T03 test
	res = a.m("POST", base+"/catalog/import", map[string]any{"requirement_ids": []string{"SAA-03"}, "folders": "group"}, 201)
	if len(anyList(res["links"])) != 1 || len(anyList(res["tests"])) != 0 || res["folders_created"].(float64) != 2 {
		t.Fatalf("existing test link: %v", res)
	}

	// the origin survives edits, copies, moves and the trash
	id := r0["id"].(string)
	a.m("PATCH", "/api/v1/requirements/"+id, map[string]any{"title": "Changed", "catalog_ref": "OTHER"}, 200)
	if got := a.m("GET", "/api/v1/requirements/"+id, nil, 200); got["catalog_ref"] != "SAA-01" || got["title"] != "Changed" {
		t.Fatalf("after edit: %v", got)
	}
	if got := a.m("POST", "/api/v1/requirements/"+id+"/copy", nil, 201); got["catalog_ref"] != "SAA-01" {
		t.Fatalf("copy: %v", got)
	}
	tid := t0["id"].(string)
	a.m("PATCH", "/api/v1/tests/"+tid, map[string]any{"title": "Changed"}, 200)
	if got := a.m("POST", "/api/v1/tests/"+tid+"/copy", nil, 201); got["catalog_ref"] != "SAA-T01" {
		t.Fatalf("test copy: %v", got)
	}
	del := a.m("DELETE", base+"/requirements/bulk", map[string]any{"ids": []string{id}}, 200)
	a.m("POST", "/api/v1/trash/"+del["trash_id"].(string)+"/restore", nil, 200)
	if got := a.m("GET", "/api/v1/requirements/"+id, nil, 200); got["catalog_ref"] != "SAA-01" {
		t.Fatalf("after restore: %v", got)
	}
	other := a.project("OTH")
	a.m("POST", base+"/requirements/move", map[string]any{"ids": []string{id}, "project_id": other}, 200)
	if got := a.m("GET", "/api/v1/requirements/"+id, nil, 200); got["catalog_ref"] != "SAA-01" || got["project_id"] != other {
		t.Fatalf("after move: %v", got)
	}

	// audited
	entries, err := e.st.ListAudit(context.Background(), e.tenant.ID, 50)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, en := range entries {
		found = found || en.Action == "catalog.import"
	}
	if !found {
		t.Fatal("no catalog.import audit entry")
	}
}

func TestCatalogImportRules(t *testing.T) {
	e := catalogEnv(t, 30, 20) // 600 requirements
	a := e.login("admin@example.com")
	pid := a.project("CAT")
	p := "/api/v1/projects/" + pid + "/catalog/import"
	a.m("POST", p, map[string]any{}, 400)
	a.m("POST", p, map[string]any{"requirement_ids": []string{"SAA-01"}, "lang": "de"}, 400)
	a.m("POST", p, map[string]any{"requirement_ids": []string{"SAA-01"}, "folders": "tree"}, 400)
	a.m("POST", p, map[string]any{"requirement_ids": []string{"SAA-01"}, "status": "nope"}, 400)
	a.m("POST", p, map[string]any{"requirement_ids": []string{"SAA-01"}, "test_status": "nope"}, 400)
	a.m("POST", p, map[string]any{"requirement_ids": []string{"SAA-01"}, "requirement_folder_id": "nope"}, 400)
	a.m("POST", p, map[string]any{"requirement_ids": []string{"SAA-01"}, "test_folder_id": "nope"}, 400)
	a.m("POST", "/api/v1/projects/nope/catalog/import", map[string]any{"requirement_ids": []string{"SAA-01"}}, 404)
	// at most 500 items: 260 requirements with their 260 tests are too many
	var ids []string
	for _, r := range items(a.m("GET", "/api/v1/catalog/requirements", nil, 200))[:260] {
		ids = append(ids, r["id"].(string))
	}
	if m := a.m("POST", p, map[string]any{"requirement_ids": ids}, 400); !strings.Contains(m["error"].(string), "500") {
		t.Fatal(m)
	}
	// the whole request is atomic: nothing was stored
	if n := len(items(a.m("GET", "/api/v1/projects/"+pid+"/requirements", nil, 200))); n != 0 {
		t.Fatalf("stored: %d", n)
	}
	// 250 + 250 fit
	res := a.m("POST", p, map[string]any{"requirement_ids": ids[:250]}, 201)
	if len(anyList(res["requirements"])) != 250 || len(anyList(res["tests"])) != 250 {
		t.Fatalf("large import: %d %d", len(anyList(res["requirements"])), len(anyList(res["tests"])))
	}

	// a tester adds tests only; requirements need requirement.write
	e.member("tester@example.com", rbac.RoleTester)
	ts := e.login("tester@example.com")
	ts.m("POST", p, map[string]any{"requirement_ids": []string{"SBA-01"}}, 403)
	res = ts.m("POST", p, map[string]any{"test_ids": []string{"SBA-T01"}}, 201)
	if len(anyList(res["tests"])) != 1 {
		t.Fatal(res)
	}
	ts.m("POST", p, map[string]any{"test_ids": []string{"SBA-T02"}, "test_status": "ready"}, 403)
	// a manager without test.write: the tests are not included by default
	e.member("reviewer@example.com", rbac.RoleReviewer)
	rv := e.login("reviewer@example.com")
	rv.m("POST", p, map[string]any{"requirement_ids": []string{"SBA-01"}}, 403)

	if truncate("ab", 5) != "ab" || truncate("åäö", 3) != "å…" {
		t.Fatal(truncate("åäö", 3))
	}
}

func TestCatalogImportWithoutTestWrite(t *testing.T) {
	e := catalogEnv(t, 2, 2)
	ctx := context.Background()
	// a role with requirement.write but not test.write
	if _, err := e.st.SaveRole(ctx, e.tenant.ID, rbac.Role{Name: "author", Permissions: []string{"project.read", "requirement.read", "requirement.write", "test.read"}}, true); err != nil {
		t.Fatal(err)
	}
	e.member("author@example.com", "author")
	a := e.login("author@example.com")
	adm := e.login("admin@example.com")
	pid := adm.project("CAT")
	p := "/api/v1/projects/" + pid + "/catalog/import"
	a.m("POST", p, map[string]any{"requirement_ids": []string{"SAA-01"}, "include_tests": true}, 403)
	a.m("POST", p, map[string]any{"requirement_ids": []string{"SAA-01"}, "status": "approved"}, 403)
	res := a.m("POST", p, map[string]any{"requirement_ids": []string{"SAA-01"}}, 201)
	if len(anyList(res["requirements"])) != 1 || len(anyList(res["tests"])) != 0 || len(anyList(res["links"])) != 0 {
		t.Fatal(res)
	}
	in := a.m("GET", "/api/v1/projects/"+pid+"/catalog", nil, 200)
	if len(in["requirements"].(map[string]any)) != 1 {
		t.Fatal(in)
	}
	// a role that reads only tests sees no requirements of the project
	if _, err := e.st.SaveRole(ctx, e.tenant.ID, rbac.Role{Name: "testreader", Permissions: []string{"project.read", "test.read"}}, true); err != nil {
		t.Fatal(err)
	}
	e.member("tr@example.com", "testreader")
	tr := e.login("tr@example.com")
	if in := tr.m("GET", "/api/v1/projects/"+pid+"/catalog", nil, 200); len(in["requirements"].(map[string]any)) != 0 {
		t.Fatal(in)
	}
	tr.m("GET", "/api/v1/catalog/requirements", nil, 403)
	if d := tr.m("GET", "/api/v1/catalog/tests/SAA-T01?project_id="+pid, nil, 200); d["linked_requirements"].([]any)[0].(map[string]any)["in_project"] != nil {
		t.Fatal(d)
	}
}

func TestCatalogHelpers(t *testing.T) {
	r := &catalog.Requirement{ID: "X-01"}
	if catalogSource(r, "sv") != "Katalog X-01" || catalogSource(r, "en") != "Catalogue X-01" {
		t.Fatal(catalogSource(r, "en"))
	}
	r.Refs = []string{"A", "B"}
	if catalogSource(r, "en") != "A; B" {
		t.Fatal(catalogSource(r, "en"))
	}
	if areaName(&catalog.Catalog{}, "nope", "en") != "nope" {
		t.Fatal("area name")
	}
	if folderPathFor(&catalog.Catalog{}, "area", "nope", "en") != nil {
		t.Fatal("unknown area")
	}
}
