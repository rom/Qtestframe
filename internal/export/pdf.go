package export

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/go-pdf/fpdf"
)

const (
	pdfMargin     = 10.0
	pdfLineH      = 4.2
	pdfFontSize   = 8.0
	pdfMinColW    = 14.0
	pdfMaxColRune = 60 // cap on rune length used for column width estimation
	pdfFooterH    = 10.0
)

type pdfWriter struct {
	pdf    *fpdf.Fpdf
	tr     func(string) string
	pageH  float64
	usable float64
}

// pdfText prepares s for the core fonts: control characters are replaced and
// UTF-8 is translated to cp1252 (unmappable runes become '.').
func (p *pdfWriter) text(s string) string {
	s = strings.ToValidUTF8(s, "?")
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return r
		case r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f:
			return -1
		}
		return r
	}, s)
	return p.tr(s)
}

// WritePDF writes ds as a landscape A4 PDF document with one table per
// section. Table header rows are repeated after page breaks and every page
// carries a page number footer.
func WritePDF(w io.Writer, ds Dataset) error {
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetMargins(pdfMargin, pdfMargin, pdfMargin)
	pdf.SetAutoPageBreak(false, pdfMargin)
	pdf.AliasNbPages("{nb}")
	p := &pdfWriter{pdf: pdf, tr: pdf.UnicodeTranslatorFromDescriptor("")}
	pageW, pageH := pdf.GetPageSize()
	p.pageH = pageH
	p.usable = pageW - 2*pdfMargin

	pdf.SetTitle(ds.Title, true)
	pdf.SetCreator("Qtestframe", true)
	pdf.SetFooterFunc(func() {
		pdf.SetY(-pdfFooterH + 2)
		pdf.SetFont("Helvetica", "I", 8)
		pdf.SetTextColor(110, 110, 110)
		pdf.CellFormat(0, 5, fmt.Sprintf("Page %d/{nb}", pdf.PageNo()), "", 0, "C", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	})

	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 18)
	pdf.MultiCell(0, 9, p.text(ds.Title), "", "L", false)
	if ds.Subtitle != "" {
		pdf.SetFont("Helvetica", "", 12)
		pdf.MultiCell(0, 6, p.text(ds.Subtitle), "", "L", false)
	}
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(90, 90, 90)
	pdf.MultiCell(0, 5, p.text("Generated "+generatedAt(ds).Format("2006-01-02 15:04:05 MST")), "", "L", false)
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(4)

	for i, s := range ds.Sections {
		p.section(s, i)
	}
	if err := pdf.Error(); err != nil {
		return err
	}
	return pdf.Output(w)
}

func (p *pdfWriter) bottom() float64 { return p.pageH - pdfMargin - pdfFooterH }

func (p *pdfWriter) section(s Section, idx int) {
	pdf := p.pdf
	// keep heading together with at least the header and a couple of rows
	if pdf.GetY()+30 > p.bottom() {
		pdf.AddPage()
	}
	pdf.SetFont("Helvetica", "B", 13)
	pdf.MultiCell(0, 7, p.text(sectionTitle(s, idx)), "", "L", false)
	pdf.Ln(1)

	n := width(s)
	if n == 0 {
		pdf.SetFont("Helvetica", "I", 9)
		pdf.MultiCell(0, 5, "(no data)", "", "L", false)
		pdf.Ln(3)
		return
	}
	widths := p.columnWidths(s, n)
	header := make([]string, n)
	for i := range header {
		header[i] = cell(s.Columns, i)
	}
	p.row(header, widths, true)
	for _, r := range s.Rows {
		if !p.row(r, widths, false) {
			pdf.AddPage()
			p.row(header, widths, true)
			p.row(r, widths, false)
		}
	}
	if len(s.Rows) == 0 {
		pdf.SetFont("Helvetica", "I", 9)
		pdf.MultiCell(0, 5, "(no rows)", "", "L", false)
	}
	pdf.Ln(5)
}

// columnWidths distributes the usable page width proportionally to the
// (capped) content length of each column.
func (p *pdfWriter) columnWidths(s Section, n int) []float64 {
	weights := make([]float64, n)
	measure := func(row []string) {
		for i := 0; i < n; i++ {
			l := 0
			for _, line := range strings.Split(cell(row, i), "\n") {
				if c := utf8.RuneCountInString(line); c > l {
					l = c
				}
			}
			if l > pdfMaxColRune {
				l = pdfMaxColRune
			}
			if float64(l) > weights[i] {
				weights[i] = float64(l)
			}
		}
	}
	measure(s.Columns)
	for i, r := range s.Rows {
		if i >= 500 { // a sample is enough
			break
		}
		measure(r)
	}
	total := 0.0
	for i := range weights {
		if weights[i] < 4 {
			weights[i] = 4
		}
		total += weights[i]
	}
	out := make([]float64, n)
	minW := pdfMinColW
	if minW*float64(n) > p.usable {
		minW = p.usable / float64(n)
	}
	// assign minimum, distribute the rest proportionally
	rest := p.usable - minW*float64(n)
	for i := range out {
		out[i] = minW + rest*weights[i]/total
	}
	return out
}

// row draws one table row at the current position. It returns false (and
// draws nothing) when the row does not fit on the current page but would fit
// on a fresh one; rows taller than a page are truncated.
func (p *pdfWriter) row(cells []string, widths []float64, header bool) bool {
	pdf := p.pdf
	if header {
		pdf.SetFont("Helvetica", "B", pdfFontSize)
		pdf.SetFillColor(48, 84, 150)
		pdf.SetTextColor(255, 255, 255)
	} else {
		pdf.SetFont("Helvetica", "", pdfFontSize)
		pdf.SetTextColor(0, 0, 0)
	}
	_, top, _, _ := pdf.GetMargins()
	maxLines := int((p.bottom() - top - 3*pdfLineH) / pdfLineH)

	lines := make([][]string, len(widths))
	maxN := 1
	for i, w := range widths {
		var ls []string
		for _, b := range pdf.SplitLines([]byte(p.text(cell(cells, i))), w) {
			ls = append(ls, string(b))
		}
		if len(ls) > maxLines {
			ls = append(ls[:maxLines-1], "...")
		}
		lines[i] = ls
		if len(ls) > maxN {
			maxN = len(ls)
		}
	}
	h := float64(maxN)*pdfLineH + 1
	y := pdf.GetY()
	if y+h > p.bottom() && y > top+pdfLineH*3 {
		return false
	}
	x := pdfMargin
	style := "D"
	if header {
		style = "FD"
	}
	pdf.SetDrawColor(170, 170, 170)
	for i, w := range widths {
		pdf.Rect(x, y, w, h, style)
		pdf.SetXY(x, y+0.5)
		pdf.MultiCell(w, pdfLineH, strings.Join(lines[i], "\n"), "", "L", false)
		x += w
	}
	pdf.SetXY(pdfMargin, y+h)
	pdf.SetTextColor(0, 0, 0)
	return true
}
