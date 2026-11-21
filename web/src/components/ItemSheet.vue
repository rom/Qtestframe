<script setup lang="ts">
// The spreadsheet view of the requirements or tests list (ItemWorkspace):
// the grid with its own columns (shown, ordered, resized and remembered per
// browser), sorting, saving through the batch endpoint (one undo step per
// edit, paste, fill-down or clear), per-cell saving state, conflicts (409)
// with a reload, a "new item" row and the export of the view.
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Select from 'primevue/select'
import Menu from 'primevue/menu'
import SpreadsheetGrid, { type ApplyKind, type CellState } from './SpreadsheetGrid.vue'
import ColumnsMenu from './ColumnsMenu.vue'
import { api, ApiError, errorMessage } from '@/lib/api'
import { sortRows, toBatch, type BatchItem, type Plan, type SheetField } from '@/lib/sheet'
import { applyValues, DEFAULT_SHEET_COLUMNS, sheetFields, type SheetRow } from '@/lib/sheetFields'
import { folderColors } from '@/lib/tree'
import { createdEntry, entry, recordUndo } from '@/lib/undoActions'
import { enumLabel } from '@/i18n/labels'
import { currentLang } from '@/i18n'
import type { Category, Folder, ItemKind } from '@/lib/types'
import { useMetaStore } from '@/stores/meta'
import { usePrefsStore } from '@/stores/prefs'
import { readPref, writePref } from '@/stores/ui'
import { useColumnLayout } from '@/composables/columns'

const props = defineProps<{
  kind: ItemKind
  projectId: string
  items: SheetRow[]
  selectedIds: string[]
  folders: Folder[]
  categories: Category[]
  folderId: string | null
  canWrite: boolean
  canReview: boolean
  canExport: boolean
  /** the filter in effect (shown in exports) */
  filter: string
}>()
const emit = defineEmits<{
  /** rows changed locally (optimistic values, new versions, reloaded conflicts) */
  replace: [rows: SheetRow[]]
  created: [row: SheetRow]
  select: [ids: string[]]
  open: [id: string]
}>()
const { t } = useI18n()
const meta = useMetaStore()
const prefs = usePrefsStore()
const isReq = computed(() => props.kind === 'requirement')
const plural = computed(() => (isReq.value ? 'requirements' : 'tests'))
const batchPath = computed(() => `/projects/${props.projectId}/${plural.value}/batch`)

// ---------------------------------------------------------------- columns
const allFields = computed(() =>
  sheetFields({
    kind: props.kind,
    t: (k, p) => t(k, p ?? {}),
    enumLabel: (g, v) => enumLabel(g, v),
    meta: meta.meta,
    categories: props.categories,
    folders: props.folders,
    canWrite: props.canWrite,
    canReview: props.canReview,
  }),
)
const optional = computed(() => allFields.value.filter((f) => !f.fixed).map((f) => ({ field: f.id, header: f.header })))
const layout = useColumnLayout(
  () => `qtf.sheet.cols.${props.kind}`,
  optional,
  () => DEFAULT_SHEET_COLUMNS[props.kind],
)
const fields = computed<SheetField<SheetRow>[]>(() => [
  ...allFields.value.filter((f) => f.fixed),
  ...layout.order.value.map((id) => allFields.value.find((f) => f.id === id)).filter((f): f is SheetField<SheetRow> => !!f),
])

// widths, per kind and browser
const widthKey = computed(() => `qtf.sheet.widths.${props.kind}`)
const readWidths = () => {
  try {
    const v = JSON.parse(readPref(widthKey.value) || '{}')
    return v && typeof v === 'object' ? (v as Record<string, number>) : {}
  } catch {
    return {}
  }
}
const widths = ref<Record<string, number>>(readWidths())
watch(widthKey, () => (widths.value = readWidths()))
let widthTimer: ReturnType<typeof setTimeout> | undefined
function onResize(id: string, w: number) {
  widths.value = { ...widths.value, [id]: w }
  clearTimeout(widthTimer)
  widthTimer = setTimeout(() => writePref(widthKey.value, JSON.stringify(widths.value)), 250)
}
function resetLayout() {
  layout.reset()
  widths.value = {}
  writePref(widthKey.value, '{}')
}

const copyHeaders = ref(readPref('qtf.sheet.copyHeaders') === 'true')
watch(copyHeaders, (v) => writePref('qtf.sheet.copyHeaders', String(v)))

