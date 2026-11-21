package api

import (
	"net/http"
	"strings"

	"github.com/rom/qtestframe/internal/mention"
	"github.com/rom/qtestframe/internal/tagsuggest"
)

// handlePeople lists who can be @mentioned in a project: the members of
// the organisation who may see it (project sharing) and the other names
// used as owners and assignees.
func (s *Server) handlePeople(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	access, ms, err := s.membersIn(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	users := make([]mention.User, 0, len(ms))
	for _, m := range ms {
		if access[m.UserID] == nil {
			continue
		}
		users = append(users, mention.User{ID: m.UserID, Email: m.Email, Name: m.Name, Avatar: m.Avatar})
	}
	names, err := s.Store.ProjectPeopleNames(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(mention.Directory(users, names)))
}

// handleSuggestTags proposes tags for a requirement or test from its title
// and description, preferring tags the project already uses.
func (s *Server) handleSuggestTags(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		Limit       int      `json:"limit"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(req.Title)+len(req.Description) > 100_000 {
		s.fail(w, r, badRequest("title and description are too long"))
		return
	}
	if req.Limit < 0 || req.Limit > 50 {
		s.fail(w, r, badRequest("limit must be between 0 and 50"))
		return
	}
	existing, err := s.Store.ProjectTags(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if strings.TrimSpace(req.Title+req.Description) == "" {
		writeJSON(w, http.StatusOK, listOf([]tagsuggest.Suggestion{}))
		return
	}
	writeJSON(w, http.StatusOK, listOf(tagsuggest.Suggest(req.Title, req.Description, existing, req.Tags, req.Limit)))
}
