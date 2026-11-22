package catalog

import (
	"regexp"
	"slices"
	"sort"
	"strings"
)

// MITRE ATT&CK and ATLAS (data/attack.yaml, generated from the ATT&CK STIX
// bundles by cmd/attackgen, and data/atlas.yaml, curated by hand): the
// tactics and techniques of the Enterprise, ICS, Mobile and ATLAS
// matrices. Catalogue risks list the adversary techniques that realise
// them (attack: and atlas:), checked against the list, and refs that cite
// an ATT&CK or ATLAS id must name one of it.

// AttackMatrix is an ATT&CK matrix (enterprise, ics, mobile) or ATLAS.
type AttackMatrix struct {
	ID      string `yaml:"id" json:"id"`
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
	URL     string `yaml:"url" json:"url"`
}

// AttackTactic is a tactic (an adversary goal) of a matrix.
type AttackTactic struct {
	ID     string `yaml:"id" json:"id"`
	Name   string `yaml:"name" json:"name"`
	Matrix string `yaml:"matrix" json:"matrix"`
	URL    string `yaml:"url" json:"url"`
}

// AttackTechnique is a technique or sub-technique (T1110.004) with the
// tactics it serves.
type AttackTechnique struct {
	ID      string   `yaml:"id" json:"id"`
	Name    string   `yaml:"name" json:"name"`
	Matrix  string   `yaml:"matrix" json:"matrix"`
	Tactics []string `yaml:"tactics" json:"tactics"`
	URL     string   `yaml:"url" json:"url"`
	// Parent is the technique of a sub-technique (derived).
	Parent string `yaml:"-" json:"parent,omitempty"`
}

// AttackMitigation is a mitigation (M1032) of a matrix.
type AttackMitigation struct {
	ID     string `yaml:"id" json:"id"`
	Name   string `yaml:"name" json:"name"`
	Matrix string `yaml:"matrix" json:"matrix"`
	URL    string `yaml:"url" json:"url"`
}

type attackFile struct {
	Matrices    []AttackMatrix     `yaml:"matrices"`
	Tactics     []AttackTactic     `yaml:"tactics"`
	Techniques  []AttackTechnique  `yaml:"techniques"`
	Mitigations []AttackMitigation `yaml:"mitigations"`
}

// AtlasMatrix is the matrix id of MITRE ATLAS (techniques AML.Tnnnn).
const AtlasMatrix = "atlas"

var (
	attackTechRe = regexp.MustCompile(`^T\d{4}(\.\d{3})?$`)
	attackTactRe = regexp.MustCompile(`^TA\d{4}$`)
	attackMitRe  = regexp.MustCompile(`^M\d{4}$`)
	atlasTechRe  = regexp.MustCompile(`^AML\.T\d{4}(\.\d{3})?$`)
	atlasTactRe  = regexp.MustCompile(`^AML\.TA\d{4}$`)
	subTechRe    = regexp.MustCompile(`T\d{4}\.\d{3}$`)
	// ids cited in refs: "MITRE ATT&CK T1110.001 (…)", "MITRE ATLAS AML.T0051 (…)"
	attackRefRe = regexp.MustCompile(`^MITRE ATT&CK ((?:TA|T|M)\d{4}(?:\.\d{3})?)\b`)
	atlasRefRe  = regexp.MustCompile(`^MITRE ATLAS (AML\.TA?\d{4}(?:\.\d{3})?)\b`)
)

// NoAttackCategories are the risk categories of hazards and failures, not
// adversaries: their risks list no ATT&CK or ATLAS technique.
var NoAttackCategories = []string{"safety", "environmental", "quality", "project", "compliance"}

// AttackRequiredCategories are the risk categories whose risks list at
// least one technique (attack: or atlas:) unless attack_exempt is set.
var AttackRequiredCategories = []string{"security"}

func (c *Catalog) addAttack(f attackFile) {
	c.Matrices = append(c.Matrices, f.Matrices...)
	c.Tactics = append(c.Tactics, f.Tactics...)
	c.Techniques = append(c.Techniques, f.Techniques...)
	c.Mitigations = append(c.Mitigations, f.Mitigations...)
}

