// Layered ("Sugiyama-style") layout of a dependency graph, left to right:
// prerequisites and parents on the left, what builds on them to the right.
// Only "flow" edges (dependencies, refinements, derivations, calls) decide
// the layers; symmetric relations (relates to, conflicts with, duplicates)
// are drawn between the placed nodes. Cycles are broken by ignoring the
// edges that close them.

export interface GraphEdgeIn {
  id: string
  source: string
  target: string
  type: string
}

export interface LayoutOptions {
  nodeWidth?: number
  nodeHeight?: number
  gapX?: number
  gapY?: number
  /** barycenter sweeps to reduce crossings */
  sweeps?: number
}

export interface PlacedNode {
  id: string
  layer: number
  order: number
  x: number
  y: number
}

export interface PlacedEdge extends GraphEdgeIn {
  /** SVG path from source box to target box */
  path: string
  /** true when the edge decides the layers (drawn solid with an arrow) */
  flow: boolean
}

export interface Layout {
  nodes: PlacedNode[]
  edges: PlacedEdge[]
  width: number
  height: number
  layers: number
}

/** Relation types without a direction: they do not order the nodes. */
export const SYMMETRIC = new Set(['relates_to', 'conflicts_with', 'duplicates'])

/**
 * The upstream → downstream direction of an edge, or null for symmetric
 * relations. "A provides B" flows A → B; for everything else ("A depends on
 * B", "A refines B", "A derived from B", "A calls B") B comes first.
 */
export function flow(e: Pick<GraphEdgeIn, 'source' | 'target' | 'type'>): [string, string] | null {
  if (SYMMETRIC.has(e.type)) return null
  return e.type === 'provides' ? [e.source, e.target] : [e.target, e.source]
}

