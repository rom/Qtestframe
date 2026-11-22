// Defects (PROJ-BUG-n) and the project's connection to Jira, GitHub Issues,
// GitLab or Request Tracker: status and severity order, links, tracker form checks and the
// errors the tracker reports. Mirrors internal/model and internal/store.
import { ApiError } from './api'
import type { Defect, DefectLink, DefectTracker, Execution } from './types'

export const DEFECT_STATUSES = ['open', 'in_progress', 'resolved', 'closed', 'rejected'] as const
export const DEFECT_OPEN_STATUSES = ['open', 'in_progress'] as const
/** most to least severe */
export const DEFECT_SEVERITIES = ['critical', 'major', 'minor', 'trivial'] as const
export const DEFECT_PRIORITIES = ['critical', 'high', 'medium', 'low'] as const
/** the items a defect can be linked to from the UI */
export const DEFECT_LINK_TYPES = ['requirement', 'test', 'execution'] as const
export type DefectLinkType = (typeof DEFECT_LINK_TYPES)[number]
export const TRACKER_TYPES = ['', 'jira', 'github', 'gitlab', 'rt'] as const
export type TrackerType = (typeof TRACKER_TYPES)[number]

export const GITHUB_API_URL = 'https://api.github.com'
export const GITLAB_URL = 'https://gitlab.com'
/** The label mapping of issues from requirements when none is set (internal/model). */
export const DEFAULT_REQUIREMENT_LABELS = 'requirement, type::{type}, priority::{priority}, {tags}'

/** Whether the status counts as open (exit criterion no_open_defects). */
export function isOpenDefect(status: string | null | undefined): boolean {
  return (DEFECT_OPEN_STATUSES as readonly string[]).includes(status ?? '')
}

function rank(list: readonly string[], value: string | null | undefined): number {
  const i = list.indexOf(value ?? '')
  return i < 0 ? list.length : i
}
/** 0 for open … 4 for rejected, unknown last */
export const statusRank = (s: string | null | undefined) => rank(DEFECT_STATUSES, s)
/** 0 for critical … 3 for trivial, unknown last */
export const severityRank = (s: string | null | undefined) => rank(DEFECT_SEVERITIES, s)

/** The number of a key such as SHOP-BUG-12 (0 when there is none). */
export function keyNumber(key: string | null | undefined): number {
  const m = /-(\d+)$/.exec(key ?? '')
  return m ? Number(m[1]) : 0
}

type Sortable = Pick<Defect, 'key' | 'status' | 'severity'>
/** Open defects first, then the most severe, then the newest key. */
export function compareDefects(a: Sortable, b: Sortable): number {
  return Number(!isOpenDefect(a.status)) - Number(!isOpenDefect(b.status)) || severityRank(a.severity) - severityRank(b.severity) || keyNumber(b.key) - keyNumber(a.key)
}
export function sortDefects<T extends Sortable>(list: readonly T[]): T[] {
  return [...list].sort(compareDefects)
}

/** PrimeVue tag severity of a defect status. */
export function statusTag(status: string | null | undefined): 'danger' | 'warn' | 'success' | 'secondary' | 'info' {
  switch (status) {
    case 'open':
      return 'danger'
    case 'in_progress':
      return 'warn'
    case 'resolved':
      return 'info'
    case 'closed':
      return 'success'
    default:
      return 'secondary'
  }
}
/** PrimeVue tag severity of a defect severity. */
export function severityTag(severity: string | null | undefined): 'danger' | 'warn' | 'info' | 'secondary' {
  switch (severity) {
    case 'critical':
      return 'danger'
    case 'major':
      return 'warn'
    case 'minor':
      return 'info'
    default:
      return 'secondary'
  }
}

/** Open defects of a list (count for badges). */
export const countOpen = (list: readonly Pick<Defect, 'status'>[]) => list.filter((d) => isOpenDefect(d.status)).length

// ---------- keys ----------

/** A defect key of this tool: SHOP-BUG-12. */
export const DEFECT_KEY_RE = /^[A-Z][A-Z0-9]{1,15}-BUG-\d+$/
export function isDefectKey(value: string | null | undefined): boolean {
  return DEFECT_KEY_RE.test((value ?? '').trim().toUpperCase())
}
/** The defect a free-text reference (legacy execution defects) names, if any. */
export function matchDefect<T extends Pick<Defect, 'key'>>(value: string | null | undefined, defects: readonly T[]): T | undefined {
  const v = (value ?? '').trim().toUpperCase()
  if (!v) return undefined
  return defects.find((d) => d.key.toUpperCase() === v || ((d as Partial<Defect>).external_key ?? '').toUpperCase() === v)
}

