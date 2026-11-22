// Built-in consistency checks (internal/tracerules builtin.go): kinds,
// texts of their violations, routes of the items involved and the fix
// actions, performed with the ordinary endpoints.
import type { RouteLocationRaw } from 'vue-router'
import { api } from './api'
import { P } from './permissions'
import type { TraceRelated, TraceViolation } from './types'

/** Built-in checks in display order (internal/model TraceBuiltinKinds). */
export const BUILTIN_KINDS = [
  'conflict_in_release',
  'unmet_dependency',
  'obsolete_parent',
  'verifies_inactive',
  'broken_prerequisite',
  'unevidenced_must',
  'unmitigated_risk',
  'unjustified_rejection',
  'duplicate_suspect',
  'potential_conflict',
  'test_duplicate_suspect',
] as const

/** Built-in checks with a threshold option (similarity or confidence) and its default. */
export const THRESHOLD_DEFAULTS: Record<string, number> = { duplicate_suspect: 0.68, test_duplicate_suspect: 0.68, potential_conflict: 0.6 }

/** Reason codes of violations (Violation.check). */
export const REASONS = ['conflict', 'rejected', 'obsolete', 'deleted', 'failing', 'cycle', 'no_evidence', 'no_mitigation', 'not_verified', 'unjustified', 'similar', 'opposite_obligation', 'threshold', 'exclusive'] as const

/** Roles of related items. */
export const ROLES = ['conflicts_with', 'depends_on', 'refines', 'derived_from', 'verifies', 'prerequisite', 'release', 'mitigation', 'duplicate', 'conflict'] as const

/** Fix actions (tracerules FixActions). */
export const FIX_ACTIONS = [
  'remove_relation',
  'remove_link',
  'remove_dependency',
  'remove_from_release',
  'set_obsolete',
  'add_justification',
  'link_mitigation',
  'mark_duplicate',
  'mark_conflict',
  'dismiss',
] as const

export type FixAction = (typeof FIX_ACTIONS)[number]

/** Label of the item link that marks a test as a duplicate (model.DuplicateLinkLabel). */
export const DUPLICATE_LINK_LABEL = 'duplicates'

/** Checks whose findings about a pair can be dismissed. */
export const DISMISSABLE = ['duplicate_suspect', 'unmet_dependency', 'broken_prerequisite', 'potential_conflict', 'test_duplicate_suspect']

export function isBuiltin(kind: string | null | undefined): boolean {
  return (BUILTIN_KINDS as readonly string[]).includes(kind ?? '')
}

/** The item kind a built-in check applies to. */
export function builtinScope(kind: string): 'requirement' | 'test' | 'risk' {
  if (kind === 'verifies_inactive' || kind === 'broken_prerequisite' || kind === 'test_duplicate_suspect') return 'test'
  if (kind === 'unmitigated_risk') return 'risk'
  return 'requirement'
}

/** Route of an item of a violation (requirement, test, risk, release, defect). */
export function itemRoute(it: { kind: string; id: string }, projectId: string): RouteLocationRaw | undefined {
  const params = { projectId }
  switch (it.kind) {
    case 'requirement':
      return { name: 'requirements', params, query: { item: it.id } }
    case 'test':
      return { name: 'tests', params, query: { item: it.id } }
    case 'risk':
      return { name: 'risks', params, query: { risk: it.id } }
    case 'release':
      return { name: 'releases', params, query: { r: it.id } }
    case 'defect':
      return { name: 'defects', params, query: { item: it.id } }
  }
  return undefined
}

/** A related item that was deleted (in the trash) has no page. */
export function isDeleted(r: Pick<TraceRelated, 'status'>): boolean {
  return r.status === 'deleted'
}

const RELATION_ROLES = ['conflicts_with', 'depends_on', 'refines', 'derived_from']

/** The fix actions offered for one related item of a violation of check. */
export function relatedFixes(check: string, r: TraceRelated): FixAction[] {
  const out: FixAction[] = []
  if (r.link && RELATION_ROLES.includes(r.role)) out.push('remove_relation')
  if (r.link && r.role === 'verifies') out.push('remove_link')
  if (r.link && r.role === 'prerequisite' && r.check !== 'failing') out.push('remove_dependency')
  if (r.kind === 'release' && check === 'conflict_in_release') out.push('remove_from_release')
  if (r.role === 'duplicate') out.push('mark_duplicate', 'dismiss')
  else if (r.role === 'conflict') out.push('mark_conflict', 'dismiss')
  else if (!r.link && r.check === 'deleted' && DISMISSABLE.includes(check)) out.push('dismiss')
  return out
}

