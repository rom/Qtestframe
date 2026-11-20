package api

import (
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func TestBulkLinks(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BLK")
	other := a.project("BLO")
	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Login", "description": "The user MUST log in."}, 201)["id"].(string)
	r2 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Logout"}, 201)["id"].(string)
	t1 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "T1"}, 201)["id"].(string)
	t2 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "T2"}, 201)["id"].(string)
	t3 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "T3"}, 201)["id"].(string)
	foreign := a.m("POST", "/api/v1/projects/"+other+"/tests", map[string]any{"title": "X"}, 201)["id"].(string)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": r1, "test_case_id": t1}, 201)

	pair := func(r, t string) map[string]any { return map[string]any{"requirement_id": r, "test_case_id": t} }
	res := a.m("POST", "/api/v1/projects/"+pid+"/links/bulk", map[string]any{"links": []any{
		pair(r1, t1), pair(r1, t2), pair(r2, t1), pair(r2, t2), pair(r2, foreign), pair(r2, t2),
	}}, 200)
	created, skipped := res["created"].([]any), res["skipped"].([]any)
	if len(created) != 3 || len(skipped) != 3 {
		t.Fatal(res)
	}
	reasons := map[string]int{}
	for _, x := range skipped {
		reasons[x.(map[string]any)["reason"].(string)]++
	}
	if reasons["exists"] != 2 || reasons["not_found"] != 1 {
		t.Fatal(reasons)
	}
	if n := len(a.m("GET", "/api/v1/projects/"+pid+"/links", nil, 200)["items"].([]any)); n != 4 {
		t.Fatal(n)
	}
	a.m("POST", "/api/v1/projects/"+pid+"/links/bulk", map[string]any{"links": []any{}}, 400)
	bad := a.m("POST", "/api/v1/projects/"+pid+"/links/bulk", map[string]any{"links": []any{map[string]any{"requirement_id": r2, "test_case_id": t3, "criterion_id": "nope"}}}, 200)
	if bad["skipped"].([]any)[0].(map[string]any)["reason"] != "invalid" {
		t.Fatal(bad)
	}

	// a text change makes the links of r1 suspect; confirm them in bulk
	a.m("PATCH", "/api/v1/requirements/"+r1, map[string]any{"description": "The user MUST log in with 2FA."}, 200)
	var all, suspect []string
	for _, x := range a.m("GET", "/api/v1/projects/"+pid+"/links", nil, 200)["items"].([]any) {
		l := x.(map[string]any)
		all = append(all, l["id"].(string))
		if l["suspect_since"] != nil {
			suspect = append(suspect, l["id"].(string))
		}
	}
	if len(suspect) != 2 {
		t.Fatal(suspect)
	}
	c := a.m("POST", "/api/v1/projects/"+pid+"/links/confirm", map[string]any{"ids": all}, 200)
	if c["confirmed"].(float64) != 2 {
		t.Fatal(c)
	}
	// links of another project are not touched
	c = a.m("POST", "/api/v1/projects/"+other+"/links/confirm", map[string]any{"ids": all}, 200)
	if c["confirmed"].(float64) != 0 {
		t.Fatal(c)
	}

	// unlink in bulk; unknown ids are skipped
	d := a.m("DELETE", "/api/v1/projects/"+pid+"/links/bulk", map[string]any{"ids": append(all[:3:3], "nope")}, 200)
	if len(d["deleted"].([]any)) != 3 || len(d["skipped"].([]any)) != 1 {
		t.Fatal(d)
	}
	if d["deleted"].([]any)[0].(map[string]any)["requirement_id"] == "" {
		t.Fatal(d)
	}
	if n := len(a.m("GET", "/api/v1/projects/"+pid+"/links", nil, 200)["items"].([]any)); n != 1 {
		t.Fatal(n)
	}
	a.m("DELETE", "/api/v1/projects/"+other+"/links/bulk", map[string]any{"ids": all[3:]}, 200)
	if n := len(a.m("GET", "/api/v1/projects/"+pid+"/links", nil, 200)["items"].([]any)); n != 1 {
		t.Fatal(n)
	}

	// viewers may not link
	e.member("v@example.com", rbac.RoleViewer)
	v := e.login("v@example.com")
	v.m("POST", "/api/v1/projects/"+pid+"/links/bulk", map[string]any{"links": []any{pair(r2, t2)}}, 403)
}
