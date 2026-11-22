package api

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

func memberRow(t *testing.T, c *client, uid string) map[string]any {
	t.Helper()
	for _, m := range items(c.m("GET", "/api/v1/members", nil, 200)) {
		if m["user_id"] == uid {
			return m
		}
	}
	return nil
}

// TestMultipleRoles: a member holds several organisation roles and gets
// the union of their permissions; "role" keeps working for older clients.
func TestMultipleRoles(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("SME")
	pp := "/api/v1/projects/" + pid

	// add with several roles; the primary role is the strongest built-in one
	res := a.m("POST", "/api/v1/members", map[string]any{"email": "sara@example.com", "roles": []string{"sme", "tester"}, "password": testPassword}, 201)
	sara := res["user_id"].(string)
	if res["role"] != "tester" || strings.Join(strs(res["roles"]), ",") != "tester,sme" {
		t.Fatal(res)
	}
	sc := e.login("sara@example.com")
	me := sc.me(t)
	perms := strs(me["permissions"])
	for _, p := range []rbac.Permission{rbac.RequirementWr, rbac.RequirementRev, rbac.RunExecute, rbac.TestWrite, rbac.CommentCreate} {
		if !slices.Contains(perms, string(p)) {
			t.Errorf("union lacks %s: %v", p, perms)
		}
	}
	if slices.Contains(perms, string(rbac.CyclePlan)) || slices.Contains(perms, string(rbac.MemberManage)) || strings.Join(strs(me["roles"]), ",") != "tester,sme" {
		t.Fatal(me)
	}
	// the SME part: requirements; the tester part: tests
	rq := sc.m("POST", pp+"/requirements", map[string]any{"title": "Written by the expert"}, 201)["id"].(string)
	sc.m("POST", pp+"/tests", map[string]any{"title": "Written by the tester"}, 201)

	// PATCH with roles; with the deprecated role (one role)
	a.m("PATCH", "/api/v1/members/"+sara, map[string]any{"roles": []string{"sme"}}, 200)
	sc.m("POST", pp+"/tests", map[string]any{"title": "no longer"}, 403)
	sc.m("PATCH", "/api/v1/requirements/"+rq, map[string]any{"title": "Refined by the expert"}, 200)
	m := a.m("PATCH", "/api/v1/members/"+sara, map[string]any{"role": "viewer"}, 200)
	if m["role"] != "viewer" || strings.Join(strs(m["roles"]), ",") != "viewer" {
		t.Fatal(m)
	}
	a.m("PATCH", "/api/v1/members/"+sara, map[string]any{"roles": []string{"sme", "viewer", "sme"}}, 200)
	if row := memberRow(t, a, sara); strings.Join(strs(row["roles"]), ",") != "sme,viewer" {
		t.Fatal(row)
	}
	a.m("PATCH", "/api/v1/members/"+sara, map[string]any{"roles": []string{}}, 400)
	a.m("PATCH", "/api/v1/members/"+sara, map[string]any{"roles": []string{"sme", "wizard"}}, 400)
	a.m("POST", "/api/v1/members", map[string]any{"email": "x@example.com"}, 400)
	// the audit log shows the added and removed roles
	if ds := auditDetails(t, e, "member.role"); !slices.ContainsFunc(ds, func(d string) bool { return strings.Contains(d, "+viewer") && strings.Contains(d, "−sme") }) {
		t.Fatal(ds)
	}
	// the role usage counts a member for each of their roles
	for _, r := range items(a.m("GET", "/api/v1/roles", nil, 200)) {
		if (r["name"] == "sme" || r["name"] == "viewer") && r["members"] != float64(1) {
			t.Errorf("usage %v", r)
		}
	}
	// a custom role held among others cannot be deleted
	a.m("POST", "/api/v1/roles", map[string]any{"name": "auditor", "permissions": []string{"project.read", "audit.read"}}, 201)
	a.m("PATCH", "/api/v1/members/"+sara, map[string]any{"roles": []string{"sme", "auditor"}}, 200)
	a.m("DELETE", "/api/v1/roles/auditor", nil, 409)
	if !slices.Contains(strs(e.login("sara@example.com").me(t)["permissions"]), "audit.read") {
		t.Fatal("custom role not in the union")
	}
	// MFA policies cover a member through any of their roles
	a.m("PUT", "/api/v1/admin/mfa/tenant", map[string]any{"mode": "roles", "roles": []string{"auditor"}, "grace_days": 30}, 200)
	if row := memberRow(t, a, sara); row["mfa_required"] != true {
		t.Fatal(row)
	}
}

