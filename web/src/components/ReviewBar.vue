<script setup lang="ts">
import { patchEntry, recordUndo } from '@/lib/undoActions'
// Review decision of a requirement (Approve / Reject) or test (Mark ready),
// for holders of requirement.review / test.review. Reviewers without write
// permission can change only the status this way.
import { computed, ref } from 'vue'
import Button from 'primevue/button'
import StatusTag from './StatusTag.vue'
import { api } from '@/lib/api'
import { P } from '@/lib/permissions'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'
import { enumLabel } from '@/i18n/labels'

const props = defineProps<{ kind: 'requirement' | 'test'; item: { id: string; key: string; status: string } }>()
const emit = defineEmits<{ changed: [item: unknown] }>()
const { t } = useI18n()
const auth = useAuthStore()
const notify = useNotify()
const busy = ref('')
const canReview = computed(() => auth.can(props.kind === 'requirement' ? P.requirementReview : P.testReview))
const decisions = computed(() =>
  props.kind === 'requirement'
    ? [
        { status: 'approved', label: t('review.approve'), icon: 'pi pi-check', severity: 'success' },
        { status: 'rejected', label: t('review.reject'), icon: 'pi pi-times', severity: 'danger' },
      ]
    : [{ status: 'ready', label: t('review.markReady'), icon: 'pi pi-check', severity: 'success' }],
)
const open = computed(() => decisions.value.filter((d) => d.status !== props.item.status))
async function decide(status: string) {
  busy.value = status
  try {
    const path = props.kind === 'requirement' ? `/requirements/${props.item.id}` : `/tests/${props.item.id}`
    const old = props.item.status
    const updated = await api.patch(path, { status })
    recordUndo(patchEntry(t('undo.a.review', { key: props.item.key, status: enumLabel('status', status) }), path, { status: old }, { status }))
    notify.ok(t('review.done', { key: props.item.key, status: enumLabel('status', status) }))
    emit('changed', updated)
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = ''
  }
}
</script>

<template>
  <div v-if="canReview && open.length" class="review-bar row" role="group" :aria-label="t('review.title')">
    <i class="pi pi-verified" aria-hidden="true" />
    <span class="grow small">
      <b>{{ t('review.title') }}</b>
      {{ item.status === 'review' ? t('review.waiting') : t('review.current') }}
      <StatusTag :value="item.status" />
    </span>
    <Button
      v-for="d in open"
      :key="d.status"
      :label="d.label"
      :icon="d.icon"
      :severity="d.severity"
      size="small"
      :outlined="item.status !== 'review'"
      :loading="busy === d.status"
      @click="decide(d.status)"
    />
  </div>
</template>

<style scoped>
.review-bar {
  gap: 8px;
  padding: 8px 12px;
  border: 1px solid var(--qtf-border);
  border-left: 4px solid var(--p-primary-color);
  border-radius: 8px;
  background: var(--qtf-card);
}
</style>
