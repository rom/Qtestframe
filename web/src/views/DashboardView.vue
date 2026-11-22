<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Chart from 'primevue/chart'
import Skeleton from 'primevue/skeleton'
import Button from 'primevue/button'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import SelectButton from 'primevue/selectbutton'
import Select from 'primevue/select'
import SummaryBar from '@/components/SummaryBar.vue'
import QualityMetrics from '@/components/QualityMetrics.vue'
import ConsistencySection from '@/components/ConsistencySection.vue'
import StatusTag from '@/components/StatusTag.vue'
import { api, asItems } from '@/lib/api'
import { COVERAGE_COLORS, RESULT_COLORS, colorFor } from '@/lib/colors'
import { seriesColor } from '@/themes/chart'
import { useBreakpoint } from '@/lib/breakpoint'
import { entriesByCount } from '@/lib/collections'
import { formatDate, formatNumber, percent, relativeTime } from '@/lib/format'
import { useI18n } from 'vue-i18n'
import { enumLabel, type EnumGroup } from '@/i18n/labels'
import { RESULT_ORDER, summarize } from '@/lib/summary'
import {
  TREND_RANGES,
  burnupSeries,
  countValues,
  describeMulti,
  groupCoverageChart,
  runProgress,
  summarizeSeries,
  trendSeries,
  weekSeries,
  type Burnup,
  type ChartData,
  type FlakyTest,
  type GroupCoverage,
  type TrendRange,
  type Trends,
} from '@/lib/metrics'
import type { Dashboard, Release } from '@/lib/types'
import {
  LEVELS_DESC,
  MITIGATION_ORDER,
  REVIEW_ORDER,
  RISK_LEVEL_COLORS,
  activityChart,
  anyData,
  countChart,
  defectAgeChart,
  heatRows,
  share,
  testTypeChart,
  type DashboardInsights,
  type MiniHeatCell,
} from '@/lib/insights'
import { P } from '@/lib/permissions'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'
import { useMetaStore } from '@/stores/meta'
import { useChartTheme } from '@/composables/chartTheme'
import { useNotify } from '@/composables/notify'

const proj = useProjectStore()
const meta = useMetaStore()
const theme = useChartTheme()
const narrow = useBreakpoint('phone')
const notify = useNotify()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const data = ref<Dashboard | null>(null)
const loading = ref(true)

// ---------- sections (tabs, kept in ?tab=) ----------
type Section = 'overview' | 'trends' | 'quality'
const SECTIONS: Section[] = ['overview', 'trends', 'quality']
const tabsId = 'dash-tabs'
const panel = (value: string) => ({ role: 'tabpanel', id: `${tabsId}_tabpanel_${value}`, 'aria-labelledby': `${tabsId}_tab_${value}` })
const initial = route.query.tab as Section
const section = ref<Section>(SECTIONS.includes(initial) ? initial : 'overview')
watch(section, (v) => {
  if ((route.query.tab ?? 'overview') !== v) router.replace({ query: { ...route.query, tab: v === 'overview' ? undefined : v } })
  if (v === 'trends') loadTrends()
})

// further key figures, loaded next to the dashboard (GET /dashboard/insights)
const insights = ref<DashboardInsights | null>(null)
async function loadInsights() {
  try {
    insights.value = await api.get<DashboardInsights>(`/projects/${proj.pid}/dashboard/insights`)
  } catch (e) {
    notify.fail(e, t('dashboard.insightsFailed'))
  }
}

async function load() {
  loading.value = true
  void loadInsights()
  try {
    data.value = await api.get<Dashboard>(`/projects/${proj.pid}/dashboard`)
  } catch (e) {
    notify.fail(e, t('dashboard.loadFailed'))
  } finally {
    loading.value = false
  }
  if (section.value === 'trends') loadTrends(true)
}
onMounted(load)

const reqTotal = computed(() => data.value?.requirements.total ?? 0)
const coveredPct = computed(() => percent(data.value?.requirements.covered ?? 0, reqTotal.value))
const results = computed(() => summarize(data.value?.results))

const kpis = computed(() => {
  const d = data.value
  return [
    { label: t('dashboard.kpiRequirements'), value: formatNumber(reqTotal.value), sub: t('dashboard.kpiUncovered', { count: d?.requirements.uncovered ?? 0 }), icon: 'pi pi-list-check', color: '#6366f1' },
    { label: t('dashboard.kpiCoverage'), value: formatNumber(coveredPct.value / 100, undefined, { style: 'percent' }), sub: t('dashboard.kpiCovered', { count: d?.requirements.covered ?? 0 }), icon: 'pi pi-shield', color: '#22c55e' },
    {
      label: t('dashboard.kpiTests'),
      value: formatNumber(d?.tests.total ?? 0),
      sub: t('dashboard.kpiTestsSplit', { manual: d?.tests.by_execution_type?.manual ?? 0, automated: d?.tests.by_execution_type?.automated ?? 0 }),
      icon: 'pi pi-check-square',
      color: '#0ea5e9',
    },
    { label: t('dashboard.kpiRuns'), value: formatNumber(d?.runs.total ?? 0), sub: t('dashboard.kpiInProgress', { count: d?.runs.by_status?.in_progress ?? 0 }), icon: 'pi pi-play-circle', color: '#f59e0b' },
    { label: t('dashboard.kpiPassRate'), value: formatNumber(results.value.passRate / 100, undefined, { style: 'percent' }), sub: t('dashboard.kpiExecutions', results.value.executed), icon: 'pi pi-chart-line', color: '#ec4899' },
    ...insightKpis.value,
  ]
})

