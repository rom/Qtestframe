package api

import (
	"testing"
)

func TestBulkCategories(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BCAT")
	mk := func(scope, dim, name string) string {
		return a.m("POST", "/api/v1/projects/"+pid+"/categories", map[string]any{"scope": scope, "dimension": dim, "name": name}, 201)["id"].(string)
	}
	ui := mk("requirement", "Zone", "UI")
	ui2 := mk("requirement", "Zone", "User interface")
	gui := mk("requirement", "Legacy", "GUI")
	api := mk("requirement", "Legacy", "API")
	mk("requirement", "Channel", "API") // the target dimension already has an API
	tst := mk("test", "Suite", "Smoke")
	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "R1", "category_ids": []string{ui2, gui}}, 201)["id"].(string)
	r2 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "R2", "category_ids": []string{ui, ui2}}, 201)["id"].(string)
	path := "/api/v1/projects/" + pid + "/categories/bulk"
	cats := func(id string) []string {
		return strs(a.m("GET", "/api/v1/requirements/"+id, nil, 200)["category_ids"])
	}

	a.m("POST", path, map[string]any{"ids": []string{}, "action": "delete"}, 400)
	a.m("POST", path, map[string]any{"ids": []string{ui}, "action": "nope"}, 400)
	a.m("POST", path, map[string]any{"ids": []string{ui, tst}, "action": "delete"}, 400) // mixed scopes
	a.m("POST", path, map[string]any{"ids": []string{ui}, "action": "color", "color": "red"}, 400)
	a.m("POST", path, map[string]any{"ids": []string{ui}, "action": "merge", "target_id": tst}, 400)

	// merge Ui and GUI into UI: items keep one UI assignment
	if res := a.m("POST", path, map[string]any{"ids": []string{ui, ui2, gui}, "action": "merge", "target_id": ui}, 200); res["changed"].(float64) != 2 {
		t.Fatal(res)
	}
	if c := cats(r1); len(c) != 1 || c[0] != ui {
		t.Fatal(c)
	}
	if c := cats(r2); len(c) != 1 || c[0] != ui {
		t.Fatal(c)
	}
	// move Legacy/API to Interface: merged with the existing Interface/API
	a.m("PATCH", "/api/v1/requirements/"+r1, map[string]any{"category_ids": []string{ui, api}}, 200)
	a.m("POST", path, map[string]any{"ids": []string{api, ui}, "action": "move", "dimension": "Channel"}, 200)
	list := items(a.m("GET", "/api/v1/projects/"+pid+"/categories", nil, 200))
	byDim := map[string][]string{}
	for _, c := range list {
		if c["scope"] == "requirement" {
			byDim[c["dimension"].(string)] = append(byDim[c["dimension"].(string)], c["name"].(string))
		}
	}
	if len(byDim["Channel"]) != 2 || len(byDim["Legacy"]) != 0 || len(byDim["Zone"]) != 0 {
		t.Fatal(byDim)
	}
	if c := cats(r1); len(c) != 2 {
		t.Fatal(c)
	}
	// colour and copy to tests (a second copy is skipped)
	a.m("POST", path, map[string]any{"ids": []string{ui}, "action": "color", "color": "#112233"}, 200)
	if res := a.m("POST", path, map[string]any{"ids": []string{ui}, "action": "copy"}, 200); res["changed"].(float64) != 1 {
		t.Fatal(res)
	}
	if res := a.m("POST", path, map[string]any{"ids": []string{ui}, "action": "copy"}, 200); res["changed"].(float64) != 0 {
		t.Fatal(res)
	}
	found := false
	for _, c := range items(a.m("GET", "/api/v1/projects/"+pid+"/categories?scope=test", nil, 200)) {
		found = found || (c["name"] == "UI" && c["color"] == "#112233")
	}
	if !found {
		t.Fatal("not copied")
	}
	// testers may edit test categories but not requirement ones
	e.member("tester@example.com", "tester")
	tc := e.login("tester@example.com")
	tc.m("POST", path, map[string]any{"ids": []string{ui}, "action": "delete"}, 403)
	tc.m("POST", path, map[string]any{"ids": []string{tst}, "action": "copy"}, 403)
	tc.m("POST", path, map[string]any{"ids": []string{tst}, "action": "color", "color": "#445566"}, 200)
	// Legacy/API was merged into Channel/API by the move, so its id is gone
	a.m("POST", path, map[string]any{"ids": []string{api}, "action": "delete"}, 404)
	if res := a.m("POST", path, map[string]any{"ids": []string{ui}, "action": "delete"}, 200); res["changed"].(float64) != 1 {
		t.Fatal(res)
	}
	if c := cats(r1); len(c) != 1 {
		t.Fatal(c)
	}
}
