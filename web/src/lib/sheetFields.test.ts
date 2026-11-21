import { describe, expect, it } from 'vitest'
import { applyValues, emptyValue, findCategories, findFolder, folderOptions, sheetFields, type FieldContext, type SheetRow } from './sheetFields'
import { isEditable, parseTSV, planPaste } from './sheet'
import type { Category, Folder } from './types'

const t = (key: string, p?: Record<string, unknown>) => (p ? `${key} ${JSON.stringify(p)}` : key)
const LABELS: Record<string, string> = { high: 'High', low: 'Low', medium: 'Medium', critical: 'Critical', draft: 'Draft', approved: 'Approved', review: 'In review', ready: 'Ready', functional: 'Functional', must: 'MUST', manual: 'Manual', automated: 'Automated' }
const enumLabel = (_g: string, v: string) => LABELS[v] ?? v

const folders: Folder[] = [
  { id: 'f1', project_id: 'p', parent_id: null, kind: 'requirement', name: 'Web', description: '', position: 0, created_at: '' },
  { id: 'f2', project_id: 'p', parent_id: 'f1', kind: 'requirement', name: 'Login', description: '', position: 0, created_at: '' },
]
const categories: Category[] = [
  { id: 'c1', project_id: 'p', scope: 'requirement', dimension: 'Area', name: 'Security', color: '', description: '' },
  { id: 'c2', project_id: 'p', scope: 'requirement', dimension: 'Area', name: 'UI', color: '', description: '' },
  { id: 'c3', project_id: 'p', scope: 'requirement', dimension: 'Risk', name: 'UI', color: '', description: '' },
]
const meta = {
  obligations: ['must', 'should'],
  requirement_types: ['functional', 'security'],
  priorities: ['critical', 'high', 'medium', 'low'],
  requirement_statuses: ['draft', 'review', 'approved', 'rejected'],
  test_statuses: ['draft', 'ready', 'review'],
}
const ctx = (over: Partial<FieldContext> = {}): FieldContext => ({ kind: 'requirement', t, enumLabel, meta, categories, folders, canWrite: true, canReview: true, ...over })
const row: SheetRow = { id: 'r1', key: 'R-1', title: 'Login', description: '', type: 'functional', priority: 'high', status: 'draft', obligation: '', tags: ['a'], category_ids: ['c1'], owner: '', source: '', folder_id: 'f2', version: 2 }
const byId = (fs: ReturnType<typeof sheetFields>, id: string) => fs.find((f) => f.id === id)!

describe('requirement fields', () => {
  const fs = sheetFields(ctx())
  it('has the spreadsheet columns, key and title first and fixed', () => {
    expect(fs.map((f) => f.id).slice(0, 12)).toEqual(['key', 'title', 'description', 'type', 'priority', 'status', 'obligation', 'category_ids', 'tags', 'owner', 'source', 'folder'])
    expect(fs.filter((f) => f.fixed).map((f) => f.id)).toEqual(['key', 'title'])
    expect(isEditable(byId(fs, 'key'), row)).toBe(false)
    expect(isEditable(byId(fs, 'coverage'), row)).toBe(false)
  })
  it('shows values as text', () => {
    expect(byId(fs, 'priority').text(row)).toBe('High')
    expect(byId(fs, 'category_ids').text(row)).toBe('Area/Security')
    expect(byId(fs, 'folder').text(row)).toBe('Web / Login')
    expect(byId(fs, 'tags').text(row)).toBe('a')
    expect(byId(fs, 'obligation').text(row)).toBe('')
  })
  it('parses enum values by value or label', () => {
    const p = byId(fs, 'priority')
    expect(p.parse!('low', row)).toEqual({ ok: true, value: 'low' })
    expect(p.parse!('Critical', row)).toEqual({ ok: true, value: 'critical' })
    const bad = p.parse!('urgent', row)
    expect(bad.ok).toBe(false)
    expect(!bad.ok && bad.error).toContain('sheet.err.invalidOption')
    expect(!bad.ok && bad.error).toContain('Critical, High, Medium, Low')
    expect(byId(fs, 'obligation').parse!('', row)).toEqual({ ok: true, value: '' })
    expect(byId(fs, 'obligation').parse!('must', row)).toEqual({ ok: true, value: 'must' })
  })
  it('parses titles, tags, categories and folders', () => {
    expect(byId(fs, 'title').parse!('  New  ', row)).toEqual({ ok: true, value: 'New' })
    expect(byId(fs, 'title').parse!(' ', row).ok).toBe(false)
    expect(byId(fs, 'title').parse!('x'.repeat(501), row).ok).toBe(false)
    expect(byId(fs, 'description').parse!('a\r\nb\n', row)).toEqual({ ok: true, value: 'a\nb' })
    expect(byId(fs, 'tags').parse!('x, y;x', row)).toEqual({ ok: true, value: ['x', 'y'] })
    expect(byId(fs, 'category_ids').parse!('Area/UI, security', row)).toEqual({ ok: true, value: ['c2', 'c1'] })
    // "UI" alone is ambiguous (two dimensions)
    expect(byId(fs, 'category_ids').parse!('UI', row).ok).toBe(false)
    expect(byId(fs, 'folder').parse!('web/login', row)).toEqual({ ok: true, value: 'f2' })
    expect(byId(fs, 'folder').parse!('Web / Login', row)).toEqual({ ok: true, value: 'f2' })
    expect(byId(fs, 'folder').parse!('f1', row)).toEqual({ ok: true, value: 'f1' })
    expect(byId(fs, 'folder').parse!('', row)).toEqual({ ok: true, value: null })
    expect(byId(fs, 'folder').parse!('common.topLevel', row)).toEqual({ ok: true, value: null })
    expect(byId(fs, 'folder').parse!('Nowhere', row).ok).toBe(false)
  })
  it('sorts enums by their order', () => {
    expect(byId(fs, 'priority').sortKey!(row)).toBe(1)
    expect(byId(fs, 'priority').sortKey!({ ...row, priority: 'odd' })).toBe('')
  })
})

