import { describe, expect, it } from 'vitest'
import {
  acceptsTestFile,
  savableRow,
  sourceInfo,
  targetsOf,
  testImportFields,
  testSaveCount,
  visibleTestRows,
  type TestImportRow,
  type TestImportState,
} from './testImport'

const row = (index: number, action: TestImportRow['action']): TestImportRow => ({
  index, location: `row ${index}`, external_id: `C${index}`, title: `T${index}`, folder: '', type: '', priority: 'medium', status: 'draft', steps: 1,
  action, links: [], unresolved: [], executions: 0, warnings: [], errors: [],
})

describe('test import helpers', () => {
  it('knows the sources and their files', () => {
    expect(sourceInfo('xray').name).toBe('Xray')
    expect(sourceInfo('nope' as never).id).toBe('testrail')
    expect(acceptsTestFile('testrail', 'suite.XML')).toBe(true)
    expect(acceptsTestFile('azure', 'cases.xml')).toBe(false)
    expect(acceptsTestFile('qtest', 'export.xlsx')).toBe(true)
  })

  it('counts and filters rows', () => {
    const rows = [row(0, 'new'), row(1, 'update'), row(2, 'conflict'), row(3, 'error'), row(4, 'unchanged')]
    expect(savableRow(rows[0], { overwrite: false, create: false })).toBe(false)
    expect(testSaveCount(rows, { overwrite: false, create: true }, new Set())).toBe(2)
    expect(testSaveCount(rows, { overwrite: true, create: true }, new Set([0]))).toBe(2)
    expect(visibleTestRows(rows, []).map((r) => r.index)).toEqual([0, 1, 2, 3])
    expect(visibleTestRows(rows, ['unchanged']).map((r) => r.index)).toEqual([4])
  })

  it('reads the targets of a preview', () => {
    const values = [{ value: 'High', count: 1, target: 'high', guessed: true }, { value: 'Low', count: 2, target: 'low', guessed: false }]
    expect(targetsOf(values, (v) => v.value)).toEqual({ High: 'high', Low: 'low' })
  })

  it('builds the form fields', () => {
    const state: TestImportState = { source: 'zephyr', folderId: 'f1', priorities: { Normal: 'low' }, statuses: {}, fields: { Browser: 'browser' }, createFolders: false, links: true, history: false }
    expect(testImportFields({ ...state, folderId: null, priorities: {}, fields: {} })).toEqual({ source: 'zephyr', folder_id: undefined, priorities: undefined, statuses: undefined, fields: undefined })
    const f = testImportFields(state, { overwrite: true, create: false, excluded: new Set([3, 1]) })
    expect(f.priorities).toBe('{"Normal":"low"}')
    expect(f.fields).toBe('{"Browser":"browser"}')
    expect(f.create_folders).toBe('false')
    expect(f.history).toBe('false')
    expect(f.overwrite_conflicts).toBe('true')
    expect(f.exclude).toBe('[1,3]')
  })
})