export function layoutGraph(nodeIds: readonly string[], edges: readonly GraphEdgeIn[], opts: LayoutOptions = {}): Layout {
  const W = opts.nodeWidth ?? 200
  const H = opts.nodeHeight ?? 52
  const GX = opts.gapX ?? 90
  const GY = opts.gapY ?? 22
  const sweeps = opts.sweeps ?? 6
  const ids = [...new Set(nodeIds)]
  const known = new Set(ids)
  const usable = edges.filter((e) => known.has(e.source) && known.has(e.target) && e.source !== e.target)

  // flow adjacency, without the edges that close a cycle
  const out = new Map<string, string[]>(ids.map((id) => [id, []]))
  const candidates: [string, string][] = []
  for (const e of usable) {
    const f = flow(e)
    if (f) candidates.push(f)
  }
  const state = new Map<string, 0 | 1 | 2>() // 0 new, 1 on stack, 2 done
  const allOut = new Map<string, string[]>(ids.map((id) => [id, []]))
  for (const [a, b] of candidates) allOut.get(a)!.push(b)
  const accepted = new Set<string>()
  const visit = (start: string) => {
    // iterative DFS so long chains do not overflow the stack
    const stack: [string, number][] = [[start, 0]]
    state.set(start, 1)
    while (stack.length) {
      const top = stack[stack.length - 1]
      const [n, i] = top
      const next = allOut.get(n)!
      if (i >= next.length) {
        state.set(n, 2)
        stack.pop()
        continue
      }
      top[1]++
      const m = next[i]
      const s = state.get(m) ?? 0
      if (s === 1) continue // back edge: closes a cycle
      accepted.add(`${n}\u0000${m}`)
      if (s === 0) {
        state.set(m, 1)
        stack.push([m, 0])
      }
    }
  }
  for (const id of ids) if (!state.get(id)) visit(id)
  for (const [a, b] of candidates) if (accepted.has(`${a}\u0000${b}`) && !out.get(a)!.includes(b)) out.get(a)!.push(b)

  // longest-path layering (topological order via Kahn)
  const indeg = new Map<string, number>(ids.map((id) => [id, 0]))
  for (const id of ids) for (const m of out.get(id)!) indeg.set(m, indeg.get(m)! + 1)
  const layer = new Map<string, number>(ids.map((id) => [id, 0]))
  const queue = ids.filter((id) => indeg.get(id) === 0)
  while (queue.length) {
    const n = queue.shift()!
    for (const m of out.get(n)!) {
      layer.set(m, Math.max(layer.get(m)!, layer.get(n)! + 1))
      indeg.set(m, indeg.get(m)! - 1)
      if (indeg.get(m) === 0) queue.push(m)
    }
  }

  // order within layers: start in input order, then barycenter sweeps
  const layers: string[][] = []
  for (const id of ids) {
    const l = layer.get(id)!
    ;(layers[l] ??= []).push(id)
  }
  const preds = new Map<string, string[]>(ids.map((id) => [id, []]))
  const succs = new Map<string, string[]>(ids.map((id) => [id, []]))
  for (const id of ids)
    for (const m of out.get(id)!) {
      succs.get(id)!.push(m)
      preds.get(m)!.push(id)
    }
  const pos = new Map<string, number>()
  const index = () => layers.forEach((ls) => ls.forEach((id, i) => pos.set(id, i)))
  index()
  const bary = (id: string, nb: Map<string, string[]>) => {
    const list = nb.get(id)!
    return list.length ? list.reduce((s, x) => s + pos.get(x)!, 0) / list.length : pos.get(id)!
  }
  for (let s = 0; s < sweeps; s++) {
    const down = s % 2 === 0
    const range = down ? layers.map((_, i) => i).slice(1) : layers.map((_, i) => i).slice(0, -1).reverse()
    for (const l of range) {
      const nb = down ? preds : succs
      const keyed = layers[l].map((id) => ({ id, b: bary(id, nb), p: pos.get(id)! }))
      keyed.sort((a, b) => a.b - b.b || a.p - b.p)
      layers[l] = keyed.map((k) => k.id)
      layers[l].forEach((id, i) => pos.set(id, i))
    }
  }

  // coordinates: layers are columns, centred vertically
  const tallest = Math.max(1, ...layers.map((ls) => ls?.length ?? 0))
  const height = tallest * H + (tallest - 1) * GY
  const placed = new Map<string, PlacedNode>()
  layers.forEach((ls, l) => {
    const colH = ls.length * H + (ls.length - 1) * GY
    const top = (height - colH) / 2
    ls.forEach((id, i) => placed.set(id, { id, layer: l, order: i, x: l * (W + GX), y: top + i * (H + GY) }))
  })
  const nLayers = layers.length
  const width = nLayers ? nLayers * W + (nLayers - 1) * GX : 0

  const placedEdges: PlacedEdge[] = usable.map((e) => {
    const f = flow(e)
    const isFlow = !!f && accepted.has(`${f[0]}\u0000${f[1]}`)
    // draw from upstream to downstream (or source → target when symmetric)
    const [a, b] = f ?? [e.source, e.target]
    return { ...e, flow: isFlow, path: edgePath(placed.get(a)!, placed.get(b)!, W, H) }
  })
  return { nodes: ids.map((id) => placed.get(id)!), edges: placedEdges, width, height, layers: nLayers }
}

/** A smooth path between two boxes: right side → left side, or an arc within a column. */
export function edgePath(a: { x: number; y: number }, b: { x: number; y: number }, w: number, h: number): string {
  const r = (n: number) => Math.round(n * 10) / 10
  if (a.x === b.x) {
    // same layer: bow out to the right
    const x = a.x + w
    const y1 = a.y + h / 2
    const y2 = b.y + h / 2
    const bow = 40 + Math.min(80, Math.abs(y2 - y1) / 4)
    return `M ${r(x)} ${r(y1)} C ${r(x + bow)} ${r(y1)}, ${r(x + bow)} ${r(y2)}, ${r(x)} ${r(y2)}`
  }
  const leftToRight = a.x < b.x
  const x1 = leftToRight ? a.x + w : a.x
  const x2 = leftToRight ? b.x : b.x + w
  const y1 = a.y + h / 2
  const y2 = b.y + h / 2
  const dx = (x2 - x1) / 2
  return `M ${r(x1)} ${r(y1)} C ${r(x1 + dx)} ${r(y1)}, ${r(x2 - dx)} ${r(y2)}, ${r(x2)} ${r(y2)}`
}

/** Ids of the nodes connected to `id` (both directions, any edge type). */
export function neighbours(id: string, edges: readonly GraphEdgeIn[]): Set<string> {
  const out = new Set<string>()
  for (const e of edges) {
    if (e.source === id) out.add(e.target)
    if (e.target === id) out.add(e.source)
  }
  return out
}
