// Reports: standard templates, the custom report builder and generate requests.
// See docs/api.md "Reports".

export const REPORT_FORMATS = ['pdf', 'docx', 'xlsx', 'csv', 'md'] as const
export type ReportFormat = (typeof REPORT_FORMATS)[number]
export const REPORT_ENTITIES = ['requirement', 'test', 'run', 'execution'] as const
export type ReportEntity = (typeof REPORT_ENTITIES)[number]

/** Display name and icon of a format. */
export const FORMAT_INFO: Record<ReportFormat, { label: string; icon: string }> = {
  pdf: { label: 'PDF', icon: 'pi pi-file-pdf' },
  docx: { label: 'Word', icon: 'pi pi-file-word' },
  xlsx: { label: 'Excel', icon: 'pi pi-file-excel' },
  csv: { label: 'CSV', icon: 'pi pi-file' },
  md: { label: 'Markdown', icon: 'pi pi-hashtag' },
}

/** The standard templates the client knows (labels are translated by id). */
export const KNOWN_TEMPLATES = ['project_status', 'requirements_coverage', 'traceability', 'run_summary', 'defects', 'test_inventory', 'risks', 'compliance', 'verification', 'soa', 'release', 'exploratory_session', 'exploratory_sessions', 'cycle'] as const

export interface TemplateParam {
  name: string
  required: boolean
}

export interface ReportTemplate {
  id: string
  name: string
  description: string
  params: TemplateParam[]
  formats: ReportFormat[]
}

export interface ReportField {
  id: string
  label: string
}

export interface CustomConfig {
  entity: ReportEntity
  query?: string
  columns: string[]
  group_by?: string
  sort_by?: string
  sort_desc?: boolean
  include_summary?: boolean
  title?: string
  run_id?: string
}

/** Default params of the known templates, used when the server sends none. */
const DEFAULT_PARAMS: Record<string, TemplateParam[]> = {
  requirements_coverage: [{ name: 'query', required: false }],
  traceability: [{ name: 'query', required: false }],
  run_summary: [{ name: 'run_id', required: true }],
  test_inventory: [{ name: 'query', required: false }],
  verification: [{ name: 'query', required: false }],
  compliance: [{ name: 'sources', required: false }],
  soa: [{ name: 'source', required: false }],
  release: [{ name: 'release_id', required: false }],
}

function isFormat(v: unknown): v is ReportFormat {
  return typeof v === 'string' && (REPORT_FORMATS as readonly string[]).includes(v)
}

/**
 * Normalise a template from GET /reports/templates. `params` may be a list
 * of names ("run_id", "query?"), objects ({name, required}) or a map
 * {name: "required"|"optional"|bool}.
 */
export function normalizeTemplate(raw: unknown): ReportTemplate | null {
  if (!raw || typeof raw !== 'object') return null
  const r = raw as Record<string, unknown>
  const id = typeof r.id === 'string' ? r.id : ''
  if (!id) return null
  let params: TemplateParam[] = []
  const p = r.params
  if (Array.isArray(p)) {
    params = p
      .map((x): TemplateParam | null => {
        if (typeof x === 'string') {
          const name = x.replace(/\?$/, '')
          return name ? { name, required: name === 'run_id' && !x.endsWith('?') } : null
        }
        if (x && typeof x === 'object' && typeof (x as { name?: unknown }).name === 'string') {
          const o = x as { name: string; required?: unknown }
          return { name: o.name, required: o.required === true || (o.required === undefined && o.name === 'run_id') }
        }
        return null
      })
      .filter((x): x is TemplateParam => !!x)
  } else if (p && typeof p === 'object') {
    params = Object.entries(p as Record<string, unknown>).map(([name, v]) => ({ name, required: v === true || v === 'required' }))
  } else {
    params = DEFAULT_PARAMS[id] ?? []
  }
  const formats = Array.isArray(r.formats) ? r.formats.filter(isFormat) : []
  return {
    id,
    name: typeof r.name === 'string' ? r.name : id,
    description: typeof r.description === 'string' ? r.description : '',
    params,
    formats: formats.length ? formats : [...REPORT_FORMATS],
  }
}

/** Missing required template params (names). */
export function missingParams(tpl: ReportTemplate, params: Record<string, string | undefined>): string[] {
  return tpl.params.filter((p) => p.required && !(params[p.name] ?? '').trim()).map((p) => p.name)
}

/** Only the params the template takes, trimmed and without empty values. */
export function templateParams(tpl: ReportTemplate, params: Record<string, string | undefined>): Record<string, string> {
  const out: Record<string, string> = {}
  for (const p of tpl.params) {
    const v = (params[p.name] ?? '').trim()
    if (v) out[p.name] = v
  }
  return out
}

export interface CustomForm {
  entity: ReportEntity | ''
  query: string
  columns: string[]
  group_by: string
  sort_by: string
  sort_desc: boolean
  include_summary: boolean
  title: string
  run_id: string
}

export function blankCustomForm(entity: ReportEntity = 'requirement'): CustomForm {
  return { entity, query: '', columns: [], group_by: '', sort_by: '', sort_desc: false, include_summary: true, title: '', run_id: '' }
}

export type CustomProblem = 'entity' | 'columns' | 'run' | 'group_by' | 'sort_by' | 'title'

