<script setup lang="ts">
// One test cycle: its plan (tests per configuration with tester and
// estimate), the board by tester, the progress with the burndown and the
// workload per tester, and its history.
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Chart from 'primevue/chart'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import Textarea from 'primevue/textarea'
import ItemRef from '@/components/ItemRef.vue'
import ItemPickerDialog from '@/components/ItemPickerDialog.vue'
import FolderPickerDialog from '@/components/FolderPickerDialog.vue'
import HistoryPanel from '@/components/HistoryPanel.vue'
import QueryInput from '@/components/QueryInput.vue'
import StatusTag from '@/components/StatusTag.vue'
import SummaryBar from '@/components/SummaryBar.vue'
import VariantSelect from '@/components/VariantSelect.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { boardColumns, burndownGap, burndownSeries, cycleSeverity, executed, minutesText, overdue, progressSummary } from '@/lib/cycles'
import { describeMulti } from '@/lib/metrics'
import { RESULT_COLORS } from '@/lib/colors'
import { formatDate, formatDateTime, formatDuration, formatNumber } from '@/lib/format'
import { P } from '@/lib/permissions'
import { uid } from '@/lib/a11y'
import { seriesColor } from '@/themes/chart'
import type { CycleReport, Release, TestCycle, TestCycleTest } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useProjectPeopleStore } from '@/stores/projectPeople'
import { useVariants } from '@/composables/variants'
import { useConfigurations } from '@/composables/configurations'
import { useChartTheme } from '@/composables/chartTheme'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'
import { enumLabel } from '@/i18n/labels'

const props = defineProps<{ cycleId: string }>()
const proj = useProjectStore()
const auth = useAuthStore()
const people = useProjectPeopleStore()
const notify = useNotify()
const route = useRoute()
const router = useRouter()
const theme = useChartTheme()
const { t } = useI18n()
const id = uid('cy')
const tabsId = uid('cy-tabs')
const panel = (value: string) => ({ role: 'tabpanel', id: `${tabsId}_tabpanel_${value}`, 'aria-labelledby': `${tabsId}_tab_${value}` })
const pid = computed(() => proj.pid)
const canPlan = computed(() => auth.can(P.cyclePlan))
const canRun = computed(() => auth.can([P.cyclePlan, P.runWrite]))
const vs = useVariants(() => pid.value)
const cfg = useConfigurations(() => pid.value)
const units = computed(() => ({ h: t('cycles.unitH'), min: t('cycles.unitMin') }))
const mins = (m: number | null | undefined) => minutesText(m, units.value)
const statusLabel = (s: string) => t(`cycles.status.${s}`)
const today = new Date().toISOString().slice(0, 10)

type TabId = 'plan' | 'board' | 'progress' | 'history'
const TABS: TabId[] = ['plan', 'board', 'progress', 'history']
const tab = ref<TabId>(TABS.includes(route.query.tab as TabId) ? (route.query.tab as TabId) : 'plan')
watch(tab, (v) => {
  router.replace({ query: { ...route.query, tab: v === 'plan' ? undefined : v } })
  if (v === 'progress') void loadProgress()
})

// ---------- people ----------
const testers = computed(() => people.people.filter((p) => p.kind === 'user' && p.user_id))
const personName = (u: string) => {
  if (!u) return t('cycles.unassigned')
  const p = testers.value.find((x) => x.user_id === u)
  return p ? p.name || p.email || u : u === auth.user?.id ? auth.user?.display_name || auth.user?.email || '' : t('cycles.unknownPerson')
}
const testerOptions = computed(() => [
  { label: t('cycles.unassigned'), value: '' },
  ...testers.value.map((p) => ({ label: p.name && p.email ? `${p.name} (${p.email})` : p.name || p.email || '', value: p.user_id! })),
])

// ---------- cycle ----------
const cycle = ref<TestCycle | null>(null)
const plan = ref<TestCycleTest[]>([])
const loadError = ref('')
async function load() {
  loadError.value = ''
  try {
    const [c, p] = await Promise.all([api.get<TestCycle>(`/cycles/${props.cycleId}`), api.get<{ items: TestCycleTest[] }>(`/cycles/${props.cycleId}/tests`)])
    cycle.value = c
    plan.value = asItems(p)
  } catch (e) {
    loadError.value = errorMessage(e)
  }
  if (tab.value === 'progress') await loadProgress()
}
const p = computed(() => cycle.value?.progress)
const kpis = computed(() => {
  const x = p.value
  if (!x) return []
  return [
    { label: t('cycles.kpiPlanned'), value: formatNumber(x.planned), sub: t('cycles.kpiUnassigned', { n: x.unassigned }, x.unassigned) },
    { label: t('cycles.kpiExecuted'), value: `${formatNumber(x.percent_executed)} %`, sub: t('cycles.executedOf', { executed: x.executed, planned: x.planned, pct: x.percent_executed }) },
    { label: t('cycles.kpiPassed'), value: formatNumber(x.passed), sub: t('cycles.kpiFailedBlocked', { failed: x.failed, blocked: x.blocked }) },
    { label: t('cycles.kpiEstimate'), value: mins(x.estimated_minutes), sub: t('cycles.kpiRemaining', { time: mins(x.remaining_minutes) }) },
    { label: t('cycles.kpiActual'), value: x.actual_ms ? formatDuration(x.actual_ms) : '—', sub: t('cycles.kpiEstimatedDone', { time: mins(x.estimated_minutes_executed) }) },
  ]
})

