import { describe, expect, it } from 'vitest'
import { acceptsFile, changedAttrMapping, clip, exchangeFields, saveCount, statusCount, visibleRows, type ExchangeRow } from './exchange'

const row = (index: number, status: ExchangeRow['status']): ExchangeRow => ({ index, row: index + 2, location: `row ${index + 2}`, status, title: 't', changes: [], errors: [], warnings: [] })
const rows = [row(0, 'new'), row(1, 'changed'), row(2, 'conflict'), row(3, 'error'), row(4, 'unchanged')]

describe('exchange helpers', () => {
  it('accepts files by mode', () => {
    expect(acceptsFile('xlsx', 'a.XLSX')).toBe(true)
    expect(acceptsFile('xlsx', 'a.csv')).toBe(false)
    expect(acceptsFile('reqif', 'a.reqifz')).toBe(true)
    expect(acceptsFile('reqif', 'a.xlsx')).toBe(false)
  })

  it('counts rows to save', () => {
    expect(saveCount(rows, { overwrite: false, create: true }, new Set())).toBe(2)
    expect(saveCount(rows, { overwrite: true, create: true }, new Set())).toBe(3)
    expect(saveCount(rows, { overwrite: true, create: false }, new Set([1]))).toBe(1)
    expect(statusCount({ new: 1, changed: 2, unchanged: 3, conflicts: 4, errors: 5 }, 'conflict')).toBe(4)
    expect(statusCount({ new: 1, changed: 2, unchanged: 3, conflicts: 4, errors: 5 }, 'error')).toBe(5)
  })

  it('filters rows', () => {
    expect(visibleRows(rows, []).map((r) => r.status)).toEqual(['new', 'changed', 'conflict', 'error'])
    expect(visibleRows(rows, ['unchanged']).length).toBe(1)
  })

  it('sends only changed mappings', () => {
    const attrs = [
      { name: 'A', type: 'string', objects: 1, sample: '', object_types: [], field: 'title', guess: 'title' },
      { name: 'B', type: 'string', objects: 1, sample: '', object_types: [], field: '', guess: '' },
    ]
    expect(changedAttrMapping(attrs, { A: 'title', B: '' })).toBeUndefined()
    expect(changedAttrMapping(attrs, { A: '', B: 'tags' })).toEqual({ A: 'ignore', B: 'tags' })
  })

  it('builds form fields', () => {
    expect(exchangeFields('xlsx', { kind: 'test' })).toEqual({ kind: 'test' })
    expect(exchangeFields('reqif', { mapping: { A: 'tags' }, folderId: 'f', hierarchy: false }, { overwrite: true, create: false, excluded: [3, 1] })).toEqual({
      mapping: '{"A":"tags"}',
      folder_id: 'f',
      hierarchy: 'false',
      overwrite_conflicts: 'true',
      create: 'false',
      exclude: '[1,3]',
    })
    expect(clip('a\n  b')).toBe('a b')
    expect(clip('x'.repeat(10), 5)).toBe('xxxx…')
  })
})
