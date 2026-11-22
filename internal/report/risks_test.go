package report

import (
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// sampleRisks address the sample requirements (r1 failed, r2 passed, r3
// without tests).
func sampleRisks() []model.Risk {
	old := t0.Add(-100 * 24 * time.Hour)
	return []model.Risk{
		{ID: "k2", Key: "P-RISK-2", Title: "Session hijack", Likelihood: 4, Impact: 3, Status: "mitigating", Owner: "ann", Mitigation: "Rotate tokens", RequirementIDs: []string{"r2"}, UpdatedAt: t0},
		{ID: "k1", Key: "P-RISK-1", Title: "Data leak", Category: "Security", Likelihood: 5, Impact: 5, Status: "identified", Owner: "bob", RequirementIDs: []string{"r1", "r3"}, UpdatedAt: old},
		{ID: "k3", Key: "P-RISK-3", Title: "Outage", Likelihood: 2, Impact: 2, Status: "mitigated", UpdatedAt: t0},
		{ID: "k4", Key: "P-RISK-4", Title: "Typo", Likelihood: 1, Impact: 1, Status: "closed", UpdatedAt: old},
		{ID: "k5", Key: "P-RISK-5", Title: "Unowned", Likelihood: 3, Impact: 4, Status: "identified", UpdatedAt: t0},
		{ID: "k6", Key: "P-RISK-6", Title: "Wrong password accepted", Likelihood: 4, Impact: 4, Status: "identified", RequirementIDs: []string{"r1"}, UpdatedAt: t0,
			Attack: []string{"T1110.004", "T1078"}, Emb3d: []string{"TID-201"}},
	}
}

func TestMitigationState(t *testing.T) {
	cov := map[string]string{"a": "passed", "b": "failed", "c": "none", "d": "not_run"}
	for _, c := range []struct {
		ids  []string
		want string
	}{
		{nil, MitigationNone},
		{[]string{"c"}, MitigationUntested},
		{[]string{"missing"}, MitigationUntested},
		{[]string{"a", "c"}, MitigationPartly},
		{[]string{"a", "b"}, MitigationTested},
		{[]string{"d"}, MitigationTested},
		{[]string{"a"}, MitigationVerified},
	} {
		if got := MitigationState(c.ids, cov); got != c.want {
			t.Errorf("%v: %s, want %s", c.ids, got, c.want)
		}
	}
}

func TestRiskReport(t *testing.T) {
	ds, err := Build(Risks, sampleData(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ds.Title != "Risk report" {
		t.Fatal(ds.Title)
	}
	sum := section(t, ds, "summary")
	for k, v := range map[string]string{
		"Risks": "6", "Open risks (identified or mitigating)": "4", "Mitigated risks": "1", "Closed risks": "1",
		"Open critical risks": "1", "Open high risks": "3", "Open risks without requirements": "1",
		"Open risks with untested mitigation": "2", "Open risks with verified mitigation": "1", "Share of open risks verified": "25.0%",
		"Open risks not updated for 90 days or more": "1",
	} {
		if got := metric(t, sum, k); got != v {
			t.Errorf("%s = %s, want %s", k, got, v)
		}
	}
	lvl := section(t, ds, "risks_by_level_and_status")
	if strings.Join(lvl.Rows[0], "|") != "Critical|1|0|0|0|0|1" || strings.Join(lvl.Rows[4], "|") != "Total|3|1|1|0|1|6" {
		t.Fatal(lvl.Rows)
	}
	heat := section(t, ds, "heat_map")
	// row 0 = likelihood 5; the closed 1×1 risk is not counted
	if len(heat.Rows) != 5 || heat.Rows[0][5] != "1 · Critical" || heat.Rows[1][3] != "1 · High" || heat.Rows[4][1] != "0 · Low" {
		t.Fatal(heat.Rows)
	}
	reg := section(t, ds, "register")
	if len(reg.Rows) != 6 || reg.Rows[0][0] != "P-RISK-1" || reg.Rows[0][10] != "P-REQ-1, P-REQ-10" || reg.Rows[0][11] != "Partly tested" {
		t.Fatal(reg.Rows[0])
	}
	top := section(t, ds, "top_risks")
	if len(top.Rows) != 4 || top.Rows[0][1] != "P-RISK-1" || top.Rows[1][1] != "P-RISK-6" {
		t.Fatal(top.Rows)
	}
	tr := section(t, ds, "traceability")
	found := false
	for _, r := range tr.Rows {
		if r[0] == "P-RISK-1" && r[3] == "P-REQ-1" && r[6] == "P-TC-2" && r[8] == "Failed" {
			found = true
		}
	}
	if !found {
		t.Fatal(tr.Rows)
	}
	un := section(t, ds, "untested_mitigations")
	if len(un.Rows) != 2 || un.Rows[0][0] != "P-RISK-1" || un.Rows[0][5] != "P-REQ-10" || un.Rows[0][6] != "P-REQ-1" || un.Rows[1][0] != "P-RISK-5" {
		t.Fatal(un.Rows)
	}
	due := section(t, ds, "due_for_review")
	if len(due.Rows) != 1 || due.Rows[0][0] != "P-RISK-1" || due.Rows[0][6] != "100" {
		t.Fatal(due.Rows)
	}
	atk := section(t, ds, "attack")
	if len(atk.Rows) != 2 || atk.Rows[0][0] != "T1078" || atk.Rows[0][1] != "Valid Accounts" || atk.Rows[1][1] != "Brute Force: Credential Stuffing" ||
		atk.Rows[1][3] != "Credential Access" || atk.Rows[1][4] != "P-RISK-6" || atk.Rows[1][5] != "1" || !strings.HasPrefix(atk.Rows[1][6], "https://attack.mitre.org/") {
		t.Fatal(atk.Rows)
	}
	if metric(t, sum, "Risks with ATT&CK techniques") != "1" || reg.Rows[1][23] != "T1110.004, T1078" {
		t.Fatal(reg.Rows[1])
	}
	emb := section(t, ds, "emb3d")
	if len(emb.Rows) != 1 || emb.Rows[0][0] != "TID-201" || emb.Rows[0][1] != "Inadequate Bootloader Protection and Verification" || emb.Rows[0][2] != "system software" ||
		emb.Rows[0][3] != "P-RISK-6" || !strings.HasPrefix(emb.Rows[0][5], "https://emb3d.mitre.org/") || metric(t, sum, "Risks with EMB3D threats") != "1" || reg.Rows[1][24] != "TID-201" {
		t.Fatal(emb.Rows, reg.Rows[1])
	}
	renderAll(t, ds)

	// no risks: every section is still there
	d := sampleData()
	d.Risks = nil
	ds, err = Build(Risks, d, nil)
	if err != nil || len(ds.Sections) != 13 || metric(t, section(t, ds, "summary"), "Share of open risks verified") != "–" {
		t.Fatal(err, names(ds))
	}
}

func TestSwedishRiskReport(t *testing.T) {
	failOnMissing(t)
	ds, _ := Build(Risks, swedishData(), nil)
	if ds.Title != "Riskrapport" {
		t.Fatal(ds.Title)
	}
	if got := section(t, ds, "heat_map").Rows[0][5]; got != "1 · Kritisk" {
		t.Fatal(got)
	}
	if got := section(t, ds, "register").Rows[0]; got[8] != "Identifierad" || got[11] != "Delvis testad" {
		t.Fatal(got)
	}
	if got := metric(t, section(t, ds, "summary"), "Andel verifierade öppna risker"); got != "25,0 %" {
		t.Fatal(got)
	}
}
