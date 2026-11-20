import { describe, expect, it } from 'vitest'
import {
  TRACE_RULE_KINDS,
  TRACE_RULE_PRESETS,
  blankRule,
  changeScope,
  cleanRule,
  draftOf,
  failing,
  filterResults,
  kindOptions,
  kindsForScope,
  presetRule,
  resultCounts,
  ruleSummary,
  rulesRoute,
  severityTone,
  targetKind,
  violationRoute,
  violationText,
} from './traceRules'
import type { TraceRule, TraceRuleResult } from './types'
import en from '@/i18n/locales/en'
import sv from '@/i18n/locales/sv'

const t = (key: string, params?: Record<string, unknown>, n?: number) => `${key}${params && Object.keys(params).length ? JSON.stringify(params) : ''}${n === undefined ? '' : `#${n}`}`
const rel = (type: string, dir: string) => `${type}/${dir}`

const rule = (p: Partial<TraceRule> = {}): TraceRule => ({
  ...blankRule(),
  id: 'r1',
  project_id: 'p',
  position: 1,
  created_by: '',
  created_at: '',
  updated_at: '',
  name: 'Rule',
  ...p,
})
const result = (p: Partial<TraceRuleResult> = {}, r: Partial<TraceRule> = {}): TraceRuleResult => ({
  rule: rule(r),
  matched: 3,
  violations: [],
  violation_count: 0,
  passed: true,
  ...p,
})

describe('kinds and options', () => {
  it('maps scopes to kinds and other ends', () => {
    expect(kindsForScope('test')).toEqual(['verifies', 'item_link'])
    expect(kindsForScope('requirement')).toEqual(['verified_by', 'relation', 'item_link'])
    expect(targetKind({ kind: 'verified_by' })).toBe('test')
    expect(targetKind({ kind: 'verifies' })).toBe('requirement')
    expect(targetKind({ kind: 'relation' })).toBe('requirement')
    expect(targetKind({ kind: 'item_link', target_type: 'test' })).toBe('test')
    expect(targetKind({ kind: 'item_link', target_type: 'url' })).toBe('')
    expect(TRACE_RULE_KINDS.map((k) => Object.values(kindOptions(k)).filter(Boolean).length)).toEqual([2, 1, 2, 2])
  })
  it('changes the scope and keeps a kind that fits', () => {
    expect(changeScope(blankRule(), 'test').kind).toBe('verifies')
    expect(changeScope({ ...blankRule(), kind: 'item_link' }, 'test').kind).toBe('item_link')
  })
})

describe('drafts', () => {
  it('builds presets with translated names', () => {
    for (const p of TRACE_RULE_PRESETS) {
      const d = presetRule(p.id, t)
      expect(d.name).toBe(`traceRules.preset.${p.id}.name`)
      expect(en.traceRules.preset).toHaveProperty(p.id)
      expect(sv.traceRules.preset).toHaveProperty(p.id)
      expect(cleanRule(d).kind).toBe(d.kind)
    }
    expect(presetRule('noSuspect', t)).toMatchObject({ min: 0, every: true, not_suspect: true })
    expect(presetRule('nope', t)).toEqual(blankRule())
  })
  it('copies a rule for editing', () => {
    const r = rule({ kind: 'relation', relation_type: 'refines', direction: 'incoming', min: 2, description: undefined as unknown as string, selector: undefined as unknown as string })
    expect(draftOf(r)).toMatchObject({ kind: 'relation', relation_type: 'refines', direction: 'incoming', min: 2, description: '', selector: '', not_suspect: false, target_query: '' })
    expect(draftOf(rule({ severity: '', scope: '', min: undefined as unknown as number, every: true }))).toMatchObject({ severity: 'error', scope: 'requirement', min: 1, every: true })
  })
  it('cleans drafts for the API', () => {
    const d = { ...blankRule(), name: ' A ', selector: ' type:x ', kind: 'verified_by', not_suspect: true, passed: true, relation_type: 'refines', direction: 'incoming', target_type: 'url', target_query: ' status:ready ', min: 2.4 }
    expect(cleanRule(d)).toMatchObject({ name: 'A', selector: 'type:x', not_suspect: true, passed: true, relation_type: '', direction: '', target_type: '', target_query: 'status:ready', min: 2 })
    expect(cleanRule({ ...d, kind: 'item_link', target_type: 'url' })).toMatchObject({ not_suspect: false, passed: false, direction: 'incoming', target_type: 'url', target_query: '' })
    expect(cleanRule({ ...d, kind: 'verifies' })).toMatchObject({ scope: 'test', passed: false, not_suspect: true })
    expect(cleanRule({ ...d, kind: 'relation', scope: 'test' })).toMatchObject({ scope: 'requirement', relation_type: 'refines' })
    expect(cleanRule({ ...d, min: 0 }).min).toBe(1)
    expect(cleanRule({ ...d, min: 0, every: true }).min).toBe(0)
    expect(cleanRule({ ...d, min: 5000 }).min).toBe(1000)
    expect(cleanRule({ ...d, min: Number.NaN }).min).toBe(1)
    expect(cleanRule({ ...d, description: undefined as unknown as string, selector: undefined as unknown as string, target_query: undefined, relation_type: undefined, kind: 'relation' })).toMatchObject({ description: '', selector: '', target_query: '', relation_type: '' })
    expect(cleanRule({ ...d, kind: 'item_link', direction: undefined, target_type: undefined })).toMatchObject({ direction: '', target_type: '' })
  })
})

