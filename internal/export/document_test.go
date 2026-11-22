package export

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func sampleDocument(headings int) *Document {
	doc := &Document{
		Title: "Spec", Subtitle: "P – Shop", Header: "Spec · 1.0", Author: "Ann", Lang: "sv",
		GeneratedAt: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
		TitleFields: []Item{{Label: "Version", Value: "1.0"}, {Label: "Empty", Value: " "}},
		Link: func(v string) string {
			if v == "P-REQ-1" {
				return "https://x.example/go/P-REQ-1"
			}
			return ""
		},
	}
	doc.Blocks = append(doc.Blocks, Heading(1, "Document control"), Fields(Item{Label: "Author", Value: "Ann"}, Item{Label: "None", Value: ""}), TOC())
	note := Note("nothing here")
	item := Heading(3, "P-REQ-1 Pay")
	item.NoTOC = true
	doc.Blocks = append(doc.Blocks,
		Heading(1, "1 Introduction"), Paragraph("First paragraph.\n\nSecond, with a\nline break."), note,
		Heading(2, "1.1 Scope"), RichParagraph(Span{Text: "The system "}, Span{Text: "MUST", Bold: true}, Span{Text: " work; "}, Span{Text: "note", Italic: true}, Span{Text: " both", Bold: true, Italic: true}, Span{Text: ""}),
		List("one", "", "two – åäö"),
		RichParagraph(Span{Text: "Caption", Bold: true}),
		Table([]string{"Key", "Title"}, [][]string{{"P-REQ-1", "Pay | card"}, {"P-REQ-2", strings.Repeat("long text ", 40)}}, 1, 3),
		Table([]string{"A", "B", "C"}, [][]string{{"1", "2", "3"}}),
		Table([]string{"Empty"}, nil),
		item, Paragraph(""),
		PageBreak(), PageBreak(),
		Heading(9, "Deep"),
	)
	for i := 0; i < headings; i++ {
		doc.Blocks = append(doc.Blocks, Heading(2, fmt.Sprintf("2.%d Section with a rather long title that will need to be shortened in the table of contents of the PDF document %d", i, i)), Paragraph("Text."))
	}
	return doc
}

func TestWriteDocumentMarkdown(t *testing.T) {
	var b bytes.Buffer
	if err := WriteDocument(&b, MD, sampleDocument(100)); err != nil {
		t.Fatal(err)
	}
	md := b.String()
	for _, want := range []string{
		"# Spec\n\nP – Shop\n\n- **Version:** 1\\.0\n- **Skapad:** 2026-03-01",
		"## Document control\n\n- **Author:** Ann\n\n## Innehåll\n\n- [1 Introduction](#1-introduction)\n  - [1\\.1 Scope](#11-scope)\n",
		"First paragraph.\n\nSecond, with a\\\nline break.", "*nothing here*",
		"The system **MUST** work; *note* ***both***", "- one\n- two – åäö", "**Caption**",
		"| [P-REQ-1](https://x.example/go/P-REQ-1) | Pay \\| card |", "*(inga rader)*", "#### P-REQ-1 Pay", "###### Deep",
		"[2\\.99 Section with", "](#299-section-with-a-rather-long-title",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in\n%s", want, md)
		}
	}
	if strings.Contains(md, "[P-REQ-1 Pay]") || strings.Contains(md, "[Document control]") {
		t.Error("item headings and headings before the contents are not listed")
	}
	if !strings.HasSuffix(md, "Text.\n") || strings.HasSuffix(md, "\n\n") {
		t.Errorf("ends with one newline: %q", md[len(md)-20:])
	}
	// no TOC block: no contents
	doc := sampleDocument(0)
	doc.Blocks = doc.Blocks[3:]
	b.Reset()
	_ = WriteDocument(&b, MD, doc)
	if strings.Contains(b.String(), "Innehåll") {
		t.Error("contents without a TOC block")
	}
}

