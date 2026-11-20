// Test configurations of the current project for selects, filters and the
// settings page.
import { computed, ref } from 'vue'
import { api, asItems } from '@/lib/api'
import type { Configuration } from '@/lib/types'

export function useConfigurations(projectId: () => string) {
  const configurations = ref<Configuration[]>([])
  const loaded = ref(false)
  const options = computed(() => configurations.value.map((c) => ({ label: c.name, value: c.id })))
  const names = computed(() => new Map(configurations.value.map((c) => [c.id, c.name])))
  async function load() {
    try {
      configurations.value = asItems(await api.get<{ items: Configuration[] }>(`/projects/${projectId()}/configurations`))
    } catch {
      // without project.read there is nothing to choose from
      configurations.value = []
    } finally {
      loaded.value = true
    }
  }
  return { configurations, options, names, loaded, load }
}
