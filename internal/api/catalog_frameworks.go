package api

import (
	"net/http"
	"slices"
	"strings"

	"github.com/rom/qtestframe/internal/catalog"
)

// MITRE D3FEND and EMB3D (catalog/frameworks.go): the lists for the
// pickers, the coverage of both frameworks by the catalogue, the checks of
// the d3fend and emb3d lists of project requirements, tests and risks, and
// their query values.

// handleCatalogD3fend: GET /catalog/d3fend?q=&tactic= — the D3FEND tactics
// and defensive techniques (the valid values of a requirement's or test's
// d3fend list) with their names, parents and links; q matches the id or
// the name.
func (s *Server) handleCatalogD3fend(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, err := s.catalogue()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	q, tactics := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q"))), splitSet(strings.ToLower(r.URL.Query().Get("tactic")))
	out := []catalog.D3fendSummary{}
	for _, t := range c.D3fend.Techniques {
		if tactics != nil && !tactics[strings.ToLower(t.Tactic)] {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(t.ID), q) && !strings.Contains(strings.ToLower(t.Name), q) {
			continue
		}
		out = append(out, c.SummarizeD3fend(t.ID))
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": c.D3fend.Version, "release": c.D3fend.Release, "url": c.D3fend.URL,
		"tactics": c.D3fend.Tactics, "items": out})
}

// handleCatalogEmb3d: GET /catalog/emb3d?q=&kind=threat|mitigation&category=
// — the EMB3D threats and mitigations (the valid values of an emb3d list;
// a risk takes threats only) and the device properties.
func (s *Server) handleCatalogEmb3d(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, err := s.catalogue()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	qv := r.URL.Query()
	q, kinds, cats := strings.ToLower(strings.TrimSpace(qv.Get("q"))), splitSet(qv.Get("kind")), splitSet(strings.ToLower(qv.Get("category")))
	match := func(id, name string) bool {
		return q == "" || strings.Contains(strings.ToLower(id), q) || strings.Contains(strings.ToLower(name), q)
	}
	out := []catalog.Emb3dSummary{}
	if kinds == nil || kinds["threat"] {
		for _, t := range c.Emb3d.Threats {
			if (cats == nil || cats[t.Category]) && match(t.ID, t.Name) {
				out = append(out, c.SummarizeEmb3d(t.ID))
			}
		}
	}
	if (kinds == nil || kinds["mitigation"]) && cats == nil {
		for _, m := range c.Emb3d.Mitigations {
			if match(m.ID, m.Name) {
				out = append(out, c.SummarizeEmb3d(m.ID))
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": c.Emb3d.Version, "url": c.Emb3d.URL, "categories": catalog.Emb3dCategories,
		"properties": c.Emb3d.Properties, "items": out})
}

// handleCatalogD3fendCoverage: GET /catalog/d3fend/coverage?catalogue= —
// per D3FEND tactic, the techniques the catalogue requirements and tests
// list, and how many catalogue risks have every ATT&CK technique
// countered by a mitigating requirement.
func (s *Server) handleCatalogD3fendCoverage(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, err := s.catalogue()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c.D3fendCoverage(r.URL.Query().Get("catalogue")))
}

// handleCatalogEmb3dCoverage: GET /catalog/emb3d/coverage?catalogue= — per
// EMB3D threat category, every threat with the catalogue risks,
// requirements and tests that address it.
func (s *Server) handleCatalogEmb3dCoverage(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, err := s.catalogue()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c.Emb3dCoverage(r.URL.Query().Get("catalogue")))
}

// checkFrameworkIDs normalises a d3fend and an emb3d list and checks them
// against the catalogue's lists (nil d3: the item takes no D3FEND list;
// threatsOnly: emb3d takes EMB3D threats only, as for a risk).
func (s *Server) checkFrameworkIDs(d3, emb *[]string, threatsOnly bool) error {
	norm := func(l *[]string) {
		out := []string{}
		for _, id := range *l {
			if id = catalog.NormalizeFrameworkID(id); id != "" && !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
		*l = out
	}
	if d3 != nil {
		norm(d3)
	}
	norm(emb)
	if (d3 == nil || len(*d3) == 0) && len(*emb) == 0 {
		return nil
	}
	c, err := s.catalogue()
	if err != nil {
		return err
	}
	var bad []string
	if d3 != nil {
		for _, id := range *d3 {
			if !c.ValidD3fend(id) {
				bad = append(bad, id)
			}
		}
		if len(bad) > 0 {
			return badRequest("unknown D3FEND technique %s (see GET /catalog/d3fend)", strings.Join(bad, ", "))
		}
	}
	for _, id := range *emb {
		if !c.ValidEmb3d(id, threatsOnly) {
			bad = append(bad, id)
		}
	}
	if len(bad) > 0 {
		if threatsOnly {
			return badRequest("unknown EMB3D threat %s: a risk lists threats TID-nnn (see GET /catalog/emb3d?kind=threat)", strings.Join(bad, ", "))
		}
		return badRequest("unknown EMB3D threat or mitigation %s (see GET /catalog/emb3d)", strings.Join(bad, ", "))
	}
	return nil
}

// d3fendValues are the query values of a d3fend list: the ids, the names
// and the tactics, and those of the parent techniques, so that
// d3fend:D3-AA finds D3-MFA and d3fend:harden every Harden technique.
func d3fendValues(ids []string) []string {
	out := append([]string{}, ids...)
	c, err := catalog.Default()
	if err != nil {
		return out
	}
	for _, id := range ids {
		for t := c.D3fendTechnique(id); t != nil; t = c.D3fendTechnique(t.Parent) {
			out = append(out, t.ID, t.Name, t.Tactic)
			if t.Parent == "" {
				break
			}
		}
	}
	return out
}

// emb3dValues are the query values of an emb3d list: the ids and names,
// the threats a listed mitigation addresses and the threat categories, so
// that emb3d:TID-201 finds a requirement that lists MID-002.
func emb3dValues(ids []string) []string {
	out := append([]string{}, ids...)
	c, err := catalog.Default()
	if err != nil {
		return out
	}
	for _, id := range ids {
		out = append(out, c.SummarizeEmb3d(id).Name)
	}
	for _, tid := range c.Emb3dThreatsOf(ids) {
		if t := c.Emb3dThreat(tid); t != nil {
			out = append(out, t.ID, t.Name, t.Category)
		}
	}
	return out
}
