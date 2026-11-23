// Chart options and data helpers of the dashboard widgets, in the colours
// and font of the active theme (the former fixed dashboard's).
import { computed } from 'vue'
import { useChartTheme } from '@/composables/chartTheme'
import { useBreakpoint } from '@/lib/breakpoint'
import { entriesByCount } from '@/lib/collections'
import { colorFor } from '@/lib/colors'
import { formatNumber } from '@/lib/format'
import { seriesColor } from '@/themes/chart'

export interface CountChart {
  labels: string[]
  datasets: { data: number[]; backgroundColor: string[]; borderRadius?: number; maxBarThickness?: number; borderWidth?: number; borderColor?: string }[]
}

export function useDashboardCharts() {
  const theme = useChartTheme()
  const narrow = useBreakpoint('phone')
  const font = () => ({ family: theme.value.font })
  const legendOptions = (show: boolean) => ({ display: show, position: 'bottom', labels: { color: theme.value.text, usePointStyle: true, boxWidth: 8, font: font() } })
  const axis = (extra: Record<string, unknown> = {}) => {
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
  /** Stacked bars over days (activity). */
  function stackedDaysOptions() {
    const c = theme.value
    return {
      ...baseOptions(true, 'x', true),
      scales: {
        x: { stacked: true, ticks: { color: c.text, font: font(), maxRotation: 0, autoSkip: true, maxTicksLimit: narrow.value ? 4 : 8 }, grid: { display: false }, border: { color: c.grid } },
        y: axis({ stacked: true, beginAtZero: true }),
      },
    }
  }
  /**
   * Counts as a chart: colours with meaning (results, coverage, severity)
   * when given, else the theme's categorical ones; doughnuts leave out zeros.
   */
  function counts(rec: Record<string, number> | undefined, label: (k: string) => string, opts: { order?: readonly string[]; colors?: Record<string, string>; doughnut?: boolean } = {}): CountChart {
    let entries = entriesByCount(rec, opts.order)
    if (opts.doughnut) entries = entries.filter(([, v]) => v > 0)
    const bg = entries.map(([k], i) => (opts.colors ? colorFor(opts.colors, k, i) : seriesColor(theme.value, i)))
    return {
      labels: entries.map(([k]) => label(k)),
      datasets: [
        opts.doughnut
          ? { data: entries.map(([, v]) => v), backgroundColor: bg, borderWidth: 2, borderColor: theme.value.surface }
          : { data: entries.map(([, v]) => v), backgroundColor: bg, borderRadius: 6, maxBarThickness: 34 },
      ],
    }
  }
  return { theme, narrow, baseOptions, doughnutOptions, lineOptions, stackedDaysOptions, counts }
}

/** Text alternative of a single-series chart for screen readers. */
export function describeCounts(title: string, c: { labels: string[]; datasets: { data: (number | null)[] }[] }): string {
  const parts = c.labels.map((l, i) => `${l}: ${formatNumber(c.datasets[0]?.data[i] ?? 0)}`)
  return parts.length ? `${title}. ${parts.join(', ')}` : title
}

export const hasCounts = (c: { datasets: { data: (number | null)[] }[] }) => c.datasets.some((d) => d.data.some((v) => (v ?? 0) > 0))
