import { describe, expect, it } from 'vitest'
import { changedKeys, diffSnapshots, diffWords, MAX_DIFF_CELLS, mergeSegments, parseSnapshot, sortFields, tokenize, valueToText, type DiffSegment } from './diff'

const join = (segs: DiffSegment[], ops: string[]) => segs.filter((s) => ops.includes(s.op)).map((s) => s.text).join('')

describe('tokenize', () => {
  it('keeps words and whitespace', () => {
    expect(tokenize('a  b\nc')).toEqual(['a', '  ', 'b', '\n', 'c'])
    expect(tokenize('')).toEqual([])
  })
})

describe('mergeSegments', () => {
  it('merges neighbours and drops empty segments', () => {
    expect(
      mergeSegments([
        { op: 'equal', text: 'a' },
        { op: 'equal', text: 'b' },
        { op: 'insert', text: '' },
        { op: 'delete', text: 'c' },
      ]),
    ).toEqual([
      { op: 'equal', text: 'ab' },
      { op: 'delete', text: 'c' },
    ])
  })
})

describe('diffWords', () => {
  it('returns a single equal segment for identical text', () => {
    expect(diffWords('the system shall', 'the system shall')).toEqual([{ op: 'equal', text: 'the system shall' }])
  })
  it('handles empty sides', () => {
    expect(diffWords('', '')).toEqual([])
    expect(diffWords(null, 'new text')).toEqual([{ op: 'insert', text: 'new text' }])
    expect(diffWords('old', undefined)).toEqual([{ op: 'delete', text: 'old' }])
  })
  it('marks replaced words', () => {
    const d = diffWords('The system SHALL log in users', 'The system MUST log out users')
    expect(join(d, ['equal', 'delete'])).toBe('The system SHALL log in users')
    expect(join(d, ['equal', 'insert'])).toBe('The system MUST log out users')
    expect(d.filter((s) => s.op === 'delete').map((s) => s.text)).toEqual(['SHALL', 'in'])
    expect(d.filter((s) => s.op === 'insert').map((s) => s.text)).toEqual(['MUST', 'out'])
  })
  it('puts deletions before insertions inside a change', () => {
    const d = diffWords('a b c', 'a x y c')
    expect(d).toEqual([
      { op: 'equal', text: 'a ' },
      { op: 'delete', text: 'b' },
      { op: 'insert', text: 'x y' },
      { op: 'equal', text: ' c' },
    ])
  })
  it('detects pure insertions and deletions', () => {
    expect(diffWords('a c', 'a b c')).toEqual([
      { op: 'equal', text: 'a ' },
      { op: 'insert', text: 'b ' },
      { op: 'equal', text: 'c' },
    ])
    const d = diffWords('one two three four', 'one four')
    expect(join(d, ['equal', 'insert'])).toBe('one four')
    expect(join(d, ['equal', 'delete'])).toBe('one two three four')
  })
  it('degrades to replace for huge inputs', () => {
    const n = Math.ceil(Math.sqrt(MAX_DIFF_CELLS)) + 10
    const a = Array.from({ length: n }, (_, i) => 'a' + i).join(' ')
    const b = Array.from({ length: n }, (_, i) => 'b' + i).join(' ')
    const d = diffWords('x ' + a, 'x ' + b)
    expect(d[0]).toEqual({ op: 'equal', text: 'x ' })
    expect(d.map((s) => s.op)).toEqual(['equal', 'delete', 'insert'])
  })
})

