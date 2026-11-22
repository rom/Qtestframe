package rbac

import (
	"sort"
	"testing"
)

func TestValidRole(t *testing.T) {
	for _, r := range Roles {
		if !ValidRole(r) {
			t.Errorf("ValidRole(%q) = false", r)
		}
	}
	for _, r := range []string{"", "root", "Admin"} {
		if ValidRole(r) {
			t.Errorf("ValidRole(%q) = true", r)
		}
	}
}

func TestCan(t *testing.T) {
	cases := []struct {
		role string
		p    Permission
		want bool
	}{
		{RoleViewer, ProjectRead, true},
		{RoleViewer, TestWrite, false},
		{RoleViewer, Export, true},
		{RoleTester, RunExecute, true},
		{RoleTester, TokenManage, true},
		{RoleTester, ScriptExecute, false},
		{RoleTester, RequirementWr, false},
		{RoleManager, ScriptExecute, true},
		{RoleManager, AuditRead, true},
		{RoleManager, ProjectDelete, false},
		{RoleManager, MemberManage, false},
		{RoleAdmin, ProjectDelete, true},
		{RoleAdmin, MemberManage, true},
		{RoleAdmin, TenantManage, true},
		{"nobody", ProjectRead, false},
	}
	for _, c := range cases {
		if got := Can(c.role, c.p); got != c.want {
			t.Errorf("Can(%q, %q) = %v, want %v", c.role, c.p, got, c.want)
		}
	}
}

func TestPermissionsOf(t *testing.T) {
	for _, r := range Roles {
		ps := PermissionsOf(r)
		if len(ps) == 0 {
			t.Fatalf("no permissions for %s", r)
		}
		if !sort.StringsAreSorted(ps) {
			t.Errorf("%s permissions not sorted: %v", r, ps)
		}
		for _, p := range ps {
			if !Can(r, Permission(p)) {
				t.Errorf("PermissionsOf(%s) contains %s but Can is false", r, p)
			}
		}
	}
	if got := PermissionsOf("nobody"); len(got) != 0 {
		t.Errorf("unknown role permissions = %v", got)
	}
	// Higher roles are supersets of lower roles (the reviewer branches off
	// the viewer: it reviews but does not write).
	chain := []string{RoleAdmin, RoleManager, RoleTestLead, RoleTester, RoleViewer}
	for i := 0; i+1 < len(chain); i++ {
		hi, lo := chain[i], chain[i+1]
		for _, p := range PermissionsOf(lo) {
			if !Can(hi, Permission(p)) {
				t.Errorf("%s lacks %s held by %s", hi, p, lo)
			}
		}
	}
}

func TestAllPermissions(t *testing.T) {
	all := AllPermissions()
	want := []Permission{TenantManage, MemberRead, MemberManage, ProjectRead, ProjectCreate,
		ProjectUpdate, ProjectDelete, RequirementRd, RequirementWr, TestRead, TestWrite,
		RunRead, RunWrite, RunExecute, ScriptExecute, Export, AuditRead, TokenManage, RoleManage, SettingsManage, RequirementRev, TestReview, WaiverApprove, CommentCreate}
	if len(all) != len(want) {
		t.Fatalf("AllPermissions has %d entries, want %d: %v", len(all), len(want), all)
	}
	set := map[string]bool{}
	for _, p := range all {
		set[p] = true
	}
	for _, p := range want {
		if !set[string(p)] {
			t.Errorf("AllPermissions missing %s", p)
		}
	}
}

func TestRank(t *testing.T) {
	if !(Rank(RoleAdmin) > Rank(RoleManager) && Rank(RoleManager) > Rank(RoleTester) &&
		Rank(RoleTester) > Rank(RoleViewer) && Rank(RoleViewer) > 0) {
		t.Errorf("unexpected ranks: %d %d %d %d", Rank(RoleAdmin), Rank(RoleManager), Rank(RoleTester), Rank(RoleViewer))
	}
	if Rank("x") != 0 || Rank("") != 0 {
		t.Error("unknown role must rank 0")
	}
}

func TestMatrix(t *testing.T) {
	m := Matrix()
	if len(m) != len(Roles) {
		t.Fatalf("Matrix has %d roles", len(m))
	}
	for _, r := range Roles {
		if len(m[r]) != len(PermissionsOf(r)) {
			t.Errorf("Matrix[%s] mismatch", r)
		}
	}
	// Mutating the copy must not affect the source.
	m[RoleViewer][0] = "mutated"
	if PermissionsOf(RoleViewer)[0] == "mutated" {
		t.Error("Matrix returned shared slice")
	}
}

