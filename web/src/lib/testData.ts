// Data-driven tests: parameters, data rows and {{name}} placeholders in
// step texts (mirrors internal/store/testdata.go).
import { deriveStatus } from './summary'
import type { DataRow } from './types'

/** a parameter name: a letter or _, then letters, digits, _ . or - */
const NAME = /^[\p{L}_][\p{L}\p{N}_.-]*$/u
/** {{name}} with optional spaces inside the braces */
const PLACEHOLDER = /\{\{\s*([\p{L}_][\p{L}\p{N}_.-]*)\s*\}\}/gu

export const MAX_PARAMETERS = 50
export const MAX_ROWS = 500
export const MAX_NAME = 50

/** The placeholder text of a parameter, e.g. {{username}}. */
export function placeholderOf(name: string): string {
  return '{{' + name + '}}'
}

export function validParameterName(name: string): boolean {
  return NAME.test(name) && [...name].length <= MAX_NAME
}

/** Why a list of parameter names cannot be saved: '' | 'invalid' | 'duplicate'. */
export function parameterProblem(names: readonly string[], i: number): '' | 'invalid' | 'duplicate' {
  const n = names[i]?.trim() ?? ''
  if (!validParameterName(n)) return 'invalid'
  if (names.some((o, j) => j < i && o.trim().toLowerCase() === n.toLowerCase())) return 'duplicate'
  return ''
}

/** Replace {{name}} with values[name]; unknown placeholders stay as they are. */
export function resolvePlaceholders(text: string, values: Record<string, string> | null | undefined): string {
  if (!text || !values || !text.includes('{{')) return text
  return text.replace(PLACEHOLDER, (m, name: string) => (Object.prototype.hasOwnProperty.call(values, name) ? values[name] : m))
}

/** Parameter names referenced in texts (first use order, no duplicates). */
export function placeholdersIn(...texts: (string | null | undefined)[]): string[] {
  const out: string[] = []
  for (const t of texts) {
    for (const m of (t ?? '').matchAll(PLACEHOLDER)) if (!out.includes(m[1])) out.push(m[1])
  }
  return out
}

/** Placeholders used in texts that are not parameters (likely typos). */
export function unknownPlaceholders(texts: readonly (string | null | undefined)[], parameters: readonly string[]): string[] {
  const known = new Set(parameters.map((p) => p.trim()))
  return placeholdersIn(...texts).filter((p) => !known.has(p))
}

/** A blank row with an empty value for every parameter. */
export function blankRow(parameters: readonly string[], name = ''): DataRow {
  return { name, values: Object.fromEntries(parameters.map((p) => [p, ''])) }
}

/** Rows after renaming a parameter (values move to the new name). */
export function renameParameter(rows: readonly DataRow[], from: string, to: string): DataRow[] {
  if (from === to) return rows.map((r) => ({ ...r, values: { ...r.values } }))
  return rows.map((r) => {
    const values: Record<string, string> = {}
    for (const [k, v] of Object.entries(r.values ?? {})) values[k === from ? to : k] = v
    if (!(to in values)) values[to] = ''
    return { ...r, values }
  })
}

/** Rows without the values of a parameter. */
export function removeParameter(rows: readonly DataRow[], name: string): DataRow[] {
  return rows.map((r) => {
    const values = { ...(r.values ?? {}) }
    delete values[name]
    return { ...r, values }
  })
}

/**
 * Parse pasted cells: tab separated (Excel, Google Sheets, LibreOffice) when
 * the text contains a tab, else comma or semicolon separated (CSV, whichever
 * occurs more often in the first line). Double quotes enclose values with
 * separators, line breaks or quotes ("" is a quote). Empty trailing lines
 * are dropped.
 */
export function parseTable(text: string): string[][] {
  const src = text.replace(/\r\n?/g, '\n')
  const first = src.split('\n', 1)[0] ?? ''
  const sep = src.includes('\t') ? '\t' : (first.match(/;/g)?.length ?? 0) > (first.match(/,/g)?.length ?? 0) ? ';' : ','
  const rows: string[][] = []
  let row: string[] = []
  let cell = ''
  let quoted = false
  let i = 0
  while (i < src.length) {
    const c = src[i]
    if (quoted) {
      if (c === '"' && src[i + 1] === '"') {
        cell += '"'
        i += 2
        continue
      }
      if (c === '"') quoted = false
      else cell += c
      i++
      continue
    }
    if (c === '"' && cell === '') quoted = true
    else if (c === sep) {
      row.push(cell)
      cell = ''
    } else if (c === '\n') {
      row.push(cell)
      rows.push(row)
      row = []
      cell = ''
    } else cell += c
    i++
  }
  if (cell !== '' || row.length) {
    row.push(cell)
    rows.push(row)
  }
  while (rows.length && rows[rows.length - 1].every((x) => x.trim() === '')) rows.pop()
  return rows
}

export interface TableImport {
  parameters: string[]
  rows: DataRow[]
}

/**
 * Turn parsed cells into test data. With header, the first line names the
 * parameters (else the given parameters are used, in order; extra columns
 * are ignored). With nameColumn, the first column holds the row names.
 */
export function tableToData(cells: string[][], opts: { header: boolean; nameColumn: boolean; parameters?: readonly string[] }): TableImport {
  const skip = opts.nameColumn ? 1 : 0
  const lines = [...cells]
  let parameters: string[]
  if (opts.header) {
    const head = lines.shift() ?? []
    parameters = head.slice(skip).map((h) => h.trim())
  } else parameters = [...(opts.parameters ?? [])]
  const rows = lines.map((cells) => {
    const values: Record<string, string> = {}
    parameters.forEach((p, i) => (values[p] = cells[i + skip] ?? ''))
    return { name: opts.nameColumn ? (cells[0] ?? '').trim() : '', values }
  })
  return { parameters, rows }
}

/** Test data as tab-separated text (header line with the parameters; row names first). */
export function dataToTSV(parameters: readonly string[], rows: readonly DataRow[]): string {
  const esc = (s: string) => (/[\t\n"]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s)
  const lines = [['', ...parameters].map(esc).join('\t')]
  for (const r of rows) lines.push([r.name ?? '', ...parameters.map((p) => r.values?.[p] ?? '')].map(esc).join('\t'))
  return lines.join('\n')
}

/** Result of an iteration from its step results (like an execution). */
export function iterationStatus(stepStatuses: readonly string[]): string {
  return deriveStatus(stepStatuses)
}

/**
 * Overall result of a data-driven execution from its iterations: failed if
 * any failed, else blocked if any is blocked, passed when all passed (or
 * were skipped), skipped when all were skipped, not_run when none has a
 * result, otherwise in_progress.
 */
export function executionStatus(iterationStatuses: readonly string[]): string {
  return deriveStatus(iterationStatuses)
}
