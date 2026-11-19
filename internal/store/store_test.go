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

func setup(t *testing.T) (*store.Store, *model.Tenant, context.Context) {
	st := store.New(testutil.OpenDB(t))
	ctx := context.Background()
	ten, err := st.CreateTenant(ctx, "acme", "Acme")
	if err != nil {
		t.Fatal(err)
	}
	return st, ten, ctx
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestSessionsTokensAndClock(t *testing.T) {
	st, ten, ctx := setup(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	st.SetClock(func() time.Time { return now })
	u := &model.User{Email: "a@example.com"}
	must(t, st.CreateUser(ctx, u))
	if n, _ := st.CountUsers(ctx); n != 1 {
		t.Fatal(n)
	}
	if u.DisplayName != "a" || u.AuthProvider != "local" {
		t.Fatal(u)
	}
	if err := st.CreateUser(ctx, &model.User{Email: "A@example.com"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	must(t, st.CreateSession(ctx, &model.Session{ID: "s1", UserID: u.ID, TenantID: ten.ID, CSRF: "c", ExpiresAt: now.Add(time.Hour)}))
	must(t, st.CreateSession(ctx, &model.Session{ID: "s2", UserID: u.ID, CSRF: "c", ExpiresAt: now.Add(time.Minute)}))
	if _, err := st.GetSession(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := st.GetSession(ctx, "s2"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	if n, err := st.PurgeExpiredSessions(ctx); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	must(t, st.DeleteUserSessions(ctx, u.ID))

	exp := st.ExpiryIn(1)
	if st.ExpiryIn(0) != nil || exp == nil {
		t.Fatal("expiry")
	}
	tok := &model.APIToken{TenantID: ten.ID, UserID: u.ID, Name: "t", Role: "viewer", Hash: "h", Prefix: "p", ExpiresAt: exp}
	must(t, st.CreateToken(ctx, tok))
	if _, err := st.GetTokenByHash(ctx, "h"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(48 * time.Hour)
	if _, err := st.GetTokenByHash(ctx, "h"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := st.GetUserByExternal(ctx, "oidc:x", "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if err := st.UpdateUser(ctx, &model.User{ID: "missing"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if err := st.RemoveMembership(ctx, ten.ID, u.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := st.GetTenantBySlug(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := st.UpdateTenant(ctx, "missing", "x"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	must(t, st.Audit(ctx, model.AuditEntry{TenantID: ten.ID, Action: "a", EntityType: "e"}))
	if es, _ := st.ListAudit(ctx, ten.ID, 0); len(es) != 1 {
		t.Fatal(es)
	}
	must(t, st.DeleteTenant(ctx, ten.ID))
}

func TestMoveAcrossProjectsAndLinks(t *testing.T) {
	st, ten, ctx := setup(t)
	a := &model.Project{Key: "AA", Name: "A"}
	b := &model.Project{Key: "BB", Name: "B"}
	must(t, st.CreateProject(ctx, ten.ID, a, true))
	must(t, st.CreateProject(ctx, ten.ID, b, true))
	cats, _ := st.ListCategories(ctx, ten.ID, a.ID, model.KindTest)
	tc := &model.TestCase{ProjectID: a.ID, Title: "t", CategoryIDs: []string{cats[0].ID},
		Steps: []model.TestStep{{Action: "a"}, {Action: "b", Expected: "c"}}}
	must(t, st.CreateTest(ctx, ten.ID, tc))
	rq := &model.Requirement{ProjectID: a.ID, Title: "r"}
	must(t, st.CreateRequirement(ctx, ten.ID, rq))
	l := &model.TraceLink{RequirementID: rq.ID, TestCaseID: tc.ID}
	must(t, st.CreateLink(ctx, ten.ID, l))
	if got, err := st.GetLink(ctx, ten.ID, l.ID); err != nil || got.TestCaseID != tc.ID {
		t.Fatal(got, err)
	}
	if _, err := st.GetLink(ctx, ten.ID, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	must(t, st.MoveItems(ctx, ten.ID, model.KindTest, a.ID, []string{tc.ID}, nil, 0, b.ID))
	got, err := st.GetTest(ctx, ten.ID, tc.ID)
	must(t, err)
	bc, _ := st.ListCategories(ctx, ten.ID, b.ID, model.KindTest)
	if got.Key != "BB-TC-1" || len(got.CategoryIDs) != 1 || got.CategoryIDs[0] == cats[0].ID || len(got.Steps) != 2 {
		t.Fatal(got, bc)
	}
	if links, _ := st.ListLinks(ctx, ten.ID, "", rq.ID, ""); len(links) != 0 {
		t.Fatal("link should be dropped on cross-project move")
	}
	if err := st.MoveItems(ctx, ten.ID, "bogus", a.ID, []string{tc.ID}, nil, 0, ""); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := st.BulkUpdateItems(ctx, ten.ID, "bogus", a.ID, nil, store.BulkUpdate{}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if err := st.UpdateProject(ctx, ten.ID, &model.Project{ID: "missing", Name: "x"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if err := st.CreateFolder(ctx, ten.ID, &model.Folder{ProjectID: "missing", Kind: "test", Name: "x"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := st.GetCategory(ctx, ten.ID, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestDeriveStatusAndCoverage(t *testing.T) {
	sr := func(s ...string) []model.StepResult {
		var out []model.StepResult
		for _, x := range s {
			out = append(out, model.StepResult{Status: x})
		}
		return out
	}
	cases := map[string][]model.StepResult{
		model.ResultNotRun:     nil,
		model.ResultFailed:     sr("passed", "failed"),
		model.ResultBlocked:    sr("blocked", "passed"),
		model.ResultPassed:     sr("passed", "skipped"),
		model.ResultSkipped:    sr("skipped"),
		model.ResultInProgress: sr("passed", "not_run"),
	}
	for want, in := range cases {
		if got := store.DeriveStatus(in); got != want {
			t.Errorf("%v: got %s want %s", in, got, want)
		}
	}
	last := map[string]string{"a": "passed", "b": "failed", "c": "not_run"}
	for want, linked := range map[string][]string{"none": nil, "passed": {"a"}, "failed": {"a", "b"}, "partial": {"a", "c"}, "not_run": {"c"}} {
		if got := store.Coverage(linked, last); got != want {
			t.Errorf("%v: %s", linked, got)
		}
	}
	paths := store.FolderPaths([]model.Folder{{ID: "1", Name: "A"}, {ID: "2", Name: "B", ParentID: ptr("1")}, {ID: "3", Name: "C", ParentID: ptr("missing")}})
	if paths["2"] != "A / B" || paths["3"] != "C" {
		t.Fatal(paths)
	}
}

func ptr(s string) *string { return &s }
