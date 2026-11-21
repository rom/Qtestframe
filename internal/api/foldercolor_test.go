package api

import "testing"

func TestFolderColor(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("COL")
	f := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "requirement", "name": "Payments", "color": " #DC2626 "}, 201)
	if f["color"] != "#dc2626" {
		t.Fatal(f)
	}
	a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "test", "name": "x", "color": "red"}, 400)
	id := f["id"].(string)
	// a rename leaves the colour; an explicit "" clears it; bad values are rejected
	if g := a.m("PATCH", "/api/v1/folders/"+id, map[string]any{"name": "Pay"}, 200); g["color"] != "#dc2626" {
		t.Fatal(g)
	}
	a.m("PATCH", "/api/v1/folders/"+id, map[string]any{"color": "#12345"}, 400)
	a.m("PATCH", "/api/v1/folders/"+id, map[string]any{"color": "#0d9488"}, 200)
	if fl := items(a.m("GET", "/api/v1/projects/"+pid+"/folders?kind=requirement", nil, 200)); len(fl) != 1 || fl[0]["color"] != "#0d9488" {
		t.Fatal(fl)
	}
	// the linked test created with a requirement goes to a mirror folder with the same colour
	rq := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "Refunds", "folder_id": id, "create_test": true}, 201)
	if rq["linked_test"] == nil {
		t.Fatal(rq)
	}
	tf := items(a.m("GET", "/api/v1/projects/"+pid+"/folders?kind=test", nil, 200))
	if len(tf) != 1 || tf[0]["name"] != "Pay" || tf[0]["color"] != "#0d9488" {
		t.Fatal(tf)
	}
	if g := a.m("PATCH", "/api/v1/folders/"+id, map[string]any{"color": ""}, 200); g["color"] != "" {
		t.Fatal(g)
	}
}
