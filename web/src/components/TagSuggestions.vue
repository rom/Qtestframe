<script setup lang="ts">
// Tag suggestions under a tags field, from the item's title and description
// (POST /projects/{pid}/tags/suggest). Tags the project already uses come
// first; selecting a suggestion adds it to the item's tags.
import { tagClass } from '@/lib/tagColors'
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, asItems } from '@/lib/api'
import { debounce } from '@/lib/collections'

interface Suggestion {
  tag: string
  reason: 'existing' | 'acronym' | 'keyword' | string
  score: number
  uses?: number
}

const model = defineModel<string[]>({ default: () => [] })
const props = defineProps<{ projectId: string; title: string; description: string; disabled?: boolean }>()
const { t } = useI18n()
const suggestions = ref<Suggestion[]>([])
let seq = 0

const fetchSuggestions = debounce(async () => {
  const text = `${props.title ?? ''} ${props.description ?? ''}`.trim()
  const n = ++seq
  if (!text || props.disabled) {
    suggestions.value = []
    return
  }
  try {
    const list = asItems(await api.post<{ items: Suggestion[] }>(`/projects/${props.projectId}/tags/suggest`, { title: props.title, description: props.description, tags: model.value }))
    if (n === seq) suggestions.value = list
  } catch {
    if (n === seq) suggestions.value = [] // suggestions are a convenience
  }
}, 600)

watch(() => [props.title, props.description, props.projectId, props.disabled], () => fetchSuggestions(), { immediate: true })
onBeforeUnmount(() => fetchSuggestions.cancel())

const visible = () => suggestions.value.filter((s) => !model.value.some((t) => t.toLowerCase() === s.tag.toLowerCase()))
function add(s: Suggestion) {
  if (!model.value.some((t) => t.toLowerCase() === s.tag.toLowerCase())) model.value = [...model.value, s.tag]
}
function hint(s: Suggestion): string {
  if (s.reason === 'existing') return t('tagSuggest.existing', { count: s.uses ?? 0 })
  if (s.reason === 'acronym') return t('tagSuggest.acronym')
  return t('tagSuggest.keyword')
}
</script>

<template>
  <div v-if="!disabled && visible().length" class="tag-suggestions row small" role="group" :aria-label="t('tagSuggest.label')">
    <span class="muted"><i class="pi pi-lightbulb" aria-hidden="true" /> {{ t('tagSuggest.label') }}</span>
    <button
      v-for="s in visible()"
      :key="s.tag"
      type="button"
      class="tag-suggestion tag-colored"
      :class="[tagClass(s.tag), { existing: s.reason === 'existing' }]"
      :title="hint(s)"
      :aria-label="t('tagSuggest.add', { tag: s.tag, hint: hint(s) })"
      @click="add(s)"
    >
      <i class="pi pi-plus" aria-hidden="true" />{{ s.tag }}
    </button>
  </div>
</template>

<style>
.tag-suggestions {
  gap: 6px;
  flex-wrap: wrap;
  margin-top: 2px;
}
.tag-suggestion {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 1px 8px;
  border-radius: 999px;
  border: 1px dashed var(--qtf-border);
  background: none;
  color: var(--qtf-text);
  font: inherit;
  cursor: pointer;
}
.tag-suggestion.existing {
  border-style: solid;
  border-color: var(--p-primary-color);
}
.tag-suggestion:hover {
  background: var(--qtf-hover);
}
.tag-suggestion:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 1px;
}
.tag-suggestion .pi {
  font-size: 10px;
}
</style>
