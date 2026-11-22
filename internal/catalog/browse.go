package catalog

import (
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/rom/qtestframe/internal/model"
)

// Browsing: localized summaries, filters and a ranked free-text search over
// the loaded catalogue. Everything works on the in-memory catalogue, so a
// search over several hundred items takes well under a millisecond.

// Lang returns lang when the catalogue is written in it, else "en".
func Lang(lang string) string {
	if slices.Contains(Languages, lang) {
		return lang
	}
	return "en"
}

// Localized returns the requirement text in lang, falling back to English
// per field.
func (r *Requirement) Localized(lang string) ReqText {
	x, en := r.Text[lang], r.Text["en"]
	if x.Title == "" {
		x.Title = en.Title
	}
	if x.Description == "" {
		x.Description = en.Description
	}
	if x.Rationale == "" {
		x.Rationale = en.Rationale
	}
	if len(x.Criteria) == 0 {
		x.Criteria = en.Criteria
	} else if len(x.Criteria) == len(en.Criteria) {
		// a threshold is often written in the English text only
		x.Criteria = append([]Criterion(nil), x.Criteria...)
		for i := range x.Criteria {
			if x.Criteria[i].Threshold == "" {
				x.Criteria[i].Threshold = en.Criteria[i].Threshold
			}
		}
	}
	return x
}

// Localized returns the test text in lang, falling back to English per
// field.
func (t *Test) Localized(lang string) TestText {
	x, en := t.Text[lang], t.Text["en"]
	if x.Title == "" {
		x.Title = en.Title
	}
	if x.Description == "" {
		x.Description = en.Description
	}
	if x.Preconditions == "" {
		x.Preconditions = en.Preconditions
	}
	if x.Expected == "" {
		x.Expected = en.Expected
	}
	if len(x.Steps) == 0 {
		x.Steps = en.Steps
	}
	return x
}

// Localized returns the risk text in lang, falling back to English per
// field.
func (k *Risk) Localized(lang string) RiskText {
	x, en := k.Text[lang], k.Text["en"]
	if x.Title == "" {
		x.Title = en.Title
	}
	if x.Description == "" {
		x.Description = en.Description
	}
	if x.Mitigation == "" {
		x.Mitigation = en.Mitigation
	}
	return x
}

// Level is the risk level of the typical likelihood and impact, as for a
// project risk (low, medium, high, critical).
func (k *Risk) Level() string { return model.RiskLevel(k.Likelihood, k.Impact) }

// Area returns the area with id, or nil.
func (c *Catalog) Area(id string) *Area {
	for i := range c.Areas {
		if c.Areas[i].ID == id {
			return &c.Areas[i]
		}
	}
	return nil
}

// Group returns the group with id, or nil.
func (c *Catalog) Group(id string) *Group {
	for i := range c.Groups {
		if c.Groups[i].ID == id {
			return &c.Groups[i]
		}
	}
	return nil
}

// OrderedAreas returns the areas in display order: by the order of their
// group in groups.yaml, then by file name (the load order).
func (c *Catalog) OrderedAreas() []*Area {
	rank := map[string]int{}
	for i, g := range c.Groups {
		rank[g.ID] = i
	}
	out := make([]*Area, len(c.Areas))
	for i := range c.Areas {
		out[i] = &c.Areas[i]
	}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Group] < rank[out[j].Group] })
	return out
}

// Filter selects catalogue items. Every field is a comma-separated list of
// accepted values ("" = any); Q is free text whose words must all occur.
//
// Source selects items by the reference sources they cite (sources.yaml):
// a requirement whose refs cite one of them, a test that cites one or
// verifies such a requirement, a risk that cites one or is mitigated by
// such a requirement. Category, Level, Attack and Tactic apply to risks
// only: Attack selects the risks that list one of the ATT&CK or ATLAS
// techniques (or a sub-technique of one), Tactic those with a technique
// of one of the tactics (TA0006, AML.TA0005).
type Filter struct {
	Area, Group, Type, Obligation, Priority, Tag, Execution string
	Catalogue                                               string
	Source                                                  string
	Category, Level                                         string
	Attack, Tactic                                          string
	Q                                                       string
}

