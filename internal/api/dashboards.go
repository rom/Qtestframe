package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Customisable dashboards (see docs/api.md "Dashboards"): the layouts of a
// project (shared ones, managed by the project's administrators, and
// personal ones), the default a project opens with, the personal
// cross-project home dashboard, and the widget data (dashboardwidgets.go).
// Until an administrator changes them, a project has the built-in layouts
// overview, trends and quality: the former fixed dashboard.

func (s *Server) routesDashboards(h routeFunc) {
	h("GET /api/v1/projects/{pid}/dashboards", s.handleListDashboards, "project.read")
	h("POST /api/v1/projects/{pid}/dashboards", s.handleCreateDashboard, "project.read") // shared: project.update
	h("PUT /api/v1/projects/{pid}/dashboards/default", s.handleSetDefaultDashboard, "project.update")
	h("GET /api/v1/projects/{pid}/dashboards/{did}", s.handleGetDashboard, "project.read")
	h("PUT /api/v1/projects/{pid}/dashboards/{did}", s.handleUpdateDashboard, "project.read") // shared: project.update
	h("DELETE /api/v1/projects/{pid}/dashboards/{did}", s.handleDeleteDashboard, "project.read")
	h("POST /api/v1/projects/{pid}/dashboards/{did}/reset", s.handleResetDashboard, "project.read")
	h("GET /api/v1/dashboards/home", s.handleGetHomeDashboard)
	h("PUT /api/v1/dashboards/home", s.handleSaveHomeDashboard)
	h("DELETE /api/v1/dashboards/home", s.handleResetHomeDashboard)
	h("GET /api/v1/dashboards/widgets", s.handleListWidgetTypes)
	h("POST /api/v1/dashboards/data", s.handleDashboardData) // each widget with the permissions of its data
}

// ---------------------------------------------------------------- built-in layouts

// BuiltinDashboards are the keys of the built-in project dashboards in
// their order.
var BuiltinDashboards = []string{"overview", "trends", "quality"}

// homeBuiltin is the key of the built-in home dashboard.
const homeBuiltin = "home"

func wg(id, typ string, x, y, w, h int, settings ...string) model.DashboardWidget {
	out := model.DashboardWidget{ID: id, Type: typ, X: x, Y: y, W: w, H: h}
	if len(settings) > 0 {
		out.Settings = map[string]json.RawMessage{}
		for i := 0; i+1 < len(settings); i += 2 {
			b, _ := json.Marshal(settings[i+1])
			out.Settings[settings[i]] = b
		}
	}
	return out
}

// builtinLayout returns the widgets of a built-in layout (nil: unknown key).
// overview, trends and quality are the sections of the former fixed
// project dashboard.
func builtinLayout(key string) []model.DashboardWidget {
	switch key {
	case "overview":
		return []model.DashboardWidget{
			wg("ov-kpis", "key_figures", 0, 0, 12, 2),
			wg("ov-mywork", "my_work", 0, 2, 12, 3),
			wg("ov-coverage", "coverage", 0, 5, 6, 4),
			wg("ov-results", "execution_results", 6, 5, 6, 4),
			wg("ov-active", "active_runs", 0, 9, 6, 4),
			wg("ov-recent", "recent_runs", 6, 9, 6, 4),
			wg("ov-reqstatus", "breakdown", 0, 13, 6, 4, "dimension", "requirement_status"),
			wg("ov-obligation", "breakdown", 6, 13, 6, 4, "dimension", "requirement_obligation", "chart", "hbar"),
			wg("ov-teststatus", "breakdown", 0, 17, 6, 4, "dimension", "test_status"),
			wg("ov-testtypes", "test_types", 6, 17, 6, 4),
			wg("ov-review", "breakdown", 0, 21, 6, 4, "dimension", "review_state", "chart", "hbar"),
			wg("ov-release", "release_progress", 6, 21, 6, 4),
		}
	case "trends":
		return []model.DashboardWidget{
			wg("tr-trend", "test_results_trend", 0, 0, 12, 5),
			wg("tr-activity", "recent_activity", 0, 5, 12, 5),
			wg("tr-weeks", "defects_per_week", 0, 10, 6, 4),
			wg("tr-burnup", "release_burnup", 6, 10, 6, 4),
		}
	case "quality":
		return []model.DashboardWidget{
			wg("qa-corr", "correlations", 0, 0, 12, 2),
			wg("qa-metrics", "quality_metrics", 0, 2, 12, 9),
			wg("qa-consistency", "consistency", 0, 11, 12, 5),
			wg("qa-flaky", "flaky_tests", 0, 16, 12, 5),
			wg("qa-severity", "breakdown", 0, 21, 6, 4, "dimension", "defect_severity"),
			wg("qa-groups", "coverage_by_group", 6, 21, 6, 4),
			wg("qa-risklevels", "breakdown", 0, 25, 6, 4, "dimension", "risk_level"),
			wg("qa-heat", "risk_heat", 6, 25, 6, 4),
			wg("qa-mitigation", "breakdown", 0, 29, 6, 4, "dimension", "risk_mitigation", "chart", "hbar"),
			wg("qa-age", "defect_age", 6, 29, 6, 4),
			wg("qa-rules", "trace_rules", 0, 33, 6, 4),
		}
	case homeBuiltin:
		return []model.DashboardWidget{
			wg("home-mywork", "my_work", 0, 0, 12, 5),
			wg("home-discussions", "discussions", 0, 5, 6, 5),
			wg("home-note", "note", 6, 5, 6, 5),
		}
	}
	return nil
}

