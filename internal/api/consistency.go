package api

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/rom/qtestframe/internal/catalog"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
	"github.com/rom/qtestframe/internal/tracerules"
)

// Built-in consistency checks (internal/tracerules builtin.go): the facts
// they need, installing their project defaults as trace rules, dismissed
// findings and the consistency section of the analytics.

func hasBuiltin(rules []model.TraceRule) bool {
	for _, r := range rules {
		if model.IsBuiltinCheck(r.Kind) {
			return true
		}
	}
	return false
}

// regulationKinds are the source kinds that count as regulations.
var regulationKinds = []string{"law", "directive"}

// traceFacts fills what the built-in checks look at into g.
func (s *Server) traceFacts(ctx context.Context, tenantID string, pr *model.Project, pc *projectContext, g *tracerules.Graph, reqs []model.Requirement, tests []model.TestCase) error {
	f := &g.Facts
	f.Reqs = make(map[string]tracerules.ReqFacts, len(reqs))
	f.FromRegulation, f.Justified = map[string]bool{}, map[string]bool{}
	cat, _ := s.catalogue() // without a catalogue nothing counts as a regulation
	for i := range reqs {
		rq := &reqs[i]
		f.Reqs[rq.ID] = tracerules.ReqFacts{Status: rq.Status, Obligation: rq.Obligation, CatalogRef: rq.CatalogRef,
			Text: strings.TrimSpace(rq.Title + " " + rq.Description), Created: rq.CreatedAt}
		if rq.CatalogRef == "" || cat == nil {
			continue
		}
		cr := cat.Requirements[rq.CatalogRef]
		if cr == nil || !fromRegulation(cat, cr) {
			continue
		}
		f.FromRegulation[rq.ID] = true
		f.Justified[rq.ID] = pc.comments[rq.ID] > 0 || ownRationale(rq.Rationale, cr)
	}
	f.TestStatus = make(map[string]string, len(tests))
	for _, t := range tests {
		f.TestStatus[t.ID] = t.Status
	}
	f.Dependencies = pc.deps
	rels, err := s.Store.ListReleases(ctx, tenantID, pr.ID)
	if err != nil {
		return err
	}
	scopes, err := s.Store.ReleaseScopes(ctx, tenantID, pr.ID)
	if err != nil {
		return err
	}
	for _, rl := range rels {
		f.Releases = append(f.Releases, tracerules.Release{ID: rl.ID, Name: rl.Name, Status: rl.Status, RequirementIDs: scopes[rl.ID]})
	}
	risks, err := s.Store.ListRisks(ctx, tenantID, pr.ID)
	if err != nil {
		return err
	}
	for _, k := range risks {
		f.Risks = append(f.Risks, tracerules.Risk{ID: k.ID, Key: k.Key, Title: k.Title, Status: k.Status, Likelihood: k.Likelihood, Impact: k.Impact, RequirementIDs: k.RequirementIDs})
	}
	trashed, err := s.Store.TrashedReferences(ctx, tenantID, pr.ID)
	if err != nil {
		return err
	}
	f.Deleted = map[string]tracerules.Item{}
	for id, it := range trashed.Items {
		f.Deleted[id] = tracerules.Item{Kind: it.Kind, ID: it.ID, Key: it.Key, Title: it.Title, Status: tracerules.ReasonDeleted}
	}
	f.DeletedRelations, f.DeletedDependencies = trashed.Relations, trashed.Dependencies
	dis, err := s.Store.ListConsistencyDismissals(ctx, tenantID, pr.ID)
	if err != nil {
		return err
	}
	f.Dismissed = map[string]bool{}
	for _, d := range dis {
		f.Dismissed[tracerules.DismissKey(d.Check, d.ItemA, d.ItemB)] = true
	}
	return nil
}

// fromRegulation: a catalogue requirement cites a law or directive.
func fromRegulation(c *catalog.Catalog, r *catalog.Requirement) bool {
	for _, id := range r.Sources {
		if src := c.Source(id); src != nil && slices.Contains(regulationKinds, src.Kind) {
			return true
		}
	}
	return false
}

// ownRationale: the project wrote its own rationale (not the catalogue's
// text in any language), which records why the requirement is rejected.
func ownRationale(rationale string, cr *catalog.Requirement) bool {
	r := strings.TrimSpace(rationale)
	if r == "" {
		return false
	}
	for _, t := range cr.Text {
		if strings.TrimSpace(t.Rationale) == r {
			return false
		}
	}
	return true
}

