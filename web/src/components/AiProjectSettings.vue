<script setup lang="ts">
// A project's opt-out of the AI assistance (PUT /projects/{pid}/ai), with
// the state of the server and the organisation switches.
import { computed, ref, watch } from 'vue'
import ToggleSwitch from 'primevue/toggleswitch'
import { useI18n } from 'vue-i18n'
import { api } from '@/lib/api'
import { uid } from '@/lib/a11y'
import { clearAiStatus, type AiStatus } from '@/lib/ai'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ projectId: string; canEdit: boolean }>()
const { t } = useI18n()
const notify = useNotify()
const id = uid('aiset')
const status = ref<AiStatus | null>(null)
const busy = ref(false)

watch(
  () => props.projectId,
  async (pid) => {
    try {
      status.value = await api.get<AiStatus>(`/projects/${pid}/ai`)
    } catch {
      status.value = null
    }
  },
  { immediate: true },
)

/** why the assistance is not available apart from the project ("" none) */
const upstream = computed(() => {
  const s = status.value
  if (!s) return ''
  if (!s.system_enabled) return t('ai.reason.off')
  if (s.reason === 'not_configured') return t('ai.reason.not_configured')
  if (!s.organisation_enabled) return t('ai.reason.organisation')
  return ''
})

async function toggle(on: boolean) {
  busy.value = true
  try {
    status.value = await api.put<AiStatus>(`/projects/${props.projectId}/ai`, { disabled: !on })
    clearAiStatus(props.projectId)
    notify.ok(t('ai.project.saved'))
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section v-if="status" class="card" style="max-width: 640px; margin-top: 16px" :aria-labelledby="`${id}-h`">
    <h2 :id="`${id}-h`" class="plain-heading" style="margin-bottom: 6px"><i class="pi pi-sparkles" aria-hidden="true" /> {{ t('ai.project.title') }}</h2>
    <div class="row nowrap" style="gap: 8px; align-items: flex-start">
      <ToggleSwitch :model-value="!status.project_disabled" :input-id="`${id}-on`" :disabled="!canEdit || busy" :aria-describedby="`${id}-hint`" @update:model-value="toggle" />
      <div>
        <label :for="`${id}-on`"><b>{{ t('ai.project.label') }}</b></label>
        <p :id="`${id}-hint`" class="small muted" style="margin: 2px 0 0">{{ t('ai.project.hint') }}</p>
        <p v-if="upstream" class="small" style="margin: 4px 0 0"><i class="pi pi-info-circle" aria-hidden="true" /> {{ upstream }}</p>
        <p v-else-if="status.available" class="small muted" style="margin: 4px 0 0">{{ t('ai.project.using', { provider: t(`ai.provider.${status.provider ?? 'anthropic'}`), model: status.model ?? '' }) }}</p>
      </div>
    </div>
  </section>
</template>
