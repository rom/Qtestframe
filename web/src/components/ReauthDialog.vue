<script setup lang="ts">
// Confirms the user's identity again (POST /auth/reauth) before a
// sensitive change such as adding or deleting a passkey: the password of
// a local account, a code of the authenticator app or a passkey.
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import Password from 'primevue/password'
import MfaCodeInput from './MfaCodeInput.vue'
import { api, errorMessage } from '@/lib/api'
import { normalizeCode } from '@/lib/mfa'
import { getPasskey, passkeyErrorKind, passkeySupport, type RequestOptionsJSON } from '@/lib/passkeys'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ 'update:visible': [boolean]; confirmed: [] }>()
const auth = useAuthStore()
const { t } = useI18n()

const hasPassword = computed(() => (auth.user?.auth_provider ?? 'local') === 'local')
const hasCode = computed(() => !!auth.me?.mfa?.enabled)
const hasPasskey = computed(() => (auth.me?.mfa?.passkeys ?? 0) > 0 && passkeySupport() === 'ok')
const password = ref('')
const code = ref('')
const error = ref('')
const busy = ref(false)
watch(
  () => props.visible,
  (v) => {
    if (v) {
      password.value = ''
      code.value = ''
      error.value = ''
    }
  },
)

function close() {
  emit('update:visible', false)
}
function failed(e: unknown) {
  const kind = passkeyErrorKind(e)
  error.value = kind === 'cancelled' ? t('passkeys.error.cancelled') : e && (e as { code?: string }).code === 'reauth_failed' ? t('passkeys.reauth.failed') : kind ? t(`passkeys.error.${kind}`) : errorMessage(e)
}
async function confirm(body: Record<string, unknown>) {
  busy.value = true
  error.value = ''
  try {
    await api.post('/auth/reauth', body)
    emit('confirmed')
    close()
  } catch (e) {
    failed(e)
  } finally {
    busy.value = false
  }
}
function submit() {
  if (hasPassword.value && password.value) return confirm({ password: password.value })
  if (hasCode.value && code.value.trim()) return confirm({ code: normalizeCode(code.value) })
  error.value = t('passkeys.reauth.failed')
}
async function withPasskey() {
  busy.value = true
  error.value = ''
  try {
    const opts = await api.post<RequestOptionsJSON>('/auth/reauth/options')
    const credential = await getPasskey(opts)
    busy.value = false
    await confirm({ credential })
  } catch (e) {
    failed(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog :visible="visible" modal :header="t('passkeys.reauth.title')" style="width: min(440px, 96vw)" @update:visible="(v: boolean) => !v && close()">
    <form id="reauth-form" class="grid" style="gap: 12px" @submit.prevent="submit">
      <p class="small" style="margin: 0">{{ t('passkeys.reauth.text') }}</p>
      <Message v-if="error" id="reauth-error" severity="error" :closable="false">{{ error }}</Message>
      <div v-if="hasPassword" v-field class="field">
        <label for="reauth-pw">{{ t('passkeys.reauth.password') }}</label>
        <Password v-model="password" input-id="reauth-pw" :feedback="false" toggle-mask fluid autocomplete="current-password" :input-props="{ 'aria-describedby': error ? 'reauth-error' : undefined }" />
      </div>
      <MfaCodeInput v-else-if="hasCode" id="reauth-code" v-model="code" :label="t('passkeys.reauth.code')" />
      <Button v-if="hasPasskey" type="button" :label="t('passkeys.reauth.usePasskey')" icon="pi pi-key" severity="secondary" outlined :loading="busy" @click="withPasskey" />
      <p v-if="!hasPassword && !hasCode && !hasPasskey" class="small muted" style="margin: 0">{{ t('passkeys.reauth.none') }}</p>
    </form>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="close" />
      <Button v-if="hasPassword || hasCode" type="submit" form="reauth-form" :label="t('passkeys.reauth.confirm')" icon="pi pi-check" :loading="busy" :disabled="hasPassword ? !password : !code.trim()" />
    </template>
  </Dialog>
</template>
