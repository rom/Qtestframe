<script setup lang="ts">
// A quality metrics chart: one measure, a bar or line chart in the theme's
// colours, a text alternative for screen readers and the same numbers as a
// table (collapsed) for everyone.
import { computed, ref } from 'vue'
import Chart from 'primevue/chart'
import { describeMulti, type ChartData } from '@/lib/metrics'
import { formatNumber } from '@/lib/format'
import { seriesColor } from '@/themes/chart'
import { useChartTheme } from '@/composables/chartTheme'
import { useBreakpoint } from '@/lib/breakpoint'
import { useI18n } from 'vue-i18n'

const props = withDefaults(
  defineProps<{
    title: string
    /** column header of the labels in the table */
    labelHeader: string
    labels: string[]
    series: { label: string; data: (number | null)[] }[]
    type?: 'bar' | 'line'
    horizontal?: boolean
    /** unit appended to values (e.g. " %", " h") */
    unit?: string
    height?: string
  }>(),
  { type: 'bar', horizontal: false, unit: '', height: '220px' },
)
const { t } = useI18n()
const theme = useChartTheme()
const narrow = useBreakpoint('phone')
const showTable = ref(false)
const tableId = `qc-${Math.random().toString(36).slice(2, 10)}`

const fmt = (n: number | null) => (n === null || n === undefined ? '–' : `${formatNumber(n)}${props.unit}`)
const data = computed<ChartData>(() => ({
  labels: props.labels,
  datasets: props.series.map((s, i) => {
    const color = seriesColor(theme.value, i)
    return props.type === 'line'
      ? { ...s, borderColor: color, backgroundColor: color, borderWidth: 2, pointRadius: props.labels.length > 30 ? 0 : 4, pointStyle: i ? 'rectRot' : 'circle', tension: 0.2, spanGaps: true }
      : { ...s, backgroundColor: color, borderRadius: 4, maxBarThickness: 28, borderSkipped: 'start' }
  }),
}))
const any = computed(() => props.series.some((s) => s.data.some((v) => (v ?? 0) > 0)))
const description = computed(() => describeMulti(props.title, data.value, (n) => fmt(n), 40))
const options = computed(() => {
  const c = theme.value
  const font = { family: c.font }
  const tick = { color: c.text, font, maxRotation: 0, autoSkip: true, maxTicksLimit: narrow.value ? 4 : 8 }
  const value = { ticks: { color: c.text, font, precision: 0 }, grid: { color: c.grid }, border: { color: c.grid }, beginAtZero: true }
  const label = { ticks: tick, grid: { display: false }, border: { color: c.grid } }
  return {
    maintainAspectRatio: false,
    indexAxis: props.horizontal ? 'y' : 'x',
    interaction: { mode: 'index', intersect: false },
    plugins: {
      legend: { display: props.series.length > 1, position: 'bottom', labels: { color: c.text, usePointStyle: true, boxWidth: 8, font } },
      tooltip: { titleFont: font, bodyFont: font, callbacks: { label: (ctx: { dataset: { label: string }; raw: number }) => `${ctx.dataset.label}: ${fmt(ctx.raw)}` } },
    },
    scales: props.horizontal ? { x: value, y: label } : { x: label, y: value },
  }
})
</script>

<template>
  <div class="qchart">
    <div v-if="any" class="chart-box" :style="{ height }" role="img" :aria-label="description">
      <Chart :type="type" :data="data" :options="options" style="height: 100%" />
    </div>
    <div v-else class="empty small">{{ t('quality.noData') }}</div>
    <button v-if="any" type="button" class="link-button small table-toggle" :aria-expanded="showTable" :aria-controls="tableId" @click="showTable = !showTable">
      <i :class="showTable ? 'pi pi-chevron-down' : 'pi pi-chevron-right'" aria-hidden="true" /> {{ showTable ? t('quality.hideTable') : t('quality.showTable') }}
    </button>
    <div v-if="any && showTable" :id="tableId" class="table-wrap">
      <table class="data-table small">
        <caption class="sr-only">{{ title }}</caption>
        <thead>
          <tr>
            <th scope="col">{{ labelHeader }}</th>
            <th v-for="s in series" :key="s.label" scope="col" class="num">{{ s.label }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(l, i) in labels" :key="l + i">
            <th scope="row">{{ l }}</th>
            <td v-for="s in series" :key="s.label" class="num">{{ fmt(s.data[i]) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.qchart { display: grid; gap: 4px; }
.table-toggle { justify-self: start; display: inline-flex; gap: 4px; align-items: center; }
.table-wrap { overflow-x: auto; max-height: 260px; overflow-y: auto; }
.data-table { border-collapse: collapse; width: 100%; }
.data-table th, .data-table td { padding: 3px 8px; border-bottom: 1px solid var(--qtf-border); text-align: left; font-weight: normal; }
.data-table thead th { font-weight: 600; }
.data-table .num { text-align: right; }
</style>
