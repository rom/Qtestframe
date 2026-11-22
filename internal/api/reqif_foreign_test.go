package api

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReqIFImportForeignTests imports a DOORS Next–shaped .reqifz with a
// test case whose description embeds an image: the test, its trace link,
// the attachment and the refines from the hierarchy are saved, and a
// second import changes nothing.
func TestReqIFImportForeignTests(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("DNG")
	doc, err := os.ReadFile(filepath.Join("..", "interop", "testdata", "dng-tests.reqif"))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range map[string][]byte{"brakes.reqif": doc, "files/track.png": pngBytes(t)} {
		w, _ := zw.Create(name)
		_, _ = w.Write(data)
	}
	_ = zw.Close()
	base := "/api/v1/projects/" + pid
	extra := map[string]string{"refines": "true"}
	pv := a.upload("POST", base+"/exchange/reqif/preview", "file", "brakes.reqifz", "", buf.Bytes(), extra, 200)
	if pv["tests"] == nil || len(pv["files"].([]any)) != 1 || pv["can_import_tests"] != true {
		t.Fatalf("preview: %v %v", pv["tests"], pv["files"])
	}
	res := a.upload("POST", base+"/exchange/reqif/apply", "file", "brakes.reqifz", "", buf.Bytes(), extra, 200)
	if res["created"].(float64) != 3 || res["links_added"].(float64) != 1 || res["relations_added"].(float64) != 1 || res["files_attached"].(float64) != 1 {
		t.Fatalf("apply: %v", res)
	}
	tests := items(a.m("GET", base+"/tests", nil, 200))
	if len(tests) != 1 || tests[0]["folder_path"] != "Acceptance tests" {
		t.Fatalf("tests: %v", tests)
	}
	tc := a.m("GET", "/api/v1/tests/"+tests[0]["id"].(string), nil, 200)
	if steps, _ := tc["steps"].([]any); len(steps) != 2 || !strings.Contains(tc["description"].(string), "](/api/v1/files/") {
		t.Errorf("test: %v", tc)
	}
	if files := items(a.m("GET", "/api/v1/tests/"+tests[0]["id"].(string)+"/attachments", nil, 200)); len(files) != 1 || files[0]["name"] != "track.png" {
		t.Errorf("attachments: %v", files)
	}
	pv = a.upload("POST", base+"/exchange/reqif/preview", "file", "brakes.reqifz", "", buf.Bytes(), extra, 200)
	if s := pv["tests"].(map[string]any)["summary"].(map[string]any); s["unchanged"].(float64) != 1 || pv["relations"].(map[string]any)["add"].(float64) != 0 {
		t.Errorf("second preview: %v %v", s, pv["relations"])
	}
	// without refines the hierarchy's relation is not removed
	pv = a.upload("POST", base+"/exchange/reqif/preview", "file", "brakes.reqifz", "", buf.Bytes(), nil, 200)
	if r := pv["relations"].(map[string]any); r["remove"].(float64) != 0 {
		t.Errorf("refines off: %v", r)
	}
	// a viewer-like role without test.write: test objects are left out
	a.m("POST", "/api/v1/roles", map[string]any{"name": "reqwriter", "permissions": []string{"project.read", "requirement.read", "requirement.write", "test.read"}}, 201)
	e.member("rw@example.com", "reqwriter")
	pv = e.login("rw@example.com").upload("POST", base+"/exchange/reqif/preview", "file", "brakes.reqifz", "", buf.Bytes(), nil, 200)
	if pv["tests"] != nil || pv["can_import_tests"] != false || len(pv["warnings"].([]any)) == 0 {
		t.Errorf("without test.write: %v %v", pv["tests"], pv["warnings"])
	}
}
