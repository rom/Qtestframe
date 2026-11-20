// Package catalog is the built-in library of standard requirements and
// tests for software development (data/*.yaml, see FORMAT.md). Projects
// copy items from it into their own requirements and tests.
package catalog

import (
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/reqlang"
)

//go:embed data/*.yaml
var dataFS embed.FS

// Languages the catalogue is written in; English is the fallback.
var Languages = []string{"en", "sv"}

// Names is a text in each language.
type Names map[string]string

// Get returns the text in lang, else in English.
func (n Names) Get(lang string) string {
	if v := n[lang]; v != "" {
		return v
	}
	return n["en"]
}

// Group is a heading for several areas.
type Group struct {
	ID   string `yaml:"id" json:"id"`
	Name Names  `yaml:"name" json:"name"`
}

// Criterion is an evaluation criterion of a requirement.
type Criterion struct {
	Method    string `yaml:"method" json:"method"`
	Text      string `yaml:"text" json:"text"`
	Threshold string `yaml:"threshold" json:"threshold,omitempty"`
}

// ReqText is the language-specific part of a requirement.
type ReqText struct {
	Title       string      `yaml:"title" json:"title"`
	Description string      `yaml:"description" json:"description"`
	Rationale   string      `yaml:"rationale" json:"rationale"`
	Criteria    []Criterion `yaml:"criteria" json:"criteria"`
}

// Requirement is a standard requirement.
type Requirement struct {
	ID         string             `yaml:"id" json:"id"`
	Area       string             `yaml:"-" json:"area"`
	Type       string             `yaml:"type" json:"type"`
	Obligation string             `yaml:"obligation" json:"obligation"`
	Priority   string             `yaml:"priority" json:"priority"`
	Tags       []string           `yaml:"tags" json:"tags"`
	Refs       []string           `yaml:"refs" json:"refs"`
	Tests      []string           `yaml:"tests" json:"tests"`
	Text       map[string]ReqText `yaml:",inline" json:"text"`
}

// Step is a test step.
type Step struct {
	Action   string `yaml:"action" json:"action"`
	Expected string `yaml:"expected" json:"expected"`
	Data     string `yaml:"data" json:"data,omitempty"`
}

// TestText is the language-specific part of a test.
type TestText struct {
	Title         string `yaml:"title" json:"title"`
	Description   string `yaml:"description" json:"description"`
	Preconditions string `yaml:"preconditions" json:"preconditions"`
	Expected      string `yaml:"expected" json:"expected"`
	Steps         []Step `yaml:"steps" json:"steps"`
}

// Test is a standard test.
type Test struct {
	ID        string              `yaml:"id" json:"id"`
	Area      string              `yaml:"-" json:"area"`
	Priority  string              `yaml:"priority" json:"priority"`
	Execution string              `yaml:"execution" json:"execution"`
	Tags      []string            `yaml:"tags" json:"tags"`
	Refs      []string            `yaml:"refs" json:"refs"`
	Text      map[string]TestText `yaml:",inline" json:"text"`
	// Requirements are the catalogue requirements this test verifies
	// (derived from the requirements' tests lists).
	Requirements []string `yaml:"-" json:"requirements"`
}

// Area is a topic with its requirements and tests.
type Area struct {
	ID           string        `yaml:"area" json:"id"`
	Prefix       string        `yaml:"prefix" json:"prefix"`
	Group        string        `yaml:"group" json:"group"`
	Name         Names         `yaml:"name" json:"name"`
	Description  Names         `yaml:"description" json:"description"`
	Requirements []Requirement `yaml:"requirements" json:"-"`
	Tests        []Test        `yaml:"tests" json:"-"`
}

// Catalog is the loaded library.
type Catalog struct {
	Groups       []Group
	Areas        []Area
	Requirements map[string]*Requirement
	Tests        map[string]*Test
}

var (
	once    sync.Once
	loaded  *Catalog
	errLoad error
)

// Default returns the built-in catalogue (loaded once).
func Default() (*Catalog, error) {
	once.Do(func() { loaded, errLoad = Load(dataFS, "data") })
	return loaded, errLoad
}

