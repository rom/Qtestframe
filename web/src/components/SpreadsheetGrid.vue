<script setup lang="ts">
// A focused, in-house spreadsheet grid (WAI-ARIA grid pattern): one row per
// item, a frozen select column, a frozen first column and a sticky header
// row; only the rows in view are rendered (virtual scrolling), so thousands
// of rows stay smooth. The grid element keeps the focus and points at the
// active cell with aria-activedescendant. Edits, pastes, fill-downs and
// clears are planned with lib/sheet.ts and handed to the parent ("apply"),
// which saves them; the parent also owns the live region ("announce").
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { tagClass } from '@/lib/tagColors'
import { uid } from '@/lib/a11y'
import {
  afterCommit,
  clampCell,
  copyBlock,
  editKey,
  inRange,
  isEditable,
  navigate,
  parseTSV,
  planCell,
  planClear,
  planFillDown,
  planPaste,
  rangeOf,
  rangeSize,
  toTSV,
  type Cell,
  type Plan,
  type PlanContext,
  type Range,
  type SheetField,
  type SheetOption,
} from '@/lib/sheet'
import { emptyValue, type SheetRow } from '@/lib/sheetFields'

export type CellState = { state: 'pending' | 'saved' | 'error'; message?: string }
export type ApplyKind = 'edit' | 'paste' | 'fill' | 'clear' | 'cut'

const props = defineProps<{
  rows: SheetRow[]
  fields: SheetField<SheetRow>[]
  widths: Record<string, number>
  label: string
  /** the "new item" row at the bottom (placeholder text); none without */
  newRow?: string
  /** texts typed into the new row so far, by field id */
  draft?: Record<string, string>
  cellStates: Map<string, CellState>
  conflicts: Set<string>
  selectedIds: Set<string>
  sort: { field: string; dir: 1 | -1 } | null
  copyHeaders: boolean
  rowColor?: (row: SheetRow) => string
  readOnlyMessage: (field: SheetField<SheetRow>, row: SheetRow | null) => string
}>()
const emit = defineEmits<{
  apply: [plan: Plan, kind: ApplyKind]
  draft: [field: string, value: unknown, text: string]
  announce: [message: string, urgent?: boolean]
  selectRows: [ids: string[], on: boolean]
  sort: [field: string]
  resize: [field: string, width: number]
  open: [id: string]
}>()
const { t } = useI18n()

const ROW_H = 34
const HEAD_H = 38
const SEL_W = 40
const OVERSCAN = 8
const gid = uid('sheet')

// ---------------------------------------------------------------- geometry
const width = (f: SheetField<SheetRow>) => props.widths[f.id] ?? f.width
const template = computed(() => [SEL_W + 'px', ...props.fields.map((f) => width(f) + 'px')].join(' '))
const totalWidth = computed(() => SEL_W + props.fields.reduce((s, f) => s + width(f), 0))
const hasNew = computed(() => props.newRow !== undefined)
const total = computed(() => props.rows.length + (hasNew.value ? 1 : 0))
const dims = computed(() => ({ rows: total.value, cols: props.fields.length, page: pageRows.value }))

const scroller = ref<HTMLElement | null>(null)
const gridEl = ref<HTMLElement | null>(null)
const scrollTop = ref(0)
const viewH = ref(480)
const pageRows = computed(() => Math.max(1, Math.floor((viewH.value - HEAD_H) / ROW_H) - 1))
const windowRows = computed(() => {
  const first = Math.max(0, Math.floor((scrollTop.value - HEAD_H) / ROW_H) - OVERSCAN)
  const last = Math.min(total.value - 1, Math.ceil((scrollTop.value + viewH.value) / ROW_H) + OVERSCAN)
  const out: number[] = []
  for (let i = first; i <= last; i++) out.push(i)
  // the active row stays rendered (its cell is the active descendant)
  if (active.value.r < first || active.value.r > last) if (active.value.r < total.value) out.push(active.value.r)
  return out
})
let ro: ResizeObserver | null = null
function measure() {
  if (scroller.value) viewH.value = scroller.value.clientHeight || 480
}
onMounted(() => {
  measure()
  if (typeof ResizeObserver !== 'undefined' && scroller.value) {
    ro = new ResizeObserver(measure)
    ro.observe(scroller.value)
  }
  document.addEventListener('copy', onCopy)
  document.addEventListener('cut', onCut)
  document.addEventListener('paste', onPaste)
})
onBeforeUnmount(() => {
  ro?.disconnect()
  document.removeEventListener('copy', onCopy)
  document.removeEventListener('cut', onCut)
  document.removeEventListener('paste', onPaste)
})
function onScroll() {
  if (scroller.value) scrollTop.value = scroller.value.scrollTop
}

