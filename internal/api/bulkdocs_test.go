package api

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"
)

func TestBulkDocuments(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	e.member("tester@example.com", "tester")
	tc := e.login("tester@example.com")
	pid := a.project("BDOC")
	up := func(c *client, name, body string) string {
		return c.upload("POST", "/api/v1/projects/"+pid+"/documents", "file", name, "text/plain", []byte(body), map[string]string{"tags": "draft"}, 201)["id"].(string)
	}
	d1 := up(a, "plan.txt", "one")
	d2 := up(a, "plan.txt", "two") // same name: the ZIP renames it
	d3 := up(tc, "notes.txt", "three")
	path := "/api/v1/projects/" + pid + "/documents/bulk"
	a.m("POST", path, map[string]any{"ids": []string{d1}}, 400)
	a.m("POST", path, map[string]any{"ids": []string{"missing"}, "add_tags": []string{"x"}}, 404)

	// the tester may only change their own file
	res := tc.m("POST", path, map[string]any{"ids": []string{d1, d3}, "add_tags": []string{"final"}, "remove_tags": []string{"DRAFT"}}, 200)
	if len(res["updated"].([]any)) != 1 || len(res["skipped"].([]any)) != 1 || res["skipped"].([]any)[0].(map[string]any)["reason"] != "not_allowed" {
		t.Fatal(res)
	}
	if f := a.m("GET", "/api/v1/files/"+d3, nil, 200); len(f["tags"].([]any)) != 1 || f["tags"].([]any)[0] != "final" {
		t.Fatal(f)
	}
	res = a.m("POST", path, map[string]any{"ids": []string{d1, d2, d3}, "add_tags": []string{"Final", "v1"}, "description": "Sprint 12"}, 200)
	if len(res["updated"].([]any)) != 3 {
		t.Fatal(res)
	}
	f := a.m("GET", "/api/v1/files/"+d3, nil, 200)
	if len(f["tags"].([]any)) != 2 || f["description"] != "Sprint 12" {
		t.Fatal(f)
	}

	// ZIP download with unique names
	resp, data := a.download("/api/v1/projects/"+pid+"/documents/zip", map[string]any{"ids": []string{d1, d2, d3}}, 200)
	if resp.Header.Get("Content-Type") != "application/zip" {
		t.Fatal(resp.Header)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, zf := range zr.File {
		rc, _ := zf.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		got[zf.Name] = string(b)
	}
	if got["plan.txt"] != "one" || got["plan (2).txt"] != "two" || got["notes.txt"] != "three" {
		t.Fatal(got)
	}
	e.member("viewer@example.com", "viewer")
	e.login("viewer@example.com").download("/api/v1/projects/"+pid+"/documents/zip", map[string]any{"ids": []string{d1}}, 200)

	// delete: the tester's own file only
	res = tc.m("DELETE", path, map[string]any{"ids": []string{d1, d3}}, 200)
	if res["deleted"].(float64) != 1 || len(res["skipped"].([]any)) != 1 {
		t.Fatal(res)
	}
	if res := a.m("DELETE", path, map[string]any{"ids": []string{d1, d2}}, 200); res["deleted"].(float64) != 2 {
		t.Fatal(res)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/documents", nil, 200)); len(l) != 0 {
		t.Fatal(l)
	}
}

func TestZipNames(t *testing.T) {
	got := zipNames([]string{"a.txt", "A.txt", "a.txt", "../x", "", "dir/b.pdf"})
	want := []string{"a.txt", "A (2).txt", "a (3).txt", ".._x", "file", "dir_b.pdf"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatal(got)
		}
	}
}
