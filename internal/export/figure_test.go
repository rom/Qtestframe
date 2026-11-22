package export

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"image/png"
	"io"
	"strings"
	"testing"
)

func sampleFigure() *Figure {
	return &Figure{Width: 300, Height: 120, Title: "Figure 1. A & B", Desc: "Two boxes <linked>", Shapes: []Shape{
		{Kind: ShapeRect, X: 10, Y: 10, W: 100, H: 30, Radius: 4, Fill: "#ffffff", Stroke: "#94a3b8", StrokeWidth: 1, Tip: "A box"},
		{Kind: ShapeRect, X: 150, Y: 10, W: 100, H: 30, Fill: "#f1f5f9", Stroke: "#94a3b8", StrokeWidth: 1, Dash: []float64{3, 2}},
		{Kind: ShapeRect, X: 10, Y: 60, W: 5, H: 30, Radius: 2, Fill: "#22c55e"},
		{Kind: ShapePath, Start: Point{110, 25}, Path: []PathSeg{{Cubic: true, C1: Point{130, 25}, C2: Point{130, 25}, P: Point{150, 25}}, {P: Point{160, 80}}},
			Stroke: "#2563eb", StrokeWidth: 1.4, Dash: []float64{2, 2.5}, Tip: "A depends on B"},
		{Kind: ShapePolygon, Points: []Point{{150, 25}, {143, 22}, {143, 28}}, Fill: "#2563eb", Stroke: "#000000", StrokeWidth: 0.5},
		{Kind: ShapeText, X: 20, Y: 28, Text: "Åsa & Öberg", Size: 9, Bold: true},
		{Kind: ShapeText, X: 250, Y: 100, Text: "end", Size: 7, Anchor: "end", Fill: "#475569"},
		{Kind: ShapeText, X: 150, Y: 110, Text: "middle", Size: 7, Anchor: "middle", Tip: "tip"},
		{Kind: ShapePath, Start: Point{0, 0}, Path: []PathSeg{{P: Point{5, 5}}}}, // no stroke: nothing
	}}
}

func TestFigureSVG(t *testing.T) {
	svg := sampleFigure().SVG()
	dec := xml.NewDecoder(bytes.NewReader(svg))
	names := map[string]int{}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("invalid SVG: %v\n%s", err, svg)
		}
		if se, ok := tok.(xml.StartElement); ok {
			names[se.Name.Local]++
		}
	}
	if names["svg"] != 1 || names["rect"] != 4 || names["path"] != 2 || names["polygon"] != 1 || names["text"] != 3 || names["title"] != 4 {
		t.Errorf("elements %v", names)
	}
	s := string(svg)
	for _, want := range []string{`viewBox="0 0 300 120"`, `role="img"`, "Figure 1. A &amp; B", "Two boxes &lt;linked&gt;", `stroke-dasharray="2 2.5"`,
		`text-anchor="end"`, `font-weight="bold"`, "Åsa &amp; Öberg", "C 130 25, 130 25, 150 25 L 160 80", `rx="4"`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
	if !bytes.Equal(svg, sampleFigure().SVG()) {
		t.Error("SVG is not deterministic")
	}
}

func TestFigurePNG(t *testing.T) {
	b, err := sampleFigure().PNG(2)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 600 || img.Bounds().Dy() != 240 {
		t.Errorf("size %v", img.Bounds())
	}
	// the green stripe is drawn, the background stays white
	if r, g, bl, _ := img.At(24, 150).RGBA(); r>>8 != 0x22 || g>>8 != 0xc5 || bl>>8 != 0x5e {
		t.Errorf("stripe colour %x %x %x", r>>8, g>>8, bl>>8)
	}
	if r, g, bl, _ := img.At(590, 5).RGBA(); r>>8 != 0xff || g>>8 != 0xff || bl>>8 != 0xff {
		t.Error("background")
	}
	if again, _ := sampleFigure().PNG(2); !bytes.Equal(again, b) {
		t.Error("PNG is not deterministic")
	}
}

func TestFitText(t *testing.T) {
	if got := FitText("Short", 8, false, 100); got != "Short" {
		t.Error(got)
	}
	long := "A rather long title that will certainly not fit into a small box"
	got := FitText(long, 8, false, 80)
	if !strings.HasSuffix(got, "…") || TextWidth(got, 8, false) > 80 || len(got) < 5 {
		t.Errorf("%q (%.1f)", got, TextWidth(got, 8, false))
	}
	if FitText(long, 8, false, 1) != "" {
		t.Error("nothing fits")
	}
	if TextWidth("WWW", 10, true) <= TextWidth("iii", 10, false) {
		t.Error("widths")
	}
	if d := dashes([]Point{{0, 0}, {10, 0}}, []float64{2, 2}); len(d) != 3 {
		t.Errorf("dashes %v", d)
	}
}

func figureDoc() *Document {
	return &Document{Title: "Doc", Lang: "en", Blocks: []Block{TOC(), Heading(1, "One"), RichParagraph(Span{Text: "Figure 1. A & B", Bold: true}),
		FigureBlock(sampleFigure()), FigureBlock(nil), FigureBlock(&Figure{Width: 4000, Height: 6000, Title: "Huge"}), Paragraph("After")}}
}

func TestWriteDocumentFigure(t *testing.T) {
	doc := figureDoc()
	var md, pdf, docx bytes.Buffer
	for f, b := range map[Format]*bytes.Buffer{MD: &md, PDF: &pdf, DOCX: &docx} {
		if err := WriteDocument(b, f, doc); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(md.String(), `![Figure 1. A \& B](data:image/svg+xml;base64,`) {
		t.Errorf("Markdown:\n%s", md.String())
	}
	if !strings.HasPrefix(pdf.String(), "%PDF") {
		t.Error("PDF")
	}
	zr, err := zip.NewReader(bytes.NewReader(docx.Bytes()), int64(docx.Len()))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		parts[f.Name] = string(b)
	}
	body := parts["word/document.xml"]
	if strings.Count(body, "<wp:inline") != 2 || !strings.Contains(body, `r:embed="rIdP2"`) || !strings.Contains(body, "<w:keepNext/>") {
		t.Errorf("DOCX body:\n%s", body)
	}
	if err := xml.Unmarshal([]byte(body), new(any)); err != nil {
		t.Error(err)
	}
	for _, p := range []string{"word/media/figure1.png", "word/media/figure1.svg", "word/media/figure2.png", "word/media/figure2.svg"} {
		if parts[p] == "" {
			t.Errorf("no %s", p)
		}
	}
	// the huge figure is shrunk to the page
	if strings.Contains(body, `cx="50800000"`) {
		t.Error("not scaled")
	}
}
