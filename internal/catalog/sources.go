package catalog

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Reference sources (data/sources.yaml): the laws, regulations, standards
// and best practices the catalogue cites. Every entry of an item's refs
// resolves to one source by its match prefixes or regular expressions, so
// a team can pick "comply with the CRA" and get every item that cites it,
// and the analytics can measure compliance per source and clause.

// SourceKind is a kind of source (law, directive, standard …).
type SourceKind struct {
	ID   string `yaml:"id" json:"id"`
	Name Names  `yaml:"name" json:"name"`
}

// Source is a law, regulation, standard or best practice cited in refs.
type Source struct {
	ID   string `yaml:"id" json:"id"`
	Kind string `yaml:"kind" json:"kind"`
	// Jurisdiction: EU, SE, US … (two capitals), "international" or "".
	Jurisdiction string `yaml:"jurisdiction" json:"jurisdiction,omitempty"`
	// Short is an optional short name, the same in every language.
	Short string `yaml:"short" json:"short,omitempty"`
	Name  Names  `yaml:"name" json:"name"`
	URL   string `yaml:"url" json:"url,omitempty"`
	// Links are the documents of a source that groups several (a family
	// of standards, a law with its implementing acts), each with its own
	// page. A ref links to the document whose match prefix it starts with.
	Links []SourceLink `yaml:"links" json:"links,omitempty"`
	// RefLinks derive a link per cited document from the refs themselves
	// (RFC 2119 → rfc-editor.org/rfc/rfc2119), for families too large to
	// list.
	RefLinks []RefLink `yaml:"ref_links" json:"-"`
	// Match are prefixes a ref starts with (case sensitive).
	Match []string `yaml:"match" json:"-"`
	// Regex are regular expressions a ref matches (Go syntax).
	Regex []string `yaml:"regex" json:"-"`
	// Clause optionally finds the clause group in a ref: the expansion of
	// ClauseFormat ("Req. $1") when set, else the first capture group,
	// else the whole match. Without it built-in patterns apply (ClauseOf).
	Clause       string `yaml:"clause" json:"-"`
	ClauseFormat string `yaml:"clause_format" json:"-"`

	res      []*regexp.Regexp
	clauseRe *regexp.Regexp
}

// SourceLink is one document of a source, with its own page.
type SourceLink struct {
	// Label names the document (its designation, the same in every
	// language), e.g. "IEC 60300-1".
	Label string `yaml:"label" json:"label"`
	URL   string `yaml:"url" json:"url"`
	// Match are the prefixes of the refs that cite the document; default:
	// the label.
	Match []string `yaml:"match" json:"-"`
}

// RefLink derives a document link from a ref: when Regex matches the ref,
// Label and URL are expanded with its capture groups ($1, $2 …).
type RefLink struct {
	Regex string `yaml:"regex" json:"-"`
	Label string `yaml:"label" json:"-"`
	URL   string `yaml:"url" json:"-"`

	re *regexp.Regexp
}

// DocLink is a link to a cited document (or to its source as a whole).
type DocLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

var jurisdictionRe = regexp.MustCompile(`^(?:[A-Z]{2}|international)$`)

