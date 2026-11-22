package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rom/qtestframe/internal/ci"
)

// fakeCI is a Qtestframe server (projects, gate, select, result uploads)
// and a GitLab (statuses, notes) in one.
type fakeCI struct {
	mu       sync.Mutex
	gate     *ci.Report
	gateErr  int
	gateReqs []map[string]any
	selReqs  []map[string]any
	uploads  []string
	statuses []map[string]any
	notes    []string
	glAuth   []string
	glFail   int
}

func newFakeCI(t *testing.T) (*fakeCI, *httptest.Server) {
	f := &fakeCI{}
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		var in map[string]any
		_ = json.Unmarshal(body, &in)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/gl/api/v4/") {
			f.glAuth = append(f.glAuth, r.Header.Get("PRIVATE-TOKEN")+"|"+r.Header.Get("JOB-TOKEN"))
			if f.glFail != 0 {
				w.WriteHeader(f.glFail)
				_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
				return
			}
			switch {
			case strings.Contains(r.URL.Path, "/statuses/"):
				f.statuses = append(f.statuses, in)
				_, _ = w.Write([]byte(`{}`))
			case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/notes"):
				_, _ = w.Write([]byte(`[]`))
			case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/notes"):
				f.notes = append(f.notes, in["body"].(string))
				_, _ = w.Write([]byte(`{}`))
			default:
				w.WriteHeader(404)
			}
			return
		}
		if r.Header.Get("Authorization") != "Bearer qtf_tok" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":"authentication required"}`))
			return
		}
		switch {
		case r.URL.Path == "/api/v1/projects":
			_, _ = w.Write([]byte(`{"items":[{"id":"p1","key":"SHOP","name":"Shop"}]}`))
		case r.URL.Path == "/api/v1/projects/p1/gate":
			f.gateReqs = append(f.gateReqs, in)
			if f.gateErr != 0 {
				w.WriteHeader(f.gateErr)
				_, _ = w.Write([]byte(`{"error":"release \"x\" not found in project","code":"bad_request"}`))
				return
			}
			rep := *f.gate
			if in["summary_only"] == true {
				rep.SummaryOnly, rep.Conditions = true, nil
			}
			_ = json.NewEncoder(w).Encode(rep)
		case r.URL.Path == "/api/v1/projects/p1/ci/select":
			f.selReqs = append(f.selReqs, in)
			sel := ci.Selection{Project: ci.Ref{ID: "p1", Key: "SHOP"}, Tests: []ci.SelectedTest{{Key: "SHOP-TC-3", Runner: "pytest"}, {Key: "SHOP-TC-5", Runner: "go"}},
				Manual: []ci.SelectedTest{{Key: "SHOP-TC-9"}}, Unknown: []string{"SHOP-REQ-99"}}
			if in["create_run"] == true {
				sel.Run = &ci.Ref{ID: "run9", Name: in["run"].(string), URL: "http://q/p/p1/runs/run9"}
			}
			_ = json.NewEncoder(w).Encode(sel)
		case r.URL.Path == "/api/v1/projects/p1/runs":
			_, _ = w.Write([]byte(`{"items":[{"id":"r1","name":"Pipeline 12","status":"in_progress"}]}`))
		case r.URL.Path == "/api/v1/runs/r1/junit":
			f.uploads = append(f.uploads, r.URL.RawQuery)
			_, _ = w.Write([]byte(`{"updated":2,"unknown":[],"unmapped":[],"rejected":[],"build":{"id":"b1","external_id":"900","number":"12"}}`))
		default:
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error":"no such endpoint"}`))
		}
	}))
	t.Cleanup(hs.Close)
	return f, hs
}

func gitlabEnv(hs *httptest.Server, extra map[string]string) func(string) string {
	m := map[string]string{"QTF_URL": hs.URL, "QTF_TOKEN": "qtf_tok", "QTF_PROJECT": "SHOP", "GITLAB_CI": "true",
		"CI_PIPELINE_ID": "900", "CI_PIPELINE_IID": "12", "CI_COMMIT_SHA": "0123456789abcdef0123456789abcdef01234567", "CI_COMMIT_REF_NAME": "feature",
		"CI_MERGE_REQUEST_IID": "5", "CI_MERGE_REQUEST_DIFF_BASE_SHA": "base000", "CI_API_V4_URL": hs.URL + "/gl/api/v4", "CI_PROJECT_ID": "42",
		"CI_PIPELINE_URL": "https://gl/p/-/pipelines/900"}
	for k, v := range extra {
		m[k] = v
	}
	return func(k string) string { return m[k] }
}

