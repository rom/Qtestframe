<script setup lang="ts">
// The Impact tab of a requirement or test: what a change of its substance
// would make suspect, out of date, outdated or stale; for a requirement its
// suspect relations, the open defects that block it and the staleness of
// its results; for a test its regression history.
import ImpactList from './ImpactList.vue'
import ItemRef from './ItemRef.vue'
import DefectTag from './DefectTag.vue'
import StaleBadge from './StaleBadge.vue'
import RegressionHistory from './RegressionHistory.vue'
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import { useI18n } from 'vue-i18n'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import { blockCounts, type ChangeImpact, type DefectBlock, type StaleResult } from '@/lib/correlations'
import { relationText } from '@/i18n/labels'
import type { Relation } from '@/lib/types'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ kind: 'requirement' | 'test'; itemId: string; projectId: string; canWrite: boolean }>()
/** changed: relations were confirmed (the counts on the item changed) */
const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()
const notify = useNotify()

const impact = ref<ChangeImpact | null>(null)
const relations = ref<Relation[]>([])
const blocks = ref<DefectBlock[]>([])
const stale = ref<StaleResult[]>([])
const loading = ref(false)
const error = ref('')
const base = computed(() => `/${props.kind === 'test' ? 'tests' : 'requirements'}/${props.itemId}`)

