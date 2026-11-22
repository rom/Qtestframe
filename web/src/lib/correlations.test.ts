import { describe, expect, it } from 'vitest'
import { blockCounts, contentChanges, impactSections, needsImpactConfirm, setSeverityLevel, severityLevel, withDefaults, type ChangeImpact } from './correlations'

const impact = (p: Partial<ChangeImpact> = {}): ChangeImpact => ({
  kind: 'requirement',
  id: 'r',
  key: 'R-1',
  title: 'x',
  requirements: [],
  tests: [],
  issues: [],
  documents: [],
  results: [],
  total: 0,
  ...p,
})

describe('content changes', () => {
  it('finds the substance fields that changed', () => {
    const before = { title: 'A', description: 'x', priority: 'low', criteria: [{ id: '1', description: 'c', method: 'test', threshold: '', position: 1 }] }
    expect(contentChanges('requirement', before, { ...before, priority: 'high' })).toEqual([])
    expect(contentChanges('requirement', before, { ...before, description: 'y' })).toEqual(['description'])
    // criteria without ids and positions compare by content
    expect(contentChanges('requirement', before, { ...before, criteria: [{ description: 'c', method: 'test', threshold: '' }] })).toEqual([])
    expect(contentChanges('requirement', before, { ...before, criteria: [] })).toEqual(['criteria'])
    expect(contentChanges('requirement', null, before)).toEqual([])
    const tb = { title: 'T', steps: [{ id: 's', action: 'a', expected: 'b', data: '' }] }
    expect(contentChanges('test', tb, { ...tb, steps: [{ action: 'a', expected: 'c', data: '' }] })).toEqual(['steps'])
    expect(contentChanges('other', tb, tb)).toEqual([])
  })
})

describe('impact', () => {
  it('lists the non-empty sections with the items not yet affected', () => {
    const imp = impact({ tests: [{ kind: 'test', id: 't', key: 'T-1', already: true }, { kind: 'test', id: 'u', key: 'T-2' }], total: 1 })
    expect(impactSections(imp)).toEqual([{ section: 'tests', items: imp.tests, fresh: 1 }])
    expect(impactSections(null)).toEqual([])
    expect(needsImpactConfirm(['description'], imp)).toBe(true)
    expect(needsImpactConfirm([], imp)).toBe(false)
    expect(needsImpactConfirm(['title'], impact())).toBe(false)
  })
  it('counts blocks', () => {
    expect(blockCounts([{ level: 'block' }, { level: 'warn' }, { level: 'warn' }] as never)).toEqual({ blocking: 1, warnings: 2 })
  })
})

describe('settings', () => {
  it('fills defaults and moves severities between levels', () => {
    const s = withDefaults(null)
    expect(s.relation_impact.refines).toBe('incoming')
    expect(s.relation_impact.relates_to).toBe('off')
    expect(s.stale_after_days).toBe(90)
    expect(withDefaults({ stale_after_days: 0 }).stale_after_days).toBe(0)
    expect(severityLevel(s, 'critical')).toBe('block')
    expect(severityLevel(s, 'minor')).toBe('warn')
    expect(severityLevel(s, 'trivial')).toBe('none')
    const m = setSeverityLevel(s, 'minor', 'block')
    expect(m.blocking_severities).toEqual(['critical', 'major', 'minor'])
    expect(m.warning_severities).toEqual([])
    expect(setSeverityLevel(m, 'critical', 'none').blocking_severities).toEqual(['major', 'minor'])
    expect(setSeverityLevel(s, 'trivial', 'warn').warning_severities).toEqual(['minor', 'trivial'])
  })
})

describe('messages', () => {
  it('cover every direction, action, relation type, level and section', async () => {
    const { default: en } = await import('@/i18n/locales/en')
    const { default: sv } = await import('@/i18n/locales/sv')
    const { IMPACT_DIRECTIONS, IMPACT_SECTIONS, REGRESSION_ACTIONS, RELATION_TYPES, CONTENT_FIELDS } = await import('./correlations')
    for (const c of [en.correlations, sv.correlations] as unknown as Record<string, Record<string, string>>[]) {
      expect(IMPACT_DIRECTIONS.filter((d) => !c.direction[d])).toEqual([])
      expect(REGRESSION_ACTIONS.filter((d) => !c.action[d])).toEqual([])
      expect(RELATION_TYPES.filter((d) => !c.relationType[d])).toEqual([])
      expect(IMPACT_SECTIONS.filter((d) => !c.section[d] || !c.already[d])).toEqual([])
      expect(['block', 'warn', 'none'].filter((d) => !c.level[d])).toEqual([])
      expect(['age', 'test_changed', 'requirement_changed'].filter((d) => !c.staleReason[d])).toEqual([])
      expect([...CONTENT_FIELDS.requirement, ...CONTENT_FIELDS.test].filter((d) => !c.field[d])).toEqual([])
    }
  })
})