// ReqSummary is a requirement in one language without the long texts.
type ReqSummary struct {
	ID         string   `json:"id"`
	Area       string   `json:"area"`
	Group      string   `json:"group"`
	Catalogue  string   `json:"catalogue"`
	Type       string   `json:"type"`
	Obligation string   `json:"obligation"`
	Priority   string   `json:"priority"`
	Tags       []string `json:"tags"`
	Refs       []string `json:"refs"`
	Sources    []string `json:"sources"`
	Tests      []string `json:"tests"`
	// Risks are the catalogue risks it mitigates.
	Risks       []string `json:"risks"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
}

// RiskSummary is a risk in one language.
type RiskSummary struct {
	ID           string   `json:"id"`
	Area         string   `json:"area"`
	Group        string   `json:"group"`
	Catalogue    string   `json:"catalogue"`
	Category     string   `json:"category"`
	CategoryName string   `json:"category_name"`
	Likelihood   int      `json:"likelihood"`
	Impact       int      `json:"impact"`
	Score        int      `json:"score"`
	Level        string   `json:"level"`
	Tags         []string `json:"tags"`
	Refs         []string `json:"refs"`
	Sources      []string `json:"sources"`
	MitigatedBy  []string `json:"mitigated_by"`
	Tests        []string `json:"tests"`
	// Attack and Atlas are the ATT&CK and ATLAS technique ids, Tactics the
	// tactic ids they serve.
	Attack      []string `json:"attack"`
	Atlas       []string `json:"atlas"`
	Tactics     []string `json:"tactics"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Mitigation  string   `json:"mitigation"`
}

