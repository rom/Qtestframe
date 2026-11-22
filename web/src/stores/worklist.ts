import { defineStore } from 'pinia'
import { api } from '@/lib/api'
import { emptySummary, type WorkSummary } from '@/lib/worklist'

const POLL_MS = 60_000

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
        this.summary = { ...emptySummary(), ...(await api.get<WorkSummary>('/my-work/count')) }
      } catch {
        /* the badge is a convenience */
      }
    },
    start() {
      if (this.timer) return
      void this.load()
      this.timer = setInterval(() => void this.load(), POLL_MS)
    },
    stop() {
      if (this.timer) clearInterval(this.timer)
      this.timer = 0
    },
  },
})