// TestMultipleRolesLastAdmin: the last-admin protection looks at the
// admin role among a member's roles.
func TestMultipleRolesLastAdmin(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	admin := e.admin.ID
	// keeping admin among other roles is fine; dropping it is not
	if m := a.m("PATCH", "/api/v1/members/"+admin, map[string]any{"roles": []string{"sme", "admin"}}, 200); m["role"] != "admin" || len(strs(m["roles"])) != 2 {
		t.Fatal(m)
	}
	a.m("PATCH", "/api/v1/members/"+admin, map[string]any{"roles": []string{"sme"}}, 409)
	a.m("DELETE", "/api/v1/members/"+admin, nil, 409)
	// re-adding the last admin by e-mail with other roles is refused too
	a.m("POST", "/api/v1/members", map[string]any{"email": "admin@example.com", "roles": []string{"viewer"}}, 409)
	a.m("POST", "/api/v1/members/bulk", map[string]any{"user_ids": []string{admin}, "action": "remove_role", "roles": []string{"admin"}}, 200) // self: skipped
	// a second admin who also tests
	other := e.member("lead@example.com", rbac.RoleTester)
	a.m("PATCH", "/api/v1/members/"+other.ID, map[string]any{"roles": []string{"tester", "admin"}}, 200)
	if n, err := e.st.CountTenantAdmins(context.Background(), e.tenant.ID); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	oc := e.login("lead@example.com")
	oc.m("PATCH", "/api/v1/members/"+admin, map[string]any{"roles": []string{"sme"}}, 200)
	// now lead is the last admin: a bulk removal of the role is refused
	a2 := e.member("hr@example.com", rbac.RoleViewer)
	oc.m("PATCH", "/api/v1/members/"+a2.ID, map[string]any{"roles": []string{"viewer"}}, 200)
	oc.m("POST", "/api/v1/roles", map[string]any{"name": "people", "permissions": []string{"project.read", "member.read", "member.manage"}}, 201)
	oc.m("PATCH", "/api/v1/members/"+a2.ID, map[string]any{"roles": []string{"people"}}, 200)
	hr := e.login("hr@example.com")
	hr.m("POST", "/api/v1/members/bulk", map[string]any{"user_ids": []string{other.ID}, "action": "remove_role", "roles": []string{"admin"}}, 409)
	hr.m("POST", "/api/v1/members/bulk", map[string]any{"user_ids": []string{other.ID}, "action": "role", "roles": []string{"tester"}}, 409)
	// adding a role in bulk keeps admin
	res := hr.m("POST", "/api/v1/members/bulk", map[string]any{"user_ids": []string{other.ID, admin}, "action": "add_role", "roles": []string{"sme"}}, 200)
	if len(res["changed"].([]any)) != 1 { // admin@ already has sme
		t.Fatal(res)
	}
	if row := memberRow(t, oc, other.ID); strings.Join(strs(row["roles"]), ",") != "admin,tester,sme" {
		t.Fatal(row)
	}
	// taking the only role away is skipped
	res = hr.m("POST", "/api/v1/members/bulk", map[string]any{"user_ids": []string{admin}, "action": "remove_role", "roles": []string{"sme"}}, 200)
	if sk := res["skipped"].([]any); len(sk) != 1 || sk[0].(map[string]any)["reason"] != "last_role" {
		t.Fatal(res)
	}
	hr.m("POST", "/api/v1/members/bulk", map[string]any{"user_ids": []string{admin}, "action": "add_role"}, 400)
}

