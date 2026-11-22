package api

import (
	"net/http"
	"slices"
	"strings"

	"github.com/rom/qtestframe/internal/analytics"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/reqlang"
)

// handleAnalytics: GET /projects/{pid}/analytics?lang= — analyses the
// requirements of a project (see package analytics). Test findings are
// included when the caller may read tests. lang is the analysis language
// ("" or "auto" detects it per requirement).
func (s *Server) handleAnalytics(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang != "" && lang != "auto" && !reqlang.Supported(lang) {
		s.fail(w, r, badRequest("lang must be auto or one of the analysis languages"))
		return
	}
	ctx := r.Context()
	pc, err := s.loadContext(ctx, p.TenantID(), pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
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
	// ?variant=: the requirements and tests of one variant, with its
	// coverage, results and verification (VariantCoverage rules)
	vc, variant, err := s.variantScope(r, p, pc)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	verif := pc.verification()
	if variant != "" {
		reqs = slices.DeleteFunc(reqs, func(rq model.Requirement) bool { return !vc.AppliesTo(rq.ID, variant) })
		tests = slices.DeleteFunc(tests, func(t model.TestCase) bool { return !vc.AppliesTo(t.ID, variant) })
		if verif, err = s.variantVerification(ctx, p, pc, vc, variant); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	linked := func(rid string) []string {
		if vc != nil {
			return vc.Tests(rid, variant)
		}
		return pc.r2t[rid]
	}
	readyTest := map[string]bool{}
	for _, t := range tests {
		readyTest[t.ID] = t.Status == model.ReadyTestStatus
	}
	rels, err := s.Store.ListRelations(ctx, p.TenantID(), pr.ID, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	conflicts := map[string][]string{}
	for _, rel := range rels {
		if rel.Type == "conflicts_with" {
			conflicts[rel.SourceID] = append(conflicts[rel.SourceID], rel.TargetKey)
			conflicts[rel.TargetID] = append(conflicts[rel.TargetID], rel.SourceKey)
		}
	}
	waived, err := s.waivedKeys(r, p, pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	in := analytics.Input{Lang: lang}
	if lang == "auto" {
		in.Lang = ""
	}
	for i := range reqs {
		rq := &reqs[i]
		pc.decorateRequirement(rq)
		if vc != nil {
			rq.Coverage, rq.LinkedTests = vc.Requirement(rq.ID, variant), len(linked(rq.ID))
		}
		ready := 0
		for _, tid := range linked(rq.ID) {
			if readyTest[tid] {
				ready++
			}
		}
		in.Requirements = append(in.Requirements, analytics.Requirement{
			ID: rq.ID, Key: rq.Key, Title: rq.Title, Description: rq.Description, Obligation: rq.Obligation, Status: rq.Status, Owner: rq.Owner,
			Criteria: len(rq.Criteria), Tests: rq.LinkedTests, Coverage: rq.Coverage, TestsReady: ready, Conflicts: conflicts[rq.ID], Suspect: rq.SuspectLinks,
			Waived: waived[rq.ID] != "",
		})
		if v := verif[rq.ID]; v != nil {
			ar := &in.Requirements[len(in.Requirements)-1]
			ar.Verification = v.State
			for _, c := range v.Criteria {
				if c.State == model.VerNone {
					ar.CriteriaNoEvidence++
				}
			}
			for _, x := range v.Inconsistencies {
				switch x.Code {
				case "child_status_unverified":
					ar.UnverifiedChildren = append(ar.UnverifiedChildren, x.Params["key"])
				case "status_inconsistent":
					ar.StatusAhead = true
				}
			}
		}
	}
	if p.Can(rbac.TestRead) {
		in.Tests = []analytics.Test{}
		for _, t := range tests {
			last, nreq := pc.last[t.ID], len(pc.t2r[t.ID])
			if vc != nil {
				last, nreq = vc.TestResult(t.ID, variant), 0
				for _, rid := range pc.t2r[t.ID] {
					if vc.AppliesTo(rid, variant) {
						nreq++
					}
				}
			}
			in.Tests = append(in.Tests, analytics.Test{ID: t.ID, Key: t.Key, Title: t.Title, Status: t.Status, LastResult: last, Requirements: nreq})
		}
	}
	writeJSON(w, http.StatusOK, analytics.Analyze(in))
}

// handleCompliance: GET /projects/{pid}/analytics/compliance?source=&lang=
// — the project's compliance with reference sources of the catalogue
// (comma-separated ids; none: every source the project uses), see
// analytics.Compliance.
func (s *Server) handleCompliance(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	c, lang, ok := s.catalogRead(w, r)
	if !ok {
		return
	}
	in, err := s.complianceInput(r, p, pr)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	in.Catalog, in.Lang = c, lang
	if v := strings.TrimSpace(r.URL.Query().Get("source")); v != "" {
		in.Sources = strings.Split(v, ",")
	}
	writeJSON(w, http.StatusOK, analytics.Compliance(in))
}

// complianceInput loads the project's requirements with the latest
// results of their linked tests.
func (s *Server) complianceInput(r *http.Request, p *Principal, pr *model.Project) (analytics.ComplianceInput, error) {
	ctx := r.Context()
	var in analytics.ComplianceInput
	pc, err := s.loadContext(ctx, p.TenantID(), pr)
	if err != nil {
		return in, err
	}
	reqs, err := s.Store.ListRequirements(ctx, p.TenantID(), pr.ID)
	if err != nil {
		return in, err
	}
	waived, err := s.waivedKeys(r, p, pr.ID)
	if err != nil {
		return in, err
	}
	// ?variant=: its requirements, with the results that count for it
	vc, variant, err := s.variantScope(r, p, pc)
	if err != nil {
		return in, err
	}
	for _, rq := range reqs {
		if vc != nil && !vc.AppliesTo(rq.ID, variant) {
			continue
		}
		x := analytics.ComplianceRequirement{ID: rq.ID, Key: rq.Key, Title: rq.Title, Status: rq.Status, Obligation: rq.Obligation,
			CatalogRef: rq.CatalogRef, Source: rq.Source, Waived: waived[rq.ID] != ""}
		if vc != nil {
			res := map[string]string{}
			for _, tid := range vc.Tests(rq.ID, variant) {
				res[tid] = vc.TestResult(tid, variant)
			}
			res = pc.stale.FilterLast(res)
			for _, tid := range vc.Tests(rq.ID, variant) {
				x.Results = append(x.Results, res[tid])
			}
		} else {
			for _, tid := range pc.r2t[rq.ID] {
				x.Results = append(x.Results, pc.evidence()[tid])
			}
		}
		in.Requirements = append(in.Requirements, x)
	}
	return in, nil
}
