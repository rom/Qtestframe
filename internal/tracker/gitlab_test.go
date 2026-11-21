package tracker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// fakeGitLab is a GitLab REST API v4 project "qa/web/shop" (ID 42).
type fakeGitLab struct {
	mu     sync.Mutex
	issues map[string]map[string]any
	notes  []string
	last   map[string]any // body of the last write
	paths  []string       // escaped paths requested
	// limited answers this many requests with 429 first
	limited int
}

func (f *fakeGitLab) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := r.URL.EscapedPath()
	f.paths = append(f.paths, r.Method+" "+p)
	if r.Header.Get("PRIVATE-TOKEN") != "glpat-test" {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
		return
	}
	if f.limited > 0 {
		f.limited--
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`Retry later`))
		return
	}
	var body map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	if body != nil {
		f.last = body
	}
	write := func(code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	proj := "/api/v4/projects/qa%2Fweb%2Fshop"
	if !strings.HasPrefix(p, proj) {
		if p == "/api/v4/users" {
			if r.URL.Query().Get("username") == "alice" {
				write(200, []map[string]any{{"id": 7, "username": "alice"}})
				return
			}
			write(200, []any{})
			return
		}
		write(404, map[string]any{"message": "404 Project Not Found"})
		return
	}
	rest := strings.TrimPrefix(p, proj)
	switch {
	case rest == "" && r.Method == "GET":
		write(200, map[string]any{"id": 42, "path_with_namespace": "qa/web/shop"})
	case rest == "/milestones":
		q := r.URL.Query()
		if q.Get("include_ancestors") != "true" {
			write(400, map[string]any{"error": "include_ancestors missing"})
			return
		}
		if q.Get("title") == "R1" || q.Get("iids[]") == "3" {
			write(200, []map[string]any{{"id": 900, "iid": 3, "title": "R1"}})
			return
		}
		write(200, []any{})
	case rest == "/issues" && r.Method == "POST":
		if body["title"] == "" {
			write(422, map[string]any{"message": map[string]any{"title": []string{"can't be blank"}}})
			return
		}
		n := len(f.issues) + 1
		iid := strconv.Itoa(n)
		is := map[string]any{"iid": n, "web_url": "https://gitlab.example/qa/web/shop/-/issues/" + iid, "title": body["title"], "description": body["description"],
			"state": "opened", "references": map[string]any{"full": "qa/web/shop#" + iid}}
		f.issues[iid] = is
		write(201, is)
	case strings.HasPrefix(rest, "/issues/"):
		parts := strings.Split(strings.TrimPrefix(rest, "/issues/"), "/")
		is, ok := f.issues[parts[0]]
		if !ok {
			write(404, map[string]any{"message": "404 Not found"})
			return
		}
		switch {
		case len(parts) == 2 && parts[1] == "notes" && r.Method == "POST":
			f.notes = append(f.notes, body["body"].(string))
			write(201, map[string]any{"id": 1})
		case len(parts) == 1 && r.Method == "PUT":
			is["title"], is["description"] = body["title"], body["description"]
			write(200, is)
		case len(parts) == 1 && r.Method == "GET":
			write(200, is)
		default:
			write(405, map[string]any{"message": "405 Method Not Allowed"})
		}
	default:
		write(404, map[string]any{"message": "404 Not Found"})
	}
}

func newFakeGitLab(t *testing.T) (*fakeGitLab, *httptest.Server) {
	f := &fakeGitLab{issues: map[string]map[string]any{}}
	srv := httptest.NewTLSServer(f)
	t.Cleanup(srv.Close)
	return f, srv
}

func noSleep(t *testing.T) *[]time.Duration {
	var waits []time.Duration
	old := sleep
	sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	t.Cleanup(func() { sleep = old })
	return &waits
}

