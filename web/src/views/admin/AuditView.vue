<script setup lang="ts">
// Audit trail: date range, user/action/entity filters, free query, paging,
// CSV and Markdown export and a detail drawer per entry.
import { computed, onMounted, ref } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import DatePicker from 'primevue/datepicker'
import Drawer from 'primevue/drawer'
import Tag from 'primevue/tag'
import Message from 'primevue/message'
import QueryInput from '@/components/QueryInput.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import { api, ApiError, asItems } from '@/lib/api'
import { actionSeverity, auditParams, prettyDetails, rangeInvalid, type AuditFilters } from '@/lib/audit'
import { formatDateTime, formatFullDateTime } from '@/lib/format'
import type { AuditEntry } from '@/lib/types'
import { useNotify } from '@/composables/notify'
import { usePrefsStore } from '@/stores/prefs'
import { useI18n } from 'vue-i18n'

const prefs = usePrefsStore()
const notify = useNotify()
const { t } = useI18n()
const entries = ref<AuditEntry[]>([])
const loading = ref(false)
const exporting = ref(false)
const error = ref<unknown>(null)
const from = ref<Date | null>(null)
const to = ref<Date | null>(null)
const user = ref('')
const action = ref('')
const entity = ref('')
const q = ref('')
const limit = ref(500)
const limits = [100, 500, 1000, 5000]
const ENTITIES = ['requirement', 'test', 'run', 'execution', 'project', 'folder', 'category', 'baseline', 'comment', 'file', 'member', 'user', 'tenant', 'token', 'session', 'report']
const entityOptions = computed(() => {
  const seen = new Set([...ENTITIES, ...entries.value.map((e) => e.entity_type).filter(Boolean)])
  return [{ label: t('common.all'), value: '' }, ...[...seen].sort().map((v) => ({ label: v, value: v }))]
})
const badRange = computed(() => rangeInvalid(from.value, to.value))
const filters = computed<AuditFilters>(() => ({ from: from.value, to: to.value, user: user.value, action: action.value, entity: entity.value, q: q.value, limit: limit.value }))
const queryError = computed(() => (error.value instanceof ApiError && error.value.code === 'invalid_query' ? error.value : null))

