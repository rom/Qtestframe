import { describe, expect, it } from 'vitest'
import {
  batchBody,
  batchFormats,
  batchMissing,
  blankCustomForm,
  buildCustomConfig,
  formFromConfig,
  generateBody,
  missingParams,
  normalizeTemplate,
  reportFileName,
  templateParams,
  toggleColumn,
  validateCustom,
  type ReportFormat,
} from './reports'

describe('normalizeTemplate', () => {
  it('accepts name lists, objects and maps', () => {
    expect(normalizeTemplate({ id: 'run_summary', params: ['run_id'] })?.params).toEqual([{ name: 'run_id', required: true }])
    expect(normalizeTemplate({ id: 'x', params: ['query', 'run_id?', ''] })?.params).toEqual([
      { name: 'query', required: false },
      { name: 'run_id', required: false },
    ])
    expect(normalizeTemplate({ id: 'x', params: [{ name: 'run_id' }, { name: 'query', required: true }, 5] })?.params).toEqual([
      { name: 'run_id', required: true },
      { name: 'query', required: true },
    ])
    expect(normalizeTemplate({ id: 'x', params: { run_id: 'required', query: 'optional' } })?.params).toEqual([
      { name: 'run_id', required: true },
      { name: 'query', required: false },
    ])
  })
  it('uses defaults for known templates and all formats', () => {
    const t = normalizeTemplate({ id: 'requirements_coverage' })!
    expect(t.params).toEqual([{ name: 'query', required: false }])
    expect(t.formats).toEqual(['pdf', 'docx', 'xlsx', 'csv'])
    expect(t.name).toBe('requirements_coverage')
    expect(normalizeTemplate({ id: 'defects', name: 'Defects', description: 'd', formats: ['csv', 'bogus'] })).toEqual({ id: 'defects', name: 'Defects', description: 'd', params: [], formats: ['csv'] })
  })
  it('rejects garbage', () => {
    expect(normalizeTemplate(null)).toBeNull()
    expect(normalizeTemplate({})).toBeNull()
  })
})

describe('template params', () => {
  const tpl = normalizeTemplate({ id: 'run_summary', params: ['run_id', 'query?'] })!
  it('finds missing required params', () => {
    expect(missingParams(tpl, {})).toEqual(['run_id'])
    expect(missingParams(tpl, { run_id: ' r ' })).toEqual([])
  })
  it('keeps only known non-empty params', () => {
    expect(templateParams(tpl, { run_id: ' r ', query: '', other: 'x' })).toEqual({ run_id: 'r' })
  })
})

describe('custom reports', () => {
  it('validates', () => {
    const f = blankCustomForm()
    expect(validateCustom(f)).toEqual(['columns'])
    expect(validateCustom({ ...f, entity: '', columns: ['key'] })).toEqual(['entity'])
    expect(validateCustom({ ...f, entity: 'execution', columns: ['key'] })).toEqual(['run'])
    expect(validateCustom({ ...f, columns: ['key'], group_by: 'x', sort_by: 'y', title: 'x'.repeat(201) }, ['key'])).toEqual(['group_by', 'sort_by', 'title'])
    expect(validateCustom({ ...f, columns: ['key'], group_by: 'x' }, [])).toEqual([])
  })
  it('builds a config without empty options', () => {
    expect(buildCustomConfig({ ...blankCustomForm(), columns: ['key', 'key', 'title'] })).toEqual({ entity: 'requirement', columns: ['key', 'title'], include_summary: true })
    expect(
      buildCustomConfig({ entity: 'execution', query: ' status:failed ', columns: ['key'], group_by: 'status', sort_by: 'key', sort_desc: true, include_summary: false, title: ' T ', run_id: ' r1 ' }),
    ).toEqual({ entity: 'execution', query: 'status:failed', columns: ['key'], group_by: 'status', sort_by: 'key', sort_desc: true, include_summary: false, title: 'T', run_id: 'r1' })
    expect(buildCustomConfig({ ...blankCustomForm('test'), entity: '', columns: ['k'], run_id: 'r' }).entity).toBe('requirement')
  })
  it('round-trips through formFromConfig', () => {
    const cfg = { entity: 'test', query: 'q', columns: ['key', 7], group_by: 'status', sort_by: 'key', sort_desc: true, include_summary: false, title: 'T', run_id: '' }
    const f = formFromConfig(cfg)
    expect(f.columns).toEqual(['key'])
    expect(f.entity).toBe('test')
    expect(f.include_summary).toBe(false)
    expect(formFromConfig(null)).toEqual(blankCustomForm())
    expect(formFromConfig({ entity: 'bogus', columns: 'x' }).entity).toBe('requirement')
  })
})

