package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/testutil/trackerfake"
)

func syncConfig(t *testing.T, list map[string]any, system string) map[string]any {
	t.Helper()
	for _, x := range items(list) {
		if x["system"] == system {
			return x
		}
	}
	t.Fatalf("no %s config in %v", system, list)
	return nil
}

func summaryOfRun(run map[string]any) map[string]float64 {
	out := map[string]float64{}
	for k, v := range run["summary"].(map[string]any) {
		out[k] = v.(float64)
	}
	return out
}

func entriesOf(run map[string]any, action string) []map[string]any {
	var out []map[string]any
	for _, x := range run["entries"].([]any) {
		m := x.(map[string]any)
		if action == "" || m["action"] == action {
			out = append(out, m)
		}
	}
	return out
}

func (c *client) hook(path string, body []byte, header map[string]string, want int) map[string]any {
	c.e.t.Helper()
	req, _ := http.NewRequest("POST", c.e.hs.URL+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer res.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode != want {
		c.e.t.Fatalf("hook %s: %d, want %d: %v", path, res.StatusCode, want, out)
	}
	return out
}

func noSyncSleep(t *testing.T) *int {
	n := 0
	old := syncSleep
	syncSleep = func(ctx context.Context, d time.Duration) error { n++; return nil }
	t.Cleanup(func() { syncSleep = old })
	return &n
}

func TestReqSyncJira(t *testing.T) {
	noSyncSleep(t)
	e := newEnv(t)
	jf := trackerfake.NewJira()
	jf.Issues["SHOP-1"] = map[string]any{"summary": "Login", "description": "The user *must* log in.",
		"priority": map[string]any{"name": "High"}, "labels": []any{"auth"}, "status": map[string]any{"name": "In Progress", "statusCategory": map[string]any{"key": "indeterminate"}}}
	jf.Issues["SHOP-2"] = map[string]any{"summary": "Logout", "status": map[string]any{"name": "To Do", "statusCategory": map[string]any{"key": "new"}},
		"issuelinks": []any{map[string]any{"id": "900", "type": map[string]any{"name": "Blocks", "outward": "blocks"}, "outwardIssue": map[string]any{"key": "SHOP-1"}}}}
	jf.Touch("SHOP-1")
	jf.Touch("SHOP-2")
	ts := httptest.NewServer(jf)
	defer ts.Close()
	e.srv.HTTPClient = ts.Client()
	a := e.login("admin@example.com")
	pid := a.project("SYN")
	base := "/api/v1/projects/" + pid
	folder := a.m("POST", base+"/folders", map[string]any{"name": "From Jira", "kind": "requirement"}, 201)["id"].(string)

	// defaults before anything is saved; not connected yet
	list := a.m("GET", base+"/req-sync", nil, 200)
	jc := syncConfig(t, list, "jira")
	if jc["enabled"] != false || jc["connected"] != false || !strings.HasSuffix(jc["webhook_url"].(string), "/api/v1/hooks/req-sync/jira/"+pid) ||
		len(jc["settings"].(map[string]any)["fields"].([]any)) != 5 || jc["settings"].(map[string]any)["conflict_policy"] != "manual" {
		t.Fatal(jc)
	}
	a.m("POST", base+"/req-sync/jira/run", nil, 400) // not configured
	a.m("PUT", base+"/req-sync/nope", map[string]any{}, 400)
	a.m("PUT", base+"/req-sync/jira", map[string]any{"interval_minutes": 2}, 400)
	a.m("PUT", base+"/req-sync/jira", map[string]any{"settings": map[string]any{"conflict_policy": "coin"}}, 400)
	a.m("PUT", base+"/req-sync/jira", map[string]any{"settings": map[string]any{"push": map[string]any{"create": true}}}, 400)
	a.m("PUT", base+"/req-sync/jira", map[string]any{"settings": map[string]any{"fields": []any{map[string]any{"field": "title", "remote": "summary", "direction": "sideways"}}}}, 400)
	a.m("PUT", base+"/req-sync/jira", map[string]any{"settings": map[string]any{"fields": []any{map[string]any{"field": "status", "remote": "status", "values": map[string]string{"Done": "finished"}}}}}, 400)
	a.m("PUT", base+"/req-sync/jira", map[string]any{"webhook_secret": "short"}, 400)
	a.m("PUT", base+"/req-sync/jira", map[string]any{"settings": map[string]any{"folder_id": "nope"}}, 400)

	a.m("PUT", base+"/import-sources", map[string]any{"jira": map[string]any{"url": ts.URL, "user": trackerfake.JiraUser, "token": trackerfake.JiraToken, "jql": "project = SHOP ORDER BY key"}}, 200)
	secret := "a-long-shared-webhook-secret"
	settings := map[string]any{"import_new": true, "folder_id": folder, "comments": true, "links": true, "max_writes_per_minute": 30,
		"push": map[string]any{"create": true, "tag": "push-me", "project_key": "SHOP"}}
	cfg := a.m("PUT", base+"/req-sync/jira", map[string]any{"enabled": true, "interval_minutes": 15, "settings": settings, "webhook_secret": secret}, 200)
	if cfg["has_webhook_secret"] != true || cfg["webhook_secret"] != nil || cfg["run_as"] != "admin@example.com" || cfg["next_run_at"] == nil || cfg["connected"] != true ||
		len(cfg["settings"].(map[string]any)["fields"].([]any)) != 5 || cfg["settings"].(map[string]any)["comments_since"] == nil {
		t.Fatal(cfg)
	}
	// the stored secret is encrypted
	var raw string
	if err := e.st.DB.QueryRowContext(context.Background(), e.st.DB.Rebind(`SELECT webhook_secret FROM req_sync_configs WHERE project_id = ?`), pid).Scan(&raw); err != nil || strings.Contains(raw, secret) {
		t.Fatalf("stored secret %q %v", raw, err)
	}
	// a stale version is refused
	a.m("PUT", base+"/req-sync/jira", map[string]any{"enabled": true, "interval_minutes": 15, "settings": settings, "version": 99}, 409)

	// preview: nothing is written
	pv := a.m("POST", base+"/req-sync/jira/run", map[string]any{"dry_run": true}, 200)
	if pv["dry_run"] != true || len(entriesOf(pv, "create_local")) != 2 || len(items(a.m("GET", base+"/requirements", nil, 200))) != 0 {
		t.Fatalf("dry run %v", pv)
	}
	if runs := items(a.m("GET", base+"/req-sync/runs", nil, 200)); len(runs) != 0 {
		t.Fatalf("a dry run is not logged: %v", runs)
	}

	// first sync: both issues become requirements, the link a relation
	run := a.m("POST", base+"/req-sync/jira/run", map[string]any{}, 200)
	if s := summaryOfRun(run); s["created_local"] != 2 || s["links"] != 1 || s["errors"] != 0 || run["status"] != "ok" {
		t.Fatalf("first sync %v", run)
	}
	reqs := byTitle(items(a.m("GET", base+"/requirements", nil, 200)))
	login, logout := reqs["Login"], reqs["Logout"]
	if login == nil || logout == nil || login["status"] != "approved" || login["priority"] != "high" || login["folder_path"] != "From Jira" ||
		login["source"] != "Jira SHOP-1" || !strings.Contains(login["description"].(string), "**must**") || logout["status"] != "draft" {
		t.Fatalf("%v\n%v", login, logout)
	}
	loginID, logoutID := login["id"].(string), logout["id"].(string)
	rels := items(a.m("GET", base+"/relations", nil, 200))
	if len(rels) != 1 || rels[0]["type"] != "depends_on" || rels[0]["source_id"] != loginID {
		t.Fatalf("relations %v", rels)
	}
	// nothing changed: nothing to do
	run = a.m("POST", base+"/req-sync/jira/run", nil, 200)
	if s := summaryOfRun(run); s["pulled"]+s["pushed"]+s["created_local"]+s["created_remote"]+s["conflicts"]+s["links"] != 0 || s["unchanged"] != 2 {
		t.Fatalf("second sync %v", run)
	}

	// an edit here goes to Jira: the title, the status (a transition) and the tags
	writes := jf.Writes
	a.m("PATCH", "/api/v1/requirements/"+loginID, map[string]any{"title": "Login with SSO", "status": "implemented", "tags": []string{"auth", "sso"}}, 200)
	run = a.m("POST", base+"/req-sync/jira/run", nil, 200)
	push := entriesOf(run, "push")
	if s := summaryOfRun(run); s["pushed"] != 1 || len(push) != 1 || push[0]["requirement_id"] != loginID {
		t.Fatalf("push %v", run)
	}
	if jf.Field("SHOP-1", "summary") != "Login with SSO" || jf.Field("SHOP-1", "status").(map[string]any)["name"] != "Done" || jf.Writes <= writes {
		t.Fatalf("jira %v", jf.Issues["SHOP-1"])
	}
	if labels := jf.Field("SHOP-1", "labels"); labels == nil || len(labels.([]any)) != 2 {
		t.Fatalf("labels %v", labels)
	}
	// and the pushed state is the new base: no flip-flop ("Done" reads back as implemented)
	run = a.m("POST", base+"/req-sync/jira/run", nil, 200)
	if s := summaryOfRun(run); s["pushed"]+s["pulled"]+s["conflicts"] != 0 {
		t.Fatalf("after push %v", run)
	}

	// an edit in Jira comes here through the regular update (history, audit)
	jf.Set("SHOP-2", "summary", "Logout everywhere")
	jf.Set("SHOP-2", "priority", map[string]any{"name": "Highest"})
	run = a.m("POST", base+"/req-sync/jira/run", nil, 200)
	if s := summaryOfRun(run); s["pulled"] != 1 {
		t.Fatalf("pull %v", run)
	}
	got := a.m("GET", "/api/v1/requirements/"+logoutID, nil, 200)
	if got["title"] != "Logout everywhere" || got["priority"] != "critical" {
		t.Fatal(got)
	}
	audits := map[string]bool{}
	for _, x := range items(a.m("GET", "/api/v1/audit?limit=200", nil, 200)) {
		audits[x["action"].(string)] = true
	}
	for _, act := range []string{"requirement.update", "req_sync.run", "req_sync.update", "requirement.sync.push"} {
		if !audits[act] {
			t.Fatalf("no %s audit entry: %v", act, audits)
		}
	}

	// both sides changed the title: the manual policy records a conflict
	a.m("PATCH", "/api/v1/requirements/"+logoutID, map[string]any{"title": "Log out (here)", "rationale": "x"}, 200)
	jf.Set("SHOP-2", "summary", "Log out (Jira)")
	run = a.m("POST", base+"/req-sync/jira/run", nil, 200)
	if s := summaryOfRun(run); s["conflicts"] != 1 {
		t.Fatalf("conflict %v", run)
	}
	conflicts := items(a.m("GET", base+"/req-sync/conflicts", nil, 200))
	if len(conflicts) != 1 {
		t.Fatal(conflicts)
	}
	c := conflicts[0]
	diff := c["fields"].([]any)[0].(map[string]any)
	if c["requirement_id"] != logoutID || c["external_id"] != "SHOP-2" || c["external_url"] != ts.URL+"/browse/SHOP-2" ||
		diff["field"] != "title" || diff["base"] != "Logout everywhere" || diff["local"] != "Log out (here)" || diff["remote"] != "Log out (Jira)" {
		t.Fatalf("conflict %v", c)
	}
	// a later sync keeps the one conflict and does not overwrite either side
	a.m("POST", base+"/req-sync/jira/run", nil, 200)
	if n := len(items(a.m("GET", base+"/req-sync/conflicts", nil, 200))); n != 1 || a.m("GET", "/api/v1/requirements/"+logoutID, nil, 200)["title"] != "Log out (here)" {
		t.Fatal("conflict kept", n)
	}
	list = a.m("GET", base+"/req-sync", nil, 200)
	if jc := syncConfig(t, list, "jira"); jc["open_conflicts"].(float64) != 1 || jc["linked"].(float64) != 2 || jc["last_status"] != "ok" {
		t.Fatal(jc)
	}
	a.m("POST", base+"/req-sync/conflicts/"+c["id"].(string)+"/resolve", map[string]any{"resolution": "maybe"}, 400)
	// the tracker's title wins
	res := a.m("POST", base+"/req-sync/conflicts/"+c["id"].(string)+"/resolve", map[string]any{"resolution": "tracker"}, 200)
	if res["conflict"].(map[string]any)["status"] != "resolved" || a.m("GET", "/api/v1/requirements/"+logoutID, nil, 200)["title"] != "Log out (Jira)" {
		t.Fatalf("resolve %v", res)
	}
	a.m("POST", base+"/req-sync/conflicts/"+c["id"].(string)+"/resolve", map[string]any{"resolution": "tracker"}, 409)
	if n := len(items(a.m("GET", base+"/req-sync/conflicts?status=resolved", nil, 200))); n != 1 {
		t.Fatal("resolved", n)
	}
	// a second conflict, decided for Qtestframe
	a.m("PATCH", "/api/v1/requirements/"+logoutID, map[string]any{"title": "Sign out"}, 200)
	jf.Set("SHOP-2", "summary", "Log off")
	a.m("POST", base+"/req-sync/jira/run", nil, 200)
	c = items(a.m("GET", base+"/req-sync/conflicts", nil, 200))[0]
	a.m("POST", base+"/req-sync/conflicts/"+c["id"].(string)+"/resolve", map[string]any{"fields": map[string]string{"title": "qtestframe"}}, 200)
	if jf.Field("SHOP-2", "summary") != "Sign out" {
		t.Fatal(jf.Field("SHOP-2", "summary"))
	}

	// a requirement made here with the push tag gets a Jira issue
	nr := a.m("POST", base+"/requirements", map[string]any{"title": "Password reset", "description": "Users **can** reset.", "tags": []string{"push-me"}, "priority": "low"}, 201)
	a.m("POST", base+"/requirements", map[string]any{"title": "Not pushed"}, 201)
	run = a.m("POST", base+"/req-sync/jira/run", nil, 200)
	created := entriesOf(run, "create_remote")
	if len(created) != 1 || created[0]["external_id"] != "SHOP-3" {
		t.Fatalf("create remote %v", run)
	}
	if jf.Field("SHOP-3", "summary") != "Password reset" || jf.Field("SHOP-3", "description") != "Users *can* reset." || jf.Field("SHOP-3", "priority").(map[string]any)["name"] != "Low" {
		t.Fatalf("SHOP-3 %v", jf.Issues["SHOP-3"])
	}
	run = a.m("POST", base+"/req-sync/jira/run", nil, 200)
	if s := summaryOfRun(run); s["created_remote"]+s["pushed"]+s["pulled"] != 0 {
		t.Fatalf("after create %v", run)
	}
	_ = nr

	// comments both ways
	jf.AddComment("SHOP-1", "Jira Person", "Please also support *passkeys*.")
	a.m("POST", "/api/v1/requirements/"+logoutID+"/comments", map[string]any{"body": "Checked with **security**."}, 201)
	run = a.m("POST", base+"/req-sync/jira/run", nil, 200)
	if len(entriesOf(run, "comment_pull")) != 1 || len(entriesOf(run, "comment_push")) != 1 {
		t.Fatalf("comments %v", run)
	}
	var found bool
	for _, cm := range items(a.m("GET", "/api/v1/requirements/"+loginID+"/comments", nil, 200)) {
		found = found || strings.Contains(cm["body"].(string), "**Jira Person** (Jira SHOP-1)") && strings.Contains(cm["body"].(string), "**passkeys**")
	}
	if !found || len(jf.Comments["SHOP-2"]) != 1 || !strings.Contains(jf.Comments["SHOP-2"][0]["body"].(string), "*security*") {
		t.Fatalf("comments here %v, in Jira %v", found, jf.Comments)
	}
	run = a.m("POST", base+"/req-sync/jira/run", nil, 200)
	if s := summaryOfRun(run); s["comments"] != 0 {
		t.Fatalf("comments are synced once %v", run)
	}

	// a relation made here becomes a Jira link
	a.m("POST", "/api/v1/requirements/"+nr["id"].(string)+"/relations", map[string]any{"target_id": loginID, "type": "relates_to"}, 201)
	run = a.m("POST", base+"/req-sync/jira/run", nil, 200)
	if len(entriesOf(run, "link_push")) != 1 {
		t.Fatalf("link push %v", run)
	}
	if links, _ := jf.Field("SHOP-3", "issuelinks").([]any); len(links) != 1 {
		t.Fatalf("links %v", jf.Issues["SHOP-3"])
	}
	run = a.m("POST", base+"/req-sync/jira/run", nil, 200)
	if s := summaryOfRun(run); s["links"] != 0 {
		t.Fatalf("links are synced once %v", run)
	}

	// webhooks: a wrong secret, the signature of Jira Cloud, the query secret of Jira Server
	hookPath := "/api/v1/hooks/req-sync/jira/" + pid
	ev, _ := json.Marshal(map[string]any{"webhookEvent": "jira:issue_updated", "issue": map[string]any{"key": "SHOP-1"}})
	a.hook(hookPath, ev, map[string]string{"X-Hub-Signature": "sha256=00"}, 401)
	a.hook(hookPath+"?secret=wrong", ev, nil, 401)
	a.hook("/api/v1/hooks/req-sync/jira/nope", ev, map[string]string{"X-Qtestframe-Secret": secret}, 401)
	jf.Set("SHOP-1", "summary", "Login with passkeys")
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(ev)
	out := a.hook(hookPath, ev, map[string]string{"X-Hub-Signature": "sha256=" + hex.EncodeToString(m.Sum(nil))}, 202)
	e.srv.WaitWebhooks()
	if out["event"] != "jira:issue_updated" || a.m("GET", "/api/v1/requirements/"+loginID, nil, 200)["title"] != "Login with passkeys" {
		t.Fatalf("webhook %v", out)
	}
	jf.Set("SHOP-1", "summary", "Login (server hook)")
	a.hook(hookPath+"?secret="+secret, ev, nil, 202)
	e.srv.WaitWebhooks()
	if a.m("GET", "/api/v1/requirements/"+loginID, nil, 200)["title"] != "Login (server hook)" {
		t.Fatal("server webhook")
	}
	runs := items(a.m("GET", base+"/req-sync/runs?source=jira", nil, 200))
	if runs[0]["trigger"] != "webhook" || runs[0]["entries"] != nil {
		t.Fatalf("log %v", runs[0])
	}
	one := a.m("GET", base+"/req-sync/runs/"+runs[0]["id"].(string), nil, 200)
	if len(entriesOf(one, "pull")) != 1 {
		t.Fatalf("run %v", one)
	}
	a.m("GET", base+"/req-sync/runs/nope", nil, 404)
	a.m("GET", base+"/req-sync/runs?source=x", nil, 400)
	a.m("GET", base+"/req-sync/conflicts?status=x", nil, 400)

	// a viewer may look but not sync
	e.member("viewer@example.com", "viewer")
	v := e.login("viewer@example.com")
	v.m("GET", base+"/req-sync", nil, 200)
	v.m("POST", base+"/req-sync/jira/run", nil, 403)
	v.m("PUT", base+"/req-sync/jira", map[string]any{}, 403)
}

func TestReqSyncAzure(t *testing.T) {
	noSyncSleep(t)
	e := newEnv(t)
	af := trackerfake.NewAzure()
	af.Items[11] = map[string]any{"System.Title": "Checkout", "System.WorkItemType": "User Story", "System.State": "Active",
		"System.Description": "<p>Pay with <b>card</b></p>", "Microsoft.VSTS.Common.Priority": 2, "System.Tags": "shop; web",
		"Microsoft.VSTS.Common.AcceptanceCriteria": "<ul><li>Card accepted</li><li>Receipt sent</li></ul>"}
	af.Items[12] = map[string]any{"System.Title": "Refund", "System.WorkItemType": "User Story", "System.State": "Closed"}
	af.Link(12, "System.LinkTypes.Related", 11)
	ts := httptest.NewServer(http.StripPrefix("", af))
	defer ts.Close()
	e.srv.HTTPClient = ts.Client()
	a := e.login("admin@example.com")
	pid := a.project("AZS")
	base := "/api/v1/projects/" + pid
	pushFolder := a.m("POST", base+"/folders", map[string]any{"name": "To Azure", "kind": "requirement"}, 201)["id"].(string)
	a.m("PUT", base+"/import-sources", map[string]any{"azure_devops": map[string]any{"url": ts.URL + "/org", "project": "Shop", "token": trackerfake.AzureToken, "work_item_types": "User Story"}}, 200)
	secret := "azure-service-hook-secret-1"
	a.m("PUT", base+"/req-sync/azure-devops", map[string]any{"enabled": true, "webhook_secret": secret, "settings": map[string]any{
		"import_new": true, "conflict_policy": "tracker", "impact": "skip", "links": true,
		"push": map[string]any{"create": true, "folder_id": pushFolder, "item_type": "Requirement"}}}, 200)
	run := a.m("POST", base+"/req-sync/azure-devops/run", nil, 200)
	if s := summaryOfRun(run); s["created_local"] != 2 || s["links"] != 1 || s["errors"] != 0 {
		t.Fatalf("first %v", run)
	}
	reqs := byTitle(items(a.m("GET", base+"/requirements", nil, 200)))
	co := reqs["Checkout"]
	if co == nil || co["status"] != "approved" || co["priority"] != "high" || reqs["Refund"]["status"] != "verified" || len(co["tags"].([]any)) != 2 {
		t.Fatalf("%v", reqs)
	}
	coID := co["id"].(string)
	full := a.m("GET", "/api/v1/requirements/"+coID, nil, 200)
	if cs := full["criteria"].([]any); len(cs) != 2 || cs[1].(map[string]any)["description"] != "Receipt sent" {
		t.Fatalf("criteria %v", full["criteria"])
	}

	// both changed the title: the tracker wins; the status pushed carries the revision test
	a.m("PATCH", "/api/v1/requirements/"+coID, map[string]any{"title": "Checkout (here)", "status": "implemented"}, 200)
	af.Set(11, "System.Title", "Checkout (Azure)")
	run = a.m("POST", base+"/req-sync/azure-devops/run", nil, 200)
	if s := summaryOfRun(run); s["pulled"] != 1 || s["pushed"] != 1 || s["conflicts"] != 0 {
		t.Fatalf("tracker wins %v", run)
	}
	if got := a.m("GET", "/api/v1/requirements/"+coID, nil, 200); got["title"] != "Checkout (Azure)" || got["status"] != "implemented" {
		t.Fatal(got)
	}
	if af.Field(11, "System.State") != "Resolved" {
		t.Fatal(af.Items[11])
	}
	last := af.Patches[len(af.Patches)-1]
	if last[0]["op"] != "test" || last[0]["path"] != "/rev" {
		t.Fatalf("patch %v", last)
	}
	run = a.m("POST", base+"/req-sync/azure-devops/run", nil, 200)
	if s := summaryOfRun(run); s["pulled"]+s["pushed"] != 0 {
		t.Fatalf("stable %v", run)
	}

	// a requirement in the push folder becomes a work item of the configured type
	nr := a.m("POST", base+"/requirements", map[string]any{"title": "Gift cards", "folder_id": pushFolder, "description": "- one\n- two", "status": "approved"}, 201)
	run = a.m("POST", base+"/req-sync/azure-devops/run", nil, 200)
	cr := entriesOf(run, "create_remote")
	if len(cr) != 1 {
		t.Fatalf("create %v", run)
	}
	id, _ := strconv.Atoi(cr[0]["external_id"].(string))
	if af.Field(id, "System.WorkItemType") != "Requirement" || af.Field(id, "System.Title") != "Gift cards" || af.Field(id, "System.State") != "Active" ||
		!strings.Contains(af.Field(id, "System.Description").(string), "<li>one</li>") {
		t.Fatalf("work item %v", af.Items[id])
	}
	// a relation becomes a work item link
	a.m("POST", "/api/v1/requirements/"+nr["id"].(string)+"/relations", map[string]any{"target_id": coID, "type": "depends_on"}, 201)
	run = a.m("POST", base+"/req-sync/azure-devops/run", nil, 200)
	if len(entriesOf(run, "link_push")) != 1 || len(af.Relations[id]) != 1 || af.Relations[id][0]["rel"] != "System.LinkTypes.Dependency-Reverse" {
		t.Fatalf("link %v %v", run, af.Relations[id])
	}

	// a service hook with basic authentication
	hookPath := "/api/v1/hooks/req-sync/azure-devops/" + pid
	ev, _ := json.Marshal(map[string]any{"eventType": "workitem.updated", "resource": map[string]any{"id": 5, "workItemId": 12, "rev": 3}})
	req := func(pw string, want int) map[string]any {
		r, _ := http.NewRequest("POST", e.hs.URL+hookPath, bytes.NewReader(ev))
		r.SetBasicAuth("azure", pw)
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		out := map[string]any{}
		_ = json.NewDecoder(res.Body).Decode(&out)
		if res.StatusCode != want {
			t.Fatalf("hook %d %v", res.StatusCode, out)
		}
		return out
	}
	req("wrong", 401)
	af.Set(12, "System.State", "Removed")
	if out := req(secret, 202); out["items"].([]any)[0] != "12" {
		t.Fatal(out)
	}
	e.srv.WaitWebhooks()
	if got := a.m("GET", "/api/v1/requirements/"+reqs["Refund"]["id"].(string), nil, 200); got["status"] != "obsolete" {
		t.Fatal(got)
	}
	// an event without a work item is ignored
	ign, _ := json.Marshal(map[string]any{"eventType": "git.push", "resource": map[string]any{}})
	a.hook(hookPath, ign, map[string]string{"X-Qtestframe-Secret": secret}, 202)

	// the tracker changed the work item between reading and writing: the
	// revision test fails, the push waits for the next sync
	a.m("PATCH", "/api/v1/requirements/"+coID, map[string]any{"title": "Checkout v2"}, 200)
	plain := ts.Config.Handler
	ts.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			af.Set(11, "System.Rev", 1) // someone saved meanwhile
		}
		plain.ServeHTTP(w, r)
	})
	run = a.m("POST", base+"/req-sync/azure-devops/run", nil, 200)
	ts.Config.Handler = plain
	if errs := entriesOf(run, "error"); len(errs) != 1 || !strings.Contains(errs[0]["message"].(string), "changed in the tracker meanwhile") || run["status"] != "partial" {
		t.Fatalf("stale push %v", run)
	}
	run = a.m("POST", base+"/req-sync/azure-devops/run", nil, 200)
	if s := summaryOfRun(run); s["pushed"] != 1 || af.Field(11, "System.Title") != "Checkout v2" {
		t.Fatalf("retried %v", run)
	}
}

