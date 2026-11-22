// Custom fields (GET /projects/{pid}/custom-fields) and item templates
// (GET /projects/{pid}/templates): types, the display text of values, the
// parsing of typed or pasted text (spreadsheet view), list columns, and
// applying a template to the form of a new requirement or test. The server
// validates every value again (internal/customfield).

export type CustomFieldEntity = 'requirement' | 'test' | 'defect' | 'risk' | 'run'
export type CustomFieldType = 'text' | 'long_text' | 'number' | 'date' | 'single_select' | 'multi_select' | 'checkbox' | 'user' | 'url'

export const CUSTOM_FIELD_ENTITIES: CustomFieldEntity[] = ['requirement', 'test', 'defect', 'risk', 'run']
export const CUSTOM_FIELD_TYPES: CustomFieldType[] = ['text', 'long_text', 'number', 'date', 'single_select', 'multi_select', 'checkbox', 'user', 'url']

/** A value: text, number, checkbox or the options of a multi select; null removes it. */
export type CustomValue = string | number | boolean | string[] | null
export type CustomValues = Record<string, CustomValue>

export interface CustomField {
  id: string
  project_id?: string
  project_type?: string
  entity_type: CustomFieldEntity
  key: string
  label: string
  type: CustomFieldType
  unit: string
  options: string[]
  required: boolean
  default: CustomValue
  help: string
  position: number
  show_in_list: boolean
  created_at?: string
  updated_at?: string
}

export interface CustomFieldArchive {
  id: string
  entity_type: CustomFieldEntity
  field_key: string
  label: string
  item_count: number
  deleted_by: string
  deleted_at: string
  values?: { id: string; key: string; value: CustomValue }[]
}

export type TemplateKind = 'requirement' | 'test'
export type TemplateSource = 'project' | 'organisation' | 'builtin'

export interface TemplateContent {
  title?: string
  description?: string
  priority?: string
  status?: string
  tags?: string[]
  obligation?: string
  type?: string
  source?: string
  rationale?: string
  criteria?: { description: string; method: string; threshold: string }[]
  preconditions?: string
  expected_result?: string
  execution_type?: string
  estimated_minutes?: number
  steps?: { action: string; expected: string; data: string }[]
  custom_fields?: CustomValues
}

export interface ItemTemplate {
  id: string
  project_id?: string
  project_type?: string
  kind: TemplateKind
  name: string
  description: string
  source: TemplateSource
  content: TemplateContent
  position: number
  default?: boolean
}

/** The query field of a custom field. */
export const cfField = (key: string) => `cf.${key}`
export const isCfField = (field: string) => field.startsWith('cf.')
export const cfKey = (field: string) => field.slice(3)

/** The fields of one kind in their order. */
export function fieldsOf(fields: readonly CustomField[], entity: CustomFieldEntity): CustomField[] {
  return fields.filter((f) => f.entity_type === entity).sort((a, b) => a.position - b.position || a.key.localeCompare(b.key))
}

/** A label with its unit: "Mass (kg)". */
export function fieldLabel(f: Pick<CustomField, 'label' | 'unit'>): string {
  return f.unit ? `${f.label} (${f.unit})` : f.label
}

/** Whether a value counts as empty (not stored). */
export function isEmpty(v: unknown): boolean {
  return v === null || v === undefined || v === '' || (Array.isArray(v) && v.length === 0)
}

/** The display text of a value; yes/no texts come from the caller. */
export function valueText(f: CustomField, v: unknown, yesNo: [string, string] = ['Yes', 'No']): string {
  if (isEmpty(v)) return ''
  switch (f.type) {
    case 'checkbox':
      return v ? yesNo[0] : yesNo[1]
    case 'multi_select':
      return Array.isArray(v) ? v.join(', ') : String(v)
    case 'number':
      return f.unit ? `${v} ${f.unit}` : String(v)
  }
  return String(v)
}

/** A sort key of a value: numbers as numbers, the rest as text. */
export function sortKey(f: CustomField, v: unknown): string | number {
  if (isEmpty(v)) return ''
  if (f.type === 'number' && typeof v === 'number') return v
  if (f.type === 'checkbox') return v ? 1 : 0
  return Array.isArray(v) ? v.join(', ').toLowerCase() : String(v).toLowerCase()
}

