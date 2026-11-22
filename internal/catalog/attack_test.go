package catalog

import (
	"slices"
	"strings"
	"testing"
)

func TestAttackFindAndSummarize(t *testing.T) {
	c, err := Load(riskFS(strings.Replace(goodRisk, "attack: [T1190]", "attack: [T1190, T1110.004]\n    atlas: [AML.T0051.000]", 1)), "d")
	if err != nil {
		t.Fatal(err)
	}
	if tt := c.Technique("T1110.004"); tt == nil || tt.Parent != "T1110" || c.FullName(tt) != "Brute Force: Credential Stuffing" {
		t.Fatalf("technique %+v", tt)
	}
	if c.Technique("AML.T0051.000").Parent != "AML.T0051" || c.Technique("AML.T0051").Parent != "" {
		t.Fatal("atlas parent")
	}
	if c.Tactic("TA0006") == nil || c.Tactic("TA9999") != nil || !c.ValidTechnique("T1190") || c.ValidTechnique("T9999") {
		t.Fatal("lookup")
	}
	if NormalizeTechnique(" aml.t0051 ") != "AML.T0051" {
		t.Fatal("normalize")
	}
	s := c.SummarizeRisk(c.Risks["XA-R01"], "en")
	if !slices.Equal(s.Attack, []string{"T1190", "T1110.004"}) || !slices.Equal(s.Atlas, []string{"AML.T0051.000"}) ||
		!slices.Equal(s.Tactics, []string{"TA0001", "TA0006", "AML.TA0005"}) {
		t.Fatalf("summary %+v", s)
	}
	ts := c.TechniqueSummaries([]string{"T1110.004", "T0000"})
	if ts[0].Name != "Brute Force: Credential Stuffing" || ts[0].Tactics[0].Name != "Credential Access" || ts[0].URL == "" || ts[1].Name != "" {
		t.Fatalf("techniques %+v", ts)
	}
	ids := func(f Filter) int { return len(c.FindRisks(f, "en")) }
	for f, want := range map[Filter]int{
		{Attack: "T1110"}: 1, {Attack: "t1110.004"}: 1, {Attack: "T1110.001"}: 0, {Attack: "AML.T0051"}: 1,
		{Tactic: "TA0006"}: 1, {Tactic: "aml.ta0005"}: 1, {Tactic: "TA0040"}: 0,
		{Q: "credential stuffing"}: 1, {Q: "T1110.004"}: 1,
	} {
		if got := ids(f); got != want {
			t.Errorf("%+v: %d, want %d", f, got, want)
		}
	}
	cov := c.AttackCoverage("")
	if len(cov) != 2 || cov[0].ID != "enterprise" || cov[0].Risks != 1 || len(cov[0].Tactics) != 2 {
		t.Fatalf("coverage %+v", cov)
	}
	ca := cov[0].Tactics[1] // Credential Access: the parent T1110 with the sub-technique's risk
	if ca.Risks != 1 || ca.Requirements != 2 || len(ca.Techniques) != 1 || ca.Techniques[0].ID != "T1110" || ca.Techniques[0].Risks != 1 {
		t.Fatalf("credential access %+v", ca)
	}
	// requirements count by their ATT&CK refs too
	if ia := cov[0].Tactics[0]; ia.Requirements != 2 {
		t.Fatalf("initial access %+v", ia)
	}
	if other := c.AttackCoverage("hardware"); other[0].Risks != 0 || other[0].Tactics[0].Techniques == nil {
		t.Fatalf("filtered %+v", other)
	}
}

func TestAttackRiskProblems(t *testing.T) {
	for change, want := range map[string]string{
		"attack: [T9999]":                          `unknown technique "T9999" in attack`,
		"attack: [T1190, T1190]":                   "T1190 listed twice in attack",
		"attack: [AML.T0051]":                      "AML.T0051 is an ATLAS technique: list it in atlas",
		"atlas: [T1190]":                           "T1190 is an ATT&CK technique: list it in attack",
		"attack: [T1110, T1110.004]":               "lists both T1110 and its sub-technique T1110.004",
		"attack: []":                               "a security risk lists at least one",
		"attack: [T1190]\n    attack_exempt: true": "attack_exempt with techniques listed",
	} {
		_, err := Load(riskFS(strings.Replace(goodRisk, "attack: [T1190]", change, 1)), "d")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: want %q, got %v", change, want, err)
		}
	}
	safety := strings.Replace(goodRisk, "category: security", "category: safety", 1)
	if _, err := Load(riskFS(safety), "d"); err == nil || !strings.Contains(err.Error(), "a safety risk is not caused by an adversary") {
		t.Errorf("safety risk with a technique: %v", err)
	}
	exempt := strings.Replace(goodRisk, "attack: [T1190]", "attack_exempt: true", 1)
	if _, err := Load(riskFS(exempt), "d"); err != nil {
		t.Errorf("exempt: %v", err)
	}
	ref := strings.Replace(goodRisk, `"CWE-89 (SQL injection)"]`, `"CWE-89 (SQL injection)", "MITRE ATT&CK T1190 (exploit)", "MITRE ATLAS AML.T0051 (prompt injection)"]`, 1)
	_, err := Load(riskFS(ref), "d")
	for _, want := range []string{"list ATT&CK techniques and tactics of a risk in attack", "list ATLAS techniques of a risk in atlas"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q, got %v", want, err)
		}
	}
	// refs of requirements must cite ids of the list
	fs := riskFS(goodRisk)
	x := string(fs["d/x.yaml"].Data)
	fs["d/x.yaml"].Data = []byte(strings.Replace(x, `refs: ["CWE-89 (SQL injection)"]`, `refs: ["MITRE ATT&CK T1078 (valid accounts)", "MITRE ATT&CK M1032 (MFA)", "MITRE ATT&CK TA0006 (credential access)", "MITRE ATLAS AML.T0099 (x)"]`, 1))
	_, err = Load(fs, "d")
	if err == nil || !strings.Contains(err.Error(), `cites T1078, which is not in the ATT&CK/ATLAS list`) || !strings.Contains(err.Error(), "cites AML.T0099") ||
		strings.Contains(err.Error(), "cites M1032") || strings.Contains(err.Error(), "cites TA0006") {
		t.Errorf("refs: %v", err)
	}
}

