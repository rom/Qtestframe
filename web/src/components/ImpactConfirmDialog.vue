<script setup lang="ts">
// Before saving a change (an item, a bulk change, an import, a baseline
// restore): the impact of the change as a tree grouped by type and depth
// with counts, and what to do with it: mark suspect (the project's rules),
// notify the owners only, or skip. Saves only when the user confirms.
import ImpactTree from './ImpactTree.vue'
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import RadioButton from 'primevue/radiobutton'
import { useI18n } from 'vue-i18n'
import { depthCounts, IMPACT_CHOICES, impactSections, type ChangeImpact, type ImpactChoice } from '@/lib/correlations'

const props = defineProps<{ impact: ChangeImpact | null; projectId: string; changed: string[]; saving?: boolean }>()
const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ confirm: [choice: ImpactChoice]; cancel: [] }>()
const { t, te } = useI18n()
const choice = ref<ImpactChoice>('suspect')
watch(visible, (v) => {
  if (v) choice.value = 'suspect'
})
const fields = computed(() => props.changed.map((f) => (te(`correlations.field.${f}`) ? t(`correlations.field.${f}`) : f)).join(', '))
const roots = computed(() => props.impact?.roots ?? [])
const statusOnly = computed(() => roots.value.length > 0 && roots.value.every((r) => r.review_only))
const header = computed(() =>
  roots.value.length > 1 ? t('correlations.confirmTitleMany', { n: roots.value.length }, roots.value.length) : t('correlations.confirmTitle', { key: props.impact?.key || roots.value[0]?.key || '' }),
)
const kinds = computed(() => impactSections(props.impact).map((s) => ({ section: s.section, n: s.items.length })))
const depths = computed(() => depthCounts(props.impact))
function close() {
  visible.value = false
  emit('cancel')
}
</script>

<template>
  <Dialog
    :visible="visible"
    :header="header"
    modal
    style="width: min(720px, 95vw)"
    class="impact-confirm-dialog"
    @update:visible="(v: boolean) => (v ? (visible = true) : close())"
  >
    <div class="grid" style="gap: 12px">
      <p v-if="changed.length" style="margin: 0">{{ t('correlations.confirmIntro', { fields }) }}</p>
      <p v-else style="margin: 0">{{ t('correlations.confirmIntroMany', { depth: impact?.depth ?? 1 }) }}</p>
      <p v-if="roots.length > 1" class="small muted" style="margin: 0">{{ t('correlations.changedItems', { keys: roots.map((r) => r.key).join(', ') }) }}</p>
      <p v-if="statusOnly" class="small muted" style="margin: 0">{{ t('correlations.confirmIntroStatus') }}</p>
      <p class="small" style="margin: 0">
        <strong>{{ t('correlations.totalAffected', { n: impact?.total ?? 0 }, impact?.total ?? 0) }}</strong> —
        {{ t('correlations.confirmSummary', { suspect: impact?.suspect ?? 0, review: impact?.review ?? 0 }) }}
      </p>
      <div class="impact-counts small">
        <div>
          <span class="muted">{{ t('correlations.byKind') }}:</span>
          <span v-for="k in kinds" :key="k.section" class="impact-chip">{{ t(`correlations.section.${k.section}`) }} <strong>{{ k.n }}</strong></span>
        </div>
        <div>
          <span class="muted">{{ t('correlations.byDepth') }}:</span>
          <span v-for="d in depths" :key="d.depth" class="impact-chip">{{ t('correlations.depth', { n: d.depth }) }} <strong>{{ d.count }}</strong></span>
        </div>
      </div>
      <div class="impact-confirm-list">
        <ImpactTree :impact="impact" :project-id="projectId" :heading-level="3" />
      </div>
      <fieldset class="impact-choice">
        <legend class="field-legend">{{ t('correlations.choiceLegend') }}</legend>
        <div v-for="c in IMPACT_CHOICES" :key="c" class="impact-choice-row">
          <RadioButton v-model="choice" :input-id="`impact-choice-${c}`" name="impact-choice" :value="c" />
          <label :for="`impact-choice-${c}`">
            <span>{{ t(`correlations.choice.${c}`) }}</span>
            <span class="small muted" style="display: block">{{ t(`correlations.choiceHint.${c}`) }}</span>
          </label>
        </div>
      </fieldset>
    </div>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="close" />
      <Button :label="t('correlations.confirmApply')" icon="pi pi-check" :loading="saving" autofocus @click="emit('confirm', choice)" />
    </template>
  </Dialog>
</template>

<style>
.impact-confirm-list {
  max-height: 42vh;
  overflow: auto;
}
.impact-counts {
  display: grid;
  gap: 4px;
}
.impact-chip {
  display: inline-block;
  margin: 0 0 0 6px;
  padding: 0 6px;
  border: 1px solid var(--qtf-border);
  border-radius: 99px;
}
.impact-choice {
  border: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 6px;
}
.impact-choice-row {
  display: flex;
  gap: 8px;
  align-items: flex-start;
}
</style>
