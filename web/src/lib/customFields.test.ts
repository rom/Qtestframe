import { describe, expect, it } from 'vitest'
import {
  applyTemplate,
  cfField,
  cfKey,
  contentOf,
  defaultValues,
  fieldLabel,
  fieldsOf,
  groupTemplates,
  isCfField,
  keyFromLabel,
  missingRequired,
  parseValue,
  sortKey,
  splitOptions,
  valueText,
  type CustomField,
  type ItemTemplate,
} from './customFields'
import { customCellText, customColumns, listedFields } from './customColumns'
import { applyValues, emptyValue, sheetFields, type FieldContext, type SheetRow } from './sheetFields'
import { toBatch } from './sheet'

const f = (type: CustomField['type'], key: string, over: Partial<CustomField> = {}): CustomField => ({
  id: key,
  entity_type: 'requirement',
  key,
  label: key[0].toUpperCase() + key.slice(1),
  type,
  unit: '',
  options: [],
  required: false,
  default: null,
  help: '',
  position: 0,
  show_in_list: false,
  ...over,
})
const mass = f('number', 'mass', { unit: 'kg', required: true, show_in_list: true, position: 1 })
const coat = f('multi_select', 'coat', { options: ['zinc', 'paint', 'none'], default: ['none'], position: 2 })
const rig = f('single_select', 'rig', { options: ['A', 'B'], entity_type: 'test' })
const ok = f('checkbox', 'ok', { position: 3 })

describe('values', () => {
  it('names and orders fields', () => {
    expect(fieldsOf([ok, coat, rig, mass], 'requirement').map((x) => x.key)).toEqual(['mass', 'coat', 'ok'])
    expect(fieldLabel(mass)).toBe('Mass (kg)')
    expect(cfField('mass')).toBe('cf.mass')
    expect(isCfField('cf.mass') && !isCfField('title')).toBe(true)
    expect(cfKey('cf.mass')).toBe('mass')
    expect(keyFromLabel('Mass (kg)')).toBe('mass_kg')
    expect(keyFromLabel('Ärende-nr')).toBe('arende_nr')
    expect(keyFromLabel('1st')).toBe('f_1st')
  })
  it('shows values as text and sorts them', () => {
    expect(valueText(mass, 2.5)).toBe('2.5 kg')
    expect(valueText(coat, ['zinc', 'paint'])).toBe('zinc, paint')
    expect(valueText(ok, true, ['Ja', 'Nej'])).toBe('Ja')
    expect(valueText(ok, false)).toBe('No')
    expect(valueText(mass, null)).toBe('')
    expect(valueText(f('text', 't'), 'x')).toBe('x')
    expect(sortKey(mass, 3)).toBe(3)
    expect(sortKey(ok, true)).toBe(1)
    expect(sortKey(coat, ['B'])).toBe('b')
    expect(sortKey(mass, undefined)).toBe('')
  })
  it('parses typed text', () => {
    expect(parseValue(mass, '1 200,5 kg')).toEqual({ ok: true, value: 1200.5 })
    expect(parseValue(mass, 'heavy')).toMatchObject({ ok: false, error: 'number' })
    expect(parseValue(f('date', 'd'), '2026-02-28')).toEqual({ ok: true, value: '2026-02-28' })
    expect(parseValue(f('date', 'd'), '2026-02-30')).toMatchObject({ ok: false, error: 'date' })
    expect(parseValue(rig, 'b')).toEqual({ ok: true, value: 'B' })
    expect(parseValue(rig, 'C')).toMatchObject({ ok: false, error: 'option' })
    expect(parseValue(coat, 'PAINT; zinc')).toEqual({ ok: true, value: ['zinc', 'paint'] })
    expect(parseValue(coat, 'gold')).toMatchObject({ ok: false, error: 'option', detail: 'gold' })
    expect(parseValue(ok, 'ja')).toEqual({ ok: true, value: true })
    expect(parseValue(ok, 'Nej', ['Ja', 'Nej'])).toEqual({ ok: true, value: false })
    expect(parseValue(ok, 'maybe')).toMatchObject({ ok: false, error: 'checkbox' })
    expect(parseValue(ok, '')).toEqual({ ok: true, value: false })
    expect(parseValue(f('user', 'u'), 'Anna@Example.com')).toEqual({ ok: true, value: 'anna@example.com' })
    expect(parseValue(f('user', 'u'), 'anna')).toMatchObject({ ok: false, error: 'email' })
    expect(parseValue(f('url', 'w'), 'https://example.org')).toEqual({ ok: true, value: 'https://example.org' })
    expect(parseValue(f('url', 'w'), 'ftp://x')).toMatchObject({ ok: false, error: 'url' })
    expect(parseValue(f('text', 't'), 'x'.repeat(1001))).toMatchObject({ ok: false, error: 'tooLong' })
    expect(parseValue(f('long_text', 'l'), 'a\r\nb')).toEqual({ ok: true, value: 'a\nb' })
    expect(parseValue(f('text', 't'), '  ')).toEqual({ ok: true, value: null })
    expect(splitOptions('a, b;\nc')).toEqual(['a', 'b', 'c'])
  })
  it('knows defaults and required fields', () => {
    expect(defaultValues([mass, coat])).toEqual({ coat: ['none'] })
    expect(missingRequired([mass, coat], {})).toEqual(['Mass'])
    expect(missingRequired([mass], { mass: 1 })).toEqual([])
  })
})

