package api

import (
	"net/http"
	"strings"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/soa"
)

// Statement of Applicability (package soa): the controls of a reference
// source with the project's applicability decisions, the derived
// implementation status and the requirements and evidence behind them.
// Reading needs requirement.read, changing the decisions requirement.write.
// The document itself is the standard report "soa" (reports/generate).

func (s *Server) routesSoA(h routeFunc) {
	h("GET /api/v1/projects/{pid}/soa/sources", s.handleSoASources, "requirement.read")
	h("GET /api/v1/projects/{pid}/soa", s.handleGetSoA, "requirement.read")
	h("PUT /api/v1/projects/{pid}/soa/decisions", s.handleSetSoADecisions, "requirement.write")
}

// soaSource is the source of a request (default: ISO/IEC 27001).
func soaSource(v string) string {
	if v = strings.TrimSpace(v); v != "" {
		return v
	}
	return DefaultSoASource
}

// DefaultSoASource is the source of a Statement of Applicability when none
// is chosen.
const DefaultSoASource = "iso-27001"

func (s *Server) handleSoASources(w http.ResponseWriter, r *http.Request, p *Principal) {
	if _, ok := s.project(w, r, p); !ok {
		return
	}
	c, err := s.catalogue()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := soa.Sources(c, reportLang(r, p, ""))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(list))
}

// soaStatement builds the statement of a project for a source in lang.
func (s *Server) soaStatement(r *http.Request, p *Principal, pr *model.Project, source, lang string) (*soa.Statement, error) {
	ctx := r.Context()
	c, err := s.catalogue()
	if err != nil {
		return nil, err
	}
	if c.Source(source) == nil {
		return nil, badRequest("unknown reference source %q", source)
	}
	pc, err := s.loadContext(ctx, p.TenantID(), pr)
	if err != nil {
		return nil, err
	}
	reqs, err := s.Store.ListRequirements(ctx, p.TenantID(), pr.ID)
	if err != nil {
		return nil, err
	}
	tests, err := s.Store.ListTests(ctx, p.TenantID(), pr.ID)
	if err != nil {
		return nil, err
	}
	byID := map[string]*model.TestCase{}
	for i := range tests {
		byID[tests[i].ID] = &tests[i]
	}
	waived, err := s.waivedKeys(r, p, pr.ID)
	if err != nil {
		return nil, err
	}
	decisions, err := s.Store.SoADecisions(ctx, p.TenantID(), pr.ID, source)
	if err != nil {
		return nil, err
	}
	in := soa.Input{Catalog: c, SourceID: source, Lang: lang, Decisions: decisions}
	for _, rq := range reqs {
		x := soa.Requirement{ID: rq.ID, Key: rq.Key, Title: rq.Title, Status: rq.Status, CatalogRef: rq.CatalogRef, Source: rq.Source, Waiver: waived[rq.ID]}
		for _, tid := range pc.r2t[rq.ID] {
			if t := byID[tid]; t != nil {
				x.Tests = append(x.Tests, soa.Evidence{ID: t.ID, Key: t.Key, Title: t.Title, Result: pc.last[tid]})
			}
		}
		in.Requirements = append(in.Requirements, x)
	}
	return soa.Build(in)
}

// waivedKeys returns, per requirement, the keys of the project-wide
// waivers in force for it (release waivers apply to their release only).
func (s *Server) waivedKeys(r *http.Request, p *Principal, projectID string) (map[string]string, error) {
	active, err := s.Store.ActiveWaivers(r.Context(), p.TenantID(), projectID, "")
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for id, ws := range active {
		keys := make([]string, len(ws))
		for i, w := range ws {
			keys[i] = w.Key
		}
		out[id] = strings.Join(keys, ", ")
	}
	return out, nil
}

// handleGetSoA: GET /projects/{pid}/soa?source=&lang=.
func (s *Server) handleGetSoA(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	st, err := s.soaStatement(r, p, pr, soaSource(r.URL.Query().Get("source")), reportLang(r, p, ""))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// handleSetSoADecisions: PUT /projects/{pid}/soa/decisions
// {"source", "decisions": [{"control_id", "applicable": true|false|null,
// "justification", "owner"}]}. Only the controls given change; a control
// of another source's list is refused.
func (s *Server) handleSetSoADecisions(w http.ResponseWriter, r *http.Request, p *Principal) {
	pr, ok := s.project(w, r, p)
	if !ok {
		return
	}
	var req struct {
		Source    string              `json:"source"`
		Decisions []model.SoADecision `json:"decisions"`
	}
	if err := decode(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	source := soaSource(req.Source)
	c, err := s.catalogue()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if c.Source(source) == nil {
		s.fail(w, r, badRequest("unknown reference source %q", source))
		return
	}
	reqs, err := s.Store.ListRequirements(r.Context(), p.TenantID(), pr.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var own []string
	for _, rq := range reqs {
		own = append(own, rq.Source)
	}
	controls, err := soa.Controls(c, source, "en", own...)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, d := range req.Decisions {
		if !soa.HasControl(controls, strings.TrimSpace(d.ControlID)) {
			s.fail(w, r, badRequest("%q is not a control of %s", d.ControlID, source))
			return
		}
		if d.Applicable != nil && !*d.Applicable && len([]rune(strings.TrimSpace(d.Justification))) < 3 {
			s.fail(w, r, badRequest("%s: an excluded control needs a justification", d.ControlID))
			return
		}
	}
	n, err := s.Store.SetSoADecisions(r.Context(), p.TenantID(), pr.ID, source, p.User.Email, req.Decisions)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if n > 0 {
		s.audit(r.Context(), p, "soa.update", "project", pr.ID, source+": "+itoa(n)+" controls")
	}
	st, err := s.soaStatement(r, p, pr, source, reportLang(r, p, ""))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": n, "statement": st})
}
