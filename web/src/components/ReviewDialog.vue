<script setup lang="ts">
// The reviewer's side of a review request: tick the checklist, comment,
// approve or ask for changes.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import SuggestInput from './SuggestInput.vue'
import { api } from '@/lib/api'
import { REVIEW_COMMENT, rememberValues, savedEntries } from '@/lib/suggest'
import { usePrefsStore } from '@/stores/prefs'
import { canApprove, canRequestChanges, openChecklistItems } from '@/lib/notifications'
import { useNotify } from '@/composables/notify'
import { useNotificationsStore } from '@/stores/notifications'
import type { ChecklistItem, ReviewRequest } from '@/lib/types'

const props = defineProps<{ review: ReviewRequest | null }>()
const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ saved: [review: ReviewRequest] }>()
const { t } = useI18n()
const notify = useNotify()
const notifications = useNotificationsStore()
const list = ref<ChecklistItem[]>([])
const comment = ref('')
const busy = ref('')
const tried = ref('')

watch(
  () => [props.review, visible.value],
  () => {
    list.value = (props.review?.checklist ?? []).map((c) => ({ ...c, note: c.note ?? '' }))
    comment.value = props.review?.comment ?? ''
    tried.value = ''
  },
  { immediate: true },
)

const open = computed(() => openChecklistItems(list.value))
const pending = computed(() => props.review?.status === 'pending')

async function save(decision = '') {
  if (!props.review) return
  tried.value = decision
  if (decision === 'approved' && !canApprove(list.value)) return
  if (decision === 'changes_requested' && !canRequestChanges(comment.value)) return
  busy.value = decision || 'save'
  try {
    const out = await api.patch<ReviewRequest>(`/reviews/${props.review.id}`, {
      checklist: list.value.map((c) => ({ text: c.text, checked: c.checked, note: c.note })),
      comment: comment.value,
      ...(decision ? { decision } : {}),
    })
    void rememberValues(usePrefsStore().prefs, props.review.project_id, savedEntries({ [REVIEW_COMMENT]: comment.value }, { [REVIEW_COMMENT]: props.review.comment }))
    notify.ok(decision ? t(`reviews.done.${decision}`, { key: out.entity_key }) : t('reviews.saved'))
    emit('saved', out)
    if (decision) {
      void notifications.load()
      visible.value = false
    }
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = ''
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="review ? t('reviews.reviewHeader', { key: review.entity_key }) : ''" :style="{ width: 'min(640px, 96vw)' }">
    <template v-if="review">
      <p class="title">{{ review.entity_title }}</p>
      <p class="small muted">{{ t('reviews.requestedBy', { who: review.requested_by }) }}<template v-if="review.due_date"> · {{ t('reviews.due', { date: review.due_date }) }}</template></p>
      <Message v-if="review.outdated" severity="warn" :closable="false">{{ t('reviews.outdated') }}</Message>
      <p v-if="review.message" class="message">{{ review.message }}</p>
      <fieldset class="checklist">
        <legend>{{ t('reviews.checklist') }}</legend>
        <p v-if="!list.length" class="small muted">{{ t('reviews.noChecklist') }}</p>
        <div v-for="(c, i) in list" :key="i" class="check">
          <div class="row">
            <Checkbox v-model="c.checked" binary :input-id="`rv-c-${i}`" :disabled="!pending" />
            <label :for="`rv-c-${i}`" class="grow">{{ c.text }}</label>
          </div>
          <InputText
            v-if="!c.checked"
            v-model="c.note"
            size="small"
            class="note"
            :disabled="!pending"
            :placeholder="t('reviews.notePlaceholder')"
            :aria-label="t('reviews.noteFor', { item: c.text })"
          />
        </div>
      </fieldset>
      <label for="rv-comment" class="block small">{{ t('reviews.comment') }}</label>
      <SuggestInput id="rv-comment" v-model="comment" field="review.comment" :project-id="review.project_id" multiline rows="3" auto-resize fluid :disabled="!pending" />
      <Message v-if="tried === 'approved' && open.length" severity="error" :closable="false">{{ t('reviews.needChecklist', { n: open.length }, open.length) }}</Message>
      <Message v-if="tried === 'changes_requested' && !comment.trim()" severity="error" :closable="false">{{ t('reviews.needComment') }}</Message>
    </template>
    <template #footer>
      <Button :label="t('common.close')" severity="secondary" text @click="visible = false" />
      <template v-if="pending">
        <Button :label="t('reviews.saveDraft')" outlined :loading="busy === 'save'" @click="save()" />
        <Button :label="t('reviews.requestChanges')" icon="pi pi-replay" severity="danger" outlined :loading="busy === 'changes_requested'" @click="save('changes_requested')" />
        <Button :label="t('reviews.approve')" icon="pi pi-check" severity="success" :loading="busy === 'approved'" @click="save('approved')" />
      </template>
    </template>
  </Dialog>
</template>

<style scoped>
.title {
  font-weight: 600;
  margin: 0 0 4px;
  overflow-wrap: anywhere;
}
.message {
  white-space: pre-wrap;
  padding: 8px 12px;
  border-left: 3px solid var(--qtf-border);
}
.checklist {
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  padding: 8px 12px;
  margin: 12px 0;
}
.check {
  padding: 6px 0;
}
.check .row {
  gap: 8px;
}
.note {
  margin: 4px 0 0 30px;
  width: calc(100% - 30px);
}
.block {
  display: block;
  margin-bottom: 4px;
}
</style>
