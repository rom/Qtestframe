package api

import (
	"fmt"
	"net/http"

	"github.com/rom/qtestframe/internal/rbac"
)

type bulkMember struct {
	UserID string `json:"user_id"`
	Email  string `json:"email,omitempty"`
	Role   string `json:"role,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// handleBulkMembers: POST /members/bulk
// {"user_ids": [...], "action": "role"|"remove", "role"?} changes the role
// of, or removes, several members at once (all or nothing). Your own
// membership is skipped (reason "self"), as are users who are not members
// ("not_found") and members who already have the role ("unchanged"). A
// change that would leave the tenant without an admin is refused (409).
func (s *Server) handleBulkMembers(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		UserIDs []string `json:"user_ids"`
		Action  string   `json:"action"`
		Role    string   `json:"role"`
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
	switch req.Action {
	case "role":
		if ok, err := s.validRole(ctx, p.TenantID(), req.Role); err != nil || !ok {
			s.fail(w, r, errOr(err, badRequest("invalid role %q", req.Role)))
			return
		}
	case "remove":
	default:
		s.fail(w, r, badRequest("action must be role or remove"))
		return
	}
	members, err := s.Store.ListTenantMembers(ctx, p.TenantID())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	admins := 0
	byID := map[string]bulkMember{}
	for _, m := range members {
		byID[m.UserID] = bulkMember{UserID: m.UserID, Email: m.Email, Role: m.Role}
		if m.Role == rbac.RoleAdmin {
			admins++
		}
	}
	changed, skipped := []bulkMember{}, []bulkMember{}
	var plan []bulkMember
	for _, id := range ids {
		m, ok := byID[id]
		switch {
		case !ok:
			skipped = append(skipped, bulkMember{UserID: id, Reason: "not_found"})
			continue
		case id == p.User.ID:
			m.Reason = "self"
			skipped = append(skipped, m)
			continue
		case req.Action == "role" && m.Role == req.Role:
			m.Reason = "unchanged"
			skipped = append(skipped, m)
			continue
		}
		if m.Role == rbac.RoleAdmin && (req.Action == "remove" || req.Role != rbac.RoleAdmin) {
			admins--
		}
		plan = append(plan, m)
	}
	if admins < 1 {
		writeError(w, http.StatusConflict, "conflict", "a tenant needs at least one admin")
		return
	}
	for _, m := range plan {
		if req.Action == "remove" {
			if err := s.Store.RemoveMembership(ctx, p.TenantID(), m.UserID); err != nil {
				s.fail(w, r, fmt.Errorf("%s: %w", m.Email, err))
				return
			}
			s.audit(ctx, p, "member.remove", "user", m.UserID, m.Email+" (bulk)")
		} else {
			if err := s.Store.SetMembership(ctx, p.TenantID(), m.UserID, req.Role); err != nil {
				s.fail(w, r, fmt.Errorf("%s: %w", m.Email, err))
				return
			}
			s.audit(ctx, p, "member.role", "user", m.UserID, req.Role+" (bulk)")
			m.Role = req.Role
		}
		changed = append(changed, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": changed, "skipped": skipped})
}