// ---------- links ----------

export const sameLink = (a: Pick<DefectLink, 'entity_type' | 'entity_id'>, b: Pick<DefectLink, 'entity_type' | 'entity_id'>) =>
  a.entity_type === b.entity_type && a.entity_id === b.entity_id

/** Add links that are not there yet (keeps order). */
export function mergeLinks(list: readonly DefectLink[], add: readonly DefectLink[]): DefectLink[] {
  const out = [...list]
  for (const l of add) if (!out.some((x) => sameLink(x, l))) out.push(l)
  return out
}

/** The links to add and to remove to get from `before` to `after`. */
export function diffLinks(before: readonly DefectLink[], after: readonly DefectLink[]): { add: DefectLink[]; remove: DefectLink[] } {
  return {
    add: after.filter((l) => !before.some((x) => sameLink(x, l))),
    remove: before.filter((l) => !after.some((x) => sameLink(x, l))),
  }
}

/** Only what the API needs of a link. */
export const linkBody = (l: DefectLink) => ({ entity_type: l.entity_type, entity_id: l.entity_id })

/** Route to the linked item (executions open in the runner, which needs the run). */
export function linkRoute(l: DefectLink, projectId: string, runId?: string): Record<string, unknown> | null {
  switch (l.entity_type) {
    case 'requirement':
      return { name: 'requirements', params: { projectId }, query: { item: l.entity_id } }
    case 'test':
      return { name: 'tests', params: { projectId }, query: { item: l.entity_id } }
    case 'run':
      return { name: 'run', params: { projectId, runId: l.entity_id } }
    case 'execution':
      return runId ? { name: 'execute', params: { projectId, runId, executionId: l.entity_id } } : null
    default:
      return null
  }
}

/** Group links by type in a fixed order (requirement, test, execution, run). */
export function groupLinks(links: readonly DefectLink[] | null | undefined): { type: string; links: DefectLink[] }[] {
  const order = ['requirement', 'test', 'execution', 'run']
  const out: { type: string; links: DefectLink[] }[] = []
  for (const type of order) {
    const ls = (links ?? []).filter((l) => l.entity_type === type)
    if (ls.length) out.push({ type, links: ls })
  }
  return out
}

/**
 * How a defect reaches an item (LinkedDefect.via): "direct" when linked to
 * the item itself, else the kind of item in between (test, execution).
 */
export function viaKind(via: string | null | undefined, entityType: string): 'direct' | 'test' | 'execution' {
  if (!via || via === entityType) return 'direct'
  return via === 'execution' ? 'execution' : 'test'
}

// ---------- reporting from an execution ----------

export interface DefectDraft {
  title: string
  description: string
  severity: string
  links: DefectLink[]
}

/**
 * A defect prefilled from a failed execution: the test's key and title, the
 * failed steps with their actual results, the run's notes and a link to the
 * execution.
 */
export function draftFromExecution(
  ex: Pick<Execution, 'id' | 'test_key' | 'test_title'>,
  opts: { notes?: string; runName?: string; failedSteps?: { n: number; action: string; expected: string; actual: string }[]; labels?: { steps: string; expected: string; actual: string; notes: string; run: string } } = {},
): DefectDraft {
  const l = opts.labels ?? { steps: 'Failed steps', expected: 'Expected', actual: 'Actual', notes: 'Notes', run: 'Run' }
  const parts: string[] = []
  if (opts.runName) parts.push(`${l.run}: ${opts.runName}`)
  const steps = opts.failedSteps ?? []
  if (steps.length) {
    parts.push(
      `${l.steps}:\n` +
        steps
          .map((s) => {
            const lines = [`${s.n}. ${s.action}`.trim()]
            if (s.expected) lines.push(`   ${l.expected}: ${s.expected}`)
            if (s.actual) lines.push(`   ${l.actual}: ${s.actual}`)
            return lines.join('\n')
          })
          .join('\n'),
    )
  }
  if (opts.notes?.trim()) parts.push(`${l.notes}:\n${opts.notes.trim()}`)
  const title = [ex.test_key, ex.test_title].filter(Boolean).join(' ').slice(0, 300)
  return { title, description: parts.join('\n\n'), severity: 'major', links: [{ entity_type: 'execution', entity_id: ex.id, key: ex.test_key, title: ex.test_title }] }
}

// ---------- tracker settings ----------

