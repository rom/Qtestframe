package export

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"path"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// Logo images: what an upload becomes before it is stored. PNG and JPEG
// are decoded and encoded again (no metadata, no interlacing, 8 bits per
// channel, at most LogoMaxSide pixels on the longer side, which also
// suits the PDF library); an SVG is drawn by RasterizeSVG into a PNG of
// LogoSVGSide pixels on the longer side, so that nothing in it is ever
// interpreted later.

// Logo image limits.
const (
	// MaxLogoBytes is the largest logo upload.
	MaxLogoBytes = 2 << 20
	// LogoMaxSide is the longest side of a stored PNG or JPEG logo.
	LogoMaxSide = 1600
	// LogoSVGSide is the longer side of a rasterised SVG logo.
	LogoSVGSide = 1000
	// maxLogoPixels bounds the decoded size of an upload.
	maxLogoPixels = 40_000_000
	minLogoSide   = 8
)

// ErrLogo marks logo uploads that cannot be used; the message is meant for
// the user.
var ErrLogo = errors.New("logo")

func logoErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrLogo, fmt.Sprintf(format, a...))
}

// PreparedLogo is a logo image ready to be stored.
type PreparedLogo struct {
	Data          []byte
	ContentType   string
	Width, Height int
	// Source is the uploaded type: png, jpeg or svg.
	Source string
}

// IsSVG reports whether an upload is an SVG image (by its name or its
// content).
func IsSVG(name string, data []byte) bool {
	if strings.EqualFold(path.Ext(name), ".svg") {
		return true
	}
	head := data
	if len(head) > 2048 {
		head = head[:2048]
	}
	ct := http.DetectContentType(data)
	return (strings.HasPrefix(ct, "text/xml") || strings.HasPrefix(ct, "text/plain")) && bytes.Contains(bytes.ToLower(head), []byte("<svg"))
}

// PrepareLogo checks and normalises an uploaded logo (PNG, JPEG or SVG).
func PrepareLogo(name string, data []byte) (*PreparedLogo, error) {
	if len(data) == 0 {
		return nil, logoErr("the file is empty")
	}
	if len(data) > MaxLogoBytes {
		return nil, logoErr("the logo is larger than %d MB", MaxLogoBytes>>20)
	}
	if IsSVG(name, data) {
		img, err := RasterizeSVG(data, LogoSVGSide)
		if err != nil {
			return nil, logoErr("%s", strings.TrimPrefix(err.Error(), "svg: "))
		}
		if opaqueEmpty(img) {
			return nil, logoErr("nothing in the SVG could be drawn (convert texts to outlines; embedded images are not drawn)")
		}
		return encodeLogo(img, "svg")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") {
		return nil, logoErr("the logo must be a PNG, JPEG or SVG image")
	}
	if cfg.Width < minLogoSide || cfg.Height < minLogoSide {
		return nil, logoErr("the logo must be at least %d × %d pixels", minLogoSide, minLogoSide)
	}
	if cfg.Width*cfg.Height > maxLogoPixels {
		return nil, logoErr("the logo has too many pixels (%d × %d)", cfg.Width, cfg.Height)
	}
	if cfg.Width/cfg.Height > 50 || cfg.Height/cfg.Width > 50 {
		return nil, logoErr("the logo's aspect ratio is too extreme")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, logoErr("the image could not be read (%v)", err)
	}
	return encodeLogo(img, format)
}

// encodeLogo scales img down to LogoMaxSide and encodes it: JPEG sources as
// JPEG, the others as 8-bit PNG.
func encodeLogo(img image.Image, source string) (*PreparedLogo, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if s := max(w, h); s > LogoMaxSide {
		w, h = max(1, w*LogoMaxSide/s), max(1, h*LogoMaxSide/s)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	if w == b.Dx() && h == b.Dy() {
		xdraw.Draw(dst, dst.Bounds(), img, b.Min, xdraw.Src)
	} else {
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
	}
	var buf bytes.Buffer
	out := &PreparedLogo{Width: w, Height: h, Source: source}
	if source == "jpeg" {
		// JPEG has no transparency: flatten onto white
		rgb := image.NewRGBA(dst.Bounds())
		xdraw.Draw(rgb, rgb.Bounds(), image.NewUniform(color.White), image.Point{}, xdraw.Src)
		xdraw.Draw(rgb, rgb.Bounds(), dst, image.Point{}, xdraw.Over)
		if err := jpeg.Encode(&buf, rgb, &jpeg.Options{Quality: 90}); err != nil {
			return nil, err
		}
		out.ContentType = "image/jpeg"
	} else {
		if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, dst); err != nil {
			return nil, err
		}
		out.ContentType = "image/png"
	}
	out.Data = buf.Bytes()
	return out, nil
}

// opaqueEmpty reports whether nothing was drawn on img.
func opaqueEmpty(img *image.RGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0 {
			return false
		}
	}
	return true
}
