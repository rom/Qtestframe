package catalog

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// Every ref of the built-in catalogue matches exactly one reference
// source (data/sources.yaml). A new area that cites a new law or standard
// fails here until its source is added.
func TestSourcesCoverEveryRef(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Sources) == 0 || len(c.SourceKinds) == 0 {
		t.Fatal("no reference sources loaded")
	}
	for _, p := range c.RefProblems() {
		t.Error(p)
	}
}

// The sources the documentation promises are in the registry and resolve
// the spellings of FORMAT.md.
func TestSourcesResolveDocumentedSpellings(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for ref, want := range map[string]string{
		"GDPR Art. 32(1)(a)": "gdpr",
		"Regulation (EU) 2024/2847 (CRA) Annex I Part I §2(d)":     "cra",
		"ISO/IEC 27001:2022 A.8.5":                                 "iso-27001",
		"ISO/IEC 27002:2022 §8.5":                                  "iso-27002",
		"IEC 62443-3-3 SR 1.7":                                     "iec-62443-3-3",
		"IEC 62443-4-2 CR 1.7":                                     "iec-62443-4-2",
		"Directive (EU) 2022/2557 (CER) Art. 13(1)(d)":             "cer",
		"NIS2 Directive (EU) 2022/2555 Art. 23":                    "nis2",
		"Regulation (EU) 2022/2554 (DORA) Art. 9(4)(c)":            "dora",
		"Directive 2014/53/EU (RED) Art. 3(3)(d) (EN 18031-1 AUM)": "red",
		"Cybersäkerhetslag (Swedish NIS2 act)":                     "se-cybersakerhetslag",
		"Säkerhetsskyddsförordning (2021:955)":                     "se-sakerhetsskydd",
		"OWASP ASVS 4.0 V2.1.1":                                    "owasp-asvs",
		"WCAG 2.2 SC 1.4.3":                                        "wcag",
		"NIST SP 800-63B §5.1.1.2":                                 "nist-800-63",
		"OWASP Top 10 2021 A01":                                    "owasp-top-10",
		"OWASP Top 10 for LLM Applications 2025 LLM01":             "owasp-llm-top-10",
		"DORA State of DevOps research":                            "dora-devops",
	} {
		if got := c.RefSource(ref); got != want {
			t.Errorf("%s: source %q, want %q", ref, got, want)
		}
	}
	if c.Source("gdpr") == nil || c.Source("nope") != nil {
		t.Error("Source lookup")
	}
}

func TestClauseOf(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for ref, want := range map[string]string{
		"GDPR Art. 32(1)(a)": "Art. 32",
		"GDPR Recital 26":    "",
		"Regulation (EU) 2024/2847 (CRA) Annex I Part I §2(d)":      "Annex I Part I",
		"Regulation (EU) 2024/2847 (CRA) Art. 14(8)":                "Art. 14",
		"ISO/IEC 27001:2022 A.8.5":                                  "A.8",
		"ISO/IEC 27001:2022 §10.2 (nonconformity)":                  "§10",
		"IEC 62443-3-3:2013 SR 1.1 RE 2":                            "SR 1",
		"IEC 62443-4-1 SM-9":                                        "SM",
		"IEC 62443-2-4:2015 SP.06":                                  "SP 6",
		"OWASP ASVS 4.0 V2.1.2":                                     "V2",
		"OWASP ASVS 4.0.3 §12.1.2":                                  "V12",
		"PCI DSS 4.0 Req. 3.5":                                      "Req. 3",
		"Google SRE workbook ch. 5":                                 "Workbook ch. 5",
		"Nielsen heuristic 4":                                       "Heuristic 4",
		"GNU GPL v3 §6 (conveying non-source forms)":                "GNU GPL",
		"WCAG 2.2 SC 1.4.10":                                        "SC 1.4",
		"NIST SP 800-63B §5.1.1.2":                                  "§5",
		"NIST SP 800-53 AC-2(3)":                                    "AC",
		"Directive (EU) 2022/2555 (NIS2) Art. 21(2)(d), Art. 21(3)": "Art. 21",
		"Säkerhetsskyddslag (2018:585) 2 kap. 1 § (kontroll)":       "2 kap.",
		"Directive 2014/53/EU (RED) Art. 3(3)(e) (EN 18031-2 DLM)":  "Art. 3(3)(e)",
		"PCI DSS v4.0 Requirement 3.4.1 (PAN masked)":               "Req. 3",
		"RFC 9110 §15.5":                                            "RFC 9110",
		"OWASP Top 10 2021 A05 (security misconfiguration)":         "A05",
		"ISO/IEC 25010:2023 time behaviour":                         "2023 time behaviour",
		"OWASP File Upload Cheat Sheet (image rewriting)":           "OWASP File Upload Cheat Sheet",
		"Google SRE book ch. 6 (latency)":                           "Book ch. 6",
		"The Twelve-Factor App, III Config":                         "III",
	} {
		src := c.Source(c.RefSource(ref))
		if src == nil {
			t.Errorf("%s: no source", ref)
			continue
		}
		if got := src.ClauseOf(ref); got != want {
			t.Errorf("%s: clause %q, want %q", ref, got, want)
		}
	}
}

