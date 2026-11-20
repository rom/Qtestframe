package reqimport

import (
	"errors"
	"strings"
	"testing"
)

func TestGuessField(t *testing.T) {
	cases := map[string]string{
		"Title": FieldTitle, "TITEL": FieldTitle, "Rubrik": FieldTitle, "Name": FieldTitle,
		"Description": FieldDescription, "Beskrivning": FieldDescription, "Krav": FieldDescription,
		"Requirement": FieldDescription, "Text": FieldDescription, "Requirement text": FieldDescription,
		"ID": FieldExternalID, "Key": FieldExternalID, "Nyckel": FieldExternalID, "Ref": FieldExternalID,
		"Requirement ID": FieldExternalID, "Krav-ID": FieldExternalID, "Req. Nr": FieldExternalID,
		"Obligation": FieldObligation, "Kravnivå": FieldObligation,
		"Type": FieldType, "Typ": FieldType, "Priority": FieldPriority, "Prioritet": FieldPriority,
		"Status": FieldStatus, "Source": FieldSource, "Källa": FieldSource, "Rationale": FieldRationale,
		"Motivering": FieldRationale, "Owner": FieldOwner, "Ägare": FieldOwner, "Tags": FieldTags,
		"Taggar": FieldTags, "Folder": FieldFolder, "Mapp": FieldFolder, "Section": FieldFolder,
		"Avsnitt": FieldFolder, "Criteria": FieldCriteria, "Acceptance criteria": FieldCriteria,
		"Acceptanskriterier": FieldCriteria, "Title *": FieldTitle, "Priority (1-4)": FieldPriority,
		"ReqIF.Text": FieldDescription, "ReqIF.Name": FieldTitle, "ReqIF.ForeignID": FieldExternalID,
		"Short title": FieldTitle, "Detailed description": FieldDescription, "Requirement status": FieldStatus,
		"Requirement owner": FieldOwner, "Requirement source": FieldSource, "Product area section": FieldFolder,
		"Main rationale": FieldRationale, "Prioritering": FieldPriority, "Kravtyp": FieldType,
		"Labels": FieldTags, "Obligation level": FieldObligation, "Main type": FieldType,
	}
	for h, want := range cases {
		if got, _ := GuessField(h); got != want {
			t.Errorf("GuessField(%q) = %q, want %q", h, got, want)
		}
	}
	for _, h := range []string{"", "Version", "Coverage", "Foo bar"} {
		if got, _ := GuessField(h); got != "" {
			t.Errorf("GuessField(%q) = %q, want ignored", h, got)
		}
	}
}

func TestValueParsers(t *testing.T) {
	ob := map[string]string{"MUST": "must", "must not": "must_not", "SHALL_NOT": "shall_not", "SKA": "shall",
		"BÖR": "should", "får inte": "must_not", "FÅR": "may", "should_not": "should_not", "The system SHALL": "shall",
		"Måste": "must", "": ""}
	for in, want := range ob {
		if got, ok := ParseObligation(in); !ok || got != want {
			t.Errorf("ParseObligation(%q) = %q %v, want %q", in, got, ok, want)
		}
	}
	if _, ok := ParseObligation("banana"); ok {
		t.Error("banana is no obligation")
	}
	if v, _ := ParseType("Icke-funktionell"); v != "non_functional" {
		t.Errorf("type: %q", v)
	}
	if v, _ := ParseType("non_functional"); v != "non_functional" {
		t.Errorf("type code: %q", v)
	}
	if v, _ := ParsePriority("Hög"); v != "high" {
		t.Errorf("priority: %q", v)
	}
	if v, _ := ParseStatus("Godkänd"); v != "approved" {
		t.Errorf("status: %q", v)
	}
	if v, _ := ParseMethod("Inspektion"); v != "inspection" {
		t.Errorf("method: %q", v)
	}
	if _, ok := ParseStatus("whatever"); ok {
		t.Error("unknown status accepted")
	}
}