// TestMultipleProjectRoles: project roles are sets too and replace the
// organisation roles' project permissions inside the project.
func TestMultipleProjectRoles(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("VLT")
	open := a.project("OPN")
	pp := "/api/v1/projects/" + pid
	rq := a.m("POST", pp+"/requirements", map[string]any{"title": "R"}, 201)["id"].(string)
	a.m("PUT", pp+"/access", map[string]any{"access": "restricted"}, 200)
	sara := e.member("sara@example.com", rbac.RoleTester)
	a.m("PATCH", "/api/v1/members/"+sara.ID, map[string]any{"roles": []string{"sme", "tester"}}, 200)
	sc := e.login("sara@example.com")
	sc.m("GET", pp, nil, 404)

	pm := a.m("POST", pp+"/members", map[string]any{"user_id": sara.ID, "roles": []string{"reviewer", "sme"}}, 201)
	if pm["role"] != "sme" || strings.Join(strs(pm["roles"]), ",") != "sme,reviewer" || strings.Join(strs(pm["org_roles"]), ",") != "tester,sme" {
		t.Fatal(pm)
	}
	my := sc.m("GET", pp, nil, 200)["my_access"].(map[string]any)
	perms := strs(my["permissions"])
	if my["source"] != "member" || strings.Join(strs(my["roles"]), ",") != "sme,reviewer" || my["role"] != "sme, reviewer" ||
		!slices.Contains(perms, string(rbac.RequirementWr)) || slices.Contains(perms, string(rbac.RunExecute)) || slices.Contains(perms, string(rbac.TestWrite)) {
		t.Fatal(my)
	}
	sc.m("PATCH", "/api/v1/requirements/"+rq, map[string]any{"title": "by the expert"}, 200)
	sc.m("POST", pp+"/tests", map[string]any{"title": "not here"}, 403)
	// in an organisation-wide project her organisation roles apply
	sc.m("POST", "/api/v1/projects/"+open+"/tests", map[string]any{"title": "here"}, 201)

	// change the set; the audit log shows the difference
	pm = a.m("PATCH", pp+"/members/"+sara.ID, map[string]any{"roles": []string{"sme", "tester"}}, 200)
	if strings.Join(strs(pm["roles"]), ",") != "tester,sme" {
		t.Fatal(pm)
	}
	sc.m("POST", pp+"/tests", map[string]any{"title": "now allowed"}, 201)
	if ds := auditDetails(t, e, "project.member.role"); !slices.ContainsFunc(ds, func(d string) bool { return strings.Contains(d, "+tester") && strings.Contains(d, "−reviewer") }) {
		t.Fatal(ds)
	}
	a.m("PATCH", pp+"/members/"+sara.ID, map[string]any{"roles": []string{}}, 400)
	// the deprecated single role still works
	a.m("PATCH", pp+"/members/"+sara.ID, map[string]any{"role": "viewer"}, 200)
	sc.m("PATCH", "/api/v1/requirements/"+rq, map[string]any{"title": "x"}, 403)

	// a project manager (not an admin) can give only roles within their project permissions
	mgr := e.member("mgr@example.com", rbac.RoleViewer)
	a.m("POST", pp+"/members", map[string]any{"user_id": mgr.ID, "roles": []string{"manager"}}, 201)
	mc := e.login("mgr@example.com")
	acc := mc.m("GET", pp+"/access", nil, 200)
	if g := strs(acc["grantable_roles"]); slices.Contains(g, "admin") || !slices.Contains(g, "sme") {
		t.Fatal(g)
	}
	mc.m("PATCH", pp+"/members/"+sara.ID, map[string]any{"roles": []string{"sme", "admin"}}, 403)
	mc.m("PATCH", pp+"/members/"+sara.ID, map[string]any{"roles": []string{"sme", "viewer"}}, 200)
	// bulk: add and take away project roles
	res := mc.m("POST", pp+"/members/bulk", map[string]any{"ids": []string{sara.ID}, "action": "add_role", "roles": []string{"tester"}}, 200)
	if ch := res["changed"].([]any); len(ch) != 1 || strings.Join(strs(ch[0].(map[string]any)["roles"]), ",") != "tester,sme,viewer" {
		t.Fatal(res)
	}
	res = mc.m("POST", pp+"/members/bulk", map[string]any{"ids": []string{sara.ID}, "action": "remove_role", "roles": []string{"tester", "viewer"}}, 200)
	if ch := res["changed"].([]any); len(ch) != 1 || ch[0].(map[string]any)["role"] != "sme" {
		t.Fatal(res)
	}
	res = mc.m("POST", pp+"/members/bulk", map[string]any{"ids": []string{sara.ID}, "action": "remove_role", "roles": []string{"sme"}}, 200)
	if sk := res["skipped"].([]any); len(sk) != 1 || sk[0].(map[string]any)["reason"] != "last_role" {
		t.Fatal(res)
	}
	mc.m("POST", pp+"/members/bulk", map[string]any{"ids": []string{sara.ID}, "action": "add_role", "roles": []string{"admin"}}, 403)
	// a member holding a role the caller cannot give is not removed by them
	a.m("PATCH", pp+"/members/"+sara.ID, map[string]any{"roles": []string{"sme", "admin"}}, 200)
	mc.m("DELETE", pp+"/members/"+sara.ID, nil, 403)
	res = mc.m("POST", pp+"/members/bulk", map[string]any{"ids": []string{sara.ID}, "action": "add_role", "roles": []string{"viewer"}}, 200)
	if len(res["changed"].([]any)) != 1 { // adding is fine, nothing is taken
		t.Fatal(res)
	}
	res = mc.m("POST", pp+"/members/bulk", map[string]any{"ids": []string{sara.ID}, "action": "role", "roles": []string{"viewer"}}, 200)
	if sk := res["skipped"].([]any); len(sk) != 1 || sk[0].(map[string]any)["reason"] != "forbidden" {
		t.Fatal(res)
	}
	// candidates show the organisation roles
	a.m("DELETE", pp+"/members/"+sara.ID, nil, 204)
	for _, c := range items(a.m("GET", pp+"/members/candidates?q=sara", nil, 200)) {
		if strings.Join(strs(c["org_roles"]), ",") != "tester,sme" {
			t.Fatal(c)
		}
	}
}

