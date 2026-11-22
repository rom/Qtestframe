package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

// sharedFixture is a restricted project with one of every resource.
type sharedFixture struct {
	pid   string
	ids   map[string]string // resource (as in resourceProjects) → id
	open  string            // an organisation-wide project
	reqID string
	tests []string
}

func newSharedFixture(t *testing.T, e *env) *sharedFixture {
	t.Helper()
	ctx := context.Background()
	a := e.login("admin@example.com")
	pid := a.project("SECRET")
	open := a.project("OPEN")
	fx := &sharedFixture{pid: pid, open: open, ids: map[string]string{}}
	pp := "/api/v1/projects/" + pid
	r1 := a.m("POST", pp+"/requirements", map[string]any{"title": "Secret requirement", "description": "classified"}, 201)["id"].(string)
	r2 := a.m("POST", pp+"/requirements", map[string]any{"title": "Second"}, 201)["id"].(string)
	gone := a.m("POST", pp+"/requirements", map[string]any{"title": "Deleted"}, 201)["id"].(string)
	a.m("DELETE", "/api/v1/requirements/"+gone, nil, 204)
	steps := []map[string]any{{"action": "open", "expected": "page"}}
	t1 := a.m("POST", pp+"/tests", map[string]any{"title": "Secret test", "steps": steps}, 201)["id"].(string)
	t2 := a.m("POST", pp+"/tests", map[string]any{"title": "Other test"}, 201)["id"].(string)
	fx.reqID, fx.tests = r1, []string{t1, t2}
	a.m("POST", pp+"/links", map[string]any{"requirement_id": r1, "test_case_id": t1}, 201)
	a.m("POST", "/api/v1/requirements/"+r1+"/relations", map[string]any{"target_id": r2, "type": "refines"}, 201)
	a.m("POST", "/api/v1/requirements/"+r1+"/item-links", map[string]any{"target_type": "url", "url": "https://example.org/spec"}, 201)
	a.m("POST", "/api/v1/requirements/"+r1+"/comments", map[string]any{"body": "secret note"}, 201)
	a.m("POST", "/api/v1/tests/"+t2+"/dependencies", map[string]any{"depends_on_id": t1}, 201)
	a.m("POST", pp+"/folders", map[string]any{"kind": "test", "name": "Suite"}, 201)
	a.m("POST", pp+"/baselines", map[string]any{"name": "B1"}, 201)
	a.m("POST", pp+"/releases", map[string]any{"name": "1.0"}, 201)
	a.m("POST", pp+"/defects", map[string]any{"title": "Secret defect", "severity": "minor"}, 201)
	a.m("POST", pp+"/risks", map[string]any{"title": "Secret risk", "likelihood": 2, "impact": 2}, 201)
	a.m("POST", pp+"/waivers", map[string]any{"title": "Secret waiver", "requirement_ids": []string{r1}}, 201)
	a.m("POST", pp+"/change-requests", map[string]any{"title": "Secret change"}, 201)
	a.m("POST", pp+"/webhooks", map[string]any{"name": "h", "url": "https://hooks.example.org/x", "events": []string{"defect.updated"}, "enabled": false}, 201)
	a.m("POST", pp+"/chat-channels", map[string]any{"name": "c", "provider": "slack", "url": "https://hooks.example.org/c", "events": []string{"defect.updated"}, "enabled": false}, 201)
	a.m("POST", pp+"/configurations", map[string]any{"name": "Chrome"}, 201)
	a.m("POST", pp+"/variants", map[string]any{"name": "EU model"}, 201)
	a.m("POST", pp+"/builds", map[string]any{"external_id": "ci-1"}, 201)
	cyc := a.m("POST", pp+"/cycles", map[string]any{"name": "Secret cycle"}, 201)["id"].(string)
	a.m("POST", pp+"/cycle-schedules", map[string]any{"name_pattern": "Secret {date}", "template_cycle_id": cyc, "frequency": "daily"}, 201)
	a.m("POST", pp+"/custom-fields", map[string]any{"entity_type": "requirement", "label": "Supplier", "type": "text"}, 201)
	a.m("POST", pp+"/templates", map[string]any{"kind": "requirement", "name": "Secret template"}, 201)
	def := a.m("POST", pp+"/reports/definitions", map[string]any{"name": "Status", "config": map[string]any{"entity": "test", "columns": []string{"key"}}}, 201)["id"].(string)
	a.m("POST", pp+"/reports/subscriptions", map[string]any{"definition_id": def, "recipients": []string{e.admin.ID}}, 201)
	a.m("POST", "/api/v1/reviews", map[string]any{"entity_type": "requirement", "entity_id": r1, "reviewers": []string{e.admin.ID}}, 201)
	a.upload("POST", "/api/v1/requirements/"+r1+"/attachments", "file", "secret.txt", "text/plain", []byte("secret"), nil, 201)
	tr := model.TraceRule{ProjectID: pid, Name: "verified", Severity: "warning", Scope: model.KindRequirement, Kind: model.TraceVerifiedBy, Min: 1, Enabled: true}
	if err := e.st.CreateTraceRule(ctx, e.tenant.ID, &tr); err != nil {
		t.Fatal(err)
	}
	run := &model.TestRun{ProjectID: pid, Name: "Secret run"}
	if err := e.st.CreateRun(ctx, e.tenant.ID, run, []string{t1}); err != nil {
		t.Fatal(err)
	}
	a.m("POST", "/api/v1/requirements/"+r1+"/verification-records", map[string]any{"method": "inspection", "result": "pass"}, 201)
	a.m("POST", pp+"/elements", map[string]any{"name": "Secret unit"}, 201)
	a.m("POST", pp+"/exploratory-sessions", map[string]any{"title": "Secret session"}, 201)
	a.m("PUT", pp+"/access", map[string]any{"access": "restricted"}, 200)
	// an id of each resource, read from the database
	q := map[string]string{
		"requirements": `SELECT id FROM requirements WHERE project_id = ?`, "tests": `SELECT id FROM test_cases WHERE project_id = ?`,
		"runs": `SELECT id FROM test_runs WHERE project_id = ?`, "executions": `SELECT e.id FROM test_executions e JOIN test_runs r ON r.id = e.run_id WHERE r.project_id = ?`,
		"step-results": `SELECT sr.id FROM step_results sr JOIN test_executions e ON e.id = sr.execution_id JOIN test_runs r ON r.id = e.run_id WHERE r.project_id = ?`,
		"folders":      `SELECT id FROM folders WHERE project_id = ?`, "categories": `SELECT id FROM categories WHERE project_id = ?`,
		"relations": `SELECT id FROM requirement_relations WHERE project_id = ?`, "baselines": `SELECT id FROM baselines WHERE project_id = ?`,
		"links": `SELECT id FROM trace_links WHERE project_id = ?`, "item-links": `SELECT id FROM item_links WHERE project_id = ?`,
		"trash": `SELECT id FROM trash WHERE project_id = ?`, "releases": `SELECT id FROM releases WHERE project_id = ?`,
		"defects": `SELECT id FROM defects WHERE project_id = ?`, "risks": `SELECT id FROM risks WHERE project_id = ?`,
		"waivers":         `SELECT id FROM waivers WHERE project_id = ?`,
		"change-requests": `SELECT id FROM change_requests WHERE project_id = ?`,
		"webhooks":        `SELECT id FROM webhooks WHERE project_id = ?`, "trace-rules": `SELECT id FROM trace_rules WHERE project_id = ?`,
		"chat-channels":  `SELECT id FROM chat_channels WHERE project_id = ?`,
		"configurations": `SELECT id FROM configurations WHERE project_id = ?`, "test-dependencies": `SELECT id FROM test_dependencies WHERE project_id = ?`,
		"reports/definitions": `SELECT id FROM report_definitions WHERE project_id = ?`, "reports/subscriptions": `SELECT id FROM report_subscriptions WHERE project_id = ?`,
		"reviews": `SELECT id FROM review_requests WHERE project_id = ?`, "files": `SELECT id FROM files WHERE project_id = ?`,
		"comments":             `SELECT c.id FROM comments c JOIN requirements r ON r.id = c.entity_id WHERE r.project_id = ?`,
		"verification-records": `SELECT id FROM verification_records WHERE project_id = ?`,
		"elements":             `SELECT id FROM system_elements WHERE project_id = ?`,
		"exploratory-sessions": `SELECT id FROM exploratory_sessions WHERE project_id = ?`,
		"variants":             `SELECT id FROM variants WHERE project_id = ?`, "builds": `SELECT id FROM builds WHERE project_id = ?`,
		"custom-fields": `SELECT id FROM custom_fields WHERE project_id = ?`, "templates": `SELECT id FROM item_templates WHERE project_id = ?`,
		"cycles": `SELECT id FROM test_cycles WHERE project_id = ?`, "cycle-schedules": `SELECT id FROM test_cycle_schedules WHERE project_id = ?`,
	}
	for res, sql := range q {
		var id string
		if err := e.st.DB.QueryRowContext(ctx, e.st.DB.Rebind(sql), pid).Scan(&id); err != nil {
			t.Fatalf("fixture %s: %v", res, err)
		}
		fx.ids[res] = id
	}
	return fx
}

