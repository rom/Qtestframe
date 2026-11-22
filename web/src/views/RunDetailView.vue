<script setup lang="ts">
import ItemLinksPanel from '@/components/ItemLinksPanel.vue'
import CustomFieldsEditor from '@/components/CustomFieldsEditor.vue'
import type { CustomValues } from '@/lib/customFields'
import MentionText from '@/components/MentionText.vue'
import ColumnsMenu from '@/components/ColumnsMenu.vue'
import { useColumnLayout, type ColumnDef } from '@/composables/columns'
import MentionInput from '@/components/MentionInput.vue'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import DiscussionPanel from '@/components/DiscussionPanel.vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Select from 'primevue/select'
import MultiSelect from 'primevue/multiselect'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import Panel from 'primevue/panel'
import Tag from 'primevue/tag'
import { useReleaseOptions } from '@/composables/releases'
import { useConfigurations } from '@/composables/configurations'
import { summaryByConfiguration } from '@/lib/configurations'
import { runReleaseName } from '@/lib/releases'
import Popover from 'primevue/popover'
import ReadinessBanner from '@/components/ReadinessBanner.vue'
import { readinessOfError, readinessSummary as explainReadiness } from '@/lib/readiness'
import ProgressSpinner from 'primevue/progressspinner'
import QueryInput from '@/components/QueryInput.vue'
import StatusTag from '@/components/StatusTag.vue'
import SummaryBar from '@/components/SummaryBar.vue'
import ItemPickerDialog from '@/components/ItemPickerDialog.vue'
import DefectLinks from '@/components/DefectLinks.vue'
import AttachmentsPanel from '@/components/AttachmentsPanel.vue'
import RunJobsPanel from '@/components/RunJobsPanel.vue'
import { skippedText, type EnqueueResult } from '@/lib/webTests'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import { api, ApiError, asItems, errorMessage } from '@/lib/api'
import { anyOf, and } from '@/lib/query'
import { formatDateTime, formatDuration, formatNumber } from '@/lib/format'
import { summarize } from '@/lib/summary'
import { RESULT_COLORS } from '@/lib/colors'
import { P } from '@/lib/permissions'
import type { Execution, TestRun } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'
import { usePrefsStore } from '@/stores/prefs'
import SuggestInput from '@/components/SuggestInput.vue'
import { RUN_NAME, rememberValues, savedEntries } from '@/lib/suggest'
import { useI18n } from 'vue-i18n'
import { enumLabel, enumOptions } from '@/i18n/labels'
import Checkbox from 'primevue/checkbox'
import { BULK_RESULTS, execCounts, execUndo, rejectedText, type ExecBulkResult } from '@/lib/bulkRuns'
import { entry, recordUndo } from '@/lib/undoActions'
const prefs = usePrefsStore()

const props = defineProps<{ runId: string }>()
const proj = useProjectStore()
const auth = useAuthStore()
const meta = useMetaStore()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()
const runStatusOptions = computed(() => enumOptions('status', meta.meta.run_statuses))
const resultOptions = computed(() => enumOptions('result', meta.meta.result_statuses))

const run = ref<TestRun | null>(null)
const executions = ref<Execution[]>([])
const loading = ref(false)
const error = ref('')
const q = ref('')
const statusFilter = ref<string[]>([])
const queryError = ref<unknown>(null)
const composed = computed(() => and(q.value, anyOf('status', statusFilter.value)))
const canWrite = computed(() => auth.can(P.runWrite))
const linkCount = ref<number | null>(null)
const canExecute = computed(() => auth.can(P.runExecute))
const route = useRoute()
const tab = ref(route.query.tab === 'discussion' ? 'discussion' : 'executions')
// unresolved threads of the run's discussion (badge of its tab)
const openDiscussions = ref(0)
const attachmentCount = ref<number | null>(null)
const tabsId = 'run-tabs'
const panel = (v: string) => ({ role: 'tabpanel', id: `${tabsId}_tabpanel_${v}`, 'aria-labelledby': `${tabsId}_tab_${v}` })

async function loadRun() {
  try {
    run.value = await api.get<TestRun>(`/runs/${props.runId}`)
  } catch (e) {
    error.value = errorMessage(e)
  }
}
async function loadExecutions() {
  loading.value = true
  queryError.value = null
  try {
    executions.value = asItems(await api.get<{ items: Execution[] }>(`/runs/${props.runId}/executions`, { q: composed.value }))
  } catch (e) {
    queryError.value = e
    if (!(e instanceof ApiError && e.code === 'invalid_query')) notify.fail(e)
  } finally {
    loading.value = false
  }
}
async function reload() {
  await Promise.all([loadRun(), loadExecutions()])
}
const rel = useReleaseOptions(() => proj.pid)
const cfg = useConfigurations(() => proj.pid)
onMounted(() => {
  reload()
  if (auth.can(P.projectRead)) {
    rel.load()
    cfg.load()
  }
})

