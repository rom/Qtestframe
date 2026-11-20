<script setup lang="ts">
// Import requirements from a customer file (CSV, XLSX, ReqIF, DOCX):
// choose a file, check the preview and the column mapping, set the options
// and import. The server parses the file; nothing is stored before Import.
import { computed, nextTick, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import Select from 'primevue/select'
import Checkbox from 'primevue/checkbox'
import Message from 'primevue/message'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import { useI18n } from 'vue-i18n'
import FileDropZone from './FileDropZone.vue'
import FolderPickerDialog from './FolderPickerDialog.vue'
import StatusTag from './StatusTag.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { upload } from '@/lib/upload'
import { errorMessage } from '@/lib/api'
import { folderPath } from '@/lib/tree'
import { uid } from '@/lib/a11y'
import { P } from '@/lib/permissions'
import { enumLabel, enumOptions } from '@/i18n/labels'
import { useAuthStore } from '@/stores/auth'
import { useMetaStore } from '@/stores/meta'
import type { Folder } from '@/lib/types'
import {
  changedMapping,
  columnChoices,
  formatOf,
  IMPORT_ACCEPT,
  IMPORT_FIELDS,
  IMPORT_MAX_MB,
  importCount,
  importFields,
  PREVIEW_ROWS,
  targetPath,
  type ImportColumn,
  type ImportOptions,
  type ImportPreview,
  type ImportResult,
} from '@/lib/reqImport'

const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ projectId: string; folders: Folder[]; folderId?: string | null }>()
const emit = defineEmits<{ imported: [result: ImportResult] }>()
const { t } = useI18n()
const auth = useAuthStore()
const meta = useMetaStore()

const file = ref<File | null>(null)
const preview = ref<ImportPreview | null>(null)
const result = ref<ImportResult | null>(null)
const busy = ref(false)
const error = ref('')
const sheet = ref<string | null>(null)
const choices = ref<Record<string, string>>({})
/** columns as the server guessed them (no mapping sent): the base of the overrides */
const guessed = ref<ImportColumn[]>([])
const excluded = ref(new Set<number>())
const pickFolder = ref(false)
const ids = { mapping: uid('imp-map'), options: uid('imp-opt'), preview: uid('imp-prev'), status: uid('imp-status') }

const canTests = computed(() => auth.can(P.testWrite))
const canReview = computed(() => auth.can(P.requirementReview))
const options = ref<ImportOptions>({ folderId: null, createFolders: true, status: 'draft', createTests: false, skipDuplicates: true })

watch(visible, (v) => {
  if (!v) return
  file.value = null
  preview.value = null
  result.value = null
  error.value = ''
  sheet.value = null
  choices.value = {}
  guessed.value = []
  excluded.value = new Set()
  options.value = { folderId: props.folderId ?? null, createFolders: true, status: 'draft', createTests: false, skipDuplicates: true }
})

const fieldOptions = computed(() => [
  { label: t('reqImport.field.ignore'), value: '' },
  ...IMPORT_FIELDS.map((f) => ({ label: t(`reqImport.field.${f}`), value: f })),
])
const statusOptions = computed(() =>
  enumOptions(
    'status',
    meta.meta.requirement_statuses.filter((s) => canReview.value || (s !== 'approved' && s !== 'rejected')),
  ),
)
const sheetOptions = computed(() => (preview.value?.sheets ?? []).map((s) => ({ label: s, value: s })))
const rootPath = computed(() => (options.value.folderId ? folderPath(props.folders, options.value.folderId) : ''))
const shown = computed(() => (preview.value?.candidates ?? []).slice(0, PREVIEW_ROWS).map((c, i) => ({ ...c, index: i })))
const willCreate = computed(() => (preview.value ? importCount(preview.value.candidates, options.value.skipDuplicates, excluded.value) : 0))
const formatLabel = computed(() => (preview.value ? t(`reqImport.format.${preview.value.format}`) : ''))

