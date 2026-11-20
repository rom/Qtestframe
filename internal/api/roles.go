package api

import (
	"net/http"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

// roleView is a role with its usage in the tenant.
type roleView struct {
	rbac.Role
	Editable  bool `json:"editable"`
	Deletable bool `json:"deletable"`
	Members   int  `json:"members"`
	Tokens    int  `json:"tokens"`
}

func (s *Server) routesRoles(h routeFunc) {
	h("GET /api/v1/roles", s.handleListRoles)
	h("POST /api/v1/roles", s.handleCreateRole, "role.manage")
	h("PUT /api/v1/roles/{name}", s.handleUpdateRole, "role.manage")
	h("POST /api/v1/roles/{name}/reset", s.handleResetRole, "role.manage")
	h("DELETE /api/v1/roles/{name}", s.handleDeleteRole, "role.manage")
	h("GET /api/v1/settings", s.handleGetSettings)
	h("PUT /api/v1/settings", s.handleUpdateSettings, "settings.manage")
}

func (s *Server) handleListRoles(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	set, err := s.Store.RoleSet(r.Context(), p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	members, tokens, err := s.Store.RoleUsage(r.Context(), p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]roleView, 0, len(set))
	for _, role := range set.List() {
		out = append(out, roleView{Role: role, Editable: role.Editable(), Deletable: !role.Builtin,
			Members: members[role.Name], Tokens: tokens[role.Name]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": len(out), "permissions": rbac.AllPermissionList})
}

// checkGrant refuses permissions the caller does not hold: nobody can give
// a role more than they have themselves.
func (s *Server) checkGrant(w http.ResponseWriter, r *http.Request, p *Principal, perms []string) bool {
	norm, bad := rbac.NormalizePermissions(perms)
	if bad != "" {
		s.fail(w, r, badRequest("unknown permission %q", bad))
		return false
	}
	if !rbac.Subset(norm, p.Permissions()) {
		writeError(w, http.StatusForbidden, "forbidden", "you can only grant permissions you hold yourself")
		return false
	}
	return true
}

func (s *Server) handleCreateRole(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	var req rbac.Role
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if !s.checkGrant(w, r, p, req.Permissions) {
		return
	}
	role, err := s.Store.SaveRole(r.Context(), p.TenantID(), rbac.Role{Name: req.Name, Label: req.Label, Description: req.Description, Permissions: req.Permissions}, true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "role.create", "role", role.Name, joinPerms(role.Permissions))
	writeJSON(w, http.StatusCreated, roleView{Role: *role, Editable: true, Deletable: true})
}

func (s *Server) handleUpdateRole(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	var req struct {
		Label       *string  `json:"label"`
		Description *string  `json:"description"`
		Permissions []string `json:"permissions"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	set, err := s.Store.RoleSet(r.Context(), p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	name := r.PathValue("name")
	cur, ok := set[name]
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "role not found")
		return
	}
	next := cur
	if req.Label != nil {
		next.Label = *req.Label
	}
	if req.Description != nil {
		next.Description = *req.Description
	}
	if req.Permissions != nil {
		// only the change has to be within the caller's permissions
		if !s.checkGrant(w, r, p, added(req.Permissions, cur.Permissions)) {
			return
		}
		next.Permissions = req.Permissions
	}
	role, err := s.Store.SaveRole(r.Context(), p.TenantID(), next, false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "role.update", "role", role.Name, joinPerms(role.Permissions))
	writeJSON(w, http.StatusOK, roleView{Role: *role, Editable: role.Editable(), Deletable: !role.Builtin})
}

func (s *Server) handleResetRole(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	name := r.PathValue("name")
	if name == rbac.RoleAdmin {
		s.fail(w, r, badRequest("the admin role cannot be changed"))
		return
	}
	if !s.checkGrant(w, r, p, rbac.DefaultPermissions(name)) {
		return
	}
	if err := s.Store.ResetRole(r.Context(), p.TenantID(), name); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "role.reset", "role", name, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteRole(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	name := r.PathValue("name")
	if err := s.Store.DeleteRole(r.Context(), p.TenantID(), name); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "role.delete", "role", name, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	st, err := s.Store.TenantSettings(r.Context(), p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireTenant(w, p) {
		return
	}
	var st model.TenantSettings
	if err := decode(r, &st); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.SaveTenantSettings(r.Context(), p.TenantID(), &st); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "settings.update", "tenant", p.TenantID(), settingsSummary(&st))
	writeJSON(w, http.StatusOK, st)
}

func added(next, cur []string) []string {
	have := map[string]bool{}
	for _, p := range cur {
		have[p] = true
	}
	var out []string
	for _, p := range next {
		if !have[p] {
			out = append(out, p)
		}
	}
	return out
}

func joinPerms(ps []string) string {
	out := ""
	for i, p := range ps {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}

func settingsSummary(st *model.TenantSettings) string {
	b := func(v bool) string {
		if v {
			return "yes"
		}
		return "no"
	}
	return "allow_unready_tests=" + b(st.AllowUnreadyTests) + " allow_unready_requirements=" + b(st.AllowUnreadyRequirements) +
		" allow_unmet_dependencies=" + b(st.AllowUnmetDependencies) + " ready_requirement_statuses=" + joinPerms(st.ReadyRequirementStatuses)
}

// requireTenant answers 400 when the caller has no current organisation.
func requireTenant(w http.ResponseWriter, p *Principal) bool {
	if p.Tenant == nil {
		writeError(w, http.StatusBadRequest, "no_tenant", "select an organisation first")
		return false
	}
	return true
}
