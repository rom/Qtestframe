<script setup lang="ts">
import StaleBadge from './StaleBadge.vue'
import { isSoftware } from '@/lib/projectTypes'
import { useUndoStore } from '@/stores/undo'
import { deletedEntry, entry, recordUndo } from '@/lib/undoActions'
import { tagClass } from '@/lib/tagColors'
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import MultiSelect from 'primevue/multiselect'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import Drawer from 'primevue/drawer'
import Dialog from 'primevue/dialog'
import SelectButton from 'primevue/selectbutton'
import FolderTree from './FolderTree.vue'
import FolderPickerDialog from './FolderPickerDialog.vue'
import QueryInput from './QueryInput.vue'
import StatusTag from './StatusTag.vue'
import CategoryChips from './CategoryChips.vue'
import DefectLinks from './DefectLinks.vue'
import CategorySelect from './CategorySelect.vue'
import TagsInput from './TagsInput.vue'
import RequirementEditor from './RequirementEditor.vue'
import TestEditor from './TestEditor.vue'
import RequirementImportDialog from './RequirementImportDialog.vue'
import ExchangeImportDialog from './ExchangeImportDialog.vue'
import Menu from 'primevue/menu'
import type { ExchangeMode } from '@/lib/exchange'
import type { ImportSourceId } from '@/lib/importSources'
import BulkReviewDialog from './BulkReviewDialog.vue'
import ApplicabilityDialog from './ApplicabilityDialog.vue'
import VariantSelect from './VariantSelect.vue'
import { useVariants } from '@/composables/variants'
import BulkRelationsDialog from './BulkRelationsDialog.vue'
import BulkItemLinksDialog from './BulkItemLinksDialog.vue'
import BulkRequirementIssuesDialog from './BulkRequirementIssuesDialog.vue'
import type { ImportResult } from '@/lib/reqImport'
import { api, ApiError, asItems } from '@/lib/api'
import { activeFilterCount, composeQuery, type QuickFilters } from '@/lib/query'
import { dragIds, encodeIds, ITEMS_MIME } from '@/lib/dnd'
import { folderColors, folderPath } from '@/lib/tree'
import { formatDate } from '@/lib/format'
import { levelRank, riskLevelSeverity } from '@/lib/risks'
import { useI18n } from 'vue-i18n'
import { enumLabel, enumOptions } from '@/i18n/labels'
import { P } from '@/lib/permissions'
import type { ItemKind, Requirement, TestCase } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { customColumns, listedFields, customCellText } from '@/lib/customColumns'
import { isCfField } from '@/lib/customFields'
import { useMetaStore } from '@/stores/meta'
import { useAuthStore } from '@/stores/auth'
import { readPref, writePref } from '@/stores/ui'
import { useNotify } from '@/composables/notify'
import { usePrefsStore } from '@/stores/prefs'
import { useBreakpoint } from '@/lib/breakpoint'
import { rememberFocus } from '@/lib/a11y'
import RelationsCell from './RelationsCell.vue'
import ColumnsMenu from './ColumnsMenu.vue'
import { useColumnLayout } from '@/composables/columns'
import MentionText from './MentionText.vue'
import HelpLink from '@/guide/HelpLink.vue'
import ItemSheet from './ItemSheet.vue'
import type { SheetRow } from '@/lib/sheetFields'
const prefs = usePrefsStore()
/** below 1200 px the folder tree lives in a drawer opened by a "Folders" button */
const treeInDrawer = useBreakpoint('wide')
const foldersOpen = ref(false)
let restoreFoldersFocus: ReturnType<typeof rememberFocus> | null = null
watch(foldersOpen, (open) => {
  if (open) restoreFoldersFocus = rememberFocus()
  else
    void nextTick(() => {
      restoreFoldersFocus?.(() => document.getElementById('ws-title'))
      restoreFoldersFocus = null
    })
})
watch(treeInDrawer, (v) => {
  if (!v) foldersOpen.value = false
})

type Item = Requirement & TestCase

const props = defineProps<{ kind: ItemKind }>()
const proj = useProjectStore()
const meta = useMetaStore()
const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()

const isReq = computed(() => props.kind === 'requirement')
const plural = computed(() => (isReq.value ? 'requirements' : 'tests'))
const pid = computed(() => proj.pid)
const canWrite = computed(() => auth.can(isReq.value ? P.requirementWrite : P.testWrite))
const canReview = computed(() => auth.can(isReq.value ? P.requirementReview : P.testReview))
const bulkReviewOpen = ref(false)
/** bulk "Applies to" (variants) of the selection */
const bulkApplicabilityOpen = ref(false)
const bulkRelationsOpen = ref(false)
const bulkLinksOpen = ref(false)
const bulkIssuesOpen = ref(false)
const shownIds = computed(() => items.value.map((i) => i.id))
const canLink = computed(() => auth.can(P.requirementWrite) || auth.can(P.testWrite))
const folders = computed(() => proj.folders[props.kind])
// rows take the colour of their folder (or of its nearest coloured parent)
const colorOfFolder = computed(() => folderColors(folders.value))
const rowFolderColor = (row: { folder_id?: string | null }) => (row.folder_id ? colorOfFolder.value.get(row.folder_id) : '') || ''
const folderRowClass = (row: Item) => (rowFolderColor(row) ? 'folder-colored' : '')
const folderRowStyle = (row: Item) => (rowFolderColor(row) ? { '--folder-color': rowFolderColor(row) } : undefined)
const categories = computed(() => proj.categoriesFor(props.kind))

// ---------- filter state ----------
const selectedFolder = ref<string | null>((route.query.folder as string) || null)
const recursive = ref(readPref('qtf.recursive') !== 'false')
const q = ref((route.query.q as string) || '')
// "show for variant": only the items that apply to it, with its coverage / results
const variant = ref<string | null>((route.query.variant as string) || null)
const variantList = useVariants(() => pid.value)
watch(pid, () => void variantList.load(), { immediate: true })
const quick = ref<QuickFilters>({})
const composed = computed(() => composeQuery(q.value, quick.value))
const queryError = ref<unknown>(null)

