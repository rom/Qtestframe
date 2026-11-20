import { describe, expect, it } from 'vitest'
import { DEFAULT_FROM, decisionsFor, fromOptions, groupSkipped, ignorableChecks, scopeBody, undoGroups } from './bulkReview'

describe('bulk review options', () => {
  it('decisions and checks per kind', () => {
    expect(decisionsFor('requirement')).toEqual(['approved', 'rejected'])
    expect(decisionsFor('test')).toEqual(['ready'])
    expect(ignorableChecks('requirement')).toContain('no_obligation')
    expect(ignorableChecks('test')).toEqual(expect.arrayContaining(['no_steps', 'no_script']))
    expect(ignorableChecks('test')).not.toContain('no_obligation')
    expect(DEFAULT_FROM).toEqual(['review'])
  })
  it('from statuses leave out the target', () => {
    expect(fromOptions('requirement', 'approved')).not.toContain('approved')
    expect(fromOptions('requirement', 'rejected')).toContain('approved')
    expect(fromOptions('test', 'ready')).toEqual(['draft', 'review', 'deprecated'])
  })
})

describe('bulk review results', () => {
  const it1 = (id: string, status: string, reason?: string) => ({ id, key: id, title: id, status, reason })
  it('groups skipped items by reason', () => {
    const g = groupSkipped([it1('a', 'draft', 'status'), it1('b', 'review', 'suspect'), it1('c', 'draft', 'status'), it1('d', 'x')])
    expect(g.map((x) => [x.reason, x.items.length])).toEqual([
      ['status', 2],
      ['other', 1],
      ['suspect', 1],
    ])
  })
  it('groups updated items by previous status for undo', () => {
    expect(undoGroups([it1('a', 'review'), it1('b', 'draft'), it1('c', 'review')])).toEqual([
      { status: 'review', ids: ['a', 'c'] },
      { status: 'draft', ids: ['b'] },
    ])
  })
  it('scope bodies', () => {
    expect(scopeBody('all', ['a'], ['b'])).toEqual({ all: true })
    expect(scopeBody('selected', ['a'], ['b'])).toEqual({ ids: ['a'] })
    expect(scopeBody('shown', ['a'], ['b'])).toEqual({ ids: ['b'] })
  })
})
