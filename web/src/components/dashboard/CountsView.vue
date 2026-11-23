<script setup lang="ts">
// Counts per value as a bar, horizontal bar or doughnut chart, or a table
// (the chart setting of a widget). Charts carry a text alternative.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Chart from 'primevue/chart'
import { describeCounts, hasCounts, useDashboardCharts } from '@/composables/dashboardCharts'
import { formatNumber, percent } from '@/lib/format'
import type { ChartKind } from '@/lib/widgets'

const props = defineProps<{
  rec: Record<string, number> | undefined
  title: string
  label: (k: string) => string
  chart: ChartKind | string
  order?: readonly string[]
  colors?: Record<string, string>
  empty: string
}>()
const { t } = useI18n()
const charts = useDashboardCharts()
const kind = computed(() => (['bar', 'hbar', 'doughnut', 'table'].includes(props.chart) ? props.chart : 'bar'))
const data = computed(() => charts.counts(props.rec, props.label, { order: props.order, colors: props.colors, doughnut: kind.value === 'doughnut' }))
const rows = computed(() => {
  const total = data.value.datasets[0].data.reduce((n, v) => n + v, 0)
  return data.value.labels.map((l, i) => ({ label: l, n: data.value.datasets[0].data[i], pct: percent(data.value.datasets[0].data[i], total) }))
})
const options = computed(() => (kind.value === 'doughnut' ? charts.doughnutOptions.value : charts.baseOptions(false, kind.value === 'hbar' ? 'y' : 'x')))
</script>

<template>
  <div v-if="!hasCounts(data)" class="empty">{{ empty }}</div>
  <table v-else-if="kind === 'table'" class="count-table">
    <caption class="sr-only">{{ title }}</caption>
    <thead>
      <tr>
        <th scope="col">{{ t('dashboards.value') }}</th>
        <th scope="col" class="num">{{ t('dashboards.count') }}</th>
        <th scope="col" class="num">%</th>
      </tr>
    </thead>
    <tbody>
      <tr v-for="r in rows" :key="r.label">
        <th scope="row">{{ r.label }}</th>
        <td class="num">{{ formatNumber(r.n) }}</td>
        <td class="num">{{ formatNumber(r.pct) }}</td>
      </tr>
    </tbody>
  </table>
  <div v-else role="img" :aria-label="describeCounts(title, data)" class="chart-fill">
    <Chart :type="kind === 'doughnut' ? 'doughnut' : 'bar'" :data="data" :options="options" style="height: 100%" />
  </div>
</template>

<style scoped>
.count-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.count-table th, .count-table td { text-align: left; padding: 4px 6px; border-bottom: 1px solid var(--qtf-border); }
.count-table th[scope='row'] { font-weight: 500; }
.count-table .num { text-align: right; font-variant-numeric: tabular-nums; }
</style>