// TestSummary is a test in one language without the steps.
type TestSummary struct {
	ID           string   `json:"id"`
	Area         string   `json:"area"`
	Group        string   `json:"group"`
	Catalogue    string   `json:"catalogue"`
	Priority     string   `json:"priority"`
	Execution    string   `json:"execution"`
	Tags         []string `json:"tags"`
	Refs         []string `json:"refs"`
	Sources      []string `json:"sources"`
	Requirements []string `json:"requirements"`
	Steps        int      `json:"steps"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// SummarizeRequirement returns r in lang.
func (c *Catalog) SummarizeRequirement(r *Requirement, lang string) ReqSummary {
	x := r.Localized(lang)
	g, k := "", ""
	if a := c.Area(r.Area); a != nil {
		g, k = a.Group, a.Catalogue
	}
	return ReqSummary{ID: r.ID, Area: r.Area, Group: g, Catalogue: k, Type: r.Type, Obligation: r.Obligation, Priority: r.Priority,
		Tags: nonNil(r.Tags), Refs: nonNil(r.Refs), Sources: nonNil(r.Sources), Tests: nonNil(r.Tests), Risks: nonNil(c.RisksOf[r.ID]), Title: x.Title, Description: x.Description}
}

// SummarizeRisk returns k in lang.
func (c *Catalog) SummarizeRisk(k *Risk, lang string) RiskSummary {
	x := k.Localized(lang)
	g, kind := "", ""
	if a := c.Area(k.Area); a != nil {
		g, kind = a.Group, a.Catalogue
	}
	cat := k.Category
	if rc := RiskCategory(k.Category); rc != nil {
		cat = rc.Name.Get(lang)
	}
	return RiskSummary{ID: k.ID, Area: k.Area, Group: g, Catalogue: kind, Category: k.Category, CategoryName: cat,
		Likelihood: k.Likelihood, Impact: k.Impact, Score: k.Likelihood * k.Impact, Level: k.Level(),
		Tags: nonNil(k.Tags), Refs: nonNil(k.Refs), Sources: nonNil(k.Sources), MitigatedBy: nonNil(k.MitigatedBy), Tests: nonNil(k.Tests),
		Attack: nonNil(k.Attack), Atlas: nonNil(k.Atlas), Tactics: nonNil(c.TacticsOf(k.Techniques())),
		Title: x.Title, Description: x.Description, Mitigation: x.Mitigation}
}

// riskSources returns the sources a risk brings in: those it cites and
// those of the requirements that mitigate it.
func (c *Catalog) riskSources(k *Risk) []string {
	out := append([]string{}, k.Sources...)
	for _, rid := range k.MitigatedBy {
		if r := c.Requirements[rid]; r != nil {
			for _, s := range r.Sources {
				if !slices.Contains(out, s) {
					out = append(out, s)
				}
			}
		}
	}
	return out
}

// FindRisks returns the risks matching f in lang (see FindRequirements):
// Area, Group, Catalogue, Category, Level, Tag, Source and Q apply.
func (c *Catalog) FindRisks(f Filter, lang string) []RiskSummary {
	lang = Lang(lang)
	areas, groups, kinds, cats, levels, tags, sources := set(f.Area), set(f.Group), set(f.Catalogue), set(f.Category), set(f.Level), set(f.Tag), set(f.Source)
	techs, tactics := set(f.Attack), set(f.Tactic)
	terms := Terms(f.Q)
	type hit struct {
		s     RiskSummary
		score int
	}
	var hits []hit
	for _, a := range c.OrderedAreas() {
		if !accepts(areas, a.ID) || !accepts(groups, a.Group) || !accepts(kinds, a.Catalogue) {
			continue
		}
		for i := range a.Risks {
			k := &a.Risks[i]
			if !accepts(cats, k.Category) || !accepts(levels, k.Level()) || !acceptsAny(tags, k.Tags) || (sources != nil && !acceptsAny(sources, c.riskSources(k))) {
				continue
			}
			if (techs != nil && !c.techniqueMatches(k.Techniques(), techs)) || (tactics != nil && !acceptsAny(tactics, c.TacticsOf(k.Techniques()))) {
				continue
			}
			x := k.Localized(lang)
			sc := score(k.ID, terms, []field{
				{lower(x.Title), 10}, {lower(k.ID), 8}, {lower(k.Tags...), 6}, {lower(k.Refs...), 4},
				{lower(x.Description), 3}, {lower(k.MitigatedBy...), 2}, {lower(x.Mitigation), 1},
				{lower(c.techniqueTerms(k)...), 4},
			})
			if sc > 0 {
				hits = append(hits, hit{c.SummarizeRisk(k, lang), sc})
			}
		}
	}
	if len(terms) > 0 {
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	}
	out := make([]RiskSummary, len(hits))
	for i, h := range hits {
		out[i] = h.s
	}
	return out
}

// SummarizeTest returns t in lang.
func (c *Catalog) SummarizeTest(t *Test, lang string) TestSummary {
	x := t.Localized(lang)
	g, k := "", ""
	if a := c.Area(t.Area); a != nil {
		g, k = a.Group, a.Catalogue
	}
	return TestSummary{ID: t.ID, Area: t.Area, Group: g, Catalogue: k, Priority: t.Priority, Execution: t.Execution,
		Tags: nonNil(t.Tags), Refs: nonNil(t.Refs), Sources: nonNil(t.Sources), Requirements: nonNil(t.Requirements), Steps: len(x.Steps), Title: x.Title, Description: x.Description}
}

// set parses a comma-separated filter value (nil = any).
func set(v string) map[string]bool {
	var out map[string]bool
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(strings.ToLower(p)); p != "" {
			if out == nil {
				out = map[string]bool{}
			}
			out[p] = true
		}
	}
	return out
}

func accepts(s map[string]bool, v string) bool { return s == nil || s[strings.ToLower(v)] }

func acceptsAny(s map[string]bool, vs []string) bool {
	if s == nil {
		return true
	}
	for _, v := range vs {
		if s[strings.ToLower(v)] {
			return true
		}
	}
	return false
}

// Terms splits free text into lower-case search words.
func Terms(q string) []string {
	return strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '.' && r != '§'
	})
}

// field is a searchable text with its weight.
type field struct {
	text   string
	weight int
}

// Score ranks an item for the search words: every word must occur in one
// of the fields (else 0). A word scores the weight of the best field it
// occurs in, doubled at the start of a word; an exact id match ranks first
// and the whole query as a phrase in the first field (the title) adds a
// bonus.
func score(id string, terms []string, fields []field) int {
	if len(terms) == 0 {
		return 1
	}
	total := 0
	if len(terms) > 1 && strings.Contains(fields[0].text, strings.Join(terms, " ")) {
		total += 50
	}
	lid := strings.ToLower(id)
	for _, t := range terms {
		if t == lid {
			total += 1000
			continue
		}
		best := 0
		for _, f := range fields {
			i := strings.Index(f.text, t)
			if i < 0 {
				continue
			}
			w := f.weight
			if i == 0 || !isWordRune(rune(f.text[i-1])) {
				w *= 2
			}
			best = max(best, w)
		}
		if best == 0 {
			return 0
		}
		total += best
	}
	return total
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

func lower(ss ...string) string { return strings.ToLower(strings.Join(ss, " ")) }

// FindRequirements returns the requirements matching f in lang: ranked by
// the search score when f.Q is set, else in catalogue order.
func (c *Catalog) FindRequirements(f Filter, lang string) []ReqSummary {
	lang = Lang(lang)
	areas, groups, types, obl, prio, tags := set(f.Area), set(f.Group), set(f.Type), set(f.Obligation), set(f.Priority), set(f.Tag)
	kinds, sources := set(f.Catalogue), set(f.Source)
	terms := Terms(f.Q)
	type hit struct {
		s     ReqSummary
		score int
	}
	var hits []hit
	for _, a := range c.OrderedAreas() {
		if !accepts(areas, a.ID) || !accepts(groups, a.Group) || !accepts(kinds, a.Catalogue) {
			continue
		}
		for i := range a.Requirements {
			r := &a.Requirements[i]
			if !accepts(types, r.Type) || !accepts(obl, r.Obligation) || !accepts(prio, r.Priority) || !acceptsAny(tags, r.Tags) || !acceptsAny(sources, r.Sources) {
				continue
			}
			x := r.Localized(lang)
			sc := score(r.ID, terms, []field{
				{lower(x.Title), 10}, {lower(r.ID), 8}, {lower(r.Tags...), 6}, {lower(r.Refs...), 4},
				{lower(x.Description), 3}, {lower(x.Rationale), 1},
			})
			if sc > 0 {
				hits = append(hits, hit{c.SummarizeRequirement(r, lang), sc})
			}
		}
	}
	if len(terms) > 0 {
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	}
	out := make([]ReqSummary, len(hits))
	for i, h := range hits {
		out[i] = h.s
	}
	return out
}

// FindTests returns the tests matching f in lang (see FindRequirements).
// Type and Obligation do not apply to tests.
func (c *Catalog) FindTests(f Filter, lang string) []TestSummary {
	lang = Lang(lang)
	areas, groups, prio, tags, exec := set(f.Area), set(f.Group), set(f.Priority), set(f.Tag), set(f.Execution)
	kinds, sources := set(f.Catalogue), set(f.Source)
	terms := Terms(f.Q)
	type hit struct {
		s     TestSummary
		score int
	}
	var hits []hit
	for _, a := range c.OrderedAreas() {
		if !accepts(areas, a.ID) || !accepts(groups, a.Group) || !accepts(kinds, a.Catalogue) {
			continue
		}
		for i := range a.Tests {
			t := &a.Tests[i]
			if !accepts(prio, t.Priority) || !accepts(exec, t.Execution) || !acceptsAny(tags, t.Tags) || (sources != nil && !acceptsAny(sources, c.testSources(t))) {
				continue
			}
			x := t.Localized(lang)
			var steps []string
			for _, s := range x.Steps {
				steps = append(steps, s.Action, s.Expected, s.Data)
			}
			sc := score(t.ID, terms, []field{
				{lower(x.Title), 10}, {lower(t.ID), 8}, {lower(t.Tags...), 6}, {lower(t.Refs...), 4},
				{lower(x.Description), 3}, {lower(x.Expected, x.Preconditions), 2}, {lower(steps...), 1},
				{lower(t.Requirements...), 2},
			})
			if sc > 0 {
				hits = append(hits, hit{c.SummarizeTest(t, lang), sc})
			}
		}
	}
	if len(terms) > 0 {
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	}
	out := make([]TestSummary, len(hits))
	for i, h := range hits {
		out[i] = h.s
	}
	return out
}

// AreaSummary is an area with its item counts.
type AreaSummary struct {
	ID           string `json:"id"`
	Prefix       string `json:"prefix"`
	Group        string `json:"group"`
	Catalogue    string `json:"catalogue"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Requirements int    `json:"requirements"`
	Tests        int    `json:"tests"`
	Risks        int    `json:"risks"`
}

