package report

import (
	"math"
	"slices"
	"sort"
)

// Layered ("Sugiyama-style") layout of a dependency graph, as the web
// client's graph view lays it out (web/src/lib/graphLayout.ts): only flow
// edges (dependencies, refinements, derivations) decide the layers, with
// prerequisites and parents first (left, or at the top when vertical);
// symmetric relations (relates to, conflicts with, duplicates) are drawn
// between the placed nodes. Cycles are broken by ignoring the edges that
// close them. The layout depends only on the order of its input.

// graphEdge is a relation "Source Type Target".
type graphEdge struct {
	Source, Target, Type string
}

// symmetricRelations have no direction: they do not order the nodes.
var symmetricRelations = []string{"relates_to", "conflicts_with", "duplicates"}

// flowOf is the upstream → downstream direction of an edge (ok false for
// symmetric relations): "A provides B" flows A → B; for everything else
// ("A depends on B", "A refines B", "A derived from B") B comes first.
func flowOf(e graphEdge) (from, to string, ok bool) {
	if slices.Contains(symmetricRelations, e.Type) {
		return "", "", false
	}
	if e.Type == "provides" {
		return e.Source, e.Target, true
	}
	return e.Target, e.Source, true
}

// layoutOpts are the sizes of a layout (points).
type layoutOpts struct {
	nodeW, nodeH float64
	gapX, gapY   float64
	vertical     bool
	sweeps       int
}

type placedNode struct {
	ID           string
	Layer, Order int
	X, Y         float64
}

type graphLayout struct {
	Nodes         []placedNode
	Flow          map[graphEdge]bool // edges that decide the layers
	Width, Height float64
	Layers        int
}

