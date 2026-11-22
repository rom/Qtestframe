// Optional AI assistance (GET /projects/{pid}/ai, POST /projects/{pid}/ai/<feature>):
// the availability, the streamed requests (server-sent events, cancellable)
// and the pure helpers of the review dialogs. Suggestions are only shown;
// what the user accepts is saved through the regular endpoints.
import { ApiError, buildUrl, getCsrfToken, handleErrorResponse, statusCode } from './api'
import type { LanguageIssue } from './types'

export type AiFeature = 'draft_tests' | 'improve_requirement' | 'summarize_run' | 'suggest_mappings' | 'suggest_criteria'

export const AI_FEATURES: readonly AiFeature[] = ['draft_tests', 'improve_requirement', 'summarize_run', 'suggest_mappings', 'suggest_criteria']

/** The endpoint of a feature below /projects/{pid}/ai/. */
export const AI_ENDPOINT: Record<AiFeature, string> = {
  draft_tests: 'draft-tests',
  improve_requirement: 'improve-requirement',
  summarize_run: 'summarize-run',
  suggest_mappings: 'suggest-mappings',
  suggest_criteria: 'suggest-criteria',
}

export type AiReason = 'off' | 'not_configured' | 'organisation' | 'project'

export interface AiStatus {
  available: boolean
  reason?: AiReason
  provider?: 'anthropic' | 'openai'
  model?: string
  features: Partial<Record<AiFeature, boolean>>
  system_enabled: boolean
  organisation_enabled: boolean
  project_disabled?: boolean
  rate_limit: number
  remaining: number
  price_input: number
  price_output: number
  prompt_version: string
}

export interface AiMeta {
  feature: AiFeature
  prompt_version: string
  provider: string
  model: string
  usage: { input_tokens: number; output_tokens: number }
  retried: boolean
  duration_ms: number
  cost: number
}

export interface AiAnswer<T> {
  meta: AiMeta
  result: T
}

// ---------------------------------------------------------------- results

export interface DraftStep {
  action: string
  data: string
  expected: string
}

export interface DraftTest {
  title: string
  description: string
  preconditions: string
  test_data: string
  expected_result: string
  steps: DraftStep[]
  criteria: number[]
  criterion_ids: string[]
}

export interface DraftTestsResult {
  requirement_id: string
  requirement_key: string
  tests: DraftTest[]
}

export interface LanguageAnalysis {
  language: string
  obligation: string
  issues: LanguageIssue[]
  score: number
}

export interface ImproveResult {
  original: { title: string; description: string }
  title: string
  description: string
  explanation: string
  before: LanguageAnalysis
  after: LanguageAnalysis
  fixed: string[]
  new: string[]
  claimed: string[]
}

export interface DraftTestRef {
  key: string
  title: string
  test_case_id: string
  execution_id: string
}

export interface RunSummaryResult {
  run_id: string
  summary: string
  causes: { title: string; explanation: string; tests: string[] }[]
  defects: { title: string; description: string; severity: string; tests: DraftTestRef[] }[]
  failed: number
}

export type MappingFramework = 'attack' | 'd3fend' | 'emb3d' | 'cwe'

export interface SuggestedMapping {
  framework: MappingFramework
  id: string
  name: string
  reason: string
}

export interface MappingsResult {
  kind: 'requirement' | 'risk'
  mappings: SuggestedMapping[]
  dropped: { framework: string; id: string }[]
}

export interface SuggestedCriterion {
  description: string
  method: string
  threshold?: string
}

export interface CriteriaResult {
  criteria: SuggestedCriterion[]
}

// ---------------------------------------------------------------- status

const statusCache = new Map<string, { at: number; value: Promise<AiStatus> }>()
const STATUS_TTL = 30_000

/** The availability in a project (cached for a short time; force reloads). */
export function aiStatus(projectId: string, fetcher: (path: string) => Promise<AiStatus>, force = false): Promise<AiStatus> {
  const hit = statusCache.get(projectId)
  if (!force && hit && Date.now() - hit.at < STATUS_TTL) return hit.value
  const value = fetcher(`/projects/${projectId}/ai`)
  value.catch(() => statusCache.delete(projectId))
  statusCache.set(projectId, { at: Date.now(), value })
  return value
}

/** Forget the cached availability (after a settings change). */
export function clearAiStatus(projectId?: string): void {
  if (projectId) statusCache.delete(projectId)
  else statusCache.clear()
}

/** Whether a feature can be used, and the message id that explains why not. */
export function featureState(s: AiStatus | null | undefined, f: AiFeature): { ok: boolean; reason: string } {
  if (!s) return { ok: false, reason: 'ai.reason.loading' }
  if (!s.available) return { ok: false, reason: `ai.reason.${s.reason ?? 'off'}` }
  if (s.features[f] === false) return { ok: false, reason: 'ai.reason.feature_off' }
  return { ok: true, reason: '' }
}

// ---------------------------------------------------------------- server-sent events

export interface SseEvent {
  event: string
  data: string
}

/**
 * Splits a server-sent event stream into events. Returns the complete
 * events of buf and the rest (an incomplete event) to keep for the next
 * chunk.
 */
export function parseSse(buf: string): { events: SseEvent[]; rest: string } {
  const events: SseEvent[] = []
  const text = buf.replace(/\r\n/g, '\n')
  const blocks = text.split('\n\n')
  const rest = blocks.pop() ?? ''
  for (const block of blocks) {
    let event = 'message'
    const data: string[] = []
    for (const line of block.split('\n')) {
      if (line.startsWith(':')) continue
      if (line.startsWith('event:')) event = line.slice(6).trim()
      else if (line.startsWith('data:')) data.push(line.slice(5).replace(/^ /, ''))
    }
    if (data.length) events.push({ event, data: data.join('\n') })
  }
  return { events, rest }
}

