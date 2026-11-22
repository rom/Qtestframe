// The personal work list ("My work", GET /api/v1/my-work): row types, where
// a row leads, and how reasons are shown. Sources are registered on the
// server (internal/api/worklist.go); a source that adds a new kind of item
// adds its route to workItemRoute and its reason labels to
// workList.reasons in the locales.
import type { RouteLocationRaw } from 'vue-router'

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
}

export interface WorkSummary {
  count: number
  overdue: number
  by_reason: Record<string, number>
  by_kind: Record<string, number>
  by_project: Record<string, number>
}

export const emptySummary = (): WorkSummary => ({ count: 0, overdue: 0, by_reason: {}, by_kind: {}, by_project: {} })

/** The reasons in the order they are offered as filters. */
export const WORK_REASONS = ['overdue', 'review_requested', 'waiver_approval', 'risk_acceptance', 'assigned', 'owner', 'mentioned', 'risk_review', 'suspect', 'stale', 'blocked'] as const

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
      return { name: 'reviews', query: { review: it.entity_id } }
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
    case 'waiver_approval':
    case 'risk_acceptance':
    case 'suspect':
    case 'stale':
      return 'warn'
    case 'assigned':
    case 'risk_review':
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