func TestGitLab(t *testing.T) {
	f, srv := newFakeGitLab(t)
	cfg := &model.DefectTracker{Type: "gitlab", URL: srv.URL, Project: "qa/web/shop", Token: "glpat-test", Labels: "qtestframe", Milestone: "R1", Assignee: "alice"}
	c, err := New(cfg, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if c.Markup() != MarkupMarkdown {
		t.Fatal(c.Markup())
	}
	ctx := context.Background()
	if err := c.Check(ctx); err != nil {
		t.Fatal(err)
	}
	is, err := c.Create(ctx, NewIssue{Title: "[SHOP-REQ-1] Login", Description: "text", Labels: []string{"type::functional", "QTESTFRAME"}, Severity: "major"})
	if err != nil || is.Key != "qa/web/shop#1" || is.Mapped != "open" || is.Closed || is.Status != "opened" || !strings.HasSuffix(is.URL, "/-/issues/1") {
		t.Fatal(is, err)
	}
	if f.last["labels"] != "qtestframe,type::functional" || f.last["milestone_id"] != float64(900) || f.last["assignee_ids"].([]any)[0] != float64(7) || !strings.Contains(f.last["description"].(string), "Severity: major") {
		t.Fatal(f.last)
	}
	// the path is URL-encoded as GitLab wants it
	if !strings.Contains(strings.Join(f.paths, "\n"), "POST /api/v4/projects/qa%2Fweb%2Fshop/issues") {
		t.Fatal(f.paths)
	}
	up, err := c.Update(ctx, "#1", NewIssue{Title: "[SHOP-REQ-1] Login v2", Description: "new", Labels: []string{"x"}})
	if err != nil || up.Title != "[SHOP-REQ-1] Login v2" || up.Description != "new" || f.last["add_labels"] != "x" {
		t.Fatal(up, err, f.last)
	}
	if err := c.Comment(ctx, "qa/web/shop#1", "all passed"); err != nil || len(f.notes) != 1 {
		t.Fatal(err, f.notes)
	}
	f.mu.Lock()
	f.issues["1"]["state"] = "closed"
	f.mu.Unlock()
	if got, err := c.Get(ctx, "1"); err != nil || got.Mapped != "closed" || !got.Closed {
		t.Fatal(got, err)
	}
	for _, k := range []string{"", "abc", "#0", "qa/web/shop#x"} {
		if _, err := c.Get(ctx, k); !errors.Is(err, ErrBadKey) {
			t.Errorf("%q: %v", k, err)
		}
		if _, err := c.Update(ctx, k, NewIssue{}); !errors.Is(err, ErrBadKey) {
			t.Errorf("%q: %v", k, err)
		}
		if err := c.Comment(ctx, k, "x"); !errors.Is(err, ErrBadKey) {
			t.Errorf("%q: %v", k, err)
		}
	}
	// clear errors
	_, err = c.Get(ctx, "#99")
	var te *Error
	if !errors.As(err, &te) || te.Status != 404 || !strings.Contains(err.Error(), "not found: check the project") || !strings.Contains(err.Error(), "404 Not found") {
		t.Fatal(err)
	}
	_, err = c.Create(ctx, NewIssue{Title: ""})
	if !errors.As(err, &te) || te.Status != 422 || !strings.Contains(err.Error(), "title can't be blank") {
		t.Fatal(err)
	}
	bad, _ := New(&model.DefectTracker{Type: "gitlab", URL: srv.URL, Project: "qa/web/shop", Token: "wrong"}, srv.Client())
	if err := bad.Check(ctx); err == nil || !strings.Contains(err.Error(), "401 (the token was rejected") || strings.Contains(err.Error(), "wrong") {
		t.Fatal(err)
	}
	other, _ := New(&model.DefectTracker{Type: "gitlab", URL: srv.URL + "/api/v4/", Project: "42", Token: "glpat-test"}, srv.Client())
	if err := other.Check(ctx); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatal(err)
	}
	// milestone by number (%3), unknown milestone and assignee
	cfg.Milestone, cfg.Assignee = "%3", "7"
	if _, err := c.Create(ctx, NewIssue{Title: "t"}); err != nil || f.last["milestone_id"] != float64(900) || f.last["assignee_ids"].([]any)[0] != float64(7) {
		t.Fatal(err, f.last)
	}
	cfg.Milestone = "R9"
	if _, err := c.Create(ctx, NewIssue{Title: "t"}); !errors.Is(err, ErrConfig) {
		t.Fatal(err)
	}
	cfg.Milestone, cfg.Assignee = "", "@nobody"
	if _, err := c.Create(ctx, NewIssue{Title: "t"}); !errors.Is(err, ErrConfig) {
		t.Fatal(err)
	}
	// a project ID as configured: the key is built when GitLab sends no reference
	g := newGitLab(&model.DefectTracker{Project: "42"}, nil)
	if is := g.issue(&glIssue{IID: 5, State: "opened"}); is.Key != "42#5" || g.base != GitLabURL+"/api/v4/projects/42" {
		t.Fatal(is, g.base)
	}
}

