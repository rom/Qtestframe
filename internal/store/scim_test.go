package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/testutil"
)

func TestSCIMStore(t *testing.T) {
	st, ten, ctx := setup(t)
	other, err := st.CreateTenant(ctx, "second", "Second")
	must(t, err)
	u := &model.User{Email: "p@example.com", AuthProvider: "scim"}
	must(t, st.CreateUser(ctx, u))

	// tokens
	tok := &model.SCIMToken{TenantID: ten.ID, Name: " Entra ", Hash: "h1", Prefix: "qtfscim_abc", CreatedBy: "a@example.com"}
	must(t, st.CreateSCIMToken(ctx, tok))
	if tok.Name != "Entra" {
		t.Fatal(tok.Name)
	}
	if err := st.CreateSCIMToken(ctx, &model.SCIMToken{TenantID: ten.ID, Name: "", Hash: "h2"}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if err := st.CreateSCIMToken(ctx, &model.SCIMToken{TenantID: ten.ID, Name: "dup", Hash: "h1"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	got, err := st.GetSCIMTokenByHash(ctx, "h1")
	if err != nil || got.ID != tok.ID || got.LastUsed == nil {
		t.Fatal(got, err)
	}
	if _, err := st.GetSCIMTokenByHash(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := st.DeleteSCIMToken(ctx, other.ID, tok.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("tenant isolation", err)
	}
	if ts, _ := st.ListSCIMTokens(ctx, ten.ID); len(ts) != 1 {
		t.Fatal(ts)
	}

	// settings
	cfg, err := st.SCIMSettings(ctx, ten.ID)
	if err != nil || cfg.DefaultRole != "viewer" {
		t.Fatal(cfg, err)
	}
	must(t, st.SaveSCIMSettings(ctx, ten.ID, &model.SCIMSettings{DefaultRole: ""}))
	must(t, st.SaveSCIMSettings(ctx, ten.ID, &model.SCIMSettings{DefaultRole: "tester"}))
	if cfg, _ := st.SCIMSettings(ctx, ten.ID); cfg.DefaultRole != "tester" {
		t.Fatal(cfg)
	}

	// users and groups
	su := &model.SCIMUser{TenantID: ten.ID, UserID: u.ID, UserName: "P", ExternalID: "x", Active: true}
	must(t, st.SaveSCIMUser(ctx, su))
	su.Active = false
	must(t, st.SaveSCIMUser(ctx, su))
	if g, err := st.GetSCIMUser(ctx, ten.ID, u.ID); err != nil || g.Active || g.UserName != "P" || g.Email != "p@example.com" {
		t.Fatal(g, err)
	}
	if _, err := st.GetSCIMUser(ctx, other.ID, u.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	g := &model.SCIMGroup{TenantID: ten.ID, DisplayName: " QA ", Members: []string{u.ID, u.ID, ""}}
	must(t, st.CreateSCIMGroup(ctx, g))
	if len(g.Members) != 1 || g.DisplayName != "QA" {
		t.Fatal(g)
	}
	if err := st.CreateSCIMGroup(ctx, &model.SCIMGroup{TenantID: ten.ID, DisplayName: "QA"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	if err := st.UpdateSCIMGroup(ctx, &model.SCIMGroup{ID: "nope", TenantID: ten.ID, DisplayName: "X"}, nil); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if err := st.UpdateSCIMGroup(ctx, &model.SCIMGroup{ID: g.ID, TenantID: ten.ID, DisplayName: " "}, nil); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	gs, err := st.SCIMUserGroups(ctx, ten.ID, u.ID)
	if err != nil || len(gs) != 1 {
		t.Fatal(gs, err)
	}
	if n, _ := st.OtherTenants(ctx, ten.ID, u.ID); n != 0 {
		t.Fatal(n)
	}
	must(t, st.SaveSCIMUser(ctx, &model.SCIMUser{TenantID: other.ID, UserID: u.ID, Active: true}))
	if n, _ := st.OtherTenants(ctx, ten.ID, u.ID); n != 1 {
		t.Fatal(n)
	}
	if err := st.DeleteSCIMGroup(ctx, other.ID, g.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	must(t, st.DeleteSCIMUser(ctx, ten.ID, u.ID))
	if err := st.DeleteSCIMUser(ctx, ten.ID, u.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if g2, _ := st.GetSCIMGroup(ctx, ten.ID, g.ID); len(g2.Members) != 0 {
		t.Fatal("group member survived", g2)
	}
	if err := st.SetUserEmail(ctx, u.ID, "bad"); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if err := st.SetUserEmail(ctx, "nope", "a@b.c"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	must(t, st.Audit(ctx, model.AuditEntry{TenantID: ten.ID, Action: "scim.user.create", EntityType: "user"}))
	must(t, st.Audit(ctx, model.AuditEntry{TenantID: ten.ID, Action: "member.add", EntityType: "user"}))
	if es, _ := st.ListAuditByPrefix(ctx, ten.ID, "scim.", 0); len(es) != 1 {
		t.Fatal(es)
	}
	// expired tokens are not found
	exp := time.Now().Add(-time.Minute)
	must(t, st.CreateSCIMToken(ctx, &model.SCIMToken{TenantID: ten.ID, Name: "old", Hash: "h3", ExpiresAt: &exp}))
	if _, err := st.GetSCIMTokenByHash(ctx, "h3"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestSCIMBackupRoundTrip(t *testing.T) {
	src := store.New(testutil.OpenDB(t))
	ctx := context.Background()
	ten, err := src.CreateTenant(ctx, "acme", "Acme")
	must(t, err)
	u := &model.User{Email: "p@example.com"}
	must(t, src.CreateUser(ctx, u))
	must(t, src.CreateSCIMToken(ctx, &model.SCIMToken{TenantID: ten.ID, Name: "Okta", Hash: "h", Prefix: "p"}))
	must(t, src.SaveSCIMSettings(ctx, ten.ID, &model.SCIMSettings{DefaultRole: "tester"}))
	must(t, src.SaveSCIMUser(ctx, &model.SCIMUser{TenantID: ten.ID, UserID: u.ID, UserName: "p", Active: false}))
	must(t, src.CreateSCIMGroup(ctx, &model.SCIMGroup{TenantID: ten.ID, DisplayName: "G", Role: "tester", Members: []string{u.ID}}))
	b, m := backup(t, src)
	for _, tb := range []string{"scim_tokens", "scim_settings", "scim_users", "scim_groups", "scim_group_members"} {
		if m.Tables[tb] != 1 {
			t.Fatalf("%s: %d rows", tb, m.Tables[tb])
		}
	}
	dst := sqliteStore(t)
	if _, err := restore(dst, b); err != nil {
		t.Fatal(err)
	}
	su, err := dst.GetSCIMUser(ctx, ten.ID, u.ID)
	if err != nil || su.Active || su.UserName != "p" {
		t.Fatal(su, err)
	}
	if gs, _ := dst.ListSCIMGroups(ctx, ten.ID); len(gs) != 1 || len(gs[0].Members) != 1 || gs[0].Role != "tester" {
		t.Fatal(gs)
	}
}
