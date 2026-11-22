package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/testutil/trackerfake"
)

func byTitle(list []map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, r := range list {
		out[r["title"].(string)] = r
	}
	return out
}

func rowsByRef(pv map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, x := range pv["rows"].([]any) {
		m := x.(map[string]any)
		out[m["ref"].(string)] = m
	}
	return out
}

func TestRequirementImportJira(t *testing.T) {
	e := newEnv(t)
	jf := trackerfake.NewJira()
	jf.Names["customfield_10050"] = "Acceptance Criteria"
	jf.Names["customfield_10060"] = "Risk class"
	jf.Schema["customfield_10050"] = map[string]string{"type": "string", "custom": "com.atlassian.jira.plugin.system.customfieldtypes:textarea"}
	jf.Issues["SHOP-1"] = map[string]any{"summary": "Login", "description": "The user *must* log in.\n* with SSO",
		"priority": map[string]any{"name": "Highest"}, "labels": []any{"auth", "web"},
		"status":            map[string]any{"name": "In Progress", "statusCategory": map[string]any{"key": "indeterminate"}},
		"customfield_10050": "* Login works\n* Logout works", "customfield_10060": map[string]any{"value": "B"},
		"issuelinks": []any{map[string]any{"id": "900", "type": map[string]any{"name": "Blocks", "outward": "blocks"}, "outwardIssue": map[string]any{"key": "SHOP-2"}}}}
	jf.Issues["SHOP-2"] = map[string]any{"summary": "Logout", "status": map[string]any{"name": "Waiting for QA", "statusCategory": map[string]any{"key": "done"}},
		"priority": map[string]any{"name": "Blocker-ish"}, "parent": map[string]any{"key": "SHOP-1"}}
	jf.Issues["SHOP-3"] = map[string]any{"summary": "Profile page"}
	ts := httptest.NewServer(jf)
	defer ts.Close()
	e.srv.HTTPClient = ts.Client()
	a := e.login("admin@example.com")
	pid := a.project("JIM")
	base := "/api/v1/projects/" + pid
	a.m("POST", base+"/custom-fields", map[string]any{"entity_type": "requirement", "key": "risk_class", "label": "Risk class", "type": "single_select", "options": []string{"A", "B", "C"}}, 201)
	folder := a.m("POST", base+"/folders", map[string]any{"name": "From Jira", "kind": "requirement"}, 201)["id"].(string)

	// no connection yet
	a.m("POST", base+"/requirements/import/jira/preview", map[string]any{"query": "project = SHOP"}, 400)
	a.m("POST", base+"/requirements/import/nope/preview", map[string]any{}, 400)
	a.m("PUT", base+"/import-sources", map[string]any{"jira": map[string]any{"url": "ftp://x"}}, 400)
	a.m("PUT", base+"/import-sources", map[string]any{"azure_devops": map[string]any{"url": "https://dev.azure.com/x"}}, 400)
	a.m("PUT", base+"/import-sources", map[string]any{"azure_devops": map[string]any{"project": "Shop"}}, 400)
	// the defect tracker's Jira connection is used without one of its own
	a.m("PUT", base+"/defect-tracker", map[string]any{"type": "jira", "url": ts.URL, "project_key": "SHOP", "user": trackerfake.JiraUser, "token": trackerfake.JiraToken}, 200)
	src := a.m("GET", base+"/import-sources", nil, 200)
	if src["jira_from_tracker"] != true || src["tracker_jira_key"] != "SHOP" {
		t.Fatal(src)
	}
	a.m("POST", base+"/import-sources/jira/check", nil, 200)

	pv := a.m("POST", base+"/requirements/import/jira/preview", map[string]any{"folder_id": folder}, 200)
	if s := summaryOf(pv); s["new"].(float64) != 3 || s["errors"].(float64) != 0 || pv["query"] != "project = SHOP ORDER BY key" || pv["source"] != "jira" {
		t.Fatalf("preview: %v %v", s, pv)
	}
	attrs := map[string]string{}
	for _, x := range pv["attributes"].([]any) {
		m := x.(map[string]any)
		attrs[m["name"].(string)] = m["field"].(string) + "/" + m["guess"].(string)
	}
	if attrs["Summary"] != "title/title" || attrs["Acceptance Criteria"] != "criteria/criteria" || attrs["Labels"] != "tags/tags" || attrs["Risk class"] != "cf.risk_class/cf.risk_class" {
		t.Fatalf("attributes %v", attrs)
	}
	for _, tg := range pv["targets"].([]any) {
		if tg.(map[string]any)["type"] == "role" {
			t.Fatal("ReqIF roles offered", tg)
		}
	}
	rows := rowsByRef(pv)
	if rows["SHOP-1"]["location"] != "Jira SHOP-1" || len(rows["SHOP-2"]["warnings"].([]any)) != 1 {
		t.Fatalf("rows %v", rows)
	}
	if rels := pv["relations"].(map[string]any); rels["add"].(float64) != 2 {
		t.Fatalf("relations %v", rels)
	}
	// SHOP-3 is left out
	res := a.m("POST", base+"/requirements/import/jira/apply", map[string]any{"folder_id": folder, "exclude_refs": []string{"SHOP-3"}}, 200)
	if res["created"].(float64) != 2 || res["skipped"].(float64) != 1 || res["relations_added"].(float64) != 2 || res["references_saved"].(float64) != 2 {
		t.Fatalf("apply %v", res)
	}
	reqs := byTitle(items(a.m("GET", base+"/requirements", nil, 200)))
	login, logout := reqs["Login"], reqs["Logout"]
	if login == nil || logout == nil || login["folder_path"] != "From Jira" || login["priority"] != "critical" || login["status"] != "approved" ||
		login["source"] != "Jira SHOP-1" || !strings.Contains(login["description"].(string), "The user **must** log in.\n- with SSO") ||
		login["custom_fields"].(map[string]any)["risk_class"] != "B" || logout["status"] != "implemented" || logout["priority"] != "medium" {
		t.Fatalf("%v\n%v", login, logout)
	}
	full := a.m("GET", "/api/v1/requirements/"+login["id"].(string), nil, 200)
	if cs := full["criteria"].([]any); len(cs) != 2 || cs[0].(map[string]any)["description"] != "Login works" {
		t.Fatalf("criteria %v", full["criteria"])
	}
	types := map[string]bool{}
	for _, r := range items(a.m("GET", base+"/relations", nil, 200)) {
		types[r["type"].(string)+" "+r["source_key"].(string)] = true
	}
	if !types["depends_on "+logout["key"].(string)] || !types["refines "+logout["key"].(string)] {
		t.Fatalf("relations %v", types)
	}

	// re-import: Jira changed SHOP-1, someone edited Logout here, the
	// parent link is gone; nothing is duplicated
	jf.Issues["SHOP-1"]["summary"] = "Login with SSO"
	delete(jf.Issues["SHOP-2"], "parent")
	a.m("PATCH", "/api/v1/requirements/"+logout["id"].(string), map[string]any{"title": "Logout (edited)"}, 200)
	pv = a.m("POST", base+"/requirements/import/jira/preview", map[string]any{"query": "project = SHOP", "mapping": map[string]string{"Risk class": ""}}, 200)
	s := summaryOf(pv)
	if s["changed"].(float64) != 1 || s["conflicts"].(float64) != 1 || s["new"].(float64) != 1 || pv["previous"].(float64) != 2 || pv["relations"].(map[string]any)["remove"].(float64) != 1 {
		t.Fatalf("re-import preview %v %v", s, pv["relations"])
	}
	res = a.m("POST", base+"/requirements/import/jira/apply", map[string]any{"query": "project = SHOP", "create": false}, 200)
	if res["updated"].(float64) != 1 || res["created"].(float64) != 0 || res["relations_removed"].(float64) != 1 {
		t.Fatalf("re-import %v", res)
	}
	reqs = byTitle(items(a.m("GET", base+"/requirements", nil, 200)))
	if len(reqs) != 2 || reqs["Login with SSO"] == nil || reqs["Logout (edited)"] == nil {
		t.Fatalf("%v", reqs)
	}
	// without relations nothing is removed or added
	pv = a.m("POST", base+"/requirements/import/jira/preview", map[string]any{"query": "project = SHOP", "relations": false}, 200)
	if rels := pv["relations"].(map[string]any); rels["add"].(float64) != 0 || rels["remove"].(float64) != 0 {
		t.Fatal(rels)
	}
	res = a.m("POST", base+"/requirements/import/jira/apply", map[string]any{"query": "project = SHOP", "relations": false, "exclude_refs": []string{"SHOP-3"}}, 200)
	if res["relations_added"].(float64) != 0 {
		t.Fatal(res)
	}

	// a Jira connection of its own (Server: classic search, bearer token)
	jf.Classic = true
	a.m("PUT", base+"/import-sources", map[string]any{"jira": map[string]any{"url": ts.URL + "/", "token": trackerfake.JiraToken, "jql": "project = SHOP"}}, 200)
	src = a.m("GET", base+"/import-sources", nil, 200)
	if src["jira_from_tracker"] != false || src["jira"].(map[string]any)["has_token"] != true || src["jira"].(map[string]any)["token"] != nil {
		t.Fatal(src)
	}
	var raw string
	if err := e.st.DB.QueryRowContext(context.Background(), db.Rebind(e.st.DB.Dialect, `SELECT import_sources FROM projects WHERE id = ?`), pid).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, trackerfake.JiraToken) {
		t.Fatal(raw)
	}
	if pv = a.m("POST", base+"/requirements/import/jira/preview", map[string]any{"limit": 1}, 200); pv["truncated"] != true || len(pv["warnings"].([]any)) == 0 {
		t.Fatal(pv)
	}
	a.m("POST", base+"/requirements/import/jira/preview", map[string]any{"query": "syntax error"}, 502)
	a.m("POST", base+"/requirements/import/jira/preview", map[string]any{"limit": 5000}, 400)
	a.m("POST", base+"/requirements/import/jira/preview", map[string]any{"mapping": map[string]string{"Summary": "nope"}}, 400)
	a.m("POST", base+"/requirements/import/jira/preview", map[string]any{"folder_id": "nope"}, 400)
	// keep the token on save, clear it on request
	a.m("PUT", base+"/import-sources", map[string]any{"jira": map[string]any{"url": ts.URL}}, 200)
	if got := a.m("GET", base+"/import-sources", nil, 200); got["jira"].(map[string]any)["has_token"] != true {
		t.Fatal(got)
	}
	a.m("PUT", base+"/import-sources", map[string]any{"jira": map[string]any{"url": ts.URL}, "clear_tokens": map[string]any{"jira": true}}, 200)
	a.m("POST", base+"/import-sources/jira/check", nil, 502)

	// a user who may only read requirements cannot import
	e.member("viewer@example.com", "viewer")
	v := e.login("viewer@example.com")
	v.m("POST", base+"/requirements/import/jira/preview", map[string]any{}, 403)
}

