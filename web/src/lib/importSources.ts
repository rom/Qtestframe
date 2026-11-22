// The connections requirements are imported from: Jira (its own, or the
// defect tracker's when that is Jira) and Azure DevOps. Form helpers of
// ImportSourcesSettings and the request bodies of the import dialog
// (internal/api/reqsources.go). Tokens are write-only.
import type { ImportSources } from './types'

export type ImportSourceId = 'jira' | 'azure-devops'
export const IMPORT_SOURCES: readonly ImportSourceId[] = ['jira', 'azure-devops']

export interface ImportSourcesForm {
  jiraUrl: string
  jiraUser: string
  jiraJql: string
  jiraToken: string
  azureUrl: string
  azureProject: string
  azureAreaPath: string
  azureTypes: string
  azureToken: string
}

export function importSourcesForm(s: Partial<ImportSources> | null | undefined): ImportSourcesForm {
  return {
    jiraUrl: s?.jira?.url ?? '',
    jiraUser: s?.jira?.user ?? '',
    jiraJql: s?.jira?.jql ?? '',
    jiraToken: '',
    azureUrl: s?.azure_devops?.url ?? '',
    azureProject: s?.azure_devops?.project ?? '',
    azureAreaPath: s?.azure_devops?.area_path ?? '',
    azureTypes: s?.azure_devops?.work_item_types ?? '',
    azureToken: '',
  }
}

export type ImportSourcesField = 'jiraUrl' | 'azureUrl' | 'azureProject'

const httpUrl = (u: string) => /^https?:\/\/[^\s/?#]+[^\s?#]*$/i.test(u.trim())

/** Problems as field → message id (importSources.invalid.<id>), as the server checks them. */
export function validateImportSources(f: ImportSourcesForm): Partial<Record<ImportSourcesField, string>> {
  const out: Partial<Record<ImportSourcesField, string>> = {}
  if (f.jiraUrl.trim() && !httpUrl(f.jiraUrl)) out.jiraUrl = 'jiraUrl'
  if (f.azureUrl.trim() && !httpUrl(f.azureUrl)) out.azureUrl = 'azureUrl'
  if (f.azureUrl.trim() && (!f.azureProject.trim() || /[/\\?#]/.test(f.azureProject.trim().replace(/^\/+|\/+$/g, '')))) out.azureProject = 'azureProject'
  if (!f.azureUrl.trim() && f.azureProject.trim()) out.azureUrl = 'azureUrlRequired'
  return out
}

/** The PUT body; tokens only when typed (or cleared). */
export function importSourcesPayload(f: ImportSourcesForm, clear: { jira?: boolean; azure?: boolean } = {}): Record<string, unknown> {
  const trim = (s: string) => s.trim().replace(/\/+$/, '')
  const jira: Record<string, unknown> = { url: trim(f.jiraUrl), user: f.jiraUser.trim(), jql: f.jiraJql.trim() }
  const azure: Record<string, unknown> = { url: trim(f.azureUrl), project: f.azureProject.trim(), area_path: f.azureAreaPath.trim(), work_item_types: f.azureTypes.trim() }
  if (f.jiraToken.trim()) jira.token = f.jiraToken.trim()
  if (f.azureToken.trim()) azure.token = f.azureToken.trim()
  const body: Record<string, unknown> = { jira, azure_devops: azure }
  if (clear.jira || clear.azure) body.clear_tokens = { jira: !!clear.jira, azure_devops: !!clear.azure }
  return body
}

/** Whether the form differs from the saved settings (a typed token counts). */
export function importSourcesDirty(f: ImportSourcesForm, saved: Partial<ImportSources> | null | undefined): boolean {
  return JSON.stringify(importSourcesPayload(f)) !== JSON.stringify(importSourcesPayload(importSourcesForm(saved))) || !!f.jiraToken.trim() || !!f.azureToken.trim()
}

/** Whether a source can be used for an import. */
export function sourceConfigured(s: Partial<ImportSources> | null | undefined, id: ImportSourceId): boolean {
  if (!s) return false
  if (id === 'jira') return !!s.jira?.url || !!s.jira_from_tracker
  return !!s.azure_devops?.url && !!s.azure_devops?.project
}

/** Work item types typed as "User Story, Requirement". */
export function splitTypes(s: string): string[] {
  return s
    .split(',')
    .map((x) => x.trim())
    .filter(Boolean)
}

export interface ExternalQuery {
  /** JQL or WIQL; empty uses the simple form (Azure DevOps) or the saved JQL (Jira) */
  query: string
  /** Azure DevOps simple form */
  areaPath?: string
  types?: string
  relations: boolean
}

/** JSON body of POST …/requirements/import/{source}/preview and …/apply. */
export function externalImportBody(
  source: ImportSourceId,
  q: ExternalQuery,
  opts: { mapping?: Record<string, string>; folderId?: string | null },
  apply?: { overwrite: boolean; create: boolean; excludedRefs: Iterable<string> },
): Record<string, unknown> {
  const body: Record<string, unknown> = { relations: q.relations }
  const query = q.query.trim()
  if (query) body.query = query
  else if (source === 'azure-devops') {
    // the dialog starts with the saved defaults: empty fields mean all
    body.area_path = (q.areaPath ?? '').trim()
    body.work_item_types = splitTypes(q.types ?? '')
  }
  if (opts.mapping) body.mapping = opts.mapping
  if (opts.folderId) body.folder_id = opts.folderId
  if (apply) {
    body.overwrite_conflicts = apply.overwrite
    body.create = apply.create
    const ex = [...apply.excludedRefs].sort()
    if (ex.length) body.exclude_refs = ex
  }
  return body
}
