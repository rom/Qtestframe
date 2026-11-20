// Bulk review (approve/reject requirements, mark tests ready): options and
// the result of POST /projects/{pid}/{requirements|tests}/review.

export type ReviewKind = 'requirement' | 'test'

export interface BulkReviewItem {
  id: string
  key: string
  title: string
  status: string
  reason?: string
}

export interface BulkReviewResult {
  decision: string
  dry_run: boolean
  updated: BulkReviewItem[]
  skipped: BulkReviewItem[]
  /** the caller's open review requests for the chosen items */
  my_reviews: number
  /** how many of them are (or would be) completed */
  reviews_completed: number
}

/** The review request decision a bulk decision completes them with. */
export function reviewOutcome(decision: string): 'approved' | 'changes_requested' {
  return decision === 'rejected' ? 'changes_requested' : 'approved'
}

/** Decisions per kind, the first being the default. */
export function decisionsFor(kind: ReviewKind): string[] {
  return kind === 'requirement' ? ['approved', 'rejected'] : ['ready']
}

/** Checks that can be ignored, per kind. */
export function ignorableChecks(kind: ReviewKind): string[] {
  const common = ['changes_requested', 'pending_review', 'suspect']
  return kind === 'requirement' ? [...common, 'no_obligation'] : [...common, 'no_steps', 'no_script']
}

/** Statuses an item may come from, per kind (the target is left out). */
export function fromOptions(kind: ReviewKind, decision: string): string[] {
  const all = kind === 'requirement' ? ['draft', 'review', 'approved', 'rejected', 'implemented', 'verified', 'obsolete'] : ['draft', 'review', 'ready', 'deprecated']
  return all.filter((s) => s !== decision)
}

/** Default statuses to review from. */
export const DEFAULT_FROM = ['review']

/** Skipped items grouped by reason, reasons ordered by count (largest first). */
export function groupSkipped(items: BulkReviewItem[]): { reason: string; items: BulkReviewItem[] }[] {
  const map = new Map<string, BulkReviewItem[]>()
  for (const it of items) {
    const r = it.reason ?? 'other'
    if (!map.has(r)) map.set(r, [])
    map.get(r)!.push(it)
  }
  return [...map.entries()].map(([reason, list]) => ({ reason, items: list })).sort((a, b) => b.items.length - a.items.length || a.reason.localeCompare(b.reason))
}

/** The updated items grouped by their previous status, to undo one bulk call per status. */
export function undoGroups(updated: BulkReviewItem[]): { status: string; ids: string[] }[] {
  const map = new Map<string, string[]>()
  for (const it of updated) {
    if (!map.has(it.status)) map.set(it.status, [])
    map.get(it.status)!.push(it.id)
  }
  return [...map.entries()].map(([status, ids]) => ({ status, ids }))
}

export type BulkScope = 'selected' | 'shown' | 'all'

/** The request body of a scope. */
export function scopeBody(scope: BulkScope, selected: string[], shown: string[]): { ids: string[] } | { all: true } {
  if (scope === 'all') return { all: true }
  return { ids: scope === 'selected' ? selected : shown }
}