var (
	areaRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	prefixRe = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,5}$`)
	tagRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
)

// Load reads and checks the catalogue in dir of fsys.
func Load(fsys fs.FS, dir string) (*Catalog, error) {
	c := &Catalog{Requirements: map[string]*Requirement{}, Tests: map[string]*Test{}}
	names, err := fs.Glob(fsys, dir+"/*.yaml")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	groups := map[string]bool{}
	for _, n := range names {
		b, err := fs.ReadFile(fsys, n)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(n, "/groups.yaml") {
			var g struct {
				Groups []Group `yaml:"groups"`
			}
			if err := strictYAML(b, &g); err != nil {
				return nil, fmt.Errorf("%s: %w", n, err)
			}
			c.Groups = g.Groups
			for _, x := range g.Groups {
				groups[x.ID] = true
			}
			continue
		}
		var a Area
		if err := strictYAML(b, &a); err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		c.Areas = append(c.Areas, a)
	}
	var errs []string
	bad := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
	areaIDs, prefixes := map[string]bool{}, map[string]bool{}
	for i := range c.Areas {
		a := &c.Areas[i]
		if !areaRe.MatchString(a.ID) || areaIDs[a.ID] {
			bad("area %q: invalid or duplicate id", a.ID)
		}
		areaIDs[a.ID] = true
		if !prefixRe.MatchString(a.Prefix) || prefixes[a.Prefix] {
			bad("area %s: invalid or duplicate prefix %q", a.ID, a.Prefix)
		}
		prefixes[a.Prefix] = true
		if !groups[a.Group] {
			bad("area %s: unknown group %q", a.ID, a.Group)
		}
		for _, l := range Languages {
			if strings.TrimSpace(a.Name[l]) == "" || strings.TrimSpace(a.Description[l]) == "" {
				bad("area %s: name and description are needed in %s", a.ID, l)
			}
		}
		reqRe := regexp.MustCompile(`^` + a.Prefix + `-\d{2,3}$`)
		testRe := regexp.MustCompile(`^` + a.Prefix + `-T\d{2,3}$`)
		for j := range a.Requirements {
			r := &a.Requirements[j]
			r.Area = a.ID
			if !reqRe.MatchString(r.ID) {
				bad("%s: id must be %s-NN", r.ID, a.Prefix)
			}
			if c.Requirements[r.ID] != nil {
				bad("%s: duplicate id", r.ID)
			}
			c.Requirements[r.ID] = r
			checkRequirement(r, bad)
		}
		for j := range a.Tests {
			t := &a.Tests[j]
			t.Area = a.ID
			if !testRe.MatchString(t.ID) {
				bad("%s: id must be %s-TNN", t.ID, a.Prefix)
			}
			if c.Tests[t.ID] != nil {
				bad("%s: duplicate id", t.ID)
			}
			c.Tests[t.ID] = t
			checkTest(t, bad)
		}
	}
	// requirement → tests; the reverse is derived
	for _, a := range c.Areas {
		for _, r := range a.Requirements {
			for _, tid := range r.Tests {
				t := c.Tests[tid]
				if t == nil {
					bad("%s: unknown test %s", r.ID, tid)
					continue
				}
				if !slices.Contains(t.Requirements, r.ID) {
					t.Requirements = append(t.Requirements, r.ID)
				}
			}
		}
	}
	for _, t := range c.Tests {
		sort.Strings(t.Requirements)
	}
	if len(errs) > 0 {
		sort.Strings(errs)
		return nil, fmt.Errorf("catalogue: %d problem(s):\n  %s", len(errs), strings.Join(errs, "\n  "))
	}
	return c, nil
}

func strictYAML(b []byte, v any) error {
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	return dec.Decode(v)
}

// keyword levels that count as the same obligation
var sameLevel = map[string]string{reqlang.LevelShall: reqlang.LevelMust, reqlang.LevelShallNot: reqlang.LevelMustNot}

func level(l string) string {
	if s, ok := sameLevel[l]; ok {
		return s
	}
	return l
}

func checkTags(id string, tags []string, bad func(string, ...any)) {
	for _, t := range tags {
		if !tagRe.MatchString(t) {
			bad("%s: tag %q must be lower case a-z, 0-9 and -", id, t)
		}
	}
}

func checkRequirement(r *Requirement, bad func(string, ...any)) {
	if !slices.Contains(model.RequirementTypes, r.Type) {
		bad("%s: unknown type %q", r.ID, r.Type)
	}
	if !slices.Contains([]string{"must", "must_not", "should", "should_not", "may"}, r.Obligation) {
		bad("%s: obligation must be must, must_not, should, should_not or may", r.ID)
	}
	if !slices.Contains(model.Priorities, r.Priority) {
		bad("%s: unknown priority %q", r.ID, r.Priority)
	}
	checkTags(r.ID, r.Tags, bad)
	for _, l := range Languages {
		x, ok := r.Text[l]
		if !ok || strings.TrimSpace(x.Title) == "" || strings.TrimSpace(x.Description) == "" || strings.TrimSpace(x.Rationale) == "" {
			bad("%s: title, description and rationale are needed in %s", r.ID, l)
			continue
		}
		if strings.HasSuffix(x.Title, ".") {
			bad("%s (%s): no full stop at the end of the title", r.ID, l)
		}
		if len(x.Criteria) == 0 {
			bad("%s (%s): at least one evaluation criterion", r.ID, l)
		}
		for _, cr := range x.Criteria {
			if !slices.Contains(model.EvaluationMethods, cr.Method) || strings.TrimSpace(cr.Text) == "" {
				bad("%s (%s): a criterion needs a method (%s) and a text", r.ID, l, strings.Join(model.EvaluationMethods, ", "))
			}
		}
		an := reqlang.AnalyzeLang(x.Description, l)
		if level(an.Obligation) != r.Obligation {
			bad("%s (%s): the description's keyword means %q, but obligation is %q", r.ID, l, an.Obligation, r.Obligation)
		}
		for _, is := range an.Issues {
			if is.Severity == "error" || is.Severity == "warning" {
				bad("%s (%s): %s: %s", r.ID, l, is.Rule, is.Message)
			}
		}
	}
	for l := range r.Text {
		if !slices.Contains(Languages, l) {
			bad("%s: unknown field or language %q", r.ID, l)
		}
	}
}

func checkTest(t *Test, bad func(string, ...any)) {
	if !slices.Contains(model.Priorities, t.Priority) {
		bad("%s: unknown priority %q", t.ID, t.Priority)
	}
	if t.Execution != model.ExecManual && t.Execution != model.ExecAutomated {
		bad("%s: execution must be manual or automated", t.ID)
	}
	checkTags(t.ID, t.Tags, bad)
	for _, l := range Languages {
		x, ok := t.Text[l]
		if !ok || strings.TrimSpace(x.Title) == "" || strings.TrimSpace(x.Description) == "" || strings.TrimSpace(x.Expected) == "" {
			bad("%s: title, description and expected are needed in %s", t.ID, l)
			continue
		}
		if strings.HasSuffix(x.Title, ".") {
			bad("%s (%s): no full stop at the end of the title", t.ID, l)
		}
		if len(x.Steps) == 0 {
			bad("%s (%s): at least one step", t.ID, l)
		}
		for i, s := range x.Steps {
			if strings.TrimSpace(s.Action) == "" || strings.TrimSpace(s.Expected) == "" {
				bad("%s (%s): step %d needs an action and an expected result", t.ID, l, i+1)
			}
		}
		if en, sv := t.Text["en"], t.Text["sv"]; l == "sv" && len(en.Steps) != len(sv.Steps) {
			bad("%s: en and sv must have the same number of steps", t.ID)
		}
	}
	for l := range t.Text {
		if !slices.Contains(Languages, l) {
			bad("%s: unknown field or language %q", t.ID, l)
		}
	}
}
