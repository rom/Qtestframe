<script setup lang="ts">
import HelpTip from '@/guide/HelpTip.vue'
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Tree from 'primevue/tree'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import TabPanels from 'primevue/tabpanels'
import TabPanel from 'primevue/tabpanel'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Checkbox from 'primevue/checkbox'
import { useReleaseOptions } from '@/composables/releases'
import { useConfigurations } from '@/composables/configurations'
import { runReleaseName } from '@/lib/releases'
import QueryInput from '@/components/QueryInput.vue'
import StatusTag from '@/components/StatusTag.vue'
import SummaryBar from '@/components/SummaryBar.vue'
import ItemPickerDialog from '@/components/ItemPickerDialog.vue'
import BulkItemLinksDialog from '@/components/BulkItemLinksDialog.vue'
import { api, ApiError, asItems, errorMessage } from '@/lib/api'
import { anyOf, and } from '@/lib/query'
import { buildTree, allKeys } from '@/lib/tree'
import { formatDateTime } from '@/lib/format'
import { summarize } from '@/lib/summary'
import { P } from '@/lib/permissions'
import ColumnsMenu from '@/components/ColumnsMenu.vue'
import { useColumnLayout, type ColumnDef } from '@/composables/columns'
import type { Readiness, TestCase, TestRun } from '@/lib/types'
import { readinessSummary } from '@/lib/readiness'
import { runsBefore, type RunBulkSet } from '@/lib/bulkRuns'
import { entry, recordUndo } from '@/lib/undoActions'
import { enumLabel } from '@/i18n/labels'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'
import { usePrefsStore } from '@/stores/prefs'
import SuggestInput from '@/components/SuggestInput.vue'
import { RUN_NAME, rememberValues } from '@/lib/suggest'
import { useI18n } from 'vue-i18n'
import { enumOptions } from '@/i18n/labels'
import HelpLink from '@/guide/HelpLink.vue'
import VariantSelect from '@/components/VariantSelect.vue'
import { useVariants } from '@/composables/variants'
const prefs = usePrefsStore()

const proj = useProjectStore()
const auth = useAuthStore()
const meta = useMetaStore()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()
const statusOptions = computed(() => enumOptions('status', meta.meta.run_statuses))

const runs = ref<TestRun[]>([])
const loading = ref(false)
const q = ref('')
const statuses = ref<string[]>([])
const queryError = ref<unknown>(null)
const composed = computed(() => and(q.value, anyOf('status', statuses.value)))

async function load() {
  loading.value = true
  queryError.value = null
  try {
    runs.value = asItems(await api.get<{ items: TestRun[] }>(`/projects/${proj.pid}/runs`, { q: composed.value }))
  } catch (e) {
    queryError.value = e
    if (!(e instanceof ApiError && e.code === 'invalid_query')) notify.fail(e)
  } finally {
    loading.value = false
  }
}
const rel = useReleaseOptions(() => proj.pid)
const cfg = useConfigurations(() => proj.pid)
// the variant a new run tests (none: variant-neutral)
const variants = useVariants(() => proj.pid)
const runVariant = ref<string | null>(null)
onMounted(() => {
  load()
  if (auth.can(P.projectRead)) {
    rel.load()
    cfg.load()
    variants.load()
  }
})

// ---------- create ----------
const dialog = ref(false)
const form = ref({ name: '', description: '', environment: '', build: '', query: '', release_id: '', configuration_ids: [] as string[] })
const pickedTests = ref<TestCase[]>([])
const folderKeys = ref<Record<string, { checked: boolean; partialChecked: boolean }>>({})
const folderExpanded = ref<Record<string, boolean>>({})
const picker = ref(false)
const saving = ref(false)
const createError = ref('')
const testFolders = computed(() => buildTree(proj.folders.test))
const selectedFolderIds = computed(() => Object.entries(folderKeys.value).filter(([, v]) => v.checked).map(([k]) => k))
const previewCount = ref<number | null>(null)
const previewError = ref<unknown>(null)

