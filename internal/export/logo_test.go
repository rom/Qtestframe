package export

import (
	"archive/zip"
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"strings"
	"testing"
	"time"
)

const testLogoSVG = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 200 100" width="200mm" height="100mm" onload="alert(1)">
  <style>.blue { fill: #0000ff } /* comment */ #green{fill:rgb(0,128,0)} @media print { rect { fill: red } }</style>
  <script>alert('x')</script>
  <defs><linearGradient id="g"><stop offset="0" stop-color="#ff0000"/><stop offset="1" stop-color="#00ff00"/></linearGradient></defs>
  <image href="https://example.com/x.png" x="0" y="0" width="200" height="100"/>
  <foreignObject x="0" y="0" width="200" height="100"><div xmlns="http://www.w3.org/1999/xhtml">hi</div></foreignObject>
  <rect class="blue" x="0" y="0" width="50" height="50"/>
  <circle id="green" cx="125" cy="25" r="20"/>
  <path d="M150 50 h40 v40 h-40 z" fill="url(#g)"/>
  <g transform="translate(0 50)" opacity="0.5"><rect x="0" y="0" width="50" height="50" style="fill:black"/></g>
  <path d="M60 60 a 20 20 0 1 0 40 0 a20,20 0 1,0 -40,0" fill="none" stroke="#000" stroke-width="4"/>
  <polygon points="110,60 140,60 125,90" fill="#123"/>
  <use href="#green" x="0" y="0"/>
</svg>`

func pixel(img image.Image, x, y int) color.NRGBA {
	return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
}

func TestRasterizeSVG(t *testing.T) {
	img, err := RasterizeSVG([]byte(testLogoSVG), 400)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 400 || b.Dy() != 200 {
		t.Fatalf("size %v", b)
	}
	// scale 2: the blue rect (class rule), the green circle (#id rule), the gradient's first stop
	if p := pixel(img, 50, 50); p.B != 255 || p.R != 0 || p.A != 255 {
		t.Errorf("blue rect: %v", p)
	}
	if p := pixel(img, 250, 50); p.G != 128 || p.A != 255 {
		t.Errorf("green circle: %v", p)
	}
	if p := pixel(img, 340, 140); p.R != 255 || p.A != 255 {
		t.Errorf("gradient square: %v", p)
	}
	// half transparent group
	if p := pixel(img, 50, 150); p.A < 120 || p.A > 135 {
		t.Errorf("opacity: %v", p)
	}
	// the arc is a ring: its centre is empty, its edge drawn
	if p := pixel(img, 160, 120); p.A != 0 {
		t.Errorf("ring centre: %v", p)
	}
	if p := pixel(img, 120, 120); p.A == 0 {
		t.Errorf("ring edge empty")
	}
	// the image, the foreign object and the CSS of @media are ignored: the top right corner is empty
	if p := pixel(img, 395, 5); p.A != 0 {
		t.Errorf("corner: %v", p)
	}
	if p := pixel(img, 250, 160); p.A == 0 {
		t.Errorf("polygon not drawn")
	}
}

func TestRasterizeSVGRejects(t *testing.T) {
	for name, svg := range map[string]string{
		"not svg":   `<html><body/></html>`,
		"entity":    `<!DOCTYPE svg [<!ENTITY x SYSTEM "file:///etc/passwd">]><svg viewBox="0 0 1 1">&x;</svg>`,
		"no size":   `<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"/></svg>`,
		"broken":    `<svg viewBox="0 0 1 1"><rect>`,
		"extreme":   `<svg viewBox="0 0 1000 1"/>`,
		"two roots": `<svg viewBox="0 0 1 1"/><svg viewBox="0 0 1 1"/>`,
	} {
		if _, err := RasterizeSVG([]byte(svg), 100); !errors.Is(err, ErrSVG) {
			t.Errorf("%s: %v", name, err)
		}
	}
	deep := strings.Repeat("<g>", 100) + strings.Repeat("</g>", 100)
	if _, err := RasterizeSVG([]byte(`<svg viewBox="0 0 1 1">`+deep+`</svg>`), 100); !errors.Is(err, ErrSVG) {
		t.Errorf("deep: %v", err)
	}
}

