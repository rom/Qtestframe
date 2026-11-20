// Flow diagrams of tests: the steps of one test as a flowchart (start,
// prerequisites, preconditions, steps, checks of the expected results with a
// "no → failed" branch, shared steps as sub-processes that can be expanded,
// automated tests as script → exit code), and the run order of a test run
// in stages derived from the test dependencies.

export type FlowKind = 'start' | 'end' | 'fail' | 'blocked' | 'prereq' | 'precondition' | 'step' | 'check' | 'call' | 'script' | 'exit' | 'empty'

export interface FlowNode {
  id: string
  kind: FlowKind
  /** main text (already shortened only by the renderer) */
  text: string
  /** second line: test data, the called test's title, the script language … */
  detail?: string
  /** step number, "3" or "3.2" inside an expanded shared step */
  number?: string
  /** nesting level of expanded shared steps (0 = the test itself) */
  depth: number
  /** the shared step instance this node belongs to (for the frame) */
  group?: string
  /** call nodes: the called test */
  callId?: string
  callKey?: string
  expanded?: boolean
  /** call nodes: why the call cannot be expanded */
  note?: 'cycle' | 'depth' | 'missing'
  /** check nodes: the node of the "no" branch */
  failId?: string
}

export interface FlowEdge {
  from: string
  to: string
  label?: 'yes' | 'no'
}

export interface FlowGroup {
  id: string
  callKey: string
  depth: number
  first: string
  last: string
}

export interface Flow {
  nodes: FlowNode[]
  edges: FlowEdge[]
  groups: FlowGroup[]
}

export interface FlowStep {
  id?: string
  action: string
  expected: string
  data?: string
  position?: number
  call_test_id?: string | null
  call_test_key?: string
  call_test_title?: string
}

export interface FlowTest {
  id: string
  key: string
  title: string
  preconditions?: string
  execution_type?: string
  script_language?: string
  expected_result?: string
  steps?: FlowStep[] | null
}

export interface FlowOptions {
  /** called tests by ID (loaded on demand) */
  calls?: Map<string, FlowTest>
  /** IDs of call instances ("<path>") that are expanded */
  expanded?: Set<string>
  /** tests that must have passed first */
  prerequisites?: { key: string; title?: string }[]
  maxDepth?: number
  /** texts of the fixed nodes (translated) */
  labels?: Partial<Record<'start' | 'passed' | 'failed' | 'blocked' | 'prereq' | 'expected' | 'overall' | 'script' | 'exit' | 'noSteps' | 'precondition', string>>
}

const DEFAULT_LABELS = {
  start: 'Start',
  passed: 'Passed',
  failed: 'Failed',
  blocked: 'Blocked',
  prereq: 'Prerequisites passed?',
  expected: 'As expected?',
  overall: 'Expected result reached?',
  script: 'Run the script',
  exit: 'Exit code 0?',
  noSteps: 'No steps yet',
  precondition: 'Preconditions',
}

export function sortedSteps(steps: FlowStep[] | null | undefined): FlowStep[] {
  return [...(steps ?? [])].sort((a, b) => (a.position ?? 0) - (b.position ?? 0))
}