export interface TrackerForm {
  type: string
  url: string
  project_key: string
  repo: string
  /** GitLab project ID or path */
  project: string
  issue_type: string
  labels: string
  milestone: string
  assignee: string
  requirement_labels: string
  requirement_issue_type: string
  /** RT queue (name or ID) */
  queue: string
  /** RT custom field that receives the labels */
  labels_field: string
  comment_on_pass: boolean
  user: string
  /** a new token; empty keeps the saved one */
  token: string
}

export function trackerForm(t: Partial<DefectTracker> | null | undefined): TrackerForm {
  return {
    type: t?.type ?? '',
    url: t?.url ?? '',
    project_key: t?.project_key ?? '',
    repo: t?.repo ?? '',
    project: t?.project ?? '',
    issue_type: t?.issue_type ?? '',
    labels: t?.labels ?? '',
    milestone: t?.milestone ?? '',
    assignee: t?.assignee ?? '',
    requirement_labels: t?.requirement_labels ?? '',
    requirement_issue_type: t?.requirement_issue_type ?? '',
    queue: t?.queue ?? '',
    labels_field: t?.labels_field ?? '',
    comment_on_pass: !!t?.comment_on_pass,
    user: t?.user ?? '',
    token: '',
  }
}

/** A GitLab project: a numeric ID or a path group/[subgroup/…]project (as the server checks it). */
export function validGitLabProject(p: string): boolean {
  const v = p.trim().replace(/^\/+|\/+$/g, '')
  if (/^\d+$/.test(v)) return true
  const segs = v.split('/')
  return segs.length >= 2 && segs.every((s) => /^[A-Za-z0-9_.][A-Za-z0-9_.-]*$/.test(s) && s.replace(/\./g, '') !== '')
}