describe('permissions', () => {
  it('a reader edits nothing', () => {
    const fs = sheetFields(ctx({ canWrite: false, canReview: false }))
    expect(fs.filter((f) => isEditable(f, row))).toEqual([])
  })
  it('a reviewer without write edits only the status, to review decisions', () => {
    const fs = sheetFields(ctx({ canWrite: false }))
    expect(fs.filter((f) => isEditable(f, row)).map((f) => f.id)).toEqual(['status'])
    const st = byId(fs, 'status')
    expect(st.parse!('approved', row)).toEqual({ ok: true, value: 'approved' })
    expect(st.parse!('review', row).ok).toBe(false)
    // the new row needs write
    expect(isEditable(st, null)).toBe(false)
  })
  it('a writer without review cannot approve, but keeps the current status', () => {
    const fs = sheetFields(ctx({ canReview: false }))
    const st = byId(fs, 'status')
    const bad = st.parse!('Approved', row)
    expect(!bad.ok && bad.error).toContain('sheet.err.reviewNeeded')
    expect(st.parse!('approved', { ...row, status: 'approved' })).toEqual({ ok: true, value: 'approved' })
    expect(st.options!(row).find((o) => o.value === 'approved')?.disabled).toBe(true)
    expect(st.options!(row).find((o) => o.value === 'review')?.disabled).toBe(false)
  })
  it('validates a pasted block with the permission rules', () => {
    const fs = sheetFields(ctx({ canReview: false }))
    const st = fs.findIndex((f) => f.id === 'status')
    const p = planPaste({ rows: [row], fields: fs, readOnly: () => 'ro' }, parseTSV('High\tApproved'), { r1: 0, c1: st - 1, r2: 0, c2: st - 1 })
    expect(p.changes).toEqual([])
    expect(p.errors.length).toBe(1)
  })
})

describe('test fields', () => {
  const fs = sheetFields(ctx({ kind: 'test' }))
  const test: SheetRow = { id: 't1', key: 'T-1', title: 'Log in', execution_type: 'manual', status: 'draft', priority: 'low', step_count: 3, estimated_minutes: 0, tags: null, category_ids: null, folder_id: null }
  it('has the test columns', () => {
    expect(fs.map((f) => f.id)).toEqual(['key', 'title', 'description', 'priority', 'status', 'execution_type', 'preconditions', 'expected_result', 'estimated_minutes', 'category_ids', 'tags', 'folder', 'step_count', 'last_result', 'linked_requirements', 'created_by', 'version'])
    expect(byId(fs, 'step_count').text(test)).toBe('3')
    expect(byId(fs, 'step_count').text({ ...test, step_count: undefined, steps: [{}, {}] })).toBe('2')
    expect(byId(fs, 'tags').text(test)).toBe('')
    expect(byId(fs, 'folder').text(test)).toBe('')
  })
  it('parses execution types and estimates', () => {
    expect(byId(fs, 'execution_type').parse!('Automated', test)).toEqual({ ok: true, value: 'automated' })
    expect(byId(fs, 'execution_type').parse!('robot', test).ok).toBe(false)
    expect(byId(fs, 'estimated_minutes').parse!(' 15 ', test)).toEqual({ ok: true, value: 15 })
    expect(byId(fs, 'estimated_minutes').parse!('', test)).toEqual({ ok: true, value: 0 })
    expect(byId(fs, 'estimated_minutes').parse!('-1', test).ok).toBe(false)
    expect(byId(fs, 'estimated_minutes').text(test)).toBe('')
  })
  it('ready is the review decision of tests', () => {
    const w = sheetFields(ctx({ kind: 'test', canReview: false }))
    expect(byId(w, 'status').parse!('ready', test).ok).toBe(false)
    expect(byId(w, 'status').parse!('review', test).ok).toBe(true)
  })
})

