// Privacy and data retention: types of GET /admin/retention, the legal
// hold of a project and its releases, the erasure of an account, and the
// helpers of their views.

export const RETENTION_CATEGORIES = ['runs', 'logs', 'notifications', 'sessions', 'chat', 'exploratory'] as const
export type RetentionCategory = (typeof RETENTION_CATEGORIES)[number]

/** Keys of an organisation's retention override (TenantPolicy.retention). */
export const RETENTION_OVERRIDE_KEYS = ['runs_days', 'runs_keep_latest', 'logs_days', 'notifications_days', 'sessions_days', 'chat_days', 'exploratory_days'] as const
export type RetentionOverrideKey = (typeof RETENTION_OVERRIDE_KEYS)[number]

/** The longest retention period in days (as on the server). */
export const MAX_RETENTION_DAYS = 36500

export interface RetentionCount {
  category: RetentionCategory
  days: number
  count: number
  held: number
}

export interface RetentionReport {
  /** "" = the data of no organisation (sign-in sessions and log) */
  tenant_id: string
  tenant_name?: string
  dry_run: boolean
  policy: { days: Record<string, number>; runs_keep_latest: number; overridden?: string[] }
  counts: RetentionCount[]
  at: string
}

export interface LegalHold {
  on: boolean
  reason: string
  by?: string
  at?: string | null
}

export interface HeldItem {
  kind: 'project' | 'release'
  id: string
  tenant_id: string
  tenant_name: string
  project_id: string
  project_key: string
  project_name: string
  name: string
  hold: LegalHold
}

export interface RetentionView {
  enabled: boolean
  hour: number
  last_run: string | null
  next_run: string | null
  reports: RetentionReport[]
  holds: HeldItem[]
}

export interface ReleaseHold {
  id: string
  name: string
  version: string
  hold: LegalHold
}

export interface LegalHoldView {
  project: LegalHold
  releases: ReleaseHold[]
  can_edit: boolean
}

export interface ErasureResult {
  user_id: string
  pseudonym: string
  email: string
  deleted: Record<string, number>
  rewritten: number
  kept: Record<string, number>
}

/** The word to type before "Purge now". */
export const PURGE_WORD = 'PURGE'

/** The count of a category in a report (undefined when the report has none). */
export function countOf(r: RetentionReport, c: RetentionCategory): RetentionCount | undefined {
  return r.counts.find((x) => x.category === c)
}

/** What a report deletes in all; held is what is kept by holds and signatures. */
export function reportTotals(r: RetentionReport): { count: number; held: number } {
  return r.counts.reduce((a, c) => ({ count: a.count + c.count, held: a.held + c.held }), { count: 0, held: 0 })
}

/** Totals over all reports. */
export function grandTotals(reports: RetentionReport[]): { count: number; held: number } {
  return reports.map(reportTotals).reduce((a, b) => ({ count: a.count + b.count, held: a.held + b.held }), { count: 0, held: 0 })
}

/** Whether a policy deletes nothing at all (every period 0 = forever). */
export function keepsForever(r: RetentionReport): boolean {
  return RETENTION_CATEGORIES.every((c) => !r.policy.days[c])
}

/** Whether the typed confirmation of an erasure matches the account's address (case and spaces ignored). */
export function erasureConfirmed(typed: string, email: string): boolean {
  return typed.trim().toLowerCase() === email.trim().toLowerCase() && email.trim() !== ''
}

/** The override form: one entry per key, null = use the system setting. */
export type OverrideForm = Record<RetentionOverrideKey, number | null>

export function overrideForm(r: Record<string, number> | null | undefined): OverrideForm {
  const out = {} as OverrideForm
  for (const k of RETENTION_OVERRIDE_KEYS) out[k] = r && typeof r[k] === 'number' ? r[k] : null
  return out
}

/** The override to send: only the keys set (an empty override is omitted). */
export function overrideValue(f: OverrideForm): Record<string, number> | undefined {
  const out: Record<string, number> = {}
  for (const k of RETENTION_OVERRIDE_KEYS) {
    const v = f[k]
    if (v !== null && v !== undefined && Number.isFinite(v)) out[k] = Math.max(0, Math.min(MAX_RETENTION_DAYS, Math.round(v)))
  }
  return Object.keys(out).length ? out : undefined
}
