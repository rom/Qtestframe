package report

import (
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
)

// The dependency graph appendix of a requirements specification
// (DocOptions.DependencyGraph): the relations between the requirements of
// the document (refines, depends on, provides, derived from, conflicts
// with, duplicates, relates to) drawn as layered graphs, as the web
// client's graph view draws them, followed by the same relations as a
// table ("P-REQ-1 depends on P-REQ-3") for readers who cannot see the
// figures. A large specification gets several figures: connected groups
// of requirements are packed into figures of at most graphFigureCap
// requirements, and a group larger than that is split by top-level folder
// (and then into parts), with the requirements at the other end of a
// relation into another figure drawn as references to that figure.

// graphFigureCap is the most requirements drawn in one figure.
var graphFigureCap = 24

// Geometry of the figures (points).
const (
	gNodeW     = 150.0
	gNodeH     = 34.0
	gPad       = 12.0
	gCompGap   = 26.0
	gMaxW      = 480.0 // content width of the page
	gMaxH      = 560.0 // page height left for the graph below caption and legend
	gLegendRow = 15.0
)

// relationStyles are the colour and dash pattern per relation type (as in
// the web client's graph view).
var relationStyles = map[string]struct {
	color string
	dash  []float64
}{
	"depends_on":     {"#2563eb", nil},
	"provides":       {"#059669", nil},
	"refines":        {"#7c3aed", nil},
	"derived_from":   {"#c2410c", nil},
	"conflicts_with": {"#dc2626", []float64{2, 2.5}},
	"duplicates":     {"#64748b", []float64{6, 2, 1.5, 2}},
	"relates_to":     {"#64748b", []float64{2.5, 3}},
}

// statusColors are the colours of the requirement statuses (as in the web
// client).
var statusColors = map[string]string{
	"draft": "#94a3b8", "review": "#f59e0b", "approved": "#3b82f6", "implemented": "#6366f1",
	"verified": "#22c55e", "rejected": "#ef4444", "obsolete": "#475569",
}

const (
	gInk      = "#1e293b"
	gMuted    = "#475569"
	gBorder   = "#94a3b8"
	gGhostBox = "#f1f5f9"
)

// relation is one relation of the specification: a requirement of the
// document, the type and the requirement at the other end (To nil when it
// is not in the document).
type relation struct {
	From   *model.Requirement
	Type   string
	ToID   string
	ToKey  string
	ToName string
	To     *model.Requirement
}

// relations are the relations from the requirements of the document, by
// source key, type and target key.
func (g *docGen) relations() []relation {
	var out []relation
	seen := map[string]bool{}
	for i := range g.d.Requirements {
		rq := &g.d.Requirements[i]
		for _, r := range rq.RelationRefs {
			if r.Kind != model.KindRequirement || r.Direction != "outgoing" || !slices.Contains(model.RelationTypes, r.Type) || r.ID == rq.ID {
				continue
			}
			k := rq.ID + "|" + r.Type + "|" + r.ID
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, relation{From: rq, Type: r.Type, ToID: r.ID, ToKey: r.Key, ToName: r.Title, To: g.reqByID[r.ID]})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.From.Key != b.From.Key {
			return naturalLess(a.From.Key, b.From.Key)
		}
		if a.Type != b.Type {
			return slices.Index(model.RelationTypes, a.Type) < slices.Index(model.RelationTypes, b.Type)
		}
		return naturalLess(a.ToKey, b.ToKey)
	})
	return out
}

func (g *docGen) hasDependencyGraph() bool {
	return g.o.DependencyGraph != nil && *g.o.DependencyGraph && len(g.relations()) > 0
}

// topFolder is the name of the top-level folder of a requirement ("" when
// it has none).
func (g *docGen) topFolder(rq *model.Requirement) string {
	byID := map[string]*model.Folder{}
	for i := range g.d.Folders {
		byID[g.d.Folders[i].ID] = &g.d.Folders[i]
	}
	if rq.FolderID == nil {
		return ""
	}
	f := byID[*rq.FolderID]
	for depth := 0; f != nil && f.ParentID != nil && depth < 64; depth++ {
		p := byID[*f.ParentID]
		if p == nil {
			break
		}
		f = p
	}
	if f == nil {
		return ""
	}
	return f.Name
}

