package api

import (
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

func TestDashboardTrendsAndFlaky(t *testing.T) {
	e := newEnv(t)
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	e.st.SetClock(func() time.Time { return now })
	a := e.login("admin@example.com")
	pid := a.project("MT")
	f := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "requirement", "name": "Top"}, 201)
	sub := a.m("POST", "/api/v1/projects/"+pid+"/folders", map[string]any{"kind": "requirement", "name": "Sub", "parent_id": f["id"]}, 201)
	cat := a.m("POST", "/api/v1/projects/"+pid+"/categories", map[string]any{"scope": "requirement", "dimension": "Zone", "name": "Inner"}, 201)
	r1 := a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "R1", "folder_id": sub["id"], "category_ids": []any{cat["id"]}}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/requirements", map[string]any{"title": "R2"}, 201)
	tc := a.m("POST", "/api/v1/projects/"+pid+"/tests", map[string]any{"title": "Flappy", "execution_type": "automated"}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/links", map[string]any{"requirement_id": r1["id"], "test_case_id": tc["id"]}, 201)
	rel := a.m("POST", "/api/v1/projects/"+pid+"/releases", map[string]any{"name": "1.0"}, 201)
	a.m("POST", "/api/v1/releases/"+rel["id"].(string)+"/requirements", map[string]any{"add": []any{r1["id"]}}, 200)
	for i, st := range []string{"passed", "failed", "passed"} {
		now = now.Add(time.Hour)
		run := a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "nightly", "test_ids": []any{tc["id"]}, "release_id": rel["id"]}, 201)
		ex := items(a.m("GET", "/api/v1/runs/"+run["id"].(string)+"/executions", nil, 200))
		a.m("PATCH", "/api/v1/executions/"+ex[0]["id"].(string), map[string]any{"status": st}, 200)
		_ = i
	}
	// an open run
	a.m("POST", "/api/v1/projects/"+pid+"/runs", map[string]any{"name": "open", "test_ids": []any{tc["id"]}}, 201)
	a.m("POST", "/api/v1/projects/"+pid+"/defects", map[string]any{"title": "d", "severity": "critical"}, 201)

	dash := a.m("GET", "/api/v1/projects/"+pid+"/dashboard", nil, 200)
	if sev := dash["defects_by_severity"].(map[string]any); sev["critical"].(float64) != 1 || sev["minor"].(float64) != 0 {
		t.Error(sev)
	}
	if ar := dash["active_runs"].([]any); len(ar) != 1 || ar[0].(map[string]any)["name"] != "open" {
		t.Error(ar)
	}
	folders := dash["coverage_by_folder"].([]any)
	if len(folders) != 2 {
		t.Fatal(folders)
	}
	for _, x := range folders {
		g := x.(map[string]any)
		if g["id"] == f["id"] && (g["name"] != "Top" || g["total"].(float64) != 1 || g["coverage"].(map[string]any)["passed"].(float64) != 1) {
			t.Error("top folder", g)
		}
		if g["id"] == "" && g["total"].(float64) != 1 {
			t.Error("no folder", g)
		}
	}
	if cs := dash["coverage_by_category"].([]any); len(cs) != 1 || cs[0].(map[string]any)["dimension"] != "Zone" {
		t.Error(cs)
	}
	fl := dash["flaky"].(map[string]any)
	if fl["total"].(float64) != 1 || fl["window"].(float64) != 10 || fl["min_flips"].(float64) != 2 {
		t.Error(fl)
	}

	// tests list: flaky flag and query fields
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/tests?q=flaky:true", nil, 200)); len(l) != 1 || l[0]["flaky"] != true || l[0]["flakiness"].(float64) != 2 {
		t.Error(l)
	}
	if l := items(a.m("GET", "/api/v1/projects/"+pid+"/tests?q=flips>=3", nil, 200)); len(l) != 0 {
		t.Error(l)
	}
	ft := a.m("GET", "/api/v1/projects/"+pid+"/flaky-tests?limit=5", nil, 200)
	if l := items(ft); len(l) != 1 || l[0]["score"].(float64) != 2 || len(l[0]["results"].([]any)) != 3 {
		t.Error(ft)
	}
	a.m("GET", "/api/v1/projects/missing/flaky-tests", nil, 404)

	// trends: today's snapshot was recorded by the dashboard
	tr := a.m("GET", "/api/v1/projects/"+pid+"/metrics/trends", nil, 200)
	if tr["from"] != "2026-02-09" || tr["to"] != "2026-03-10" || tr["days"].(float64) != 30 {
		t.Error(tr)
	}
	pts := tr["points"].([]any)
	if len(pts) != 1 {
		t.Fatal(pts)
	}
	p0 := pts[0].(map[string]any)
	if p0["day"] != "2026-03-10" || p0["coverage_pct"].(float64) != 50 || p0["pass_rate"].(float64) != 100 || p0["open_defects"].(float64) != 1 || p0["passed"].(float64) != 1 {
		t.Error(p0)
	}
	if w := tr["defect_weeks"].([]any); len(w) != 5 || w[4].(map[string]any)["opened"].(float64) != 1 {
		t.Error(w)
	}
	if tr := a.m("GET", "/api/v1/projects/"+pid+"/metrics/trends?range=365", nil, 200); tr["days"].(float64) != 365 {
		t.Error(tr["days"])
	}
	if tr := a.m("GET", "/api/v1/projects/"+pid+"/metrics/trends?since=2026-03-01", nil, 200); tr["days"].(float64) != 10 || tr["from"] != "2026-03-01" {
		t.Error(tr)
	}
	if tr := a.m("GET", "/api/v1/projects/"+pid+"/metrics/trends?since=1990-01-01", nil, 200); tr["from"] == "1990-01-01" {
		t.Error(tr["from"])
	}
	a.m("GET", "/api/v1/projects/"+pid+"/metrics/trends?range=7", nil, 400)
	a.m("GET", "/api/v1/projects/"+pid+"/metrics/trends?since=2030-01-01", nil, 400)
	a.m("GET", "/api/v1/projects/"+pid+"/metrics/trends?since=x", nil, 400)
	a.m("GET", "/api/v1/projects/missing/metrics/trends", nil, 404)

	bu := a.m("GET", "/api/v1/releases/"+rel["id"].(string)+"/burnup", nil, 200)
	if p := bu["points"].([]any); len(p) != 1 || p[0].(map[string]any)["passed"].(float64) != 1 || p[0].(map[string]any)["scope"].(float64) != 1 {
		t.Error(bu)
	}
	a.m("GET", "/api/v1/releases/missing/burnup", nil, 404)

	// a viewer may read them
	e.member("viewer@example.com", "viewer")
	v := e.login("viewer@example.com")
	v.m("GET", "/api/v1/projects/"+pid+"/metrics/trends?range=90", nil, 200)
	v.m("GET", "/api/v1/projects/"+pid+"/flaky-tests", nil, 200)
}

