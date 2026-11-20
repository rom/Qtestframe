import { describe, expect, it } from 'vitest'
import { choiceValue, parseChoice, planAdd, planRemove, reasonCounts, relationChoices } from './bulkRelations'
import type { Relation } from './types'

const r = (id: string, s: string, d: string, type: string) => ({ id, source_id: s, target_id: d, type }) as Relation

describe('relationChoices', () => {
  it('has both directions except for symmetric types', () => {
    const c = relationChoices().map(choiceValue)
    expect(c).toContain('refines:outgoing')
    expect(c).toContain('refines:incoming')
    expect(c).toContain('relates_to:outgoing')
    expect(c).not.toContain('relates_to:incoming')
    expect(c).not.toContain('conflicts_with:incoming')
  })
  it('parses values', () => {
    expect(parseChoice('refines:incoming')).toEqual({ type: 'refines', direction: 'incoming' })
    expect(parseChoice('refines')).toBeNull()
    expect(parseChoice('')).toBeNull()
  })
})

describe('planAdd', () => {
  const existing = [r('1', 'a', 'p', 'refines'), r('2', 'x', 'a', 'relates_to')]
  it('relates each selected requirement to each target, without self and existing relations', () => {
    expect(planAdd(existing, ['a', 'b', 'p'], ['p'], { type: 'refines', direction: 'outgoing' })).toEqual([{ source_id: 'b', target_id: 'p', type: 'refines' }])
  })
  it('turns the incoming direction around', () => {
    expect(planAdd(existing, ['a', 'b'], ['p'], { type: 'refines', direction: 'incoming' })).toEqual([
      { source_id: 'p', target_id: 'a', type: 'refines' },
      { source_id: 'p', target_id: 'b', type: 'refines' },
    ])
  })
  it('treats symmetric relations as existing in both directions and drops duplicates', () => {
    expect(planAdd(existing, ['a', 'a'], ['x', 'y'], { type: 'relates_to', direction: 'outgoing' })).toEqual([{ source_id: 'a', target_id: 'y', type: 'relates_to' }])
  })
})

describe('planRemove', () => {
  const existing = [r('1', 'a', 'p', 'refines'), r('2', 'p', 'b', 'refines'), r('3', 'x', 'a', 'relates_to'), r('4', 'c', 'd', 'refines')]
  const ids = (l: Relation[]) => l.map((x) => x.id)
  it('matches type and direction', () => {
    expect(ids(planRemove(existing, ['a', 'b'], [], { type: 'refines', direction: 'outgoing' }))).toEqual(['1'])
    expect(ids(planRemove(existing, ['a', 'b'], [], { type: 'refines', direction: 'incoming' }))).toEqual(['2'])
  })
  it('takes any relation without a choice and either direction for symmetric types', () => {
    expect(ids(planRemove(existing, ['a'], [], null))).toEqual(['1', '3'])
    expect(ids(planRemove(existing, ['a'], [], { type: 'relates_to', direction: 'outgoing' }))).toEqual(['3'])
  })
  it('limits the other end to the targets', () => {
    expect(ids(planRemove(existing, ['a'], ['x'], null))).toEqual(['3'])
    expect(ids(planRemove(existing, ['a', 'b'], ['p'], null))).toEqual(['1', '2'])
  })
})

it('counts skip reasons', () => {
  expect(reasonCounts([{ reason: 'exists' }, { reason: 'exists' }, { reason: 'self' }])).toEqual({ exists: 2, self: 1 })
})
