import { describe, expect, it } from 'vitest'
import {
  blankRow,
  dataToTSV,
  executionStatus,
  iterationStatus,
  parameterProblem,
  parseTable,
  placeholderOf,
  placeholdersIn,
  removeParameter,
  renameParameter,
  resolvePlaceholders,
  tableToData,
  unknownPlaceholders,
  validParameterName,
} from './testData'

describe('placeholders', () => {
  it('resolves known parameters and keeps unknown ones', () => {
    const v = { user: 'alice', pw: 's3cret', å: 'ö' }
    expect(resolvePlaceholders('Log in as {{user}}', v)).toBe('Log in as alice')
    expect(resolvePlaceholders('{{ user }}/{{pw}}', v)).toBe('alice/s3cret')
    expect(resolvePlaceholders('{{unknown}} {{user}}', v)).toBe('{{unknown}} alice')
    expect(resolvePlaceholders('{{å}}', v)).toBe('ö')
    expect(resolvePlaceholders('{user} {{ 1x }}', v)).toBe('{user} {{ 1x }}')
    expect(resolvePlaceholders('{{user}}', null)).toBe('{{user}}')
    expect(resolvePlaceholders('', v)).toBe('')
    expect(resolvePlaceholders('{{toString}}', v)).toBe('{{toString}}')
  })
  it('lists placeholders and the unknown ones', () => {
    expect(placeholdersIn('{{a}} {{ b }}', null, '{{a}} {{c}}')).toEqual(['a', 'b', 'c'])
    expect(unknownPlaceholders(['{{a}} {{b}}', undefined], [' a '])).toEqual(['b'])
    expect(placeholderOf('qty')).toBe('{{qty}}')
  })
  it('validates parameter names', () => {
    for (const ok of ['user', '_x', 'a.b-c', 'åäö']) expect(validParameterName(ok)).toBe(true)
    for (const bad of ['1a', 'a b', '', 'x'.repeat(51)]) expect(validParameterName(bad)).toBe(false)
    expect(parameterProblem(['a', 'A', '1'], 0)).toBe('')
    expect(parameterProblem(['a', 'A', '1'], 1)).toBe('duplicate')
    expect(parameterProblem(['a', 'A', '1'], 2)).toBe('invalid')
    expect(parameterProblem([], 3)).toBe('invalid')
  })
})

describe('rows', () => {
  it('builds, renames and removes', () => {
    expect(blankRow(['a', 'b'], 'r')).toEqual({ name: 'r', values: { a: '', b: '' } })
    const rows = [{ name: '', values: { a: '1', b: '2' } }, { name: 'x', values: undefined as unknown as Record<string, string> }]
    expect(renameParameter(rows, 'a', 'c')).toEqual([{ name: '', values: { c: '1', b: '2' } }, { name: 'x', values: { c: '' } }])
    const same = renameParameter(rows, 'a', 'a')
    expect(same[0]).toEqual(rows[0])
    expect(same[0].values).not.toBe(rows[0].values)
    expect(removeParameter(rows, 'a')).toEqual([{ name: '', values: { b: '2' } }, { name: 'x', values: {} }])
  })
})

describe('pasted tables', () => {
  it('parses tab separated cells (spreadsheets)', () => {
    expect(parseTable('user\tpw\r\nalice\t"a\tb"\nbob\t\n\n')).toEqual([
      ['user', 'pw'],
      ['alice', 'a\tb'],
      ['bob', ''],
    ])
  })
  it('parses CSV with quotes, line breaks and semicolons', () => {
    expect(parseTable('a,b\n"x, y","say ""hi""\nthere"')).toEqual([
      ['a', 'b'],
      ['x, y', 'say "hi"\nthere'],
    ])
    expect(parseTable('a;b\n1;2,5')).toEqual([
      ['a', 'b'],
      ['1', '2,5'],
    ])
    expect(parseTable('')).toEqual([])
    expect(parseTable('a,"b')).toEqual([['a', 'b']])
    expect(parseTable('a,b,')).toEqual([['a', 'b', '']])
  })
  it('turns cells into parameters and rows', () => {
    const cells = [
      ['', 'user', 'pw'],
      ['Admin', 'root', 'x'],
      ['Guest', 'guest'],
    ]
    expect(tableToData(cells, { header: true, nameColumn: true })).toEqual({
      parameters: ['user', 'pw'],
      rows: [
        { name: 'Admin', values: { user: 'root', pw: 'x' } },
        { name: 'Guest', values: { user: 'guest', pw: '' } },
      ],
    })
    expect(tableToData([['1', '2', '3']], { header: false, nameColumn: false, parameters: ['a', 'b'] })).toEqual({
      parameters: ['a', 'b'],
      rows: [{ name: '', values: { a: '1', b: '2' } }],
    })
    expect(tableToData([[]], { header: false, nameColumn: true })).toEqual({ parameters: [], rows: [{ name: '', values: {} }] })
    expect(tableToData([], { header: true, nameColumn: false })).toEqual({ parameters: [], rows: [] })
  })
  it('writes tab separated text that parses back', () => {
    const rows = [
      { name: 'A', values: { u: 'x\ty', p: 'say "hi"' } },
      { name: '', values: { u: '1' } },
      { name: '', values: undefined as unknown as Record<string, string> },
    ]
    const tsv = dataToTSV(['u', 'p'], rows)
    expect(tableToData(parseTable(tsv), { header: true, nameColumn: true }).rows).toEqual([rows[0], { name: '', values: { u: '1', p: '' } }])
  })
})

describe('derived results', () => {
  it('derives iteration and execution results', () => {
    expect(iterationStatus(['passed', 'passed'])).toBe('passed')
    expect(iterationStatus(['passed', 'not_run'])).toBe('in_progress')
    expect(executionStatus(['passed', 'failed', 'blocked'])).toBe('failed')
    expect(executionStatus(['passed', 'blocked'])).toBe('blocked')
    expect(executionStatus(['passed', 'skipped'])).toBe('passed')
    expect(executionStatus(['skipped'])).toBe('skipped')
    expect(executionStatus([])).toBe('not_run')
  })
})
