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
/** Transitive impact: depth (model.DefaultImpactDepth / MaxImpactDepth), modes and the other edge kinds with a rule. */
export const DEFAULT_IMPACT_DEPTH = 3
export const MAX_IMPACT_DEPTH = 10
export const IMPACT_MODES = ['suspect', 'review'] as const
export const IMPACT_EDGE_KINDS = ['verifies', 'calls', 'test_depends_on', 'item_link', 'risk', 'defect', 'variant', 'code'] as const
/** Edge kinds whose rule can mark suspect (the others only recommend a review). */
export const MARKABLE_EDGES = ['verifies']
/** What saving does with the impact (?impact=). */
export const IMPACT_CHOICES = ['suspect', 'notify', 'skip'] as const
export type ImpactChoice = (typeof IMPACT_CHOICES)[number]

export interface ImpactRule {
  direction: string
  mode: string
}
export const DEFAULT_IMPACT_RULES: Record<string, ImpactRule> = {
  verifies: { direction: 'both', mode: 'suspect' },
  calls: { direction: 'incoming', mode: 'review' },
  test_depends_on: { direction: 'incoming', mode: 'review' },
  item_link: { direction: 'both', mode: 'review' },
  risk: { direction: 'both', mode: 'review' },
  defect: { direction: 'incoming', mode: 'review' },
  variant: { direction: 'outgoing', mode: 'review' },
  code: { direction: 'incoming', mode: 'review' },
}

export interface CorrelationSettings {
  relation_impact: Record<string, string>
  flag_issues: boolean
  /** 0: results never go stale by age */
  stale_after_days: number
  stale_on_change: boolean
  blocking_severities: string[]
  warning_severities: string[]
  regression_action: string
  /** 1..10 */
  impact_depth: number
  /** relation type → suspect | review */
  relation_modes: Record<string, string>
  impact_rules: Record<string, ImpactRule>
  /** what a changed variant applicability does: review (default) | suspect | off */
  applicability_impact: string
}

/** The values of CorrelationSettings.applicability_impact. */
export const APPLICABILITY_IMPACTS = ['review', 'suspect', 'off']

/** One step of the path along which a change reaches an item. */
export interface ImpactStep {
  kind: string
  id: string
  key: string
  edge?: string
  direction?: string
}

/** A changed item of an impact. */
export interface ImpactRef {
  kind: string
  id: string
  key: string
  title: string
  review_only?: boolean
  fields?: string[]
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
  depth?: number
  /** suspect | review */
  mode?: string
  path?: ImpactStep[]
  owner?: string
  status?: string
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
  risks?: ImpactItem[]
  defects?: ImpactItem[]
  links?: ImpactItem[]
  variants?: ImpactItem[]
  code?: ImpactItem[]
  total: number
  roots?: ImpactRef[]
  depth?: number
  suspect?: number
  review?: number
  by_kind?: Record<string, number>
  by_depth?: Record<string, number>
}

