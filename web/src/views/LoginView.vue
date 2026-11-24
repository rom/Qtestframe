<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Divider from 'primevue/divider'
import Checkbox from 'primevue/checkbox'
import MfaCodeInput from '@/components/MfaCodeInput.vue'
import { codeComplete, isChallenge, mfaErrorKind, normalizeCode, type MfaChallenge } from '@/lib/mfa'
import { api, errorMessage } from '@/lib/api'
import { conditionalAvailable, getPasskey, passkeyErrorKind, passkeySupport, type RequestOptionsJSON } from '@/lib/passkeys'
import type { AuthProvider, Providers } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from 'vue-i18n'
import { chooseLocal, LANGUAGES, LANGUAGE_NAMES, type Lang } from '@/i18n'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()
const { t, locale } = useI18n()
/** Language switch before sign-in: stored on this device (qtf.lang). */
function setLang(l: Lang) {
  chooseLocal(l)
}

const email = ref('')
const password = ref('')
const tenant = ref((route.query.tenant as string) || '')
const showTenant = ref(!!tenant.value)
const error = ref('')
const busy = ref(false)
const providers = ref<Providers>({ local: true, oidc: [], saml: [] })
const sso = ref<(AuthProvider & { kind: string })[]>([])

// ---------- passkeys ----------
const support = passkeySupport()
const passkeyBusy = ref(false)
let conditional: AbortController | null = null
function passkeyError(e: unknown): string {
  const kind = passkeyErrorKind(e)
  return kind ? t(`passkeys.error.${kind}`) : errorMessage(e)
}
function stopConditional() {
  conditional?.abort()
  conditional = null
}
/** Passwordless sign-in: modal (button) or as suggestions of the e-mail field (conditional). */
async function passkeySignIn(mode: 'modal' | 'conditional') {
  if (support !== 'ok') {
    if (mode === 'modal') error.value = t(`passkeys.error.${support}`)
    return
  }
  stopConditional()
  const ctl = new AbortController()
  if (mode === 'conditional') conditional = ctl
  else {
    error.value = ''
    passkeyBusy.value = true
  }
  try {
    const opts = await api.post<RequestOptionsJSON>('/auth/passkeys/login/options', undefined, { noAuthRedirect: true })
    const credential = await getPasskey(opts, { conditional: mode === 'conditional', signal: ctl.signal })
    passkeyBusy.value = true
    await auth.loginPasskey(credential, tenant.value.trim())
    done()
  } catch (e) {
    // a cancelled or replaced suggestion request is no error
    if (!(mode === 'conditional' && (ctl.signal.aborted || passkeyErrorKind(e) === 'cancelled'))) error.value = passkeyError(e)
  } finally {
    if (conditional === ctl) conditional = null
    passkeyBusy.value = false
  }
}
onBeforeUnmount(stopConditional)

// ---------- second factor ----------
const challenge = ref<MfaChallenge | null>(null)
const code = ref('')
const useRecovery = ref(false)
const remember = ref(false)
const codeError = ref('')
const codeInput = ref<InstanceType<typeof MfaCodeInput> | null>(null)
async function toSecondFactor(ch: MfaChallenge) {
  challenge.value = ch
  code.value = ''
  codeError.value = ''
  useRecovery.value = false
  await nextTick()
  codeInput.value?.focus()
}
function mfaError(e: unknown): string {
  const kind = mfaErrorKind(e)
  return kind ? t(`mfa.error.${kind}`) : errorMessage(e)
}
async function submitCode() {
  codeError.value = ''
  if (!codeComplete(code.value, useRecovery.value)) {
    codeError.value = useRecovery.value ? t('mfa.error.recoveryFormat') : t('mfa.error.format')
    return
  }
  busy.value = true
  try {
    await auth.verifyMfa(normalizeCode(code.value), remember.value)
    done()
  } catch (e) {
    if (mfaErrorKind(e) === 'expired') {
      // start again with the password
      challenge.value = null
      password.value = ''
      error.value = mfaError(e)
      return
    }
    codeError.value = mfaError(e)
    code.value = ''
    codeInput.value?.focus()
  } finally {
    busy.value = false
  }
}
async function switchMethod() {
  useRecovery.value = !useRecovery.value
  code.value = ''
  codeError.value = ''
  await nextTick()
  codeInput.value?.focus()
}
const hasCode = () => !!challenge.value?.methods.includes('totp')
const hasPasskey = () => !!challenge.value?.methods.includes('passkey')
async function passkeySecondFactor() {
  codeError.value = ''
  if (support !== 'ok') {
    codeError.value = t(`passkeys.error.${support}`)
    return
  }
  passkeyBusy.value = true
  try {
    const opts = await api.post<RequestOptionsJSON>('/auth/mfa/passkey/options', undefined, { noAuthRedirect: true })
    const credential = await getPasskey(opts)
    await auth.verifyMfaPasskey(credential, remember.value)
    done()
  } catch (e) {
    if (mfaErrorKind(e) === 'expired') {
      challenge.value = null
      password.value = ''
      error.value = mfaError(e)
      return
    }
    codeError.value = passkeyError(e)
  } finally {
    passkeyBusy.value = false
  }
}
async function cancelSecondFactor() {
  // drops the pending sign-in on the server
  await api.post('/auth/logout', undefined, { noAuthRedirect: true }).catch(() => undefined)
  challenge.value = null
  password.value = ''
  router.replace({ name: 'login', query: { ...route.query, mfa: undefined } })
}
function done() {
  // without an explicit redirect, '/' leads to the preferred start page
  const target = (route.query.redirect as string) || '/'
  router.replace(target.startsWith('/') && !target.startsWith('//') ? target : '/')
}

