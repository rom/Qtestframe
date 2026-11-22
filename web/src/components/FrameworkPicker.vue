<script setup lang="ts">
// Chooses MITRE D3FEND techniques or MITRE EMB3D threats and mitigations
// from the catalogue's lists (GET /catalog/d3fend, /catalog/emb3d). Ids
// the list no longer has stay visible, flagged, so that they can be
// removed; the server rejects unknown ids.
import { computed, onMounted, ref } from 'vue'
import MultiSelect from 'primevue/multiselect'
import Message from 'primevue/message'
import { loadD3fend, loadEmb3d, pickerOptions, unknownIds, type D3fendTechnique, type Emb3dItem } from '@/lib/frameworks'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  /** d3fend: defensive techniques; emb3d: threats and mitigations; threats: EMB3D threats only (risks) */
  kind: 'd3fend' | 'emb3d' | 'threats'
  inputId: string
  disabled?: boolean
}>()
const model = defineModel<string[]>({ required: true })
const { t } = useI18n()

const list = ref<(D3fendTechnique | Emb3dItem)[] | null>(null)
const failed = ref(false)
onMounted(async () => {
  try {
    if (props.kind === 'd3fend') list.value = (await loadD3fend()).items
    else {
      const items = (await loadEmb3d()).items
      list.value = props.kind === 'threats' ? items.filter((x) => x.kind === 'threat') : items
    }
  } catch {
    failed.value = true
  }
})
const byId = computed(() => (list.value ? new Map(list.value.map((x) => [x.id, x])) : null))
const options = computed(() => pickerOptions(list.value ?? [], model.value, !!list.value))
const unknown = computed(() => unknownIds(model.value, byId.value))
const chosen = computed(() => model.value.map((id) => byId.value?.get(id)).filter((x): x is D3fendTechnique | Emb3dItem => !!x))
const hintId = computed(() => `${props.inputId}-hint`)
const detail = (x: D3fendTechnique | Emb3dItem) => ('tactic' in x ? x.tactic : x.kind === 'threat' ? x.category : t('frameworks.emb3d.mitigation'))
</script>

<template>
  <div class="fw-picker">
    <MultiSelect
      v-model="model"
      :input-id="inputId"
      :options="options"
      option-label="label"
      option-value="value"
      filter
      display="chip"
      :virtual-scroller-options="{ itemSize: 36 }"
      :loading="!list && !failed"
      :placeholder="t(`frameworks.${kind}.choose`)"
      :disabled="disabled"
      :aria-describedby="hintId"
      fluid
    />
    <small :id="hintId" class="muted">
      {{ t(`frameworks.${kind}.hint`) }}
      <template v-for="x in chosen" :key="x.id">
        · <a :href="x.url" target="_blank" rel="noopener noreferrer">{{ x.id }}<span class="sr-only"> {{ x.name }}, {{ detail(x) }} ({{ t('links.newTab') }})</span></a>
      </template>
    </small>
    <Message v-if="unknown.length" severity="warn" :closable="false" class="small">{{ t('frameworks.unknown', { ids: unknown.join(', ') }) }}</Message>
    <Message v-if="failed" severity="warn" :closable="false" class="small">{{ t('frameworks.loadFailed') }}</Message>
  </div>
</template>

<style scoped>
.fw-picker { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
</style>
