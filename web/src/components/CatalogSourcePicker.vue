<script setup lang="ts">
// Choose the laws, regulations, standards and best practices to comply
// with (GET /catalog/sources): grouped by kind, searchable, with the number
// of catalogue requirements citing each and the tests they bring.
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Dialog from 'primevue/dialog'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import InputText from 'primevue/inputtext'
import Tag from 'primevue/tag'
import { groupSources, toggleIds, type CatalogSource, type CatalogSourceKind } from '@/lib/catalog'
import { useI18n } from 'vue-i18n'

const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ sources: CatalogSource[]; kinds: CatalogSourceKind[]; selected: string[] }>()
const emit = defineEmits<{ apply: [ids: string[]] }>()
const { t, te } = useI18n()

const q = ref('')
const chosen = ref(new Set<string>())
watch(visible, (v) => {
  if (v) {
    chosen.value = new Set(props.selected)
    q.value = ''
  }
})
const groups = computed(() => groupSources(props.sources, props.kinds, q.value))
const shownCount = computed(() => groups.value.reduce((n, g) => n + g.sources.length, 0))
const totals = computed(() => {
  let reqs = 0
  let tests = 0
  for (const s of props.sources) {
    if (!chosen.value.has(s.id)) continue
    reqs += s.requirements
    tests += s.tests
  }
  return { reqs, tests }
})
const jurisdiction = (j?: string) => (!j ? '' : te(`catalog.sources.jurisdiction.${j}`) ? t(`catalog.sources.jurisdiction.${j}`) : j)
const toggle = (id: string, on: boolean) => (chosen.value = toggleIds(chosen.value, [id], on))
const toggleGroup = (ids: string[], on: boolean) => (chosen.value = toggleIds(chosen.value, ids, on))
const groupState = (ids: string[]) => {
  const n = ids.filter((id) => chosen.value.has(id)).length
  return n === 0 ? 'none' : n === ids.length ? 'all' : 'some'
}
function apply() {
  // keep the registry order
  emit(
    'apply',
    props.sources.filter((s) => chosen.value.has(s.id)).map((s) => s.id),
  )
  visible.value = false
}
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="t('catalog.sources.pickTitle')" :style="{ width: 'min(760px, 96vw)' }" :breakpoints="{ '640px': '100vw' }">
    <p class="small muted" style="margin-top: 0">{{ t('catalog.sources.pickIntro') }}</p>
    <IconField class="src-search">
      <InputIcon class="pi pi-search" />
      <InputText v-model="q" type="search" :placeholder="t('catalog.sources.search')" :aria-label="t('catalog.sources.search')" autofocus />
    </IconField>
    <div class="small muted" role="status" aria-live="polite">{{ t('catalog.sources.shown', { n: shownCount }, shownCount) }}</div>
    <div class="src-groups">
      <fieldset v-for="g in groups" :key="g.kind" class="src-group">
        <legend class="row nowrap" style="gap: 8px">
          <Checkbox
            :model-value="groupState(g.sources.map((s) => s.id)) === 'all'"
            :indeterminate="groupState(g.sources.map((s) => s.id)) === 'some'"
            binary
            :input-id="`src-kind-${g.kind}`"
            @update:model-value="(v: boolean) => toggleGroup(g.sources.map((s) => s.id), v)"
          />
          <label :for="`src-kind-${g.kind}`"><b>{{ g.label }}</b> <span class="muted small">({{ g.sources.length }})</span></label>
        </legend>
        <ul class="plain-list">
          <li v-for="s in g.sources" :key="s.id" class="src-item">
            <Checkbox :model-value="chosen.has(s.id)" binary :input-id="`src-${s.id}`" @update:model-value="(v: boolean) => toggle(s.id, v)" />
            <label :for="`src-${s.id}`" class="grow src-label">
              <span class="src-name">{{ s.name }}</span>
              <span class="row small muted" style="gap: 6px; flex-wrap: wrap">
                <Tag v-if="s.short && s.short !== s.name" :value="s.short" severity="secondary" />
                <span v-if="s.jurisdiction">{{ jurisdiction(s.jurisdiction) }}</span>
                <span>{{ t('catalog.sources.counts', { requirements: t('catalog.nRequirements', { n: s.requirements }, s.requirements), tests: t('catalog.nTests', { n: s.tests }, s.tests) }) }}</span>
              </span>
            </label>
            <a v-if="s.url" :href="s.url" target="_blank" rel="noopener noreferrer" class="small" :aria-label="t('catalog.sources.openSource', { name: s.name })"><i class="pi pi-external-link" aria-hidden="true" /></a>
          </li>
        </ul>
      </fieldset>
      <p v-if="!groups.length" class="muted">{{ t('catalog.sources.noMatch') }}</p>
    </div>
    <template #footer>
      <span class="small grow" role="status" aria-live="polite">
        {{ chosen.size ? t('catalog.sources.chosenSummary', { n: chosen.size, requirements: t('catalog.nRequirements', { n: totals.reqs }, totals.reqs), tests: t('catalog.nTests', { n: totals.tests }, totals.tests) }, chosen.size) : t('catalog.sources.noneChosen') }}
      </span>
      <Button :label="t('catalog.sources.clear')" text severity="secondary" :disabled="!chosen.size" @click="chosen = new Set()" />
      <Button :label="t('common.cancel')" severity="secondary" outlined @click="visible = false" />
      <Button :label="t('catalog.sources.apply')" icon="pi pi-check" @click="apply" />
    </template>
  </Dialog>
</template>

<style scoped>
.src-search { width: 100%; margin-bottom: 6px; }
.src-search :deep(input) { width: 100%; }
.src-groups { display: grid; gap: 10px; margin-top: 8px; max-height: 60vh; overflow: auto; }
.src-group { border: 1px solid var(--qtf-border); border-radius: 6px; padding: 6px 10px 8px; margin: 0; }
.src-group legend { padding: 0 4px; }
.src-item { display: flex; gap: 8px; align-items: flex-start; padding: 5px 0; border-top: 1px solid var(--qtf-border); }
.src-item:first-child { border-top: 0; }
.src-item > .p-checkbox { margin-top: 2px; flex: none; }
.src-label { display: grid; gap: 2px; cursor: pointer; min-width: 0; }
.src-name { overflow-wrap: anywhere; }
</style>