export type TrackerField = 'type' | 'url' | 'project_key' | 'repo' | 'project' | 'queue'
/** Problems of a tracker form as field → message id (tracker.invalid.<id>), as the server checks them. */
export function validateTracker(f: TrackerForm): Partial<Record<TrackerField, string>> {
  const out: Partial<Record<TrackerField, string>> = {}
  const url = f.url.trim()
  const httpUrl = /^https?:\/\/[^\s/]+/i.test(url)
  switch (f.type) {
    case '':
      break
    case 'jira':
      if (!httpUrl) out.url = 'jiraUrl'
      if (!/^[A-Z][A-Z0-9_]{0,19}$/.test(f.project_key.trim().toUpperCase())) out.project_key = 'projectKey'
      break
    case 'github':
      if (url && !httpUrl) out.url = 'githubUrl'
      if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(f.repo.trim())) out.repo = 'repo'
      break
    case 'gitlab': {
      if (url && !/^https:\/\/[^\s/]+/i.test(url)) out.url = 'gitlabUrl'
      // a pasted project address is reduced to the path, as on the server
      const base = (url || GITLAB_URL).replace(/\/+$/, '').replace(/\/api\/v4$/, '')
      const p = f.project.trim().startsWith(base + '/') ? f.project.trim().slice(base.length + 1) : f.project
      if (!validGitLabProject(p)) out.project = 'gitlabProject'
      break
    }
    case 'rt':
      if (!httpUrl) out.url = 'rtUrl'
      if (!f.queue.trim() || /[/\\?#]/.test(f.queue)) out.queue = 'queue'
      break
    default:
      out.type = 'type'
  }
  return out
}

/** The PUT body: only the fields of the chosen type; a token only when one was typed. */
export function trackerPayload(f: TrackerForm, clearToken = false): Record<string, unknown> {
  if (!f.type) return { type: '' }
  const body: Record<string, unknown> = { type: f.type, url: f.url.trim().replace(/\/+$/, '') }
  if (f.type === 'rt') {
    Object.assign(body, { url: rtBase(f.url), queue: f.queue.trim(), labels_field: f.labels_field.trim(), labels: f.labels.trim() })
  } else if (f.type === 'jira') {
    Object.assign(body, { project_key: f.project_key.trim().toUpperCase(), issue_type: f.issue_type.trim(), user: f.user.trim(), requirement_issue_type: f.requirement_issue_type.trim() })
  } else {
    if (f.type === 'gitlab') body.project = f.project.trim()
    else body.repo = f.repo.trim()
    Object.assign(body, { labels: f.labels.trim(), milestone: f.milestone.trim(), assignee: f.assignee.trim() })
  }
  Object.assign(body, { requirement_labels: f.requirement_labels.trim(), comment_on_pass: f.comment_on_pass })
  if (f.token.trim()) body.token = f.token.trim()
  else if (clearToken) body.clear_token = true
  return body
}

/** An RT address without /REST/2.0 and trailing slashes (as the server stores it). */
export function rtBase(url: string): string {
  return url.trim().replace(/\/+$/, '').replace(/\/REST\/2\.0$/, '').replace(/\/+$/, '')
}

/** Whether the form differs from the saved settings (a typed token counts). */
export function trackerDirty(f: TrackerForm, saved: Partial<DefectTracker> | null | undefined): boolean {
  return JSON.stringify(trackerPayload(f)) !== JSON.stringify(trackerPayload(trackerForm(saved))) || !!f.token.trim()
}

/** The tracker's product name (GitLab, GitHub, Jira; "" for none). */
export function trackerName(type: string | null | undefined): string {
  return TRACKER_NAMES[type ?? ''] ?? ''
}
const TRACKER_NAMES: Record<string, string> = { jira: 'Jira', github: 'GitHub', gitlab: 'GitLab', rt: 'RT' }

/** Example of an issue key for the import dialog. */
export function issueKeyExample(t: Partial<DefectTracker> | null | undefined): string {
  if (t?.type === 'jira') return `${t.project_key || 'SHOP'}-123`
  if (t?.type === 'github') return `${t.repo || 'owner/repo'}#123`
  if (t?.type === 'gitlab') return `${t.project && !/^\d+$/.test(t.project) ? t.project : 'group/project'}#123`
  if (t?.type === 'rt') return '#123'
  return ''
}

/** Whether a key looks like an issue of the tracker (before asking it). */
export function validIssueKey(t: Partial<DefectTracker> | null | undefined, key: string): boolean {
  const k = key.trim()
  if (t?.type === 'jira') return /^[A-Z][A-Z0-9_]*-\d+$/i.test(k)
  if (t?.type === 'github') return /^(?:[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+)?#?\d+$/.test(k)
  if (t?.type === 'gitlab') return /^(?:[A-Za-z0-9_.-]+(?:\/[A-Za-z0-9_.-]+)+)?#?\d+$/.test(k)
  if (t?.type === 'rt') return /^(?:rt\s*)?#?\d+$/i.test(k) && !/^(?:rt\s*)?#?0+$/i.test(k)
  return false
}

/** The issue key as the server stores it (SHOP-12, owner/repo#12, group/project#12), for comparing. */
export function normalizeIssueKey(t: Partial<DefectTracker> | null | undefined, key: string): string {
  const k = key.trim()
  if (t?.type === 'rt') {
    const m = /^(?:rt\s*)?#?(\d+)$/i.exec(k)
    return m ? `#${Number(m[1])}` : k.toLowerCase()
  }
  if (t?.type === 'github' || t?.type === 'gitlab') {
    const own = t.type === 'github' ? t.repo : t.project
    const m = /^(?:([A-Za-z0-9_.-]+(?:\/[A-Za-z0-9_.-]+)+))?#?(\d+)$/.exec(k)
    return m ? `${(m[1] || own || '').toLowerCase()}#${m[2]}` : k.toLowerCase()
  }
  return k.toUpperCase()
}

/** The defect that already mirrors an issue (to avoid importing it twice). */
export function findImported<T extends Pick<Defect, 'external_key'>>(defects: readonly T[], t: Partial<DefectTracker> | null | undefined, key: string): T | undefined {
  const want = normalizeIssueKey(t, key)
  return defects.find((d) => !!d.external_key && normalizeIssueKey(t, d.external_key) === want)
}

/** The tracker's own message of a 502 tracker_error (null for other errors). */
export function trackerError(e: unknown): string | null {
  return e instanceof ApiError && e.status === 502 && e.code === 'tracker_error' ? e.message : null
}

export interface SyncResult {
  synced: number
  changed: number
  failed: { key: string; error: string }[]
}
/** Normalise the answer of POST …/defects/sync. */
export function syncResult(res: Partial<SyncResult> | null | undefined): SyncResult {
  return { synced: res?.synced ?? 0, changed: res?.changed ?? 0, failed: res?.failed ?? [] }
}

/** Whether the defect can be pushed (no issue yet) or synced (has one). */
export const canPush = (d: Pick<Defect, 'external_key'>) => !d.external_key
export const canSync = (d: Pick<Defect, 'external_key' | 'external_system'>, trackerType: string) => !!d.external_key && !!trackerType && (!d.external_system || d.external_system === trackerType)