/** The answer of a change sent with ?dry_run=impact. */
export interface ImpactDryRun {
  dry_run: 'impact'
  status: number
  impact: ChangeImpact
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
  test: ['title', 'description', 'preconditions', 'steps', 'expected_result', 'script', 'execution_type', 'format', 'gherkin'],
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

export type ImpactSection = 'requirements' | 'tests' | 'risks' | 'defects' | 'links' | 'issues' | 'documents' | 'results' | 'variants' | 'code'
export const IMPACT_SECTIONS: ImpactSection[] = ['requirements', 'tests', 'risks', 'defects', 'links', 'issues', 'documents', 'results', 'variants', 'code']

/** The non-empty sections of an impact, with the number of items not yet affected. */
export function impactSections(imp: ChangeImpact | null): { section: ImpactSection; items: ImpactItem[]; fresh: number }[] {
  if (!imp) return []
  return IMPACT_SECTIONS.map((section) => {
    const items = imp[section] ?? []
    return { section, items, fresh: items.filter((i) => !i.already).length }
  }).filter((s) => s.items.length > 0)
}

/** The items of an impact grouped by section, then by depth (the tree of the confirmation dialog), with counts. */
export function impactTree(imp: ChangeImpact | null): { section: ImpactSection; count: number; depths: { depth: number; items: ImpactItem[] }[] }[] {
  return impactSections(imp).map((s) => {
    const by = new Map<number, ImpactItem[]>()
    for (const it of s.items) {
      const d = it.depth ?? 1
      if (!by.has(d)) by.set(d, [])
      by.get(d)!.push(it)
    }
    return { section: s.section, count: s.items.length, depths: [...by.entries()].sort((a, b) => a[0] - b[0]).map(([depth, items]) => ({ depth, items })) }
  })
}

/** Items per depth of an impact, in depth order. */
export function depthCounts(imp: ChangeImpact | null): { depth: number; count: number }[] {
  const m = new Map<number, number>()
  for (const s of impactSections(imp)) for (const it of s.items) m.set(it.depth ?? 1, (m.get(it.depth ?? 1) ?? 0) + 1)
  return [...m.entries()].sort((a, b) => a[0] - b[0]).map(([depth, count]) => ({ depth, count }))
}

/** All items of an impact in section order. */
export function impactItems(imp: ChangeImpact | null): (ImpactItem & { section: ImpactSection })[] {
  return impactSections(imp).flatMap((s) => s.items.map((it) => ({ ...it, section: s.section })))
}

/** The path of an item as text, with edge labels from label(edge, direction): A → refines → B → verifies → T-12. */
export function pathText(path: ImpactStep[] | undefined, label: (edge: string, direction?: string) => string = (e) => e): string {
  if (!path?.length) return ''
  return path.map((st, i) => (i === 0 ? st.key : `${label(st.edge ?? '', st.direction)} → ${st.key}`)).join(' → ')
}

/** Nodes and edges of the impact graph (the changed items and every affected item, each reached along the last step of its path). */
export function impactGraph(imp: ChangeImpact | null): {
  nodes: { id: string; kind: string; key: string; title: string; depth: number; mode: string; root: boolean }[]
  edges: { id: string; source: string; target: string; type: string; direction: string }[]
} {
  const nodes = new Map<string, { id: string; kind: string; key: string; title: string; depth: number; mode: string; root: boolean }>()
  const edges = new Map<string, { id: string; source: string; target: string; type: string; direction: string }>()
  for (const r of imp?.roots ?? []) nodes.set(`${r.kind}:${r.id}`, { id: `${r.kind}:${r.id}`, kind: r.kind, key: r.key, title: r.title, depth: 0, mode: 'root', root: true })
  for (const it of impactItems(imp)) {
    const id = `${it.kind}:${it.id}`
    if (!nodes.has(id)) nodes.set(id, { id, kind: it.kind, key: it.key, title: it.title ?? '', depth: it.depth ?? 1, mode: it.mode ?? 'suspect', root: false })
    const p = it.path ?? []
    if (p.length >= 2) {
      const from = p[p.length - 2]!
      const last = p[p.length - 1]!
      const source = `${from.kind}:${from.id}`
      if (!nodes.has(source)) nodes.set(source, { id: source, kind: from.kind, key: from.key, title: '', depth: p.length - 2, mode: 'suspect', root: false })
      edges.set(`${source}>${id}`, { id: `${source}>${id}`, source, target: id, type: last.edge ?? '', direction: last.direction ?? '' })
    }
  }
  return { nodes: [...nodes.values()], edges: [...edges.values()] }
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
    impact_depth: s?.impact_depth || DEFAULT_IMPACT_DEPTH,
    relation_modes: Object.fromEntries(RELATION_TYPES.map((t) => [t, s?.relation_modes?.[t] === 'review' ? 'review' : 'suspect'])),
    impact_rules: Object.fromEntries(
      IMPACT_EDGE_KINDS.map((k) => {
        const r = s?.impact_rules?.[k] ?? DEFAULT_IMPACT_RULES[k]!
        return [k, { direction: r.direction || 'off', mode: MARKABLE_EDGES.includes(k) && r.mode === 'suspect' ? 'suspect' : 'review' }]
      }),
    ),
    applicability_impact: APPLICABILITY_IMPACTS.includes(s?.applicability_impact ?? '') ? s!.applicability_impact! : 'review',
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
