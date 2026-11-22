// Two-way sync of requirements with Jira and Azure DevOps
// (internal/api/reqsync.go): the types of the API, the settings form of
// TrackerSyncView and its request body, and helpers of the log and the
// conflict view. The webhook secret is write-only.
import type { ImportSourceId } from './importSources'

export type SyncDirection = 'pull' | 'push' | 'both' | 'none'
export const SYNC_DIRECTIONS: readonly SyncDirection[] = ['both', 'pull', 'push', 'none']
export type SyncPolicy = 'tracker' | 'qtestframe' | 'manual'
export const SYNC_POLICIES: readonly SyncPolicy[] = ['manual', 'tracker', 'qtestframe']
export const SYNC_IMPACTS = ['suspect', 'notify', 'skip'] as const
export const SYNC_FIELDS = ['title', 'description', 'status', 'priority', 'tags', 'criteria', 'rationale'] as const
export type SyncFieldName = (typeof SYNC_FIELDS)[number]
/** fields whose values are mapped (tracker value ↔ requirement value) */
export const MAPPED_VALUE_FIELDS: readonly string[] = ['status', 'priority']

export interface ReqSyncField {
  field: SyncFieldName
  remote: string
  direction: SyncDirection
  values?: Record<string, string>
  push_values?: Record<string, string>
}

export interface ReqSyncSettings {
  fields: ReqSyncField[]
  query: string
  import_new: boolean
  folder_id: string
  push: { create: boolean; folder_id: string; tag: string; query: string; item_type: string; project_key: string }
  conflict_policy: SyncPolicy
  impact: (typeof SYNC_IMPACTS)[number]
  comments: boolean
  links: boolean
  max_writes_per_minute: number
  comments_since?: string
}

export interface ReqSyncConfig {
  id: string
  system: ImportSourceId
  enabled: boolean
  settings: ReqSyncSettings
  interval_minutes: number
  has_webhook_secret: boolean
  webhook_url: string
  run_as: string
  next_run_at: string | null
  last_run_at: string | null
  last_status: string
  last_error: string
  failures: number
  open_conflicts: number
  linked: number
  version: number
  updated_by: string
  updated_at: string
  connected: boolean
}

export interface ReqSyncEntry {
  requirement_id?: string
  requirement_key?: string
  external_id?: string
  title?: string
  action: string
  fields?: string[]
  changes?: Record<string, string>
  message?: string
}

export interface ReqSyncSummary {
  pulled: number
  pushed: number
  created_local: number
  created_remote: number
  conflicts: number
  comments: number
  links: number
  unchanged: number
  errors: number
}

export interface ReqSyncRun {
  id: string
  system: ImportSourceId
  trigger: 'manual' | 'schedule' | 'webhook' | 'resolve'
  dry_run: boolean
  status: 'ok' | 'partial' | 'error' | 'running'
  summary: ReqSyncSummary
  entries?: ReqSyncEntry[] | null
  error?: string
  started_by: string
  started_at: string
  finished_at: string | null
}

export interface ReqSyncFieldDiff {
  field: string
  base: string
  local: string
  remote: string
}

export interface ReqSyncConflict {
  id: string
  system: ImportSourceId
  requirement_id: string
  requirement_key: string
  title: string
  external_id: string
  external_url?: string
  fields: ReqSyncFieldDiff[]
  status: 'open' | 'resolved' | 'dismissed'
  resolution?: string
  resolved_by?: string
  resolved_at?: string
  created_at: string
  updated_at: string
}

export const SYNC_SOURCE_NAMES: Record<ImportSourceId, string> = { jira: 'Jira', 'azure-devops': 'Azure DevOps' }

// ---------------------------------------------------------------- settings form

export interface SyncFieldForm {
  field: SyncFieldName
  remote: string
  direction: SyncDirection
  /** "tracker value = requirement value" lines */
  values: string
  pushValues: string
}

export interface SyncForm {
  enabled: boolean
  interval: number
  query: string
  importNew: boolean
  folderId: string
  pushCreate: boolean
  pushFolderId: string
  pushTag: string
  pushQuery: string
  itemType: string
  projectKey: string
  policy: SyncPolicy
  impact: (typeof SYNC_IMPACTS)[number]
  comments: boolean
  links: boolean
  maxWrites: number
  fields: SyncFieldForm[]
  secret: string
  version: number
}

/** "a = b" lines of a value map, sorted by key. */
export function mapText(m: Record<string, string> | null | undefined): string {
  return Object.keys(m ?? {})
    .sort((a, b) => a.localeCompare(b))
    .map((k) => `${k} = ${m![k]}`)
    .join('\n')
}

/** Parses "a = b" lines (blank lines and lines without "=" are ignored; the last "=" separates). */
export function parseMapText(s: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const line of s.split('\n')) {
    const i = line.lastIndexOf('=')
    if (i < 0) continue
    const k = line.slice(0, i).trim()
    const v = line.slice(i + 1).trim()
    if (k) out[k] = v
  }
  return out
}

export function syncForm(c: ReqSyncConfig | null | undefined): SyncForm {
  const s = c?.settings
  return {
    enabled: !!c?.enabled,
    interval: c?.interval_minutes ?? 0,
    query: s?.query ?? '',
    importNew: !!s?.import_new,
    folderId: s?.folder_id ?? '',
    pushCreate: !!s?.push?.create,
    pushFolderId: s?.push?.folder_id ?? '',
    pushTag: s?.push?.tag ?? '',
    pushQuery: s?.push?.query ?? '',
    itemType: s?.push?.item_type ?? '',
    projectKey: s?.push?.project_key ?? '',
    policy: s?.conflict_policy ?? 'manual',
    impact: s?.impact ?? 'suspect',
    comments: !!s?.comments,
    links: !!s?.links,
    maxWrites: s?.max_writes_per_minute || 60,
    fields: (s?.fields ?? []).map((f) => ({ field: f.field, remote: f.remote, direction: f.direction, values: mapText(f.values), pushValues: mapText(f.push_values) })),
    secret: '',
    version: c?.version ?? 0,
  }
}