func TestSVGHelpers(t *testing.T) {
	for in, want := range map[string]color.NRGBA{
		"#abc": {0xaa, 0xbb, 0xcc, 255}, "#11223344": {0x11, 0x22, 0x33, 0x44}, "rgb(10%, 0, 255)": {26, 0, 255, 255},
		"rgba(1,2,3,0.5)": {1, 2, 3, 128}, "Navy": {0, 0, 128, 255},
	} {
		if c, ok := parseColor(in); !ok || c != want {
			t.Errorf("%s: %v %v", in, c, ok)
		}
	}
	for _, bad := range []string{"#12", "#ggg", "rgb(1)", "hsl(1,2,3)", "rgb(a,b,c)"} {
		if _, ok := parseColor(bad); ok {
			t.Errorf("%s accepted", bad)
		}
	}
	if v := numbers("1,2 3-4.5.5e1"); len(v) != 5 || v[3] != -4.5 || v[4] != 5 {
		t.Errorf("numbers %v", v)
	}
	m := parseTransform("translate(10,20) scale(2) rotate(90) skewX(0) matrix(1 0 0 1 0 0) bogus(1)")
	if p := m.apply(1, 0); abs(p.X-10) > 1e-9 || abs(p.Y-22) > 1e-9 {
		t.Errorf("transform %v", p)
	}
	if p := parseTransform("rotate(180 5 5) skewY(0)").apply(0, 0); abs(p.X-10) > 1e-9 || abs(p.Y-10) > 1e-9 {
		t.Errorf("rotate about %v", p)
	}
	for in, want := range map[string]float64{"10": 10, "1in": 96, "50%": 50, "72pt": 96, "2.54cm": 96, "25.4mm": 96} {
		if v, ok := svgLength(in, 100); !ok || abs(v-want) > 1e-9 {
			t.Errorf("%s: %v", in, v)
		}
	}
	if _, ok := svgLength("x", 1); ok {
		t.Error("bad length accepted")
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func TestSVGShapes(t *testing.T) {
	svg := `<svg viewBox="0 0 100 100" width="100" height="100">
	<rect x="10" y="10" width="30" height="20" rx="5" fill="#f00" stroke="#00f" stroke-width="2"/>
	<ellipse cx="70" cy="20" rx="20" ry="10" fill="currentColor" color="#0f0"/>
	<line x1="0" y1="50" x2="100" y2="50" stroke="black" stroke-width="2"/>
	<polyline points="10 70 30 90 50 70" fill="none" stroke="#000"/>
	<path d="M60 60 C 60 80 80 80 80 60 S 100 40 100 60 Q 90 90 70 95 T 60 60 L 65 65 H 70 V 70 Z m 5 5 l 1 1 z" fill="#808080"/>
	<rect x="0" y="0" width="5" height="5" display="none"/>
	<rect x="95" y="95" width="5" height="5" style="visibility:hidden"/>
	<rect x="0" y="95" width="5" height="5" fill="url(#missing) #ff00ff"/>
	</svg>`
	img, err := RasterizeSVG([]byte(svg), 100)
	if err != nil {
		t.Fatal(err)
	}
	if p := pixel(img, 25, 20); p.R != 255 || p.A != 255 {
		t.Errorf("rect %v", p)
	}
	if p := pixel(img, 70, 20); p.G != 255 {
		t.Errorf("currentColor %v", p)
	}
	if p := pixel(img, 50, 50); p.A == 0 {
		t.Errorf("line")
	}
	if p := pixel(img, 2, 2); p.A != 0 {
		t.Errorf("display none drawn")
	}
	if p := pixel(img, 97, 97); p.A != 0 {
		t.Errorf("hidden drawn")
	}
	if p := pixel(img, 2, 97); p.R != 255 || p.B != 255 {
		t.Errorf("fallback paint %v", p)
	}
}

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestPrepareLogo(t *testing.T) {
	p, err := PrepareLogo("logo.png", testPNG(t, 2000, 500))
	if err != nil {
		t.Fatal(err)
	}
	if p.ContentType != "image/png" || p.Width != LogoMaxSide || p.Height != 400 || p.Source != "png" {
		t.Errorf("png: %+v", p.Width)
	}
	var jb bytes.Buffer
	_ = jpeg.Encode(&jb, image.NewRGBA(image.Rect(0, 0, 40, 20)), nil)
	if p, err := PrepareLogo("logo.jpg", jb.Bytes()); err != nil || p.ContentType != "image/jpeg" || p.Width != 40 {
		t.Errorf("jpeg: %v", err)
	}
	if p, err := PrepareLogo("logo.svg", []byte(testLogoSVG)); err != nil || p.ContentType != "image/png" || p.Width != LogoSVGSide || p.Height != LogoSVGSide/2 || p.Source != "svg" {
		t.Errorf("svg: %v", err)
	}
	// detected from the content too
	if !IsSVG("logo", []byte(`<?xml version="1.0"?><svg viewBox="0 0 1 1"/>`)) || IsSVG("a.png", testPNG(t, 8, 8)) {
		t.Error("IsSVG")
	}
	for name, data := range map[string][]byte{
		"empty": {}, "text": []byte("hello"), "tiny": testPNG(t, 4, 4), "thin": testPNG(t, 8, 800),
		"blank.svg": []byte(`<svg viewBox="0 0 10 10"><text>A</text></svg>`), "bad.svg": []byte(`<svg>`),
		"big": make([]byte, MaxLogoBytes+1),
	} {
		if _, err := PrepareLogo(name, data); !errors.Is(err, ErrLogo) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func testLogo(t *testing.T, placement string) *Logo {
	p, err := PrepareLogo("logo.svg", []byte(testLogoSVG))
	if err != nil {
		t.Fatal(err)
	}
	return &Logo{Data: p.Data, ContentType: p.ContentType, Width: p.Width, Height: p.Height, Placement: placement, MaxWidthMM: 60, MaxHeightMM: 25, Alt: "ACME"}
}

func TestLogoSize(t *testing.T) {
	l := &Logo{Data: []byte{1}, ContentType: "image/png", Width: 960, Height: 480}
	if w, h := l.SizeMM(); abs(w-40) > 1e-9 || abs(h-20) > 1e-9 { // default 50 × 20 box
		t.Errorf("default %v %v", w, h)
	}
	l.MaxWidthMM, l.MaxHeightMM = 1000, 1000
	if w, _ := l.SizeMM(); abs(w-254) < 1e-9 || w > MaxLogoWidthMM {
		t.Errorf("clamped %v", w)
	}
	small := &Logo{Data: []byte{1}, ContentType: "image/jpeg", Width: 96, Height: 96, MaxWidthMM: 100, MaxHeightMM: 40}
	if w, h := small.SizeMM(); abs(w-25.4) > 1e-9 || abs(h-25.4) > 1e-9 {
		t.Errorf("not enlarged %v %v", w, h)
	}
	if w, _ := (*Logo)(nil).SizeMM(); w != 0 {
		t.Error("nil logo")
	}
}

func zipPart(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name == name {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			return b
		}
	}
	return nil
}

func TestLogoInReports(t *testing.T) {
	for _, pl := range LogoPlacements {
		ds := Dataset{Title: "Report", GeneratedAt: time.Unix(0, 0), Sections: []Section{{Name: "a", Columns: []string{"A"}, Rows: [][]string{{"1"}}}}, Logo: testLogo(t, pl)}
		var pdf, docx, md bytes.Buffer
		if err := WritePDF(&pdf, ds); err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(pdf.Bytes(), []byte("/Subtype /Image")) {
			t.Errorf("%s: no image in the PDF", pl)
		}
		if err := WriteDOCX(&docx, ds); err != nil {
			t.Fatal(err)
		}
		if zipPart(t, docx.Bytes(), "word/media/logo.png") == nil {
			t.Errorf("%s: no logo part", pl)
		}
		doc := string(zipPart(t, docx.Bytes(), "word/document.xml"))
		if !strings.Contains(doc, `r:embed="rIdLogo"`) || !strings.Contains(doc, `descr="ACME"`) {
			t.Errorf("%s: no logo picture: %s", pl, doc[:300])
		}
		if !strings.Contains(string(zipPart(t, docx.Bytes(), "word/_rels/document.xml.rels")), "media/logo.png") {
			t.Errorf("%s: no relationship", pl)
		}
		if err := WriteMarkdown(&md, ds); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(md.String(), "data:image/png;base64,") {
			t.Errorf("%s: markdown %q", pl, md.String()[:80])
		}
		if pl != LogoLeft && !strings.Contains(md.String(), `align="`+pl+`"`) {
			t.Errorf("%s: markdown placement", pl)
		}
	}
	// without a logo nothing changes
	var docx bytes.Buffer
	_ = WriteDOCX(&docx, Dataset{Title: "x"})
	if zipPart(t, docx.Bytes(), "word/media/logo.png") != nil {
		t.Error("logo part without a logo")
	}
}

func TestLogoInDocuments(t *testing.T) {
	doc := &Document{Title: "Spec", Lang: "sv", GeneratedAt: time.Unix(0, 0), Blocks: []Block{TOC(), Heading(1, "A"), Paragraph("x")}, Logo: testLogo(t, LogoRight)}
	var pdf, docx, md bytes.Buffer
	if err := WriteDocument(&pdf, PDF, doc); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf.Bytes(), []byte("/Subtype /Image")) {
		t.Error("no image in the PDF")
	}
	if err := WriteDocument(&docx, DOCX, doc); err != nil {
		t.Fatal(err)
	}
	body := string(zipPart(t, docx.Bytes(), "word/document.xml"))
	if !strings.Contains(body, `<w:jc w:val="right"/>`) || zipPart(t, docx.Bytes(), "word/media/logo.png") == nil {
		t.Error("no logo in the DOCX")
	}
	if err := WriteDocument(&md, MD, doc); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(md.String(), `<p align="right"><img src="data:image/png;base64,`) {
		t.Errorf("markdown %q", md.String()[:60])
	}
	// a JPEG logo
	var jb bytes.Buffer
	_ = jpeg.Encode(&jb, image.NewRGBA(image.Rect(0, 0, 40, 20)), nil)
	doc.Logo = &Logo{Data: jb.Bytes(), ContentType: "image/jpeg", Width: 40, Height: 20}
	docx.Reset()
	pdf.Reset()
	if err := WriteDocument(&docx, DOCX, doc); err != nil || zipPart(t, docx.Bytes(), "word/media/logo.jpeg") == nil {
		t.Errorf("jpeg docx %v", err)
	}
	if err := WriteDocument(&pdf, PDF, doc); err != nil || !bytes.Contains(pdf.Bytes(), []byte("/DCTDecode")) {
		t.Errorf("jpeg pdf %v", err)
	}
	// a broken image does not stop the document
	doc.Logo = &Logo{Data: []byte("not a png"), ContentType: "image/png", Width: 10, Height: 10}
	pdf.Reset()
	if err := WriteDocument(&pdf, PDF, doc); err != nil {
		t.Errorf("broken logo: %v", err)
	}
}
