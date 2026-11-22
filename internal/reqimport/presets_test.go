package reqimport

import (
	"errors"
	"testing"
)

// The column layout of a DOORS module exported with "File > Export >
// Spreadsheet" / CSV: headings are objects with an Object Heading and no
// Object Text.
const doorsCSV = "Absolute Number,Object Level,Object Number,Object Heading,Object Text,Priority,Created By,Last Modified On\n" +
	"1,1,1,General,,,jdoe,2026-09-20\n" +
	"2,2,1-1,,The brake controller shall start up within 200 ms.,High,jdoe,2026-09-20\n" +
	"3,1,2,Braking,,,jdoe,2026-09-20\n" +
	"4,2,2.1,Emergency braking,,,jdoe,2026-09-20\n" +
	"5,3,2.1-1,,The system SHALL apply full braking force.,High,jdoe,2026-09-21\n" +
	"6,2,2.2,Parking brake,The parking brake MUST hold on a 20 % slope.,Medium,jdoe,2026-09-21\n" +
	"7,1,3,Maintenance,,,jdoe,2026-09-22\n" +
	"8,2,3-1,,Service intervals SHOULD be shown.,Low,jdoe,2026-09-22\n"

func TestDOORSPreset(t *testing.T) {
	res := mustParse(t, []byte(doorsCSV), "module.csv", Options{Preset: "doors"})
	if res.Preset != "doors" || len(res.Candidates) != 4 {
		for _, c := range res.Candidates {
			t.Logf("%+v", c)
		}
		t.Fatalf("doors: %d candidates", len(res.Candidates))
	}
	want := []struct{ id, folder, title, prio string }{
		{"2", "General", "The brake controller shall start up within", "high"},
		{"5", "Braking/Emergency braking", "The system SHALL apply full braking force", "high"},
		{"6", "Braking", "Parking brake", "medium"},
		{"8", "Maintenance", "Service intervals SHOULD be shown", "low"},
	}
	for i, w := range want {
		c := res.Candidates[i]
		if c.ExternalID != w.id || c.Folder != w.folder || c.Priority != w.prio || c.Title[:len(w.title)] != w.title {
			t.Errorf("candidate %d: %+v", i, c)
		}
	}
	// without the preset, heading rows are requirements
	if res := mustParse(t, []byte(doorsCSV), "module.csv", Options{}); len(res.Candidates) != 8 {
		t.Errorf("no preset: %d", len(res.Candidates))
	}
	// an explicit mapping still wins
	res = mustParse(t, []byte(doorsCSV), "module.csv", Options{Preset: "DOORS", Mapping: map[string]string{"created by": "owner"}})
	if res.Candidates[0].Owner != "jdoe" {
		t.Errorf("mapping over preset: %+v", res.Candidates[0])
	}
	if _, err := Parse([]byte(doorsCSV), "m.csv", Options{Preset: "jira"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("unknown preset: %v", err)
	}
}

func TestPolarionPreset(t *testing.T) {
	data := xlsxOf(t, []string{"Sheet1"}, map[string][][]any{"Sheet1": {
		{"Product Requirements Specification"},
		{"ID", "Outline Number", "Type", "Title", "Description", "Status", "Severity", "Priority", "Assignee", "Author"},
		{"ELIB-101", "1", "Heading", "User Accounts", "", "", "", "", "", "admin"},
		{"ELIB-102", "1-1", "System Requirement", "Self-service registration", "Patrons must be able to register online.", "Draft", "Must Have", "High", "anna.berg", "admin"},
		{"ELIB-103", "1-2", "System Requirement", "E-mail verification", "The portal should send a link.", "Reviewed", "Should Have", "Medium", "", "admin"},
		{"ELIB-104", "2", "Heading", "Loans", "", "", "", "", "", "admin"},
		{"ELIB-106", "2.1", "Heading", "Renewals", "", "", "", "", "", "admin"},
		{"ELIB-105", "2.1-1", "Functional Requirement", "Loan renewal", "A patron may renew a loan.", "Approved", "Nice to Have", "Low", "", "admin"},
	}}, nil)
	res := mustParse(t, data, "prs.xlsx", Options{Preset: "polarion"})
	if len(res.Candidates) != 3 {
		t.Fatalf("polarion: %+v", res.Candidates)
	}
	a, b, c := res.Candidates[0], res.Candidates[1], res.Candidates[2]
	if a.ExternalID != "ELIB-102" || a.Folder != "User Accounts" || a.Obligation != "must" || a.Owner != "anna.berg" || a.Status != "draft" {
		t.Errorf("a: %+v", a)
	}
	if b.Status != "review" || b.Obligation != "should" {
		t.Errorf("b: %+v", b)
	}
	if c.Folder != "Loans/Renewals" || c.Obligation != "may" || c.Type != "functional" || c.Status != "approved" {
		t.Errorf("c: %+v", c)
	}
	if outlineLevel("1.2-1") != 3 || outlineLevel("") != 0 || outlineLevel("3.") != 1 {
		t.Error("outlineLevel")
	}
}

func TestHeadingNumberInText(t *testing.T) {
	// no level column: "2.1 Title" gives the level
	csv := "Absolute Number,Object Heading,Object Text\n1,1 Intro,\n2,1.1 Scope,\n3,,Text A SHALL be.\n4,2 Next,\n5,,Text B SHALL be.\n"
	res := mustParse(t, []byte(csv), "m.csv", Options{Preset: "doors"})
	if len(res.Candidates) != 2 || res.Candidates[0].Folder != "1 Intro/1.1 Scope" || res.Candidates[1].Folder != "2 Next" {
		t.Errorf("numbered headings: %+v", res.Candidates)
	}
}
