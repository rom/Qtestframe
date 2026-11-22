package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Change and defect correlations (store/correlations.go, staleness.go,
// regressions.go): change impact across relations, result staleness,
// defects that block requirements and regressions.

func (s *Server) routesCorrelations(h routeFunc) {
	h("GET /api/v1/projects/{pid}/correlation-settings", s.handleGetCorrelationSettings, "project.read")
	h("PUT /api/v1/projects/{pid}/correlation-settings", s.handlePutCorrelationSettings, "project.update")
	h("GET /api/v1/requirements/{id}/impact", s.handleImpact(model.KindRequirement), "requirement.read")
	h("GET /api/v1/tests/{id}/impact", s.handleImpact(model.KindTest), "test.read")
	h("GET /api/v1/baselines/{id}/impact", s.handleBaselineImpact, "requirement.read")
	h("POST /api/v1/projects/{pid}/impact", s.handleProjectImpact, "requirement.read")
	h("GET /api/v1/projects/{pid}/impact/export", s.handleProjectImpactExport, "requirement.read")
	h("POST /api/v1/relations/{id}/confirm", s.handleConfirmRelation, "requirement.write")
	h("POST /api/v1/requirements/{id}/relations/confirm", s.handleConfirmRequirementRelations, "requirement.write")
	h("POST /api/v1/item-links/{id}/issue/confirm", s.handleConfirmIssue, "requirement.write")
	h("GET /api/v1/requirements/{id}/blocking-defects", s.handleBlockingDefects, "requirement.read")
	h("GET /api/v1/requirements/{id}/stale-results", s.handleStaleResults, "requirement.read")
	h("GET /api/v1/defects/{id}/regressions", s.handleRegressions(model.KindDefect), "project.read")
	h("GET /api/v1/tests/{id}/regressions", s.handleRegressions(model.KindTest), "test.read")
}

// ---------------------------------------------------------------- settings

func (s *Server) handleGetCorrelationSettings(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	set, err := s.Store.CorrelationSettings(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, set)
}

func (s *Server) handlePutCorrelationSettings(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	// a partial body keeps the current values of the fields it leaves out
	set, err := s.Store.CorrelationSettings(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var raw map[string]json.RawMessage
	if err := decode(r, &raw); err != nil {
		s.fail(w, r, err)
		return
	}
	if v, ok := raw["relation_impact"]; ok {
		// the relation map is replaced, not merged
		set.RelationImpact = nil
		if err := json.Unmarshal(v, &set.RelationImpact); err != nil {
			s.fail(w, r, badRequest("relation_impact must map relation types to %s", strings.Join(model.ImpactDirections, ", ")))
			return
		}
	}
	delete(raw, "relation_impact")
	b, _ := json.Marshal(raw)
	if err := json.Unmarshal(b, set); err != nil {
		s.fail(w, r, badRequest("invalid settings: %v", err))
		return
	}
	if err := s.Store.SaveCorrelationSettings(r.Context(), p.TenantID(), pr.ID, set); err != nil {
		s.fail(w, r, err)
		return
	}
	out, _ := json.Marshal(set)
	s.audit(r.Context(), p, "correlation_settings.update", "project", pr.ID, string(out))
	writeJSON(w, http.StatusOK, set)
}

// ---------------------------------------------------------------- impact

// itemProject returns the project of a requirement or test.
func (s *Server) itemProject(ctx context.Context, p *Principal, kind, id string) (projectID, key string, err error) {
	if kind == model.KindTest {
		t, err := s.Store.GetTest(ctx, p.TenantID(), id)
		if err != nil {
			return "", "", err
		}
		return t.ProjectID, t.Key, nil
	}
	q, err := s.Store.GetRequirement(ctx, p.TenantID(), id)
	if err != nil {
		return "", "", err
	}
	return q.ProjectID, q.Key, nil
}

// handleImpact: GET /requirements/{id}/impact and /tests/{id}/impact —
// what a change of the item's substance makes suspect, out of date,
// outdated or stale, or recommends for review, up to the project's impact
// depth or ?depth= (the preview before saving, and the Impact tab).
func (s *Server) handleImpact(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		ctx := r.Context()
		depth, err := itemImpactDepth(r)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		pid, _, err := s.itemProject(ctx, p, kind, r.PathValue("id"))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		x, err := s.Store.LoadImpactIndex(ctx, p.TenantID(), pid)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !x.Has(kind, r.PathValue("id")) {
			s.fail(w, r, store.ErrNotFound)
			return
		}
		imp, _ := x.Analyze([]store.ImpactRoot{{Kind: kind, ID: r.PathValue("id")}}, depth)
		s.redactImpact(p, imp)
		writeJSON(w, http.StatusOK, imp)
	}
}

// redactImpact leaves out the tests (and their results) a caller without
// test.read may not see, with what is reached through them.
func (s *Server) redactImpact(p *Principal, imp *model.ChangeImpact) {
	if p.Can(rbac.TestRead) {
		return
	}
	store.FilterImpact(imp, func(it model.ImpactItem) bool {
		return it.Kind != model.KindTest && it.Kind != "result"
	})
}

// BaselineImpact is the impact of the changes between a baseline and the
// current state: per changed item whose substance changed, and in total.
type BaselineImpact struct {
	Baseline string `json:"baseline"`
	// Changes: the items whose substance changed since the baseline.
	Changes []BaselineImpactChange `json:"changes"`
	// Totals: distinct items affected, by kind (requirements, tests,
	// issues, documents, results).
	Totals map[string]int `json:"totals"`
}

// BaselineImpactChange is one changed item with its impact.
type BaselineImpactChange struct {
	model.BaselineChange
	Impact *model.ChangeImpact `json:"impact"`
}

// handleBaselineImpact: GET /baselines/{id}/impact — the project-wide
// impact of what changed since the baseline.
func (s *Server) handleBaselineImpact(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	b, err := s.Store.GetBaseline(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pr, err := s.Store.GetProject(ctx, p.TenantID(), b.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items, err := s.currentItems(r, p, pr, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	diff := store.CompareItems(b.Name, "current", b.Items, items)
	x, err := s.Store.LoadImpactIndex(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := BaselineImpact{Baseline: b.Name, Changes: []BaselineImpactChange{}, Totals: map[string]int{}}
	seen := map[string]map[string]bool{}
	for _, c := range diff.Changed {
		if len(store.ContentChanged(c.EntityType, c.ChangedFields)) == 0 {
			continue
		}
		if c.EntityType == model.KindTest && !p.Can(rbac.TestRead) {
			continue
		}
		imp := x.Of(c.EntityType, c.EntityID)
		if imp == nil {
			continue
		}
		s.redactImpact(p, imp)
		out.Changes = append(out.Changes, BaselineImpactChange{BaselineChange: c, Impact: imp})
		for kind, list := range map[string][]model.ImpactItem{"requirements": imp.Requirements, "tests": imp.Tests, "issues": imp.Issues, "documents": imp.Documents, "results": imp.Results} {
			if seen[kind] == nil {
				seen[kind] = map[string]bool{}
			}
			for _, it := range list {
				seen[kind][it.ID] = true
			}
		}
	}
	for _, kind := range []string{"requirements", "tests", "issues", "documents", "results"} {
		out.Totals[kind] = len(seen[kind])
	}
	sort.SliceStable(out.Changes, func(i, j int) bool { return out.Changes[i].Impact.Total > out.Changes[j].Impact.Total })
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------- confirm

func (s *Server) handleConfirmRelation(w http.ResponseWriter, r *http.Request, p *Principal) {
	if _, err := s.Store.GetRelation(r.Context(), p.TenantID(), r.PathValue("id")); err != nil {
		s.fail(w, r, err)
		return
	}
	s.confirmRelations(w, r, p, []string{r.PathValue("id")}, "")
}

func (s *Server) handleConfirmRequirementRelations(w http.ResponseWriter, r *http.Request, p *Principal) {
	if err := s.itemExists(r.Context(), p, model.KindRequirement, r.PathValue("id")); err != nil {
		s.fail(w, r, err)
		return
	}
	s.confirmRelations(w, r, p, nil, r.PathValue("id"))
}

func (s *Server) confirmRelations(w http.ResponseWriter, r *http.Request, p *Principal, ids []string, reqID string) {
	rels, err := s.Store.ConfirmRelations(r.Context(), p.TenantID(), ids, reqID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, rel := range rels {
		s.audit(r.Context(), p, "relation.confirm", "relation", rel.ID, rel.SourceKey+" "+rel.Type+" "+rel.TargetKey)
	}
	writeJSON(w, http.StatusOK, map[string]any{"confirmed": len(rels)})
}

// handleConfirmIssue: POST /item-links/{id}/issue/confirm — the issue is
// still right although its requirement changed; clears the flag.
func (s *Server) handleConfirmIssue(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	l, err := s.Store.GetItemLink(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if l.Issue == nil || l.SourceType != model.KindRequirement {
		s.fail(w, r, badRequest("the link is not an issue of a requirement"))
		return
	}
	if err := s.Store.ClearIssueOutdated(ctx, p.TenantID(), l.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(ctx, p, "requirement.issue.confirm", model.KindRequirement, l.SourceID, l.Issue.Key)
	l, err = s.Store.GetItemLink(ctx, p.TenantID(), l.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

// ---------------------------------------------------------------- blocking defects, staleness

// handleBlockingDefects: GET /requirements/{id}/blocking-defects — the
// open defects that block the requirement or warn about it.
func (s *Server) handleBlockingDefects(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	q, err := s.Store.GetRequirement(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	set, err := s.Store.CorrelationSettings(ctx, p.TenantID(), q.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	blocks, err := s.Store.DefectBlocks(ctx, p.TenantID(), q.ProjectID, set)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list := blocks[q.ID]
	if list == nil {
		list = []model.DefectBlock{}
	}
	n, warn := store.BlockCounts(list)
	writeJSON(w, http.StatusOK, map[string]any{"blocked": n > 0, "blocking": n, "warnings": warn, "items": list})
}

// StaleResult is the staleness of the latest result of a test verifying a
// requirement.
type StaleResult struct {
	TestID string `json:"test_id"`
	Key    string `json:"key"`
	Title  string `json:"title"`
	Result string `json:"result"`
	model.Staleness
}

// handleStaleResults: GET /requirements/{id}/stale-results — the latest
// results of the tests verifying the requirement with their staleness.
func (s *Server) handleStaleResults(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	q, err := s.Store.GetRequirement(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := []StaleResult{}
	if !p.Can(rbac.TestRead) {
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
		return
	}
	x, err := s.Store.LoadStaleIndex(ctx, p.TenantID(), q.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	last, err := s.Store.LastResults(ctx, p.TenantID(), q.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	links, err := s.Store.ListLinks(ctx, p.TenantID(), q.ProjectID, q.ID, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, l := range links {
		t, err := s.Store.GetTest(ctx, p.TenantID(), l.TestCaseID)
		if err != nil {
			continue
		}
		res := last[t.ID]
		if res == "" {
			res = model.ResultNotRun
		}
		out = append(out, StaleResult{TestID: t.ID, Key: t.Key, Title: t.Title, Result: res, Staleness: x.Test(t.ID)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "stale_after_days": x.Settings().StaleAfterDays, "stale_on_change": x.Settings().StaleOnChange})
}

// ---------------------------------------------------------------- regressions

// handleRegressions: GET /defects/{id}/regressions and /tests/{id}/regressions.
func (s *Server) handleRegressions(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		ctx := r.Context()
		id := r.PathValue("id")
		var err error
		if kind == model.KindDefect {
			_, err = s.Store.GetDefect(ctx, p.TenantID(), id)
		} else {
			_, err = s.Store.GetTest(ctx, p.TenantID(), id)
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		list, err := s.Store.ListRegressions(ctx, p.TenantID(), kind, id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": list})
	}
}

// detectRegressions handles a failed execution: reopens (or follows up
// with a regression defect) the fixed defects its test verified, sends the
// webhooks and tells each defect's owner (assignee, else its author).
func (s *Server) detectRegressions(r *http.Request, p *Principal, e *model.Execution) {
	if e == nil || e.Status != model.ResultFailed {
		return
	}
	ctx := r.Context()
	outs, err := s.Store.DetectRegressions(ctx, p.TenantID(), e, p.User.Email)
	if err != nil {
		s.Log.Warn("detect regressions", "err", err)
		return
	}
	for _, oc := range outs {
		reg := oc.Regression
		d := oc.Defect
		switch reg.Action {
		case "created":
			s.audit(ctx, p, "defect.regression", model.KindDefect, oc.Created.ID, oc.Created.Key+" regression of "+d.Key+" ("+reg.TestKey+" failed)")
			s.notifyEvent(r, p, d.ProjectID, "defect.created", map[string]any{"defect": oc.Created})
			d = oc.Created
		default:
			s.audit(ctx, p, "defect.regression", model.KindDefect, d.ID, d.Key+" reopened ("+reg.TestKey+" failed; was "+reg.PreviousStatus+")")
			s.notifyEvent(r, p, d.ProjectID, "defect.status", map[string]any{"defect": d})
		}
		s.notifyEvent(r, p, d.ProjectID, "defect.regression", map[string]any{"regression": reg, "defect": d})
		owner := d.Assignee
		if owner == "" {
			owner = d.CreatedBy
		}
		if owner == "" {
			continue
		}
		title := d.Key + " " + d.Title
		body := reg.TestKey + " failed again"
		if reg.Action == "created" {
			body += " (regression of " + reg.DefectKey + ")"
		} else {
			body += " (reopened, was " + reg.PreviousStatus + ")"
		}
		s.notifyUsers(ctx, p, s.recipients(ctx, p, owner), model.Notification{ProjectID: d.ProjectID, Event: "defect.regression",
			Title: title, Body: body, EntityType: model.KindDefect, EntityID: d.ID})
	}
}