func TestRequirementImportAzure(t *testing.T) {
	e := newEnv(t)
	af := trackerfake.NewAzure()
	af.Items[11] = map[string]any{"System.Title": "Login", "System.Description": "<div><b>Must</b> log in.</div>", "System.State": "Active",
		"System.Tags": "auth; web", "System.WorkItemType": "User Story", "Microsoft.VSTS.Common.Priority": 2.0,
		"Microsoft.VSTS.Common.AcceptanceCriteria": "<ul><li>Login works</li><li>[analysis] Audit trail</li></ul>"}
	af.Items[12] = map[string]any{"System.Title": "Logout", "System.WorkItemType": "User Story", "System.State": "Removed"}
	af.Items[13] = map[string]any{"System.Title": "A bug", "System.WorkItemType": "Bug"}
	af.Link(12, "System.LinkTypes.Hierarchy-Reverse", 11)
	af.Link(11, "System.LinkTypes.Related", 12)
	af.Link(12, "System.LinkTypes.Related", 11)
	ts := httptest.NewServer(af)
	defer ts.Close()
	e.srv.HTTPClient = ts.Client()
	a := e.login("admin@example.com")
	pid := a.project("AZI")
	base := "/api/v1/projects/" + pid
	a.m("POST", base+"/requirements/import/azure-devops/preview", map[string]any{}, 400)
	got := a.m("PUT", base+"/import-sources", map[string]any{"azure_devops": map[string]any{"url": ts.URL + "/org/", "project": "Shop", "token": trackerfake.AzureToken,
		"area_path": "Shop", "work_item_types": "User Story, Requirement"}}, 200)
	if az := got["azure_devops"].(map[string]any); az["has_token"] != true || az["token"] != nil || az["url"] != ts.URL+"/org" {
		t.Fatal(got)
	}
	a.m("POST", base+"/import-sources/azure-devops/check", nil, 200)
	pv := a.m("POST", base+"/requirements/import/azure-devops/preview", map[string]any{}, 200)
	if s := summaryOf(pv); s["new"].(float64) != 2 || !strings.Contains(pv["query"].(string), "UNDER 'Shop'") || pv["relations"].(map[string]any)["add"].(float64) != 2 {
		t.Fatalf("%v %v", s, pv)
	}
	res := a.m("POST", base+"/requirements/import/azure-devops/apply", map[string]any{}, 200)
	if res["created"].(float64) != 2 || res["relations_added"].(float64) != 2 {
		t.Fatal(res)
	}
	reqs := byTitle(items(a.m("GET", base+"/requirements", nil, 200)))
	login := reqs["Login"]
	if login["priority"] != "high" || login["status"] != "approved" || login["description"] != "**Must** log in." || reqs["Logout"]["status"] != "obsolete" ||
		login["source"] != "Azure DevOps 11" || len(login["tags"].([]any)) != 2 {
		t.Fatalf("%v", reqs)
	}
	full := a.m("GET", "/api/v1/requirements/"+login["id"].(string), nil, 200)
	if cs := full["criteria"].([]any); len(cs) != 2 || cs[1].(map[string]any)["method"] != "analysis" {
		t.Fatalf("criteria %v", full["criteria"])
	}
	// a WIQL query of its own: all types; re-import updates
	af.Items[11]["System.Title"] = "Login v2"
	pv = a.m("POST", base+"/requirements/import/azure-devops/preview", map[string]any{"query": "SELECT [System.Id] FROM WorkItems"}, 200)
	if s := summaryOf(pv); s["new"].(float64) != 1 || s["changed"].(float64) != 1 || s["unchanged"].(float64) != 1 {
		t.Fatalf("%v", s)
	}
	// the simple form with explicit values
	pv = a.m("POST", base+"/requirements/import/azure-devops/preview", map[string]any{"area_path": "Shop\\Web", "work_item_types": []string{"Bug"}}, 200)
	if s := summaryOf(pv); s["new"].(float64) != 1 || !strings.Contains(pv["query"].(string), "IN ('Bug')") {
		t.Fatalf("%v %v", s, pv["query"])
	}
	a.m("POST", base+"/requirements/import/azure-devops/preview", map[string]any{"query": "syntax error"}, 502)
	a.m("PUT", base+"/import-sources", map[string]any{"azure_devops": map[string]any{"url": ts.URL + "/org", "project": "Nope"}}, 200)
	a.m("POST", base+"/import-sources/azure-devops/check", nil, 502)
	// removing the URL removes the connection and its token
	a.m("PUT", base+"/import-sources", map[string]any{}, 200)
	if got := a.m("GET", base+"/import-sources", nil, 200); got["azure_devops"].(map[string]any)["has_token"] != false {
		t.Fatal(got)
	}
}

