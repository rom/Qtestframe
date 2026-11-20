package api

import (
	"errors"
	"net/http"
	"slices"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// Bulk operations on item links: list the links of a project, add links
// from many items at once and remove many links. Each link needs the write
// permission of the item that carries it and the read permission of the
// item it points to; links that fail are skipped with a reason.

type itemLinkSkip struct {
	ID         string  `json:"id,omitempty"`
	SourceType string  `json:"source_type,omitempty"`
	SourceID   string  `json:"source_id,omitempty"`
	TargetType string  `json:"target_type,omitempty"`
	TargetID   *string `json:"target_id,omitempty"`
	URL        string  `json:"url,omitempty"`
	Reason     string  `json:"reason"`
	Message    string  `json:"message,omitempty"`
}

// readableLink: the caller may see what the link points to.
func readableLink(p *Principal, l *model.ItemLink) bool {
	return p.Can(linkReadPerm(l.SourceType)) && (l.TargetType == "url" || p.Can(linkReadPerm(l.TargetType)))
}

// handleListProjectItemLinks: GET /projects/{pid}/item-links?source_type=
// → the links carried by the project's items that the caller may read.
func (s *Server) handleListProjectItemLinks(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	st := r.URL.Query().Get("source_type")
	if st != "" && !slices.Contains(model.ItemLinkSourceTypes, st) {
		s.fail(w, r, badRequest("source_type must be requirement, test or run"))
		return
	}
	links, err := s.Store.ListProjectItemLinks(r.Context(), p.TenantID(), pr.ID, st)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]model.ItemLink, 0, len(links))
	for i := range links {
		if readableLink(p, &links[i]) {
			out = append(out, links[i])
		}
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

// handleBulkCreateItemLinks: POST /projects/{pid}/item-links/bulk
// {"links":[{source_type, source_id, target_type, target_id?, url?, label?}]}
// → {created, skipped}.
func (s *Server) handleBulkCreateItemLinks(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Links []struct {
			SourceType string  `json:"source_type"`
			SourceID   string  `json:"source_id"`
			TargetType string  `json:"target_type"`
			TargetID   *string `json:"target_id"`
			URL        string  `json:"url"`
			Label      string  `json:"label"`
		} `json:"links"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(req.Links) == 0 {
		s.fail(w, r, badRequest("choose at least one link"))
		return
	}
	if len(req.Links) > maxBulkQuality {
		s.fail(w, r, badRequest("at most %d links at a time", maxBulkQuality))
		return
	}
	ctx := r.Context()
	// the project of every source, to skip items of other projects
	byType := map[string][]string{}
	for _, x := range req.Links {
		if slices.Contains(model.ItemLinkSourceTypes, x.SourceType) {
			byType[x.SourceType] = append(byType[x.SourceType], x.SourceID)
		}
	}
	projectOf := map[string]string{}
	for typ, ids := range byType {
		m, err := s.Store.LinkSourceProjects(ctx, p.TenantID(), typ, uniqStrings(ids))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for id, pid := range m {
			projectOf[typ+":"+id] = pid
		}
	}
	created, skipped := []model.ItemLink{}, []itemLinkSkip{}
	for _, x := range req.Links {
		skip := func(reason, msg string) {
			skipped = append(skipped, itemLinkSkip{SourceType: x.SourceType, SourceID: x.SourceID, TargetType: x.TargetType, TargetID: x.TargetID, URL: x.URL, Reason: reason, Message: msg})
		}
		if !slices.Contains(model.ItemLinkSourceTypes, x.SourceType) {
			skip("invalid", "source_type must be requirement, test or run")
			continue
		}
		if projectOf[x.SourceType+":"+x.SourceID] != pr.ID {
			skip("not_found", "")
			continue
		}
		if !p.Can(linkWritePerm(x.SourceType)) || (x.TargetType != "url" && slices.Contains(model.ItemLinkTargetTypes, x.TargetType) && !p.Can(linkReadPerm(x.TargetType))) {
			skip("not_allowed", "")
			continue
		}
		l := &model.ItemLink{SourceType: x.SourceType, SourceID: x.SourceID, TargetType: x.TargetType, TargetID: x.TargetID, URL: x.URL, Label: x.Label, CreatedBy: p.User.Email}
		err := s.Store.CreateItemLink(ctx, p.TenantID(), l)
		switch {
		case errors.Is(err, store.ErrNotFound):
			skip("not_found", "")
			continue
		case errors.Is(err, store.ErrConflict):
			skip("exists", "")
			continue
		case errors.Is(err, store.ErrInvalid):
			skip("invalid", err.Error())
			continue
		case err != nil:
			s.fail(w, r, err)
			return
		}
		target := l.URL
		if l.TargetID != nil {
			target = l.TargetType + " " + l.TargetKey
		}
		s.audit(ctx, p, l.SourceType+".link.add", l.SourceType, l.SourceID, l.SourceKey+" → "+target)
		l.Direction = "outgoing"
		created = append(created, *l)
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": created, "skipped": skipped})
}

// handleBulkDeleteItemLinks: DELETE /projects/{pid}/item-links/bulk
// {"ids":[...]} → {deleted: [ItemLink], skipped}. The deleted links are
// returned so a client can restore them.
func (s *Server) handleBulkDeleteItemLinks(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	ids, err := bulkIDs(req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	all, err := s.Store.ListProjectItemLinks(ctx, p.TenantID(), pr.ID, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	byID := map[string]model.ItemLink{}
	for _, l := range all {
		byID[l.ID] = l
	}
	deleted, skipped := []model.ItemLink{}, []itemLinkSkip{}
	for _, id := range ids {
		l, ok := byID[id]
		switch {
		case !ok || !readableLink(p, &l):
			skipped = append(skipped, itemLinkSkip{ID: id, Reason: "not_found"})
			continue
		case !p.Can(linkWritePerm(l.SourceType)):
			skipped = append(skipped, itemLinkSkip{ID: id, Reason: "not_allowed"})
			continue
		}
		if err := s.Store.DeleteItemLink(ctx, p.TenantID(), id); err != nil {
			s.fail(w, r, err)
			return
		}
		target := l.URL
		if l.TargetID != nil {
			target = l.TargetType + " " + l.TargetKey
		}
		s.audit(ctx, p, l.SourceType+".link.remove", l.SourceType, l.SourceID, l.SourceKey+" → "+target)
		deleted = append(deleted, l)
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted, "skipped": skipped})
}
