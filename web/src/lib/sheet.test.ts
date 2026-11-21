import { describe, expect, it } from 'vitest'
import {
  afterCommit,
  clampCell,
  copyBlock,
  editKey,
  inRange,
  isHeaderRow,
  matchOption,
  navigate,
  parseTSV,
  planClear,
  planFillDown,
  planPaste,
  rangeOf,
  rangeSize,
  sortRows,
  splitList,
  toBatch,
  toTSV,
  tsvCell,
  type PlanContext,
  type SheetField,
} from './sheet'

const D = { rows: 10, cols: 5, page: 4 }
const at = (r: number, c: number) => ({ r, c })

describe('keyboard navigation', () => {
  it('moves with the arrows and stops at the edges', () => {
    expect(navigate({ key: 'ArrowDown' }, at(0, 0), D)).toEqual({ cell: at(1, 0), extend: false })
    expect(navigate({ key: 'ArrowRight' }, at(0, 0), D)?.cell).toEqual(at(0, 1))
    expect(navigate({ key: 'ArrowUp' }, at(0, 2), D)?.cell).toEqual(at(0, 2))
    expect(navigate({ key: 'ArrowLeft' }, at(3, 0), D)?.cell).toEqual(at(3, 0))
    expect(navigate({ key: 'ArrowDown' }, at(9, 0), D)?.cell).toEqual(at(9, 0))
  })
  it('jumps to the edge with Ctrl or Cmd', () => {
    expect(navigate({ key: 'ArrowDown', ctrlKey: true }, at(2, 3), D)?.cell).toEqual(at(9, 3))
    expect(navigate({ key: 'ArrowLeft', metaKey: true }, at(2, 3), D)?.cell).toEqual(at(2, 0))
    expect(navigate({ key: 'ArrowUp', ctrlKey: true }, at(2, 3), D)?.cell).toEqual(at(0, 3))
    expect(navigate({ key: 'ArrowRight', ctrlKey: true }, at(2, 1), D)?.cell).toEqual(at(2, 4))
  })
  it('goes to the row ends with Home and End, to the sheet ends with Ctrl', () => {
    expect(navigate({ key: 'Home' }, at(4, 3), D)?.cell).toEqual(at(4, 0))
    expect(navigate({ key: 'End' }, at(4, 1), D)?.cell).toEqual(at(4, 4))
    expect(navigate({ key: 'Home', ctrlKey: true }, at(4, 3), D)?.cell).toEqual(at(0, 0))
    expect(navigate({ key: 'End', ctrlKey: true }, at(4, 1), D)?.cell).toEqual(at(9, 4))
  })
  it('pages up and down', () => {
    expect(navigate({ key: 'PageDown' }, at(1, 2), D)?.cell).toEqual(at(5, 2))
    expect(navigate({ key: 'PageDown' }, at(8, 2), D)?.cell).toEqual(at(9, 2))
    expect(navigate({ key: 'PageUp' }, at(2, 2), D)?.cell).toEqual(at(0, 2))
    expect(navigate({ key: 'PageDown' }, at(0, 0), { ...D, page: 0 })?.cell).toEqual(at(1, 0))
  })
  it('extends the selection with Shift', () => {
    expect(navigate({ key: 'ArrowDown', shiftKey: true }, at(0, 0), D)).toEqual({ cell: at(1, 0), extend: true })
    expect(navigate({ key: 'End', shiftKey: true, ctrlKey: true }, at(0, 0), D)).toEqual({ cell: at(9, 4), extend: true })
  })
  it('tabs right and left, wrapping rows, and lets focus leave at the ends', () => {
    expect(navigate({ key: 'Tab' }, at(0, 3), D)).toEqual({ cell: at(0, 4), extend: false })
    expect(navigate({ key: 'Tab' }, at(0, 4), D)?.cell).toEqual(at(1, 0))
    expect(navigate({ key: 'Tab', shiftKey: true }, at(1, 0), D)).toEqual({ cell: at(0, 4), extend: false })
    expect(navigate({ key: 'Tab' }, at(9, 4), D)).toBeNull()
    expect(navigate({ key: 'Tab', shiftKey: true }, at(0, 0), D)).toBeNull()
    expect(navigate({ key: 'Tab', ctrlKey: true }, at(0, 0), D)).toBeNull()
  })
  it('ignores other keys, Alt and an empty sheet', () => {
    expect(navigate({ key: 'a' }, at(0, 0), D)).toBeNull()
    expect(navigate({ key: 'ArrowDown', altKey: true }, at(0, 0), D)).toBeNull()
    expect(navigate({ key: 'ArrowDown' }, at(0, 0), { rows: 0, cols: 3, page: 1 })).toBeNull()
  })
  it('moves after a commit with Enter, Shift+Enter, Tab and Shift+Tab', () => {
    expect(afterCommit('enter', at(2, 2), D)).toEqual(at(3, 2))
    expect(afterCommit('shift-enter', at(2, 2), D)).toEqual(at(1, 2))
    expect(afterCommit('tab', at(2, 4), D)).toEqual(at(3, 0))
    expect(afterCommit('shift-tab', at(2, 0), D)).toEqual(at(1, 4))
    expect(afterCommit('tab', at(9, 4), D)).toEqual(at(9, 4))
    expect(afterCommit('none', at(2, 2), D)).toEqual(at(2, 2))
  })
  it('starts editing with F2, Enter or a printable key', () => {
    expect(editKey({ key: 'F2' })).toBe('edit')
    expect(editKey({ key: 'Enter' })).toBe('edit')
    expect(editKey({ key: 'Enter', shiftKey: true })).toBeNull()
    expect(editKey({ key: 'x' })).toBe('type')
    expect(editKey({ key: 'Ä', shiftKey: true })).toBe('type')
    expect(editKey({ key: ' ' })).toBeNull()
    expect(editKey({ key: 'c', ctrlKey: true })).toBeNull()
    expect(editKey({ key: 'ArrowDown' })).toBeNull()
  })
})