/** Key figures from the insights (shown once they are loaded). */
const insightKpis = computed(() => {
  const x = insights.value
  if (!x) return []
  const pct = (n: number, total: number) => formatNumber(share(n, total) / 100, undefined, { style: 'percent' })
  const rel = x.release
  return [
    { label: t('dashboard.kpiVerified'), value: pct(x.requirements.passing, x.requirements.total), sub: t('dashboard.kpiVerifiedSub', { count: x.requirements.passing }, x.requirements.passing), icon: 'pi pi-verified', color: '#16a34a' },
    {
      label: t('dashboard.kpiOpenRisks'),
      value: formatNumber(x.risks.open),
      sub: t('dashboard.kpiOpenRisksSub', { critical: x.risks.by_level.critical ?? 0, high: x.risks.by_level.high ?? 0 }),
      icon: 'pi pi-exclamation-triangle',
      color: '#dc2626',
    },
    {
      label: t('dashboard.kpiOpenDefects'),
      value: formatNumber(x.defects.open),
      sub: x.defects.open ? t('dashboard.kpiDefectAge', { days: formatNumber(x.defects.average_days) }) : t('dashboard.kpiNoOpenDefects'),
      icon: 'pi pi-bug',
      color: '#ea580c',
    },
    { label: t('dashboard.kpiSuspect'), value: formatNumber(x.suspect.links), sub: t('dashboard.kpiSuspectSub', { count: x.suspect.requirements }, x.suspect.requirements), icon: 'pi pi-link', color: '#d97706' },
    {
      label: t('dashboard.kpiRuleViolations'),
      value: formatNumber(x.trace_rules.violations),
      sub: x.trace_rules.rules ? t('dashboard.kpiRulesSub', { failed: x.trace_rules.failed, rules: x.trace_rules.rules }) : t('dashboard.kpiNoRules'),
      icon: 'pi pi-sitemap',
      color: '#8b5cf6',
    },
    {
      label: t('dashboard.kpiRelease'),
      value: rel ? pct(rel.executed, rel.total) : '–',
      sub: rel ? t('dashboard.kpiReleaseSub', { name: rel.name, executed: rel.executed, total: rel.total }) : t('dashboard.kpiNoRelease'),
      icon: 'pi pi-flag',
      color: '#0ea5e9',
    },
  ]
})

// ---------- chart options ----------
function font() {
  return { family: theme.value.font }
}
function legendOptions(show: boolean) {
  return { display: show, position: 'bottom', labels: { color: theme.value.text, usePointStyle: true, boxWidth: 8, font: font() } }
}
function axis(extra: Record<string, unknown> = {}) {
  const c = theme.value
  return { ticks: { color: c.text, precision: 0, font: font() }, grid: { color: c.grid }, border: { color: c.grid }, ...extra }
}
function baseOptions(legend = false, indexAxis: 'x' | 'y' = 'x', stacked = false) {
  return {
    maintainAspectRatio: false,
    indexAxis,
    plugins: { legend: legendOptions(legend), tooltip: { titleFont: font(), bodyFont: font() } },
    scales: { x: axis({ stacked }), y: axis({ stacked, beginAtZero: true }) },
  }
}
const doughnutOptions = computed(() => ({
  maintainAspectRatio: false,
  cutout: '62%',
  plugins: {
    legend: { position: narrow.value ? 'bottom' : 'right', labels: { color: theme.value.text, usePointStyle: true, boxWidth: 8, font: font() } },
    tooltip: { titleFont: font(), bodyFont: font() },
  },
}))
/** Lines over days: percentages (0–100) or counts. */
function lineOptions(pct: boolean) {
  const c = theme.value
  return {
    maintainAspectRatio: false,
    spanGaps: true,
    interaction: { mode: 'index', intersect: false },
    plugins: { legend: legendOptions(true), tooltip: { titleFont: font(), bodyFont: font() } },
    scales: {
      x: { ticks: { color: c.text, font: font(), maxRotation: 0, autoSkip: true, maxTicksLimit: narrow.value ? 4 : 8 }, grid: { display: false }, border: { color: c.grid } },
      y: pct
        ? { min: 0, max: 100, ticks: { color: c.text, font: font(), callback: (v: number) => `${v} %` }, grid: { color: c.grid }, border: { color: c.grid } }
        : axis({ beginAtZero: true }),
    },
  }
}

/** Bars: categorical colours of the active theme. */
function bar(group: EnumGroup, rec: Record<string, number> | undefined, order?: readonly string[]) {
  const entries = entriesByCount(rec, order)
  return {
    labels: entries.map(([k]) => (group === 'obligation' ? k.replace(/_/g, ' ').toUpperCase() : enumLabel(group, k))),
    datasets: [{ data: entries.map(([, v]) => v), backgroundColor: entries.map((_, i) => seriesColor(theme.value, i)), borderRadius: 6, maxBarThickness: 34 }],
  }
}
/** Doughnuts show results and coverage: their colours carry meaning (the legend names each). */
function doughnut(group: EnumGroup, rec: Record<string, number> | undefined, colors: Record<string, string>, order?: readonly string[]) {
  const entries = entriesByCount(rec, order).filter(([, v]) => v > 0)
  return {
    labels: entries.map(([k]) => enumLabel(group, k)),
    datasets: [{ data: entries.map(([, v]) => v), backgroundColor: entries.map(([k], i) => colorFor(colors, k, i)), borderWidth: 2, borderColor: theme.value.surface }],
  }
}
/** Text alternative of a chart for screen readers. */
function describe(title: string, c: { labels: string[]; datasets: { data: (number | null)[] }[] }) {
  const parts = c.labels.map((l, i) => `${l}: ${formatNumber(c.datasets[0].data[i] ?? 0)}`)
  return parts.length ? `${title}. ${parts.join(', ')}` : title
}
const fmtPct = (n: number) => `${formatNumber(n)} %`

const coverageChart = computed(() => doughnut('coverage', data.value?.coverage, COVERAGE_COLORS, ['passed', 'failed', 'partial', 'not_run', 'none']))
const statusChart = computed(() => bar('status', data.value?.requirements.by_status, meta.meta.requirement_statuses))
const obligationChart = computed(() => bar('obligation', data.value?.requirements.by_obligation, meta.meta.obligations))
const resultsChart = computed(() => doughnut('result', data.value?.results, RESULT_COLORS, RESULT_ORDER))
const testStatusChart = computed(() => bar('status', data.value?.tests.by_status, meta.meta.test_statuses))
const hasData = (c: { datasets: { data: number[] }[] }) => c.datasets[0].data.some((v) => v > 0)
const hasAny = (c: ChartData) => c.datasets.some((d) => d.data.some((v) => (v ?? 0) > 0))

