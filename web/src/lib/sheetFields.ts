// The columns of the spreadsheet view of requirements and tests: how each
// field shows as text (display, copy, export), sorts, parses typed or
// pasted text (enum values by value or label, categories as Dimension/Name,
// folders by path) and who may edit it (the RBAC rules of the editors:
// writing needs requirement.write / test.write, a review decision also the
// review permission, and a reviewer may change the status alone).
import type { Category, Folder, ItemKind } from './types'
import { buildTree, flatten, folderPath } from './tree'
import { matchOption, splitList, type Parsed, type SheetField, type SheetOption } from './sheet'
import { cfField, fieldLabel, isEmpty, parseValue, sortKey as cfSortKey, valueText, type CustomField } from './customFields'

/** A requirement or test as the list endpoints return it. */
export type SheetRow = Record<string, unknown> & { id: string; key: string; version?: number }

export type Translate = (key: string, params?: Record<string, unknown>) => string
export type EnumLabel = (group: 'status' | 'priority' | 'obligation' | 'requirementType' | 'executionType' | 'coverage' | 'result', value: string) => string

export interface FieldContext {
  kind: ItemKind
  t: Translate
  enumLabel: EnumLabel
  meta: { obligations: string[]; requirement_types: string[]; priorities: string[]; requirement_statuses: string[]; test_statuses: string[] }
  categories: readonly Category[]
  folders: readonly Folder[]
  canWrite: boolean
  canReview: boolean
  /** the project's custom fields of the kind (columns cf.<key>) */
  customFields?: readonly CustomField[]
}

/** Statuses that are review decisions (internal/api/review.go). */
export const REVIEW_DECISIONS: Record<ItemKind, string[]> = { requirement: ['approved', 'rejected'], test: ['ready'] }

/** Columns shown when nothing was chosen. */
export const DEFAULT_SHEET_COLUMNS: Record<ItemKind, string[]> = {
  requirement: ['description', 'type', 'priority', 'status', 'obligation', 'category_ids', 'tags', 'owner', 'source', 'folder'],
  test: ['description', 'priority', 'status', 'execution_type', 'preconditions', 'expected_result', 'category_ids', 'tags', 'folder', 'step_count'],
}

const str = (v: unknown) => (v === null || v === undefined ? '' : String(v))
const list = (v: unknown) => (Array.isArray(v) ? (v as unknown[]).map(String) : [])

export function categoryLabel(c: Category): string {
  return `${c.dimension}/${c.name}`
}

/** Folder choices: top level, then every folder by its full path. */
export function folderOptions(folders: readonly Folder[], topLevel: string): SheetOption[] {
  return [{ value: '', label: topLevel }, ...flatten(buildTree(folders)).map((f) => ({ value: f.id, label: folderPath(folders, f.id) }))]
}

/** The folder of a typed path ("A / B", "A/B"; empty is the top level). */
export function findFolder(folders: readonly Folder[], text: string): { ok: true; id: string | null } | { ok: false } {
  const want = text.split('/').map((s) => s.trim().toLowerCase()).filter(Boolean).join('/')
  if (!want) return { ok: true, id: null }
  const f = folders.find((x) => folderPath(folders, x.id, '/').toLowerCase() === want)
  return f ? { ok: true, id: f.id } : { ok: false }
}

/** Categories of typed text ("Dimension/Name, …"; a name alone when it is unique). */
export function findCategories(categories: readonly Category[], text: string): { ok: true; ids: string[] } | { ok: false; unknown: string } {
  const ids: string[] = []
  for (const part of splitList(text)) {
    const p = part.toLowerCase()
    let c = categories.find((x) => categoryLabel(x).toLowerCase() === p || `${x.dimension} / ${x.name}`.toLowerCase() === p)
    if (!c) {
      const byName = categories.filter((x) => x.name.toLowerCase() === p)
      if (byName.length === 1) c = byName[0]
    }
    if (!c) return { ok: false, unknown: part }
    if (!ids.includes(c.id)) ids.push(c.id)
  }
  return { ok: true, ids }
}

