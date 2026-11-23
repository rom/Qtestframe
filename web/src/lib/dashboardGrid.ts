// Layout logic of the dashboard grid (components/dashboard/DashboardGrid.vue):
// widgets are rectangles on a grid of GRID_COLS columns and rows of equal
// height. Placement is free (gaps stay where the user leaves them); a widget
// that is moved or resized onto others pushes them down. tidy() closes the
// gaps on request. All functions are pure and return new arrays.

export const GRID_COLS = 12
export const MAX_H = 24
export const MAX_ROWS = 400

export interface GridItem {
  id: string
  x: number
  y: number
  w: number
  h: number
}

const int = (n: unknown, d: number) => (typeof n === 'number' && Number.isFinite(n) ? Math.round(n) : d)

/** Whether two items share a cell. */
export function overlaps(a: GridItem, b: GridItem): boolean {
  return a.id !== b.id && a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h
}

/** An item kept within the grid: 1 ≤ w ≤ cols, 0 ≤ x ≤ cols − w, 1 ≤ h ≤ MAX_H, 0 ≤ y ≤ MAX_ROWS. */
export function clampItem<T extends GridItem>(it: T, cols = GRID_COLS): T {
  const w = Math.min(cols, Math.max(1, int(it.w, 1)))
  const h = Math.min(MAX_H, Math.max(1, int(it.h, 1)))
  const x = Math.min(cols - w, Math.max(0, int(it.x, 0)))
  const y = Math.min(MAX_ROWS, Math.max(0, int(it.y, 0)))
  return { ...it, x, y, w, h }
}

/** Reading order: by row, then column (the order of the widgets for keyboards and screen readers, and on phones). */
export function readingOrder<T extends GridItem>(items: readonly T[]): T[] {
  return [...items].sort((a, b) => a.y - b.y || a.x - b.x || a.id.localeCompare(b.id))
}

/**
 * Resolves overlaps: the item fixedId (if any) stays where it is, every
 * other item that overlaps one already placed moves down below it (in
 * reading order, so pushes cascade). Items without overlaps keep their place.
 */
export function resolve<T extends GridItem>(items: readonly T[], fixedId?: string, cols = GRID_COLS): T[] {
  const all = items.map((it) => clampItem(it, cols))
  const fixed = all.find((it) => it.id === fixedId)
  const placed: T[] = fixed ? [fixed] : []
  for (const it of readingOrder(all.filter((x) => x !== fixed))) {
    const cur = { ...it }
    for (;;) {
      const hit = placed.filter((p) => overlaps(cur, p))
      if (!hit.length) break
      cur.y = Math.max(...hit.map((p) => p.y + p.h))
    }
    placed.push(cur)
  }
  // keep the caller's order
  const byId = new Map(placed.map((p) => [p.id, p]))
  return all.map((it) => byId.get(it.id) as T)
}

/** Moves an item to (x, y); the items in the way move down. */
export function moveItem<T extends GridItem>(items: readonly T[], id: string, x: number, y: number, cols = GRID_COLS): T[] {
  return resolve(
    items.map((it) => (it.id === id ? clampItem({ ...it, x, y }, cols) : it)),
    id,
    cols,
  )
}

/** Resizes an item to w × h (kept within the grid); the items in the way move down. */
export function resizeItem<T extends GridItem>(items: readonly T[], id: string, w: number, h: number, cols = GRID_COLS): T[] {
  return resolve(
    items.map((it) => {
      if (it.id !== id) return it
      const ww = Math.min(cols - it.x, Math.max(1, Math.round(w)))
      return clampItem({ ...it, w: ww, h }, cols)
    }),
    id,
    cols,
  )
}

/** Closes the gaps: every item moves up as far as it can (in reading order). */
export function tidy<T extends GridItem>(items: readonly T[], cols = GRID_COLS): T[] {
  const placed: T[] = []
  for (const it of readingOrder(items.map((x) => clampItem(x, cols)))) {
    const cur = { ...it, y: 0 }
    for (;;) {
      const hit = placed.filter((p) => overlaps(cur, p))
      if (!hit.length) break
      cur.y = Math.max(...hit.map((p) => p.y + p.h))
    }
    placed.push(cur)
  }
  const byId = new Map(placed.map((p) => [p.id, p]))
  return items.map((it) => byId.get(it.id) as T)
}

