// Dashboard metrics: trends over time, release burn-up, run progress,
// coverage by group and flaky tests (data shaping for the charts; the
// server aggregates, see docs/api.md "Dashboard and metrics").
import { RESULT_COLORS, COVERAGE_COLORS, colorFor } from './colors'
import { summarize } from './summary'

export const TREND_RANGES = [30, 90, 365] as const
export type TrendRange = (typeof TREND_RANGES)[number]

export interface MetricData {
  requirements: number
  covered: number
  coverage: Record<string, number>
  requirement_status?: Record<string, number>
  tests: number
  test_status?: Record<string, number>
  results: Record<string, number>
  executions: Record<string, number>
  open_defects: Record<string, number>
  open_risks?: Record<string, number>
}

export interface TrendPoint {
  day: string
  source: 'recorded' | 'backfill'
  requirements: number
  covered: number
  coverage_pct: number
  passed: number
  tests: number
  executed: number
  pass_rate: number
  open_defects: number
  data: MetricData
}

export interface DefectWeek {
  week: string
  opened: number
  closed: number
}

export interface Trends {
  from: string
  to: string
  days: number
  points: TrendPoint[]
  defect_weeks: DefectWeek[]
}

export interface BurnupPoint {
  day: string
  scope: number
  covered: number
  passed: number
}

export interface Burnup {
  release_id: string
  name: string
  target_date: string
  from: string
  to: string
  points: BurnupPoint[]
}

export interface FlakyResult {
  status: string
  at: string
  run_id: string
  execution_id: string
  flip: boolean
}

export interface FlakyTest {
  test_id: string
  key: string
  title: string
  execution_type: string
  score: number
  window: number
  results: FlakyResult[]
  last_flip: string | null
}

export interface GroupCoverage {
  id: string
  name: string
  dimension?: string
  total: number
  coverage: Record<string, number>
}

/** A chart dataset (the subset of Chart.js the dashboard uses). */
export interface Series {
  label: string
  data: (number | null)[]
  [option: string]: unknown
}

export interface ChartData {
  labels: string[]
  datasets: Series[]
}

const DAY_MS = 86_400_000

function parseDay(day: string): number {
  return Date.parse(day + 'T00:00:00Z')
}

/** The days from `from` to `to` (inclusive, YYYY-MM-DD, UTC). */
export function dayRange(from: string, to: string): string[] {
  const a = parseDay(from)
  const b = parseDay(to)
  if (isNaN(a) || isNaN(b)) return []
  const out: string[] = []
  for (let t = a; t <= b && out.length < 4000; t += DAY_MS) out.push(new Date(t).toISOString().slice(0, 10))
  return out
}

/**
 * Coverage % and pass rate per day of the range; days without a snapshot
 * are null (the line spans the gap).
 */
export function trendSeries(tr: Pick<Trends, 'from' | 'to' | 'points'> | null | undefined): { days: string[]; coverage: (number | null)[]; passRate: (number | null)[] } {
  if (!tr) return { days: [], coverage: [], passRate: [] }
  const byDay = new Map(tr.points.map((p) => [p.day, p]))
  const days = dayRange(tr.from, tr.to)
  return {
    days,
    coverage: days.map((d) => byDay.get(d)?.coverage_pct ?? null),
    passRate: days.map((d) => byDay.get(d)?.pass_rate ?? null),
  }
}

/** The number of days that have data (not null). */
export function countValues(data: readonly (number | null)[]): number {
  return data.filter((v) => v !== null).length
}

export interface SeriesSummary {
  first: number
  last: number
  min: number
  max: number
}

/** First, last, lowest and highest value of a series (null values skipped). */
export function summarizeSeries(data: readonly (number | null)[]): SeriesSummary | null {
  const vals = data.filter((v): v is number => v !== null)
  if (!vals.length) return null
  return { first: vals[0], last: vals[vals.length - 1], min: Math.min(...vals), max: Math.max(...vals) }
}

