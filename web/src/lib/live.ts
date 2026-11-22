// Live updates: the events of GET /api/v1/events/stream and the pure
// decisions around them (reconnect delays, which events concern a view).

export type LiveKind = 'change' | 'notification' | 'presence' | 'reset'

export interface LivePresenceInfo {
  user_id: string
  name: string
  avatar?: string
  state: 'viewing' | 'editing' | 'left'
}

/** One event of the stream; never titles or content. */
export interface LiveEvent {
  id?: string
  kind: LiveKind
  /** entity type: requirement, test, run, execution, cycle, exploratory_session, defect, risk, release, project */
  type?: string
  entity_id?: string
  /** the run of an execution */
  parent_id?: string
  project_id?: string
  /** version of a requirement or test after the change */
  version?: number
  action?: string
  actor_id?: string
  actor_name?: string
  presence?: LivePresenceInfo
  time?: string
}

export const STREAM_PATH = '/api/v1/events/stream'

/** The stream URL; a reconnect made by the client resumes with last_event_id. */
export function streamUrl(lastEventId?: string | null): string {
  return lastEventId ? `${STREAM_PATH}?last_event_id=${encodeURIComponent(lastEventId)}` : STREAM_PATH
}

/** Reconnect delay: exponential from 1 s up to 60 s, with ±20 % jitter. */
export function backoffDelay(attempt: number, random: () => number = Math.random): number {
  const base = Math.min(60_000, 1000 * 2 ** Math.max(0, Math.min(attempt, 10)))
  return Math.round(base * (0.8 + 0.4 * random()))
}

/** What a view cares about: entity types, a project, an entity or a parent. */
export interface LiveFilter {
  types?: string[]
  projectId?: string | null
  entityId?: string | null
  /** the run of executions (the run itself matches too) */
  parentId?: string | null
  /** only actions starting with this (e.g. "comment.") */
  actionPrefix?: string
  /** react to the signed-in user's own changes too (default: yes, other tabs) */
  includeOwn?: boolean
  /** the signed-in user's id (to recognise own events) */
  me?: string | null
}

/** Whether a change event concerns a view; a reset concerns every view. */
export function matches(ev: LiveEvent, f: LiveFilter): boolean {
  if (ev.kind === 'reset') return true
  if (ev.kind !== 'change') return false
  if (f.types && !f.types.includes(ev.type ?? '')) return false
  if (f.projectId && ev.project_id !== f.projectId) return false
  if (f.entityId && ev.entity_id !== f.entityId) return false
  if (f.parentId && ev.parent_id !== f.parentId && ev.entity_id !== f.parentId) return false
  if (f.actionPrefix && !(ev.action ?? '').startsWith(f.actionPrefix)) return false
  if (f.includeOwn === false && f.me && ev.actor_id === f.me) return false
  return true
}

/** Key of an item in the presence map. */
export function presenceKey(type: string, id: string): string {
  return `${type}:${id}`
}

/** Apply a presence event to the list of people on an item (sorted by name). */
export function applyPresence(list: readonly LivePresenceInfo[], p: LivePresenceInfo): LivePresenceInfo[] {
  const rest = list.filter((x) => x.user_id !== p.user_id)
  if (p.state !== 'left') rest.push({ ...p })
  return rest.sort((a, b) => a.name.localeCompare(b.name) || a.user_id.localeCompare(b.user_id))
}

/** A change of an open item that its editor should tell about. */
export interface NewerVersion {
  by: string
  version: number
  deleted: boolean
}

/**
 * Whether an event means the open item was changed by someone else: a
 * newer version (or any change of an unversioned item), or its deletion.
 */
export function newerVersion(ev: LiveEvent, type: string, id: string, version: number | null | undefined, me: string | null | undefined): NewerVersion | null {
  if (ev.kind !== 'change' || ev.type !== type || ev.entity_id !== id) return null
  if (me && ev.actor_id === me) return null
  const deleted = !!ev.action && /\.delete$/.test(ev.action)
  if (!deleted && version && ev.version && ev.version <= version) return null
  return { by: ev.actor_name || '', version: ev.version ?? 0, deleted }
}

/**
 * Throttle for screen reader announcements: at most one message per
 * interval, and never the same text twice in a row within two intervals.
 */
export function makeThrottle(intervalMs: number, now: () => number = Date.now) {
  let last = -Infinity
  let lastText = ''
  return (text: string): boolean => {
    const t = now()
    if (t - last < intervalMs) return false
    if (text === lastText && t - last < intervalMs * 2) return false
    last = t
    lastText = text
    return true
  }
}
