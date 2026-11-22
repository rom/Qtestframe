<script setup lang="ts">
// Map a trace link to the criteria the test verifies, each optionally
// through one step of the test.
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Select from 'primevue/select'
import Message from 'primevue/message'
import { useI18n } from 'vue-i18n'
import { useEscapeClose } from '@/composables/escapeClose'
import { api, errorMessage } from '@/lib/api'
import { uid } from '@/lib/a11y'
import type { EvaluationCriterion, TestCase, TraceLink } from '@/lib/types'

const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ link: TraceLink | null; criteria: EvaluationCriterion[]; testKey?: string }>()
const emit = defineEmits<{ saved: [link: TraceLink] }>()
const { t } = useI18n()
useEscapeClose(visible)
const id = uid('lcrit')
const chosen = ref<Record<string, boolean>>({})
const steps = ref<Record<string, string>>({})
const test = ref<TestCase | null>(null)
const error = ref('')
const saving = ref(false)

watch(visible, async (v) => {
  if (!v || !props.link) return
  error.value = ''
  chosen.value = {}
  steps.value = {}
  for (const c of props.link.criteria ?? []) {
    chosen.value[c.criterion_id] = true
    steps.value[c.criterion_id] = c.step_id ?? ''
  }
  try {
    test.value = await api.get<TestCase>(`/tests/${props.link.test_case_id}`)
  } catch {
    test.value = null
  }
})
const stepOptions = computed(() => [
  { label: t('verification.wholeTest'), value: '' },
  ...(test.value?.steps ?? []).map((s, i) => ({ label: t('verification.stepOption', { n: i + 1, action: s.action || s.expected || '…' }), value: s.id })),
])

async function save() {
  if (!props.link) return
  saving.value = true
  error.value = ''
  const criteria = props.criteria.filter((c) => chosen.value[c.id]).map((c) => ({ criterion_id: c.id, step_id: steps.value[c.id] || undefined }))
  try {
    emit('saved', await api.put<TraceLink>(`/links/${props.link.id}/criteria`, { criteria }))
    visible.value = false
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog
    v-model:visible="visible"
    :close-on-escape="false"
    modal
    :header="t('verification.mapCriteriaOf', { key: testKey || link?.test_case_key || '' })"
    :style="{ width: 'min(620px, calc(100vw - 16px))' }"
    :breakpoints="{ '640px': '100vw' }"
  >
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <p class="small muted" style="margin-top: 0">{{ t('verification.mapCriteriaHint') }}</p>
    <fieldset class="plain-fieldset">
      <legend class="sr-only">{{ t('verification.criteria') }}</legend>
      <div v-for="(c, i) in criteria" :key="c.id" class="map-row">
        <span class="row nowrap" style="gap: 8px; align-items: flex-start">
          <Checkbox v-model="chosen[c.id]" binary :input-id="`${id}-${c.id}`" />
          <label :for="`${id}-${c.id}`">{{ i + 1 }}. {{ c.description }}</label>
        </span>
        <Select
          v-if="chosen[c.id] && stepOptions.length > 1"
          v-model="steps[c.id]"
          :options="stepOptions"
          option-label="label"
          option-value="value"
          size="small"
          :aria-label="t('verification.stepFor', { n: i + 1 })"
          class="map-step"
        />
      </div>
    </fieldset>
    <div class="row" style="justify-content: flex-end; gap: 8px; margin-top: 12px">
      <Button :label="t('common.cancel')" severity="secondary" text @click="visible = false" />
      <Button :label="t('common.save')" icon="pi pi-check" :loading="saving" @click="save" />
    </div>
  </Dialog>
</template>

<style scoped>
.plain-fieldset {
  border: 0;
  margin: 0;
  padding: 0;
}
.map-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 12px;
  align-items: center;
  justify-content: space-between;
  padding: 6px 0;
  border-bottom: 1px solid var(--qtf-border);
}
.map-step {
  min-width: 0;
  max-width: 100%;
  width: 260px;
}
</style>