/** Opened and closed defects per week as two bar series. */
export function weekSeries(weeks: readonly DefectWeek[] | null | undefined): { weeks: string[]; opened: number[]; closed: number[] } {
  const w = weeks ?? []
  return { weeks: w.map((x) => x.week), opened: w.map((x) => x.opened), closed: w.map((x) => x.closed) }
}

/** Scope, covered and passed per day of a burn-up. */
export function burnupSeries(b: Burnup | null | undefined): { days: string[]; scope: number[]; covered: number[]; passed: number[] } {
  const p = b?.points ?? []
  return { days: p.map((x) => x.day), scope: p.map((x) => x.scope), covered: p.map((x) => x.covered), passed: p.map((x) => x.passed) }
}

/** Result statuses of the run progress bars ("not run" includes in progress). */
export const PROGRESS_ORDER = ['passed', 'failed', 'blocked', 'skipped', 'not_run'] as const

/**
 * Stacked bars of run progress: one dataset per result, one bar per run.
 * label names a result (for the legend).
 */
export function runProgress(runs: readonly { name: string; summary?: Record<string, number> }[] | null | undefined, label: (status: string) => string): ChartData {
  const rs = runs ?? []
  const counts = rs.map((r) => {
    const segs = Object.fromEntries(summarize(r.summary).segments.map((s) => [s.status, s.count]))
    segs.not_run = (segs.not_run ?? 0) + (segs.in_progress ?? 0)
    return segs
  })
  return {
    labels: rs.map((r) => r.name),
    datasets: PROGRESS_ORDER.map((st) => ({
      label: label(st),
      data: counts.map((c) => c[st] ?? 0),
      backgroundColor: RESULT_COLORS[st],
    })).filter((d) => d.data.some((v) => (v ?? 0) > 0)),
  }
}

export const COVERAGE_ORDER = ['passed', 'failed', 'partial', 'not_run', 'none'] as const

/**
 * Stacked bars of coverage per folder or category. name labels a group
 * (the empty folder id is "no folder"); label names a coverage state.
 */
export function groupCoverageChart(groups: readonly GroupCoverage[] | null | undefined, name: (g: GroupCoverage) => string, label: (coverage: string) => string): ChartData {
  const gs = groups ?? []
  return {
    labels: gs.map(name),
    datasets: COVERAGE_ORDER.map((c, i) => ({
      label: label(c),
      data: gs.map((g) => g.coverage[c] ?? 0),
      backgroundColor: colorFor(COVERAGE_COLORS, c, i),
    })).filter((d) => d.data.some((v) => (v ?? 0) > 0)),
  }
}

/**
 * Text alternative of a chart with several series: per label the values of
 * every series ("Label: A 1, B 2"), limited to `max` labels (the last ones).
 */
export function describeMulti(title: string, c: ChartData, fmt: (n: number) => string = String, max = 12): string {
  const start = Math.max(0, c.labels.length - max)
  const parts: string[] = []
  for (let i = start; i < c.labels.length; i++) {
    const vals = c.datasets.filter((d) => d.data[i] !== null && d.data[i] !== undefined).map((d) => `${d.label} ${fmt(d.data[i] as number)}`)
    if (vals.length) parts.push(`${c.labels[i]}: ${vals.join(', ')}`)
  }
  return parts.length ? `${title}. ${parts.join('; ')}` : title
}

/** Decisive results (passed / failed) count for flips. */
function decisive(status: string): boolean {
  return status === 'passed' || status === 'failed'
}

/**
 * Executions (any order) whose decisive result differs from the previous
 * decisive result of the same test in time order: the flips of a history.
 */
export function flipIds<T extends { id: string; status: string; finished_at?: string | null; started_at?: string | null; run_created_at?: string | null }>(execs: readonly T[]): Set<string> {
  const when = (e: T) => Date.parse(e.finished_at || e.started_at || e.run_created_at || '') || 0
  const sorted = execs.filter((e) => decisive(e.status)).sort((a, b) => when(a) - when(b) || (a.id < b.id ? -1 : 1))
  const out = new Set<string>()
  for (let i = 1; i < sorted.length; i++) if (sorted[i].status !== sorted[i - 1].status) out.add(sorted[i].id)
  return out
}
