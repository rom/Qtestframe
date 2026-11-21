package report

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
)

func ptrS(s string) *string { return &s }

// sampleDocData is a small project with folders, categories, criteria,
// steps (one calling another test), links, a release, runs, a
// configuration, members and a baseline.
func sampleDocData() *DocData {
	t1 := t0.Add(time.Hour)
	return &DocData{
		Project: model.Project{Key: "P", Name: "Shop", Description: "The web shop and its back office."},
		Folders: []model.Folder{
			{ID: "f1", Kind: "requirement", Name: "Checkout", Description: "Paying for the cart.", Position: 2},
			{ID: "f2", Kind: "requirement", Name: "Payment", ParentID: ptrS("f1"), Position: 1},
			{ID: "f3", Kind: "requirement", Name: "Empty folder", Position: 1},
			{ID: "tf", Kind: "test", Name: "Checkout tests"},
		},
		Categories: map[string]model.Category{
			"c1": {ID: "c1", Scope: "requirement", Dimension: "Level", Name: "System"},
			"c2": {ID: "c2", Scope: "requirement", Dimension: "Level", Name: "Component"},
		},
		Requirements: []model.Requirement{
			{ID: "r1", Key: "P-REQ-1", Title: "Pay by card", FolderID: ptrS("f2"), Description: "The system MUST accept card payments.\n\nIt SHOULD accept wallets; it may be slow.",
				Rationale: "Most customers pay by card.", Type: "functional", Priority: "high", Obligation: "must", Status: "approved", Source: "Customer",
				Owner: "po@example.com", Tags: []string{"pay"}, CategoryIDs: []string{"c1"}, Version: 3, FolderPath: "Checkout / Payment",
				Criteria:     []model.EvaluationCriterion{{Description: "Visa is accepted", Method: "test", Threshold: "2 s"}},
				RelationRefs: []model.ItemRef{{Kind: "requirement", Key: "P-REQ-2"}, {Kind: "test", Key: "P-TC-1"}}},
			{ID: "r2", Key: "P-REQ-2", Title: "Receipt", FolderID: ptrS("f1"), Description: "Systemet SKA skicka ett kvitto.", Type: "security", Obligation: "shall", Status: "draft", CategoryIDs: []string{"c2"}},
			{ID: "r3", Key: "P-REQ-10", Title: "Loose", Description: "No keyword here.", Type: "", Status: "review", Source: "Standard"},
		},
		Tests: []model.TestCase{
			{ID: "t1", Key: "P-TC-1", Title: "Card payment", FolderID: ptrS("tf"), Description: "Pay with a test card.", Preconditions: "A cart with one item.",
				ExecutionType: "manual", Priority: "high", Status: "ready", EstimatedMin: 10, Tags: []string{"smoke"}, Version: 2,
				Steps:          []model.TestStep{{Action: "Open the cart", Data: "cart 1", Expected: "The cart is shown"}, {CallTestID: ptrS("t2")}},
				ExpectedResult: "The order is paid.", Parameters: []string{"card"}, DataRows: []model.DataRow{{Name: "visa", Values: map[string]string{"card": "4111"}}, {Values: map[string]string{"card": "5500"}}}},
			{ID: "t2", Key: "P-TC-2", Title: "Log in", ExecutionType: "automated", ScriptLanguage: "playwright", Priority: "medium", Status: "draft"},
		},
		AllTests: map[string]model.TestRef{"t1": {ID: "t1", Key: "P-TC-1", Title: "Card payment"}, "t2": {ID: "t2", Key: "P-TC-2", Title: "Log in"}},
		Links: []model.TraceLink{
			{RequirementID: "r1", TestCaseID: "t1", RequirementKey: "P-REQ-1", TestCaseKey: "P-TC-1"},
			{RequirementID: "r1", TestCaseID: "t1", RequirementKey: "P-REQ-1", TestCaseKey: "P-TC-1"}, // duplicate (per criterion)
			{RequirementID: "r2", TestCaseID: "t2", RequirementKey: "P-REQ-2", TestCaseKey: "P-TC-2"},
			{RequirementID: "rx", TestCaseID: "t2", RequirementKey: "P-REQ-99", TestCaseKey: "P-TC-2"},
		},
		LastResults:   map[string]string{"t1": "passed"},
		Prerequisites: map[string][]string{"t1": {"P-TC-2"}},
		Releases: []model.Release{
			{Name: "R1", Version: "1.0", Status: "active", TargetDate: "2026-12-01", ExitCriteria: []model.ExitCriterion{{Type: "coverage", Threshold: 90}, {Type: "no_open_defects", Level: "major"},
				{Type: "risks_mitigated"}, {Type: "trace_rules", Level: "warning"}, {Type: "trace_rules"}, {Type: "passed"}, {Type: "mandatory_passed"}, {Type: "executed"},
				{Type: "no_failed"}, {Type: "approved"}, {Type: "no_suspect"}, {Type: "no_open_defects"}, {Type: "custom_one"}}, Requirements: 2, Runs: 1},
			{Name: "R0", Status: "released"},
			{Name: "Old", Status: "cancelled"},
		},
		Runs: []model.TestRun{
			{Name: "Sprint 1", Status: "completed", Environment: "staging", Build: "1.0", CreatedAt: t0, StartedAt: &t0, CompletedAt: &t1, ReleaseName: "R1", Summary: map[string]int{"total": 2}},
			{Name: "Sprint 2", Status: "planned", Environment: "staging", Build: "1.1", CreatedAt: t1},
		},
		Configurations: []model.Configuration{{Name: "Chrome", Description: "Desktop", Values: []model.DimensionValue{{Dimension: "Browser", Value: "Chrome"}}}},
		Members:        []model.ProjectMember{{Email: "t@example.com", Name: "Tess", Role: "tester"}, {Email: "m@example.com", Role: "manager"}, {Email: "a@example.com", Role: "admin"}, {Email: "v@example.com", Role: "viewer"}, {Email: "c@example.com", Role: "custom_role"}},
		Settings:       model.TenantSettings{AllowUnreadyTests: true},
		Baseline:       &model.Baseline{Name: "BL-1", CreatedAt: t0},
		Signoffs:       []model.Signoff{{UserName: "Ann", Meaning: "approved", SignedAt: t0, Valid: true}, {UserEmail: "bo@example.com", Meaning: "reviewed", SignedAt: t0}},
		Revisions:      []Revision{{Version: "1.0", Date: t0, Author: "ann@example.com", Name: "spec-1.pdf"}, {Version: "v2.1", Date: t0, Author: "ann@example.com", Name: "spec-2.pdf"}},
		Now:            t0,
	}
}

