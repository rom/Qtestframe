package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// Runs on SQLite and (with QTF_TEST_POSTGRES_DSN) on PostgreSQL.

func TestConsistencyDismissalsAndTrash(t *testing.T) {
	f := newMetricsFixture(t)
	st, ctx, ten, pid := f.st, f.ctx, f.ten, f.p.ID
	c := &model.Requirement{ProjectID: pid, Title: "C"}
	must(t, st.CreateRequirement(ctx, ten, c))

	// dismiss a suspected duplicate (twice is fine), list, undo
	d, err := st.DismissConsistency(ctx, ten, pid, model.CheckDuplicateSuspect, f.b.ID, f.a.ID, "u@example.com")
	must(t, err)
	if _, err := st.DismissConsistency(ctx, ten, pid, model.CheckDuplicateSuspect, f.a.ID, f.b.ID, "u@example.com"); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListConsistencyDismissals(ctx, ten, pid)
	if err != nil || len(list) != 1 || list[0].Check != model.CheckDuplicateSuspect || store.DismissalKey(list[0].Check, list[0].ItemA, list[0].ItemB) != store.DismissalKey(d.Check, f.a.ID, f.b.ID) {
		t.Fatal(list, err)
	}
	for _, bad := range []struct{ check, a, b string }{
		{model.CheckConflictInRelease, f.a.ID, f.b.ID},  // not dismissable
		{model.CheckDuplicateSuspect, f.a.ID, f.a.ID},   // the same item
		{model.CheckDuplicateSuspect, f.a.ID, "nope"},   // both must exist
		{model.CheckBrokenPrerequisite, f.a.ID, "gone"}, // a test is needed
	} {
		if _, err := st.DismissConsistency(ctx, ten, pid, bad.check, bad.a, bad.b, ""); err == nil {
			t.Fatal("accepted", bad)
		}
	}
	other := &model.Project{Key: "OT", Name: "Other"}
	must(t, st.CreateProject(ctx, ten, other, false))
	if _, err := st.DismissConsistency(ctx, ten, other.ID, model.CheckDuplicateSuspect, f.a.ID, f.b.ID, ""); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("other project", err)
	}
	must(t, st.UndismissConsistency(ctx, ten, pid, model.CheckDuplicateSuspect, f.b.ID, f.a.ID))
	if list, _ = st.ListConsistencyDismissals(ctx, ten, pid); len(list) != 0 {
		t.Fatal(list)
	}

	// references to items deleted into the trash
	must(t, st.CreateRelation(ctx, ten, &model.Relation{ProjectID: pid, SourceID: f.a.ID, TargetID: c.ID, Type: "depends_on"}))
	must(t, st.AddTestDependency(ctx, ten, &model.TestDependency{ProjectID: pid, TestID: f.t1.ID, DependsOnID: f.t2.ID}))
	refs, err := st.TrashedReferences(ctx, ten, pid)
	if err != nil || len(refs.Items) != 0 || len(refs.Relations) != 0 {
		t.Fatal(refs, err)
	}
	if _, _, err := st.TrashRequirements(ctx, ten, pid, []string{c.ID}, "u"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.TrashTests(ctx, ten, pid, []string{f.t2.ID}, "u"); err != nil {
		t.Fatal(err)
	}
	refs, err = st.TrashedReferences(ctx, ten, pid)
	must(t, err)
	if len(refs.Relations) != 1 || refs.Relations[0].SourceID != f.a.ID || refs.Relations[0].TargetID != c.ID || refs.Relations[0].Type != "depends_on" {
		t.Fatalf("relations %+v", refs.Relations)
	}
	if len(refs.Dependencies) != 1 || refs.Dependencies[0].TestID != f.t1.ID || refs.Dependencies[0].DependsOnID != f.t2.ID {
		t.Fatalf("dependencies %+v", refs.Dependencies)
	}
	if it := refs.Items[c.ID]; it.Kind != model.KindRequirement || it.Title != "C" || refs.Items[f.t2.ID].Kind != model.KindTest {
		t.Fatalf("items %+v", refs.Items)
	}
	// a dependency on a deleted test can be acknowledged
	if _, err := st.DismissConsistency(ctx, ten, pid, model.CheckBrokenPrerequisite, f.t1.ID, f.t2.ID, ""); err != nil {
		t.Fatal(err)
	}
	// both ends deleted: nothing to report
	if _, _, err := st.TrashTests(ctx, ten, pid, []string{f.t1.ID}, "u"); err != nil {
		t.Fatal(err)
	}
	if refs, _ = st.TrashedReferences(ctx, ten, pid); len(refs.Dependencies) != 0 {
		t.Fatalf("%+v", refs.Dependencies)
	}

	scopes, err := st.ReleaseScopes(ctx, ten, pid)
	if err != nil || len(scopes[f.rel.ID]) != 2 {
		t.Fatal(scopes, err)
	}
}