// builtinDashboard is the unchanged built-in layout key of a project.
func builtinDashboard(key, projectID string) *model.Dashboard {
	return &model.Dashboard{ID: key, ProjectID: projectID, Builtin: key, Shared: true, Widgets: builtinLayout(key)}
}

// ---------------------------------------------------------------- project dashboards

// projectDashboards returns the dashboards the caller sees in a project:
// the built-in layouts (or the shared dashboards that replace them), the
// other shared ones, then the caller's personal ones; and the default.
func (s *Server) projectDashboards(r *http.Request, p *Principal, pid string) ([]model.Dashboard, string, error) {
	ctx := r.Context()
	stored, err := s.Store.ListDashboards(ctx, p.TenantID(), pid, p.User.ID)
	if err != nil {
		return nil, "", err
	}
	out := []model.Dashboard{}
	for _, key := range BuiltinDashboards {
		i := slices.IndexFunc(stored, func(d model.Dashboard) bool { return d.Shared && d.Builtin == key })
		if i >= 0 {
			out = append(out, stored[i])
			stored = slices.Delete(stored, i, i+1)
		} else {
			out = append(out, *builtinDashboard(key, pid))
		}
	}
	out = append(out, stored...)
	def, err := s.Store.DefaultDashboard(ctx, p.TenantID(), pid)
	if err != nil {
		return nil, "", err
	}
	defID := ""
	for i := range out {
		d := &out[i]
		if d.ID == def || (d.Shared && d.Builtin != "" && d.Builtin == def) {
			defID = d.ID
		}
	}
	if defID == "" {
		defID = out[0].ID
	}
	canManage := p.Can(rbac.ProjectUpdate)
	for i := range out {
		d := &out[i]
		d.Default = d.ID == defID
		d.Editable = !d.Shared || canManage
	}
	return out, defID, nil
}

// findDashboard returns the dashboard {did} of the project as the caller
// sees it (another user's personal dashboard: not found).
func (s *Server) findDashboard(r *http.Request, p *Principal, pid string) (*model.Dashboard, error) {
	all, _, err := s.projectDashboards(r, p, pid)
	if err != nil {
		return nil, err
	}
	did := r.PathValue("did")
	for i := range all {
		if all[i].ID == did {
			return &all[i], nil
		}
	}
	return nil, notFoundDashboard()
}

func notFoundDashboard() error { return &apiErr{http.StatusNotFound, "not_found", "dashboard not found"} }

func (s *Server) handleListDashboards(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	list, def, err := s.projectDashboards(r, p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "total": len(list), "default_id": def, "can_manage": p.Can(rbac.ProjectUpdate)})
}

