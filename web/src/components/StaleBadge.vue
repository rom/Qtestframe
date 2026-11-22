<script setup lang="ts">
// "Stale" next to a test result: the result is older than the project's
// limit, or the test or a requirement it verifies changed since, so it does
// not count as passing evidence. Text and icon, not only colour.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatDateTime } from '@/lib/format'

const props = defineProps<{ stale?: boolean | null; reason?: string | null; recordedAt?: string | null; result?: string | null }>()
const { t, te } = useI18n()
const why = computed(() => {
  const k = `correlations.staleReason.${props.reason ?? ''}`
  return props.reason && te(k) ? t(k) : t('correlations.staleHint')
})
const title = computed(() => (props.recordedAt ? `${why.value} (${formatDateTime(props.recordedAt)})` : why.value))
</script>

<template>
  <span v-if="stale" class="stale-badge" :title="title">
    <i class="pi pi-clock" aria-hidden="true" />
    <span>{{ t('correlations.stale') }}</span>
    <span class="sr-only">: {{ title }}</span>
  </span>
</template>

<style>
.stale-badge {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  padding: 0 6px;
  border-radius: 999px;
  border: 1px dashed var(--p-amber-600, #d97706);
  color: var(--p-amber-800, #92400e);
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
}
.app-dark .stale-badge {
  color: var(--p-amber-300, #fcd34d);
  border-color: var(--p-amber-400, #fbbf24);
}
</style>