// graphGroup is the requirements of one figure.
type graphGroup struct {
	ids []string
	// split: a part of a connected group too large for one figure, by
	// top-level folder (folder; "" for the requirements without one when
	// byFolder)
	split, byFolder bool
	folder          string
	part, parts     int // part of the folder (1-based; 0 when it is whole)
}

// components are the connected groups of ids (in their order) under edges.
func components(ids []string, edges []graphEdge) [][]string {
	adj := map[string][]string{}
	for _, e := range edges {
		adj[e.Source] = append(adj[e.Source], e.Target)
		adj[e.Target] = append(adj[e.Target], e.Source)
	}
	pos := map[string]int{}
	for i, id := range ids {
		pos[id] = i
	}
	seen := map[string]bool{}
	var out [][]string
	for _, id := range ids {
		if seen[id] {
			continue
		}
		var comp []string
		queue := []string{id}
		seen[id] = true
		for len(queue) > 0 {
			n := queue[0]
			queue = queue[1:]
			comp = append(comp, n)
			for _, m := range adj[n] {
				if _, ok := pos[m]; ok && !seen[m] {
					seen[m] = true
					queue = append(queue, m)
				}
			}
		}
		out = append(out, comp)
	}
	return out
}

// graphGroups divides the related requirements into figures.
func (g *docGen) graphGroups(ids []string, edges []graphEdge) []graphGroup {
	pos := map[string]int{}
	for i, id := range ids {
		pos[id] = i
	}
	byPos := func(s []string) {
		sort.SliceStable(s, func(i, j int) bool { return pos[s[i]] < pos[s[j]] })
	}
	var out []graphGroup
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			byPos(cur)
			out = append(out, graphGroup{ids: cur})
			cur = nil
		}
	}
	for _, comp := range components(ids, edges) {
		if len(comp) <= graphFigureCap {
			if len(cur)+len(comp) > graphFigureCap {
				flush()
			}
			cur = append(cur, comp...)
			continue
		}
		flush()
		// too large for one figure: by top-level folder, then in parts
		byPos(comp)
		folders := map[string][]string{}
		var order []string
		for _, id := range comp {
			f := g.topFolder(g.reqByID[id])
			if _, ok := folders[f]; !ok {
				order = append(order, f)
			}
			folders[f] = append(folders[f], id)
		}
		for _, f := range order {
			// parts follow the relations (breadth first) so that related requirements stay together
			var seq []string
			for _, c := range components(folders[f], edges) {
				seq = append(seq, c...)
			}
			parts := (len(seq) + graphFigureCap - 1) / graphFigureCap
			for p := 0; p < parts; p++ {
				chunk := append([]string{}, seq[p*graphFigureCap:min(len(seq), (p+1)*graphFigureCap)]...)
				byPos(chunk)
				gg := graphGroup{ids: chunk, split: true, byFolder: len(order) > 1 || f != "", folder: f}
				if parts > 1 {
					gg.part, gg.parts = p+1, parts
				}
				out = append(out, gg)
			}
		}
	}
	flush()
	return out
}

