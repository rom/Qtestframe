package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Per-project access (project sharing).
//
// A project is either organisation-wide (model.ProjectAccessOrganisation:
// every member of the organisation sees it with their organisation role) or
// restricted (model.ProjectAccessRestricted: only its project members and the
// organisation's administrators see it). A project member is a user or a
// SCIM group with a project role, one of the organisation's roles.
//
// The effective permissions of a caller in a project:
//
//   - organisation administrators (the admin role; super administrators)
//     keep their organisation permissions in every project;
//   - a project member gets the project-scoped permissions of the project
//     roles (the union of the roles of the membership) instead of those of
//     the organisation roles, in both modes, so project roles can raise or
//     lower a user's rights in one project. A direct (user) membership wins
//     over group memberships; several groups give the union of their roles;
//   - anyone else gets the organisation role in an organisation-wide project
//     and no access (404) to a restricted one.
//
// Organisation-level permissions (members, roles, settings, tokens, audit,
// creating projects) always come from the organisation role. An API token
// never exceeds its own role, inside a project too.
//
// The check is central: authed resolves the project of every request —
// {pid}, or the project of the {id} resource (see projectOfRequest) — and
// scopes the principal's permissions to it before the route's permissions
// are checked, so handlers' p.Can checks are project-aware. Handlers that
// reach a second project (moves, cross-project links, lists across
// projects) check it with canIn or visibleProjects.

// projectScoped lists the permissions a project role governs.
var projectScoped = map[string]bool{
	string(rbac.ProjectRead): true, string(rbac.ProjectUpdate): true, string(rbac.ProjectDelete): true,
	string(rbac.RequirementRd): true, string(rbac.RequirementWr): true, string(rbac.RequirementRev): true, string(rbac.WaiverApprove): true,
	string(rbac.ChangeCreate): true, string(rbac.ChangeApprove): true,
	string(rbac.TestRead): true, string(rbac.TestWrite): true, string(rbac.TestReview): true,
	string(rbac.RunRead): true, string(rbac.RunWrite): true, string(rbac.RunExecute): true, string(rbac.CyclePlan): true,
	string(rbac.EnvironmentManage): true,
	string(rbac.ScriptExecute):     true, string(rbac.Export): true, string(rbac.CommentCreate): true,
}

// adminUser reports whether the caller is an organisation administrator
// (or a super administrator); an API token keeps its own role as the cap.
func (p *Principal) adminUser() bool {
	if p.User.SuperAdmin {
		return true
	}
	return rbac.HasRole(p.memberRoles, rbac.RoleAdmin)
}

// orgPermissions returns the organisation-level effective permissions.
func (p *Principal) orgPermissions() []string {
	if p.User.SuperAdmin && p.Token == nil {
		return rbac.AllPermissions()
	}
	if p.orgPerms != nil {
		return p.orgPerms
	}
	return rbac.PermissionsOfAll(p.roleList())
}

// accessOf computes the caller's access to a project from its mode and the
// caller's grant there; nil means no access.
func (p *Principal) accessOf(mode string, g store.ProjectGrant) *model.ProjectAccessInfo {
	if p.Tenant == nil {
		return nil
	}
	org := p.orgPermissions()
	var acc *model.ProjectAccessInfo
	switch {
	case p.adminUser():
		acc = &model.ProjectAccessInfo{Source: "admin", Permissions: append([]string{}, org...)}
	case g.Member():
		acc = &model.ProjectAccessInfo{Source: "member", Roles: g.UserRoles}
		if len(g.UserRoles) == 0 {
			acc.Source, acc.Roles = "group", g.GroupRoles
		}
		acc.Role = strings.Join(acc.Roles, ", ")
		granted := p.roles.PermissionsOfAll(acc.Roles)
		acc.Permissions = mergeProjectPerms(granted, org, p.tokenPerms, p.Token != nil)
	case mode == model.ProjectAccessOrganisation:
		acc = &model.ProjectAccessInfo{Source: "organisation", Permissions: append([]string{}, org...)}
	default:
		return nil
	}
	acc.CanShare = slices.Contains(acc.Permissions, string(rbac.ProjectUpdate))
	return acc
}