// ---------------------------------------------------------------- active cell and selection
const active = ref<Cell>({ r: 0, c: 0 })
const anchor = ref<Cell>({ r: 0, c: 0 })
const range = ref<Range>({ r1: 0, c1: 0, r2: 0, c2: 0 })
const focused = ref(false)
const cellId = (r: number, c: number) => `${gid}-r${r}-c${c}`
const rowAt = (r: number): SheetRow | null => props.rows[r] ?? null
const isNewRow = (r: number) => hasNew.value && r === props.rows.length
const activeId = computed(() => (total.value && props.fields.length && windowRows.value.includes(active.value.r) ? cellId(active.value.r, active.value.c) : undefined))

watch(
  () => [total.value, props.fields.length],
  () => {
    const a = clampCell(active.value, Math.max(1, total.value), Math.max(1, props.fields.length))
    if (a.r !== active.value.r || a.c !== active.value.c) setActive(a, false)
    else range.value = clampRange(range.value)
  },
)
function clampRange(g: Range): Range {
  const lr = Math.max(0, total.value - 1)
  const lc = Math.max(0, props.fields.length - 1)
  return { r1: Math.min(g.r1, lr), c1: Math.min(g.c1, lc), r2: Math.min(g.r2, lr), c2: Math.min(g.c2, lc) }
}
function setActive(cell: Cell, extend: boolean) {
  active.value = cell
  if (!extend) anchor.value = cell
  range.value = rangeOf(anchor.value, cell)
  scrollIntoView(cell)
}
/** the selection without the new row (operations on items) */
const dataRange = computed<Range | null>(() => {
  const g = range.value
  const last = props.rows.length - 1
  if (last < 0 || g.r1 > last) return null
  return { ...g, r2: Math.min(g.r2, last) }
})

function scrollIntoView(cell: Cell) {
  const el = scroller.value
  if (!el) return
  const top = HEAD_H + cell.r * ROW_H
  if (top - HEAD_H < el.scrollTop) el.scrollTop = top - HEAD_H
  else if (top + ROW_H > el.scrollTop + el.clientHeight) el.scrollTop = top + ROW_H - el.clientHeight
  // columns behind the frozen ones (select + first column) scroll into view
  const frozen = SEL_W + (props.fields[0] ? width(props.fields[0]) : 0)
  // the frozen first column: back to the start of the row, as Home does in a spreadsheet
  if (cell.c === 0) el.scrollLeft = 0
  else {
    let left = SEL_W
    for (let i = 0; i < cell.c; i++) left += width(props.fields[i])
    const right = left + width(props.fields[cell.c])
    if (left - frozen < el.scrollLeft) el.scrollLeft = left - frozen
    else if (right > el.scrollLeft + el.clientWidth) el.scrollLeft = right - el.clientWidth
  }
  scrollTop.value = el.scrollTop
}

/** Focus a cell from outside (e.g. the new row after an item was created). */
function focusCell(r: number, c: number) {
  setActive(clampCell({ r, c }, Math.max(1, total.value), Math.max(1, props.fields.length)), false)
  void nextTick(() => gridEl.value?.focus({ preventScroll: true }))
}
defineExpose({ focusCell })

// ---------------------------------------------------------------- cell content
const stateOf = (r: number, c: number): CellState | undefined => {
  const row = rowAt(r)
  const f = props.fields[c]
  return row && f ? props.cellStates.get(`${row.id}:${f.api ?? f.id}`) : undefined
}
function cellText(r: number, c: number): string {
  const f = props.fields[c]
  if (isNewRow(r)) return props.draft?.[f.id] ?? ''
  const row = rowAt(r)
  return row ? f.text(row) : ''
}
const canEdit = (r: number, c: number) => {
  const f = props.fields[c]
  if (!f) return false
  return isEditable(f, isNewRow(r) ? null : rowAt(r))
}
const tagsOf = (r: number, c: number) => (props.fields[c].editor === 'tags' ? cellText(r, c).split(', ').filter(Boolean) : [])

// ---------------------------------------------------------------- planning
const planCtx = computed<PlanContext<SheetRow>>(() => ({ rows: props.rows, fields: props.fields, readOnly: (f, row) => props.readOnlyMessage(f, row) }))

function refuse(plan: Plan): boolean {
  if (plan.refused === 'no_fit') {
    const room = { rows: props.rows.length - range.value.r1, cols: props.fields.length - range.value.c1 }
    emit('announce', t('sheet.err.noFit', { rows: Math.max(0, room.rows), cols: room.cols }), true)
    return true
  }
  if (plan.refused === 'empty') {
    emit('announce', t('sheet.err.clipboardEmpty'), true)
    return true
  }
  if (plan.errors.length) {
    const first = plan.errors.slice(0, 3).map((e) => t('sheet.cellError', { key: props.rows[e.row]?.key ?? '', field: props.fields[e.col]?.header ?? '', message: e.message }))
    const more = plan.errors.length > 3 ? ' ' + t('sheet.moreErrors', { n: plan.errors.length - 3 }, plan.errors.length - 3) : ''
    emit('announce', t('sheet.err.refused', plan.errors.length) + ' ' + first.join(' ') + more, true)
    return true
  }
  return false
}