/** The fix actions of the violation itself (not tied to one related item). */
export function itemFixes(v: Pick<TraceViolation, 'fixes'>): FixAction[] {
  return (v.fixes ?? []).filter((f): f is FixAction => f === 'set_obsolete' || f === 'add_justification' || f === 'link_mitigation')
}

/** The permission a fix action needs (check: the rule kind of the violation). */
export function fixPermission(f: FixAction, check = ''): string {
  if (builtinScope(check) === 'test' && (f === 'mark_duplicate' || f === 'dismiss')) return P.testWrite
  switch (f) {
    case 'remove_link':
    case 'remove_dependency':
      return P.testWrite
    case 'remove_from_release':
      return P.projectUpdate
    case 'link_mitigation':
    case 'add_justification':
      return P.requirementRead
  }
  return P.requirementWrite
}

/**
 * Performs a fix action that changes data (navigation actions return
 * false without a request). check is the rule kind of the violation.
 */
export async function applyFix(f: FixAction, check: string, v: Pick<TraceViolation, 'id' | 'kind'>, r: TraceRelated | null, projectId: string): Promise<boolean> {
  switch (f) {
    case 'remove_relation':
      if (r?.link) await api.del(`/relations/${r.link}`)
      return true
    case 'remove_link':
      if (r?.link) await api.del(`/links/${r.link}`)
      return true
    case 'remove_dependency':
      if (r?.link) await api.del(`/test-dependencies/${r.link}`)
      return true
    case 'remove_from_release':
      if (r) await api.post(`/releases/${r.id}/requirements`, { remove: [v.id] })
      return true
    case 'set_obsolete':
      await api.patch(`/requirements/${v.id}`, { status: 'obsolete' })
      return true
    case 'mark_duplicate':
      if (r && v.kind === 'test') await api.post(`/tests/${v.id}/item-links`, { target_type: 'test', target_id: r.id, label: DUPLICATE_LINK_LABEL })
      else if (r) await api.post(`/requirements/${v.id}/relations`, { target_id: r.id, type: 'duplicates' })
      return true
    case 'mark_conflict':
      if (r) await api.post(`/requirements/${v.id}/relations`, { target_id: r.id, type: 'conflicts_with' })
      return true
    case 'dismiss':
      if (r) await api.post(`/projects/${projectId}/consistency/dismissals`, { check, item_id: v.id, other_id: r.id })
      return true
  }
  return false
}

/** Route of a navigation fix (add_justification, link_mitigation). */
export function fixRoute(f: FixAction, v: Pick<TraceViolation, 'id' | 'kind'>, projectId: string): RouteLocationRaw | undefined {
  if (f === 'add_justification') return { name: 'requirements', params: { projectId }, query: { item: v.id, tab: 'comments' } }
  if (f === 'link_mitigation') return { name: 'risks', params: { projectId }, query: { risk: v.id } }
  return undefined
}

type T = (key: string, params?: Record<string, unknown>, plural?: number) => string

/** One-line text of a violation of a built-in check, e.g. "Conflicts with REQ-2 in 1.0". */
export function violationLine(v: Pick<TraceViolation, 'check' | 'note' | 'related'>, t: T): string {
  const rel = (v.related ?? []).filter((r) => r.kind !== 'release')
  const releases = (v.related ?? []).filter((r) => r.kind === 'release').map((r) => r.key)
  const keys = rel.map((r) => r.key || r.id.slice(0, 8)).join(', ')
  return t(`consistency.reason.${v.check || 'conflict'}`, { keys, releases: releases.join(', '), note: v.note ?? '', n: rel.length }, Math.max(rel.length, 1))
}

/** Text of one related item: its role and why it matters. */
export function relatedLine(r: TraceRelated, t: T, statusLabel: (s: string) => string = (s) => s): string {
  const role = t(`consistency.role.${r.role}`)
  if (r.check === 'deleted') return `${role} · ${t('consistency.deleted')}`
  if (r.note && r.role === 'duplicate') return `${role} · ${t('consistency.similarity', { pct: r.note })}`
  if (r.role === 'conflict') return `${role} · ${t(`consistency.reason.${r.check || 'exclusive'}`, { keys: r.key, note: r.note ?? '', n: 1 }, 1)}`
  if (r.check === 'failing') return `${role} · ${t('consistency.failing')}`
  if (r.check === 'cycle') return `${role} · ${t('consistency.cycle')}`
  return r.status ? `${role} · ${statusLabel(r.status)}` : role
}
