package app

import (
	"context"
	"fmt"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// SeedHistoryDays is how many days back the demo project starts.
const SeedHistoryDays = 42

// seedClock is the backdated time source of the demo data: at while set,
// else the live store's clock.
type seedClock struct {
	live *store.Store
	at   time.Time
}

func (c *seedClock) now() time.Time {
	if c.at.IsZero() {
		return c.live.Now()
	}
	return c.at
}

func (c *seedClock) advance(d time.Duration) { c.at = c.at.Add(d) }

// seedHistory adds a release, nightly runs with results (one test flaky)
// and defects opened and resolved over the past weeks.
func seedHistory(ctx context.Context, st *store.Store, clock *seedClock, tenantID, projectID string, reqs []*model.Requirement, testIDs []string) error {
	start := clock.live.Now()
	day := func(n int) time.Time { return start.AddDate(0, 0, -n).Truncate(time.Hour) }

	rel := &model.Release{ProjectID: projectID, Name: "1.4", Version: "1.4.0", Status: "active",
		TargetDate: start.AddDate(0, 0, 14).Format(time.DateOnly), Description: "Spring release of the portal.", CreatedBy: "seed"}
	if err := st.CreateRelease(ctx, tenantID, rel); err != nil {
		return err
	}
	var scope []string
	for _, i := range []int{0, 1, 3, 5} {
		if i < len(reqs) {
			scope = append(scope, reqs[i].ID)
		}
	}
	if _, _, err := st.SetReleaseRequirements(ctx, tenantID, rel.ID, scope, nil); err != nil {
		return err
	}

	const P, F, B = model.ResultPassed, model.ResultFailed, model.ResultBlocked
	// results per nightly run (days ago) and test (seed order)
	nightly := []struct {
		ago     int
		results []string
	}{
		{30, []string{P, F, P, B, P}},
		{24, []string{P, F, F, F, P}},
		{18, []string{P, P, P, P, F}},
		{12, []string{P, P, F, P, P}},
		{7, []string{P, P, P, P, P}},
		{3, []string{P, P, F, P, P}},
	}
	for n, nr := range nightly {
		clock.at = day(nr.ago).Add(-2 * time.Hour)
		run := &model.TestRun{ProjectID: projectID, Name: fmt.Sprintf("Nightly regression #%d", n+1), Environment: "ci",
			Build: fmt.Sprintf("1.4.0-b%d", 100+n), CreatedBy: "seed", ReleaseID: &rel.ID}
		if err := st.CreateRun(ctx, tenantID, run, testIDs); err != nil {
			return err
		}
		es, err := st.ListExecutions(ctx, tenantID, run.ID)
		if err != nil {
			return err
		}
		pos := map[string]int{}
		for i, id := range testIDs {
			pos[id] = i
		}
		for _, e := range es {
			i, ok := pos[e.TestCaseID]
			if !ok || i >= len(nr.results) {
				continue
			}
			clock.advance(3 * time.Minute)
			s := nr.results[i]
			if _, err := st.UpdateExecution(ctx, tenantID, e.ID, "ci", store.ExecutionUpdate{Status: &s}); err != nil {
				return err
			}
		}
	}

	link := func(i int) []model.DefectLink {
		if i >= len(testIDs) {
			return nil
		}
		return []model.DefectLink{{EntityType: model.KindTest, EntityID: testIDs[i]}}
	}
	defects := []struct {
		opened, resolved int
		title, severity  string
		test             int
	}{
		{28, 20, "Lockout counter resets after 4 minutes", "major", 2},
		{24, 17, "Error message reveals that the account exists", "critical", 1},
		{18, 0, "Statement CSV uses a comma as decimal separator", "minor", 4},
		{4, 0, "Lockout test times out intermittently on CI", "major", 2},
	}
	for _, d := range defects {
		clock.at = day(d.opened)
		df := &model.Defect{ProjectID: projectID, Title: d.title, Severity: d.severity, Priority: "medium", CreatedBy: "seed", Links: link(d.test)}
		if err := st.CreateDefect(ctx, tenantID, df); err != nil {
			return err
		}
		if d.resolved > 0 {
			clock.at = day(d.resolved)
			df.Status = "resolved"
			if err := st.UpdateDefect(ctx, tenantID, df); err != nil {
				return err
			}
		}
	}
	return nil
}