async function runPreview() {
  if (!file.value) return
  busy.value = true
  error.value = ''
  try {
    const mapping = changedMapping(guessed.value, choices.value)
    const res = await upload<ImportPreview>(`/projects/${props.projectId}/requirements/import/preview`, file.value, {
      fields: importFields({ sheet: sheet.value, mapping }),
    })
    preview.value = res
    sheet.value = res.sheet || null
    if (!mapping) guessed.value = res.columns
    choices.value = columnChoices(res.columns, mapping ? choices.value : {})
    excluded.value = new Set()
  } catch (e) {
    error.value = errorMessage(e)
    preview.value = null
  } finally {
    busy.value = false
  }
}

function onFiles(files: File[]) {
  const f = files[0]
  if (!f) return
  result.value = null
  if (!formatOf(f.name)) {
    error.value = t('reqImport.unsupported', { name: f.name })
    return
  }
  file.value = f
  preview.value = null
  sheet.value = null
  choices.value = {}
  guessed.value = []
  void runPreview()
}

function onSheet(v: string) {
  sheet.value = v
  choices.value = {}
  guessed.value = []
  void runPreview()
}

function onMapping(header: string, field: string) {
  choices.value = { ...choices.value, [header]: field }
  void runPreview()
}

function toggle(index: number, on: boolean) {
  const s = new Set(excluded.value)
  if (on) s.delete(index)
  else s.add(index)
  excluded.value = s
}