describe('texts', () => {
  it('summarises rules', () => {
    expect(ruleSummary(rule({ selector: 'obligation:must', kind: 'verified_by', min: 1, not_suspect: true, passed: true }), t, rel)).toBe(
      'traceRules.summary.sentence{"who":"traceRules.summary.matching.requirement{\\"selector\\":\\"obligation:must\\"}","what":"traceRules.summary.verifiedBy#1"} (traceRules.cond.notSuspect, traceRules.cond.passed)',
    )
    expect(ruleSummary(rule({ scope: 'test', kind: 'verifies', min: 2 }), t, rel)).toBe('traceRules.summary.sentence{"who":"traceRules.summary.all.test","what":"traceRules.summary.verifies#2"}')
    expect(ruleSummary(rule({ kind: 'relation', relation_type: 'refines', direction: 'incoming', target_query: 'type:functional' }), t, rel)).toContain('refines/incoming')
    expect(ruleSummary(rule({ kind: 'relation', relation_type: 'refines' }), t, rel)).toContain('refines/outgoing')
    expect(ruleSummary(rule({ kind: 'relation', direction: 'outgoing' }), t, rel)).toContain('traceRules.anyRelation')
    expect(ruleSummary(rule({ kind: 'relation', direction: 'outgoing' }), t, rel)).toContain('traceRules.direction.outgoing')
    expect(ruleSummary(rule({ kind: 'item_link', target_type: 'url', direction: 'incoming' }), t, rel)).toContain('traceRules.targetType.url')
    expect(ruleSummary(rule({ kind: 'item_link' }), t, rel)).toContain('traceRules.anyTarget')
    expect(ruleSummary(rule({ kind: 'item_link', target_type: 'url', target_query: 'x' }), t, rel)).not.toContain('cond.target')
    const none = ruleSummary(rule({ kind: 'verified_by', min: 0, every: true, not_suspect: true }), t, rel)
    expect(none).toContain('traceRules.summary.colon')
    expect(none).toContain('traceRules.summary.everyOf.verified_by{\\"conds\\":\\"traceRules.cond.notSuspect\\"}')
    expect(ruleSummary(rule({ kind: 'relation', min: 0, every: true }), t, rel)).toContain('–')
    expect(ruleSummary(rule({ kind: 'verified_by', min: 1, every: true }), t, rel)).toContain('traceRules.cond.every')
  })
  it('says what a violation lacks', () => {
    expect(violationText({ found: 0, required: 1, total: 0 }, t)).toBe('traceRules.violation.missing{"found":0,"required":1}')
    expect(violationText({ found: 1, required: 2, total: 3 }, t)).toBe('traceRules.violation.partial{"found":1,"total":3,"required":2}')
  })
  it('has tones and routes', () => {
    expect(severityTone('warning')).toBe('warn')
    expect(severityTone('error')).toBe('danger')
    expect(violationRoute({ kind: 'test', id: 't' }, 'p')).toEqual({ name: 'tests', params: { projectId: 'p' }, query: { item: 't' } })
    expect(violationRoute({ kind: 'requirement', id: 'r' }, 'p')).toEqual({ name: 'requirements', params: { projectId: 'p' }, query: { item: 'r' } })
    expect(rulesRoute('p')).toEqual({ name: 'traceability', params: { projectId: 'p' }, query: { view: 'rules' } })
  })
})

describe('results', () => {
  const v = (key: string, title = '') => ({ kind: 'requirement', id: key, key, title, found: 0, required: 1, total: 0 })
  const list = [
    result({}, { id: 'a', name: 'Alpha' }),
    result({ passed: false, violations: [v('R-1', 'Login')], violation_count: 1 }, { id: 'b', name: 'Beta', severity: 'error' }),
    result({ passed: false, violations: [v('R-2')], violation_count: 4 }, { id: 'c', name: 'Gamma', severity: 'warning' }),
    result({ passed: false, violations: [v('R-3')], violation_count: 2 }, { id: 'd', name: 'Delta', enabled: false }),
  ]
  it('counts', () => {
    expect(resultCounts(list)).toEqual({ rules: 3, passed: 1, errors: 1, warnings: 1, disabled: 1, violations: 5 })
    expect(resultCounts(null).rules).toBe(0)
    expect(failing(list[3])).toBe(false)
    expect(failing(list[1])).toBe(true)
  })
  it('filters', () => {
    expect(filterResults(list, true).map((r) => r.rule.id)).toEqual(['b', 'c'])
    expect(filterResults(list, false, 'login').map((r) => r.rule.id)).toEqual(['b'])
    expect(filterResults(list, false, ' gam ').map((r) => r.rule.id)).toEqual(['c'])
    expect(filterResults(list, false).length).toBe(4)
    expect(filterResults(undefined, true)).toEqual([])
  })
})