// TestProjectAccessRouteWalk walks every registered route and asserts that
// someone outside a restricted project (an organisation manager, with a
// session and with an API token) gets 404 or 403 for each route about the
// project or one of its resources.
func TestProjectAccessRouteWalk(t *testing.T) {
	e := newEnv(t)
	fx := newSharedFixture(t, e)
	e.member("mallory@example.com", rbac.RoleManager)
	m := e.login("mallory@example.com")
	tok := m.m("POST", "/api/v1/tokens", map[string]any{"name": "ci"}, 201)["token"].(string)
	bt := e.anon()
	bt.bearer = tok
	// every {id} resource must be known: project resource or not
	for _, rt := range e.srv.Routes() {
		if res := resourceOf(rt.Pattern); res != "" {
			if _, ok := resourceProjects[res]; !ok && !nonProjectResources[res] {
				t.Errorf("%s: resource %q is neither in resourceProjects nor in nonProjectResources", rt.Pattern, res)
			}
		}
	}
	walked := 0
	for _, rt := range e.srv.Routes() {
		if rt.Public {
			continue
		}
		method, path, _ := strings.Cut(rt.Pattern, " ")
		res := resourceOf(rt.Pattern)
		var url string
		switch {
		case strings.Contains(path, "{pid}"):
			url = strings.ReplaceAll(path, "{pid}", fx.pid)
		case res != "" && resourceProjects[res] != "":
			id, ok := fx.ids[res]
			if !ok {
				t.Errorf("%s: no fixture for resource %q", rt.Pattern, res)
				continue
			}
			url = strings.ReplaceAll(path, "{id}", id)
		default:
			continue
		}
		for _, x := range []string{"{file}", "{mid}"} {
			url = strings.ReplaceAll(url, x, "x")
		}
		for _, c := range []*client{m, bt} {
			res := c.raw(method, url, "{}")
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusForbidden {
				t.Errorf("%s as a non-member (%s): status %d: %.200s", rt.Pattern, map[bool]string{true: "token", false: "session"}[c.bearer != ""], res.StatusCode, body)
			}
			if strings.Contains(string(body), "Secret") || strings.Contains(string(body), "secret") {
				t.Errorf("%s leaks: %.200s", rt.Pattern, body)
			}
		}
		walked++
	}
	if walked < 200 {
		t.Fatalf("only %d routes walked", walked)
	}
	// lists across projects leave the project out
	for _, c := range []*client{m, bt} {
		for _, p := range []string{"/api/v1/projects", "/api/v1/search?q=secret", "/api/v1/reviews?requested_by=me", "/api/v1/reviews/in-review", "/api/v1/notifications", "/api/v1/catalog/requirements?project_id=" + fx.pid, "/api/v1/oslc/catalog"} {
			res := c.raw("GET", p, nil)
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if strings.Contains(string(body), fx.pid) || strings.Contains(string(body), "SECRET") || strings.Contains(string(body), "Secret") {
				t.Errorf("GET %s leaks the restricted project: %.300s", p, body)
			}
		}
	}
	// moving into the restricted project is refused
	of := m.m("POST", "/api/v1/projects/"+fx.open+"/folders", map[string]any{"kind": "test", "name": "Mine"}, 201)
	m.m("POST", "/api/v1/folders/"+of["id"].(string)+"/move", map[string]any{"project_id": fx.pid}, 404)
	ot := m.m("POST", "/api/v1/projects/"+fx.open+"/tests", map[string]any{"title": "Mine"}, 201)
	m.m("POST", "/api/v1/projects/"+fx.open+"/tests/move", map[string]any{"ids": []string{ot["id"].(string)}, "project_id": fx.pid}, 404)
	m.m("POST", "/api/v1/projects/"+fx.open+"/folders/bulk", map[string]any{"ids": []string{of["id"].(string)}, "action": "move", "project_id": fx.pid}, 404)
}

