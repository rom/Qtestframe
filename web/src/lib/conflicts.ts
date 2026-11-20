// Edit conflicts: someone saved an item after you opened it (the API answers
// 412 "stale" with the item as it is saved now). Which fields did they
// change, and did you change the same ones?
import { ApiError } from './api'

export interface Stale<T> {
  /** the item as it is saved now */
  current: T
  changedBy: string
  /** ISO time of the newer version */
  changedAt: string
}

/** The details of a 412 "stale" answer, or null for any other error. */
export function staleOf<T>(e: unknown): Stale<T> | null {
  if (!(e instanceof ApiError) || e.status !== 412 || e.code !== 'stale') return null
  const b = (e.body ?? {}) as { current?: T; changed_by?: string; changed_at?: string }
  if (!b.current) return null
  return { current: b.current, changedBy: b.changed_by ?? '', changedAt: b.changed_at ?? '' }
}

export interface FieldConflict {
  field: string
  /** their (saved) value */
  theirs: unknown
  /** your (unsaved) value */
  mine: unknown
  /** you changed this field too, differently */
  both: boolean
}

type Normalize = (v: unknown) => unknown

/** Lists keep only the given keys of their objects (ignores ids, positions, keys). */
export function project(keys: string[]): Normalize {
  return (v) => (Array.isArray(v) ? v.map((x) => (x && typeof x === 'object' ? Object.fromEntries(keys.map((k) => [k, (x as Record<string, unknown>)[k] ?? ''])) : x)) : [])
}
/** Order does not matter (tags, categories). */
export const asSet: Normalize = (v) => (Array.isArray(v) ? [...v].map(String).sort() : [])

const same = (a: unknown, b: unknown) => JSON.stringify(a ?? null) === JSON.stringify(b ?? null)

/**
 * The fields they changed (saved value differs from what you started from),
 * with your value; `both` when you changed it too and to something else.
 */
export function fieldConflicts(fields: string[], base: object, theirs: object, mine: object, normalize: Record<string, Normalize> = {}): FieldConflict[] {
  const out: FieldConflict[] = []
  const get = (o: object, f: string) => {
    const v = (o as Record<string, unknown>)[f]
    return normalize[f] ? normalize[f](v) : (v ?? (typeof v === 'number' ? 0 : ''))
  }
  for (const f of fields) {
    const b = get(base, f)
    const t = get(theirs, f)
    if (same(b, t)) continue
    const m = get(mine, f)
    out.push({ field: f, theirs: t, mine: m, both: !same(m, b) && !same(m, t) })
  }
  return out
}

/** A short text of a value for the comparison table. */
export function displayValue(v: unknown, max = 160): string {
  if (v === null || v === undefined || v === '') return '—'
  if (Array.isArray(v)) {
    if (!v.length) return '—'
    if (v.every((x) => typeof x !== 'object')) return clip(v.join(', '), max)
    return clip(v.map((x) => Object.values(x as Record<string, unknown>).filter((y) => y !== '' && y !== null).join(' · ')).join(' / '), max)
  }
  return clip(String(v), max)
}

function clip(s: string, max: number): string {
  const t = s.replace(/\s+/g, ' ').trim()
  return t.length > max ? t.slice(0, max - 1) + '…' : t
}

/** The editable fields of a requirement and how to compare them. */
export const REQUIREMENT_FIELDS = {
  fields: ['title', 'description', 'obligation', 'type', 'priority', 'status', 'source', 'rationale', 'owner', 'tags', 'category_ids', 'folder_id', 'criteria'],
  normalize: { tags: asSet, category_ids: asSet, criteria: project(['description', 'method', 'threshold']) } as Record<string, Normalize>,
}

/** The editable fields of a test and how to compare them. */
export const TEST_FIELDS = {
  fields: ['title', 'description', 'preconditions', 'execution_type', 'script_language', 'script', 'expected_result', 'priority', 'status', 'estimated_minutes', 'tags', 'category_ids', 'folder_id', 'steps'],
  normalize: { tags: asSet, category_ids: asSet, steps: project(['action', 'expected', 'data', 'call_test_id']) } as Record<string, Normalize>,
}
