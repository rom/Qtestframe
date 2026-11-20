<script setup lang="ts">
// The signed-in user's review requests across projects.
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Message from 'primevue/message'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import ReviewDialog from '@/components/ReviewDialog.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { isoDay, notificationTarget, overdue, reviewSeverity } from '@/lib/notifications'
import { formatDate } from '@/lib/format'
import type { ReviewRequest } from '@/lib/types'

const { t } = useI18n()
const status = ref<'pending' | ''>('pending')
const statuses = computed(() => [
  { label: t('reviews.filterPending'), value: 'pending' },
  { label: t('reviews.filterAll'), value: '' },
])
const list = ref<ReviewRequest[]>([])
const loading = ref(false)
const error = ref('')
const current = ref<ReviewRequest | null>(null)
const working = ref(false)
const today = isoDay()

async function load() {
  loading.value = true
  error.value = ''
  try {
    list.value = asItems(await api.get<{ items: ReviewRequest[] }>('/reviews', { reviewer: 'me', status: status.value }))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)
watch(status, load)

function work(r: ReviewRequest) {
  current.value = r
  working.value = true
}
function itemRoute(r: ReviewRequest) {
  return notificationTarget({ project_id: r.project_id, entity_type: r.entity_type, entity_id: r.entity_id })!
}
</script>

<template>
  <section>
    <div class="page-header">
      <div>
        <h1>{{ t('titles.reviews') }}</h1>
        <div class="sub">{{ t('reviews.intro') }}</div>
      </div>
      <div class="actions">
        <SelectButton v-model="status" :options="statuses" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('reviews.filter')" />
      </div>
    </div>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div class="card">
      <DataTable :value="list" :loading="loading" data-key="id" size="small" striped-rows>
        <Column :header="t('reviews.item')">
          <template #body="{ data }">
            <RouterLink :to="itemRoute(data)" class="key">{{ data.entity_key }}</RouterLink>
            <span class="title">{{ data.entity_title }}</span>
            <span v-if="data.outdated" class="small muted"> · {{ t('reviews.outdatedShort') }}</span>
          </template>
        </Column>
        <Column field="requested_by" :header="t('reviews.requestedByCol')"><template #body="{ data }"><span class="small">{{ data.requested_by }}</span></template></Column>
        <Column field="due_date" :header="t('reviews.dueDate')" sortable style="width: 130px">
          <template #body="{ data }">
            <span class="small" :class="{ late: overdue(data, today) }">{{ data.due_date ? formatDate(data.due_date) : '' }}</span>
            <span v-if="overdue(data, today)" class="sr-only">{{ t('reviews.overdue') }}</span>
          </template>
        </Column>
        <Column field="status" :header="t('common.status')" style="width: 160px">
          <template #body="{ data }"><Tag :value="t(`reviews.status.${data.status}`)" :severity="reviewSeverity(data.status)" /></template>
        </Column>
        <Column style="width: 120px">
          <template #header><span class="sr-only">{{ t('common.actions') }}</span></template>
          <template #body="{ data }">
            <Button :label="data.status === 'pending' ? t('reviews.start') : t('reviews.show')" size="small" :outlined="data.status !== 'pending'" @click="work(data)" />
          </template>
        </Column>
        <template #empty><div class="empty">{{ status === 'pending' ? t('reviews.nonePending') : t('reviews.noneAtAll') }}</div></template>
      </DataTable>
    </div>
    <ReviewDialog v-model:visible="working" :review="current" @saved="load" />
  </section>
</template>

<style scoped>
.title {
  margin-left: 6px;
  overflow-wrap: anywhere;
}
.late {
  color: var(--p-red-600, #c62828);
  font-weight: 600;
}
</style>
