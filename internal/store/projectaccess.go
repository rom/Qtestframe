package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

// ---------------------------------------------------------------- project access

// Project member types.
const (
	MemberUser  = "user"
	MemberGroup = "group"
)

// ValidProjectAccess reports whether mode is a project access mode.
func ValidProjectAccess(mode string) bool {
	return mode == model.ProjectAccessOrganisation || mode == model.ProjectAccessRestricted
}

// SetProjectAccess changes a project's access mode.
func (s *Store) SetProjectAccess(ctx context.Context, tenantID, projectID, mode string) error {
	if !ValidProjectAccess(mode) {
		return Invalidf("access must be %s or %s", model.ProjectAccessOrganisation, model.ProjectAccessRestricted)
	}
	res, err := s.c(ctx).exec(ctx, `UPDATE projects SET access = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`, mode, s.now(), projectID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "project")
	}
	return nil
}

// ProjectAccessMode returns a project's access mode.
func (s *Store) ProjectAccessMode(ctx context.Context, tenantID, projectID string) (string, error) {
	var mode string
	err := s.c(ctx).row(ctx, `SELECT access FROM projects WHERE id = ? AND tenant_id = ?`, projectID, tenantID).Scan(&mode)
	return mode, mapErr(err, "project")
}

// ProjectGrant is what the project memberships give one user in one
// project: the role of a direct membership and the roles of the SCIM
// groups (of the user) that are project members.
type ProjectGrant struct {
	UserRole   string
	GroupRoles []string
}

// Member reports whether the grant makes the user a project member.
func (g ProjectGrant) Member() bool { return g.UserRole != "" || len(g.GroupRoles) > 0 }

// grantRows reads (project_id, member_type, role) rows into grants.
func grantRows(rows *sql.Rows) (map[string]ProjectGrant, error) {
	defer rows.Close()
	out := map[string]ProjectGrant{}
	for rows.Next() {
		var pid, typ, role string
		if err := rows.Scan(&pid, &typ, &role); err != nil {
			return nil, err
		}
		g := out[pid]
		if typ == MemberUser {
			g.UserRole = role
		} else {
			g.GroupRoles = append(g.GroupRoles, role)
		}
		out[pid] = g
	}
	return out, rows.Err()
}

const grantSQL = `SELECT project_id, member_type, role FROM project_members WHERE tenant_id = ? AND (
	(member_type = 'user' AND member_id = ?) OR
	(member_type = 'group' AND member_id IN (SELECT group_id FROM scim_group_members WHERE tenant_id = ? AND user_id = ?)))`

// ProjectGrants returns the project grants of a user in every project of
// a tenant (projects without a grant are left out).
func (s *Store) ProjectGrants(ctx context.Context, tenantID, userID string) (map[string]ProjectGrant, error) {
	rows, err := s.c(ctx).query(ctx, grantSQL+` ORDER BY project_id, member_type DESC, role`, tenantID, userID, tenantID, userID)
	if err != nil {
		return nil, err
	}
	return grantRows(rows)
}

// ProjectGrantOf returns the grant of a user in one project.
func (s *Store) ProjectGrantOf(ctx context.Context, tenantID, projectID, userID string) (ProjectGrant, error) {
	rows, err := s.c(ctx).query(ctx, grantSQL+` AND project_id = ? ORDER BY member_type DESC, role`, tenantID, userID, tenantID, userID, projectID)
	if err != nil {
		return ProjectGrant{}, err
	}
	m, err := grantRows(rows)
	return m[projectID], err
}

// ProjectUserGrants returns the grant of every user with one in a project
// (directly or through SCIM groups), by user id.
func (s *Store) ProjectUserGrants(ctx context.Context, tenantID, projectID string) (map[string]ProjectGrant, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT member_id, 'user', role FROM project_members WHERE tenant_id = ? AND project_id = ? AND member_type = 'user'
		UNION ALL
		SELECT gm.user_id, 'group', pm.role FROM project_members pm JOIN scim_group_members gm ON gm.group_id = pm.member_id AND gm.tenant_id = pm.tenant_id
		WHERE pm.tenant_id = ? AND pm.project_id = ? AND pm.member_type = 'group'`, tenantID, projectID, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	return grantRows(rows)
}

// TenantProjectIDs returns the ids of a tenant's projects.
func (s *Store) TenantProjectIDs(ctx context.Context, tenantID string) ([]string, error) {
	modes, err := s.ProjectAccessModes(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(modes))
	for id := range modes {
		out = append(out, id)
	}
	return out, nil
}

// ProjectAccessModes returns the access mode of every project of a tenant.
func (s *Store) ProjectAccessModes(ctx context.Context, tenantID string) (map[string]string, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT id, access FROM projects WHERE tenant_id = ?`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, mode string
		if err := rows.Scan(&id, &mode); err != nil {
			return nil, err
		}
		out[id] = mode
	}
	return out, rows.Err()
}

