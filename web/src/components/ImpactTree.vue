<script setup lang="ts">
// The items of a change impact as a tree: per section (requirements, tests,
// risks, …) with its count, per depth with its count, each item with its
// effect (suspect or review) and the path along which the change reaches it
// (the confirmation dialog of every change).
import ItemRef from './ItemRef.vue'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { impactTree, type ChangeImpact, type ImpactItem } from '@/lib/correlations'
import { useImpactLabels } from '@/composables/impact'

const props = defineProps<{ impact: ChangeImpact | null; projectId: string; headingLevel?: 3 | 4 }>()
const { t } = useI18n()
const { pathOf } = useImpactLabels()
const tree = computed(() => impactTree(props.impact))
const tag = computed(() => `h${props.headingLevel ?? 3}`)

function route(it: ImpactItem) {
  const pid = props.projectId
  switch (it.kind) {
    case 'requirement':
      return { name: 'requirements', params: { projectId: pid }, query: { item: it.id } }
    case 'test':
    case 'result':
      return { name: 'tests', params: { projectId: pid }, query: { item: it.test_id || it.id } }
    case 'risk':
      return { name: 'risks', params: { projectId: pid }, query: { item: it.id } }
    case 'defect':
      return { name: 'defects', params: { projectId: pid }, query: { item: it.id } }
    case 'document':
      return { name: 'documents', params: { projectId: pid } }
  }
  return undefined
}
</script>

<template>
  <div class="impact-tree">
    <p v-if="!tree.length" class="muted small">{{ t('correlations.nothing') }}</p>
    <section v-for="s in tree" :key="s.section" class="impact-tree-section">
      <component :is="tag" class="impact-heading">
        {{ t(`correlations.section.${s.section}`) }}
        <span class="count">{{ s.count }}</span>
      </component>
      <details v-for="d in s.depths" :key="d.depth" class="impact-depth" :open="d.depth <= 2">
        <summary>
          {{ t('correlations.depth', { n: d.depth }) }} <span class="count">{{ d.items.length }}</span>
        </summary>
        <ul class="plain-list">
          <li v-for="it in d.items" :key="`${it.kind}:${it.id}`" class="impact-row">
            <span class="impact-row-main">
              <a v-if="it.kind === 'issue' && it.url" :href="it.url" target="_blank" rel="noopener noreferrer" class="mono"
                >{{ it.key }}<span class="sr-only"> ({{ t('links.newTab') }})</span></a
              >
              <ItemRef v-else-if="route(it)" :to="route(it)" :item-key="it.key" :title="it.title" title-class="ellipsis" grow />
              <span v-else class="mono">{{ it.key }} <span v-if="it.title" class="muted">{{ it.title }}</span></span>
              <span v-if="(it.path?.length ?? 0) > 2" class="small muted impact-path">{{ t('correlations.reachedVia', { path: pathOf(it.path) }) }}</span>
            </span>
            <span class="row small nowrap" style="gap: 6px">
              <span v-if="it.already" class="impact-already"> <i class="pi pi-check-circle" aria-hidden="true" /> {{ t(`correlations.already.${s.section}`) }} </span>
              <span v-else class="impact-mode" :class="`impact-mode-${it.mode ?? 'suspect'}`">{{ t(`correlations.mode.${it.mode ?? 'suspect'}`) }}</span>
            </span>
          </li>
        </ul>
      </details>
    </section>
  </div>
</template>

<style>
.impact-tree {
  display: grid;
  gap: 12px;
}
.impact-depth > summary {
  cursor: pointer;
  font-size: 13px;
  padding: 2px 0;
}
.impact-depth > summary .count,
.impact-heading .count {
  display: inline-block;
  min-width: 18px;
  padding: 0 6px;
  border-radius: 99px;
  font-size: 11px;
  font-weight: normal;
  background: var(--qtf-hover);
  border: 1px solid var(--qtf-border);
  text-align: center;
}
.impact-depth ul {
  margin-left: 14px;
}
.impact-row-main {
  display: grid;
  min-width: 0;
  flex: 1 1 260px;
}
.impact-path {
  overflow-wrap: anywhere;
}
.impact-mode {
  padding: 0 6px;
  border-radius: 4px;
  border: 1px solid var(--qtf-border);
}
.impact-mode-review {
  border-style: dashed;
}
</style>
