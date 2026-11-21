package store_test

import (
	"encoding/json"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

func TestDashboardInsightQueries(t *testing.T) {
	f := newMetricsFixture(t) // now = 2026-03-10 12:00
	st, ctx, ten, pid := f.st, f.ctx, f.ten, f.p.ID

	// review states: none, then pending → approved, a cancelled one is ignored
	rev := &model.User{Email: "rev@example.com"}
	must(t, st.CreateUser(ctx, rev))
	must(t, st.SetMembership(ctx, ten, rev.ID, "tester"))
	states, err := st.RequirementReviewStates(ctx, ten, pid)
	if err != nil || len(states) != 0 {
		t.Fatal(states, err)
	}
	*f.now = at(9, 9)
	_, err = st.CreateReviewRequests(ctx, ten, model.ReviewRequest{EntityType: model.KindRequirement, EntityID: f.a.ID, RequestedBy: "x"}, []string{rev.ID})
	must(t, err)
	*f.now = at(9, 10)
	rr2, err := st.CreateReviewRequests(ctx, ten, model.ReviewRequest{EntityType: model.KindRequirement, EntityID: f.b.ID, RequestedBy: "x"}, []string{rev.ID})
	must(t, err)
	rr2[0].Status = "cancelled"
	must(t, st.UpdateReviewRequest(ctx, ten, &rr2[0]))
	states, err = st.RequirementReviewStates(ctx, ten, pid)
	if err != nil || len(states) != 1 || states[f.a.ID] != "pending" {
		t.Fatal(states, err)
	}
	// a newer request (another reviewer) decides the state
	rev2 := &model.User{Email: "rev2@example.com"}
	must(t, st.CreateUser(ctx, rev2))
	must(t, st.SetMembership(ctx, ten, rev2.ID, "tester"))
	*f.now = at(10, 9)
	rr3, err := st.CreateReviewRequests(ctx, ten, model.ReviewRequest{EntityType: model.KindRequirement, EntityID: f.a.ID, RequestedBy: "x"}, []string{rev2.ID})
	must(t, err)
	rr3[0].Status = "changes_requested"
	must(t, st.UpdateReviewRequest(ctx, ten, &rr3[0]))
	if states, _ = st.RequirementReviewStates(ctx, ten, pid); states[f.a.ID] != "changes_requested" {
		t.Fatal(states)
	}

	// suspect links
	links, reqs, err := st.SuspectCounts(ctx, ten, pid)
	if err != nil || links != 0 || reqs != 0 {
		t.Fatal(links, reqs, err)
	}
	if _, err := st.MarkLinksSuspect(ctx, ten, model.KindTest, f.t1.ID, "changed"); err != nil {
		t.Fatal(err)
	}
	if links, reqs, err = st.SuspectCounts(ctx, ten, pid); err != nil || links != 1 || reqs != 1 {
		t.Fatal(links, reqs, err)
	}

	// defect ages at 03-10 12:00: d2 (critical, 03-08) is open, d1 resolved;
	// add one reported 40 days before
	*f.now = at(10, 12).AddDate(0, 0, -40)
	must(t, st.CreateDefect(ctx, ten, &model.Defect{ProjectID: pid, Title: "old", Severity: "minor"}))
	*f.now = at(10, 12)
	ages, err := st.OpenDefectAges(ctx, ten, pid, at(10, 12))
	if err != nil {
		t.Fatal(err)
	}
	if ages.Open != 2 || ages.Buckets["lt7"] != 1 || ages.Buckets["30to90"] != 1 || ages.Buckets["7to30"] != 0 || ages.BySeverity["critical"]["lt7"] != 1 ||
		ages.BySeverity["minor"]["30to90"] != 1 || ages.OldestDays != 40 || ages.AverageDays != 21 {
		t.Fatalf("%+v", ages)
	}

	// risks with their requirements
	k := &model.Risk{ProjectID: pid, Title: "k2", Likelihood: 5, Impact: 5, Status: "mitigating", RequirementIDs: []string{f.a.ID, f.b.ID}}
	must(t, st.CreateRisk(ctx, ten, k))
	points, err := st.RiskPoints(ctx, ten, pid)
	if err != nil || len(points) != 2 || points[0].Likelihood != 4 || len(points[0].RequirementIDs) != 0 || len(points[1].RequirementIDs) != 2 || points[1].Status != "mitigating" {
		t.Fatal(points, err)
	}
	empty := &model.Project{Key: "EM", Name: "Empty"}
	must(t, st.CreateProject(ctx, ten, empty, false))
	if points, err = st.RiskPoints(ctx, ten, empty.ID); err != nil || len(points) != 0 {
		t.Fatal(points, err)
	}

	// activity: executions finished on 03-03, 03-06, 03-08 (2 each), defects
	// on 03-04, 03-08 and 01-29 (out of range), revisions recorded here
	*f.now = at(9, 15)
	must(t, st.AddRevision(ctx, ten, model.KindRequirement, f.a.ID, 2, "update", "x", json.RawMessage(`{"title":"A"}`)))
	must(t, st.AddRevision(ctx, ten, model.KindTest, f.t2.ID, 2, "update", "x", json.RawMessage(`{"title":"T2"}`)))
	days, err := st.Activity(ctx, ten, pid, at(1, 0), at(10, 12))
	if err != nil || len(days) != 10 || days[0].Day != "2026-03-01" || days[9].Day != "2026-03-10" {
		t.Fatal(days, err)
	}
	byDay := map[string]store.ActivityDay{}
	for _, d := range days {
		byDay[d.Day] = d
	}
	if byDay["2026-03-03"].Executions != 2 || byDay["2026-03-06"].Executions != 2 || byDay["2026-03-08"].Executions != 2 || byDay["2026-03-08"].Defects != 1 ||
		byDay["2026-03-04"].Defects != 1 || byDay["2026-03-09"].Changes != 2 || byDay["2026-03-10"].Changes != 0 {
		t.Fatalf("%+v", days)
	}
	// another project sees nothing
	days, _ = st.Activity(ctx, ten, empty.ID, at(1, 0), at(10, 12))
	for _, d := range days {
		if d.Changes+d.Executions+d.Defects != 0 {
			t.Fatal(d)
		}
	}
	if d, _ := st.Activity(ctx, ten, pid, at(6, 1), at(5, 23)); len(d) != 0 {
		t.Fatal(d)
	}
}
