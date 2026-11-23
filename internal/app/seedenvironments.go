package app

import (
	"context"
	"slices"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// SeedDemoEnvironments gives the demo project p test environments and test
// data: the environments its runs name in free text (staging, staging-eu,
// ci, local; the runs and their results are linked to them, with the
// component versions), a hardware-in-the-loop rig in maintenance that
// needs approval, a device lab shared by the organisation, bookings this
// week (one waiting for the administrator's approval), and test data sets
// (a synthetic set the data-driven login test takes its rows from, an
// anonymised set with a refresh request, an expired production copy).
func SeedDemoEnvironments(ctx context.Context, st *store.Store, tenantID string, p *model.Project) error {
	dana, err := demoUser(ctx, st, tenantID, DemoMemberEmail, "Dana Lind", rbac.RoleTester)
	if err != nil {
		return err
	}
	erik, err := demoUser(ctx, st, tenantID, DemoOutsiderEmail, "Erik Berg", rbac.RoleTester)
	if err != nil {
		return err
	}
	owner := dana.ID
	members, err := st.ListTenantMembers(ctx, tenantID)
	if err != nil {
		return err
	}
	for _, m := range members {
		if m.Role == rbac.RoleAdmin {
			owner = m.UserID
			break
		}
	}
	cfgs, err := st.ListConfigurations(ctx, tenantID, p.ID)
	if err != nil {
		return err
	}
	var cfgIDs []string
	for _, c := range cfgs {
		cfgIDs = append(cfgIDs, c.ID)
	}
	comps := func(kv ...string) []model.Component {
		var out []model.Component
		for i := 0; i+1 < len(kv); i += 2 {
			out = append(out, model.Component{Name: kv[i], Version: kv[i+1]})
		}
		return out
	}
	envs := []*model.Environment{
		{ProjectID: p.ID, Name: "staging", Type: "staging", URLs: []string{"https://staging.example.org", "https://staging.example.org/admin"},
			Description: "Shared staging system, refreshed from the release branch every night.", OwnerID: owner, Status: "available",
			Components: comps("Web app", "1.4.0-rc1", "API", "2.3.1", "PostgreSQL", "16.4", "Redis", "7.2"), Tags: []string{"web", "nightly"},
			ConfigurationIDs: cfgIDs, Capacity: 2, CredentialsRef: "vault: kv/qa/staging (team QA)", CreatedBy: "seed"},
		{ProjectID: p.ID, Name: "staging-eu", Type: "pre_prod", URLs: []string{"https://eu.staging.example.org"},
			Description: "Pre-production in the EU region; bookings need the owner's approval.", OwnerID: owner, Status: "available",
			Components: comps("Web app", "1.3.2", "API", "2.2.0", "PostgreSQL", "15.8"), Tags: []string{"eu"}, Capacity: 1, RequiresApproval: true,
			CredentialsRef: "vault: kv/qa/staging-eu", CreatedBy: "seed"},
		{ProjectID: p.ID, Name: "ci", Type: "test", Description: "Ephemeral containers of the CI pipeline.", OwnerID: owner, Status: "in_use",
			Components: comps("Web app", "main", "PostgreSQL", "16.4"), Tags: []string{"ci"}, Capacity: 10, CreatedBy: "seed"},
		{ProjectID: p.ID, Name: "local", Type: "dev", Description: "A developer's machine.", Status: "available", Capacity: 50, CreatedBy: "seed"},
		{ProjectID: p.ID, Name: "HIL rig 1", Type: "hil_rig", Description: "Hardware-in-the-loop rig with the payment terminal, in lab B2.", OwnerID: owner,
			Status: "maintenance", StatusNote: "Firmware update of the terminal until Friday", Components: comps("Terminal firmware", "5.0.1", "Rig controller", "3.2"),
			Tags: []string{"hardware", "lab"}, Capacity: 1, RequiresApproval: true, CredentialsRef: "Lab key cabinet B2, slot 4", CreatedBy: "seed"},
		{Name: "Device lab", Type: "device", Description: "Phones and tablets shared by every project of the organisation.", OwnerID: owner, Status: "available",
			Components: comps("iOS", "18.1", "Android", "15"), Tags: []string{"mobile"}, Capacity: 3, CreatedBy: "seed"},
	}
	shared, err := st.ListEnvironments(ctx, tenantID, "", true)
	if err != nil {
		return err
	}
	for i, e := range envs {
		if e.ProjectID == "" {
			// the shared lab exists once (a second demo project books it too)
			if j := slices.IndexFunc(shared, func(x model.Environment) bool { return x.Name == e.Name }); j >= 0 {
				envs[i] = &shared[j]
				continue
			}
		}
		if err := st.CreateEnvironment(ctx, tenantID, e); err != nil {
			return err
		}
		snap, err := st.GetEnvironment(ctx, tenantID, e.ID)
		if err != nil {
			return err
		}
		if err := st.AddRevision(ctx, tenantID, model.KindEnvironment, e.ID, snap.Version, "create", "seed", store.SnapshotEnvironment(snap)); err != nil {
			return err
		}
	}
	// the runs and results that name them in free text
	if _, err := st.LinkEnvironmentNames(ctx, tenantID, p.ID); err != nil {
		return err
	}

	staging, eu, rig, lab := envs[0], envs[1], envs[4], envs[5]
	now := st.Now().Truncate(time.Hour)
	at := func(days, hour int) time.Time {
		d := now.AddDate(0, 0, days)
		return time.Date(d.Year(), d.Month(), d.Day(), hour, 0, 0, 0, time.UTC)
	}
	var cycleID *string
	if cycles, err := st.ListCycles(ctx, tenantID, p.ID); err == nil {
		for i := range cycles {
			if cycles[i].Status == model.CycleActive {
				cycleID = &cycles[i].ID
				break
			}
		}
	}
	bookings := []struct {
		e        *model.Environment
		user     *model.User
		from, to time.Time
		purpose  string
		cycle    *string
		approved bool
	}{
		{staging, dana, at(0, 8), at(0, 17), "Sprint 14 regression, payments", cycleID, true},
		{staging, erik, at(1, 9), at(1, 12), "Exploratory session on the new checkout", nil, true},
		{eu, erik, at(2, 8), at(2, 16), "Check the EU tax rules before the release", nil, false},
		{rig, dana, at(7, 8), at(8, 17), "Terminal firmware 5.0.1 acceptance", nil, true},
		{lab, dana, at(1, 13), at(1, 16), "Mobile layout check", nil, true},
	}
	for _, x := range bookings {
		b := &model.Booking{EnvironmentID: x.e.ID, ProjectID: p.ID, UserID: x.user.ID, BookedBy: x.user.Email, StartsAt: x.from, EndsAt: x.to,
			Purpose: x.purpose, CycleID: x.cycle}
		if err := st.CreateBooking(ctx, tenantID, b, x.approved); err != nil {
			return err
		}
	}

	// test data sets
	day := func(n int) string { return now.AddDate(0, 0, n).Format(time.DateOnly) }
	logins := &model.DataSet{ProjectID: p.ID, Name: "Login accounts", Kind: "table", Classification: "synthetic", OwnerID: owner, ExpiresOn: day(10),
		Description: "Accounts of every type for the login tests. Synthetic: made up, safe to share.", Tags: []string{"login", "accounts"},
		Parameters: []string{"email", "password", "role"},
		DataRows: []model.DataRow{
			{Name: "Customer", Values: map[string]string{"email": "carol@example.com", "password": "Customer-123", "role": "Customer"}},
			{Name: "Business", Values: map[string]string{"email": "bob@example.com", "password": "Business-123", "role": "Business account"}},
			{Name: "Administrator", Values: map[string]string{"email": "admin@example.com", "password": "Admin-123", "role": "Administrator"}},
			{Name: "Auditor", Values: map[string]string{"email": "audrey@example.com", "password": "Auditor-123", "role": "Auditor"}},
		}, CreatedBy: "seed"}
	customers := &model.DataSet{ProjectID: p.ID, Name: "Customer records", Kind: "table", Classification: "anonymised", OwnerID: owner, ExpiresOn: day(60),
		Description: "Customer records from production, anonymised (names, addresses and card numbers replaced).", Tags: []string{"customers"},
		Parameters: []string{"customer", "country", "segment"},
		DataRows: []model.DataRow{
			{Values: map[string]string{"customer": "C-1001", "country": "SE", "segment": "private"}},
			{Values: map[string]string{"customer": "C-1002", "country": "DE", "segment": "business"}},
			{Values: map[string]string{"customer": "C-1003", "country": "FR", "segment": "private"}},
		}, CreatedBy: "seed"}
	orders := &model.DataSet{ProjectID: p.ID, Name: "Order export (production copy)", Kind: "files", Classification: "production_copy", OwnerID: owner,
		ExpiresOn: day(-3), Description: "A copy of last month's production orders for the performance tests. Handle as confidential; delete when expired.",
		Tags: []string{"performance"}, CreatedBy: "seed"}
	for _, d := range []*model.DataSet{logins, customers, orders} {
		if err := st.CreateDataSet(ctx, tenantID, d); err != nil {
			return err
		}
	}
	// the data-driven login test takes its rows from the set
	tests, err := st.ListTests(ctx, tenantID, p.ID)
	if err != nil {
		return err
	}
	var loginTest string
	for _, t := range tests {
		if len(t.Parameters) > 0 {
			loginTest = t.ID
			break
		}
	}
	if loginTest != "" {
		if err := st.SetDataLink(ctx, tenantID, &model.DataLink{DataSetID: logins.ID, ItemType: model.KindTest, ItemID: loginTest, Usage: "source", CreatedBy: "seed"}); err != nil {
			return err
		}
		ci := envs[2].ID
		run := &model.TestRun{ProjectID: p.ID, Name: "Login accounts (shared data)", Environment: "ci", EnvironmentID: &ci, Build: "1.4.0-rc2", CreatedBy: "seed"}
		if err := st.CreateRun(ctx, tenantID, run, []string{loginTest}); err != nil {
			return err
		}
		if err := st.SetDataLink(ctx, tenantID, &model.DataLink{DataSetID: logins.ID, ItemType: "run", ItemID: run.ID, Usage: "reference", CreatedBy: "seed"}); err != nil {
			return err
		}
	}
	if len(cfgIDs) > 0 {
		if err := st.SetDataLink(ctx, tenantID, &model.DataLink{DataSetID: customers.ID, ItemType: "configuration", ItemID: cfgIDs[0], Usage: "reference", CreatedBy: "seed"}); err != nil {
			return err
		}
	}
	for _, d := range []*model.DataSet{logins, customers, orders} {
		snap, err := st.GetDataSet(ctx, tenantID, d.ID)
		if err != nil {
			return err
		}
		if err := st.AddRevision(ctx, tenantID, model.KindDataSet, d.ID, snap.Version, "create", "seed", store.SnapshotDataSet(snap)); err != nil {
			return err
		}
	}
	return st.CreateRefreshRequest(ctx, tenantID, &model.RefreshRequest{DataSetID: customers.ID, RequesterID: dana.ID, RequestedBy: dana.Email,
		Reason: "Add customers from Norway and Finland for the Nordic tax tests.", NeededBy: day(5)})
}