// dependencyGraphAppendix adds the appendix.
func (g *docGen) dependencyGraphAppendix() {
	rels := g.relations()
	if g.o.DependencyGraph == nil || !*g.o.DependencyGraph || len(rels) == 0 {
		return
	}
	l := g.l
	g.add(export.PageBreak())
	g.h(1, l.t("Dependency graph"))
	letter := string(rune('A' + g.num[0] - 1))

	// the requirements with relations inside the document, in key order
	var edges []graphEdge
	related := map[string]bool{}
	for _, r := range rels {
		if r.To == nil {
			continue
		}
		edges = append(edges, graphEdge{Source: r.From.ID, Target: r.ToID, Type: r.Type})
		related[r.From.ID], related[r.ToID] = true, true
	}
	var ids []string
	idx := make([]int, len(g.d.Requirements))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return naturalLess(g.d.Requirements[idx[a]].Key, g.d.Requirements[idx[b]].Key) })
	for _, i := range idx {
		if related[g.d.Requirements[i].ID] {
			ids = append(ids, g.d.Requirements[i].ID)
		}
	}
	groups := g.graphGroups(ids, edges)
	figOf := map[string]int{}
	for i, gr := range groups {
		for _, id := range gr.ids {
			figOf[id] = i + 1
		}
	}
	figName := func(n int) string { return letter + "." + strconv.Itoa(n) }

	intro := l.n("The figures show the relations between the {n} requirement of this specification that has relations.",
		"The figures show the relations between the {n} requirements of this specification that have relations.", len(ids))
	if len(ids) == 0 {
		intro = l.t("No two requirements of this specification are related; the relations to requirements outside it are listed below.")
	}
	intro += " " + l.t("Requirements that build on others are drawn to the right of (or below) what they build on. An arrow points from a requirement to the one it refines, depends on, provides for or is derived from; lines without an arrow are relations without a direction. The colour on the left of each box is the status of the requirement.")
	if len(groups) > 1 {
		intro += " " + g.f("The graph is divided into {n} figures of at most {max} requirements; a requirement related to one in another figure is drawn as a grey reference to that figure.",
			"n", strconv.Itoa(len(groups)), "max", strconv.Itoa(graphFigureCap))
	}
	without := len(g.d.Requirements) - len(ids)
	if without > 0 && len(ids) > 0 {
		intro += " " + l.n("{n} requirement without relations in this specification is not drawn.", "{n} requirements without relations in this specification are not drawn.", without)
	}
	intro += " " + g.f("Table {table} lists every relation as text.", "table", letter+".1")
	g.add(export.Paragraph(intro))

	for i, gr := range groups {
		title := l.t("Requirement relations")
		switch {
		case gr.split && gr.byFolder && gr.folder != "":
			title = g.f("Requirement relations in {folder}", "folder", gr.folder)
		case gr.split && gr.byFolder:
			title = l.t("Requirement relations without folder")
		}
		if gr.parts > 0 {
			title += " (" + g.f("part {n} of {total}", "n", strconv.Itoa(gr.part), "total", strconv.Itoa(gr.parts)) + ")"
		}
		caption := g.f("Figure {n}. {title}", "n", figName(i+1), "title", title)
		fig := g.graphFigure(gr.ids, edges, figOf, i+1, figName)
		fig.Title = caption
		g.add(export.RichParagraph(export.Span{Text: caption, Bold: true}), export.FigureBlock(fig))
	}

	// the relations as text
	g.add(export.RichParagraph(export.Span{Text: g.f("Table {n}. Relations between the requirements", "n", letter+".1"), Bold: true}))
	cols := []string{l.t("Requirement"), l.t("Relation"), l.t("Related requirement"), l.t("Figure")}
	rows := make([][]string, 0, len(rels))
	for _, r := range rels {
		other := strings.TrimSpace(r.ToKey + " " + r.ToName)
		fig := ""
		if r.To == nil {
			other += " (" + l.t("not in this specification") + ")"
		} else {
			other = strings.TrimSpace(r.To.Key + " " + r.To.Title)
			if a, b := figOf[r.From.ID], figOf[r.ToID]; a == b {
				fig = figName(a)
			} else {
				fig = figName(a) + ", " + figName(b)
			}
		}
		rows = append(rows, []string{strings.TrimSpace(r.From.Key + " " + r.From.Title), g.enum(EnumRelation, r.Type), other, fig})
	}
	g.add(export.Table(cols, rows, 3, 1.6, 3, 1))
}

// graphNode is a box of a figure: a requirement of the figure, or a
// reference to another figure (to one requirement there, or to all the
// related requirements there: refs of them).
type graphNode struct {
	id    string
	rq    *model.Requirement
	ghost int // the figure the requirement is in (0: it is in this one)
	refs  int // a reference to refs requirements of figure ghost
}

// graphMaxRefs is the most references to single requirements of other
// figures that a figure draws; with more, it draws one reference per
// other figure.
const graphMaxRefs = 8

