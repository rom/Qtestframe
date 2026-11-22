package catalog

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// testD3fend and testEmb3d are small D3FEND and EMB3D lists.
const testD3fend = `version: "1.6.0"
release: "2026-08-31"
url: https://d3fend.mitre.org/
tactics:
  - {id: Harden, name: "Harden", url: https://d3fend.mitre.org/tactic/d3f:Harden/}
  - {id: Detect, name: "Detect", url: https://d3fend.mitre.org/tactic/d3f:Detect/}
techniques:
  - {id: D3-AA, name: "Agent Authentication", tactic: Harden, url: "https://d3fend.mitre.org/technique/d3f:AgentAuthentication/", attack: [T1110]}
  - {id: D3-MFA, name: "Multi-factor Authentication", tactic: Harden, parent: D3-AA, url: "https://d3fend.mitre.org/technique/d3f:Multi-factorAuthentication/", attack: [T1110, T1110.004]}
  - {id: D3-NTA, name: "Network Traffic Analysis", tactic: Detect, url: "https://d3fend.mitre.org/technique/d3f:NetworkTrafficAnalysis/", attack: [T1190]}
`

const testEmb3d = `version: "2.0.1"
url: https://emb3d.mitre.org/
properties:
  - {id: PID-11, name: "Device includes a microprocessor", category: "hardware"}
threats:
  - {id: TID-101, name: "Power Consumption Analysis Side Channel", category: "hardware", maturity: "proof of concept", url: https://emb3d.mitre.org/threats/TID-101.html, properties: [PID-11], mitigations: [MID-059]}
  - {id: TID-327, name: "Out of Bounds Memory Access", category: "application software", maturity: "observed in the wild", url: https://emb3d.mitre.org/threats/TID-327.html, properties: [PID-11], mitigations: [MID-008]}
mitigations:
  - {id: MID-008, name: "Decidable Protocols and Parsers", maturity: "intermediate", url: https://emb3d.mitre.org/mitigations/MID-008.html}
  - {id: MID-059, name: "Software Patterns for Side Channel Resistance", maturity: "intermediate", url: https://emb3d.mitre.org/mitigations/MID-059.html}
`

// frameworkFS is riskFS(goodRisk) with the D3FEND and EMB3D lists and
// the fields set by edit on x.yaml.
func frameworkFS(edit func(string) string) fstest.MapFS {
	fs := riskFS(goodRisk)
	fs["d/d3fend.yaml"] = &fstest.MapFile{Data: []byte(testD3fend)}
	fs["d/emb3d.yaml"] = &fstest.MapFile{Data: []byte(testEmb3d)}
	fs["d/x.yaml"] = &fstest.MapFile{Data: []byte(edit(string(fs["d/x.yaml"].Data)))}
	return fs
}

// withFields gives XA-01 d3fend and emb3d, the test XA-T01 and the risk
// XA-R01 emb3d.
func withFields(req, test, risk string) func(string) string {
	return func(s string) string {
		s = strings.Replace(s, "  - id: XA-01\n", "  - id: XA-01\n"+req, 1)
		s = strings.Replace(s, "  - id: XA-T01\n", "  - id: XA-T01\n"+test, 1)
		return strings.Replace(s, "    attack: [T1190]\n", "    attack: [T1190]\n"+risk, 1)
	}
}