func TestDefectTrackerRT(t *testing.T) {
	e := newEnv(t)
	rf := trackerfake.NewRT()
	ts := httptest.NewServer(rf)
	defer ts.Close()
	e.srv.HTTPClient = ts.Client()
	a := e.login("admin@example.com")
	f := qualityFixture(t, a, "RTD")
	base := "/api/v1/projects/" + f.pid
	a.m("PUT", base+"/defect-tracker", map[string]any{"type": "rt", "url": "not a url", "queue": "QA"}, 400)
	a.m("PUT", base+"/defect-tracker", map[string]any{"type": "rt", "url": ts.URL + "/rt", "queue": ""}, 400)
	a.m("PUT", base+"/defect-tracker", map[string]any{"type": "rt", "url": ts.URL + "/rt", "queue": "a/b"}, 400)
	tr := a.m("PUT", base+"/defect-tracker", map[string]any{"type": "rt", "url": ts.URL + "/rt/REST/2.0/", "queue": "QA", "token": trackerfake.RTToken,
		"labels_field": "Tags", "milestone": "x", "project_key": "SHOP"}, 200)
	if tr["url"] != ts.URL+"/rt" || tr["queue"] != "QA" || tr["milestone"] != "" || tr["has_token"] != true || tr["token"] != nil {
		t.Fatal(tr)
	}
	a.m("POST", base+"/defect-tracker/check", nil, 200)

	d := a.m("POST", base+"/defects", map[string]any{"title": "Crash on login", "severity": "major", "links": []map[string]any{{"entity_type": "requirement", "entity_id": f.r1}}}, 201)
	pushed := a.m("POST", "/api/v1/defects/"+d["id"].(string)+"/push", nil, 200)
	if pushed["external_key"] != "#101" || pushed["external_system"] != "rt" || pushed["external_status"] != "new" {
		t.Fatal(pushed)
	}
	rf.SetStatus(101, "resolved")
	synced := a.m("POST", "/api/v1/defects/"+d["id"].(string)+"/sync", nil, 200)
	if synced["status"] != "closed" {
		t.Fatal(synced)
	}
	rf.SetStatus(101, "rejected")
	if res := a.m("POST", base+"/defects/sync", nil, 200); res["changed"].(float64) != 1 {
		t.Fatal(res)
	}
	// an existing ticket becomes a defect
	rf.Tickets[50] = map[string]any{"id": 50, "Subject": "Old ticket", "Status": "stalled"}
	imp := a.m("POST", base+"/defects/import", map[string]any{"external_key": "rt#50"}, 201)
	if imp["external_key"] != "#50" || imp["status"] != "in_progress" || imp["title"] != "Old ticket" {
		t.Fatal(imp)
	}
	a.m("POST", base+"/defects/import", map[string]any{"external_key": "50"}, 409)
	a.m("POST", base+"/defects/import", map[string]any{"external_key": "SHOP-1"}, 400)

	// an issue from a requirement: create, update (a comment) and sync
	l := a.m("POST", "/api/v1/requirements/"+f.r1+"/issue", nil, 201)["link"].(map[string]any)
	issue := l["issue"].(map[string]any)
	if issue["key"] != "#102" || issue["tracker"] != "rt" || !strings.HasPrefix(l["label"].(string), "RT #102") {
		t.Fatal(l)
	}
	if c := rf.Tickets[102]["Content"].(string); strings.Contains(c, "qtestframe:begin") || !strings.Contains(c, "The user MUST log in.") {
		t.Fatal(c)
	}
	a.m("PUT", "/api/v1/requirements/"+f.r1+"/issue", nil, 200)
	if len(rf.Comments[102]) != 1 {
		t.Fatal(rf.Comments)
	}
	a.m("POST", "/api/v1/requirements/"+f.r1+"/issue/sync", nil, 200)
}
