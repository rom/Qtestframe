package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeTestImport serves projects, test folders and the test import.
func fakeTestImport(t *testing.T) (*[]map[string]string, *httptest.Server) {
	var mu sync.Mutex
	var calls []map[string]string
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/projects":
			_, _ = w.Write([]byte(`{"items":[{"id":"p1","key":"SHOP","name":"Shop"}]}`))
		case "/api/v1/projects/p1/folders":
			_, _ = w.Write([]byte(`{"items":[{"id":"f1","name":"Legacy","parent_id":null}]}`))
		case "/api/v1/projects/p1/tests/import/preview", "/api/v1/projects/p1/tests/import/apply":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				w.WriteHeader(400)
				return
			}
			call := map[string]string{"path": r.URL.Path}
			for k, v := range r.MultipartForm.Value {
				call[k] = v[0]
			}
			if f, h, err := r.FormFile("file"); err == nil {
				call["file"] = h.Filename
				f.Close()
			}
			calls = append(calls, call)
			if strings.HasSuffix(r.URL.Path, "preview") {
				_, _ = w.Write([]byte(`{"source_name":"TestRail","format":"csv","total":2,"summary":{"new":1,"update":0,"unchanged":1,"conflict":0,"error":0},
					"rows":[{"action":"new","external_id":"C1","title":"Sign in","warnings":["no requirement matches SHOP-9"],"errors":[]},
					{"action":"unchanged","external_id":"C2","title":"Sign out","key":"SHOP-TC-2","warnings":[],"errors":["bad"]}],
					"history":{"runs":1,"executions":2,"known":1},"unresolved_links":["SHOP-9"],"warnings":["file warning"],"errors":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"created":1,"updated":0,"unchanged":1,"skipped":0,"folders_created":2,"links_added":0,"runs_created":1,"results_imported":1,
				"warnings":["file warning","late warning"],"skipped_rows":[{"location":"row 3","title":"X","reason":"excluded"}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(hs.Close)
	return &calls, hs
}

func TestImportTestsCommand(t *testing.T) {
	calls, hs := fakeTestImport(t)
	withEnv(t, func(k string) string {
		return map[string]string{"QTF_URL": hs.URL, "QTF_TOKEN": "<token>"}[k]
	})
	file := filepath.Join(t.TempDir(), "cases.csv")
	if err := os.WriteFile(file, []byte("ID,Title\nC1,Sign in\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		code := Run(append([]string{"import-tests"}, args...), Env{Stdout: &out, Stderr: &errb})
		return code, out.String(), errb.String()
	}
	code, out, errs := run("--from", "testrail", file, "--project", "SHOP", "--dry-run", "--folder", "Legacy")
	if code != 0 || !strings.Contains(out, "new       C1 Sign in") || !strings.Contains(out, "→ SHOP-TC-2") || !strings.Contains(errs, "dry run: nothing imported") ||
		!strings.Contains(errs, "no requirement matches SHOP-9") || !strings.Contains(errs, "1 past result(s) in 1 run(s)") {
		t.Fatalf("dry run: %d\n%s\n%s", code, out, errs)
	}
	if len(*calls) != 1 || (*calls)[0]["folder_id"] != "f1" || (*calls)[0]["source"] != "testrail" || (*calls)[0]["file"] != "cases.csv" {
		t.Fatalf("calls: %v", *calls)
	}
	code, _, errs = run("--from", "TestRail", "--project", "SHOP", "--no-history", "--overwrite", file)
	if code != 0 || !strings.Contains(errs, "imported into SHOP: 1 created") || !strings.Contains(errs, "late warning") || !strings.Contains(errs, "skipped row 3") {
		t.Fatalf("apply: %d %s", code, errs)
	}
	last := (*calls)[len(*calls)-1]
	if !strings.HasSuffix(last["path"], "apply") || last["history"] != "false" || last["overwrite_conflicts"] != "true" {
		t.Errorf("apply call: %v", last)
	}
	for _, args := range [][]string{
		{"--from", "nope", file, "--project", "SHOP"},
		{"--from", "xray", "--project", "SHOP"},
		{"--from", "xray", file},
		{"--from", "xray", file, "--project", "SHOP", "--folder", "Missing"},
		{"--from", "xray", filepath.Join(t.TempDir(), "missing.csv"), "--project", "SHOP"},
	} {
		if code, _, _ := run(args...); code == 0 {
			t.Errorf("%v: exit 0", args)
		}
	}
}
