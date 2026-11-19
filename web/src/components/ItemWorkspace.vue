<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
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
import { formatDate, humanize, options, pluralize } from '@/lib/format'
import { P } from '@/lib/permissions'
import type { ItemKind, Requirement, TestCase } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useMetaStore } from '@/stores/meta'
import { useAuthStore } from '@/stores/auth'
import { readPref, writePref } from '@/stores/ui'
import { useNotify } from '@/composables/notify'

type Item = Requirement & TestCase

const props = defineProps<{ kind: ItemKind }>()
const proj = useProjectStore()
const meta = useMetaStore()
const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const notify = useNotify()

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
    if (!(e instanceof ApiError && e.code === 'invalid_query')) notify.fail(e, `Could not load ${plural.value}`)
  } finally {
    loading.value = false
  }
}

watch([selectedFolder, recursive, () => quick.value], () => load(), { deep: true })
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
        { field: 'obligation', header: 'Obligation', width: '120px' },
        { field: 'status', header: 'Status', width: '120px' },
        { field: 'priority', header: 'Priority', width: '100px' },
        { field: 'type', header: 'Type', width: '120px' },
        { field: 'coverage', header: 'Coverage', width: '110px' },
        { field: 'linked_tests', header: 'Tests', width: '80px' },
        { field: 'category_ids', header: 'Categories', sortable: false },
        { field: 'folder_path', header: 'Folder' },
        { field: 'owner', header: 'Owner' },
        { field: 'version', header: 'Ver.', width: '70px' },
        { field: 'updated_at', header: 'Updated', width: '110px' },
      ]
    : [
        { field: 'execution_type', header: 'Type', width: '120px' },
        { field: 'status', header: 'Status', width: '110px' },
        { field: 'priority', header: 'Priority', width: '100px' },
        { field: 'last_result', header: 'Last result', width: '120px' },
        { field: 'linked_requirements', header: 'Reqs', width: '80px' },
        { field: 'category_ids', header: 'Categories', sortable: false },
        { field: 'folder_path', header: 'Folder' },
        { field: 'estimated_minutes', header: 'Est. min', width: '90px' },
        { field: 'updated_at', header: 'Updated', width: '110px' },
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
  e.dataTransfer.setData('text/plain', ids.length > 1 ? `${ids.length} ${plural.value}` : row.key)
  e.dataTransfer.effectAllowed = 'move'
}

