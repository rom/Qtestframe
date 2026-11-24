<script setup lang="ts">
// Backup and restore (super admins): scheduled backups with their history,
// "Run backup now", download and verification of their files; download a
// ZIP of all data, restore one.
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import ProgressBar from 'primevue/progressbar'
import Tag from 'primevue/tag'
import FileDropZone from '@/components/FileDropZone.vue'
import { api, errorMessage } from '@/lib/api'
import { upload } from '@/lib/upload'
import { formatFileSize } from '@/lib/files'
import { formatDateTime, formatNumber, getFormatLocale } from '@/lib/format'
import { hasFile, isRunning, retentionId, runSeconds, scheduleParts, shortHash, statusSeverity, verifyState, type BackupCheck, type BackupList, type BackupRun } from '@/lib/backups'
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

// ---------------------------------------------------------------- scheduled backups

const list = ref<BackupList | null>(null)
const loadError = ref('')
const starting = ref(false)
const busyRun = ref('')
const check = ref<{ run: BackupRun; result: BackupCheck } | null>(null)
let timer: ReturnType<typeof setInterval> | undefined

async function load() {
  try {
    list.value = await api.get<BackupList>('/admin/backups')
    loadError.value = ''
  } catch (e) {
    loadError.value = errorMessage(e)
  }
}
const running = computed(() => isRunning(list.value))
// poll while a backup runs
watch(running, (on) => {
  if (on && !timer) timer = setInterval(load, 3000)
  if (!on && timer) {
    clearInterval(timer)
    timer = undefined
  }
})
onMounted(load)
onUnmounted(() => timer && clearInterval(timer))

const settings = computed(() => list.value?.settings)
const scheduleText = computed(() => {
  const s = settings.value
  if (!s) return ''
  const p = scheduleParts(s.schedule)
  if (p.kind === 'daily') return t('backup.sched.daily', { time: p.time })
  if (p.kind === 'every') return t('backup.sched.every', { every: p.every })
  if (p.kind === 'weekly') {
    // 2026-11-22 is a Sunday
    const day = new Intl.DateTimeFormat(getFormatLocale(), { weekday: 'long', timeZone: 'UTC' }).format(new Date(Date.UTC(2026, 10, 22 + (p.day ?? 0))))
    return t('backup.sched.weekly', { day, time: p.time })
  }
  return s.schedule
})
const retentionText = computed(() => {
  const s = settings.value
  return s ? t(`backup.sched.retention.${retentionId(s.keep_last, s.keep_days)}`, { last: s.keep_last, days: s.keep_days }) : ''
})

async function runNow() {
  starting.value = true
  try {
    await api.post('/admin/backups/run')
    notify.ok(t('backup.sched.started'))
    await load()
  } catch (e) {
    notify.fail(e, t('backup.sched.startFailed'))
  } finally {
    starting.value = false
  }
}

async function downloadRun(r: BackupRun) {
  busyRun.value = r.id
  try {
    const name = await api.download(`/admin/backups/${encodeURIComponent(r.id)}/download`, undefined, r.file_name)
    notify.ok(t('backup.downloaded'), name)
  } catch (e) {
    notify.fail(e, t('backup.sched.downloadFailed'))
  } finally {
    busyRun.value = ''
  }
}

async function verifyRun(r: BackupRun) {
  busyRun.value = r.id
  check.value = null
  try {
    const result = await api.post<BackupCheck>(`/admin/backups/${encodeURIComponent(r.id)}/verify`)
    check.value = { run: r, result }
    await load()
  } catch (e) {
    notify.fail(e, t('backup.sched.verifyFailed'))
  } finally {
    busyRun.value = ''
  }
}

function duration(r: BackupRun): string {
  const s = runSeconds(r)
  if (s == null) return ''
  return s < 60 ? t('backup.sched.seconds', { n: s }) : t('backup.sched.minutes', { n: Math.round(s / 60) })
}

