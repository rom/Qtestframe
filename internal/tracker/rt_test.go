package tracker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/testutil/trackerfake"
)

func TestRT(t *testing.T) {
	f := trackerfake.NewRT()
	srv := httptest.NewServer(f)
	defer srv.Close()
	ctx := context.Background()
	cfg := &model.DefectTracker{Type: "rt", URL: srv.URL + "/rt/REST/2.0/", Queue: "QA", Token: trackerfake.RTToken, LabelsField: "Tags", Labels: "qtestframe"}
	c, err := New(cfg, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if c.Markup() != MarkupMarkdown {
		t.Fatal(c.Markup())
	}
	if err := c.Check(ctx); err != nil {
		t.Fatal(err)
	}
	is, err := c.Create(ctx, NewIssue{Title: "Login fails", Description: Managed(MarkupMarkdown, "Steps"), Severity: "major", Labels: []string{"login"}})
	if err != nil {
		t.Fatal(err)
	}
	if is.Key != "#101" || is.Mapped != "open" || is.Closed || !strings.HasSuffix(is.URL, "/rt/Ticket/Display.html?id=101") || is.Title != "Login fails" {
		t.Fatalf("%+v", is)
	}
	tk := f.Tickets[101]
	if tk["Content"] != "Steps\n\nSeverity: major" {
		t.Fatalf("content %q", tk["Content"])
	}
	if cf, _ := json.Marshal(tk["CustomFields"]); string(cf) != `{"Tags":["qtestframe","login"]}` {
		t.Fatalf("custom fields %s", cf)
	}
	for status, want := range map[string]string{"open": "in_progress", "stalled": "in_progress", "resolved": "closed", "rejected": "rejected", "deleted": "rejected", "qa": ""} {
		f.SetStatus(101, status)
		is, err := c.Get(ctx, "RT#101")
		if err != nil {
			t.Fatal(err)
		}
		if is.Mapped != want || is.Closed != (want == "closed" || want == "rejected") {
			t.Fatalf("%s: %+v", status, is)
		}
	}
	if _, err := c.Update(ctx, "101", NewIssue{Title: "Login fails on Safari", Description: "New text"}); err != nil {
		t.Fatal(err)
	}
	if f.Tickets[101]["Subject"] != "Login fails on Safari" || len(f.Comments[101]) != 1 || f.Comments[101][0] != "New text" {
		t.Fatalf("%v %v", f.Tickets[101], f.Comments)
	}
	if err := c.Comment(ctx, "#101", "All tests passed"); err != nil || len(f.Comments[101]) != 2 {
		t.Fatal(err, f.Comments)
	}
	for _, bad := range []string{"SHOP-1", "#0", "rt"} {
		if _, err := c.Get(ctx, bad); !errors.Is(err, ErrBadKey) {
			t.Fatal(bad, err)
		}
		if _, err := c.Update(ctx, bad, NewIssue{}); !errors.Is(err, ErrBadKey) {
			t.Fatal(bad, err)
		}
		if err := c.Comment(ctx, bad, "x"); !errors.Is(err, ErrBadKey) {
			t.Fatal(bad, err)
		}
	}
	if _, err := c.Get(ctx, "#999"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatal(err)
	}
	if _, err := c.Update(ctx, "#999", NewIssue{Title: "x"}); err == nil {
		t.Fatal("updated a missing ticket")
	}
	// a wrong queue and a wrong token
	bad := *cfg
	bad.Queue = "Nope"
	cb, _ := New(&bad, srv.Client())
	if err := cb.Check(ctx); err == nil {
		t.Fatal("unknown queue accepted")
	}
	if _, err := cb.Create(ctx, NewIssue{Title: "x"}); err == nil {
		t.Fatal("created in an unknown queue")
	}
	bad = *cfg
	bad.Token = "wrong"
	cb, _ = New(&bad, srv.Client())
	if err := cb.Check(ctx); err == nil || !strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "wrong") {
		t.Fatal(err)
	}
	// without a labels field no custom fields are sent
	nl := *cfg
	nl.LabelsField = ""
	cn, _ := New(&nl, srv.Client())
	if _, err := cn.Create(ctx, NewIssue{Title: "x", Labels: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Last["CustomFields"]; ok {
		t.Fatalf("%v", f.Last)
	}
	if !Owns(cfg, "rt", "#5") || Owns(cfg, "jira", "#5") {
		t.Fatal("owns")
	}
	if RTBase(" https://rt.example.com/REST/2.0/ ") != "https://rt.example.com" {
		t.Fatal(RTBase("https://rt.example.com/REST/2.0/"))
	}
}