const items = ref<Item[]>([])
const loading = ref(false)
const selection = ref<Item[]>([])

async function load() {
  if (!pid.value) return
  loading.value = true
  queryError.value = null
  try {
    const res = await api.get<{ items: Item[] }>(`/projects/${pid.value}/${plural.value}`, {
      q: composed.value,
      folder_id: selectedFolder.value,
      recursive: selectedFolder.value ? recursive.value : undefined,
      variant: variant.value || undefined,
    })
    items.value = asItems(res)
    const ids = new Set(items.value.map((i) => i.id))
    selection.value = selection.value.filter((s) => ids.has(s.id))
  } catch (e) {
    queryError.value = e
    if (!(e instanceof ApiError && e.code === 'invalid_query')) notify.fail(e, isReq.value ? t('workspace.loadRequirementsFailed') : t('workspace.loadTestsFailed'))
  } finally {
    loading.value = false
  }
}

watch([selectedFolder, recursive, () => quick.value, variant], () => load(), { deep: true })
watch(selectedFolder, () => {
  foldersOpen.value = false
})
watch(recursive, (v) => writePref('qtf.recursive', String(v)))
watch([selectedFolder, q, variant], () => {
  const query = { ...route.query, folder: selectedFolder.value || undefined, q: q.value || undefined, variant: variant.value || undefined }
  router.replace({ query })
})
onMounted(() => {
  load()
  if (route.query.item) openEditor(String(route.query.item))
})
watch(
  () => route.query.item,
  (id) => {
    if (id && id !== editorId.value) openEditor(String(id))
  },
)

// ---------- layout: the list or the spreadsheet (?view=sheet, remembered per browser) ----------
type Layout = 'list' | 'sheet'
const isLayout = (v: unknown): v is Layout => v === 'list' || v === 'sheet'
const layoutKey = () => `qtf.layout.${props.kind}`
const storedLayout = readPref(layoutKey())
const layoutMode = ref<Layout>(isLayout(route.query.view) ? route.query.view : isLayout(storedLayout) ? storedLayout : 'list')
// the choice is stored (also one that came with the link) and shown in the address
watch(
  layoutMode,
  (v) => {
    writePref(layoutKey(), v)
    if ((route.query.view ?? 'list') !== v) void router.replace({ query: { ...route.query, view: v === 'sheet' ? 'sheet' : undefined } })
  },
  { immediate: true },
)
watch(
  () => route.query.view,
  (v) => {
    if (isLayout(v)) layoutMode.value = v
  },
)
const layoutOptions = computed(() => [
  { value: 'list', label: t('sheet.layoutList'), icon: 'pi pi-list' },
  { value: 'sheet', label: t('sheet.layoutSheet'), icon: 'pi pi-th-large' },
])
const canExport = computed(() => auth.can(P.export))
const sheetItems = computed(() => items.value as unknown as SheetRow[])
const selectionIds = computed(() => selection.value.map((s) => s.id))
function onSheetReplace(rows: SheetRow[]) {
  if (!rows.length) return
  const by = new Map(rows.map((r) => [r.id, r as unknown as Item]))
  items.value = items.value.map((i) => by.get(i.id) ?? i)
  selection.value = selection.value.map((s) => by.get(s.id) ?? s)
}
function onSheetCreated(row: SheetRow) {
  items.value = [...items.value, row as unknown as Item]
}
function onSheetSelect(ids: string[]) {
  const want = new Set(ids)
  selection.value = items.value.filter((i) => want.has(i.id))
}

function clearFilters() {
  quick.value = {}
  q.value = ''
  load()
}

