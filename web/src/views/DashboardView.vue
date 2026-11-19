<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Chart from 'primevue/chart'
import Skeleton from 'primevue/skeleton'
import Button from 'primevue/button'
import SummaryBar from '@/components/SummaryBar.vue'
import StatusTag from '@/components/StatusTag.vue'
import { api } from '@/lib/api'
import { COVERAGE_COLORS, OBLIGATION_COLORS, RESULT_COLORS, STATUS_COLORS, colorFor } from '@/lib/colors'
import { entriesByCount } from '@/lib/collections'
import { humanize, percent, relativeTime } from '@/lib/format'
import { RESULT_ORDER, summarize } from '@/lib/summary'
import type { Dashboard } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useMetaStore } from '@/stores/meta'
import { useChartTheme } from '@/composables/chartTheme'
import { useNotify } from '@/composables/notify'

const proj = useProjectStore()
const meta = useMetaStore()
const theme = useChartTheme()
const notify = useNotify()
const data = ref<Dashboard | null>(null)
const loading = ref(true)

async function load() {
  loading.value = true
  try {
    data.value = await api.get<Dashboard>(`/projects/${proj.pid}/dashboard`)
  } catch (e) {
    notify.fail(e, 'Could not load dashboard')
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
    { label: 'Requirements', value: reqTotal.value, sub: `${d?.requirements.uncovered ?? 0} uncovered`, icon: 'pi pi-list-check', color: '#6366f1' },
    { label: 'Coverage', value: coveredPct.value + '%', sub: `${d?.requirements.covered ?? 0} covered by tests`, icon: 'pi pi-shield', color: '#22c55e' },
    {
      label: 'Tests',
      value: d?.tests.total ?? 0,
      sub: `${d?.tests.by_execution_type?.manual ?? 0} manual · ${d?.tests.by_execution_type?.automated ?? 0} automated`,
      icon: 'pi pi-check-square',
      color: '#0ea5e9',
    },
    { label: 'Test runs', value: d?.runs.total ?? 0, sub: `${d?.runs.by_status?.in_progress ?? 0} in progress`, icon: 'pi pi-play-circle', color: '#f59e0b' },
    { label: 'Pass rate', value: results.value.passRate + '%', sub: `${results.value.executed} executions`, icon: 'pi pi-chart-line', color: '#ec4899' },
  ]
})

function baseOptions(legend = false, indexAxis: 'x' | 'y' = 'x') {
  const t = theme.value
  return {
    maintainAspectRatio: false,
    indexAxis,
    plugins: { legend: { display: legend, position: 'bottom', labels: { color: t.text, usePointStyle: true, boxWidth: 8 } } },
    scales: {
      x: { ticks: { color: t.text, precision: 0 }, grid: { color: t.grid } },
      y: { ticks: { color: t.text, precision: 0 }, grid: { color: t.grid }, beginAtZero: true },
    },
  }
}
const doughnutOptions = computed(() => ({
  maintainAspectRatio: false,
  cutout: '62%',
  plugins: { legend: { position: 'right', labels: { color: theme.value.text, usePointStyle: true, boxWidth: 8 } } },
}))

function bar(rec: Record<string, number> | undefined, colors: Record<string, string>, order?: readonly string[]) {
  const entries = entriesByCount(rec, order)
  return {
    labels: entries.map(([k]) => humanize(k)),
    datasets: [{ data: entries.map(([, v]) => v), backgroundColor: entries.map(([k], i) => colorFor(colors, k, i)), borderRadius: 6, maxBarThickness: 34 }],
  }
}
function doughnut(rec: Record<string, number> | undefined, colors: Record<string, string>, order?: readonly string[]) {
  const entries = entriesByCount(rec, order).filter(([, v]) => v > 0)
  return {
    labels: entries.map(([k]) => humanize(k)),
    datasets: [{ data: entries.map(([, v]) => v), backgroundColor: entries.map(([k], i) => colorFor(colors, k, i)), borderWidth: 0 }],
  }
}