export type ParseResult = { ok: true; value: CustomValue } | { ok: false; error: 'number' | 'date' | 'option' | 'checkbox' | 'email' | 'url' | 'tooLong'; detail?: string }

const YES = ['true', 'yes', 'y', '1', 'x', 'ja', 'j', 'sant', '✓']
const NO = ['false', 'no', 'n', '0', 'nej', 'falskt']

/** Splits a multi-select text (comma, semicolon or one per line). */
export function splitOptions(s: string): string[] {
  return s
    .split(/[,;\n]/)
    .map((x) => x.trim())
    .filter(Boolean)
}

/** Parses typed or pasted text into the value of a field; empty text is null. */
export function parseValue(f: CustomField, raw: string, yesNo: [string, string] = ['Yes', 'No']): ParseResult {
  const s = raw.trim()
  if (!s) return { ok: true, value: f.type === 'checkbox' ? false : null }
  switch (f.type) {
    case 'text':
      return s.length > 1000 ? { ok: false, error: 'tooLong' } : { ok: true, value: s }
    case 'long_text':
      return raw.length > 100000 ? { ok: false, error: 'tooLong' } : { ok: true, value: raw.replace(/\r\n?/g, '\n') }
    case 'number': {
      let n = s
      if (f.unit && n.endsWith(f.unit)) n = n.slice(0, -f.unit.length).trim()
      n = n.replace(/[\s ]/g, '')
      if ((n.match(/,/g) ?? []).length === 1 && !n.includes('.')) n = n.replace(',', '.')
      const v = Number(n)
      return n !== '' && Number.isFinite(v) ? { ok: true, value: v } : { ok: false, error: 'number', detail: s }
    }
    case 'date':
      return /^\d{4}-\d{2}-\d{2}$/.test(s) && !Number.isNaN(Date.parse(s + 'T00:00:00Z')) && new Date(s + 'T00:00:00Z').toISOString().startsWith(s)
        ? { ok: true, value: s }
        : { ok: false, error: 'date', detail: s }
    case 'single_select': {
      const o = f.options.find((x) => x.toLowerCase() === s.toLowerCase())
      return o ? { ok: true, value: o } : { ok: false, error: 'option', detail: s }
    }
    case 'multi_select': {
      const out: string[] = []
      for (const part of splitOptions(s)) {
        const o = f.options.find((x) => x.toLowerCase() === part.toLowerCase())
        if (!o) return { ok: false, error: 'option', detail: part }
        if (!out.includes(o)) out.push(o)
      }
      out.sort((a, b) => f.options.indexOf(a) - f.options.indexOf(b))
      return { ok: true, value: out }
    }
    case 'checkbox': {
      const l = s.toLowerCase()
      if (YES.includes(l) || l === yesNo[0].toLowerCase()) return { ok: true, value: true }
      if (NO.includes(l) || l === yesNo[1].toLowerCase()) return { ok: true, value: false }
      return { ok: false, error: 'checkbox', detail: s }
    }
    case 'user':
      return /^[^\s@]+@[^\s@]+$/.test(s) ? { ok: true, value: s.toLowerCase() } : { ok: false, error: 'email', detail: s }
    case 'url':
      return /^https?:\/\/[^\s/]+/i.test(s) ? { ok: true, value: s } : { ok: false, error: 'url', detail: s }
  }
  return { ok: true, value: s }
}

/** Values a new item starts with: the defaults of its fields. */
export function defaultValues(fields: readonly CustomField[]): CustomValues {
  const out: CustomValues = {}
  for (const f of fields) if (!isEmpty(f.default)) out[f.key] = Array.isArray(f.default) ? [...f.default] : f.default
  return out
}

/** Labels of the required fields that have no value. */
export function missingRequired(fields: readonly CustomField[], values: CustomValues | undefined): string[] {
  return fields.filter((f) => f.required && f.type !== 'checkbox' && isEmpty(values?.[f.key])).map((f) => f.label)
}