// ---------------------------------------------------------------- sorting
const sort = ref<{ field: string; dir: 1 | -1 } | null>({ field: 'key', dir: 1 })
function onSort(id: string) {
  const cur = sort.value
  sort.value = !cur || cur.field !== id ? { field: id, dir: 1 } : cur.dir === 1 ? { field: id, dir: -1 } : null
}
const sortOptions = computed(() => [{ value: '', label: t('sheet.unsorted') }, ...fields.value.map((f) => ({ value: f.id, label: f.header }))])
const sortField = computed({
  get: () => sort.value?.field ?? '',
  set: (v: string) => (sort.value = v ? { field: v, dir: sort.value?.dir ?? 1 } : null),
})
// The order is decided when the sort or the set of rows changes, not when a
// value changes: an edited row stays where it is (as in a spreadsheet) and a
// new item is added at the bottom until the next sort.
const order = ref<string[]>([])
function resort() {
  const s = sort.value
  order.value = (s ? sortRows(props.items, allFields.value.find((f) => f.id === s.field), s.dir) : props.items).map((r) => r.id)
}
const idList = computed(() => props.items.map((r) => r.id).join('\n'))
watch([sort, () => props.kind], resort, { immediate: true })
watch(idList, (_now, before) => {
  const known = new Set(order.value)
  const ids = props.items.map((r) => r.id)
  const added = ids.filter((id) => !known.has(id))
  // a created item joins at the bottom; anything else (a reload, a filter) sorts again
  if (before && added.length === 1 && ids.length === order.value.length + 1) order.value = [...order.value, added[0]]
  else resort()
})
const rows = computed(() => {
  const by = new Map(props.items.map((r) => [r.id, r]))
  return order.value.map((id) => by.get(id)).filter((r): r is SheetRow => !!r)
})

// ---------------------------------------------------------------- announcements
const status = ref('')
const alertText = ref('')
function announce(msg: string, urgent = false) {
  // a repeated message is announced again
  if (urgent) {
    alertText.value = ''
    setTimeout(() => (alertText.value = msg), 30)
    status.value = ''
  } else {
    status.value = ''
    setTimeout(() => (status.value = msg), 30)
    alertText.value = ''
  }
}
/** why a cell cannot be edited: it is computed, or the user lacks the permission */
const readOnlyMessage = (f: SheetField<SheetRow>) => (f.editor === 'none' || !f.parse ? t('sheet.err.readOnly', { field: f.header }) : t('sheet.err.noPermission', { field: f.header }))

// ---------------------------------------------------------------- saving
const cellStates = reactive(new Map<string, CellState>())
const conflicts = reactive(new Set<string>())
const conflictCurrent = new Map<string, SheetRow>()
const busy = ref(false)
const cellKey = (id: string, field: string) => `${id}:${field}`
const savedTimers = new Map<string, ReturnType<typeof setTimeout>>()
function markSaved(keys: string[]) {
  for (const k of keys) {
    cellStates.set(k, { state: 'saved' })
    clearTimeout(savedTimers.get(k))
    savedTimers.set(
      k,
      setTimeout(() => {
        if (cellStates.get(k)?.state === 'saved') cellStates.delete(k)
        savedTimers.delete(k)
      }, 2500),
    )
  }
}
const undoLabel = (plan: Plan, kind: ApplyKind, row?: SheetRow) => {
  const n = plan.changes.length
  if (kind === 'edit' && row) return t('undo.a.sheetEdit', { key: row.key, field: fields.value[plan.changes[0].col]?.header ?? '' })
  return t(`undo.a.sheet_${kind}`, { n }, n)
}

// Saves run one after another: a second edit of a row waits for the first,
// so it is sent with the version the first one produced.
let queue: Promise<void> = Promise.resolve()
const current = (id: string) => props.items.find((r) => r.id === id)

function apply(plan: Plan, kind: ApplyKind) {
  if (!plan.changes.length) return
  const shown = rows.value
  const { items, before } = toBatch(shown, plan.changes, false)
  const keys = items.flatMap((it) => Object.keys(it.set).map((f) => cellKey(it.id, f)))
  const label = undoLabel(plan, kind, shown[plan.changes[0].row])
  const one = plan.changes.length === 1 ? plan.changes[0] : null
  const done = one ? t('sheet.savedCell', { field: fields.value[one.col]?.header ?? '', key: shown[one.row]?.key ?? '' }) : t('sheet.saved', plan.changes.length)
  // optimistic: show the new values while saving
  emit(
    'replace',
    items.map((it) => applyValues(current(it.id) ?? (shown.find((r) => r.id === it.id) as SheetRow), it.set, props.folders)),
  )
  for (const k of keys) cellStates.set(k, { state: 'pending' })
  for (const it of items) conflicts.delete(it.id)
  queue = queue.then(() => send(items, before, keys, label, done))
}

