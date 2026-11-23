import { defineStore } from 'pinia'
import { api } from '@/lib/api'
import { debounce } from '@/lib/collections'
import { emptySummary, type WorkSummary } from '@/lib/worklist'
import { useLiveStore } from './live'

const POLL_MS = 60_000
/** with live updates the count follows changes, at most this often */
const LIVE_DEBOUNCE_MS = 3000
let unsubscribe: (() => void) | null = null

/** The summary of the signed-in user's work list, polled for the badge in
 *  the main navigation (GET /my-work/count). */
export const useWorkListStore = defineStore('worklist', {
  state: () => ({ summary: emptySummary() as WorkSummary, timer: 0 as ReturnType<typeof setInterval> | 0 }),
  getters: {
    count: (s) => s.summary.count,
  },
  actions: {
    async load() {
      try {
        // polling: no activity of the session (inactivity timeout)
        this.summary = { ...emptySummary(), ...(await api.get<WorkSummary>('/my-work/count', undefined, { background: true })) }
      } catch {
        /* the badge is a convenience */
      }
    },
    /** Load now and keep the count current: from live updates while the
     *  event stream is open, by polling otherwise (the fallback). */
    start() {
      if (this.timer) return
      void this.load()
      const live = useLiveStore()
      this.timer = setInterval(() => {
        if (!live.active) void this.load()
      }, POLL_MS)
      const soon = debounce(() => void this.load(), LIVE_DEBOUNCE_MS)
      unsubscribe = live.subscribe((ev) => {
        if (ev.kind === 'change' || ev.kind === 'notification' || ev.kind === 'reset') soon()
      })
    },
    stop() {
      if (this.timer) clearInterval(this.timer)
      this.timer = 0
      unsubscribe?.()
      unsubscribe = null
    },
  },
})
