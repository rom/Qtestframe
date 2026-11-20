// Releases: exit criteria (types, parameters, display) and links to the
// items that block a criterion (mirrors internal/model ExitCriterionTypes).
import type { RouteLocationRaw } from 'vue-router'
import type { CriterionResult, ExitCriterion, ReleaseItem, TestRun } from './types'

export const RELEASE_STATUSES = ['planned', 'active', 'released', 'cancelled'] as const
export const CRITERION_TYPES = [
  'coverage',
  'passed',
  'mandatory_passed',
  'executed',
  'no_failed',
  'approved',
  'no_suspect',
  'no_open_defects',
  'risks_mitigated',
  'trace_rules',
] as const
/** Criteria measured in percent of a total, with a threshold. */
export const PERCENT_CRITERIA = ['coverage', 'passed', 'executed', 'approved'] as const
/** Defect severities from most to least severe (no_open_defects level). */
export const DEFECT_SEVERITIES = ['critical', 'major', 'minor', 'trivial'] as const
/** The least a release note must have when releasing despite a no-go. */
export const MIN_RELEASE_NOTE = 3

export type CriterionParam = 'threshold' | 'severity' | 'riskLevel' | 'ruleSeverity' | 'none'
/** Levels of the trace_rules criterion: errors only, or errors and warnings. */
export const RULE_LEVELS = ['error', 'warning'] as const

/** What a criterion type is parameterised by. */
export function criterionParam(type: string): CriterionParam {
  if ((PERCENT_CRITERIA as readonly string[]).includes(type)) return 'threshold'
  if (type === 'no_open_defects') return 'severity'
  if (type === 'risks_mitigated') return 'riskLevel'
  if (type === 'trace_rules') return 'ruleSeverity'
  return 'none'
}

/** A new criterion of a type with its default parameter. */
export function newCriterion(type: string): ExitCriterion {
  switch (criterionParam(type)) {
    case 'threshold':
      return { type, threshold: 100 }
    case 'severity':
      return { type, level: 'major' }
    case 'riskLevel':
      return { type, level: 'high' }
    case 'ruleSeverity':
      return { type, level: 'error' }
  }
  return { type }
}

/** Changes the type of a criterion, keeping a threshold where it still applies. */
export function changeCriterionType(c: ExitCriterion, type: string): ExitCriterion {
  const next = newCriterion(type)
  if (criterionParam(type) === 'threshold' && criterionParam(c.type) === 'threshold' && c.threshold) next.threshold = c.threshold
  return next
}

/** Criteria as sent to the API: only the parameter each type uses. */
export function cleanCriteria(list: readonly ExitCriterion[]): ExitCriterion[] {
  return list.map((c) => {
    switch (criterionParam(c.type)) {
      case 'threshold': {
        const n = Number(c.threshold)
        return { type: c.type, threshold: Number.isFinite(n) && n > 0 ? Math.min(100, n) : 100 }
      }
      case 'severity':
      case 'riskLevel':
        return c.level ? { type: c.type, level: c.level } : { type: c.type }
      case 'ruleSeverity':
        return { type: c.type, level: c.level === 'warning' ? 'warning' : 'error' }
    }
    return { type: c.type }
  })
}

/** Percent without needless decimals: 95 → "95 %", 92.54 → "92.5 %". */
export function percent(v: number | null | undefined): string {
  const n = Math.round((v ?? 0) * 10) / 10
  return `${n} %`
}

type T = (key: string, params?: Record<string, unknown>) => string

/**
 * One-line description of a criterion, e.g. "Coverage at least 95 %" or
 * "No open defects of severity major or higher". level(kind, value)
 * translates a severity or risk level.
 */
export function criterionText(c: ExitCriterion, t: T, level: (kind: 'severity' | 'riskLevel', value: string) => string): string {
  const base = `releasesView.criterion.${c.type}`
  switch (criterionParam(c.type)) {
    case 'threshold':
      return t(`${base}.withThreshold`, { threshold: percent(c.threshold || 100) })
    case 'severity':
    case 'riskLevel': {
      const kind = criterionParam(c.type) as 'severity' | 'riskLevel'
      return c.level ? t(`${base}.withLevel`, { level: level(kind, c.level) }) : t(`${base}.any`)
    }
    case 'ruleSeverity':
      return t(`${base}.${c.level === 'warning' ? 'withWarnings' : 'errorsOnly'}`)
  }
  return t(`${base}.label`)
}

export interface ResultValues {
  /** measured value ("92.5 %" or a count) */
  actual: string
  /** what it must be ("≥ 95 %" or "0") */
  target: string
}

/** Actual vs target of an evaluated criterion. */
export function resultValues(r: CriterionResult): ResultValues {
  if (criterionParam(r.type) === 'threshold') return { actual: percent(r.actual), target: `≥ ${percent(r.threshold || 100)}` }
  return { actual: String(Math.round(r.actual ?? 0)), target: '0' }
}

/** Route of an item a criterion refers to. */
export function releaseItemRoute(item: Pick<ReleaseItem, 'kind' | 'id'>, projectId: string): RouteLocationRaw {
  const params = { projectId }
  switch (item.kind) {
    case 'test':
      return { name: 'tests', params, query: { item: item.id } }
    case 'risk':
      return { name: 'risks', params, query: { risk: item.id } }
    case 'defect':
      return { name: 'defects', params, query: { defect: item.id } }
    case 'trace_rule':
      return { name: 'traceability', params, query: { view: 'rules' } }
  }
  return { name: 'requirements', params, query: { item: item.id } }
}

/** Counts of met and unmet criteria. */
export function criteriaCounts(list: readonly Pick<CriterionResult, 'met'>[] | null | undefined): { met: number; unmet: number } {
  const met = (list ?? []).filter((c) => c.met).length
  return { met, unmet: (list ?? []).length - met }
}

/** Runs of a release. */
export function runsOfRelease<R extends Pick<TestRun, 'release_id'>>(runs: readonly R[], releaseId: string): R[] {
  return runs.filter((r) => r.release_id === releaseId)
}

/** Name of a run's release: release_name when the API sends it, else looked up by id. */
export function runReleaseName(run: Pick<TestRun, 'release_id' | 'release_name'>, names: ReadonlyMap<string, string>): string {
  if (!run.release_id) return ''
  return run.release_name || names.get(run.release_id) || ''
}

/** The name of the baseline created when a release is marked released (see api releaseBaseline). */
export function releaseBaselineName(r: { name: string; version?: string }): string {
  return r.version && r.version !== r.name ? `Release ${r.name} (${r.version})` : `Release ${r.name}`
}

/** Whether the release can still be marked released. */
export function canRelease(status: string): boolean {
  return status === 'planned' || status === 'active'
}
