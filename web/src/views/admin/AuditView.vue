<script setup lang="ts">
import { onMounted, ref } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Tag from 'primevue/tag'
import { api, asItems } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import type { AuditEntry } from '@/lib/types'
import { useNotify } from '@/composables/notify'
import { usePrefsStore } from '@/stores/prefs'
import { useI18n } from 'vue-i18n'
const prefs = usePrefsStore()

const notify = useNotify()
const { t } = useI18n()
const entries = ref<AuditEntry[]>([])
const loading = ref(false)
const q = ref('')
const limit = ref(200)

async function load() {
  loading.value = true
  try {
    entries.value = asItems(await api.get<{ items: AuditEntry[] }>('/audit', { limit: limit.value, q: q.value.trim() }))
  } catch (e) {
    notify.fail(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)
function severity(action: string) {
  if (/delete|remove|revoke/i.test(action)) return 'danger'
  if (/create|add/i.test(action)) return 'success'
  if (/login|logout|auth/i.test(action)) return 'info'
  return 'secondary'
}
</script>

<template>
  <div class="card">
    <div class="filterbar">
      <IconField class="grow">
        <InputIcon class="pi pi-filter" />
        <InputText v-model="q" class="w-full" :placeholder="t('admin.auditFilter')" :aria-label="t('admin.auditFilter')" @keydown.enter="load" />
      </IconField>
      <Select v-model="limit" :options="[100, 200, 500, 1000]" style="width: 110px" :aria-label="t('admin.auditLimit')" @change="load" />
      <Button :label="t('common.apply')" icon="pi pi-search" severity="secondary" @click="load" />
    </div>
    <DataTable v-stack :value="entries" data-key="id" :loading="loading" :aria-busy="loading" paginator :rows="prefs.rowsFor(50)" size="small" sort-field="created_at" :sort-order="-1">
      <Column field="created_at" :header="t('common.time')" sortable style="width: 160px"><template #body="{ data }"><span class="small">{{ formatDateTime(data.created_at) }}</span></template></Column>
      <Column field="user_email" :header="t('common.user')" sortable style="width: 220px" />
      <Column field="action" :header="t('admin.action')" sortable style="width: 170px"><template #body="{ data }"><Tag :value="data.action" :severity="severity(data.action)" /></template></Column>
      <Column field="entity_type" :header="t('admin.entity')" sortable style="width: 130px" />
      <Column field="entity_id" :header="t('admin.entityId')" style="width: 120px"><template #body="{ data }"><span class="mono small" :title="data.entity_id">{{ data.entity_id?.slice(0, 8) }}</span></template></Column>
      <Column field="details" :header="t('admin.detailsCol')"><template #body="{ data }"><span class="small mono" style="word-break: break-word">{{ data.details }}</span></template></Column>
      <template #empty><div class="empty"><i class="pi pi-history" />{{ t('admin.noAudit') }}</div></template>
    </DataTable>
  </div>
</template>