func TestReqSyncScheduleAndBackoff(t *testing.T) {
	noSyncSleep(t)
	e := newEnv(t)
	jf := trackerfake.NewJira()
	jf.Issues["SHOP-1"] = map[string]any{"summary": "Login"}
	limited := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if limited {
			w.Header().Set("Retry-After", "3600")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		jf.ServeHTTP(w, r)
	}))
	defer ts.Close()
	e.srv.HTTPClient = ts.Client()
	now := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	e.srv.Clock = func() time.Time { return now }
	a := e.login("admin@example.com")
	pid := a.project("SCH")
	base := "/api/v1/projects/" + pid
	a.m("PUT", base+"/import-sources", map[string]any{"jira": map[string]any{"url": ts.URL, "user": trackerfake.JiraUser, "token": trackerfake.JiraToken, "jql": "project = SHOP"}}, 200)
	a.m("PUT", base+"/req-sync/jira", map[string]any{"enabled": true, "interval_minutes": 30, "settings": map[string]any{"import_new": true}}, 200)
	ctx := context.Background()
	if n, err := e.srv.RunReqSyncJobs(ctx); err != nil || n != 0 {
		t.Fatal("not due yet", n, err)
	}
	now = now.Add(31 * time.Minute)
	if n, err := e.srv.RunReqSyncJobs(ctx); err != nil || n != 1 {
		t.Fatal("due", n, err)
	}
	if len(items(a.m("GET", base+"/requirements", nil, 200))) != 1 {
		t.Fatal("scheduled sync imported nothing")
	}
	cfg := syncConfig(t, a.m("GET", base+"/req-sync", nil, 200), "jira")
	if cfg["last_status"] != "ok" || !strings.HasPrefix(cfg["next_run_at"].(string), "2026-03-01T10:01") {
		t.Fatal(cfg)
	}
	// rate limited: the run fails and the next one waits for the tracker
	limited = true
	now = now.Add(31 * time.Minute)
	if _, err := e.srv.RunReqSyncJobs(ctx); err != nil {
		t.Fatal(err)
	}
	cfg = syncConfig(t, a.m("GET", base+"/req-sync", nil, 200), "jira")
	if cfg["last_status"] != "error" || cfg["failures"].(float64) != 1 || !strings.Contains(cfg["last_error"].(string), "429") || !strings.HasPrefix(cfg["next_run_at"].(string), "2026-03-01T11:02") {
		t.Fatal(cfg)
	}
	a.m("POST", base+"/req-sync/jira/run", nil, 502)
	limited = false
	run := a.m("POST", base+"/req-sync/jira/run", nil, 200)
	if run["status"] != "ok" {
		t.Fatal(run)
	}
	// the account the sync runs as lost its permission
	u := e.member("lead@example.com", "viewer")
	x, err := e.st.ReqSync(ctx, e.tenant.ID, pid, "jira")
	if err != nil {
		t.Fatal(err)
	}
	x.RunAs, x.Version = u.ID, 0
	if err := e.st.SaveReqSync(ctx, x, nil); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	if n, _ := e.srv.RunReqSyncJobs(ctx); n != 0 {
		t.Fatal("ran without permission")
	}
	if cfg = syncConfig(t, a.m("GET", base+"/req-sync", nil, 200), "jira"); !strings.Contains(cfg["last_error"].(string), "requirement.write") {
		t.Fatal(cfg)
	}
	// the run-as account is removed: recorded as failed
	x, _ = e.st.ReqSync(ctx, e.tenant.ID, pid, "jira")
	x.RunAs, x.Version = "", 0
	_ = e.st.SaveReqSync(ctx, x, nil)
	now = now.Add(24 * time.Hour)
	_, _ = e.srv.RunReqSyncJobs(ctx)
	if cfg = syncConfig(t, a.m("GET", base+"/req-sync", nil, 200), "jira"); !strings.Contains(cfg["last_error"].(string), "no account") {
		t.Fatal(cfg)
	}
	// the clear flag removes the webhook secret; disabled webhooks are ignored
	a.m("PUT", base+"/req-sync/jira", map[string]any{"enabled": false, "webhook_secret": "0123456789abcdef0123"}, 200)
	ev := []byte(`{"webhookEvent":"jira:issue_updated","issue":{"key":"SHOP-1"}}`)
	if out := a.hook("/api/v1/hooks/req-sync/jira/"+pid+"?secret=0123456789abcdef0123", ev, nil, 202); out["ignored"] != true {
		t.Fatal(out)
	}
	if c := a.m("PUT", base+"/req-sync/jira", map[string]any{"clear_webhook_secret": true}, 200); c["has_webhook_secret"] != false {
		t.Fatal(c)
	}
	a.hook("/api/v1/hooks/req-sync/jira/"+pid+"?secret=0123456789abcdef0123", ev, nil, 401)
}

