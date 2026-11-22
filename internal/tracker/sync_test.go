package tracker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/testutil/trackerfake"
)

func TestMarkdownJira(t *testing.T) {
	md := "# Goal\nThe user **must** log in with `SSO` and *passkeys*.\n- one\n  - two\n1. first\n> quoted\n```go\nx := **y**\n```\nSee [docs](https://example.test/docs) and ~~old~~."
	got := MarkdownJira(md)
	want := "h1. Goal\nThe user *must* log in with {{SSO}} and _passkeys_.\n* one\n** two\n# first\nbq. quoted\n{code:go}\nx := **y**\n{code}\nSee [docs|https://example.test/docs] and -old-."
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	// and back
	if back := JiraMarkdown(got); !strings.Contains(back, "**must**") || !strings.Contains(back, "[docs](https://example.test/docs)") {
		t.Fatal(back)
	}
	if MarkdownJira("```\nopen") != "{code}\nopen\n{code}" {
		t.Fatal(MarkdownJira("```\nopen"))
	}
	if h := markdownHTML("- a\n- **b**"); h != "<div><ul><li>a</li><li><strong>b</strong></li></ul></div>" {
		t.Fatal(h)
	}
}

func TestHookKeys(t *testing.T) {
	if ev, keys := JiraHookKeys([]byte(`{"webhookEvent":"jira:issue_updated","issue":{"key":"SHOP-1"}}`)); ev != "jira:issue_updated" || len(keys) != 1 || keys[0] != "SHOP-1" {
		t.Fatal(ev, keys)
	}
	if ev, keys := JiraHookKeys([]byte(`not json`)); ev != "" || keys != nil {
		t.Fatal(ev, keys)
	}
	for body, want := range map[string]string{
		`{"eventType":"workitem.updated","resource":{"id":77,"workItemId":12,"revision":{"id":12}}}`: "12",
		`{"eventType":"workitem.updated","resource":{"id":77,"revision":{"id":13}}}`:                 "13",
		`{"eventType":"workitem.created","resource":{"id":14}}`:                                      "14",
		`{"eventType":"workitem.commented","resource":{"id":15}}`:                                    "15",
		`{"eventType":"workitem.updated","resource":{"id":77}}`:                                      "",
	} {
		_, keys := AzureHookKeys([]byte(body))
		if strings.Join(keys, ",") != want {
			t.Fatal(body, keys)
		}
	}
	if ev, _ := AzureHookKeys([]byte(`[`)); ev != "" {
		t.Fatal(ev)
	}
}

