// Package soa builds a Statement of Applicability (SoA) for a reference
// source of the catalogue's source registry: for every control (or clause)
// of the source, whether it applies and why, who owns it, how far it is
// implemented (derived from the adopted requirements and their
// verification) and the requirements and evidence behind it.
//
// The controls of a source come from a shipped control list (data/*.yaml;
// ISO/IEC 27001:2022 Annex A ships its 93 control identifiers and short
// names) or, for every other source, from the clause groups of the refs
// that cite it in the catalogue and in the project's own requirements.
// Applicability, justification and owner are the project's decisions,
// stored per project and source (store.SoADecisions).
package soa

import (
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/rom/qtestframe/internal/catalog"
	"github.com/rom/qtestframe/internal/model"
)

//go:embed data/*.yaml
var dataFS embed.FS

// Implementation states of a control (model.SoAStatuses).
const (
	Implemented    = "implemented"     // every requirement of the control is verified
	Partial        = "partial"         // some are verified or waived
	Planned        = "planned"         // requirements are adopted, none verified yet
	NotImplemented = "not_implemented" // no requirement addresses it
)

// Requirement states (as in analytics: passed, failing, blocked, untested).
const (
	StatePassed   = "passed"
	StateFailing  = "failing"
	StateBlocked  = "blocked"
	StateUntested = "untested"
)

// shippedControl is a control of a shipped list.
type shippedControl struct {
	ID string `yaml:"id"`
	En string `yaml:"en"`
	Sv string `yaml:"sv"`
}

type shippedGroup struct {
	ID       string           `yaml:"id"`
	Title    catalog.Names    `yaml:"title"`
	Controls []shippedControl `yaml:"controls"`
}

// controlList is a shipped control list of a source.
type controlList struct {
	Source        string         `yaml:"source"`
	Title         catalog.Names  `yaml:"title"`
	Control       string         `yaml:"control"`
	ControlFormat string         `yaml:"control_format"`
	Groups        []shippedGroup `yaml:"groups"`

	re  *regexp.Regexp
	ids map[string]bool
}

var (
	loadOnce sync.Once
	lists    map[string]*controlList
	errLists error
)

// shipped returns the shipped control lists by source id.
func shipped() (map[string]*controlList, error) {
	loadOnce.Do(func() { lists, errLists = loadLists(dataFS, "data") })
	return lists, errLists
}

func loadLists(fsys fs.FS, dir string) (map[string]*controlList, error) {
	names, err := fs.Glob(fsys, dir+"/*.yaml")
	if err != nil {
		return nil, err
	}
	out := map[string]*controlList{}
	for _, n := range names {
		b, err := fs.ReadFile(fsys, n)
		if err != nil {
			return nil, err
		}
		var l controlList
		if err := yaml.Unmarshal(b, &l); err != nil {
			return nil, fmt.Errorf("soa %s: %w", n, err)
		}
		if l.Source == "" || out[l.Source] != nil {
			return nil, fmt.Errorf("soa %s: missing or duplicate source", n)
		}
		if l.re, err = regexp.Compile(l.Control); err != nil || l.Control == "" {
			return nil, fmt.Errorf("soa %s: control pattern %q: %v", n, l.Control, err)
		}
		l.ids = map[string]bool{}
		for _, g := range l.Groups {
			for _, c := range g.Controls {
				if c.ID == "" || c.En == "" || l.ids[c.ID] {
					return nil, fmt.Errorf("soa %s: control %q: empty, untitled or duplicate", n, c.ID)
				}
				l.ids[c.ID] = true
			}
		}
		out[l.Source] = &l
	}
	return out, nil
}

// controlOf finds the control of a ref in a shipped list ("" none).
func (l *controlList) controlOf(ref string) string {
	idx := l.re.FindStringSubmatchIndex(ref)
	if idx == nil {
		return ""
	}
	id := ref[idx[0]:idx[1]]
	if l.ControlFormat != "" {
		id = string(l.re.ExpandString(nil, l.ControlFormat, ref, idx))
	}
	if !l.ids[id] {
		return ""
	}
	return id
}

// Control is a control or clause of a source.
type Control struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Group      string `json:"group"`
	GroupTitle string `json:"group_title"`
}

