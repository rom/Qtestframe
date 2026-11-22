// Test cycles: statuses, progress figures, the board by tester, the
// burndown series and the schedule texts (mirrors internal/model/cycles.go).
import type { CycleBurndownPoint, CycleProgress, CycleSchedule, TestCycle, TestCycleTest } from './types'

export const CYCLE_STATUSES = ['planned', 'active', 'closed'] as const
export type CycleStatus = (typeof CYCLE_STATUSES)[number]

/** Tag severity of a cycle status. */
export function cycleSeverity(status: string): 'secondary' | 'info' | 'success' {
  return status === 'active' ? 'info' : status === 'closed' ? 'success' : 'secondary'
}

/** Statuses a planned test counts as executed with (a final result). */
export const FINAL_RESULTS = ['passed', 'failed', 'blocked', 'skipped'] as const

export function executed(status: string | null | undefined): boolean {
  return (FINAL_RESULTS as readonly string[]).includes(status ?? '')
}

/** Minutes as "1 h 30 min" / "45 min" / "0 min" (units translated by the caller). */
export function minutesText(min: number | null | undefined, units: { h: string; min: string } = { h: 'h', min: 'min' }): string {
  const m = Math.max(0, Math.round(min ?? 0))
  const h = Math.floor(m / 60)
  const rest = m % 60
  if (!h) return `${rest} ${units.min}`
  return rest ? `${h} ${units.h} ${rest} ${units.min}` : `${h} ${units.h}`
}

/** Result counts of a progress as a summary (for SummaryBar: status → count). */
export function progressSummary(p: CycleProgress | null | undefined): Record<string, number> {
  if (!p) return {}
  const out: Record<string, number> = {
    passed: p.passed,
    failed: p.failed,
    blocked: p.blocked,
    skipped: p.skipped,
    in_progress: p.in_progress,
    not_run: p.not_run,
  }
  out.total = p.planned
  return out
}

/** Whether a cycle's end date has passed while tests are still open. */
export function overdue(c: Pick<TestCycle, 'status' | 'end_date' | 'progress'>, today: string): boolean {
  return c.status !== 'closed' && !!c.end_date && c.end_date < today && c.progress.executed < c.progress.planned
}

export interface TesterColumn {
  /** user id ("" = not assigned) */
  assignee: string
  rows: TestCycleTest[]
  open: TestCycleTest[]
  done: TestCycleTest[]
  estimate: number
  remaining: number
}

/**
 * The board by tester: one column per tester (in the order of order, then
 * by name via label, the unassigned column last) with the open and the
 * executed tests.
 */
export function boardColumns(plan: readonly TestCycleTest[], label: (id: string) => string): TesterColumn[] {
  const by = new Map<string, TesterColumn>()
  for (const r of plan) {
    let c = by.get(r.assignee_id)
    if (!c) {
      c = { assignee: r.assignee_id, rows: [], open: [], done: [], estimate: 0, remaining: 0 }
      by.set(r.assignee_id, c)
    }
    c.rows.push(r)
    c.estimate += r.estimate_minutes
    if (executed(r.status)) c.done.push(r)
    else {
      c.open.push(r)
      c.remaining += r.estimate_minutes
    }
  }
  return [...by.values()].sort((a, b) => {
    if (!a.assignee !== !b.assignee) return a.assignee ? -1 : 1
    return label(a.assignee).localeCompare(label(b.assignee))
  })
}

export interface BurndownSeries {
  days: string[]
  remaining: (number | null)[]
  ideal: number[]
}

/** The burndown as chart series: tests or estimated minutes. */
export function burndownSeries(points: readonly CycleBurndownPoint[] | null | undefined, minutes = false): BurndownSeries {
  const p = points ?? []
  return {
    days: p.map((x) => x.day),
    remaining: p.map((x) => (minutes ? x.remaining_minutes : x.remaining)),
    ideal: p.map((x) => (minutes ? x.ideal_minutes : x.ideal)),
  }
}

/**
 * How the latest actual value compares with the ideal line on that day:
 * > 0 behind schedule (more remaining than ideal), < 0 ahead, null unknown.
 */
export function burndownGap(points: readonly CycleBurndownPoint[] | null | undefined, minutes = false): number | null {
  const p = [...(points ?? [])].reverse().find((x) => (minutes ? x.remaining_minutes : x.remaining) !== null)
  if (!p) return null
  const actual = (minutes ? p.remaining_minutes : p.remaining) ?? 0
  return Math.round((actual - (minutes ? p.ideal_minutes : p.ideal)) * 10) / 10
}

/** The name a schedule gives its next cycle on day (YYYY-MM-DD). */
export function scheduledName(pattern: string, day: string, n: number): string {
  const name = pattern.replaceAll('{date}', day).replaceAll('{n}', String(n))
  return name === pattern && !pattern.includes('{') ? `${pattern} ${day}` : name
}

/** Weekday names, Sunday first (as the e-mailed reports). */
export type ScheduleText = (key: string, params?: Record<string, unknown>) => string

/** One-line description of a schedule, e.g. "Weekly on Monday at 06:00 (UTC)". */
export function scheduleText(s: Pick<CycleSchedule, 'frequency' | 'weekday' | 'day_of_month' | 'time' | 'timezone'>, t: ScheduleText, weekday: (d: number) => string): string {
  const at = { time: s.time, tz: s.timezone }
  if (s.frequency === 'daily') return t('cycles.schedule.daily', at)
  if (s.frequency === 'monthly') return t('cycles.schedule.monthly', { ...at, day: s.day_of_month })
  return t('cycles.schedule.weekly', { ...at, day: weekday(s.weekday) })
}

export interface TimelineBar {
  id: string
  name: string
  status: string
  /** offset and width in percent of the range */
  left: number
  width: number
  start: string
  end: string
}

const DAY = 86_400_000
const dayNum = (d: string) => Math.floor(Date.parse(`${d}T00:00:00Z`) / DAY)

/**
 * A timeline of cycles with dates: bars over the range from the earliest
 * start to the latest end (a cycle without end date ends on its start
 * date, without start date it starts on its end date).
 */
export function timeline(cycles: readonly Pick<TestCycle, 'id' | 'name' | 'status' | 'start_date' | 'end_date'>[]): { from: string; to: string; bars: TimelineBar[] } {
  const dated = cycles.filter((c) => c.start_date || c.end_date)
  if (!dated.length) return { from: '', to: '', bars: [] }
  const span = dated.map((c) => ({ c, s: c.start_date || c.end_date, e: c.end_date || c.start_date }))
  const from = span.reduce((m, x) => (x.s < m ? x.s : m), span[0].s)
  const to = span.reduce((m, x) => (x.e > m ? x.e : m), span[0].e)
  const total = dayNum(to) - dayNum(from) + 1
  const bars = span
    .sort((a, b) => a.s.localeCompare(b.s) || a.c.name.localeCompare(b.c.name))
    .map(({ c, s, e }) => ({
      id: c.id,
      name: c.name,
      status: c.status,
      start: s,
      end: e,
      left: Math.round(((dayNum(s) - dayNum(from)) / total) * 1000) / 10,
      width: Math.max(1, Math.round(((dayNum(e) - dayNum(s) + 1) / total) * 1000) / 10),
    }))
  return { from, to, bars }
}
