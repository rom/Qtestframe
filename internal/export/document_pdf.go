package export

import (
	"fmt"
	"io"
	"strings"

	"github.com/go-pdf/fpdf"
)

// Page layout of documents: portrait A4.
const (
	docPDFMargin = 18.0
	docBodySize  = 10.0
	docBodyLineH = 5.0
	docTOCLineH  = 6.0
	docLabelW    = 42.0 // label column of key/value tables
)

// headings: font size per level
var docHeadingSize = []float64{16, 13, 11.5, 10.5, 10, 10}

// docPDF renders a Document; it reuses the table rows of the report writer.
type docPDF struct {
	*pdfWriter
	doc *Document
	ds  Dataset
	// bookmark level of the previous heading (outline levels may only
	// grow one at a time)
	lastBookmark int
	// scratch lays out blocks off the page to measure them (keepTogether)
	scratch  *docPDF
	measured int
}

// newDocPDF returns a writer of doc on a new PDF (no pages yet).
func newDocPDF(doc *Document) *docPDF {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(docPDFMargin, docPDFMargin, docPDFMargin)
	pdf.SetAutoPageBreak(false, docPDFMargin)
	pageW, pageH := pdf.GetPageSize()
	return &docPDF{
		pdfWriter:    &pdfWriter{pdf: pdf, tr: pdf.UnicodeTranslatorFromDescriptor(""), pageH: pageH, usable: pageW - 2*docPDFMargin, margin: docPDFMargin},
		doc:          doc,
		ds:           doc.dataset(),
		lastBookmark: -1,
	}
}

