<script setup lang="ts">
// Attachments of a requirement, test, run or execution: list, preview,
// download, remove, upload (drop zone / picker) and attach from the archive.
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import FileDropZone from './FileDropZone.vue'
import UploadQueue from './UploadQueue.vue'
import FilePreviewDialog from './FilePreviewDialog.vue'
import ArchivePickerDialog from './ArchivePickerDialog.vue'
import UserAvatar from './UserAvatar.vue'
import { useI18n } from 'vue-i18n'
import { api, asItems, errorMessage } from '@/lib/api'
import { attachmentsPath, contentUrl, fileIcon, formatFileSize, previewKind, sortFiles, type AttachableType } from '@/lib/files'
import { formatDateTime, formatFullDateTime, getFormatLocale } from '@/lib/format'
import type { FileInfo } from '@/lib/types'
import { useNotify } from '@/composables/notify'
import { useUploads } from '@/composables/uploads'
import { useMetaStore } from '@/stores/meta'

const props = defineProps<{ type: AttachableType; entityId: string; projectId: string; canWrite: boolean }>()
const emit = defineEmits<{ count: [n: number] }>()
const { t } = useI18n()
const notify = useNotify()
const meta = useMetaStore()
const maxMb = computed(() => (meta.meta as { max_upload_mb?: number }).max_upload_mb || undefined)

const files = ref<FileInfo[]>([])
const loading = ref(false)
const error = ref('')
const path = computed(() => attachmentsPath(props.type, props.entityId))
const { queue, announcement, run, dismiss } = useUploads()
const preview = ref<FileInfo | null>(null)
const previewOpen = ref(false)
const archiveOpen = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    files.value = sortFiles(asItems(await api.get<FileInfo[]>(path.value)))
    emit('count', files.value.length)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(path, load, { immediate: true })

async function onFiles(list: File[]) {
  const added = await run<FileInfo>(list, path.value, {}, {
    done: (name) => t('files.uploadedNamed', { name }),
    failed: (name, err) => t('files.uploadFailedNamed', { name, error: err }),
  })
  if (added.length) {
    files.value = sortFiles([...added.filter((f) => f?.id), ...files.value.filter((f) => !added.some((a) => a?.id === f.id))])
    emit('count', files.value.length)
  }
}

async function onPick(ids: string[]) {
  let ok = 0
  for (const id of ids) {
    try {
      await api.post(`${path.value}/link`, { file_id: id })
      ok++
    } catch (e) {
      notify.fail(e, t('files.linkFailed'))
    }
  }
  if (ok) {
    notify.ok(t('files.linked', { count: ok }, ok))
    await load()
  }
}

function open(f: FileInfo) {
  preview.value = f
  previewOpen.value = true
}

function remove(f: FileInfo) {
  const last = f.kind === 'attachment' && (f.links?.length ?? 1) <= 1
  notify.confirmDanger(
    last ? t('files.confirmRemoveLast', { name: f.name }) : t('files.confirmRemove', { name: f.name }),
    async () => {
      try {
        await api.del(`${path.value}/${f.id}`)
        files.value = files.value.filter((x) => x.id !== f.id)
        emit('count', files.value.length)
        notify.ok(t('files.removed'), f.name)
      } catch (e) {
        notify.fail(e)
      }
    },
    t('files.removeHeader'),
    t('common.remove'),
  )
}
</script>

<template>
  <div class="attachments">
    <template v-if="canWrite">
      <FileDropZone :max-mb="maxMb" @files="onFiles" />
      <div class="row" style="margin: 8px 0">
        <Button :label="t('files.attachFromArchive')" icon="pi pi-folder-open" size="small" severity="secondary" text @click="archiveOpen = true" />
      </div>
      <UploadQueue :queue="queue" :announcement="announcement" @dismiss="dismiss" />
    </template>
    <Message v-if="error" severity="error" :closable="false">{{ t('files.unavailable', { error }) }}</Message>
    <div v-else-if="loading && !files.length" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
    <div v-else-if="!files.length" class="muted small" style="padding: 8px 0">{{ t('files.none') }}</div>
    <ul v-else class="plain-list file-list" :aria-label="t('files.title')">
      <li v-for="f in files" :key="f.id" class="file-row">
        <i :class="fileIcon(f.content_type, f.name)" class="file-icon" aria-hidden="true" />
        <div class="grow" style="min-width: 0">
          <button v-if="previewKind(f) !== 'download'" type="button" class="link-button ellipsis file-name" @click="open(f)">{{ f.name }}</button>
          <a v-else :href="contentUrl(f.id)" :download="f.name" class="ellipsis file-name">{{ f.name }}</a>
          <div class="small muted row" style="gap: 4px 8px">
            <span>{{ formatFileSize(f.size, getFormatLocale()) }}</span>
            <span class="row nowrap" style="gap: 4px"><UserAvatar :email="f.uploaded_by" :name="f.uploaded_by" :size="16" /> {{ f.uploaded_by || t('common.unknown') }}</span>
            <span :title="formatFullDateTime(f.created_at)">{{ formatDateTime(f.created_at) }}</span>
          </div>
          <div v-if="f.description" class="small">{{ f.description }}</div>
        </div>
        <div class="row nowrap" style="gap: 0">
          <Button v-if="previewKind(f) !== 'download'" icon="pi pi-eye" text rounded size="small" severity="secondary" :aria-label="t('files.previewNamed', { name: f.name })" v-tooltip="t('files.preview')" @click="open(f)" />
          <a :href="contentUrl(f.id)" :download="f.name" class="p-button p-button-text p-button-rounded p-button-secondary p-button-icon-only p-button-sm" :aria-label="t('files.downloadNamed', { name: f.name })" v-tooltip="t('files.download')"><i class="pi pi-download" aria-hidden="true" /></a>
          <Button v-if="canWrite" icon="pi pi-times" text rounded size="small" severity="danger" :aria-label="t('files.removeNamed', { name: f.name })" v-tooltip="t('common.remove')" @click="remove(f)" />
        </div>
      </li>
    </ul>
    <FilePreviewDialog v-model:visible="previewOpen" :file="preview" />
    <ArchivePickerDialog v-model:visible="archiveOpen" :project-id="projectId" :exclude="files.map((f) => f.id)" @pick="onPick" />
  </div>
</template>

<style scoped>
.file-list {
  display: grid;
  gap: 0;
}
.file-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 0;
  border-bottom: 1px solid var(--qtf-border);
}
.file-icon {
  font-size: 20px;
  color: var(--p-primary-color);
  flex: none;
}
.file-name {
  display: block;
  max-width: 100%;
  font-weight: 600;
  text-align: left;
}
</style>
