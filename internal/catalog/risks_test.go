package catalog

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// riskFS is a small catalogue with two areas: X has two requirements and
// a risk mitigated by a requirement of each area.
func riskFS(risks string) fstest.MapFS {
	req := func(id, test string) string {
		return `  - id: ` + id + `
    type: security
    obligation: must
    priority: high
    tests: [` + test + `]
    refs: ["CWE-89 (SQL injection)"]
    en:
      title: One
      description: The system MUST log in within 2 seconds.
      rationale: r
      criteria: [{method: test, text: t}]
    sv:
      title: Ett
      description: Systemet SKA logga in inom 2 sekunder.
      rationale: r
      criteria: [{method: test, text: t}]
`
	}
	test := func(id string) string {
		return `  - id: ` + id + `
    priority: high
    execution: manual
    en: {title: t, description: d, preconditions: p, expected: e, steps: [{action: a, expected: e}]}
    sv: {title: t, description: d, preconditions: p, expected: e, steps: [{action: a, expected: e}]}
`
	}
	area := func(id, prefix, body string) string {
		return "area: " + id + "\nprefix: " + prefix + "\ngroup: security\nname: {en: " + id + ", sv: " + id + "}\ndescription: {en: d, sv: d}\n" + body
	}
	return fstest.MapFS{
		"d/groups.yaml":  {Data: []byte("groups:\n  - id: security\n    name: {en: S, sv: S}\n")},
		"d/sources.yaml": {Data: []byte("kinds:\n  - id: standard\n    name: {en: S, sv: S}\nsources:\n  - id: cwe\n    kind: standard\n    name: {en: CWE, sv: CWE}\n    match: [\"CWE-\"]\n  - id: iso-31000\n    kind: standard\n    name: {en: ISO 31000, sv: ISO 31000}\n    match: [\"ISO 31000\"]\n")},
		"d/attack.yaml":  {Data: []byte(testAttack)},
		"d/x.yaml":       {Data: []byte(area("x", "XA", "requirements:\n"+req("XA-01", "XA-T01")+req("XA-02", "XA-T01")+"tests:\n"+test("XA-T01")+risks))},
		"d/y.yaml":       {Data: []byte(area("y", "YB", "requirements:\n"+req("YB-01", "YB-T01")+"tests:\n"+test("YB-T01")))},
	}
}

// testAttack is a small ATT&CK and ATLAS list.
const testAttack = `matrices:
  - {id: enterprise, name: Enterprise, version: "20.1", url: https://attack.mitre.org/matrices/enterprise}
  - {id: atlas, name: ATLAS, version: "5", url: https://atlas.mitre.org/matrices/ATLAS}
tactics:
  - {id: TA0001, name: Initial Access, matrix: enterprise, url: https://attack.mitre.org/tactics/TA0001}
  - {id: TA0006, name: Credential Access, matrix: enterprise, url: https://attack.mitre.org/tactics/TA0006}
  - {id: AML.TA0005, name: Execution, matrix: atlas, url: https://atlas.mitre.org/tactics/AML.TA0005}
techniques:
  - {id: T1190, name: Exploit Public-Facing Application, matrix: enterprise, tactics: [TA0001], url: https://attack.mitre.org/techniques/T1190}
  - {id: T1110, name: Brute Force, matrix: enterprise, tactics: [TA0006], url: https://attack.mitre.org/techniques/T1110}
  - {id: T1110.004, name: Credential Stuffing, matrix: enterprise, tactics: [TA0006], url: https://attack.mitre.org/techniques/T1110/004}
  - {id: AML.T0051, name: LLM Prompt Injection, matrix: atlas, tactics: [AML.TA0005], url: https://atlas.mitre.org/techniques/AML.T0051}
  - {id: AML.T0051.000, name: Direct, matrix: atlas, tactics: [AML.TA0005], url: https://atlas.mitre.org/techniques/AML.T0051.000}
mitigations:
  - {id: M1032, name: Multi-factor Authentication, matrix: enterprise, url: https://attack.mitre.org/mitigations/M1032}
`

