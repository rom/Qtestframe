// Package rbac implements role based access control for tenant members.
//
// Every user has exactly one role per tenant. A role is a named set of
// permissions. Six roles are built in (admin, manager, test lead, tester,
// reviewer, viewer); each tenant can change the permissions of every built-in role
// except admin, and create its own roles (see Set). Super administrators (a
// global user flag) implicitly hold all permissions in every tenant.
package rbac

import (
	"regexp"
	"sort"
)

// Permission is an action on a resource class.
type Permission string

// Permissions.
const (
	TenantManage   Permission = "tenant.manage"
	MemberRead     Permission = "member.read"
	MemberManage   Permission = "member.manage"
	RoleManage     Permission = "role.manage"
	SettingsManage Permission = "settings.manage"
	ProjectRead    Permission = "project.read"
	ProjectCreate  Permission = "project.create"
	ProjectUpdate  Permission = "project.update"
	ProjectDelete  Permission = "project.delete"
	RequirementRd  Permission = "requirement.read"
	RequirementWr  Permission = "requirement.write"
	RequirementRev Permission = "requirement.review"
	// WaiverApprove approves, rejects and revokes waivers and deviations
	// and signs the acceptance of residual risks.
	WaiverApprove Permission = "waiver.approve"
	TestRead      Permission = "test.read"
	TestWrite     Permission = "test.write"
	TestReview    Permission = "test.review"
	RunRead       Permission = "run.read"
	RunWrite      Permission = "run.write"
	RunExecute    Permission = "run.execute"
	ScriptExecute Permission = "script.execute"
	Export        Permission = "export"
	AuditRead     Permission = "audit.read"
	TokenManage   Permission = "token.manage"
)

// AllPermissionList lists every permission in display order (grouped by
// resource).
var AllPermissionList = []Permission{
	ProjectRead, ProjectCreate, ProjectUpdate, ProjectDelete,
	RequirementRd, RequirementWr, RequirementRev, WaiverApprove,
	TestRead, TestWrite, TestReview,
	RunRead, RunWrite, RunExecute, ScriptExecute,
	Export, AuditRead, TokenManage,
	MemberRead, MemberManage, RoleManage, SettingsManage, TenantManage,
}

// Built-in roles.
const (
	RoleAdmin    = "admin"
	RoleManager  = "manager"
	RoleTestLead = "test_lead"
	RoleTester   = "tester"
	RoleReviewer = "reviewer"
	RoleViewer   = "viewer"
)

// Roles lists the built-in roles from most to least privileged.
var Roles = []string{RoleAdmin, RoleManager, RoleTestLead, RoleTester, RoleReviewer, RoleViewer}

var readPerms = []Permission{ProjectRead, RequirementRd, TestRead, RunRead, MemberRead, Export}

func with(extra ...Permission) []Permission {
	return append(append([]Permission{}, readPerms...), extra...)
}

// defaults are the permissions of the built-in roles before a tenant
// changes them.
var defaults = map[string][]Permission{
	RoleViewer:   with(),
	RoleReviewer: with(RequirementRev, TestReview),
	RoleTester:   with(RunExecute, RunWrite, TestWrite, TokenManage),
	RoleTestLead: with(RunExecute, RunWrite, TestWrite, TestReview, ScriptExecute, AuditRead, TokenManage, SettingsManage),
	RoleManager: with(RunExecute, RunWrite, TestWrite, TestReview, RequirementWr, RequirementRev, WaiverApprove, ProjectCreate,
		ProjectUpdate, ScriptExecute, AuditRead, TokenManage, SettingsManage),
	RoleAdmin: append([]Permission{}, AllPermissionList...),
}

// Role is the definition of a role in a tenant.
type Role struct {
	Name        string   `json:"name"`
	Label       string   `json:"label,omitempty"`
	Description string   `json:"description,omitempty"`
	Permissions []string `json:"permissions"`
	// Builtin roles cannot be deleted; admin cannot be changed at all.
	Builtin bool `json:"builtin"`
	// Customized is set on a built-in role whose permissions differ from
	// the defaults of this version.
	Customized bool `json:"customized,omitempty"`
}

// Editable reports whether the role's permissions may be changed.
func (r Role) Editable() bool { return r.Name != RoleAdmin }

var nameRE = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

// ValidName reports whether name can name a custom role: 2–32 characters,
// lower-case letters, digits and "_", starting with a letter.
func ValidName(name string) bool { return nameRE.MatchString(name) }

// ValidPermission reports whether p is a known permission.
func ValidPermission(p string) bool {
	for _, q := range AllPermissionList {
		if string(q) == p {
			return true
		}
	}
	return false
}

// IsBuiltin reports whether name is a built-in role.
func IsBuiltin(name string) bool {
	_, ok := defaults[name]
	return ok
}

// DefaultPermissions returns the default permissions of a built-in role
// (sorted), or nil.
func DefaultPermissions(name string) []string {
	ps, ok := defaults[name]
	if !ok {
		return nil
	}
	return sortedStrings(ps)
}

