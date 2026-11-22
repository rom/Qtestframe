package export

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// The PNG rendering of a figure (the fallback image of DOCX for word
// processors that do not read its SVG): pure Go, with the Go fonts. Text
// is narrowed where the Go font is wider than Helvetica, so that it keeps
// within the boxes laid out with TextWidth.

var (
	fontsOnce        sync.Once
	fontReg, fontBld *opentype.Font
)

func pngFonts() {
	fontReg, _ = opentype.Parse(goregular.TTF)
	fontBld, _ = opentype.Parse(gobold.TTF)
}

// PNG renders the figure at scale pixels per point.
func (f *Figure) PNG(scale float64) ([]byte, error) {
	fontsOnce.Do(pngFonts)
	w, h := max(1, int(math.Ceil(f.Width*scale))), max(1, int(math.Ceil(f.Height*scale)))
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	r := &raster{dst: dst, k: scale, z: vector.NewRasterizer(w, h), faces: map[faceKey]font.Face{}}
	defer r.close()
	for _, s := range f.Shapes {
		r.shape(s)
	}
	var b bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&b, dst); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

type faceKey struct {
	size float64
	bold bool
}

type raster struct {
	dst   *image.RGBA
	k     float64
	z     *vector.Rasterizer
	faces map[faceKey]font.Face
}

func (r *raster) close() {
	for _, f := range r.faces {
		_ = f.Close()
	}
}

func colorOf(c string) (color.Color, bool) {
	rr, g, b, ok := rgb(c)
	if !ok {
		return nil, false
	}
	return color.RGBA{uint8(rr), uint8(g), uint8(b), 0xff}, true
}

// fill fills the polygons (non-zero winding; inner rings in the opposite
// direction make holes), rasterising only their bounding box.
func (r *raster) fill(c color.Color, rings ...[]Point) {
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, ring := range rings {
		for _, p := range ring {
			minX, minY = min(minX, p.X*r.k), min(minY, p.Y*r.k)
			maxX, maxY = max(maxX, p.X*r.k), max(maxY, p.Y*r.k)
		}
	}
	box := image.Rect(int(math.Floor(minX)), int(math.Floor(minY)), int(math.Ceil(maxX))+1, int(math.Ceil(maxY))+1).Intersect(r.dst.Bounds())
	if box.Empty() {
		return
	}
	ox, oy := float64(box.Min.X), float64(box.Min.Y)
	r.z.Reset(box.Dx(), box.Dy())
	for _, ring := range rings {
		if len(ring) < 3 {
			continue
		}
		r.z.MoveTo(float32(ring[0].X*r.k-ox), float32(ring[0].Y*r.k-oy))
		for _, p := range ring[1:] {
			r.z.LineTo(float32(p.X*r.k-ox), float32(p.Y*r.k-oy))
		}
		r.z.ClosePath()
	}
	r.z.Draw(r.dst, box, image.NewUniform(c), image.Point{})
}

// roundRect is the outline of a rounded rectangle (clockwise).
func roundRect(x, y, w, h, rad float64) []Point {
	rad = max(0, min(rad, w/2, h/2))
	if rad == 0 {
		return []Point{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}}
	}
	var out []Point
	corner := func(cx, cy, from float64) {
		for i := 0; i <= 6; i++ {
			a := from + float64(i)*math.Pi/12
			out = append(out, Point{cx + rad*math.Cos(a), cy + rad*math.Sin(a)})
		}
	}
	corner(x+w-rad, y+rad, -math.Pi/2)
	corner(x+w-rad, y+h-rad, 0)
	corner(x+rad, y+h-rad, math.Pi/2)
	corner(x+rad, y+rad, math.Pi)
	return out
}

func reverse(ps []Point) []Point {
	out := make([]Point, len(ps))
	for i, p := range ps {
		out[len(ps)-1-i] = p
	}
	return out
}

// flatten turns a path into a polyline.
func flatten(s Shape) []Point {
	pts := []Point{s.Start}
	cur := s.Start
	for _, sg := range s.Path {
		if !sg.Cubic {
			pts = append(pts, sg.P)
			cur = sg.P
			continue
		}
		const n = 20
		for i := 1; i <= n; i++ {
			t := float64(i) / n
			u := 1 - t
			pts = append(pts, Point{
				u*u*u*cur.X + 3*u*u*t*sg.C1.X + 3*u*t*t*sg.C2.X + t*t*t*sg.P.X,
				u*u*u*cur.Y + 3*u*u*t*sg.C1.Y + 3*u*t*t*sg.C2.Y + t*t*t*sg.P.Y,
			})
		}
		cur = sg.P
	}
	return pts
}

