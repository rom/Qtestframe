package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// Bulk operations on defects and risks. Every item is checked before any
// is saved, so a bad value changes nothing.

const maxBulkQuality = 1000

func bulkIDs(ids []string) ([]string, error) {
	ids = uniqStrings(ids)
	if len(ids) == 0 {
		return nil, badRequest("choose at least one item")
	}
	if len(ids) > maxBulkQuality {
		return nil, badRequest("at most %d items at a time", maxBulkQuality)
	}
	return ids, nil
}

// ---------------------------------------------------------------- defects

func (s *Server) bulkDefectsOf(r *http.Request, p *Principal, projectID string, ids []string) ([]*model.Defect, error) {
	ids, err := bulkIDs(ids)
	if err != nil {
		return nil, err
	}
	var out []*model.Defect
	for _, id := range ids {
		d, err := s.Store.GetDefect(r.Context(), p.TenantID(), id)
		if err != nil {
			return nil, err
		}
		if d.ProjectID != projectID {
			return nil, fmt.Errorf("defect %s: %w", id, store.ErrNotFound)
		}
		out = append(out, d)
	}
	return out, nil
}

// handleBulkDefects: POST /projects/{pid}/defects/bulk
// {"ids": [...], "set": {"status", "severity", "priority", "assignee"}}.
func (s *Server) handleBulkDefects(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		IDs []string `json:"ids"`
		Set struct {
			Status, Severity, Priority, Assignee *string
		} `json:"set"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	set := req.Set
	if set.Status == nil && set.Severity == nil && set.Priority == nil && set.Assignee == nil {
		s.fail(w, r, badRequest("nothing to change: set status, severity, priority or assignee"))
		return
	}
	list, err := s.bulkDefectsOf(r, p, pr.ID, req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	type change struct {
		d                      *model.Defect
		oldStatus, oldAssignee string
	}
	var changes []change
	for _, d := range list {
		c := change{d, d.Status, d.Assignee}
		for _, f := range []struct{ src, dst *string }{{set.Status, &d.Status}, {set.Severity, &d.Severity}, {set.Priority, &d.Priority}, {set.Assignee, &d.Assignee}} {
			if f.src != nil {
				*f.dst = *f.src
			}
		}
		if err := store.ValidateDefect(d); err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", d.Key, err))
			return
		}
		changes = append(changes, c)
	}
	ctx := r.Context()
	for _, c := range changes {
		d := c.d
		if err := s.Store.UpdateDefect(ctx, p.TenantID(), d); err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", d.Key, err))
			return
		}
		detail := d.Key + " (bulk)"
		if c.oldStatus != d.Status {
			detail = d.Key + " " + c.oldStatus + " → " + d.Status + " (bulk)"
			s.notifyDefect(r, p, d, "defect.status")
		}
		if c.oldAssignee != d.Assignee && d.Assignee != "" {
			s.notifyDefect(r, p, d, "defect.assigned")
		}
		s.notifyEvent(r, p, pr.ID, "defect.updated", map[string]any{"defect": d, "bulk": true})
		s.audit(ctx, p, "defect.update", "defect", d.ID, detail)
	}
	s.audit(ctx, p, "defect.bulk_update", "defect", "", fmt.Sprintf("%d defects", len(changes)))
	writeJSON(w, http.StatusOK, map[string]int{"updated": len(changes)})
}

// handleBulkDeleteDefects: DELETE /projects/{pid}/defects/bulk {"ids": [...]}.
func (s *Server) handleBulkDeleteDefects(w http.ResponseWriter, r *http.Request, p *Principal) {
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
	list, err := s.bulkDefectsOf(r, p, pr.ID, req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, d := range list {
		if err := s.Store.DeleteDefect(r.Context(), p.TenantID(), d.ID); err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", d.Key, err))
			return
		}
		s.audit(r.Context(), p, "defect.delete", "defect", d.ID, d.Key+" (bulk)")
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": len(list)})
}

// handleBulkPushDefects: POST /projects/{pid}/defects/push {"ids": [...]}
// creates tracker issues for the defects that have none yet.
func (s *Server) handleBulkPushDefects(w http.ResponseWriter, r *http.Request, p *Principal) {
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
	list, err := s.bulkDefectsOf(r, p, pr.ID, req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	c, t, err := s.trackerClient(r, p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pushed, skipped := []map[string]string{}, []string{}
	failed := []map[string]string{}
	for _, d := range list {
		if d.ExternalKey != "" {
			skipped = append(skipped, d.Key)
			continue
		}
		if err := s.pushDefect(r, p, c, t, d); err != nil {
			failed = append(failed, map[string]string{"key": d.Key, "error": err.Error()})
			continue
		}
		pushed = append(pushed, map[string]string{"key": d.Key, "external_key": d.ExternalKey})
	}
	writeJSON(w, http.StatusOK, map[string]any{"pushed": pushed, "skipped": skipped, "failed": failed})
}

// ---------------------------------------------------------------- risks

func (s *Server) bulkRisksOf(r *http.Request, p *Principal, projectID string, ids []string) ([]*model.Risk, error) {
	ids, err := bulkIDs(ids)
	if err != nil {
		return nil, err
	}
	var out []*model.Risk
	for _, id := range ids {
		k, err := s.Store.GetRisk(r.Context(), p.TenantID(), id)
		if err != nil {
			return nil, err
		}
		if k.ProjectID != projectID {
			return nil, fmt.Errorf("risk %s: %w", id, store.ErrNotFound)
		}
		out = append(out, k)
	}
	return out, nil
}

// handleBulkRisks: POST /projects/{pid}/risks/bulk
// {"ids": [...], "set": {"status", "owner", "category", "likelihood", "impact"},
// "add_requirement_ids": [...], "remove_requirement_ids": [...]}.
func (s *Server) handleBulkRisks(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		IDs []string `json:"ids"`
		Set struct {
			Status, Owner, Category *string
			Likelihood, Impact      *json.Number
		} `json:"set"`
		AddRequirementIDs    []string `json:"add_requirement_ids"`
		RemoveRequirementIDs []string `json:"remove_requirement_ids"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	set := req.Set
	fields := set.Status != nil || set.Owner != nil || set.Category != nil || set.Likelihood != nil || set.Impact != nil
	if !fields && len(req.AddRequirementIDs) == 0 && len(req.RemoveRequirementIDs) == 0 {
		s.fail(w, r, badRequest("nothing to change"))
		return
	}
	num := func(n *json.Number) (int, error) {
		v, err := n.Int64()
		if err != nil {
			return 0, badRequest("likelihood and impact must be whole numbers from 1 to 5")
		}
		return int(v), nil
	}
	list, err := s.bulkRisksOf(r, p, pr.ID, req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, k := range list {
		for _, f := range []struct{ src, dst *string }{{set.Status, &k.Status}, {set.Owner, &k.Owner}, {set.Category, &k.Category}} {
			if f.src != nil {
				*f.dst = *f.src
			}
		}
		for _, f := range []struct {
			src *json.Number
			dst *int
		}{{set.Likelihood, &k.Likelihood}, {set.Impact, &k.Impact}} {
			if f.src != nil {
				v, err := num(f.src)
				if err != nil {
					s.fail(w, r, err)
					return
				}
				if v == 0 {
					s.fail(w, r, badRequest("likelihood and impact must be whole numbers from 1 to 5"))
					return
				}
				*f.dst = v
			}
		}
		if err := store.ValidateRisk(k); err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", k.Key, err))
			return
		}
	}
	ctx := r.Context()
	for _, k := range list {
		if fields {
			if err := s.Store.UpdateRisk(ctx, p.TenantID(), k); err != nil {
				s.fail(w, r, fmt.Errorf("%s: %w", k.Key, err))
				return
			}
		}
		if len(req.AddRequirementIDs) > 0 || len(req.RemoveRequirementIDs) > 0 {
			if err := s.Store.SetRiskRequirements(ctx, p.TenantID(), k.ID, req.AddRequirementIDs, req.RemoveRequirementIDs); err != nil {
				s.fail(w, r, fmt.Errorf("%s: %w", k.Key, err))
				return
			}
		}
		s.audit(ctx, p, "risk.update", "risk", k.ID, k.Key+" "+k.Status+" (bulk)")
		s.notifyEvent(r, p, pr.ID, "risk.updated", map[string]any{"risk": k, "bulk": true})
	}
	s.audit(ctx, p, "risk.bulk_update", "risk", "", fmt.Sprintf("%d risks", len(list)))
	writeJSON(w, http.StatusOK, map[string]int{"updated": len(list)})
}

// handleBulkDeleteRisks: DELETE /projects/{pid}/risks/bulk {"ids": [...]}.
func (s *Server) handleBulkDeleteRisks(w http.ResponseWriter, r *http.Request, p *Principal) {
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
	list, err := s.bulkRisksOf(r, p, pr.ID, req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, k := range list {
		if err := s.Store.DeleteRisk(r.Context(), p.TenantID(), k.ID); err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", k.Key, err))
			return
		}
		s.audit(r.Context(), p, "risk.delete", "risk", k.ID, k.Key+" (bulk)")
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": len(list)})
}