// loadAttack checks the technique list and indexes it. The matrices are
// in file order with ATLAS last (atlas.yaml sorts before attack.yaml).
func (c *Catalog) loadAttack(bad func(string, ...any)) {
	last := func(m string) int {
		if m == AtlasMatrix {
			return 1
		}
		return 0
	}
	sort.SliceStable(c.Matrices, func(i, j int) bool { return last(c.Matrices[i].ID) < last(c.Matrices[j].ID) })
	sort.SliceStable(c.Tactics, func(i, j int) bool { return last(c.Tactics[i].Matrix) < last(c.Tactics[j].Matrix) })
	sort.SliceStable(c.Techniques, func(i, j int) bool { return last(c.Techniques[i].Matrix) < last(c.Techniques[j].Matrix) })
	c.matrixByID, c.tacticByID, c.techByID, c.mitByID = map[string]*AttackMatrix{}, map[string]*AttackTactic{}, map[string]*AttackTechnique{}, map[string]*AttackMitigation{}
	for i := range c.Matrices {
		m := &c.Matrices[i]
		if !areaRe.MatchString(m.ID) || c.matrixByID[m.ID] != nil || m.Name == "" || !validURL(m.URL) {
			bad("attack matrix %q: invalid or duplicate id, or no name or url", m.ID)
		}
		c.matrixByID[m.ID] = m
	}
	idOK := func(matrix, id string, attack, atlas *regexp.Regexp) bool {
		if matrix == AtlasMatrix {
			return atlas.MatchString(id)
		}
		return attack.MatchString(id)
	}
	for i := range c.Tactics {
		t := &c.Tactics[i]
		switch {
		case c.matrixByID[t.Matrix] == nil:
			bad("attack tactic %s: unknown matrix %q", t.ID, t.Matrix)
		case !idOK(t.Matrix, t.ID, attackTactRe, atlasTactRe) || c.tacticByID[t.ID] != nil:
			bad("attack tactic %q: invalid or duplicate id", t.ID)
		case t.Name == "" || !validURL(t.URL):
			bad("attack tactic %s: name and url are needed", t.ID)
		}
		c.tacticByID[t.ID] = t
	}
	for i := range c.Techniques {
		t := &c.Techniques[i]
		switch {
		case c.matrixByID[t.Matrix] == nil:
			bad("attack technique %s: unknown matrix %q", t.ID, t.Matrix)
		case !idOK(t.Matrix, t.ID, attackTechRe, atlasTechRe) || c.techByID[t.ID] != nil:
			bad("attack technique %q: invalid or duplicate id", t.ID)
		case t.Name == "" || !validURL(t.URL):
			bad("attack technique %s: name and url are needed", t.ID)
		case len(t.Tactics) == 0:
			bad("attack technique %s: at least one tactic", t.ID)
		}
		for _, ta := range t.Tactics {
			if x := c.tacticByID[ta]; x == nil || x.Matrix != t.Matrix {
				bad("attack technique %s: tactic %s is not one of matrix %s", t.ID, ta, t.Matrix)
			}
		}
		c.techByID[t.ID] = t
	}
	for i := range c.Techniques {
		t := &c.Techniques[i]
		if subTechRe.MatchString(t.ID) {
			p := t.ID[:len(t.ID)-4]
			if c.techByID[p] == nil {
				bad("attack technique %s: no parent technique %s", t.ID, p)
			}
			t.Parent = p
		}
	}
	for i := range c.Mitigations {
		m := &c.Mitigations[i]
		// Enterprise and Mobile share some mitigations (M1013)
		if c.matrixByID[m.Matrix] == nil || !attackMitRe.MatchString(m.ID) || (c.mitByID[m.ID] != nil && c.mitByID[m.ID].Matrix == m.Matrix) || m.Name == "" || !validURL(m.URL) {
			bad("attack mitigation %q: invalid or duplicate id, unknown matrix, or no name or url", m.ID)
		}
		if c.mitByID[m.ID] == nil {
			c.mitByID[m.ID] = m
		}
	}
}

// Technique returns the technique or sub-technique with id, or nil.
func (c *Catalog) Technique(id string) *AttackTechnique { return c.techByID[id] }

// Tactic returns the tactic with id, or nil.
func (c *Catalog) Tactic(id string) *AttackTactic { return c.tacticByID[id] }

// FullName is the technique's name with its parent's for a sub-technique
// ("Brute Force: Credential Stuffing").
func (c *Catalog) FullName(t *AttackTechnique) string {
	if p := c.techByID[t.Parent]; p != nil {
		return p.Name + ": " + t.Name
	}
	return t.Name
}