async function moveItems(ids: string[], folderId: string | null) {
  try {
    await api.post(`/projects/${pid.value}/${plural.value}/move`, { ids, folder_id: folderId })
    notify.ok(`Moved ${pluralize(ids.length, isReq.value ? 'requirement' : 'test')}`, folderId ? folderPath(folders.value, folderId) : 'Top level')
    await load()
  } catch (e) {
    notify.fail(e, 'Move failed')
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
    notify.ok(`Updated ${pluralize(selectedIds.value.length, isReq.value ? 'requirement' : 'test')}`)
    await load()
  } catch (e) {
    notify.fail(e, 'Bulk update failed')
  }
}
function bulkDelete() {
  const n = selectedIds.value.length
  notify.confirmDanger(`Delete ${pluralize(n, isReq.value ? 'requirement' : 'test')}? This cannot be undone.`, async () => {
    try {
      await api.del(`/projects/${pid.value}/${plural.value}/bulk`, { ids: selectedIds.value })
      selection.value = []
      notify.ok(`Deleted ${n}`)
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

const statusOptions = computed(() => options(isReq.value ? meta.meta.requirement_statuses : meta.meta.test_statuses))
const thirdOptions = computed(() => (isReq.value ? options(meta.meta.obligations) : options(['manual', 'automated'])))

// ---------- editor ----------
const editorOpen = ref(false)
const editorId = ref<string | null>(null)
function openEditor(id: string | null) {
  editorId.value = id
  editorOpen.value = true
  if (id && route.query.item !== id) router.replace({ query: { ...route.query, item: id } })
}
watch(editorOpen, (open) => {
  if (!open) {
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
  if (!editorId.value) return isReq.value ? 'New requirement' : 'New test'
  const it = items.value.find((i) => i.id === editorId.value)
  return it ? `${it.key} · ${it.title}` : isReq.value ? 'Requirement' : 'Test'
})
const folderLabel = computed(() => (selectedFolder.value ? folderPath(folders.value, selectedFolder.value) : `All ${plural.value}`))
</script>

<template>
  <div class="workspace">
    <aside class="card tree-pane">
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
    <section class="card" style="min-width: 0">
      <div class="page-header" style="margin-bottom: 10px">
        <div>
          <h1 style="font-size: 17px">{{ folderLabel }}</h1>
          <div class="sub">{{ pluralize(items.length, isReq ? 'requirement' : 'test') }}</div>
        </div>
        <div class="actions">
          <span v-if="selectedFolder" class="row small">
            <ToggleSwitch v-model="recursive" input-id="rec" /><label for="rec">Include subfolders</label>
          </span>
          <MultiSelect
            v-model="visibleFields"
            :options="allColumns"
            option-label="header"
            option-value="field"
            placeholder="Columns"
            :max-selected-labels="0"
            selected-items-label="Columns"
            style="width: 130px"
            aria-label="Columns"
          />
          <Button icon="pi pi-refresh" severity="secondary" text rounded aria-label="Refresh" v-tooltip="'Refresh'" @click="load" />
          <Button v-if="canWrite" :label="isReq ? 'New requirement' : 'New test'" icon="pi pi-plus" @click="openEditor(null)" />
        </div>
      </div>

      <div class="filterbar">
        <QueryInput v-model="q" :scope="kind" :error="queryError" :composed="composed" @submit="load" />
      </div>
      <div class="filterbar">
        <MultiSelect v-model="quick.status" :options="statusOptions" option-label="label" option-value="value" placeholder="Status" :max-selected-labels="2" style="min-width: 140px" show-clear />
        <MultiSelect
          v-if="isReq"
          v-model="quick.obligation"
          :options="options(meta.meta.obligations)"
          option-label="label"
          option-value="value"
          placeholder="Obligation"
          :max-selected-labels="2"
          style="min-width: 140px"
          show-clear
        />
        <MultiSelect
          v-else
          v-model="quick.execution_type"
          :options="options(['manual', 'automated'])"
          option-label="label"
          option-value="value"
          placeholder="Execution"
          style="min-width: 140px"
          show-clear
        />
        <MultiSelect v-model="quick.priority" :options="options(meta.meta.priorities)" option-label="label" option-value="value" placeholder="Priority" :max-selected-labels="2" style="min-width: 130px" show-clear />
        <MultiSelect
          v-if="isReq"
          v-model="quick.coverage"
          :options="options(['none', 'not_run', 'passed', 'failed', 'partial'])"
          option-label="label"
          option-value="value"
          placeholder="Coverage"
          style="min-width: 130px"
          show-clear
        />
        <MultiSelect
          v-else
          v-model="quick.last_result"
          :options="options(meta.meta.result_statuses)"
          option-label="label"
          option-value="value"
          placeholder="Last result"
          style="min-width: 130px"
          show-clear
        />
        <div style="min-width: 180px; max-width: 280px">
          <CategorySelect v-model="quick.categories" :categories="categories" value-key="path" placeholder="Category" />
        </div>
        <Button v-if="activeFilterCount(quick) || q" label="Clear" icon="pi pi-filter-slash" text severity="secondary" @click="clearFilters" />
      </div>
      <div v-if="composed && composed !== q" class="small muted mono" style="margin: -4px 0 10px">q = {{ composed }}</div>

      <div v-if="selection.length && canWrite" class="bulkbar">
        <b>{{ selection.length }} selected</b>
        <Select v-model="bulkStatus" :options="statusOptions" option-label="label" option-value="value" placeholder="Set status" size="small" style="width: 140px" />
        <Select v-model="bulkPriority" :options="options(meta.meta.priorities)" option-label="label" option-value="value" placeholder="Set priority" size="small" style="width: 140px" />
        <Select v-model="bulkThird" :options="thirdOptions" option-label="label" option-value="value" :placeholder="isReq ? 'Set obligation' : 'Set execution'" size="small" style="width: 150px" />
        <Button label="Categories" icon="pi pi-plus" size="small" severity="secondary" outlined @click="openCatDialog('add')" />
        <Button label="Categories" icon="pi pi-minus" size="small" severity="secondary" outlined @click="openCatDialog('remove')" />
        <Button label="Move" icon="pi pi-folder" size="small" severity="secondary" outlined @click="moveDialog = true" />
        <Button label="Delete" icon="pi pi-trash" size="small" severity="danger" outlined @click="bulkDelete" />
        <Button icon="pi pi-times" size="small" text rounded severity="secondary" aria-label="Clear selection" @click="selection = []" />
      </div>

      <DataTable
        v-model:selection="selection"
        :value="items"
        data-key="id"
        :loading="loading"
        paginator
        :rows="25"
        :rows-per-page-options="[25, 50, 100, 250]"
        removable-sort
        sort-field="key"
        :sort-order="1"
        row-hover
        scrollable
        size="small"
        striped-rows
        :meta-key-selection="false"
        paginator-template="FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink RowsPerPageDropdown CurrentPageReport"
        current-page-report-template="{first}–{last} of {totalRecords}"
        @row-click="(e) => openEditor((e.data as Item).id)"
      >
        <Column selection-mode="multiple" style="width: 2.6rem" />
        <Column v-if="canWrite" style="width: 2rem">
          <template #body="{ data }">
            <i class="pi pi-bars drag-handle" draggable="true" title="Drag to a folder" @dragstart="onRowDrag($event, data as Item)" @click.stop />
          </template>
        </Column>
        <Column field="key" header="Key" sortable style="width: 130px">
          <template #body="{ data }"><span class="key">{{ data.key }}</span></template>
        </Column>
        <Column field="title" header="Title" sortable style="min-width: 240px">
          <template #body="{ data }"><span class="clickable" style="font-weight: 500">{{ data.title }}</span></template>
        </Column>
        <Column v-for="c in visibleColumns" :key="c.field" :field="c.field" :header="c.header" :sortable="c.sortable !== false" :style="c.width ? { width: c.width } : {}">
          <template #body="{ data }">
            <StatusTag v-if="c.field === 'obligation'" :value="data.obligation" kind="obligation" />
            <StatusTag v-else-if="c.field === 'status'" :value="data.status" />
            <StatusTag v-else-if="c.field === 'priority'" :value="data.priority" kind="priority" />
            <StatusTag v-else-if="c.field === 'coverage'" :value="data.coverage" kind="coverage" />
            <StatusTag v-else-if="c.field === 'last_result'" :value="data.last_result" kind="result" />
            <span v-else-if="c.field === 'execution_type'" class="row nowrap small">
              <i :class="data.execution_type === 'automated' ? 'pi pi-bolt' : 'pi pi-user'" class="muted" />{{ humanize(data.execution_type) }}
            </span>
            <CategoryChips v-else-if="c.field === 'category_ids'" :ids="data.category_ids" :categories="categories" />
            <span v-else-if="c.field === 'updated_at'" class="small muted">{{ formatDate(data.updated_at) }}</span>
            <span v-else-if="c.field === 'type'" class="small">{{ humanize(data.type) }}</span>
            <span v-else-if="c.field === 'folder_path'" class="small muted">{{ data.folder_path }}</span>
            <span v-else>{{ data[c.field] }}</span>
          </template>
        </Column>
        <template #empty>
          <div class="empty">
            <i class="pi pi-inbox" />
            <div>No {{ plural }} {{ composed ? 'match the filter' : 'here yet' }}.</div>
          </div>
        </template>
      </DataTable>
    </section>
  </div>

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

  <Dialog v-model:visible="catDialog" :header="catMode === 'add' ? 'Add categories' : 'Remove categories'" modal style="width: min(480px, 95vw)">
    <div class="grid" style="gap: 12px">
      <SelectButton
        v-model="catMode"
        :options="[
          { l: 'Add', v: 'add' },
          { l: 'Remove', v: 'remove' },
        ]"
        option-label="l"
        option-value="v"
        :allow-empty="false"
      />
      <CategorySelect v-model="catIds" :categories="categories" />
      <div class="small muted">Applies to {{ pluralize(selection.length, isReq ? 'requirement' : 'test') }}.</div>
    </div>
    <template #footer>
      <Button label="Cancel" text severity="secondary" @click="catDialog = false" />
      <Button label="Apply" icon="pi pi-check" :disabled="!catIds.length" @click="applyCats" />
    </template>
  </Dialog>
  <FolderPickerDialog v-model:visible="moveDialog" :folders="folders" header="Move selection to folder" @pick="onPickFolder" />
</template>
