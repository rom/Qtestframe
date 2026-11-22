// Test environments, their bookings and test data sets: types and pure
// helpers (status colours, booking times, the week timeline, capacity,
// expiry, CSV paste). Mirrors internal/model/environments.go; see
// docs/api.md "Test environments" and "Test data sets".
import type { DataRow } from './types'

export const ENVIRONMENT_TYPES = ['dev', 'test', 'staging', 'pre_prod', 'prod_like', 'lab', 'hil_rig', 'device'] as const
export const ENVIRONMENT_STATUSES = ['available', 'in_use', 'maintenance', 'broken', 'retired'] as const
export const BOOKING_STATUSES = ['pending', 'approved', 'rejected', 'cancelled'] as const
/** Bookings that take up capacity. */
export const ACTIVE_BOOKING_STATUSES: readonly string[] = ['pending', 'approved']
export const DATA_SET_KINDS = ['table', 'files'] as const
export const DATA_CLASSIFICATIONS = ['synthetic', 'anonymised', 'production_copy'] as const
export const DATA_SET_STATUSES = ['active', 'archived'] as const
/** A data set expiring within this many days is "expiring" (model.DataSetExpiryDays). */
export const DATA_SET_EXPIRY_DAYS = 14

export interface Component {
  name: string
  version: string
}

export interface Environment {
  id: string
  /** "" for an environment shared by the organisation */
  project_id: string
  shared: boolean
  name: string
  type: string
  urls: string[]
  description: string
  components: Component[]
  owner_id: string
  status: string
  status_note: string
  status_by_health: boolean
  tags: string[]
  configuration_ids: string[]
  variant_ids: string[]
  capacity: number
  requires_approval: boolean
  credentials_ref: string
  has_secret: boolean
  health_url: string
  health_interval_minutes: number
  health_status: string
  health_checked_at: string | null
  health_error: string
  version: number
  created_by: string
  created_at: string
  updated_at: string
  booked_now: number
  next_booking?: string | null
  run_count: number
  project_key?: string
}

export interface Booking {
  id: string
  environment_id: string
  project_id: string
  user_id: string
  booked_by: string
  starts_at: string
  ends_at: string
  purpose: string
  run_id: string | null
  cycle_id: string | null
  session_id: string | null
  status: string
  decided_by: string
  decided_at: string | null
  decision_note: string
  created_at: string
  updated_at: string
  environment_name?: string
  project_key?: string
  run_name?: string
  cycle_name?: string
  session_key?: string
  /** booked for a project the caller does not see */
  hidden?: boolean
}

/** The booking part of a run, cycle or session dialog. */
export interface BookingInput {
  starts_at: string
  ends_at: string
  purpose: string
}

export interface ReleaseEnvironment {
  environment_id: string
  name: string
  type: string
  components: Component[]
  executions: number
  passed: number
  failed: number
  blocked: number
  last_at: string | null
}

export interface DataLink {
  id: string
  data_set_id: string
  item_type: 'test' | 'configuration' | 'run' | string
  item_id: string
  usage: 'source' | 'reference' | string
  created_by: string
  created_at: string
  item_key: string
  item_title: string
  data_set_name?: string
}

export interface DataSet {
  id: string
  project_id: string
  name: string
  description: string
  kind: string
  parameters: string[]
  data_rows: DataRow[]
  classification: string
  owner_id: string
  expires_on: string
  tags: string[]
  status: string
  version: number
  created_by: string
  created_at: string
  updated_at: string
  links: DataLink[]
  usage_count: number
  last_used_at: string | null
  open_refreshes: number
  file_count: number
  expired: boolean
  expiring: boolean
}

export interface DataUsage {
  id: string
  data_set_id: string
  data_version: number
  run_id: string | null
  test_case_id: string | null
  used_by: string
  used_at: string
  run_name?: string
  test_key?: string
}

export interface RefreshRequest {
  id: string
  data_set_id: string
  requester_id: string
  requested_by: string
  reason: string
  needed_by: string
  status: string
  resolved_by: string
  resolved_at: string | null
  resolution_note: string
  created_at: string
  updated_at: string
}

type Severity = 'success' | 'info' | 'warn' | 'danger' | 'secondary' | 'contrast'