onMounted(async () => {
  if (route.query.error) error.value = String(route.query.error)
  // an identity provider sign-in that needs the second factor (/login?mfa=1)
  if (route.query.mfa) {
    try {
      const ch = await api.get<MfaChallenge>('/auth/mfa/challenge', undefined, { noAuthRedirect: true })
      if (isChallenge(ch)) void toSecondFactor(ch)
    } catch (e) {
      error.value = mfaError(e)
    }
  }
  try {
    providers.value = await api.get<Providers>('/auth/providers', undefined, { noAuthRedirect: true })
    sso.value = [
      ...(providers.value.oidc ?? []).map((p) => ({ ...p, kind: 'oidc' })),
      ...(providers.value.saml ?? []).map((p) => ({ ...p, kind: 'saml' })),
    ]
  } catch {
    /* keep local login */
  }
  // passkey suggestions in the e-mail field (conditional mediation)
  if (providers.value.passkeys && !challenge.value && (await conditionalAvailable())) void passkeySignIn('conditional')
})

async function submit() {
  error.value = ''
  stopConditional()
  busy.value = true
  try {
    const res = await auth.login(email.value.trim(), password.value, tenant.value.trim())
    if (isChallenge(res)) {
      void toSecondFactor(res)
      return
    }
    done()
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <section class="login-hero" :aria-label="t('auth.about')">
      <div class="row" style="gap: 12px; font-weight: 700; font-size: 18px; color: #fff">
        <div aria-hidden="true" style="width: 36px; height: 36px; border-radius: 9px; background: linear-gradient(135deg, #6366f1, #22d3ee); display: grid; place-items: center">
          <i class="pi pi-check" />
        </div>
        Qtestframe
      </div>
      <div>
        <p class="hero-title">{{ t('auth.heroTitle') }}<br />{{ t('auth.heroTitle2') }}</p>
        <p style="max-width: 460px; line-height: 1.6; opacity: 0.85">{{ t('auth.heroText') }}</p>
        <div class="row" style="gap: 18px; margin-top: 24px; opacity: 0.85">
          <span><i class="pi pi-list-check" aria-hidden="true" /> {{ t('auth.featureRequirements') }}</span>
          <span><i class="pi pi-check-square" aria-hidden="true" /> {{ t('auth.featureTestDesign') }}</span>
          <span><i class="pi pi-play-circle" aria-hidden="true" /> {{ t('auth.featureOperations') }}</span>
          <span><i class="pi pi-th-large" aria-hidden="true" /> {{ t('auth.featureTraceability') }}</span>
        </div>
      </div>
      <div class="small" style="opacity: 0.6">© Qtestframe</div>
    </section>
    <main id="main-content" class="login-form" tabindex="-1">
      <div class="box">
        <div class="lang-switch" role="group" :aria-label="t('language.label')">
          <i class="pi pi-globe muted" aria-hidden="true" />
          <button
            v-for="l in LANGUAGES"
            :key="l"
            type="button"
            :lang="l"
            :class="{ active: locale === l }"
            :aria-pressed="locale === l"
            @click="setLang(l)"
          >
            {{ LANGUAGE_NAMES[l] }}
          </button>
        </div>
        <template v-if="challenge">
          <h1 id="mfa-heading" tabindex="-1" style="margin: 0 0 4px; font-size: 1.5em">{{ t('mfa.loginTitle') }}</h1>
          <p class="muted" style="margin: 0 0 20px">{{ !hasCode() ? t('passkeys.mfaText') : useRecovery ? t('mfa.loginRecoveryText') : t('mfa.loginText') }}</p>
          <form class="grid mfa-login" style="gap: 14px" @submit.prevent="submitCode">
            <MfaCodeInput v-if="hasCode()" id="mfa-code" ref="codeInput" v-model="code" :recovery="useRecovery" :error="codeError" autofocus />
            <Message v-else-if="codeError" id="mfa-passkey-error" severity="error" :closable="false">{{ codeError }}</Message>
            <div v-if="challenge.remember_days > 0" class="row nowrap small">
              <Checkbox v-model="remember" input-id="mfa-remember" binary />
              <label for="mfa-remember">{{ t('mfa.remember', { n: challenge.remember_days }, challenge.remember_days) }}</label>
            </div>
            <template v-if="hasCode()">
              <Button type="submit" :label="t('mfa.verify')" icon="pi pi-sign-in" :loading="busy" :disabled="!code.trim()" />
              <button type="button" class="link-button small login-link" @click="switchMethod">{{ useRecovery ? t('mfa.useApp') : t('mfa.useRecovery') }}</button>
            </template>
            <Button v-if="hasPasskey()" type="button" :label="t('passkeys.use')" icon="pi pi-key" :severity="hasCode() ? 'secondary' : undefined" :outlined="hasCode()" :loading="passkeyBusy" @click="passkeySecondFactor" />
            <button type="button" class="link-button small login-link" @click="cancelSecondFactor">{{ t('mfa.cancelSignIn') }}</button>
          </form>
        </template>
        <template v-else>
        <h1 style="margin: 0 0 4px; font-size: 1.5em">{{ t('auth.signIn') }}</h1>
        <p class="muted" style="margin: 0 0 20px">{{ t('auth.welcome') }}</p>
        <Message v-if="error" id="login-error" severity="error" :closable="false" style="margin-bottom: 14px">{{ error }}</Message>
        <form v-if="providers.local !== false" class="grid" style="gap: 14px" @submit.prevent="submit">
          <div v-field class="field">
            <label for="email">{{ t('auth.email') }}</label>
            <InputText id="email" v-model="email" type="email" :autocomplete="providers.passkeys ? 'username webauthn' : 'username'" required autofocus :invalid="!!error" :aria-describedby="error ? 'login-error' : undefined" />
          </div>
          <div v-field class="field">
            <label for="pw">{{ t('auth.password') }}</label>
            <Password input-id="pw" v-model="password" :feedback="false" toggle-mask autocomplete="current-password" required fluid :invalid="!!error" :input-props="{ 'aria-describedby': error ? 'login-error' : undefined }" />
          </div>
          <div v-if="showTenant" v-field class="field">
            <label for="tenant">{{ t('auth.tenant') }}</label>
            <InputText id="tenant" v-model="tenant" :placeholder="t('auth.tenantPlaceholder')" />
          </div>
          <button v-else type="button" class="link-button small login-link" @click="showTenant = true">{{ t('auth.signInToTenant') }}</button>
          <Button type="submit" :label="t('auth.signIn')" icon="pi pi-sign-in" :loading="busy" :disabled="!email || !password" />
        </form>
        <div v-if="providers.passkeys" class="grid" style="gap: 6px; margin-top: 10px">
          <Button
            type="button"
            :label="t('passkeys.signIn')"
            icon="pi pi-key"
            severity="secondary"
            outlined
            class="w-full"
            :loading="passkeyBusy"
            :aria-describedby="support !== 'ok' ? 'passkey-support' : undefined"
            @click="passkeySignIn('modal')"
          />
          <span v-if="support !== 'ok'" id="passkey-support" class="small muted">{{ t(`passkeys.error.${support}`) }}</span>
        </div>
        <template v-if="sso.length">
          <Divider align="center"><span class="muted small">{{ t('auth.orContinueWith') }}</span></Divider>
          <div class="grid" style="gap: 8px">
            <Button
              v-for="p in sso"
              :key="p.kind + p.name"
              as="a"
              :href="p.login_url"
              :label="p.display_name || p.name"
              :icon="p.icon ? `pi pi-${p.icon}` : p.kind === 'saml' ? 'pi pi-shield' : 'pi pi-key'"
              severity="secondary"
              outlined
              class="w-full"
            />
          </div>
        </template>
        </template>
        <p v-if="providers.privacy_notice_url" class="small login-privacy">
          <a :href="providers.privacy_notice_url" target="_blank" rel="noopener noreferrer"><i class="pi pi-shield" aria-hidden="true" /> {{ t('privacy.notice.link') }}<span class="sr-only"> {{ t('privacy.notice.newWindow') }}</span></a>
        </p>
      </div>
    </main>
  </div>
</template>

<style scoped>
.lang-switch {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 4px;
  margin-bottom: 12px;
  font-size: 13px;
}
.login-privacy {
  margin: 16px 0 0;
  text-align: center;
}
.login-link {
  color: var(--p-primary-color);
  justify-self: start;
}
.lang-switch button {
  all: unset;
  cursor: pointer;
  padding: 2px 8px;
  min-height: 24px;
  border-radius: 6px;
  color: var(--qtf-muted, inherit);
}
.lang-switch button:hover,
.lang-switch button:focus-visible {
  background: color-mix(in srgb, var(--p-primary-color) 12%, transparent);
}
@media (pointer: coarse) {
  .lang-switch button {
    min-height: 44px;
    display: inline-flex;
    align-items: center;
  }
}
.lang-switch button.active {
  color: var(--p-primary-color);
  font-weight: 600;
}
</style>
