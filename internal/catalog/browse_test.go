package catalog

import (
	"slices"
	"testing"
	"time"
)

func sample(t *testing.T, areas, n int) *Catalog {
	t.Helper()
	c, err := Load(SampleFS(areas, n), "data")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSampleLoads(t *testing.T) {
	c := sample(t, 12, 25)
	if len(c.Requirements) != 300 || len(c.Tests) != 300 || len(c.Areas) != 12 {
		t.Fatalf("%d requirements, %d tests, %d areas", len(c.Requirements), len(c.Tests), len(c.Areas))
	}
	if got := c.Tests["SAA-T02"].Requirements; len(got) != 2 {
		t.Fatalf("T02 verifies %v", got)
	}
}

func TestOverviewAndOrder(t *testing.T) {
	c := sample(t, 8, 3)
	ov := c.Overview("sv")
	if len(ov) != 6 || ov[0].ID != "security" || ov[0].Name != "Grupp security" {
		t.Fatalf("overview: %+v", ov)
	}
	// security has areas 1 and 7 (8 areas over 6 groups)
	if len(ov[0].Areas) != 2 || ov[0].Requirements != 6 || ov[0].Tests != 6 || ov[0].Areas[0].Name != "Exempelområde 1" {
		t.Fatalf("security: %+v", ov[0])
	}
	if c.OrderedAreas()[1].ID != "sample-7" {
		t.Fatalf("order: %s", c.OrderedAreas()[1].ID)
	}
	if c.Area("nope") != nil || c.Group("nope") != nil || c.Group("privacy") == nil {
		t.Fatal("lookups")
	}
	if Lang("de") != "en" || Lang("sv") != "sv" {
		t.Fatal("lang")
	}
	tags := c.Tags("requirement")
	if tags[0].Tag != "sample" || tags[0].Count != 24 {
		t.Fatalf("tags: %v", tags)
	}
	if tt := c.Tags("test"); len(tt) != 1 || tt[0].Count != 24 {
		t.Fatalf("test tags: %v", tt)
	}
}

func TestFindRequirements(t *testing.T) {
	c := sample(t, 4, 6)
	all := c.FindRequirements(Filter{}, "en")
	if len(all) != 24 || all[0].ID != "SAA-01" {
		t.Fatalf("all: %d %v", len(all), all[0].ID)
	}
	if r := c.FindRequirements(Filter{Area: "sample-2"}, "en"); len(r) != 6 || r[0].Area != "sample-2" || r[0].Group != "privacy" {
		t.Fatalf("area: %v", r)
	}
	if r := c.FindRequirements(Filter{Group: "security,quality"}, "en"); len(r) != 12 {
		t.Fatalf("groups: %d", len(r))
	}
	if r := c.FindRequirements(Filter{Obligation: "must", Priority: "medium"}, "en"); len(r) == 0 {
		t.Fatal("no must/medium")
	} else {
		for _, x := range r {
			if x.Obligation != "must" || x.Priority != "medium" {
				t.Fatal(x)
			}
		}
	}
	if r := c.FindRequirements(Filter{Type: "performance", Tag: "TAG-1"}, "en"); len(r) == 0 {
		t.Fatal("type/tag")
	}
	// search: Swedish text, all words must match
	sv := c.FindRequirements(Filter{Q: "granskningspost 3"}, "sv")
	if len(sv) == 0 || sv[0].Title != "Granskningspost 3 i område 1" {
		t.Fatalf("sv search: %v", sv)
	}
	if r := c.FindRequirements(Filter{Q: "granskningspost nothing"}, "sv"); len(r) != 0 {
		t.Fatalf("AND search: %v", r)
	}
	// an exact id ranks first, title matches before description matches
	if r := c.FindRequirements(Filter{Q: "sac-04"}, "en"); r[0].ID != "SAC-04" {
		t.Fatalf("id: %v", r[0])
	}
	if r := c.FindRequirements(Filter{Q: "area 3"}, "en"); r[0].Area != "sample-3" {
		t.Fatalf("rank: %v", r[0])
	}
	if r := c.FindRequirements(Filter{Q: "25010"}, "en"); len(r) != 24 {
		t.Fatalf("refs: %d", len(r))
	}
}

// A search for a CWE or ATT&CK id finds the items that cite it in refs.
func TestFindByWeaknessAndTechnique(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for q, want := range map[string]string{"CWE-79": "INP-05", "cwe-639": "ACC-02", "T1110": "AUTH-09", "T1110.001": "AUTH-09", "M0930": "ICS-01"} {
		r := c.FindRequirements(Filter{Q: q}, "sv")
		if !slices.ContainsFunc(r, func(s ReqSummary) bool { return s.ID == want }) {
			t.Errorf("%s: %s not found", q, want)
		}
	}
	if r := c.FindTests(Filter{Q: "T1110.001"}, "en"); !slices.ContainsFunc(r, func(s TestSummary) bool { return s.ID == "AUTH-T09" }) {
		t.Errorf("T1110.001: AUTH-T09 not found in %v", r)
	}
}

func TestFindTests(t *testing.T) {
	c := sample(t, 3, 4)
	if r := c.FindTests(Filter{}, "sv"); len(r) != 12 || r[0].Title != "Granskningspost 1 skrivs" || r[0].Steps != 1 {
		t.Fatalf("all: %v", r)
	}
	if r := c.FindTests(Filter{Execution: "automated"}, "en"); len(r) != 6 {
		t.Fatalf("automated: %d", len(r))
	}
	if r := c.FindTests(Filter{Q: "sab-01"}, "en"); len(r) == 0 || r[0].ID != "SAB-T01" {
		t.Fatalf("by requirement: %v", r)
	}
	if r := c.FindTests(Filter{Q: "event-3", Area: "sample-1", Priority: "critical,high,medium,low", Tag: "sample"}, "en"); len(r) != 1 {
		t.Fatalf("steps: %v", r)
	}
}

func TestLocalizedFallsBack(t *testing.T) {
	r := &Requirement{Text: map[string]ReqText{"en": {Title: "T", Description: "D", Rationale: "R", Criteria: []Criterion{{Method: "test", Text: "C"}}}, "sv": {Title: "S"}}}
	if x := r.Localized("sv"); x.Title != "S" || x.Description != "D" || x.Rationale != "R" || len(x.Criteria) != 1 {
		t.Fatal(x)
	}
	ts := &Test{Text: map[string]TestText{"en": {Title: "T", Description: "D", Preconditions: "P", Expected: "E", Steps: []Step{{Action: "a", Expected: "e"}}}}}
	if x := ts.Localized("sv"); x.Title != "T" || x.Preconditions != "P" || x.Expected != "E" || x.Description != "D" || len(x.Steps) != 1 {
		t.Fatal(x)
	}
	c := &Catalog{}
	if s := c.SummarizeRequirement(r, "sv"); s.Group != "" || s.Tags == nil {
		t.Fatal(s)
	}
	if s := c.SummarizeTest(ts, "en"); s.Group != "" || s.Requirements == nil {
		t.Fatal(s)
	}
}

// Searching several hundred items stays fast.
func TestSearchScales(t *testing.T) {
	c := sample(t, 20, 25) // 500 requirements, 500 tests
	start := time.Now()
	for i := 0; i < 20; i++ {
		c.FindRequirements(Filter{Q: "audit record 7 area"}, "sv")
		c.FindTests(Filter{Q: "record written"}, "en")
	}
	if d := time.Since(start) / 20; d > 50*time.Millisecond {
		t.Fatalf("search takes %v", d)
	}
}
