import { describe, expect, it } from 'vitest'
import {
  GRID_COLS,
  MAX_H,
  applyKey,
  cellAt,
  clampItem,
  freeSpot,
  gridRows,
  keyChange,
  moveItem,
  overlaps,
  readingOrder,
  resizeItem,
  resolve,
  sameLayout,
  tidy,
  type GridItem,
} from './dashboardGrid'

const it_ = (id: string, x: number, y: number, w: number, h: number): GridItem => ({ id, x, y, w, h })
const anyOverlap = (items: GridItem[]) => items.some((a) => items.some((b) => overlaps(a, b)))

describe('dashboard grid', () => {
  it('detects overlaps', () => {
    expect(overlaps(it_('a', 0, 0, 6, 4), it_('b', 5, 3, 2, 2))).toBe(true)
    expect(overlaps(it_('a', 0, 0, 6, 4), it_('b', 6, 0, 6, 4))).toBe(false)
    expect(overlaps(it_('a', 0, 0, 6, 4), it_('b', 0, 4, 6, 4))).toBe(false)
    expect(overlaps(it_('a', 0, 0, 6, 4), it_('a', 0, 0, 6, 4))).toBe(false)
  })

  it('clamps items into the grid', () => {
    expect(clampItem(it_('a', 10, -2, 6, 0))).toEqual(it_('a', 6, 0, 6, 1))
    expect(clampItem(it_('a', 0, 0, 20, 99))).toEqual(it_('a', 0, 0, GRID_COLS, MAX_H))
    expect(clampItem({ id: 'a', x: Number.NaN, y: 1.6, w: 2.4, h: 3 })).toEqual(it_('a', 0, 2, 2, 3))
  })

  it('orders by row then column', () => {
    expect(readingOrder([it_('c', 0, 4, 6, 4), it_('b', 6, 0, 6, 4), it_('a', 0, 0, 6, 4)]).map((x) => x.id)).toEqual(['a', 'b', 'c'])
  })

  it('pushes items down out of the way (cascading) and keeps the order', () => {
    const items = [it_('a', 0, 0, 6, 4), it_('b', 0, 4, 6, 4), it_('c', 0, 8, 12, 2)]
    const out = moveItem(items, 'c', 0, 0)
    expect(out.map((x) => x.id)).toEqual(['a', 'b', 'c'])
    expect(out.find((x) => x.id === 'c')).toEqual(it_('c', 0, 0, 12, 2))
    expect(out.find((x) => x.id === 'a')!.y).toBe(2)
    expect(out.find((x) => x.id === 'b')!.y).toBe(6)
    expect(anyOverlap(out)).toBe(false)
  })

  it('swaps vertically neighbouring items when moved up', () => {
    const out = moveItem([it_('a', 0, 0, 6, 4), it_('b', 0, 4, 6, 4)], 'b', 0, 0)
    expect(out).toEqual([it_('a', 0, 4, 6, 4), it_('b', 0, 0, 6, 4)])
  })

  it('leaves unrelated items and gaps alone', () => {
    const items = [it_('a', 0, 0, 6, 4), it_('b', 6, 10, 6, 4)]
    expect(moveItem(items, 'a', 1, 2)).toEqual([it_('a', 1, 2, 6, 4), it_('b', 6, 10, 6, 4)])
  })

  it('moves within the columns', () => {
    expect(moveItem([it_('a', 0, 0, 6, 4)], 'a', 9, -3)).toEqual([it_('a', 6, 0, 6, 4)])
  })

  it('resizes and pushes', () => {
    const out = resizeItem([it_('a', 0, 0, 6, 4), it_('b', 6, 0, 6, 4)], 'a', 8, 5)
    expect(out[0]).toEqual(it_('a', 0, 0, 8, 5))
    expect(out[1]).toEqual(it_('b', 6, 5, 6, 4))
    // never wider than the columns right of x
    expect(resizeItem([it_('a', 8, 0, 4, 4)], 'a', 12, 4)[0].w).toBe(4)
  })

  it('resolves an imported layout with overlaps', () => {
    const out = resolve([it_('a', 0, 0, 12, 4), it_('b', 0, 0, 12, 4), it_('c', 3, 2, 3, 3)])
    expect(anyOverlap(out)).toBe(false)
    expect(out[0].y).toBe(0)
  })

  it('tidies the gaps', () => {
    const out = tidy([it_('a', 0, 3, 6, 4), it_('b', 6, 10, 6, 4), it_('c', 0, 20, 12, 2)])
    expect(out).toEqual([it_('a', 0, 0, 6, 4), it_('b', 6, 0, 6, 4), it_('c', 0, 4, 12, 2)])
  })

  it('finds a free spot', () => {
    expect(freeSpot([], 6, 4)).toEqual({ x: 0, y: 0 })
    expect(freeSpot([it_('a', 0, 0, 6, 4)], 6, 4)).toEqual({ x: 6, y: 0 })
    expect(freeSpot([it_('a', 0, 0, 6, 4), it_('b', 6, 0, 6, 4)], 6, 4)).toEqual({ x: 0, y: 4 })
    expect(freeSpot([it_('a', 0, 0, 12, 4)], 20, 2)).toEqual({ x: 0, y: 4 })
    expect(gridRows([it_('a', 0, 2, 12, 4)])).toBe(6)
  })

  it('maps keys to changes', () => {
    const a = it_('a', 3, 2, 6, 4)
    expect(keyChange(a, 'ArrowLeft', 'move')).toEqual({ x: 2 })
    expect(keyChange(a, 'ArrowRight', 'move')).toEqual({ x: 4 })
    expect(keyChange(it_('a', 6, 0, 6, 4), 'ArrowRight', 'move')).toEqual({ x: 6 })
    expect(keyChange(a, 'ArrowUp', 'move')).toEqual({ y: 1 })
    expect(keyChange(it_('a', 0, 0, 6, 4), 'ArrowUp', 'move')).toEqual({ y: 0 })
    expect(keyChange(a, 'ArrowDown', 'move')).toEqual({ y: 3 })
    expect(keyChange(a, 'Home', 'move')).toEqual({ x: 0 })
    expect(keyChange(a, 'End', 'move')).toEqual({ x: 6 })
    expect(keyChange(a, 'ArrowLeft', 'resize')).toEqual({ w: 5 })
    expect(keyChange(a, 'ArrowRight', 'resize')).toEqual({ w: 7 })
    expect(keyChange(it_('a', 0, 0, 12, 4), 'ArrowRight', 'resize')).toEqual({ w: 12 })
    expect(keyChange(a, 'ArrowUp', 'resize')).toEqual({ h: 3 })
    expect(keyChange(it_('a', 0, 0, 1, 1), 'ArrowUp', 'resize')).toEqual({ h: 1 })
    expect(keyChange(a, 'ArrowDown', 'resize')).toEqual({ h: 5 })
    expect(keyChange(a, 'Home', 'resize')).toEqual({ w: 1 })
    expect(keyChange(a, 'End', 'resize')).toEqual({ w: 9 })
    expect(keyChange(a, 'a', 'move')).toBeNull()
    expect(keyChange(a, 'Tab', 'resize')).toBeNull()
  })

  it('applies keys to the layout', () => {
    const items = [it_('a', 0, 0, 6, 4), it_('b', 6, 0, 6, 4)]
    expect(applyKey(items, 'a', 'ArrowRight', 'move')).toEqual([it_('a', 1, 0, 6, 4), it_('b', 6, 4, 6, 4)])
    expect(applyKey(items, 'a', 'ArrowDown', 'resize')).toEqual([it_('a', 0, 0, 6, 5), it_('b', 6, 0, 6, 4)])
    expect(applyKey(items, 'x', 'ArrowDown', 'resize')).toBeNull()
    expect(applyKey(items, 'a', 'q', 'resize')).toBeNull()
  })

  it('finds the cell under the pointer', () => {
    expect(cellAt(250, 190, 100, 96)).toEqual({ x: 2, y: 1 })
    expect(cellAt(250, 190, 100, 96, 1, 1)).toEqual({ x: 1, y: 0 })
    expect(cellAt(10, 10, 0, 0)).toEqual({ x: 10, y: 10 })
  })

  it('compares layouts', () => {
    expect(sameLayout([it_('a', 0, 0, 1, 1)], [it_('a', 0, 0, 1, 1)])).toBe(true)
    expect(sameLayout([it_('a', 0, 0, 1, 1)], [it_('a', 0, 1, 1, 1)])).toBe(false)
    expect(sameLayout([it_('a', 0, 0, 1, 1)], [])).toBe(false)
    expect(sameLayout([it_('a', 0, 0, 1, 1)], [it_('b', 0, 0, 1, 1)])).toBe(false)
  })
})
