package api

import (
	"testing"
)

func TestBulkTags(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BTG")
	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "R1", "tags": []string{"ui", "legacy"}}, 201)["id"].(string)
	r2 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "R2", "tags": []string{"UI-old"}}, 201)["id"].(string)
	t1 := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "T1", "tags": []string{"legacy", "smoke"}}, 201)["id"].(string)
	doc := a.upload("POST", "/api/v1/projects/"+pid+"/documents", "file", "spec.txt", "text/plain", []byte("x"), map[string]string{"tags": "legacy"}, 201)["id"].(string)
	tags := func(path string) []string { return strs(a.m("GET", path, nil, 200)["tags"]) }

	// items: add and remove tags on a selection (case-insensitive removal, no duplicates)
	bulk := "/api/v1/projects/" + pid + "/requirements/bulk"
	a.m("POST", bulk, map[string]any{"ids": []string{r1, r2}, "set": map[string]any{"add_tags": []string{"release-2", "UI"}, "remove_tags": []string{"LEGACY"}}}, 200)
	if got := tags("/api/v1/requirements/" + r1); len(got) != 2 || got[0] != "ui" || got[1] != "release-2" {
		t.Fatal(got)
	}
	if got := tags("/api/v1/requirements/" + r2); len(got) != 3 {
		t.Fatal(got)
	}
	if hist := items(a.m("GET", "/api/v1/requirements/"+r1+"/history", nil, 200)); hist[0]["action"] != "bulk_update" {
		t.Fatal(hist[0])
	}
	// a reviewer cannot change tags through the bulk endpoint
	e.member("rita@example.com", "reviewer")
	e.login("rita@example.com").m("POST", bulk, map[string]any{"ids": []string{r1}, "set": map[string]any{"add_tags": []string{"x"}}}, 403)

	// project: merge UI-old and UI into ui, rename smoke, delete legacy everywhere
	path := "/api/v1/projects/" + pid + "/tags/bulk"
	a.m("POST", path, map[string]any{"tags": []string{"ui"}, "action": "merge", "to": "x"}, 400)
	a.m("POST", path, map[string]any{"tags": []string{"a", "b"}, "action": "rename", "to": "x"}, 400)
	a.m("POST", path, map[string]any{"tags": []string{"a"}, "action": "nope"}, 400)
	res := a.m("POST", path, map[string]any{"tags": []string{"UI-old", "UI"}, "action": "merge", "to": "ui"}, 200)
	if res["requirements"].(float64) != 1 {
		t.Fatal(res)
	}
	if got := tags("/api/v1/requirements/" + r2); len(got) != 2 || got[0] != "ui" {
		t.Fatal(got)
	}
	a.m("POST", path, map[string]any{"tags": []string{"smoke"}, "action": "rename", "to": "sanity"}, 200)
	if got := tags("/api/v1/tests/" + t1); got[1] != "sanity" {
		t.Fatal(got)
	}
	res = a.m("POST", path, map[string]any{"tags": []string{"legacy"}, "action": "delete"}, 200)
	if res["tests"].(float64) != 1 || res["documents"].(float64) != 1 {
		t.Fatal(res)
	}
	if got := strs(a.m("GET", "/api/v1/files/"+doc, nil, 200)["tags"]); len(got) != 0 {
		t.Fatal(got)
	}
	for _, tc := range items(a.m("GET", "/api/v1/projects/"+pid+"/tags", nil, 200)) {
		if tc["tag"] == "legacy" || tc["tag"] == "smoke" || tc["tag"] == "UI-old" {
			t.Fatal("tag still there", tc)
		}
	}
	// testers have test.write but not requirement.write
	e.member("tester@example.com", "tester")
	e.login("tester@example.com").m("POST", path, map[string]any{"tags": []string{"ui"}, "action": "delete"}, 403)
}
