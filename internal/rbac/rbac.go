// Package rbac implements role based access control for tenant members.
//
// A user holds one or more roles per tenant (and, as a project member, one
// or more project roles); the effective permissions are the union of the
// permissions of the held roles (see Set.PermissionsOfAll). A role is a
// named set of permissions. Seven roles are built in (admin, manager, test
// lead, tester, subject matter expert, reviewer, viewer); each tenant can
// change the permissions of every built-in role except admin, and create
// its own roles (see Set). Holding admin among one's roles makes a user an
// organisation administrator. Super administrators (a global user flag)
// implicitly hold all permissions in every tenant.
package rbac

import (
	"regexp"
	"sort"
	"strings"
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
	// ChangeCreate proposes changes to requirements and tests in change
	// requests (and assesses, applies and verifies them).
	ChangeCreate Permission = "change.create"
	// ChangeApprove decides change requests as a member of the change
	// control board and allows the emergency override of change control.
	ChangeApprove Permission = "change.approve"
	TestRead      Permission = "test.read"
	TestWrite     Permission = "test.write"
	TestReview    Permission = "test.review"
	RunRead       Permission = "run.read"
	RunWrite      Permission = "run.write"
	RunExecute    Permission = "run.execute"
	// CommentCreate writes comments in discussions (and replies, resolves,
	// reopens and reacts); reading them needs the read permission of the
	// item.
	CommentCreate Permission = "comment.create"
	// CyclePlan creates and plans test cycles: their tests, testers,
	// estimates, runs and recurring schedules.
	CyclePlan Permission = "cycle.plan"
	// EnvironmentManage creates and changes test environments (their
	// components, status, health check and secret), approves their
	// bookings and reveals their secret.
	EnvironmentManage Permission = "environment.manage"
	ScriptExecute     Permission = "script.execute"
	Export            Permission = "export"
	AuditRead         Permission = "audit.read"
	TokenManage       Permission = "token.manage"
)

// AllPermissionList lists every permission in display order (grouped by
// resource).
var AllPermissionList = []Permission{
	ProjectRead, ProjectCreate, ProjectUpdate, ProjectDelete,
	RequirementRd, RequirementWr, RequirementRev, WaiverApprove, ChangeCreate, ChangeApprove,
	TestRead, TestWrite, TestReview,
	RunRead, RunWrite, RunExecute, CyclePlan, EnvironmentManage, ScriptExecute,
	CommentCreate, Export, AuditRead, TokenManage,
	MemberRead, MemberManage, RoleManage, SettingsManage, TenantManage,
}

// Built-in roles.
const (
	RoleAdmin    = "admin"
	RoleManager  = "manager"
	RoleTestLead = "test_lead"
	RoleTester   = "tester"
	// RoleSME is the subject matter expert: a domain expert who writes and
	// reviews requirements (see docs/security.md, "Subject matter expert").
	RoleSME      = "sme"
	RoleReviewer = "reviewer"
	RoleViewer   = "viewer"
)

// Roles lists the built-in roles from most to least privileged.
var Roles = []string{RoleAdmin, RoleManager, RoleTestLead, RoleTester, RoleSME, RoleReviewer, RoleViewer}

var readPerms = []Permission{ProjectRead, RequirementRd, TestRead, RunRead, MemberRead, Export}

func with(extra ...Permission) []Permission {
	return append(append([]Permission{}, readPerms...), extra...)
}

