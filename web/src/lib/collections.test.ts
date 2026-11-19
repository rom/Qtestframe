import { describe, expect, it, vi } from 'vitest'
import { debounce, entriesByCount, groupBy, linkKey, moveItem, renumber, toggle } from './collections'

describe('arrays', () => {
  it('moveItem', () => {
    expect(moveItem([1, 2, 3], 0, 2)).toEqual([2, 3, 1])
    expect(moveItem([1, 2, 3], 2, 0)).toEqual([3, 1, 2])
    expect(moveItem([1, 2, 3], 1, 99)).toEqual([1, 3, 2])
    expect(moveItem([1, 2, 3], 1, -5)).toEqual([2, 1, 3])
    expect(moveItem([1, 2, 3], 5, 0)).toEqual([1, 2, 3])
  })
  it('renumber', () => {
    expect(renumber([{ position: 9, a: 1 }, { a: 2 }])).toEqual([
      { position: 1, a: 1 },
      { position: 2, a: 2 },
    ])
  })
  it('groupBy keeps order', () => {
    const g = groupBy(['b1', 'a1', 'b2'], (s) => s[0])
    expect(g).toEqual([
      { key: 'b', items: ['b1', 'b2'] },
      { key: 'a', items: ['a1'] },
    ])
  })
  it('entriesByCount', () => {
    expect(entriesByCount({ a: 1, b: 3, c: 3 })).toEqual([
      ['b', 3],
      ['c', 3],
      ['a', 1],
    ])
    expect(entriesByCount({ x: 1, must: 2, may: 5, y: 1 }, ['must', 'may'])).toEqual([
      ['must', 2],
      ['may', 5],
      ['x', 1],
      ['y', 1],
    ])
    expect(entriesByCount(null)).toEqual([])
  })
  it('toggle and linkKey', () => {
    expect(toggle([1, 2], 2)).toEqual([1])
    expect(toggle([1], 2)).toEqual([1, 2])
    expect(linkKey('r', 't')).toBe('r|t')
  })
})

describe('debounce', () => {
  it('calls once with the latest args, supports cancel and flush', () => {
    vi.useFakeTimers()
    const fn = vi.fn()
    const d = debounce(fn, 100)
    d(1)
    d(2)
    vi.advanceTimersByTime(99)
    expect(fn).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1)
    expect(fn).toHaveBeenCalledWith(2)
    d(3)
    d.cancel()
    vi.advanceTimersByTime(200)
    expect(fn).toHaveBeenCalledTimes(1)
    d(4)
    d.flush()
    expect(fn).toHaveBeenLastCalledWith(4)
    d.flush()
    d.cancel()
    expect(fn).toHaveBeenCalledTimes(2)
    vi.useRealTimers()
  })
})