// ---------------------------------------------------------------- editing
interface Editing {
  r: number
  c: number
  text: string
  /** typed over the cell: arrows commit and move */
  typed: boolean
  error: string
}
const editing = ref<Editing | null>(null)
const editorEl = ref<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null>(null)
const setEditor = (el: unknown) => {
  if (el) editorEl.value = el as HTMLInputElement
}
const editorErrId = `${gid}-editor-error`
const editingField = computed(() => (editing.value ? props.fields[editing.value.c] : null))
const editOptions = computed<SheetOption[]>(() => {
  const e = editing.value
  const f = editingField.value
  if (!e || !f?.options) return []
  return f.options(isNewRow(e.r) ? null : rowAt(e.r))
})
/** the value of a select editor: the option of the current text */
function optionValue(f: SheetField<SheetRow>, r: number): string {
  const row = isNewRow(r) ? null : rowAt(r)
  if (f.value && row) {
    const v = f.value(row)
    return v === null || v === undefined ? '' : String(v)
  }
  const draft = props.draft?.[f.id] ?? ''
  const opt = f.options?.(row).find((o) => o.label === draft)
  return opt?.value ?? ''
}
function startEdit(how: 'edit' | 'type', key = '') {
  const { r, c } = active.value
  const f = props.fields[c]
  if (!f) return
  if (!canEdit(r, c)) {
    emit('announce', props.readOnlyMessage(f, isNewRow(r) ? null : rowAt(r)))
    return
  }
  let text = how === 'type' ? key : cellText(r, c)
  if (f.editor === 'select') {
    text = optionValue(f, r)
    if (how === 'type' && key) {
      const opts = f.options?.(isNewRow(r) ? null : rowAt(r)) ?? []
      const hit = opts.find((o) => !o.disabled && o.label.toLowerCase().startsWith(key.toLowerCase()))
      if (hit) text = hit.value
    }
  }
  editing.value = { r, c, text, typed: how === 'type', error: '' }
  void nextTick(() => {
    const el = editorEl.value
    if (!el) return
    el.focus({ preventScroll: true })
    if (how === 'edit' && 'setSelectionRange' in el && el.tagName !== 'SELECT') {
      const n = (el as HTMLInputElement).value.length
      ;(el as HTMLInputElement).setSelectionRange(n, n)
    }
  })
}
function cancelEdit() {
  editing.value = null
  void nextTick(() => gridEl.value?.focus({ preventScroll: true }))
}
type CommitHow = 'enter' | 'shift-enter' | 'tab' | 'shift-tab' | 'none' | 'up' | 'down' | 'left' | 'right'
/** Save the edit and move; false when the value was refused (the editor stays open). */
function commitEdit(how: CommitHow, refocus = true): boolean {
  const e = editing.value
  if (!e) return true
  const f = props.fields[e.c]
  const row = isNewRow(e.r) ? null : rowAt(e.r)
  if (isNewRow(e.r)) {
    const p = f.parse!(e.text, null)
    if (!p.ok) {
      e.error = p.error
      emit('announce', p.error, true)
      return false
    }
    const shown = f.editor === 'select' ? (editOptions.value.find((o) => o.value === e.text)?.label ?? '') : e.text
    editing.value = null
    emit('draft', f.api ?? f.id, p.value, shown)
  } else {
    const res = planCell(planCtx.value, e.r, e.c, e.text)
    if (res.error) {
      e.error = res.error.message
      emit('announce', res.error.message, true)
      return false
    }
    editing.value = null
    if (res.change && row) emit('apply', { changes: [res.change], errors: [], range: { r1: e.r, c1: e.c, r2: e.r, c2: e.c } }, 'edit')
  }
  const map: Record<string, Parameters<typeof afterCommit>[0]> = { enter: 'enter', 'shift-enter': 'shift-enter', tab: 'tab', 'shift-tab': 'shift-tab' }
  let next = afterCommit(map[how] ?? 'none', { r: e.r, c: e.c }, dims.value)
  if (how === 'up' || how === 'down' || how === 'left' || how === 'right') {
    const key = { up: 'ArrowUp', down: 'ArrowDown', left: 'ArrowLeft', right: 'ArrowRight' }[how]
    next = navigate({ key }, { r: e.r, c: e.c }, dims.value)?.cell ?? next
  }
  setActive(next, false)
  if (refocus) void nextTick(() => gridEl.value?.focus({ preventScroll: true }))
  return true
}
function onEditorKey(ev: KeyboardEvent) {
  const e = editing.value
  if (!e) return
  const f = props.fields[e.c]
  const multiline = f.editor === 'textarea'
  ev.stopPropagation()
  if (ev.key === 'Escape') {
    ev.preventDefault()
    cancelEdit()
  } else if (ev.key === 'Enter' && !ev.altKey && !(multiline && ev.shiftKey) && !ev.isComposing) {
    ev.preventDefault()
    if (ev.ctrlKey || ev.metaKey) return
    commitEdit(ev.shiftKey ? 'shift-enter' : 'enter')
  } else if (ev.key === 'Enter' && ev.altKey && multiline) {
    // Alt+Enter: a line break, as in Excel
    ev.preventDefault()
    const el = ev.target as HTMLTextAreaElement
    const s = el.selectionStart ?? e.text.length
    const end = el.selectionEnd ?? s
    e.text = e.text.slice(0, s) + '\n' + e.text.slice(end)
    void nextTick(() => el.setSelectionRange(s + 1, s + 1))
  } else if (ev.key === 'Tab') {
    ev.preventDefault()
    commitEdit(ev.shiftKey ? 'shift-tab' : 'tab')
  } else if (e.typed && !multiline && f.editor !== 'select' && ['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight'].includes(ev.key)) {
    // typing over a cell: the arrows commit and move, as in a spreadsheet
    ev.preventDefault()
    commitEdit(ev.key.slice(5).toLowerCase() as 'up' | 'down' | 'left' | 'right')
  }
}
function onEditorBlur(ev: FocusEvent) {
  const to = ev.relatedTarget as Node | null
  if (to && (ev.currentTarget as HTMLElement).closest('.sheet-cell')?.contains(to)) return
  // leaving the cell commits; a refused value is dropped (the error was announced)
  if (editing.value && !commitEdit('none', false)) editing.value = null
}
function toggleCategory(id: string, on: boolean) {
  const e = editing.value
  const f = editingField.value
  if (!e || !f?.options) return
  const opts = editOptions.value
  const label = opts.find((o) => o.value === id)?.label ?? ''
  const cur = e.text
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
  const rest = cur.filter((x) => x.toLowerCase() !== label.toLowerCase())
  e.text = (on ? [...rest, label] : rest).join(', ')
  void nextTick(() => editorEl.value?.focus({ preventScroll: true }))
}
const checkedCategory = (label: string) =>
  !!editing.value &&
  editing.value.text
    .split(',')
    .map((s) => s.trim().toLowerCase())
    .includes(label.toLowerCase())

