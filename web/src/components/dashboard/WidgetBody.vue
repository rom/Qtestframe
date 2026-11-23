<script setup lang="ts">
// The content of one dashboard widget, drawn from its data (see
// lib/widgets.ts for the types and internal/api/dashboardwidgets.go for
// the data). Inline controls (trend range, release, grouping) change the
// widget for this view only: they emit "override".
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Chart from 'primevue/chart'
import Skeleton from 'primevue/skeleton'
import Message from 'primevue/message'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import SummaryBar from '@/components/SummaryBar.vue'
import StatusTag from '@/components/StatusTag.vue'
import MyWorkWidget from '@/components/MyWorkWidget.vue'
import QualityMetrics from '@/components/QualityMetrics.vue'
import ConsistencySection from '@/components/ConsistencySection.vue'
import CountsView from './CountsView.vue'
import { useDashboardCharts } from '@/composables/dashboardCharts'
import type { WidgetEntry } from '@/composables/widgetData'
import { COVERAGE_COLORS, RESULT_COLORS } from '@/lib/colors'
import { seriesColor } from '@/themes/chart'
import { formatDate, formatNumber, percent, relativeTime } from '@/lib/format'
import { enumLabel } from '@/i18n/labels'
import { RESULT_ORDER, summarize } from '@/lib/summary'
import { burnupSeries, countValues, describeMulti, groupCoverageChart, runProgress, summarizeSeries, trendSeries, weekSeries, type Burnup, type ChartData, type FlakyTest, type GroupCoverage, type Trends } from '@/lib/metrics'
import { LEVELS_DESC, MITIGATION_ORDER, REVIEW_ORDER, RISK_LEVEL_COLORS, activityChart, anyData, defectAgeChart, heatRows, share, testTypeChart, type DashboardInsights, type MiniHeatCell } from '@/lib/insights'
import { criterionText } from '@/lib/releases'
import { renderComment } from '@/lib/commentMarkdown'
import { threadTarget } from '@/lib/discussions'
import type { CycleBurndownPoint, DiscussionThread, ReleaseStatusReport, TestRun } from '@/lib/types'
import type { ExploratoryMetrics } from '@/lib/exploratory'
import { setting, type DashboardWidget } from '@/lib/widgets'
import { useMetaStore } from '@/stores/meta'

const props = defineProps<{
  widget: DashboardWidget
  entry?: WidgetEntry
  /** the project of the widget ('' on the home dashboard without one) */
  projectId: string
  /** settings changed inline for this view */
  overrides?: Record<string, unknown>
  /** bumped by refresh: self-loading widgets load again */
  tick?: number
  uid: string
}>()
const emit = defineEmits<{ override: [key: string, value: unknown] }>()
const { t, te } = useI18n()
const meta = useMetaStore()
const charts = useDashboardCharts()
const theme = charts.theme

const type = computed(() => props.widget.type)
const opt = <T = string,>(key: string): T => (props.overrides?.[key] ?? setting<T>(props.widget, key)) as T
const chartKind = computed(() => opt<string>('chart') || 'bar')
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const d = computed<any>(() => props.entry?.data ?? null)
const pid = computed(() => props.projectId)
const errorText = computed(() => {
  const e = props.entry?.error
  if (!e) return ''
  if (e.status === 403) return t('dashboards.noAccess')
  if (e.status === 404) return t('dashboards.notFound')
  return e.error || t('dashboards.loadFailed')
})
const showSkeleton = computed(() => !props.entry || (!props.entry.loaded && props.entry.data == null && !props.entry.error))