func TestItemSourcesAndFilter(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	reqs := c.FindRequirements(Filter{Source: "cra"}, "en")
	if len(reqs) == 0 {
		t.Fatal("no CRA requirements")
	}
	for _, r := range reqs {
		if !slices.Contains(r.Sources, "cra") {
			t.Fatalf("%s has sources %v", r.ID, r.Sources)
		}
	}
	both := c.FindRequirements(Filter{Source: "cra, gdpr"}, "en")
	if len(both) <= len(reqs) {
		t.Errorf("cra,gdpr gives %d, cra alone %d", len(both), len(reqs))
	}
	tests := c.FindTests(Filter{Source: "cra"}, "en")
	brought := map[string]bool{}
	for _, r := range reqs {
		for _, id := range r.Tests {
			brought[id] = true
		}
	}
	for id := range brought {
		if !slices.ContainsFunc(tests, func(x TestSummary) bool { return x.ID == id }) {
			t.Errorf("test %s verifies a CRA requirement but is not found by the CRA filter", id)
		}
	}
	ov := c.SourcesOverview("sv", "", false)
	var cra *SourceSummary
	for i := range ov {
		if ov[i].ID == "cra" {
			cra = &ov[i]
		}
		if ov[i].Requirements+ov[i].Tests == 0 {
			t.Errorf("%s listed without items", ov[i].ID)
		}
	}
	if cra == nil || cra.Requirements != len(reqs) || cra.Tests != len(tests) || cra.Kind != "law" || !strings.Contains(cra.Name, "Cyberresilien") {
		t.Fatalf("cra summary %+v (reqs %d, tests %d)", cra, len(reqs), len(tests))
	}
	if len(c.SourcesOverview("en", "", true)) != len(c.Sources) {
		t.Error("all=true leaves out sources")
	}
	if n := len(c.SourcesOverview("en", "no-such-type", false)); n != 0 {
		t.Errorf("unknown catalogue type gives %d sources", n)
	}
	if len(c.SourceKindsOverview("sv")) != len(c.SourceKinds) {
		t.Error("kinds")
	}
	// ordered by kind
	rank := map[string]int{}
	for i, k := range c.SourceKinds {
		rank[k.ID] = i
	}
	for i := 1; i < len(ov); i++ {
		if rank[ov[i-1].Kind] > rank[ov[i].Kind] {
			t.Fatalf("%s before %s", ov[i-1].ID, ov[i].ID)
		}
	}
}

func TestSourcesOfText(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	got := c.SourcesOfText("gdpr art. 32; ISO/IEC 27001:2022 A.8.5\nunknown thing; ")
	if !slices.Equal(got, []string{"gdpr", "iso-27001"}) {
		t.Errorf("got %v", got)
	}
	if got := c.SourcesOfText(""); len(got) != 0 {
		t.Errorf("empty text gives %v", got)
	}
}

func TestLoadChecksSources(t *testing.T) {
	area := []byte(`area: x
prefix: XX
group: g
name: {en: X, sv: X}
description: {en: X, sv: X}
requirements:
  - id: XX-01
    type: security
    obligation: must
    priority: high
    refs: ["Known §1", "Unknown 2", "Both 3"]
    tests: [XX-T01]
    en: {title: T, description: The system MUST log every sign-in., rationale: R, criteria: [{method: test, text: C}]}
    sv: {title: T, description: Systemet SKA logga varje inloggning., rationale: R, criteria: [{method: test, text: C}]}
tests:
  - id: XX-T01
    priority: high
    execution: manual
    en: {title: T, description: D, preconditions: P, expected: E, steps: [{action: A, expected: E}]}
    sv: {title: T, description: D, preconditions: P, expected: E, steps: [{action: A, expected: E}]}
`)
	groups := []byte("groups:\n  - id: g\n    name: {en: G, sv: G}\n")
	good := []byte(`kinds:
  - id: standard
    name: {en: Standards, sv: Standarder}
sources:
  - id: known
    kind: standard
    name: {en: Known, sv: Känd}
    match: ["Known", "Both"]
  - id: other
    kind: standard
    name: {en: Other, sv: Annan}
    regex: ['^Both \d']
`)
	c, err := Load(fstest.MapFS{"d/groups.yaml": {Data: groups}, "d/x.yaml": {Data: area}, "d/sources.yaml": {Data: good}}, "d")
	if err != nil {
		t.Fatal(err)
	}
	probs := strings.Join(c.RefProblems(), "\n")
	for _, want := range []string{`ref "Unknown 2" matches no source: add the source to data/sources.yaml`, `ref "Both 3" matches several sources (known, other)`} {
		if !strings.Contains(probs, want) {
			t.Errorf("missing %q in\n%s", want, probs)
		}
	}
	if strings.Contains(probs, "Known §1") {
		t.Errorf("known ref reported:\n%s", probs)
	}
	if got := c.Requirements["XX-01"].Sources; !slices.Equal(got, []string{"known"}) {
		t.Errorf("sources %v", got)
	}
	bad := []byte(`kinds:
  - id: standard
    name: {en: Standards}
sources:
  - id: Bad Id
    kind: nope
    jurisdiction: Europe
    url: http://example.com
    name: {en: B}
  - id: re
    kind: standard
    name: {en: R, sv: R}
    regex: ['(']
    clause: '['
  - id: fmt
    kind: standard
    name: {en: F, sv: F}
    match: [F]
    clause_format: 'x$1'
`)
	_, err = Load(fstest.MapFS{"d/groups.yaml": {Data: groups}, "d/x.yaml": {Data: area}, "d/sources.yaml": {Data: bad}}, "d")
	if err == nil {
		t.Fatal("bad registry accepted")
	}
	for _, want := range []string{"source kind standard: name is needed in sv", `source "Bad Id": invalid`, "unknown kind", "jurisdiction", "https://", "needs at least one match", "regex", "clause", "clause_format needs clause", "name is needed in sv"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%s", want, err)
		}
	}
	if _, err := Load(fstest.MapFS{"d/groups.yaml": {Data: groups}, "d/sources.yaml": {Data: []byte("sources: [")}}, "d"); err == nil {
		t.Fatal("bad yaml accepted")
	}
}
