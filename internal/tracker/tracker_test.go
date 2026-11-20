package tracker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
)

func TestJira(t *testing.T) {
	var created map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "qa@example.com" || p != "tok" {
			w.WriteHeader(401)
			return
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/2/issue":
			_ = json.NewDecoder(r.Body).Decode(&created)
			_, _ = w.Write([]byte(`{"key":"SHOP-7"}`))
		case r.URL.Path == "/rest/api/2/issue/SHOP-7":
			_, _ = w.Write([]byte(`{"key":"SHOP-7","fields":{"summary":"Login fails","status":{"name":"In Review","statusCategory":{"key":"indeterminate"}}}}`))
		case r.URL.Path == "/rest/api/2/issue/SHOP-8":
			_, _ = w.Write([]byte(`{"key":"SHOP-8","fields":{"summary":"x","status":{"name":"Done","statusCategory":{"key":"done"}},"resolution":{"name":"Won't Do"}}}`))
		case r.URL.Path == "/rest/api/2/project/SHOP":
			_, _ = w.Write([]byte(`{"key":"SHOP"}`))
		default:
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"errorMessages":["Issue does not exist"]}`))
		}
	}))
	defer srv.Close()
	cfg := &model.DefectTracker{Type: "jira", URL: srv.URL, ProjectKey: "SHOP", IssueType: "Bug", User: "qa@example.com", Token: "tok"}
	c, err := New(cfg, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.Check(ctx); err != nil {
		t.Fatal(err)
	}
	is, err := c.Create(ctx, NewIssue{Title: "Login fails", Description: "Steps", Severity: "critical"})
	if err != nil || is.Key != "SHOP-7" || is.Mapped != "in_progress" || is.URL != srv.URL+"/browse/SHOP-7" || is.Status != "In Review" {
		t.Fatal(is, err)
	}
	f := created["fields"].(map[string]any)
	if f["summary"] != "Login fails" || !strings.Contains(f["description"].(string), "Severity: critical") || f["issuetype"].(map[string]any)["name"] != "Bug" {
		t.Fatal(created)
	}
	if is, err := c.Get(ctx, "shop-8"); err != nil || is.Mapped != "rejected" {
		t.Fatal(is, err)
	}
	_, err = c.Get(ctx, "SHOP-404")
	var te *Error
	if !errors.As(err, &te) || te.Status != 404 || !strings.Contains(err.Error(), "does not exist") {
		t.Fatal(err)
	}
	// keys that are not Jira keys never reach the tracker
	for _, k := range []string{"", "SHOP", "SHOP-1/../../x", "#12"} {
		if _, err := c.Get(ctx, k); !errors.Is(err, ErrBadKey) {
			t.Errorf("%q: %v", k, err)
		}
	}
	bad, _ := New(&model.DefectTracker{Type: "jira", URL: srv.URL, ProjectKey: "SHOP", Token: "x"}, srv.Client())
	if err := bad.Check(ctx); err == nil {
		t.Fatal("bearer token accepted")
	}
	for cat, want := range map[string]string{"new": "open", "indeterminate": "in_progress", "done": "closed", "odd": ""} {
		if got := jiraMapped(cat, "Done"); got != want {
			t.Errorf("%s: %s", cat, got)
		}
	}
}

func TestGitHub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ghp" {
			w.WriteHeader(401)
			return
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/repos/acme/shop/issues":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["labels"] == nil {
				w.WriteHeader(422)
				return
			}
			_, _ = w.Write([]byte(`{"number":12,"html_url":"https://github.com/acme/shop/issues/12","title":"Login fails","state":"open"}`))
		case r.URL.Path == "/repos/acme/shop/issues/12":
			_, _ = w.Write([]byte(`{"number":12,"html_url":"u","title":"Login fails","state":"closed","state_reason":"completed"}`))
		case r.URL.Path == "/repos/acme/shop/issues/13":
			_, _ = w.Write([]byte(`{"number":13,"html_url":"u","title":"x","state":"closed","state_reason":"not_planned"}`))
		case r.URL.Path == "/repos/acme/shop":
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c, _ := New(&model.DefectTracker{Type: "github", URL: srv.URL, Repo: "acme/shop", Token: "ghp", Labels: "bug, qtestframe"}, srv.Client())
	ctx := context.Background()
	if err := c.Check(ctx); err != nil {
		t.Fatal(err)
	}
	is, err := c.Create(ctx, NewIssue{Title: "Login fails"})
	if err != nil || is.Key != "acme/shop#12" || is.Mapped != "open" {
		t.Fatal(is, err)
	}
	if is, err := c.Get(ctx, "acme/shop#12"); err != nil || is.Mapped != "closed" || is.Status != "closed (completed)" {
		t.Fatal(is, err)
	}
	if is, err := c.Get(ctx, "#13"); err != nil || is.Mapped != "rejected" {
		t.Fatal(is, err)
	}
	if _, err := c.Get(ctx, "abc"); !errors.Is(err, ErrBadKey) {
		t.Fatal("bad key", err)
	}
	if _, err := New(&model.DefectTracker{}, nil); err == nil {
		t.Fatal("no tracker")
	}
	if _, err := New(&model.DefectTracker{Type: "github"}, nil); err != nil {
		t.Fatal(err)
	}
	unreachable, _ := New(&model.DefectTracker{Type: "github", URL: "http://127.0.0.1:1", Repo: "a/b"}, nil)
	if err := unreachable.Check(ctx); err == nil || !strings.Contains(err.Error(), "cannot reach") {
		t.Fatal(err)
	}
	long := (&Error{Status: 500, Body: strings.Repeat("x", 400)}).Error()
	if !strings.HasSuffix(long, "…") {
		t.Fatal(long)
	}
}