func TestDefectFoundIn(t *testing.T) {
	st, ten, ctx := setup(t)
	p := &model.Project{Key: "DF", Name: "Defects"}
	must(t, st.CreateProject(ctx, ten.ID, p, false))
	d := &model.Defect{ProjectID: p.ID, Title: "Crash", FoundIn: "production"}
	must(t, st.CreateDefect(ctx, ten.ID, d))
	got, err := st.GetDefect(ctx, ten.ID, d.ID)
	if err != nil || got.FoundIn != "production" {
		t.Fatal(got, err)
	}
	got.FoundIn = "test"
	must(t, st.UpdateDefect(ctx, ten.ID, got))
	if list, _ := st.ListDefects(ctx, ten.ID, p.ID); list[0].FoundIn != "test" {
		t.Fatal(list[0].FoundIn)
	}
	got.FoundIn = "space"
	if err := st.UpdateDefect(ctx, ten.ID, got); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestQualityLoaders(t *testing.T) {
	f := newMetricsFixture(t) // now 2026-03-10 12:00
	st, ctx, ten, pid := f.st, f.ctx, f.ten, f.p.ID
	*f.now = at(2, 9)
	must(t, st.AddRevision(ctx, ten, model.KindRequirement, f.a.ID, 1, "create", "u", store.SnapshotRequirement(f.a)))
	*f.now = at(5, 9)
	f.a.Description = "The system SHOULD be fast."
	must(t, st.AddRevision(ctx, ten, model.KindRequirement, f.a.ID, 2, "update", "u", store.SnapshotRequirement(f.a)))
	*f.now = at(8, 9)
	f.a.Description = "The system MUST answer within 2 s."
	must(t, st.AddRevision(ctx, ten, model.KindRequirement, f.a.ID, 3, "update", "u", store.SnapshotRequirement(f.a)))
	*f.now = at(10, 12)

	all, err := st.QualityRevisions(ctx, ten, pid, nil)
	if err != nil || len(all) != 3 || all[0].Action != "create" || all[2].Version != 3 || all[2].Description != "" {
		t.Fatalf("%+v %v", all, err)
	}
	from := at(6, 0)
	revs, err := st.QualityRevisions(ctx, ten, pid, &from)
	must(t, err)
	// texts: the revision of 03-05 (the last before the range) and of 03-08
	if revs[0].Description != "" || revs[1].Description != "The system SHOULD be fast." || revs[2].Description != "The system MUST answer within 2 s." || revs[2].Title != "A" {
		t.Fatalf("%+v", revs)
	}
	execs, err := st.QualityExecutions(ctx, ten, pid)
	if err != nil || len(execs) != 6 {
		t.Fatal(len(execs), err)
	}
	for _, x := range execs {
		if x.At.IsZero() || x.TestID == "" || x.RunID == "" || x.Status == model.ResultNotRun {
			t.Fatalf("%+v", x)
		}
	}
	rev := &model.User{Email: "rev@example.com"}
	must(t, st.CreateUser(ctx, rev))
	must(t, st.SetMembership(ctx, ten, rev.ID, "tester"))
	rr, err := st.CreateReviewRequests(ctx, ten, model.ReviewRequest{EntityType: model.KindRequirement, EntityID: f.a.ID, RequestedBy: "x"}, []string{rev.ID})
	must(t, err)
	*f.now = at(10, 15)
	rr[0].Status = "approved"
	must(t, st.UpdateReviewRequest(ctx, ten, &rr[0]))
	reviews, err := st.QualityReviews(ctx, ten, pid)
	if err != nil || len(reviews) != 1 || reviews[0].DecidedAt == nil || reviews[0].DecidedAt.Sub(reviews[0].CreatedAt) != 3*time.Hour {
		t.Fatal(reviews, err)
	}
}
