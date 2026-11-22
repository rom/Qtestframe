<script setup lang="ts">
// The risks a requirement mitigates (shown in the requirement editor), with
// adding and removing links from the requirement's side.
import ItemRef from './ItemRef.vue'
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import { api, asItems } from '@/lib/api'
import { P } from '@/lib/permissions'
import { riskLevelSeverity, riskStatusSeverity } from '@/lib/risks'
import type { Risk } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ requirementId: string; projectId: string }>()
const { t } = useI18n()
const auth = useAuthStore()
const notify = useNotify()
const canWrite = computed(() => auth.can(P.requirementWrite))
const risks = ref<Risk[]>([])
const all = ref<Risk[]>([])
const failed = ref(false)
const adding = ref(false)
const choice = ref<string | null>(null)
const busy = ref(false)

async function load(id: string) {
  failed.value = false
  try {
    risks.value = asItems(await api.get<{ items: Risk[] }>(`/requirements/${id}/risks`))
  } catch {
    risks.value = []
    failed.value = true
  }
}
watch(() => props.requirementId, load, { immediate: true })

async function startAdd() {
  adding.value = true
  choice.value = null
  try {
    all.value = asItems(await api.get<{ items: Risk[] }>(`/projects/${props.projectId}/risks`))
  } catch (e) {
    notify.fail(e)
  }
}
const options = computed(() => {
  const linked = new Set(risks.value.map((r) => r.id))
  return all.value.filter((r) => !linked.has(r.id)).map((r) => ({ label: `${r.key} ${r.title}`, value: r.id }))
})
async function change(add: string[], remove: string[]) {
  busy.value = true
  try {
    risks.value = asItems(await api.post<{ items: Risk[] }>(`/requirements/${props.requirementId}/risks`, { add, remove }))
    notify.ok(add.length ? t('risksView.mitigates.added') : t('risksView.mitigates.removed'))
    adding.value = false
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div v-if="!failed" class="requirement-risks">
    <div class="section-title">
      <i class="pi pi-shield" aria-hidden="true" /> {{ t('risksView.mitigates.title', { count: risks.length }, risks.length) }}
      <span class="grow" />
      <Button v-if="canWrite && !adding" :label="t('risksView.mitigates.add')" icon="pi pi-plus" size="small" text @click="startAdd" />
      <RouterLink :to="{ name: 'risks', params: { projectId } }" class="small" style="text-transform: none; letter-spacing: 0">{{ t('risksView.openRegister') }}</RouterLink>
    </div>
    <div v-if="adding" class="row add-row">
      <Select
        v-model="choice"
        :options="options"
        option-label="label"
        option-value="value"
        filter
        :placeholder="t('risksView.mitigates.choose')"
        :aria-label="t('risksView.mitigates.choose')"
        :empty-message="t('risksView.mitigates.none')"
        class="grow"
        size="small"
      />
      <Button :label="t('common.add')" size="small" :disabled="!choice" :loading="busy" @click="choice && change([choice], [])" />
      <Button :label="t('common.cancel')" size="small" text severity="secondary" @click="adding = false" />
    </div>
    <div v-if="!risks.length" class="muted small">{{ t('risksView.noRisksForRequirement') }}</div>
    <div v-for="r in risks" :key="r.id" class="row nowrap" style="padding: 5px 0; border-bottom: 1px solid var(--qtf-border)">
      <ItemRef :to="{ name: 'risks', params: { projectId }, query: { risk: r.id } }" :item-key="r.key" :title="r.title" title-class="ellipsis" grow />
      <span class="small muted" :title="t('risksView.score')">{{ r.score }}</span>
      <Tag :value="t(`risksView.level.${r.level}`)" :severity="riskLevelSeverity(r.level)" />
      <Tag :value="t(`risksView.status.${r.status}`)" :severity="riskStatusSeverity(r.status)" />
      <Button
        v-if="canWrite"
        icon="pi pi-times"
        size="small"
        text
        rounded
        severity="secondary"
        :disabled="busy"
        :aria-label="t('risksView.mitigates.remove', { key: r.key })"
        v-tooltip.top="t('risksView.mitigates.remove', { key: r.key })"
        @click="change([], [r.id])"
      />
    </div>
  </div>
</template>

<style scoped>
.add-row { gap: 6px; margin: 4px 0 6px; align-items: center; }
</style>