// docText renders doc as Markdown (the most inspectable format).
func docText(t *testing.T, doc *export.Document) string {
	t.Helper()
	var b bytes.Buffer
	if err := export.WriteDocument(&b, export.MD, doc); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestRequirementsSpec(t *testing.T) {
	d := sampleDocData()
	doc, err := BuildDocument(DocRequirementsSpec, d, DocOptions{Author: "Ann", Fields: []string{"description", "rationale", "criteria", "type", "priority",
		"obligation", "status", "source", "owner", "tags", "categories", "relations", "tests", "results", "version"}})
	if err != nil {
		t.Fatal(err)
	}
	md := docText(t, doc)
	for _, want := range []string{
		"# Requirements specification", `Version:** 3\.0`, "**Document ID:** P-SRS", "BL-1 (2026",
		"## Document control", "### Revision history", "| 3.0 |", "This version", "### Approvals", "| Ann | approved |",
		"## Contents", "[1 Introduction](#1-introduction)", `  - [1\.1 Purpose](#11-purpose)`, "[Appendix A. Traceability matrix](#appendix-a-traceability-matrix)",
		"## 1 Introduction", "The web shop and its back office.", `The key words "MUST"`, "BCP 14",
		"## 2 Requirements", "### 2.1 Without folder", "#### P-REQ-10 Loose", "### 2.2 Checkout", "Paying for the cart.", "#### P-REQ-2 Receipt", "#### 2.2.1 Payment", "##### P-REQ-1 Pay by card",
		"The system **MUST** accept card payments.", "It **SHOULD** accept wallets; it may be slow.", "Systemet **SKA** skicka",
		"**Verified by:** P-TC-1 (Passed)", "**Related requirements:** P-REQ-2", "**Categories:** Level: System", "**Rationale:** Most customers pay by card.",
		"| 1 | Visa is accepted | Test | 2 s |", "**Source:** Customer",
		"## Appendix A. Traceability matrix", "| P-REQ-1 | Pay by card | MUST | P-TC-1 (Passed) |", "1 requirement has no test.",
		"## Appendix B. Glossary and references", "ISO/IEC/IEEE 29148:2018", "| Customer | Source of 1 requirement |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in\n%s", want, md)
		}
	}
	if strings.Contains(md, "Empty folder") {
		t.Error("folders without requirements are left out")
	}
	if strings.Contains(md, "**may**") {
		t.Error("a lowercase RFC keyword is not emphasised")
	}
	// order: Without folder, then Checkout (position 2 after Empty folder 1, which is empty)
	if strings.Index(md, "P-REQ-10 Loose") > strings.Index(md, "P-REQ-2 Receipt") {
		t.Error("items without folder come first")
	}

	// fields can be left out; no appendices
	f := false
	doc, err = BuildDocument(DocRequirementsSpec, sampleDocData(), DocOptions{Fields: []string{"description"}, Traceability: &f, Glossary: &f, Title: "SRS", Version: "7"})
	if err != nil {
		t.Fatal(err)
	}
	md = docText(t, doc)
	if strings.Contains(md, "Verified by") || strings.Contains(md, "Appendix") || !strings.Contains(md, "# SRS") || !strings.Contains(md, "| 7 |") {
		t.Errorf("fields and appendices not honoured:\n%s", md)
	}
}

