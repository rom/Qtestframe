import { defineStore } from 'pinia'
import { api, asItems } from '@/lib/api'
import type { RoleInfo } from '@/lib/types'
import { customRoleLabels } from '@/i18n/labels'
import { useAuthStore } from './auth'

/** The roles of the current organisation (GET /roles), with custom role labels. */
export const useRolesStore = defineStore('roles', {
  state: () => ({ roles: [] as RoleInfo[], permissions: [] as string[], tenant: '', loading: null as Promise<void> | null }),
  actions: {
    ensure(force = false): Promise<void> {
      const tenant = useAuthStore().tenant?.id ?? ''
      if (!force && this.tenant === tenant && this.loading) return this.loading
      this.tenant = tenant
      this.loading = (async () => {
        if (!tenant) {
          this.roles = []
          return
        }
        try {
          const res = await api.get<{ items: RoleInfo[]; permissions: string[] }>('/roles')
          this.roles = asItems(res)
          this.permissions = res.permissions ?? []
          for (const r of this.roles) if (!r.builtin) customRoleLabels.set(r.name, r.label || r.name)
        } catch {
          this.loading = null // try again next time
        }
      })()
      return this.loading
    },
  },
})