// ---------- status actions ----------
const busy = ref(false)
async function setStatus(status: string) {
  if (!cycle.value) return
  busy.value = true
  try {
    cycle.value = await api.patch<TestCycle>(`/cycles/${cycle.value.id}`, { status })
    notify.ok(t(`cycles.statusChanged.${status}`), cycle.value.name)
    await load()
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = false
  }
}
async function createRun() {
  if (!cycle.value) return
  busy.value = true
  try {
    const run = await api.post<{ id: string; name: string }>(`/cycles/${cycle.value.id}/runs`)
    notify.ok(t('cycles.runCreated'), run.name)
    await load()
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = false
  }
}
async function remove() {
  if (!cycle.value || !window.confirm(t('cycles.confirmDelete', { name: cycle.value.name }))) return
  try {
    await api.del(`/cycles/${cycle.value.id}`)
    notify.ok(t('cycles.deleted'), cycle.value.name)
    router.push({ name: 'cycles', params: { projectId: pid.value } })
  } catch (e) {
    notify.fail(e)
  }
}

// ---------- edit ----------
const releases = ref<Release[]>([])
const releaseOptions = computed(() => releases.value.map((r) => ({ label: r.version && r.version !== r.name ? `${r.name} (${r.version})` : r.name, value: r.id })))
const editDialog = ref(false)
const editError = ref('')
const edit = reactive({ name: '', goal: '', description: '', owner_id: '', release_id: null as string | null, variant_id: null as string | null, environment: '', configuration_ids: [] as string[], start_date: '', end_date: '', scheduled_start: '' })
function toLocal(iso: string | null): string {
  if (!iso) return ''
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}
function openEdit() {
  const c = cycle.value
  if (!c) return
  Object.assign(edit, {
    name: c.name,
    goal: c.goal,
    description: c.description,
    owner_id: c.owner_id,
    release_id: c.release_id,
    variant_id: c.variant_id,
    environment: c.environment,
    configuration_ids: [...c.configuration_ids],
    start_date: c.start_date,
    end_date: c.end_date,
    scheduled_start: toLocal(c.scheduled_start_at),
  })
  editError.value = ''
  editDialog.value = true
}
async function saveEdit() {
  if (!cycle.value) return
  busy.value = true
  editError.value = ''
  const body: Record<string, unknown> = { ...edit, release_id: edit.release_id ?? '', variant_id: edit.variant_id ?? '' }
  delete body.scheduled_start
  if (edit.scheduled_start) body.scheduled_start_at = new Date(edit.scheduled_start).toISOString()
  else body.clear_scheduled_start = true
  try {
    await api.patch(`/cycles/${cycle.value.id}`, body)
    editDialog.value = false
    notify.ok(t('cycles.saved'))
    await load()
  } catch (e) {
    editError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

// ---------- plan ----------
const selection = ref<TestCycleTest[]>([])
const selectedIds = computed(() => selection.value.map((r) => r.id))
const bulkTester = ref<string | null>(null)
const bulkEstimate = ref<number | null>(null)
async function bulkUpdate(body: Record<string, unknown>) {
  if (!cycle.value || !selectedIds.value.length) return
  try {
    const res = await api.patch<{ updated: number }>(`/cycles/${cycle.value.id}/tests`, { ids: selectedIds.value, ...body })
    notify.ok(t('cycles.plan.updated', { n: res.updated }, res.updated))
    selection.value = []
    await load()
  } catch (e) {
    notify.fail(e)
  }
}
async function removeRows() {
  if (!cycle.value || !selectedIds.value.length || !window.confirm(t('cycles.plan.confirmRemove', { n: selectedIds.value.length }, selectedIds.value.length))) return
  try {
    const res = await api.del<{ removed: number }>(`/cycles/${cycle.value.id}/tests`, { ids: selectedIds.value })
    notify.ok(t('cycles.plan.removed', { n: res.removed }, res.removed))
    selection.value = []
    await load()
  } catch (e) {
    notify.fail(e)
  }
}
const execRoute = (r: TestCycleTest) => (r.run_id && r.execution_id ? { name: 'execute', params: { projectId: pid.value, runId: r.run_id, executionId: r.execution_id } } : undefined)
const testRoute = (r: TestCycleTest) => ({ name: 'tests', params: { projectId: pid.value }, query: { item: r.test_case_id } })

// ---------- add tests ----------
type AddMode = 'selection' | 'query' | 'folder' | 'regression' | 'copy'
const addDialog = ref(false)
const addError = ref<unknown>(null)
const add = reactive({ mode: 'selection' as AddMode, tests: [] as { id: string; key: string; title: string }[], query: '', folder: null as string | null, recursive: true, requirements: '', related: false, copy: null as string | null, assignee: '', estimate: null as number | null, configs: [] as string[] })
const picker = ref(false)
const folderPicker = ref(false)
const modeOptions = computed(() => (['selection', 'query', 'folder', 'regression', 'copy'] as AddMode[]).map((m) => ({ label: t(`cycles.add.mode.${m}`), value: m })))
const cycleConfigOptions = computed(() => cfg.options.value.filter((o) => cycle.value?.configuration_ids.includes(o.value)))
const otherCycles = ref<TestCycle[]>([])
const copyOptions = computed(() => otherCycles.value.filter((c) => c.id !== cycle.value?.id).map((c) => ({ label: c.name, value: c.id })))
const folderName = computed(() => {
  const f = proj.folders.test.find((x) => x.id === add.folder)
  return f?.name ?? ''
})
async function openAdd() {
  Object.assign(add, { mode: 'selection', tests: [], query: '', folder: null, recursive: true, requirements: '', related: false, copy: null, assignee: '', estimate: null, configs: [] })
  addError.value = null
  addDialog.value = true
  try {
    otherCycles.value = asItems(await api.get<{ items: TestCycle[] }>(`/projects/${pid.value}/cycles`))
  } catch {
    otherCycles.value = []
  }
}
const addReady = computed(() => {
  switch (add.mode) {
    case 'selection':
      return add.tests.length > 0
    case 'query':
      return add.query.trim() !== ''
    case 'folder':
      return !!add.folder
    case 'regression':
      return add.requirements.trim() !== ''
    case 'copy':
      return !!add.copy
  }
  return false
})
async function submitAdd() {
  if (!cycle.value) return
  busy.value = true
  addError.value = null
  const body: Record<string, unknown> = { assignee_id: add.assignee }
  if (add.estimate !== null) body.estimate_minutes = add.estimate
  if (add.configs.length) body.configuration_ids = add.configs
  if (add.mode === 'selection') body.test_ids = add.tests.map((x) => x.id)
  if (add.mode === 'query') body.query = add.query
  if (add.mode === 'folder') Object.assign(body, { folder_id: add.folder, recursive: add.recursive })
  if (add.mode === 'regression')
    body.regression = {
      requirement_ids: add.requirements
        .split(/[\s,;]+/)
        .map((s) => s.trim())
        .filter(Boolean),
      include_related: add.related,
      variant: cycle.value.variant_id ?? '',
    }
  if (add.mode === 'copy') body.copy_from = add.copy
  try {
    const res = await api.post<{ added: number }>(`/cycles/${cycle.value.id}/tests`, body)
    addDialog.value = false
    notify.ok(t('cycles.add.added', { n: res.added }, res.added))
    await load()
  } catch (e) {
    addError.value = e
  } finally {
    busy.value = false
  }
}

// ---------- board ----------
const columns = computed(() => boardColumns(plan.value, personName))

// ---------- progress ----------
const report = ref<CycleReport | null>(null)
async function loadProgress() {
  try {
    report.value = await api.get<CycleReport>(`/cycles/${props.cycleId}/progress`)
  } catch (e) {
    notify.fail(e)
  }
}
const burnUnit = ref<'tests' | 'minutes'>('tests')
const unitOptions = computed(() => [
  { label: t('cycles.progressTab.unitTests'), value: 'tests' },
  { label: t('cycles.progressTab.unitMinutes'), value: 'minutes' },
])
const burnChart = computed(() => {
  const s = burndownSeries(report.value?.burndown, burnUnit.value === 'minutes')
  const ideal = theme.value.text
  return {
    labels: s.days.map(formatDate),
    datasets: [
      { label: t('cycles.progressTab.remaining'), data: s.remaining, borderColor: seriesColor(theme.value, 0), backgroundColor: seriesColor(theme.value, 0), pointRadius: 2, borderWidth: 2, spanGaps: false },
      { label: t('cycles.progressTab.ideal'), data: s.ideal, borderColor: ideal, backgroundColor: ideal, borderDash: [5, 4], pointRadius: 0, borderWidth: 1.5 },
    ],
  }
})
const gap = computed(() => burndownGap(report.value?.burndown, burnUnit.value === 'minutes'))
const gapText = computed(() => {
  const g = gap.value
  if (g === null) return ''
  const n = burnUnit.value === 'minutes' ? mins(Math.abs(g)) : formatNumber(Math.abs(g))
  return g > 0 ? t('cycles.progressTab.behind', { n }) : g < 0 ? t('cycles.progressTab.ahead', { n }) : t('cycles.progressTab.onTrack')
})
const workloadChart = computed(() => {
  const list = report.value?.testers ?? []
  return {
    labels: list.map((x) => personName(x.assignee_id)),
    datasets: [
      { label: enumLabel('result', 'passed'), data: list.map((x) => x.passed), backgroundColor: RESULT_COLORS.passed },
      { label: enumLabel('result', 'failed'), data: list.map((x) => x.failed), backgroundColor: RESULT_COLORS.failed },
      { label: enumLabel('result', 'blocked'), data: list.map((x) => x.blocked), backgroundColor: RESULT_COLORS.blocked },
      { label: t('cycles.progressTab.otherDone'), data: list.map((x) => x.executed - x.passed - x.failed - x.blocked), backgroundColor: RESULT_COLORS.skipped },
      { label: t('cycles.progressTab.open'), data: list.map((x) => x.planned - x.executed), backgroundColor: RESULT_COLORS.not_run },
    ].filter((d) => d.data.some((v) => v > 0)),
  }
})
const workloadHeight = computed(() => `${Math.max(160, 60 + (report.value?.testers.length ?? 0) * 38)}px`)
function font() {
  return { family: theme.value.font }
}
const lineOptions = computed(() => {
  const c = theme.value
  return {
    maintainAspectRatio: false,
    interaction: { mode: 'index', intersect: false },
    plugins: { legend: { display: true, position: 'bottom', labels: { color: c.text, usePointStyle: true, boxWidth: 8, font: font() } }, tooltip: { titleFont: font(), bodyFont: font() } },
    scales: {
      x: { ticks: { color: c.text, font: font(), maxRotation: 0, autoSkip: true, maxTicksLimit: 8 }, grid: { display: false }, border: { color: c.grid } },
      y: { beginAtZero: true, ticks: { color: c.text, precision: 0, font: font() }, grid: { color: c.grid }, border: { color: c.grid } },
    },
  }
})
const barOptions = computed(() => {
  const c = theme.value
  const axis = { ticks: { color: c.text, precision: 0, font: font() }, grid: { color: c.grid }, border: { color: c.grid }, stacked: true }
  return {
    maintainAspectRatio: false,
    indexAxis: 'y',
    plugins: { legend: { display: true, position: 'bottom', labels: { color: c.text, usePointStyle: true, boxWidth: 8, font: font() } }, tooltip: { titleFont: font(), bodyFont: font() } },
    scales: { x: { ...axis, beginAtZero: true }, y: axis },
  }
})

onMounted(async () => {
  void people.ensure(pid.value)
  await Promise.all([
    load(),
    vs.load(),
    cfg.load(),
    (async () => {
      try {
        releases.value = asItems(await api.get<{ items: Release[] }>(`/projects/${pid.value}/releases`))
      } catch {
        releases.value = []
      }
    })(),
  ])
})
watch(() => props.cycleId, () => void load())
</script>

<template>
  <div class="page-header">
    <div style="min-width: 0">
      <RouterLink :to="{ name: 'cycles', params: { projectId: pid } }" class="small"><i class="pi pi-arrow-left" aria-hidden="true" /> {{ t('cycles.back') }}</RouterLink>
      <h1 class="row nowrap" style="gap: 8px">
        <span class="ellipsis">{{ cycle?.name ?? t('cycles.title') }}</span>
        <Tag v-if="cycle" :value="statusLabel(cycle.status)" :severity="cycleSeverity(cycle.status)" />
        <Tag v-if="cycle && overdue(cycle, today)" :value="t('cycles.overdue')" severity="warn" />
      </h1>
      <div v-if="cycle" class="sub">
        <span v-if="cycle.goal">{{ cycle.goal }} · </span>
        {{ cycle.start_date ? formatDate(cycle.start_date) : '…' }} – {{ cycle.end_date ? formatDate(cycle.end_date) : '…' }}
        · {{ t('cycles.ownedBy', { name: personName(cycle.owner_id) }) }}
        <span v-if="cycle.release_name"> · {{ t('cycles.forRelease', { name: cycle.release_name }) }}</span>
        <span v-if="cycle.variant_name"> · {{ t('cycles.forVariant', { name: cycle.variant_name }) }}</span>
        <span v-if="cycle.build_label"> · {{ t('cycles.forBuild', { name: cycle.build_label }) }}</span>
        <span v-if="cycle.scheduled_start_at"> · <i class="pi pi-clock" aria-hidden="true" /> {{ t('cycles.startsAt', { when: formatDateTime(cycle.scheduled_start_at) }) }}</span>
      </div>
    </div>
    <div v-if="cycle" class="actions">
      <HelpLink page="test-cycles" />
      <template v-if="canPlan">
        <Button v-if="cycle.status === 'planned'" :label="t('cycles.start')" icon="pi pi-play" :loading="busy" :disabled="!plan.length || !canRun" v-tooltip="t('cycles.startHint')" @click="setStatus('active')" />
        <Button v-if="cycle.status === 'active' && cycle.progress.not_in_run > 0" :label="t('cycles.createRun', { n: cycle.progress.not_in_run })" icon="pi pi-plus-circle" :loading="busy" :disabled="!canRun" @click="createRun" />
        <Button v-if="cycle.status === 'active'" :label="t('cycles.close')" icon="pi pi-check-circle" severity="secondary" outlined :loading="busy" @click="setStatus('closed')" />
        <Button v-if="cycle.status === 'closed'" :label="t('cycles.reopen')" icon="pi pi-replay" severity="secondary" outlined :loading="busy" @click="setStatus('active')" />
        <Button icon="pi pi-pencil" text rounded severity="secondary" :aria-label="t('cycles.edit')" v-tooltip="t('cycles.edit')" aria-haspopup="dialog" @click="openEdit" />
        <Button icon="pi pi-trash" text rounded severity="danger" :aria-label="t('cycles.delete')" v-tooltip="t('cycles.delete')" @click="remove" />
      </template>
    </div>
  </div>

  <Message v-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>

  <template v-if="cycle">
    <ul class="kpis plain-list" :aria-label="t('cycles.kpis')">
      <li v-for="k in kpis" :key="k.label" class="card kpi">
        <div class="kpi-label small muted">{{ k.label }}</div>
        <div class="kpi-value">{{ k.value }}</div>
        <div class="small muted">{{ k.sub }}</div>
      </li>
    </ul>
    <div class="card" style="padding: 10px 14px; margin-top: 10px">
      <SummaryBar :summary="progressSummary(cycle.progress)" show-text />
    </div>

    <Tabs :id="tabsId" v-model:value="tab" class="editor-tabs" style="margin-top: 12px" scrollable>
      <TabList :aria-label="t('cycles.sections')">
        <Tab value="plan"><i class="pi pi-list-check" aria-hidden="true" /> {{ t('cycles.tabPlan') }}</Tab>
        <Tab value="board"><i class="pi pi-users" aria-hidden="true" /> {{ t('cycles.tabBoard') }}</Tab>
        <Tab value="progress"><i class="pi pi-chart-line" aria-hidden="true" /> {{ t('cycles.tabProgress') }}</Tab>
        <Tab value="history"><i class="pi pi-history" aria-hidden="true" /> {{ t('cycles.tabHistory') }}</Tab>
      </TabList>
    </Tabs>

    <!-- plan -->
    <section v-if="tab === 'plan'" v-bind="panel('plan')" style="margin-top: 12px">
      <h2 class="sr-only">{{ t('cycles.tabPlan') }}</h2>
      <div v-if="canPlan" class="filterbar">
        <Button :label="t('cycles.add.button')" icon="pi pi-plus" size="small" :disabled="cycle.status === 'closed'" aria-haspopup="dialog" @click="openAdd" />
        <template v-if="selection.length">
          <span class="small muted">{{ t('cycles.plan.selected', { n: selection.length }, selection.length) }}</span>
          <Select v-model="bulkTester" :options="testerOptions" option-label="label" option-value="value" filter size="small" :placeholder="t('cycles.plan.assignTo')" :aria-label="t('cycles.plan.assignTo')" style="min-width: 180px" />
          <Button :label="t('cycles.plan.assign')" size="small" outlined :disabled="bulkTester === null" @click="bulkUpdate({ assignee_id: bulkTester })" />
          <InputNumber v-model="bulkEstimate" :min="0" :max="100000" size="small" :placeholder="t('cycles.plan.estimateMin')" :aria-label="t('cycles.plan.estimateMin')" :input-style="{ width: '110px' }" />
          <Button :label="t('cycles.plan.setEstimate')" size="small" outlined :disabled="bulkEstimate === null" @click="bulkUpdate({ estimate_minutes: bulkEstimate })" />
          <Button :label="t('cycles.plan.remove')" icon="pi pi-trash" size="small" severity="danger" text @click="removeRows" />
        </template>
      </div>
      <DataTable v-model:selection="selection" :value="plan" data-key="id" size="small" class="card" striped-rows :aria-label="t('cycles.plan.tableLabel')" paginator :rows="50" :rows-per-page-options="[25, 50, 100, 500]">
        <template #empty>{{ t('cycles.plan.empty') }}</template>
        <Column v-if="canPlan" selection-mode="multiple" header-style="width: 3rem" />
        <Column field="test_key" :header="t('cycles.plan.test')" sortable>
          <template #body="{ data }"><ItemRef :to="testRoute(data)" :item-key="data.test_key" :title="data.test_title" title-class="small muted ellipsis" /></template>
        </Column>
        <Column field="configuration_name" :header="t('cycles.plan.configuration')" sortable style="width: 140px">
          <template #body="{ data }"><span class="small">{{ data.configuration_name || '—' }}</span></template>
        </Column>
        <Column field="assignee_id" :header="t('cycles.plan.tester')" sortable style="width: 170px">
          <template #body="{ data }"><span class="small" :class="{ muted: !data.assignee_id }">{{ personName(data.assignee_id) }}</span></template>
        </Column>
        <Column field="estimate_minutes" :header="t('cycles.plan.estimate')" sortable style="width: 110px">
          <template #body="{ data }"><span class="small">{{ mins(data.estimate_minutes) }}</span></template>
        </Column>
        <Column field="status" :header="t('cycles.plan.result')" sortable style="width: 130px">
          <template #body="{ data }">
            <RouterLink v-if="execRoute(data)" :to="execRoute(data)!" :aria-label="t('cycles.plan.openExecution', { key: data.test_key })"><StatusTag :value="data.status" kind="result" /></RouterLink>
            <span v-else class="small muted">{{ t('cycles.plan.notInRun') }}</span>
          </template>
        </Column>
        <Column field="duration_ms" :header="t('cycles.plan.actual')" sortable style="width: 110px">
          <template #body="{ data }"><span class="small">{{ executed(data.status) && data.duration_ms ? formatDuration(data.duration_ms) : '—' }}</span></template>
        </Column>
      </DataTable>
    </section>

    <!-- board by tester -->
    <section v-else-if="tab === 'board'" v-bind="panel('board')" style="margin-top: 12px">
      <h2 class="sr-only">{{ t('cycles.tabBoard') }}</h2>
      <div v-if="!columns.length" class="card empty-state"><p>{{ t('cycles.plan.empty') }}</p></div>
      <div v-else class="board">
        <section v-for="c in columns" :key="c.assignee || 'none'" class="card board-col" :aria-labelledby="`${id}-col-${c.assignee || 'none'}`">
          <h3 :id="`${id}-col-${c.assignee || 'none'}`" class="board-head">
            <span class="ellipsis">{{ personName(c.assignee) }}</span>
            <span class="small muted">{{ t('cycles.board.counts', { done: c.done.length, total: c.rows.length }) }}</span>
          </h3>
          <div class="small muted">{{ t('cycles.remainingOf', { remaining: mins(c.remaining), total: mins(c.estimate) }) }}</div>
          <ul class="plain-list board-list" :aria-label="t('cycles.board.open')">
            <li v-for="r in c.open" :key="r.id" class="board-item">
              <ItemRef :to="execRoute(r) ?? testRoute(r)" :item-key="r.test_key" :title="r.test_title" title-class="small ellipsis" grow />
              <span class="small muted nowrap">{{ r.configuration_name }}{{ r.configuration_name ? ' · ' : '' }}{{ mins(r.estimate_minutes) }}</span>
            </li>
            <li v-if="!c.open.length" class="small muted">{{ t('cycles.board.allDone') }}</li>
          </ul>
          <details v-if="c.done.length" class="board-done">
            <summary class="small">{{ t('cycles.board.done', { n: c.done.length }, c.done.length) }}</summary>
            <ul class="plain-list board-list">
              <li v-for="r in c.done" :key="r.id" class="board-item">
                <ItemRef :to="execRoute(r) ?? testRoute(r)" :item-key="r.test_key" :title="r.test_title" title-class="small ellipsis" grow />
                <StatusTag :value="r.status" kind="result" />
              </li>
            </ul>
          </details>
        </section>
      </div>
    </section>

    <!-- progress -->
    <section v-else-if="tab === 'progress'" v-bind="panel('progress')" style="margin-top: 12px" class="charts">
      <section class="card" aria-labelledby="cy-burn-h">
        <div class="row" style="justify-content: space-between">
          <h2 id="cy-burn-h" style="margin: 0; font-size: 16px">{{ t('cycles.progressTab.burndown') }}</h2>
          <SelectButton v-model="burnUnit" :options="unitOptions" option-label="label" option-value="value" :allow-empty="false" size="small" :aria-label="t('cycles.progressTab.unit')" />
        </div>
        <div class="small muted">{{ t('cycles.progressTab.burndownHelp') }} <b v-if="gapText">{{ gapText }}</b></div>
        <div class="chart-box">
          <div v-if="report" role="img" :aria-label="describeMulti(t('cycles.progressTab.burndown'), burnChart, formatNumber, 10)" class="chart-fill">
            <Chart type="line" :data="burnChart" :options="lineOptions" style="height: 100%" />
          </div>
        </div>
      </section>
      <section class="card" aria-labelledby="cy-load-h">
        <h2 id="cy-load-h" style="margin: 0; font-size: 16px">{{ t('cycles.progressTab.workload') }}</h2>
        <div v-if="report && report.testers.length" :style="{ height: workloadHeight }" role="img" :aria-label="describeMulti(t('cycles.progressTab.workload'), workloadChart, formatNumber, 20)">
          <Chart type="bar" :data="workloadChart" :options="barOptions" style="height: 100%" />
        </div>
        <table v-if="report && report.testers.length" class="load-table small">
          <caption class="sr-only">{{ t('cycles.progressTab.workload') }}</caption>
          <thead>
            <tr>
              <th scope="col">{{ t('cycles.plan.tester') }}</th>
              <th scope="col">{{ t('cycles.progressTab.planned') }}</th>
              <th scope="col">{{ t('cycles.progressTab.executed') }}</th>
              <th scope="col">{{ t('cycles.plan.estimate') }}</th>
              <th scope="col">{{ t('cycles.progressTab.remainingTime') }}</th>
              <th scope="col">{{ t('cycles.plan.actual') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="x in report.testers" :key="x.assignee_id || 'none'">
              <th scope="row">{{ personName(x.assignee_id) }}</th>
              <td>{{ x.planned }}</td>
              <td>{{ x.executed }}</td>
              <td>{{ mins(x.estimated_minutes) }}</td>
              <td>{{ mins(x.remaining_minutes) }}</td>
              <td>{{ x.actual_ms ? formatDuration(x.actual_ms) : '—' }}</td>
            </tr>
          </tbody>
        </table>
        <p v-else class="small muted">{{ t('cycles.plan.empty') }}</p>
      </section>
    </section>

    <!-- history -->
    <section v-else v-bind="panel('history')" style="margin-top: 12px" class="card">
      <h2 class="sr-only">{{ t('cycles.tabHistory') }}</h2>
      <HistoryPanel entity="cycle" :entity-id="cycle.id" :can-restore="false" :current-version="cycle.version" />
    </section>
  </template>

  <!-- add tests -->
  <Dialog v-model:visible="addDialog" modal :header="t('cycles.add.title')" style="width: min(680px, 96vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="submitAdd">
      <Message v-if="addError && !(addError as { code?: string }).code?.startsWith('invalid_query')" severity="error" :closable="false">{{ errorMessage(addError) }}</Message>
      <div class="field">
        <span :id="`${id}-mode-label`" class="field-label">{{ t('cycles.add.how') }}</span>
        <SelectButton v-model="add.mode" :options="modeOptions" option-label="label" option-value="value" :allow-empty="false" :aria-labelledby="`${id}-mode-label`" class="wrap-buttons" />
      </div>
      <div v-if="add.mode === 'selection'" class="field">
        <div class="row">
          <Button :label="t('cycles.add.pick')" icon="pi pi-search" size="small" outlined type="button" aria-haspopup="dialog" @click="picker = true" />
          <span class="small muted">{{ t('cycles.add.picked', { n: add.tests.length }, add.tests.length) }}</span>
        </div>
        <div v-if="add.tests.length" class="small">{{ add.tests.map((x) => x.key).join(', ') }}</div>
      </div>
      <div v-else-if="add.mode === 'query'" class="field">
        <QueryInput v-model="add.query" scope="test" :error="addError" :placeholder="t('cycles.add.queryPlaceholder')" />
      </div>
      <div v-else-if="add.mode === 'folder'" class="field">
        <div class="row">
          <Button :label="t('cycles.add.chooseFolder')" icon="pi pi-folder" size="small" outlined type="button" aria-haspopup="dialog" @click="folderPicker = true" />
          <span class="small">{{ folderName || t('cycles.add.noFolder') }}</span>
        </div>
        <div class="row">
          <Checkbox v-model="add.recursive" binary :input-id="`${id}-rec`" />
          <label :for="`${id}-rec`">{{ t('cycles.add.recursive') }}</label>
        </div>
      </div>
      <template v-else-if="add.mode === 'regression'">
        <div class="field">
          <label :for="`${id}-reqs`">{{ t('cycles.add.changedRequirements') }}</label>
          <InputText :id="`${id}-reqs`" v-model="add.requirements" placeholder="REQ-12, REQ-14" :aria-describedby="`${id}-reqs-help`" />
          <small :id="`${id}-reqs-help`" class="muted">{{ t('cycles.add.regressionHelp') }}</small>
        </div>
        <div class="row">
          <Checkbox v-model="add.related" binary :input-id="`${id}-rel`" />
          <label :for="`${id}-rel`">{{ t('cycles.add.includeRelated') }}</label>
        </div>
      </template>
      <div v-else class="field">
        <label :for="`${id}-copy`">{{ t('cycles.add.copyFrom') }}</label>
        <Select v-model="add.copy" :input-id="`${id}-copy`" :options="copyOptions" option-label="label" option-value="value" filter :placeholder="t('cycles.add.chooseCycle')" />
        <small class="muted">{{ t('cycles.add.copyHelp') }}</small>
      </div>
      <div v-if="add.mode !== 'copy'" class="form-grid">
        <div class="field">
          <label :for="`${id}-tester`">{{ t('cycles.plan.tester') }}</label>
          <Select v-model="add.assignee" :input-id="`${id}-tester`" :options="testerOptions" option-label="label" option-value="value" filter />
        </div>
        <div class="field">
          <label :for="`${id}-est`">{{ t('cycles.plan.estimateMin') }}</label>
          <InputNumber v-model="add.estimate" :input-id="`${id}-est`" :min="0" :max="100000" :placeholder="t('cycles.add.defaultEstimate')" :aria-describedby="`${id}-est-help`" fluid />
          <small :id="`${id}-est-help`" class="muted">{{ t('cycles.add.estimateHelp') }}</small>
        </div>
        <div v-if="cycleConfigOptions.length" class="field" style="grid-column: 1 / -1">
          <label :for="`${id}-acfg`">{{ t('cycles.plan.configuration') }}</label>
          <MultiSelect v-model="add.configs" :input-id="`${id}-acfg`" :options="cycleConfigOptions" option-label="label" option-value="value" display="chip" :placeholder="t('cycles.add.allConfigurations')" />
        </div>
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" severity="secondary" text @click="addDialog = false" />
        <Button type="submit" :label="t('cycles.add.submit')" icon="pi pi-check" :loading="busy" :disabled="!addReady" />
      </div>
    </form>
  </Dialog>
  <ItemPickerDialog v-model:visible="picker" kind="test" :project-id="pid" :exclude="plan.map((r) => r.test_case_id)" :confirm-label="t('cycles.add.usePicked')" @pick="(items) => (add.tests = items.map((x) => ({ id: x.id, key: x.key, title: x.title })))" />
  <FolderPickerDialog v-model:visible="folderPicker" :folders="proj.folders.test" :header="t('cycles.add.chooseFolder')" @pick="(f) => (add.folder = f)" />

  <!-- edit -->
  <Dialog v-model:visible="editDialog" modal :header="t('cycles.editTitle')" style="width: min(680px, 96vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="saveEdit">
      <Message v-if="editError" severity="error" :closable="false">{{ editError }}</Message>
      <div class="field">
        <label :for="`${id}-ename`">{{ t('common.name') }}</label>
        <InputText :id="`${id}-ename`" v-model="edit.name" maxlength="200" required />
      </div>
      <div class="field">
        <label :for="`${id}-egoal`">{{ t('cycles.goal') }}</label>
        <InputText :id="`${id}-egoal`" v-model="edit.goal" maxlength="1000" />
      </div>
      <div class="form-grid">
        <div class="field">
          <label :for="`${id}-eowner`">{{ t('cycles.owner') }}</label>
          <Select v-model="edit.owner_id" :input-id="`${id}-eowner`" :options="testerOptions.slice(1)" option-label="label" option-value="value" filter />
        </div>
        <div class="field">
          <label :for="`${id}-eenv`">{{ t('cycles.environment') }}</label>
          <InputText :id="`${id}-eenv`" v-model="edit.environment" maxlength="200" />
        </div>
        <div class="field">
          <label :for="`${id}-estart`">{{ t('cycles.startDate') }}</label>
          <InputText :id="`${id}-estart`" v-model="edit.start_date" type="date" />
        </div>
        <div class="field">
          <label :for="`${id}-eend`">{{ t('cycles.endDate') }}</label>
          <InputText :id="`${id}-eend`" v-model="edit.end_date" type="date" />
        </div>
        <div class="field">
          <label :for="`${id}-erel`">{{ t('cycles.release') }}</label>
          <Select v-model="edit.release_id" :input-id="`${id}-erel`" :options="releaseOptions" option-label="label" option-value="value" show-clear :placeholder="t('cycles.none')" />
        </div>
        <div class="field">
          <label :id="`${id}-evar-label`" :for="`${id}-evar`">{{ t('cycles.variant') }}</label>
          <VariantSelect v-model="edit.variant_id" :input-id="`${id}-evar`" :labelledby="`${id}-evar-label`" :variants="vs.variants.value" :placeholder="t('cycles.none')" />
        </div>
      </div>
      <div class="field">
        <label :for="`${id}-ecfgs`">{{ t('cycles.configurations') }}</label>
        <MultiSelect v-model="edit.configuration_ids" :input-id="`${id}-ecfgs`" :options="cfg.options.value" option-label="label" option-value="value" display="chip" filter :placeholder="t('cycles.noConfigurations')" />
      </div>
      <div v-if="cycle?.status === 'planned'" class="field">
        <label :for="`${id}-esched`">{{ t('cycles.scheduledStart') }}</label>
        <InputText :id="`${id}-esched`" v-model="edit.scheduled_start" type="datetime-local" />
      </div>
      <div class="field">
        <label :for="`${id}-edesc`">{{ t('common.description') }}</label>
        <Textarea :id="`${id}-edesc`" v-model="edit.description" rows="3" auto-resize maxlength="20000" />
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" severity="secondary" text @click="editDialog = false" />
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="busy" :disabled="!edit.name.trim()" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.kpis {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
  gap: 10px;
  margin-top: 8px;
}
.kpi {
  padding: 10px 14px;
}
.kpi-value {
  font-size: 22px;
  font-weight: 700;
}
.empty-state {
  text-align: center;
  padding: 24px;
}
.board {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 12px;
  align-items: start;
}
.board-col {
  padding: 10px 12px;
}
.board-head {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-size: 15px;
  margin: 0 0 4px;
}
.board-list {
  margin-top: 8px;
  display: grid;
  gap: 6px;
}
.board-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border: 1px solid var(--qtf-border);
  border-radius: 6px;
  background: var(--qtf-bg);
}
.board-done {
  margin-top: 8px;
}
.charts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(420px, 100%), 1fr));
  gap: 12px;
}
.charts > .card {
  padding: 12px 14px;
}
.chart-box {
  height: 300px;
  margin-top: 8px;
}
.chart-fill {
  height: 100%;
}
.load-table {
  width: 100%;
  border-collapse: collapse;
  margin-top: 10px;
}
.load-table th,
.load-table td {
  text-align: start;
  padding: 4px 8px;
  border-bottom: 1px solid var(--qtf-border);
}
.wrap-buttons :deep(.p-togglebutton) {
  flex: 0 0 auto;
}
.wrap-buttons {
  flex-wrap: wrap;
}
</style>