func TestRateLimit(t *testing.T) {
	waits := noSleep(t)
	f, srv := newFakeGitLab(t)
	c, _ := New(&model.DefectTracker{Type: "gitlab", URL: srv.URL, Project: "qa/web/shop", Token: "glpat-test"}, srv.Client())
	ctx := context.Background()
	// limited twice, then served: waited the Retry-After both times
	f.limited = 2
	if _, err := c.Create(ctx, NewIssue{Title: "t"}); err != nil || len(*waits) != 2 || (*waits)[0] != time.Second {
		t.Fatal(err, *waits)
	}
	// limited more often than retried: the error says how long to wait
	f.limited = 5
	err := c.Check(ctx)
	var te *Error
	if !errors.As(err, &te) || te.Status != 429 || te.RetryAfter != time.Second || !strings.Contains(err.Error(), "rate limited") || !strings.Contains(err.Error(), "try again in 1s") {
		t.Fatal(err)
	}
	f.limited = 0

	// a long wait is not waited for
	for _, tc := range []struct {
		h     http.Header
		code  int
		wait  time.Duration
		limit bool
	}{
		{http.Header{"Retry-After": {"120"}}, 429, 120 * time.Second, true},
		{http.Header{"Retry-After": {now().Add(30 * time.Second).UTC().Format(http.TimeFormat)}}, 429, 30 * time.Second, true},
		{http.Header{"Ratelimit-Remaining": {"0"}, "Ratelimit-Reset": {strconv.Itoa(int(now().Add(20 * time.Second).Unix()))}}, 403, 20 * time.Second, true},
		{http.Header{"X-Ratelimit-Remaining": {"0"}}, 403, time.Second, true},
		{http.Header{}, 429, time.Second, true},
		{http.Header{}, 403, 0, false},
		{http.Header{"Retry-After": {"5"}}, 500, 0, false},
	} {
		d, limited := retryAfter(&http.Response{StatusCode: tc.code, Header: tc.h})
		if limited != tc.limit || (d-tc.wait).Abs() > 2*time.Second {
			t.Errorf("%v %d: %v %v", tc.h, tc.code, d, limited)
		}
	}
	long := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(429)
	}))
	defer long.Close()
	*waits = nil
	gh, _ := New(&model.DefectTracker{Type: "github", URL: long.URL, Repo: "a/b"}, long.Client())
	if err := gh.Check(ctx); !errors.As(err, &te) || te.RetryAfter != time.Hour || len(*waits) != 0 {
		t.Fatal(err, *waits)
	}
	// a cancelled wait ends the request
	sleep = func(ctx context.Context, d time.Duration) error { return context.Canceled }
	f.limited = 1
	if err := c.Check(ctx); err == nil || !strings.Contains(err.Error(), "cannot reach") {
		t.Fatal(err)
	}
}

