<script setup lang="ts">
// The confirmation dialog of the impact gate (composables/impact.ts),
// mounted once in the project layout: bulk changes, imports and baseline
// restores ask here what to do with the impact of their change.
import ImpactConfirmDialog from './ImpactConfirmDialog.vue'
import { computed } from 'vue'
import { impactGate, settleImpact } from '@/composables/impact'
import type { ImpactChoice } from '@/lib/correlations'

const visible = computed({
  get: () => impactGate.open,
  set: (v: boolean) => {
    if (!v) settleImpact(null)
  },
})
</script>

<template>
  <ImpactConfirmDialog
    v-model:visible="visible"
    :impact="impactGate.impact"
    :project-id="impactGate.projectId"
    :changed="impactGate.fields"
    @confirm="(c: ImpactChoice) => settleImpact(c)"
    @cancel="settleImpact(null)"
  />
</template>
