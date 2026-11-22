package export

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
)

// Logo is the logotype shown at the top of the first page of a report or
// generated document: a PNG or JPEG image (see PrepareLogo), where it is
// placed and how large it may be. The image keeps its aspect ratio and is
// never shown larger than its own size at 96 dpi.
type Logo struct {
	// Data is the PNG or JPEG image, ContentType "image/png" or
	// "image/jpeg"; Width and Height are its size in pixels.
	Data          []byte
	ContentType   string
	Width, Height int
	// Placement is LogoLeft (default), LogoCenter or LogoRight.
	Placement string
	// MaxWidthMM and MaxHeightMM bound the printed size (defaults
	// DefaultLogoMaxWidthMM, DefaultLogoMaxHeightMM).
	MaxWidthMM, MaxHeightMM float64
	// Alt is the text alternative (the organisation's or project's name).
	Alt string
}

// Logo placements.
const (
	LogoLeft   = "left"
	LogoCenter = "center"
	LogoRight  = "right"
)

// LogoPlacements lists the valid placements.
var LogoPlacements = []string{LogoLeft, LogoCenter, LogoRight}

// Logo size bounds and defaults (millimetres).
const (
	DefaultLogoMaxWidthMM  = 50
	DefaultLogoMaxHeightMM = 20
	MinLogoMM              = 5
	MaxLogoWidthMM         = 170
	MaxLogoHeightMM        = 40
)

const mmPerPixel = 25.4 / 96

// ok reports whether the logo can be drawn.
func (l *Logo) ok() bool {
	return l != nil && len(l.Data) > 0 && l.Width > 0 && l.Height > 0 && (l.ContentType == "image/png" || l.ContentType == "image/jpeg")
}

// SizeMM returns the printed size of the logo in millimetres: its own size
// at 96 dpi, scaled down to fit the maximum width and height.
func (l *Logo) SizeMM() (w, h float64) {
	if !l.ok() {
		return 0, 0
	}
	mw, mh := l.MaxWidthMM, l.MaxHeightMM
	if mw <= 0 {
		mw = DefaultLogoMaxWidthMM
	}
	if mh <= 0 {
		mh = DefaultLogoMaxHeightMM
	}
	mw = min(max(mw, MinLogoMM), MaxLogoWidthMM)
	mh = min(max(mh, MinLogoMM), MaxLogoHeightMM)
	w, h = float64(l.Width)*mmPerPixel, float64(l.Height)*mmPerPixel
	k := min(1, mw/w, mh/h)
	return w * k, h * k
}

func (l *Logo) placement() string {
	switch l.Placement {
	case LogoCenter, LogoRight:
		return l.Placement
	}
	return LogoLeft
}

func (l *Logo) ext() string {
	if l.ContentType == "image/jpeg" {
		return "jpeg"
	}
	return "png"
}

// pdfLogo draws the logo at the top of the current page between the left
// margin and left+usable; it returns the height used (0 without a logo).
func pdfLogo(pdf *fpdf.Fpdf, l *Logo, left, top, usable float64) float64 {
	if !l.ok() {
		return 0
	}
	w, h := l.SizeMM()
	w, h = min(w, usable), h*min(1, usable/w)
	name := "qtf-logo"
	info := pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: strings.ToUpper(l.ext())}, bytes.NewReader(l.Data))
	if info == nil || pdf.Err() {
		// an image the PDF library cannot read must not stop the report
		pdf.ClearError()
		return 0
	}
	x := left
	switch l.placement() {
	case LogoCenter:
		x = left + (usable-w)/2
	case LogoRight:
		x = left + usable - w
	}
	pdf.ImageOptions(name, x, top, w, h, false, fpdf.ImageOptions{ImageType: strings.ToUpper(l.ext())}, 0, "")
	return h
}

// docxLogoRel is the relationship of the logo image (rIdLogo).
func docxLogoRel(l *Logo) string {
	if !l.ok() {
		return ""
	}
	return `<Relationship Id="rIdLogo" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/logo.` + l.ext() + `"/>`
}

// docxLogoPart is the name of the logo image part.
func docxLogoPart(l *Logo) string { return "word/media/logo." + l.ext() }

// docxLogo writes the logo as an inline picture in a paragraph aligned to
// its placement (the document must declare the wp, a and pic namespaces).
func docxLogo(b *strings.Builder, l *Logo, id int) {
	if !l.ok() {
		return
	}
	w, h := l.SizeMM()
	const emuPerMM = 36000
	cx, cy := int(w*emuPerMM), int(h*emuPerMM)
	jc := map[string]string{LogoLeft: "left", LogoCenter: "center", LogoRight: "right"}[l.placement()]
	alt := esc(l.Alt)
	fmt.Fprintf(b, `<w:p><w:pPr><w:spacing w:after="240"/><w:jc w:val="%s"/></w:pPr><w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0">`+
		`<wp:extent cx="%d" cy="%d"/><wp:docPr id="%d" name="Logo" descr="%s"/><wp:cNvGraphicFramePr><a:graphicFrameLocks noChangeAspect="1"/></wp:cNvGraphicFramePr>`+
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic>`+
		`<pic:nvPicPr><pic:cNvPr id="%d" name="logo.%s" descr="%s"/><pic:cNvPicPr/></pic:nvPicPr>`+
		`<pic:blipFill><a:blip r:embed="rIdLogo"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`+
		`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>`+
		`</pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`,
		jc, cx, cy, id, alt, id, l.ext(), alt, cx, cy)
}

// markdownLogo is the logo as a Markdown image with a data URI (an HTML
// <p align> keeps the placement where the viewer renders HTML; plain
// Markdown shows the image).
func markdownLogo(l *Logo) string {
	if !l.ok() {
		return ""
	}
	w, _ := l.SizeMM()
	px := int(w / mmPerPixel)
	alt := strings.NewReplacer(`"`, "", "<", "", ">", "", "\n", " ").Replace(l.Alt)
	if alt == "" {
		alt = "Logo"
	}
	src := "data:" + l.ContentType + ";base64," + base64.StdEncoding.EncodeToString(l.Data)
	if l.placement() == LogoLeft {
		return "![" + strings.NewReplacer("[", "", "]", "").Replace(alt) + "](" + src + ")"
	}
	return fmt.Sprintf(`<p align="%s"><img src="%s" alt="%s" width="%d"></p>`, l.placement(), src, alt, px)
}
