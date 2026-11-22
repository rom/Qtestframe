package api

import (
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

// TestProjectCreatorAPI: a project shows who started it (created, loaded
// or duplicated it) and when.
func TestProjectCreatorAPI(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	created := a.m("POST", "/api/v1/projects", map[string]any{"key": "ORIG", "name": "Original"}, 201)
	if created["created_by"] != e.admin.ID || created["created_by_name"] != "admin" || created["created_by_email"] != "admin@example.com" || created["created_at"] == "" {
		t.Fatal(created)
	}
	pid := created["id"].(string)
	got := a.m("GET", "/api/v1/projects/"+pid, nil, 200)
	if got["created_by_name"] != "admin" || got["created_by"] != e.admin.ID {
		t.Fatal(got)
	}

	// another administrator loads the project from an archive: they started
	// the loaded copy
	other := e.member("other@example.com", rbac.RoleAdmin)
	o := e.login("other@example.com")
	_, archive := a.getBytes("/api/v1/projects/"+pid+"/archive", 200)
	out := o.upload("POST", "/api/v1/projects/import", "archive", "orig.zip", "application/zip", archive, map[string]string{"key": "LOADED"}, 201)
	if lp := out["project"].(map[string]any); lp["created_by"] != other.ID || lp["created_by_email"] != "other@example.com" {
		t.Fatal(lp)
	}
	dup := o.m("POST", "/api/v1/projects/"+pid+"/duplicate", map[string]any{"key": "DUP"}, 201)
	if dp := dup["project"].(map[string]any); dp["created_by"] != other.ID {
		t.Fatal(dp)
	}
	byKey := map[string]map[string]any{}
	for _, p := range items(a.m("GET", "/api/v1/projects", nil, 200)) {
		byKey[p["key"].(string)] = p
	}
	if byKey["ORIG"]["created_by"] != e.admin.ID || byKey["LOADED"]["created_by"] != other.ID || byKey["DUP"]["created_by_name"] != "other" {
		t.Fatal(byKey)
	}
}
