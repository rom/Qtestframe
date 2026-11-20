package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/tracker"
)

// Defects: reading needs project.read, reporting and editing run.execute
// (testers report defects), deleting and the tracker settings project.update.

var defectSchema = &query.Schema{
	Fields:  []string{"key", "title", "description", "status", "severity", "priority", "assignee", "external", "created_by", "id", "open", "linked"},
	Aliases: map[string]string{"external_key": "external"},
}

func (s *Server) routesDefects(h routeFunc) {
	h("GET /api/v1/projects/{pid}/defects", s.handleListDefects, "project.read")
	h("POST /api/v1/projects/{pid}/defects", s.handleCreateDefect, "run.execute")
	h("POST /api/v1/projects/{pid}/defects/bulk", s.handleBulkDefects, "run.execute")
	h("DELETE /api/v1/projects/{pid}/defects/bulk", s.handleBulkDeleteDefects, "project.update")
	h("POST /api/v1/projects/{pid}/defects/push", s.handleBulkPushDefects, "run.execute")
	h("GET /api/v1/defects/{id}", s.handleGetDefect, "project.read")
	h("PATCH /api/v1/defects/{id}", s.handleUpdateDefect, "run.execute")
	h("DELETE /api/v1/defects/{id}", s.handleDeleteDefect, "project.update")
	h("POST /api/v1/defects/{id}/links", s.handleDefectLink(true), "run.execute")
	h("DELETE /api/v1/defects/{id}/links", s.handleDefectLink(false), "run.execute")
	h("GET /api/v1/requirements/{id}/defects", s.handleDefectsOf(model.KindRequirement), "requirement.read")
	h("GET /api/v1/tests/{id}/defects", s.handleDefectsOf(model.KindTest), "test.read")
	h("GET /api/v1/executions/{id}/defects", s.handleDefectsOf("execution"), "run.read")
	h("GET /api/v1/projects/{pid}/defect-tracker", s.handleGetTracker, "project.read")
	h("PUT /api/v1/projects/{pid}/defect-tracker", s.handlePutTracker, "project.update")
	h("POST /api/v1/projects/{pid}/defect-tracker/check", s.handleCheckTracker, "project.update")
	h("POST /api/v1/defects/{id}/push", s.handlePushDefect, "run.execute")
	h("POST /api/v1/defects/{id}/sync", s.handleSyncDefect, "run.execute")
	h("POST /api/v1/projects/{pid}/defects/sync", s.handleSyncDefects, "run.execute")
	h("POST /api/v1/projects/{pid}/defects/import", s.handleImportDefect, "run.execute")
}

func defectRecord(d *model.Defect) query.MapRecord {
	open := "false"
	if store.DefectOpen(d.Status) {
		open = "true"
	}
	var linked []string
	for _, l := range d.Links {
		linked = append(linked, l.Key)
	}
	return query.MapRecord{"key": {d.Key}, "title": {d.Title}, "description": {d.Description}, "status": {d.Status}, "severity": {d.Severity},
		"priority": {d.Priority}, "assignee": {d.Assignee}, "external": {d.ExternalKey}, "created_by": {d.CreatedBy}, "id": {d.ID},
		"open": {open}, "linked": linked}
}