func writeDocPDF(w io.Writer, doc *Document) error {
	p := newDocPDF(doc)
	pdf := p.pdf
	pdf.AliasNbPages("{nb}")
	pageW, _ := pdf.GetPageSize()
	pdf.SetTitle(doc.Title, true)
	pdf.SetSubject(doc.Subtitle, true)
	if doc.Author != "" {
		pdf.SetAuthor(doc.Author, true)
	}
	pdf.SetCreator("Qtestframe", true)
	pdf.SetHeaderFuncMode(func() {
		if pdf.PageNo() == 1 || doc.Header == "" {
			return
		}
		pdf.SetY(8)
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(110, 110, 110)
		pdf.CellFormat(0, 5, p.text(doc.Header), "", 0, "R", false, 0, "")
		pdf.SetDrawColor(200, 200, 200)
		pdf.Line(docPDFMargin, 13.5, pageW-docPDFMargin, 13.5)
		pdf.SetTextColor(0, 0, 0)
		pdf.SetXY(docPDFMargin, docPDFMargin)
	}, false)
	pdf.SetFooterFunc(func() {
		pdf.SetY(-pdfFooterH - 2)
		pdf.SetFont("Helvetica", "I", 8)
		pdf.SetTextColor(110, 110, 110)
		pdf.CellFormat(0, 5, fmt.Sprintf(p.ds.label("page"), pdf.PageNo(), "{nb}"), "", 0, "C", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	})

	p.titlePage()
	entries, _ := doc.toc()
	links := map[int]int{} // block index → link id
	pages := map[int]int{} // block index → page
	tocStart, tocPages := 0, 0
	for _, e := range entries {
		links[e.block] = pdf.AddLink()
	}
	kept := 0 // blocks before this one were checked to stay together
	for i, b := range doc.Blocks {
		switch b.Kind {
		case BlockHeading:
			if i >= kept {
				kept = p.keepTogether(i)
			}
			p.heading(i, b)
			if id, ok := links[i]; ok {
				pdf.SetLink(id, pdf.GetY(), pdf.PageNo())
				pages[i] = pdf.PageNo()
			}
			p.headingText(b)
		case BlockTOC:
			if len(entries) == 0 {
				continue
			}
			p.newPage()
			tocStart, tocPages = pdf.PageNo(), p.tocPageCount(len(entries))
			for k := 1; k < tocPages; k++ {
				pdf.AddPage()
			}
			pdf.AddPage()
		default:
			p.block(i, b)
		}
	}
	if tocPages > 0 {
		last := pdf.PageNo()
		p.drawTOC(entries, tocStart, links, pages)
		pdf.SetPage(last)
	}
	if err := pdf.Error(); err != nil {
		return err
	}
	return pdf.Output(w)
}

// block draws block i (b) of the document, but not the table of contents.
func (p *docPDF) block(i int, b Block) {
	pdf := p.pdf
	doc := p.doc
	switch b.Kind {
	case BlockHeading:
		p.heading(i, b)
		p.headingText(b)
	case BlockParagraph:
		need := 20.0
		if i+1 < len(doc.Blocks) && doc.Blocks[i+1].Kind == BlockFigure && doc.Blocks[i+1].Figure != nil {
			// a figure's caption stays with the figure
			f := doc.Blocks[i+1].Figure
			need = 10 + f.Height*figureScale(f, p.usable, p.bottom()-p.top()-10)
		}
		if b.caption() && pdf.GetY()+need > p.bottom() && !p.fresh() {
			pdf.AddPage() // a caption stays with what follows
		}
		p.paragraph(b)
	case BlockTable:
		p.table(b)
	case BlockFields:
		p.fields(b)
	case BlockList:
		p.list(b)
	case BlockFigure:
		p.figure(b)
	case BlockPageBreak:
		p.newPage()
	}
}

// keepTogether starts a new page before block i when the unit of blocks
// that should stay on one page (Document.keepUnit) does not fit on the rest
// of this page but fits on an empty one; a unit longer than a page flows.
// It returns the end of the unit (i when there is none).
func (p *docPDF) keepTogether(i int) int {
	end := p.doc.keepUnit(i)
	if end <= i || p.fresh() || p.doc.Blocks[i].NewPage {
		return end
	}
	h, ok := p.measure(p.doc.Blocks[i:end])
	if ok && p.pdf.GetY()+headingGap(p.doc.Blocks[i])+h > p.bottom() {
		p.pdf.AddPage()
	}
	return end
}

// measure lays blocks out at the top of an empty page and returns their
// height; false when they do not fit on one page.
func (p *docPDF) measure(blocks []Block) (float64, bool) {
	if p.scratch == nil || p.measured >= 200 {
		// a fresh scratch PDF now and then: it keeps every page in memory
		p.scratch, p.measured = newDocPDF(&Document{}), 0
	}
	s := p.scratch
	p.measured++
	s.doc = &Document{Blocks: blocks}
	s.pdf.AddPage()
	page := s.pdf.PageNo()
	for j, b := range blocks {
		s.block(j, b)
	}
	if s.pdf.Error() != nil || s.pdf.PageNo() != page {
		return 0, false
	}
	return s.pdf.GetY() - s.top(), true
}

// headingGap is the space above a heading that is not at the top of a page.
func headingGap(b Block) float64 { return []float64{5, 4, 3, 2, 2, 2}[clampLevel(b.Level)-1] }

func (p *docPDF) top() float64 { return docPDFMargin }

// fresh reports whether nothing was drawn on the current page yet.
func (p *docPDF) fresh() bool { return p.pdf.GetY() <= p.top()+0.5 }

func (p *docPDF) newPage() {
	if !p.fresh() {
		p.pdf.AddPage()
	}
}

func (p *docPDF) titlePage() {
	pdf := p.pdf
	pdf.AddPage()
	pdf.SetY(70)
	pdf.SetDrawColor(47, 84, 150)
	pdf.SetLineWidth(0.8)
	pdf.Line(docPDFMargin, 64, docPDFMargin+p.usable, 64)
	pdf.SetLineWidth(0.2)
	pdf.SetFont("Helvetica", "B", 24)
	pdf.SetTextColor(31, 56, 100)
	pdf.MultiCell(0, 11, p.text(p.doc.Title), "", "L", false)
	pdf.SetTextColor(0, 0, 0)
	if p.doc.Subtitle != "" {
		pdf.Ln(2)
		pdf.SetFont("Helvetica", "", 14)
		pdf.SetTextColor(89, 89, 89)
		pdf.MultiCell(0, 7, p.text(p.doc.Subtitle), "", "L", false)
		pdf.SetTextColor(0, 0, 0)
	}
	pdf.Ln(12)
	items := append([]Item{}, p.doc.TitleFields...)
	items = append(items, Item{Label: p.ds.label("generated"), Value: generatedAt(p.ds).Format("2006-01-02 15:04 MST")})
	p.fields(Block{Kind: BlockFields, Fields: items})
	pdf.AddPage()
}

// heading moves to where heading i (b) goes (a new page when it asks for
// one, or when it would end up without the text that follows) and adds its
// bookmark.
func (p *docPDF) heading(i int, b Block) {
	pdf := p.pdf
	l := clampLevel(b.Level)
	if b.NewPage {
		p.newPage()
	}
	// keep a run of headings together with the text after them
	extra := 0.0
	for j := i + 1; j < len(p.doc.Blocks) && p.doc.Blocks[j].Kind == BlockHeading; j++ {
		extra += 12
	}
	if pdf.GetY()+28+extra > p.bottom() {
		pdf.AddPage()
	}
	if !p.fresh() {
		pdf.Ln(headingGap(b))
	}
	bl := min(l-1, p.lastBookmark+1)
	pdf.Bookmark(p.text(oneLine(b.Text)), bl, -1)
	p.lastBookmark = bl
}

func (p *docPDF) headingText(b Block) {
	pdf := p.pdf
	l := clampLevel(b.Level)
	size := docHeadingSize[l-1]
	pdf.SetFont("Helvetica", "B", size)
	if l <= 2 {
		pdf.SetTextColor(47, 84, 150)
	} else {
		pdf.SetTextColor(31, 56, 100)
	}
	pdf.MultiCell(0, size*0.5, p.text(oneLine(b.Text)), "", "L", false)
	if l == 1 {
		y := pdf.GetY() + 0.5
		pdf.SetDrawColor(47, 84, 150)
		pdf.Line(docPDFMargin, y, docPDFMargin+p.usable, y)
		pdf.Ln(1)
	}
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(1.5)
}

// flow writes running text with automatic page breaks.
func (p *docPDF) flow(fn func()) {
	pdf := p.pdf
	pdf.SetAutoPageBreak(true, p.pageH-p.bottom())
	fn()
	pdf.SetAutoPageBreak(false, docPDFMargin)
}

func (p *docPDF) paragraph(b Block) {
	if strings.TrimSpace(b.plain()) == "" {
		return
	}
	pdf := p.pdf
	p.flow(func() {
		pdf.SetX(docPDFMargin)
		for _, sp := range b.spans() {
			style := ""
			if sp.Bold {
				style += "B"
			}
			if sp.Italic {
				style += "I"
			}
			pdf.SetFont("Helvetica", style, docBodySize)
			pdf.Write(docBodyLineH, p.text(strings.ReplaceAll(sp.Text, "\r\n", "\n")))
		}
		pdf.Ln(docBodyLineH)
	})
	pdf.Ln(2)
}

func (p *docPDF) list(b Block) {
	pdf := p.pdf
	p.flow(func() {
		for _, it := range b.Items {
			if strings.TrimSpace(it) == "" {
				continue
			}
			pdf.SetFont("Helvetica", "", docBodySize)
			pdf.SetX(docPDFMargin + 2)
			pdf.CellFormat(4, docBodyLineH, p.text("•"), "", 0, "L", false, 0, "")
			pdf.MultiCell(p.usable-6, docBodyLineH, p.text(it), "", "L", false)
			pdf.Ln(0.8)
		}
	})
	pdf.Ln(1.5)
}

// widthsOf scales relative widths to the usable page width (the report
// writer's distribution when none are given).
func (p *docPDF) widthsOf(b Block) []float64 {
	n := width(Section{Columns: b.Columns, Rows: b.Rows})
	if len(b.Widths) != n {
		return p.columnWidths(Section{Columns: b.Columns, Rows: b.Rows}, n)
	}
	total := 0.0
	for _, w := range b.Widths {
		total += w
	}
	out := make([]float64, n)
	for i, w := range b.Widths {
		out[i] = p.usable * w / total
	}
	return out
}

func (p *docPDF) table(b Block) {
	pdf := p.pdf
	if len(b.Rows) == 0 {
		pdf.SetFont("Helvetica", "I", 9)
		pdf.MultiCell(0, 5, p.text(p.ds.label("noRows")), "", "L", false)
		pdf.Ln(2)
		return
	}
	widths := p.widthsOf(b)
	header := make([]string, len(widths))
	for i := range header {
		header[i] = cell(b.Columns, i)
	}
	if pdf.GetY()+3*pdfLineH+4 > p.bottom() {
		pdf.AddPage()
	}
	p.row(header, widths, true)
	for _, r := range b.Rows {
		if !p.row(r, widths, false) {
			pdf.AddPage()
			p.row(header, widths, true)
			p.row(r, widths, false)
		}
	}
	pdf.Ln(3)
}

// figure draws a figure block centred, shrunk to the page when it is
// larger; it starts a new page when it does not fit below.
func (p *docPDF) figure(b Block) {
	f := b.Figure
	if f == nil || f.Width <= 0 || f.Height <= 0 {
		return
	}
	pdf := p.pdf
	k := figureScale(f, p.usable, p.bottom()-p.top()-10) // room for its caption
	h := f.Height * k
	if pdf.GetY()+h > p.bottom() {
		pdf.AddPage()
	}
	y := pdf.GetY()
	p.pdfFigure(f, docPDFMargin+(p.usable-f.Width*k)/2, y, k)
	pdf.SetXY(docPDFMargin, y+h+3)
}

func (p *docPDF) fields(b Block) {
	pdf := p.pdf
	widths := []float64{docLabelW, p.usable - docLabelW}
	for _, f := range b.fieldItems() {
		r := []string{f.Label, f.Value}
		if !p.styledRow(r, widths, false, true) {
			pdf.AddPage()
			p.styledRow(r, widths, false, true)
		}
	}
	pdf.Ln(3)
}

// tocPageCount is the number of pages n entries need.
func (p *docPDF) tocPageCount(n int) int {
	first, per := p.tocLines()
	if n <= first {
		return 1
	}
	return 1 + (n-first+per-1)/per
}

// tocLines are the entries on the first and on further TOC pages.
func (p *docPDF) tocLines() (first, per int) {
	avail := p.bottom() - p.top()
	return int((avail - 14) / docTOCLineH), int(avail / docTOCLineH)
}

// drawTOC fills the reserved pages with the entries, their page numbers
// and links.
func (p *docPDF) drawTOC(entries []tocEntry, start int, links, pages map[int]int) {
	pdf := p.pdf
	first, per := p.tocLines()
	page, y := start, p.top()
	pdf.SetPage(page)
	pdf.SetXY(docPDFMargin, y)
	pdf.SetFont("Helvetica", "B", docHeadingSize[0])
	pdf.SetTextColor(47, 84, 150)
	pdf.CellFormat(0, 9, p.text(p.ds.label("contents")), "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	y = p.top() + 14
	onPage := 0
	limit := first
	numW := 12.0
	for _, e := range entries {
		if onPage == limit {
			page++
			pdf.SetPage(page)
			y, onPage, limit = p.top(), 0, per
		}
		indent := float64(e.level-1) * 6
		style := ""
		if e.level == 1 {
			style = "B"
		}
		pdf.SetFont("Helvetica", style, docBodySize)
		textW := p.usable - indent - numW
		title := p.text(e.text)
		for pdf.GetStringWidth(title+" ") > textW && len(title) > 4 {
			title = strings.TrimRight(title[:len(title)-4], " ") + "..."
		}
		x := docPDFMargin + indent
		pdf.SetXY(x, y)
		pdf.CellFormat(pdf.GetStringWidth(title)+1, docTOCLineH, title, "", 0, "L", false, 0, "")
		// dot leaders
		dotsX := pdf.GetX()
		pdf.SetFont("Helvetica", "", docBodySize)
		dotW := pdf.GetStringWidth(".")
		if gap := docPDFMargin + p.usable - numW - dotsX - 1; gap > dotW {
			pdf.SetTextColor(150, 150, 150)
			pdf.CellFormat(gap, docTOCLineH, strings.Repeat(".", int(gap/dotW)), "", 0, "R", false, 0, "")
			pdf.SetTextColor(0, 0, 0)
		}
		pdf.SetXY(docPDFMargin+p.usable-numW, y)
		pdf.SetFont("Helvetica", style, docBodySize)
		pdf.CellFormat(numW, docTOCLineH, fmt.Sprint(pages[e.block]), "", 0, "R", false, 0, "")
		pdf.Link(x, y, p.usable-indent, docTOCLineH, links[e.block])
		y += docTOCLineH
		onPage++
	}
}
