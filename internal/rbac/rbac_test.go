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
	// Higher roles are supersets of lower roles.
	for i := 0; i+1 < len(Roles); i++ {
		hi, lo := Roles[i], Roles[i+1]
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
		RunRead, RunWrite, RunExecute, ScriptExecute, Export, AuditRead, TokenManage}
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
