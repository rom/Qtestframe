// Scheduled backups (Administration → Backup, super admins): types of
// GET /admin/backups, POST /admin/backups/run and
// POST /admin/backups/{id}/verify, and the helpers of the view.

export type BackupStatus = 'running' | 'succeeded' | 'failed'
export type BackupKind = 'schedule' | 'manual'

/** One backup the server took (table backup_runs). */
export interface BackupRun {
  id: string
  kind: BackupKind
  status: BackupStatus
  started_by: string
  started_at: string
  finished_at?: string
  file_name: string
  size: number
  sha256: string
  encrypted: boolean
  remote_key: string
  rows: number
  schema_version: number
  error: string
  pruned_at?: string
  verified_at?: string
  verify_error: string
}

/** The settings of the scheduled backups (section backup of the system settings; no secrets). */
export interface BackupSettings {
  enabled: boolean
  schedule: string
  schedule_error?: string
  directory: string
  keep_last: number
  keep_days: number
  encrypted: boolean
  s3_enabled: boolean
  s3_target?: string
  next_run_at?: string
}

export interface BackupList {
  settings: BackupSettings
  running: boolean
  runs: BackupRun[]
}

export interface BackupManifest {
  format_version: number
  schema_version: number
  created_at: string
  app_version: string
  driver: string
  tables: Record<string, number>
}

/** The outcome of "Verify". */
export interface BackupCheck {
  ok: boolean
  error?: string
  sha256: string
  sha256_ok: boolean
  encrypted: boolean
  manifest?: BackupManifest
}

/** A schedule split up for its text: backups.schedule.<kind>. */
export interface ScheduleParts {
  kind: 'daily' | 'weekly' | 'every' | 'invalid'
  time?: string
  /** weekday 0 (Sunday) – 6 */
  day?: number
  every?: string
}

const DAYS = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat']

/** Reads a schedule as the server writes it ("daily 02:00", "weekly mon 03:00", "every 6h"). */
export function scheduleParts(s: string): ScheduleParts {
  const f = s.trim().toLowerCase().split(/\s+/)
  const clock = (c: string | undefined) => (c && /^\d{1,2}:\d{2}$/.test(c) ? c.padStart(5, '0') : undefined)
  if (f[0] === 'daily' && f.length === 2 && clock(f[1])) return { kind: 'daily', time: clock(f[1]) }
  if (f[0] === 'weekly' && f.length === 3 && DAYS.includes(f[1]) && clock(f[2])) return { kind: 'weekly', day: DAYS.indexOf(f[1]), time: clock(f[2]) }
  if (f[0] === 'every' && f.length === 2 && /^(\d+h)?(\d+m)?$/.test(f[1]) && f[1] !== '') return { kind: 'every', every: f[1] }
  return { kind: 'invalid' }
}

/** Severity of the status tag. */
export function statusSeverity(s: BackupStatus): 'info' | 'success' | 'danger' {
  return s === 'running' ? 'info' : s === 'succeeded' ? 'success' : 'danger'
}

/** Whether the file of a run can be downloaded and verified. */
export function hasFile(r: BackupRun): boolean {
  return r.status !== 'running' && r.file_name !== '' && !r.pruned_at
}

/** Seconds a run took (null while it runs). */
export function runSeconds(r: BackupRun): number | null {
  if (!r.finished_at) return null
  const ms = Date.parse(r.finished_at) - Date.parse(r.started_at)
  return Number.isFinite(ms) ? Math.max(0, Math.round(ms / 1000)) : null
}

/** The first characters of a hash, for the table. */
export function shortHash(h: string, n = 12): string {
  return h.length > n ? h.slice(0, n) + '…' : h
}

/** The message id of the retention rules (backups.retention.<id>). */
export function retentionId(keepLast: number, keepDays: number): 'none' | 'last' | 'days' | 'both' {
  if (keepLast > 0 && keepDays > 0) return 'both'
  if (keepLast > 0) return 'last'
  if (keepDays > 0) return 'days'
  return 'none'
}

/** Whether the view should poll (a backup runs). */
export function isRunning(l: BackupList | null): boolean {
  return !!l && (l.running || l.runs.some((r) => r.status === 'running'))
}

/** The verification state of a run for its badge. */
export function verifyState(r: BackupRun): 'none' | 'ok' | 'failed' {
  if (!r.verified_at) return 'none'
  return r.verify_error ? 'failed' : 'ok'
}