/** The keys of a definition derived from its label, as the server does. */
export function keyFromLabel(label: string): string {
  let k = label
    .toLowerCase()
    .replace(/[åä]/g, 'a')
    .replace(/ö/g, 'o')
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
  if (!/^[a-z]/.test(k)) k = 'f_' + k
  return k.slice(0, 40).replace(/_+$/, '')
}

// ---------------------------------------------------------------- templates

/** The form fields a requirement or test template fills in. */
export interface TemplateForm {
  title: string
  description: string
  priority: string
  status: string
  tags: string[]
  obligation?: string
  type?: string
  source?: string
  rationale?: string
  criteria?: { description: string; method: string; threshold: string; id?: string }[]
  preconditions?: string
  expected_result?: string
  execution_type?: string
  estimated_minutes?: number | null
  steps?: { action: string; expected: string; data: string; [k: string]: unknown }[]
  custom_fields?: CustomValues
}

/**
 * Applies a template to the form of a new item: every field the template
 * sets is replaced, tags are added, and custom values are taken for the
 * fields the project has.
 */
export function applyTemplate<F extends TemplateForm>(form: F, c: TemplateContent, fields: readonly CustomField[]): F {
  const out: F = { ...form }
  const set = <K extends keyof TemplateForm>(k: K, v: TemplateForm[K] | undefined) => {
    if (v !== undefined && v !== '' && k in form) (out as TemplateForm)[k] = v
  }
  set('title', c.title)
  set('description', c.description)
  set('priority', c.priority)
  set('status', c.status)
  set('obligation', c.obligation)
  set('type', c.type)
  set('source', c.source)
  set('rationale', c.rationale)
  set('preconditions', c.preconditions)
  set('expected_result', c.expected_result)
  set('execution_type', c.execution_type)
  if (c.estimated_minutes && 'estimated_minutes' in form) out.estimated_minutes = c.estimated_minutes
  if (c.criteria?.length && 'criteria' in form) out.criteria = c.criteria.map((x) => ({ description: x.description, method: x.method || 'test', threshold: x.threshold ?? '' }))
  if (c.steps?.length && 'steps' in form) out.steps = c.steps.map((x) => ({ action: x.action ?? '', expected: x.expected ?? '', data: x.data ?? '' }))
  if (c.tags?.length) out.tags = [...new Set([...(form.tags ?? []), ...c.tags])]
  if (c.custom_fields) {
    const keys = new Set(fields.map((f) => f.key))
    const cf: CustomValues = { ...(form.custom_fields ?? {}) }
    for (const [k, v] of Object.entries(c.custom_fields)) if (keys.has(k)) cf[k] = Array.isArray(v) ? [...v] : v
    out.custom_fields = cf
  }
  return out
}

/** The template content of an item's form (to save an item as a template). */
export function contentOf(kind: TemplateKind, f: TemplateForm): TemplateContent {
  const c: TemplateContent = { title: f.title, description: f.description, priority: f.priority, status: f.status, tags: f.tags }
  if (kind === 'requirement') {
    Object.assign(c, { obligation: f.obligation, type: f.type, source: f.source, rationale: f.rationale, criteria: f.criteria?.map(({ description, method, threshold }) => ({ description, method, threshold })) })
  } else {
    Object.assign(c, { preconditions: f.preconditions, expected_result: f.expected_result, execution_type: f.execution_type, estimated_minutes: f.estimated_minutes, steps: f.steps?.map(({ action, expected, data }) => ({ action, expected, data })) })
  }
  const cf = Object.fromEntries(Object.entries(f.custom_fields ?? {}).filter(([, v]) => !isEmpty(v)))
  if (Object.keys(cf).length) c.custom_fields = cf
  return c
}

/** Templates grouped for a picker: project, organisation, built-in. */
export function groupTemplates(list: readonly ItemTemplate[]): { source: TemplateSource; items: ItemTemplate[] }[] {
  const order: TemplateSource[] = ['project', 'organisation', 'builtin']
  return order.map((source) => ({ source, items: list.filter((t) => t.source === source) })).filter((g) => g.items.length)
}
