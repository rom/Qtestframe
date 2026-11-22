package api

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/report"
	"github.com/rom/qtestframe/internal/store"
)

// Exploratory testing sessions (model.ExploratorySession). Permissions
// follow test runs: reading needs run.read; planning, changing and
// deleting sessions run.write; running a session (the timer), taking
// notes, raising defects from notes and the debrief run.execute. Only the
// session's tester runs it, takes its notes and debriefs it themselves;
// anyone else needs run.write as well (a test lead stopping a session, a
// reviewer debriefing it). A session without tester is assigned to whoever
// starts it.

var exploratorySchema = &query.Schema{
	Fields:  []string{"key", "title", "mission", "status", "tester", "outcome", "created_by", "id", "area", "requirement", "test", "notes", "bugs", "timebox"},
	Aliases: map[string]string{"charter": "mission", "assignee": "tester", "owner": "tester", "folder": "area", "result": "outcome"},
}

func (s *Server) routesExploratory(h routeFunc) {
	h("GET /api/v1/projects/{pid}/exploratory-sessions", s.handleListExploratory, "run.read")
	h("POST /api/v1/projects/{pid}/exploratory-sessions", s.handleCreateExploratory, "run.write")
	h("GET /api/v1/projects/{pid}/exploratory-metrics", s.handleExploratoryMetrics, "run.read")
	h("GET /api/v1/exploratory-sessions/{id}", s.handleGetExploratory, "run.read")
	h("PATCH /api/v1/exploratory-sessions/{id}", s.handleUpdateExploratory, "run.write")
	h("DELETE /api/v1/exploratory-sessions/{id}", s.handleDeleteExploratory, "run.write")
	h("POST /api/v1/exploratory-sessions/{id}/timer", s.handleExploratoryTimer, "run.execute")
	h("PUT /api/v1/exploratory-sessions/{id}/debrief", s.handleDebriefExploratory, "run.execute")
	h("DELETE /api/v1/exploratory-sessions/{id}/debrief", s.handleReopenExploratory, "run.write")
	h("GET /api/v1/exploratory-sessions/{id}/history", s.handleExploratoryHistory, "run.read")
	h("GET /api/v1/exploratory-sessions/{id}/report", s.handleExploratoryReport, "run.read", "export")
	h("GET /api/v1/exploratory-sessions/{id}/notes", s.handleListNotes, "run.read")
	h("POST /api/v1/exploratory-sessions/{id}/notes", s.handleAddNote, "run.execute")
	h("PATCH /api/v1/exploratory-sessions/{id}/notes/{note}", s.handleUpdateNote, "run.execute")
	h("DELETE /api/v1/exploratory-sessions/{id}/notes/{note}", s.handleDeleteNote, "run.execute")
	h("POST /api/v1/exploratory-sessions/{id}/notes/{note}/defect", s.handleNoteDefect, "run.execute")
}

func exploratoryRecord(x *model.ExploratorySession) query.MapRecord {
	rec := query.MapRecord{"key": {x.Key}, "title": {x.Title}, "mission": {x.Mission}, "status": {x.Status}, "tester": {x.Tester},
		"created_by": {x.CreatedBy}, "id": {x.ID}, "notes": {itoa(x.NoteCount)}, "bugs": {itoa(x.BugCount)}, "timebox": {itoa(x.TimeboxMinutes)}}
	if x.Debrief != nil {
		rec["outcome"] = []string{x.Debrief.Outcome}
	}
	for _, it := range x.Scope {
		switch it.Type {
		case "folder":
			rec["area"] = append(rec["area"], it.Title)
		case model.KindRequirement:
			rec["requirement"] = append(rec["requirement"], it.Key)
		case model.KindTest:
			rec["test"] = append(rec["test"], it.Key)
		}
	}
	return rec
}