// GroupSummary is a group with its areas and counts.
type GroupSummary struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Requirements int           `json:"requirements"`
	Tests        int           `json:"tests"`
	Risks        int           `json:"risks"`
	Areas        []AreaSummary `json:"areas"`
}

// Overview returns the groups (in order) with their areas in lang; groups
// without areas are left out.
func (c *Catalog) Overview(lang string) []GroupSummary {
	lang = Lang(lang)
	out := []GroupSummary{}
	for _, g := range c.Groups {
		gs := GroupSummary{ID: g.ID, Name: g.Name.Get(lang), Areas: []AreaSummary{}}
		for _, a := range c.OrderedAreas() {
			if a.Group != g.ID {
				continue
			}
			as := AreaSummary{ID: a.ID, Prefix: a.Prefix, Group: a.Group, Catalogue: a.Catalogue, Name: a.Name.Get(lang), Description: a.Description.Get(lang),
				Requirements: len(a.Requirements), Tests: len(a.Tests), Risks: len(a.Risks)}
			gs.Areas = append(gs.Areas, as)
			gs.Requirements += as.Requirements
			gs.Tests += as.Tests
			gs.Risks += as.Risks
		}
		if len(gs.Areas) > 0 {
			out = append(out, gs)
		}
	}
	return out
}

