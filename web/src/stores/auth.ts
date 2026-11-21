import { defineStore } from 'pinia'
import { api, ApiError, setCsrfToken } from '@/lib/api'
import { can } from '@/lib/permissions'
import { useProjectStore } from '@/stores/project'
import type { Me } from '@/lib/types'
import { isChallenge, type MfaChallenge } from '@/lib/mfa'
import { clearSuggestCache } from '@/lib/suggest'

export const useAuthStore = defineStore('auth', {
  state: () => ({ me: null as Me | null, loaded: false }),
  getters: {
    user: (s) => s.me?.user ?? null,
    tenant: (s) => s.me?.tenant ?? null,
    isSuperAdmin: (s) => !!s.me?.user?.super_admin,
  },
  actions: {
    /** Whether the user holds perm; inside a project (ProjectLayout) with the project's effective permissions. */
    can(perm: string | string[]): boolean {
      const proj = useProjectStore()
      return can(this.me, perm, proj.scoped ? proj.project?.my_access : null)
    },
    set(me: Me | null) {
      // cached autocomplete answers belong to one user in one organisation (and preference)
      const ac = (m: Me | null) => `${m?.user?.id}|${m?.tenant?.id}|${m?.user?.preferences?.autocomplete ?? ''}`
      if (ac(me) !== ac(this.me)) clearSuggestCache()
      this.me = me
      this.loaded = true
      setCsrfToken(me?.csrf_token)
    },
    clear() {
      this.set(null)
    },
    async fetchMe(): Promise<Me | null> {
      try {
        this.set(await api.get<Me>('/auth/me', undefined, { noAuthRedirect: true }))
      } catch (e) {
        if (!(e instanceof ApiError) || e.status !== 401) console.warn('auth/me failed', e)
        this.set(null)
      }
      return this.me
    },
    /** Password sign-in: the session (Me), or the second factor still to give. */
    async login(email: string, password: string, tenant?: string): Promise<Me | MfaChallenge> {
      const res = await api.post<Me | MfaChallenge>('/auth/login', { email, password, tenant: tenant || undefined }, { noAuthRedirect: true })
      if (isChallenge(res)) return res
      this.set(res)
      return res
    },
    /** The second factor of a pending sign-in (authenticator or recovery code). */
    async verifyMfa(code: string, remember: boolean) {
      const me = await api.post<Me>('/auth/mfa/verify', { code, remember }, { noAuthRedirect: true })
      this.set(me)
      return me
    },
    async logout() {
      try {
        await api.post('/auth/logout', undefined, { noAuthRedirect: true })
      } finally {
        this.clear()
      }
    },
    async switchTenant(tenantId: string) {
      this.set(await api.post<Me>('/auth/switch-tenant', { tenant_id: tenantId }))
    },
  },
})
