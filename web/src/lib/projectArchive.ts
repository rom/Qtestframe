// Project archives: save a project as a ZIP file and load one as a new
// project (GET /projects/{pid}/archive, POST /projects/import,
// POST /projects/{pid}/duplicate).
import type { Project } from './types'

/** manifest.json of a project archive. */
export interface ArchiveManifest {
  kind: string
  format_version: number
  schema_version: number
  app_version: string
  created_at: string
  driver?: string
  project: { id: string; key: string; name: string }
  tables: Record<string, number>
  files: number
  file_bytes: number
}

/** The answer of a load (or its dry run) and of a duplication. */
export interface ProjectLoadResult {
  project?: Project
  manifest: ArchiveManifest
  imported: Record<string, number>
  skipped?: Record<string, number>
  files: number
  dry_run?: boolean
  key_taken?: boolean
}

/** The project key rule of the server: 2–16 characters A–Z and 0–9, a letter first. */
export const PROJECT_KEY_RE = /^[A-Z][A-Z0-9]{1,15}$/
export const MAX_PROJECT_KEY = 16

/** Upper case without spaces and other characters. */
export function normalizeProjectKey(key: string): string {
  return key.toUpperCase().replace(/[^A-Z0-9]/g, '').slice(0, MAX_PROJECT_KEY)
}

export function validProjectKey(key: string): boolean {
  return PROJECT_KEY_RE.test(normalizeProjectKey(key))
}

/**
 * The key itself when it is free, else the first free key with a number
 * appended (DEMO → DEMO2, DEMO3 …; shortened to fit 16 characters).
 */
export function freeProjectKey(key: string, taken: Iterable<string>): string {
  const base = normalizeProjectKey(key) || 'P'
  const used = new Set([...taken].map((k) => k.toUpperCase()))
  if (!used.has(base) && PROJECT_KEY_RE.test(base)) return base
  for (let n = 2; n < 10000; n++) {
    const suffix = String(n)
    const k = base.slice(0, MAX_PROJECT_KEY - suffix.length) + suffix
    if (!used.has(k) && PROJECT_KEY_RE.test(k)) return k
  }
  return base
}

/** The most telling counts of an archive, for a summary. */
export interface ArchiveSummary {
  requirements: number
  tests: number
  runs: number
  executions: number
  defects: number
  risks: number
  baselines: number
  files: number
  rows: number
}

export function archiveSummary(tables: Record<string, number> | undefined, files = 0): ArchiveSummary {
  const n = (t: string) => Number(tables?.[t] ?? 0) || 0
  return {
    requirements: n('requirements'),
    tests: n('test_cases'),
    runs: n('test_runs'),
    executions: n('test_executions'),
    defects: n('defects'),
    risks: n('risks'),
    baselines: n('baselines'),
    files,
    rows: totalRows(tables),
  }
}

export function totalRows(counts: Record<string, number> | undefined): number {
  return Object.values(counts ?? {}).reduce((a, b) => a + (Number(b) || 0), 0)
}

/** Whether a file looks like a ZIP archive by name or type. */
export function looksLikeZip(file: { name: string; type?: string }): boolean {
  return /\.zip$/i.test(file.name) || file.type === 'application/zip' || file.type === 'application/x-zip-compressed'
}
