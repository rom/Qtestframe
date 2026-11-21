<script setup lang="ts">
// The risks a requirement addresses (shown in the requirement editor).
import ItemRef from './ItemRef.vue'
import { ref, watch } from 'vue'
import Tag from 'primevue/tag'
import { api, asItems } from '@/lib/api'
import { riskLevelSeverity, riskStatusSeverity } from '@/lib/risks'
import type { Risk } from '@/lib/types'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ requirementId: string; projectId: string }>()
const { t } = useI18n()
const risks = ref<Risk[]>([])
const failed = ref(false)

watch(
  () => props.requirementId,
  async (id) => {
    failed.value = false
    try {
      risks.value = asItems(await api.get<{ items: Risk[] }>(`/requirements/${id}/risks`))
    } catch {
      risks.value = []
      failed.value = true
    }
  },
  { immediate: true },
)
</script>

<template>
  <div v-if="!failed" class="requirement-risks">
    <div class="section-title">
      <i class="pi pi-shield" aria-hidden="true" /> {{ t('risksView.addressesRisks', { count: risks.length }, risks.length) }}
      <span class="grow" />
      <RouterLink :to="{ name: 'risks', params: { projectId } }" class="small" style="text-transform: none; letter-spacing: 0">{{ t('risksView.openRegister') }}</RouterLink>
    </div>
    <div v-if="!risks.length" class="muted small">{{ t('risksView.noRisksForRequirement') }}</div>
    <div v-for="r in risks" :key="r.id" class="row nowrap" style="padding: 5px 0; border-bottom: 1px solid var(--qtf-border)">
      <ItemRef :to="{ name: 'risks', params: { projectId }, query: { risk: r.id } }" :item-key="r.key" :title="r.title" title-class="ellipsis" grow />
      <span class="small muted" :title="t('risksView.score')">{{ r.score }}</span>
      <Tag :value="t(`risksView.level.${r.level}`)" :severity="riskLevelSeverity(r.level)" />
      <Tag :value="t(`risksView.status.${r.status}`)" :severity="riskStatusSeverity(r.status)" />
    </div>
  </div>
</template>
