<script setup lang="ts">
// Review requests across projects: the ones sent to the signed-in user, and
// the ones they asked for, with bulk actions for each.
import ItemRef from '../components/ItemRef.vue'
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import ReviewDialog from '@/components/ReviewDialog.vue'
import InReviewList from '@/components/InReviewList.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { isoDay, notificationTarget, overdue, reviewSeverity } from '@/lib/notifications'
import { formatDate } from '@/lib/format'
import { useNotify } from '@/composables/notify'
import { useNotificationsStore } from '@/stores/notifications'
import type { ReviewRequest } from '@/lib/types'

const { t } = useI18n()
const notify = useNotify()
const notifications = useNotificationsStore()
const side = ref<'mine' | 'requested'>('mine')
const sides = computed(() => [
  { label: t('reviews.toReview'), value: 'mine' },
  { label: t('reviews.requestedByMe'), value: 'requested' },
])
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
const inReview = ref<InstanceType<typeof InReviewList> | null>(null)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const params = side.value === 'mine' ? { reviewer: 'me', status: status.value } : { requested_by: 'me', status: status.value }
    list.value = asItems(await api.get<{ items: ReviewRequest[] }>('/reviews', params))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)
watch([status, side], () => {
  selection.value = []
  void load()
})

function work(r: ReviewRequest) {
  current.value = r
  working.value = true
}
function itemRoute(r: ReviewRequest) {
  return notificationTarget({ project_id: r.project_id, entity_type: r.entity_type, entity_id: r.entity_id })!
}