func TestTextHelpers(t *testing.T) {
	if id, rest := splitID("REQ-12: The system SHALL log in."); id != "REQ-12" || rest != "The system SHALL log in." {
		t.Errorf("splitID: %q %q", id, rest)
	}
	if id, rest := splitID("[SRS-001] Användaren SKA kunna logga in"); id != "SRS-001" || !strings.HasPrefix(rest, "Användaren") {
		t.Errorf("splitID bracket: %q %q", id, rest)
	}
	if id, _ := splitID("K12 - Systemet SKA starta"); id != "K12" {
		t.Errorf("splitID dash: %q", id)
	}
	if id, rest := splitID("The system SHALL: work"); id != "" || rest != "The system SHALL: work" {
		t.Errorf("no id expected: %q %q", id, rest)
	}
	if id, _ := splitID("REQ-1:"); id != "" {
		t.Errorf("id without text: %q", id)
	}
	if got := deriveTitle("The system shall allow users to reset their password by e-mail."); got != "The system shall allow users to reset their…" {
		t.Errorf("deriveTitle: %q", got)
	}
	if got := deriveTitle("Short one."); got != "Short one" {
		t.Errorf("deriveTitle short: %q", got)
	}
	if got := joinFolder("A / B", "C\\D", " > E"); got != "A/B/C/D/E" {
		t.Errorf("joinFolder: %q", got)
	}
	if got := FolderParts("A/ /B"); len(got) != 2 {
		t.Errorf("FolderParts: %v", got)
	}
	if got := cleanCell("'=SUM(A1)"); got != "=SUM(A1)" {
		t.Errorf("cleanCell: %q", got)
	}
	if got := cleanCell("'quoted"); got != "'quoted" {
		t.Errorf("cleanCell keeps quotes: %q", got)
	}
	cr := parseCriteria("- Login works\n2) inspection: Code reviewed\n[analys] Load model\n\n• plain")
	if len(cr) != 4 || cr[0].Method != "" || cr[1].Method != "inspection" || cr[1].Description != "Code reviewed" ||
		cr[2].Method != "analysis" || cr[2].Description != "Load model" || cr[3].Description != "plain" {
		t.Errorf("criteria: %+v", cr)
	}
	if tags := splitTags("a, b;#c | d"); strings.Join(tags, ",") != "a,b,c,d" {
		t.Errorf("tags: %v", tags)
	}
	if columnName(0) != "A" || columnName(25) != "Z" || columnName(26) != "AA" {
		t.Error("columnName")
	}
	if truncRunes("åäö", 5) != "åäö" || truncRunes("abcdef", 4) != "abc…" {
		t.Error("truncRunes")
	}
}

func TestDetectFormat(t *testing.T) {
	cases := []struct{ explicit, name, data, want string }{
		{"", "a.csv", "x", FormatCSV},
		{"", "a.XLSX", "x", FormatXLSX},
		{"", "a.reqifz", "x", FormatReqIF},
		{"", "a.docx", "x", FormatDOCX},
		{"excel", "a.bin", "x", FormatXLSX},
		{"word", "", "x", FormatDOCX},
		{"reqifz", "", "x", FormatReqIF},
		{"CSV", "", "x", FormatCSV},
		{"", "upload", `<?xml version="1.0"?><REQ-IF>`, FormatReqIF},
		{"", "upload", "Title,Description", FormatCSV},
	}
	for _, c := range cases {
		got, err := DetectFormat(c.explicit, c.name, []byte(c.data))
		if err != nil || got != c.want {
			t.Errorf("DetectFormat(%q,%q) = %q %v, want %q", c.explicit, c.name, got, err, c.want)
		}
	}
	for _, c := range []struct{ explicit, name string }{{"pdf", ""}, {"", "a.xls"}, {"", "a.doc"}} {
		if _, err := DetectFormat(c.explicit, c.name, []byte("x")); !errors.Is(err, ErrInvalid) {
			t.Errorf("DetectFormat(%q,%q): want ErrInvalid, got %v", c.explicit, c.name, err)
		}
	}
	// sniffed ZIP content
	docx := docxOf(t, wPara("", "x", false), "")
	if f, _ := DetectFormat("", "upload", docx); f != FormatDOCX {
		t.Errorf("sniff docx: %q", f)
	}
	xl := xlsxOf(t, []string{"S"}, map[string][][]any{"S": {{"Title"}}}, nil)
	if f, _ := DetectFormat("", "upload", xl); f != FormatXLSX {
		t.Errorf("sniff xlsx: %q", f)
	}
	rz := zipOf(t, map[string]string{"spec.reqif": "<REQ-IF/>"})
	if f, _ := DetectFormat("", "upload", rz); f != FormatReqIF {
		t.Errorf("sniff reqifz: %q", f)
	}
	if _, err := DetectFormat("", "upload", zipOf(t, map[string]string{"x.txt": "x"})); err == nil {
		t.Error("unknown zip accepted")
	}
	if _, err := DetectFormat("", "upload", []byte("PK\x03\x04garbage")); err == nil {
		t.Error("damaged zip accepted")
	}
}

func TestParseOptionErrors(t *testing.T) {
	if _, err := Parse([]byte("  \n"), "a.csv", Options{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("empty: %v", err)
	}
	if _, err := Parse([]byte("Title\nx"), "a.csv", Options{Mapping: map[string]string{"Title": "nonsense"}}); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad mapping: %v", err)
	}
	var b strings.Builder
	b.WriteString("Title\n")
	for i := 0; i < 12; i++ {
		b.WriteString("Requirement\n")
	}
	if _, err := Parse([]byte(b.String()), "a.csv", Options{MaxCandidates: 10}); err == nil || !strings.Contains(err.Error(), "at most 10") {
		t.Errorf("limit: %v", err)
	}
	res := mustParse(t, []byte("Title,Description\n,\n"), "a.csv", Options{})
	if len(res.Candidates) != 0 || len(res.Warnings) == 0 {
		t.Errorf("no candidates should warn: %+v", res)
	}
}
