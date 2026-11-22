// CI/CD integration: the CI setup panel (GET /projects/{pid}/ci/setup) and
// the server-side posting of commit statuses and merge request notes to
// GitLab (PUT /projects/{pid}/ci-settings, GET …/ci/deliveries).

export interface CIVariable {
  name: string
  value?: string
  masked: boolean
  required: boolean
  description: string
}

export interface CITemplate {
  name: string
  kind: 'gitlab' | 'gitlab-selection' | 'gitlab-component' | 'github'
  /** where the file goes in the repository */
  file: string
  url?: string
  content: string
}

export interface CISetup {
  server_url: string
  project_key: string
  project_id: string
  token_role: string
  variables: CIVariable[]
  templates: CITemplate[]
}

export interface CISettings {
  provider: '' | 'gitlab'
  api_url: string
  project: string
  post_status: boolean
  post_note: boolean
  release_id: string
  conditions: string[]
}

export interface CIDelivery {
  id: string
  run_id: string
  build_id: string
  commit_sha: string
  merge_request: string
  status: 'queued' | 'sending' | 'retry' | 'sent' | 'failed'
  attempts: number
  status_code: number
  error: string
  outcome: string
  created_by: string
  created_at: string
  next_attempt_at?: string
  sent_at?: string
}

/** The editable form of the CI posting settings. */
export interface CIForm {
  enabled: boolean
  api_url: string
  project: string
  post_status: boolean
  post_note: boolean
  release_id: string
  /** comma-separated gate conditions */
  conditions: string
  token: string
}

export function ciForm(ci: Partial<CISettings> | null | undefined): CIForm {
  return {
    enabled: ci?.provider === 'gitlab',
    api_url: ci?.api_url ?? '',
    project: ci?.project ?? '',
    post_status: ci?.post_status ?? true,
    post_note: ci?.post_note ?? true,
    release_id: ci?.release_id ?? '',
    conditions: (ci?.conditions ?? []).join(', '),
    token: '',
  }
}

/** Splits comma-separated conditions (empty parts are dropped). */
export function splitConditions(s: string): string[] {
  return s
    .split(',')
    .map((x) => x.trim())
    .filter(Boolean)
}

export function ciPayload(f: CIForm, clearToken = false): { ci: CISettings; ci_token?: string; clear_ci_token?: boolean } {
  const ci: CISettings = f.enabled
    ? { provider: 'gitlab', api_url: f.api_url.trim(), project: f.project.trim(), post_status: f.post_status, post_note: f.post_note, release_id: f.release_id, conditions: splitConditions(f.conditions) }
    : { provider: '', api_url: f.api_url.trim(), project: f.project.trim(), post_status: false, post_note: false, release_id: f.release_id, conditions: splitConditions(f.conditions) }
  const out: { ci: CISettings; ci_token?: string; clear_ci_token?: boolean } = { ci }
  if (clearToken) out.clear_ci_token = true
  else if (f.token.trim()) out.ci_token = f.token.trim()
  return out
}

export type CIField = 'api_url' | 'project' | 'token'

/** Problems of the form (field → i18n key under ci.posting.invalid). */
export function validateCI(f: CIForm, hasToken: boolean): Partial<Record<CIField, string>> {
  const p: Partial<Record<CIField, string>> = {}
  if (!f.enabled) return p
  if (f.api_url.trim() && !/^https?:\/\/[^\s/]+/i.test(f.api_url.trim())) p.api_url = 'url'
  if (!f.project.trim()) p.project = 'project'
  if (!hasToken && !f.token.trim()) p.token = 'token'
  if (/\s/.test(f.token.trim())) p.token = 'tokenSpace'
  return p
}

/** PrimeVue Tag severity of a delivery status. */
export function deliverySeverity(status: CIDelivery['status']): 'success' | 'danger' | 'warn' | 'info' {
  switch (status) {
    case 'sent':
      return 'success'
    case 'failed':
      return 'danger'
    case 'retry':
      return 'warn'
  }
  return 'info'
}

/** A short commit SHA. */
export const shortSHA = (sha: string) => sha.slice(0, 8)

/** The API of gitlab.com (the default of api_url). */
export const GITLAB_DEFAULT_API = 'https://gitlab.com/api/v4'
