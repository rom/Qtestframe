package api

import (
	"errors"
	"net/http"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// Bulk operations on traceability links: link many requirement/test pairs,
// unlink many links and confirm many suspect links in one request. Pairs
// that already exist or do not belong to the project are skipped with a
// reason; the status that follows evidence is synced once at the end.

type linkPair struct {
	RequirementID string `json:"requirement_id"`
	TestCaseID    string `json:"test_case_id"`
}

type linkSkip struct {
	ID            string `json:"id,omitempty"`
	RequirementID string `json:"requirement_id,omitempty"`
	TestCaseID    string `json:"test_case_id,omitempty"`
	Reason        string `json:"reason"`
}

// projectLinks returns the project's links by id and by pair.
func (s *Server) projectLinks(r *http.Request, p *Principal, projectID string) (map[string]model.TraceLink, map[linkPair]bool, error) {
	links, err := s.Store.ListLinks(r.Context(), p.TenantID(), projectID, "", "")
	if err != nil {
		return nil, nil, err
	}
	byID, pairs := map[string]model.TraceLink{}, map[linkPair]bool{}
	for _, l := range links {
		byID[l.ID] = l
		pairs[linkPair{l.RequirementID, l.TestCaseID}] = true
	}
	return byID, pairs, nil
}

// handleBulkCreateLinks: POST /projects/{pid}/links/bulk
// {"links":[{requirement_id, test_case_id, criterion_id?}]} → {created, skipped}.
func (s *Server) handleBulkCreateLinks(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Links []struct {
			linkPair
			CriterionID *string `json:"criterion_id"`
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
	reqs, err := s.Store.ListRequirements(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	tests, err := s.Store.ListTests(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reqIn, testIn := map[string]bool{}, map[string]bool{}
	for _, x := range reqs {
		reqIn[x.ID] = true
	}
	for _, x := range tests {
		testIn[x.ID] = true
	}
	_, existing, err := s.projectLinks(r, p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	created, skipped := []model.TraceLink{}, []linkSkip{}
	var touched []string
	for _, in := range req.Links {
		x := in.linkPair
		skip := func(reason string) {
			skipped = append(skipped, linkSkip{RequirementID: x.RequirementID, TestCaseID: x.TestCaseID, Reason: reason})
		}
		switch {
		case !reqIn[x.RequirementID] || !testIn[x.TestCaseID]:
			skip("not_found")
			continue
		case existing[x]:
			skip("exists")
			continue
		}
		l := model.TraceLink{RequirementID: x.RequirementID, TestCaseID: x.TestCaseID, CriterionID: in.CriterionID}
		if err := s.Store.CreateLink(ctx, p.TenantID(), &l); errors.Is(err, store.ErrInvalid) {
			skip("invalid") // e.g. a criterion of another requirement
			continue
		} else if err != nil {
			s.fail(w, r, err)
			return
		}
		existing[x] = true
		created = append(created, l)
		touched = append(touched, l.RequirementID)
		s.audit(ctx, p, "link.create", "link", l.ID, l.RequirementKey+" -> "+l.TestCaseKey)
	}
	changes := s.syncEvidence(ctx, p, pr.ID, uniqStrings(touched))
	if changes == nil {
		changes = []store.StatusChange{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": created, "skipped": skipped, "status_changes": changes})
}

// handleBulkDeleteLinks: DELETE /projects/{pid}/links/bulk {"ids":[...]}
// → {deleted: [TraceLink], skipped}. The deleted links are returned so a
// client can restore them.
func (s *Server) handleBulkDeleteLinks(w http.ResponseWriter, r *http.Request, p *Principal) {
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
	byID, _, err := s.projectLinks(r, p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	deleted, skipped := []model.TraceLink{}, []linkSkip{}
	var touched []string
	for _, id := range ids {
		l, ok := byID[id]
		if !ok {
			skipped = append(skipped, linkSkip{ID: id, Reason: "not_found"})
			continue
		}
		if err := s.Store.DeleteLink(ctx, p.TenantID(), id); err != nil {
			s.fail(w, r, err)
			return
		}
		deleted = append(deleted, l)
		touched = append(touched, l.RequirementID)
		s.audit(ctx, p, "link.delete", "link", id, l.RequirementKey+" -> "+l.TestCaseKey)
	}
	changes := s.syncEvidence(ctx, p, pr.ID, uniqStrings(touched))
	if changes == nil {
		changes = []store.StatusChange{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted, "skipped": skipped, "status_changes": changes})
}

// handleBulkConfirmLinks: POST /projects/{pid}/links/confirm {"ids":[...]}
// → {confirmed, status_changes}. Links that are not suspect are ignored.
func (s *Server) handleBulkConfirmLinks(w http.ResponseWriter, r *http.Request, p *Principal) {
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
	byID, _, err := s.projectLinks(r, p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var mine []string
	for _, id := range ids {
		if _, ok := byID[id]; ok {
			mine = append(mine, id)
		}
	}
	if len(mine) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"confirmed": 0, "status_changes": []store.StatusChange{}})
		return
	}
	s.confirmLinks(w, r, p, mine, "", "")
}