// ---------- overview: active runs ----------
const activeRuns = computed(() => data.value?.active_runs ?? [])
const progressChart = computed(() => {
  const c = runProgress(activeRuns.value, (s) => enumLabel('result', s))
  c.datasets = c.datasets.map((d) => ({ ...d, borderRadius: 4, maxBarThickness: 26 }))
  return c
})
const progressHeight = computed(() => `${Math.max(160, 60 + activeRuns.value.length * 38)}px`)

// ---------- insights: charts ----------
const auth = useAuthStore()
const canReadRisks = computed(() => auth.can(P.requirementRead))
const testTypeData = computed(() => {
  const c = testTypeChart(insights.value?.tests.by_type, (ty) => enumLabel('executionType', ty), (s) => enumLabel('result', s))
  c.datasets = c.datasets.map((d) => ({ ...d, borderRadius: 4, maxBarThickness: 34 }))
  return c
})
const reviewChart = computed(() => {
  const c = countChart(insights.value?.requirements.review, REVIEW_ORDER, (k) => t(`dashboard.review.${k}`), (_, i) => seriesColor(theme.value, i))
  c.datasets = c.datasets.map((d) => ({ ...d, borderRadius: 6, maxBarThickness: 34 }))
  return c
})
const riskLevelChart = computed(() => {
  const c = countChart(insights.value?.risks.by_level, LEVELS_DESC, (k) => t(`risksView.level.${k}`), (k) => RISK_LEVEL_COLORS[k])
  c.datasets = c.datasets.map((d) => ({ ...d, borderRadius: 6, maxBarThickness: 34 }))
  return c
})
const mitigationChart = computed(() => {
  const c = countChart(insights.value?.risks.mitigation, MITIGATION_ORDER, (k) => t(`dashboard.mitigation.${k}`), (_, i) => seriesColor(theme.value, i))
  c.datasets = c.datasets.map((d) => ({ ...d, borderRadius: 6, maxBarThickness: 26 }))
  return c
})
const defectAgeData = computed(() => {
  const c = defectAgeChart(insights.value?.defects, (s) => t(`defectsView.severity.${s}`), (b) => t(`dashboard.ageBucket.${b}`))
  c.datasets = c.datasets.map((d) => ({ ...d, borderRadius: 4, maxBarThickness: 34 }))
  return c
})
const ageSummary = computed(() => {
  const d = insights.value?.defects
  return d?.open ? t('dashboard.defectAgeSummary', { avg: formatNumber(d.average_days), oldest: formatNumber(d.oldest_days) }) : ''
})
const heat = computed(() => heatRows(insights.value?.risks.heat))
const heatCellLabel = (c: MiniHeatCell) =>
  t('dashboard.heatCell', { likelihood: c.likelihood, impact: c.impact, level: t(`risksView.level.${c.level}`), count: t('risksView.riskCount', { count: c.count }, c.count) })
const heatTotal = computed(() => heat.value.reduce((n, r) => n + r.cells.reduce((m, c) => m + c.count, 0), 0))
const ruleRows = computed(() => {
  const r = insights.value?.trace_rules
  if (!r) return []
  return (['rules', 'errors', 'warnings', 'violations'] as const).map((k) => ({ key: k, label: t(`dashboard.ruleCounts.${k}`), value: r[k] }))
})
const activityData = computed(() => {
  const c = activityChart(insights.value?.activity.days, formatDate, (s) => t(`dashboard.activitySeries.${s}`), (i) => seriesColor(theme.value, i))
  c.datasets = c.datasets.map((d) => ({ ...d, borderRadius: 3, maxBarThickness: 14 }))
  return c
})
const activityDescription = computed(() => {
  const tot = insights.value?.activity.total
  if (!tot) return t('dashboard.activity')
  return `${t('dashboard.activity')}. ${t('dashboard.activityTotal', { changes: tot.changes, executions: tot.executions, defects: tot.defects })}. ${describeMulti('', activityData.value, formatNumber, 30).replace(/^\.\s*/, '')}`
})
const activityOptions = computed(() => {
  const c = theme.value
  return {
    ...baseOptions(true, 'x', true),
    scales: {
      x: { stacked: true, ticks: { color: c.text, font: font(), maxRotation: 0, autoSkip: true, maxTicksLimit: narrow.value ? 4 : 8 }, grid: { display: false }, border: { color: c.grid } },
      y: axis({ stacked: true, beginAtZero: true }),
    },
  }
})
const releaseSummary = computed(() => {
  const r = insights.value?.release
  return r ? { ...r.executions, total: r.total } : null
})

// ---------- quality ----------
const SEVERITY_COLORS: Record<string, string> = { critical: '#dc2626', major: '#ea580c', minor: '#d97706', trivial: '#64748b' }
const SEVERITY_ORDER = ['critical', 'major', 'minor', 'trivial']
const severityChart = computed(() => {
  const rec = data.value?.defects_by_severity ?? {}
  const keys = [...SEVERITY_ORDER, ...Object.keys(rec).filter((k) => !SEVERITY_ORDER.includes(k))]
  return {
    labels: keys.map((k) => t(`defectsView.severity.${k}`)),
    datasets: [{ data: keys.map((k) => rec[k] ?? 0), backgroundColor: keys.map((k, i) => colorFor(SEVERITY_COLORS, k, i)), borderRadius: 6, maxBarThickness: 34 }],
  }
})

type GroupBy = 'folder' | 'category'
const groupBy = ref<GroupBy>('folder')
const groupOptions = computed(() => [
  { value: 'folder', label: t('dashboard.byFolder') },
  { value: 'category', label: t('dashboard.byCategory') },
])
const groups = computed<GroupCoverage[]>(() => (groupBy.value === 'folder' ? data.value?.coverage_by_folder : data.value?.coverage_by_category) ?? [])
const groupName = (g: GroupCoverage) => (groupBy.value === 'folder' ? g.name || t('dashboard.noFolder') : g.dimension ? `${g.dimension}: ${g.name}` : g.name)
const groupChart = computed(() => {
  const c = groupCoverageChart(groups.value, groupName, (s) => enumLabel('coverage', s))
  c.datasets = c.datasets.map((d) => ({ ...d, borderRadius: 4, maxBarThickness: 24 }))
  return c
})
const groupHeight = computed(() => `${Math.max(200, 60 + groups.value.length * 32)}px`)