func withEnv(t *testing.T, fn func(string) string) {
	old := getenv
	getenv = fn
	t.Cleanup(func() { getenv = old })
}

func failingReport() *ci.Report {
	return &ci.Report{Pass: false, Verdict: "fail", Project: ci.Ref{ID: "p1", Key: "SHOP"}, Run: &ci.Ref{Name: "Pipeline 12"}, URL: "http://q/p/p1/runs/r1",
		Conditions: []ci.Condition{{Type: "no_failed", Label: "no_failed", State: "fail", Actual: 1, Available: true, Items: []ci.Item{{Key: "SHOP-TC-3", Title: "Pay"}}}},
		Results:    &ci.Results{Total: 2, Passed: 1, Failed: 1}, Failures: []ci.Item{{Key: "SHOP-TC-3", Title: "Pay", Note: "failed"}}}
}

func TestGateCommand(t *testing.T) {
	f, hs := newFakeCI(t)
	withEnv(t, gitlabEnv(hs, map[string]string{"QTF_GITLAB_TOKEN": "gltok"}))
	f.gate = failingReport()
	dir := t.TempDir()
	out := filepath.Join(dir, "gate.xml")
	r := run(t, "", "gate", "--run", "Pipeline 12", "--release", "1.0", "--require", "no_failed", "--require", "coverage>=80,must", "--format", "junit", "--output", out, "--post-status", "--post-comment")
	if r.code != 1 || !strings.Contains(r.stdout, "Qtestframe quality gate: FAILED") || !strings.Contains(r.stdout, "SHOP-TC-3  Pay") || !strings.Contains(r.stderr, "quality gate failed: 1 of 1") {
		t.Fatalf("%+v", r)
	}
	b, err := os.ReadFile(out)
	if err != nil || !strings.Contains(string(b), `<testsuite name="qtestframe.gate"`) {
		t.Fatal(string(b), err)
	}
	req := f.gateReqs[0]
	if req["run"] != "Pipeline 12" || req["release"] != "1.0" || len(req["require"].([]any)) != 2 || req["merge_request"] != "5" || req["commit"] == nil {
		t.Fatal(req)
	}
	if len(f.statuses) != 1 || f.statuses[0]["name"] != "qtestframe/gate" || f.statuses[0]["state"] != "failed" || f.statuses[0]["pipeline_id"] != 900.0 || f.statuses[0]["ref"] != nil {
		t.Fatal(f.statuses)
	}
	if len(f.notes) != 1 || !strings.Contains(f.notes[0], ci.NoteMarker("SHOP")) || f.glAuth[0] != "gltok|" || !strings.Contains(r.stderr, "GitLab: note added to !5") {
		t.Fatal(f.notes, f.glAuth, r.stderr)
	}
	// pass: exit 0; markdown to stdout
	f.gate = &ci.Report{Pass: true, Verdict: "pass", Project: ci.Ref{Key: "SHOP"}}
	if r = run(t, "", "gate", "--format", "markdown"); r.code != 0 || !strings.Contains(r.stdout, "quality gate passed") || strings.Contains(r.stdout, "Qtestframe quality gate: PASSED") {
		t.Fatalf("%+v", r)
	}
	if r = run(t, "", "gate", "--format", "json", "--run-id", "r1"); r.code != 0 || !strings.Contains(r.stdout, `"pass": true`) || f.gateReqs[len(f.gateReqs)-1]["run"] != "r1" {
		t.Fatalf("%+v", r)
	}
	// errors: exit 2
	f.gateErr = 400
	if r = run(t, "", "gate", "--release", "x"); r.code != 2 || !strings.Contains(r.stderr, "not found") {
		t.Fatalf("%+v", r)
	}
	f.gateErr = 0
	for _, args := range [][]string{{"gate", "--format", "html"}, {"gate", "--bogus"}, {"gate", "--project", "NOPE"}, {"gate", "--token", "qtf_bad"}} {
		if r = run(t, "", args...); r.code != 2 {
			t.Errorf("%v: %+v", args, r)
		}
	}
	withEnv(t, func(k string) string { return map[string]string{"QTF_URL": hs.URL, "QTF_TOKEN": "qtf_tok"}[k] })
	if r = run(t, "", "gate"); r.code != 2 || !strings.Contains(r.stderr, "--project KEY is required") {
		t.Fatalf("%+v", r)
	}
	// posting outside CI only warns
	if r = run(t, "", "gate", "--project", "SHOP", "--post-status"); r.code != 0 || !strings.Contains(r.stderr, "not running in GitLab CI") {
		t.Fatalf("%+v", r)
	}
}

