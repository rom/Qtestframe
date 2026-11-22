// Requirement import (RequirementImportDialog): types of the preview and
// result responses and pure helpers for building the upload form.

export type ImportFormat = 'csv' | 'xlsx' | 'reqif' | 'docx'

/** Candidate fields a column can map to (internal/reqimport Fields). */
export const IMPORT_FIELDS = [
  'title',
  'description',
  'external_id',
  'obligation',
  'type',
  'priority',
  'status',
  'source',
  'rationale',
  'owner',
  'tags',
  'folder',
  'criteria',
] as const
export type ImportField = (typeof IMPORT_FIELDS)[number]

/** Value of an ignored column in the mapping. */
export const IGNORE = 'ignore'

/** File types offered by the file chooser. */
export const IMPORT_ACCEPT = '.csv,.tsv,.txt,.xlsx,.xlsm,.reqif,.reqifz,.docx'

/** Largest accepted file (server limit, MB). */
export const IMPORT_MAX_MB = 20

/** Number of candidates shown in the preview table. */
export const PREVIEW_ROWS = 50

export interface ImportColumn {
  index: number
  header: string
  /** '' = ignored */
  field: string
}

export interface ImportCandidate {
  title: string
  description: string
  obligation: string
  obligation_from: '' | 'column' | 'text'
  type: string
  priority: string
  status: string
  source: string
  rationale: string
  owner: string
  tags: string[]
  folder: string
  criteria: { description: string; method: string; threshold: string }[]
  external_id: string
  row: number
  location: string
  warnings: string[]
  duplicate?: '' | 'external_id' | 'title'
}

export interface ImportPreview {
  format: ImportFormat
  sheet?: string
  sheets?: string[]
  columns: ImportColumn[]
  candidates: ImportCandidate[]
  warnings: string[]
  errors: string[]
  fields: string[]
  /** mapping presets for the exports of other tools (DOORS, Polarion) */
  presets?: { id: string; name: string }[]
  preset?: string
  total: number
  duplicates: number
  target_folder: string
}

export interface ImportResult {
  created: number
  skipped: number
  folders_created: number
  tests_created: number
  dry_run: boolean
  format: ImportFormat
  items: { id: string; key: string; title: string; row: number; location: string; folder_id: string | null }[]
  skipped_items: { title: string; row: number; location: string; reason: string }[]
  warnings: string[]
  errors: string[]
}

export interface ImportOptions {
  folderId: string | null
  createFolders: boolean
  status: string
  createTests: boolean
  skipDuplicates: boolean
  dryRun?: boolean
}

/** Format of a file by its extension (null when unsupported). */
export function formatOf(name: string): ImportFormat | null {
  const ext = name.toLowerCase().split('.').pop() ?? ''
  if (['csv', 'tsv', 'txt'].includes(ext)) return 'csv'
  if (['xlsx', 'xlsm'].includes(ext)) return 'xlsx'
  if (['reqif', 'reqifz'].includes(ext)) return 'reqif'
  if (ext === 'docx') return 'docx'
  return null
}

/**
 * Mapping to send: only the columns whose field differs from the server's
 * guess (header → field, "ignore" for an ignored column). Undefined when
 * nothing changed, so the server keeps guessing.
 */
export function changedMapping(columns: readonly ImportColumn[], chosen: Readonly<Record<string, string>>): Record<string, string> | undefined {
  const out: Record<string, string> = {}
  for (const c of columns) {
    if (!(c.header in chosen)) continue
    const want = chosen[c.header] || ''
    if (want !== (c.field || '')) out[c.header] = want || IGNORE
  }
  return Object.keys(out).length ? out : undefined
}

/** Merge a new preview's columns into the user's choices (a new file resets them). */
export function columnChoices(columns: readonly ImportColumn[], previous: Readonly<Record<string, string>> = {}): Record<string, string> {
  const out: Record<string, string> = {}
  for (const c of columns) out[c.header] = c.header in previous ? previous[c.header] : c.field || ''
  return out
}

/** Multipart fields of a preview or import request. */
export function importFields(
  opts: { sheet?: string | null; mapping?: Record<string, string>; preset?: string | null },
  options?: ImportOptions,
  exclude?: Iterable<number>,
): Record<string, string | undefined> {
  const f: Record<string, string | undefined> = {
    sheet: opts.sheet || undefined,
    mapping: opts.mapping ? JSON.stringify(opts.mapping) : undefined,
    preset: opts.preset || undefined,
  }
  if (options) {
    f.folder_id = options.folderId || undefined
    f.create_folders = String(options.createFolders)
    f.status = options.status
    f.create_tests = String(options.createTests)
    f.skip_duplicates = String(options.skipDuplicates)
    if (options.dryRun) f.dry_run = 'true'
  }
  const ex = exclude ? [...exclude].sort((a, b) => a - b) : []
  if (ex.length) f.exclude = JSON.stringify(ex)
  return f
}

/** Number of candidates an import will create with these options. */
export function importCount(cands: readonly ImportCandidate[], skipDuplicates: boolean, excluded: ReadonlySet<number>): number {
  return cands.filter((c, i) => !excluded.has(i) && !(skipDuplicates && c.duplicate)).length
}

/** Folder path shown in the preview: target root, then the candidate's path. */
export function targetPath(root: string, folder: string, createFolders: boolean): string {
  const parts = [root, createFolders ? folder.split('/').join(' / ') : ''].filter((p) => p.trim())
  return parts.join(' / ')
}