async function send(items: BatchItem[], before: BatchItem[], keys: string[], label: string, done: string) {
  busy.value = true
  // the versions the rows have now (after earlier saves of this sheet)
  const withVersions = items.map((it) => {
    const v = current(it.id)?.version
    return v ? { ...it, version: v } : it
  })
  try {
    const res = await api.post<{ updated: number; items: { id: string; version: number; updated_at?: string }[] }>(batchPath.value, { items: withVersions })
    const ver = new Map(res.items.map((x) => [x.id, x]))
    emit(
      'replace',
      items
        .map((it) => current(it.id))
        .filter((r): r is SheetRow => !!r)
        .map((r) => {
          const v = ver.get(r.id)
          return { ...r, version: v?.version ?? r.version, ...(v?.updated_at ? { updated_at: v.updated_at } : {}) }
        }),
    )
    markSaved(keys)
    recordUndo(
      entry(
        label,
        () => api.post(batchPath.value, { items: before }),
        () => api.post(batchPath.value, { items }),
      ),
    )
    announce(done)
  } catch (e) {
    // nothing was saved (one transaction): show the saved values again
    emit(
      'replace',
      before
        .map((b) => {
          const r = current(b.id)
          return r ? applyValues(r, b.set, props.folders) : null
        })
        .filter((r): r is SheetRow => !!r),
    )
    if (e instanceof ApiError && e.status === 409 && e.code === 'stale') {
      const list = ((e.body as { conflicts?: { id: string; key: string; current: SheetRow }[] })?.conflicts ?? []).filter((c) => c?.id)
      for (const c of list) {
        conflicts.add(c.id)
        if (c.current) conflictCurrent.set(c.id, c.current)
      }
      for (const k of keys) cellStates.set(k, { state: 'error', message: t('sheet.err.notSavedConflict') })
      announce(t('sheet.err.conflict', { keys: list.map((c) => c.key).join(', ') }, list.length), true)
    } else {
      const failed = (e instanceof ApiError ? (e.body as { id?: string })?.id : undefined) ?? ''
      const msg = errorMessage(e)
      for (const k of keys) cellStates.set(k, failed && !k.startsWith(failed + ':') ? { state: 'error', message: t('sheet.err.notSavedOther') } : { state: 'error', message: msg })
      announce(t('sheet.err.saveFailed', { message: msg }), true)
    }
  } finally {
    busy.value = false
  }
}

/** Reload the rows someone else changed (their saved versions replace ours). */
async function reloadConflicts() {
  const ids = [...conflicts]
  const fresh: SheetRow[] = []
  for (const id of ids) {
    let cur = conflictCurrent.get(id)
    try {
      cur = await api.get<SheetRow>(`/${plural.value}/${id}`)
    } catch {
      /* keep the copy from the conflict answer */
    }
    if (cur) fresh.push(cur)
    conflicts.delete(id)
    conflictCurrent.delete(id)
    for (const k of [...cellStates.keys()]) if (k.startsWith(id + ':')) cellStates.delete(k)
  }
  emit('replace', fresh)
  announce(t('sheet.reloaded', fresh.length))
}
// the parent reloaded: forget states of rows that are gone
watch(
  () => props.items,
  (list) => {
    const ids = new Set(list.map((r) => r.id))
    for (const id of [...conflicts]) if (!ids.has(id)) conflicts.delete(id)
  },
)

// ---------------------------------------------------------------- new item row
const draft = reactive<Record<string, string>>({})
const draftValues = reactive<Record<string, unknown>>({})
const grid = ref<InstanceType<typeof SpreadsheetGrid> | null>(null)
const newRowLabel = computed(() => (props.canWrite ? (isReq.value ? t('sheet.newRequirement') : t('sheet.newTest')) : undefined))
async function onDraft(apiField: string, value: unknown, text: string) {
  const f = allFields.value.find((x) => (x.api ?? x.id) === apiField)
  if (!f) return
  draft[f.id] = text
  draftValues[apiField] = value
  if (apiField !== 'title' && !draftValues.title) {
    announce(t('sheet.typeTitle'))
    return
  }
  if (!String(draftValues.title ?? '').trim()) return
  await createItem(f.id)
}
async function createItem(col: string) {
  const body: Record<string, unknown> = { folder_id: props.folderId, ...draftValues }
  if (isReq.value) body.create_test = prefs.createLinkedTest(meta.meta.auto_create_test)
  try {
    busy.value = true
    const created = await api.post<SheetRow>(`/projects/${props.projectId}/${plural.value}`, body)
    for (const k of Object.keys(draft)) delete draft[k]
    for (const k of Object.keys(draftValues)) delete draftValues[k]
    emit('created', created)
    recordUndo(createdEntry(t('undo.a.create', { key: created.key }), () => api.delTrash(`/${plural.value}/${created.id}`)))
    announce(t('sheet.created', { key: created.key }))
    // keep typing new items: the new row is below the created one
    const c = Math.max(0, fields.value.findIndex((x) => x.id === col))
    setTimeout(() => grid.value?.focusCell(props.items.length, c), 0)
  } catch (e) {
    announce(t('sheet.err.createFailed', { message: errorMessage(e) }), true)
  } finally {
    busy.value = false
  }
}

