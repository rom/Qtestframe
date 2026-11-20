// Releases of the current project for selects and run lists (names by id).
import { computed, ref } from 'vue'
import { api, asItems } from '@/lib/api'
import type { Release } from '@/lib/types'
import { t } from '@/i18n'

export function useReleaseOptions(projectId: () => string) {
  const releases = ref<Release[]>([])
  const names = computed(() => new Map(releases.value.map((r) => [r.id, r.name])))
  /** Select options: "No release" first, then the releases (newest first, as the API lists them). */
  const options = computed(() => [
    { label: t('releasesView.noRelease'), value: '' },
    ...releases.value.map((r) => ({ label: r.version && r.version !== r.name ? `${r.name} (${r.version})` : r.name, value: r.id })),
  ])
  async function load() {
    try {
      releases.value = asItems(await api.get<{ items: Release[] }>(`/projects/${projectId()}/releases`))
    } catch {
      // without project.read the release column stays empty
      releases.value = []
    }
  }
  return { releases, names, options, load }
}
