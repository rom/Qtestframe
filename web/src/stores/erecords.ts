import { defineStore } from 'pinia'
import { api } from '@/lib/api'
import type { ERecordsInfo, ERecordsSettings } from '@/lib/erecords'
import { useAuthStore } from '@/stores/auth'

/** The electronic-records settings of the current organisation (the signature dialogs adapt to them). */
export const useERecordsStore = defineStore('erecords', {
  state: () => ({ settings: null as ERecordsSettings | null, actions: [] as string[], edits: [] as string[], tenant: '' }),
  getters: {
    enabled: (s) => !!s.settings?.enabled,
    meanings: (s) => s.settings?.meanings ?? [],
  },
  actions: {
    async ensure(force = false) {
      const tid = useAuthStore().tenant?.id ?? ''
      if (!tid) {
        this.settings = null
        this.tenant = ''
        return
      }
      if (!force && this.tenant === tid && this.settings) return
      try {
        const r = await api.get<ERecordsInfo>('/erecords')
        this.settings = r.settings
        this.actions = r.signature_actions ?? []
        this.edits = r.reason_edits ?? []
        this.tenant = tid
      } catch {
        this.settings = null
      }
    },
    set(s: ERecordsSettings) {
      this.settings = s
    },
  },
})
