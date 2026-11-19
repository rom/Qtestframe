<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Divider from 'primevue/divider'
import { api, errorMessage } from '@/lib/api'
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

onMounted(async () => {
  if (route.query.error) error.value = String(route.query.error)
  try {
    providers.value = await api.get<Providers>('/auth/providers', undefined, { noAuthRedirect: true })
    sso.value = [
      ...(providers.value.oidc ?? []).map((p) => ({ ...p, kind: 'oidc' })),
      ...(providers.value.saml ?? []).map((p) => ({ ...p, kind: 'saml' })),
    ]
  } catch {
    /* keep local login */
  }
})

async function submit() {
  error.value = ''
  busy.value = true
  try {
    await auth.login(email.value.trim(), password.value, tenant.value.trim())
    // without an explicit redirect, '/' leads to the preferred start page
    const target = (route.query.redirect as string) || '/'
    router.replace(target.startsWith('/') && !target.startsWith('//') ? target : '/')
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
        <h1 style="margin: 0 0 4px; font-size: 1.5em">{{ t('auth.signIn') }}</h1>
        <p class="muted" style="margin: 0 0 20px">{{ t('auth.welcome') }}</p>
        <Message v-if="error" id="login-error" severity="error" :closable="false" style="margin-bottom: 14px">{{ error }}</Message>
        <form v-if="providers.local !== false" class="grid" style="gap: 14px" @submit.prevent="submit">
          <div v-field class="field">
            <label for="email">{{ t('auth.email') }}</label>
            <InputText id="email" v-model="email" type="email" autocomplete="username" required autofocus :invalid="!!error" :aria-describedby="error ? 'login-error' : undefined" />
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
        <template v-if="sso.length">
          <Divider align="center"><span class="muted small">{{ t('auth.orContinueWith') }}</span></Divider>
          <div class="grid" style="gap: 8px">
            <Button
              v-for="p in sso"
              :key="p.kind + p.name"
              as="a"
              :href="p.login_url"
              :label="p.display_name || p.name"
              :icon="p.kind === 'saml' ? 'pi pi-shield' : 'pi pi-key'"
              severity="secondary"
              outlined
              class="w-full"
            />
          </div>
        </template>
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