const flaky = computed<FlakyTest[]>(() => data.value?.flaky?.items ?? [])
const resultLabel = (s: string) => enumLabel('result', s)
const sequenceText = (f: FlakyTest) => t('dashboard.flakySequence', { seq: f.results.map((r) => resultLabel(r.status)).join(', ') })

// ---------- trends ----------
const range = ref<TrendRange>(30)
const rangeOptions = computed(() => TREND_RANGES.map((n) => ({ value: n, label: t('dashboard.rangeDays', { n }) })))
const trends = ref<Trends | null>(null)
const trendsLoading = ref(false)
let trendsFor = ''

async function loadTrends(force = false) {
  const key = `${proj.pid}:${range.value}`
  if (!force && trendsFor === key) return
  trendsFor = key
  trendsLoading.value = true
  try {
    trends.value = await api.get<Trends>(`/projects/${proj.pid}/metrics/trends?range=${range.value}`)
  } catch (e) {
    trendsFor = ''
    notify.fail(e, t('dashboard.trendLoadFailed'))
  } finally {
    trendsLoading.value = false
  }
  loadReleases()
}
watch(range, () => loadTrends())

const trend = computed(() => trendSeries(trends.value))
const trendChart = computed<ChartData>(() => {
  const s = trend.value
  const c1 = '#16a34a'
  const c2 = seriesColor(theme.value, 0)
  return {
    labels: s.days.map(formatDate),
    datasets: [
      { label: t('dashboard.seriesCoverage'), data: s.coverage, borderColor: c1, backgroundColor: c1, pointStyle: 'circle', pointRadius: s.days.length > 60 ? 0 : 2, borderWidth: 2, tension: 0.2 },
      { label: t('dashboard.seriesPassRate'), data: s.passRate, borderColor: c2, backgroundColor: c2, pointStyle: 'rectRot', pointRadius: s.days.length > 60 ? 0 : 3, borderWidth: 2, borderDash: [6, 4], tension: 0.2 },
    ],
  }
})
const trendDays = computed(() => countValues(trend.value.coverage))
const trendDescription = computed(() => {
  const parts = trendChart.value.datasets
    .map((d) => {
      const s = summarizeSeries(d.data)
      return s && t('dashboard.trendSummary', { series: d.label, first: fmtPct(s.first), last: fmtPct(s.last), min: fmtPct(s.min), max: fmtPct(s.max) })
    })
    .filter(Boolean)
  return [t('dashboard.trendTitle'), t('dashboard.trendDays', trendDays.value), ...parts].join('. ')
})

const weeks = computed(() => weekSeries(trends.value?.defect_weeks))
const weekChart = computed<ChartData>(() => ({
  labels: weeks.value.weeks.map(formatDate),
  datasets: [
    { label: t('dashboard.seriesOpened'), data: weeks.value.opened, backgroundColor: '#ef4444', borderRadius: 4, maxBarThickness: 18 },
    { label: t('dashboard.seriesClosed'), data: weeks.value.closed, backgroundColor: '#22c55e', borderRadius: 4, maxBarThickness: 18 },
  ],
}))

