package api

import (
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func roleByName(list []map[string]any, name string) map[string]any {
	for _, r := range list {
		if r["name"] == name {
			return r
		}
	}
	return nil
}

func TestRolesManagement(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("ROL")
	e.member("viewer@example.com", rbac.RoleViewer)
	e.member("lead@example.com", rbac.RoleTestLead)
	e.member("mgr@example.com", rbac.RoleManager)
	v := e.login("viewer@example.com")

	roles := items(v.m("GET", "/api/v1/roles", nil, 200))
	if lead := roleByName(roles, "test_lead"); lead == nil || !strings.Contains(strings.Join(strs(lead["permissions"]), ","), "settings.manage") || lead["members"].(float64) != 1 {
		t.Fatalf("test lead: %v", roles)
	}
	if adm := roleByName(roles, "admin"); adm["editable"] != false || adm["deletable"] != false {
		t.Errorf("admin: %v", adm)
	}

	// custom role
	a.m("POST", "/api/v1/roles", map[string]any{"name": "Auditor", "permissions": []string{"project.read"}}, 400)
	a.m("POST", "/api/v1/roles", map[string]any{"name": "auditor", "permissions": []string{"nope"}}, 400)
	a.m("POST", "/api/v1/roles", map[string]any{"name": "tester", "permissions": []string{}}, 409)
	created := a.m("POST", "/api/v1/roles", map[string]any{"name": "auditor", "label": "Auditor", "permissions": []string{"audit.read", "project.read", "member.read"}}, 201)
	if created["label"] != "Auditor" || created["deletable"] != true {
		t.Fatal(created)
	}
	a.m("POST", "/api/v1/roles", map[string]any{"name": "auditor", "permissions": []string{}}, 409)
	a.m("POST", "/api/v1/members", map[string]any{"email": "aud@example.com", "role": "auditor", "password": testPassword}, 201)
	a.m("POST", "/api/v1/members", map[string]any{"email": "x@example.com", "role": "ghost", "password": testPassword}, 400)
	aud := e.login("aud@example.com")
	aud.m("GET", "/api/v1/audit", nil, 200)
	aud.m("POST", "/api/v1/projects", map[string]any{"key": "NOPE", "name": "x"}, 403)
	if meta := aud.m("GET", "/api/v1/meta", nil, 200); !strings.Contains(strings.Join(strs(meta["roles"]), ","), "auditor") {
		t.Errorf("meta roles: %v", meta["roles"])
	}
	if rb := aud.m("GET", "/api/v1/rbac", nil, 200); len(strs(rb["matrix"].(map[string]any)["auditor"])) != 3 {
		t.Errorf("rbac: %v", rb)
	}

	// change a built-in role: viewers may no longer export, testers no longer write tests
	a.m("PUT", "/api/v1/roles/admin", map[string]any{"permissions": []string{}}, 400)
	a.m("PUT", "/api/v1/roles/ghost", map[string]any{"permissions": []string{}}, 404)
	tester := a.m("PUT", "/api/v1/roles/tester", map[string]any{"permissions": []string{"project.read", "test.read", "run.read", "run.execute"}}, 200)
	if tester["customized"] != true {
		t.Error(tester)
	}
	e.member("tester@example.com", rbac.RoleTester)
	ts := e.login("tester@example.com")
	ts.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "x"}, 403)
	a.call("POST", "/api/v1/roles/tester/reset", nil, 204, nil)
	ts.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "x"}, 201)
	a.m("POST", "/api/v1/roles/auditor/reset", nil, 400)
	a.m("POST", "/api/v1/roles/admin/reset", nil, 400)
	upd := a.m("PUT", "/api/v1/roles/auditor", map[string]any{"label": "Audit", "description": "Reads the audit log"}, 200)
	if upd["label"] != "Audit" || len(strs(upd["permissions"])) != 3 {
		t.Error(upd)
	}

	// only role.manage may change roles, and nobody can grant what they do not hold
	mgr := e.login("mgr@example.com")
	mgr.m("POST", "/api/v1/roles", map[string]any{"name": "x1", "permissions": []string{}}, 403)
	a.m("POST", "/api/v1/roles", map[string]any{"name": "rolekeeper", "permissions": []string{"role.manage", "project.read", "member.read"}}, 201)
	a.m("POST", "/api/v1/members", map[string]any{"email": "keeper@example.com", "role": "rolekeeper", "password": testPassword}, 201)
	k := e.login("keeper@example.com")
	k.m("PUT", "/api/v1/roles/rolekeeper", map[string]any{"permissions": []string{"role.manage", "project.read", "member.read", "tenant.manage"}}, 403)
	k.m("POST", "/api/v1/roles", map[string]any{"name": "big", "permissions": []string{"member.manage"}}, 403)
	k.m("PUT", "/api/v1/roles/rolekeeper", map[string]any{"permissions": []string{"role.manage", "project.read"}}, 200) // removing is fine
	k.m("POST", "/api/v1/roles/manager/reset", nil, 403)

	// tokens cannot exceed the creator's permissions
	aud.m("POST", "/api/v1/tokens", map[string]any{"name": "t", "role": "tester"}, 403) // auditor lacks token.manage
	ts.m("POST", "/api/v1/tokens", map[string]any{"name": "t", "role": "manager"}, 400)
	tok := ts.m("POST", "/api/v1/tokens", map[string]any{"name": "t", "role": "viewer"}, 201)
	b := e.anon()
	b.bearer = tok["token"].(string)
	b.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "x"}, 403)

	// delete: built-in no, in use no, then yes
	a.m("DELETE", "/api/v1/roles/viewer", nil, 400)
	a.m("DELETE", "/api/v1/roles/auditor", nil, 409)
	audUser, _ := e.st.GetUserByEmail(t.Context(), "aud@example.com")
	a.m("PATCH", "/api/v1/members/"+audUser.ID, map[string]any{"role": "viewer"}, 200)
	a.m("PATCH", "/api/v1/members/"+audUser.ID, map[string]any{"role": "ghost"}, 400)
	a.call("DELETE", "/api/v1/roles/auditor", nil, 204, nil)
	a.m("DELETE", "/api/v1/roles/auditor", nil, 404)
	audit := items(a.m("GET", "/api/v1/audit?q=action:role.*", nil, 200))
	if len(audit) < 5 {
		t.Errorf("audit: %d", len(audit))
	}
}

