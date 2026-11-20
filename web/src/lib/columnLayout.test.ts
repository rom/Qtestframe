import { describe, expect, it } from 'vitest'
import { moveColumn, normalizeOrder, orderFromKeys, toggleColumn } from './columnLayout'

const all = ['status', 'owner', 'tags', 'relations']

describe('column layout', () => {
  it('normalizes a saved order', () => {
    expect(normalizeOrder(['tags', 'status', 'tags', 'gone', 3], all, ['status'])).toEqual(['tags', 'status'])
    expect(normalizeOrder(null, all, ['status', 'gone'])).toEqual(['status'])
    expect(normalizeOrder([], all, ['status'])).toEqual([])
    expect(normalizeOrder(['relation_count'], all, [], { relation_count: 'relations' })).toEqual(['relations'])
  })
  it('shows, hides and moves columns', () => {
    expect(toggleColumn(['status'], 'tags', true)).toEqual(['status', 'tags'])
    expect(toggleColumn(['status', 'tags'], 'tags', true)).toEqual(['status', 'tags'])
    expect(toggleColumn(['status', 'tags'], 'status', false)).toEqual(['tags'])
    expect(moveColumn(['a', 'b', 'c'], 'c', -1)).toEqual(['a', 'c', 'b'])
    expect(moveColumn(['a', 'b', 'c'], 'a', -1)).toEqual(['a', 'b', 'c'])
    expect(moveColumn(['a', 'b', 'c'], 'a', 5)).toEqual(['b', 'c', 'a'])
    expect(moveColumn(['a'], 'x', 1)).toEqual(['a'])
  })
  it('follows the order of a header drag', () => {
    expect(orderFromKeys(['_sel', 'key', 'tags', 'title', 'status', undefined, 'owner'], ['status', 'owner', 'tags'])).toEqual(['tags', 'status', 'owner'])
    expect(orderFromKeys(['status'], ['status', 'owner'])).toEqual(['status', 'owner'])
  })
})
