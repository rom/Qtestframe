import { describe, expect, it } from 'vitest'
import { filterRisks, heatGrid, highestLevel, levelCounts, levelOfScore, levelRank, riskLevel, riskLevelSeverity, riskOpen, riskScore, riskStatusSeverity } from './risks'
import type { Risk } from './types'

const risk = (p: Partial<Risk>): Risk => ({
  id: 'r', project_id: 'p', key: 'P-RISK-1', title: 'Outage', description: '', category: '', likelihood: 3, impact: 3, status: 'identified',
  mitigation: '', owner: '', created_by: '', created_at: '', updated_at: '', score: 9, level: 'medium', requirement_ids: [], covered: 0, verified: 0, ...p,
})

describe('risk scores and levels', () => {
  it('multiplies likelihood and impact within 1–5', () => {
    expect(riskScore(3, 4)).toBe(12)
    expect(riskScore(0, 9)).toBe(5)
    expect(riskScore(Number.NaN, 2)).toBe(2)
  })
  it('maps scores to levels at the boundaries', () => {
    expect([1, 4, 5, 9, 10, 16, 17, 25].map(levelOfScore)).toEqual(['low', 'low', 'medium', 'medium', 'high', 'high', 'critical', 'critical'])
    expect(riskLevel(5, 4)).toBe('critical')
    expect(riskLevel(2, 2)).toBe('low')
  })
  it('colours levels and statuses', () => {
    expect(['low', 'medium', 'high', 'critical', ''].map(riskLevelSeverity)).toEqual(['success', 'info', 'warn', 'danger', 'secondary'])
    expect(['identified', 'mitigating', 'mitigated', 'accepted', 'closed'].map(riskStatusSeverity)).toEqual(['warn', 'info', 'success', 'contrast', 'secondary'])
  })
  it('knows open statuses and ranks', () => {
    expect(riskOpen('identified')).toBe(true)
    expect(riskOpen('accepted')).toBe(false)
    expect(riskOpen(undefined)).toBe(false)
    expect(levelRank('critical')).toBe(3)
    expect(levelRank(null)).toBe(-1)
  })
})

describe('heatGrid', () => {
  it('builds 5×5 cells, likelihood 5 on top', () => {
    const g = heatGrid([risk({ likelihood: 5, impact: 1 }), risk({ likelihood: 5, impact: 1, status: 'mitigated' }), risk({ likelihood: 1, impact: 5, status: 'closed' })])
    expect(g.map((r) => r.likelihood)).toEqual([5, 4, 3, 2, 1])
    expect(g[0].cells.map((c) => c.impact)).toEqual([1, 2, 3, 4, 5])
    expect(g[0].cells[0]).toMatchObject({ likelihood: 5, impact: 1, score: 5, level: 'medium', count: 2, open: 1 })
    expect(g[4].cells[4]).toMatchObject({ score: 5, count: 0 })
    expect(g[0].cells[4].level).toBe('critical')
  })
  it('can include closed risks and clamps odd values', () => {
    const g = heatGrid([risk({ likelihood: 1, impact: 5, status: 'closed' }), risk({ likelihood: 9, impact: 0 })], true)
    expect(g[4].cells[4].count).toBe(1)
    expect(g[0].cells[0].count).toBe(1)
  })
})

describe('filterRisks', () => {
  const list = [
    risk({ id: 'a', likelihood: 4, impact: 5, level: 'critical', status: 'identified', owner: 'Ann' }),
    risk({ id: 'b', likelihood: 2, impact: 2, level: 'low', status: 'closed', title: 'Typo', key: 'P-RISK-2' }),
  ]
  it('filters by cell, level, status and text', () => {
    expect(filterRisks(list, {}).length).toBe(2)
    expect(filterRisks(list, { likelihood: 4, impact: 5 }).map((r) => r.id)).toEqual(['a'])
    expect(filterRisks(list, { impact: 2 }).map((r) => r.id)).toEqual(['b'])
    expect(filterRisks(list, { levels: ['low'] }).map((r) => r.id)).toEqual(['b'])
    expect(filterRisks(list, { statuses: ['identified'] }).map((r) => r.id)).toEqual(['a'])
    expect(filterRisks(list, { text: ' ann ' }).map((r) => r.id)).toEqual(['a'])
    expect(filterRisks(list, { text: 'risk-2' }).map((r) => r.id)).toEqual(['b'])
  })
  it('counts levels and finds the highest', () => {
    expect(levelCounts([...list, risk({ level: 'odd' })])).toEqual({ low: 1, medium: 0, high: 0, critical: 1 })
    expect(highestLevel(list)).toBe('critical')
    expect(highestLevel([])).toBe('')
  })
})
