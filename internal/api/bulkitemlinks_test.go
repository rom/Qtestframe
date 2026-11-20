package api

import (
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func TestBulkItemLinks(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BIL")
	other := a.project("BIO")
	req := func(p, title string) string {
		return a.m("POST", "/api/v1/projects/"+p+"/requirements", map[string]any{"title": title}, 201)["id"].(string)
	}
	r1, r2 := req(pid, "One"), req(pid, "Two")
	foreign := req(other, "Elsewhere")
	tc := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "T"}, 201)["id"].(string)
	a.m("POST", "/api/v1/requirements/"+r1+"/item-links", map[string]any{"target_type": "url", "url": "https://example.org/spec"}, 201)

	link := func(st, sid, tt string, tid *string, url string) map[string]any {
		return map[string]any{"source_type": st, "source_id": sid, "target_type": tt, "target_id": tid, "url": url, "label": "Spec"}
	}
	res := a.m("POST", "/api/v1/projects/"+pid+"/item-links/bulk", map[string]any{"links": []any{
		link("requirement", r1, "url", nil, "https://example.org/spec"), // exists
		link("requirement", r2, "url", nil, "https://example.org/spec"), // created
		link("requirement", r1, "test", &tc, ""),                        // created
		link("requirement", r2, "test", &tc, ""),                        // created
		link("requirement", r1, "requirement", &r1, ""),                 // self: invalid
		link("requirement", r1, "url", nil, "javascript:alert(1)"),      // invalid url
		link("requirement", foreign, "url", nil, "https://x.org"),       // other project
		link("requirement", "missing", "url", nil, "https://x.org"),     // not found
		link("defect", r1, "url", nil, "https://x.org"),                 // bad source type
	}}, 200)
	created, skipped := res["created"].([]any), res["skipped"].([]any)
	if len(created) != 3 {
		t.Fatal(res)
	}
	reasons := map[string]int{}
	for _, x := range skipped {
		reasons[x.(map[string]any)["reason"].(string)]++
	}
	if reasons["exists"] != 1 || reasons["invalid"] != 3 || reasons["not_found"] != 2 {
		t.Fatal(reasons)
	}
	if created[0].(map[string]any)["label"] != "Spec" || created[0].(map[string]any)["source_key"] == "" {
		t.Fatal(created[0])
	}
	a.m("POST", "/api/v1/projects/"+pid+"/item-links/bulk", map[string]any{"links": []any{}}, 400)

	all := items(a.m("GET", "/api/v1/projects/"+pid+"/item-links?source_type=requirement", nil, 200))
	if len(all) != 4 {
		t.Fatal(len(all))
	}
	if n := len(items(a.m("GET", "/api/v1/projects/"+pid+"/item-links?source_type=test", nil, 200))); n != 0 {
		t.Fatal(n)
	}
	a.m("GET", "/api/v1/projects/"+pid+"/item-links?source_type=defect", nil, 400)
	var ids []string
	for _, x := range all {
		ids = append(ids, x["id"].(string))
	}

	// a tester may read requirement links but not change them
	e.member("t@example.com", rbac.RoleTester)
	tt := e.login("t@example.com")
	d := tt.m("DELETE", "/api/v1/projects/"+pid+"/item-links/bulk", map[string]any{"ids": ids}, 200)
	if len(d["deleted"].([]any)) != 0 || d["skipped"].([]any)[0].(map[string]any)["reason"] != "not_allowed" {
		t.Fatal(d)
	}
	c := tt.m("POST", "/api/v1/projects/"+pid+"/item-links/bulk", map[string]any{"links": []any{link("requirement", r2, "url", nil, "https://y.org")}}, 200)
	if c["skipped"].([]any)[0].(map[string]any)["reason"] != "not_allowed" {
		t.Fatal(c)
	}
	// but may link tests
	c = tt.m("POST", "/api/v1/projects/"+pid+"/item-links/bulk", map[string]any{"links": []any{link("test", tc, "requirement", &r1, "")}}, 200)
	if len(c["created"].([]any)) != 1 {
		t.Fatal(c)
	}

	// another project's bulk delete does not touch these
	d = a.m("DELETE", "/api/v1/projects/"+other+"/item-links/bulk", map[string]any{"ids": ids}, 200)
	if len(d["deleted"].([]any)) != 0 {
		t.Fatal(d)
	}
	d = a.m("DELETE", "/api/v1/projects/"+pid+"/item-links/bulk", map[string]any{"ids": append(ids[:3:3], "nope")}, 200)
	if len(d["deleted"].([]any)) != 3 || len(d["skipped"].([]any)) != 1 {
		t.Fatal(d)
	}
	if n := len(items(a.m("GET", "/api/v1/projects/"+pid+"/item-links", nil, 200))); n != 2 {
		t.Fatal(n)
	}
}