// TestMultipleRolesTokensAndGroups: API tokens hold role sets capped by
// the user's roles; several project groups and SCIM groups union.
func TestMultipleRolesTokensAndGroups(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.login("admin@example.com")
	pid := a.project("TOK")
	pp := "/api/v1/projects/" + pid
	sara := e.member("sara@example.com", rbac.RoleTester)
	if err := e.st.SetMembershipRoles(ctx, e.tenant.ID, sara.ID, []string{"sme", "tester"}); err != nil {
		t.Fatal(err)
	}
	sc := e.login("sara@example.com")
	// without roles a token gets all of the user's roles
	res := sc.m("POST", "/api/v1/tokens", map[string]any{"name": "all"}, 201)
	if it := res["item"].(map[string]any); strings.Join(strs(it["roles"]), ",") != "tester,sme" || it["role"] != "tester" {
		t.Fatal(it)
	}
	b := e.anon()
	b.bearer = res["token"].(string)
	b.m("POST", pp+"/requirements", map[string]any{"title": "via token"}, 201)
	b.m("POST", pp+"/tests", map[string]any{"title": "via token"}, 201)
	// a token with the SME role only
	res = sc.m("POST", "/api/v1/tokens", map[string]any{"name": "sme", "roles": []string{"sme"}}, 201)
	c := e.anon()
	c.bearer = res["token"].(string)
	c.m("POST", pp+"/requirements", map[string]any{"title": "sme token"}, 201)
	c.m("POST", pp+"/tests", map[string]any{"title": "no"}, 403)
	sc.m("POST", "/api/v1/tokens", map[string]any{"name": "too much", "roles": []string{"sme", "manager"}}, 400)
	sc.m("POST", "/api/v1/tokens", map[string]any{"name": "bad", "roles": []string{"wizard"}}, 400)
	// the user loses the tester role: the "all" token loses it too
	if err := e.st.SetMembershipRoles(ctx, e.tenant.ID, sara.ID, []string{"sme"}); err != nil {
		t.Fatal(err)
	}
	b.m("POST", pp+"/tests", map[string]any{"title": "gone"}, 403)
	b.m("POST", pp+"/requirements", map[string]any{"title": "still"}, 201)

	// two SCIM groups in a restricted project: the union of their roles
	a.m("PUT", pp+"/access", map[string]any{"access": "restricted"}, 200)
	g1 := &model.SCIMGroup{TenantID: e.tenant.ID, DisplayName: "Experts", Members: []string{sara.ID}}
	g2 := &model.SCIMGroup{TenantID: e.tenant.ID, DisplayName: "Testers", Members: []string{sara.ID}}
	for _, g := range []*model.SCIMGroup{g1, g2} {
		if err := e.st.CreateSCIMGroup(ctx, g); err != nil {
			t.Fatal(err)
		}
	}
	a.m("POST", pp+"/members", map[string]any{"group_id": g1.ID, "roles": []string{"sme"}}, 201)
	a.m("POST", pp+"/members", map[string]any{"group_id": g2.ID, "roles": []string{"tester", "reviewer"}}, 201)
	my := e.login("sara@example.com").m("GET", pp, nil, 200)["my_access"].(map[string]any)
	if my["source"] != "group" || strings.Join(strs(my["roles"]), ",") != "tester,sme,reviewer" || !slices.Contains(strs(my["permissions"]), "run.execute") {
		t.Fatal(my)
	}
}

