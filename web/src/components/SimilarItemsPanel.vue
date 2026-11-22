<script setup lang="ts">
// "Similar requirements" (or tests) while an item is written: the five
// most similar items of the project (POST /projects/{pid}/similar), asked
// again a moment after the text stops changing. A score at or above the
// project's duplicate threshold marks a likely duplicate.
import ItemRef from './ItemRef.vue'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import Tag from 'primevue/tag'
import { api } from '@/lib/api'
import { debounce } from '@/lib/collections'
import { itemRoute } from '@/lib/consistency'
import { percent, scoreSeverity, similarBody, similarKey, type SimilarHit, type SimilarKind, type SimilarResponse } from '@/lib/similarity'
import { enumLabel } from '@/i18n/labels'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  projectId: string
  kind: SimilarKind
  title: string
  description: string
  details?: string[]
  /** the item being edited (not listed) */
  excludeId?: string | null
  disabled?: boolean
}>()
const { t } = useI18n()
const hits = ref<SimilarHit[]>([])
const threshold = ref(1)
const engine = ref('tfidf')
const loading = ref(false)
let seq = 0
let last = ''

const body = computed(() => (props.disabled ? null : similarBody(props.kind, { title: props.title, description: props.description, details: props.details ?? [] }, props.excludeId)))

const fetchHits = debounce(async () => {
  const b = body.value
  const key = similarKey(b)
  if (key === last) return
  last = key
  const n = ++seq
  if (!b) {
    hits.value = []
    return
  }
  loading.value = true
  try {
    const res = await api.post<SimilarResponse>(`/projects/${props.projectId}/similar`, b)
    if (n === seq) {
      hits.value = res.items
      threshold.value = res.threshold
      engine.value = res.engine
    }
  } catch {
    if (n === seq) hits.value = [] // a convenience: no error message
  } finally {
    if (n === seq) loading.value = false
  }
}, 700)

watch(body, () => fetchHits(), { immediate: true })
onBeforeUnmount(() => fetchHits.cancel())

const likely = computed(() => hits.value.filter((h) => h.score >= threshold.value).length)
const headingId = `similar-${Math.random().toString(36).slice(2, 8)}`
</script>

<template>
  <section v-if="hits.length" class="similar-panel" :aria-labelledby="headingId" :aria-busy="loading">
    <div class="row" style="gap: 6px; flex-wrap: wrap">
      <i class="pi pi-clone" aria-hidden="true" />
      <h3 :id="headingId" class="plain-heading similar-heading">{{ kind === 'test' ? t('similar.titleTests') : t('similar.title') }}</h3>
      <span class="small muted" aria-live="polite">{{ likely ? t('similar.likely', { n: likely }, likely) : t('similar.none') }}</span>
    </div>
    <ul class="plain-list similar-list">
      <li v-for="h in hits" :key="h.id" class="row similar-hit">
        <Tag :value="t('similar.score', { pct: percent(h.score) })" :severity="scoreSeverity(h.score, threshold)" class="similar-score" />
        <ItemRef :to="itemRoute(h, projectId)" :item-key="h.key" :title="h.title" :hint="h.title" title-class="ellipsis" grow />
        <span class="small muted nowrap">{{ enumLabel('status', h.status) }}</span>
      </li>
    </ul>
    <p class="small muted similar-foot">{{ engine === 'embeddings' ? t('similar.engineEmbeddings') : t('similar.engineLocal') }}</p>
  </section>
</template>

<style scoped>
.similar-panel {
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  padding: 8px 10px;
  display: grid;
  gap: 4px;
}
.similar-heading { font-size: 13px; margin: 0; }
.similar-list { display: grid; gap: 2px; }
.similar-hit { gap: 8px; min-width: 0; }
.similar-score { min-width: 3.6em; justify-content: center; }
.similar-foot { margin: 0; }
</style>
