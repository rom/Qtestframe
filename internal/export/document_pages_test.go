package export

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"
)

// filled is a document whose first content page is nearly full.
func filled(lines int) *Document {
	doc := &Document{Title: "T", Header: "T · 1", Lang: "en"}
	doc.Blocks = append(doc.Blocks, Heading(1, "1 Chapter"))
	for i := range lines {
		doc.Blocks = append(doc.Blocks, Paragraph(fmt.Sprintf("Filler %d", i)))
	}
	return doc
}

func item(key string, keep, newPage bool, rows int) []Block {
	h := Heading(3, key+" Item")
	h.NoTOC, h.KeepTogether, h.NewPage = true, keep, newPage
	var fs []Item
	for i := range rows {
		fs = append(fs, Item{Label: fmt.Sprintf("%s field %d", key, i), Value: "value"})
	}
	return []Block{h, Paragraph(key + " description."), Fields(fs...)}
}

func pdfOf(t *testing.T, doc *Document) [][]pdfText {
	t.Helper()
	var b bytes.Buffer
	if err := WriteDocument(&b, PDF, doc); err != nil {
		t.Fatal(err)
	}
	return pdfPages(t, b.Bytes())
}

// TestPDFKeepTogether: an item that does not fit on the rest of a page
// starts on the next one (with the folder heading and description right
// before it); without KeepTogether it is split; an item longer than a page
// flows.
func TestPDFKeepTogether(t *testing.T) {
	const lines = 29
	split := filled(lines)
	split.Blocks = append(split.Blocks, item("A", false, false, 8)...)
	pages := pdfOf(t, split)
	if a, last := pageOf(pages, "A Item"), pageOf(pages, "A field 7"); a != 1 || last != 2 {
		t.Fatalf("without keep-together the item is split: heading on page %d, last field on %d", a+1, last+1)
	}

	kept := filled(lines)
	folder := Heading(2, "1.1 Folder")
	kept.Blocks = append(kept.Blocks, folder, Paragraph("Folder description."))
	kept.Blocks = append(kept.Blocks, item("A", true, false, 8)...)
	kept.Blocks = append(kept.Blocks, item("B", true, false, 2)...)
	pages = pdfOf(t, kept)
	if f := pageOf(pages, fmt.Sprintf("Filler %d", lines-1)); f != 1 {
		t.Fatal("filler on page", f+1)
	}
	for _, s := range []string{"1.1 Folder", "Folder description.", "A Item", "A field 7", "B Item", "B field 1"} {
		if p := pageOf(pages, s); p != 2 {
			t.Errorf("%q on page %d, want 3", s, p+1)
		}
	}
	// the folder heading is the first text of its page (after the header)
	if top := pages[2][1].Text; top != "1.1 Folder" {
		t.Errorf("page 3 starts with %q", top)
	}

	long := filled(10)
	long.Blocks = append(long.Blocks, item("L", true, false, 90)...)
	pages = pdfOf(t, long)
	if h, last := pageOf(pages, "L Item"), pageOf(pages, "L field 89"); h != 1 || last <= h {
		t.Fatalf("a long item flows from where it is: heading on page %d, last field on %d", h+1, last+1)
	}
}

// TestPDFNewPage: a NewPage heading starts a new page, but not an empty
// one, and a run of headings before an item moves with it.
func TestPDFNewPage(t *testing.T) {
	doc := filled(2)
	doc.Blocks = append(doc.Blocks, item("A", true, true, 2)...) // after text: new page
	doc.Blocks = append(doc.Blocks, item("B", true, true, 2)...)
	g := Heading(2, "1.1 Group")
	g.NewPage = true
	doc.Blocks = append(doc.Blocks, g)
	doc.Blocks = append(doc.Blocks, item("C", true, false, 2)...) // with its group heading
	doc.Blocks = append(doc.Blocks, PageBreak())
	doc.Blocks = append(doc.Blocks, item("D", true, true, 2)...) // the page is still empty: no blank page
	pages := pdfOf(t, doc)
	want := map[string]int{"Filler 1": 1, "A Item": 2, "B Item": 3, "1.1 Group": 4, "C Item": 4, "D Item": 5}
	for s, p := range want {
		if got := pageOf(pages, s); got != p {
			t.Errorf("%q on page %d, want %d", s, got+1, p+1)
		}
	}
	if len(pages) != 6 {
		t.Errorf("%d pages, want 6 (no blank page)", len(pages))
	}
	for _, s := range []string{"A Item", "B Item", "1.1 Group", "D Item"} {
		if top := pages[want[s]][1].Text; top != s { // [0] is the running header
			t.Errorf("page %d starts with %q, want %q", want[s]+1, top, s)
		}
	}
}

// docxBody returns word/document.xml.
func docxBody(t *testing.T, doc *Document) string {
	t.Helper()
	var b bytes.Buffer
	if err := WriteDocument(&b, DOCX, doc); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			data, _ := io.ReadAll(rc)
			rc.Close()
			return string(data)
		}
	}
	t.Fatal("no document.xml")
	return ""
}

// docxBlocks splits the body into its top-level paragraphs and tables.
var docxBlockRe = regexp.MustCompile(`(?s)<w:tbl>.*?</w:tbl>|<w:p>.*?</w:p>|<w:p/>`)

func TestDOCXNewPageAndKeepTogether(t *testing.T) {
	doc := filled(2)
	doc.Blocks = append(doc.Blocks, item("A", true, true, 3)...)
	doc.Blocks = append(doc.Blocks, item("B", true, false, 3)...)
	doc.Blocks = append(doc.Blocks, Heading(2, "1.2 After"), Paragraph("Free text."))
	x := docxBody(t, doc)
	if n := strings.Count(x, "<w:pageBreakBefore/>"); n != 1 {
		t.Fatalf("%d page breaks before", n)
	}
	blocks := docxBlockRe.FindAllString(x, -1)
	find := func(s string) int {
		for i, b := range blocks {
			if strings.Contains(b, ">"+s+"<") {
				return i
			}
		}
		t.Fatalf("%q not found", s)
		return -1
	}
	if a := blocks[find("A Item")]; !strings.Contains(a, "<w:pageBreakBefore/>") || !strings.Contains(a, "ItemHeading") {
		t.Fatal(a)
	}
	// B's description and all its field rows but the last are kept with the next
	if d := blocks[find("B description.")]; !strings.Contains(d, "<w:keepNext/>") {
		t.Fatal(d)
	}
	tbl := blocks[find("B description.")+1]
	rows := strings.Split(tbl, "<w:tr>")[1:]
	if len(rows) != 3 {
		t.Fatal(len(rows), tbl)
	}
	for i, r := range rows {
		if strings.Contains(r, "<w:keepNext/>") != (i < 2) || !strings.Contains(r, "<w:cantSplit/>") {
			t.Errorf("row %d: %s", i, r)
		}
	}
	// text outside items is not kept
	if f := blocks[find("Free text.")]; strings.Contains(f, "keepNext") {
		t.Fatal(f)
	}
	if f := blocks[find("Filler 1")]; strings.Contains(f, "keepNext") {
		t.Fatal(f)
	}
}

func TestMarkdownIgnoresPageLayout(t *testing.T) {
	doc := filled(1)
	doc.Blocks = append(doc.Blocks, item("A", true, true, 1)...)
	var b bytes.Buffer
	if err := WriteDocument(&b, MD, doc); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "Filler 0\n\n#### A Item\n\nA description.") {
		t.Fatal(b.String())
	}
}
