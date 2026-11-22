// Quality metrics: the ranges offered, key figures and series of the
// charts (mirrors internal/analytics quality.go).
import type { QualityPeriod, QualityReport } from './types'

/** Ranges (days) the API accepts. */
export const QUALITY_RANGES = [30, 90, 180, 365] as const
export type QualityRange = (typeof QUALITY_RANGES)[number]

/** Phases a defect can be found in (internal/model DefectPhases). */
export const DEFECT_PHASES = ['requirements', 'design', 'development', 'test', 'acceptance', 'production'] as const

/** Result age buckets (days). */
export const AGE_BUCKETS = ['0-7', '8-30', '31-90', '91-180', '181+'] as const

/** The key figures, in display order. */
export const QUALITY_KPIS = ['volatility', 'density', 'escaped', 'effectiveness', 'stale', 'review', 'fix', 'quality'] as const
export type QualityKpi = (typeof QUALITY_KPIS)[number]

export interface KpiValue {
  id: QualityKpi
  /** the headline number */
  value: number
  /** a percentage (shown with %) */
  percent?: boolean
  /** hours (shown as a duration) */
  hours?: boolean
  /** parameters of the explanation line */
  params: Record<string, number>
}

/** Key figures of a report. */
export function qualityKpis(r: QualityReport): KpiValue[] {
  return [
    { id: 'volatility', value: r.volatility.per_requirement, params: { changes: r.volatility.changes, changed: r.volatility.changed } },
    { id: 'density', value: r.defect_density.per_requirement, params: { defects: r.defect_density.defects, requirements: r.defect_density.requirements } },
    { id: 'escaped', value: r.escaped.percent, percent: true, params: { escaped: r.escaped.escaped, defects: r.escaped.defects } },
    { id: 'effectiveness', value: r.effectiveness.per_hour, params: { perTest: r.effectiveness.per_test, hours: r.effectiveness.hours } },
    { id: 'stale', value: r.result_age.stale_percent, percent: true, params: { stale: r.result_age.stale, days: r.result_age.stale_days, never: r.result_age.never } },
    { id: 'review', value: r.review_turnaround.median_hours, hours: true, params: { count: r.review_turnaround.count, pending: r.review_turnaround.pending } },
    { id: 'fix', value: r.fix_time.mean_hours, hours: true, params: { count: r.fix_time.count, open: r.fix_time.open } },
    { id: 'quality', value: r.requirement_quality.percent, percent: true, params: { n: r.requirement_quality.with_warnings, total: r.requirement_quality.requirements } },
  ]
}

/** Values of a trend (value, or count with byCount). */
export function trendValues(tr: readonly QualityPeriod[] | null | undefined, byCount = false): number[] {
  return (tr ?? []).map((p) => (byCount ? p.count : p.value))
}

/** A duration in hours as "3.5 h" or, from two days on, "4.2 d". */
export function hoursText(h: number, fmt: (n: number) => string, units: { h: string; d: string }): string {
  if (h >= 48) return `${fmt(Math.round((h / 24) * 10) / 10)} ${units.d}`
  return `${fmt(Math.round(h * 10) / 10)} ${units.h}`
}