// ---------------------------------------------------------------- keyboard
function onKeydown(e: KeyboardEvent) {
  if (editing.value || e.target !== gridEl.value) return
  const ctrl = e.ctrlKey || e.metaKey
  const key = e.key
  if (!total.value || !props.fields.length) return
  if (ctrl && !e.altKey && key.toLowerCase() === 'a') {
    e.preventDefault()
    anchor.value = { r: 0, c: 0 }
    range.value = { r1: 0, c1: 0, r2: total.value - 1, c2: props.fields.length - 1 }
    emit('announce', t('sheet.selectedAll', { rows: props.rows.length, cols: props.fields.length }))
    return
  }
  if (ctrl && !e.altKey && key.toLowerCase() === 'd') {
    e.preventDefault()
    if (!dataRange.value) return
    const plan = planFillDown(planCtx.value, dataRange.value)
    if (plan.refused === 'nothing') emit('announce', t('sheet.nothingToFill'))
    else if (!refuse(plan)) emit('apply', plan, 'fill')
    return
  }
  if (ctrl && !e.altKey && !e.shiftKey && (key.toLowerCase() === 'c' || key.toLowerCase() === 'x')) {
    // no preventDefault: the browser copies the hidden field's text
    const text = copyText()
    if (text === null) return
    viaClip(text)
    if (key.toLowerCase() === 'x' && dataRange.value) {
      const plan = planClear(planCtx.value, dataRange.value, emptyValue)
      if (!plan.refused) emit('apply', plan, 'cut')
    }
    return
  }
  if (ctrl && !e.altKey && !e.shiftKey && key.toLowerCase() === 'v') {
    viaClip(null)
    return
  }
  if (ctrl && key === 'Enter') {
    e.preventDefault()
    const row = rowAt(active.value.r)
    if (row) emit('open', row.id)
    return
  }
  if (e.altKey && e.shiftKey && (key === 'ArrowLeft' || key === 'ArrowRight')) {
    e.preventDefault()
    const f = props.fields[active.value.c]
    const w = Math.max(60, Math.min(900, width(f) + (key === 'ArrowRight' ? 20 : -20)))
    emit('resize', f.id, w)
    emit('announce', t('sheet.resized', { column: f.header, width: w }))
    return
  }
  if (key === 'Escape') {
    if (range.value.r1 !== range.value.r2 || range.value.c1 !== range.value.c2) {
      e.preventDefault()
      setActive(active.value, false)
    }
    return
  }
  if (key === ' ' && !ctrl) {
    e.preventDefault()
    toggleRows(e.shiftKey)
    return
  }
  if ((key === 'Delete' || key === 'Backspace') && !ctrl && !e.altKey) {
    e.preventDefault()
    if (!dataRange.value) return
    const plan = planClear(planCtx.value, dataRange.value, emptyValue)
    if (plan.refused === 'nothing') emit('announce', t('sheet.nothingToClear'))
    else emit('apply', plan, 'clear')
    return
  }
  const how = editKey(e)
  if (how === 'edit' && (key !== 'Enter' || canEdit(active.value.r, active.value.c))) {
    e.preventDefault()
    startEdit('edit')
    return
  }
  if (how === 'type') {
    e.preventDefault()
    startEdit('type', key)
    return
  }
  const move = key === 'Enter' ? { cell: afterCommit(e.shiftKey ? 'shift-enter' : 'enter', active.value, dims.value), extend: false } : navigate(e, active.value, dims.value)
  if (move) {
    e.preventDefault()
    setActive(move.cell, move.extend)
  }
}
function toggleRows(rangeRows: boolean) {
  const g = rangeRows ? range.value : { r1: active.value.r, r2: active.value.r, c1: 0, c2: 0 }
  const ids: string[] = []
  for (let r = g.r1; r <= Math.min(g.r2, props.rows.length - 1); r++) ids.push(props.rows[r].id)
  if (!ids.length) return
  const on = !ids.every((id) => props.selectedIds.has(id))
  emit('selectRows', ids, on)
  emit('announce', on ? t('sheet.rowsSelected', ids.length) : t('sheet.rowsDeselected', ids.length))
}

