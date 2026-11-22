package tracker

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/testutil/trackerfake"
)

func jiraFixture() *trackerfake.Jira {
	f := trackerfake.NewJira()
	f.Names["customfield_10050"] = "Acceptance Criteria"
	f.Names["customfield_10060"] = "Risk"
	f.Schema["customfield_10050"] = map[string]string{"type": "string", "custom": "com.atlassian.jira.plugin.system.customfieldtypes:textarea"}
	f.Issues["SHOP-1"] = map[string]any{
		"summary": "Login", "description": "h2. Goal\nThe user *must* log in with {{SSO}}.\n* one\n** two\n# first\n[docs|https://example.test/docs]",
		"priority": map[string]any{"name": "Highest"}, "labels": []any{"auth", "web"},
		"status":            map[string]any{"name": "In Progress", "statusCategory": map[string]any{"key": "indeterminate"}},
		"issuetype":         map[string]any{"name": "Story"},
		"customfield_10050": "Login works\nLogout works",
		"customfield_10060": map[string]any{"value": "High", "child": map[string]any{"value": "Security"}},
		"issuelinks": []any{
			map[string]any{"id": "900", "type": map[string]any{"name": "Blocks", "outward": "blocks"}, "outwardIssue": map[string]any{"key": "SHOP-2"}},
			map[string]any{"id": "901", "type": map[string]any{"name": "Relates", "outward": "relates to"}, "inwardIssue": map[string]any{"key": "SHOP-3"}},
		},
		"comment": map[string]any{"comments": []any{}}, "watches": map[string]any{"watchCount": 1}, "customfield_10099": nil,
	}
	f.Issues["SHOP-2"] = map[string]any{"summary": "Logout", "status": map[string]any{"name": "Done", "statusCategory": map[string]any{"key": "done"}},
		"parent": map[string]any{"key": "SHOP-1"}, "description": map[string]any{"type": "doc", "content": []any{
			map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "From API v3"}}}}}}
	f.Issues["SHOP-3"] = map[string]any{"summary": "Profile", "issuelinks": []any{
		map[string]any{"id": "901", "type": map[string]any{"name": "Relates", "outward": "relates to"}, "outwardIssue": map[string]any{"key": "SHOP-1"}},
		map[string]any{"id": "902", "type": map[string]any{"name": "Duplicate", "outward": "duplicates"}, "outwardIssue": map[string]any{"key": "SHOP-2"}},
		map[string]any{"id": "903", "type": map[string]any{"name": "Cloners", "outward": "clones"}, "outwardIssue": map[string]any{"key": "SHOP-1"}},
		map[string]any{"id": "904", "type": map[string]any{"name": "Implements", "outward": "implements"}, "outwardIssue": map[string]any{"key": "SHOP-1"}},
	}, "customfield_10070": 3.5, "customfield_10071": true}
	f.Issues["OTHER-1"] = map[string]any{"summary": "Elsewhere"}
	return f
}