async function load() {
  if (badRange.value) return
  loading.value = true
  error.value = null
  try {
    entries.value = asItems(await api.get<{ items: AuditEntry[] }>('/audit', auditParams(filters.value)))
  } catch (e) {
    error.value = e
    if (!(e instanceof ApiError && e.code === 'invalid_query')) notify.fail(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)
function reset() {
  from.value = to.value = null
  user.value = action.value = entity.value = q.value = ''
  load()
}
async function exportAs(format: 'csv' | 'md') {
  exporting.value = true
  try {
    const name = await api.download('/audit', auditParams(filters.value, format), `audit-log.${format}`)
    notify.ok(t('audit.exported'), name)
  } catch (e) {
    notify.fail(e, t('audit.exportFailed'))
  } finally {
    exporting.value = false
  }
}

const detail = ref<AuditEntry | null>(null)
const drawer = ref(false)
function show(e: AuditEntry) {
  detail.value = e
  drawer.value = true
}
const details = computed(() => prettyDetails(detail.value?.details))
</script>

<template>
  <div class="card">
    <form class="audit-filters" @submit.prevent="load">
      <div v-field class="field">
        <label>{{ t('audit.from') }}</label>
        <DatePicker v-model="from" date-format="yy-mm-dd" show-icon show-button-bar :max-date="to ?? undefined" :invalid="badRange" />
      </div>
      <div v-field class="field">
        <label>{{ t('audit.to') }}</label>
        <DatePicker v-model="to" date-format="yy-mm-dd" show-icon show-button-bar :min-date="from ?? undefined" :invalid="badRange" />
      </div>
      <div v-field class="field"><label>{{ t('common.user') }}</label><InputText v-model="user" :placeholder="t('audit.userPlaceholder')" /></div>
      <div v-field class="field"><label>{{ t('admin.action') }}</label><InputText v-model="action" :placeholder="t('audit.actionPlaceholder')" /></div>
      <div v-field class="field"><label>{{ t('admin.entity') }}</label><Select v-model="entity" :options="entityOptions" option-label="label" option-value="value" filter :placeholder="t('common.all')" /></div>
      <div v-field class="field"><label>{{ t('admin.auditLimit') }}</label><Select v-model="limit" :options="limits" /></div>
      <div class="field query-field">
        <span class="field-label" aria-hidden="true">{{ t('audit.query') }}</span>
        <QueryInput v-model="q" scope="audit" :error="queryError" :placeholder="t('admin.auditFilter')" @submit="load" />
      </div>
      <div class="row actions-row">
        <span v-if="badRange" class="small error-text" role="alert">{{ t('audit.badRange') }}</span>
        <span class="grow" />
        <Button :label="t('common.clear')" text severity="secondary" @click="reset" />
        <Button :label="t('audit.exportCsv')" icon="pi pi-file" severity="secondary" outlined :loading="exporting" :disabled="badRange" @click="exportAs('csv')" />
        <Button :label="t('audit.exportMd')" icon="pi pi-hashtag" severity="secondary" outlined :loading="exporting" :disabled="badRange" @click="exportAs('md')" />
        <Button type="submit" :label="t('common.apply')" icon="pi pi-search" :disabled="badRange" />
      </div>
    </form>
    <Message v-if="entries.length >= limit" severity="info" :closable="false" style="margin-bottom: 8px">{{ t('audit.limitReached', { limit }) }}</Message>
    <DataTable v-stack :value="entries" data-key="id" :loading="loading" :aria-busy="loading" paginator :rows="prefs.rowsFor(50)" :rows-per-page-options="prefs.rowsPerPage" size="small" sort-field="created_at" :sort-order="-1">
      <Column field="created_at" :header="t('common.time')" sortable style="width: 160px"><template #body="{ data }"><span class="small" :title="formatFullDateTime(data.created_at)">{{ formatDateTime(data.created_at) }}</span></template></Column>
      <Column field="user_email" :header="t('common.user')" sortable style="width: 220px">
        <template #body="{ data }"><span class="row nowrap small" style="gap: 6px; min-width: 0"><UserAvatar v-if="data.user_email" :avatar="data.user_avatar" :user-id="data.user_id" :email="data.user_email" :name="data.user_name || data.user_email" :size="24" /><span class="ellipsis" :title="data.user_email">{{ data.user_email || t('common.system') }}</span></span></template>
      </Column>
      <Column field="action" :header="t('admin.action')" sortable style="width: 170px"><template #body="{ data }"><Tag :value="data.action" :severity="actionSeverity(data.action)" /></template></Column>
      <Column field="entity_type" :header="t('admin.entity')" sortable style="width: 130px" />
      <Column field="details" :header="t('admin.detailsCol')"><template #body="{ data }"><span class="small mono audit-details">{{ data.details }}</span></template></Column>
      <Column style="width: 60px">
        <template #body="{ data }"><Button icon="pi pi-angle-right" text rounded size="small" severity="secondary" :aria-label="t('audit.showDetails', { action: data.action, time: formatDateTime(data.created_at) })" v-tooltip="t('common.details')" @click="show(data)" /></template>
      </Column>
      <template #empty><div class="empty"><i class="pi pi-history" />{{ t('admin.noAudit') }}</div></template>
    </DataTable>
  </div>

  <Drawer v-model:visible="drawer" position="right" :header="t('audit.entry')" class="audit-drawer" :style="{ width: 'min(520px, 100vw)' }">
    <dl v-if="detail" class="kv">
      <dt class="small muted">{{ t('common.time') }}</dt><dd>{{ formatFullDateTime(detail.created_at) }}</dd>
      <dt class="small muted">{{ t('common.user') }}</dt><dd class="row nowrap" style="gap: 6px"><UserAvatar v-if="detail.user_email" :avatar="detail.user_avatar" :user-id="detail.user_id" :email="detail.user_email" :name="detail.user_name || detail.user_email" :size="28" />{{ detail.user_name ? `${detail.user_name} <${detail.user_email}>` : detail.user_email || t('common.system') }} <span v-if="detail.user_id" class="mono small muted">{{ detail.user_id }}</span></dd>
      <dt class="small muted">{{ t('admin.action') }}</dt><dd><Tag :value="detail.action" :severity="actionSeverity(detail.action)" /></dd>
      <dt class="small muted">{{ t('admin.entity') }}</dt><dd>{{ detail.entity_type || '—' }}</dd>
      <dt class="small muted">{{ t('admin.entityId') }}</dt><dd class="mono small" style="word-break: break-all">{{ detail.entity_id || '—' }}</dd>
      <dt class="small muted">{{ t('audit.entryId') }}</dt><dd class="mono small" style="word-break: break-all">{{ detail.id }}</dd>
    </dl>
    <div v-if="detail" class="section-title">{{ t('admin.detailsCol') }}</div>
    <pre v-if="detail && details.text" class="console" tabindex="0" :aria-label="t('admin.detailsCol')">{{ details.text }}</pre>
    <div v-else-if="detail" class="muted small">—</div>
  </Drawer>
</template>

<style scoped>
.audit-filters {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 180px), 1fr));
  gap: 10px 12px;
  margin-bottom: 12px;
}
.query-field,
.actions-row {
  grid-column: 1 / -1;
}
.audit-details {
  word-break: break-word;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.kv dd {
  margin: 0;
}
</style>
