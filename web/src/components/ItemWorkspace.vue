<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import MultiSelect from 'primevue/multiselect'
import Select from 'primevue/select'
import ToggleSwitch from 'primevue/toggleswitch'
import Drawer from 'primevue/drawer'
import Dialog from 'primevue/dialog'
import SelectButton from 'primevue/selectbutton'
import FolderTree from './FolderTree.vue'
import FolderPickerDialog from './FolderPickerDialog.vue'
import QueryInput from './QueryInput.vue'
import StatusTag from './StatusTag.vue'
import CategoryChips from './CategoryChips.vue'
import CategorySelect from './CategorySelect.vue'
import RequirementEditor from './RequirementEditor.vue'
import TestEditor from './TestEditor.vue'
import { api, ApiError, asItems } from '@/lib/api'
import { activeFilterCount, composeQuery, type QuickFilters } from '@/lib/query'
import { dragIds, encodeIds, ITEMS_MIME } from '@/lib/dnd'
import { folderPath } from '@/lib/tree'
import { formatDate } from '@/lib/format'
import { useI18n } from 'vue-i18n'
import { enumLabel, enumOptions } from '@/i18n/labels'
import { P } from '@/lib/permissions'
import type { ItemKind, Requirement, TestCase } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useMetaStore } from '@/stores/meta'
import { useAuthStore } from '@/stores/auth'
import { readPref, writePref } from '@/stores/ui'
import { useNotify } from '@/composables/notify'
import { usePrefsStore } from '@/stores/prefs'
import { useBreakpoint } from '@/lib/breakpoint'
import { rememberFocus } from '@/lib/a11y'
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
const canLink = computed(() => auth.can(P.requirementWrite) || auth.can(P.testWrite))
const folders = computed(() => proj.folders[props.kind])
const categories = computed(() => proj.categoriesFor(props.kind))

// ---------- filter state ----------
const selectedFolder = ref<string | null>((route.query.folder as string) || null)
const recursive = ref(readPref('qtf.recursive') !== 'false')
const q = ref((route.query.q as string) || '')
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

watch([selectedFolder, recursive, () => quick.value], () => load(), { deep: true })
watch(selectedFolder, () => {
  foldersOpen.value = false
})
watch(recursive, (v) => writePref('qtf.recursive', String(v)))
watch([selectedFolder, q], () => {
  const query = { ...route.query, folder: selectedFolder.value || undefined, q: q.value || undefined }
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
}
const allColumns = computed<Col[]>(() =>
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
        { field: 'version', header: t('workspace.colVersion'), width: '70px' },
        { field: 'relation_count', header: t('workspace.colRelations'), width: '105px' },
        { field: 'comment_count', header: t('workspace.colComments'), width: '120px' },
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
        { field: 'version', header: t('workspace.colVersion'), width: '70px' },
        { field: 'comment_count', header: t('workspace.colComments'), width: '120px' },
        { field: 'updated_at', header: t('workspace.colUpdated'), width: '115px' },
      ],
)
const defaultCols = computed(() =>
  isReq.value
    ? ['obligation', 'status', 'priority', 'coverage', 'linked_tests', 'category_ids']
    : ['execution_type', 'status', 'priority', 'last_result', 'linked_requirements', 'category_ids'],
)
const colKey = computed(() => `qtf.cols.${props.kind}`)
const visibleFields = ref<string[]>(loadCols())
function loadCols(): string[] {
  try {
    const v = JSON.parse(readPref(`qtf.cols.${props.kind}`) || 'null')
    if (Array.isArray(v)) return v
  } catch {
    /* ignore */
  }
  return defaultCols.value
}
watch(visibleFields, (v) => writePref(colKey.value, JSON.stringify(v)))
const visibleColumns = computed(() => allColumns.value.filter((c) => visibleFields.value.includes(c.field)))

// ---------- drag rows to folders ----------
function onRowDrag(e: DragEvent, row: Item) {
  if (!e.dataTransfer || !canWrite.value) return
  const ids = dragIds(row.id, selection.value)
  e.dataTransfer.setData(ITEMS_MIME, encodeIds(ids))
  e.dataTransfer.setData('text/plain', ids.length > 1 ? t(isReq.value ? 'workspace.dragRequirements' : 'workspace.dragTests', { count: ids.length }) : row.key)
  e.dataTransfer.effectAllowed = 'move'
}

async function moveItems(ids: string[], folderId: string | null) {
  try {
    await api.post(`/projects/${pid.value}/${plural.value}/move`, { ids, folder_id: folderId })
    notify.ok(t(isReq.value ? 'workspace.movedRequirements' : 'workspace.movedTests', ids.length), folderId ? folderPath(folders.value, folderId) : t('common.topLevel'))
    await load()
  } catch (e) {
    notify.fail(e, t('workspace.moveFailed'))
  }
}

async function onFoldersChanged() {
  await proj.loadFolders(props.kind)
  if (selectedFolder.value && !folders.value.some((f) => f.id === selectedFolder.value)) selectedFolder.value = null
  await load()
}

