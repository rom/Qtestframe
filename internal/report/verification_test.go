package report

import (
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
)

func verificationSample(lang string) *Data {
	d := sampleData()
	d.Lang = lang
	crit := "c1"
	at := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	d.Verification = &VerificationData{
		Strict: true,
		Requirements: map[string]*model.RequirementVerification{
			"r1": {RequirementID: "r1", Key: "P-REQ-1", State: model.VerPartial, Own: model.VerPartial, CriteriaVerified: 1, CriteriaTotal: 2,
				Criteria: []model.CriterionVerification{
					{CriterionID: "c1", Position: 1, Description: "Seal drawing", Method: "inspection", State: model.VerVerified,
						Evidence: []model.Evidence{{Kind: "record", Key: "inspection", Title: "Ann", Result: "conditional", State: "passed", Note: "conditional", Date: "2026-02-01"}}},
					{CriterionID: "c2", Position: 2, Description: "Leak test", Method: "test", State: model.VerPending,
						Evidence: []model.Evidence{{Kind: "test", Key: "P-TC-1", Result: "not_run", State: "pending", StepPosition: 2, Note: "step_not_run"}}},
				},
				Evidence: []model.Evidence{{Kind: "test", Key: "P-TC-2", Result: "passed", State: "passed", Plain: true}},
				Children: []model.ChildVerification{{ID: "r2", Key: "P-REQ-2", Status: "draft", State: model.VerNone}},
				Reasons: []model.VerificationReason{{Code: "criterion_pending", Params: map[string]string{"criterion": "2"}},
					{Code: "child_not_verified", Params: map[string]string{"key": "P-REQ-2", "state": "none"}}},
				Inconsistencies: []model.VerificationReason{{Code: "status_inconsistent", Params: map[string]string{"state": "partial"}},
					{Code: "child_status_unverified", Params: map[string]string{"key": "P-REQ-2", "status": "draft"}}},
			},
			"r2": {RequirementID: "r2", Key: "P-REQ-2", State: model.VerNone, Own: model.VerNone, Reasons: []model.VerificationReason{{Code: "no_evidence"},
				{Code: "evidence_failed", Params: map[string]string{"kind": "test", "key": "P-TC-9"}}, {Code: "unknown", Text: "x"}}},
		},
		Records: []model.VerificationRecord{
			{ID: "v1", RequirementID: "r1", RequirementKey: "P-REQ-1", CriterionID: &crit, CriterionText: "Seal drawing", Method: "inspection", Result: "conditional",
				PerformedBy: "Ann", PerformedOn: "2026-02-01", Validity: "valid", Signoff: &model.RecordSignoff{By: "bo@example.com", At: at},
				Files: []model.FileRef{{ID: "f", Name: "seal.pdf"}}},
			{ID: "v2", RequirementID: "r2", RequirementKey: "P-REQ-2", Method: "review", Result: "fail", Validity: "expired", Signoff: &model.RecordSignoff{By: "x", Name: "Xi", At: at}},
			{ID: "v3", RequirementID: "gone", Method: "review", Result: "pass", Validity: "valid"},
		},
		Elements: []model.ElementCoverage{{ElementID: "e1", Name: "Power", Path: "Vehicle / Power", Kind: "subsystem", Requirements: 2, Covered: 1, Verified: 1, Tests: 1, TestsPassed: 1}},
	}
	return d
}

func TestVerificationReport(t *testing.T) {
	failOnMissing(t)
	ds, err := Build(Verification, verificationSample("sv"), nil)
	if err != nil {
		t.Fatal(err)
	}
	checkSwedish(t, ds)
	renderAll(t, ds)
	names := []string{}
	for _, s := range ds.Sections {
		names = append(names, s.Name)
	}
	if strings.Join(names, ",") != "summary,by_state,criteria,not_verified,records,hierarchy,inconsistencies,elements" {
		t.Fatal(names)
	}
	sec := func(name string) export.Section {
		for _, s := range ds.Sections {
			if s.Name == name {
				return s
			}
		}
		t.Fatal(name)
		return export.Section{}
	}
	crit := sec("criteria")
	if len(crit.Rows) != 4 || !strings.Contains(crit.Rows[0][5], "Inspektion 2026-02-01 (Ann): Godkänd med villkor") || !strings.Contains(crit.Rows[1][5], "steg 2") {
		t.Fatal(crit.Rows)
	}
	why := sec("not_verified")
	if len(why.Rows) != 2 || !strings.Contains(why.Rows[0][4], "kriterium 2 är inte godkänt än") || !strings.Contains(why.Rows[1][4], "testfall P-TC-9 underkändes") {
		t.Fatal(why.Rows)
	}
	if recs := sec("records"); len(recs.Rows) != 2 || recs.Rows[0][9] != "seal.pdf" || recs.Rows[1][1] != "hela kravet" {
		t.Fatal(recs.Rows)
	}
	if inc := sec("inconsistencies"); !strings.Contains(inc.Rows[0][3], "Delvis verifierat") {
		t.Fatal(inc.Rows)
	}
	// English, and without data
	if ds, err = Build(Verification, verificationSample("en"), nil); err != nil || ds.Sections[0].Rows[1][1] != "0 (0.0%)" {
		t.Fatal(err, ds.Sections[0].Rows)
	}
	d := sampleData()
	if ds, err = Build(Verification, d, nil); err != nil || len(ds.Sections) != 5 {
		t.Fatal(err, ds)
	}
}

func TestSpecificationVerification(t *testing.T) {
	failOnMissing(t)
	d := sampleDocData()
	d.Verification = map[string]*model.RequirementVerification{}
	for _, rq := range d.Requirements {
		v := &model.RequirementVerification{RequirementID: rq.ID, State: model.VerVerified, CriteriaTotal: len(rq.Criteria), CriteriaVerified: len(rq.Criteria)}
		for _, c := range rq.Criteria {
			v.Criteria = append(v.Criteria, model.CriterionVerification{CriterionID: c.ID, State: model.VerVerified,
				Evidence: []model.Evidence{{Kind: "test", Key: "P-TC-1", Result: "passed", State: "passed"}}})
		}
		d.Verification[rq.ID] = v
	}
	doc, err := BuildDocument(DocRequirementsSpec, d, DocOptions{Lang: "sv", Fields: RequirementDocFields})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range doc.Blocks {
		if b.Kind == export.BlockTable && len(b.Columns) > 0 && b.Columns[len(b.Columns)-1] == "Verifiering" {
			found = true
		}
	}
	if !found {
		t.Fatal("no verification column")
	}
}
