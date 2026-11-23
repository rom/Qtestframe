package export

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"strconv"
	"strings"

	"golang.org/x/image/vector"
)

// A small, safe SVG rasteriser for logotypes. The SVG is parsed as plain
// XML and drawn by this code: nothing in it is executed or fetched.
// Scripts, event handlers, foreign objects, embedded or linked images,
// <use> references, fonts and text, filters, masks and clip paths are
// ignored; so are external style sheets and entity declarations (the XML
// decoder does not expand them). Drawn are paths, rectangles (with rounded
// corners), circles, ellipses, lines, polylines and polygons with their
// fill and stroke colours, opacities, stroke widths and transforms; a
// gradient paints with its first stop colour. Styles come from
// presentation attributes, simple <style> rules (element, .class and #id
// selectors) and style attributes. Text should be converted to outlines
// before the logo is uploaded.

// SVG input limits.
const (
	MaxSVGBytes    = 2 << 20
	maxSVGElements = 20000
	maxSVGDepth    = 64
	maxSVGPoints   = 2_000_000
)

// ErrSVG marks SVG files that cannot be drawn.
var ErrSVG = errors.New("svg")

func svgErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrSVG, fmt.Sprintf(format, a...))
}

type svgNode struct {
	name     string
	attrs    map[string]string
	children []*svgNode
	text     string
}

// parseSVGTree reads the elements of an SVG document.
func parseSVGTree(data []byte) (*svgNode, error) {
	if len(data) > MaxSVGBytes {
		return nil, svgErr("the SVG is larger than %d MB", MaxSVGBytes>>20)
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	dec.Entity = map[string]string{} // no entities beyond the XML ones
	var root *svgNode
	var stack []*svgNode
	count := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, svgErr("not a valid SVG file (%v)", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			count++
			if count > maxSVGElements {
				return nil, svgErr("the SVG has more than %d elements", maxSVGElements)
			}
			if len(stack) >= maxSVGDepth {
				return nil, svgErr("the SVG is nested deeper than %d levels", maxSVGDepth)
			}
			n := &svgNode{name: strings.ToLower(t.Name.Local), attrs: map[string]string{}}
			for _, a := range t.Attr {
				if a.Name.Space != "" && a.Name.Space != "http://www.w3.org/2000/svg" {
					continue // xlink:href, xml:space, foreign namespaces
				}
				n.attrs[strings.ToLower(a.Name.Local)] = a.Value
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, svgErr("more than one root element")
				}
				if n.name != "svg" {
					return nil, svgErr("the root element is <%s>, not <svg>", n.name)
				}
				root = n
			} else {
				p := stack[len(stack)-1]
				p.children = append(p.children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 && stack[len(stack)-1].name == "style" && len(stack[len(stack)-1].text) < 256<<10 {
				stack[len(stack)-1].text += string(t)
			}
		case xml.Directive:
			if bytes.Contains(bytes.ToUpper(t), []byte("ENTITY")) {
				return nil, svgErr("entity declarations are not allowed")
			}
		}
	}
	if root == nil {
		return nil, svgErr("no <svg> element")
	}
	return root, nil
}

// ---------------------------------------------------------------- styles

type srPaint struct {
	set, none bool
	c         color.NRGBA
	current   bool // currentColor
}

type srStyle struct {
	fill, stroke     srPaint
	color            color.NRGBA
	fillOp, strokeOp float64
	opacity          float64
	strokeWidth      float64
	hidden           bool
}

func defaultSrStyle() srStyle {
	black := color.NRGBA{0, 0, 0, 255}
	return srStyle{fill: srPaint{set: true, c: black}, stroke: srPaint{set: true, none: true}, color: black,
		fillOp: 1, strokeOp: 1, opacity: 1, strokeWidth: 1}
}

type cssRule struct {
	sel   string // "rect", ".cls-1", "#logo"
	decls map[string]string
}

func parseDecls(s string) map[string]string {
	out := map[string]string{}
	for _, d := range strings.Split(s, ";") {
		k, v, ok := strings.Cut(d, ":")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "!important"))
		if k != "" {
			out[k] = strings.TrimSpace(v)
		}
	}
	return out
}

// parseCSS reads the simple rules of a <style> element.
func parseCSS(s string) []cssRule {
	for {
		i := strings.Index(s, "/*")
		if i < 0 {
			break
		}
		j := strings.Index(s[i+2:], "*/")
		if j < 0 {
			s = s[:i]
			break
		}
		s = s[:i] + s[i+2+j+2:]
	}
	var out []cssRule
	for _, block := range strings.Split(s, "}") {
		sel, body, ok := strings.Cut(block, "{")
		if !ok || strings.Contains(sel, "@") {
			continue
		}
		decls := parseDecls(body)
		for _, one := range strings.Split(sel, ",") {
			one = strings.TrimSpace(one)
			if one == "" || strings.ContainsAny(one, " >+~:[") {
				continue
			}
			out = append(out, cssRule{sel: one, decls: decls})
		}
	}
	return out
}