// ---------- bulk ----------
const selection = ref<ReviewRequest[]>([])
const pendingSel = computed(() => selection.value.filter((r) => r.status === 'pending'))
watch(list, (l) => {
  const ids = new Set(l.map((r) => r.id))
  selection.value = selection.value.filter((s) => ids.has(s.id))
})
type Action = 'approve' | 'request_changes' | 'cancel' | 'remind' | 'due_date'
const dialog = ref<Action | null>(null)
const comment = ref('')
const due = ref('')
const ack = ref(false)
const busy = ref(false)
function open(a: Action) {
  comment.value = ''
  due.value = ''
  ack.value = false
  dialog.value = a
}
const canApply = computed(() => {
  if (dialog.value === 'approve') return ack.value
  if (dialog.value === 'request_changes') return !!comment.value.trim()
  return true
})
async function apply() {
  const action = dialog.value
  if (!action) return
  busy.value = true
  try {
    const res = await api.post<{ updated: { id: string }[]; skipped: { key: string; reason: string }[] }>('/reviews/bulk', {
      ids: pendingSel.value.map((r) => r.id),
      action,
      comment: comment.value,
      due_date: due.value,
    })
    dialog.value = null
    notify.ok(
      t(`reviews.bulk.done.${action}`, { n: res.updated.length }, res.updated.length),
      res.skipped.length ? t('reviews.bulk.skipped', { n: res.skipped.length }, res.skipped.length) : undefined,
    )
    if (action === 'approve' || action === 'request_changes') void notifications.load()
    await load()
    void inReview.value?.load()
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section>
    <div class="page-header">
      <div>
        <h1>{{ t('titles.reviews') }}</h1>
        <div class="sub">{{ side === 'mine' ? t('reviews.intro') : t('reviews.introRequested') }}</div>
      </div>
      <div class="actions">
        <SelectButton v-model="side" :options="sides" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('reviews.side')" />
        <SelectButton v-model="status" :options="statuses" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('reviews.filter')" />
      </div>
    </div>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div class="card">
      <div v-if="selection.length" class="bulkbar" role="toolbar" :aria-label="t('reviews.bulk.toolbar')">
        <b>{{ t('common.selected', { count: selection.length }) }}</b>
        <template v-if="pendingSel.length">
          <template v-if="side === 'mine'">
            <Button :label="t('reviews.approve')" icon="pi pi-check" size="small" severity="success" outlined aria-haspopup="dialog" @click="open('approve')" />
            <Button :label="t('reviews.requestChanges')" icon="pi pi-replay" size="small" severity="danger" outlined aria-haspopup="dialog" @click="open('request_changes')" />
          </template>
          <template v-else>
            <Button :label="t('reviews.bulk.remind')" icon="pi pi-bell" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="open('remind')" />
            <Button :label="t('reviews.bulk.dueDate')" icon="pi pi-calendar" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="open('due_date')" />
            <Button :label="t('reviews.bulk.cancel')" icon="pi pi-times" size="small" severity="danger" outlined aria-haspopup="dialog" @click="open('cancel')" />
          </template>
          <span v-if="pendingSel.length < selection.length" class="small muted">{{ t('reviews.bulk.onlyPending', { n: pendingSel.length, total: selection.length }) }}</span>
        </template>
        <span v-else class="small muted">{{ t('reviews.bulk.nonePending') }}</span>
        <Button icon="pi pi-times" size="small" text rounded severity="secondary" :aria-label="t('workspace.clearSelection')" v-tooltip.bottom="t('workspace.clearSelection')" @click="selection = []" />
      </div>
      <DataTable v-model:selection="selection" :value="list" :loading="loading" data-key="id" size="small" striped-rows>
        <Column selection-mode="multiple" style="width: 2.6rem" />
        <Column :header="t('reviews.item')">
          <template #body="{ data }">
            <ItemRef :to="itemRoute(data)" :item-key="data.entity_key" :title="data.entity_title" title-class="title" />
            <span v-if="data.outdated" class="small muted"> · {{ t('reviews.outdatedShort') }}</span>
          </template>
        </Column>
        <Column v-if="side === 'mine'" field="requested_by" :header="t('reviews.requestedByCol')"><template #body="{ data }"><span class="small">{{ data.requested_by }}</span></template></Column>
        <Column v-else field="reviewer_email" :header="t('reviews.reviewers')"><template #body="{ data }"><span class="small">{{ data.reviewer_name || data.reviewer_email }}</span></template></Column>
        <Column field="due_date" :header="t('reviews.dueDate')" sortable style="width: 130px">
          <template #body="{ data }">
            <span class="small" :class="{ 'late error-text': overdue(data, today) }">{{ data.due_date ? formatDate(data.due_date) : '' }}</span>
            <span v-if="overdue(data, today)" class="sr-only">{{ t('reviews.overdue') }}</span>
          </template>
        </Column>
        <Column field="status" :header="t('common.status')" style="width: 160px">
          <template #body="{ data }"><Tag :value="t(`reviews.status.${data.status}`)" :severity="reviewSeverity(data.status)" /></template>
        </Column>
        <Column v-if="side === 'mine'" style="width: 120px">
          <template #header><span class="sr-only">{{ t('common.actions') }}</span></template>
          <template #body="{ data }">
            <Button :label="data.status === 'pending' ? t('reviews.start') : t('reviews.show')" size="small" :outlined="data.status !== 'pending'" @click="work(data)" />
          </template>
        </Column>
        <template #empty>
          <div class="empty">{{ side === 'requested' ? t('reviews.noneRequested') : status === 'pending' ? t('reviews.nonePending') : t('reviews.noneAtAll') }}</div>
        </template>
      </DataTable>
    </div>
    <InReviewList v-if="side === 'mine'" ref="inReview" />
    <ReviewDialog v-model:visible="working" :review="current" @saved="() => (load(), inReview?.load())" />

    <Dialog :visible="!!dialog" modal :header="dialog ? t(`reviews.bulk.header.${dialog}`, { n: pendingSel.length }, pendingSel.length) : ''" :style="{ width: 'min(500px, 96vw)' }" @update:visible="(v: boolean) => !v && (dialog = null)">
      <div class="form">
        <template v-if="dialog === 'approve' || dialog === 'request_changes' || dialog === 'remind'">
          <label for="bk-rv-comment">{{ dialog === 'remind' ? t('reviews.message') : t('reviews.comment') }}</label>
          <Textarea id="bk-rv-comment" v-model="comment" rows="3" auto-resize fluid />
        </template>
        <div v-if="dialog === 'approve'" class="row ack">
          <Checkbox v-model="ack" binary input-id="bk-rv-ack" />
          <label for="bk-rv-ack">{{ t('reviews.bulk.ack') }}</label>
        </div>
        <p v-if="dialog === 'request_changes'" class="small muted">{{ t('reviews.needComment') }}</p>
        <template v-if="dialog === 'due_date'">
          <label for="bk-rv-due">{{ t('reviews.dueDate') }}</label>
          <InputText id="bk-rv-due" v-model="due" type="date" aria-describedby="bk-rv-due-help" />
          <small id="bk-rv-due-help" class="muted">{{ t('reviews.bulk.emptyClears') }}</small>
        </template>
        <p v-if="dialog === 'cancel'" class="small">{{ t('reviews.bulk.cancelExplain') }}</p>
      </div>
      <template #footer>
        <Button :label="t('common.close')" severity="secondary" text @click="dialog = null" />
        <Button :label="dialog ? t(`reviews.bulk.apply.${dialog}`) : ''" :severity="dialog === 'cancel' || dialog === 'request_changes' ? 'danger' : undefined" :disabled="!canApply" :loading="busy" @click="apply" />
      </template>
    </Dialog>
  </section>
</template>

<style scoped>
.title {
  margin-left: 6px;
  overflow-wrap: anywhere;
}
.late {
  font-weight: 600;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.form > label {
  font-weight: 600;
  font-size: 13px;
}
.ack {
  gap: 8px;
  align-items: flex-start;
  flex-wrap: nowrap;
  margin-top: 6px;
}
</style>
