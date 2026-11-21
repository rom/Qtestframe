package catalog

import (
	"strings"
	"testing"
	"testing/fstest"
)

// The built-in library loads and passes every check (format, languages,
// obligation keywords, requirement analysis, cross references).
func TestDefaultCatalogue(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Areas) == 0 || len(c.Requirements) == 0 || len(c.Tests) == 0 || len(c.Groups) == 0 {
		t.Fatalf("empty catalogue: %d areas, %d requirements, %d tests", len(c.Areas), len(c.Requirements), len(c.Tests))
	}
	for id, tc := range c.Tests {
		if len(tc.Requirements) == 0 {
			t.Errorf("%s verifies no catalogue requirement (list it in a requirement's tests)", id)
		}
	}
	t.Logf("%d areas, %d requirements, %d tests", len(c.Areas), len(c.Requirements), len(c.Tests))
}

func TestLoadReportsProblems(t *testing.T) {
	fsys := fstest.MapFS{
		"d/groups.yaml": {Data: []byte("groups:\n  - id: security\n    name: {en: S, sv: S}\n")},
		"d/x.yaml": {Data: []byte(`area: x
prefix: X
group: nope
name: {en: X}
description: {en: X, sv: X}
requirements:
  - id: Y-1
    type: wish
    obligation: must
    priority: urgent
    tags: [Bad Tag]
    tests: [X-T99]
    en:
      title: Fast.
      description: The system should be fast etc.
      rationale: r
      criteria: []
    de:
      title: x
tests:
  - id: X-T01
    priority: high
    execution: sometimes
    en:
      title: t
      description: d
      preconditions: p
      expected: e
      steps: [{action: a, expected: ""}]
`)},
	}
	_, err := Load(fsys, "d")
	if err == nil {
		t.Fatal("no error")
	}
	if !strings.Contains(err.Error(), "X-T01: verifies no requirement") {
		t.Errorf("unreferenced test accepted:\n%s", err)
	}
	for _, want := range []string{"unknown group", "prefix", "needed in sv", "id must be X-NN", "unknown type", "unknown priority", "tag", "full stop", "criterion", "keyword means", "unknown field or language \"de\"", "execution must be", "step 1", "unknown test X-T99"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%s", want, err)
		}
	}
	if _, err := Load(fstest.MapFS{"d/bad.yaml": {Data: []byte("area: [")}}, "d"); err == nil {
		t.Fatal("bad yaml accepted")
	}
	if _, err := Load(fstest.MapFS{"d/x.yaml": {Data: []byte("area: x\nunknown: 1\n")}}, "d"); err == nil {
		t.Fatal("unknown field accepted")
	}
}

// Every item is complete in every language: the same criteria (count,
// methods, thresholds) and steps (count, data) as English, and no text
// left empty that English has. Every requirement lists a test.
func TestLoadReportsIncompleteTranslations(t *testing.T) {
	fsys := fstest.MapFS{
		"d/groups.yaml": {Data: []byte("groups:\n  - id: security\n    name: {en: S, sv: S}\n")},
		"d/x.yaml": {Data: []byte(`area: x
prefix: X
group: security
name: {en: X, sv: X}
description: {en: X, sv: X}
requirements:
  - id: X-01
    type: security
    obligation: must
    priority: high
    tests: [X-T01]
    en:
      title: One
      description: The system MUST log in within 2 seconds.
      rationale: r
      criteria:
        - {method: test, text: t, threshold: "≤ 2 s"}
        - {method: review, text: t}
    sv:
      title: Ett
      description: Systemet SKA logga in inom 2 sekunder.
      rationale: r
      criteria:
        - {method: inspection, text: t}
  - id: X-02
    type: security
    obligation: must
    priority: high
    tests: [X-T01]
    en:
      title: Two
      description: The system MUST log in within 2 seconds.
      rationale: r
      criteria: [{method: test, text: t, threshold: "≤ 2 s"}]
    sv:
      title: Två
      description: Systemet SKA logga in inom 2 sekunder.
      rationale: r
      criteria: [{method: inspection, text: t}]
  - id: X-03
    type: security
    obligation: must
    priority: high
    en:
      title: Three
      description: The system MUST log in within 2 seconds.
      rationale: r
      criteria: [{method: test, text: t}]
    sv:
      title: Tre
      description: Systemet SKA logga in inom 2 sekunder.
      rationale: r
      criteria: [{method: test, text: t}]
tests:
  - id: X-T01
    priority: high
    execution: manual
    en:
      title: t
      description: d
      preconditions: p
      expected: e
      steps: [{action: a, expected: e, data: "x"}]
    sv:
      title: t
      description: d
      preconditions: ""
      expected: e
      steps: [{action: a, expected: e}]
`)},
	}
	_, err := Load(fsys, "d")
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{
		"X-01: en and sv must have the same number of criteria",
		"X-02 (sv): criterion 1 has method \"inspection\"",
		"X-02 (sv): criterion 1 needs the threshold",
		"X-03: lists no test",
		"X-T01: title, description, preconditions and expected are needed in sv",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%s", want, err)
		}
	}
}

func TestNamesFallBackToEnglish(t *testing.T) {
	n := Names{"en": "Security", "sv": "Säkerhet"}
	if n.Get("sv") != "Säkerhet" || n.Get("de") != "Security" {
		t.Fatal(n)
	}
}

// Every group has areas, and the library stays large: a change that drops
// whole areas by mistake fails here.
func TestLibraryCoverage(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	used := map[string]int{}
	for _, a := range c.Areas {
		used[a.Group]++
		if len(a.Requirements) < 5 || len(a.Tests) < 5 {
			t.Errorf("area %s has only %d requirements and %d tests", a.ID, len(a.Requirements), len(a.Tests))
		}
	}
	for _, g := range c.Groups {
		if used[g.ID] == 0 {
			t.Errorf("group %s has no areas", g.ID)
		}
	}
	if len(c.Requirements) < 500 || len(c.Tests) < 400 {
		t.Errorf("the library shrank: %d requirements, %d tests", len(c.Requirements), len(c.Tests))
	}
}