func (s *Server) handleListDefects(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	q, err := query.Parse(r.URL.Query().Get("q"), defectSchema)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.Store.ListDefects(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := []model.Defect{}
	for i := range list {
		if q.Match(defectRecord(&list[i])) {
			out = append(out, list[i])
		}
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

func (s *Server) handleCreateDefect(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req model.Defect
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	d := &model.Defect{ProjectID: pr.ID, Title: req.Title, Description: req.Description, Status: req.Status, Severity: req.Severity,
		Priority: req.Priority, Assignee: req.Assignee, ExternalKey: req.ExternalKey, ExternalURL: req.ExternalURL, CreatedBy: p.User.Email, Links: req.Links}
	if err := s.Store.CreateDefect(r.Context(), p.TenantID(), d); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "defect.create", "defect", d.ID, d.Key+" "+d.Title)
	s.notifyDefect(r, p, d, "defect.created")
	s.writeDefect(w, r, p, d.ID, http.StatusCreated)
}

func (s *Server) writeDefect(w http.ResponseWriter, r *http.Request, p *Principal, id string, code int) {
	d, err := s.Store.GetDefect(r.Context(), p.TenantID(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, code, d)
}

func (s *Server) handleGetDefect(w http.ResponseWriter, r *http.Request, p *Principal) {
	s.writeDefect(w, r, p, r.PathValue("id"), http.StatusOK)
}

func (s *Server) handleUpdateDefect(w http.ResponseWriter, r *http.Request, p *Principal) {
	d, err := s.Store.GetDefect(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req struct {
		Title, Description, Status, Severity, Priority, Assignee, ExternalKey, ExternalURL *string
	}
	body, err := readBody(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var raw map[string]*string
	if err := json.Unmarshal(body, &raw); err != nil {
		s.fail(w, r, badRequest("invalid JSON: %v", err))
		return
	}
	req.Title, req.Description, req.Status, req.Severity = raw["title"], raw["description"], raw["status"], raw["severity"]
	req.Priority, req.Assignee, req.ExternalKey, req.ExternalURL = raw["priority"], raw["assignee"], raw["external_key"], raw["external_url"]
	old, oldAssignee := d.Status, d.Assignee
	for _, f := range []struct{ src, dst *string }{{req.Title, &d.Title}, {req.Description, &d.Description}, {req.Status, &d.Status},
		{req.Severity, &d.Severity}, {req.Priority, &d.Priority}, {req.Assignee, &d.Assignee}, {req.ExternalKey, &d.ExternalKey}, {req.ExternalURL, &d.ExternalURL}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	if err := s.Store.UpdateDefect(r.Context(), p.TenantID(), d); err != nil {
		s.fail(w, r, err)
		return
	}
	detail := d.Key
	if old != d.Status {
		detail += " " + old + " → " + d.Status
		s.notifyDefect(r, p, d, "defect.status")
	}
	if oldAssignee != d.Assignee && d.Assignee != "" {
		s.notifyDefect(r, p, d, "defect.assigned")
	}
	s.notifyEvent(r, p, d.ProjectID, "defect.updated", map[string]any{"defect": d})
	s.audit(r.Context(), p, "defect.update", "defect", d.ID, detail)
	s.writeDefect(w, r, p, d.ID, http.StatusOK)
}

func (s *Server) handleDeleteDefect(w http.ResponseWriter, r *http.Request, p *Principal) {
	d, err := s.Store.GetDefect(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteDefect(r.Context(), p.TenantID(), d.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "defect.delete", "defect", d.ID, d.Key)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDefectLink(add bool) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		var l model.DefectLink
		if err := decode(r, &l); err != nil {
			s.fail(w, r, err)
			return
		}
		id := r.PathValue("id")
		var err error
		if add {
			err = s.Store.LinkDefect(r.Context(), p.TenantID(), id, l)
		} else {
			err = s.Store.UnlinkDefect(r.Context(), p.TenantID(), id, l)
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		action := "defect.link"
		if !add {
			action = "defect.unlink"
		}
		s.audit(r.Context(), p, action, "defect", id, l.EntityType+" "+l.EntityID)
		s.writeDefect(w, r, p, id, http.StatusOK)
	}
}

func (s *Server) handleDefectsOf(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		id := r.PathValue("id")
		if kind == "execution" {
			if _, err := s.Store.GetExecution(r.Context(), p.TenantID(), id); err != nil {
				s.fail(w, r, err)
				return
			}
		} else if err := s.itemExists(r.Context(), p, kind, id); err != nil {
			s.fail(w, r, err)
			return
		}
		list, err := s.Store.DefectsOf(r.Context(), p.TenantID(), kind, id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, listOf(list))
	}
}

// releaseDefects: the open defects of a release's scope (exit criterion no_open_defects).
func (s *Server) releaseDefects(r *http.Request, p *Principal, scope []string, level string) []store.ReleaseItem {
	list, err := s.Store.OpenDefectsOfRequirements(r.Context(), p.TenantID(), scope, level)
	if err != nil {
		return nil
	}
	out := []store.ReleaseItem{}
	for _, d := range list {
		out = append(out, store.ReleaseItem{Kind: model.KindDefect, ID: d.ID, Key: d.Key, Title: d.Title, Note: d.Severity})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// ---------------------------------------------------------------- tracker

func (s *Server) handleGetTracker(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	t, err := s.Store.DefectTracker(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	t.Token = ""
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handlePutTracker(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		model.DefectTracker
		ClearToken bool `json:"clear_token"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	t := req.DefectTracker
	if err := s.Store.SaveDefectTracker(r.Context(), p.TenantID(), pr.ID, &t, req.ClearToken); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "defect_tracker.update", "project", pr.ID, t.Type+" "+t.URL+" "+t.ProjectKey+t.Repo)
	t.Token = ""
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) trackerClient(r *http.Request, p *Principal, projectID string) (tracker.Client, *model.DefectTracker, error) {
	t, err := s.Store.DefectTracker(r.Context(), p.TenantID(), projectID)
	if err != nil {
		return nil, nil, err
	}
	c, err := tracker.New(t, s.HTTPClient)
	if err != nil {
		return nil, nil, badRequest("%v", err)
	}
	return c, t, nil
}

// trackerFail reports an error of the external tracker as 502.
func (s *Server) trackerFail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, tracker.ErrBadKey) {
		s.fail(w, r, badRequest("%v", err))
		return
	}
	if _, ok := err.(*tracker.Error); ok || strings.Contains(err.Error(), "cannot reach") || strings.Contains(err.Error(), "unexpected answer") {
		writeError(w, http.StatusBadGateway, "tracker_error", err.Error())
		return
	}
	s.fail(w, r, err)
}

func (s *Server) handleCheckTracker(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	c, _, err := s.trackerClient(r, p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := c.Check(r.Context()); err != nil {
		s.trackerFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func applyIssue(d *model.Defect, t *model.DefectTracker, is *tracker.Issue) {
	d.ExternalSystem, d.ExternalKey, d.ExternalURL, d.ExternalStatus = t.Type, is.Key, is.URL, is.Status
}

// handlePushDefect creates the defect as an issue in the tracker.
func (s *Server) handlePushDefect(w http.ResponseWriter, r *http.Request, p *Principal) {
	d, err := s.Store.GetDefect(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if d.ExternalKey != "" {
		s.fail(w, r, store.Conflictf("the defect is already linked to %s", d.ExternalKey))
		return
	}
	c, t, err := s.trackerClient(r, p, d.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.pushDefect(r, p, c, t, d); err != nil {
		s.trackerFail(w, r, err)
		return
	}
	s.writeDefect(w, r, p, d.ID, http.StatusOK)
}

// pushDefect creates the tracker issue of a defect and links it.
func (s *Server) pushDefect(r *http.Request, p *Principal, c tracker.Client, t *model.DefectTracker, d *model.Defect) error {
	desc := d.Description
	var refs []string
	for _, l := range d.Links {
		if l.Key != "" {
			refs = append(refs, l.EntityType+" "+l.Key+" "+l.Title)
		}
	}
	if len(refs) > 0 {
		desc = strings.TrimSpace(desc + "\n\nQtestframe " + d.Key + ":\n- " + strings.Join(refs, "\n- "))
	}
	is, err := c.Create(r.Context(), tracker.NewIssue{Title: d.Key + " " + d.Title, Description: desc, Severity: d.Severity, Priority: d.Priority})
	if err != nil {
		return err
	}
	applyIssue(d, t, is)
	if err := s.Store.MarkDefectSynced(r.Context(), p.TenantID(), d, is.Status, ""); err != nil {
		return err
	}
	s.audit(r.Context(), p, "defect.push", "defect", d.ID, d.Key+" → "+is.Key)
	return nil
}

// syncDefect reads the issue's status and maps it onto the defect.
func (s *Server) syncDefect(r *http.Request, p *Principal, c tracker.Client, t *model.DefectTracker, d *model.Defect) error {
	is, err := c.Get(r.Context(), d.ExternalKey)
	if err != nil {
		return err
	}
	old := d.Status
	applyIssue(d, t, is)
	if err := s.Store.MarkDefectSynced(r.Context(), p.TenantID(), d, is.Status, is.Mapped); err != nil {
		return err
	}
	if old != d.Status {
		s.audit(r.Context(), p, "defect.sync", "defect", d.ID, d.Key+" "+old+" → "+d.Status+" ("+is.Key+" "+is.Status+")")
		s.notifyDefect(r, p, d, "defect.status")
	}
	return nil
}

func (s *Server) handleSyncDefect(w http.ResponseWriter, r *http.Request, p *Principal) {
	d, err := s.Store.GetDefect(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if d.ExternalKey == "" {
		s.fail(w, r, badRequest("the defect is not linked to an issue in the tracker"))
		return
	}
	c, t, err := s.trackerClient(r, p, d.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.syncDefect(r, p, c, t, d); err != nil {
		s.trackerFail(w, r, err)
		return
	}
	s.writeDefect(w, r, p, d.ID, http.StatusOK)
}

// handleSyncDefects refreshes every linked defect of the project.
func (s *Server) handleSyncDefects(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	c, t, err := s.trackerClient(r, p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.Store.ListDefects(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// an optional body {"ids": [...]} limits the sync to those defects
	if body, err := readBody(r); err == nil && len(strings.TrimSpace(string(body))) > 0 {
		var req struct {
			IDs []string `json:"ids"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			s.fail(w, r, badRequest("invalid JSON: %v", err))
			return
		}
		if req.IDs != nil {
			want := map[string]bool{}
			for _, id := range req.IDs {
				want[id] = true
			}
			list = slices.DeleteFunc(list, func(d model.Defect) bool { return !want[d.ID] })
		}
	}
	synced, changed := 0, 0
	var failed []map[string]string
	for i := range list {
		d := &list[i]
		if d.ExternalKey == "" || d.ExternalSystem != t.Type {
			continue
		}
		before := d.Status
		if err := s.syncDefect(r, p, c, t, d); err != nil {
			failed = append(failed, map[string]string{"key": d.Key, "error": err.Error()})
			continue
		}
		synced++
		if d.Status != before {
			changed++
		}
	}
	if failed == nil {
		failed = []map[string]string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"synced": synced, "changed": changed, "failed": failed})
}

// handleImportDefect creates a defect from an existing issue: {"external_key", "links"}.
func (s *Server) handleImportDefect(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		ExternalKey string             `json:"external_key"`
		Severity    string             `json:"severity"`
		Links       []model.DefectLink `json:"links"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if req.ExternalKey = strings.TrimSpace(req.ExternalKey); req.ExternalKey == "" {
		s.fail(w, r, badRequest("external_key is required"))
		return
	}
	c, t, err := s.trackerClient(r, p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	is, err := c.Get(r.Context(), req.ExternalKey)
	if err != nil {
		s.trackerFail(w, r, err)
		return
	}
	if existing, err := s.Store.ListDefects(r.Context(), p.TenantID(), pr.ID); err == nil {
		for _, x := range existing {
			if strings.EqualFold(x.ExternalKey, is.Key) {
				s.fail(w, r, store.Conflictf("%s is already defect %s", is.Key, x.Key))
				return
			}
		}
	}
	d := &model.Defect{ProjectID: pr.ID, Title: is.Title, Status: is.Mapped, Severity: req.Severity, CreatedBy: p.User.Email, Links: req.Links}
	applyIssue(d, t, is)
	if err := s.Store.CreateDefect(r.Context(), p.TenantID(), d); err != nil {
		s.fail(w, r, err)
		return
	}
	_ = s.Store.MarkDefectSynced(r.Context(), p.TenantID(), d, is.Status, "")
	s.audit(r.Context(), p, "defect.import", "defect", d.ID, d.Key+" ← "+is.Key)
	s.writeDefect(w, r, p, d.ID, http.StatusCreated)
}