func TestSearchJira(t *testing.T) {
	for _, classic := range []bool{false, true} {
		f := jiraFixture()
		f.Classic = classic
		srv := httptest.NewServer(f)
		cfg := &model.DefectTracker{Type: "jira", URL: srv.URL + "/", User: trackerfake.JiraUser, Token: trackerfake.JiraToken}
		res, err := SearchJira(context.Background(), cfg, srv.Client(), "project = SHOP ORDER BY key", 0)
		if err != nil {
			t.Fatal(classic, err)
		}
		if len(res.Items) != 3 || res.Truncated || res.Items[0].Key != "SHOP-1" || res.Items[2].Key != "SHOP-3" || res.System != SourceJira || res.Base != srv.URL {
			t.Fatalf("classic=%v: %+v", classic, res)
		}
		one := res.Items[0]
		desc := one.Values["description"]
		for _, want := range []string{"## Goal", "The user **must** log in with `SSO`.", "- one\n  - two", "1. first", "[docs](https://example.test/docs)"} {
			if !strings.Contains(desc, want) {
				t.Errorf("description lacks %q:\n%s", want, desc)
			}
		}
		if one.Values["priority"] != "Highest" || one.Values["labels"] != "auth, web" || one.Values["status"] != "In Progress" || one.Category != "indeterminate" ||
			one.Type != "Story" || one.Values["customfield_10060"] != "High / Security" || one.URL != srv.URL+"/browse/SHOP-1" {
			t.Fatalf("%+v", one)
		}
		if _, ok := one.Values["comment"]; ok {
			t.Fatal("comment field imported")
		}
		if len(one.Links) != 1 || one.Links[0] != (SourceLink{Ref: "link:900", Type: "depends_on", Name: "Blocks", Source: "SHOP-2", Target: "SHOP-1"}) {
			t.Fatalf("%+v", one.Links)
		}
		two := res.Items[1]
		if two.Values["description"] != "From API v3" || len(two.Links) != 1 || two.Links[0].Type != "refines" || two.Links[0].Target != "SHOP-1" {
			t.Fatalf("%+v", two)
		}
		three := res.Items[2]
		types := []string{}
		for _, l := range three.Links {
			types = append(types, l.Type)
		}
		if strings.Join(types, ",") != "relates_to,duplicates,derived_from," || three.Values["customfield_10070"] != "3.5" || three.Values["customfield_10071"] != "true" {
			t.Fatalf("%v %+v", types, three)
		}
		// fields: the mapped ones first, names from the search
		names := map[string]SourceField{}
		for _, fd := range res.Fields {
			names[fd.ID] = fd
		}
		if res.Fields[0].ID != "description" || names["customfield_10050"] != (SourceField{ID: "customfield_10050", Name: "Acceptance Criteria", Type: FieldMarkdown}) ||
			names["labels"].Type != FieldList || names["customfield_10070"].Name != "customfield_10070" {
			t.Fatalf("%+v", res.Fields)
		}
		// a limit truncates
		res, err = SearchJira(context.Background(), cfg, srv.Client(), "project = SHOP", 2)
		if err != nil || len(res.Items) != 2 || !res.Truncated {
			t.Fatal(err, res)
		}
		// errors of the query and of the credentials
		if _, err := SearchJira(context.Background(), cfg, srv.Client(), "syntax error", 0); err == nil || !strings.Contains(err.Error(), "Error in the JQL Query") {
			t.Fatal(err)
		}
		bad := *cfg
		bad.Token = "nope"
		if _, err := SearchJira(context.Background(), &bad, srv.Client(), "x", 0); err == nil || !strings.Contains(err.Error(), "401") {
			t.Fatal(err)
		}
		srv.Close()
	}
	// a bearer token (Jira Data Center personal access token)
	f := jiraFixture()
	srv := httptest.NewServer(f)
	defer srv.Close()
	res, err := SearchJira(context.Background(), &model.DefectTracker{URL: srv.URL, Token: trackerfake.JiraToken}, nil, "project = OTHER", 0)
	if err != nil || len(res.Items) != 1 {
		t.Fatal(err, res)
	}
}

