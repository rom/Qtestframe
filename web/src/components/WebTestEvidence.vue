<script setup lang="ts">
// Evidence of a web test (Playwright) in the execution runner: the
// screenshots, the trace and the video that the server attached to the
// execution after its last run (attachments tagged "web-test").
import { computed, ref, watch } from 'vue'
import HelpTip from '@/guide/HelpTip.vue'
import FilePreviewDialog from './FilePreviewDialog.vue'
import { api, asItems } from '@/lib/api'
import { attachmentsPath, contentUrl, formatFileSize } from '@/lib/files'
import { getFormatLocale } from '@/lib/format'
import { evidenceCount, showTraceCommand, TRACE_VIEWER, webEvidence } from '@/lib/webTests'
import type { FileInfo } from '@/lib/types'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ executionId: string; reloadKey?: string }>()
const { t } = useI18n()
const files = ref<FileInfo[]>([])
const loaded = ref(false)
const ev = computed(() => webEvidence(files.value))
const preview = ref<FileInfo | null>(null)
const previewOpen = ref(false)

async function load() {
  try {
    files.value = asItems(await api.get<FileInfo[]>(attachmentsPath('execution', props.executionId)))
  } catch {
    files.value = []
  } finally {
    loaded.value = true
  }
}
watch(() => [props.executionId, props.reloadKey], load, { immediate: true })
function open(f: FileInfo) {
  preview.value = f
  previewOpen.value = true
}
</script>

<template>
  <section class="card web-evidence" aria-labelledby="web-evidence-h">
    <h2 id="web-evidence-h" class="plain-heading" style="margin-bottom: 8px">
      <i class="pi pi-images" aria-hidden="true" /> {{ t('webTests.evidence') }}
      <HelpTip id="runner.webEvidence" :label="t('webTests.evidence')" />
    </h2>
    <p v-if="loaded && !evidenceCount(ev)" class="muted small">{{ t('webTests.evidenceNone') }}</p>
    <template v-if="ev.screenshots.length">
      <h3 class="plain-heading small-heading">{{ t('webTests.screenshots') }}</h3>
      <ul class="plain-list shots">
        <li v-for="f in ev.screenshots" :key="f.id">
          <button type="button" class="shot" :aria-label="t('webTests.openScreenshot', { name: f.name })" @click="open(f)">
            <img :src="contentUrl(f.id, true)" :alt="t('webTests.screenshotAlt', { name: f.name })" loading="lazy" />
          </button>
          <span class="small muted">{{ f.name }}</span>
        </li>
      </ul>
    </template>
    <template v-if="ev.traces.length">
      <h3 class="plain-heading small-heading">{{ t('webTests.trace') }}</h3>
      <p class="small">{{ t('webTests.traceHelp') }}</p>
      <div v-for="f in ev.traces" :key="f.id" class="row trace-row">
        <a :href="contentUrl(f.id)" :download="f.name" class="p-button p-button-sm p-button-outlined p-button-secondary"><i class="pi pi-download" aria-hidden="true" />&nbsp;{{ t('webTests.downloadTrace', { name: f.name }) }}</a>
        <span class="small muted">{{ formatFileSize(f.size, getFormatLocale()) }}</span>
        <code class="mono small">{{ showTraceCommand(f.name) }}</code>
      </div>
      <a :href="TRACE_VIEWER" target="_blank" rel="noopener noreferrer" class="small">{{ t('webTests.traceViewer') }} <i class="pi pi-external-link" aria-hidden="true" /></a>
    </template>
    <template v-if="ev.videos.length">
      <h3 class="plain-heading small-heading">{{ t('webTests.video') }}</h3>
      <video v-for="f in ev.videos" :key="f.id" :src="contentUrl(f.id, true)" controls preload="metadata" class="video" :aria-label="t('webTests.videoLabel', { name: f.name })" />
    </template>
    <FilePreviewDialog v-model:visible="previewOpen" :file="preview" />
  </section>
</template>

<style scoped>
.small-heading {
  font-size: 14px;
  margin: 12px 0 6px;
}
.shots {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}
.shots li {
  display: grid;
  gap: 4px;
  max-width: 260px;
}
.shot {
  padding: 0;
  border: 1px solid var(--qtf-border);
  border-radius: 6px;
  background: none;
  cursor: zoom-in;
  overflow: hidden;
}
.shot img {
  display: block;
  width: 100%;
  max-height: 180px;
  object-fit: cover;
  object-position: top;
}
.trace-row {
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 6px;
}
.video {
  display: block;
  max-width: 100%;
  max-height: 360px;
  border-radius: 6px;
}
</style>
