// The spreadsheet view's logic, free of Vue: cell ranges, keyboard
// navigation, the clipboard format (tab-separated values, as Excel and
// LibreOffice copy and paste them) and the planning of edits (a paste, a
// fill-down, clearing cells) with validation, so a refused paste writes
// nothing. See components/SpreadsheetGrid.vue and ItemSheet.vue.

// ---------------------------------------------------------------- cells and ranges

export interface Cell {
  /** row index among the data rows (the "new item" row is rows.length) */
  r: number
  /** column index among the visible columns */
  c: number
}

/** A rectangle of cells, inclusive and normalised (r1 ≤ r2, c1 ≤ c2). */
export interface Range {
  r1: number
  c1: number
  r2: number
  c2: number
}

export function rangeOf(a: Cell, b: Cell): Range {
  return { r1: Math.min(a.r, b.r), c1: Math.min(a.c, b.c), r2: Math.max(a.r, b.r), c2: Math.max(a.c, b.c) }
}

export function inRange(g: Range | null | undefined, r: number, c: number): boolean {
  return !!g && r >= g.r1 && r <= g.r2 && c >= g.c1 && c <= g.c2
}

export function rangeSize(g: Range): { rows: number; cols: number; cells: number } {
  const rows = g.r2 - g.r1 + 1
  const cols = g.c2 - g.c1 + 1
  return { rows, cols, cells: rows * cols }
}

/** A cell kept inside the sheet (an empty sheet keeps 0,0). */
export function clampCell(cell: Cell, rows: number, cols: number): Cell {
  return { r: Math.max(0, Math.min(rows - 1, cell.r)), c: Math.max(0, Math.min(cols - 1, cell.c)) }
}

// ---------------------------------------------------------------- keyboard navigation

export interface NavKey {
  key: string
  shiftKey?: boolean
  ctrlKey?: boolean
  metaKey?: boolean
  altKey?: boolean
}

export interface Dims {
  rows: number
  cols: number
  /** rows per page (Page Up / Page Down) */
  page: number
}

export interface Move {
  cell: Cell
  /** extend the selection from its anchor (Shift) instead of collapsing it */
  extend: boolean
}

/**
 * Where a navigation key moves the active cell, or null when the key is not
 * a navigation key here (Tab on the last cell and Shift+Tab on the first
 * return null, so focus can leave the grid). Enter is not handled here: on
 * an editable cell it starts editing (see editKey).
 *
 * Arrows move by one cell, Ctrl/Cmd+arrow to the edge; Home and End go to
 * the first and last column of the row, Ctrl/Cmd+Home and +End to the
 * first and last cell; Page Up and Page Down move by a page. Shift extends
 * the selection. Tab and Shift+Tab move right and left and wrap to the next
 * and previous row.
 */
export function navigate(e: NavKey, cell: Cell, d: Dims): Move | null {
  if (d.rows <= 0 || d.cols <= 0 || e.altKey) return null
  const ctrl = !!(e.ctrlKey || e.metaKey)
  const extend = !!e.shiftKey
  const lastR = d.rows - 1
  const lastC = d.cols - 1
  const at = (r: number, c: number, ext = extend): Move => ({ cell: clampCell({ r, c }, d.rows, d.cols), extend: ext })
  switch (e.key) {
    case 'ArrowUp':
      return at(ctrl ? 0 : cell.r - 1, cell.c)
    case 'ArrowDown':
      return at(ctrl ? lastR : cell.r + 1, cell.c)
    case 'ArrowLeft':
      return at(cell.r, ctrl ? 0 : cell.c - 1)
    case 'ArrowRight':
      return at(cell.r, ctrl ? lastC : cell.c + 1)
    case 'Home':
      return ctrl ? at(0, 0) : at(cell.r, 0)
    case 'End':
      return ctrl ? at(lastR, lastC) : at(cell.r, lastC)
    case 'PageUp':
      return at(cell.r - Math.max(1, d.page), cell.c)
    case 'PageDown':
      return at(cell.r + Math.max(1, d.page), cell.c)
    case 'Tab':
      if (ctrl) return null
      if (e.shiftKey) {
        if (cell.c > 0) return at(cell.r, cell.c - 1, false)
        return cell.r > 0 ? at(cell.r - 1, lastC, false) : null
      }
      if (cell.c < lastC) return at(cell.r, cell.c + 1, false)
      return cell.r < lastR ? at(cell.r + 1, 0, false) : null
    default:
      return null
  }
}