// dashes splits a polyline into its dashes (the whole line without a dash
// pattern).
func dashes(pts []Point, dash []float64) [][]Point {
	total := 0.0
	for _, d := range dash {
		total += d
	}
	if len(dash) == 0 || total <= 0 {
		return [][]Point{pts}
	}
	var out [][]Point
	idx, left, on := 0, dash[0], true
	cur := []Point{pts[0]}
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		seg := math.Hypot(b.X-a.X, b.Y-a.Y)
		pos := 0.0
		for seg-pos > left {
			pos += left
			t := pos / seg
			p := Point{a.X + (b.X-a.X)*t, a.Y + (b.Y-a.Y)*t}
			if on {
				out = append(out, append(cur, p))
			}
			cur = []Point{p}
			on = !on
			idx = (idx + 1) % len(dash)
			left = dash[idx]
		}
		left -= seg - pos
		cur = append(cur, b)
	}
	if on && len(cur) > 1 {
		out = append(out, cur)
	}
	return out
}

// stroke draws a polyline of width w: a quadrilateral per segment and a
// square at every joint.
func (r *raster) stroke(c color.Color, pts []Point, w float64) {
	var rings [][]Point
	hw := w / 2
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		dx, dy := b.X-a.X, b.Y-a.Y
		l := math.Hypot(dx, dy)
		if l == 0 {
			continue
		}
		nx, ny := -dy/l*hw, dx/l*hw
		ring := []Point{{a.X + nx, a.Y + ny}, {b.X + nx, b.Y + ny}, {b.X - nx, b.Y - ny}, {a.X - nx, a.Y - ny}}
		rings = append(rings, ring)
		if i > 1 {
			rings = append(rings, []Point{{a.X - hw*0.7, a.Y - hw*0.7}, {a.X + hw*0.7, a.Y - hw*0.7}, {a.X + hw*0.7, a.Y + hw*0.7}, {a.X - hw*0.7, a.Y + hw*0.7}})
		}
	}
	// one by one: overlapping rings in opposite directions would cancel
	for _, ring := range rings {
		r.fill(c, ring)
	}
}

func (r *raster) face(size float64, bold bool) font.Face {
	key := faceKey{math.Round(size*r.k*4) / 4, bold}
	if f, ok := r.faces[key]; ok {
		return f
	}
	src := fontReg
	if bold {
		src = fontBld
	}
	f, err := opentype.NewFace(src, &opentype.FaceOptions{Size: key.size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil
	}
	r.faces[key] = f
	return f
}

func (r *raster) text(s Shape) {
	c, ok := colorOf(s.Fill)
	if !ok {
		c = color.Black
	}
	size := s.Size
	face := r.face(size, s.Bold)
	if face == nil || s.Text == "" {
		return
	}
	// as wide as Helvetica at most (the layout measured it so)
	want := TextWidth(s.Text, s.Size, s.Bold) * r.k
	got := float64(font.MeasureString(face, s.Text)) / 64
	if got > want*1.02 && got > 0 {
		size *= want / got
		face = r.face(size, s.Bold)
		got = float64(font.MeasureString(face, s.Text)) / 64
	}
	x := s.X * r.k
	switch s.Anchor {
	case "middle":
		x -= got / 2
	case "end":
		x -= got
	}
	d := font.Drawer{Dst: r.dst, Src: image.NewUniform(c), Face: face, Dot: fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.Int26_6(s.Y * r.k * 64)}}
	d.DrawString(s.Text)
}

func (r *raster) shape(s Shape) {
	fc, fill := colorOf(s.Fill)
	sc, stroke := colorOf(s.Stroke)
	stroke = stroke && s.StrokeWidth > 0
	switch s.Kind {
	case ShapeRect:
		outer := roundRect(s.X, s.Y, s.W, s.H, s.Radius)
		if fill {
			r.fill(fc, outer)
		}
		if stroke {
			hw := s.StrokeWidth / 2
			if len(s.Dash) > 0 {
				pts := append(append([]Point{}, outer...), outer[0])
				for _, d := range dashes(pts, s.Dash) {
					r.stroke(sc, d, s.StrokeWidth)
				}
				return
			}
			o := roundRect(s.X-hw, s.Y-hw, s.W+2*hw, s.H+2*hw, s.Radius+hw)
			in := roundRect(s.X+hw, s.Y+hw, s.W-2*hw, s.H-2*hw, max(0, s.Radius-hw))
			r.fill(sc, o, reverse(in))
		}
	case ShapePath:
		if stroke {
			for _, d := range dashes(flatten(s), s.Dash) {
				r.stroke(sc, d, s.StrokeWidth)
			}
		}
	case ShapePolygon:
		if fill {
			r.fill(fc, s.Points)
		}
		if stroke {
			r.stroke(sc, append(append([]Point{}, s.Points...), s.Points[0]), s.StrokeWidth)
		}
	case ShapeText:
		r.text(s)
	}
}