// mergeProjectPerms: the project-scoped permissions of the project role
// (capped by an API token's role) and the organisation-level permissions of
// the organisation role.
func mergeProjectPerms(granted, org, token []string, isToken bool) []string {
	seen := map[string]bool{}
	cap := map[string]bool{}
	for _, t := range token {
		cap[t] = true
	}
	out := []string{}
	for _, q := range granted {
		if projectScoped[q] && !seen[q] && (!isToken || cap[q]) {
			seen[q] = true
			out = append(out, q)
		}
	}
	for _, q := range org {
		if !projectScoped[q] && !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	slices.Sort(out)
	return out
}

// projectAccess returns the caller's access to a project (nil: none);
// store.ErrNotFound when the project does not exist in the tenant.
func (s *Server) projectAccess(ctx context.Context, p *Principal, projectID string) (*model.ProjectAccessInfo, error) {
	if p.Tenant == nil {
		return nil, fmt.Errorf("project: %w", store.ErrNotFound)
	}
	mode, err := s.Store.ProjectAccessMode(ctx, p.TenantID(), projectID)
	if err != nil {
		return nil, err
	}
	var g store.ProjectGrant
	if !p.adminUser() {
		if g, err = s.Store.ProjectGrantOf(ctx, p.TenantID(), projectID, p.User.ID); err != nil {
			return nil, err
		}
	}
	return p.accessOf(mode, g), nil
}

// scope sets the principal's permissions to its access in a project.
func (p *Principal) scope(projectID string, acc *model.ProjectAccessInfo) {
	if p.orgPerms == nil {
		p.orgPerms = p.orgPermissions()
	}
	p.projectID = projectID
	p.perms = acc.Permissions
	p.access = acc
}

// errNoProject is the answer for a project the caller may not see: the
// same as for one that does not exist.
var errNoProject = fmt.Errorf("project: %w", store.ErrNotFound)

// canIn reports whether the caller holds perm in a project; it returns
// errNoProject when the caller may not see the project and errForbidden
// when perm is missing. perm "" only checks visibility.
func (s *Server) canIn(ctx context.Context, p *Principal, projectID string, perm rbac.Permission) error {
	if projectID == "" {
		return nil
	}
	if projectID == p.projectID && p.access != nil {
		return checkPerm(p.access.Permissions, p, perm)
	}
	acc, err := s.projectAccess(ctx, p, projectID)
	if err != nil {
		return err
	}
	if acc == nil {
		return errNoProject
	}
	return checkPerm(acc.Permissions, p, perm)
}

func checkPerm(perms []string, p *Principal, perm rbac.Permission) error {
	if perm == "" || (p.User.SuperAdmin && p.Token == nil) || slices.Contains(perms, string(perm)) {
		return nil
	}
	return fmt.Errorf("%w: permission %s required in the project", errForbidden, perm)
}

// requireOrg answers 403 for a caller outside any organisation (routes
// that filter by project access instead of a route permission).
func requireOrg(w http.ResponseWriter, p *Principal) bool {
	if p.Tenant == nil {
		writeError(w, http.StatusForbidden, "forbidden", "permission project.read required")
		return false
	}
	return true
}

// visibleProjects returns the access of the caller to every project of the
// tenant it may see, by project id.
func (s *Server) visibleProjects(ctx context.Context, p *Principal) (map[string]*model.ProjectAccessInfo, error) {
	out := map[string]*model.ProjectAccessInfo{}
	if p.Tenant == nil {
		return out, nil
	}
	modes, err := s.Store.ProjectAccessModes(ctx, p.TenantID())
	if err != nil {
		return nil, err
	}
	grants := map[string]store.ProjectGrant{}
	if !p.adminUser() {
		if grants, err = s.Store.ProjectGrants(ctx, p.TenantID(), p.User.ID); err != nil {
			return nil, err
		}
	}
	for id, mode := range modes {
		if acc := p.accessOf(mode, grants[id]); acc != nil {
			out[id] = acc
		}
	}
	return out, nil
}

// visibleIn reports whether the caller holds perm in the project (perm ""
// = sees it), from a visibleProjects result.
func visibleIn(vis map[string]*model.ProjectAccessInfo, p *Principal, projectID string, perm rbac.Permission) bool {
	acc := vis[projectID]
	return acc != nil && checkPerm(acc.Permissions, p, perm) == nil
}

// userCanSee reports whether another user (by id) may see a project with
// the given permission: used before notifying or e-mailing someone about a
// project. Errors count as no.
func (s *Server) userCanSee(ctx context.Context, tenantID, projectID, userID string, perm rbac.Permission) bool {
	if projectID == "" {
		return true
	}
	p, err := s.principalFor(ctx, tenantID, userID)
	if err != nil {
		return false
	}
	return s.canIn(ctx, p, projectID, perm) == nil
}

// membersIn returns the tenant members who may see a project, with their
// access there (by user id), and the members themselves.
func (s *Server) membersIn(ctx context.Context, tenantID, projectID string) (map[string]*model.ProjectAccessInfo, []model.Membership, error) {
	ms, err := s.Store.ListTenantMembers(ctx, tenantID)
	if err != nil {
		return nil, nil, err
	}
	set, err := s.Store.RoleSet(ctx, tenantID)
	if err != nil {
		return nil, nil, err
	}
	mode, err := s.Store.ProjectAccessMode(ctx, tenantID, projectID)
	if err != nil {
		return nil, nil, err
	}
	grants, err := s.Store.ProjectUserGrants(ctx, tenantID, projectID)
	if err != nil {
		return nil, nil, err
	}
	tenant := &model.Tenant{ID: tenantID}
	out := map[string]*model.ProjectAccessInfo{}
	for _, m := range ms {
		perms := set.PermissionsOfAll(m.Roles)
		p := &Principal{User: &model.User{ID: m.UserID}, Tenant: tenant, Role: m.Role, Roles: m.Roles, memberRoles: m.Roles, roles: set, perms: perms, orgPerms: perms}
		if acc := p.accessOf(mode, grants[m.UserID]); acc != nil {
			out[m.UserID] = acc
		}
	}
	return out, ms, nil
}

// principalFor builds the session-less principal of a tenant member.
func (s *Server) principalFor(ctx context.Context, tenantID, userID string) (*Principal, error) {
	u, err := s.Store.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u.Disabled {
		return nil, errors.New("the account is disabled")
	}
	t, err := s.Store.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	p := &Principal{User: u, Tenant: t}
	if m, err := s.Store.GetMembership(ctx, tenantID, userID); err == nil {
		p.setRoles(m.Roles)
	} else if u.SuperAdmin {
		p.setRoles([]string{rbac.RoleAdmin})
	} else {
		return nil, errors.New("no longer a member of the organisation")
	}
	if err := s.resolvePerms(ctx, p, nil); err != nil {
		return nil, err
	}
	return p, nil
}

// ---------------------------------------------------------------- request project

// resourceProjects maps the resource of an {id} route (the path after
// /api/v1/ up to {id}) to the query of its project: one ? for the id and
// one for the tenant. The route walk test (access_test.go) fails for an
// {id} route that is neither here nor in nonProjectResources.
var resourceProjects = map[string]string{
	"requirements":          `SELECT project_id FROM requirements WHERE id = ? AND tenant_id = ?`,
	"tests":                 `SELECT project_id FROM test_cases WHERE id = ? AND tenant_id = ?`,
	"runs":                  `SELECT project_id FROM test_runs WHERE id = ? AND tenant_id = ?`,
	"executions":            `SELECT r.project_id FROM test_executions e JOIN test_runs r ON r.id = e.run_id WHERE e.id = ? AND r.tenant_id = ?`,
	"step-results":          `SELECT r.project_id FROM step_results sr JOIN test_executions e ON e.id = sr.execution_id JOIN test_runs r ON r.id = e.run_id WHERE sr.id = ? AND r.tenant_id = ?`,
	"folders":               `SELECT project_id FROM folders WHERE id = ? AND tenant_id = ?`,
	"categories":            `SELECT project_id FROM categories WHERE id = ? AND tenant_id = ?`,
	"relations":             `SELECT project_id FROM requirement_relations WHERE id = ? AND tenant_id = ?`,
	"baselines":             `SELECT project_id FROM baselines WHERE id = ? AND tenant_id = ?`,
	"links":                 `SELECT project_id FROM trace_links WHERE id = ? AND tenant_id = ?`,
	"item-links":            `SELECT project_id FROM item_links WHERE id = ? AND tenant_id = ?`,
	"trash":                 `SELECT project_id FROM trash WHERE id = ? AND tenant_id = ?`,
	"releases":              `SELECT project_id FROM releases WHERE id = ? AND tenant_id = ?`,
	"defects":               `SELECT project_id FROM defects WHERE id = ? AND tenant_id = ?`,
	"risks":                 `SELECT project_id FROM risks WHERE id = ? AND tenant_id = ?`,
	"verification-records":  `SELECT project_id FROM verification_records WHERE id = ? AND tenant_id = ?`,
	"elements":              `SELECT project_id FROM system_elements WHERE id = ? AND tenant_id = ?`,
	"waivers":               `SELECT project_id FROM waivers WHERE id = ? AND tenant_id = ?`,
	"change-requests":       `SELECT project_id FROM change_requests WHERE id = ? AND tenant_id = ?`,
	"exploratory-sessions":  `SELECT project_id FROM exploratory_sessions WHERE id = ? AND tenant_id = ?`,
	"webhooks":              `SELECT project_id FROM webhooks WHERE id = ? AND tenant_id = ?`,
	"chat-channels":         `SELECT project_id FROM chat_channels WHERE id = ? AND tenant_id = ?`,
	"trace-rules":           `SELECT project_id FROM trace_rules WHERE id = ? AND tenant_id = ?`,
	"custom-fields":         `SELECT project_id FROM custom_fields WHERE id = ? AND tenant_id = ?`,
	"templates":             `SELECT project_id FROM item_templates WHERE id = ? AND tenant_id = ?`,
	"configurations":        `SELECT project_id FROM configurations WHERE id = ? AND tenant_id = ?`,
	"variants":              `SELECT project_id FROM variants WHERE id = ? AND tenant_id = ?`,
	"builds":                `SELECT project_id FROM builds WHERE id = ? AND tenant_id = ?`,
	"cycles":                `SELECT project_id FROM test_cycles WHERE id = ? AND tenant_id = ?`,
	"cycle-schedules":       `SELECT project_id FROM test_cycle_schedules WHERE id = ? AND tenant_id = ?`,
	"test-dependencies":     `SELECT project_id FROM test_dependencies WHERE id = ? AND tenant_id = ?`,
	"reports/definitions":   `SELECT project_id FROM report_definitions WHERE id = ? AND tenant_id = ?`,
	"reports/subscriptions": `SELECT project_id FROM report_subscriptions WHERE id = ? AND tenant_id = ?`,
	"reviews":               `SELECT project_id FROM review_requests WHERE id = ? AND tenant_id = ?`,
	"files":                 `SELECT COALESCE(project_id, '') FROM files WHERE id = ? AND tenant_id = ?`,
	"comments":              `SELECT COALESCE(project_id, '') FROM comments WHERE id = ? AND tenant_id = ?`,
	// shared environments (and their bookings) belong to no project
	"environments":               `SELECT COALESCE(project_id, '') FROM environments WHERE id = ? AND tenant_id = ?`,
	"bookings":                   `SELECT COALESCE(e.project_id, '') FROM environment_bookings b JOIN environments e ON e.id = b.environment_id WHERE b.id = ? AND b.tenant_id = ?`,
	"test-data-sets":             `SELECT project_id FROM test_data_sets WHERE id = ? AND tenant_id = ?`,
	"step-definitions":           `SELECT project_id FROM step_definitions WHERE id = ? AND tenant_id = ?`,
	"test-data-refresh-requests": `SELECT d.project_id FROM test_data_refresh_requests q JOIN test_data_sets d ON d.id = q.data_set_id WHERE q.id = ? AND q.tenant_id = ?`,
}

// nonProjectResources are {id} resources that belong to no project.
var nonProjectResources = map[string]bool{
	"auth/sessions": true, "auth/passkeys": true, "tenants": true, "users": true, "tokens": true, "admin/tenants": true,
	"scim/tokens": true, "scim/groups": true, "avatars": true,
	"catalog/requirements": true, "catalog/tests": true, "catalog/risks": true,
	// the organisation's field sets and templates per project type
	"field-sets/fields": true, "template-sets/templates": true,
	// scheduled backups of the whole installation (super administrators)
	"admin/backups": true,
}

// resourceOf returns the resource of a route pattern with {id} ("" for
// none): "GET /api/v1/reports/definitions/{id}/generate" → "reports/definitions".
func resourceOf(pattern string) string {
	_, path, _ := strings.Cut(pattern, " ")
	rest, ok := strings.CutPrefix(path, "/api/v1/")
	if !ok {
		return ""
	}
	before, _, ok := strings.Cut(rest, "/{id}")
	if !ok {
		return ""
	}
	return before
}

// projectOfRequest returns the project a request is about: {pid}, or the
// project of its {id} resource ("" when there is none or the resource does
// not exist — the handler then answers 404 itself).
func (s *Server) projectOfRequest(r *http.Request, p *Principal) (string, error) {
	if pid := r.PathValue("pid"); pid != "" {
		return pid, nil
	}
	id := r.PathValue("id")
	if id == "" {
		return "", nil
	}
	q, ok := resourceProjects[resourceOf(r.Pattern)]
	if !ok {
		return "", nil
	}
	pid, err := s.Store.ProjectOf(r.Context(), q, id, p.TenantID())
	if errors.Is(err, store.ErrNotFound) {
		return "", nil
	}
	return pid, err
}

// scopeRequest scopes the principal to the request's project; false when
// it answered (no access: 404).
func (s *Server) scopeRequest(w http.ResponseWriter, r *http.Request, p *Principal) bool {
	if p.Tenant == nil {
		return true
	}
	pid, err := s.projectOfRequest(r, p)
	if err != nil {
		s.fail(w, r, err)
		return false
	}
	if pid == "" {
		return true
	}
	acc, err := s.projectAccess(r.Context(), p, pid)
	if errors.Is(err, store.ErrNotFound) {
		return true // the handler answers 404
	}
	if err != nil {
		s.fail(w, r, err)
		return false
	}
	if acc == nil {
		s.fail(w, r, errNoProject)
		return false
	}
	p.scope(pid, acc)
	return true
}