func TestAttackListProblems(t *testing.T) {
	fs := riskFS(goodRisk)
	fs["d/atlas.yaml"] = fs["d/attack.yaml"]
	if _, err := Load(fs, "d"); err == nil || !strings.Contains(err.Error(), "invalid or duplicate id") {
		t.Errorf("duplicate list: %v", err)
	}
	bad := `matrices:
  - {id: Bad Matrix, name: "", version: "1", url: http://x}
  - {id: atlas, name: ATLAS, version: "5", url: https://atlas.mitre.org/}
tactics:
  - {id: TA0001, name: Initial Access, matrix: nope, url: https://x.org/}
  - {id: T0001, name: x, matrix: atlas, url: https://x.org/}
  - {id: AML.TA0001, name: "", matrix: atlas, url: https://x.org/}
techniques:
  - {id: T1190, name: x, matrix: nope, tactics: [TA0001], url: https://x.org/}
  - {id: T1191, name: x, matrix: atlas, tactics: [TA0001], url: https://x.org/}
  - {id: AML.T0001, name: "", matrix: atlas, tactics: [AML.TA0001], url: https://x.org/}
  - {id: AML.T0002, name: x, matrix: atlas, tactics: [], url: https://x.org/}
  - {id: AML.T0003, name: x, matrix: atlas, tactics: [TA0001], url: https://x.org/}
  - {id: AML.T0004.001, name: x, matrix: atlas, tactics: [AML.TA0001], url: https://x.org/}
mitigations:
  - {id: X1, name: x, matrix: atlas, url: https://x.org/}
`
	fs2 := riskFS(goodRisk)
	fs2["d/attack.yaml"].Data = []byte(bad)
	_, err := Load(fs2, "d")
	for _, want := range []string{
		`attack matrix "Bad Matrix"`, "attack tactic TA0001: unknown matrix", `attack tactic "T0001": invalid`,
		"attack tactic AML.TA0001: name and url", "attack technique T1190: unknown matrix", `attack technique "T1191": invalid`,
		"attack technique AML.T0001: name and url", "attack technique AML.T0002: at least one tactic",
		"attack technique AML.T0003: tactic TA0001 is not one of matrix atlas", "no parent technique AML.T0004", `attack mitigation "X1"`,
	} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}

// The built-in list and the risks' mapping: every security risk maps to
// ATT&CK or ATLAS (or is exempted), no hazard does, and every catalogue
// item's ATT&CK refs resolve (checked by Load).
func TestAttackLibrary(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]int{}
	for _, tt := range c.Techniques {
		m[tt.Matrix]++
	}
	if m["enterprise"] < 600 || m["ics"] < 80 || m["mobile"] < 100 || m["atlas"] < 20 {
		t.Errorf("techniques per matrix %v", m)
	}
	mapped, exempt := 0, 0
	for _, k := range c.Risks {
		if len(k.Techniques()) > 0 {
			mapped++
		}
		if k.AttackExempt {
			exempt++
		}
	}
	if mapped < 200 || exempt > 10 {
		t.Errorf("%d of %d risks mapped, %d exempt", mapped, len(c.Risks), exempt)
	}
	cov := c.AttackCoverage("")
	used := 0
	for _, x := range cov {
		for _, ta := range x.Tactics {
			if ta.Risks > 0 {
				used++
			}
		}
	}
	if used < 20 {
		t.Errorf("only %d tactics have risks", used)
	}
	t.Logf("%d risks mapped (%d exempt), %d tactics with risks, techniques %v", mapped, exempt, used, m)
}