async function doImport() {
  if (!file.value || !preview.value || !willCreate.value) return
  busy.value = true
  error.value = ''
  try {
    const mapping = changedMapping(guessed.value, choices.value)
    const res = await upload<ImportResult>(`/projects/${props.projectId}/requirements/import`, file.value, {
      fields: importFields({ sheet: sheet.value, mapping }, options.value, excluded.value),
    })
    result.value = res
    emit('imported', res)
    await nextTick()
    document.getElementById(ids.status)?.focus()
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

function dupLabel(d: string | undefined) {
  return d === 'external_id' ? t('reqImport.duplicateId') : t('reqImport.duplicateTitle')
}
</script>

<template>
  <Dialog
    v-model:visible="visible"
    :header="t('reqImport.title')"
    modal
    maximizable
    :maximize-button-props="{ 'aria-label': t('common.toggleMaximize') }"
    :style="{ width: 'min(1100px, calc(100vw - 16px))' }"
    :breakpoints="{ '640px': '100vw' }"
    :close-on-escape="!busy"
  >
    <div class="grid imp" style="gap: 14px" :aria-busy="busy">
      <div class="row small muted" style="align-items: flex-start">
        <p class="grow" style="margin: 0">{{ t('reqImport.intro') }}</p>
        <HelpLink page="requirements" :anchor="{ en: 'import-requirements', sv: 'importera-krav' }" />
      </div>

      <FileDropZone
        :multiple="false"
        :max-mb="IMPORT_MAX_MB"
        :accept="IMPORT_ACCEPT"
        :label="file ? t('reqImport.otherFile') : t('reqImport.chooseFile')"
        :disabled="busy"
        @files="onFiles"
      />
      <div v-if="file" class="row small" aria-live="polite">
        <i class="pi pi-file" aria-hidden="true" />
        <b class="ellipsis">{{ file.name }}</b>
        <span v-if="preview" class="muted">{{ t('reqImport.detected', { format: formatLabel, count: preview.total }) }}</span>
        <span v-if="busy" class="muted"><i class="pi pi-spin pi-spinner" aria-hidden="true" /> {{ t('reqImport.reading') }}</span>
      </div>
      <Message v-if="error" severity="error" :closable="false" role="alert">{{ error }}</Message>

      <template v-if="result">
        <div :id="ids.status" tabindex="-1" role="status">
          <Message severity="success" :closable="false">
            {{ t('reqImport.done', { created: result.created, skipped: result.skipped }) }}
            <template v-if="result.folders_created"> {{ t('reqImport.doneFolders', { count: result.folders_created }) }}</template>
            <template v-if="result.tests_created"> {{ t('reqImport.doneTests', { count: result.tests_created }) }}</template>
          </Message>
        </div>
        <ul v-if="result.items.length" class="imp-list small">
          <li v-for="it in result.items.slice(0, PREVIEW_ROWS)" :key="it.id"><span class="key">{{ it.key }}</span> {{ it.title }}</li>
          <li v-if="result.items.length > PREVIEW_ROWS" class="muted">{{ t('reqImport.andMore', { count: result.items.length - PREVIEW_ROWS }) }}</li>
        </ul>
        <details v-if="result.skipped_items.length" class="small">
          <summary>{{ t('reqImport.skippedList', { count: result.skipped_items.length }) }}</summary>
          <ul class="imp-list">
            <li v-for="s in result.skipped_items" :key="s.location + s.title">{{ s.location }}: {{ s.title }} — {{ t(`reqImport.reason.${s.reason}`) }}</li>
          </ul>
        </details>
      </template>

      <template v-else-if="preview">
        <Message v-for="(w, i) in preview.warnings" :key="'w' + i" severity="warn" :closable="false">{{ w }}</Message>
        <details v-if="preview.errors.length" class="small">
          <summary>{{ t('reqImport.problems', { count: preview.errors.length }) }}</summary>
          <ul class="imp-list">
            <li v-for="(e, i) in preview.errors" :key="i">{{ e }}</li>
          </ul>
        </details>

        <section :aria-labelledby="ids.mapping">
          <h3 :id="ids.mapping" class="section-title">{{ t('reqImport.columns') }}</h3>
          <p class="small muted" style="margin: 0 0 8px">{{ t('reqImport.columnsHint') }}</p>
          <div v-if="sheetOptions.length > 1" v-field class="field" style="max-width: 320px; margin-bottom: 8px">
            <label>{{ t('reqImport.sheet') }}</label>
            <Select :model-value="sheet" :options="sheetOptions" option-label="label" option-value="value" :disabled="busy" @update:model-value="onSheet" />
          </div>
          <div class="imp-cols">
            <div v-for="c in preview.columns" :key="c.header" v-field class="field">
              <label class="ellipsis" :title="c.header">{{ c.header }}</label>
              <Select
                :model-value="choices[c.header] ?? ''"
                :options="fieldOptions"
                option-label="label"
                option-value="value"
                :disabled="busy"
                @update:model-value="(v: string) => onMapping(c.header, v)"
              />
            </div>
            <p v-if="!preview.columns.length" class="small muted">{{ t('reqImport.noColumns') }}</p>
          </div>
        </section>

        <section :aria-labelledby="ids.options">
          <h3 :id="ids.options" class="section-title">{{ t('reqImport.options') }}</h3>
          <div class="form-grid">
            <div class="field">
              <span class="label" :id="ids.options + '-target'">{{ t('reqImport.target') }}</span>
              <div class="row nowrap">
                <Button
                  :label="rootPath || t('common.topLevel')"
                  icon="pi pi-folder"
                  severity="secondary"
                  outlined
                  class="ellipsis"
                  :aria-describedby="ids.options + '-target'"
                  aria-haspopup="dialog"
                  @click="pickFolder = true"
                />
              </div>
            </div>
            <div v-field class="field">
              <label>{{ t('reqImport.status') }}</label>
              <Select v-model="options.status" :options="statusOptions" option-label="label" option-value="value" />
              <span class="small muted">{{ t('reqImport.statusHint') }}</span>
            </div>
            <div class="field full imp-checks">
              <span class="row nowrap"><Checkbox v-model="options.createFolders" binary input-id="imp-folders" /><label for="imp-folders">{{ t('reqImport.createFolders') }}</label></span>
              <span class="row nowrap"><Checkbox v-model="options.skipDuplicates" binary input-id="imp-dups" /><label for="imp-dups">{{ t('reqImport.skipDuplicates', { count: preview.duplicates }) }}</label></span>
              <span v-if="canTests" class="row nowrap"><Checkbox v-model="options.createTests" binary input-id="imp-tests" /><label for="imp-tests">{{ t('reqImport.createTests') }}</label></span>
            </div>
          </div>
        </section>

        <section :aria-labelledby="ids.preview">
          <h3 :id="ids.preview" class="section-title">{{ t('reqImport.preview') }}</h3>
          <p class="small muted" style="margin: 0 0 8px">
            {{ preview.total > PREVIEW_ROWS ? t('reqImport.showingFirst', { shown: PREVIEW_ROWS, total: preview.total }) : t('reqImport.showingAll', { total: preview.total }) }}
          </p>
          <DataTable :value="shown" data-key="index" size="small" scrollable scroll-height="45vh" class="imp-table" :aria-labelledby="ids.preview">
            <Column :header="t('reqImport.include')" style="width: 70px">
              <template #body="{ data }">
                <Checkbox
                  :model-value="!excluded.has(data.index)"
                  binary
                  :input-id="`imp-inc-${data.index}`"
                  :aria-label="t('reqImport.includeRow', { title: data.title })"
                  @update:model-value="(v: boolean) => toggle(data.index, v)"
                />
              </template>
            </Column>
            <Column field="location" :header="t('reqImport.location')" style="width: 120px" />
            <Column field="external_id" :header="t('reqImport.field.external_id')" style="width: 110px"><template #body="{ data }"><span class="mono">{{ data.external_id }}</span></template></Column>
            <Column field="title" :header="t('reqImport.field.title')" style="min-width: 220px" body-class="stack-main">
              <template #body="{ data }">
                <div>{{ data.title }}</div>
                <div v-if="data.description && data.description !== data.title" class="small muted clamp">{{ data.description }}</div>
              </template>
            </Column>
            <Column :header="t('reqImport.field.obligation')" style="width: 120px">
              <template #body="{ data }">
                <StatusTag v-if="data.obligation" :value="data.obligation" kind="obligation" />
                <span v-if="data.obligation_from === 'text'" class="small muted"> {{ t('reqImport.detectedFromText') }}</span>
              </template>
            </Column>
            <Column :header="t('reqImport.field.folder')" style="min-width: 140px">
              <template #body="{ data }"><span class="small">{{ targetPath(rootPath, data.folder, options.createFolders) || t('common.topLevel') }}</span></template>
            </Column>
            <Column :header="t('reqImport.notes')" style="min-width: 160px">
              <template #body="{ data }">
                <span v-if="data.duplicate" class="small warn-text">{{ dupLabel(data.duplicate) }}<template v-if="options.skipDuplicates"> — {{ t('reqImport.willSkip') }}</template></span>
                <ul v-if="data.warnings.length" class="imp-list small muted">
                  <li v-for="(w, i) in data.warnings" :key="i">{{ w }}</li>
                </ul>
                <span v-if="data.priority || data.status || data.type" class="small muted">
                  {{ [enumLabel('requirementType', data.type), enumLabel('priority', data.priority), enumLabel('status', data.status)].filter(Boolean).join(' · ') }}
                </span>
              </template>
            </Column>
            <template #empty><div class="empty">{{ t('reqImport.none') }}</div></template>
          </DataTable>
        </section>
      </template>
    </div>
    <template #footer>
      <Button :label="result ? t('common.close') : t('common.cancel')" text severity="secondary" @click="visible = false" />
      <Button
        v-if="preview && !result"
        :label="t('reqImport.importN', { count: willCreate }, willCreate)"
        icon="pi pi-upload"
        :loading="busy"
        :disabled="busy || !willCreate"
        @click="doImport"
      />
    </template>
  </Dialog>
  <FolderPickerDialog v-model:visible="pickFolder" :folders="folders" :header="t('reqImport.target')" @pick="(id) => (options.folderId = id)" />
</template>

<style scoped>
.imp-cols {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
  gap: 8px 12px;
}
.imp-cols .field {
  min-width: 0;
}
.imp-checks {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 20px;
}
.imp-list {
  margin: 4px 0 0;
  padding-left: 18px;
}
.clamp {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.warn-text {
  color: var(--p-orange-600, #b45309);
}
.label {
  font-weight: 600;
  font-size: 0.875rem;
}
</style>
