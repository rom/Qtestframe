import { describe, expect, it } from 'vitest'
import { applyPresence, backoffDelay, makeThrottle, matches, newerVersion, presenceKey, streamUrl, type LiveEvent } from './live'

const change = (x: Partial<LiveEvent>): LiveEvent => ({ kind: 'change', ...x })

describe('live updates', () => {
  it('builds the stream URL', () => {
    expect(streamUrl()).toBe('/api/v1/events/stream')
    expect(streamUrl('ab-1 2')).toBe('/api/v1/events/stream?last_event_id=ab-1%202')
  })

  it('backs off exponentially with jitter, up to a minute', () => {
    expect(backoffDelay(0, () => 0.5)).toBe(1000)
    expect(backoffDelay(1, () => 0.5)).toBe(2000)
    expect(backoffDelay(3, () => 0)).toBe(6400)
    expect(backoffDelay(3, () => 1)).toBe(9600)
    expect(backoffDelay(50, () => 0.5)).toBe(60000)
    expect(backoffDelay(-3, () => 0.5)).toBe(1000)
  })

  it('matches events to the views they concern', () => {
    const ev = change({ type: 'execution', entity_id: 'e1', parent_id: 'run1', project_id: 'p1', action: 'execution.update', actor_id: 'u1' })
    expect(matches(ev, {})).toBe(true)
    expect(matches(ev, { types: ['execution'], projectId: 'p1' })).toBe(true)
    expect(matches(ev, { types: ['requirement'] })).toBe(false)
    expect(matches(ev, { projectId: 'p2' })).toBe(false)
    expect(matches(ev, { entityId: 'e1' })).toBe(true)
    expect(matches(ev, { entityId: 'e2' })).toBe(false)
    expect(matches(ev, { parentId: 'run1' })).toBe(true)
    expect(matches(change({ type: 'run', entity_id: 'run1' }), { parentId: 'run1' })).toBe(true)
    expect(matches(ev, { parentId: 'run2' })).toBe(false)
    expect(matches(ev, { actionPrefix: 'comment.' })).toBe(false)
    expect(matches(change({ action: 'comment.create' }), { actionPrefix: 'comment.' })).toBe(true)
    expect(matches(ev, { includeOwn: false, me: 'u1' })).toBe(false)
    expect(matches(ev, { me: 'u1' })).toBe(true)
    // resets concern everyone, notifications and presence no list
    expect(matches({ kind: 'reset' }, { types: ['defect'], projectId: 'x' })).toBe(true)
    expect(matches({ kind: 'notification' }, {})).toBe(false)
    expect(matches({ kind: 'presence', type: 'requirement' }, {})).toBe(false)
  })

  it('keeps the people on an item', () => {
    expect(presenceKey('test', 't1')).toBe('test:t1')
    let l = applyPresence([], { user_id: 'b', name: 'Bo', state: 'viewing' })
    l = applyPresence(l, { user_id: 'a', name: 'Al', state: 'viewing' })
    l = applyPresence(l, { user_id: 'b', name: 'Bo', state: 'editing' })
    expect(l).toEqual([
      { user_id: 'a', name: 'Al', state: 'viewing' },
      { user_id: 'b', name: 'Bo', state: 'editing' },
    ])
    expect(applyPresence(l, { user_id: 'a', name: 'Al', state: 'left' }).map((p) => p.user_id)).toEqual(['b'])
  })

  it('recognises a newer version of the open item', () => {
    const ev = change({ type: 'requirement', entity_id: 'r1', version: 4, actor_id: 'u2', actor_name: 'Bea', action: 'requirement.update' })
    expect(newerVersion(ev, 'requirement', 'r1', 3, 'me')).toEqual({ by: 'Bea', version: 4, deleted: false })
    expect(newerVersion(ev, 'requirement', 'r1', 4, 'me')).toBeNull()
    expect(newerVersion(ev, 'requirement', 'r2', 3, 'me')).toBeNull()
    expect(newerVersion(ev, 'test', 'r1', 3, 'me')).toBeNull()
    expect(newerVersion(ev, 'requirement', 'r1', 3, 'u2')).toBeNull() // own save
    expect(newerVersion({ ...ev, kind: 'presence' }, 'requirement', 'r1', 3, 'me')).toBeNull()
    expect(newerVersion({ ...ev, action: 'requirement.delete', version: 4 }, 'requirement', 'r1', 4, 'me')?.deleted).toBe(true)
    // unversioned items (executions): every change by someone else
    expect(newerVersion(change({ type: 'execution', entity_id: 'e', actor_id: 'x' }), 'execution', 'e', null, 'me')).toEqual({ by: '', version: 0, deleted: false })
  })

  it('throttles announcements', () => {
    let now = 0
    const ok = makeThrottle(1000, () => now)
    expect(ok('a')).toBe(true)
    expect(ok('b')).toBe(false)
    now = 1500
    expect(ok('a')).toBe(false) // the same text again so soon
    expect(ok('b')).toBe(true)
    now = 5000
    expect(ok('b')).toBe(true)
  })
})