// ---------------------------------------------------------------- selection
const selected = computed(() => new Set(props.selectedIds))
function onSelectRows(ids: string[], on: boolean) {
  const next = new Set(props.selectedIds)
  for (const id of ids) {
    if (on) next.add(id)
    else next.delete(id)
  }
  emit('select', [...next])
}
const colorOf = computed(() => folderColors(props.folders))
const rowColor = (r: SheetRow) => (r.folder_id ? colorOf.value.get(r.folder_id as string) : '') || ''

// ---------------------------------------------------------------- export
const exportMenu = ref<InstanceType<typeof Menu> | null>(null)
const exporting = ref(false)
const exportItems = computed(() => [
  { label: t('sheet.exportXlsx'), icon: 'pi pi-file-excel', command: () => exportView('xlsx') },
  { label: t('sheet.exportCsv'), icon: 'pi pi-file', command: () => exportView('csv') },
  { label: t('sheet.exportMd'), icon: 'pi pi-hashtag', command: () => exportView('md') },
])
async function exportView(format: 'xlsx' | 'csv' | 'md') {
  exporting.value = true
  try {
    const cols = fields.value
    const body = {
      format,
      name: isReq.value ? 'requirements' : 'tests',
      title: isReq.value ? t('sheet.exportTitleRequirements') : t('sheet.exportTitleTests'),
      lang: currentLang(),
      filter: props.filter,
      columns: cols.map((f) => f.header),
      rows: rows.value.map((r) => cols.map((f) => f.text(r))),
    }
    const name = await api.postDownload(`/projects/${props.projectId}/export/view`, body, `${body.name}.${format}`)
    announce(t('sheet.exported', { name }))
  } catch (e) {
    announce(t('sheet.err.exportFailed', { message: errorMessage(e) }), true)
  } finally {
    exporting.value = false
  }
}

const gridLabel = computed(() => (isReq.value ? t('sheet.gridRequirements') : t('sheet.gridTests')))
const shortcuts = computed(() => [
  ['↑ ↓ ← →', t('sheet.keys.move')],
  ['Tab / Shift+Tab', t('sheet.keys.tab')],
  ['Home / End, Ctrl+Home / Ctrl+End', t('sheet.keys.ends')],
  ['Page Up / Page Down', t('sheet.keys.page')],
  ['Shift + ' + t('sheet.keys.arrowsOrClick'), t('sheet.keys.extend')],
  ['Ctrl+A', t('sheet.keys.all')],
  ['F2, Enter, ' + t('sheet.keys.typing'), t('sheet.keys.edit')],
  ['Enter / Tab', t('sheet.keys.commit')],
  ['Alt+Enter', t('sheet.keys.newline')],
  ['Escape', t('sheet.keys.cancel')],
  ['Ctrl+C / Ctrl+X / Ctrl+V', t('sheet.keys.clipboard')],
  ['Ctrl+D', t('sheet.keys.fill')],
  ['Delete / Backspace', t('sheet.keys.clear')],
  ['Space / Shift+Space', t('sheet.keys.rows')],
  ['Ctrl+Enter', t('sheet.keys.open')],
  ['Alt+Shift+← →', t('sheet.keys.resize')],
  ['Ctrl+Z / Ctrl+Y', t('sheet.keys.undo')],
])
</script>

