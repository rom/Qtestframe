import { beforeEach, describe, expect, it, vi } from 'vitest'

const calls: [string, string, unknown?][] = []
vi.mock('./api', () => ({
  api: {
    del: vi.fn(async (p: string) => void calls.push(['DELETE', p])),
    post: vi.fn(async (p: string, b?: unknown) => void calls.push(['POST', p, b])),
    patch: vi.fn(async (p: string, b?: unknown) => void calls.push(['PATCH', p, b])),
  },
}))

import { applyFix, builtinScope, fixPermission, fixRoute, isBuiltin, itemFixes, itemRoute, relatedFixes, relatedLine, violationLine } from './consistency'
import { blankRule, cleanRule, ruleSummary, violationRoute, violationText } from './traceRules'
import type { TraceRelated } from './types'

const t = (k: string, p?: Record<string, unknown>) => (p ? `${k} ${JSON.stringify(p)}` : k)
const rel = (x: Partial<TraceRelated>): TraceRelated => ({ kind: 'requirement', id: 'r2', key: 'R-2', title: 'Two', role: 'depends_on', ...x })

describe('built-in checks', () => {
  beforeEach(() => void (calls.length = 0))
  it('knows kinds and scopes', () => {
    expect(isBuiltin('duplicate_suspect')).toBe(true)
    expect(isBuiltin('verified_by')).toBe(false)
    expect(builtinScope('broken_prerequisite')).toBe('test')
    expect(builtinScope('unmitigated_risk')).toBe('risk')
    expect(builtinScope('conflict_in_release')).toBe('requirement')
  })
  it('routes items of every kind', () => {
    expect(itemRoute({ kind: 'risk', id: 'k' }, 'p')).toEqual({ name: 'risks', params: { projectId: 'p' }, query: { risk: 'k' } })
    expect(itemRoute({ kind: 'release', id: 'r' }, 'p')).toMatchObject({ name: 'releases', query: { r: 'r' } })
    expect(itemRoute({ kind: 'test', id: 't' }, 'p')).toMatchObject({ name: 'tests' })
    expect(itemRoute({ kind: 'defect', id: 'd' }, 'p')).toMatchObject({ name: 'defects' })
    expect(itemRoute({ kind: 'x', id: 'd' }, 'p')).toBeUndefined()
    expect(violationRoute({ kind: 'risk', id: 'k' }, 'p')).toMatchObject({ name: 'risks' })
  })
  it('offers the fixes that apply to a related item', () => {
    expect(relatedFixes('unmet_dependency', rel({ link: 'rel1', check: 'rejected' }))).toEqual(['remove_relation'])
    expect(relatedFixes('unmet_dependency', rel({ check: 'deleted' }))).toEqual(['dismiss'])
    expect(relatedFixes('verifies_inactive', rel({ role: 'verifies', link: 'l1' }))).toEqual(['remove_link'])
    expect(relatedFixes('broken_prerequisite', rel({ role: 'prerequisite', link: 'd1', check: 'cycle' }))).toEqual(['remove_dependency'])
    expect(relatedFixes('broken_prerequisite', rel({ role: 'prerequisite', link: 'd1', check: 'failing' }))).toEqual([])
    expect(relatedFixes('conflict_in_release', rel({ kind: 'release', role: 'release' }))).toEqual(['remove_from_release'])
    expect(relatedFixes('duplicate_suspect', rel({ role: 'duplicate' }))).toEqual(['mark_duplicate', 'dismiss'])
    expect(itemFixes({ fixes: ['set_obsolete', 'remove_relation', 'add_justification'] })).toEqual(['set_obsolete', 'add_justification'])
    expect(fixPermission('remove_link')).toBe('test.write')
    expect(fixPermission('remove_from_release')).toBe('project.update')
    expect(fixPermission('mark_duplicate')).toBe('requirement.write')
    expect(fixPermission('link_mitigation')).toBe('requirement.read')
  })
  it('performs fixes with the ordinary endpoints', async () => {
    const v = { id: 'r1', kind: 'requirement' }
    expect(await applyFix('remove_relation', 'x', v, rel({ link: 'rel1' }), 'p')).toBe(true)
    await applyFix('remove_link', 'x', v, rel({ link: 'l1' }), 'p')
    await applyFix('remove_dependency', 'x', v, rel({ link: 'd1' }), 'p')
    await applyFix('remove_from_release', 'x', v, rel({ kind: 'release', id: 'rl' }), 'p')
    await applyFix('set_obsolete', 'x', v, null, 'p')
    await applyFix('mark_duplicate', 'duplicate_suspect', v, rel({}), 'p')
    await applyFix('dismiss', 'duplicate_suspect', v, rel({}), 'p')
    expect(await applyFix('link_mitigation', 'x', v, null, 'p')).toBe(false)
    expect(calls).toEqual([
      ['DELETE', '/relations/rel1'],
      ['DELETE', '/links/l1'],
      ['DELETE', '/test-dependencies/d1'],
      ['POST', '/releases/rl/requirements', { remove: ['r1'] }],
      ['PATCH', '/requirements/r1', { status: 'obsolete' }],
      ['POST', '/requirements/r1/relations', { target_id: 'r2', type: 'duplicates' }],
      ['POST', '/projects/p/consistency/dismissals', { check: 'duplicate_suspect', item_id: 'r1', other_id: 'r2' }],
    ])
    expect(fixRoute('add_justification', v, 'p')).toMatchObject({ name: 'requirements', query: { item: 'r1' } })
    expect(fixRoute('link_mitigation', { id: 'k', kind: 'risk' }, 'p')).toMatchObject({ name: 'risks', query: { risk: 'k' } })
    expect(fixRoute('dismiss', v, 'p')).toBeUndefined()
  })
  it('phrases violations', () => {
    const v = { check: 'conflict', related: [rel({}), rel({ kind: 'release', id: 'x', key: '1.0', role: 'release' })] }
    expect(violationLine(v, t)).toContain('"keys":"R-2"')
    expect(violationLine(v, t)).toContain('"releases":"1.0"')
    expect(violationText({ ...v, found: 1, required: 0, total: 1 }, t)).toContain('consistency.reason.conflict')
    expect(relatedLine(rel({ check: 'deleted' }), t)).toBe('consistency.role.depends_on · consistency.deleted')
    expect(relatedLine(rel({ role: 'duplicate', note: '95%' }), t)).toContain('consistency.similarity')
    expect(relatedLine(rel({ check: 'failing', role: 'prerequisite' }), t)).toContain('consistency.failing')
    expect(relatedLine(rel({ check: 'cycle', role: 'prerequisite' }), t)).toContain('consistency.cycle')
    expect(relatedLine(rel({ status: 'approved' }), t, (s) => s.toUpperCase())).toBe('consistency.role.depends_on · APPROVED')
  })
  it('cleans built-in rule drafts', () => {
    const d = { ...blankRule(), name: ' Dups ', kind: 'duplicate_suspect', selector: ' type:x ', threshold: 3 }
    expect(cleanRule(d)).toEqual({ name: 'Dups', description: '', severity: 'error', enabled: true, scope: 'requirement', selector: 'type:x', kind: 'duplicate_suspect', min: 1, threshold: 1 })
    expect(cleanRule({ ...blankRule(), name: 'R', kind: 'unmitigated_risk', selector: 'x' })).toMatchObject({ scope: 'risk', selector: '', level: 'high' })
    expect(ruleSummary({ ...blankRule(), kind: 'obsolete_parent' }, t, () => '')).toBe('consistency.check.obsolete_parent.description')
  })
})