// ---------- bulk actions ----------
const selectedIds = computed(() => selection.value.map((s) => s.id))
async function bulkSet(set: Record<string, unknown>) {
  if (!selectedIds.value.length) return
  try {
    await api.post(`/projects/${pid.value}/${plural.value}/bulk`, { ids: selectedIds.value, set })
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
      await api.del(`/projects/${pid.value}/${plural.value}/bulk`, { ids: selectedIds.value })
      selection.value = []
      notify.ok(t('workspace.deletedCount', { count: n }))
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
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
function onCopied(it: TestCase) {
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
          <MultiSelect
            v-model="visibleFields"
            :options="allColumns"
            option-label="header"
            option-value="field"
            :placeholder="t('common.columns')"
            :max-selected-labels="0"
            :selected-items-label="t('common.columns')"
            style="min-width: 130px"
            :aria-label="t('common.columns')"
          />
          <Button icon="pi pi-refresh" severity="secondary" text rounded :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="load" />
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
        <Button v-if="activeFilterCount(quick) || q" :label="t('workspace.clearFilters')" icon="pi pi-filter-slash" text severity="secondary" @click="clearFilters" />
      </div>
      <div v-if="composed && composed !== q" class="small muted mono" style="margin: -4px 0 10px">q = {{ composed }}</div>

      <div v-if="selection.length && canWrite" class="bulkbar" role="toolbar" :aria-label="t('workspace.bulkActions')">
        <b>{{ t('common.selected', { count: selection.length }) }}</b>
        <Select v-model="bulkStatus" :options="statusOptions" option-label="label" option-value="value" :placeholder="t('workspace.setStatus')" :aria-label="t('workspace.setStatus')" size="small" style="min-width: 140px" />
        <Select v-model="bulkPriority" :options="priorityOptions" option-label="label" option-value="value" :placeholder="t('workspace.setPriority')" :aria-label="t('workspace.setPriority')" size="small" style="min-width: 140px" />
        <Select v-model="bulkThird" :options="thirdOptions" option-label="label" option-value="value" :placeholder="isReq ? t('workspace.setObligation') : t('workspace.setExecution')" :aria-label="isReq ? t('workspace.setObligation') : t('workspace.setExecution')" size="small" style="min-width: 150px" />
        <Button :label="t('workspace.categoriesButton')" icon="pi pi-plus" size="small" severity="secondary" outlined :aria-label="t('workspace.addCategories')" v-tooltip.bottom="t('workspace.addCategories')" @click="openCatDialog('add')" />
        <Button :label="t('workspace.categoriesButton')" icon="pi pi-minus" size="small" severity="secondary" outlined :aria-label="t('workspace.removeCategories')" v-tooltip.bottom="t('workspace.removeCategories')" @click="openCatDialog('remove')" />
        <Button :label="t('common.move')" icon="pi pi-folder" size="small" severity="secondary" outlined @click="moveDialog = true" />
        <Button :label="t('common.delete')" icon="pi pi-trash" size="small" severity="danger" outlined @click="bulkDelete" />
        <Button icon="pi pi-times" size="small" text rounded severity="secondary" :aria-label="t('workspace.clearSelection')" v-tooltip.bottom="t('workspace.clearSelection')" @click="selection = []" />
      </div>

      <DataTable
        v-model:selection="selection"
        v-stack
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
        <Column selection-mode="multiple" style="width: 2.6rem" />
        <Column v-if="canWrite" style="width: 2rem">
          <template #body="{ data }">
            <i class="pi pi-bars drag-handle" draggable="true" aria-hidden="true" :title="t('workspace.dragToFolder')" @dragstart="onRowDrag($event, data as Item)" @click.stop />
          </template>
        </Column>
        <Column field="key" :header="t('workspace.colKey')" sortable style="width: 130px">
          <template #body="{ data }"><span class="key">{{ data.key }}</span></template>
        </Column>
        <Column field="title" :header="t('workspace.colTitle')" sortable style="min-width: 240px" body-class="stack-main">
          <template #body="{ data }">
            <button type="button" class="link-button item-title" :aria-label="t('workspace.openItem', { key: data.key, title: data.title })" @click.stop="openEditor((data as Item).id)">{{ data.title }}</button>
          </template>
        </Column>
        <Column v-for="c in visibleColumns" :key="c.field" :field="c.field" :header="c.header" :sortable="c.sortable !== false" :style="c.width ? { width: c.width } : {}">
          <template #body="{ data }">
            <StatusTag v-if="c.field === 'obligation'" :value="data.obligation" kind="obligation" />
            <StatusTag v-else-if="c.field === 'status'" :value="data.status" />
            <StatusTag v-else-if="c.field === 'priority'" :value="data.priority" kind="priority" />
            <StatusTag v-else-if="c.field === 'coverage'" :value="data.coverage" kind="coverage" />
            <StatusTag v-else-if="c.field === 'last_result'" :value="data.last_result" kind="result" />
            <span v-else-if="c.field === 'execution_type'" class="row nowrap small">
              <i :class="data.execution_type === 'automated' ? 'pi pi-bolt' : 'pi pi-user'" class="muted" />{{ enumLabel('executionType', data.execution_type) }}
            </span>
            <CategoryChips v-else-if="c.field === 'category_ids'" :ids="data.category_ids" :categories="categories" />
            <span v-else-if="c.field === 'updated_at'" class="small muted">{{ formatDate(data.updated_at) }}</span>
            <span v-else-if="c.field === 'type'" class="small">{{ enumLabel('requirementType', data.type) }}</span>
            <span v-else-if="c.field === 'folder_path'" class="small muted">{{ data.folder_path }}</span>
            <span v-else-if="c.field === 'comment_count' || c.field === 'relation_count'" class="small" :class="{ muted: !data[c.field] }">
              <i :class="c.field === 'comment_count' ? 'pi pi-comments' : 'pi pi-sitemap'" class="muted" style="font-size: 11px" /> {{ data[c.field] ?? 0 }}
            </span>
            <span v-else>{{ data[c.field] }}</span>
          </template>
        </Column>
        <template #empty>
          <div class="empty">
            <i class="pi pi-inbox" />
            <div>{{ isReq ? (composed ? t('workspace.emptyRequirementsFilter') : t('workspace.emptyRequirements')) : composed ? t('workspace.emptyTestsFilter') : t('workspace.emptyTests') }}</div>
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
</template>