// ---------------------------------------------------------------- clipboard
// Ctrl/Cmd+C, X and V go through a hidden text field: the browser copies
// its selected text and pastes into it, which works in every browser (the
// document listeners cover the browser's Edit menu while the grid has focus)
const clip = ref<HTMLTextAreaElement | null>(null)
const ours = (e?: Event) => !editing.value && document.activeElement === gridEl.value && (!e || e.target !== clip.value)
function viaClip(text: string | null) {
  const el = clip.value
  if (!el) return
  el.value = text ?? ''
  el.focus({ preventScroll: true })
  el.select()
  // back to the grid after the copy; after a paste that never came (an empty clipboard) too
  setTimeout(
    () => {
      if (document.activeElement === el) gridEl.value?.focus({ preventScroll: true })
    },
    text !== null ? 0 : 400,
  )
}
function onClipPaste(e: ClipboardEvent) {
  e.preventDefault()
  // handled here only (the document listener would see the grid focused again)
  e.stopPropagation()
  const text = e.clipboardData?.getData('text/plain') ?? ''
  gridEl.value?.focus({ preventScroll: true })
  pasteText(text)
}
function copyText(): string | null {
  if (!dataRange.value) return null
  const block = copyBlock(props.rows, props.fields, dataRange.value, props.copyHeaders)
  emit('announce', t('sheet.copied', rangeSize(dataRange.value).cells))
  return toTSV(block) + '\r\n'
}
function onCopy(e: ClipboardEvent) {
  if (!ours(e) || !dataRange.value || !e.clipboardData) return
  e.preventDefault()
  e.clipboardData.setData('text/plain', copyText() ?? '')
}
function onCut(e: ClipboardEvent) {
  if (!ours(e) || !dataRange.value) return
  onCopy(e)
  const plan = planClear(planCtx.value, dataRange.value, emptyValue)
  if (!plan.refused) emit('apply', plan, 'cut')
}
function onPaste(e: ClipboardEvent) {
  if (!ours(e) || !e.clipboardData) return
  e.preventDefault()
  pasteText(e.clipboardData.getData('text/plain'))
}
function pasteText(text: string) {
  const g = range.value
  if (isNewRow(g.r1)) {
    emit('announce', t('sheet.err.pasteNewRow'), true)
    return
  }
  const plan = planPaste(planCtx.value, parseTSV(text), g)
  if (refuse(plan)) return
  if (!plan.changes.length) {
    emit('announce', t('sheet.nothingChanged'))
    return
  }
  if (plan.range) {
    anchor.value = { r: plan.range.r1, c: plan.range.c1 }
    active.value = { r: plan.range.r1, c: plan.range.c1 }
    range.value = plan.range
  }
  emit('apply', plan, 'paste')
}

// ---------------------------------------------------------------- mouse
let dragging = false
function onCellDown(e: MouseEvent, r: number, c: number) {
  if (e.button !== 0) return
  if (editing.value && editing.value.r === r && editing.value.c === c) return
  if (editing.value && !commitEdit('none', false)) editing.value = null
  e.preventDefault()
  gridEl.value?.focus({ preventScroll: true })
  setActive({ r, c }, e.shiftKey)
  dragging = true
  window.addEventListener('mouseup', stopDrag, { once: true })
}
function onCellEnter(r: number, c: number) {
  if (dragging) setActive({ r, c }, true)
}
function stopDrag() {
  dragging = false
}
function onCellDblClick(r: number, c: number) {
  active.value = { r, c }
  if (canEdit(r, c)) startEdit('edit')
  else if (c === 0 && rowAt(r)) emit('open', rowAt(r)!.id)
}
function onCheck(row: SheetRow, ev: Event) {
  emit('selectRows', [row.id], (ev.target as HTMLInputElement).checked)
}
const allSelected = computed(() => props.rows.length > 0 && props.rows.every((r) => props.selectedIds.has(r.id)))
const someSelected = computed(() => !allSelected.value && props.rows.some((r) => props.selectedIds.has(r.id)))
function onCheckAll(ev: Event) {
  emit(
    'selectRows',
    props.rows.map((r) => r.id),
    (ev.target as HTMLInputElement).checked,
  )
}

