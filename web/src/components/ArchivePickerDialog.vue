<script setup lang="ts">
// Pick existing files of the project's document archive (attach from archive).
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Select from 'primevue/select'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import QueryInput from './QueryInput.vue'
import { useI18n } from 'vue-i18n'
import { useEscapeClose } from '@/composables/escapeClose'
import { api, asItems } from '@/lib/api'
import { ARCHIVE_KINDS, archiveParams, fileIcon, formatFileSize, sortFiles } from '@/lib/files'
import { formatDate, getFormatLocale } from '@/lib/format'
import type { FileInfo } from '@/lib/types'

const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ projectId: string; exclude?: string[] }>()
const emit = defineEmits<{ pick: [ids: string[]] }>()
const { t } = useI18n()
useEscapeClose(visible)
const files = ref<FileInfo[]>([])
const loading = ref(false)
const error = ref<unknown>(null)
const q = ref('')
const kind = ref('')
const selected = ref<string[]>([])
const kindOptions = computed(() => [{ label: t('documents.allKinds'), value: '' }, ...ARCHIVE_KINDS.map((k) => ({ label: t(`documents.kind.${k}`), value: k }))])
const shown = computed(() => files.value.filter((f) => !(props.exclude ?? []).includes(f.id)))

async function load() {
  loading.value = true
  error.value = null
  try {
    files.value = sortFiles(asItems(await api.get<{ items: FileInfo[] }>(`/projects/${props.projectId}/documents`, archiveParams(kind.value, q.value))))
  } catch (e) {
    error.value = e
  } finally {
    loading.value = false
  }
}
watch(visible, (v) => {
  if (v) {
    selected.value = []
    load()
  }
})
function confirm() {
  emit('pick', selected.value)
  visible.value = false
}
</script>

<template>
  <Dialog v-model:visible="visible" :close-on-escape="false" modal :header="t('files.attachFromArchive')" :style="{ width: 'min(720px, calc(100vw - 16px))' }" :breakpoints="{ '640px': '100vw' }">
    <div class="filterbar">
      <QueryInput v-model="q" scope="document" :error="error" :placeholder="t('documents.filterPlaceholder', { example: 'tag:spec type:pdf' })" @submit="load" />
      <Select v-model="kind" :options="kindOptions" option-label="label" option-value="value" :placeholder="t('documents.allKinds')" :aria-label="t('documents.kindFilter')" style="min-width: 150px" @change="load" />
    </div>
    <Message v-if="error && !(error as { code?: string }).code?.includes('invalid_query')" severity="error" :closable="false">{{ (error as Error).message }}</Message>
    <div v-if="loading" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
    <div v-else-if="!shown.length" class="empty"><i class="pi pi-inbox" aria-hidden="true" />{{ t('files.archiveEmpty') }}</div>
    <fieldset v-else class="plain-fieldset">
      <legend class="sr-only">{{ t('files.chooseFromArchive') }}</legend>
      <ul class="plain-list pick-list">
        <li v-for="f in shown" :key="f.id">
          <Checkbox v-model="selected" :input-id="`ap-${f.id}`" :value="f.id" />
          <label :for="`ap-${f.id}`" class="row nowrap grow" style="min-width: 0; cursor: pointer">
            <i :class="fileIcon(f.content_type, f.name)" class="muted" aria-hidden="true" />
            <span class="ellipsis grow">{{ f.name }}</span>
            <span class="small muted file-meta">{{ t(`documents.kind.${f.kind}`) }} · {{ formatFileSize(f.size, getFormatLocale()) }} · {{ formatDate(f.created_at) }}</span>
          </label>
        </li>
      </ul>
    </fieldset>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="visible = false" />
      <Button :label="t('files.attachSelected', { count: selected.length }, selected.length)" icon="pi pi-link" :disabled="!selected.length" @click="confirm" />
    </template>
  </Dialog>
</template>

<style scoped>
.pick-list {
  display: grid;
  gap: 2px;
  max-height: 50vh;
  overflow: auto;
}
.file-meta {
  white-space: nowrap;
}
.pick-list li {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 4px;
  border-bottom: 1px solid var(--qtf-border);
}
@media (max-width: 640px) {
  .pick-list .file-meta {
    display: none;
  }
}
</style>
