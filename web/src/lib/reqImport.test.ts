import { describe, expect, it } from 'vitest'
import en from '@/i18n/locales/en'
import { changedMapping, columnChoices, formatOf, IMPORT_FIELDS, importCount, importFields, targetPath, type ImportCandidate } from './reqImport'

const cols = [
  { index: 0, header: 'ID', field: 'external_id' },
  { index: 1, header: 'Titel', field: 'title' },
  { index: 2, header: 'Notes', field: '' },
]

describe('reqImport helpers', () => {
  it('detects the format from the extension', () => {
    expect(formatOf('Krav.CSV')).toBe('csv')
    expect(formatOf('a.tsv')).toBe('csv')
    expect(formatOf('a.xlsx')).toBe('xlsx')
    expect(formatOf('a.reqifz')).toBe('reqif')
    expect(formatOf('a.docx')).toBe('docx')
    expect(formatOf('a.pdf')).toBeNull()
    expect(formatOf('noext')).toBeNull()
  })
  it('sends only changed columns', () => {
    const chosen = columnChoices(cols)
    expect(chosen).toEqual({ ID: 'external_id', Titel: 'title', Notes: '' })
    expect(changedMapping(cols, chosen)).toBeUndefined()
    expect(changedMapping(cols, { ...chosen, ID: '', Notes: 'rationale' })).toEqual({ ID: 'ignore', Notes: 'rationale' })
    expect(changedMapping(cols, { Other: 'title' })).toBeUndefined()
  })
  it('keeps earlier choices for known columns', () => {
    expect(columnChoices(cols, { Notes: 'tags', Gone: 'title' })).toEqual({ ID: 'external_id', Titel: 'title', Notes: 'tags' })
  })
  it('builds the form fields', () => {
    expect(importFields({ sheet: '', mapping: undefined })).toEqual({ sheet: undefined, mapping: undefined })
    const f = importFields(
      { sheet: 'Krav', mapping: { ID: 'ignore' } },
      { folderId: 'f1', createFolders: false, status: 'review', createTests: true, skipDuplicates: true, dryRun: true },
      new Set([3, 1]),
    )
    expect(f).toEqual({
      sheet: 'Krav',
      mapping: '{"ID":"ignore"}',
      folder_id: 'f1',
      create_folders: 'false',
      status: 'review',
      create_tests: 'true',
      skip_duplicates: 'true',
      dry_run: 'true',
      exclude: '[1,3]',
    })
    expect(importFields({}, { folderId: null, createFolders: true, status: 'draft', createTests: false, skipDuplicates: false }).folder_id).toBeUndefined()
  })
  it('counts what will be created', () => {
    const c = (duplicate = '') => ({ duplicate }) as ImportCandidate
    const list = [c(), c('title'), c(), c('external_id')]
    expect(importCount(list, true, new Set())).toBe(2)
    expect(importCount(list, false, new Set([0]))).toBe(3)
  })
  it('shows target paths', () => {
    expect(targetPath('Kund', 'A/B', true)).toBe('Kund / A / B')
    expect(targetPath('', 'A/B', true)).toBe('A / B')
    expect(targetPath('Kund', 'A/B', false)).toBe('Kund')
    expect(targetPath('', '', true)).toBe('')
  })
  it('has a label for every field', () => {
    const labels = (en as unknown as { reqImport: { field: Record<string, string> } }).reqImport.field
    expect(IMPORT_FIELDS.filter((f) => !labels[f])).toEqual([])
    expect(labels.ignore).toBeTruthy()
  })
})
