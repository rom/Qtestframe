package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// metricsFixture builds a project with ten days of history (2026-03-01 to
// 2026-03-10, now = 2026-03-10 12:00 UTC):
//
//	requirements A, B (03-01); tests T1, T2 (03-01); links A–T1 (03-02), B–T2 (03-05)
//	run 1 (03-03): T1 passed, T2 failed
//	run 2 (03-06): T1 failed, T2 passed
//	run 3 (03-08, release R): T1 passed, T2 passed
//	defect D1 major 03-04, resolved 03-07; D2 critical 03-08, open; risk open (high)
type metricsFixture struct {
	st             *store.Store
	ctx            context.Context
	ten            string
	p              *model.Project
	a, b           *model.Requirement
	t1, t2         *model.TestCase
	rel            *model.Release
	now            *time.Time
	flipT1, lastT1 time.Time
}

func at(day, hour int) time.Time { return time.Date(2026, 3, day, hour, 0, 0, 0, time.UTC) }

func newMetricsFixture(t *testing.T) *metricsFixture {
	st, ten, ctx := setup(t)
	now := at(1, 9)
	st.SetClock(func() time.Time { return now })
	f := &metricsFixture{st: st, ctx: ctx, ten: ten.ID, now: &now}
	f.p = &model.Project{Key: "MX", Name: "Metrics"}
	must(t, st.CreateProject(ctx, ten.ID, f.p, false))
	f.a = &model.Requirement{ProjectID: f.p.ID, Title: "A"}
	f.b = &model.Requirement{ProjectID: f.p.ID, Title: "B"}
	must(t, st.CreateRequirement(ctx, ten.ID, f.a))
	must(t, st.CreateRequirement(ctx, ten.ID, f.b))
	f.t1 = &model.TestCase{ProjectID: f.p.ID, Title: "T1"}
	f.t2 = &model.TestCase{ProjectID: f.p.ID, Title: "T2", ExecutionType: model.ExecAutomated}
	must(t, st.CreateTest(ctx, ten.ID, f.t1))
	must(t, st.CreateTest(ctx, ten.ID, f.t2))
	f.rel = &model.Release{ProjectID: f.p.ID, Name: "R", TargetDate: "2026-03-20"}
	must(t, st.CreateRelease(ctx, ten.ID, f.rel))
	if _, _, err := st.SetReleaseRequirements(ctx, ten.ID, f.rel.ID, []string{f.a.ID, f.b.ID}, nil); err != nil {
		t.Fatal(err)
	}
	now = at(2, 9)
	must(t, st.CreateLink(ctx, ten.ID, &model.TraceLink{RequirementID: f.a.ID, TestCaseID: f.t1.ID}))
	now = at(5, 9)
	must(t, st.CreateLink(ctx, ten.ID, &model.TraceLink{RequirementID: f.b.ID, TestCaseID: f.t2.ID}))
	run := func(day int, rel bool, r1, r2 string) {
		now = at(day, 10)
		rn := &model.TestRun{ProjectID: f.p.ID, Name: "run"}
		if rel {
			rn.ReleaseID = &f.rel.ID
		}
		must(t, st.CreateRun(ctx, ten.ID, rn, []string{f.t1.ID, f.t2.ID}))
		es, err := st.ListExecutions(ctx, ten.ID, rn.ID)
		must(t, err)
		for _, e := range es {
			s := r2
			if e.TestCaseID == f.t1.ID {
				s = r1
			}
			now = now.Add(time.Minute)
			if _, err := st.UpdateExecution(ctx, ten.ID, e.ID, "u", store.ExecutionUpdate{Status: &s}); err != nil {
				t.Fatal(err)
			}
			if e.TestCaseID == f.t1.ID {
				f.lastT1 = now
			}
		}
	}
	run(3, false, model.ResultPassed, model.ResultFailed)
	run(6, false, model.ResultFailed, model.ResultPassed)
	run(8, true, model.ResultPassed, model.ResultPassed)
	f.flipT1 = f.lastT1
	now = at(4, 9)
	d1 := &model.Defect{ProjectID: f.p.ID, Title: "d1", Severity: "major"}
	must(t, st.CreateDefect(ctx, ten.ID, d1))
	now = at(7, 9)
	d1.Status = "resolved"
	must(t, st.UpdateDefect(ctx, ten.ID, d1))
	now = at(8, 9)
	must(t, st.CreateDefect(ctx, ten.ID, &model.Defect{ProjectID: f.p.ID, Title: "d2", Severity: "critical"}))
	must(t, st.CreateRisk(ctx, ten.ID, &model.Risk{ProjectID: f.p.ID, Title: "k", Likelihood: 4, Impact: 3}))
	now = at(10, 12)
	return f
}

