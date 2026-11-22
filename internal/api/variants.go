package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/query"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

// Variants of a project, the applicability of requirements and tests and
// the coverage per variant (see docs/api.md "Variants and applicability").

func (s *Server) routesVariants(h routeFunc) {
	h("GET /api/v1/projects/{pid}/variants", s.handleListVariants, "project.read")
	h("POST /api/v1/projects/{pid}/variants", s.handleCreateVariant, "project.update")
	h("GET /api/v1/projects/{pid}/variants/matrix", s.handleVariantMatrix, "requirement.read")
	h("PUT /api/v1/projects/{pid}/applicability", s.handleBulkApplicability, "project.read")
	h("GET /api/v1/variants/{id}", s.handleGetVariant, "project.read")
	h("PATCH /api/v1/variants/{id}", s.handleUpdateVariant, "project.update")
	h("DELETE /api/v1/variants/{id}", s.handleDeleteVariant, "project.update")
	h("GET /api/v1/requirements/{id}/applicability", s.handleGetApplicability(model.KindRequirement), "requirement.read")
	h("PUT /api/v1/requirements/{id}/applicability", s.handlePutApplicability(model.KindRequirement), "requirement.write")
	h("GET /api/v1/tests/{id}/applicability", s.handleGetApplicability(model.KindTest), "test.read")
	h("PUT /api/v1/tests/{id}/applicability", s.handlePutApplicability(model.KindTest), "test.write")
	h("GET /api/v1/requirements/{id}/variants", s.handleRequirementVariants, "requirement.read")
}

// ---------------------------------------------------------------- project context

// loadVariants loads the variants and applicability of the context's project.
func (s *Server) loadVariants(ctx context.Context, tenantID string, pc *projectContext) error {
	list, err := s.Store.ListVariants(ctx, tenantID, pc.project.ID)
	if err != nil {
		return err
	}
	pc.variants = store.NewVariantSet(list)
	if pc.applic, err = s.Store.ListApplicability(ctx, tenantID, pc.project.ID); err != nil {
		return err
	}
	for _, a := range pc.applic {
		pc.variants.Refs(a)
	}
	return nil
}

// variantCoverage returns the coverage per variant (loading the executions once).
func (s *Server) variantCoverage(ctx context.Context, tenantID string, pc *projectContext) (*store.VariantCoverage, error) {
	if pc.vcov != nil {
		return pc.vcov, nil
	}
	vc, err := s.Store.NewVariantCoverage(ctx, tenantID, pc.project.ID, pc.variants, pc.applic, pc.r2t)
	if err != nil {
		return nil, err
	}
	pc.vcov = vc
	return vc, nil
}

// applicability returns an item's applicability (nil: all variants).
func (pc *projectContext) applicability(id string) *model.Applicability {
	if pc.applic == nil {
		return nil
	}
	return pc.applic[id]
}

// variantNames are the names of the variants an item applies to (the
// query field variant).
func (pc *projectContext) variantNames(id string) []string {
	if pc.variants.Len() == 0 {
		return nil
	}
	var out []string
	for _, v := range pc.variants.AppliesTo(pc.applicability(id)) {
		out = append(out, v.Name)
	}
	return out
}

// applicabilityKind is "all" or "selected" (the query field applicability).
func (pc *projectContext) applicabilityKind(id string) string {
	if pc.applicability(id).All() {
		return "all"
	}
	return "selected"
}

// listVariant resolves the ?variant= parameter of a list ("" for none);
// an unknown variant is a bad request.
func (pc *projectContext) listVariant(r *http.Request) (string, error) {
	ref := strings.TrimSpace(r.URL.Query().Get("variant"))
	if ref == "" {
		return "", nil
	}
	v := pc.variants.Find(ref)
	if v == nil {
		return "", badRequest("unknown variant %q", ref)
	}
	return v.ID, nil
}

// ---------------------------------------------------------------- variants

type variantInput struct {
	ParentID         *string            `json:"parent_id"`
	Name             *string            `json:"name"`
	Description      *string            `json:"description"`
	Kind             *string            `json:"kind"`
	Attributes       *map[string]string `json:"attributes"`
	ConfigurationIDs *[]string          `json:"configuration_ids"`
	Position         *int               `json:"position"`
}