// ---------- columns ----------
interface Col {
  field: string
  header: string
  width?: string
  sortable?: boolean
  /** field to sort by when it differs from the displayed one */
  sortField?: string | ((item: any) => string)
}
/** the project's custom fields of the kind as columns (cf.<key>) */
const customCols = computed<Col[]>(() => customColumns(proj.customFieldsFor(isReq.value ? 'requirement' : 'test')))
const cellText = (row: Item, field: string) => customCellText(proj.customFieldsFor(isReq.value ? 'requirement' : 'test'), row, field, [t('common.yes'), t('common.no')])
const allColumns = computed<Col[]>(() => [...baseColumns.value, ...customCols.value])
const baseColumns = computed<Col[]>(() =>
  isReq.value
    ? [
        { field: 'obligation', header: t('workspace.colObligation'), width: '120px' },
        { field: 'status', header: t('workspace.colStatus'), width: '120px' },
        { field: 'priority', header: t('workspace.colPriority'), width: '100px' },
        { field: 'type', header: t('workspace.colType'), width: '130px' },
        { field: 'coverage', header: t('workspace.colCoverage'), width: '120px' },
        { field: 'linked_tests', header: t('workspace.colTests'), width: '85px' },
        { field: 'category_ids', header: t('workspace.colCategories'), sortable: false },
        { field: 'folder_path', header: t('workspace.colFolder') },
        { field: 'owner', header: t('workspace.colOwner') },
        { field: 'source', header: t('workspace.colSource') },
        { field: 'tags', header: t('workspace.colTags'), sortable: false },
        { field: 'relations', header: t('workspace.colRelations'), sortField: 'relation_count', width: '240px' },
        { field: 'relation_count', header: t('workspace.colRelationCount'), width: '105px' },
        { field: 'criteria_count', header: t('workspace.colCriteria'), width: '90px' },
        { field: 'version', header: t('workspace.colVersion'), width: '70px' },
        { field: 'comment_count', header: t('workspace.colComments'), width: '120px' },
        { field: 'suspect_links', header: t('workspace.colSuspect'), width: '110px' },
        { field: 'risk_level', header: t('risksView.colRisk'), width: '105px', sortField: (r: Item) => String(levelRank((r as Requirement).risk_level)) },
        { field: 'open_defects', header: t('risksView.colOpenDefects'), width: '115px' },
        { field: 'verification', header: t('verification.column'), width: '150px' },
        { field: 'created_by', header: t('workspace.colCreatedBy') },
        { field: 'created_at', header: t('workspace.colCreated'), width: '115px' },
        { field: 'updated_at', header: t('workspace.colUpdated'), width: '115px' },
      ]
    : [
        { field: 'execution_type', header: t('workspace.colExecution'), width: '135px' },
        { field: 'status', header: t('workspace.colStatus'), width: '110px' },
        { field: 'priority', header: t('workspace.colPriority'), width: '100px' },
        { field: 'last_result', header: t('workspace.colLastResult'), width: '130px' },
        { field: 'linked_requirements', header: t('workspace.colReqs'), width: '80px' },
        { field: 'category_ids', header: t('workspace.colCategories'), sortable: false },
        { field: 'folder_path', header: t('workspace.colFolder') },
        { field: 'estimated_minutes', header: t('workspace.colEstimate'), width: '100px' },
        { field: 'script_language', header: t('workspace.colScriptLanguage'), width: '110px' },
        { field: 'step_count', header: t('workspace.colSteps'), width: '80px' },
        { field: 'tags', header: t('workspace.colTags'), sortable: false },
        { field: 'relations', header: t('workspace.colRelations'), sortable: false, width: '240px' },
        { field: 'defects', header: t('workspace.colDefects'), sortable: false },
        { field: 'version', header: t('workspace.colVersion'), width: '70px' },
        { field: 'comment_count', header: t('workspace.colComments'), width: '120px' },
        { field: 'suspect_links', header: t('workspace.colSuspect'), width: '110px' },
        { field: 'created_by', header: t('workspace.colCreatedBy') },
        { field: 'created_at', header: t('workspace.colCreated'), width: '115px' },
        { field: 'updated_at', header: t('workspace.colUpdated'), width: '115px' },
      ],
)
const defaultCols = computed(() =>
  isReq.value
    ? ['obligation', 'status', 'priority', 'coverage', 'linked_tests', 'category_ids', ...listedFields(proj.customFieldsFor('requirement'))]
    : ['execution_type', 'status', 'priority', 'last_result', 'linked_requirements', 'category_ids', ...listedFields(proj.customFieldsFor('test'))],
)
// visible optional columns in their order (Columns menu, or drag a header);
// the old count-only "Relations" column became the list of relations
const layout = useColumnLayout(() => `qtf.cols.${props.kind}`, allColumns, () => defaultCols.value, { relation_count: 'relations' })
const visibleColumns = layout.visible

// ---------- drag rows to folders ----------
function onRowDrag(e: DragEvent, row: Item) {
  if (!e.dataTransfer || !canWrite.value) return
  const ids = dragIds(row.id, selection.value)
  e.dataTransfer.setData(ITEMS_MIME, encodeIds(ids))
  e.dataTransfer.setData('text/plain', ids.length > 1 ? t(isReq.value ? 'workspace.dragRequirements' : 'workspace.dragTests', { count: ids.length }) : row.key)
  e.dataTransfer.effectAllowed = 'move'
}

async function moveItems(ids: string[], folderId: string | null) {
  const path = `/projects/${pid.value}/${plural.value}/move`
  const byFolder = new Map<string | null, string[]>()
  for (const id of ids) {
    const old = items.value.find((x) => x.id === id)?.folder_id ?? null
    byFolder.set(old, [...(byFolder.get(old) ?? []), id])
  }
  try {
    await api.post(path, { ids, folder_id: folderId })
    recordUndo(
      entry(
        t('undo.a.move', { n: ids.length }),
        async () => {
          for (const [f, group] of byFolder) await api.post(path, { ids: group, folder_id: f })
        },
        () => api.post(path, { ids, folder_id: folderId }),
      ),
    )
    notify.ok(t(isReq.value ? 'workspace.movedRequirements' : 'workspace.movedTests', ids.length), folderId ? folderPath(folders.value, folderId) : t('common.topLevel'))
    await load()
  } catch (e) {
    notify.fail(e, t('workspace.moveFailed'))
  }
}

const undoStore = useUndoStore()
watch(
  () => undoStore.version,
  async () => {
    await proj.loadFolders(props.kind)
    await load()
  },
)

// ---------- import (requirements only) ----------
const importOpen = ref(false)
async function onImported(res: ImportResult | { folders_created: number }) {
  if (res.folders_created) await proj.loadFolders(props.kind)
  await load()
}

// ---------- exchange: ReqIF with updates, the Excel round-trip ----------
const exchangeOpen = ref(false)
const exchangeMode = ref<ExchangeMode | ImportSourceId>('xlsx')
const exchangeMenu = ref<InstanceType<typeof Menu> | null>(null)
function openExchange(mode: ExchangeMode | ImportSourceId) {
  exchangeMode.value = mode
  exchangeOpen.value = true
}
async function exportRoundTrip() {
  try {
    const name = await api.download(`/projects/${pid.value}/exchange/xlsx`, { kind: props.kind, q: composed.value || undefined, folder_id: selectedFolder.value || undefined, recursive: String(recursive.value) }, `${props.kind}s-roundtrip.xlsx`)
    notify.ok(t('exchange.exported', { name }))
  } catch (e) {
    notify.fail(e, t('exchange.exportFailed'))
  }
}
const exchangeItems = computed(() => [
  ...(isReq.value && canWrite.value
    ? [
        { label: t('exchange.fromFile'), icon: 'pi pi-upload', command: () => (importOpen.value = true) },
        { label: t('exchange.fromReqif'), icon: 'pi pi-sitemap', command: () => openExchange('reqif') },
        { label: t('exchange.fromJira'), icon: 'pi pi-cloud-download', command: () => openExchange('jira') },
        { label: t('exchange.fromAzure'), icon: 'pi pi-microsoft', command: () => openExchange('azure-devops') },
      ]
    : []),
  ...(canExport.value ? [{ label: t('exchange.exportXlsx'), icon: 'pi pi-file-excel', command: exportRoundTrip }] : []),
  ...(canWrite.value ? [{ label: t('exchange.importXlsx'), icon: 'pi pi-file-import', command: () => openExchange('xlsx') }] : []),
])