func snapshotsByDay(t *testing.T, f *metricsFixture) map[string]store.MetricSnapshot {
	t.Helper()
	ss, err := f.st.ListSnapshots(f.ctx, f.ten, f.p.ID, "2026-01-01", "2026-12-31")
	must(t, err)
	out := map[string]store.MetricSnapshot{}
	for _, s := range ss {
		out[s.Day] = s
	}
	return out
}

func TestSnapshotsBackfillAndRecord(t *testing.T) {
	f := newMetricsFixture(t)
	st, ctx := f.st, f.ctx
	wrote, err := st.RecordSnapshot(ctx, f.ten, f.p.ID, false)
	if err != nil || !wrote {
		t.Fatal(wrote, err)
	}
	days := snapshotsByDay(t, f)
	if len(days) != 10 {
		t.Fatalf("%d snapshots: %v", len(days), days)
	}
	d1 := days["2026-03-01"]
	if d1.Source != store.SnapshotBackfill || d1.Data.Requirements != 2 || d1.Data.Covered != 0 || d1.Data.Coverage["none"] != 2 || d1.Data.Tests != 2 || d1.Data.Results["not_run"] != 2 {
		t.Errorf("03-01: %+v", d1)
	}
	if d1.Data.RequirementStatus != nil || d1.Data.OpenRisks != nil {
		t.Errorf("backfilled statuses: %+v", d1.Data)
	}
	if d := days["2026-03-02"].Data; d.Covered != 1 || d.Coverage["not_run"] != 1 || d.CoveragePct() != 50 {
		t.Errorf("03-02: %+v", d)
	}
	d3 := days["2026-03-03"].Data
	if d3.Coverage["passed"] != 1 || d3.Results["passed"] != 1 || d3.Results["failed"] != 1 || d3.Executions["passed"] != 1 || d3.Executions["failed"] != 1 || d3.PassRate() != 50 {
		t.Errorf("03-03: %+v", d3)
	}
	if d := days["2026-03-04"].Data; len(d.Executions) != 0 || d.OpenDefects["major"] != 1 {
		t.Errorf("03-04: %+v", d)
	}
	if d := days["2026-03-06"].Data; d.Coverage["failed"] != 1 || d.Coverage["passed"] != 1 || d.OpenDefects["major"] != 1 {
		t.Errorf("03-06: %+v", d)
	}
	if d := days["2026-03-07"].Data; d.OpenDefects["major"] != 0 {
		t.Errorf("03-07: %+v", d)
	}
	td := days["2026-03-10"]
	if td.Source != store.SnapshotRecorded || td.Data.Coverage["passed"] != 2 || td.Data.PassRate() != 100 || td.Data.OpenDefects["critical"] != 1 ||
		td.Data.OpenRisks["high"] != 1 || td.Data.RequirementStatus["draft"] != 2 || td.Data.TestStatus["draft"] != 2 || !td.CreatedAt.Equal(at(10, 12)) {
		t.Errorf("today: %+v", td)
	}
	// already recorded today; a refresh within the hour is skipped
	if wrote, err := st.RecordSnapshot(ctx, f.ten, f.p.ID, true); err != nil || wrote {
		t.Error("refresh within the hour", wrote, err)
	}
	*f.now = at(10, 14)
	if wrote, err := st.RecordSnapshot(ctx, f.ten, f.p.ID, false); err != nil || wrote {
		t.Error("record without refresh", wrote, err)
	}
	if wrote, err := st.RecordSnapshot(ctx, f.ten, f.p.ID, true); err != nil || !wrote {
		t.Error("refresh", wrote, err)
	}
	if got := snapshotsByDay(t, f)["2026-03-10"]; !got.CreatedAt.Equal(at(10, 14)) {
		t.Errorf("refreshed: %+v", got)
	}
	// the next day: the job records the projects without a snapshot of the day
	other := &model.Project{Key: "OT", Name: "Other"}
	must(t, st.CreateProject(ctx, f.ten, other, false))
	*f.now = at(11, 1)
	if n, err := st.RecordAllSnapshots(ctx); err != nil || n != 2 {
		t.Error("all", n, err)
	}
	if n, err := st.RecordAllSnapshots(ctx); err != nil || n != 0 {
		t.Error("all again", n, err)
	}
	ss, err := st.ListSnapshots(ctx, f.ten, f.p.ID, "2026-03-09", "2026-03-11")
	if err != nil || len(ss) != 3 || ss[0].Day != "2026-03-09" || ss[2].Day != "2026-03-11" {
		t.Errorf("range: %+v %v", ss, err)
	}
	// a project without history starts today
	if os, _ := st.ListSnapshots(ctx, f.ten, other.ID, "2026-01-01", "2026-12-31"); len(os) != 1 || os[0].Day != "2026-03-11" {
		t.Errorf("other: %+v", os)
	}
	// deleting the project removes its snapshots
	must(t, st.DeleteProject(ctx, f.ten, f.p.ID))
	var n int
	must(t, st.DB.QueryRowContext(ctx, st.DB.Rebind(`SELECT COUNT(*) FROM metric_snapshots WHERE project_id = ?`), f.p.ID).Scan(&n))
	if n != 0 {
		t.Errorf("%d snapshots left", n)
	}
	// errors: closed database
	st.DB.Close()
	if _, err := st.RecordSnapshot(ctx, f.ten, other.ID, false); err == nil {
		t.Error("closed db")
	}
	if _, err := st.RecordAllSnapshots(ctx); err == nil {
		t.Error("closed db: all")
	}
	if _, err := st.ListSnapshots(ctx, f.ten, other.ID, "", "~"); err == nil {
		t.Error("closed db: list")
	}
	if _, err := st.ComputeMetrics(ctx, f.ten, other.ID); err == nil {
		t.Error("closed db: compute")
	}
	if _, err := st.DefectWeeks(ctx, f.ten, other.ID, at(1, 0), at(2, 0)); err == nil {
		t.Error("closed db: weeks")
	}
	if _, err := st.OpenDefectsBySeverity(ctx, f.ten, other.ID); err == nil {
		t.Error("closed db: severity")
	}
	if _, err := st.FlakyTests(ctx, f.ten, other.ID); err == nil {
		t.Error("closed db: flaky")
	}
	if _, err := st.FlakyScores(ctx, f.ten, other.ID); err == nil {
		t.Error("closed db: flaky scores")
	}
}

