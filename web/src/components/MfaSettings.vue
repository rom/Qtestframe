<script setup lang="ts">
// Profile: multi-factor authentication (authenticator app, recovery codes,
// remembered devices).
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { useI18n } from 'vue-i18n'
import MfaEnrol from './MfaEnrol.vue'
import MfaRecoveryCodes from './MfaRecoveryCodes.vue'
import MfaCodeInput from './MfaCodeInput.vue'
import { api, errorMessage } from '@/lib/api'
import { codeComplete, daysLeft, mfaErrorKind, normalizeCode, type MfaStatus } from '@/lib/mfa'
import { formatDate } from '@/lib/format'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'

const auth = useAuthStore()
const notify = useNotify()
const { t } = useI18n()

const status = ref<MfaStatus | null>(null)
const loadError = ref('')
async function load() {
  loadError.value = ''
  try {
    status.value = await api.get<MfaStatus>('/auth/mfa')
  } catch (e) {
    loadError.value = errorMessage(e)
  }
}
onMounted(load)
const days = computed(() => daysLeft(status.value?.enrolment_deadline))
const lowCodes = computed(() => !!status.value?.enabled && status.value.recovery_codes_left <= 3)

// ---------- set up ----------
const enrolOpen = ref(false)
const codes = ref<string[]>([])
function onEnrolled(c: string[]) {
  codes.value = c
  notify.ok(t('mfa.enabledToast'))
  void load()
  void auth.fetchMe()
}
function closeEnrol() {
  enrolOpen.value = false
  codes.value = []
}

// ---------- turn off / new recovery codes (both need a current code) ----------
type Action = 'disable' | 'codes'
const action = ref<Action | null>(null)
const code = ref('')
const useRecovery = ref(false)
const codeError = ref('')
const busy = ref(false)
const newCodes = ref<string[]>([])
function open(a: Action) {
  action.value = a
  code.value = ''
  codeError.value = ''
  useRecovery.value = false
  newCodes.value = []
}
function close() {
  action.value = null
  newCodes.value = []
}
async function submit() {
  codeError.value = ''
  // either kind of code is accepted, whichever field label is shown
  if (!codeComplete(code.value, false) && !codeComplete(code.value, true)) {
    codeError.value = useRecovery.value ? t('mfa.error.recoveryFormat') : t('mfa.error.format')
    return
  }
  busy.value = true
  try {
    if (action.value === 'disable') {
      await api.post('/auth/mfa/disable', { code: normalizeCode(code.value) })
      notify.ok(t('mfa.disabledToast'))
      close()
      void auth.fetchMe()
    } else {
      const res = await api.post<{ recovery_codes: string[] }>('/auth/mfa/recovery-codes', { code: normalizeCode(code.value) })
      newCodes.value = res?.recovery_codes ?? []
    }
    await load()
  } catch (e) {
    const kind = mfaErrorKind(e)
    codeError.value = kind ? t(`mfa.error.${kind}`) : errorMessage(e)
  } finally {
    busy.value = false
  }
}

// ---------- remembered devices ----------
const forgetting = ref(false)
async function forgetDevices() {
  forgetting.value = true
  try {
    const res = await api.del<{ forgotten: number }>('/auth/mfa/trusted-devices')
    notify.ok(t('mfa.devicesForgotten', { n: res?.forgotten ?? 0 }, res?.forgotten ?? 0))
    await load()
  } catch (e) {
    notify.fail(e)
  } finally {
    forgetting.value = false
  }
}
</script>