/** Build the flowchart of a test. */
export function buildFlow(test: FlowTest, opts: FlowOptions = {}): Flow {
  const L = { ...DEFAULT_LABELS, ...opts.labels }
  const maxDepth = opts.maxDepth ?? 4
  const nodes: FlowNode[] = []
  const edges: FlowEdge[] = []
  const groups: FlowGroup[] = []
  let prev: { id: string; label?: 'yes' | 'no' } | null = null
  const add = (n: FlowNode) => {
    nodes.push(n)
    if (prev) edges.push({ from: prev.id, to: n.id, ...(prev.label ? { label: prev.label } : {}) })
    prev = { id: n.id }
    return n
  }
  const check = (id: string, text: string, depth: number, group: string | undefined, failKind: 'fail' | 'blocked' = 'fail', number?: string) => {
    const failId = id + '-no'
    add({ id, kind: 'check', text, depth, group, failId, number })
    nodes.push({ id: failId, kind: failKind, text: failKind === 'fail' ? L.failed : L.blocked, depth, group })
    edges.push({ from: id, to: failId, label: 'no' })
    prev = { id, label: 'yes' }
  }

  add({ id: 'start', kind: 'start', text: L.start, detail: `${test.key} ${test.title}`.trim(), depth: 0 })
  if (opts.prerequisites?.length) {
    check('prereq', L.prereq, 0, undefined, 'blocked')
    const node = nodes.find((n) => n.id === 'prereq')!
    node.detail = opts.prerequisites.map((p) => p.key).join(', ')
  }
  if (test.preconditions?.trim()) add({ id: 'pre', kind: 'precondition', text: L.precondition, detail: test.preconditions.trim(), depth: 0 })

  if (test.execution_type === 'automated') {
    add({ id: 'script', kind: 'script', text: L.script, detail: test.script_language || '', depth: 0 })
    check('exit', L.exit, 0, undefined)
  } else {
    const walk = (t: FlowTest, path: string, prefix: string, depth: number, group: string | undefined, seen: Set<string>) => {
      const steps = sortedSteps(t.steps)
      steps.forEach((s, i) => {
        const number = prefix + (i + 1)
        const id = `${path}s${i + 1}`
        if (s.call_test_id) {
          const instance = `${path}c${i + 1}`
          const called = opts.calls?.get(s.call_test_id)
          const want = opts.expanded?.has(instance) ?? false
          let note: FlowNode['note']
          if (seen.has(s.call_test_id)) note = 'cycle'
          else if (depth + 1 > maxDepth) note = 'depth'
          else if (want && !called) note = 'missing'
          const open = want && !note && !!called
          add({
            id,
            kind: 'call',
            text: s.call_test_key || called?.key || '',
            detail: s.call_test_title || called?.title || '',
            number,
            depth,
            group,
            callId: s.call_test_id,
            callKey: instance,
            expanded: open,
            ...(note ? { note } : {}),
          })
          if (open && called) {
            const before = nodes.length
            walk(called, instance + '-', number + '.', depth + 1, instance, new Set([...seen, s.call_test_id]))
            if (nodes.length > before) {
              const inner = nodes.slice(before).filter((n) => n.group === instance && n.kind !== 'fail')
              groups.push({ id: instance, callKey: called.key, depth: depth + 1, first: inner[0]?.id ?? id, last: inner[inner.length - 1]?.id ?? id })
            }
          }
          return
        }
        add({ id, kind: 'step', text: s.action || '–', detail: s.data || '', number, depth, group })
        if (s.expected?.trim()) {
          check(id + 'x', L.expected, depth, group, 'fail')
          nodes[nodes.length - 2].detail = s.expected.trim()
        }
      })
      return steps.length
    }
    const count = walk(test, '', '', 0, undefined, new Set([test.id]))
    if (!count) add({ id: 'empty', kind: 'empty', text: L.noSteps, depth: 0 })
    if (test.expected_result?.trim()) {
      check('overall', L.overall, 0, undefined)
      nodes.find((n) => n.id === 'overall')!.detail = test.expected_result.trim()
    }
  }
  add({ id: 'end', kind: 'end', text: L.passed, depth: 0 })
  return { nodes, edges, groups }
}

/** IDs of the called tests the flow needs (expanded calls). */
export function callsToLoad(flow: Flow, loaded: Map<string, unknown>): string[] {
  return [...new Set(flow.nodes.filter((n) => n.kind === 'call' && n.note === 'missing' && n.callId && !loaded.has(n.callId)).map((n) => n.callId!))]
}

// ---------------------------------------------------------------- layout

export interface PlacedFlowNode extends FlowNode {
  x: number
  y: number
  w: number
  h: number
}

export interface FlowLayout {
  nodes: PlacedFlowNode[]
  edges: (FlowEdge & { path: string; lx: number; ly: number })[]
  frames: { id: string; label: string; x: number; y: number; w: number; h: number }[]
  width: number
  height: number
}

export const FLOW_SIZE = { w: 300, h: 56, checkH: 76, termH: 40, gap: 30, indent: 26, failW: 110, failGap: 50 }

function heightOf(kind: FlowKind): number {
  if (kind === 'check') return FLOW_SIZE.checkH
  if (kind === 'start' || kind === 'end' || kind === 'fail' || kind === 'blocked') return FLOW_SIZE.termH
  return FLOW_SIZE.h
}