func TestTenantSettings(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	e.member("lead@example.com", rbac.RoleTestLead)
	e.member("tester@example.com", rbac.RoleTester)
	a.m("PUT", "/api/v1/settings", map[string]any{}, 200) // back to the strict defaults
	st := a.m("GET", "/api/v1/settings", nil, 200)
	if st["allow_unready_tests"] != false || st["allow_unmet_dependencies"] != false || strings.Join(strs(st["ready_requirement_statuses"]), ",") != "approved,implemented,verified" {
		t.Fatal(st)
	}
	lead := e.login("lead@example.com")
	got := lead.m("PUT", "/api/v1/settings", map[string]any{"allow_unready_tests": true, "ready_requirement_statuses": []string{"approved", "approved", "review"}}, 200)
	if got["allow_unready_tests"] != true || strings.Join(strs(got["ready_requirement_statuses"]), ",") != "approved,review" {
		t.Fatal(got)
	}
	lead.m("PUT", "/api/v1/settings", map[string]any{"ready_requirement_statuses": []string{"bogus"}}, 400)
	e.login("tester@example.com").m("PUT", "/api/v1/settings", map[string]any{}, 403)
	if st := e.login("tester@example.com").m("GET", "/api/v1/settings", nil, 200); st["allow_unready_tests"] != true {
		t.Error(st)
	}
	// empty list falls back to the defaults
	if got := a.m("PUT", "/api/v1/settings", map[string]any{"ready_requirement_statuses": []string{}}, 200); len(strs(got["ready_requirement_statuses"])) != 3 {
		t.Error(got)
	}
}
