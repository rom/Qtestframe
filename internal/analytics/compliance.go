package analytics

import (
	"slices"
	"sort"
	"strings"

	"github.com/rom/qtestframe/internal/catalog"
)

// Compliance: how far a project fulfils selected laws, regulations,
// standards and best practices (the reference sources of the catalogue,
// internal/catalog/data/sources.yaml).
//
// For each source the catalogue requirements that cite it are the
// yardstick. A catalogue requirement is adopted when a project
// requirement was imported from it (catalog_ref) and is not rejected or
// obsolete; the others are gaps. Project requirements that were not
// imported from such an item but name the source in their source field
// ("GDPR Art. 32; …") count as the project's own requirements for it.
// Every counted requirement is fulfilled when it has tests and the latest
// result of each is passed; otherwise it is failing (a test failed),
// blocked (a test is blocked, none failed) or untested. The compliance
// percentage is the fulfilled share of all requirements for the source
// (the catalogue's plus the project's own), in total and per clause or
// article group (catalog.Source.ClauseOf).

// Fulfilment states of a requirement.
const (
	StatePassed   = "passed"
	StateFailing  = "failing"
	StateBlocked  = "blocked"
	StateUntested = "untested"
)

// Obligation groups of the compliance split.
var ObligationGroups = []string{"must", "should", "may", "other"}

// obligationGroup maps an obligation level to must, should, may or other.
func obligationGroup(o string) string {
	switch o {
	case "must", "must_not", "shall", "shall_not":
		return "must"
	case "should", "should_not":
		return "should"
	case "may", "optional":
		return "may"
	}
	return "other"
}

// ComplianceRequirement is a project requirement as the compliance
// analysis sees it.
type ComplianceRequirement struct {
	ID, Key, Title, Status, Obligation string
	// CatalogRef: the catalogue id it was imported from ("" none).
	CatalogRef string
	// Source: the free-text source field.
	Source string
	// Results: the latest result of each linked test ("" = not run).
	Results []string
}

// ComplianceInput is what Compliance analyses.
type ComplianceInput struct {
	Catalog *catalog.Catalog
	// Sources are the source ids to analyse; empty: every source the
	// project has adopted requirements of or cites itself.
	Sources      []string
	Requirements []ComplianceRequirement
	// Lang of the catalogue titles and source names.
	Lang string
}

// Fulfilment counts requirements by fulfilment state.
type Fulfilment struct {
	Passed   int `json:"passed"`
	Failing  int `json:"failing"`
	Blocked  int `json:"blocked"`
	Untested int `json:"untested"`
}

func (f *Fulfilment) add(state string) {
	switch state {
	case StatePassed:
		f.Passed++
	case StateFailing:
		f.Failing++
	case StateBlocked:
		f.Blocked++
	default:
		f.Untested++
	}
}

// ObligationCompliance is the adoption of one obligation group.
type ObligationCompliance struct {
	Obligation string `json:"obligation"` // must | should | may | other
	Catalogue  int    `json:"catalogue"`
	Adopted    int    `json:"adopted"`
	Missing    int    `json:"missing"`
	Own        int    `json:"own"`
	Fulfilled  int    `json:"fulfilled"`
	Percent    int    `json:"percent"`
}

// ClauseCompliance is the compliance of one clause or article group.
type ClauseCompliance struct {
	// Clause such as "Art. 32", "A.8", "Annex I Part I"; "" = the source
	// as a whole (refs that name no clause).
	Clause     string     `json:"clause"`
	Catalogue  int        `json:"catalogue"`
	Adopted    int        `json:"adopted"`
	Missing    int        `json:"missing"`
	Own        int        `json:"own"`
	Fulfilment Fulfilment `json:"fulfilment"`
	Percent    int        `json:"percent"`
}

// ComplianceGap is a catalogue requirement the project has not adopted.
type ComplianceGap struct {
	CatalogID  string   `json:"catalog_id"`
	Area       string   `json:"area"`
	Title      string   `json:"title"`
	Obligation string   `json:"obligation"`
	Priority   string   `json:"priority"`
	Clauses    []string `json:"clauses"`
	Refs       []string `json:"refs"`
	Tests      int      `json:"tests"`
	// Inactive lists the project copies that do not count (rejected or
	// obsolete).
	Inactive []Item `json:"inactive,omitempty"`
}

// ComplianceItem is a project requirement counted for a source.
type ComplianceItem struct {
	ID         string   `json:"id"`
	Key        string   `json:"key"`
	Title      string   `json:"title"`
	Obligation string   `json:"obligation"`
	State      string   `json:"state"`
	CatalogRef string   `json:"catalog_ref,omitempty"`
	Own        bool     `json:"own"`
	Clauses    []string `json:"clauses"`
}

