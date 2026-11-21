<script setup lang="ts">
// Public page of an e-mail invitation (/invite/:token): choose name and
// password (local accounts) or sign in with single sign-on.
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import { api, ApiError, errorMessage } from '@/lib/api'
import type { AuthProvider, Me, Providers } from '@/lib/types'
import { formatDateTime } from '@/lib/format'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'
import { useNotify } from '@/composables/notify'
import { enumLabel } from '@/i18n/labels'
import { useI18n } from 'vue-i18n'

interface Invitation {
  email: string
  display_name: string
  tenant_name: string
  role: string
  invited_by: string
  expires_at: string
  sso: boolean
  local: boolean
  password_min_length?: number
}

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const proj = useProjectStore()
const notify = useNotify()
const { t } = useI18n()
const token = computed(() => String(route.params.token ?? ''))
const inv = ref<Invitation | null>(null)
const state = ref<'loading' | 'ok' | 'invalid' | 'expired' | 'error'>('loading')
const loadError = ref('')
const name = ref('')
const password = ref('')
const repeat = ref('')
const error = ref('')
const busy = ref(false)
const sso = ref<(AuthProvider & { kind: string })[]>([])

onMounted(async () => {
  try {
    inv.value = await api.get<Invitation>(`/invitations/${encodeURIComponent(token.value)}`, undefined, { noAuthRedirect: true })
    name.value = inv.value?.display_name ?? ''
    state.value = 'ok'
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) state.value = 'invalid'
    else if (e instanceof ApiError && e.status === 410) state.value = 'expired'
    else {
      state.value = 'error'
      loadError.value = errorMessage(e)
    }
    return
  }
  if (inv.value?.sso || inv.value?.local === false) {
    try {
      const p = await api.get<Providers>('/auth/providers', undefined, { noAuthRedirect: true })
      sso.value = [...(p.oidc ?? []).map((x) => ({ ...x, kind: 'oidc' })), ...(p.saml ?? []).map((x) => ({ ...x, kind: 'saml' }))]
    } catch {
      sso.value = []
    }
  }
})

async function accept() {
  error.value = ''
  if (password.value !== repeat.value) {
    error.value = t('profile.passwordsDiffer')
    return
  }
  busy.value = true
  try {
    const me = await api.post<Me>(`/invitations/${encodeURIComponent(token.value)}/accept`, { display_name: name.value.trim(), password: password.value }, { noAuthRedirect: true })
    auth.set(me)
    proj.reset()
    notify.ok(t('email.accept.welcome', { tenant: inv.value?.tenant_name ?? '' }))
    router.replace('/')
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
async function signOut() {
  await auth.logout().catch(() => undefined)
  proj.reset()
}
</script>

<template>
  <div class="login-page" style="grid-template-columns: minmax(0, 1fr)">
    <main id="main-content" class="login-form" tabindex="-1">
      <div class="box card invite-box">
        <div v-if="state === 'loading'" class="row" role="status"><ProgressSpinner style="width: 24px; height: 24px" :aria-label="t('email.accept.loading')" /> {{ t('email.accept.loading') }}</div>
        <template v-else-if="state !== 'ok'">
          <h1 style="margin: 0 0 12px; font-size: 1.5em"><i class="pi pi-envelope" aria-hidden="true" /> {{ t('titles.invite') }}</h1>
          <Message severity="warn" :closable="false" style="margin-bottom: 14px">
            {{ state === 'invalid' ? t('email.accept.invalid') : state === 'expired' ? t('email.accept.expired') : loadError }}
          </Message>
          <RouterLink :to="{ name: 'login' }" class="p-button p-button-secondary p-button-outlined">{{ t('email.accept.toLogin') }}</RouterLink>
        </template>
        <template v-else-if="inv">
          <h1 style="margin: 0 0 4px; font-size: 1.5em"><i class="pi pi-envelope" aria-hidden="true" /> {{ t('email.accept.title', { tenant: inv.tenant_name }) }}</h1>
          <p class="muted" style="margin: 0 0 6px">{{ t('email.accept.intro', { inviter: inv.invited_by, tenant: inv.tenant_name, role: enumLabel('role', inv.role) }) }}</p>
          <p class="small muted" style="margin: 0 0 16px">{{ t('email.accept.expires', { date: formatDateTime(inv.expires_at) }) }}</p>
          <Message v-if="auth.me" severity="info" :closable="false" style="margin-bottom: 14px">
            {{ t('email.accept.signedInAs', { email: auth.me.user.email }) }}
            <div style="margin-top: 8px"><Button :label="t('email.accept.signOut')" icon="pi pi-sign-out" size="small" severity="secondary" outlined @click="signOut" /></div>
          </Message>
          <template v-else-if="inv.sso || !inv.local">
            <p style="margin: 0 0 12px">{{ t('email.accept.ssoIntro') }}</p>
            <div class="grid" style="gap: 8px">
              <Button v-for="p in sso" :key="p.kind + p.name" as="a" :href="p.login_url" :label="p.display_name || p.name" :icon="p.kind === 'saml' ? 'pi pi-shield' : 'pi pi-key'" severity="secondary" outlined class="w-full" />
              <RouterLink v-if="!sso.length" :to="{ name: 'login' }" class="p-button p-button-secondary p-button-outlined">{{ t('email.accept.toLogin') }}</RouterLink>
            </div>
          </template>
          <template v-else>
            <Message v-if="error" id="inv-error" severity="error" :closable="false" style="margin-bottom: 14px">{{ error }}</Message>
            <form class="grid" style="gap: 14px" @submit.prevent="accept">
              <div v-field class="field">
                <label for="inv-email">{{ t('email.accept.email') }}</label>
                <InputText id="inv-email" :model-value="inv.email" type="email" autocomplete="username" readonly />
              </div>
              <div v-field class="field">
                <label for="inv-name">{{ t('email.accept.name') }}</label>
                <InputText id="inv-name" v-model="name" autocomplete="name" required autofocus :maxlength="100" />
              </div>
              <div v-field class="field">
                <label for="inv-pw">{{ t('email.accept.password') }}</label>
                <Password v-model="password" input-id="inv-pw" toggle-mask fluid autocomplete="new-password" required :input-props="{ 'aria-describedby': error ? 'inv-error inv-pw-hint' : 'inv-pw-hint' }" />
                <small id="inv-pw-hint" class="muted">{{ t('email.accept.passwordHint', { min: inv.password_min_length ?? 10 }) }}</small>
              </div>
              <div v-field class="field">
                <label for="inv-rep">{{ t('email.accept.repeat') }}</label>
                <Password v-model="repeat" input-id="inv-rep" :feedback="false" toggle-mask fluid autocomplete="new-password" required :input-props="{ 'aria-describedby': error ? 'inv-error' : undefined }" />
              </div>
              <Button type="submit" :label="t('email.accept.submit')" icon="pi pi-sign-in" :loading="busy" :disabled="!name.trim() || !password || !repeat" />
            </form>
          </template>
        </template>
      </div>
    </main>
  </div>
</template>

<style scoped>
.invite-box {
  max-width: 460px;
}
</style>