<template>
  <div class="item-sheet">
    <div class="sheet-toolbar" role="toolbar" :aria-label="t('sheet.toolbar')">
      <ColumnsMenu :columns="optional" :order="layout.order.value" @toggle="(id, on) => layout.toggle(id, on)" @move="(id, d) => layout.move(id, d)" @reset="resetLayout" />
      <span class="row nowrap small sort-pick">
        <label :for="`sheet-sort-${kind}`">{{ t('sheet.sortLabel') }}</label>
        <Select v-model="sortField" :input-id="`sheet-sort-${kind}`" :options="sortOptions" option-label="label" option-value="value" size="small" style="min-width: 150px" />
        <Button
          v-if="sort"
          :icon="sort.dir === 1 ? 'pi pi-sort-amount-up-alt' : 'pi pi-sort-amount-down'"
          size="small"
          severity="secondary"
          text
          rounded
          :aria-label="sort.dir === 1 ? t('sheet.sortAscending') : t('sheet.sortDescending')"
          :aria-pressed="sort.dir === -1"
          v-tooltip.bottom="sort.dir === 1 ? t('sheet.sortAscending') : t('sheet.sortDescending')"
          @click="sort = { field: sort.field, dir: sort.dir === 1 ? -1 : 1 }"
        />
      </span>
      <span class="row nowrap small">
        <Checkbox v-model="copyHeaders" binary :input-id="`sheet-copy-headers-${kind}`" />
        <label :for="`sheet-copy-headers-${kind}`">{{ t('sheet.copyHeaders') }}</label>
      </span>
      <Button
        v-if="canExport"
        :label="t('sheet.exportView')"
        icon="pi pi-download"
        size="small"
        severity="secondary"
        outlined
        aria-haspopup="menu"
        :loading="exporting"
        class="export-view"
        @click="exportMenu?.toggle($event)"
      />
      <Menu ref="exportMenu" :model="exportItems" popup />
      <details class="sheet-keys small">
        <summary>{{ t('sheet.keys.title') }}</summary>
        <dl>
          <template v-for="[k, d] in shortcuts" :key="k">
            <dt><kbd>{{ k }}</kbd></dt>
            <dd>{{ d }}</dd>
          </template>
        </dl>
      </details>
      <span v-if="busy" class="small muted row nowrap"><i class="pi pi-spin pi-spinner" aria-hidden="true" />{{ t('sheet.saving') }}</span>
    </div>

    <div v-if="conflicts.size" class="sheet-conflict" role="group" :aria-label="t('sheet.conflictTitle')">
      <i class="pi pi-exclamation-triangle" aria-hidden="true" />
      <span>{{ t('sheet.conflictBanner', conflicts.size) }}</span>
      <Button :label="t('sheet.reloadRows', conflicts.size)" icon="pi pi-refresh" size="small" severity="warn" @click="reloadConflicts" />
    </div>

    <SpreadsheetGrid
      ref="grid"
      :rows="rows"
      :fields="fields"
      :widths="widths"
      :label="gridLabel"
      :new-row="newRowLabel"
      :draft="draft"
      :cell-states="cellStates"
      :conflicts="conflicts"
      :selected-ids="selected"
      :sort="sort"
      :copy-headers="copyHeaders"
      :row-color="rowColor"
      :read-only-message="readOnlyMessage"
      @apply="apply"
      @draft="onDraft"
      @announce="announce"
      @select-rows="onSelectRows"
      @sort="onSort"
      @resize="onResize"
      @open="(id) => emit('open', id)"
    />
    <div class="sheet-status small" role="status" aria-live="polite" aria-atomic="true">{{ status }}</div>
    <div class="sheet-alert small" role="alert" aria-atomic="true">{{ alertText }}</div>
  </div>
</template>

<style scoped>
.item-sheet {
  display: grid;
  gap: 8px;
  min-width: 0;
}
.sheet-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 14px;
}
.sort-pick {
  gap: 6px;
}
.sheet-keys summary {
  cursor: pointer;
  color: var(--p-primary-color);
}
.sheet-keys[open] {
  flex-basis: 100%;
}
.sheet-keys dl {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 4px 14px;
  margin: 8px 0 0;
}
.sheet-keys dd {
  margin: 0;
}
.sheet-keys kbd {
  font-family: var(--qtf-mono);
  font-size: 11.5px;
  padding: 1px 6px;
  border: 1px solid var(--qtf-border);
  border-radius: 4px;
}
.sheet-conflict {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  padding: 8px 12px;
  border-radius: 8px;
  border: 1px solid var(--p-orange-400, #fb923c);
  background: color-mix(in srgb, var(--p-orange-400, #fb923c) 14%, var(--qtf-card));
  color: var(--qtf-text);
}
.sheet-status:empty,
.sheet-alert:empty {
  display: none;
}
.sheet-status {
  color: var(--qtf-muted);
}
.sheet-alert {
  padding: 6px 10px;
  border-radius: 6px;
  border: 1px solid var(--p-red-500, #ef4444);
  color: var(--qtf-text);
  background: color-mix(in srgb, var(--p-red-500, #ef4444) 12%, var(--qtf-card));
}
@media (max-width: 640px) {
  .sort-pick {
    flex-wrap: wrap;
  }
}
</style>
