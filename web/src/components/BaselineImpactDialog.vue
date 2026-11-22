<script setup lang="ts">
// The project-wide impact of the changes since a baseline
// (GET /baselines/{id}/impact): per changed item what its change makes
// suspect, out of date, outdated or stale, and the totals.
import ImpactList from './ImpactList.vue'
import { ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Tag from 'primevue/tag'
import { useI18n } from 'vue-i18n'
import { api, errorMessage } from '@/lib/api'
import { IMPACT_SECTIONS, type BaselineImpact } from '@/lib/correlations'

const props = defineProps<{ baselineId: string | null; baselineName: string; projectId: string }>()
const visible = defineModel<boolean>('visible', { default: false })
const { t } = useI18n()
const data = ref<BaselineImpact | null>(null)
const loading = ref(false)
const error = ref('')

async function load() {
  if (!props.baselineId) return
  loading.value = true
  error.value = ''
  try {
    data.value = await api.get<BaselineImpact>(`/baselines/${props.baselineId}/impact`)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(visible, (v) => v && load())
</script>

<template>
  <Dialog v-model:visible="visible" :header="t('correlations.baselineTitle', { name: baselineName })" modal style="width: min(760px, 96vw)" class="baseline-impact-dialog">
    <div class="grid" style="gap: 12px" :aria-busy="loading">
      <p class="small muted" style="margin: 0">{{ t('correlations.baselineIntro') }}</p>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <div v-else-if="loading && !data" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
      <template v-else-if="data">
        <div class="row" role="status">
          <Tag v-for="s in IMPACT_SECTIONS.filter((x) => data!.totals[x])" :key="s" :value="`${t(`correlations.section.${s}`)}: ${data.totals[s] ?? 0}`" severity="secondary" />
        </div>
        <RouterLink :to="{ name: 'impact', params: { projectId }, query: { baseline: baselineId ?? undefined } }" class="small">
          <i class="pi pi-bullseye" aria-hidden="true" /> {{ t('correlations.openAnalysis') }}
        </RouterLink>
        <p v-if="!data.changes.length" class="muted small">{{ t('correlations.baselineNone') }}</p>
        <details v-for="c in data.changes" :key="c.entity_type + c.entity_id" class="baseline-impact-item">
          <summary>
            <span class="key">{{ c.key }}</span> {{ c.title }}
            <span class="muted small">— {{ t('correlations.affects', { n: c.impact.total }, c.impact.total) }}</span>
          </summary>
          <div style="padding: 8px 0 8px 16px">
            <ImpactList :impact="c.impact" :project-id="projectId" :heading-level="3" />
          </div>
        </details>
      </template>
    </div>
  </Dialog>
</template>

<style>
.baseline-impact-item {
  border-bottom: 1px solid var(--qtf-border);
  padding: 6px 0;
}
.baseline-impact-item summary {
  cursor: pointer;
}
</style>