func TestReqSyncPacer(t *testing.T) {
	sleeps := noSyncSleep(t)
	p := &syncPacer{gap: time.Hour}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := p.wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if *sleeps != 2 {
		t.Fatal("sleeps", *sleeps)
	}
	p.writes = maxSyncWrites
	if err := p.wait(ctx); err != errWriteBudget {
		t.Fatal(err)
	}
}

func TestReqSyncStore(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.login("admin@example.com")
	pid := a.project("STO")
	rq := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "R"}, 201)
	// an import between two syncs keeps the sync state of the reference
	ref := &store.ExternalRef{ProjectID: pid, System: "jira", EntityType: model.KindRequirement, EntityID: rq["id"].(string), ExternalID: "SHOP-9",
		Sync: &store.SyncState{Local: map[string]string{"title": "R"}, Remote: map[string]string{"title": "R"}}, RemoteVersion: "v1"}
	if err := e.st.SaveExternalRef(ctx, e.tenant.ID, ref); err != nil {
		t.Fatal(err)
	}
	if err := e.st.SaveExternalRef(ctx, e.tenant.ID, &store.ExternalRef{ProjectID: pid, System: "jira", EntityType: model.KindRequirement, EntityID: rq["id"].(string), ExternalID: "SHOP-9", Version: 3}); err != nil {
		t.Fatal(err)
	}
	refs, _ := e.st.ListExternalRefs(ctx, e.tenant.ID, pid, "jira")
	if len(refs) != 1 || refs[0].Sync == nil || refs[0].Sync.Local["title"] != "R" || refs[0].RemoteVersion != "v1" || refs[0].Version != 3 {
		t.Fatalf("%+v", refs)
	}
	// the log keeps the newest runs
	for i := 0; i < 105; i++ {
		run := &model.ReqSyncRun{ProjectID: pid, System: "jira", Trigger: "manual", Status: "ok", StartedAt: time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC)}
		if err := e.st.SaveReqSyncRun(ctx, e.tenant.ID, run); err != nil {
			t.Fatal(err)
		}
	}
	runs, err := e.st.ListReqSyncRuns(ctx, e.tenant.ID, pid, "jira", 0, false)
	if err != nil || len(runs) != 100 || runs[0].StartedAt.Second() != 44 {
		t.Fatal(len(runs), err)
	}
	// conflicts: one open per requirement
	for i := 0; i < 2; i++ {
		c := &model.ReqSyncConflict{ProjectID: pid, System: "jira", RequirementID: rq["id"].(string), ExternalID: "SHOP-9", Fields: []model.ReqSyncFieldDiff{{Field: "title", Local: "a", Remote: strconv.Itoa(i)}}}
		if err := e.st.SaveReqSyncConflict(ctx, e.tenant.ID, c); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := e.st.ListReqSyncConflicts(ctx, e.tenant.ID, pid, "", "open")
	if len(list) != 1 || list[0].Fields[0].Remote != "1" || list[0].RequirementKey == "" {
		t.Fatalf("%+v", list)
	}
	if n, _ := e.st.CloseReqSyncConflicts(ctx, e.tenant.ID, pid, "jira", rq["id"].(string), "dismissed", "x", "me"); n != 1 {
		t.Fatal(n)
	}
	// claims
	x := &store.ReqSyncRow{TenantID: e.tenant.ID, ProjectID: pid, System: "jira", Settings: defaultSyncSettings("jira")}
	if err := e.st.SaveReqSync(ctx, x, nil); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if ok, _ := e.st.ClaimReqSync(ctx, x.ID, now, now.Add(time.Minute)); !ok {
		t.Fatal("claim")
	}
	if ok, _ := e.st.ClaimReqSync(ctx, x.ID, now, now.Add(time.Minute)); ok {
		t.Fatal("claimed twice")
	}
	if err := e.st.ReleaseReqSync(ctx, x.ID); err != nil {
		t.Fatal(err)
	}
	if ok, _ := e.st.ClaimReqSync(ctx, x.ID, now, now.Add(time.Minute)); !ok {
		t.Fatal("claim after release")
	}
}
