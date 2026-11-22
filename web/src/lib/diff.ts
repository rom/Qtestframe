// Word-level text diff and snapshot comparison for the revision history.

export type DiffOp = 'equal' | 'insert' | 'delete'

export interface DiffSegment {
  op: DiffOp
  text: string
}

/** Split text into words and whitespace runs, keeping every character. */
export function tokenize(text: string): string[] {
  if (!text) return []
  return text.match(/\s+|[^\s]+/g) ?? []
}

/** Merge adjacent segments with the same operation and drop empty ones. */
export function mergeSegments(segs: readonly DiffSegment[]): DiffSegment[] {
  const out: DiffSegment[] = []
  for (const s of segs) {
    if (!s.text) continue
    const last = out[out.length - 1]
    if (last && last.op === s.op) last.text += s.text
    else out.push({ op: s.op, text: s.text })
  }
  return out
}

/** Above this many token comparisons the diff degrades to delete + insert. */
export const MAX_DIFF_CELLS = 4_000_000

/**
 * Word-level diff of two strings (longest common subsequence over word and
 * whitespace tokens). Concatenating the `equal` + `delete` segments yields
 * `before`, `equal` + `insert` segments yields `after`.
 */
export function diffWords(before: string | null | undefined, after: string | null | undefined): DiffSegment[] {
  const a = tokenize(before ?? '')
  const b = tokenize(after ?? '')
  // Common prefix / suffix are cheap and keep the LCS table small.
  let start = 0
  while (start < a.length && start < b.length && a[start] === b[start]) start++
  let endA = a.length
  let endB = b.length
  while (endA > start && endB > start && a[endA - 1] === b[endB - 1]) {
    endA--
    endB--
  }
  const prefix: DiffSegment = { op: 'equal', text: a.slice(0, start).join('') }
  const suffix: DiffSegment = { op: 'equal', text: a.slice(endA).join('') }
  const ma = a.slice(start, endA)
  const mb = b.slice(start, endB)
  const middle: DiffSegment[] = []
  if (ma.length * mb.length > MAX_DIFF_CELLS) {
    middle.push({ op: 'delete', text: ma.join('') }, { op: 'insert', text: mb.join('') })
  } else {
    const n = ma.length
    const m = mb.length
    // lcs[i][j] = LCS length of ma[i:] and mb[j:]
    const lcs: Uint32Array[] = Array.from({ length: n + 1 }, () => new Uint32Array(m + 1))
    for (let i = n - 1; i >= 0; i--) {
      for (let j = m - 1; j >= 0; j--) {
        lcs[i][j] = ma[i] === mb[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1])
      }
    }
    let i = 0
    let j = 0
    while (i < n && j < m) {
      if (ma[i] === mb[j]) {
        middle.push({ op: 'equal', text: ma[i] })
        i++
        j++
      } else if (lcs[i + 1][j] >= lcs[i][j + 1]) {
        middle.push({ op: 'delete', text: ma[i++] })
      } else {
        middle.push({ op: 'insert', text: mb[j++] })
      }
    }
    while (i < n) middle.push({ op: 'delete', text: ma[i++] })
    while (j < m) middle.push({ op: 'insert', text: mb[j++] })
  }
  return mergeSegments([prefix, ...reorder(middle), suffix])
}

/** Within each run of changes, put deletions before insertions (more readable). */
function reorder(segs: DiffSegment[]): DiffSegment[] {
  const out: DiffSegment[] = []
  let del: DiffSegment[] = []
  let ins: DiffSegment[] = []
  const flush = () => {
    out.push(...del, ...ins)
    del = []
    ins = []
  }
  for (const s of segs) {
    // whitespace that is equal between two changes stays attached to the change
    if (s.op === 'delete') del.push(s)
    else if (s.op === 'insert') ins.push(s)
    else {
      flush()
      out.push(s)
    }
  }
  flush()
  return out
}

// ---------- snapshots ----------

/** Fields never shown in a revision diff (derived, bookkeeping). */
export const IGNORED_FIELDS = new Set([
  'id',
  'project_id',
  'key',
  'version',
  'position',
  'created_by',
  'created_at',
  'updated_at',
  'folder_path',
  'linked_tests',
  'linked_requirements',
  'coverage',
  'last_result',
  'comment_count',
  'relation_count',
  'called_by',
  'linked_test',
])

/** Display order for known fields; unknown fields follow alphabetically. */
const FIELD_ORDER = [
  'title',
  'description',
  'obligation',
  'type',
  'execution_type',
  'priority',
  'status',
  'preconditions',
  'source',
  'rationale',
  'owner',
  'estimated_minutes',
  'script_language',
  'script',
  'expected_result',
  'folder_id',
  'category_ids',
  'tags',
  'criteria',
  'steps',
]

const ITEM_KEYS = ['description', 'method', 'threshold', 'action', 'expected', 'data']
const ITEM_SKIP = new Set(['id', 'position', 'requirement_id', 'test_case_id', 'call_test_title', 'call_test_id'])

export type Snapshot = Record<string, unknown>

export interface TextOptions {
  /** Resolve ids (e.g. category ids, folder ids) to readable names. */
  resolve?: (field: string, id: string) => string | undefined
}

