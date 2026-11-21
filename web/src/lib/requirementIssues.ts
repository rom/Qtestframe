// Issues created from requirements in the project's tracker (GitLab,
// GitHub or Jira): GET/POST/PUT /requirements/{id}/issue, …/issue/sync and
// the bulk endpoints /projects/{pid}/requirements/issues[/sync]. The issue
// is an external item link of the requirement that carries `issue`.
import { ApiError } from './api'
import type { ItemLink } from './types'

/** The requirement's link to an issue of the tracker type (the first one). */
export function issueLinkOf(links: readonly ItemLink[] | null | undefined, tracker: string): ItemLink | undefined {
  return (links ?? []).find((l) => l.direction !== 'incoming' && !!l.issue && l.issue.tracker === tracker)
}

/** PrimeVue tag severity of an issue state: open issues are work in progress. */
export function issueStateTag(state: string | null | undefined): 'info' | 'success' | 'secondary' {
  if (state === 'open') return 'info'
  if (state === 'closed') return 'success'
  return 'secondary'
}

/** The existing link of a 409 issue_exists answer to POST …/issue (null otherwise). */
export function existingIssue(e: unknown): ItemLink | null {
  if (!(e instanceof ApiError) || e.status !== 409 || e.code !== 'issue_exists') return null
  const b = e.body as { link?: ItemLink } | null | undefined
  return b?.link ?? null
}

export interface IssueResult {
  key: string
  issue_key: string
  url: string
  state?: string
}
export interface BulkIssueResult {
  created: IssueResult[]
  updated: IssueResult[]
  skipped: IssueResult[]
  failed: { key: string; error: string }[]
}
/** Normalise the answer of POST /projects/{pid}/requirements/issues. */
export function bulkIssueResult(res: Partial<BulkIssueResult> | null | undefined): BulkIssueResult {
  return { created: res?.created ?? [], updated: res?.updated ?? [], skipped: res?.skipped ?? [], failed: res?.failed ?? [] }
}

export interface IssueSyncResult {
  synced: number
  changed: number
  commented: number
  failed: { key: string; error: string }[]
}
/** Normalise the answer of POST /projects/{pid}/requirements/issues/sync. */
export function issueSyncResult(res: Partial<IssueSyncResult> | null | undefined): IssueSyncResult {
  return { synced: res?.synced ?? 0, changed: res?.changed ?? 0, commented: res?.commented ?? 0, failed: res?.failed ?? [] }
}

/** The labels a template list adds (for the settings preview), as the server maps them. */
export function previewLabels(tmpl: string, req: { type?: string; priority?: string; obligation?: string; status?: string; tags?: string[] }, fallback: string): string[] {
  let t = tmpl.trim()
  if (!t) t = fallback
  if (t.toLowerCase() === 'none') return []
  const clean = (v: string | undefined) => (v ?? '').replace(/,/g, ' ').trim()
  const vals: Record<string, string> = { '{type}': clean(req.type), '{priority}': clean(req.priority), '{obligation}': clean(req.obligation), '{status}': clean(req.status) }
  const expand = (item: string) => {
    for (const [ph, v] of Object.entries(vals)) {
      if (item.includes(ph)) {
        if (!v) return ''
        item = item.split(ph).join(v)
      }
    }
    return item.trim()
  }
  const out: string[] = []
  const add = (l: string) => {
    if (l && !out.some((x) => x.toLowerCase() === l.toLowerCase())) out.push(l)
  }
  for (const raw of t.split(',')) {
    const item = raw.trim()
    if (item.includes('{tags}')) {
      for (const tag of req.tags ?? []) if (clean(tag)) add(expand(item.split('{tags}').join(clean(tag))))
      continue
    }
    add(expand(item))
  }
  return out
}