async function onFoldersChanged() {
  await proj.loadFolders(props.kind)
  if (selectedFolder.value && !folders.value.some((f) => f.id === selectedFolder.value)) selectedFolder.value = null
  await load()
}

// ---------- bulk actions ----------
async function onBulkReviewed() {
  selection.value = []
  await load()
}
const selectedIds = computed(() => selection.value.map((s) => s.id))
async function bulkSet(set: Record<string, unknown>) {
  if (!selectedIds.value.length) return
  const ids = [...selectedIds.value]
  const field = (k: string) => (k === 'add_category_ids' || k === 'remove_category_ids' ? 'category_ids' : k === 'add_tags' || k === 'remove_tags' ? 'tags' : k)
  const before = ids.map((id) => {
    const row = (items.value.find((x) => x.id === id) ?? {}) as Record<string, unknown>
    const old: Record<string, unknown> = {}
    for (const k of Object.keys(set)) old[field(k)] = Array.isArray(row[field(k)]) ? [...(row[field(k)] as unknown[])] : row[field(k)]
    return { id, old }
  })
  const bulkPath = `/projects/${pid.value}/${plural.value}/bulk`
  try {
    await api.post(bulkPath, { ids, set })
    recordUndo(
      entry(
        t('undo.a.bulk', { n: ids.length }),
        async () => {
          for (const b of before) await api.patch(`/${plural.value}/${b.id}`, b.old)
        },
        () => api.post(bulkPath, { ids, set }),
      ),
    )
    notify.ok(t(isReq.value ? 'workspace.updatedRequirements' : 'workspace.updatedTests', selectedIds.value.length))
    await load()
  } catch (e) {
    notify.fail(e, t('workspace.bulkFailed'))
  }
}
function bulkDelete() {
  const n = selectedIds.value.length
  notify.confirmDanger(t(isReq.value ? 'workspace.confirmDeleteRequirements' : 'workspace.confirmDeleteTests', n), async () => {
    try {
      const ids = [...selectedIds.value]
      const path = `/projects/${pid.value}/${plural.value}/bulk`
      const res = await api.del<{ trash_id?: string }>(path, { ids })
      if (res?.trash_id) {
        const label = n === 1 ? t('undo.a.delete', { key: selection.value[0]?.key ?? '' }) : t('undo.a.deleteMany', { n })
        recordUndo(deletedEntry(label, res.trash_id, async () => (await api.del<{ trash_id?: string }>(path, { ids }))?.trash_id ?? ''))
      }
      selection.value = []
      notify.ok(t('workspace.deletedCount', { count: n }), t('undo.hint'))
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}
const tagDialog = ref(false)
const tagMode = ref<'add' | 'remove'>('add')
const tagValues = ref<string[]>([])
function openTagDialog(mode: 'add' | 'remove') {
  tagMode.value = mode
  tagValues.value = []
  tagDialog.value = true
}
async function applyTags() {
  tagDialog.value = false
  if (tagValues.value.length) await bulkSet(tagMode.value === 'add' ? { add_tags: tagValues.value } : { remove_tags: tagValues.value })
}
const catDialog = ref(false)
const catMode = ref<'add' | 'remove'>('add')
const catIds = ref<string[]>([])
function openCatDialog(mode: 'add' | 'remove') {
  catMode.value = mode
  catIds.value = []
  catDialog.value = true
}
async function applyCats() {
  catDialog.value = false
  await bulkSet(catMode.value === 'add' ? { add_category_ids: catIds.value } : { remove_category_ids: catIds.value })
}
const moveDialog = ref(false)
function onPickFolder(id: string | null) {
  moveItems(selectedIds.value, id)
}

const bulkStatus = ref<string | null>(null)
const bulkPriority = ref<string | null>(null)
const bulkThird = ref<string | null>(null)
watch(bulkStatus, (v) => v && bulkSet({ status: v }).then(() => (bulkStatus.value = null)))
watch(bulkPriority, (v) => v && bulkSet({ priority: v }).then(() => (bulkPriority.value = null)))
watch(bulkThird, (v) => v && bulkSet(isReq.value ? { obligation: v } : { execution_type: v }).then(() => (bulkThird.value = null)))

const statusOptions = computed(() => enumOptions('status', isReq.value ? meta.meta.requirement_statuses : meta.meta.test_statuses))
const thirdOptions = computed(() => (isReq.value ? enumOptions('obligation', meta.meta.obligations) : enumOptions('executionType', ['manual', 'automated'])))
const priorityOptions = computed(() => enumOptions('priority', meta.meta.priorities))
const coverageOptions = computed(() => enumOptions('coverage', ['none', 'not_run', 'passed', 'failed', 'partial']))
const resultOptions = computed(() => enumOptions('result', meta.meta.result_statuses))
const pageReport = computed(() => t('common.pageReport', { first: '{first}', last: '{last}', total: '{totalRecords}' }))

// ---------- editor ----------
const editorOpen = ref(false)
const editorId = ref<string | null>(null)
// PrimeVue's Drawer does not give focus back on close: do it here
let restoreFocus: ReturnType<typeof rememberFocus> | null = null
const pageHeading = () => document.getElementById('ws-title')
function openEditor(id: string | null) {
  if (!editorOpen.value) restoreFocus = rememberFocus()
  editorId.value = id
  editorOpen.value = true
  if (id && route.query.item !== id) router.replace({ query: { ...route.query, item: id } })
}
watch(editorOpen, (open) => {
  if (!open) {
    void nextTick(() => {
      restoreFocus?.(pageHeading)
      restoreFocus = null
    })
    editorId.value = null
    if (route.query.item) router.replace({ query: { ...route.query, item: undefined } })
  }
})
function onSaved(it: Requirement | TestCase) {
  load()
  if (it?.id && !editorId.value) openEditor(it.id)
}
function onDeleted() {
  editorOpen.value = false
  load()
}
function onCopied(it: { id: string }) {
  load()
  if (it?.id) openEditor(it.id)
}
const editorTitle = computed(() => {
  if (!editorId.value) return isReq.value ? t('workspace.newRequirement') : t('workspace.newTest')
  const it = items.value.find((i) => i.id === editorId.value)
  return it ? `${it.key} · ${it.title}` : isReq.value ? t('workspace.requirement') : t('workspace.test')
})
const folderLabel = computed(() => (selectedFolder.value ? folderPath(folders.value, selectedFolder.value) : isReq.value ? t('workspace.allRequirements') : t('workspace.allTests')))
</script>

<template>
  <div class="workspace">
    <aside v-if="!treeInDrawer" class="card tree-pane" :aria-label="t('folders.title')">
      <FolderTree
        v-model:selected="selectedFolder"
        :kind="kind"
        :project-id="pid"
        :folders="folders"
        :can-write="canWrite"
        @changed="onFoldersChanged"
        @drop-items="moveItems"
      />
    </aside>
    <section class="card" style="min-width: 0" :aria-busy="loading" aria-labelledby="ws-title">
      <div class="page-header" style="margin-bottom: 10px">
        <div>
          <h1 id="ws-title" class="ws-title" tabindex="-1">{{ folderLabel }}</h1>
          <div class="sub">{{ t(isReq ? 'counts.requirements' : 'counts.tests', items.length) }}</div>
        </div>
        <div class="actions">
          <HelpLink :page="isReq ? 'requirements' : 'tests'" />
          <Button
            v-if="treeInDrawer"
            :label="t('workspace.folders')"
            icon="pi pi-folder"
            severity="secondary"
            outlined
            aria-haspopup="dialog"
            :aria-expanded="foldersOpen"
            @click="foldersOpen = true"
          />
          <span v-if="selectedFolder" class="row small nowrap">
            <ToggleSwitch v-model="recursive" input-id="rec" /><label for="rec">{{ t('workspace.includeSubfolders') }}</label>
          </span>
          <SelectButton
            v-model="layoutMode"
            :options="layoutOptions"
            option-label="label"
            option-value="value"
            :allow-empty="false"
            :aria-label="t('sheet.layout')"
            class="layout-toggle"
          >
            <template #option="{ option }"><i :class="option.icon" aria-hidden="true" /><span>{{ option.label }}</span></template>
          </SelectButton>
          <ColumnsMenu
            v-if="layoutMode === 'list'"
            :columns="allColumns"
            :order="layout.order.value"
            @toggle="(id, on) => layout.toggle(id, on)"
            @move="(id, d) => layout.move(id, d)"
            @reset="layout.reset()"
          />
          <Button icon="pi pi-refresh" severity="secondary" text rounded :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="load" />
          <Button
            v-if="canWrite"
            :label="t('catalog.addFromCatalog')"
            icon="pi pi-book"
            severity="secondary"
            outlined
            class="catalog-entry"
            v-tooltip.bottom="t('catalog.addFromCatalogHint')"
            @click="router.push({ name: 'catalog', params: { projectId: pid }, query: isReq ? {} : { tab: 'tests' } })"
          />
          <Button
            v-if="exchangeItems.length"
            :label="t('reqImport.button')"
            icon="pi pi-upload"
            severity="secondary"
            outlined
            class="exchange-menu"
            aria-haspopup="menu"
            aria-controls="exchange-menu"
            v-tooltip.bottom="t('exchange.menuHint')"
            @click="exchangeMenu?.toggle($event)"
          />
          <Menu id="exchange-menu" ref="exchangeMenu" :model="exchangeItems" popup />
          <Button
            v-if="canReview && !selection.length && items.length"
            :label="t('bulkReview.toolbar')"
            icon="pi pi-verified"
            severity="secondary"
            outlined
            aria-haspopup="dialog"
            v-tooltip.bottom="isReq ? t('bulkReview.toolbarHintRequirement') : t('bulkReview.toolbarHintTest')"
            @click="bulkReviewOpen = true"
          />
          <Button v-if="canWrite" :label="isReq ? t('workspace.newRequirement') : t('workspace.newTest')" icon="pi pi-plus" @click="openEditor(null)" />
        </div>
      </div>

      <div class="filterbar">
        <QueryInput v-model="q" :scope="kind" :error="queryError" :composed="composed" @submit="load" />
      </div>
      <div class="filterbar">
        <MultiSelect v-model="quick.status" :options="statusOptions" option-label="label" option-value="value" :placeholder="t('workspace.filterStatus')" :aria-label="t('workspace.filterStatus')" :max-selected-labels="2" style="min-width: 140px" show-clear />
        <MultiSelect
          v-if="isReq"
          v-model="quick.obligation"
          :options="thirdOptions"
          option-label="label"
          option-value="value"
          :placeholder="t('workspace.filterObligation')" :aria-label="t('workspace.filterObligation')"
          :max-selected-labels="2"
          style="min-width: 140px"
          show-clear
        />
        <MultiSelect
          v-else
          v-model="quick.execution_type"
          :options="thirdOptions"
          option-label="label"
          option-value="value"
          :placeholder="t('workspace.filterExecution')" :aria-label="t('workspace.filterExecution')"
          style="min-width: 140px"
          show-clear
        />
        <MultiSelect v-model="quick.priority" :options="priorityOptions" option-label="label" option-value="value" :placeholder="t('workspace.filterPriority')" :aria-label="t('workspace.filterPriority')" :max-selected-labels="2" style="min-width: 130px" show-clear />
        <MultiSelect
          v-if="isReq"
          v-model="quick.coverage"
          :options="coverageOptions"
          option-label="label"
          option-value="value"
          :placeholder="t('workspace.filterCoverage')" :aria-label="t('workspace.filterCoverage')"
          style="min-width: 130px"
          show-clear
        />
        <MultiSelect
          v-else
          v-model="quick.last_result"
          :options="resultOptions"
          option-label="label"
          option-value="value"
          :placeholder="t('workspace.filterLastResult')" :aria-label="t('workspace.filterLastResult')"
          style="min-width: 130px"
          show-clear
        />
        <div style="min-width: 180px; max-width: 280px">
          <CategorySelect v-model="quick.categories" :categories="categories" value-key="path" :placeholder="t('workspace.filterCategory')" :aria-label="t('workspace.filterCategory')" />
        </div>
        <VariantSelect v-if="variantList.variants.value.length" v-model="variant" :variants="variantList.variants.value" :aria-label="t('variants.showFor')" />
        <Button v-if="activeFilterCount(quick) || q" :label="t('workspace.clearFilters')" icon="pi pi-filter-slash" text severity="secondary" @click="clearFilters" />
      </div>
      <div v-if="composed && composed !== q" class="small muted mono" style="margin: -4px 0 10px">q = {{ composed }}</div>

      <div v-if="selection.length && (canWrite || canReview)" class="bulkbar" role="toolbar" :aria-label="t('workspace.bulkActions')">
        <b>{{ t('common.selected', { count: selection.length }) }}</b>
        <Button
          v-if="canReview"
          :label="isReq ? t('bulkReview.buttonRequirement') : t('bulkReview.buttonTest')"
          icon="pi pi-verified"
          size="small"
          severity="success"
          outlined
          aria-haspopup="dialog"
          @click="bulkReviewOpen = true"
        />
        <template v-if="canWrite">
        <Select v-model="bulkStatus" :options="statusOptions" option-label="label" option-value="value" :placeholder="t('workspace.setStatus')" :aria-label="t('workspace.setStatus')" size="small" style="min-width: 140px" />
        <Select v-model="bulkPriority" :options="priorityOptions" option-label="label" option-value="value" :placeholder="t('workspace.setPriority')" :aria-label="t('workspace.setPriority')" size="small" style="min-width: 140px" />
        <Select v-model="bulkThird" :options="thirdOptions" option-label="label" option-value="value" :placeholder="isReq ? t('workspace.setObligation') : t('workspace.setExecution')" :aria-label="isReq ? t('workspace.setObligation') : t('workspace.setExecution')" size="small" style="min-width: 150px" />
        <Button :label="t('workspace.categoriesButton')" icon="pi pi-plus" size="small" severity="secondary" outlined :aria-label="t('workspace.addCategories')" v-tooltip.bottom="t('workspace.addCategories')" @click="openCatDialog('add')" />
        <Button :label="t('workspace.categoriesButton')" icon="pi pi-minus" size="small" severity="secondary" outlined :aria-label="t('workspace.removeCategories')" v-tooltip.bottom="t('workspace.removeCategories')" @click="openCatDialog('remove')" />
        <Button :label="t('workspace.tagsButton')" icon="pi pi-plus" size="small" severity="secondary" outlined :aria-label="t('workspace.addTags')" v-tooltip.bottom="t('workspace.addTags')" @click="openTagDialog('add')" />
        <Button :label="t('workspace.tagsButton')" icon="pi pi-minus" size="small" severity="secondary" outlined :aria-label="t('workspace.removeTags')" v-tooltip.bottom="t('workspace.removeTags')" @click="openTagDialog('remove')" />
        <Button :label="t('common.move')" icon="pi pi-folder" size="small" severity="secondary" outlined @click="moveDialog = true" />
        <Button :label="t('links.title')" icon="pi pi-link" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="bulkLinksOpen = true" />
        <Button v-if="variantList.variants.value.length" :label="t('variants.appliesTo')" icon="pi pi-clone" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="bulkApplicabilityOpen = true" />
        <Button v-if="isReq" :label="t('relations.bulk.button')" icon="pi pi-sitemap" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="bulkRelationsOpen = true" />
        <Button v-if="isReq && canWrite" :label="t('reqIssue.bulk.button')" icon="pi pi-ticket" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="bulkIssuesOpen = true" />
        <Button
          v-if="isSoftware(proj.project)"
          :label="t('prompts.forSelection')"
          icon="pi pi-sparkles"
          size="small"
          severity="secondary"
          outlined
          @click="$router.push({ name: 'prompts', params: { projectId: pid }, query: { kind: isReq ? 'implement' : 'automate_tests', ids: selectedIds.join(',') } })"
        />
        <Button :label="t('common.delete')" icon="pi pi-trash" size="small" severity="danger" outlined @click="bulkDelete" />
        </template>
        <Button icon="pi pi-times" size="small" text rounded severity="secondary" :aria-label="t('workspace.clearSelection')" v-tooltip.bottom="t('workspace.clearSelection')" @click="selection = []" />
      </div>

      <ItemSheet
        v-if="layoutMode === 'sheet'"
        :kind="kind"
        :project-id="pid"
        :items="sheetItems"
        :selected-ids="selectionIds"
        :folders="folders"
        :categories="categories"
        :folder-id="selectedFolder"
        :can-write="canWrite"
        :can-review="canReview"
        :can-export="canExport"
        :filter="composed"
        @replace="onSheetReplace"
        @created="onSheetCreated"
        @select="onSheetSelect"
        @open="openEditor"
      />
      <DataTable
        v-else
        :ref="layout.bind"
        v-model:selection="selection"
        :row-class="folderRowClass"
        :row-style="folderRowStyle"
        v-stack
        reorderable-columns
        @column-reorder="layout.onReorder()"
        :value="items"
        data-key="id"
        :loading="loading"
        paginator
        :rows="prefs.rows"
        :rows-per-page-options="prefs.rowsPerPage"
        removable-sort
        sort-field="key"
        :sort-order="1"
        row-hover
        scrollable
        size="small"
        striped-rows
        :meta-key-selection="false"
        paginator-template="FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink RowsPerPageDropdown CurrentPageReport"
        :current-page-report-template="pageReport"
        @row-click="(e) => openEditor((e.data as Item).id)"
      >
        <Column selection-mode="multiple" column-key="_select" :reorderable-column="false" style="width: 2.6rem" />
        <Column v-if="canWrite" column-key="_drag" :reorderable-column="false" style="width: 2rem">
          <template #body="{ data }">
            <i class="pi pi-bars drag-handle" draggable="true" aria-hidden="true" :title="t('workspace.dragToFolder')" @dragstart="onRowDrag($event, data as Item)" @click.stop />
          </template>
        </Column>
        <Column field="key" :header="t('workspace.colKey')" :reorderable-column="false" sortable style="width: 130px">
          <template #body="{ data }"><button type="button" class="link-button key-link" tabindex="-1" @click.stop="openEditor((data as Item).id)"><span class="key">{{ data.key }}</span></button></template>
        </Column>
        <Column field="title" :header="t('workspace.colTitle')" :reorderable-column="false" sortable style="min-width: 240px" body-class="stack-main">
          <template #body="{ data }">
            <button type="button" class="link-button item-title" :aria-label="t('workspace.openItem', { key: data.key, title: data.title })" @click.stop="openEditor((data as Item).id)">{{ data.title }}</button>
          </template>
        </Column>
        <Column v-for="c in visibleColumns" :key="c.field" :column-key="c.field" :field="c.field" :sort-field="c.sortField ?? c.field" :header="c.header" :sortable="c.sortable !== false" :style="c.width ? { width: c.width } : {}">
          <template #body="{ data }">
            <StatusTag v-if="c.field === 'obligation'" :value="data.obligation" kind="obligation" />
            <StatusTag v-else-if="c.field === 'status'" :value="data.status" />
            <StatusTag v-else-if="c.field === 'priority'" :value="data.priority" kind="priority" />
            <span v-else-if="c.field === 'coverage'" class="row nowrap" style="gap: 4px">
              <StatusTag :value="data.coverage" kind="coverage" />
              <StaleBadge :stale="!!data.stale_results" />
              <span v-if="data.blocking_defects" class="blocked-badge small" :title="t('correlations.blockedCount', { n: data.blocking_defects }, data.blocking_defects)">
                <i class="pi pi-ban" aria-hidden="true" />{{ t('correlations.blocked') }}<span class="sr-only">: {{ t('correlations.blockedCount', { n: data.blocking_defects }, data.blocking_defects) }}</span>
              </span>
            </span>
            <StatusTag v-else-if="c.field === 'verification'" :value="(data as Requirement).verification" kind="verification" empty="" />
            <span v-else-if="c.field === 'last_result'" class="row nowrap" style="gap: 4px">
              <StatusTag :value="data.last_result" kind="result" />
              <StaleBadge :stale="data.stale" :reason="data.stale_reason" />
              <span v-if="data.flaky" class="flaky-badge" :title="t('workspace.flakyHint', data.flakiness ?? 0)" :aria-label="t('workspace.flakyHint', data.flakiness ?? 0)">
                <i class="pi pi-sync" aria-hidden="true" /> {{ t('workspace.flaky') }}
              </span>
            </span>
            <span v-else-if="c.field === 'execution_type'" class="row nowrap small">
              <i :class="data.execution_type === 'automated' ? 'pi pi-bolt' : 'pi pi-user'" class="muted" />{{ enumLabel('executionType', data.execution_type) }}
            </span>
            <CategoryChips v-else-if="c.field === 'category_ids'" :ids="data.category_ids" :categories="categories" />
            <span v-else-if="c.field === 'updated_at' || c.field === 'created_at'" class="small muted">{{ formatDate(data[c.field]) }}</span>
            <RelationsCell v-else-if="c.field === 'relations'" :refs="data.relations" :project-id="pid" />
            <MentionText v-else-if="c.field === 'owner'" :text="data.owner" class="small" />
            <span v-else-if="c.field === 'tags'" class="row small" style="gap: 4px">
              <span v-for="tag in data.tags ?? []" :key="tag" class="tag-chip" :class="tagClass(tag)">{{ tag }}</span>
            </span>
            <DefectLinks v-else-if="c.field === 'defects'" :defects="data.defects" class="small" />
            <span v-else-if="c.field === 'script_language'" class="small mono">{{ data.execution_type === 'automated' ? data.script_language : '' }}</span>
            <span v-else-if="c.field === 'criteria_count' || c.field === 'step_count'" class="small" :class="{ muted: !data[c.field] }">{{ data[c.field] ?? 0 }}</span>
            <span v-else-if="c.field === 'created_by' || c.field === 'source'" class="small">{{ data[c.field] }}</span>
            <span v-else-if="c.field === 'type'" class="small">{{ enumLabel('requirementType', data.type) }}</span>
            <span v-else-if="c.field === 'folder_path'" class="small muted row nowrap" style="gap: 6px"><span v-if="rowFolderColor(data)" class="folder-swatch" aria-hidden="true" :style="{ background: rowFolderColor(data) }" />{{ data.folder_path }}</span>
            <span v-else-if="c.field === 'suspect_links'" class="small" :class="data.suspect_links ? 'suspect' : 'muted'">
              <i v-if="data.suspect_links" class="pi pi-exclamation-triangle" aria-hidden="true" /> {{ data.suspect_links ?? 0 }}
            </span>
            <Tag v-else-if="c.field === 'risk_level' && data.risk_level" :value="t(`risksView.level.${data.risk_level}`)" :severity="riskLevelSeverity(data.risk_level)" />
            <span v-else-if="c.field === 'risk_level'" class="muted small">—</span>
            <span v-else-if="c.field === 'open_defects'" class="small" :class="data.open_defects ? 'suspect' : 'muted'">
              <i v-if="data.open_defects" class="pi pi-exclamation-circle" aria-hidden="true" /> {{ data.open_defects ?? 0 }}
            </span>
            <span v-else-if="c.field === 'comment_count' || c.field === 'relation_count'" class="small" :class="{ muted: !data[c.field] }">
              <i :class="c.field === 'comment_count' ? 'pi pi-comments' : 'pi pi-sitemap'" class="muted" style="font-size: 11px" /> {{ data[c.field] ?? 0 }}
            </span>
            <span v-else-if="isCfField(c.field)" class="small">{{ cellText(data, c.field) }}</span>
            <span v-else>{{ data[c.field] }}</span>
          </template>
        </Column>
        <template #empty>
          <div class="empty">
            <i class="pi pi-inbox" />
            <div>{{ isReq ? (composed ? t('workspace.emptyRequirementsFilter') : t('workspace.emptyRequirements')) : composed ? t('workspace.emptyTestsFilter') : t('workspace.emptyTests') }}</div>
            <div v-if="!composed && !selectedFolder && canWrite" class="small" style="margin-top: 8px">
              {{ t('catalog.emptyProjectHint') }}
              <RouterLink :to="{ name: 'catalog', params: { projectId: pid }, query: isReq ? {} : { tab: 'tests' } }">{{ t('catalog.addFromCatalog') }}</RouterLink>
            </div>
          </div>
        </template>
      </DataTable>
    </section>
  </div>

  <Drawer v-if="treeInDrawer" v-model:visible="foldersOpen" position="left" class="folders-drawer" :header="t('folders.title')" block-scroll>
    <FolderTree
      v-model:selected="selectedFolder"
      :kind="kind"
      :project-id="pid"
      :folders="folders"
      :can-write="canWrite"
      hide-title
      @changed="onFoldersChanged"
      @drop-items="moveItems"
    />
  </Drawer>
  <Drawer v-model:visible="editorOpen" position="right" class="editor-drawer" :header="editorTitle" block-scroll>
    <RequirementEditor
      v-if="isReq && editorOpen"
      :project-id="pid"
      :item-id="editorId"
      :folder-id="selectedFolder"
      :categories="categories"
      :folders="folders"
      :can-write="canWrite"
      :can-link="canLink"
      @saved="onSaved"
      @deleted="onDeleted"
      @copied="onCopied"
      @close="editorOpen = false"
    />
    <TestEditor
      v-else-if="editorOpen"
      :project-id="pid"
      :item-id="editorId"
      :folder-id="selectedFolder"
      :categories="categories"
      :folders="folders"
      :can-write="canWrite"
      :can-link="canLink"
      @saved="onSaved"
      @deleted="onDeleted"
      @copied="onCopied"
      @close="editorOpen = false"
    />
  </Drawer>

  <Dialog v-model:visible="tagDialog" :header="tagMode === 'add' ? t('workspace.addTags') : t('workspace.removeTags')" modal style="width: min(480px, 95vw)">
    <div class="grid" style="gap: 10px">
      <TagsInput v-model="tagValues" :aria-label="t('common.tags')" />
      <div class="small muted">{{ t(isReq ? 'workspace.appliesToRequirements' : 'workspace.appliesToTests', selection.length) }}<template v-if="tagMode === 'remove'"> · {{ t('workspace.removeTagsHint') }}</template></div>
    </div>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="tagDialog = false" />
      <Button :label="t('common.apply')" icon="pi pi-check" :disabled="!tagValues.length" @click="applyTags" />
    </template>
  </Dialog>
  <Dialog v-model:visible="catDialog" :header="catMode === 'add' ? t('workspace.addCategories') : t('workspace.removeCategories')" modal style="width: min(480px, 95vw)">
    <div class="grid" style="gap: 12px">
      <SelectButton
        v-model="catMode"
        :options="[
          { l: t('common.add'), v: 'add' },
          { l: t('common.remove'), v: 'remove' },
        ]"
        option-label="l"
        option-value="v"
        :allow-empty="false"
      />
      <CategorySelect v-model="catIds" :categories="categories" :aria-label="t('common.categories')" />
      <div class="small muted">{{ t(isReq ? 'workspace.appliesToRequirements' : 'workspace.appliesToTests', selection.length) }}</div>
    </div>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="catDialog = false" />
      <Button :label="t('common.apply')" icon="pi pi-check" :disabled="!catIds.length" @click="applyCats" />
    </template>
  </Dialog>
  <FolderPickerDialog v-model:visible="moveDialog" :folders="folders" :header="t('workspace.moveSelection')" @pick="onPickFolder" />
  <RequirementImportDialog v-if="isReq && canWrite" v-model:visible="importOpen" :project-id="pid" :folders="folders" :folder-id="selectedFolder" @imported="onImported" />
  <ExchangeImportDialog
    v-if="canWrite"
    v-model:visible="exchangeOpen"
    :project-id="pid"
    :kind="kind"
    :mode="exchangeMode"
    :folders="folders"
    :folder-id="selectedFolder"
    :can-export="canExport"
    @imported="onImported"
  />
  <ApplicabilityDialog
    v-model:visible="bulkApplicabilityOpen"
    :project-id="pid"
    :item-type="kind"
    :item-ids="selectedIds"
    :variants="variantList.variants.value"
    @saved="() => { notify.ok(t('variants.applicabilitySaved')); load() }"
  />
  <BulkReviewDialog
    v-model:visible="bulkReviewOpen"
    :kind="isReq ? 'requirement' : 'test'"
    :project-id="pid"
    :selected="selectedIds"
    :shown="shownIds"
    @done="onBulkReviewed"
  />
  <BulkItemLinksDialog v-if="canWrite" v-model:visible="bulkLinksOpen" :source-type="isReq ? 'requirement' : 'test'" :project-id="pid" :selected="selectedIds" />
  <BulkRelationsDialog v-if="isReq && canWrite" v-model:visible="bulkRelationsOpen" :project-id="pid" :selected="selectedIds" @done="load" />
  <BulkRequirementIssuesDialog v-if="isReq && canWrite" v-model:visible="bulkIssuesOpen" :project-id="pid" :selected="selectedIds" />
</template>