// projectFingerprint serialises the rows of a project (and of the main
// child tables), to see that requests elsewhere left it unchanged.
func projectFingerprint(t *testing.T, e *env, pid string) string {
	t.Helper()
	var b strings.Builder
	for _, q := range []string{
		`SELECT * FROM projects WHERE id = ?`,
		`SELECT * FROM requirements WHERE project_id = ? ORDER BY id`, `SELECT * FROM test_cases WHERE project_id = ? ORDER BY id`,
		`SELECT * FROM folders WHERE project_id = ? ORDER BY id`, `SELECT * FROM categories WHERE project_id = ? ORDER BY id`,
		`SELECT * FROM trace_links WHERE project_id = ? ORDER BY id`, `SELECT * FROM test_runs WHERE project_id = ? ORDER BY id`,
		`SELECT * FROM requirement_relations WHERE project_id = ? ORDER BY id`, `SELECT * FROM baselines WHERE project_id = ? ORDER BY id`,
		`SELECT * FROM releases WHERE project_id = ? ORDER BY id`, `SELECT * FROM defects WHERE project_id = ? ORDER BY id`,
		`SELECT * FROM risks WHERE project_id = ? ORDER BY id`, `SELECT * FROM item_links WHERE project_id = ? ORDER BY id`,
		`SELECT * FROM webhooks WHERE project_id = ? ORDER BY id`, `SELECT * FROM trace_rules WHERE project_id = ? ORDER BY id`,
		`SELECT * FROM configurations WHERE project_id = ? ORDER BY id`, `SELECT * FROM test_dependencies WHERE project_id = ? ORDER BY id`,
		`SELECT * FROM report_definitions WHERE project_id = ? ORDER BY id`, `SELECT * FROM review_requests WHERE project_id = ? ORDER BY id`,
		`SELECT * FROM files WHERE project_id = ? ORDER BY id`, `SELECT * FROM trash WHERE project_id = ? ORDER BY id`,
		`SELECT e.* FROM test_executions e JOIN test_runs r ON r.id = e.run_id WHERE r.project_id = ? ORDER BY e.id`,
		`SELECT c.* FROM comments c JOIN requirements r ON r.id = c.entity_id WHERE r.project_id = ? ORDER BY c.id`,
		`SELECT x.* FROM requirement_categories x JOIN requirements r ON r.id = x.requirement_id WHERE r.project_id = ? ORDER BY 1, 2`,
		`SELECT x.* FROM attachments x JOIN files f ON f.id = x.file_id WHERE f.project_id = ? ORDER BY 1, 2, 3`,
		`SELECT * FROM project_members WHERE project_id = ? ORDER BY member_id`,
	} {
		rows, err := e.st.DB.QueryContext(context.Background(), e.st.DB.Rebind(q), pid)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintln(&b, vals...)
		}
		rows.Close()
	}
	return b.String()
}

