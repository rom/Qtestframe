<script setup lang="ts">
// A trace link marked suspect (the requirement or test changed after the
// link was made): shows why and since when, and lets someone who may edit
// links confirm that the test still verifies the requirement.
import Button from 'primevue/button'
import { useI18n } from 'vue-i18n'
import { api } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import type { TraceLink } from '@/lib/types'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ link: TraceLink; canConfirm: boolean }>()
const emit = defineEmits<{ confirmed: [] }>()
const { t } = useI18n()
const notify = useNotify()

async function confirm() {
  try {
    const res = await api.post<{ confirmed: number; status_changes: { key: string; to: string }[] }>(`/links/${props.link.id}/confirm`)
    notify.ok(t('suspect.confirmed'), res?.status_changes?.length ? t('suspect.statusChanged', { key: res.status_changes[0].key, status: res.status_changes[0].to }) : undefined)
    emit('confirmed')
  } catch (e) {
    notify.fail(e)
  }
}
</script>

<template>
  <span v-if="link.suspect_since" class="suspect row nowrap" :title="`${link.suspect_reason ?? ''} (${formatDateTime(link.suspect_since)})`">
    <i class="pi pi-exclamation-triangle" aria-hidden="true" />
    <span>{{ t('suspect.label') }}</span>
    <span class="sr-only">: {{ link.suspect_reason }}, {{ formatDateTime(link.suspect_since) }}</span>
    <Button v-if="canConfirm" :label="t('suspect.confirm')" size="small" text @click="confirm" />
  </span>
</template>

<style>
.suspect {
  gap: 4px;
  font-size: 12px;
  color: var(--p-amber-700, #b45309);
}
.app-dark .suspect {
  color: var(--p-amber-300);
}
</style>
