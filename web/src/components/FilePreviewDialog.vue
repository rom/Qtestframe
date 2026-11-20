<script setup lang="ts">
// Inline preview of an image (img), PDF or text file (sandboxed iframe).
import { computed } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import { useI18n } from 'vue-i18n'
import { useEscapeClose } from '@/composables/escapeClose'
import { contentUrl, formatFileSize, previewKind } from '@/lib/files'
import { getFormatLocale } from '@/lib/format'
import type { FileInfo } from '@/lib/types'

const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ file: FileInfo | null }>()
const { t } = useI18n()
useEscapeClose(visible)
const kind = computed(() => previewKind(props.file))
const inline = computed(() => (props.file ? contentUrl(props.file.id, true) : ''))
const download = computed(() => (props.file ? contentUrl(props.file.id) : ''))
</script>

<template>
  <Dialog v-model:visible="visible" :close-on-escape="false" modal maximizable :header="file?.name || t('files.preview')" :style="{ width: 'min(960px, calc(100vw - 16px))' }" :breakpoints="{ '640px': '100vw' }">
    <template v-if="file">
      <div class="small muted" style="margin-bottom: 8px">{{ file.content_type }} · {{ formatFileSize(file.size, getFormatLocale()) }}<template v-if="file.description"> · {{ file.description }}</template></div>
      <div v-if="kind === 'image'" class="preview-box">
        <img :src="inline" :alt="file.description || file.name" style="max-width: 100%; max-height: 70vh; display: block; margin: 0 auto" />
      </div>
      <iframe v-else-if="kind === 'pdf' || kind === 'text'" :src="inline" sandbox="" :title="t('files.previewOf', { name: file.name })" class="preview-frame" referrerpolicy="no-referrer" />
      <p v-if="kind === 'pdf'" class="small muted" style="margin: 6px 0 0">{{ t('files.pdfHint') }}</p>
      <p v-else-if="kind === 'download'" class="muted">{{ t('files.noPreview') }}</p>
    </template>
    <template #footer>
      <a v-if="file && kind !== 'download'" :href="inline" target="_blank" rel="noopener noreferrer" class="p-button p-button-text p-button-secondary">
        <i class="pi pi-external-link" aria-hidden="true" />&nbsp;{{ t('files.openNewTab') }}
      </a>
      <a v-if="file" :href="download" class="p-button p-button-secondary p-button-outlined" :download="file.name"><i class="pi pi-download" aria-hidden="true" />&nbsp;{{ t('files.download') }}</a>
      <Button :label="t('common.close')" @click="visible = false" />
    </template>
  </Dialog>
</template>

<style scoped>
.preview-box {
  background: repeating-conic-gradient(var(--qtf-border) 0% 25%, transparent 0% 50%) 50% / 16px 16px;
  border-radius: 8px;
  padding: 8px;
}
.preview-frame {
  width: 100%;
  height: 70vh;
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  background: #fff;
}
</style>