// TestProjectAccessCrossProject sends every write route of an
// organisation-wide project the ids of a restricted project's resources (in
// the usual body fields) as someone who may write there but not see the
// restricted project: nothing of the restricted project may change or leak.
func TestProjectAccessCrossProject(t *testing.T) {
	e := newEnv(t)
	fx := newSharedFixture(t, e)
	e.member("mallory@example.com", rbac.RoleManager)
	m := e.login("mallory@example.com")
	before := projectFingerprint(t, e, fx.pid)
	keys := []string{"ids", "requirement_ids", "test_ids", "test_case_ids", "item_ids", "run_ids", "defect_ids", "risk_ids",
		"release_ids", "baseline_ids", "folder_ids", "category_ids", "link_ids", "file_ids", "definition_ids", "configuration_ids",
		"requirement_id", "test_case_id", "test_id", "source_id", "target_id", "id", "release_id", "folder_id", "parent_id", "target_folder_id",
		"definition_id", "file_id", "run_id", "entity_id", "depends_on_id", "configuration_id"}
	actions := []string{"", "delete", "move", "remove", "copy", "merge", "color", "status", "assign", "close", "link", "unlink", "add", "approve", "set"}
	sent := 0
	for _, rt := range e.srv.Routes() {
		method, path, _ := strings.Cut(rt.Pattern, " ")
		if rt.Public || method == "GET" || !strings.Contains(path, "{pid}") || strings.Contains(path, "/members") || strings.Contains(path, "/access") {
			continue
		}
		url := strings.ReplaceAll(path, "{pid}", fx.open)
		for res, id := range fx.ids {
			for _, action := range actions {
				body := map[string]any{"action": action, "status": "approved", "project_id": fx.open, "kind": "requirement", "entity_type": strings.TrimSuffix(res, "s")}
				for _, k := range keys {
					if strings.HasSuffix(k, "_ids") || k == "ids" {
						body[k] = []string{id}
					} else {
						body[k] = id
					}
				}
				body["links"] = []map[string]any{{"requirement_id": id, "test_case_id": id, "source_id": id, "target_id": id, "entity_id": id}}
				res := m.raw(method, url, body)
				data, _ := io.ReadAll(res.Body)
				res.Body.Close()
				if strings.Contains(string(data), "Secret") || strings.Contains(string(data), "secret note") {
					t.Errorf("%s with %s ids leaks: %.300s", rt.Pattern, res.Request.URL.Path, data)
				}
				sent++
			}
		}
	}
	if after := projectFingerprint(t, e, fx.pid); after != before {
		t.Errorf("requests in another project changed the restricted project:\nbefore %s\nafter  %s", before, after)
	}
	if sent < 1000 {
		t.Fatalf("only %d requests", sent)
	}
}

