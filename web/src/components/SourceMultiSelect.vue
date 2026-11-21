<script setup lang="ts">
// Choose reference sources of the catalogue (laws, standards, best
// practices; GET /catalog/sources), grouped by kind and searchable.
import { computed, ref, watch } from 'vue'
import MultiSelect from 'primevue/multiselect'
import { api } from '@/lib/api'
import { groupSources, type CatalogSources } from '@/lib/catalog'
import { useI18n } from 'vue-i18n'

const model = defineModel<string[]>({ default: () => [] })
defineProps<{ placeholder?: string; ariaLabel?: string; inputId?: string }>()
const emit = defineEmits<{ loaded: [data: CatalogSources | null] }>()
const { locale } = useI18n()
const data = ref<CatalogSources | null>(null)
const lang = computed(() => (String(locale.value).startsWith('sv') ? 'sv' : 'en'))

watch(
  lang,
  async (l) => {
    try {
      data.value = await api.get<CatalogSources>('/catalog/sources', { lang: l })
    } catch {
      data.value = null
    }
    emit('loaded', data.value)
  },
  { immediate: true },
)
const options = computed(() =>
  groupSources(data.value?.sources ?? [], data.value?.kinds ?? []).map((g) => ({
    label: g.label,
    items: g.sources.map((s) => ({ label: s.short && s.short !== s.name ? `${s.short} – ${s.name}` : s.name, value: s.id })),
  })),
)
</script>

<template>
  <MultiSelect
    v-model="model"
    :input-id="inputId"
    :options="options"
    option-label="label"
    option-value="value"
    option-group-label="label"
    option-group-children="items"
    filter
    display="chip"
    :max-selected-labels="4"
    :placeholder="placeholder"
    :aria-label="ariaLabel"
    class="source-select"
  />
</template>

<style scoped>
.source-select { min-width: 16rem; max-width: 100%; }
</style>