async function load() {
  loading.value = true
  error.value = ''
  try {
    impact.value = await api.get<ChangeImpact>(`${base.value}/impact`)
    if (props.kind === 'requirement') {
      const [rels, bd, sr] = await Promise.all([
        api.get<Relation[] | { items: Relation[] }>(`${base.value}/relations`),
        api.get<{ items: DefectBlock[] }>(`${base.value}/blocking-defects`),
        api.get<{ items: StaleResult[] }>(`${base.value}/stale-results`),
      ])
      relations.value = asItems(rels).filter((r) => r.suspect_since)
      blocks.value = asItems(bd)
      stale.value = asItems(sr).filter((x) => x.stale)
    }
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(() => props.itemId, load, { immediate: true })
defineExpose({ reload: load })

const counts = computed(() => blockCounts(blocks.value))
/** the requirement at the other end of a relation */
function other(r: Relation) {
  return r.source_id === props.itemId ? { id: r.target_id, key: r.target_key, title: r.target_title } : { id: r.source_id, key: r.source_key, title: r.source_title }
}
async function confirm(r?: Relation) {
  try {
    const res = await api.post<{ confirmed: number }>(r ? `/relations/${r.id}/confirm` : `${base.value}/relations/confirm`)
    notify.ok(t('correlations.relationsConfirmed', { n: res?.confirmed ?? 0 }, res?.confirmed ?? 0))
    await load()
    emit('changed')
  } catch (e) {
    notify.fail(e)
  }
}
</script>

<template>
  <div class="grid impact-panel" style="gap: 16px" :aria-busy="loading">
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>

    <section v-if="kind === 'requirement'" :aria-label="t('correlations.blockingTitle')">
      <h3 class="impact-heading">
        {{ t('correlations.blockingTitle') }}
        <span v-if="blocks.length" class="count">{{ blocks.length }}</span>
      </h3>
      <p v-if="!blocks.length" class="muted small">{{ t('correlations.notBlocked') }}</p>
      <template v-else>
        <p v-if="counts.blocking" class="blocked-badge" style="margin: 0 0 6px"><i class="pi pi-ban" aria-hidden="true" />{{ t('correlations.blockedBy', { n: counts.blocking }, counts.blocking) }}</p>
        <ul class="plain-list">
          <li v-for="b in blocks" :key="b.defect_id" class="impact-row">
            <ItemRef :to="{ name: 'defects', params: { projectId }, query: { item: b.defect_id } }" :item-key="b.key" :title="b.title" title-class="ellipsis" grow />
            <span class="row small nowrap" style="gap: 6px">
              <DefectTag :value="b.severity" kind="severity" />
              <span :class="b.level === 'block' ? 'blocked-badge' : 'warned-badge'">
                <i :class="b.level === 'block' ? 'pi pi-ban' : 'pi pi-exclamation-triangle'" aria-hidden="true" />
                {{ b.level === 'block' ? t('correlations.blocks') : t('correlations.warns') }}
              </span>
              <span class="muted">{{ b.via === 'requirement' ? t('defectsView.viaDirect') : t('correlations.viaTest', { key: b.via }) }}</span>
            </span>
          </li>
        </ul>
      </template>
    </section>

    <section v-if="kind === 'requirement' && (relations.length || stale.length)">
      <h3 class="impact-heading">{{ t('correlations.currentTitle') }}</h3>
      <div v-if="relations.length" class="grid" style="gap: 6px">
        <div class="row" style="justify-content: space-between">
          <span class="small">{{ t('correlations.suspectRelations', { n: relations.length }, relations.length) }}</span>
          <Button v-if="canWrite && relations.length > 1" :label="t('suspect.confirmAll', { n: relations.length })" size="small" text @click="confirm()" />
        </div>
        <ul class="plain-list">
          <li v-for="r in relations" :key="r.id" class="impact-row">
            <span class="row nowrap" style="gap: 6px; min-width: 0">
              <span class="muted small">{{ relationText(r.type, r.source_id === itemId ? 'outgoing' : 'incoming') }}</span>
              <ItemRef :to="{ name: 'requirements', params: { projectId }, query: { item: other(r).id } }" :item-key="other(r).key" :title="other(r).title" title-class="ellipsis" grow />
            </span>
            <span class="suspect row nowrap" :title="`${r.suspect_reason ?? ''} (${formatDateTime(r.suspect_since ?? '')})`">
              <i class="pi pi-exclamation-triangle" aria-hidden="true" />
              <span>{{ r.suspect_end === (r.source_id === itemId ? 'source' : 'target') ? t('correlations.thisSuspect') : t('correlations.otherSuspect') }}</span>
              <span class="sr-only">: {{ r.suspect_reason }}, {{ formatDateTime(r.suspect_since ?? '') }}</span>
              <Button v-if="canWrite" :label="t('suspect.confirm')" size="small" text :aria-label="t('correlations.confirmRelation', { key: other(r).key })" @click="confirm(r)" />
            </span>
          </li>
        </ul>
      </div>
      <div v-if="stale.length" class="grid" style="gap: 6px">
        <span class="small">{{ t('correlations.staleResults', { n: stale.length }, stale.length) }}</span>
        <ul class="plain-list">
          <li v-for="s in stale" :key="s.test_id" class="impact-row">
            <ItemRef :to="{ name: 'tests', params: { projectId }, query: { item: s.test_id } }" :item-key="s.key" :title="s.title" title-class="ellipsis" grow />
            <StaleBadge :stale="true" :reason="s.reason" :recorded-at="s.recorded_at" :result="s.result" />
          </li>
        </ul>
      </div>
    </section>

    <section>
      <h3 class="impact-heading">{{ kind === 'test' ? t('correlations.ifTestChanges') : t('correlations.ifRequirementChanges') }}</h3>
      <p class="muted small">{{ kind === 'test' ? t('correlations.ifTestChangesHint') : t('correlations.ifRequirementChangesHint') }}</p>
      <ImpactList :impact="impact" :project-id="projectId" :heading-level="4" />
      <RouterLink :to="{ name: 'impact', params: { projectId }, query: { items: `${kind}:${itemId}` } }" class="small">
        <i class="pi pi-bullseye" aria-hidden="true" /> {{ t('correlations.openAnalysis') }}
      </RouterLink>
    </section>

    <section v-if="kind === 'test'">
      <h3 class="impact-heading">{{ t('correlations.regressionsTitle') }}</h3>
      <RegressionHistory entity-type="test" :entity-id="itemId" :project-id="projectId" />
    </section>
  </div>
</template>
