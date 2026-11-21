// Web tests (script language "playwright"): the starter template, the
// project settings form (base URLs, test variables), background jobs and
// the evidence files of a web test.
import type { FileInfo } from './types'

export const PLAYWRIGHT = 'playwright'

/** The starter script of a new web test (the body of one Playwright test). */
export const WEB_STARTER = `// Paths are relative to the project's base URL (Settings → Web tests).
await page.goto('/');
await expect(page).toHaveTitle(/./);
// await page.getByRole('link', { name: 'Sign in' }).click();
// await page.getByLabel('E-mail').fill(process.env.QTF_VAR_USERNAME ?? '');
// await expect(page.getByRole('heading', { level: 1 })).toHaveText('Welcome');
`

/** A complete spec file calls test(...) itself; otherwise the script is the body of one test (mirrors internal/runner). */
export function isCompleteSpec(script: string): boolean {
  return /(^|[^.\w$])test(\.describe|\.only)?\s*\(/.test(script)
}

// ---------------------------------------------------------------- jobs

export type JobState = 'queued' | 'running' | 'passed' | 'failed' | 'skipped' | 'cancelled' | 'error' | 'interrupted'

/** A background job of an automated test (GET /runs/{id}/jobs, execution.job). */
export interface ScriptJob {
  id: string
  run_id: string
  execution_id: string
  batch_id: string
  seq: number
  state: JobState | string
  after: string[]
  requested_by: string
  message: string
  created_at: string
  started_at: string | null
  finished_at: string | null
  test_key?: string
  test_title?: string
  configuration_name?: string
}

export interface JobCounts {
  queued: number
  running: number
  done: number
  passed: number
  failed: number
  skipped: number
  cancelled: number
  error: number
  total: number
}

export interface RunJobs {
  items: ScriptJob[]
  counts: JobCounts
  active: boolean
}

export interface SkippedExecution {
  execution_id: string
  test_key: string
  configuration_name?: string
  reason: string
  code: string
}

export interface EnqueueResult {
  queued: ScriptJob[]
  skipped: SkippedExecution[]
}

export const EMPTY_COUNTS: JobCounts = { queued: 0, running: 0, done: 0, passed: 0, failed: 0, skipped: 0, cancelled: 0, error: 0, total: 0 }

/** A job that is still queued or running. */
export function jobActive(job: Pick<ScriptJob, 'state'> | null | undefined): boolean {
  return !!job && (job.state === 'queued' || job.state === 'running')
}

/** Share of the current jobs that are done, 0–100. */
export function jobProgress(c: JobCounts | null | undefined): number {
  if (!c || !c.total) return 0
  return Math.round((c.done / c.total) * 100)
}

/** Delay before the next poll of a running job: quick at first, then every 5 s. */
export function pollDelay(attempt: number): number {
  return Math.min(5000, 1000 + Math.max(0, attempt) * 500)
}

/** Skipped executions per reason code, e.g. { not_ready: 2, active: 1 }. */
export function skippedByCode(list: SkippedExecution[] | null | undefined): Record<string, number> {
  const out: Record<string, number> = {}
  for (const s of list ?? []) out[s.code] = (out[s.code] ?? 0) + 1
  return out
}

/** "KEY (configuration): reason" lines of skipped executions. */
export function skippedText(list: SkippedExecution[] | null | undefined, max = 10): string {
  const all = list ?? []
  const lines = all.slice(0, max).map((s) => `${s.test_key}${s.configuration_name ? ` (${s.configuration_name})` : ''}: ${s.reason}`)
  if (all.length > max) lines.push(`… +${all.length - max}`)
  return lines.join('\n')
}

// ---------------------------------------------------------------- evidence

export const EVIDENCE_TAG = 'web-test'

export interface WebEvidence {
  screenshots: FileInfo[]
  traces: FileInfo[]
  videos: FileInfo[]
}

function kindOf(f: FileInfo): 'screenshot' | 'trace' | 'video' | '' {
  const tags = f.tags ?? []
  if (tags.includes('trace')) return 'trace'
  if (tags.includes('video')) return 'video'
  if (tags.includes('screenshot')) return 'screenshot'
  if (f.content_type.startsWith('image/')) return 'screenshot'
  if (f.content_type.startsWith('video/')) return 'video'
  if (/\.zip$/i.test(f.name)) return 'trace'
  return ''
}

/** The evidence files of a web test among an execution's attachments. */
export function webEvidence(files: FileInfo[] | null | undefined): WebEvidence {
  const out: WebEvidence = { screenshots: [], traces: [], videos: [] }
  for (const f of files ?? []) {
    if (!(f.tags ?? []).includes(EVIDENCE_TAG)) continue
    const k = kindOf(f)
    if (k === 'screenshot') out.screenshots.push(f)
    else if (k === 'trace') out.traces.push(f)
    else if (k === 'video') out.videos.push(f)
  }
  return out
}

export function evidenceCount(e: WebEvidence): number {
  return e.screenshots.length + e.traces.length + e.videos.length
}

/** The command that opens a downloaded trace. */
export function showTraceCommand(name = 'trace.zip'): string {
  return `npx playwright show-trace ${name}`
}

export const TRACE_VIEWER = 'https://trace.playwright.dev/'

// ---------------------------------------------------------------- settings

export interface TestVariable {
  name: string
  value: string
  secret: boolean
  has_value?: boolean
}

export interface EnvironmentURL {
  environment: string
  base_url: string
}

/** GET /projects/{pid}/web-tests */
export interface WebTestSettings {
  base_url: string
  environment_urls: EnvironmentURL[] | null
  variables: TestVariable[] | null
  enabled?: boolean
  browser?: string
}

export interface WebTestsForm {
  base_url: string
  environment_urls: EnvironmentURL[]
  variables: TestVariable[]
}

export function webTestsForm(s: WebTestSettings | null | undefined): WebTestsForm {
  return {
    base_url: s?.base_url ?? '',
    environment_urls: (s?.environment_urls ?? []).map((e) => ({ ...e })),
    variables: (s?.variables ?? []).map((v) => ({ name: v.name, value: v.secret ? '' : v.value, secret: v.secret, has_value: !!v.has_value })),
  }
}

/** The PUT body: rows without content are dropped; an empty secret keeps its saved value. */
export function webTestsPayload(f: WebTestsForm): WebTestsForm {
  return {
    base_url: f.base_url.trim(),
    environment_urls: f.environment_urls
      .map((e) => ({ environment: e.environment.trim(), base_url: e.base_url.trim() }))
      .filter((e) => e.environment || e.base_url),
    variables: f.variables.filter((v) => v.name.trim() || v.value).map((v) => ({ name: v.name.trim(), value: v.value, secret: v.secret })),
  }
}

export function validBaseUrl(u: string): boolean {
  if (!/^https?:\/\/\S+$/i.test(u)) return false
  try {
    return !!new URL(u).host
  } catch {
    return false
  }
}

const NAME_RE = /^[A-Za-z_][A-Za-z0-9_]{0,63}$/

export type WebTestsIssue = 'url' | 'environment' | 'duplicate' | 'name' | 'too_long'

/** Problems of the form by field: base_url, env.<i>.environment, env.<i>.base_url, var.<i>.name, var.<i>.value. */
export function validateWebTests(f: WebTestsForm): Record<string, WebTestsIssue> {
  const out: Record<string, WebTestsIssue> = {}
  if (f.base_url.trim() && !validBaseUrl(f.base_url.trim())) out.base_url = 'url'
  const envs = new Set<string>()
  f.environment_urls.forEach((e, i) => {
    const env = e.environment.trim()
    const url = e.base_url.trim()
    if (!env && !url) return
    if (!env) out[`env.${i}.environment`] = 'environment'
    else if (envs.has(env.toLowerCase())) out[`env.${i}.environment`] = 'duplicate'
    envs.add(env.toLowerCase())
    if (!validBaseUrl(url)) out[`env.${i}.base_url`] = 'url'
  })
  const names = new Set<string>()
  f.variables.forEach((v, i) => {
    const n = v.name.trim()
    if (!n && !v.value) return
    if (!NAME_RE.test(n)) out[`var.${i}.name`] = 'name'
    else if (names.has(n.toUpperCase())) out[`var.${i}.name`] = 'duplicate'
    names.add(n.toUpperCase())
    if (v.value.length > 4096) out[`var.${i}.value`] = 'too_long'
  })
  return out
}

export function webTestsDirty(f: WebTestsForm, saved: WebTestSettings | null | undefined): boolean {
  return JSON.stringify(webTestsPayload(f)) !== JSON.stringify(webTestsPayload(webTestsForm(saved)))
}

/** The environment variable a test variable arrives as, e.g. QTF_VAR_USERNAME. */
export function variableEnvName(name: string): string {
  return 'QTF_VAR_' + name.trim().toUpperCase().replace(/[^A-Z0-9]/g, '_')
}
