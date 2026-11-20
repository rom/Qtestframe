package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestReportCommand(t *testing.T) {
	var mu sync.Mutex
	var uploads []string
	var unknown []string
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer qtf_tok" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":"authentication required"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/runs/r1/junit":
			mr, _ := r.MultipartReader()
			mu.Lock()
			for {
				p, err := mr.NextPart()
				if err != nil {
					break
				}
				_, _ = io.Copy(io.Discard, p)
				uploads = append(uploads, p.FileName())
			}
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"updated": 2, "unknown": unknown, "unmapped": []string{}, "rejected": []any{}})
		case r.URL.Path == "/api/v1/runs/r1/results":
			_, _ = w.Write([]byte(`{"updated":1,"unknown":[],"rejected":[]}`))
		default:
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error":"no such endpoint"}`))
		}
	}))
	defer hs.Close()
	dir := t.TempDir()
	for _, n := range []string{"a.xml", "b.xml", "r.json"} {
		body := `<testsuite/>`
		if strings.HasSuffix(n, ".json") {
			body = `{"results":[{"test_key":"X-TC-1","status":"passed"}]}`
		}
		if err := os.WriteFile(filepath.Join(dir, n), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("QTF_URL", hs.URL)
	t.Setenv("QTF_TOKEN", "qtf_tok")

	// files after --junit (as a shell expands a glob), flags after files
	r := run(t, "", "report", "--junit", filepath.Join(dir, "a.xml"), filepath.Join(dir, "b.xml"), filepath.Join(dir, "r.json"), "--run-id", "r1")
	if r.code != 0 || !strings.Contains(r.stdout, "run r1: 3 result(s) recorded") || len(uploads) != 2 || !strings.Contains(r.stderr, "uploaded 1 JSON result(s)") {
		t.Fatalf("%+v %v", r, uploads)
	}
	unknown = []string{"X-TC-9"}
	r = run(t, "", "report", "--run-id", "r1", "--junit", filepath.Join(dir, "*.xml"), "--fail-on-unknown")
	if r.code != 1 || !strings.Contains(r.stdout, "unknown test keys (1): X-TC-9") || !strings.Contains(r.stderr, "(--fail-on-unknown)") {
		t.Errorf("fail on unknown: %+v", r)
	}
	if r = run(t, "", "report", "--run-id", "r1", "--junit", filepath.Join(dir, "a.xml")); r.code != 0 {
		t.Errorf("unknown without the flag: %+v", r)
	}
	if r = run(t, "", "report", "--token", "qtf_bad", "--run-id", "r1", "--junit", filepath.Join(dir, "a.xml")); r.code != 1 || !strings.Contains(r.stderr, "401") {
		t.Errorf("token flag: %+v", r)
	}
	if r = run(t, "", "report", "--run-id", "nope", "--junit", filepath.Join(dir, "a.xml")); r.code != 1 || !strings.Contains(r.stderr, "no such endpoint") {
		t.Errorf("missing run: %+v", r)
	}
	if r = run(t, "", "report", "--run-id", "r1"); r.code != 1 || !strings.Contains(r.stderr, "nothing to report") {
		t.Errorf("no files: %+v", r)
	}
	if r = run(t, "", "report", "--bogus"); r.code != 1 {
		t.Errorf("bogus: %+v", r)
	}
	if r = run(t, "", "report", "-h"); r.code != 0 || !strings.Contains(r.stderr, "-create-missing") {
		t.Errorf("help: %+v", r)
	}
	if r = run(t, "", "help"); !strings.Contains(r.stdout, "qtestframe report") {
		t.Error("usage text misses report")
	}
	var l stringList
	_ = l.Set("a")
	_ = l.Set("b")
	if l.String() != "a,b" {
		t.Error(l.String())
	}
}
