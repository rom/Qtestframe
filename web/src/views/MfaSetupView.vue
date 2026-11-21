<script setup lang="ts">
// Forced set-up of a second factor: the MFA policy requires one and the
// grace period is over (Me.mfa.enrolment_required, see router beforeEach).
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import MfaEnrol from '@/components/MfaEnrol.vue'
import MfaRecoveryCodes from '@/components/MfaRecoveryCodes.vue'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'
import { useI18n } from 'vue-i18n'

const auth = useAuthStore()
const proj = useProjectStore()
const router = useRouter()
const { t } = useI18n()
const codes = ref<string[]>([])

async function finish() {
  await auth.fetchMe()
  // '/' resolves to the start page
  router.replace('/')
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
      <div class="box card mfa-setup">
        <h1 style="margin: 0 0 4px; font-size: 1.5em"><i class="pi pi-shield" aria-hidden="true" /> {{ codes.length ? t('mfa.recoveryCodes') : t('mfa.requiredTitle') }}</h1>
        <p class="muted" style="margin: 0 0 16px">{{ codes.length ? t('mfa.codesExplain') : t('mfa.requiredIntro', { email: auth.user?.email ?? '' }) }}</p>
        <template v-if="codes.length">
          <MfaRecoveryCodes :codes="codes" :account="auth.user?.email ?? ''" />
          <Button :label="t('mfa.continue')" icon="pi pi-arrow-right" icon-pos="right" style="margin-top: 12px" @click="finish" />
        </template>
        <MfaEnrol v-else @enrolled="(c: string[]) => (codes = c)" />
        <button type="button" class="link-button small" style="margin-top: 16px; color: var(--p-primary-color)" @click="signOut">{{ t('nav.signOut') }}</button>
      </div>
    </main>
  </div>
</template>

<style scoped>
.mfa-setup {
  max-width: 640px;
}
</style>