/** Tag severity of an environment status. */
export function environmentSeverity(status: string): Severity {
  switch (status) {
    case 'available':
      return 'success'
    case 'in_use':
      return 'info'
    case 'maintenance':
      return 'warn'
    case 'broken':
      return 'danger'
  }
  return 'secondary'
}

/** Tag severity of a booking status. */
export function bookingSeverity(status: string): Severity {
  return status === 'approved' ? 'success' : status === 'pending' ? 'warn' : status === 'rejected' ? 'danger' : 'secondary'
}

/** Tag severity of a data classification (a production copy warns). */
export function classificationSeverity(c: string): Severity {
  return c === 'production_copy' ? 'danger' : c === 'anonymised' ? 'warn' : 'success'
}

/** Icon of an environment type. */
export function environmentIcon(type: string): string {
  switch (type) {
    case 'lab':
    case 'hil_rig':
      return 'pi pi-microchip'
    case 'device':
      return 'pi pi-mobile'
    case 'dev':
      return 'pi pi-code'
    case 'prod_like':
    case 'pre_prod':
      return 'pi pi-globe'
  }
  return 'pi pi-server'
}

/** Whether an environment can be booked and selected (not retired). */
export function bookable(e: Pick<Environment, 'status'>): boolean {
  return e.status !== 'retired'
}

/** Components as "app 2.3.1, DB 16". */
export function componentsText(cs: readonly Component[] | null | undefined): string {
  return (cs ?? []).map((c) => `${c.name} ${c.version}`.trim()).join(', ')
}

/** Drops empty component rows and trims the others. */
export function cleanComponents(cs: readonly Component[]): Component[] {
  return cs.map((c) => ({ name: c.name.trim(), version: c.version.trim() })).filter((c) => c.name || c.version)
}

/** Duplicate component names (ignoring case). */
export function duplicateComponents(cs: readonly Component[]): string[] {
  const seen = new Set<string>()
  const dup = new Set<string>()
  for (const c of cs) {
    const k = c.name.trim().toLowerCase()
    if (!k) continue
    if (seen.has(k)) dup.add(c.name.trim())
    seen.add(k)
  }
  return [...dup]
}

const pad = (n: number) => String(n).padStart(2, '0')

