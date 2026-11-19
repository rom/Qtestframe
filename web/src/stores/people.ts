import { defineStore } from 'pinia'
import { api, asItems } from '@/lib/api'
import { P } from '@/lib/permissions'
import type { Membership } from '@/lib/types'
import { useAuthStore } from './auth'

/**
 * Avatars of other users, looked up by id or e-mail. Filled from the member
 * list of the current tenant (when the server includes `avatar` there) and
 * from the signed-in user.
 */
export const usePeopleStore = defineStore('people', {
  state: () => ({ byId: {} as Record<string, string>, byEmail: {} as Record<string, string>, tenant: '' as string, loading: null as Promise<void> | null }),
  actions: {
    remember(m: { user_id?: string; id?: string; email?: string; avatar?: string }) {
      if (m.avatar === undefined) return
      const id = m.user_id ?? m.id
      if (id) this.byId[id] = m.avatar
      if (m.email) this.byEmail[m.email.toLowerCase()] = m.avatar
    },
    /** Load the member list once per tenant (needs member.read; silently skipped otherwise). */
    ensure(): Promise<void> {
      const auth = useAuthStore()
      const tenant = auth.tenant?.id ?? ''
      if (this.tenant === tenant && this.loading) return this.loading
      this.tenant = tenant
      this.loading = (async () => {
        if (!tenant || !auth.can(P.memberRead)) return
        try {
          for (const m of asItems(await api.get<{ items: Membership[] }>('/members'))) this.remember(m)
        } catch {
          /* avatars are decoration */
        }
      })()
      return this.loading
    },
    avatarFor(who: { id?: string | null; email?: string | null; avatar?: string | null }): string {
      if (who.avatar) return who.avatar
      const auth = useAuthStore()
      const me = auth.user
      if (me && ((who.id && who.id === me.id) || (who.email && who.email.toLowerCase() === me.email.toLowerCase()))) return me.avatar ?? ''
      if (who.id && this.byId[who.id] !== undefined) return this.byId[who.id]
      if (who.email && this.byEmail[who.email.toLowerCase()] !== undefined) return this.byEmail[who.email.toLowerCase()]
      return ''
    },
  },
})
