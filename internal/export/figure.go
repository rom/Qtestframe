package export

import (
	"bytes"
	"encoding/base64"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/go-pdf/fpdf"
)

// Figure is a vector drawing (a diagram) of a document: shapes on a canvas
// of Width × Height points (1/72 inch). The writers keep it a vector
// graphic: SVG in Markdown and DOCX (with a PNG rendering for word
// processors that do not read SVG), PDF drawing operators in PDF. Text is
// set in Helvetica (TextWidth measures it).
type Figure struct {
	Width, Height float64
	// Title names the figure (SVG <title>, the Markdown alternative text);
	// Desc is its text alternative (SVG <desc>, the DOCX description).
	Title  string
	Desc   string
	Shapes []Shape
}

// ShapeKind is the kind of a figure shape.
type ShapeKind int

// Shape kinds.
const (
	// ShapeRect: X, Y, W, H with rounded corners of Radius.
	ShapeRect ShapeKind = iota
	// ShapePath: an open path from Start through Path (stroked only).
	ShapePath
	// ShapePolygon: the closed polygon Points.
	ShapePolygon
	// ShapeText: Text with its baseline at X, Y; Anchor start (default),
	// middle or end.
	ShapeText
)

// Point is a point of a figure.
type Point struct{ X, Y float64 }

// PathSeg is a segment of a path: a line to P, or (Cubic) a Bézier curve
// to P with the control points C1 and C2.
type PathSeg struct {
	Cubic  bool
	C1, C2 Point
	P      Point
}

// Shape is one element of a figure. Colours are "#rrggbb" ("" for none).
type Shape struct {
	Kind        ShapeKind
	X, Y, W, H  float64
	Radius      float64
	Start       Point
	Path        []PathSeg
	Points      []Point
	Fill        string
	Stroke      string
	StrokeWidth float64
	Dash        []float64
	Text        string
	Size        float64
	Bold        bool
	Anchor      string
	// Tip is a tooltip (SVG <title>).
	Tip string
}

// FigureBlock returns a block with the figure f.
func FigureBlock(f *Figure) Block { return Block{Kind: BlockFigure, Figure: f} }

// ---------------------------------------------------------------- text metrics

var (
	measureMu  sync.Mutex
	measurePDF *fpdf.Fpdf
	measureTr  func(string) string
)

// TextWidth is the width in points of s set in Helvetica (bold) at size
// points, as the PDF writer sets it.
func TextWidth(s string, size float64, bold bool) float64 {
	measureMu.Lock()
	defer measureMu.Unlock()
	if measurePDF == nil {
		measurePDF = fpdf.New("P", "pt", "A4", "")
		measureTr = measurePDF.UnicodeTranslatorFromDescriptor("")
	}
	style := ""
	if bold {
		style = "B"
	}
	measurePDF.SetFont("Helvetica", style, size)
	return measurePDF.GetStringWidth(measureTr(s))
}

