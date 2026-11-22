// Pure helpers for the chat channels of a project (Slack, Teams, Mattermost,
// Rocket.Chat, Google Chat, Matrix, Discord).

export type ChatProvider = 'slack' | 'teams' | 'teams_legacy' | 'mattermost' | 'rocketchat' | 'googlechat' | 'matrix' | 'discord'

export const CHAT_PROVIDERS: ChatProvider[] = ['slack', 'teams', 'teams_legacy', 'mattermost', 'rocketchat', 'googlechat', 'matrix', 'discord']

/** Product names (not translated). */
export const PROVIDER_NAMES: Record<ChatProvider, string> = {
  slack: 'Slack',
  teams: 'Microsoft Teams',
  teams_legacy: 'Microsoft Teams (Office 365 connector)',
  mattermost: 'Mattermost',
  rocketchat: 'Rocket.Chat',
  googlechat: 'Google Chat',
  matrix: 'Matrix',
  discord: 'Discord',
}

export interface ChatFilters {
  failed_only: boolean
  min_priority: string
  release_id: string
}

export interface ChatChannel {
  id: string
  project_id: string
  name: string
  provider: ChatProvider
  /** write-only: never returned */
  url?: string
  token?: string
  has_secret: boolean
  target: string
  homeserver: string
  room: string
  events: string[]
  filters: ChatFilters
  language: 'en' | 'sv'
  digest_minutes: number
  enabled: boolean
  restricted_content: boolean
  failures: number
  disabled_reason: string
  created_by: string
  created_at: string
  updated_at: string
  last_delivery_at: string | null
  last_status: number
  last_error: string
  pending: number
  next_digest_at: string | null
}

export interface ChatDelivery {
  id: string
  channel_id: string
  event: string
  count: number
  status: 'queued' | 'sending' | 'retry' | 'sent' | 'failed'
  attempts: number
  status_code: number
  error: string
  created_at: string
  next_attempt_at: string | null
  sent_at: string | null
}

export interface ChatEvent {
  event: string
  group: string
  labels: Record<string, string>
}

export interface ChatGroup {
  id: string
  labels: Record<string, string>
}

export interface EventOptionGroup {
  label: string
  group: string
  items: { label: string; value: string }[]
}

function label(labels: Record<string, string>, lang: string, fallback: string): string {
  return labels[lang] ?? labels.en ?? fallback
}

/**
 * The events of the registry as option groups, in the order of the groups
 * the server gives; an event of an unknown group goes under the last one.
 */
export function eventOptionGroups(events: ChatEvent[], groups: ChatGroup[], lang: string): EventOptionGroup[] {
  const out = groups.map((g) => ({ group: g.id, label: label(g.labels, lang, g.id), items: [] as { label: string; value: string }[] }))
  for (const e of events) {
    const g = out.find((o) => o.group === e.group) ?? out[out.length - 1]
    g?.items.push({ label: label(e.labels, lang, e.event), value: e.event })
  }
  return out.filter((g) => g.items.length)
}

/** "All events" or the labels of the chosen events, shortened after max. */
export function eventsText(events: string[], all: ChatEvent[], lang: string, allText: string, max = 3): string {
  if (events.includes('*')) return allText
  const names = events.map((e) => {
    const ev = all.find((a) => a.event === e)
    return ev ? label(ev.labels, lang, e) : e
  })
  return names.length > max ? `${names.slice(0, max).join(', ')} +${names.length - max}` : names.join(', ')
}

export function validChatUrl(raw: string): boolean {
  try {
    const u = new URL(raw.trim())
    return (u.protocol === 'https:' || u.protocol === 'http:') && !!u.host && !u.username && !u.password
  } catch {
    return false
  }
}

export function validMatrixRoom(raw: string): boolean {
  return /^[!#][^:\s]+:\S+$/.test(raw.trim())
}

export type ChatForm = Partial<ChatChannel> & { url?: string; token?: string }

/** Problems of a channel form by field ("" fields are fine). */
export function chatFormProblems(f: ChatForm, creating: boolean): Record<string, boolean> {
  const p: Record<string, boolean> = {}
  if (!f.name?.trim()) p.name = true
  if (!f.events?.length) p.events = true
  if ((f.digest_minutes ?? 0) < 0 || (f.digest_minutes ?? 0) > 1440) p.digest = true
  if (f.provider === 'matrix') {
    if (!validChatUrl(f.homeserver ?? '')) p.homeserver = true
    if (!validMatrixRoom(f.room ?? '')) p.room = true
    if (creating && !f.token?.trim()) p.token = true
  } else {
    const u = f.url?.trim() ?? ''
    if ((creating || u) && !validChatUrl(u)) p.url = true
  }
  return p
}

/** The PATCH / POST body of a form: a blank secret is left out (kept). */
export function chatBody(f: ChatForm): Record<string, unknown> {
  const body: Record<string, unknown> = {
    name: f.name?.trim(),
    events: f.events,
    filters: f.filters,
    language: f.language,
    digest_minutes: f.digest_minutes ?? 0,
    enabled: f.enabled,
    restricted_content: f.restricted_content,
  }
  if (f.provider === 'matrix') {
    body.homeserver = f.homeserver?.trim()
    body.room = f.room?.trim()
    if (f.token?.trim()) body.token = f.token.trim()
  } else if (f.url?.trim()) body.url = f.url.trim()
  return body
}

/** State of a channel for its status tag. */
export function channelState(c: Pick<ChatChannel, 'enabled' | 'disabled_reason' | 'last_delivery_at' | 'last_status' | 'last_error'>): 'off' | 'disabled' | 'ok' | 'failed' | 'never' {
  if (!c.enabled) return c.disabled_reason ? 'disabled' : 'off'
  if (!c.last_delivery_at) return 'never'
  return c.last_error || c.last_status >= 300 || c.last_status === 0 ? 'failed' : 'ok'
}

/** PrimeVue Tag severity of a delivery status. */
export function deliverySeverity(status: ChatDelivery['status']): 'success' | 'danger' | 'warn' | 'info' {
  if (status === 'sent') return 'success'
  if (status === 'failed') return 'danger'
  if (status === 'retry') return 'warn'
  return 'info'
}
