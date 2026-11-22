package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

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
	if err := s.normalizeOwner(r.Context(), p, pr.ID, set.Owner); err != nil {
		s.fail(w, r, err)
		return
	}
	oldOwners := map[string]string{}
	for _, k := range list {
		oldOwners[k.ID] = k.Owner
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
		s.riskAssigned(r, p, k, oldOwners[k.ID], k.ReviewOwner)
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

// ---------------------------------------------------------------- releases

// handleBulkReleases: POST /projects/{pid}/releases/bulk
// {"ids": [...], "set": {"status", "target_date", "exit_criteria"},
// "add_requirement_ids", "remove_requirement_ids", "query"}.
// Releasing needs a go for each release: releases with a no-go are
// skipped (reason "no_go"); an override needs the single-release dialog.
func (s *Server) handleBulkReleases(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		IDs []string `json:"ids"`
		Set struct {
			Status       *string                `json:"status"`
			TargetDate   *string                `json:"target_date"`
			ExitCriteria *[]model.ExitCriterion `json:"exit_criteria"`
		} `json:"set"`
		AddRequirementIDs    []string `json:"add_requirement_ids"`
		RemoveRequirementIDs []string `json:"remove_requirement_ids"`
		Query                string   `json:"query"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	set := req.Set
	fields := set.Status != nil || set.TargetDate != nil || set.ExitCriteria != nil
	scope := len(req.AddRequirementIDs) > 0 || len(req.RemoveRequirementIDs) > 0 || req.Query != ""
	if !fields && !scope {
		s.fail(w, r, badRequest("nothing to change"))
		return
	}
	if scope && set.Status != nil && *set.Status == "released" {
		s.fail(w, r, badRequest("change the scope first, then mark the releases released"))
		return
	}
	ids, err := bulkIDs(req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	var list []*model.Release
	for _, id := range ids {
		rel, err := s.Store.GetRelease(ctx, p.TenantID(), id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if rel.ProjectID != pr.ID {
			s.fail(w, r, fmt.Errorf("release %s: %w", id, store.ErrNotFound))
			return
		}
		list = append(list, rel)
	}
	add := req.AddRequirementIDs
	if req.Query != "" {
		pc, err := s.loadContext(ctx, p.TenantID(), pr)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		matches, err := s.filterRequirements(r, p, pr, pc, req.Query, "", true)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, q := range matches {
			add = append(add, q.ID)
		}
	}
	type skip struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Reason string `json:"reason"`
	}
	updated, skipped := []string{}, []skip{}
	// check every release before saving any
	type plan struct {
		rel       *model.Release
		releasing bool
		report    *store.ReleaseStatusReport
	}
	var plans []plan
	for _, rel := range list {
		pl := plan{rel: rel}
		if set.Status != nil && *set.Status != rel.Status {
			if *set.Status == "released" {
				rep, err := s.releaseStatus(r, p, rel.ID)
				if err != nil {
					s.fail(w, r, err)
					return
				}
				if !rep.Go {
					skipped = append(skipped, skip{rel.ID, rel.Name, "no_go"})
					continue
				}
				pl.releasing, pl.report = true, rep
			}
			rel.Status = *set.Status
		}
		if set.TargetDate != nil {
			rel.TargetDate = *set.TargetDate
		}
		if set.ExitCriteria != nil {
			rel.ExitCriteria = append([]model.ExitCriterion(nil), (*set.ExitCriteria)...)
		}
		if err := store.ValidateRelease(rel); err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", rel.Name, err))
			return
		}
		plans = append(plans, pl)
	}
	for _, pl := range plans {
		rel := pl.rel
		if scope {
			if _, _, err := s.Store.SetReleaseRequirements(ctx, p.TenantID(), rel.ID, add, req.RemoveRequirementIDs); err != nil {
				s.fail(w, r, fmt.Errorf("%s: %w", rel.Name, err))
				return
			}
		}
		if fields {
			if err := s.Store.UpdateRelease(ctx, p.TenantID(), rel, p.User.Email); err != nil {
				s.fail(w, r, fmt.Errorf("%s: %w", rel.Name, err))
				return
			}
		}
		updated = append(updated, rel.Name)
		s.audit(ctx, p, "release.update", "release", rel.ID, rel.Name+" "+rel.Status+" (bulk)")
		out, _ := s.Store.GetRelease(ctx, p.TenantID(), rel.ID)
		if out == nil {
			out = rel
		}
		s.notifyEvent(r, p, pr.ID, "release.updated", map[string]any{"release": out, "bulk": true})
		if pl.releasing {
			s.audit(ctx, p, "release.released", "release", rel.ID, rel.Name+" (go, bulk)")
			s.releaseBaseline(r, p, out)
			s.notifyEvent(r, p, pr.ID, "release.released", map[string]any{"release": out, "status": pl.report})
		}
	}
	s.audit(ctx, p, "release.bulk_update", "release", "", fmt.Sprintf("%d updated, %d skipped", len(updated), len(skipped)))
	writeJSON(w, http.StatusOK, map[string]any{"updated": updated, "skipped": skipped})
}

// handleBulkDeleteReleases: DELETE /projects/{pid}/releases/bulk {"ids": [...]}.
func (s *Server) handleBulkDeleteReleases(w http.ResponseWriter, r *http.Request, p *Principal) {
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
	var list []*model.Release
	for _, id := range ids {
		rel, err := s.Store.GetRelease(r.Context(), p.TenantID(), id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if rel.ProjectID != pr.ID {
			s.fail(w, r, fmt.Errorf("release %s: %w", id, store.ErrNotFound))
			return
		}
		list = append(list, rel)
	}
	for _, rel := range list {
		if err := s.Store.DeleteRelease(r.Context(), p.TenantID(), rel.ID); err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", rel.Name, err))
			return
		}
		s.audit(r.Context(), p, "release.delete", "release", rel.ID, rel.Name+" (bulk)")
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": len(list)})
}

// ---------------------------------------------------------------- baselines

type bulkBaseline struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Reason      string `json:"reason,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
}

func (s *Server) bulkBaselinesOf(r *http.Request, p *Principal, projectID string, ids []string) ([]*model.Baseline, error) {
	ids, err := bulkIDs(ids)
	if err != nil {
		return nil, err
	}
	var out []*model.Baseline
	for _, id := range ids {
		b, err := s.Store.GetBaseline(r.Context(), p.TenantID(), id)
		if err != nil {
			return nil, err
		}
		if b.ProjectID != projectID {
			return nil, fmt.Errorf("baseline %s: %w", id, store.ErrNotFound)
		}
		out = append(out, b)
	}
	return out, nil
}

// handleBulkDeleteBaselines: DELETE /projects/{pid}/baselines/bulk {"ids": [...]}.
// Signed baselines are kept and listed in skipped (reason "signed").
func (s *Server) handleBulkDeleteBaselines(w http.ResponseWriter, r *http.Request, p *Principal) {
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
	list, err := s.bulkBaselinesOf(r, p, pr.ID, req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	skipped := []bulkBaseline{}
	n := 0
	for _, b := range list {
		if c, err := s.Store.SignoffCount(ctx, p.TenantID(), b.ID); err != nil {
			s.fail(w, r, err)
			return
		} else if c > 0 {
			skipped = append(skipped, bulkBaseline{ID: b.ID, Name: b.Name, Reason: "signed"})
			continue
		}
		if err := s.Store.DeleteBaseline(ctx, p.TenantID(), b.ID); err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", b.Name, err))
			return
		}
		n++
		s.audit(ctx, p, "baseline.delete", "baseline", b.ID, b.Name+" (bulk)")
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": n, "skipped": skipped})
}

// handleBulkSignBaselines: POST /projects/{pid}/baselines/signoffs
// {"ids": [...], "meaning", "comment", "password"}: one confirmation signs
// every baseline; each signature has the fingerprint of its own baseline.
// Baselines the caller already signed with that meaning are skipped
// (reason "already").
func (s *Server) handleBulkSignBaselines(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req signRequest
	if !s.checkSigner(w, r, p, &req) {
		return
	}
	if req.Meaning == "" {
		s.fail(w, r, badRequest("meaning must be one of %s", strings.Join(model.SignoffMeanings, ", ")))
		return
	}
	list, err := s.bulkBaselinesOf(r, p, pr.ID, req.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	signed, skipped := []bulkBaseline{}, []bulkBaseline{}
	for _, b := range list {
		// a savepoint: a conflict skips this baseline, the rest goes on
		var so *model.Signoff
		err := s.Store.Savepoint(r.Context(), func() (err error) {
			so, err = s.signBaseline(r, p, b.ID, req)
			return err
		})
		if errors.Is(err, store.ErrConflict) {
			skipped = append(skipped, bulkBaseline{ID: b.ID, Name: b.Name, Reason: "already"})
			continue
		}
		if err != nil {
			s.fail(w, r, fmt.Errorf("%s: %w", b.Name, err))
			return
		}
		signed = append(signed, bulkBaseline{ID: b.ID, Name: b.Name, ContentHash: so.ContentHash})
	}
	writeJSON(w, http.StatusOK, map[string]any{"signed": signed, "skipped": skipped})
}