// ---------- configurations ----------
const runConfigs = computed(() => run.value?.configurations ?? [])
const hasConfigs = computed(() => runConfigs.value.length > 0 || executions.value.some((e) => e.configuration_id))
/** '' all, 'none' executions without configuration, else a configuration id */
const configFilter = ref('')
const perConfig = computed(() => summaryByConfiguration(executions.value, runConfigs.value))
const configFilterOptions = computed(() => [
  { label: t('configurations.all'), value: '' },
  ...perConfig.value.map((c) => ({ label: c.id ? c.name : t('configurations.withoutShort'), value: c.id || 'none' })),
])
const shownExecutions = computed(() =>
  configFilter.value ? executions.value.filter((e) => (e.configuration_id ?? 'none') === configFilter.value) : executions.value,
)
/** the execution's test key, with its configuration when it has one */
function exLabel(ex: Execution): string {
  return ex.configuration_name ? `${ex.test_key} (${ex.configuration_name})` : ex.test_key
}

const summary = computed(() => summarize(run.value?.summary))

async function setStatus(status: string) {
  if (!run.value) return
  try {
    run.value = await api.patch<TestRun>(`/runs/${run.value.id}`, { status })
    notify.ok(t('run.statusUpdated'), enumLabel('status', status))
  } catch (e) {
    notify.fail(e)
    loadRun()
  }
}

// ---------- edit ----------
const editDialog = ref(false)
const editForm = ref({ name: '', description: '', environment: '', build: '', release_id: '', configuration_ids: [] as string[], custom_fields: {} as CustomValues })
const editError = ref('')
function openEdit() {
  if (!run.value) return
  const r = run.value
  editForm.value = { name: r.name, description: r.description, environment: r.environment, build: r.build, release_id: r.release_id ?? '', configuration_ids: (r.configurations ?? []).map((c) => c.id), custom_fields: { ...(r.custom_fields ?? {}) } as CustomValues }
  editError.value = ''
  editDialog.value = true
}
async function saveEdit() {
  editError.value = ''
  const { configuration_ids, ...fields } = editForm.value
  const before = (run.value?.configurations ?? []).map((c) => c.id)
  const changed = before.join(',') !== configuration_ids.join(',')
  const oldName = run.value?.name
  try {
    run.value = await api.patch<TestRun>(`/runs/${props.runId}`, changed ? editForm.value : fields)
    void rememberValues(prefs.prefs, proj.pid, savedEntries({ [RUN_NAME]: fields.name }, { [RUN_NAME]: oldName }))
    editDialog.value = false
    notify.ok(t('run.updated'))
    if (changed) {
      await reload()
      if (configFilter.value && !configFilterOptions.value.some((o) => o.value === configFilter.value)) configFilter.value = ''
    }
  } catch (e) {
    editError.value = errorMessage(e)
  }
}
function remove() {
  notify.confirmDanger(t('run.confirmDelete', { name: run.value?.name ?? '' }), async () => {
    try {
      await api.del(`/runs/${props.runId}`)
      router.push({ name: 'runs', params: { projectId: proj.pid } })
    } catch (e) {
      notify.fail(e)
    }
  })
}

// ---------- executions ----------
// ---------- bulk ----------
const selection = ref<Execution[]>([])
const selectedIds = computed(() => selection.value.map((e) => e.id))
const bulkResultOptions = computed(() => enumOptions('result', [...BULK_RESULTS]))
const bulkResult = ref<string | null>(null)
const bulkDialog = ref(false)
const bulkForm = reactive({ status: '' as string, assigneeOn: false, assignee: '', note: '', onlyNotRun: false })
watch(executions, (list) => {
  const ids = new Set(list.map((e) => e.id))
  selection.value = selection.value.filter((s) => ids.has(s.id))
})
watch(bulkResult, (v) => v && bulkUpdate({ status: v }, false).then(() => (bulkResult.value = null)))
function openBulk() {
  Object.assign(bulkForm, { status: '', assigneeOn: false, assignee: '', note: '', onlyNotRun: false })
  bulkDialog.value = true
}
async function applyBulk() {
  const set: { status?: string; assignee?: string; notes_append?: string } = {}
  if (bulkForm.status) set.status = bulkForm.status
  if (bulkForm.assigneeOn) set.assignee = bulkForm.assignee.trim()
  if (bulkForm.note.trim()) set.notes_append = bulkForm.note.trim()
  bulkDialog.value = false
  await bulkUpdate(set, bulkForm.onlyNotRun)
}
async function bulkUpdate(set: { status?: string; assignee?: string; notes_append?: string }, onlyNotRun: boolean) {
  const ids = [...selectedIds.value]
  if (!ids.length || !Object.keys(set).length) return
  const notesBefore = new Map(executions.value.map((e) => [e.id, e.notes ?? '']))
  const path = `/runs/${props.runId}/executions/bulk`
  try {
    const res = await api.post<ExecBulkResult>(path, { ids, set, ...(onlyNotRun ? { from: ['not_run'] } : {}) })
    const c = execCounts(res)
    const undo = execUndo(res.updated ?? [], set, notesBefore)
    if (undo.length) {
      recordUndo(
        entry(
          t('run.bulk.undo', { n: undo.length }),
          async () => {
            for (const u of undo) await api.patch(`/executions/${u.id}`, u.body)
          },
          () => api.post(path, { ids: undo.map((u) => u.id), set }),
        ),
      )
    }
    const extra = c.skipped ? t('run.bulk.skipped', { n: c.skipped }, c.skipped) : undefined
    if (c.rejected) notify.failText(t('run.bulk.rejected', { n: c.rejected, updated: c.updated }, c.rejected), rejectedText(res.rejected ?? []))
    else notify.ok(t('run.bulk.updated', { n: c.updated }, c.updated), extra)
    await reload()
  } catch (e) {
    notify.fail(e)
  }
}
function bulkRemove() {
  const ids = [...selectedIds.value]
  notify.confirmDanger(
    t('run.bulk.confirmRemove', { n: ids.length }, ids.length),
    async () => {
      try {
        const res = await api.post<ExecBulkResult>(`/runs/${props.runId}/executions/bulk`, { ids, remove: true })
        notify.ok(t('run.bulk.removed', { n: res.removed ?? 0 }, res.removed ?? 0))
        selection.value = []
        await reload()
      } catch (e) {
        notify.fail(e)
      }
    },
    undefined,
    t('run.bulk.remove'),
  )
}
/** assignee being edited per execution id (committed on blur or Enter) */
const assigneeDraft = reactive<Record<string, string>>({})