func (r cssRule) matches(n *svgNode) bool {
	switch {
	case strings.HasPrefix(r.sel, "."):
		for _, c := range strings.Fields(n.attrs["class"]) {
			if "."+c == r.sel {
				return true
			}
		}
		return false
	case strings.HasPrefix(r.sel, "#"):
		return n.attrs["id"] != "" && "#"+n.attrs["id"] == r.sel
	}
	return strings.EqualFold(r.sel, n.name) || r.sel == "*"
}

var svgColorNames = map[string]color.NRGBA{
	"black": {0, 0, 0, 255}, "white": {255, 255, 255, 255}, "red": {255, 0, 0, 255}, "green": {0, 128, 0, 255},
	"blue": {0, 0, 255, 255}, "yellow": {255, 255, 0, 255}, "orange": {255, 165, 0, 255}, "purple": {128, 0, 128, 255},
	"gray": {128, 128, 128, 255}, "grey": {128, 128, 128, 255}, "silver": {192, 192, 192, 255}, "navy": {0, 0, 128, 255},
	"teal": {0, 128, 128, 255}, "maroon": {128, 0, 0, 255}, "olive": {128, 128, 0, 255}, "lime": {0, 255, 0, 255},
	"aqua": {0, 255, 255, 255}, "cyan": {0, 255, 255, 255}, "fuchsia": {255, 0, 255, 255}, "magenta": {255, 0, 255, 255},
	"darkblue": {0, 0, 139, 255}, "darkred": {139, 0, 0, 255}, "darkgreen": {0, 100, 0, 255}, "darkgray": {169, 169, 169, 255},
	"darkgrey": {169, 169, 169, 255}, "lightgray": {211, 211, 211, 255}, "lightgrey": {211, 211, 211, 255},
	"gold": {255, 215, 0, 255}, "pink": {255, 192, 203, 255}, "brown": {165, 42, 42, 255}, "steelblue": {70, 130, 180, 255},
	"royalblue": {65, 105, 225, 255}, "crimson": {220, 20, 60, 255}, "dodgerblue": {30, 144, 255, 255},
	"midnightblue": {25, 25, 112, 255}, "slategray": {112, 128, 144, 255}, "dimgray": {105, 105, 105, 255},
	"whitesmoke": {245, 245, 245, 255}, "skyblue": {135, 206, 235, 255}, "indigo": {75, 0, 130, 255},
	"tomato": {255, 99, 71, 255}, "coral": {255, 127, 80, 255}, "forestgreen": {34, 139, 34, 255},
}

// parseColor reads a CSS colour (#rgb, #rrggbb, #rgba, #rrggbbaa, rgb(),
// rgba(), a few names).
func parseColor(s string) (color.NRGBA, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if c, ok := svgColorNames[s]; ok {
		return c, true
	}
	if h, ok := strings.CutPrefix(s, "#"); ok {
		hex := func(x string) (uint8, bool) {
			v, err := strconv.ParseUint(x, 16, 8)
			return uint8(v), err == nil
		}
		switch len(h) {
		case 3, 4:
			var c [4]uint8
			c[3] = 255
			for i := 0; i < len(h); i++ {
				v, ok := hex(h[i:i+1] + h[i:i+1])
				if !ok {
					return color.NRGBA{}, false
				}
				c[i] = v
			}
			return color.NRGBA{c[0], c[1], c[2], c[3]}, true
		case 6, 8:
			var c [4]uint8
			c[3] = 255
			for i := 0; i < len(h)/2; i++ {
				v, ok := hex(h[2*i : 2*i+2])
				if !ok {
					return color.NRGBA{}, false
				}
				c[i] = v
			}
			return color.NRGBA{c[0], c[1], c[2], c[3]}, true
		}
		return color.NRGBA{}, false
	}
	if args, ok := strings.CutPrefix(s, "rgba("); ok {
		s = "rgb(" + args
	}
	if args, ok := strings.CutPrefix(s, "rgb("); ok {
		args = strings.TrimSuffix(args, ")")
		parts := strings.FieldsFunc(args, func(r rune) bool { return r == ',' || r == ' ' || r == '/' })
		if len(parts) < 3 {
			return color.NRGBA{}, false
		}
		var c [4]uint8
		c[3] = 255
		for i := 0; i < len(parts) && i < 4; i++ {
			p := parts[i]
			pct := strings.HasSuffix(p, "%")
			v, err := strconv.ParseFloat(strings.TrimSuffix(p, "%"), 64)
			if err != nil {
				return color.NRGBA{}, false
			}
			switch {
			case i == 3 && !pct:
				v *= 255
			case pct:
				v = v * 255 / 100
			}
			c[i] = uint8(math.Round(min(max(v, 0), 255)))
		}
		return color.NRGBA{c[0], c[1], c[2], c[3]}, true
	}
	return color.NRGBA{}, false
}

