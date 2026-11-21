package api

import (
	"io"
	"strings"
	"testing"
)

func TestBatchUpdateRequirements(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BAT")
	var ids []string
	for _, title := range []string{"One", "Two", "Three"} {
		ids = append(ids, a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": title, "create_test": false}, 201)["id"].(string))
	}
	path := "/api/v1/projects/" + pid + "/requirements/batch"

	// shape errors
	a.m("POST", path, map[string]any{"items": []any{}}, 400)
	a.m("POST", path, map[string]any{"items": []any{map[string]any{"id": ids[0], "set": map[string]any{}}}}, 400)
	a.m("POST", path, map[string]any{"items": []any{map[string]any{"set": map[string]any{"title": "x"}}}}, 400)
	a.m("POST", path, map[string]any{"items": []any{
		map[string]any{"id": ids[0], "set": map[string]any{"title": "x"}},
		map[string]any{"id": ids[0], "set": map[string]any{"title": "y"}},
	}}, 400)
	a.m("POST", path, map[string]any{"items": []any{map[string]any{"id": "missing", "set": map[string]any{"title": "x"}}}}, 404)

	// different values per item, in one go; versions are answered
	res := a.m("POST", path, map[string]any{"items": []any{
		map[string]any{"id": ids[0], "version": 1, "set": map[string]any{"title": "One!", "priority": "high", "tags": []string{"a", "b"}}},
		map[string]any{"id": ids[1], "version": 1, "set": map[string]any{"description": "Line 1\nLine 2", "obligation": "must"}},
	}}, 200)
	if res["updated"].(float64) != 2 {
		t.Fatal(res)
	}
	saved := items(res)
	if saved[0]["id"] != ids[0] || saved[0]["version"].(float64) != 2 || saved[0]["updated_at"] == "" {
		t.Fatal(saved)
	}
	r0 := a.m("GET", "/api/v1/requirements/"+ids[0], nil, 200)
	if r0["title"] != "One!" || r0["priority"] != "high" || len(r0["tags"].([]any)) != 2 {
		t.Fatal(r0)
	}
	if r1 := a.m("GET", "/api/v1/requirements/"+ids[1], nil, 200); r1["description"] != "Line 1\nLine 2" || r1["obligation"] != "must" || r1["title"] != "Two" {
		t.Fatal(r1)
	}
	// the history has the change, as for a single save
	if h := a.m("GET", "/api/v1/requirements/"+ids[0]+"/history", nil, 200); len(items(h)) < 2 {
		t.Fatal(h)
	}

	// an invalid value in the second item: nothing is saved, the item is named
	body := a.m("POST", path, map[string]any{"items": []any{
		map[string]any{"id": ids[2], "set": map[string]any{"title": "Three!"}},
		map[string]any{"id": ids[1], "set": map[string]any{"priority": "urgent"}},
	}}, 400)
	if body["index"].(float64) != 1 || body["id"] != ids[1] || !strings.Contains(body["error"].(string), "priority") {
		t.Fatal(body)
	}
	if r2 := a.m("GET", "/api/v1/requirements/"+ids[2], nil, 200); r2["title"] != "Three" || r2["version"].(float64) != 1 {
		t.Fatal("rolled back", r2)
	}

	// an outdated version: 409 with every conflict, nothing saved
	body = a.m("POST", path, map[string]any{"items": []any{
		map[string]any{"id": ids[2], "version": 1, "set": map[string]any{"title": "Three!"}},
		map[string]any{"id": ids[0], "version": 1, "set": map[string]any{"title": "Stale"}},
	}}, 409)
	conf := body["conflicts"].([]any)
	if body["code"] != "stale" || len(conf) != 1 {
		t.Fatal(body)
	}
	c := conf[0].(map[string]any)
	if c["id"] != ids[0] || c["version"].(float64) != 2 || c["current"].(map[string]any)["title"] != "One!" {
		t.Fatal(c)
	}
	if r2 := a.m("GET", "/api/v1/requirements/"+ids[2], nil, 200); r2["title"] != "Three" {
		t.Fatal(r2)
	}
	// without a version the change is saved (as undo does)
	a.m("POST", path, map[string]any{"items": []any{map[string]any{"id": ids[0], "set": map[string]any{"title": "One"}}}}, 200)

	// items of another project are not found
	other := a.project("BAO")
	oid := a.m("POST", "/api/v1/projects/"+other+"/requirements", map[string]any{"title": "Elsewhere", "create_test": false}, 201)["id"].(string)
	a.m("POST", path, map[string]any{"items": []any{map[string]any{"id": oid, "set": map[string]any{"title": "x"}}}}, 404)

	// permissions as for a PATCH: a reviewer may only decide the status, a viewer nothing
	e.member("rita.review@example.com", "reviewer")
	e.member("vera.view@example.com", "viewer")
	rev := e.login("rita.review@example.com")
	rev.m("POST", path, map[string]any{"items": []any{map[string]any{"id": ids[0], "set": map[string]any{"status": "approved"}}}}, 200)
	rev.m("POST", path, map[string]any{"items": []any{map[string]any{"id": ids[1], "set": map[string]any{"title": "No"}}}}, 403)
	e.login("vera.view@example.com").m("POST", path, map[string]any{"items": []any{map[string]any{"id": ids[1], "set": map[string]any{"title": "No"}}}}, 403)
	if r1 := a.m("GET", "/api/v1/requirements/"+ids[1], nil, 200); r1["title"] != "Two" {
		t.Fatal(r1)
	}
}

func TestBatchUpdateTests(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("BAT")
	var ids []string
	for _, title := range []string{"T1", "T2"} {
		ids = append(ids, a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": title, "steps": []any{map[string]any{"action": "Do", "expected": "Done"}}}, 201)["id"].(string))
	}
	path := "/api/v1/projects/" + pid + "/tests/batch"
	a.m("POST", path, map[string]any{"items": []any{
		map[string]any{"id": ids[0], "version": 1, "set": map[string]any{"preconditions": "Signed in", "execution_type": "automated"}},
		map[string]any{"id": ids[1], "version": 1, "set": map[string]any{"expected_result": "Works", "status": "review"}},
	}}, 200)
	t0 := a.m("GET", "/api/v1/tests/"+ids[0], nil, 200)
	if t0["preconditions"] != "Signed in" || t0["execution_type"] != "automated" || len(t0["steps"].([]any)) != 1 {
		t.Fatal("steps are kept", t0)
	}
	a.m("POST", path, map[string]any{"items": []any{map[string]any{"id": ids[1], "set": map[string]any{"execution_type": "robot"}}}}, 400)
	// a tester writes tests but may not mark them ready (a review decision)
	e.member("tom.test@example.com", "tester")
	tom := e.login("tom.test@example.com")
	tom.m("POST", path, map[string]any{"items": []any{map[string]any{"id": ids[1], "set": map[string]any{"title": "T2b"}}}}, 200)
	tom.m("POST", path, map[string]any{"items": []any{
		map[string]any{"id": ids[0], "set": map[string]any{"title": "T1b"}},
		map[string]any{"id": ids[1], "set": map[string]any{"status": "ready"}},
	}}, 403)
	if t0 := a.m("GET", "/api/v1/tests/"+ids[0], nil, 200); t0["title"] != "T1" {
		t.Fatal(t0)
	}
	// a tester cannot touch requirements
	rq := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "R", "create_test": false}, 201)["id"].(string)
	tom.m("POST", "/api/v1/projects/"+pid+"/requirements/batch", map[string]any{"items": []any{map[string]any{"id": rq, "set": map[string]any{"title": "x"}}}}, 403)
	// a requirement id on the tests batch is not found
	a.m("POST", path, map[string]any{"items": []any{map[string]any{"id": rq, "set": map[string]any{"title": "x"}}}}, 404)
}

