<script setup lang="ts">
import { ref } from 'vue'
import Password from 'primevue/password'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { api, errorMessage } from '@/lib/api'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { formatDate, humanize } from '@/lib/format'

const auth = useAuthStore()
const notify = useNotify()
const current = ref('')
const next = ref('')
const confirm = ref('')
const error = ref('')
const busy = ref(false)

async function change() {
  error.value = ''
  if (next.value !== confirm.value) {
    error.value = 'The new passwords do not match.'
    return
  }
  busy.value = true
  try {
    await api.post('/auth/password', { current_password: current.value, new_password: next.value })
    current.value = next.value = confirm.value = ''
    notify.ok('Password changed')
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="page-header"><h1>Profile</h1></div>
  <div class="charts">
    <div class="card">
      <h3>Account</h3>
      <div class="grid" style="gap: 10px">
        <div><span class="muted small">Name</span><div>{{ auth.user?.display_name || '—' }}</div></div>
        <div><span class="muted small">E-mail</span><div>{{ auth.user?.email }}</div></div>
        <div><span class="muted small">Sign-in provider</span><div>{{ auth.user?.auth_provider || 'local' }}</div></div>
        <div><span class="muted small">Member since</span><div>{{ formatDate(auth.user?.created_at) }}</div></div>
        <div v-if="auth.isSuperAdmin"><Tag severity="danger" value="Super administrator" /></div>
      </div>
      <h3 style="margin-top: 18px">Tenant memberships</h3>
      <div v-for="m in auth.me?.memberships ?? []" :key="m.tenant_id" class="row" style="justify-content: space-between; padding: 6px 0; border-bottom: 1px solid var(--qtf-border)">
        <span>{{ m.tenant_name || m.tenant_slug }}</span><Tag :value="humanize(m.role)" severity="secondary" />
      </div>
      <div v-if="!(auth.me?.memberships ?? []).length" class="muted">No memberships.</div>
    </div>
    <div class="card">
      <h3>Change password</h3>
      <Message v-if="auth.user?.auth_provider && auth.user.auth_provider !== 'local'" severity="info" :closable="false" style="margin-bottom: 12px">
        Your account signs in via {{ auth.user.auth_provider }}; the password is managed by your identity provider.
      </Message>
      <Message v-if="error" severity="error" :closable="false" style="margin-bottom: 12px">{{ error }}</Message>
      <form class="grid" style="gap: 12px" @submit.prevent="change">
        <div class="field"><label>Current password</label><Password v-model="current" :feedback="false" toggle-mask fluid autocomplete="current-password" /></div>
        <div class="field"><label>New password</label><Password v-model="next" toggle-mask fluid autocomplete="new-password" /></div>
        <div class="field"><label>Repeat new password</label><Password v-model="confirm" :feedback="false" toggle-mask fluid autocomplete="new-password" /></div>
        <div><Button type="submit" label="Change password" icon="pi pi-lock" :loading="busy" :disabled="!current || !next" /></div>
      </form>
    </div>
  </div>
</template>