func TestReportPostsStatus(t *testing.T) {
	f, hs := newFakeCI(t)
	withEnv(t, gitlabEnv(hs, map[string]string{"CI_JOB_TOKEN": "jobtok", "QTF_POST_STATUS": "true", "QTF_POST_COMMENT": "1"}))
	f.gate = failingReport()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.xml"), []byte(`<testsuite/>`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := run(t, "", "report", "--run", "Pipeline 12", "--junit", filepath.Join(dir, "a.xml"))
	if r.code != 0 || !strings.Contains(r.stdout, "build 12") {
		t.Fatalf("%+v", r)
	}
	q := f.uploads[0]
	for _, want := range []string{"build_id=900", "build_number=12", "commit=0123456789abcdef0123456789abcdef01234567", "branch=feature", "merge_request=5", "pipeline_url="} {
		if !strings.Contains(q, want) {
			t.Errorf("upload query %q lacks %s", q, want)
		}
	}
	if f.gateReqs[0]["summary_only"] != true || f.gateReqs[0]["run"] != "r1" {
		t.Fatal(f.gateReqs)
	}
	if len(f.statuses) != 1 || f.statuses[0]["name"] != "qtestframe/tests" || f.glAuth[0] != "|jobtok" || len(f.notes) != 1 {
		t.Fatal(f.statuses, f.glAuth)
	}
	// the job token refused: a warning with a hint, the exit code stays 0
	f.glFail = 403
	r = run(t, "", "report", "--run", "Pipeline 12", "--junit", filepath.Join(dir, "a.xml"))
	if r.code != 0 || !strings.Contains(r.stderr, "does not accept the job token") {
		t.Fatalf("%+v", r)
	}
	// --ci none: no build
	f.glFail = 0
	r = run(t, "", "report", "--ci", "none", "--run", "Pipeline 12", "--junit", filepath.Join(dir, "a.xml"), "--post-status=false", "--post-comment=false")
	if r.code != 0 || f.uploads[len(f.uploads)-1] != "" {
		t.Fatalf("%+v %v", r, f.uploads)
	}
	if r = run(t, "", "report", "--ci", "x", "--run-id", "r1", "--junit", filepath.Join(dir, "a.xml")); r.code != 1 {
		t.Fatalf("%+v", r)
	}
}

