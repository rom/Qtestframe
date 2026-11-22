// Pure helpers for discussions: comments grouped into threads, open
// counts, reactions and where a thread's item is.
import type { Comment, DiscussionKind, DiscussionThread } from './types'
import type { RouteTarget } from './notifications'

export interface Thread {
  root: Comment
  replies: Comment[]
  /** not resolved and with at least one comment that is not removed */
  open: boolean
  /** every comment is removed */
  removed: boolean
}

/** Comments (any order) → threads, oldest thread first, replies oldest first. */
export function groupThreads(comments: readonly Comment[]): Thread[] {
  const sorted = [...comments].sort((a, b) => a.created_at.localeCompare(b.created_at) || a.id.localeCompare(b.id))
  const byRoot = new Map<string, Thread>()
  const order: string[] = []
  const orphans: Comment[] = []
  for (const c of sorted) {
    if (!c.parent_id) {
      byRoot.set(c.id, { root: c, replies: [], open: false, removed: false })
      order.push(c.id)
    }
  }
  for (const c of sorted) {
    if (!c.parent_id) continue
    const t = byRoot.get(c.parent_id)
    if (t) t.replies.push(c)
    else orphans.push(c)
  }
  // replies whose first comment is gone for good: each its own thread
  for (const c of orphans) {
    byRoot.set(c.id, { root: c, replies: [], open: false, removed: false })
    order.push(c.id)
  }
  return order.map((id) => {
    const t = byRoot.get(id)!
    const live = [t.root, ...t.replies].some((c) => !c.deleted)
    t.removed = !live
    t.open = live && !t.root.resolved_at
    return t
  })
}

/** Number of open threads. */
export function openCount(comments: readonly Comment[]): number {
  return groupThreads(comments).filter((t) => t.open).length
}

/** Comments that are not removed. */
export function liveCount(comments: readonly Comment[]): number {
  return comments.filter((c) => !c.deleted).length
}

/** Whether a comment was edited (more than a second after it was written). */
export function wasEdited(c: Pick<Comment, 'created_at' | 'updated_at' | 'deleted'>): boolean {
  if (c.deleted || !c.updated_at) return false
  return new Date(c.updated_at).getTime() - new Date(c.created_at).getTime() > 1000
}

/** The reactions a comment can get, with how they are shown. */
export const REACTIONS: { key: string; emoji: string }[] = [
  { key: '+1', emoji: '👍' },
  { key: '-1', emoji: '👎' },
  { key: 'heart', emoji: '❤️' },
  { key: 'tada', emoji: '🎉' },
  { key: 'eyes', emoji: '👀' },
  { key: 'check', emoji: '✅' },
]

/** Reactions of a comment in display order: [{key, emoji, count, mine}]. */
export function reactionList(c: Pick<Comment, 'reactions'>, me: string | undefined): { key: string; emoji: string; count: number; mine: boolean }[] {
  const r = c.reactions ?? {}
  return REACTIONS.filter((x) => (r[x.key]?.length ?? 0) > 0).map((x) => ({ ...x, count: r[x.key].length, mine: !!me && r[x.key].includes(me) }))
}

/** The API path of an item's comments. */
export function commentsPath(kind: DiscussionKind, id: string): string {
  const base: Record<DiscussionKind, string> = { requirement: 'requirements', test: 'tests', defect: 'defects', risk: 'risks', run: 'runs', review: 'reviews' }
  return `/${base[kind]}/${id}/comments`
}

/** Where a thread's item is shown. */
export function threadTarget(t: Pick<DiscussionThread, 'root'>): RouteTarget {
  const params = { projectId: t.root.project_id }
  const id = t.root.entity_id
  switch (t.root.entity_type) {
    case 'requirement':
      return { name: 'requirements', params, query: { item: id, tab: 'comments' } }
    case 'test':
      return { name: 'tests', params, query: { item: id, tab: 'comments' } }
    case 'defect':
      return { name: 'defects', params, query: { item: id } }
    case 'risk':
      return { name: 'risks', params, query: { risk: id } }
    case 'run':
      return { name: 'run', params: { ...params, runId: id }, query: { tab: 'discussion' } }
    case 'review':
      return { name: 'reviews', params: {}, query: { review: id } }
  }
  return { name: 'dashboard', params }
}
