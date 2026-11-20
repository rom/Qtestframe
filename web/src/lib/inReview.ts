// Requirements and tests in status "review" that concern the signed-in
// user (GET /reviews/in-review), shown under My reviews.

export interface InReviewItem {
  entity_type: 'requirement' | 'test' | string
  entity_id: string
  key: string
  title: string
  project_id: string
  project_key: string
  project_name: string
  owner: string
  created_by: string
  version: number
  updated_at: string
  /** owner | author | requested | reviewer */
  reasons: string[]
  request_id?: string
}

/** Reasons that make an item personally yours (not just reviewable by role). */
const PERSONAL = new Set(['owner', 'author', 'requested'])

export const isPersonal = (it: InReviewItem): boolean => it.reasons.some((r) => PERSONAL.has(r))

/** All items, or only those you own, wrote or were asked to review. */
export function filterInReview(items: InReviewItem[], onlyMine: boolean): InReviewItem[] {
  return onlyMine ? items.filter(isPersonal) : items
}

/** Whole days an item has waited since its last change. */
export function daysWaiting(updatedAt: string, now: Date = new Date()): number {
  const t = Date.parse(updatedAt)
  return Number.isNaN(t) ? 0 : Math.max(0, Math.floor((now.getTime() - t) / 86_400_000))
}
