package api

import (
	"net/http"
	"strings"

	"github.com/rom/qtestframe/internal/catalog"
)

// MITRE ATT&CK and ATLAS in the catalogue (catalog/attack.go): the
// technique list and the coverage of tactics by catalogue risks and
// requirements.

// handleCatalogAttack: GET /catalog/attack?catalogue= — per matrix and
// tactic, the catalogue risks that list one of its techniques and the
// requirements that cite one or mitigate such a risk, with the techniques
// used.
func (s *Server) handleCatalogAttack(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, err := s.catalogue()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"matrices": c.AttackCoverage(r.URL.Query().Get("catalogue"))})
}

// catalogTactic is a tactic of the technique list.
type catalogTactic struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Matrix string `json:"matrix"`
	URL    string `json:"url"`
}

// handleCatalogTechniques: GET /catalog/attack/techniques?q=&matrix=&tactic=
// — the ATT&CK and ATLAS techniques (the valid values of a risk's attack
// list) with their names, tactics and links; q matches the id or the name.
func (s *Server) handleCatalogTechniques(w http.ResponseWriter, r *http.Request, p *Principal) {
	c, err := s.catalogue()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	qv := r.URL.Query()
	q, matrices, tactics := strings.ToLower(strings.TrimSpace(qv.Get("q"))), splitSet(qv.Get("matrix")), splitSet(strings.ToUpper(qv.Get("tactic")))
	out := []catalog.TechniqueSummary{}
	for _, t := range c.Techniques {
		if matrices != nil && !matrices[t.Matrix] {
			continue
		}
		ts, _ := c.SummarizeTechnique(t.ID)
		if q != "" && !strings.Contains(strings.ToLower(ts.ID), q) && !strings.Contains(strings.ToLower(ts.Name), q) {
			continue
		}
		if tactics != nil && !anyTactic(ts, tactics) {
			continue
		}
		out = append(out, ts)
	}
	tl := []catalogTactic{}
	for _, t := range c.Tactics {
		if matrices == nil || matrices[t.Matrix] {
			tl = append(tl, catalogTactic{ID: t.ID, Name: t.Name, Matrix: t.Matrix, URL: t.URL})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"matrices": c.Matrices, "tactics": tl, "items": out})
}

func anyTactic(t catalog.TechniqueSummary, want map[string]bool) bool {
	for _, ta := range t.Tactics {
		if want[ta.ID] {
			return true
		}
	}
	return false
}

// splitSet is the set of comma-separated values of v (nil when empty).
func splitSet(v string) map[string]bool {
	var out map[string]bool
	for _, x := range strings.Split(v, ",") {
		if x = strings.TrimSpace(x); x != "" {
			if out == nil {
				out = map[string]bool{}
			}
			out[x] = true
		}
	}
	return out
}