describe('templates', () => {
  const form = { title: '', description: 'x', priority: 'medium', status: 'draft', tags: ['a'], obligation: '', type: 'functional', criteria: [] as { description: string; method: string; threshold: string }[], custom_fields: { mass: 1 } }
  it('fills in what the template defines', () => {
    const out = applyTemplate(form, { title: 'T', priority: 'high', tags: ['a', 'b'], criteria: [{ description: 'c', method: '', threshold: '' }], custom_fields: { coat: ['zinc'], other: 'x' }, steps: [{ action: 'a', expected: 'e', data: '' }] }, [mass, coat])
    expect(out.title).toBe('T')
    expect(out.description).toBe('x')
    expect(out.priority).toBe('high')
    expect(out.tags).toEqual(['a', 'b'])
    expect(out.criteria).toEqual([{ description: 'c', method: 'test', threshold: '' }])
    expect(out.custom_fields).toEqual({ mass: 1, coat: ['zinc'] })
    expect('steps' in out).toBe(false)
  })
  it('fills test fields', () => {
    const tf = { title: '', description: '', priority: 'medium', status: 'draft', tags: [], preconditions: '', expected_result: '', execution_type: 'manual', estimated_minutes: 0 as number | null, steps: [] as { action: string; expected: string; data: string }[] }
    const out = applyTemplate(tf, { preconditions: 'p', estimated_minutes: 30, steps: [{ action: 'a', expected: 'e', data: 'd' }] }, [])
    expect(out.preconditions).toBe('p')
    expect(out.estimated_minutes).toBe(30)
    expect(out.steps).toEqual([{ action: 'a', expected: 'e', data: 'd' }])
    expect(contentOf('test', out)).toMatchObject({ preconditions: 'p', steps: [{ action: 'a', expected: 'e', data: 'd' }] })
  })
  it('turns a form into content', () => {
    const c = contentOf('requirement', { ...form, criteria: [{ description: 'c', method: 'test', threshold: '1', id: 'x' } as never], custom_fields: { mass: 2, coat: null } })
    expect(c.criteria).toEqual([{ description: 'c', method: 'test', threshold: '1' }])
    expect(c.custom_fields).toEqual({ mass: 2 })
  })
  it('groups templates by source', () => {
    const tpl = (id: string, source: ItemTemplate['source']): ItemTemplate => ({ id, kind: 'test', name: id, description: '', source, content: {}, position: 0 })
    expect(groupTemplates([tpl('b', 'builtin'), tpl('p', 'project')]).map((g) => g.source)).toEqual(['project', 'builtin'])
  })
})

describe('columns', () => {
  it('lists custom fields as columns', () => {
    const cols = customColumns([mass, coat])
    expect(cols.map((c) => c.field)).toEqual(['cf.mass', 'cf.coat'])
    expect(cols[0].header).toBe('Mass (kg)')
    expect(cols[0].sortField({ custom_fields: { mass: 3 } })).toBe(3)
    expect(listedFields([mass, coat])).toEqual(['cf.mass'])
    expect(customCellText([mass], { custom_fields: { mass: 2 } }, 'cf.mass', ['Y', 'N'])).toBe('2 kg')
    expect(customCellText([mass], {}, 'cf.nope', ['Y', 'N'])).toBe('')
  })
})

describe('spreadsheet', () => {
  const t = (key: string, p?: Record<string, unknown>) => (p ? `${key} ${JSON.stringify(p)}` : key)
  const meta = { obligations: ['must'], requirement_types: ['functional'], priorities: ['high'], requirement_statuses: ['draft'], test_statuses: ['draft'] }
  const ctx: FieldContext = { kind: 'requirement', t, enumLabel: (_g, v) => v, meta, categories: [], folders: [], canWrite: true, canReview: true, customFields: [mass, coat, ok] }
  const fs = sheetFields(ctx)
  const by = (id: string) => fs.find((x) => x.id === id)!
  const row: SheetRow = { id: 'r1', key: 'R-1', title: 'x', version: 1, custom_fields: { mass: 2, coat: ['zinc'] } }
  it('has a column per custom field', () => {
    expect(by('cf.mass').text(row)).toBe('2 kg')
    expect(by('cf.coat').text(row)).toBe('zinc')
    expect(by('cf.ok').text(row)).toBe('')
    expect(by('cf.ok').text({ ...row, custom_fields: { ok: true } })).toBe('common.yes')
    expect(by('cf.ok').editor).toBe('select')
    expect(by('cf.ok').options!(row).map((o) => o.value)).toEqual(['true', 'false'])
    expect(by('cf.mass').sortKey!(row)).toBe(2)
    expect(by('cf.mass').value!(row)).toBe(2)
    expect(by('cf.mass').parse!('3,5', row)).toEqual({ ok: true, value: 3.5 })
    expect(by('cf.mass').parse!('', row)).toMatchObject({ ok: false })
    expect(by('cf.coat').parse!('gold', row)).toMatchObject({ ok: false })
    expect(by('cf.coat').parse!('', row)).toEqual({ ok: true, value: null })
    expect(emptyValue(by('cf.coat'))).toBeNull()
  })
  it('saves custom fields as one object per item and applies it locally', () => {
    const { items, before } = toBatch([row], [
      { row: 0, col: 0, field: 'cf.mass', value: 3, before: 2 },
      { row: 0, col: 1, field: 'cf.coat', value: null, before: ['zinc'] },
      { row: 0, col: 2, field: 'title', value: 'y', before: 'x' },
    ])
    expect(items[0].set).toEqual({ custom_fields: { mass: 3, coat: null }, title: 'y' })
    expect(before[0].set).toEqual({ custom_fields: { mass: 2, coat: ['zinc'] }, title: 'x' })
    const out = applyValues(row, items[0].set, [])
    expect(out.custom_fields).toEqual({ mass: 3 })
  })
})
