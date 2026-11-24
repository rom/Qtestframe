import { describe, expect, it } from 'vitest'
import { countOf, erasureConfirmed, grandTotals, keepsForever, overrideForm, overrideValue, reportTotals, type RetentionReport } from './privacy'

const report = (counts: [string, number, number][], days: Record<string, number> = {}): RetentionReport => ({
  tenant_id: 't',
  dry_run: true,
  policy: { days, runs_keep_latest: 0 },
  counts: counts.map(([category, count, held]) => ({ category: category as RetentionReport['counts'][number]['category'], days: days[category] ?? 0, count, held })),
  at: '2026-01-01T00:00:00Z',
})

describe('privacy', () => {
  it('sums the counts of reports', () => {
    const a = report([
      ['runs', 3, 1],
      ['logs', 2, 0],
    ])
    const b = report([['notifications', 5, 2]])
    expect(reportTotals(a)).toEqual({ count: 5, held: 1 })
    expect(grandTotals([a, b])).toEqual({ count: 10, held: 3 })
    expect(grandTotals([])).toEqual({ count: 0, held: 0 })
    expect(countOf(a, 'logs')?.count).toBe(2)
    expect(countOf(a, 'chat')).toBeUndefined()
  })

  it('knows a policy that keeps everything', () => {
    expect(keepsForever(report([], {}))).toBe(true)
    expect(keepsForever(report([], { runs: 0, chat: 30 }))).toBe(false)
  })

  it('checks the typed confirmation of an erasure', () => {
    expect(erasureConfirmed(' Anna@Example.com ', 'anna@example.com')).toBe(true)
    expect(erasureConfirmed('anna@example.org', 'anna@example.com')).toBe(false)
    expect(erasureConfirmed('', '')).toBe(false)
  })

  it('converts an organisation override to and from its form', () => {
    const f = overrideForm({ runs_days: 30, bogus: 1 })
    expect(f.runs_days).toBe(30)
    expect(f.logs_days).toBeNull()
    expect(overrideValue(f)).toEqual({ runs_days: 30 })
    f.logs_days = -4
    f.chat_days = 99999999
    f.notifications_days = 1.6
    expect(overrideValue(f)).toEqual({ runs_days: 30, logs_days: 0, chat_days: 36500, notifications_days: 2 })
    expect(overrideValue(overrideForm(null))).toBeUndefined()
  })
})
