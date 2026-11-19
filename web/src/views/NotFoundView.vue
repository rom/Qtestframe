<script setup lang="ts">
import Button from 'primevue/button'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<template>
  <div class="empty">
    <i class="pi pi-compass" aria-hidden="true" />
    <h1 style="font-size: 20px">{{ t('notFound.title') }}</h1>
    <Button as="router-link" :to="{ name: 'projects' }" :label="t('notFound.back')" icon="pi pi-arrow-left" />
  </div>
</template>
