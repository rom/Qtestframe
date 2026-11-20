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
      expected: e
      steps: [{action: a, expected: ""}]
`)},
	}
	_, err := Load(fsys, "d")
	if err == nil {
		t.Fatal("no error")
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

func TestNamesFallBackToEnglish(t *testing.T) {
	n := Names{"en": "Security", "sv": "Säkerhet"}
	if n.Get("sv") != "Säkerhet" || n.Get("de") != "Security" {
		t.Fatal(n)
	}
}