const goodRisk = `risks:
  - id: XA-R01
    category: security
    likelihood: 4
    impact: 4
    tags: [injection]
    refs: ["ISO 31000:2018 §6.4.2 (risk identification)", "CWE-89 (SQL injection)"]
    mitigated_by: [XA-01, YB-01]
    attack: [T1190]
    tests: [YB-T01]
    en:
      title: Data stolen through injection
      description: An attacker injects a query and reads all data.
      mitigation: Use parameterised queries.
    sv:
      title: Data stjäls genom injektion
      description: En angripare injicerar en fråga och läser all data.
      mitigation: Använd parametriserade frågor.
`

func TestRisksLoadAndLink(t *testing.T) {
	c, err := Load(riskFS(goodRisk), "d")
	if err != nil {
		t.Fatal(err)
	}
	k := c.Risks["XA-R01"]
	if k == nil || k.Area != "x" || k.Level() != "high" || !slices.Equal(k.Sources, []string{"cwe", "iso-31000"}) {
		t.Fatalf("risk %+v", k)
	}
	if !slices.Equal(c.RisksOf["YB-01"], []string{"XA-R01"}) || len(c.RisksOf["XA-02"]) != 0 {
		t.Fatalf("reverse %v", c.RisksOf)
	}
	if s := c.SummarizeRequirement(c.Requirements["XA-01"], "sv"); !slices.Equal(s.Risks, []string{"XA-R01"}) {
		t.Fatalf("summary %+v", s)
	}
	rs := c.SummarizeRisk(k, "sv")
	if rs.Title != "Data stjäls genom injektion" || rs.CategoryName != "Informationssäkerhet" || rs.Score != 16 || rs.Mitigation == "" {
		t.Fatalf("risk summary %+v", rs)
	}
	ov := c.Overview("en")
	if ov[0].Risks != 1 || ov[0].Areas[0].Risks != 1 {
		t.Fatalf("overview %+v", ov)
	}
	if ks := c.KindsOverview("en"); ks[0].Risks != 1 {
		t.Fatalf("kinds %+v", ks)
	}
	if tags := c.Tags("risk"); len(tags) != 1 || tags[0].Tag != "injection" {
		t.Fatalf("tags %v", tags)
	}
	// sources: a risk counts for the sources it cites
	for _, s := range c.SourcesOverview("en", "", false) {
		if s.ID == "iso-31000" && (s.Risks != 1 || s.Requirements != 0) {
			t.Fatalf("source %+v", s)
		}
	}
	if !strings.Contains(c.SourcesMarkdown("en"), "0 requirements, 0 tests, 1 risk") {
		t.Fatalf("markdown:\n%s", c.SourcesMarkdown("en"))
	}
	if len(c.RefProblems()) != 0 {
		t.Fatal(c.RefProblems())
	}
}

func TestFindRisks(t *testing.T) {
	c, err := Load(riskFS(goodRisk), "d")
	if err != nil {
		t.Fatal(err)
	}
	ids := func(rs []RiskSummary) []string {
		var out []string
		for _, r := range rs {
			out = append(out, r.ID)
		}
		return out
	}
	for _, tc := range []struct {
		f    Filter
		want []string
	}{
		{Filter{}, []string{"XA-R01"}},
		{Filter{Area: "y"}, nil},
		{Filter{Category: "security", Level: "high,critical"}, []string{"XA-R01"}},
		{Filter{Level: "low"}, nil},
		{Filter{Q: "injection"}, []string{"XA-R01"}},
		{Filter{Q: "YB-01"}, []string{"XA-R01"}},
		{Filter{Q: "nothing"}, nil},
		{Filter{Source: "cwe"}, []string{"XA-R01"}},
		{Filter{Tag: "nope"}, nil},
	} {
		if got := ids(c.FindRisks(tc.f, "en")); !slices.Equal(got, tc.want) {
			t.Errorf("%+v: %v, want %v", tc.f, got, tc.want)
		}
	}
	if sv := c.FindRisks(Filter{Q: "injektion"}, "sv"); len(sv) != 1 {
		t.Fatalf("sv search: %v", sv)
	}
	// Localized falls back to English per field
	k := c.Risks["XA-R01"]
	k.Text["sv"] = RiskText{Title: "Bara titel"}
	if x := k.Localized("sv"); x.Title != "Bara titel" || x.Mitigation != "Use parameterised queries." {
		t.Fatalf("localized %+v", x)
	}
}

