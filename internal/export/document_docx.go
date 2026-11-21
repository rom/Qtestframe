package export

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Documents are portrait A4 with 2 cm margins (twips).
const (
	docDocxPageW   = 11906
	docDocxPageH   = 16838
	docDocxMargin  = 1134
	docDocxContent = docDocxPageW - 2*docDocxMargin
	docDocxLabelW  = 2400
)

const docDocxContentTypes = xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
	`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
	`<Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/>` +
	`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>` +
	`</Types>`

const docDocxDocRels = xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
	`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/>` +
	`</Relationships>`

// docDocxExtraStyles are the styles documents add to the report styles:
// further heading levels, the item heading (not in the table of contents),
// the contents entries and list paragraphs.
var docDocxExtraStyles = func() string {
	var b strings.Builder
	sizes := []int{32, 26, 23, 21, 20, 20}
	for l := 2; l <= 6; l++ {
		color := "2F5496"
		if l > 2 {
			color = "1F3864"
		}
		fmt.Fprintf(&b, `<w:style w:type="paragraph" w:styleId="Heading%d"><w:name w:val="heading %d"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/>`+
			`<w:pPr><w:keepNext/><w:keepLines/><w:spacing w:before="%d" w:after="100"/><w:outlineLvl w:val="%d"/></w:pPr><w:rPr><w:b/><w:color w:val="%s"/><w:sz w:val="%d"/><w:szCs w:val="%d"/></w:rPr></w:style>`,
			l, l, 300-30*l, l-1, color, sizes[l-1], sizes[l-1])
	}
	b.WriteString(`<w:style w:type="paragraph" w:styleId="ItemHeading"><w:name w:val="Item Heading"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/>` +
		`<w:pPr><w:keepNext/><w:keepLines/><w:pBdr><w:bottom w:val="single" w:sz="4" w:space="1" w:color="A6A6A6"/></w:pBdr><w:spacing w:before="240" w:after="80"/></w:pPr><w:rPr><w:b/><w:color w:val="1F3864"/><w:sz w:val="22"/><w:szCs w:val="22"/></w:rPr></w:style>`)
	b.WriteString(`<w:style w:type="paragraph" w:styleId="TOCHeading"><w:name w:val="TOC Heading"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/>` +
		`<w:pPr><w:spacing w:before="0" w:after="200"/></w:pPr><w:rPr><w:b/><w:color w:val="2F5496"/><w:sz w:val="32"/><w:szCs w:val="32"/></w:rPr></w:style>`)
	for l := 1; l <= 6; l++ {
		fmt.Fprintf(&b, `<w:style w:type="paragraph" w:styleId="TOC%d"><w:name w:val="toc %d"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:uiPriority w:val="39"/>`+
			`<w:pPr><w:tabs><w:tab w:val="right" w:leader="dot" w:pos="%d"/></w:tabs><w:spacing w:after="60"/><w:ind w:left="%d"/></w:pPr></w:style>`, l, l, docDocxContent, (l-1)*240)
	}
	b.WriteString(`<w:style w:type="paragraph" w:styleId="ListBullet"><w:name w:val="List Bullet"/><w:basedOn w:val="Normal"/>` +
		`<w:pPr><w:spacing w:after="60"/><w:ind w:left="360" w:hanging="240"/></w:pPr></w:style>`)
	b.WriteString(`<w:style w:type="paragraph" w:styleId="Footer"><w:name w:val="footer"/><w:basedOn w:val="Normal"/><w:pPr><w:jc w:val="center"/><w:spacing w:after="0"/></w:pPr><w:rPr><w:color w:val="6E6E6E"/><w:sz w:val="16"/></w:rPr></w:style>`)
	b.WriteString(`<w:style w:type="character" w:styleId="Hyperlink"><w:name w:val="Hyperlink"/><w:rPr><w:color w:val="0563C1"/></w:rPr></w:style>`)
	return b.String()
}()

func docDocxStyles(lang string) string {
	s := strings.Replace(docxStyles, "{{LANG}}", lang, 1)
	return strings.Replace(s, "</w:styles>", docDocxExtraStyles+"</w:styles>", 1)
}

func docDocxFooter(ds Dataset) string {
	word, _, _ := strings.Cut(ds.label("page"), " ")
	return xmlHeader + `<w:ftr xmlns:w="` + nsW + `" xmlns:r="` + nsR + `"><w:p><w:pPr><w:pStyle w:val="Footer"/></w:pPr>` +
		`<w:r><w:t xml:space="preserve">` + esc(word) + ` </w:t></w:r>` +
		`<w:fldSimple w:instr=" PAGE "><w:r><w:t>1</w:t></w:r></w:fldSimple><w:r><w:t xml:space="preserve"> / </w:t></w:r>` +
		`<w:fldSimple w:instr=" NUMPAGES "><w:r><w:t>1</w:t></w:r></w:fldSimple></w:p></w:ftr>`
}

// docxSpans writes paragraph spans as runs.
func docxSpans(b *strings.Builder, spans []Span) {
	for _, sp := range spans {
		if sp.Text == "" {
			continue
		}
		if !sp.Bold && !sp.Italic {
			docxRuns(b, sp.Text, false)
			continue
		}
		var rb strings.Builder
		docxRuns(&rb, sp.Text, false)
		pr := "<w:rPr>"
		if sp.Bold {
			pr += "<w:b/><w:bCs/>"
		}
		if sp.Italic {
			pr += "<w:i/><w:iCs/>"
		}
		b.WriteString(strings.Replace(rb.String(), "<w:r>", "<w:r>"+pr+"</w:rPr>", 1))
	}
}

// docxGridTable writes a table with the given column widths (twips).
func docxGridTable(b *strings.Builder, columns []string, rows [][]string, colW []int, header, label bool) {
	n := len(colW)
	b.WriteString(`<w:tbl><w:tblPr><w:tblStyle w:val="QTable"/><w:tblW w:w="5000" w:type="pct"/><w:tblLayout w:type="fixed"/><w:tblLook w:val="04A0" w:firstRow="1" w:lastRow="0" w:firstColumn="0" w:lastColumn="0" w:noHBand="1" w:noVBand="1"/></w:tblPr><w:tblGrid>`)
	for _, w := range colW {
		fmt.Fprintf(b, `<w:gridCol w:w="%d"/>`, w)
	}
	b.WriteString("</w:tblGrid>")
	row := func(cells []string, head bool) {
		b.WriteString("<w:tr>")
		if head {
			b.WriteString(`<w:trPr><w:tblHeader/><w:cantSplit/></w:trPr>`)
		}
		for i := 0; i < n; i++ {
			fmt.Fprintf(b, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/>`, colW[i])
			switch {
			case head:
				b.WriteString(`<w:shd w:val="clear" w:color="auto" w:fill="D9E2F3"/>`)
			case label && i == 0:
				b.WriteString(`<w:shd w:val="clear" w:color="auto" w:fill="F2F2F2"/>`)
			}
			b.WriteString("</w:tcPr><w:p>")
			docxRuns(b, cell(cells, i), head || (label && i == 0))
			b.WriteString("</w:p></w:tc>")
		}
		b.WriteString("</w:tr>")
	}
	if header {
		row(columns, true)
	}
	for _, r := range rows {
		row(r, false)
	}
	b.WriteString("</w:tbl><w:p/>") // a table must not be followed directly by another or end the body
}