// graphFigure draws the requirements ids (figure n) with the edges between
// them and references to the related requirements of other figures.
func (g *docGen) graphFigure(ids []string, all []graphEdge, figOf map[string]int, n int, figName func(int) string) *export.Figure {
	in := map[string]bool{}
	for _, id := range ids {
		in[id] = true
	}
	nodes := map[string]*graphNode{}
	order := append([]string{}, ids...)
	for _, id := range ids {
		nodes[id] = &graphNode{id: id, rq: g.reqByID[id]}
	}
	outside := map[string]bool{}
	for _, e := range all {
		if in[e.Source] != in[e.Target] {
			outside[e.Source], outside[e.Target] = !in[e.Source], !in[e.Target]
		}
	}
	nOutside := 0
	for _, o := range outside {
		if o {
			nOutside++
		}
	}
	perFigure := nOutside > graphMaxRefs
	refOf := func(id string) string {
		if perFigure {
			return "fig:" + strconv.Itoa(figOf[id])
		}
		return "ref:" + id
	}
	var edges []graphEdge
	seen := map[graphEdge]bool{}
	types := map[string]bool{}
	refsTo := map[string]map[string]bool{}
	for _, e := range all {
		if !in[e.Source] && !in[e.Target] {
			continue
		}
		ge := e
		for _, end := range []*string{&ge.Source, &ge.Target} {
			if in[*end] {
				continue
			}
			gid := refOf(*end)
			if nodes[gid] == nil {
				nodes[gid] = &graphNode{id: gid, rq: g.reqByID[*end], ghost: figOf[*end]}
				refsTo[gid] = map[string]bool{}
				order = append(order, gid)
			}
			refsTo[gid][*end] = true
			*end = gid
		}
		if seen[ge] {
			continue
		}
		seen[ge] = true
		edges = append(edges, ge)
		types[e.Type] = true
	}
	if perFigure {
		for gid, set := range refsTo {
			nodes[gid].refs, nodes[gid].rq = len(set), nil
		}
		// the references in figure order, after the requirements
		sort.SliceStable(order[len(ids):], func(i, j int) bool { return nodes[order[len(ids)+i]].ghost < nodes[order[len(ids)+j]].ghost })
	}

	// lay out every connected group, both ways; keep the one that needs less shrinking
	comps := components(order, edges)
	best := struct {
		boxes     []compBox
		w, h, fit float64
		vertical  bool
	}{fit: -1}
	for _, vertical := range []bool{false, true} {
		o := layoutOpts{nodeW: gNodeW, nodeH: gNodeH, gapX: 56, gapY: 14, vertical: vertical}
		if vertical {
			o.gapX, o.gapY = 14, 30
		}
		var boxes []compBox
		for _, c := range comps {
			var ce []graphEdge
			cs := map[string]bool{}
			for _, id := range c {
				cs[id] = true
			}
			for _, e := range edges {
				if cs[e.Source] && cs[e.Target] {
					ce = append(ce, e)
				}
			}
			boxes = append(boxes, compBox{lay: layered(c, ce, o), edges: ce})
		}
		w, h := shelfPack(boxes)
		fit := min(1, gMaxW/w, gMaxH/h)
		if fit > best.fit+1e-9 {
			best.boxes, best.w, best.h, best.fit, best.vertical = boxes, w, h, fit, vertical
		}
	}

	fig := &export.Figure{}
	width := math.Max(best.w, 380) + 2*gPad
	legendY := best.h + 2*gPad
	// edges below the boxes
	for _, b := range best.boxes {
		pos := map[string]placedNode{}
		for _, p := range b.lay.Nodes {
			p.X, p.Y = round1(p.X+b.x+gPad+(width-2*gPad-best.w)/2), round1(p.Y+b.y+gPad)
			pos[p.ID] = p
		}
		for _, e := range b.edges {
			fig.Shapes = append(fig.Shapes, edgeShapes(e, pos, best.vertical, g.relationTip(e, nodes, figName))...)
		}
		for _, id := range sortedIDs(pos) {
			fig.Shapes = append(fig.Shapes, g.nodeShapes(nodes[id], pos[id], figName)...)
		}
	}
	legend, lh := g.legend(types, statusesOf(nodes), width, legendY)
	fig.Shapes = append(fig.Shapes, legend...)
	fig.Width, fig.Height = round1(width), round1(legendY+lh+gPad/2)

	var keys []string
	for _, id := range ids {
		keys = append(keys, nodes[id].rq.Key)
	}
	fig.Desc = g.f("Diagram of the relations between {n} requirements: {keys}. Table {table} lists the relations as text.",
		"n", strconv.Itoa(len(ids)), "keys", strings.Join(keys, ", "), "table", strings.SplitN(figName(n), ".", 2)[0]+".1")
	return fig
}