// column resizing with the pointer
let resizing: { id: string; x: number; w: number } | null = null
function startResize(e: PointerEvent, f: SheetField<SheetRow>) {
  e.preventDefault()
  e.stopPropagation()
  resizing = { id: f.id, x: e.clientX, w: width(f) }
  ;(e.target as HTMLElement).setPointerCapture?.(e.pointerId)
}
function onResizeMove(e: PointerEvent) {
  if (!resizing) return
  emit('resize', resizing.id, Math.max(60, Math.min(900, Math.round(resizing.w + e.clientX - resizing.x))))
}
function endResize() {
  resizing = null
}

const sortLabel = (f: SheetField<SheetRow>) => (props.sort?.field === f.id ? (props.sort.dir === 1 ? 'ascending' : 'descending') : 'none')
const stateText = (s: CellState | undefined) => (s ? t(`sheet.state.${s.state}`) + (s.message ? ': ' + s.message : '') : '')
</script>

<template>
  <div ref="scroller" class="sheet-scroll" @scroll.passive="onScroll">
    <div
      :id="gid"
      ref="gridEl"
      role="grid"
      class="sheet"
      :class="{ focused }"
      tabindex="0"
      :aria-label="label"
      :aria-rowcount="total + 1"
      :aria-colcount="fields.length + 1"
      aria-multiselectable="true"
      :aria-activedescendant="editing ? undefined : activeId"
      :aria-describedby="`${gid}-help`"
      :style="{ width: totalWidth + 'px' }"
      @keydown="onKeydown"
      @focus="focused = true"
      @blur="focused = false"
    >
      <div role="rowgroup" class="sheet-head">
        <div role="row" aria-rowindex="1" class="sheet-row" :style="{ gridTemplateColumns: template, height: HEAD_H + 'px' }">
          <div role="columnheader" aria-colindex="1" class="sheet-cell head frozen sel">
            <input type="checkbox" tabindex="-1" :checked="allSelected" :indeterminate="someSelected" :aria-label="t('sheet.selectAllRows')" @change="onCheckAll" />
          </div>
          <div
            v-for="(f, ci) in fields"
            :key="f.id"
            role="columnheader"
            :aria-colindex="ci + 2"
            :aria-sort="sortLabel(f)"
            class="sheet-cell head"
            :class="{ frozen: ci === 0, first: ci === 0 }"
          >
            <button type="button" class="sort-btn" tabindex="-1" :title="t('sheet.sortBy', { column: f.header })" @click="emit('sort', f.id)">
              <span class="head-text">{{ f.header }}</span>
              <i v-if="sort?.field === f.id" :class="sort.dir === 1 ? 'pi pi-sort-amount-up-alt' : 'pi pi-sort-amount-down'" aria-hidden="true" />
              <i v-if="!f.editor || f.editor === 'none'" class="pi pi-lock ro-icon" aria-hidden="true" :title="t('sheet.readOnly')" />
            </button>
            <span class="resize" aria-hidden="true" @pointerdown="startResize($event, f)" @pointermove="onResizeMove" @pointerup="endResize" @pointercancel="endResize" />
          </div>
        </div>
      </div>
      <div role="rowgroup" class="sheet-body" :style="{ height: total * ROW_H + 'px' }">
        <div
          v-for="r in windowRows"
          :key="isNewRow(r) ? '__new' : rows[r].id"
          role="row"
          :aria-rowindex="r + 2"
          class="sheet-row"
          :class="{ 'new-row': isNewRow(r), conflict: !isNewRow(r) && conflicts.has(rows[r].id), picked: !isNewRow(r) && selectedIds.has(rows[r].id) }"
          :style="{ gridTemplateColumns: template, top: r * ROW_H + 'px', height: ROW_H + 'px', '--folder-color': !isNewRow(r) && rowColor ? rowColor(rows[r]) || 'transparent' : 'transparent' }"
        >
          <div role="gridcell" aria-colindex="1" class="sheet-cell frozen sel" :aria-readonly="true">
            <input
              v-if="!isNewRow(r)"
              type="checkbox"
              tabindex="-1"
              :checked="selectedIds.has(rows[r].id)"
              :aria-label="t('sheet.selectRow', { key: rows[r].key })"
              @change="onCheck(rows[r], $event)"
            />
            <i v-else class="pi pi-plus muted" aria-hidden="true" />
            <i v-if="!isNewRow(r) && conflicts.has(rows[r].id)" class="pi pi-exclamation-triangle conflict-icon" :title="t('sheet.conflictRow')" aria-hidden="true" />
          </div>
          <div
            v-for="(f, c) in fields"
            :id="cellId(r, c)"
            :key="f.id"
            :role="c === 0 ? 'rowheader' : 'gridcell'"
            :aria-colindex="c + 2"
            :aria-selected="inRange(range, r, c)"
            :aria-readonly="!canEdit(r, c)"
            class="sheet-cell"
            :class="{
              frozen: c === 0,
              first: c === 0,
              active: active.r === r && active.c === c,
              selected: inRange(range, r, c),
              readonly: !canEdit(r, c),
              editing: editing?.r === r && editing?.c === c,
              [`st-${stateOf(r, c)?.state}`]: !!stateOf(r, c),
            }"
            @mousedown="onCellDown($event, r, c)"
            @mouseenter="onCellEnter(r, c)"
            @dblclick="onCellDblClick(r, c)"
          >
            <template v-if="editing && editing.r === r && editing.c === c">
              <select
                v-if="f.editor === 'select'"
                  :ref="setEditor"
                v-model="editing.text"
                class="cell-editor"
                :aria-label="f.header"
                :aria-invalid="!!editing.error"
                :aria-describedby="editing.error ? editorErrId : undefined"
                @keydown="onEditorKey"
                @blur="onEditorBlur"
              >
                <option v-for="o in editOptions" :key="o.value" :value="o.value" :disabled="o.disabled">{{ o.label }}</option>
              </select>
              <textarea
                v-else-if="f.editor === 'textarea'"
                  :ref="setEditor"
                v-model="editing.text"
                class="cell-editor multi"
                :aria-label="f.header"
                :aria-invalid="!!editing.error"
                :aria-describedby="editing.error ? editorErrId : `${gid}-multi-hint`"
                @keydown="onEditorKey"
                @blur="onEditorBlur"
              />
              <input
                v-else
                  :ref="setEditor"
                v-model="editing.text"
                class="cell-editor"
                type="text"
                :aria-label="f.header"
                :aria-invalid="!!editing.error"
                :aria-describedby="editing.error ? editorErrId : f.editor === 'tags' || f.editor === 'categories' ? `${gid}-list-hint` : undefined"
                autocomplete="off"
                @keydown="onEditorKey"
                @blur="onEditorBlur"
              />
              <div v-if="f.editor === 'categories' && editOptions.length" class="cat-pick" role="group" :aria-label="f.header" @mousedown.stop.prevent>
                <label v-for="o in editOptions" :key="o.value" class="small">
                  <input type="checkbox" tabindex="-1" :checked="checkedCategory(o.label)" @change="toggleCategory(o.value, ($event.target as HTMLInputElement).checked)" />
                  {{ o.label }}
                </label>
              </div>
              <div v-if="editing.error" :id="editorErrId" class="cell-error">{{ editing.error }}</div>
            </template>
            <template v-else>
              <span v-if="isNewRow(r) && !cellText(r, c)" class="placeholder">{{ c === 1 ? newRow : c === 0 ? t('sheet.newRowKey') : '' }}</span>
              <span v-else-if="c === 0" class="key">{{ cellText(r, c) }}</span>
              <span v-else-if="f.editor === 'tags'" class="cell-tags">
                <span v-for="tag in tagsOf(r, c)" :key="tag" class="tag-chip" :class="tagClass(tag)">{{ tag }}</span>
              </span>
              <span v-else class="cell-text">{{ cellText(r, c) }}</span>
              <i v-if="stateOf(r, c)?.state === 'pending'" class="pi pi-spin pi-spinner st-icon" aria-hidden="true" />
              <i v-else-if="stateOf(r, c)?.state === 'saved'" class="pi pi-check st-icon" aria-hidden="true" />
              <i v-else-if="stateOf(r, c)?.state === 'error'" class="pi pi-exclamation-circle st-icon" aria-hidden="true" :title="stateOf(r, c)?.message" />
              <span v-if="stateOf(r, c)" class="sr-only">, {{ stateText(stateOf(r, c)) }}</span>
            </template>
          </div>
        </div>
      </div>
    </div>
    <span :id="`${gid}-help`" class="sr-only">{{ t('sheet.gridHelp') }}</span>
    <span :id="`${gid}-multi-hint`" class="sr-only">{{ t('sheet.multiHint') }}</span>
    <span :id="`${gid}-list-hint`" class="sr-only">{{ t('sheet.listHint') }}</span>
    <textarea ref="clip" class="sr-only" tabindex="-1" :aria-label="t('sheet.clipboard')" @paste="onClipPaste" />
  </div>