// ---------- readiness (see ReadinessBanner) ----------
const notAllowed = (ex: Execution) => !!ex.readiness && !ex.readiness.allowed
const readinessSummary = computed(() => ({
  blocked: executions.value.filter(notAllowed).length,
  warned: executions.value.filter((e) => e.readiness && e.readiness.allowed && !e.readiness.ready).length,
}))
function readinessText(ex: Execution): string {
  const n = ex.readiness?.issues.length ?? 0
  return n ? t('readiness.notReady', { count: n }, n) : t('readiness.ok')
}
function readinessTitle(ex: Execution): string {
  return explainReadiness(ex.readiness, t, enumLabel)
}

// ---------- columns ----------
const execColumns = computed<ColumnDef[]>(() => [
  { field: 'configuration', header: t('configurations.columnOne'), width: '150px' },
  { field: 'status', header: t('common.status'), width: '130px' },
  { field: 'readiness', header: t('readiness.column'), width: '150px', sortable: false },
  { field: 'assignee', header: t('run.assignee'), width: '180px' },
  { field: 'executed_by', header: t('run.executedBy'), width: '150px' },
  { field: 'defects', header: t('run.defects'), sortable: false },
  { field: 'duration_ms', header: t('common.duration'), width: '110px' },
  { field: 'finished_at', header: t('run.finished'), width: '150px' },
])
const execLayout = useColumnLayout(
  () => 'qtf.cols.executions',
  execColumns,
  () => ['configuration', 'status', 'readiness', 'assignee', 'executed_by', 'defects', 'duration_ms', 'finished_at'],
)
async function setAssignee(ex: Execution, value: string) {
  if (value === ex.assignee) return
  try {
    const updated = await api.patch<Execution>(`/executions/${ex.id}`, { assignee: value })
    Object.assign(ex, updated ?? { assignee: value })
  } catch (e) {
    notify.fail(e)
  }
}
async function quickStatus(ex: Execution, status: string) {
  try {
    const updated = await api.patch<Execution>(`/executions/${ex.id}`, { status })
    Object.assign(ex, updated ?? { status })
    loadRun()
  } catch (e) {
    const rd = readinessOfError(e)
    if (rd) {
      ex.readiness = rd
      notify.failText(t('readiness.saveRefused') + ' – ' + exLabel(ex), explainReadiness(rd, t, enumLabel))
    } else notify.fail(e)
  }
}
// the full explanation of an execution's readiness, in a popover
const readinessPop = ref<InstanceType<typeof Popover> | null>(null)
const readinessOf = ref<Execution | null>(null)
function showReadiness(ev: Event, ex: Execution) {
  readinessOf.value = ex
  readinessPop.value?.toggle(ev)
}
// ---------- automated tests as background jobs (see RunJobsPanel) ----------
const canScript = computed(() => auth.can(P.scriptExecute) && meta.meta.runner_enabled === true)
const jobsPanel = ref<InstanceType<typeof RunJobsPanel> | null>(null)
const queueing = ref(false)
async function runAutomated(ids?: string[]) {
  queueing.value = true
  try {
    const res = await api.post<EnqueueResult>(`/runs/${props.runId}/run-automated`, ids?.length ? { execution_ids: ids } : {})
    const n = res?.queued?.length ?? 0
    const skipped = res?.skipped ?? []
    const detail = skipped.length ? `${t('webTests.skippedN', { n: skipped.length }, skipped.length)}\n${skippedText(skipped)}` : undefined
    if (n) notify.ok(t('webTests.queuedN', { n }, n), detail)
    else notify.info(t('webTests.nothingQueued'), detail)
    jobsPanel.value?.refresh()
    await reload()
  } catch (e) {
    notify.fail(e)
  } finally {
    queueing.value = false
  }
}
const picker = ref(false)
async function addTests(items: { id: string }[]) {
  try {
    await api.post(`/runs/${props.runId}/executions`, { test_ids: items.map((i) => i.id) })
    notify.ok(t('run.addedTests', items.length))
    reload()
  } catch (e) {
    notify.fail(e)
  }
}
function execute(ex: Execution) {
  router.push({ name: 'execute', params: { projectId: proj.pid, runId: props.runId, executionId: ex.id } })
}
function startNext() {
  const next = executions.value.find((e) => e.status === 'not_run' || e.status === 'in_progress') ?? executions.value[0]
  if (next) execute(next)
}