// TestProjectSharing covers the sharing API and the effective permissions
// of project roles.
func TestProjectSharing(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("SHR")
	pp := "/api/v1/projects/" + pid
	rq := a.m("POST", pp+"/requirements", map[string]any{"title": "R"}, 201)["id"].(string)
	vic := e.member("vic@example.com", rbac.RoleManager)
	tess := e.member("tess@example.com", rbac.RoleViewer)
	e.member("olga@example.com", rbac.RoleManager)
	v, ts, o := e.login("vic@example.com"), e.login("tess@example.com"), e.login("olga@example.com")

	// organisation mode: everybody sees it; a project role changes the rights
	acc := a.m("GET", pp+"/access", nil, 200)
	if acc["access"] != "organisation" || acc["members"].(float64) != 0 {
		t.Fatal(acc)
	}
	ts.m("PATCH", "/api/v1/requirements/"+rq, map[string]any{"title": "x"}, 403)
	a.m("POST", pp+"/members", map[string]any{"email": "tess@example.com", "role": "manager"}, 201)
	ts.m("PATCH", "/api/v1/requirements/"+rq, map[string]any{"title": "by tess"}, 200)
	pr := ts.m("GET", pp, nil, 200)
	my := pr["my_access"].(map[string]any)
	if my["source"] != "member" || my["role"] != "manager" || my["can_share"] != true {
		t.Fatal(my)
	}
	// lowering: vic (org manager) is a viewer here
	a.m("POST", pp+"/members", map[string]any{"user_id": vic.ID, "role": "viewer"}, 201)
	v.m("PATCH", "/api/v1/requirements/"+rq, map[string]any{"title": "x"}, 403)
	v.m("PATCH", pp, map[string]any{"name": "x"}, 403)
	// organisation-level permissions stay those of the organisation role
	v.m("GET", "/api/v1/audit", nil, 200)
	ts.m("GET", "/api/v1/audit", nil, 403)

	// restricted: olga (not a member) loses it, tess and vic keep it
	a.m("PUT", pp+"/access", map[string]any{"access": "restricted"}, 200)
	o.m("GET", pp, nil, 404)
	o.m("GET", "/api/v1/requirements/"+rq, nil, 404)
	if n := len(items(o.m("GET", "/api/v1/projects", nil, 200))); n != 0 {
		t.Fatal(n)
	}
	ts.m("GET", "/api/v1/requirements/"+rq, nil, 200)
	list := items(ts.m("GET", "/api/v1/projects", nil, 200))
	if len(list) != 1 || list[0]["access"] != "restricted" || list[0]["my_access"].(map[string]any)["role"] != "manager" {
		t.Fatal(list)
	}
	// the notification of the share reached tess
	ns := ts.m("GET", "/api/v1/notifications", nil, 200)
	if got := items(ns); len(got) == 0 || got[0]["event"] != "project.shared" {
		t.Fatal(ns)
	}

	// members API
	ms := items(a.m("GET", pp+"/members", nil, 200))
	if len(ms) != 2 {
		t.Fatal(ms)
	}
	cands := items(ts.m("GET", pp+"/members/candidates?q=olga", nil, 200))
	if len(cands) != 1 || cands[0]["email"] != "olga@example.com" {
		t.Fatal(cands)
	}
	// tess (project manager) cannot give admin, can give tester
	ts.m("POST", pp+"/members", map[string]any{"email": "olga@example.com", "role": "admin"}, 403)
	ts.m("POST", pp+"/members", map[string]any{"email": "olga@example.com", "role": "tester"}, 201)
	ts.m("POST", pp+"/members", map[string]any{"email": "nobody@example.com", "role": "tester"}, 400)
	ts.m("POST", pp+"/members", map[string]any{"email": "olga@example.com", "role": "nope"}, 400)
	o.m("GET", pp, nil, 200)
	o.m("PATCH", pp+"/members/"+vic.ID, map[string]any{"role": "tester"}, 403) // tester: no project.update
	ts.m("PATCH", pp+"/members/"+vic.ID, map[string]any{"role": "tester"}, 200)
	ts.m("PATCH", pp+"/members/missing", map[string]any{"role": "tester"}, 404)
	res := ts.m("POST", pp+"/members/bulk", map[string]any{"ids": []string{vic.ID, "missing"}, "action": "role", "role": "reviewer"}, 200)
	if len(res["changed"].([]any)) != 1 || len(res["skipped"].([]any)) != 1 {
		t.Fatal(res)
	}
	ts.m("POST", pp+"/members/bulk", map[string]any{"ids": []string{vic.ID}, "action": "bogus"}, 400)
	ts.m("DELETE", pp+"/members/"+vic.ID, nil, 204)
	v.m("GET", pp, nil, 404)
	// a member may leave
	o.m("DELETE", pp+"/members/"+o.me(t)["user"].(map[string]any)["id"].(string), nil, 204)
	o.m("GET", pp, nil, 404)

	// audit trail
	au := a.m("GET", "/api/v1/audit", nil, 200)
	var actions []string
	for _, x := range items(au) {
		actions = append(actions, x["action"].(string))
	}
	for _, want := range []string{"project.access", "project.member.add", "project.member.role", "project.member.remove"} {
		if !slices.Contains(actions, want) {
			t.Errorf("audit: no %s in %v", want, actions)
		}
	}

	// restricting as a non-admin keeps the caller in the project
	pid2 := v.project("VIC")
	v.m("PUT", "/api/v1/projects/"+pid2+"/access", map[string]any{"access": "restricted"}, 200)
	v.m("GET", "/api/v1/projects/"+pid2, nil, 200)
	ms2 := items(v.m("GET", "/api/v1/projects/"+pid2+"/members", nil, 200))
	if len(ms2) != 1 || ms2[0]["role"] != "manager" {
		t.Fatal(ms2)
	}
	ts.m("GET", "/api/v1/projects/"+pid2, nil, 404)
	v.m("PUT", "/api/v1/projects/"+pid2+"/access", map[string]any{"access": "secret"}, 400)
	// an administrator always sees it
	a.m("GET", "/api/v1/projects/"+pid2, nil, 200)

	// removing someone from the organisation removes their project roles
	if err := e.st.RemoveMembership(context.Background(), e.tenant.ID, tess.ID); err != nil {
		t.Fatal(err)
	}
	if ms := items(a.m("GET", pp+"/members", nil, 200)); len(ms) != 0 {
		t.Fatal(ms)
	}
}

