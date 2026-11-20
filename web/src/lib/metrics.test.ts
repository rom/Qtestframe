import { describe, expect, it } from 'vitest'
import {
  burnupSeries,
  countValues,
  dayRange,
  describeMulti,
  flipIds,
  groupCoverageChart,
  runProgress,
  summarizeSeries,
  trendSeries,
  weekSeries,
  type TrendPoint,
} from './metrics'

const point = (day: string, coverage: number, pass: number): TrendPoint => ({
  day,
  source: 'recorded',
  requirements: 2,
  covered: 1,
  coverage_pct: coverage,
  passed: 1,
  tests: 2,
  executed: 2,
  pass_rate: pass,
  open_defects: 0,
  data: { requirements: 2, covered: 1, coverage: {}, tests: 2, results: {}, executions: {}, open_defects: {} },
})

describe('dayRange', () => {
  it('lists the days inclusive', () => {
    expect(dayRange('2026-02-27', '2026-03-02')).toEqual(['2026-02-27', '2026-02-28', '2026-03-01', '2026-03-02'])
    expect(dayRange('2026-03-02', '2026-03-01')).toEqual([])
    expect(dayRange('x', '2026-03-01')).toEqual([])
  })
})

describe('trendSeries', () => {
  it('fills missing days with null', () => {
    const s = trendSeries({ from: '2026-03-01', to: '2026-03-03', points: [point('2026-03-01', 50, 80), point('2026-03-03', 75, 90)] })
    expect(s.days).toEqual(['2026-03-01', '2026-03-02', '2026-03-03'])
    expect(s.coverage).toEqual([50, null, 75])
    expect(s.passRate).toEqual([80, null, 90])
    expect(countValues(s.coverage)).toBe(2)
  })
  it('handles no data', () => {
    expect(trendSeries(null)).toEqual({ days: [], coverage: [], passRate: [] })
  })
})

describe('summarizeSeries', () => {
  it('summarises values', () => {
    expect(summarizeSeries([null, 3, 1, 5, null, 4])).toEqual({ first: 3, last: 4, min: 1, max: 5 })
    expect(summarizeSeries([null])).toBeNull()
  })
})

describe('weekSeries and burnupSeries', () => {
  it('splits into series', () => {
    expect(weekSeries([{ week: '2026-03-02', opened: 2, closed: 1 }])).toEqual({ weeks: ['2026-03-02'], opened: [2], closed: [1] })
    expect(weekSeries(undefined)).toEqual({ weeks: [], opened: [], closed: [] })
    const b = burnupSeries({ release_id: 'r', name: 'R', target_date: '', from: 'a', to: 'b', points: [{ day: '2026-03-01', scope: 3, covered: 2, passed: 1 }] })
    expect(b).toEqual({ days: ['2026-03-01'], scope: [3], covered: [2], passed: [1] })
    expect(burnupSeries(null).days).toEqual([])
  })
})

describe('runProgress', () => {
  it('stacks results per run and drops empty results', () => {
    const c = runProgress(
      [
        { name: 'A', summary: { total: 5, passed: 2, failed: 1, in_progress: 1 } },
        { name: 'B', summary: { blocked: 1 } },
        { name: 'C' },
      ],
      (s) => s.toUpperCase(),
    )
    expect(c.labels).toEqual(['A', 'B', 'C'])
    expect(c.datasets.map((d) => d.label)).toEqual(['PASSED', 'FAILED', 'BLOCKED', 'NOT_RUN'])
    expect(c.datasets.find((d) => d.label === 'NOT_RUN')!.data).toEqual([2, 0, 0])
    expect(c.datasets[0].backgroundColor).toBe('#22c55e')
    expect(runProgress(null, String)).toEqual({ labels: [], datasets: [] })
  })
})

describe('groupCoverageChart', () => {
  it('stacks coverage per group', () => {
    const c = groupCoverageChart(
      [
        { id: 'f', name: 'Auth', total: 3, coverage: { passed: 2, none: 1 } },
        { id: '', name: '', total: 1, coverage: { failed: 1 } },
      ],
      (g) => g.name || '(none)',
      (s) => s,
    )
    expect(c.labels).toEqual(['Auth', '(none)'])
    expect(c.datasets.map((d) => d.label)).toEqual(['passed', 'failed', 'none'])
    expect(c.datasets[2].data).toEqual([1, 0])
    expect(groupCoverageChart(undefined, (g) => g.name, String).labels).toEqual([])
  })
})

describe('describeMulti', () => {
  it('describes every series per label', () => {
    const c = { labels: ['a', 'b', 'c'], datasets: [{ label: 'X', data: [1, null, 3] }, { label: 'Y', data: [2, null, 4] }] }
    expect(describeMulti('T', c)).toBe('T. a: X 1, Y 2; c: X 3, Y 4')
    expect(describeMulti('T', c, (n) => `${n} %`, 1)).toBe('T. c: X 3 %, Y 4 %')
    expect(describeMulti('T', { labels: [], datasets: [] })).toBe('T')
  })
})

describe('flipIds', () => {
  it('marks results that differ from the previous decisive one', () => {
    const e = (id: string, status: string, at: string) => ({ id, status, finished_at: at })
    const ids = flipIds([
      e('4', 'passed', '2026-03-04T00:00:00Z'),
      e('1', 'passed', '2026-03-01T00:00:00Z'),
      e('2', 'blocked', '2026-03-02T00:00:00Z'),
      e('3', 'failed', '2026-03-03T00:00:00Z'),
      e('5', 'passed', '2026-03-05T00:00:00Z'),
      { id: '0', status: 'passed', run_created_at: '2026-02-01T00:00:00Z' },
      { id: '9', status: 'not_run' },
    ])
    expect([...ids].sort()).toEqual(['3', '4'])
    // ties are ordered by id
    expect([...flipIds([e('b', 'failed', '2026-03-01T00:00:00Z'), e('a', 'passed', '2026-03-01T00:00:00Z')])]).toEqual(['b'])
    expect(flipIds([]).size).toBe(0)
  })
})