func TestRequirementsSpecGrouping(t *testing.T) {
	doc, err := BuildDocument(DocRequirementsSpec, sampleDocData(), DocOptions{GroupBy: GroupByType})
	if err != nil {
		t.Fatal(err)
	}
	md := docText(t, doc)
	for _, want := range []string{"### 2.1 Functional", "### 2.2 Security", "### 2.3 Without type"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q", want)
		}
	}
	doc, _ = BuildDocument(DocRequirementsSpec, sampleDocData(), DocOptions{GroupBy: GroupByCategory})
	md = docText(t, doc)
	for _, want := range []string{"### 2.1 Level: Component", "### 2.2 Level: System", "### 2.3 Without category", "Category (Level)"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q", want)
		}
	}
	d := sampleDocData()
	d.Folders, d.Categories = nil, nil
	doc, _ = BuildDocument(DocRequirementsSpec, d, DocOptions{GroupBy: GroupByCategory, Dimension: "Nope"})
	if md = docText(t, doc); !strings.Contains(md, "### 2.1 Without category") {
		t.Error("unknown dimension: everything without category")
	}
	doc, _ = BuildDocument(DocRequirementsSpec, d, DocOptions{})
	if md = docText(t, doc); strings.Contains(md, "Without folder") || !strings.Contains(md, "### P-REQ-1 Pay by card") {
		t.Errorf("no folders: the requirements follow the chapter heading directly:\n%s", md)
	}
}

func TestTestPlan(t *testing.T) {
	d := sampleDocData()
	doc, err := BuildDocument(DocTestPlan, d, DocOptions{Author: "Tess", Fields: TestDocFields})
	if err != nil {
		t.Fatal(err)
	}
	md := docText(t, doc)
	for _, want := range []string{
		"# Test plan", "P-TP", "29119-3",
		"## 2 Test items and features to be tested", "| P-REQ-1 | Pay by card | Checkout / Payment | MUST | High | 1 |",
		"1 requirement in scope is not verified",
		"## 3 Test approach", "Test cases by execution type", "| Manual | 1 |", "### 3.2 Entry criteria", "it has the status Ready; (Not enforced",
		"the requirements it verifies have one of the statuses Approved, Implemented, Verified;", "1 of 2 test cases have the status Ready.",
		"### 3.3 Exit criteria", "**R1 1.0** (Active)", "At least 90% of the requirements in scope have a test.", "severity Major or higher",
		"risk of level Low or higher", "without errors or warnings", "Custom one", "The release has no exit criteria.",
		"## 4 Test environment", "| Chrome | Browser: Chrome | Desktop |", "| staging | 1.0, 1.1 | 2 |",
		"## 5 Roles and responsibilities", "| Tess | t@example.com | Tester |", "Custom role",
		"## 6 Schedule", "| R1 | 1.0 | Active | 2026-12-01 | 2 | 1 |", "| Sprint 1 | Completed | R1 |",
		"## 7 Test specification", "### 7.2 Checkout tests", "#### P-TC-1 Card payment", "**Execution type:** Manual", "**Verifies:** P-REQ-1",
		"**Prerequisites:** P-TC-2", "**Latest result:** Passed", "**Estimated time:** 10 min", "**Preconditions**", "A cart with one item.",
		"| 1 | Open the cart | cart 1 | The cart is shown |", "| 2 | Call P-TC-2 Log in |", "The order is paid.", "| visa | 4111 |", "| 2 | 5500 |",
		"### 7.1 Without folder", "**Execution type:** Automated (playwright)", "**Verifies:** P-REQ-2, P-REQ-99", "**Latest result:** Not run",
		"## Appendix A. Coverage", "| P-REQ-10 | Loose | Unspecified |", "### A.2 Requirements without tests", "| P-REQ-10 | Loose | Review |",
		"ISO/IEC/IEEE 29119-3:2021", "Exit criteria",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in\n%s", want, md)
		}
	}

	// no releases, runs, configurations: the defaults and placeholders
	d = sampleDocData()
	d.Releases, d.Runs, d.Configurations, d.Members, d.Tests, d.Baseline = nil, nil, nil, nil, nil, nil
	doc, err = BuildDocument(DocTestPlan, d, DocOptions{})
	if err != nil {
		t.Fatal(err)
	}
	md = docText(t, doc)
	for _, want := range []string{"default exit criteria", "At least 100% of the requirements in scope have a test.", "No test configurations are defined",
		"No releases or test runs are planned yet.", "No test cases match the selection.", "Current data of the project"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(md, "Approvals") {
		t.Error("approvals only with a baseline")
	}
}

