// Autocomplete for free-text fields (GET /suggest, POST /suggest/history;
// see docs/api.md#autocomplete). A field with a field key suggests the
// values the user saved in it before (their history, in any project they
// may read) and, for a few short fields, the values used in the project.
// The server ranks; this module holds the field keys, the preference
// logic, the request helpers with a short cache, and the pure helpers of
// the popup (SuggestInput.vue, composables/suggest.ts).
import { api, asItems } from './api'
import type { Preferences } from './types'

/** Field keys (internal/api/suggest.go suggestFields). */
export const SUGGEST_FIELDS = [
  'requirement.title',
  'requirement.source',
  'requirement.owner',
  'requirement.rationale',
  'test.title',
  'test.preconditions',
  'test.expected',
  'test.step.action',
  'test.step.expected',
  'test.step.data',
  'execution.notes',
  'execution.step.actual',
  'run.name',
  'defect.title',
  'defect.description',
  'risk.title',
  'risk.mitigation',
  'risk.owner',
  'comment',
  'review.comment',
  'release.name',
  'search',
] as const
export type SuggestField = (typeof SUGGEST_FIELDS)[number]

/** Field keys that read like message keys elsewhere (locales.test.ts looks for quoted keys). */
export const REVIEW_COMMENT: SuggestField = 'review.comment'
export const RUN_NAME: SuggestField = 'run.name'

/** Fields that belong to no project. */
export const GLOBAL_FIELDS: readonly SuggestField[] = ['search']

export interface Suggestion {
  value: string
  source: 'history' | 'project'
  uses: number
  last_used?: string
}

/** Values of Preferences.autocomplete ('' = on, both sources). */
export const AUTOCOMPLETE_MODES = ['', 'history', 'project', 'off'] as const
export type AutocompleteMode = (typeof AUTOCOMPLETE_MODES)[number]

/** Wait after the last keystroke before asking the server. */
export const SUGGEST_DEBOUNCE_MS = 200
/** The most suggestions shown (the server's limit). */
export const SUGGEST_LIMIT = 8
/** Longer texts are not matched (the server's limit). */
export const MAX_QUERY = 200
/** Longer values are not recorded (the server's limit). */
export const MAX_VALUE = 1000
/** How long an answer is reused for the same field, project and text. */
export const CACHE_MS = 30_000

export function autocompleteMode(prefs: Preferences | null | undefined): AutocompleteMode {
  const m = prefs?.autocomplete ?? ''
  return (AUTOCOMPLETE_MODES as readonly string[]).includes(m) ? (m as AutocompleteMode) : ''
}

/** Whether fields show suggestions at all (and call the server). */
export function autocompleteEnabled(prefs: Preferences | null | undefined): boolean {
  return autocompleteMode(prefs) !== 'off'
}

/** Whether saved values are recorded in the user's history. */
export function recordsHistory(prefs: Preferences | null | undefined): boolean {
  const m = autocompleteMode(prefs)
  return m === '' || m === 'history'
}

export function isSuggestField(f: string | null | undefined): f is SuggestField {
  return !!f && (SUGGEST_FIELDS as readonly string[]).includes(f)
}

/** Whether a field may ask for suggestions in this project ('' = none). */
export function canSuggest(field: string | null | undefined, projectId: string | null | undefined): boolean {
  if (!isSuggestField(field)) return false
  return GLOBAL_FIELDS.includes(field) ? true : !!projectId
}

/** The text a field asks about: the whole value, trimmed; null = do not ask. */
export function queryOf(value: string | null | undefined, opts: { allowEmpty?: boolean } = {}): string | null {
  const q = (value ?? '').trim()
  if (q.length > MAX_QUERY) return null
  if (!q && !opts.allowEmpty) return null
  return q
}

/**
 * The next active option for an arrow key: -1 is "none" (the typed text),
 * so the list wraps through it: ↓ from none to the first, ↑ from the first
 * back to none.
 */
