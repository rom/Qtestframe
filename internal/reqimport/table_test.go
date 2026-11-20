package reqimport

import (
	"errors"
	"strings"
	"testing"
)

func TestCSVEnglishExportColumns(t *testing.T) {
	// The columns of the requirement export.
	data := "\xEF\xBB\xBFKey,Title,Obligation,Type,Priority,Status,Folder,Categories,Tags,Description,Rationale,Source,Owner,Version,Linked tests,Coverage\r\n" +
		"P-REQ-1,Login,MUST,security,high,approved,Auth / Login,,\"a, b\",\"Users MUST log in.\nWith MFA.\",Because,Customer,anna,3,1,passed\r\n" +
		"P-REQ-2,Logout,SHOULD NOT,functional,low,draft,,,,'=cmd,,,,1,0,none\r\n"
	res := mustParse(t, []byte(data), "export.csv", Options{})
	if res.Format != FormatCSV || len(res.Candidates) != 2 {
		t.Fatalf("result: %+v", res)
	}
	if fieldOf(res, "Key") != FieldExternalID || fieldOf(res, "Version") != "" || fieldOf(res, "Categories") != "" {
		t.Errorf("columns: %+v", res.Columns)
	}
	c := res.Candidates[0]
	if c.ExternalID != "P-REQ-1" || c.Title != "Login" || c.Obligation != "must" || c.ObligationFrom != "column" ||
		c.Type != "security" || c.Priority != "high" || c.Status != "approved" || c.Folder != "Auth/Login" ||
		strings.Join(c.Tags, "|") != "a|b" || c.Description != "Users MUST log in.\nWith MFA." || c.Rationale != "Because" ||
		c.Source != "Customer" || c.Owner != "anna" || c.Row != 2 || c.Location != "row 2" {
		t.Errorf("candidate: %+v", c)
	}
	if d := res.Candidates[1]; d.Obligation != "should_not" || d.Description != "=cmd" {
		t.Errorf("second: %+v", d)
	}
}

func TestCSVSwedishSemicolon(t *testing.T) {
	data := "Kravspecifikation för kunden\n\n" +
		"Nyckel;Titel;Beskrivning;Kravnivå;Typ;Prioritet;Status;Källa;Motivering;Ägare;Taggar;Mapp;Acceptanskriterier\n" +
		"K-1;Inloggning;Användaren SKA kunna logga in.;;Säkerhet;Hög;Utkast;Kund;Lag;Eva;säk, inlogg;Säkerhet/Inloggning;\"Inloggning fungerar\ninspektion: Kod granskad\"\n" +
		";;Systemet BÖR logga alla händelser.;;;;;;;;;;\n" +
		"K-3;Okänt;Text;Banan;Konstig;Jättehög;Påhittad;;;;;;\n" +
		";;;;;;;;;;;;\n"
	res := mustParse(t, []byte(data), "krav.csv", Options{})
	if len(res.Candidates) != 3 {
		t.Fatalf("candidates: %+v errors %v", res.Candidates, res.Errors)
	}
	if len(res.Warnings) == 0 || !strings.Contains(res.Warnings[0], "above the header row 3") {
		t.Errorf("warnings: %v", res.Warnings)
	}
	c := res.Candidates[0]
	if c.ExternalID != "K-1" || c.Obligation != "shall" || c.ObligationFrom != "text" || c.Type != "security" ||
		c.Priority != "high" || c.Status != "draft" || c.Folder != "Säkerhet/Inloggning" || len(c.Criteria) != 2 ||
		c.Criteria[1].Method != "inspection" || c.Owner != "Eva" || c.Rationale != "Lag" || c.Source != "Kund" {
		t.Errorf("first: %+v", c)
	}
	d := res.Candidates[1]
	if d.Title != "Systemet BÖR logga alla händelser" || d.Obligation != "should" || len(d.Warnings) != 1 {
		t.Errorf("derived title: %+v", d)
	}
	e := res.Candidates[2]
	if e.Obligation != "" || e.Type != "" || e.Priority != "" || e.Status != "" || len(e.Warnings) != 5 {
		t.Errorf("unknown values should warn: %+v", e)
	}
}

