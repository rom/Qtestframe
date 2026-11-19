<script setup lang="ts">
import Button from 'primevue/button'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<template>
  <div class="empty">
    <i class="pi pi-compass" />
    <h2>{{ t('notFound.title') }}</h2>
    <RouterLink :to="{ name: 'projects' }"><Button :label="t('notFound.back')" icon="pi pi-arrow-left" /></RouterLink>
  </div>
</template>