/** Vertical layout: the main path top to bottom, "no" branches to the right. */
export function layoutFlow(flow: Flow): FlowLayout {
  const S = FLOW_SIZE
  const maxDepth = Math.max(0, ...flow.nodes.map((n) => n.depth))
  const placed = new Map<string, PlacedFlowNode>()
  const failOf = new Map(flow.nodes.filter((n) => n.failId).map((n) => [n.failId!, n.id]))
  let y = 0
  for (const n of flow.nodes) {
    if (failOf.has(n.id)) continue
    const h = heightOf(n.kind)
    const x = n.depth * S.indent
    placed.set(n.id, { ...n, x, y, w: S.w, h })
    y += h + S.gap
  }
  const mainRight = S.w + maxDepth * S.indent
  for (const n of flow.nodes) {
    const owner = failOf.get(n.id)
    if (!owner) continue
    const o = placed.get(owner)!
    const h = S.termH
    placed.set(n.id, { ...n, x: mainRight + S.failGap, y: o.y + (o.h - h) / 2, w: S.failW, h })
  }
  const edges = flow.edges.map((e) => {
    const a = placed.get(e.from)!
    const b = placed.get(e.to)!
    if (e.label === 'no') {
      const sx = a.x + a.w - 40 // right corner of the diamond (220 wide, centred)
      const sy = a.y + a.h / 2
      return { ...e, path: `M ${sx} ${sy} L ${b.x} ${b.y + b.h / 2}`, lx: (sx + b.x) / 2, ly: sy - 6 }
    }
    const sx = a.x + a.w / 2
    const sy = a.y + a.h
    const tx = b.x + b.w / 2
    const ty = b.y
    const path = sx === tx ? `M ${sx} ${sy} L ${tx} ${ty}` : `M ${sx} ${sy} L ${sx} ${(sy + ty) / 2} L ${tx} ${(sy + ty) / 2} L ${tx} ${ty}`
    return { ...e, path, lx: sx + 8, ly: sy + 14 }
  })
  const frames = flow.groups.map((g) => {
    const first = placed.get(g.first)!
    const last = placed.get(g.last)!
    const x = g.depth * S.indent - 12
    return { id: g.id, label: g.callKey, x, y: first.y - 22, w: S.w + 24, h: last.y + last.h - first.y + 30 }
  })
  const nodes = [...placed.values()]
  const width = Math.max(mainRight, ...nodes.map((n) => n.x + n.w)) + 10
  return { nodes, edges, frames, width, height: Math.max(0, y - S.gap) }
}

/** Split text into at most `lines` lines of about `width` characters. */
export function wrapText(text: string, width: number, lines: number): string[] {
  const words = text.replace(/\s+/g, ' ').trim().split(' ').filter(Boolean)
  const out: string[] = []
  let cur = ''
  for (const w of words) {
    const word = w.length > width ? w.slice(0, width - 1) + '…' : w
    if (!cur) cur = word
    else if ((cur + ' ' + word).length <= width) cur += ' ' + word
    else {
      out.push(cur)
      cur = word
      if (out.length === lines) break
    }
  }
  if (out.length < lines && cur) out.push(cur)
  const used = out.join(' ').replace(/…/g, '').length
  if (out.length === lines && used < words.join(' ').length) {
    const last = out[lines - 1]
    out[lines - 1] = (last.length >= width ? last.slice(0, width - 1) : last).replace(/…$/, '') + '…'
  }
  return out
}

// ---------------------------------------------------------------- run order

export interface StageTest {
  id: string
  key: string
  title: string
  status: string
  position?: number
}

export interface Stage {
  index: number
  tests: (StageTest & { after: string[] })[]
}

export interface RunOrder {
  stages: Stage[]
  /** dependencies on tests that are not in the run */
  external: { test: string; on: string }[]
  /** dependencies ignored because they close a cycle */
  cyclic: { test: string; on: string }[]
}

/**
 * Stages of a run: a test comes after every test of the run it depends on
 * (stage = longest chain of prerequisites). Tests without prerequisites in
 * the run are in stage 1.
 */
export function runOrder(tests: StageTest[], deps: { test_id: string; depends_on_id: string; depends_on_key?: string }[]): RunOrder {
  const inRun = new Map(tests.map((t) => [t.id, t]))
  const pre = new Map<string, string[]>()
  const external: RunOrder['external'] = []
  for (const d of deps) {
    if (!inRun.has(d.test_id)) continue
    if (!inRun.has(d.depends_on_id)) {
      external.push({ test: d.test_id, on: d.depends_on_key || d.depends_on_id })
      continue
    }
    if (d.test_id === d.depends_on_id) continue
    pre.set(d.test_id, [...(pre.get(d.test_id) ?? []), d.depends_on_id])
  }
  const cyclic: RunOrder['cyclic'] = []
  const level = new Map<string, number>()
  const state = new Map<string, 1 | 2>() // 1 visiting, 2 done
  const visit = (id: string): number => {
    if (state.get(id) === 2) return level.get(id)!
    state.set(id, 1)
    let lv = 0
    for (const p of pre.get(id) ?? []) {
      if (state.get(p) === 1) {
        cyclic.push({ test: id, on: p })
        continue
      }
      lv = Math.max(lv, visit(p) + 1)
    }
    state.set(id, 2)
    level.set(id, lv)
    return lv
  }
  const ordered = [...tests].sort((a, b) => (a.position ?? 0) - (b.position ?? 0) || a.key.localeCompare(b.key, undefined, { numeric: true }))
  for (const t of ordered) visit(t.id)
  const stages: Stage[] = []
  for (const t of ordered) {
    const lv = level.get(t.id) ?? 0
    while (stages.length <= lv) stages.push({ index: stages.length + 1, tests: [] })
    const after = (pre.get(t.id) ?? []).filter((p) => !cyclic.some((c) => c.test === t.id && c.on === p)).map((p) => inRun.get(p)!.key)
    stages[lv].tests.push({ ...t, after })
  }
  return { stages, external, cyclic }
}