func TestLoadReportsRiskProblems(t *testing.T) {
	bad := `risks:
  - id: YB-R01
    category: weather
    likelihood: 0
    impact: 6
    tags: [Bad Tag]
    refs: [" ISO 31000"]
    mitigated_by: [XA-99, XA-01, XA-01]
    tests: [XA-T99]
    en:
      title: Wrong.
      description: d
      mitigation: ""
    de:
      title: x
  - id: XA-R02
    category: privacy
    likelihood: 3
    impact: 3
    en: {title: t, description: d, mitigation: m}
    sv: {title: t, description: d, mitigation: m}
`
	_, err := Load(riskFS(bad), "d")
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{
		"YB-R01: id must be XA-RNN",
		"YB-R01: category must be one of",
		"YB-R01: likelihood and impact must be between 1 and 5",
		`YB-R01: tag "Bad Tag"`,
		"surrounding spaces",
		"YB-R01: unknown requirement XA-99 in mitigated_by",
		"YB-R01: requirement XA-01 listed twice",
		"YB-R01: unknown test XA-T99",
		"YB-R01: title, description and mitigation are needed in en",
		`YB-R01: unknown field or language "de"`,
		"XA-R02: lists no requirement that mitigates it",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%s", want, err)
		}
	}
	dup := strings.Replace(goodRisk, "risks:\n", "risks:\n"+strings.TrimPrefix(goodRisk, "risks:\n"), 1)
	if _, err := Load(riskFS(dup), "d"); err == nil || !strings.Contains(err.Error(), "XA-R01: duplicate id") {
		t.Fatalf("duplicate accepted: %v", err)
	}
	title := strings.Replace(goodRisk, "title: Data stolen through injection", "title: Data stolen.", 1)
	if _, err := Load(riskFS(title), "d"); err == nil || !strings.Contains(err.Error(), "no full stop") {
		t.Fatalf("full stop accepted: %v", err)
	}
	long := strings.Replace(goodRisk, "title: Data stolen through injection", "title: "+strings.Repeat("a", MaxRiskTitle+1), 1)
	if _, err := Load(riskFS(long), "d"); err == nil || !strings.Contains(err.Error(), "title longer") {
		t.Fatalf("long title accepted: %v", err)
	}
}

// The built-in risks: enough of them, complete in both languages (checked
// by Load), each mitigated by at least one requirement, and the areas
// where risks make sense have them.
func TestRiskLibrary(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Risks) < 500 {
		t.Errorf("only %d catalogue risks", len(c.Risks))
	}
	cats := map[string]int{}
	for _, k := range c.Risks {
		cats[k.Category]++
		if len(k.MitigatedBy) == 0 {
			t.Errorf("%s has no mitigating requirement", k.ID)
		}
		if RiskCategory(k.Category) == nil {
			t.Errorf("%s: category %s", k.ID, k.Category)
		}
	}
	for _, want := range []string{"security", "privacy", "safety", "availability", "ai", "supply-chain", "compliance"} {
		if cats[want] < 5 {
			t.Errorf("only %d %s risks", cats[want], want)
		}
	}
	for _, a := range c.Areas {
		if n := len(a.Risks); n < 3 || n > 8 {
			t.Errorf("area %s has %d risks: give every area 3 to 8", a.ID, n)
		}
	}
	for _, g := range c.Overview("en") {
		if (g.ID == "security" || g.ID == "privacy") && g.Risks < 30 {
			t.Errorf("group %s has only %d risks", g.ID, g.Risks)
		}
	}
	t.Logf("%d risks: %v", len(c.Risks), cats)
}