// checkRiskAttack checks the attack:, atlas: and attack_exempt fields of k.
func (c *Catalog) checkRiskAttack(k *Risk, bad func(string, ...any)) {
	check := func(field string, ids []string, atlas bool) {
		seen := map[string]bool{}
		for _, id := range ids {
			t := c.techByID[id]
			switch {
			case seen[id]:
				bad("%s: technique %s listed twice in %s", k.ID, id, field)
			case t == nil:
				bad("%s: unknown technique %q in %s (see data/attack.yaml and data/atlas.yaml; never a revoked or deprecated id)", k.ID, id, field)
			case (t.Matrix == AtlasMatrix) != atlas:
				if atlas {
					bad("%s: %s is an ATT&CK technique: list it in attack", k.ID, id)
				} else {
					bad("%s: %s is an ATLAS technique: list it in atlas", k.ID, id)
				}
			}
			seen[id] = true
		}
		for _, id := range ids {
			if t := c.techByID[id]; t != nil && t.Parent != "" && seen[t.Parent] {
				bad("%s: %s lists both %s and its sub-technique %s: keep the sub-technique", k.ID, field, t.Parent, id)
			}
		}
	}
	check("attack", k.Attack, false)
	check("atlas", k.Atlas, true)
	n := len(k.Attack) + len(k.Atlas)
	switch {
	case n > 0 && slices.Contains(NoAttackCategories, k.Category):
		bad("%s: a %s risk is not caused by an adversary: it lists no attack or atlas technique (FORMAT.md, \"MITRE ATT&CK\")", k.ID, k.Category)
	case n > 0 && k.AttackExempt:
		bad("%s: attack_exempt with techniques listed", k.ID)
	case n == 0 && slices.Contains(AttackRequiredCategories, k.Category) && !k.AttackExempt:
		bad("%s: a %s risk lists at least one ATT&CK technique (attack) or ATLAS technique (atlas), or sets attack_exempt: true", k.ID, k.Category)
	}
	for _, ref := range k.Refs {
		if m := attackRefRe.FindStringSubmatch(ref); m != nil && !attackMitRe.MatchString(m[1]) {
			bad("%s: ref %q: list ATT&CK techniques and tactics of a risk in attack, not in refs", k.ID, ref)
		}
		if atlasRefRe.MatchString(ref) {
			bad("%s: ref %q: list ATLAS techniques of a risk in atlas, not in refs", k.ID, ref)
		}
	}
}

// checkAttackRefs checks that refs citing an ATT&CK or ATLAS id name one
// of the list.
func (c *Catalog) checkAttackRefs(id string, refs []string, bad func(string, ...any)) {
	for _, ref := range refs {
		var x string
		if m := attackRefRe.FindStringSubmatch(ref); m != nil {
			x = m[1]
		} else if m := atlasRefRe.FindStringSubmatch(ref); m != nil {
			x = m[1]
		} else {
			continue
		}
		if c.techByID[x] == nil && c.tacticByID[x] == nil && c.mitByID[x] == nil {
			bad("%s: ref %q cites %s, which is not in the ATT&CK/ATLAS list (revoked, deprecated or mistyped)", id, ref, x)
		}
	}
}

// attackRefIDs returns the technique ids that refs cite.
func (c *Catalog) attackRefIDs(refs []string) []string {
	var out []string
	for _, ref := range refs {
		var x string
		if m := attackRefRe.FindStringSubmatch(ref); m != nil {
			x = m[1]
		} else if m := atlasRefRe.FindStringSubmatch(ref); m != nil {
			x = m[1]
		}
		if c.techByID[x] != nil && !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}

// TechniqueSummary is a technique with its names and tactics, for display.
type TechniqueSummary struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Matrix string `json:"matrix"`
	// Tactics are the tactics it serves, with names.
	Tactics []TacticSummary `json:"tactics"`
	URL     string          `json:"url"`
}

// TacticSummary is a tactic for display.
type TacticSummary struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Matrix string `json:"matrix,omitempty"`
	URL    string `json:"url,omitempty"`
}

// SummarizeTechnique returns the technique with id for display (ok false
// when the id is unknown).
func (c *Catalog) SummarizeTechnique(id string) (TechniqueSummary, bool) {
	t := c.techByID[id]
	if t == nil {
		return TechniqueSummary{ID: id}, false
	}
	s := TechniqueSummary{ID: t.ID, Name: c.FullName(t), Matrix: t.Matrix, URL: t.URL, Tactics: []TacticSummary{}}
	for _, ta := range t.Tactics {
		if x := c.tacticByID[ta]; x != nil {
			s.Tactics = append(s.Tactics, TacticSummary{ID: x.ID, Name: x.Name, URL: x.URL})
		}
	}
	return s, true
}

// Techniques returns the techniques with ids for display (unknown ids
// with their id only).
func (c *Catalog) TechniqueSummaries(ids []string) []TechniqueSummary {
	out := make([]TechniqueSummary, 0, len(ids))
	for _, id := range ids {
		s, _ := c.SummarizeTechnique(id)
		out = append(out, s)
	}
	return out
}

// TacticsOf returns the tactic ids of the techniques ids, distinct.
func (c *Catalog) TacticsOf(ids []string) []string {
	var out []string
	for _, id := range ids {
		if t := c.techByID[id]; t != nil {
			for _, ta := range t.Tactics {
				if !slices.Contains(out, ta) {
					out = append(out, ta)
				}
			}
		}
	}
	return out
}

