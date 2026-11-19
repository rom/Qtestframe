import { describe, expect, it } from 'vitest'
import { deriveStatus, summarize } from './summary'

describe('summarize', () => {
  it('handles empty input', () => {
    const s = summarize(null)
    expect(s.total).toBe(0)
    expect(s.segments).toEqual([])
    expect(s.progress).toBe(0)
    expect(s.passRate).toBe(0)
  })
  it('computes progress and pass rate in canonical order', () => {
    const s = summarize({ not_run: 2, failed: 1, passed: 6, blocked: 1, weird: 0, custom: 1 } as Record<string, number>)
    expect(s.total).toBe(11)
    expect(s.executed).toBe(9)
    expect(s.passed).toBe(6)
    expect(s.failed).toBe(1)
    expect(s.progress).toBe(82)
    expect(s.passRate).toBe(67)
    expect(s.segments.map((x) => x.status)).toEqual(['passed', 'failed', 'blocked', 'not_run', 'custom'])
    expect(s.segments[0].color).toBe('#22c55e')
    expect(s.segments[4].color).toBe('#64748b')
    expect(Math.round(s.segments.reduce((a, b) => a + b.pct, 0))).toBe(100)
  })
  it('respects an explicit total by padding not_run', () => {
    const s = summarize({ total: 10, passed: 4 })
    expect(s.total).toBe(10)
    expect(s.segments.find((x) => x.status === 'not_run')?.count).toBe(6)
    expect(summarize({ total: 1, passed: 4 }).total).toBe(4)
  })
  it('ignores non-number values', () => {
    expect(summarize({ passed: 'x' } as unknown as Record<string, number>).total).toBe(0)
  })
})

describe('deriveStatus', () => {
  it('derives an overall status', () => {
    expect(deriveStatus([])).toBe('not_run')
    expect(deriveStatus(['passed', 'failed', 'blocked'])).toBe('failed')
    expect(deriveStatus(['passed', 'blocked'])).toBe('blocked')
    expect(deriveStatus(['not_run', 'not_run'])).toBe('not_run')
    expect(deriveStatus(['passed', 'not_run'])).toBe('in_progress')
    expect(deriveStatus(['passed', ''])).toBe('in_progress')
    expect(deriveStatus(['passed', 'skipped'])).toBe('passed')
    expect(deriveStatus(['skipped', 'skipped'])).toBe('skipped')
  })
})
