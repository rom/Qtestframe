package store

import (
	"encoding/json"
	"strings"

	"github.com/rom/qtestframe/internal/rbac"
)

// Role sets (migration 0052): memberships, project members, invitations and
// API tokens store all their roles as a JSON list in the roles column and
// the first of them, the primary role, in the role column. Rows from before
// the migration (or from an older backup) have roles = '[]' and read as
// [role].

// normRoles normalises a role set; it fails when the set is empty.
func normRoles(roles []string) ([]string, string, error) {
	rs := rbac.NormalizeRoles(roles)
	if len(rs) == 0 {
		return nil, "", Invalidf("at least one role is required")
	}
	return rs, encodeRoles(rs), nil
}

// encodeRoles returns the JSON of a role set.
func encodeRoles(roles []string) string {
	raw, _ := json.Marshal(rbac.NormalizeRoles(roles))
	return string(raw)
}

// decodeRoles reads a roles column; an empty list falls back to the
// primary role.
func decodeRoles(raw, role string) []string {
	var rs []string
	_ = json.Unmarshal([]byte(raw), &rs)
	rs = rbac.NormalizeRoles(rs)
	if len(rs) == 0 && role != "" {
		rs = []string{role}
	}
	if rs == nil {
		rs = []string{}
	}
	return rs
}

// likeRole returns a LIKE pattern (escape character !) that matches a
// roles column holding role.
func likeRole(role string) string {
	r := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(role)
	return `%"` + r + `"%`
}

// hasRoleSQL is a condition on a table's role and roles columns (prefix
// "" or "alias."): the row holds the role given as two arguments (the role
// and likeRole of it).
func hasRoleSQL(prefix string) string {
	return "(" + prefix + "role = ? OR " + prefix + "roles LIKE ? ESCAPE '!')"
}
