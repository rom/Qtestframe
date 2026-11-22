package tracker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type hostCall struct {
	method, path, auth string
	body               map[string]any
}

func fakeHost(t *testing.T, handle func(c hostCall) (int, string)) (*httptest.Server, *[]hostCall) {
	var mu sync.Mutex
	var calls []hostCall
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c := hostCall{method: r.Method, path: r.URL.EscapedPath() + "?" + r.URL.RawQuery, auth: r.Header.Get("PRIVATE-TOKEN") + "|" + r.Header.Get("JOB-TOKEN") + "|" + r.Header.Get("Authorization")}
		_ = json.Unmarshal(b, &c.body)
		mu.Lock()
		calls = append(calls, c)
		mu.Unlock()
		code, out := handle(c)
		w.WriteHeader(code)
		_, _ = w.Write([]byte(out))
	}))
	t.Cleanup(hs.Close)
	return hs, &calls
}

func TestGitLabHost(t *testing.T) {
	ctx := context.Background()
	page := func(from, n int, marker int) string {
		var list []map[string]any
		for i := from; i < from+n; i++ {
			body := fmt.Sprintf("note %d", i)
			if i == marker {
				body = "<!-- m --> summary"
			}
			list = append(list, map[string]any{"id": i, "body": body})
		}
		b, _ := json.Marshal(list)
		return string(b)
	}
	hs, calls := fakeHost(t, func(c hostCall) (int, string) {
		switch {
		case strings.HasPrefix(c.path, "/api/v4/projects/g%2Fp/statuses/aaa"):
			if c.body["state"] == "running" {
				return 400, `{"message":"Cannot transition status via :run from :running"}`
			}
			return 201, `{}`
		case strings.HasPrefix(c.path, "/api/v4/projects/g%2Fp/statuses/bad"):
			return 403, `{"message":"403 Forbidden"}`
		case c.method == "GET" && strings.Contains(c.path, "merge_requests/3/notes") && strings.Contains(c.path, "page=1&"):
			return 200, page(1, 100, -1)
		case c.method == "GET" && strings.Contains(c.path, "merge_requests/3/notes") && strings.Contains(c.path, "page=2&"):
			return 200, page(101, 5, 103)
		case c.method == "GET" && strings.Contains(c.path, "merge_requests/4/notes"):
			return 200, `[]`
		case c.method == "PUT" && strings.Contains(c.path, "merge_requests/3/notes/103"):
			return 200, `{}`
		case c.method == "POST" && strings.Contains(c.path, "merge_requests/4/notes"):
			return 201, `{}`
		}
		return 404, `{"message":"404 Not Found"}`
	})
	g := NewGitLabHost(hs.URL+"/api/v4", "/g/p/", "tok", GitLabPrivateToken, hs.Client())
	if g.Name() != "GitLab" {
		t.Fatal(g.Name())
	}
	long := strings.Repeat("x", 300)
	if err := g.SetStatus(ctx, CommitStatus{SHA: "aaa", State: StatusSuccess, Name: "qtestframe/gate", TargetURL: "https://q/x", Description: long, Ref: "main", PipelineID: "77"}); err != nil {
		t.Fatal(err)
	}
	c := (*calls)[0]
	if c.auth != "tok||" || c.body["pipeline_id"] != 77.0 || c.body["ref"] != "main" || len([]rune(c.body["description"].(string))) != 255 || c.body["target_url"] != "https://q/x" {
		t.Fatalf("%+v", c)
	}
	// the same state again is not an error
	if err := g.SetStatus(ctx, CommitStatus{SHA: "aaa", State: StatusRunning, Name: "n"}); err != nil {
		t.Fatal(err)
	}
	err := g.SetStatus(ctx, CommitStatus{SHA: "bad", State: StatusFailed, Name: "n"})
	var te *Error
	if !errors.As(err, &te) || te.Status != 403 || !strings.HasPrefix(err.Error(), "GitLab answered 403") {
		t.Fatal(err)
	}
	// the marker is on the second page: updated
	if up, err := g.UpsertNote(ctx, "!3", "<!-- m -->", "new"); err != nil || !up {
		t.Fatal(up, err)
	}
	if last := (*calls)[len(*calls)-1]; last.method != "PUT" || last.body["body"] != "new" {
		t.Fatalf("%+v", last)
	}
	if up, err := g.UpsertNote(ctx, "4", "<!-- m -->", "first"); err != nil || up {
		t.Fatal(up, err)
	}
	if _, err := g.UpsertNote(ctx, "x", "m", "b"); err == nil {
		t.Fatal("bad iid accepted")
	}
	if _, err := g.UpsertNote(ctx, "9", "m", "b"); err == nil || !strings.Contains(err.Error(), "GitLab answered 404") {
		t.Fatal(err)
	}
	// the job token goes in JOB-TOKEN; gitlab.com by default
	j := NewGitLabHost(hs.URL, "g/p", "job", GitLabJobToken, nil).(*gitlabHost)
	j.hc = hs.Client()
	if err := j.SetStatus(ctx, CommitStatus{SHA: "aaa", State: StatusSuccess, Name: "n"}); err != nil {
		t.Fatal(err)
	}
	if last := (*calls)[len(*calls)-1]; last.auth != "|job|" {
		t.Fatal(last.auth)
	}
	if d := NewGitLabHost("", "1", "", GitLabPrivateToken, nil).(*gitlabHost); d.base != "https://gitlab.com/api/v4/projects/1" {
		t.Fatal(d.base)
	}
}

