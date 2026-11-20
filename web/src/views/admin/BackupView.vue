<script setup lang="ts">
// Backup and restore (super admins): download a ZIP of all data, restore one.
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import ProgressBar from 'primevue/progressbar'
import FileDropZone from '@/components/FileDropZone.vue'
import { api, errorMessage } from '@/lib/api'
import { upload } from '@/lib/upload'
import { formatFileSize } from '@/lib/files'
import { formatNumber, getFormatLocale } from '@/lib/format'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const notify = useNotify()
const auth = useAuthStore()
const proj = useProjectStore()
const router = useRouter()
const CONFIRM_WORD = 'RESTORE'

const downloading = ref(false)
async function downloadBackup() {
  downloading.value = true
  try {
    const name = await api.download('/admin/backup', undefined, 'qtestframe-backup.zip')
    notify.ok(t('backup.downloaded'), name)
  } catch (e) {
    notify.fail(e, t('backup.downloadFailed'))
  } finally {
    downloading.value = false
  }
}

const file = ref<File | null>(null)
const confirmText = ref('')
const restoring = ref(false)
const percent = ref(0)
const error = ref('')
const result = ref<Record<string, number> | null>(null)
const ready = computed(() => !!file.value && confirmText.value.trim() === CONFIRM_WORD && !restoring.value)
const totalRows = computed(() => Object.values(result.value ?? {}).reduce((a, b) => a + (Number(b) || 0), 0))
function pick(list: File[]) {
  file.value = list[0] ?? null
  result.value = null
  error.value = ''
}
async function restore() {
  if (!file.value || !ready.value) return
  restoring.value = true
  error.value = ''
  percent.value = 0
  try {
    const res = await upload<{ restored: Record<string, number> }>('/admin/restore', file.value, {
      field: 'backup',
      fields: { confirm: CONFIRM_WORD },
      onProgress: (p) => (percent.value = p),
    })
    result.value = res?.restored ?? {}
    file.value = null
    confirmText.value = ''
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    restoring.value = false
  }
}
/** everyone was signed out: go to the login page */
function signInAgain() {
  auth.clear()
  proj.reset()
  router.push({ name: 'login' })
}
</script>

<template>
  <div class="backup-grid">
    <section class="card" aria-labelledby="h-backup">
      <h2 id="h-backup" class="plain-heading"><i class="pi pi-download" aria-hidden="true" /> {{ t('backup.backupTitle') }}</h2>
      <p class="small">{{ t('backup.backupText') }}</p>
      <Button :label="t('backup.download')" icon="pi pi-download" :loading="downloading" @click="downloadBackup" />
      <div class="section-title">{{ t('backup.cli') }}</div>
      <p class="small muted" style="margin-top: 0">{{ t('backup.cliText') }}</p>
      <pre class="console" tabindex="0" :aria-label="t('backup.cli')">qtestframe backup --out FILE.zip
qtestframe restore --in FILE.zip --yes</pre>
    </section>

    <section class="card" aria-labelledby="h-restore">
      <h2 id="h-restore" class="plain-heading"><i class="pi pi-upload" aria-hidden="true" /> {{ t('backup.restoreTitle') }}</h2>
      <Message severity="warn" :closable="false">{{ t('backup.restoreWarning') }}</Message>
      <template v-if="result">
        <Message severity="success" :closable="false" style="margin-top: 12px">
          <div role="status">{{ t('backup.restored', { rows: formatNumber(totalRows), tables: Object.keys(result).length }) }}</div>
        </Message>
        <div class="table-scroll" tabindex="0" role="region" :aria-label="t('backup.resultTable')" style="margin-top: 8px; max-height: 260px">
          <table class="result-table small">
            <caption class="sr-only">{{ t('backup.resultTable') }}</caption>
            <thead><tr><th scope="col">{{ t('backup.table') }}</th><th scope="col" style="text-align: right">{{ t('backup.rows') }}</th></tr></thead>
            <tbody><tr v-for="(n, tbl) in result" :key="tbl"><th scope="row" class="mono" style="font-weight: normal">{{ tbl }}</th><td style="text-align: right">{{ formatNumber(n) }}</td></tr></tbody>
          </table>
        </div>
        <Button :label="t('backup.signInAgain')" icon="pi pi-sign-in" style="margin-top: 12px" @click="signInAgain" />
      </template>
      <form v-else class="grid" style="gap: 12px; margin-top: 12px" @submit.prevent="restore">
        <FileDropZone :multiple="false" :max-mb="1024" accept=".zip,application/zip" :label="t('backup.chooseFile')" @files="pick" />
        <div v-if="file" class="small"><i class="pi pi-file" aria-hidden="true" /> {{ file.name }} <span class="muted">{{ formatFileSize(file.size, getFormatLocale()) }}</span></div>
        <div v-field class="field">
          <label>{{ t('backup.typeConfirm', { word: CONFIRM_WORD }) }}</label>
          <InputText v-model="confirmText" autocomplete="off" spellcheck="false" class="mono" />
        </div>
        <ProgressBar v-if="restoring" :value="percent" :aria-label="t('backup.restoring')" style="height: 6px" />
        <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
        <div><Button type="submit" :label="t('backup.restore')" icon="pi pi-exclamation-triangle" severity="danger" :disabled="!ready" :loading="restoring" /></div>
      </form>
    </section>
  </div>
</template>

<style scoped>
.backup-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 380px), 1fr));
  gap: 16px;
}
.result-table {
  width: 100%;
  border-collapse: collapse;
}
.result-table th,
.result-table td {
  padding: 4px 6px;
  border-top: 1px solid var(--qtf-border);
  text-align: left;
}
</style>