export function nextActive(current: number, count: number, dir: 1 | -1): number {
  if (count <= 0) return -1
  const n = count + 1 // the options and "none"
  const pos = (current + 1 + dir + n) % n
  return pos - 1
}

/** A debounced function with cancel (timers injectable for tests). */
export function debounce<A extends unknown[]>(fn: (...args: A) => void, ms: number) {
  let timer: ReturnType<typeof setTimeout> | undefined
  const call = (...args: A) => {
    clearTimeout(timer)
    timer = setTimeout(() => {
      timer = undefined
      fn(...args)
    }, ms)
  }
  const cancel = () => {
    clearTimeout(timer)
    timer = undefined
  }
  return { call, cancel, pending: () => timer !== undefined }
}

// ---------- requests ----------

const cache = new Map<string, { at: number; items: Suggestion[] }>()

function cacheKey(field: string, projectId: string, q: string) {
  return `${field}\u0000${projectId}\u0000${q.toLowerCase()}`
}

/** Forget cached answers (after recording, on sign-out, after clearing). */
export function clearSuggestCache(): void {
  cache.clear()
}

/** GET /suggest: the ranked suggestions of a field for the typed text. */
export async function fetchSuggestions(field: SuggestField, q: string, projectId: string, opts: { signal?: AbortSignal; now?: number } = {}): Promise<Suggestion[]> {
  const now = opts.now ?? Date.now()
  const pid = GLOBAL_FIELDS.includes(field) ? '' : projectId
  const key = cacheKey(field, pid, q)
  const hit = cache.get(key)
  if (hit && now - hit.at < CACHE_MS) return hit.items
  const res = await api.get<{ items: Suggestion[] }>('/suggest', { field, q, project_id: pid, limit: SUGGEST_LIMIT }, { signal: opts.signal })
  const items = asItems(res)
  if (cache.size > 200) cache.clear()
  cache.set(key, { at: now, items })
  return items
}

export interface SuggestEntry {
  field: SuggestField
  value: string
}

type Values = Partial<Record<SuggestField, string | null | undefined | (string | null | undefined)[]>>

function list(v: string | null | undefined | (string | null | undefined)[]): string[] {
  return (Array.isArray(v) ? v : [v]).map((x) => (x ?? '').trim()).filter((x) => x && x.length <= MAX_VALUE)
}

/**
 * The values to record after a save: the non-empty values of each field
 * that the item did not have before (so an unchanged field is not counted
 * again on every save), each once.
 */
export function savedEntries(after: Values, before: Values = {}): SuggestEntry[] {
  const out: SuggestEntry[] = []
  for (const [f, v] of Object.entries(after) as [SuggestField, Values[SuggestField]][]) {
    const old = new Set(list(before[f]))
    const seen = new Set<string>()
    for (const x of list(v)) {
      if (old.has(x) || seen.has(x)) continue
      seen.add(x)
      out.push({ field: f, value: x })
    }
  }
  return out
}

/**
 * POST /suggest/history after a successful save. Does nothing when the
 * preference does not record; errors are ignored (autocomplete is a
 * convenience and must never break a save).
 */
export async function rememberValues(prefs: Preferences | null | undefined, projectId: string, entries: SuggestEntry[]): Promise<void> {
  if (!entries.length || !recordsHistory(prefs)) return
  try {
    await api.post('/suggest/history', { project_id: projectId, entries })
    clearSuggestCache()
  } catch {
    /* ignored */
  }
}

/** DELETE /suggest/history: clears the user's history; the number of values deleted. */
export async function clearHistory(): Promise<number> {
  const res = await api.del<{ deleted: number }>('/suggest/history')
  clearSuggestCache()
  return res?.deleted ?? 0
}

/** GET /suggest/history: how many values the user has stored. */
export async function historyCount(): Promise<number> {
  const res = await api.get<{ count: number }>('/suggest/history')
  return res?.count ?? 0
}
