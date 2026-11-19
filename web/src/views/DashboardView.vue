<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Chart from 'primevue/chart'
import Skeleton from 'primevue/skeleton'
import Button from 'primevue/button'
import SummaryBar from '@/components/SummaryBar.vue'
import StatusTag from '@/components/StatusTag.vue'
import { api } from '@/lib/api'
import { COVERAGE_COLORS, RESULT_COLORS, colorFor } from '@/lib/colors'
import { seriesColor } from '@/themes/chart'
import { useBreakpoint } from '@/lib/breakpoint'
import { entriesByCount } from '@/lib/collections'
import { formatNumber, percent, relativeTime } from '@/lib/format'
import { useI18n } from 'vue-i18n'
import { enumLabel, type EnumGroup } from '@/i18n/labels'
import { RESULT_ORDER, summarize } from '@/lib/summary'
import type { Dashboard } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useMetaStore } from '@/stores/meta'
import { useChartTheme } from '@/composables/chartTheme'
import { useNotify } from '@/composables/notify'

const proj = useProjectStore()
const meta = useMetaStore()
const theme = useChartTheme()
const narrow = useBreakpoint('phone')
const notify = useNotify()
const { t } = useI18n()
const data = ref<Dashboard | null>(null)
const loading = ref(true)

async function load() {
  loading.value = true
  try {
    data.value = await api.get<Dashboard>(`/projects/${proj.pid}/dashboard`)
  } catch (e) {
    notify.fail(e, t('dashboard.loadFailed'))
  } finally {
    loading.value = false
  }
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
  ]
})

function font() {
  return { family: theme.value.font }
}
function baseOptions(legend = false, indexAxis: 'x' | 'y' = 'x') {
  const c = theme.value
  return {
    maintainAspectRatio: false,
    indexAxis,
    plugins: { legend: { display: legend, position: 'bottom', labels: { color: c.text, usePointStyle: true, boxWidth: 8, font: font() } }, tooltip: { titleFont: font(), bodyFont: font() } },
    scales: {
      x: { ticks: { color: c.text, precision: 0, font: font() }, grid: { color: c.grid }, border: { color: c.grid } },
      y: { ticks: { color: c.text, precision: 0, font: font() }, grid: { color: c.grid }, border: { color: c.grid }, beginAtZero: true },
    },
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
function describe(title: string, c: { labels: string[]; datasets: { data: number[] }[] }) {
  const parts = c.labels.map((l, i) => `${l}: ${formatNumber(c.datasets[0].data[i] ?? 0)}`)
  return parts.length ? `${title}. ${parts.join(', ')}` : title
}

const coverageChart = computed(() => doughnut('coverage', data.value?.coverage, COVERAGE_COLORS, ['passed', 'failed', 'partial', 'not_run', 'none']))
const statusChart = computed(() => bar('status', data.value?.requirements.by_status, meta.meta.requirement_statuses))
const obligationChart = computed(() => bar('obligation', data.value?.requirements.by_obligation, meta.meta.obligations))
const resultsChart = computed(() => doughnut('result', data.value?.results, RESULT_COLORS, RESULT_ORDER))
const testStatusChart = computed(() => bar('status', data.value?.tests.by_status, meta.meta.test_statuses))
const hasData = (c: { datasets: { data: number[] }[] }) => c.datasets[0].data.some((v) => v > 0)
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('dashboard.title') }}</h1>
      <div class="sub">{{ proj.project?.description }}</div>
    </div>
    <div class="actions"><Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="load" /></div>
  </div>

  <div v-if="loading && !data" class="grid" aria-busy="true" :aria-label="t('common.loading')">
    <div class="kpis"><Skeleton v-for="i in 5" :key="i" height="78px" /></div>
    <div class="charts"><Skeleton v-for="i in 4" :key="i" height="280px" /></div>
  </div>
  <div v-else-if="data" class="grid" :aria-busy="loading">
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
    </div>
  </div>
</template>