func TestJiraMarkdown(t *testing.T) {
	cases := map[string]string{
		"h1. Title":                        "# Title",
		"bq. quoted":                       "> quoted",
		"*bold* and _it_ and -gone-":       "**bold** and *it* and ~~gone~~",
		"a well-known +under+ word":        "a well-known under word",
		"{{code *x*}} stays":               "`code *x*` stays",
		"[https://example.test]":           "<https://example.test>",
		"[~alice] says hi":                 "@alice says hi",
		"!shot.png|thumbnail!":             "![](shot.png)",
		"{color:red}red{color}":            "red",
		"----":                             "---",
		"||A||B||\n|1|2|":                  "| A | B |\n| --- | --- |\n| 1 | 2 |",
		"|1|2|":                            "|   |   |\n| --- | --- |\n| 1 | 2 |",
		"{code:java}\nint *x*;\n{code}":    "```java\nint *x*;\n```",
		"{noformat}raw *x*{noformat}":      "```\nraw *x*\n```",
		"{code}\nunclosed":                 "```\nunclosed\n```",
		"{quote}\nwise\nwords\n{quote}":    "> wise\n> words",
		"{panel:title=x}\ninside\n{panel}": "inside",
		"line\\\\next":                     "line  \nnext",
		"- dash\n\n\n\nafter":              "- dash\n\nafter",
		"{code:language=go}x{code}":        "```go\nx\n```",
		"{code}\na\nb{code}":               "```\na\nb\n```",
	}
	for in, want := range cases {
		if got := JiraMarkdown(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestAzure(t *testing.T) {
	f := trackerfake.NewAzure()
	f.Items[11] = map[string]any{"System.Title": "Login", "System.Description": "<div><b>Must</b> log in.<br>Second line</div><ul><li>one</li></ul>",
		"System.State": "Active", "System.Tags": "auth; web", "System.WorkItemType": "User Story", "Microsoft.VSTS.Common.Priority": 1.0,
		"Microsoft.VSTS.Common.AcceptanceCriteria": "<ul><li>Login works</li><li>Logout works</li></ul>", "System.Rev": 4.0,
		"System.AssignedTo": map[string]any{"displayName": "Alice", "uniqueName": "alice@example.test"}, "System.AreaPath": "Shop\\Web"}
	f.Items[12] = map[string]any{"System.Title": "Logout", "System.WorkItemType": "User Story", "System.State": "Closed"}
	f.Items[13] = map[string]any{"System.Title": "A bug", "System.WorkItemType": "Bug"}
	f.Link(12, "System.LinkTypes.Hierarchy-Reverse", 11)
	f.Link(11, "System.LinkTypes.Hierarchy-Forward", 12)
	f.Link(11, "System.LinkTypes.Related", 12)
	f.Link(12, "System.LinkTypes.Related", 11)
	f.Link(12, "System.LinkTypes.Dependency-Reverse", 11)
	f.Link(11, "System.LinkTypes.Dependency-Forward", 12)
	f.Link(12, "System.LinkTypes.Duplicate-Reverse", 11)
	f.Link(12, "Custom.Implements-Forward", 11)
	f.Link(12, "Custom.Implements-Reverse", 11)
	f.Link(12, "Microsoft.VSTS.Common.TestedBy-Forward", 11)
	f.Relations[12] = append(f.Relations[12], map[string]any{"rel": "Hyperlink", "url": "https://example.test"}, map[string]any{"rel": "x", "url": "https://h/workItems/abc"})
	srv := httptest.NewServer(f)
	defer srv.Close()
	ctx := context.Background()
	cfg := &model.AzureDevOpsSource{URL: srv.URL + "/org/", Project: "Shop", Token: trackerfake.AzureToken}
	a := NewAzure(cfg, srv.Client())
	if err := a.Check(ctx); err != nil {
		t.Fatal(err)
	}
	q := AzureWIQL(`Shop\Web's`, []string{"User Story", " ", "Requirement"})
	if q != `SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project AND [System.AreaPath] UNDER 'Shop\Web''s' AND [System.WorkItemType] IN ('User Story', 'Requirement') ORDER BY [System.Id]` {
		t.Fatal(q)
	}
	res, err := a.Search(ctx, q, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 || res.Truncated || res.Base != srv.URL+"/org/Shop" || res.System != SourceAzure {
		t.Fatalf("%+v", res)
	}
	one := res.Items[0]
	if one.Key != "11" || one.Type != "User Story" || one.Values["System.Tags"] != "auth, web" || one.Values["Microsoft.VSTS.Common.Priority"] != "1" ||
		one.Values["System.AssignedTo"] != "Alice" || !strings.HasSuffix(one.URL, "/_workitems/edit/11") {
		t.Fatalf("%+v", one)
	}
	if _, ok := one.Values["System.Rev"]; ok {
		t.Fatal("bookkeeping field imported")
	}
	if d := one.Values["System.Description"]; !strings.Contains(d, "**Must** log in.\nSecond line") || !strings.Contains(d, "- one") {
		t.Fatalf("%q", d)
	}
	if c := one.Values[AzureCriteria]; c != "- Login works\n- Logout works" {
		t.Fatalf("%q", c)
	}
	refs := map[string]string{}
	for _, it := range res.Items {
		for _, l := range it.Links {
			refs[l.Ref] = l.Type + " " + l.Source + ">" + l.Target
		}
	}
	want := map[string]string{
		"System.LinkTypes.Hierarchy-Reverse:12:11": "refines 12>11",
		"related:11:12": "relates_to 11>12",
		"System.LinkTypes.Dependency-Reverse:12:11": "depends_on 12>11",
		"System.LinkTypes.Duplicate-Reverse:12:11":  "duplicates 12>11",
		"Custom.Implements-Forward:12:11":           " 12>11",
	}
	if len(refs) != len(want) {
		t.Fatalf("%v", refs)
	}
	for k, v := range want {
		if refs[k] != v {
			t.Fatalf("%s: %q in %v", k, refs[k], refs)
		}
	}
	if res.Fields[0].ID != "Microsoft.VSTS.Common.AcceptanceCriteria" && res.Fields[0].ID != "System.Description" {
		t.Fatalf("%+v", res.Fields)
	}
	// all types, truncated
	res, err = a.Search(ctx, AzureWIQL("", nil), 2)
	if err != nil || len(res.Items) != 2 || !res.Truncated {
		t.Fatal(err, res)
	}
	if _, err := a.Search(ctx, "syntax error", 0); err == nil || !strings.Contains(err.Error(), "TF51005") {
		t.Fatal(err)
	}
	bad := *cfg
	bad.Token = "nope"
	if err := NewAzure(&bad, nil).Check(ctx); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatal(err)
	}
	bad = *cfg
	bad.Project = "Nope"
	if err := NewAzure(&bad, srv.Client()).Check(ctx); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatal(err)
	}
	if _, err := NewAzure(&bad, srv.Client()).Search(ctx, "x", 0); err == nil {
		t.Fatal("searched an unknown project")
	}
}

func TestDefaultSourceMapping(t *testing.T) {
	if DefaultSourceMapping(SourceJira)["summary"] != "title" || DefaultSourceMapping(SourceAzure)[AzureCriteria] != "criteria" {
		t.Fatal("mapping")
	}
}