func TestBackfillLimit(t *testing.T) {
	st, ten, ctx := setup(t)
	now := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	st.SetClock(func() time.Time { return now })
	p := &model.Project{Key: "OLD", Name: "Old"}
	must(t, st.CreateProject(ctx, ten.ID, p, false))
	must(t, st.CreateRequirement(ctx, ten.ID, &model.Requirement{ProjectID: p.ID, Title: "r"}))
	now = now.AddDate(2, 0, 0)
	if _, err := st.RecordSnapshot(ctx, ten.ID, p.ID, false); err != nil {
		t.Fatal(err)
	}
	ss, err := st.ListSnapshots(ctx, ten.ID, p.ID, "2000-01-01", "2100-01-01")
	if err != nil || len(ss) != store.MaxBackfillDays+1 || ss[0].Data.Requirements != 1 {
		t.Fatalf("%d snapshots %v", len(ss), err)
	}
}

func TestComputeMetricsAndDefects(t *testing.T) {
	f := newMetricsFixture(t)
	d, err := f.st.ComputeMetrics(f.ctx, f.ten, f.p.ID)
	if err != nil || d.Requirements != 2 || d.Covered != 2 || d.CoveragePct() != 100 || d.PassRate() != 100 {
		t.Fatal(d, err)
	}
	var empty store.MetricData
	if empty.CoveragePct() != 0 || empty.PassRate() != 0 {
		t.Error("empty")
	}
	sev, err := f.st.OpenDefectsBySeverity(f.ctx, f.ten, f.p.ID)
	if err != nil || sev["critical"] != 1 || sev["major"] != 0 || len(sev) != len(model.DefectSeverities) {
		t.Error(sev, err)
	}
	// weeks: 2026-03-02 (Mon) … 2026-03-08 (Sun) and the week of 03-09
	ws, err := f.st.DefectWeeks(f.ctx, f.ten, f.p.ID, at(1, 0), at(10, 0))
	if err != nil || len(ws) != 3 {
		t.Fatal(ws, err)
	}
	if ws[0].Week != "2026-02-23" || ws[1].Week != "2026-03-02" || ws[1].Opened != 2 || ws[1].Closed != 1 || ws[2].Opened != 0 {
		t.Errorf("%+v", ws)
	}
	if got := store.WeekStart(time.Date(2026, 3, 8, 23, 0, 0, 0, time.UTC)); store.Day(got) != "2026-03-02" {
		t.Error(got)
	}
}