// loadSources checks the registry and compiles its expressions.
func (c *Catalog) loadSources(bad func(string, ...any)) {
	kinds := map[string]bool{}
	for _, k := range c.SourceKinds {
		if !areaRe.MatchString(k.ID) || kinds[k.ID] {
			bad("source kind %q: invalid or duplicate id", k.ID)
		}
		kinds[k.ID] = true
		for _, l := range Languages {
			if strings.TrimSpace(k.Name[l]) == "" {
				bad("source kind %s: name is needed in %s", k.ID, l)
			}
		}
	}
	c.sourceByID = map[string]*Source{}
	for i := range c.Sources {
		s := &c.Sources[i]
		if !areaRe.MatchString(s.ID) || c.sourceByID[s.ID] != nil {
			bad("source %q: invalid or duplicate id", s.ID)
		}
		c.sourceByID[s.ID] = s
		if !kinds[s.Kind] {
			bad("source %s: unknown kind %q", s.ID, s.Kind)
		}
		if s.Jurisdiction != "" && !jurisdictionRe.MatchString(s.Jurisdiction) {
			bad("source %s: jurisdiction must be two capitals (EU, SE …) or international", s.ID)
		}
		if s.URL != "" && !validURL(s.URL) {
			bad("source %s: url must start with https:// and have no spaces", s.ID)
		}
		labels := map[string]bool{}
		for _, l := range s.Links {
			switch {
			case strings.TrimSpace(l.Label) == "" || labels[l.Label]:
				bad("source %s: link %q: empty or duplicate label", s.ID, l.Label)
			case !validURL(l.URL):
				bad("source %s: link %s: url must start with https:// and have no spaces", s.ID, l.Label)
			}
			labels[l.Label] = true
			for _, m := range l.Match {
				if strings.TrimSpace(m) == "" {
					bad("source %s: link %s: empty match prefix", s.ID, l.Label)
				}
			}
		}
		for i := range s.RefLinks {
			rl := &s.RefLinks[i]
			re, err := regexp.Compile(rl.Regex)
			switch {
			case err != nil:
				bad("source %s: ref_links regex %q: %v", s.ID, rl.Regex, err)
				continue
			case strings.TrimSpace(rl.Label) == "":
				bad("source %s: ref_links %q: label is needed", s.ID, rl.Regex)
			case !validURL(rl.URL):
				bad("source %s: ref_links %q: url must start with https:// and have no spaces", s.ID, rl.Regex)
			}
			rl.re = re
		}
		for _, l := range Languages {
			if strings.TrimSpace(s.Name[l]) == "" {
				bad("source %s: name is needed in %s", s.ID, l)
			}
		}
		if len(s.Match)+len(s.Regex) == 0 {
			bad("source %s: needs at least one match prefix or regex", s.ID)
		}
		for _, m := range s.Match {
			if strings.TrimSpace(m) == "" {
				bad("source %s: empty match prefix", s.ID)
			}
		}
		for _, x := range s.Regex {
			re, err := regexp.Compile(x)
			if err != nil {
				bad("source %s: regex %q: %v", s.ID, x, err)
				continue
			}
			s.res = append(s.res, re)
		}
		if s.ClauseFormat != "" && s.Clause == "" {
			bad("source %s: clause_format needs clause", s.ID)
		}
		if s.Clause != "" {
			re, err := regexp.Compile(s.Clause)
			if err != nil {
				bad("source %s: clause %q: %v", s.ID, s.Clause, err)
			} else {
				s.clauseRe = re
			}
		}
	}
}

func validURL(u string) bool {
	return strings.HasPrefix(u, "https://") && len(u) > len("https://") && !strings.ContainsAny(u, " \t\r\n<>\"")
}

// DocLink returns the most specific link for ref, a ref of the source:
// the document a ref_links pattern derives from it, else the link whose
// match prefix (the longest) it starts with, else the source's own url.
// ok is false when the source has no link at all.
func (s *Source) DocLink(ref string) (DocLink, bool) {
	if d, ok := s.refLink(ref); ok {
		return d, true
	}
	best, n := -1, 0
	for i, l := range s.Links {
		for _, m := range linkMatch(l) {
			if strings.HasPrefix(ref, m) && len(m) > n {
				best, n = i, len(m)
			}
		}
	}
	if best >= 0 {
		return DocLink{Label: s.Links[best].Label, URL: s.Links[best].URL}, true
	}
	if s.URL != "" {
		return DocLink{Label: s.Short, URL: s.URL}, true
	}
	return DocLink{}, false
}

func linkMatch(l SourceLink) []string {
	if len(l.Match) > 0 {
		return l.Match
	}
	return []string{l.Label}
}

// refLink expands the first ref_links pattern that matches ref.
func (s *Source) refLink(ref string) (DocLink, bool) {
	for _, rl := range s.RefLinks {
		if rl.re == nil {
			continue
		}
		idx := rl.re.FindStringSubmatchIndex(ref)
		if idx == nil {
			continue
		}
		u := string(rl.re.ExpandString(nil, rl.URL, ref, idx))
		if !validURL(u) {
			continue
		}
		return DocLink{Label: string(rl.re.ExpandString(nil, rl.Label, ref, idx)), URL: u}, true
	}
	return DocLink{}, false
}

// Matches reports whether ref belongs to the source.
func (s *Source) Matches(ref string) bool {
	for _, m := range s.Match {
		if strings.HasPrefix(ref, m) {
			return true
		}
	}
	for _, re := range s.res {
		if re.MatchString(ref) {
			return true
		}
	}
	return false
}

// rest is ref without the source's matching prefix.
func (s *Source) rest(ref string) string {
	best := ""
	for _, m := range s.Match {
		if strings.HasPrefix(ref, m) && len(m) > len(best) {
			best = m
		}
	}
	return ref[len(best):]
}

