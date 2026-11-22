package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

func TestMaxOverlap(t *testing.T) {
	h := func(n int) time.Time { return time.Date(2026, 5, 1, n, 0, 0, 0, time.UTC) }
	bk := func(a, b int) model.Booking { return model.Booking{StartsAt: h(a), EndsAt: h(b)} }
	list := []model.Booking{bk(8, 10), bk(9, 11), bk(10, 12), bk(13, 14)}
	if n := store.MaxOverlap(list, h(0), h(23)); n != 2 {
		t.Fatal(n) // 8–10 and 9–11 overlap; 10–12 only touches 8–10
	}
	if n := store.MaxOverlap(list, h(12), h(13)); n != 0 {
		t.Fatal(n)
	}
	if n := store.MaxOverlap(nil, h(0), h(1)); n != 0 {
		t.Fatal(n)
	}
}

func TestDataSetExpiry(t *testing.T) {
	for _, c := range []struct {
		on, today         string
		expired, expiring bool
	}{
		{"", "2026-05-01", false, false},
		{"2026-04-30", "2026-05-01", true, false},
		{"2026-05-01", "2026-05-01", false, true},
		{"2026-05-15", "2026-05-01", false, true},
		{"2026-05-16", "2026-05-01", false, false},
		{"2026-05-10", "bad", false, false},
	} {
		if x, y := store.DataSetExpiry(c.on, c.today); x != c.expired || y != c.expiring {
			t.Errorf("%v: %v %v", c, x, y)
		}
	}
}