type svgRaster struct {
	dst       *image.RGBA
	z         vector.Rasterizer
	css       []cssRule
	gradients map[string]color.NRGBA
	points    int
	vw, vh    float64 // the viewport in user units (for percentages)
}

func (r *svgRaster) paint(v string, cur srPaint) srPaint {
	v = strings.TrimSpace(v)
	switch strings.ToLower(v) {
	case "":
		return cur
	case "none", "transparent":
		return srPaint{set: true, none: true}
	case "currentcolor":
		return srPaint{set: true, current: true}
	case "inherit":
		return cur
	}
	if strings.HasPrefix(strings.ToLower(v), "url(") {
		end := strings.Index(v, ")")
		if end < 0 {
			return cur
		}
		id := strings.Trim(strings.TrimSpace(v[4:end]), `"'`)
		if c, ok := r.gradients[strings.TrimPrefix(id, "#")]; ok {
			return srPaint{set: true, c: c}
		}
		if fb := strings.TrimSpace(v[end+1:]); fb != "" {
			return r.paint(fb, cur)
		}
		return srPaint{set: true, none: true}
	}
	if c, ok := parseColor(v); ok {
		return srPaint{set: true, c: c}
	}
	return cur
}

func parseOpacity(v string, cur float64) float64 {
	v = strings.TrimSpace(v)
	pct := strings.HasSuffix(v, "%")
	f, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
	if err != nil {
		return cur
	}
	if pct {
		f /= 100
	}
	return min(max(f, 0), 1)
}

// declarations returns the style properties of n: presentation
// attributes, then the <style> rules, then the style attribute.
func (r *svgRaster) declarations(n *svgNode) map[string]string {
	out := map[string]string{}
	for _, k := range []string{"fill", "stroke", "fill-opacity", "stroke-opacity", "opacity", "stroke-width", "display", "visibility", "color"} {
		if v, ok := n.attrs[k]; ok {
			out[k] = v
		}
	}
	for _, rule := range r.css {
		if rule.matches(n) {
			for k, v := range rule.decls {
				out[k] = v
			}
		}
	}
	for k, v := range parseDecls(n.attrs["style"]) {
		out[k] = v
	}
	return out
}

func (r *svgRaster) style(n *svgNode, parent srStyle) srStyle {
	st := parent
	st.opacity = 1 // multiplied below
	d := r.declarations(n)
	if v, ok := d["color"]; ok {
		if c, ok := parseColor(v); ok {
			st.color = c
		}
	}
	st.fill = r.paint(d["fill"], st.fill)
	st.stroke = r.paint(d["stroke"], st.stroke)
	if v, ok := d["fill-opacity"]; ok {
		st.fillOp = parseOpacity(v, st.fillOp)
	}
	if v, ok := d["stroke-opacity"]; ok {
		st.strokeOp = parseOpacity(v, st.strokeOp)
	}
	if v, ok := d["opacity"]; ok {
		st.opacity = parseOpacity(v, 1)
	}
	st.opacity *= parent.opacity
	if v, ok := d["stroke-width"]; ok {
		if w, ok := r.length(v, r.vw); ok && w >= 0 {
			st.strokeWidth = w
		}
	}
	if v := strings.TrimSpace(d["display"]); v == "none" {
		st.hidden = true
	}
	if v := strings.TrimSpace(d["visibility"]); v == "hidden" || v == "collapse" {
		st.hidden = true
	}
	return st
}

// length reads a length in user units (px; mm, cm, in, pt and pc are
// converted; % of ref).
func (r *svgRaster) length(v string, ref float64) (float64, bool) {
	return svgLength(v, ref)
}

