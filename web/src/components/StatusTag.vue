<script setup lang="ts">
import { computed } from 'vue'
import Tag from 'primevue/tag'
import { coverageSeverity, obligationSeverity, prioritySeverity, resultSeverity, statusSeverity } from '@/lib/colors'
import { enumLabel, obligationHint, obligationKeyword } from '@/i18n/labels'

const props = defineProps<{ value?: string | null; kind?: 'status' | 'result' | 'coverage' | 'obligation' | 'priority'; empty?: string }>()

const severity = computed(() => {
  switch (props.kind) {
    case 'result':
      return resultSeverity(props.value)
    case 'coverage':
      return coverageSeverity(props.value)
    case 'obligation':
      return obligationSeverity(props.value)
    case 'priority':
      return prioritySeverity(props.value)
    default:
      return statusSeverity(props.value)
  }
})
// RFC 2119 keywords stay English in the tag; the tooltip explains them in the active language
const label = computed(() => (props.kind === 'obligation' ? obligationKeyword(props.value) : enumLabel(props.kind ?? 'status', props.value)))
const hint = computed(() => (props.kind === 'obligation' ? obligationHint(props.value) : ''))
</script>

<template>
  <Tag
    v-if="value"
    v-tooltip.top="hint || undefined"
    :value="label"
    :severity="severity"
    :class="{ 'obligation-tag': kind === 'obligation' }"
    style="white-space: nowrap"
    :aria-label="hint ? `${label} – ${hint}` : undefined"
  />
  <span v-else class="muted small">{{ empty ?? '—' }}</span>
</template>