// Default clause patterns, tried in order on the ref without the
// source prefix; the first that matches gives the clause.
var clausePatterns = []struct {
	re  *regexp.Regexp
	fmt func(m []string) string
}{
	{regexp.MustCompile(`\bAnnex\s+([IVXLC]+)(?:\s+Part\s+([IVXLC]+))?\b`), func(m []string) string {
		if m[2] != "" {
			return "Annex " + m[1] + " Part " + m[2]
		}
		return "Annex " + m[1]
	}},
	{regexp.MustCompile(`\bArt(?:icle)?\.?\s*(\d+)`), func(m []string) string { return "Art. " + m[1] }},
	{regexp.MustCompile(`\bA\.(\d+)`), func(m []string) string { return "A." + m[1] }},
	{regexp.MustCompile(`\b(SR|CR|SC|ZCR|SP|Req\.|Requirement)\s*\.?(\d+)`), func(m []string) string {
		if m[1] == "Requirement" {
			m[1] = "Req."
		}
		return m[1] + " " + strings.TrimLeft(m[2], "0")
	}},
	{regexp.MustCompile(`\b(LLM\d+|API\d+|A\d{2})\b`), func(m []string) string { return m[1] }},
	{regexp.MustCompile(`\b([A-Z]{2,5}(?:-[A-Z]{2,5})*)-\d+`), func(m []string) string { return m[1] }},
	{regexp.MustCompile(`\b([A-Z]{2,4})\.\d+`), func(m []string) string { return m[1] }},
	{regexp.MustCompile(`\bV(\d+)`), func(m []string) string { return "V" + m[1] }},
	{regexp.MustCompile(`\b(?:[Cc]h(?:apter)?\.?)\s*(\d+)`), func(m []string) string { return "ch. " + m[1] }},
	{regexp.MustCompile(`(\d+)\s*kap\.`), func(m []string) string { return m[1] + " kap." }},
	{regexp.MustCompile(`§\s*(\d+)`), func(m []string) string { return "§" + m[1] }},
}

// ClauseOf returns the clause group of a ref of the source (e.g. "Art. 32",
// "A.8", "Annex I Part I", "SR 1"), "" when the ref names none.
// The first letter is upper case.
func (s *Source) ClauseOf(ref string) string {
	return upperFirst(s.clauseOf(ref))
}

func (s *Source) clauseOf(ref string) string {
	if s.clauseRe != nil {
		idx := s.clauseRe.FindStringSubmatchIndex(ref)
		switch {
		case idx == nil:
			return ""
		case s.ClauseFormat != "":
			return strings.TrimSpace(string(s.clauseRe.ExpandString(nil, s.ClauseFormat, ref, idx)))
		case len(idx) > 2 && idx[2] >= 0:
			return strings.TrimSpace(ref[idx[2]:idx[3]])
		default:
			return strings.TrimSpace(strings.TrimRight(ref[idx[0]:idx[1]], ":( "))
		}
	}
	rest := s.rest(ref)
	for _, p := range clausePatterns {
		if m := p.re.FindStringSubmatch(rest); m != nil {
			return p.fmt(m)
		}
	}
	return ""
}

func upperFirst(s string) string {
	for i, r := range s {
		return strings.ToUpper(string(r)) + s[i+len(string(r)):]
	}
	return s
}

// Source returns the source with id, or nil.
func (c *Catalog) Source(id string) *Source { return c.sourceByID[id] }

// MatchRef returns the sources a ref matches (exactly one in a valid
// catalogue).
func (c *Catalog) MatchRef(ref string) []*Source {
	var out []*Source
	for i := range c.Sources {
		if c.Sources[i].Matches(ref) {
			out = append(out, &c.Sources[i])
		}
	}
	return out
}

// RefSource returns the id of the source of ref ("" when none; the first
// when several match).
func (c *Catalog) RefSource(ref string) string {
	if id, ok := c.refSource[ref]; ok {
		return id
	}
	if m := c.MatchRef(ref); len(m) > 0 {
		return m[0].ID
	}
	return ""
}

// SourcesOfRefs returns the distinct source ids of refs, in registry order.
func (c *Catalog) SourcesOfRefs(refs []string) []string {
	set := map[string]bool{}
	for _, r := range refs {
		if id := c.RefSource(r); id != "" {
			set[id] = true
		}
	}
	out := []string{}
	for _, s := range c.Sources {
		if set[s.ID] {
			out = append(out, s.ID)
		}
	}
	return out
}

// SourcesOfText resolves free text, such as the source field of a project
// requirement ("GDPR Art. 32; ISO/IEC 27001:2022 A.8.5"), to source ids:
// each part separated by ";" or a line break is matched like a ref, and
// failing that without regard to case.
func (c *Catalog) SourcesOfText(text string) []string {
	var refs []string
	for _, p := range strings.FieldsFunc(text, func(r rune) bool { return r == ';' || r == '\n' }) {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if c.RefSource(p) != "" {
			refs = append(refs, p)
			continue
		}
		lp := strings.ToLower(p)
	sources:
		for i := range c.Sources {
			for _, m := range c.Sources[i].Match {
				if strings.HasPrefix(lp, strings.ToLower(m)) {
					refs = append(refs, c.Sources[i].Match[0]+p[len(m):])
					break sources
				}
			}
		}
	}
	return c.SourcesOfRefs(refs)
}

