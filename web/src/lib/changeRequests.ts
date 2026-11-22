// Change requests (mirrors internal/model ChangeRequest): states, the
// proposed changes as field diffs, who may do what, and the change
// control answer of a blocked save.
import type { Severity } from './colors'
import { ApiError } from './api'
import { diffWords, valueToText, type DiffSegment } from './diff'
import type { ChangeImpact } from './correlations'

export const CR_STATUSES = ['draft', 'submitted', 'assessed', 'in_review', 'approved', 'rejected', 'deferred', 'implemented', 'verified', 'closed'] as const
export const CR_OPEN_STATUSES: readonly string[] = ['draft', 'submitted', 'assessed', 'in_review', 'approved', 'deferred']
export const CR_CATEGORIES = ['correction', 'enhancement', 'regulatory', 'defect_driven', 'risk_driven', 'other'] as const
export const CR_DECISIONS = ['approve', 'reject', 'defer'] as const
export const CHANGE_CONTROL_MODES = ['off', 'warn', 'enforce'] as const

/** The fields a change request can propose per kind (model.CRProposalFields). */
export const CR_FIELDS: Record<'requirement' | 'test', readonly string[]> = {
  requirement: ['title', 'description', 'obligation', 'type', 'priority', 'status', 'source', 'rationale', 'owner', 'tags', 'custom_fields'],
  test: ['title', 'description', 'preconditions', 'expected_result', 'execution_type', 'priority', 'status', 'estimated_minutes', 'owner', 'tags', 'custom_fields'],
}

export interface ChangeRequestItem {
  id: string
  entity_type: 'requirement' | 'test'
  entity_id: string
  key: string
  title: string
  status: string
  base_version: number
  current_version: number
  proposed: Record<string, unknown>
  current?: Record<string, unknown>
  outdated: boolean
  missing?: boolean
  applied_version: number
}

export interface ChangeRequestSignoff {
  id: string
  user_id: string
  user_email: string
  user_name: string
  decision: string
  meaning: string
  comment: string
  content_hash: string
  signed_at: string
  valid: boolean
}

export interface ChangeRequestEvent {
  id: string
  action: string
  actor: string
  comment: string
  at: string
}

export interface ChangeRequest {
  id: string
  project_id: string
  key: string
  title: string
  reason: string
  category: string
  priority: string
  status: string
  impact_action: string
  impact: ChangeImpact | null
  impact_notes: string
  effort: string
  assessed_by: string
  assessed_at: string | null
  links: { defect_ids: string[]; risk_ids: string[]; issues: string[] }
  defects: { id: string; key: string; title: string }[]
  risks: { id: string; key: string; title: string }[]
  baseline_on_close: boolean
  baseline_id: string
  decided_by: string
  decided_at: string | null
  decision_comment: string
  applied_by: string
  applied_at: string | null
  verified_by: string
  verified_at: string | null
  closed_by: string
  closed_at: string | null
  created_by: string
  created_at: string
  updated_at: string
  items: ChangeRequestItem[]
  approvals: ChangeRequestSignoff[]
  events: ChangeRequestEvent[]
  content_hash: string
  outdated: boolean
}

export interface ChangeBoard {
  users: string[]
  roles: string[]
  quorum: string
}

export interface ChangeControlSettings {
  mode: string
  protected_statuses: string[]
  lock_signed_baselines: boolean
  board: ChangeBoard
  signature_meaning: string
  baseline_on_close: boolean
  updated_by?: string
  updated_at?: string | null
}

/** The answer of GET /requirements/{id}/change-control. */
export interface ItemChangeControl {
  key: string
  mode: string
  protected: boolean
  blocked: boolean
  reasons: string[]
  open_change_requests: { id: string; key: string; title: string; status: string }[]
  can_propose: boolean
  can_override: boolean
}

export const crOpen = (status: string) => CR_OPEN_STATUSES.includes(status)

/** Tag severity of a change request state. */
export function crStatusSeverity(status: string): Severity {
  switch (status) {
    case 'approved':
    case 'implemented':
    case 'verified':
      return 'success'
    case 'submitted':
    case 'assessed':
    case 'in_review':
      return 'warn'
    case 'rejected':
      return 'danger'
    case 'deferred':
    case 'closed':
      return 'contrast'
  }
  return 'secondary'
}

/** Icon of a change request state (never colour alone). */
export function crStatusIcon(status: string): string {
  switch (status) {
    case 'submitted':
      return 'pi pi-send'
    case 'assessed':
      return 'pi pi-sitemap'
    case 'in_review':
      return 'pi pi-clock'
    case 'approved':
      return 'pi pi-check-circle'
    case 'rejected':
      return 'pi pi-times-circle'
    case 'deferred':
      return 'pi pi-pause'
    case 'implemented':
      return 'pi pi-cog'
    case 'verified':
      return 'pi pi-verified'
    case 'closed':
      return 'pi pi-lock'
  }
  return 'pi pi-pencil'
}

/** The steps of the workflow, for the progress bar of the detail view. */
export const CR_STEPS = ['draft', 'submitted', 'assessed', 'in_review', 'approved', 'implemented', 'verified', 'closed'] as const

/** How far along the workflow a state is (index into CR_STEPS; rejected and deferred stop at in_review). */
export function crStepIndex(status: string): number {
  if (status === 'rejected' || status === 'deferred') return CR_STEPS.indexOf('in_review')
  return Math.max(0, CR_STEPS.indexOf(status as (typeof CR_STEPS)[number]))
}

export interface FieldDiff {
  field: string
  before: string
  after: string
  /** word diff for text fields (null: show before → after) */
  segments: DiffSegment[] | null
}

const TEXT_FIELDS = new Set(['title', 'description', 'rationale', 'source', 'preconditions', 'expected_result'])