describe('valueToText', () => {
  it('formats scalars, lists and objects', () => {
    expect(valueToText('title', null)).toBe('')
    expect(valueToText('title', 'x')).toBe('x')
    expect(valueToText('estimated_minutes', 5)).toBe('5')
    expect(valueToText('tags', ['a', 'b'])).toBe('a, b')
    expect(valueToText('meta', { a: 1 })).toBe('{"a":1}')
  })
  it('resolves ids', () => {
    const resolve = (f: string, id: string) => (f === 'category_ids' && id === 'c1' ? 'Area/Core' : f === 'folder_id' && id === 'f1' ? 'Root/Sub' : undefined)
    expect(valueToText('category_ids', ['c1', 'c2'], { resolve })).toBe('Area/Core, c2')
    expect(valueToText('folder_id', 'f1', { resolve })).toBe('Root/Sub')
    expect(valueToText('folder_id', 'zz', { resolve })).toBe('zz')
  })
  it('numbers criteria and steps', () => {
    expect(
      valueToText('criteria', [
        { id: '1', description: 'Fast', method: 'test', threshold: '< 1 s', position: 1 },
        { id: '2', description: 'Nice', method: 'review', threshold: '', position: 2 },
      ]),
    ).toBe('1. Fast | test | < 1 s\n2. Nice | review')
    expect(
      valueToText('steps', [
        { id: 's', action: 'Open', expected: 'Shown', data: '', extra: { a: 1 }, flag: true },
        { id: 't', call_test_id: 'x', call_test_key: 'P-TC-2', call_test_title: 'Login' },
        { id: 'u', call_test_id: 'y' },
        null,
      ]),
    ).toBe('1. Open | Shown | {"a":1} | true\n2. Calls P-TC-2 – Login\n3. Calls y\n4. null')
  })
})

describe('snapshots', () => {
  it('sorts fields by display order', () => {
    expect(sortFields(['zeta', 'tags', 'title', 'alpha', 'title'])).toEqual(['title', 'tags', 'alpha', 'zeta'])
  })
  it('finds changed keys ignoring derived fields', () => {
    const a = { title: 'A', tags: [], description: '', version: 1, updated_at: 'x', owner: 'bob' }
    const b = { title: 'B', tags: null, version: 2, updated_at: 'y', owner: 'bob', status: 'draft' }
    expect(changedKeys(a, b)).toEqual(['title', 'status'])
    expect(changedKeys(null, { title: 'x' })).toEqual(['title'])
    expect(changedKeys(undefined, undefined)).toEqual([])
  })
  it('diffs snapshots by field', () => {
    const a = { title: 'Login works', priority: 'high', tags: ['x'] }
    const b = { title: 'Login fails', priority: 'low', tags: ['x', 'y'] }
    const d = diffSnapshots(a, b)
    expect(d.map((c) => c.field)).toEqual(['title', 'priority', 'tags'])
    expect(d[1]).toEqual({ field: 'priority', before: 'high', after: 'low', segments: null })
    expect(d[0].segments?.some((s) => s.op === 'insert' && s.text === 'fails')).toBe(true)
    expect(d[2].segments).not.toBeNull()
  })
  it('limits to the given fields and skips ignored ones', () => {
    const d = diffSnapshots({ title: 'a', owner: 'x' }, { title: 'b', owner: 'y' }, ['owner', 'updated_at'])
    expect(d.map((c) => c.field)).toEqual(['owner'])
    expect(diffSnapshots({ title: 'a' }, { title: 'b' }, [])).toEqual([])
    expect(diffSnapshots({ title: 'a' }, { title: 'b' }, null).map((c) => c.field)).toEqual(['title'])
    // listed but textually identical fields are skipped
    expect(diffSnapshots({ owner: 'x' }, { owner: 'x' }, ['owner'])).toEqual([])
  })
  it('ignores ids and positions of list items', () => {
    const a = { steps: [{ id: '1', position: 1, action: 'Open', expected: '', call_test_id: null }] }
    const b = { steps: [{ id: '9', position: 1, action: 'Open', data: '' }] }
    expect(changedKeys(a, b)).toEqual([])
    expect(changedKeys({ tags: ['x'] }, { tags: ['y'] })).toEqual(['tags'])
    expect(changedKeys({ steps: [null, [1]] }, { steps: [null, [1]] })).toEqual([])
  })
  it('treats a missing previous snapshot as all added', () => {
    const d = diffSnapshots(null, { title: 'New', status: 'draft' })
    expect(d.map((c) => [c.field, c.before, c.after])).toEqual([
      ['title', '', 'New'],
      ['status', '', 'draft'],
    ])
  })
  it('parses snapshots', () => {
    expect(parseSnapshot({ a: 1 })).toEqual({ a: 1 })
    expect(parseSnapshot('{"a":1}')).toEqual({ a: 1 })
    expect(parseSnapshot('[1]')).toBeNull()
    expect(parseSnapshot('nope')).toBeNull()
    expect(parseSnapshot('')).toBeNull()
    expect(parseSnapshot(null)).toBeNull()
    expect(parseSnapshot([1])).toBeNull()
    expect(parseSnapshot(5)).toBeNull()
  })
})