describe('ranges', () => {
  it('normalises, tests and measures', () => {
    const g = rangeOf(at(3, 4), at(1, 2))
    expect(g).toEqual({ r1: 1, c1: 2, r2: 3, c2: 4 })
    expect(inRange(g, 2, 3)).toBe(true)
    expect(inRange(g, 0, 3)).toBe(false)
    expect(inRange(null, 0, 0)).toBe(false)
    expect(rangeSize(g)).toEqual({ rows: 3, cols: 3, cells: 9 })
    expect(clampCell(at(20, -1), 10, 5)).toEqual(at(9, 0))
  })
})

describe('TSV', () => {
  it('parses rows and cells as Excel copies them', () => {
    expect(parseTSV('a\tb\r\nc\td\r\n')).toEqual([
      ['a', 'b'],
      ['c', 'd'],
    ])
    expect(parseTSV('a\tb\nc\td')).toEqual([
      ['a', 'b'],
      ['c', 'd'],
    ])
    expect(parseTSV('one')).toEqual([['one']])
    expect(parseTSV('')).toEqual([])
    expect(parseTSV('a\t\tc')).toEqual([['a', '', 'c']])
    expect(parseTSV('a\t')).toEqual([['a', '']])
  })
  it('parses quoted cells with tabs, line breaks and quotes', () => {
    expect(parseTSV('"line 1\nline 2"\tx\r\n"say ""hi"""\t"a\tb"')).toEqual([
      ['line 1\nline 2', 'x'],
      ['say "hi"', 'a\tb'],
    ])
    // a quote inside a cell is literal
    expect(parseTSV('5" screen\tx')).toEqual([['5" screen', 'x']])
  })
  it('keeps an empty row in the middle', () => {
    expect(parseTSV('a\n\nb\n')).toEqual([['a'], [''], ['b']])
  })
  it('serialises and round-trips', () => {
    expect(tsvCell('plain')).toBe('plain')
    expect(tsvCell('a\tb')).toBe('"a\tb"')
    expect(tsvCell('"quoted"')).toBe('"""quoted"""')
    const rows = [
      ['Key', 'Description'],
      ['R-1', 'Line 1\nLine "2"'],
      ['R-2', ''],
    ]
    expect(toTSV(rows)).toBe('Key\tDescription\r\nR-1\t"Line 1\nLine ""2"""\r\nR-2\t')
    expect(parseTSV(toTSV(rows))).toEqual(rows)
  })
})

