<script setup lang="ts">
// Forced password change after an administrator reset (must_change_password).
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import Password from 'primevue/password'
import Button from 'primevue/button'
import Message from 'primevue/message'
import { api, errorMessage } from '@/lib/api'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const auth = useAuthStore()
const proj = useProjectStore()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()
const current = ref('')
const next = ref('')
const repeat = ref('')
const error = ref('')
const busy = ref(false)

async function submit() {
  error.value = ''
  if (next.value !== repeat.value) {
    error.value = t('profile.passwordsDiffer')
    return
  }
  if (next.value === current.value) {
    error.value = t('passwordChange.sameAsOld')
    return
  }
  busy.value = true
  try {
    await api.post('/auth/password', { current_password: current.value, new_password: next.value })
    await auth.fetchMe()
    if (auth.me?.user) auth.me.user.must_change_password = false
    notify.ok(t('profile.passwordChanged'))
    // '/' resolves to the start page
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
  router.push({ name: 'login' })
}
</script>

<template>
  <div class="login-page" style="grid-template-columns: minmax(0, 1fr)">
    <main id="main-content" class="login-form" tabindex="-1">
      <div class="box card">
        <h1 style="margin: 0 0 4px; font-size: 1.5em"><i class="pi pi-lock" aria-hidden="true" /> {{ t('passwordChange.title') }}</h1>
        <p class="muted" style="margin: 0 0 16px">{{ t('passwordChange.intro', { email: auth.user?.email ?? '' }) }}</p>
        <Message v-if="error" id="cpw-error" severity="error" :closable="false" style="margin-bottom: 14px">{{ error }}</Message>
        <form class="grid" style="gap: 14px" @submit.prevent="submit">
          <div v-field class="field">
            <label for="cpw-cur">{{ t('passwordChange.temporary') }}</label>
            <Password v-model="current" input-id="cpw-cur" :feedback="false" toggle-mask fluid autocomplete="current-password" required autofocus />
          </div>
          <div v-field class="field">
            <label for="cpw-new">{{ t('profile.newPassword') }}</label>
            <Password v-model="next" input-id="cpw-new" toggle-mask fluid autocomplete="new-password" required :input-props="{ 'aria-describedby': error ? 'cpw-error' : undefined }" />
          </div>
          <div v-field class="field">
            <label for="cpw-rep">{{ t('profile.repeatPassword') }}</label>
            <Password v-model="repeat" input-id="cpw-rep" :feedback="false" toggle-mask fluid autocomplete="new-password" required :input-props="{ 'aria-describedby': error ? 'cpw-error' : undefined }" />
          </div>
          <Button type="submit" :label="t('passwordChange.submit')" icon="pi pi-check" :loading="busy" :disabled="!current || !next || !repeat" />
          <button type="button" class="link-button small" style="justify-self: start; color: var(--p-primary-color)" @click="signOut">{{ t('nav.signOut') }}</button>
        </form>
      </div>
    </main>
  </div>
</template>
