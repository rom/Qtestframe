// Verification correlations: criterion-level verification, verification
// records, the roll-up along the requirement hierarchy (refines) and system
// elements. Pure helpers for the views (see internal/verification).
import type { Severity } from './colors'
import type { ElementCoverage, Evidence, SystemElement, VerificationReason, VerificationState } from './types'

/** Verification states in display order (most to least verified). */
export const VERIFICATION_STATES: VerificationState[] = ['verified', 'partial', 'failed', 'pending', 'none']
/** Results of a verification record. */
export const RECORD_RESULTS = ['pass', 'fail', 'conditional'] as const
/** Kinds of system element (internal/model ElementKinds). */
export const ELEMENT_KINDS = ['system', 'subsystem', 'component', 'product', 'supplier', 'other'] as const
/** Codes of the reasons a requirement is not verified (and of inconsistencies). */
export const REASON_CODES = [
  'no_evidence',
  'criterion_missing',
  'criterion_failed',
  'criterion_pending',
  'evidence_failed',
  'evidence_pending',
  'child_not_verified',
  'status_inconsistent',
  'child_status_unverified',
] as const
/** Notes on evidence that does not count or has not passed. */
export const EVIDENCE_NOTES = ['suspect', 'not_run', 'step_not_run', 'expired', 'invalidated', 'superseded', 'conditional'] as const

const SEVERITY: Record<string, Severity> = { verified: 'success', partial: 'warn', failed: 'danger', pending: 'secondary', none: 'contrast' }
export function verificationSeverity(v: string | null | undefined): Severity {
  return SEVERITY[v ?? ''] ?? 'secondary'
}

/** Icon of a state (shown with its text, never instead of it). */
export function verificationIcon(v: string | null | undefined): string {
  switch (v) {
    case 'verified':
      return 'pi pi-verified'
    case 'partial':
      return 'pi pi-circle-fill'
    case 'failed':
      return 'pi pi-times-circle'
    case 'pending':
      return 'pi pi-clock'
  }
  return 'pi pi-circle'
}

/** Severity of an evidence state (passed / failed / pending / void). */
export function evidenceSeverity(state: string): Severity {
  return state === 'passed' ? 'success' : state === 'failed' ? 'danger' : state === 'void' ? 'contrast' : 'secondary'
}

type Translate = (key: string, params?: Record<string, unknown>) => string

/** The translated text of a reason ("criterion 2 has no evidence"). */
export function reasonText(t: Translate, te: (k: string) => boolean, r: VerificationReason): string {
  const key = `verification.reason.${r.code}`
  if (!te(key)) return r.text
  const p: Record<string, string> = { ...(r.params ?? {}) }
  if (p.kind) p.kind = t(`verification.evidenceKind.${p.kind}`)
  if (p.state) p.state = t(`enums.verification.${p.state}`)
  if (p.status) p.status = te(`enums.status.${p.status}`) ? t(`enums.status.${p.status}`) : p.status
  return t(key, p)
}

/** A short label of a piece of evidence: "DEMO-TC-1 · step 2" or "Inspection · 2026-01-10". */
export function evidenceLabel(t: Translate, e: Evidence): string {
  if (e.kind === 'record') return [t(`enums.evaluationMethod.${e.key}`), e.date].filter(Boolean).join(' · ')
  return e.step_position ? `${e.key} · ${t('verification.stepN', { n: e.step_position })}` : e.key
}

/** A node of the requirement hierarchy (refines) with its children. */
export interface HierarchyNode<T> {
  item: T
  depth: number
  children: HierarchyNode<T>[]
}

/**
 * Builds the requirement hierarchy from parent ids (a child refines its
 * parents). A requirement with several parents appears under each; roots
 * are requirements without a parent among the items. Cycles are cut.
 */
export function buildHierarchy<T extends { requirement_id: string; parent_ids: string[] }>(items: T[]): HierarchyNode<T>[] {
  const byId = new Map(items.map((i) => [i.requirement_id, i]))
  const kids = new Map<string, T[]>()
  for (const i of items)
    for (const p of i.parent_ids ?? [])
      if (byId.has(p)) {
        if (!kids.has(p)) kids.set(p, [])
        kids.get(p)!.push(i)
      }
  const make = (i: T, depth: number, path: Set<string>): HierarchyNode<T> => {
    const next = new Set(path).add(i.requirement_id)
    const children = (kids.get(i.requirement_id) ?? []).filter((c) => !next.has(c.requirement_id)).map((c) => make(c, depth + 1, next))
    return { item: i, depth, children }
  }
  return items.filter((i) => !(i.parent_ids ?? []).some((p) => byId.has(p))).map((i) => make(i, 0, new Set()))
}

/** Depth-first list of a hierarchy, leaving out the children of collapsed nodes. */
export function flattenHierarchy<T>(nodes: HierarchyNode<T>[], collapsed: Set<string>, id: (t: T) => string): HierarchyNode<T>[] {
  const out: HierarchyNode<T>[] = []
  const walk = (ns: HierarchyNode<T>[]) => {
    for (const n of ns) {
      out.push(n)
      if (!collapsed.has(id(n.item))) walk(n.children)
    }
  }
  walk(nodes)
  return out
}

/** Only the branches that contain a node matching keep (with their ancestors). */
export function filterHierarchy<T>(nodes: HierarchyNode<T>[], keep: (t: T) => boolean): HierarchyNode<T>[] {
  const out: HierarchyNode<T>[] = []
  for (const n of nodes) {
    const children = filterHierarchy(n.children, keep)
    if (keep(n.item) || children.length) out.push({ ...n, children })
  }
  return out
}

/** "42/50 verified" share of an element (0–100, rounded). */
export function elementPercent(c: Pick<ElementCoverage, 'verified' | 'requirements'>): number {
  return c.requirements ? Math.round((c.verified * 100) / c.requirements) : 0
}

/** Element options for a select: the path, indented by depth. */
export function elementOptions(list: SystemElement[]): { label: string; value: string }[] {
  return list.map((e) => ({ label: e.path || e.name, value: e.id }))
}

/** Ids of an element and every element below it. */
export function subtreeIds(list: { id: string; parent_id: string | null }[], id: string): Set<string> {
  const out = new Set([id])
  let grew = true
  while (grew) {
    grew = false
    for (const e of list)
      if (e.parent_id && out.has(e.parent_id) && !out.has(e.id)) {
        out.add(e.id)
        grew = true
      }
  }
  return out
}

/** Today as YYYY-MM-DD (local time). */
export function today(d = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

/** Problems of a record form (keys of verification.problem). */
export function recordProblems(f: { method: string; result: string; performed_on: string; valid_until: string }): string[] {
  const out: string[] = []
  const date = /^\d{4}-\d{2}-\d{2}$/
  if (!f.method) out.push('method')
  if (!f.result) out.push('result')
  if (f.performed_on && !date.test(f.performed_on)) out.push('date')
  if (f.valid_until && !date.test(f.valid_until)) out.push('date')
  if (f.valid_until && f.performed_on && f.valid_until < f.performed_on) out.push('validUntil')
  return [...new Set(out)]
}