func sortedIDs(m map[string]placedNode) []string {
	out := make([]string, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := m[out[i]], m[out[j]]
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		if a.X != b.X {
			return a.X < b.X
		}
		return out[i] < out[j]
	})
	return out
}

// compBox is the layout of one connected group at its place in the figure.
type compBox struct {
	lay   graphLayout
	edges []graphEdge
	x, y  float64
}

// shelfPack places the boxes in rows (in their order), trying row widths
// for the one whose result fits the page best; it returns the size.
func shelfPack(boxes []compBox) (w, h float64) {
	widest, total := 0.0, 0.0
	for _, b := range boxes {
		widest = max(widest, b.lay.Width)
		total += b.lay.Width + gCompGap
	}
	place := func(limit float64, apply bool) (float64, float64) {
		x, y, rowH, maxW := 0.0, 0.0, 0.0, 0.0
		for i := range boxes {
			b := &boxes[i]
			if x > 0 && x+b.lay.Width > limit+0.01 {
				x, y, rowH = 0, y+rowH+gCompGap, 0
			}
			if apply {
				b.x, b.y = x, y
			}
			maxW = max(maxW, x+b.lay.Width)
			rowH = max(rowH, b.lay.Height)
			x += b.lay.Width + gCompGap
		}
		return maxW, y + rowH
	}
	bestLimit, bestFit := widest, -1.0
	for limit := widest; ; limit += widest / 4 {
		w, h := place(limit, false)
		if fit := min(1, gMaxW/w, gMaxH/h); fit > bestFit+1e-9 {
			bestFit, bestLimit = fit, limit
		}
		if limit >= total || widest == 0 {
			break
		}
	}
	return place(bestLimit, true)
}

func statusesOf(nodes map[string]*graphNode) map[string]bool {
	out := map[string]bool{}
	for _, nd := range nodes {
		if nd.ghost == 0 && nd.rq != nil {
			out[nd.rq.Status] = true
		}
	}
	return out
}

func (g *docGen) relationTip(e graphEdge, nodes map[string]*graphNode, figName func(int) string) string {
	key := func(id string) string {
		if nd := nodes[id]; nd != nil && nd.rq != nil {
			return nd.rq.Key
		} else if nd != nil && nd.ghost > 0 {
			return g.f("Figure {n}", "n", figName(nd.ghost))
		}
		return ""
	}
	return key(e.Source) + " " + g.enum(EnumRelation, e.Type) + " " + key(e.Target)
}

