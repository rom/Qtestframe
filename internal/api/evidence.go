package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// Suspect links and "status follows evidence" (see store/evidence.go).

// snapshotOf returns the snapshot of a requirement or test as it is now.
func (s *Server) snapshotOf(ctx context.Context, p *Principal, kind, id string) json.RawMessage {
	if kind == model.KindTest {
		if t, err := s.Store.GetTest(ctx, p.TenantID(), id); err == nil {
			return store.SnapshotTest(t)
		}
		return nil
	}
	if r, err := s.Store.GetRequirement(ctx, p.TenantID(), id); err == nil {
		return store.SnapshotRequirement(r)
	}
	return nil
}

// afterContentChange marks the links of an item suspect when its content
// changed (compared with the snapshot before), then lets the statuses of
// the affected requirements follow their evidence.
func (s *Server) afterContentChange(ctx context.Context, p *Principal, kind, id, key, projectID string, before json.RawMessage) {
	if before == nil {
		return
	}
	after := s.snapshotOf(ctx, p, kind, id)
	changed := store.ContentChanged(kind, store.ChangedFields(before, after))
	if len(changed) == 0 {
		return
	}
	reason := kind + " " + key + " changed (" + strings.Join(changed, ", ") + ") by " + p.User.Email
	// results recorded before now go stale, documents generated before now are outdated
	if err := s.Store.MarkContentChanged(ctx, p.TenantID(), kind, id); err != nil {
		s.Log.Warn("mark content changed", "err", err)
	}
	reqs, err := s.Store.MarkLinksSuspect(ctx, p.TenantID(), kind, id, reason)
	if err != nil {
		s.Log.Warn("mark links suspect", "err", err)
		return
	}
	if len(reqs) > 0 {
		s.audit(ctx, p, "link.suspect", kind, id, reason)
	}
	if kind == model.KindRequirement {
		// along the requirement relations, to tracker issues (correlations.go)
		reqs = append(reqs, s.applyChangeImpact(ctx, p, id, key, reason)...)
		// stale results of its tests may change its status even without links
		reqs = append(reqs, id)
	} else {
		reqs = append(reqs, s.requirementsOfTests(ctx, p, projectID, []string{id})...)
	}
	s.syncEvidence(ctx, p, projectID, reqs)
}

// syncEvidence applies "status follows evidence" to requirements and audits
// the changes.
// reqIDs nil means all requirements of the project.
func (s *Server) syncEvidence(ctx context.Context, p *Principal, projectID string, reqIDs []string) []store.StatusChange {
	if reqIDs != nil && len(reqIDs) == 0 {
		return nil
	}
	changes, err := s.Store.SyncEvidenceStatus(ctx, p.TenantID(), projectID, reqIDs, p.User.Email)
	if err != nil {
		s.Log.Warn("sync evidence status", "err", err)
	}
	for _, c := range changes {
		s.audit(ctx, p, "requirement.status.evidence", "requirement", c.ID, c.Key+" "+c.From+" → "+c.To+" ("+c.Reason+")")
	}
	return changes
}

// requirementsOfTests returns the requirements linked to tests.
func (s *Server) requirementsOfTests(ctx context.Context, p *Principal, projectID string, testIDs []string) []string {
	links, err := s.Store.ListLinks(ctx, p.TenantID(), projectID, "", "")
	if err != nil {
		return nil
	}
	want := map[string]bool{}
	for _, t := range testIDs {
		want[t] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, l := range links {
		if want[l.TestCaseID] && !seen[l.RequirementID] {
			seen[l.RequirementID] = true
			out = append(out, l.RequirementID)
		}
	}
	return out
}

// handleConfirmLink: POST /links/{id}/confirm — the test still verifies the
// requirement; clears the suspect mark.
func (s *Server) handleConfirmLink(w http.ResponseWriter, r *http.Request, p *Principal) {
	s.confirmLinks(w, r, p, []string{r.PathValue("id")}, "", "")
}

// handleConfirmItemLinks: POST /requirements/{id}/links/confirm and
// /tests/{id}/links/confirm — confirms all suspect links of an item.
func (s *Server) handleConfirmItemLinks(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		if err := s.itemExists(r.Context(), p, kind, r.PathValue("id")); err != nil {
			s.fail(w, r, err)
			return
		}
		s.confirmLinks(w, r, p, nil, kind, r.PathValue("id"))
	}
}

func (s *Server) confirmLinks(w http.ResponseWriter, r *http.Request, p *Principal, ids []string, kind, itemID string) {
	ctx := r.Context()
	if len(ids) == 1 {
		if _, err := s.Store.GetLink(ctx, p.TenantID(), ids[0]); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	links, err := s.Store.ConfirmLinks(ctx, p.TenantID(), ids, kind, itemID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	byProject := map[string][]string{}
	for _, l := range links {
		s.audit(ctx, p, "link.confirm", "link", l.ID, l.RequirementID+" ↔ "+l.TestCaseID)
		byProject[l.ProjectID] = append(byProject[l.ProjectID], l.RequirementID)
	}
	var changes []store.StatusChange
	for pid, reqs := range byProject {
		changes = append(changes, s.syncEvidence(ctx, p, pid, reqs)...)
	}
	if changes == nil {
		changes = []store.StatusChange{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"confirmed": len(links), "status_changes": changes})
}