export interface AssistOptions {
  signal?: AbortSignal
  /** characters written by the service so far */
  onProgress?: (chars: number) => void
}

/**
 * Runs a feature request, streamed: resolves with the answer, rejects with
 * an ApiError (code ai_unavailable, rate_limited, ai_refused, ai_service,
 * ai_invalid_answer, ai_timeout, …) or an AbortError when cancelled.
 */
export async function runAssist<T>(projectId: string, feature: AiFeature, body: Record<string, unknown>, opts: AssistOptions = {}): Promise<AiAnswer<T>> {
  const headers: Record<string, string> = { Accept: 'text/event-stream', 'Content-Type': 'application/json' }
  const csrf = getCsrfToken()
  if (csrf) headers['X-CSRF-Token'] = csrf
  const res = await fetch(buildUrl(`/projects/${projectId}/ai/${AI_ENDPOINT[feature]}`), {
    method: 'POST',
    headers,
    credentials: 'same-origin',
    body: JSON.stringify(body),
    signal: opts.signal,
  })
  if (!res.ok) {
    let code = statusCode(res.status)
    let message = res.statusText || `HTTP ${res.status}`
    let parsed: unknown
    try {
      parsed = await res.json()
      const b = parsed as { error?: string; code?: string }
      if (b.error) message = b.error
      if (b.code) code = b.code
    } catch {
      /* not JSON */
    }
    const err = new ApiError(res.status, code, message, undefined, parsed)
    handleErrorResponse(err)
    throw err
  }
  const type = res.headers.get('Content-Type') ?? ''
  if (!type.includes('text/event-stream') || !res.body) return (await res.json()) as AiAnswer<T>
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buf = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (value) buf += decoder.decode(value, { stream: true })
    const { events, rest } = parseSse(done ? buf + '\n\n' : buf)
    buf = rest
    for (const ev of events) {
      const data = JSON.parse(ev.data) as Record<string, unknown>
      if (ev.event === 'progress') opts.onProgress?.(Number(data.chars) || 0)
      else if (ev.event === 'result') return data as unknown as AiAnswer<T>
      else if (ev.event === 'error') throw new ApiError(Number(data.status) || 502, String(data.code ?? 'ai_service'), String(data.error ?? ''), undefined, data)
    }
    if (done) break
  }
  throw new ApiError(502, 'ai_service', 'the answer ended early')
}

// ---------------------------------------------------------------- helpers

/** "1,234 → 567 tokens" style numbers and the estimated cost ("" none). */
export function formatCost(cost: number, locale: string): string {
  if (!cost) return ''
  const digits = cost < 0.01 ? 4 : 2
  return new Intl.NumberFormat(locale, { minimumFractionDigits: digits, maximumFractionDigits: digits }).format(cost)
}

/**
 * The issues of before that the rewrite fixed (their rule is in fixed, as a
 * multiset), and the ones that remain.
 */
export function splitFixed(before: readonly LanguageIssue[], fixed: readonly string[]): { fixed: LanguageIssue[]; remaining: LanguageIssue[] } {
  const left = new Map<string, number>()
  for (const r of fixed) left.set(r, (left.get(r) ?? 0) + 1)
  const out = { fixed: [] as LanguageIssue[], remaining: [] as LanguageIssue[] }
  // the last issues of a rule are the fixed ones (the server reports counts)
  const total = new Map<string, number>()
  for (const i of before) total.set(i.rule, (total.get(i.rule) ?? 0) + 1)
  const seen = new Map<string, number>()
  for (const i of before) {
    const n = (seen.get(i.rule) ?? 0) + 1
    seen.set(i.rule, n)
    const keep = (total.get(i.rule) ?? 0) - (left.get(i.rule) ?? 0)
    if (n > keep) out.fixed.push(i)
    else out.remaining.push(i)
  }
  return out
}

/** The preconditions of a drafted test, with its test data appended. */
export function draftPreconditions(t: Pick<DraftTest, 'preconditions' | 'test_data'>, dataLabel: string): string {
  const pre = t.preconditions.trim()
  const data = t.test_data.trim()
  if (!data) return pre
  return pre ? `${pre}\n\n${dataLabel}: ${data}` : `${dataLabel}: ${data}`
}

/** The body of POST /projects/{pid}/tests for a drafted test (a manual draft). */
export function draftTestBody(t: DraftTest, dataLabel: string): Record<string, unknown> {
  return {
    title: t.title.trim(),
    description: t.description.trim(),
    preconditions: draftPreconditions(t, dataLabel),
    expected_result: t.expected_result.trim(),
    steps: t.steps.filter((s) => s.action.trim()).map((s) => ({ action: s.action.trim(), data: s.data.trim(), expected: s.expected.trim() })),
    status: 'draft',
    execution_type: 'manual',
  }
}

/** Tags for accepted CWE mappings ("CWE-79"), without the ones already there. */
export function addCweTags(tags: readonly string[], ids: readonly string[]): string[] {
  const out = [...tags]
  const have = new Set(tags.map((t) => t.toUpperCase()))
  for (const id of ids) if (!have.has(id.toUpperCase())) out.push(id)
  return out
}

/** Adds ids to a list without duplicates. */
export function addIds(list: readonly string[] | null | undefined, ids: readonly string[]): string[] {
  const out = [...(list ?? [])]
  for (const id of ids) if (!out.includes(id)) out.push(id)
  return out
}