// TestProjectSharingGroupsAndTokens: a SCIM group as project member, and an
// API token capped by its role inside a project.
func TestProjectSharingGroupsAndTokens(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.login("admin@example.com")
	pid := a.project("GRP")
	pp := "/api/v1/projects/" + pid
	rq := a.m("POST", pp+"/requirements", map[string]any{"title": "R"}, 201)["id"].(string)
	a.m("PUT", pp+"/access", map[string]any{"access": "restricted"}, 200)
	gina := e.member("gina@example.com", rbac.RoleTester)
	g := &model.SCIMGroup{TenantID: e.tenant.ID, DisplayName: "QA", Members: []string{gina.ID}}
	if err := e.st.CreateSCIMGroup(ctx, g); err != nil {
		t.Fatal(err)
	}
	gc := e.login("gina@example.com")
	gc.m("GET", pp, nil, 404)
	gc.m("GET", "/api/v1/requirements/"+rq, nil, 404)
	a.m("POST", pp+"/members", map[string]any{"group_id": g.ID, "role": "manager"}, 201)
	pr := gc.m("GET", pp, nil, 200)
	if my := pr["my_access"].(map[string]any); my["source"] != "group" || my["role"] != "manager" {
		t.Fatal(my)
	}
	gc.m("PATCH", "/api/v1/requirements/"+rq, map[string]any{"title": "by group"}, 200)
	// a direct membership wins over the group
	a.m("POST", pp+"/members", map[string]any{"user_id": gina.ID, "role": "viewer"}, 201)
	gc.m("PATCH", "/api/v1/requirements/"+rq, map[string]any{"title": "x"}, 403)
	if err := e.st.DeleteSCIMGroup(ctx, e.tenant.ID, g.ID); err != nil {
		t.Fatal(err)
	}
	if ms := items(a.m("GET", pp+"/members", nil, 200)); len(ms) != 1 {
		t.Fatal(ms)
	}
	// a viewer token of a project manager stays a viewer in the project
	a.m("PATCH", pp+"/members/"+gina.ID, map[string]any{"role": "manager"}, 200)
	tok := gc.m("POST", "/api/v1/tokens", map[string]any{"name": "ro", "role": "viewer"}, 201)["token"].(string)
	b := e.anon()
	b.bearer = tok
	b.m("GET", "/api/v1/requirements/"+rq, nil, 200)
	b.m("PATCH", "/api/v1/requirements/"+rq, map[string]any{"title": "x"}, 403)
	// a deleted custom role in use by a project member is refused
	a.m("POST", "/api/v1/roles", map[string]any{"name": "auditor", "permissions": []string{"project.read"}}, 201)
	a.m("PATCH", pp+"/members/"+gina.ID, map[string]any{"role": "auditor"}, 200)
	a.m("DELETE", "/api/v1/roles/auditor", nil, 409)
}

