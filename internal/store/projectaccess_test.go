package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

func TestProjectAccessStore(t *testing.T) {
	st, ten, ctx := setup(t)
	p := &model.Project{Key: "ACC", Name: "Access"}
	must(t, st.CreateProject(ctx, ten.ID, p, false))
	if p.Access != model.ProjectAccessOrganisation {
		t.Fatal(p.Access)
	}
	u := &model.User{Email: "m@example.com"}
	must(t, st.CreateUser(ctx, u))
	out := &model.User{Email: "out@example.com"}
	must(t, st.CreateUser(ctx, out))
	must(t, st.SetMembership(ctx, ten.ID, u.ID, "tester"))

	// access mode
	if err := st.SetProjectAccess(ctx, ten.ID, p.ID, "public"); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if err := st.SetProjectAccess(ctx, ten.ID, "missing", model.ProjectAccessRestricted); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	must(t, st.SetProjectAccess(ctx, ten.ID, p.ID, model.ProjectAccessRestricted))
	if mode, err := st.ProjectAccessMode(ctx, ten.ID, p.ID); err != nil || mode != model.ProjectAccessRestricted {
		t.Fatal(mode, err)
	}
	got, err := st.GetProject(ctx, ten.ID, p.ID)
	if err != nil || got.Access != model.ProjectAccessRestricted {
		t.Fatal(got, err)
	}

	// members
	for _, c := range []struct {
		typ, id, role string
		want          error
	}{
		{store.MemberUser, u.ID, "", store.ErrInvalid},
		{store.MemberUser, out.ID, "viewer", store.ErrInvalid}, // not in the tenant
		{store.MemberGroup, "missing", "viewer", store.ErrInvalid},
		{"robot", u.ID, "viewer", store.ErrInvalid},
	} {
		if _, err := st.SetProjectMember(ctx, ten.ID, p.ID, c.typ, c.id, c.role, "a"); !errors.Is(err, c.want) {
			t.Errorf("%+v: %v", c, err)
		}
	}
	if _, err := st.SetProjectMember(ctx, ten.ID, "missing", store.MemberUser, u.ID, "viewer", "a"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	created, err := st.SetProjectMember(ctx, ten.ID, p.ID, store.MemberUser, u.ID, "viewer", "a")
	if err != nil || !created {
		t.Fatal(created, err)
	}
	created, err = st.SetProjectMember(ctx, ten.ID, p.ID, store.MemberUser, u.ID, "manager", "a")
	if err != nil || created {
		t.Fatal(created, err)
	}
	g := &model.SCIMGroup{TenantID: ten.ID, DisplayName: "QA", Members: []string{u.ID}}
	must(t, st.CreateSCIMGroup(ctx, g))
	_, err = st.SetProjectMember(ctx, ten.ID, p.ID, store.MemberGroup, g.ID, "tester", "a")
	must(t, err)
	ms, err := st.ListProjectMembers(ctx, ten.ID, p.ID)
	if err != nil || len(ms) != 2 || ms[0].Type != store.MemberUser || ms[0].Email != "m@example.com" || ms[0].OrgRole != "tester" || ms[1].Name != "QA" {
		t.Fatal(ms, err)
	}
	gr, err := st.ProjectGrantOf(ctx, ten.ID, p.ID, u.ID)
	if err != nil || strings.Join(gr.UserRoles, ",") != "manager" || len(gr.GroupRoles) != 1 || !gr.Member() {
		t.Fatal(gr, err)
	}
	all, err := st.ProjectGrants(ctx, ten.ID, u.ID)
	if err != nil || len(all) != 1 {
		t.Fatal(all, err)
	}
	users, err := st.ProjectUserGrants(ctx, ten.ID, p.ID)
	if err != nil || strings.Join(users[u.ID].UserRoles, ",") != "manager" || len(users[u.ID].GroupRoles) != 1 {
		t.Fatal(users, err)
	}
	ids, err := st.TenantProjectIDs(ctx, ten.ID)
	if err != nil || len(ids) != 1 {
		t.Fatal(ids, err)
	}
	if pid, err := st.ProjectOf(ctx, `SELECT project_id FROM requirements WHERE id = ? AND tenant_id = ?`, "missing", ten.ID); !errors.Is(err, store.ErrNotFound) || pid != "" {
		t.Fatal(pid, err)
	}
	// a copy of the members
	q := &model.Project{Key: "ACC2", Name: "Copy"}
	must(t, st.CreateProject(ctx, ten.ID, q, false))
	must(t, st.CopyProjectMembers(ctx, ten.ID, p.ID, q.ID, "b"))
	if ms, _ := st.ListProjectMembers(ctx, ten.ID, q.ID); len(ms) != 2 || ms[0].AddedBy != "b" {
		t.Fatal(ms)
	}
	// removal
	if err := st.RemoveProjectMember(ctx, ten.ID, p.ID, store.MemberUser, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	must(t, st.RemoveProjectMember(ctx, ten.ID, p.ID, store.MemberGroup, g.ID))
	if _, err := st.GetProjectMember(ctx, ten.ID, p.ID, store.MemberGroup, g.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if !store.ValidProjectAccess(model.ProjectAccessOrganisation) || store.ValidProjectAccess("") {
		t.Fatal("ValidProjectAccess")
	}
}
