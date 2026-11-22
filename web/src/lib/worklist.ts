// The personal work list ("My work", GET /api/v1/my-work): row types, where
// a row leads, and how reasons are shown. Sources are registered on the
// server (internal/api/worklist.go); a source that adds a new kind of item
// adds its route to workItemRoute and its reason labels to
// workList.reasons in the locales.
import type { LocationQuery, RouteLocationRaw } from 'vue-router'

export interface WorkItem {
  kind: string
  id: string
  entity_type: string
  entity_id: string
  run_id?: string
  key: string
  title: string
  status: string
  project_id: string
  project_key: string
  project_name: string
  reasons: string[]
  detail?: string
  due?: string
  overdue: boolean
  updated_at: string
  sources: string[]
  /** pending review requests of the item waiting for the caller's decision */
  review_ids?: string[]
  /** the caller's own review requests of the item, pending or returned */
  requested_review_ids?: string[]
}

export interface WorkSummary {
  /** rows that need the caller (the badge) */
  count: number
  /** rows that only wait for others (awaiting_review, in_review) */
  waiting: number
  overdue: number
  by_reason: Record<string, number>
  by_kind: Record<string, number>
  by_project: Record<string, number>
}

export const emptySummary = (): WorkSummary => ({ count: 0, waiting: 0, overdue: 0, by_reason: {}, by_kind: {}, by_project: {} })

/** The reasons in the order they are offered as filters. */
export const WORK_REASONS = ['overdue', 'review_requested', 'review_returned', 'waiver_approval', 'risk_acceptance', 'assigned', 'owner', 'mentioned', 'risk_review', 'suspect', 'stale', 'blocked', 'in_review', 'awaiting_review'] as const

/** Reasons of rows that wait for someone else: listed, not counted. */
export const WAITING_REASONS: readonly string[] = ['awaiting_review', 'in_review']

/** Whether a row only waits for others (every reason is a waiting one). */
export function isWaiting(it: Pick<WorkItem, 'reasons'>): boolean {
  return it.reasons.length > 0 && it.reasons.every((r) => WAITING_REASONS.includes(r))
}

/** The sides of the Reviews view of My work. */
export type ReviewSide = 'mine' | 'requested' | 'in_review'
export const REVIEW_SIDES: readonly ReviewSide[] = ['mine', 'requested', 'in_review']

/** The state of My work kept in the address: the view (all work or the
 *  reviews), and in the reviews view the side and pending/all. */
export interface WorkViewState {
  view: 'all' | 'reviews'
  side: ReviewSide
  status: 'pending' | 'all'
}

const q1 = (q: LocationQuery, k: string) => (typeof q[k] === 'string' ? (q[k] as string) : '')

/** Reads the view state from the query (?view=reviews&side=&status=). */
export function workViewState(q: LocationQuery): WorkViewState {
  const side = q1(q, 'side') as ReviewSide
  return {
    view: q1(q, 'view') === 'reviews' ? 'reviews' : 'all',
    side: REVIEW_SIDES.includes(side) ? side : 'mine',
    status: q1(q, 'status') === 'all' ? 'all' : 'pending',
  }
}

/** The query of a view state (defaults left out). */
export function workViewQuery(s: WorkViewState): Record<string, string | undefined> {
  if (s.view !== 'reviews') return { view: undefined, side: undefined, status: undefined }
  return { view: 'reviews', side: s.side === 'mine' ? undefined : s.side, status: s.status === 'pending' ? undefined : s.status }
}

/** My work's reviews view, opening a review request when id is given (the
 *  target of notifications, discussions and the old /reviews address). */
export function reviewsRoute(id?: string, side?: ReviewSide): RouteLocationRaw {
  const query: Record<string, string> = { view: 'reviews' }
  if (side && side !== 'mine') query.side = side
  if (id) query.review = id
  return { name: 'myWork', query }
}

/** The review request a row opens: the first one waiting for the caller. */
export function rowReviewId(it: Pick<WorkItem, 'review_ids'>): string {
  return it.review_ids?.[0] ?? ''
}

/** Where a row of the work list leads. */
export function workItemRoute(it: Pick<WorkItem, 'project_id' | 'entity_type' | 'entity_id' | 'run_id'>): RouteLocationRaw {
  const params = { projectId: it.project_id }
  switch (it.entity_type) {
    case 'requirement':
      return { name: 'requirements', params, query: { item: it.entity_id } }
    case 'test':
      return { name: 'tests', params, query: { item: it.entity_id } }
    case 'defect':
      return { name: 'defects', params, query: { item: it.entity_id } }
    case 'risk':
      return { name: 'risks', params, query: { risk: it.entity_id } }
    case 'waiver':
      return { name: 'waivers', params, query: { waiver: it.entity_id } }
    case 'run':
      return { name: 'run', params: { ...params, runId: it.entity_id }, query: { tab: 'discussion' } }
    case 'review':
      return reviewsRoute(it.entity_id)
    case 'cycle':
      return { name: 'cycle', params: { ...params, cycleId: it.entity_id } }
    case 'exploratory_session':
      return { name: 'exploratorySession', params: { ...params, sessionId: it.entity_id } }
    case 'execution':
      return it.run_id ? { name: 'execute', params: { ...params, runId: it.run_id, executionId: it.entity_id } } : { name: 'runs', params }
  }
  return { name: 'dashboard', params }
}

/** Tag severity of a reason. */
export function reasonSeverity(reason: string): 'danger' | 'warn' | 'info' | 'secondary' {
  switch (reason) {
    case 'overdue':
    case 'blocked':
      return 'danger'
    case 'review_requested':
    case 'review_returned':
    case 'waiver_approval':
    case 'risk_acceptance':
    case 'suspect':
    case 'stale':
      return 'warn'
    case 'assigned':
    case 'risk_review':
    case 'in_review':
      return 'info'
  }
  return 'secondary'
}

/** Icon of a kind of item. */
export function workKindIcon(kind: string): string {
  switch (kind) {
    case 'requirement':
      return 'pi pi-list-check'
    case 'test':
      return 'pi pi-check-square'
    case 'defect':
      return 'pi pi-exclamation-circle'
    case 'risk':
      return 'pi pi-shield'
    case 'waiver':
      return 'pi pi-pen-to-square'
    case 'execution':
      return 'pi pi-play-circle'
  }
  return 'pi pi-inbox'
}

/** Rows with a reason ("" all). */
export function filterByReason(list: readonly WorkItem[], reason: string): WorkItem[] {
  return reason ? list.filter((it) => it.reasons.includes(reason)) : [...list]
}

/** The reasons present in a summary, in WORK_REASONS order, then any others. */
export function presentReasons(s: WorkSummary): string[] {
  const known: string[] = WORK_REASONS.filter((r) => (s.by_reason[r] ?? 0) > 0)
  const other = Object.keys(s.by_reason)
    .filter((r) => !known.includes(r) && s.by_reason[r] > 0)
    .sort()
  return [...known, ...other]
}
