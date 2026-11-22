import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import { HIDDEN_GRACE_MS, START_DELAY_MS, setEventSourceFactory, useLiveStore, type EventSourceLike } from './live'
import { useAuthStore } from './auth'
import { useMetaStore } from './meta'
import type { LiveEvent } from '@/lib/live'
import type { Me } from '@/lib/types'

class FakeSource implements EventSourceLike {
  static all: FakeSource[] = []
  listeners: Record<string, ((e: MessageEvent) => void)[]> = {}
  onerror: ((e: Event) => void) | null = null
  closed = false
  constructor(public url: string) {
    FakeSource.all.push(this)
  }
  addEventListener(type: string, fn: (e: MessageEvent) => void) {
    ;(this.listeners[type] ??= []).push(fn)
  }
  close() {
    this.closed = true
  }
  emit(type: string, data: unknown, id = '') {
    const e = { data: JSON.stringify(data), lastEventId: id } as MessageEvent
    for (const fn of this.listeners[type] ?? []) fn(e)
  }
  fail() {
    this.onerror?.(new Event('error'))
  }
}
const last = () => FakeSource.all[FakeSource.all.length - 1]

let visibility: DocumentVisibilityState = 'visible'

function signIn(prefs: Record<string, unknown> = {}) {
  useMetaStore().meta.live_updates = true
  useAuthStore().me = { user: { id: 'me', email: 'me@example.com', preferences: prefs }, tenant: { id: 't1' } } as unknown as Me
}