func TestErrorMessages(t *testing.T) {
	for body, want := range map[string]string{
		`{"message":"403 Forbidden"}`:                                    "403 Forbidden",
		`{"message":{"labels":["is invalid"],"title":["x"]}}`:            "labels is invalid; title x",
		`{"error":"insufficient_scope","error_description":"needs api"}`: "insufficient_scope; needs api",
		`{"errorMessages":["a","b"],"errors":{}}`:                        "a, b",
		`<html>oops</html>`:                                              "<html>oops</html>",
		``:                                                               "",
		`{"message":null}`:                                               `{"message":null}`,
		`{"message":12}`:                                                 "12",
	} {
		if got := errorMessage(body); got != want {
			t.Errorf("%s: %q", body, got)
		}
	}
	e := &Error{Status: 403, Body: `{"message":"403 Forbidden"}`}
	if s := e.Error(); s != "the tracker answered 403 (permission denied: the token lacks a scope or role the request needs): 403 Forbidden" {
		t.Fatal(s)
	}
	if s := (&Error{Status: 502}).Error(); s != "the tracker answered 502" {
		t.Fatal(s)
	}
}

func TestDoc(t *testing.T) {
	md := NewDoc(MarkupMarkdown)
	md.Note("from " + md.Link("A-REQ-1", "https://q.example/go/A-REQ-1"))
	md.Heading("Requirement")
	md.Text("  The [system] shall | work.  ")
	md.Text("")
	md.Fields([][2]string{{"Priority", "high"}, {"Owner", ""}})
	md.Fields(nil)
	md.Table([]string{"Test", "Title"}, [][]string{{md.Link("A-TC-1", ""), md.Cell("a | b\nc")}, {md.Bold("x"), md.Cell("")}})
	md.Table([]string{"x"}, nil)
	want := "> from [A-REQ-1](https://q.example/go/A-REQ-1)\n\n### Requirement\n\nThe [system] shall | work.\n\n- **Priority:** high\n\n| Test | Title |\n| --- | --- |\n| A-TC-1 | a \\| b<br>c |\n| **x** |   |"
	if got := md.String(); got != want {
		t.Fatalf("%q", got)
	}
	if got := md.Link("a [b]", "u"); got != `[a \[b\]](u)` {
		t.Fatal(got)
	}
	j := NewDoc(MarkupJira)
	j.Note("n")
	j.Heading("H  1")
	j.Fields([][2]string{{"Priority", "high"}})
	j.Table([]string{"A", "B"}, [][]string{{j.Link("x|y", "u"), j.Cell("1\n2")}})
	if got := j.String(); got != "{quote}n{quote}\n\nh3. H 1\n\n* *Priority:* high\n\n||A||B||\n|[x/y|u]|1\\\\ 2|" {
		t.Fatalf("%q", got)
	}
	if j.Bold("b") != "*b*" {
		t.Fatal(j.Bold("b"))
	}
	// managed parts
	m := Managed(MarkupMarkdown, "new")
	if m != BeginMarker+"\nnew\n"+EndMarker || Managed(MarkupJira, "new") != "new" {
		t.Fatal(m)
	}
	cur := "intro\n" + BeginMarker + "\nold\n" + EndMarker + "\nnotes"
	if got := MergeManaged(MarkupMarkdown, cur, m); got != "intro\n"+m+"\nnotes" {
		t.Fatal(got)
	}
	if got := MergeManaged(MarkupMarkdown, "  hand written ", m); got != m+"\n\nhand written" {
		t.Fatal(got)
	}
	if MergeManaged(MarkupMarkdown, "", m) != m || MergeManaged(MarkupJira, "old", "new") != "new" {
		t.Fatal("merge")
	}
}

