<script setup lang="ts">
// Import tests from another test management tool (TestRail, Xray, Zephyr,
// qTest, Azure Test Plans): pick the source and the export file, check the
// preview and adjust the mapping of priorities, statuses and custom fields,
// then save. Tests imported before are updated, not duplicated; past
// results become completed runs. Nothing is saved before Save; the save is
// one transaction, after the impact gate.
import { computed, nextTick, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import Select from 'primevue/select'
import Checkbox from 'primevue/checkbox'
import RadioButton from 'primevue/radiobutton'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import { useI18n } from 'vue-i18n'
import FileDropZone from './FileDropZone.vue'
import FolderPickerDialog from './FolderPickerDialog.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { upload } from '@/lib/upload'
import { errorMessage } from '@/lib/api'
import { folderPath } from '@/lib/tree'
import { uid } from '@/lib/a11y'
import type { Folder } from '@/lib/types'
import { impactGateFor } from '@/composables/impact'
import type { ImpactDryRun } from '@/lib/correlations'
import {
  acceptsTestFile,
  savableRow,
  sourceInfo,
  targetsOf,
  TEST_IMPORT_ACTIONS,
  TEST_IMPORT_MAX_MB,
  TEST_IMPORT_SOURCES,
  testImportFields,
  testSaveCount,
  visibleTestRows,
  type TestImportAction,
  type TestImportPreview,
  type TestImportResult,
  type TestImportRow,
  type TestImportSource,
} from '@/lib/testImport'

const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ projectId: string; folders: Folder[]; folderId?: string | null }>()
const emit = defineEmits<{ imported: [result: TestImportResult] }>()
const { t } = useI18n()

const source = ref<TestImportSource>('testrail')
const file = ref<File | null>(null)
const preview = ref<TestImportPreview | null>(null)
const result = ref<TestImportResult | null>(null)
const busy = ref(false)
const error = ref('')
const folderId = ref<string | null>(null)
const priorities = ref<Record<string, string>>({})
const statuses = ref<Record<string, string>>({})
const fields = ref<Record<string, string>>({})
const createFolders = ref(true)
const links = ref(true)
const history = ref(true)
const overwrite = ref(false)
const create = ref(true)
const excluded = ref(new Set<number>())
const filter = ref<TestImportAction[]>([])
const pickFolder = ref(false)
const ids = {
  source: uid('ti-src'),
  howto: uid('ti-how'),
  mapping: uid('ti-map'),
  prio: uid('ti-prio'),
  stat: uid('ti-stat'),
  fields: uid('ti-fields'),
  rows: uid('ti-rows'),
  status: uid('ti-status'),
  target: uid('ti-target'),
  row: uid('ti-row'),
}

/** rows shown at once */
const SHOWN = 200
const info = computed(() => sourceInfo(source.value))
const rootPath = computed(() => (folderId.value ? folderPath(props.folders, folderId.value) : ''))
const opts = computed(() => ({ overwrite: overwrite.value, create: create.value }))
const toSave = computed(() => (preview.value ? testSaveCount(preview.value.rows, opts.value, excluded.value) : 0))
const shownRows = computed(() => (preview.value ? visibleTestRows(preview.value.rows, filter.value).slice(0, SHOWN) : []))
const filtered = computed(() => (preview.value ? visibleTestRows(preview.value.rows, filter.value).length : 0))
const newResults = computed(() => (preview.value ? preview.value.history.executions - preview.value.history.known : 0))
const priorityOptions = computed(() => [{ label: t('testImport.defaultValue', { value: t('enums.priority.medium') }), value: '' }, ...(preview.value?.priority_options ?? []).map((p) => ({ label: t(`enums.priority.${p}`), value: p }))])
const statusOptions = computed(() => [{ label: t('testImport.defaultValue', { value: t('enums.status.draft') }), value: '' }, ...(preview.value?.status_options ?? []).map((s) => ({ label: t(`enums.status.${s}`), value: s }))])
const fieldOptions = computed(() => [{ label: t('testImport.notImported'), value: '' }, ...(preview.value?.custom_fields ?? []).map((f) => ({ label: f.label, value: f.key }))])

