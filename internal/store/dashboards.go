package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/model"
)

// Dashboards: layouts of widgets (see model.Dashboard). The widgets are
// checked by the caller (the API knows the widget types); the store keeps
// the structure (sizes and places) within the grid.

const dashboardCols = `id, project_id, owner_id, name, builtin, source_id, widgets, position, created_by, created_at, updated_at`

func scanDashboard(r interface{ Scan(...any) error }) (*model.Dashboard, error) {
	var d model.Dashboard
	var pid, owner sql.NullString
	var widgets string
	if err := r.Scan(&d.ID, &pid, &owner, &d.Name, &d.Builtin, &d.SourceID, &widgets, &d.Position, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return nil, err
	}
	d.ProjectID, d.OwnerID = pid.String, owner.String
	d.Shared = d.OwnerID == ""
	d.Stored = true
	_ = json.Unmarshal([]byte(widgets), &d.Widgets)
	if d.Widgets == nil {
		d.Widgets = []model.DashboardWidget{}
	}
	return &d, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ValidateDashboard normalises the name and checks the structure of the
// widgets: unique ids, every widget inside the grid. Overlaps are left to
// the client (it compacts the layout).
func ValidateDashboard(d *model.Dashboard) error {
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" || len([]rune(d.Name)) > 100 {
		return Invalidf("dashboard name is required (at most 100 characters)")
	}
	if d.Widgets == nil {
		d.Widgets = []model.DashboardWidget{}
	}
	if len(d.Widgets) > model.DashboardMaxWidgets {
		return Invalidf("a dashboard can have at most %d widgets", model.DashboardMaxWidgets)
	}
	seen := map[string]bool{}
	for i := range d.Widgets {
		w := &d.Widgets[i]
		w.ID = strings.TrimSpace(w.ID)
		if w.ID == "" {
			w.ID = newID()
		}
		if len(w.ID) > 64 || seen[w.ID] {
			return Invalidf("widget %d: the id must be unique (at most 64 characters)", i+1)
		}
		seen[w.ID] = true
		w.Title = strings.TrimSpace(w.Title)
		if len([]rune(w.Title)) > 200 {
			return Invalidf("widget %d: the title may have at most 200 characters", i+1)
		}
		if w.W < 1 || w.W > model.DashboardColumns || w.X < 0 || w.X+w.W > model.DashboardColumns {
			return Invalidf("widget %d: x and w must keep it within %d columns", i+1, model.DashboardColumns)
		}
		if w.H < 1 || w.H > model.DashboardMaxHeight || w.Y < 0 || w.Y > model.DashboardMaxRows {
			return Invalidf("widget %d: h must be 1 to %d and y 0 to %d", i+1, model.DashboardMaxHeight, model.DashboardMaxRows)
		}
		n := 0
		for _, v := range w.Settings {
			n += len(v)
		}
		if n > 16<<10 {
			return Invalidf("widget %d: the settings are too large (at most 16 KiB)", i+1)
		}
	}
	return nil
}

