import { beforeEach, describe, expect, it, vi } from 'vitest'

const calls: [string, string, unknown?][] = []
vi.mock('./api', () => ({
  api: {
    del: vi.fn(async (p: string, b?: unknown) => void calls.push(['DELETE', p, b])),
    post: vi.fn(async (p: string, b?: unknown) => {
      calls.push(['POST', p, b])
      return {}
    }),
  },
}))

import { applyFix, builtinScope, fixPermission, isBuiltin, relatedFixes, relatedLine, THRESHOLD_DEFAULTS } from './consistency'
import { dismissCheck, dismissPair, markConflict, markDuplicate, mergePair, percent, scoreSeverity, similarBody, similarKey, suggestedSurvivor, type SimilarItem } from './similarity'
import { cleanRule, blankRule } from './traceRules'
import type { TraceRelated } from './types'

const item = (id: string, links = 0, kind: 'requirement' | 'test' = 'requirement'): SimilarItem => ({ id, kind, key: id.toUpperCase(), title: id, status: 'draft', links, created_at: '' })
const t = (k: string, p?: Record<string, unknown>) => (p ? `${k} ${JSON.stringify(p)}` : k)

describe('similarity', () => {
  beforeEach(() => void (calls.length = 0))
  it('builds the request only for enough text', () => {
    expect(similarBody('requirement', { title: 'Login', description: '', details: [] })).toBeNull()
    const b = similarBody('test', { title: ' Sign in ', description: 'with a password', details: [' ', 'Open the page'] }, 't1', 3)
    expect(b).toEqual({ kind: 'test', title: 'Sign in', description: 'with a password', details: ['Open the page'], exclude_id: 't1', limit: 3 })
    expect(similarKey(b)).toBe(JSON.stringify(b))
    expect(similarKey(null)).toBe('')
  })
  it('formats scores', () => {
    expect(percent(0.829)).toBe(82)
    expect(percent(1)).toBe(100)
    expect(scoreSeverity(0.7, 0.68)).toBe('warn')
    expect(scoreSeverity(0.5, 0.68)).toBe('secondary')
  })
  it('names the dismissable check and proposes a survivor', () => {
    expect(dismissCheck('requirement', true)).toBe('potential_conflict')
    expect(dismissCheck('requirement', false)).toBe('duplicate_suspect')
    expect(dismissCheck('test', false)).toBe('test_duplicate_suspect')
    expect(suggestedSurvivor({ a: item('a', 1), b: item('b', 3) }).id).toBe('b')
    expect(suggestedSurvivor({ a: item('a', 2), b: item('b', 2) }).id).toBe('a')
  })
  it('acts with the ordinary endpoints', async () => {
    await markDuplicate('requirement', item('b'), item('a'))
    await markDuplicate('test', item('t2', 0, 'test'), item('t1', 0, 'test'))
    await markConflict(item('b'), item('a'))
    await dismissPair('p', 'potential_conflict', item('a'), item('b'))
    await dismissPair('p', 'potential_conflict', item('a'), item('b'), false)
    await mergePair('p', 'test', item('t1', 0, 'test'), item('t2', 0, 'test'))
    expect(calls).toEqual([
      ['POST', '/requirements/b/relations', { target_id: 'a', type: 'duplicates' }],
      ['POST', '/tests/t2/item-links', { target_type: 'test', target_id: 't1', label: 'duplicates' }],
      ['POST', '/requirements/b/relations', { target_id: 'a', type: 'conflicts_with' }],
      ['POST', '/projects/p/consistency/dismissals', { check: 'potential_conflict', item_id: 'b', other_id: 'a' }],
      ['DELETE', '/projects/p/consistency/dismissals', { check: 'potential_conflict', item_id: 'b', other_id: 'a' }],
      ['POST', '/projects/p/consistency/merge', { kind: 'test', survivor_id: 't1', duplicate_id: 't2' }],
    ])
  })
})

describe('similarity checks', () => {
  beforeEach(() => void (calls.length = 0))
  const rel = (x: Partial<TraceRelated>): TraceRelated => ({ kind: 'requirement', id: 'r2', key: 'R-2', title: 'Two', role: 'conflict', ...x })
  it('are built-in checks with thresholds', () => {
    expect(isBuiltin('potential_conflict')).toBe(true)
    expect(builtinScope('test_duplicate_suspect')).toBe('test')
    expect(THRESHOLD_DEFAULTS.potential_conflict).toBe(0.6)
    const r = cleanRule({ ...blankRule(), name: 'C', kind: 'potential_conflict', threshold: 0 })
    expect(r.threshold).toBe(0.6)
    expect(cleanRule({ ...blankRule(), name: 'D', kind: 'test_duplicate_suspect', threshold: 0.2 }).threshold).toBe(0.5)
  })
  it('offer marking and dismissing', async () => {
    expect(relatedFixes('potential_conflict', rel({}))).toEqual(['mark_conflict', 'dismiss'])
    expect(fixPermission('mark_duplicate', 'test_duplicate_suspect')).toBe('test.write')
    expect(fixPermission('dismiss', 'test_duplicate_suspect')).toBe('test.write')
    expect(fixPermission('mark_conflict', 'potential_conflict')).toBe('requirement.write')
    await applyFix('mark_conflict', 'potential_conflict', { id: 'r1', kind: 'requirement' }, rel({}), 'p')
    await applyFix('mark_duplicate', 'test_duplicate_suspect', { id: 't2', kind: 'test' }, rel({ kind: 'test', id: 't1', role: 'duplicate' }), 'p')
    expect(calls).toEqual([
      ['POST', '/requirements/r1/relations', { target_id: 'r2', type: 'conflicts_with' }],
      ['POST', '/tests/t2/item-links', { target_type: 'test', target_id: 't1', label: 'duplicates' }],
    ])
    expect(relatedLine(rel({ check: 'threshold', note: '≤ 2 s ↔ ≥ 5 s' }), t)).toContain('consistency.reason.threshold')
  })
})