func TestIssueWritesGitHubJira(t *testing.T) {
	var mu sync.Mutex
	var reqs []string
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		reqs = append(reqs, r.Method+" "+r.URL.Path)
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		bodies = append(bodies, b)
		switch {
		case r.URL.Path == "/repos/acme/shop/milestones":
			_, _ = w.Write([]byte(`[{"number":4,"title":"Sprint 9"}]`))
		case r.URL.Path == "/repos/acme/shop/issues" && r.Method == "POST":
			_, _ = w.Write([]byte(`{"number":3,"html_url":"h","title":"t","body":"b","state":"open"}`))
		case r.URL.Path == "/repos/acme/shop/issues/3":
			_, _ = w.Write([]byte(`{"number":3,"html_url":"h","title":"t2","body":"b2","state":"open"}`))
		case r.URL.Path == "/repos/acme/shop/issues/3/labels", r.URL.Path == "/repos/acme/shop/issues/3/comments":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/rest/api/2/issue" && r.Method == "POST":
			_, _ = w.Write([]byte(`{"key":"SHOP-9"}`))
		case r.URL.Path == "/rest/api/2/issue/SHOP-9" && r.Method == "PUT":
			w.WriteHeader(204)
		case r.URL.Path == "/rest/api/2/issue/SHOP-9/comment":
			w.WriteHeader(201)
		case r.URL.Path == "/rest/api/2/issue/SHOP-9":
			_, _ = w.Write([]byte(`{"key":"SHOP-9","fields":{"summary":"s","description":"d","status":{"name":"Done","statusCategory":{"key":"done"}}}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	gh, _ := New(&model.DefectTracker{Type: "github", URL: srv.URL, Repo: "acme/shop", Milestone: "sprint 9", Assignee: "bob"}, srv.Client())
	is, err := gh.Create(ctx, NewIssue{Title: "t", Labels: []string{"req"}})
	if err != nil || is.Description != "b" || bodies[1]["milestone"] != float64(4) || bodies[1]["assignees"].([]any)[0] != "bob" {
		t.Fatal(is, err, bodies)
	}
	if up, err := gh.Update(ctx, "#3", NewIssue{Title: "t2", Description: "b2", Labels: []string{"req"}}); err != nil || up.Title != "t2" {
		t.Fatal(up, err)
	}
	if err := gh.Comment(ctx, "acme/shop#3", "ok"); err != nil {
		t.Fatal(err)
	}
	if _, err := gh.Update(ctx, "x", NewIssue{}); !errors.Is(err, ErrBadKey) {
		t.Fatal(err)
	}
	if err := gh.Comment(ctx, "x", ""); !errors.Is(err, ErrBadKey) {
		t.Fatal(err)
	}
	gh2, _ := New(&model.DefectTracker{Type: "github", URL: srv.URL, Repo: "acme/shop", Milestone: "7"}, srv.Client())
	if _, err := gh2.Create(ctx, NewIssue{Title: "t"}); err != nil || bodies[len(bodies)-1]["milestone"] != float64(7) {
		t.Fatal(err)
	}
	gh3, _ := New(&model.DefectTracker{Type: "github", URL: srv.URL, Repo: "acme/shop", Milestone: "nope"}, srv.Client())
	if _, err := gh3.Create(ctx, NewIssue{Title: "t"}); !errors.Is(err, ErrConfig) {
		t.Fatal(err)
	}
	gh4, _ := New(&model.DefectTracker{Type: "github", URL: srv.URL, Repo: "other/repo", Milestone: "x"}, srv.Client())
	if _, err := gh4.Create(ctx, NewIssue{Title: "t"}); err == nil {
		t.Fatal("milestones of an unknown repo")
	}

	j, _ := New(&model.DefectTracker{Type: "jira", URL: srv.URL, ProjectKey: "SHOP", IssueType: "Bug"}, srv.Client())
	if j.Markup() != MarkupJira {
		t.Fatal(j.Markup())
	}
	n := len(bodies)
	is, err = j.Create(ctx, NewIssue{Title: "s", IssueType: "Task", Labels: []string{"type::functional", "two words"}})
	f := bodies[n]["fields"].(map[string]any)
	if err != nil || !is.Closed || is.Description != "d" || f["issuetype"].(map[string]any)["name"] != "Task" || f["labels"].([]any)[1] != "two_words" {
		t.Fatal(is, err, f)
	}
	if _, err := j.Update(ctx, "shop-9", NewIssue{Title: "s", Description: "d", Labels: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	if err := j.Comment(ctx, "SHOP-9", "ok"); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Update(ctx, "#1", NewIssue{}); !errors.Is(err, ErrBadKey) {
		t.Fatal(err)
	}
	if err := j.Comment(ctx, "", ""); !errors.Is(err, ErrBadKey) {
		t.Fatal(err)
	}
	if got := mergeLabels([]string{"a", " A ", ""}, []string{"b"}); strings.Join(got, ",") != "a,b" {
		t.Fatal(got)
	}
}
