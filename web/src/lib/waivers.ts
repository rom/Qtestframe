// Waivers and deviations (mirrors internal/model Waiver): states, filters
// and who may do what.
import type { Severity } from './colors'

export const WAIVER_TYPES = ['waiver', 'deviation'] as const
export const WAIVER_STATUSES = ['draft', 'requested', 'approved', 'rejected', 'expired', 'revoked'] as const

export interface WaiverApproval {
  id: string
  waiver_id: string
  user_id: string
  user_email: string
  user_name: string
  /** approve | reject */
  decision: string
  comment: string
  content_hash: string
  signed_at: string
  /** the waiver still has the content that was signed */
  valid: boolean
}

export interface Waiver {
  id: string
  project_id: string
  key: string
  type: string
  title: string
  release_id: string | null
  release_name?: string
  requirement_ids: string[]
  requirements: { id: string; key: string; title: string }[]
  /** limits the waiver to these variants (each with the variants below it); empty: every variant */
  variant_ids?: string[]
  variants?: { id: string; name: string }[]
  justification: string
  compensating: string
  conditions: string
  risk_id: string | null
  risk_key?: string
  requested_by: string
  approvers: string[]
  status: string
  expires_on: string
  decided_at: string | null
  decided_by: string
  decision_comment: string
  created_by: string
  created_at: string
  updated_at: string
  approvals: WaiverApproval[]
  /** approved and not expired today */
  active: boolean
  content_hash: string
}

/** Tag severity of a waiver state. */
export function waiverStatusSeverity(status: string): Severity {
  switch (status) {
    case 'approved':
      return 'success'
    case 'requested':
      return 'warn'
    case 'rejected':
    case 'revoked':
      return 'danger'
    case 'expired':
      return 'contrast'
  }
  return 'secondary'
}

/** Icon of a waiver state (the state is never shown by colour alone). */
export function waiverStatusIcon(status: string): string {
  switch (status) {
    case 'approved':
      return 'pi pi-check-circle'
    case 'requested':
      return 'pi pi-clock'
    case 'rejected':
      return 'pi pi-times-circle'
    case 'revoked':
      return 'pi pi-ban'
    case 'expired':
      return 'pi pi-hourglass'
  }
  return 'pi pi-pencil'
}

export const waiverEditable = (w: Pick<Waiver, 'status'>) => w.status === 'draft' || w.status === 'requested'

export interface WaiverFilter {
  text?: string
  statuses?: string[]
  types?: string[]
  /** a release id, "project" (project-wide only) or "" (any) */
  release?: string
  activeOnly?: boolean
}

/** The waivers matching a filter. */
export function filterWaivers<W extends Waiver>(list: readonly W[], f: WaiverFilter): W[] {
  const q = (f.text ?? '').trim().toLowerCase()
  return list.filter((w) => {
    if (f.statuses?.length && !f.statuses.includes(w.status)) return false
    if (f.types?.length && !f.types.includes(w.type)) return false
    if (f.release === 'project' && w.release_id) return false
    if (f.release && f.release !== 'project' && w.release_id !== f.release) return false
    if (f.activeOnly && !w.active) return false
    if (!q) return true
    const hay = [w.key, w.title, w.justification, w.compensating, w.conditions, w.requested_by, ...w.requirements.map((r) => `${r.key} ${r.title}`)].join(' ').toLowerCase()
    return hay.includes(q)
  })
}

/** Days from today (YYYY-MM-DD) to the expiry date (negative: past; null: none). */
export function daysToExpiry(expiresOn: string, today: string): number | null {
  if (!expiresOn) return null
  const a = Date.parse(`${today}T00:00:00Z`)
  const b = Date.parse(`${expiresOn}T00:00:00Z`)
  if (Number.isNaN(a) || Number.isNaN(b)) return null
  return Math.round((b - a) / 86_400_000)
}

/** Whether an approved waiver expires within days (default 30). */
export function expiresSoon(w: Pick<Waiver, 'status' | 'expires_on'>, today: string, days = 30): boolean {
  const d = daysToExpiry(w.expires_on, today)
  return w.status === 'approved' && d !== null && d >= 0 && d <= days
}

export type DecisionBlock = 'not_requested' | 'own' | 'not_approver' | 'already' | null

/**
 * Why the user cannot sign a decision on the waiver now (null: they can).
 * Mirrors the server: requested only, not the requester, a designated
 * approver when there are any, one valid signature per content.
 */
export function decisionBlock(w: Waiver, email: string | undefined): DecisionBlock {
  if (w.status !== 'requested') return 'not_requested'
  const me = (email ?? '').toLowerCase()
  if (!me || w.requested_by.toLowerCase() === me) return 'own'
  if (w.approvers.length && !w.approvers.includes(me)) return 'not_approver'
  if (w.approvals.some((a) => a.valid && a.user_email.toLowerCase() === me)) return 'already'
  return null
}

/** Approval progress: designated approvers who approved the current content. */
export function approvalProgress(w: Waiver): { approved: number; needed: number } {
  const valid = w.approvals.filter((a) => a.valid && a.decision === 'approve').map((a) => a.user_email.toLowerCase())
  if (!w.approvers.length) return { approved: Math.min(valid.length, 1), needed: 1 }
  return { approved: w.approvers.filter((e) => valid.includes(e)).length, needed: w.approvers.length }
}

/** Approver e-mail addresses from free text (comma, semicolon, space or line separated). */
export function parseApprovers(text: string): string[] {
  const out: string[] = []
  for (const part of text.split(/[\s,;]+/)) {
    const e = part.trim().toLowerCase()
    if (e && !out.includes(e)) out.push(e)
  }
  return out
}
