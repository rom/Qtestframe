<script setup lang="ts">
import Button from 'primevue/button'
</script>

<template>
  <div class="empty">
    <i class="pi pi-compass" />
    <h2>Page not found</h2>
    <RouterLink :to="{ name: 'projects' }"><Button label="Back to projects" icon="pi pi-arrow-left" /></RouterLink>
  </div>
</template>