func (in *variantInput) apply(v *model.Variant) {
	if in.ParentID != nil {
		v.ParentID = in.ParentID
		if *in.ParentID == "" {
			v.ParentID = nil
		}
	}
	for _, f := range []struct{ src, dst *string }{{in.Name, &v.Name}, {in.Description, &v.Description}, {in.Kind, &v.Kind}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	if in.Attributes != nil {
		v.Attributes = *in.Attributes
	}
	if in.ConfigurationIDs != nil {
		v.ConfigurationIDs = *in.ConfigurationIDs
	}
	if in.Position != nil {
		v.Position = *in.Position
	}
}

func (s *Server) handleListVariants(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	list, err := s.Store.ListVariants(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

func (s *Server) handleCreateVariant(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var in variantInput
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	v := &model.Variant{ProjectID: pr.ID, CreatedBy: p.User.Email}
	in.apply(v)
	if err := s.Store.CreateVariant(r.Context(), p.TenantID(), v); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "variant.create", "variant", v.ID, v.Name)
	s.writeVariant(w, r, p, v.ID, http.StatusCreated)
}

func (s *Server) writeVariant(w http.ResponseWriter, r *http.Request, p *Principal, id string, status int) {
	v, err := s.Store.GetVariant(r.Context(), p.TenantID(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.Store.ListVariants(r.Context(), p.TenantID(), v.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if full := store.NewVariantSet(list).Get(id); full != nil {
		v = full
	}
	writeJSON(w, status, v)
}

func (s *Server) handleGetVariant(w http.ResponseWriter, r *http.Request, p *Principal) {
	s.writeVariant(w, r, p, r.PathValue("id"), http.StatusOK)
}

func (s *Server) handleUpdateVariant(w http.ResponseWriter, r *http.Request, p *Principal) {
	v, err := s.Store.GetVariant(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in variantInput
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	in.apply(v)
	if err := s.Store.UpdateVariant(r.Context(), p.TenantID(), v); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "variant.update", "variant", v.ID, v.Name)
	s.writeVariant(w, r, p, v.ID, http.StatusOK)
}

func (s *Server) handleDeleteVariant(w http.ResponseWriter, r *http.Request, p *Principal) {
	v, err := s.Store.GetVariant(r.Context(), p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.DeleteVariant(r.Context(), p.TenantID(), v.ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, "variant.delete", "variant", v.ID, v.Name)
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- applicability

// applicabilityView is an item's applicability with the variants it
// covers.
type applicabilityView struct {
	model.Applicability
	All bool `json:"all"`
	// AppliesTo: the variants it applies to.
	AppliesTo []model.VariantRef `json:"applies_to"`
}

func (s *Server) applicabilityView(r *http.Request, p *Principal, projectID string, a *model.Applicability) (*applicabilityView, error) {
	list, err := s.Store.ListVariants(r.Context(), p.TenantID(), projectID)
	if err != nil {
		return nil, err
	}
	vs := store.NewVariantSet(list)
	vs.Refs(a)
	out := &applicabilityView{Applicability: *a, All: a.All(), AppliesTo: []model.VariantRef{}}
	if out.VariantIDs == nil {
		out.VariantIDs = []string{}
	}
	for _, v := range vs.AppliesTo(a) {
		out.AppliesTo = append(out.AppliesTo, model.VariantRef{ID: v.ID, Name: v.Name})
	}
	return out, nil
}

// itemProject returns the project of a requirement or test.
func (s *Server) itemProject(r *http.Request, p *Principal, kind, id string) (string, string, error) {
	if kind == model.KindTest {
		t, err := s.Store.GetTest(r.Context(), p.TenantID(), id)
		if err != nil {
			return "", "", err
		}
		return t.ProjectID, t.Key, nil
	}
	rq, err := s.Store.GetRequirement(r.Context(), p.TenantID(), id)
	if err != nil {
		return "", "", err
	}
	return rq.ProjectID, rq.Key, nil
}

func (s *Server) handleGetApplicability(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		pid, _, err := s.itemProject(r, p, kind, r.PathValue("id"))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		a, err := s.Store.GetApplicability(r.Context(), p.TenantID(), kind, r.PathValue("id"))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		out, err := s.applicabilityView(r, p, pid, a)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

type applicabilityInput struct {
	VariantIDs []string `json:"variant_ids"`
	Expression string   `json:"expression"`
}

func applicabilityDetails(key string, a *model.Applicability) string {
	if a.All() {
		return key + ": all variants"
	}
	d := fmt.Sprintf("%s: %d variant(s)", key, len(a.VariantIDs))
	if a.Expression != "" {
		d += " expression " + a.Expression
	}
	return d
}

func (s *Server) handlePutApplicability(kind string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p *Principal) {
		id := r.PathValue("id")
		pid, key, err := s.itemProject(r, p, kind, id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		var in applicabilityInput
		if err := decode(r, &in); err != nil {
			s.fail(w, r, err)
			return
		}
		a := &model.Applicability{VariantIDs: in.VariantIDs, Expression: in.Expression}
		if _, err := s.Store.SetApplicability(r.Context(), p.TenantID(), pid, kind, []string{id}, a, p.User.Email); err != nil {
			s.fail(w, r, err)
			return
		}
		s.audit(r.Context(), p, kind+".applicability", kind, id, applicabilityDetails(key, a))
		out, err := s.applicabilityView(r, p, pid, a)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// handleBulkApplicability: PUT /projects/{pid}/applicability — the same
// applicability for several requirements or tests.
func (s *Server) handleBulkApplicability(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var in struct {
		ItemType string   `json:"item_type"`
		ItemIDs  []string `json:"item_ids"`
		applicabilityInput
	}
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	perm := rbac.RequirementWr
	if in.ItemType == model.KindTest {
		perm = rbac.TestWrite
	}
	if !p.Can(perm) {
		writeError(w, http.StatusForbidden, "forbidden", "permission "+string(perm)+" required")
		return
	}
	if len(in.ItemIDs) == 0 || len(in.ItemIDs) > 1000 {
		s.fail(w, r, badRequest("item_ids must list 1 to 1000 items"))
		return
	}
	a := &model.Applicability{VariantIDs: in.VariantIDs, Expression: in.Expression}
	n, err := s.Store.SetApplicability(r.Context(), p.TenantID(), pr.ID, in.ItemType, in.ItemIDs, a, p.User.Email)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.audit(r.Context(), p, in.ItemType+".applicability", "project", pr.ID, applicabilityDetails(fmt.Sprintf("%d %s(s)", n, in.ItemType), a))
	out, err := s.applicabilityView(r, p, pr.ID, a)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": n, "applicability": out})
}

// ---------------------------------------------------------------- coverage per variant

type variantCell struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Coverage string `json:"coverage"`
}

type variantCoverageRow struct {
	Variant  model.Variant `json:"variant"`
	Applies  bool          `json:"applies"`
	Coverage string        `json:"coverage"`
	// Tests: the linked tests that apply to the variant.
	Tests []string `json:"tests"`
	// Configurations: the coverage in each configuration a counted test
	// is planned in (id "" for results without configuration).
	Configurations []variantCell `json:"configurations"`
}

// handleRequirementVariants: GET /requirements/{id}/variants — the
// requirement's coverage in every variant and, per variant, in every
// configuration.
func (s *Server) handleRequirementVariants(w http.ResponseWriter, r *http.Request, p *Principal) {
	ctx := r.Context()
	rq, err := s.Store.GetRequirement(ctx, p.TenantID(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pr, err := s.Store.GetProject(ctx, p.TenantID(), rq.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pc, err := s.loadContext(ctx, p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	vc, err := s.variantCoverage(ctx, p.TenantID(), pc)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cfgs, err := s.Store.ListConfigurations(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	keys := map[string]string{}
	for _, l := range pc.links {
		keys[l.TestCaseID] = l.TestCaseKey
	}
	rows := []variantCoverageRow{}
	for _, v := range pc.variants.List() {
		row := variantCoverageRow{Variant: v, Applies: vc.AppliesTo(rq.ID, v.ID), Coverage: vc.Requirement(rq.ID, v.ID), Tests: []string{}, Configurations: []variantCell{}}
		if row.Applies {
			res := vc.Results(v.ID)
			used := map[string]bool{}
			for _, t := range vc.Tests(rq.ID, v.ID) {
				row.Tests = append(row.Tests, keys[t])
				for cfg := range res[t] {
					used[cfg] = true
				}
			}
			for _, c := range cfgs {
				if used[c.ID] {
					row.Configurations = append(row.Configurations, variantCell{c.ID, c.Name, vc.RequirementIn(rq.ID, v.ID, c.ID)})
				}
			}
			if used[store.NoConfiguration] {
				row.Configurations = append(row.Configurations, variantCell{"", "", vc.RequirementIn(rq.ID, v.ID, store.NoConfiguration)})
			}
			sort.SliceStable(row.Tests, func(i, j int) bool { return naturalKeyLess(row.Tests[i], row.Tests[j]) })
		}
		rows = append(rows, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"requirement_id": rq.ID, "applicability": pc.applicability(rq.ID), "variants": rows})
}

type matrixRequirement struct {
	ID         string `json:"id"`
	Key        string `json:"key"`
	Title      string `json:"title"`
	Obligation string `json:"obligation"`
	// Coverage per variant id (not_applicable where it does not apply).
	Coverage map[string]string `json:"coverage"`
}

// handleVariantMatrix: GET /projects/{pid}/variants/matrix — the coverage
// of the (filtered) requirements in every variant, optionally in one
// configuration (?configuration=<id>|none), with a summary per variant.
func (s *Server) handleVariantMatrix(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	ctx := r.Context()
	pc, err := s.loadContext(ctx, p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cfg, inCfg := r.URL.Query().Get("configuration"), false
	if cfg != "" {
		inCfg = true
		if cfg == "none" {
			cfg = store.NoConfiguration
		} else if c, err := s.Store.GetConfiguration(ctx, p.TenantID(), cfg); err != nil || c.ProjectID != pr.ID {
			s.fail(w, r, badRequest("unknown configuration %q", cfg))
			return
		}
	}
	// the matrix is always over every variant: ?variant= does not filter it
	q, err := query.Parse(r.URL.Query().Get("q"), requirementSchema)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reqs, err := s.Store.ListRequirements(ctx, p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	vc, err := s.variantCoverage(ctx, p.TenantID(), pc)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	variants := pc.variants.List()
	if ids := splitList(r.URL.Query().Get("variants")); len(ids) > 0 {
		var sel []model.Variant
		for _, ref := range ids {
			v := pc.variants.Find(ref)
			if v == nil {
				s.fail(w, r, badRequest("unknown variant %q", ref))
				return
			}
			sel = append(sel, *v)
		}
		variants = sel
	}
	inFolder := pc.folderFilter(r.URL.Query().Get("folder_id"), true)
	summary := map[string]map[string]int{}
	for _, v := range variants {
		summary[v.ID] = map[string]int{}
	}
	rows := []matrixRequirement{}
	for i := range reqs {
		rq := &reqs[i]
		pc.decorateRequirement(rq)
		if !inFolder(rq.FolderID) || !q.Match(pc.requirementRecord(rq)) {
			continue
		}
		row := matrixRequirement{ID: rq.ID, Key: rq.Key, Title: rq.Title, Obligation: rq.Obligation, Coverage: map[string]string{}}
		for _, v := range variants {
			cov := vc.Requirement(rq.ID, v.ID)
			if inCfg {
				cov = vc.RequirementIn(rq.ID, v.ID, cfg)
			}
			row.Coverage[v.ID] = cov
			summary[v.ID][cov]++
			if cov != store.CoverageNotApplicable {
				summary[v.ID]["applicable"]++
			}
		}
		rows = append(rows, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"variants": variants, "requirements": rows, "summary": summary, "configuration": r.URL.Query().Get("configuration")})
}
