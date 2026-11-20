package api

import (
	"net/http"

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
			Criteria: len(rq.Criteria), Tests: rq.LinkedTests, Coverage: rq.Coverage, TestsReady: ready, Conflicts: conflicts[rq.ID],
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
