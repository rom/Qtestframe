// Views and live updates: refresh what a view shows when others change it,
// and tell others who is viewing or editing an item (presence).
import { computed, onBeforeUnmount, onMounted, watch } from 'vue'
import { api } from '@/lib/api'
import { debounce } from '@/lib/collections'
import { matches, presenceKey, type LiveEvent, type LiveFilter, type LivePresenceInfo } from '@/lib/live'
import { useAuthStore } from '@/stores/auth'
import { useLiveStore } from '@/stores/live'

export interface LiveRefreshOptions {
  /** wait this long after the last event before reloading (default 800 ms) */
  delay?: number
  /** announced (politely, throttled) when others changed what is shown */
  announce?: () => string
}

/**
 * Call reload (debounced) when a change event matches the filter, or after
 * a reset (events were missed). filter returns null while there is nothing
 * to refresh. reload should keep the selection and scroll position: reload
 * the rows without showing a spinner.
 */
export function useLiveRefresh(filter: () => LiveFilter | null, reload: () => unknown, opts: LiveRefreshOptions = {}) {
  const live = useLiveStore()
  const auth = useAuthStore()
  let byOthers = false
  const run = debounce(() => {
    void reload()
    if (byOthers && opts.announce) live.announce(opts.announce())
    byOthers = false
  }, opts.delay ?? 800)
  let off: (() => void) | null = null
  onMounted(() => {
    off = live.subscribe((ev: LiveEvent) => {
      const f = filter()
      if (!f || !matches(ev, { ...f, me: auth.user?.id })) return
      if (ev.kind === 'change' && ev.actor_id !== auth.user?.id) byOthers = true
      run()
    })
  })
  onBeforeUnmount(() => {
    off?.()
    run.cancel()
  })
}

/** Presence heartbeat interval (the server forgets after live.presence_ttl). */
export const PRESENCE_BEAT_MS = 20_000

/**
 * Tell the server that the signed-in user views (or, while editing() is
 * true, edits) an item, every 20 s while it is open and the tab is shown;
 * "left" when it closes. Returns the others on the item.
 */
export function usePresence(type: () => string, id: () => string | null | undefined, editing: () => boolean) {
  const live = useLiveStore()
  let timer: ReturnType<typeof setInterval> | null = null
  let current: { type: string; id: string } | null = null

  const others = computed<LivePresenceInfo[]>(() => {
    const i = id()
    return i && live.enabled ? (live.presence[presenceKey(type(), i)] ?? []) : []
  })

  async function send(t: string, i: string, state: string) {
    try {
      const res = await api.post<{ others: LivePresenceInfo[] }>('/presence', { entity_type: t, entity_id: i, state })
      if (state !== 'left' && res?.others) live.setPresence(t, i, res.others)
    } catch {
      /* presence is a convenience */
    }
  }
  function beat() {
    if (!current || !live.enabled) return
    if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return
    void send(current.type, current.id, editing() ? 'editing' : 'viewing')
  }
  function leave() {
    if (current && live.enabled) void send(current.type, current.id, 'left')
    current = null
    if (timer) clearInterval(timer)
    timer = null
  }
  function follow() {
    const i = id()
    const t = type()
    if (current && current.id === i && current.type === t && live.enabled) return
    leave()
    if (!i || !live.enabled) return
    current = { type: t, id: i }
    beat()
    timer = setInterval(beat, PRESENCE_BEAT_MS)
  }

  watch([() => id(), () => type(), () => live.enabled], follow)
  watch(editing, (now, before) => {
    if (now !== before) beat()
  })
  // when the stream (re)connects, presence events sent before were missed:
  // announce again, the answer brings who else is on the item now
  watch(
    () => live.active,
    (now, before) => {
      if (now && !before) beat()
    },
  )
  const onVisible = () => {
    if (typeof document !== 'undefined' && document.visibilityState === 'visible') beat()
  }
  onMounted(() => {
    follow()
    document.addEventListener('visibilitychange', onVisible)
  })
  onBeforeUnmount(() => {
    document.removeEventListener('visibilitychange', onVisible)
    leave()
  })
  return { others }
}
