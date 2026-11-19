import { describe, expect, it } from 'vitest'
import { byteToIndex, hasNonAscii, positionToIndex, queryTerms, segments, splitByTerms } from './highlight'

describe('segments', () => {
  it('returns nothing for empty text', () => {
    expect(segments('', [{ offset: 0, length: 1 }])).toEqual([])
  })
  it('returns the whole text when no ranges', () => {
    expect(segments('abc', null)).toEqual([{ text: 'abc', mark: false }])
    expect(segments('abc', undefined)).toEqual([{ text: 'abc', mark: false }])
  })
  it('splits by keyword offsets', () => {
    const s = segments('The system MUST log and SHOULD alert', [
      { offset: 24, length: 6, kind: 'should' },
      { offset: 11, length: 4, kind: 'must', title: 'MUST' },
    ])
    expect(s).toEqual([
      { text: 'The system ', mark: false },
      { text: 'MUST', mark: true, kind: 'must', title: 'MUST' },
      { text: ' log and ', mark: false },
      { text: 'SHOULD', mark: true, kind: 'should', title: undefined },
      { text: ' alert', mark: false },
    ])
  })
  it('handles ranges at the edges, overlaps and invalid ranges', () => {
    const s = segments('abcdef', [
      { offset: 0, length: 3, kind: 'a' },
      { offset: 1, length: 1, kind: 'inside' },
      { offset: 2, length: 3, kind: 'overlap' },
      { offset: 4, length: 0 },
      { offset: -1, length: 2 },
      { offset: 5, length: 10, kind: 'tail' },
    ])
    expect(s.map((x) => [x.text, x.kind ?? null])).toEqual([
      ['abc', 'a'],
      ['de', 'overlap'],
      ['f', 'tail'],
    ])
  })
  it('ignores ranges beyond the text', () => {
    expect(segments('ab', [{ offset: 5, length: 2 }])).toEqual([{ text: 'ab', mark: false }])
  })
  it('converts UTF-8 byte offsets for non-ASCII text', () => {
    const text = 'Größe MUST'
    // "Größe " is 8 bytes in UTF-8 (ö and ß are 2 bytes)
    const s = segments(text, [{ offset: 8, length: 4 }])
    expect(s).toEqual([
      { text: 'Größe ', mark: false },
      { text: 'MUST', mark: true, kind: undefined, title: undefined },
    ])
    // character offsets when bytes=false
    expect(segments(text, [{ offset: 6, length: 4 }], false)[1].text).toBe('MUST')
  })
})

describe('byteToIndex', () => {
  it('maps multi-byte characters', () => {
    expect(byteToIndex('aé€😀b', 0)).toBe(0)
    expect(byteToIndex('aé€😀b', -3)).toBe(0)
    expect(byteToIndex('aé€😀b', 1)).toBe(1)
    expect(byteToIndex('aé€😀b', 3)).toBe(2)
    expect(byteToIndex('aé€😀b', 6)).toBe(3)
    expect(byteToIndex('aé€😀b', 10)).toBe(5)
    expect(byteToIndex('aé€😀b', 11)).toBe(6)
    expect(byteToIndex('aé€😀b', 99)).toBe(6)
    // offset inside a multibyte char rounds down
    expect(byteToIndex('é', 1)).toBe(0)
  })
  it('positionToIndex', () => {
    expect(hasNonAscii('abc')).toBe(false)
    expect(positionToIndex('abc', 2)).toBe(2)
    expect(positionToIndex('abc', 9)).toBe(3)
    expect(positionToIndex('abc', -1)).toBe(0)
    expect(positionToIndex('éx', 2)).toBe(1)
  })
})

describe('query terms', () => {
  it('extracts plain words', () => {
    expect(queryTerms('')).toEqual([])
    expect(queryTerms(null)).toEqual([])
    expect(queryTerms('status:approved login AND -tag:x "time out" title~/re/ key:REQ-1* a OR not')).toEqual([
      'approved',
      'login',
      'time',
      'out',
      'REQ-1',
    ])
    expect(queryTerms('version>=2 name!=foo')).toEqual(['foo'])
  })
  it('splits text by terms', () => {
    expect(splitByTerms('', ['a'])).toEqual([])
    expect(splitByTerms('abc', [])).toEqual([{ text: 'abc', mark: false }])
    expect(splitByTerms('Login fails at login', ['login'])).toEqual([
      { text: 'Login', mark: true },
      { text: ' fails at ', mark: false },
      { text: 'login', mark: true },
    ])
    expect(splitByTerms('a.b axb', ['a.b'])).toEqual([
      { text: 'a.b', mark: true },
      { text: ' axb', mark: false },
    ])
  })
})
