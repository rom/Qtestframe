import { describe, expect, it } from 'vitest'
import { ApiError } from './api'
import { asSet, displayValue, fieldConflicts, project, REQUIREMENT_FIELDS, staleOf, TEST_FIELDS } from './conflicts'

describe('staleOf', () => {
  it('reads a 412 stale answer', () => {
    const e = new ApiError(412, 'stale', 'changed', undefined, { current: { id: 'x', version: 3 }, changed_by: 'Anna', changed_at: '2026-01-01T10:00:00Z' })
    expect(staleOf(e)).toEqual({ current: { id: 'x', version: 3 }, changedBy: 'Anna', changedAt: '2026-01-01T10:00:00Z' })
    expect(staleOf(new ApiError(412, 'stale', 'x', undefined, { current: { id: 'x' } }))).toEqual({ current: { id: 'x' }, changedBy: '', changedAt: '' })
  })
  it('ignores other errors', () => {
    expect(staleOf(new ApiError(409, 'conflict', 'x'))).toBeNull()
    expect(staleOf(new ApiError(412, 'stale', 'x'))).toBeNull()
    expect(staleOf(new Error('x'))).toBeNull()
  })
})

describe('fieldConflicts', () => {
  const base = { title: 'A', description: 'd', tags: ['x', 'y'], steps: [{ id: '1', action: 'go', expected: 'ok', position: 1 }], estimated: 5 }
  const norm = { tags: asSet, steps: project(['action', 'expected']) }
  it('lists what they changed, and whether you changed it too', () => {
    const theirs = { ...base, title: 'B', description: 'd2', tags: ['y', 'x'] }
    const mine = { ...base, title: 'C', description: 'd2' }
    expect(fieldConflicts(['title', 'description', 'tags', 'steps', 'estimated'], base, theirs, mine, norm)).toEqual([
      { field: 'title', theirs: 'B', mine: 'C', both: true },
      { field: 'description', theirs: 'd2', mine: 'd2', both: false },
    ])
  })
  it('compares lists by content, not ids or positions', () => {
    const theirs = { ...base, steps: [{ id: '9', action: 'go', expected: 'ok', position: 3 }] }
    expect(fieldConflicts(['steps'], base, theirs, base, norm)).toEqual([])
    const changed = { ...base, steps: [{ action: 'stop', expected: 'ok' }] }
    expect(fieldConflicts(['steps'], base, changed, base, norm)).toEqual([{ field: 'steps', theirs: [{ action: 'stop', expected: 'ok' }], mine: [{ action: 'go', expected: 'ok' }], both: false }])
  })
  it('treats missing values as empty', () => {
    expect(fieldConflicts(['owner', 'n'], { owner: '' }, { owner: undefined }, {}, {})).toEqual([])
    expect(project(['a'])(undefined)).toEqual([])
    expect(project(['a'])(['s'])).toEqual(['s'])
    expect(asSet(null)).toEqual([])
  })
})

describe('displayValue', () => {
  it('shortens values for the table', () => {
    expect(displayValue('')).toBe('—')
    expect(displayValue(null)).toBe('—')
    expect(displayValue([])).toBe('—')
    expect(displayValue(['a', 'b'])).toBe('a, b')
    expect(displayValue([{ action: 'go', expected: '' }, { action: 'x', expected: 'y' }])).toBe('go / x · y')
    expect(displayValue('a\n  b')).toBe('a b')
    expect(displayValue('abcdef', 4)).toBe('abc…')
    expect(displayValue(5)).toBe('5')
  })
})

describe('item fields', () => {
  it('compares requirement criteria and test steps by content', () => {
    const r = { title: 'A', tags: ['a'], criteria: [{ id: '1', description: 'x', method: 'test', threshold: '', position: 1 }] }
    const theirs = { ...r, tags: ['a'], criteria: [{ id: '1', description: 'y', method: 'test', threshold: '', position: 1 }] }
    expect(fieldConflicts(REQUIREMENT_FIELDS.fields, r, theirs, r, REQUIREMENT_FIELDS.normalize).map((c) => c.field)).toEqual(['criteria'])
    const tc = { title: 'T', steps: [{ id: 's', action: 'a', expected: 'e', data: '', call_test_id: null, position: 1 }] }
    const mine = { title: 'T', steps: [{ uid: 1, id: 's', action: 'a', expected: 'e', data: '', call_test_id: null }] }
    expect(fieldConflicts(TEST_FIELDS.fields, tc, { ...tc, title: 'U' }, mine, TEST_FIELDS.normalize)).toEqual([{ field: 'title', theirs: 'U', mine: 'T', both: false }])
  })
})

it('compares test data', () => {
  const base = { parameters: ['user'], data_rows: [{ name: 'a', values: { user: 'x' } }] }
  const theirs = { parameters: ['user'], data_rows: [{ name: 'a', values: { user: 'y' } }] }
  expect(fieldConflicts(TEST_FIELDS.fields, base, theirs, base, TEST_FIELDS.normalize).map((c) => c.field)).toEqual(['data_rows'])
  expect(fieldConflicts(['parameters'], { parameters: null }, { parameters: ['p'] }, {}, TEST_FIELDS.normalize)).toEqual([{ field: 'parameters', theirs: ['p'], mine: [], both: false }])
})
