package api

import (
	"strings"
	"testing"
)

func TestRiskReport(t *testing.T) {
	e := newEnv(t)
	a := e.login("admin@example.com")
	fx := newReportFixture(t, a)
	base := "/api/v1/projects/" + fx.pid
	var reqID string
	for _, r := range items(a.m("GET", base+"/requirements", nil, 200)) {
		if r["key"] == fx.reqKey {
			reqID = r["id"].(string)
		}
	}
	a.m("POST", base+"/risks", map[string]any{"title": "Account takeover", "likelihood": 5, "impact": 4, "owner": "sec", "mitigation": "Lock out after 5 tries",
		"requirement_ids": []string{reqID}}, 201)
	a.m("POST", base+"/risks", map[string]any{"title": "Slow reports", "likelihood": 2, "impact": 2, "status": "mitigated"}, 201)

	_, data := a.download(base+"/reports/generate", map[string]any{"template": "risks", "format": "csv"}, 200)
	files := zipFiles(t, data)
	if !strings.Contains(files["summary.csv"], "Risks,2") || !strings.Contains(files["summary.csv"], "Open critical risks,1") {
		t.Fatal(files["summary.csv"])
	}
	if reg := files["register.csv"]; !strings.Contains(reg, "Account takeover") || !strings.Contains(reg, fx.reqKey) || !strings.Contains(reg, "Tested, not all passed") {
		t.Fatal(reg)
	}
	// the requirement has a passed and a failed test
	if tr := files["traceability.csv"]; !strings.Contains(tr, fx.testKey) || !strings.Contains(tr, "Failed") || !strings.Contains(tr, "Passed") {
		t.Fatal(tr)
	}
	for _, name := range []string{"heat_map.csv", "top_risks.csv", "untested_mitigations.csv", "due_for_review.csv", "risks_by_level_and_status.csv"} {
		if _, ok := files[name]; !ok {
			t.Fatal("missing", name, files)
		}
	}
	_, data = a.download(base+"/reports/generate", map[string]any{"template": "risks", "format": "md", "lang": "sv"}, 200)
	if md := string(data); !strings.Contains(md, "# Riskrapport") || !strings.Contains(md, "Värmekarta") || !strings.Contains(md, "Account takeover") {
		t.Fatal(md)
	}
	if got := reportReadPerms(reportJob{template: "risks"}); len(got) != 3 {
		t.Fatal(got)
	}
}
