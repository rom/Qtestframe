package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

func TestCycleBurndownAndProgress(t *testing.T) {
	now := time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC)
	d := func(day, hour int) *time.Time { x := time.Date(2026, 3, day, hour, 0, 0, 0, time.UTC); return &x }
	exec := "e"
	cy := &model.TestCycle{StartDate: "2026-03-01", EndDate: "2026-03-08", CreatedAt: now}
	plan := []model.TestCycleTest{
		{EstimateMinutes: 10, AddedAt: *d(1, 8), Status: model.ResultPassed, FinishedAt: d(2, 10), ExecutionID: &exec, DurationMS: 1000, AssigneeID: "a"},
		{EstimateMinutes: 20, AddedAt: *d(1, 8), Status: model.ResultFailed, FinishedAt: d(4, 10), ExecutionID: &exec, DurationMS: 2000, AssigneeID: "a"},
		{EstimateMinutes: 30, AddedAt: *d(3, 8), Status: model.ResultNotRun, AssigneeID: "b"},
		{EstimateMinutes: 40, AddedAt: *d(1, 8), Status: model.ResultBlocked, AssigneeID: ""}, // a result without finish time: counts from today
		{EstimateMinutes: 5, AddedAt: *d(1, 8), Status: model.ResultInProgress, ExecutionID: &exec, AssigneeID: "b"},
	}
	b := store.CycleBurndown(cy, plan, now)
	if len(b) != 8 || b[0].Day != "2026-03-01" || b[7].Day != "2026-03-08" {
		t.Fatal(b)
	}
	rem := func(i int) int { return *b[i].Remaining }
	// day 1: four planned, none done; day 2: one done; day 3: the late one
	// added; day 4: two done; day 5 (today): the blocked one counts as done
	if rem(0) != 4 || rem(1) != 3 || rem(2) != 4 || rem(3) != 3 || rem(4) != 2 || *b[4].RemainingMin != 35 {
		t.Fatal(rem(0), rem(1), rem(2), rem(3), rem(4))
	}
	if b[5].Remaining != nil || b[0].Ideal != 5 || b[7].Ideal != 0 || b[0].IdealMin != 105 {
		t.Fatal(b[5], b[0], b[7])
	}
	// without dates: from the creation day to today; a single day
	b = store.CycleBurndown(&model.TestCycle{CreatedAt: now}, plan, now)
	if len(b) != 1 || b[0].Ideal != 5 {
		t.Fatal(b)
	}
	// a start date after today
	b = store.CycleBurndown(&model.TestCycle{StartDate: "2026-04-01", CreatedAt: now}, plan, now)
	if len(b) != 1 || b[0].Remaining != nil {
		t.Fatal(b)
	}
	// at most a year
	b = store.CycleBurndown(&model.TestCycle{StartDate: "2020-01-01", EndDate: "2030-01-01", CreatedAt: now}, nil, now)
	if len(b) != store.MaxBackfillDays+1 {
		t.Fatal(len(b))
	}

	p := store.CycleProgressOf(plan)
	if p.Planned != 5 || p.Executed != 3 || p.Passed != 1 || p.Failed != 1 || p.Blocked != 1 || p.InProgress != 1 || p.NotRun != 1 ||
		p.NotInRun != 2 || p.Unassigned != 1 || p.EstimatedMin != 105 || p.RemainingMin != 35 || p.ActualMS != 3000 || p.PercentDone != 60 || p.PercentPassed != 20 {
		t.Fatalf("%+v", p)
	}
	if p := store.CycleProgressOf(nil); p.PercentDone != 0 {
		t.Fatal(p)
	}
	loads := store.CycleTesters(plan)
	if len(loads) != 3 || loads[0].AssigneeID != "a" || loads[0].Executed != 2 || loads[2].AssigneeID != "" || loads[1].RemainingMin != 35 {
		t.Fatalf("%+v", loads)
	}
}