// defaults are the permissions of the built-in roles before a tenant
// changes them.
var defaults = map[string][]Permission{
	RoleViewer:   with(),
	RoleReviewer: with(RequirementRev, TestReview, CommentCreate),
	// The subject matter expert takes part in writing requirements: it
	// creates and edits them (text, criteria, rationale, sources, relations,
	// trace and item links, attachments), reviews them and discusses them,
	// and reads everything else in the project for context. It does not
	// write tests, execute or plan runs and cycles, approve waivers or
	// change requests (it proposes them: change.create), change
	// project settings or members, or create API tokens.
	RoleSME:      with(RequirementWr, RequirementRev, ChangeCreate, CommentCreate),
	RoleTester:   with(RunExecute, RunWrite, TestWrite, TokenManage, CommentCreate),
	RoleTestLead: with(RunExecute, RunWrite, CyclePlan, EnvironmentManage, TestWrite, TestReview, ScriptExecute, AuditRead, TokenManage, SettingsManage, CommentCreate, ChangeCreate, ChangeApprove),
	RoleManager: with(RunExecute, RunWrite, CyclePlan, EnvironmentManage, TestWrite, TestReview, RequirementWr, RequirementRev, WaiverApprove, ChangeCreate, ChangeApprove, ProjectCreate, CommentCreate,
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

// NormalizeRoles trims, de-duplicates and orders a list of role names:
// admin first, then the other built-in roles from most to least
// privileged, then custom roles alphabetically. Empty names are dropped.
// The first role of the result is the primary role (the one older clients
// see as "role"); a list holding admin always has admin as its primary
// role.
func NormalizeRoles(roles []string) []string {
	seen := map[string]bool{}
	builtin, custom := []string{}, []string{}
	for _, r := range roles {
		r = strings.TrimSpace(r)
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		if IsBuiltin(r) {
			builtin = append(builtin, r)
		} else {
			custom = append(custom, r)
		}
	}
	sort.SliceStable(builtin, func(i, j int) bool { return builtinOrder[builtin[i]] < builtinOrder[builtin[j]] })
	sort.Strings(custom)
	return append(builtin, custom...)
}

var builtinOrder = func() map[string]int {
	m := map[string]int{}
	for i, r := range Roles {
		m[r] = i
	}
	return m
}()

// Primary returns the primary role of a normalised role list ("" when
// empty).
func Primary(roles []string) string {
	if len(roles) == 0 {
		return ""
	}
	return roles[0]
}

// HasRole reports whether role is in roles.
func HasRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

// Diff returns the roles of b that are not in a (added) and those of a
// that are not in b (removed), both in NormalizeRoles order.
func Diff(a, b []string) (added, removed []string) {
	for _, r := range NormalizeRoles(b) {
		if !HasRole(a, r) {
			added = append(added, r)
		}
	}
	for _, r := range NormalizeRoles(a) {
		if !HasRole(b, r) {
			removed = append(removed, r)
		}
	}
	return added, removed
}

// DescribeChange describes a change of role sets for the audit log:
// "+sme −viewer" ("" when nothing changed).
func DescribeChange(before, after []string) string {
	added, removed := Diff(before, after)
	var parts []string
	for _, r := range added {
		parts = append(parts, "+"+r)
	}
	for _, r := range removed {
		parts = append(parts, "−"+r)
	}
	return strings.Join(parts, " ")
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

// ValidAll reports whether the list is not empty and every role exists in
// the set; it returns the first unknown role.
func (s Set) ValidAll(roles []string) (bool, string) {
	if len(roles) == 0 {
		return false, ""
	}
	for _, r := range roles {
		if !s.Valid(r) {
			return false, r
		}
	}
	return true, ""
}

// PermissionsOfAll returns the union of the permissions of several roles
// (sorted; unknown roles add nothing).
func (s Set) PermissionsOfAll(roles []string) []string {
	var ps []Permission
	for _, r := range roles {
		for _, q := range s[r].Permissions {
			ps = append(ps, Permission(q))
		}
	}
	return sortedStrings(ps)
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

// PermissionsOfAll returns the union of the default permissions of
// built-in roles.
func PermissionsOfAll(roles []string) []string { return builtins.PermissionsOfAll(roles) }

// AllPermissions returns every permission (used for super admins).
func AllPermissions() []string { return sortedStrings(AllPermissionList) }

// Rank returns the privilege rank of a built-in role.
func Rank(role string) int { return builtins.Rank(role) }

// Matrix returns the default role → permissions mapping.
func Matrix() map[string][]string { return builtins.Matrix() }
