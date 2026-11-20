package reqimport

import (
	"errors"
	"strings"
	"testing"
)

const testStyles = `<w:style w:type="paragraph" w:styleId="Rubrik1"><w:name w:val="heading 1"/></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Rubrik2"><w:name w:val="heading 2"/><w:basedOn w:val="Rubrik1"/></w:style>` +
	`<w:style w:type="paragraph" w:styleId="MyHeading"><w:name w:val="Custom heading"/><w:pPr><w:outlineLvl w:val="2"/></w:pPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Derived"><w:name w:val="Derived"/><w:basedOn w:val="MyHeading"/></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Normal"><w:name w:val="Normal"/></w:style>`

func TestDOCXParagraphsAndHeadings(t *testing.T) {
	body := wPara("Title", "Customer specification", false) +
		wPara("Rubrik1", "1 Security", false) +
		wPara("", "This chapter describes security.", false) +
		wPara("", "REQ-12: The system SHALL lock an account after five failed logins.", false) +
		wPara("Rubrik2", "1.1 Passwords", false) +
		wPara("", "Passwords MUST have at least 12 characters and MUST NOT be reused. The rules are:", false) +
		wPara("ListParagraph", "one digit", true) +
		wPara("ListParagraph", "one letter", true) +
		wPara("", "Users may choose a password manager.", false) + // lowercase: not normative in an uppercase document
		wPara("MyHeading", "Deep heading", false) +
		wPara("", "[SRS-7] The service SHOULD respond\nwithin one second.", false) +
		wPara("Heading1", "2 Övrigt", false) +
		wPara("Derived", "Under", false) +
		wPara("", "Systemet SKA logga alla inloggningar.", false) +
		`<w:p><w:pPr><w:outlineLvl w:val="0"/></w:pPr><w:r><w:t>3 Direct outline</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>Text in a </w:t></w:r><w:r><w:tab/><w:t>tab MAY</w:t></w:r><w:r><w:delText>deleted SHALL</w:delText></w:r>` +
		`<mc:AlternateContent><mc:Choice><w:r><w:t> appear</w:t></w:r></mc:Choice><mc:Fallback><w:r><w:t> twice</w:t></w:r></mc:Fallback></mc:AlternateContent></w:p>`
	res := mustParse(t, docxOf(t, body, testStyles), "spec.docx", Options{})
	if res.Format != FormatDOCX {
		t.Fatalf("format %q", res.Format)
	}
	got := map[string]Candidate{}
	for _, c := range res.Candidates {
		got[c.Title] = c
	}
	if len(res.Candidates) != 5 {
		for _, c := range res.Candidates {
			t.Logf("%q folder=%q obl=%q", c.Title, c.Folder, c.Obligation)
		}
		t.Fatalf("want 5 candidates, got %d", len(res.Candidates))
	}
	a := res.Candidates[0]
	if a.ExternalID != "REQ-12" || a.Obligation != "shall" || a.Folder != "1 Security" || a.Title != "The system SHALL lock an account after five…" ||
		a.Location != "paragraph 4" || len(a.Warnings) != 0 {
		t.Errorf("first: %+v", a)
	}
	b := res.Candidates[1]
	if b.Folder != "1 Security/1.1 Passwords" || b.Obligation != "must_not" || !strings.HasSuffix(b.Description, "are:\n- one digit\n- one letter") {
		t.Errorf("list: %+v", b)
	}
	c := res.Candidates[2]
	if c.ExternalID != "SRS-7" || c.Folder != "1 Security/1.1 Passwords/Deep heading" || c.Description != "The service SHOULD respond\nwithin one second." {
		t.Errorf("third: %+v", c)
	}
	d := res.Candidates[3]
	if d.Folder != "2 Övrigt/Under" {
		t.Errorf("swedish: %+v", d)
	}
	if d.Obligation != "shall" {
		t.Errorf("swedish obligation: %+v", d)
	}
	e := res.Candidates[4]
	if e.Folder != "3 Direct outline" || e.Description != "Text in a tab MAY appear" || e.Obligation != "may" {
		t.Errorf("runs: %+v", e)
	}
}