// ListDashboards returns the shared dashboards of a project and the
// personal ones of userID there: shared first, then by position.
func (s *Store) ListDashboards(ctx context.Context, tenantID, projectID, userID string) ([]model.Dashboard, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+dashboardCols+` FROM dashboards WHERE tenant_id = ? AND project_id = ? AND (owner_id IS NULL OR owner_id = ?)
		ORDER BY CASE WHEN owner_id IS NULL THEN 0 ELSE 1 END, position, created_at, id`, tenantID, projectID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Dashboard{}
	for rows.Next() {
		d, err := scanDashboard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// GetDashboard returns a stored dashboard (the caller checks who may see it).
func (s *Store) GetDashboard(ctx context.Context, tenantID, id string) (*model.Dashboard, error) {
	d, err := scanDashboard(s.c(ctx).row(ctx, `SELECT `+dashboardCols+` FROM dashboards WHERE id = ? AND tenant_id = ?`, id, tenantID))
	return d, mapErr(err, "dashboard")
}

// CreateDashboard stores a new project dashboard (shared when OwnerID is
// "") after the others of its kind, or a home dashboard (ProjectID "").
func (s *Store) CreateDashboard(ctx context.Context, tenantID string, d *model.Dashboard) error {
	if err := ValidateDashboard(d); err != nil {
		return err
	}
	if d.ProjectID == "" && d.OwnerID == "" {
		return Invalidf("a home dashboard needs an owner")
	}
	return s.tx(ctx, func(c conn) error {
		var n, pos int
		var err error
		switch {
		case d.ProjectID == "":
			err = c.row(ctx, `SELECT COUNT(*), COALESCE(MAX(position), 0) FROM dashboards WHERE tenant_id = ? AND project_id IS NULL AND owner_id = ?`, tenantID, d.OwnerID).Scan(&n, &pos)
			if err == nil && n > 0 {
				return Conflictf("the home dashboard already exists")
			}
		case d.OwnerID == "":
			err = c.row(ctx, `SELECT COUNT(*), COALESCE(MAX(position), 0) FROM dashboards WHERE tenant_id = ? AND project_id = ? AND owner_id IS NULL`, tenantID, d.ProjectID).Scan(&n, &pos)
		default:
			err = c.row(ctx, `SELECT COUNT(*), COALESCE(MAX(position), 0) FROM dashboards WHERE tenant_id = ? AND project_id = ? AND owner_id = ?`, tenantID, d.ProjectID, d.OwnerID).Scan(&n, &pos)
		}
		if err != nil {
			return err
		}
		if n >= model.DashboardMaxPerUser {
			return Invalidf("at most %d dashboards of this kind", model.DashboardMaxPerUser)
		}
		if d.Builtin != "" && d.OwnerID == "" {
			// a built-in layout is replaced at most once
			var k int
			if err := c.row(ctx, `SELECT COUNT(*) FROM dashboards WHERE tenant_id = ? AND project_id = ? AND owner_id IS NULL AND builtin = ?`, tenantID, d.ProjectID, d.Builtin).Scan(&k); err != nil {
				return err
			}
			if k > 0 {
				return Conflictf("the built-in dashboard %s is already replaced", d.Builtin)
			}
		}
		d.ID = newID()
		if d.Position == 0 {
			d.Position = pos + 1
		}
		d.CreatedAt = s.now()
		d.UpdatedAt = d.CreatedAt
		d.Shared, d.Stored = d.OwnerID == "", true
		widgets, _ := json.Marshal(d.Widgets)
		_, err = c.exec(ctx, `INSERT INTO dashboards (id, tenant_id, project_id, owner_id, name, builtin, source_id, widgets, position, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.ID, tenantID, nullIfEmpty(d.ProjectID), nullIfEmpty(d.OwnerID), d.Name, d.Builtin, d.SourceID, string(widgets), d.Position, d.CreatedBy, d.CreatedAt, d.UpdatedAt)
		return mapErr(err, "dashboard")
	})
}

// UpdateDashboard saves the name and the widgets of a stored dashboard.
func (s *Store) UpdateDashboard(ctx context.Context, tenantID string, d *model.Dashboard) error {
	if err := ValidateDashboard(d); err != nil {
		return err
	}
	d.UpdatedAt = s.now()
	widgets, _ := json.Marshal(d.Widgets)
	res, err := s.c(ctx).exec(ctx, `UPDATE dashboards SET name = ?, widgets = ?, updated_at = ? WHERE id = ? AND tenant_id = ?`, d.Name, string(widgets), d.UpdatedAt, d.ID, tenantID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "dashboard")
	}
	return nil
}

// DeleteDashboard removes a stored dashboard; a project default naming it
// is cleared.
func (s *Store) DeleteDashboard(ctx context.Context, tenantID, id string) error {
	return s.tx(ctx, func(c conn) error {
		res, err := c.exec(ctx, `DELETE FROM dashboards WHERE id = ? AND tenant_id = ?`, id, tenantID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return mapErr(sql.ErrNoRows, "dashboard")
		}
		_, err = c.exec(ctx, `DELETE FROM dashboard_defaults WHERE tenant_id = ? AND dashboard_id = ?`, tenantID, id)
		return err
	})
}

// HomeDashboard returns the stored home dashboard of a user (ErrNotFound:
// none, the built-in one applies).
func (s *Store) HomeDashboard(ctx context.Context, tenantID, userID string) (*model.Dashboard, error) {
	d, err := scanDashboard(s.c(ctx).row(ctx, `SELECT `+dashboardCols+` FROM dashboards WHERE tenant_id = ? AND project_id IS NULL AND owner_id = ?`, tenantID, userID))
	return d, mapErr(err, "home dashboard")
}

// DefaultDashboard returns the dashboard a project opens with ("" none set).
func (s *Store) DefaultDashboard(ctx context.Context, tenantID, projectID string) (string, error) {
	var id string
	err := s.c(ctx).row(ctx, `SELECT dashboard_id FROM dashboard_defaults WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID).Scan(&id)
	if err != nil && !db.IsNoRows(err) {
		return "", err
	}
	return id, nil
}

// SetDefaultDashboard sets the dashboard a project opens with ("" clears it).
func (s *Store) SetDefaultDashboard(ctx context.Context, tenantID, projectID, dashboardID, userID string) error {
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `DELETE FROM dashboard_defaults WHERE tenant_id = ? AND project_id = ?`, tenantID, projectID); err != nil {
			return err
		}
		if dashboardID == "" {
			return nil
		}
		_, err := c.exec(ctx, `INSERT INTO dashboard_defaults (project_id, tenant_id, dashboard_id, updated_by, updated_at) VALUES (?, ?, ?, ?, ?)`,
			projectID, tenantID, dashboardID, userID, s.now())
		return err
	})
}
