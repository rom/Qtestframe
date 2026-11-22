<script setup lang="ts">
// Import of a risk register (POST /projects/{pid}/risks/import): a CSV or
// XLSX file with the columns of the risks report's Risk register (Key,
// Title, Likelihood, Impact, Status, Mitigation, ATT&CK techniques, EMB3D
// threats …) is previewed (dry run) and then imported: new rows become
// risks, rows naming a risk's key update it. Unknown ATT&CK and EMB3D ids
// are left out with a warning on the row.
import { computed, ref } from 'vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { useI18n } from 'vue-i18n'
import { errorMessage } from '@/lib/api'
import { upload } from '@/lib/upload'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ projectId: string }>()
const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ imported: [] }>()
const { t } = useI18n()
const notify = useNotify()

interface ImportRow {
  row: number
  key?: string
  title: string
  action: 'create' | 'update' | 'unchanged' | 'error'
  error?: string
  warnings: string[]
}
interface ImportResult {
  dry_run: boolean
  sheet: string
  created: number
  updated: number
  unchanged: number
  errors: number
  rows: ImportRow[]
}

const file = ref<File | null>(null)
const preview = ref<ImportResult | null>(null)
const error = ref('')
const busy = ref(false)
const input = ref<HTMLInputElement>()
const severity = (a: string) => (a === 'create' ? 'success' : a === 'update' ? 'info' : a === 'error' ? 'danger' : 'secondary')
const changes = computed(() => (preview.value ? preview.value.created + preview.value.updated : 0))

function reset() {
  file.value = null
  preview.value = null
  error.value = ''
}
async function send(dry: boolean): Promise<ImportResult | null> {
  if (!file.value) return null
  busy.value = true
  error.value = ''
  try {
    return await upload<ImportResult>(`/projects/${props.projectId}/risks/import`, file.value, { fields: { dry_run: dry ? 'true' : undefined } })
  } catch (e) {
    error.value = errorMessage(e)
    return null
  } finally {
    busy.value = false
  }
}
async function onFile(e: Event) {
  const el = e.target as HTMLInputElement
  const f = el.files?.[0]
  el.value = ''
  if (!f) return
  file.value = f
  preview.value = await send(true)
}
async function apply() {
  const res = await send(false)
  if (!res) return
  notify.ok(t('riskImport.done', { created: res.created, updated: res.updated }))
  emit('imported')
  visible.value = false
  reset()
}
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="t('riskImport.title')" :style="{ width: 'min(900px, 96vw)' }" @hide="reset">
    <p class="small muted" style="margin-top: 0">{{ t('riskImport.intro') }}</p>
    <div class="row" style="gap: 8px">
      <input ref="input" type="file" accept=".csv,.txt,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" class="sr-only" tabindex="-1" aria-hidden="true" @change="onFile" />
      <Button :label="file ? t('riskImport.otherFile') : t('riskImport.chooseFile')" icon="pi pi-file-import" :loading="busy && !preview" @click="input?.click()" />
      <span v-if="file" class="small">{{ file.name }}</span>
    </div>
    <Message v-if="error" severity="error" :closable="false" style="margin-top: 10px">{{ error }}</Message>
    <template v-if="preview">
      <p class="small" style="margin: 12px 0 6px" role="status">
        {{ t('riskImport.summary', { created: preview.created, updated: preview.updated, unchanged: preview.unchanged, errors: preview.errors, sheet: preview.sheet }) }}
      </p>
      <DataTable :value="preview.rows" size="small" scrollable scroll-height="50vh" data-key="row" :aria-label="t('riskImport.rows')">
        <Column field="row" :header="t('riskImport.row')" style="width: 70px" />
        <Column :header="t('riskImport.action')" style="width: 120px">
          <template #body="{ data }"><Tag :value="t(`riskImport.actions.${data.action}`)" :severity="severity(data.action)" /></template>
        </Column>
        <Column field="key" :header="t('riskImport.key')" style="width: 120px"><template #body="{ data }"><span class="mono">{{ data.key }}</span></template></Column>
        <Column :header="t('riskImport.titleCol')">
          <template #body="{ data }">
            <div>{{ data.title }}</div>
            <div v-if="data.error" class="small error-text">{{ data.error }}</div>
            <ul v-if="data.warnings.length" class="small muted" style="margin: 2px 0 0; padding-inline-start: 18px">
              <li v-for="(w, i) in data.warnings" :key="i">{{ w }}</li>
            </ul>
          </template>
        </Column>
      </DataTable>
    </template>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="visible = false" />
      <Button :label="t('riskImport.import', { n: changes })" icon="pi pi-check" :disabled="!preview || !changes" :loading="busy && !!preview" @click="apply" />
    </template>
  </Dialog>
</template>
