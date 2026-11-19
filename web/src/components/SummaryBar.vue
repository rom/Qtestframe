<script setup lang="ts">
import { computed } from 'vue'
import { summarize } from '@/lib/summary'
import { humanize } from '@/lib/format'

const props = defineProps<{ summary?: Record<string, number> | null; showText?: boolean }>()
const s = computed(() => summarize(props.summary))
const title = computed(() => s.value.segments.map((x) => `${humanize(x.status)}: ${x.count}`).join(' · ') || 'No executions')
</script>

<template>
  <div :title="title" style="display: flex; flex-direction: column; gap: 4px; min-width: 120px">
    <div class="sbar">
      <span v-for="seg in s.segments" :key="seg.status" :style="{ width: seg.pct + '%', background: seg.color }" />
    </div>
    <div v-if="showText" class="small muted row nowrap" style="justify-content: space-between">
      <span>{{ s.executed }}/{{ s.total }} executed</span>
      <span v-if="s.executed">{{ s.passRate }}% pass</span>
    </div>
  </div>
</template>
