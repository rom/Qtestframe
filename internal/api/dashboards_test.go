package api

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/rbac"
)

func widget(id, typ string, x, y, w, h int, settings map[string]any) map[string]any {
	out := map[string]any{"id": id, "type": typ, "x": x, "y": y, "w": w, "h": h}
	if settings != nil {
		out["settings"] = settings
	}
	return out
}

func dashIDs(list []map[string]any) []string {
	var out []string
	for _, d := range list {
		out = append(out, d["id"].(string))
	}
	return out
}

func TestDashboardLayouts(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("DSH")
	base := "/api/v1/projects/" + pid + "/dashboards"
	e.member("vera@example.com", rbac.RoleViewer)
	v := e.login("vera@example.com")
	e.member("mona@example.com", rbac.RoleManager)
	m := e.login("mona@example.com")

	// the built-in layouts: the former fixed dashboard
	l := a.m("GET", base, nil, 200)
	list := items(l)
	if !slices.Equal(dashIDs(list), []string{"overview", "trends", "quality"}) || l["default_id"] != "overview" || l["can_manage"] != true {
		t.Fatal(l)
	}
	if list[0]["stored"] != false || list[0]["shared"] != true || list[0]["default"] != true || len(list[0]["widgets"].([]any)) != 12 {
		t.Fatal(list[0])
	}
	if vl := v.m("GET", base, nil, 200); vl["can_manage"] != false || items(vl)[0]["editable"] != false {
		t.Fatal(vl)
	}
	a.m("GET", base+"/quality", nil, 200)
	a.m("GET", base+"/nope", nil, 404)

	// a viewer cannot change shared ones, only make personal ones
	v.m("PUT", base+"/overview", map[string]any{"widgets": []any{}}, 403)
	v.m("POST", base, map[string]any{"name": "Shared", "shared": true}, 403)
	v.m("PUT", base+"/default", map[string]any{"dashboard_id": "trends"}, 403)
	mine := v.m("POST", base, map[string]any{"source_id": "overview", "name": "My overview"}, 201)
	if mine["shared"] != false || mine["builtin"] != "overview" || len(mine["widgets"].([]any)) != 12 || mine["editable"] != true {
		t.Fatal(mine)
	}
	mid := mine["id"].(string)
	// only its owner sees and changes it
	a.m("GET", base+"/"+mid, nil, 404)
	a.m("PUT", base+"/"+mid, map[string]any{"name": "x"}, 404)
	if ids := dashIDs(items(v.m("GET", base, nil, 200))); !slices.Contains(ids, mid) {
		t.Fatal(ids)
	}
	if ids := dashIDs(items(a.m("GET", base, nil, 200))); slices.Contains(ids, mid) {
		t.Fatal(ids)
	}
	upd := v.m("PUT", base+"/"+mid, map[string]any{"name": "Mine", "widgets": []any{widget("n1", "note", 0, 0, 6, 3, map[string]any{"text": "**hi**"})}}, 200)
	if upd["name"] != "Mine" || len(upd["widgets"].([]any)) != 1 {
		t.Fatal(upd)
	}
	// validation
	for _, bad := range []any{
		[]any{widget("a", "nope", 0, 0, 6, 3, nil)},
		[]any{widget("a", "note", 8, 0, 6, 3, nil)},
		[]any{widget("a", "note", 0, 0, 0, 3, nil)},
		[]any{widget("a", "note", 0, 0, 6, 99, nil)},
		[]any{widget("a", "note", 0, 0, 6, 3, nil), widget("a", "note", 6, 0, 6, 3, nil)},
	} {
		v.m("PUT", base+"/"+mid, map[string]any{"widgets": bad}, 400)
	}
	v.m("POST", base, map[string]any{"name": "x", "format": "other"}, 400)
	v.m("POST", base, map[string]any{"name": "x", "format": "qtestframe-dashboard", "version": 2}, 400)
	v.m("POST", base, map[string]any{"name": "x", "source_id": "missing"}, 400)
	// import of an export
	imp := v.m("POST", base, map[string]any{"format": "qtestframe-dashboard", "version": 1, "name": "Imported", "widgets": upd["widgets"]}, 201)
	if imp["name"] != "Imported" || len(imp["widgets"].([]any)) != 1 {
		t.Fatal(imp)
	}
	v.m("PUT", base+"/"+imp["id"].(string), map[string]any{"name": " "}, 400)
	// reset: back to the layout it was copied from
	if r := v.m("POST", base+"/"+mid+"/reset", nil, 200); len(r["widgets"].([]any)) != 12 {
		t.Fatal(r)
	}
	if r := v.m("POST", base+"/"+imp["id"].(string)+"/reset", nil, 200); len(r["widgets"].([]any)) != 12 {
		t.Fatal(r) // no source: the overview
	}

	// a manager changes the built-in overview: stored as a shared dashboard
	ov := m.m("PUT", base+"/overview", map[string]any{"widgets": []any{widget("k", "key_figures", 0, 0, 12, 2, nil)}}, 200)
	ovID := ov["id"].(string)
	if ovID == "overview" || ov["builtin"] != "overview" || ov["stored"] != true || ov["name"] != "Overview" {
		t.Fatal(ov)
	}
	if l := items(v.m("GET", base, nil, 200)); l[0]["id"] != ovID || len(l[0]["widgets"].([]any)) != 1 || l[0]["default"] != true {
		t.Fatal(l[0])
	}
	m.m("DELETE", base+"/trends", nil, 400) // built-in
	// a further shared dashboard, made the default
	sh := m.m("POST", base, map[string]any{"name": "Release health", "shared": true, "widgets": []any{widget("q", "saved_query", 0, 0, 6, 4, map[string]any{"query": "status:draft"})}}, 201)
	shID := sh["id"].(string)
	m.m("PUT", base+"/default", map[string]any{"dashboard_id": mid}, 400) // personal
	d := m.m("PUT", base+"/default", map[string]any{"dashboard_id": shID}, 200)
	if d["default_id"] != shID {
		t.Fatal(d)
	}
	if l := v.m("GET", base, nil, 200); l["default_id"] != shID {
		t.Fatal(l)
	}
	// the overview as default stays the default when it is reset
	m.m("PUT", base+"/default", map[string]any{"dashboard_id": ovID}, 200)
	if r := m.m("POST", base+"/"+ovID+"/reset", nil, 200); r["id"] != "overview" || r["stored"] != false || r["default"] != true {
		t.Fatal(r)
	}
	if l := m.m("GET", base, nil, 200); l["default_id"] != "overview" || len(items(l)[0]["widgets"].([]any)) != 12 {
		t.Fatal(l)
	}
	// deleting the default shared dashboard clears the default
	m.m("PUT", base+"/default", map[string]any{"dashboard_id": shID}, 200)
	v.m("DELETE", base+"/"+shID, nil, 403)
	m.m("DELETE", base+"/"+shID, nil, 204)
	if l := m.m("GET", base, nil, 200); l["default_id"] != "overview" {
		t.Fatal(l)
	}
	m.m("PUT", base+"/default", map[string]any{"dashboard_id": ""}, 200)
	v.m("DELETE", base+"/"+mid, nil, 204)
	v.m("DELETE", base+"/"+mid, nil, 404)

	// the audit log records the changes of shared dashboards
	au := items(a.m("GET", "/api/v1/audit?q=action:dashboard.create", nil, 200))
	if len(au) != 1 {
		t.Fatal(au)
	}
}

