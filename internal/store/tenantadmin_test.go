package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

func TestTenantPolicyQuotasAndSuspension(t *testing.T) {
	st, ten, ctx := setup(t)
	p, err := st.TenantPolicy(ctx, ten.ID)
	must(t, err)
	if p.MaxMembers != 0 || p.AllowedProviders == nil || p.EmailDomains == nil {
		t.Fatalf("%+v", p)
	}
	if _, err := st.TenantPolicy(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := st.SetTenantPolicy(ctx, "nope", model.TenantPolicy{}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	for _, bad := range []model.TenantPolicy{
		{MaxProjects: -1}, {MaxStorageMB: 1e9}, {AllowedProviders: []string{"oidc:"}}, {AllowedProviders: []string{"oidc:a b"}},
		{EmailDomains: []string{"localhost"}}, {EmailDomains: []string{"a@b.example"}}, {Note: strings.Repeat("n", 2001)},
	} {
		if _, err := st.SetTenantPolicy(ctx, ten.ID, bad); !errors.Is(err, store.ErrInvalid) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	p, err = st.SetTenantPolicy(ctx, ten.ID, model.TenantPolicy{MaxMembers: 1, MaxProjects: 1, MaxStorageMB: 1,
		AllowedProviders: []string{"local", "saml:adfs", "local", ""}, EmailDomains: []string{" Example.com ", "@example.com", "b.example.org"}})
	must(t, err)
	if strings.Join(p.AllowedProviders, ",") != "local,saml:adfs" || strings.Join(p.EmailDomains, ",") != "example.com,b.example.org" {
		t.Fatalf("%+v", p)
	}
	if id, _ := st.TenantByEmailDomain(ctx, "EXAMPLE.com"); id != ten.ID {
		t.Fatal(id)
	}
	if id, _ := st.TenantByEmailDomain(ctx, "example"); id != "" {
		t.Fatal(id)
	}
	if id, _ := st.TenantByEmailDomain(ctx, ""); id != "" {
		t.Fatal(id)
	}
	other, err := st.CreateTenant(ctx, "other", "Other")
	must(t, err)
	if _, err := st.SetTenantPolicy(ctx, other.ID, model.TenantPolicy{EmailDomains: []string{"b.example.org"}}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}

	// one member, one project, one MB
	a := &model.User{Email: "a@example.com"}
	b := &model.User{Email: "b@example.com"}
	must(t, st.CreateUser(ctx, a))
	must(t, st.CreateUser(ctx, b))
	must(t, st.SetMembership(ctx, ten.ID, a.ID, "tester"))
	must(t, st.SetMembership(ctx, ten.ID, a.ID, "viewer")) // a change, not a new member
	if err := st.SetMembership(ctx, ten.ID, b.ID, "tester"); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	must(t, st.SetMembership(ctx, other.ID, b.ID, "tester")) // another organisation
	must(t, st.CreateProject(ctx, ten.ID, &model.Project{Key: "ONE", Name: "One"}, false))
	if err := st.CreateProject(ctx, ten.ID, &model.Project{Key: "TWO", Name: "Two"}, false); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	must(t, st.CreateFile(ctx, ten.ID, &model.FileInfo{Name: "a.bin"}, make([]byte, 600<<10)))
	if err := st.CreateFile(ctx, ten.ID, &model.FileInfo{Name: "b.bin"}, make([]byte, 600<<10)); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	must(t, st.CreateFile(ctx, "", &model.FileInfo{Name: "avatar.png", Kind: model.FileAvatar}, make([]byte, 2<<20))) // global files have no quota
	u, err := st.TenantUsage(ctx, ten.ID)
	must(t, err)
	if u.Members != 1 || u.Projects != 1 || u.Files != 1 || u.StorageBytes != 600<<10 {
		t.Fatalf("%+v", u)
	}

	// suspension
	sus, err := st.SetTenantSuspended(ctx, ten.ID, true)
	must(t, err)
	if !sus.Suspended() || sus.SuspendedAt == nil {
		t.Fatal(sus)
	}
	got, _ := st.GetTenant(ctx, ten.ID)
	if !got.Suspended() {
		t.Fatal("not stored")
	}
	back, err := st.SetTenantSuspended(ctx, ten.ID, false)
	must(t, err)
	if back.Suspended() {
		t.Fatal(back)
	}
	var none *model.Tenant
	if none.Suspended() {
		t.Fatal("nil tenant")
	}
}

func TestPasswordHistoryAndSuperAdmins(t *testing.T) {
	st, _, ctx := setup(t)
	u := &model.User{Email: "h@example.com", PasswordHash: "hash-1", SuperAdmin: true}
	must(t, st.CreateUser(ctx, u))
	if u.PasswordChangedAt == nil {
		t.Fatal("no change time")
	}
	first := *u.PasswordChangedAt
	u.DisplayName = "renamed"
	must(t, st.UpdateUser(ctx, u))
	if h, _ := st.PasswordHistory(ctx, u.ID); len(h) != 0 || !u.PasswordChangedAt.Equal(first) {
		t.Fatal(h, u.PasswordChangedAt)
	}
	for i := 2; i <= 30; i++ {
		u.PasswordHash = "hash-" + string(rune('a'+i))
		must(t, st.UpdateUser(ctx, u))
	}
	h, err := st.PasswordHistory(ctx, u.ID)
	must(t, err)
	if len(h) != store.MaxPasswordHistory || h[len(h)-1] == "hash-1" {
		t.Fatal(len(h), h[len(h)-1])
	}
	// an SSO link removes the password
	u.PasswordHash = ""
	must(t, st.UpdateUser(ctx, u))
	if u.PasswordChangedAt != nil {
		t.Fatal(u.PasswordChangedAt)
	}
	must(t, st.StartPasswordAge(ctx, u.ID))
	got, _ := st.GetUser(ctx, u.ID)
	if got.PasswordChangedAt == nil {
		t.Fatal("no start")
	}
	if _, err := st.PasswordHistory(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if err := st.UpdateUser(ctx, &model.User{ID: "nope"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	n, err := st.CountActiveSuperAdmins(ctx)
	must(t, err)
	off := &model.User{Email: "off@example.com", SuperAdmin: true, Disabled: true}
	must(t, st.CreateUser(ctx, off))
	if m, _ := st.CountActiveSuperAdmins(ctx); m != n {
		t.Fatal(m, n)
	}
}
