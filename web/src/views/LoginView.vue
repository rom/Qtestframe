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

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

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
    const target = (route.query.redirect as string) || '/projects'
    router.replace(target.startsWith('/') && !target.startsWith('//') ? target : '/projects')
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <section class="login-hero">
      <div class="row" style="gap: 12px; font-weight: 700; font-size: 18px; color: #fff">
        <div style="width: 36px; height: 36px; border-radius: 9px; background: linear-gradient(135deg, #6366f1, #22d3ee); display: grid; place-items: center">
          <i class="pi pi-check" />
        </div>
        Qtestframe
      </div>
      <div>
        <h1>Requirements and tests,<br />traced end to end.</h1>
        <p style="max-width: 460px; line-height: 1.6; opacity: 0.85">
          Capture requirements with obligation-aware language analysis, design manual and automated tests, run campaigns and
          prove coverage, all in one multi-tenant workspace.
        </p>
        <div class="row" style="gap: 18px; margin-top: 24px; opacity: 0.85">
          <span><i class="pi pi-list-check" /> Requirements</span>
          <span><i class="pi pi-check-square" /> Test design</span>
          <span><i class="pi pi-play-circle" /> Operations</span>
          <span><i class="pi pi-th-large" /> Traceability</span>
        </div>
      </div>
      <div class="small" style="opacity: 0.6">© Qtestframe</div>
    </section>
    <section class="login-form">
      <div class="box">
        <h2 style="margin: 0 0 4px">Sign in</h2>
        <p class="muted" style="margin: 0 0 20px">Welcome back. Please enter your details.</p>
        <Message v-if="error" severity="error" :closable="false" style="margin-bottom: 14px">{{ error }}</Message>
        <form v-if="providers.local !== false" class="grid" style="gap: 14px" @submit.prevent="submit">
          <div class="field">
            <label for="email">E-mail</label>
            <InputText id="email" v-model="email" type="email" autocomplete="username" required autofocus />
          </div>
          <div class="field">
            <label for="pw">Password</label>
            <Password input-id="pw" v-model="password" :feedback="false" toggle-mask autocomplete="current-password" required fluid />
          </div>
          <div v-if="showTenant" class="field">
            <label for="tenant">Tenant (slug, optional)</label>
            <InputText id="tenant" v-model="tenant" placeholder="e.g. acme" />
          </div>
          <a v-else href="#" class="small" @click.prevent="showTenant = true">Sign in to a specific tenant…</a>
          <Button type="submit" label="Sign in" icon="pi pi-sign-in" :loading="busy" :disabled="!email || !password" />
        </form>
        <template v-if="sso.length">
          <Divider align="center"><span class="muted small">or continue with</span></Divider>
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
    </section>
  </div>
</template>
