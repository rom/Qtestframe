<script setup lang="ts">
// Create (or update) the tracker issues of the selected requirements and
// refresh their states: POST /projects/{pid}/requirements/issues and
// …/issues/sync. Requirements that have an issue are skipped unless
// "update" is chosen, so no requirement gets a second issue.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import { api, errorMessage } from '@/lib/api'
import { trackerName } from '@/lib/defectTracker'
import { bulkIssueResult, issueSyncResult, type BulkIssueResult, type IssueSyncResult } from '@/lib/requirementIssues'
import type { DefectTracker } from '@/lib/types'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ projectId: string; selected: string[] }>()
const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ done: [] }>()
const { t } = useI18n()
const notify = useNotify()

const trackerType = ref<string | null>(null)
const loadError = ref('')
const update = ref(false)
const busy = ref<'' | 'create' | 'sync'>('')
const result = ref<BulkIssueResult | null>(null)
const syncRes = ref<IssueSyncResult | null>(null)
const tracker = computed(() => trackerName(trackerType.value))

watch(visible, async (v) => {
  if (!v) return
  update.value = false
  result.value = null
  syncRes.value = null
  loadError.value = ''
  trackerType.value = null
  try {
    trackerType.value = (await api.get<DefectTracker>(`/projects/${props.projectId}/defect-tracker`)).type
  } catch (e) {
    loadError.value = errorMessage(e)
  }
})

async function create() {
  busy.value = 'create'
  syncRes.value = null
  try {
    result.value = bulkIssueResult(await api.post<BulkIssueResult>(`/projects/${props.projectId}/requirements/issues`, { ids: props.selected, update: update.value }))
    const r = result.value
    notify.ok(t('reqIssue.bulk.result', { created: r.created.length, updated: r.updated.length, skipped: r.skipped.length, failed: r.failed.length }))
    emit('done')
  } catch (e) {
    notify.fail(e, t('reqIssue.failed'))
  } finally {
    busy.value = ''
  }
}
async function sync() {
  busy.value = 'sync'
  result.value = null
  try {
    syncRes.value = issueSyncResult(await api.post<IssueSyncResult>(`/projects/${props.projectId}/requirements/issues/sync`, { ids: props.selected }))
    const r = syncRes.value
    notify.ok(t('reqIssue.bulk.syncResult', { synced: r.synced, changed: r.changed, commented: r.commented, failed: r.failed.length }))
    emit('done')
  } catch (e) {
    notify.fail(e, t('reqIssue.failed'))
  } finally {
    busy.value = ''
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="tracker ? t('reqIssue.bulk.title', { tracker }) : t('reqIssue.bulk.button')" style="width: min(620px, 95vw)">
    <div class="grid" style="gap: 12px">
      <Message v-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>
      <Message v-else-if="trackerType === ''" severity="info" :closable="false">{{ t('reqIssue.bulk.noTracker') }}</Message>
      <template v-else-if="trackerType">
        <p class="small" style="margin: 0">{{ t('reqIssue.bulk.selected', { n: selected.length }, selected.length) }}</p>
        <p class="small muted" style="margin: 0">{{ t('reqIssue.bulk.intro', { tracker }) }}</p>
        <span class="row nowrap"
          ><Checkbox v-model="update" binary input-id="bulk-issue-update" /><label for="bulk-issue-update">{{ t('reqIssue.bulk.updateExisting') }}</label></span
        >
        <div v-if="result" role="status" class="small">
          <p style="margin: 0 0 4px">{{ t('reqIssue.bulk.result', { created: result.created.length, updated: result.updated.length, skipped: result.skipped.length, failed: result.failed.length }) }}</p>
          <ul class="plain-list">
            <li v-for="r in [...result.created, ...result.updated, ...result.skipped]" :key="r.key">
              <span class="key">{{ r.key }}</span> →
              <a :href="r.url" target="_blank" rel="noopener noreferrer">{{ r.issue_key }}<span class="sr-only"> ({{ t('links.newTab') }})</span></a>
            </li>
          </ul>
        </div>
        <div v-if="syncRes" role="status" class="small">
          {{ t('reqIssue.bulk.syncResult', { synced: syncRes.synced, changed: syncRes.changed, commented: syncRes.commented, failed: syncRes.failed.length }) }}
        </div>
        <Message v-for="f in result?.failed ?? syncRes?.failed ?? []" :key="f.key" severity="error" :closable="false">{{ t('reqIssue.bulk.failedItem', { key: f.key, error: f.error }) }}</Message>
      </template>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.close')" severity="secondary" text @click="visible = false" />
        <template v-if="trackerType">
          <Button :label="t('reqIssue.bulk.sync')" icon="pi pi-refresh" severity="secondary" outlined :loading="busy === 'sync'" :disabled="!!busy" @click="sync" />
          <Button :label="update ? t('reqIssue.bulk.createUpdate') : t('reqIssue.bulk.create')" icon="pi pi-ticket" :loading="busy === 'create'" :disabled="!!busy" @click="create" />
        </template>
      </div>
    </div>
  </Dialog>
</template>