func TestFrameworksLoadFindAndCover(t *testing.T) {
	c, err := Load(frameworkFS(withFields("    d3fend: [D3-NTA, D3-MFA]\n    emb3d: [MID-008]\n", "    d3fend: [D3-NTA]\n    emb3d: [TID-101]\n", "    emb3d: [TID-327]\n")), "d")
	if err != nil {
		t.Fatal(err)
	}
	if c.D3fendTechnique("D3-MFA").Parent != "D3-AA" || !c.ValidD3fend("D3-AA") || c.ValidD3fend("D3-XX") || c.Emb3dThreat("TID-101") == nil || c.Emb3dMitigation("MID-008") == nil {
		t.Fatal("lookup")
	}
	if !c.ValidEmb3d("MID-008", false) || c.ValidEmb3d("MID-008", true) || !c.ValidEmb3d("TID-101", true) || NormalizeFrameworkID(" d3-mfa ") != "D3-MFA" {
		t.Fatal("validity")
	}
	if !slices.Equal(c.Emb3dMitigation("MID-059").Threats, []string{"TID-101"}) || !slices.Equal(c.Emb3dThreatsOf([]string{"MID-008", "TID-101"}), []string{"TID-101", "TID-327"}) {
		t.Fatal("derived threats")
	}
	if s := c.SummarizeD3fend("D3-MFA"); s.Name != "Multi-factor Authentication" || s.ParentName != "Agent Authentication" || s.Attack != 2 || s.Tactic != "Harden" {
		t.Fatalf("d3fend summary %+v", s)
	}
	if s := c.SummarizeEmb3d("TID-101"); s.Kind != "threat" || s.Category != "hardware" || c.SummarizeEmb3d("MID-008").Kind != "mitigation" || c.SummarizeEmb3d("X").Kind != "" {
		t.Fatalf("emb3d summary %+v", s)
	}
	if !slices.Equal(c.D3fendTacticsOf([]string{"D3-NTA", "D3-MFA", "D3-AA"}), []string{"Detect", "Harden"}) {
		t.Fatal("tactics")
	}
	if !slices.Equal(c.CountersOf("T1110.004"), []string{"D3-AA", "D3-MFA"}) || len(c.CountersOf("T1078")) != 0 {
		t.Fatalf("counters %v", c.CountersOf("T1110.004"))
	}
	rs := c.SummarizeRequirement(c.Requirements["XA-01"], "en")
	if !slices.Equal(rs.D3fend, []string{"D3-NTA", "D3-MFA"}) || !slices.Equal(rs.Emb3d, []string{"MID-008"}) {
		t.Fatalf("summary %+v", rs)
	}
	if ts := c.SummarizeTest(c.Tests["XA-T01"], "en"); !slices.Equal(ts.D3fend, []string{"D3-NTA"}) || !slices.Equal(c.SummarizeRisk(c.Risks["XA-R01"], "en").Emb3d, []string{"TID-327"}) {
		t.Fatalf("test summary %+v", ts)
	}
	reqs := func(f Filter) int { return len(c.FindRequirements(f, "en")) }
	for f, want := range map[Filter]int{
		{D3fend: "D3-MFA"}: 1, {D3fend: "d3-aa"}: 1, {D3fend: "harden"}: 1, {D3fend: "D3-XX"}: 0,
		{Emb3d: "MID-008"}: 1, {Emb3d: "TID-327"}: 1, {Emb3d: "TID-101"}: 0, {Q: "multi-factor"}: 1, {Q: "MID-008"}: 1,
	} {
		if got := reqs(f); got != want {
			t.Errorf("requirements %+v: %d, want %d", f, got, want)
		}
	}
	if n := len(c.FindTests(Filter{Emb3d: "hardware"}, "en")); n != 1 {
		t.Errorf("tests by threat category: %d", n)
	}
	if n := len(c.FindTests(Filter{D3fend: "D3-MFA"}, "en")); n != 0 {
		t.Errorf("tests by D3-MFA: %d", n)
	}
	for f, want := range map[Filter]int{{D3fend: "D3-NTA"}: 1, {D3fend: "D3-MFA"}: 0, {Emb3d: "TID-327"}: 1, {Emb3d: "MID-008"}: 0, {Emb3d: "application software"}: 1} {
		if got := len(c.FindRisks(f, "en")); got != want {
			t.Errorf("risks %+v: %d, want %d", f, got, want)
		}
	}
	// the risk's T1190 is countered by D3-NTA, which its mitigating XA-01 lists
	def := c.RiskDefenses(c.Risks["XA-R01"], "en")
	if len(def) != 1 || def[0].Technique.ID != "T1190" || !def[0].Covered || len(def[0].Requirements) != 1 || def[0].Requirements[0].ID != "XA-01" ||
		!def[0].Requirements[0].Mitigates || !slices.Equal(def[0].Requirements[0].D3fend, []string{"D3-NTA"}) || def[0].D3fend[0].ID != "D3-NTA" {
		t.Fatalf("defenses %+v", def)
	}
	cov := c.D3fendCoverage("")
	if cov.Version != "1.6.0" || cov.Risks != 1 || cov.Covered != 1 || cov.Gaps != 0 || len(cov.Tactics) != 2 {
		t.Fatalf("d3fend coverage %+v", cov)
	}
	h := cov.Tactics[0] // Harden: D3-MFA counts for its parent D3-AA too
	if h.Total != 2 || h.Used != 2 || h.Requirements != 1 || h.Tests != 0 || h.Techniques[0].ID != "D3-AA" || h.Techniques[0].Requirements != 1 {
		t.Fatalf("harden %+v", h)
	}
	if d := cov.Tactics[1]; d.Requirements != 1 || d.Tests != 1 {
		t.Fatalf("detect %+v", d)
	}
	ec := c.Emb3dCoverage("")
	if ec.Version != "2.0.1" || len(ec.Categories) != 4 || ec.Mitigations != 1 {
		t.Fatalf("emb3d coverage %+v", ec)
	}
	hw, app := ec.Categories[0], ec.Categories[2]
	if hw.Total != 1 || hw.Tests != 1 || hw.Addressed != 0 || app.Risks != 1 || app.Requirements != 1 || app.Addressed != 1 {
		t.Fatalf("categories %+v %+v", hw, app)
	}
	if c.Emb3dCoverage("hardware").Categories[2].Risks != 0 || c.D3fendCoverage("hardware").Risks != 0 {
		t.Fatal("catalogue type filter")
	}
	// without a mitigating requirement that counters it the technique is a gap
	c2, err := Load(frameworkFS(withFields("    d3fend: [D3-MFA]\n", "", "")), "d")
	if err != nil {
		t.Fatal(err)
	}
	if def := c2.RiskDefenses(c2.Risks["XA-R01"], "en"); def[0].Covered || len(def[0].Requirements) != 0 {
		t.Fatalf("gap %+v", def)
	}
	if cov := c2.D3fendCoverage(""); cov.Gaps != 1 || cov.Covered != 0 {
		t.Fatalf("gap coverage %+v", cov)
	}
}

