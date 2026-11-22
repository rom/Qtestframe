import { describe, expect, it } from 'vitest'
import { boardColumns, burndownGap, burndownSeries, cycleSeverity, executed, minutesText, overdue, progressSummary, scheduledName, scheduleText, timeline } from './cycles'
import type { CycleBurndownPoint, CycleProgress, TestCycleTest } from './types'

const progress = (p: Partial<CycleProgress> = {}): CycleProgress => ({
  planned: 4, executed: 2, passed: 1, failed: 1, blocked: 0, skipped: 0, in_progress: 1, not_run: 1, not_in_run: 0, unassigned: 1,
  estimated_minutes: 40, remaining_minutes: 20, actual_ms: 60000, estimated_minutes_executed: 20, percent_executed: 50, percent_passed: 25, ...p,
})
const row = (id: string, assignee: string, status: string, est = 10): TestCycleTest => ({
  id, cycle_id: 'c', test_case_id: id, test_key: id, test_title: id, configuration_id: null, assignee_id: assignee, estimate_minutes: est,
  position: 0, added_by: '', added_at: '', execution_id: null, run_id: null, status, finished_at: null, duration_ms: 0,
})

describe('cycles', () => {
  it('statuses and results', () => {
    expect(cycleSeverity('active')).toBe('info')
    expect(cycleSeverity('closed')).toBe('success')
    expect(cycleSeverity('planned')).toBe('secondary')
    expect(executed('blocked')).toBe(true)
    expect(executed('in_progress')).toBe(false)
    expect(executed(undefined)).toBe(false)
  })
  it('formats minutes', () => {
    expect(minutesText(0)).toBe('0 min')
    expect(minutesText(45)).toBe('45 min')
    expect(minutesText(90)).toBe('1 h 30 min')
    expect(minutesText(120, { h: 't', min: 'min' })).toBe('2 t')
    expect(minutesText(null)).toBe('0 min')
  })
  it('summarises progress and finds overdue cycles', () => {
    expect(progressSummary(null)).toEqual({})
    expect(progressSummary(progress())).toMatchObject({ passed: 1, failed: 1, in_progress: 1, not_run: 1, total: 4 })
    expect(overdue({ status: 'active', end_date: '2026-01-01', progress: progress() }, '2026-01-02')).toBe(true)
    expect(overdue({ status: 'closed', end_date: '2026-01-01', progress: progress() }, '2026-01-02')).toBe(false)
    expect(overdue({ status: 'active', end_date: '', progress: progress() }, '2026-01-02')).toBe(false)
    expect(overdue({ status: 'active', end_date: '2026-01-01', progress: progress({ executed: 4 }) }, '2026-01-02')).toBe(false)
  })
  it('builds the board by tester', () => {
    const cols = boardColumns([row('a', 'u2', 'passed'), row('b', '', 'not_run', 5), row('c', 'u1', 'not_run'), row('d', 'u2', 'not_run', 15)], (id) => ({ u1: 'Ann', u2: 'Bob' })[id] ?? '')
    expect(cols.map((c) => c.assignee)).toEqual(['u1', 'u2', ''])
    expect(cols[1].done.map((r) => r.id)).toEqual(['a'])
    expect(cols[1].open.map((r) => r.id)).toEqual(['d'])
    expect(cols[1].estimate).toBe(25)
    expect(cols[1].remaining).toBe(15)
  })
  it('makes burndown series and the gap to the ideal line', () => {
    const pts: CycleBurndownPoint[] = [
      { day: '2026-01-01', remaining: 4, remaining_minutes: 40, ideal: 4, ideal_minutes: 40 },
      { day: '2026-01-02', remaining: 3, remaining_minutes: 30, ideal: 2, ideal_minutes: 20 },
      { day: '2026-01-03', remaining: null, remaining_minutes: null, ideal: 0, ideal_minutes: 0 },
    ]
    expect(burndownSeries(pts)).toEqual({ days: ['2026-01-01', '2026-01-02', '2026-01-03'], remaining: [4, 3, null], ideal: [4, 2, 0] })
    expect(burndownSeries(pts, true).remaining).toEqual([40, 30, null])
    expect(burndownSeries(null).days).toEqual([])
    expect(burndownGap(pts)).toBe(1)
    expect(burndownGap(pts, true)).toBe(10)
    expect(burndownGap([])).toBeNull()
  })
  it('names scheduled cycles and describes schedules', () => {
    expect(scheduledName('Nightly {date}', '2026-02-03', 4)).toBe('Nightly 2026-02-03')
    expect(scheduledName('Sprint {n}', '2026-02-03', 4)).toBe('Sprint 4')
    expect(scheduledName('Weekly', '2026-02-03', 1)).toBe('Weekly 2026-02-03')
    const t = (k: string, p?: Record<string, unknown>) => `${k} ${JSON.stringify(p)}`
    expect(scheduleText({ frequency: 'daily', weekday: 1, day_of_month: 1, time: '06:00', timezone: 'UTC' }, t, String)).toContain('cycles.schedule.daily')
    expect(scheduleText({ frequency: 'weekly', weekday: 1, day_of_month: 1, time: '06:00', timezone: 'UTC' }, t, (d) => `day${d}`)).toContain('day1')
    expect(scheduleText({ frequency: 'monthly', weekday: 1, day_of_month: 15, time: '06:00', timezone: 'UTC' }, t, String)).toContain('"day":15')
  })
  it('lays out a timeline', () => {
    expect(timeline([{ id: 'x', name: 'X', status: 'planned', start_date: '', end_date: '' }]).bars).toEqual([])
    const tl = timeline([
      { id: 'b', name: 'B', status: 'active', start_date: '2026-01-06', end_date: '2026-01-10' },
      { id: 'a', name: 'A', status: 'closed', start_date: '2026-01-01', end_date: '2026-01-05' },
      { id: 'c', name: 'C', status: 'planned', start_date: '', end_date: '2026-01-10' },
    ])
    expect(tl.from).toBe('2026-01-01')
    expect(tl.to).toBe('2026-01-10')
    expect(tl.bars.map((b) => b.id)).toEqual(['a', 'b', 'c'])
    expect(tl.bars[0]).toMatchObject({ left: 0, width: 50 })
    expect(tl.bars[1]).toMatchObject({ left: 50, width: 50 })
    expect(tl.bars[2]).toMatchObject({ left: 90, width: 10 })
  })
})