// resolveRefs maps every ref of the loaded items to its source and sets
// the items' Sources.
func (c *Catalog) resolveRefs() {
	c.refSource = map[string]string{}
	for _, a := range c.Areas {
		for _, r := range a.Requirements {
			for _, ref := range r.Refs {
				c.refSource[ref] = c.RefSource(ref)
			}
		}
		for _, t := range a.Tests {
			for _, ref := range t.Refs {
				c.refSource[ref] = c.RefSource(ref)
			}
		}
	}
	for _, r := range c.Requirements {
		r.Sources = c.SourcesOfRefs(r.Refs)
	}
	for _, t := range c.Tests {
		t.Sources = c.SourcesOfRefs(t.Refs)
	}
}

// RefProblems lists the refs that match no source or several: every ref
// must match exactly one (checked by the package tests, so that a new area
// citing a new source is caught before it is merged).
func (c *Catalog) RefProblems() []string {
	var out []string
	check := func(id string, refs []string) {
		for _, ref := range refs {
			m := c.MatchRef(ref)
			switch len(m) {
			case 1:
			case 0:
				out = append(out, fmt.Sprintf("%s: ref %q matches no source: add the source to data/sources.yaml (see FORMAT.md, \"Reference sources\") or spell the ref as an existing source does", id, ref))
			default:
				ids := make([]string, len(m))
				for i, s := range m {
					ids[i] = s.ID
				}
				out = append(out, fmt.Sprintf("%s: ref %q matches several sources (%s): make their match patterns distinct", id, ref, strings.Join(ids, ", ")))
			}
		}
	}
	for _, a := range c.Areas {
		for _, r := range a.Requirements {
			check(r.ID, r.Refs)
		}
		for _, t := range a.Tests {
			check(t.ID, t.Refs)
		}
	}
	sort.Strings(out)
	return out
}

// testSources returns the sources a test brings in: those it cites and
// those of the requirements it verifies.
func (c *Catalog) testSources(t *Test) []string {
	out := append([]string{}, t.Sources...)
	for _, rid := range t.Requirements {
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

// SourceSummary is a source in one language with its counts.
type SourceSummary struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Jurisdiction string `json:"jurisdiction,omitempty"`
	Short        string `json:"short,omitempty"`
	Name         string `json:"name"`
	URL          string `json:"url,omitempty"`
	// Requirements cite the source; Tests cite it or verify such a
	// requirement (what an import of the requirements brings).
	Requirements int `json:"requirements"`
	Tests        int `json:"tests"`
	// Catalogues are the catalogue types with items that cite it.
	Catalogues []string `json:"catalogues"`
}

// SourceKindSummary is a source kind in one language.
type SourceKindSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SourceKindsOverview returns the source kinds in order, in lang.
func (c *Catalog) SourceKindsOverview(lang string) []SourceKindSummary {
	lang = Lang(lang)
	out := []SourceKindSummary{}
	for _, k := range c.SourceKinds {
		out = append(out, SourceKindSummary{ID: k.ID, Name: k.Name.Get(lang)})
	}
	return out
}

// SourcesOverview returns the sources in lang, grouped by kind (registry
// order within a kind), with their counts in the catalogue types of
// catalogue (comma-separated, "" = all). Sources no item cites are left
// out unless all is set.
func (c *Catalog) SourcesOverview(lang, catalogue string, all bool) []SourceSummary {
	lang = Lang(lang)
	kinds := set(catalogue)
	reqs, tests, cats := map[string]int{}, map[string]int{}, map[string]map[string]bool{}
	for _, a := range c.Areas {
		if !accepts(kinds, a.Catalogue) {
			continue
		}
		for _, r := range a.Requirements {
			for _, s := range r.Sources {
				reqs[s]++
				if cats[s] == nil {
					cats[s] = map[string]bool{}
				}
				cats[s][a.Catalogue] = true
			}
		}
		for i := range a.Tests {
			for _, s := range c.testSources(&a.Tests[i]) {
				tests[s]++
				if cats[s] == nil {
					cats[s] = map[string]bool{}
				}
				cats[s][a.Catalogue] = true
			}
		}
	}
	rank := map[string]int{}
	for i, k := range c.SourceKinds {
		rank[k.ID] = i
	}
	out := []SourceSummary{}
	for _, s := range c.Sources {
		if !all && reqs[s.ID]+tests[s.ID] == 0 {
			continue
		}
		ss := SourceSummary{ID: s.ID, Kind: s.Kind, Jurisdiction: s.Jurisdiction, Short: s.Short, Name: s.Name.Get(lang), URL: s.URL,
			Requirements: reqs[s.ID], Tests: tests[s.ID], Catalogues: []string{}}
		for _, k := range c.Kinds {
			if cats[s.ID][k.ID] {
				ss.Catalogues = append(ss.Catalogues, k.ID)
			}
		}
		out = append(out, ss)
	}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Kind] < rank[out[j].Kind] })
	return out
}