func TestSyncJira(t *testing.T) {
	f := trackerfake.NewJira()
	f.Classic = true // Jira Server / Data Center: the classic search and a personal access token
	f.Issues["SHOP-1"] = map[string]any{"summary": "Login", "status": map[string]any{"name": "To Do", "statusCategory": map[string]any{"key": "new"}}}
	f.Touch("SHOP-1")
	ts := httptest.NewServer(f)
	defer ts.Close()
	cfg := &model.DefectTracker{Type: "jira", URL: ts.URL, Token: trackerfake.JiraToken}
	c := NewSyncJira(cfg, ts.Client(), "SHOP", "")
	ctx := context.Background()
	if c.System() != SourceJira {
		t.Fatal(c.System())
	}
	res, err := c.Search(ctx, "project = SHOP", 0)
	if err != nil || len(res.Items) != 1 || res.Items[0].Version == "" {
		t.Fatal(res, err)
	}
	v := res.Items[0].Version
	// the issue changed since it was read
	f.Set("SHOP-1", "summary", "Login (edited)")
	if err := c.Update(ctx, "SHOP-1", v, []FieldWrite{{ID: "summary", Kind: WriteText, Value: "x"}}); !errors.Is(err, ErrRemoteChanged) {
		t.Fatal(err)
	}
	items, err := c.Fetch(ctx, []string{"SHOP-1", "SHOP-99", "bad key"})
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	// fields, a label list, an option and a transition
	err = c.Update(ctx, "SHOP-1", items[0].Version, []FieldWrite{{ID: "summary", Kind: WriteText, Value: "Login v2"}, {ID: "labels", Kind: WriteList, Value: "a b, c"},
		{ID: "priority", Kind: WriteOption, Value: "High"}, {ID: "description", Kind: WriteMarkdown, Value: "**bold**"}, {ID: "status", Kind: WriteStatus, Value: "in progress"}})
	if err != nil {
		t.Fatal(err)
	}
	if f.Field("SHOP-1", "summary") != "Login v2" || f.Field("SHOP-1", "description") != "*bold*" || f.Field("SHOP-1", "status").(map[string]any)["name"] != "In Progress" {
		t.Fatal(f.Issues["SHOP-1"])
	}
	if l := f.Field("SHOP-1", "labels").([]any); len(l) != 2 || l[0] != "a_b" {
		t.Fatal(l)
	}
	// a status the workflow does not offer
	if err := c.Update(ctx, "SHOP-1", "", []FieldWrite{{ID: "status", Kind: WriteStatus, Value: "Archived"}}); !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "Done") {
		t.Fatal(err)
	}
	if err := c.Update(ctx, "SHOP-404", "", nil); !errors.Is(err, ErrRemoteMissing) {
		t.Fatal(err)
	}
	key, err := c.Create(ctx, "", []FieldWrite{{ID: "summary", Kind: WriteText, Value: "New"}, {ID: "status", Kind: WriteStatus, Value: "Done"}, {ID: "priority", Kind: WriteOption, Value: ""}})
	if err != nil || key != "SHOP-2" || f.Field("SHOP-2", "status").(map[string]any)["name"] != "Done" || f.Field("SHOP-2", "issuetype").(map[string]any)["name"] != "Story" {
		t.Fatal(key, err, f.Issues["SHOP-2"])
	}
	if _, err := c.Create(ctx, "Task", []FieldWrite{{ID: "summary", Kind: WriteText, Value: "x"}, {ID: "status", Kind: WriteStatus, Value: "Nowhere"}}); err == nil || !strings.Contains(err.Error(), "created SHOP-3") {
		t.Fatal(err)
	}
	if _, err := NewSyncJira(cfg, ts.Client(), "", "").Create(ctx, "", nil); !errors.Is(err, ErrConfig) {
		t.Fatal(err)
	}
	if _, err := c.Create(ctx, "", nil); err == nil {
		t.Fatal("no summary")
	}
	// comments
	id, err := c.AddComment(ctx, "SHOP-1", "Hello **there**")
	if err != nil || id == "" {
		t.Fatal(id, err)
	}
	cs, err := c.Comments(ctx, "SHOP-1")
	if err != nil || len(cs) != 1 || cs[0].Body != "Hello **there**" || cs[0].Author != "Sync Bot" || cs[0].Created.Year() != 2030 {
		t.Fatal(cs, err)
	}
	if _, err := c.Comments(ctx, "bad"); !errors.Is(err, ErrBadKey) {
		t.Fatal(err)
	}
	if _, err := c.AddComment(ctx, "bad", "x"); !errors.Is(err, ErrBadKey) {
		t.Fatal(err)
	}
	// links: depends_on is the reverse of Blocks
	if err := c.AddLink(ctx, SourceLink{Type: "depends_on", Source: "SHOP-1", Target: "SHOP-2"}); err != nil {
		t.Fatal(err)
	}
	items, _ = c.Fetch(ctx, []string{"SHOP-1", "SHOP-2"})
	var found bool
	for _, it := range items {
		for _, l := range it.Links {
			found = found || l.Type == "depends_on" && l.Source == "SHOP-1" && l.Target == "SHOP-2"
		}
	}
	if !found {
		t.Fatal(items)
	}
	if err := c.AddLink(ctx, SourceLink{Type: "refines", Source: "SHOP-1", Target: "SHOP-2"}); !errors.Is(err, ErrConfig) {
		t.Fatal(err)
	}
	for _, rel := range []string{"relates_to", "duplicates", "derived_from"} {
		if n, _ := JiraLinkType(rel); n == "" {
			t.Fatal(rel)
		}
	}
	if jiraTime("nonsense").IsZero() != true || jiraTime("2026-01-02T03:04:05Z").IsZero() {
		t.Fatal("times")
	}
}

