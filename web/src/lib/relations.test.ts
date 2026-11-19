import { describe, expect, it } from 'vitest'
import { groupRelations, otherEnd, relationLabel, relationTypeOptions } from './relations'
import type { Relation } from './types'

function rel(id: string, type: string, direction: string | undefined, other: string, self = 'me'): Relation {
  const incoming = direction === 'incoming'
  return {
    id,
    project_id: 'p',
    type,
    direction,
    created_by: '',
    created_at: '',
    source_id: incoming ? other : self,
    source_key: incoming ? other.toUpperCase() : 'ME',
    source_title: incoming ? 'Other ' + other : 'Me',
    target_id: incoming ? self : other,
    target_key: incoming ? 'ME' : other.toUpperCase(),
    target_title: incoming ? 'Me' : 'Other ' + other,
  }
}

describe('relations', () => {
  it('labels both directions', () => {
    expect(relationLabel('refines', 'outgoing')).toBe('refines (parent)')
    expect(relationLabel('refines', 'incoming')).toBe('refined by (child)')
    expect(relationLabel('depends_on', 'incoming')).toBe('required by')
    expect(relationLabel('relates_to', undefined)).toBe('relates to')
    expect(relationLabel('custom_kind', 'incoming')).toBe('incoming custom kind')
    expect(relationLabel('custom_kind', 'outgoing')).toBe('custom kind')
  })
  it('offers all types', () => {
    const o = relationTypeOptions()
    expect(o).toHaveLength(6)
    expect(o[0]).toEqual({ label: 'refines (parent)', value: 'refines' })
  })
  it('finds the other end', () => {
    expect(otherEnd(rel('1', 'refines', 'outgoing', 'r2'))).toEqual({ id: 'r2', key: 'R2', title: 'Other r2' })
    expect(otherEnd(rel('1', 'refines', 'incoming', 'r3'))).toEqual({ id: 'r3', key: 'R3', title: 'Other r3' })
    // without direction: infer from selfId
    const r = { ...rel('1', 'refines', 'incoming', 'r3'), direction: undefined }
    expect(otherEnd(r, 'me').id).toBe('r3')
    expect(otherEnd(r).id).toBe('me')
  })
  it('groups by type and direction', () => {
    const list = [
      rel('a', 'relates_to', 'outgoing', 'r9'),
      rel('b', 'refines', 'incoming', 'r10'),
      rel('c', 'refines', 'incoming', 'r2'),
      rel('d', 'refines', 'outgoing', 'r1'),
      rel('e', 'weird', 'outgoing', 'r5'),
      rel('f', 'aaa', 'outgoing', 'r6'),
      { ...rel('g', 'depends_on', 'incoming', 'r7'), direction: undefined },
    ]
    const g = groupRelations(list, 'me')
    expect(g.map((x) => x.label)).toEqual(['refines (parent)', 'refined by (child)', 'required by', 'relates to', 'aaa', 'weird'])
    expect(g[1].items.map((r) => r.id)).toEqual(['c', 'b'])
    expect(groupRelations(null)).toEqual([])
  })
})