/** The spreadsheet columns of a kind, key and title first. */
export function sheetFields(ctx: FieldContext): SheetField<SheetRow>[] {
  const { t, enumLabel } = ctx
  const isReq = ctx.kind === 'requirement'
  const write = () => ctx.canWrite
  const ok = (value: unknown): Parsed => ({ ok: true, value })
  const fail = (error: string): Parsed => ({ ok: false, error })

  const text = (id: string, header: string, width: number, opts: { multiline?: boolean; required?: boolean; max?: number; trim?: boolean } = {}): SheetField<SheetRow> => ({
    id,
    header,
    width,
    editor: opts.multiline ? 'textarea' : 'text',
    clearable: !opts.required,
    text: (r) => str(r[id]),
    value: (r) => str(r[id]),
    editable: write,
    parse: (raw) => {
      let v = opts.multiline ? raw.replace(/\r\n?/g, '\n') : raw.replace(/[\r\n]+/g, ' ')
      if (opts.trim !== false) v = opts.multiline ? v.replace(/\s+$/, '') : v.trim()
      if (opts.required && !v.trim()) return fail(t('sheet.err.required', { field: header }))
      if (opts.max && v.length > opts.max) return fail(t('sheet.err.tooLong', { field: header, max: opts.max }))
      return ok(v)
    },
  })

  const enumField = (id: string, header: string, width: number, group: Parameters<EnumLabel>[0], values: () => string[], extra: Partial<SheetField<SheetRow>> = {}): SheetField<SheetRow> => {
    const options = (): SheetOption[] => values().map((v) => ({ value: v, label: v === '' ? t('sheet.autoObligation') : enumLabel(group, v) }))
    const order = () => values()
    return {
      id,
      header,
      width,
      editor: 'select',
      text: (r) => (str(r[id]) === '' && group === 'obligation' ? '' : enumLabel(group, str(r[id]))),
      sortKey: (r) => {
        const i = order().indexOf(str(r[id]))
        return i < 0 ? '' : i
      },
      value: (r) => str(r[id]),
      editable: write,
      options,
      parse: (raw, row) => {
        const opts = (extra.options ?? options)(row)
        if (!raw.trim() && group === 'obligation') return ok('')
        const o = matchOption(opts, raw)
        if (!o) return fail(t('sheet.err.invalidOption', { value: raw.trim(), field: header, options: opts.filter((x) => !x.disabled).map((x) => x.label).join(', ') }))
        if (o.disabled && o.value !== str(row?.[id])) return fail(t('sheet.err.reviewNeeded', { value: o.label }))
        return ok(o.value)
      },
      ...extra,
    }
  }

  const statuses = () => (isReq ? ctx.meta.requirement_statuses : ctx.meta.test_statuses)
  const decisions = REVIEW_DECISIONS[ctx.kind]
  /** a status the user may set on the row: a review decision needs review, any other change write */
  const allowed = (row: SheetRow | null, s: string) => (row && s === str(row.status)) || (decisions.includes(s) ? ctx.canReview : ctx.canWrite)
  const status = enumField('status', t('sheet.col.status'), 130, 'status', statuses, {
    editable: (row) => (row === null ? ctx.canWrite : ctx.canWrite || ctx.canReview),
    options: (row) => statuses().map((v) => ({ value: v, label: enumLabel('status', v), disabled: !allowed(row, v) })),
  })

  const key: SheetField<SheetRow> = { id: 'key', header: t('sheet.col.key'), width: 120, fixed: true, editor: 'none', text: (r) => str(r.key) }
  const title = { ...text('title', t('sheet.col.title'), 280, { required: true, max: 500 }), fixed: true }
  const description = text('description', t('sheet.col.description'), 320, { multiline: true })
  const priority = enumField('priority', t('sheet.col.priority'), 110, 'priority', () => ctx.meta.priorities)

  const tags: SheetField<SheetRow> = {
    id: 'tags',
    header: t('sheet.col.tags'),
    width: 180,
    editor: 'tags',
    clearable: true,
    text: (r) => list(r.tags).join(', '),
    value: (r) => list(r.tags),
    editable: write,
    parse: (raw) => {
      const v = splitList(raw)
      const bad = v.find((x) => x.length > 64)
      return bad ? fail(t('sheet.err.tooLong', { field: t('sheet.col.tags'), max: 64 })) : ok(v)
    },
  }

  const catLabels = () => new Map(ctx.categories.map((c) => [c.id, categoryLabel(c)]))
  const categories: SheetField<SheetRow> = {
    id: 'category_ids',
    header: t('sheet.col.categories'),
    width: 200,
    editor: 'categories',
    clearable: true,
    text: (r) => {
      const m = catLabels()
      return list(r.category_ids)
        .map((id) => m.get(id))
        .filter(Boolean)
        .join(', ')
    },
    value: (r) => list(r.category_ids),
    editable: write,
    options: () => ctx.categories.map((c) => ({ value: c.id, label: categoryLabel(c) })),
    parse: (raw) => {
      const res = findCategories(ctx.categories, raw)
      return res.ok ? ok(res.ids) : fail(t('sheet.err.unknownCategory', { value: res.unknown }))
    },
  }

  const folder: SheetField<SheetRow> = {
    id: 'folder',
    api: 'folder_id',
    header: t('sheet.col.folder'),
    width: 200,
    editor: 'select',
    clearable: true,
    text: (r) => folderPath(ctx.folders as Folder[], (r.folder_id as string | null) ?? null),
    value: (r) => (r.folder_id as string | null) ?? null,
    editable: write,
    options: () => folderOptions(ctx.folders, t('common.topLevel')),
    parse: (raw) => {
      if (raw.trim() && raw.trim() === t('common.topLevel')) return ok(null)
      const byId = ctx.folders.find((f) => f.id === raw)
      if (byId) return ok(byId.id)
      const res = findFolder(ctx.folders, raw)
      return res.ok ? ok(res.id) : fail(t('sheet.err.unknownFolder', { value: raw.trim() }))
    },
  }

  const readonly = (id: string, header: string, width: number, textOf: (r: SheetRow) => string, sortKey?: (r: SheetRow) => string | number): SheetField<SheetRow> => ({
    id,
    header,
    width,
    editor: 'none',
    text: textOf,
    sortKey,
  })
  const count = (id: string) => (r: SheetRow) => Number(r[id] ?? 0)

  const yesNo: [string, string] = [t('common.yes'), t('common.no')]
  const custom = (f: CustomField): SheetField<SheetRow> => {
    const header = fieldLabel(f)
    const cur = (r: SheetRow) => (r.custom_fields as Record<string, unknown> | undefined)?.[f.key]
    const editor = f.type === 'long_text' ? 'textarea' : f.type === 'single_select' || f.type === 'checkbox' ? 'select' : 'text'
    return {
      id: cfField(f.key),
      header,
      width: f.type === 'long_text' ? 260 : 160,
      editor,
      clearable: !f.required,
      text: (r) => valueText(f, cur(r), yesNo),
      sortKey: (r) => cfSortKey(f, cur(r)),
      value: (r) => cur(r) ?? null,
      editable: write,
      options:
        editor === 'select'
          ? () => (f.type === 'checkbox' ? [{ value: 'true', label: yesNo[0] }, { value: 'false', label: yesNo[1] }] : f.options.map((o) => ({ value: o, label: o })))
          : undefined,
      parse: (raw) => {
        const p = parseValue(f, raw, yesNo)
        if (!p.ok) return fail(t(`customFields.err.${p.error}`, { field: header, value: p.detail ?? '', options: f.options.join(', ') }))
        if (f.required && isEmpty(p.value)) return fail(t('sheet.err.required', { field: header }))
        return ok(p.value)
      },
    }
  }
  const customs = (ctx.customFields ?? []).map(custom)

  if (isReq) {
    return [
      key,
      title,
      description,
      enumField('type', t('sheet.col.type'), 140, 'requirementType', () => ctx.meta.requirement_types),
      priority,
      status,
      enumField('obligation', t('sheet.col.obligation'), 130, 'obligation', () => ['', ...ctx.meta.obligations], { clearable: true }),
      categories,
      tags,
      text('owner', t('sheet.col.owner'), 150),
      text('source', t('sheet.col.source'), 160),
      { ...folder, sortKey: folder.text },
      readonly('coverage', t('sheet.col.coverage'), 120, (r) => enumLabel('coverage', str(r.coverage))),
      readonly('linked_tests', t('sheet.col.linkedTests'), 100, (r) => str(r.linked_tests ?? 0), count('linked_tests')),
      readonly('version', t('sheet.col.version'), 80, (r) => str(r.version ?? ''), count('version')),
      ...customs,
    ]
  }
  return [
    key,
    title,
    description,
    priority,
    status,
    enumField('execution_type', t('sheet.col.execution'), 130, 'executionType', () => ['manual', 'automated']),
    text('preconditions', t('sheet.col.preconditions'), 260, { multiline: true }),
    text('expected_result', t('sheet.col.expected'), 260, { multiline: true }),
    {
      id: 'estimated_minutes',
      header: t('sheet.col.estimate'),
      width: 110,
      editor: 'text',
      clearable: true,
      text: (r) => (Number(r.estimated_minutes ?? 0) ? str(r.estimated_minutes) : ''),
      sortKey: count('estimated_minutes'),
      value: (r) => Number(r.estimated_minutes ?? 0),
      editable: write,
      parse: (raw) => {
        const v = raw.trim()
        if (!v) return ok(0)
        return /^\d{1,6}$/.test(v) ? ok(Number(v)) : fail(t('sheet.err.notMinutes', { value: v }))
      },
    },
    categories,
    tags,
    { ...folder, sortKey: folder.text },
    readonly('step_count', t('sheet.col.steps'), 90, (r) => str(r.step_count ?? (Array.isArray(r.steps) ? (r.steps as unknown[]).length : 0)), count('step_count')),
    readonly('last_result', t('sheet.col.lastResult'), 120, (r) => enumLabel('result', str(r.last_result))),
    readonly('linked_requirements', t('sheet.col.linkedRequirements'), 120, (r) => str(r.linked_requirements ?? 0), count('linked_requirements')),
    readonly('created_by', t('sheet.col.createdBy'), 180, (r) => str(r.created_by)),
    readonly('version', t('sheet.col.version'), 80, (r) => str(r.version ?? ''), count('version')),
    ...customs,
  ]
}

/** The empty value a cleared cell gets. */
export function emptyValue(f: SheetField<SheetRow>): unknown {
  if (f.id.startsWith('cf.')) return null
  if (f.editor === 'tags' || f.editor === 'categories') return []
  if (f.id === 'folder') return null
  if (f.id === 'estimated_minutes') return 0
  return ''
}

/** Apply saved values to a row locally (the folder also updates its path). */
export function applyValues(row: SheetRow, set: Record<string, unknown>, folders: readonly Folder[]): SheetRow {
  const out = { ...row, ...set }
  if (set.custom_fields && typeof set.custom_fields === 'object') {
    // merged like the server does: null removes a value
    const cf: Record<string, unknown> = { ...((row.custom_fields as Record<string, unknown> | undefined) ?? {}) }
    for (const [k, v] of Object.entries(set.custom_fields as Record<string, unknown>)) {
      if (isEmpty(v)) delete cf[k]
      else cf[k] = v
    }
    out.custom_fields = cf
  }
  if ('folder_id' in set) out.folder_path = folderPath(folders as Folder[], (set.folder_id as string | null) ?? null)
  if ('steps' in set && Array.isArray(set.steps)) out.step_count = set.steps.length
  return out
}