func TestDOCXLowercaseDocument(t *testing.T) {
	// No uppercase keywords anywhere: lowercase ones count.
	body := wPara("Heading1", "Intro", false) + wPara("", "The system shall start.", false) + wPara("", "Nothing here.", false)
	res := mustParse(t, docxOf(t, body, ""), "spec.docx", Options{})
	if len(res.Candidates) != 1 || res.Candidates[0].Folder != "Intro" {
		t.Errorf("lowercase: %+v", res.Candidates)
	}
}

func TestDOCXTables(t *testing.T) {
	body := wPara("Heading1", "Krav", false) +
		wTable([][]string{
			{"ID", "Krav", "Prioritet", "Avsnitt"},
			{"K1", "Systemet SKA starta.", "restart:Hög", "Start"},
			{"K2", "Systemet BÖR stängas.", "<<", "Stopp"},
			{"span2:Merged across", "Låg", ""},
		}) +
		wTable([][]string{
			{"Free text", "Another"},
			{"The unit MUST be red.", "Nothing"},
		}) +
		wTable([][]string{
			{"Title", "Description", "Owner"},
			{"Nested", "<<", "Anna"},
		})
	res := mustParse(t, docxOf(t, body, ""), "spec.docx", Options{})
	if len(res.Candidates) != 4 || len(res.Errors) != 1 || !strings.Contains(res.Errors[0], "table 1, row 4") {
		t.Fatalf("want 4 candidates and an error for the merged row, got %+v (errors %v)", res.Candidates, res.Errors)
	}
	a, b := res.Candidates[0], res.Candidates[1]
	if a.ExternalID != "K1" || a.Folder != "Krav/Start" || a.Priority != "high" || a.Location != "table 1, row 2" {
		t.Errorf("row 1: %+v", a)
	}
	if b.Priority != "high" || b.Obligation != "should" || b.Folder != "Krav/Stopp" {
		t.Errorf("vertical merge: %+v", b)
	}
	free := res.Candidates[2]
	if free.Description != "The unit MUST be red." || free.Location != "table 2, row 2" || free.Folder != "Krav" {
		t.Errorf("free table: %+v", free)
	}
	if n := res.Candidates[3]; n.Title != "Nested" || n.Owner != "Anna" {
		t.Errorf("third table: %+v", n)
	}
	if fieldOf(res, "Prioritet") != FieldPriority || fieldOf(res, "Owner") != FieldOwner {
		t.Errorf("columns: %+v", res.Columns)
	}
}

func TestDOCXNestedAndBroken(t *testing.T) {
	nested := "<w:tbl><w:tr><w:tc>" + wPara("", "Outer cell", false) +
		wTable([][]string{{"Inner MUST work"}}) + "</w:tc></w:tr></w:tbl>"
	res := mustParse(t, docxOf(t, nested, ""), "n.docx", Options{})
	if len(res.Candidates) != 1 || res.Candidates[0].Description != "Inner MUST work" {
		t.Errorf("nested: %+v", res.Candidates)
	}
	if _, err := Parse([]byte("plain"), "x.docx", Options{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("not zip: %v", err)
	}
	if _, err := Parse(zipOf(t, map[string]string{"a": "b"}), "x.docx", Options{}); err == nil || !strings.Contains(err.Error(), "document.xml") {
		t.Errorf("no document: %v", err)
	}
	broken := zipOf(t, map[string]string{"word/document.xml": "<w:document><w:body><w:p>"})
	if _, err := Parse(broken, "x.docx", Options{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("broken xml: %v", err)
	}
	empty := docxOf(t, wPara("", "Nothing normative.", false), "")
	res = mustParse(t, empty, "e.docx", Options{})
	if len(res.Candidates) != 0 || len(res.Warnings) != 1 {
		t.Errorf("empty: %+v", res)
	}
}