watch(visible, (v) => {
  if (!v) return
  file.value = null
  preview.value = null
  result.value = null
  error.value = ''
  folderId.value = props.folderId ?? null
  createFolders.value = true
  links.value = true
  history.value = true
  overwrite.value = false
  create.value = true
  resetMapping()
})

function resetMapping() {
  priorities.value = {}
  statuses.value = {}
  fields.value = {}
  excluded.value = new Set()
  filter.value = []
}

function state() {
  return {
    source: source.value,
    folderId: folderId.value,
    priorities: priorities.value,
    statuses: statuses.value,
    fields: fields.value,
    createFolders: createFolders.value,
    links: links.value,
    history: history.value,
  }
}

function onSource() {
  result.value = null
  if (file.value && !acceptsTestFile(source.value, file.value.name)) {
    file.value = null
    preview.value = null
    return
  }
  resetMapping()
  preview.value = null
  if (file.value) void runPreview()
}

async function runPreview() {
  if (!file.value) return
  busy.value = true
  error.value = ''
  result.value = null
  try {
    const res = await upload<TestImportPreview>(`/projects/${props.projectId}/tests/import/preview`, file.value, { fields: testImportFields(state()) })
    preview.value = res
    priorities.value = targetsOf(res.priorities, (v) => v.value)
    statuses.value = targetsOf(res.statuses, (v) => v.value)
    fields.value = targetsOf(res.fields, (f) => f.name)
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
  if (!acceptsTestFile(source.value, f.name)) {
    error.value = t('testImport.unsupported', { name: f.name, source: info.value.name, formats: info.value.formats.join(', ') })
    return
  }
  file.value = f
  preview.value = null
  resetMapping()
  void runPreview()
}

function choose(which: 'priority' | 'status' | 'field', name: string, value: string) {
  const target = which === 'priority' ? priorities : which === 'status' ? statuses : fields
  target.value = { ...target.value, [name]: value }
  void runPreview()
}

function onFolder(id: string | null) {
  folderId.value = id
  if (preview.value) void runPreview()
}

function toggle(index: number, on: boolean) {
  const s = new Set(excluded.value)
  if (on) s.delete(index)
  else s.add(index)
  excluded.value = s
}

function toggleFilter(a: TestImportAction) {
  filter.value = filter.value.includes(a) ? filter.value.filter((x) => x !== a) : [...filter.value, a]
}

const severity: Record<TestImportAction, string> = { new: 'info', update: 'success', conflict: 'warn', error: 'danger', unchanged: 'secondary' }
const icon: Record<TestImportAction, string> = { new: 'pi pi-plus', update: 'pi pi-pencil', conflict: 'pi pi-exclamation-triangle', error: 'pi pi-times-circle', unchanged: 'pi pi-check' }

function rowLabel(r: TestImportRow) {
  return [r.external_id, r.title].filter(Boolean).join(' ')
}

async function save() {
  if (!file.value || !preview.value || !toSave.value) return
  busy.value = true
  error.value = ''
  try {
    const path = `/projects/${props.projectId}/tests/import/apply`
    const f = file.value
    const body = testImportFields(state(), { ...opts.value, excluded: excluded.value })
    const gate = await impactGateFor(props.projectId, (q) => upload<ImpactDryRun>(`${path}?dry_run=${q.dry_run}`, f, { fields: body }))
    if (!gate) return
    const res = await upload<TestImportResult>(`${path}?impact=${gate.impact}`, f, { fields: body })
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
</script>

<template>
  <Dialog
    v-model:visible="visible"
    :header="t('testImport.title')"
    modal
    maximizable
    :maximize-button-props="{ 'aria-label': t('common.toggleMaximize') }"
    :style="{ width: 'min(1100px, calc(100vw - 16px))' }"
    :breakpoints="{ '640px': '100vw' }"
    :close-on-escape="!busy"
  >
    <div class="grid" style="gap: 14px" :aria-busy="busy">
      <div class="row small muted" style="align-items: flex-start">
        <p class="grow" style="margin: 0">{{ t('testImport.intro') }}</p>
        <HelpLink page="interoperability" :anchor="{ en: 'import-tests-from-other-tools', sv: 'importera-testfall-från-andra-verktyg' }" />
      </div>

      <template v-if="!result">
        <fieldset class="ti-sources" :aria-describedby="ids.howto" :disabled="busy">
          <legend class="label">{{ t('testImport.source') }}</legend>
          <span v-for="s in TEST_IMPORT_SOURCES" :key="s.id" class="row nowrap">
            <RadioButton v-model="source" :value="s.id" :input-id="`${ids.source}-${s.id}`" :name="ids.source" @update:model-value="onSource" />
            <label :for="`${ids.source}-${s.id}`">{{ s.name }} <span class="muted small">({{ s.formats.join(', ') }})</span></label>
          </span>
        </fieldset>
        <p :id="ids.howto" class="small" style="margin: 0">{{ t(`testImport.howTo.${source}`) }}</p>
        <FileDropZone
          :multiple="false"
          :max-mb="TEST_IMPORT_MAX_MB"
          :accept="info.accept"
          :label="file ? t('exchange.otherFile') : t('exchange.chooseFile')"
          :disabled="busy"
          @files="onFiles"
        />
        <div v-if="file" class="row small" aria-live="polite">
          <i class="pi pi-file" aria-hidden="true" />
          <b class="ellipsis">{{ file.name }}</b>
          <span v-if="preview" class="muted">{{ preview.source_name }} · {{ preview.format.toUpperCase() }}</span>
          <span v-if="busy" class="muted"><i class="pi pi-spin pi-spinner" aria-hidden="true" /> {{ t('exchange.reading') }}</span>
        </div>
      </template>
      <Message v-if="error" severity="error" :closable="false" role="alert">{{ error }}</Message>

      <template v-if="result">
        <div :id="ids.status" tabindex="-1" role="status">
          <Message severity="success" :closable="false">
            {{ t('testImport.done', { created: result.created, updated: result.updated, unchanged: result.unchanged, skipped: result.skipped }) }}
            <template v-if="result.folders_created"> {{ t('exchange.doneFolders', { count: result.folders_created }) }}</template>
            <template v-if="result.links_added"> {{ t('testImport.doneLinks', { count: result.links_added }) }}</template>
            <template v-if="result.results_imported"> {{ t('testImport.doneHistory', { results: result.results_imported, runs: result.runs_created }) }}</template>
          </Message>
        </div>
        <Message v-for="(w, i) in result.warnings" :key="'rw' + i" severity="warn" :closable="false">{{ w }}</Message>
        <p v-if="result.unresolved_links.length" class="small" style="margin: 0">{{ t('testImport.unresolved', { refs: result.unresolved_links.join(', ') }) }}</p>
        <ul v-if="result.items.length" class="ti-list small">
          <li v-for="it in result.items.slice(0, 50)" :key="it.index">
            <span class="key">{{ it.key }}</span> {{ it.title }} — {{ t(`testImport.itemAction.${it.action}`) }}
          </li>
          <li v-if="result.items.length > 50" class="muted">{{ t('exchange.andMore', { count: result.items.length - 50 }) }}</li>
        </ul>
        <details v-if="result.skipped_rows.length" class="small">
          <summary>{{ t('exchange.skippedList', { count: result.skipped_rows.length }) }}</summary>
          <ul class="ti-list">
            <li v-for="s in result.skipped_rows" :key="s.index">{{ s.location }}: {{ s.title }} — {{ t(`exchange.reason.${s.reason}`) }}</li>
          </ul>
        </details>
      </template>

      <template v-else-if="preview">
        <Message v-for="(w, i) in [...preview.warnings, ...preview.errors]" :key="'w' + i" severity="warn" :closable="false">{{ w }}</Message>
        <p v-if="preview.previous" class="small" style="margin: 0"><i class="pi pi-replay" aria-hidden="true" /> {{ t('testImport.previous', { count: preview.previous, source: preview.source_name }) }}</p>
        <p v-if="preview.unresolved_links.length" class="small" style="margin: 0" role="note">
          <i class="pi pi-link" aria-hidden="true" /> {{ t('testImport.unresolved', { refs: preview.unresolved_links.join(', ') }) }}
        </p>

        <section :aria-labelledby="ids.mapping">
          <h3 :id="ids.mapping" class="section-title">{{ t('testImport.mapping') }}</h3>
          <p class="small muted" style="margin: 0 0 8px">{{ t('testImport.mappingHint') }}</p>
          <div class="grid" style="gap: 12px">
            <div v-if="preview.priorities.length" role="group" :aria-labelledby="ids.prio">
              <h4 :id="ids.prio" class="ti-sub">{{ t('testImport.priorities') }}</h4>
              <div class="ti-map">
                <div v-for="v in preview.priorities" :key="v.value" v-field class="field">
                  <label class="ellipsis" :title="v.value">{{ v.value }} <span class="muted small">({{ v.count }})</span></label>
                  <Select :model-value="priorities[v.value] ?? ''" :options="priorityOptions" option-label="label" option-value="value" :disabled="busy" @update:model-value="(x: string) => choose('priority', v.value, x)" />
                </div>
              </div>
            </div>
            <div v-if="preview.statuses.length" role="group" :aria-labelledby="ids.stat">
              <h4 :id="ids.stat" class="ti-sub">{{ t('testImport.statuses') }}</h4>
              <div class="ti-map">
                <div v-for="v in preview.statuses" :key="v.value" v-field class="field">
                  <label class="ellipsis" :title="v.value">{{ v.value }} <span class="muted small">({{ v.count }})</span></label>
                  <Select :model-value="statuses[v.value] ?? ''" :options="statusOptions" option-label="label" option-value="value" :disabled="busy" @update:model-value="(x: string) => choose('status', v.value, x)" />
                </div>
              </div>
            </div>
            <div v-if="preview.fields.length" role="group" :aria-labelledby="ids.fields">
              <h4 :id="ids.fields" class="ti-sub">{{ t('testImport.fields') }}</h4>
              <p v-if="!preview.custom_fields.length" class="small muted" style="margin: 0 0 6px">{{ t('testImport.noCustomFields') }}</p>
              <div class="ti-map">
                <div v-for="f in preview.fields" :key="f.name" v-field class="field">
                  <label class="ellipsis" :title="f.name">{{ f.name }} <span class="muted small">({{ f.count }})</span></label>
                  <Select :model-value="fields[f.name] ?? ''" :options="fieldOptions" option-label="label" option-value="value" :disabled="busy || !preview.custom_fields.length" @update:model-value="(x: string) => choose('field', f.name, x)" />
                  <span class="small muted ellipsis" :title="f.sample">{{ f.sample }}</span>
                </div>
              </div>
            </div>
          </div>
        </section>

        <section :aria-label="t('exchange.placement')">
          <div class="form-grid">
            <div class="field">
              <span :id="ids.target" class="label">{{ t('exchange.testTarget') }}</span>
              <Button :label="rootPath || t('common.topLevel')" icon="pi pi-folder" severity="secondary" outlined class="ellipsis" :aria-describedby="ids.target" aria-haspopup="dialog" :disabled="busy" @click="pickFolder = true" />
            </div>
            <div class="field">
              <span class="row nowrap"><Checkbox v-model="createFolders" binary input-id="ti-folders" :disabled="busy" /><label for="ti-folders">{{ t('testImport.createFolders') }}</label></span>
              <span class="row nowrap"><Checkbox v-model="links" binary input-id="ti-links" :disabled="busy" /><label for="ti-links">{{ t('testImport.linkRequirements') }}</label></span>
              <span v-if="preview.history.executions" class="row nowrap">
                <Checkbox v-model="history" binary input-id="ti-history" :disabled="busy || !newResults" />
                <label for="ti-history">{{ t('testImport.history', { results: newResults, runs: preview.history.runs }) }}</label>
              </span>
            </div>
          </div>
        </section>

        <div class="ti-checks small">
          <span v-if="preview.summary.conflict" class="row nowrap">
            <Checkbox v-model="overwrite" binary input-id="ti-overwrite" /><label for="ti-overwrite">{{ t('testImport.overwrite', { count: preview.summary.conflict }) }}</label>
          </span>
          <span v-if="preview.summary.new" class="row nowrap">
            <Checkbox v-model="create" binary input-id="ti-create" /><label for="ti-create">{{ t('exchange.createNew', { count: preview.summary.new }) }}</label>
          </span>
        </div>

        <section :aria-labelledby="ids.rows">
          <h3 :id="ids.rows" class="section-title">{{ t('testImport.changes') }}</h3>
          <div class="row wrap ti-filter" role="group" :aria-label="t('exchange.filterLabel')">
            <Button
              v-for="a in TEST_IMPORT_ACTIONS"
              :key="a"
              size="small"
              :severity="filter.includes(a) ? undefined : 'secondary'"
              :outlined="!filter.includes(a)"
              :icon="icon[a]"
              :label="t(`testImport.action.${a}`) + ' ' + preview.summary[a]"
              :aria-pressed="filter.includes(a)"
              @click="toggleFilter(a)"
            />
          </div>
          <p class="small muted" style="margin: 6px 0">
            {{ filtered > SHOWN ? t('exchange.showingFirst', { shown: SHOWN, total: filtered }) : t('exchange.showingAll', { total: filtered }) }}
          </p>
          <DataTable :value="shownRows" data-key="index" size="small" scrollable scroll-height="45vh" :aria-labelledby="ids.rows">
            <Column :header="t('exchange.include')" style="width: 70px">
              <template #body="{ data }">
                <Checkbox
                  v-if="savableRow(data, opts)"
                  :model-value="!excluded.has(data.index)"
                  binary
                  :input-id="`${ids.row}-${data.index}`"
                  :aria-label="t('exchange.includeRow', { item: rowLabel(data) })"
                  @update:model-value="(v: boolean) => toggle(data.index, v)"
                />
              </template>
            </Column>
            <Column field="location" :header="t('exchange.location')" style="width: 130px" />
            <Column :header="t('exchange.item')" style="min-width: 200px">
              <template #body="{ data }">
                <span v-if="data.external_id" class="mono small">{{ data.external_id }}</span> {{ data.title }}
                <div class="small muted">
                  <span v-if="data.key" class="key">{{ data.key }}</span>
                  <span v-if="data.folder"> <i class="pi pi-folder" aria-hidden="true" /> {{ data.folder }}</span>
                  · {{ t('testImport.steps', { count: data.steps }, data.steps) }}
                  <span v-if="data.executions"> · {{ t('testImport.results', { count: data.executions }, data.executions) }}</span>
                </div>
              </template>
            </Column>
            <Column :header="t('exchange.statusColumn')" style="width: 130px">
              <template #body="{ data }">
                <Tag :severity="severity[data.action as TestImportAction]" :icon="icon[data.action as TestImportAction]" :value="t(`testImport.action.${data.action}`)" style="white-space: nowrap" />
              </template>
            </Column>
            <Column :header="t('exchange.details')" style="min-width: 240px">
              <template #body="{ data }">
                <ul v-if="data.errors.length" class="ti-list small error-text">
                  <li v-for="(e, i) in data.errors" :key="'e' + i">{{ e }}</li>
                </ul>
                <p v-if="data.links.length" class="small" style="margin: 0">{{ t('testImport.linksTo', { keys: data.links.join(', ') }) }}</p>
                <ul v-if="data.warnings.length" class="ti-list small muted">
                  <li v-for="(w, i) in data.warnings" :key="'w' + i">{{ w }}</li>
                </ul>
              </template>
            </Column>
            <template #empty><div class="empty">{{ t('exchange.none') }}</div></template>
          </DataTable>
        </section>
      </template>
    </div>
    <template #footer>
      <Button :label="result ? t('common.close') : t('common.cancel')" text severity="secondary" @click="visible = false" />
      <Button v-if="preview && !result" :label="t('exchange.saveN', { count: toSave }, toSave)" icon="pi pi-check" :loading="busy" :disabled="busy || !toSave" @click="save" />
    </template>
  </Dialog>
  <FolderPickerDialog v-model:visible="pickFolder" :folders="folders" :header="t('exchange.testTarget')" @pick="onFolder" />
</template>

<style scoped>
.ti-sources {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 20px;
  border: 0;
  margin: 0;
  padding: 0;
}
.ti-sources legend {
  margin-bottom: 6px;
  padding: 0;
}
.ti-map {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 10px 12px;
}
.ti-map .field {
  min-width: 0;
}
.ti-sub {
  font-size: 0.875rem;
  margin: 0 0 6px;
}
.ti-checks,
.ti-filter {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 20px;
}
.ti-filter {
  gap: 6px;
}
.ti-list {
  margin: 2px 0 0;
  padding-left: 18px;
}
.label {
  font-weight: 600;
  font-size: 0.875rem;
}
</style>
