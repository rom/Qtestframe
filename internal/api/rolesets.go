package api

import (
	"context"
	"slices"
	"strings"

	"github.com/rom/qtestframe/internal/rbac"
)

// Several roles per member (organisation members, project members,
// invitations and API tokens). Requests name the roles as "roles": [...]
// or, as before, one "role"; "roles" wins when both are given. Responses
// carry both: "roles" and "role", the primary role (the first of the
// roles, deprecated).

// requestedRoles returns the normalised roles of a request.
func requestedRoles(role string, roles []string) []string {
	if len(roles) == 0 && strings.TrimSpace(role) != "" {
		roles = []string{role}
	}
	return rbac.NormalizeRoles(roles)
}

// validRoles checks that roles is not empty and that the tenant has every
// role in it.
func (s *Server) validRoles(ctx context.Context, tenantID string, roles []string) error {
	set, err := s.Store.RoleSet(ctx, tenantID)
	if err != nil {
		return err
	}
	if ok, bad := set.ValidAll(roles); !ok {
		if bad == "" {
			return badRequest("at least one role is required")
		}
		return badRequest("invalid role %q", bad)
	}
	return nil
}

// Bulk role actions on members: "role" gives exactly the given roles,
// "add_role" adds them, "remove_role" takes them away (a member keeps at
// least one role).
const (
	bulkSetRoles    = "role"
	bulkAddRoles    = "add_role"
	bulkRemoveRoles = "remove_role"
)

func bulkRoleAction(action string) bool {
	return action == bulkSetRoles || action == bulkAddRoles || action == bulkRemoveRoles
}

// applyRoleAction returns a member's roles after a bulk role action.
func applyRoleAction(action string, cur, given []string) []string {
	switch action {
	case bulkAddRoles:
		return rbac.NormalizeRoles(append(slices.Clone(cur), given...))
	case bulkRemoveRoles:
		return slices.DeleteFunc(slices.Clone(cur), func(r string) bool { return rbac.HasRole(given, r) })
	}
	return rbac.NormalizeRoles(given)
}

// rolesText is a role set for messages and the audit log ("sme, tester").
func rolesText(roles []string) string { return strings.Join(roles, ", ") }