// docxWidths turns relative widths into twips (equal widths when none or
// the wrong number are given).
func docxWidths(rel []float64, n int) []int {
	out := make([]int, n)
	total := 0.0
	for _, w := range rel {
		total += w
	}
	for i := range out {
		if len(rel) == n && total > 0 {
			out[i] = int(float64(docDocxContent) * rel[i] / total)
		} else {
			out[i] = docDocxContent / n
		}
	}
	return out
}

func docxPageBreak(b *strings.Builder) { b.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`) }

func docxTOC(b *strings.Builder, ds Dataset, entries []tocEntry, levels int) {
	docxPara(b, "TOCHeading", ds.label("contents"))
	begin := `<w:r><w:fldChar w:fldCharType="begin" w:dirty="true"/></w:r><w:r><w:instrText xml:space="preserve"> TOC \o "1-` +
		strconv.Itoa(levels) + `" \h \z \u </w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r>`
	end := `<w:r><w:fldChar w:fldCharType="end"/></w:r>`
	if len(entries) == 0 {
		b.WriteString("<w:p>" + begin + end + "</w:p>")
		return
	}
	for i, e := range entries {
		fmt.Fprintf(b, `<w:p><w:pPr><w:pStyle w:val="TOC%d"/></w:pPr>`, e.level)
		if i == 0 {
			b.WriteString(begin)
		}
		fmt.Fprintf(b, `<w:hyperlink w:anchor="_Toc%d" w:history="1">`, e.block)
		docxRuns(b, e.text, false)
		b.WriteString("</w:hyperlink>")
		if i == len(entries)-1 {
			b.WriteString(end)
		}
		b.WriteString("</w:p>")
	}
}

