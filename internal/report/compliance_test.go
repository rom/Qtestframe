package report

import (
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/catalog"
	"github.com/rom/qtestframe/internal/model"
)

// complianceData is the sample data with a requirement imported from a
// CRA catalogue item (passed) and an own one citing GDPR (failing).
func complianceData(t *testing.T) *Data {
	t.Helper()
	c, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	cra := c.FindRequirements(catalog.Filter{Source: "cra"}, "en")
	d := sampleData()
	d.Requirements = append(d.Requirements,
		model.Requirement{ID: "r4", Key: "P-REQ-4", Title: "Updates", Status: "approved", Obligation: "must", CatalogRef: cra[0].ID},
		model.Requirement{ID: "r5", Key: "P-REQ-5", Title: "Encryption", Status: "approved", Obligation: "must", Source: "GDPR Art. 32(1)(a)"})
	d.Links = append(d.Links,
		model.TraceLink{RequirementID: "r4", TestCaseID: "t1", RequirementKey: "P-REQ-4", TestCaseKey: "P-TC-1"},
		model.TraceLink{RequirementID: "r5", TestCaseID: "t2", RequirementKey: "P-REQ-5", TestCaseKey: "P-TC-2"})
	return d
}

func TestComplianceReport(t *testing.T) {
	if _, ok := Lookup(Compliance); !ok {
		t.Fatal("compliance template not registered")
	}
	d := complianceData(t)
	ds, err := Build(Compliance, d, map[string]string{"sources": "cra,gdpr"})
	if err != nil {
		t.Fatal(err)
	}
	if ds.Title != "Compliance" {
		t.Fatal(ds.Title)
	}
	src := section(t, ds, "sources")
	if len(src.Rows) != 2 || !strings.Contains(src.Rows[0][0], "Cyber Resilience Act") || src.Rows[0][4] != "1" || src.Rows[1][6] != "1" || src.Rows[1][8] != "1" {
		t.Fatalf("sources: %v", src.Rows)
	}
	section(t, ds, "clauses_cra")
	if g := section(t, ds, "gaps"); len(g.Rows) == 0 || g.Rows[0][0] != "CRA" {
		t.Fatalf("gaps: %v", g.Rows)
	}
	open := section(t, ds, "not_fulfilled")
	if len(open.Rows) != 1 || open.Rows[0][1] != "P-REQ-5" || open.Rows[0][3] != "Failing" || open.Rows[0][4] != "Own" || open.Rows[0][5] != "Art. 32" {
		t.Fatalf("not fulfilled: %v", open.Rows)
	}
	renderAll(t, ds)

	// Swedish, with an unknown source named in the subtitle
	failOnMissing(t)
	d.Lang = "sv"
	ds, err = Build(Compliance, d, map[string]string{"sources": "gdpr,nope"})
	if err != nil {
		t.Fatal(err)
	}
	checkSwedish(t, ds)
	if ds.Title != "Efterlevnad" || !strings.Contains(ds.Subtitle, "Okända källor: nope") || section(t, ds, "not_fulfilled").Rows[0][3] != "Underkänt" {
		t.Fatalf("%s / %s / %v", ds.Title, ds.Subtitle, section(t, ds, "not_fulfilled").Rows)
	}
	if _, err := Build(Compliance, d, map[string]string{"sources": "nope"}); err == nil {
		t.Fatal("only unknown sources accepted")
	}
	// without sources: those the project uses
	ds, err = Build(Compliance, d, nil)
	if err != nil || len(section(t, ds, "sources").Rows) < 2 {
		t.Fatalf("default sources: %v %v", err, ds)
	}
	l := newLoc("sv")
	if l.sourceKind("x_y") != "X y" || l.clauses([]string{"", "A.8"}) != "(allmänt), A.8" || l.state("other") == "" {
		t.Error("labels")
	}
}
