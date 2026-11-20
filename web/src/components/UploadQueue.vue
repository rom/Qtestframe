<script setup lang="ts">
import ProgressBar from 'primevue/progressbar'
import Button from 'primevue/button'
import { useI18n } from 'vue-i18n'
import { formatFileSize } from '@/lib/files'
import { getFormatLocale } from '@/lib/format'
import type { QueuedUpload } from '@/composables/uploads'

defineProps<{ queue: QueuedUpload[]; announcement: string }>()
const emit = defineEmits<{ dismiss: [id: number] }>()
const { t } = useI18n()
</script>

<template>
  <ul v-if="queue.length" class="plain-list upload-queue" :aria-label="t('files.uploads')">
    <li v-for="q in queue" :key="q.id">
      <div class="row nowrap" style="justify-content: space-between">
        <span class="ellipsis small"><b>{{ q.name }}</b> <span class="muted">{{ formatFileSize(q.size, getFormatLocale()) }}</span></span>
        <span v-if="q.error" class="row nowrap">
          <span class="small error-text">{{ t('files.uploadFailed') }}</span>
          <Button icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('files.dismiss', { name: q.name })" @click="emit('dismiss', q.id)" />
        </span>
        <span v-else class="small muted">{{ q.done ? t('files.uploaded') : `${q.percent} %` }}</span>
      </div>
      <ProgressBar :value="q.percent" :show-value="false" :aria-label="t('files.uploading', { name: q.name })" style="height: 4px" :class="{ failed: !!q.error }" />
      <div v-if="q.error" class="small error-text">{{ q.error }}</div>
    </li>
  </ul>
  <div class="sr-only" role="status" aria-live="polite">{{ announcement }}</div>
</template>

<style scoped>
.upload-queue {
  display: grid;
  gap: 8px;
  margin: 8px 0;
}
.failed :deep(.p-progressbar-value) {
  background: var(--p-red-500);
}
</style>
