<script setup lang="ts">
// The regression history of a defect or test: each time a test that had
// verified the fix of a defect failed again, and what happened (the defect
// was reopened, or a regression defect was created).
import ItemRef from './ItemRef.vue'
import { ref, watch } from 'vue'
import Message from 'primevue/message'
import { useI18n } from 'vue-i18n'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import type { DefectRegression } from '@/lib/correlations'

const props = defineProps<{ entityType: 'defect' | 'test'; entityId: string; projectId: string }>()
const emit = defineEmits<{ count: [n: number] }>()
const { t } = useI18n()
const items = ref<DefectRegression[]>([])
const error = ref('')
const loading = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    items.value = asItems(await api.get<{ items: DefectRegression[] }>(`/${props.entityType === 'defect' ? 'defects' : 'tests'}/${props.entityId}/regressions`))
    emit('count', items.value.length)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(() => props.entityId, load, { immediate: true })
</script>

<template>
  <div :aria-busy="loading">
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <p v-else-if="!items.length && !loading" class="muted small">{{ t('correlations.noRegressions') }}</p>
    <ul v-else class="plain-list">
      <li v-for="g in items" :key="g.id" class="impact-row regression-row">
        <span class="row small" style="gap: 6px; min-width: 0">
          <i class="pi pi-replay" aria-hidden="true" />
          <span>{{ formatDateTime(g.created_at) }}</span>
          <ItemRef v-if="entityType === 'defect'" :to="{ name: 'tests', params: { projectId }, query: { item: g.test_case_id } }" :item-key="g.test_key" />
          <ItemRef v-else :to="{ name: 'defects', params: { projectId }, query: { item: g.defect_id } }" :item-key="g.defect_key" />
          <span>{{ t('correlations.failedAgain') }}</span>
          <RouterLink v-if="g.run_id" :to="{ name: 'run', params: { projectId, runId: g.run_id } }">{{ g.run_name }}</RouterLink>
        </span>
        <span class="small">
          <template v-if="g.action === 'created' && g.regression_defect_id">
            {{ t('correlations.regressionCreated') }}
            <ItemRef :to="{ name: 'defects', params: { projectId }, query: { item: g.regression_defect_id } }" :item-key="g.regression_defect_key" />
          </template>
          <template v-else>{{ t('correlations.regressionReopened', { status: t(`defectsView.status.${g.previous_status}`) }) }}</template>
        </span>
      </li>
    </ul>
  </div>
</template>
