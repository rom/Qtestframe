import { describe, expect, it } from 'vitest'
import { QUALITY_KPIS, hoursText, qualityKpis, trendValues } from './quality'
import type { QualityReport } from './types'

const dur = { count: 2, mean_hours: 30, median_hours: 6, p90_hours: 50 }
const report = {
  from: '2026-01-01',
  to: '2026-03-31',
  period: 'week',
  periods: ['2026-01-05'],
  stale_days: 30,
  volatility: { changes: 4, requirements: 10, changed: 2, per_requirement: 0.4, trend: [{ period: '2026-01-05', value: 0.4, count: 4, total: 10 }], most_changed: [] },
  defect_density: { defects: 3, requirements: 10, per_requirement: 0.3, by_severity: [], by_requirement: [], by_folder: [], by_category: [] },
  escaped: { defects: 4, escaped: 1, percent: 25, production: 1, after_release: 0, by_phase: [], by_release: [], trend: [], items: [] },
  effectiveness: { defects: 2, tests: 2, executions: 4, hours: 2, per_test: 1, per_hour: 1, top_tests: [], trend: [], untimed_percent: 0 },
  result_age: { tests: 5, with_result: 4, never: 1, buckets: [], stale_days: 30, stale: 2, stale_percent: 50, median_days: 12, oldest: [] },
  review_turnaround: { ...dur, pending: 1, oldest_pending_days: 2, trend: [] },
  fix_time: { ...dur, open: 3, by_severity: [], trend: [] },
  requirement_quality: { requirements: 10, with_warnings: 3, percent: 30, by_rule: [], trend: [], items: [] },
} as QualityReport

describe('quality metrics', () => {
  it('builds the key figures', () => {
    const k = qualityKpis(report)
    expect(k.map((x) => x.id)).toEqual([...QUALITY_KPIS])
    expect(k[2]).toMatchObject({ value: 25, percent: true, params: { escaped: 1, defects: 4 } })
    expect(k[5]).toMatchObject({ value: 6, hours: true })
    expect(k[7].params).toEqual({ n: 3, total: 10 })
  })
  it('reads trends and formats hours', () => {
    expect(trendValues(report.volatility.trend)).toEqual([0.4])
    expect(trendValues(report.volatility.trend, true)).toEqual([4])
    expect(trendValues(null)).toEqual([])
    const f = (n: number) => String(n)
    expect(hoursText(5.25, f, { h: 'h', d: 'd' })).toBe('5.3 h')
    expect(hoursText(72, f, { h: 'h', d: 'd' })).toBe('3 d')
  })
})