// SourceInfo describes a source for the SoA picker.
type SourceInfo struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Short string `json:"short,omitempty"`
	Name  string `json:"name"`
	// Title of the statement (the control list's, else the source name).
	Title string `json:"title"`
	// Shipped: the controls come from a shipped list (else from the
	// clause groups of the catalogue's refs).
	Shipped  bool `json:"shipped"`
	Controls int  `json:"controls"`
}

// mapper finds the control of a ref of one source.
type mapper struct {
	src  *catalog.Source
	list *controlList
}

func (m mapper) of(ref string) string {
	if m.list != nil {
		return m.list.controlOf(ref)
	}
	return m.src.ClauseOf(ref)
}

func mapperFor(c *catalog.Catalog, sourceID string) (mapper, error) {
	src := c.Source(sourceID)
	if src == nil {
		return mapper{}, fmt.Errorf("unknown reference source %q", sourceID)
	}
	ls, err := shipped()
	if err != nil {
		return mapper{}, err
	}
	return mapper{src: src, list: ls[sourceID]}, nil
}

// Controls returns the controls of a source in order, in lang. extra are
// further refs (the source fields of the project's own requirements) whose
// clauses add controls to a derived list.
func Controls(c *catalog.Catalog, sourceID, lang string, extra ...string) ([]Control, error) {
	m, err := mapperFor(c, sourceID)
	if err != nil {
		return nil, err
	}
	lang = catalog.Lang(lang)
	if m.list != nil {
		var out []Control
		for _, g := range m.list.Groups {
			for _, x := range g.Controls {
				t := x.En
				if lang == "sv" && x.Sv != "" {
					t = x.Sv
				}
				out = append(out, Control{ID: x.ID, Title: t, Group: g.ID, GroupTitle: g.Title.Get(lang)})
			}
		}
		return out, nil
	}
	seen := map[string]bool{}
	add := func(ref string) {
		if c.RefSource(ref) != sourceID && !slices.Contains(c.SourcesOfText(ref), sourceID) {
			return
		}
		if k := m.of(ref); k != "" {
			seen[k] = true
		}
	}
	for _, rq := range c.Requirements {
		for _, ref := range rq.Refs {
			add(ref)
		}
	}
	for _, text := range extra {
		for _, part := range splitSource(text) {
			add(part)
		}
	}
	ids := make([]string, 0, len(seen))
	for k := range seen {
		ids = append(ids, k)
	}
	sort.Slice(ids, func(i, j int) bool { return naturalLess(ids[i], ids[j]) })
	out := make([]Control, len(ids))
	for i, id := range ids {
		out[i] = Control{ID: id, Title: ""}
	}
	return out, nil
}

// Sources lists the registry sources an SoA can be made for: those with a
// shipped control list and those whose catalogue refs name clauses, in
// registry order.
func Sources(c *catalog.Catalog, lang string) ([]SourceInfo, error) {
	ls, err := shipped()
	if err != nil {
		return nil, err
	}
	lang = catalog.Lang(lang)
	clauses := map[string]map[string]bool{}
	for _, rq := range c.Requirements {
		for _, ref := range rq.Refs {
			id := c.RefSource(ref)
			if id == "" || ls[id] != nil {
				continue
			}
			if k := c.Source(id).ClauseOf(ref); k != "" {
				if clauses[id] == nil {
					clauses[id] = map[string]bool{}
				}
				clauses[id][k] = true
			}
		}
	}
	out := []SourceInfo{}
	for i := range c.Sources {
		s := &c.Sources[i]
		info := SourceInfo{ID: s.ID, Kind: s.Kind, Short: s.Short, Name: s.Name.Get(lang), Title: s.Name.Get(lang)}
		if l := ls[s.ID]; l != nil {
			info.Shipped, info.Controls = true, len(l.ids)
			info.Title = l.Title.Get(lang)
		} else {
			info.Controls = len(clauses[s.ID])
		}
		if info.Controls > 0 {
			out = append(out, info)
		}
	}
	// shipped lists first: they are the complete statements
	sort.SliceStable(out, func(i, j int) bool { return out[i].Shipped && !out[j].Shipped })
	return out, nil
}