func TestSelectCommand(t *testing.T) {
	f, hs := newFakeCI(t)
	withEnv(t, gitlabEnv(hs, nil))
	var gitCalls [][]string
	oldGit := git
	git = func(_ context.Context, args ...string) (string, error) {
		gitCalls = append(gitCalls, args)
		if args[0] == "log" {
			return "aaaa111\x1fFix checkout (REQ-4, SHOP-TC-7, OTHER-TC-1)\n\x1e\nbbbb222\x1fDocs\x1e", nil
		}
		return "src/cart/a.go\nREADME.md\n", nil
	}
	t.Cleanup(func() { git = oldGit })
	dir := t.TempDir()
	mapFile := filepath.Join(dir, "map.yml")
	if err := os.WriteFile(mapFile, []byte("- paths: [src/cart/**]\n  tests: [shop-tc-3]\n  tags: [cart]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := run(t, "", "select", "--map", mapFile, "--format", "pytest", "--tag", "always-run")
	if r.code != 0 || r.stdout != "SHOP_TC_3 or SHOP_TC_5\n" || !strings.Contains(r.stderr, "selected 2 automated test(s) of SHOP from 2 commit(s), 2 changed file(s), 3 key(s)") ||
		!strings.Contains(r.stderr, "1 manual test(s) not included") || !strings.Contains(r.stderr, "unknown keys: SHOP-REQ-99") {
		t.Fatalf("%+v", r)
	}
	if strings.Join(gitCalls[0], " ") != "log --format=%H%x1f%B%x1e base000..0123456789abcdef0123456789abcdef01234567" || gitCalls[1][0] != "diff" {
		t.Fatal(gitCalls)
	}
	req := f.selReqs[0]
	keys := req["keys"].([]any)
	if req["merge_request"] != "5" || len(req["commits"].([]any)) != 2 || len(keys) != 3 || keys[0] != "SHOP-REQ-4" || keys[2] != "SHOP-TC-3" ||
		len(req["tags"].([]any)) != 2 || req["all"] != nil {
		t.Fatal(req)
	}
	// child pipeline with a created run
	out := filepath.Join(dir, "child.yml")
	r = run(t, "", "select", "--commits", "aaaa111", "--files", "x.go", "--git=false", "--format", "gitlab-child-pipeline", "--output", out,
		"--create-run", "--run", "Pipeline 12", "--image", "pytest=python:3.13", "--job", "go=go test -run \"$QTF_FILTER\" ./...", "--gate-release", "1.0", "--gate-require", "no_failed")
	if r.code != 0 || !strings.Contains(r.stderr, `created run "Pipeline 12" (run9)`) {
		t.Fatalf("%+v", r)
	}
	y, _ := os.ReadFile(out)
	for _, want := range []string{"qtestframe-tests:pytest", "python:3.13", "qtestframe report --run-id run9", "--release 1.0 --require no_failed", `go test -run \"$QTF_FILTER\"`} {
		if !strings.Contains(string(y), want) {
			t.Errorf("child pipeline lacks %q:\n%s", want, y)
		}
	}
	if req := f.selReqs[1]; req["create_run"] != true || req["build"] != "12" || len(req["commits"].([]any)) != 1 {
		t.Fatal(req)
	}
	// nothing known: --fallback all selects everything
	withEnv(t, func(k string) string {
		return map[string]string{"QTF_URL": hs.URL, "QTF_TOKEN": "qtf_tok", "QTF_PROJECT": "SHOP"}[k]
	})
	if r = run(t, "", "select", "--fallback", "all", "--format", "keys", "--map", filepath.Join(dir, "none.yml")); r.code != 0 || f.selReqs[2]["all"] != true || r.stdout != "SHOP-TC-3\nSHOP-TC-5\n" {
		t.Fatalf("%+v %v", r, f.selReqs[2])
	}
	if r = run(t, "", "select"); r.code != 0 || !strings.Contains(r.stderr, "the change is unknown") {
		t.Fatalf("%+v", r)
	}
	for _, args := range [][]string{{"select", "--format", "nope"}, {"select", "--job", "novalue"}, {"select", "--project", ""}, {"select", "--project", "NOPE"}} {
		if r = run(t, "", args...); r.code != 1 {
			t.Errorf("%v: %+v", args, r)
		}
	}
	git = func(context.Context, ...string) (string, error) { return "", io.ErrUnexpectedEOF }
	if r = run(t, "", "select", "--since", "HEAD~3"); r.code != 1 {
		t.Fatalf("%+v", r)
	}
	if r = run(t, "", "help"); !strings.Contains(r.stdout, "qtestframe gate") || !strings.Contains(r.stdout, "qtestframe select") {
		t.Error(r.stdout)
	}
}

func TestCodeHostChoice(t *testing.T) {
	withEnv(t, func(k string) string { return map[string]string{"QTF_GITHUB_TOKEN": "gh"}[k] })
	if h, how, _ := codeHost(ci.Env{System: ci.SystemGitHub, Project: "o/r"}, "auto", nil); h == nil || h.Name() != "GitHub" || how == "" {
		t.Fatal(how)
	}
	for _, e := range []ci.Env{{System: ci.SystemGitHub}, {System: ci.SystemGitLab}, {System: ci.SystemGitLab, APIURL: "x", Project: "1"}, {}} {
		if h, _, why := codeHost(e, "", nil); h != nil || why == "" {
			t.Errorf("%+v", e)
		}
	}
	withEnv(t, func(string) string { return "" })
	if h, _, why := codeHost(ci.Env{System: ci.SystemGitHub, Project: "o/r"}, "github", nil); h != nil || !strings.Contains(why, "QTF_GITHUB_TOKEN") {
		t.Fatal(why)
	}
	if !zeroSHA("0000") || zeroSHA("01") {
		t.Error("zeroSHA")
	}
	if (&exitError{code: 1}).Error() != "" {
		t.Error("empty exit error")
	}
	if mapFlag(map[string]string{}).String() != "" {
		t.Error("mapFlag")
	}
}
