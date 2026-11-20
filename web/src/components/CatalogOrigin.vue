<script setup lang="ts">
// A subtle note in the editors: the catalogue entry an item was copied
// from, with the standards it cites and a link to it in the catalogue.
import { ref, watch } from 'vue'
import { api } from '@/lib/api'
import { originText, type CatalogKind } from '@/lib/catalog'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ kind: CatalogKind; catalogRef: string; projectId: string }>()
const { t, locale } = useI18n()
const refs = ref<string[]>([])

watch(
  () => [props.kind, props.catalogRef],
  async () => {
    refs.value = []
    if (!props.catalogRef) return
    try {
      const d = await api.get<{ refs?: string[] }>(`/catalog/${props.kind === 'requirement' ? 'requirements' : 'tests'}/${encodeURIComponent(props.catalogRef)}`, {
        lang: String(locale.value).startsWith('sv') ? 'sv' : 'en',
      })
      refs.value = d?.refs ?? []
    } catch {
      /* the entry may no longer exist in this version of the catalogue */
    }
  },
  { immediate: true },
)
</script>

<template>
  <div v-if="catalogRef" class="catalog-origin small muted" :title="t('catalog.originHint', { id: catalogRef })">
    <i class="pi pi-book" aria-hidden="true" />
    {{ t('catalog.origin', { origin: originText(catalogRef, refs) }) }}
    <RouterLink :to="{ name: 'catalog', params: { projectId }, query: { tab: kind === 'test' ? 'tests' : undefined, item: catalogRef } }">{{ t('catalog.originOpen') }}</RouterLink>
  </div>
</template>

<style scoped>
.catalog-origin { display: flex; gap: 6px; align-items: baseline; flex-wrap: wrap; overflow-wrap: anywhere; }
</style>