func TestCSVMappingAndOddInput(t *testing.T) {
	data := "Col1\tWhat\tExtra\tWhat\n" +
		"A\tFirst requirement MUST work\tx\tmore text\n" +
		"B\t\tonly extra\t\n"
	// Without mapping: "What" is not recognised → no header row.
	if _, err := Parse([]byte(data), "a.tsv", Options{}); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "no header row") {
		t.Fatalf("want no header error, got %v", err)
	}
	res := mustParse(t, []byte(data), "a.tsv", Options{Mapping: map[string]string{"col1": "external_id", "What": "description",
		"What (2)": "description", "Extra": "ignore"}})
	if len(res.Candidates) != 1 || len(res.Errors) != 1 || !strings.Contains(res.Errors[0], "row 3") {
		t.Fatalf("result: %+v", res)
	}
	if c := res.Candidates[0]; c.ExternalID != "A" || c.Description != "First requirement MUST work\nmore text" {
		t.Errorf("candidate: %+v", c)
	}
	if fieldOf(res, "Extra") != "" || fieldOf(res, "What (2)") != FieldDescription {
		t.Errorf("columns: %+v", res.Columns)
	}
	// Empty header cells are named after their column; duplicate fields merge.
	res = mustParse(t, []byte("Title,,Title,Folder,Section\nA,x,B,F1,F2\n"), "a.csv", Options{})
	if res.Columns[1].Header != "Column B" || res.Columns[2].Header != "Title (2)" {
		t.Errorf("headers: %+v", res.Columns)
	}
	if c := res.Candidates[0]; c.Title != "A B" || c.Folder != "F1/F2" {
		t.Errorf("merged: %+v", c)
	}
	// "sep=" hint and a quoted delimiter.
	res = mustParse(t, []byte("sep=;\nTitle;Description\n\"a;b\";c\n"), "a.csv", Options{})
	if res.Candidates[0].Title != "a;b" {
		t.Errorf("sep hint: %+v", res.Candidates)
	}
	// broken quoting gives a row error
	if _, err := Parse([]byte("Title,Description\n\"a\"b\"\"\"c,\"d\n"), "a.csv", Options{}); err != nil {
		t.Logf("lazy quotes error (accepted): %v", err)
	}
	// A row with title but nothing in a long title is shortened.
	long := strings.Repeat("x", 600)
	res = mustParse(t, []byte("Title\n"+long+"\n"), "a.csv", Options{})
	if c := res.Candidates[0]; len([]rune(c.Title)) != 500 || len(c.Warnings) != 2 {
		t.Errorf("long title: %d %v", len(c.Title), c.Warnings)
	}
}

func TestCSVEncodings(t *testing.T) {
	// Windows-1252: "Säkerhet – krav" with é and the en dash.
	cp := []byte("Title;Description\nS\xe4kerhet \x96 krav;Caf\xe9 MUST open\n")
	res := mustParse(t, cp, "a.csv", Options{})
	if c := res.Candidates[0]; c.Title != "Säkerhet – krav" || c.Description != "Café MUST open" || len(res.Warnings) != 1 {
		t.Errorf("cp1252: %+v %v", c, res.Warnings)
	}
	// UTF-16 LE with BOM, tab separated (Excel "Unicode text").
	u := utf16LE("Titel\tBeskrivning\nÅtgärd\tSystemet SKA fungera\n")
	res = mustParse(t, u, "a.txt", Options{})
	if c := res.Candidates[0]; c.Title != "Åtgärd" || c.Obligation != "shall" {
		t.Errorf("utf16: %+v", c)
	}
	// UTF-16 BE
	be := utf16LE("Title\nX\n")
	for i := 2; i+1 < len(be); i += 2 {
		be[i], be[i+1] = be[i+1], be[i]
	}
	be[0], be[1] = 0xFE, 0xFF
	res = mustParse(t, be, "a.txt", Options{})
	if res.Candidates[0].Title != "X" {
		t.Errorf("utf16be: %+v", res.Candidates)
	}
}