func TestReleaseBurnup(t *testing.T) {
	f := newMetricsFixture(t)
	b, err := f.st.ReleaseBurnup(f.ctx, f.ten, f.rel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if b.From != "2026-03-01" || b.To != "2026-03-10" || len(b.Points) != 10 || b.TargetDate != "2026-03-20" || b.Name != "R" {
		t.Fatalf("%+v", b)
	}
	p := map[string]store.BurnupPoint{}
	for _, x := range b.Points {
		p[x.Day] = x
	}
	// only the release's run (03-08) counts for passed
	if x := p["2026-03-01"]; x.Scope != 2 || x.Covered != 0 || x.Passed != 0 {
		t.Errorf("03-01 %+v", x)
	}
	if x := p["2026-03-05"]; x.Covered != 2 || x.Passed != 0 {
		t.Errorf("03-05 %+v", x)
	}
	if x := p["2026-03-08"]; x.Passed != 2 {
		t.Errorf("03-08 %+v", x)
	}
	// a released release ends on its release day
	*f.now = at(9, 12)
	f.rel.Status = "released"
	must(t, f.st.UpdateRelease(f.ctx, f.ten, f.rel, "u"))
	*f.now = at(10, 12)
	b, err = f.st.ReleaseBurnup(f.ctx, f.ten, f.rel.ID)
	if err != nil || b.To != "2026-03-09" || len(b.Points) != 9 {
		t.Errorf("released: %+v %v", b, err)
	}
	if _, err := f.st.ReleaseBurnup(f.ctx, f.ten, "missing"); err == nil {
		t.Error("missing release")
	}
}

func TestFlakiness(t *testing.T) {
	r := func(ss ...string) []store.FlakyResult {
		var out []store.FlakyResult
		for _, s := range ss {
			out = append(out, store.FlakyResult{Status: s})
		}
		return out
	}
	const P, F, B = model.ResultPassed, model.ResultFailed, model.ResultBlocked
	if n, w := store.Flakiness(nil); n != 0 || len(w) != 0 {
		t.Error(n, w)
	}
	if n, _ := store.Flakiness(r(P, P, F, F, F)); n != 1 {
		t.Error("one change", n)
	}
	// blocked results are ignored
	if n, w := store.Flakiness(r(P, B, F, B, P)); n != 2 || len(w) != 3 || !w[1].Flip || w[0].Flip {
		t.Error("blocked", n, w)
	}
	// only the last FlakyWindow results count
	seq := r(P, F, P, F, P, P, P, P, P, P, P, P, F)
	if n, w := store.Flakiness(seq); n != 2 || len(w) != store.FlakyWindow || w[0].Status != F || w[0].Flip {
		t.Error("window", n, len(w))
	}

	f := newMetricsFixture(t)
	scores, err := f.st.FlakyScores(f.ctx, f.ten, f.p.ID)
	if err != nil || scores[f.t1.ID] != 2 || scores[f.t2.ID] != 1 {
		t.Fatal(scores, err)
	}
	fl, err := f.st.FlakyTests(f.ctx, f.ten, f.p.ID)
	if err != nil || len(fl) != 1 {
		t.Fatal(fl, err)
	}
	x := fl[0]
	if x.TestID != f.t1.ID || x.Key != f.t1.Key || x.Title != "T1" || x.Score != 2 || x.Window != 3 || x.LastFlip == nil || !x.LastFlip.Equal(f.flipT1) ||
		x.ExecutionType != model.ExecManual || x.Results[2].Status != P || !x.Results[2].Flip || x.Results[0].RunID == "" || x.Results[0].ExecutionID == "" {
		t.Errorf("%+v", x)
	}
	// ordering: higher score first, then the latest flip
	*f.now = at(10, 13)
	rn := &model.TestRun{ProjectID: f.p.ID, Name: "more"}
	must(t, f.st.CreateRun(f.ctx, f.ten, rn, []string{f.t2.ID}))
	es, _ := f.st.ListExecutions(f.ctx, f.ten, rn.ID)
	s := model.ResultFailed
	if _, err := f.st.UpdateExecution(f.ctx, f.ten, es[0].ID, "u", store.ExecutionUpdate{Status: &s}); err != nil {
		t.Fatal(err)
	}
	fl, err = f.st.FlakyTests(f.ctx, f.ten, f.p.ID)
	if err != nil || len(fl) != 2 || fl[0].TestID != f.t2.ID {
		t.Errorf("order: %+v %v", fl, err)
	}
}