func TestExportView(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("EXV")
	path := "/api/v1/projects/" + pid + "/export/view"
	view := map[string]any{"format": "csv", "name": "Requirements sheet", "title": "Requirements", "lang": "sv", "filter": "status = draft",
		"columns": []string{"Key", "Title", "Status"}, "rows": [][]string{{"EXV-1", "=cmd", "Utkast"}, {"EXV-2", "Two"}}}
	res := a.raw("POST", path, view)
	data, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(res.Header.Get("Content-Disposition"), "EXV-requirements-sheet-") || !strings.HasSuffix(strings.TrimSuffix(res.Header.Get("Content-Disposition"), `"`), ".csv") {
		t.Fatal(res.StatusCode, res.Header, string(data))
	}
	csv := string(data)
	if !strings.Contains(csv, "Key,Title,Status") || !strings.Contains(csv, "EXV-1,'=cmd,Utkast") || !strings.Contains(csv, "EXV-2,Two") {
		t.Fatal(csv)
	}
	for _, f := range []string{"xlsx", "md", "pdf", "docx"} {
		view["format"] = f
		res := a.raw("POST", path, view)
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 || len(data) == 0 {
			t.Fatal(f, res.StatusCode, string(data))
		}
		if f == "md" && !strings.Contains(string(data), "| EXV-1 |") {
			t.Fatal(string(data))
		}
	}
	view["format"] = "bmp"
	a.m("POST", path, view, 400)
	view["format"] = "csv"
	view["columns"] = []string{}
	a.m("POST", path, view, 400)
	view["columns"] = []string{"Key"}
	a.m("POST", path, view, 400) // rows wider than the columns
	if sectionName("  ") != "view" || sectionName("Tests – 2026") != "tests--2026" {
		t.Fatal(sectionName("Tests – 2026"))
	}
}