<template>
  <section id="mfa" class="card" aria-labelledby="h-mfa" style="grid-column: 1 / -1">
    <div class="row" style="margin-bottom: 10px">
      <h2 id="h-mfa" style="margin: 0"><i class="pi pi-shield" aria-hidden="true" /> {{ t('mfa.title') }}</h2>
      <Tag v-if="status" :value="status.enabled ? t('mfa.on') : t('mfa.off')" :severity="status.enabled ? 'success' : 'secondary'" :icon="status.enabled ? 'pi pi-check' : undefined" />
    </div>
    <Message v-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>
    <template v-else-if="status">
      <p class="small muted" style="margin: 0 0 12px">{{ t('mfa.intro') }}</p>
      <Message v-if="status.required && !status.enabled" severity="warn" :closable="false" style="margin-bottom: 12px">
        {{ days ? t('mfa.requiredBy', { date: formatDate(status.enrolment_deadline), n: days }, days) : t('mfa.requiredNow') }}
      </Message>
      <div v-if="!status.enabled">
        <Button :label="t('mfa.setUp')" icon="pi pi-mobile" @click="enrolOpen = true" />
      </div>
      <template v-else>
        <div class="kv">
          <span class="small muted">{{ t('mfa.method') }}</span><span>{{ t('mfa.authenticatorApp') }}</span>
          <span class="small muted">{{ t('mfa.since') }}</span><span>{{ formatDate(status.enabled_at) }}</span>
          <span class="small muted">{{ t('mfa.recoveryCodes') }}</span>
          <span :class="{ 'error-text': lowCodes }">{{ t('mfa.codesLeft', { left: status.recovery_codes_left, total: status.recovery_codes_total }) }}</span>
          <template v-if="status.remember_days > 0 || status.trusted_devices > 0">
            <span class="small muted">{{ t('mfa.rememberedDevices') }}</span><span>{{ t('mfa.devices', { n: status.trusted_devices }, status.trusted_devices) }}</span>
          </template>
        </div>
        <Message v-if="lowCodes" severity="warn" :closable="false" style="margin: 10px 0">{{ t('mfa.codesLow') }}</Message>
        <div class="row" style="gap: 8px; margin-top: 12px">
          <Button :label="t('mfa.newCodes')" icon="pi pi-refresh" severity="secondary" outlined @click="open('codes')" />
          <Button v-if="status.trusted_devices > 0" :label="t('mfa.forgetDevices')" icon="pi pi-times" severity="secondary" outlined :loading="forgetting" @click="forgetDevices" />
          <Button :label="t('mfa.turnOff')" icon="pi pi-power-off" severity="danger" outlined :disabled="status.required" @click="open('disable')" />
        </div>
        <p v-if="status.required" class="small muted" style="margin: 8px 0 0">{{ t('mfa.cannotTurnOff') }}</p>
      </template>
    </template>

    <Dialog :visible="enrolOpen" modal :header="codes.length ? t('mfa.recoveryCodes') : t('mfa.setUp')" style="width: min(640px, 96vw)" @update:visible="(v: boolean) => !v && closeEnrol()">
      <MfaRecoveryCodes v-if="codes.length" :codes="codes" :account="auth.user?.email ?? ''" />
      <MfaEnrol v-else-if="enrolOpen" @enrolled="onEnrolled" />
      <template v-if="codes.length" #footer>
        <Button :label="t('mfa.savedCodes')" icon="pi pi-check" @click="closeEnrol" />
      </template>
    </Dialog>

    <Dialog :visible="!!action" modal :header="action === 'disable' ? t('mfa.turnOffTitle') : t('mfa.newCodesTitle')" style="width: min(480px, 96vw)" @update:visible="(v: boolean) => !v && close()">
      <MfaRecoveryCodes v-if="newCodes.length" :codes="newCodes" :account="auth.user?.email ?? ''" />
      <form v-else id="mfa-reauth" class="grid" style="gap: 12px" @submit.prevent="submit">
        <p class="small" style="margin: 0">{{ action === 'disable' ? t('mfa.turnOffText') : t('mfa.newCodesText') }}</p>
        <MfaCodeInput id="mfa-reauth-code" v-model="code" :recovery="useRecovery" :error="codeError" autofocus />
        <button type="button" class="link-button small" style="justify-self: start; color: var(--p-primary-color)" @click="useRecovery = !useRecovery">
          {{ useRecovery ? t('mfa.useApp') : t('mfa.useRecovery') }}
        </button>
      </form>
      <template #footer>
        <template v-if="newCodes.length"><Button :label="t('mfa.savedCodes')" icon="pi pi-check" @click="close" /></template>
        <template v-else>
          <Button :label="t('common.cancel')" text severity="secondary" @click="close" />
          <Button type="submit" form="mfa-reauth" :label="action === 'disable' ? t('mfa.turnOff') : t('mfa.newCodes')" :severity="action === 'disable' ? 'danger' : undefined" :loading="busy" />
        </template>
      </template>
    </Dialog>
  </section>
</template>
