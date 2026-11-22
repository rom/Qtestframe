<script setup lang="ts">
// The "Assist" button of an editor or page: a popover with the AI actions
// that fit the place. When the assistance is not available (not set up, off
// for the organisation or the project, a feature turned off, nothing to work
// on) the actions stay visible, disabled, with the reason. Choosing one
// emits select; the AssistDialog runs it.
import { computed, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Popover from 'primevue/popover'
import { api } from '@/lib/api'
import { aiStatus, featureState, type AiFeature, type AiStatus } from '@/lib/ai'

export interface AssistAction {
  feature: AiFeature
  /** button text (default ai.action.<feature>) */
  label?: string
  /** message id why the action cannot be used here ("" none) */
  blocked?: string
}

const props = defineProps<{ projectId: string; actions: AssistAction[]; size?: 'small' }>()
const emit = defineEmits<{ select: [feature: AiFeature] }>()
const { t } = useI18n()
const pop = ref<InstanceType<typeof Popover>>()
const status = ref<AiStatus | null>(null)
const failed = ref(false)
const panelId = useId()

async function load(force = false) {
  failed.value = false
  try {
    status.value = await aiStatus(props.projectId, (p) => api.get<AiStatus>(p), force)
  } catch {
    failed.value = true
  }
}
watch(() => props.projectId, () => load(), { immediate: true })

const rows = computed(() =>
  props.actions.map((a) => {
    const st = featureState(status.value, a.feature)
    const reason = !st.ok ? st.reason : a.blocked || (status.value && status.value.remaining <= 0 ? 'ai.reason.rate_limited' : '')
    return { ...a, text: a.label ?? t(`ai.action.${a.feature}`), reason, id: `${panelId}-${a.feature}` }
  }),
)
const unavailable = computed(() => (status.value && !status.value.available ? t(`ai.reason.${status.value.reason ?? 'off'}`) : ''))

function open(e: Event) {
  void load(true)
  pop.value?.toggle(e)
}

function choose(f: AiFeature) {
  pop.value?.hide()
  emit('select', f)
}
</script>

<template>
  <Button
    type="button"
    :label="t('ai.assist')"
    icon="pi pi-sparkles"
    severity="secondary"
    outlined
    :size="size"
    aria-haspopup="dialog"
    :aria-controls="panelId"
    :title="unavailable || t('ai.assistTitle')"
    @click="open"
  />
  <Popover ref="pop" :aria-label="t('ai.menuTitle')">
    <div :id="panelId" class="assist-menu">
      <b class="assist-title"><i class="pi pi-sparkles" aria-hidden="true" /> {{ t('ai.menuTitle') }}</b>
      <p v-if="failed" class="small">{{ t('ai.statusFailed') }}</p>
      <p v-else-if="unavailable" class="small assist-reason">{{ unavailable }}</p>
      <ul class="plain-list assist-actions">
        <li v-for="r in rows" :key="r.feature">
          <Button
            type="button"
            :label="r.text"
            text
            size="small"
            class="assist-action"
            :disabled="!!r.reason || failed"
            :aria-describedby="r.reason && !unavailable ? `${r.id}-why` : undefined"
            @click="choose(r.feature)"
          />
          <span v-if="r.reason && !unavailable" :id="`${r.id}-why`" class="small muted assist-why">{{ t(r.reason) }}</span>
        </li>
      </ul>
      <p v-if="status?.available" class="small muted assist-notice">
        <i class="pi pi-info-circle" aria-hidden="true" />
        {{ t('ai.dataNotice', { provider: t(`ai.provider.${status.provider ?? 'anthropic'}`) }) }}
        <span v-if="status.rate_limit"> {{ t('ai.remaining', { n: status.remaining, max: status.rate_limit }) }}</span>
      </p>
    </div>
  </Popover>
</template>

<style scoped>
.assist-menu {
  display: grid;
  gap: 6px;
  max-width: 340px;
}
.assist-title {
  display: flex;
  gap: 6px;
  align-items: center;
}
.assist-actions {
  display: grid;
  gap: 2px;
}
.assist-action {
  justify-content: flex-start;
  width: 100%;
}
.assist-why {
  display: block;
  padding: 0 10px 4px;
}
.assist-notice,
.assist-reason {
  margin: 0;
}
</style>