export type SyncProblem = 'interval' | 'maxWrites' | 'pushScope' | 'remote' | 'duplicate' | 'secret'

/** Problems as key → message id (reqSync.invalid.<id>), as the server checks them. */
export function validateSync(f: SyncForm): Record<string, SyncProblem> {
  const out: Record<string, SyncProblem> = {}
  if (f.interval !== 0 && (f.interval < 5 || f.interval > 10080 || !Number.isInteger(f.interval))) out.interval = 'interval'
  if (f.maxWrites < 1 || f.maxWrites > 600) out.maxWrites = 'maxWrites'
  if (f.pushCreate && !f.pushFolderId && !f.pushTag.trim() && !f.pushQuery.trim()) out.pushScope = 'pushScope'
  const seen = new Set<string>()
  f.fields.forEach((x, i) => {
    if (!x.remote.trim()) out[`remote${i}`] = 'remote'
    if (seen.has(x.field)) out[`field${i}`] = 'duplicate'
    seen.add(x.field)
  })
  const s = f.secret.trim()
  if (s && (s.length < 16 || s.length > 200)) out.secret = 'secret'
  return out
}

/** The PUT body. */
export function syncPayload(f: SyncForm, clearSecret = false): Record<string, unknown> {
  const body: Record<string, unknown> = {
    enabled: f.enabled,
    interval_minutes: f.interval,
    version: f.version,
    settings: {
      query: f.query.trim(),
      import_new: f.importNew,
      folder_id: f.folderId,
      push: { create: f.pushCreate, folder_id: f.pushFolderId, tag: f.pushTag.trim(), query: f.pushQuery.trim(), item_type: f.itemType.trim(), project_key: f.projectKey.trim() },
      conflict_policy: f.policy,
      impact: f.impact,
      comments: f.comments,
      links: f.links,
      max_writes_per_minute: f.maxWrites,
      fields: f.fields.map((x) => {
        const out: ReqSyncField = { field: x.field, remote: x.remote.trim(), direction: x.direction }
        if (MAPPED_VALUE_FIELDS.includes(x.field)) {
          out.values = parseMapText(x.values)
          out.push_values = parseMapText(x.pushValues)
        }
        return out
      }),
    },
  }
  if (clearSecret) body.clear_webhook_secret = true
  else if (f.secret.trim()) body.webhook_secret = f.secret.trim()
  return body
}

/** A field not mapped yet, for "Add field". */
export function nextField(f: SyncForm): SyncFieldName | null {
  return SYNC_FIELDS.find((x) => !f.fields.some((y) => y.field === x)) ?? null
}

/** A random webhook secret (32 hex characters of the browser's random source). */
export function randomSecret(): string {
  const b = new Uint8Array(24)
  crypto.getRandomValues(b)
  return Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('')
}

/** The webhook URL a source is set up with: Jira Server and Data Center have no secrets, so it goes in the URL. */
export function hookAddress(c: Pick<ReqSyncConfig, 'system' | 'webhook_url'>, withSecretParam: boolean): string {
  return withSecretParam && c.system === 'jira' ? `${c.webhook_url}?secret=…` : c.webhook_url
}

// ---------------------------------------------------------------- log and conflicts

export function runSeverity(status: string): 'success' | 'warn' | 'danger' | 'info' {
  switch (status) {
    case 'ok':
      return 'success'
    case 'partial':
      return 'warn'
    case 'error':
      return 'danger'
  }
  return 'info'
}

export const ENTRY_ICONS: Record<string, string> = {
  pull: 'pi pi-arrow-down',
  push: 'pi pi-arrow-up',
  create_local: 'pi pi-download',
  create_remote: 'pi pi-upload',
  conflict: 'pi pi-exclamation-triangle',
  comment_pull: 'pi pi-comment',
  comment_push: 'pi pi-comment',
  link_pull: 'pi pi-link',
  link_push: 'pi pi-link',
  skip: 'pi pi-minus-circle',
  error: 'pi pi-times-circle',
}

/** The counts of a summary that are not zero, in display order. */
export function summaryParts(s: ReqSyncSummary | null | undefined): Array<[keyof ReqSyncSummary, number]> {
  const order: Array<keyof ReqSyncSummary> = ['pulled', 'pushed', 'created_local', 'created_remote', 'conflicts', 'comments', 'links', 'errors', 'unchanged']
  return order.filter((k) => (s?.[k] ?? 0) > 0).map((k) => [k, s![k]])
}

/** Field decisions of a conflict ready to send: those chosen, the others with the default. */
export function resolveBody(c: ReqSyncConflict, choice: Record<string, string>, all?: 'tracker' | 'qtestframe' | 'dismiss'): { resolution?: string; fields?: Record<string, string> } {
  if (all) return { resolution: all }
  const fields: Record<string, string> = {}
  for (const f of c.fields) if (choice[f.field]) fields[f.field] = choice[f.field]
  return { fields }
}

/** Whether every field of a conflict has a decision. */
export function fullyChosen(c: ReqSyncConflict, choice: Record<string, string>): boolean {
  return c.fields.length > 0 && c.fields.every((f) => !!choice[f.field])
}