func TestSyncAzure(t *testing.T) {
	f := trackerfake.NewAzure()
	f.Items[5] = map[string]any{"System.Title": "Checkout", "System.WorkItemType": "User Story", "System.State": "New"}
	ts := httptest.NewServer(f)
	defer ts.Close()
	c := NewSyncAzure(&model.AzureDevOpsSource{URL: ts.URL + "/org", Project: "Shop", Token: trackerfake.AzureToken}, ts.Client(), "")
	ctx := context.Background()
	if c.System() != SourceAzure {
		t.Fatal(c.System())
	}
	items, err := c.Fetch(ctx, []string{"5", "6", "x"})
	if err != nil || len(items) != 1 || items[0].Version != "1" {
		t.Fatal(items, err)
	}
	if l, _ := c.Fetch(ctx, []string{"x"}); l != nil {
		t.Fatal(l)
	}
	err = c.Update(ctx, "5", "1", []FieldWrite{{ID: "System.Title", Kind: WriteText, Value: "Checkout v2"}, {ID: "System.Tags", Kind: WriteList, Value: "a, b"},
		{ID: "Microsoft.VSTS.Common.Priority", Kind: WriteOption, Value: "2"}, {ID: "System.State", Kind: WriteStatus, Value: "Active"}, {ID: "System.Description", Kind: WriteMarkdown, Value: "x"}})
	if err != nil || f.Field(5, "System.Tags") != "a; b" || f.Field(5, "Microsoft.VSTS.Common.Priority") != float64(2) || f.Field(5, "System.State") != "Active" {
		t.Fatal(err, f.Items[5])
	}
	// the revision read is no longer current
	if err := c.Update(ctx, "5", "1", []FieldWrite{{ID: "System.Title", Kind: WriteText, Value: "stale"}}); !errors.Is(err, ErrRemoteChanged) {
		t.Fatal(err)
	}
	if err := c.Update(ctx, "99", "", []FieldWrite{{ID: "System.Title", Kind: WriteText, Value: "x"}}); !errors.Is(err, ErrRemoteMissing) {
		t.Fatal(err)
	}
	if err := c.Update(ctx, "x", "", nil); !errors.Is(err, ErrBadKey) {
		t.Fatal(err)
	}
	key, err := c.Create(ctx, "", []FieldWrite{{ID: "System.Title", Kind: WriteText, Value: "New"}, {ID: "System.State", Kind: WriteStatus, Value: "Active"}})
	if err != nil || f.Field(1001, "System.WorkItemType") != "User Story" || f.Field(1001, "System.State") != "Active" || key != "1001" {
		t.Fatal(key, err, f.Items)
	}
	if _, err := c.Create(ctx, "Bug", nil); err == nil {
		t.Fatal("no title")
	}
	id, err := c.AddComment(ctx, "5", "Hi **all**")
	if err != nil || id == "" {
		t.Fatal(err)
	}
	f.AddComment(5, "Ann", "<p>Seen</p>")
	cs, err := c.Comments(ctx, "5")
	if err != nil || len(cs) != 2 || cs[0].Body != "Hi **all**" || cs[1].Author != "Ann" || cs[1].Body != "Seen" {
		t.Fatal(cs, err)
	}
	if _, err := c.Comments(ctx, "x"); !errors.Is(err, ErrBadKey) {
		t.Fatal(err)
	}
	if _, err := c.AddComment(ctx, "x", "y"); !errors.Is(err, ErrBadKey) {
		t.Fatal(err)
	}
	if err := c.AddLink(ctx, SourceLink{Type: "refines", Source: "1001", Target: "5"}); err != nil || f.Relations[1001][0]["rel"] != "System.LinkTypes.Hierarchy-Reverse" {
		t.Fatal(err, f.Relations)
	}
	if err := c.AddLink(ctx, SourceLink{Type: "derived_from", Source: "1001", Target: "5"}); !errors.Is(err, ErrConfig) {
		t.Fatal(err)
	}
	if err := c.AddLink(ctx, SourceLink{Type: "relates_to", Source: "1001", Target: "x"}); !errors.Is(err, ErrBadKey) {
		t.Fatal(err)
	}
	for _, rel := range []string{"relates_to", "depends_on", "duplicates"} {
		if AzureLinkType(rel) == "" {
			t.Fatal(rel)
		}
	}
	// a write refused with a conflict status
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusConflict) }))
	defer ts2.Close()
	c2 := NewSyncAzure(&model.AzureDevOpsSource{URL: ts2.URL, Project: "P"}, ts2.Client(), "")
	if err := c2.Update(ctx, "1", "", nil); !errors.Is(err, ErrRemoteChanged) {
		t.Fatal(err)
	}
}