// release burn-up
const releases = ref<Release[]>([])
const releaseId = ref<string | null>(null)
const burnup = ref<Burnup | null>(null)
let releasesLoaded = false
async function loadReleases() {
  if (releasesLoaded) return
  releasesLoaded = true
  try {
    releases.value = asItems(await api.get<Release[]>(`/projects/${proj.pid}/releases`))
  } catch {
    releases.value = []
  }
  const pick = releases.value.find((r) => r.status === 'active') ?? releases.value.find((r) => r.status === 'planned') ?? releases.value[0]
  releaseId.value = pick?.id ?? null
}
watch(releaseId, async (id) => {
  burnup.value = null
  if (!id) return
  try {
    burnup.value = await api.get<Burnup>(`/releases/${id}/burnup`)
  } catch (e) {
    notify.fail(e, t('dashboard.burnupLoadFailed'))
  }
})
const releaseOptions = computed(() => releases.value.map((r) => ({ value: r.id, label: r.version && r.version !== r.name ? `${r.name} (${r.version})` : r.name })))
const burnupChart = computed<ChartData>(() => {
  const b = burnupSeries(burnup.value)
  const scope = theme.value.text
  return {
    labels: b.days.map(formatDate),
    datasets: [
      { label: t('dashboard.seriesScope'), data: b.scope, borderColor: scope, backgroundColor: scope, borderDash: [4, 4], stepped: true, pointRadius: 0, borderWidth: 2 },
      { label: t('dashboard.seriesCovered'), data: b.covered, borderColor: seriesColor(theme.value, 0), backgroundColor: seriesColor(theme.value, 0), pointStyle: 'rectRot', pointRadius: 0, borderWidth: 2 },
      { label: t('dashboard.seriesPassed'), data: b.passed, borderColor: '#16a34a', backgroundColor: 'rgba(34, 197, 94, 0.18)', fill: 'origin', pointRadius: 0, borderWidth: 2 },
    ],
  }
})
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('dashboard.title') }}</h1>
      <div class="sub">{{ proj.project?.description }}</div>
    </div>
    <div class="actions"><Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="load" /></div>
  </div>

  <Tabs :id="tabsId" v-model:value="section" class="editor-tabs dash-tabs" scrollable>
    <TabList :aria-label="t('dashboard.sections')">
      <Tab value="overview"><i class="pi pi-th-large" aria-hidden="true" /> {{ t('dashboard.tabOverview') }}</Tab>
      <Tab value="trends"><i class="pi pi-chart-line" aria-hidden="true" /> {{ t('dashboard.tabTrends') }}</Tab>
      <Tab value="quality"><i class="pi pi-verified" aria-hidden="true" /> {{ t('dashboard.tabQuality') }}</Tab>
    </TabList>
  </Tabs>

  <div v-if="loading && !data" class="grid" aria-busy="true" :aria-label="t('common.loading')">
    <div class="kpis"><Skeleton v-for="i in 5" :key="i" height="78px" /></div>
    <div class="charts"><Skeleton v-for="i in 4" :key="i" height="280px" /></div>
  </div>

  <!-- overview -->
  <div v-else-if="data && section === 'overview'" class="grid" :aria-busy="loading" v-bind="panel('overview')">
    <ul class="kpis plain-list" :aria-label="t('dashboard.kpis')">
      <li v-for="k in kpis" :key="k.label" class="card kpi">
        <div class="icon" aria-hidden="true" :style="{ background: k.color + '1f', color: k.color }"><i :class="k.icon" /></div>
        <div class="kpi-text">
          <div class="value">{{ k.value }}</div>
          <div class="label">{{ k.label }}</div>
          <div class="small muted">{{ k.sub }}</div>
        </div>
      </li>
    </ul>

    <div class="charts">
      <section class="card" aria-labelledby="ch-coverage">
        <h2 id="ch-coverage">{{ t('dashboard.requirementCoverage') }}</h2>
        <div class="chart-box">
          <div v-if="hasData(coverageChart)" role="img" :aria-label="describe(t('dashboard.requirementCoverage'), coverageChart)" class="chart-fill">
            <Chart type="doughnut" :data="coverageChart" :options="doughnutOptions" style="height: 100%" />
          </div>
          <div v-else class="empty">{{ t('dashboard.noRequirements') }}</div>
        </div>
      </section>
      <section class="card" aria-labelledby="ch-results">
        <h2 id="ch-results">{{ t('dashboard.executionResults') }}</h2>
        <div class="chart-box">
          <div v-if="hasData(resultsChart)" role="img" :aria-label="describe(t('dashboard.executionResults'), resultsChart)" class="chart-fill">
            <Chart type="doughnut" :data="resultsChart" :options="doughnutOptions" style="height: 100%" />
          </div>
          <div v-else class="empty">{{ t('dashboard.noExecutions') }}</div>
        </div>
      </section>
      <section class="card" aria-labelledby="ch-active">
        <div class="row" style="justify-content: space-between; margin-bottom: 8px">
          <h2 id="ch-active" style="margin: 0">{{ t('dashboard.activeRuns') }}</h2>
          <RouterLink :to="{ name: 'runs', params: { projectId: proj.pid } }" class="small">{{ t('dashboard.allRuns') }}</RouterLink>
        </div>
        <div v-if="hasAny(progressChart)" class="chart-box" :style="{ height: progressHeight }" role="img" :aria-label="describeMulti(t('dashboard.activeRuns'), progressChart, formatNumber)">
          <Chart type="bar" :data="progressChart" :options="baseOptions(true, 'y', true)" style="height: 100%" />
        </div>
        <div v-else class="empty">{{ t('dashboard.noActiveRuns') }}</div>
      </section>
      <section class="card" aria-labelledby="ch-runs">
        <div class="row" style="justify-content: space-between; margin-bottom: 8px">
          <h2 id="ch-runs" style="margin: 0">{{ t('dashboard.recentRuns') }}</h2>
          <RouterLink :to="{ name: 'runs', params: { projectId: proj.pid } }" class="small">{{ t('dashboard.allRuns') }}</RouterLink>
        </div>
        <div v-if="!(data.recent_runs ?? []).length" class="empty">{{ t('dashboard.noRuns') }}</div>
        <RouterLink
          v-for="r in data.recent_runs ?? []"
          :key="r.id"
          :to="{ name: 'run', params: { projectId: proj.pid, runId: r.id } }"
          class="recent-run"
        >
          <div class="row nowrap" style="justify-content: space-between">
            <span class="ellipsis" style="font-weight: 600">{{ r.name }}</span>
            <StatusTag :value="r.status" />
          </div>
          <div class="row nowrap" style="margin-top: 4px">
            <SummaryBar :summary="r.summary" class="grow" />
            <span class="small muted">{{ relativeTime(r.created_at) }}</span>
          </div>
        </RouterLink>
      </section>
      <section class="card" aria-labelledby="ch-status">
        <h2 id="ch-status">{{ t('dashboard.requirementsByStatus') }}</h2>
        <div class="chart-box" role="img" :aria-label="describe(t('dashboard.requirementsByStatus'), statusChart)">
          <Chart type="bar" :data="statusChart" :options="baseOptions()" style="height: 100%" />
        </div>
      </section>
      <section class="card" aria-labelledby="ch-obligation">
        <h2 id="ch-obligation">{{ t('dashboard.requirementsByObligation') }}</h2>
        <div class="chart-box" role="img" :aria-label="describe(t('dashboard.requirementsByObligation'), obligationChart)">
          <Chart type="bar" :data="obligationChart" :options="baseOptions(false, 'y')" style="height: 100%" />
        </div>
      </section>
      <section class="card" aria-labelledby="ch-tests">
        <h2 id="ch-tests">{{ t('dashboard.testsByStatus') }}</h2>
        <div class="chart-box" role="img" :aria-label="describe(t('dashboard.testsByStatus'), testStatusChart)">
          <Chart type="bar" :data="testStatusChart" :options="baseOptions()" style="height: 100%" />
        </div>
      </section>
      <template v-if="insights">
        <section class="card" aria-labelledby="ch-types">
          <h2 id="ch-types">{{ t('dashboard.testTypes') }}</h2>
          <div class="chart-box">
            <div v-if="anyData(testTypeData)" role="img" :aria-label="describeMulti(t('dashboard.testTypes'), testTypeData, formatNumber)" class="chart-fill">
              <Chart type="bar" :data="testTypeData" :options="baseOptions(true, 'x', true)" style="height: 100%" />
            </div>
            <div v-else class="empty">{{ t('dashboard.noTests') }}</div>
          </div>
        </section>
        <section class="card" aria-labelledby="ch-review">
          <h2 id="ch-review">{{ t('dashboard.reviewState') }}</h2>
          <div class="chart-box">
            <div v-if="anyData(reviewChart)" role="img" :aria-label="describe(t('dashboard.reviewState'), reviewChart)" class="chart-fill">
              <Chart type="bar" :data="reviewChart" :options="baseOptions(false, 'y')" style="height: 100%" />
            </div>
            <div v-else class="empty">{{ t('dashboard.noRequirements') }}</div>
          </div>
        </section>
        <section class="card" aria-labelledby="ch-release">
          <h2 id="ch-release">{{ t('dashboard.releaseProgress') }}</h2>
          <div v-if="!insights.release" class="empty">{{ t('dashboard.noActiveRelease') }}</div>
          <template v-else>
            <div class="row" style="justify-content: space-between">
              <RouterLink :to="{ name: 'releases', params: { projectId: proj.pid }, query: { r: insights.release.id } }" class="release-name">{{ insights.release.name }}</RouterLink>
              <span class="small muted">{{ t(`releasesView.status.${insights.release.status}`) }}</span>
            </div>
            <div v-if="insights.release.target_date" class="small muted">{{ t('dashboard.burnupTarget', { date: formatDate(insights.release.target_date) }) }}</div>
            <div v-if="!insights.release.total" class="empty">{{ t('dashboard.noReleaseRuns') }}</div>
            <template v-else>
              <SummaryBar :summary="releaseSummary" class="release-bar" />
              <p class="small">
                {{ t('dashboard.releaseSummary', { runs: insights.release.runs, executed: insights.release.executed, total: insights.release.total, passed: insights.release.passed }) }}
              </p>
            </template>
          </template>
        </section>
      </template>
    </div>
  </div>

  <!-- trends -->
  <div v-else-if="data && section === 'trends'" class="grid" :aria-busy="trendsLoading" v-bind="panel('trends')">
    <div class="row trend-toolbar">
      <span id="dash-range" class="small muted">{{ t('dashboard.trendRange') }}</span>
      <SelectButton v-model="range" :options="rangeOptions" option-label="label" option-value="value" :allow-empty="false" aria-labelledby="dash-range" />
    </div>
    <section class="card" aria-labelledby="ch-trend">
      <h2 id="ch-trend">{{ t('dashboard.trendTitle') }}</h2>
      <div class="chart-box tall">
        <Skeleton v-if="trendsLoading && !trends" height="100%" />
        <div v-else-if="trendDays > 0" role="img" :aria-label="trendDescription" class="chart-fill">
          <Chart type="line" :data="trendChart" :options="lineOptions(true)" style="height: 100%" />
        </div>
        <div v-else class="empty">{{ t('dashboard.noTrend') }}</div>
      </div>
    </section>
    <section v-if="insights" class="card" aria-labelledby="ch-activity">
      <h2 id="ch-activity">{{ t('dashboard.activity') }}</h2>
      <p class="small muted" style="margin-top: 0">{{ t('dashboard.activityTotal', { changes: insights.activity.total.changes, executions: insights.activity.total.executions, defects: insights.activity.total.defects }) }}</p>
      <div class="chart-box">
        <div v-if="anyData(activityData)" role="img" :aria-label="activityDescription" class="chart-fill">
          <Chart type="bar" :data="activityData" :options="activityOptions" style="height: 100%" />
        </div>
        <div v-else class="empty">{{ t('dashboard.noActivity') }}</div>
      </div>
    </section>
    <div class="charts">
      <section class="card" aria-labelledby="ch-weeks">
        <h2 id="ch-weeks">{{ t('dashboard.defectsPerWeek') }}</h2>
        <div class="chart-box">
          <div v-if="hasAny(weekChart)" role="img" :aria-label="describeMulti(t('dashboard.defectsPerWeek'), weekChart, formatNumber)" class="chart-fill">
            <Chart type="bar" :data="weekChart" :options="baseOptions(true)" style="height: 100%" />
          </div>
          <div v-else class="empty">{{ t('dashboard.noDefectsInPeriod') }}</div>
        </div>
      </section>
      <section class="card" aria-labelledby="ch-burnup">
        <div class="row" style="justify-content: space-between; margin-bottom: 8px">
          <h2 id="ch-burnup" style="margin: 0">{{ t('dashboard.burnup') }}</h2>
          <Select v-if="releases.length" v-model="releaseId" :options="releaseOptions" option-label="label" option-value="value" size="small" :aria-label="t('dashboard.burnupRelease')" class="release-select" />
        </div>
        <div v-if="burnup?.target_date" class="small muted">{{ t('dashboard.burnupTarget', { date: formatDate(burnup.target_date) }) }}</div>
        <div class="chart-box">
          <div v-if="!releases.length" class="empty">{{ t('dashboard.noReleases') }}</div>
          <Skeleton v-else-if="!burnup" height="100%" />
          <div v-else role="img" :aria-label="describeMulti(t('dashboard.burnup') + ' ' + burnup.name, burnupChart, formatNumber, 8)" class="chart-fill">
            <Chart type="line" :data="burnupChart" :options="lineOptions(false)" style="height: 100%" />
          </div>
        </div>
      </section>
    </div>
  </div>

  <!-- quality -->
  <div v-else-if="data && section === 'quality'" class="grid" v-bind="panel('quality')">
    <section v-if="data.correlations" class="card" aria-labelledby="ch-corr">
      <h2 id="ch-corr">{{ t('correlations.dashboardTitle') }}</h2>
      <p class="small muted" style="margin-top: 0">{{ t('correlations.dashboardHint') }}</p>
      <ul class="plain-list corr-counts">
        <li>
          <RouterLink :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { q: 'blocked>0' } }" class="blocked-badge">
            <i class="pi pi-ban" aria-hidden="true" />{{ t('correlations.dashBlocked', { n: data.correlations.blocked_requirements }, data.correlations.blocked_requirements) }}
          </RouterLink>
        </li>
        <li class="warned-badge"><i class="pi pi-exclamation-triangle" aria-hidden="true" />{{ t('correlations.dashWarned', { n: data.correlations.warned_requirements }, data.correlations.warned_requirements) }}</li>
        <li>
          <RouterLink :to="{ name: 'tests', params: { projectId: proj.pid }, query: { q: 'stale:true' } }">
            <i class="pi pi-clock" aria-hidden="true" /> {{ t('correlations.dashStale', { n: data.correlations.stale_results }, data.correlations.stale_results) }}
          </RouterLink>
        </li>
        <li>
          <RouterLink :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { q: 'suspect_relations>0' } }">
            <i class="pi pi-exclamation-triangle" aria-hidden="true" /> {{ t('correlations.dashSuspect', { n: data.correlations.suspect_relations }, data.correlations.suspect_relations) }}
          </RouterLink>
        </li>
      </ul>
    </section>
    <div class="card"><QualityMetrics :project-id="proj.pid" id-prefix="dq" /></div>
    <ConsistencySection :project-id="proj.pid" compact />
    <section class="card" aria-labelledby="ch-flaky">
      <h2 id="ch-flaky">{{ t('dashboard.flakyTests') }}</h2>
      <p class="small muted" style="margin-top: 0">{{ t('dashboard.flakyHint', { min: data.flaky?.min_flips ?? 2, window: data.flaky?.window ?? 10 }) }}</p>
      <div v-if="!flaky.length" class="empty">{{ t('dashboard.noFlaky') }}</div>
      <ol v-else class="plain-list flaky-list">
        <li v-for="f in flaky" :key="f.test_id" class="flaky-row">
          <div class="flaky-main">
            <RouterLink :to="{ name: 'tests', params: { projectId: proj.pid }, query: { item: f.test_id } }" class="flaky-link">
              <span class="key">{{ f.key }}</span> <span>{{ f.title }}</span>
            </RouterLink>
            <div class="small muted">
              <i :class="f.execution_type === 'automated' ? 'pi pi-bolt' : 'pi pi-user'" aria-hidden="true" /> {{ enumLabel('executionType', f.execution_type) }}
              <span v-if="f.last_flip"> · {{ t('dashboard.lastFlip', { when: relativeTime(f.last_flip) }) }}</span>
            </div>
          </div>
          <div class="flaky-seq" role="img" :aria-label="sequenceText(f)">
            <span v-for="r in f.results" :key="r.execution_id" class="dot" :class="[r.status, { flip: r.flip }]" :title="`${resultLabel(r.status)} · ${formatDate(r.at)}`">
              <i :class="r.status === 'passed' ? 'pi pi-check' : 'pi pi-times'" aria-hidden="true" />
            </span>
          </div>
          <span class="flaky-score">{{ t('dashboard.flips', f.score) }}</span>
        </li>
      </ol>
      <div v-if="(data.flaky?.total ?? 0) > flaky.length" class="small muted" style="margin-top: 8px">
        {{ t('dashboard.flakyMore', { shown: flaky.length, total: data.flaky?.total }) }}
        <RouterLink :to="{ name: 'tests', params: { projectId: proj.pid }, query: { q: 'flaky:true' } }">{{ t('dashboard.allFlaky') }}</RouterLink>
      </div>
    </section>
    <div class="charts">
      <section class="card" aria-labelledby="ch-severity">
        <h2 id="ch-severity">{{ t('dashboard.defectsBySeverity') }}</h2>
        <div class="chart-box">
          <div v-if="hasData(severityChart)" role="img" :aria-label="describe(t('dashboard.defectsBySeverity'), severityChart)" class="chart-fill">
            <Chart type="bar" :data="severityChart" :options="baseOptions()" style="height: 100%" />
          </div>
          <div v-else class="empty">{{ t('dashboard.noOpenDefects') }}</div>
        </div>
      </section>
      <section class="card" aria-labelledby="ch-groups">
        <div class="row" style="justify-content: space-between; margin-bottom: 8px">
          <h2 id="ch-groups" style="margin: 0">{{ t('dashboard.coverageByGroup') }}</h2>
          <SelectButton v-model="groupBy" :options="groupOptions" option-label="label" option-value="value" :allow-empty="false" size="small" :aria-label="t('dashboard.groupBy')" />
        </div>
        <div v-if="groups.length" class="chart-box" :style="{ height: groupHeight }" role="img" :aria-label="describeMulti(t('dashboard.coverageByGroup'), groupChart, formatNumber, 20)">
          <Chart type="bar" :data="groupChart" :options="baseOptions(true, 'y', true)" style="height: 100%" />
        </div>
        <div v-else class="empty">{{ t('dashboard.noRequirements') }}</div>
      </section>
    </div>
    <div v-if="insights" class="charts">
      <section class="card" aria-labelledby="ch-risk-levels">
        <div class="row" style="justify-content: space-between; margin-bottom: 8px">
          <h2 id="ch-risk-levels" style="margin: 0">{{ t('dashboard.risksByLevel') }}</h2>
          <RouterLink v-if="canReadRisks" :to="{ name: 'risks', params: { projectId: proj.pid } }" class="small">{{ t('dashboard.riskRegister') }}</RouterLink>
        </div>
        <div class="chart-box">
          <div v-if="insights.risks.open" role="img" :aria-label="describe(t('dashboard.risksByLevel'), riskLevelChart)" class="chart-fill">
            <Chart type="bar" :data="riskLevelChart" :options="baseOptions()" style="height: 100%" />
          </div>
          <div v-else class="empty">{{ t('dashboard.noOpenRisks') }}</div>
        </div>
      </section>
      <section class="card" aria-labelledby="ch-heat">
        <h2 id="ch-heat">{{ t('dashboard.riskHeat') }}</h2>
        <div v-if="!heatTotal" class="empty">{{ t('dashboard.noRisks') }}</div>
        <template v-else>
          <div class="heat-wrap">
            <table class="mini-heat">
              <caption class="sr-only">{{ t('dashboard.riskHeatCaption') }}</caption>
              <thead>
                <tr>
                  <th scope="col" class="corner"><span class="small">{{ t('risksView.likelihood') }} ↓ / {{ t('risksView.impact') }} →</span></th>
                  <th v-for="i in 5" :key="i" scope="col">{{ i }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in heat" :key="row.likelihood">
                  <th scope="row">{{ row.likelihood }}</th>
                  <td v-for="c in row.cells" :key="c.impact" :class="['mh-cell', 'mh-' + c.level, { empty: !c.count }]" :title="heatCellLabel(c)">
                    <span aria-hidden="true">{{ c.count || '' }}</span>
                    <span class="sr-only">{{ heatCellLabel(c) }}</span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <ul class="plain-list mh-legend" :aria-label="t('dashboard.riskLevels')">
            <li v-for="l in LEVELS_DESC" :key="l"><span :class="['mh-swatch', 'mh-' + l]" aria-hidden="true" /> {{ t(`risksView.level.${l}`) }}</li>
          </ul>
        </template>
      </section>
      <section class="card" aria-labelledby="ch-mitigation">
        <h2 id="ch-mitigation">{{ t('dashboard.mitigationTitle') }}</h2>
        <div class="chart-box">
          <div v-if="insights.risks.open" role="img" :aria-label="describe(t('dashboard.mitigationTitle'), mitigationChart)" class="chart-fill">
            <Chart type="bar" :data="mitigationChart" :options="baseOptions(false, 'y')" style="height: 100%" />
          </div>
          <div v-else class="empty">{{ t('dashboard.noOpenRisks') }}</div>
        </div>
      </section>
      <section class="card" aria-labelledby="ch-age">
        <h2 id="ch-age">{{ t('dashboard.defectAge') }}</h2>
        <p v-if="ageSummary" class="small muted" style="margin-top: 0">{{ ageSummary }}</p>
        <div class="chart-box">
          <div v-if="anyData(defectAgeData)" role="img" :aria-label="describeMulti(t('dashboard.defectAge'), defectAgeData, formatNumber)" class="chart-fill">
            <Chart type="bar" :data="defectAgeData" :options="baseOptions(true, 'x', true)" style="height: 100%" />
          </div>
          <div v-else class="empty">{{ t('dashboard.noOpenDefects') }}</div>
        </div>
      </section>
      <section class="card" aria-labelledby="ch-rules">
        <div class="row" style="justify-content: space-between; margin-bottom: 8px">
          <h2 id="ch-rules" style="margin: 0">{{ t('dashboard.traceRules') }}</h2>
          <RouterLink :to="{ name: 'traceability', params: { projectId: proj.pid } }" class="small">{{ t('dashboard.allRules') }}</RouterLink>
        </div>
        <div v-if="!insights.trace_rules.rules" class="empty">{{ t('dashboard.noTraceRules') }}</div>
        <template v-else>
          <p class="rule-verdict" :class="insights.trace_rules.passed ? 'ok' : 'bad'">
            <i :class="insights.trace_rules.passed ? 'pi pi-check-circle' : 'pi pi-times-circle'" aria-hidden="true" />
            {{ insights.trace_rules.passed ? t('dashboard.rulesPassed') : t('dashboard.rulesFailed') }}
          </p>
          <dl class="rule-stats">
            <div v-for="r in ruleRows" :key="r.key">
              <dt>{{ r.label }}</dt>
              <dd>{{ formatNumber(r.value) }}</dd>
            </div>
          </dl>
        </template>
      </section>
    </div>
  </div>
</template>

<style scoped>
.dash-tabs { margin-bottom: 12px; }
.chart-box.tall { height: 300px; }
.trend-toolbar { gap: 10px; flex-wrap: wrap; }
.release-select { min-width: 160px; max-width: 100%; }
.flaky-list { display: grid; gap: 0; }
.flaky-row { display: grid; grid-template-columns: minmax(0, 1fr) auto auto; gap: 12px; align-items: center; padding: 8px 0; border-bottom: 1px solid var(--qtf-border); }
.flaky-row:last-child { border-bottom: 0; }
.flaky-main { min-width: 0; }
.flaky-link { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 600; }
.flaky-seq { display: flex; gap: 3px; flex-wrap: wrap; }
.dot { width: 18px; height: 18px; border-radius: 50%; display: inline-grid; place-items: center; font-size: 9px; color: #fff; }
.dot.passed { background: #15803d; }
.dot.failed { background: #b91c1c; border-radius: 3px; }
.dot.flip { outline: 2px solid var(--qtf-text); outline-offset: 1px; }
.flaky-score { font-size: 12px; font-weight: 600; white-space: nowrap; }
.release-name { font-weight: 600; }
.release-bar { margin: 12px 0 4px; }
/* mini risk heat map: the level is told by colour, border style and the
   legend (and the cell text for screen readers), never by colour alone */
.heat-wrap { overflow-x: auto; }
.mini-heat { border-collapse: separate; border-spacing: 3px; margin: 0 auto; }
.mini-heat th { font-size: 12px; font-weight: 600; color: var(--qtf-muted); padding: 2px 4px; text-align: center; }
.mini-heat .corner { text-align: left; max-width: 90px; font-weight: 500; }
.mh-cell { width: 40px; height: 32px; text-align: center; font-weight: 700; color: #111; border-radius: 5px; border: 2px solid transparent; }
.mh-cell.empty { opacity: 0.5; }
.mh-low { background: #bbf7d0; }
.mh-medium { background: #fde68a; border: 2px dotted #92400e; }
.mh-high { background: #fdba74; border: 2px dashed #9a3412; }
.mh-critical { background: repeating-linear-gradient(135deg, #fca5a5 0 6px, #f87171 6px 9px); border: 2px solid #7f1d1d; }
.mh-legend { display: flex; flex-wrap: wrap; gap: 6px 14px; margin-top: 10px; font-size: 12px; }
.mh-legend li { display: inline-flex; align-items: center; gap: 6px; }
.mh-swatch { display: inline-block; width: 18px; height: 14px; border-radius: 3px; }
.rule-verdict { display: flex; align-items: center; gap: 8px; font-weight: 600; margin: 0 0 12px; }
.rule-verdict.ok i { color: var(--p-green-600, #16a34a); }
.rule-verdict.bad i { color: var(--p-red-600, #dc2626); }
.rule-stats { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin: 0; }
.rule-stats div { border: 1px solid var(--qtf-border); border-radius: 8px; padding: 8px 10px; }
.rule-stats dt { color: var(--qtf-muted); font-size: 12px; }
.rule-stats dd { margin: 0; font-size: 22px; font-weight: 700; }
@media (forced-colors: active) {
  .mh-cell, .mh-swatch { forced-color-adjust: none; }
}
@media (max-width: 640px) {
  .flaky-row { grid-template-columns: minmax(0, 1fr) auto; }
  .flaky-seq { grid-column: 1 / -1; }
}
.corr-counts {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 20px;
}
</style>