// TestSCIMSeveralGroupsSeveralRoles: a provisioned user in two mapped
// groups holds both roles; the read-only roles attribute lists them.
func TestSCIMSeveralGroupsSeveralRoles(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin@example.com")
	sc := e.scim(e.scimToken(admin, "idp"))
	u := sc.want("POST", "/Users", entraUser("sara@example.com", "x1"), 201)["id"].(string)
	g1 := sc.want("POST", "/Groups", map[string]any{"displayName": "Experts", "members": []any{map[string]any{"value": u}}}, 201)["id"].(string)
	g2 := sc.want("POST", "/Groups", map[string]any{"displayName": "Testers", "members": []any{map[string]any{"value": u}}}, 201)["id"].(string)
	admin.m("PATCH", "/api/v1/scim/groups/"+g1, map[string]any{"role": rbac.RoleSME}, 200)
	admin.m("PATCH", "/api/v1/scim/groups/"+g2, map[string]any{"role": rbac.RoleTester}, 200)
	m, err := e.st.GetMembership(context.Background(), e.tenant.ID, u)
	if err != nil || strings.Join(m.Roles, ",") != "tester,sme" {
		t.Fatal(m, err)
	}
	roles := sc.want("GET", "/Users/"+u, nil, 200)["roles"].([]any)
	if len(roles) != 2 || roles[0].(map[string]any)["value"] != "tester" || roles[0].(map[string]any)["primary"] != true || roles[1].(map[string]any)["value"] != "sme" {
		t.Fatal(roles)
	}
	if list := listResources(t, sc.want("GET", "/Users", nil, 200)); len(list[0]["roles"].([]any)) != 2 {
		t.Fatal(list)
	}
	// leaving one group takes its role away
	sc.want("PATCH", "/Groups/"+g2, patch(map[string]any{"op": "remove", "path": "members", "value": []any{map[string]any{"value": u}}}), 200)
	if m, _ := e.st.GetMembership(context.Background(), e.tenant.ID, u); strings.Join(m.Roles, ",") != "sme" {
		t.Fatal(m)
	}
	// hand-made changes of the roles are refused for provisioned members
	admin.m("PATCH", "/api/v1/members/"+u, map[string]any{"roles": []string{"sme", "tester"}}, 409)
}
