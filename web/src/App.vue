<script setup lang="ts">
import Toast from 'primevue/toast'
import ConfirmDialog from 'primevue/confirmdialog'
import { usePrimeVue } from 'primevue/config'
import { watch } from 'vue'
import { usePrefsStore } from '@/stores/prefs'
import { useAuthStore } from '@/stores/auth'
import { applyPreference, bindPrimeVue } from '@/i18n'

usePrefsStore().start()

// language: PrimeVue texts follow vue-i18n; the user's preference wins once /auth/me is known
bindPrimeVue(usePrimeVue().config)
const auth = useAuthStore()
watch(
  () => auth.me?.user?.preferences?.language,
  (lang) => applyPreference(lang),
  { immediate: true },
)
</script>

<template>
  <RouterView />
  <Toast position="top-right" />
  <ConfirmDialog />
</template>
