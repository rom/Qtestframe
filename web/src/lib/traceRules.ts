// Traceability rules: kinds, presets, drafts as sent to the API, texts and
// filtering of evaluation results (mirrors internal/model TraceRule and
// internal/tracerules).
import type { RouteLocationRaw } from 'vue-router'
import type { TraceRule, TraceRuleFields, TraceRuleResult, TraceViolation } from './types'
import { builtinScope, isBuiltin, itemRoute, THRESHOLD_DEFAULTS, violationLine } from './consistency'

export const TRACE_RULE_KINDS = ['verified_by', 'verifies', 'relation', 'item_link'] as const
export const TRACE_RULE_SEVERITIES = ['error', 'warning'] as const
export const TRACE_RULE_SCOPES = ['requirement', 'test'] as const
/** Directions of relations and item links; '' is any. */
export const TRACE_DIRECTIONS = ['', 'incoming', 'outgoing'] as const
/** What an item link may point to (internal/model ItemLinkTargetTypes). */
export const TRACE_TARGET_TYPES = ['requirement', 'test', 'run', 'file', 'report', 'url'] as const

type Scope = (typeof TRACE_RULE_SCOPES)[number]

/** Rule kinds that apply to items of a scope. */
export function kindsForScope(scope: string): string[] {
  if (scope === 'test') return ['verifies', 'item_link']
  return ['verified_by', 'relation', 'item_link']
}

/** Kind of item at the other end of a rule's traces ('' when it is not a requirement or test). */
export function targetKind(rule: Pick<TraceRuleFields, 'kind' | 'target_type'>): Scope | '' {
  switch (rule.kind) {
    case 'verified_by':
      return 'test'
    case 'item_link':
      return rule.target_type === 'requirement' || rule.target_type === 'test' ? rule.target_type : ''
  }
  return 'requirement'
}

/** Which options a rule kind uses. */
export function kindOptions(kind: string): { notSuspect: boolean; passed: boolean; relationType: boolean; direction: boolean; targetType: boolean } {
  return {
    notSuspect: kind === 'verified_by' || kind === 'verifies',
    passed: kind === 'verified_by',
    relationType: kind === 'relation',
    direction: kind === 'relation' || kind === 'item_link',
    targetType: kind === 'item_link',
  }
}

/** An empty rule draft. */
export function blankRule(): TraceRuleFields {
  return { name: '', description: '', severity: 'error', enabled: true, scope: 'requirement', selector: '', kind: 'verified_by', min: 1, not_suspect: false, passed: false, relation_type: '', direction: '', target_type: '', target_query: '', every: false }
}

export interface TraceRulePreset {
  id: string
  rule: Partial<TraceRuleFields>
}

/** Useful rules to start from; names and descriptions are translated by id. */
export const TRACE_RULE_PRESETS: readonly TraceRulePreset[] = [
  { id: 'mustVerified', rule: { scope: 'requirement', selector: 'obligation:must OR obligation:shall', kind: 'verified_by', min: 1, severity: 'error' } },
  { id: 'mustPassed', rule: { scope: 'requirement', selector: 'obligation:must OR obligation:shall', kind: 'verified_by', min: 1, passed: true, not_suspect: true, severity: 'error' } },
  { id: 'noSuspect', rule: { scope: 'requirement', selector: 'obligation:must OR obligation:shall', kind: 'verified_by', min: 0, not_suspect: true, every: true, severity: 'warning' } },
  { id: 'testVerifies', rule: { scope: 'test', selector: '', kind: 'verifies', min: 1, severity: 'warning' } },
  { id: 'businessRefined', rule: { scope: 'requirement', selector: 'type:business', kind: 'relation', relation_type: 'refines', direction: 'incoming', min: 1, severity: 'warning' } },
  { id: 'securityVerified', rule: { scope: 'requirement', selector: 'type:security', kind: 'verified_by', min: 2, severity: 'warning' } },
]

type T = (key: string, params?: Record<string, unknown>, plural?: number) => string

/** A draft from a preset with its translated name and description. */
export function presetRule(id: string, t: T): TraceRuleFields {
  const p = TRACE_RULE_PRESETS.find((x) => x.id === id)
  const base = blankRule()
  if (!p) return base
  return { ...base, ...p.rule, name: t(`traceRules.preset.${id}.name`), description: t(`traceRules.preset.${id}.description`) }
}