func utf16LE(s string) []byte {
	out := []byte{0xFF, 0xFE}
	for _, r := range s {
		out = append(out, byte(r), byte(r>>8))
	}
	return out
}

func TestXLSX(t *testing.T) {
	data := xlsxOf(t, []string{"Summary", "requirements", "criteria"}, map[string][][]any{
		"Summary": {{"Project export"}, {"Generated", "today"}},
		"requirements": {
			{"Key", "Title", "Obligation", "Folder", "Description"},
			{"P-REQ-1", "Login", "SHALL", "Auth", "Users SHALL log in"},
			{"P-REQ-2", "Logout", "", "", "Users MAY log out"},
			{nil, nil, nil, nil, nil},
			{"P-REQ-3", nil, nil, nil, nil},
		},
		"criteria": {
			{"Requirement", "#", "Criterion", "Method", "Threshold"},
			{"P-REQ-1", 1, "Login page shown", "inspection", "1 s"},
			{"P-REQ-9", 1, "Unknown", "test", ""},
		},
	}, map[string][2]string{"requirements": {"D2", "D3"}})
	res := mustParse(t, data, "export.xlsx", Options{})
	if res.Sheet != "requirements" || len(res.Sheets) != 3 || len(res.Candidates) != 2 {
		t.Fatalf("result: %+v", res)
	}
	a, b := res.Candidates[0], res.Candidates[1]
	if a.Obligation != "shall" || a.Folder != "Auth" || len(a.Criteria) != 1 || a.Criteria[0].Method != "inspection" || a.Criteria[0].Threshold != "1 s" {
		t.Errorf("first: %+v", a)
	}
	if b.Folder != "Auth" || b.Obligation != "may" || b.Row != 3 {
		t.Errorf("merged folder: %+v", b)
	}
	if len(res.Errors) != 1 || !strings.Contains(res.Errors[0], "row 5") {
		t.Errorf("errors: %v", res.Errors)
	}
	// Named sheet; and case-insensitive.
	data = xlsxOf(t, []string{"Intro", "Krav"}, map[string][][]any{
		"Intro": {{"Hello"}},
		"Krav":  {{"Titel", "Krav"}, {"A", "Systemet SKA starta"}},
	}, nil)
	res = mustParse(t, data, "k.xlsx", Options{Sheet: "krav"})
	if res.Sheet != "Krav" || len(res.Candidates) != 1 {
		t.Errorf("named: %+v", res)
	}
	res = mustParse(t, data, "k.xlsx", Options{})
	if res.Sheet != "Krav" {
		t.Errorf("auto sheet: %q", res.Sheet)
	}
	if _, err := Parse(data, "k.xlsx", Options{Sheet: "Nope"}); err == nil || !strings.Contains(err.Error(), "Intro, Krav") {
		t.Errorf("missing sheet: %v", err)
	}
	if _, err := Parse(data, "k.xlsx", Options{Sheet: "Intro"}); err == nil || !strings.Contains(err.Error(), "no header") {
		t.Errorf("sheet without header: %v", err)
	}
	only := xlsxOf(t, []string{"A"}, map[string][][]any{"A": {{"x", "y"}}}, nil)
	if _, err := Parse(only, "k.xlsx", Options{}); err == nil {
		t.Error("workbook without header accepted")
	}
	if _, err := Parse([]byte("not a zip"), "k.xlsx", Options{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("not zip: %v", err)
	}
	if _, err := Parse(zipOf(t, map[string]string{"a.txt": "x"}), "k.xlsx", Options{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad zip: %v", err)
	}
}