// ---------------------------------------------------------------- download and restore

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
const passphrase = ref('')
const restoring = ref(false)
const percent = ref(0)
const error = ref('')
const result = ref<Record<string, number> | null>(null)
const ready = computed(() => !!file.value && confirmText.value.trim() === CONFIRM_WORD && !restoring.value)
const totalRows = computed(() => Object.values(result.value ?? {}).reduce((a, b) => a + (Number(b) || 0), 0))
function pick(files: File[]) {
  file.value = files[0] ?? null
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
      fields: { confirm: CONFIRM_WORD, passphrase: passphrase.value || undefined },
      onProgress: (p) => (percent.value = p),
    })
    result.value = res?.restored ?? {}
    file.value = null
    confirmText.value = ''
    passphrase.value = ''
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
    <section class="card backup-wide" aria-labelledby="h-scheduled">
      <div class="row" style="gap: 8px; align-items: center; flex-wrap: wrap">
        <h2 id="h-scheduled" class="plain-heading" style="margin: 0"><i class="pi pi-history" aria-hidden="true" /> {{ t('backup.sched.title') }}</h2>
        <Tag v-if="settings" :value="settings.enabled ? t('backup.sched.on') : t('backup.sched.off')" :severity="settings.enabled ? 'success' : 'secondary'" />
        <span class="grow" />
        <RouterLink :to="{ name: 'systemSettings', query: { section: 'backup' } }" class="p-button p-button-outlined p-button-sm"><i class="pi pi-cog" aria-hidden="true" />&nbsp;{{ t('backup.sched.settings') }}</RouterLink>
        <Button :label="t('backup.sched.runNow')" icon="pi pi-play" size="small" :loading="starting" :disabled="running || !settings?.directory" @click="runNow" />
      </div>
      <p class="small muted">{{ t('backup.sched.intro') }}</p>
      <Message v-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>
      <template v-if="settings">
        <dl class="backup-facts">
          <dt>{{ t('backup.sched.schedule') }}</dt>
          <dd>
            {{ scheduleText }}
            <span v-if="settings.schedule_error" class="error-text">— {{ settings.schedule_error }}</span>
          </dd>
          <dt>{{ t('backup.sched.next') }}</dt>
          <dd>{{ settings.next_run_at ? formatDateTime(settings.next_run_at) : t('backup.sched.noNext') }}</dd>
          <dt>{{ t('backup.sched.directory') }}</dt>
          <dd :class="{ mono: !!settings.directory }">{{ settings.directory || t('backup.sched.noDirectory') }}</dd>
          <dt>{{ t('backup.sched.retentionLabel') }}</dt>
          <dd>{{ retentionText }}</dd>
          <dt>{{ t('backup.sched.encryption') }}</dt>
          <dd>{{ settings.encrypted ? t('backup.sched.encrypted') : t('backup.sched.notEncrypted') }}</dd>
          <dt>{{ t('backup.sched.upload') }}</dt>
          <dd :class="{ mono: settings.s3_enabled }">{{ settings.s3_enabled ? settings.s3_target || t('backup.sched.uploadIncomplete') : t('backup.sched.noUpload') }}</dd>
        </dl>
      </template>

      <div role="status" aria-live="polite">
        <Message v-if="check" :severity="check.result.ok ? 'success' : 'error'" :closable="false" style="margin: 8px 0">
          <div>
            <b>{{ check.result.ok ? t('backup.sched.intact', { file: check.run.file_name }) : t('backup.sched.damaged', { file: check.run.file_name }) }}</b>
            <span v-if="check.result.ok && check.result.manifest">
              {{ t('backup.sched.manifest', { rows: formatNumber(Object.values(check.result.manifest.tables).reduce((a, b) => a + b, 0)), tables: Object.keys(check.result.manifest.tables).length, schema: check.result.manifest.schema_version, version: check.result.manifest.app_version, date: formatDateTime(check.result.manifest.created_at) }) }}
            </span>
            <span v-else>{{ check.result.error }}</span>
          </div>
        </Message>
        <span v-if="running" class="small muted"><i class="pi pi-spin pi-spinner" aria-hidden="true" /> {{ t('backup.sched.running') }}</span>
      </div>

      <h3 class="plain-heading backup-sub">{{ t('backup.sched.history') }}</h3>
      <p v-if="list && !list.runs.length" class="small muted">{{ t('backup.sched.none') }}</p>
      <div v-else-if="list" class="table-scroll" tabindex="0" role="region" :aria-label="t('backup.sched.history')">
        <table class="result-table small">
          <caption class="sr-only">{{ t('backup.sched.history') }}</caption>
          <thead>
            <tr>
              <th scope="col">{{ t('backup.sched.startedAt') }}</th>
              <th scope="col">{{ t('backup.sched.status') }}</th>
              <th scope="col">{{ t('backup.sched.file') }}</th>
              <th scope="col" style="text-align: right">{{ t('backup.sched.size') }}</th>
              <th scope="col">{{ t('backup.sched.sha256') }}</th>
              <th scope="col">{{ t('backup.sched.verified') }}</th>
              <th scope="col"><span class="sr-only">{{ t('backup.sched.actions') }}</span></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in list.runs" :key="r.id">
              <th scope="row" style="font-weight: normal">
                {{ formatDateTime(r.started_at) }}
                <div class="muted">{{ t(`backup.sched.kind.${r.kind}`) }} · {{ r.started_by }}<template v-if="duration(r)"> · {{ duration(r) }}</template></div>
              </th>
              <td>
                <Tag :value="t(`backup.sched.state.${r.status}`)" :severity="statusSeverity(r.status)" />
                <div v-if="r.error" class="error-text backup-error">{{ r.error }}</div>
              </td>
              <td>
                <span class="mono">{{ r.file_name || '—' }}</span>
                <div class="muted">
                  <template v-if="r.encrypted"><i class="pi pi-lock" aria-hidden="true" /> {{ t('backup.sched.encryptedShort') }}</template>
                  <template v-if="r.remote_key"> <i class="pi pi-cloud-upload" aria-hidden="true" /> {{ t('backup.sched.uploaded') }}</template>
                  <template v-if="r.pruned_at"> {{ t('backup.sched.pruned', { date: formatDateTime(r.pruned_at) }) }}</template>
                </div>
              </td>
              <td style="text-align: right">{{ r.size ? formatFileSize(r.size, getFormatLocale()) : '' }}</td>
              <td><span v-if="r.sha256" class="mono" :title="r.sha256">{{ shortHash(r.sha256) }}</span></td>
              <td>
                <Tag v-if="verifyState(r) === 'ok'" :value="t('backup.sched.verifyOk')" severity="success" />
                <Tag v-else-if="verifyState(r) === 'failed'" :value="t('backup.sched.verifyBad')" severity="danger" />
                <span v-else class="muted">—</span>
              </td>
              <td class="backup-actions">
                <template v-if="hasFile(r)">
                  <Button icon="pi pi-download" text size="small" :aria-label="t('backup.sched.downloadNamed', { file: r.file_name })" :loading="busyRun === r.id" :disabled="!!busyRun" @click="downloadRun(r)" />
                  <Button icon="pi pi-check-square" text size="small" :aria-label="t('backup.sched.verifyNamed', { file: r.file_name })" :disabled="!!busyRun" @click="verifyRun(r)" />
                </template>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section class="card" aria-labelledby="h-backup">
      <h2 id="h-backup" class="plain-heading"><i class="pi pi-download" aria-hidden="true" /> {{ t('backup.backupTitle') }}</h2>
      <p class="small">{{ t('backup.backupText') }}</p>
      <Button :label="t('backup.download')" icon="pi pi-download" :loading="downloading" @click="downloadBackup" />
      <div class="section-title">{{ t('backup.cli') }}</div>
      <p class="small muted" style="margin-top: 0">{{ t('backup.cliText') }}</p>
      <pre class="console" tabindex="0" :aria-label="t('backup.cli')">qtestframe backup --out FILE.zip [--encrypt]
qtestframe backup verify FILE.zip
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
        <FileDropZone :multiple="false" :max-mb="1024" accept=".zip,.enc,application/zip" :label="t('backup.chooseFile')" @files="pick" />
        <div v-if="file" class="small"><i class="pi pi-file" aria-hidden="true" /> {{ file.name }} <span class="muted">{{ formatFileSize(file.size, getFormatLocale()) }}</span></div>
        <div v-field class="field">
          <label>{{ t('backup.passphrase') }}</label>
          <InputText v-model="passphrase" type="password" autocomplete="off" aria-describedby="restore-pass-hint" />
          <small id="restore-pass-hint" class="muted">{{ t('backup.passphraseHint') }}</small>
        </div>
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
.backup-wide {
  grid-column: 1 / -1;
}
.backup-facts {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 4px 16px;
  margin: 8px 0;
}
.backup-facts dt {
  font-weight: 600;
}
.backup-facts dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.backup-sub {
  margin: 16px 0 8px;
}
.backup-error {
  max-width: 32ch;
  overflow-wrap: anywhere;
}
.backup-actions {
  white-space: nowrap;
  text-align: right;
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
  vertical-align: top;
}
</style>
