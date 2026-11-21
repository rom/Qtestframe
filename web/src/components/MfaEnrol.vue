<script setup lang="ts">
// Setting up an authenticator app: QR code (or the key to type in), then a
// code from the app to confirm. Emits the recovery codes once confirmed.
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import { useI18n } from 'vue-i18n'
import MfaCodeInput from './MfaCodeInput.vue'
import { api, errorMessage } from '@/lib/api'
import { groupSecret, isTotpCode, mfaErrorKind, normalizeCode, qrModules, qrPath, type TotpSetup } from '@/lib/mfa'
import { useNotify } from '@/composables/notify'

const emit = defineEmits<{ enrolled: [codes: string[]] }>()
const { t } = useI18n()
const notify = useNotify()

const setup = ref<TotpSetup | null>(null)
const loadError = ref('')
const code = ref('')
const error = ref('')
const busy = ref(false)
const showKey = ref(false)

const qr = computed(() => {
  if (!setup.value) return null
  const m = qrModules(setup.value.otpauth_uri)
  return { size: m.length + 8, path: qrPath(m, 4) }
})
const grouped = computed(() => groupSecret(setup.value?.secret ?? ''))

async function start() {
  loadError.value = ''
  try {
    setup.value = await api.post<TotpSetup>('/auth/mfa/totp')
  } catch (e) {
    loadError.value = errorMessage(e)
  }
}
onMounted(start)

async function confirm() {
  error.value = ''
  if (!isTotpCode(code.value)) {
    error.value = t('mfa.error.format')
    return
  }
  busy.value = true
  try {
    const res = await api.post<{ recovery_codes: string[] }>('/auth/mfa/totp/confirm', { code: normalizeCode(code.value) })
    emit('enrolled', res?.recovery_codes ?? [])
  } catch (e) {
    const kind = mfaErrorKind(e)
    error.value = kind === 'invalid' ? t('mfa.error.confirm') : kind ? t(`mfa.error.${kind}`) : errorMessage(e)
  } finally {
    busy.value = false
  }
}
async function copyKey() {
  try {
    await navigator.clipboard.writeText(setup.value?.secret ?? '')
    notify.ok(t('mfa.keyCopied'))
  } catch (e) {
    notify.fail(e)
  }
}
</script>

<template>
  <div class="mfa-enrol">
    <Message v-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>
    <div v-else-if="!setup" class="muted small" role="status">{{ t('common.loading') }}</div>
    <template v-else>
      <ol class="steps">
        <li>
          <p>{{ t('mfa.step1') }}</p>
        </li>
        <li>
          <p>{{ t('mfa.step2') }}</p>
          <div class="qr-row">
            <svg v-if="qr" class="qr" :viewBox="`0 0 ${qr.size} ${qr.size}`" role="img" :aria-label="t('mfa.qrLabel', { account: setup.account })" shape-rendering="crispEdges">
              <rect width="100%" height="100%" fill="#fff" />
              <path :d="qr.path" fill="#000" />
            </svg>
            <div class="qr-side">
              <a :href="setup.otpauth_uri" class="small">{{ t('mfa.openApp') }}</a>
              <button type="button" class="link-button small" :aria-expanded="showKey" aria-controls="mfa-manual-key" @click="showKey = !showKey">{{ t('mfa.cannotScan') }}</button>
              <div v-if="showKey" id="mfa-manual-key" class="manual">
                <span class="small muted">{{ t('mfa.manualKey') }}</span>
                <code class="secret" :aria-label="t('mfa.manualKey')">{{ grouped }}</code>
                <Button :label="t('common.copy')" icon="pi pi-copy" size="small" severity="secondary" outlined @click="copyKey" />
                <span class="small muted">{{ t('mfa.manualDetails', { account: setup.account, issuer: setup.issuer, digits: setup.digits, period: setup.period }) }}</span>
              </div>
            </div>
          </div>
        </li>
        <li>
          <p>{{ t('mfa.step3') }}</p>
          <form class="row confirm" style="align-items: flex-start; gap: 10px" @submit.prevent="confirm">
            <MfaCodeInput id="mfa-enrol-code" v-model="code" :error="error" />
            <Button type="submit" :label="t('mfa.activate')" icon="pi pi-check" :loading="busy" style="margin-top: 22px" />
          </form>
        </li>
      </ol>
    </template>
  </div>
</template>

<style scoped>
.steps {
  margin: 0;
  padding-left: 1.4em;
  display: grid;
  gap: 10px;
}
.steps p {
  margin: 0 0 8px;
}
.qr-row {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  align-items: flex-start;
}
.qr {
  width: 200px;
  height: 200px;
  flex: none;
  border-radius: 6px;
  border: 1px solid var(--qtf-border);
}
.qr-side {
  display: grid;
  gap: 8px;
  min-width: 0;
  flex: 1 1 200px;
  justify-items: start;
}
.manual {
  overflow-wrap: anywhere;
  display: grid;
  gap: 6px;
  justify-items: start;
  min-width: 0;
}
.secret {
  font-size: 1.05em;
  word-break: break-all;
  padding: 4px 8px;
  border-radius: 6px;
  background: color-mix(in srgb, var(--p-primary-color) 8%, transparent);
}
.link-button {
  color: var(--p-primary-color);
}
</style>