function itemLine(x: unknown, i: number): string {
  if (x === null || typeof x !== 'object') return `${i + 1}. ${String(x)}`
  const o = x as Record<string, unknown>
  if (o.call_test_id) {
    const k = o.call_test_key || o.call_test_id
    return `${i + 1}. Calls ${k}${o.call_test_title ? ' – ' + o.call_test_title : ''}`
  }
  const parts: string[] = []
  for (const k of ITEM_KEYS) if (o[k] !== undefined && o[k] !== null && o[k] !== '') parts.push(String(o[k]))
  for (const [k, v] of Object.entries(o)) {
    if (ITEM_KEYS.includes(k) || ITEM_SKIP.has(k) || k === 'call_test_key' || v === null || v === '' || v === undefined) continue
    parts.push(typeof v === 'object' ? JSON.stringify(v) : String(v))
  }
  return `${i + 1}. ${parts.join(' | ')}`
}

/** Readable text for a snapshot value: lists of objects become numbered lines. */
export function valueToText(field: string, value: unknown, opts: TextOptions = {}): string {
  if (value === null || value === undefined) return ''
  if (Array.isArray(value)) {
    if (value.every((v) => typeof v !== 'object' || v === null)) {
      return value.map((v) => (opts.resolve && typeof v === 'string' ? opts.resolve(field, v) ?? v : String(v))).join(', ')
    }
    return value.map((v, i) => itemLine(v, i)).join('\n')
  }
  if (typeof value === 'object') return JSON.stringify(value)
  if (typeof value === 'string' && opts.resolve) return opts.resolve(field, value) ?? value
  return String(value)
}

function isEmpty(v: unknown): boolean {
  return v === null || v === undefined || v === '' || (Array.isArray(v) && v.length === 0)
}

/** Drop volatile keys (ids, positions) of list items before comparing. */
function comparable(v: unknown): unknown {
  if (!Array.isArray(v)) return v
  return v.map((x) => {
    if (x === null || typeof x !== 'object' || Array.isArray(x)) return x
    const o: Record<string, unknown> = {}
    for (const [k, val] of Object.entries(x as Record<string, unknown>)) {
      if (k === 'id' || k === 'position' || k === 'requirement_id' || k === 'test_case_id' || k === 'call_test_key' || k === 'call_test_title') continue
      if (val === null || val === '' || val === undefined) continue
      o[k] = val
    }
    return o
  })
}

function same(a: unknown, b: unknown): boolean {
  if (isEmpty(a) && isEmpty(b)) return true
  return JSON.stringify(comparable(a)) === JSON.stringify(comparable(b))
}

/** Sort field names by the display order. */
export function sortFields(fields: Iterable<string>): string[] {
  const idx = (f: string) => {
    const i = FIELD_ORDER.indexOf(f)
    return i < 0 ? FIELD_ORDER.length : i
  }
  return [...new Set(fields)].sort((a, b) => idx(a) - idx(b) || a.localeCompare(b))
}

/** Top-level keys that differ between two snapshots (ignoring derived fields). */
export function changedKeys(before: Snapshot | null | undefined, after: Snapshot | null | undefined): string[] {
  const keys = new Set([...Object.keys(before ?? {}), ...Object.keys(after ?? {})])
  const out: string[] = []
  for (const k of keys) {
    if (IGNORED_FIELDS.has(k)) continue
    if (k === 'custom_fields') {
      // one change per custom field, as the server reports them
      const b = (before?.[k] ?? {}) as Record<string, unknown>
      const a = (after?.[k] ?? {}) as Record<string, unknown>
      for (const ck of new Set([...Object.keys(b), ...Object.keys(a)])) if (!same(b[ck], a[ck])) out.push(`custom_fields.${ck}`)
      continue
    }
    if (!same(before?.[k], after?.[k])) out.push(k)
  }
  return sortFields(out)
}

/** The value of a field in a snapshot; custom_fields.<key> reads a custom field. */
function fieldValue(snap: Snapshot | null | undefined, field: string): unknown {
  if (field.startsWith('custom_fields.')) return ((snap?.custom_fields ?? {}) as Record<string, unknown>)[field.slice(14)]
  return snap?.[field]
}

export interface FieldChange {
  field: string
  before: string
  after: string
  /** Word diff for multi-word values, else null (show before → after). */
  segments: DiffSegment[] | null
}

/** Long-text fields always rendered as a word diff. */
const TEXT_FIELDS = new Set(['title', 'description', 'rationale', 'preconditions', 'script', 'expected_result', 'source', 'criteria', 'steps'])

/**
 * Field-by-field comparison of two snapshots. `fields` (e.g. the revision's
 * changed_fields) limits the output; when null/undefined every differing
 * field is listed. A missing `before` treats every non-empty field as added.
 * Fields whose readable text is identical are omitted.
 */
export function diffSnapshots(
  before: Snapshot | null | undefined,
  after: Snapshot | null | undefined,
  fields?: readonly string[] | null,
  opts: TextOptions = {},
): FieldChange[] {
  const list = fields ? sortFields(fields.filter((f) => !IGNORED_FIELDS.has(f))) : changedKeys(before, after)
  const out: FieldChange[] = []
  for (const field of list) {
    const b = valueToText(field, fieldValue(before, field), opts)
    const a = valueToText(field, fieldValue(after, field), opts)
    if (a === b) continue
    const wordy = TEXT_FIELDS.has(field) || /\s/.test(b) || /\s/.test(a)
    out.push({ field, before: b, after: a, segments: wordy ? diffWords(b, a) : null })
  }
  return out
}

/** Parse a revision snapshot that may arrive as an object or a JSON string. */
export function parseSnapshot(raw: unknown): Snapshot | null {
  if (raw === null || raw === undefined || raw === '') return null
  if (typeof raw === 'string') {
    try {
      const v = JSON.parse(raw)
      return v && typeof v === 'object' && !Array.isArray(v) ? (v as Snapshot) : null
    } catch {
      return null
    }
  }
  return typeof raw === 'object' && !Array.isArray(raw) ? (raw as Snapshot) : null
}
