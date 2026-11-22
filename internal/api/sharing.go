package api

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Project sharing: the access mode of a project and its members (see
// access.go for the access model). Reading needs project.read in the
// project, changing project.update there; nobody can give a project role
// with project permissions they do not hold there themselves.

func (s *Server) routesSharing(h routeFunc) {
	h("GET /api/v1/projects/{pid}/access", s.handleGetProjectAccess, "project.read")
	h("PUT /api/v1/projects/{pid}/access", s.atomic(s.handlePutProjectAccess), "project.update")
	h("GET /api/v1/projects/{pid}/members", s.handleListProjectMembers, "project.read")
	h("GET /api/v1/projects/{pid}/members/candidates", s.handleProjectMemberCandidates, "project.update")
	h("POST /api/v1/projects/{pid}/members", s.atomic(s.handleAddProjectMember), "project.update")
	h("POST /api/v1/projects/{pid}/members/bulk", s.atomic(s.handleBulkProjectMembers), "project.update")
	h("PATCH /api/v1/projects/{pid}/members/{mid}", s.atomic(s.handleUpdateProjectMember), "project.update")
	h("DELETE /api/v1/projects/{pid}/members/{mid}", s.atomic(s.handleRemoveProjectMember), "project.read") // project.update, or leaving: see below
}

// keepSharing gives a loaded or duplicated project its sharing: a duplicate
// gets the members of the original (fromID); a restricted project keeps
// the caller in it (with their organisation role, unless they are an
// administrator or already a member).
func (s *Server) keepSharing(ctx context.Context, p *Principal, projectID, fromID string) error {
	if fromID != "" {
		if err := s.Store.CopyProjectMembers(ctx, p.TenantID(), fromID, projectID, p.User.Email); err != nil {
			return err
		}
	}
	mode, err := s.Store.ProjectAccessMode(ctx, p.TenantID(), projectID)
	if err != nil || mode != model.ProjectAccessRestricted || p.adminUser() || len(p.memberRoles) == 0 {
		return err
	}
	g, err := s.Store.ProjectGrantOf(ctx, p.TenantID(), projectID, p.User.ID)
	if err != nil || len(g.UserRoles) > 0 {
		return err
	}
	_, err = s.Store.SetProjectMemberRoles(ctx, p.TenantID(), projectID, store.MemberUser, p.User.ID, p.memberRoles, p.User.Email)
	return err
}

// accessResponse is the sharing state of a project.
type accessResponse struct {
	Access   string                   `json:"access"`
	Members  int                      `json:"members"`
	MyAccess *model.ProjectAccessInfo `json:"my_access"`
	// Roles the caller may give in the project.
	GrantableRoles []string `json:"grantable_roles"`
}

func (s *Server) accessState(ctx context.Context, p *Principal, pr *model.Project) (*accessResponse, error) {
	ms, err := s.Store.ListProjectMembers(ctx, p.TenantID(), pr.ID)
	if err != nil {
		return nil, err
	}
	return &accessResponse{Access: pr.Access, Members: len(ms), MyAccess: p.access, GrantableRoles: s.grantableRoles(p)}, nil
}

// grantableRoles lists the roles whose project permissions the caller
// holds in the scoped project (every role for administrators).
func (s *Server) grantableRoles(p *Principal) []string {
	out := []string{}
	if p.roles == nil {
		return out
	}
	for _, name := range p.roles.Names() {
		if s.mayGrant(p, name) {
			out = append(out, name)
		}
	}
	return out
}

func (s *Server) mayGrant(p *Principal, role string) bool {
	if !p.roles.Valid(role) {
		return false
	}
	if p.adminUser() && p.Token == nil {
		return true
	}
	var scoped []string
	for _, q := range p.roles.PermissionsOf(role) {
		if projectScoped[q] {
			scoped = append(scoped, q)
		}
	}
	return rbac.Subset(scoped, p.perms)
}

func (s *Server) handleGetProjectAccess(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	res, err := s.accessState(r.Context(), p, pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handlePutProjectAccess: PUT /projects/{pid}/access {"access":
// "organisation"|"restricted"}. Restricting a project keeps the caller in
// it: a caller who is neither an administrator nor a project member
// becomes a member with their organisation role.
func (s *Server) handlePutProjectAccess(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Access string `json:"access"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	if err := s.Store.SetProjectAccess(ctx, p.TenantID(), pr.ID, req.Access); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Access == model.ProjectAccessRestricted && !p.adminUser() && p.access != nil && p.access.Source == "organisation" && len(p.memberRoles) > 0 {
		if _, err := s.Store.SetProjectMemberRoles(ctx, p.TenantID(), pr.ID, store.MemberUser, p.User.ID, p.memberRoles, p.User.Email); err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(ctx, p, "project.member.add", "project", pr.ID, p.User.Email+" as "+rolesText(p.memberRoles)+" (restricting the project)")
	}
	if req.Access != pr.Access {
		s.audit(ctx, p, "project.access", "project", pr.ID, pr.Key+": "+pr.Access+" → "+req.Access)
	}
	pr.Access = req.Access
	if acc, err := s.projectAccess(ctx, p, pr.ID); err == nil && acc != nil {
		p.access, p.perms = acc, acc.Permissions
	}
	res, err := s.accessState(ctx, p, pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleListProjectMembers(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	ms, err := s.Store.ListProjectMembers(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(ms))
}

// memberCandidate is a user or group that can be added to a project.
type memberCandidate struct {
	Type     string   `json:"type"`
	ID       string   `json:"id"`
	Email    string   `json:"email,omitempty"`
	Name     string   `json:"display_name,omitempty"`
	Avatar   string   `json:"avatar,omitempty"`
	OrgRole  string   `json:"org_role,omitempty"`
	OrgRoles []string `json:"org_roles,omitempty"`
}

// handleProjectMemberCandidates: GET /projects/{pid}/members/candidates?q=
// lists the organisation's members and SCIM groups that are not project
// members yet (q filters by e-mail or name), at most 50.
func (s *Server) handleProjectMemberCandidates(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	ctx := r.Context()
	current, err := s.Store.ListProjectMembers(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	in := map[string]bool{}
	for _, m := range current {
		in[m.Type+":"+m.ID] = true
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	match := func(xs ...string) bool {
		if q == "" {
			return true
		}
		for _, x := range xs {
			if strings.Contains(strings.ToLower(x), q) {
				return true
			}
		}
		return false
	}
	out := []memberCandidate{}
	ms, err := s.Store.ListTenantMembers(ctx, p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, m := range ms {
		if !in[store.MemberUser+":"+m.UserID] && match(m.Email, m.Name) {
			out = append(out, memberCandidate{Type: store.MemberUser, ID: m.UserID, Email: m.Email, Name: m.Name, Avatar: m.Avatar, OrgRole: m.Role, OrgRoles: m.Roles})
		}
	}
	gs, err := s.Store.ListSCIMGroups(ctx, p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, g := range gs {
		if !in[store.MemberGroup+":"+g.ID] && match(g.DisplayName) {
			out = append(out, memberCandidate{Type: store.MemberGroup, ID: g.ID, Name: g.DisplayName})
		}
	}
	if len(out) > 50 {
		out = out[:50]
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

type memberReq struct {
	UserID  string   `json:"user_id"`
	Email   string   `json:"email"`
	GroupID string   `json:"group_id"`
	Role    string   `json:"role"`
	Roles   []string `json:"roles"`
}

// resolveMember finds the user (by id or e-mail; an existing member of
// the organisation) or group of a request.
func (s *Server) resolveMember(ctx context.Context, p *Principal, req memberReq) (typ, id, label string, err error) {
	switch {
	case req.GroupID != "":
		g, err := s.Store.GetSCIMGroup(ctx, p.TenantID(), req.GroupID)
		if err != nil {
			return "", "", "", badRequest("no such group in the organisation")
		}
		return store.MemberGroup, g.ID, g.DisplayName, nil
	case req.UserID != "" || req.Email != "":
		var u *model.User
		if req.UserID != "" {
			u, err = s.Store.GetUser(ctx, req.UserID)
		} else {
			u, err = s.Store.GetUserByEmail(ctx, req.Email)
		}
		if err == nil {
			_, err = s.Store.GetMembership(ctx, p.TenantID(), u.ID)
		}
		if err != nil {
			return "", "", "", badRequest("the user is not a member of the organisation (add them under Members first)")
		}
		return store.MemberUser, u.ID, u.Email, nil
	}
	return "", "", "", badRequest("user_id, email or group_id is required")
}

// checkProjectGrant checks that the caller may give (or take) each of the
// roles in the project.
func (s *Server) checkProjectGrant(p *Principal, roles ...string) error {
	if len(roles) == 0 {
		return badRequest("at least one role is required")
	}
	for _, role := range roles {
		if !p.roles.Valid(role) {
			return badRequest("invalid role %q", role)
		}
		if !s.mayGrant(p, role) {
			return fmt.Errorf("%w: the role %s has project permissions you do not hold in this project", errForbidden, role)
		}
	}
	return nil
}

// checkProjectChange checks a change of a member's project roles: the
// caller must be able to give every added role and to take every removed
// one (roles the member keeps are not checked).
func (s *Server) checkProjectChange(p *Principal, before, after []string) error {
	if len(after) == 0 {
		return badRequest("at least one role is required")
	}
	added, removed := rbac.Diff(before, after)
	for _, rs := range [][]string{added, removed} {
		if len(rs) == 0 {
			continue
		}
		if err := s.checkProjectGrant(p, rs...); err != nil {
			return err
		}
	}
	return nil
}

// handleAddProjectMember: POST /projects/{pid}/members {"user_id" |
// "email" | "group_id", "roles" (or "role")} adds a member (or changes the
// roles of one) and notifies an added user.
func (s *Server) handleAddProjectMember(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req memberReq
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	roles := requestedRoles(req.Role, req.Roles)
	if err := s.checkProjectGrant(p, roles...); err != nil {
		s.fail(w, r, err)
		return
	}
	typ, id, label, err := s.resolveMember(ctx, p, req)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	before, _ := s.Store.GetProjectMember(ctx, p.TenantID(), pr.ID, typ, id)
	if before != nil {
		if err := s.checkProjectChange(p, before.Roles, roles); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	created, err := s.Store.SetProjectMemberRoles(ctx, p.TenantID(), pr.ID, typ, id, roles, p.User.Email)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	switch {
	case created:
		s.audit(ctx, p, "project.member.add", "project", pr.ID, typ+" "+label+" as "+rolesText(roles))
		s.notifyShared(ctx, p, pr, typ, id, roles)
	case before != nil && rbac.DescribeChange(before.Roles, roles) != "":
		s.audit(ctx, p, "project.member.role", "project", pr.ID, typ+" "+label+": "+rbac.DescribeChange(before.Roles, roles)+" → "+rolesText(roles))
	}
	m, err := s.Store.GetProjectMember(ctx, p.TenantID(), pr.ID, typ, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, m)
}

// notifyShared tells a user (or the users of a group) that a project was
// shared with them (in the app; by e-mail per their preferences).
func (s *Server) notifyShared(ctx context.Context, p *Principal, pr *model.Project, typ, id string, roles []string) {
	users := []string{id}
	if typ == store.MemberGroup {
		g, err := s.Store.GetSCIMGroup(ctx, p.TenantID(), id)
		if err != nil {
			return
		}
		users = g.Members
	}
	users = slices.DeleteFunc(slices.Clone(users), func(u string) bool { return u == p.User.ID })
	s.notifyUsers(ctx, p, users, model.Notification{ProjectID: pr.ID, Event: "project.shared",
		Title: pr.Key + " " + pr.Name, Body: rolesBody(roles), EntityType: "project", EntityID: pr.ID})
}

// rolesBody is the body of a "project shared" notification.
func rolesBody(roles []string) string {
	if len(roles) == 1 {
		return "role: " + roles[0]
	}
	return "roles: " + rolesText(roles)
}

// projectMember loads {mid} (a user or group id) of the {pid} project.
func (s *Server) projectMember(w http.ResponseWriter, r *http.Request, p *Principal, pr *model.Project) (*model.ProjectMember, bool) {
	mid := r.PathValue("mid")
	m, err := s.Store.GetProjectMember(r.Context(), p.TenantID(), pr.ID, store.MemberUser, mid)
	if err != nil {
		m, err = s.Store.GetProjectMember(r.Context(), p.TenantID(), pr.ID, store.MemberGroup, mid)
	}
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	return m, true
}

func memberLabel(m *model.ProjectMember) string {
	if m.Email != "" {
		return m.Type + " " + m.Email
	}
	return m.Type + " " + m.Name
}

// handleUpdateProjectMember: PATCH /projects/{pid}/members/{mid}
// {"roles"} (or {"role"}) gives a member exactly these project roles.
func (s *Server) handleUpdateProjectMember(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	m, ok := s.projectMember(w, r, p, pr)
	if !ok {
		return
	}
	var req struct {
		Role  string   `json:"role"`
		Roles []string `json:"roles"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	roles := requestedRoles(req.Role, req.Roles)
	if err := s.checkProjectGrant(p, roles...); err != nil {
		s.fail(w, r, err)
		return
	}
	// nobody takes a role from someone they could not have given it to
	if err := s.checkProjectChange(p, m.Roles, roles); err != nil {
		s.fail(w, r, err)
		return
	}
	if _, err := s.Store.SetProjectMemberRoles(ctx, p.TenantID(), pr.ID, m.Type, m.ID, roles, p.User.Email); err != nil {
		s.fail(w, r, err)
		return
	}
	if change := rbac.DescribeChange(m.Roles, roles); change != "" {
		s.audit(ctx, p, "project.member.role", "project", pr.ID, memberLabel(m)+": "+change+" → "+rolesText(roles))
	}
	out, err := s.Store.GetProjectMember(ctx, p.TenantID(), pr.ID, m.Type, m.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRemoveProjectMember: DELETE /projects/{pid}/members/{mid}.
// Members may always leave; removing someone else needs a role at least
// as strong as theirs in the project.
func (s *Server) handleRemoveProjectMember(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	m, ok := s.projectMember(w, r, p, pr)
	if !ok {
		return
	}
	ctx := r.Context()
	if m.ID != p.User.ID {
		if !p.Can(rbac.ProjectUpdate) {
			writeError(w, http.StatusForbidden, "forbidden", "permission project.update required")
			return
		}
		if err := s.checkProjectGrant(p, m.Roles...); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if err := s.Store.RemoveProjectMember(ctx, p.TenantID(), pr.ID, m.Type, m.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "project.member.remove", "project", pr.ID, memberLabel(m)+" ("+rolesText(m.Roles)+")")
	w.WriteHeader(http.StatusNoContent)
}

type bulkProjectMember struct {
	ID     string   `json:"id"`
	Type   string   `json:"type"`
	Label  string   `json:"label"`
	Role   string   `json:"role,omitempty"`
	Roles  []string `json:"roles,omitempty"`
	Reason string   `json:"reason,omitempty"`
}

// handleBulkProjectMembers: POST /projects/{pid}/members/bulk {"ids":
// [user or group ids], "action": "role"|"add_role"|"remove_role"|"remove",
// "roles"? (or "role")} gives several project members exactly the roles,
// adds roles to them, takes roles from them or removes them (all or
// nothing). Members who are not in the project ("not_found"), whose roles
// would not change ("unchanged"), who would be left without a role
// ("last_role") or whose change the caller could not make ("forbidden")
// are skipped.
func (s *Server) handleBulkProjectMembers(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		IDs    []string `json:"ids"`
		Action string   `json:"action"`
		Role   string   `json:"role"`
		Roles  []string `json:"roles"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ids, err := bulkIDs(req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	given := requestedRoles(req.Role, req.Roles)
	switch {
	case bulkRoleAction(req.Action):
		if err := s.checkProjectGrant(p, given...); err != nil {
			s.fail(w, r, err)
			return
		}
	case req.Action == "remove":
	default:
		s.fail(w, r, badRequest("action must be role, add_role, remove_role or remove"))
		return
	}
	ctx := r.Context()
	members, err := s.Store.ListProjectMembers(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	byID := map[string]*model.ProjectMember{}
	for i := range members {
		byID[members[i].ID] = &members[i]
	}
	changed, skipped := []bulkProjectMember{}, []bulkProjectMember{}
	for _, id := range ids {
		m, ok := byID[id]
		if !ok {
			skipped = append(skipped, bulkProjectMember{ID: id, Reason: "not_found"})
			continue
		}
		item := bulkProjectMember{ID: m.ID, Type: m.Type, Label: strings.TrimPrefix(memberLabel(m), m.Type+" "), Role: m.Role, Roles: m.Roles}
		var next []string
		if req.Action != "remove" {
			next = applyRoleAction(req.Action, m.Roles, given)
		}
		switch {
		case req.Action != "remove" && slices.Equal(next, m.Roles):
			item.Reason = "unchanged"
		case req.Action != "remove" && len(next) == 0:
			item.Reason = "last_role"
		case req.Action == "remove" && m.ID != p.User.ID && s.checkProjectGrant(p, m.Roles...) != nil:
			item.Reason = "forbidden"
		case req.Action != "remove" && s.checkProjectChange(p, m.Roles, next) != nil:
			item.Reason = "forbidden"
		}
		if item.Reason != "" {
			skipped = append(skipped, item)
			continue
		}
		if req.Action != "remove" {
			if _, err := s.Store.SetProjectMemberRoles(ctx, p.TenantID(), pr.ID, m.Type, m.ID, next, p.User.Email); err != nil {
				s.fail(w, r, err)
				return
			}
			s.audit(ctx, p, "project.member.role", "project", pr.ID, memberLabel(m)+": "+rbac.DescribeChange(m.Roles, next)+" → "+rolesText(next)+" (bulk)")
			item.Roles, item.Role = next, rbac.Primary(next)
		} else {
			if err := s.Store.RemoveProjectMember(ctx, p.TenantID(), pr.ID, m.Type, m.ID); err != nil {
				s.fail(w, r, err)
				return
			}
			s.audit(ctx, p, "project.member.remove", "project", pr.ID, memberLabel(m)+" ("+rolesText(m.Roles)+", bulk)")
		}
		changed = append(changed, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": changed, "skipped": skipped})
}
