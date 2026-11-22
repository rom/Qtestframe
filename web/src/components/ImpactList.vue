<script setup lang="ts">
// The items a change of a requirement or test makes suspect, out of date,
// outdated or stale, grouped by section (used by the Impact tab, the
// confirmation before saving and the impact of a baseline diff).
import ItemRef from './ItemRef.vue'
import StatusTag from './StatusTag.vue'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { impactSections, type ChangeImpact, type ImpactItem } from '@/lib/correlations'
import { relationText } from '@/i18n/labels'
import { useImpactLabels } from '@/composables/impact'

const props = defineProps<{ impact: ChangeImpact | null; projectId: string; headingLevel?: 3 | 4 }>()
const { t } = useI18n()
const { pathOf } = useImpactLabels()
const sections = computed(() => impactSections(props.impact))
const tag = computed(() => `h${props.headingLevel ?? 3}`)

function route(it: ImpactItem) {
  switch (it.kind) {
    case 'requirement':
      return { name: 'requirements', params: { projectId: props.projectId }, query: { item: it.id } }
    case 'test':
    case 'result':
      return { name: 'tests', params: { projectId: props.projectId }, query: { item: it.test_id || it.id } }
    case 'document':
      return { name: 'documents', params: { projectId: props.projectId } }
    case 'risk':
      return { name: 'risks', params: { projectId: props.projectId } }
    case 'defect':
      return { name: 'defects', params: { projectId: props.projectId }, query: { item: it.id } }
  }
  return undefined
}
function note(it: ImpactItem): string {
  // reached in more than one step: the path
  if ((it.path?.length ?? 0) > 2) return t('correlations.reachedVia', { path: pathOf(it.path) })
  if (it.kind === 'requirement' && it.relation) {
    return it.relation === 'verifies' ? t('correlations.viaVerifies') : t('correlations.viaRelation', { relation: relationText(it.relation, it.direction) })
  }
  if (it.kind === 'test' && it.via) return t('correlations.viaRequirement', { key: it.via })
  return ''
}
</script>

<template>
  <div class="impact-list grid" style="gap: 12px">
    <p v-if="!sections.length" class="muted small">{{ t('correlations.nothing') }}</p>
    <section v-for="s in sections" :key="s.section" class="impact-section">
      <component :is="tag" class="impact-heading">
        {{ t(`correlations.section.${s.section}`) }}
        <span class="count">{{ s.items.length }}</span>
      </component>
      <ul class="plain-list">
        <li v-for="it in s.items" :key="`${it.id}|${it.requirement_id ?? ''}`" class="impact-row">
          <a v-if="it.kind === 'issue' && it.url" :href="it.url" target="_blank" rel="noopener noreferrer" class="mono"
            >{{ it.key }}<span class="sr-only"> ({{ t('links.newTab') }})</span></a
          >
          <ItemRef v-else-if="route(it)" :to="route(it)" :item-key="it.key" :title="it.title" title-class="ellipsis" grow />
          <span v-else class="mono">{{ it.key }}</span>
          <span class="row small nowrap" style="gap: 6px">
            <span v-if="note(it)" class="muted">{{ note(it) }}</span>
            <StatusTag v-if="it.kind === 'result' && it.result" :value="it.result" kind="result" />
            <span v-if="it.mode === 'review' && !it.already" class="impact-mode impact-mode-review">{{ t('correlations.mode.review') }}</span>
            <span v-if="it.already" class="impact-already">
              <i class="pi pi-check-circle" aria-hidden="true" /> {{ t(`correlations.already.${s.section}`) }}
            </span>
          </span>
        </li>
      </ul>
    </section>
  </div>
</template>

<style>
.impact-heading {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0 0 4px;
  font-size: 14px;
}
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
.impact-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 4px 8px;
  padding: 4px 0;
  border-bottom: 1px solid var(--qtf-border);
  min-width: 0;
}
.impact-row:last-child {
  border-bottom: 0;
}
.impact-already {
  color: var(--qtf-muted, var(--p-text-muted-color));
}
</style>