/** The cell after committing an edit with Enter (down), Shift+Enter (up), Tab or Shift+Tab. */
export function afterCommit(how: 'enter' | 'shift-enter' | 'tab' | 'shift-tab' | 'none', cell: Cell, d: Dims): Cell {
  switch (how) {
    case 'enter':
      return clampCell({ r: cell.r + 1, c: cell.c }, d.rows, d.cols)
    case 'shift-enter':
      return clampCell({ r: cell.r - 1, c: cell.c }, d.rows, d.cols)
    case 'tab':
    case 'shift-tab':
      return navigate({ key: 'Tab', shiftKey: how === 'shift-tab' }, cell, d)?.cell ?? cell
    default:
      return cell
  }
}

/**
 * Whether a key starts editing the active cell: F2 and Enter edit with the
 * current value, a printable character replaces it (typing over the cell,
 * as in a spreadsheet). Keys with Ctrl, Cmd or Alt never do.
 */
export function editKey(e: NavKey): 'edit' | 'type' | null {
  if (e.ctrlKey || e.metaKey || e.altKey) return null
  if (e.key === 'F2' || (e.key === 'Enter' && !e.shiftKey)) return 'edit'
  if (e.key.length === 1 && e.key !== ' ') return 'type'
  return null
}

// ---------------------------------------------------------------- clipboard (TSV)

/**
 * Parse tab-separated values as spreadsheets put them on the clipboard:
 * rows end with CRLF or LF (a trailing line break ends the last row, it does
 * not add an empty one), cells are separated by tabs, and a cell in double
 * quotes may hold tabs, line breaks and doubled quotes ("").
 */
export function parseTSV(text: string): string[][] {
  const rows: string[][] = []
  if (!text) return rows
  let row: string[] = []
  let cell = ''
  let i = 0
  let quoted = false
  let atCellStart = true
  const n = text.length
  while (i < n) {
    const ch = text[i]
    if (quoted) {
      if (ch === '"') {
        if (text[i + 1] === '"') {
          cell += '"'
          i += 2
          continue
        }
        quoted = false
        i++
        continue
      }
      cell += ch
      i++
      continue
    }
    if (ch === '"' && atCellStart) {
      quoted = true
      atCellStart = false
      i++
      continue
    }
    if (ch === '\t') {
      row.push(cell)
      cell = ''
      atCellStart = true
      i++
      continue
    }
    if (ch === '\r' || ch === '\n') {
      row.push(cell)
      rows.push(row)
      row = []
      cell = ''
      atCellStart = true
      i += ch === '\r' && text[i + 1] === '\n' ? 2 : 1
      continue
    }
    cell += ch
    atCellStart = false
    i++
  }
  if (!atCellStart || cell !== '' || row.length) {
    row.push(cell)
    rows.push(row)
  }
  return rows
}