// Requirement is a project requirement as the SoA sees it.
type Requirement struct {
	ID, Key, Title, Status string
	// CatalogRef: the catalogue id it was imported from ("" none).
	CatalogRef string
	// Source: its free-text source field.
	Source string
	// Tests: the linked tests with their latest results ("" = not run).
	Tests []Evidence
	// Waiver: keys of the waivers in force for it ("" none).
	Waiver string
}

// Evidence is a test that verifies a requirement of a control.
type Evidence struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Title  string `json:"title"`
	Result string `json:"result"`
}

// Input is what Build works on.
type Input struct {
	Catalog      *catalog.Catalog
	SourceID     string
	Lang         string
	Requirements []Requirement
	// Decisions by control id.
	Decisions map[string]model.SoADecision
}

// RowRequirement is a requirement counted for a control.
type RowRequirement struct {
	ID         string `json:"id"`
	Key        string `json:"key"`
	Title      string `json:"title"`
	State      string `json:"state"`
	CatalogRef string `json:"catalog_ref,omitempty"`
	Waiver     string `json:"waiver,omitempty"`
}

// Row is one control of the statement.
type Row struct {
	Control
	// Applicable: nil = not decided.
	Applicable    *bool      `json:"applicable"`
	Justification string     `json:"justification"`
	Owner         string     `json:"owner"`
	UpdatedBy     string     `json:"updated_by,omitempty"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
	// Status: the implementation state ("" for an excluded control).
	Status       string           `json:"status"`
	Requirements []RowRequirement `json:"requirements"`
	Evidence     []Evidence       `json:"evidence"`
	// Catalogue: catalogue requirements that cite the control; Missing:
	// those the project has not adopted.
	Catalogue int `json:"catalogue"`
	Missing   int `json:"missing"`
	// Suggested: the applicability the data suggests for an undecided
	// control (requirements are adopted for it).
	Suggested bool `json:"suggested"`
}

// Summary counts the controls.
type Summary struct {
	Controls       int `json:"controls"`
	Applicable     int `json:"applicable"`
	Excluded       int `json:"excluded"`
	Undecided      int `json:"undecided"`
	Implemented    int `json:"implemented"`
	Partial        int `json:"partial"`
	Planned        int `json:"planned"`
	NotImplemented int `json:"not_implemented"`
}

// Statement is the Statement of Applicability of a project for a source.
type Statement struct {
	Source  SourceInfo `json:"source"`
	URL     string     `json:"url,omitempty"`
	Summary Summary    `json:"summary"`
	Rows    []Row      `json:"rows"`
}

func inactive(status string) bool { return status == "rejected" || status == "obsolete" }

// stateOf is the fulfilment state of a requirement from its tests.
func stateOf(tests []Evidence) string {
	if len(tests) == 0 {
		return StateUntested
	}
	failed, blocked, passed := false, false, 0
	for _, t := range tests {
		switch t.Result {
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
	case passed == len(tests):
		return StatePassed
	}
	return StateUntested
}

// StatusOf derives the implementation state of a control from the states
// of its requirements and whether they are waived.
func StatusOf(reqs []RowRequirement) string {
	if len(reqs) == 0 {
		return NotImplemented
	}
	passed, waived := 0, 0
	for _, r := range reqs {
		switch {
		case r.State == StatePassed:
			passed++
		case r.Waiver != "":
			waived++
		}
	}
	switch {
	case passed == len(reqs):
		return Implemented
	case passed > 0 || waived > 0:
		return Partial
	}
	return Planned
}

func splitSource(text string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(text, func(r rune) bool { return r == ';' || r == '\n' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Build makes the statement.
func Build(in Input) (*Statement, error) {
	c := in.Catalog
	if c == nil {
		return nil, fmt.Errorf("no catalogue")
	}
	m, err := mapperFor(c, in.SourceID)
	if err != nil {
		return nil, err
	}
	lang := catalog.Lang(in.Lang)
	var own []string
	for _, r := range in.Requirements {
		if r.Source != "" && !inactive(r.Status) {
			own = append(own, r.Source)
		}
	}
	controls, err := Controls(c, in.SourceID, lang, own...)
	if err != nil {
		return nil, err
	}
	st := &Statement{Rows: []Row{}, URL: m.src.URL}
	st.Source = SourceInfo{ID: m.src.ID, Kind: m.src.Kind, Short: m.src.Short, Name: m.src.Name.Get(lang), Title: m.src.Name.Get(lang), Shipped: m.list != nil, Controls: len(controls)}
	if m.list != nil {
		st.Source.Title = m.list.Title.Get(lang)
	}
	// the controls of a ref list of the source
	controlsOf := func(refs []string) []string {
		var out []string
		for _, ref := range refs {
			if c.RefSource(ref) != in.SourceID && !slices.Contains(c.SourcesOfText(ref), in.SourceID) {
				continue
			}
			if k := m.of(ref); k != "" && !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
		return out
	}
	// catalogue requirements per control and the adopted ones
	adopted := map[string]bool{}
	for _, r := range in.Requirements {
		if r.CatalogRef != "" && !inactive(r.Status) {
			adopted[r.CatalogRef] = true
		}
	}
	catalogue, missing := map[string]int{}, map[string]int{}
	for _, rq := range c.Requirements {
		for _, k := range controlsOf(rq.Refs) {
			catalogue[k]++
			if !adopted[rq.ID] {
				missing[k]++
			}
		}
	}
	// project requirements per control
	reqs := map[string][]RowRequirement{}
	evidence := map[string][]Evidence{}
	for _, r := range in.Requirements {
		if inactive(r.Status) {
			continue
		}
		var ks []string
		if rq := c.Requirements[r.CatalogRef]; rq != nil {
			ks = controlsOf(rq.Refs)
		}
		for _, k := range controlsOf(splitSource(r.Source)) {
			if !slices.Contains(ks, k) {
				ks = append(ks, k)
			}
		}
		rr := RowRequirement{ID: r.ID, Key: r.Key, Title: r.Title, State: stateOf(r.Tests), CatalogRef: r.CatalogRef, Waiver: r.Waiver}
		for _, k := range ks {
			reqs[k] = append(reqs[k], rr)
			for _, t := range r.Tests {
				if !slices.ContainsFunc(evidence[k], func(e Evidence) bool { return e.ID == t.ID }) {
					evidence[k] = append(evidence[k], t)
				}
			}
		}
	}
	for _, ctl := range controls {
		row := Row{Control: ctl, Requirements: reqs[ctl.ID], Evidence: evidence[ctl.ID], Catalogue: catalogue[ctl.ID], Missing: missing[ctl.ID]}
		if row.Requirements == nil {
			row.Requirements = []RowRequirement{}
		}
		if row.Evidence == nil {
			row.Evidence = []Evidence{}
		}
		sort.Slice(row.Requirements, func(i, j int) bool { return naturalLess(row.Requirements[i].Key, row.Requirements[j].Key) })
		sort.Slice(row.Evidence, func(i, j int) bool { return naturalLess(row.Evidence[i].Key, row.Evidence[j].Key) })
		if d, ok := in.Decisions[ctl.ID]; ok {
			row.Applicable, row.Justification, row.Owner, row.UpdatedBy = d.Applicable, d.Justification, d.Owner, d.UpdatedBy
			if !d.UpdatedAt.IsZero() {
				at := d.UpdatedAt
				row.UpdatedAt = &at
			}
		}
		st.Summary.Controls++
		switch {
		case row.Applicable == nil:
			st.Summary.Undecided++
			row.Suggested = len(row.Requirements) > 0
			row.Status = StatusOf(row.Requirements)
		case *row.Applicable:
			st.Summary.Applicable++
			row.Status = StatusOf(row.Requirements)
			switch row.Status {
			case Implemented:
				st.Summary.Implemented++
			case Partial:
				st.Summary.Partial++
			case Planned:
				st.Summary.Planned++
			default:
				st.Summary.NotImplemented++
			}
		default:
			st.Summary.Excluded++
		}
		st.Rows = append(st.Rows, row)
	}
	return st, nil
}

// HasControl reports whether the controls of a source (as Controls returns
// them) include id.
func HasControl(controls []Control, id string) bool {
	return slices.ContainsFunc(controls, func(c Control) bool { return c.ID == id })
}

// naturalLess compares with digit runs compared by value ("A.5.2" < "A.5.10").
func naturalLess(a, b string) bool {
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
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			a, b = a[i:], b[j:]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}