func writeDocDOCX(w io.Writer, doc *Document) error {
	ds := doc.dataset()
	zw := zip.NewWriter(w)
	mod := generatedAt(ds)
	part := func(name string) (io.Writer, error) {
		return zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: mod})
	}
	for _, p := range []struct{ name, body string }{
		{"[Content_Types].xml", docDocxContentTypes},
		{"_rels/.rels", docxRootRels},
		{"word/_rels/document.xml.rels", docDocxDocRels},
		{"word/styles.xml", docDocxStyles(ds.label("docLang"))},
		{"word/footer1.xml", docDocxFooter(ds)},
		{"docProps/core.xml", docDocxCore(doc)},
	} {
		f, err := part(p.name)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(f, p.body); err != nil {
			return err
		}
	}
	f, err := part("word/document.xml")
	if err != nil {
		return err
	}
	bw := bufio.NewWriterSize(f, 64<<10)
	var b strings.Builder
	flush := func() error {
		_, err := bw.WriteString(b.String())
		b.Reset()
		return err
	}
	b.WriteString(xmlHeader)
	b.WriteString(`<w:document xmlns:w="` + nsW + `" xmlns:r="` + nsR + `"><w:body>`)
	// title page
	b.WriteString(`<w:p><w:pPr><w:spacing w:before="3600"/></w:pPr></w:p>`)
	docxPara(&b, "Title", doc.Title)
	if doc.Subtitle != "" {
		docxPara(&b, "Subtitle", doc.Subtitle)
	}
	b.WriteString(`<w:p><w:pPr><w:spacing w:before="480"/></w:pPr></w:p>`)
	tf := append([]Item{}, doc.TitleFields...)
	tf = append(tf, Item{Label: ds.label("generated"), Value: generatedAt(ds).Format("2006-01-02 15:04 MST")})
	docDocxFields(&b, tf)
	docxPageBreak(&b)

	entries, _ := doc.toc()
	inTOC := map[int]bool{}
	for _, e := range entries {
		inTOC[e.block] = true
	}
	for i, blk := range doc.Blocks {
		switch blk.Kind {
		case BlockHeading:
			l := clampLevel(blk.Level)
			style := "Heading" + strconv.Itoa(l)
			if blk.NoTOC {
				style = "ItemHeading"
			}
			b.WriteString(`<w:p><w:pPr><w:pStyle w:val="` + style + `"/></w:pPr>`)
			if inTOC[i] {
				fmt.Fprintf(&b, `<w:bookmarkStart w:id="%d" w:name="_Toc%d"/>`, i, i)
			}
			docxRuns(&b, oneLine(blk.Text), false)
			if inTOC[i] {
				fmt.Fprintf(&b, `<w:bookmarkEnd w:id="%d"/>`, i)
			}
			b.WriteString("</w:p>")
		case BlockParagraph:
			if strings.TrimSpace(blk.plain()) == "" {
				continue
			}
			for _, para := range splitParagraphs(blk.spans()) {
				b.WriteString("<w:p>")
				docxSpans(&b, para)
				b.WriteString("</w:p>")
			}
		case BlockTable:
			if len(blk.Rows) == 0 {
				docxSpansPara(&b, []Span{{Text: ds.label("noRows"), Italic: true}})
				continue
			}
			n := width(Section{Columns: blk.Columns, Rows: blk.Rows})
			docxGridTable(&b, blk.Columns, blk.Rows, docxWidths(blk.Widths, n), true, false)
		case BlockFields:
			docDocxFields(&b, blk.fieldItems())
		case BlockList:
			for _, it := range blk.Items {
				if strings.TrimSpace(it) != "" {
					docxPara(&b, "ListBullet", "•\t"+it)
				}
			}
		case BlockPageBreak:
			docxPageBreak(&b)
		case BlockTOC:
			if len(entries) > 0 {
				if i > 0 {
					docxPageBreak(&b)
				}
				docxTOC(&b, ds, entries, doc.tocLevels())
				docxPageBreak(&b)
			}
		}
		if b.Len() > 32<<10 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	fmt.Fprintf(&b, `<w:sectPr><w:footerReference w:type="default" r:id="rId2"/><w:pgSz w:w="%d" w:h="%d"/><w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="567" w:footer="567" w:gutter="0"/><w:titlePg/></w:sectPr>`,
		docDocxPageW, docDocxPageH, docDocxMargin, docDocxMargin, docDocxMargin, docDocxMargin)
	b.WriteString("</w:body></w:document>")
	if err := flush(); err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	return zw.Close()
}

func docxSpansPara(b *strings.Builder, spans []Span) {
	b.WriteString("<w:p>")
	docxSpans(b, spans)
	b.WriteString("</w:p>")
}

func docDocxFields(b *strings.Builder, items []Item) {
	if len(items) == 0 {
		return
	}
	rows := make([][]string, len(items))
	for i, it := range items {
		rows[i] = []string{it.Label, it.Value}
	}
	docxGridTable(b, nil, rows, []int{docDocxLabelW, docDocxContent - docDocxLabelW}, false, true)
}

// splitParagraphs splits spans at blank lines into paragraphs.
func splitParagraphs(spans []Span) [][]Span {
	var out [][]Span
	var cur []Span
	for _, sp := range spans {
		parts := strings.Split(strings.ReplaceAll(sp.Text, "\r\n", "\n"), "\n\n")
		for i, part := range parts {
			if i > 0 {
				if len(cur) > 0 {
					out = append(out, cur)
				}
				cur = nil
			}
			part = strings.Trim(part, "\n")
			if part != "" {
				s := sp
				s.Text = part
				cur = append(cur, s)
			}
		}
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

func docDocxCore(doc *Document) string {
	ds := doc.dataset()
	s := docxCore(ds)
	if doc.Author != "" {
		s = strings.Replace(s, "<dc:creator>Qtestframe</dc:creator>", "<dc:creator>"+esc(doc.Author)+"</dc:creator>", 1)
	}
	return s
}
