// Dashboard insights (GET /projects/{pid}/dashboard/insights): types and the
// chart data built from them. See docs/api.md "Dashboard insights".
import { RESULT_COLORS } from './colors'
import { PROGRESS_ORDER, type ChartData } from './metrics'
import { RISK_LEVELS, RISK_SCALE, levelOfScore } from './risks'

export interface ActivityDay {
  day: string
  changes: number
  executions: number
  defects: number
}

export interface InsightRelease {
  id: string
  name: string
  version: string
  status: string
  target_date: string
  runs: number
  executions: Record<string, number>
  total: number
  executed: number
  passed: number
}

export interface DashboardInsights {
  requirements: { total: number; with_tests: number; passing: number; review: Record<string, number>; by_status: Record<string, number> }
  tests: { total: number; by_type: Record<string, Record<string, number>> }
  risks: {
    total: number
    open: number
    by_level: Record<string, number>
    by_status: Record<string, number>
    /** heat[likelihood-1][impact-1]: risks that are not closed */
    heat: number[][]
    mitigation: Record<string, number>
  }
  defects: { open: number; buckets: Record<string, number>; by_severity: Record<string, Record<string, number>>; average_days: number; oldest_days: number }
  suspect: { links: number; requirements: number }
  trace_rules: { rules: number; failed: number; errors: number; warnings: number; violations: number; passed: boolean }
  release: InsightRelease | null
  activity: { from: string; to: string; days: ActivityDay[] | null; total: ActivityDay }
}

/** Age groups of open defects, youngest first. */
export const AGE_BUCKETS = ['lt7', '7to30', '30to90', 'gt90'] as const
/** Defect severities, most severe first. */
export const SEVERITY_ORDER = ['critical', 'major', 'minor', 'trivial'] as const
/** Review states of requirements on the dashboard. */
export const REVIEW_ORDER = ['none', 'pending', 'changes_requested', 'approved'] as const
/** Mitigation states of open risks (internal/report MitigationStates). */
export const MITIGATION_ORDER = ['no_requirements', 'untested', 'partly_tested', 'tested', 'verified'] as const
export const ACTIVITY_SERIES = ['changes', 'executions', 'defects'] as const

/** Colours that carry meaning: severities and risk levels (red = worst). */
export const SEVERITY_COLORS: Record<string, string> = { critical: '#dc2626', major: '#ea580c', minor: '#d97706', trivial: '#64748b' }
export const RISK_LEVEL_COLORS: Record<string, string> = { critical: '#dc2626', high: '#ea580c', medium: '#d97706', low: '#16a34a' }

/** n / total as 0–100 (0 when total is 0). */
export function share(n: number, total: number): number {
  return total > 0 ? (n * 100) / total : 0
}

/** Open defects by age group, stacked by severity (severities without open defects are left out). */
export function defectAgeChart(
  d: DashboardInsights['defects'] | null | undefined,
  severityLabel: (s: string) => string,
  bucketLabel: (b: string) => string,
): ChartData {
  const by = d?.by_severity ?? {}
  const sevs = [...SEVERITY_ORDER, ...Object.keys(by).filter((s) => !(SEVERITY_ORDER as readonly string[]).includes(s))]
  return {
    labels: AGE_BUCKETS.map(bucketLabel),
    datasets: sevs
      .map((s, i) => ({
        label: severityLabel(s),
        data: AGE_BUCKETS.map((b) => by[s]?.[b] ?? 0),
        backgroundColor: SEVERITY_COLORS[s] ?? ['#0ea5e9', '#8b5cf6'][i % 2],
      }))
      .filter((ds) => ds.data.some((v) => v > 0)),
  }
}

/** One bar per execution type, stacked by last result (in progress counts as not run). */
export function testTypeChart(byType: Record<string, Record<string, number>> | null | undefined, typeLabel: (t: string) => string, resultLabel: (r: string) => string): ChartData {
  const types = Object.keys(byType ?? {}).sort((a, b) => (a === 'manual' ? -1 : b === 'manual' ? 1 : a.localeCompare(b)))
  const counts = types.map((t) => {
    const c = { ...(byType?.[t] ?? {}) }
    c.not_run = (c.not_run ?? 0) + (c.in_progress ?? 0)
    return c
  })
  return {
    labels: types.map(typeLabel),
    datasets: PROGRESS_ORDER.map((r) => ({ label: resultLabel(r), data: counts.map((c) => c[r] ?? 0), backgroundColor: RESULT_COLORS[r] })).filter((ds) => ds.data.some((v) => v > 0)),
  }
}

/** A single-series bar of counts in a fixed order. */
export function countChart(rec: Record<string, number> | null | undefined, order: readonly string[], label: (k: string) => string, color: (k: string, i: number) => string): ChartData {
  return {
    labels: order.map(label),
    datasets: [{ label: '', data: order.map((k) => rec?.[k] ?? 0), backgroundColor: order.map((k, i) => color(k, i)) }],
  }
}

/** Activity per day: changes, executions and defects as three series. */
export function activityChart(days: readonly ActivityDay[] | null | undefined, dayLabel: (d: string) => string, seriesLabel: (s: string) => string, color: (i: number) => string): ChartData {
  const ds = days ?? []
  return {
    labels: ds.map((d) => dayLabel(d.day)),
    datasets: ACTIVITY_SERIES.map((s, i) => ({ label: seriesLabel(s), data: ds.map((d) => d[s]), backgroundColor: color(i), borderColor: color(i) })),
  }
}

export interface MiniHeatCell {
  likelihood: number
  impact: number
  level: string
  count: number
}

/** The heat map from heat[likelihood-1][impact-1]: rows likelihood 5 (top) to 1, cells impact 1 to 5. */
export function heatRows(heat: readonly (readonly number[])[] | null | undefined): { likelihood: number; cells: MiniHeatCell[] }[] {
  return [...RISK_SCALE].reverse().map((l) => ({
    likelihood: l,
    cells: RISK_SCALE.map((i) => ({ likelihood: l, impact: i, level: levelOfScore(l * i), count: heat?.[l - 1]?.[i - 1] ?? 0 })),
  }))
}

/** Risk levels from highest to lowest. */
export const LEVELS_DESC = [...RISK_LEVELS].reverse()

/** Whether any value of a chart is above 0. */
export function anyData(c: ChartData): boolean {
  return c.datasets.some((d) => d.data.some((v) => (v ?? 0) > 0))
}