describe('helpers', () => {
  it('matches options by value or label', () => {
    const opts = [
      { value: 'non_functional', label: 'Non-functional' },
      { value: 'high', label: 'Hög' },
    ]
    expect(matchOption(opts, 'non functional')?.value).toBe('non_functional')
    expect(matchOption(opts, ' NON-FUNCTIONAL ')?.value).toBe('non_functional')
    expect(matchOption(opts, 'hög')?.value).toBe('high')
    expect(matchOption(opts, 'urgent')).toBeNull()
    expect(matchOption(opts, '  ')).toBeNull()
  })
  it('splits lists', () => {
    expect(splitList('a, b;c\nA, ,d')).toEqual(['a', 'b', 'c', 'd'])
    expect(splitList('')).toEqual([])
  })
})

// ---------- planning ----------

interface Row {
  id: string
  key: string
  title: string
  priority: string
  tags: string[]
  version: number
}
const PRIO = ['critical', 'high', 'medium', 'low']
const fields: SheetField<Row>[] = [
  { id: 'key', header: 'Key', width: 100, editor: 'none', text: (r) => r.key },
  {
    id: 'title',
    header: 'Title',
    width: 200,
    editor: 'text',
    text: (r) => r.title,
    parse: (s) => (s.trim() ? { ok: true, value: s.trim() } : { ok: false, error: 'Title is required' }),
  },
  {
    id: 'priority',
    header: 'Priority',
    width: 100,
    editor: 'select',
    text: (r) => r.priority,
    sortKey: (r) => PRIO.indexOf(r.priority),
    parse: (s) => (PRIO.includes(s.trim().toLowerCase()) ? { ok: true, value: s.trim().toLowerCase() } : { ok: false, error: `"${s}" is not a priority` }),
  },
  {
    id: 'tags',
    header: 'Tags',
    width: 100,
    editor: 'tags',
    clearable: true,
    text: (r) => r.tags.join(', '),
    value: (r) => r.tags,
    parse: (s) => ({ ok: true, value: splitList(s) }),
    editable: (r) => r?.key !== 'R-3',
  },
]
const rows: Row[] = [
  { id: 'a', key: 'R-1', title: 'One', priority: 'high', tags: [], version: 3 },
  { id: 'b', key: 'R-2', title: 'Two', priority: 'low', tags: ['x'], version: 1 },
  { id: 'c', key: 'R-3', title: 'Three', priority: 'medium', tags: ['y'], version: 2 },
]
const ctx: PlanContext<Row> = { rows, fields, readOnly: (f) => `${f.header} cannot be edited` }
const sel = (r1: number, c1: number, r2 = r1, c2 = c1) => ({ r1, c1, r2, c2 })

describe('paste', () => {
  it('pastes a 2×2 block from the active cell', () => {
    const p = planPaste(ctx, parseTSV('Uno\tcritical\r\nDos\tmedium\r\n'), sel(0, 1))
    expect(p.errors).toEqual([])
    expect(p.range).toEqual(sel(0, 1, 1, 2))
    expect(p.changes.map((c) => [c.row, c.field, c.value, c.before])).toEqual([
      [0, 'title', 'Uno', 'One'],
      [0, 'priority', 'critical', 'high'],
      [1, 'title', 'Dos', 'Two'],
      [1, 'priority', 'medium', 'low'],
    ])
  })
  it('refuses the whole paste when one enum value is invalid', () => {
    const p = planPaste(ctx, [
      ['Uno', 'critical'],
      ['Dos', 'urgent'],
    ], sel(0, 1))
    expect(p.changes).toEqual([])
    expect(p.errors).toEqual([{ row: 1, col: 2, message: '"urgent" is not a priority' }])
  })
  it('refuses a block that does not fit', () => {
    expect(planPaste(ctx, [['a'], ['b']], sel(2, 1)).refused).toBe('no_fit')
    expect(planPaste(ctx, [['a', 'b', 'c', 'd']], sel(0, 1)).refused).toBe('no_fit')
    expect(planPaste(ctx, [], sel(0, 1)).refused).toBe('empty')
  })
  it('fills the selection with a single value', () => {
    const p = planPaste(ctx, [['low']], sel(0, 2, 2, 2))
    expect(p.changes.map((c) => [c.row, c.value])).toEqual([
      [0, 'low'],
      [2, 'low'],
    ])
  })
  it('leaves out a header row and accepts unchanged read-only cells', () => {
    const p = planPaste(ctx, parseTSV('Key\tTitle\nR-1\tFirst\nR-2\tSecond'), sel(0, 0))
    expect(p.skippedHeader).toBe(true)
    expect(p.errors).toEqual([])
    expect(p.changes.map((c) => c.value)).toEqual(['First', 'Second'])
    expect(isHeaderRow(['key', 'TITLE'], fields, 0)).toBe(true)
    expect(isHeaderRow(['Title'], fields, 0)).toBe(false)
  })
  it('refuses a change to a read-only or not permitted cell', () => {
    expect(planPaste(ctx, [['R-9']], sel(0, 0)).errors[0].message).toBe('Key cannot be edited')
    expect(planPaste(ctx, [['z']], sel(2, 3)).errors[0].message).toBe('Tags cannot be edited')
  })
})