/** A draft of an existing rule (for editing). */
export function draftOf(r: TraceRule): TraceRuleFields {
  const b = blankRule()
  return {
    name: r.name,
    description: r.description ?? '',
    severity: r.severity || b.severity,
    enabled: r.enabled,
    scope: r.scope || b.scope,
    selector: r.selector ?? '',
    kind: r.kind,
    min: r.min ?? 1,
    not_suspect: !!r.not_suspect,
    passed: !!r.passed,
    relation_type: r.relation_type ?? '',
    direction: r.direction ?? '',
    target_type: r.target_type ?? '',
    target_query: r.target_query ?? '',
    every: !!r.every,
    ...(r.threshold ? { threshold: r.threshold } : {}),
    ...(r.level ? { level: r.level } : {}),
  }
}

/** Changes the scope of a draft, choosing a kind that applies to it. */
export function changeScope(d: TraceRuleFields, scope: string): TraceRuleFields {
  const kinds = kindsForScope(scope)
  return { ...d, scope, kind: kinds.includes(d.kind) ? d.kind : kinds[0] }
}

/** The draft as sent to the API: only the options its kind uses. */
export function cleanRule(d: TraceRuleFields): TraceRuleFields {
  if (isBuiltin(d.kind)) {
    const scope = builtinScope(d.kind)
    const out: TraceRuleFields = {
      name: d.name.trim(),
      description: (d.description ?? '').trim(),
      severity: d.severity,
      enabled: d.enabled,
      scope,
      selector: scope === 'risk' ? '' : (d.selector ?? '').trim(),
      kind: d.kind,
      min: 1,
    }
    if (d.kind in THRESHOLD_DEFAULTS) out.threshold = Math.min(1, Math.max(0.5, Number(d.threshold) || THRESHOLD_DEFAULTS[d.kind]))
    if (d.kind === 'unmitigated_risk') out.level = d.level || 'high'
    return out
  }
  const o = kindOptions(d.kind)
  const min = Math.round(Number(d.min))
  const out: TraceRuleFields = {
    name: d.name.trim(),
    description: (d.description ?? '').trim(),
    severity: d.severity,
    enabled: d.enabled,
    scope: kindsForScope(d.scope).includes(d.kind) ? d.scope : d.kind === 'verifies' ? 'test' : 'requirement',
    selector: (d.selector ?? '').trim(),
    kind: d.kind,
    min: Number.isFinite(min) ? Math.max(d.every ? 0 : 1, Math.min(1000, min)) : 1,
    every: !!d.every,
    not_suspect: o.notSuspect && !!d.not_suspect,
    passed: o.passed && !!d.passed,
    relation_type: o.relationType ? d.relation_type ?? '' : '',
    direction: o.direction ? d.direction ?? '' : '',
    target_type: o.targetType ? d.target_type ?? '' : '',
    target_query: '',
  }
  if (targetKind(out)) out.target_query = (d.target_query ?? '').trim()
  return out
}

/**
 * One-line description of what a rule requires, e.g. "Requirements
 * matching obligation:must need at least 1 linked test (not suspect)".
 * relation(type, direction) translates a relation type.
 */
