import { defineStore } from 'pinia'
import { api } from '@/lib/api'
import type { AppNotification } from '@/lib/types'

const POLL_MS = 60_000

/** The signed-in user's notifications; the unread count is polled. */
export const useNotificationsStore = defineStore('notifications', {
  state: () => ({ items: [] as AppNotification[], unread: 0, timer: 0 as ReturnType<typeof setInterval> | 0 }),
  actions: {
    async load(unreadOnly = false, background = false) {
      try {
        const res = await api.get<{ items: AppNotification[]; unread: number }>('/notifications', unreadOnly ? { unread: 1 } : undefined, { background })
        this.items = res.items ?? []
        this.unread = res.unread ?? 0
      } catch {
        /* notifications are a convenience */
      }
    },
    async markRead(ids?: string[]) {
      await api.post('/notifications/read', ids ? { ids } : {})
      const now = new Date().toISOString()
      for (const n of this.items) if (!ids || ids.includes(n.id)) n.read_at = n.read_at ?? now
      this.unread = ids ? Math.max(0, this.unread - ids.length) : 0
    },
    async markUnread(ids: string[]) {
      const res = await api.post<{ marked: number }>('/notifications/unread', { ids })
      for (const n of this.items) if (ids.includes(n.id)) n.read_at = null
      this.unread += res.marked
    },
    /** Delete notifications: the given ones, or all read ones. */
    async remove(which: string[] | 'read') {
      const res = await api.del<{ deleted: number }>('/notifications', which === 'read' ? { read: true } : { ids: which })
      const gone = (n: AppNotification) => (which === 'read' ? !!n.read_at : which.includes(n.id))
      this.unread -= this.items.filter((n) => gone(n) && !n.read_at).length
      this.items = this.items.filter((n) => !gone(n))
      return res.deleted
    },
    start() {
      if (this.timer) return
      void this.load()
      this.timer = setInterval(() => void this.load(false, true), POLL_MS)
    },
    stop() {
      if (this.timer) clearInterval(this.timer)
      this.timer = 0
    },
  },
})