// edgeShapes draws an edge: a curve between the boxes (from the upstream
// box to the downstream one, as the graph view) with an arrowhead at the
// target of a directed relation.
func edgeShapes(e graphEdge, pos map[string]placedNode, vertical bool, tip string) []export.Shape {
	a, b := pos[e.Source], pos[e.Target]
	swapped := false
	if from, _, ok := flowOf(e); ok && from == e.Target {
		a, b, swapped = b, a, true // draw upstream → downstream
	}
	var start, c1, c2, end export.Point
	W, H := gNodeW, gNodeH
	if !vertical {
		if a.X == b.X {
			x := a.X + W
			y1, y2 := a.Y+H/2, b.Y+H/2
			bow := 30 + math.Min(60, math.Abs(y2-y1)/4)
			start, c1, c2, end = export.Point{X: x, Y: y1}, export.Point{X: x + bow, Y: y1}, export.Point{X: x + bow, Y: y2}, export.Point{X: x, Y: y2}
		} else {
			x1, x2 := a.X+W, b.X
			if a.X > b.X {
				x1, x2 = a.X, b.X+W
			}
			y1, y2 := a.Y+H/2, b.Y+H/2
			dx := (x2 - x1) / 2
			start, c1, c2, end = export.Point{X: x1, Y: y1}, export.Point{X: x1 + dx, Y: y1}, export.Point{X: x2 - dx, Y: y2}, export.Point{X: x2, Y: y2}
		}
	} else {
		if a.Y == b.Y {
			y := a.Y + H
			x1, x2 := a.X+W/2, b.X+W/2
			bow := 24 + math.Min(48, math.Abs(x2-x1)/4)
			start, c1, c2, end = export.Point{X: x1, Y: y}, export.Point{X: x1, Y: y + bow}, export.Point{X: x2, Y: y + bow}, export.Point{X: x2, Y: y}
		} else {
			y1, y2 := a.Y+H, b.Y
			if a.Y > b.Y {
				y1, y2 = a.Y, b.Y+H
			}
			x1, x2 := a.X+W/2, b.X+W/2
			dy := (y2 - y1) / 2
			start, c1, c2, end = export.Point{X: x1, Y: y1}, export.Point{X: x1, Y: y1 + dy}, export.Point{X: x2, Y: y2 - dy}, export.Point{X: x2, Y: y2}
		}
	}
	st, ok := relationStyles[e.Type]
	if !ok {
		st.color, st.dash = gMuted, []float64{3, 3}
	}
	out := []export.Shape{{Kind: export.ShapePath, Start: start, Path: []export.PathSeg{{Cubic: true, C1: c1, C2: c2, P: end}},
		Stroke: st.color, StrokeWidth: 1.4, Dash: st.dash, Tip: tip}}
	_, _, directed := flowOf(e)
	if directed {
		// the arrowhead at the target's end
		at, from := end, c2
		if swapped { // the target is where the curve starts
			at, from = start, c1
		}
		out = append(out, arrowHead(at, from, st.color))
	}
	return out
}

// arrowHead is a filled triangle at tip pointing away from from.
func arrowHead(tip, from export.Point, color string) export.Shape {
	dx, dy := tip.X-from.X, tip.Y-from.Y
	l := math.Hypot(dx, dy)
	if l == 0 {
		dx, dy, l = 1, 0, 1
	}
	ux, uy := dx/l, dy/l
	const length, half = 7.0, 3.2
	bx, by := tip.X-ux*length, tip.Y-uy*length
	return export.Shape{Kind: export.ShapePolygon, Fill: color, Points: []export.Point{
		{X: round1(tip.X), Y: round1(tip.Y)},
		{X: round1(bx - uy*half), Y: round1(by + ux*half)},
		{X: round1(bx + uy*half), Y: round1(by - ux*half)},
	}}
}