// layered lays out the nodes ids (in this order) with the edges between them.
func layered(ids []string, edges []graphEdge, o layoutOpts) graphLayout {
	if o.sweeps == 0 {
		o.sweeps = 6
	}
	known := map[string]bool{}
	var nodes []string
	for _, id := range ids {
		if !known[id] {
			known[id] = true
			nodes = append(nodes, id)
		}
	}
	var usable []graphEdge
	for _, e := range edges {
		if known[e.Source] && known[e.Target] && e.Source != e.Target {
			usable = append(usable, e)
		}
	}
	// flow adjacency without the edges that close a cycle (iterative DFS)
	allOut := map[string][]string{}
	type pair struct{ a, b string }
	var candidates []pair
	for _, e := range usable {
		if a, b, ok := flowOf(e); ok {
			candidates = append(candidates, pair{a, b})
			allOut[a] = append(allOut[a], b)
		}
	}
	state := map[string]int{} // 0 new, 1 on stack, 2 done
	accepted := map[pair]bool{}
	for _, start := range nodes {
		if state[start] != 0 {
			continue
		}
		type frame struct {
			n string
			i int
		}
		stack := []frame{{start, 0}}
		state[start] = 1
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			next := allOut[top.n]
			if top.i >= len(next) {
				state[top.n] = 2
				stack = stack[:len(stack)-1]
				continue
			}
			m := next[top.i]
			top.i++
			if state[m] == 1 {
				continue // closes a cycle
			}
			accepted[pair{top.n, m}] = true
			if state[m] == 0 {
				state[m] = 1
				stack = append(stack, frame{m, 0})
			}
		}
	}
	out := map[string][]string{}
	for _, c := range candidates {
		if accepted[c] && !slices.Contains(out[c.a], c.b) {
			out[c.a] = append(out[c.a], c.b)
		}
	}
	// longest-path layering (Kahn)
	indeg := map[string]int{}
	for _, id := range nodes {
		for _, m := range out[id] {
			indeg[m]++
		}
	}
	layer := map[string]int{}
	var queue []string
	for _, id := range nodes {
		if indeg[id] == 0 {
			queue = append(queue, id)
		}
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, m := range out[n] {
			layer[m] = max(layer[m], layer[n]+1)
			indeg[m]--
			if indeg[m] == 0 {
				queue = append(queue, m)
			}
		}
	}
	// order within layers: input order, then barycenter sweeps
	var layers [][]string
	for _, id := range nodes {
		l := layer[id]
		for len(layers) <= l {
			layers = append(layers, nil)
		}
		layers[l] = append(layers[l], id)
	}
	preds, succs := map[string][]string{}, map[string][]string{}
	for _, id := range nodes {
		for _, m := range out[id] {
			succs[id] = append(succs[id], m)
			preds[m] = append(preds[m], id)
		}
	}
	pos := map[string]float64{}
	index := func() {
		for _, ls := range layers {
			for i, id := range ls {
				pos[id] = float64(i)
			}
		}
	}
	index()
	bary := func(id string, nb map[string][]string) float64 {
		list := nb[id]
		if len(list) == 0 {
			return pos[id]
		}
		s := 0.0
		for _, x := range list {
			s += pos[x]
		}
		return s / float64(len(list))
	}
	for s := 0; s < o.sweeps; s++ {
		down := s%2 == 0
		var rng []int
		if down {
			for i := 1; i < len(layers); i++ {
				rng = append(rng, i)
			}
		} else {
			for i := len(layers) - 2; i >= 0; i-- {
				rng = append(rng, i)
			}
		}
		for _, l := range rng {
			nb := succs
			if down {
				nb = preds
			}
			type keyed struct {
				id   string
				b, p float64
			}
			ks := make([]keyed, len(layers[l]))
			for i, id := range layers[l] {
				ks[i] = keyed{id, bary(id, nb), pos[id]}
			}
			sort.SliceStable(ks, func(i, j int) bool {
				if ks[i].b != ks[j].b {
					return ks[i].b < ks[j].b
				}
				return ks[i].p < ks[j].p
			})
			for i, k := range ks {
				layers[l][i] = k.id
				pos[k.id] = float64(i)
			}
		}
	}
	// coordinates: layers are columns centred vertically, or rows centred horizontally
	widest := 1
	for _, ls := range layers {
		widest = max(widest, len(ls))
	}
	n := len(layers)
	W, H := o.nodeW, o.nodeH
	lay := graphLayout{Layers: n, Flow: map[graphEdge]bool{}}
	placed := map[string]placedNode{}
	if o.vertical {
		if n > 0 {
			lay.Width = float64(widest)*W + float64(widest-1)*o.gapX
			lay.Height = float64(n)*H + float64(n-1)*o.gapY
		}
		for l, ls := range layers {
			rowW := float64(len(ls))*W + float64(len(ls)-1)*o.gapX
			left := (lay.Width - rowW) / 2
			for i, id := range ls {
				placed[id] = placedNode{ID: id, Layer: l, Order: i, X: left + float64(i)*(W+o.gapX), Y: float64(l) * (H + o.gapY)}
			}
		}
	} else {
		if n > 0 {
			lay.Height = float64(widest)*H + float64(widest-1)*o.gapY
			lay.Width = float64(n)*W + float64(n-1)*o.gapX
		}
		for l, ls := range layers {
			colH := float64(len(ls))*H + float64(len(ls)-1)*o.gapY
			top := (lay.Height - colH) / 2
			for i, id := range ls {
				placed[id] = placedNode{ID: id, Layer: l, Order: i, X: float64(l) * (W + o.gapX), Y: top + float64(i)*(H+o.gapY)}
			}
		}
	}
	for _, id := range nodes {
		lay.Nodes = append(lay.Nodes, placed[id])
	}
	for _, e := range usable {
		if a, b, ok := flowOf(e); ok && accepted[pair{a, b}] {
			lay.Flow[e] = true
		}
	}
	return lay
}

// round1 rounds to a tenth (stable coordinates).
func round1(v float64) float64 { return math.Round(v*10) / 10 }
