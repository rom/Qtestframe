package api

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestReportBatch(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	f := newReportFixture(t, a)
	defs := "/api/v1/projects/" + f.pid + "/reports/definitions"
	d1 := a.m("POST", defs, map[string]any{"name": "Open tests", "config": map[string]any{"entity": "test", "columns": []string{"key", "title"}}}, 201)["id"].(string)
	d2 := a.m("POST", defs, map[string]any{"name": "All requirements", "config": map[string]any{"entity": "requirement", "columns": []string{"key"}}}, 201)["id"].(string)
	batch := "/api/v1/projects/" + f.pid + "/reports/batch"

	a.m("POST", batch, map[string]any{"format": "csv"}, 400)
	a.m("POST", batch, map[string]any{"definitions": []string{"missing"}, "format": "csv"}, 404)
	a.m("POST", batch, map[string]any{"templates": []map[string]any{{"template": "project_status"}}, "format": "gif"}, 400)
	// a run summary without its run fails the whole ZIP
	a.m("POST", batch, map[string]any{"templates": []map[string]any{{"template": "run_summary"}}, "format": "csv"}, 400)

	// ZIP with two templates and two definitions
	resp, data := a.download(batch, map[string]any{
		"templates":   []map[string]any{{"template": "project_status"}, {"template": "run_summary", "params": map[string]string{"run_id": f.runID}}},
		"definitions": []string{d1, d2},
		"format":      "csv",
	}, 200)
	if resp.Header.Get("Content-Type") != "application/zip" || !strings.Contains(resp.Header.Get("Content-Disposition"), "RPT-reports-") {
		t.Fatal(resp.Header)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, zf := range zr.File {
		names = append(names, zf.Name)
	}
	if len(names) != 4 || !strings.HasPrefix(names[0], "project_status-") || !strings.HasPrefix(names[2], "Open tests-") {
		t.Fatal(names)
	}

	// saved in the archive: the failing one is listed, the others are saved
	res := a.m("POST", batch, map[string]any{
		"templates":   []map[string]any{{"template": "defects"}, {"template": "run_summary"}},
		"definitions": []string{d1},
		"format":      "pdf",
		"archive":     true,
	}, 200)
	if len(res["files"].([]any)) != 2 || len(res["failed"].([]any)) != 1 || res["failed"].([]any)[0].(map[string]any)["name"] != "run_summary" {
		t.Fatal(res)
	}
	docs := items(a.m("GET", "/api/v1/projects/"+f.pid+"/documents?kind=report", nil, 200))
	if len(docs) < 2 {
		t.Fatal(docs)
	}
	// viewers may download a ZIP but not save to the archive or delete definitions
	e.member("viewer@example.com", "viewer")
	v := e.login("viewer@example.com")
	v.m("POST", batch, map[string]any{"definitions": []string{d1}, "format": "csv", "archive": true}, 403)
	v.m("DELETE", defs+"/bulk", map[string]any{"ids": []string{d1}}, 403)

	if res := a.m("DELETE", defs+"/bulk", map[string]any{"ids": []string{d1, d2}}, 200); res["deleted"].(float64) != 2 {
		t.Fatal(res)
	}
	if l := items(a.m("GET", defs, nil, 200)); len(l) != 0 {
		t.Fatal(l)
	}
	a.m("DELETE", defs+"/bulk", map[string]any{"ids": []string{"missing"}}, 404)
}
