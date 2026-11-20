<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Message from 'primevue/message'
import StatusTag from './StatusTag.vue'
import DefectLinks from './DefectLinks.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDateTime, formatDuration, formatFullDateTime } from '@/lib/format'
import type { Execution } from '@/lib/types'
import { flipIds } from '@/lib/metrics'
import { usePrefsStore } from '@/stores/prefs'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ projectId: string; testId: string }>()
const emit = defineEmits<{ count: [n: number] }>()
const prefs = usePrefsStore()
const { t } = useI18n()
const items = ref<Execution[]>([])
const loading = ref(false)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    items.value = asItems(await api.get<Execution[]>(`/tests/${props.testId}/executions`))
    emit('count', items.value.length)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(() => props.testId, load, { immediate: true })
const when = (e: Execution) => e.finished_at || e.started_at || e.run_created_at
// results that differ from the test's previous passed / failed result
const flips = computed(() => flipIds(items.value))
</script>

<template>
  <Message v-if="error" severity="error" :closable="false">{{ t('testExecutions.unavailable', { error }) }}</Message>
  <DataTable v-else v-stack :value="items" data-key="id" :loading="loading" :aria-busy="loading" size="small" paginator :rows="prefs.rowsFor(10)" :always-show-paginator="false">
    <Column :header="t('testExecutions.run')" style="min-width: 160px">
      <template #body="{ data }">
        <RouterLink :to="{ name: 'run', params: { projectId, runId: data.run_id } }">{{ data.run_name || t('testExecutions.run') }}</RouterLink>
        <div class="small muted">
          <span v-if="data.run_environment"><i class="pi pi-server" style="font-size: 10px" /> {{ data.run_environment }}</span>
          <span v-if="data.run_build" class="mono"> <i class="pi pi-box" style="font-size: 10px" /> {{ data.run_build }}</span>
        </div>
      </template>
    </Column>
    <Column field="status" :header="t('common.status')" style="width: 150px">
      <template #body="{ data }">
        <span class="row nowrap" style="gap: 4px">
          <StatusTag :value="data.status" kind="result" />
          <span v-if="flips.has(data.id)" class="flip-mark" :title="t('testExecutions.flipHint')"><i class="pi pi-sync" aria-hidden="true" /> {{ t('testExecutions.flip') }}</span>
        </span>
      </template>
    </Column>
    <Column field="executed_by" :header="t('testExecutions.executedBy')" style="width: 140px"><template #body="{ data }"><span class="small">{{ data.executed_by || '—' }}</span></template></Column>
    <Column :header="t('common.date')" style="width: 130px">
      <template #body="{ data }"><span class="small muted" :title="formatFullDateTime(when(data))">{{ formatDateTime(when(data)) }}</span></template>
    </Column>
    <Column :header="t('common.duration')" style="width: 95px"><template #body="{ data }"><span class="small">{{ formatDuration(data.duration_ms) }}</span></template></Column>
    <Column :header="t('testExecutions.defects')"><template #body="{ data }"><DefectLinks :defects="data.defects" /></template></Column>
    <template #empty><div class="muted small">{{ t('testExecutions.none') }}</div></template>
  </DataTable>
</template>