describe('live store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    FakeSource.all = []
    setEventSourceFactory((url) => new FakeSource(url))
    visibility = 'visible'
    vi.spyOn(document, 'visibilityState', 'get').mockImplementation(() => visibility)
    vi.useFakeTimers()
  })
  afterEach(() => {
    setEventSourceFactory(null)
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('stays off without server support, sign-in, or when the user turned it off', async () => {
    const s = useLiveStore()
    s.start()
    expect(s.status).toBe('off')
    expect(FakeSource.all).toHaveLength(0)
    signIn({ live_updates_off: true })
    await nextTick()
    expect(s.enabled).toBe(false)
    expect(FakeSource.all).toHaveLength(0)
    signIn()
    await nextTick()
    expect(s.enabled).toBe(true)
    vi.advanceTimersByTime(START_DELAY_MS)
    expect(FakeSource.all).toHaveLength(1)
    expect(s.status).toBe('connecting')
  })

  it('opens one shared stream and hands events to the subscribers', async () => {
    signIn()
    const s = useLiveStore()
    s.start()
    s.start() // idempotent
    expect(FakeSource.all).toHaveLength(0) // the page loads first
    vi.advanceTimersByTime(START_DELAY_MS)
    expect(FakeSource.all).toHaveLength(1)
    const got: LiveEvent[] = []
    const off = s.subscribe((e) => got.push(e))
    last().emit('hello', { heartbeat_ms: 25000 })
    expect(s.status).toBe('open')
    expect(s.active).toBe(true)
    last().emit('change', { type: 'requirement', entity_id: 'r1', version: 2 }, 'a-1')
    expect(got).toEqual([{ kind: 'change', type: 'requirement', entity_id: 'r1', version: 2 }])
    expect(s.lastEventId).toBe('a-1')
    off()
    last().emit('change', { type: 'test', entity_id: 't1' }, 'a-2')
    expect(got).toHaveLength(1)
    // a broken listener does not stop the others
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    s.subscribe(() => {
      throw new Error('boom')
    })
    const second: LiveEvent[] = []
    s.subscribe((e) => second.push(e))
    last().emit('notification', {}, 'a-3')
    expect(second.map((e) => e.kind)).toEqual(['notification'])
    expect(warn).toHaveBeenCalled()
    // garbage is ignored
    last().listeners.change[0]({ data: '{oops', lastEventId: '' } as MessageEvent)
    expect(second).toHaveLength(1)
  })

  it('reconnects with backoff and resumes from the last event', async () => {
    signIn()
    const s = useLiveStore()
    s.start()
    vi.advanceTimersByTime(START_DELAY_MS)
    last().emit('hello', {})
    last().emit('change', { type: 'run', entity_id: 'x' }, 'abc-7')
    last().fail()
    expect(s.status).toBe('retrying')
    expect(FakeSource.all[0].closed).toBe(true)
    vi.advanceTimersByTime(1300)
    expect(FakeSource.all).toHaveLength(2)
    expect(last().url).toBe('/api/v1/events/stream?last_event_id=abc-7')
    // failing again waits longer (about 2 s)
    last().fail()
    vi.advanceTimersByTime(1300)
    expect(FakeSource.all).toHaveLength(2)
    vi.advanceTimersByTime(1300)
    expect(FakeSource.all).toHaveLength(3)
    // a reset forgets the position: the views reload
    const got: string[] = []
    s.subscribe((e) => got.push(e.kind))
    last().emit('reset', {})
    expect(got).toEqual(['reset'])
    expect(s.lastEventId).toBeNull()
    // shutdown of the server: reconnect; signed out: stop
    last().emit('hello', {})
    last().emit('bye', { reason: 'shutdown' })
    expect(s.status).toBe('retrying')
    vi.advanceTimersByTime(1300)
    expect(FakeSource.all).toHaveLength(4)
    last().emit('bye', { reason: 'unauthorized' })
    expect(s.status).toBe('off')
    vi.advanceTimersByTime(120_000)
    expect(FakeSource.all).toHaveLength(4)
  })

  it('pauses in a hidden tab and resumes when shown', async () => {
    signIn()
    const s = useLiveStore()
    s.start()
    vi.advanceTimersByTime(START_DELAY_MS)
    last().emit('hello', {})
    last().emit('change', { type: 'test', entity_id: 'x' }, 'id-1')
    visibility = 'hidden'
    document.dispatchEvent(new Event('visibilitychange'))
    vi.advanceTimersByTime(HIDDEN_GRACE_MS - 1000)
    expect(s.status).toBe('open') // a short switch keeps the stream
    vi.advanceTimersByTime(2000)
    expect(s.status).toBe('paused')
    expect(last().closed).toBe(true)
    visibility = 'visible'
    document.dispatchEvent(new Event('visibilitychange'))
    expect(FakeSource.all).toHaveLength(2)
    expect(last().url).toContain('last_event_id=id-1')
    // a retry that comes due while hidden waits for the tab
    last().fail()
    visibility = 'hidden'
    vi.advanceTimersByTime(5000)
    expect(s.status).toBe('paused')
    expect(FakeSource.all).toHaveLength(2)
  })

  it('keeps presence, restarts for another organisation and stops on sign-out', async () => {
    signIn()
    const s = useLiveStore()
    s.start()
    vi.advanceTimersByTime(START_DELAY_MS)
    last().emit('hello', {})
    last().emit('presence', { type: 'requirement', entity_id: 'r1', presence: { user_id: 'u2', name: 'Bea', state: 'viewing' } })
    last().emit('presence', { type: 'requirement', entity_id: 'r1', presence: { user_id: 'u1', name: 'Al', state: 'editing' } })
    expect(s.presence['requirement:r1'].map((p) => p.name)).toEqual(['Al', 'Bea'])
    last().emit('presence', { type: 'requirement', entity_id: 'r1', presence: { user_id: 'u2', name: 'Bea', state: 'left' } })
    expect(s.presence['requirement:r1'].map((p) => p.name)).toEqual(['Al'])
    s.setPresence('test', 't1', [
      { user_id: 'z', name: 'Zed', state: 'viewing' },
      { user_id: 'y', name: 'Ann', state: 'editing' },
    ])
    expect(s.presence['test:t1'].map((p) => p.name)).toEqual(['Ann', 'Zed'])
    s.restart()
    expect(FakeSource.all).toHaveLength(2)
    expect(last().url).toBe('/api/v1/events/stream')
    expect(s.presence).toEqual({})
    s.stop()
    expect(s.status).toBe('off')
    expect(last().closed).toBe(true)
  })

  it('announces politely and rarely', () => {
    const s = useLiveStore()
    s.announce('The list was updated')
    expect(s.announcement).toBe('The list was updated')
    s.announce('Something else')
    expect(s.announcement).toBe('The list was updated')
    vi.advanceTimersByTime(31_000)
    s.announce('Something else')
    expect(s.announcement).toBe('Something else')
    s.announce('')
    expect(s.announcement).toBe('Something else')
  })

  it('stays off without EventSource in the browser', () => {
    setEventSourceFactory(null)
    const had = (globalThis as { EventSource?: unknown }).EventSource
    delete (globalThis as { EventSource?: unknown }).EventSource
    signIn()
    const s = useLiveStore()
    s.start()
    vi.advanceTimersByTime(START_DELAY_MS)
    expect(s.status).toBe('off')
    if (had) (globalThis as { EventSource?: unknown }).EventSource = had
  })
})
