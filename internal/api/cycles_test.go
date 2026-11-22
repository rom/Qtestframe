package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

func TestTestCycles(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	lead := e.member("lead@example.com", rbac.RoleTestLead)
	tester := e.member("tester@example.com", rbac.RoleTester)
	pid := a.project("CYC")
	pp := "/api/v1/projects/" + pid
	mk := func(title string, est int) (string, string) {
		m := a.m("POST", pp+"/tests", map[string]any{"title": title, "estimated_minutes": est, "tags": []string{"smoke"}}, 201)
		return m["id"].(string), m["key"].(string)
	}
	t1, k1 := mk("Login", 10)
	t2, _ := mk("Logout", 0)
	t3, _ := mk("Search", 5)
	cfgA := a.m("POST", pp+"/configurations", map[string]any{"name": "Chrome"}, 201)["id"].(string)
	cfgB := a.m("POST", pp+"/configurations", map[string]any{"name": "Firefox"}, 201)["id"].(string)

	// a past execution of t2 gives its default estimate (average duration)
	run := a.m("POST", pp+"/runs", map[string]any{"name": "Old", "test_ids": []string{t2}}, 201)
	ex := items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200))[0]
	a.m("PATCH", "/api/v1/executions/"+ex["id"].(string), map[string]any{"status": "passed", "duration_ms": 125000}, 200)

	// permissions: a tester reads, a test lead plans
	tc := e.login("tester@example.com")
	lc := e.login("lead@example.com")
	tc.m("POST", pp+"/cycles", map[string]any{"name": "No"}, 403)
	rel := a.m("POST", pp+"/releases", map[string]any{"name": "1.0"}, 201)["id"].(string)
	cy := lc.m("POST", pp+"/cycles", map[string]any{"name": "Sprint 14 regression", "goal": "Regression of sprint 14", "release_id": rel,
		"configuration_ids": []string{cfgA, cfgB}, "start_date": "2026-01-01", "end_date": "2026-01-10"}, 201)
	cid := cy["id"].(string)
	if cy["owner_id"] != lead.ID || cy["status"] != "planned" || cy["release_name"] != "1.0" {
		t.Fatal(cy)
	}
	lc.m("POST", pp+"/cycles", map[string]any{"name": "sprint 14 regression"}, 409)
	lc.m("POST", pp+"/cycles", map[string]any{"name": "X", "end_date": "2025-01-01", "start_date": "2026-01-01"}, 400)
	lc.m("POST", pp+"/cycles", map[string]any{"name": "X", "status": "active"}, 400)
	lc.m("POST", pp+"/cycles", map[string]any{"name": "X", "owner_id": "nobody"}, 400)

	// plan: by selection (every configuration), by query, per configuration
	cp := "/api/v1/cycles/" + cid
	res := lc.m("POST", cp+"/tests", map[string]any{"test_ids": []string{t1, t2}, "assignee_id": tester.ID}, 200)
	if res["added"].(float64) != 4 {
		t.Fatal(res)
	}
	res = lc.m("POST", cp+"/tests", map[string]any{"query": "title:Search", "configuration_ids": []string{cfgA}}, 200)
	if res["added"].(float64) != 1 {
		t.Fatal(res)
	}
	lc.m("POST", cp+"/tests", map[string]any{"test_ids": []string{t1}}, 200) // already planned: nothing added
	lc.m("POST", cp+"/tests", map[string]any{}, 400)
	lc.m("POST", cp+"/tests", map[string]any{"test_ids": []string{"nope"}}, 404)
	tc.m("POST", cp+"/tests", map[string]any{"test_ids": []string{t3}}, 403)
	plan := items(tc.m("GET", cp+"/tests", nil, 200))
	if len(plan) != 5 {
		t.Fatal(plan)
	}
	est := map[string]float64{}
	for _, x := range plan {
		est[x["test_case_id"].(string)] = x["estimate_minutes"].(float64)
	}
	if est[t1] != 10 || est[t2] != 3 || est[t3] != 5 {
		t.Fatal(est)
	}
	if n := len(items(tc.m("GET", cp+"/tests?assignee="+tester.ID, nil, 200))); n != 4 {
		t.Fatal(n)
	}
	if n := len(items(tc.m("GET", cp+"/tests?assignee=none", nil, 200))); n != 1 {
		t.Fatal(n)
	}
	// estimates and testers per row
	var searchRow string
	for _, x := range plan {
		if x["test_case_id"] == t3 {
			searchRow = x["id"].(string)
		}
	}
	lc.m("PATCH", cp+"/tests", map[string]any{"ids": []string{searchRow}, "estimate_minutes": 7, "assignee_id": lead.ID}, 200)
	lc.m("PATCH", cp+"/tests", map[string]any{"ids": []string{searchRow}}, 400)
	lc.m("PATCH", cp+"/tests", map[string]any{"all": true, "estimate_minutes": -1}, 400)
	got := lc.m("GET", cp, nil, 200)["progress"].(map[string]any)
	if got["planned"].(float64) != 5 || got["estimated_minutes"].(float64) != 2*10+2*3+7 || got["unassigned"].(float64) != 0 || got["not_in_run"].(float64) != 5 {
		t.Fatal(got)
	}

	// starting the cycle creates its run with the planned executions, assigned
	cy = lc.m("PATCH", cp, map[string]any{"status": "active"}, 200)
	if len(cy["run_ids"].([]any)) != 1 || cy["started_at"] == nil {
		t.Fatal(cy)
	}
	runID := cy["run_ids"].([]any)[0].(string)
	execs := items(a.m("GET", "/api/v1/runs/"+runID+"/executions", nil, 200))
	if len(execs) != 5 {
		t.Fatal(len(execs))
	}
	r := a.m("GET", "/api/v1/runs/"+runID, nil, 200)
	if r["cycle_id"] != cid || r["cycle_name"] != "Sprint 14 regression" || r["release_id"] != rel {
		t.Fatal(r)
	}
	if n := len(items(a.m("GET", pp+"/runs?q=cycle:sprint", nil, 200))); n != 1 {
		t.Fatal(n)
	}
	if n := len(items(a.m("GET", pp+"/tests?q=cycle:%22Sprint+14%22", nil, 200))); n != 3 {
		t.Fatal(n)
	}
	assigned := 0
	for _, x := range execs {
		if x["assignee"] == "tester@example.com" {
			assigned++
		}
	}
	if assigned != 4 {
		t.Fatal(execs)
	}
	// nothing new to run
	lc.m("POST", cp+"/runs", nil, 400)

	// my work: the tester's unexecuted cycle tests
	work, err := e.st.AssignedCycleTests(context.Background(), e.tenant.ID, tester.ID)
	if err != nil || len(work) != 4 || work[0].CycleName != "Sprint 14 regression" || work[0].ProjectKey != "CYC" {
		t.Fatal(err, work)
	}

	// results: two passed, one failed
	for i, x := range execs {
		st := []string{"passed", "passed", "failed"}
		if i < len(st) {
			tc.m("PATCH", "/api/v1/executions/"+x["id"].(string), map[string]any{"status": st[i], "duration_ms": 60000}, 200)
		}
	}
	prog := tc.m("GET", cp+"/progress", nil, 200)
	p := prog["cycle"].(map[string]any)["progress"].(map[string]any)
	if p["executed"].(float64) != 3 || p["passed"].(float64) != 2 || p["failed"].(float64) != 1 || p["percent_executed"].(float64) != 60 || p["actual_ms"].(float64) != 180000 {
		t.Fatal(p)
	}
	testers := prog["testers"].([]any)
	if len(testers) != 2 || testers[0].(map[string]any)["assignee_id"] != tester.ID {
		t.Fatal(testers)
	}
	burn := prog["burndown"].([]any)
	if len(burn) != 10 || burn[0].(map[string]any)["ideal"].(float64) != 5 || burn[9].(map[string]any)["ideal"].(float64) != 0 {
		t.Fatal(burn)
	}
	work, _ = e.st.AssignedCycleTests(context.Background(), e.tenant.ID, tester.ID)
	if len(work) != 1 {
		t.Fatal(work)
	}

	// planning more tests and creating the next run
	t4, _ := mk("Profile", 0)
	lc.m("POST", cp+"/tests", map[string]any{"tests": []map[string]any{{"test_id": t4, "configuration_id": cfgB, "estimate_minutes": 4}}}, 200)
	run2 := lc.m("POST", cp+"/runs", nil, 201)
	if run2["name"] != "Sprint 14 regression (2)" || run2["summary"].(map[string]any)["total"].(float64) != 1 {
		t.Fatal(run2)
	}

	// release exit criterion: the cycle fully executed
	a.m("PATCH", "/api/v1/releases/"+rel, map[string]any{"exit_criteria": []map[string]any{{"type": "cycle_executed"}}}, 400)
	a.m("PATCH", "/api/v1/releases/"+rel, map[string]any{"exit_criteria": []map[string]any{{"type": "cycle_executed", "cycle_id": "nope"}}}, 400)
	a.m("PATCH", "/api/v1/releases/"+rel, map[string]any{"exit_criteria": []map[string]any{{"type": "cycle_executed", "cycle_id": cid}}}, 200)
	st := a.m("GET", "/api/v1/releases/"+rel+"/status", nil, 200)
	crit := st["criteria"].([]any)[0].(map[string]any)
	if crit["met"] != false || crit["threshold"].(float64) != 100 || crit["actual"].(float64) != 50 || len(crit["items"].([]any)) != 3 {
		t.Fatal(crit)
	}
	_, data := a.download(pp+"/reports/generate", map[string]any{"template": "release", "format": "md", "params": map[string]string{"release_id": rel}}, 200)
	if !strings.Contains(string(data), "Test cycle executed") {
		t.Fatal(string(data))
	}

	// the cycle report
	_, data = a.download(pp+"/reports/generate", map[string]any{"template": "cycle", "format": "md", "params": map[string]string{"cycle_id": cid}}, 200)
	if !strings.Contains(string(data), "Sprint 14 regression") || !strings.Contains(string(data), "Workload per tester") || !strings.Contains(string(data), k1) {
		t.Fatal(string(data))
	}
	_, data = a.download(pp+"/reports/generate", map[string]any{"template": "cycle", "format": "md"}, 200)
	if !strings.Contains(string(data), "Sprint 14 regression") {
		t.Fatal(string(data))
	}
	a.m("POST", pp+"/reports/generate", map[string]any{"template": "cycle", "format": "md", "params": map[string]string{"cycle_id": "nope"}}, 400)

	// copy of a previous cycle
	cy2 := lc.m("POST", pp+"/cycles", map[string]any{"name": "Sprint 15 regression", "copy_from": cid}, 201)
	if cy2["progress"].(map[string]any)["planned"].(float64) != 6 || cy2["release_id"] != rel || len(cy2["configuration_ids"].([]any)) != 2 {
		t.Fatal(cy2)
	}
	cy3 := lc.m("POST", pp+"/cycles", map[string]any{"name": "Without configurations"}, 201)
	res = lc.m("POST", "/api/v1/cycles/"+cy3["id"].(string)+"/tests", map[string]any{"copy_from": cid}, 200)
	if res["added"].(float64) != 4 {
		t.Fatal(res) // the four tests, without configuration
	}

	// list and query
	list := items(tc.m("GET", pp+"/cycles", nil, 200))
	if len(list) != 3 || list[0]["id"] != cid {
		t.Fatal(list)
	}
	if n := len(items(tc.m("GET", pp+"/cycles?q=assignee:tester%40example.com+status:planned", nil, 200))); n != 2 {
		t.Fatal(n)
	}
	if n := len(items(tc.m("GET", pp+"/cycles?q=test:"+k1+"+progress>=50", nil, 200))); n != 1 {
		t.Fatal(n)
	}
	tc.m("GET", pp+"/cycles?q=nofield:x", nil, 400)

	// history and audit
	hist := items(tc.m("GET", cp+"/history", nil, 200))
	if len(hist) < 4 {
		t.Fatal(hist)
	}
	// remove rows, close, delete
	lc.m("DELETE", cp+"/tests", map[string]any{"ids": []string{searchRow}}, 200)
	lc.m("DELETE", cp+"/tests", map[string]any{}, 400)
	cy = lc.m("PATCH", cp, map[string]any{"status": "closed"}, 200)
	if cy["closed_at"] == nil {
		t.Fatal(cy)
	}
	lc.m("POST", cp+"/tests", map[string]any{"test_ids": []string{t3}}, 409)
	lc.m("POST", cp+"/runs", nil, 409)
	lc.m("DELETE", "/api/v1/cycles/"+cy3["id"].(string), nil, 204)
	tc.m("GET", "/api/v1/cycles/"+cy3["id"].(string), nil, 404)

	// another tenant sees nothing
	var n int
	if err := e.st.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM audit_log WHERE action LIKE 'cycle.%'`).Scan(&n); err != nil || n < 8 {
		t.Fatal(err, n)
	}
}

func TestCycleSchedules(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	pid := a.project("SCH")
	pp := "/api/v1/projects/" + pid
	t1 := a.m("POST", pp+"/tests", map[string]any{"title": "Nightly check", "tags": []string{"nightly"}}, 201)["id"].(string)
	tpl := a.m("POST", pp+"/cycles", map[string]any{"name": "Nightly template", "goal": "Nightly"}, 201)["id"].(string)
	a.m("POST", "/api/v1/cycles/"+tpl+"/tests", map[string]any{"test_ids": []string{t1}, "assignee_id": e.admin.ID}, 200)

	a.m("POST", pp+"/cycle-schedules", map[string]any{"name_pattern": "Nightly {date}"}, 400) // no template, no query
	a.m("POST", pp+"/cycle-schedules", map[string]any{"name_pattern": "Nightly {date}", "query": "tag:(", "frequency": "daily"}, 400)
	a.m("POST", pp+"/cycle-schedules", map[string]any{"name_pattern": "Nightly {date}", "template_cycle_id": tpl, "frequency": "hourly"}, 400)
	x := a.m("POST", pp+"/cycle-schedules", map[string]any{"name_pattern": "Nightly {date}", "template_cycle_id": tpl, "frequency": "daily", "time": "02:00",
		"timezone": "Europe/Stockholm", "duration_days": 2, "auto_start": true}, 201)
	sid := x["id"].(string)
	if x["next_run_at"] == nil || x["template_name"] != "Nightly template" || x["owner_id"] != e.admin.ID {
		t.Fatal(x)
	}
	if n := len(items(a.m("GET", pp+"/cycle-schedules", nil, 200))); n != 1 {
		t.Fatal(n)
	}
	// run now: a cycle from the template, started (its run created)
	cy := a.m("POST", "/api/v1/cycle-schedules/"+sid+"/run", nil, 201)
	if !strings.HasPrefix(cy["name"].(string), "Nightly 20") || cy["status"] != "active" || len(cy["run_ids"].([]any)) != 1 || cy["schedule_id"] != sid || cy["goal"] != "Nightly" {
		t.Fatal(cy)
	}
	// the schedule fires when due (the background job); same day: a new name
	if _, err := e.st.DB.ExecContext(context.Background(), e.st.DB.Rebind(`UPDATE test_cycle_schedules SET next_run_at = ? WHERE id = ?`), time.Now().UTC().Add(-time.Minute), sid); err != nil {
		t.Fatal(err)
	}
	n, err := e.srv.RunCycleJobs(context.Background())
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	x = a.m("GET", pp+"/cycle-schedules", nil, 200)["items"].([]any)[0].(map[string]any)
	if x["runs"].(float64) != 2 || x["last_error"] != "" || x["last_cycle_id"] == nil {
		t.Fatal(x)
	}
	list := items(a.m("GET", pp+"/cycles", nil, 200))
	if len(list) != 3 {
		t.Fatal(list)
	}
	// a schedule by query
	x2 := a.m("POST", pp+"/cycle-schedules", map[string]any{"name_pattern": "Weekly", "query": "tag:nightly", "frequency": "weekly", "weekday": 3}, 201)
	cy = a.m("POST", "/api/v1/cycle-schedules/"+x2["id"].(string)+"/run", nil, 201)
	if cy["status"] != "planned" || cy["progress"].(map[string]any)["planned"].(float64) != 1 {
		t.Fatal(cy)
	}
	a.m("PATCH", "/api/v1/cycle-schedules/"+x2["id"].(string), map[string]any{"enabled": false}, 200)
	x2 = a.m("GET", pp+"/cycle-schedules", nil, 200)["items"].([]any)[1].(map[string]any)
	if x2["enabled"] != false || x2["next_run_at"] != nil {
		t.Fatal(x2)
	}
	a.m("PATCH", "/api/v1/cycle-schedules/"+x2["id"].(string), map[string]any{"duration_days": 0, "frequency": "monthly", "day_of_month": 40}, 400)
	a.m("DELETE", "/api/v1/cycle-schedules/"+x2["id"].(string), nil, 204)
	a.m("DELETE", "/api/v1/cycle-schedules/"+x2["id"].(string), nil, 404)

	// a one-off scheduled start
	when := time.Now().UTC().Add(time.Hour)
	one := a.m("POST", pp+"/cycles", map[string]any{"name": "Later", "scheduled_start_at": when}, 201)
	a.m("POST", "/api/v1/cycles/"+one["id"].(string)+"/tests", map[string]any{"test_ids": []string{t1}}, 200)
	if n, _ := e.srv.RunCycleJobs(context.Background()); n != 0 {
		t.Fatal(n)
	}
	if _, err := e.st.DB.ExecContext(context.Background(), e.st.DB.Rebind(`UPDATE test_cycles SET scheduled_start_at = ? WHERE id = ?`), time.Now().UTC().Add(-time.Minute), one["id"]); err != nil {
		t.Fatal(err)
	}
	if n, err := e.srv.RunCycleJobs(context.Background()); n != 1 || err != nil {
		t.Fatal(n, err)
	}
	one = a.m("GET", "/api/v1/cycles/"+one["id"].(string), nil, 200)
	if one["status"] != model.CycleActive || len(one["run_ids"].([]any)) != 1 || one["scheduled_start_at"] != nil {
		t.Fatal(one)
	}
	// a scheduled start whose owner may no longer plan is dropped
	viewer := e.member("viewer@example.com", rbac.RoleViewer)
	bad := a.m("POST", pp+"/cycles", map[string]any{"name": "Bad", "owner_id": viewer.ID, "scheduled_start_at": when}, 201)
	if _, err := e.st.DB.ExecContext(context.Background(), e.st.DB.Rebind(`UPDATE test_cycles SET scheduled_start_at = ? WHERE id = ?`), time.Now().UTC().Add(-time.Minute), bad["id"]); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.srv.RunCycleJobs(context.Background()); n != 0 {
		t.Fatal(n)
	}
	bad = a.m("GET", "/api/v1/cycles/"+bad["id"].(string), nil, 200)
	if bad["status"] != "planned" || bad["scheduled_start_at"] != nil {
		t.Fatal(bad)
	}
	// deleting the template keeps the schedule (without template)
	a.m("DELETE", "/api/v1/cycles/"+tpl, nil, 204)
	x = a.m("GET", pp+"/cycle-schedules", nil, 200)["items"].([]any)[0].(map[string]any)
	if x["template_cycle_id"] != nil {
		t.Fatal(x)
	}
}
