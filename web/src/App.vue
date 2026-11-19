<script setup lang="ts">
import Toast from 'primevue/toast'
import ConfirmDialog from 'primevue/confirmdialog'
import { usePrefsStore } from '@/stores/prefs'

usePrefsStore().start()
</script>

<template>
  <RouterView />
  <Toast position="top-right" />
  <ConfirmDialog />
</template>
