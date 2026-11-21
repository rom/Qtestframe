package export

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

var ts = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

func sample() Dataset {
	return Dataset{
		Title:       "Requirements – Überblick",
		Subtitle:    "Project <ACME> & \"friends\"",
		GeneratedAt: ts,
		Sections: []Section{
			{Name: "requirements", Title: "Requirements", Columns: []string{"Key", "Title", "Note"},
				Rows: [][]string{
					{"REQ-1", "=HYPERLINK(\"http://evil\")", "multi\nline\ttab"},
					{"REQ-2", "Grüße €", "-1"},
					{"REQ-3", "ctrl\x01char\x0b", "+cmd"},
					{"REQ-4"}, // short row
				}},
			{Name: "tests", Title: "Tests", Columns: []string{"Key", "Status"},
				Rows: [][]string{{"TC-1", "@passed"}, {"TC-2", "failed", "extra"}}},
		},
	}
}

func TestParseFormat(t *testing.T) {
	for in, want := range map[string]Format{"csv": CSV, " XLSX ": XLSX, ".pdf": PDF, "Docx": DOCX, "excel": XLSX, "word": DOCX} {
		got, err := ParseFormat(in)
		if err != nil || got != want {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	if _, err := ParseFormat("html"); err == nil {
		t.Fatal("expected error")
	}
	if CSV.Extension(true) != "zip" || CSV.Extension(false) != "csv" || PDF.Extension(true) != "pdf" {
		t.Fatal("extension")
	}
	if !strings.HasPrefix(CSV.ContentType(), "text/csv") || CSV.ContentTypeFor(true) != "application/zip" ||
		PDF.ContentTypeFor(true) != "application/pdf" || !strings.Contains(XLSX.ContentType(), "spreadsheetml") ||
		!strings.Contains(DOCX.ContentType(), "wordprocessingml") || Format("x").ContentType() != "application/octet-stream" {
		t.Fatal("content types")
	}
	if err := Write(io.Discard, Format("x"), sample()); err == nil {
		t.Fatal("expected error for unknown format")
	}
}

func TestCSVSingle(t *testing.T) {
	ds := sample()
	ds.Sections = ds.Sections[:1]
	var buf bytes.Buffer
	if err := Write(&buf, CSV, ds); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "\r\n") {
		t.Fatal("expected CRLF")
	}
	recs, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 5 || recs[0][0] != "Key" {
		t.Fatalf("%v", recs)
	}
	if recs[1][1] != `'=HYPERLINK("http://evil")` || recs[2][2] != "'-1" || recs[3][2] != "'+cmd" {
		t.Fatalf("injection protection: %q", recs)
	}
	if recs[1][2] != "multi\nline\ttab" || recs[2][1] != "Grüße €" || len(recs[4]) != 3 {
		t.Fatalf("%q", recs)
	}
	for _, s := range []string{"\tx", "\rx", "@x", "=1"} {
		if sanitizeCSVCell(s) != "'"+s {
			t.Fatalf("sanitize %q", s)
		}
	}
	if sanitizeCSVCell("") != "" || sanitizeCSVCell("ok") != "ok" {
		t.Fatal("sanitize plain")
	}
	// zero sections => empty output
	buf.Reset()
	if err := WriteCSV(&buf, Dataset{}); err != nil || buf.Len() != 0 {
		t.Fatal("empty")
	}
}

func TestCSVZip(t *testing.T) {
	ds := sample()
	ds.Sections = append(ds.Sections,
		Section{Name: "tests", Rows: [][]string{{"dup"}}},
		Section{Name: "", Title: "Über Sicht/x", Rows: [][]string{{"1"}}},
		Section{Name: "***"},
	)
	var buf bytes.Buffer
	if err := WriteCSV(&buf, ds); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	contents := map[string]string{}
	for _, f := range zr.File {
		names = append(names, f.Name)
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		contents[f.Name] = string(b)
	}
	want := "requirements.csv,tests.csv,tests-2.csv,ber_Sichtx.csv,section-5.csv"
	if strings.Join(names, ",") != want {
		t.Fatalf("names %v", names)
	}
	if !(contents["tests.csv"] == "Key,Status,\r\nTC-1,'@passed,\r\nTC-2,failed,extra\r\n") {
		t.Fatalf("%q", contents["tests.csv"])
	}
	if contents["tests-2.csv"] != "dup\r\n" {
		t.Fatalf("%q", contents["tests-2.csv"])
	}
}

type failWriter struct{ n int }

func (f *failWriter) Write(p []byte) (int, error) {
	if f.n <= 0 {
		return 0, errors.New("boom")
	}
	f.n--
	return len(p), nil
}

func TestWriteErrors(t *testing.T) {
	ds := sample()
	for _, f := range []Format{CSV, XLSX, PDF, DOCX} {
		if err := Write(&failWriter{}, f, ds); err == nil {
			t.Errorf("%s: expected write error", f)
		}
	}
	one := ds
	one.Sections = ds.Sections[:1]
	if err := WriteCSV(&failWriter{}, one); err == nil {
		t.Error("single csv: expected error")
	}
}

func TestXLSX(t *testing.T) {
	ds := sample()
	long := strings.Repeat("x", 40)
	ds.Sections = append(ds.Sections,
		Section{Name: long, Columns: []string{"A"}, Rows: [][]string{{strings.Repeat("y", 40000)}}},
		Section{Name: long, Columns: []string{"A"}},
		Section{Name: "summary", Columns: []string{"A"}, Rows: [][]string{{"1"}}},
		Section{Name: "a/b:c?[d]*", Columns: []string{"A"}},
		Section{Title: "'"},
	)
	var buf bytes.Buffer
	if err := Write(&buf, XLSX, ds); err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sheets := f.GetSheetList()
	want := []string{"Summary", "requirements", "tests", long[:31], long[:27] + " (2)", "summary (2)", "a_b_c__d__", "Sheet 7"}
	if fmt.Sprint(sheets) != fmt.Sprint(want) {
		t.Fatalf("sheets %q", sheets)
	}
	if v, _ := f.GetCellValue("Summary", "A1"); v != ds.Title {
		t.Fatalf("title %q", v)
	}
	if v, _ := f.GetCellValue("Summary", "B6"); v != "4" {
		t.Fatalf("row count %q", v)
	}
	if v, _ := f.GetCellValue("Summary", "B3"); !strings.HasPrefix(v, "2026-03-04") {
		t.Fatalf("generated %q", v)
	}
	rows, err := f.GetRows("requirements")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(rows[0]) != "[Key Title Note]" || rows[1][1] != `=HYPERLINK("http://evil")` || rows[3][1] != "ctrlchar" {
		t.Fatalf("rows %q", rows)
	}
	if fs, _ := f.GetCellFormula("requirements", "B2"); fs != "" {
		t.Fatal("must not be a formula")
	}
	st, _ := f.GetCellStyle("requirements", "A1")
	style, _ := f.GetStyle(st)
	if style == nil || style.Font == nil || !style.Font.Bold || len(style.Fill.Color) == 0 {
		t.Fatalf("header style %+v", style)
	}
	panes, _ := f.GetPanes("requirements")
	if !panes.Freeze || panes.YSplit != 1 {
		t.Fatalf("panes %+v", panes)
	}
	if v, _ := f.GetCellValue(long[:31], "A2"); len(v) != maxCellChars {
		t.Fatalf("cell truncation %d", len(v))
	}
}

func TestPDF(t *testing.T) {
	ds := sample()
	var rows [][]string
	for i := 0; i < 300; i++ {
		rows = append(rows, []string{fmt.Sprintf("REQ-%d", i), strings.Repeat("Lorem ipsum dolor sit amet ", i%7+1), "Ünïcödé ✓ 日本"})
	}
	ds.Sections = append(ds.Sections,
		Section{Name: "big", Title: "Big", Columns: []string{"Key", "Text", "U"}, Rows: rows},
		Section{Name: "huge", Columns: []string{"A", "B"}, Rows: [][]string{{strings.Repeat("word ", 5000), "x"}, {"after", "y"}}},
		Section{Name: "empty"},
		Section{Name: "norows", Columns: []string{"A"}},
		Section{Name: "wide", Columns: make([]string, 30), Rows: [][]string{make([]string, 30)}},
	)
	var buf bytes.Buffer
	if err := Write(&buf, PDF, ds); err != nil {
		t.Fatal(err)
	}
	out := buf.Bytes()
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("not a PDF")
	}
	if n := bytes.Count(out, []byte("/Type /Page\n")); n < 5 {
		t.Fatalf("expected many pages, got %d", n)
	}
	// minimal dataset without subtitle
	buf.Reset()
	if err := WritePDF(&buf, Dataset{Title: "x"}); err != nil || !bytes.HasPrefix(buf.Bytes(), []byte("%PDF")) {
		t.Fatal(err)
	}
}

