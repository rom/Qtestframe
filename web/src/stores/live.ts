import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import { applyPresence, backoffDelay, makeThrottle, presenceKey, streamUrl, type LiveEvent, type LiveKind, type LivePresenceInfo } from '@/lib/live'
import { useAuthStore } from './auth'
import { useMetaStore } from './meta'

/**
 * off: not connected (turned off, signed out, not started); connecting /
 * retrying: opening the stream (again); open: events arrive; paused: the tab
 * is in the background (the stream is closed until it is shown again).
 */
export type LiveStatus = 'off' | 'connecting' | 'open' | 'retrying' | 'paused'

type Listener = (ev: LiveEvent) => void

/** The part of EventSource the store uses (tests pass a fake). */
export interface EventSourceLike {
  addEventListener(type: string, fn: (e: MessageEvent) => void): void
  close(): void
  onerror: ((e: Event) => void) | null
}
type Factory = (url: string) => EventSourceLike

let factory: Factory | null = null
/** Replace how the stream is opened (tests); null = the browser's EventSource. */
export function setEventSourceFactory(f: Factory | null): void {
  factory = f
}
function openSource(url: string): EventSourceLike | null {
  if (factory) return factory(url)
  if (typeof EventSource === 'undefined') return null
  return new EventSource(url) as unknown as EventSourceLike
}

/** A tab in the background keeps its stream this long, then closes it. */
export const HIDDEN_GRACE_MS = 60_000
/**
 * The stream opens this long after the app started: the page loads its
 * data first (and tools that wait for an idle network see one).
 */
export const START_DELAY_MS = 2000
/** At most one screen reader announcement per this interval. */
export const ANNOUNCE_INTERVAL_MS = 30_000

const KINDS: LiveKind[] = ['change', 'notification', 'presence', 'reset']

// one document listener for the current store (a new Pinia replaces it)
let visibilityHandler: (() => void) | null = null
let visibilityHooked = false

/**
 * The single shared event stream of the web client: reconnects with
 * backoff, resumes with the last event id, pauses while the tab is hidden,
 * keeps who is on which item (presence) and hands every event to the
 * subscribed views. Off when the server or the user turned live updates off;
 * the views then keep polling or reloading as before.
 */