func svgLength(v string, ref float64) (float64, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	k := 1.0
	for _, u := range []struct {
		s string
		k float64
	}{{"px", 1}, {"mm", 96 / 25.4}, {"cm", 96 / 2.54}, {"in", 96}, {"pt", 96.0 / 72}, {"pc", 16}, {"em", 16}, {"%", -1}} {
		if strings.HasSuffix(v, u.s) {
			v, k = strings.TrimSpace(strings.TrimSuffix(v, u.s)), u.k
			break
		}
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	if k < 0 {
		return f * ref / 100, true
	}
	return f * k, true
}

// ---------------------------------------------------------------- transforms

// srMatrix maps (x, y) to (a x + c y + e, b x + d y + f).
type srMatrix [6]float64

var srIdentity = srMatrix{1, 0, 0, 1, 0, 0}

func (m srMatrix) mul(n srMatrix) srMatrix { // m · n (n applied first)
	return srMatrix{
		m[0]*n[0] + m[2]*n[1], m[1]*n[0] + m[3]*n[1],
		m[0]*n[2] + m[2]*n[3], m[1]*n[2] + m[3]*n[3],
		m[0]*n[4] + m[2]*n[5] + m[4], m[1]*n[4] + m[3]*n[5] + m[5],
	}
}

func (m srMatrix) apply(x, y float64) Point {
	return Point{m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]}
}

func (m srMatrix) scale() float64 { return math.Sqrt(math.Abs(m[0]*m[3] - m[1]*m[2])) }

func parseTransform(s string) srMatrix {
	m := srIdentity
	for {
		s = strings.TrimLeft(s, " \t\r\n,")
		open := strings.Index(s, "(")
		if open < 0 {
			return m
		}
		end := strings.Index(s[open:], ")")
		if end < 0 {
			return m
		}
		name := strings.ToLower(strings.TrimSpace(s[:open]))
		args := numbers(s[open+1 : open+end])
		s = s[open+end+1:]
		arg := func(i int, def float64) float64 {
			if i < len(args) {
				return args[i]
			}
			return def
		}
		var t srMatrix
		switch name {
		case "matrix":
			if len(args) < 6 {
				continue
			}
			t = srMatrix{args[0], args[1], args[2], args[3], args[4], args[5]}
		case "translate":
			t = srMatrix{1, 0, 0, 1, arg(0, 0), arg(1, 0)}
		case "scale":
			sx := arg(0, 1)
			t = srMatrix{sx, 0, 0, arg(1, sx), 0, 0}
		case "rotate":
			a := arg(0, 0) * math.Pi / 180
			cx, cy := arg(1, 0), arg(2, 0)
			cos, sin := math.Cos(a), math.Sin(a)
			t = srMatrix{1, 0, 0, 1, cx, cy}.mul(srMatrix{cos, sin, -sin, cos, 0, 0}).mul(srMatrix{1, 0, 0, 1, -cx, -cy})
		case "skewx":
			t = srMatrix{1, 0, math.Tan(arg(0, 0) * math.Pi / 180), 1, 0, 0}
		case "skewy":
			t = srMatrix{1, math.Tan(arg(0, 0) * math.Pi / 180), 0, 1, 0, 0}
		default:
			continue
		}
		m = m.mul(t)
	}
}

// numbers reads the numbers of a list ("1,2 3-4.5.5e2").
func numbers(s string) []float64 {
	var out []float64
	sc := numScanner{s: s}
	for {
		v, ok := sc.number()
		if !ok {
			return out
		}
		out = append(out, v)
	}
}

type numScanner struct {
	s string
	i int
}

func (sc *numScanner) skip() {
	for sc.i < len(sc.s) && strings.IndexByte(" \t\r\n,", sc.s[sc.i]) >= 0 {
		sc.i++
	}
}

func (sc *numScanner) number() (float64, bool) {
	sc.skip()
	start := sc.i
	j := sc.i
	if j < len(sc.s) && (sc.s[j] == '+' || sc.s[j] == '-') {
		j++
	}
	digits, dot := false, false
	for j < len(sc.s) {
		c := sc.s[j]
		switch {
		case c >= '0' && c <= '9':
			digits = true
		case c == '.' && !dot:
			dot = true
		case (c == 'e' || c == 'E') && digits:
			k := j + 1
			if k < len(sc.s) && (sc.s[k] == '+' || sc.s[k] == '-') {
				k++
			}
			if k < len(sc.s) && sc.s[k] >= '0' && sc.s[k] <= '9' {
				j = k
				for j < len(sc.s) && sc.s[j] >= '0' && sc.s[j] <= '9' {
					j++
				}
			}
			goto done
		default:
			goto done
		}
		j++
	}
done:
	if !digits {
		sc.i = start
		return 0, false
	}
	v, err := strconv.ParseFloat(sc.s[start:j], 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		sc.i = start
		return 0, false
	}
	sc.i = j
	return v, true
}

// flag reads an arc flag (0 or 1, possibly without a separator).
func (sc *numScanner) flag() (bool, bool) {
	sc.skip()
	if sc.i < len(sc.s) && (sc.s[sc.i] == '0' || sc.s[sc.i] == '1') {
		sc.i++
		return sc.s[sc.i-1] == '1', true
	}
	return false, false
}