func TestTestLead(t *testing.T) {
	for p, want := range map[Permission]bool{SettingsManage: true, ScriptExecute: true, TestWrite: true, RequirementWr: false, RoleManage: false, MemberManage: false} {
		if Can(RoleTestLead, p) != want {
			t.Errorf("test lead %s = %v", p, !want)
		}
	}
	if !(Rank(RoleManager) > Rank(RoleTestLead) && Rank(RoleTestLead) > Rank(RoleTester)) {
		t.Error("test lead rank")
	}
}

func TestOverlayAndSet(t *testing.T) {
	s := Overlay([]Role{
		{Name: RoleAdmin, Permissions: []string{"export"}},                                  // ignored
		{Name: RoleTester, Permissions: []string{"test.read", "project.read", "test.read"}}, // narrowed
		{Name: RoleViewer, Permissions: DefaultPermissions(RoleViewer), Description: "read only"},
		{Name: "auditor", Label: "Auditor", Permissions: []string{"audit.read", "project.read", "bogus"}},
	})
	if !s.Can(RoleAdmin, RoleManage) || len(s.PermissionsOf(RoleAdmin)) != len(AllPermissionList) {
		t.Error("admin must keep all permissions")
	}
	if got := s.PermissionsOf(RoleTester); len(got) != 2 || !s[RoleTester].Customized || !s[RoleTester].Builtin {
		t.Errorf("tester: %+v", s[RoleTester])
	}
	if s[RoleViewer].Customized || s[RoleViewer].Description != "read only" {
		t.Errorf("viewer: %+v", s[RoleViewer])
	}
	a := s["auditor"]
	if a.Builtin || a.Label != "Auditor" || len(a.Permissions) != 0 && a.Permissions[0] == "bogus" {
		t.Errorf("auditor: %+v", a)
	}
	if !s.Valid("auditor") || s.Valid("nobody") || s.Can("nobody", ProjectRead) {
		t.Error("Valid/Can")
	}
	names := s.Names()
	if names[0] != RoleAdmin || names[len(names)-1] != "auditor" || len(s.List()) != len(names) {
		t.Errorf("names: %v", names)
	}
	if s.Rank(RoleAdmin) <= s.Rank(RoleManager) || s.Rank("nobody") != 0 || s.Rank(RoleTester) != 2 {
		t.Error("rank")
	}
	if len(s.Matrix()) != 7 || !Role(s[RoleTester]).Editable() || s[RoleAdmin].Editable() {
		t.Error("matrix/editable")
	}
}

func TestNamesAndPermissions(t *testing.T) {
	for n, want := range map[string]bool{"auditor": true, "qa_lead2": true, "A": false, "a": false, "1x": false, "x-y": false, "abcdefghijabcdefghijabcdefghijabc": false} {
		if ValidName(n) != want {
			t.Errorf("ValidName(%q)", n)
		}
	}
	if got, bad := NormalizePermissions([]string{"test.read", "export", "test.read"}); bad != "" || len(got) != 2 || got[0] != "export" {
		t.Errorf("%v %q", got, bad)
	}
	if _, bad := NormalizePermissions([]string{"x"}); bad != "x" {
		t.Error("bad permission not reported")
	}
	if !IsBuiltin(RoleTestLead) || IsBuiltin("auditor") || DefaultPermissions("auditor") != nil {
		t.Error("IsBuiltin/DefaultPermissions")
	}
	if !Subset([]string{"a"}, []string{"a", "b"}) || Subset([]string{"c"}, []string{"a"}) {
		t.Error("Subset")
	}
	if got := Intersect([]string{"b", "a", "c"}, []string{"a", "b"}); len(got) != 2 || got[0] != "a" {
		t.Errorf("Intersect %v", got)
	}
}

func TestReviewer(t *testing.T) {
	for p, want := range map[Permission]bool{RequirementRev: true, TestReview: true, RequirementWr: false, TestWrite: false, ProjectRead: true} {
		if Can(RoleReviewer, p) != want {
			t.Errorf("reviewer %s = %v", p, !want)
		}
	}
	if !Can(RoleManager, RequirementRev) || !Can(RoleTestLead, TestReview) || Can(RoleTestLead, RequirementRev) || Can(RoleTester, TestReview) {
		t.Error("review permissions of the other roles")
	}
}