// ---------- CI hint ----------
const ciDialog = ref(false)
const origin = typeof window !== 'undefined' ? window.location.origin : ''
const ciSnippet = computed(
  () => `curl -X POST ${origin}/api/v1/runs/${props.runId}/results \\
  -H "Authorization: Bearer qtf_<api-token>" \\
  -H "Content-Type: application/json" \\
  -d '{"results":[{"test_key":"${executions.value[0]?.test_key ?? 'PROJ-TC-1'}","status":"passed","duration_ms":1234,"output":"ok"}]}'`,
)
const ciJunitSnippet = computed(
  () => `curl -X POST ${origin}/api/v1/runs/${props.runId}/junit \\
  -H "Authorization: Bearer qtf_<api-token>" \\
  -H "Content-Type: application/xml" \\
  --data-binary @results/junit.xml`,
)
const ciCliSnippet = computed(
  () => `QTF_URL=${origin} QTF_TOKEN=qtf_<api-token> \\
  qtestframe report --run-id ${props.runId} --junit 'results/*.xml'${runConfigs.value.length ? ` --configuration '${runConfigs.value[0].name}'` : ''}`,
)
async function copySnippet(text: string = ciSnippet.value) {
  try {
    await navigator.clipboard.writeText(text)
    notify.ok(t('run.copied'))
  } catch {
    notify.info(t('errors.copyFailed'), t('errors.copyManually'))
  }
}
</script>