const projectMemberSelect = `SELECT pm.project_id, pm.member_type, pm.member_id, pm.role, pm.added_by, pm.created_at, pm.updated_at,
	COALESCE(u.email, ''), COALESCE(u.display_name, g.display_name, ''), COALESCE(u.avatar, ''), COALESCE(m.role, '')
	FROM project_members pm
	LEFT JOIN users u ON pm.member_type = 'user' AND u.id = pm.member_id
	LEFT JOIN memberships m ON pm.member_type = 'user' AND m.tenant_id = pm.tenant_id AND m.user_id = pm.member_id
	LEFT JOIN scim_groups g ON pm.member_type = 'group' AND g.id = pm.member_id AND g.tenant_id = pm.tenant_id`

func scanProjectMember(r interface{ Scan(...any) error }) (*model.ProjectMember, error) {
	var m model.ProjectMember
	if err := r.Scan(&m.ProjectID, &m.Type, &m.ID, &m.Role, &m.AddedBy, &m.CreatedAt, &m.UpdatedAt, &m.Email, &m.Name, &m.Avatar, &m.OrgRole); err != nil {
		return nil, err
	}
	return &m, nil
}

// ListProjectMembers returns the members of a project (users by e-mail,
// then groups by name).
func (s *Store) ListProjectMembers(ctx context.Context, tenantID, projectID string) ([]model.ProjectMember, error) {
	rows, err := s.c(ctx).query(ctx, projectMemberSelect+` WHERE pm.tenant_id = ? AND pm.project_id = ? ORDER BY pm.member_type DESC, COALESCE(u.email, g.display_name, '')`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ProjectMember{}
	for rows.Next() {
		m, err := scanProjectMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// GetProjectMember returns one member of a project.
func (s *Store) GetProjectMember(ctx context.Context, tenantID, projectID, typ, id string) (*model.ProjectMember, error) {
	m, err := scanProjectMember(s.c(ctx).row(ctx, projectMemberSelect+` WHERE pm.tenant_id = ? AND pm.project_id = ? AND pm.member_type = ? AND pm.member_id = ?`, tenantID, projectID, typ, id))
	return m, mapErr(err, "project member")
}

// SetProjectMember adds a member to a project or changes its role; it
// reports whether the member was added. A user must be a member of the
// tenant, a group one of its SCIM groups.
func (s *Store) SetProjectMember(ctx context.Context, tenantID, projectID, typ, id, role, addedBy string) (bool, error) {
	role = strings.TrimSpace(role)
	if role == "" {
		return false, Invalidf("role is required")
	}
	created := false
	err := s.tx(ctx, func(c conn) error {
		var n int
		switch typ {
		case MemberUser:
			if err := c.row(ctx, `SELECT COUNT(*) FROM memberships WHERE tenant_id = ? AND user_id = ?`, tenantID, id).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				return Invalidf("the user is not a member of the organisation")
			}
		case MemberGroup:
			if err := c.row(ctx, `SELECT COUNT(*) FROM scim_groups WHERE tenant_id = ? AND id = ?`, tenantID, id).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				return Invalidf("no such group in the organisation")
			}
		default:
			return Invalidf("type must be user or group")
		}
		if err := c.row(ctx, `SELECT COUNT(*) FROM projects WHERE tenant_id = ? AND id = ?`, tenantID, projectID).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return mapErr(sql.ErrNoRows, "project")
		}
		now := s.now()
		res, err := c.exec(ctx, `UPDATE project_members SET role = ?, updated_at = ? WHERE tenant_id = ? AND project_id = ? AND member_type = ? AND member_id = ?`,
			role, now, tenantID, projectID, typ, id)
		if err != nil {
			return err
		}
		if k, _ := res.RowsAffected(); k > 0 {
			return nil
		}
		created = true
		_, err = c.exec(ctx, `INSERT INTO project_members (tenant_id, project_id, member_type, member_id, role, added_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			tenantID, projectID, typ, id, role, addedBy, now, now)
		return mapErr(err, "project member")
	})
	return created, err
}

// RemoveProjectMember removes a member from a project.
func (s *Store) RemoveProjectMember(ctx context.Context, tenantID, projectID, typ, id string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM project_members WHERE tenant_id = ? AND project_id = ? AND member_type = ? AND member_id = ?`, tenantID, projectID, typ, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "project member")
	}
	return nil
}

// CopyProjectMembers copies the members of one project to another
// (duplicating a project keeps its sharing).
func (s *Store) CopyProjectMembers(ctx context.Context, tenantID, fromID, toID, addedBy string) error {
	now := s.now()
	_, err := s.c(ctx).exec(ctx, `INSERT INTO project_members (tenant_id, project_id, member_type, member_id, role, added_by, created_at, updated_at)
		SELECT tenant_id, ?, member_type, member_id, role, ?, ?, ? FROM project_members WHERE tenant_id = ? AND project_id = ?`,
		toID, addedBy, now, now, tenantID, fromID)
	return err
}

// ProjectOf runs a project lookup query (one ? for the id, one for the
// tenant) and returns the project id ("" for a resource of no project).
func (s *Store) ProjectOf(ctx context.Context, query, id, tenantID string) (string, error) {
	var pid string
	err := s.c(ctx).row(ctx, query, id, tenantID).Scan(&pid)
	return pid, mapErr(err, "resource")
}
