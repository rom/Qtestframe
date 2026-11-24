// Organisations and accounts as super administrators manage them
// (Administration → Organisations / Users): types of GET /admin/tenants and
// the helpers of the views.
import type { Membership, Tenant, User } from '@/lib/types'

/** Limits and rules of an organisation only super administrators change. */
export interface TenantPolicy {
  max_members: number
  max_projects: number
  max_storage_mb: number
  ai_disabled: boolean
  allowed_providers: string[]
  email_domains: string[]
  note: string
  /** overrides of the system retention (days, 0 = forever); missing keys use the system setting */
  retention?: Record<string, number>
}

export interface TenantUsage {
  members: number
  projects: number
  requirements: number
  tests: number
  runs: number
  files: number
  storage_bytes: number
  last_activity?: string
}

export interface AdminTenant extends Tenant {
  policy: TenantPolicy
  usage: TenantUsage
  ai_enabled: boolean
  erecords_enabled: boolean
  mfa_required: boolean
}

/** "used / limit", or the count alone without a limit. */
export function quotaText(used: number, limit: number): string {
  return limit > 0 ? `${used} / ${limit}` : String(used)
}

/** Whether a usage has reached (or passed) its limit. */
export function atLimit(used: number, limit: number): boolean {
  return limit > 0 && used >= limit
}

/** Storage in use compared with its limit in MB (0 = none). */
export function storageAtLimit(bytes: number, limitMB: number): boolean {
  return limitMB > 0 && bytes >= limitMB * 1024 * 1024
}

/** An empty policy (no limits, no rules). */
export function blankPolicy(): TenantPolicy {
  return { max_members: 0, max_projects: 0, max_storage_mb: 0, ai_disabled: false, allowed_providers: [], email_domains: [], note: '' }
}

/** Domains typed as text (spaces, commas, lines; a leading @ is dropped) → list. */
export function parseDomains(text: string): string[] {
  const out: string[] = []
  for (let d of text.split(/[\s,;]+/)) {
    d = d.trim().replace(/^@/, '').toLowerCase()
    if (d && !out.includes(d)) out.push(d)
  }
  return out
}

/** The sign-in methods a policy can allow: password and every provider. */
export function methodOptions(providers: { oidc: { name: string; display_name: string }[]; saml: { name: string; display_name: string }[] }, local: string): { value: string; label: string }[] {
  return [
    { value: 'local', label: local },
    ...providers.oidc.map((p) => ({ value: `oidc:${p.name}`, label: `${p.display_name || p.name} (OIDC)` })),
    ...providers.saml.map((p) => ({ value: `saml:${p.name}`, label: `${p.display_name || p.name} (SAML)` })),
  ]
}

/** The typed confirmation of "Delete organisation" matches its slug. */
export function deleteConfirmed(t: Pick<Tenant, 'slug'>, typed: string): boolean {
  return typed.trim().toLowerCase() === t.slug.toLowerCase()
}

/** Filters of the user list. */
export interface UserFilter {
  text: string
  status: 'all' | 'active' | 'disabled'
  superAdmin: boolean
  provider: string
  mfa: 'all' | 'on' | 'off'
}

export function blankUserFilter(): UserFilter {
  return { text: '', status: 'all', superAdmin: false, provider: '', mfa: 'all' }
}

/** The users matching a filter. */
export function filterUsers(users: User[], f: UserFilter): User[] {
  const text = f.text.trim().toLowerCase()
  return users.filter((u) => {
    if (text && !`${u.email} ${u.display_name}`.toLowerCase().includes(text)) return false
    if (f.status === 'active' && u.disabled) return false
    if (f.status === 'disabled' && !u.disabled) return false
    if (f.superAdmin && !u.super_admin) return false
    if (f.provider && (u.auth_provider || 'local') !== f.provider) return false
    if (f.mfa === 'on' && !u.mfa_enabled) return false
    if (f.mfa === 'off' && u.mfa_enabled) return false
    return true
  })
}

/** The sign-in methods of the users (for the provider filter). */
export function userProviders(users: User[]): string[] {
  return [...new Set(users.map((u) => u.auth_provider || 'local'))].sort()
}

/** Whether u is the only enabled super administrator (cannot be demoted or disabled). */
export function lastSuperAdmin(users: User[], u: User): boolean {
  return u.super_admin && !u.disabled && users.filter((x) => x.super_admin && !x.disabled).length <= 1
}

export interface UserMemberships {
  items: Membership[]
  sessions: number
}
