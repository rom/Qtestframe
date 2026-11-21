import { defineStore } from 'pinia'
import { api, asItems } from '@/lib/api'
import type { Category, Folder, ItemKind, ListResponse, Project } from '@/lib/types'

export const useProjectStore = defineStore('project', {
  state: () => ({
    project: null as Project | null,
    folders: { requirement: [] as Folder[], test: [] as Folder[] } as Record<ItemKind, Folder[]>,
    categories: [] as Category[],
    loading: false,
    /** set while a project page is shown: permission checks then use the project's (see auth.can) */
    scoped: false,
  }),
  getters: {
    pid: (s) => s.project?.id ?? '',
  },
  actions: {
    async open(id: string) {
      if (this.project?.id === id) return
      this.loading = true
      try {
        this.project = await api.get<Project>(`/projects/${id}`)
        this.folders = { requirement: [], test: [] }
        this.categories = []
        await Promise.all([this.loadFolders('requirement'), this.loadFolders('test'), this.loadCategories()])
      } finally {
        this.loading = false
      }
    },
    reset() {
      this.project = null
    },
    /** Reloads the open project (e.g. after its sharing changed). */
    async refresh() {
      if (!this.project) return
      this.project = await api.get<Project>(`/projects/${this.project.id}`)
    },
    async loadFolders(kind: ItemKind) {
      if (!this.project) return
      const res = await api.get<ListResponse<Folder> | Folder[]>(`/projects/${this.project.id}/folders`, { kind })
      this.folders[kind] = asItems(res)
    },
    async loadCategories() {
      if (!this.project) return
      const res = await api.get<ListResponse<Category> | Category[]>(`/projects/${this.project.id}/categories`)
      this.categories = asItems(res)
    },
    categoriesFor(scope: ItemKind): Category[] {
      return this.categories.filter((c) => c.scope === scope)
    },
  },
})