// TestProjectSharingEdges: errors of the sharing API, archives of
// restricted projects, reviews and report owners in restricted projects.
func TestProjectSharingEdges(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.login("admin@example.com")
	man := e.member("man@example.com", rbac.RoleManager)
	ted := e.member("ted@example.com", rbac.RoleTester)
	e.member("vera@example.com", rbac.RoleViewer)
	mc, tc, vc := e.login("man@example.com"), e.login("ted@example.com"), e.login("vera@example.com")
	pid := mc.project("EDGE")
	pp := "/api/v1/projects/" + pid
	rq := mc.m("POST", pp+"/requirements", map[string]any{"title": "R"}, 201)["id"].(string)
	// errors
	mc.m("GET", "/api/v1/projects/missing/access", nil, 404)
	mc.m("PUT", pp+"/access", "not json", 400)
	mc.m("POST", pp+"/members", "not json", 400)
	mc.m("POST", pp+"/members", map[string]any{"role": "viewer"}, 400)
	mc.m("POST", pp+"/members", map[string]any{"group_id": "missing", "role": "viewer"}, 400)
	mc.m("POST", pp+"/members", map[string]any{"user_id": "missing", "role": "viewer"}, 400)
	mc.m("POST", pp+"/members/bulk", "not json", 400)
	mc.m("POST", pp+"/members/bulk", map[string]any{"ids": []string{}, "action": "remove"}, 400)
	mc.m("POST", pp+"/members/bulk", map[string]any{"ids": []string{"x"}, "action": "role", "role": "admin"}, 403)
	mc.m("DELETE", pp+"/members/missing", nil, 404)
	tc.m("PUT", pp+"/access", map[string]any{"access": "restricted"}, 403) // tester: no project.update
	// the manager restricts the project and becomes a member; Ted is a tester member, Vera nothing
	acc := mc.m("PUT", pp+"/access", map[string]any{"access": "restricted"}, 200)
	if acc["members"].(float64) != 1 || acc["my_access"].(map[string]any)["source"] != "member" {
		t.Fatal(acc)
	}
	mc.m("POST", pp+"/members", map[string]any{"user_id": ted.ID, "role": "tester"}, 201)
	mc.m("POST", pp+"/members", map[string]any{"user_id": ted.ID, "role": "reviewer"}, 200)
	mc.m("PATCH", pp+"/members/"+ted.ID, "not json", 400)
	mc.m("PATCH", pp+"/members/"+ted.ID, map[string]any{"role": "admin"}, 403)
	// an admin member: the manager can neither change nor remove them
	a.m("POST", pp+"/members", map[string]any{"email": "vera@example.com", "role": "admin"}, 201)
	vera, _ := e.st.GetUserByEmail(ctx, "vera@example.com")
	mc.m("PATCH", pp+"/members/"+vera.ID, map[string]any{"role": "viewer"}, 403)
	mc.m("DELETE", pp+"/members/"+vera.ID, nil, 403)
	res := mc.m("POST", pp+"/members/bulk", map[string]any{"ids": []string{vera.ID, ted.ID, man.ID}, "action": "remove"}, 200)
	if len(res["changed"].([]any)) != 2 || len(res["skipped"].([]any)) != 1 {
		t.Fatal(res) // ted and the manager themselves; vera (admin) skipped
	}
	mc.m("GET", pp, nil, 404) // left the project
	ted2 := tc
	ted2.m("GET", pp, nil, 404)
	vc.m("GET", pp, nil, 200)
	a.m("POST", pp+"/members", map[string]any{"user_id": man.ID, "role": "manager"}, 201)

	// review requests only go to people who see the project
	mc.m("POST", "/api/v1/reviews", map[string]any{"entity_type": "requirement", "entity_id": rq, "reviewers": []string{"ted@example.com"}}, 400)
	rv := items(mc.m("POST", "/api/v1/reviews", map[string]any{"entity_type": "requirement", "entity_id": rq, "reviewers": []string{"vera@example.com"}}, 201))[0]["id"].(string)
	if n := len(items(mc.m("GET", "/api/v1/reviews?project_id="+pid+"&entity_type=requirement", nil, 200))); n != 1 {
		t.Fatal(n)
	}
	tc.m("GET", "/api/v1/reviews?project_id="+pid, nil, 404)
	tc.m("GET", "/api/v1/reviews/"+rv, nil, 404)
	tc.m("POST", "/api/v1/reviews", map[string]any{"entity_type": "requirement", "entity_id": rq, "reviewers": []string{"vera@example.com"}}, 404)
	// mentions of people without access do not notify them
	mc.m("POST", "/api/v1/requirements/"+rq+"/comments", map[string]any{"body": "see @ted and @vera"}, 201)
	for _, x := range items(tc.m("GET", "/api/v1/notifications", nil, 200)) {
		if x["project_id"] == pid {
			t.Fatal(x)
		}
	}
	if len(items(vc.m("GET", "/api/v1/notifications", nil, 200))) == 0 {
		t.Fatal("vera was not notified")
	}
	// people (mentions, assignees) are the people who see the project
	for _, x := range items(mc.m("GET", pp+"/people", nil, 200)) {
		if x["email"] == "ted@example.com" {
			t.Fatal(x)
		}
	}
	// report recipients and the owner of a scheduled report
	recips := mc.m("GET", pp+"/reports/recipients", nil, 200)
	for _, x := range items(recips) {
		if x["email"] == "ted@example.com" {
			t.Fatal(recips)
		}
	}
	if _, err := e.srv.ownerPrincipal(ctx, e.tenant.ID, ted.ID, pid); err == nil {
		t.Fatal("ted may not see the project")
	}
	if p, err := e.srv.ownerPrincipal(ctx, e.tenant.ID, man.ID, pid); err != nil || !p.Can(rbac.Export) {
		t.Fatal(p, err)
	}
	if e.srv.userCanSee(ctx, e.tenant.ID, pid, "missing", "") || !e.srv.userCanSee(ctx, e.tenant.ID, "", "missing", "") {
		t.Fatal("userCanSee")
	}
	// a duplicate keeps the members and the caller; an archive keeps the loader in it
	dup := mc.m("POST", pp+"/duplicate", map[string]any{"key": "EDGE2"}, 201)["project"].(map[string]any)
	if dup["access"] != "restricted" {
		t.Fatal(dup)
	}
	dms := items(mc.m("GET", "/api/v1/projects/"+dup["id"].(string)+"/members", nil, 200))
	if len(dms) != 2 {
		t.Fatal(dms)
	}
	_, archive := mc.getBytes(pp+"/archive", 200)
	e.member("lou@example.com", rbac.RoleManager)
	lc := e.login("lou@example.com")
	loaded := lc.upload("POST", "/api/v1/projects/import", "archive", "a.zip", "application/zip", archive, map[string]string{"key": "LOU"}, 201)["project"].(map[string]any)
	lc.m("GET", "/api/v1/projects/"+loaded["id"].(string), nil, 200)
	if ms := items(lc.m("GET", "/api/v1/projects/"+loaded["id"].(string)+"/members", nil, 200)); len(ms) != 1 || ms[0]["role"] != "manager" {
		t.Fatal(ms)
	}
	// opening the project to the organisation again
	mc.m("PUT", pp+"/access", map[string]any{"access": "organisation"}, 200)
	tc.m("GET", pp, nil, 200)
}

