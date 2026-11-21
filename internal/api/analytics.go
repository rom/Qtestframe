package api

import (
	"net/http"
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
	in := analytics.Input{Lang: lang}
	if lang == "auto" {
		in.Lang = ""
	}
	for i := range reqs {
		rq := &reqs[i]
		pc.decorateRequirement(rq)
		ready := 0
		for _, tid := range pc.r2t[rq.ID] {
			if readyTest[tid] {
				ready++
			}
		}
		in.Requirements = append(in.Requirements, analytics.Requirement{
			ID: rq.ID, Key: rq.Key, Title: rq.Title, Description: rq.Description, Obligation: rq.Obligation, Status: rq.Status, Owner: rq.Owner,
			Criteria: len(rq.Criteria), Tests: rq.LinkedTests, Coverage: rq.Coverage, TestsReady: ready, Conflicts: conflicts[rq.ID], Suspect: rq.SuspectLinks,
		})
	}
	if p.Can(rbac.TestRead) {
		in.Tests = []analytics.Test{}
		for _, t := range tests {
			in.Tests = append(in.Tests, analytics.Test{ID: t.ID, Key: t.Key, Title: t.Title, Status: t.Status, LastResult: pc.last[t.ID], Requirements: len(pc.t2r[t.ID])})
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
	for _, rq := range reqs {
		x := analytics.ComplianceRequirement{ID: rq.ID, Key: rq.Key, Title: rq.Title, Status: rq.Status, Obligation: rq.Obligation,
			CatalogRef: rq.CatalogRef, Source: rq.Source}
		for _, tid := range pc.r2t[rq.ID] {
			x.Results = append(x.Results, pc.last[tid])
		}
		in.Requirements = append(in.Requirements, x)
	}
	return in, nil
}
