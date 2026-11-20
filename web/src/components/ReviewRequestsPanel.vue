<script setup lang="ts">
// Review requests of a requirement or test: who was asked, their decision,
// and "Request review" (choose reviewers, message, due date, checklist).
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import MultiSelect from 'primevue/multiselect'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import ReviewDialog from './ReviewDialog.vue'
import { api, asItems } from '@/lib/api'
import { P } from '@/lib/permissions'
import { parseChecklist, reviewSeverity } from '@/lib/notifications'
import { formatDate } from '@/lib/format'
import { useAuthStore } from '@/stores/auth'
import { useProjectPeopleStore } from '@/stores/projectPeople'
import { useNotify } from '@/composables/notify'
import type { ReviewRequest } from '@/lib/types'

const props = defineProps<{ kind: 'requirement' | 'test'; item: { id: string; key: string; project_id?: string } }>()
const { t } = useI18n()
const auth = useAuthStore()
const people = useProjectPeopleStore()
const notify = useNotify()
const reviews = ref<ReviewRequest[]>([])
const asking = ref(false)
const reviewers = ref<string[]>([])
const message = ref('')
const due = ref('')
const checklist = ref('')
const busy = ref(false)
const current = ref<ReviewRequest | null>(null)
const working = ref(false)

const canRequest = computed(() => auth.can(props.kind === 'requirement' ? P.requirementWrite : P.testWrite))
const users = computed(() => people.people.filter((p) => p.kind === 'user' && p.user_id).map((p) => ({ label: p.name || p.email || p.handle, value: p.user_id! })))
const visible = computed(() => reviews.value.filter((r) => r.status !== 'cancelled'))

async function load() {
  try {
    reviews.value = asItems(await api.get<{ items: ReviewRequest[] }>('/reviews', { entity_type: props.kind, entity_id: props.item.id }))
  } catch {
    reviews.value = []
  }
}
onMounted(load)
watch(() => props.item.id, load)

async function openAsk() {
  await people.ensure(props.item.project_id ?? null)
  const defaults = await api.get<Record<string, string[]>>('/review-checklists').catch(() => ({}) as Record<string, string[]>)
  checklist.value = (defaults[props.kind] ?? []).join('\n')
  reviewers.value = []
  message.value = ''
  due.value = ''
  asking.value = true
}

async function send() {
  busy.value = true
  try {
    const res = await api.post<{ items: ReviewRequest[] }>('/reviews', {
      entity_type: props.kind,
      entity_id: props.item.id,
      reviewers: reviewers.value,
      message: message.value,
      due_date: due.value,
      checklist: parseChecklist(checklist.value),
    })
    const n = asItems(res).length
    notify.ok(n ? t('reviews.requested', { n }, n) : t('reviews.alreadyPending'))
    asking.value = false
    await load()
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = false
  }
}

async function cancel(r: ReviewRequest) {
  try {
    await api.patch(`/reviews/${r.id}`, { cancel: true })
    await load()
  } catch (e) {
    notify.fail(e)
  }
}

function work(r: ReviewRequest) {
  current.value = r
  working.value = true
}
const mine = (r: ReviewRequest) => r.reviewer_id === auth.user?.id

</script>

<template>
  <div class="review-requests">
    <div class="row head">
      <span class="small grow"><b>{{ t('reviews.panelTitle') }}</b></span>
      <Button v-if="canRequest" :label="t('reviews.request')" icon="pi pi-user-plus" size="small" text @click="openAsk" />
    </div>
    <ul v-if="visible.length" class="list">
      <li v-for="r in visible" :key="r.id" class="row">
        <span class="grow small">
          {{ r.reviewer_name || r.reviewer_email }}
          <Tag :value="t(`reviews.status.${r.status}`)" :severity="reviewSeverity(r.status)" />
          <span v-if="r.outdated && r.status !== 'pending'" class="muted">· {{ t('reviews.outdatedShort') }}</span>
          <span v-if="r.decided_at" class="muted">· {{ formatDate(r.decided_at) }}</span>
          <span v-if="r.comment" class="comment">{{ r.comment }}</span>
        </span>
        <Button v-if="mine(r) && r.status === 'pending'" :label="t('reviews.start')" size="small" @click="work(r)" />
        <Button v-else-if="mine(r)" :label="t('reviews.show')" size="small" text @click="work(r)" />
        <Button
          v-if="r.status === 'pending' && canRequest"
          icon="pi pi-times"
          size="small"
          text
          rounded
          severity="secondary"
          :aria-label="t('reviews.cancelFor', { who: r.reviewer_name || r.reviewer_email })"
          @click="cancel(r)"
        />
      </li>
    </ul>
    <p v-else class="small muted">{{ t('reviews.none') }}</p>

    <Dialog v-model:visible="asking" modal :header="t('reviews.requestHeader', { key: item.key })" :style="{ width: 'min(560px, 96vw)' }">
      <div class="form">
        <label for="rr-who">{{ t('reviews.reviewers') }}</label>
        <MultiSelect
          v-model="reviewers"
          input-id="rr-who"
          :options="users"
          option-label="label"
          option-value="value"
          filter
          display="chip"
          fluid
          :placeholder="t('reviews.chooseReviewers')"
        />
        <label for="rr-msg">{{ t('reviews.message') }}</label>
        <Textarea id="rr-msg" v-model="message" rows="3" auto-resize fluid />
        <label for="rr-due">{{ t('reviews.dueDate') }}</label>
        <InputText id="rr-due" v-model="due" type="date" />
        <label for="rr-list">{{ t('reviews.checklist') }}</label>
        <Textarea id="rr-list" v-model="checklist" rows="6" auto-resize fluid aria-describedby="rr-list-help" />
        <small id="rr-list-help" class="muted">{{ t('reviews.checklistHelp') }}</small>
      </div>
      <template #footer>
        <Button :label="t('common.cancel')" severity="secondary" text @click="asking = false" />
        <Button :label="t('reviews.send')" icon="pi pi-send" :disabled="!reviewers.length" :loading="busy" @click="send" />
      </template>
    </Dialog>
    <ReviewDialog v-model:visible="working" :review="current" @saved="load" />
  </div>
</template>

<style scoped>
.review-requests {
  padding: 8px 12px;
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  background: var(--qtf-card);
}
.head {
  gap: 8px;
}
.list {
  list-style: none;
  margin: 4px 0 0;
  padding: 0;
}
.list li {
  gap: 8px;
  padding: 4px 0;
}
.comment {
  display: block;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.form label {
  font-weight: 600;
  font-size: 13px;
  margin-top: 6px;
}
</style>
