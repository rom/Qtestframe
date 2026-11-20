// Plain-language explanations of why a test cannot (or should not yet) be
// executed (see Readiness from the server): what is wrong, what would fix it
// and which execution setting decides whether it blocks.
import type { Readiness, ReadinessIssue } from './types'

type T = (key: string, params?: Record<string, unknown>) => string
type Label = (group: 'status' | 'result', value: string) => string

export interface IssueExplanation {
  /** what is wrong, e.g. "DEMO-TC-2 is Draft — only tests marked Ready can be executed." */
  reason: string
  /** what to do about it */
  fix: string
  /** the setting that decides, and whether it blocks or only warns */
  rule: string
}

/** "a", "a or b", "a, b or c" in the active language */
export function orList(items: string[], t: T): string {
  if (items.length <= 1) return items[0] ?? ''
  return t('readiness.orList', { list: items.slice(0, -1).join(', '), last: items[items.length - 1] })
}

const SETTING_KEYS: Record<string, string> = {
  allow_unready_tests: 'execSettings.allowUnreadyTests',
  allow_unready_requirements: 'execSettings.allowUnreadyRequirements',
  allow_unmet_dependencies: 'execSettings.allowUnmetDependencies',
}

export function explainIssue(i: ReadinessIssue, t: T, label: Label): IssueExplanation {
  const required = i.required ?? []
  const setting = SETTING_KEYS[i.setting ?? ''] ? t(SETTING_KEYS[i.setting!]) : ''
  const rule = setting ? t(i.blocking ? 'readiness.ruleBlocks' : 'readiness.ruleAllows', { setting }) : ''
  switch (i.kind) {
    case 'test_status': {
      const status = label('status', i.status)
      const fixKey = i.status === 'draft' ? 'readiness.fixTestDraft' : i.status === 'review' ? 'readiness.fixTestReview' : 'readiness.fixTestOther'
      return { reason: t('readiness.reasonTest', { key: i.key, status, required: orList(required.map((s) => label('status', s)), t) || label('status', 'ready') }), fix: t(fixKey), rule }
    }
    case 'requirement_status': {
      const status = label('status', i.status)
      const list = orList(required.map((s) => label('status', s)), t)
      const before = ['draft', 'review'].includes(i.status) && required.includes('approved')
      const fixKey = before ? 'readiness.fixRequirementApprove' : ['rejected', 'obsolete'].includes(i.status) ? 'readiness.fixRequirementDead' : 'readiness.fixRequirementOther'
      return { reason: t('readiness.reasonRequirement', { key: i.key, status, required: list }), fix: t(fixKey, { required: list }), rule }
    }
    default: {
      const result = label('result', i.status || 'not_run')
      return {
        reason: t(i.in_run ? 'readiness.reasonDependencyInRun' : 'readiness.reasonDependency', { key: i.key, result }),
        fix: t('readiness.fixDependency', { key: i.key }),
        rule,
      }
    }
  }
}

/** One line for tooltips and error messages: the reasons that block, else all. */
export function readinessSummary(rd: Readiness | null | undefined, t: T, label: Label): string {
  if (!rd || !rd.issues.length) return ''
  const blocking = rd.issues.filter((i) => i.blocking)
  const list = blocking.length ? blocking : rd.issues
  return list.map((i) => explainIssue(i, t, label).reason).join(' ')
}

/** The readiness in a 409 not_ready error body, if any. */
export function readinessOfError(e: unknown): Readiness | null {
  const err = e as { status?: number; code?: string; body?: { readiness?: Readiness } } | null
  if (!err || err.status !== 409 || err.code !== 'not_ready') return null
  return err.body?.readiness ?? null
}
