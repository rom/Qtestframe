// Change and defect correlations: change impact across relations, result
// staleness, defects that block requirements and regressions (mirrors
// internal/model/correlations.go and internal/store/evidence.go).

/** Impact directions of a relation type (CorrelationSettings.relation_impact). */
export const IMPACT_DIRECTIONS = ['incoming', 'outgoing', 'both', 'off'] as const
/** What happens when a test that verified a fixed defect fails again. */
export const REGRESSION_ACTIONS = ['reopen', 'new_defect', 'off'] as const
/** Relation types (model.RelationTypes). */
export const RELATION_TYPES = ['refines', 'depends_on', 'provides', 'derived_from', 'conflicts_with', 'duplicates', 'relates_to'] as const
export const SEVERITIES = ['critical', 'major', 'minor', 'trivial'] as const
export const DEFAULT_STALE_AFTER_DAYS = 90

export interface CorrelationSettings {
  relation_impact: Record<string, string>
  flag_issues: boolean
  /** 0: results never go stale by age */
  stale_after_days: number
  stale_on_change: boolean
  blocking_severities: string[]
  warning_severities: string[]
  regression_action: string
}

export interface ImpactItem {
  /** requirement | test | issue | document | result */
  kind: string
  id: string
  key: string
  title?: string
  relation?: string
  direction?: string
  via?: string
  already?: boolean
  requirement_id?: string
  test_id?: string
  result?: string
  url?: string
}

export interface ChangeImpact {
  kind: string
  id: string
  key: string
  title: string
  requirements: ImpactItem[]
  tests: ImpactItem[]
  issues: ImpactItem[]
  documents: ImpactItem[]
  results: ImpactItem[]
  total: number
}

export interface DefectBlock {
  defect_id: string
  key: string
  title: string
  severity: string
  status: string
  /** block | warn */
  level: string
  /** requirement or the key of the test */
  via: string
}

export interface DefectRegression {
  id: string
  defect_id: string
  defect_key: string
  regression_defect_id: string | null
  regression_defect_key?: string
  test_case_id: string
  test_key: string
  execution_id: string
  run_id?: string
  run_name?: string
  /** reopened | created */
  action: string
  previous_status: string
  created_by: string
  created_at: string
}

export interface StaleResult {
  test_id: string
  key: string
  title: string
  result: string
  stale: boolean
  /** age | test_changed | requirement_changed */
  reason?: string
  recorded_at?: string
}

export interface BaselineImpactChange {
  entity_type: string
  entity_id: string
  key: string
  title: string
  changed_fields?: string[]
  impact: ChangeImpact
}

export interface BaselineImpact {
  baseline: string
  changes: BaselineImpactChange[]
  totals: Record<string, number>
}

/** The fields whose change makes links suspect and results stale (store.RequirementContentFields / TestContentFields). */
export const CONTENT_FIELDS: Record<string, readonly string[]> = {
  requirement: ['title', 'description', 'obligation', 'criteria'],
  test: ['title', 'description', 'preconditions', 'steps', 'expected_result', 'script', 'execution_type'],
}

/** Normalises a field value for comparing (criteria and steps without ids and positions). */
function norm(field: string, v: unknown): string {
  if (Array.isArray(v)) {
    if (field === 'criteria') return JSON.stringify(v.map((c: Record<string, unknown>) => [c.description ?? '', c.method ?? '', c.threshold ?? '']))
    if (field === 'steps') return JSON.stringify(v.map((s: Record<string, unknown>) => [s.action ?? '', s.expected ?? '', s.data ?? '', s.call_test_id ?? null]))
  }
  return JSON.stringify(v ?? '')
}

/** The content fields that differ between two versions of a requirement or test (the ones that carry the change impact). */
export function contentChanges(kind: string, before: Record<string, unknown> | null, after: Record<string, unknown>): string[] {
  if (!before) return []
  return (CONTENT_FIELDS[kind] ?? []).filter((f) => f in after && norm(f, before[f]) !== norm(f, after[f]))
}

export type ImpactSection = 'requirements' | 'tests' | 'issues' | 'documents' | 'results'
export const IMPACT_SECTIONS: ImpactSection[] = ['requirements', 'tests', 'issues', 'documents', 'results']

/** The non-empty sections of an impact, with the number of items not yet affected. */
export function impactSections(imp: ChangeImpact | null): { section: ImpactSection; items: ImpactItem[]; fresh: number }[] {
  if (!imp) return []
  return IMPACT_SECTIONS.map((section) => {
    const items = imp[section] ?? []
    return { section, items, fresh: items.filter((i) => !i.already).length }
  }).filter((s) => s.items.length > 0)
}

/** Whether saving the change should be confirmed first: the substance changed and something becomes suspect, out of date or stale. */
export function needsImpactConfirm(changed: string[], imp: ChangeImpact | null): boolean {
  return changed.length > 0 && !!imp && imp.total > 0
}

/** Counts of blocking defects and warnings. */
export function blockCounts(blocks: DefectBlock[]): { blocking: number; warnings: number } {
  return { blocking: blocks.filter((b) => b.level === 'block').length, warnings: blocks.filter((b) => b.level !== 'block').length }
}

/** A complete settings object from what the server sent (defaults for missing fields). */
export function withDefaults(s: Partial<CorrelationSettings> | null | undefined): CorrelationSettings {
  const impact: Record<string, string> = {}
  for (const t of RELATION_TYPES) impact[t] = s?.relation_impact?.[t] ?? (['refines', 'derived_from', 'depends_on'].includes(t) ? 'incoming' : 'off')
  return {
    relation_impact: impact,
    flag_issues: s?.flag_issues ?? true,
    stale_after_days: s?.stale_after_days ?? DEFAULT_STALE_AFTER_DAYS,
    stale_on_change: s?.stale_on_change ?? true,
    blocking_severities: [...(s?.blocking_severities ?? ['critical', 'major'])],
    warning_severities: [...(s?.warning_severities ?? ['minor'])],
    regression_action: s?.regression_action ?? 'reopen',
  }
}

/** The block level of a severity in the settings: block, warn or none. */
export function severityLevel(s: CorrelationSettings, severity: string): 'block' | 'warn' | 'none' {
  if (s.blocking_severities.includes(severity)) return 'block'
  if (s.warning_severities.includes(severity)) return 'warn'
  return 'none'
}

/** Sets the block level of a severity (a severity is in at most one list). */
export function setSeverityLevel(s: CorrelationSettings, severity: string, level: 'block' | 'warn' | 'none'): CorrelationSettings {
  const blocking = s.blocking_severities.filter((x) => x !== severity)
  const warning = s.warning_severities.filter((x) => x !== severity)
  if (level === 'block') blocking.push(severity)
  if (level === 'warn') warning.push(severity)
  const order = (list: string[]) => SEVERITIES.filter((x) => list.includes(x))
  return { ...s, blocking_severities: order(blocking), warning_severities: order(warning) }
}