// ---------------------------------------------------------------- geometry

// svgPath is a shape in device pixels: closed and open subpaths.
type svgPath struct {
	subs   [][]Point
	closed []bool
}

type pathBuilder struct {
	m      srMatrix
	p      svgPath
	cur    []Point
	tol    float64
	budget *int
}

func (b *pathBuilder) point(x, y float64) {
	if *b.budget <= 0 {
		return
	}
	*b.budget--
	b.cur = append(b.cur, b.m.apply(x, y))
}

func (b *pathBuilder) end(closed bool) {
	if len(b.cur) > 1 {
		b.p.subs = append(b.p.subs, b.cur)
		b.p.closed = append(b.p.closed, closed)
	}
	b.cur = nil
}

// cubic adds the curve from (x0, y0) to (x3, y3) (after the current
// point) as line segments.
func (b *pathBuilder) cubic(x0, y0, x1, y1, x2, y2, x3, y3 float64) {
	p0, p1, p2, p3 := b.m.apply(x0, y0), b.m.apply(x1, y1), b.m.apply(x2, y2), b.m.apply(x3, y3)
	l := math.Hypot(p1.X-p0.X, p1.Y-p0.Y) + math.Hypot(p2.X-p1.X, p2.Y-p1.Y) + math.Hypot(p3.X-p2.X, p3.Y-p2.Y)
	n := int(min(max(math.Ceil(l/b.tol), 2), 64))
	for i := 1; i <= n; i++ {
		t := float64(i) / float64(n)
		u := 1 - t
		x := u*u*u*x0 + 3*u*u*t*x1 + 3*u*t*t*x2 + t*t*t*x3
		y := u*u*u*y0 + 3*u*u*t*y1 + 3*u*t*t*y2 + t*t*t*y3
		b.point(x, y)
	}
}

// arc adds an elliptical arc (SVG endpoint parameterisation) from (x1, y1)
// to (x2, y2).
func (b *pathBuilder) arc(x1, y1, rx, ry, phiDeg float64, large, sweep bool, x2, y2 float64) {
	if rx == 0 || ry == 0 || (x1 == x2 && y1 == y2) {
		b.point(x2, y2)
		return
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	phi := phiDeg * math.Pi / 180
	cos, sin := math.Cos(phi), math.Sin(phi)
	dx, dy := (x1-x2)/2, (y1-y2)/2
	xp, yp := cos*dx+sin*dy, -sin*dx+cos*dy
	lam := xp*xp/(rx*rx) + yp*yp/(ry*ry)
	if lam > 1 {
		s := math.Sqrt(lam)
		rx, ry = rx*s, ry*s
	}
	num := rx*rx*ry*ry - rx*rx*yp*yp - ry*ry*xp*xp
	den := rx*rx*yp*yp + ry*ry*xp*xp
	k := 0.0
	if den > 0 && num > 0 {
		k = math.Sqrt(num / den)
	}
	if large == sweep {
		k = -k
	}
	cxp, cyp := k*rx*yp/ry, -k*ry*xp/rx
	cx, cy := cos*cxp-sin*cyp+(x1+x2)/2, sin*cxp+cos*cyp+(y1+y2)/2
	ang := func(ux, uy, vx, vy float64) float64 {
		a := math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy)
		return a
	}
	t1 := ang(1, 0, (xp-cxp)/rx, (yp-cyp)/ry)
	dt := ang((xp-cxp)/rx, (yp-cyp)/ry, (-xp-cxp)/rx, (-yp-cyp)/ry)
	if !sweep && dt > 0 {
		dt -= 2 * math.Pi
	} else if sweep && dt < 0 {
		dt += 2 * math.Pi
	}
	span := math.Abs(dt) * max(rx, ry) * b.m.scale()
	n := int(min(max(math.Ceil(span/b.tol), 4), 128))
	for i := 1; i <= n; i++ {
		t := t1 + dt*float64(i)/float64(n)
		ex, ey := rx*math.Cos(t), ry*math.Sin(t)
		b.point(cos*ex-sin*ey+cx, sin*ex+cos*ey+cy)
	}
}

