<script setup lang="ts">
// Administration → AI assistance: the organisation's switch (ai_enabled of
// GET/PUT /settings, settings.manage) with the state of the server's
// service (GET /ai), the features that are on and what is sent where.
import { computed, onMounted, ref } from 'vue'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import ToggleSwitch from 'primevue/toggleswitch'
import Tag from 'primevue/tag'
import { api, errorMessage } from '@/lib/api'
import { AI_FEATURES, clearAiStatus, type AiStatus } from '@/lib/ai'
import { P } from '@/lib/permissions'
import type { TenantSettings } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const auth = useAuthStore()
const notify = useNotify()
const settings = ref<TenantSettings | null>(null)
const status = ref<AiStatus | null>(null)
const error = ref('')
const busy = ref(false)
const canEdit = computed(() => auth.can(P.settingsManage))

async function load() {
  error.value = ''
  try {
    const [s, st] = await Promise.all([api.get<TenantSettings>('/settings'), api.get<AiStatus>('/ai')])
    settings.value = s
    status.value = st
  } catch (e) {
    error.value = errorMessage(e)
  }
}
onMounted(load)

async function toggle(on: boolean) {
  if (!settings.value) return
  busy.value = true
  try {
    settings.value = await api.put<TenantSettings>('/settings', { ...settings.value, ai_enabled: on })
    status.value = await api.get<AiStatus>('/ai')
    clearAiStatus()
    notify.ok(t('ai.org.saved'))
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = false
  }
}

const serverState = computed(() => {
  const s = status.value
  if (!s) return ''
  if (!s.system_enabled) return t('ai.org.serverOff')
  if (s.reason === 'not_configured') return t('ai.reason.not_configured')
  return t('ai.org.serverOn', { provider: t(`ai.provider.${s.provider ?? 'anthropic'}`), model: s.model ?? '' })
})
</script>

<template>
  <div class="card" style="max-width: 820px">
    <h2 class="plain-heading" style="margin: 0"><i class="pi pi-sparkles" aria-hidden="true" /> {{ t('ai.org.title') }}</h2>
    <p class="small muted" style="margin-top: 4px">{{ t('ai.org.intro') }}</p>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div v-else-if="!settings || !status" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
    <div v-else class="grid" style="gap: 16px">
      <Message v-if="!canEdit" severity="info" :closable="false">{{ t('execSettings.readOnly') }}</Message>
      <section aria-labelledby="ai-server-h">
        <h3 id="ai-server-h" class="plain-heading" style="margin: 0 0 4px; font-size: 15px">{{ t('ai.org.server') }}</h3>
        <p class="small" style="margin: 0">{{ serverState }}</p>
        <ul v-if="status.system_enabled && status.reason !== 'not_configured'" class="plain-list row" style="margin-top: 8px" :aria-label="t('ai.org.features')">
          <li v-for="f in AI_FEATURES" :key="f">
            <Tag :value="t(`ai.action.${f}`)" :severity="status.features[f] ? 'success' : 'secondary'" :icon="status.features[f] ? 'pi pi-check' : 'pi pi-minus'" />
            <span class="sr-only">: {{ status.features[f] ? t('ai.org.featureOn') : t('ai.org.featureOff') }}</span>
          </li>
        </ul>
        <p v-if="status.system_enabled && status.rate_limit" class="small muted" style="margin: 6px 0 0">{{ t('ai.org.rateLimit', { n: status.rate_limit }) }}</p>
      </section>
      <div class="row nowrap" style="gap: 8px; align-items: flex-start">
        <ToggleSwitch :model-value="!!settings.ai_enabled" input-id="ai-org-on" :disabled="!canEdit || busy" aria-describedby="ai-org-hint" @update:model-value="toggle" />
        <div>
          <label for="ai-org-on"><b>{{ t('ai.org.label') }}</b></label>
          <p id="ai-org-hint" class="small muted" style="margin: 2px 0 0">{{ t('ai.org.hint') }}</p>
        </div>
      </div>
      <Message severity="warn" :closable="false">{{ t('ai.org.privacy') }}</Message>
    </div>
  </div>
</template>
