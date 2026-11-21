import { defineStore } from 'pinia'
import { api, ApiError, setCsrfToken } from '@/lib/api'
import { can } from '@/lib/permissions'
import { useProjectStore } from '@/stores/project'
import type { Me } from '@/lib/types'

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
    async login(email: string, password: string, tenant?: string) {
      const me = await api.post<Me>('/auth/login', { email, password, tenant: tenant || undefined }, { noAuthRedirect: true })
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
