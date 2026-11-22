// Pure helpers for notifications, review requests and sign-offs.
import type { AppNotification, ChecklistItem, ReviewRequest, Signoff } from './types'

export interface RouteTarget {
  name: string
  params?: Record<string, string>
  query?: Record<string, string>
}

/** Where a notification leads: the item it is about, else the project. */
export function notificationTarget(n: Pick<AppNotification, 'project_id' | 'entity_type' | 'entity_id'>): RouteTarget | null {
  if (!n.project_id) return null
  const params = { projectId: n.project_id }
  switch (n.entity_type) {
    case 'requirement':
      return { name: 'requirements', params, query: { item: n.entity_id } }
    case 'test':
      return { name: 'tests', params, query: { item: n.entity_id } }
    case 'defect':
      return { name: 'defects', params, query: { item: n.entity_id } }
    case 'risk':
      return { name: 'risks', params, query: { risk: n.entity_id } }
    case 'waiver':
      return { name: 'waivers', params, query: { waiver: n.entity_id } }
    case 'execution':
      return { name: 'runs', params }
    case 'cycle':
      return { name: 'cycle', params: { ...params, cycleId: n.entity_id } }
  }
  return { name: 'dashboard', params }
}

/** Icon per notification event. */
export function notificationIcon(event: string): string {
  if (event === 'mention') return 'pi pi-at'
  if (event === 'assigned' || event === 'defect.assigned') return 'pi pi-user'
  const [group, action] = event.split('.')
  if (group === 'review') return action === 'completed' ? 'pi pi-verified' : 'pi pi-eye'
  if (event.startsWith('defect.')) return 'pi pi-exclamation-circle'
  if (event === 'project.shared') return 'pi pi-share-alt'
  if (group === 'waiver') return action === 'requested' ? 'pi pi-pen-to-square' : action === 'approved' ? 'pi pi-verified' : 'pi pi-ban'
  return 'pi pi-bell'
}

/** i18n key suffix for an event ("defect.status" → "defect_status"). */
export function eventKey(event: string): string {
  return event.replace(/[^a-z0-9]+/gi, '_')
}

/** Text of an unread badge: "", "1"…"99", "99+". */
export function badgeText(n: number): string {
  if (!n || n < 0) return ''
  return n > 99 ? '99+' : String(n)
}

/** Checklist items that still keep an approval back (neither checked nor explained). */
export function openChecklistItems(list: ChecklistItem[]): ChecklistItem[] {
  return list.filter((c) => !c.checked && !(c.note ?? '').trim())
}

/** Whether the reviewer may approve now. */
export function canApprove(list: ChecklistItem[]): boolean {
  return openChecklistItems(list).length === 0
}

/** Whether a request for changes has the comment it needs. */
export function canRequestChanges(comment: string): boolean {
  return comment.trim().length > 0
}

/** Checklist texts (one per line) → items; blank lines are dropped. */
export function parseChecklist(text: string): ChecklistItem[] {
  return text
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
    .map((t) => ({ text: t, checked: false }))
}

/** Items → one text per line. */
export function checklistText(list: { text: string }[]): string {
  return list.map((c) => c.text).join('\n')
}

/** Severity of a review status tag. */
export function reviewSeverity(status: string): 'success' | 'danger' | 'secondary' | 'info' {
  if (status === 'approved') return 'success'
  if (status === 'changes_requested') return 'danger'
  if (status === 'cancelled') return 'secondary'
  return 'info'
}

/** Whether a pending review is past its due date (YYYY-MM-DD) on day today. */
export function overdue(r: Pick<ReviewRequest, 'status' | 'due_date'>, today: string): boolean {
  return r.status === 'pending' && !!r.due_date && r.due_date < today
}

/** Today as YYYY-MM-DD in local time. */
export function isoDay(d = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

/** Short form of a content hash for display. */
export function shortHash(h: string): string {
  return h.slice(0, 12)
}

/** Sign-offs grouped by meaning, in the order authored, reviewed, approved, released. */
export function signoffsByMeaning(list: Signoff[]): { meaning: string; items: Signoff[] }[] {
  const order = ['authored', 'reviewed', 'approved', 'released']
  const out: { meaning: string; items: Signoff[] }[] = []
  for (const m of order) {
    const items = list.filter((s) => s.meaning === m)
    if (items.length) out.push({ meaning: m, items })
  }
  return out
}

/** Webhook URL check matching the server: http(s) with a host and no credentials. */
export function validWebhookUrl(raw: string): boolean {
  try {
    const u = new URL(raw.trim())
    return (u.protocol === 'https:' || u.protocol === 'http:') && !!u.host && !u.username && !u.password
  } catch {
    return false
  }
}

/** A delivery outcome: "ok", "failed" or "never". */
export function deliveryState(w: { last_delivery_at: string | null; last_status: number; last_error: string }): 'ok' | 'failed' | 'never' {
  if (!w.last_delivery_at) return 'never'
  return w.last_error || w.last_status >= 300 || w.last_status === 0 ? 'failed' : 'ok'
}