// pathData turns SVG path data into subpaths.
func (b *pathBuilder) pathData(d string) {
	sc := numScanner{s: d}
	var cmd byte
	var x, y, sx, sy float64 // current and subpath start
	var cx2, cy2 float64     // last control point (S, T)
	var prev byte            // previous command
	num := func() (float64, bool) { return sc.number() }
	for {
		sc.skip()
		if sc.i >= len(sc.s) {
			break
		}
		c := sc.s[sc.i]
		if strings.IndexByte("MmLlHhVvCcSsQqTtAaZz", c) >= 0 {
			cmd = c
			sc.i++
		} else if cmd == 0 {
			return
		}
		rel := cmd >= 'a'
		ox, oy := 0.0, 0.0
		if rel {
			ox, oy = x, y
		}
		switch cmd {
		case 'Z', 'z':
			if len(b.cur) > 0 {
				b.end(true)
			}
			x, y = sx, sy
			prev = 'Z'
			continue
		case 'M', 'm':
			px, ok1 := num()
			py, ok2 := num()
			if !ok1 || !ok2 {
				return
			}
			b.end(false)
			x, y = ox+px, oy+py
			sx, sy = x, y
			b.point(x, y)
			if cmd == 'M' {
				cmd = 'L'
			} else {
				cmd = 'l'
			}
			prev = 'M'
			continue
		}
		if len(b.cur) == 0 {
			b.point(x, y)
		}
		switch cmd {
		case 'L', 'l':
			px, ok1 := num()
			py, ok2 := num()
			if !ok1 || !ok2 {
				return
			}
			x, y = ox+px, oy+py
			b.point(x, y)
		case 'H', 'h':
			px, ok := num()
			if !ok {
				return
			}
			x = ox + px
			b.point(x, y)
		case 'V', 'v':
			py, ok := num()
			if !ok {
				return
			}
			y = oy + py
			b.point(x, y)
		case 'C', 'c', 'S', 's':
			var x1, y1 float64
			if cmd == 'C' || cmd == 'c' {
				a, ok1 := num()
				bb, ok2 := num()
				if !ok1 || !ok2 {
					return
				}
				x1, y1 = ox+a, oy+bb
			} else if strings.IndexByte("CcSs", prev) >= 0 {
				x1, y1 = 2*x-cx2, 2*y-cy2
			} else {
				x1, y1 = x, y
			}
			v := make([]float64, 4)
			for i := range v {
				f, ok := num()
				if !ok {
					return
				}
				v[i] = f
			}
			x2, y2, x3, y3 := ox+v[0], oy+v[1], ox+v[2], oy+v[3]
			b.cubic(x, y, x1, y1, x2, y2, x3, y3)
			cx2, cy2 = x2, y2
			x, y = x3, y3
		case 'Q', 'q', 'T', 't':
			var qx, qy float64
			if cmd == 'Q' || cmd == 'q' {
				a, ok1 := num()
				bb, ok2 := num()
				if !ok1 || !ok2 {
					return
				}
				qx, qy = ox+a, oy+bb
			} else if strings.IndexByte("QqTt", prev) >= 0 {
				qx, qy = 2*x-cx2, 2*y-cy2
			} else {
				qx, qy = x, y
			}
			a, ok1 := num()
			bb, ok2 := num()
			if !ok1 || !ok2 {
				return
			}
			ex, ey := ox+a, oy+bb
			b.cubic(x, y, x+2.0/3*(qx-x), y+2.0/3*(qy-y), ex+2.0/3*(qx-ex), ey+2.0/3*(qy-ey), ex, ey)
			cx2, cy2 = qx, qy
			x, y = ex, ey
		case 'A', 'a':
			rx, ok1 := num()
			ry, ok2 := num()
			rot, ok3 := num()
			large, ok4 := sc.flag()
			sweep, ok5 := sc.flag()
			px, ok6 := num()
			py, ok7 := num()
			if !(ok1 && ok2 && ok3 && ok4 && ok5 && ok6 && ok7) {
				return
			}
			ex, ey := ox+px, oy+py
			b.arc(x, y, rx, ry, rot, large, sweep, ex, ey)
			x, y = ex, ey
		}
		prev = cmd
	}
	b.end(false)
}

// ellipse adds a closed ellipse.
func (b *pathBuilder) ellipse(cx, cy, rx, ry float64) {
	if rx <= 0 || ry <= 0 {
		return
	}
	span := 2 * math.Pi * max(rx, ry) * b.m.scale()
	n := int(min(max(math.Ceil(span/b.tol), 12), 256))
	for i := 0; i < n; i++ {
		t := 2 * math.Pi * float64(i) / float64(n)
		b.point(cx+rx*math.Cos(t), cy+ry*math.Sin(t))
	}
	b.end(true)
}

