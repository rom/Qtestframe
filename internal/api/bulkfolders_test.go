package api

import (
	"testing"
)

func TestBulkFolders(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BFO")
	other := a.project("BFX")
	mk := func(kind, name string, parent any) string {
		return a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": kind, "name": name, "parent_id": parent}, 201)["id"].(string)
	}
	fa := mk("requirement", "A", nil)
	fa1 := mk("requirement", "A1", fa)
	fb := mk("requirement", "B", nil)
	fc := mk("requirement", "C", nil)
	ft := mk("test", "T", nil)
	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "In A1", "folder_id": fa1}, 201)["id"].(string)
	r2 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "In B", "folder_id": fb}, 201)["id"].(string)
	path := "/api/v1/projects/" + pid + "/folders/bulk"
	parentOf := func(id string) any {
		for _, f := range items(a.m("GET", "/api/v1/projects/"+pid+"/folders", nil, 200)) {
			if f["id"] == id {
				return f["parent_id"]
			}
		}
		t.Fatal("folder gone", id)
		return nil
	}

	a.m("POST", path, map[string]any{"ids": []string{fa}, "action": "nope"}, 400)
	a.m("POST", path, map[string]any{"ids": []string{fa, ft}, "action": "delete"}, 400)
	a.m("POST", path, map[string]any{"ids": []string{"missing"}, "action": "delete"}, 404)
	// into a selected folder or its subfolder: nothing moves
	a.m("POST", path, map[string]any{"ids": []string{fa, fb}, "action": "move", "parent_id": fa1}, 409)
	if parentOf(fb) != nil {
		t.Fatal("B moved")
	}

	// A (with A1, also selected) and B into C: A1 stays inside A
	if res := a.m("POST", path, map[string]any{"ids": []string{fa, fa1, fb}, "action": "move", "parent_id": fc}, 200); res["moved"].(float64) != 2 {
		t.Fatal(res)
	}
	if parentOf(fa) != fc || parentOf(fb) != fc || parentOf(fa1) != fa {
		t.Fatal("wrong tree")
	}
	// back to the top level
	a.m("POST", path, map[string]any{"ids": []string{fa, fb}, "action": "move", "parent_id": ""}, 200)
	if parentOf(fa) != nil {
		t.Fatal("not at top")
	}
	// testers cannot move requirement folders
	e.member("tester@example.com", "tester")
	e.login("tester@example.com").m("POST", path, map[string]any{"ids": []string{fa}, "action": "move", "parent_id": fc}, 403)

	// to another project, with the requirements inside
	if res := a.m("POST", path, map[string]any{"ids": []string{fb}, "action": "move", "project_id": other}, 200); res["moved"].(float64) != 1 {
		t.Fatal(res)
	}
	if got := a.m("GET", "/api/v1/requirements/"+r2, nil, 200); got["project_id"] != other {
		t.Fatal(got)
	}
	// delete without contents keeps the requirement; with cascade deletes it
	a.m("POST", path, map[string]any{"ids": []string{fc}, "action": "delete"}, 200)
	if res := a.m("POST", path, map[string]any{"ids": []string{fa, fa1}, "action": "delete", "cascade": true}, 200); res["deleted"].(float64) != 1 {
		t.Fatal(res)
	}
	a.m("GET", "/api/v1/requirements/"+r1, nil, 404)
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/folders?kind=requirement", nil, 200)); len(l) != 0 {
		t.Fatal(l)
	}
}