export function ruleSummary(r: TraceRuleFields, t: T, relation: (type: string, direction: string) => string): string {
  if (isBuiltin(r.kind)) return t(`consistency.check.${r.kind}.description`)
  const who = r.selector ? t(`traceRules.summary.matching.${r.scope === 'test' ? 'test' : 'requirement'}`, { selector: r.selector }) : t(`traceRules.summary.all.${r.scope === 'test' ? 'test' : 'requirement'}`)
  let what: string
  switch (r.kind) {
    case 'relation':
      what = t('traceRules.summary.relation', { relation: r.relation_type ? relation(r.relation_type, r.direction || 'outgoing') : t('traceRules.anyRelation') }, r.min)
      break
    case 'item_link':
      what = t('traceRules.summary.itemLink', { target: r.target_type ? t(`traceRules.targetType.${r.target_type}`) : t('traceRules.anyTarget') }, r.min)
      break
    case 'verifies':
      what = t('traceRules.summary.verifies', {}, r.min)
      break
    default:
      what = t('traceRules.summary.verifiedBy', {}, r.min)
  }
  const conds: string[] = []
  const o = kindOptions(r.kind)
  if (o.notSuspect && r.not_suspect) conds.push(t('traceRules.cond.notSuspect'))
  if (o.passed && r.passed) conds.push(t('traceRules.cond.passed'))
  if (r.kind === 'item_link' && r.direction) conds.push(t(`traceRules.direction.${r.direction}`))
  if (r.kind === 'relation' && !r.relation_type && r.direction) conds.push(t(`traceRules.direction.${r.direction}`))
  if (r.target_query && targetKind(r)) conds.push(t('traceRules.cond.target', { query: r.target_query }))
  // "no suspect link at all": every trace must meet the conditions, none required
  if (r.every && r.min === 0) return t('traceRules.summary.colon', { who, what: t(`traceRules.summary.everyOf.${r.kind}`, { conds: conds.join(', ') || '–' }) })
  if (r.every) conds.push(t('traceRules.cond.every'))
  const text = t('traceRules.summary.sentence', { who, what })
  return conds.length ? `${text} (${conds.join(', ')})` : text
}

/** What a violating item lacks, e.g. "0 of 1 required" or "1 of 2 qualify, 2 required". */
export function violationText(v: Pick<TraceViolation, 'found' | 'required' | 'total' | 'check' | 'note' | 'related'>, t: T): string {
  if (v.check) return violationLine(v, t)
  if (v.total > v.found) return t('traceRules.violation.partial', { found: v.found, total: v.total, required: v.required })
  return t('traceRules.violation.missing', { found: v.found, required: v.required })
}

/** Whether a result counts as failing (disabled rules never fail). */
export function failing(r: Pick<TraceRuleResult, 'passed'> & { rule: Pick<TraceRule, 'enabled'> }): boolean {
  return r.rule.enabled && !r.passed
}

export interface ResultCounts {
  /** enabled rules */
  rules: number
  passed: number
  errors: number
  warnings: number
  disabled: number
  violations: number
}

/** Counts of evaluated rules (disabled rules apart). */
export function resultCounts(list: readonly TraceRuleResult[] | null | undefined): ResultCounts {
  const c: ResultCounts = { rules: 0, passed: 0, errors: 0, warnings: 0, disabled: 0, violations: 0 }
  for (const r of list ?? []) {
    if (!r.rule.enabled) {
      c.disabled++
      continue
    }
    c.rules++
    c.violations += r.violation_count
    if (r.passed) c.passed++
    else if (r.rule.severity === 'warning') c.warnings++
    else c.errors++
  }
  return c
}

/** Results shown: with onlyViolations only failing enabled rules; text filters by rule name or violating item. */
export function filterResults(list: readonly TraceRuleResult[] | null | undefined, onlyViolations: boolean, text = ''): TraceRuleResult[] {
  const q = text.trim().toLowerCase()
  return (list ?? []).filter((r) => {
    if (onlyViolations && !failing(r)) return false
    if (!q) return true
    return r.rule.name.toLowerCase().includes(q) || r.violations.some((v) => `${v.key} ${v.title}`.toLowerCase().includes(q))
  })
}

/** Route of a violating item. */
export function violationRoute(v: Pick<TraceViolation, 'kind' | 'id'>, projectId: string): RouteLocationRaw {
  if (v.kind !== 'test' && v.kind !== 'requirement') return itemRoute(v, projectId) ?? { name: 'traceability', params: { projectId } }
  return { name: v.kind === 'test' ? 'tests' : 'requirements', params: { projectId }, query: { item: v.id } }
}

/** Route of the rules tab of the traceability page. */
export function rulesRoute(projectId: string): RouteLocationRaw {
  return { name: 'traceability', params: { projectId }, query: { view: 'rules' } }
}

/** Severity of a PrimeVue tag / message for a rule severity. */
export function severityTone(severity: string): 'danger' | 'warn' {
  return severity === 'warning' ? 'warn' : 'danger'
}
