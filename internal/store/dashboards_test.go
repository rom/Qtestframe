package store_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

func TestDashboardsStore(t *testing.T) {
	st, ten, ctx := setup(t)
	p := &model.Project{Key: "DB", Name: "Dash"}
	must(t, st.CreateProject(ctx, ten.ID, p, false))
	u := &model.User{Email: "dash@example.com"}
	must(t, st.CreateUser(ctx, u))

	// a shared dashboard replacing a built-in layout, only once
	sh := &model.Dashboard{ProjectID: p.ID, Name: " Overview ", Builtin: "overview", Widgets: []model.DashboardWidget{{Type: "note", W: 6, H: 3}}}
	must(t, st.CreateDashboard(ctx, ten.ID, sh))
	if sh.Name != "Overview" || !sh.Shared || sh.Position != 1 || sh.Widgets[0].ID == "" {
		t.Fatalf("%+v", sh)
	}
	if err := st.CreateDashboard(ctx, ten.ID, &model.Dashboard{ProjectID: p.ID, Name: "x", Builtin: "overview"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	mine := &model.Dashboard{ProjectID: p.ID, OwnerID: u.ID, Name: "Mine"}
	must(t, st.CreateDashboard(ctx, ten.ID, mine))
	if mine.Shared || mine.Widgets == nil {
		t.Fatalf("%+v", mine)
	}
	if l, err := st.ListDashboards(ctx, ten.ID, p.ID, u.ID); err != nil || len(l) != 2 || l[0].ID != sh.ID {
		t.Fatal(l, err)
	}
	if l, err := st.ListDashboards(ctx, ten.ID, p.ID, "other"); err != nil || len(l) != 1 {
		t.Fatal(l, err)
	}
	// validation
	for _, bad := range []model.Dashboard{
		{ProjectID: p.ID, Name: ""},
		{ProjectID: p.ID, Name: "x", Widgets: []model.DashboardWidget{{ID: "a", W: 13, H: 1}}},
		{ProjectID: p.ID, Name: "x", Widgets: []model.DashboardWidget{{ID: "a", W: 1, H: 0}}},
		{ProjectID: p.ID, Name: "x", Widgets: []model.DashboardWidget{{ID: "a", W: 1, H: 1}, {ID: "a", W: 1, H: 1}}},
		{ProjectID: p.ID, Name: "x", Widgets: []model.DashboardWidget{{ID: "a", W: 1, H: 1, Settings: map[string]json.RawMessage{"t": make([]byte, 17<<10)}}}},
		{Name: "home without owner"},
	} {
		d := bad
		if err := st.CreateDashboard(ctx, ten.ID, &d); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("%+v: %v", bad, err)
		}
	}
	// update, delete; the default naming it is cleared
	mine.Name = "Renamed"
	must(t, st.UpdateDashboard(ctx, ten.ID, mine))
	if g, err := st.GetDashboard(ctx, ten.ID, mine.ID); err != nil || g.Name != "Renamed" {
		t.Fatal(g, err)
	}
	must(t, st.SetDefaultDashboard(ctx, ten.ID, p.ID, sh.ID, "a"))
	if d, err := st.DefaultDashboard(ctx, ten.ID, p.ID); err != nil || d != sh.ID {
		t.Fatal(d, err)
	}
	must(t, st.DeleteDashboard(ctx, ten.ID, sh.ID))
	if d, err := st.DefaultDashboard(ctx, ten.ID, p.ID); err != nil || d != "" {
		t.Fatal(d, err)
	}
	if err := st.DeleteDashboard(ctx, ten.ID, sh.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if err := st.UpdateDashboard(ctx, ten.ID, sh); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	must(t, st.SetDefaultDashboard(ctx, ten.ID, p.ID, "", "a"))

	// the home dashboard: one per user
	if _, err := st.HomeDashboard(ctx, ten.ID, u.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	must(t, st.CreateDashboard(ctx, ten.ID, &model.Dashboard{OwnerID: u.ID, Name: "Home"}))
	if err := st.CreateDashboard(ctx, ten.ID, &model.Dashboard{OwnerID: u.ID, Name: "Home"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	if h, err := st.HomeDashboard(ctx, ten.ID, u.ID); err != nil || h.ProjectID != "" || h.OwnerID != u.ID {
		t.Fatal(h, err)
	}
	// at most DashboardMaxPerUser of a kind
	for i := 1; i < model.DashboardMaxPerUser; i++ {
		must(t, st.CreateDashboard(ctx, ten.ID, &model.Dashboard{ProjectID: p.ID, OwnerID: u.ID, Name: "n"}))
	}
	if err := st.CreateDashboard(ctx, ten.ID, &model.Dashboard{ProjectID: p.ID, OwnerID: u.ID, Name: "n"}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	// deleting the project removes its dashboards
	must(t, st.DeleteProject(ctx, ten.ID, p.ID))
	if l, err := st.ListDashboards(ctx, ten.ID, p.ID, u.ID); err != nil || len(l) != 0 {
		t.Fatal(l, err)
	}
}