describe('generate', () => {
  it('builds template and custom bodies', () => {
    expect(generateBody({ template: 'project_status', params: {} }, 'pdf')).toEqual({ format: 'pdf', template: 'project_status' })
    expect(generateBody({ template: 'run_summary', params: { run_id: 'r' } }, 'docx', true, ' Name ')).toEqual({ format: 'docx', template: 'run_summary', params: { run_id: 'r' }, archive: true, name: 'Name' })
    expect(generateBody({ custom: { entity: 'test', columns: ['key'] } }, 'csv', true)).toEqual({ format: 'csv', custom: { entity: 'test', columns: ['key'] }, archive: true })
  })
  it('sends the UI language', () => {
    expect(generateBody({ template: 'project_status' }, 'pdf', false, undefined, 'sv')).toEqual({ format: 'pdf', template: 'project_status', lang: 'sv' })
    expect(generateBody({ custom: { entity: 'test', columns: ['key'] } }, 'xlsx', true, 'N', 'en')).toMatchObject({ lang: 'en', archive: true, name: 'N' })
    expect('lang' in generateBody({ template: 'defects' }, 'csv', false, undefined, '')).toBe(false)
  })
  it('names downloads', () => {
    expect(reportFileName('DEMO-run summary', 'pdf', new Date(2026, 10, 9))).toBe('DEMO-run-summary-20261109.pdf')
    expect(reportFileName('', 'csv', new Date(2026, 0, 1))).toBe('report-20260101.csv')
    expect(reportFileName('***', 'csv', new Date(2026, 0, 1))).toBe('report-20260101.csv')
  })
  it('toggles columns', () => {
    expect(toggleColumn(['a'], 'b')).toEqual(['a', 'b'])
    expect(toggleColumn(['a', 'b'], 'a')).toEqual(['b'])
  })
})

describe('report batch', () => {
  const row = (kind: 'template' | 'definition', id: string, formats: ReportFormat[] = ['pdf', 'docx', 'xlsx', 'csv'], required: string[] = []) => ({ kind, id, name: id, description: '', formats, required })
  it('formats are those every selected report supports', () => {
    expect(batchFormats([])).toEqual(['pdf', 'docx', 'xlsx', 'csv'])
    expect(batchFormats([row('template', 'a', ['pdf', 'csv']), row('definition', 'b')])).toEqual(['pdf', 'csv'])
  })
  it('builds the request and finds missing parameters', () => {
    const rows = [row('template', 'run_summary', undefined, ['run_id']), row('template', 'defects'), row('definition', 'd1')]
    expect(batchMissing(rows, {})).toEqual(['run_id'])
    expect(batchMissing(rows, { run_id: 'r' })).toEqual([])
    expect(batchBody(rows, 'pdf', true, { run_id: 'r', other: 'x' })).toEqual({
      templates: [
        { template: 'run_summary', params: { run_id: 'r' } },
        { template: 'defects', params: {} },
      ],
      definitions: ['d1'],
      format: 'pdf',
      archive: true,
    })
    expect(batchBody(rows, 'csv', false, {}, 'sv')).toMatchObject({ format: 'csv', archive: false, lang: 'sv' })
    expect('lang' in batchBody(rows, 'csv', false, {})).toBe(false)
  })
})