func (b *pathBuilder) rect(x, y, w, h, rx, ry float64) {
	if w <= 0 || h <= 0 {
		return
	}
	if rx <= 0 && ry > 0 {
		rx = ry
	}
	if ry <= 0 && rx > 0 {
		ry = rx
	}
	rx, ry = min(rx, w/2), min(ry, h/2)
	if rx <= 0 {
		b.point(x, y)
		b.point(x+w, y)
		b.point(x+w, y+h)
		b.point(x, y+h)
		b.end(true)
		return
	}
	corner := func(cx, cy, from float64) {
		for i := 0; i <= 8; i++ {
			a := from + float64(i)*math.Pi/16
			b.point(cx+rx*math.Cos(a), cy+ry*math.Sin(a))
		}
	}
	corner(x+w-rx, y+ry, -math.Pi/2)
	corner(x+w-rx, y+h-ry, 0)
	corner(x+rx, y+h-ry, math.Pi/2)
	corner(x+rx, y+ry, math.Pi)
	b.end(true)
}

// ---------------------------------------------------------------- drawing

func ringArea(ps []Point) float64 {
	a := 0.0
	for i := range ps {
		j := (i + 1) % len(ps)
		a += ps[i].X*ps[j].Y - ps[j].X*ps[i].Y
	}
	return a / 2
}