func TestEnvironmentsStore(t *testing.T) {
	st, ten, ctx := setup(t)
	p := &model.Project{Key: "EV", Name: "Envs"}
	must(t, st.CreateProject(ctx, ten.ID, p, false))
	other := &model.Project{Key: "OT", Name: "Other"}
	must(t, st.CreateProject(ctx, ten.ID, other, false))
	u := &model.User{Email: "lead@example.com", PasswordHash: "x"}
	must(t, st.CreateUser(ctx, u))
	must(t, st.SetMembership(ctx, ten.ID, u.ID, "test_lead"))

	// validation
	for _, bad := range []model.Environment{
		{ProjectID: p.ID},
		{ProjectID: p.ID, Name: "x", Type: "spaceship"},
		{ProjectID: p.ID, Name: "x", Status: "gone"},
		{ProjectID: p.ID, Name: "x", Capacity: -1},
		{ProjectID: p.ID, Name: "x", HealthURL: "ftp://example.org"},
		{ProjectID: p.ID, Name: "x", HealthIntervalMinutes: 5000},
		{ProjectID: p.ID, Name: "x", Components: []model.Component{{Name: "app"}, {Name: "APP"}}},
		{ProjectID: p.ID, Name: "x", Components: []model.Component{{Version: "1"}}},
		{ProjectID: p.ID, Name: "x", OwnerID: "nobody"},
		{ProjectID: p.ID, Name: "x", ConfigurationIDs: []string{"missing"}},
		{Name: "shared", VariantIDs: []string{"v"}},
	} {
		e := bad
		if err := st.CreateEnvironment(ctx, ten.ID, &e); !errors.Is(err, store.ErrInvalid) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	e := &model.Environment{ProjectID: p.ID, Name: "Staging", Type: "staging", OwnerID: u.ID, Capacity: 2, Secret: "s3cr3t-value",
		Components: []model.Component{{Name: " app ", Version: "2.3.1"}, {Name: "", Version: ""}, {Name: "DB", Version: "16"}}, URLs: []string{"https://s.example.org", ""}}
	must(t, st.CreateEnvironment(ctx, ten.ID, e))
	if e.Secret != "" || !e.HasSecret || len(e.Components) != 2 || e.Components[0].Name != "app" || len(e.URLs) != 1 {
		t.Fatalf("%+v", e)
	}
	if err := st.CreateEnvironment(ctx, ten.ID, &model.Environment{ProjectID: p.ID, Name: "staging"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	shared := &model.Environment{Name: "Device lab", Type: "device", Capacity: 1, RequiresApproval: true}
	must(t, st.CreateEnvironment(ctx, ten.ID, shared))
	if err := st.CreateEnvironment(ctx, ten.ID, &model.Environment{Name: "device LAB"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	// the same name in another project is fine
	must(t, st.CreateEnvironment(ctx, ten.ID, &model.Environment{ProjectID: other.ID, Name: "Staging"}))
	list, err := st.ListEnvironments(ctx, ten.ID, p.ID, true)
	if err != nil || len(list) != 2 || list[0].Name != "Device lab" || !list[0].Shared || list[1].ProjectKey != "EV" {
		t.Fatal(list, err)
	}
	if list, _ := st.ListEnvironments(ctx, ten.ID, p.ID, false); len(list) != 1 {
		t.Fatal(list)
	}
	if list, _ := st.ListEnvironments(ctx, ten.ID, "", false); len(list) != 1 || list[0].ID != shared.ID {
		t.Fatal(list)
	}
	if s, err := st.EnvironmentSecret(ctx, ten.ID, e.ID); err != nil || s != "s3cr3t-value" {
		t.Fatal(s, err)
	}
	// find by name: the project's own first, then shared; not another project's
	if f, err := st.FindEnvironment(ctx, ten.ID, p.ID, "STAGING"); err != nil || f.ID != e.ID {
		t.Fatal(f, err)
	}
	if f, err := st.FindEnvironment(ctx, ten.ID, p.ID, shared.ID); err != nil || f.ID != shared.ID {
		t.Fatal(f, err)
	}
	if _, err := st.FindEnvironment(ctx, ten.ID, other.ID, e.ID); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}

	// runs: by id, or linked by their free-text name; results snapshot the versions
	t1 := &model.TestCase{ProjectID: p.ID, Title: "One"}
	must(t, st.CreateTest(ctx, ten.ID, t1))
	run := &model.TestRun{ProjectID: p.ID, Name: "R1", Environment: " staging "}
	must(t, st.CreateRun(ctx, ten.ID, run, []string{t1.ID}))
	if run.EnvironmentID == nil || *run.EnvironmentID != e.ID {
		t.Fatal(run.EnvironmentID)
	}
	byID := &model.TestRun{ProjectID: p.ID, Name: "R2", EnvironmentID: &shared.ID}
	must(t, st.CreateRun(ctx, ten.ID, byID, nil))
	if byID.Environment != "Device lab" {
		t.Fatal(byID.Environment)
	}
	foreign := "nope"
	if err := st.CreateRun(ctx, ten.ID, &model.TestRun{ProjectID: other.ID, Name: "R3", EnvironmentID: &e.ID}, nil); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if err := st.CreateRun(ctx, ten.ID, &model.TestRun{ProjectID: p.ID, Name: "R4", EnvironmentID: &foreign}, nil); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	es, _ := st.ListExecutions(ctx, ten.ID, run.ID)
	passed := model.ResultPassed
	x, err := st.UpdateExecution(ctx, ten.ID, es[0].ID, "lead@example.com", store.ExecutionUpdate{Status: &passed})
	must(t, err)
	got, _ := st.GetExecution(ctx, ten.ID, x.ID)
	if got.EnvironmentName != "Staging" || len(got.EnvironmentComponents) != 2 || got.EnvironmentComponents[0].Version != "2.3.1" {
		t.Fatalf("%+v", got)
	}
	// a later version change does not change the recorded result
	e.Components[0].Version = "2.4.0"
	e.Name = "Staging A"
	must(t, st.UpdateEnvironment(ctx, ten.ID, e))
	got, _ = st.GetExecution(ctx, ten.ID, x.ID)
	if got.EnvironmentComponents[0].Version != "2.3.1" {
		t.Fatal(got.EnvironmentComponents)
	}
	if r, _ := st.GetRun(ctx, ten.ID, run.ID); r.Environment != "Staging A" {
		t.Fatal(r.Environment) // the free-text field follows the name
	}
	if s, _ := st.EnvironmentSecret(ctx, ten.ID, e.ID); s != "s3cr3t-value" {
		t.Fatal("secret lost on update", s)
	}
	e.ClearSecret = true
	must(t, st.UpdateEnvironment(ctx, ten.ID, e))
	if g, _ := st.GetEnvironment(ctx, ten.ID, e.ID); g.HasSecret || g.Version != 3 {
		t.Fatal(g)
	}
	// taking the result back clears the snapshot
	notRun := model.ResultNotRun
	_, err = st.UpdateExecution(ctx, ten.ID, es[0].ID, "x", store.ExecutionUpdate{Status: &notRun})
	must(t, err)
	if got, _ = st.GetExecution(ctx, ten.ID, x.ID); got.EnvironmentID != nil || got.EnvironmentName != "" {
		t.Fatal(got)
	}
	_, err = st.UpdateExecution(ctx, ten.ID, es[0].ID, "x", store.ExecutionUpdate{Status: &passed})
	must(t, err)
	rel := &model.Release{ProjectID: p.ID, Name: "1.0"}
	must(t, st.CreateRelease(ctx, ten.ID, rel))
	run.ReleaseID = &rel.ID
	must(t, st.UpdateRun(ctx, ten.ID, run))
	renv, err := st.ReleaseEnvironments(ctx, ten.ID, rel.ID)
	if err != nil || len(renv) != 1 || renv[0].Executions != 1 || renv[0].Passed != 1 || renv[0].Components[0].Version != "2.4.0" || renv[0].Type != "staging" {
		t.Fatal(renv, err)
	}
	// set and clear the environment of a cycle and a session
	cy := &model.TestCycle{ProjectID: p.ID, Name: "C"}
	must(t, st.CreateCycle(ctx, ten.ID, cy))
	must(t, st.SetItemEnvironment(ctx, ten.ID, "cycle", cy.ID, &e.ID))
	if c, _ := st.GetCycle(ctx, ten.ID, cy.ID); c.EnvironmentID == nil || c.Environment != "Staging A" {
		t.Fatal(c)
	}
	xs := &model.ExploratorySession{ProjectID: p.ID, Title: "S"}
	must(t, st.CreateExploratorySession(ctx, ten.ID, xs))
	must(t, st.SetItemEnvironment(ctx, ten.ID, model.KindExploratory, xs.ID, &shared.ID))
	if g, _ := st.GetExploratorySession(ctx, ten.ID, xs.ID); g.EnvironmentName != "Device lab" {
		t.Fatal(g)
	}
	if id, _ := st.ItemEnvironment(ctx, ten.ID, model.KindExploratory, xs.ID); id == nil || *id != shared.ID {
		t.Fatal(id)
	}
	must(t, st.SetItemEnvironment(ctx, ten.ID, model.KindExploratory, xs.ID, nil))
	if err := st.SetItemEnvironment(ctx, ten.ID, "risk", xs.ID, nil); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := st.ItemEnvironment(ctx, ten.ID, "risk", xs.ID); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}

	// bookings: capacity 2, then full
	at := func(d, h int) time.Time { return time.Date(2030, 1, d, h, 0, 0, 0, time.UTC) }
	book := func(env *model.Environment, from, to time.Time, approved bool) (*model.Booking, error) {
		b := &model.Booking{EnvironmentID: env.ID, ProjectID: p.ID, UserID: u.ID, BookedBy: u.Email, StartsAt: from, EndsAt: to, Purpose: "test", RunID: &run.ID}
		return b, st.CreateBooking(ctx, ten.ID, b, approved)
	}
	b1, err := book(e, at(1, 8), at(1, 12), false)
	must(t, err)
	if b1.Status != "approved" {
		t.Fatal(b1.Status)
	}
	_, err = book(e, at(1, 10), at(1, 14), false)
	must(t, err)
	_, err = book(e, at(1, 11), at(1, 13), false)
	var ce *store.BookingConflictError
	if !errors.As(err, &ce) || !errors.Is(err, store.ErrConflict) || ce.Conflict.Capacity != 2 || len(ce.Conflict.Overlapping) != 2 || ce.Error() == "" {
		t.Fatal(err)
	}
	// touching is fine
	_, err = book(e, at(1, 14), at(1, 15), false)
	must(t, err)
	if _, err := book(e, at(1, 15), at(1, 15), false); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := book(e, at(1, 15), at(1, 15).AddDate(2, 0, 0), false); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	bad := &model.Booking{EnvironmentID: e.ID, ProjectID: p.ID, StartsAt: at(3, 1), EndsAt: at(3, 2), CycleID: &foreign}
	if err := st.CreateBooking(ctx, ten.ID, bad, false); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if err := st.CreateBooking(ctx, ten.ID, &model.Booking{EnvironmentID: e.ID, ProjectID: other.ID, StartsAt: at(3, 1), EndsAt: at(3, 2)}, false); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	// approval: pending takes capacity; approve, reject, cancel
	pb, err := book(shared, at(2, 8), at(2, 9), false)
	must(t, err)
	if pb.Status != "pending" {
		t.Fatal(pb.Status)
	}
	if _, err := book(shared, at(2, 8), at(2, 9), true); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	pend, _ := st.PendingBookings(ctx, ten.ID)
	if len(pend) != 1 || pend[0].ID != pb.ID || pend[0].RunName != "R1" || pend[0].EnvironmentName != "Device lab" {
		t.Fatal(pend)
	}
	if _, err := st.DecideBooking(ctx, ten.ID, pb.ID, "maybe", "x", ""); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	ok, err := st.DecideBooking(ctx, ten.ID, pb.ID, "approved", "lead@example.com", "fine")
	if err != nil || ok.Status != "approved" || ok.DecidedBy != "lead@example.com" || ok.DecidedAt == nil {
		t.Fatal(ok, err)
	}
	if _, err := st.DecideBooking(ctx, ten.ID, pb.ID, "rejected", "x", ""); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	// moving an approved booking of an environment that needs approval asks again
	ok.StartsAt, ok.EndsAt = at(2, 10), at(2, 11)
	must(t, st.UpdateBooking(ctx, ten.ID, ok, false))
	if g, _ := st.GetBooking(ctx, ten.ID, pb.ID); g.Status != "pending" || g.DecidedBy != "" {
		t.Fatal(g)
	}
	if _, err := st.DecideBooking(ctx, ten.ID, pb.ID, "cancelled", "x", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DecideBooking(ctx, ten.ID, pb.ID, "cancelled", "x", ""); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	g, _ := st.GetBooking(ctx, ten.ID, pb.ID)
	if err := st.UpdateBooking(ctx, ten.ID, g, true); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	// lists
	if l, _ := st.ListBookings(ctx, ten.ID, e.ID, store.BookingFilter{From: at(1, 11), To: at(1, 12)}); len(l) != 2 {
		t.Fatal(l)
	}
	if l, _ := st.ListBookings(ctx, ten.ID, shared.ID, store.BookingFilter{Statuses: []string{"cancelled"}}); len(l) != 1 {
		t.Fatal(l)
	}
	if l, _ := st.ListProjectBookings(ctx, ten.ID, p.ID, store.BookingFilter{}); len(l) != 4 {
		t.Fatal(len(l))
	}
	if l, _ := st.ItemBookings(ctx, ten.ID, "run", run.ID); len(l) != 4 {
		t.Fatal(len(l))
	}
	if _, err := st.ItemBookings(ctx, ten.ID, "risk", run.ID); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	// booked now / next booking
	st2 := st.WithClock(func() time.Time { return at(1, 9) })
	if g, _ := st2.GetEnvironment(ctx, ten.ID, e.ID); g.BookedNow != 1 || g.NextBooking == nil || !g.NextBooking.Equal(at(1, 10)) {
		t.Fatal(g.BookedNow, g.NextBooking)
	}
	// retired environments cannot be booked nor selected
	e.Status = "retired"
	must(t, st.UpdateEnvironment(ctx, ten.ID, e))
	if _, err := book(e, at(5, 1), at(5, 2), true); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if err := st.SetItemEnvironment(ctx, ten.ID, "cycle", cy.ID, &e.ID); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	must(t, st.DeleteBooking(ctx, ten.ID, b1.ID))
	if err := st.DeleteBooking(ctx, ten.ID, b1.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}

	// health checks: due, a failure breaks an available environment, a pass repairs it
	h := &model.Environment{ProjectID: p.ID, Name: "Health", HealthURL: "http://health.invalid/ok", HealthIntervalMinutes: 5}
	must(t, st.CreateEnvironment(ctx, ten.ID, h))
	now := st.Now()
	due, err := st.DueHealthChecks(ctx, now, 10)
	if err != nil || len(due) != 1 || due[0].EnvironmentID != h.ID {
		t.Fatal(due, err)
	}
	before, after, err := st.RecordHealth(ctx, ten.ID, h.ID, false, "HTTP 503", now)
	if err != nil || before != "available" || after != "broken" {
		t.Fatal(before, after, err)
	}
	if g, _ := st.GetEnvironment(ctx, ten.ID, h.ID); !g.StatusByHealth || g.HealthStatus != "down" || g.HealthError != "HTTP 503" || g.StatusNote == "" {
		t.Fatal(g)
	}
	if due, _ := st.DueHealthChecks(ctx, now.Add(time.Minute), 10); len(due) != 0 {
		t.Fatal(due)
	}
	if due, _ := st.DueHealthChecks(ctx, now.Add(6*time.Minute), 10); len(due) != 1 {
		t.Fatal(due)
	}
	if b, a, _ := st.RecordHealth(ctx, ten.ID, h.ID, true, "", now); b != "broken" || a != "available" {
		t.Fatal(b, a)
	}
	// a broken status set by hand stays
	he, _ := st.GetEnvironment(ctx, ten.ID, h.ID)
	he.Status = "broken"
	must(t, st.UpdateEnvironment(ctx, ten.ID, he))
	if b, a, _ := st.RecordHealth(ctx, ten.ID, h.ID, true, "", now); b != "broken" || a != "broken" {
		t.Fatal(b, a)
	}
	if _, _, err := st.RecordHealth(ctx, ten.ID, "missing", true, "", now); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if snap := store.SnapshotEnvironment(he); len(snap) == 0 {
		t.Fatal("no snapshot")
	}
	must(t, st.DeleteEnvironment(ctx, ten.ID, h.ID))
	if err := st.DeleteEnvironment(ctx, ten.ID, h.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestLinkEnvironmentNames(t *testing.T) {
	st, ten, ctx := setup(t)
	p := &model.Project{Key: "LN", Name: "Link"}
	must(t, st.CreateProject(ctx, ten.ID, p, false))
	t1 := &model.TestCase{ProjectID: p.ID, Title: "One"}
	must(t, st.CreateTest(ctx, ten.ID, t1))
	run := &model.TestRun{ProjectID: p.ID, Name: "R", Environment: "qa"}
	must(t, st.CreateRun(ctx, ten.ID, run, []string{t1.ID}))
	cy := &model.TestCycle{ProjectID: p.ID, Name: "C", Environment: "QA"}
	must(t, st.CreateCycle(ctx, ten.ID, cy))
	es, _ := st.ListExecutions(ctx, ten.ID, run.ID)
	failed := model.ResultFailed
	_, err := st.UpdateExecution(ctx, ten.ID, es[0].ID, "x", store.ExecutionUpdate{Status: &failed})
	must(t, err)
	if g, _ := st.GetExecution(ctx, ten.ID, es[0].ID); g.EnvironmentName != "qa" || g.EnvironmentID != nil {
		t.Fatal(g) // the free text is recorded without an item
	}
	e := &model.Environment{ProjectID: p.ID, Name: "QA", Components: []model.Component{{Name: "app", Version: "1"}}}
	must(t, st.CreateEnvironment(ctx, ten.ID, e))
	n, err := st.LinkEnvironmentNames(ctx, ten.ID, p.ID)
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if g, _ := st.GetExecution(ctx, ten.ID, es[0].ID); g.EnvironmentID == nil || g.EnvironmentName != "QA" || len(g.EnvironmentComponents) != 1 {
		t.Fatal(g)
	}
	if c, _ := st.GetCycle(ctx, ten.ID, cy.ID); c.EnvironmentID == nil {
		t.Fatal(c)
	}
}

func TestDataSetsStore(t *testing.T) {
	st, ten, ctx := setup(t)
	p := &model.Project{Key: "TD", Name: "Data"}
	must(t, st.CreateProject(ctx, ten.ID, p, false))
	other := &model.Project{Key: "OT", Name: "Other"}
	must(t, st.CreateProject(ctx, ten.ID, other, false))
	u := &model.User{Email: "owner@example.com", PasswordHash: "x"}
	must(t, st.CreateUser(ctx, u))
	must(t, st.SetMembership(ctx, ten.ID, u.ID, "tester"))
	for _, bad := range []model.DataSet{
		{ProjectID: p.ID},
		{ProjectID: p.ID, Name: "x", Kind: "sql"},
		{ProjectID: p.ID, Name: "x", Classification: "secret"},
		{ProjectID: p.ID, Name: "x", Status: "gone"},
		{ProjectID: p.ID, Name: "x", ExpiresOn: "tomorrow"},
		{ProjectID: p.ID, Name: "x", Parameters: []string{"1bad"}},
		{ProjectID: p.ID, Name: "x", OwnerID: "nobody"},
	} {
		d := bad
		if err := st.CreateDataSet(ctx, ten.ID, &d); !errors.Is(err, store.ErrInvalid) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	today := st.Now().Format("2006-01-02")
	d := &model.DataSet{ProjectID: p.ID, Name: "Users", OwnerID: u.ID, ExpiresOn: today, Parameters: []string{"user", "role"},
		DataRows: []model.DataRow{{Name: "a", Values: map[string]string{"user": "ann", "role": "admin", "extra": "dropped"}}, {Values: map[string]string{"user": "bob"}}}}
	must(t, st.CreateDataSet(ctx, ten.ID, d))
	if d.Kind != "table" || d.Classification != "synthetic" || d.DataRows[1].Values["role"] != "" || len(d.DataRows[0].Values) != 2 {
		t.Fatalf("%+v", d)
	}
	if err := st.CreateDataSet(ctx, ten.ID, &model.DataSet{ProjectID: p.ID, Name: "USERS"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	files := &model.DataSet{ProjectID: p.ID, Name: "Dump", Kind: "files", Classification: "production_copy", Parameters: []string{"ignored"}}
	must(t, st.CreateDataSet(ctx, ten.ID, files))
	if len(files.Parameters) != 0 {
		t.Fatal(files.Parameters)
	}
	// a data-driven test takes its rows from the set
	tc := &model.TestCase{ProjectID: p.ID, Title: "Login", Parameters: []string{"user"}, DataRows: []model.DataRow{{Values: map[string]string{"user": "inline"}}},
		Steps: []model.TestStep{{Action: "Sign in as {{user}}", Expected: "ok"}}}
	must(t, st.CreateTest(ctx, ten.ID, tc))
	foreignTest := &model.TestCase{ProjectID: other.ID, Title: "Else"}
	must(t, st.CreateTest(ctx, ten.ID, foreignTest))
	for _, bad := range []model.DataLink{
		{DataSetID: d.ID, ItemType: "risk", ItemID: tc.ID},
		{DataSetID: d.ID, ItemType: model.KindTest, ItemID: tc.ID, Usage: "owner"},
		{DataSetID: d.ID, ItemType: "run", ItemID: tc.ID, Usage: "source"},
		{DataSetID: files.ID, ItemType: model.KindTest, ItemID: tc.ID, Usage: "source"},
		{DataSetID: d.ID, ItemType: model.KindTest, ItemID: foreignTest.ID},
	} {
		l := bad
		if err := st.SetDataLink(ctx, ten.ID, &l); !errors.Is(err, store.ErrInvalid) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	if err := st.SetDataLink(ctx, ten.ID, &model.DataLink{DataSetID: "missing", ItemType: model.KindTest, ItemID: tc.ID}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	must(t, st.SetDataLink(ctx, ten.ID, &model.DataLink{DataSetID: d.ID, ItemType: model.KindTest, ItemID: tc.ID}))
	l := &model.DataLink{DataSetID: d.ID, ItemType: model.KindTest, ItemID: tc.ID, Usage: "source"}
	must(t, st.SetDataLink(ctx, ten.ID, l)) // the usage of the existing link changes
	run := &model.TestRun{ProjectID: p.ID, Name: "R"}
	must(t, st.CreateRun(ctx, ten.ID, run, []string{tc.ID}))
	es, _ := st.ListExecutions(ctx, ten.ID, run.ID)
	x, err := st.GetExecution(ctx, ten.ID, es[0].ID)
	must(t, err)
	if len(x.Iterations) != 2 || x.Iterations[0].Values["user"] != "ann" || x.StepResults[0].Action != "Sign in as ann" {
		t.Fatalf("%+v", x.Iterations)
	}
	must(t, st.SetDataLink(ctx, ten.ID, &model.DataLink{DataSetID: d.ID, ItemType: "run", ItemID: run.ID}))
	got, err := st.GetDataSet(ctx, ten.ID, d.ID)
	if err != nil || got.UsageCount != 1 || got.LastUsedAt == nil || len(got.Links) != 2 || !got.Expiring || got.Expired {
		t.Fatalf("%+v %v", got, err)
	}
	if links, _ := st.ItemDataLinks(ctx, ten.ID, model.KindTest, tc.ID); len(links) != 1 || links[0].Usage != "source" || links[0].DataSetName != "Users" || links[0].ItemKey != tc.Key {
		t.Fatal(links)
	}
	us, err := st.DataSetUsage(ctx, ten.ID, d.ID, 0)
	if err != nil || len(us) != 1 || us[0].DataVersion != 1 || us[0].RunName != "R" || us[0].TestKey != tc.Key {
		t.Fatal(us, err)
	}
	// another source for the test makes the first a reference
	d2 := &model.DataSet{ProjectID: p.ID, Name: "Other users", Parameters: []string{"user"}, DataRows: []model.DataRow{{Values: map[string]string{"user": "carl"}}}}
	must(t, st.CreateDataSet(ctx, ten.ID, d2))
	must(t, st.SetDataLink(ctx, ten.ID, &model.DataLink{DataSetID: d2.ID, ItemType: model.KindTest, ItemID: tc.ID, Usage: "source"}))
	if links, _ := st.ItemDataLinks(ctx, ten.ID, model.KindTest, tc.ID); len(links) != 2 || (links[0].Usage == "source") == (links[1].Usage == "source") {
		t.Fatal(links)
	}
	// an archived source is not used: the test's own rows are
	d2.Status = "archived"
	must(t, st.UpdateDataSet(ctx, ten.ID, d2, 0))
	run2 := &model.TestRun{ProjectID: p.ID, Name: "R2"}
	must(t, st.CreateRun(ctx, ten.ID, run2, []string{tc.ID}))
	es2, _ := st.ListExecutions(ctx, ten.ID, run2.ID)
	if x2, _ := st.GetExecution(ctx, ten.ID, es2[0].ID); len(x2.Iterations) != 1 || x2.Iterations[0].Values["user"] != "inline" {
		t.Fatalf("%+v", x2.Iterations)
	}
	// update with a version check
	got.Description = "changed"
	if err := st.UpdateDataSet(ctx, ten.ID, got, 99); !errors.Is(err, store.ErrStale) {
		t.Fatal(err)
	}
	must(t, st.UpdateDataSet(ctx, ten.ID, got, got.Version))
	if got.Version != 2 {
		t.Fatal(got.Version)
	}
	if err := st.UpdateDataSet(ctx, ten.ID, &model.DataSet{ID: "missing", ProjectID: p.ID, Name: "m"}, 0); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	list, err := st.ListDataSets(ctx, ten.ID, p.ID)
	if err != nil || len(list) != 3 || list[2].Name != "Users" || len(list[2].Links) != 2 {
		t.Fatal(list, err)
	}
	if exp, _ := st.ExpiringDataSets(ctx, ten.ID); len(exp) != 1 || exp[0].ID != d.ID {
		t.Fatal(exp)
	}
	if len(store.SnapshotDataSet(got)) == 0 {
		t.Fatal("no snapshot")
	}
	// refresh requests
	if err := st.CreateRefreshRequest(ctx, ten.ID, &model.RefreshRequest{DataSetID: d.ID}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if err := st.CreateRefreshRequest(ctx, ten.ID, &model.RefreshRequest{DataSetID: d.ID, Reason: "x", NeededBy: "soon"}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	q := &model.RefreshRequest{DataSetID: d.ID, RequesterID: u.ID, RequestedBy: u.Email, Reason: "More roles", NeededBy: today}
	must(t, st.CreateRefreshRequest(ctx, ten.ID, q))
	if open, _ := st.OpenRefreshRequests(ctx, ten.ID); len(open) != 1 || open[0].DataSetName != "Users" || open[0].ProjectID != p.ID {
		t.Fatal(open)
	}
	if _, err := st.ResolveRefreshRequest(ctx, ten.ID, q.ID, "open", "x", ""); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	done, err := st.ResolveRefreshRequest(ctx, ten.ID, q.ID, "done", "owner@example.com", "added")
	if err != nil || done.Status != "done" || done.ResolvedAt == nil {
		t.Fatal(done, err)
	}
	if _, err := st.ResolveRefreshRequest(ctx, ten.ID, q.ID, "done", "x", ""); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	if _, err := st.ResolveRefreshRequest(ctx, ten.ID, "missing", "done", "x", ""); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if l, _ := st.ListRefreshRequests(ctx, ten.ID, d.ID); len(l) != 1 {
		t.Fatal(l)
	}
	links, _ := st.ItemDataLinks(ctx, ten.ID, "run", run.ID)
	must(t, st.DeleteDataLink(ctx, ten.ID, d.ID, links[0].ID))
	if err := st.DeleteDataLink(ctx, ten.ID, d.ID, links[0].ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	must(t, st.DeleteDataSet(ctx, ten.ID, d.ID))
	if err := st.DeleteDataSet(ctx, ten.ID, d.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
}
