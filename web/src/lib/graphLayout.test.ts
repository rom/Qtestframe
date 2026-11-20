import { describe, expect, it } from 'vitest'
import { edgePath, flow, layoutGraph, neighbours } from './graphLayout'

const e = (source: string, target: string, type = 'depends_on', id = `${source}-${target}`) => ({ id, source, target, type })

describe('flow', () => {
  it('puts prerequisites and parents first', () => {
    expect(flow(e('A', 'B', 'depends_on'))).toEqual(['B', 'A'])
    expect(flow(e('A', 'B', 'refines'))).toEqual(['B', 'A'])
    expect(flow(e('A', 'B', 'provides'))).toEqual(['A', 'B'])
    expect(flow(e('A', 'B', 'calls'))).toEqual(['B', 'A'])
    expect(flow(e('A', 'B', 'relates_to'))).toBeNull()
  })
})

describe('layoutGraph', () => {
  it('layers a chain left to right', () => {
    // C depends on B, B depends on A
    const l = layoutGraph(['C', 'B', 'A'], [e('C', 'B'), e('B', 'A')])
    const by = Object.fromEntries(l.nodes.map((n) => [n.id, n]))
    expect([by.A.layer, by.B.layer, by.C.layer]).toEqual([0, 1, 2])
    expect(by.A.x).toBeLessThan(by.B.x)
    expect(l.layers).toBe(3)
    expect(l.width).toBe(3 * 200 + 2 * 90)
    expect(l.edges.every((x) => x.flow && x.path.startsWith('M '))).toBe(true)
  })
  it('uses the longest path and centres columns', () => {
    // D depends on A and on C; C depends on B; B depends on A
    const l = layoutGraph(['A', 'B', 'C', 'D', 'E'], [e('D', 'A'), e('D', 'C'), e('C', 'B'), e('B', 'A'), e('E', 'A')])
    const by = Object.fromEntries(l.nodes.map((n) => [n.id, n]))
    expect(by.D.layer).toBe(3)
    expect(by.E.layer).toBe(1)
    expect(l.height).toBe(2 * 52 + 22) // layer 1 has B and E
    expect(by.D.y).toBe((l.height - 52) / 2)
  })
  it('survives cycles and ignores symmetric relations for layering', () => {
    const l = layoutGraph(['A', 'B', 'C'], [e('A', 'B'), e('B', 'C'), e('C', 'A'), e('A', 'C', 'relates_to', 'r1'), e('A', 'A'), e('A', 'Z')])
    expect(l.nodes).toHaveLength(3)
    expect(l.edges).toHaveLength(4) // self edge and unknown node dropped
    expect(l.edges.filter((x) => !x.flow)).toHaveLength(2) // the cycle-closing edge and the symmetric one
    expect(new Set(l.nodes.map((n) => n.layer)).size).toBe(3)
  })
  it('reduces crossings with barycenter sweeps', () => {
    // two parallel chains given in crossed order
    const l = layoutGraph(['a1', 'b1', 'b2', 'a2'], [e('a2', 'a1'), e('b2', 'b1')])
    const by = Object.fromEntries(l.nodes.map((n) => [n.id, n]))
    expect(by.a1.order === by.a2.order).toBe(true)
    expect(by.b1.order === by.b2.order).toBe(true)
  })
  it('handles an empty graph and isolated nodes', () => {
    expect(layoutGraph([], [])).toEqual({ nodes: [], edges: [], width: 0, height: 52, layers: 0 })
    const l = layoutGraph(['X', 'Y'], [])
    expect(l.nodes.map((n) => n.layer)).toEqual([0, 0])
  })
})

describe('helpers', () => {
  it('draws paths between and within columns', () => {
    expect(edgePath({ x: 0, y: 0 }, { x: 290, y: 74 }, 200, 52)).toBe('M 200 26 C 245 26, 245 100, 290 100')
    expect(edgePath({ x: 290, y: 0 }, { x: 0, y: 0 }, 200, 52)).toBe('M 290 26 C 245 26, 245 26, 200 26')
    expect(edgePath({ x: 0, y: 0 }, { x: 0, y: 74 }, 200, 52)).toMatch(/^M 200 26 C 258\.5 26, 258\.5 100, 200 100$/)
  })
  it('finds neighbours', () => {
    expect([...neighbours('B', [e('A', 'B'), e('B', 'C'), e('D', 'E')])].sort()).toEqual(['A', 'C'])
  })
})