<template>
  <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
  <div v-else-if="!run" class="empty"><ProgressSpinner style="width: 36px; height: 36px" :aria-label="t('common.loading')" /></div>
  <template v-else>
    <div class="page-header">
      <div>
        <div class="row small" style="margin-bottom: 2px">
          <RouterLink :to="{ name: 'runs', params: { projectId: proj.pid } }" class="muted">{{ t('nav.operations') }}</RouterLink>
          <i class="pi pi-angle-right muted" />
        </div>
        <h1>{{ run.name }}</h1>
        <div class="sub row">
          <span v-if="run.environment"><i class="pi pi-server" aria-hidden="true" /><span class="sr-only">{{ t('common.environment') }}:</span> {{ run.environment }}</span>
          <RouterLink v-if="runReleaseName(run, rel.names.value)" :to="{ name: 'releases', params: { projectId: proj.pid }, query: { r: run.release_id } }">
            <Tag :value="runReleaseName(run, rel.names.value)" icon="pi pi-box" severity="secondary" :aria-label="t('releasesView.runRelease', { name: runReleaseName(run, rel.names.value) })" />
          </RouterLink>
          <span v-if="run.build" class="mono"><i class="pi pi-box" aria-hidden="true" /><span class="sr-only">{{ t('common.build') }}:</span> {{ run.build }}</span>
          <span v-if="run.variant_name"><i class="pi pi-clone" aria-hidden="true" /><span class="sr-only">{{ t('variants.variant') }}:</span> {{ run.variant_name }}</span>
          <span>{{ t('run.createdAt', { date: formatDateTime(run.created_at) }) }}{{ run.created_by ? t('run.createdBy', { user: run.created_by }) : '' }}</span>
          <span v-if="run.started_at">{{ t('run.startedAt', { date: formatDateTime(run.started_at) }) }}</span>
          <span v-if="run.completed_at">{{ t('run.completedAt', { date: formatDateTime(run.completed_at) }) }}</span>
        </div>
      </div>
      <div class="actions">
        <Select
          v-if="canWrite"
          :model-value="run.status"
          :options="runStatusOptions"
          option-label="label"
          option-value="value"
          style="min-width: 160px"
          :aria-label="t('run.status')"
          @update:model-value="setStatus"
        >
          <template #value="{ value }"><StatusTag :value="value" /></template>
        </Select>
        <StatusTag v-else :value="run.status" />
        <Button v-if="canExecute && executions.length" :label="t('run.execute')" icon="pi pi-play" @click="startNext" />
        <Button v-if="canScript && executions.length" :label="t('webTests.runAll')" icon="pi pi-bolt" severity="secondary" outlined :loading="queueing" v-tooltip.bottom="t('webTests.runAllHint')" @click="runAutomated()" />
        <Button v-if="canWrite" :label="t('run.addTests')" icon="pi pi-plus" severity="secondary" outlined @click="picker = true" />
        <Button :label="t('run.ciImport')" icon="pi pi-cloud-upload" severity="secondary" outlined @click="ciDialog = true" />
        <Button v-if="canWrite" icon="pi pi-pencil" severity="secondary" text rounded :aria-label="t('common.edit')" v-tooltip="t('common.edit')" @click="openEdit" />
        <Button v-if="canWrite" icon="pi pi-trash" severity="danger" text rounded :aria-label="t('common.delete')" v-tooltip="t('common.delete')" @click="remove" />
      </div>
    </div>
    <p v-if="run.description" class="muted" style="margin-top: -8px">{{ run.description }}</p>

    <div class="kpis" style="margin-bottom: 16px">
      <div class="card">
        <div class="small muted">{{ t('run.progress') }}</div>
        <div class="kpi"><div class="value">{{ formatNumber(summary.progress / 100, undefined, { style: 'percent' }) }}</div><span class="muted small">{{ t('run.executed', { executed: summary.executed, total: summary.total }) }}</span></div>
        <SummaryBar :summary="run.summary" style="margin-top: 8px" />
      </div>
      <div class="card">
        <div class="small muted">{{ t('run.passRate') }}</div>
        <div class="value" style="font-size: 24px; font-weight: 700">{{ formatNumber(summary.passRate / 100, undefined, { style: 'percent' }) }}</div>
      </div>
      <div v-for="s in summary.segments" :key="s.status" class="card">
        <div class="small muted row nowrap"><span aria-hidden="true" :style="{ width: '10px', height: '10px', borderRadius: '50%', background: RESULT_COLORS[s.status] ?? s.color }" />{{ enumLabel('result', s.status) }}</div>
        <div class="value" style="font-size: 24px; font-weight: 700">{{ s.count }}</div>
      </div>
    </div>

    <section v-if="hasConfigs" class="card" style="margin-bottom: 16px" aria-labelledby="h-per-config">
      <h2 id="h-per-config" class="plain-heading" style="margin-bottom: 8px">{{ t('configurations.perConfiguration') }}</h2>
      <div class="table-scroll">
        <table class="per-config">
          <thead>
            <tr>
              <th scope="col">{{ t('configurations.columnOne') }}</th>
              <th scope="col">{{ t('run.progress') }}</th>
              <th scope="col">{{ t('run.passRate') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="c in perConfig" :key="c.id">
              <th scope="row">
                <button type="button" class="link-button" :aria-pressed="configFilter === (c.id || 'none')" :title="t('configurations.showOnly')" @click="configFilter = configFilter === (c.id || 'none') ? '' : c.id || 'none'">{{ c.id ? c.name : t('configurations.withoutShort') }}</button>
              </th>
              <td style="min-width: 200px"><SummaryBar :summary="c.summary" show-text /></td>
              <td>{{ formatNumber(summarize(c.summary).passRate / 100, undefined, { style: 'percent' }) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <RunJobsPanel ref="jobsPanel" :run-id="runId" :can-cancel="canScript" @changed="reload" />

    <Tabs :id="tabsId" v-model:value="tab" class="editor-tabs" scrollable>
      <TabList>
        <Tab value="executions"><i class="pi pi-list" aria-hidden="true" /> {{ t('common.executions') }} <span class="count">{{ executions.length }}</span></Tab>
        <Tab value="attachments"><i class="pi pi-paperclip" aria-hidden="true" /> {{ t('files.tab') }} <span v-if="attachmentCount !== null" class="count">{{ attachmentCount }}</span></Tab>
        <Tab value="links"><i class="pi pi-link" aria-hidden="true" /> {{ t('links.title') }} <span v-if="linkCount !== null" class="count">{{ linkCount }}</span></Tab>
        <Tab value="discussion"><i class="pi pi-comments" aria-hidden="true" /> {{ t('discussions.tab') }} <span v-if="openDiscussions" class="count" :title="t('discussions.openBadge', { n: openDiscussions }, openDiscussions)">{{ openDiscussions }}</span><span v-if="openDiscussions" class="sr-only">{{ t('discussions.openBadge', { n: openDiscussions }, openDiscussions) }}</span></Tab>
      </TabList>
    </Tabs>
    <div v-show="tab === 'discussion'" v-bind="panel('discussion')" class="card">
      <DiscussionPanel entity="run" :entity-id="runId" :project-id="proj.pid" @open="(n) => (openDiscussions = n)" />
    </div>
    <div v-if="tab === 'links'" v-bind="panel('links')" class="card">
      <ItemLinksPanel item-type="run" :item-id="runId" :project-id="proj.pid" :can-write="canWrite" @count="(n) => (linkCount = n)" />
    </div>
    <div v-if="tab === 'attachments'" v-bind="panel('attachments')" class="card">
      <AttachmentsPanel type="run" :entity-id="runId" :project-id="proj.pid" :can-write="canExecute" @count="(n) => (attachmentCount = n)" />
    </div>
    <div v-show="tab === 'executions'" v-bind="panel('executions')" class="card">
      <div class="filterbar">
        <QueryInput v-model="q" scope="execution" :error="queryError" :composed="composed" :placeholder="t('run.filterPlaceholder', { example: 'assignee:alice title:login' })" @submit="loadExecutions" />
        <ColumnsMenu
          :columns="execColumns"
          :order="execLayout.order.value"
          @toggle="(id, on) => execLayout.toggle(id, on)"
          @move="(id, d) => execLayout.move(id, d)"
          @reset="execLayout.reset()"
        />
        <MultiSelect v-model="statusFilter" :options="resultOptions" option-label="label" option-value="value" :placeholder="t('common.status')" :aria-label="t('common.status')" style="min-width: 150px" show-clear @change="loadExecutions" />
        <Select v-if="hasConfigs" v-model="configFilter" :options="configFilterOptions" option-label="label" option-value="value" :aria-label="t('configurations.filter')" style="min-width: 170px" />
        <Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="reload" />
      </div>
      <Message v-if="readinessSummary.blocked || readinessSummary.warned" :severity="readinessSummary.blocked ? 'warn' : 'info'" :closable="false" style="margin-bottom: 10px">
        <i class="pi pi-exclamation-triangle" aria-hidden="true" />
        {{ t('readiness.runSummary', { blocked: readinessSummary.blocked, total: executions.length, warned: readinessSummary.warned }) }}
        <RouterLink v-if="auth.can(P.settingsManage)" :to="{ name: 'executionSettings' }">{{ t('readiness.settingsLink') }}</RouterLink>
      </Message>
      <div v-if="selection.length && canExecute" class="bulkbar" role="toolbar" :aria-label="t('run.bulk.toolbar')">
        <b>{{ t('common.selected', { count: selection.length }) }}</b>
        <Select v-model="bulkResult" :options="bulkResultOptions" option-label="label" option-value="value" :placeholder="t('run.bulk.setResult')" :aria-label="t('run.bulk.setResult')" size="small" style="min-width: 150px" />
        <Button :label="t('run.bulk.more')" icon="pi pi-pencil" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="openBulk" />
        <Button v-if="canScript" :label="t('webTests.runSelected')" icon="pi pi-bolt" size="small" severity="secondary" outlined :loading="queueing" @click="runAutomated(selectedIds)" />
        <Button v-if="canWrite" :label="t('run.bulk.remove')" icon="pi pi-minus-circle" size="small" severity="danger" outlined @click="bulkRemove" />
        <Button icon="pi pi-times" size="small" text rounded severity="secondary" :aria-label="t('workspace.clearSelection')" v-tooltip.bottom="t('workspace.clearSelection')" @click="selection = []" />
      </div>
      <DataTable
        :ref="execLayout.bind"
        v-model:selection="selection"
        v-stack
        :value="shownExecutions"
        data-key="id"
        :loading="loading"
        :aria-busy="loading"
        paginator
        :rows="prefs.rowsFor(50)"
        :rows-per-page-options="prefs.rowsPerPage"
        row-hover
        sort-field="position"
        :sort-order="1"
        size="small"
        reorderable-columns
        @column-reorder="execLayout.onReorder()"
      >
        <Column v-if="canExecute" selection-mode="multiple" column-key="_select" :reorderable-column="false" style="width: 2.6rem" />
        <Column field="position" header="#" sortable :reorderable-column="false" style="width: 60px" />
        <Column field="test_key" :header="t('run.test')" sortable :reorderable-column="false" style="width: 140px">
          <template #body="{ data }">
            <RouterLink :to="{ name: 'tests', params: { projectId: proj.pid }, query: { item: data.test_case_id } }" class="key">{{ data.test_key }}</RouterLink>
          </template>
        </Column>
        <Column field="test_title" :header="t('common.title')" sortable :reorderable-column="false" style="min-width: 200px" body-class="stack-main">
          <template #body="{ data }">
            <RouterLink :to="{ name: 'tests', params: { projectId: proj.pid }, query: { item: data.test_case_id } }" class="item-ref" tabindex="-1"><span class="item-ref-title">{{ data.test_title }}</span></RouterLink>
          </template>
        </Column>
        <template v-for="c in execLayout.visible.value" :key="c.field">
        <Column v-if="c.field === 'configuration' && hasConfigs" field="configuration_name" column-key="configuration" :header="c.header" sortable style="width: 150px">
          <template #body="{ data }"><Tag v-if="data.configuration_name" :value="data.configuration_name" icon="pi pi-sliders-h" severity="secondary" /><span v-else class="muted">—</span></template>
        </Column>
        <Column v-else-if="c.field === 'status'" field="status" column-key="status" :header="c.header" sortable style="width: 130px">
          <template #body="{ data }">
            <StatusTag :value="data.status" kind="result" />
            <span v-if="data.iteration_count" class="small muted" style="margin-left: 4px" :title="t('testData.iterationsCount', { n: data.iteration_count }, data.iteration_count)"><i class="pi pi-table" aria-hidden="true" /> {{ data.iteration_count }}<span class="sr-only"> {{ t('testData.iterationsCount', { n: data.iteration_count }, data.iteration_count) }}</span></span>
          </template>
        </Column>
        <Column v-else-if="c.field === 'readiness'" column-key="readiness" :header="c.header" style="width: 150px">
          <template #body="{ data }">
            <button
              v-if="data.readiness && !data.readiness.ready"
              type="button"
              class="readiness-cell small link-button"
              :class="{ blocked: notAllowed(data) }"
              :title="readinessTitle(data)"
              aria-haspopup="dialog"
              @click="showReadiness($event, data)"
            >
              <i :class="notAllowed(data) ? 'pi pi-ban' : 'pi pi-exclamation-triangle'" aria-hidden="true" />
              {{ notAllowed(data) ? t('readiness.cannotRun') : readinessText(data) }}
              <span class="sr-only">: {{ readinessTitle(data) }}</span>
            </button>
            <span v-else-if="data.readiness" class="small muted"><i class="pi pi-check" aria-hidden="true" /> {{ t('readiness.ok') }}</span>
          </template>
        </Column>
        <Column v-else-if="c.field === 'assignee'" field="assignee" column-key="assignee" :header="c.header" sortable style="width: 180px">
          <template #body="{ data }">
            <MentionInput
              v-if="canWrite"
              :model-value="assigneeDraft[data.id] ?? data.assignee"
              size="small"
              :placeholder="t('run.unassigned')"
              :aria-label="t('run.assigneeOf', { key: exLabel(data) })"
              class="w-full"
              @update:model-value="(v: string) => (assigneeDraft[data.id] = v)"
              @blur="setAssignee(data, assigneeDraft[data.id] ?? data.assignee)"
              @keydown.enter="setAssignee(data, assigneeDraft[data.id] ?? data.assignee)"
            />
            <MentionText v-else-if="data.assignee" :text="data.assignee" />
            <span v-else>—</span>
          </template>
        </Column>
        <Column v-else-if="c.field === 'executed_by'" field="executed_by" column-key="executed_by" :header="c.header" sortable style="width: 150px" />
        <Column v-else-if="c.field === 'defects'" column-key="defects" :header="c.header" style="min-width: 120px"><template #body="{ data }"><DefectLinks :defects="data.defects" /></template></Column>
        <Column v-else-if="c.field === 'duration_ms'" field="duration_ms" column-key="duration_ms" :header="c.header" sortable style="width: 110px"><template #body="{ data }">{{ formatDuration(data.duration_ms) }}</template></Column>
        <Column v-else-if="c.field === 'finished_at'" field="finished_at" column-key="finished_at" :header="c.header" sortable style="width: 150px"><template #body="{ data }"><span class="small muted">{{ formatDateTime(data.finished_at) }}</span></template></Column>
        </template>
        <Column column-key="_actions" :reorderable-column="false" style="width: 150px">
          <template #body="{ data }">
            <div class="row nowrap" style="gap: 0">
              <template v-if="canExecute">
                <Button
                  icon="pi pi-check"
                  text
                  rounded
                  size="small"
                  severity="success"
                  :disabled="notAllowed(data)"
                  v-tooltip="notAllowed(data) ? readinessTitle(data) : t('run.markPassed')"
                  :aria-label="t('run.markPassedKey', { key: exLabel(data) })"
                  @click="quickStatus(data, 'passed')"
                />
                <Button
                  icon="pi pi-times"
                  text
                  rounded
                  size="small"
                  severity="danger"
                  :disabled="notAllowed(data)"
                  v-tooltip="notAllowed(data) ? readinessTitle(data) : t('run.markFailed')"
                  :aria-label="t('run.markFailedKey', { key: exLabel(data) })"
                  @click="quickStatus(data, 'failed')"
                />
              </template>
              <Button :icon="canExecute ? 'pi pi-play' : 'pi pi-eye'" text rounded size="small" v-tooltip="canExecute ? t('run.execute') : t('run.view')" :aria-label="canExecute ? t('run.executeKey', { key: exLabel(data) }) : t('run.viewKey', { key: exLabel(data) })" @click="execute(data)" />
            </div>
          </template>
        </Column>
        <template #empty><div class="empty"><i class="pi pi-inbox" />{{ composed ? t('run.noneFilter') : t('run.none') }}</div></template>
      </DataTable>
    </div>

    <Dialog v-model:visible="editDialog" :header="t('run.edit')" modal style="width: min(560px, 95vw)">
      <Message v-if="editError" severity="error" :closable="false" style="margin-bottom: 10px">{{ editError }}</Message>
      <div class="form-grid">
        <div v-field class="field full"><label>{{ t('common.name') }}</label><SuggestInput v-model="editForm.name" field="run.name" :project-id="proj.pid" /></div>
        <div v-field class="field"><label>{{ t('common.environment') }}</label><InputText v-model="editForm.environment" /></div>
        <div v-field class="field"><label>{{ t('common.build') }}</label><InputText v-model="editForm.build" /></div>
        <div v-if="rel.releases.value.length" v-field class="field full">
          <label for="run-edit-release">{{ t('releasesView.release') }}</label>
          <Select v-model="editForm.release_id" input-id="run-edit-release" :options="rel.options.value" option-label="label" option-value="value" />
        </div>
        <div v-if="cfg.configurations.value.length || editForm.configuration_ids.length" class="field full">
          <label for="run-edit-configs">{{ t('configurations.runField') }}</label>
          <MultiSelect v-model="editForm.configuration_ids" input-id="run-edit-configs" :options="cfg.options.value" option-label="label" option-value="value" display="chip" filter :placeholder="t('configurations.runNone')" aria-describedby="run-edit-configs-help" />
          <small id="run-edit-configs-help" class="muted">{{ t('configurations.editHelp') }}</small>
        </div>
        <div v-field class="field full"><label>{{ t('common.description') }}</label><Textarea v-model="editForm.description" rows="3" auto-resize /></div>
      </div>
      <CustomFieldsEditor v-model="editForm.custom_fields" :fields="proj.customFieldsFor('run')" :project-id="proj.pid" />
      <template #footer>
        <Button :label="t('common.cancel')" text severity="secondary" @click="editDialog = false" />
        <Button :label="t('common.save')" icon="pi pi-check" :disabled="!editForm.name.trim()" @click="saveEdit" />
      </template>
    </Dialog>

    <Dialog v-model:visible="ciDialog" :header="t('run.ciHeader')" modal style="width: min(720px, 96vw)">
      <i18n-t keypath="run.ciIntro" tag="p">
        <template #link><RouterLink :to="{ name: 'tokens' }">{{ t('run.ciLink') }}</RouterLink></template>
      </i18n-t>
      <pre class="console" tabindex="0" :aria-label="t('run.ciHeader')">{{ ciSnippet }}</pre>
      <i18n-t keypath="run.ciStatus" tag="p" class="small muted">
        <template #status><code>status</code></template>
        <template #values>{{ meta.meta.result_statuses.join(', ') }}</template>
        <template #updated><code>updated</code></template>
        <template #unknown><code>unknown</code></template>
      </i18n-t>
      <Message v-if="runConfigs.length" severity="info" :closable="false" style="margin-bottom: 10px">
        <i18n-t keypath="configurations.ciHint" tag="span">
          <template #param><code>?configuration=</code></template>
          <template #field><code>configuration</code></template>
          <template #flag><code>--configuration</code></template>
        </i18n-t>
      </Message>
      <Panel :header="t('run.endpoint')" toggleable collapsed>
        <div class="mono">POST /api/v1/runs/{{ runId }}/results</div>
        <div class="mono">POST /api/v1/runs/{{ runId }}/junit</div>
      </Panel>
      <Panel :header="t('run.ciJunitHeader')" toggleable collapsed>
        <p class="small">{{ t('run.ciJunitIntro') }}</p>
        <pre class="console" tabindex="0" :aria-label="t('run.ciJunitHeader')">{{ ciJunitSnippet }}</pre>
        <Button :label="t('common.copy')" icon="pi pi-copy" severity="secondary" text size="small" @click="copySnippet(ciJunitSnippet)" />
      </Panel>
      <Panel :header="t('run.ciCliHeader')" toggleable collapsed>
        <p class="small">{{ t('run.ciCliIntro') }}</p>
        <pre class="console" tabindex="0" :aria-label="t('run.ciCliHeader')">{{ ciCliSnippet }}</pre>
        <Button :label="t('common.copy')" icon="pi pi-copy" severity="secondary" text size="small" @click="copySnippet(ciCliSnippet)" />
        <i18n-t keypath="run.ciPlaywright" tag="p" class="small muted">
          <template #pkg><code>@qtestframe/playwright-reporter</code></template>
        </i18n-t>
      </Panel>
      <template #footer>
        <Button :label="t('common.copy')" icon="pi pi-copy" severity="secondary" @click="copySnippet()" />
        <Button :label="t('common.close')" @click="ciDialog = false" />
      </template>
    </Dialog>
    <ItemPickerDialog v-model:visible="picker" kind="test" :project-id="proj.pid" :exclude="executions.map((e) => e.test_case_id)" :header="t('run.addTestsHeader')" @pick="addTests" />
  </template>
  <Popover ref="readinessPop" class="readiness-pop" :aria-label="t('readiness.column')">
    <ReadinessBanner v-if="readinessOf" :readiness="readinessOf.readiness" :project-id="proj.pid" />
  </Popover>
  <Dialog v-model:visible="bulkDialog" :header="t('run.bulk.dialogTitle', { n: selection.length }, selection.length)" modal style="width: min(520px, 96vw)">
    <div class="bulk-form">
      <label for="bk-result">{{ t('run.bulk.result') }}</label>
      <Select v-model="bulkForm.status" input-id="bk-result" :options="[{ label: t('run.bulk.unchanged'), value: '' }, ...bulkResultOptions]" option-label="label" option-value="value" fluid />
      <div class="row">
        <Checkbox v-model="bulkForm.assigneeOn" binary input-id="bk-assign-on" />
        <label for="bk-assign-on">{{ t('run.bulk.assignTo') }}</label>
      </div>
      <MentionInput v-model="bulkForm.assignee" :project-id="proj.pid" :disabled="!bulkForm.assigneeOn" :placeholder="t('run.bulk.emptyUnassigns')" :aria-label="t('run.bulk.assignTo')" />
      <label for="bk-note">{{ t('run.bulk.note') }}</label>
      <Textarea id="bk-note" v-model="bulkForm.note" rows="2" auto-resize fluid aria-describedby="bk-note-help" />
      <small id="bk-note-help" class="muted">{{ t('run.bulk.noteHelp') }}</small>
      <div class="row">
        <Checkbox v-model="bulkForm.onlyNotRun" binary input-id="bk-only" />
        <label for="bk-only">{{ t('run.bulk.onlyNotRun') }}</label>
      </div>
      <small class="muted">{{ t('run.bulk.readinessHelp') }}</small>
    </div>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="bulkDialog = false" />
      <Button :label="t('common.apply')" icon="pi pi-check" :disabled="!bulkForm.status && !bulkForm.assigneeOn && !bulkForm.note.trim()" @click="applyBulk" />
    </template>
  </Dialog>
</template>


<style scoped>
.table-scroll {
  overflow-x: auto;
}
.per-config {
  border-collapse: collapse;
  width: 100%;
}
.per-config th,
.per-config td {
  text-align: left;
  padding: 4px 8px;
  border-bottom: 1px solid var(--qtf-border);
}
.per-config thead th {
  font-size: 12px;
  font-weight: 600;
}
.bulk-form {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.bulk-form > label {
  font-weight: 600;
  font-size: 13px;
  margin-top: 6px;
}
.bulk-form .row {
  gap: 8px;
  margin-top: 6px;
}
</style>