function openCreate() {
  const d = new Date()
  form.value = { name: t('runs.defaultName', { date: d.toISOString().slice(0, 10) }), description: '', environment: '', build: '', query: '', release_id: '', configuration_ids: [] }
  runVariant.value = null
  pickedTests.value = []
  folderKeys.value = {}
  folderExpanded.value = allKeys(testFolders.value)
  createError.value = ''
  previewCount.value = null
  dialog.value = true
}
async function preview() {
  previewError.value = null
  previewCount.value = null
  if (!form.value.query.trim()) return
  try {
    const res = await api.get<{ items: TestCase[]; total: number }>(`/projects/${proj.pid}/tests`, { q: form.value.query })
    previewCount.value = res?.total ?? asItems(res).length
  } catch (e) {
    previewError.value = e
  }
}
function addPicked(items: { id: string }[]) {
  const have = new Set(pickedTests.value.map((t) => t.id))
  pickedTests.value = [...pickedTests.value, ...(items as TestCase[]).filter((t) => !have.has(t.id))]
}
// readiness of the picked tests (warn before the run is created)
const pickedReadiness = ref<Record<string, Readiness>>({})
watch(
  () => pickedTests.value.map((x) => x.id).join(','),
  async (ids) => {
    if (!ids) {
      pickedReadiness.value = {}
      return
    }
    try {
      const res = await api.get<{ tests: Record<string, Readiness> }>(`/projects/${proj.pid}/readiness`, { test_ids: ids })
      pickedReadiness.value = res.tests ?? {}
    } catch {
      pickedReadiness.value = {}
    }
  },
)
const pickedNotReady = computed(() => pickedTests.value.filter((x) => pickedReadiness.value[x.id] && !pickedReadiness.value[x.id].ready))
const nothingSelected = computed(() => !pickedTests.value.length && !selectedFolderIds.value.length && !form.value.query.trim())
async function create() {
  saving.value = true
  createError.value = ''
  try {
    const { release_id, ...fields } = form.value
    const run = await api.post<TestRun>(`/projects/${proj.pid}/runs`, {
      ...fields,
      ...(release_id ? { release_id } : {}),
      ...(runVariant.value ? { variant_id: runVariant.value } : {}),
      test_ids: pickedTests.value.map((t) => t.id),
      folder_ids: selectedFolderIds.value,
    })
    dialog.value = false
    void rememberValues(prefs.prefs, proj.pid, fields.name?.trim() ? [{ field: RUN_NAME, value: fields.name }] : [])
    notify.ok(t('runs.created'), run?.name)
    if (run?.id) router.push({ name: 'run', params: { projectId: proj.pid, runId: run.id } })
    else load()
  } catch (e) {
    createError.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}

// ---------- bulk ----------
const selection = ref<TestRun[]>([])
const canBulk = computed(() => auth.can(P.runWrite))
const selectedIds = computed(() => selection.value.map((r) => r.id))
const bulkLinksOpen = ref(false)
const bulkStatus = ref<string | null>(null)
const bulkRelease = ref<string | null>(null)
const envDialog = ref(false)
const bulkEnv = ref('')
const bulkBuild = ref('')
const bulkEnvOn = ref(false)
const bulkBuildOn = ref(false)
const bulkPath = computed(() => `/projects/${proj.pid}/runs/bulk`)
watch(runs, (list) => {
  const ids = new Set(list.map((r) => r.id))
  selection.value = selection.value.filter((s) => ids.has(s.id))
})
async function bulkSet(set: RunBulkSet) {
  const ids = [...selectedIds.value]
  if (!ids.length) return
  const before = runsBefore(runs.value, ids, set)
  try {
    await api.post(bulkPath.value, { ids, set })
    recordUndo(
      entry(
        t('runs.bulk.undo', { n: ids.length }),
        async () => {
          for (const b of before) await api.patch(`/runs/${b.id}`, b.before)
        },
        () => api.post(bulkPath.value, { ids, set }),
      ),
    )
    notify.ok(t('runs.bulk.updated', { n: ids.length }, ids.length))
    await load()
  } catch (e) {
    notify.fail(e, t('runs.bulk.failed'))
  }
}
watch(bulkStatus, (v) => v && bulkSet({ status: v }).then(() => (bulkStatus.value = null)))
watch(bulkRelease, (v) => v !== null && bulkSet({ release_id: v }).then(() => (bulkRelease.value = null)))
function openEnv() {
  bulkEnv.value = ''
  bulkBuild.value = ''
  bulkEnvOn.value = true
  bulkBuildOn.value = false
  envDialog.value = true
}
async function applyEnv() {
  const set: RunBulkSet = {}
  if (bulkEnvOn.value) set.environment = bulkEnv.value.trim()
  if (bulkBuildOn.value) set.build = bulkBuild.value.trim()
  envDialog.value = false
  await bulkSet(set)
}
function bulkDelete() {
  const ids = [...selectedIds.value]
  notify.confirmDanger(t('runs.bulk.confirmDelete', { n: ids.length }, ids.length), async () => {
    try {
      await api.del(bulkPath.value, { ids })
      notify.ok(t('runs.bulk.deleted', { n: ids.length }, ids.length))
      selection.value = []
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}

function remove(r: TestRun) {
  notify.confirmDanger(t('runs.confirmDelete', { name: r.name }), async () => {
    try {
      await api.del(`/runs/${r.id}`)
      notify.ok(t('runs.deleted'))
      load()
    } catch (e) {
      notify.fail(e)
    }
  })
}
const progress = (r: TestRun) => summarize(r.summary)

// ---------- columns ----------
const runColumns = computed<ColumnDef[]>(() => [
  { field: 'status', header: t('common.status') },
  { field: 'environment', header: t('common.environment') },
  { field: 'build', header: t('common.build') },
  { field: 'release', header: t('releasesView.release'), sortField: 'release_id' },
  { field: 'configurations', header: t('configurations.column'), sortable: false },
  { field: 'progress', header: t('runs.progress'), sortable: false },
  { field: 'executions', header: t('runs.executions'), sortable: false },
  { field: 'created_by', header: t('workspace.colCreatedBy') },
  { field: 'created_at', header: t('common.created') },
])
const runLayout = useColumnLayout(
  () => 'qtf.cols.runs',
  runColumns,
  () => ['status', 'environment', 'build', 'release', 'configurations', 'progress', 'executions', 'created_at'],
)
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('runs.title') }}</h1>
      <div class="sub">{{ t('runs.subtitle') }}</div>
    </div>
    <div class="actions">
      <HelpLink page="runs" />
      <Button v-if="auth.can(P.runWrite)" :label="t('runs.new')" icon="pi pi-plus" @click="openCreate" />
    </div>
  </div>
  <div class="card">
    <div class="filterbar">
      <QueryInput v-model="q" scope="run" :error="queryError" :composed="composed" :placeholder="t('runs.filterPlaceholder', { example: 'environment:staging build:1.4*' })" @submit="load" />
      <MultiSelect v-model="statuses" :options="statusOptions" option-label="label" option-value="value" :placeholder="t('common.status')" :aria-label="t('common.status')" style="min-width: 150px" show-clear @change="load" />
      <ColumnsMenu
        :columns="runColumns"
        :order="runLayout.order.value"
        @toggle="(id, on) => runLayout.toggle(id, on)"
        @move="(id, d) => runLayout.move(id, d)"
        @reset="runLayout.reset()"
      />
    </div>
    <div v-if="selection.length && canBulk" class="bulkbar" role="toolbar" :aria-label="t('runs.bulk.toolbar')">
      <b>{{ t('common.selected', { count: selection.length }) }}</b>
      <Select v-model="bulkStatus" :options="statusOptions" option-label="label" option-value="value" :placeholder="t('runs.bulk.setStatus')" :aria-label="t('runs.bulk.setStatus')" size="small" style="min-width: 150px" />
      <Select v-if="rel.releases.value.length" v-model="bulkRelease" :options="rel.options.value" option-label="label" option-value="value" :placeholder="t('runs.bulk.setRelease')" :aria-label="t('runs.bulk.setRelease')" size="small" style="min-width: 150px" />
      <Button :label="t('runs.bulk.envBuild')" icon="pi pi-server" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="openEnv" />
      <Button :label="t('links.title')" icon="pi pi-link" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="bulkLinksOpen = true" />
      <Button :label="t('common.delete')" icon="pi pi-trash" size="small" severity="danger" outlined @click="bulkDelete" />
      <Button icon="pi pi-times" size="small" text rounded severity="secondary" :aria-label="t('workspace.clearSelection')" v-tooltip.bottom="t('workspace.clearSelection')" @click="selection = []" />
    </div>
    <DataTable
      :ref="runLayout.bind"
      v-model:selection="selection"
      v-stack
      :value="runs"
      data-key="id"
      :loading="loading"
      :aria-busy="loading"
      paginator
      :rows="prefs.rowsFor(25)"
      row-hover
      sort-field="created_at"
      :sort-order="-1"
      class="clickable"
      reorderable-columns
      @column-reorder="runLayout.onReorder()"
      @row-click="(e) => router.push({ name: 'run', params: { projectId: proj.pid, runId: (e.data as TestRun).id } })"
    >
      <Column v-if="canBulk" selection-mode="multiple" column-key="_select" :reorderable-column="false" style="width: 2.6rem" />
      <Column field="name" :header="t('common.name')" sortable :reorderable-column="false" body-class="stack-main">
        <template #body="{ data }">
          <div style="min-width: 0">
            <RouterLink :to="{ name: 'run', params: { projectId: proj.pid, runId: data.id } }" class="project-link" @click.stop>{{ data.name }}</RouterLink>
            <div class="small muted ellipsis" style="max-width: 360px">{{ data.description }}</div>
          </div>
        </template>
      </Column>
      <template v-for="c in runLayout.visible.value" :key="c.field">
      <Column v-if="c.field === 'status'" field="status" column-key="status" :header="c.header" sortable style="width: 130px"><template #body="{ data }"><StatusTag :value="data.status" /></template></Column>
      <Column v-else-if="c.field === 'environment'" field="environment" column-key="environment" :header="c.header" sortable />
      <Column v-else-if="c.field === 'build'" field="build" column-key="build" :header="c.header" sortable><template #body="{ data }"><span class="mono">{{ data.build }}</span></template></Column>
      <Column v-else-if="c.field === 'release'" field="release_id" column-key="release" :header="c.header" sortable>
        <template #body="{ data }">
          <RouterLink v-if="runReleaseName(data, rel.names.value)" :to="{ name: 'releases', params: { projectId: proj.pid }, query: { r: data.release_id } }" @click.stop><Tag :value="runReleaseName(data, rel.names.value)" icon="pi pi-box" severity="secondary" /></RouterLink>
        </template>
      </Column>
      <Column v-else-if="c.field === 'configurations'" column-key="configurations" :header="c.header" style="min-width: 140px">
        <template #body="{ data }">
          <span v-if="data.configurations?.length" class="row" style="gap: 4px">
            <Tag v-for="c2 in data.configurations.slice(0, 3)" :key="c2.id" :value="c2.name" icon="pi pi-sliders-h" severity="secondary" />
            <span v-if="data.configurations.length > 3" class="small muted">{{ t('configurations.more', { n: data.configurations.length - 3 }) }}</span>
          </span>
        </template>
      </Column>
      <Column v-else-if="c.field === 'progress'" column-key="progress" :header="c.header" style="min-width: 220px">
        <template #body="{ data }"><SummaryBar :summary="data.summary" show-text /></template>
      </Column>
      <Column v-else-if="c.field === 'executions'" column-key="executions" :header="c.header" style="width: 120px"><template #body="{ data }">{{ progress(data).total }}</template></Column>
      <Column v-else-if="c.field === 'created_by'" field="created_by" column-key="created_by" :header="c.header" sortable />
      <Column v-else-if="c.field === 'created_at'" field="created_at" column-key="created_at" :header="c.header" sortable style="width: 160px">
        <template #body="{ data }"><span class="small muted">{{ formatDateTime(data.created_at) }}</span></template>
      </Column>
      </template>
      <Column v-if="auth.can(P.runWrite)" column-key="_actions" :reorderable-column="false" style="width: 60px">
        <template #body="{ data }"><Button icon="pi pi-trash" text rounded severity="danger" :aria-label="t('runs.deleteNamed', { name: data.name })" v-tooltip="t('common.delete')" @click.stop="remove(data)" /></template>
      </Column>
      <template #empty><div class="empty"><i class="pi pi-play-circle" />{{ t('runs.none') }}</div></template>
    </DataTable>
  </div>

  <Dialog v-model:visible="dialog" :header="t('runs.newHeader')" modal maximizable :maximize-button-props="{ 'aria-label': t('common.toggleMaximize') }" style="width: min(760px, 96vw)">
    <div class="grid" style="gap: 14px">
      <Message v-if="createError" severity="error" :closable="false">{{ createError }}</Message>
      <div class="form-grid">
        <div v-field class="field full"><label>{{ t('common.name') }}</label><SuggestInput v-model="form.name" field="run.name" :project-id="proj.pid" autofocus /></div>
        <div v-field class="field"><label>{{ t('common.environment') }}</label><HelpTip id="run.environment" :label="t('common.environment')" /><InputText v-model="form.environment" :placeholder="t('runs.envPlaceholder')" /></div>
        <div v-field class="field"><label>{{ t('common.build') }}</label><HelpTip id="run.build" :label="t('common.build')" /><InputText v-model="form.build" :placeholder="t('runs.buildPlaceholder')" /></div>
        <div v-if="rel.releases.value.length" v-field class="field full">
          <label for="run-release">{{ t('releasesView.release') }}</label>
          <Select v-model="form.release_id" input-id="run-release" :options="rel.options.value" option-label="label" option-value="value" />
        </div>
        <div v-if="cfg.configurations.value.length" class="field full">
          <label for="run-configs">{{ t('configurations.runField') }}</label>
          <HelpTip id="run.configurations" :label="t('configurations.runField')" />
          <MultiSelect v-model="form.configuration_ids" input-id="run-configs" :options="cfg.options.value" option-label="label" option-value="value" display="chip" filter :placeholder="t('configurations.runNone')" aria-describedby="run-configs-help" />
          <small id="run-configs-help" class="muted">{{ t('configurations.runHelp') }}</small>
        </div>
        <div v-if="variants.variants.value.length" class="field full">
          <label id="run-variant-label" for="run-variant">{{ t('variants.runField') }}</label>
          <VariantSelect v-model="runVariant" input-id="run-variant" labelledby="run-variant-label" :variants="variants.variants.value" :placeholder="t('variants.runNone')" aria-describedby="run-variant-help" />
          <small id="run-variant-help" class="muted">{{ t('variants.runHelp') }}</small>
        </div>
        <div v-field class="field full"><label>{{ t('common.description') }}</label><Textarea v-model="form.description" rows="2" auto-resize /></div>
      </div>
      <div class="section-title" style="margin: 0">{{ t('runs.include') }} <HelpTip id="run.include" :label="t('runs.include')" /></div>
      <Tabs value="tests" scrollable>
        <TabList>
          <Tab value="tests">{{ t('runs.tabTests', { count: pickedTests.length }) }}</Tab>
          <Tab value="folders">{{ t('runs.tabFolders', { count: selectedFolderIds.length }) }}</Tab>
          <Tab value="query">{{ form.query.trim() ? t('runs.tabQueryActive') : t('runs.tabQuery') }}</Tab>
        </TabList>
        <TabPanels>
          <TabPanel value="tests">
            <div class="row" style="margin-bottom: 8px">
              <Button :label="t('runs.pickTests')" icon="pi pi-search" severity="secondary" outlined @click="picker = true" />
              <Button v-if="pickedTests.length" :label="t('common.clear')" text severity="secondary" @click="pickedTests = []" />
            </div>
            <div style="max-height: 220px; overflow: auto">
              <div v-for="tc in pickedTests" :key="tc.id" class="row nowrap" style="padding: 4px 0; border-bottom: 1px solid var(--qtf-border)">
                <span class="key">{{ tc.key }}</span><span class="grow ellipsis">{{ tc.title }}</span>
                <span
                  v-if="pickedReadiness[tc.id] && !pickedReadiness[tc.id].ready"
                  class="readiness-cell small"
                  :class="{ blocked: !pickedReadiness[tc.id].allowed }"
                  :title="readinessSummary(pickedReadiness[tc.id], t, enumLabel)"
                >
                  <i :class="pickedReadiness[tc.id].allowed ? 'pi pi-exclamation-triangle' : 'pi pi-ban'" aria-hidden="true" />
                  {{ pickedReadiness[tc.id].allowed ? t('readiness.notReady', { count: pickedReadiness[tc.id].issues.length }, pickedReadiness[tc.id].issues.length) : t('readiness.cannotRun') }}
                </span>
                <Button icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('common.removeItem', { key: tc.key })" @click="pickedTests = pickedTests.filter((x) => x.id !== tc.id)" />
              </div>
              <div v-if="!pickedTests.length" class="muted small">{{ t('runs.noneSelectedTests') }}</div>
            </div>
            <Message v-if="pickedNotReady.length" severity="warn" :closable="false" style="margin-top: 8px">
              {{ t('readiness.pickedNotReady', { count: pickedNotReady.length }, pickedNotReady.length) }} {{ t('readiness.pickedHint') }}
            </Message>
          </TabPanel>
          <TabPanel value="folders">
            <div v-if="!testFolders.length" class="muted small">{{ t('runs.noFolders') }}</div>
            <Tree v-else v-model:selection-keys="folderKeys" v-model:expanded-keys="folderExpanded" :value="testFolders" selection-mode="checkbox" :aria-label="t('runs.tabFolders', { count: selectedFolderIds.length })" :pt="{ rootChildren: { 'aria-multiselectable': 'true' }, pcNodeCheckbox: { input: { 'aria-hidden': 'true', tabindex: '-1' } } }" style="max-height: 260px; overflow: auto; padding: 0; border: none" />
            <div class="small muted">{{ t('runs.foldersHint') }}</div>
          </TabPanel>
          <TabPanel value="query">
            <div class="row">
              <QueryInput v-model="form.query" scope="test" :error="previewError" :placeholder="t('runs.queryPlaceholder', { example: 'category:Negative priority=high' })" @submit="preview" />
              <Button :label="t('runs.preview')" severity="secondary" outlined @click="preview" />
            </div>
            <div v-if="previewCount !== null" class="small" style="margin-top: 6px">{{ t('runs.previewCount', previewCount) }}</div>
          </TabPanel>
        </TabPanels>
      </Tabs>
    </div>
    <template #footer>
      <span v-if="nothingSelected" class="small muted" style="margin-right: auto">{{ t('runs.startsEmpty') }}</span>
      <Button :label="t('common.cancel')" text severity="secondary" @click="dialog = false" />
      <Button :label="t('runs.create')" icon="pi pi-check" :loading="saving" :disabled="!form.name.trim()" @click="create" />
    </template>
  </Dialog>
  <ItemPickerDialog v-model:visible="picker" kind="test" :project-id="proj.pid" :exclude="pickedTests.map((t) => t.id)" @pick="addPicked" />
  <Dialog v-model:visible="envDialog" :header="t('runs.bulk.envHeader', { n: selection.length }, selection.length)" modal style="width: min(460px, 96vw)">
    <div class="env-form">
      <div class="row">
        <Checkbox v-model="bulkEnvOn" binary input-id="bk-env-on" />
        <label for="bk-env-on">{{ t('runs.bulk.setEnvironment') }}</label>
      </div>
      <InputText v-model="bulkEnv" :disabled="!bulkEnvOn" fluid :aria-label="t('runs.bulk.setEnvironment')" :placeholder="t('runs.bulk.emptyClears')" />
      <div class="row">
        <Checkbox v-model="bulkBuildOn" binary input-id="bk-build-on" />
        <label for="bk-build-on">{{ t('runs.bulk.setBuild') }}</label>
      </div>
      <InputText v-model="bulkBuild" :disabled="!bulkBuildOn" fluid :aria-label="t('runs.bulk.setBuild')" :placeholder="t('runs.bulk.emptyClears')" />
    </div>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="envDialog = false" />
      <Button :label="t('common.apply')" icon="pi pi-check" :disabled="!bulkEnvOn && !bulkBuildOn" @click="applyEnv" />
    </template>
  </Dialog>
  <BulkItemLinksDialog v-if="canBulk" v-model:visible="bulkLinksOpen" source-type="run" :project-id="proj.pid" :selected="selectedIds" />
</template>

<style scoped>
.env-form {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.env-form .row {
  gap: 8px;
}
</style>
