import { describe, expect, it } from 'vitest'
import { activityChart, anyData, countChart, defectAgeChart, heatRows, share, testTypeChart, REVIEW_ORDER } from './insights'

const id = (s: string) => s

describe('insights', () => {
  it('computes shares', () => {
    expect(share(1, 4)).toBe(25)
    expect(share(3, 0)).toBe(0)
  })

  it('stacks open defects by age and severity', () => {
    const c = defectAgeChart(
      {
        open: 3,
        buckets: { lt7: 1, '7to30': 0, '30to90': 1, gt90: 1 },
        by_severity: { critical: { lt7: 1 }, minor: { '30to90': 1, gt90: 1 }, major: {}, odd: { gt90: 0 } },
        average_days: 50,
        oldest_days: 120,
      },
      (s) => `S:${s}`,
      (b) => `B:${b}`,
    )
    expect(c.labels).toEqual(['B:lt7', 'B:7to30', 'B:30to90', 'B:gt90'])
    expect(c.datasets.map((d) => d.label)).toEqual(['S:critical', 'S:minor'])
    expect(c.datasets[1].data).toEqual([0, 0, 1, 1])
    expect(defectAgeChart(null, id, id).datasets).toEqual([])
  })

  it('stacks tests by type and result', () => {
    const c = testTypeChart({ automated: { passed: 2, failed: 1 }, manual: { passed: 1, in_progress: 1, not_run: 2 } }, (t) => t.toUpperCase(), id)
    expect(c.labels).toEqual(['MANUAL', 'AUTOMATED'])
    const byLabel = Object.fromEntries(c.datasets.map((d) => [d.label, d.data]))
    expect(byLabel).toEqual({ passed: [1, 2], failed: [0, 1], not_run: [3, 0] })
    expect(anyData(testTypeChart(undefined, id, id))).toBe(false)
  })

  it('counts in a fixed order', () => {
    const c = countChart({ pending: 2, approved: 1 }, REVIEW_ORDER, id, (_, i) => `c${i}`)
    expect(c.labels).toEqual(['none', 'pending', 'changes_requested', 'approved'])
    expect(c.datasets[0].data).toEqual([0, 2, 0, 1])
    expect(c.datasets[0].backgroundColor).toEqual(['c0', 'c1', 'c2', 'c3'])
    expect(anyData(c)).toBe(true)
  })

  it('builds the activity series', () => {
    const c = activityChart(
      [
        { day: '2026-03-01', changes: 1, executions: 0, defects: 2 },
        { day: '2026-03-02', changes: 0, executions: 5, defects: 0 },
      ],
      (d) => d.slice(5),
      (s) => s,
      (i) => `#${i}`,
    )
    expect(c.labels).toEqual(['03-01', '03-02'])
    expect(c.datasets.map((d) => [d.label, d.data])).toEqual([
      ['changes', [1, 0]],
      ['executions', [0, 5]],
      ['defects', [2, 0]],
    ])
    expect(activityChart(null, id, id, String).labels).toEqual([])
  })

  it('lays out the heat map with likelihood 5 on top', () => {
    const heat = [
      [1, 0, 0, 0, 0],
      [0, 0, 0, 0, 0],
      [0, 0, 0, 0, 0],
      [0, 0, 0, 0, 0],
      [0, 0, 0, 2, 3],
    ]
    const rows = heatRows(heat)
    expect(rows.map((r) => r.likelihood)).toEqual([5, 4, 3, 2, 1])
    expect(rows[0].cells[4]).toEqual({ likelihood: 5, impact: 5, level: 'critical', count: 3 })
    expect(rows[0].cells[3].count).toBe(2)
    expect(rows[4].cells[0]).toEqual({ likelihood: 1, impact: 1, level: 'low', count: 1 })
    expect(heatRows(null)[2].cells[2]).toEqual({ likelihood: 3, impact: 3, level: 'medium', count: 0 })
  })
})
