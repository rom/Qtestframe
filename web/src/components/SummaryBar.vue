<script setup lang="ts">
import { computed } from 'vue'
import { summarize } from '@/lib/summary'
import { useI18n } from 'vue-i18n'
import { enumLabel } from '@/i18n/labels'

const props = defineProps<{ summary?: Record<string, number> | null; showText?: boolean }>()
const s = computed(() => summarize(props.summary))
const { t } = useI18n()
const title = computed(() => s.value.segments.map((x) => `${enumLabel('result', x.status)}: ${x.count}`).join(' · ') || t('summary.none'))
</script>

<template>
  <div :title="title" style="display: flex; flex-direction: column; gap: 4px; min-width: 120px">
    <div class="sbar">
      <span v-for="seg in s.segments" :key="seg.status" :style="{ width: seg.pct + '%', background: seg.color }" />
    </div>
    <div v-if="showText" class="small muted row nowrap" style="justify-content: space-between">
      <span>{{ t('summary.executed', { executed: s.executed, total: s.total }) }}</span>
      <span v-if="s.executed">{{ t('summary.pass', { rate: s.passRate }) }}</span>
    </div>
  </div>
</template>