const coverageChart = computed(() => doughnut(data.value?.coverage, COVERAGE_COLORS, ['passed', 'failed', 'partial', 'not_run', 'none']))
const statusChart = computed(() => bar(data.value?.requirements.by_status, STATUS_COLORS, meta.meta.requirement_statuses))
const obligationChart = computed(() => bar(data.value?.requirements.by_obligation, OBLIGATION_COLORS, meta.meta.obligations))
const resultsChart = computed(() => doughnut(data.value?.results, RESULT_COLORS, RESULT_ORDER))
const testStatusChart = computed(() => bar(data.value?.tests.by_status, STATUS_COLORS, meta.meta.test_statuses))
const hasData = (c: { datasets: { data: number[] }[] }) => c.datasets[0].data.some((v) => v > 0)
</script>

<template>
  <div class="page-header">
    <div>
      <h1>Dashboard</h1>
      <div class="sub">{{ proj.project?.description }}</div>
    </div>
    <div class="actions"><Button icon="pi pi-refresh" text rounded severity="secondary" aria-label="Refresh" @click="load" /></div>
  </div>

  <div v-if="loading && !data" class="grid">
    <div class="kpis"><Skeleton v-for="i in 5" :key="i" height="78px" /></div>
    <div class="charts"><Skeleton v-for="i in 4" :key="i" height="280px" /></div>
  </div>
  <div v-else-if="data" class="grid">
    <div class="kpis">
      <div v-for="k in kpis" :key="k.label" class="card kpi">
        <div class="icon" :style="{ background: k.color + '1f', color: k.color }"><i :class="k.icon" /></div>
        <div>
          <div class="value">{{ k.value }}</div>
          <div class="label">{{ k.label }}</div>
          <div class="small muted">{{ k.sub }}</div>
        </div>
      </div>
    </div>

    <div class="charts">
      <div class="card">
        <h3>Requirement coverage</h3>
        <div style="height: 240px">
          <Chart v-if="hasData(coverageChart)" type="doughnut" :data="coverageChart" :options="doughnutOptions" style="height: 100%" />
          <div v-else class="empty">No requirements yet.</div>
        </div>
      </div>
      <div class="card">
        <h3>Execution results</h3>
        <div style="height: 240px">
          <Chart v-if="hasData(resultsChart)" type="doughnut" :data="resultsChart" :options="doughnutOptions" style="height: 100%" />
          <div v-else class="empty">No executions yet.</div>
        </div>
      </div>
      <div class="card">
        <h3>Requirements by status</h3>
        <div style="height: 240px"><Chart type="bar" :data="statusChart" :options="baseOptions()" style="height: 100%" /></div>
      </div>
      <div class="card">
        <h3>Requirements by obligation</h3>
        <div style="height: 240px"><Chart type="bar" :data="obligationChart" :options="baseOptions(false, 'y')" style="height: 100%" /></div>
      </div>
      <div class="card">
        <h3>Tests by status</h3>
        <div style="height: 240px"><Chart type="bar" :data="testStatusChart" :options="baseOptions()" style="height: 100%" /></div>
      </div>
      <div class="card">
        <div class="row" style="justify-content: space-between; margin-bottom: 8px">
          <h3 style="margin: 0">Recent runs</h3>
          <RouterLink :to="{ name: 'runs', params: { projectId: proj.pid } }" class="small">All runs →</RouterLink>
        </div>
        <div v-if="!(data.recent_runs ?? []).length" class="empty">No runs yet.</div>
        <RouterLink
          v-for="r in data.recent_runs ?? []"
          :key="r.id"
          :to="{ name: 'run', params: { projectId: proj.pid, runId: r.id } }"
          style="display: block; padding: 8px 0; border-bottom: 1px solid var(--qtf-border); color: inherit; text-decoration: none"
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
      </div>
    </div>
  </div>
</template>
