package api

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/suggest"
)

// Autocomplete for free-text fields (internal/suggest ranks).
//
//	GET    /suggest?field=&q=&project_id=&limit=  suggestions for a field
//	POST   /suggest/history                        record saved values
//	GET    /suggest/history                        how many values are stored
//	DELETE /suggest/history                        clear the caller's history
//
// A suggestion comes from the caller's own history of the field (values
// they saved, in any project of the organisation they may still read with
// the field's read permission) and, for a few short fields, from the values
// already used in the project. A project field needs project_id and read
// access there (404 otherwise, as for any project). The personal
// preference Preferences.Autocomplete turns the sources off: "off" answers
// an empty list and records nothing, "project" records nothing.

type suggestField struct {
	perm rbac.Permission
	// global fields belong to no project (project_id must be empty)
	global bool
}

// suggestFields are the field keys of the web client (web/src/lib/suggest.ts).
var suggestFields = map[string]suggestField{
	"requirement.title":     {perm: rbac.RequirementRd},
	"requirement.source":    {perm: rbac.RequirementRd},
	"requirement.owner":     {perm: rbac.RequirementRd},
	"requirement.rationale": {perm: rbac.RequirementRd},
	"test.title":            {perm: rbac.TestRead},
	"test.preconditions":    {perm: rbac.TestRead},
	"test.expected":         {perm: rbac.TestRead},
	"test.step.action":      {perm: rbac.TestRead},
	"test.step.expected":    {perm: rbac.TestRead},
	"test.step.data":        {perm: rbac.TestRead},
	"execution.notes":       {perm: rbac.RunRead},
	"execution.step.actual": {perm: rbac.RunRead},
	"run.name":              {perm: rbac.RunRead},
	"defect.title":          {perm: rbac.ProjectRead},
	"defect.description":    {perm: rbac.ProjectRead},
	"risk.title":            {perm: rbac.RequirementRd},
	"risk.mitigation":       {perm: rbac.RequirementRd},
	"risk.owner":            {perm: rbac.RequirementRd},
	"comment":               {perm: rbac.ProjectRead},
	"review.comment":        {perm: rbac.ProjectRead},
	"release.name":          {perm: rbac.ProjectRead},
	"search":                {global: true},
}

// maxSuggestEntries bounds the values of one POST /suggest/history.
const maxSuggestEntries = 200

func (s *Server) routesSuggest(h func(string, handlerFunc, ...string)) {
	h("GET /api/v1/suggest", s.handleSuggest) // read access to project_id: see suggest.go
	h("GET /api/v1/suggest/history", s.handleSuggestHistoryCount)
	h("POST /api/v1/suggest/history", s.handleRecordSuggest)
	h("DELETE /api/v1/suggest/history", s.handleClearSuggest)
}

// suggestSources returns which sources the caller's preference allows.
func suggestSources(p *Principal) (history, project bool) {
	switch p.User.Preferences.Autocomplete {
	case "off":
		return false, false
	case "history":
		return true, false
	case "project":
		return false, true
	}
	return true, true
}

// checkSuggestProject checks a field's project: none for a global field;
// otherwise one the caller may read with the field's permission.
func (s *Server) checkSuggestProject(ctx context.Context, p *Principal, f suggestField, projectID string) error {
	if f.global {
		if projectID != "" {
			return badRequest("this field belongs to no project: project_id must be empty")
		}
		return nil
	}
	if projectID == "" {
		return badRequest("project_id is required for this field")
	}
	return s.canIn(ctx, p, projectID, f.perm)
}

func (s *Server) handleSuggest(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireOrg(w, p) {
		return
	}
	ctx := r.Context()
	q := r.URL.Query()
	key := q.Get("field")
	f, ok := suggestFields[key]
	if !ok {
		s.fail(w, r, badRequest("unknown field %q", key))
		return
	}
	pid := q.Get("project_id")
	if err := s.checkSuggestProject(ctx, p, f, pid); err != nil {
		s.fail(w, r, err)
		return
	}
	if !s.suggestLimiter.allow(p.User.ID) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many autocomplete requests, try again later")
		return
	}
	history, project := suggestSources(p)
	if src := q.Get("sources"); src != "" {
		want := strings.Split(src, ",")
		history = history && slices.Contains(want, suggest.SourceHistory)
		project = project && slices.Contains(want, suggest.SourceProject)
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	cands := []suggest.Candidate{}
	if history {
		rows, err := s.Store.SuggestHistory(ctx, p.TenantID(), p.User.ID, key)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if f.global {
			for _, h := range rows {
				cands = append(cands, h.Candidate)
			}
		} else if len(rows) > 0 {
			// only values saved in projects the caller may still read
			vis, err := s.visibleProjects(ctx, p)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			for _, h := range rows {
				if visibleIn(vis, p, h.ProjectID, f.perm) {
					cands = append(cands, h.Candidate)
				}
			}
		}
	}
	if project && store.HasProjectValues(key) {
		vals, err := s.Store.SuggestProjectValues(ctx, p.TenantID(), pid, key)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		cands = append(cands, vals...)
	}
	writeJSON(w, http.StatusOK, listOf(suggest.Rank(q.Get("q"), cands, limit, s.Store.Now())))
}

func (s *Server) handleRecordSuggest(w http.ResponseWriter, r *http.Request, p *Principal) {
	if !requireOrg(w, p) {
		return
	}
	var req struct {
		ProjectID string               `json:"project_id"`
		Entries   []store.SuggestEntry `json:"entries"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(req.Entries) > maxSuggestEntries {
		s.fail(w, r, badRequest("at most %d entries", maxSuggestEntries))
		return
	}
	ctx := r.Context()
	checked := map[string]bool{}
	keep := []store.SuggestEntry{}
	for _, e := range req.Entries {
		f, ok := suggestFields[e.Field]
		if !ok {
			s.fail(w, r, badRequest("unknown field %q", e.Field))
			return
		}
		if e.ProjectID == "" && !f.global {
			e.ProjectID = req.ProjectID
		}
		ck := e.ProjectID + "|" + string(f.perm)
		if !checked[ck] {
			if err := s.checkSuggestProject(ctx, p, f, e.ProjectID); err != nil {
				s.fail(w, r, err)
				return
			}
			checked[ck] = true
		}
		if _, ok := suggest.Normalize(e.Value); ok {
			keep = append(keep, e)
		}
	}
	if history, _ := suggestSources(p); !history {
		keep = nil // the preference stops the recording
	}
	if err := s.Store.RecordSuggestHistory(ctx, p.TenantID(), p.User.ID, keep); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"recorded": len(keep)})
}

func (s *Server) handleSuggestHistoryCount(w http.ResponseWriter, r *http.Request, p *Principal) {
	n, err := s.Store.CountSuggestHistory(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": n})
}

func (s *Server) handleClearSuggest(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	n, err := s.Store.ClearSuggestHistory(ctx, "", p.User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if p.Tenant != nil {
		s.audit(ctx, p, "profile.autocomplete_clear", "user", p.User.ID, strconv.FormatInt(n, 10)+" values")
	}
	writeJSON(w, http.StatusOK, map[string]int64{"deleted": n})
}
