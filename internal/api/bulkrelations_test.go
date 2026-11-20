package api

import (
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func TestBulkRelations(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BRL")
	other := a.project("BRO")
	mk := func(p, title string) string {
		return a.m("POST", "/api/v1/projects/"+p+"/requirements", map[string]any{"title": title}, 201)["id"].(string)
	}
	parent, c1, c2, c3 := mk(pid, "Parent"), mk(pid, "Child 1"), mk(pid, "Child 2"), mk(pid, "Child 3")
	foreign := mk(other, "Elsewhere")
	a.m("POST", "/api/v1/requirements/"+c1+"/relations", map[string]any{"target_id": parent, "type": "refines"}, 201)

	rel := func(src, dst, typ string) map[string]any {
		return map[string]any{"source_id": src, "target_id": dst, "type": typ}
	}
	res := a.m("POST", "/api/v1/projects/"+pid+"/relations/bulk", map[string]any{"relations": []any{
		rel(c1, parent, "refines"),     // exists
		rel(c2, parent, "refines"),     // created
		rel(c3, parent, "refines"),     // created
		rel(parent, c2, "refines"),     // cycle
		rel(c2, c2, "relates_to"),      // self
		rel(c2, foreign, "relates_to"), // other project
		rel(c2, c3, "nonsense"),        // unknown type
		rel(c2, c3, "depends_on"),      // created
	}}, 200)
	created, skipped := res["created"].([]any), res["skipped"].([]any)
	if len(created) != 3 {
		t.Fatal(res)
	}
	reasons := map[string]int{}
	for _, x := range skipped {
		reasons[x.(map[string]any)["reason"].(string)]++
	}
	if reasons["exists"] != 1 || reasons["invalid"] != 2 || reasons["self"] != 1 || reasons["not_found"] != 1 {
		t.Fatal(reasons)
	}
	list := a.m("GET", "/api/v1/projects/"+pid+"/relations", nil, 200)["items"].([]any)
	if len(list) != 4 {
		t.Fatal(len(list))
	}
	a.m("POST", "/api/v1/projects/"+pid+"/relations/bulk", map[string]any{"relations": []any{}}, 400)

	var ids []string
	for _, x := range list {
		ids = append(ids, x.(map[string]any)["id"].(string))
	}
	// another project's bulk delete does not touch these
	d := a.m("DELETE", "/api/v1/projects/"+other+"/relations/bulk", map[string]any{"ids": ids}, 200)
	if len(d["deleted"].([]any)) != 0 || len(d["skipped"].([]any)) != 4 {
		t.Fatal(d)
	}
	d = a.m("DELETE", "/api/v1/projects/"+pid+"/relations/bulk", map[string]any{"ids": ids[:3]}, 200)
	if len(d["deleted"].([]any)) != 3 || d["deleted"].([]any)[0].(map[string]any)["source_id"] == "" {
		t.Fatal(d)
	}
	if n := len(a.m("GET", "/api/v1/projects/"+pid+"/relations", nil, 200)["items"].([]any)); n != 1 {
		t.Fatal(n)
	}

	e.member("v@example.com", rbac.RoleViewer)
	v := e.login("v@example.com")
	v.m("POST", "/api/v1/projects/"+pid+"/relations/bulk", map[string]any{"relations": []any{rel(c2, parent, "refines")}}, 403)
	v.m("DELETE", "/api/v1/projects/"+pid+"/relations/bulk", map[string]any{"ids": ids[3:]}, 403)
}
