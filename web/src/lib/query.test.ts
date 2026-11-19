import { describe, expect, it } from 'vitest'
import {
  activeFilterCount,
  and,
  anyOf,
  clause,
  composeQuery,
  group,
  parseTags,
  QUERY_FIELDS,
  QUERY_HELP,
  QUERY_OPERATORS,
  quoteValue,
} from './query'

describe('quoteValue', () => {
  it('leaves simple tokens bare', () => {
    expect(quoteValue('approved')).toBe('approved')
    expect(quoteValue('REQ-1')).toBe('REQ-1')
    expect(quoteValue('1.5')).toBe('1.5')
  })
  it('quotes spaces, keywords, special characters and escapes', () => {
    expect(quoteValue('Network based')).toBe('"Network based"')
    expect(quoteValue('OR')).toBe('"OR"')
    expect(quoteValue('not')).toBe('"not"')
    expect(quoteValue('-x')).toBe('"-x"')
    expect(quoteValue('Polarity/Negative')).toBe('"Polarity/Negative"')
    expect(quoteValue('say "hi" \\ there')).toBe('"say \\"hi\\" \\\\ there"')
    expect(quoteValue('')).toBe('""')
  })
})

describe('clauses', () => {
  it('builds clauses', () => {
    expect(clause('status', '=', 'draft')).toBe('status=draft')
    expect(clause('category', ':', 'File based')).toBe('category:"File based"')
  })
  it('anyOf', () => {
    expect(anyOf('status', [])).toBe('')
    expect(anyOf('status', null)).toBe('')
    expect(anyOf('status', ['draft'])).toBe('status=draft')
    expect(anyOf('status', ['draft', '', 'review'], ':')).toBe('(status:draft OR status:review)')
  })
})

describe('group/and', () => {
  it('groups only when needed', () => {
    expect(group('')).toBe('')
    expect(group('status:a')).toBe('status:a')
    expect(group('"a b"')).toBe('"a b"')
    expect(group('title~/a b/')).toBe('title~/a b/')
    expect(group('a\\ b')).toBe('a\\ b')
    expect(group('(a OR b)')).toBe('(a OR b)')
    expect(group('(a) OR (b)')).toBe('((a) OR (b))')
    expect(group('("(" OR b)')).toBe('("(" OR b)')
    expect(group('(a \\) b)')).toBe('(a \\) b)')
    expect(group('a OR b')).toBe('(a OR b)')
    expect(group('(a OR b')).toBe('((a OR b)')
  })
  it('and joins non-empty parts', () => {
    expect(and()).toBe('')
    expect(and('', null, undefined)).toBe('')
    expect(and(' a OR b ')).toBe('a OR b')
    expect(and('a OR b', 'status=x')).toBe('(a OR b) AND status=x')
  })
})

describe('composeQuery', () => {
  it('composes free text and quick filters', () => {
    expect(composeQuery('')).toBe('')
    expect(composeQuery(null, {})).toBe('')
    expect(composeQuery('login fail', { status: ['draft', 'review'], obligation: ['must'] })).toBe(
      '(login fail) AND (status=draft OR status=review) AND obligation=must',
    )
    expect(
      composeQuery(undefined, {
        priority: ['high'],
        execution_type: ['manual'],
        last_result: ['failed'],
        coverage: ['none'],
        categories: ['Medium/Network based'],
      }),
    ).toBe('priority=high AND execution_type=manual AND last_result=failed AND coverage=none AND dimension="Medium/Network based"')
  })
  it('counts active filters', () => {
    expect(activeFilterCount({})).toBe(0)
    expect(activeFilterCount({ status: ['a', 'b'], categories: ['x'] })).toBe(3)
  })
})

describe('parseTags', () => {
  it('splits and deduplicates', () => {
    expect(parseTags('a, b  c,a')).toEqual(['a', 'b', 'c'])
    expect(parseTags('  ')).toEqual([])
  })
})

describe('help data', () => {
  it('is populated', () => {
    expect(QUERY_HELP.length).toBeGreaterThan(10)
    expect(QUERY_OPERATORS.length).toBe(4)
    expect(QUERY_FIELDS.requirement).toContain('obligation')
    expect(QUERY_FIELDS.test).toContain('execution_type')
  })
})
