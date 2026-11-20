package api

import (
	"net/http"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
)

// trashHeader carries the trash entry of a single deletion (DELETE
// /requirements/{id}, /tests/{id}), which answers 204 without a body.
const trashHeader = "X-Trash-Id"

// handleListTrash lists the deletions of a project that can be undone;
// entries of a kind the caller cannot read are left out.
func (s *Server) handleListTrash(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	all, err := s.Store.ListTrash(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := all[:0]
	for _, e := range all {
		read := rbac.RequirementRd
		if e.Kind == model.KindTest {
			read = rbac.TestRead
		}
		if p.Can(read) {
			out = append(out, e)
		}
	}
	writeJSON(w, http.StatusOK, listOf(out))
}

// handleRestoreTrash undoes a deletion. It needs the write permission of
// the kind, and the review permission when a restored item carries a review
// decision (an approved requirement, a ready test).
func (s *Server) handleRestoreTrash(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	e, err := s.Store.GetTrash(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	statuses, err := s.Store.TrashStatuses(ctx, p.TenantID(), e.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, st := range statuses {
		if !authorizeChange(w, p, e.Kind, false, "", st) {
			return
		}
	}
	if len(statuses) == 0 && !authorizeChange(w, p, e.Kind, false, "", "") {
		return
	}
	restored, err := s.Store.RestoreTrash(ctx, p.TenantID(), e.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, restored.Kind+".restore_deleted", restored.Kind, strings.Join(restored.ItemIDs, ","), strings.Join(restored.Keys, ", "))
	writeJSON(w, http.StatusOK, restored)
}