// FitText shortens s with an ellipsis so that it is at most w points wide.
func FitText(s string, size float64, bold bool, w float64) string {
	s = strings.Join(strings.Fields(s), " ")
	if TextWidth(s, size, bold) <= w {
		return s
	}
	r := []rune(s)
	lo, hi := 0, len(r)
	for lo < hi { // the longest prefix that fits with the ellipsis
		mid := (lo + hi + 1) / 2
		if TextWidth(strings.TrimRight(string(r[:mid]), " ")+"…", size, bold) <= w {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo == 0 {
		return ""
	}
	return strings.TrimRight(string(r[:lo]), " ") + "…"
}

// ---------------------------------------------------------------- SVG

// fnum formats a coordinate (two decimals at most).
func fnum(v float64) string {
	v = math.Round(v*100) / 100
	if v == 0 {
		v = 0 // no "-0"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func (s Shape) pathData() string {
	var b strings.Builder
	b.WriteString("M " + fnum(s.Start.X) + " " + fnum(s.Start.Y))
	for _, sg := range s.Path {
		if sg.Cubic {
			b.WriteString(" C " + fnum(sg.C1.X) + " " + fnum(sg.C1.Y) + ", " + fnum(sg.C2.X) + " " + fnum(sg.C2.Y) + ", " + fnum(sg.P.X) + " " + fnum(sg.P.Y))
		} else {
			b.WriteString(" L " + fnum(sg.P.X) + " " + fnum(sg.P.Y))
		}
	}
	return b.String()
}

func svgPaint(b *strings.Builder, s Shape) {
	fill := s.Fill
	if fill == "" {
		fill = "none"
	}
	b.WriteString(` fill="` + esc(fill) + `"`)
	if s.Stroke != "" && s.StrokeWidth > 0 {
		b.WriteString(` stroke="` + esc(s.Stroke) + `" stroke-width="` + fnum(s.StrokeWidth) + `"`)
		if len(s.Dash) > 0 {
			ds := make([]string, len(s.Dash))
			for i, d := range s.Dash {
				ds[i] = fnum(d)
			}
			b.WriteString(` stroke-dasharray="` + strings.Join(ds, " ") + `"`)
		}
	}
}

// SVG renders the figure as a standalone SVG document (role img, labelled
// by its title and description).
func (f *Figure) SVG() []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" version="1.1" width="` + fnum(f.Width) + `pt" height="` + fnum(f.Height) + `pt" viewBox="0 0 ` +
		fnum(f.Width) + " " + fnum(f.Height) + `" role="img" aria-labelledby="fig-title fig-desc" font-family="Helvetica, Arial, sans-serif">` + "\n")
	b.WriteString(`<title id="fig-title">` + esc(f.Title) + `</title>` + "\n")
	b.WriteString(`<desc id="fig-desc">` + esc(f.Desc) + `</desc>` + "\n")
	b.WriteString(`<rect x="0" y="0" width="` + fnum(f.Width) + `" height="` + fnum(f.Height) + `" fill="#ffffff"/>` + "\n")
	for _, s := range f.Shapes {
		tip := ""
		if s.Tip != "" {
			tip = "<title>" + esc(s.Tip) + "</title>"
		}
		closeTag := func(name string) {
			if tip != "" {
				b.WriteString(">" + tip + "</" + name + ">\n")
			} else {
				b.WriteString("/>\n")
			}
		}
		switch s.Kind {
		case ShapeRect:
			b.WriteString(`<rect x="` + fnum(s.X) + `" y="` + fnum(s.Y) + `" width="` + fnum(s.W) + `" height="` + fnum(s.H) + `"`)
			if s.Radius > 0 {
				b.WriteString(` rx="` + fnum(s.Radius) + `"`)
			}
			svgPaint(&b, s)
			closeTag("rect")
		case ShapePath:
			b.WriteString(`<path d="` + s.pathData() + `"`)
			s.Fill = ""
			svgPaint(&b, s)
			closeTag("path")
		case ShapePolygon:
			pts := make([]string, len(s.Points))
			for i, p := range s.Points {
				pts[i] = fnum(p.X) + "," + fnum(p.Y)
			}
			b.WriteString(`<polygon points="` + strings.Join(pts, " ") + `"`)
			svgPaint(&b, s)
			closeTag("polygon")
		case ShapeText:
			fill := s.Fill
			if fill == "" {
				fill = "#000000"
			}
			b.WriteString(`<text x="` + fnum(s.X) + `" y="` + fnum(s.Y) + `" font-size="` + fnum(s.Size) + `" fill="` + esc(fill) + `"`)
			if s.Bold {
				b.WriteString(` font-weight="bold"`)
			}
			if s.Anchor == "middle" || s.Anchor == "end" {
				b.WriteString(` text-anchor="` + s.Anchor + `"`)
			}
			b.WriteString(">" + esc(s.Text) + tip + "</text>\n")
		}
	}
	b.WriteString("</svg>\n")
	return []byte(b.String())
}

// svgDataURI is the figure as a data: URI.
func (f *Figure) svgDataURI() string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(f.SVG())
}

// ---------------------------------------------------------------- colours

// rgb parses "#rrggbb" (ok false for anything else).
func rgb(c string) (r, g, b int, ok bool) {
	if len(c) != 7 || c[0] != '#' {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(c[1:], 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(v >> 16 & 0xff), int(v >> 8 & 0xff), int(v & 0xff), true
}

// ---------------------------------------------------------------- PDF

// pdfFigure draws f at (x, y) mm scaled by k (mm per point).
func (p *pdfWriter) pdfFigure(f *Figure, x, y, k float64) {
	pdf := p.pdf
	X := func(v float64) float64 { return x + v*k }
	Y := func(v float64) float64 { return y + v*k }
	for _, s := range f.Shapes {
		fillR, fillG, fillB, fill := rgb(s.Fill)
		strokeR, strokeG, strokeB, stroke := rgb(s.Stroke)
		stroke = stroke && s.StrokeWidth > 0
		if fill {
			pdf.SetFillColor(fillR, fillG, fillB)
		}
		if stroke {
			pdf.SetDrawColor(strokeR, strokeG, strokeB)
			pdf.SetLineWidth(s.StrokeWidth * k)
			dash := make([]float64, len(s.Dash))
			for i, d := range s.Dash {
				dash[i] = d * k
			}
			pdf.SetDashPattern(dash, 0)
		}
		style := ""
		switch {
		case fill && stroke:
			style = "FD"
		case fill:
			style = "F"
		case stroke:
			style = "D"
		}
		switch s.Kind {
		case ShapeRect:
			if style == "" {
				continue
			}
			if s.Radius > 0 {
				pdf.RoundedRect(X(s.X), Y(s.Y), s.W*k, s.H*k, min(s.Radius, s.W/2, s.H/2)*k, "1234", style)
			} else {
				pdf.Rect(X(s.X), Y(s.Y), s.W*k, s.H*k, style)
			}
		case ShapePath:
			if !stroke {
				continue
			}
			pdf.MoveTo(X(s.Start.X), Y(s.Start.Y))
			for _, sg := range s.Path {
				if sg.Cubic {
					pdf.CurveBezierCubicTo(X(sg.C1.X), Y(sg.C1.Y), X(sg.C2.X), Y(sg.C2.Y), X(sg.P.X), Y(sg.P.Y))
				} else {
					pdf.LineTo(X(sg.P.X), Y(sg.P.Y))
				}
			}
			pdf.DrawPath("D")
		case ShapePolygon:
			if style == "" || len(s.Points) < 3 {
				continue
			}
			pts := make([]fpdf.PointType, len(s.Points))
			for i, pt := range s.Points {
				pts[i] = fpdf.PointType{X: X(pt.X), Y: Y(pt.Y)}
			}
			pdf.Polygon(pts, style)
		case ShapeText:
			fs := ""
			if s.Bold {
				fs = "B"
			}
			pdf.SetFont("Helvetica", fs, s.Size*k/0.352778) // font size in points
			r, g, b, ok := rgb(s.Fill)
			if !ok {
				r, g, b = 0, 0, 0
			}
			pdf.SetTextColor(r, g, b)
			t := p.text(s.Text)
			tx := X(s.X)
			switch s.Anchor {
			case "middle":
				tx -= pdf.GetStringWidth(t) / 2
			case "end":
				tx -= pdf.GetStringWidth(t)
			}
			pdf.Text(tx, Y(s.Y), t)
		}
		if stroke && len(s.Dash) > 0 {
			pdf.SetDashPattern(nil, 0)
		}
	}
	pdf.SetDashPattern(nil, 0)
	pdf.SetLineWidth(0.2)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetFillColor(255, 255, 255)
	pdf.SetTextColor(0, 0, 0)
}

// pdfPointMM is one point in millimetres.
const pdfPointMM = 25.4 / 72

// figureScale is the scale (mm per point) of f in a box of w × h mm: the
// figure at its natural size when it fits, else shrunk to fit.
func figureScale(f *Figure, w, h float64) float64 {
	k := pdfPointMM
	if f.Width > 0 {
		k = min(k, w/f.Width)
	}
	if f.Height > 0 {
		k = min(k, h/f.Height)
	}
	return k
}

// markdownFigure is the Markdown image of f (an SVG data URI).
func markdownFigure(f *Figure) string {
	alt := strings.NewReplacer("[", "(", "]", ")", "\n", " ").Replace(f.Title)
	var b bytes.Buffer
	b.WriteString("![" + escapeInline(alt) + "](" + f.svgDataURI() + ")")
	return b.String()
}
