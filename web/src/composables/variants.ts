// Variants of the current project for selects, filters and the variant views.
import { computed, ref } from 'vue'
import { api, asItems } from '@/lib/api'
import { variantOptions, type Variant } from '@/lib/variants'

export function useVariants(projectId: () => string) {
  const variants = ref<Variant[]>([])
  const loaded = ref(false)
  const options = computed(() => variantOptions(variants.value))
  const names = computed(() => new Map(variants.value.map((v) => [v.id, v.name])))
  async function load() {
    try {
      variants.value = asItems(await api.get<{ items: Variant[] }>(`/projects/${projectId()}/variants`))
    } catch {
      // without project.read there is nothing to choose from
      variants.value = []
    } finally {
      loaded.value = true
    }
  }
  return { variants, options, names, loaded, load }
}