func TestDOCX(t *testing.T) {
	ds := sample()
	ds.Sections = append(ds.Sections, Section{Name: "empty"}, Section{Columns: []string{"Only header"}})
	var buf bytes.Buffer
	if err := Write(&buf, DOCX, ds); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		parts[f.Name] = string(b)
		// every part must be well-formed XML
		dec := xml.NewDecoder(bytes.NewReader(b))
		for {
			_, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s: %v", f.Name, err)
			}
		}
	}
	for _, p := range []string{"[Content_Types].xml", "_rels/.rels", "word/_rels/document.xml.rels", "word/document.xml", "word/styles.xml", "docProps/core.xml"} {
		if _, ok := parts[p]; !ok {
			t.Fatalf("missing part %s", p)
		}
	}
	doc := parts["word/document.xml"]
	for _, s := range []string{"Requirements – Überblick", "Project &lt;ACME&gt; &amp; &#34;friends&#34;", "REQ-1",
		`<w:pStyle w:val="Heading1"/>`, `<w:pStyle w:val="Title"/>`, "<w:tblHeader/>", `w:fill="D9E2F3"`, "<w:b/>",
		"Grüße €", "<w:br/>", "<w:tab/>", "(no data)", "(no rows)", "ctrlchar", "Section 4"} {
		if !strings.Contains(doc, s) {
			t.Errorf("document.xml missing %q", s)
		}
	}
	if strings.ContainsRune(doc, '\x01') {
		t.Fatal("control char not stripped")
	}
	if !strings.Contains(parts["docProps/core.xml"], "2026-03-04T05:06:07Z") {
		t.Fatal("core date")
	}
	if !strings.Contains(parts["word/styles.xml"], `w:styleId="QTable"`) {
		t.Fatal("table style")
	}
	// extract plain text through the XML decoder
	var text strings.Builder
	dec := xml.NewDecoder(strings.NewReader(doc))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if cd, ok := tok.(xml.CharData); ok {
			text.Write(cd)
		}
	}
	if !strings.Contains(text.String(), `Project <ACME> & "friends"`) {
		t.Fatalf("text %q", text.String())
	}
}

func TestHelpers(t *testing.T) {
	if got := cleanXML("a\x00b\xffc"); got != "ab�c" {
		t.Fatalf("%q", got)
	}
	if cleanXML("ok\t\n") != "ok\t\n" || !validXMLRune(0x10000) || validXMLRune(0xFFFF) {
		t.Fatal("validXMLRune")
	}
	if truncateRunes("äöü", 2) != "äö" || truncateRunes("ab", 5) != "ab" {
		t.Fatal("truncateRunes")
	}
	if (Dataset{}).sectionTitle(Section{Name: "n"}, 0) != "n" || (Dataset{}).sectionTitle(Section{}, 2) != "Section 3" ||
		(Dataset{Lang: "sv"}).sectionTitle(Section{}, 0) != "Avsnitt 1" || (Dataset{Lang: "xx"}).sectionTitle(Section{}, 0) != "Section 1" {
		t.Fatal("sectionTitle")
	}
	if generatedAt(Dataset{}).IsZero() {
		t.Fatal("generatedAt default")
	}
}