// SourceCompliance is the compliance with one source.
type SourceCompliance struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Jurisdiction string `json:"jurisdiction,omitempty"`
	Short        string `json:"short,omitempty"`
	Name         string `json:"name"`
	URL          string `json:"url,omitempty"`
	// Catalogue: catalogue requirements citing the source; Adopted of
	// them are in the project; Missing are not; Own: project requirements
	// citing it that do not come from those.
	Catalogue int `json:"catalogue"`
	Adopted   int `json:"adopted"`
	Missing   int `json:"missing"`
	Own       int `json:"own"`
	// Fulfilment of the adopted catalogue requirements and own requirements.
	Fulfilment Fulfilment `json:"fulfilment"`
	// AdoptionPercent: adopted of the catalogue requirements.
	AdoptionPercent int `json:"adoption_percent"`
	// Percent: fulfilled of all (catalogue + own) requirements;
	// MandatoryPercent: the same for MUST-level requirements (-1 none).
	Percent          int                    `json:"percent"`
	MandatoryPercent int                    `json:"mandatory_percent"`
	Obligations      []ObligationCompliance `json:"obligations"`
	Clauses          []ClauseCompliance     `json:"clauses"`
	// Gaps (missing catalogue requirements) and Items (counted project
	// requirements, not fulfilled first), at most MaxItems each.
	Gaps       []ComplianceGap  `json:"gaps"`
	Items      []ComplianceItem `json:"items"`
	GapsTotal  int              `json:"gaps_total"`
	ItemsTotal int              `json:"items_total"`
}

// ComplianceSummary sums up the analysed sources (a requirement counted
// for several sources counts once).
type ComplianceSummary struct {
	Sources    int        `json:"sources"`
	Catalogue  int        `json:"catalogue"`
	Adopted    int        `json:"adopted"`
	Missing    int        `json:"missing"`
	Own        int        `json:"own"`
	Fulfilment Fulfilment `json:"fulfilment"`
	Percent    int        `json:"percent"`
}

// ComplianceReport is the result of Compliance.
type ComplianceReport struct {
	Summary ComplianceSummary  `json:"summary"`
	Sources []SourceCompliance `json:"sources"`
	// Unknown are requested source ids that are not in the registry.
	Unknown []string `json:"unknown,omitempty"`
}

// StateOf returns the fulfilment state of a requirement with the latest
// results of its linked tests.
func StateOf(results []string) string {
	if len(results) == 0 {
		return StateUntested
	}
	failed, blocked, passed := false, false, 0
	for _, r := range results {
		switch r {
		case "passed":
			passed++
		case "failed":
			failed = true
		case "blocked":
			blocked = true
		}
	}
	switch {
	case failed:
		return StateFailing
	case blocked:
		return StateBlocked
	case passed == len(results):
		return StatePassed
	}
	return StateUntested
}

// stateRank orders states from worst to best.
var stateRank = map[string]int{StateFailing: 0, StateBlocked: 1, StateUntested: 2, StatePassed: 3}

func percent(a, b int) int {
	if b == 0 {
		return 0
	}
	return int(float64(a)*100/float64(b) + 0.5)
}

