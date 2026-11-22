package store_test

import (
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
)

func TestProjectCreator(t *testing.T) {
	st, ten, ctx := setup(t)
	ada := &model.User{Email: "ada@example.com", DisplayName: "Ada Lovelace"}
	must(t, st.CreateUser(ctx, ada))
	bob := &model.User{Email: "bob@example.com"}
	must(t, st.CreateUser(ctx, bob))

	named := &model.Project{Key: "NAMED", Name: "Named", CreatedBy: ada.ID}
	must(t, st.CreateProject(ctx, ten.ID, named, false))
	mailOnly := &model.Project{Key: "MAIL", Name: "Mail only", CreatedBy: bob.ID}
	must(t, st.CreateProject(ctx, ten.ID, mailOnly, false))
	gone := &model.Project{Key: "GONE", Name: "Gone", CreatedBy: "no-such-user"}
	must(t, st.CreateProject(ctx, ten.ID, gone, false))
	seeded := &model.Project{Key: "SEED", Name: "Seeded"}
	must(t, st.CreateProject(ctx, ten.ID, seeded, false))

	want := map[string][3]string{
		"NAMED": {ada.ID, "Ada Lovelace", "ada@example.com"},
		"MAIL":  {bob.ID, "bob", "bob@example.com"}, // the default display name
		"GONE":  {"no-such-user", "", ""},
		"SEED":  {"", "", ""},
	}
	ps, err := st.ListProjects(ctx, ten.ID)
	must(t, err)
	if len(ps) != 4 {
		t.Fatal(len(ps))
	}
	for _, p := range ps {
		if got := [3]string{p.CreatedBy, p.CreatedByName, p.CreatedByEmail}; got != want[p.Key] {
			t.Errorf("%s: %v, want %v", p.Key, got, want[p.Key])
		}
	}
	got, err := st.GetProject(ctx, ten.ID, named.ID)
	must(t, err)
	if got.CreatedByName != "Ada Lovelace" || got.CreatedAt.IsZero() {
		t.Fatal(got)
	}

	// the seed sets the creator once the account exists, only when unknown
	must(t, st.SetProjectCreatorIfUnknown(ctx, ten.ID, seeded.ID, bob.ID))
	must(t, st.SetProjectCreatorIfUnknown(ctx, ten.ID, named.ID, bob.ID))
	if p, _ := st.GetProject(ctx, ten.ID, seeded.ID); p.CreatedBy != bob.ID {
		t.Fatal(p.CreatedBy)
	}
	if p, _ := st.GetProject(ctx, ten.ID, named.ID); p.CreatedBy != ada.ID {
		t.Fatal(p.CreatedBy)
	}
}

// TestProjectCreatorBackfill runs the backfill of migration 0050 again:
// projects created before it get their creator from the audit log.
func TestProjectCreatorBackfill(t *testing.T) {
	st, ten, ctx := setup(t)
	ada := &model.User{Email: "ada@example.com", DisplayName: "Ada"}
	must(t, st.CreateUser(ctx, ada))
	bob := &model.User{Email: "bob@example.com", DisplayName: "Bob"}
	must(t, st.CreateUser(ctx, bob))
	created := &model.Project{Key: "OLD", Name: "Old"}
	must(t, st.CreateProject(ctx, ten.ID, created, false))
	loaded := &model.Project{Key: "LOADED", Name: "Loaded"}
	must(t, st.CreateProject(ctx, ten.ID, loaded, false))
	unknown := &model.Project{Key: "UNKNOWN", Name: "Unknown"}
	must(t, st.CreateProject(ctx, ten.ID, unknown, false))
	audit := func(user, action, id string) {
		must(t, st.Audit(ctx, model.AuditEntry{TenantID: ten.ID, UserID: user, Action: action, EntityType: "project", EntityID: id}))
	}
	audit(ada.ID, "project.create", created.ID)
	audit(bob.ID, "project.update", created.ID) // not a creation
	audit(bob.ID, "project.load", loaded.ID)
	audit(ada.ID, "project.update", unknown.ID)
	audit("", "project.create", unknown.ID) // no user (command line)

	ms, err := db.Migrations()
	must(t, err)
	var backfill string
	for _, m := range ms {
		if m.Version == 50 {
			for _, stmt := range strings.Split(m.SQL, ";") {
				if i := strings.Index(stmt, "UPDATE projects"); i >= 0 {
					backfill = stmt[i:]
				}
			}
		}
	}
	if backfill == "" {
		t.Fatal("migration 0050 has no backfill")
	}
	if _, err := st.DB.ExecContext(ctx, backfill); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		p    *model.Project
		want string
	}{{created, ada.ID}, {loaded, bob.ID}, {unknown, ""}} {
		p, err := st.GetProject(ctx, ten.ID, c.p.ID)
		must(t, err)
		if p.CreatedBy != c.want {
			t.Errorf("%s: %q, want %q", p.Key, p.CreatedBy, c.want)
		}
	}
}
