<script setup lang="ts">
// Before saving a change of the substance of a requirement or test: lists
// everything the change will make suspect, out of date, outdated or stale,
// and saves only when the user confirms.
import ImpactList from './ImpactList.vue'
import { computed } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import { useI18n } from 'vue-i18n'
import type { ChangeImpact } from '@/lib/correlations'

const props = defineProps<{ impact: ChangeImpact | null; projectId: string; changed: string[]; saving?: boolean }>()
const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ confirm: [] }>()
const { t, te } = useI18n()
const fields = computed(() => props.changed.map((f) => (te(`correlations.field.${f}`) ? t(`correlations.field.${f}`) : f)).join(', '))
</script>

<template>
  <Dialog v-model:visible="visible" :header="t('correlations.confirmTitle', { key: impact?.key ?? '' })" modal style="width: min(640px, 95vw)" class="impact-confirm-dialog">
    <div class="grid" style="gap: 12px">
      <p style="margin: 0">{{ t('correlations.confirmIntro', { fields }) }}</p>
      <p class="muted small" style="margin: 0">{{ t('correlations.confirmCount', { n: impact?.total ?? 0 }, impact?.total ?? 0) }}</p>
      <div class="impact-confirm-list">
        <ImpactList :impact="impact" :project-id="projectId" :heading-level="3" />
      </div>
    </div>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="visible = false" />
      <Button :label="t('correlations.confirmSave')" icon="pi pi-check" :loading="saving" autofocus @click="emit('confirm')" />
    </template>
  </Dialog>
</template>

<style>
.impact-confirm-list {
  max-height: 50vh;
  overflow: auto;
}
</style>
