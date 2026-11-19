// User preferences (Me.user.preferences): normalisation, defaults and the
// pure decisions the app derives from them.
import type { Preferences } from './types'

export const THEMES = ['light', 'dark', 'system'] as const
export const START_PAGES = ['projects', 'last_project', 'default_project', 'search'] as const
export const PAGE_SIZES = [10, 25, 50, 100] as const
export const DATE_FORMATS = ['locale', 'iso', 'relative'] as const

export const DEFAULT_PAGE_SIZE = 25

/** Clean up whatever the server sent: unknown values become "default". */
export function normalizePrefs(raw: Partial<Preferences> | null | undefined): Preferences {
  const p = raw ?? {}
  const out: Preferences = {}
  if (p.theme && (THEMES as readonly string[]).includes(p.theme)) out.theme = p.theme
  if (p.start_page && (START_PAGES as readonly string[]).includes(p.start_page)) out.start_page = p.start_page
  if (p.default_project_id) out.default_project_id = String(p.default_project_id)
  if (p.default_tenant_id) out.default_tenant_id = String(p.default_tenant_id)
  const ps = Number(p.page_size)
  if ((PAGE_SIZES as readonly number[]).includes(ps)) out.page_size = ps
  if (p.date_format && (DATE_FORMATS as readonly string[]).includes(p.date_format)) out.date_format = p.date_format
  if (p.timezone) out.timezone = String(p.timezone)
  if (p.compact_tables === true) out.compact_tables = true
  if (p.hide_analysis === true) out.hide_analysis = true
  if (typeof p.create_linked_test === 'boolean') out.create_linked_test = p.create_linked_test
  return out
}

const KEYS: (keyof Preferences)[] = [
  'theme',
  'start_page',
  'default_project_id',
  'default_tenant_id',
  'page_size',
  'date_format',
  'timezone',
  'compact_tables',
  'hide_analysis',
  'create_linked_test',
]

/**
 * PATCH /auth/me body for a preferences form: every key is present; default
 * values ("", 0, false, null) are sent as null so the server resets them.
 * `create_linked_test` keeps an explicit `false`.
 */
export function preferencesPatch(form: Partial<Preferences>): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const k of KEYS) {
    const v = form[k] as unknown
    if (k === 'create_linked_test') out[k] = typeof v === 'boolean' ? v : null
    else if (v === '' || v === 0 || v === false || v === null || v === undefined) out[k] = null
    else out[k] = v
  }
  return out
}

/** Effective dark mode: preference first, then this device's toggle, then the OS. */
export function resolveDark(pref: string | null | undefined, local: string | null | undefined, systemDark: boolean): boolean {
  if (pref === 'dark') return true
  if (pref === 'light') return false
  if (pref === 'system') return systemDark
  if (local === 'dark') return true
  if (local === 'light') return false
  return systemDark
}

/** Default rows per page. */
export function pageSize(prefs: Preferences | null | undefined, fallback = DEFAULT_PAGE_SIZE): number {
  const n = Number(prefs?.page_size)
  return (PAGE_SIZES as readonly number[]).includes(n) ? n : fallback
}

/** Rows-per-page options always containing the preferred size, sorted. */
export function rowsOptions(size: number, base: readonly number[] = [10, 25, 50, 100, 250]): number[] {
  return [...new Set([...base, size])].filter((n) => n > 0).sort((a, b) => a - b)
}

/** Default of the "Also create linked test" checkbox. */
export function createLinkedTestDefault(prefs: Preferences | null | undefined, serverDefault: boolean | null | undefined): boolean {
  if (typeof prefs?.create_linked_test === 'boolean') return prefs.create_linked_test
  return serverDefault !== false
}

export type StartRoute = { name: string; params?: Record<string, string> }

/**
 * Where "/" (and the post-login redirect) leads. Project-based start pages
 * fall back to the project list when the project is unknown.
 */
export function startRoute(prefs: Preferences | null | undefined, ctx: { lastProjectId?: string | null; projectIds?: readonly string[] | null } = {}): StartRoute {
  const known = (id: string | null | undefined): id is string => !!id && (!ctx.projectIds || ctx.projectIds.includes(id))
  switch (prefs?.start_page) {
    case 'search':
      return { name: 'search' }
    case 'last_project':
      return known(ctx.lastProjectId) ? { name: 'dashboard', params: { projectId: ctx.lastProjectId } } : { name: 'projects' }
    case 'default_project':
      return known(prefs.default_project_id) ? { name: 'dashboard', params: { projectId: prefs.default_project_id } } : { name: 'projects' }
    default:
      return { name: 'projects' }
  }
}

// ---------- last opened project (per tenant, this browser) ----------

type KV = Pick<Storage, 'getItem' | 'setItem'>

function storage(): KV | null {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage
  } catch {
    return null
  }
}

export function lastProjectKey(tenantId: string | null | undefined): string {
  return 'qtf.lastProject' + (tenantId ? '.' + tenantId : '')
}

export function readLastProject(tenantId: string | null | undefined, store: KV | null = storage()): string | null {
  try {
    return store?.getItem(lastProjectKey(tenantId)) || null
  } catch {
    return null
  }
}

export function writeLastProject(tenantId: string | null | undefined, projectId: string, store: KV | null = storage()): void {
  try {
    store?.setItem(lastProjectKey(tenantId), projectId)
  } catch {
    /* storage unavailable */
  }
}

// ---------- time zones ----------

export const FALLBACK_TIMEZONES = [
  'UTC',
  'Africa/Cairo',
  'Africa/Johannesburg',
  'Africa/Lagos',
  'America/Anchorage',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'America/Mexico_City',
  'America/New_York',
  'America/Sao_Paulo',
  'America/Toronto',
  'Asia/Dubai',
  'Asia/Hong_Kong',
  'Asia/Kolkata',
  'Asia/Seoul',
  'Asia/Shanghai',
  'Asia/Singapore',
  'Asia/Tokyo',
  'Australia/Melbourne',
  'Australia/Sydney',
  'Europe/Amsterdam',
  'Europe/Berlin',
  'Europe/Helsinki',
  'Europe/Istanbul',
  'Europe/London',
  'Europe/Madrid',
  'Europe/Moscow',
  'Europe/Paris',
  'Europe/Rome',
  'Europe/Stockholm',
  'Europe/Warsaw',
  'Europe/Zurich',
  'Pacific/Auckland',
  'Pacific/Honolulu',
]

/**
 * IANA zones for the time zone select. Uses Intl.supportedValuesOf when the
 * runtime provides it, else a fallback list; `current` is always included.
 */
export function timeZones(current?: string | null, intl: { supportedValuesOf?: (k: string) => string[] } = Intl as never): string[] {
  let list: string[] = []
  try {
    const fn = intl.supportedValuesOf
    if (typeof fn === 'function') list = fn.call(intl, 'timeZone')
  } catch {
    list = []
  }
  if (!Array.isArray(list) || !list.length) list = [...FALLBACK_TIMEZONES]
  const set = new Set(list)
  if (!set.has('UTC')) set.add('UTC')
  if (current) set.add(current)
  return [...set].sort((a, b) => (a === 'UTC' ? -1 : b === 'UTC' ? 1 : a.localeCompare(b)))
}

/** The browser's own zone, if known. */
export function browserTimeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || ''
  } catch {
    return ''
  }
}