// ---------- key figures ----------
const kpis = computed(() => {
  const x = d.value?.dashboard
  if (!x) return []
  const reqTotal = x.requirements?.total ?? 0
  const results = summarize(x.results)
  const pct = (n: number, total: number) => formatNumber(share(n, total) / 100, undefined, { style: 'percent' })
  const out = [
    { label: t('dashboard.kpiRequirements'), value: formatNumber(reqTotal), sub: t('dashboard.kpiUncovered', { count: x.requirements?.uncovered ?? 0 }), icon: 'pi pi-list-check', color: '#6366f1' },
    { label: t('dashboard.kpiCoverage'), value: formatNumber(percent(x.requirements?.covered ?? 0, reqTotal) / 100, undefined, { style: 'percent' }), sub: t('dashboard.kpiCovered', { count: x.requirements?.covered ?? 0 }), icon: 'pi pi-shield', color: '#22c55e' },
    {
      label: t('dashboard.kpiTests'),
      value: formatNumber(x.tests?.total ?? 0),
      sub: t('dashboard.kpiTestsSplit', { manual: x.tests?.by_execution_type?.manual ?? 0, automated: x.tests?.by_execution_type?.automated ?? 0 }),
      icon: 'pi pi-check-square',
      color: '#0ea5e9',
    },
    { label: t('dashboard.kpiRuns'), value: formatNumber(x.runs?.total ?? 0), sub: t('dashboard.kpiInProgress', { count: x.runs?.by_status?.in_progress ?? 0 }), icon: 'pi pi-play-circle', color: '#f59e0b' },
    { label: t('dashboard.kpiPassRate'), value: formatNumber(results.passRate / 100, undefined, { style: 'percent' }), sub: t('dashboard.kpiExecutions', results.executed), icon: 'pi pi-chart-line', color: '#ec4899' },
  ]
  const i = d.value?.insights as DashboardInsights | undefined
  if (i) {
    const rel = i.release
    out.push(
      { label: t('dashboard.kpiVerified'), value: pct(i.requirements.passing, i.requirements.total), sub: t('dashboard.kpiVerifiedSub', { count: i.requirements.passing }, i.requirements.passing), icon: 'pi pi-verified', color: '#16a34a' },
      { label: t('dashboard.kpiOpenRisks'), value: formatNumber(i.risks.open), sub: t('dashboard.kpiOpenRisksSub', { critical: i.risks.by_level.critical ?? 0, high: i.risks.by_level.high ?? 0 }), icon: 'pi pi-exclamation-triangle', color: '#dc2626' },
      {
        label: t('dashboard.kpiOpenDefects'),
        value: formatNumber(i.defects.open),
        sub: i.defects.open ? t('dashboard.kpiDefectAge', { days: formatNumber(i.defects.average_days) }) : t('dashboard.kpiNoOpenDefects'),
        icon: 'pi pi-bug',
        color: '#ea580c',
      },
      { label: t('dashboard.kpiSuspect'), value: formatNumber(i.suspect.links), sub: t('dashboard.kpiSuspectSub', { count: i.suspect.requirements }, i.suspect.requirements), icon: 'pi pi-link', color: '#d97706' },
      {
        label: t('dashboard.kpiRuleViolations'),
        value: formatNumber(i.trace_rules.violations),
        sub: i.trace_rules.rules ? t('dashboard.kpiRulesSub', { failed: i.trace_rules.failed, rules: i.trace_rules.rules }) : t('dashboard.kpiNoRules'),
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
    )
  }
  return out
})

// ---------- breakdowns ----------
const SEVERITY_COLORS: Record<string, string> = { critical: '#dc2626', major: '#ea580c', minor: '#d97706', trivial: '#64748b' }
const SEVERITY_ORDER = ['critical', 'major', 'minor', 'trivial']
/** a defect or risk status (unknown ones as the generic status labels) */
const tr = (kind: 'defect' | 'risk') => (k: string) => {
  const key = kind === 'defect' ? `defectsView.status.${k}` : `risksView.status.${k}`
  return te(key) ? t(key) : enumLabel('status', k)
}
interface DimView {
  label: (k: string) => string
  order?: readonly string[]
  colors?: Record<string, string>
  empty: string
}
const dimView = computed<DimView>(() => {
  switch (opt('dimension')) {
    case 'requirement_obligation':
      return { label: (k) => k.replace(/_/g, ' ').toUpperCase(), order: meta.meta.obligations, empty: t('dashboard.noRequirements') }
    case 'review_state':
      return { label: (k) => t(`dashboard.review.${k}`), order: REVIEW_ORDER, empty: t('dashboard.noRequirements') }
    case 'test_status':
      return { label: (k) => enumLabel('status', k), order: meta.meta.test_statuses, empty: t('dashboard.noTests') }
    case 'test_execution_type':
      return { label: (k) => enumLabel('executionType', k), empty: t('dashboard.noTests') }
    case 'run_status':
      return { label: (k) => enumLabel('status', k), empty: t('dashboard.noRuns') }
    case 'defect_severity':
      return { label: (k) => t(`defectsView.severity.${k}`), order: SEVERITY_ORDER, colors: SEVERITY_COLORS, empty: t('dashboard.noOpenDefects') }
    case 'defect_status':
      return { label: tr('defect'), empty: t('dashboards.noDefects') }
    case 'risk_level':
      return { label: (k) => t(`risksView.level.${k}`), order: LEVELS_DESC, colors: RISK_LEVEL_COLORS, empty: t('dashboard.noOpenRisks') }
    case 'risk_status':
      return { label: tr('risk'), empty: t('dashboard.noRisks') }
    case 'risk_mitigation':
      return { label: (k) => t(`dashboard.mitigation.${k}`), order: MITIGATION_ORDER, empty: t('dashboard.noOpenRisks') }
    default:
      return { label: (k) => enumLabel('status', k), order: meta.meta.requirement_statuses, empty: t('dashboard.noRequirements') }
  }
})
// the former chart showed every severity, also without defects
const breakdownRec = computed<Record<string, number> | undefined>(() => {
  const c = d.value?.counts as Record<string, number> | undefined
  if (opt('dimension') === 'defect_severity' && c) return Object.fromEntries([...SEVERITY_ORDER.map((k) => [k, c[k] ?? 0]), ...Object.entries(c).filter(([k]) => !SEVERITY_ORDER.includes(k))])
  return c
})
const verifLabel = (k: string) => enumLabel('verification', k)

// ---------- runs ----------
const activeRuns = computed<TestRun[]>(() => d.value?.active_runs ?? [])
const progressChart = computed(() => {
  const c = runProgress(activeRuns.value, (s) => enumLabel('result', s))
  c.datasets = c.datasets.map((x) => ({ ...x, borderRadius: 4, maxBarThickness: 26 }))
  return c
})
const hasAny = (c: ChartData) => c.datasets.some((x) => x.data.some((v) => (v ?? 0) > 0))

// ---------- test types, activity, defect age ----------
const testTypeData = computed(() => {
  const c = testTypeChart(d.value?.by_type, (ty) => enumLabel('executionType', ty), (s) => enumLabel('result', s))
  c.datasets = c.datasets.map((x) => ({ ...x, borderRadius: 4, maxBarThickness: 34 }))
  return c
})
const activity = computed(() => d.value?.activity as DashboardInsights['activity'] | undefined)
const activityData = computed(() => {
  const c = activityChart(activity.value?.days, formatDate, (s) => t(`dashboard.activitySeries.${s}`), (i) => seriesColor(theme.value, i))
  c.datasets = c.datasets.map((x) => ({ ...x, borderRadius: 3, maxBarThickness: 14 }))
  return c
})
const activityDescription = computed(() => {
  const tot = activity.value?.total
  if (!tot) return t('dashboard.activity')
  return `${t('dashboard.activity')}. ${t('dashboard.activityTotal', { changes: tot.changes, executions: tot.executions, defects: tot.defects })}. ${describeMulti('', activityData.value, formatNumber, 30).replace(/^\.\s*/, '')}`
})
const defects = computed(() => d.value?.defects as DashboardInsights['defects'] | undefined)
const defectAgeData = computed(() => {
  const c = defectAgeChart(defects.value, (s) => t(`defectsView.severity.${s}`), (b) => t(`dashboard.ageBucket.${b}`))
  c.datasets = c.datasets.map((x) => ({ ...x, borderRadius: 4, maxBarThickness: 34 }))
  return c
})

// ---------- trends ----------
const rangeOptions = computed(() => [30, 90, 365].map((n) => ({ value: n, label: t('dashboard.rangeDays', { n }) })))
const range = computed({ get: () => Number(opt('range')) || 30, set: (v: number) => emit('override', 'range', v) })
const trend = computed(() => trendSeries(d.value as Trends | null))
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
const fmtPct = (n: number) => `${formatNumber(n)} %`
const trendDays = computed(() => countValues(trend.value.coverage))
const trendDescription = computed(() => {
  const parts = trendChart.value.datasets
    .map((x) => {
      const s = summarizeSeries(x.data)
      return s && t('dashboard.trendSummary', { series: x.label, first: fmtPct(s.first), last: fmtPct(s.last), min: fmtPct(s.min), max: fmtPct(s.max) })
    })
    .filter(Boolean)
  return [t('dashboard.trendTitle'), t('dashboard.trendDays', trendDays.value), ...parts].join('. ')
})
const weekChart = computed<ChartData>(() => {
  const w = weekSeries(d.value?.defect_weeks)
  return {
    labels: w.weeks.map(formatDate),
    datasets: [
      { label: t('dashboard.seriesOpened'), data: w.opened, backgroundColor: '#ef4444', borderRadius: 4, maxBarThickness: 18 },
      { label: t('dashboard.seriesClosed'), data: w.closed, backgroundColor: '#22c55e', borderRadius: 4, maxBarThickness: 18 },
    ],
  }
})

// ---------- releases and cycles ----------
interface Pick_ {
  id: string
  name: string
  version?: string
  status: string
}
const pickLabel = (r: Pick_) => (r.version && r.version !== r.name ? `${r.name} (${r.version})` : r.name)
const releaseOptions = computed(() => ((d.value?.releases ?? []) as Pick_[]).map((r) => ({ value: r.id, label: pickLabel(r) })))
const releaseId = computed({ get: () => (d.value?.release as Pick_ | null)?.id ?? null, set: (v: string | null) => emit('override', 'release_id', v ?? '') })
const cycleOptions = computed(() => ((d.value?.cycles ?? []) as Pick_[]).map((r) => ({ value: r.id, label: r.name })))
const cycleId = computed({ get: () => (d.value?.cycle as Pick_ | null)?.id ?? null, set: (v: string | null) => emit('override', 'cycle_id', v ?? '') })
const burnup = computed(() => (d.value?.burnup ?? null) as Burnup | null)
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
const burndown = computed<CycleBurndownPoint[]>(() => d.value?.report?.burndown ?? [])
const burndownChart = computed<ChartData>(() => ({
  labels: burndown.value.map((p) => formatDate(p.day)),
  datasets: [
    { label: t('dashboards.remaining'), data: burndown.value.map((p) => p.remaining), borderColor: seriesColor(theme.value, 0), backgroundColor: seriesColor(theme.value, 0), pointRadius: 0, borderWidth: 2 },
    { label: t('dashboards.ideal'), data: burndown.value.map((p) => p.ideal), borderColor: theme.value.text, backgroundColor: theme.value.text, borderDash: [4, 4], pointRadius: 0, borderWidth: 2 },
  ],
}))
const report = computed(() => (d.value?.report ?? null) as ReleaseStatusReport | null)
const tt = (key: string, params?: Record<string, unknown>) => t(key, params ?? {})
const levelLabel = (kind: 'severity' | 'riskLevel', v: string) => t(kind === 'severity' ? `releasesView.severity.${v}` : `risksView.level.${v}`)
const releaseRun = computed(() => {
  const r = d.value?.release as DashboardInsights['release'] | undefined
  return r ? { ...r.executions, total: r.total } : null
})

// ---------- flaky, groups, heat, rules ----------
const flaky = computed<FlakyTest[]>(() => ((d.value?.flaky?.items ?? []) as FlakyTest[]).slice(0, Number(opt('limit')) || 10))
const resultLabel = (s: string) => enumLabel('result', s)
const sequenceText = (f: FlakyTest) => t('dashboard.flakySequence', { seq: f.results.map((r) => resultLabel(r.status)).join(', ') })
const groupOptions = computed(() => [
  { value: 'folder', label: t('dashboard.byFolder') },
  { value: 'category', label: t('dashboard.byCategory') },
])
const groupBy = computed({ get: () => opt('group') || 'folder', set: (v: string) => emit('override', 'group', v) })
const groups = computed<GroupCoverage[]>(() => (groupBy.value === 'category' ? d.value?.coverage_by_category : d.value?.coverage_by_folder) ?? [])
const groupName = (g: GroupCoverage) => (groupBy.value === 'folder' ? g.name || t('dashboard.noFolder') : g.dimension ? `${g.dimension}: ${g.name}` : g.name)
const groupChart = computed(() => {
  const c = groupCoverageChart(groups.value, groupName, (s) => enumLabel('coverage', s))
  c.datasets = c.datasets.map((x) => ({ ...x, borderRadius: 4, maxBarThickness: 24 }))
  return c
})
const heat = computed(() => heatRows(d.value?.heat))
const heatCellLabel = (c: MiniHeatCell) =>
  t('dashboard.heatCell', { likelihood: c.likelihood, impact: c.impact, level: t(`risksView.level.${c.level}`), count: t('risksView.riskCount', { count: c.count }, c.count) })
const heatTotal = computed(() => heat.value.reduce((n, r) => n + r.cells.reduce((m, c) => m + c.count, 0), 0))
const rules = computed(() => d.value?.trace_rules as DashboardInsights['trace_rules'] | undefined)
const ruleRows = computed(() => (['rules', 'errors', 'warnings', 'violations'] as const).map((k) => ({ key: k, label: t(`dashboard.ruleCounts.${k}`), value: rules.value?.[k] ?? 0 })))

// ---------- lists ----------
const threads = computed<DiscussionThread[]>(() => d.value?.items ?? [])
interface QueryRow {
  id: string
  key: string
  title: string
  status: string
  extra?: string
}
const queryRows = computed<QueryRow[]>(() => d.value?.items ?? [])
function rowRoute(entity: string, r: QueryRow) {
  const params = { projectId: pid.value }
  switch (entity) {
    case 'tests':
      return { name: 'tests', params, query: { item: r.id } }
    case 'defects':
      return { name: 'defects', params, query: { item: r.id } }
    case 'risks':
      return { name: 'risks', params, query: { risk: r.id } }
    case 'runs':
      return { name: 'run', params: { ...params, runId: r.id } }
    default:
      return { name: 'requirements', params, query: { item: r.id } }
  }
}
const listRoute = (entity: string) => {
  const name = entity === 'runs' ? 'runs' : entity
  const q = opt('query')
  return { name, params: { projectId: pid.value }, query: q ? { q } : {} }
}
const extraLabel = (entity: string, v: string) => {
  if (!v) return ''
  if (entity === 'requirements') return enumLabel('coverage', v)
  if (entity === 'tests') return enumLabel('result', v)
  if (entity === 'defects') return t(`defectsView.severity.${v}`)
  if (entity === 'risks') return t(`risksView.level.${v}`)
  return v
}
const statusText = (entity: string, s: string) => (entity === 'defects' ? tr('defect')(s) : entity === 'risks' ? tr('risk')(s) : enumLabel('status', s))
const noteHtml = computed(() => renderComment(String(opt('text') ?? ''), [], t('links.newTab')))
const exploratory = computed(() => d.value as ExploratoryMetrics | null)
const frameworkRows = computed<{ id: string; name: string; covered: number; total: number }[]>(() => d.value?.rows ?? [])
const frameworkRoute = computed(() => ({ name: 'catalog-attack', params: { projectId: pid.value }, query: { framework: opt('framework') } }))
</script>

<template>
  <!-- widgets with their own data and controls -->
  <QualityMetrics v-if="type === 'quality_metrics' && pid" :key="`q${tick}`" :project-id="pid" :id-prefix="uid" hide-title />
  <ConsistencySection v-else-if="type === 'consistency' && pid" :key="`c${tick}`" :project-id="pid" compact bare />
  <div v-else-if="type === 'note'" class="note-md">
    <!-- eslint-disable-next-line vue/no-v-html -- sanitised Markdown (lib/commentMarkdown) -->
    <div v-if="opt('text')" v-html="noteHtml" />
    <p v-else class="muted small">{{ t('dashboards.noteEmpty') }}</p>
  </div>
  <p v-else-if="!pid && widget.type !== 'my_work' && widget.type !== 'discussions'" class="empty">{{ t('dashboards.chooseProject') }}</p>
  <Message v-else-if="entry?.error" severity="warn" :closable="false" class="widget-msg">{{ errorText }}</Message>
  <Skeleton v-else-if="showSkeleton" height="100%" />

  <template v-else-if="d !== null || type === 'release_progress'">
    <ul v-if="type === 'key_figures'" class="kpis plain-list" :aria-label="t('dashboard.kpis')">
      <li v-for="k in kpis" :key="k.label" class="kpi kpi-cell">
        <div class="icon" aria-hidden="true" :style="{ background: k.color + '1f', color: k.color }"><i :class="k.icon" /></div>
        <div class="kpi-text">
          <div class="value">{{ k.value }}</div>
          <div class="label">{{ k.label }}</div>
          <div class="small muted">{{ k.sub }}</div>
        </div>
      </li>
    </ul>

    <CountsView
      v-else-if="type === 'coverage'"
      :rec="d.coverage"
      :title="t('dashboard.requirementCoverage')"
      :label="(k: string) => enumLabel('coverage', k)"
      :order="['passed', 'failed', 'partial', 'not_run', 'none']"
      :colors="COVERAGE_COLORS"
      :chart="chartKind"
      :empty="t('dashboard.noRequirements')"
    />
    <CountsView
      v-else-if="type === 'execution_results'"
      :rec="d.results"
      :title="t('dashboard.executionResults')"
      :label="(k: string) => enumLabel('result', k)"
      :order="RESULT_ORDER"
      :colors="RESULT_COLORS"
      :chart="chartKind"
      :empty="t('dashboard.noExecutions')"
    />
    <div v-else-if="type === 'verification_status'" class="stack">
      <p class="small muted">{{ t('dashboards.criteriaVerified', { n: d.criteria_verified, total: d.criteria_total }) }}<span v-if="d.inconsistent"> · {{ t('dashboards.inconsistent', { n: d.inconsistent }, d.inconsistent) }}</span></p>
      <div class="grow-box">
        <CountsView :rec="d.counts" :title="t('dashboards.types.verification_status')" :label="verifLabel" :chart="chartKind" :empty="t('dashboard.noRequirements')" />
      </div>
      <RouterLink :to="{ name: 'verification', params: { projectId: pid } }" class="small">{{ t('dashboards.openVerification') }}</RouterLink>
    </div>
    <CountsView
      v-else-if="type === 'breakdown'"
      :rec="breakdownRec"
      :title="widget.title || t(`dashboards.dimension.${opt('dimension')}`)"
      :label="dimView.label"
      :order="dimView.order"
      :colors="dimView.colors"
      :chart="chartKind"
      :empty="dimView.empty"
    />

    <template v-else-if="type === 'test_types'">
      <CountsView v-if="chartKind === 'table'" :rec="Object.fromEntries(Object.entries(d.by_type ?? {}).map(([k, v]) => [k, Object.values(v as Record<string, number>).reduce((a, b) => a + b, 0)]))" :title="t('dashboard.testTypes')" :label="(k: string) => enumLabel('executionType', k)" chart="table" :empty="t('dashboard.noTests')" />
      <div v-else-if="anyData(testTypeData)" role="img" :aria-label="describeMulti(t('dashboard.testTypes'), testTypeData, formatNumber)" class="chart-fill">
        <Chart type="bar" :data="testTypeData" :options="charts.baseOptions(true, 'x', true)" style="height: 100%" />
      </div>
      <div v-else class="empty">{{ t('dashboard.noTests') }}</div>
    </template>

    <template v-else-if="type === 'active_runs'">
      <div v-if="hasAny(progressChart)" class="chart-fill" role="img" :aria-label="describeMulti(t('dashboard.activeRuns'), progressChart, formatNumber)">
        <Chart type="bar" :data="progressChart" :options="charts.baseOptions(true, 'y', true)" style="height: 100%" />
      </div>
      <div v-else class="empty">{{ t('dashboard.noActiveRuns') }}</div>
    </template>

    <div v-else-if="type === 'recent_runs'">
      <div class="row" style="justify-content: flex-end"><RouterLink :to="{ name: 'runs', params: { projectId: pid } }" class="small">{{ t('dashboard.allRuns') }}</RouterLink></div>
      <div v-if="!(d.recent_runs ?? []).length" class="empty">{{ t('dashboard.noRuns') }}</div>
      <RouterLink v-for="r in d.recent_runs ?? []" :key="r.id" :to="{ name: 'run', params: { projectId: pid, runId: r.id } }" class="recent-run">
        <div class="row nowrap" style="justify-content: space-between">
          <span class="ellipsis" style="font-weight: 600">{{ r.name }}</span>
          <StatusTag :value="r.status" />
        </div>
        <div class="row nowrap" style="margin-top: 4px">
          <SummaryBar :summary="r.summary" class="grow" />
          <span class="small muted">{{ relativeTime(r.created_at) }}</span>
        </div>
      </RouterLink>
    </div>

    <div v-else-if="type === 'test_results_trend'" class="stack">
      <div class="row toolbar">
        <SelectButton v-model="range" :options="rangeOptions" option-label="label" option-value="value" :allow-empty="false" size="small" :aria-label="t('dashboard.trendRange')" />
      </div>
      <div class="grow-box">
        <div v-if="trendDays > 0" role="img" :aria-label="trendDescription" class="chart-fill">
          <Chart type="line" :data="trendChart" :options="charts.lineOptions(true)" style="height: 100%" />
        </div>
        <div v-else class="empty">{{ t('dashboard.noTrend') }}</div>
      </div>
    </div>

    <div v-else-if="type === 'defects_per_week'" class="stack">
      <div class="row toolbar">
        <SelectButton v-model="range" :options="rangeOptions" option-label="label" option-value="value" :allow-empty="false" size="small" :aria-label="t('dashboard.trendRange')" />
      </div>
      <div class="grow-box">
        <div v-if="hasAny(weekChart)" role="img" :aria-label="describeMulti(t('dashboard.defectsPerWeek'), weekChart, formatNumber)" class="chart-fill">
          <Chart type="bar" :data="weekChart" :options="charts.baseOptions(true)" style="height: 100%" />
        </div>
        <div v-else class="empty">{{ t('dashboard.noDefectsInPeriod') }}</div>
      </div>
    </div>

    <div v-else-if="type === 'recent_activity'" class="stack">
      <p v-if="activity" class="small muted">{{ t('dashboard.activityTotal', { changes: activity.total.changes, executions: activity.total.executions, defects: activity.total.defects }) }}</p>
      <div class="grow-box">
        <div v-if="anyData(activityData)" role="img" :aria-label="activityDescription" class="chart-fill">
          <Chart type="bar" :data="activityData" :options="charts.stackedDaysOptions()" style="height: 100%" />
        </div>
        <div v-else class="empty">{{ t('dashboard.noActivity') }}</div>
      </div>
    </div>

    <div v-else-if="type === 'flaky_tests'">
      <p class="small muted" style="margin-top: 0">{{ t('dashboard.flakyHint', { min: d.flaky?.min_flips ?? 2, window: d.flaky?.window ?? 10 }) }}</p>
      <div v-if="!flaky.length" class="empty">{{ t('dashboard.noFlaky') }}</div>
      <ol v-else class="plain-list flaky-list">
        <li v-for="f in flaky" :key="f.test_id" class="flaky-row">
          <div class="flaky-main">
            <RouterLink :to="{ name: 'tests', params: { projectId: pid }, query: { item: f.test_id } }" class="flaky-link"><span class="key">{{ f.key }}</span> <span>{{ f.title }}</span></RouterLink>
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
      <div v-if="(d.flaky?.total ?? 0) > flaky.length" class="small muted" style="margin-top: 8px">
        {{ t('dashboard.flakyMore', { shown: flaky.length, total: d.flaky?.total }) }}
        <RouterLink :to="{ name: 'tests', params: { projectId: pid }, query: { q: 'flaky:true' } }">{{ t('dashboard.allFlaky') }}</RouterLink>
      </div>
    </div>

    <div v-else-if="type === 'coverage_by_group'" class="stack">
      <div class="row toolbar">
        <SelectButton v-model="groupBy" :options="groupOptions" option-label="label" option-value="value" :allow-empty="false" size="small" :aria-label="t('dashboard.groupBy')" />
      </div>
      <div class="grow-box">
        <div v-if="groups.length" class="chart-fill" role="img" :aria-label="describeMulti(t('dashboard.coverageByGroup'), groupChart, formatNumber, 20)">
          <Chart type="bar" :data="groupChart" :options="charts.baseOptions(true, 'y', true)" style="height: 100%" />
        </div>
        <div v-else class="empty">{{ t('dashboard.noRequirements') }}</div>
      </div>
    </div>

    <div v-else-if="type === 'defect_age'" class="stack">
      <p v-if="defects?.open" class="small muted">{{ t('dashboard.defectAgeSummary', { avg: formatNumber(defects.average_days), oldest: formatNumber(defects.oldest_days) }) }}</p>
      <div class="grow-box">
        <div v-if="anyData(defectAgeData)" role="img" :aria-label="describeMulti(t('dashboard.defectAge'), defectAgeData, formatNumber)" class="chart-fill">
          <Chart type="bar" :data="defectAgeData" :options="charts.baseOptions(true, 'x', true)" style="height: 100%" />
        </div>
        <div v-else class="empty">{{ t('dashboard.noOpenDefects') }}</div>
      </div>
    </div>

    <div v-else-if="type === 'risk_heat'">
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
    </div>

    <div v-else-if="type === 'trace_rules'">
      <div class="row" style="justify-content: flex-end"><RouterLink :to="{ name: 'traceability', params: { projectId: pid } }" class="small">{{ t('dashboard.allRules') }}</RouterLink></div>
      <div v-if="!rules?.rules" class="empty">{{ t('dashboard.noTraceRules') }}</div>
      <template v-else>
        <p class="rule-verdict" :class="rules.passed ? 'ok' : 'bad'">
          <i :class="rules.passed ? 'pi pi-check-circle' : 'pi pi-times-circle'" aria-hidden="true" />
          {{ rules.passed ? t('dashboard.rulesPassed') : t('dashboard.rulesFailed') }}
        </p>
        <dl class="rule-stats">
          <div v-for="r in ruleRows" :key="r.key">
            <dt>{{ r.label }}</dt>
            <dd>{{ formatNumber(r.value) }}</dd>
          </div>
        </dl>
      </template>
    </div>

    <div v-else-if="type === 'correlations'">
      <p class="small muted" style="margin-top: 0">{{ t('correlations.dashboardHint') }}</p>
      <ul v-if="d.correlations" class="plain-list corr-counts">
        <li>
          <RouterLink :to="{ name: 'requirements', params: { projectId: pid }, query: { q: 'blocked>0' } }" class="blocked-badge">
            <i class="pi pi-ban" aria-hidden="true" />{{ t('correlations.dashBlocked', { n: d.correlations.blocked_requirements }, d.correlations.blocked_requirements) }}
          </RouterLink>
        </li>
        <li class="warned-badge"><i class="pi pi-exclamation-triangle" aria-hidden="true" />{{ t('correlations.dashWarned', { n: d.correlations.warned_requirements }, d.correlations.warned_requirements) }}</li>
        <li>
          <RouterLink :to="{ name: 'tests', params: { projectId: pid }, query: { q: 'stale:true' } }"><i class="pi pi-clock" aria-hidden="true" /> {{ t('correlations.dashStale', { n: d.correlations.stale_results }, d.correlations.stale_results) }}</RouterLink>
        </li>
        <li>
          <RouterLink :to="{ name: 'requirements', params: { projectId: pid }, query: { q: 'suspect_relations>0' } }">
            <i class="pi pi-exclamation-triangle" aria-hidden="true" /> {{ t('correlations.dashSuspect', { n: d.correlations.suspect_relations }, d.correlations.suspect_relations) }}
          </RouterLink>
        </li>
      </ul>
    </div>

    <div v-else-if="type === 'release_progress'">
      <div v-if="!d?.release" class="empty">{{ t('dashboard.noActiveRelease') }}</div>
      <template v-else>
        <div class="row" style="justify-content: space-between">
          <RouterLink :to="{ name: 'releases', params: { projectId: pid }, query: { r: d.release.id } }" class="release-name">{{ d.release.name }}</RouterLink>
          <span class="small muted">{{ t(`releasesView.status.${d.release.status}`) }}</span>
        </div>
        <div v-if="d.release.target_date" class="small muted">{{ t('dashboard.burnupTarget', { date: formatDate(d.release.target_date) }) }}</div>
        <div v-if="!d.release.total" class="empty">{{ t('dashboard.noReleaseRuns') }}</div>
        <template v-else>
          <SummaryBar :summary="releaseRun" class="release-bar" />
          <p class="small">{{ t('dashboard.releaseSummary', { runs: d.release.runs, executed: d.release.executed, total: d.release.total, passed: d.release.passed }) }}</p>
        </template>
      </template>
    </div>

    <div v-else-if="type === 'release_readiness'" class="stack">
      <div v-if="!releaseOptions.length" class="empty">{{ t('dashboard.noReleases') }}</div>
      <template v-else>
        <div class="row toolbar">
          <Select v-model="releaseId" :options="releaseOptions" option-label="label" option-value="value" size="small" :aria-label="t('dashboard.burnupRelease')" class="pick-select" />
          <Tag v-if="report" :value="report.go ? t('dashboards.go') : t('dashboards.noGo')" :severity="report.go ? 'success' : 'danger'" :icon="report.go ? 'pi pi-check' : 'pi pi-times'" />
          <span v-if="report?.variant" class="small muted">{{ report.variant.name }}</span>
        </div>
        <ul v-if="report" class="plain-list criteria">
          <li v-for="(c, i) in report.criteria" :key="i" class="row nowrap">
            <i :class="c.state === 'waived' ? 'pi pi-shield' : c.met ? 'pi pi-check-circle ok' : 'pi pi-times-circle bad'" aria-hidden="true" />
            <span class="sr-only">{{ c.state === 'waived' ? t('dashboards.waived') : c.met ? t('dashboards.met') : t('dashboards.unmet') }}:</span>
            <span class="grow">{{ criterionText(c, tt, levelLabel) }}</span>
          </li>
          <li v-if="!report.criteria.length" class="small muted">{{ t('dashboards.noCriteria') }}</li>
        </ul>
        <ul v-if="report?.variants?.length" class="plain-list variant-gates" :aria-label="t('dashboards.perVariant')">
          <li v-for="g in report.variants" :key="g.variant.id">
            <Tag :value="`${g.variant.name}: ${g.go ? t('dashboards.go') : t('dashboards.noGo')}`" :severity="g.go ? 'success' : 'danger'" />
          </li>
        </ul>
        <RouterLink v-if="d.release" :to="{ name: 'releases', params: { projectId: pid }, query: { r: d.release.id } }" class="small">{{ t('dashboards.openRelease') }}</RouterLink>
      </template>
    </div>

    <div v-else-if="type === 'release_burnup'" class="stack">
      <div v-if="!releaseOptions.length" class="empty">{{ t('dashboard.noReleases') }}</div>
      <template v-else>
        <div class="row toolbar">
          <Select v-model="releaseId" :options="releaseOptions" option-label="label" option-value="value" size="small" :aria-label="t('dashboard.burnupRelease')" class="pick-select" />
          <span v-if="burnup?.target_date" class="small muted">{{ t('dashboard.burnupTarget', { date: formatDate(burnup.target_date) }) }}</span>
        </div>
        <div class="grow-box">
          <div v-if="burnup" role="img" :aria-label="describeMulti(t('dashboard.burnup') + ' ' + burnup.name, burnupChart, formatNumber, 8)" class="chart-fill">
            <Chart type="line" :data="burnupChart" :options="charts.lineOptions(false)" style="height: 100%" />
          </div>
        </div>
      </template>
    </div>

    <div v-else-if="type === 'cycle_burndown'" class="stack">
      <div v-if="!cycleOptions.length" class="empty">{{ t('dashboards.noCycles') }}</div>
      <template v-else>
        <div class="row toolbar">
          <Select v-model="cycleId" :options="cycleOptions" option-label="label" option-value="value" size="small" :aria-label="t('dashboards.cycle')" class="pick-select" />
          <RouterLink v-if="cycleId" :to="{ name: 'cycle', params: { projectId: pid, cycleId } }" class="small">{{ t('dashboards.openCycle') }}</RouterLink>
        </div>
        <div class="grow-box">
          <div v-if="burndown.length" role="img" :aria-label="describeMulti(t('dashboards.types.cycle_burndown'), burndownChart, formatNumber, 8)" class="chart-fill">
            <Chart type="line" :data="burndownChart" :options="charts.lineOptions(false)" style="height: 100%" />
          </div>
          <div v-else class="empty">{{ t('dashboards.noBurndown') }}</div>
        </div>
      </template>
    </div>

    <MyWorkWidget v-else-if="type === 'my_work'" :project-id="pid || undefined" :data="d" />

    <div v-else-if="type === 'discussions'">
      <div class="row" style="justify-content: flex-end">
        <RouterLink v-if="pid" :to="{ name: 'discussions', params: { projectId: pid } }" class="small">{{ t('dashboards.allDiscussions') }}</RouterLink>
      </div>
      <div v-if="!threads.length" class="empty">{{ t('dashboards.noDiscussions') }}</div>
      <ul v-else class="plain-list rows">
        <li v-for="th in threads" :key="th.root.id">
          <RouterLink :to="threadTarget(th)" class="ellipsis thread-link">
            <span class="key">{{ th.entity_key }}</span> {{ th.entity_title }}
          </RouterLink>
          <div class="small muted">{{ th.root.user_name || th.root.user_email }} · {{ t('dashboards.replies', { n: th.replies }, th.replies) }} · {{ relativeTime(th.last_at) }}</div>
        </li>
      </ul>
    </div>

    <div v-else-if="type === 'saved_query'">
      <div v-if="opt('display') === 'count'" class="big-count">
        <RouterLink :to="listRoute(d.entity)">
          <span class="value">{{ formatNumber(d.total) }}</span>
          <span class="label">{{ t(`dashboards.entityCount.${d.entity}`, d.total) }}</span>
        </RouterLink>
        <div v-if="opt('query')" class="small muted query-text"><code>{{ opt('query') }}</code></div>
      </div>
      <template v-else>
        <div class="row" style="justify-content: space-between">
          <span class="small muted">{{ t('dashboards.matches', { n: d.total }, d.total) }}</span>
          <RouterLink :to="listRoute(d.entity)" class="small">{{ t('dashboards.openList') }}</RouterLink>
        </div>
        <div v-if="!queryRows.length" class="empty">{{ t('dashboards.noMatches') }}</div>
        <table v-else class="count-table">
          <caption class="sr-only">{{ widget.title || t('dashboards.types.saved_query') }}</caption>
          <thead>
            <tr>
              <th scope="col">{{ t('dashboards.item') }}</th>
              <th scope="col">{{ t('dashboards.status') }}</th>
              <th v-if="d.entity !== 'runs'" scope="col">{{ t(`dashboards.extra.${d.entity}`) }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in queryRows" :key="r.id">
              <td class="title-cell">
                <RouterLink :to="rowRoute(d.entity, r)"><span v-if="r.key" class="key">{{ r.key }}</span> {{ r.title }}</RouterLink>
              </td>
              <td>{{ statusText(d.entity, r.status) }}</td>
              <td v-if="d.entity !== 'runs'">{{ extraLabel(d.entity, r.extra ?? '') }}</td>
            </tr>
          </tbody>
        </table>
      </template>
    </div>

    <div v-else-if="type === 'traceability_gaps'" class="gaps">
      <section v-for="k in ['requirements', 'tests']" :key="k" :aria-labelledby="`${uid}-${k}`">
        <template v-if="d[k]">
          <h3 :id="`${uid}-${k}`" class="plain-heading small-heading">{{ t(`dashboards.gaps.${k}`, { n: d[k].gaps, total: d[k].total }) }}</h3>
          <ul class="plain-list rows">
            <li v-for="r in d[k].items" :key="r.id">
              <RouterLink :to="rowRoute(k, r)" class="ellipsis"><span class="key">{{ r.key }}</span> {{ r.title }}</RouterLink>
            </li>
          </ul>
          <RouterLink v-if="d[k].gaps > d[k].items.length" :to="{ name: 'traceability', params: { projectId: pid } }" class="small">{{ t('dashboards.allGaps') }}</RouterLink>
        </template>
      </section>
    </div>

    <div v-else-if="type === 'exploratory_metrics' && exploratory">
      <dl class="rule-stats">
        <div>
          <dt>{{ t('dashboards.sessions') }}</dt>
          <dd>{{ formatNumber(exploratory.sessions) }}</dd>
        </div>
        <div>
          <dt>{{ t('dashboards.hours') }}</dt>
          <dd>{{ formatNumber(exploratory.hours) }}</dd>
        </div>
        <div>
          <dt>{{ t('dashboards.bugs') }}</dt>
          <dd>{{ formatNumber(exploratory.bugs) }}</dd>
        </div>
        <div>
          <dt>{{ t('dashboards.bugsPerHour') }}</dt>
          <dd>{{ formatNumber(exploratory.bugs_per_hour) }}</dd>
        </div>
      </dl>
      <RouterLink :to="{ name: 'exploratory', params: { projectId: pid } }" class="small">{{ t('dashboards.openExploratory') }}</RouterLink>
    </div>

    <div v-else-if="type === 'framework_coverage'">
      <p class="small muted">{{ t('dashboards.frameworkSummary', { covered: d.covered, total: d.total }) }}</p>
      <div v-if="!frameworkRows.length" class="empty">{{ t('dashboards.noFramework') }}</div>
      <ul v-else class="plain-list fw-rows">
        <li v-for="r in frameworkRows" :key="r.id">
          <div class="row nowrap" style="justify-content: space-between">
            <span class="ellipsis">{{ r.name }}</span>
            <span class="small nowrap">{{ r.covered }} / {{ r.total }}</span>
          </div>
          <div class="fw-bar" role="presentation"><span :style="{ width: `${percent(r.covered, r.total)}%` }" /></div>
        </li>
      </ul>
      <RouterLink :to="frameworkRoute" class="small">{{ t('dashboards.openFrameworks') }}</RouterLink>
    </div>
  </template>
</template>

<style scoped>
.stack { display: flex; flex-direction: column; height: 100%; min-height: 0; gap: 6px; }
.stack > p { margin: 0; }
.grow-box { flex: 1; min-height: 0; }
.toolbar { gap: 8px; flex-wrap: wrap; }
.pick-select { min-width: 160px; max-width: 100%; }
.widget-msg { margin: 0; }
.kpis { margin: 0; gap: 10px; }
.kpi-cell { border: 1px solid var(--qtf-border); border-radius: 8px; padding: 8px 10px; }
.kpi-cell .icon { width: 36px; height: 36px; font-size: 16px; }
.kpi-cell .value { font-size: 20px; }
.recent-run { display: block; }
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
.rule-verdict.ok i, .criteria .ok { color: var(--p-green-600, #16a34a); }
.rule-verdict.bad i, .criteria .bad { color: var(--p-red-600, #dc2626); }
.rule-stats { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin: 0 0 8px; }
.rule-stats div { border: 1px solid var(--qtf-border); border-radius: 8px; padding: 8px 10px; }
.rule-stats dt { color: var(--qtf-muted); font-size: 12px; }
.rule-stats dd { margin: 0; font-size: 22px; font-weight: 700; }
.corr-counts { display: flex; flex-wrap: wrap; gap: 8px 20px; }
.criteria li { gap: 8px; padding: 3px 0; align-items: baseline; }
.variant-gates { display: flex; flex-wrap: wrap; gap: 6px; }
.rows li { padding: 5px 0; border-top: 1px solid var(--qtf-border); min-width: 0; }
.thread-link { display: block; font-weight: 600; }
.big-count { display: grid; place-items: center; height: 100%; text-align: center; }
.big-count a { display: grid; text-decoration: none; }
.big-count .value { font-size: 40px; font-weight: 700; line-height: 1.1; }
.big-count .label { color: var(--qtf-muted); }
.query-text code { font-size: 12px; }
.count-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.count-table th, .count-table td { text-align: left; padding: 4px 6px; border-bottom: 1px solid var(--qtf-border); }
.title-cell { max-width: 0; width: 60%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.gaps { display: grid; gap: 10px; }
.small-heading { font-size: 13px; font-weight: 600; margin: 0 0 4px; }
.fw-rows li { padding: 4px 0; }
.fw-bar { height: 6px; border-radius: 3px; background: var(--qtf-border); overflow: hidden; margin-top: 2px; }
.fw-bar span { display: block; height: 100%; background: var(--p-primary-color, #2563eb); }
.note-md :deep(p:first-child) { margin-top: 0; }
@media (forced-colors: active) {
  .mh-cell, .mh-swatch { forced-color-adjust: none; }
}
@media (max-width: 640px) {
  .flaky-row { grid-template-columns: minmax(0, 1fr) auto; }
  .flaky-seq { grid-column: 1 / -1; }
}
</style>
