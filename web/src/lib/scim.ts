// SCIM provisioning (Administration → Provisioning): what the admin page
// shows about tokens, groups and provisioning events (internal/api/scim.go).

/** A SCIM token as listed (the secret is only returned when created). */
export interface ScimToken {
  id: string
  name: string
  prefix: string
  created_by: string
  created_at: string
  expires_at?: string | null
  last_used?: string | null
}

/** A group pushed by the identity provider and the role it grants ('' = none). */
export interface ScimGroup {
  id: string
  display_name: string
  external_id: string
  role: string
  members: number
}

/** GET /scim: the provisioning set-up of the organisation. */
export interface ScimOverview {
  base_url: string
  /** role of provisioned users without a mapped group ('' = no access) */
  default_role: string
  tokens: ScimToken[]
  groups: ScimGroup[]
  users: number
  active_users: number
}

/** Provisioning events with their own label (audit actions "scim.<kind>.<verb>"). */
export const SCIM_EVENTS = [
  'user_create',
  'user_update',
  'user_deactivate',
  'user_reactivate',
  'user_delete',
  'group_create',
  'group_update',
  'group_delete',
  'group_role',
  'settings',
  'token_create',
  'token_delete',
] as const
export type ScimEvent = (typeof SCIM_EVENTS)[number] | 'other'

/** The label key suffix of an audit action ("scim.user.create" → "user_create"). */
export function scimEvent(action: string): ScimEvent {
  const key = action.replace(/^scim\./, '').replace(/\./g, '_')
  return action.startsWith('scim.') && (SCIM_EVENTS as readonly string[]).includes(key) ? (key as ScimEvent) : 'other'
}

/** Tag colour of an event. */
export function scimEventSeverity(action: string): 'success' | 'warn' | 'danger' | 'info' | 'secondary' {
  const e = scimEvent(action)
  if (e.endsWith('_create') || e === 'user_reactivate') return 'success'
  if (e.endsWith('_delete')) return 'danger'
  if (e === 'user_deactivate') return 'warn'
  if (e === 'group_role' || e === 'settings') return 'info'
  return 'secondary'
}

const ACTOR_PREFIX = 'SCIM: '

/** Who made a provisioning change: the identity provider's token or a person. */
export function scimActor(email: string): { scim: boolean; name: string } {
  return email.startsWith(ACTOR_PREFIX) ? { scim: true, name: email.slice(ACTOR_PREFIX.length) } : { scim: false, name: email }
}

/** Whether a token has expired (no expiry = never). */
export function tokenExpired(t: Pick<ScimToken, 'expires_at'>, now = Date.now()): boolean {
  return !!t.expires_at && new Date(t.expires_at).getTime() <= now
}

/** Groups without a role first (they need attention), then by name. */
export function sortGroups(groups: readonly ScimGroup[]): ScimGroup[] {
  return [...groups].sort((a, b) => Number(!!a.role) - Number(!!b.role) || a.display_name.localeCompare(b.display_name))
}

/** Role options with a first entry for "none" (value ''). */
export function withNone<T extends { label: string; value: string }>(options: readonly T[], noneLabel: string): { label: string; value: string }[] {
  return [{ label: noneLabel, value: '' }, ...options.filter((o) => o.value !== '')]
}