func TestCyclesStore(t *testing.T) {
	st, ten, ctx := setup(t)
	p := &model.Project{Key: "CY", Name: "Cycles"}
	must(t, st.CreateProject(ctx, ten.ID, p, false))
	other := &model.Project{Key: "OT", Name: "Other"}
	must(t, st.CreateProject(ctx, ten.ID, other, false))
	u := &model.User{Email: "lead@example.com", PasswordHash: "x"}
	must(t, st.CreateUser(ctx, u))
	must(t, st.SetMembership(ctx, ten.ID, u.ID, "test_lead"))
	t1 := &model.TestCase{ProjectID: p.ID, Title: "One", EstimatedMin: 3}
	must(t, st.CreateTest(ctx, ten.ID, t1))
	t2 := &model.TestCase{ProjectID: other.ID, Title: "Elsewhere"}
	must(t, st.CreateTest(ctx, ten.ID, t2))

	cy := &model.TestCycle{ProjectID: p.ID, Name: "C1", OwnerID: u.ID}
	must(t, st.CreateCycle(ctx, ten.ID, cy))
	if err := st.CreateCycle(ctx, ten.ID, &model.TestCycle{ProjectID: p.ID, Name: "X", Status: "nope"}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if err := st.CreateCycle(ctx, ten.ID, &model.TestCycle{ProjectID: p.ID, Name: "X", StartDate: "01/02/2026"}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	// a cycle created closed is started and closed at once
	closed := &model.TestCycle{ProjectID: p.ID, Name: "Old", Status: model.CycleClosed}
	must(t, st.CreateCycle(ctx, ten.ID, closed))
	if closed.StartedAt == nil || closed.ClosedAt == nil {
		t.Fatal(closed)
	}
	if _, err := st.AddCycleTests(ctx, ten.ID, closed.ID, "x", []store.CycleTestInput{{TestID: t1.ID}}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	if _, err := st.AddCycleTests(ctx, ten.ID, cy.ID, "x", []store.CycleTestInput{{TestID: t2.ID}}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := st.AddCycleTests(ctx, ten.ID, cy.ID, "x", []store.CycleTestInput{{TestID: t1.ID, AssigneeID: "nobody"}}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	bad := -1
	if _, err := st.AddCycleTests(ctx, ten.ID, cy.ID, "x", []store.CycleTestInput{{TestID: t1.ID, EstimateMinutes: &bad}}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	cfg := "missing"
	if _, err := st.AddCycleTests(ctx, ten.ID, cy.ID, "x", []store.CycleTestInput{{TestID: t1.ID, ConfigurationID: &cfg}}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if n, err := st.AddCycleTests(ctx, ten.ID, cy.ID, "x", []store.CycleTestInput{{TestID: t1.ID, AssigneeID: u.ID}}); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	plan, err := st.CycleTests(ctx, ten.ID, cy.ID)
	if err != nil || len(plan) != 1 || plan[0].EstimateMinutes != 3 || plan[0].Status != model.ResultNotRun {
		t.Fatal(plan, err)
	}
	// updates: validation, unknown tester
	if _, err := st.UpdateCycleTests(ctx, ten.ID, cy.ID, nil, store.CycleTestUpdate{}, ""); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	nobody := "nobody"
	if _, err := st.UpdateCycleTests(ctx, ten.ID, cy.ID, nil, store.CycleTestUpdate{AssigneeID: &nobody}, ""); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	// the run: created once, assigned
	run, err := st.CreateCycleRun(ctx, ten.ID, cy.ID, "lead@example.com", map[string]string{u.ID: u.Email})
	if err != nil || run == nil || run.CycleID == nil || *run.CycleID != cy.ID {
		t.Fatal(run, err)
	}
	if again, err := st.CreateCycleRun(ctx, ten.ID, cy.ID, "x", nil); err != nil || again != nil {
		t.Fatal(again, err)
	}
	// unassigning the row also clears the open execution
	none := ""
	if n, err := st.UpdateCycleTests(ctx, ten.ID, cy.ID, []string{plan[0].ID}, store.CycleTestUpdate{AssigneeID: &none}, ""); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	es, _ := st.ListExecutions(ctx, ten.ID, run.ID)
	if len(es) != 1 || es[0].Assignee != "" {
		t.Fatal(es)
	}
	names, err := st.CycleNamesByTest(ctx, ten.ID, p.ID)
	if err != nil || len(names[t1.ID]) != 1 || names[t1.ID][0] != "C1" {
		t.Fatal(names, err)
	}
	// renaming to another cycle's name, unknown cycle
	cy.Name = "old"
	if err := st.UpdateCycle(ctx, ten.ID, cy); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	if err := st.UpdateCycle(ctx, ten.ID, &model.TestCycle{ID: "missing", ProjectID: p.ID, Name: "Z"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := st.GetCycle(ctx, ten.ID, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := st.RemoveCycleTests(ctx, ten.ID, "missing", []string{"x"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := st.CopyCyclePlan(ctx, ten.ID, cy.ID, "missing", "x"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	oc := &model.TestCycle{ProjectID: other.ID, Name: "O"}
	must(t, st.CreateCycle(ctx, ten.ID, oc))
	if _, err := st.CopyCyclePlan(ctx, ten.ID, cy.ID, oc.ID, "x"); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	// the project archive keeps cycles, plans and the runs' cycle; a loaded
	// schedule is off
	x := &model.CycleSchedule{ProjectID: p.ID, NamePattern: "N {n}", TemplateCycleID: &cy.ID, OwnerID: u.ID, Enabled: true,
		Schedule: model.Schedule{Frequency: model.FrequencyDaily, TimeOfDay: "05:00"}}
	must(t, st.CreateCycleSchedule(ctx, ten.ID, x))
	if x.NextRunAt == nil || x.Timezone != "UTC" || x.DurationDays != 1 {
		t.Fatal(x)
	}
	for _, bad := range []model.CycleSchedule{
		{ProjectID: p.ID, NamePattern: "", TemplateCycleID: &cy.ID, OwnerID: u.ID, Schedule: model.Schedule{Frequency: "daily", TimeOfDay: "05:00"}},
		{ProjectID: p.ID, NamePattern: "N", OwnerID: u.ID, Schedule: model.Schedule{Frequency: "daily", TimeOfDay: "05:00"}},
		{ProjectID: p.ID, NamePattern: "N", Query: "x", Schedule: model.Schedule{Frequency: "daily", TimeOfDay: "05:00"}},
		{ProjectID: p.ID, NamePattern: "N", Query: "x", OwnerID: u.ID, DurationDays: 400, Schedule: model.Schedule{Frequency: "daily", TimeOfDay: "05:00"}},
		{ProjectID: p.ID, NamePattern: "N", Query: "x", OwnerID: u.ID, Schedule: model.Schedule{Frequency: "daily", TimeOfDay: "25:00"}},
		{ProjectID: p.ID, NamePattern: "N", TemplateCycleID: &oc.ID, OwnerID: u.ID, Schedule: model.Schedule{Frequency: "daily", TimeOfDay: "05:00"}},
	} {
		bad := bad
		if err := st.CreateCycleSchedule(ctx, ten.ID, &bad); !errors.Is(err, store.ErrInvalid) && !errors.Is(err, store.ErrNotFound) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	b, _ := saveProject(t, st, ten.ID, p.ID)
	res, err := loadProject(st, ten.ID, b, store.ProjectImport{Key: "CY2", Name: "Copy"})
	if err != nil || res.Imported["test_cycles"] != 2 || res.Imported["test_cycle_tests"] != 1 || res.Imported["test_cycle_schedules"] != 1 {
		t.Fatal(res, err)
	}
	loaded, err := st.ListCycles(ctx, ten.ID, res.Project.ID)
	if err != nil || len(loaded) != 2 || len(loaded[0].RunIDs) != 1 || loaded[0].ID == cy.ID {
		t.Fatal(loaded, err)
	}
	ls, err := st.ListCycleSchedules(ctx, ten.ID, res.Project.ID)
	if err != nil || len(ls) != 1 || ls[0].Enabled || ls[0].NextRunAt != nil || *ls[0].TemplateCycleID != loaded[0].ID {
		t.Fatal(ls, err)
	}
	// deleting: the run stays without cycle
	must(t, st.DeleteCycle(ctx, ten.ID, cy.ID))
	if r, err := st.GetRun(ctx, ten.ID, run.ID); err != nil || r.CycleID != nil {
		t.Fatal(r, err)
	}
	if err := st.DeleteCycle(ctx, ten.ID, cy.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if err := st.UpdateCycleSchedule(ctx, ten.ID, &model.CycleSchedule{ID: "missing", ProjectID: p.ID, NamePattern: "N", Query: "q", OwnerID: u.ID,
		Schedule: model.Schedule{Frequency: "daily", TimeOfDay: "05:00"}}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := st.GetCycleSchedule(ctx, ten.ID, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if w, err := st.AssignedCycleTests(ctx, ten.ID, ""); err != nil || len(w) != 0 {
		t.Fatal(w, err)
	}
}
