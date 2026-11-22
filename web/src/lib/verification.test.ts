import { describe, expect, it } from 'vitest'
import {
  buildHierarchy,
  elementOptions,
  elementPercent,
  evidenceLabel,
  evidenceSeverity,
  filterHierarchy,
  flattenHierarchy,
  reasonText,
  recordProblems,
  subtreeIds,
  today,
  verificationIcon,
  verificationSeverity,
} from './verification'

const t = (k: string, p?: Record<string, unknown>) => (p ? `${k}(${Object.entries(p).map(([a, b]) => `${a}=${b}`).join(',')})` : k)

describe('verification helpers', () => {
  it('maps states to severities and icons', () => {
    expect(verificationSeverity('verified')).toBe('success')
    expect(verificationSeverity('failed')).toBe('danger')
    expect(verificationSeverity(undefined)).toBe('secondary')
    expect(verificationIcon('pending')).toBe('pi pi-clock')
    expect(verificationIcon('none')).toBe('pi pi-circle')
    expect(verificationIcon('partial')).toContain('circle-fill')
    expect(verificationIcon('failed')).toContain('times')
    expect(verificationIcon('verified')).toContain('verified')
    expect(evidenceSeverity('passed')).toBe('success')
    expect(evidenceSeverity('failed')).toBe('danger')
    expect(evidenceSeverity('void')).toBe('contrast')
    expect(evidenceSeverity('pending')).toBe('secondary')
  })
  it('translates reasons with their parameters', () => {
    const te = (k: string) => k !== 'verification.reason.unknown' && k !== 'enums.status.custom'
    expect(reasonText(t, te, { code: 'criterion_missing', params: { criterion: '2' }, text: '' })).toBe('verification.reason.criterion_missing(criterion=2)')
    expect(reasonText(t, te, { code: 'evidence_failed', params: { kind: 'test', key: 'T-1' }, text: '' })).toBe('verification.reason.evidence_failed(kind=verification.evidenceKind.test,key=T-1)')
    expect(reasonText(t, te, { code: 'child_status_unverified', params: { key: 'R-2', status: 'custom', state: 'none' }, text: '' })).toContain('status=custom')
    expect(reasonText(t, te, { code: 'unknown', text: 'raw' })).toBe('raw')
  })
  it('labels evidence', () => {
    expect(evidenceLabel(t, { kind: 'test', id: 't', key: 'T-1', title: '', result: 'passed', state: 'passed', step_position: 2 })).toBe('T-1 · verification.stepN(n=2)')
    expect(evidenceLabel(t, { kind: 'test', id: 't', key: 'T-1', title: '', result: 'passed', state: 'passed' })).toBe('T-1')
    expect(evidenceLabel(t, { kind: 'record', id: 'r', key: 'inspection', title: 'Ann', result: 'pass', state: 'passed', date: '2026-01-10' })).toBe('enums.evaluationMethod.inspection · 2026-01-10')
  })
  it('builds, filters and flattens the hierarchy', () => {
    const items = [
      { requirement_id: 'p', parent_ids: [], state: 'partial' },
      { requirement_id: 'c1', parent_ids: ['p'], state: 'verified' },
      { requirement_id: 'c2', parent_ids: ['p', 'gone'], state: 'failed' },
      { requirement_id: 'g', parent_ids: ['c2'], state: 'none' },
      { requirement_id: 'x', parent_ids: ['y'], state: 'none' },
      { requirement_id: 'y', parent_ids: ['x'], state: 'none' }, // a cycle without root
      { requirement_id: 'solo', parent_ids: [], state: 'verified' },
    ]
    const tree = buildHierarchy(items)
    expect(tree.map((n) => n.item.requirement_id)).toEqual(['p', 'solo'])
    expect(tree[0].children.map((n) => n.item.requirement_id)).toEqual(['c1', 'c2'])
    const flat = flattenHierarchy(tree, new Set(), (i) => i.requirement_id)
    expect(flat.map((n) => `${n.item.requirement_id}:${n.depth}`)).toEqual(['p:0', 'c1:1', 'c2:1', 'g:2', 'solo:0'])
    expect(flattenHierarchy(tree, new Set(['p']), (i) => i.requirement_id).length).toBe(2)
    const failing = filterHierarchy(tree, (i) => i.state === 'failed')
    expect(flattenHierarchy(failing, new Set(), (i) => i.requirement_id).map((n) => n.item.requirement_id)).toEqual(['p', 'c2'])
  })
  it('handles elements', () => {
    expect(elementPercent({ verified: 42, requirements: 50 })).toBe(84)
    expect(elementPercent({ verified: 0, requirements: 0 })).toBe(0)
    const list = [
      { id: 'a', parent_id: null, name: 'A', path: 'A' },
      { id: 'b', parent_id: 'a', name: 'B', path: 'A / B' },
      { id: 'c', parent_id: 'b', name: 'C', path: '' },
      { id: 'd', parent_id: null, name: 'D' },
    ]
    expect([...subtreeIds(list, 'a')].sort()).toEqual(['a', 'b', 'c'])
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    expect(elementOptions(list as any).map((o) => o.label)).toEqual(['A', 'A / B', 'C', 'D'])
  })
  it('checks record forms', () => {
    expect(today(new Date(2026, 0, 5))).toBe('2026-01-05')
    expect(recordProblems({ method: 'review', result: 'pass', performed_on: '2026-01-10', valid_until: '' })).toEqual([])
    expect(recordProblems({ method: '', result: '', performed_on: '10/01/2026', valid_until: '2025-01-01' })).toEqual(['method', 'result', 'date'])
    expect(recordProblems({ method: 'review', result: 'pass', performed_on: '2026-01-10', valid_until: '2026-01-01' })).toEqual(['validUntil'])
  })
})