describe('fill down and clear', () => {
  it('fills the top value down the selection', () => {
    const p = planFillDown(ctx, sel(0, 1, 2, 2))
    expect(p.errors).toEqual([])
    expect(p.changes.map((c) => [c.row, c.field, c.value])).toEqual([
      [1, 'title', 'One'],
      [2, 'title', 'One'],
      [1, 'priority', 'high'],
      [2, 'priority', 'high'],
    ])
  })
  it('fills a single row from the row above and skips read-only columns', () => {
    const p = planFillDown(ctx, sel(1, 0, 1, 2))
    expect(p.changes.map((c) => [c.row, c.field, c.value])).toEqual([
      [1, 'title', 'One'],
      [1, 'priority', 'high'],
    ])
    expect(planFillDown(ctx, sel(0, 1)).refused).toBe('nothing')
  })
  it('refuses a fill over a cell the user may not edit', () => {
    const p = planFillDown(ctx, sel(1, 3, 2, 3))
    expect(p.changes).toEqual([])
    expect(p.errors).toEqual([{ row: 2, col: 3, message: 'Tags cannot be edited' }])
  })
  it('clears only clearable, editable cells', () => {
    const p = planClear(ctx, sel(0, 0, 2, 3))
    expect(p.changes.map((c) => [c.row, c.field, c.value])).toEqual([[1, 'tags', []]])
    expect(planClear(ctx, sel(0, 1)).refused).toBe('nothing')
  })
})

describe('copy, sort and batches', () => {
  it('copies a block with or without the headers', () => {
    expect(copyBlock(rows, fields, sel(0, 0, 1, 1), true)).toEqual([
      ['Key', 'Title'],
      ['R-1', 'One'],
      ['R-2', 'Two'],
    ])
    expect(toTSV(copyBlock(rows, fields, sel(1, 2, 1, 3), false))).toBe('low\tx')
  })
  it('sorts by a field, empty values last', () => {
    expect(sortRows(rows, fields[2], 1).map((r) => r.key)).toEqual(['R-1', 'R-3', 'R-2'])
    expect(sortRows(rows, fields[2], -1).map((r) => r.key)).toEqual(['R-2', 'R-3', 'R-1'])
    expect(sortRows(rows, fields[3], 1).map((r) => r.key)).toEqual(['R-2', 'R-3', 'R-1'])
    expect(sortRows(rows, undefined, 1)).toEqual(rows)
    const keys = [{ ...rows[0], key: 'R-10' }, { ...rows[0], key: 'R-9' }]
    expect(sortRows(keys, fields[0], 1).map((r) => r.key)).toEqual(['R-9', 'R-10'])
  })
  it('groups changes per item with versions and old values', () => {
    const p = planPaste(ctx, parseTSV('Uno\tcritical\nDos\tmedium'), sel(0, 1))
    const b = toBatch(rows, p.changes)
    expect(b.items).toEqual([
      { id: 'a', version: 3, set: { title: 'Uno', priority: 'critical' } },
      { id: 'b', version: 1, set: { title: 'Dos', priority: 'medium' } },
    ])
    expect(b.before).toEqual([
      { id: 'a', set: { title: 'One', priority: 'high' } },
      { id: 'b', set: { title: 'Two', priority: 'low' } },
    ])
    expect(toBatch(rows, p.changes, false).items[0].version).toBeUndefined()
  })
})