func TestHomeDashboard(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	h := a.m("GET", "/api/v1/dashboards/home", nil, 200)
	if h["id"] != "home" || h["stored"] != false || len(h["widgets"].([]any)) != 3 {
		t.Fatal(h)
	}
	pid := a.project("HOM")
	saved := a.m("PUT", "/api/v1/dashboards/home", map[string]any{"widgets": []any{
		widget("w", "my_work", 0, 0, 12, 3, nil), widget("c", "coverage", 0, 3, 6, 4, map[string]any{"project_id": pid}),
	}}, 200)
	if saved["stored"] != true || saved["name"] != "Home" || len(saved["widgets"].([]any)) != 2 {
		t.Fatal(saved)
	}
	again := a.m("PUT", "/api/v1/dashboards/home", map[string]any{"name": "Start", "widgets": saved["widgets"]}, 200)
	if again["id"] != saved["id"] || again["name"] != "Start" {
		t.Fatal(again)
	}
	if g := a.m("GET", "/api/v1/dashboards/home", nil, 200); g["name"] != "Start" {
		t.Fatal(g)
	}
	// another user has their own
	e.member("vera@example.com", rbac.RoleViewer)
	if g := e.login("vera@example.com").m("GET", "/api/v1/dashboards/home", nil, 200); g["stored"] != false {
		t.Fatal(g)
	}
	a.m("PUT", "/api/v1/dashboards/home", map[string]any{"widgets": []any{widget("x", "nope", 0, 0, 6, 3, nil)}}, 400)
	if r := a.m("DELETE", "/api/v1/dashboards/home", nil, 200); r["stored"] != false {
		t.Fatal(r)
	}
	a.m("DELETE", "/api/v1/dashboards/home", nil, 200)
	if wt := items(a.m("GET", "/api/v1/dashboards/widgets", nil, 200)); len(wt) != len(widgetTypes) {
		t.Fatal(wt)
	}
}

