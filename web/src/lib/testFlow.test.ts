import { describe, expect, it } from 'vitest'
import { buildFlow, callsToLoad, layoutFlow, runOrder, wrapText, type FlowTest } from './testFlow'

const shared: FlowTest = { id: 'sh', key: 'TC-9', title: 'Log in', steps: [{ action: 'Open', expected: 'Form', position: 1 }, { action: 'Submit', expected: '', position: 2 }] }
const test: FlowTest = {
  id: 't1',
  key: 'TC-1',
  title: 'Order',
  preconditions: 'A user',
  expected_result: 'Order placed',
  steps: [
    { action: 'Pay', expected: 'Receipt', data: 'card', position: 2 },
    { action: '', expected: '', position: 1, call_test_id: 'sh', call_test_key: 'TC-9', call_test_title: 'Log in' },
  ],
}

describe('buildFlow', () => {
  it('draws a manual test', () => {
    const f = buildFlow(test, { prerequisites: [{ key: 'TC-0' }] })
    expect(f.nodes.map((n) => n.kind)).toEqual(['start', 'check', 'blocked', 'precondition', 'call', 'step', 'check', 'fail', 'check', 'fail', 'end'])
    const prereq = f.nodes.find((n) => n.id === 'prereq')!
    expect(prereq.detail).toBe('TC-0')
    const pay = f.nodes.find((n) => n.text === 'Pay')!
    expect(pay.number).toBe('2')
    expect(pay.detail).toBe('card')
    expect(f.nodes.find((n) => n.id === 's2x')?.detail).toBe('Receipt')
    expect(f.edges).toContainEqual({ from: 'prereq', to: 'pre', label: 'yes' })
    expect(f.edges).toContainEqual({ from: 's2x', to: 'overall', label: 'yes' })
    expect(f.edges).toContainEqual({ from: 's2x', to: 's2x-no', label: 'no' })
    expect(f.edges.at(-1)).toEqual({ from: 'overall', to: 'end', label: 'yes' })
  })
  it('expands shared steps on demand', () => {
    const collapsed = buildFlow(test, { expanded: new Set(['c1']) })
    expect(collapsed.nodes.find((n) => n.kind === 'call')?.note).toBe('missing')
    expect(callsToLoad(collapsed, new Map())).toEqual(['sh'])
    const f = buildFlow(test, { expanded: new Set(['c1']), calls: new Map([['sh', shared]]) })
    const inner = f.nodes.filter((n) => n.group === 'c1')
    expect(inner.map((n) => n.number ?? n.kind)).toEqual(['1.1', 'check', 'fail', '1.2'])
    expect(f.groups).toEqual([{ id: 'c1', callKey: 'TC-9', depth: 1, first: 'c1-s1', last: 'c1-s2' }])
    expect(callsToLoad(f, new Map([['sh', 1]]))).toEqual([])
  })
  it('stops at cycles and depth', () => {
    const loop: FlowTest = { id: 'a', key: 'A', title: 'A', steps: [{ action: '', expected: '', call_test_id: 'a', call_test_key: 'A' }] }
    const f = buildFlow(loop, { expanded: new Set(['c1']), calls: new Map([['a', loop]]) })
    expect(f.nodes.find((n) => n.kind === 'call')?.note).toBe('cycle')
    const deep = buildFlow(test, { expanded: new Set(['c1']), calls: new Map([['sh', shared]]), maxDepth: 0 })
    expect(deep.nodes.find((n) => n.kind === 'call')?.note).toBe('depth')
  })
  it('draws automated and empty tests', () => {
    const auto = buildFlow({ id: 'x', key: 'TC-2', title: 'API', execution_type: 'automated', script_language: 'python' })
    expect(auto.nodes.map((n) => n.kind)).toEqual(['start', 'script', 'check', 'fail', 'end'])
    expect(auto.nodes[1].detail).toBe('python')
    const empty = buildFlow({ id: 'y', key: 'TC-3', title: 'New', steps: [] }, { labels: { noSteps: 'Inga steg' } })
    expect(empty.nodes.map((n) => n.text)).toContain('Inga steg')
  })
})

describe('layoutFlow', () => {
  it('stacks the main path and puts failures to the right', () => {
    const l = layoutFlow(buildFlow(test, { expanded: new Set(['c1']), calls: new Map([['sh', shared]]) }))
    const main = l.nodes.filter((n) => n.kind !== 'fail' && n.kind !== 'blocked')
    for (let i = 1; i < main.length; i++) expect(main[i].y).toBeGreaterThan(main[i - 1].y)
    const fail = l.nodes.find((n) => n.id === 's2x-no')!
    const owner = l.nodes.find((n) => n.id === 's2x')!
    expect(fail.x).toBeGreaterThan(owner.x + owner.w - 40)
    expect(fail.y + fail.h / 2).toBe(owner.y + owner.h / 2)
    expect(l.frames).toHaveLength(1)
    expect(l.nodes.find((n) => n.id === 'c1-s1')!.x).toBeGreaterThan(0)
    expect(l.edges.every((e) => e.path.startsWith('M '))).toBe(true)
    expect(l.width).toBeGreaterThan(300)
    expect(l.height).toBeGreaterThan(0)
  })
})

describe('wrapText', () => {
  it('wraps and shortens', () => {
    expect(wrapText('one two three four', 9, 2)).toEqual(['one two', 'three…'])
    expect(wrapText('short', 20, 2)).toEqual(['short'])
    expect(wrapText('averyveryverylongword', 8, 1)).toEqual(['averyve…'])
    expect(wrapText('', 8, 2)).toEqual([])
  })
})

describe('runOrder', () => {
  it('groups tests into stages', () => {
    const tests = [
      { id: 'a', key: 'TC-1', title: 'A', status: 'passed', position: 1 },
      { id: 'b', key: 'TC-2', title: 'B', status: 'not_run', position: 2 },
      { id: 'c', key: 'TC-3', title: 'C', status: 'not_run', position: 3 },
      { id: 'd', key: 'TC-4', title: 'D', status: 'not_run', position: 4 },
    ]
    const o = runOrder(tests, [
      { test_id: 'b', depends_on_id: 'a' },
      { test_id: 'c', depends_on_id: 'b' },
      { test_id: 'c', depends_on_id: 'a' },
      { test_id: 'd', depends_on_id: 'zz', depends_on_key: 'TC-99' },
      { test_id: 'x', depends_on_id: 'a' },
    ])
    expect(o.stages.map((s) => s.tests.map((t) => t.key))).toEqual([['TC-1', 'TC-4'], ['TC-2'], ['TC-3']])
    expect(o.stages[2].tests[0].after).toEqual(['TC-2', 'TC-1'])
    expect(o.external).toEqual([{ test: 'd', on: 'TC-99' }])
  })
  it('breaks cycles', () => {
    const tests = [
      { id: 'a', key: 'A', title: '', status: '' },
      { id: 'b', key: 'B', title: '', status: '' },
    ]
    const o = runOrder(tests, [
      { test_id: 'a', depends_on_id: 'b' },
      { test_id: 'b', depends_on_id: 'a' },
      { test_id: 'a', depends_on_id: 'a' },
    ])
    expect(o.cyclic).toHaveLength(1)
    expect(o.stages.flatMap((s) => s.tests).length).toBe(2)
  })
})