// KindSummary is a catalogue type with its counts.
type KindSummary struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Areas        int    `json:"areas"`
	Requirements int    `json:"requirements"`
	Tests        int    `json:"tests"`
	Risks        int    `json:"risks"`
}

// KindsOverview returns the catalogue types (in order) in lang; types
// without areas are left out.
func (c *Catalog) KindsOverview(lang string) []KindSummary {
	lang = Lang(lang)
	out := []KindSummary{}
	for _, k := range c.Kinds {
		ks := KindSummary{ID: k.ID, Name: k.Name.Get(lang), Description: k.Description.Get(lang)}
		for _, a := range c.Areas {
			if a.Catalogue == k.ID {
				ks.Areas++
				ks.Requirements += len(a.Requirements)
				ks.Tests += len(a.Tests)
				ks.Risks += len(a.Risks)
			}
		}
		if ks.Areas > 0 {
			out = append(out, ks)
		}
	}
	return out
}

// TagCount is a tag and how many items carry it.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// Tags returns the tags of the requirements (kind "requirement"), risks
// (kind "risk") or tests, most used first.
func (c *Catalog) Tags(kind string) []TagCount {
	n := map[string]int{}
	for _, a := range c.Areas {
		if kind == model.KindRisk {
			for _, k := range a.Risks {
				for _, t := range k.Tags {
					n[t]++
				}
			}
			continue
		}
		if kind == "requirement" {
			for _, r := range a.Requirements {
				for _, t := range r.Tags {
					n[t]++
				}
			}
			continue
		}
		for _, t := range a.Tests {
			for _, g := range t.Tags {
				n[g]++
			}
		}
	}
	out := make([]TagCount, 0, len(n))
	for t, k := range n {
		out = append(out, TagCount{t, k})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out
}

// Techniques are the ATT&CK and ATLAS technique ids of the risk.
func (k *Risk) Techniques() []string {
	return append(append([]string{}, k.Attack...), k.Atlas...)
}

// techniqueTerms are the ids and names of the risk's techniques, for the
// free-text search ("T1110", "credential stuffing").
func (c *Catalog) techniqueTerms(k *Risk) []string {
	var out []string
	for _, id := range k.Techniques() {
		out = append(out, id)
		if t := c.techByID[id]; t != nil {
			out = append(out, c.FullName(t))
		}
	}
	return out
}