func TestTestPlanManyRuns(t *testing.T) {
	d := sampleDocData()
	d.Runs = nil
	for i := 0; i < 60; i++ {
		d.Runs = append(d.Runs, model.TestRun{Name: fmt.Sprintf("Run %d", i), Status: "completed", CreatedAt: t0.Add(time.Duration(i) * time.Hour)})
	}
	doc, _ := BuildDocument(DocTestPlan, d, DocOptions{})
	md := docText(t, doc)
	if !strings.Contains(md, "The 50 most recent of 60 test runs.") || strings.Contains(md, "| Run 9 |") || !strings.Contains(md, "| Run 59 |") {
		t.Error("the schedule lists the 50 most recent runs")
	}
}

func TestDocOptionsNormalize(t *testing.T) {
	var o DocOptions
	if err := o.Normalize("nope"); err == nil {
		t.Error("unknown kind")
	}
	if err := (&DocOptions{GroupBy: "x"}).Normalize(DocRequirementsSpec); err == nil {
		t.Error("unknown grouping")
	}
	if err := (&DocOptions{Fields: []string{"steps"}}).Normalize(DocRequirementsSpec); err == nil {
		t.Error("test field in a specification")
	}
	if err := (&DocOptions{Title: strings.Repeat("x", 201)}).Normalize(DocTestPlan); err == nil {
		t.Error("long title")
	}
	o = DocOptions{GroupBy: "type", Fields: []string{"steps", "steps", " tags "}}
	if err := o.Normalize(DocTestPlan); err != nil || o.GroupBy != GroupByFolder || len(o.Fields) != 2 || !*o.Traceability || !*o.Glossary {
		t.Errorf("normalized %+v, %v", o, err)
	}
	o = DocOptions{}
	_ = o.Normalize(DocRequirementsSpec)
	if o.has("results") || !o.has("tests") {
		t.Error("default fields: all but results")
	}
	if NextVersion(nil) != "1.0" || NextVersion([]Revision{{Version: "v3.2"}, {Version: "x"}, {Version: "2"}}) != "4.0" {
		t.Error("NextVersion")
	}
	if DocTag(DocTestPlan) != "test-plan" || DocTag(DocRequirementsSpec) != "requirements-specification" {
		t.Error("DocTag")
	}
	if DocTitle(DocTestPlan, "sv") != "Testplan" || DocTitle(DocRequirementsSpec, "") != "Requirements specification" {
		t.Error("DocTitle")
	}
	if _, err := BuildDocument("x", sampleDocData(), DocOptions{}); err == nil {
		t.Error("BuildDocument checks the kind")
	}
}

