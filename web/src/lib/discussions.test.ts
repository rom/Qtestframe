import { describe, expect, it } from 'vitest'
import { commentsPath, groupThreads, liveCount, openCount, reactionList, threadTarget, wasEdited } from './discussions'
import type { Comment } from './types'

const c = (id: string, at: string, extra: Partial<Comment> = {}): Comment => ({
  id,
  project_id: 'p1',
  entity_type: 'requirement',
  entity_id: 'r1',
  parent_id: '',
  user_id: 'u1',
  user_email: 'a@example.com',
  user_name: 'A',
  body: id,
  created_at: at,
  updated_at: at,
  ...extra,
})

describe('groupThreads', () => {
  const list = [
    c('r2', '2026-01-02T00:00:00Z', { parent_id: 't1' }),
    c('t1', '2026-01-01T00:00:00Z'),
    c('t2', '2026-01-03T00:00:00Z', { resolved_at: '2026-01-04T00:00:00Z' }),
    c('r1', '2026-01-01T01:00:00Z', { parent_id: 't1', deleted: true, body: '' }),
    c('t3', '2026-01-05T00:00:00Z', { deleted: true, body: '' }),
    c('o1', '2026-01-06T00:00:00Z', { parent_id: 'gone' }),
  ]
  it('groups replies under their first comment, oldest first', () => {
    const th = groupThreads(list)
    expect(th.map((t) => t.root.id)).toEqual(['t1', 't2', 't3', 'o1'])
    expect(th[0].replies.map((r) => r.id)).toEqual(['r1', 'r2'])
    expect(th.map((t) => t.open)).toEqual([true, false, false, true])
    expect(th[2].removed).toBe(true)
  })
  it('counts', () => {
    expect(openCount(list)).toBe(2)
    expect(liveCount(list)).toBe(4)
  })
})

describe('comment helpers', () => {
  it('tells edits apart from writing', () => {
    expect(wasEdited(c('x', '2026-01-01T00:00:00Z'))).toBe(false)
    expect(wasEdited({ ...c('x', '2026-01-01T00:00:00Z'), updated_at: '2026-01-01T00:05:00Z' })).toBe(true)
    expect(wasEdited({ ...c('x', '2026-01-01T00:00:00Z'), updated_at: '2026-01-01T00:05:00Z', deleted: true })).toBe(false)
  })
  it('lists reactions in order with mine marked', () => {
    const r = reactionList({ reactions: { heart: ['u2'], '+1': ['u1', 'u2'], bogus: ['u1'] } }, 'u1')
    expect(r).toEqual([
      { key: '+1', emoji: '👍', count: 2, mine: true },
      { key: 'heart', emoji: '❤️', count: 1, mine: false },
    ])
    expect(reactionList({}, undefined)).toEqual([])
  })
  it('knows the paths and targets', () => {
    expect(commentsPath('run', 'x')).toBe('/runs/x/comments')
    expect(commentsPath('review', 'x')).toBe('/reviews/x/comments')
    const root = c('t1', '2026-01-01T00:00:00Z')
    expect(threadTarget({ root })).toEqual({ name: 'requirements', params: { projectId: 'p1' }, query: { item: 'r1', tab: 'comments' } })
    expect(threadTarget({ root: { ...root, entity_type: 'test' } }).name).toBe('tests')
    expect(threadTarget({ root: { ...root, entity_type: 'defect' } }).query).toEqual({ item: 'r1' })
    expect(threadTarget({ root: { ...root, entity_type: 'risk' } }).query).toEqual({ risk: 'r1' })
    expect(threadTarget({ root: { ...root, entity_type: 'run' } })).toEqual({ name: 'run', params: { projectId: 'p1', runId: 'r1' }, query: { tab: 'discussion' } })
    expect(threadTarget({ root: { ...root, entity_type: 'review' } })).toEqual({ name: 'myWork', params: {}, query: { view: 'reviews', review: 'r1' } })
    expect(threadTarget({ root: { ...root, entity_type: 'other' } }).name).toBe('dashboard')
  })
})
