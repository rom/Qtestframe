<script setup lang="ts">
// The project's verification settings: strict criterion coverage, and
// whether debriefed exploratory sessions count as evidence.
import { ref, watch } from 'vue'
import ToggleSwitch from 'primevue/toggleswitch'
import { useI18n } from 'vue-i18n'
import { api } from '@/lib/api'
import { uid } from '@/lib/a11y'
import { formatDateTime } from '@/lib/format'
import type { VerificationSettings } from '@/lib/types'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ projectId: string; canEdit: boolean }>()
const { t } = useI18n()
const notify = useNotify()
const id = uid('vset')
const settings = ref<VerificationSettings | null>(null)
const busy = ref(false)

watch(
  () => props.projectId,
  async (pid) => {
    try {
      settings.value = await api.get<VerificationSettings>(`/projects/${pid}/verification-settings`)
    } catch {
      settings.value = null
    }
  },
  { immediate: true },
)
async function toggle(v: boolean, field: 'strict_criteria' | 'exploratory_evidence' = 'strict_criteria') {
  busy.value = true
  try {
    settings.value = await api.put<VerificationSettings>(`/projects/${props.projectId}/verification-settings`, { [field]: v })
    notify.ok(t('verification.settingsSaved'))
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section v-if="settings" class="card" style="max-width: 640px; margin-top: 16px" :aria-labelledby="`${id}-h`">
    <h2 :id="`${id}-h`" class="plain-heading" style="margin-bottom: 6px">{{ t('verification.settingsTitle') }}</h2>
    <div class="row nowrap" style="gap: 8px; align-items: flex-start">
      <ToggleSwitch :model-value="settings.strict_criteria" :input-id="`${id}-strict`" :disabled="!canEdit || busy" :aria-describedby="`${id}-hint`" @update:model-value="toggle" />
      <div>
        <label :for="`${id}-strict`"><b>{{ t('verification.strictLabel') }}</b></label>
        <p :id="`${id}-hint`" class="small muted" style="margin: 2px 0 0">{{ t('verification.strictHint') }}</p>
      </div>
    </div>
    <div class="row nowrap" style="gap: 8px; align-items: flex-start; margin-top: 10px">
      <ToggleSwitch :model-value="!!settings.exploratory_evidence" :input-id="`${id}-expl`" :disabled="!canEdit || busy" :aria-describedby="`${id}-expl-hint`" @update:model-value="(v: boolean) => toggle(v, 'exploratory_evidence')" />
      <div>
        <label :for="`${id}-expl`"><b>{{ t('exploratory.evidenceLabel') }}</b></label>
        <p :id="`${id}-expl-hint`" class="small muted" style="margin: 2px 0 0">{{ t('exploratory.evidenceHint') }}</p>
        <p v-if="settings.updated_by" class="small muted" style="margin: 2px 0 0">{{ t('verification.settingsChanged', { user: settings.updated_by, date: formatDateTime(settings.updated_at ?? '') }) }}</p>
      </div>
    </div>
  </section>
</template>