func TestFrameworkProblems(t *testing.T) {
	for _, tc := range []struct{ req, test, risk, want string }{
		{"    d3fend: [D3-XX]\n", "", "", `XA-01: unknown D3FEND technique "D3-XX"`},
		{"    d3fend: [D3-NTA, D3-NTA]\n", "", "", "D3FEND technique D3-NTA listed twice"},
		{"    d3fend: [D3-AA, D3-MFA]\n", "", "", "lists both D3-AA and its sub-technique D3-MFA"},
		{"    emb3d: [MID-999]\n", "", "", `unknown EMB3D threat or mitigation "MID-999"`},
		{"    emb3d: [MID-008, MID-008]\n", "", "", "EMB3D id MID-008 listed twice"},
		{"", "    emb3d: [PID-11]\n", "", `XA-T01: unknown EMB3D threat or mitigation "PID-11"`},
		{"", "    d3fend: [D3-YY]\n", "", `XA-T01: unknown D3FEND technique "D3-YY"`},
		{"", "", "    emb3d: [MID-008]\n", `XA-R01: unknown EMB3D threat "MID-008"`},
		{"    d3fend: [D3-AA, D3-MFA, D3-NTA, D3-A1, D3-A2, D3-A3, D3-A4, D3-A5, D3-A6]\n", "", "", "9 D3FEND techniques, at most 8"},
	} {
		_, err := Load(frameworkFS(withFields(tc.req, tc.test, tc.risk)), "d")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q %q %q: want %q, got %v", tc.req, tc.test, tc.risk, tc.want, err)
		}
	}
	refs := func(s string) string {
		return strings.Replace(s, `"CWE-89 (SQL injection)"]`, `"MITRE EMB3D MID-008"]`, 1)
	}
	if _, err := Load(frameworkFS(refs), "d"); err == nil || !strings.Contains(err.Error(), "list EMB3D ids in emb3d, not in refs") {
		t.Errorf("ref citing EMB3D: %v", err)
	}
	safety := func(s string) string {
		return strings.Replace(withFields("", "", "    emb3d: [TID-101]\n")(s), "category: security", "category: safety", 1)
	}
	if _, err := Load(frameworkFS(safety), "d"); err == nil || !strings.Contains(err.Error(), "it lists no EMB3D threat") {
		t.Errorf("safety risk with a threat: %v", err)
	}
	bad := frameworkFS(withFields("", "", ""))
	bad["d/d3fend.yaml"] = &fstest.MapFile{Data: []byte(strings.Replace(testD3fend, "parent: D3-AA", "parent: D3-ZZ", 1) +
		"  - {id: D3-X, name: \"X\", tactic: Evict, url: \"https://d3fend.mitre.org/x/\", attack: [T9999]}\n  - {id: bad, name: \"\", tactic: Harden, url: \"\"}\n")}
	bad["d/emb3d.yaml"] = &fstest.MapFile{Data: []byte(strings.Replace(strings.Replace(testEmb3d, "properties: [PID-11], mitigations: [MID-059]", "properties: [PID-99], mitigations: [MID-999]", 1), `category: "application software"`, `category: "firmware"`, 1))}
	_, err := Load(bad, "d")
	for _, want := range []string{"d3fend technique D3-MFA: unknown parent D3-ZZ", `d3fend technique D3-X: unknown tactic "Evict"`, "ATT&CK technique T9999 is not in attack.yaml",
		`d3fend technique "bad": invalid or duplicate id`, "emb3d threat TID-101: unknown property PID-99", "emb3d threat TID-101: unknown mitigation MID-999", `unknown category "firmware"`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q in %v", want, err)
		}
	}
}

