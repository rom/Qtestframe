package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rom/qtestframe/internal/stubgen"
)

// fakeStubs serves projects and /tests/stubs from stubgen with a test
// whose title can change.
type fakeStubs struct {
	mu    sync.Mutex
	title string
	reqs  []map[string]any
	path  string // override the returned path
}

func newFakeStubs(t *testing.T) (*fakeStubs, *httptest.Server) {
	f := &fakeStubs{title: "Pay with a card"}
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/projects":
			_, _ = w.Write([]byte(`{"items":[{"id":"p1","key":"SHOP","name":"Shop"}]}`))
		case "/api/v1/projects/p1/tests/stubs":
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			f.reqs = append(f.reqs, in)
			if in["query"] == "none" {
				_, _ = w.Write([]byte(`{"files":[]}`))
				return
			}
			file, err := stubgen.Generate(in["framework"].(string), stubgen.Test{Key: "SHOP-TC-1", Title: f.title, Steps: []stubgen.Step{{Action: "Pay"}}},
				stubgen.Options{Package: strOf(in["package"])})
			if err != nil {
				w.WriteHeader(400)
				return
			}
			if f.path != "" {
				file.Path = f.path
			}
			marked := 0
			if in["mark_script"] == true {
				marked = 1
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"files": []any{map[string]any{"path": file.Path, "content": file.Content, "key": file.Key, "test_id": "t1"}}, "marked": marked})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(hs.Close)
	return f, hs
}

func strOf(v any) string {
	s, _ := v.(string)
	return s
}

func TestStubsCommand(t *testing.T) {
	f, hs := newFakeStubs(t)
	withEnv(t, func(k string) string { return map[string]string{"QTF_URL": hs.URL, "QTF_TOKEN": "qtf_tok", "QTF_PROJECT": "SHOP"}[k] })
	dir := filepath.Join(t.TempDir(), "e2e-tests")
	file := filepath.Join(dir, "shop_tc_1_test.go")
	read := func() string {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	// dry run writes nothing
	r := run(t, "", "stubs", "--framework", "go", "--out", dir, "--dry-run")
	if r.code != 0 || !strings.Contains(r.stdout, "created") || !strings.Contains(r.stderr, "dry run") {
		t.Fatalf("%+v", r)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("dry run wrote")
	}
	r = run(t, "", "stubs", "--framework", "go", "--out", dir)
	if r.code != 0 || !strings.Contains(r.stdout, "created") || !strings.Contains(r.stderr, "1 created") {
		t.Fatalf("%+v", r)
	}
	// the package defaults to the directory's name; the query to automated tests
	if !strings.Contains(read(), "package e2e_tests") || f.reqs[1]["query"] != "execution_type:automated" || f.reqs[1]["format"] != "json" {
		t.Fatal(read(), f.reqs[1])
	}
	if r = run(t, "", "stubs", "--framework", "go", "--out", dir); r.code != 0 || !strings.Contains(r.stdout, "unchanged") {
		t.Fatalf("%+v", r)
	}
	// the test changed, the stub did not: regenerated
	f.title = "Pay with a saved card"
	if r = run(t, "", "stubs", "--framework", "go", "--out", dir); r.code != 0 || !strings.Contains(r.stdout, "regenerated") || !strings.Contains(read(), "PayWithASavedCard") {
		t.Fatalf("%+v", r)
	}
	// implemented: only the header block changes
	impl := strings.Replace(read(), "\tt.Skip(\"qtestframe stub: not implemented\")\n", "\tif 1+1 != 2 {\n\t\tt.Fatal()\n\t}\n", 1)
	if err := os.WriteFile(file, []byte(impl), 0o644); err != nil {
		t.Fatal(err)
	}
	f.title = "Pay by card"
	if r = run(t, "", "stubs", "--framework", "go", "--out", dir); r.code != 0 || !strings.Contains(r.stdout, "header block updated") ||
		!strings.Contains(read(), "// SHOP-TC-1: Pay by card") || !strings.Contains(read(), "t.Fatal()") || strings.Contains(read(), "PayByCard") {
		t.Fatalf("%+v\n%s", r, read())
	}
	// a file of your own is left alone
	if err := os.WriteFile(file, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r = run(t, "", "stubs", "--framework", "go", "--out", dir); r.code != 0 || !strings.Contains(r.stdout, "left alone") || read() != "package x\n" {
		t.Fatalf("%+v", r)
	}
	// mark the scripts: a second request with the relative directory
	wd, _ := os.Getwd()
	t.Chdir(filepath.Dir(dir))
	if r = run(t, "", "stubs", "--framework", "pytest", "--out", "e2e-tests", "--mark-script", "--query", "", "--ids", "SHOP-TC-1"); r.code != 0 || !strings.Contains(r.stderr, "marked the script of 1 test") {
		t.Fatalf("%+v", r)
	}
	last := f.reqs[len(f.reqs)-1]
	if last["mark_script"] != true || last["dir"] != "e2e-tests" || last["ids"].([]any)[0] != "SHOP-TC-1" {
		t.Fatal(last)
	}
	t.Chdir(wd)
	// nothing matches
	if r = run(t, "", "stubs", "--framework", "go", "--out", dir, "--query", "none"); r.code != 0 || !strings.Contains(r.stderr, "no tests of SHOP match") {
		t.Fatalf("%+v", r)
	}
	// errors
	for _, args := range [][]string{
		{"stubs"}, {"stubs", "--framework", "cobol"}, {"stubs", "--framework", "go", "--project", "NOPE"},
		{"stubs", "--framework", "go", "--out", dir, "--mark-script"}, {"stubs", "--bogus"},
	} {
		if r = run(t, "", args...); r.code == 0 {
			t.Errorf("%v: %+v", args, r)
		}
	}
	withEnv(t, func(k string) string { return map[string]string{"QTF_URL": hs.URL, "QTF_TOKEN": "qtf_tok"}[k] })
	if r = run(t, "", "stubs", "--framework", "go"); r.code == 0 || !strings.Contains(r.stderr, "--project KEY is required") {
		t.Fatalf("%+v", r)
	}
	withEnv(t, func(k string) string { return map[string]string{"QTF_URL": hs.URL, "QTF_TOKEN": "qtf_tok", "QTF_PROJECT": "SHOP"}[k] })
	f.path = "../evil.go"
	if r = run(t, "", "stubs", "--framework", "go", "--out", dir); r.code == 0 || !strings.Contains(r.stderr, "unexpected stub path") {
		t.Fatalf("%+v", r)
	}
}
