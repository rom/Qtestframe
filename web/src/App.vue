<script setup lang="ts">
import Toast from 'primevue/toast'
import ConfirmDialog from 'primevue/confirmdialog'
import { usePrimeVue } from 'primevue/config'
import { nextTick, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { usePrefsStore } from '@/stores/prefs'
import { useAuthStore } from '@/stores/auth'
import { applyPreference, bindPrimeVue } from '@/i18n'
import { focusPageStart, isPageChange } from '@/lib/a11y'

usePrefsStore().start()
const { t } = useI18n()

// language: PrimeVue texts follow vue-i18n; the user's preference wins once /auth/me is known
bindPrimeVue(usePrimeVue().config)
const auth = useAuthStore()
watch(
  () => auth.me?.user?.preferences?.language,
  (lang) => applyPreference(lang),
  { immediate: true },
)

// ---------- page changes: move focus to the new page and announce its title ----------
const announcement = ref('')
const router = useRouter()
let pending = 0
router.afterEach((to, from, failure) => {
  if (failure || !isPageChange(to, from)) return
  const ticket = ++pending
  announcement.value = ''
  void nextTick(async () => {
    // views render their h1 once their data is there; wait a little for it
    for (let i = 0; i < 30 && ticket === pending; i++) {
      if (document.querySelector('#main-content h1')) break
      await new Promise((r) => setTimeout(r, 50))
    }
    if (ticket !== pending) return
    // keep focus where it is when something inside the new page already took it (e.g. autofocus)
    const active = document.activeElement
    const main = document.getElementById('main-content')
    if (!active || active === document.body || !main?.contains(active)) focusPageStart(document)
    announcement.value = document.title
  })
})

function skipToMain() {
  focusPageStart(document)
}

// errors stay assertive (role=alert); everything else is announced politely
const toastPt = {
  message: ({ props }: { props: { message?: { severity?: string } } }) => {
    const urgent = props.message?.severity === 'error' || props.message?.severity === 'warn'
    return { role: urgent ? 'alert' : 'status', 'aria-live': urgent ? 'assertive' : 'polite' }
  },
}
const toastBreakpoints = { '640px': { width: 'calc(100vw - 24px)', right: '12px', left: '12px' } }
</script>

<template>
  <a class="skip-link" href="#main-content" @click.prevent="skipToMain">{{ t('nav.skipToMain') }}</a>
  <RouterView />
  <div class="sr-only" role="status" aria-live="polite" aria-atomic="true">{{ announcement }}</div>
  <Toast position="top-right" :pt="toastPt" :breakpoints="toastBreakpoints" />
  <ConfirmDialog :breakpoints="{ '640px': '100vw' }" />
</template>
