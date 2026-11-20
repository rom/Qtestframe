import { defineStore } from 'pinia'
import { api, asItems } from '@/lib/api'
import type { Person } from '@/lib/mentions'
import { usePeopleStore } from './people'

const MAX_AGE_MS = 60_000

/**
 * The people who can be @mentioned in a project (GET /projects/{pid}/people):
 * members of the organisation and other names used as owner or assignee.
 * Loaded once per project and refreshed after a minute.
 */
export const useProjectPeopleStore = defineStore('projectPeople', {
  state: () => ({ projectId: '', people: [] as Person[], loadedAt: 0, loading: null as Promise<void> | null }),
  actions: {
    ensure(projectId: string | undefined | null): Promise<void> {
      if (!projectId) return Promise.resolve()
      const fresh = this.projectId === projectId && Date.now() - this.loadedAt < MAX_AGE_MS
      if (fresh && this.loading) return this.loading
      if (this.projectId !== projectId) this.people = []
      this.projectId = projectId
      this.loadedAt = Date.now()
      this.loading = (async () => {
        try {
          const list = asItems(await api.get<{ items: Person[] }>(`/projects/${projectId}/people`))
          if (this.projectId !== projectId) return
          this.people = list
          const avatars = usePeopleStore()
          for (const p of list) if (p.kind === 'user') avatars.remember({ user_id: p.user_id, email: p.email, avatar: p.avatar ?? '' })
        } catch {
          this.loadedAt = 0 // retry on the next use; mentions are a convenience
        }
      })()
      return this.loading
    },
    /** Forget the cache, e.g. after a new owner name was saved. */
    invalidate() {
      this.loadedAt = 0
    },
  },
})