func dataOf(t *testing.T, res map[string]any, id string) map[string]any {
	t.Helper()
	r, ok := res["results"].(map[string]any)[id].(map[string]any)
	if !ok {
		t.Fatalf("no result %s: %v", id, res)
	}
	if r["error"] != nil {
		t.Fatalf("%s: %v", id, r["error"])
	}
	d, _ := r["data"].(map[string]any)
	return d
}

func errOf(t *testing.T, res map[string]any, id string) map[string]any {
	t.Helper()
	r := res["results"].(map[string]any)[id].(map[string]any)
	e, ok := r["error"].(map[string]any)
	if !ok {
		t.Fatalf("%s: no error: %v", id, r)
	}
	return e
}

func TestDashboardWidgetData(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("WDG")
	pp := "/api/v1/projects/" + pid
	r1 := a.m("POST", pp+"/requirements", map[string]any{"title": "Login"}, 201)["id"].(string)
	a.m("POST", pp+"/requirements", map[string]any{"title": "Logout"}, 201)
	t1 := a.m("POST", pp+"/tests", map[string]any{"title": "Valid login"}, 201)["id"].(string)
	a.m("POST", pp+"/tests", map[string]any{"title": "Orphan"}, 201)
	a.m("POST", pp+"/links", map[string]any{"requirement_id": r1, "test_case_id": t1}, 201)
	rel := a.m("POST", pp+"/releases", map[string]any{"name": "R1", "status": "active"}, 201)["id"].(string)

	w := func(id, typ string, settings map[string]any) map[string]any {
		return map[string]any{"id": id, "type": typ, "project_id": pid, "settings": settings}
	}
	all := []any{}
	for typ := range widgetTypes {
		all = append(all, w(typ, typ, nil))
	}
	all = append(all,
		w("bd-defects", "breakdown", map[string]any{"dimension": "defect_status"}),
		w("bd-risk", "breakdown", map[string]any{"dimension": "risk_level"}),
		w("bd-bad", "breakdown", map[string]any{"dimension": "nope"}),
		w("q-tests", "saved_query", map[string]any{"entity": "tests", "query": "title:orphan"}),
		w("q-bad", "saved_query", map[string]any{"query": "nofield:x"}),
		w("q-entity", "saved_query", map[string]any{"entity": "users"}),
		w("fw-d3", "framework_coverage", map[string]any{"framework": "d3fend"}),
		w("fw-emb", "framework_coverage", map[string]any{"framework": "emb3d"}),
		w("fw-bad", "framework_coverage", map[string]any{"framework": "x"}),
		w("trend90", "test_results_trend", map[string]any{"range": 90}),
		w("trend365", "defects_per_week", map[string]any{"range": "365"}),
		w("trendbad", "defects_per_week", map[string]any{"range": "x"}),
		w("ready", "release_readiness", map[string]any{"release_id": rel}),
		map[string]any{"id": "noproj", "type": "coverage"},
		map[string]any{"id": "badproj", "type": "coverage", "project_id": "../x"},
		map[string]any{"id": "unknown", "type": "nope", "project_id": pid},
	)
	res := a.m("POST", "/api/v1/dashboards/data", map[string]any{"widgets": all}, 200)
	for typ := range widgetTypes {
		dataOf(t, res, typ)
	}
	if d := dataOf(t, res, "coverage"); d["coverage"] == nil {
		t.Fatal(d)
	}
	kf := dataOf(t, res, "key_figures")
	if kf["dashboard"].(map[string]any)["requirements"].(map[string]any)["total"].(float64) != 2 || kf["insights"] == nil {
		t.Fatal(kf)
	}
	if d := dataOf(t, res, "breakdown"); d["dimension"] != "requirement_status" || d["counts"].(map[string]any)["draft"].(float64) != 2 {
		t.Fatal(d)
	}
	if d := dataOf(t, res, "saved_query"); d["entity"] != "requirements" || d["total"].(float64) != 2 || len(d["items"].([]any)) != 2 {
		t.Fatal(d)
	}
	if d := dataOf(t, res, "q-tests"); d["total"].(float64) != 1 || d["items"].([]any)[0].(map[string]any)["title"] != "Orphan" {
		t.Fatal(d)
	}
	if e := errOf(t, res, "q-bad"); e["code"] != "invalid_query" || e["status"].(float64) != 400 {
		t.Fatal(e)
	}
	errOf(t, res, "q-entity")
	errOf(t, res, "bd-bad")
	errOf(t, res, "fw-bad")
	errOf(t, res, "noproj")
	errOf(t, res, "badproj")
	errOf(t, res, "unknown")
	dataOf(t, res, "bd-defects")
	dataOf(t, res, "bd-risk")
	dataOf(t, res, "fw-d3")
	dataOf(t, res, "fw-emb")
	gaps := dataOf(t, res, "traceability_gaps")
	if g := gaps["requirements"].(map[string]any); g["total"].(float64) != 2 || g["gaps"].(float64) != 1 || g["items"].([]any)[0].(map[string]any)["title"] != "Logout" {
		t.Fatal(gaps)
	}
	if g := gaps["tests"].(map[string]any); g["gaps"].(float64) != 1 {
		t.Fatal(gaps)
	}
	if d := dataOf(t, res, "trend90"); d["range"].(float64) != 90 || d["points"] == nil {
		t.Fatal(d)
	}
	if d := dataOf(t, res, "trend365"); d["range"].(float64) != 365 || d["defect_weeks"] == nil {
		t.Fatal(d)
	}
	if d := dataOf(t, res, "trendbad"); d["range"].(float64) != 30 {
		t.Fatal(d)
	}
	if d := dataOf(t, res, "ready"); d["release"].(map[string]any)["id"] != rel || d["report"] == nil {
		t.Fatal(d)
	}
	if d := dataOf(t, res, "release_burnup"); d["release"].(map[string]any)["id"] != rel || d["burnup"] == nil {
		t.Fatal(d)
	}
	if d := dataOf(t, res, "note"); d != nil {
		t.Fatal(d)
	}
	// a widget without project on the home dashboard: my work of all projects
	home := a.m("POST", "/api/v1/dashboards/data", map[string]any{"widgets": []any{
		map[string]any{"id": "mw", "type": "my_work"}, map[string]any{"id": "dis", "type": "discussions", "settings": map[string]any{"status": "all"}},
		map[string]any{"id": "cov", "type": "coverage", "settings": map[string]any{"project_id": pid}},
	}}, 200)
	dataOf(t, home, "mw")
	dataOf(t, home, "dis")
	dataOf(t, home, "cov")
	many := []any{}
	for i := 0; i <= MaxWidgetRequests; i++ {
		many = append(many, map[string]any{"id": "n", "type": "note"})
	}
	a.m("POST", "/api/v1/dashboards/data", map[string]any{"widgets": many}, 400)
	a.m("POST", "/api/v1/dashboards/data", nil, 400)
}