// TestDocumentsSwedish: every text of both documents is translated.
func TestDocumentsSwedish(t *testing.T) {
	failOnMissing(t)
	for _, kind := range DocKinds {
		for _, group := range DocGroupings {
			fields := RequirementDocFields
			if kind == DocTestPlan {
				fields = TestDocFields
			}
			d := sampleDocData()
			doc, err := BuildDocument(kind, d, DocOptions{Lang: "sv", GroupBy: group, Fields: fields})
			if err != nil {
				t.Fatal(err)
			}
			md := docText(t, doc)
			d.Releases, d.Runs, d.Configurations, d.Tests, d.Requirements, d.Signoffs = nil, nil, nil, nil, nil, nil
			doc, _ = BuildDocument(kind, d, DocOptions{Lang: "sv", GroupBy: group})
			md += docText(t, doc)
			for _, en := range []string{"Introduction", "Purpose", "Document control", "Revision history", "Glossary", "Traceability matrix", "Test approach", "Without folder"} {
				if strings.Contains(md, en) {
					t.Errorf("%s/%s: English %q in the Swedish document", kind, group, en)
				}
			}
			for _, sv := range []string{"Inledning", "Syfte", "Dokumentinformation", "Revisionshistorik", "Innehåll"} {
				if !strings.Contains(md, sv) {
					t.Errorf("%s/%s: missing %q", kind, group, sv)
				}
			}
		}
	}
	doc, _ := BuildDocument(DocRequirementsSpec, sampleDocData(), DocOptions{Lang: "sv"})
	if md := docText(t, doc); !strings.Contains(md, "SKA") || !strings.Contains(md, "BÖR") || !strings.Contains(md, "FÅR") || !strings.Contains(md, "# Kravspecifikation") {
		t.Errorf("Swedish key words SKA/BÖR/FÅR:\n%s", md)
	}
}

// TestDocumentsLarge: 2,000 requirements and tests render quickly in every
// format.
func TestDocumentsLarge(t *testing.T) {
	d := largeDocData(2000)
	for _, kind := range DocKinds {
		start := time.Now()
		doc, err := BuildDocument(kind, d, DocOptions{})
		if err != nil {
			t.Fatal(err)
		}
		built := time.Since(start)
		for _, f := range export.DocumentFormats {
			s := time.Now()
			var b bytes.Buffer
			if err := export.WriteDocument(&b, f, doc); err != nil {
				t.Fatal(err)
			}
			took := time.Since(s)
			t.Logf("%s %s: built in %v, written in %v, %d KB", kind, f, built, took, b.Len()>>10)
			if took > 30*time.Second {
				t.Errorf("%s %s took %v", kind, f, took)
			}
		}
	}
}

func largeDocData(n int) *DocData {
	d := &DocData{Project: model.Project{Key: "BIG", Name: "Big"}, AllTests: map[string]model.TestRef{}, Now: t0}
	for f := 0; f < 20; f++ {
		d.Folders = append(d.Folders, model.Folder{ID: fmt.Sprintf("rf%d", f), Kind: "requirement", Name: fmt.Sprintf("Area %d", f)},
			model.Folder{ID: fmt.Sprintf("tf%d", f), Kind: "test", Name: fmt.Sprintf("Suite %d", f)})
	}
	for i := 0; i < n; i++ {
		rid, tid := fmt.Sprintf("r%d", i), fmt.Sprintf("t%d", i)
		d.Requirements = append(d.Requirements, model.Requirement{ID: rid, Key: fmt.Sprintf("BIG-REQ-%d", i+1), Title: fmt.Sprintf("Requirement %d", i),
			FolderID: ptrS(fmt.Sprintf("rf%d", i%20)), Description: "The system SHALL do thing " + fmt.Sprint(i) + " and it SHOULD log it. " + strings.Repeat("Some more text. ", 10),
			Obligation: "shall", Type: "functional", Priority: "medium", Status: "approved", Criteria: []model.EvaluationCriterion{{Description: "works", Method: "test"}}})
		d.Tests = append(d.Tests, model.TestCase{ID: tid, Key: fmt.Sprintf("BIG-TC-%d", i+1), Title: fmt.Sprintf("Test %d", i), FolderID: ptrS(fmt.Sprintf("tf%d", i%20)),
			ExecutionType: "manual", Priority: "medium", Status: "ready", Preconditions: "Logged in.",
			Steps: []model.TestStep{{Action: "Do it", Expected: "Done"}, {Action: "Check it", Data: "x", Expected: "Fine"}, {Action: "Undo it", Expected: "Gone"}}})
		d.AllTests[tid] = model.TestRef{ID: tid, Key: fmt.Sprintf("BIG-TC-%d", i+1)}
		d.Links = append(d.Links, model.TraceLink{RequirementID: rid, TestCaseID: tid, RequirementKey: fmt.Sprintf("BIG-REQ-%d", i+1), TestCaseKey: fmt.Sprintf("BIG-TC-%d", i+1)})
	}
	return d
}