func (s *Server) handleGetDashboard(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	d, err := s.findDashboard(r, p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// dashboardReq is the body of a create or update: the name, the widgets
// and (create) whether it is shared and what it was copied from.
type dashboardReq struct {
	Name     *string                  `json:"name"`
	Widgets  *[]model.DashboardWidget `json:"widgets"`
	Shared   bool                     `json:"shared"`
	SourceID string                   `json:"source_id"`
	// Format and Version: an imported export (checked when present).
	Format  string `json:"format"`
	Version int    `json:"version"`
}

func (req *dashboardReq) check() error {
	if req.Format != "" && req.Format != model.DashboardExportFormat {
		return badRequest("format must be %s", model.DashboardExportFormat)
	}
	if req.Version > 1 {
		return badRequest("the dashboard was exported by a newer version (format version %d)", req.Version)
	}
	return nil
}

func (s *Server) handleCreateDashboard(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req dashboardReq
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := req.check(); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Shared && !p.Can(rbac.ProjectUpdate) {
		writeError(w, http.StatusForbidden, "forbidden", "permission project.update required for a shared dashboard")
		return
	}
	d := &model.Dashboard{ProjectID: pr.ID, CreatedBy: p.User.Email, Widgets: []model.DashboardWidget{}}
	if !req.Shared {
		d.OwnerID = p.User.ID
	}
	if req.SourceID != "" {
		// a copy: of a dashboard the caller sees (its widgets unless given)
		all, _, err := s.projectDashboards(r, p, pr.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		i := slices.IndexFunc(all, func(x model.Dashboard) bool { return x.ID == req.SourceID })
		if i < 0 {
			s.fail(w, r, badRequest("source_id: no such dashboard"))
			return
		}
		src := all[i]
		d.Builtin, d.Widgets, d.Name = src.Builtin, src.Widgets, src.Name
		if src.Stored {
			d.SourceID = src.ID
		}
		if req.Shared {
			// a shared copy does not replace the built-in layout
			d.Builtin = ""
		}
	}
	if req.Name != nil {
		d.Name = *req.Name
	}
	if strings.TrimSpace(d.Name) == "" && d.Builtin != "" {
		d.Name = builtinNames[d.Builtin]
	}
	if req.Widgets != nil {
		d.Widgets = *req.Widgets
	}
	if err := checkWidgets(d.Widgets, false); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.CreateDashboard(r.Context(), p.TenantID(), d); err != nil {
		s.fail(w, r, err)
		return
	}
	if d.Shared {
		s.audit(r.Context(), p, "dashboard.create", "dashboard", d.ID, d.Name)
	}
	d.Editable = true
	writeJSON(w, http.StatusCreated, d)
}

// builtinNames name the copies of the built-in layouts (the client shows
// the built-in ones in the user's language).
var builtinNames = map[string]string{"overview": "Overview", "trends": "Trends", "quality": "Quality", homeBuiltin: "Home"}

func (s *Server) handleUpdateDashboard(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	d, err := s.findDashboard(r, p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !d.Editable {
		writeError(w, http.StatusForbidden, "forbidden", "permission project.update required to change a shared dashboard")
		return
	}
	var req dashboardReq
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := req.check(); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Name != nil {
		d.Name = *req.Name
	}
	if req.Widgets != nil {
		d.Widgets = *req.Widgets
	}
	if err := checkWidgets(d.Widgets, false); err != nil {
		s.fail(w, r, err)
		return
	}
	if d.Builtin != "" && strings.TrimSpace(d.Name) == "" {
		d.Name = builtinNames[d.Builtin]
	}
	if !d.Stored {
		// the first change of a built-in layout stores the shared
		// dashboard that replaces it
		d.CreatedBy = p.User.Email
		err = s.Store.CreateDashboard(r.Context(), p.TenantID(), d)
	} else {
		err = s.Store.UpdateDashboard(r.Context(), p.TenantID(), d)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if d.Shared {
		s.audit(r.Context(), p, "dashboard.update", "dashboard", d.ID, d.Name)
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDeleteDashboard(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	d, err := s.findDashboard(r, p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !d.Editable {
		writeError(w, http.StatusForbidden, "forbidden", "permission project.update required to delete a shared dashboard")
		return
	}
	if !d.Stored {
		s.fail(w, r, badRequest("a built-in dashboard cannot be deleted"))
		return
	}
	if err := s.Store.DeleteDashboard(r.Context(), p.TenantID(), d.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	if d.Shared {
		s.audit(r.Context(), p, "dashboard.delete", "dashboard", d.ID, d.Name)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleResetDashboard returns a dashboard to its default layout: a shared
// dashboard that replaces a built-in one to that layout (the stored one is
// removed), any other to the layout it was copied from (the built-in
// overview when that is gone).
func (s *Server) handleResetDashboard(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	all, _, err := s.projectDashboards(r, p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	i := slices.IndexFunc(all, func(x model.Dashboard) bool { return x.ID == r.PathValue("did") })
	if i < 0 {
		s.fail(w, r, notFoundDashboard())
		return
	}
	d := all[i]
	if !d.Editable {
		writeError(w, http.StatusForbidden, "forbidden", "permission project.update required to reset a shared dashboard")
		return
	}
	if !d.Stored {
		writeJSON(w, http.StatusOK, d)
		return
	}
	if d.Shared && d.Builtin != "" {
		if err := s.Store.DeleteDashboard(r.Context(), p.TenantID(), d.ID); err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(r.Context(), p, "dashboard.reset", "dashboard", d.ID, d.Name)
		out := builtinDashboard(d.Builtin, pr.ID)
		out.Editable, out.Default = true, d.Default
		writeJSON(w, http.StatusOK, out)
		return
	}
	layout := builtinLayout(d.Builtin)
	if src := slices.IndexFunc(all, func(x model.Dashboard) bool { return d.SourceID != "" && x.ID == d.SourceID }); src >= 0 {
		layout = all[src].Widgets
	}
	if layout == nil {
		layout = builtinLayout("overview")
	}
	d.Widgets = layout
	if err := s.Store.UpdateDashboard(r.Context(), p.TenantID(), &d); err != nil {
		s.fail(w, r, err)
		return
	}
	if d.Shared {
		s.audit(r.Context(), p, "dashboard.reset", "dashboard", d.ID, d.Name)
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleSetDefaultDashboard(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		DashboardID string `json:"dashboard_id"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	all, _, err := s.projectDashboards(r, p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id := req.DashboardID
	if id != "" {
		i := slices.IndexFunc(all, func(x model.Dashboard) bool { return x.ID == id })
		if i < 0 || !all[i].Shared {
			s.fail(w, r, badRequest("dashboard_id must name a shared dashboard of the project"))
			return
		}
		if all[i].Builtin != "" {
			// a built-in key stays the default when its layout is reset
			id = all[i].Builtin
		}
	}
	if err := s.Store.SetDefaultDashboard(r.Context(), p.TenantID(), pr.ID, id, p.User.Email); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "dashboard.default", "project", pr.ID, id)
	list, def, err := s.projectDashboards(r, p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "total": len(list), "default_id": def, "can_manage": true})
}

// ---------------------------------------------------------------- home dashboard

func (s *Server) homeDashboard(r *http.Request, p *Principal) (*model.Dashboard, error) {
	d, err := s.Store.HomeDashboard(r.Context(), p.TenantID(), p.User.ID)
	if errors.Is(err, store.ErrNotFound) {
		return &model.Dashboard{ID: homeBuiltin, Builtin: homeBuiltin, OwnerID: p.User.ID, Widgets: builtinLayout(homeBuiltin), Editable: true}, nil
	}
	if err != nil {
		return nil, err
	}
	d.Editable = true
	return d, nil
}

func (s *Server) handleGetHomeDashboard(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireOrg(w, p) {
		return
	}
	d, err := s.homeDashboard(r, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleSaveHomeDashboard(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireOrg(w, p) {
		return
	}
	d, err := s.homeDashboard(r, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req dashboardReq
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := req.check(); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.Name != nil {
		d.Name = *req.Name
	}
	if strings.TrimSpace(d.Name) == "" {
		d.Name = builtinNames[homeBuiltin]
	}
	if req.Widgets != nil {
		d.Widgets = *req.Widgets
	}
	if err := checkWidgets(d.Widgets, true); err != nil {
		s.fail(w, r, err)
		return
	}
	if d.Stored {
		err = s.Store.UpdateDashboard(r.Context(), p.TenantID(), d)
	} else {
		d.CreatedBy = p.User.Email
		err = s.Store.CreateDashboard(r.Context(), p.TenantID(), d)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleResetHomeDashboard(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireOrg(w, p) {
		return
	}
	d, err := s.Store.HomeDashboard(r.Context(), p.TenantID(), p.User.ID)
	if err == nil {
		err = s.Store.DeleteDashboard(r.Context(), p.TenantID(), d.ID)
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	d, err = s.homeDashboard(r, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}