// techniqueMatches reports whether one of ids is one of want (lower
// case, see set) or a sub-technique of one of them.
func (c *Catalog) techniqueMatches(ids []string, want map[string]bool) bool {
	for _, id := range ids {
		if want[strings.ToLower(id)] {
			return true
		}
		if t := c.techByID[id]; t != nil && t.Parent != "" && want[strings.ToLower(t.Parent)] {
			return true
		}
	}
	return false
}

// ValidTechnique reports whether id is a known ATT&CK or ATLAS technique
// (for the techniques of project risks).
func (c *Catalog) ValidTechnique(id string) bool { return c.techByID[id] != nil }

// NormalizeTechnique returns id in canonical form (upper case, "aml." →
// "AML.").
func NormalizeTechnique(id string) string {
	return strings.ToUpper(strings.TrimSpace(id))
}

// CoverageTechnique is a technique with the catalogue items that address it.
type CoverageTechnique struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	// Risks list it (or one of its sub-techniques); Requirements cite it
	// in refs or mitigate such a risk.
	Risks        int `json:"risks"`
	Requirements int `json:"requirements"`
}

// CoverageTactic is a tactic with the counts of the risks and
// requirements that address its techniques.
type CoverageTactic struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	URL          string              `json:"url"`
	Risks        int                 `json:"risks"`
	Requirements int                 `json:"requirements"`
	Techniques   []CoverageTechnique `json:"techniques"`
}

// CoverageMatrix is a matrix with its tactics in order.
type CoverageMatrix struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	Version string           `json:"version"`
	URL     string           `json:"url"`
	Risks   int              `json:"risks"`
	Tactics []CoverageTactic `json:"tactics"`
}

// AttackCoverage returns, per matrix and tactic, how many catalogue risks
// (in the catalogue types of catalogue, "" = all) list a technique of the
// tactic and how many requirements cite one or mitigate such a risk, with
// the techniques used. Techniques no item addresses are left out.
func (c *Catalog) AttackCoverage(catalogue string) []CoverageMatrix {
	kinds := set(catalogue)
	// technique id (parents included) → risk ids / requirement ids
	techRisks, techReqs := map[string]map[string]bool{}, map[string]map[string]bool{}
	add := func(m map[string]map[string]bool, tech, item string) {
		for t := c.techByID[tech]; t != nil; t = c.techByID[t.Parent] {
			if m[t.ID] == nil {
				m[t.ID] = map[string]bool{}
			}
			m[t.ID][item] = true
			if t.Parent == "" {
				break
			}
		}
	}
	for _, a := range c.Areas {
		if !accepts(kinds, a.Catalogue) {
			continue
		}
		for _, k := range a.Risks {
			for _, id := range append(append([]string{}, k.Attack...), k.Atlas...) {
				add(techRisks, id, k.ID)
				for _, rid := range k.MitigatedBy {
					add(techReqs, id, rid)
				}
			}
		}
		for _, r := range a.Requirements {
			for _, id := range c.attackRefIDs(r.Refs) {
				add(techReqs, id, r.ID)
			}
		}
	}
	var out []CoverageMatrix
	for _, m := range c.Matrices {
		cm := CoverageMatrix{ID: m.ID, Name: m.Name, Version: m.Version, URL: m.URL, Tactics: []CoverageTactic{}}
		mRisks := map[string]bool{}
		for _, ta := range c.Tactics {
			if ta.Matrix != m.ID {
				continue
			}
			ct := CoverageTactic{ID: ta.ID, Name: ta.Name, URL: ta.URL, Techniques: []CoverageTechnique{}}
			risks, reqs := map[string]bool{}, map[string]bool{}
			for i := range c.Techniques {
				t := &c.Techniques[i]
				if !slices.Contains(t.Tactics, ta.ID) || len(techRisks[t.ID])+len(techReqs[t.ID]) == 0 {
					continue
				}
				if t.Parent == "" {
					ct.Techniques = append(ct.Techniques, CoverageTechnique{ID: t.ID, Name: c.FullName(t), URL: t.URL, Risks: len(techRisks[t.ID]), Requirements: len(techReqs[t.ID])})
				}
				for x := range techRisks[t.ID] {
					risks[x], mRisks[x] = true, true
				}
				for x := range techReqs[t.ID] {
					reqs[x] = true
				}
			}
			ct.Risks, ct.Requirements = len(risks), len(reqs)
			cm.Tactics = append(cm.Tactics, ct)
		}
		cm.Risks = len(mRisks)
		out = append(out, cm)
	}
	return out
}