func (s *Server) handleListExploratory(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	v := r.URL.Query()
	q, err := query.Parse(v.Get("q"), exploratorySchema)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f := store.ExploratoryFilter{Status: v.Get("status"), Tester: v.Get("tester"), Outcome: v.Get("outcome")}
	if f.Status != "" && !slices.Contains(model.ExploratoryStatuses, f.Status) {
		s.fail(w, r, badRequest("status must be one of %s", strings.Join(model.ExploratoryStatuses, ", ")))
		return
	}
	if v.Get("mine") == "true" {
		f.Tester = p.User.Email
	}
	for _, k := range []struct{ param, typ string }{{"requirement_id", model.KindRequirement}, {"test_id", model.KindTest}, {"folder_id", "folder"}} {
		if id := v.Get(k.param); id != "" {
			f.ScopeType, f.ScopeID = k.typ, id
		}
	}
	list, err := s.Store.ListExploratorySessions(r.Context(), p.TenantID(), pr.ID, f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := []model.ExploratorySession{}
	var recs []query.MapRecord
	for i := range list {
		if rec := exploratoryRecord(&list[i]); q.Match(rec) {
			out = append(out, list[i])
			recs = append(recs, rec)
		}
	}
	sortByQuery(q, out, recs)
	writeJSON(w, http.StatusOK, listOf(out))
}

// exploratoryBody is the editable part of a session in a request.
type exploratoryBody struct {
	Title          *string                       `json:"title"`
	Mission        *string                       `json:"mission"`
	Tester         *string                       `json:"tester"`
	TimeboxMinutes *int                          `json:"timebox_minutes"`
	Scope          *[]model.ExploratoryScopeItem `json:"scope"`
	// EnvironmentID (id or name, "" none) and Booking: the environment
	// explored and a booking of it.
	itemEnvReq
}

func (b *exploratoryBody) apply(x *model.ExploratorySession) {
	if b.Title != nil {
		x.Title = *b.Title
	}
	if b.Mission != nil {
		x.Mission = *b.Mission
	}
	if b.Tester != nil {
		x.Tester = *b.Tester
	}
	if b.TimeboxMinutes != nil {
		x.TimeboxMinutes = *b.TimeboxMinutes
	}
	if b.Scope != nil {
		x.Scope = *b.Scope
	}
}

// exploratoryRevision stores a history revision of a session.
func (s *Server) exploratoryRevision(ctx context.Context, p *Principal, x *model.ExploratorySession, action string) {
	if err := s.Store.AddRevision(ctx, p.TenantID(), model.KindExploratory, x.ID, x.Version, action, p.User.Email, store.SnapshotExploratory(x)); err != nil {
		s.Log.Warn("exploratory revision", "session", x.ID, "err", err)
	}
}

// checkSessionTester verifies that a tester is a member who may run sessions in
// the project.
func (s *Server) checkSessionTester(ctx context.Context, p *Principal, projectID, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || strings.EqualFold(email, p.User.Email) {
		return nil
	}
	u, err := s.Store.GetUserByEmail(ctx, email)
	if err != nil || !s.userCanSee(ctx, p.TenantID(), projectID, u.ID, rbac.RunExecute) {
		return badRequest("%s cannot run sessions in this project", email)
	}
	return nil
}

func (s *Server) handleCreateExploratory(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var b exploratoryBody
	if err := decode(r, &b); err != nil {
		s.fail(w, r, err)
		return
	}
	x := &model.ExploratorySession{ProjectID: pr.ID, CreatedBy: p.User.Email, Tester: p.User.Email}
	b.apply(x)
	ctx := r.Context()
	if err := s.checkSessionTester(ctx, p, pr.ID, x.Tester); err != nil {
		s.fail(w, r, err)
		return
	}
	err := s.Store.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.CreateExploratorySession(ctx, p.TenantID(), x); err != nil {
			return err
		}
		return s.applyItemEnvironment(r.WithContext(ctx), p, model.KindExploratory, x.ID, pr.ID, b.itemEnvReq)
	})
	if err != nil {
		if !s.writeBookingConflict(w, r, p, err) {
			s.fail(w, r, err)
		}
		return
	}
	s.audit(ctx, p, "exploratory.create", model.KindExploratory, x.ID, x.Key+" "+x.Title)
	out, err := s.Store.GetExploratorySession(ctx, p.TenantID(), x.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.exploratoryRevision(ctx, p, out, "create")
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleGetExploratory(w http.ResponseWriter, r *http.Request, p *Principal) {
	x, err := s.Store.GetExploratorySession(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, x)
}

func (s *Server) handleUpdateExploratory(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	x, err := s.Store.GetExploratorySession(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var b exploratoryBody
	if err := decode(r, &b); err != nil {
		s.fail(w, r, err)
		return
	}
	oldTester := x.Tester
	b.apply(x)
	if !strings.EqualFold(oldTester, strings.TrimSpace(x.Tester)) {
		if err := s.checkSessionTester(ctx, p, x.ProjectID, x.Tester); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if err := s.Store.UpdateExploratorySession(ctx, p.TenantID(), x); err != nil {
		s.fail(w, r, err)
		return
	}
	if b.EnvironmentID != nil || b.Booking != nil {
		if err := s.applyItemEnvironment(r, p, model.KindExploratory, x.ID, x.ProjectID, b.itemEnvReq); err != nil {
			if !s.writeBookingConflict(w, r, p, err) {
				s.fail(w, r, err)
			}
			return
		}
	}
	s.audit(ctx, p, "exploratory.update", model.KindExploratory, x.ID, x.Key+" "+x.Title)
	s.writeExploratory(w, r, p, x.ID, "update")
}

// writeExploratory answers with the session (after a change: action names
// the history revision; "" = none).
func (s *Server) writeExploratory(w http.ResponseWriter, r *http.Request, p *Principal, id, action string) {
	out, err := s.Store.GetExploratorySession(r.Context(), p.TenantID(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if action != "" {
		s.exploratoryRevision(r.Context(), p, out, action)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteExploratory(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	x, err := s.Store.GetExploratorySession(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteExploratorySession(ctx, p.TenantID(), x.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "exploratory.delete", model.KindExploratory, x.ID, x.Key+" "+x.Title)
	w.WriteHeader(http.StatusNoContent)
}

// mayRun reports whether the caller may run (time, take notes for,
// debrief) a session: its tester, or anyone with run.write. It answers 403
// otherwise.
func mayRun(w http.ResponseWriter, p *Principal, x *model.ExploratorySession) bool {
	if x.Tester == "" || strings.EqualFold(x.Tester, p.User.Email) || p.Can(rbac.RunWrite) {
		return true
	}
	writeError(w, http.StatusForbidden, "forbidden", "only the session's tester (or someone with run.write) can do this")
	return false
}

func (s *Server) handleExploratoryTimer(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	var req struct {
		Action string `json:"action"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	x, err := s.Store.GetExploratorySession(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !mayRun(w, p, x) {
		return
	}
	if req.Action == store.TimerStart && x.Tester == "" {
		// a session without tester is the caller's from its start
		x.Tester = p.User.Email
		if err := s.Store.UpdateExploratorySession(ctx, p.TenantID(), x); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	out, err := s.Store.ExploratoryTimer(ctx, p.TenantID(), x.ID, req.Action)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "exploratory."+req.Action, model.KindExploratory, x.ID, x.Key)
	if req.Action == store.TimerStop {
		s.exploratoryRevision(ctx, p, out, "stop")
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDebriefExploratory(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	var d model.ExploratoryDebrief
	if err := decode(r, &d); err != nil {
		s.fail(w, r, err)
		return
	}
	x, err := s.Store.GetExploratorySession(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !mayRun(w, p, x) {
		return
	}
	d.By, d.Name = p.User.Email, signerName(p)
	out, err := s.Store.DebriefExploratorySession(ctx, p.TenantID(), x.ID, &d)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	details := x.Key + " " + d.Outcome
	if out.Debrief != nil && out.Debrief.Reviewed {
		details += " (reviewer)"
	}
	s.audit(ctx, p, "exploratory.debrief", model.KindExploratory, x.ID, details)
	s.exploratoryRevision(ctx, p, out, "debrief")
	s.afterExploratoryEvidence(r, p, out)
	writeJSON(w, http.StatusOK, out)
}

// afterExploratoryEvidence lets the status of the scoped requirements
// follow their verification when debriefed sessions count as evidence.
func (s *Server) afterExploratoryEvidence(r *http.Request, p *Principal, x *model.ExploratorySession) {
	ids := x.RequirementIDs()
	if len(ids) == 0 {
		return
	}
	st, err := s.Store.VerificationSettings(r.Context(), p.TenantID(), x.ProjectID)
	if err != nil || !st.ExploratoryEvidence {
		return
	}
	s.syncEvidence(r.Context(), p, x.ProjectID, ids)
}

func (s *Server) handleReopenExploratory(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	out, err := s.Store.ReopenExploratorySession(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "exploratory.reopen", model.KindExploratory, out.ID, out.Key)
	s.exploratoryRevision(ctx, p, out, "reopen")
	s.afterExploratoryEvidence(r, p, out)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleExploratoryHistory(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	x, err := s.Store.GetExploratorySession(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	revs, err := s.Store.ListRevisions(ctx, p.TenantID(), model.KindExploratory, x.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(revs))
}

func (s *Server) handleExploratoryMetrics(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	m, err := s.Store.ExploratoryMetrics(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// handleExploratoryReport renders the session report through the report
// generation: ?format=md|pdf|docx (default md), lang, archive=true saves
// it in the project's documents instead.
func (s *Server) handleExploratoryReport(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	x, err := s.Store.GetExploratorySession(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pr, err := s.Store.GetProject(ctx, p.TenantID(), x.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v := r.URL.Query()
	format := v.Get("format")
	if format == "" {
		format = "md"
	}
	s.generate(w, r, p, pr, reportJob{template: report.ExploratorySession, params: map[string]string{"session_id": x.ID}, format: format,
		archive: v.Get("archive") == "true", stem: strings.ToLower(x.Key), lang: reportLang(r, p, v.Get("lang"))})
}

// ---------------------------------------------------------------- notes

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	x, err := s.Store.GetExploratorySession(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.Store.ListExploratoryNotes(ctx, p.TenantID(), x.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

type noteBody struct {
	Type    *string   `json:"type"`
	Body    *string   `json:"body"`
	FileIDs *[]string `json:"file_ids"`
}

func (s *Server) handleAddNote(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	var b noteBody
	if err := decode(r, &b); err != nil {
		s.fail(w, r, err)
		return
	}
	x, err := s.Store.GetExploratorySession(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !mayRun(w, p, x) {
		return
	}
	n := &model.ExploratoryNote{SessionID: x.ID, CreatedBy: p.User.Email}
	if b.Type != nil {
		n.Type = *b.Type
	}
	if b.Body != nil {
		n.Body = *b.Body
	}
	if b.FileIDs != nil {
		n.FileIDs = *b.FileIDs
	}
	if err := s.Store.AddExploratoryNote(ctx, p.TenantID(), n); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "exploratory.note.add", model.KindExploratory, x.ID, x.Key+" "+n.Type)
	out, err := s.Store.GetExploratoryNote(ctx, p.TenantID(), x.ID, n.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// note loads a note of the request's session that the caller may change:
// its author, or anyone with run.write.
func (s *Server) note(w http.ResponseWriter, r *http.Request, p *Principal) (*model.ExploratorySession, *model.ExploratoryNote, bool) {
	ctx := r.Context()
	x, err := s.Store.GetExploratorySession(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return nil, nil, false
	}
	n, err := s.Store.GetExploratoryNote(ctx, p.TenantID(), x.ID, r.PathValue("note"))
	if err != nil {
		s.fail(w, r, err)
		return nil, nil, false
	}
	if !strings.EqualFold(n.CreatedBy, p.User.Email) && !p.Can(rbac.RunWrite) {
		writeError(w, http.StatusForbidden, "forbidden", "only the note's author (or someone with run.write) can change it")
		return nil, nil, false
	}
	return x, n, true
}

func (s *Server) handleUpdateNote(w http.ResponseWriter, r *http.Request, p *Principal) {
	var b noteBody
	if err := decode(r, &b); err != nil {
		s.fail(w, r, err)
		return
	}
	x, n, ok := s.note(w, r, p)
	if !ok {
		return
	}
	if b.Type != nil {
		n.Type = *b.Type
	}
	if b.Body != nil {
		n.Body = *b.Body
	}
	if b.FileIDs != nil {
		n.FileIDs = *b.FileIDs
	}
	ctx := r.Context()
	if err := s.Store.UpdateExploratoryNote(ctx, p.TenantID(), n); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "exploratory.note.update", model.KindExploratory, x.ID, x.Key+" "+n.Type)
	out, err := s.Store.GetExploratoryNote(ctx, p.TenantID(), x.ID, n.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request, p *Principal) {
	x, n, ok := s.note(w, r, p)
	if !ok {
		return
	}
	if err := s.Store.DeleteExploratoryNote(r.Context(), p.TenantID(), x.ID, n.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "exploratory.note.delete", model.KindExploratory, x.ID, x.Key+" "+n.Type)
	w.WriteHeader(http.StatusNoContent)
}

// NoteDefect prefills a defect from a note: the title is the note's first
// line, the description the note with the session's key and charter; the
// defect is linked to the session and to the requirements in its scope.
func NoteDefect(x *model.ExploratorySession, n *model.ExploratoryNote) *model.Defect {
	title, _, _ := strings.Cut(strings.TrimSpace(n.Body), "\n")
	title = strings.TrimSpace(title)
	if r := []rune(title); len(r) > 200 {
		title = string(r[:200]) + "…"
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(n.Body))
	b.WriteString("\n\n---\n")
	b.WriteString("Found in exploratory session " + x.Key + " " + x.Title)
	b.WriteString(" at " + report.Clock(n.ElapsedSeconds) + " of session time.")
	if m := strings.TrimSpace(x.Mission); m != "" {
		b.WriteString("\nCharter: " + m)
	}
	d := &model.Defect{ProjectID: x.ProjectID, Title: title, Description: b.String(), FoundIn: "test",
		Links: []model.DefectLink{{EntityType: model.KindExploratory, EntityID: x.ID}}}
	for _, id := range x.RequirementIDs() {
		d.Links = append(d.Links, model.DefectLink{EntityType: model.KindRequirement, EntityID: id})
	}
	return d
}

// handleNoteDefect raises a defect from a note. The body may override the
// prefilled fields (title, description, severity, priority, assignee); the
// note is marked with the defect, and a note of another type becomes a bug.
func (s *Server) handleNoteDefect(w http.ResponseWriter, r *http.Request, p *Principal) {
	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Severity    string  `json:"severity"`
		Priority    string  `json:"priority"`
		Assignee    string  `json:"assignee"`
	}
	// the body is optional: without one the prefilled defect is created
	if r.ContentLength != 0 {
		if err := decode(r, &req); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	x, n, ok := s.note(w, r, p)
	if !ok {
		return
	}
	if n.DefectID != nil {
		s.fail(w, r, store.Conflictf("defect %s was already raised from this note", n.DefectKey))
		return
	}
	if x.Status == "debriefed" {
		s.fail(w, r, store.Conflictf("reopen the debrief to raise defects from its notes"))
		return
	}
	d := NoteDefect(x, n)
	if req.Title != nil {
		d.Title = *req.Title
	}
	if req.Description != nil {
		d.Description = *req.Description
	}
	d.Severity, d.Priority, d.Assignee, d.CreatedBy = req.Severity, req.Priority, req.Assignee, p.User.Email
	ctx := r.Context()
	err := s.Store.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.CreateDefect(ctx, p.TenantID(), d); err != nil {
			return err
		}
		return s.Store.SetNoteDefect(ctx, p.TenantID(), n.ID, d.ID)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if n.Type != "bug" {
		n.Type = "bug"
		if err := s.Store.UpdateExploratoryNote(ctx, p.TenantID(), n); err != nil {
			s.Log.Warn("exploratory note type", "note", n.ID, "err", err)
		}
	}
	s.audit(ctx, p, "defect.create", "defect", d.ID, d.Key+" "+d.Title+" (from "+x.Key+")")
	s.notifyDefect(r, p, d, "defect.created")
	s.writeDefect(w, r, p, d.ID, http.StatusCreated)
}

// exploratoryReportData loads the exploratory reports' data: the session
// of session_id (default the newest debriefed session of the project, or
// the newest session) with its
// notes, or the sessions and metrics of the project.
func (s *Server) exploratoryReportData(r *http.Request, p *Principal, pr *model.Project, id string, params map[string]string, d *report.Data) error {
	ctx := r.Context()
	switch id {
	case report.ExploratorySession:
		d.Exploratory = &report.ExploratoryData{}
		sid := strings.TrimSpace(params["session_id"])
		if sid == "" {
			list, err := s.Store.ListExploratorySessions(ctx, p.TenantID(), pr.ID, store.ExploratoryFilter{})
			if err != nil || len(list) == 0 {
				return err
			}
			sid = list[0].ID
			for _, x := range list {
				if x.Debrief != nil {
					sid = x.ID
					break
				}
			}
		}
		x, err := s.Store.GetExploratorySession(ctx, p.TenantID(), sid)
		if err != nil || x.ProjectID != pr.ID {
			return badRequest("exploratory session %q does not belong to project %s", sid, pr.Key)
		}
		d.Exploratory.Session = x
		d.Exploratory.Notes, err = s.Store.ListExploratoryNotes(ctx, p.TenantID(), x.ID)
		return err
	case report.ExploratorySessions:
		list, err := s.Store.ListExploratorySessions(ctx, p.TenantID(), pr.ID, store.ExploratoryFilter{})
		if err != nil {
			return err
		}
		m, err := s.Store.ExploratoryMetrics(ctx, p.TenantID(), pr.ID)
		if err != nil {
			return err
		}
		d.Exploratory = &report.ExploratoryData{Sessions: list, Metrics: m}
	}
	return nil
}