// TestDashboardWidgetPermissions: a widget shows only what its viewer may
// see on the API: no requirements without requirement.read, nothing of a
// restricted project.
func TestDashboardWidgetPermissions(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("PRM")
	pp := "/api/v1/projects/" + pid
	a.m("POST", pp+"/requirements", map[string]any{"title": "Secret"}, 201)
	a.m("POST", "/api/v1/roles", map[string]any{"name": "auditor", "permissions": []string{"project.read"}}, 201)
	e.member("aud@example.com", "auditor")
	au := e.login("aud@example.com")
	req := map[string]any{"widgets": []any{
		map[string]any{"id": "q", "type": "saved_query", "project_id": pid},
		map[string]any{"id": "k", "type": "key_figures", "project_id": pid},
		map[string]any{"id": "g", "type": "traceability_gaps", "project_id": pid},
	}}
	res := au.m("POST", "/api/v1/dashboards/data", req, 200)
	if e := errOf(t, res, "q"); e["status"].(float64) != 403 {
		t.Fatal(e)
	}
	errOf(t, res, "g")
	dataOf(t, res, "k") // project.read: the dashboard's figures, as on the fixed dashboard

	// a restricted project: not found for someone outside it
	a.m("PUT", pp+"/access", map[string]any{"access": "restricted"}, 200)
	e.member("olga@example.com", rbac.RoleManager)
	o := e.login("olga@example.com")
	res = o.m("POST", "/api/v1/dashboards/data", req, 200)
	for _, id := range []string{"q", "k", "g"} {
		if e := errOf(t, res, id); e["status"].(float64) != 404 {
			t.Fatal(id, e)
		}
	}
	o.m("GET", pp+"/dashboards", nil, 404)
	// the admin still sees everything
	res = a.m("POST", "/api/v1/dashboards/data", req, 200)
	if d := dataOf(t, res, "q"); d["total"].(float64) != 1 {
		t.Fatal(d)
	}
}

// TestWidgetRegistryInSync: the client's widget registry
// (web/src/lib/widgets.ts) has the types of the server's.
func TestWidgetRegistryInSync(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "web", "src", "lib", "widgets.ts"))
	if err != nil {
		t.Skip("web sources not available:", err)
	}
	var client []string
	for _, m := range regexp.MustCompile(`\{ type: '([a-z_]+)'|type: '([a-z_]+)',\n`).FindAllStringSubmatch(string(src), -1) {
		client = append(client, m[1]+m[2])
	}
	var server []string
	for k := range widgetTypes {
		server = append(server, k)
	}
	slices.Sort(client)
	slices.Sort(server)
	if !slices.Equal(client, server) {
		t.Fatalf("client %v\nserver %v", client, server)
	}
	// and the breakdown dimensions and query entities
	for _, want := range append(BreakdownDimensions(), SavedQueryEntities()...) {
		if !strings.Contains(string(src), "'"+want+"'") {
			t.Errorf("web/src/lib/widgets.ts lacks %s", want)
		}
	}
}
