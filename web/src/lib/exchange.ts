// Exchanging data with other tools (ExchangeImportDialog): the Excel
// round-trip and the ReqIF import with re-import. Types of the preview and
// apply responses (internal/interop, internal/api/exchange*.go) and pure
// helpers.

export type ExchangeKind = 'requirement' | 'test'
export type ExchangeMode = 'xlsx' | 'reqif'
export type RowStatus = 'new' | 'changed' | 'unchanged' | 'conflict' | 'error'

/** Row statuses in the order the preview shows them. */
export const ROW_STATUSES: readonly RowStatus[] = ['new', 'changed', 'conflict', 'error', 'unchanged']

export interface ExchangeChange {
  field: string
  label: string
  cell: string
  old: string
  new: string
}

export interface ExchangeCellError {
  field: string
  label: string
  cell: string
  message: string
}

export interface ExchangeRow {
  index: number
  row: number
  location: string
  status: RowStatus
  id?: string
  key?: string
  ref?: string
  title: string
  base_version?: number
  version?: number
  changes: ExchangeChange[]
  errors: ExchangeCellError[]
  warnings: string[]
}

export interface ExchangeSummary {
  new: number
  changed: number
  unchanged: number
  conflicts: number
  errors: number
}

export interface ExchangeField {
  key: string
  label: string
  type: string
  values?: string[]
  read_only: boolean
  relation?: string
}

export interface ExchangeDiff {
  kind: ExchangeKind
  file: string
  rows: ExchangeRow[]
  summary: ExchangeSummary
  columns: ExchangeField[]
  ignored: string[]
  warnings: string[]
}

export interface ReqIFAttribute {
  name: string
  type: string
  values?: string[]
  objects: number
  sample: string
  object_types: string[]
  field: string
  guess: string
}

export interface ReqIFTarget {
  key: string
  label: string
  type: string
}

export interface ReqIFRelationChange {
  ref: string
  type: string
  file_type: string
  source: string
  target: string
  action: 'add' | 'remove' | 'keep'
}

export interface ReqIFPreview extends ExchangeDiff {
  title: string
  tool: string
  qtestframe: boolean
  attributes: ReqIFAttribute[]
  targets: ReqIFTarget[]
  relation_types: Record<string, string>
  relations: { add: number; remove: number; keep: number; changes: ReqIFRelationChange[] }
  folders: number
  skipped: string[]
  errors: string[]
  previous: number
  target_folder: string
}

export interface ExchangeResult {
  created: number
  updated: number
  unchanged: number
  skipped: number
  folders_created: number
  relations_added: number
  relations_removed: number
  links_added: number
  links_removed: number
  references_saved?: number
  items: { index: number; location: string; id: string; key: string; title: string; action: 'created' | 'updated'; version: number }[]
  skipped_rows: { index: number; location: string; title: string; reason: 'error' | 'conflict' | 'excluded' | 'new' }[]
}

/** File types offered by the file chooser. */
export const EXCHANGE_ACCEPT: Record<ExchangeMode, string> = { xlsx: '.xlsx,.xlsm', reqif: '.reqif,.reqifz,.xml' }

/** Largest accepted file (server limit, MB). */
export const EXCHANGE_MAX_MB = 20

/** Whether a file name fits the mode. */
export function acceptsFile(mode: ExchangeMode, name: string): boolean {
  const ext = name.toLowerCase().split('.').pop() ?? ''
  return mode === 'xlsx' ? ext === 'xlsx' || ext === 'xlsm' : ext === 'reqif' || ext === 'reqifz' || ext === 'xml'
}

/** Count of rows of a status in the summary. */
export function statusCount(s: ExchangeSummary, status: RowStatus): number {
  switch (status) {
    case 'new':
      return s.new
    case 'changed':
      return s.changed
    case 'unchanged':
      return s.unchanged
    case 'conflict':
      return s.conflicts
    default:
      return s.errors
  }
}

/** Whether a row can be saved (with the options). */
export function savable(r: ExchangeRow, opts: { overwrite: boolean; create: boolean }): boolean {
  switch (r.status) {
    case 'new':
      return opts.create
    case 'changed':
      return true
    case 'conflict':
      return opts.overwrite
    default:
      return false
  }
}

/** Rows that an apply with these options and exclusions would save. */
export function saveCount(rows: readonly ExchangeRow[], opts: { overwrite: boolean; create: boolean }, excluded: ReadonlySet<number>): number {
  return rows.filter((r) => !excluded.has(r.index) && savable(r, opts)).length
}

/** Rows shown for the status filter ([] = all but unchanged). */
export function visibleRows(rows: readonly ExchangeRow[], statuses: readonly RowStatus[]): ExchangeRow[] {
  const want = statuses.length ? new Set(statuses) : new Set<RowStatus>(['new', 'changed', 'conflict', 'error'])
  return rows.filter((r) => want.has(r.status))
}

/** Mapping to send: only attributes whose field differs from the guess. */
export function changedAttrMapping(attrs: readonly ReqIFAttribute[], chosen: Readonly<Record<string, string>>): Record<string, string> | undefined {
  const out: Record<string, string> = {}
  for (const a of attrs) {
    if (!(a.name in chosen)) continue
    const want = chosen[a.name] ?? ''
    if (want !== (a.guess || '')) out[a.name] = want || 'ignore'
  }
  return Object.keys(out).length ? out : undefined
}

/** Multipart fields of a preview or apply request. */
export function exchangeFields(
  mode: ExchangeMode,
  opts: { mapping?: Record<string, string>; folderId?: string | null; hierarchy?: boolean; kind?: ExchangeKind },
  apply?: { overwrite: boolean; create: boolean; excluded: Iterable<number> },
): Record<string, string | undefined> {
  const f: Record<string, string | undefined> = {}
  if (mode === 'reqif') {
    f.mapping = opts.mapping ? JSON.stringify(opts.mapping) : undefined
    f.folder_id = opts.folderId || undefined
    f.hierarchy = opts.hierarchy === false ? 'false' : undefined
  } else {
    f.kind = opts.kind
  }
  if (apply) {
    f.overwrite_conflicts = String(apply.overwrite)
    f.create = String(apply.create)
    const ex = [...apply.excluded].sort((a, b) => a - b)
    if (ex.length) f.exclude = JSON.stringify(ex)
  }
  return f
}

/** Shortened text for a diff cell. */
export function clip(s: string, n = 140): string {
  const one = s.replace(/\s+/g, ' ').trim()
  return one.length > n ? one.slice(0, n - 1) + '…' : one
}