/** Validation problems of a custom report (empty when valid). `fields` = known column ids. */
export function validateCustom(f: CustomForm, fields?: readonly string[]): CustomProblem[] {
  const out: CustomProblem[] = []
  if (!f.entity || !(REPORT_ENTITIES as readonly string[]).includes(f.entity)) out.push('entity')
  if (!f.columns.length) out.push('columns')
  if (f.entity === 'execution' && !f.run_id.trim()) out.push('run')
  const known = fields && fields.length ? new Set(fields) : null
  if (known && f.group_by && !known.has(f.group_by)) out.push('group_by')
  if (known && f.sort_by && !known.has(f.sort_by)) out.push('sort_by')
  if (f.title.length > 200) out.push('title')
  return out
}

/** CustomConfig for the API from the builder form (empty options left out). */
export function buildCustomConfig(f: CustomForm): CustomConfig {
  const cfg: CustomConfig = { entity: (f.entity || 'requirement') as ReportEntity, columns: [...new Set(f.columns)] }
  const q = f.query.trim()
  if (q) cfg.query = q
  if (f.group_by) cfg.group_by = f.group_by
  if (f.sort_by) {
    cfg.sort_by = f.sort_by
    cfg.sort_desc = !!f.sort_desc
  }
  cfg.include_summary = !!f.include_summary
  const title = f.title.trim()
  if (title) cfg.title = title
  if (cfg.entity === 'execution' && f.run_id.trim()) cfg.run_id = f.run_id.trim()
  return cfg
}

/** Builder form from a stored config (unknown values dropped). */
export function formFromConfig(raw: unknown): CustomForm {
  const c = (raw && typeof raw === 'object' ? raw : {}) as Partial<Record<keyof CustomConfig, unknown>>
  const str = (v: unknown) => (typeof v === 'string' ? v : '')
  const entity = (REPORT_ENTITIES as readonly string[]).includes(str(c.entity)) ? (c.entity as ReportEntity) : 'requirement'
  return {
    entity,
    query: str(c.query),
    columns: Array.isArray(c.columns) ? c.columns.filter((x): x is string => typeof x === 'string') : [],
    group_by: str(c.group_by),
    sort_by: str(c.sort_by),
    sort_desc: c.sort_desc === true,
    include_summary: c.include_summary !== false,
    title: str(c.title),
    run_id: str(c.run_id),
  }
}

export interface GenerateRequest {
  template?: string
  params?: Record<string, string>
  custom?: CustomConfig
  format: ReportFormat
  archive?: boolean
  name?: string
  /** language of the report's texts: the current UI language */
  lang?: string
}

/**
 * Body of POST /projects/{pid}/reports/generate. `lang` (the current UI
 * language) makes the server produce the report in that language.
 */
export function generateBody(
  source: { template: string; params?: Record<string, string> } | { custom: CustomConfig },
  format: ReportFormat,
  archive = false,
  name?: string,
  lang?: string,
): GenerateRequest {
  const body: GenerateRequest = { format }
  if (lang) body.lang = lang
  if ('template' in source) {
    body.template = source.template
    if (source.params && Object.keys(source.params).length) body.params = source.params
  } else body.custom = source.custom
  if (archive) {
    body.archive = true
    const n = (name ?? '').trim()
    if (n) body.name = n
  }
  return body
}

/** Fallback filename of a downloaded report. */
export function reportFileName(base: string, format: ReportFormat, now: Date = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  const stamp = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}`
  const clean = (base || 'report').replace(/[^\w.-]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 60) || 'report'
  return `${clean}-${stamp}.${format}`
}

/** Toggle a column in the selection (appended at the end). */
export function toggleColumn(cols: readonly string[], id: string): string[] {
  return cols.includes(id) ? cols.filter((c) => c !== id) : [...cols, id]
}

/** Fields offered when the server cannot be asked (mirrors common columns). */
export const FALLBACK_FIELDS: Record<ReportEntity, string[]> = {
  requirement: ['key', 'title', 'obligation', 'type', 'priority', 'status', 'owner', 'coverage', 'version'],
  test: ['key', 'title', 'execution_type', 'priority', 'status', 'last_result', 'version'],
  run: ['name', 'status', 'environment', 'build', 'created_by', 'created_at'],
  execution: ['key', 'title', 'status', 'assignee', 'executed_by', 'duration_ms'],
}

// ---------------------------------------------------------------- batch

/** A row of the "several reports at once" list. */
export interface BatchRow {
  kind: 'template' | 'definition'
  id: string
  name: string
  description: string
  /** formats it can be rendered in */
  formats: readonly ReportFormat[]
  /** parameters that must be given (e.g. run_id) */
  required: string[]
}

/** Formats every selected row supports (all formats when nothing is selected). */
export function batchFormats(rows: Pick<BatchRow, 'formats'>[]): ReportFormat[] {
  return REPORT_FORMATS.filter((f) => rows.every((r) => r.formats.includes(f)))
}

/** The body of POST /projects/{pid}/reports/batch (`lang`: the UI language of the reports). */
export function batchBody(rows: BatchRow[], format: ReportFormat, archive: boolean, params: Record<string, string>, lang?: string) {
  const pick = (r: BatchRow) => Object.fromEntries(r.required.filter((n) => params[n]).map((n) => [n, params[n]]))
  return {
    templates: rows.filter((r) => r.kind === 'template').map((r) => ({ template: r.id, params: pick(r) })),
    definitions: rows.filter((r) => r.kind === 'definition').map((r) => r.id),
    format,
    archive,
    ...(lang ? { lang } : {}),
  }
}

/** Required parameters of the selected rows that have no value yet. */
export function batchMissing(rows: Pick<BatchRow, 'required'>[], params: Record<string, string>): string[] {
  return [...new Set(rows.flatMap((r) => r.required))].filter((n) => !params[n])
}
