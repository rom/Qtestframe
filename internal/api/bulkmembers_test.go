package api

import (
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func TestBulkMembers(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	t1 := e.member("t1@example.com", rbac.RoleTester)
	t2 := e.member("t2@example.com", rbac.RoleTester)
	v := e.member("v@example.com", rbac.RoleViewer)
	ad2 := e.member("ad2@example.com", rbac.RoleAdmin)
	role := func(uid string) string {
		for _, m := range items(a.m("GET", "/api/v1/members", nil, 200)) {
			if m["user_id"] == uid {
				return m["role"].(string)
			}
		}
		return ""
	}
	reasons := func(res map[string]any) map[string]int {
		out := map[string]int{}
		for _, x := range res["skipped"].([]any) {
			out[x.(map[string]any)["reason"].(string)]++
		}
		return out
	}

	res := a.m("POST", "/api/v1/members/bulk", map[string]any{"user_ids": []string{t1.ID, t2.ID, v.ID, e.admin.ID, "nobody"}, "action": "role", "role": "viewer"}, 200)
	if len(res["changed"].([]any)) != 2 || role(t1.ID) != "viewer" || role(t2.ID) != "viewer" {
		t.Fatal(res)
	}
	if r := reasons(res); r["unchanged"] != 1 || r["self"] != 1 || r["not_found"] != 1 {
		t.Fatal(r)
	}
	a.m("POST", "/api/v1/members/bulk", map[string]any{"user_ids": []string{t1.ID}, "action": "role", "role": "wizard"}, 400)
	a.m("POST", "/api/v1/members/bulk", map[string]any{"user_ids": []string{t1.ID}, "action": "promote"}, 400)
	a.m("POST", "/api/v1/members/bulk", map[string]any{"user_ids": []string{}, "action": "remove"}, 400)

	// remove two members, the caller is kept
	res = a.m("POST", "/api/v1/members/bulk", map[string]any{"user_ids": []string{t1.ID, ad2.ID, e.admin.ID}, "action": "remove"}, 200)
	if len(res["changed"].([]any)) != 2 || role(t1.ID) != "" || role(ad2.ID) != "" || role(e.admin.ID) != "admin" {
		t.Fatal(res)
	}

	// someone who may manage members but is no admin cannot remove the last admin
	a.m("POST", "/api/v1/roles", map[string]any{"name": "people", "permissions": []string{"project.read", "member.read", "member.manage"}}, 201)
	e.member("hr@example.com", "people")
	hr := e.login("hr@example.com")
	hr.m("POST", "/api/v1/members/bulk", map[string]any{"user_ids": []string{e.admin.ID, t2.ID}, "action": "remove"}, 409)
	if role(e.admin.ID) != "admin" || role(t2.ID) != "viewer" {
		t.Fatal("changed despite the refusal")
	}
	hr.m("POST", "/api/v1/members/bulk", map[string]any{"user_ids": []string{e.admin.ID}, "action": "role", "role": "viewer"}, 409)

	// others may not
	e.member("t3@example.com", rbac.RoleTester)
	e.login("t3@example.com").m("POST", "/api/v1/members/bulk", map[string]any{"user_ids": []string{t2.ID}, "action": "remove"}, 403)
}