/** An ISO time as the value of an <input type="datetime-local"> (local time). */
export function toLocalInput(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** The value of a datetime-local input as an ISO time ("" when empty or invalid). */
export function fromLocalInput(v: string): string {
  if (!v) return ''
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? '' : d.toISOString()
}

/** A default booking: from the next full hour for hours (local datetime-local values). */
export function defaultBookingWindow(now: Date, hours = 4): { from: string; to: string } {
  const start = new Date(now)
  start.setMinutes(0, 0, 0)
  start.setHours(start.getHours() + 1)
  const end = new Date(start.getTime() + hours * 3600_000)
  return { from: toLocalInput(start.toISOString()), to: toLocalInput(end.toISOString()) }
}

/** Whether a booking window is valid (end after start). */
export function validWindow(from: string, to: string): boolean {
  const a = Date.parse(from)
  const b = Date.parse(to)
  return !Number.isNaN(a) && !Number.isNaN(b) && b > a
}

/**
 * The largest number of the bookings that overlap one another at a moment
 * inside [from, to) (store.MaxOverlap): bookings that only touch do not
 * overlap.
 */
export function maxOverlap(list: readonly Pick<Booking, 'starts_at' | 'ends_at'>[], from: string, to: string): number {
  const lo = Date.parse(from)
  const hi = Date.parse(to)
  const evs: [number, number][] = []
  for (const b of list) {
    const s = Math.max(Date.parse(b.starts_at), lo)
    const e = Math.min(Date.parse(b.ends_at), hi)
    if (s < e) evs.push([s, 1], [e, -1])
  }
  evs.sort((a, b) => a[0] - b[0] || a[1] - b[1])
  let n = 0
  let best = 0
  for (const [, d] of evs) {
    n += d
    best = Math.max(best, n)
  }
  return best
}

/** Whether one more booking fits in [from, to) next to the active bookings. */
export function fits(capacity: number, bookings: readonly Booking[], from: string, to: string, exceptId = ''): boolean {
  const active = bookings.filter((b) => b.id !== exceptId && ACTIVE_BOOKING_STATUSES.includes(b.status))
  return maxOverlap(active, from, to) + 1 <= Math.max(1, capacity)
}

/** The Monday 00:00 (local) of the week of d. */
export function startOfWeek(d: Date): Date {
  const x = new Date(d)
  x.setHours(0, 0, 0, 0)
  const wd = (x.getDay() + 6) % 7
  x.setDate(x.getDate() - wd)
  return x
}

/** The days of a timeline from start (local midnights). */
export function timelineDays(start: Date, days = 7): Date[] {
  return Array.from({ length: days }, (_, i) => {
    const d = new Date(start)
    d.setDate(d.getDate() + i)
    return d
  })
}

export interface TimelineBar {
  booking: Booking
  /** position and width in % of the timeline */
  left: number
  width: number
  /** the row of the bar (overlapping bookings get their own lanes) */
  lane: number
}

/** Bars of the bookings that fall in [from, to), in lanes so that overlapping bars do not cover each other. */
export function timelineBars(bookings: readonly Booking[], from: Date, to: Date): { bars: TimelineBar[]; lanes: number } {
  const lo = from.getTime()
  const hi = to.getTime()
  const span = hi - lo
  const visible = bookings
    .filter((b) => Date.parse(b.ends_at) > lo && Date.parse(b.starts_at) < hi)
    .sort((a, b) => Date.parse(a.starts_at) - Date.parse(b.starts_at) || a.id.localeCompare(b.id))
  const laneEnds: number[] = []
  const bars: TimelineBar[] = []
  for (const b of visible) {
    const s = Math.max(Date.parse(b.starts_at), lo)
    const e = Math.min(Date.parse(b.ends_at), hi)
    let lane = laneEnds.findIndex((end) => end <= s)
    if (lane < 0) {
      lane = laneEnds.length
      laneEnds.push(e)
    } else laneEnds[lane] = e
    bars.push({ booking: b, left: ((s - lo) / span) * 100, width: Math.max(((e - s) / span) * 100, 0.8), lane })
  }
  return { bars, lanes: Math.max(1, laneEnds.length) }
}

/** expired | expiring | "" for an expiry date (YYYY-MM-DD) and today. */
export function expiryState(expiresOn: string | null | undefined, today: string): 'expired' | 'expiring' | '' {
  if (!expiresOn || !/^\d{4}-\d{2}-\d{2}$/.test(today)) return ''
  if (expiresOn < today) return 'expired'
  const t = new Date(today + 'T00:00:00Z')
  t.setUTCDate(t.getUTCDate() + DATA_SET_EXPIRY_DAYS)
  return expiresOn <= t.toISOString().slice(0, 10) ? 'expiring' : ''
}

/** Splits one CSV or tab-separated line (quotes with "" escapes). */
function splitLine(line: string, sep: string): string[] {
  const out: string[] = []
  let cur = ''
  let q = false
  for (let i = 0; i < line.length; i++) {
    const c = line[i]
    if (q) {
      if (c === '"' && line[i + 1] === '"') {
        cur += '"'
        i++
      } else if (c === '"') q = false
      else cur += c
    } else if (c === '"') q = true
    else if (c === sep) {
      out.push(cur)
      cur = ''
    } else cur += c
  }
  out.push(cur)
  return out.map((s) => s.trim())
}

/**
 * Parses pasted table data (CSV, semicolon or tab separated; the first line
 * names the parameters) into parameters and rows. A column named "name"
 * (or "#") gives the row names.
 */
export function parseTable(text: string): { parameters: string[]; rows: DataRow[] } {
  const lines = text.split(/\r?\n/).filter((l) => l.trim() !== '')
  if (!lines.length) return { parameters: [], rows: [] }
  const head = lines[0]
  const sep = head.includes('\t') ? '\t' : head.includes(';') && !head.includes(',') ? ';' : ','
  const cols = splitLine(head, sep)
  const nameCol = cols.findIndex((c) => c.toLowerCase() === 'name' || c === '#')
  const parameters = cols.filter((c, i) => i !== nameCol && c !== '')
  const rows: DataRow[] = lines.slice(1).map((l) => {
    const cells = splitLine(l, sep)
    const values: Record<string, string> = {}
    cols.forEach((c, i) => {
      if (i !== nameCol && c !== '') values[c] = cells[i] ?? ''
    })
    return { name: nameCol >= 0 ? (cells[nameCol] ?? '') : '', values }
  })
  return { parameters, rows }
}
