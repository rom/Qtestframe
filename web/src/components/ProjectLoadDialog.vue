<script setup lang="ts">
// Load project: upload a project archive, check it (dry run), choose the
// key and name of the new project, load it (POST /projects/import).
import { computed, nextTick, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import ProgressBar from 'primevue/progressbar'
import FileDropZone from '@/components/FileDropZone.vue'
import { errorMessage } from '@/lib/api'
import { upload } from '@/lib/upload'
import { formatFileSize } from '@/lib/files'
import { formatDate, formatNumber, getFormatLocale } from '@/lib/format'
import { archiveSummary, freeProjectKey, normalizeProjectKey, validProjectKey, type ProjectLoadResult } from '@/lib/projectArchive'
import type { Project } from '@/lib/types'
import { useI18n } from 'vue-i18n'

const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ takenKeys: string[] }>()
const emit = defineEmits<{ loaded: [project: Project, result: ProjectLoadResult] }>()
const { t } = useI18n()

const file = ref<File | null>(null)
const preview = ref<ProjectLoadResult | null>(null)
const key = ref('')
const name = ref('')
const phase = ref<'idle' | 'checking' | 'loading'>('idle')
const percent = ref(0)
const error = ref('')

watch(visible, (v) => {
  if (v) reset()
})
function reset() {
  file.value = null
  preview.value = null
  key.value = ''
  name.value = ''
  phase.value = 'idle'
  percent.value = 0
  error.value = ''
}

const summary = computed(() => (preview.value ? archiveSummary(preview.value.manifest.tables, preview.value.manifest.files) : null))
const keyTaken = computed(() => props.takenKeys.some((k) => k.toUpperCase() === normalizeProjectKey(key.value)))
const keyProblem = computed(() => {
  if (!key.value) return ''
  if (!validProjectKey(key.value)) return t('projectArchive.keyInvalid')
  if (keyTaken.value) return t('projectArchive.keyTaken', { key: normalizeProjectKey(key.value) })
  return ''
})
const busy = computed(() => phase.value !== 'idle')
const ready = computed(() => !!file.value && !!preview.value && !!name.value.trim() && validProjectKey(key.value) && !keyTaken.value && !busy.value)

async function pick(list: File[]) {
  file.value = list[0] ?? null
  preview.value = null
  error.value = ''
  if (!file.value) return
  phase.value = 'checking'
  percent.value = 0
  try {
    const res = await upload<ProjectLoadResult>('/projects/import', file.value, {
      field: 'archive',
      fields: { dry_run: 'true' },
      onProgress: (p) => (percent.value = p),
    })
    preview.value = res
    key.value = freeProjectKey(res.manifest.project.key, props.takenKeys)
    name.value = res.manifest.project.name
    phase.value = 'idle'
    await nextTick()
    // the key is what most often needs a change
    document.getElementById('pl-key')?.focus()
  } catch (e) {
    error.value = errorMessage(e)
    file.value = null
    phase.value = 'idle'
  }
}

async function load() {
  if (!file.value || !ready.value) return
  phase.value = 'loading'
  percent.value = 0
  error.value = ''
  try {
    const res = await upload<ProjectLoadResult>('/projects/import', file.value, {
      field: 'archive',
      fields: { key: normalizeProjectKey(key.value), name: name.value.trim() },
      onProgress: (p) => (percent.value = p),
    })
    visible.value = false
    if (res.project) emit('loaded', res.project, res)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    phase.value = 'idle'
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" :header="t('projectArchive.loadTitle')" modal class="project-load-dialog" style="width: min(560px, 95vw)" :closable="!busy">
    <form id="project-load-form" class="grid" style="gap: 12px" @submit.prevent="load">
      <p class="small muted" style="margin: 0">{{ t('projectArchive.loadText') }}</p>
      <FileDropZone :multiple="false" :max-mb="1024" accept=".zip,application/zip" :label="t('projectArchive.chooseFile')" :disabled="busy" @files="pick" />
      <div v-if="file" class="small"><i class="pi pi-file" aria-hidden="true" /> {{ file.name }} <span class="muted">{{ formatFileSize(file.size, getFormatLocale()) }}</span></div>
      <ProgressBar v-if="busy" :value="percent" :aria-label="phase === 'checking' ? t('projectArchive.checking') : t('projectArchive.loading')" style="height: 6px" />
      <div v-if="busy" class="small muted" role="status">{{ phase === 'checking' ? t('projectArchive.checking') : t('projectArchive.loading') }}</div>
      <template v-if="preview && summary">
        <section class="archive-preview" aria-labelledby="pl-source">
          <h3 id="pl-source" class="plain-heading small">{{ t('projectArchive.source', { key: preview.manifest.project.key, name: preview.manifest.project.name }) }}</h3>
          <div class="small muted">{{ t('projectArchive.savedAt', { date: formatDate(preview.manifest.created_at), version: preview.manifest.app_version || '?' }) }}</div>
          <ul class="plain-list counts small">
            <li>{{ t('projectArchive.countRequirements', { n: formatNumber(summary.requirements) }, summary.requirements) }}</li>
            <li>{{ t('projectArchive.countTests', { n: formatNumber(summary.tests) }, summary.tests) }}</li>
            <li>{{ t('projectArchive.countRuns', { n: formatNumber(summary.runs) }, summary.runs) }}</li>
            <li>{{ t('projectArchive.countFiles', { n: formatNumber(summary.files) }, summary.files) }}</li>
          </ul>
          <Message v-if="preview.skipped && Object.keys(preview.skipped).length" severity="info" :closable="false">{{ t('projectArchive.willSkip') }}</Message>
        </section>
        <div class="field">
          <label for="pl-key">{{ t('common.key') }}</label>
          <InputText
            id="pl-key"
            v-model="key"
            class="mono"
            style="text-transform: uppercase"
            autocomplete="off"
            spellcheck="false"
            required
            :invalid="!!keyProblem"
            :aria-describedby="keyProblem ? 'pl-key-hint pl-key-problem' : 'pl-key-hint'"
          />
          <small id="pl-key-hint" class="muted">{{ t('projectArchive.keyHint', { example: (normalizeProjectKey(key) || 'KEY') + '-REQ-1' }) }}</small>
          <small v-if="keyProblem" id="pl-key-problem" class="warn-text" aria-live="polite">{{ keyProblem }}</small>
        </div>
        <div class="field">
          <label for="pl-name">{{ t('common.name') }}</label>
          <InputText id="pl-name" v-model="name" required maxlength="200" />
        </div>
      </template>
      <Message v-if="error" severity="error" :closable="false"><span role="alert">{{ error }}</span></Message>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" severity="secondary" text :disabled="busy" @click="visible = false" />
        <Button type="submit" :label="t('projectArchive.load')" icon="pi pi-upload" :loading="phase === 'loading'" :disabled="!ready" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.archive-preview {
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  padding: 10px 12px;
  display: grid;
  gap: 6px;
}
.archive-preview h3 {
  margin: 0;
}
.counts {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 16px;
}
</style>
