<script setup lang="ts">
import { computed } from 'vue'
import Tag from 'primevue/tag'
import { coverageSeverity, obligationSeverity, prioritySeverity, resultSeverity, statusSeverity } from '@/lib/colors'
import { humanize } from '@/lib/format'

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
const label = computed(() => (props.kind === 'obligation' ? (props.value ?? '').replace(/_/g, ' ').toUpperCase() : humanize(props.value)))
</script>

<template>
  <Tag v-if="value" :value="label" :severity="severity" :class="{ 'obligation-tag': kind === 'obligation' }" />
  <span v-else class="muted small">{{ empty ?? '—' }}</span>
</template>