// fillRings fills rings with colour c (rings of opposite direction cancel:
// non-zero winding).
func (r *svgRaster) fillRings(c color.NRGBA, rings [][]Point) {
	if c.A == 0 || len(rings) == 0 {
		return
	}
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, ring := range rings {
		for _, p := range ring {
			minX, minY, maxX, maxY = min(minX, p.X), min(minY, p.Y), max(maxX, p.X), max(maxY, p.Y)
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
		r.z.MoveTo(float32(ring[0].X-ox), float32(ring[0].Y-oy))
		for _, p := range ring[1:] {
			r.z.LineTo(float32(p.X-ox), float32(p.Y-oy))
		}
		r.z.ClosePath()
	}
	r.z.Draw(r.dst, box, image.NewUniform(c), image.Point{})
}

// strokeRings outlines the subpaths with a line of width w: a quadrilateral
// per segment and a polygon at every joint, all turning the same way so
// that their overlaps do not cancel.
func strokeRings(p svgPath, w float64) [][]Point {
	hw := w / 2
	var out [][]Point
	add := func(ring []Point) {
		if ringArea(ring) < 0 {
			for i, j := 0, len(ring)-1; i < j; i, j = i+1, j-1 {
				ring[i], ring[j] = ring[j], ring[i]
			}
		}
		out = append(out, ring)
	}
	joint := func(c Point) {
		n := 8
		ring := make([]Point, n)
		for i := range ring {
			a := 2 * math.Pi * float64(i) / float64(n)
			ring[i] = Point{c.X + hw*math.Cos(a), c.Y + hw*math.Sin(a)}
		}
		add(ring)
	}
	for si, pts := range p.subs {
		if p.closed[si] && len(pts) > 1 {
			pts = append(append([]Point{}, pts...), pts[0])
		}
		for i := 1; i < len(pts); i++ {
			a, b := pts[i-1], pts[i]
			dx, dy := b.X-a.X, b.Y-a.Y
			l := math.Hypot(dx, dy)
			if l == 0 {
				continue
			}
			nx, ny := -dy/l*hw, dx/l*hw
			add([]Point{{a.X + nx, a.Y + ny}, {b.X + nx, b.Y + ny}, {b.X - nx, b.Y - ny}, {a.X - nx, a.Y - ny}})
			if i < len(pts)-1 || p.closed[si] {
				joint(b)
			}
		}
	}
	return out
}

func withAlpha(c color.NRGBA, k float64) color.NRGBA {
	c.A = uint8(math.Round(float64(c.A) * min(max(k, 0), 1)))
	return c
}

func (r *svgRaster) resolve(p srPaint, st srStyle) (color.NRGBA, bool) {
	if !p.set || p.none {
		return color.NRGBA{}, false
	}
	if p.current {
		return st.color, true
	}
	return p.c, true
}

func (r *svgRaster) draw(p svgPath, st srStyle, m srMatrix) {
	if c, ok := r.resolve(st.fill, st); ok {
		var rings [][]Point
		for _, s := range p.subs {
			if len(s) >= 3 {
				rings = append(rings, s)
			}
		}
		r.fillRings(withAlpha(c, st.fillOp*st.opacity), rings)
	}
	if c, ok := r.resolve(st.stroke, st); ok && st.strokeWidth > 0 {
		w := max(st.strokeWidth*m.scale(), 0.5)
		r.fillRings(withAlpha(c, st.strokeOp*st.opacity), strokeRings(p, w))
	}
}

func (r *svgRaster) num(n *svgNode, key string, ref float64) float64 {
	v, _ := r.length(n.attrs[key], ref)
	return v
}

// walk draws n and its children.
func (r *svgRaster) walk(n *svgNode, parent srStyle, pm srMatrix, root bool) {
	switch n.name {
	case "defs", "symbol", "clippath", "mask", "pattern", "marker", "title", "desc", "metadata", "style", "script",
		"foreignobject", "image", "text", "use", "filter", "lineargradient", "radialgradient", "switch", "a:link":
		return
	}
	st := r.style(n, parent)
	if st.hidden {
		return
	}
	m := pm
	if t, ok := n.attrs["transform"]; ok && !root {
		m = pm.mul(parseTransform(t))
	}
	b := &pathBuilder{m: m, tol: 0.75, budget: &r.points}
	switch n.name {
	case "svg", "g", "a":
		for _, c := range n.children {
			r.walk(c, st, m, false)
		}
		return
	case "path":
		b.pathData(n.attrs["d"])
	case "rect":
		b.rect(r.num(n, "x", r.vw), r.num(n, "y", r.vh), r.num(n, "width", r.vw), r.num(n, "height", r.vh), r.num(n, "rx", r.vw), r.num(n, "ry", r.vh))
	case "circle":
		rad := r.num(n, "r", math.Hypot(r.vw, r.vh)/math.Sqrt2)
		b.ellipse(r.num(n, "cx", r.vw), r.num(n, "cy", r.vh), rad, rad)
	case "ellipse":
		b.ellipse(r.num(n, "cx", r.vw), r.num(n, "cy", r.vh), r.num(n, "rx", r.vw), r.num(n, "ry", r.vh))
	case "line":
		b.point(r.num(n, "x1", r.vw), r.num(n, "y1", r.vh))
		b.point(r.num(n, "x2", r.vw), r.num(n, "y2", r.vh))
		b.end(false)
		st.fill = srPaint{set: true, none: true}
	case "polyline", "polygon":
		v := numbers(n.attrs["points"])
		for i := 0; i+1 < len(v); i += 2 {
			b.point(v[i], v[i+1])
		}
		b.end(n.name == "polygon")
	default:
		return
	}
	r.draw(b.p, st, m)
}

// collect finds the gradients (first stop colour) and style rules.
func (r *svgRaster) collect(n *svgNode) {
	switch n.name {
	case "style":
		r.css = append(r.css, parseCSS(n.text)...)
	case "lineargradient", "radialgradient":
		if id := n.attrs["id"]; id != "" {
			for _, s := range n.children {
				if s.name != "stop" {
					continue
				}
				d := parseDecls(s.attrs["style"])
				cv := s.attrs["stop-color"]
				if v, ok := d["stop-color"]; ok {
					cv = v
				}
				c, ok := parseColor(cv)
				if !ok {
					c = color.NRGBA{0, 0, 0, 255}
				}
				op := s.attrs["stop-opacity"]
				if v, ok := d["stop-opacity"]; ok {
					op = v
				}
				if op != "" {
					c = withAlpha(c, parseOpacity(op, 1))
				}
				r.gradients[id] = c
				break
			}
		}
	}
	for _, c := range n.children {
		r.collect(c)
	}
}

// svgSize returns the viewBox (or 0 0 width height) of the root and its
// aspect ratio.
func svgSize(root *svgNode) (minX, minY, vw, vh float64, err error) {
	if vb := numbers(root.attrs["viewbox"]); len(vb) == 4 && vb[2] > 0 && vb[3] > 0 {
		return vb[0], vb[1], vb[2], vb[3], nil
	}
	w, okW := svgLength(root.attrs["width"], 0)
	h, okH := svgLength(root.attrs["height"], 0)
	if okW && okH && w > 0 && h > 0 {
		return 0, 0, w, h, nil
	}
	return 0, 0, 0, 0, svgErr("the SVG has neither a viewBox nor a width and height")
}

// RasterizeSVG draws an SVG document on a transparent image whose longer
// side is maxSide pixels.
func RasterizeSVG(data []byte, maxSide int) (*image.RGBA, error) {
	root, err := parseSVGTree(data)
	if err != nil {
		return nil, err
	}
	minX, minY, vw, vh, err := svgSize(root)
	if err != nil {
		return nil, err
	}
	if vw/vh > 50 || vh/vw > 50 {
		return nil, svgErr("the SVG's aspect ratio is too extreme")
	}
	k := float64(maxSide) / max(vw, vh)
	w, h := max(1, int(math.Round(vw*k))), max(1, int(math.Round(vh*k)))
	r := &svgRaster{dst: image.NewRGBA(image.Rect(0, 0, w, h)), gradients: map[string]color.NRGBA{}, points: maxSVGPoints, vw: vw, vh: vh}
	r.collect(root)
	m := srMatrix{k, 0, 0, k, -minX * k, -minY * k}
	r.walk(root, defaultSrStyle(), m, true)
	if r.points <= 0 {
		return nil, svgErr("the SVG is too complex")
	}
	return r.dst, nil
}
