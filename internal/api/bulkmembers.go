package api

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/rom/qtestframe/internal/rbac"
)

type bulkMember struct {
	UserID string   `json:"user_id"`
	Email  string   `json:"email,omitempty"`
	Role   string   `json:"role,omitempty"`
	Roles  []string `json:"roles,omitempty"`
	Reason string   `json:"reason,omitempty"`
}

// handleBulkMembers: POST /members/bulk {"user_ids": [...], "action":
// "role"|"add_role"|"remove_role"|"remove", "roles"? (or "role")} gives
// several members exactly the roles ("role"), adds roles to them
// ("add_role"), takes roles from them ("remove_role") or removes them
// (all or nothing). Your own membership is skipped (reason "self"), as are
// users who are not members ("not_found"), members provisioned by SCIM
// ("scim"), members whose roles would not change ("unchanged") and members
// who would be left without a role ("last_role"). A change that would
// leave the tenant without an admin is refused (409).
func (s *Server) handleBulkMembers(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		UserIDs []string `json:"user_ids"`
		Action  string   `json:"action"`
		Role    string   `json:"role"`
		Roles   []string `json:"roles"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ids, err := bulkIDs(req.UserIDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	given := requestedRoles(req.Role, req.Roles)
	switch {
	case bulkRoleAction(req.Action):
		if err := s.validRoles(ctx, p.TenantID(), given); err != nil {
			s.fail(w, r, err)
			return
		}
	case req.Action == "remove":
	default:
		s.fail(w, r, badRequest("action must be role, add_role, remove_role or remove"))
		return
	}
	members, err := s.Store.ListTenantMembers(ctx, p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	admins := 0
	byID := map[string]bulkMember{}
	scim := map[string]bool{} // provisioned by SCIM: managed in the identity provider
	for _, m := range members {
		byID[m.UserID] = bulkMember{UserID: m.UserID, Email: m.Email, Role: m.Role, Roles: m.Roles}
		if rbac.HasRole(m.Roles, rbac.RoleAdmin) {
			admins++
		}
		if m.SCIM {
			scim[m.UserID] = true
		}
	}
	type step struct {
		m    bulkMember
		next []string
	}
	changed, skipped := []bulkMember{}, []bulkMember{}
	var plan []step
	for _, id := range ids {
		m, ok := byID[id]
		var next []string
		if ok && req.Action != "remove" {
			next = applyRoleAction(req.Action, m.Roles, given)
		}
		switch {
		case !ok:
			skipped = append(skipped, bulkMember{UserID: id, Reason: "not_found"})
			continue
		case id == p.User.ID:
			m.Reason = "self"
			skipped = append(skipped, m)
			continue
		case scim[id]:
			m.Reason = "scim"
			skipped = append(skipped, m)
			continue
		case req.Action != "remove" && slices.Equal(next, m.Roles):
			m.Reason = "unchanged"
			skipped = append(skipped, m)
			continue
		case req.Action != "remove" && len(next) == 0:
			m.Reason = "last_role"
			skipped = append(skipped, m)
			continue
		}
		if rbac.HasRole(m.Roles, rbac.RoleAdmin) && !rbac.HasRole(next, rbac.RoleAdmin) {
			admins--
		}
		plan = append(plan, step{m, next})
	}
	if admins < 1 {
		writeError(w, http.StatusConflict, "conflict", "a tenant needs at least one admin")
		return
	}
	for _, st := range plan {
		m := st.m
		if req.Action == "remove" {
			if err := s.Store.RemoveMembership(ctx, p.TenantID(), m.UserID); err != nil {
				s.fail(w, r, fmt.Errorf("%s: %w", m.Email, err))
				return
			}
			s.audit(ctx, p, "member.remove", "user", m.UserID, m.Email+" (bulk)")
		} else {
			if err := s.Store.SetMembershipRoles(ctx, p.TenantID(), m.UserID, st.next); err != nil {
				s.fail(w, r, fmt.Errorf("%s: %w", m.Email, err))
				return
			}
			s.audit(ctx, p, "member.role", "user", m.UserID, m.Email+": "+rbac.DescribeChange(m.Roles, st.next)+" → "+rolesText(st.next)+" (bulk)")
			m.Roles, m.Role = st.next, rbac.Primary(st.next)
		}
		changed = append(changed, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": changed, "skipped": skipped})
}