/** The proposed changes of an item, field by field (in CR_FIELDS order). */
export function itemDiffs(it: Pick<ChangeRequestItem, 'entity_type' | 'proposed' | 'current'>): FieldDiff[] {
  const order = CR_FIELDS[it.entity_type] ?? []
  const fields = Object.keys(it.proposed).sort((a, b) => order.indexOf(a) - order.indexOf(b))
  return fields.map((field) => {
    const before = valueToText(field, it.current?.[field])
    const after = valueToText(field, it.proposed[field])
    return { field, before, after, segments: TEXT_FIELDS.has(field) ? diffWords(before, after) : null }
  })
}

/**
 * The proposal of an edit: the proposable fields whose value differs
 * between the loaded item and the edited form (JSON comparison).
 */
export function proposalOf(kind: 'requirement' | 'test', loaded: Record<string, unknown>, edited: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const f of CR_FIELDS[kind]) {
    if (!(f in edited)) continue
    const a = loaded[f] ?? null
    const b = edited[f] ?? null
    const empty = (v: unknown) => v === null || v === '' || (Array.isArray(v) && !v.length) || (typeof v === 'object' && v !== null && !Array.isArray(v) && !Object.keys(v).length)
    if (empty(a) && empty(b)) continue
    if (JSON.stringify(a) !== JSON.stringify(b)) out[f] = b
  }
  return out
}

/** The same proposed values for several items (the bulk "Propose change"). */
export function bulkProposal(set: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(set)) if (v !== undefined && v !== null && v !== '') out[k] = v
  return out
}

/** Whether a board seat is held: listed, holding a listed role, or nobody listed. */
export function onBoard(b: ChangeBoard, email: string | undefined, roles: readonly string[]): boolean {
  if (!b.users.length && !b.roles.length) return true
  const me = (email ?? '').toLowerCase()
  return b.users.some((u) => u.toLowerCase() === me) || b.roles.some((r) => roles.includes(r))
}

export type CRDecisionBlock = 'not_in_review' | 'own' | 'not_on_board' | 'already' | 'outdated' | null

/** Why the user cannot sign a decision now (null: they can). Mirrors the server. */
export function crDecisionBlock(cr: ChangeRequest, email: string | undefined, board: boolean): CRDecisionBlock {
  if (cr.status !== 'in_review') return 'not_in_review'
  const me = (email ?? '').toLowerCase()
  if (!me || cr.created_by.toLowerCase() === me) return 'own'
  if (!board) return 'not_on_board'
  if (cr.outdated) return 'outdated'
  if (cr.approvals.some((a) => a.valid && a.user_email.toLowerCase() === me)) return 'already'
  return null
}

/** Approvals so far and needed under the quorum (mirrors BoardProgress). */
export function boardProgress(b: ChangeBoard, cr: Pick<ChangeRequest, 'approvals'>): { approved: number; needed: number } {
  const approved = new Set(cr.approvals.filter((a) => a.valid && a.decision === 'approve').map((a) => a.user_email.toLowerCase())).size
  let needed = 1
  if (b.quorum === 'all' && (b.users.length || b.roles.length)) needed = b.users.length + b.roles.length
  else if (/^\d+$/.test(b.quorum)) needed = Number(b.quorum)
  return { approved, needed }
}

/** The actions a change request offers now (the server checks again). */
export function crActions(cr: Pick<ChangeRequest, 'status' | 'items'>, can: { create: boolean }): string[] {
  if (!can.create) return []
  switch (cr.status) {
    case 'draft':
      return cr.items.length ? ['edit', 'submit', 'delete'] : ['edit', 'delete']
    case 'submitted':
      return ['assess', 'withdraw']
    case 'assessed':
      return ['assess', 'review', 'withdraw']
    case 'in_review':
      return ['withdraw']
    case 'approved':
      return ['apply']
    case 'rejected':
      return ['withdraw', 'close', 'delete']
    case 'deferred':
      return ['withdraw', 'close']
    case 'implemented':
      return ['verify']
    case 'verified':
      return ['close']
  }
  return []
}

export interface CRFilter {
  text?: string
  statuses?: string[]
  categories?: string[]
  openOnly?: boolean
}

/** The change requests matching a filter. */
export function filterCRs<C extends ChangeRequest>(list: readonly C[], f: CRFilter): C[] {
  const q = (f.text ?? '').trim().toLowerCase()
  return list.filter((c) => {
    if (f.statuses?.length && !f.statuses.includes(c.status)) return false
    if (f.categories?.length && !f.categories.includes(c.category)) return false
    if (f.openOnly && !crOpen(c.status)) return false
    if (!q) return true
    const hay = [c.key, c.title, c.reason, c.created_by, ...c.items.map((i) => `${i.key} ${i.title}`)].join(' ').toLowerCase()
    return hay.includes(q)
  })
}

export interface ChangeBlocked {
  key: string
  reasons: string[]
  open: string[]
}

/** The change control answer of a blocked save (409 change_control), else null. */
export function changeBlocked(e: unknown): ChangeBlocked | null {
  if (!(e instanceof ApiError) || e.status !== 409 || e.code !== 'change_control') return null
  const b = (e.body ?? {}) as { key?: string; reasons?: string[]; open_change_requests?: string[] }
  return { key: b.key ?? '', reasons: b.reasons ?? [], open: b.open_change_requests ?? [] }
}

/** Parse "a@x, b@y; c@z" into normalised addresses. */
export function parseEmails(text: string): string[] {
  const out: string[] = []
  for (const part of text.split(/[\s,;]+/)) {
    const e = part.trim().toLowerCase()
    if (e && !out.includes(e)) out.push(e)
  }
  return out
}
