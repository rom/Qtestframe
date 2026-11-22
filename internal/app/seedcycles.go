package app

import (
	"context"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// SeedDemoCycles gives the demo project p test cycles: a closed "Sprint 13
// regression", an active "Sprint 14 regression" for the active release
// with testers, estimates and results over the past days (burndown,
// workload), a planned "Sprint 15 regression" copied from it that starts
// by itself, a recurring nightly smoke cycle and the release exit
// criterion "Sprint 14 fully executed". The testers are the demo users
// (created after the bootstrap administrator, see cmdSeedDemo) and the
// organisation's first administrator, who owns the cycles.
func SeedDemoCycles(ctx context.Context, live *store.Store, tenantID string, p *model.Project) error {
	dana, err := demoUser(ctx, live, tenantID, DemoMemberEmail, "Dana Lind", rbac.RoleTester)
	if err != nil {
		return err
	}
	erik, err := demoUser(ctx, live, tenantID, DemoOutsiderEmail, "Erik Berg", rbac.RoleTester)
	if err != nil {
		return err
	}
	owner := dana.ID
	members, err := live.ListTenantMembers(ctx, tenantID)
	if err != nil {
		return err
	}
	for _, m := range members {
		if m.Role == rbac.RoleAdmin {
			owner = m.UserID
			break
		}
	}
	clock := &seedClock{live: live}
	st := live.WithClock(clock.now)
	now := live.Now()
	day := func(n int) time.Time { return now.AddDate(0, 0, n).Truncate(time.Hour) }
	date := func(n int) string { return now.AddDate(0, 0, n).Format(time.DateOnly) }

	tests, err := st.ListTests(ctx, tenantID, p.ID)
	if err != nil {
		return err
	}
	var ids []string
	for _, t := range tests {
		if t.Status != "deprecated" && len(ids) < 12 {
			ids = append(ids, t.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var releaseID *string
	rels, err := st.ListReleases(ctx, tenantID, p.ID)
	if err != nil {
		return err
	}
	for i := range rels {
		if rels[i].Status == "active" {
			releaseID = &rels[i].ID
			break
		}
	}
	testers := []string{dana.ID, erik.ID, owner}
	plan := func(c *model.TestCycle, n int) error {
		in := make([]store.CycleTestInput, 0, n)
		for i, id := range ids[:min(n, len(ids))] {
			in = append(in, store.CycleTestInput{TestID: id, AssigneeID: testers[i%len(testers)]})
		}
		_, err := st.AddCycleTests(ctx, tenantID, c.ID, "seed", in)
		return err
	}
	emails := map[string]string{dana.ID: dana.Email, erik.ID: erik.Email}
	if u, err := st.GetUser(ctx, owner); err == nil {
		emails[owner] = u.Email
	}
	// results of a cycle's run, one test after another from day from on
	results := func(c *model.TestCycle, from int, statuses []string) error {
		clock.at = day(from)
		run, err := st.CreateCycleRun(ctx, tenantID, c.ID, "seed", emails)
		if err != nil || run == nil {
			return err
		}
		c.Status = model.CycleActive
		if err := st.UpdateCycle(ctx, tenantID, c); err != nil {
			return err
		}
		es, err := st.ListExecutions(ctx, tenantID, run.ID)
		if err != nil {
			return err
		}
		for i, e := range es {
			if i >= len(statuses) {
				break
			}
			clock.at = day(from + 1 + i*2/3).Add(time.Duration(i) * 37 * time.Minute)
			s, ms := statuses[i], int64(4+i%5)*60000
			by := emails[testers[i%len(testers)]]
			if _, err := st.UpdateExecution(ctx, tenantID, e.ID, by, store.ExecutionUpdate{Status: &s, DurationMS: &ms}); err != nil {
				return err
			}
		}
		return nil
	}
	const P, F, B = model.ResultPassed, model.ResultFailed, model.ResultBlocked

	// Sprint 13: done and closed
	clock.at = day(-24)
	s13 := &model.TestCycle{ProjectID: p.ID, Name: "Sprint 13 regression", Goal: "Regression of the sprint 13 changes", OwnerID: owner,
		StartDate: date(-23), EndDate: date(-16), Environment: "staging", CreatedBy: "seed"}
	if err := st.CreateCycle(ctx, tenantID, s13); err != nil {
		return err
	}
	if err := plan(s13, 8); err != nil {
		return err
	}
	if err := results(s13, -23, []string{P, P, F, P, P, P, B, P}); err != nil {
		return err
	}
	clock.at = day(-16)
	s13.Status = model.CycleClosed
	if err := st.UpdateCycle(ctx, tenantID, s13); err != nil {
		return err
	}

	// Sprint 14: active, about half executed
	clock.at = day(-10)
	s14 := &model.TestCycle{ProjectID: p.ID, Name: "Sprint 14 regression", Goal: "Regression of the sprint 14 changes before release", OwnerID: owner,
		ReleaseID: releaseID, StartDate: date(-9), EndDate: date(4), Environment: "staging", CreatedBy: "seed"}
	if err := st.CreateCycle(ctx, tenantID, s14); err != nil {
		return err
	}
	if err := plan(s14, 12); err != nil {
		return err
	}
	if err := results(s14, -9, []string{P, P, F, P, B, P, P}); err != nil {
		return err
	}

	// Sprint 15: planned, copied from sprint 14, starts by itself
	clock.at = time.Time{}
	start := day(5)
	s15 := &model.TestCycle{ProjectID: p.ID, Name: "Sprint 15 regression", Goal: "Regression of the sprint 15 changes", OwnerID: owner,
		ReleaseID: releaseID, StartDate: date(5), EndDate: date(18), Environment: "staging", ScheduledStartAt: &start, CreatedBy: "seed"}
	if err := st.CreateCycle(ctx, tenantID, s15); err != nil {
		return err
	}
	if _, err := st.CopyCyclePlan(ctx, tenantID, s14.ID, s15.ID, "seed"); err != nil {
		return err
	}

	// a recurring smoke cycle from sprint 13's plan
	x := &model.CycleSchedule{ProjectID: p.ID, NamePattern: "Weekly smoke {date}", TemplateCycleID: &s13.ID, OwnerID: owner, DurationDays: 2,
		Enabled: true, CreatedBy: "seed", Schedule: model.Schedule{Frequency: model.FrequencyWeekly, Weekday: 1, TimeOfDay: "06:00", Timezone: "Europe/Stockholm", DayOfMonth: 1}}
	if err := st.CreateCycleSchedule(ctx, tenantID, x); err != nil {
		return err
	}

	// the active release is ready only when sprint 14 is fully executed
	if releaseID != nil {
		rel, err := st.GetRelease(ctx, tenantID, *releaseID)
		if err != nil {
			return err
		}
		rel.ExitCriteria = append(rel.ExitCriteria, model.ExitCriterion{Type: "cycle_executed", Threshold: 100, CycleID: s14.ID})
		if err := st.UpdateRelease(ctx, tenantID, rel, "seed"); err != nil {
			return err
		}
	}
	return nil
}
