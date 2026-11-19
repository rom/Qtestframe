// Package rbac implements role based access control for tenant members.
//
// Every user has exactly one role per tenant. Roles map to a fixed set of
// permissions. Super administrators (a global user flag) implicitly hold all
// permissions in every tenant.
package rbac

import "sort"

// Permission is an action on a resource class.
type Permission string

// Permissions.
const (
	TenantManage   Permission = "tenant.manage"
	MemberRead     Permission = "member.read"
	MemberManage   Permission = "member.manage"
	ProjectRead    Permission = "project.read"
	ProjectCreate  Permission = "project.create"
	ProjectUpdate  Permission = "project.update"
	ProjectDelete  Permission = "project.delete"
	RequirementRd  Permission = "requirement.read"
	RequirementWr  Permission = "requirement.write"
	TestRead       Permission = "test.read"
	TestWrite      Permission = "test.write"
	RunRead        Permission = "run.read"
	RunWrite       Permission = "run.write"
	RunExecute     Permission = "run.execute"
	ScriptExecute  Permission = "script.execute"
	Export         Permission = "export"
	AuditRead      Permission = "audit.read"
	TokenManage    Permission = "token.manage"
)

// Roles.
const (
	RoleAdmin   = "admin"
	RoleManager = "manager"
	RoleTester  = "tester"
	RoleViewer  = "viewer"
)

// Roles lists roles from most to least privileged.
var Roles = []string{RoleAdmin, RoleManager, RoleTester, RoleViewer}

var readPerms = []Permission{ProjectRead, RequirementRd, TestRead, RunRead, MemberRead, Export}

var matrix = map[string][]Permission{
	RoleViewer: readPerms,
	RoleTester: append(append([]Permission{}, readPerms...),
		RunExecute, RunWrite, TestWrite, TokenManage),
	RoleManager: append(append([]Permission{}, readPerms...),
		RunExecute, RunWrite, TestWrite, RequirementWr, ProjectCreate,
		ProjectUpdate, ScriptExecute, AuditRead, TokenManage),
	RoleAdmin: append(append([]Permission{}, readPerms...),
		RunExecute, RunWrite, TestWrite, RequirementWr, ProjectCreate,
		ProjectUpdate, ProjectDelete, ScriptExecute, AuditRead, TokenManage,
		MemberManage, TenantManage),
}

// ValidRole reports whether r is a known role.
func ValidRole(r string) bool {
	_, ok := matrix[r]
	return ok
}

// Can reports whether role grants permission p.
func Can(role string, p Permission) bool {
	for _, q := range matrix[role] {
		if q == p {
			return true
		}
	}
	return false
}

// PermissionsOf returns the sorted permission list of a role.
func PermissionsOf(role string) []string {
	out := make([]string, 0, len(matrix[role]))
	for _, p := range matrix[role] {
		out = append(out, string(p))
	}
	sort.Strings(out)
	return out
}

// AllPermissions returns every permission (used for super admins).
func AllPermissions() []string {
	return PermissionsOf(RoleAdmin)
}

// Rank returns the privilege rank of a role (higher is more privileged);
// unknown roles rank 0.
func Rank(role string) int {
	for i, r := range Roles {
		if r == role {
			return len(Roles) - i
		}
	}
	return 0
}

// Matrix returns a copy of the role → permissions mapping for display.
func Matrix() map[string][]string {
	out := map[string][]string{}
	for r := range matrix {
		out[r] = PermissionsOf(r)
	}
	return out
}