/** One cell for the clipboard: quoted when it holds a tab, a line break or starts with a quote. */
export function tsvCell(v: string): string {
  if (/[\t\r\n]/.test(v) || v.startsWith('"')) return '"' + v.replace(/"/g, '""') + '"'
  return v
}

/** Rows as tab-separated values (CRLF between rows, as Excel writes them). */
export function toTSV(rows: readonly (readonly string[])[]): string {
  return rows.map((r) => r.map(tsvCell).join('\t')).join('\r\n')
}

// ---------------------------------------------------------------- fields

export type EditorKind = 'text' | 'textarea' | 'select' | 'tags' | 'categories' | 'none'

export interface SheetOption {
  value: string
  label: string
  disabled?: boolean
}

export type Parsed = { ok: true; value: unknown } | { ok: false; error: string }

/** A column of the sheet: how a cell shows, sorts, parses and saves. */
export interface SheetField<R = Record<string, unknown>> {
  id: string
  header: string
  editor: EditorKind
  /** default width in px */
  width: number
  /** cannot be hidden (key, title) */
  fixed?: boolean
  /** shown as text (display, copy, export) */
  text(row: R): string
  /** sort key; defaults to the text */
  sortKey?(row: R): string | number
  /** the saved value (API form) */
  value?(row: R): unknown
  /** typed or pasted text to the API value */
  parse?(text: string, row: R | null): Parsed
  /** choices of a select */
  options?(row: R | null): SheetOption[]
  /** whether the cell can be edited; null row means the "new item" row */
  editable?(row: R | null): boolean
  /** Delete / Backspace may empty it */
  clearable?: boolean
  /** the API field when it differs from id */
  api?: string
}

export const fieldApi = (f: SheetField<never>): string => f.api ?? f.id

export function isEditable<R>(f: SheetField<R>, row: R | null): boolean {
  return f.editor !== 'none' && !!f.parse && (f.editable ? f.editable(row) : true)
}

const norm = (s: string) => s.trim().toLowerCase().replace(/[\s_-]+/g, ' ')

/** An option for typed or pasted text: its value or label, ignoring case, spaces, _ and -. */
export function matchOption(options: readonly SheetOption[], text: string): SheetOption | null {
  const want = norm(text)
  if (!want) return null
  return options.find((o) => norm(o.value) === want) ?? options.find((o) => norm(o.label) === want) ?? null
}

/** Split a list cell ("a, b; c" or one per line) into trimmed, distinct values. */
export function splitList(text: string): string[] {
  const out: string[] = []
  const seen = new Set<string>()
  for (const raw of text.split(/[,;\n]/)) {
    const v = raw.trim()
    if (v && !seen.has(v.toLowerCase())) {
      seen.add(v.toLowerCase())
      out.push(v)
    }
  }
  return out
}

// ---------------------------------------------------------------- sorting

export type SortDir = 1 | -1

const collator = typeof Intl !== 'undefined' ? new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' }) : null

export function compareKeys(a: string | number, b: string | number): number {
  if (typeof a === 'number' && typeof b === 'number') return a - b
  const x = String(a)
  const y = String(b)
  // empty cells last whatever the direction is handled by the caller
  return collator ? collator.compare(x, y) : x < y ? -1 : x > y ? 1 : 0
}

/** Rows sorted by a field (stable; empty values last). */
export function sortRows<R>(rows: readonly R[], field: SheetField<R> | undefined, dir: SortDir): R[] {
  if (!field) return [...rows]
  const key = (r: R) => (field.sortKey ? field.sortKey(r) : field.text(r))
  const keyed = rows.map((r, i) => ({ r, i, k: key(r) }))
  keyed.sort((a, b) => {
    const ea = a.k === '' || a.k === undefined || a.k === null
    const eb = b.k === '' || b.k === undefined || b.k === null
    if (ea !== eb) return ea ? 1 : -1
    return compareKeys(a.k, b.k) * dir || a.i - b.i
  })
  return keyed.map((x) => x.r)
}

// ---------------------------------------------------------------- planning edits

export interface Change {
  row: number
  col: number
  field: string
  value: unknown
  before: unknown
}

export interface CellError {
  row: number
  col: number
  message: string
}

export interface Plan {
  changes: Change[]
  errors: CellError[]
  /** the cells the plan covers (to select them afterwards) */
  range: Range | null
  /** the pasted block's first row was the column headers and was left out */
  skippedHeader?: boolean
  /** a reason the whole operation was refused (beyond cell errors) */
  refused?: 'no_fit' | 'empty' | 'nothing'
}

export interface PlanContext<R> {
  rows: readonly R[]
  fields: readonly SheetField<R>[]
  /** message for a cell that cannot be edited */
  readOnly(field: SheetField<R>, row: R): string
}

const same = (a: unknown, b: unknown) => JSON.stringify(a ?? null) === JSON.stringify(b ?? null)

const valueOf = <R>(f: SheetField<R>, row: R): unknown => (f.value ? f.value(row) : (row as Record<string, unknown>)[fieldApi(f as SheetField<never>)])

/** One cell set from text: a change, nothing (unchanged) or an error. */
export function planCell<R>(ctx: PlanContext<R>, r: number, c: number, text: string): { change?: Change; error?: CellError } {
  const row = ctx.rows[r]
  const f = ctx.fields[c]
  if (!row || !f) return {}
  if (!isEditable(f, row)) {
    // pasting the same text over a read-only cell is fine (a copied block that includes the key)
    if (text.trim() === f.text(row).trim()) return {}
    return { error: { row: r, col: c, message: ctx.readOnly(f, row) } }
  }
  const p = f.parse!(text, row)
  if (!p.ok) return { error: { row: r, col: c, message: p.error } }
  const before = valueOf(f, row)
  if (same(p.value, before)) return {}
  return { change: { row: r, col: c, field: fieldApi(f as SheetField<never>), value: p.value, before } }
}

/** Whether the first pasted row is the headers of the columns it lands on. */
export function isHeaderRow<R>(block: readonly string[], fields: readonly SheetField<R>[], c1: number): boolean {
  if (!block.length) return false
  return block.every((h, i) => {
    const f = fields[c1 + i]
    return !!f && norm(h) === norm(f.header)
  })
}

/**
 * A paste of a block at the selection. A single value fills the whole
 * selection; a larger block lands at the selection's top-left cell and must
 * fit in the sheet. A first row that repeats the column headers is left out.
 * Any invalid cell refuses the whole paste (errors), so nothing is saved.
 */
export function planPaste<R>(ctx: PlanContext<R>, block: string[][], sel: Range): Plan {
  let data = block.filter((r, i) => !(i === block.length - 1 && r.length === 1 && r[0] === '' && block.length > 1))
  if (!data.length || (data.length === 1 && data[0].length === 0)) return { changes: [], errors: [], range: null, refused: 'empty' }
  let skippedHeader = false
  if (data.length > 1 && isHeaderRow(data[0], ctx.fields, sel.c1)) {
    data = data.slice(1)
    skippedHeader = true
  }
  const width = Math.max(...data.map((r) => r.length))
  const single = data.length === 1 && width === 1
  const target: Range = single ? sel : { r1: sel.r1, c1: sel.c1, r2: sel.r1 + data.length - 1, c2: sel.c1 + width - 1 }
  if (target.r2 >= ctx.rows.length || target.c2 >= ctx.fields.length) return { changes: [], errors: [], range: target, refused: 'no_fit', skippedHeader }
  const changes: Change[] = []
  const errors: CellError[] = []
  for (let r = target.r1; r <= target.r2; r++) {
    for (let c = target.c1; c <= target.c2; c++) {
      const text = single ? data[0][0] : (data[r - target.r1][c - target.c1] ?? '')
      const res = planCell(ctx, r, c, text)
      if (res.change) changes.push(res.change)
      if (res.error) errors.push(res.error)
    }
  }
  return { changes: errors.length ? [] : changes, errors, range: target, skippedHeader }
}

/**
 * Fill down (Ctrl+D): every selected cell takes the value of the top row of
 * the selection; with a single selected row, the row above. Columns that
 * cannot be edited are left alone; other refused cells refuse the fill.
 */
export function planFillDown<R>(ctx: PlanContext<R>, sel: Range): Plan {
  const src = sel.r1 === sel.r2 ? sel.r1 - 1 : sel.r1
  const first = sel.r1 === sel.r2 ? sel.r1 : sel.r1 + 1
  if (src < 0 || first > sel.r2 || sel.r2 >= ctx.rows.length) return { changes: [], errors: [], range: sel, refused: 'nothing' }
  const changes: Change[] = []
  const errors: CellError[] = []
  for (let c = sel.c1; c <= sel.c2; c++) {
    const f = ctx.fields[c]
    if (!f || f.editor === 'none' || !f.parse) continue
    const from = ctx.rows[src]
    for (let r = first; r <= sel.r2; r++) {
      const row = ctx.rows[r]
      if (!isEditable(f, row)) {
        errors.push({ row: r, col: c, message: ctx.readOnly(f, row) })
        continue
      }
      // the source's value as text through the parser: the same checks as typing it
      const p = f.parse(f.text(from), row)
      if (!p.ok) {
        errors.push({ row: r, col: c, message: p.error })
        continue
      }
      const before = valueOf(f, row)
      if (!same(p.value, before)) changes.push({ row: r, col: c, field: fieldApi(f as SheetField<never>), value: p.value, before })
    }
  }
  const range: Range = { ...sel, r1: src }
  if (!changes.length && !errors.length) return { changes, errors, range, refused: 'nothing' }
  return { changes: errors.length ? [] : changes, errors, range }
}

/** Delete / Backspace: empty the editable, clearable cells (others are skipped). */
export function planClear<R>(ctx: PlanContext<R>, sel: Range, empty: (f: SheetField<R>) => unknown = (f) => (f.editor === 'tags' || f.editor === 'categories' ? [] : '')): Plan {
  const changes: Change[] = []
  for (let r = sel.r1; r <= Math.min(sel.r2, ctx.rows.length - 1); r++) {
    for (let c = sel.c1; c <= sel.c2; c++) {
      const f = ctx.fields[c]
      const row = ctx.rows[r]
      if (!f || !f.clearable || !isEditable(f, row)) continue
      const before = valueOf(f, row)
      const value = empty(f)
      if (!same(value, before)) changes.push({ row: r, col: c, field: fieldApi(f as SheetField<never>), value, before })
    }
  }
  return { changes, errors: [], range: sel, refused: changes.length ? undefined : 'nothing' }
}

/** The selected block as text rows (for copying), optionally with the headers first. */
export function copyBlock<R>(rows: readonly R[], fields: readonly SheetField<R>[], sel: Range, headers: boolean): string[][] {
  const out: string[][] = []
  const cols = fields.slice(sel.c1, sel.c2 + 1)
  if (headers) out.push(cols.map((f) => f.header))
  for (let r = sel.r1; r <= Math.min(sel.r2, rows.length - 1); r++) out.push(cols.map((f) => f.text(rows[r])))
  return out
}

// ---------------------------------------------------------------- batches

export interface BatchItem {
  id: string
  version?: number
  set: Record<string, unknown>
}

/**
 * Changes grouped per item, as the batch endpoint takes them; `before`
 * holds the old values for undo. With `versions` each item carries the
 * version it was loaded at (a conflict check).
 */
export function toBatch<R extends { id: string; version?: number }>(rows: readonly R[], changes: readonly Change[], withVersions = true): { items: BatchItem[]; before: BatchItem[] } {
  const by = new Map<string, { item: BatchItem; before: BatchItem }>()
  for (const ch of changes) {
    const row = rows[ch.row]
    if (!row) continue
    let e = by.get(row.id)
    if (!e) {
      e = { item: { id: row.id, set: {} }, before: { id: row.id, set: {} } }
      if (withVersions && row.version) e.item.version = row.version
      by.set(row.id, e)
    }
    e.item.set[ch.field] = ch.value
    e.before.set[ch.field] = ch.before
  }
  const list = [...by.values()]
  return { items: list.map((x) => x.item), before: list.map((x) => x.before) }
}