func TestWriteDocumentDOCX(t *testing.T) {
	var b bytes.Buffer
	if err := WriteDocument(&b, DOCX, sampleDocument(5)); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		parts[f.Name] = string(data)
	}
	doc := parts["word/document.xml"]
	for _, want := range []string{
		`<w:pStyle w:val="Title"/>`, `TOC \o "1-3" \h \z \u`, `w:fldCharType="begin" w:dirty="true"`, `<w:hyperlink w:anchor="_Toc3"`,
		`<w:bookmarkStart w:id="3" w:name="_Toc3"/>`, `<w:pStyle w:val="ItemHeading"/>`, `<w:pStyle w:val="Heading6"/>`, `<w:br w:type="page"/>`,
		`<w:b/><w:bCs/></w:rPr><w:t xml:space="preserve">MUST</w:t>`, `<w:i/><w:iCs/>`, `<w:pStyle w:val="ListBullet"/>`,
		`<w:shd w:val="clear" w:color="auto" w:fill="F2F2F2"/>`, `<w:tblHeader/>`, "inga rader", `<w:footerReference w:type="default" r:id="rId2"/>`, `<w:titlePg/>`,
		`w:orient`,
	} {
		if want == `w:orient` {
			if strings.Contains(doc, want) {
				t.Error("documents are portrait")
			}
			continue
		}
		if !strings.Contains(doc, want) {
			t.Errorf("document.xml: missing %s", want)
		}
	}
	if !strings.Contains(parts["word/footer1.xml"], "Sida") || !strings.Contains(parts["word/footer1.xml"], "NUMPAGES") {
		t.Error(parts["word/footer1.xml"])
	}
	if !strings.Contains(parts["word/styles.xml"], `w:styleId="TOC2"`) || !strings.Contains(parts["word/styles.xml"], "sv-SE") {
		t.Error("styles")
	}
	if !strings.Contains(parts["docProps/core.xml"], "<dc:creator>Ann</dc:creator>") || !strings.Contains(parts["[Content_Types].xml"], "footer1.xml") {
		t.Error("core / content types")
	}
	// without entries the field is still valid
	d := sampleDocument(0)
	d.Blocks = []Block{TOC(), Heading(1, "Only")}
	d.Blocks[1].NoTOC = true
	b.Reset()
	if err := WriteDocument(&b, DOCX, d); err != nil {
		t.Fatal(err)
	}
}

func TestWriteDocumentPDF(t *testing.T) {
	for _, n := range []int{0, 3, 120} {
		var b bytes.Buffer
		if err := WriteDocument(&b, PDF, sampleDocument(n)); err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(b.Bytes(), []byte("%PDF")) || !bytes.Contains(b.Bytes(), []byte("/Outlines")) {
			t.Errorf("%d headings: not a PDF with bookmarks", n)
		}
	}
	p := &docPDF{pdfWriter: &pdfWriter{pageH: 297, margin: docPDFMargin}}
	first, per := p.tocLines()
	if p.tocPageCount(1) != 1 || p.tocPageCount(first) != 1 || p.tocPageCount(first+1) != 2 || p.tocPageCount(first+per+1) != 3 {
		t.Errorf("toc pages: first %d per %d", first, per)
	}
}

func TestWriteDocumentErrors(t *testing.T) {
	if err := WriteDocument(io.Discard, CSV, &Document{}); err == nil {
		t.Error("CSV is not a document format")
	}
	if got := gfmSlug("1.2 Scope & Purpose_x", map[string]int{}); got != "12-scope--purpose_x" {
		t.Error(got)
	}
	seen := map[string]int{}
	if gfmSlug("A", seen) != "a" || gfmSlug("A", seen) != "a-1" {
		t.Error("duplicate slugs")
	}
	ps := splitParagraphs([]Span{{Text: "a\n\nb"}, {Text: "c", Bold: true}, {Text: "\n\n"}, {Text: "d"}})
	if len(ps) != 3 || ps[1][0].Text != "b" || !ps[1][1].Bold || ps[2][0].Text != "d" {
		t.Errorf("%+v", ps)
	}
	if markdownSpans([]Span{{Text: "# not a heading"}, {Text: "\n1. not a list"}}) != `\# not a heading\`+"\n"+`1\. not a list` {
		t.Error(markdownSpans([]Span{{Text: "# not a heading"}, {Text: "\n1. not a list"}}))
	}
	var m MarkdownWriter
	if err := m.Flush(io.Discard); err != nil {
		t.Error(err)
	}
}