// builtinState is a built-in check with its default and the project's rule.
type builtinState struct {
	Kind        string           `json:"kind"`
	Scope       string           `json:"scope"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Severity    string           `json:"severity"`
	Enabled     bool             `json:"enabled"`
	Installed   bool             `json:"installed"`
	Rule        *model.TraceRule `json:"rule,omitempty"`
}

func builtinStates(rules []model.TraceRule) []builtinState {
	out := make([]builtinState, 0, len(tracerules.Builtins))
	for _, b := range tracerules.Builtins {
		st := builtinState{Kind: b.Kind, Scope: model.BuiltinScope(b.Kind), Name: b.Name, Description: b.Description, Severity: b.Severity, Enabled: b.Enabled}
		for i := range rules {
			if rules[i].Kind == b.Kind {
				r := rules[i]
				st.Installed, st.Rule = true, &r
				break
			}
		}
		out = append(out, st)
	}
	return out
}

// handleListBuiltinRules: GET /projects/{pid}/trace-rules/builtins — the
// built-in checks with their project defaults and whether the project has
// them (installed, with its rule).
func (s *Server) handleListBuiltinRules(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	rules, err := s.Store.ListTraceRules(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(builtinStates(rules)))
}

// handleInstallBuiltinRules: POST /projects/{pid}/trace-rules/builtins
// {kinds?} — creates the built-in checks the project does not have yet
// (all, or the given kinds) with their defaults. Returns the created rules.
func (s *Server) handleInstallBuiltinRules(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var body struct {
		Kinds []string `json:"kinds"`
	}
	if err := decodeOptional(r, &body); err != nil {
		s.fail(w, r, err)
		return
	}
	for _, k := range body.Kinds {
		if !model.IsBuiltinCheck(k) {
			s.fail(w, r, badRequest("kind %q is not a built-in check (%s)", k, strings.Join(model.TraceBuiltinKinds, ", ")))
			return
		}
	}
	ctx := r.Context()
	rules, err := s.Store.ListTraceRules(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	names := map[string]bool{}
	for _, x := range rules {
		names[x.Name] = true
	}
	created := []model.TraceRule{}
	for _, st := range builtinStates(rules) {
		if st.Installed || (len(body.Kinds) > 0 && !slices.Contains(body.Kinds, st.Kind)) {
			continue
		}
		var b tracerules.Builtin
		for _, x := range tracerules.Builtins {
			if x.Kind == st.Kind {
				b = x
			}
		}
		rule := tracerules.BuiltinRule(b, pr.ID)
		for n := 2; names[rule.Name]; n++ { // a rule of the same name exists
			rule.Name = b.Name + " (" + itoa(n) + ")"
		}
		names[rule.Name] = true
		rule.CreatedBy = p.User.Email
		if err := s.Store.CreateTraceRule(ctx, p.TenantID(), &rule); err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(ctx, p, "trace_rule.create", "trace_rule", rule.ID, rule.Name)
		created = append(created, rule)
	}
	writeJSON(w, http.StatusCreated, listOf(created))
}

// dismissBody names a finding: the check, the item that has the
// violation and the other item (related).
type dismissBody struct {
	Check   string `json:"check"`
	ItemID  string `json:"item_id"`
	OtherID string `json:"other_id"`
}

func (s *Server) handleListDismissals(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	list, err := s.Store.ListConsistencyDismissals(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

// handleDismiss: POST (dismiss) or DELETE (restore) /projects/{pid}/
// consistency/dismissals {check, item_id, other_id}. Needs write
// permission on the item kind of the check (requirement.write; test.write
// for prerequisites).
func (s *Server) handleDismiss(add bool) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		pr, ok := s.project(w, r, p)
		if !ok {
			return
		}
		var b dismissBody
		if err := decode(r, &b); err != nil {
			s.fail(w, r, err)
			return
		}
		if !slices.Contains(store.DismissableChecks, b.Check) {
			s.fail(w, r, badRequest("check must be one of %s", strings.Join(store.DismissableChecks, ", ")))
			return
		}
		perm := rbac.RequirementWr
		if b.Check == model.CheckBrokenPrerequisite {
			perm = rbac.TestWrite
		}
		ctx := r.Context()
		if err := s.canIn(ctx, p, pr.ID, perm); err != nil {
			s.fail(w, r, err)
			return
		}
		if !add {
			if err := s.Store.UndismissConsistency(ctx, p.TenantID(), pr.ID, b.Check, b.ItemID, b.OtherID); err != nil {
				s.fail(w, r, err)
				return
			}
			s.audit(ctx, p, "consistency.undismiss", "project", pr.ID, b.Check+" "+b.ItemID+" "+b.OtherID)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		d, err := s.Store.DismissConsistency(ctx, p.TenantID(), pr.ID, b.Check, b.ItemID, b.OtherID, p.User.Email)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(ctx, p, "consistency.dismiss", "project", pr.ID, b.Check+" "+b.ItemID+" "+b.OtherID)
		writeJSON(w, http.StatusCreated, d)
	}
}

// ConsistencyReport is the analytics view of the built-in checks: every
// check evaluated, with the project's own rule where it has one and the
// default (not installed, does not count for releases) otherwise.
type ConsistencyReport struct {
	TraceRulesReport
	Checks []builtinState `json:"checks"`
	// Installed: built-in checks the project has as rules.
	Installed int `json:"installed"`
}

// handleConsistency: GET /projects/{pid}/analytics/consistency?limit= —
// evaluates every built-in check on the project.
func (s *Server) handleConsistency(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	rep, err := s.consistencyReport(r.Context(), p.TenantID(), pr, violationLimit(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) consistencyReport(ctx context.Context, tenantID string, pr *model.Project, limit int) (*ConsistencyReport, error) {
	rules, err := s.Store.ListTraceRules(ctx, tenantID, pr.ID)
	if err != nil {
		return nil, err
	}
	states := builtinStates(rules)
	use := make([]model.TraceRule, 0, len(states))
	installed := 0
	for _, st := range states {
		if st.Installed {
			installed++
			use = append(use, *st.Rule)
			continue
		}
		for _, b := range tracerules.Builtins {
			if b.Kind == st.Kind {
				rule := tracerules.BuiltinRule(b, pr.ID)
				rule.ID = "builtin:" + b.Kind
				_ = store.ValidateTraceRule(&rule)
				use = append(use, rule)
			}
		}
	}
	g, err := s.traceGraph(ctx, tenantID, pr, true)
	if err != nil {
		return nil, err
	}
	results := tracerules.Evaluate(use, g, nil)
	limitViolations(results, limit)
	return &ConsistencyReport{TraceRulesReport: summarize(results), Checks: states, Installed: installed}, nil
}
