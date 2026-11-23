// The environments a project can use (its own and the organisation's shared
// ones) for pickers, filters and the environments page.
import { computed, ref } from 'vue'
import { api, asItems } from '@/lib/api'
import { bookable, type Environment } from '@/lib/environments'

export function useEnvironments(projectId: () => string) {
  const environments = ref<Environment[]>([])
  const loaded = ref(false)
  const names = computed(() => new Map(environments.value.map((e) => [e.id, e.name])))
  /** Options of a picker: bookable environments (and the current one even when retired). */
  const options = (current?: string | null) =>
    environments.value.filter((e) => bookable(e) || e.id === current).map((e) => ({ label: e.name, value: e.id, env: e }))
  async function load() {
    try {
      environments.value = asItems(await api.get<{ items: Environment[] }>(`/projects/${projectId()}/environments`))
    } catch {
      environments.value = []
    } finally {
      loaded.value = true
    }
  }
  const byId = (id: string | null | undefined) => environments.value.find((e) => e.id === id)
  return { environments, names, options, byId, loaded, load }
}