</template>

<style scoped>
.sheet-scroll {
  position: relative;
  overflow: auto;
  max-height: clamp(320px, calc(100vh - 330px), 900px);
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  background: var(--qtf-card);
  overscroll-behavior: contain;
}
.sheet {
  position: relative;
  min-width: 100%;
  outline: none;
  font-size: 13px;
}
.sheet:focus-visible {
  outline: none !important;
  box-shadow: none !important;
}
.sheet-head {
  position: sticky;
  top: 0;
  z-index: 4;
}
.sheet-row {
  display: grid;
  position: relative;
}
.sheet-body {
  position: relative;
}
.sheet-body .sheet-row {
  position: absolute;
  left: 0;
  right: 0;
}
.sheet-cell {
  position: relative;
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 8px;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  border-right: 1px solid var(--qtf-border);
  border-bottom: 1px solid var(--qtf-border);
  background: var(--qtf-card);
  color: var(--qtf-text);
  cursor: cell;
  user-select: none;
}
.sheet-cell.head {
  font-weight: 600;
  font-size: 12px;
  background: color-mix(in srgb, var(--qtf-text) 5%, var(--qtf-card));
  border-bottom: 2px solid var(--qtf-border);
  padding: 0;
  cursor: default;
}
.sheet-cell.frozen {
  position: sticky;
  z-index: 2;
}
.sheet-cell.sel {
  left: 0;
  justify-content: center;
  padding: 0;
  box-shadow: inset 4px 0 0 var(--folder-color, transparent);
}
.sheet-cell.first {
  left: 40px;
  border-right: 2px solid var(--qtf-border);
}
.sheet-head .sheet-cell.frozen {
  z-index: 5;
}
.sort-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  height: 100%;
  padding: 0 14px 0 8px;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.sort-btn .head-text {
  overflow: hidden;
  text-overflow: ellipsis;
}
.ro-icon {
  font-size: 10px;
  color: var(--qtf-muted);
}
.resize {
  position: absolute;
  top: 0;
  right: -3px;
  width: 8px;
  height: 100%;
  cursor: col-resize;
  z-index: 1;
  touch-action: none;
}
.resize:hover {
  background: color-mix(in srgb, var(--p-primary-color) 40%, transparent);
}
.cell-text {
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
  min-width: 0;
}
.cell-tags {
  display: flex;
  gap: 4px;
  overflow: hidden;
}
.placeholder {
  color: var(--qtf-muted);
  font-style: italic;
}
.sheet-cell.readonly:not(.head):not(.sel) {
  background: color-mix(in srgb, var(--qtf-text) 4%, var(--qtf-card));
  color: color-mix(in srgb, var(--qtf-text) 85%, var(--qtf-card));
}
.sheet-cell.selected:not(.head):not(.sel) {
  background: color-mix(in srgb, var(--p-primary-color) 14%, var(--qtf-card));
}
.sheet-row.picked .sheet-cell.sel {
  background: color-mix(in srgb, var(--p-primary-color) 18%, var(--qtf-card));
}
.sheet-cell.active {
  outline: 2px solid color-mix(in srgb, var(--p-primary-color) 55%, transparent);
  outline-offset: -2px;
  z-index: 3;
}
.sheet.focused .sheet-cell.active {
  outline: 3px solid var(--qtf-focus);
  outline-offset: -3px;
  box-shadow: inset 0 0 0 4px var(--qtf-focus-inner);
}
.sheet-cell.editing {
  overflow: visible;
  padding: 0;
  z-index: 6;
}
.cell-editor {
  width: 100%;
  height: 100%;
  min-width: 0;
  border: 2px solid var(--qtf-focus);
  border-radius: 0;
  padding: 0 6px;
  font: inherit;
  background: var(--qtf-card);
  color: var(--qtf-text);
  outline: none;
}
.cell-editor.multi {
  position: absolute;
  top: 0;
  left: 0;
  width: max(100%, 340px);
  height: 140px;
  padding: 6px;
  resize: both;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.25);
}
.cat-pick {
  position: absolute;
  top: 100%;
  left: 0;
  min-width: 100%;
  max-height: 200px;
  overflow: auto;
  display: grid;
  gap: 2px;
  padding: 6px 8px;
  background: var(--qtf-card);
  border: 1px solid var(--qtf-border);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.25);
  white-space: nowrap;
}
.cat-pick label {
  display: flex;
  gap: 6px;
  align-items: center;
}
.cell-error {
  position: absolute;
  top: calc(100% + 2px);
  left: 0;
  z-index: 7;
  max-width: 360px;
  white-space: normal;
  padding: 6px 8px;
  border-radius: 6px;
  font-size: 12px;
  background: var(--p-red-700, #b91c1c);
  color: #fff;
}
.st-icon {
  font-size: 11px;
  flex: none;
  color: var(--qtf-muted);
}
.sheet-cell.st-error {
  box-shadow: inset 0 0 0 2px var(--p-red-500, #ef4444);
}
.sheet-cell.st-error .st-icon {
  color: var(--p-red-600, #dc2626);
}
.sheet-cell.st-saved .st-icon {
  color: var(--p-green-600, #16a34a);
}
.sheet-cell.st-pending {
  background: color-mix(in srgb, var(--p-yellow-400, #facc15) 16%, var(--qtf-card));
}
.sheet-row.conflict .sheet-cell:not(.head) {
  background: color-mix(in srgb, var(--p-orange-400, #fb923c) 18%, var(--qtf-card));
}
.conflict-icon {
  color: var(--p-orange-600, #ea580c);
  font-size: 12px;
}
.sheet-row.new-row .sheet-cell {
  border-top: 1px dashed var(--qtf-border);
}
:global(.app-dark) .sheet-cell.st-error .st-icon,
:global([data-theme^='high_contrast']) .sheet-cell.st-error .st-icon {
  color: var(--p-red-300, #fca5a5);
}
@media (forced-colors: active) {
  .sheet-cell.active {
    outline: 3px solid Highlight;
  }
  .sheet-cell.selected:not(.head):not(.sel) {
    background: Highlight;
    color: HighlightText;
  }
}
</style>
