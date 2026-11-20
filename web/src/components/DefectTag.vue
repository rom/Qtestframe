<script setup lang="ts">
// Status or severity of a defect as a coloured tag.
import { computed } from 'vue'
import Tag from 'primevue/tag'
import { useI18n } from 'vue-i18n'
import { severityTag, statusTag } from '@/lib/defectTracker'

const props = defineProps<{ value?: string | null; kind?: 'status' | 'severity' }>()
const { t, te } = useI18n()
const label = computed(() => {
  const k = `defectsView.${props.kind === 'severity' ? 'severity' : 'status'}.${props.value ?? ''}`
  return te(k) ? t(k) : (props.value ?? '')
})
const severity = computed(() => (props.kind === 'severity' ? severityTag(props.value) : statusTag(props.value)))
</script>

<template>
  <Tag v-if="value" :value="label" :severity="severity" style="white-space: nowrap" />
  <span v-else class="muted small">—</span>
</template>