// TestPrincipalFor covers the principal of another user (notifications,
// scheduled reports).
func TestPrincipalFor(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	u := mkUser(t, e.st, "out@example.com", false)
	if _, err := e.srv.principalFor(ctx, e.tenant.ID, u.ID); err == nil {
		t.Fatal("not a member")
	}
	if _, err := e.srv.principalFor(ctx, "missing", e.admin.ID); err == nil {
		t.Fatal("no tenant")
	}
	sup := mkUser(t, e.st, "sup@example.com", true)
	if p, err := e.srv.principalFor(ctx, e.tenant.ID, sup.ID); err != nil || p.Role != rbac.RoleAdmin {
		t.Fatal(p, err)
	}
	d := e.member("dis@example.com", rbac.RoleViewer)
	d.Disabled = true
	if err := e.st.UpdateUser(ctx, d); err != nil {
		t.Fatal(err)
	}
	if _, err := e.srv.principalFor(ctx, e.tenant.ID, d.ID); err == nil {
		t.Fatal("disabled")
	}
	if _, err := e.srv.ownerPrincipal(ctx, e.tenant.ID, d.ID, ""); err == nil {
		t.Fatal("disabled owner")
	}
}

func (c *client) me(t *testing.T) map[string]any {
	t.Helper()
	return c.m("GET", "/api/v1/auth/me", nil, 200)
}
