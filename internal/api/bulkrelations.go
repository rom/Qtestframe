package api

import (
	"errors"
	"net/http"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// Bulk operations on requirement relations: create many relations (for
// example make many requirements refine one parent) and delete many in one
// request. Relations that cannot be created are skipped with a reason.

type relationSkip struct {
	ID       string `json:"id,omitempty"`
	SourceID string `json:"source_id,omitempty"`
	TargetID string `json:"target_id,omitempty"`
	Type     string `json:"type,omitempty"`
	Reason   string `json:"reason"`
	Message  string `json:"message,omitempty"`
}

// handleBulkCreateRelations: POST /projects/{pid}/relations/bulk
// {"relations":[{source_id, target_id, type}]} → {created, skipped}.
func (s *Server) handleBulkCreateRelations(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Relations []struct {
			SourceID string `json:"source_id"`
			TargetID string `json:"target_id"`
			Type     string `json:"type"`
		} `json:"relations"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if len(req.Relations) == 0 {
		s.fail(w, r, badRequest("choose at least one relation"))
		return
	}
	if len(req.Relations) > maxBulkQuality {
		s.fail(w, r, badRequest("at most %d relations at a time", maxBulkQuality))
		return
	}
	ctx := r.Context()
	reqs, err := s.Store.ListRequirements(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	inProject := map[string]bool{}
	for _, x := range reqs {
		inProject[x.ID] = true
	}
	existing, err := s.Store.ListRelations(ctx, p.TenantID(), pr.ID, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	type triple struct{ src, dst, typ string }
	have := map[triple]bool{}
	for _, x := range existing {
		have[triple{x.SourceID, x.TargetID, x.Type}] = true
	}
	created, skipped := []model.Relation{}, []relationSkip{}
	for _, x := range req.Relations {
		skip := func(reason, msg string) {
			skipped = append(skipped, relationSkip{SourceID: x.SourceID, TargetID: x.TargetID, Type: x.Type, Reason: reason, Message: msg})
		}
		k := triple{x.SourceID, x.TargetID, x.Type}
		switch {
		case !inProject[x.SourceID] || !inProject[x.TargetID]:
			skip("not_found", "")
			continue
		case x.SourceID == x.TargetID:
			skip("self", "")
			continue
		case have[k]:
			skip("exists", "")
			continue
		}
		rel := &model.Relation{SourceID: x.SourceID, TargetID: x.TargetID, Type: x.Type, CreatedBy: p.User.Email}
		err := s.Store.Savepoint(ctx, func() error { return s.Store.CreateRelation(ctx, p.TenantID(), rel) })
		if errors.Is(err, store.ErrInvalid) {
			skip("invalid", err.Error()) // an unknown type or a refines cycle
			continue
		} else if errors.Is(err, store.ErrConflict) {
			skip("exists", "")
			continue
		} else if err != nil {
			s.fail(w, r, err)
			return
		}
		have[k] = true
		rel.Direction = "outgoing"
		created = append(created, *rel)
		s.audit(ctx, p, "relation.create", "relation", rel.ID, rel.SourceKey+" "+rel.Type+" "+rel.TargetKey)
	}
	s.syncEvidence(ctx, p, pr.ID, refinesEnds(created))
	writeJSON(w, http.StatusOK, map[string]any{"created": created, "skipped": skipped})
}

// handleBulkDeleteRelations: DELETE /projects/{pid}/relations/bulk
// {"ids":[...]} → {deleted: [Relation], skipped}. The deleted relations are
// returned so a client can restore them.
func (s *Server) handleBulkDeleteRelations(w http.ResponseWriter, r *http.Request, p *Principal) {
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
	existing, err := s.Store.ListRelations(ctx, p.TenantID(), pr.ID, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	byID := map[string]model.Relation{}
	for _, x := range existing {
		byID[x.ID] = x
	}
	deleted, skipped := []model.Relation{}, []relationSkip{}
	for _, id := range ids {
		rel, ok := byID[id]
		if !ok {
			skipped = append(skipped, relationSkip{ID: id, Reason: "not_found"})
			continue
		}
		if err := s.Store.DeleteRelation(ctx, p.TenantID(), id); err != nil {
			s.fail(w, r, err)
			return
		}
		rel.Direction = "outgoing"
		deleted = append(deleted, rel)
		s.audit(ctx, p, "relation.delete", "relation", id, rel.SourceKey+" "+rel.Type+" "+rel.TargetKey)
	}
	s.syncEvidence(ctx, p, pr.ID, refinesEnds(deleted))
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted, "skipped": skipped})
}

// refinesEnds returns both ends of the refines relations among rels: the
// verification of the parents rolls up their children.
func refinesEnds(rels []model.Relation) []string {
	out := []string{}
	for _, r := range rels {
		if r.Type == "refines" {
			out = append(out, r.SourceID, r.TargetID)
		}
	}
	return out
}
