// Risks: likelihood × impact scores, levels, the 5×5 heat map and filters
// of the risk register (mirrors internal/model RiskLevel).
import type { Severity } from './colors'
import type { Risk } from './types'

/** Risk levels from lowest to highest. */
export const RISK_LEVELS = ['low', 'medium', 'high', 'critical'] as const
/** States of a risk; identified and mitigating still need action. */
export const RISK_STATUSES = ['identified', 'mitigating', 'mitigated', 'accepted', 'closed'] as const
export const RISK_OPEN_STATUSES = ['identified', 'mitigating'] as const
/** Likelihood and impact go from 1 to 5. */
export const RISK_SCALE = [1, 2, 3, 4, 5] as const

const clamp = (n: number) => Math.min(5, Math.max(1, Math.round(Number.isFinite(n) ? n : 1)))

/** likelihood × impact, each clamped to 1–5. */
export function riskScore(likelihood: number, impact: number): number {
  return clamp(likelihood) * clamp(impact)
}

/** The level of a score: low 1–4, medium 5–9, high 10–16, critical 17–25. */
export function levelOfScore(score: number): (typeof RISK_LEVELS)[number] {
  if (score >= 17) return 'critical'
  if (score >= 10) return 'high'
  if (score >= 5) return 'medium'
  return 'low'
}

export function riskLevel(likelihood: number, impact: number): (typeof RISK_LEVELS)[number] {
  return levelOfScore(riskScore(likelihood, impact))
}

/** Tag severity of a risk level ("" = none). */
export function riskLevelSeverity(level: string | null | undefined): Severity {
  switch (level) {
    case 'critical':
      return 'danger'
    case 'high':
      return 'warn'
    case 'medium':
      return 'info'
    case 'low':
      return 'success'
  }
  return 'secondary'
}

/** Tag severity of a risk status. */
export function riskStatusSeverity(status: string | null | undefined): Severity {
  switch (status) {
    case 'identified':
      return 'warn'
    case 'mitigating':
      return 'info'
    case 'mitigated':
      return 'success'
    case 'accepted':
      return 'contrast'
  }
  return 'secondary'
}

/** Whether a risk still needs action. */
export function riskOpen(status: string | null | undefined): boolean {
  return (RISK_OPEN_STATUSES as readonly string[]).includes(status ?? '')
}

/** Rank of a level for sorting (-1 for none). */
export function levelRank(level: string | null | undefined): number {
  return (RISK_LEVELS as readonly string[]).indexOf(level ?? '')
}

export interface HeatCell {
  likelihood: number
  impact: number
  score: number
  level: string
  count: number
  /** risks in the cell that still need action */
  open: number
}

export interface HeatRow {
  likelihood: number
  cells: HeatCell[]
}

/**
 * The 5×5 heat map: one row per likelihood from 5 (top) to 1, one cell per
 * impact from 1 to 5. Closed risks are left out unless includeClosed.
 */
export function heatGrid(risks: readonly Pick<Risk, 'likelihood' | 'impact' | 'status'>[], includeClosed = false): HeatRow[] {
  const rows: HeatRow[] = [...RISK_SCALE].reverse().map((l) => ({
    likelihood: l,
    cells: RISK_SCALE.map((i) => ({ likelihood: l, impact: i, score: l * i, level: levelOfScore(l * i), count: 0, open: 0 })),
  }))
  for (const r of risks) {
    if (!includeClosed && r.status === 'closed') continue
    const cell = rows[5 - clamp(r.likelihood)].cells[clamp(r.impact) - 1]
    cell.count++
    if (riskOpen(r.status)) cell.open++
  }
  return rows
}

/** Where a residual score comes from (internal/model ResidualOf). */
export const RESIDUAL_SOURCES = ['inherent', 'verified', 'manual'] as const

type ResidualFields = Pick<Risk, 'likelihood' | 'impact'> & Partial<Pick<Risk, 'residual_likelihood' | 'residual_impact' | 'residual_source'>>

/** The likelihood and impact in effect after mitigation: the residual assessment once it applies, else the inherent one. */
export function residualPoint(r: ResidualFields): { likelihood: number; impact: number } {
  if (r.residual_source && r.residual_source !== 'inherent' && r.residual_likelihood && r.residual_impact) return { likelihood: r.residual_likelihood, impact: r.residual_impact }
  return { likelihood: r.likelihood, impact: r.impact }
}

/** Risks placed at their residual point (for heatGrid "after mitigation"). */
export function atResidual<T extends ResidualFields>(risks: readonly T[]): T[] {
  return risks.map((r) => ({ ...r, ...residualPoint(r) }))
}

/** Whether a level is above a risk acceptance threshold (default medium). */
export function aboveThreshold(level: string, threshold: string | null | undefined): boolean {
  const t = levelRank(threshold) < 0 ? levelRank('medium') : levelRank(threshold)
  return levelRank(level) > t
}

export interface RiskFilter {
  /** filter the heat-map cell on the residual point instead of the inherent one */
  residual?: boolean
  likelihood?: number | null
  impact?: number | null
  levels?: readonly string[]
  statuses?: readonly string[]
  text?: string
}

/** Risks matching a filter (heat-map cell, levels, statuses, free text). */
export function filterRisks<T extends Risk>(risks: readonly T[], f: RiskFilter): T[] {
  const text = (f.text ?? '').trim().toLowerCase()
  return risks.filter(
    (r) =>
      (!f.likelihood || (f.residual ? residualPoint(r).likelihood : r.likelihood) === f.likelihood) &&
      (!f.impact || (f.residual ? residualPoint(r).impact : r.impact) === f.impact) &&
      (!f.levels?.length || f.levels.includes(r.level)) &&
      (!f.statuses?.length || f.statuses.includes(r.status)) &&
      (!text || [r.key, r.title, r.category, r.owner, r.description, r.mitigation].some((v) => (v ?? '').toLowerCase().includes(text))),
  )
}

/** Counts of risks per level (all levels present). */
export function levelCounts(risks: readonly Pick<Risk, 'level'>[]): Record<string, number> {
  const out: Record<string, number> = Object.fromEntries(RISK_LEVELS.map((l) => [l, 0]))
  for (const r of risks) if (r.level in out) out[r.level]++
  return out
}

/** Highest level among risks ("" when none). */
export function highestLevel(risks: readonly Pick<Risk, 'level'>[]): string {
  let best = -1
  for (const r of risks) best = Math.max(best, levelRank(r.level))
  return best < 0 ? '' : RISK_LEVELS[best]
}