func sortedStrings(ps []Permission) []string {
	out := make([]string, 0, len(ps))
	seen := map[Permission]bool{}
	for _, p := range ps {
		if !seen[p] {
			seen[p] = true
			out = append(out, string(p))
		}
	}
	sort.Strings(out)
	return out
}

// NormalizePermissions validates, de-duplicates and sorts a permission list.
// It returns the first unknown permission, if any.
func NormalizePermissions(ps []string) ([]string, string) {
	conv := make([]Permission, 0, len(ps))
	for _, p := range ps {
		if !ValidPermission(p) {
			return nil, p
		}
		conv = append(conv, Permission(p))
	}
	return sortedStrings(conv), ""
}

// Set is the resolved set of roles of a tenant: the built-in roles (with the
// tenant's changes) and the tenant's own roles.
type Set map[string]Role

// Builtins returns the built-in roles with their default permissions.
func Builtins() Set {
	s := Set{}
	for _, name := range Roles {
		s[name] = Role{Name: name, Permissions: DefaultPermissions(name), Builtin: true}
	}
	return s
}

// Overlay applies stored role rows (changed built-in roles and custom roles)
// to the built-in defaults. Rows for admin are ignored: admin always holds
// every permission.
func Overlay(rows []Role) Set {
	s := Builtins()
	for _, r := range rows {
		if r.Name == RoleAdmin {
			continue
		}
		perms, _ := NormalizePermissions(r.Permissions)
		if def, ok := s[r.Name]; ok && def.Builtin {
			def.Permissions = perms
			def.Customized = !equal(perms, DefaultPermissions(r.Name))
			if r.Description != "" {
				def.Description = r.Description
			}
			s[r.Name] = def
			continue
		}
		s[r.Name] = Role{Name: r.Name, Label: r.Label, Description: r.Description, Permissions: perms}
	}
	return s
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Valid reports whether role exists in the set.
func (s Set) Valid(role string) bool {
	_, ok := s[role]
	return ok
}

// Can reports whether role grants permission p.
func (s Set) Can(role string, p Permission) bool {
	for _, q := range s[role].Permissions {
		if q == string(p) {
			return true
		}
	}
	return false
}

// PermissionsOf returns the sorted permissions of a role (empty if unknown).
func (s Set) PermissionsOf(role string) []string {
	return append([]string{}, s[role].Permissions...)
}

// Rank orders roles by privilege: admin highest, then by the number of
// permissions; unknown roles rank 0. Used to pick the strongest of several
// roles mapped from identity-provider groups.
func (s Set) Rank(role string) int {
	r, ok := s[role]
	switch {
	case !ok:
		return 0
	case role == RoleAdmin:
		return len(AllPermissionList) + 1
	default:
		return len(r.Permissions)
	}
}

// Names returns the role names: built-in roles first (most privileged
// first), then custom roles alphabetically.
func (s Set) Names() []string {
	out := []string{}
	for _, r := range Roles {
		if _, ok := s[r]; ok {
			out = append(out, r)
		}
	}
	var custom []string
	for name, r := range s {
		if !r.Builtin {
			custom = append(custom, name)
		}
	}
	sort.Strings(custom)
	return append(out, custom...)
}

// List returns the roles in Names order.
func (s Set) List() []Role {
	out := make([]Role, 0, len(s))
	for _, n := range s.Names() {
		out = append(out, s[n])
	}
	return out
}

// Matrix returns role → permissions for display.
func (s Set) Matrix() map[string][]string {
	out := map[string][]string{}
	for n, r := range s {
		out[n] = append([]string{}, r.Permissions...)
	}
	return out
}

// Subset reports whether every permission of a is in b.
func Subset(a, b []string) bool {
	in := map[string]bool{}
	for _, p := range b {
		in[p] = true
	}
	for _, p := range a {
		if !in[p] {
			return false
		}
	}
	return true
}

// Intersect returns the permissions present in both lists (sorted).
func Intersect(a, b []string) []string {
	in := map[string]bool{}
	for _, p := range b {
		in[p] = true
	}
	out := []string{}
	for _, p := range a {
		if in[p] {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// The functions below use the built-in defaults; they serve places that
// have no tenant (configuration checks, command line).

var builtins = Builtins()

// ValidRole reports whether r is a built-in role.
func ValidRole(r string) bool { return builtins.Valid(r) }

// Can reports whether the built-in role grants permission p by default.
func Can(role string, p Permission) bool { return builtins.Can(role, p) }

// PermissionsOf returns the default permissions of a built-in role.
func PermissionsOf(role string) []string { return builtins.PermissionsOf(role) }

// AllPermissions returns every permission (used for super admins).
func AllPermissions() []string { return sortedStrings(AllPermissionList) }

// Rank returns the privilege rank of a built-in role.
func Rank(role string) int { return builtins.Rank(role) }

// Matrix returns the default role → permissions mapping.
func Matrix() map[string][]string { return builtins.Matrix() }
