package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

// RoleSet returns the roles of a tenant: the built-in roles with the
// tenant's changes, plus its custom roles.
func (s *Store) RoleSet(ctx context.Context, tenantID string) (rbac.Set, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT name, label, description, permissions FROM roles WHERE tenant_id = ?`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []rbac.Role
	for rows.Next() {
		var r rbac.Role
		var perms string
		if err := rows.Scan(&r.Name, &r.Label, &r.Description, &perms); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(perms), &r.Permissions)
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rbac.Overlay(list), nil
}

// SaveRole creates or updates a role of a tenant. Built-in roles keep their
// name and label; admin cannot be changed. create reports whether a new
// custom role is expected (it must not exist yet).
func (s *Store) SaveRole(ctx context.Context, tenantID string, r rbac.Role, create bool) (*rbac.Role, error) {
	r.Name = strings.TrimSpace(r.Name)
	r.Label = strings.TrimSpace(r.Label)
	r.Description = strings.TrimSpace(r.Description)
	if r.Name == rbac.RoleAdmin {
		return nil, Invalidf("the admin role always has every permission and cannot be changed")
	}
	builtin := rbac.IsBuiltin(r.Name)
	if create && builtin {
		return nil, Conflictf("role %s already exists", r.Name)
	}
	if !builtin && !rbac.ValidName(r.Name) {
		return nil, Invalidf("role name must be 2–32 lower-case letters, digits or _ and start with a letter")
	}
	if len(r.Label) > 64 || len(r.Description) > 500 {
		return nil, Invalidf("role label or description is too long")
	}
	if !builtin && r.Label == "" {
		r.Label = r.Name
	}
	perms, bad := rbac.NormalizePermissions(r.Permissions)
	if bad != "" {
		return nil, Invalidf("unknown permission %q", bad)
	}
	r.Permissions = perms
	if builtin {
		r.Label = ""
	}
	raw, _ := json.Marshal(perms)
	err := s.tx(ctx, func(c conn) error {
		var n int
		if err := c.row(ctx, `SELECT COUNT(*) FROM roles WHERE tenant_id = ? AND name = ?`, tenantID, r.Name).Scan(&n); err != nil {
			return err
		}
		switch {
		case n > 0 && create:
			return Conflictf("role %s already exists", r.Name)
		case n == 0 && !create && !builtin:
			return fmt.Errorf("role: %w", ErrNotFound)
		case n > 0:
			_, err := c.exec(ctx, `UPDATE roles SET label = ?, description = ?, permissions = ?, updated_at = ? WHERE tenant_id = ? AND name = ?`,
				r.Label, r.Description, string(raw), s.now(), tenantID, r.Name)
			return err
		default:
			_, err := c.exec(ctx, `INSERT INTO roles (tenant_id, name, label, description, permissions, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				tenantID, r.Name, r.Label, r.Description, string(raw), s.now(), s.now())
			return mapErr(err, "role")
		}
	})
	if err != nil {
		return nil, err
	}
	set, err := s.RoleSet(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := set[r.Name]
	return &out, nil
}

// ResetRole restores the default permissions of a built-in role.
func (s *Store) ResetRole(ctx context.Context, tenantID, name string) error {
	if !rbac.IsBuiltin(name) {
		return Invalidf("only built-in roles can be reset")
	}
	_, err := s.c(ctx).exec(ctx, `DELETE FROM roles WHERE tenant_id = ? AND name = ?`, tenantID, name)
	return err
}

// RoleUsage counts the members and API tokens of a tenant per role.
func (s *Store) RoleUsage(ctx context.Context, tenantID string) (members, tokens map[string]int, err error) {
	members, tokens = map[string]int{}, map[string]int{}
	for _, q := range []struct {
		sql string
		out map[string]int
	}{
		{`SELECT role, COUNT(*) FROM memberships WHERE tenant_id = ? GROUP BY role`, members},
		{`SELECT role, COUNT(*) FROM api_tokens WHERE tenant_id = ? GROUP BY role`, tokens},
	} {
		rows, err := s.c(ctx).query(ctx, q.sql, tenantID)
		if err != nil {
			return nil, nil, err
		}
		for rows.Next() {
			var role string
			var n int
			if err := rows.Scan(&role, &n); err != nil {
				rows.Close()
				return nil, nil, err
			}
			q.out[role] = n
		}
		rows.Close()
	}
	return members, tokens, nil
}

// DeleteRole deletes a custom role that no member or API token uses.
func (s *Store) DeleteRole(ctx context.Context, tenantID, name string) error {
	if rbac.IsBuiltin(name) {
		return Invalidf("built-in roles cannot be deleted")
	}
	return s.tx(ctx, func(c conn) error {
		var n int
		if err := c.row(ctx, `SELECT (SELECT COUNT(*) FROM memberships WHERE tenant_id = ? AND role = ?) + (SELECT COUNT(*) FROM api_tokens WHERE tenant_id = ? AND role = ?) + (SELECT COUNT(*) FROM project_members WHERE tenant_id = ? AND role = ?)`,
			tenantID, name, tenantID, name, tenantID, name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return Conflictf("role %s is still used by %d members, project members or API tokens; give them another role first", name, n)
		}
		res, err := c.exec(ctx, `DELETE FROM roles WHERE tenant_id = ? AND name = ?`, tenantID, name)
		if err != nil {
			return err
		}
		if k, _ := res.RowsAffected(); k == 0 {
			return fmt.Errorf("role: %w", ErrNotFound)
		}
		return nil
	})
}

// TenantSettings returns a tenant's settings with defaults filled in.
func (s *Store) TenantSettings(ctx context.Context, tenantID string) (*model.TenantSettings, error) {
	var raw string
	if err := s.c(ctx).row(ctx, `SELECT settings FROM tenants WHERE id = ?`, tenantID).Scan(&raw); err != nil {
		return nil, mapErr(err, "tenant")
	}
	var st model.TenantSettings
	_ = json.Unmarshal([]byte(raw), &st)
	if len(st.ReadyRequirementStatuses) == 0 {
		st.ReadyRequirementStatuses = append([]string{}, model.DefaultReadyRequirementStatuses...)
	}
	return &st, nil
}

// SaveTenantSettings validates and stores a tenant's settings.
func (s *Store) SaveTenantSettings(ctx context.Context, tenantID string, st *model.TenantSettings) error {
	seen := map[string]bool{}
	var statuses []string
	for _, v := range st.ReadyRequirementStatuses {
		if !model.Valid(model.RequirementStatuses, v) {
			return Invalidf("unknown requirement status %q", v)
		}
		if !seen[v] {
			seen[v] = true
			statuses = append(statuses, v)
		}
	}
	if len(statuses) == 0 {
		statuses = append([]string{}, model.DefaultReadyRequirementStatuses...)
	}
	st.ReadyRequirementStatuses = statuses
	raw, _ := json.Marshal(st)
	res, err := s.c(ctx).exec(ctx, `UPDATE tenants SET settings = ? WHERE id = ?`, string(raw), tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("tenant: %w", ErrNotFound)
	}
	return nil
}