func TestCoverageGroupsAndActiveRuns(t *testing.T) {
	root, child := "root", "child"
	pc := &projectContext{folders: []model.Folder{{ID: root, Name: "Root"}, {ID: child, Name: "Child", ParentID: &root}},
		categories: map[string]model.Category{}}
	var reqs []model.Requirement
	for i := 0; i < MaxCoverageGroups+3; i++ {
		id := string(rune('a' + i))
		pc.folders = append(pc.folders, model.Folder{ID: id, Name: id})
		pc.categories[id] = model.Category{ID: id, Name: id}
		reqs = append(reqs, model.Requirement{FolderID: &id, CategoryIDs: []string{id, "unknown"}, Coverage: "none"})
	}
	reqs = append(reqs, model.Requirement{FolderID: &child, Coverage: "passed"}, model.Requirement{FolderID: &root, Coverage: "failed"})
	fs, cs := pc.coverageGroups(reqs)
	if len(fs) != MaxCoverageGroups || fs[0].ID != root || fs[0].Total != 2 || fs[0].Coverage["passed"] != 1 || len(cs) != MaxCoverageGroups {
		t.Errorf("%+v %+v", fs, cs)
	}
	var runs []model.TestRun
	for i := 0; i < MaxActiveRuns+2; i++ {
		runs = append(runs, model.TestRun{Status: "planned"}, model.TestRun{Status: "completed"})
	}
	if got := activeRuns(runs); len(got) != MaxActiveRuns {
		t.Error(len(got))
	}
}
