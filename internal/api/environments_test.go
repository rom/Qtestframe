package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

func TestEnvironments(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	lead := e.member("lead@example.com", rbac.RoleTestLead)
	tester := e.member("tester@example.com", rbac.RoleTester)
	e.member("viewer@example.com", rbac.RoleViewer)
	lc, tc, vc := e.login("lead@example.com"), e.login("tester@example.com"), e.login("viewer@example.com")
	pid := a.project("ENV")
	pp := "/api/v1/projects/" + pid
	cfg := a.m("POST", pp+"/configurations", map[string]any{"name": "Chrome"}, 201)["id"].(string)

	// a tester may not manage environments; a test lead may
	tc.m("POST", pp+"/environments", map[string]any{"name": "No"}, 403)
	env := lc.m("POST", pp+"/environments", map[string]any{"name": "Staging", "type": "staging", "urls": []string{"https://staging.example.org"},
		"components": []map[string]string{{"name": "app", "version": "2.3.1"}, {"name": "DB", "version": "16"}}, "capacity": 1,
		"configuration_ids": []string{cfg}, "tags": []string{"web"}, "credentials_ref": "vault: kv/qa/staging", "secret": "pa55-word-x"}, 201)
	eid := env["id"].(string)
	if env["owner_id"] != lead.ID || env["has_secret"] != true || env["secret"] != nil || env["shared"] != false {
		t.Fatal(env)
	}
	lc.m("POST", pp+"/environments", map[string]any{"name": "staging"}, 409)
	lc.m("POST", pp+"/environments", map[string]any{"name": "X", "type": "rocket"}, 400)
	lc.m("POST", pp+"/environments", map[string]any{"name": "X", "owner_id": "nobody"}, 400)
	// a shared environment needs environment.manage in the organisation
	shared := lc.m("POST", pp+"/environments", map[string]any{"name": "Device lab", "type": "device", "shared": true, "requires_approval": true, "owner_id": lead.ID}, 201)
	sid := shared["id"].(string)
	if shared["shared"] != true || shared["project_id"] != "" {
		t.Fatal(shared)
	}
	list := items(vc.m("GET", pp+"/environments", nil, 200))
	if len(list) != 2 {
		t.Fatal(list)
	}
	if l := items(vc.m("GET", pp+"/environments?q=component:app", nil, 200)); len(l) != 1 || l[0]["id"] != eid {
		t.Fatal(l)
	}
	if l := items(vc.m("GET", pp+"/environments?q=shared:true", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	vc.m("GET", pp+"/environments?q=bogus:", nil, 400)
	if l := items(vc.m("GET", "/api/v1/environments", nil, 200)); len(l) != 1 || l[0]["id"] != sid {
		t.Fatal(l)
	}
	if g := vc.m("GET", "/api/v1/environments/"+sid, nil, 200); g["name"] != "Device lab" {
		t.Fatal(g)
	}

	// update; the secret is never returned and revealed only to managers, with an audit entry
	vc.m("PATCH", "/api/v1/environments/"+eid, map[string]any{"status": "maintenance"}, 403)
	up := lc.m("PATCH", "/api/v1/environments/"+eid, map[string]any{"status": "maintenance", "status_note": "DB upgrade",
		"components": []map[string]string{{"name": "app", "version": "2.4.0"}, {"name": "DB", "version": "16"}}}, 200)
	if up["status"] != "maintenance" || up["version"].(float64) != 2 || up["has_secret"] != true {
		t.Fatal(up)
	}
	vc.m("POST", "/api/v1/environments/"+eid+"/secret", nil, 403)
	if s := lc.m("POST", "/api/v1/environments/"+eid+"/secret", nil, 200); s["secret"] != "pa55-word-x" {
		t.Fatal(s)
	}
	lc.m("POST", "/api/v1/environments/"+sid+"/secret", nil, 404) // none stored
	var n int
	if err := e.st.DB.QueryRowContext(context.Background(), e.st.DB.Rebind(`SELECT COUNT(*) FROM audit_log WHERE action = 'environment.secret_reveal'`)).Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	var stored string
	_ = e.st.DB.QueryRowContext(context.Background(), e.st.DB.Rebind(`SELECT secret FROM environments WHERE id = ?`), eid).Scan(&stored)
	if !strings.HasPrefix(stored, "enc:v1:") {
		t.Fatalf("secret stored as %q", stored)
	}
	if h := items(vc.m("GET", "/api/v1/environments/"+eid+"/history", nil, 200)); len(h) != 2 {
		t.Fatal(h)
	}
	lc.m("PATCH", "/api/v1/environments/"+eid, map[string]any{"status": "available"}, 200)

	// a run selects the environment (by name) and books it at once
	t1 := a.m("POST", pp+"/tests", map[string]any{"title": "Login"}, 201)["id"].(string)
	run := tc.m("POST", pp+"/runs", map[string]any{"name": "R1", "test_ids": []string{t1}, "environment_id": "staging",
		"booking": map[string]any{"starts_at": "2030-01-01T08:00:00Z", "ends_at": "2030-01-01T12:00:00Z", "purpose": "regression"}}, 201)
	rid := run["id"].(string)
	if run["environment_id"] != eid || run["environment"] != "Staging" {
		t.Fatal(run)
	}
	bk := items(tc.m("GET", "/api/v1/runs/"+rid+"/bookings", nil, 200))
	if len(bk) != 1 || bk[0]["status"] != "approved" || bk[0]["run_name"] != "R1" {
		t.Fatal(bk)
	}
	// a second booking at the same time does not fit (capacity 1): nothing is created
	res := tc.raw("POST", pp+"/runs", map[string]any{"name": "R2", "test_ids": []string{t1}, "environment_id": eid,
		"booking": map[string]any{"starts_at": "2030-01-01T10:00:00Z", "ends_at": "2030-01-01T11:00:00Z"}})
	if res.StatusCode != http.StatusConflict {
		t.Fatal(res.StatusCode)
	}
	res.Body.Close()
	if l := items(tc.m("GET", pp+"/runs?q=name:R2", nil, 200)); len(l) != 0 {
		t.Fatal("run created despite the conflict", l)
	}
	conflict := tc.raw("POST", "/api/v1/environments/"+eid+"/bookings", map[string]any{"starts_at": "2030-01-01T11:00:00Z", "ends_at": "2030-01-01T13:00:00Z"})
	var cb map[string]any
	if err := json.NewDecoder(conflict.Body).Decode(&cb); err != nil {
		t.Fatal(err)
	}
	conflict.Body.Close()
	if cb["code"] != "booking_conflict" || cb["capacity"].(float64) != 1 || len(cb["overlapping"].([]any)) != 1 {
		t.Fatal(cb)
	}
	vc.m("POST", "/api/v1/environments/"+eid+"/bookings", map[string]any{"starts_at": "2030-01-02T11:00:00Z", "ends_at": "2030-01-02T13:00:00Z"}, 403)
	// the run's environment can change by free text (linked by name) and be cleared
	if r := tc.m("PATCH", "/api/v1/runs/"+rid, map[string]any{"environment": "device lab"}, 200); r["environment_id"] != sid {
		t.Fatal(r)
	}
	if r := tc.m("PATCH", "/api/v1/runs/"+rid, map[string]any{"environment_id": ""}, 200); r["environment_id"] != nil || r["environment"] != "" {
		t.Fatal(r)
	}
	tc.m("PATCH", "/api/v1/runs/"+rid, map[string]any{"environment_id": "nope"}, 400)
	tc.m("PATCH", "/api/v1/runs/"+rid, map[string]any{"environment_id": eid}, 200)
	// a result records the environment and its versions
	ex := items(tc.m("GET", "/api/v1/runs/"+rid+"/executions", nil, 200))[0]
	tc.m("PATCH", "/api/v1/executions/"+ex["id"].(string), map[string]any{"status": "passed"}, 200)
	got := tc.m("GET", "/api/v1/executions/"+ex["id"].(string), nil, 200)
	if got["environment_name"] != "Staging" || len(got["environment_components"].([]any)) != 2 {
		t.Fatal(got)
	}
	// the release shows the tested environments (and its report)
	rel := a.m("POST", pp+"/releases", map[string]any{"name": "1.0"}, 201)["id"].(string)
	tc.m("PATCH", "/api/v1/runs/"+rid, map[string]any{"release_id": rel}, 200)
	renv := items(vc.m("GET", "/api/v1/releases/"+rel+"/environments", nil, 200))
	if len(renv) != 1 || renv[0]["name"] != "Staging" || renv[0]["passed"].(float64) != 1 {
		t.Fatal(renv)
	}
	_, data := a.download(pp+"/reports/generate", map[string]any{"template": "release", "format": "md", "params": map[string]string{"release_id": rel}}, 200)
	if !strings.Contains(string(data), "Tested environments") || !strings.Contains(string(data), "app 2.4.0, DB 16") {
		t.Fatal(string(data))
	}

	// a shared environment that needs approval: pending, then approved by its owner
	cy := lc.m("POST", pp+"/cycles", map[string]any{"name": "Sprint", "environment_id": sid,
		"booking": map[string]any{"starts_at": "2030-02-01T08:00:00Z", "ends_at": "2030-02-01T10:00:00Z", "purpose": "cycle"}}, 201)
	if cy["environment_id"] != sid || cy["environment"] != "Device lab" {
		t.Fatal(cy)
	}
	// the owner booked it: no approval needed
	if bs := items(lc.m("GET", "/api/v1/cycles/"+cy["id"].(string)+"/bookings", nil, 200)); len(bs) != 1 || bs[0]["status"] != "approved" {
		t.Fatal(bs)
	}
	xs := tc.m("POST", pp+"/exploratory-sessions", map[string]any{"title": "Explore", "environment_id": "Device lab",
		"booking": map[string]any{"starts_at": "2030-02-02T08:00:00Z", "ends_at": "2030-02-02T10:00:00Z"}}, 201)
	if xs["environment_name"] != "Device lab" {
		t.Fatal(xs)
	}
	pend := items(tc.m("GET", "/api/v1/exploratory-sessions/"+xs["id"].(string)+"/bookings", nil, 200))
	if len(pend) != 1 || pend[0]["status"] != "pending" || pend[0]["session_key"] == "" {
		t.Fatal(pend)
	}
	bid := pend[0]["id"].(string)
	// My work: the owner approves, the tester waits
	mw := items(lc.m("GET", "/api/v1/my-work?source=environments", nil, 200))
	if len(mw) != 1 || mw[0]["kind"] != model.KindBooking || mw[0]["reasons"].([]any)[0] != "booking_approval" {
		t.Fatal(mw)
	}
	if mw := items(tc.m("GET", "/api/v1/my-work?source=environments", nil, 200)); len(mw) != 1 || mw[0]["reasons"].([]any)[0] != "awaiting_approval" {
		t.Fatal(mw)
	}
	// notifications to the approver
	if ns := items(lc.m("GET", "/api/v1/notifications", nil, 200)); len(ns) == 0 || ns[0]["event"] != "environment.booking_requested" {
		t.Fatal(ns)
	}
	tc.m("POST", "/api/v1/bookings/"+bid+"/decision", map[string]any{"decision": "approved"}, 403)
	lc.m("POST", "/api/v1/bookings/"+bid+"/decision", map[string]any{"decision": "maybe"}, 400)
	dec := lc.m("POST", "/api/v1/bookings/"+bid+"/decision", map[string]any{"decision": "approved", "note": "ok"}, 200)
	if dec["status"] != "approved" || dec["decided_by"] != "lead@example.com" {
		t.Fatal(dec)
	}
	lc.m("POST", "/api/v1/bookings/"+bid+"/decision", map[string]any{"decision": "rejected"}, 409)
	if ns := items(tc.m("GET", "/api/v1/notifications", nil, 200)); len(ns) == 0 || ns[0]["event"] != "environment.booking_approved" {
		t.Fatal(ns)
	}
	// the booker moves it (needs approval again) and cancels it
	vc.m("PATCH", "/api/v1/bookings/"+bid, map[string]any{"purpose": "x"}, 403)
	mv := tc.m("PATCH", "/api/v1/bookings/"+bid, map[string]any{"starts_at": "2030-02-02T09:00:00Z", "ends_at": "2030-02-02T11:00:00Z"}, 200)
	if mv["status"] != "pending" {
		t.Fatal(mv)
	}
	if g := tc.m("GET", "/api/v1/bookings/"+bid, nil, 200); g["purpose"] != "" || g["environment_name"] != "Device lab" {
		t.Fatal(g)
	}
	vc.m("POST", "/api/v1/bookings/"+bid+"/cancel", nil, 403)
	if c := tc.m("POST", "/api/v1/bookings/"+bid+"/cancel", nil, 200); c["status"] != "cancelled" {
		t.Fatal(c)
	}
	tc.m("POST", "/api/v1/bookings/"+bid+"/cancel", nil, 409)
	// the calendar of the project
	cal := items(vc.m("GET", pp+"/bookings?from=2030-01-01&to=2030-03-01", nil, 200))
	if len(cal) != 3 {
		t.Fatal(cal)
	}
	if l := items(vc.m("GET", "/api/v1/environments/"+sid+"/bookings?status=approved", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	vc.m("GET", pp+"/bookings?from=tomorrow", nil, 400)

	// health check: a failing URL marks the environment broken, a passing one repairs it
	var up200 atomic.Bool
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if up200.Load() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer hs.Close()
	e.srv.HTTPClient = hs.Client()
	lc.m("POST", "/api/v1/environments/"+eid+"/check", nil, 400) // no URL yet
	lc.m("PATCH", "/api/v1/environments/"+eid, map[string]any{"health_url": "ftp://x"}, 400)
	lc.m("PATCH", "/api/v1/environments/"+eid, map[string]any{"health_url": hs.URL + "/health", "health_interval_minutes": 1}, 200)
	if n, err := e.srv.RunEnvironmentHealthChecks(context.Background()); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	g := vc.m("GET", "/api/v1/environments/"+eid, nil, 200)
	if g["status"] != "broken" || g["health_status"] != "down" || g["health_error"] != "HTTP 503" || g["status_by_health"] != true {
		t.Fatal(g)
	}
	if ns := items(lc.m("GET", "/api/v1/notifications", nil, 200)); ns[0]["event"] != "environment.status_changed" {
		t.Fatal(ns[0])
	}
	up200.Store(true)
	if g := lc.m("POST", "/api/v1/environments/"+eid+"/check", nil, 200); g["status"] != "available" || g["health_status"] != "up" {
		t.Fatal(g)
	}
	tc.m("POST", "/api/v1/environments/"+eid+"/check", nil, 403)

	// deleting keeps the results' snapshot
	tc.m("DELETE", "/api/v1/environments/"+eid, nil, 403)
	lc.m("DELETE", "/api/v1/environments/"+eid, nil, 204)
	lc.m("GET", "/api/v1/environments/"+eid, nil, 404)
	if got := tc.m("GET", "/api/v1/executions/"+ex["id"].(string), nil, 200); got["environment_name"] != "Staging" || got["environment_id"] != nil {
		t.Fatal(got)
	}
	_ = tester
}

func TestTestDataSets(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	owner := e.member("owner@example.com", rbac.RoleTester)
	e.member("viewer@example.com", rbac.RoleViewer)
	oc, vc := e.login("owner@example.com"), e.login("viewer@example.com")
	pid := a.project("TDS")
	pp := "/api/v1/projects/" + pid

	vc.m("POST", pp+"/test-data-sets", map[string]any{"name": "No"}, 403)
	ds := oc.m("POST", pp+"/test-data-sets", map[string]any{"name": "Users", "classification": "anonymised", "parameters": []string{"user"},
		"data_rows": []map[string]any{{"name": "a", "values": map[string]string{"user": "ann"}}, {"values": map[string]string{"user": "bob"}}}, "tags": []string{"login"}}, 201)
	did := ds["id"].(string)
	if ds["owner_id"] != owner.ID || ds["version"].(float64) != 1 || ds["kind"] != "table" {
		t.Fatal(ds)
	}
	oc.m("POST", pp+"/test-data-sets", map[string]any{"name": "users"}, 409)
	oc.m("POST", pp+"/test-data-sets", map[string]any{"name": "X", "classification": "top secret"}, 400)
	prod := oc.m("POST", pp+"/test-data-sets", map[string]any{"name": "Prod copy", "kind": "files", "classification": "production_copy", "expires_on": "2000-01-01"}, 201)
	if prod["expired"] != true {
		t.Fatal(prod)
	}
	if l := items(vc.m("GET", pp+"/test-data-sets?q=classification:production_copy", nil, 200)); len(l) != 1 || l[0]["name"] != "Prod copy" {
		t.Fatal(l)
	}
	if l := items(vc.m("GET", pp+"/test-data-sets?q=expired:true", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	vc.m("GET", pp+"/test-data-sets?q=(", nil, 400)
	// a data-driven test takes its rows from the set; a run records the use
	tc := a.m("POST", pp+"/tests", map[string]any{"title": "Login", "parameters": []string{"user"}, "data_rows": []map[string]any{{"values": map[string]string{"user": "inline"}}},
		"steps": []map[string]any{{"action": "Sign in as {{user}}", "expected": "ok"}}}, 201)
	tid := tc["id"].(string)
	vc.m("POST", "/api/v1/test-data-sets/"+did+"/links", map[string]any{"item_type": "test", "item_id": tid, "usage": "source"}, 403)
	oc.m("POST", "/api/v1/test-data-sets/"+did+"/links", map[string]any{"item_type": "test", "item_id": "nope"}, 400)
	l := oc.m("POST", "/api/v1/test-data-sets/"+did+"/links", map[string]any{"item_type": "test", "item_id": tid, "usage": "source"}, 200)
	if len(l["links"].([]any)) != 1 {
		t.Fatal(l)
	}
	if links := items(vc.m("GET", "/api/v1/tests/"+tid+"/data-sets", nil, 200)); len(links) != 1 || links[0]["data_set_name"] != "Users" {
		t.Fatal(links)
	}
	run := a.m("POST", pp+"/runs", map[string]any{"name": "R", "test_ids": []string{tid}}, 201)
	ex := items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200))[0]
	full := a.m("GET", "/api/v1/executions/"+ex["id"].(string), nil, 200)
	if its := full["iterations"].([]any); len(its) != 2 {
		t.Fatal(full["iterations"])
	}
	if u := items(vc.m("GET", "/api/v1/test-data-sets/"+did+"/usage", nil, 200)); len(u) != 1 || u[0]["run_name"] != "R" {
		t.Fatal(u)
	}
	a.m("POST", "/api/v1/test-data-sets/"+did+"/links", map[string]any{"item_type": "run", "item_id": run["id"]}, 200)
	if links := items(vc.m("GET", "/api/v1/runs/"+run["id"].(string)+"/data-sets", nil, 200)); len(links) != 1 {
		t.Fatal(links)
	}
	cfg := a.m("POST", pp+"/configurations", map[string]any{"name": "Chrome"}, 201)["id"].(string)
	a.m("POST", "/api/v1/test-data-sets/"+did+"/links", map[string]any{"item_type": "configuration", "item_id": cfg}, 200)
	if links := items(vc.m("GET", "/api/v1/configurations/"+cfg+"/data-sets", nil, 200)); len(links) != 1 {
		t.Fatal(links)
	}
	// update with a version check, history
	oc.m("PATCH", "/api/v1/test-data-sets/"+did, map[string]any{"description": "x", "version": 99}, 409)
	up := oc.m("PATCH", "/api/v1/test-data-sets/"+did, map[string]any{"description": "Accounts", "expires_on": "2099-12-31", "version": 1}, 200)
	if up["version"].(float64) != 2 {
		t.Fatal(up)
	}
	if h := items(vc.m("GET", "/api/v1/test-data-sets/"+did+"/history", nil, 200)); len(h) < 3 {
		t.Fatal(h)
	}
	// attachments of a data set
	f := oc.upload("POST", "/api/v1/test-data-sets/"+prod["id"].(string)+"/attachments", "file", "orders.csv", "text/csv", []byte("id\n1\n"), nil, 201)
	if f["id"] == nil {
		t.Fatal(f)
	}
	if g := vc.m("GET", "/api/v1/test-data-sets/"+prod["id"].(string), nil, 200); g["file_count"].(float64) != 1 {
		t.Fatal(g)
	}
	// refresh requests: asked by someone who executes, closed by the owner
	vc.m("POST", "/api/v1/test-data-sets/"+did+"/refresh-requests", map[string]any{"reason": "x"}, 403)
	e.member("runner@example.com", rbac.RoleTester)
	rc := e.login("runner@example.com")
	rc.m("POST", "/api/v1/test-data-sets/"+did+"/refresh-requests", map[string]any{}, 400)
	q := rc.m("POST", "/api/v1/test-data-sets/"+did+"/refresh-requests", map[string]any{"reason": "More roles", "needed_by": "2030-01-01"}, 201)
	if ns := items(oc.m("GET", "/api/v1/notifications", nil, 200)); len(ns) == 0 || ns[0]["event"] != "testdata.refresh_requested" {
		t.Fatal(ns)
	}
	mw := items(oc.m("GET", "/api/v1/my-work?source=test_data", nil, 200))
	reasons := map[string]bool{}
	for _, it := range mw {
		for _, r := range it["reasons"].([]any) {
			reasons[r.(string)] = true
		}
	}
	if !reasons["refresh_requested"] || !reasons["data_expired"] {
		t.Fatal(mw)
	}
	if l := items(vc.m("GET", "/api/v1/test-data-sets/"+did+"/refresh-requests", nil, 200)); len(l) != 1 {
		t.Fatal(l)
	}
	vc.m("POST", "/api/v1/test-data-refresh-requests/"+q["id"].(string)+"/resolve", map[string]any{"status": "done"}, 403)
	oc.m("POST", "/api/v1/test-data-refresh-requests/"+q["id"].(string)+"/resolve", map[string]any{"status": "later"}, 400)
	if r := oc.m("POST", "/api/v1/test-data-refresh-requests/"+q["id"].(string)+"/resolve", map[string]any{"status": "done", "note": "added"}, 200); r["status"] != "done" {
		t.Fatal(r)
	}
	if ns := items(rc.m("GET", "/api/v1/notifications", nil, 200)); len(ns) == 0 || ns[0]["event"] != "testdata.refresh_resolved" {
		t.Fatal(ns)
	}
	// unlink and delete
	links := items(vc.m("GET", "/api/v1/runs/"+run["id"].(string)+"/data-sets", nil, 200))
	oc.m("DELETE", "/api/v1/test-data-sets/"+did+"/links/"+links[0]["id"].(string), nil, 204)
	oc.m("DELETE", "/api/v1/test-data-sets/"+did+"/links/"+links[0]["id"].(string), nil, 404)
	vc.m("DELETE", "/api/v1/test-data-sets/"+did, nil, 403)
	oc.m("DELETE", "/api/v1/test-data-sets/"+did, nil, 204)
	oc.m("GET", "/api/v1/test-data-sets/"+did, nil, 404)
}