// nodeShapes draws a box: the key and the status on the first line, the
// shortened title on the second, the status colour on the left; a
// reference to another figure is grey with a dashed border.
func (g *docGen) nodeShapes(nd *graphNode, p placedNode, figName func(int) string) []export.Shape {
	x, y := p.X, p.Y
	rq := nd.rq
	key, title := "", ""
	if rq != nil {
		key, title = rq.Key, rq.Title
	}
	tip := strings.TrimSpace(key + " " + title)
	if nd.ghost > 0 && nd.refs > 0 {
		label := g.f("see Figure {n}", "n", figName(nd.ghost))
		return []export.Shape{
			{Kind: export.ShapeRect, X: x, Y: y, W: gNodeW, H: gNodeH, Radius: 4, Fill: gGhostBox, Stroke: gBorder, StrokeWidth: 0.8, Dash: []float64{3, 2}, Tip: label},
			{Kind: export.ShapeText, X: x + 8, Y: y + 13.5, Text: export.FitText(label, 8.5, true, gNodeW-16), Size: 8.5, Bold: true, Fill: gMuted},
			{Kind: export.ShapeText, X: x + 8, Y: y + 26, Text: export.FitText(g.l.n("{n} related requirement", "{n} related requirements", nd.refs), 7.5, false, gNodeW-16), Size: 7.5, Fill: gMuted},
		}
	}
	if nd.ghost > 0 {
		return []export.Shape{
			{Kind: export.ShapeRect, X: x, Y: y, W: gNodeW, H: gNodeH, Radius: 4, Fill: gGhostBox, Stroke: gBorder, StrokeWidth: 0.8, Dash: []float64{3, 2}, Tip: tip},
			{Kind: export.ShapeText, X: x + 8, Y: y + 13.5, Text: export.FitText(key, 8.5, true, gNodeW-16), Size: 8.5, Bold: true, Fill: gMuted},
			{Kind: export.ShapeText, X: x + 8, Y: y + 26, Text: export.FitText(g.f("see Figure {n}", "n", figName(nd.ghost)), 7.5, false, gNodeW-16), Size: 7.5, Fill: gMuted},
		}
	}
	status := ""
	if rq != nil {
		status = rq.Status
	}
	color, ok := statusColors[status]
	if !ok {
		color = gBorder
	}
	label := export.FitText(g.enum(EnumStatus, status), 6.5, false, 52)
	keyW := gNodeW - 18 - export.TextWidth(label, 6.5, false) - 6
	return []export.Shape{
		{Kind: export.ShapeRect, X: x, Y: y, W: gNodeW, H: gNodeH, Radius: 4, Fill: "#ffffff", Stroke: gBorder, StrokeWidth: 0.8, Tip: tip},
		{Kind: export.ShapeRect, X: x, Y: y, W: 5, H: gNodeH, Radius: 2, Fill: color},
		{Kind: export.ShapeText, X: x + 10, Y: y + 13.5, Text: export.FitText(key, 8.5, true, keyW), Size: 8.5, Bold: true, Fill: gInk},
		{Kind: export.ShapeText, X: x + gNodeW - 6, Y: y + 13.5, Text: label, Size: 6.5, Fill: gMuted, Anchor: "end"},
		{Kind: export.ShapeText, X: x + 10, Y: y + 26, Text: export.FitText(title, 7.5, false, gNodeW-16), Size: 7.5, Fill: gInk},
	}
}

// legend draws the relation types and statuses of a figure from y, in
// rows of at most width; it returns the shapes and their height.
func (g *docGen) legend(types, statuses map[string]bool, width, y float64) ([]export.Shape, float64) {
	var out []export.Shape
	x0 := gPad
	x, row := x0, 0
	const size = 7.5
	place := func(w float64) (float64, float64) {
		if x > x0 && x+w > width-gPad {
			x, row = x0, row+1
		}
		px, py := x, y+float64(row)*gLegendRow
		x += w + 14
		return px, py
	}
	heading := func(text string) {
		w := export.TextWidth(text, size, true)
		px, py := place(w)
		out = append(out, export.Shape{Kind: export.ShapeText, X: px, Y: py + 9, Text: text, Size: size, Bold: true, Fill: gInk})
		x -= 8
	}
	heading(g.l.t("Relations") + ":")
	for _, t := range model.RelationTypes {
		if !types[t] {
			continue
		}
		st := relationStyles[t]
		label := g.enum(EnumRelation, t)
		w := 26 + export.TextWidth(label, size, false)
		px, py := place(w)
		ly := py + 6.5
		out = append(out, export.Shape{Kind: export.ShapePath, Start: export.Point{X: px, Y: ly}, Path: []export.PathSeg{{P: export.Point{X: px + 20, Y: ly}}},
			Stroke: st.color, StrokeWidth: 1.4, Dash: st.dash})
		if _, _, directed := flowOf(graphEdge{Type: t}); directed {
			out = append(out, arrowHead(export.Point{X: px + 22, Y: ly}, export.Point{X: px, Y: ly}, st.color))
		}
		out = append(out, export.Shape{Kind: export.ShapeText, X: px + 26, Y: py + 9, Text: label, Size: size, Fill: gInk})
	}
	x, row = x0, row+1
	heading(g.l.t("Status") + ":")
	for _, s := range model.RequirementStatuses {
		if !statuses[s] {
			continue
		}
		label := g.enum(EnumStatus, s)
		w := 14 + export.TextWidth(label, size, false)
		px, py := place(w)
		out = append(out, export.Shape{Kind: export.ShapeRect, X: px, Y: py + 2, W: 9, H: 9, Radius: 2, Fill: statusColors[s]},
			export.Shape{Kind: export.ShapeText, X: px + 13, Y: py + 9, Text: label, Size: size, Fill: gInk})
	}
	return out, float64(row+1) * gLegendRow
}