export const useLiveStore = defineStore('live', () => {
  const auth = useAuthStore()
  const meta = useMetaStore()
  const status = ref<LiveStatus>('off')
  const lastEventId = ref<string | null>(null)
  const presence = ref<Record<string, LivePresenceInfo[]>>({})
  /** polite live region text (AppLayout) */
  const announcement = ref('')
  const listeners = new Set<Listener>()
  let source: EventSourceLike | null = null
  let attempt = 0
  let retryTimer: ReturnType<typeof setTimeout> | null = null
  let startTimer: ReturnType<typeof setTimeout> | null = null
  let notBefore = 0
  let hiddenTimer: ReturnType<typeof setTimeout> | null = null
  let wanted = false
  const throttle = makeThrottle(ANNOUNCE_INTERVAL_MS)

  /** live updates may be used: server on, signed in, not turned off by the user */
  const enabled = computed(() => !!meta.meta.live_updates && !!auth.me?.user && !auth.me.user.preferences?.live_updates_off)
  /** events arrive now (polling can pause) */
  const active = computed(() => status.value === 'open')

  function emit(ev: LiveEvent) {
    for (const l of [...listeners]) {
      try {
        l(ev)
      } catch (e) {
        console.warn('live listener failed', e)
      }
    }
  }

  /** Receive every event; returns the function that unsubscribes. */
  function subscribe(fn: Listener): () => void {
    listeners.add(fn)
    return () => {
      listeners.delete(fn)
    }
  }

  function clearTimers() {
    if (retryTimer) clearTimeout(retryTimer)
    if (startTimer) clearTimeout(startTimer)
    retryTimer = startTimer = null
  }

  function disconnect(next: LiveStatus) {
    clearTimers()
    source?.close()
    source = null
    status.value = next
  }

  function hidden() {
    return typeof document !== 'undefined' && document.visibilityState === 'hidden'
  }

  /** Handle one message of the stream. */
  function handle(kind: LiveKind, m: MessageEvent) {
    if (m.lastEventId) lastEventId.value = m.lastEventId
    let ev: LiveEvent
    try {
      ev = { ...(JSON.parse(String(m.data || '{}')) as LiveEvent), kind }
    } catch {
      return
    }
    if (kind === 'presence' && ev.presence && ev.type && ev.entity_id) {
      const k = presenceKey(ev.type, ev.entity_id)
      presence.value = { ...presence.value, [k]: applyPresence(presence.value[k] ?? [], ev.presence) }
    }
    if (kind === 'reset') lastEventId.value = null
    emit(ev)
  }

  function connect() {
    clearTimers()
    source?.close()
    status.value = attempt ? 'retrying' : 'connecting'
    const es = openSource(streamUrl(lastEventId.value))
    if (!es) {
      status.value = 'off'
      return
    }
    source = es
    const opened = () => {
      if (source !== es) return
      status.value = 'open'
      attempt = 0
    }
    es.addEventListener('open', opened)
    es.addEventListener('hello', opened)
    for (const k of KINDS) es.addEventListener(k, (m) => source === es && handle(k, m))
    es.addEventListener('bye', (m) => {
      if (source !== es) return
      let reason = ''
      try {
        reason = (JSON.parse(String(m.data || '{}')) as { reason?: string }).reason ?? ''
      } catch {
        /* no reason */
      }
      if (reason === 'unauthorized') disconnect('off')
      else retry()
    })
    // the browser would retry by itself, but not after an error status
    // (503 at the connection limit): one place decides, with backoff
    es.onerror = () => {
      if (source === es) retry()
    }
  }

  function retry() {
    source?.close()
    source = null
    clearTimers()
    status.value = 'retrying'
    const delay = backoffDelay(attempt++)
    retryTimer = setTimeout(() => {
      retryTimer = null
      if (!wanted || !enabled.value) status.value = 'off'
      else if (hidden()) status.value = 'paused'
      else connect()
    }, delay)
  }

  function sync() {
    if (!wanted || !enabled.value) {
      disconnect('off')
      presence.value = {}
      return
    }
    if (hidden() || source || retryTimer || startTimer) return
    const wait = notBefore - Date.now()
    if (wait > 0) {
      startTimer = setTimeout(() => {
        startTimer = null
        sync()
      }, wait)
      return
    }
    connect()
  }

  function onVisibility() {
    if (hidden()) {
      if (hiddenTimer) clearTimeout(hiddenTimer)
      hiddenTimer = setTimeout(() => {
        hiddenTimer = null
        if (hidden() && status.value !== 'off') disconnect('paused')
      }, HIDDEN_GRACE_MS)
      return
    }
    if (hiddenTimer) clearTimeout(hiddenTimer)
    hiddenTimer = null
    if (wanted && enabled.value && !source) {
      attempt = 0
      connect() // resumes with the last event id: missed events arrive
    }
  }

  /** Open the stream (once signed in); idempotent. */
  function start() {
    if (!wanted) notBefore = Date.now() + START_DELAY_MS
    wanted = true
    visibilityHandler = onVisibility
    if (!visibilityHooked && typeof document !== 'undefined') {
      visibilityHooked = true
      document.addEventListener('visibilitychange', () => visibilityHandler?.())
    }
    sync()
  }

  /** Close the stream (sign-out). */
  function stop() {
    wanted = false
    lastEventId.value = null
    sync()
  }

  /** Reconnect from scratch (another organisation was chosen). */
  function restart() {
    lastEventId.value = null
    presence.value = {}
    attempt = 0
    disconnect('off')
    sync()
  }

  watch(enabled, () => sync())

  /** Replace who else is on an item (answer of POST /presence). */
  function setPresence(type: string, id: string, others: LivePresenceInfo[]) {
    presence.value = { ...presence.value, [presenceKey(type, id)]: others.reduce<LivePresenceInfo[]>((l, o) => applyPresence(l, o), []) }
  }

  /** Tell screen reader users, politely and rarely (throttled). */
  function announce(text: string) {
    if (text && throttle(text)) announcement.value = text
  }

  return { status, lastEventId, presence, announcement, enabled, active, subscribe, start, stop, restart, setPresence, announce, handle }
})
