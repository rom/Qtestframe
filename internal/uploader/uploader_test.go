package uploader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fake is a minimal stand-in for the Qtestframe API.
type fake struct {
	mu       sync.Mutex
	runs     []map[string]any
	created  map[string]string
	junit    []string // file names of the last JUnit upload
	junitQ   string
	jsonQ    string
	json     []json.RawMessage
	authFail bool
	importSt int
}

func (f *fake) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": []map[string]any{{"id": "p1", "key": "SHOP", "name": "Shop"}}})
	})
	mux.HandleFunc("GET /api/v1/projects/{pid}/runs", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		writeJSON(w, 200, map[string]any{"items": f.runs})
	})
	mux.HandleFunc("POST /api/v1/projects/{pid}/runs", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.created = req
		f.mu.Unlock()
		writeJSON(w, 201, map[string]any{"id": "new-run", "name": req["name"], "status": "planned"})
	})
	mux.HandleFunc("POST /api/v1/runs/{id}/junit", func(w http.ResponseWriter, r *http.Request) {
		if f.importSt != 0 {
			writeJSON(w, f.importSt, map[string]any{"error": "boom", "code": "bad_request"})
			return
		}
		mr, err := r.MultipartReader()
		if err != nil {
			t.Error(err)
			return
		}
		f.mu.Lock()
		f.junitQ = r.URL.RawQuery
		f.junit = nil
		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}
			b, _ := io.ReadAll(p)
			f.junit = append(f.junit, p.FileName()+":"+string(b))
		}
		f.mu.Unlock()
		writeJSON(w, 200, map[string]any{"updated": 2, "unknown": []string{"SHOP-TC-99"}, "unmapped": []string{"no key"},
			"rejected": []map[string]any{{"test_key": "SHOP-TC-3", "reason": "the test SHOP-TC-3 has status \"draft\""}},
			"created":  []map[string]any{{"key": "SHOP-TC-5", "title": "made"}}})
	})
	mux.HandleFunc("POST /api/v1/runs/{id}/results", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Results []json.RawMessage }
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.json = req.Results
		f.jsonQ = r.URL.RawQuery
		f.mu.Unlock()
		writeJSON(w, 200, map[string]any{"updated": len(req.Results), "unknown": []string{}, "rejected": []any{}})
	})
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer qtf_secret" || f.authFail {
			writeJSON(w, 401, map[string]any{"error": "authentication required", "code": "unauthorized"})
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(hs.Close)
	return hs
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func files(t *testing.T) (dir string) {
	dir = t.TempDir()
	for name, body := range map[string]string{
		"a.xml":    `<testsuite><testcase name="SHOP-TC-1"/></testsuite>`,
		"b.xml":    `<testsuite><testcase name="SHOP-TC-2"/></testsuite>`,
		"r.json":   `{"results":[{"test_key":"SHOP-TC-1","status":"passed"},{"test_key":"SHOP-TC-2","status":"failed"}]}`,
		"bad.json": `{nope`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestReportCreatesRunAndUploads(t *testing.T) {
	f := &fake{}
	hs := f.server(t)
	dir := files(t)
	var log bytes.Buffer
	sum, err := Report(context.Background(), Options{URL: hs.URL + "/api/v1/", Token: "qtf_secret", Project: "shop", Run: "Nightly",
		CreateRun: true, Build: "1.2.3", Environment: "staging", JUnit: []string{filepath.Join(dir, "*.xml"), filepath.Join(dir, "a.xml")},
		JSON: []string{filepath.Join(dir, "r.json")}, CreateMissing: true, Log: &log})
	if err != nil {
		t.Fatal(err)
	}
	if f.created["name"] != "Nightly" || f.created["build"] != "1.2.3" || f.created["environment"] != "staging" {
		t.Errorf("created: %v", f.created)
	}
	if len(f.junit) != 2 || !strings.HasPrefix(f.junit[0], "a.xml:<testsuite>") || f.junitQ != "create_missing=true" || len(f.json) != 2 {
		t.Errorf("uploads: %v %q %v", f.junit, f.junitQ, f.json)
	}
	if sum.RunID != "new-run" || sum.Updated != 4 || len(sum.Unknown) != 1 || len(sum.Unmapped) != 1 || len(sum.Rejected) != 1 || len(sum.Created) != 1 {
		t.Errorf("summary: %+v", sum)
	}
	if !strings.Contains(log.String(), `created run "Nightly"`) || !strings.Contains(log.String(), "uploaded 2 JUnit file(s)") {
		t.Error(log.String())
	}
	var out bytes.Buffer
	sum.Print(&out)
	for _, want := range []string{"run new-run: 4 result(s) recorded", "SHOP-TC-5  made", "unknown test keys (1): SHOP-TC-99",
		"without a test key (1): no key", `SHOP-TC-3: the test SHOP-TC-3 has status "draft"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("summary misses %q:\n%s", want, out.String())
		}
	}

	// --configuration is passed to both imports
	if _, err := Report(context.Background(), Options{URL: hs.URL, Token: "qtf_secret", RunID: "r9", Configuration: "Chrome / Windows",
		JUnit: []string{filepath.Join(dir, "a.xml")}, JSON: []string{filepath.Join(dir, "r.json")}}); err != nil {
		t.Fatal(err)
	}
	if f.junitQ != "configuration=Chrome+%2F+Windows" || f.jsonQ != "configuration=Chrome+%2F+Windows" {
		t.Errorf("configuration: %q %q", f.junitQ, f.jsonQ)
	}

	// --fail-on-unknown
	sum, err = Report(context.Background(), Options{URL: hs.URL, Token: "qtf_secret", RunID: "r9", JUnit: []string{filepath.Join(dir, "a.xml")}, FailOnUnknown: true})
	if !errors.Is(err, ErrUnknown) || sum == nil || sum.RunID != "r9" || f.junitQ != "" {
		t.Errorf("fail on unknown: %v %+v", err, sum)
	}
}

func TestReportFindsExistingRun(t *testing.T) {
	f := &fake{runs: []map[string]any{
		{"id": "old", "name": "Nightly", "status": "completed", "created_at": "2026-01-01T00:00:00Z"},
		{"id": "aborted", "name": "Nightly", "status": "aborted", "created_at": "2026-03-01T00:00:00Z"},
		{"id": "newer", "name": "Nightly", "status": "completed", "created_at": "2026-02-01T00:00:00Z"},
		{"id": "other", "name": "Other", "status": "planned", "created_at": "2026-04-01T00:00:00Z"},
	}}
	hs := f.server(t)
	dir := files(t)
	sum, err := Report(context.Background(), Options{URL: hs.URL, Token: "qtf_secret", Project: "SHOP", Run: "Nightly", CreateRun: true,
		JSON: []string{filepath.Join(dir, "r.json")}})
	if err != nil || sum.RunID != "newer" || f.created != nil {
		t.Fatalf("%v %+v %v", err, sum, f.created)
	}
	f.runs = f.runs[1:2]
	if sum, err := Report(context.Background(), Options{URL: hs.URL, Token: "qtf_secret", Project: "SHOP", Run: "Nightly", JSON: []string{filepath.Join(dir, "r.json")}}); err != nil || sum.RunID != "aborted" {
		t.Fatalf("%v %+v", err, sum)
	}
}

func TestReportErrors(t *testing.T) {
	f := &fake{}
	hs := f.server(t)
	dir := files(t)
	a := []string{filepath.Join(dir, "a.xml")}
	ctx := context.Background()
	cases := map[string]struct {
		o    Options
		want string
	}{
		"nothing":     {Options{URL: hs.URL, Token: "qtf_secret", RunID: "r"}, "nothing to report"},
		"no match":    {Options{URL: hs.URL, Token: "qtf_secret", RunID: "r", JUnit: []string{filepath.Join(dir, "*.none")}}, "no file matches"},
		"bad glob":    {Options{URL: hs.URL, Token: "qtf_secret", RunID: "r", JUnit: []string{"[x"}}, "invalid pattern"},
		"bad json":    {Options{URL: hs.URL, Token: "qtf_secret", RunID: "r", JSON: []string{filepath.Join(dir, "bad.json")}}, "invalid JSON"},
		"json glob":   {Options{URL: hs.URL, Token: "qtf_secret", RunID: "r", JSON: []string{"[x"}}, "invalid pattern"},
		"dir":         {Options{URL: hs.URL, Token: "qtf_secret", RunID: "r", JUnit: []string{dir}}, "is a directory"},
		"json dir":    {Options{URL: hs.URL, Token: "qtf_secret", RunID: "r", JSON: []string{dir}}, "is a directory"},
		"url":         {Options{URL: "ftp://x", Token: "qtf_secret", RunID: "r", JUnit: a}, "invalid server URL"},
		"url2":        {Options{URL: "://", Token: "qtf_secret", RunID: "r", JUnit: a}, "invalid server URL"},
		"token":       {Options{URL: hs.URL, RunID: "r", JUnit: a}, "API token is required"},
		"both":        {Options{URL: hs.URL, Token: "qtf_secret", RunID: "r", Run: "x", JUnit: a}, "not both"},
		"neither":     {Options{URL: hs.URL, Token: "qtf_secret", JUnit: a}, "--run NAME or --run-id"},
		"no project":  {Options{URL: hs.URL, Token: "qtf_secret", Run: "x", JUnit: a}, "--project KEY is required"},
		"unknown prj": {Options{URL: hs.URL, Token: "qtf_secret", Project: "NOPE", Run: "x", JUnit: a}, `project "NOPE" not found`},
		"no run":      {Options{URL: hs.URL, Token: "qtf_secret", Project: "SHOP", Run: "x", JUnit: a}, "pass --create-run"},
		"unauth":      {Options{URL: hs.URL, Token: "qtf_wrong", RunID: "r", JUnit: a}, "server answered 401: authentication required"},
		"unauth prj":  {Options{URL: hs.URL, Token: "qtf_wrong", Project: "SHOP", Run: "x", JUnit: a}, "401"},
		"unauth json": {Options{URL: hs.URL, Token: "qtf_wrong", RunID: "r", JSON: []string{filepath.Join(dir, "r.json")}}, "401"},
		"down":        {Options{URL: "http://127.0.0.1:1", Token: "qtf_secret", RunID: "r", JUnit: a}, "connect"},
	}
	for name, c := range cases {
		if _, err := Report(ctx, c.o); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v (want %q)", name, err, c.want)
		}
	}
	f.importSt = 400
	if _, err := Report(ctx, Options{URL: hs.URL, Token: "qtf_secret", RunID: "r", JUnit: a}); err == nil || !strings.Contains(err.Error(), "400: boom") {
		t.Error(err)
	}
	// run lookup / creation failures
	c, _ := New(hs.URL, "qtf_secret", nil)
	f.authFail = true
	if _, err := c.FindRun(ctx, "p1", "x"); err == nil {
		t.Error("find run")
	}
	if _, err := c.CreateRun(ctx, "p1", "x", "", ""); err == nil {
		t.Error("create run")
	}
	if _, err := Report(ctx, Options{URL: hs.URL, Token: "qtf_secret", Project: "SHOP", Run: "x", JUnit: a}); err == nil {
		t.Error("report")
	}
}

func TestClientEdgeCases(t *testing.T) {
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/projects":
			w.WriteHeader(502)
			_, _ = w.Write([]byte(strings.Repeat("<html>bad gateway</html>", 20)))
		case "/api/v1/projects/p/runs":
			_, _ = w.Write([]byte("not json"))
		case "/api/v1/projects/q/runs":
			if r.Method == "POST" {
				w.WriteHeader(500)
				return
			}
			_, _ = w.Write([]byte(`{"items":[]}`))
		}
	}))
	defer hs.Close()
	c, err := New(hs.URL+"/", "qtf_x", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, err = c.ProjectByKey(ctx, "X")
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != 502 || !strings.HasSuffix(ae.Message, "…") || len(ae.Message) > 210 {
		t.Errorf("%v", err)
	}
	if _, err := c.FindRun(ctx, "p", "x"); err == nil || !strings.Contains(err.Error(), "unexpected response") {
		t.Error(err)
	}
	if r, err := c.FindRun(ctx, "q", "x"); r != nil || err != nil {
		t.Error(r, err)
	}
	if _, err := c.CreateRun(ctx, "q", "x", "", ""); err == nil || err.Error() != "server answered 500" {
		t.Error(err)
	}
	if _, err := c.ImportJSON(ctx, "r", nil, "Chrome"); err == nil {
		t.Error("import json against an unknown path")
	}
	if err := c.sendJSON(ctx, "GET", "/x", func() {}, nil); err == nil {
		t.Error("marshal error")
	}
	if err := c.do(ctx, "BAD METHOD", "/x", nil, "", nil); err == nil {
		t.Error("bad method")
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := c.ImportJUnit(cctx, "r", []File{{Name: "x.xml", Data: []byte("<testsuite/>")}}, false, ""); err == nil {
		t.Error("cancelled")
	}
}