func TestGitHubHost(t *testing.T) {
	ctx := context.Background()
	hs, calls := fakeHost(t, func(c hostCall) (int, string) {
		switch {
		case strings.HasPrefix(c.path, "/repos/o/r/statuses/"):
			return 201, `{}`
		case c.method == "GET" && strings.HasPrefix(c.path, "/repos/o/r/issues/7/comments"):
			return 200, `[{"id": 5, "body": "x"}, {"id": 6, "body": "<!-- m -->"}]`
		case c.method == "GET" && strings.HasPrefix(c.path, "/repos/o/r/issues/8/comments"):
			return 200, `[]`
		case c.method == "PATCH" && c.path == "/repos/o/r/issues/comments/6?":
			return 200, `{}`
		case c.method == "POST" && strings.HasPrefix(c.path, "/repos/o/r/issues/8/comments"):
			return 201, `{}`
		}
		return 404, `{"message":"Not Found"}`
	})
	g := NewGitHubHost(hs.URL, "o/r", "ghtok", hs.Client())
	if g.Name() != "GitHub" {
		t.Fatal(g.Name())
	}
	for state, want := range map[string]string{StatusFailed: "failure", StatusRunning: "pending", StatusCanceled: "error", StatusSuccess: "success"} {
		if err := g.SetStatus(ctx, CommitStatus{SHA: "abc", State: state, Name: "qtestframe/tests", TargetURL: "https://q", Description: strings.Repeat("y", 200)}); err != nil {
			t.Fatal(err)
		}
		c := (*calls)[len(*calls)-1]
		if c.body["state"] != want || c.body["context"] != "qtestframe/tests" || len([]rune(c.body["description"].(string))) != 140 || c.auth != "||Bearer ghtok" {
			t.Fatalf("%s: %+v", state, c)
		}
	}
	if up, err := g.UpsertNote(ctx, "#7", "<!-- m -->", "b"); err != nil || !up {
		t.Fatal(up, err)
	}
	if up, err := g.UpsertNote(ctx, "8", "<!-- m -->", "b"); err != nil || up {
		t.Fatal(up, err)
	}
	if _, err := g.UpsertNote(ctx, "0", "m", "b"); err == nil {
		t.Fatal("0 accepted")
	}
	if _, err := g.UpsertNote(ctx, "9", "m", "b"); err == nil || !strings.HasPrefix(err.Error(), "GitHub answered 404") {
		t.Fatal(err)
	}
	if d := NewGitHubHost("", "o/r", "", nil).(*githubHost); d.base != "https://api.github.com/repos/o/r" {
		t.Fatal(d.base)
	}
}