/** The first free place for a w × h item (top to bottom, left to right). */
export function freeSpot(items: readonly GridItem[], w: number, h: number, cols = GRID_COLS): { x: number; y: number } {
  const ww = Math.min(cols, Math.max(1, w))
  const bottom = gridRows(items)
  for (let y = 0; y <= bottom; y++) {
    for (let x = 0; x + ww <= cols; x++) {
      const probe = { id: '\u0000', x, y, w: ww, h }
      if (!items.some((it) => overlaps(probe, it))) return { x, y }
    }
  }
  return { x: 0, y: bottom }
}

/** The number of rows the items take. */
export function gridRows(items: readonly GridItem[]): number {
  return items.reduce((n, it) => Math.max(n, it.y + it.h), 0)
}

export type KeyMode = 'move' | 'resize'

/**
 * The change of a key on a move or resize handle: arrows move by one cell
 * (resize: change the width or height by one), Home/End go to the first or
 * last column (resize: the narrowest and the full width). null: not a key
 * of the grid.
 */
export function keyChange(item: GridItem, key: string, mode: KeyMode, cols = GRID_COLS): Partial<GridItem> | null {
  if (mode === 'move') {
    switch (key) {
      case 'ArrowLeft':
        return { x: Math.max(0, item.x - 1) }
      case 'ArrowRight':
        return { x: Math.min(cols - item.w, item.x + 1) }
      case 'ArrowUp':
        return { y: Math.max(0, item.y - 1) }
      case 'ArrowDown':
        return { y: Math.min(MAX_ROWS, item.y + 1) }
      case 'Home':
        return { x: 0 }
      case 'End':
        return { x: cols - item.w }
    }
    return null
  }
  switch (key) {
    case 'ArrowLeft':
      return { w: Math.max(1, item.w - 1) }
    case 'ArrowRight':
      return { w: Math.min(cols - item.x, item.w + 1) }
    case 'ArrowUp':
      return { h: Math.max(1, item.h - 1) }
    case 'ArrowDown':
      return { h: Math.min(MAX_H, item.h + 1) }
    case 'Home':
      return { w: 1 }
    case 'End':
      return { w: cols - item.x }
  }
  return null
}

/** Applies a key on a handle to the layout (unchanged layout: the same array). */
export function applyKey<T extends GridItem>(items: readonly T[], id: string, key: string, mode: KeyMode, cols = GRID_COLS): T[] | null {
  const it = items.find((x) => x.id === id)
  if (!it) return null
  const ch = keyChange(it, key, mode, cols)
  if (!ch) return null
  const next = { ...it, ...ch }
  return mode === 'move' ? moveItem(items, id, next.x, next.y, cols) : resizeItem(items, id, next.w, next.h, cols)
}

/**
 * The grid cell under a point of the grid element (pointer dragging):
 * px, py relative to the grid's top left, colWidth and rowHeight include
 * the gap. The cell of the item's top left is the pointer's cell minus
 * where the item was grabbed (grabX, grabY in cells).
 */
export function cellAt(px: number, py: number, colWidth: number, rowHeight: number, grabX = 0, grabY = 0): { x: number; y: number } {
  return {
    x: Math.floor(px / Math.max(1, colWidth)) - grabX,
    y: Math.floor(py / Math.max(1, rowHeight)) - grabY,
  }
}

/** Whether two layouts place every item alike. */
export function sameLayout(a: readonly GridItem[], b: readonly GridItem[]): boolean {
  if (a.length !== b.length) return false
  const m = new Map(b.map((it) => [it.id, it]))
  return a.every((it) => {
    const o = m.get(it.id)
    return !!o && o.x === it.x && o.y === it.y && o.w === it.w && o.h === it.h
  })
}