// The embedded lists and the catalogue's mappings.
func TestFrameworksOfTheCatalogue(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if c.D3fend.Version == "" || len(c.D3fend.Tactics) != 7 || len(c.D3fend.Techniques) < 250 || c.D3fendTechnique("D3-MFA") == nil || len(c.CountersOf("T1110.001")) == 0 {
		t.Fatalf("d3fend %s: %d techniques", c.D3fend.Version, len(c.D3fend.Techniques))
	}
	if c.Emb3d.Version == "" || len(c.Emb3d.Threats) < 80 || len(c.Emb3d.Mitigations) < 85 || len(c.Emb3d.Properties) < 50 {
		t.Fatalf("emb3d %s: %d threats", c.Emb3d.Version, len(c.Emb3d.Threats))
	}
	var d3, emb int
	for _, r := range c.Requirements {
		if len(r.D3fend) > 0 {
			d3++
		}
		if len(r.Emb3d) > 0 {
			emb++
		}
	}
	if d3 < 150 || emb < 40 {
		t.Fatalf("requirements with D3FEND %d, with EMB3D %d", d3, emb)
	}
	// the embedded security area is mapped throughout: every risk lists
	// EMB3D threats, and most requirements an EMB3D id
	a := c.Area("embedded-security")
	if a == nil || a.Catalogue != "hardware" || len(a.Requirements) < 25 {
		t.Fatal("embedded-security area")
	}
	n := 0
	for _, r := range a.Requirements {
		if len(r.Emb3d) > 0 {
			n++
		}
	}
	if n*10 < len(a.Requirements)*8 {
		t.Errorf("only %d of %d embedded security requirements list EMB3D ids", n, len(a.Requirements))
	}
	for _, k := range a.Risks {
		if len(k.Emb3d) == 0 {
			t.Errorf("%s lists no EMB3D threat", k.ID)
		}
	}
	for _, k := range c.Areas[0].Risks {
		_ = c.RiskDefenses(&k, "sv")
	}
	if cov := c.D3fendCoverage(""); cov.Risks < 100 || cov.Covered == 0 {
		t.Errorf("d3fend coverage %+v", cov.Risks)
	}
	if hw := c.Emb3dCoverage("hardware"); hw.Mitigations < 30 {
		t.Errorf("emb3d mitigations used %d", hw.Mitigations)
	}
}