describe('helpers', () => {
  it('finds folders and categories', () => {
    expect(findFolder(folders, ' Web ')).toEqual({ ok: true, id: 'f1' })
    expect(findFolder(folders, 'Login')).toEqual({ ok: false })
    expect(findCategories(categories, '')).toEqual({ ok: true, ids: [] })
    expect(findCategories(categories, 'Risk / UI')).toEqual({ ok: true, ids: ['c3'] })
    expect(findCategories(categories, 'Nope')).toEqual({ ok: false, unknown: 'Nope' })
    expect(folderOptions(folders, 'Top').map((o) => o.label)).toEqual(['Top', 'Web', 'Web / Login'])
  })
  it('empties and applies values', () => {
    const fs = sheetFields(ctx())
    expect(emptyValue(byId(fs, 'tags'))).toEqual([])
    expect(emptyValue(byId(fs, 'folder'))).toBeNull()
    expect(emptyValue(byId(fs, 'owner'))).toBe('')
    expect(applyValues(row, { folder_id: 'f1', title: 'X' }, folders)).toMatchObject({ folder_id: 'f1', folder_path: 'Web', title: 'X' })
    expect(applyValues(row, { steps: [1, 2] }, folders).step_count).toBe(2)
  })
})

describe('read-only and computed columns', () => {
  it('show and sort counts, results and versions', () => {
    const rq = sheetFields(ctx())
    const r: SheetRow = { ...row, coverage: 'passed', linked_tests: 3, version: 7 }
    expect(['coverage', 'linked_tests', 'version'].map((id) => byId(rq, id).text(r))).toEqual(['passed', '3', '7'])
    expect(byId(rq, 'linked_tests').sortKey!(r)).toBe(3)
    expect(byId(rq, 'version').sortKey!({ ...r, version: undefined })).toBe(0)
    expect(byId(rq, 'folder').sortKey!(r)).toBe('Web / Login')
    expect(byId(rq, 'owner').value!({ ...r, owner: null })).toBe('')
    const ts = sheetFields(ctx({ kind: 'test' }))
    const tc: SheetRow = { id: 't', key: 'T-1', last_result: 'passed', linked_requirements: 2, created_by: 'a@b.c', estimated_minutes: 5, version: 1 }
    expect(['last_result', 'linked_requirements', 'created_by', 'estimated_minutes', 'version'].map((id) => byId(ts, id).text(tc))).toEqual(['passed', '2', 'a@b.c', '5', '1'])
    expect(byId(ts, 'estimated_minutes').sortKey!(tc)).toBe(5)
    expect(byId(ts, 'estimated_minutes').value!(tc)).toBe(5)
    expect(byId(ts, 'linked_requirements').sortKey!(tc)).toBe(2)
    expect(byId(ts, 'step_count').sortKey!({ ...tc, step_count: 4 })).toBe(4)
    expect(byId(ts, 'execution_type').sortKey!({ ...tc, execution_type: 'automated' })).toBe(1)
    expect(byId(ts, 'execution_type').options!(null).map((o) => o.label)).toEqual(['Manual', 'Automated'])
  })
  it('labels the empty obligation and lists category options', () => {
    const rq = sheetFields(ctx())
    expect(byId(rq, 'obligation').options!(row)[0]).toEqual({ value: '', label: 'sheet.autoObligation' })
    expect(byId(rq, 'category_ids').options!(row).map((o) => o.label)).toEqual(['Area/Security', 'Area/UI', 'Risk/UI'])
    expect(byId(rq, 'folder').options!(row)[0].value).toBe('')
    expect(byId(rq, 'tags').parse!('x'.repeat(65), row).ok).toBe(false)
    expect(byId(rq, 'owner').parse!(' anna \n', row)).toEqual({ ok: true, value: 'anna' })
  })
})
