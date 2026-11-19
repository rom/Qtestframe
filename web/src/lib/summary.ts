// Run summaries (map of result status → count).
import { RESULT_COLORS } from './colors'
import { percent } from './format'

export const RESULT_ORDER = ['passed', 'failed', 'blocked', 'skipped', 'in_progress', 'not_run']

export interface SummarySegment {
  status: string
  count: number
  pct: number
  color: string
}

export interface RunSummary {
  total: number
  executed: number
  passed: number
  failed: number
  progress: number
  passRate: number
  segments: SummarySegment[]
}

/**
 * Summarise a run's result counts. A `total` key, when present, is used as
 * the total; otherwise counts are summed.
 */
export function summarize(summary: Record<string, number> | null | undefined): RunSummary {
  const s = summary ?? {}
  const counts: Record<string, number> = {}
  let sum = 0
  for (const [k, v] of Object.entries(s)) {
    if (k === 'total' || typeof v !== 'number') continue
    counts[k] = v
    sum += v
  }
  const total = typeof s.total === 'number' && s.total >= sum ? s.total : sum
  if (total > sum) counts.not_run = (counts.not_run ?? 0) + (total - sum)
  const keys = [...RESULT_ORDER, ...Object.keys(counts).filter((k) => !RESULT_ORDER.includes(k)).sort()]
  const segments = keys
    .filter((k) => (counts[k] ?? 0) > 0)
    .map((k) => ({ status: k, count: counts[k], pct: total ? (counts[k] / total) * 100 : 0, color: RESULT_COLORS[k] ?? '#64748b' }))
  const executed = total - (counts.not_run ?? 0) - (counts.in_progress ?? 0)
  const passed = counts.passed ?? 0
  return {
    total,
    executed,
    passed,
    failed: counts.failed ?? 0,
    progress: percent(executed, total),
    passRate: percent(passed, executed),
    segments,
  }
}

/**
 * Overall execution status derived from step results (mirrors the server):
 * failed > blocked > in_progress (partially done) > passed > skipped > not_run.
 */
export function deriveStatus(stepStatuses: readonly string[]): string {
  if (stepStatuses.length === 0) return 'not_run'
  if (stepStatuses.includes('failed')) return 'failed'
  if (stepStatuses.includes('blocked')) return 'blocked'
  const pending = stepStatuses.filter((s) => s === 'not_run' || s === 'in_progress' || !s).length
  if (pending === stepStatuses.length) return 'not_run'
  if (pending > 0) return 'in_progress'
  if (stepStatuses.includes('passed')) return 'passed'
  return 'skipped'
}