// Compliance analyses the project's compliance with the sources of in.
func Compliance(in ComplianceInput) ComplianceReport {
	c := in.Catalog
	lang := catalog.Lang(in.Lang)
	out := ComplianceReport{Sources: []SourceCompliance{}}
	if c == nil {
		return out
	}

	// project requirements by catalogue id (active ones) and their own sources
	byRef, inactiveByRef := map[string][]*ComplianceRequirement{}, map[string][]*ComplianceRequirement{}
	ownSources := map[string][]string{} // requirement id → source ids named in its source field
	for i := range in.Requirements {
		r := &in.Requirements[i]
		if r.CatalogRef != "" {
			if inactive(r.Status) {
				inactiveByRef[r.CatalogRef] = append(inactiveByRef[r.CatalogRef], r)
			} else {
				byRef[r.CatalogRef] = append(byRef[r.CatalogRef], r)
			}
		}
		if !inactive(r.Status) && strings.TrimSpace(r.Source) != "" {
			ownSources[r.ID] = c.SourcesOfText(r.Source)
		}
	}

	// the sources to analyse
	var ids []string
	if len(in.Sources) > 0 {
		for _, id := range in.Sources {
			id = strings.TrimSpace(id)
			if id == "" || slices.Contains(ids, id) {
				continue
			}
			if c.Source(id) == nil {
				out.Unknown = append(out.Unknown, id)
				continue
			}
			ids = append(ids, id)
		}
	} else {
		used := map[string]bool{}
		for ref := range byRef {
			if rq := c.Requirements[ref]; rq != nil {
				for _, s := range rq.Sources {
					used[s] = true
				}
			}
		}
		for _, ss := range ownSources {
			for _, s := range ss {
				used[s] = true
			}
		}
		for _, s := range c.Sources {
			if used[s.ID] {
				ids = append(ids, s.ID)
			}
		}
		rank := map[string]int{}
		for i, k := range c.SourceKinds {
			rank[k.ID] = i
		}
		sort.SliceStable(ids, func(i, j int) bool { return rank[c.Source(ids[i]).Kind] < rank[c.Source(ids[j]).Kind] })
	}

	// summary sets
	sumCat, sumAdopted, sumOwn := map[string]bool{}, map[string]bool{}, map[string]bool{}
	sumState := map[string]string{} // catalogue id or "own:"+requirement id → state

	for _, id := range ids {
		src := c.Source(id)
		sc := SourceCompliance{ID: src.ID, Kind: src.Kind, Jurisdiction: src.Jurisdiction, Short: src.Short, Name: src.Name.Get(lang), URL: src.URL,
			Gaps: []ComplianceGap{}, Items: []ComplianceItem{}, Clauses: []ClauseCompliance{}, Obligations: []ObligationCompliance{}}
		clauses := map[string]*ClauseCompliance{}
		clause := func(k string) *ClauseCompliance {
			if clauses[k] == nil {
				clauses[k] = &ClauseCompliance{Clause: k}
			}
			return clauses[k]
		}
		obl := map[string]*ObligationCompliance{}
		for _, g := range ObligationGroups {
			obl[g] = &ObligationCompliance{Obligation: g}
		}
		var items []ComplianceItem
		mandatory, mandatoryOK := 0, 0
		fromCatalogue := map[string]bool{} // project requirement ids counted as adopted for this source
		for _, a := range c.OrderedAreas() {
			for i := range a.Requirements {
				rq := &a.Requirements[i]
				if !slices.Contains(rq.Sources, id) {
					continue
				}
				var cls []string
				for _, ref := range rq.Refs {
					if c.RefSource(ref) == id {
						if k := src.ClauseOf(ref); !slices.Contains(cls, k) {
							cls = append(cls, k)
						}
					}
				}
				g := obligationGroup(rq.Obligation)
				sc.Catalogue++
				obl[g].Catalogue++
				sumCat[rq.ID] = true
				for _, k := range cls {
					clause(k).Catalogue++
				}
				copies := byRef[rq.ID]
				if len(copies) == 0 {
					sc.Missing++
					obl[g].Missing++
					for _, k := range cls {
						clause(k).Missing++
					}
					gap := ComplianceGap{CatalogID: rq.ID, Area: rq.Area, Title: rq.Localized(lang).Title, Obligation: rq.Obligation, Priority: rq.Priority,
						Clauses: cls, Refs: rq.Refs, Tests: len(rq.Tests)}
					for _, x := range inactiveByRef[rq.ID] {
						gap.Inactive = append(gap.Inactive, Item{Kind: "requirement", ID: x.ID, Key: x.Key, Title: x.Title, Note: x.Status})
					}
					sc.Gaps = append(sc.Gaps, gap)
					if g == "must" {
						mandatory++
					}
					continue
				}
				// adopted: the worst state of its copies counts
				state := StatePassed
				for _, x := range copies {
					st := StateOf(x.Results)
					if stateRank[st] < stateRank[state] {
						state = st
					}
					fromCatalogue[x.ID] = true
					items = append(items, ComplianceItem{ID: x.ID, Key: x.Key, Title: x.Title, Obligation: rq.Obligation, State: st, CatalogRef: rq.ID, Clauses: cls})
				}
				sc.Adopted++
				obl[g].Adopted++
				sc.Fulfilment.add(state)
				sumAdopted[rq.ID] = true
				sumState[rq.ID] = state
				if state == StatePassed {
					obl[g].Fulfilled++
				}
				if g == "must" {
					mandatory++
					if state == StatePassed {
						mandatoryOK++
					}
				}
				for _, k := range cls {
					cl := clause(k)
					cl.Adopted++
					cl.Fulfilment.add(state)
				}
			}
		}
		// the project's own requirements naming the source
		for i := range in.Requirements {
			r := &in.Requirements[i]
			if fromCatalogue[r.ID] || !slices.Contains(ownSources[r.ID], id) {
				continue
			}
			var cls []string
			for _, part := range strings.FieldsFunc(r.Source, func(x rune) bool { return x == ';' || x == '\n' }) {
				part = strings.TrimSpace(part)
				if slices.Contains(c.SourcesOfText(part), id) {
					if k := src.ClauseOf(part); !slices.Contains(cls, k) {
						cls = append(cls, k)
					}
				}
			}
			st := StateOf(r.Results)
			g := obligationGroup(r.Obligation)
			sc.Own++
			obl[g].Own++
			sc.Fulfilment.add(st)
			sumOwn[r.ID] = true
			sumState["own:"+r.ID] = st
			if st == StatePassed {
				obl[g].Fulfilled++
			}
			if g == "must" {
				mandatory++
				if st == StatePassed {
					mandatoryOK++
				}
			}
			for _, k := range cls {
				cl := clause(k)
				cl.Own++
				cl.Fulfilment.add(st)
			}
			items = append(items, ComplianceItem{ID: r.ID, Key: r.Key, Title: r.Title, Obligation: r.Obligation, State: st, Own: true, Clauses: cls})
		}
		sc.AdoptionPercent = percent(sc.Adopted, sc.Catalogue)
		sc.Percent = percent(sc.Fulfilment.Passed, sc.Catalogue+sc.Own)
		sc.MandatoryPercent = -1
		if mandatory > 0 {
			sc.MandatoryPercent = percent(mandatoryOK, mandatory)
		}
		for _, g := range ObligationGroups {
			o := obl[g]
			if o.Catalogue+o.Own == 0 {
				continue
			}
			o.Percent = percent(o.Fulfilled, o.Catalogue+o.Own)
			sc.Obligations = append(sc.Obligations, *o)
		}
		for _, cl := range clauses {
			cl.Percent = percent(cl.Fulfilment.Passed, cl.Catalogue+cl.Own)
			sc.Clauses = append(sc.Clauses, *cl)
		}
		sort.Slice(sc.Clauses, func(i, j int) bool { return clauseLess(sc.Clauses[i].Clause, sc.Clauses[j].Clause) })
		// gaps: MUST first, then priority, then catalogue order (stable)
		sort.SliceStable(sc.Gaps, func(i, j int) bool {
			gi, gj := slices.Index(ObligationGroups, obligationGroup(sc.Gaps[i].Obligation)), slices.Index(ObligationGroups, obligationGroup(sc.Gaps[j].Obligation))
			if gi != gj {
				return gi < gj
			}
			return priorityRank(sc.Gaps[i].Priority) < priorityRank(sc.Gaps[j].Priority)
		})
		sort.SliceStable(items, func(i, j int) bool { return stateRank[items[i].State] < stateRank[items[j].State] })
		sc.GapsTotal, sc.ItemsTotal = len(sc.Gaps), len(items)
		if len(sc.Gaps) > MaxItems {
			sc.Gaps = sc.Gaps[:MaxItems]
		}
		if len(items) > MaxItems {
			items = items[:MaxItems]
		}
		sc.Items = append(sc.Items, items...)
		out.Sources = append(out.Sources, sc)
	}

	s := &out.Summary
	s.Sources = len(out.Sources)
	s.Catalogue, s.Adopted, s.Own = len(sumCat), len(sumAdopted), len(sumOwn)
	s.Missing = s.Catalogue - s.Adopted
	for _, st := range sumState {
		s.Fulfilment.add(st)
	}
	s.Percent = percent(s.Fulfilment.Passed, s.Catalogue+s.Own)
	return out
}

func priorityRank(p string) int {
	switch p {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	}
	return 4
}

// clauseLess orders clauses naturally ("Art. 5" < "Art. 32"); the
// clause-less group comes last.
func clauseLess(a, b string) bool {
	if a == "" || b == "" {
		return a != "" && b == ""
	}
	return naturalCompare(a, b) < 0
}

// naturalCompare compares with digit runs compared by value.
func naturalCompare(a, b string) int {
	isDigit := func(c byte) bool { return c >= '0' && c <= '9' }
	for a != "" && b != "" {
		if isDigit(a[0]) && isDigit(b[0]) {
			i, j := 0, 0
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			na, nb := strings.TrimLeft(a[:i], "0"), strings.TrimLeft(b[:j], "0")
			if len(na) != len(nb) {
				if len(na) < len(nb) {
					return -1
				}
				return 1
			}
			if c := strings.Compare(na, nb); c != 0 {
				return c
			}
			a, b = a[i:], b[j:]
			continue
		}
		if a[0] != b[0] {
			if a[0] < b[0] {
				return -1
			}
			return 1
		}
		a, b = a[1:], b[1:]
	}
	return len(a) - len(b)
}
